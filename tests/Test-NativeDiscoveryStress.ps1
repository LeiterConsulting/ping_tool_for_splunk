#Requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,

    [ValidateRange(1024, 65535)]
    [int]$UIPort = 18092,

    [ValidateRange(10, 1000)]
    [int]$Iterations = 100,

    [ValidateRange(2, 10)]
    [int]$LargeIterations = 10,

    [string]$ExpectedVersion = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$resolvedBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $tempRoot "PingMonitorNativeDiscoveryStress-$([Guid]::NewGuid().ToString('N'))"
$process = $null
$client = $null
$originalPath = $env:PATH

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw "Assertion failed: $Message"
    }
}

function New-JsonContent {
    param([hashtable]$Payload)
    return [Net.Http.StringContent]::new(
        ($Payload | ConvertTo-Json -Compress),
        [Text.Encoding]::UTF8,
        'application/json'
    )
}

function Invoke-DiscoveryRequest {
    param([hashtable]$Payload)
    $content = New-JsonContent $Payload
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $response = $null
    try {
        $response = $client.PostAsync("http://127.0.0.1:$UIPort/api/discovery/run", $content).GetAwaiter().GetResult()
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) {
            throw "Discovery returned HTTP $([int]$response.StatusCode): $text"
        }
        return [PSCustomObject]@{
            Body      = $text | ConvertFrom-Json
            ElapsedMs = $watch.Elapsed.TotalMilliseconds
        }
    }
    finally {
        $watch.Stop()
        $content.Dispose()
        if ($null -ne $response) {
            $response.Dispose()
        }
    }
}

function Invoke-RawDiscoveryRequest {
    param([hashtable]$Payload)
    $content = New-JsonContent $Payload
    $response = $null
    try {
        $response = $client.PostAsync("http://127.0.0.1:$UIPort/api/discovery/run", $content).GetAwaiter().GetResult()
        return [PSCustomObject]@{
            StatusCode = [int]$response.StatusCode
            Content    = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        }
    }
    finally {
        $content.Dispose()
        if ($null -ne $response) {
            $response.Dispose()
        }
    }
}

function Get-Percentile {
    param([double[]]$Values, [double]$Percentile)
    $sorted = @($Values | Sort-Object)
    $index = [Math]::Max(0, [Math]::Min($sorted.Count - 1, [Math]::Ceiling(($Percentile / 100) * $sorted.Count) - 1))
    return [Math]::Round($sorted[$index], 2)
}

function Write-ResourceSnapshot {
    param([string]$Label, [int]$ProcessID)
    $snapshot = Get-Process -Id $ProcessID
    Write-Host ("Resources {0}: working_set_mb={1}, private_mb={2}, handles={3}, threads={4}" -f `
        $Label,
        [Math]::Round($snapshot.WorkingSet64 / 1MB, 2),
        [Math]::Round($snapshot.PrivateMemorySize64 / 1MB, 2),
        $snapshot.HandleCount,
        $snapshot.Threads.Count)
}

try {
    if (Get-NetTCPConnection -LocalPort $UIPort -State Listen -ErrorAction SilentlyContinue) {
        throw "TCP port $UIPort is already in use"
    }

    [IO.Directory]::CreateDirectory($testRoot) | Out-Null
    [IO.File]::WriteAllText((Join-Path $testRoot 'config.json'), @'
{
  "config_schema_version": 2,
  "monitoring": {
    "pings_per_cycle": 1,
    "cycle_interval_seconds": 60,
    "timeout_ms": 250,
    "parallel_threads": 1,
    "emit_individual_pings": false,
    "ping": { "mode": "auto" }
  },
  "logging": {
    "results": {
      "path": "./logs/ping_results.log",
      "max_size_mb": 5,
      "retention_files": 2,
      "retention_days": 1,
      "compress_rotated": false
    }
  },
  "outputs": {
    "mode": "file",
    "hec": { "enabled": false },
    "metrics": { "enabled": false },
    "delivery": {
      "spool_path": "./data/outbox",
      "max_spool_bytes": "32MB",
      "max_envelopes": 1000,
      "drain_max_envelopes": 25
    }
  },
  "discovery": {
    "history_path": "./data/discovery",
    "retention_scans": 25,
    "retention_days": 0,
    "schedules": []
  },
  "diagnostics": { "enabled": false, "handle_probe_mode": "none" },
  "debug": { "emit_memory_stats": false }
}
'@, [Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText(
        (Join-Path $testRoot 'endpoints.csv'),
        "ip,hostname,endpoint_id,dev`r`n127.0.0.1,loopback,stress-canary,false`r`n",
        [Text.UTF8Encoding]::new($false)
    )
    [IO.File]::WriteAllText(
        (Join-Path $testRoot 'DiscoverEndpoints.ps1'),
        "throw 'A stale adjacent discovery script must never run'`r`n",
        [Text.UTF8Encoding]::new($false)
    )

    $stdoutPath = Join-Path $testRoot 'runtime_stdout.log'
    $stderrPath = Join-Path $testRoot 'runtime_stderr.log'
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
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($process.HasExited) {
            $stderr = Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
            throw "Stress runtime exited before becoming ready: $stderr"
        }
        try {
            $status = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/status" -TimeoutSec 2
            break
        }
        catch {
            Start-Sleep -Milliseconds 100
        }
    }
    Assert-True ($null -ne $status) 'stress runtime should become ready'
    Assert-True ($status.discovery_engine -eq 'native_go') 'stress runtime should use native Go discovery'
    $scriptPathProperty = $status.PSObject.Properties['discovery_script_path']
    Assert-True ($null -eq $scriptPathProperty -or [string]::IsNullOrWhiteSpace([string]$scriptPathProperty.Value)) 'adjacent PowerShell must remain inactive'
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
        Assert-True ($status.version -eq $ExpectedVersion) "runtime version should be $ExpectedVersion"
    }

    $systemBaseline = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/system" -TimeoutSec 10
    Assert-True ($systemBaseline.resources.platform -eq 'windows') 'system evidence should identify the Windows sampler'
    Assert-True ($systemBaseline.resources.process.pid -eq $process.Id) 'system evidence should identify the collector process'
    Assert-True ($systemBaseline.resources.process.working_set_bytes -gt 0) 'system evidence should report observed process memory'
    Assert-True ($systemBaseline.resources.host.total_memory_bytes -gt 0) 'system evidence should report installed host memory'
    Assert-True ($systemBaseline.runtime.monitoring_workers.configured -eq 1) 'system evidence should report configured monitoring workers'
    Assert-True ($systemBaseline.capacity.discovery_probe_worker_maximum -eq 256) 'system evidence should publish the native probe safety ceiling'
    Assert-True ($systemBaseline.capacity.discovery_dns_worker_maximum -eq 128) 'system evidence should publish the native DNS safety ceiling'

    $client = [Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromMinutes(10)
    $smallPayload = @{
        target_network = '127.0.0.0'
        subnet_mask    = 30
        timeout_ms     = 100
        throttle_limit = 2
    }

    Write-Host 'Stress stage: warm-up and validation boundaries'
    $warmup = Invoke-DiscoveryRequest $smallPayload
    Assert-True ($warmup.Body.evidence.hosts_considered -eq 2 -and $warmup.Body.evidence.hosts_probed -eq 2) 'warm-up scan accounting should be complete'
    Assert-True ($warmup.Body.evidence.hosts_indeterminate -eq 0) 'warm-up scan should have no indeterminate probes'
    $baseline = Get-Process -Id $process.Id
    $baselineWorkingSet = $baseline.WorkingSet64
    $baselinePrivate = $baseline.PrivateMemorySize64
    $baselineHandles = $baseline.HandleCount
    $baselineThreads = $baseline.Threads.Count
    Write-ResourceSnapshot 'baseline' $process.Id

    $invalidCases = @(
        @{ Payload = @{ target_network = '127.0.0.0'; subnet_mask = 30; timeout_ms = 99; throttle_limit = 2 }; Pattern = 'timeout_ms' },
        @{ Payload = @{ target_network = '127.0.0.0'; subnet_mask = 30; timeout_ms = 60001; throttle_limit = 2 }; Pattern = 'timeout_ms' },
        @{ Payload = @{ target_network = '127.0.0.0'; subnet_mask = 30; timeout_ms = 100; throttle_limit = -1 }; Pattern = 'throttle_limit' },
        @{ Payload = @{ target_network = '127.0.0.0'; subnet_mask = 15; timeout_ms = 100; throttle_limit = 2 }; Pattern = 'subnet_mask' },
        @{ Payload = @{ target_network = '192.168.1'; subnet_mask = 24; timeout_ms = 100; throttle_limit = 2 }; Pattern = 'target_network' }
    )
    foreach ($case in $invalidCases) {
        $invalid = Invoke-RawDiscoveryRequest $case.Payload
        Assert-True ($invalid.StatusCode -eq 400) "invalid request should return HTTP 400, got $($invalid.StatusCode)"
        Assert-True ($invalid.Content -match $case.Pattern) "invalid request response should identify $($case.Pattern)"
    }

    $legacyHigh = Invoke-DiscoveryRequest @{
        target_network = '127.0.0.0'
        subnet_mask    = 30
        timeout_ms     = 100
        throttle_limit = 4097
    }
    Assert-True ($legacyHigh.Body.evidence.requested_concurrency -eq 4097) 'legacy high concurrency should remain valid and be reported'
    Assert-True ($legacyHigh.Body.evidence.probe_workers -eq 2) 'effective ICMP workers should be bounded by the two-host target'
    Assert-True ($legacyHigh.Body.evidence.dns_workers -le 2) 'effective DNS workers should be bounded by observed hosts'

    Write-Host "Stress stage: $LargeIterations real Windows ICMP /24 scans and DNS enrichment"
    $largePayload = @{
        target_network = '127.0.0.0'
        subnet_mask    = 24
        timeout_ms     = 100
        throttle_limit = 4096
    }
    $large = Invoke-DiscoveryRequest $largePayload
    $largeEvidence = $large.Body.evidence
    Assert-True ($largeEvidence.engine -eq 'native_go') 'large scan should report the native engine'
    Assert-True ($largeEvidence.hosts_considered -eq 254 -and $largeEvidence.hosts_probed -eq 254) 'large scan should account for all 254 /24 hosts'
    Assert-True ($largeEvidence.hosts_indeterminate -eq 0) 'large scan should contain no indeterminate probes'
    Assert-True (($largeEvidence.hosts_observed + $largeEvidence.hosts_not_observed) -eq 254) 'large scan outcomes should balance exactly'
    Assert-True (@($large.Body.items).Count -eq $largeEvidence.hosts_observed) 'large scan observations should match returned endpoints'
    Assert-True (@($largeEvidence.probe_backends).Count -gt 0) 'large scan should identify its probe backend'
    Assert-True ($largeEvidence.requested_concurrency -eq 4096) 'large scan should retain requested concurrency as evidence'
    Assert-True ($largeEvidence.probe_workers -eq 254) 'large /24 scan should use no more workers than usable hosts'
    Assert-True ($largeEvidence.dns_workers -le 128) 'large scan should enforce the DNS worker ceiling'
    $afterFirstLarge = Get-Process -Id $process.Id
    $firstLargeHandles = $afterFirstLarge.HandleCount
    $firstLargeThreads = $afterFirstLarge.Threads.Count
    $largeHandleSamples = [Collections.Generic.List[int]]::new()
    $largeThreadSamples = [Collections.Generic.List[int]]::new()
    $largeHandleSamples.Add($firstLargeHandles)
    $largeThreadSamples.Add($firstLargeThreads)
    Write-ResourceSnapshot 'after-first-large-scan' $process.Id

    for ($largeIteration = 2; $largeIteration -le $LargeIterations; $largeIteration++) {
        $largeRepeat = Invoke-DiscoveryRequest $largePayload
        $repeatEvidence = $largeRepeat.Body.evidence
        Assert-True ($repeatEvidence.hosts_probed -eq 254 -and $repeatEvidence.hosts_indeterminate -eq 0) "large scan $largeIteration should have complete evidence"
        Assert-True (($repeatEvidence.hosts_observed + $repeatEvidence.hosts_not_observed) -eq 254) "large scan $largeIteration outcomes should balance"
        Assert-True ($repeatEvidence.probe_workers -eq 254 -and $repeatEvidence.dns_workers -le 128) "large scan $largeIteration should preserve worker ceilings"
        $largeProcessSample = Get-Process -Id $process.Id
        $largeHandleSamples.Add($largeProcessSample.HandleCount)
        $largeThreadSamples.Add($largeProcessSample.Threads.Count)
    }
    Start-Sleep -Milliseconds 500
    $afterRepeatedLarge = Get-Process -Id $process.Id
    $steadyLargeHandleGrowth = $afterRepeatedLarge.HandleCount - $firstLargeHandles
    $steadyLargeThreadGrowth = $afterRepeatedLarge.Threads.Count - $firstLargeThreads
    $lastLargeHandleGrowth = $largeHandleSamples[$largeHandleSamples.Count - 1] - $largeHandleSamples[$largeHandleSamples.Count - 2]
    $lastLargeThreadGrowth = $largeThreadSamples[$largeThreadSamples.Count - 1] - $largeThreadSamples[$largeThreadSamples.Count - 2]
    Write-ResourceSnapshot 'after-repeated-large-scans' $process.Id
    Assert-True ($steadyLargeHandleGrowth -lt 1536) "repeated /24 scans should remain within the bounded Windows ICMP pool; post-first-scan handle growth was $steadyLargeHandleGrowth"
    Assert-True ($steadyLargeThreadGrowth -lt 256) "repeated /24 scans should remain within the bounded Windows ICMP pool; post-first-scan thread growth was $steadyLargeThreadGrowth"
    Assert-True ($lastLargeHandleGrowth -lt 128) "the last repeated /24 scan should not restart material handle growth; growth was $lastLargeHandleGrowth"
    Assert-True ($lastLargeThreadGrowth -lt 32) "the last repeated /24 scan should not restart material thread growth; growth was $lastLargeThreadGrowth"

    Write-Host "Stress stage: $Iterations sequential /30 discovery iterations"
    $durations = [Collections.Generic.List[double]]::new()
    $scanIDs = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    for ($iteration = 1; $iteration -le $Iterations; $iteration++) {
        $sample = Invoke-DiscoveryRequest $smallPayload
        $evidence = $sample.Body.evidence
        Assert-True ($evidence.hosts_considered -eq 2 -and $evidence.hosts_probed -eq 2) "soak iteration $iteration should account for both hosts"
        Assert-True ($evidence.hosts_indeterminate -eq 0) "soak iteration $iteration should not be indeterminate"
        Assert-True (($evidence.hosts_observed + $evidence.hosts_not_observed) -eq 2) "soak iteration $iteration outcomes should balance"
        Assert-True ($scanIDs.Add([string]$sample.Body.scan_id)) "soak iteration $iteration should produce a unique scan ID"
        $durations.Add($sample.ElapsedMs)
    }
    Write-ResourceSnapshot 'after-soak' $process.Id

    Write-Host 'Stress stage: concurrent-run exclusion, stream cancellation, and recovery'
    $longClient = [Net.Http.HttpClient]::new()
    $longCancellation = [Threading.CancellationTokenSource]::new()
    $longRequest = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Post, "http://127.0.0.1:$UIPort/api/discovery/stream")
    $longRequest.Content = New-JsonContent @{
        target_network = '127.0.0.0'
        subnet_mask    = 16
        timeout_ms     = 100
        throttle_limit = 1
    }
    $longResponse = $longClient.SendAsync(
        $longRequest,
        [Net.Http.HttpCompletionOption]::ResponseHeadersRead,
        $longCancellation.Token
    ).GetAwaiter().GetResult()
    Assert-True ($longResponse.IsSuccessStatusCode) 'long-running stream should start successfully'

    $activeSystem = $null
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $candidate = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/system" -TimeoutSec 5
        if ($candidate.runtime.discovery_probe_workers.active -gt 0) {
            $activeSystem = $candidate
            break
        }
        Start-Sleep -Milliseconds 50
    }
    Assert-True ($null -ne $activeSystem) 'system evidence should observe an active discovery worker during a scan'
    Assert-True ($activeSystem.runtime.discovery_probe_workers.configured -eq 1) 'system evidence should report the effective discovery worker count'
    Assert-True ($activeSystem.runtime.discovery_probe_workers.peak -ge 1) 'system evidence should preserve process-lifetime discovery peak'

    for ($attempt = 1; $attempt -le 10; $attempt++) {
        $collision = Invoke-RawDiscoveryRequest $smallPayload
        Assert-True ($collision.StatusCode -eq 400 -and $collision.Content -match 'another discovery run is already active') "concurrent run $attempt should be rejected explicitly"
    }

    $longCancellation.Cancel()
    $longResponse.Dispose()
    $longRequest.Dispose()
    $longCancellation.Dispose()
    $longClient.Dispose()

    $recovered = $false
    $recoveryDeadline = [DateTime]::UtcNow.AddSeconds(15)
    while ([DateTime]::UtcNow -lt $recoveryDeadline) {
        $recovery = Invoke-RawDiscoveryRequest $smallPayload
        if ($recovery.StatusCode -eq 200) {
            $recoveryBody = $recovery.Content | ConvertFrom-Json
            $recovered = $recoveryBody.evidence.hosts_probed -eq 2 -and $recoveryBody.evidence.hosts_indeterminate -eq 0
            break
        }
        Assert-True ($recovery.StatusCode -eq 400 -and $recovery.Content -match 'another discovery run is already active') 'recovery wait should fail only because cancellation is still draining'
        Start-Sleep -Milliseconds 100
    }
    Assert-True $recovered 'discovery should release its lock and recover after stream cancellation'
    $idleSystem = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/system" -TimeoutSec 10
    Assert-True ($idleSystem.runtime.discovery_probe_workers.active -eq 0) 'system evidence should return discovery workers to idle after cancellation and recovery'
    Write-ResourceSnapshot 'after-cancellation-recovery' $process.Id

    Write-Host 'Stress stage: retention and resource-growth verification'
    Start-Sleep -Milliseconds 750
    $after = Get-Process -Id $process.Id
    $workingSetGrowthMB = [Math]::Round(($after.WorkingSet64 - $baselineWorkingSet) / 1MB, 2)
    $privateGrowthMB = [Math]::Round(($after.PrivateMemorySize64 - $baselinePrivate) / 1MB, 2)
    $handleGrowth = $after.HandleCount - $baselineHandles
    $threadGrowth = $after.Threads.Count - $baselineThreads
    Write-Host ("Resource growth: working_set_mb={0}, private_mb={1}, handles={2}, threads={3}" -f $workingSetGrowthMB, $privateGrowthMB, $handleGrowth, $threadGrowth)
    Assert-True ($workingSetGrowthMB -lt 128) "working-set growth should stay below 128 MiB, got $workingSetGrowthMB MiB"
    Assert-True ($privateGrowthMB -lt 160) "private-memory growth should stay below 160 MiB, got $privateGrowthMB MiB"
    Assert-True ($handleGrowth -lt 2304) "bounded Windows ICMP handle growth should stay below 2304, got $handleGrowth"
    Assert-True ($threadGrowth -lt 300) "bounded Windows ICMP thread growth should stay below 300, got $threadGrowth"

    $scanFiles = @(Get-ChildItem -LiteralPath (Join-Path $testRoot 'data\discovery\scans') -Filter '*.json' -File)
    Assert-True ($scanFiles.Count -le 25) "retention should keep at most 25 scan files, got $($scanFiles.Count)"
    $history = Invoke-RestMethod -Uri "http://127.0.0.1:$UIPort/api/discovery/history?limit=200" -TimeoutSec 10
    Assert-True (@($history.scans).Count -le 25) 'history index should honor the 25-scan retention bound'

    [string]$stderrText = Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
    [string]$stdoutText = Get-Content -LiteralPath $stdoutPath -Raw -ErrorAction SilentlyContinue
    Assert-True ($stderrText -notmatch '(?im)panic|fatal|data race') 'stress runtime stderr should contain no panic, fatal error, or race report'
    Assert-True ($stdoutText -notmatch '(?im)panic|fatal|data race') 'stress runtime stdout should contain no panic, fatal error, or race report'
    Assert-True (-not $process.HasExited) 'stress runtime should remain alive after the soak'

    $summary = [PSCustomObject]@{
        Version                 = $status.version
        Engine                  = $status.discovery_engine
        Iterations              = $Iterations
        LargeIterations         = $LargeIterations
        SmallScanP50Ms          = Get-Percentile $durations.ToArray() 50
        SmallScanP95Ms          = Get-Percentile $durations.ToArray() 95
        SmallScanP99Ms          = Get-Percentile $durations.ToArray() 99
        LargeScanHosts          = $largeEvidence.hosts_probed
        LargeScanObserved       = $largeEvidence.hosts_observed
        LargeScanElapsedMs      = [Math]::Round($large.ElapsedMs, 2)
        ProbeBackends           = @($largeEvidence.probe_backends)
        CollisionRejections     = 10
        CancellationRecovered   = $recovered
        RetainedScans           = $scanFiles.Count
        WorkingSetGrowthMB      = $workingSetGrowthMB
        PrivateMemoryGrowthMB   = $privateGrowthMB
        HandleGrowth            = $handleGrowth
        ThreadGrowth            = $threadGrowth
        PostWarmHandleGrowth    = $steadyLargeHandleGrowth
        PostWarmThreadGrowth    = $steadyLargeThreadGrowth
        LastLargeHandleGrowth   = $lastLargeHandleGrowth
        LastLargeThreadGrowth   = $lastLargeThreadGrowth
        ObservedProbePeak       = $idleSystem.runtime.discovery_probe_workers.peak
        StressProcessResponsive = $after.Responding
    }
    $summary | ConvertTo-Json -Depth 4
}
finally {
    $env:PATH = $originalPath
    if ($null -ne $client) {
        $client.Dispose()
    }
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }

    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        [IO.Path]::GetFileName($resolvedTestRoot).StartsWith('PingMonitorNativeDiscoveryStress-', [StringComparison]::Ordinal)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
