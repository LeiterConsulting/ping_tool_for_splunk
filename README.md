# Splunk Ping Monitor v2.0

**Enterprise-grade network availability monitoring for Splunk — zero dependencies, maximum flexibility.**

A cross-platform ping monitoring tool that sends structured data directly to Splunk for visualization, alerting, and long-term analysis. Designed for airgapped environments and high-scale deployments.

---

## Why This Tool?

| Challenge | Solution |
|-----------|----------|
| **Airgapped networks** | Zero external dependencies — uses only native OS tools |
| **Cross-platform fleet** | Single data model across Windows, Linux, macOS, BSD, containers, and even iOS |
| **High event volume** | Summary-only mode reduces events by up to 80% with no data loss |
| **Long-term retention** | Native Splunk Metrics support for efficient time-series storage |
| **Mixed historical data** | Dual-mode dashboard queries work across events and metrics seamlessly |
| **Custom metadata** | Enrich endpoints with entity type, vendor, device role, and notes |

---

## What's New in v2.0

### 🎯 Flexible Event Volume Control
Choose your output granularity:
- **Full mode**: Every ping attempt + summaries (detailed troubleshooting)
- **Summary-only**: One event per endpoint per cycle (~80% reduction)
- **Metrics-only**: Pure numeric time-series (maximum efficiency)

### 📊 Native Splunk Metrics
Send ping data directly to Splunk's metrics store for:
- Faster `mstats` queries on large datasets
- Efficient long-term retention
- Better performance for dashboards and reports

### 🏷️ Endpoint Enrichment
Add business context to every ping:
```csv
ip,hostname,group,description,entitytype,device,vendor,additional_notes
192.168.1.1,core-rtr,network,Primary router,infrastructure,router,Cisco,Site A
```

All fields propagate to both events and metrics for consistent filtering.

### 🔄 Dual-Mode Dashboard
The included Splunk dashboard automatically works with:
- Historical event data (pre-upgrade)
- New metrics data (post-upgrade)
- Mixed data during transition periods

---

## Editions

| Edition | Platform | Script | Config |
|---------|----------|--------|--------|
| 🪟 **Windows** | PowerShell 7.4+ | `PingMonitor.ps1` | `config.psd1` |
| 🐧 **Unix** | POSIX Shell | `ping_monitor.sh` | `config.conf` |

Both editions produce identical JSON output and support the same features.

---

## Quick Start

### Windows

```powershell
# 1. Edit your endpoints
notepad endpoints.csv

# 2. Test run
pwsh -File .\PingMonitor.ps1 -RunOnce

# 3. Run continuously
pwsh -File .\PingMonitor.ps1
```

### Unix/Linux/macOS

```bash
# 1. Make executable
chmod +x ping_monitor.sh

# 2. Edit your endpoints
nano endpoints.csv

# 3. Test run
./ping_monitor.sh --once

# 4. Run continuously
./ping_monitor.sh
```

---

## Configuration

### Endpoints (endpoints.csv)

**Minimal format:**
```csv
ip,hostname
192.168.1.1,router
10.0.0.50,app-server
```

**Full format with enrichment:**
```csv
ip,hostname,group,description,entitytype,device,vendor,additional_notes
192.168.1.1,router,network,Core Router,infrastructure,router,Cisco,Primary site
10.0.0.50,app-server,servers,Production App,server,vm,VMware,Critical
8.8.8.8,google-dns,external,Google DNS,external,dns,Google,Baseline
```

### Windows (config.psd1)

```powershell
@{
    pings_per_cycle = 4
    cycle_interval_seconds = 60
    timeout_ms = 1000
    parallel_threads = 10
    
    # Event volume control
    emit_individual_pings = $true  # Set to $false for ~80% reduction
    
    # Output mode: "file", "hec", or "both"
    output_mode = "file"
    log_path = "./logs/ping_results.log"
    
    # Splunk HEC (direct ingestion)
    hec = @{
        enabled = $false
        url = "https://splunk:8088/services/collector/event"
        token = "your-token"
        index = "network"
        sourcetype = "ping_monitor"
        verify_ssl = $true
        ssl_protocol = "Tls12"
    }
    
    # Splunk Metrics (high-efficiency storage)
    metrics = @{
        enabled = $false
        mode = "dual"  # "dual" or "metrics_only"
        index = "ping_metrics"
        hec_url = "https://splunk:8088/services/collector"
        token = "your-token"
        verify_ssl = $true
        ssl_protocol = "Tls12"
    }
}
```

### Unix (config.conf)

```bash
PINGS_PER_CYCLE=4
CYCLE_INTERVAL=60
PING_TIMEOUT=2

OUTPUT_MODE="file"  # file, hec, or both
LOG_PATH="./logs/ping_results.log"

# Splunk HEC
HEC_URL="https://splunk:8088/services/collector/event"
HEC_TOKEN="your-token"
HEC_INDEX="network"
HEC_SOURCETYPE="ping_monitor"
HEC_VERIFY_SSL="true"
HEC_TLS_VERSION="1.2"

# Metrics
METRICS_ENABLED="false"
METRICS_MODE="dual"
METRICS_INDEX="ping_metrics"
METRICS_HEC_URL="https://splunk:8088/services/collector"
METRICS_TOKEN="your-token"
```

---

## Output Modes

### Mode Comparison

| Mode | Events/Cycle/Endpoint | Best For |
|------|----------------------|----------|
| **Full** (`emit_individual_pings=true`) | `pings + 1` (e.g., 5 for 4 pings) | Detailed troubleshooting |
| **Summary-only** (`emit_individual_pings=false`) | `1` | Balanced monitoring |
| **Metrics-only** (`metrics.mode="metrics_only"`) | `0` events, metrics only | Maximum efficiency |

### Event Reduction Math

With `pings_per_cycle = 4`:
- Full mode: 5 events per endpoint per cycle
- Summary-only: 1 event per endpoint per cycle
- **Reduction: 80%**

---

## Splunk Integration

### Option A: Universal Forwarder (File-Based)

1. Set `output_mode = "file"` 
2. Configure a monitor input:

```ini
# inputs.conf
[monitor:///opt/ping_monitor/logs/ping_results.log]
sourcetype = ping_monitor
index = network
```

```ini
# props.conf
[ping_monitor]
TIME_FORMAT = %Y-%m-%dT%H:%M:%S.%6N%:z
TIME_PREFIX = "timestamp":"
SHOULD_LINEMERGE = false
KV_MODE = json
```

### Option B: HEC (Direct HTTP)

1. Create an HEC token in Splunk
2. Set `output_mode = "hec"` and configure the `hec` block

**Splunk Cloud URL Formats:**

| Deployment | URL Format |
|------------|------------|
| **AWS** | `https://http-inputs-<instance>.splunkcloud.com:443/services/collector/event` |
| **GCP / Azure** | `https://http-inputs.<instance>.splunkcloud.com:443/services/collector/event` |
| **FedRAMP (GovCloud)** | `https://http-inputs.<instance>.splunkcloudgc.com:443/services/collector/event` |

> Replace `<instance>` with your Splunk Cloud instance name (the subdomain from your Splunk Cloud URL).

**Windows Example (config.psd1):**
```powershell
hec = @{
    enabled = $true
    # AWS Splunk Cloud:
    url = "https://http-inputs-mycompany.splunkcloud.com:443/services/collector/event"
    token = "your-hec-token"
    index = "network_monitoring"
    sourcetype = "ping_monitor"
    verify_ssl = $true
    ssl_protocol = "Tls12"
}
```

**Unix Example (config.conf):**
```bash
# GCP/Azure Splunk Cloud:
HEC_URL="https://http-inputs.mycompany.splunkcloud.com:443/services/collector/event"
HEC_TOKEN="your-hec-token"
HEC_INDEX="network_monitoring"
HEC_SOURCETYPE="ping_monitor"
HEC_VERIFY_SSL="true"
HEC_TLS_VERSION="1.2"
```

**FedRAMP Example:**
```bash
# FedRAMP (GovCloud) Splunk Cloud:
HEC_URL="https://http-inputs.mycompany.splunkcloudgc.com:443/services/collector/event"
```

### Option C: Metrics Index

1. Create a metrics-type index in Splunk
2. Enable the `metrics` block in config
3. Query with `mstats`:

```spl
| mstats avg(ping.avg_latency_ms) WHERE index=ping_metrics BY hostname
```

### Dashboard Installation

1. Copy `splunk/ping_dashboard.xml` to Splunk
2. Replace `YOUR_INDEX`, `YOUR_SOURCETYPE`, `YOUR_METRICS_INDEX` placeholders
3. Install `splunk/macros.conf` for dual-mode queries (see below)

### Macros Installation (Required for Dual-Mode Dashboard)

The macros enable seamless queries across both event and metrics data. **This is required if you're using the dual-mode dashboard or transitioning from events to metrics.**

#### Option 1: Add to Search App (Quick)

```bash
# Linux/macOS
sudo cp splunk/macros.conf $SPLUNK_HOME/etc/apps/search/local/macros.conf

# Windows (PowerShell as Admin)
Copy-Item splunk\macros.conf "$env:SPLUNK_HOME\etc\apps\search\local\macros.conf"
```

#### Option 2: Create Dedicated App (Recommended)

```bash
# Create app directory structure
mkdir -p $SPLUNK_HOME/etc/apps/ping_monitor/local
mkdir -p $SPLUNK_HOME/etc/apps/ping_monitor/default
mkdir -p $SPLUNK_HOME/etc/apps/ping_monitor/metadata

# Copy macros
cp splunk/macros.conf $SPLUNK_HOME/etc/apps/ping_monitor/local/

# Create app.conf
cat > $SPLUNK_HOME/etc/apps/ping_monitor/default/app.conf << 'EOF'
[install]
is_configured = 0

[ui]
is_visible = 1
label = Ping Monitor

[launcher]
author = Your Organization
description = Network availability monitoring with dual-mode support
version = 2.0.0
EOF

# Set permissions
cat > $SPLUNK_HOME/etc/apps/ping_monitor/metadata/local.meta << 'EOF'
[]
export = system
EOF
```

#### Customize Default Index/Sourcetype

Edit the macros to match your environment. The no-argument versions use these defaults:

```properties
# In macros.conf, update these lines:
[ping_summary_events]
definition = `ping_summary_events(your_index, your_sourcetype)`

[ping_summary_metrics]
definition = `ping_summary_metrics(your_metrics_index, 1m)`

[ping_data_union]
definition = `ping_data_union(your_index, your_sourcetype, your_metrics_index, 1m)`
```

#### Verify Installation

After restarting Splunk (or running `| rest /services/admin/macros`), test:

```spl
| `ping_summary_events`
| `ping_data_union`
```

#### Available Macros

| Macro | Arguments | Description |
|-------|-----------|-------------|
| `ping_summary_events` | (index, sourcetype) | Query event-based summaries |
| `ping_summary_metrics` | (metrics_index, span) | Query metrics with mstats |
| `ping_data_union` | (index, sourcetype, metrics_index, span) | Union of events + metrics |
| `ping_latest_status` | — | Latest status per endpoint |
| `ping_loss_trend` | (span) | Packet loss over time |
| `ping_latency_by_group` | — | Average latency by group |
| `ping_endpoints_with_issues` | — | Endpoints with >0% loss (last hour) |

---

## Running as a Service

### Windows (Task Scheduler or NSSM)

```powershell
# Using the included installer
.\Install-Service.ps1 -Install

# Or via Task Scheduler:
# Action: pwsh.exe -NoProfile -File "C:\path\to\PingMonitor.ps1"
```

### Unix (systemd/launchd/OpenRC)

```bash
# Automatic detection of init system
sudo ./install_unix.sh
```

---

## Platform Support

### Windows
- Windows Server 2016+
- Windows 10/11
- Requires PowerShell 7.4+

### Unix/Linux/macOS
| Platform | Status |
|----------|--------|
| Ubuntu/Debian | ✅ Full support |
| CentOS/RHEL/Rocky | ✅ Full support |
| Alpine Linux | ✅ Minimal footprint |
| macOS | ✅ Full support |
| FreeBSD/OpenBSD | ✅ Full support |
| Raspberry Pi OS | ✅ Full support |
| Docker containers | ✅ Alpine recommended |
| iSH (iOS) | ✅ Cron-based |

---

## Configuration Reference

### Core Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `pings_per_cycle` | 4 | ICMP pings per endpoint per cycle |
| `cycle_interval_seconds` | 60 | Seconds between cycles |
| `timeout_ms` | 1000 | Ping timeout (ms) |
| `parallel_threads` | 10 | Concurrent pings (Windows) |
| `emit_individual_pings` | true | Emit per-ping events |

### HEC Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `hec.enabled` | false | Enable HEC output |
| `hec.url` | — | HEC endpoint URL |
| `hec.token` | — | HEC token |
| `hec.index` | main | Target index |
| `hec.verify_ssl` | true | Verify certificates |
| `hec.ssl_protocol` | Default | TLS version |

### Metrics Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `metrics.enabled` | false | Enable metrics output |
| `metrics.mode` | dual | `dual` or `metrics_only` |
| `metrics.index` | — | Metrics index (must be metrics-type) |
| `metrics.hec_url` | — | Metrics HEC URL |

### Endpoint Columns

| Column | Required | Description |
|--------|----------|-------------|
| `ip` | ✅ | IP address to ping |
| `hostname` | ✅ | Friendly name |
| `group` | — | Grouping for filtering |
| `description` | — | Description text |
| `entitytype` | — | Entity classification |
| `device` | — | Device role/type |
| `vendor` | — | Vendor/manufacturer |
| `additional_notes` | — | Free-form notes |

---

## Troubleshooting

### PowerShell Version
```powershell
# Check version (must be 7.4+)
$PSVersionTable.PSVersion

# Install latest
winget install Microsoft.PowerShell
```

### HEC Errors
- Verify URL includes `/services/collector/event` (or `/services/collector` for metrics)
- Check token is enabled and has index access
- For self-signed certs: `verify_ssl = $false`
- For TLS issues: `ssl_protocol = "Tls12"`

### High Memory
Reduce `parallel_threads` when monitoring many endpoints.

---

## File Structure

```
Ping Tool for Splunk/
├── PingMonitor.ps1          # Windows monitoring script
├── ping_monitor.sh          # Unix monitoring script
├── config.psd1              # Windows configuration
├── config.conf              # Unix configuration
├── endpoints.csv            # Target endpoints
├── endpoints_minimal.csv    # Minimal example
├── logs/                    # Output logs (auto-created)
├── splunk/
│   ├── ping_dashboard.xml   # Splunk dashboard
│   └── macros.conf          # Dual-mode query macros
├── Install-Service.ps1      # Windows service installer
└── install_unix.sh          # Unix service installer
```

---

## License

MIT License — free to use, modify, and distribute.

---

**v2.0.0** — Cross-platform ping monitoring with event reduction, Splunk Metrics, and enrichment support.
