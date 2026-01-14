# Metrics Time Resolution Guide

When using Splunk metrics for ping monitoring, you may notice that data appears to update only every 5 minutes even though the script sends metrics every cycle (default: 60 seconds). This guide explains how to adjust time resolution for both the metrics index and dashboard queries.

---

## Understanding the Issue

Splunk metrics indexes have a **minimum time span** (`minSpanAllowed`) that controls the finest granularity available in searches. By default, this is often set to `5m` or `10m`, which means:

- Data ingested every 60 seconds is still stored
- But `mstats` queries aggregate to the minimum span
- Dashboard tiles appear to update only at 5-minute intervals

---

## Solution 1: Adjust Metrics Index Configuration

To allow finer-grained queries, modify your metrics index settings in Splunk.

### indexes.conf

```ini
[ping_metrics]
datatype = metric
minSpanAllowed = 1s
```

After editing, restart Splunk or run:
```
splunk reload index
```

### Verify Current Setting

```spl
| rest /services/data/indexes/ping_metrics 
| table title datatype minSpanAllowed
```

---

## Solution 2: Adjust Dashboard Queries

Even with `minSpanAllowed = 1s`, dashboard queries may still use a larger span by default. Update your `mstats` queries to specify the desired span.

### Before (default span, often 5m)

```spl
| mstats avg(ping.avg_latency_ms) WHERE index=ping_metrics BY hostname
```

### After (explicit 1-minute span)

```spl
| mstats avg(ping.avg_latency_ms) WHERE index=ping_metrics BY hostname span=1m
```

---

## Modifying Dashboard Tiles

### XML Dashboards (Classic)

Find the `<search>` element for each panel and add/modify the `span` parameter:

```xml
<panel>
  <title>Average Latency</title>
  <chart>
    <search>
      <query>
| mstats avg(ping.avg_latency_ms) WHERE index=ping_metrics BY hostname span=1m
      </query>
      <earliest>-60m@m</earliest>
      <latest>now</latest>
    </search>
    <!-- chart options -->
  </chart>
</panel>
```

### Dashboard Studio (JSON)

In the data source definition, modify the query:

```json
{
  "dataSources": {
    "latency_metrics": {
      "type": "ds.search",
      "options": {
        "query": "| mstats avg(ping.avg_latency_ms) WHERE index=ping_metrics BY hostname span=1m",
        "queryParameters": {
          "earliest": "-60m@m",
          "latest": "now"
        }
      }
    }
  }
}
```

---

## Common Span Values

| Span | Use Case |
|------|----------|
| `span=1s` | Real-time debugging (high overhead) |
| `span=1m` | Standard monitoring with 60s cycle |
| `span=5m` | Reduced query load, trend analysis |
| `span=1h` | Long-term dashboards, capacity planning |

**Recommendation:** Match your `span` to your `cycle_interval_seconds` in config. If pinging every 60 seconds, use `span=1m`.

---

## Modifying the Ping Monitor Splunk App

The Ping Monitor app includes dashboards in:
```
splunk_app/ping_monitor/default/data/ui/views/
```

### ping_overview.xml

1. Open `ping_overview.xml` in a text editor
2. Find all `mstats` queries
3. Add `span=1m` (or your preferred interval)

**Example change:**

```xml
<!-- Before -->
<query>| mstats avg(ping.avg_latency_ms) WHERE index=$metrics_index$ BY hostname</query>

<!-- After -->
<query>| mstats avg(ping.avg_latency_ms) WHERE index=$metrics_index$ BY hostname span=1m</query>
```

4. Save and refresh the dashboard in Splunk

### Using Local Overrides (Recommended)

Instead of editing `default/`, create overrides in `local/` to preserve upgrades:

```
splunk_app/ping_monitor/local/data/ui/views/ping_overview.xml
```

Copy the entire file from `default/` and make your modifications there.

---

## Verifying Data Resolution

### Check Raw Data Points

```spl
| mstats count WHERE index=ping_metrics BY _time span=1s
| head 100
```

If you see data points at 1-second intervals, data is being stored at full resolution.

### Check Actual Ingestion Rate

```spl
| mstats count WHERE index=ping_metrics span=1m
| timechart span=1m count
```

You should see consistent counts matching your endpoint count × cycles per minute.

---

## Performance Considerations

| Setting | Impact |
|---------|--------|
| `span=1s` | Highest query cost, most data points |
| `span=1m` | Good balance for 60s monitoring cycles |
| `span=5m` | Lower cost, acceptable for most dashboards |

For dashboards with many panels or long time ranges, consider:
- Using `span=5m` for overview panels
- Using `span=1m` only for drilldown/detail views
- Setting appropriate `earliest`/`latest` limits

---

## Troubleshooting

### Data shows but only at 5m intervals

1. Check `minSpanAllowed` on the index
2. Verify `span=` is set in your query
3. Confirm the query span isn't larger than `minSpanAllowed`

### No metrics data at all

1. Verify `metrics.enabled = $true` in config
2. Check `metrics.index` matches your actual index name
3. Confirm the index is type `metric` (not `event`)
4. Check HEC token has write access to the metrics index

### Query returns "no results"

```spl
| mcatalog values(metric_name) WHERE index=ping_metrics
```

If this returns nothing, metrics aren't reaching the index.

---

## Quick Reference

### Set 1-minute resolution everywhere

1. **Index:** `minSpanAllowed = 1s` in indexes.conf
2. **Queries:** Add `span=1m` to all `mstats` commands
3. **Config:** Ensure `cycle_interval_seconds = 60`

### Example complete mstats query

```spl
| mstats 
    avg(ping.avg_latency_ms) AS avg_latency
    max(ping.max_latency_ms) AS max_latency
    avg(ping.packet_loss_pct) AS packet_loss
  WHERE index=ping_metrics 
  BY hostname, target_ip, group
  span=1m
| where packet_loss > 0
```

---

*Last updated: 14 Jan 2026*
