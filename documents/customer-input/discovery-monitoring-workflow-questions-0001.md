# Discovery and Monitoring Workflow Questions

This document collects the decisions and examples needed to finish the naming, maintenance, alerting, scheduled-review, DHCP, and subnet-metadata workflows. Existing Ping Monitor configurations and endpoint files will remain valid while these features are introduced.

Please provide representative examples rather than production credentials or other sensitive information. Sanitized hostnames, domains, asset identifiers, and subnet values are sufficient.

The implementation currently follows the safe defaults at the end of this document so release work can continue without guessing destructive migration behavior. These questions can refine later behavior without invalidating existing configuration files or discovery review records.

## 1. Naming and Classification Rules

Please provide at least five representative hostname or FQDN examples and the fields each example should produce.

The planned interface uses ordered match/assignment pairs. Each pair selects Hostname, FQDN, or either; supplies a Go RE2 regular expression; and maps literal values or named captures such as `(?P<site>...)` and `${site}` into Group, Entity Type, Device, and Vendor. Operators can test a sample name and preview affected endpoints or discovery rows before applying anything.

| Hostname or FQDN | Group | Entity Type | Device | Vendor |
|---|---|---|---|---|
| Example: `nyc-srv-vm-001.example.com` | Example: `New York Servers` | Example: `Server` | Example: `Virtual Machine` | Example: `VMware` |
|  |  |  |  |  |
|  |  |  |  |  |
|  |  |  |  |  |
|  |  |  |  |  |
|  |  |  |  |  |

Questions:

1. Which value should rules inspect: short hostname, FQDN, or both?
2. Do names contain reusable components such as site, environment, device class, operating system, or vendor codes?
3. Should an earlier matching rule win, or may several rules fill different fields? The proposed default is ordered rules where later rules may fill only fields that remain blank.
4. Should automatic rules ever overwrite a value entered by a person? The proposed default is no: automatic classification fills blank fields and records the rule that supplied each value.
5. Are the allowed values for Group, Entity Type, Device, and Vendor controlled lists, or may operators enter new values freely?
6. Should naming rules run only during discovery review, or also when an operator manually creates or edits an endpoint? The proposed default is both, with a preview before changes are applied.
7. Should matching be case-sensitive? The proposed default follows the expression exactly; rules that should ignore case can begin with `(?i)`.
8. Do any required naming rules depend on regex features such as lookaround or backreferences? Go's RE2 engine intentionally excludes those features so evaluation time remains bounded. Most naming conventions can use anchored patterns and named captures instead.

## 2. Existing Dev/Test Endpoint Migration

The current `dev=true` field is classification-only and does not stop pings. Changing that meaning automatically could silently stop monitoring existing devices.

Questions:

1. Should existing Dev/Test endpoints be converted to Maintenance, converted to Production, or reviewed individually?
2. If a bulk conversion is desired, should all existing Dev/Test endpoints be treated the same way, or are there groups that require different mappings?
3. Should the legacy Dev/Test dashboard remain visible during a transition period? The proposed default is yes, until every legacy row has been explicitly reviewed.

The proposed safe default is to preserve all current Dev/Test behavior and provide a previewed, confirmation-gated migration tool.

## 3. Maintenance Mode

The proposed Maintenance mode is an indefinite, explicit probe suppression state. Existing `maintenance_until` windows will continue to provide time-bounded maintenance.

Questions:

1. Is an indefinite Maintenance mode required in addition to timed maintenance windows?
2. Should a maintenance reason be mandatory before Maintenance mode can be saved? The proposed default is yes for new changes, while preserving legacy files that have no reason.
3. When an indefinite Maintenance endpoint returns to Production, should alerting automatically return to its previous setting or always default to enabled?

## 4. Alerting Disabled

Alerting Disabled will continue all probes and health evaluation while marking the endpoint ineligible for packaged Splunk alerts.

Questions:

1. Should the flag suppress all packaged endpoint alerts—Down, high packet loss, and high latency—or only selected alert types? The proposed default is all packaged endpoint alerts.
2. Should Alerting Disabled endpoints remain included in availability and latency reports? The proposed default is yes because their measurements remain valid.
3. Should dashboards show a visible Alerting Disabled badge and filter? The proposed default is yes.
4. Should an alerting-disable reason be captured? If so, should it be mandatory?

## 5. Scheduled Discovery Review

The proposed workflow keeps scan evidence immutable and stores review decisions separately. Operators will be able to filter Needs Review, select many candidates, apply common fields, apply naming rules, choose Production or Maintenance, set alert eligibility, and approve selected records into the endpoint inventory.

Questions:

1. Besides Needs Review, Approved, Ignored, and Deferred, are additional review states required?
2. Should ignored candidates stay hidden permanently, reappear after a set time, or reappear when their evidence changes?
3. Should approval immediately save to `endpoints.csv`, or stage changes in the normal endpoint draft for a final Save? The proposed default is to stage changes for a final revision-safe Save.
4. Is recording review time and source rule sufficient, or is the reviewing username also required? The current local UI has no authentication identity to use as a trustworthy username.
5. Should Maintenance candidates be added to the endpoint inventory in a probe-suppressed state, or remain only in the discovery inventory until moved to Production?

## 6. DHCP and Dynamic-Address Identity

ICMP discovery observes an IP address and response. It cannot, by itself, prove that a device seen at a different IP is the same asset.

Questions:

1. Which stable identifiers can be supplied for DHCP devices?

   - CMDB asset ID
   - MAC address
   - DHCP client identifier
   - DHCP reservation identifier
   - Forward-confirmed unique FQDN
   - Another identifier

2. Can Ping Monitor receive DHCP lease data or a periodic export from the DHCP platform?
3. Is a forward-confirmed unique FQDN acceptable as a medium-confidence identity match, or must DHCP reconciliation require MAC, client ID, or CMDB asset ID?
4. For a DHCP observation with no stable identifier, should it be excluded from the New Device count and labeled Unresolved Dynamic Observation? This is the proposed default because it avoids falsely claiming either a new asset or a known asset.
5. Should dynamic-address status be assigned at the subnet level, individual-device level, or both? The proposed implementation supports both, with an individual override taking precedence.

## 7. Subnet Catalog

Please provide two or three sanitized subnet examples using the desired format.

| CIDR | Subnet Name | VLAN | Location | Addressing Mode |
|---|---|---|---|---|
| Example: `10.20.30.0/24` | Example: `Headquarters Workstations` | Example: `230` | Example: `Main Office / Floor 2` | `dhcp` |
|  |  |  |  | `static` or `dhcp` |
|  |  |  |  | `static` or `dhcp` |

Questions:

1. Should VLAN be stored as a number, a display name such as `VLAN 230`, or both VLAN ID and name?
2. Is Location free text, or should it be split into Site, Building, Floor, and Room?
3. May the same CIDR appear at more than one location, such as overlapping networks in separate routing domains? If yes, a routing-domain or tenant identifier is also required.
4. Should schedules reference subnet catalog entries by a stable subnet ID, or continue using CIDR values? The proposed default keeps existing CIDR schedule targets valid and attaches catalog metadata by normalized CIDR.
5. Is there an existing subnet spreadsheet, IPAM export, or CMDB export that should be importable?

## Proposed Defaults if No Additional Direction Is Provided

- Preserve all legacy `dev` behavior until explicitly migrated.
- New endpoints use Production or Maintenance rather than Dev/Test.
- Maintenance suppresses probes and packaged alerts without fabricating Down state.
- Alerting Disabled preserves probes, health state, metrics, and reporting while suppressing all packaged endpoint alerts.
- Automatic classification fills blank fields and never overwrites human-entered values.
- Scheduled discovery review stages approved devices into the endpoint draft before a final save.
- Unidentified DHCP observations are labeled unresolved and excluded from New Device counts.
- Existing schedule targets and all current PSD1, JSON, YAML, and endpoint CSV files remain valid.
