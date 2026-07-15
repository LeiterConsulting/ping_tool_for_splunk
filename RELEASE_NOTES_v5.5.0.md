# Ping Tool for Splunk - Release v5.5.0

## Truth in signal

- Schema v3 separates the probe observation from the hysteretic endpoint state. It records state confidence and the configured down/recovery and cadence-derived stale thresholds with every summary.
- Latency is emitted only when the backend measured a value. Platform-reported values such as `time<1ms` and Windows `RoundTripTime == 0` are represented as censored measurements with a 1 ms upper bound, not as an invented midpoint or wall-clock substitute.
- Probe/backend failures remain invalid observations with no packet-loss or latency number. ICMP status and probe elapsed time are retained separately for diagnosis.
- Splunk app 2.9.0 uses collector state for current health. Legacy event and metric history remains searchable through explicitly labeled inference.

## Delivery reliability and performance

- Each completed event/metrics cycle is atomically persisted to a bounded durable outbox before network delivery. Capacity exhaustion fails closed rather than dropping old or new signal.
- A separate delivery worker drains the outbox, so HEC latency, retry delay, outage, or indexer acknowledgment polling cannot stretch probe cadence.
- Per-sink offsets survive restarts: if event delivery succeeds and metrics delivery fails, the event batch is not sent again.
- Optional Splunk indexer acknowledgment uses a stable request-channel GUID and polls the HEC acknowledgment endpoint. Delivery status identifies `hec_accepted_only`, `indexed_acknowledged`, or `mixed` confirmation.

## Runtime resilience

- Endpoint CSV input now rejects malformed rows, non-IP targets, duplicate canonical addresses, duplicate endpoint IDs, and invalid dev flags. Hot reload retains the last known-good inventory.
- Scheduler admission accounts for every per-ping timeout, inter-probe spacing, worker count, and cycle interval. Impossible inventories are rejected, while cycle logs expose worst-case utilization, dispatch pacing, and overruns.
- Invalid preferred config files now stop startup with the parse error instead of being silently replaced by a generated fallback.
- The admin API treats HEC tokens as write-only. Blank token fields preserve stored values; responses expose only configured/not-configured flags.

## Service operations

- `Install-Service.ps1` supports non-admin validation/status plus elevated install, start, stop, restart, and uninstall operations.
- Service definitions verify executable, working directory, arguments, stdout/stderr rotation, delayed automatic start, graceful stop, application restart, and SCM recovery.
- The repository includes a safe definition test and an elevated disposable lifecycle canary covering create, health, restart, stop, and removal.
