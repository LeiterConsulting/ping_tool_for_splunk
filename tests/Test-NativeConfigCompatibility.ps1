#Requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [ValidateRange(5, 200)]
    [int]$Iterations = 25,

    [string]$ExpectedVersion = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$resolvedBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $tempRoot "PingMonitorNativeConfig-$([Guid]::NewGuid().ToString('N'))"
$originalPath = $env:PATH

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

function Invoke-Binary {
    param([string[]]$Arguments, [switch]$AllowFailure)
    $output = @(& $resolvedBinary @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "pingmonitor exited with $exitCode`n$($output -join [Environment]::NewLine)"
    }
    return [PSCustomObject]@{ ExitCode = $exitCode; Output = ($output -join [Environment]::NewLine) }
}

function Get-Percentile {
    param([double[]]$Values, [double]$Percentile)
    $sorted = @($Values | Sort-Object)
    $index = [Math]::Max(0, [Math]::Min($sorted.Count - 1, [Math]::Ceiling(($Percentile / 100) * $sorted.Count) - 1))
    return [Math]::Round($sorted[$index], 2)
}

try {
    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'config.psd1') -Destination (Join-Path $testRoot 'config.psd1')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'endpoints.csv') -Destination (Join-Path $testRoot 'endpoints.csv')

    # Keep core Windows commands available while making pwsh unavailable to the
    # child process. A successful PSD1 validation then proves the packaged
    # collector did not execute PowerShell to load its configuration.
    $env:PATH = "$env:SystemRoot\System32;$env:SystemRoot"
    $version = Invoke-Binary -Arguments @('--version')
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
        Assert-True ($version.Output -match [regex]::Escape($ExpectedVersion)) "runtime version should be $ExpectedVersion"
    }

    $arguments = @(
        '--validate',
        '--config', (Join-Path $testRoot 'config.psd1'),
        '--endpoints', (Join-Path $testRoot 'endpoints.csv')
    )
    $first = Invoke-Binary -Arguments $arguments
    Assert-True ($first.Output -match 'validation successful') 'existing PSD1 deployment should validate without pwsh'
    Assert-True ($first.Output -match 'config=config.psd1') 'advisor should identify PSD1 as the active source'

    $durations = [Collections.Generic.List[double]]::new()
    for ($iteration = 1; $iteration -le $Iterations; $iteration++) {
        $watch = [Diagnostics.Stopwatch]::StartNew()
        $result = Invoke-Binary -Arguments $arguments
        $watch.Stop()
        Assert-True ($result.Output -match 'validation successful') "PSD1 validation iteration $iteration should succeed"
        $durations.Add($watch.Elapsed.TotalMilliseconds)
    }

    $malformedPath = Join-Path $testRoot 'duplicate.psd1'
    [IO.File]::WriteAllText($malformedPath, "@{`r`n  timeout_ms = 1000`r`n  TIMEOUT_MS = 2000`r`n}`r`n", [Text.UTF8Encoding]::new($false))
    $malformed = Invoke-Binary -Arguments @('--validate', '--config', $malformedPath, '--endpoints', (Join-Path $testRoot 'endpoints.csv')) -AllowFailure
    Assert-True ($malformed.ExitCode -ne 0) 'duplicate case-insensitive PSD1 keys should fail validation'
    Assert-True ($malformed.Output -match 'duplicate key' -and $malformed.Output -match 'psd1:3:3') "PSD1 failure should identify the duplicate and source location; output: $($malformed.Output)"

    [PSCustomObject]@{
        Version          = $ExpectedVersion
        PowerShellInPath = $false
        Iterations       = $Iterations
        ValidationP50Ms  = Get-Percentile $durations.ToArray() 50
        ValidationP95Ms  = Get-Percentile $durations.ToArray() 95
        ValidationP99Ms  = Get-Percentile $durations.ToArray() 99
        DuplicateBlocked = $true
    } | ConvertTo-Json
}
finally {
    $env:PATH = $originalPath
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        [IO.Path]::GetFileName($resolvedTestRoot).StartsWith('PingMonitorNativeConfig-', [StringComparison]::Ordinal)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
