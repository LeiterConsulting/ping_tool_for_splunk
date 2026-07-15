# Ping Monitor for Splunk 2.9.2 (build 41)

- Fixes saved reports scanning zero events because the event source expanded to `search search ...` under saved-search dispatch.
- Moves operational reports, health lookup generation, and packaged alerts to the configured metrics index, matching the proven Metrics Summary execution path.
- Avoids dynamic event subsearch materialization and its 50,000-result truncation while retaining explicit streaming macros for historical event searches.
- Keeps collector-emitted state authoritative for current health and preserves intentional empty-state rows when no endpoint meets a review threshold.

Package: `splunk_app/dist/ping_monitor_2.9.2_build41_20260715.tar.gz`
