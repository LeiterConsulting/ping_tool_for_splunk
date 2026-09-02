#Requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18094,

    [ValidateRange(100, 10000)]
    [int]$Requests = 1000,

    [ValidateRange(2, 128)]
    [int]$Parallelism = 32,

    [ValidateRange(1, 60)]
    [int]$QuiescenceSeconds = 10,

    [string]$ExpectedVersion = '',

    [switch]$RaceInstrumented
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$resolvedBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $tempRoot "PingMonitorSystemStress-$([Guid]::NewGuid().ToString('N'))"
$process = $null
$client = $null

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

function Invoke-SystemAPIWave {
    param(
        [Net.Http.HttpClient]$Client,
        [string]$URL,
        [int]$RequestCount,
        [int]$WaveParallelism,
        [int]$ExpectedPID,
        [Collections.Generic.HashSet[string]]$ObservedSamples
    )

    $remaining = $RequestCount
    $watch = [Diagnostics.Stopwatch]::StartNew()
    while ($remaining -gt 0) {
        $batchSize = [Math]::Min($remaining, $WaveParallelism)
        $tasks = [Collections.Generic.List[Threading.Tasks.Task[Net.Http.HttpResponseMessage]]]::new()
        for ($index = 0; $index -lt $batchSize; $index++) {
            $tasks.Add($Client.GetAsync($URL))
        }
        [Threading.Tasks.Task]::WaitAll([Threading.Tasks.Task[]]$tasks.ToArray())
        foreach ($task in $tasks) {
            $response = $task.GetAwaiter().GetResult()
            try {
                Assert-True ($response.IsSuccessStatusCode) "system API should return success, got $([int]$response.StatusCode)"
                $payload = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
                Assert-True ($payload.resources.process.pid -eq $ExpectedPID) 'concurrent sample should retain process identity'
                [void]$ObservedSamples.Add([string]$payload.resources.observed_at)
            }
            finally {
                $response.Dispose()
            }
        }
        $remaining -= $batchSize
    }
    $watch.Stop()

    return [PSCustomObject]@{
        ElapsedMs = [Math]::Round($watch.Elapsed.TotalMilliseconds, 2)
        RequestsPerSecond = [Math]::Round($RequestCount / $watch.Elapsed.TotalSeconds, 2)
    }
}

try {
    if (Get-NetTCPConnection -LocalPort $UIPort -State Listen -ErrorAction SilentlyContinue) {
        throw "TCP port $UIPort is already in use"
    }
    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'config.psd1') -Destination (Join-Path $testRoot 'config.psd1')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'endpoints.csv') -Destination (Join-Path $testRoot 'endpoints.csv')
    $stdoutPath = Join-Path $testRoot 'stdout.log'
    $stderrPath = Join-Path $testRoot 'stderr.log'
    $process = Start-Process `
        -FilePath $resolvedBinary `
        -ArgumentList @('-ui-only', '-ui-listen', "127.0.0.1:$UIPort", '-config', 'config.psd1', '-endpoints', 'endpoints.csv') `
        -WorkingDirectory $testRoot `
        -WindowStyle Hidden `
        -RedirectStandardOutput $stdoutPath `
        -RedirectStandardError $stderrPath `
        -PassThru

    $status = $null
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($process.HasExited) {
            throw "System stress runtime exited before ready: $(Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue)"
        }
        try {
            $status = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/status" -TimeoutSec 2
            break
        }
        catch {
            Start-Sleep -Milliseconds 100
        }
    }
    Assert-True ($null -ne $status) 'system stress runtime should become ready'
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
        Assert-True ($status.version -eq $ExpectedVersion) "runtime version should be $ExpectedVersion"
    }

    # Allow the second cached sample to calculate CPU deltas.
    Start-Sleep -Milliseconds 2100
    $baselineEvidence = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/system" -TimeoutSec 5
    Assert-True ($baselineEvidence.resources.process.pid -eq $process.Id) 'system API should report the collector PID'
    Assert-True ($baselineEvidence.resources.process.cpu_host_capacity_percent -ne $null) 'second Windows sample should report collector CPU'
    Assert-True ($baselineEvidence.resources.host.cpu_percent -ne $null) 'second Windows sample should report host CPU'
    Assert-True ($baselineEvidence.resources.process.working_set_bytes -gt 0) 'system API should report process memory'
    Assert-True ($baselineEvidence.resources.host.total_memory_bytes -gt 0) 'system API should report host memory capacity'

    $baseline = Get-Process -Id $process.Id
    $baselineWorkingSet = $baseline.WorkingSet64
    $baselineHandles = $baseline.HandleCount
    $baselineThreads = $baseline.Threads.Count
    $baselineTCPConnections = @(Get-NetTCPConnection -OwningProcess $process.Id -ErrorAction SilentlyContinue).Count
    $client = [Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromSeconds(15)
    $url = "http://127.0.0.1:$UIPort/api/system"
    $observedSamples = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $firstWave = Invoke-SystemAPIWave -Client $client -URL $url -RequestCount $Requests `
        -WaveParallelism $Parallelism -ExpectedPID $process.Id -ObservedSamples $observedSamples

    $systemPage = $client.GetAsync("http://127.0.0.1:$UIPort/system").GetAwaiter().GetResult()
    try {
        $systemHTML = $systemPage.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        Assert-True ($systemPage.IsSuccessStatusCode -and $systemHTML -match 'id="system"') 'deep-linked System page should remain available after stress'
    }
    finally {
        $systemPage.Dispose()
    }

    $client.Dispose()
    $client = $null
    $coldWorkingSetLimitMB = if ($RaceInstrumented) { 128 } else { 64 }
    $coldHandleLimit = if ($RaceInstrumented) { 512 } else { 256 }
    $coldThreadLimit = if ($RaceInstrumented) { 96 } else { 64 }

    $immediate = Get-Process -Id $process.Id
    $immediateHandleGrowth = $immediate.HandleCount - $baselineHandles
    $immediateThreadGrowth = $immediate.Threads.Count - $baselineThreads
    $immediateTCPConnections = @(Get-NetTCPConnection -OwningProcess $process.Id -ErrorAction SilentlyContinue).Count
    $settleWatch = [Diagnostics.Stopwatch]::StartNew()
    do {
        Start-Sleep -Milliseconds 250
        $after = Get-Process -Id $process.Id
        $workingSetGrowthMB = [Math]::Round(($after.WorkingSet64 - $baselineWorkingSet) / 1MB, 2)
        $handleGrowth = $after.HandleCount - $baselineHandles
        $threadGrowth = $after.Threads.Count - $baselineThreads
        $tcpConnections = @(Get-NetTCPConnection -OwningProcess $process.Id -ErrorAction SilentlyContinue).Count
        $quiescent = $workingSetGrowthMB -lt $coldWorkingSetLimitMB -and
            $handleGrowth -lt $coldHandleLimit -and
            $threadGrowth -lt $coldThreadLimit
    } while (-not $quiescent -and $settleWatch.Elapsed.TotalSeconds -lt $QuiescenceSeconds)
    $settleWatch.Stop()

    Assert-True ($workingSetGrowthMB -lt $coldWorkingSetLimitMB) "cold system API stress should keep working-set growth below $coldWorkingSetLimitMB MiB, got $workingSetGrowthMB"
    Assert-True ($handleGrowth -lt $coldHandleLimit) "cold system API stress should settle handle growth below $coldHandleLimit within $QuiescenceSeconds seconds, got $handleGrowth with $tcpConnections process-owned TCP connections"
    Assert-True ($threadGrowth -lt $coldThreadLimit) "cold system API stress should settle thread growth below $coldThreadLimit within $QuiescenceSeconds seconds, got $threadGrowth"

    $warmBaselineWorkingSet = $after.WorkingSet64
    $warmBaselineHandles = $after.HandleCount
    $warmBaselineThreads = $after.Threads.Count
    $client = [Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromSeconds(15)
    $secondWave = Invoke-SystemAPIWave -Client $client -URL $url -RequestCount $Requests `
        -WaveParallelism $Parallelism -ExpectedPID $process.Id -ObservedSamples $observedSamples
    $client.Dispose()
    $client = $null

    $warmWorkingSetLimitMB = if ($RaceInstrumented) { 64 } else { 32 }
    $warmHandleLimit = if ($RaceInstrumented) { 128 } else { 64 }
    $warmThreadLimit = if ($RaceInstrumented) { 32 } else { 16 }
    $warmSettleWatch = [Diagnostics.Stopwatch]::StartNew()
    do {
        Start-Sleep -Milliseconds 250
        $after = Get-Process -Id $process.Id
        $warmWorkingSetGrowthMB = [Math]::Round(($after.WorkingSet64 - $warmBaselineWorkingSet) / 1MB, 2)
        $warmHandleGrowth = $after.HandleCount - $warmBaselineHandles
        $warmThreadGrowth = $after.Threads.Count - $warmBaselineThreads
        $warmTCPConnections = @(Get-NetTCPConnection -OwningProcess $process.Id -ErrorAction SilentlyContinue).Count
        $warmQuiescent = $warmWorkingSetGrowthMB -lt $warmWorkingSetLimitMB -and
            $warmHandleGrowth -lt $warmHandleLimit -and
            $warmThreadGrowth -lt $warmThreadLimit
    } while (-not $warmQuiescent -and $warmSettleWatch.Elapsed.TotalSeconds -lt $QuiescenceSeconds)
    $warmSettleWatch.Stop()

    Assert-True ($warmWorkingSetGrowthMB -lt $warmWorkingSetLimitMB) "warm system API stress should keep incremental working-set growth below $warmWorkingSetLimitMB MiB, got $warmWorkingSetGrowthMB"
    Assert-True ($warmHandleGrowth -lt $warmHandleLimit) "warm system API stress should settle incremental handle growth below $warmHandleLimit within $QuiescenceSeconds seconds, got $warmHandleGrowth with $warmTCPConnections process-owned TCP connections"
    Assert-True ($warmThreadGrowth -lt $warmThreadLimit) "warm system API stress should settle incremental thread growth below $warmThreadLimit within $QuiescenceSeconds seconds, got $warmThreadGrowth"
    Assert-True (-not $process.HasExited -and $after.Responding) 'collector should remain responsive after system API stress'

    [string]$stderrText = Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
    Assert-True ($stderrText -notmatch '(?im)panic|fatal|data race') 'system stress stderr should contain no panic, fatal error, or race report'

    [PSCustomObject]@{
        Version            = $status.version
        RaceInstrumented   = $RaceInstrumented.IsPresent
        RequestsPerWave    = $Requests
        TotalRequests      = $Requests * 2
        Parallelism        = $Parallelism
        FirstWaveElapsedMs = $firstWave.ElapsedMs
        FirstWaveRequestsPerSecond = $firstWave.RequestsPerSecond
        SecondWaveElapsedMs = $secondWave.ElapsedMs
        SecondWaveRequestsPerSecond = $secondWave.RequestsPerSecond
        CachedSampleCount  = $observedSamples.Count
        ColdWorkingSetGrowthMB = $workingSetGrowthMB
        WarmWorkingSetGrowthMB = $warmWorkingSetGrowthMB
        BaselineTCPConnections = $baselineTCPConnections
        ColdImmediateHandleGrowth = $immediateHandleGrowth
        ImmediateTCPConnections = $immediateTCPConnections
        ColdHandleGrowth   = $handleGrowth
        ColdTCPConnections = $tcpConnections
        ColdImmediateThreadGrowth = $immediateThreadGrowth
        ColdThreadGrowth   = $threadGrowth
        ColdQuiescenceMs   = [Math]::Round($settleWatch.Elapsed.TotalMilliseconds, 2)
        WarmHandleGrowth   = $warmHandleGrowth
        WarmTCPConnections = $warmTCPConnections
        WarmThreadGrowth   = $warmThreadGrowth
        WarmQuiescenceMs   = [Math]::Round($warmSettleWatch.Elapsed.TotalMilliseconds, 2)
        Responsive         = $after.Responding
    } | ConvertTo-Json
}
finally {
    if ($null -ne $client) { $client.Dispose() }
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        [IO.Path]::GetFileName($resolvedTestRoot).StartsWith('PingMonitorSystemStress-', [StringComparison]::Ordinal)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
