# Splunk Ping Monitor

Enterprise-grade network availability monitoring for Splunk with a primary Go runtime, an embedded local admin UI, and direct support for file, HEC, and metrics-based output.

## Latest Published Release

- Go runtime: `v5.7.2`
- Splunk app: `2.9.2` build `41`
- Current runtime release notes: [RELEASE_NOTES_v5.7.2.md](RELEASE_NOTES_v5.7.2.md)
- Current Splunk app release notes: [RELEASE_NOTES_splunk_app_2.9.2.md](RELEASE_NOTES_splunk_app_2.9.2.md)
- Historical version details: [past_versions.md](past_versions.md)

## Current Runtime Options

| Runtime | Status | Platforms | Config |
|---------|--------|-----------|--------|
| Go v5.7.2 | Primary runtime | Windows, Linux, macOS | `config.psd1` preferred; `config.yaml` and `config.json` supported as fallbacks |
| `ping_monitor.sh` v2.0.0 | Supported alternate Unix runtime | POSIX shell environments | `config.conf` |

The top-level README now describes the current published release only. Older PowerShell generations, earlier Go milestones, and archived changelog entries live in [past_versions.md](past_versions.md).

## What The Current Release Includes

- Configuration Advisor with multi-error inventory validation, deterministic worst-case schedule modeling, operating profiles, revision-safe fixes, and a bounded non-SLA host benchmark.
- Versioned, non-cacheable admin UI assets so browser sessions cannot mix an upgraded API with stale controls.
- Operator-focused embedded admin UI with live collector/cycle/delivery truth, revision-safe endpoint and config editing, discovery, dev/prod marking, and HEC connectivity tests.
- Drop-in reuse of existing deployment files when the runtime starts next to `config.psd1` and `endpoints.csv`.
- Automatic `endpoints.csv` hot reload between monitoring cycles with last-known-good protection on invalid edits.
- Dev/test endpoint segmentation with dedicated Prod Devices and Dev Devices dashboards that keep platform and pool-specific views separate.
- Cross-platform Go release binaries for Windows amd64, Linux amd64/arm64, and macOS amd64/arm64.

## Quick Start

### Reuse An Existing Deployment

If you already have a deployment folder with `config.psd1` and `endpoints.csv`, place the current Go binary in that same folder and start it there. The runtime will pick up the co-located files automatically.

Windows:

```powershell
.\pingmonitor.exe --ui-listen 0.0.0.0:8080
```

Linux or macOS:

```bash
chmod +x ./pingmonitor
./pingmonitor --config ./config.psd1 --endpoints ./endpoints.csv --ui-listen 0.0.0.0:8080
```

Open `http://<collector-address>:8080` to manage the live deployment.

### Useful Runtime Flags

| Flag | Purpose |
|------|---------|
| `--config` | Path to `config.psd1` (preferred), `config.yaml`, or `config.json` |
| `--endpoints` | Path to `endpoints.csv` |
| `--ui-listen` | Bind address for the local admin UI |
| `--ui-only` | Start the UI without starting the monitoring engine |
| `--validate` | Validate config, inventory, and worst-case scheduler capacity without probing or creating runtime state |
| `--run-once` | Run a single cycle and exit |
| `--max-cycles` | Stop after a fixed number of cycles |
| `--ping-mode` | Override `ping.mode` with `auto`, `raw`, or `exec` |
| `--version` | Print the runtime version |

### Configuration Advisor

Version 5.7 uses the same deterministic schedule planner for startup admission, service preflight, CLI analysis, and the web UI. It reports all detectable issues in one pass, including duplicate targets or IDs, missing octets, invalid addresses and booleans, whitespace normalization, incomplete output settings, signal-quality risks, and queue-free worst-case capacity.

```powershell
# Read-only analysis; exits nonzero when blockers exist
.\pingmonitor.exe analyze --profile current

# Preview a recommended profile without changing files
.\pingmonitor.exe optimize --profile standard

# Apply only unambiguous inventory cleanup, with timestamped backup
.\pingmonitor.exe optimize --profile standard --apply-safe

# Apply the previewed profile; restart the collector afterward
.\pingmonitor.exe optimize --profile sla --apply-profile

# Measure planner/filesystem overhead and the configured backend against loopback only
.\pingmonitor.exe benchmark --profile current

# List standard, SLA, high-latency, large-inventory, low-resource, and current profiles
.\pingmonitor.exe profiles
```

The advisor never guesses a malformed IP address. Exact duplicate rows with identical normalized metadata are safe-fixable; conflicting duplicates and incomplete addresses remain blockers for operator review. Profile application is blocked until ambiguous inventory problems are resolved, because an incomplete inventory cannot produce a truthful worker recommendation. The benchmark probes only `127.0.0.1` to verify the configured ping backend and any fallback; it never pings monitored devices.

## Configuration Model

### Go Runtime Configuration

- `config.psd1` is the preferred configuration file for `pingmonitor.exe`.
- `config.yaml` and `config.json` are supported fallback formats for the Go runtime.
- If no supported config file exists, the Go runtime and editable UI can initialize a new `config.yaml` automatically.
- Relative paths in `config.psd1` are resolved from the directory containing that config file.
- The embedded UI edits the same active config file the runtime uses.

See the checked-in sample in [config.psd1](config.psd1) for the full current schema.

### Unix Shell Configuration

The alternate shell runtime uses [config.conf](config.conf) and [ping_monitor.sh](ping_monitor.sh). The Go runtime does not load `config.conf`.

### Endpoints (`endpoints.csv`)

Minimal format:

```csv
ip,hostname,dev
192.168.1.1,router,false
10.0.0.50,app-server,false
```

Full format:

```csv
ip,hostname,group,description,entitytype,device,vendor,additional_notes,endpoint_id,dev
192.168.1.1,router,network,Core Router,infrastructure,router,Cisco,Primary site,,false
10.0.0.50,app-server,servers,Production App,server,vm,VMware,Critical,,false
```

Endpoint file rules:

- Legacy two-column files (`ip,hostname`) are still accepted.
- `ip` and `hostname` headers are required; column order is otherwise flexible.
- The `ip` value must be a literal IPv4 or IPv6 address. DNS names, incomplete rows, invalid `dev` values, duplicate canonical IPs, and duplicate endpoint IDs are rejected.
- `endpoint_id` is optional; the runtime derives a stable target-based ID when it is blank.
- `dev=true` endpoints emit `record_type=summary_dev` and, when enabled, `record_type=ping_dev`.
- Production rollups stay on `record_type=summary`, so dev/test systems do not skew customer-facing availability.

## Hot Loading And The Local Admin UI

The current Go runtime separates endpoint hot reload from engine configuration loading:

- `endpoints.csv` is checked between cycles and reloaded automatically when the file changes.
- Invalid endpoint edits do not replace the active set; the runtime keeps the last known good endpoint list until the file is corrected.
- The embedded UI loads the active deployment files at startup, so an existing deployment can be managed in place without re-entering configuration.
- The Overview distinguishes UI-only, active-cycle, next-cycle, endpoint-reload, Splunk-delivery, and durable-outbox state instead of inferring runtime health from file contents.
- Live status polling uses the startup-effective configuration and does not repeatedly invoke PowerShell to parse `config.psd1`.
- Endpoint edits made in the UI are written back to the live endpoint file that the runtime hot reloads.
- Config and endpoint saves carry a file revision. A stale browser draft receives `409 Conflict` instead of silently overwriting a newer disk edit.
- Config edits made in the UI are saved directly to the active config file, but engine-level settings are loaded at process start. Restart the runtime or service after config changes that should affect monitoring behavior.
- When a saved config revision is not active, monitor mode exposes a confirmation-gated **Restart Collector** action. It performs a controlled in-process engine restart, revalidates revisions, and resumes the last known-good configuration if activation fails.
- HEC tokens are write-only in the API. A blank token field preserves the stored token; the UI receives only a configured/not-configured flag.
- When the UI saves config or endpoints over an existing file, it creates a timestamped `.bak` backup first.

The UI supports:

- advisor analysis, current-versus-proposed schedule evidence, safe fixes, confirmed profile application, and a bounded local benchmark
- full endpoint CRUD
- explicitly selected bulk dev/prod and delete actions, with destructive confirmations
- cancellable discovery with host-count preflight plus merge or overwrite workflows
- HEC event and metrics endpoint test actions
- settings help modals for the runtime configuration surface

If you only want to edit files without running the monitor:

```powershell
.\pingmonitor.exe --ui-listen 0.0.0.0:8080 --ui-only
```

## Current Settings Overview

| Group | Key Examples | Purpose |
|-------|--------------|---------|
| Core cycle | `pings_per_cycle`, `cycle_interval_seconds`, `timeout_ms`, `parallel_threads` | Controls ping count, cycle cadence, timeout, and concurrency |
| Event volume | `emit_individual_pings` | Keeps per-ping events on or off while summary events always remain |
| Output and logging | `output_mode`, `log_path`, `log_rotation_size_mb` | Chooses file, HEC, or both and controls local log output |
| Ping engine | `ping.mode` | Selects `auto`, `raw`, or `exec`; Windows uses native ICMP in `auto`/`raw` |
| Health | `health.down_after_failures`, `health.recovery_after_successes`, `health.stale_after_intervals` | Controls state hysteresis and checkpoint freshness |
| Diagnostics and debug | `diagnostics.enabled`, `diagnostics.handle_probe_mode`, `debug.emit_memory_stats` | Enables runtime troubleshooting and memory instrumentation |
| HEC events | `hec.enabled`, `hec.url`, `hec.token`, `hec.index`, `hec.sourcetype`, `hec.retry.*`, `hec.use_ack` | Controls direct event delivery, retry behavior, and optional indexer acknowledgment |
| Metrics | `metrics.enabled`, `metrics.mode`, `metrics.index`, `metrics.hec_url`, `metrics.token`, `metrics.use_metrics_index`, `metrics.use_ack` | Controls metrics delivery and confirmation behavior |
| Durable delivery | `delivery.spool_path`, `delivery.max_spool_bytes`, `delivery.max_envelopes`, `delivery.drain_max_envelopes` | Bounds the fsynced outbox and catch-up work without allowing silent drops |

Default/current sample values live in [config.psd1](config.psd1).

## Signal And Delivery Truth Contract

- `observation_status` describes what the current probe batch observed: reply, partial reply, no reply, or probe error.
- `state` is the collector's hysteretic decision. It changes to down only after `health.down_after_failures` consecutive valid full-loss cycles and recovers only after `health.recovery_after_successes` valid successful cycles.
- `state_confidence` is `pending` during a down/recovery transition, `confirmed` after the threshold is met, and `unknown` for an invalid measurement.
- Packet loss is an observation, not a substitute for state. The Splunk app uses collector state for current v3 health and labels any state inferred from older history.
- Exact latency is emitted only when the selected backend measured RTT. A platform result such as `time<1ms` is represented as censored with `latency_upper_bound_ms=1`; it is counted as a successful reply but excluded from exact min/average/max calculations.
- `probe_elapsed_ms` is diagnostic wall time and is never presented as network RTT.

For network output, a cycle is written atomically to the durable outbox before the background delivery worker contacts Splunk. `/api/status` reports backlog age/count/bytes and a confirmation mode:

- `hec_accepted_only`: Splunk returned a successful HEC response, but indexer acknowledgment is disabled.
- `indexed_acknowledged`: all enabled network sinks confirmed indexing through HEC indexer acknowledgment.
- `mixed`: only some enabled sinks require indexer acknowledgment.

Enable `use_ack` only after [indexer acknowledgment is enabled on the corresponding Splunk HEC token](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/9.0/get-data-with-http-event-collector/about-http-event-collector-indexer-acknowledgment). Failed or unconfirmed batches remain in the outbox across restarts. If the configured capacity is exhausted, the monitor stops accepting new cycles and reports the error rather than deleting signal.

## Splunk Output And App Setup

### Output Modes

| Mode | Best For | Notes |
|------|----------|-------|
| `file` | Universal Forwarder or local archival | Writes JSON events to the configured log file |
| `hec` | Direct Splunk ingestion | Uses the `hec` block |
| `both` | Hybrid rollout or migration | Writes file output and HEC together |
| `metrics.mode = "metrics_only"` | Lowest event volume | Skips event summaries and sends metrics only |

### Splunk App

Install the current packaged app from `splunk_app/dist/ping_monitor_2.9.2_build41_20260715.tar.gz`, then:

1. Open **Ping Monitor -> Setup**.
2. Save the events index, sourcetype, and metrics index.
3. Use **Ping Monitor Overview** for whole-platform statistics, **Prod Devices** for current production-only breakdowns, **Dev Devices** for current dev/test devices, and **Asset Health Correlation** for enrichment workflows.

The current app package is AppInspect-validated for this release and includes the separate Prod Devices dashboard plus current-mode Dev Devices membership behavior.

### File-Based Ingestion

If you prefer file output, configure a Splunk monitor input for the runtime log file and parse it as JSON.

## Running As A Service

### Windows (Go Runtime, Shipped Installer)

The repository ships [Install-Service.ps1](Install-Service.ps1), which installs the Go runtime through NSSM with delayed automatic start, graceful shutdown, application restart, Windows Service Control Manager recovery, and rotating stdout/stderr logs.

Do not register `pingmonitor.exe` directly with `New-Service` or `sc.exe create`. The current Go binary is a long-running console application, not a native Windows Service Control Manager executable, and does not implement the SCM service dispatcher. Use the shipped NSSM installer for a Windows service, or use Task Scheduler when a service wrapper is not desired.

Prerequisites are PowerShell 7.4+, administrator rights for lifecycle mutations, and a vetted NSSM 2.24+ `nssm.exe` either on `PATH` or beside `Install-Service.ps1`. The installer does not download or execute a remote binary automatically.

Run validation first from any PowerShell 7.4+ session. Validation does not change service state and does not require elevation:

```powershell
# Validate a deployment folder and show the exact service definition
.\Install-Service.ps1 -Validate `
  -BinaryPath D:\pingmonitor\pingmonitor.exe `
  -ConfigPath D:\pingmonitor\config.psd1 `
  -EndpointsPath D:\pingmonitor\endpoints.csv
```

Open PowerShell with **Run as administrator** for installation and lifecycle operations:

```powershell
# Install, verify, and start the service. The UI listens on 0.0.0.0:8080 by default.
.\Install-Service.ps1 -Install `
  -BinaryPath D:\pingmonitor\pingmonitor.exe `
  -ConfigPath D:\pingmonitor\config.psd1 `
  -EndpointsPath D:\pingmonitor\endpoints.csv

# Inspect persisted NSSM paths, arguments, log files, process ID, and status
.\Install-Service.ps1 -Status
.\Install-Service.ps1 -Status -Json

# Control the verified service instance
.\Install-Service.ps1 -Stop
.\Install-Service.ps1 -Start
.\Install-Service.ps1 -Restart

# Replace an existing definition after changing paths or service settings
.\Install-Service.ps1 -Install `
  -BinaryPath D:\pingmonitor\pingmonitor.exe `
  -ConfigPath D:\pingmonitor\config.psd1 `
  -EndpointsPath D:\pingmonitor\endpoints.csv `
  -ForceReinstall

# Remove the service without deleting deployment data or logs
.\Install-Service.ps1 -Uninstall
```

The default working directory is the directory containing the selected Go binary. Therefore a service installed from the repository can safely target a separate deployment directory; relative runtime paths continue to resolve beside that deployed binary. Override this with `-WorkingDirectory` or place service output elsewhere with `-LogDirectory`.

By default the service:

- runs as `LocalSystem` through NSSM;
- starts using `AutomaticDelayedStart`;
- restarts the monitored application after 10 seconds;
- has SCM restart actions for failures of the NSSM service process;
- gives the console application 15 seconds to shut down cleanly;
- rotates `service_stdout.log` and `service_stderr.log` at 10 MB and at least daily;
- binds the optional admin UI to `0.0.0.0:8080`.

Use a different address/port or disable the UI when required:

```powershell
# Bind the UI to a different address or port
.\Install-Service.ps1 -Install -UIListen 0.0.0.0:8090

# Run without the embedded UI
.\Install-Service.ps1 -Install -DisableUI
```

Version 5.7 does not require `-AllowRemoteUI` for non-loopback listeners; the switch remains accepted for command-line compatibility. Authentication and access-policy enforcement are deferred to v6 or later, so operators should treat the configured listener as an administrative endpoint.

`-Validate` now runs the full Configuration Advisor preflight and shows all blockers and recommendations before NSSM is changed. Start, restart, and install operations include a startup stabilization check; failures automatically include the recent NSSM stdout/stderr tail when available.

Before an upgrade, validate the replacement binary, stop the service, replace the binary, and restart it. If the executable path changes, use `-ForceReinstall` so the persisted NSSM definition is verified again.

The repository includes two service tests:

```powershell
# Safe definition/path/argument tests; does not require elevation
.\tests\Test-ServiceInstaller.ps1 -BinaryPath D:\pingmonitor\pingmonitor.exe

# Full create/start/health/restart/stop/remove canary; requires elevation
.\tests\Test-WindowsServiceLifecycle.ps1 -BinaryPath D:\pingmonitor\pingmonitor.exe
```

The lifecycle test creates a uniquely named temporary deployment and service, verifies `/healthz`, and removes both in a `finally` block. Do not give it the name of a production service.

### Linux (Go Runtime Under systemd)

Use the published Linux amd64 or arm64 binary, place it in your deployment directory, and run it under `systemd` with the same flags you would use interactively.

Example unit:

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

Then run:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ping_monitor
```

### macOS And Other Unix Platforms

The current Go runtime can be launched under your native service manager with the same `--config`, `--endpoints`, and `--ui-listen` flags shown above.

Example `launchd` plist for the Go runtime on macOS:

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

If you prefer the alternate shell runtime on Unix platforms, the repository ships [install_unix.sh](install_unix.sh), which can create a service/daemon configuration for:

- `systemd`
- `launchd`
- `OpenRC`
- cron-based minimal environments

Run:

```bash
sudo ./install_unix.sh
```

## Platform Support

### Go Runtime

- Windows Server 2016+
- Windows 10/11
- Linux amd64 and arm64
- macOS amd64 and arm64

### Shell Runtime

- Ubuntu and Debian
- CentOS, RHEL, Rocky, AlmaLinux
- Alpine Linux
- macOS
- FreeBSD and OpenBSD
- Raspberry Pi OS
- iSH or other minimal cron-driven environments

## Additional Documentation

- [BEST_PRACTICES.md](BEST_PRACTICES.md)
- [go/README.md](go/README.md)
- [splunk_app/ping_monitor/README.md](splunk_app/ping_monitor/README.md)
- [past_versions.md](past_versions.md)

## License

MIT License.

*Last updated: 20 July 2026*
