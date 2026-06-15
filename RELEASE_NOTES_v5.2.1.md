# Ping Tool for Splunk - Release v5.2.1

Release date: 2026-06-15

## Purpose

Hotfix release to preserve endpoint CSV backward compatibility after v5.2.0.

## What Changed

- Restored compatibility by keeping optional `dev` as the final `endpoints.csv` column.
- Kept `dev` parsing optional so existing endpoint files continue to load unchanged.
- Updated runtime version markers and build defaults to `v5.2.1`.
- Expanded build scripts to include `linux/arm64` in the default target matrix.

## Go Runtime Artifacts

Current binaries:

- `go/dist/pingmonitor_v5.2.1_linux_amd64`
- `go/dist/pingmonitor_v5.2.1_linux_arm64`
- `go/dist/pingmonitor_v5.2.1_darwin_amd64`
- `go/dist/pingmonitor_v5.2.1_darwin_arm64`
- `go/dist/pingmonitor_v5.2.1_windows_amd64.exe`

Version check (Windows binary):

- `Ping Monitor v5 (Go) - v5.2.1`

## Splunk App Packaging

- Splunk app version remains `2.7.3` (build `31`) for this hotfix cycle.
- Existing package artifact remains valid:
  - `splunk_app/dist/ping_monitor_2.7.3_build31_20260615.tar.gz`

## Notes

- v5.2.0 remains tagged historically for the initial dev endpoint release.
- v5.2.1 should be used for production deployments requiring strict endpoint CSV schema compatibility.
