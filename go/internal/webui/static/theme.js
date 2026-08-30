(function initializePingMonitorTheme(globalObject) {
  'use strict';

  const storageKey = 'pingmonitor.ui.preferences.v1';
  const defaults = Object.freeze({
    schema_version: 1,
    color_scheme: 'signal-blue',
    density: 'comfortable',
    motion: 'system',
  });
  const colorSchemes = new Set(['signal-blue', 'orbit-violet', 'radar-coral', 'daylight', 'high-visibility']);
  const densities = new Set(['comfortable', 'compact']);
  const motionModes = new Set(['system', 'reduced']);

  function normalize(candidate = {}) {
    const source = candidate && typeof candidate === 'object' ? candidate : {};
    return {
      schema_version: 1,
      color_scheme: colorSchemes.has(source.color_scheme) ? source.color_scheme : defaults.color_scheme,
      density: densities.has(source.density) ? source.density : defaults.density,
      motion: motionModes.has(source.motion) ? source.motion : defaults.motion,
      ...(typeof source.updated_at === 'string' && source.updated_at ? { updated_at: source.updated_at } : {}),
    };
  }

  function apply(documentObject, candidate) {
    if (!documentObject?.documentElement) {
      return normalize(candidate);
    }
    const preferences = normalize(candidate);
    const root = documentObject.documentElement;
    root.dataset.colorScheme = preferences.color_scheme;
    root.dataset.density = preferences.density;
    root.dataset.motion = preferences.motion;
    root.style.colorScheme = preferences.color_scheme === 'daylight' ? 'light' : 'dark';
    return preferences;
  }

  function readCached(storage) {
    if (!storage) {
      return normalize(defaults);
    }
    try {
      const raw = storage.getItem(storageKey);
      return raw ? normalize(JSON.parse(raw)) : normalize(defaults);
    } catch (_error) {
      return normalize(defaults);
    }
  }

  function cache(storage, candidate) {
    const preferences = normalize(candidate);
    if (!storage) {
      return preferences;
    }
    try {
      storage.setItem(storageKey, JSON.stringify(preferences));
    } catch (_error) {
      // Appearance still works for this page even when storage is unavailable.
    }
    return preferences;
  }

  const api = Object.freeze({
    storageKey,
    defaults,
    normalize,
    apply,
    readCached,
    cache,
  });

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = api;
  }
  if (globalObject) {
    globalObject.PingMonitorTheme = api;
    let storage = null;
    try {
      storage = globalObject.localStorage;
    } catch (_error) {
      // A locked-down browser may deny storage while still allowing the UI.
    }
    apply(globalObject.document, readCached(storage));
  }
})(typeof window !== 'undefined' ? window : null);
