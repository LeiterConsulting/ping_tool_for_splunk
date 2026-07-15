#Requires -Version 7.4
<#
.SYNOPSIS
    Installs and controls Ping Monitor as a supervised Windows service through NSSM.

.DESCRIPTION
    The Go runtime is the default service target. The installer validates the binary,
    configuration, endpoint inventory, UI bind address, working directory, and log
    directory before changing Windows service state. Installation configures delayed
    automatic start, application restart, SCM recovery, graceful shutdown, and rotating
    stdout/stderr logs.

    Status and Validate do not require elevation. Install, Uninstall, Start, Stop, and
    Restart require an elevated PowerShell session.

.EXAMPLE
    .\Install-Service.ps1 -Validate -BinaryPath D:\pingmonitor\pingmonitor.exe -ConfigPath D:\pingmonitor\config.psd1 -EndpointsPath D:\pingmonitor\endpoints.csv

.EXAMPLE
    .\Install-Service.ps1 -Install -BinaryPath D:\pingmonitor\pingmonitor.exe -ConfigPath D:\pingmonitor\config.psd1 -EndpointsPath D:\pingmonitor\endpoints.csv

.EXAMPLE
    .\Install-Service.ps1 -Restart

.EXAMPLE
    .\Install-Service.ps1 -Status -Json
#>

[CmdletBinding(DefaultParameterSetName = 'Status', SupportsShouldProcess = $true)]
param(
    [Parameter(ParameterSetName = 'Install', Mandatory = $true)]
    [switch]$Install,

    [Parameter(ParameterSetName = 'Uninstall', Mandatory = $true)]
    [switch]$Uninstall,

    [Parameter(ParameterSetName = 'Start', Mandatory = $true)]
    [switch]$Start,

    [Parameter(ParameterSetName = 'Stop', Mandatory = $true)]
    [switch]$Stop,

    [Parameter(ParameterSetName = 'Restart', Mandatory = $true)]
    [switch]$Restart,

    [Parameter(ParameterSetName = 'Validate', Mandatory = $true)]
    [switch]$Validate,

    [Parameter(ParameterSetName = 'Status')]
    [switch]$Status,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [ValidateSet('go', 'powershell')]
    [string]$Runtime = 'go',

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [ValidateSet('v4.0.0', 'v3.3.3')]
    [string]$Version = 'v4.0.0',

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$PingMonitorScriptName,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$BinaryPath,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$ConfigPath,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$EndpointsPath,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$WorkingDirectory,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$LogDirectory,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [string]$UIListen = '0.0.0.0:8080',

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [switch]$DisableUI,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Validate')]
    [switch]$AllowRemoteUI,

    [Parameter(ParameterSetName = 'Install')]
    [ValidateSet('AutomaticDelayedStart', 'Automatic', 'Manual')]
    [string]$StartupType = 'AutomaticDelayedStart',

    [Parameter(ParameterSetName = 'Install')]
    [switch]$ForceReinstall,

    [Parameter(ParameterSetName = 'Install')]
    [switch]$NoStart,

    [Parameter()]
    [ValidatePattern('^[A-Za-z0-9_.-]+$')]
    [string]$ServiceName = 'SplunkPingMonitor',

    [Parameter()]
    [ValidateRange(5, 300)]
    [int]$WaitTimeoutSeconds = 45,

    [Parameter(ParameterSetName = 'Status')]
    [Parameter(ParameterSetName = 'Validate')]
    [switch]$Json
)

$ErrorActionPreference = 'Stop'
$ScriptDir = if ([string]::IsNullOrWhiteSpace($PSScriptRoot)) {
    Split-Path -Parent $MyInvocation.MyCommand.Path
}
else {
    $PSScriptRoot
}

$script:NssmPath = $null

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Assert-Administrator {
    if (-not (Test-IsAdministrator)) {
        throw "This action requires an elevated PowerShell session. Reopen PowerShell with 'Run as administrator'."
    }
}

function Resolve-AbsolutePath {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$BasePath,
        [switch]$MustExist,
        [switch]$Directory
    )

    $candidate = if ([IO.Path]::IsPathRooted($Path)) { $Path } else { Join-Path $BasePath $Path }
    $fullPath = [IO.Path]::GetFullPath($candidate)
    if ($MustExist -and -not (Test-Path -LiteralPath $fullPath)) {
        throw "Required path does not exist: $fullPath"
    }
    if ($MustExist -and $Directory -and -not (Test-Path -LiteralPath $fullPath -PathType Container)) {
        throw "Expected a directory: $fullPath"
    }
    if ($MustExist -and -not $Directory -and -not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
        throw "Expected a file: $fullPath"
    }
    return $fullPath
}

function Find-Nssm {
    $command = Get-Command nssm.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    $bundled = Join-Path $ScriptDir 'nssm.exe'
    if (Test-Path -LiteralPath $bundled -PathType Leaf) {
        return (Resolve-Path -LiteralPath $bundled).Path
    }
    throw "nssm.exe was not found in PATH or beside Install-Service.ps1. Install NSSM 2.24+ before continuing."
}

function Invoke-Nssm {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [switch]$AllowFailure
    )
    if (-not $script:NssmPath) {
        $script:NssmPath = Find-Nssm
    }
    $output = @(& $script:NssmPath @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "NSSM command failed (exit $exitCode): nssm $($Arguments -join ' ')`n$($output -join [Environment]::NewLine)"
    }
    return [pscustomobject]@{ ExitCode = $exitCode; Output = ($output -join [Environment]::NewLine).Trim() }
}

function ConvertTo-ServiceArgumentString {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $quoted = foreach ($argument in $Arguments) {
        if ($argument -notmatch '[\s"]') {
            $argument
            continue
        }
        '"' + ($argument -replace '(\\*)"', '$1$1\"' -replace '(\\+)$', '$1$1') + '"'
    }
    return $quoted -join ' '
}

function Assert-UIListenSafe {
    param([string]$ListenAddress, [bool]$RemoteAllowed)
    if ([string]::IsNullOrWhiteSpace($ListenAddress)) {
        throw 'UIListen cannot be blank unless -DisableUI is used.'
    }

    $hostPart = $null
    $portPart = $null
    if ($ListenAddress -match '^\[(?<host>[^\]]+)\]:(?<port>\d+)$') {
        $hostPart = $Matches.host
        $portPart = [int]$Matches.port
    }
    elseif ($ListenAddress -match '^(?<host>[^:]+):(?<port>\d+)$') {
        $hostPart = $Matches.host
        $portPart = [int]$Matches.port
    }
    else {
        throw "UIListen must use host:port syntax, for example 0.0.0.0:8080. Received: $ListenAddress"
    }
    if ($portPart -lt 1 -or $portPart -gt 65535) {
        throw "UIListen port is outside 1-65535: $portPart"
    }

    # v5.6 intentionally permits non-loopback listeners. -AllowRemoteUI remains
    # accepted for command-line compatibility but is no longer required.
}

function Resolve-GoBinary {
    if ($BinaryPath) {
        return Resolve-AbsolutePath -Path $BinaryPath -BasePath $ScriptDir -MustExist
    }
    $coLocated = Join-Path $ScriptDir 'pingmonitor.exe'
    if (Test-Path -LiteralPath $coLocated -PathType Leaf) {
        return (Resolve-Path -LiteralPath $coLocated).Path
    }
    $candidate = Get-ChildItem -LiteralPath (Join-Path $ScriptDir 'dist') -Filter 'pingmonitor_*_windows_amd64.exe' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if ($candidate) {
        return $candidate.FullName
    }
    throw "Go binary not found. Supply -BinaryPath or build pingmonitor.exe first."
}

function Get-DesiredServiceDefinition {
    if ($Runtime -eq 'go') {
        $application = Resolve-GoBinary
        $workDir = if ($WorkingDirectory) {
            Resolve-AbsolutePath -Path $WorkingDirectory -BasePath $ScriptDir -MustExist -Directory
        }
        else {
            Split-Path -Parent $application
        }
        $configFile = if ($ConfigPath) {
            Resolve-AbsolutePath -Path $ConfigPath -BasePath $ScriptDir -MustExist
        }
        else {
            Resolve-AbsolutePath -Path 'config.psd1' -BasePath $workDir -MustExist
        }
        $endpointFile = if ($EndpointsPath) {
            Resolve-AbsolutePath -Path $EndpointsPath -BasePath $ScriptDir -MustExist
        }
        else {
            Resolve-AbsolutePath -Path 'endpoints.csv' -BasePath $workDir -MustExist
        }
        if (-not $DisableUI) {
            Assert-UIListenSafe -ListenAddress $UIListen -RemoteAllowed $AllowRemoteUI.IsPresent
        }

        $runtimeVersion = @(& $application --version 2>&1)
        if ($LASTEXITCODE -ne 0) {
            throw "The selected Go binary failed its --version check: $application"
        }
        $validationOutput = @(& $application --validate --config $configFile --endpoints $endpointFile 2>&1)
        if ($LASTEXITCODE -ne 0) {
            throw "The selected Go deployment failed runtime validation: $($validationOutput -join ' ')"
        }
        $args = @('--config', $configFile, '--endpoints', $endpointFile)
        if (-not $DisableUI) {
            $args += @('--ui-listen', $UIListen)
        }
        $displayVersion = ($runtimeVersion -join ' ').Trim()
        $displayName = "Splunk Ping Monitor (Go $displayVersion)"
    }
    else {
        $scriptName = if ($PingMonitorScriptName) { $PingMonitorScriptName } elseif ($Version -eq 'v3.3.3') { 'PingMonitor_v3_3_3.ps1' } else { 'PingMonitor_v4_0_0.ps1' }
        $monitorScript = Resolve-AbsolutePath -Path $scriptName -BasePath $ScriptDir -MustExist
        $pwsh = (Get-Command pwsh.exe -ErrorAction SilentlyContinue).Source
        if (-not $pwsh) {
            throw 'PowerShell 7.4 or newer is required for the legacy PowerShell service runtime.'
        }
        $application = $pwsh
        $workDir = if ($WorkingDirectory) { Resolve-AbsolutePath -Path $WorkingDirectory -BasePath $ScriptDir -MustExist -Directory } else { Split-Path -Parent $monitorScript }
        $args = @('-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', $monitorScript)
        $configFile = $null
        $endpointFile = $null
        $displayVersion = $Version
        $displayName = "Splunk Ping Monitor (PowerShell $Version)"
    }

    $logs = if ($LogDirectory) {
        Resolve-AbsolutePath -Path $LogDirectory -BasePath $workDir
    }
    else {
        Join-Path $workDir 'logs'
    }

    return [pscustomobject]@{
        ServiceName       = $ServiceName
        DisplayName       = $displayName
        Description       = "Monitors network endpoints and sends truthful ping observations to Splunk ($displayVersion)"
        Runtime           = $Runtime
        RuntimeVersion    = $displayVersion
        Application       = $application
        Arguments         = $args
        ArgumentString    = ConvertTo-ServiceArgumentString -Arguments $args
        WorkingDirectory  = [IO.Path]::GetFullPath($workDir)
        ConfigPath        = $configFile
        EndpointsPath     = $endpointFile
        UIListen          = if ($DisableUI) { $null } else { $UIListen }
        LogDirectory      = [IO.Path]::GetFullPath($logs)
        StdoutPath        = [IO.Path]::GetFullPath((Join-Path $logs 'service_stdout.log'))
        StderrPath        = [IO.Path]::GetFullPath((Join-Path $logs 'service_stderr.log'))
        StartupType       = $StartupType
    }
}

function Get-ServiceObject {
    return Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
}

function Wait-ServiceState {
    param([Parameter(Mandatory = $true)][string]$DesiredState)
    $service = Get-ServiceObject
    if (-not $service) {
        throw "Service '$ServiceName' is not installed."
    }
    $service.WaitForStatus($DesiredState, [TimeSpan]::FromSeconds($WaitTimeoutSeconds))
    $service.Refresh()
    if ($service.Status.ToString() -ne $DesiredState) {
        throw "Service '$ServiceName' did not reach $DesiredState within $WaitTimeoutSeconds seconds. Current state: $($service.Status)"
    }
    return $service
}

function Wait-ServiceRemoved {
    $deadline = [DateTime]::UtcNow.AddSeconds($WaitTimeoutSeconds)
    do {
        if (-not (Get-ServiceObject)) { return }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Service '$ServiceName' was not removed within $WaitTimeoutSeconds seconds."
}

function Get-NssmSetting {
    param([string[]]$Arguments)
    $result = Invoke-Nssm -Arguments (@('get', $ServiceName) + $Arguments) -AllowFailure
    if ($result.ExitCode -ne 0) { return $null }
    return $result.Output.Trim()
}

function Assert-InstalledDefinition {
    param([Parameter(Mandatory = $true)]$Definition)
    $checks = @(
        @{ Name = 'Application'; Actual = Get-NssmSetting @('Application'); Expected = $Definition.Application },
        @{ Name = 'AppDirectory'; Actual = Get-NssmSetting @('AppDirectory'); Expected = $Definition.WorkingDirectory },
        @{ Name = 'AppParameters'; Actual = Get-NssmSetting @('AppParameters'); Expected = $Definition.ArgumentString },
        @{ Name = 'AppStdout'; Actual = Get-NssmSetting @('AppStdout'); Expected = $Definition.StdoutPath },
        @{ Name = 'AppStderr'; Actual = Get-NssmSetting @('AppStderr'); Expected = $Definition.StderrPath }
    )
    $mismatches = foreach ($check in $checks) {
        if ($check.Actual.Trim('"') -ne $check.Expected.Trim('"')) {
            "$($check.Name): expected '$($check.Expected)', found '$($check.Actual)'"
        }
    }
    if ($mismatches) {
        throw "Installed service verification failed:`n$($mismatches -join [Environment]::NewLine)"
    }
}

function Stop-ServiceInternal {
    $service = Get-ServiceObject
    if (-not $service) { throw "Service '$ServiceName' is not installed." }
    if ($service.Status -eq 'Stopped') { return $service }
    Stop-Service -Name $ServiceName
    return Wait-ServiceState -DesiredState 'Stopped'
}

function Remove-ServiceInternal {
    $service = Get-ServiceObject
    if (-not $service) { return }
    if ($service.Status -ne 'Stopped') {
        Stop-ServiceInternal | Out-Null
    }
    Invoke-Nssm -Arguments @('remove', $ServiceName, 'confirm') | Out-Null
    Wait-ServiceRemoved
}

function Install-PingMonitorService {
    Assert-Administrator
    $definition = Get-DesiredServiceDefinition
    $existing = Get-ServiceObject
    if ($existing -and -not $ForceReinstall) {
        throw "Service '$ServiceName' already exists. Use -ForceReinstall to replace its definition safely."
    }
    if (-not $PSCmdlet.ShouldProcess($ServiceName, 'Install and configure Windows service')) { return }
    if ($existing) {
        Remove-ServiceInternal
    }

    New-Item -ItemType Directory -Path $definition.LogDirectory -Force | Out-Null
    $script:NssmPath = Find-Nssm
    try {
        Invoke-Nssm -Arguments @('install', $ServiceName, $definition.Application, $definition.ArgumentString) | Out-Null
        $startValue = switch ($StartupType) {
            'AutomaticDelayedStart' { 'SERVICE_DELAYED_AUTO_START' }
            'Automatic' { 'SERVICE_AUTO_START' }
            'Manual' { 'SERVICE_DEMAND_START' }
        }
        $settings = @(
            @('DisplayName', $definition.DisplayName),
            @('Description', $definition.Description),
            @('AppDirectory', $definition.WorkingDirectory),
            @('Start', $startValue),
            @('AppExit', 'Default', 'Restart'),
            @('AppRestartDelay', '10000'),
            @('AppThrottle', '1500'),
            @('AppStopMethodSkip', '0'),
            @('AppStopMethodConsole', '15000'),
            @('AppStopMethodWindow', '1500'),
            @('AppStopMethodThreads', '1500'),
            @('AppStdout', $definition.StdoutPath),
            @('AppStderr', $definition.StderrPath),
            @('AppStdoutCreationDisposition', '4'),
            @('AppStderrCreationDisposition', '4'),
            @('AppRotateFiles', '1'),
            @('AppRotateOnline', '1'),
            @('AppRotateSeconds', '86400'),
            @('AppRotateBytes', '10485760')
        )
        foreach ($setting in $settings) {
            Invoke-Nssm -Arguments (@('set', $ServiceName) + $setting) | Out-Null
        }

        & sc.exe failure $ServiceName 'reset=' 86400 'actions=' 'restart/10000/restart/30000/restart/60000' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Unable to configure SCM recovery actions for '$ServiceName'." }
        & sc.exe failureflag $ServiceName 1 | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Unable to enable SCM recovery actions for '$ServiceName'." }

        Assert-InstalledDefinition -Definition $definition
        if (-not $NoStart) {
            Start-Service -Name $ServiceName
            Wait-ServiceState -DesiredState 'Running' | Out-Null
            Start-Sleep -Milliseconds 750
            $service = Get-ServiceObject
            $service.Refresh()
            if ($service.Status -ne 'Running') {
                throw "Service '$ServiceName' exited during startup stabilization. Review $($definition.StderrPath)."
            }
        }
    }
    catch {
        $installError = $_
        try { Remove-ServiceInternal } catch { Write-Warning "Rollback failed: $($_.Exception.Message)" }
        throw $installError
    }

    Write-Host "Service '$ServiceName' installed and verified." -ForegroundColor Green
    Write-Host "Application: $($definition.Application)"
    Write-Host "Working directory: $($definition.WorkingDirectory)"
    Write-Host "Configuration: $($definition.ConfigPath)"
    Write-Host "Endpoints: $($definition.EndpointsPath)"
    Write-Host "Logs: $($definition.LogDirectory)"
    Write-Host "Startup: $StartupType"
    Write-Host "State: $((Get-ServiceObject).Status)"
}

function Uninstall-PingMonitorService {
    Assert-Administrator
    if (-not (Get-ServiceObject)) {
        Write-Host "Service '$ServiceName' is not installed."
        return
    }
    if ($PSCmdlet.ShouldProcess($ServiceName, 'Stop and remove Windows service')) {
        Remove-ServiceInternal
        Write-Host "Service '$ServiceName' removed." -ForegroundColor Green
    }
}

function Start-PingMonitorService {
    Assert-Administrator
    $service = Get-ServiceObject
    if (-not $service) { throw "Service '$ServiceName' is not installed." }
    if ($service.Status -eq 'Running') { Write-Host "Service '$ServiceName' is already running."; return }
    if ($PSCmdlet.ShouldProcess($ServiceName, 'Start Windows service')) {
        Start-Service -Name $ServiceName
        Wait-ServiceState -DesiredState 'Running' | Out-Null
        Write-Host "Service '$ServiceName' is running." -ForegroundColor Green
    }
}

function Stop-PingMonitorService {
    Assert-Administrator
    if ($PSCmdlet.ShouldProcess($ServiceName, 'Stop Windows service')) {
        Stop-ServiceInternal | Out-Null
        Write-Host "Service '$ServiceName' is stopped." -ForegroundColor Green
    }
}

function Restart-PingMonitorService {
    Assert-Administrator
    $service = Get-ServiceObject
    if (-not $service) { throw "Service '$ServiceName' is not installed." }
    if ($PSCmdlet.ShouldProcess($ServiceName, 'Restart Windows service')) {
        if ($service.Status -ne 'Stopped') { Stop-ServiceInternal | Out-Null }
        Start-Service -Name $ServiceName
        Wait-ServiceState -DesiredState 'Running' | Out-Null
        Write-Host "Service '$ServiceName' restarted successfully." -ForegroundColor Green
    }
}

function Show-ServiceStatus {
    $service = Get-ServiceObject
    if (-not $service) {
        $result = [pscustomobject]@{ ServiceName = $ServiceName; Installed = $false; Status = 'NotInstalled' }
    }
    else {
        $cim = Get-CimInstance Win32_Service -Filter "Name='$ServiceName'"
        $script:NssmPath = try { Find-Nssm } catch { $null }
        $result = [pscustomobject]@{
            ServiceName       = $service.Name
            DisplayName       = $service.DisplayName
            Installed         = $true
            Status            = $service.Status.ToString()
            StartType         = $service.StartType.ToString()
            ProcessId         = $cim.ProcessId
            Application       = if ($script:NssmPath) { Get-NssmSetting @('Application') } else { $null }
            Arguments         = if ($script:NssmPath) { Get-NssmSetting @('AppParameters') } else { $null }
            WorkingDirectory  = if ($script:NssmPath) { Get-NssmSetting @('AppDirectory') } else { $null }
            StdoutPath        = if ($script:NssmPath) { Get-NssmSetting @('AppStdout') } else { $null }
            StderrPath        = if ($script:NssmPath) { Get-NssmSetting @('AppStderr') } else { $null }
            ExitAction        = if ($script:NssmPath) { Get-NssmSetting @('AppExit', 'Default') } else { $null }
        }
    }
    if ($Json) { $result | ConvertTo-Json -Depth 4; return }
    $result | Format-List
}

function Test-ServiceDefinition {
    $definition = Get-DesiredServiceDefinition
    $script:NssmPath = Find-Nssm
    $result = [pscustomobject]@{
        Valid             = $true
        NssmPath          = $script:NssmPath
        ServiceName       = $definition.ServiceName
        Runtime           = $definition.Runtime
        RuntimeVersion    = $definition.RuntimeVersion
        Application       = $definition.Application
        Arguments         = $definition.ArgumentString
        WorkingDirectory  = $definition.WorkingDirectory
        ConfigPath        = $definition.ConfigPath
        EndpointsPath     = $definition.EndpointsPath
        UIListen          = $definition.UIListen
        LogDirectory      = $definition.LogDirectory
        StartupType       = $definition.StartupType
    }
    if ($Json) { $result | ConvertTo-Json -Depth 4; return }
    Write-Host 'Service definition is valid.' -ForegroundColor Green
    $result | Format-List
}

switch ($PSCmdlet.ParameterSetName) {
    'Install' { Install-PingMonitorService }
    'Uninstall' { Uninstall-PingMonitorService }
    'Start' { Start-PingMonitorService }
    'Stop' { Stop-PingMonitorService }
    'Restart' { Restart-PingMonitorService }
    'Validate' { Test-ServiceDefinition }
    default { Show-ServiceStatus }
}
