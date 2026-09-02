# Splunk Ping Monitor

Enterprise-grade network availability monitoring for Splunk with a primary Go runtime, an embedded local admin UI, and direct support for file, HEC, and metrics-based output.

## Current Release

- Go runtime: `v6.0.0`
- Splunk app: `3.4.0` build `46`
- Runtime release notes: [RELEASE_NOTES_v6.0.0.md](RELEASE_NOTES_v6.0.0.md)
- Splunk app release notes: [RELEASE_NOTES_splunk_app_3.4.0.md](RELEASE_NOTES_splunk_app_3.4.0.md)
- Historical version details: [past_versions.md](past_versions.md)

The latest tagged runtime is `v6.0.0`. The v5.12 native-discovery work was incorporated into v6 rather than published as a separate customer release.

## v6.0.0 Platform Highlights

Version 6 removes PowerShell and NSSM from the default Windows runtime path. The same Go executable now hosts monitoring, manual and scheduled discovery, configuration parsing, the administration UI, and a native Windows Service Control Manager service. Existing PSD1/JSON/YAML configurations, endpoint inventories, discovery history, and explicit PowerShell discovery overrides remain supported.

The new System dashboard exposes observed host and collector CPU, host and process memory, the Go memory limit and garbage-collector state, process handles and threads, and active/configured/peak monitoring, discovery-probe, and DNS workers. OS counters that cannot be measured remain visibly unavailable rather than being reported as healthy zeros. Sampling is cached for two seconds so additional dashboard tabs do not multiply operating-system queries.

## v5.12.0 Native Discovery Highlights

Version 5.12.0 moves manual and scheduled discovery into the Go runtime. Discovery now uses the collector's configured ping family, bounded goroutine workers, millisecond timeout semantics, context cancellation with in-flight platform probes bounded by their configured timeout, a context-aware Go DNS resolver, and direct structured results without a PowerShell child process or temporary CSV. Each observation records its probe backend and measured-versus-censored latency provenance. A backend failure makes the scan indeterminate and blocks missing-device delta calculation instead of turning a measurement failure into false absence.

Existing PSD1/JSON/YAML configurations, endpoint CSVs, discovery history, schedules, and API request shapes remain valid. Native discovery safely limits each scan to 256 ICMP workers and 128 DNS workers; higher legacy concurrency values remain accepted, are clamped at execution, and are exposed alongside the effective worker counts in scan evidence. `DiscoverEndpoints.ps1` remains a standalone compatibility utility, and an explicitly configured `--discovery-script` still selects the deprecated PowerShell compatibility path. Adjacent scripts are ignored by default.

## v5.11.2 Hotfix Highlights

Version 5.11.2 makes the discovery script embedded in the executable authoritative by default. A stale `DiscoverEndpoints.ps1` left beside an upgraded executable can no longer silently shadow the corrected embedded workflow. Advanced operators can still opt into a custom external script with `--discovery-script`; missing explicit overrides fail visibly instead of falling back.

## v5.11.1 Hotfix Highlights

Version 5.11.1 makes discovery adapter selection truthful and route-aware. Explicit subnet scans no longer require a local default gateway; automatic local scans use the lowest effective IPv4 default-route metric, allow one unambiguous isolated IPv4 interface with a warning, and reject ambiguous multi-interface selection with actionable candidate evidence. Existing configurations, endpoints, discovery history, and Splunk app 3.2.0 remain compatible.

## v5.11.0 Release Highlights

Version 5.11.0 adds explicit Production/Maintenance/Legacy Dev modes, independent Alerting Enabled policy, stable Asset IDs and DHCP-aware discovery identity, a durable Needs Review/Deferred/Ignored discovery registry, a discovery subnet catalog, and ordered regex naming-convention pairs with test/preview controls. Existing config files and minimal endpoint CSV files remain valid; legacy `dev=true` continues to ping until an operator explicitly migrates it to Maintenance.

## Current Runtime Options

| Runtime | Status | Platforms | Config |
|---------|--------|-----------|--------|
| Go v6.0.0 | Current release | Windows, Linux, macOS | versioned `config.json` for new deployments; existing PSD1/JSON/YAML files remain supported |
| `ping_monitor.sh` v2.0.0 | Supported alternate Unix runtime | POSIX shell environments | `config.conf` |

The top-level README now describes the current published release only. Older PowerShell generations, earlier Go milestones, and archived changelog entries live in [past_versions.md](past_versions.md).

## What The Current Codebase Includes

- Configuration Advisor with multi-error inventory validation, deterministic worst-case schedule modeling, operating profiles, revision-safe fixes, and a bounded non-SLA host benchmark.
- Live size-based result-log rotation with count/age retention, optional compression, and runtime/advisor evidence.
- Native Go, FQDN-enriched discovery with bounded concurrency, cancellation, CSV export, indexed scan history, actionable target deltas, schedule health, and timezone-aware weekly schedules.
- Durable discovery scan summaries and per-IP evidence through the configured event pipeline, with a dedicated Splunk Discovery Inventory view.
- Independent monitoring policy and maintenance windows that suppress probes explicitly rather than fabricating downtime.
- A versioned config upgrade workflow that leaves legacy files untouched until an operator activates the generated config.
- Versioned, non-cacheable admin UI assets so browser sessions cannot mix an upgraded API with stale controls.
- Operator-focused embedded admin UI with live collector/cycle/delivery truth, revision-safe endpoint and config editing, discovery, dev/prod marking, and HEC connectivity tests.
- A System dashboard with low-overhead observed resource counters and live worker-pool utilization.
- Native Windows SCM service install, validation, status, start, stop, restart, and uninstall commands with bounded service-log rotation and recovery actions.
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
| `--config` | Path to `config.psd1`, `config.json`, `config.yaml`, or `config.yml` |
| `--endpoints` | Path to `endpoints.csv` |
| `--ui-listen` | Bind address for the local admin UI |
| `--ui-only` | Start the UI without starting the monitoring engine |
| `--validate` | Validate config, inventory, and worst-case scheduler capacity without probing or creating runtime state |
| `--run-once` | Run a single cycle and exit |
| `--max-cycles` | Stop after a fixed number of cycles |
| `--ping-mode` | Override `ping.mode` with `auto`, `raw`, or `exec` |
| `--discovery-script` | Deprecated compatibility override that explicitly runs an external `DiscoverEndpoints.ps1`; omission uses native Go discovery |
| `--version` | Print the runtime version |

Native discovery is also available without starting the monitor or web UI:

```powershell
.\pingmonitor.exe discovery `
  --target 192.168.1.0/24 `
  --timeout-ms 500 `
  --concurrency 64 `
  --format csv `
  --output .\discovered_endpoints.csv
```

The command uses the same bounded Go ICMP/DNS engine as the UI. It refuses to overwrite an existing output unless `--force` is explicit and can emit structured JSON with `--format json`.

### Configuration Advisor

Version 5.11 uses the same deterministic schedule planner for startup admission, service preflight, CLI analysis, and the web UI. It reports all detectable issues in one pass, including duplicate targets or IDs, missing octets, invalid addresses and booleans, monitoring-policy errors, invalid maintenance timestamps, discovery-schedule errors, unbounded log retention, incomplete output settings, signal-quality risks, and queue-free worst-case capacity.

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

- New deployments initialize the grouped, versioned schema-v2 format in `config.json`; [config.example.json](config.example.json) is the reference.
- Existing `config.psd1`, flat JSON, and flat YAML files are schema-v1 inputs and remain valid without edits.
- Startup preserves the compatibility resolution order: an existing co-located `config.psd1` is preferred, followed by `config.json`, `config.yaml`, and `config.yml`.
- Relative paths are resolved from the directory containing the active config file.
- The embedded UI edits the same active config file the runtime uses.

Preview a side-by-side migration without changing the active file:

```powershell
.\pingmonitor.exe config upgrade --config .\config.psd1 --to .\config.json --check
```

Use `--apply` when ready. The runtime writes and reload-verifies the target but never switches the service automatically; update the service's `--config` argument after validation. The legacy sample remains available in [config.psd1](config.psd1).

Optional `discovery.subnets` entries pair an IPv4 CIDR with a stable subnet ID, name, VLAN, location, `static` or `dhcp` addressing mode, and routing domain. Existing schedule target arrays remain CIDR strings; the most-specific catalog entry that contains a scan target enriches its discovery and CMDB evidence.

Optional `classification.rules` entries are evaluated in order against Hostname, FQDN, or either. Go RE2 named captures such as `(?P<site>...)` can be referenced in Group, Entity Type, Device, and Vendor assignments as `${site}`. Fill-blank behavior is the default, overwrite is explicit, and the UI can preview exact matches, changes, and rule provenance before applying a draft.

### Unix Shell Configuration

The alternate shell runtime uses [config.conf](config.conf) and [ping_monitor.sh](ping_monitor.sh). The Go runtime does not load `config.conf`.

### Endpoints (`endpoints.csv`)

Minimal format:

```csv
ip,hostname,dev
192.168.1.1,router,false
10.0.0.50,app-server,false
```

Extended inventory formats may add the following optional policy, identity, subnet, and discovery-evidence columns to the legacy fields:

```csv
ip,hostname,fqdn,group,description,entitytype,device,vendor,additional_notes,endpoint_id,asset_id,device_mode,dev,monitoring_enabled,alerting_enabled,alerting_reason,maintenance_until,maintenance_reason,dynamic_address,classification_source,discovery_review_state,discovery_reviewed_at,discovery_review_note,subnet_id,subnet_name,subnet_vlan,subnet_location,addressing_mode,routing_domain,dns_status,dns_forward_confirmed,discovered_at,discovery_scan_id,discovery_source,discovery_latency_ms,discovery_probe_backend,discovery_latency_source,discovery_latency_resolution_ms,discovery_latency_censored,discovery_latency_upper_bound_ms,discovery_probe_elapsed_ms
192.168.1.1,router,router.example.com,network,Edge router,infrastructure,router,Cisco,Core gateway,,CMDB-1001,production,false,true,true,,,,false,rule:network,approved,2026-08-25T12:00:00Z,Approved after discovery review,core,Core Network,100,Headquarters,static,corp,forward_confirmed,true,2026-09-02T12:00:00Z,scan-example,192.168.1.0/24,1.25,windows_icmp,windows_icmp_rtt,1,false,,1.47
```

Endpoint file rules:

- Legacy two-column files (`ip,hostname`) are still accepted.
- `ip` and `hostname` headers are required; column order is otherwise flexible.
- The `ip` value must be a literal IPv4 or IPv6 address. DNS names, incomplete rows, invalid `dev` values, duplicate canonical IPs, duplicate endpoint IDs, and duplicate nonblank Asset IDs are rejected.
- `endpoint_id` is optional; the runtime derives a stable target-based ID when it is blank.
- `fqdn`, operational-policy, CMDB identity, subnet metadata, and discovery-evidence fields are optional.
- Reviewed discovery evidence—including DNS status, scan identity, probe backend, measured/censored latency semantics, resolution, upper bound, and elapsed probe time—survives UI import and endpoint save/load.
- Missing `monitoring_enabled` or `alerting_enabled` means `true`, so old endpoint files keep being monitored and remain eligible for packaged alerts.
- `device_mode=maintenance`, `monitoring_enabled=false`, or a future RFC 3339 `maintenance_until` suppresses probing without fabricating packet loss. Maintenance evidence uses `record_type=monitoring_control` and `measurement_valid=false`.
- `alerting_enabled=false` does not suppress probing, health evaluation, dashboards, or reports. It marks the signal ineligible for packaged Down, packet-loss, and latency alerts.
- Existing `dev=true` rows resolve to `device_mode=legacy_dev` and continue to emit the legacy dev record types. This preserves old behavior until explicit migration.
- `dynamic_address=true` tells discovery delta logic not to treat IP as durable asset identity. Asset ID is preferred, forward-confirmed FQDN is the fallback, and unresolved dynamic observations are retained but excluded from New/Missing asset counts.
- `discovery_review_state`, `discovery_reviewed_at`, and `discovery_review_note` are optional. Old endpoint files remain valid; an endpoint present in the saved monitored inventory is treated as approved discovery identity even when these columns are absent.
- Production rollups stay on `record_type=summary`, so dev/test systems do not skew customer-facing availability.

## Hot Loading And The Local Admin UI

The current Go runtime separates endpoint hot reload from engine configuration loading:

- The admin UI is divided into URL-backed Overview, Advisor, Endpoints, Discovery, System, and Settings pages instead of one anchor-scrolled document. Settings has dedicated Appearance, Runtime, Discovery and CMDB, Splunk Delivery, and Diagnostics subpages.
- Primary actions remain visible while selection, export, reset, test, reorder, and destructive actions use contextual dropdowns or overflow menus. Draft endpoint/config state remains in memory while moving between pages.
- The Endpoints page gives the inventory table the full workspace width and places a collapsible editor below it. A row highlight identifies the single editor target, while independent checkboxes build a multi-device bulk selection; the current editor target and an Open Editor shortcut remain visible above the table.
- Discovery Controls can be collapsed from its visible caret, and Discovery Results uses the full workspace width below it so large review tables are not constrained by a side-by-side layout.
- The navigation collapses to an icon rail on narrower displays, action groups wrap without overflowing, and settings subpage links remain horizontally scrollable at phone widths.
- `endpoints.csv` is checked between cycles and reloaded automatically when the file changes.
- Invalid endpoint edits do not replace the active set; the runtime keeps the last known good endpoint list until the file is corrected.
- The embedded UI loads the active deployment files at startup, so an existing deployment can be managed in place without re-entering configuration.
- The Overview distinguishes UI-only, active-cycle, next-cycle, endpoint-reload, Splunk-delivery, and durable-outbox state instead of inferring runtime health from file contents.
- Live status polling uses the startup-effective configuration and does not repeatedly invoke PowerShell to parse `config.psd1`.
- Endpoint edits made in the UI are written back to the live endpoint file that the runtime hot reloads.
- Config and endpoint saves carry a file revision. A stale browser draft receives `409 Conflict` instead of silently overwriting a newer disk edit.
- Appearance uses the same semantic `data-color-scheme` theme contract as SNMP for Splunk with Ping-specific Signal Blue, Orbit Violet, Radar Coral, Daylight, and High Visibility palettes. Display density and reduced-motion preferences use the same root-level contract.
- Saved appearance preferences live in `ui_preferences.json` beside the deployment. The file is independent of PSD1/JSON/YAML collector configuration, is written with revision protection, takes effect immediately, and never requires a collector restart. If the file is missing or invalid, the UI remains usable with built-in defaults and reports the fallback on the Appearance page.
- Config edits made in the UI are saved directly to the active config file, but engine-level settings are loaded at process start. Restart the runtime or service after config changes that should affect monitoring behavior.
- When a saved config revision is not active, monitor mode exposes a confirmation-gated **Restart Collector** action. It performs a controlled in-process engine restart, revalidates revisions, and resumes the last known-good configuration if activation fails.
- HEC tokens are write-only in the API. A blank token field preserves the stored token; the UI receives only a configured/not-configured flag.
- When the UI saves config or endpoints over an existing file, it creates a timestamped `.bak` backup first.

The UI supports:

- advisor analysis, current-versus-proposed schedule evidence, safe fixes, confirmed profile application, and a bounded local benchmark
- full endpoint CRUD through a full-width inventory and compact four-column desktop editor, with explicit single-device and multi-device selection cues
- explicitly selected bulk Production/Maintenance, alert eligibility, pause/resume, and delete actions, with confirmations for signal-affecting changes
- cancellable native discovery with host-count preflight, structured probe/DNS evidence, complete CSV export, DHCP-safe scan deltas, and merge or overwrite workflows
- durable Needs Review, Deferred, and Ignored discovery queues, explicit Production/Maintenance assignment before staging, per-result Asset ID entry, and bulk review for CMDB fields, address-allocation policy, alert policy, and previewed naming-rule application
- an ordered regex match/assignment builder with named captures, fill-blank/overwrite policy, reordering, sample tests, exact change previews, and classification provenance
- a discovery subnet catalog for subnet name, VLAN, location, routing domain, and static/DHCP behavior
- a Discovery Operations view for schedule health, retained history, and explicit All/New/Missing review
- weekly, timezone-aware discovery schedules with review-only import policy and bounded count/age history retention
- explicit pause/resume monitoring controls and timed maintenance
- HEC event and metrics endpoint test actions
- contextual information modals for every configuration field plus endpoint, discovery, advisor, and signal-semantics controls
- a card-based Appearance page with curated accessible palettes, full-page live preview, compact/comfortable density, motion preferences, explicit save/discard actions, and contextual help

If you only want to edit files without running the monitor:

```powershell
.\pingmonitor.exe --ui-listen 0.0.0.0:8080 --ui-only
```

## Current Settings Overview

| Group | Key Examples | Purpose |
|-------|--------------|---------|
| Core cycle | `pings_per_cycle`, `cycle_interval_seconds`, `timeout_ms`, `parallel_threads` | Controls ping count, cycle cadence, timeout, and concurrency |
| Event volume | `emit_individual_pings` | Keeps per-ping events on or off while summary events always remain |
| Output and logging | `output_mode`, `log_path`, `log_rotation_size_mb`, `log_retention_files`, `log_retention_days`, `log_compress_rotated` | Chooses file, HEC, or both and bounds local log output |
| Ping engine | `ping.mode` | Selects `auto`, `raw`, or `exec`; Windows uses native ICMP in `auto`/`raw` |
| Health | `health.down_after_failures`, `health.recovery_after_successes`, `health.stale_after_intervals` | Controls state hysteresis and checkpoint freshness |
| Diagnostics and debug | `diagnostics.enabled`, `diagnostics.handle_probe_mode`, `debug.emit_memory_stats` | Enables runtime troubleshooting and memory instrumentation |
| HEC events | `hec.enabled`, `hec.url`, `hec.token`, `hec.index`, `hec.sourcetype`, `hec.retry.*`, `hec.use_ack` | Controls direct event delivery, retry behavior, and optional indexer acknowledgment |
| Metrics | `metrics.enabled`, `metrics.mode`, `metrics.index`, `metrics.hec_url`, `metrics.token`, `metrics.use_metrics_index`, `metrics.use_ack` | Controls metrics delivery and confirmation behavior |
| Durable delivery | `delivery.spool_path`, `delivery.max_spool_bytes`, `delivery.max_envelopes`, `delivery.drain_max_envelopes` | Bounds the fsynced outbox and catch-up work without allowing silent drops |
| Discovery | `discovery.history_path`, `discovery.retention_scans`, `discovery.retention_days`, `discovery.schedules` | Stores bounded scan evidence and defines review-only weekly discovery schedules |

Default/current sample values live in [config.psd1](config.psd1).

## Signal And Delivery Truth Contract

- `observation_status` describes what the current probe batch observed: reply, partial reply, no reply, or probe error.
- `state` is the collector's hysteretic decision. It changes to down only after `health.down_after_failures` consecutive valid full-loss cycles and recovers only after `health.recovery_after_successes` valid successful cycles.
- `state_confidence` is `pending` during a down/recovery transition, `confirmed` after the threshold is met, and `unknown` for an invalid measurement.
- Packet loss is an observation, not a substitute for state. The Splunk app uses collector state for current v3 health and labels any state inferred from older history.
- Exact latency is emitted only when the selected backend measured RTT. A platform result such as `time<1ms` is represented as censored with `latency_upper_bound_ms=1`; it is counted as a successful reply but excluded from exact min/average/max calculations.

### Discovery And CMDB Evidence Contract

- Discovery records addresses that replied during a bounded scan. It does not prove ownership, device lifecycle, or a durable asset identity.
- `new` means an address was observed in the selected scan but not in the prior retained scan for the same target.
- `missing` means an address was observed previously but not in the selected scan. It is not proof of downtime, removal, or decommissioning.
- Scheduled results remain review-only. Unknown observations enter the durable **Needs Review** queue without being silently classified as Production. Loading All, New, or Missing places evidence in the discovery review table; only an explicit mode assignment, merge, and **Save Endpoints** changes monitored inventory and makes the review state Approved.
- Deferred, Ignored, and Needs Review decisions are retained in `reviews.json` under the configured discovery history path. They are applied to later scans without rewriting the immutable scan snapshots.
- Discovery reconciles observations against both the review registry and `endpoints.csv`. A unique Asset ID is the preferred canonical identity; a forward-confirmed FQDN can correlate a DHCP device after its IP changes. A dynamic observation with neither remains **Unresolved Dynamic** and is excluded from New/Missing asset counts rather than being falsely declared a new device.
- Completed scans emit `discovery_scan_summary` and `discovery_observation` records through the configured event pipeline. The Splunk **Discovery Inventory** dashboard presents that evidence separately from monitored health.
- Metrics-only mode has no discovery-event equivalent. Local scan history remains available, and the advisor warns operators to select `metrics.mode=dual` when Splunk discovery evidence is required.
- FQDN and DNS-forward confirmation are correlation evidence. DNS can be absent, stale, or reassigned and must not be treated as an authoritative CMDB key.
- ICMP alone cannot establish MAC address, serial number, owner, operating system, or business lifecycle. Enrich those attributes from authoritative discovery and CMDB sources.
- `probe_elapsed_ms` is diagnostic wall time and is never presented as network RTT.
- `monitoring_control` events state why an endpoint was not probed. Their observation is `suppressed`, and Splunk presents the endpoint as Maintenance or Paused rather than Down.

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

Install the current candidate app from `splunk_app/dist/ping_monitor_3.4.0_build46_20260902.tar.gz`, then:

1. Open **Ping Monitor -> Setup**.
2. Save the events index, sourcetype, and metrics index.
3. Use **Ping Monitor Overview** for whole-platform statistics, **Prod Devices** for production-only breakdowns, **Dev Devices** for dev/test devices, **CMDB Inventory** for monitored identity/policy/state, **Discovery Inventory** for CMDB-oriented scan evidence, and **Asset Health Correlation** for enrichment workflows.

The CMDB view reads old summaries and schema-v4 control events together. Historical v1-v3 events remain searchable through the normalization macros.

### File-Based Ingestion

If you prefer file output, configure a Splunk monitor input for the runtime log file and parse it as JSON.

## Running As A Service

### Windows (Native Go Service)

The v6 Windows executable implements the Service Control Manager dispatcher directly. It does not require NSSM or a PowerShell service wrapper. Validation and status are read-only; lifecycle mutations require an elevated terminal.

Validate first from the deployment directory:

```powershell
.\pingmonitor.exe service validate `
  --config .\config.psd1 `
  --endpoints .\endpoints.csv `
  --ui-listen 0.0.0.0:8080
```

Then open PowerShell or Windows Terminal with **Run as administrator**:

```powershell
# Install with delayed automatic start and start immediately.
.\pingmonitor.exe service install `
  --config .\config.psd1 `
  --endpoints .\endpoints.csv `
  --ui-listen 0.0.0.0:8080

# Inspect and control the persisted service.
.\pingmonitor.exe service status --json
.\pingmonitor.exe service restart
.\pingmonitor.exe service stop
.\pingmonitor.exe service start

# Remove only the SCM definition; deployment data and logs remain.
.\pingmonitor.exe service uninstall
```

The install command stores absolute config and endpoint paths, waits for collector readiness, uses delayed automatic start and SCM recovery by default, and rotates `service.log` at 10 MiB with five retained archives. `--disable-ui`, `--startup auto|manual`, `--log-dir`, `--no-start`, and `--wait` customize the definition. Existing relative paths inside the selected config still resolve from that config's directory.

Use `service status --json` before migration. It identifies native v6, NSSM, direct legacy, and other-wrapper definitions. Existing v5 NSSM services remain valid. To migrate, validate the intended v6 paths, stop the old service, and run `service install --force`; the replacement changes only the SCM definition and preserves configuration, endpoint, discovery, and log data. [Install-Service.ps1](Install-Service.ps1) remains the legacy v5/NSSM controller and must not be used to install a second host under the same service name.

The native service has two test layers. The ordinary Go suite tests service readiness, health, and graceful stop in process. The elevated lifecycle test creates a uniquely named canary, verifies install/status/start/restart/PID replacement/stop/logging/uninstall, and removes it in a `finally` block:

```powershell
.\tests\Test-NativeWindowsServiceLifecycle.ps1 -BinaryPath D:\pingmonitor\pingmonitor.exe
```

Never give the canary a production service name.

For the complete customer-facing sequence—including production input validation, isolated canary interpretation, guarded foreground shutdown, clean SCM installation, full-cycle signal verification, maintenance-window restart validation, rollback boundaries, and escalation evidence—use [Validate and Cut Over Ping Monitor v6 to a Native Windows Service](documents/troubleshooting/native-v6-service-validation-and-cutover-0005.md).

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
