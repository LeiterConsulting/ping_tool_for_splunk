# Ping Monitor v5.7.0 Release Notes

Release date: 20 July 2026

Version 5.7.0 makes deployment readiness inspectable and actionable. Startup admission, service validation, the CLI, and the embedded UI now use one deterministic worst-case schedule model instead of producing different answers from different surfaces.

## Configuration Advisor

- Reports all detectable config and endpoint issues in one pass instead of stopping at the first invalid CSV row.
- Identifies duplicate canonical IPs, duplicate endpoint IDs, missing or invalid addresses, likely missing IPv4 octets, missing hostnames, invalid `dev` values, and normalizable whitespace.
- Never invents or guesses a malformed address.
- Distinguishes safe, unambiguous cleanup from changes that need operator confirmation.
- Preserves timestamped backups and rejects stale web requests with `409 Conflict` revision protection.
- Blocks profile sizing while invalid or ambiguous inventory rows make the endpoint count untrustworthy.

## Truthful Capacity Model

- Corrects the per-endpoint budget: fully timed-out attempts already consume the inter-attempt spacing, so that spacing is no longer counted twice.
- Models the actual staggered producer and finite endpoint-worker pool rather than relying only on average worker utilization.
- Publishes current, minimum, and headroom-adjusted recommended worker counts plus a minimum interval alternative.
- Uses endpoint-worker terminology; `parallel_threads` controls concurrent endpoint jobs, not a matching number of operating-system threads.
- Keeps runtime admission and operator recommendations on the same planner and adds regression coverage for the reported 166-endpoint workload.

For 166 endpoints, four attempts, a 3,000 ms timeout, and a 60-second interval, the corrected model uses a 12.5-second per-endpoint budget, requires 44 endpoint workers to avoid dispatch backpressure, and recommends 50 for operational headroom. For the standard four-attempt, 1,000 ms profile, the same inventory requires 14 and recommends 20.

## Profiles And Controlled Remediation

Published profiles include Standard Monitoring, SLA / High Confidence, High-Latency WAN, Large Inventory, Low Resource, and Current Signal Semantics. The current profile preserves sampling, timeout, and cadence while right-sizing only worker capacity.

New CLI workflows:

```powershell
.\pingmonitor.exe analyze --profile current
.\pingmonitor.exe optimize --profile standard
.\pingmonitor.exe optimize --profile standard --apply-safe
.\pingmonitor.exe optimize --profile sla --apply-profile
.\pingmonitor.exe benchmark --profile current
.\pingmonitor.exe profiles
```

Add `--format json` for automation. Preview is the default; mutation requires an explicit apply flag.

## Embedded UI

- Adds a dedicated Advisor workspace with readiness, blocker, warning, and safe-fix counts.
- Shows current and proposed schedule evidence side by side.
- Explains every finding, its evidence, and its recommendation.
- Requires confirmation for safe inventory cleanup and profile application.
- Remains usable for recovery analysis even when strict endpoint loading fails.
- Adds a bounded local benchmark clearly labeled as non-SLA evidence. It verifies the configured ping backend with three loopback-only probes, but never pings monitored devices, changes health state, or sends Splunk events.

## Windows Service Reliability

- `pingmonitor.exe --validate` now emits the complete advisor preflight.
- The NSSM installer runs that full preflight before changing service state.
- Install, start, and restart use a stabilization window.
- Failed starts include recent NSSM stderr/stdout output automatically when logs are available.

## Compatibility

- Existing config formats and event/metric schemas are unchanged.
- Historical Splunk data remains searchable and compatible with the current normalization layer.
- The current Splunk app remains version 2.9.2 build 41; v5.7.0 does not require a dashboard migration.
- The admin UI continues to bind to `0.0.0.0:8080` when requested or installed with defaults. Authentication and access-policy enforcement remain deferred to v6 or later.
