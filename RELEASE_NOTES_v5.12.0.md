# Ping Monitor v5.12.0 Release Notes

Release date: 2026-09-02

## Native Go Discovery

Manual and scheduled subnet discovery now run inside the Go runtime. The default path no longer launches PowerShell, materializes an embedded script, writes a temporary result CSV, or infers progress from console text.

- A bounded goroutine worker pool probes IPv4 targets from `/16` through `/30`.
- The native engine uses at most 256 ICMP workers and 128 DNS workers per scan. Existing higher concurrency settings remain valid and are safely clamped; each summary records the requested value and both effective worker counts.
- Discovery uses the same configured `ping.mode` family as continuous monitoring. On Windows, `auto` and `native` use the Windows ICMP API; controlled fallback remains available in `auto` mode.
- Millisecond discovery timeouts are preserved instead of being rounded up to whole seconds by `Test-Connection`.
- Request cancellation propagates through scan and DNS-resolution workers. A platform ICMP call already in flight remains bounded by its configured per-host timeout before that worker exits.
- Explicit targets bypass local-interface selection entirely.
- Automatic local discovery uses the preferred outbound IPv4 address when it matches an active interface, accepts one unambiguous isolated interface with a warning, and fails closed when multiple candidates cannot be distinguished.
- Reverse DNS results are normalized and forward-confirmed with the context-aware Go resolver. This avoids uncancellable Windows native resolver calls retaining an unbounded set of blocked OS resources. PTR-only and unresolved observations remain explicitly identified.

## Truth in Signal

Discovery continues to mean “observed responding to ICMP at this point in time,” not “all other addresses are down.” v5.12.0 adds evidence needed to defend that distinction:

- Every discovered endpoint records the probe backend, latency source, latency resolution, censoring state, upper bound, and elapsed probe time when available.
- A sub-millisecond Windows ICMP result remains censored as `<1 ms`; the runtime does not invent a `0 ms` measurement.
- Scan summaries record hosts considered, hosts probed, hosts observed, addresses without an ICMP observation, indeterminate probes, probe backends, latency sources, requested/effective worker counts, and DNS evidence counts.
- Timeout, unreachable, TTL-expired, and packet-too-big outcomes mean the target was not observed by the scan. They are not converted into monitoring state.
- Backend errors, parser failures, empty results, cancellation, and other measurement failures are indeterminate. Any indeterminate address fails the scan before history/delta persistence, preventing a broken probe backend from manufacturing “missing device” changes.
- A complete scan with zero responders is valid evidence and produces a zero-observation scan summary.

## Compatibility

- Existing PSD1, JSON, and YAML configurations remain valid without edits.
- Existing endpoint CSVs remain valid. The new discovery evidence columns are optional and additive.
- Existing manual and scheduled discovery request shapes, subnet catalogs, history indexes, review records, naming rules, and retained snapshots remain readable.
- Existing Splunk searches continue to work. New fields are additive, and the packaged Discovery Inventory dashboard labels older observations as legacy/unknown when backend provenance is absent.
- `DiscoverEndpoints.ps1` remains available as a standalone compatibility utility.
- `--discovery-script <path>` remains supported as a deprecated, explicit compatibility override. It is the only way the collector selects PowerShell discovery; adjacent scripts are ignored.
- PowerShell remains required for the Windows service-management scripts and PSD1 tooling, but not for default manual or scheduled discovery.

## Splunk App 3.3.0

The companion Splunk app surfaces native discovery engine, backend, latency-source, host-accounting, indeterminate, and DNS-evidence fields. Historical v5.11 and older events remain searchable and render with legacy/unknown provenance where the fields did not yet exist.

## Upgrade

1. Stop the Ping Monitor service or foreground process.
2. Verify the v5.12.0 archive against `SHA256SUMS_v5.12.0.txt`.
3. Replace the executable at the exact path used by the service.
4. Keep the existing config, endpoints, discovery history, review store, and appearance preferences.
5. Unless a custom compatibility script is still required, remove `--discovery-script` from the service arguments. An adjacent `DiscoverEndpoints.ps1` does not affect native discovery.
6. Start the collector and confirm `/api/status` reports `version=v5.12.0`, `discovery_engine=native_go`, and no `discovery_script_path`.
7. Run a small explicit `/30` discovery and confirm the result reports `evidence.engine=native_go`, `hosts_probed=2`, `hosts_indeterminate=0`, and a probe backend.

A service reinstall is unnecessary when the executable, config, endpoint paths, and service wrapper remain unchanged.

## Validation Gates

The release candidate is expected to pass:

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- JavaScript discovery/export/review/theme tests
- Native discovery unit tests for target enumeration, DNS confirmation, measured and censored latency, zero responders, cancellation, local-interface selection, and fail-closed indeterminate evidence
- A packaged Windows binary test that removes `pwsh` from `PATH`, plants a failing adjacent script, runs API discovery, and verifies native Windows ICMP evidence
- Race-enabled `/16` worker-pool and cancellation stress tests plus a packaged Windows API soak covering repeated `/24` scans, DNS enrichment, legacy high-concurrency clamping, sequential runs, concurrent-run rejection, cancellation recovery, retention, and bounded process resources
- Explicit PowerShell compatibility-script validation
- Windows service-definition validation
- Cross-platform Windows, Linux, and macOS builds and release checksum verification

The Windows executable remains unsigned. Verify downloads against the published checksum manifest before deployment.
