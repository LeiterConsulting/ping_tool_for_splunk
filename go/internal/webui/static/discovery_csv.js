(function exposeDiscoveryCsv(root) {
  'use strict';

  const columns = Object.freeze([
    'ip', 'hostname', 'fqdn', 'group', 'description', 'entitytype', 'device', 'vendor',
    'additional_notes', 'asset_id', 'device_mode', 'dev', 'monitoring_enabled', 'alerting_enabled',
    'alerting_reason', 'maintenance_until', 'maintenance_reason', 'dynamic_address', 'classification_source',
    'discovery_review_state', 'discovery_reviewed_at', 'discovery_review_note',
    'subnet_id', 'subnet_name', 'subnet_vlan', 'subnet_location', 'addressing_mode', 'routing_domain',
    'dns_status', 'dns_forward_confirmed', 'discovered_at', 'discovery_scan_id',
    'discovery_source', 'discovery_latency_ms', 'discovery_probe_backend',
    'discovery_latency_source', 'discovery_latency_resolution_ms', 'discovery_latency_censored',
    'discovery_latency_upper_bound_ms', 'discovery_probe_elapsed_ms',
  ]);

  function safeCellText(value) {
    const text = String(value ?? '');
    return /^[=+\-@\t\r]/.test(text) ? `'${text}` : text;
  }

  function csvCell(value) {
    return `"${safeCellText(value).replaceAll('"', '""')}"`;
  }

  function build(items, indexes) {
    const source = Array.isArray(items) ? items : [];
    const selected = Array.isArray(indexes) ? indexes : [];
    const lines = [columns.map(csvCell).join(',')];
    selected.forEach((index) => {
      const endpoint = source[index] || {};
      lines.push(columns.map((column) => csvCell(endpoint[column])).join(','));
    });
    return `\uFEFF${lines.join('\r\n')}\r\n`;
  }

  root.PingMonitorDiscoveryCsv = Object.freeze({ columns, csvCell, build });
}(typeof globalThis === 'undefined' ? this : globalThis));
