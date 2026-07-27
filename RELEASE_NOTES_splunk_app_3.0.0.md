# Ping Monitor for Splunk 3.0.0 Release Notes

Release date: 27 July 2026
Build: 42
Paired collector: Ping Monitor v5.9.0

- Adds the **CMDB Inventory** dashboard for current endpoint identity, FQDN coverage, monitoring policy, maintenance, first/last seen, and collector-confirmed state.
- Adds schema-v4 awareness for `monitoring_control` events so intentionally suppressed probes appear as **Paused** or **Maintenance**, never as inferred downtime.
- Retains v1-v3 normalization and historical summary searches.
- Leaves existing Overview, Prod Devices, Dev Devices, metrics reports, alerts, and asset-correlation searches compatible.
- Uses the Setup-configured events index and sourcetype for the new inventory view.
