# Ping Monitor v5.10.0 Release Notes

Release date: July 27, 2026

## Discovery Operations And CMDB Evidence

- Adds an indexed discovery-history API and an operator-facing Discovery Operations view.
- Shows configured weekly schedules, due/catch-up state, last success, last attempt, and the most recent error.
- Lists retained scans with target, source schedule, observation time, duration, and new/missing/unchanged counts.
- Loads All, New, or Missing evidence from a retained scan into the existing review, CSV export, and explicit endpoint-import workflow.
- Exports discovery results as RFC 4180-compatible CSV with scan provenance, DNS/FQDN evidence, observation status, and spreadsheet-formula neutralization.
- Preserves DNS status, forward confirmation, discovery time, scan ID, source, and discovery latency when reviewed endpoints are saved and reloaded.
- Adds `discovery.retention_scans` and `discovery.retention_days`, plus automatic history pruning and an advisor warning when both limits are disabled.
- Emits durable `discovery_scan_summary` and `discovery_observation` events through the configured file/HEC output pipeline.
- Pairs with Splunk app 3.1.0, which adds a Discovery Inventory dashboard and additive discovery macros.

## Truth In Signal

- Dev remains classification only and does not suppress probes.
- `monitoring_enabled=false` and active maintenance windows remain the endpoint-level suppression controls.
- A missing discovery address is explicitly described as “not observed in this scan,” not down, removed, or decommissioned.
- Discovery evidence does not include a health `state` or synthetic packet loss.
- FQDN and forward-confirmed DNS remain correlation evidence rather than authoritative asset identity.
- Scheduled discovery remains review-only and never mutates monitored inventory automatically.
- Discovery event evidence is intentionally suppressed in `metrics_only` mode; the advisor explains that `dual` or event output is required for the Splunk Discovery Inventory.

## Reliability And Scale

- Shares one serialized output manager between monitoring and discovery so file rotation, HEC delivery, and the durable outbox use the same ordering and recovery guarantees.
- Keeps completed discovery history locally even if configured event-output acceptance fails.
- Preserves the last-known-good output manager across controlled collector restarts.
- Validates exact 50 MB file rotation without record loss.
- Validates retained discovery history at 73,728 observations across 36 scans.

## Operator Guidance

- Adds contextual information dialogs for every configuration field.
- Adds contextual help for endpoint policy, discovery controls/results/history, and advisor evidence.
- Documents defaults, accepted values, performance tradeoffs, compatibility behavior, and signal semantics.
- Adds advisor guidance when discovery evidence cannot reach Splunk or history retention is unbounded.

## Compatibility

- Existing PSD1, JSON, YAML, and endpoint CSV files remain valid.
- Missing discovery-retention settings preserve the existing unbounded behavior and receive an advisor warning.
- New versioned configurations default to 365-day discovery-history retention.
- Existing endpoint CSVs do not require discovery-evidence columns.
- Existing monitoring event schemas, searches, and Splunk dashboards remain supported.
- Dev/prod and maintenance semantics are unchanged.

## Release Validation

- Full Go test suite and `go vet`.
- Full Go race-enabled test suite.
- Windows service installer validation.
- Endpoint discovery-evidence round-trip tests.
- Discovery history index, delta, retention, schedule status, list, detail, and scale tests.
- Exact-threshold log-rotation test with record-count validation.
- Automated contextual-help coverage for all 69 configuration fields and all static titled panels.
- CSV escaping, provenance, and spreadsheet-formula safety tests.
- Live browser validation of discovery history, contextual help, CSV export completion, and a clean browser console.
- Live bounded loopback discovery with persisted history and baseline-delta validation.
- Live file/HEC delivery of discovery evidence with a healthy, empty durable outbox.
- Live Splunk 3.1.0 install, discovery macro/dashboard registration, discovery inventory searches, and legacy monitoring searches.
- Splunk AppInspect precert: 0 errors, 0 failures, 0 future failures, 4 expected warnings, and 103 successful checks.

## Release Artifacts

- Windows amd64 executable and ready-to-extract ZIP.
- Linux amd64 and arm64 executables.
- macOS amd64 and arm64 executables.
- Splunk app 3.1.0 build 43 package.
- SHA-256 checksum manifest.

## Environment-Dependent Validation

The non-elevated release workstation cannot perform the final create/start/stop/delete Windows Service lifecycle. The installer validation suite passes, and service lifecycle testing remains the one administrator-context deployment check to repeat on a clean Windows host before broad rollout.
