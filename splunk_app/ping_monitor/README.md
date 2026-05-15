# Ping Monitor for Splunk

Enterprise network availability monitoring with native Splunk dashboards, KV Store-backed setup, and Cloud-ready packaging.

## Version 2.7.2

### What's New in v2.7.2
- **Native-light dashboard refresh**: Removed the custom dark presentation and aligned the Overview, Setup, and Asset Correlation views with standard Splunk Web styling
- **Splunk Cloud hardening**: Reworked searches and packaging for Cloud compatibility, including KV Store-backed health state, Cloud-safe metadata, and app reload triggers for custom config
- **AppInspect precert validation**: The current release package passes AppInspect precert with no errors or failures; remaining warnings are Windows host capability checks only

## Quick Start

1. **Install the App**: Upload the packaged archive from `splunk_app/dist/` via Splunk Web → Manage Apps → Install from File. Current validated artifact: `ping_monitor_2.7.2_build30_20260515.tar.gz`
2. **Run Setup**: Navigate to **Ping Monitor → Setup** and configure your index/sourcetype
3. **Start Monitoring**: The ping monitor script will send data, and dashboards will display it automatically

## Features

- **Zero-Config Dashboard**: Dashboards read settings from KV Store - no XML editing required
- **First-Run Setup Wizard**: Configure index/sourcetype via UI on first launch
- **Real-time Ping Monitoring**: Track endpoint availability and latency
- **Health Correlation**: Enrich your data with ping health status from a KV Store-backed lookup
- **Built-in Alerts**: Pre-configured alerts for down endpoints, packet loss, and high latency
- **Splunk Cloud-Ready Package**: AppInspect-clean packaging and app metadata for Cloud deployment

## Dashboards

1. **Ping Monitor Overview** - Main dashboard with availability, latency, and status
2. **Asset Health Correlation** - Enrich other data sources with ping health
3. **Setup** - First-run configuration wizard

## Configuration

Settings are stored in KV Store via the lookup `ping_monitor_settings_lookup`:

| Setting | Description | Default |
|---------|-------------|---------|
| `index` | Events index | `ping` |
| `sourcetype` | Ping monitor sourcetype | `ping_monitor` |
| `metrics_index` | Metrics index (if using metrics mode) | `ping_metrics` |

### Change Settings

1. Navigate to **Ping Monitor → Setup**
2. Enter your index, sourcetype, and metrics index
3. Click **Save Configuration**

Or directly via search:
```spl
| makeresults 
| eval _key="global", index="your_index", sourcetype="ping_monitor", metrics_index="ping_metrics", configured=true()
| outputlookup ping_monitor_settings_lookup append=false key_field=_key
```

## Cloud Readiness

- Saved searches and macros avoid REST-based configuration lookups and `map`
- Health correlation state is stored in KV Store instead of a shipped CSV lookup
- The app includes the metadata and reload triggers needed to avoid restart-only updates for custom config changes
- Current precert validation result: 0 errors, 0 failures on the packaged release artifact

## Saved Searches & Alerts

All saved searches automatically read configuration from KV store.

| Search | Schedule | Description |
|--------|----------|-------------|
| Update Health Lookup | Every 5 min | Maintains lookup for correlation |
| Endpoint Down Alert | Every 5 min | 100% packet loss |
| High Packet Loss Alert | Every 5 min | >25% packet loss |
| High Latency Alert | Every 5 min | >200ms latency |
| Daily Availability Report | 8 AM daily | Daily summary |
| Weekly Trend Report | 9 AM Monday | Weekly analysis |

**Note**: Alerts are disabled by default. Enable them in Settings → Saved Searches.

## Requirements

- Splunk Enterprise 8.0+ or Splunk Cloud
- Ping Monitor script sending data via HEC
- KV Store enabled (default in Splunk)

## Support

GitHub: https://github.com/LeiterConsulting/ping_tool_for_splunk
