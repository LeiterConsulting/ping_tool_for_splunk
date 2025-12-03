#Requires -Version 7.4
<#
.SYNOPSIS
    Splunk Ping Monitor - Monitors endpoint availability and sends results to Splunk.

.DESCRIPTION
    This script pings endpoints defined in a CSV file and outputs results either to a log file
    (for Splunk Universal Forwarder ingestion) or directly to Splunk via HTTP Event Collector (HEC).

.PARAMETER ConfigPath
    Path to the PowerShell data configuration file (.psd1). Defaults to config.psd1 in the script directory.

.PARAMETER EndpointsPath
    Path to the CSV file containing endpoints. Defaults to endpoints.csv in the script directory.

.PARAMETER RunOnce
    If specified, runs a single ping cycle and exits. Otherwise runs continuously.

.EXAMPLE
    .\PingMonitor.ps1
    Runs continuously using default config.psd1 and endpoints.csv

.EXAMPLE
    .\PingMonitor.ps1 -ConfigPath "C:\Config\myconfig.psd1" -RunOnce
    Runs a single cycle with custom config path

.NOTES
    Author: Splunk Ping Monitor
    Version: 1.2.0
    Requires: PowerShell 7.4+ (no external modules - airgap friendly)
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
        pings_per_cycle = 4
        cycle_interval_seconds = 60
        timeout_ms = 1000
        parallel_threads = 10
        output_mode = "file"
        log_path = Join-Path $ScriptDir "logs\ping_results.log"
        log_rotation_size_mb = 50
        hec = @{
            enabled = $false
            url = ""
            token = ""
            index = "main"
            sourcetype = "ping_monitor"
            verify_ssl = $true
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
    $endpoints = @()
    
    foreach ($row in $csvData) {
        # Check for required fields (ip and hostname)
        if (-not $row.ip -or -not $row.hostname) {
            Write-Warning "Skipping row with missing ip or hostname: $($row | ConvertTo-Json -Compress)"
            continue
        }
        
        $endpoint = [PSCustomObject]@{
            ip          = $row.ip.Trim()
            hostname    = $row.hostname.Trim()
            group       = if ($row.PSObject.Properties['group'] -and $row.group) { $row.group.Trim() } else { "default" }
            description = if ($row.PSObject.Properties['description'] -and $row.description) { $row.description.Trim() } else { "" }
        }
        
        $endpoints += $endpoint
    }
    
    if ($endpoints.Count -eq 0) {
        throw "No valid endpoints found in CSV file"
    }
    
    Write-Host "Loaded $($endpoints.Count) endpoints from CSV" -ForegroundColor Green
    return $endpoints
}
#endregion

#region Ping Functions
function Invoke-PingEndpoint {
    param(
        [PSCustomObject]$Endpoint,
        [int]$Count,
        [int]$TimeoutMs
    )
    
    $results = @()
    $successCount = 0
    $totalLatency = 0
    $minLatency = [int]::MaxValue
    $maxLatency = 0
    
    for ($i = 0; $i -lt $Count; $i++) {
        $timestamp = Get-Date -Format "o"
        
        try {
            $pingResult = Test-Connection -TargetName $Endpoint.ip -Count 1 -TimeoutSeconds ([math]::Ceiling($TimeoutMs / 1000)) -ErrorAction Stop
            
            $latency = [int]$pingResult.Latency
            $ttl = $pingResult.Reply.Options.Ttl
            
            $successCount++
            $totalLatency += $latency
            $minLatency = [math]::Min($minLatency, $latency)
            $maxLatency = [math]::Max($maxLatency, $latency)
            
            $results += [PSCustomObject]@{
                timestamp       = $timestamp
                target_ip       = $Endpoint.ip
                hostname        = $Endpoint.hostname
                group           = $Endpoint.group
                description     = $Endpoint.description
                status          = "success"
                latency_ms      = $latency
                ttl             = $ttl
                ping_number     = ($i + 1)
                pings_in_cycle  = $Count
            }
        }
        catch {
            $results += [PSCustomObject]@{
                timestamp       = $timestamp
                target_ip       = $Endpoint.ip
                hostname        = $Endpoint.hostname
                group           = $Endpoint.group
                description     = $Endpoint.description
                status          = "failed"
                latency_ms      = -1
                ttl             = -1
                ping_number     = ($i + 1)
                pings_in_cycle  = $Count
                error_message   = $_.Exception.Message
            }
        }
    }
    
    # Calculate summary statistics
    $packetLossPct = [math]::Round((($Count - $successCount) / $Count) * 100, 2)
    $avgLatency = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
    
    # Add summary record
    $summaryTimestamp = Get-Date -Format "o"
    $summary = [PSCustomObject]@{
        timestamp           = $summaryTimestamp
        target_ip           = $Endpoint.ip
        hostname            = $Endpoint.hostname
        group               = $Endpoint.group
        description         = $Endpoint.description
        record_type         = "summary"
        pings_sent          = $Count
        pings_successful    = $successCount
        pings_failed        = ($Count - $successCount)
        packet_loss_pct     = $packetLossPct
        avg_latency_ms      = $avgLatency
        min_latency_ms      = if ($successCount -gt 0) { $minLatency } else { -1 }
        max_latency_ms      = if ($successCount -gt 0) { $maxLatency } else { -1 }
    }
    
    return @{
        individual = $results
        summary    = $summary
    }
}

function Invoke-ParallelPing {
    param(
        [array]$Endpoints,
        [int]$PingsPerCycle,
        [int]$TimeoutMs,
        [int]$ParallelThreads
    )
    
    $allResults = @{
        individual = @()
        summaries  = @()
    }
    
    # Use ForEach-Object -Parallel for concurrent pinging
    $results = $Endpoints | ForEach-Object -ThrottleLimit $ParallelThreads -Parallel {
        # Import function in parallel scope
        $endpoint = $_
        $count = $using:PingsPerCycle
        $timeout = $using:TimeoutMs
        
        $results = @()
        $successCount = 0
        $totalLatency = 0
        $minLatency = [int]::MaxValue
        $maxLatency = 0
        
        for ($i = 0; $i -lt $count; $i++) {
            $timestamp = Get-Date -Format "o"
            
            try {
                $pingResult = Test-Connection -TargetName $endpoint.ip -Count 1 -TimeoutSeconds ([math]::Ceiling($timeout / 1000)) -ErrorAction Stop
                
                $latency = [int]$pingResult.Latency
                $ttl = $pingResult.Reply.Options.Ttl
                
                $successCount++
                $totalLatency += $latency
                $minLatency = [math]::Min($minLatency, $latency)
                $maxLatency = [math]::Max($maxLatency, $latency)
                
                $results += [PSCustomObject]@{
                    timestamp       = $timestamp
                    target_ip       = $endpoint.ip
                    hostname        = $endpoint.hostname
                    group           = $endpoint.group
                    description     = $endpoint.description
                    status          = "success"
                    latency_ms      = $latency
                    ttl             = $ttl
                    ping_number     = ($i + 1)
                    pings_in_cycle  = $count
                    record_type     = "ping"
                }
            }
            catch {
                $results += [PSCustomObject]@{
                    timestamp       = $timestamp
                    target_ip       = $endpoint.ip
                    hostname        = $endpoint.hostname
                    group           = $endpoint.group
                    description     = $endpoint.description
                    status          = "failed"
                    latency_ms      = -1
                    ttl             = -1
                    ping_number     = ($i + 1)
                    pings_in_cycle  = $count
                    error_message   = $_.Exception.Message
                    record_type     = "ping"
                }
            }
        }
        
        # Calculate summary statistics
        $packetLossPct = [math]::Round((($count - $successCount) / $count) * 100, 2)
        $avgLatency = if ($successCount -gt 0) { [math]::Round($totalLatency / $successCount, 2) } else { -1 }
        
        # Summary record
        $summary = [PSCustomObject]@{
            timestamp           = (Get-Date -Format "o")
            target_ip           = $endpoint.ip
            hostname            = $endpoint.hostname
            group               = $endpoint.group
            description         = $endpoint.description
            record_type         = "summary"
            pings_sent          = $count
            pings_successful    = $successCount
            pings_failed        = ($count - $successCount)
            packet_loss_pct     = $packetLossPct
            avg_latency_ms      = $avgLatency
            min_latency_ms      = if ($successCount -gt 0) { $minLatency } else { -1 }
            max_latency_ms      = if ($successCount -gt 0) { $maxLatency } else { -1 }
        }
        
        return @{
            individual = $results
            summary    = $summary
        }
    }
    
    # Collect all results
    foreach ($result in $results) {
        $allResults.individual += $result.individual
        $allResults.summaries += $result.summary
    }
    
    return $allResults
}
#endregion

#region Output Functions
function Write-ToLogFile {
    param(
        [array]$Results,
        [string]$LogPath
    )
    
    # Ensure log directory exists
    $logDir = Split-Path -Parent $LogPath
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    
    # Write each result as a JSON line
    foreach ($result in $Results) {
        $jsonLine = $result | ConvertTo-Json -Compress
        Add-Content -Path $LogPath -Value $jsonLine -Encoding UTF8
    }
}

function Send-ToSplunkHEC {
    param(
        [array]$Results,
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
    
    # Batch events for efficiency
    $batchSize = 100
    $batches = @()
    $currentBatch = @()
    
    foreach ($result in $Results) {
        $event = @{
            time       = [DateTimeOffset]::Parse($result.timestamp).ToUnixTimeSeconds()
            host       = $env:COMPUTERNAME
            source     = "ping_monitor"
            sourcetype = $HecConfig.sourcetype
            index      = $HecConfig.index
            event      = $result
        }
        
        $currentBatch += ($event | ConvertTo-Json -Compress)
        
        if ($currentBatch.Count -ge $batchSize) {
            $batches += ,($currentBatch -join "`n")
            $currentBatch = @()
        }
    }
    
    # Don't forget remaining items
    if ($currentBatch.Count -gt 0) {
        $batches += ,($currentBatch -join "`n")
    }
    
    # Send batches
    foreach ($batch in $batches) {
        try {
            $splatParams = @{
                Uri         = $HecConfig.url
                Method      = "POST"
                Headers     = $headers
                Body        = $batch
                ErrorAction = "Stop"
            }
            
            if (-not $HecConfig.verify_ssl) {
                $splatParams['SkipCertificateCheck'] = $true
            }
            
            $response = Invoke-RestMethod @splatParams
            $successCount++
        }
        catch {
            Write-Warning "Failed to send batch to HEC: $($_.Exception.Message)"
            $failCount++
        }
    }
    
    Write-Host "HEC: Sent $successCount batches successfully, $failCount failed" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Yellow" })
    return ($failCount -eq 0)
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
        [array]$Endpoints,
        [switch]$RunOnce
    )
    
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  Splunk Ping Monitor Started" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "Endpoints: $($Endpoints.Count)" -ForegroundColor White
    Write-Host "Pings per cycle: $($Config.pings_per_cycle)" -ForegroundColor White
    Write-Host "Cycle interval: $($Config.cycle_interval_seconds) seconds" -ForegroundColor White
    Write-Host "Output mode: $($Config.output_mode)" -ForegroundColor White
    Write-Host "----------------------------------------" -ForegroundColor Gray
    
    $cycleCount = 0
    
    do {
        $cycleCount++
        $cycleStart = Get-Date
        
        Write-Host "`n[$cycleStart] Starting cycle #$cycleCount..." -ForegroundColor Cyan
        
        # Perform parallel pings
        $results = Invoke-ParallelPing -Endpoints $Endpoints `
                                       -PingsPerCycle $Config.pings_per_cycle `
                                       -TimeoutMs $Config.timeout_ms `
                                       -ParallelThreads $Config.parallel_threads
        
        # Combine all results for output
        $allResults = $results.individual + $results.summaries
        
        # Output based on configuration
        switch ($Config.output_mode.ToLower()) {
            "file" {
                Invoke-LogRotation -LogPath $Config.log_path -MaxSizeMB $Config.log_rotation_size_mb
                Write-ToLogFile -Results $allResults -LogPath $Config.log_path
                Write-Host "Results written to: $($Config.log_path)" -ForegroundColor Green
            }
            "hec" {
                Send-ToSplunkHEC -Results $allResults -HecConfig $Config.hec
            }
            "both" {
                Invoke-LogRotation -LogPath $Config.log_path -MaxSizeMB $Config.log_rotation_size_mb
                Write-ToLogFile -Results $allResults -LogPath $Config.log_path
                Write-Host "Results written to: $($Config.log_path)" -ForegroundColor Green
                Send-ToSplunkHEC -Results $allResults -HecConfig $Config.hec
            }
            default {
                Write-Warning "Unknown output mode: $($Config.output_mode). Defaulting to file."
                Write-ToLogFile -Results $allResults -LogPath $Config.log_path
            }
        }
        
        # Display summary
        $summaryArray = @($results.summaries)
        $successCount = @($summaryArray | Where-Object { $_.packet_loss_pct -eq 0 }).Count
        $partialCount = @($summaryArray | Where-Object { $_.packet_loss_pct -gt 0 -and $_.packet_loss_pct -lt 100 }).Count
        $failedCount = @($summaryArray | Where-Object { $_.packet_loss_pct -eq 100 }).Count
        
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
    
    Write-Host "`nPing Monitor completed." -ForegroundColor Cyan
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
