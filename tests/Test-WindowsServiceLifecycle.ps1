#Requires -Version 7.4
#Requires -RunAsAdministrator
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [string]$ServiceName = 'SplunkPingMonitorCanary',

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18081
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$installer = Join-Path $repoRoot 'Install-Service.ps1'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "PingMonitorServiceCanary-$([Guid]::NewGuid().ToString('N'))"

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

function Get-Status {
    return ((& $installer -Status -ServiceName $ServiceName -Json) | ConvertFrom-Json)
}

try {
    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    [IO.File]::WriteAllText((Join-Path $testRoot 'config.json'), @'
{
  "pings_per_cycle": 1,
  "cycle_interval_seconds": 2,
  "timeout_ms": 250,
  "parallel_threads": 1,
  "output_mode": "file",
  "log_path": "./logs/ping_results.log",
  "log_rotation_size_mb": 5,
  "emit_individual_pings": false,
  "ping": { "mode": "auto" },
  "diagnostics": { "enabled": true, "handle_probe_mode": "none" },
  "hec": { "enabled": false },
  "metrics": { "enabled": false }
}
'@)
    [IO.File]::WriteAllText((Join-Path $testRoot 'endpoints.csv'), "ip,hostname,endpoint_id,dev`r`n127.0.0.1,loopback,service-canary,false`r`n")

    & $installer -Install -ServiceName $ServiceName -BinaryPath $BinaryPath `
        -ConfigPath (Join-Path $testRoot 'config.json') -EndpointsPath (Join-Path $testRoot 'endpoints.csv') `
        -WorkingDirectory $testRoot -LogDirectory (Join-Path $testRoot 'logs') `
        -UIListen "127.0.0.1:$UIPort" -StartupType Manual -NoStart

    $installed = Get-Status
    Assert-True $installed.Installed 'service should be installed'
    Assert-True ($installed.Status -eq 'Stopped') 'service should initially be stopped'
    Assert-True ($installed.WorkingDirectory -eq $testRoot) 'NSSM should persist the working directory'

    & $installer -Start -ServiceName $ServiceName
    $running = Get-Status
    Assert-True ($running.Status -eq 'Running') 'service should start'
    Assert-True ($running.ProcessId -gt 0) 'running service should have a process ID'
    $firstPid = $running.ProcessId

    $health = Invoke-WebRequest -Uri "http://127.0.0.1:$UIPort/healthz" -TimeoutSec 10
    Assert-True ($health.StatusCode -eq 200 -and $health.Content -eq 'ok') 'service health endpoint should respond'

    & $installer -Restart -ServiceName $ServiceName
    $restarted = Get-Status
    Assert-True ($restarted.Status -eq 'Running') 'service should restart'
    Assert-True ($restarted.ProcessId -gt 0 -and $restarted.ProcessId -ne $firstPid) 'restart should replace the service process'

    & $installer -Stop -ServiceName $ServiceName
    $stopped = Get-Status
    Assert-True ($stopped.Status -eq 'Stopped') 'service should stop cleanly'
    Assert-True (Test-Path -LiteralPath (Join-Path $testRoot 'logs\ping_results.log')) 'the monitored cycle should write a result'

    & $installer -Uninstall -ServiceName $ServiceName
    $removed = Get-Status
    Assert-True (-not $removed.Installed) 'service should uninstall'
    Write-Host 'Windows service lifecycle integration test passed.' -ForegroundColor Green
}
finally {
    try {
        if ((Get-Status).Installed) {
            & $installer -Uninstall -ServiceName $ServiceName -Confirm:$false
        }
    }
    catch {
        Write-Warning "Canary service cleanup failed: $($_.Exception.Message)"
    }

    $resolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    $resolvedTest = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTest.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $resolvedTest) -like 'PingMonitorServiceCanary-*') {
        Remove-Item -LiteralPath $resolvedTest -Recurse -Force -ErrorAction SilentlyContinue
    }
}
