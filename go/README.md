# Ping Monitor v5 (Go)

Ping Monitor v5.3.1 is the current published Go runtime. It is the primary cross-platform runtime for this project and is designed for drop-in upgrades, bounded resource usage, endpoint hot reload, and live deployment management through the embedded local admin UI.

## Current Go Release

- Version: `v5.3.1`
- Primary runtime status: current and recommended
- Top-level release notes: [../RELEASE_NOTES_v5.3.1.md](../RELEASE_NOTES_v5.3.1.md)
- Historical runtime notes: [../past_versions.md](../past_versions.md)

## What The Go Runtime Includes

- A single binary runtime for Windows, Linux, and macOS.
- Drop-in reuse of existing `config.psd1` and `endpoints.csv` deployment files.
- Automatic `endpoints.csv` hot reload between cycles with last-known-good fallback on invalid edits.
- Embedded local admin UI for endpoint CRUD, discovery, config editing, dev/prod marking, and HEC connectivity tests.
- Resilient HEC and metrics delivery with retry support and optional dead-letter handling.

## Runtime Compatibility

- Uses `config.psd1` and `endpoints.csv` fields compatible with the older Windows runtime model.
- Emits the same core event schema (`record_type=ping` and `record_type=summary`) plus `summary_dev` and `ping_dev` for dev endpoints.
- Maintains metrics compatibility behavior through `metrics.compat_mode`.

## Quick Start

From the repository root:

```powershell
# Build the current Windows binary
go -C .\go build -o .\pingmonitor.exe .\go\cmd\pingmonitor

# Run one cycle and exit
.\pingmonitor.exe --run-once

# Run the monitor with the local admin UI
.\pingmonitor.exe --ui-listen 127.0.0.1:8080

# Launch the local admin UI without starting the monitor engine
.\pingmonitor.exe --ui-listen 127.0.0.1:8080 --ui-only
```

With the default file names, the binary prefers `config.psd1` and `endpoints.csv` next to the executable. That keeps in-place upgrades aligned with existing deployment folders.

## Runtime Flags

| Flag | Purpose |
|------|---------|
| `--config` | Path to `config.psd1` (preferred), `config.yaml`, or `config.json` |
| `--endpoints` | Path to `endpoints.csv` |
| `--run-once` | Run a single cycle and exit |
| `--max-cycles` | Stop after a fixed number of cycles |
| `--ping-mode` | Override `ping.mode` with `auto`, `raw`, or `exec` |
| `--ui-listen` | Bind address for the local admin UI |
| `--ui-only` | Serve the local admin UI without starting the monitor engine |
| `--version` | Print the runtime version |

## Configuration Model

- `config.psd1` is the preferred config format for the Go runtime.
- `config.yaml` and `config.json` are supported fallbacks.
- If no supported config exists, the runtime and editable UI can initialize a new `config.yaml`.
- Relative config paths are resolved from the directory containing the selected config file.
- The current checked-in sample lives in [../config.psd1](../config.psd1).

### Endpoint File Rules

- `endpoints.csv` accepts legacy two-column files (`ip,hostname`).
- The optional `dev` flag remains a trailing column when present.
- `dev=true` endpoints emit `record_type=summary_dev` and `record_type=ping_dev`.
- Standard production searches that use `record_type=summary` remain unaffected by dev/test devices.

## Hot Reload And Runtime Behavior

- The runtime loads engine configuration at process start.
- The runtime checks `endpoints.csv` between cycles and applies changes on the next cycle.
- If `endpoints.csv` becomes temporarily invalid, the runtime keeps using the last known good endpoint set and logs the error.
- If raw ICMP is unavailable and `ping.mode=auto` is in use, the runtime falls back to the OS `ping` command and stays there for the remainder of the run.
- If Splunk HEC or the metrics endpoint is unavailable, the runtime continues running and retries delivery with backoff instead of exiting.

## Embedded Local Admin UI

The optional local admin UI runs from the same Go binary and works against the same live deployment files as the runtime itself.

It provides:

- endpoint CRUD with bulk dev/prod actions
- config editing against the active config file
- discovery with staged import workflows
- HEC event and metrics endpoint validation
- settings help modals for the runtime configuration surface

The UI serves these key routes:

- `GET /healthz`
- `GET /api/status`
- `GET` and `PUT /api/endpoints`
- `GET` and `PUT /api/config`
- `POST /api/discovery/run`
- `POST /api/output/test`

Operational notes:

- Existing `config.psd1` and `endpoints.csv` files next to the binary are loaded at startup and surfaced in the UI immediately.
- Endpoint edits saved in the UI are written back to the live endpoint file that the runtime hot reloads.
- Config edits saved in the UI are written back to the active config file, but engine-level config changes still require a process or service restart.
- When saving over an existing config or endpoint file, the UI creates a timestamped `.bak` backup first.
- The discovery workflow ships through an embedded PowerShell script fallback, so drop-in deployments do not need a separate `DiscoverEndpoints.ps1` file just to use the UI.

## Ping Mode

The Go runtime supports multiple ping strategies:

- `auto`: try raw ICMP first, then fall back to OS `ping` when needed
- `raw`: raw ICMP only
- `exec`: OS `ping` only

Override on the command line:

```powershell
.\pingmonitor.exe --run-once --ping-mode auto
```

Or via config:

```powershell
ping = @{
	mode = "auto"
}
```

## HEC And Metrics Notes

- `output_mode` controls file, HEC, or dual event output.
- `metrics.enabled` and `metrics.mode` control metrics delivery.
- `hec.retry.*` settings define retry behavior.
- If `hec.drop_on_failure = $true` and `hec.dead_letter_path` is set, dead-letter output can be preserved and rotated.

Example dead-letter settings:

```powershell
hec = @{
	dead_letter_path = "./logs/hec_deadletter.ndjson"
	dead_letter_rotation_size_mb = 50
}
```

## Build Artifacts

Build all current Go release targets:

- PowerShell: `pwsh -File .\go\build.ps1 -Version v5.3.1`
- Bash: `./go/build.sh dist`

Current default targets:

- Windows amd64
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

## Running As A Service

### Windows

From the repository root:

```powershell
.\Install-Service.ps1 -Install -Runtime go
```

The shipped installer:

- installs the Go runtime as the default Windows service target
- enables the local admin UI on `127.0.0.1:8080` by default
- supports `-UIListen` to select a different bind address or port
- supports `-DisableUI` for a headless service

For an in-place upgrade, replace the existing binary in the deployment folder and restart the service. The co-located `config.psd1` and `endpoints.csv` are picked up automatically and shown in the UI.

### Linux

Run the published Linux binary under `systemd` with the same flags you would use interactively.

Example:

```ini
[Unit]
Description=Splunk Ping Monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/ping_monitor
ExecStart=/opt/ping_monitor/pingmonitor --config /opt/ping_monitor/config.psd1 --endpoints /opt/ping_monitor/endpoints.csv --ui-listen 127.0.0.1:8080
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### macOS

Run the published macOS binary under `launchd` or your preferred native service manager.

Example `launchd` configuration:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.leiterconsulting.pingmonitor</string>
	<key>ProgramArguments</key>
	<array>
		<string>/opt/ping_monitor/pingmonitor</string>
		<string>--config</string>
		<string>/opt/ping_monitor/config.psd1</string>
		<string>--endpoints</string>
		<string>/opt/ping_monitor/endpoints.csv</string>
		<string>--ui-listen</string>
		<string>127.0.0.1:8080</string>
	</array>
	<key>WorkingDirectory</key>
	<string>/opt/ping_monitor</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
```

## Related Documentation

- [../README.md](../README.md)
- [../BEST_PRACTICES.md](../BEST_PRACTICES.md)
- [../splunk_app/ping_monitor/README.md](../splunk_app/ping_monitor/README.md)
- [../past_versions.md](../past_versions.md)
