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
- Keeps each naming-pair title synchronized with its Rule ID and gives reorder/remove controls unique accessible names when multiple pairs are present.

## Multi-Page Admin UI

- Replaces the single anchor-scrolled administration document with URL-backed Overview, Advisor, Endpoints, Discovery, and Settings pages.
- Splits Settings into Runtime, Discovery and CMDB, Splunk Delivery, and Diagnostics subpages while retaining unsaved draft state during in-app navigation.
- Consolidates endpoint and discovery bulk actions into contextual selectors and moves reset, export, test, reorder, and destructive commands into overflow menus.
- Adds a responsive navigation rail and wrapping action layouts for narrower screens.
- Keeps naming-rule construction full width, moves per-rule reorder/remove commands into an overflow menu, and prevents preview-only sample text from marking configuration dirty.

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
- Interactive naming-builder validation for valid and invalid RE2 rules, hostname and FQDN sources, pair ordering/removal, contextual help, and non-persistent reset behavior.
- API, persistence, restart, DHCP address-change, endpoint CSV round-trip, validation, and discovery-delta tests.
- JavaScript syntax checks, Splunk XML parsing, Configuration Advisor validation, and Windows service-definition validation.
- Direct-route, browser history, single-visible-page, contextual-action, draft-preservation, and interactive regex naming-rule checks against the deployed Windows binary.
- Splunk AppInspect precert on the packaged app: 0 errors, 0 failures, 0 future failures, 4 expected warnings, and 103 successful checks.

## Environment-Dependent Checks

- The multi-page shell, settings subpages, contextual action enablement, browser back/forward behavior, endpoint selection preservation, non-persistent naming-rule preview, and deployed v5.11.0 runtime status were interactively verified at `http://127.0.0.1:8080`.
- Creating, starting, stopping, and deleting a real Windows Service still requires an elevated clean-host validation pass. Installer definition and preflight validation are automated and pass.
