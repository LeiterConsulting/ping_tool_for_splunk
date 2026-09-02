#Requires -Version 7.4
#Requires -RunAsAdministrator
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [string]$ServiceName = 'SplunkPingMonitorNativeCanary',

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18082
)

$ErrorActionPreference = 'Stop'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "PingMonitorNativeServiceCanary-$([Guid]::NewGuid().ToString('N'))"
$resolvedBinary = [IO.Path]::GetFullPath($BinaryPath)

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

function Invoke-PingMonitor {
    param([string[]]$Arguments)
    $output = @(& $resolvedBinary @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) {
        throw "pingmonitor exited with $LASTEXITCODE`n$($output -join [Environment]::NewLine)"
    }
    return $output
}

function Invoke-PingMonitorExpectFailure {
    param([string[]]$Arguments, [string]$ExpectedPattern)
    $output = @(& $resolvedBinary @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) {
        throw "pingmonitor unexpectedly succeeded: $($Arguments -join ' ')"
    }
    $text = $output -join [Environment]::NewLine
    if ($text -notmatch $ExpectedPattern) {
        throw "pingmonitor failed without expected evidence '$ExpectedPattern'`n$text"
    }
    return $output
}

function Get-NativeStatus {
    $raw = Invoke-PingMonitor -Arguments @('service', 'status', '--name', $ServiceName, '--json')
    return ($raw -join [Environment]::NewLine) | ConvertFrom-Json
}

try {
    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    [IO.File]::WriteAllText((Join-Path $testRoot 'config.psd1'), @'
@{
    config_schema_version = 2
    pings_per_cycle = 1
    cycle_interval_seconds = 2
    timeout_ms = 250
    parallel_threads = 1
    emit_individual_pings = $false
    output_mode = 'file'
    log_path = './logs/ping_results.log'
    log_rotation_size_mb = 5
    log_retention_files = 2
    log_retention_days = 1
    log_compress_rotated = $false
    ping = @{ mode = 'auto' }
    health = @{
        down_after_failures = 1
        recovery_after_successes = 1
        stale_after_intervals = 2
    }
}
'@)
    [IO.File]::WriteAllText((Join-Path $testRoot 'endpoints.csv'), "ip,hostname,endpoint_id,dev`r`n127.0.0.1,loopback,native-service-canary,false`r`n")

    $common = @(
        '--name', $ServiceName,
        '--config', (Join-Path $testRoot 'config.psd1'),
        '--endpoints', (Join-Path $testRoot 'endpoints.csv'),
        '--ui-listen', "127.0.0.1:$UIPort",
        '--log-dir', (Join-Path $testRoot 'service-logs'),
        '--startup', 'manual'
    )
    Invoke-PingMonitor -Arguments (@('service', 'validate') + $common) | Out-Host
    Invoke-PingMonitor -Arguments (@('service', 'install') + $common + @('--no-start')) | Out-Host

    $installed = Get-NativeStatus
    Assert-True $installed.installed 'native service should be installed'
    Assert-True ($installed.host_model -eq 'native_go_v6') 'service should use the native Go SCM host'
    Assert-True ($installed.state -eq 'stopped') 'service should initially be stopped'

    Invoke-PingMonitorExpectFailure -Arguments (@('service', 'install') + $common + @('--no-start')) -ExpectedPattern 'already exists' | Out-Host
    $preserved = Get-NativeStatus
    Assert-True ($preserved.installed -and $preserved.state -eq 'stopped' -and $preserved.host_model -eq 'native_go_v6') 'a non-forced install should preserve the existing definition'

    Invoke-PingMonitor -Arguments @('service', 'start', '--name', $ServiceName) | Out-Host
    $running = Get-NativeStatus
    Assert-True ($running.state -eq 'running') 'native service should start'
    Assert-True ($running.process_id -gt 0) 'native service should report its process ID'
    $firstPID = $running.process_id

    $health = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$UIPort/healthz" -TimeoutSec 10
    Assert-True ($health.StatusCode -eq 200 -and $health.Content -eq 'ok') 'native service health endpoint should respond'

    Invoke-PingMonitor -Arguments @('service', 'restart', '--name', $ServiceName) | Out-Host
    $restarted = Get-NativeStatus
    Assert-True ($restarted.state -eq 'running') 'native service should restart'
    Assert-True ($restarted.process_id -gt 0 -and $restarted.process_id -ne $firstPID) 'restart should replace the service process'

    Invoke-PingMonitor -Arguments @('service', 'stop', '--name', $ServiceName) | Out-Host
    $stopped = Get-NativeStatus
    Assert-True ($stopped.state -eq 'stopped') 'native service should stop cleanly'
    Assert-True (Test-Path -LiteralPath (Join-Path $testRoot 'service-logs\service.log')) 'native service should retain a bounded host log'

    Invoke-PingMonitor -Arguments (@('service', 'install') + $common + @('--force', '--no-start')) | Out-Host
    $replaced = Get-NativeStatus
    Assert-True ($replaced.installed -and $replaced.state -eq 'stopped' -and $replaced.host_model -eq 'native_go_v6') 'forced replacement should complete without a service marked-for-deletion timeout'
    Invoke-PingMonitor -Arguments @('service', 'start', '--name', $ServiceName) | Out-Host
    $replacementHealth = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$UIPort/healthz" -TimeoutSec 10
    Assert-True ($replacementHealth.StatusCode -eq 200 -and $replacementHealth.Content -eq 'ok') 'replacement service should start and become healthy'
    Invoke-PingMonitor -Arguments @('service', 'stop', '--name', $ServiceName) | Out-Host

    Invoke-PingMonitor -Arguments @('service', 'uninstall', '--name', $ServiceName) | Out-Host
    $removed = Get-NativeStatus
    Assert-True (-not $removed.installed) 'native service should uninstall'
    Write-Host 'Native Windows service lifecycle integration test passed.' -ForegroundColor Green
}
finally {
    try {
        if ((Get-NativeStatus).installed) {
            Invoke-PingMonitor -Arguments @('service', 'uninstall', '--name', $ServiceName) | Out-Null
        }
    }
    catch {
        Write-Warning "Native canary service cleanup failed: $($_.Exception.Message)"
    }

    $resolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    $resolvedTest = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTest.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $resolvedTest) -like 'PingMonitorNativeServiceCanary-*') {
        Remove-Item -LiteralPath $resolvedTest -Recurse -Force -ErrorAction SilentlyContinue
    }
}
