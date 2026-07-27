# Ping Monitor v5.9.0 Release Notes

Release date: 27 July 2026

Version 5.9.0 adds durable discovery, explicit monitoring policy, bounded log retention, and an upgradeable configuration model while preserving existing PSD1, flat JSON/YAML, and endpoint CSV deployments.

## Truth in Signal

- `dev` remains an inventory classification and no longer implies whether an endpoint is probed.
- `monitoring_enabled=false` pauses probes without manufacturing packet loss, latency, or a down state.
- `maintenance_until` suppresses probes until the RFC 3339 timestamp expires; monitoring resumes automatically afterward.
- Suppressed endpoints emit `record_type=monitoring_control` with `observation_status=suppressed` and an explicit `maintenance` or `disabled` state.
- Event schema v4 carries FQDN and monitoring-policy fields while retaining the established summary and ping record types.

## Discovery and CMDB Foundations

- Discovery results include FQDN, reverse-DNS status, forward-confirmation evidence, measured discovery RTT, scan identity, source, and discovery time.
- The UI exports all or selected results as a correctly quoted UTF-8 CSV.
- Every completed scan is stored as a JSON snapshot with a target-specific new/missing/unchanged delta.
- Multiple timezone-aware weekly discovery schedules are supported. Scheduled results remain review-only and are never silently imported into monitoring.
- Scheduled discovery runs even when the optional admin UI listener is disabled.
- The paired Splunk app 3.0.0 build 42 adds a current CMDB Inventory dashboard and continues to normalize older event history.
- The CMDB dashboard defaults to a responsive 24-hour search window; operators can expand the time control when older inventory history is required.

## Log Rotation and Retention

- Size checks now occur during continuous writes, not only when the collector opens the file.
- Rotation flushes and syncs the active log, creates uniquely named archives, reopens the live file, and then performs optional gzip compression.
- Retention can be bounded by archive count, age, or both.
- New schema-v2 deployments default to 50 MB rotation, 10 archives, 14 days, and compressed archives.
- Legacy configs with no retention fields keep their prior unlimited-retention behavior until explicitly upgraded or edited.
- The advisor and runtime status expose log size, threshold, archive, and retention evidence.

## Configuration Upgrade Path

- New deployments use grouped, versioned `config.json` schema 2.
- Existing `config.psd1`, flat JSON, and flat YAML files continue to load without modification as schema 1.
- `pingmonitor config upgrade --config <source> --to <config.json> --check` previews an upgrade.
- Replace `--check` with `--apply` to write and reload-verify the new file without modifying the source. Existing targets receive a timestamped backup.
- Service definitions must be updated to point `--config` at the upgraded file; migration is never activated implicitly.

## Endpoint Compatibility

- Existing `ip,hostname` CSV files remain valid.
- New optional columns are `fqdn`, `monitoring_enabled`, `maintenance_until`, and `maintenance_reason`.
- Omitted `monitoring_enabled` means enabled, preserving every existing monitored endpoint.
- The advisor validates new identity and policy fields and excludes deliberately suppressed endpoints from worker-capacity recommendations.

## Upgrade Notes

1. Back up the deployment folder.
2. Replace the executable and matching `Install-Service.ps1`.
3. Keep the existing config and endpoints files for a no-migration upgrade.
4. Run `.\pingmonitor.exe --validate --config <active-config> --endpoints .\endpoints.csv`.
5. Use the optional config upgrade command only when ready to adopt the grouped JSON format.
6. Install `ping_monitor_3.0.0_build42_20260727.tar.gz` in Splunk and restart Splunk if requested.
