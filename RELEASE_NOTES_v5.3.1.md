# Ping Tool for Splunk - Release v5.3.1

Release date: 2026-06-15

## Highlights

- Embedded Go admin UI now supports live deployment management instead of read-only inspection.
- Existing deployments remain drop-in compatible: the runtime still picks up co-located `config.psd1` and `endpoints.csv` automatically.
- Dev/test endpoints remain excluded from production rollups while the Splunk app now shows current dev-mode devices correctly.
- Release artifacts are built for Windows amd64, Linux amd64/arm64, and macOS amd64/arm64.

## What Changed

- Expanded the embedded Go admin UI to include:
  - full endpoint CRUD against the live CSV
  - discovery with staged import, duplicate handling, pagination, sorting, and bulk actions
  - live config editing with HEC test actions
  - settings help modals for operational guidance
- Kept endpoint CSV backward compatibility by preserving the optional trailing `dev` flag semantics for endpoint records.
- Updated service and runtime documentation for:
  - installing and starting the Go runtime as the service target
  - choosing the embedded UI bind address and port with `--ui-listen` / `-UIListen`
  - opening an existing deployment in place without re-entering config manually
- Corrected the Splunk app Dev Devices dashboard so it:
  - filters on each target's latest dev state instead of stale historical membership alone
  - uses SimpleXML-safe top-level SPL so the dashboard renders current dev devices correctly in Splunk Web
- Finalized the Go runtime release metadata at `v5.3.1` and the Splunk app release metadata at `2.7.4` build `32`.

## Release Assets

Go runtime artifacts:

- `go/dist/pingmonitor_v5.3.1_windows_amd64.exe`
- `go/dist/pingmonitor_v5.3.1_linux_amd64`
- `go/dist/pingmonitor_v5.3.1_linux_arm64`
- `go/dist/pingmonitor_v5.3.1_darwin_amd64`
- `go/dist/pingmonitor_v5.3.1_darwin_arm64`

Splunk app artifact:

- `splunk_app/dist/ping_monitor_2.7.4_build32_20260615.tar.gz`

AppInspect report:

- `splunk_app/dist/ping_monitor_2.7.4_build32_20260615_appinspect.txt`

## AppInspect (precert)

Command run:

- `splunk-appinspect inspect splunk_app/dist/ping_monitor_2.7.4_build32_20260615.tar.gz --mode precert`

Result summary:

- errors: `0`
- failures: `0`
- future_failures: `0`
- warnings: `4`
- success: `103`
- not_applicable: `142`

Warnings are limited to expected cross-OS capability checks when run on Windows plus the informational `collections.conf` notice.

## Validation

- Windows release binary version check: `Ping Monitor v5 (Go) - v5.3.1`
- Splunk Dev Devices dashboard validated after deployment with current dev devices rendering correctly.
- Release package AppInspect run completed successfully on the exact final tarball.

## Service Launch Examples

```powershell
# Default local UI bind
.\Install-Service.ps1 -Install -Runtime go

# Custom local UI port
.\Install-Service.ps1 -Install -Runtime go -UIListen 127.0.0.1:8090
```

## Drop-In Upgrade Behavior

If `pingmonitor.exe` starts in a deployment folder that already contains `config.psd1` and `endpoints.csv`, it loads those files automatically and the embedded UI surfaces them immediately for live editing.