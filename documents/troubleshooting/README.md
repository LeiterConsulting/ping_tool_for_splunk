# Ping Monitor Troubleshooting FAQ

This folder contains symptom-oriented troubleshooting guides for Ping Monitor operators and administrators. Start with the question that most closely matches the observed behavior, then follow the linked procedure from identification through verification.

The short answers below help route an incident. The numbered documents contain the complete explanation, commands, decision points, recovery procedure, success criteria, and evidence to collect if the problem remains unresolved.

## Which guide should I use?

| ID | Question | Use this guide when | Full procedure |
|----|----------|---------------------|----------------|
| 0001 | Why does the Windows service enter Paused or fail to start while the executable runs manually? | The service changes from Stopped to Paused, `Start-Service` fails, or the interactive executable behaves differently from the managed instance. | [Windows Service Enters Paused State or Fails to Start](windows-service-paused-or-fails-to-start-0001.md) |
| 0002 | Why is `Install-Service.ps1` missing `-Validate` or another documented parameter? | PowerShell rejects a parameter before validation begins, suggesting that the executable and service-management script came from different releases. | [Install-Service.ps1 Is Missing an Expected Parameter](install-service-script-version-mismatch-0002.md) |
| 0003 | Why does discovery report that no active adapter with a default gateway was found? | Discovery stops before scanning, including when an explicit target was supplied or the scanner uses static routes or an isolated interface. | [Discovery Cannot Find an Adapter with a Default Gateway](discovery-no-default-gateway-0003.md) |

Catalog coverage: Troubleshooting 0001 through 0003.

## The service is Paused. Is it actually paused by an operator?

Usually not. For an NSSM-hosted installation, **Paused** commonly means that Ping Monitor exited and NSSM is waiting through its restart delay or throttling period. Other service wrappers can report failures differently, so identify the hosting model before assuming NSSM behavior.

Start with [Troubleshooting 0001](windows-service-paused-or-fails-to-start-0001.md). It explains how to distinguish:

- NSSM;
- direct Windows Service Control Manager registration;
- WinSW or another service wrapper;
- Task Scheduler.

It then covers configuration validation, persisted paths, service logs, restart or reinstall decisions, health verification, and escalation evidence.

## Why does Ping Monitor run in PowerShell but fail as a service?

The interactive and managed launches may not use the same executable, arguments, configuration, endpoint file, working directory, Windows account, permissions, UI port, or environment.

A successful interactive launch proves that one command line can run. It does not prove that the installed service uses that command line. Use [Troubleshooting 0001](windows-service-paused-or-fails-to-start-0001.md) to compare the managed definition with the working interactive command.

## Why does PowerShell say that `-Validate` cannot be found?

PowerShell validates script parameters before executing the script body. If `-Validate` is missing, no deployment validation occurred—the local `Install-Service.ps1` simply does not define that parameter.

This commonly happens when only `pingmonitor.exe` was upgraded and an older service script remained in the deployment directory. Use [Troubleshooting 0002](install-service-script-version-mismatch-0002.md) to identify both versions, run the executable's read-only validation fallback, obtain the script from the matching release tag, and verify it before service repair.

## Should I always download the newest `Install-Service.ps1`?

Use the script from the release tag matching the installed executable, not an unversioned copy from the default branch. A script that is newer than the executable can be just as misleading as one that is older.

First run:

```powershell
./pingmonitor.exe --version
```

Then obtain `Install-Service.ps1` from that exact tag. [Troubleshooting 0002](install-service-script-version-mismatch-0002.md) provides the tagged URL pattern, inspection steps, hashing guidance, and safe replacement procedure.

## Do I need to reinstall the Windows service after every upgrade?

Not necessarily.

- If the executable was replaced in place and the persisted application, arguments, working directory, and log paths still match, validate and restart the service.
- If the binary path, deployment directory, arguments, wrapper, or service settings changed, rebuild the definition using `-ForceReinstall`.
- If `pingmonitor.exe` was registered directly with `New-Service` or `sc.exe create`, replace that unsupported native registration with NSSM, Task Scheduler, or another approved wrapper.

Use the decision procedure in [Troubleshooting 0001](windows-service-paused-or-fails-to-start-0001.md) before changing service state.

## Can I validate the deployment if the installer script is old?

Yes. The current Go executable has a read-only validation command:

```powershell
./pingmonitor.exe --validate `
  --config "$PWD\config.psd1" `
  --endpoints "$PWD\endpoints.csv"
```

This validates the executable's configuration, endpoint inventory, and scheduler capacity. It does not validate the Windows service's persisted paths or execution account. After correcting the script mismatch, run the installer-level validation described in [Troubleshooting 0002](install-service-script-version-mismatch-0002.md).

## Why does discovery require a default gateway for a remote target?

It should not. Ping Monitor v5.11.0 evaluated the local adapter before it evaluated an explicit discovery target, so isolated scanners and statically routed hosts could fail before scanning. Ping Monitor v5.11.1 corrected the script, but an older adjacent script could still override the embedded fix after an executable-only upgrade. Ping Monitor v5.11.2 corrects both the network-selection logic and the upgrade path by making its version-matched embedded script authoritative by default.

Use [Troubleshooting 0003](discovery-no-default-gateway-0003.md) to identify the affected script, upgrade the standalone and embedded copies correctly, verify explicit-target behavior, and collect sanitized route evidence if automatic local discovery remains ambiguous.

## Which issue should I fix first when both symptoms occur?

Fix the installer/runtime mismatch first:

1. Follow [Troubleshooting 0002](install-service-script-version-mismatch-0002.md).
2. Confirm that executable and installer validation both succeed.
3. Follow [Troubleshooting 0001](windows-service-paused-or-fails-to-start-0001.md) to inspect and repair the managed service definition.

An old installer cannot reliably report or repair a service installed for a newer deployment.

## What information is safe and useful in a troubleshooting report?

Useful evidence generally includes:

- executable version and SHA-256 hash;
- installer script SHA-256 hash and supported syntax;
- Windows service host and persisted command line;
- Configuration Advisor output;
- recent service stdout and stderr;
- service state, process ID, and `/api/status` output;
- the exact start, restart, or validation error.

Remove HEC tokens, passwords, authorization headers, private keys, and other secrets before sharing evidence. Each numbered guide contains a problem-specific collection checklist.

## How is this FAQ maintained?

Every troubleshooting document uses a four-digit sequential ID and ends its filename with that ID, for example:

```text
descriptive-troubleshooting-topic-0003.md
```

When a new document is added to this folder, update this README in the same commit:

1. Add the document to the routing table.
2. Update the catalog coverage line.
3. Add or revise the FAQ question that routes readers to it.
4. Cross-link related procedures when one issue can lead to another.
5. Keep the short answer here concise; keep commands and full recovery logic in the numbered document.

Each numbered guide should contain:

- a generic symptom description;
- the troubleshooting goal;
- an explanation of why the behavior occurs;
- safe identification and validation steps;
- decision-based recovery actions;
- success criteria;
- evidence to collect when escalation is required;
- a reminder to remove secrets.

Avoid customer names, correspondence references, environment-specific credentials, and incident-only assumptions. Examples may use conventional placeholder paths, addresses, and service names.

Maintainers can identify Markdown files missing from the catalog with this PowerShell check, run from this folder:

```powershell
$catalog = Get-Content ./README.md -Raw

Get-ChildItem -File -Filter '*.md' |
  Where-Object {
    $_.Name -ne 'README.md' -and
    $catalog -notmatch [regex]::Escape($_.Name)
  }
```

No output means every numbered Markdown document is referenced by the FAQ landing page.
