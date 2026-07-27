# Install-Service.ps1 Is Missing an Expected Parameter

Document ID: Troubleshooting 0002

Applies to: Ping Monitor on Windows when the executable and service-management script may have come from different releases

## What is the symptom?

A documented `Install-Service.ps1` command fails before performing any validation or service operation. PowerShell reports an error similar to:

```text
Install-Service.ps1: A parameter cannot be found that matches parameter name 'Validate'.
```

The same problem can affect other parameters introduced in newer releases, including `-ForceReinstall`, `-Restart`, `-WorkingDirectory`, or `-Json`.

The Ping Monitor executable may still start normally from PowerShell because the executable and `Install-Service.ps1` are separate files. Updating one does not automatically update the other.

## What is the goal?

The goal is to use a Ping Monitor executable and `Install-Service.ps1` from the same release, verify the script before executing it, validate the deployment, and then repair or control the Windows service using supported commands.

## Why does this happen?

Common causes include:

- replacing `pingmonitor.exe` without replacing `Install-Service.ps1`;
- extracting an executable-only archive over an older deployment;
- copying the service script from another server or an older checkout;
- downloading the repository's default branch instead of the tag matching the installed executable;
- retaining an older deployment directory while launching a newer executable from another directory;
- saving a web page instead of the raw PowerShell file.

The folder name is not reliable version evidence. A directory ending in `-main`, for example, may have been downloaded at any point in time and may contain an older script.

## Step 1: Identify the executable version

From the deployment directory, run:

```powershell
./pingmonitor.exe --version
```

Record the complete version, such as `v5.9.0`. Use that exact release tag when obtaining the matching service script.

If the executable does not support `--version`, it is itself old enough that the complete deployment should be reviewed before changing the service.

## Step 2: Inspect the installed script without running it

List the parameters exposed by the local script:

```powershell
(Get-Command ./Install-Service.ps1).Parameters.Keys | Sort-Object
```

Display its supported command forms:

```powershell
Get-Command ./Install-Service.ps1 -Syntax
```

If a parameter shown in the documentation is absent, stop retrying the command. PowerShell rejects unknown parameters before the script can validate files, inspect the service, or write diagnostic logs.

Record the current script hash before replacing it:

```powershell
Get-FileHash ./Install-Service.ps1 -Algorithm SHA256
```

## Step 3: Validate with the executable while the script is being corrected

The Go executable provides its own read-only validation command. This does not install, remove, start, or stop a service:

```powershell
./pingmonitor.exe --validate `
  --config "$PWD\config.psd1" `
  --endpoints "$PWD\endpoints.csv"
```

Resolve every reported blocker before attempting to start the service. Recommendations that are not identified as blockers do not prevent startup.

This command validates the current executable, configuration, endpoint inventory, and scheduler capacity. It does not verify the Windows service's persisted application path, arguments, working directory, or execution account.

## Step 4: Obtain the script from the matching release tag

Prefer the source archive attached to the same tagged release as the executable. Extract `Install-Service.ps1` from that archive rather than copying the file from an unversioned checkout.

The raw tagged-file URL follows this pattern:

```text
https://raw.githubusercontent.com/LeiterConsulting/ping_tool_for_splunk/<release-tag>/Install-Service.ps1
```

For example, an executable reporting `v5.9.0` should use:

```text
https://raw.githubusercontent.com/LeiterConsulting/ping_tool_for_splunk/v5.9.0/Install-Service.ps1
```

Do not substitute `main` for the release tag. The default branch may be newer or older than the deployed executable.

When downloading through a browser, confirm that the result is a PowerShell source file rather than an HTML page. The first lines should contain PowerShell comments and a `param` block—not HTML markup.

## Step 5: Replace the script safely

Replacing the service-management script does not require replacing `config.psd1` or `endpoints.csv`.

Keep the old script for comparison:

```powershell
Rename-Item `
  -LiteralPath ./Install-Service.ps1 `
  -NewName Install-Service.previous.ps1
```

Place the tagged `Install-Service.ps1` in the deployment directory. Then calculate its hash and compare it with the checksum supplied by the project or release operator:

```powershell
Get-FileHash ./Install-Service.ps1 -Algorithm SHA256
```

Review the file before execution. If Windows marked the verified file as downloaded from the internet, remove that mark explicitly:

```powershell
Unblock-File ./Install-Service.ps1
```

Confirm the required parameters are now present:

```powershell
Get-Command ./Install-Service.ps1 -Syntax
```

The command forms required by current troubleshooting procedures should include `-Validate`, `-Status`, `-Restart`, and `-ForceReinstall`.

## Step 6: Confirm the NSSM prerequisite

The current Windows service installer requires a vetted NSSM executable either beside `Install-Service.ps1` or available on `PATH`. It does not automatically download and execute NSSM.

Check both locations:

```powershell
Get-Command nssm.exe -ErrorAction SilentlyContinue
Test-Path ./nssm.exe
```

If NSSM is unavailable, obtain the organization-approved version before continuing. Do not download and execute a service wrapper from an unverified third-party location.

If the existing installation uses another service wrapper or Task Scheduler, use the matching troubleshooting procedure instead of assuming the NSSM installer controls it.

## Step 7: Run the current deployment validation

The script-level validation displays the desired service definition in addition to running the Configuration Advisor:

```powershell
./Install-Service.ps1 -Validate `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv"
```

The output should identify the intended executable, configuration, endpoint inventory, working directory, UI address, and runtime version.

## Step 8: Inspect or repair the service definition

Inspect the currently persisted service definition:

```powershell
./Install-Service.ps1 -Status
./Install-Service.ps1 -Status -Json
```

If its application, arguments, or working directory does not match the validated definition, reinstall it from an elevated PowerShell session:

```powershell
./Install-Service.ps1 -Install `
  -BinaryPath "$PWD\pingmonitor.exe" `
  -ConfigPath "$PWD\config.psd1" `
  -EndpointsPath "$PWD\endpoints.csv" `
  -ForceReinstall
```

Stop any interactively launched `pingmonitor.exe` before starting or reinstalling the service. A second process can conflict with the single-instance guard or the configured UI port.

If the service definition already matches, use the current controller so startup stabilization and recent log output are included:

```powershell
./Install-Service.ps1 -Restart
```

## Step 9: Verify recovery

Confirm the service remains running:

```powershell
./Install-Service.ps1 -Status
Get-Service SplunkPingMonitor
```

Verify the local health and runtime endpoints:

```powershell
Invoke-WebRequest 'http://127.0.0.1:8080/healthz' -UseBasicParsing

Invoke-RestMethod 'http://127.0.0.1:8080/api/status' |
  ConvertTo-Json -Depth 6
```

Successful recovery has all of these characteristics:

- the executable and script come from the same tagged release;
- the expected installer parameters are present;
- executable and installer validation complete successfully;
- the persisted service paths match the intended deployment;
- the service remains **Running** beyond the stabilization window;
- `/healthz` returns HTTP `200` with `ok`;
- `/api/status` reports the expected version and `runtime.state` of `running`.

## What should be collected if it still fails?

Collect the following without including secrets:

```powershell
./pingmonitor.exe --version
Get-FileHash ./pingmonitor.exe -Algorithm SHA256
Get-FileHash ./Install-Service.ps1 -Algorithm SHA256
Get-Command ./Install-Service.ps1 -Syntax
./Install-Service.ps1 -Status -Json
```

Also include:

- the tagged release used to obtain each file;
- the complete output from executable and installer validation;
- the complete error returned by `-Restart` or `-ForceReinstall`;
- the last 100 lines of the configured service stdout and stderr logs.

Do not include HEC tokens, passwords, or other secrets in a troubleshooting report.
