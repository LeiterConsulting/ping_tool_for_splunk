# Ping Monitor Splunk App 3.1.0 Release Notes

Build: `43`

Paired collector: Ping Monitor `v5.10.0`

## Discovery Inventory

- Adds a dedicated CMDB-oriented dashboard for `discovery_observation` records.
- Shows each IP's first and last observed time, latest applicable scan evidence, target network, FQDN, DNS-forward confirmation, discovery latency, enrichment, and scan identity.
- Adds scan-history reporting from `discovery_scan_summary` records, including observed, new, missing, unchanged, duration, schedule, and baseline status.
- Keeps discovery evidence separate from monitored health. “Not observed” never becomes Down, removal, or decommissioning.

## Compatibility

- Existing monitoring macros, dashboards, reports, alerts, and historical searches keep their existing record-type filters.
- New discovery macros are additive and use the event index and sourcetype already configured in Setup.
- Existing schema-v1 through schema-v4 ping history remains searchable without reindexing.
- The app remains installable over 3.0.0 while retaining local Setup/KV Store configuration.

## Required Collector

Discovery dashboards require Ping Monitor v5.10.0 or later to emit discovery event evidence. Earlier collector data and normal monitoring dashboards remain supported.
