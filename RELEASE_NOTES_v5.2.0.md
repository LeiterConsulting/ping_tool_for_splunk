# Ping Tool for Splunk - Release v5.2.0

Release date: 2026-06-15

## Highlights

- Added optional dev endpoint routing in Go runtime:
  - New endpoints.csv column: dev
  - dev=true endpoints emit `record_type=summary_dev` and `record_type=ping_dev`
  - Production rollups remain clean on `record_type=summary`
- Added Splunk app Dev Devices dashboard for dedicated dev/test visibility
- Updated Windows service installer to default to Go runtime (`pingmonitor.exe`) while preserving legacy PowerShell runtime mode
- Updated docs and endpoint samples to include dev-aware schema and Go-first service guidance

## Splunk App

- App version: 2.7.3
- Build: 31
- Packaged artifact:
  - `splunk_app/dist/ping_monitor_2.7.3_build31_20260615.tar.gz`

### AppInspect (precert)

- Command run:
  - `splunk-appinspect inspect splunk_app/dist/ping_monitor_2.7.3_build31_20260615.tar.gz --mode precert`
- Result summary:
  - errors: 0
  - failures: 0
  - warnings: 4
  - success: 103
  - not_applicable: 142
- Warnings are expected environment checks (AArch64/symlink checks that require alternate OS/API execution) and informational `collections.conf` presence.

## Go Binaries

Built artifacts:

- `go/dist/pingmonitor_v5.2.0_linux_amd64`
- `go/dist/pingmonitor_v5.2.0_linux_arm64`
- `go/dist/pingmonitor_v5.2.0_darwin_amd64`
- `go/dist/pingmonitor_v5.2.0_darwin_arm64`
- `go/dist/pingmonitor_v5.2.0_windows_amd64.exe`

## Upgrade Notes

- For production dashboards/reports, continue querying `record_type=summary`.
- For dev/test monitoring, use `record_type=summary_dev` or the Splunk app Dev Devices page.
- Windows service install defaults to Go runtime. Use `-Runtime powershell` only for legacy deployments.
