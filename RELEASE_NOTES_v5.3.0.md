# Ping Tool for Splunk - Release v5.3.0

Release date: 2026-06-15

## Purpose

Feature release for the Go runtime that introduces the first embedded local admin UI preview and aligns all runtime version markers/documentation for immediate Windows testing.

## What Changed

- Added an optional embedded local web UI to the Go runtime.
- Added read-only `GET /healthz`, `GET /api/status`, and `GET /api/endpoints` routes.
- Added a clean-room Ping Monitor shell that preserves continuity with the SNMP app's interface language while keeping a distinct blue/teal product palette.
- Centralized the Go runtime version marker in code and updated diagnostics/build defaults to `v5.3.0`.
- Updated operator documentation and runtime docs for the new UI startup flow.

## Validated Windows Artifact

Current compiled test artifact:

- `go/dist/pingmonitor_v5.3.0_windows_amd64.exe`

Version check:

- `Ping Monitor v5 (Go) - v5.3.0`

## Recommended Test Launch

From a test folder containing the binary plus `config.psd1` and `endpoints.csv`:

```powershell
.\pingmonitor_v5.3.0_windows_amd64.exe --config .\config.psd1 --endpoints .\endpoints.csv --ui-listen 127.0.0.1:8080
```

Then open `http://127.0.0.1:8080`.

## Notes

- The embedded UI preview is currently read-only.
- Endpoint editing, config editing, and discovery workflows are not part of `v5.3.0` yet.
- The Splunk app version remains `2.7.3` (build `31`) for this runtime-focused release.