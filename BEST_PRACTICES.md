# Ping Monitor Best Practices & Troubleshooting (Config Tuning)

This guide focuses on configuration patterns that help the PowerShell Ping Monitor run reliably at scale (large endpoint lists, long runtimes, and/or constrained hosts).

Applies primarily to `PingMonitor_v4_0_0.ps1` (recommended). Most guidance also applies to `PingMonitor_v3_3_3.ps1`, but v4 is designed to be more stable under large lists due to bounded scheduling.

---

## Quick Triage (What To Change First)

If you’re seeing high CPU / memory / handle growth under a large endpoint list:

1. Reduce event volume
   - Set `emit_individual_pings = $false` (summary-only) to cut output dramatically.
   - If you use metrics, consider `metrics.mode = "metrics_only"`.
2. Reduce concurrency
   - Lower `parallel_threads` until the host stabilizes (start around 25–50 for large lists).
3. Reduce per-endpoint work
   - Lower `pings_per_cycle` (1 is usually enough for availability monitoring).
   - Reduce `ping_timeout_ms` to prevent long-lived in-flight work (e.g., 250–1000ms depending on network).
4. Increase `interval_seconds`
   - Give the host time to recover between cycles (especially with thousands of endpoints).

---

## Mental Model: What Drives Resource Usage

These are the biggest levers:

- **Endpoint count**: More endpoints means more work per cycle and larger output.
- **Concurrency (`parallel_threads`)**: Higher concurrency increases throughput but also increases concurrent allocations, socket usage (HEC), and in-flight runspace work.
- **Timeout (`ping_timeout_ms`)**: Long timeouts keep work items alive longer, inflating in-flight sets.
- **Output volume**:
  - `emit_individual_pings = $true` multiplies events by `pings_per_cycle`.
  - File output and HEC output both cost CPU and memory; large JSON payloads can dominate.
- **Retry / buffering** (HEC): Retries and large buffers can increase memory use if Splunk is unavailable.

v4 uses a bounded scheduler so it does not start all endpoints at once. That helps avoid “runaway” pressure where each cycle creates too many in-flight objects.

---

## Recommended Baselines (Large Lists)

### Large list, events (summary-only)
Use this for availability monitoring with low overhead:

- `emit_individual_pings = $false`
- `pings_per_cycle = 1`
- `parallel_threads = 25` (increase gradually)
- `ping_timeout_ms = 500` (tune to your network)
- `interval_seconds = 5` (or higher)

### Large list, metrics-only
Use this for long-term time-series at scale:

- `emit_individual_pings = $false`
- `metrics.enabled = $true`
- `metrics.mode = "metrics_only"`
- `parallel_threads = 25–100` depending on host/network
- `interval_seconds = 5–30` depending on how much resolution you need

---

## Configuration Best Practices (PowerShell / v4)

### 1) Concurrency: `parallel_threads`

- Treat `parallel_threads` as an “in-flight work cap”.
- For large lists, start lower (25–50) and scale up while watching CPU, WS/PM, and handle counts.
- If you see intermittent spikes, that’s often okay; the key is that usage returns to a stable band over time.

When to lower it:
- Host CPU stays pinned.
- Handles climb and don’t return to baseline.
- WS/PM climbs cycle-over-cycle without leveling.

### 2) Output volume: `emit_individual_pings` and `pings_per_cycle`

Event count roughly scales like:

- Summary-only: ~$\text{endpoints}$ events per cycle
- With individual pings: ~$\text{endpoints} \times \text{pings_per_cycle}$ events per cycle (plus summary)

If Splunk ingestion or file I/O becomes the bottleneck, your ping work may “back up” behind output.

### 3) Interval sizing: `interval_seconds`

Set the interval so a full cycle reliably completes before the next begins.
If you run cycles back-to-back (interval too low), you can create sustained pressure even with bounded scheduling.

Rule of thumb:
- If a cycle takes $T$ seconds, keep `interval_seconds >= T` (or increase endpoint list / lower threads until it fits).

### 4) Timeout sizing: `ping_timeout_ms`

Long timeouts are the most common reason a large list “feels heavy”.

- Start with 500–1000ms for WAN-ish lists.
- Use 250–500ms for LAN-only lists.
- If you need multi-second timeouts, compensate with lower `parallel_threads` and/or higher `interval_seconds`.

### 5) DNS / hostnames

- Prefer stable IPs for large lists when possible.
- If you rely on hostnames and DNS is slow, you can see cycle time inflate.

---

## HEC Delivery: Preventing Backpressure and Memory Spikes

If Splunk HEC is down or intermittently failing:

- Keep batch sizes reasonable (e.g., 50–200).
- Cap buffer growth (`max_buffer_events` / `max_buffer_bytes` in config).
- Enable retry if you can tolerate delayed delivery; otherwise prefer dropping with an optional dead-letter.

If you see memory spikes when HEC is unreachable:
- Lower `max_buffer_events` / `max_buffer_bytes`.
- Reduce event volume (summary-only / metrics-only).
- Consider enabling dead-letter persistence so you can replay later without holding everything in RAM.

---

## File Output: Avoiding Disk Bottlenecks

File output is convenient for debugging but can become a bottleneck at scale.

Recommendations:
- Use summary-only mode for file output.
- Place output on fast storage.
- If you need per-ping events, consider limiting `pings_per_cycle` and increasing `interval_seconds`.

---

## Diagnostics: What To Watch

When diagnosing stability, you care about trends over time, not a single snapshot.

Useful signals:
- **Working Set (WS)**: overall memory footprint seen by the OS.
- **Private Memory (PM)**: memory owned by the process.
- **Handles**: leaks often show up here first.
- **Thread count**: should stay roughly stable.

v4 supports per-cycle memory stats (see config defaults in the script) and an alias toggle `debug.emit_memory_stats`.

---

## Troubleshooting Playbook

### Symptom: Memory climbs steadily over many cycles
Try:
- Set `emit_individual_pings = $false`.
- Lower `parallel_threads`.
- Lower `max_buffer_bytes` / `max_buffer_events` (HEC).
- Increase `interval_seconds`.

### Symptom: Handles spike and never recover
Try:
- Lower `parallel_threads`.
- Reduce output sinks (disable file output; reduce HEC batching pressure).
- Enable diagnostics and confirm handle deltas are not strictly increasing.

### Symptom: Cycles take longer and longer
Try:
- Lower `ping_timeout_ms`.
- Check DNS latency if using hostnames.
- Reduce HEC retry aggressiveness.
- Increase `interval_seconds`.

### Symptom: Splunk ingestion can’t keep up
Try:
- Summary-only mode.
- Metrics-only mode for high-scale time series.
- Reduce `pings_per_cycle`.
- Reduce per-cycle volume by increasing `interval_seconds`.

---

## Safe Scaling Workflow

1. Start small (10–50 endpoints) and validate output.
2. Scale to 100–500 endpoints and watch cycle time.
3. Scale to full list and tune:
   - First tune volume (`emit_individual_pings`, metrics-only)
   - Then tune concurrency (`parallel_threads`)
   - Then tune timeouts (`ping_timeout_ms`) and interval (`interval_seconds`)
4. Run an endurance test (many cycles) and verify WS/PM/Handles stay in a stable band.
