const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(
  path.join(__dirname, '..', 'static', 'review_workflow.js'),
  'utf8',
);
const sandbox = {};
vm.runInNewContext(source, sandbox, { filename: 'review_workflow.js' });
const workflow = sandbox.PingMonitorDiscoveryReview;

test('unknown observations remain Needs Review with an unassigned mode', () => {
  const endpoint = { ip: '192.0.2.10', hostname: 'new-device' };
  assert.equal(workflow.reviewState(endpoint), 'needs_review');
  assert.equal(workflow.deviceMode(endpoint), 'unassigned');
  assert.deepEqual(Array.from(workflow.unassignedSelection([endpoint], [0])), [0]);
});

test('explicit Production or Maintenance satisfies the review mode gate', () => {
  const items = [
    { discovery_review_state: 'needs_review', device_mode: 'production' },
    { discovery_review_state: 'needs_review', device_mode: 'maintenance' },
  ];
  assert.equal(workflow.deviceMode(items[0]), 'production');
  assert.equal(workflow.deviceMode(items[1]), 'maintenance');
  assert.deepEqual(Array.from(workflow.unassignedSelection(items, [0, 1])), []);
});

test('durable review filtering preserves source indexes', () => {
  const items = [
    { discovery_review_state: 'needs_review' },
    { discovery_review_state: 'deferred' },
    { discovery_review_state: 'ignored' },
    { discovery_review_state: 'approved' },
    { _review_staged: true },
  ];
  assert.deepEqual(Array.from(workflow.indexedByReview(items, 'needs_review'), row => row.index), [0]);
  assert.deepEqual(Array.from(workflow.indexedByReview(items, 'deferred'), row => row.index), [1]);
  assert.deepEqual(Array.from(workflow.indexedByReview(items, 'ignored'), row => row.index), [2]);
  assert.deepEqual(Array.from(workflow.indexedByReview(items, 'approved'), row => row.index), [3]);
  assert.deepEqual(Array.from(workflow.indexedByReview(items, 'staged'), row => row.index), [4]);
  assert.equal(workflow.indexedByReview(items, 'all').length, 5);
});

test('staging records approval metadata without mutating scan evidence', () => {
  const observed = { ip: '192.0.2.20', hostname: 'laptop', device_mode: 'production' };
  const staged = workflow.markStaged(observed, '2026-08-25T12:00:00Z', 'CMDB owner confirmed');
  assert.notEqual(staged, observed);
  assert.equal(observed.discovery_review_state, undefined);
  assert.equal(staged.discovery_review_state, 'approved');
  assert.equal(staged.discovery_reviewed_at, '2026-08-25T12:00:00Z');
  assert.equal(staged.discovery_review_note, 'CMDB owner confirmed');
});
