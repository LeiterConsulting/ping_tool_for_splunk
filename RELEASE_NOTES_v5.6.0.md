# Ping Tool for Splunk - Release v5.6.0

## Truthful operator status

- The Overview now reports the collector mode and uptime, active/next monitoring cycle, last production success/partial/failure counts, endpoint-reload state, Splunk delivery confirmation, and durable-outbox backlog.
- UI-only mode is labeled explicitly and never presented as a running monitor. Config changes show that a restart is required; endpoint changes show when hot reload is still pending.
- Invalid endpoint reloads retain the last known-good inventory and surface the error. Reverting an invalid file to the previous valid revision now reports recovery and clears stale warning state.

## Safe administration workflows

- Config and endpoint APIs use SHA-256 file revisions. Stale browser drafts receive `409 Conflict` and cannot overwrite newer disk changes.
- Reloading from disk and leaving with unsaved changes require confirmation. Config saves explain restart semantics instead of implying that engine settings changed live.
- Bulk actions operate only on checked rows. Destructive endpoint actions require confirmation, while the selected endpoint has a separate delete control.
- Endpoint drafts validate literal IP addresses, duplicate canonical addresses, and required hostnames before save.
- Discovery estimates the scan size before execution, warns on large ranges, supports cancellation, and keeps results staged until the operator saves endpoints.

## Interface quality and performance

- The navigation and information hierarchy now center on Overview, Endpoints, Discovery, and Configuration, with a compact advanced-settings disclosure for lower-frequency options.
- Responsive layouts support narrow screens without hiding table access, while keyboard row selection, selected-tab state, live status regions, and clearer labels improve accessibility.
- Runtime status refreshes independently without reloading or re-rendering the endpoint editor. It uses the startup-effective config instead of reparsing `config.psd1` through PowerShell on every poll, preserving drafts and avoiding recurring helper-process overhead.
- Response hardening adds content-type, framing, referrer, and content-security headers without changing listener reachability.

## Service and listener behavior

- The Windows service installer defaults to `0.0.0.0:8080`, matching the v5.6 lab deployment. It validates non-loopback addresses without requiring `-AllowRemoteUI`; the switch remains accepted for command-line compatibility.
- Authentication, TLS termination, and listener access-policy enforcement remain deferred to v6 or later.
