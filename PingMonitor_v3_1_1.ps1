#Requires -Version 7.4
<#
.SYNOPSIS
    Splunk Ping Monitor v3.1.1 - Fixes for metrics_only mode, restored HEC batching, improved file streaming.

.DESCRIPTION
    This script pings endpoints defined in a CSV file and outputs results either to a log file
    (for Splunk Universal Forwarder ingestion) or directly to Splunk via HTTP Event Collector (HEC).

    VERSION 3.1.1 FIXES:
    ====================
    1) BUG FIX - metrics_only mode now correctly suppresses ALL events:
       - When metrics.enabled=true AND metrics.mode="metrics_only":
         - Forcibly sets emit_individual_pings=false (overrides config)
         - Forcibly sets emit_event_summaries=false
       - Only metrics are sent; no ping or summary events emitted

    2) REGRESSION FIX - Restored proper HEC batching:
       - v3.1 sent one HEC POST per endpoint (high overhead)
       - v3.1.1 uses cycle-level StringBuilder buffer
       - Flushes at batchSize=100 events OR at end of cycle
       - Reduces HTTP overhead significantly for large endpoint counts

    3) IMPROVEMENT - Single file handle per cycle:
       - Opens StreamWriter once at cycle start (after rotation check)
       - Writes lines as results arrive (still streaming)
       - Closes at end of cycle
       - Eliminates per-endpoint file open/close churn

    BACKWARD COMPATIBILITY:
    =======================
    - All existing config.psd1 files work unchanged
    - All existing endpoints.csv files work unchanged
    - Event JSON structure identical to v2/v3/v3.1
    - Metrics payload structure identical to v2/v3/v3.1
    - HEC sourcetype, source, index behavior unchanged
    - record_type=ping and record_type=summary fields preserved

.PARAMETER ConfigPath
    Path to the PowerShell data configuration file (.psd1). Defaults to config.psd1 in the script directory.

.PARAMETER EndpointsPath
    Path to the CSV file containing endpoints. Defaults to endpoints.csv in the script directory.

.PARAMETER RunOnce
    If specified, runs a single ping cycle and exits. Otherwise runs continuously.

.EXAMPLE
    .\PingMonitor_v3_1_1.ps1
    Runs continuously using default config.psd1 and endpoints.csv

.EXAMPLE
    .\PingMonitor_v3_1_1.ps1 -ConfigPath "C:\Config\myconfig.psd1" -RunOnce
    Runs a single cycle with custom config path

.NOTES
    Author: Splunk Ping Monitor
    Version: 3.1.1
    Requires: PowerShell 7.4+ (no external modules - airgap friendly)
    
    Memory Characteristics:
    - RAM usage stabilizes immediately (no accumulation)
    - Results streamed as endpoints complete
    - Individual ping objects only created when needed
    - Single Ping instance reused per worker
    - HEC batching reduces memory churn vs per-event POSTs
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
    
    $config = Import-PowerShellDataFile -Path $Path
    
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
    
    foreach ($key in $defaults.Keys) {
        if (-not $config.ContainsKey($key)) {
            $config[$key] = $defaults[$key]
        }
    }
    
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
    $endpoints = [System.Collections.Generic.List[PSCustomObject]]::new()
    
    foreach ($row in $csvData) {
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

#region Output Coordinator - v3.1.1 Improved Sinks

<#
    V3.1.1 OUTPUT COORDINATOR
    =========================
    Provides cycle-level resource management for file and HEC output:
    - File: Single StreamWriter per cycle (not per endpoint)
    - HEC: Cycle-level StringBuilder buffer with batch flushing
    - Metrics: Per-summary immediate send (unchanged)
#>

# --- File Sink Functions ---

function Open-FileWriter {
    <#
    .SYNOPSIS
        Open a StreamWriter for the cycle. Call once at cycle start.
    #>
    param([string]$LogPath)
    
    $logDir = Split-Path -Parent $LogPath
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    
    return [System.IO.StreamWriter]::new($LogPath, $true, [System.Text.Encoding]::UTF8)
}

function Write-JsonLinesToWriter {
    <#
    .SYNOPSIS
        Write a list of PSCustomObjects as JSON lines to an open StreamWriter.
    #>
    param(
        [System.IO.StreamWriter]$Writer,
        [System.Collections.Generic.List[PSCustomObject]]$Results
    )
    
    foreach ($result in $Results) {
        $jsonLine = $result | ConvertTo-Json -Compress
        $Writer.WriteLine($jsonLine)
    }
}

function Close-FileWriter {
    <#
    .SYNOPSIS
        Flush and close the StreamWriter.
    #>
    param([System.IO.StreamWriter]$Writer)
    
    if ($null -ne $Writer) {
        $Writer.Flush()
        $Writer.Dispose()
    }
}

# --- HEC Buffer Functions ---

function Initialize-HecBuffer {
    <#
    .SYNOPSIS
        Initialize a cycle-level HEC buffer for batching events.
    #>
    param([int]$BatchSize = 100)
    
    return @{
        Builder   = [System.Text.StringBuilder]::new()
        Count     = 0
        BatchSize = $BatchSize
    }
}

function Add-EventsToHecBuffer {
    <#
    .SYNOPSIS
        Add events to the HEC buffer. Returns number of successful flushes and failures.
    .DESCRIPTION
        Appends newline-delimited HEC envelope JSON for each event.
        Flushes when buffer reaches batch size.
        Returns hashtable with SuccessCount and FailCount from any flushes.
    #>
    param(
        [hashtable]$Buffer,
        [System.Collections.Generic.List[PSCustomObject]]$Events,
        [hashtable]$HecConfig,
        [string]$Hostname
    )
    
    $successCount = 0
    $failCount = 0
    
    foreach ($event in $Events) {
        # Build HEC envelope (schema identical to v2/v3/v3.1)
        $hecEvent = @{
            time       = [DateTimeOffset]::Parse($event.timestamp).ToUnixTimeSeconds()
            host       = $Hostname
            source     = "ping_monitor"
            sourcetype = $HecConfig.sourcetype
            index      = $HecConfig.index
            event      = $event
        }
        
        $eventJson = $hecEvent | ConvertTo-Json -Compress
        
        # Add newline separator if not first event in buffer
        if ($Buffer.Count -gt 0) {
            [void]$Buffer.Builder.Append("`n")
        }
        [void]$Buffer.Builder.Append($eventJson)
        $Buffer.Count++
        
        # Flush when batch size reached
        if ($Buffer.Count -ge $Buffer.BatchSize) {
            $flushResult = Flush-HecBuffer -Buffer $Buffer -HecConfig $HecConfig
            if ($flushResult) { $successCount++ } else { $failCount++ }
        }
    }
    
    return @{ SuccessCount = $successCount; FailCount = $failCount }
}

function Flush-HecBuffer {
    <#
    .SYNOPSIS
        Flush the HEC buffer by POSTing its contents. Clears buffer after.
    .RETURNS
        $true on success, $false on failure.
    #>
    param(
        [hashtable]$Buffer,
        [hashtable]$HecConfig
    )
    
    if ($Buffer.Count -eq 0) {
        return $true
    }
    
    $body = $Buffer.Builder.ToString()
    
    $headers = @{
        "Authorization" = "Splunk $($HecConfig.token)"
        "Content-Type"  = "application/json"
    }
    
    try {
        $splatParams = @{
            Uri         = $HecConfig.url
            Method      = "POST"
            Headers     = $headers
            Body        = $body
            TimeoutSec  = 10
            ErrorAction = "Stop"
        }
        
        if (-not $HecConfig.verify_ssl) { $splatParams['SkipCertificateCheck'] = $true }
        if ($HecConfig.ssl_protocol -and $HecConfig.ssl_protocol -ne 'Default') {
            $splatParams['SslProtocol'] = $HecConfig.ssl_protocol
        }
        
        Invoke-RestMethod @splatParams | Out-Null
        
        # Clear buffer for reuse
        [void]$Buffer.Builder.Clear()
        $Buffer.Count = 0
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
        Write-Warning "Failed to send HEC batch: $errorMessage"
        
        # Clear buffer even on failure to prevent re-sending same data
        [void]$Buffer.Builder.Clear()
        $Buffer.Count = 0
        return $false
    }
}

# --- Metrics Sink (unchanged from v3.1) ---

function Send-ToMetricsSink {
    <#
    .SYNOPSIS
        Send a single summary as metrics to Splunk HEC metrics endpoint.
    .DESCRIPTION
        Schema identical to v2/v3/v3.1 for backward compatibility.
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
    
    # Metrics event schema identical to v2/v3/v3.1
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

function Invoke-StreamingParallelPing {
    <#
    .SYNOPSIS
        Execute pings with true streaming - process and emit as each endpoint completes.
    .DESCRIPTION
        V3.1.1: True streaming with improved output coordination:
        - Polls for completed runspaces (Handle.IsCompleted)
        - Writes to file via passed StreamWriter (single handle per cycle)
        - Batches HEC events via passed buffer (flushes at threshold)
        - Sends metrics immediately per summary
        - Disposes runspace right after processing
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
        [System.IO.StreamWriter]$FileWriter,
        [hashtable]$HecBuffer,
        [hashtable]$HecConfig,
        [hashtable]$MetricsConfig
    )
    
    $hostname = $env:COMPUTERNAME
    
    # Counters for cycle summary display
    $totalSuccess = 0
    $totalPartial = 0
    $totalFailed = 0
    $hecBatchSuccessCount = 0
    $hecBatchFailCount = 0
    $hecEventCount = 0
    $metricsSuccessCount = 0
    $metricsFailCount = 0
    
    # Create runspace pool
    $runspacePool = [runspacefactory]::CreateRunspacePool(1, $ParallelThreads)
    $runspacePool.Open()
    
    # Track active runspaces
    $activeRunspaces = [System.Collections.Generic.List[hashtable]]::new()
    
    # Worker scriptblock - gated ping object creation, Ping reuse, TTL null safety
    $pingScriptBlock = {
        param($Endpoint, $Count, $Timeout, $EmitIndividual)
        
        # Only create individual results list if needed (gated creation)
        $individualResults = if ($EmitIndividual) { 
            [System.Collections.Generic.List[PSCustomObject]]::new() 
        } else { 
            $null 
        }
        
        $successCount = 0
        $totalLatency = 0
        $minLatency = [int]::MaxValue
        $maxLatency = 0
        
        # Single Ping object reused for all iterations
        $ping = $null
        try {
            $ping = [System.Net.NetworkInformation.Ping]::new()
            
            for ($i = 0; $i -lt $Count; $i++) {
                $timestamp = Get-Date -Format "o"
                
                try {
                    $reply = $ping.Send($Endpoint.ip, $Timeout)
                    
                    if ($reply.Status -eq [System.Net.NetworkInformation.IPStatus]::Success) {
                        $latency = [int]$reply.RoundtripTime
                        # TTL null safety
                        $ttl = if ($null -ne $reply.Options) { $reply.Options.Ttl } else { -1 }
                        
                        $successCount++
                        $totalLatency += $latency
                        $minLatency = [math]::Min($minLatency, $latency)
                        $maxLatency = [math]::Max($maxLatency, $latency)
                        
                        # Only build individual ping object if needed
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
            # Dispose single Ping object at end
            if ($null -ne $ping) { $ping.Dispose() }
        }
        
        # Calculate summary statistics
        $packetLossPct = [math]::Round((($Count - $successCount) / $Count) * 100, 2)
        $avgLatency = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
        
        # Summary record (schema identical to v2/v3/v3.1)
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
    
    # TRUE STREAMING: Process runspaces as they complete
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
                $result = if ($rawResult.Count -gt 0) { $rawResult[0] } else { $null }
                
                if ($null -ne $result) {
                    $summary = $result.Summary
                    $individual = $result.Individual
                    
                    # Update cycle counters
                    if ($summary.packet_loss_pct -eq 0) { $totalSuccess++ }
                    elseif ($summary.packet_loss_pct -lt 100) { $totalPartial++ }
                    else { $totalFailed++ }
                    
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
                    
                    # Write to file sink (single StreamWriter for cycle)
                    if ($eventsToEmit.Count -gt 0 -and $null -ne $FileWriter) {
                        Write-JsonLinesToWriter -Writer $FileWriter -Results $eventsToEmit
                    }
                    
                    # Add to HEC buffer (batched, flushes at threshold)
                    if ($eventsToEmit.Count -gt 0 -and $null -ne $HecBuffer -and $HecConfig.enabled) {
                        $hecEventCount += $eventsToEmit.Count
                        $flushResult = Add-EventsToHecBuffer -Buffer $HecBuffer -Events $eventsToEmit -HecConfig $HecConfig -Hostname $hostname
                        $hecBatchSuccessCount += $flushResult.SuccessCount
                        $hecBatchFailCount += $flushResult.FailCount
                    }
                    
                    # Send metrics immediately (unchanged from v3.1)
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
                # Immediate disposal after processing
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
        TotalSuccess          = $totalSuccess
        TotalPartial          = $totalPartial
        TotalFailed           = $totalFailed
        HecBatchSuccessCount  = $hecBatchSuccessCount
        HecBatchFailCount     = $hecBatchFailCount
        HecEventCount         = $hecEventCount
        MetricsSuccessCount   = $metricsSuccessCount
        MetricsFailCount      = $metricsFailCount
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
    Write-Host "  Splunk Ping Monitor v3.1.1 Started" -ForegroundColor Cyan
    Write-Host "  (Fixed batching + metrics_only)" -ForegroundColor DarkCyan
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
    
    # V3.1.1 FIX: metrics_only mode OVERRIDES event emission settings
    # When metrics_only is active, NO events should be emitted (neither pings nor summaries)
    if ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
        $emitEventSummaries = $false
        $emitIndividualPings = $false  # FIX: Force this off regardless of config
        Write-Host "metrics_only mode: All event emission disabled (pings + summaries)" -ForegroundColor DarkYellow
    }
    
    $logPath = $Config.log_path
    $cycleCount = 0
    
    do {
        $cycleCount++
        $cycleStart = Get-Date
        
        Write-Host "`n[$cycleStart] Starting cycle #$cycleCount..." -ForegroundColor Cyan
        
        # === CYCLE-LEVEL RESOURCE SETUP ===
        
        # File writer (single handle per cycle)
        $fileWriter = $null
        $useFileOutput = ($Config.output_mode -eq 'file' -or $Config.output_mode -eq 'both') -and ($emitIndividualPings -or $emitEventSummaries)
        
        if ($useFileOutput) {
            Invoke-LogRotation -LogPath $logPath -MaxSizeMB $Config.log_rotation_size_mb
            $fileWriter = Open-FileWriter -LogPath $logPath
        }
        
        # HEC buffer (cycle-level batching)
        $hecBuffer = $null
        $useHecOutput = ($Config.output_mode -eq 'hec' -or $Config.output_mode -eq 'both') -and 
                        $Config.hec.enabled -and 
                        ($emitIndividualPings -or $emitEventSummaries)
        
        if ($useHecOutput) {
            $hecBuffer = Initialize-HecBuffer -BatchSize 100
        }
        
        # === STREAMING PARALLEL PING ===
        
        try {
            $cycleStats = Invoke-StreamingParallelPing `
                -Endpoints $Endpoints `
                -PingsPerCycle $Config.pings_per_cycle `
                -TimeoutMs $Config.timeout_ms `
                -ParallelThreads $Config.parallel_threads `
                -EmitIndividualPings $emitIndividualPings `
                -EmitEventSummaries $emitEventSummaries `
                -EmitMetrics $emitMetrics `
                -OutputMode $Config.output_mode `
                -FileWriter $fileWriter `
                -HecBuffer $hecBuffer `
                -HecConfig $Config.hec `
                -MetricsConfig $Config.metrics
        }
        finally {
            # === CYCLE-LEVEL RESOURCE CLEANUP ===
            
            # Close file writer
            if ($null -ne $fileWriter) {
                Close-FileWriter -Writer $fileWriter
            }
            
            # Flush remaining HEC events
            if ($null -ne $hecBuffer -and $hecBuffer.Count -gt 0) {
                if (Flush-HecBuffer -Buffer $hecBuffer -HecConfig $Config.hec) {
                    $cycleStats.HecBatchSuccessCount++
                } else {
                    $cycleStats.HecBatchFailCount++
                }
            }
        }
        
        # === CYCLE OUTPUT SUMMARY ===
        
        if ($useFileOutput) {
            Write-Host "File: Results streamed to $logPath" -ForegroundColor Green
        }
        
        if ($useHecOutput) {
            $totalHecBatches = $cycleStats.HecBatchSuccessCount + $cycleStats.HecBatchFailCount
            Write-Host "HEC: $($cycleStats.HecEventCount) events in $totalHecBatches batches ($($cycleStats.HecBatchSuccessCount) OK, $($cycleStats.HecBatchFailCount) failed)" -ForegroundColor $(if ($cycleStats.HecBatchFailCount -eq 0) { "Green" } else { "Yellow" })
        }
        
        if ($emitMetrics) {
            Write-Host "Metrics: $($cycleStats.MetricsSuccessCount) sent, $($cycleStats.MetricsFailCount) failed" -ForegroundColor $(if ($cycleStats.MetricsFailCount -eq 0) { "Green" } else { "Yellow" })
        }
        elseif ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
            Write-Host "Metrics-only mode: $($cycleStats.MetricsSuccessCount) metrics sent, events skipped" -ForegroundColor Gray
        }
        
        Write-Host "Cycle #$cycleCount complete - Success: $($cycleStats.TotalSuccess) | Partial: $($cycleStats.TotalPartial) | Failed: $($cycleStats.TotalFailed)" -ForegroundColor $(
            if ($cycleStats.TotalFailed -eq 0) { "Green" }
            elseif ($cycleStats.TotalFailed -lt $Endpoints.Count) { "Yellow" }
            else { "Red" }
        )
        
        # Preserve timing behavior
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
    
    Write-Host "`nPing Monitor v3.1.1 completed." -ForegroundColor Cyan
}
#endregion

#region Script Entry Point
try {
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
    
    Start-PingMonitor -Config $config -Endpoints $endpoints -RunOnce:$RunOnce
}
catch {
    Write-Error "Fatal error: $($_.Exception.Message)"
    Write-Error $_.ScriptStackTrace
    exit 1
}
#endregion
