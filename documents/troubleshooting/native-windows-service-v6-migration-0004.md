# Install or Migrate the Native v6 Windows Service

Document ID: Troubleshooting 0004

Applies to: Ping Monitor v6 on Windows, including clean installations and intentional migration from an existing NSSM-hosted v5 service

Default service name: `SplunkPingMonitor`

## What is the goal?

The goal is to run Ping Monitor as a native Windows Service Control Manager service using the same v6 executable that runs the collector. The resulting service must retain the intended config and endpoint paths, report Running only after collector readiness, respond to Stop and Restart, preserve bounded service logs, and leave existing deployment data untouched.

## What changed in v6?

Version 6 adds a native SCM dispatcher and lifecycle commands to `pingmonitor.exe`. NSSM and `Install-Service.ps1` are no longer required for the default Go runtime.

Do not create the service manually with `New-Service` or `sc.exe create`. A correct native definition requires the internal `service-run` arguments, absolute deployment paths, service-log location, readiness handling, and recovery policy that `pingmonitor.exe service install` configures together.

Existing v5 NSSM services remain valid. Migration is explicit; installing the v6 executable does not silently replace a service definition.

## Step 1: Record the existing state

Run these commands before changing service state:

```powershell
.\pingmonitor.exe --version
.\pingmonitor.exe service status --json
sc.exe qc SplunkPingMonitor
Get-CimInstance Win32_Service -Filter "Name='SplunkPingMonitor'" |
  Select-Object Name, State, StartMode, PathName
```

Interpret `host_model` as follows:

- `native_go_v6`: the service already uses the native v6 host;
- `nssm`: the service uses the supported legacy v5 wrapper;
- `direct_legacy_or_unknown`: `pingmonitor.exe` is registered without a recognizable v6 service command and should be replaced;
- `other_wrapper`: inspect that wrapper before replacing it.

For an NSSM service, also save its application, arguments, working directory, and output paths so rollback remains possible:

```powershell
nssm get SplunkPingMonitor Application
nssm get SplunkPingMonitor AppParameters
nssm get SplunkPingMonitor AppDirectory
nssm get SplunkPingMonitor AppStdout
nssm get SplunkPingMonitor AppStderr
```

## Step 2: Validate the intended v6 deployment

Validation is read-only and does not require elevation:

```powershell
.\pingmonitor.exe service validate `
  --config "$PWD\config.psd1" `
  --endpoints "$PWD\endpoints.csv" `
  --ui-listen 0.0.0.0:8080
```

Use the actual active config path when JSON or YAML is authoritative. Resolve every Configuration Advisor blocker before proceeding. Recommendations are not startup blockers unless the output labels them as such.

## Step 3: Perform a clean installation

For a service name that is not installed, open PowerShell or Windows Terminal with **Run as administrator** and run:

```powershell
.\pingmonitor.exe service install `
  --config "$PWD\config.psd1" `
  --endpoints "$PWD\endpoints.csv" `
  --ui-listen 0.0.0.0:8080
```

The default is delayed automatic startup. Use `--startup auto` or `--startup manual` when required. Use `--disable-ui` for a headless service; monitoring and scheduled discovery continue. Use `--no-start` to create and inspect the definition before starting it.

## Step 4: Migrate an existing definition

Only use `--force` after Step 1 identifies the existing host and Step 2 validates the replacement arguments.

```powershell
.\pingmonitor.exe service stop

.\pingmonitor.exe service install `
  --config "$PWD\config.psd1" `
  --endpoints "$PWD\endpoints.csv" `
  --ui-listen 0.0.0.0:8080 `
  --force
```

The force operation stops and replaces the SCM definition. It does not delete configuration, endpoints, identity, health state, discovery history, review data, appearance preferences, result logs, or service logs.

If the existing definition uses an approved wrapper other than NSSM, follow that wrapper's shutdown and rollback requirements before replacing it.

## Step 5: Verify service and signal health

```powershell
.\pingmonitor.exe service status --json
Invoke-WebRequest 'http://127.0.0.1:8080/healthz' -UseBasicParsing
Invoke-RestMethod 'http://127.0.0.1:8080/api/status' | ConvertTo-Json -Depth 8
Invoke-RestMethod 'http://127.0.0.1:8080/api/system' | ConvertTo-Json -Depth 8
```

Confirm all of the following:

- `installed` is true, `host_model` is `native_go_v6`, and state is `running`;
- the persisted binary command contains `service-run` and the intended absolute paths;
- `/healthz` returns HTTP 200 with `ok`;
- `/api/status` reports the expected v6 version, endpoint count, and running runtime state;
- `/api/system` identifies the service PID and reports worker capacity without unavailable values masquerading as zeros;
- a complete monitoring cycle is emitted and the delivery backlog remains within policy; and
- the bounded `logs\service.log` contains no startup failure.

Exercise control handling once during a maintenance window:

```powershell
$before = (.\pingmonitor.exe service status --json | ConvertFrom-Json).process_id
.\pingmonitor.exe service restart
$after = (.\pingmonitor.exe service status --json | ConvertFrom-Json).process_id
if ($before -eq $after) { throw 'Service PID did not change after restart.' }
```

## Rollback boundary

Rollback if the native service cannot remain Running, health does not return, the intended files are not active, or monitoring/delivery evidence is incomplete. Stop and uninstall the native definition:

```powershell
.\pingmonitor.exe service stop
.\pingmonitor.exe service uninstall
```

Then recreate the previously recorded NSSM or wrapper definition using its approved controller and the exact paths captured in Step 1. Do not improvise a raw `sc.exe create` definition.

## What should be collected if it still fails?

Collect the following after removing credentials and tokens:

```powershell
.\pingmonitor.exe --version
.\pingmonitor.exe service status --json
.\pingmonitor.exe service validate --config "$PWD\config.psd1" --endpoints "$PWD\endpoints.csv" --ui-listen 0.0.0.0:8080
sc.exe qc SplunkPingMonitor
Get-Content -LiteralPath .\logs\service.log -Tail 200
```

Also collect the relevant Service Control Manager events, `/api/status`, `/api/system`, the exact lifecycle command, and its complete error. Do not include HEC tokens, passwords, private keys, or authorization headers.

Maintainers can exercise the full isolated lifecycle, including non-forced preservation and forced replacement, from an elevated repository checkout:

```powershell
.\tests\Test-NativeWindowsServiceLifecycle.ps1 -BinaryPath .\dist\pingmonitor_windows_amd64_v6.0.0.exe
```

The test uses a unique canary service name and a guarded temporary directory. It must never be given a production service name.
