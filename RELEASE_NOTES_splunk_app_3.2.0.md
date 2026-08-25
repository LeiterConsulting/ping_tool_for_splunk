# Ping Monitor Splunk App 3.2.0 Release Notes

Build: `44`

Paired collector: Ping Monitor `v5.11.0`

## Discovery Review Evidence

- Adds Review State filtering and review note/time columns to Discovery Inventory.
- Normalizes durable Needs Review, Deferred, Ignored, and Approved evidence emitted by the collector.
- Labels older discovery history without review fields as `legacy_untracked`; it is not silently presented as approved.
- Keeps unresolved DHCP observations visible without presenting them as durable New or Missing assets.

## CMDB And Alert Policy

- Carries Production/Maintenance/Legacy Dev mode, alert eligibility and reason, Asset ID, dynamic identity, naming-rule provenance, and subnet context.
- Packaged Down, packet-loss, and latency alerts honor `ping.alerting_enabled`; older metrics without that field retain the explicit compatibility default.

## Compatibility

- Existing monitoring macros, dashboards, reports, alerts, and historical searches retain their normal record-type filters.
- Existing schema-v1 through schema-v5 history remains searchable without reindexing.
- Setup/KV Store values remain local and survive an app upgrade.
- Review state saved only in the collector's local registry becomes Splunk-visible with the next matching discovery observation; the collector UI is authoritative immediately.

## Required Collector

The complete review and identity workflow requires Ping Monitor v5.11.0. Earlier collector history remains searchable through the compatibility normalization.
