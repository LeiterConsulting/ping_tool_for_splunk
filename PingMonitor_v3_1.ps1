#Requires -Version 7.4
<#
.SYNOPSIS
    Splunk Ping Monitor v3.1 - True streaming runspaces, lowest RAM, backward compatible.

.DESCRIPTION
    This script pings endpoints defined in a CSV file and outputs results either to a log file
    (for Splunk Universal Forwarder ingestion) or directly to Splunk via HTTP Event Collector (HEC).

    VERSION 3.1.0 CHANGES (True Streaming):
    =======================================
    Building on v3.0.0, this release implements true streaming execution:

    A) True streaming runspace completion:
       - Process runspaces AS THEY COMPLETE (Handle.IsCompleted polling)
       - Immediately emit results and dispose runspace
       - Prevents completed results from piling up in memory

    B) Eliminated cycle-wide result concatenation:
       - No $allResults list built for entire cycle
       - Results emitted directly as each endpoint completes
       - Summaries and individual pings handled separately

    C) Gated individual ping object creation:
       - When emit_individual_pings=false, workers don't create ping objects at all
       - Only counters computed, only summary built
       - Significant memory savings for summary-only mode

    D) Incremental sink streaming:
       - File output streamed per-endpoint completion
       - HEC batching with immediate flush per endpoint batch
       - Metrics sent as summaries arrive

    E) Ping object reuse per worker:
       - Single System.Net.NetworkInformation.Ping instance per endpoint
       - Reused for all pings_per_cycle iterations
       - Disposed once at worker end

    F) TTL null safety:
       - Handles PingReply.Options = $null gracefully
       - Returns -1 for TTL when Options unavailable

    BACKWARD COMPATIBILITY:
    =======================
    - All existing config.psd1 files work unchanged
    - All existing endpoints.csv files work unchanged
    - Event JSON structure identical to v2/v3
    - Metrics payload structure identical to v2/v3
    - HEC sourcetype, source, index behavior unchanged

.PARAMETER ConfigPath
    Path to the PowerShell data configuration file (.psd1). Defaults to config.psd1 in the script directory.

.PARAMETER EndpointsPath
    Path to the CSV file containing endpoints. Defaults to endpoints.csv in the script directory.

.PARAMETER RunOnce
    If specified, runs a single ping cycle and exits. Otherwise runs continuously.

.EXAMPLE
    .\PingMonitor_v3_1.ps1
    Runs continuously using default config.psd1 and endpoints.csv

.EXAMPLE
    .\PingMonitor_v3_1.ps1 -ConfigPath "C:\Config\myconfig.psd1" -RunOnce
    Runs a single cycle with custom config path

.NOTES
    Author: Splunk Ping Monitor
    Version: 3.1.0
    Requires: PowerShell 7.4+ (no external modules - airgap friendly)
    
    Memory Characteristics:
    - RAM usage stabilizes immediately (no accumulation)
    - Results streamed as endpoints complete
    - Individual ping objects only created when needed
    - Single Ping instance reused per worker
#>

[CmdletBinding()]
param(
    [Parameter()]
    [string]$ConfigPath,

    [Parameter()]
    [string]$EndpointsPath,

    [Parameter()]
    [switch]$RunOnce
)

# Set strict mode for better error handling
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Get script directory for relative paths
$ScriptDir = $PSScriptRoot
if ([string]::IsNullOrEmpty($ScriptDir)) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if ([string]::IsNullOrEmpty($ScriptDir)) {
    $ScriptDir = Get-Location
}

#region Configuration Loading
function Get-Configuration {
    param([string]$Path)
    
    if (-not (Test-Path $Path)) {
        throw "Configuration file not found: $Path"
    }
    
    # Import PowerShell data file (.psd1) - returns a hashtable natively
    $config = Import-PowerShellDataFile -Path $Path
    
    # Set defaults for any missing values
    $defaults = @{
        pings_per_cycle        = 4
        cycle_interval_seconds = 60
        timeout_ms             = 1000
        parallel_threads       = 10
        output_mode            = "file"
        log_path               = Join-Path $ScriptDir "logs\ping_results.log"
        log_rotation_size_mb   = 50
        emit_individual_pings  = $true
        hec                    = @{
            enabled      = $false
            url          = ""
            token        = ""
            index        = "main"
            sourcetype   = "ping_monitor"
            verify_ssl   = $true
            ssl_protocol = "Default"
        }
        metrics                = @{
            enabled      = $false
            mode         = "dual"
            index        = ""
            hec_url      = ""
            token        = ""
            verify_ssl   = $true
            ssl_protocol = "Default"
        }
    }
    
    # Apply defaults for missing top-level keys
    foreach ($key in $defaults.Keys) {
        if (-not $config.ContainsKey($key)) {
            $config[$key] = $defaults[$key]
        }
    }
    
    # Ensure HEC sub-properties have defaults
    if ($config.ContainsKey('hec') -and $null -ne $config.hec) {
        foreach ($hecKey in $defaults.hec.Keys) {
            if (-not $config.hec.ContainsKey($hecKey)) {
                $config.hec[$hecKey] = $defaults.hec[$hecKey]
            }
        }
    }
    else {
        $config['hec'] = $defaults.hec
    }
    
    # Ensure metrics sub-properties have defaults
    if ($config.ContainsKey('metrics') -and $null -ne $config.metrics) {
        foreach ($metricsKey in $defaults.metrics.Keys) {
            if (-not $config.metrics.ContainsKey($metricsKey)) {
                $config.metrics[$metricsKey] = $defaults.metrics[$metricsKey]
            }
        }
    }
    else {
        $config['metrics'] = $defaults.metrics
    }
    
    # Validate and sanitize numeric values
    if ($config.pings_per_cycle -lt 1) {
        Write-Warning "Invalid pings_per_cycle ($($config.pings_per_cycle)). Using default: 4"
        $config.pings_per_cycle = 4
    }
    if ($config.cycle_interval_seconds -lt 1) {
        Write-Warning "Invalid cycle_interval_seconds ($($config.cycle_interval_seconds)). Using default: 60"
        $config.cycle_interval_seconds = 60
    }
    if ($config.timeout_ms -lt 100) {
        Write-Warning "Invalid timeout_ms ($($config.timeout_ms)). Using default: 1000"
        $config.timeout_ms = 1000
    }
    if ($config.parallel_threads -lt 1) {
        Write-Warning "Invalid parallel_threads ($($config.parallel_threads)). Using default: 10"
        $config.parallel_threads = 10
    }
    if ($config.log_rotation_size_mb -lt 1) {
        Write-Warning "Invalid log_rotation_size_mb ($($config.log_rotation_size_mb)). Using default: 50"
        $config.log_rotation_size_mb = 50
    }
    
    # Validate output_mode
    $validModes = @('file', 'hec', 'both')
    if ($config.output_mode -notin $validModes) {
        Write-Warning "Invalid output_mode '$($config.output_mode)'. Using default: file"
        $config.output_mode = 'file'
    }
    
    return $config
}
#endregion

#region Endpoint Loading
function Get-Endpoints {
    param([string]$Path)
    
    if (-not (Test-Path $Path)) {
        throw "Endpoints file not found: $Path"
    }
    
    $csvData = Import-Csv -Path $Path
    
    # V3 OPTIMIZATION: Use List[T] instead of array +=
    $endpoints = [System.Collections.Generic.List[PSCustomObject]]::new()
    
    foreach ($row in $csvData) {
        # Check for required fields (ip and hostname)
        if (-not $row.ip -or -not $row.hostname) {
            Write-Warning "Skipping row with missing ip or hostname: $($row | ConvertTo-Json -Compress)"
            continue
        }
        
        $endpoint = [PSCustomObject]@{
            ip               = $row.ip.Trim()
            hostname         = $row.hostname.Trim()
            group            = if ($row.PSObject.Properties['group'] -and $row.group) { $row.group.Trim() } else { "default" }
            description      = if ($row.PSObject.Properties['description'] -and $row.description) { $row.description.Trim() } else { "" }
            entitytype       = if ($row.PSObject.Properties['entitytype'] -and $row.entitytype) { $row.entitytype.Trim() } else { "" }
            device           = if ($row.PSObject.Properties['device'] -and $row.device) { $row.device.Trim() } else { "" }
            vendor           = if ($row.PSObject.Properties['vendor'] -and $row.vendor) { $row.vendor.Trim() } else { "" }
            additional_notes = if ($row.PSObject.Properties['additional_notes'] -and $row.additional_notes) { $row.additional_notes.Trim() } else { "" }
        }
        
        $endpoints.Add($endpoint)
    }
    
    if ($endpoints.Count -eq 0) {
        throw "No valid endpoints found in CSV file"
    }
    
    Write-Host "Loaded $($endpoints.Count) endpoints from CSV" -ForegroundColor Green
    return $endpoints
}
#endregion

#region Output Sink Functions - v3.1 Streaming Sinks

<#
    V3.1 STREAMING SINKS
    ====================
    Simple sink abstractions for streaming output as results arrive.
    These maintain compatibility with v2/v3 payload formats.
#>

function Initialize-FileSink {
    <#
    .SYNOPSIS
        Initialize file sink - ensures directory exists, returns path.
    #>
    param([string]$LogPath)
    
    $logDir = Split-Path -Parent $LogPath
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    return $LogPath
}

function Write-ToFileSink {
    <#
    .SYNOPSIS
        Write a batch of results to file sink using StreamWriter.
    .DESCRIPTION
        V3.1: Called per-endpoint completion for immediate streaming.
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Results,
        [string]$LogPath
    )
    
    if ($Results.Count -eq 0) { return }
    
    $streamWriter = $null
    try {
        $streamWriter = [System.IO.StreamWriter]::new($LogPath, $true, [System.Text.Encoding]::UTF8)
        foreach ($result in $Results) {
            $jsonLine = $result | ConvertTo-Json -Compress
            $streamWriter.WriteLine($jsonLine)
        }
    }
    finally {
        if ($null -ne $streamWriter) { $streamWriter.Dispose() }
    }
}

function Send-ToHECSink {
    <#
    .SYNOPSIS
        Send a batch of events to Splunk HEC.
    .DESCRIPTION
        V3.1: Called per-endpoint completion for immediate streaming.
        Uses StringBuilder for efficient payload construction.
        Schema identical to v2/v3.
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Results,
        [hashtable]$HecConfig,
        [string]$Hostname
    )
    
    if ($Results.Count -eq 0) { return $true }
    
    if (-not $HecConfig.enabled -or [string]::IsNullOrEmpty($HecConfig.url) -or [string]::IsNullOrEmpty($HecConfig.token)) {
        return $false
    }
    
    $headers = @{
        "Authorization" = "Splunk $($HecConfig.token)"
        "Content-Type"  = "application/json"
    }
    
    # Build payload with StringBuilder
    $payloadBuilder = [System.Text.StringBuilder]::new()
    $isFirst = $true
    
    foreach ($result in $Results) {
        # HEC envelope identical to v2/v3
        $hecEvent = @{
            time       = [DateTimeOffset]::Parse($result.timestamp).ToUnixTimeSeconds()
            host       = $Hostname
            source     = "ping_monitor"
            sourcetype = $HecConfig.sourcetype
            index      = $HecConfig.index
            event      = $result
        }
        
        $eventJson = $hecEvent | ConvertTo-Json -Compress
        
        if (-not $isFirst) { [void]$payloadBuilder.Append("`n") }
        [void]$payloadBuilder.Append($eventJson)
        $isFirst = $false
    }
    
    $body = $payloadBuilder.ToString()
    
    try {
        $splatParams = @{
            Uri         = $HecConfig.url
            Method      = "POST"
            Headers     = $headers
            Body        = $body
            TimeoutSec  = 5
            ErrorAction = "Stop"
        }
        
        if (-not $HecConfig.verify_ssl) { $splatParams['SkipCertificateCheck'] = $true }
        if ($HecConfig.ssl_protocol -and $HecConfig.ssl_protocol -ne 'Default') {
            $splatParams['SslProtocol'] = $HecConfig.ssl_protocol
        }
        
        Invoke-RestMethod @splatParams | Out-Null
        return $true
    }
    catch {
        $errorMessage = $_.Exception.Message
        try {
            if ($_.Exception.PSObject.Properties.Name -contains 'Response' -and $_.Exception.Response) {
                $response = $_.Exception.Response
                if ($response.PSObject.Properties.Name -contains 'Content' -and $response.Content) {
                    $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                    if (-not [string]::IsNullOrWhiteSpace($responseBody)) {
                        $errorMessage = "$errorMessage | HEC response: $responseBody"
                    }
                }
            }
        } catch { }
        Write-Warning "Failed to send events to HEC: $errorMessage"
        return $false
    }
}

function Send-ToMetricsSink {
    <#
    .SYNOPSIS
        Send summary as metrics to Splunk HEC metrics endpoint.
    .DESCRIPTION
        V3.1: Called per-endpoint completion for immediate streaming.
        Schema identical to v2/v3.
    #>
    param(
        [PSCustomObject]$Summary,
        [hashtable]$MetricsConfig,
        [string]$Hostname
    )
    
    if (-not $MetricsConfig.enabled) { return $true }
    
    if ([string]::IsNullOrWhiteSpace($MetricsConfig.hec_url) -or [string]::IsNullOrWhiteSpace($MetricsConfig.token)) {
        return $false
    }
    
    $headers = @{
        "Authorization" = "Splunk $($MetricsConfig.token)"
        "Content-Type"  = "application/json"
    }
    
    # Metrics event schema identical to v2/v3
    $metricsEvent = @{
        time   = [DateTimeOffset]::Parse($Summary.timestamp).ToUnixTimeSeconds()
        host   = $Hostname
        source = "ping_monitor"
        index  = $MetricsConfig.index
        event  = "metric"
        fields = @{
            "metric_name:ping.avg_latency_ms"   = [double]($Summary.avg_latency_ms)
            "metric_name:ping.min_latency_ms"   = [double]($Summary.min_latency_ms)
            "metric_name:ping.max_latency_ms"   = [double]($Summary.max_latency_ms)
            "metric_name:ping.packet_loss_pct"  = [double]($Summary.packet_loss_pct)
            "metric_name:ping.pings_sent"       = [int]($Summary.pings_sent)
            "metric_name:ping.pings_successful" = [int]($Summary.pings_successful)
            hostname         = $Summary.hostname
            target_ip        = $Summary.target_ip
            group            = $Summary.group
            description      = $Summary.description
            entitytype       = $Summary.entitytype
            device           = $Summary.device
            vendor           = $Summary.vendor
            additional_notes = $Summary.additional_notes
        }
    }
    
    $body = $metricsEvent | ConvertTo-Json -Depth 5 -Compress
    
    try {
        $splatParams = @{
            Uri         = $MetricsConfig.hec_url
            Method      = "POST"
            Headers     = $headers
            Body        = $body
            TimeoutSec  = 5
            ErrorAction = "Stop"
        }
        
        if (-not $MetricsConfig.verify_ssl) { $splatParams['SkipCertificateCheck'] = $true }
        if ($MetricsConfig.ssl_protocol -and $MetricsConfig.ssl_protocol -ne 'Default') {
            $splatParams['SslProtocol'] = $MetricsConfig.ssl_protocol
        }
        
        Invoke-RestMethod @splatParams | Out-Null
        return $true
    }
    catch {
        $errorMessage = $_.Exception.Message
        try {
            if ($_.Exception.PSObject.Properties.Name -contains 'Response' -and $_.Exception.Response) {
                $response = $_.Exception.Response
                if ($response.PSObject.Properties.Name -contains 'Content' -and $response.Content) {
                    $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                    if (-not [string]::IsNullOrWhiteSpace($responseBody)) {
                        $errorMessage = "$errorMessage | HEC response: $responseBody"
                    }
                }
            }
        } catch { }
        Write-Warning "Failed to send metrics to HEC: $errorMessage"
        return $false
    }
}

#endregion

#region Log Rotation
function Invoke-LogRotation {
    param(
        [string]$LogPath,
        [int]$MaxSizeMB
    )
    
    if (-not (Test-Path $LogPath)) { return }
    
    $logFile = Get-Item $LogPath
    $sizeMB = $logFile.Length / 1MB
    
    if ($sizeMB -ge $MaxSizeMB) {
        $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
        $archivePath = $LogPath -replace '\.log$', "_$timestamp.log"
        
        Write-Host "Rotating log file (Size: $([math]::Round($sizeMB, 2)) MB)" -ForegroundColor Yellow
        Move-Item -Path $LogPath -Destination $archivePath -Force
        
        $logDir = Split-Path -Parent $LogPath
        $logBaseName = [System.IO.Path]::GetFileNameWithoutExtension($LogPath)
        $oldLogs = Get-ChildItem -Path $logDir -Filter "$logBaseName`_*.log" | Sort-Object LastWriteTime -Descending | Select-Object -Skip 5
        
        foreach ($oldLog in $oldLogs) {
            Remove-Item $oldLog.FullName -Force
            Write-Host "Removed old log: $($oldLog.Name)" -ForegroundColor Gray
        }
    }
}
#endregion

#region True Streaming Parallel Ping Execution

<#
    V3.1 TRUE STREAMING EXECUTION
    =============================
    - Runspaces processed as they complete (Handle.IsCompleted polling)
    - Results emitted immediately per-endpoint
    - No cycle-wide accumulation
    - Individual ping objects only created when emit_individual_pings=true
    - Single Ping object reused per worker
    - TTL null safety
#>

function Invoke-StreamingParallelPing {
    <#
    .SYNOPSIS
        Execute pings with true streaming - process and emit as each endpoint completes.
    .DESCRIPTION
        V3.1: True streaming implementation:
        - Polls for completed runspaces
        - Immediately processes and emits results
        - Disposes runspace right after processing
        - No accumulation of all results
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Endpoints,
        [int]$PingsPerCycle,
        [int]$TimeoutMs,
        [int]$ParallelThreads,
        [bool]$EmitIndividualPings,
        [bool]$EmitEventSummaries,
        [bool]$EmitMetrics,
        [string]$OutputMode,
        [string]$LogPath,
        [hashtable]$HecConfig,
        [hashtable]$MetricsConfig
    )
    
    $hostname = $env:COMPUTERNAME
    
    # Counters for cycle summary display
    $totalSuccess = 0
    $totalPartial = 0
    $totalFailed = 0
    $hecSuccessCount = 0
    $hecFailCount = 0
    $metricsSuccessCount = 0
    $metricsFailCount = 0
    
    # Create runspace pool
    $runspacePool = [runspacefactory]::CreateRunspacePool(1, $ParallelThreads)
    $runspacePool.Open()
    
    # Track active runspaces
    $activeRunspaces = [System.Collections.Generic.List[hashtable]]::new()
    
    # V3.1 WORKER SCRIPTBLOCK
    # - Reuses single Ping object for all iterations
    # - Only creates individual ping objects when $EmitIndividual is true
    # - TTL null safety
    $pingScriptBlock = {
        param($Endpoint, $Count, $Timeout, $EmitIndividual)
        
        # V3.1: Only create individual results list if needed
        $individualResults = if ($EmitIndividual) { 
            [System.Collections.Generic.List[PSCustomObject]]::new() 
        } else { 
            $null 
        }
        
        $successCount = 0
        $totalLatency = 0
        $minLatency = [int]::MaxValue
        $maxLatency = 0
        
        # V3.1: Single Ping object reused for all iterations
        $ping = $null
        try {
            $ping = [System.Net.NetworkInformation.Ping]::new()
            
            for ($i = 0; $i -lt $Count; $i++) {
                $timestamp = Get-Date -Format "o"
                
                try {
                    $reply = $ping.Send($Endpoint.ip, $Timeout)
                    
                    if ($reply.Status -eq [System.Net.NetworkInformation.IPStatus]::Success) {
                        $latency = [int]$reply.RoundtripTime
                        # V3.1: TTL null safety - Options can be null on some platforms/responses
                        $ttl = if ($null -ne $reply.Options) { $reply.Options.Ttl } else { -1 }
                        
                        $successCount++
                        $totalLatency += $latency
                        $minLatency = [math]::Min($minLatency, $latency)
                        $maxLatency = [math]::Max($maxLatency, $latency)
                        
                        # V3.1: Only build individual ping object if needed
                        if ($EmitIndividual) {
                            $individualResults.Add([PSCustomObject]@{
                                timestamp        = $timestamp
                                target_ip        = $Endpoint.ip
                                hostname         = $Endpoint.hostname
                                group            = $Endpoint.group
                                description      = $Endpoint.description
                                entitytype       = $Endpoint.entitytype
                                device           = $Endpoint.device
                                vendor           = $Endpoint.vendor
                                additional_notes = $Endpoint.additional_notes
                                status           = "success"
                                latency_ms       = $latency
                                ttl              = $ttl
                                ping_number      = ($i + 1)
                                pings_in_cycle   = $Count
                                record_type      = "ping"
                            })
                        }
                    }
                    else {
                        if ($EmitIndividual) {
                            $individualResults.Add([PSCustomObject]@{
                                timestamp        = $timestamp
                                target_ip        = $Endpoint.ip
                                hostname         = $Endpoint.hostname
                                group            = $Endpoint.group
                                description      = $Endpoint.description
                                entitytype       = $Endpoint.entitytype
                                device           = $Endpoint.device
                                vendor           = $Endpoint.vendor
                                additional_notes = $Endpoint.additional_notes
                                status           = "failed"
                                latency_ms       = -1
                                ttl              = -1
                                ping_number      = ($i + 1)
                                pings_in_cycle   = $Count
                                error_message    = "Ping failed with status: $($reply.Status)"
                                record_type      = "ping"
                            })
                        }
                    }
                }
                catch {
                    if ($EmitIndividual) {
                        $individualResults.Add([PSCustomObject]@{
                            timestamp        = $timestamp
                            target_ip        = $Endpoint.ip
                            hostname         = $Endpoint.hostname
                            group            = $Endpoint.group
                            description      = $Endpoint.description
                            entitytype       = $Endpoint.entitytype
                            device           = $Endpoint.device
                            vendor           = $Endpoint.vendor
                            additional_notes = $Endpoint.additional_notes
                            status           = "failed"
                            latency_ms       = -1
                            ttl              = -1
                            ping_number      = ($i + 1)
                            pings_in_cycle   = $Count
                            error_message    = $_.Exception.Message
                            record_type      = "ping"
                        })
                    }
                }
            }
        }
        finally {
            # V3.1: Dispose single Ping object at end
            if ($null -ne $ping) { $ping.Dispose() }
        }
        
        # Calculate summary statistics
        $packetLossPct = [math]::Round((($Count - $successCount) / $Count) * 100, 2)
        $avgLatency = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
        
        # Summary record (schema identical to v2/v3)
        $summary = [PSCustomObject]@{
            timestamp        = (Get-Date -Format "o")
            target_ip        = $Endpoint.ip
            hostname         = $Endpoint.hostname
            group            = $Endpoint.group
            description      = $Endpoint.description
            entitytype       = $Endpoint.entitytype
            device           = $Endpoint.device
            vendor           = $Endpoint.vendor
            additional_notes = $Endpoint.additional_notes
            record_type      = "summary"
            pings_sent       = $Count
            pings_successful = $successCount
            pings_failed     = ($Count - $successCount)
            packet_loss_pct  = $packetLossPct
            avg_latency_ms   = $avgLatency
            min_latency_ms   = if ($successCount -gt 0) { $minLatency } else { -1 }
            max_latency_ms   = if ($successCount -gt 0) { $maxLatency } else { -1 }
        }
        
        return @{
            Individual = $individualResults
            Summary    = $summary
        }
    }
    
    # Start all runspaces
    foreach ($endpoint in $Endpoints) {
        $powershell = [powershell]::Create()
        $powershell.RunspacePool = $runspacePool
        [void]$powershell.AddScript($pingScriptBlock)
        [void]$powershell.AddArgument($endpoint)
        [void]$powershell.AddArgument($PingsPerCycle)
        [void]$powershell.AddArgument($TimeoutMs)
        [void]$powershell.AddArgument($EmitIndividualPings)
        
        $activeRunspaces.Add(@{
            PowerShell = $powershell
            Handle     = $powershell.BeginInvoke()
            Endpoint   = $endpoint
        })
    }
    
    # V3.1 TRUE STREAMING: Process runspaces as they complete
    while ($activeRunspaces.Count -gt 0) {
        # Find completed runspaces
        $completedIndices = [System.Collections.Generic.List[int]]::new()
        
        for ($i = 0; $i -lt $activeRunspaces.Count; $i++) {
            if ($activeRunspaces[$i].Handle.IsCompleted) {
                $completedIndices.Add($i)
            }
        }
        
        # Process completed runspaces (reverse order to safely remove)
        for ($j = $completedIndices.Count - 1; $j -ge 0; $j--) {
            $idx = $completedIndices[$j]
            $runspace = $activeRunspaces[$idx]
            
            try {
                $rawResult = $runspace.PowerShell.EndInvoke($runspace.Handle)
                
                # EndInvoke returns PSDataCollection - extract the actual hashtable
                $result = if ($rawResult.Count -gt 0) { $rawResult[0] } else { $null }
                
                if ($null -ne $result) {
                    $summary = $result.Summary
                    $individual = $result.Individual
                    
                    # Update cycle counters
                    if ($summary.packet_loss_pct -eq 0) { $totalSuccess++ }
                    elseif ($summary.packet_loss_pct -lt 100) { $totalPartial++ }
                    else { $totalFailed++ }
                    
                    # V3.1: IMMEDIATE STREAMING - emit results right now
                    
                    # Build events list for this endpoint
                    $eventsToEmit = [System.Collections.Generic.List[PSCustomObject]]::new()
                    
                    if ($EmitIndividualPings -and $null -ne $individual) {
                        foreach ($item in $individual) {
                            $eventsToEmit.Add($item)
                        }
                    }
                    
                    if ($EmitEventSummaries) {
                        $eventsToEmit.Add($summary)
                    }
                    
                    # Emit to file sink
                    if ($eventsToEmit.Count -gt 0 -and ($OutputMode -eq 'file' -or $OutputMode -eq 'both')) {
                        Write-ToFileSink -Results $eventsToEmit -LogPath $LogPath
                    }
                    
                    # Emit to HEC sink
                    if ($eventsToEmit.Count -gt 0 -and ($OutputMode -eq 'hec' -or $OutputMode -eq 'both')) {
                        if (Send-ToHECSink -Results $eventsToEmit -HecConfig $HecConfig -Hostname $hostname) {
                            $hecSuccessCount++
                        } else {
                            $hecFailCount++
                        }
                    }
                    
                    # Emit metrics if enabled
                    if ($EmitMetrics) {
                        if (Send-ToMetricsSink -Summary $summary -MetricsConfig $MetricsConfig -Hostname $hostname) {
                            $metricsSuccessCount++
                        } else {
                            $metricsFailCount++
                        }
                    }
                }
            }
            catch {
                Write-Warning "Runspace failed for $($runspace.Endpoint.hostname): $($_.Exception.Message)"
                $totalFailed++
            }
            finally {
                # V3.1: Immediate disposal after processing
                $runspace.PowerShell.Dispose()
            }
            
            # Remove from active list
            $activeRunspaces.RemoveAt($idx)
        }
        
        # Small sleep to avoid busy-wait if nothing completed
        if ($completedIndices.Count -eq 0 -and $activeRunspaces.Count -gt 0) {
            Start-Sleep -Milliseconds 10
        }
    }
    
    # Clean up runspace pool
    $runspacePool.Close()
    $runspacePool.Dispose()
    
    # Return cycle statistics for display
    return @{
        TotalSuccess        = $totalSuccess
        TotalPartial        = $totalPartial
        TotalFailed         = $totalFailed
        HecSuccessCount     = $hecSuccessCount
        HecFailCount        = $hecFailCount
        MetricsSuccessCount = $metricsSuccessCount
        MetricsFailCount    = $metricsFailCount
    }
}

#endregion

#region Main Execution
function Start-PingMonitor {
    param(
        [hashtable]$Config,
        [System.Collections.Generic.List[PSCustomObject]]$Endpoints,
        [switch]$RunOnce
    )
    
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  Splunk Ping Monitor v3.1 Started" -ForegroundColor Cyan
    Write-Host "  (True streaming, lowest RAM)" -ForegroundColor DarkCyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "Endpoints: $($Endpoints.Count)" -ForegroundColor White
    Write-Host "Pings per cycle: $($Config.pings_per_cycle)" -ForegroundColor White
    Write-Host "Cycle interval: $($Config.cycle_interval_seconds) seconds" -ForegroundColor White
    Write-Host "Output mode: $($Config.output_mode)" -ForegroundColor White
    Write-Host "Individual pings: $(if ($Config.emit_individual_pings) { 'enabled' } else { 'disabled (summary only)' })" -ForegroundColor White
    Write-Host "Metrics: $(if ($Config.metrics.enabled) { $Config.metrics.mode } else { 'disabled' })" -ForegroundColor White
    Write-Host "----------------------------------------" -ForegroundColor Gray
    
    # Determine emission modes
    $emitIndividualPings = $Config.emit_individual_pings
    $emitEventSummaries = $true
    $emitMetrics = $Config.metrics.enabled
    
    # metrics_only mode suppresses event summaries
    if ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
        $emitEventSummaries = $false
    }
    
    # Initialize file sink if needed
    $logPath = $Config.log_path
    if ($Config.output_mode -eq 'file' -or $Config.output_mode -eq 'both') {
        $logPath = Initialize-FileSink -LogPath $Config.log_path
    }
    
    $cycleCount = 0
    
    do {
        $cycleCount++
        $cycleStart = Get-Date
        
        Write-Host "`n[$cycleStart] Starting cycle #$cycleCount..." -ForegroundColor Cyan
        
        # Check log rotation before cycle
        if ($Config.output_mode -eq 'file' -or $Config.output_mode -eq 'both') {
            Invoke-LogRotation -LogPath $logPath -MaxSizeMB $Config.log_rotation_size_mb
        }
        
        # V3.1: True streaming parallel ping with immediate emission
        $cycleStats = Invoke-StreamingParallelPing `
            -Endpoints $Endpoints `
            -PingsPerCycle $Config.pings_per_cycle `
            -TimeoutMs $Config.timeout_ms `
            -ParallelThreads $Config.parallel_threads `
            -EmitIndividualPings $emitIndividualPings `
            -EmitEventSummaries $emitEventSummaries `
            -EmitMetrics $emitMetrics `
            -OutputMode $Config.output_mode `
            -LogPath $logPath `
            -HecConfig $Config.hec `
            -MetricsConfig $Config.metrics
        
        # Display cycle summary
        if ($Config.output_mode -eq 'file' -or $Config.output_mode -eq 'both') {
            Write-Host "Results streamed to: $logPath" -ForegroundColor Green
        }
        
        if (($Config.output_mode -eq 'hec' -or $Config.output_mode -eq 'both') -and $Config.hec.enabled) {
            Write-Host "HEC: $($cycleStats.HecSuccessCount) endpoints sent, $($cycleStats.HecFailCount) failed" -ForegroundColor $(if ($cycleStats.HecFailCount -eq 0) { "Green" } else { "Yellow" })
        }
        
        if ($emitMetrics) {
            Write-Host "Metrics: $($cycleStats.MetricsSuccessCount) sent, $($cycleStats.MetricsFailCount) failed" -ForegroundColor $(if ($cycleStats.MetricsFailCount -eq 0) { "Green" } else { "Yellow" })
        }
        elseif ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
            Write-Host "Metrics-only mode: Events skipped" -ForegroundColor Gray
        }
        
        Write-Host "Cycle #$cycleCount complete - Success: $($cycleStats.TotalSuccess) | Partial: $($cycleStats.TotalPartial) | Failed: $($cycleStats.TotalFailed)" -ForegroundColor $(
            if ($cycleStats.TotalFailed -eq 0) { "Green" }
            elseif ($cycleStats.TotalFailed -lt $Endpoints.Count) { "Yellow" }
            else { "Red" }
        )
        
        # Preserve timing behavior from v2/v3
        if (-not $RunOnce) {
            $cycleEnd = Get-Date
            $cycleDuration = ($cycleEnd - $cycleStart).TotalSeconds
            $sleepTime = [math]::Max(0, $Config.cycle_interval_seconds - $cycleDuration)
            
            if ($sleepTime -gt 0) {
                Write-Host "Sleeping for $([math]::Round($sleepTime, 1)) seconds..." -ForegroundColor Gray
                Start-Sleep -Seconds $sleepTime
            }
        }
        
    } while (-not $RunOnce)
    
    Write-Host "`nPing Monitor v3.1 completed." -ForegroundColor Cyan
}
#endregion

#region Script Entry Point
try {
    # Set default paths if not provided
    if ([string]::IsNullOrEmpty($ConfigPath)) {
        $ConfigPath = Join-Path $ScriptDir "config.psd1"
    }
    if ([string]::IsNullOrEmpty($EndpointsPath)) {
        $EndpointsPath = Join-Path $ScriptDir "endpoints.csv"
    }
    
    Write-Host "Loading configuration from: $ConfigPath" -ForegroundColor Gray
    $config = Get-Configuration -Path $ConfigPath
    
    Write-Host "Loading endpoints from: $EndpointsPath" -ForegroundColor Gray
    $endpoints = Get-Endpoints -Path $EndpointsPath
    
    # Start the monitor
    Start-PingMonitor -Config $config -Endpoints $endpoints -RunOnce:$RunOnce
}
catch {
    Write-Error "Fatal error: $($_.Exception.Message)"
    Write-Error $_.ScriptStackTrace
    exit 1
}
#endregion
