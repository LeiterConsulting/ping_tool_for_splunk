# Validate and Cut Over Ping Monitor v6 to a Native Windows Service

Document ID: Troubleshooting 0005

Applies to: Ping Monitor v6 on Windows when a validated foreground collector will be replaced by a native Windows Service Control Manager instance

Default service name: `SplunkPingMonitor`

## What is the goal?

The goal is to prove the native Windows service lifecycle in isolation, validate the production configuration, stop only the expected foreground collector, install the production service, and verify both service health and monitoring signal before declaring the cutover complete.

This procedure keeps the working foreground collector online while the isolated canary runs. Production is interrupted only after the canary and configuration gates pass.

This is a clean-install procedure. If `SplunkPingMonitor` already exists, use [Install or Migrate the Native v6 Windows Service](native-windows-service-v6-migration-0004.md) instead. Do not use `--force` until the existing service host and rollback requirements have been identified.

## Prerequisites

- A Windows x64 Ping Monitor v6 executable from the intended release.
- The production configuration and endpoint inventory already tested in foreground mode.
- A repository or source package containing `tests\Test-NativeWindowsServiceLifecycle.ps1`.
- PowerShell 7.4 or newer for the lifecycle test harness.
- A PowerShell or Windows Terminal session opened with **Run as administrator**.
- Port `18082` available for the isolated canary and the intended production UI port available after the foreground collector stops.
- A rollback copy of the previous executable and a record of the previous launch command.

Examples below use `C:\Ping Monitor` as the deployment directory and `C:\Source\ping_tool_for_splunk` as the repository directory. Replace them with the actual local paths.

## Step 1: Define and inspect the intended deployment

Run the following in the elevated PowerShell 7.4 session:

```powershell
$DeploymentPath = 'C:\Ping Monitor'
$RepositoryPath = 'C:\Source\ping_tool_for_splunk'
$BinaryPath = Join-Path $DeploymentPath 'pingmonitor.exe'
$ConfigPath = Join-Path $DeploymentPath 'config.psd1'
$EndpointsPath = Join-Path $DeploymentPath 'endpoints.csv'
$UIListen = '0.0.0.0:8080'

& $BinaryPath --version
Get-FileHash -Algorithm SHA256 -LiteralPath $BinaryPath
& $BinaryPath service status --json
```

For a clean installation, status should report `installed: false`. If it reports an installed service, stop this procedure and follow Troubleshooting 0004.

Record the executable version and SHA-256 hash with the change record. Do not continue if the version is not the intended release.

## Step 2: Validate the production inputs

This validation is read-only:

```powershell
& $BinaryPath service validate `
  --config $ConfigPath `
  --endpoints $EndpointsPath `
  --ui-listen $UIListen

if ($LASTEXITCODE -ne 0) {
    throw 'Production service validation failed. No service changes were made.'
}
```

Resolve every Configuration Advisor blocker before proceeding. Warnings and opportunities require review but are not startup blockers unless the output explicitly says otherwise.

## Step 3: Exercise the isolated SCM lifecycle

The lifecycle test creates a temporary `SplunkPingMonitorNativeCanary` service using a loopback endpoint and port `18082`. It does not use the production configuration, endpoint inventory, service name, or UI port.

```powershell
Set-Location $RepositoryPath

.\tests\Test-NativeWindowsServiceLifecycle.ps1 `
  -BinaryPath $BinaryPath
```

The canary verifies:

- native service validation and installation;
- preservation of an existing definition when `--force` is absent;
- start and HTTP readiness;
- restart with a new process ID;
- graceful stop and bounded service-log creation;
- intentional forced replacement;
- start after replacement; and
- uninstall and guarded temporary-file cleanup.

The message below is expected during the non-forced preservation test:

```text
service install failed: service already exists
```

It is not a canary failure when the test continues. The required final result is:

```text
Native Windows service lifecycle integration test passed.
```

Confirm cleanup:

```powershell
$canary = Get-Service -Name 'SplunkPingMonitorNativeCanary' -ErrorAction SilentlyContinue
if ($null -ne $canary) {
    throw 'The canary service remains installed. Stop before production cutover.'
}
```

Do not continue if the test terminates before the final success message or the canary remains installed. Preserve the complete output for troubleshooting.

## Step 4: Identify and stop the foreground collector safely

Do not stop a process solely because it owns the expected port. Confirm its executable first:

```powershell
$ProductionPort = 8080
$listeners = @(Get-NetTCPConnection -LocalPort $ProductionPort -State Listen -ErrorAction SilentlyContinue)
$ownerPIDs = @($listeners | Select-Object -ExpandProperty OwningProcess -Unique)

if ($ownerPIDs.Count -ne 1) {
    throw "Expected one process to own port $ProductionPort; found $($ownerPIDs.Count)."
}

$owner = Get-CimInstance Win32_Process -Filter "ProcessId = $($ownerPIDs[0])"
if ($null -eq $owner -or [string]::IsNullOrWhiteSpace($owner.ExecutablePath)) {
    throw 'The listener owner could not be identified. Stop and investigate.'
}

$owner | Select-Object ProcessId, ExecutablePath, CommandLine

$expectedBinary = [IO.Path]::GetFullPath($BinaryPath)
$actualBinary = [IO.Path]::GetFullPath($owner.ExecutablePath)
if (-not $actualBinary.Equals($expectedBinary, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'The production UI port is owned by an unexpected executable. Stop and investigate.'
}

Stop-Process -Id $owner.ProcessId
Start-Sleep -Seconds 2

if (Get-NetTCPConnection -LocalPort $ProductionPort -State Listen -ErrorAction SilentlyContinue) {
    throw "Port $ProductionPort is still occupied. Do not install the service."
}
```

This is the production interruption boundary. If the service cannot be installed, restore the previously approved foreground launch while the failure is investigated.

## Step 5: Validate again and install the production service

Validation is deliberately repeated after the foreground process stops so no changed file or unexpected port owner crosses the cutover boundary.

```powershell
Set-Location $DeploymentPath

& $BinaryPath service validate `
  --config $ConfigPath `
  --endpoints $EndpointsPath `
  --ui-listen $UIListen

if ($LASTEXITCODE -ne 0) {
    throw 'Final production validation failed. The service was not installed.'
}

& $BinaryPath service install `
  --config $ConfigPath `
  --endpoints $EndpointsPath `
  --ui-listen $UIListen `
  --startup delayed-auto

if ($LASTEXITCODE -ne 0) {
    throw 'Production service installation failed.'
}
```

Do not add `--force` to work around an unexpected existing-service error. Re-enter the identification and migration procedure instead.

The installer records absolute config and endpoint paths, configures the native `service-run` dispatcher, waits for readiness, enables delayed automatic startup, applies SCM recovery actions, and writes a bounded `logs\service.log` by default.

## Step 6: Verify the installed definition and health endpoint

```powershell
$nativeStatus = & $BinaryPath service status --json | ConvertFrom-Json
$windowsService = Get-CimInstance Win32_Service -Filter "Name='SplunkPingMonitor'"
$health = Invoke-WebRequest 'http://127.0.0.1:8080/healthz' -UseBasicParsing

$nativeStatus
$windowsService | Select-Object Name, DisplayName, State, StartMode, ProcessId, PathName, ExitCode
$health | Select-Object StatusCode, StatusDescription, Content
```

Required evidence:

- `installed` is `true`;
- `host_model` is `native_go_v6`;
- service state is `running`;
- delayed automatic startup is enabled unless another approved startup policy was selected;
- the persisted command contains `service-run` and the intended absolute paths;
- the Windows service PID matches the process listening on port `8080`; and
- `/healthz` returns HTTP 200 with content `ok`.

An HTTP 200 health response proves that the web and collector process reached readiness. It does not by itself prove that a complete monitoring cycle or downstream delivery succeeded.

## Step 7: Verify one complete monitoring cycle and signal delivery

Wait for at least one configured observation interval. The following loop allows up to three minutes:

```powershell
$deadline = [DateTime]::UtcNow.AddMinutes(3)

do {
    $status = Invoke-RestMethod 'http://127.0.0.1:8080/api/status'
    if ($null -ne $status.runtime.last_cycle_completed_at) { break }
    Start-Sleep -Seconds 2
} while ([DateTime]::UtcNow -lt $deadline)

if ($null -eq $status.runtime.last_cycle_completed_at) {
    throw 'No complete monitoring cycle was observed before the verification deadline.'
}

$system = Invoke-RestMethod 'http://127.0.0.1:8080/api/system'

[pscustomobject]@{
    Version = $status.version
    RuntimeState = $status.runtime.state
    ActiveEndpoints = $status.runtime.active_endpoints
    LastCycleCompleted = $status.runtime.last_cycle_completed_at
    LastCycleDurationMs = $status.runtime.last_cycle_duration_ms
    Success = $status.runtime.last_production_success
    Partial = $status.runtime.last_production_partial
    Failed = $status.runtime.last_production_failed
    DeliveryState = $status.delivery.state
    PendingEnvelopes = $status.delivery.pending_envelopes
    ServicePID = $system.resources.process.pid
    ConfiguredWorkers = $system.runtime.monitoring_workers.configured
    PeakWorkers = $system.runtime.monitoring_workers.peak
}
```

Confirm that:

- runtime state remains `running`;
- the active endpoint count matches the intended inventory;
- a cycle completed without an overrun or fatal runtime error;
- success, partial, and failed counts total the active endpoint count;
- HEC deployments report healthy delivery and an acceptable pending queue;
- file-output deployments show current bounded result-log status; and
- unavailable resource counters are reported as unavailable rather than as fabricated zeros.

Success, partial, and failed endpoint counts are monitoring observations, not service errors. Investigate them as signal only when they disagree with known endpoint behavior or evidence quality.

Review the bounded service log:

```powershell
Get-Content -LiteralPath (Join-Path $DeploymentPath 'logs\service.log') -Tail 200

Select-String `
  -LiteralPath (Join-Path $DeploymentPath 'logs\service.log') `
  -Pattern 'panic|fatal|data race|run failed|service host failed' `
  -CaseSensitive:$false
```

No output from `Select-String` is the expected result.

## Step 8: Verify restart handling during a maintenance window

The isolated canary already proves restart behavior for the executable and host. When production policy permits an additional brief interruption, verify the installed definition as well:

```powershell
$before = (& $BinaryPath service status --json | ConvertFrom-Json).process_id
& $BinaryPath service restart

if ($LASTEXITCODE -ne 0) {
    throw 'Production service restart failed.'
}

$afterStatus = & $BinaryPath service status --json | ConvertFrom-Json
$after = $afterStatus.process_id

if ($afterStatus.state -ne 'running') {
    throw 'Service did not return to Running after restart.'
}

if ($before -eq $after) {
    throw 'Service PID did not change after restart.'
}

Invoke-WebRequest 'http://127.0.0.1:8080/healthz' -UseBasicParsing
```

After restart, repeat the complete-cycle and delivery checks from Step 7. Process-local uptime, peaks, and restart-sensitive counters are expected to reset.

## Routine service control

Run lifecycle mutations from an elevated terminal:

```powershell
& $BinaryPath service status --json
& $BinaryPath service restart
& $BinaryPath service stop
& $BinaryPath service start
```

Use the web interface only after `/healthz` returns `ok`. Do not run a second foreground collector against the same config, endpoints, state, logs, or UI port while the service is active.

## Rollback and stop conditions

Stop and roll back if any of the following occurs:

- the canary does not end with its explicit success message;
- production validation reports a blocker;
- the UI port is owned by an unexpected executable;
- installation reports an unexpected existing service;
- the installed host model is not `native_go_v6`;
- the service cannot remain Running or `/healthz` does not return `ok`;
- the intended config or endpoint revisions are not active;
- a complete monitoring cycle cannot finish; or
- configured output develops an unexplained backlog or persistent failure.

Remove only the failed native SCM definition:

```powershell
& $BinaryPath service stop
& $BinaryPath service uninstall
```

Then restore the previous approved executable and launch method using the paths and command recorded before cutover. Do not improvise a raw `New-Service` or `sc.exe create` definition.

## What should be collected if the procedure fails?

Collect the following after removing credentials, HEC tokens, authorization headers, passwords, and private keys:

```powershell
& $BinaryPath --version
Get-FileHash -Algorithm SHA256 -LiteralPath $BinaryPath
& $BinaryPath service status --json
& $BinaryPath service validate --config $ConfigPath --endpoints $EndpointsPath --ui-listen $UIListen
sc.exe qc SplunkPingMonitor
Get-CimInstance Win32_Service -Filter "Name='SplunkPingMonitor'" |
  Select-Object Name, State, StartMode, ProcessId, PathName, ExitCode
Get-Content -LiteralPath (Join-Path $DeploymentPath 'logs\service.log') -Tail 200
```

Also include the complete canary output, exact failing command, `/api/status`, `/api/system`, relevant Service Control Manager events, and the expected endpoint count. Do not include the contents of `.env` or any credential-bearing configuration fields.
