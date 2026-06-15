# Ping Monitor for Splunk

Enterprise network availability monitoring with native Splunk dashboards, KV Store-backed setup, and Cloud-ready packaging.

## Version 2.7.5

### What's New in v2.7.5
- **Production devices dashboard**: Added a dedicated Prod Devices page immediately to the right of Ping Monitor Overview so production-only health can be reviewed separately from the whole-platform overview
- **Prod/dev parity**: Prod Devices and Dev Devices now share the same device-pool breakdown tiles, including endpoint count, availability, average latency, packet-loss count, health table, and trend charts
- **Overview clarified**: Ping Monitor Overview remains the whole-platform view rather than a production-only slice
- **Current-mode dev dashboarding**: The Dev Devices dashboard follows each endpoint's latest `dev` state instead of lingering on historical `summary_dev` events after a device is moved back to production
- **Runtime guidance refresh**: Updated operator documentation for the Go v5.3.1 service flow, local admin UI port selection, and drop-in reuse of existing `config.psd1` and `endpoints.csv` files
- **Dev devices support**: Added a dedicated Dev Devices dashboard and dev-specific summary stream (`record_type=summary_dev`) so development/test endpoints are visible without skewing production stats
- **Native-light dashboard refresh**: Removed the custom dark presentation and aligned the Overview, Setup, and Asset Correlation views with standard Splunk Web styling
- **Splunk Cloud hardening**: Reworked searches and packaging for Cloud compatibility, including KV Store-backed health state, Cloud-safe metadata, and app reload triggers for custom config
- **AppInspect precert validation**: `ping_monitor_2.7.5_build33_20260615.tar.gz` passes AppInspect precert with 0 errors, 0 failures, 4 warnings, and 103 successful checks. The warnings remain the expected Windows-host capability checks plus the informational `collections.conf` notice.

## Quick Start

1. **Install the App**: Upload the packaged archive from `splunk_app/dist/` via Splunk Web → Manage Apps → Install from File. Current release artifact: `ping_monitor_2.7.5_build33_20260615.tar.gz`
2. **Run Setup**: Navigate to **Ping Monitor → Setup** and configure your events index, sourcetype, and metrics index
3. **Start Monitoring**: Start the Go runtime service or process, and dashboards will display data automatically

## Paired Go Runtime

This Splunk app is intended to pair with the current Go runtime release, `v5.3.1`.

- Drop `pingmonitor.exe` into an existing deployment folder and it will pick up the co-located `config.psd1` and `endpoints.csv` on startup.
- If the embedded admin UI is enabled, those active files are loaded into the UI immediately so the existing deployment can be managed without rebuilding configuration by hand.
- The local admin UI bind can be chosen with `--ui-listen` on direct launches or `-UIListen` when installing the Windows service.

## Features

- **Zero-Config Dashboard**: Dashboards read settings from KV Store - no XML editing required
- **Setup Page**: Save the events index, sourcetype, and metrics index from the in-app configuration page
- **Real-time Ping Monitoring**: Track endpoint availability and latency
- **Health Correlation**: Enrich your data with ping health status from a KV Store-backed lookup
- **Built-in Alerts**: Pre-configured alerts for down endpoints, packet loss, and high latency
- **Splunk Cloud-Ready Package**: AppInspect-clean packaging and app metadata for Cloud deployment

## Dashboards

1. **Ping Monitor Overview** - Whole-platform dashboard across production and dev/test devices
2. **Prod Devices** - Dedicated production-only view for devices whose latest mode is not dev
3. **Dev Devices** - Dedicated dev/test view for endpoints currently flagged as development/test
4. **Asset Health Correlation** - Enrich other data sources with ping health
5. **Setup** - In-app configuration page for events and metrics data sources

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
