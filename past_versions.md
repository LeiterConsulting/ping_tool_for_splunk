# Past Versions

This file archives the historical runtime notes, older release summaries, and changelog entries that were previously mixed into the top-level README.

For the current published release, use [README.md](README.md) and [RELEASE_NOTES_v5.3.1.md](RELEASE_NOTES_v5.3.1.md).

## Historical Runtime Summary

| Runtime | Version | Status | Notes |
|---------|---------|--------|-------|
| Ping Monitor v5 (Go) | `v5.3.0` | Superseded by `v5.3.1` | First embedded local admin UI release |
| Ping Monitor v5 (Go) | `v5.2.1` | Superseded | Endpoint CSV backward-compatibility hotfix |
| Ping Monitor v5 (Go) | `v5.2.0` | Superseded | Dev endpoint routing and first Dev Devices dashboard |
| PowerShell runtime | `v4.0.0` | Legacy supported | Bounded scheduler, HEC hardening, optional dead-letter |
| PowerShell runtime | `v3.3.3` | Legacy supported | Previous stable PowerShell line |
| Shell runtime | `v2.0.0` | Supported alternate runtime | POSIX shell edition with retry, batching, and event ID support |
| Legacy PowerShell runtime | `v1.x` | Deprecated | Retained only for older environments |

## Archived Release Notes

- [RELEASE_NOTES_v5.3.0.md](RELEASE_NOTES_v5.3.0.md)
- [RELEASE_NOTES_v5.2.1.md](RELEASE_NOTES_v5.2.1.md)
- [RELEASE_NOTES_v5.2.0.md](RELEASE_NOTES_v5.2.0.md)

## Historical Feature Notes

### v3.3.x Memory And Handle Optimization

Production hardening across the PowerShell line included:

- reusable `RunspacePool` creation at startup instead of per cycle
- persistent HEC buffering for retry-across-cycles behavior
- metrics batching to reduce POST volume dramatically
- handle leak fixes around async waits and HTTP response streams
- optional PM, WS, GC, and handle diagnostics per cycle

Example diagnostics block from that line:

```powershell
diagnostics = @{
    enabled = $true
    handle_probe_mode = "none"
}
```

### v3.3.x Metrics Compatibility Mode

- `compat_mode=true` preserved the older payload shape by default
- metrics transport was batched at cycle end
- new config keys were added with safe defaults so existing `config.psd1` files continued to work

```powershell
metrics = @{
    enabled = $true
    compat_mode = $true
    batch_size = 100
    max_buffer_events = 5000
    max_buffer_bytes = "5MB"
}
```

### v3.2.x Retry-Safe HEC Batching

- automatic batching with configurable batch size
- retry with fixed or exponential backoff
- memory caps on buffered events and bytes
- drop-newest behavior when the buffer was full

```powershell
hec = @{
    batch_size = 100
    max_buffer_events = 5000
    max_buffer_bytes = "5MB"
    retry = @{
        enabled = $true
        max_attempts = 3
        base_delay_ms = 250
        jitter_pct = 20
        backoff = "exponential"
    }
}
```

Additional v3.2.x details:

- deterministic `event_id` support for `| dedup event_id`
- reduced memory allocation pressure in the streaming loop

### Shell Runtime v2.0.0

The shell edition reached parity with the older PowerShell feature set in several areas:

- HEC batching with retry support
- SHA256 `event_id` generation using `sha256sum`, `shasum`, or `openssl`
- single metrics POST per cycle
- explicit version flags (`--version` and `-V`)

Example shell settings from that milestone:

```bash
HEC_BATCH_SIZE=100
HEC_MAX_BUFFER_EVENTS=5000
HEC_RETRY_ENABLED=true
HEC_RETRY_MAX_ATTEMPTS=3
HEC_RETRY_BASE_DELAY_MS=250
HEC_RETRY_BACKOFF=exponential
```

### v2.x Historical Themes

- full, summary-only, and metrics-only event volume control
- native Splunk metrics support for `mstats`
- endpoint enrichment fields propagated into both events and metrics
- dual-mode dashboard behavior during event-to-metrics transitions
- support for multi-word filters in dashboard tokens

## Archived Changelog

### v5.3.0

- Added an embedded local admin UI to the Go runtime for live endpoint CRUD, discovery, dev-device management, and config editing.
- Exposed editable runtime API surfaces from `pingmonitor.exe`.
- Added HEC event and metrics endpoint validation from the Settings page.
- Preserved drop-in compatibility with existing deployment files.

### v5.2.1

- Restored endpoint CSV backward compatibility by keeping optional `dev` as the final column.
- Kept `dev` parsing optional so older files continued to load unchanged.
- Expanded default build coverage to include Linux arm64 artifacts.

### v5.2.0

- Added optional `dev` endpoint routing in the Go runtime.
- Introduced `record_type=summary_dev` and `record_type=ping_dev` for dev/test endpoints.
- Added the first Splunk app Dev Devices dashboard.
- Updated Windows service install defaults to favor the Go runtime.

### v5.1.0

- Added automatic `endpoints.csv` reload between cycles.
- Added last-known-good fallback for invalid endpoint edits.
- Improved HEC and metrics retry resilience across Splunk restarts.
- Added native `.psd1` parsing fallback when `pwsh` was unavailable on macOS or Linux.

### v3.2.1

- Added retry-safe HEC batching and configurable backoff.
- Added buffer caps with drop-newest policy.
- Added deterministic `event_id` generation for deduplication.
- Improved error logging with HEC response details.

### v2.7.1

- Added support for multi-word dashboard filter values such as `Network Printers` and `Palo Alto`.
- Fixed SPL token quoting for exact-match filters on `entitytype`, `device`, `vendor`, and `group`.

### v2.7.0

- Improved HEC response logging.
- Improved dual-mode events and metrics operation.
- Cleaned up batch-send console output.

### v2.6.0

- Added the asset correlation dashboard and setup wizard line.

### v2.5.x

- Added summary-only mode, endpoint enrichment, and early dual-mode query support.
