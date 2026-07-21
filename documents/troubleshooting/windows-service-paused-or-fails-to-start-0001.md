# Windows Service Enters Paused State or Fails to Start

Document ID: Troubleshooting 0001

Applies to: Ping Monitor v5.7.x on Windows when launched by NSSM, another service wrapper, a manually created Windows service, or Task Scheduler

Default service name: `SplunkPingMonitor`

## What is the symptom?

The Ping Monitor executable starts successfully from an interactive PowerShell session, but the Windows service does not remain running. Common observations include:

- Task Manager or the Services console changes the service from **Stopped** to **Paused**.
- `Start-Service SplunkPingMonitor` reports that the service could not start.
- Running `./pingmonitor.exe` from the deployment directory appears to work.
- The Configuration Advisor recommends more workers, the configuration is updated, but the service still fails to start.

## What is the goal?

The goal is to confirm that the Windows service launches the intended Ping Monitor binary with the intended configuration, endpoint inventory, working directory, and UI address. The service should remain in the **Running** state, expose a healthy `/healthz` endpoint, and monitor the expected number of endpoints without repeatedly exiting.

## First determine how Ping Monitor is being launched

Do not assume that the installed service uses NSSM. Inspect the Windows service definition first:

```powershell
sc.exe qc SplunkPingMonitor

Get-CimInstance Win32_Service -Filter "Name='SplunkPingMonitor'" |
  Select-Object Name, State, StartMode, PathName
```

Interpret `BINARY_PATH_NAME` or `PathName` as follows:

- A path involving `nssm.exe`, or a service whose parameters can be read with `nssm get`, uses NSSM.
- A path pointing directly to `pingmonitor.exe` is a native SCM registration created with a command such as `New-Service` or `sc.exe create`.
- A path involving WinSW, `srvany.exe`, or another host uses that service wrapper. Its configuration and logs must be checked using that wrapper's documentation.
- A Scheduled Task is not a Windows service. It must be inspected in Task Scheduler or with `Get-ScheduledTask`; `Get-Service` and `Start-Service` do not control it.

The current Ping Monitor Go executable is a long-running console application. It does not implement the native Windows Service Control Manager dispatcher and must not be registered directly with `New-Service` or `sc.exe create`. A direct registration can fail with a service-control timeout even though the same executable runs correctly from PowerShell.

## Why does an NSSM-hosted service show Paused?

The shipped installer hosts Ping Monitor through NSSM and configures NSSM to restart Ping Monitor if the application exits and to wait 10 seconds between restart attempts.

NSSM reports the service as **Paused** while it is waiting for the next restart. Repeated short-lived startup failures increase this backoff period. For an NSSM-hosted installation, Paused therefore usually means that the Ping Monitor child process started and then exited; it does not normally mean that an operator manually paused monitoring.

Other wrappers may report failures differently. A direct SCM registration normally fails during service startup because Ping Monitor does not provide the native service-control handshake. Therefore, verify the hosting model before treating Paused as proof of NSSM restart backoff.

NSSM documents this restart-delay and throttling behavior in its [official usage guide](https://www.nssm.cc/usage).

An executable that works interactively can still fail as a service because the two launches may use different:

- executable paths or binary versions;
- command-line arguments;
- configuration or endpoint files;
- working directories and relative paths;
- Windows accounts and file permissions;
- UI ports or single-instance locks;
- environment variables.

The service may also still reference an older deployment directory after an upgrade. Increasing `parallel_threads` does not repair a stale service definition. A successful interactive launch also does not prove that the service is reading the same configuration file.

## Before troubleshooting

1. Open PowerShell using **Run as administrator**.
2. Change to the directory containing `pingmonitor.exe`, `Install-Service.ps1`, `config.psd1`, and `endpoints.csv`.
3. Stop any interactively launched Ping Monitor with `Ctrl+C` before starting the service. A second instance can conflict with the single-instance guard or UI port `8080`.

The examples below use this deployment directory:

```powershell
Set-Location 'C:\Ping Monitor\ping_tool_for_splunk-main'
```

Replace that path if Ping Monitor is installed elsewhere.

## Step 1: Identify the hosting model

Run the `sc.exe qc` and `Get-CimInstance Win32_Service` commands above and record the complete executable path.

If the service points directly to `pingmonitor.exe`, proceed to **Replace a direct native service registration** under Step 5. Do not spend time adjusting worker counts to repair the SCM handshake.

If the service uses NSSM, continue through every step below. For another service wrapper, use the validation commands in Step 2, then inspect that wrapper's persisted executable, arguments, working directory, account, and logs.

If Ping Monitor is a Scheduled Task, confirm its action, arguments, start-in directory, execution account, last-run result, and history in Task Scheduler. The NSSM-specific status and reinstall commands below do not apply to a Scheduled Task.

## Step 2: Confirm that the intended binary and files validate

Run the Configuration Advisor without changing the service:

```powershell
./Install-Service.ps1 -Validate `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv"
```

The result should report `READY` and `validation successful`. Resolve every blocker before continuing. Recommendations are advisory unless explicitly identified as startup blockers.

Confirm the executable version:

```powershell
./pingmonitor.exe --version
```

## Step 3: Inspect an NSSM service definition

For an NSSM-hosted service, capture the persisted definition:

```powershell
./Install-Service.ps1 -Status
./Install-Service.ps1 -Status -Json
```

Compare these fields with the current deployment:

- `Application` must identify the current `pingmonitor.exe`.
- `Arguments` must identify the intended `config.psd1` and `endpoints.csv`.
- `WorkingDirectory` must identify the current deployment directory.
- `StdoutPath` and `StderrPath` must identify accessible log files.
- `ExitAction` should be `Restart`.

If the executable, arguments, or working directory identifies an older folder, the NSSM service definition must be reinstalled.

For another wrapper, locate the equivalent application path, arguments, working directory, output paths, and restart policy in its configuration file or management interface.

## Step 4: Read the actual service failure

### NSSM installation

The NSSM service logs are more useful than the generic `Start-Service` error:

```powershell
$status = ./Install-Service.ps1 -Status -Json | ConvertFrom-Json

Get-Content -LiteralPath $status.StderrPath -Tail 100
Get-Content -LiteralPath $status.StdoutPath -Tail 100
```

Look for messages such as:

- configuration or endpoint validation failures;
- duplicate endpoint targets;
- scheduler capacity failures;
- missing files or access denied errors;
- an address or port already in use;
- a single-instance lock held by another process;
- HEC or output initialization failures.

Check for another running instance if the logs report a lock or port conflict:

```powershell
Get-CimInstance Win32_Process -Filter "Name='pingmonitor.exe'" |
  Select-Object ProcessId, ExecutablePath, CommandLine
```

### Direct native service registration

Capture the complete `Start-Service` error and query recent Service Control Manager events:

```powershell
try {
  Start-Service SplunkPingMonitor -ErrorAction Stop
}
catch {
  $_ | Format-List * -Force
}

Get-WinEvent -FilterHashtable @{
  LogName = 'System'
  ProviderName = 'Service Control Manager'
  StartTime = (Get-Date).AddHours(-1)
} | Where-Object Message -Match 'SplunkPingMonitor|Ping Monitor' |
  Select-Object TimeCreated, Id, LevelDisplayName, Message
```

A timeout or failure to connect to the service controller is expected when the console executable was registered directly. Replace that definition with a supported wrapper rather than repeatedly retrying it.

### Task Scheduler or another wrapper

For Task Scheduler, inspect **Last Run Result**, enable task history, and confirm the configured action and start-in directory. For another wrapper, use its own log and status facilities. In every case, compare the wrapper's command line with the command that succeeds interactively.

## Step 5: Choose the correct recovery action

### When the persisted definition is already correct

Use the shipped controller instead of Task Manager or raw `Start-Service`. It performs a clean stop/start, waits for stabilization, and includes recent logs if startup fails:

```powershell
./Install-Service.ps1 -Restart
```

If the service is stopped rather than paused, this is also available:

```powershell
./Install-Service.ps1 -Start
```

### When paths, arguments, or service settings are stale

Reinstall the service definition explicitly:

```powershell
./Install-Service.ps1 -Install `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv" `
  -ForceReinstall
```

`-ForceReinstall` removes and recreates the NSSM service definition. It does not delete the deployment configuration, endpoint inventory, or service logs. The installer then verifies the persisted paths and starts the service with a stabilization check.

Reinstallation is recommended after an upgrade when the binary or deployment directory changed. It is not inherently required when the binary was replaced in place and the persisted service definition still matches.

### Replace a direct native service registration

If `sc.exe qc` shows `pingmonitor.exe` directly in `BINARY_PATH_NAME`, replace the unsupported native registration with the shipped NSSM definition.

First record the existing definition and confirm the service name. Then run the installer from an elevated PowerShell session:

```powershell
./Install-Service.ps1 -Install `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv" `
  -ForceReinstall
```

The force-reinstall operation removes the existing Windows service registration and replaces it with the verified NSSM-hosted definition. It does not delete the deployment files.

If NSSM is intentionally not permitted, use Task Scheduler or an approved service wrapper and configure it with the same absolute binary, config, endpoint, and working-directory paths. Direct SCM registration is not supported by Ping Monitor v5.7.x.

### When Ping Monitor is a Scheduled Task

Update the task rather than reinstalling a Windows service. Its action should identify the current executable and use absolute arguments:

```text
Program/script: C:\Ping Monitor\ping_tool_for_splunk-main\pingmonitor.exe
Arguments: --config "C:\Ping Monitor\ping_tool_for_splunk-main\config.psd1" --endpoints "C:\Ping Monitor\ping_tool_for_splunk-main\endpoints.csv" --ui-listen 0.0.0.0:8080
Start in: C:\Ping Monitor\ping_tool_for_splunk-main
```

Configure the task to run whether the user is logged on or not, grant the execution account access to the deployment and log directories, and enable restart-on-failure according to local policy.

## Step 6: Verify the result

Check the service state and persisted definition:

```powershell
./Install-Service.ps1 -Status
Get-Service SplunkPingMonitor
```

For a Scheduled Task, replace those commands with `Get-ScheduledTask` and `Get-ScheduledTaskInfo` and confirm that the task remains active with a successful last-run result.

Verify the embedded health endpoint:

```powershell
Invoke-WebRequest 'http://127.0.0.1:8080/healthz' -UseBasicParsing
```

Verify runtime status:

```powershell
Invoke-RestMethod 'http://127.0.0.1:8080/api/status' |
  ConvertTo-Json -Depth 6
```

Successful recovery has all of these characteristics, adjusted for the selected hosting model:

- the Windows service remains **Running** beyond the startup stabilization window, or the Scheduled Task reports a successful active run;
- `/healthz` returns HTTP `200` with `ok`;
- `/api/status` reports the expected Ping Monitor version and `runtime.state` of `running`;
- `runtime.active_endpoints` matches the intended inventory;
- the service is no longer cycling through **Paused** restart backoff;
- stderr contains no new startup failure.

## What should be collected if it still fails?

Collect the following without deleting or rotating the logs. The first block applies to every Windows service definition:

```powershell
./pingmonitor.exe --version
sc.exe qc SplunkPingMonitor
Get-CimInstance Win32_Service -Filter "Name='SplunkPingMonitor'" |
  Select-Object Name, State, StartMode, PathName
```

For an NSSM-hosted installation, also collect:

```powershell
./Install-Service.ps1 -Status -Json
./Install-Service.ps1 -Validate `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv"

$status = ./Install-Service.ps1 -Status -Json | ConvertFrom-Json
Get-Content -LiteralPath $status.StderrPath -Tail 100
Get-Content -LiteralPath $status.StdoutPath -Tail 100
```

For another wrapper or Task Scheduler, collect its configuration, execution account, recent logs or history, and last exit result instead. Include the complete error returned by the relevant start/restart action. Do not include HEC tokens, passwords, or other secrets in a troubleshooting report.
