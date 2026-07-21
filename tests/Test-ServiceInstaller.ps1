#Requires -Version 7.4
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$installer = Join-Path $repoRoot 'Install-Service.ps1'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "Ping Monitor Installer Test $([Guid]::NewGuid().ToString('N'))"

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Assertion failed: $Message" }
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
  "emit_individual_pings": false,
  "ping": { "mode": "auto" },
  "hec": { "enabled": false },
  "metrics": { "enabled": false }
}
'@)
    [IO.File]::WriteAllText((Join-Path $testRoot 'endpoints.csv'), "ip,hostname,endpoint_id,dev`r`n127.0.0.1,loopback,service-canary,false`r`n")

    $json = & $installer -Validate -ServiceName PingMonitorDefinitionTest -BinaryPath $BinaryPath `
        -ConfigPath (Join-Path $testRoot 'config.json') -EndpointsPath (Join-Path $testRoot 'endpoints.csv') `
        -WorkingDirectory $testRoot -LogDirectory (Join-Path $testRoot 'service logs') -UIListen '127.0.0.1:18081' -Json
    Assert-True ($LASTEXITCODE -eq 0) 'validation command should succeed'
    $definition = $json | ConvertFrom-Json
    Assert-True $definition.Valid 'definition should be valid'
    Assert-True ($definition.WorkingDirectory -eq $testRoot) 'working directory should be preserved'
    Assert-True ($definition.Arguments -match '".*Ping Monitor Installer Test.*config.json"') 'paths containing spaces should be quoted'
    Assert-True ($definition.StartupType -eq 'AutomaticDelayedStart') 'delayed automatic start should be the default'

    $defaultJson = & $installer -Validate -ServiceName PingMonitorDefinitionTest -BinaryPath $BinaryPath `
        -ConfigPath (Join-Path $testRoot 'config.json') -EndpointsPath (Join-Path $testRoot 'endpoints.csv') `
        -WorkingDirectory $testRoot -Json
    $defaultDefinition = $defaultJson | ConvertFrom-Json
    Assert-True ($defaultDefinition.UIListen -eq '0.0.0.0:8080') 'v5.7 service UI should default to 0.0.0.0:8080'

    $remoteJson = & $installer -Validate -ServiceName PingMonitorDefinitionTest -BinaryPath $BinaryPath `
        -ConfigPath (Join-Path $testRoot 'config.json') -EndpointsPath (Join-Path $testRoot 'endpoints.csv') `
        -WorkingDirectory $testRoot -UIListen '0.0.0.0:18081' -Json
    $remoteDefinition = $remoteJson | ConvertFrom-Json
    Assert-True ($remoteDefinition.UIListen -eq '0.0.0.0:18081') 'remote UI bind should be accepted without an opt-in switch'

    $missingService = (& $installer -Status -ServiceName "PingMonitorMissing$([Guid]::NewGuid().ToString('N'))" -Json) | ConvertFrom-Json
    Assert-True (-not $missingService.Installed) 'status should work without elevation for an absent service'
    Write-Host 'Service installer definition tests passed.' -ForegroundColor Green
}
finally {
    $resolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    $resolvedTest = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTest.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $resolvedTest) -like 'Ping Monitor Installer Test *') {
        Remove-Item -LiteralPath $resolvedTest -Recurse -Force -ErrorAction SilentlyContinue
    }
}
