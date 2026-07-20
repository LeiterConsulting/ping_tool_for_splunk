# Ping Monitor v5.7.1 Release Notes

Release date: 20 July 2026

Version 5.7.1 adds a confirmation-gated restart action to the embedded admin UI and tightens startup truth for safely fixable duplicate inventory rows.

## Controlled In-App Restart

- Shows **Restart Collector** only in monitor mode when the config revision saved on disk differs from the active engine revision.
- Requires an explicit browser confirmation and a matching config and endpoints revision.
- Runs the Configuration Advisor again before accepting the restart.
- Stops the monitoring engine cleanly inside the existing process, reloads config and endpoints, and resumes monitoring without relying on NSSM or spawning a competing child process.
- Keeps the HTTP admin interface available while the engine restarts so progress and errors remain visible.
- Records restart-in-progress, restart count, completion time, and last restart error in runtime status.
- Resumes the last known-good configuration if files change during activation or reload validation fails.

## Inventory Readiness Correction

An identical duplicate endpoint still offers an automatic safe fix, but it now remains a startup blocker until that fix is applied. This aligns advisor readiness with strict runtime loading: preflight cannot report ready for a file the engine would reject.

## Compatibility

- No event, metric, config-schema, or Splunk dashboard migration is required.
- Splunk app 2.9.2 build 41 remains current.
- The controlled restart works for standalone and NSSM-hosted deployments because it does not terminate the service process.
