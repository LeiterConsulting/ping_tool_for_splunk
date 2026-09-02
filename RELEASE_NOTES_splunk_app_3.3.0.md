# Ping Monitor Splunk App 3.3.0 Release Notes

Build: `45`

Paired collector: Ping Monitor `v5.12.0`

## Native Discovery Evidence

- Discovery Inventory exposes the discovery engine, actual ICMP probe backend, and latency source emitted by the native Go engine.
- Measured latency and censored upper bounds remain distinguishable; values such as Windows `<1 ms` are not converted to zero.
- Scan History shows requested concurrency, effective ICMP/DNS worker counts, hosts considered, hosts probed, observed endpoints, addresses without an ICMP observation, indeterminate probes, and DNS forward-confirmed/PTR-only/unresolved counts.
- A scan with indeterminate probes is prevented from reaching the collector's history/event pipeline, protecting New/Missing calculations from backend failures.

## Historical Compatibility

- Existing monitoring dashboards, reports, alerts, macros, and record-type filters retain their behavior.
- Existing discovery observations remain searchable without reindexing.
- Older observations without probe provenance are labeled `legacy/unknown` rather than being presented as native evidence.
- Older scan summaries naturally leave newly added host-accounting columns blank; their original observed/new/missing/unchanged fields remain available.

## Required Collector

Native discovery evidence requires Ping Monitor v5.12.0. Earlier discovery and monitoring history remains available through the existing compatibility normalization.
