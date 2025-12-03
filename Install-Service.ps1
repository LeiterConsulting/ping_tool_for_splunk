#Requires -Version 7.4
#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs or uninstalls the Splunk Ping Monitor as a Windows Service using NSSM.

.DESCRIPTION
    This script uses NSSM (Non-Sucking Service Manager) to create a Windows Service
    that runs the PingMonitor.ps1 script continuously.

.PARAMETER Install
    Installs the service.

.PARAMETER Uninstall
    Uninstalls the service.

.PARAMETER Start
    Starts the service (if installed).

.PARAMETER Stop
    Stops the service (if running).

.PARAMETER Status
    Shows the current service status.

.EXAMPLE
    .\Install-Service.ps1 -Install
    Installs the Ping Monitor as a Windows Service

.EXAMPLE
    .\Install-Service.ps1 -Uninstall
    Removes the Windows Service

.NOTES
    Requires Administrator privileges
    Requires NSSM to be installed or downloaded
#>

[CmdletBinding(DefaultParameterSetName = 'Status')]
param(
    [Parameter(ParameterSetName = 'Install')]
    [switch]$Install,

    [Parameter(ParameterSetName = 'Uninstall')]
    [switch]$Uninstall,

    [Parameter(ParameterSetName = 'Start')]
    [switch]$Start,

    [Parameter(ParameterSetName = 'Stop')]
    [switch]$Stop,

    [Parameter(ParameterSetName = 'Status')]
    [switch]$Status
)

$ErrorActionPreference = "Stop"

# Configuration
$ServiceName = "SplunkPingMonitor"
$ServiceDisplayName = "Splunk Ping Monitor"
$ServiceDescription = "Monitors network endpoints and sends ping results to Splunk"
$ScriptDir = $PSScriptRoot
$PingMonitorScript = Join-Path $ScriptDir "PingMonitor.ps1"
$NssmPath = Join-Path $ScriptDir "nssm.exe"
$NssmDownloadUrl = "https://nssm.cc/release/nssm-2.24.zip"

#region Helper Functions
function Test-NssmInstalled {
    # Check if NSSM is in PATH
    $nssmInPath = Get-Command "nssm.exe" -ErrorAction SilentlyContinue
    if ($nssmInPath) {
        return $nssmInPath.Source
    }
    
    # Check if NSSM is in script directory
    if (Test-Path $NssmPath) {
        return $NssmPath
    }
    
    return $null
}

function Install-Nssm {
    Write-Host "NSSM not found. Downloading..." -ForegroundColor Yellow
    
    $zipPath = Join-Path $env:TEMP "nssm.zip"
    $extractPath = Join-Path $env:TEMP "nssm"
    
    try {
        # Download NSSM
        Invoke-WebRequest -Uri $NssmDownloadUrl -OutFile $zipPath -UseBasicParsing
        
        # Extract
        Expand-Archive -Path $zipPath -DestinationPath $extractPath -Force
        
        # Find the correct architecture binary
        $arch = if ([Environment]::Is64BitOperatingSystem) { "win64" } else { "win32" }
        $nssmExe = Get-ChildItem -Path $extractPath -Recurse -Filter "nssm.exe" | 
                   Where-Object { $_.DirectoryName -like "*$arch*" } | 
                   Select-Object -First 1
        
        if (-not $nssmExe) {
            throw "Could not find NSSM executable in downloaded archive"
        }
        
        # Copy to script directory
        Copy-Item -Path $nssmExe.FullName -Destination $NssmPath -Force
        
        # Cleanup
        Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
        Remove-Item $extractPath -Recurse -Force -ErrorAction SilentlyContinue
        
        Write-Host "NSSM installed to: $NssmPath" -ForegroundColor Green
        return $NssmPath
    }
    catch {
        Write-Error "Failed to download/install NSSM: $($_.Exception.Message)"
        Write-Host @"

Please manually download NSSM from: https://nssm.cc/download
Extract and place nssm.exe in: $ScriptDir
Or add nssm.exe to your system PATH.
"@ -ForegroundColor Yellow
        exit 1
    }
}

function Get-ServiceStatus {
    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    return $service
}
#endregion

#region Main Actions
function Install-PingMonitorService {
    Write-Host "Installing $ServiceDisplayName..." -ForegroundColor Cyan
    
    # Check if already installed
    $existing = Get-ServiceStatus
    if ($existing) {
        Write-Warning "Service '$ServiceName' is already installed. Use -Uninstall first to reinstall."
        return
    }
    
    # Ensure NSSM is available
    $nssm = Test-NssmInstalled
    if (-not $nssm) {
        $nssm = Install-Nssm
    }
    
    # Verify PingMonitor.ps1 exists
    if (-not (Test-Path $PingMonitorScript)) {
        Write-Error "PingMonitor.ps1 not found at: $PingMonitorScript"
        exit 1
    }
    
    # Find PowerShell 7
    $pwshPath = (Get-Command pwsh -ErrorAction SilentlyContinue).Source
    if (-not $pwshPath) {
        # Try common installation paths
        $commonPaths = @(
            "$env:ProgramFiles\PowerShell\7\pwsh.exe",
            "$env:ProgramFiles(x86)\PowerShell\7\pwsh.exe",
            "$env:LOCALAPPDATA\Microsoft\PowerShell\7\pwsh.exe"
        )
        foreach ($path in $commonPaths) {
            if (Test-Path $path) {
                $pwshPath = $path
                break
            }
        }
    }
    
    if (-not $pwshPath) {
        Write-Error "PowerShell 7 (pwsh.exe) not found. Please install PowerShell 7.4+"
        exit 1
    }
    
    Write-Host "Using PowerShell: $pwshPath" -ForegroundColor Gray
    Write-Host "Using NSSM: $nssm" -ForegroundColor Gray
    
    # Install service with NSSM
    $arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$PingMonitorScript`""
    
    & $nssm install $ServiceName $pwshPath $arguments
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install service"
        exit 1
    }
    
    # Configure service properties
    & $nssm set $ServiceName DisplayName $ServiceDisplayName
    & $nssm set $ServiceName Description $ServiceDescription
    & $nssm set $ServiceName AppDirectory $ScriptDir
    & $nssm set $ServiceName Start SERVICE_AUTO_START
    
    # Configure restart on failure
    & $nssm set $ServiceName AppExit Default Restart
    & $nssm set $ServiceName AppRestartDelay 10000
    
    # Configure logging (stdout/stderr to files)
    $logDir = Join-Path $ScriptDir "logs"
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }
    
    & $nssm set $ServiceName AppStdout (Join-Path $logDir "service_stdout.log")
    & $nssm set $ServiceName AppStderr (Join-Path $logDir "service_stderr.log")
    & $nssm set $ServiceName AppStdoutCreationDisposition 4
    & $nssm set $ServiceName AppStderrCreationDisposition 4
    & $nssm set $ServiceName AppRotateFiles 1
    & $nssm set $ServiceName AppRotateBytes 10485760
    
    Write-Host "`n✅ Service installed successfully!" -ForegroundColor Green
    Write-Host @"

Service Name: $ServiceName
Display Name: $ServiceDisplayName

To start the service:
  .\Install-Service.ps1 -Start
  OR
  Start-Service $ServiceName

To view status:
  .\Install-Service.ps1 -Status
  OR
  Get-Service $ServiceName

To view logs:
  - Service logs: $logDir\service_stdout.log
  - Ping results: $logDir\ping_results.log
"@ -ForegroundColor White
}

function Uninstall-PingMonitorService {
    Write-Host "Uninstalling $ServiceDisplayName..." -ForegroundColor Cyan
    
    $service = Get-ServiceStatus
    if (-not $service) {
        Write-Warning "Service '$ServiceName' is not installed."
        return
    }
    
    # Stop service if running
    if ($service.Status -eq 'Running') {
        Write-Host "Stopping service..." -ForegroundColor Yellow
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }
    
    # Get NSSM
    $nssm = Test-NssmInstalled
    if (-not $nssm) {
        # Try using sc.exe as fallback
        Write-Host "NSSM not found, using sc.exe..." -ForegroundColor Yellow
        & sc.exe delete $ServiceName
    }
    else {
        & $nssm remove $ServiceName confirm
    }
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ Service uninstalled successfully!" -ForegroundColor Green
    }
    else {
        Write-Error "Failed to uninstall service"
    }
}

function Start-PingMonitorService {
    $service = Get-ServiceStatus
    if (-not $service) {
        Write-Error "Service '$ServiceName' is not installed. Use -Install first."
        return
    }
    
    if ($service.Status -eq 'Running') {
        Write-Host "Service is already running." -ForegroundColor Yellow
        return
    }
    
    Write-Host "Starting $ServiceDisplayName..." -ForegroundColor Cyan
    Start-Service -Name $ServiceName
    Start-Sleep -Seconds 2
    
    $service = Get-ServiceStatus
    if ($service.Status -eq 'Running') {
        Write-Host "✅ Service started successfully!" -ForegroundColor Green
    }
    else {
        Write-Warning "Service may not have started. Status: $($service.Status)"
    }
}

function Stop-PingMonitorService {
    $service = Get-ServiceStatus
    if (-not $service) {
        Write-Error "Service '$ServiceName' is not installed."
        return
    }
    
    if ($service.Status -ne 'Running') {
        Write-Host "Service is not running." -ForegroundColor Yellow
        return
    }
    
    Write-Host "Stopping $ServiceDisplayName..." -ForegroundColor Cyan
    Stop-Service -Name $ServiceName -Force
    Start-Sleep -Seconds 2
    
    $service = Get-ServiceStatus
    if ($service.Status -eq 'Stopped') {
        Write-Host "✅ Service stopped successfully!" -ForegroundColor Green
    }
    else {
        Write-Warning "Service may not have stopped. Status: $($service.Status)"
    }
}

function Show-ServiceStatus {
    $service = Get-ServiceStatus
    
    Write-Host "`n========================================" -ForegroundColor Cyan
    Write-Host "  Splunk Ping Monitor Service Status" -ForegroundColor Cyan
    Write-Host "========================================`n" -ForegroundColor Cyan
    
    if (-not $service) {
        Write-Host "Status: NOT INSTALLED" -ForegroundColor Yellow
        Write-Host "`nUse -Install to install the service." -ForegroundColor Gray
        return
    }
    
    $statusColor = switch ($service.Status) {
        'Running' { 'Green' }
        'Stopped' { 'Red' }
        default { 'Yellow' }
    }
    
    Write-Host "Service Name:    $($service.Name)"
    Write-Host "Display Name:    $($service.DisplayName)"
    Write-Host "Status:          $($service.Status)" -ForegroundColor $statusColor
    Write-Host "Start Type:      $($service.StartType)"
    
    # Show recent log entries if available
    $logPath = Join-Path $ScriptDir "logs\ping_results.log"
    if (Test-Path $logPath) {
        $logInfo = Get-Item $logPath
        Write-Host "`nLog File:        $logPath"
        Write-Host "Log Size:        $([math]::Round($logInfo.Length / 1KB, 2)) KB"
        Write-Host "Last Modified:   $($logInfo.LastWriteTime)"
    }
    
    Write-Host "`n----------------------------------------" -ForegroundColor Gray
    Write-Host "Commands:" -ForegroundColor Gray
    Write-Host "  Start:     .\Install-Service.ps1 -Start"
    Write-Host "  Stop:      .\Install-Service.ps1 -Stop"
    Write-Host "  Uninstall: .\Install-Service.ps1 -Uninstall"
}
#endregion

#region Main Execution
switch ($PSCmdlet.ParameterSetName) {
    'Install' { Install-PingMonitorService }
    'Uninstall' { Uninstall-PingMonitorService }
    'Start' { Start-PingMonitorService }
    'Stop' { Stop-PingMonitorService }
    'Status' { Show-ServiceStatus }
}
#endregion
