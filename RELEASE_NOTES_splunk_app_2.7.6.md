# Ping Monitor for Splunk - Release 2.7.6 Build 34

Release date: 2026-06-25

## Highlights

- Fixed Setup/dashboard config token reliability so unresolved literal tokens no longer leak into UI text when KV configuration fields are missing or delayed.
- Added safe fallback config rows for Setup, Overview, Prod Devices, and Dev Devices so first-run installs stay stable without manual KV pre-seeding.
- Verified live in Splunk after app upload and restart: Setup, Overview, Prod Devices, and Dev Devices all render resolved config values (`ping`, `ping_monitor`, `ping_metrics`) with no `$result.*` leakage.

## Changed Views

- `splunk_app/ping_monitor/default/data/ui/views/setup.xml`
- `splunk_app/ping_monitor/default/data/ui/views/ping_overview.xml`
- `splunk_app/ping_monitor/default/data/ui/views/prod_devices.xml`
- `splunk_app/ping_monitor/default/data/ui/views/dev_devices.xml`

## Paired Runtime

- Go runtime version unchanged: `v5.3.1`

## Release Assets

- `splunk_app/dist/ping_monitor_2.7.6_build34_20260625.tar.gz`
- `splunk_app/dist/ping_monitor_2.7.6_build34_20260625_appinspect.txt`

## AppInspect (precert)

Command run:

- `Set-Location "d:\vscode projects\ping_tool_for_splunk\splunk_app"; splunk-appinspect inspect dist/ping_monitor_2.7.6_build34_20260625.tar.gz --mode precert`

Result summary:

- errors: `0`
- failures: `0`
- future_failures: `0`
- warnings: `4`
- success: `103`
- not_applicable: `142`

Warnings are limited to expected cross-OS capability checks when run on Windows plus the informational `collections.conf` notice.

## Validation Notes

- Installed and validated in Splunk Web from App Listing (`Ping Monitor` version `2.7.6`).
- Setup page now shows resolved values and no literal `$result.last_updated$` leak.
- Overview/Prod/Dev pages render with resolved config tokens and no literal `$result.pm_index$` or similar token leakage.
