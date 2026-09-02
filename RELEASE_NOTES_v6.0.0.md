# Ping Monitor v6.0.0 Release Notes

Release date: 2026-09-02

Status: released (`v6.0.0`)

## Why This Is A Major Release

Version 6 changes the Windows deployment contract. Monitoring, discovery, configuration parsing, service hosting, and the administration UI now run in the Go executable by default. The release removes PowerShell and NSSM from the normal Windows runtime path while deliberately retaining supported v5 deployment data and explicit compatibility routes.

## System Dashboard And Capacity Evidence

The administration UI adds a dedicated System page and `GET /api/system` endpoint. It reports:

- observed host CPU and installed, available, and used physical memory;
- collector CPU as both host-capacity percentage and logical-core equivalent;
- working set, peak working set, private memory, handles, and OS threads;
- Go heap, reserved memory, stack, next-GC target, memory limit, GC cycles and pauses, goroutines, and `GOMAXPROCS`;
- active, configured, and process-lifetime peak monitoring, discovery-probe, and DNS worker counts; and
- monitoring workload shape plus enforced discovery safety ceilings.

Resource sampling is cached for two seconds. Multiple dashboard tabs reuse the same evidence rather than multiplying operating-system queries. A counter that cannot be measured on the current platform is returned as `null` with an `unavailable_fields` explanation; it is never represented as a healthy zero.

## Splunk Delivery Evidence

- Event and metrics connection definitions expose Indexer ACK as a first-class confirmation choice.
- Connection tests exercise the selected contract: accepted-only tests say indexing is unconfirmed, while ACK-enabled tests wait for Splunk to confirm the acknowledgment ID.
- Runtime delivery status distinguishes HEC acceptance from indexed acknowledgment and does not invent a success timestamp when the outbox is merely empty at startup.
- Metrics include the collector's exact observation epoch so current-state searches do not mistake a metrics bucket boundary for the observation time.

## Native Windows Service

The Windows executable now implements the Windows Service Control Manager dispatcher and provides:

```powershell
.\pingmonitor.exe service validate
.\pingmonitor.exe service install
.\pingmonitor.exe service status --json
.\pingmonitor.exe service start
.\pingmonitor.exe service stop
.\pingmonitor.exe service restart
.\pingmonitor.exe service uninstall
```

The native service reports Running only after collector initialization, accepts Stop and Shutdown without offering a misleading Pause control, uses delayed automatic start by default, applies escalating failure-recovery delays, and writes a bounded 10 MiB service log with five retained archives. Install validation runs the same Configuration Advisor and scheduler-capacity checks as an interactive start before changing SCM state.

`service status --json` identifies native v6, NSSM, direct legacy, and other-wrapper definitions. Replacing an existing definition requires explicit `--force`. Service replacement does not remove configuration, endpoint, discovery, or log data.

## Native Configuration And Discovery

- PSD1 is parsed by a non-executing Go parser. Loading configuration no longer launches PowerShell.
- Parser diagnostics include source locations and reject duplicate keys, malformed input, integer overflow, and unterminated strings or comments.
- Existing supported PSD1, flat JSON/YAML, and grouped schema-v2 JSON/YAML configurations remain valid.
- `pingmonitor discovery` exposes the native discovery engine directly with CSV or JSON output and overwrite protection.
- Manual and scheduled discovery retain bounded ICMP/DNS workers, cancellation, FQDN verification, measured-versus-censored latency provenance, and fail-closed indeterminate evidence.
- `--discovery-script` remains an explicit deprecated compatibility override. Adjacent scripts are not selected automatically.

## Compatibility And Migration

No configuration migration is required to start v6. Existing config files, endpoint CSV files, discovery history, review state, appearance preferences, and Splunk event searches remain supported. The optional config-upgrade command remains available for operators who want grouped schema-v2 JSON.

Existing NSSM services remain valid until intentionally migrated. The recommended migration is:

1. Record `service status --json` and the existing service definition.
2. Validate the v6 binary with explicit config, endpoint, and UI arguments.
3. Stop the existing service.
4. Run `service install --force` with those same paths.
5. Confirm Running state, `/healthz`, `/api/status`, `/api/system`, and a complete monitoring cycle.
6. Preserve the recorded definition until the observation window is complete so rollback remains possible.

The legacy `Install-Service.ps1` remains available for v5/NSSM and PowerShell-runtime deployments. It is not a prerequisite for the native v6 service.

## Release Gates

The release must not be tagged until all of these are green:

- `go test ./...`, `go vet ./...`, and `go test -race ./...`;
- JavaScript syntax and browser verification of all UI routes, contextual help, and responsive System layout;
- packaged-binary concurrent System API stress with cache, identity, deep-link, responsiveness, and resource-growth assertions;
- native discovery packaged-binary, soak, cancellation, collision, retention, and resource-bound tests;
- PSD1 compatibility and malformed-input tests;
- packaged-binary PSD1 validation with `pwsh` removed from `PATH`, including repeated timing samples and source-located duplicate rejection;
- native service in-process readiness/health/stop tests;
- elevated temporary-SCM install/refusal-without-force/start/health/restart/PID-change/stop/forced-replacement/restart/uninstall test;
- cross-platform release builds and checksum verification; and
- clean-deployment and in-place-NSSM-migration rehearsals with documented rollback.

The Windows executable remains unsigned. Verify release artifacts using the published SHA-256 manifest.

## Release-Candidate Validation — 2026-09-02

The exact candidate artifacts passed:

- `go test ./...`, `go vet ./...`, and `go test -race ./...` with the Windows C toolchain active;
- packaged-binary native config, discovery, cancellation, collision, retention, resource-bound, and System API stress suites;
- JavaScript syntax, CSV export safety, XML parsing, route/API health, and configuration compatibility checks;
- the elevated isolated Windows SCM lifecycle canary, including non-forced replacement refusal, start, health, restart with PID replacement, stop, forced replacement, log handling, and uninstall;
- an in-place lab cutover with checksum-verified rollback preservation, native service status, matching SCM/System API PID, one complete 78-endpoint cycle, healthy accepted-only delivery, and zero pending envelopes;
- live event and metrics HEC acceptance tests returning HTTP 200 with indexing explicitly reported as unconfirmed while Indexer ACK is disabled; and
- end-to-end Splunk current-state parity: all 78 current event and metric states matched, all 78 lookup rows used exact collector observation time, and no Down row retained latency.

The production-service restart exercise and 12–24 hour soak were explicitly deferred from this validation tranche and remain recommended post-release observation work.
