# Ping Monitor for Splunk

Enterprise network availability monitoring with intelligent asset correlation.

## Version 2.0.0

## Quick Start

1. **Install the App**: Upload `ping_monitor-2.0.0.tar.gz` via Splunk Web → Manage Apps → Install from File
2. **Run Setup**: Navigate to **Ping Monitor → Setup** and configure your index/sourcetype
3. **Start Monitoring**: The ping monitor script will send data, and dashboards will display it automatically

## Features

- **Zero-Config Dashboard**: Dashboards read settings from KV store - no XML editing required
- **First-Run Setup Wizard**: Configure index/sourcetype via UI on first launch
- **Real-time Ping Monitoring**: Track endpoint availability and latency
- **Asset Discovery**: Automatically discover related data sources in Splunk
- **Health Correlation**: Enrich your data with ping health status
- **Built-in Alerts**: Pre-configured alerts for down endpoints, packet loss, and high latency

## Dashboards

1. **Ping Monitor Overview** - Main dashboard with availability, latency, and status
2. **Asset Discovery** - Find indexes/sourcetypes containing your monitored assets
3. **Asset Health Correlation** - Enrich other data sources with ping health
4. **Setup** - First-run configuration wizard

## Configuration

Settings are stored in KV Store (`ping_monitor_settings` collection):

| Setting | Description | Default |
|---------|-------------|---------|
| `index` | Events index | `ping` |
| `sourcetype` | Ping monitor sourcetype | `ping_monitor` |
| `metrics_index` | Metrics index (if using metrics mode) | `ping_metrics` |

### Change Settings

1. Navigate to **Ping Monitor → Setup**
2. Enter your index, sourcetype, and metrics index
3. Click the search button to save

Or directly via search:
```spl
| makeresults 
| eval _key="global", index="your_index", sourcetype="ping_monitor", metrics_index="ping_metrics", configured="true"
| outputlookup ping_monitor_settings_lookup append=false key_field=_key
```

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
