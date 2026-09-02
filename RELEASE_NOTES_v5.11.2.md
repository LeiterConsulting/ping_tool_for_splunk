# Ping Monitor v5.11.2 Release Notes

Release date: September 2, 2026

## Stale Discovery Script Override Hotfix

- The discovery script embedded in `pingmonitor` is now the authoritative default and always matches the executable version.
- A stale `DiscoverEndpoints.ps1` left beside the executable, configuration, or endpoint inventory no longer silently overrides the embedded workflow after an executable-only upgrade.
- Operators who intentionally maintain a custom discovery script can opt in with `--discovery-script <path>`.
- An explicitly configured custom script that is missing or unreadable is reported as unavailable. The collector does not silently fall back and misrepresent which workflow it executed.
- The status API continues to expose `discovery_script_path`; default deployments now report `embedded:DiscoverEndpoints.ps1`.

This closes the v5.11.1 upgrade trap in which the corrected embedded script was present but an older adjacent script still won path resolution. The older script's line 86 then produced the original `No active network adapter with a default gateway found` error even though the UI correctly reported collector v5.11.1.

## Retained v5.11.1 Network Selection Fix

- Explicit discovery targets bypass local gateway detection.
- Automatic local discovery uses active route and interface metrics.
- A single isolated IPv4 interface is allowed with a warning.
- Ambiguous interfaces or equally preferred default routes fail closed with candidate evidence and an instruction to use `-TargetNetwork`.
- The downloadable and embedded discovery scripts remain byte-identical at script version 2.5.3.

## Compatibility

- Existing PSD1, JSON, and YAML configurations remain valid without migration.
- Existing endpoints, discovery history, review state, naming rules, UI preferences, and CMDB evidence remain compatible.
- Splunk app 3.2.0 build 44 remains compatible and does not require reinstallation.
- Existing Windows service definitions do not require reinstallation when the executable remains at the configured path.

## Upgrade

1. Stop the collector or Windows service.
2. Replace `pingmonitor.exe` with v5.11.2.
3. Existing adjacent discovery scripts may remain for direct command-line use; the collector ignores them unless `--discovery-script` was explicitly added to its command line.
4. Keep the existing configuration, endpoints, preferences, and discovery data.
5. Validate the deployment and restart the collector.
6. Confirm `/api/status` reports `v5.11.2` and `discovery_script_path` reports `embedded:DiscoverEndpoints.ps1` unless a custom override was deliberately configured.

Direct users of `DiscoverEndpoints.ps1` should still replace an older script with the included version 2.5.3 file.

## Validation

- Regression tests confirm that an unconfigured adjacent stale script is ignored.
- Regression tests confirm that a deliberate external override is honored and a missing explicit override does not silently fall back.
- The packaged-binary test plants a v2.5.2 failing script beside the configuration, starts the new executable, verifies the embedded path is selected, and completes an explicit discovery through the API.
- The complete Go, `go vet`, race-detection, PowerShell network-selection, service-definition, JavaScript, artifact, and checksum gates are run against the release candidate.

The Windows executable remains unsigned. Verify downloads against `SHA256SUMS_v5.11.2.txt` before deployment.
