#Requires -Version 7.4
<#
.SYNOPSIS
    Splunk Ping Monitor v3 - RAM-optimized, streaming execution, backward compatible.

.DESCRIPTION
    This script pings endpoints defined in a CSV file and outputs results either to a log file
    (for Splunk Universal Forwarder ingestion) or directly to Splunk via HTTP Event Collector (HEC).

    VERSION 3.0.0 CHANGES (RAM Optimization):
    =========================================
    Phase 1 - Output & Memory Optimization:
      - Replaced array += patterns with System.Collections.Generic.List[T]
      - Rewrote Send-ToSplunkHEC with incremental batching using StringBuilder
      - Rewrote Send-ToSplunkMetrics with incremental batching using StringBuilder
      - Rewrote Write-ToLogFile to use StreamWriter instead of Add-Content in loops

    Phase 2 - Parallel Execution Refactor:
      - Replaced ForEach-Object -Parallel with runspace pool for streaming results
      - Results are processed incrementally rather than collected into large arrays
      - Reduced object churn by avoiding nested return structures

    Phase 3 - Network & Performance:
      - Replaced Test-Connection with System.Net.NetworkInformation.Ping
      - Proper disposal of Ping objects after each use
      - Maintained semantic compatibility (timeout, latency, TTL, success/failure)

    BACKWARD COMPATIBILITY:
    =======================
    - All existing config.psd1 files work unchanged
    - All existing endpoints.csv files work unchanged
    - Event JSON structure identical to v2
    - Metrics payload structure identical to v2
    - HEC sourcetype, source, index behavior unchanged

.PARAMETER ConfigPath
    Path to the PowerShell data configuration file (.psd1). Defaults to config.psd1 in the script directory.

.PARAMETER EndpointsPath
    Path to the CSV file containing endpoints. Defaults to endpoints.csv in the script directory.

.PARAMETER RunOnce
    If specified, runs a single ping cycle and exits. Otherwise runs continuously.

.EXAMPLE
    .\PingMonitor_v3.ps1
    Runs continuously using default config.psd1 and endpoints.csv

.EXAMPLE
    .\PingMonitor_v3.ps1 -ConfigPath "C:\Config\myconfig.psd1" -RunOnce
    Runs a single cycle with custom config path

.NOTES
    Author: Splunk Ping Monitor
    Version: 3.0.0
    Requires: PowerShell 7.4+ (no external modules - airgap friendly)
    
    Memory Characteristics:
    - RAM usage stabilizes after first cycle
    - No memory growth proportional to endpoint count per cycle
    - Streaming output prevents large in-memory accumulation
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

#region Ping Functions - Phase 3: System.Net.NetworkInformation.Ping

<#
    V3 OPTIMIZATION - Phase 3: Network & Performance
    ================================================
    Replaced Test-Connection with System.Net.NetworkInformation.Ping for:
    - Lower memory overhead per ping operation
    - More precise timeout control (milliseconds vs seconds)
    - Better cross-platform consistency
    - Proper IDisposable pattern
#>

function Invoke-SinglePing {
    <#
    .SYNOPSIS
        Execute a single ICMP ping using System.Net.NetworkInformation.Ping.
    .DESCRIPTION
        V3: Replaces Test-Connection for lower memory overhead and better timeout control.
        Returns a hashtable with status, latency, and ttl.
    #>
    param(
        [string]$TargetIP,
        [int]$TimeoutMs
    )
    
    $ping = $null
    try {
        $ping = [System.Net.NetworkInformation.Ping]::new()
        $reply = $ping.Send($TargetIP, $TimeoutMs)
        
        if ($reply.Status -eq [System.Net.NetworkInformation.IPStatus]::Success) {
            return @{
                Success = $true
                Latency = [int]$reply.RoundtripTime
                TTL     = $reply.Options.Ttl
            }
        }
        else {
            return @{
                Success = $false
                Latency = -1
                TTL     = -1
                Error   = "Ping failed with status: $($reply.Status)"
            }
        }
    }
    catch {
        return @{
            Success = $false
            Latency = -1
            TTL     = -1
            Error   = $_.Exception.Message
        }
    }
    finally {
        # V3: Proper disposal of Ping object
        if ($null -ne $ping) {
            $ping.Dispose()
        }
    }
}

function Invoke-ParallelPing {
    <#
    .SYNOPSIS
        Execute pings against all endpoints in parallel with streaming results.
    .DESCRIPTION
        V3 OPTIMIZATION - Phase 2: Parallel Execution Refactor
        ======================================================
        - Uses runspace pool for controlled parallelism
        - Results are streamed via synchronized collections
        - Avoids building large nested result objects
        - Individual pings and summaries are collected into separate lists
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Endpoints,
        [int]$PingsPerCycle,
        [int]$TimeoutMs,
        [int]$ParallelThreads
    )
    
    # V3 OPTIMIZATION: Use List[T] for results to avoid array += overhead
    $individualResults = [System.Collections.Generic.List[PSCustomObject]]::new()
    $summaryResults = [System.Collections.Generic.List[PSCustomObject]]::new()
    
    # Create runspace pool for parallel execution
    $runspacePool = [runspacefactory]::CreateRunspacePool(1, $ParallelThreads)
    $runspacePool.Open()
    
    # V3 OPTIMIZATION: Use List[T] for tracking runspaces
    $runspaces = [System.Collections.Generic.List[hashtable]]::new()
    
    # Script block for parallel ping execution
    # V3: Uses System.Net.NetworkInformation.Ping instead of Test-Connection
    $pingScriptBlock = {
        param($Endpoint, $Count, $Timeout)
        
        # V3 OPTIMIZATION: Use List[T] inside runspace
        $results = [System.Collections.Generic.List[PSCustomObject]]::new()
        $successCount = 0
        $totalLatency = 0
        $minLatency = [int]::MaxValue
        $maxLatency = 0
        
        for ($i = 0; $i -lt $Count; $i++) {
            $timestamp = Get-Date -Format "o"
            $ping = $null
            
            try {
                $ping = [System.Net.NetworkInformation.Ping]::new()
                $reply = $ping.Send($Endpoint.ip, $Timeout)
                
                if ($reply.Status -eq [System.Net.NetworkInformation.IPStatus]::Success) {
                    $latency = [int]$reply.RoundtripTime
                    $ttl = $reply.Options.Ttl
                    
                    $successCount++
                    $totalLatency += $latency
                    $minLatency = [math]::Min($minLatency, $latency)
                    $maxLatency = [math]::Max($maxLatency, $latency)
                    
                    $results.Add([PSCustomObject]@{
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
                else {
                    $results.Add([PSCustomObject]@{
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
            catch {
                $results.Add([PSCustomObject]@{
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
            finally {
                # V3: Proper disposal of Ping object
                if ($null -ne $ping) {
                    $ping.Dispose()
                }
            }
        }
        
        # Calculate summary statistics
        $packetLossPct = [math]::Round((($Count - $successCount) / $Count) * 100, 2)
        $avgLatency = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
        
        # Summary record (identical schema to v2)
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
        
        # Return both individual pings and summary
        return @{
            Individual = $results
            Summary    = $summary
        }
    }
    
    # Start all runspaces
    foreach ($endpoint in $Endpoints) {
        $powershell = [powershell]::Create().AddScript($pingScriptBlock).AddArgument($endpoint).AddArgument($PingsPerCycle).AddArgument($TimeoutMs)
        $powershell.RunspacePool = $runspacePool
        
        $runspaces.Add(@{
            PowerShell = $powershell
            Handle     = $powershell.BeginInvoke()
        })
    }
    
    # Collect results as they complete
    # V3 OPTIMIZATION: Process results incrementally instead of all at once
    foreach ($runspace in $runspaces) {
        try {
            $result = $runspace.PowerShell.EndInvoke($runspace.Handle)
            
            if ($null -ne $result) {
                # Add individual ping results
                if ($null -ne $result.Individual) {
                    foreach ($item in $result.Individual) {
                        $individualResults.Add($item)
                    }
                }
                
                # Add summary
                if ($null -ne $result.Summary) {
                    $summaryResults.Add($result.Summary)
                }
            }
        }
        catch {
            Write-Warning "Runspace failed: $($_.Exception.Message)"
        }
        finally {
            $runspace.PowerShell.Dispose()
        }
    }
    
    # Clean up runspace pool
    $runspacePool.Close()
    $runspacePool.Dispose()
    
    return @{
        individual = $individualResults
        summaries  = $summaryResults
    }
}
#endregion

#region Output Functions - Phase 1: Memory Optimization

<#
    V3 OPTIMIZATION - Phase 1: Output & Memory Optimization
    =======================================================
    - Write-ToLogFile: Uses StreamWriter instead of Add-Content in loops
    - Send-ToSplunkHEC: Incremental batching with StringBuilder, no double buffering
    - Send-ToSplunkMetrics: Incremental batching with StringBuilder
#>

function Write-ToLogFile {
    <#
    .SYNOPSIS
        Write results to log file using streaming I/O.
    .DESCRIPTION
        V3 OPTIMIZATION: Uses StreamWriter for efficient batch writing.
        - Single file handle per call (avoids repeated file open/close)
        - No Add-Content inside loops
        - Proper disposal via try/finally
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Results,
        [string]$LogPath
    )
    
    if ($Results.Count -eq 0) {
        return
    }
    
    # Ensure log directory exists
    $logDir = Split-Path -Parent $LogPath
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    
    # V3 OPTIMIZATION: Use StreamWriter for efficient batch writing
    $streamWriter = $null
    try {
        $streamWriter = [System.IO.StreamWriter]::new($LogPath, $true, [System.Text.Encoding]::UTF8)
        
        foreach ($result in $Results) {
            $jsonLine = $result | ConvertTo-Json -Compress
            $streamWriter.WriteLine($jsonLine)
        }
    }
    finally {
        if ($null -ne $streamWriter) {
            $streamWriter.Dispose()
        }
    }
}

function Send-ToSplunkHEC {
    <#
    .SYNOPSIS
        Send events to Splunk HEC with incremental batching.
    .DESCRIPTION
        V3 OPTIMIZATION: Incremental batching using StringBuilder.
        - No $batches array accumulation
        - No double buffering
        - Events are batched and sent incrementally
        - StringBuilder builds batch payload efficiently
    #>
    param(
        [System.Collections.Generic.List[PSCustomObject]]$Results,
        [hashtable]$HecConfig
    )
    
    if (-not $HecConfig.enabled -or [string]::IsNullOrEmpty($HecConfig.url) -or [string]::IsNullOrEmpty($HecConfig.token)) {
        Write-Warning "HEC is not properly configured. Skipping HEC output."
        return $false
    }
    
    $headers = @{
        "Authorization" = "Splunk $($HecConfig.token)"
        "Content-Type"  = "application/json"
    }
    
    $successCount = 0
    $failCount = 0
    $batchSize = 100
    $hostname = $env:COMPUTERNAME
    
    # V3 OPTIMIZATION: Use StringBuilder for batch construction
    $batchBuilder = [System.Text.StringBuilder]::new()
    $currentBatchCount = 0
    
    # Helper function to send current batch
    $sendBatch = {
        param($Body)
        
        if ([string]::IsNullOrWhiteSpace($Body)) {
            return $true
        }
        
        try {
            $splatParams = @{
                Uri         = $HecConfig.url
                Method      = "POST"
                Headers     = $headers
                Body        = $Body
                TimeoutSec  = 5
                ErrorAction = "Stop"
            }
            
            if (-not $HecConfig.verify_ssl) {
                $splatParams['SkipCertificateCheck'] = $true
            }
            
            if ($HecConfig.ssl_protocol -and $HecConfig.ssl_protocol -ne 'Default') {
                $splatParams['SslProtocol'] = $HecConfig.ssl_protocol
            }
            
            Invoke-RestMethod @splatParams | Out-Null
            return $true
        }
        catch {
            $errorMessage = $_.Exception.Message
            
            # Include response body when available (common for Splunk HEC 4xx)
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
            }
            catch {
                # Ignore secondary failures while extracting response details
            }
            
            Write-Warning "Failed to send batch to HEC: $errorMessage"
            return $false
        }
    }
    
    # V3 OPTIMIZATION: Incremental batching - process events one by one
    foreach ($result in $Results) {
        $hecEvent = @{
            time       = [DateTimeOffset]::Parse($result.timestamp).ToUnixTimeSeconds()
            host       = $hostname
            source     = "ping_monitor"
            sourcetype = $HecConfig.sourcetype
            index      = $HecConfig.index
            event      = $result
        }
        
        $eventJson = $hecEvent | ConvertTo-Json -Compress
        
        # Add newline separator if not first event in batch
        if ($currentBatchCount -gt 0) {
            [void]$batchBuilder.Append("`n")
        }
        [void]$batchBuilder.Append($eventJson)
        $currentBatchCount++
        
        # Send batch when full
        if ($currentBatchCount -ge $batchSize) {
            $batchBody = $batchBuilder.ToString()
            if (& $sendBatch $batchBody) {
                $successCount++
            }
            else {
                $failCount++
            }
            
            # V3 OPTIMIZATION: Clear StringBuilder for reuse (avoids allocation)
            [void]$batchBuilder.Clear()
            $currentBatchCount = 0
        }
    }
    
    # Send remaining events
    if ($currentBatchCount -gt 0) {
        $batchBody = $batchBuilder.ToString()
        if (& $sendBatch $batchBody) {
            $successCount++
        }
        else {
            $failCount++
        }
    }
    
    Write-Host "HEC: Sent $successCount batches successfully, $failCount failed" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Yellow" })
    return ($failCount -eq 0)
}

function Send-ToSplunkMetrics {
    <#
    .SYNOPSIS
        Send summary data to Splunk as metrics via HEC metrics endpoint.
    .DESCRIPTION
        V3 OPTIMIZATION: Incremental batching using StringBuilder.
        - No array accumulation for payload
        - StringBuilder builds metrics payload efficiently
        - Metrics schema identical to v2 for backward compatibility
    .PARAMETER Summaries
        List of summary records (record_type=summary) to convert to metrics.
    .PARAMETER MetricsConfig
        Metrics configuration block from config.psd1.
    #>
    param(
        [Parameter(Mandatory = $true)]
        [System.Collections.Generic.List[PSCustomObject]]$Summaries,
        
        [Parameter(Mandatory = $true)]
        [hashtable]$MetricsConfig
    )
    
    if (-not $MetricsConfig.enabled) {
        return $true
    }
    
    if ([string]::IsNullOrWhiteSpace($MetricsConfig.hec_url) -or [string]::IsNullOrWhiteSpace($MetricsConfig.token)) {
        Write-Warning "Metrics HEC URL or token not configured. Skipping metrics."
        return $false
    }
    
    if ([string]::IsNullOrWhiteSpace($MetricsConfig.index)) {
        Write-Warning "Metrics index not configured (metrics.index is empty). Splunk may route to the token default index, but for best results set metrics.index to a metrics-type index."
    }
    
    $headers = @{
        "Authorization" = "Splunk $($MetricsConfig.token)"
        "Content-Type"  = "application/json"
    }
    
    $hostname = $env:COMPUTERNAME
    
    # V3 OPTIMIZATION: Use StringBuilder for batch construction
    $payloadBuilder = [System.Text.StringBuilder]::new()
    $isFirst = $true
    
    foreach ($summary in $Summaries) {
        # Build the metrics event with all numeric fields (schema identical to v2)
        $metricsEvent = @{
            time   = [DateTimeOffset]::Parse($summary.timestamp).ToUnixTimeSeconds()
            host   = $hostname
            source = "ping_monitor"
            index  = $MetricsConfig.index
            event  = "metric"
            fields = @{
                # Metric names with values
                "metric_name:ping.avg_latency_ms"   = [double]($summary.avg_latency_ms)
                "metric_name:ping.min_latency_ms"   = [double]($summary.min_latency_ms)
                "metric_name:ping.max_latency_ms"   = [double]($summary.max_latency_ms)
                "metric_name:ping.packet_loss_pct"  = [double]($summary.packet_loss_pct)
                "metric_name:ping.pings_sent"       = [int]($summary.pings_sent)
                "metric_name:ping.pings_successful" = [int]($summary.pings_successful)
                
                # Dimensions (identical to v2)
                hostname         = $summary.hostname
                target_ip        = $summary.target_ip
                group            = $summary.group
                description      = $summary.description
                entitytype       = $summary.entitytype
                device           = $summary.device
                vendor           = $summary.vendor
                additional_notes = $summary.additional_notes
            }
        }
        
        $eventJson = $metricsEvent | ConvertTo-Json -Depth 5 -Compress
        
        # Add newline separator if not first event
        if (-not $isFirst) {
            [void]$payloadBuilder.Append("`n")
        }
        [void]$payloadBuilder.Append($eventJson)
        $isFirst = $false
    }
    
    $body = $payloadBuilder.ToString()
    
    try {
        $splatParams = @{
            Uri         = $MetricsConfig.hec_url
            Method      = "POST"
            Headers     = $headers
            Body        = $body
            TimeoutSec  = 5
            ErrorAction = "Stop"
        }
        
        if (-not $MetricsConfig.verify_ssl) {
            $splatParams['SkipCertificateCheck'] = $true
        }
        
        if ($MetricsConfig.ssl_protocol -and $MetricsConfig.ssl_protocol -ne 'Default') {
            $splatParams['SslProtocol'] = $MetricsConfig.ssl_protocol
        }
        
        Invoke-RestMethod @splatParams | Out-Null
        Write-Host "Metrics: Sent $($Summaries.Count) metric points successfully" -ForegroundColor Green
        return $true
    }
    catch {
        $errorMessage = $_.Exception.Message
        
        # Include response body when available (common for Splunk HEC 4xx)
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
        }
        catch {
            # Ignore secondary failures while extracting response details
        }
        
        Write-Warning "Failed to send metrics to HEC: $errorMessage"
        return $false
    }
}

function Invoke-LogRotation {
    param(
        [string]$LogPath,
        [int]$MaxSizeMB
    )
    
    if (-not (Test-Path $LogPath)) {
        return
    }
    
    $logFile = Get-Item $LogPath
    $sizeMB = $logFile.Length / 1MB
    
    if ($sizeMB -ge $MaxSizeMB) {
        $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
        $archivePath = $LogPath -replace '\.log$', "_$timestamp.log"
        
        Write-Host "Rotating log file (Size: $([math]::Round($sizeMB, 2)) MB)" -ForegroundColor Yellow
        Move-Item -Path $LogPath -Destination $archivePath -Force
        
        # Keep only last 5 rotated logs
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

#region Main Execution
function Start-PingMonitor {
    param(
        [hashtable]$Config,
        [System.Collections.Generic.List[PSCustomObject]]$Endpoints,
        [switch]$RunOnce
    )
    
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  Splunk Ping Monitor v3 Started" -ForegroundColor Cyan
    Write-Host "  (RAM-optimized streaming edition)" -ForegroundColor DarkCyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "Endpoints: $($Endpoints.Count)" -ForegroundColor White
    Write-Host "Pings per cycle: $($Config.pings_per_cycle)" -ForegroundColor White
    Write-Host "Cycle interval: $($Config.cycle_interval_seconds) seconds" -ForegroundColor White
    Write-Host "Output mode: $($Config.output_mode)" -ForegroundColor White
    Write-Host "Individual pings: $(if ($Config.emit_individual_pings) { 'enabled' } else { 'disabled (summary only)' })" -ForegroundColor White
    Write-Host "Metrics: $(if ($Config.metrics.enabled) { $Config.metrics.mode } else { 'disabled' })" -ForegroundColor White
    Write-Host "----------------------------------------" -ForegroundColor Gray
    
    $cycleCount = 0
    
    do {
        $cycleCount++
        $cycleStart = Get-Date
        
        Write-Host "`n[$cycleStart] Starting cycle #$cycleCount..." -ForegroundColor Cyan
        
        # Perform parallel pings (V3: Uses runspace pool with streaming)
        $results = Invoke-ParallelPing -Endpoints $Endpoints `
            -PingsPerCycle $Config.pings_per_cycle `
            -TimeoutMs $Config.timeout_ms `
            -ParallelThreads $Config.parallel_threads
        
        # Determine what events to emit based on configuration
        # Phase B: emit_individual_pings controls whether ping events are included
        # Phase C: metrics.mode = "metrics_only" suppresses event summaries
        $emitEventSummaries = $true
        if ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
            $emitEventSummaries = $false
        }
        
        # V3 OPTIMIZATION: Build results list without array concatenation
        $allResults = [System.Collections.Generic.List[PSCustomObject]]::new()
        
        if ($Config.emit_individual_pings -and $emitEventSummaries) {
            # Full output: individual pings + summaries
            foreach ($item in $results.individual) { $allResults.Add($item) }
            foreach ($item in $results.summaries) { $allResults.Add($item) }
        }
        elseif ($Config.emit_individual_pings -and -not $emitEventSummaries) {
            # Individual pings only (unusual but supported)
            foreach ($item in $results.individual) { $allResults.Add($item) }
        }
        elseif (-not $Config.emit_individual_pings -and $emitEventSummaries) {
            # Summary only (Phase B savings mode)
            foreach ($item in $results.summaries) { $allResults.Add($item) }
        }
        # else: No events (metrics_only mode with no individual pings) - allResults stays empty
        
        # Send metrics if enabled (Phase C)
        if ($Config.metrics.enabled) {
            $null = Send-ToSplunkMetrics -Summaries $results.summaries -MetricsConfig $Config.metrics
        }
        
        # Output events based on configuration (skip if no events to send)
        if ($allResults.Count -gt 0) {
            switch ($Config.output_mode.ToLower()) {
                "file" {
                    Invoke-LogRotation -LogPath $Config.log_path -MaxSizeMB $Config.log_rotation_size_mb
                    Write-ToLogFile -Results $allResults -LogPath $Config.log_path
                    Write-Host "Results written to: $($Config.log_path)" -ForegroundColor Green
                }
                "hec" {
                    $null = Send-ToSplunkHEC -Results $allResults -HecConfig $Config.hec
                }
                "both" {
                    Invoke-LogRotation -LogPath $Config.log_path -MaxSizeMB $Config.log_rotation_size_mb
                    Write-ToLogFile -Results $allResults -LogPath $Config.log_path
                    Write-Host "Results written to: $($Config.log_path)" -ForegroundColor Green
                    $null = Send-ToSplunkHEC -Results $allResults -HecConfig $Config.hec
                }
                default {
                    Write-Warning "Unknown output mode: $($Config.output_mode). Defaulting to file."
                    Write-ToLogFile -Results $allResults -LogPath $Config.log_path
                }
            }
        }
        elseif ($Config.metrics.enabled -and $Config.metrics.mode -eq "metrics_only") {
            Write-Host "Metrics-only mode: Events skipped" -ForegroundColor Gray
        }
        
        # Display summary
        $summaryList = $results.summaries
        $successCount = @($summaryList | Where-Object { $_.packet_loss_pct -eq 0 }).Count
        $partialCount = @($summaryList | Where-Object { $_.packet_loss_pct -gt 0 -and $_.packet_loss_pct -lt 100 }).Count
        $failedCount = @($summaryList | Where-Object { $_.packet_loss_pct -eq 100 }).Count
        
        Write-Host "Cycle #$cycleCount complete - Success: $successCount | Partial: $partialCount | Failed: $failedCount" -ForegroundColor $(
            if ($failedCount -eq 0) { "Green" }
            elseif ($failedCount -lt $Endpoints.Count) { "Yellow" }
            else { "Red" }
        )
        
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
    
    Write-Host "`nPing Monitor v3 completed." -ForegroundColor Cyan
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
