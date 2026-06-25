# Splunk Ping Monitor

Enterprise-grade network availability monitoring for Splunk with a primary Go runtime, an embedded local admin UI, and direct support for file, HEC, and metrics-based output.

## Latest Published Release

- Go runtime: `v5.3.1`
- Splunk app: `2.7.6` build `34`
- Current runtime release notes: [RELEASE_NOTES_v5.3.1.md](RELEASE_NOTES_v5.3.1.md)
- Current Splunk app release notes: [RELEASE_NOTES_splunk_app_2.7.6.md](RELEASE_NOTES_splunk_app_2.7.6.md)
- Historical version details: [past_versions.md](past_versions.md)

## Current Runtime Options

| Runtime | Status | Platforms | Config |
|---------|--------|-----------|--------|
| Go v5.3.1 | Primary runtime | Windows, Linux, macOS | `config.psd1` preferred; `config.yaml` and `config.json` supported as fallbacks |
| `ping_monitor.sh` v2.0.0 | Supported alternate Unix runtime | POSIX shell environments | `config.conf` |

The top-level README now describes the current published release only. Older PowerShell generations, earlier Go milestones, and archived changelog entries live in [past_versions.md](past_versions.md).

## What The Current Release Includes

- Embedded local admin UI for live endpoint CRUD, discovery, dev/prod marking, settings editing, and HEC connectivity tests.
- Drop-in reuse of existing deployment files when the runtime starts next to `config.psd1` and `endpoints.csv`.
- Automatic `endpoints.csv` hot reload between monitoring cycles with last-known-good protection on invalid edits.
- Dev/test endpoint segmentation with dedicated Prod Devices and Dev Devices dashboards that keep platform and pool-specific views separate.
- Cross-platform Go release binaries for Windows amd64, Linux amd64/arm64, and macOS amd64/arm64.

## Quick Start

### Reuse An Existing Deployment

If you already have a deployment folder with `config.psd1` and `endpoints.csv`, place the current Go binary in that same folder and start it there. The runtime will pick up the co-located files automatically.

Windows:

```powershell
.\pingmonitor.exe --ui-listen 127.0.0.1:8080
```

Linux or macOS:

```bash
chmod +x ./pingmonitor
./pingmonitor --config ./config.psd1 --endpoints ./endpoints.csv --ui-listen 127.0.0.1:8080
```

Open `http://127.0.0.1:8080` to manage the live deployment.

### Useful Runtime Flags

| Flag | Purpose |
|------|---------|
| `--config` | Path to `config.psd1` (preferred), `config.yaml`, or `config.json` |
| `--endpoints` | Path to `endpoints.csv` |
| `--ui-listen` | Bind address for the local admin UI |
| `--ui-only` | Start the UI without starting the monitoring engine |
| `--run-once` | Run a single cycle and exit |
| `--max-cycles` | Stop after a fixed number of cycles |
| `--ping-mode` | Override `ping.mode` with `auto`, `raw`, or `exec` |
| `--version` | Print the runtime version |

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
ip,hostname,group,description,entitytype,device,vendor,additional_notes,dev
192.168.1.1,router,network,Core Router,infrastructure,router,Cisco,Primary site,false
10.0.0.50,app-server,servers,Production App,server,vm,VMware,Critical,false
10.0.50.10,lab-api,dev,Dev API Node,service,vm,VMware,Excluded from production summary stats,true
8.8.8.8,google-dns,external,Google DNS,external,dns,Google,Baseline,false
```

Endpoint file rules:

- Legacy two-column files (`ip,hostname`) are still accepted.
- The optional `dev` flag must remain the trailing column when present.
- `dev=true` endpoints emit `record_type=summary_dev` and, when enabled, `record_type=ping_dev`.
- Production rollups stay on `record_type=summary`, so dev/test systems do not skew customer-facing availability.

## Hot Loading And The Local Admin UI

The current Go runtime separates endpoint hot reload from engine configuration loading:

- `endpoints.csv` is checked between cycles and reloaded automatically when the file changes.
- Invalid endpoint edits do not replace the active set; the runtime keeps the last known good endpoint list until the file is corrected.
- The embedded UI loads the active deployment files at startup, so an existing deployment can be managed in place without re-entering configuration.
- Endpoint edits made in the UI are written back to the live endpoint file that the runtime hot reloads.
- Config edits made in the UI are saved directly to the active config file, but engine-level settings are loaded at process start. Restart the runtime or service after config changes that should affect monitoring behavior.
- When the UI saves config or endpoints over an existing file, it creates a timestamped `.bak` backup first.

The UI supports:

- full endpoint CRUD
- bulk dev/prod changes
- discovery with merge or overwrite workflows
- HEC event and metrics endpoint test actions
- settings help modals for the runtime configuration surface

If you only want to edit files without running the monitor:

```powershell
.\pingmonitor.exe --ui-listen 127.0.0.1:8080 --ui-only
```

## Current Settings Overview

| Group | Key Examples | Purpose |
|-------|--------------|---------|
| Core cycle | `pings_per_cycle`, `cycle_interval_seconds`, `timeout_ms`, `parallel_threads` | Controls ping count, cycle cadence, timeout, and concurrency |
| Event volume | `emit_individual_pings` | Keeps per-ping events on or off while summary events always remain |
| Output and logging | `output_mode`, `log_path`, `log_rotation_size_mb` | Chooses file, HEC, or both and controls local log output |
| Ping engine | `ping.mode` | Selects `auto`, `raw`, or `exec` |
| Diagnostics and debug | `diagnostics.enabled`, `diagnostics.handle_probe_mode`, `debug.emit_memory_stats` | Enables runtime troubleshooting and memory instrumentation |
| HEC events | `hec.enabled`, `hec.url`, `hec.token`, `hec.index`, `hec.sourcetype`, `hec.retry.*`, `dead_letter_path` | Controls direct event delivery, retry behavior, and optional dead-letter output |
| Metrics | `metrics.enabled`, `metrics.mode`, `metrics.index`, `metrics.hec_url`, `metrics.token`, `metrics.compat_mode`, `metrics.use_metrics_index` | Controls metrics delivery and compatibility behavior |

Default/current sample values live in [config.psd1](config.psd1).

## Splunk Output And App Setup

### Output Modes

| Mode | Best For | Notes |
|------|----------|-------|
| `file` | Universal Forwarder or local archival | Writes JSON events to the configured log file |
| `hec` | Direct Splunk ingestion | Uses the `hec` block |
| `both` | Hybrid rollout or migration | Writes file output and HEC together |
| `metrics.mode = "metrics_only"` | Lowest event volume | Skips event summaries and sends metrics only |

### Splunk App

Install the current packaged app from `splunk_app/dist/ping_monitor_2.7.6_build34_20260625.tar.gz`, then:

1. Open **Ping Monitor -> Setup**.
2. Save the events index, sourcetype, and metrics index.
3. Use **Ping Monitor Overview** for whole-platform statistics, **Prod Devices** for current production-only breakdowns, **Dev Devices** for current dev/test devices, and **Asset Health Correlation** for enrichment workflows.

The current app package is AppInspect-validated for this release and includes the separate Prod Devices dashboard plus current-mode Dev Devices membership behavior.

### File-Based Ingestion

If you prefer file output, configure a Splunk monitor input for the runtime log file and parse it as JSON.

## Running As A Service

### Windows (Go Runtime, Shipped Installer)

The repository ships [Install-Service.ps1](Install-Service.ps1), which installs the Go runtime as a Windows service by default.

```powershell
# Install the Go runtime as a Windows service and enable the local UI on 127.0.0.1:8080
.\Install-Service.ps1 -Install -Runtime go

# Bind the UI to a different local port
.\Install-Service.ps1 -Install -Runtime go -UIListen 127.0.0.1:8090

# Disable the local UI for the service
# .\Install-Service.ps1 -Install -Runtime go -DisableUI
```

When the service starts, it uses the same active deployment files as the interactive runtime, so replacing the binary and restarting the service preserves the existing configuration and immediately exposes it in the UI.

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
ExecStart=/opt/ping_monitor/pingmonitor --config /opt/ping_monitor/config.psd1 --endpoints /opt/ping_monitor/endpoints.csv --ui-listen 127.0.0.1:8080
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

*Last updated: 15 June 2026*
