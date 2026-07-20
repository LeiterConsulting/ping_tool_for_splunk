# Ping Monitor v5 (Go)

Ping Monitor v5.7.2 is the current Go runtime. It ensures the confirmation-gated controlled restart cannot be hidden by stale browser assets after an upgrade.

## Current Go Release

- Version: `v5.7.2`
- Primary runtime status: current and recommended
- Top-level release notes: [../RELEASE_NOTES_v5.7.2.md](../RELEASE_NOTES_v5.7.2.md)
- Historical runtime notes: [../past_versions.md](../past_versions.md)

## What The Go Runtime Includes

- A single binary runtime for Windows, Linux, and macOS.
- Drop-in reuse of existing `config.psd1` and `endpoints.csv` deployment files.
- Automatic `endpoints.csv` hot reload between cycles with last-known-good fallback on invalid edits.
- Embedded admin UI with live runtime truth, revision-safe endpoint/config editing, cancellable discovery, dev/prod marking, and HEC connectivity tests.
- Configuration Advisor with tolerant multi-error CSV inspection, worst-case capacity evidence, operating profiles, safe inventory cleanup, and confirmed config optimization.
- Fsynced, bounded HEC/metrics outbox with asynchronous delivery, restart recovery, per-sink progress, and optional indexer acknowledgment.

## Runtime Compatibility

- Uses `config.psd1` and `endpoints.csv` fields compatible with the older Windows runtime model.
- Emits schema v3 events with stable collector, endpoint, cycle, and event identities plus measured-versus-censored latency metadata. Core `record_type` values remain compatible.
- Maintains metrics compatibility behavior through `metrics.compat_mode`.

## Quick Start

From the repository root:

```powershell
# Build the current Windows binary
go -C .\go build -o .\pingmonitor.exe .\go\cmd\pingmonitor

# Run one cycle and exit
.\pingmonitor.exe --run-once

# Run the monitor with the local admin UI
.\pingmonitor.exe --ui-listen 0.0.0.0:8080

# Launch the local admin UI without starting the monitor engine
.\pingmonitor.exe --ui-listen 0.0.0.0:8080 --ui-only
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
| `--validate` | Validate config, endpoints, and worst-case scheduler capacity without probing |
| `--version` | Print the runtime version |

Advisor subcommands are separate from legacy runtime flags:

```powershell
.\pingmonitor.exe analyze --profile current
.\pingmonitor.exe optimize --profile standard
.\pingmonitor.exe optimize --profile standard --apply-safe
.\pingmonitor.exe optimize --profile sla --apply-profile
.\pingmonitor.exe benchmark --profile current
.\pingmonitor.exe profiles
```

Use `--format json` with `analyze`, `optimize`, or `benchmark` for automation. Analysis is read-only. The benchmark is explicitly non-SLA: it exercises config/inventory parsing, the schedule planner, temporary writes beside the deployment config, and three probes to `127.0.0.1` using the configured backend. It does not probe monitored endpoints, update health state, or contact Splunk.

## Configuration Model

- `config.psd1` is the preferred config format for the Go runtime.
- `config.yaml` and `config.json` are supported fallbacks.
- If no supported config exists, the runtime and editable UI can initialize a new `config.yaml`.
- Relative config paths are resolved from the directory containing the selected config file.
- The current checked-in sample lives in [../config.psd1](../config.psd1).

### Endpoint File Rules

- `endpoints.csv` accepts legacy two-column files (`ip,hostname`).
- The optional `endpoint_id` column is persisted by the UI; when absent, a deterministic ID is derived from the target IP.
- Target values must be literal IP addresses. Malformed rows, duplicate canonical IPs/IDs, and invalid `dev` values are rejected.
- `dev=true` endpoints emit `record_type=summary_dev` and `record_type=ping_dev`.
- Standard production searches that use `record_type=summary` remain unaffected by dev/test devices.

## Hot Reload And Runtime Behavior

- The runtime loads engine configuration at process start.
- The runtime checks `endpoints.csv` between cycles and applies changes on the next cycle.
- If `endpoints.csv` becomes temporarily invalid, the runtime keeps using the last known good endpoint set and logs the error.
- Windows uses the native ICMP API without spawning a process per probe. Other platforms use UDP/raw ICMP, with a controlled OS-command fallback in `auto` mode.
- Probe dispatches are spread across each cycle. Startup rejects a worst-case workload that cannot fit the requested cadence, and cycle logs expose utilization/overrun telemetry.
- Completed network-output cycles are fsynced to a bounded outbox; a separate worker performs HEC requests so network latency cannot extend the probe cycle.
- A per-deployment lock prevents two monitor engines from emitting duplicate data from the same configuration.
- Health state is checkpointed next to the config file and uses configurable down/recovery hysteresis.
- If Splunk HEC or the metrics endpoint is unavailable, the runtime continues running and retries delivery with backoff instead of exiting.

## Embedded Local Admin UI

The optional local admin UI runs from the same Go binary and works against the same live deployment files as the runtime itself.

It provides:

- endpoint CRUD with bulk dev/prod actions
- explicit-selection safeguards for bulk actions and revision conflict protection for saves
- live collector, monitoring-cycle, endpoint-reload, Splunk-delivery, and outbox status
- config editing against the active config file
- cancellable discovery with host-count preflight and staged import workflows
- HEC event and metrics endpoint validation
- settings help modals for the runtime configuration surface
- a dedicated Advisor view with readiness counts, current/proposed schedule evidence, findings, change previews, safe fixes, profiles, and a bounded benchmark
- a confirmation-gated Restart Collector action when a saved config revision is not yet active

The UI serves these key routes:

- `GET /healthz`
- `GET /api/status`
- `GET` and `PUT /api/endpoints`
- `GET` and `PUT /api/config`
- `GET /api/advisor` and `GET /api/advisor/profiles`
- `POST /api/advisor/apply` and `POST /api/advisor/benchmark`
- `POST /api/runtime/restart`
- `POST /api/discovery/run`
- `POST /api/output/test`

Operational notes:

- Existing `config.psd1` and `endpoints.csv` files next to the binary are loaded at startup and surfaced in the UI immediately.
- HEC tokens are write-only: API responses never contain stored credentials, and a blank token field preserves the stored value.
- Endpoint edits saved in the UI are written back to the live endpoint file that the runtime hot reloads.
- Stale config or endpoint drafts are rejected with `409 Conflict` instead of overwriting a newer file revision.
- Config edits saved in the UI are written back to the active config file, but engine-level config changes still require a process or service restart.
- When saving over an existing config or endpoint file, the UI creates a timestamped `.bak` backup first.
- The discovery workflow ships through an embedded PowerShell script fallback, so drop-in deployments do not need a separate `DiscoverEndpoints.ps1` file just to use the UI.

## Ping Mode

The Go runtime supports multiple ping strategies:

- `auto`: try native/UDP ICMP first, then fall back to OS `ping` when needed
- `raw`: native/UDP ICMP only
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
- Completed HEC/metrics cycles are persisted atomically in `delivery.spool_path` before network delivery. Legacy `drop_on_failure` and dead-letter settings are retained only for config compatibility and do not permit v5.5 to discard a failed cycle.
- `hec.use_ack` and `metrics.use_ack` upgrade delivery confirmation from HEC acceptance to Splunk indexer acknowledgment. `/api/status` states which confirmation mode is active.
- Do not enable `use_ack` until indexer acknowledgment is enabled on the matching Splunk HEC token.

## Build Artifacts

Build all current Go release targets:

- PowerShell: `pwsh -File .\go\build.ps1 -Version v5.7.2`
- Bash: `./go/build.sh dist`

Current default targets:

- Windows amd64
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

## Running As A Service

### Windows

Validate the intended deployment from a non-elevated PowerShell 7.4+ session:

```powershell
.\Install-Service.ps1 -Validate `
  -BinaryPath D:\pingmonitor\pingmonitor.exe `
  -ConfigPath D:\pingmonitor\config.psd1 `
  -EndpointsPath D:\pingmonitor\endpoints.csv
```

Then use an elevated session to install and control it:

```powershell
.\Install-Service.ps1 -Install `
  -BinaryPath D:\pingmonitor\pingmonitor.exe `
  -ConfigPath D:\pingmonitor\config.psd1 `
  -EndpointsPath D:\pingmonitor\endpoints.csv
.\Install-Service.ps1 -Status
.\Install-Service.ps1 -Restart
.\Install-Service.ps1 -Stop
.\Install-Service.ps1 -Start
.\Install-Service.ps1 -Uninstall
```

The shipped installer:

- installs the Go runtime as the default Windows service target
- defaults the working directory to the selected binary's deployment folder
- verifies persisted NSSM application, arguments, working-directory, and log settings
- uses delayed automatic start, graceful console shutdown, NSSM restart, and SCM recovery actions
- rotates service stdout/stderr logs
- enables the admin UI on `0.0.0.0:8080` by default
- supports `-UIListen` to select a different bind address or port
- accepts loopback and non-loopback UI binds; `-AllowRemoteUI` remains a compatibility no-op in v5.7
- supports `-DisableUI` for a headless service
- supports `-ForceReinstall` when a binary or deployment path changes
- runs the complete advisor during validation and surfaces recent service stdout/stderr automatically when a start fails

For an in-place upgrade, validate the replacement binary, stop the service, replace the existing binary, and restart the service. If the executable path changes, reinstall with `-ForceReinstall` so NSSM's persisted definition is checked again.

Run `tests\Test-ServiceInstaller.ps1` for safe definition tests. An elevated `tests\Test-WindowsServiceLifecycle.ps1` performs a complete temporary create/start/health/restart/stop/remove canary.

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
ExecStart=/opt/ping_monitor/pingmonitor --config /opt/ping_monitor/config.psd1 --endpoints /opt/ping_monitor/endpoints.csv --ui-listen 0.0.0.0:8080
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
		<string>0.0.0.0:8080</string>
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
