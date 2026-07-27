(function exposeDiscoveryCsv(root) {
  'use strict';

  const columns = Object.freeze([
    'ip', 'hostname', 'fqdn', 'group', 'description', 'entitytype', 'device', 'vendor',
    'additional_notes', 'dev', 'monitoring_enabled', 'maintenance_until', 'maintenance_reason',
    'dns_status', 'dns_forward_confirmed', 'discovered_at', 'discovery_scan_id',
    'discovery_source', 'discovery_latency_ms',
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
