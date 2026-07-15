# Ping Monitor for Splunk 2.9.0 (build 39)

- Uses collector-emitted state as the source of truth for current event and metric health views.
- Displays observation status, state confidence, signal quality, and state source so provisional, invalid, and legacy-inferred results are not presented as confirmed health.
- Adds schema v3 metrics-state support while preserving labeled inference for pre-v3 metrics history.
- Preserves v1/v2 event history through `ping_normalize`, including exact-event deduplication and the legacy per-minute normalization path.
- Uses the schema v3 cadence-derived stale threshold for current event and metric health, with a 120-second fallback only for older history that lacks the field.
- Updates current-health reports, endpoint-down review, overview, prod/dev, and asset-correlation dashboards to avoid reconstructing current state from packet loss.
- Passes local AppInspect precert validation with 0 errors, 0 failures, 4 environment/informational warnings, and 103 successful checks.

Package: `splunk_app/dist/ping_monitor_2.9.0_build39_20260715.tar.gz`
