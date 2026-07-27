const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(
  path.join(__dirname, '..', 'static', 'discovery_csv.js'),
  'utf8',
);
const sandbox = {};
vm.runInNewContext(source, sandbox, { filename: 'discovery_csv.js' });
const exporter = sandbox.PingMonitorDiscoveryCsv;

test('exports the complete discovery schema with RFC 4180 escaping', () => {
  const csv = exporter.build([
    {
      ip: '192.0.2.10',
      hostname: 'router, "east"',
      fqdn: 'router-east.example.test',
      discovery_scan_id: 'scan-1',
      discovery_latency_ms: 0.42,
    },
  ], [0]);

  assert.ok(csv.startsWith('\uFEFF"ip","hostname","fqdn"'));
  assert.match(csv, /"router, ""east"""/);
  assert.match(csv, /"scan-1","",\"0\.42\"/);
  assert.ok(csv.endsWith('\r\n'));
  assert.equal(csv.split('\r\n').filter(Boolean).length, 2);
});

test('neutralizes spreadsheet formulas in discovery-controlled text', () => {
  const csv = exporter.build([{ ip: '192.0.2.11', hostname: '=WEBSERVICE("bad")' }], [0]);
  assert.match(csv, /"'=WEBSERVICE\(""bad""\)"/);
});

test('exports only requested rows', () => {
  const csv = exporter.build([
    { ip: '192.0.2.1' },
    { ip: '192.0.2.2' },
    { ip: '192.0.2.3' },
  ], [2, 0]);
  const rows = csv.replace(/^\uFEFF/, '').trim().split('\r\n');
  assert.equal(rows.length, 3);
  assert.match(rows[1], /^"192\.0\.2\.3"/);
  assert.match(rows[2], /^"192\.0\.2\.1"/);
});
