# Ping Monitor v5.11.1 Release Notes

Release date: September 1, 2026

## Discovery Network Selection Hotfix

- Explicit `-TargetNetwork` scans no longer require local adapter or default-gateway detection before the requested IPv4/CIDR is validated and scanned.
- Automatic local discovery correlates active IPv4 configurations with `0.0.0.0/0` routes and selects the interface with the lowest effective route plus interface metric.
- A host with no default route can use its only unambiguous active, non-loopback, non-APIPA IPv4 interface. The script reports that fallback as a warning rather than claiming a gateway exists.
- Multiple equally preferred default routes or multiple active IPv4 interfaces without a default route now fail closed with the candidate interfaces and an instruction to specify `-TargetNetwork`.
- Gateway-based device classification is omitted for explicit remote targets instead of inventing a gateway from an unrelated local interface.
- The downloadable `DiscoverEndpoints.ps1` and the copy embedded in `pingmonitor` are version 2.5.3 and byte-identical.

## Compatibility

- Existing PSD1, JSON, and YAML configurations remain valid without migration.
- Existing endpoint CSV files, discovery history, review state, naming rules, and CMDB evidence remain compatible.
- Splunk app 3.2.0 build 44 remains the paired app; no Splunk app reinstall is required for this collector-only hotfix.
- Windows service definitions do not need to be reinstalled when the executable and companion script are replaced in their existing paths.

## Upgrade

1. Stop the collector or Windows service.
2. Replace `pingmonitor.exe` with the v5.11.1 Windows binary.
3. Replace any standalone `DiscoverEndpoints.ps1` beside the executable, configuration, or endpoint file. A standalone script takes precedence over the embedded copy.
4. Keep the existing configuration, endpoint inventory, UI preferences, and discovery data directories.
5. Run `./pingmonitor.exe --validate --config ./config.psd1 --endpoints ./endpoints.csv` using the deployment's actual paths.
6. Start the collector and confirm `/api/status` reports `v5.11.1` and a running state.

Direct command-line users may replace only `DiscoverEndpoints.ps1` for immediate discovery recovery, but replacing the collector as well keeps the embedded fallback and reported product version aligned.

## Validation

- PowerShell parser validation covers both discovery-script copies.
- Network-selection regression tests cover explicit-target bypass, route-table gateway selection, effective metrics, isolated single-interface fallback, ambiguous interfaces, tied default routes, and invalid loopback/APIPA candidates.
- An end-to-end explicit `/30` discovery smoke test confirms that adapter detection is skipped and CSV output is produced.
- A packaged-binary runtime test starts v5.11.1 in UI-only mode with no standalone discovery script, confirms the embedded fallback is selected, runs discovery through `/api/discovery/run`, and verifies the explicit-target bypass in returned logs.

The Windows executable remains unsigned. Verify downloads against `SHA256SUMS_v5.11.1.txt` before deployment.
