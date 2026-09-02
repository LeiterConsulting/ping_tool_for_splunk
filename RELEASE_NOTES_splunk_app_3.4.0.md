# Ping Monitor Splunk App 3.4.0 Release Notes

Build: `46`

Paired collector: Ping Monitor `v6.0.0`

Status: released 2026-09-02

## Truthful Current Signal

- Current metrics searches use `ping.observed_at_epoch`, emitted by the collector for every observation, rather than treating a five-minute `mstats` bucket boundary as an exact last-seen time.
- Latest state, observation validity, successful-ping count, packet loss, and latency are normalized together. Latency is cleared whenever the newest observation cannot support it, so old RTT evidence cannot appear beside a newer Down or probe-error state.
- Pre-v6 metric history remains searchable through an explicitly labeled legacy bucket upper-bound fallback. No historical reindex is required.

## Correlation Lookup Reliability

- `Ping Monitor - Update Health Lookup` is enabled by default and runs every minute, keeping lookup age inside the normal two-cycle freshness policy.
- Lookup rows include health state, observation status, state confidence, signal quality, state source, freshness source, latency validity, exact last-seen epoch, and stale threshold.
- Setup shows whether the lookup is current, empty, or stale. Packaged alert searches remain disabled until an administrator deliberately enables them.

## Discovery Performance

- Discovery Inventory now runs one scan-summary base search and one inventory base search, then reuses those results for the target selector, tiles, tables, and charts.
- The default evidence window is seven days. Operators can still expand it when historical review is intentional.
- Existing discovery observations and scan summaries remain compatible.

## Overview Performance And Current-State Semantics

- The default Events view scans and normalizes the selected historical window once, then reuses that result across its twelve summary visualizations. This avoids exhausting Splunk's historical-search concurrency with one full scan per panel.
- The individual-ping table remains an independent search because it uses a different record type.
- “Hosts with Issues” now evaluates each endpoint's newest coherent, freshness-aware observation instead of counting any packet-loss sample anywhere in the selected window as a current issue.

## Release-Candidate Validation — 2026-09-02

- The exact packaged archive passed Splunk AppInspect precert with 0 errors, 0 failures, 0 future failures, 5 non-blocking warnings, and 102 successful checks. The warnings are Windows-host limitations, the intentional one-minute lookup cadence, the informational KV Store collection notice, and the Windows symlink check limitation.
- The package was installed in Splunk Enterprise 10.0.1 without a restart requirement.
- Overview, Prod Devices, Dev Devices, CMDB Inventory, Discovery Inventory, Asset Health Correlation, and Setup rendered in a signed-in browser without search failures or unresolved configuration tokens.
- A live Overview reload used one active `overview_events_base` historical search for its twelve summary panels; all thirteen visible event-mode panels completed successfully.
- Current Health Status, Metrics Summary, Endpoint Down Review, High Packet Loss Review, High Latency Review, and Health Lookup Status all completed successfully against live data.
- The health lookup contained all 78 endpoints using exact collector observation time, with no stale rows and no Down row carrying latency. A synchronized event/metric comparison matched 78 of 78 endpoint states.

The app is published with Ping Monitor v6.0.0 release assets.
