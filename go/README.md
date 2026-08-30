# Ping Monitor v5 (Go)

Ping Monitor v5.11.0 is the current Go runtime. It adds durable discovery review, DHCP-safe asset reconciliation, bounded local logging, discovery history and scheduling, durable Splunk evidence, explicit monitoring and alert policy, and an opt-in versioned configuration upgrade path.

## Current Go Release

- Version: `v5.11.0`
- Primary runtime status: current and recommended
- Top-level release notes: [../RELEASE_NOTES_v5.11.0.md](../RELEASE_NOTES_v5.11.0.md)
- Historical runtime notes: [../past_versions.md](../past_versions.md)

## What The Go Runtime Includes

- A single binary runtime for Windows, Linux, and macOS.
- Drop-in reuse of existing `config.psd1` and `endpoints.csv` deployment files.
- Automatic `endpoints.csv` hot reload between cycles with last-known-good fallback on invalid edits.
- Embedded admin UI with live runtime truth, revision-safe endpoint/config editing, cancellable discovery, dev/prod marking, and HEC connectivity tests.
- Configuration Advisor with tolerant multi-error CSV inspection, worst-case capacity evidence, operating profiles, safe inventory cleanup, and confirmed config optimization.
- Fsynced, bounded HEC/metrics outbox with asynchronous delivery, restart recovery, per-sink progress, and optional indexer acknowledgment.
- Continuous result-log rotation with archive retention and optional gzip compression.
- Durable indexed discovery snapshots, bounded retention, CSV export, FQDN evidence, actionable target deltas, schedule health, and weekly schedules.
- Scan summaries and truthful observed/not-observed discovery evidence delivered through the same serialized file/HEC event pipeline as monitoring output.
- Probe suppression through explicit monitoring and maintenance policy, independent of dev/prod classification.

## Runtime Compatibility

- Uses existing `config.psd1`, flat JSON/YAML, and `endpoints.csv` fields without requiring migration.
- Emits schema v4 events with stable identities, measured-versus-censored latency, FQDN, and monitoring-policy metadata. Core summary and ping `record_type` values remain compatible.
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
| `--config` | Path to `config.psd1`, `config.json`, `config.yaml`, or `config.yml` |
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

- New deployments initialize grouped schema-v2 `config.json`.
- Existing `config.psd1`, flat `config.json`, `config.yaml`, and `config.yml` remain schema-v1-compatible inputs.
- When multiple default-named files exist, startup preserves the existing resolution order: PSD1, JSON, YAML, YML.
- Relative config paths are resolved from the directory containing the selected config file.
- The schema-v2 reference is [../config.example.json](../config.example.json); [../config.psd1](../config.psd1) remains the legacy-compatible example.

Preview or apply a non-destructive migration:

```powershell
.\pingmonitor.exe config upgrade --config .\config.psd1 --to .\config.json --check
.\pingmonitor.exe config upgrade --config .\config.psd1 --to .\config.json --apply
```

The source is never modified. Point the process or service at the new file only after reviewing it.

### Endpoint File Rules

- `endpoints.csv` accepts legacy two-column files (`ip,hostname`).
- The optional `endpoint_id` column is persisted by the UI; when absent, a deterministic ID is derived from the target IP.
- Optional `fqdn`, `monitoring_enabled`, `maintenance_until`, and `maintenance_reason` columns add identity and monitoring policy.
- Omitted `monitoring_enabled` is enabled. `dev` remains classification only.
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

- URL-backed Overview, Advisor, Endpoints, Discovery, and Settings pages, with separate Runtime, Discovery and CMDB, Splunk Delivery, and Diagnostics settings subpages
- responsive full navigation and compact icon-rail layouts without losing the configured `0.0.0.0:8080` listener behavior
- endpoint CRUD with consolidated bulk Production/Maintenance, alerting, monitoring, and deletion actions
- explicit-selection safeguards for bulk actions and revision conflict protection for saves
- live collector, monitoring-cycle, endpoint-reload, Splunk-delivery, and outbox status
- config editing against the active config file
- cancellable discovery with host-count preflight, FQDN evidence, CSV export, indexed scan history/deltas, a durable Needs Review/Deferred/Ignored registry, and explicitly staged import workflows
- DHCP-safe identity reconciliation against saved endpoints and review records; unique Asset ID is preferred, verified FQDN can correlate an address change, and unidentified dynamic observations remain unresolved instead of being counted as new assets
- Discovery Operations with schedule due/error state and explicit All/New/Missing historical review
- timezone-aware weekly discovery schedules that remain review-only
- monitoring pause/resume and timed maintenance controls
- HEC event and metrics endpoint validation
- contextual help modals for every configuration field and the endpoint, discovery, advisor, and signal-semantics surfaces
- a dedicated Advisor view with readiness counts, current/proposed schedule evidence, findings, change previews, safe fixes, profiles, and a bounded benchmark
- a confirmation-gated Restart Collector action when a saved config revision is not yet active

The browser UI serves these page routes:

- `GET /`, `/advisor`, `/endpoints`, and `/discovery`
- `GET /settings`, `/settings/discovery`, `/settings/splunk`, and `/settings/diagnostics`

The UI serves these key API routes:

- `GET /healthz`
- `GET /api/status`
- `GET` and `PUT /api/endpoints`
- `GET` and `PUT /api/config`
- `GET /api/advisor` and `GET /api/advisor/profiles`
- `POST /api/advisor/apply` and `POST /api/advisor/benchmark`
- `POST /api/runtime/restart`
- `POST /api/discovery/run`
- `GET /api/discovery/history` and `GET /api/discovery/history?scan_id=<id>`
- `POST /api/discovery/reviews`
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
- Discovery observations do not have a metrics representation. Use `metrics.mode=dual` when the Splunk Discovery Inventory dashboard must receive discovery evidence.
- `hec.retry.*` settings define retry behavior.
- Completed HEC/metrics cycles are persisted atomically in `delivery.spool_path` before network delivery. Legacy `drop_on_failure` and dead-letter settings are retained only for config compatibility and do not permit v5.5 to discard a failed cycle.
- `hec.use_ack` and `metrics.use_ack` upgrade delivery confirmation from HEC acceptance to Splunk indexer acknowledgment. `/api/status` states which confirmation mode is active.
- Do not enable `use_ack` until indexer acknowledgment is enabled on the matching Splunk HEC token.

## Build Artifacts

Build all current Go release targets:

- PowerShell: `pwsh -File .\go\build.ps1 -Version v5.11.0`
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

Do not point a manually created `New-Service` or `sc.exe create` definition directly at `pingmonitor.exe`. The Go runtime is a console application and does not implement the native Windows Service Control Manager dispatcher. Use the shipped NSSM installer or Task Scheduler.

The shipped installer:

- installs the Go runtime as the default Windows service target
- defaults the working directory to the selected binary's deployment folder
- verifies persisted NSSM application, arguments, working-directory, and log settings
- uses delayed automatic start, graceful console shutdown, NSSM restart, and SCM recovery actions
- rotates service stdout/stderr logs
- enables the admin UI on `0.0.0.0:8080` by default
- supports `-UIListen` to select a different bind address or port
- accepts loopback and non-loopback UI binds; `-AllowRemoteUI` remains a compatibility no-op in v5.9
- supports `-DisableUI` for a headless service
- supports `-ForceReinstall` when a binary or deployment path changes
- runs the complete advisor during validation and surfaces recent service stdout/stderr automatically when a start fails

When standard filenames are used, the installer can auto-detect the config and `endpoints.csv` in the binary's working directory. Pass an explicit absolute `-ConfigPath` when both a legacy PSD1 and an upgraded JSON file exist. `-DisableUI` disables the HTTP listener but does not disable configured discovery schedules.

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
