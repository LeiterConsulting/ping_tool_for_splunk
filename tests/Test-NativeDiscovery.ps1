#Requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18082,

    [string]$ExpectedVersion = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$resolvedBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $tempRoot "PingMonitorNativeDiscovery-$([Guid]::NewGuid().ToString('N'))"
$process = $null
$originalPath = $env:PATH

try {
    if (Get-NetTCPConnection -LocalPort $UIPort -State Listen -ErrorAction SilentlyContinue) {
        throw "TCP port $UIPort is already in use"
    }

    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'config.example.json') -Destination (Join-Path $testRoot 'config.json')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'endpoints.example.csv') -Destination (Join-Path $testRoot 'endpoints.csv')
    [IO.File]::WriteAllText(
        (Join-Path $testRoot 'DiscoverEndpoints.ps1'),
        "# Version: 2.5.2`r`nthrow 'No active network adapter with a default gateway found'`r`n",
        [Text.UTF8Encoding]::new($false)
    )

    $stdoutPath = Join-Path $testRoot 'runtime_stdout.log'
    $stderrPath = Join-Path $testRoot 'runtime_stderr.log'

    # Prove the default path does not need pwsh. System32 remains available so
    # the Go compatibility ping fallback can still run if native ICMP is unavailable.
    $env:PATH = "$env:SystemRoot\System32;$env:SystemRoot"
    $process = Start-Process `
        -FilePath $resolvedBinary `
        -ArgumentList @('-ui-only', '-ui-listen', "127.0.0.1:$UIPort", '-config', 'config.json', '-endpoints', 'endpoints.csv') `
        -WorkingDirectory $testRoot `
        -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutPath `
        -RedirectStandardError $stderrPath `
        -PassThru
    $env:PATH = $originalPath

    $status = $null
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        if ($process.HasExited) {
            $stderr = Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
            throw "Native discovery runtime exited before becoming ready: $stderr"
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
        throw "Native discovery runtime did not become ready on port $UIPort"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion) -and $status.version -ne $ExpectedVersion) {
        throw "Expected runtime $ExpectedVersion, got '$($status.version)'"
    }
    if ($status.discovery_engine -ne 'native_go') {
        throw "Expected native_go discovery, got '$($status.discovery_engine)'"
    }
    $scriptPathProperty = $status.PSObject.Properties['discovery_script_path']
    if ($null -ne $scriptPathProperty -and -not [string]::IsNullOrWhiteSpace([string]$scriptPathProperty.Value)) {
        throw "Default native discovery unexpectedly selected script '$($scriptPathProperty.Value)'"
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

    if (@($result.items).Count -lt 1) {
        throw 'Native discovery returned no loopback observations'
    }
    if ($result.evidence.engine -ne 'native_go' -or $result.evidence.hosts_probed -ne 2 -or $result.evidence.hosts_indeterminate -ne 0) {
        throw "Native discovery evidence was incomplete: $($result.evidence | ConvertTo-Json -Compress)"
    }
    if ([string]$result.logs -notmatch 'Discovery engine: native_go') {
        throw 'Discovery logs do not identify the native engine'
    }
    $firstItem = @($result.items)[0]
    if ([string]::IsNullOrWhiteSpace([string]$firstItem.discovery_probe_backend)) {
        throw 'Native discovery endpoint is missing probe backend evidence'
    }

    $streamResponse = Invoke-WebRequest `
        -Uri "http://127.0.0.1:$UIPort/api/discovery/stream" `
        -Method Post `
        -ContentType 'application/json' `
        -Body $body `
        -TimeoutSec 30
    $streamText = if ($streamResponse.Content -is [byte[]]) {
        [Text.Encoding]::UTF8.GetString($streamResponse.Content)
    }
    else {
        [string]$streamResponse.Content
    }
    $streamEvents = @($streamText -split "`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { $_ | ConvertFrom-Json })
    $complete = $streamEvents | Where-Object { $null -ne $_.PSObject.Properties['type'] -and $_.type -eq 'complete' } | Select-Object -Last 1
    $streamError = $streamEvents | Where-Object { $null -ne $_.PSObject.Properties['type'] -and $_.type -eq 'error' } | Select-Object -First 1
    if ($null -ne $streamError -or $null -eq $complete -or $complete.evidence.engine -ne 'native_go' -or @($complete.items).Count -lt 1) {
        throw "Native discovery stream did not complete truthfully: $($streamEvents | ConvertTo-Json -Depth 5 -Compress)"
    }

    Write-Host "Native discovery passed without pwsh: version=$($status.version), target=$($result.target), observations=$(@($result.items).Count), backend=$($firstItem.discovery_probe_backend)." -ForegroundColor Green
}
finally {
    $env:PATH = $originalPath
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }

    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        [IO.Path]::GetFileName($resolvedTestRoot).StartsWith('PingMonitorNativeDiscovery-', [StringComparison]::Ordinal)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
