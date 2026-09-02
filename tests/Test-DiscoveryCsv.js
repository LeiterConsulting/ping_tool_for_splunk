'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const repoRoot = path.resolve(__dirname, '..');
const sourcePath = path.join(repoRoot, 'go', 'internal', 'webui', 'static', 'discovery_csv.js');
const context = {};
vm.createContext(context);
vm.runInContext(fs.readFileSync(sourcePath, 'utf8'), context, { filename: sourcePath });

const exporter = context.PingMonitorDiscoveryCsv;
assert.ok(exporter, 'CSV exporter should be exposed');
assert.equal(exporter.columns.length, 40, 'export schema should contain 40 fields');
assert.equal(new Set(exporter.columns).size, exporter.columns.length, 'export columns should be unique');

for (const required of [
  'ip', 'hostname', 'fqdn', 'asset_id', 'device_mode', 'monitoring_enabled',
  'alerting_enabled', 'dynamic_address', 'subnet_name', 'subnet_vlan', 'subnet_location',
  'dns_status', 'discovery_scan_id', 'discovery_probe_backend', 'discovery_latency_source',
]) {
  assert.ok(exporter.columns.includes(required), `export schema should contain ${required}`);
}

const items = [
  {
    ip: '=1+1',
    hostname: '+formula',
    fqdn: '-formula.example',
    group: '@formula',
    description: '\ttab-formula',
    additional_notes: 'quote " and comma, plus\r\nnewline',
    discovery_probe_backend: 'windows_icmp',
    discovery_latency_source: 'windows_icmp_rtt',
  },
  { ip: '192.0.2.2', hostname: 'unselected' },
  { ip: '192.0.2.3', hostname: 'selected', vendor: null },
];

const csv = exporter.build(items, [0, 2]);
assert.equal(csv.charCodeAt(0), 0xFEFF, 'CSV should begin with a UTF-8 BOM');
assert.ok(csv.endsWith('\r\n'), 'CSV should end with CRLF');
assert.ok(csv.includes('"discovery_probe_backend"'), 'header should contain probe provenance');
assert.ok(csv.includes('"discovery_latency_source"'), 'header should contain latency provenance');
assert.ok(csv.includes("'=1+1"), 'equals-prefixed content should be neutralized');
assert.ok(csv.includes("'+formula"), 'plus-prefixed content should be neutralized');
assert.ok(csv.includes("'-formula.example"), 'minus-prefixed content should be neutralized');
assert.ok(csv.includes("'@formula"), 'at-prefixed content should be neutralized');
assert.ok(csv.includes("'\ttab-formula"), 'tab-prefixed content should be neutralized');
assert.ok(csv.includes('quote "" and comma, plus\r\nnewline'), 'quotes and embedded newlines should be escaped');
assert.ok(!csv.includes('unselected'), 'unselected rows should not be exported');
assert.ok(csv.includes('"selected"'), 'selected rows should be exported');

const expectedPhysicalLines = 4; // header, first row split by its embedded newline, and second row
assert.equal(csv.trimEnd().split(/\r?\n/).length, expectedPhysicalLines, 'CSV physical line layout should remain stable');

const emptySelection = exporter.build(items, []);
assert.equal(emptySelection.slice(1), `${exporter.columns.map(exporter.csvCell).join(',')}\r\n`, 'empty selection should emit only the header');

console.log(JSON.stringify({
  passed: true,
  columns: exporter.columns.length,
  selectedRows: 2,
  formulaNeutralization: true,
  quoteAndNewlineEscaping: true,
}, null, 2));
