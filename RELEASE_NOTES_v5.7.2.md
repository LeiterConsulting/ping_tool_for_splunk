# Ping Monitor v5.7.2 Release Notes

Release date: 20 July 2026

Version 5.7.2 fixes an upgrade-time browser cache issue that could hide the v5.7.1 **Restart Collector** control while the collector API correctly reported **Restart required**.

## Admin UI Asset Consistency

- Adds the runtime version to the `app.js` and `app.css` asset URLs so every binary upgrade receives a distinct browser cache key.
- Sends `Cache-Control: no-store`, `Pragma: no-cache`, and an immediate expiry for the HTML shell and embedded static assets.
- Gives the conditional **Restart Collector** button a stronger amber treatment so it is visually associated with the restart-required state.
- Adds regression coverage for versioned asset URLs and cache-prevention response headers.

## Compatibility

- The monitor engine, configuration schema, event schema, metrics schema, and controlled-restart API are unchanged from v5.7.1.
- Splunk app 2.9.2 build 41 remains current and requires no migration.
