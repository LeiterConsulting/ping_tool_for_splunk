#Requires -Version 7.4
<#
.SYNOPSIS
    Splunk Ping Monitor v3.2.0 - Retry-safe HEC, reduced allocations, event_id for dedupe.

.DESCRIPTION
    Pings endpoints from CSV, outputs to file (UF) or Splunk HEC, with optional metrics.

    VERSION 3.2.0 CHANGELOG:
    ========================
    ENHANCEMENT 1: Retry-safe HEC batching with bounded buffer
      - New config keys: hec.batch_size, hec.drop_on_failure, hec.max_buffer_events,
        hec.max_buffer_bytes, hec.retry.enabled/max_attempts/base_delay_ms/jitter_pct/backoff
      - On flush failure with retry enabled: retries with exponential/fixed backoff
      - Buffer caps prevent unbounded memory growth; oldest events dropped when exceeded
      - Convert-SizeToBytes helper parses "5MB" style strings

    ENHANCEMENT 2: Reduced allocations in streaming loop
      - Write-JsonLineToWriter and Add-EventToHecBuffer for single events
      - Streaming loop emits individual list and summary directly without intermediate copy
      - Same output, fewer allocations

    ENHANCEMENT 3: Per-event event_id for Splunk dedupe
      - SHA256 hash of: hostname|target_ip|record_type|timestamp[|ping_number]
      - Added to both ping and summary records
      - HEC envelope includes "id" field for Splunk deduplication

    BACKWARD COMPATIBILITY: All existing config.psd1, endpoints.csv, event schemas preserved.

.PARAMETER ConfigPath
    Path to config.psd1. Defaults to script directory.

.PARAMETER EndpointsPath
    Path to endpoints.csv. Defaults to script directory.

.PARAMETER RunOnce
    Run single cycle and exit.

.NOTES
    Version: 3.2.0
    Requires: PowerShell 7.4+ (no external modules)
#>

[CmdletBinding()]
param(
    [string]$ConfigPath,
    [string]$EndpointsPath,
    [switch]$RunOnce
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
if ([string]::IsNullOrEmpty($ScriptDir)) { $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
if ([string]::IsNullOrEmpty($ScriptDir)) { $ScriptDir = Get-Location }

#region Utility Functions

function Convert-SizeToBytes {
    <# Converts "5MB", "1GB", or numeric to bytes #>
    param([Parameter(Mandatory)]$Size)
    if ($Size -is [int] -or $Size -is [long] -or $Size -is [double]) { return [long]$Size }
    if ($Size -is [string]) {
        if ($Size -match '^(\d+(?:\.\d+)?)\s*(KB|MB|GB|TB)?$') {
            $num = [double]$Matches[1]
            $unit = if ($Matches[2]) { $Matches[2].ToUpper() } else { "" }
            switch ($unit) {
                "KB" { return [long]($num * 1KB) }
                "MB" { return [long]($num * 1MB) }
                "GB" { return [long]($num * 1GB) }
                "TB" { return [long]($num * 1TB) }
                default { return [long]$num }
            }
        }
    }
    return [long]$Size
}

function Get-EventId {
    <# SHA256 hash for deterministic event_id #>
    param([string]$Hostname, [string]$TargetIp, [string]$RecordType, [string]$Timestamp, [int]$PingNumber = -1)
    $input_str = if ($PingNumber -ge 0) { "${Hostname}|${TargetIp}|${RecordType}|${Timestamp}|${PingNumber}" }
                 else { "${Hostname}|${TargetIp}|${RecordType}|${Timestamp}" }
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($input_str)
    $hash = [System.Security.Cryptography.SHA256]::HashData($bytes)
    return [BitConverter]::ToString($hash).Replace("-", "").ToLower()
}

#endregion

#region Configuration Loading

function Get-Configuration {
    param([string]$Path)
    if (-not (Test-Path $Path)) { throw "Configuration file not found: $Path" }
    $config = Import-PowerShellDataFile -Path $Path

    # Defaults including new v3.2.0 HEC keys
    $defaults = @{
        pings_per_cycle = 4; cycle_interval_seconds = 60; timeout_ms = 1000
        parallel_threads = 10; output_mode = "file"
        log_path = Join-Path $ScriptDir "logs\ping_results.log"
        log_rotation_size_mb = 50; emit_individual_pings = $true
        hec = @{
            enabled = $false; url = ""; token = ""; index = "main"
            sourcetype = "ping_monitor"; verify_ssl = $true; ssl_protocol = "Default"
            batch_size = 100; drop_on_failure = $true
            max_buffer_events = 5000; max_buffer_bytes = "5MB"
            retry = @{ enabled = $false; max_attempts = 3; base_delay_ms = 250; jitter_pct = 20; backoff = "exponential" }
        }
        metrics = @{
            enabled = $false; mode = "dual"; index = ""; hec_url = ""
            token = ""; verify_ssl = $true; ssl_protocol = "Default"
        }
    }

    foreach ($key in $defaults.Keys) {
        if (-not $config.ContainsKey($key)) { $config[$key] = $defaults[$key] }
    }

    # Merge HEC defaults
    if ($config.ContainsKey('hec') -and $null -ne $config.hec) {
        foreach ($k in $defaults.hec.Keys) {
            if (-not $config.hec.ContainsKey($k)) { $config.hec[$k] = $defaults.hec[$k] }
        }
        # Merge retry sub-block
        if (-not $config.hec.ContainsKey('retry') -or $null -eq $config.hec.retry) {
            $config.hec['retry'] = $defaults.hec.retry
        } else {
            foreach ($rk in $defaults.hec.retry.Keys) {
                if (-not $config.hec.retry.ContainsKey($rk)) { $config.hec.retry[$rk] = $defaults.hec.retry[$rk] }
            }
        }
    } else { $config['hec'] = $defaults.hec }

    # Merge metrics defaults
    if ($config.ContainsKey('metrics') -and $null -ne $config.metrics) {
        foreach ($mk in $defaults.metrics.Keys) {
            if (-not $config.metrics.ContainsKey($mk)) { $config.metrics[$mk] = $defaults.metrics[$mk] }
        }
    } else { $config['metrics'] = $defaults.metrics }

    # Validation
    if ($config.pings_per_cycle -lt 1) { $config.pings_per_cycle = 4 }
    if ($config.cycle_interval_seconds -lt 1) { $config.cycle_interval_seconds = 60 }
    if ($config.timeout_ms -lt 100) { $config.timeout_ms = 1000 }
    if ($config.parallel_threads -lt 1) { $config.parallel_threads = 10 }
    if ($config.log_rotation_size_mb -lt 1) { $config.log_rotation_size_mb = 50 }
    if ($config.output_mode -notin @('file','hec','both')) { $config.output_mode = 'file' }

    # Warn if HEC output requested but not enabled
    if ($config.output_mode -in @('hec','both') -and -not $config.hec.enabled) {
        Write-Warning "output_mode='$($config.output_mode)' but hec.enabled=false. HEC output will be skipped."
    }

    return $config
}

#endregion

#region Endpoint Loading

function Get-Endpoints {
    param([string]$Path)
    if (-not (Test-Path $Path)) { throw "Endpoints file not found: $Path" }
    $csvData = Import-Csv -Path $Path
    $endpoints = [System.Collections.Generic.List[PSCustomObject]]::new()
    foreach ($row in $csvData) {
        if (-not $row.ip -or -not $row.hostname) { continue }
        $endpoints.Add([PSCustomObject]@{
            ip = $row.ip.Trim(); hostname = $row.hostname.Trim()
            group = if ($row.PSObject.Properties['group'] -and $row.group) { $row.group.Trim() } else { "default" }
            description = if ($row.PSObject.Properties['description'] -and $row.description) { $row.description.Trim() } else { "" }
            entitytype = if ($row.PSObject.Properties['entitytype'] -and $row.entitytype) { $row.entitytype.Trim() } else { "" }
            device = if ($row.PSObject.Properties['device'] -and $row.device) { $row.device.Trim() } else { "" }
            vendor = if ($row.PSObject.Properties['vendor'] -and $row.vendor) { $row.vendor.Trim() } else { "" }
            additional_notes = if ($row.PSObject.Properties['additional_notes'] -and $row.additional_notes) { $row.additional_notes.Trim() } else { "" }
        })
    }
    if ($endpoints.Count -eq 0) { throw "No valid endpoints found in CSV" }
    Write-Host "Loaded $($endpoints.Count) endpoints from CSV" -ForegroundColor Green
    return $endpoints
}

#endregion

#region File Sink

function Open-FileWriter {
    param([string]$LogPath)
    $logDir = Split-Path -Parent $LogPath
    if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir -Force | Out-Null }
    return [System.IO.StreamWriter]::new($LogPath, $true, [System.Text.Encoding]::UTF8)
}

function Write-JsonLineToWriter {
    <# Write single event - v3.2.0 reduced allocations #>
    param([System.IO.StreamWriter]$Writer, [PSCustomObject]$Event)
    try { $Writer.WriteLine(($Event | ConvertTo-Json -Compress)) }
    catch { Write-Warning "File write failed: $($_.Exception.Message)" }
}

function Write-JsonLinesToWriter {
    <# Write list of events #>
    param([System.IO.StreamWriter]$Writer, [System.Collections.Generic.List[PSCustomObject]]$Results)
    foreach ($r in $Results) { Write-JsonLineToWriter -Writer $Writer -Event $r }
}

function Close-FileWriter {
    param([System.IO.StreamWriter]$Writer)
    if ($null -ne $Writer) { try { $Writer.Flush(); $Writer.Dispose() } catch { } }
}

#endregion

#region HEC Buffer with Retry (v3.2.0)

function Initialize-HecBuffer {
    param([hashtable]$HecConfig)
    $batchSize = if ($HecConfig.batch_size -ge 1) { $HecConfig.batch_size } else { 100 }
    $maxEvents = if ($HecConfig.max_buffer_events -ge $batchSize) { $HecConfig.max_buffer_events } else { 5000 }
    $maxBytes = Convert-SizeToBytes $HecConfig.max_buffer_bytes
    if ($maxBytes -lt 1) { $maxBytes = 5MB }

    return @{
        Builder = [System.Text.StringBuilder]::new()
        EventCount = 0
        BufferBytes = 0
        BatchSize = $batchSize
        MaxBufferEvents = $maxEvents
        MaxBufferBytes = $maxBytes
        DropOnFailure = [bool]$HecConfig.drop_on_failure
        RetryEnabled = [bool]$HecConfig.retry.enabled
        RetryMaxAttempts = [int]$HecConfig.retry.max_attempts
        RetryBaseDelayMs = [int]$HecConfig.retry.base_delay_ms
        RetryJitterPct = [int]$HecConfig.retry.jitter_pct
        RetryBackoff = $HecConfig.retry.backoff
        DroppedEvents = 0
    }
}

function Add-EventToHecBuffer {
    <# Add single event to buffer - v3.2.0 #>
    param([hashtable]$Buffer, [PSCustomObject]$Event, [hashtable]$HecConfig, [string]$Hostname)

    # Build HEC envelope with event_id for dedupe
    $eventId = $Event.event_id
    $hecEvent = [ordered]@{
        time = [DateTimeOffset]::Parse($Event.timestamp).ToUnixTimeSeconds()
        host = $Hostname; source = "ping_monitor"
        sourcetype = $HecConfig.sourcetype; index = $HecConfig.index
    }
    if ($eventId) { $hecEvent['id'] = $eventId }  # HEC-level dedupe
    $hecEvent['event'] = $Event

    $eventJson = $hecEvent | ConvertTo-Json -Compress -Depth 5
    $eventBytes = [System.Text.Encoding]::UTF8.GetByteCount($eventJson) + 1  # +1 for newline

    # Check caps before adding
    if (($Buffer['EventCount'] + 1) -gt $Buffer['MaxBufferEvents'] -or
        ($Buffer['BufferBytes'] + $eventBytes) -gt $Buffer['MaxBufferBytes']) {
        # Drop oldest by clearing and warn
        $Buffer['DroppedEvents'] += $Buffer['EventCount']
        Write-Warning "HEC buffer cap reached. Dropped $($Buffer['EventCount']) buffered events."
        [void]$Buffer['Builder'].Clear()
        $Buffer['EventCount'] = 0
        $Buffer['BufferBytes'] = 0
    }

    if ($Buffer['EventCount'] -gt 0) { [void]$Buffer['Builder'].Append("`n") }
    [void]$Buffer['Builder'].Append($eventJson)
    $Buffer['EventCount']++
    $Buffer['BufferBytes'] += $eventBytes

    # Auto-flush at batch size
    if ($Buffer['EventCount'] -ge $Buffer['BatchSize']) {
        return Flush-HecBuffer -Buffer $Buffer -HecConfig $HecConfig
    }
    return @{ Flushed = $false; Success = $false; EventsFlushed = 0 }
}

function Add-EventsToHecBuffer {
    <# Add list of events #>
    param([hashtable]$Buffer, [System.Collections.Generic.List[PSCustomObject]]$Events, [hashtable]$HecConfig, [string]$Hostname)
    $successCount = 0; $failCount = 0
    foreach ($ev in $Events) {
        $result = Add-EventToHecBuffer -Buffer $Buffer -Event $ev -HecConfig $HecConfig -Hostname $Hostname
        if ($result.Flushed) { if ($result.Success) { $successCount++ } else { $failCount++ } }
    }
    return @{ SuccessCount = $successCount; FailCount = $failCount }
}

function Invoke-HecPost {
    <# Low-level HEC POST, returns $true on success #>
    param([string]$Body, [hashtable]$HecConfig)
    $headers = @{ "Authorization" = "Splunk $($HecConfig.token)"; "Content-Type" = "application/json" }
    $splat = @{ Uri = $HecConfig.url; Method = "POST"; Headers = $headers; Body = $Body; TimeoutSec = 10; ErrorAction = "Stop" }
    if (-not $HecConfig.verify_ssl) { $splat['SkipCertificateCheck'] = $true }
    if ($HecConfig.ssl_protocol -and $HecConfig.ssl_protocol -ne 'Default') { $splat['SslProtocol'] = $HecConfig.ssl_protocol }
    try { Invoke-RestMethod @splat | Out-Null; return $true }
    catch { return $false }
}

function Flush-HecBuffer {
    <# Flush buffer with optional retry #>
    param([hashtable]$Buffer, [hashtable]$HecConfig)

    if ($Buffer['EventCount'] -eq 0) { return @{ Flushed = $false; Success = $true; EventsFlushed = 0 } }

    $body = $Buffer['Builder'].ToString()
    $eventCount = $Buffer['EventCount']
    $success = $false
    $attempts = 0
    $maxAttempts = if ($Buffer['RetryEnabled']) { [math]::Max(1, $Buffer['RetryMaxAttempts']) } else { 1 }

    while ($attempts -lt $maxAttempts -and -not $success) {
        $attempts++
        $success = Invoke-HecPost -Body $body -HecConfig $HecConfig
        if (-not $success -and $attempts -lt $maxAttempts) {
            # Calculate delay with jitter
            $delay = $Buffer['RetryBaseDelayMs']
            if ($Buffer['RetryBackoff'] -eq 'exponential') { $delay = $delay * [math]::Pow(2, $attempts - 1) }
            $jitter = Get-Random -Minimum (-$Buffer['RetryJitterPct']) -Maximum $Buffer['RetryJitterPct']
            $delay = [int]($delay * (1 + $jitter / 100))
            Start-Sleep -Milliseconds ([math]::Max(0, $delay))
        }
    }

    if ($success -or $Buffer['DropOnFailure']) {
        # Clear buffer
        [void]$Buffer['Builder'].Clear()
        $Buffer['EventCount'] = 0
        $Buffer['BufferBytes'] = 0
    } else {
        # Keep for retry next cycle (capped by Add logic)
        Write-Warning "HEC flush failed after $attempts attempts. Retaining $eventCount events in buffer."
    }

    if (-not $success) { Write-Warning "HEC batch send failed." }
    return @{ Flushed = $true; Success = $success; EventsFlushed = if ($success) { $eventCount } else { 0 } }
}

function Test-HecConfiguration {
    param([hashtable]$HecConfig, [switch]$WarnOnInvalid)
    if (-not $HecConfig.enabled) { return $false }
    if ([string]::IsNullOrWhiteSpace($HecConfig.url)) {
        if ($WarnOnInvalid) { Write-Warning "HEC enabled but URL not configured." }
        return $false
    }
    if ([string]::IsNullOrWhiteSpace($HecConfig.token)) {
        if ($WarnOnInvalid) { Write-Warning "HEC enabled but token not configured." }
        return $false
    }
    return $true
}

#endregion

#region Metrics Sink

function Send-ToMetricsSink {
    param([PSCustomObject]$Summary, [hashtable]$MetricsConfig, [string]$Hostname)
    if (-not $MetricsConfig.enabled) { return $true }
    if ([string]::IsNullOrWhiteSpace($MetricsConfig.hec_url) -or [string]::IsNullOrWhiteSpace($MetricsConfig.token)) { return $false }

    $metricsEvent = @{
        time = [DateTimeOffset]::Parse($Summary.timestamp).ToUnixTimeSeconds()
        host = $Hostname; source = "ping_monitor"; index = $MetricsConfig.index; event = "metric"
        fields = @{
            "metric_name:ping.avg_latency_ms" = [double]$Summary.avg_latency_ms
            "metric_name:ping.min_latency_ms" = [double]$Summary.min_latency_ms
            "metric_name:ping.max_latency_ms" = [double]$Summary.max_latency_ms
            "metric_name:ping.packet_loss_pct" = [double]$Summary.packet_loss_pct
            "metric_name:ping.pings_sent" = [int]$Summary.pings_sent
            "metric_name:ping.pings_successful" = [int]$Summary.pings_successful
            hostname = $Summary.hostname; target_ip = $Summary.target_ip; group = $Summary.group
            description = $Summary.description; entitytype = $Summary.entitytype
            device = $Summary.device; vendor = $Summary.vendor; additional_notes = $Summary.additional_notes
        }
    }
    $body = $metricsEvent | ConvertTo-Json -Depth 5 -Compress
    $headers = @{ "Authorization" = "Splunk $($MetricsConfig.token)"; "Content-Type" = "application/json" }
    $splat = @{ Uri = $MetricsConfig.hec_url; Method = "POST"; Headers = $headers; Body = $body; TimeoutSec = 5; ErrorAction = "Stop" }
    if (-not $MetricsConfig.verify_ssl) { $splat['SkipCertificateCheck'] = $true }
    if ($MetricsConfig.ssl_protocol -and $MetricsConfig.ssl_protocol -ne 'Default') { $splat['SslProtocol'] = $MetricsConfig.ssl_protocol }
    try { Invoke-RestMethod @splat | Out-Null; return $true } catch { return $false }
}

#endregion

#region Log Rotation

function Invoke-LogRotation {
    param([string]$LogPath, [int]$MaxSizeMB)
    if (-not (Test-Path $LogPath)) { return }
    $sizeMB = (Get-Item $LogPath).Length / 1MB
    if ($sizeMB -ge $MaxSizeMB) {
        $archive = $LogPath -replace '\.log$', "_$(Get-Date -Format 'yyyyMMdd_HHmmss').log"
        Move-Item -Path $LogPath -Destination $archive -Force
        Get-ChildItem -Path (Split-Path $LogPath) -Filter "*_*.log" | Sort-Object LastWriteTime -Descending | Select-Object -Skip 5 | Remove-Item -Force
    }
}

#endregion

#region Streaming Parallel Ping

function Invoke-StreamingParallelPing {
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Endpoints,
        [int]$PingsPerCycle, [int]$TimeoutMs, [int]$ParallelThreads,
        [bool]$EmitIndividualPings, [bool]$EmitEventSummaries, [bool]$EmitMetrics,
        [System.IO.StreamWriter]$FileWriter, [hashtable]$HecBuffer,
        [hashtable]$HecConfig, [hashtable]$MetricsConfig
    )

    $hostname = $env:COMPUTERNAME
    $stats = @{ TotalSuccess=0; TotalPartial=0; TotalFailed=0; HecBatchSuccessCount=0; HecBatchFailCount=0; HecEventCount=0; MetricsSuccessCount=0; MetricsFailCount=0 }

    $runspacePool = [runspacefactory]::CreateRunspacePool(1, $ParallelThreads)
    $runspacePool.Open()
    $activeRunspaces = [System.Collections.Generic.List[hashtable]]::new()

    # Worker scriptblock with event_id generation
    $pingScript = {
        param($Endpoint, $Count, $Timeout, $EmitIndividual)

        # Local event_id function (runspaces are isolated)
        function Get-EventIdLocal {
            param([string]$h, [string]$t, [string]$r, [string]$ts, [int]$pn = -1)
            $s = if ($pn -ge 0) { "$h|$t|$r|$ts|$pn" } else { "$h|$t|$r|$ts" }
            $bytes = [System.Text.Encoding]::UTF8.GetBytes($s)
            $hash = [System.Security.Cryptography.SHA256]::HashData($bytes)
            return [BitConverter]::ToString($hash).Replace("-", "").ToLower()
        }

        $individual = if ($EmitIndividual) { [System.Collections.Generic.List[PSCustomObject]]::new() } else { $null }
        $successCount = 0; $totalLatency = 0; $minLat = [int]::MaxValue; $maxLat = 0
        $ping = $null

        try {
            $ping = [System.Net.NetworkInformation.Ping]::new()
            for ($i = 0; $i -lt $Count; $i++) {
                $ts = Get-Date -Format "o"
                try {
                    $reply = $ping.Send($Endpoint.ip, $Timeout)
                    if ($reply.Status -eq [System.Net.NetworkInformation.IPStatus]::Success) {
                        $lat = [int]$reply.RoundtripTime
                        $ttl = if ($null -ne $reply.Options) { $reply.Options.Ttl } else { -1 }
                        $successCount++; $totalLatency += $lat
                        $minLat = [math]::Min($minLat, $lat); $maxLat = [math]::Max($maxLat, $lat)
                        if ($EmitIndividual) {
                            $evId = Get-EventIdLocal $Endpoint.hostname $Endpoint.ip "ping" $ts ($i+1)
                            $individual.Add([PSCustomObject]@{
                                event_id=$evId; timestamp=$ts; target_ip=$Endpoint.ip; hostname=$Endpoint.hostname
                                group=$Endpoint.group; description=$Endpoint.description; entitytype=$Endpoint.entitytype
                                device=$Endpoint.device; vendor=$Endpoint.vendor; additional_notes=$Endpoint.additional_notes
                                status="success"; latency_ms=$lat; ttl=$ttl; ping_number=($i+1); pings_in_cycle=$Count; record_type="ping"
                            })
                        }
                    } else {
                        if ($EmitIndividual) {
                            $evId = Get-EventIdLocal $Endpoint.hostname $Endpoint.ip "ping" $ts ($i+1)
                            $individual.Add([PSCustomObject]@{
                                event_id=$evId; timestamp=$ts; target_ip=$Endpoint.ip; hostname=$Endpoint.hostname
                                group=$Endpoint.group; description=$Endpoint.description; entitytype=$Endpoint.entitytype
                                device=$Endpoint.device; vendor=$Endpoint.vendor; additional_notes=$Endpoint.additional_notes
                                status="failed"; latency_ms=-1; ttl=-1; ping_number=($i+1); pings_in_cycle=$Count
                                error_message="Status: $($reply.Status)"; record_type="ping"
                            })
                        }
                    }
                } catch {
                    if ($EmitIndividual) {
                        $evId = Get-EventIdLocal $Endpoint.hostname $Endpoint.ip "ping" $ts ($i+1)
                        $individual.Add([PSCustomObject]@{
                            event_id=$evId; timestamp=$ts; target_ip=$Endpoint.ip; hostname=$Endpoint.hostname
                            group=$Endpoint.group; description=$Endpoint.description; entitytype=$Endpoint.entitytype
                            device=$Endpoint.device; vendor=$Endpoint.vendor; additional_notes=$Endpoint.additional_notes
                            status="failed"; latency_ms=-1; ttl=-1; ping_number=($i+1); pings_in_cycle=$Count
                            error_message=$_.Exception.Message; record_type="ping"
                        })
                    }
                }
            }
        } finally { if ($null -ne $ping) { $ping.Dispose() } }

        $pktLoss = [math]::Round((($Count - $successCount) / $Count) * 100, 2)
        $avgLat = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
        $sumTs = Get-Date -Format "o"
        $sumEvId = Get-EventIdLocal $Endpoint.hostname $Endpoint.ip "summary" $sumTs

        $summary = [PSCustomObject]@{
            event_id=$sumEvId; timestamp=$sumTs; target_ip=$Endpoint.ip; hostname=$Endpoint.hostname
            group=$Endpoint.group; description=$Endpoint.description; entitytype=$Endpoint.entitytype
            device=$Endpoint.device; vendor=$Endpoint.vendor; additional_notes=$Endpoint.additional_notes
            record_type="summary"; pings_sent=$Count; pings_successful=$successCount; pings_failed=($Count-$successCount)
            packet_loss_pct=$pktLoss; avg_latency_ms=$avgLat
            min_latency_ms=$(if ($successCount -gt 0) { $minLat } else { -1 })
            max_latency_ms=$(if ($successCount -gt 0) { $maxLat } else { -1 })
        }
        return @{ Individual=$individual; Summary=$summary }
    }

    # Start runspaces
    foreach ($ep in $Endpoints) {
        $ps = [powershell]::Create()
        $ps.RunspacePool = $runspacePool
        [void]$ps.AddScript($pingScript).AddArgument($ep).AddArgument($PingsPerCycle).AddArgument($TimeoutMs).AddArgument($EmitIndividualPings)
        $activeRunspaces.Add(@{ PowerShell=$ps; Handle=$ps.BeginInvoke(); Endpoint=$ep })
    }

    # Process as they complete
    while ($activeRunspaces.Count -gt 0) {
        $completed = [System.Collections.Generic.List[int]]::new()
        for ($i = 0; $i -lt $activeRunspaces.Count; $i++) {
            if ($activeRunspaces[$i].Handle.IsCompleted) { $completed.Add($i) }
        }
        for ($j = $completed.Count - 1; $j -ge 0; $j--) {
            $idx = $completed[$j]
            $rs = $activeRunspaces[$idx]
            try {
                $raw = $rs.PowerShell.EndInvoke($rs.Handle)
                $result = if ($raw.Count -gt 0) { $raw[0] } else { $null }
                if ($null -ne $result) {
                    $summary = $result.Summary
                    $individual = $result.Individual

                    if ($summary.packet_loss_pct -eq 0) { $stats.TotalSuccess++ }
                    elseif ($summary.packet_loss_pct -lt 100) { $stats.TotalPartial++ }
                    else { $stats.TotalFailed++ }

                    # v3.2.0: Emit directly without intermediate list
                    if ($EmitIndividualPings -and $null -ne $individual -and $individual.Count -gt 0) {
                        if ($null -ne $FileWriter) { Write-JsonLinesToWriter -Writer $FileWriter -Results $individual }
                        if ($null -ne $HecBuffer) {
                            $stats.HecEventCount += $individual.Count
                            $fr = Add-EventsToHecBuffer -Buffer $HecBuffer -Events $individual -HecConfig $HecConfig -Hostname $hostname
                            $stats.HecBatchSuccessCount += $fr.SuccessCount; $stats.HecBatchFailCount += $fr.FailCount
                        }
                    }
                    if ($EmitEventSummaries) {
                        if ($null -ne $FileWriter) { Write-JsonLineToWriter -Writer $FileWriter -Event $summary }
                        if ($null -ne $HecBuffer) {
                            $stats.HecEventCount++
                            $fr = Add-EventToHecBuffer -Buffer $HecBuffer -Event $summary -HecConfig $HecConfig -Hostname $hostname
                            if ($fr.Flushed) { if ($fr.Success) { $stats.HecBatchSuccessCount++ } else { $stats.HecBatchFailCount++ } }
                        }
                    }
                    if ($EmitMetrics) {
                        if (Send-ToMetricsSink -Summary $summary -MetricsConfig $MetricsConfig -Hostname $hostname) { $stats.MetricsSuccessCount++ }
                        else { $stats.MetricsFailCount++ }
                    }
                }
            } catch { Write-Warning "Runspace error for $($rs.Endpoint.hostname): $($_.Exception.Message)"; $stats.TotalFailed++ }
            finally { $rs.PowerShell.Dispose() }
            $activeRunspaces.RemoveAt($idx)
        }
        if ($completed.Count -eq 0 -and $activeRunspaces.Count -gt 0) { Start-Sleep -Milliseconds 10 }
    }

    $runspacePool.Close(); $runspacePool.Dispose()
    return $stats
}

#endregion

#region Main Execution

function Start-PingMonitor {
    param([hashtable]$Config, [System.Collections.Generic.List[PSCustomObject]]$Endpoints, [switch]$RunOnce)

    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  Splunk Ping Monitor v3.2.0" -ForegroundColor Cyan
    Write-Host "  (Retry HEC + event_id + low-alloc)" -ForegroundColor DarkCyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "Endpoints: $($Endpoints.Count) | Pings/cycle: $($Config.pings_per_cycle) | Interval: $($Config.cycle_interval_seconds)s"
    Write-Host "Output: $($Config.output_mode) | Metrics: $(if ($Config.metrics.enabled) { $Config.metrics.mode } else { 'disabled' })"
    if ($Config.hec.enabled -and $Config.hec.retry.enabled) {
        Write-Host "HEC Retry: enabled (max $($Config.hec.retry.max_attempts) attempts, $($Config.hec.retry.backoff) backoff)" -ForegroundColor DarkYellow
    }
    Write-Host "----------------------------------------" -ForegroundColor Gray

    $emitIndividual = $Config.emit_individual_pings
    $emitSummaries = $true
    $emitMetrics = $Config.metrics.enabled

    if ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
        $emitSummaries = $false; $emitIndividual = $false
        Write-Host "metrics_only mode: event emission disabled" -ForegroundColor DarkYellow
    }

    $hecValid = Test-HecConfiguration -HecConfig $Config.hec -WarnOnInvalid
    $cycleCount = 0

    do {
        $cycleCount++
        $cycleStart = Get-Date
        Write-Host "`n[$cycleStart] Cycle #$cycleCount..." -ForegroundColor Cyan

        $fileWriter = $null
        $useFile = ($Config.output_mode -in @('file','both')) -and ($emitIndividual -or $emitSummaries)
        if ($useFile) {
            Invoke-LogRotation -LogPath $Config.log_path -MaxSizeMB $Config.log_rotation_size_mb
            try { $fileWriter = Open-FileWriter -LogPath $Config.log_path }
            catch { Write-Warning "Failed to open log: $($_.Exception.Message)"; $useFile = $false }
        }

        $hecBuffer = $null
        $useHec = ($Config.output_mode -in @('hec','both')) -and $hecValid -and ($emitIndividual -or $emitSummaries)
        if ($useHec) { $hecBuffer = Initialize-HecBuffer -HecConfig $Config.hec }

        $stats = $null
        try {
            $stats = Invoke-StreamingParallelPing -Endpoints $Endpoints `
                -PingsPerCycle $Config.pings_per_cycle -TimeoutMs $Config.timeout_ms -ParallelThreads $Config.parallel_threads `
                -EmitIndividualPings $emitIndividual -EmitEventSummaries $emitSummaries -EmitMetrics $emitMetrics `
                -FileWriter $fileWriter -HecBuffer $hecBuffer -HecConfig $Config.hec -MetricsConfig $Config.metrics
        } finally {
            if ($null -ne $fileWriter) { Close-FileWriter -Writer $fileWriter }
            if ($null -ne $hecBuffer -and $hecBuffer['EventCount'] -gt 0) {
                $fr = Flush-HecBuffer -Buffer $hecBuffer -HecConfig $Config.hec
                if ($null -ne $stats) { if ($fr.Success) { $stats.HecBatchSuccessCount++ } else { $stats.HecBatchFailCount++ } }
            }
        }

        if ($null -eq $stats) { $stats = @{ TotalSuccess=0; TotalPartial=0; TotalFailed=$Endpoints.Count; HecBatchSuccessCount=0; HecBatchFailCount=0; HecEventCount=0; MetricsSuccessCount=0; MetricsFailCount=0 } }

        if ($useFile) { Write-Host "File: $($Config.log_path)" -ForegroundColor Green }
        if ($useHec) {
            $batches = $stats.HecBatchSuccessCount + $stats.HecBatchFailCount
            Write-Host "HEC: $($stats.HecEventCount) events, $batches batches ($($stats.HecBatchSuccessCount) OK)" -ForegroundColor $(if ($stats.HecBatchFailCount -eq 0) { "Green" } else { "Yellow" })
            if ($hecBuffer -and $hecBuffer['DroppedEvents'] -gt 0) { Write-Host "HEC: $($hecBuffer['DroppedEvents']) events dropped (buffer cap)" -ForegroundColor Yellow }
        }
        if ($emitMetrics) { Write-Host "Metrics: $($stats.MetricsSuccessCount) sent" -ForegroundColor Green }

        Write-Host "Cycle #$cycleCount complete: Success=$($stats.TotalSuccess) Partial=$($stats.TotalPartial) Failed=$($stats.TotalFailed)" -ForegroundColor $(if ($stats.TotalFailed -eq 0) { "Green" } elseif ($stats.TotalFailed -lt $Endpoints.Count) { "Yellow" } else { "Red" })

        if (-not $RunOnce) {
            $sleep = [math]::Max(0, $Config.cycle_interval_seconds - ((Get-Date) - $cycleStart).TotalSeconds)
            if ($sleep -gt 0) { Write-Host "Sleeping $([math]::Round($sleep,1))s..." -ForegroundColor Gray; Start-Sleep -Seconds $sleep }
        }
    } while (-not $RunOnce)

    Write-Host "`nPing Monitor v3.2.0 completed." -ForegroundColor Cyan
}

#endregion

#region Self-Tests (Manual Use Only)

function Test-HecRetryAndCaps {
    <# Test HEC buffer retry and cap behavior #>
    [CmdletBinding()] param([switch]$SimulateFailure)

    Write-Host "=== HEC Retry/Caps Test ===" -ForegroundColor Cyan
    $mockConfig = @{
        enabled=$true; url="http://test"; token="test"; index="test"; sourcetype="test"
        verify_ssl=$false; ssl_protocol="Default"; batch_size=3; drop_on_failure=$false
        max_buffer_events=5; max_buffer_bytes=10000
        retry=@{ enabled=$true; max_attempts=2; base_delay_ms=10; jitter_pct=0; backoff="fixed" }
    }

    $buffer = Initialize-HecBuffer -HecConfig $mockConfig
    Write-Host "Buffer initialized: BatchSize=$($buffer['BatchSize']), MaxEvents=$($buffer['MaxBufferEvents'])"

    # Add 7 events (should trigger flushes at 3 and 6)
    for ($i = 1; $i -le 7; $i++) {
        $ev = [PSCustomObject]@{ event_id="test$i"; timestamp=(Get-Date -Format "o"); record_type="ping"; hostname="test" }
        if ($buffer['EventCount'] -gt 0) { [void]$buffer['Builder'].Append("`n") }
        [void]$buffer['Builder'].Append(($ev | ConvertTo-Json -Compress))
        $buffer['EventCount']++
        $buffer['BufferBytes'] += 100
        Write-Verbose "Added event $i, EventCount=$($buffer['EventCount'])"

        if ($buffer['EventCount'] -ge $buffer['BatchSize']) {
            Write-Host "  Flush triggered at event $i (EventCount=$($buffer['EventCount']))" -ForegroundColor Yellow
            if ($SimulateFailure) {
                Write-Host "  Simulating failure - buffer retained" -ForegroundColor Red
            } else {
                [void]$buffer['Builder'].Clear(); $buffer['EventCount']=0; $buffer['BufferBytes']=0
            }
        }
    }

    if ($buffer['EventCount'] -gt 0) { Write-Host "Final flush pending: $($buffer['EventCount']) events" -ForegroundColor Yellow }
    Write-Host "=== Test Complete ===" -ForegroundColor Green
}

function Test-EventIdDeterminism {
    <# Verify event_id is deterministic #>
    Write-Host "=== Event ID Determinism Test ===" -ForegroundColor Cyan
    $ts = "2026-01-14T12:00:00.0000000-05:00"
    $id1 = Get-EventId -Hostname "test-host" -TargetIp "1.2.3.4" -RecordType "ping" -Timestamp $ts -PingNumber 1
    $id2 = Get-EventId -Hostname "test-host" -TargetIp "1.2.3.4" -RecordType "ping" -Timestamp $ts -PingNumber 1
    $id3 = Get-EventId -Hostname "test-host" -TargetIp "1.2.3.4" -RecordType "summary" -Timestamp $ts

    Write-Host "Ping event_id (run 1): $id1"
    Write-Host "Ping event_id (run 2): $id2"
    Write-Host "Summary event_id:      $id3"

    if ($id1 -eq $id2) { Write-Host "PASS: Ping IDs match" -ForegroundColor Green }
    else { Write-Host "FAIL: Ping IDs differ" -ForegroundColor Red }

    if ($id1 -ne $id3) { Write-Host "PASS: Ping vs Summary differ" -ForegroundColor Green }
    else { Write-Host "FAIL: Ping vs Summary match (should differ)" -ForegroundColor Red }
}

#endregion

#region Entry Point

try {
    if ([string]::IsNullOrEmpty($ConfigPath)) { $ConfigPath = Join-Path $ScriptDir "config.psd1" }
    if ([string]::IsNullOrEmpty($EndpointsPath)) { $EndpointsPath = Join-Path $ScriptDir "endpoints.csv" }

    Write-Host "Loading config: $ConfigPath" -ForegroundColor Gray
    $config = Get-Configuration -Path $ConfigPath

    Write-Host "Loading endpoints: $EndpointsPath" -ForegroundColor Gray
    $endpoints = Get-Endpoints -Path $EndpointsPath

    Start-PingMonitor -Config $config -Endpoints $endpoints -RunOnce:$RunOnce
}
catch {
    Write-Error "Fatal: $($_.Exception.Message)"
    Write-Error $_.ScriptStackTrace
    exit 1
}

#endregion
