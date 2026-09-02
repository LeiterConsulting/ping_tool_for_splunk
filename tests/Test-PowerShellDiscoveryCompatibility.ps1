#Requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18086,

    [string]$ExpectedVersion = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$resolvedBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $tempRoot "PingMonitorPowerShellDiscovery-$([Guid]::NewGuid().ToString('N'))"
$process = $null

try {
    if (Get-NetTCPConnection -LocalPort $UIPort -State Listen -ErrorAction SilentlyContinue) {
        throw "TCP port $UIPort is already in use"
    }

    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'config.example.json') -Destination (Join-Path $testRoot 'config.json')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'endpoints.example.csv') -Destination (Join-Path $testRoot 'endpoints.csv')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'DiscoverEndpoints.ps1') -Destination (Join-Path $testRoot 'DiscoverEndpoints.ps1')

    $stdoutPath = Join-Path $testRoot 'runtime_stdout.log'
    $stderrPath = Join-Path $testRoot 'runtime_stderr.log'
    $process = Start-Process `
        -FilePath $resolvedBinary `
        -ArgumentList @('-ui-only', '-ui-listen', "127.0.0.1:$UIPort", '-config', 'config.json', '-endpoints', 'endpoints.csv', '-discovery-script', 'DiscoverEndpoints.ps1') `
        -WorkingDirectory $testRoot `
        -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutPath `
        -RedirectStandardError $stderrPath `
        -PassThru

    $status = $null
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        if ($process.HasExited) {
            $stderr = Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
            throw "PowerShell compatibility runtime exited before becoming ready: $stderr"
        }
        try {
            $status = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/status" -TimeoutSec 2
            break
        }
        catch {
            Start-Sleep -Milliseconds 250
        }
    }
    if ($null -eq $status) {
        throw "PowerShell compatibility runtime did not become ready on port $UIPort"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion) -and $status.version -ne $ExpectedVersion) {
        throw "Expected runtime $ExpectedVersion, got '$($status.version)'"
    }
    if ($status.discovery_engine -ne 'powershell_compat' -or -not $status.discovery_available) {
        throw "Expected available powershell_compat discovery, got $($status | ConvertTo-Json -Compress)"
    }

    $body = @{
        target_network = '127.0.0.0'
        subnet_mask    = 30
        timeout_ms     = 100
        throttle_limit = 2
    } | ConvertTo-Json
    $result = Invoke-RestMethod `
        -Uri "http://127.0.0.1:$UIPort/api/discovery/run" `
        -Method Post `
        -ContentType 'application/json' `
        -Body $body `
        -TimeoutSec 30

    if (@($result.items).Count -lt 1 -or $result.evidence.engine -ne 'powershell_compat') {
        throw "PowerShell compatibility discovery did not return expected evidence: $($result | ConvertTo-Json -Depth 4 -Compress)"
    }

    Write-Host "PowerShell discovery compatibility passed: version=$($status.version), observations=$(@($result.items).Count)." -ForegroundColor Green
}
finally {
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }

    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        [IO.Path]::GetFileName($resolvedTestRoot).StartsWith('PingMonitorPowerShellDiscovery-', [StringComparison]::Ordinal)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
