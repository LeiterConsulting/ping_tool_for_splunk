# Ping Monitor for Splunk

Enterprise network availability monitoring with intelligent asset correlation.

## Version 2.0.0

### Features

- **Real-time Ping Monitoring**: Track endpoint availability and latency
- **Dual-Mode Support**: Works with both events and metrics data
- **Asset Discovery**: Automatically discover related data sources in Splunk
- **Health Correlation**: Enrich your data with ping health status
- **Service Health Analysis**: View health by entity type, group, and vendor
- **Alerting**: Built-in alerts for down endpoints, packet loss, and high latency

### Dashboards

1. **Ping Monitor Overview** - Main dashboard with availability, latency, and status
2. **Asset Discovery** - Find indexes/sourcetypes containing your monitored assets
3. **Asset Health Correlation** - Enrich other data sources with ping health

### Configuration

After installation, navigate to Ping Monitor > Ping Overview and set:
- **Events Index**: Your ping events index (default: main)
- **Sourcetype**: Your ping sourcetype (default: ping_monitor)
- **Metrics Index**: Your metrics index if using metrics mode (default: ping_metrics)

### Saved Searches & Alerts

- **Update Health Lookup** - Runs every 5 minutes to maintain correlation lookup
- **Endpoint Down Alert** - Triggers when endpoints are unreachable
- **High Packet Loss Alert** - Triggers on >25% packet loss
- **High Latency Alert** - Triggers on >200ms latency
- **Daily Availability Report** - Daily summary
- **Weekly Trend Report** - Weekly analysis

### Macros

The app includes macros for flexible queries:
- `ping_data_union(events_index, sourcetype, metrics_index, span)` - Union of events + metrics
- `ping_summaries` / `ping_summaries(index, sourcetype)` - Summary records
- `ping_metrics` / `ping_metrics(metrics_index, span)` - Metrics data
- `ping_asset_inventory` - Asset inventory with health status

### Requirements

- Splunk Enterprise 8.0+ or Splunk Cloud
- Ping Monitor script sending data via HEC

### Support

GitHub: https://github.com/YourOrg/ping-tool-for-splunk
