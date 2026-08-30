const test = require('node:test');
const assert = require('node:assert/strict');

const theme = require('../static/theme.js');

test('theme normalization keeps the Ping palette contract bounded', () => {
  assert.deepEqual(theme.normalize({
    color_scheme: 'orbit-violet',
    density: 'compact',
    motion: 'reduced',
  }), {
    schema_version: 1,
    color_scheme: 'orbit-violet',
    density: 'compact',
    motion: 'reduced',
  });
  assert.deepEqual(theme.normalize({
    color_scheme: 'splunk-dark',
    density: 'tiny',
    motion: 'animated',
  }), theme.defaults);
});

test('theme application uses shared root data attributes and light color scheme', () => {
  const root = { dataset: {}, style: {} };
  const applied = theme.apply({ documentElement: root }, {
    color_scheme: 'daylight',
    density: 'compact',
    motion: 'system',
  });
  assert.equal(root.dataset.colorScheme, 'daylight');
  assert.equal(root.dataset.density, 'compact');
  assert.equal(root.dataset.motion, 'system');
  assert.equal(root.style.colorScheme, 'light');
  assert.equal(applied.color_scheme, 'daylight');
});

test('saved cache is normalized and corrupt cache fails safe', () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  theme.cache(storage, { color_scheme: 'radar-coral', density: 'comfortable', motion: 'reduced' });
  assert.equal(theme.readCached(storage).color_scheme, 'radar-coral');
  values.set(theme.storageKey, '{broken');
  assert.deepEqual(theme.readCached(storage), theme.defaults);
});
