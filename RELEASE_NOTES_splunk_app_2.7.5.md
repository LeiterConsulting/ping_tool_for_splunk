# Ping Monitor for Splunk - Release 2.7.5 Build 33

Release date: 2026-06-15

## Highlights

- Added a dedicated `Prod Devices` dashboard immediately to the right of `Ping Monitor Overview` so production-only health can be reviewed without changing the whole-platform overview.
- Kept `Ping Monitor Overview` as the aggregate platform view across both production and dev/test devices.
- Brought `Prod Devices` and `Dev Devices` into tile parity with endpoint count, availability, average latency, packet-loss count, health table, and trend charts.
- Scoped production membership from each target's latest `dev` state and treated missing `dev` values as production so existing endpoint files continue to behave correctly.

## Paired Runtime

- Go runtime version unchanged: `v5.3.1`
- Existing `v5.3.1` Windows/Linux/macOS binaries remain the intended runtime pairing for this app build.

## Release Assets

- `splunk_app/dist/ping_monitor_2.7.5_build33_20260615.tar.gz`
- `splunk_app/dist/ping_monitor_2.7.5_build33_20260615_appinspect.txt`

## AppInspect (precert)

Command run:

- `Set-Location "d:\vscode projects\ping_tool_for_splunk\splunk_app"; splunk-appinspect inspect dist/ping_monitor_2.7.5_build33_20260615.tar.gz --mode precert`

Result summary:

- errors: `0`
- failures: `0`
- future_failures: `0`
- warnings: `4`
- success: `103`
- not_applicable: `142`

Warnings are limited to the expected Windows-host capability checks plus the informational `collections.conf` notice.

## Upload Checklist

- Upload `splunk_app/dist/ping_monitor_2.7.5_build33_20260615.tar.gz` in Splunk Web.
- Confirm nav order is `Ping Monitor Overview`, `Prod Devices`, `Dev Devices`, `Asset Health Correlation`, `Setup`.
- Confirm `Ping Monitor Overview` still reflects the whole platform.
- Confirm `Prod Devices` excludes targets whose latest mode is dev/test and still includes targets with no explicit `dev` flag.
- Confirm `Dev Devices` continues to reflect latest-mode dev/test membership.