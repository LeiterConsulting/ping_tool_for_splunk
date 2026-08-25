# Ping Monitor v5.11.0 Release Notes

Release date: August 25, 2026

## Discovery Review And CMDB Workflow

- Adds durable Needs Review, Deferred, and Ignored queues without rewriting immutable discovery snapshots.
- Requires an explicit Production or Maintenance decision before a discovered device can be staged for monitored inventory.
- Persists review notes, review time, Asset ID, dynamic-address policy, and approved inventory state across restart.
- Keeps historical endpoints and configurations valid; saved legacy inventory is treated as approved even when the new review columns are absent.
- Adds review filters, bulk review actions, per-device Asset ID editing, and complete review metadata to discovery CSV export.

## DHCP And Identity Truth

- Uses unique Asset ID as the preferred durable identity and a unique forward-confirmed FQDN as a fallback correlation signal.
- Carries known identity and policy to a new DHCP address only when the correlation is unique; ambiguous evidence fails closed.
- Excludes unresolved dynamic observations from New/Missing asset counts instead of presenting an IP change as CMDB lifecycle truth.
- Rejects duplicate nonblank Asset IDs case-insensitively in both runtime and UI validation.
- Preserves delta continuity when an identity is promoted from FQDN evidence to an Asset ID.

## Policy, Naming, And Subnet Context

- Renames the operator-facing Dev workflow to explicit Production, Maintenance, and Legacy Dev modes while retaining legacy `dev` behavior.
- Separates Alerting Enabled from monitoring: alert-disabled devices continue to be measured but are excluded from packaged alert searches.
- Adds subnet name, VLAN, location, addressing mode, and routing-domain enrichment for discovery.
- Adds ordered Go RE2 naming-convention rules with named captures, test/preview controls, fill-blank or overwrite behavior, and rule provenance.

## Splunk Pairing

- Pairs with Splunk app 3.2.0 build 44.
- Adds review-state filtering and review metadata to Discovery Inventory.
- Carries device mode, alert policy, Asset ID, DHCP identity, naming provenance, and subnet metadata through CMDB normalization.
- Labels historical discovery events without review fields as `legacy_untracked` rather than fabricating approval.

## Compatibility

- Existing PSD1, JSON, YAML, and endpoint CSV files remain valid and are not automatically rewritten.
- Missing new configuration sections and endpoint columns retain compatibility defaults.
- Legacy `dev=true` continues to ping and emit legacy record types until explicitly migrated to Maintenance.
- Existing monitoring event history and dashboards remain searchable without reindexing.

## Release Validation

- Full Go test suite and full race-enabled Go test suite.
- Deterministic browser-workflow tests for review gating, filters, approval metadata, and immutable scan evidence.
- API, persistence, restart, DHCP address-change, endpoint CSV round-trip, validation, and discovery-delta tests.
- JavaScript syntax checks, Splunk XML parsing, Configuration Advisor validation, and Windows service-definition validation.
- Splunk AppInspect precert on the packaged app: 0 errors, 0 failures, 0 future failures, 4 expected warnings, and 103 successful checks.

## Environment-Dependent Checks

- A fresh interactive click-through of the final review controls could not be completed because the workstation's in-app browser automation session became unavailable. The workflow is covered by deterministic JavaScript, API, HTTP/static-shell, and Go integration tests; an operator click-through remains recommended before broad rollout.
- Creating, starting, stopping, and deleting a real Windows Service still requires an elevated clean-host validation pass. Installer definition and preflight validation are automated and pass.
