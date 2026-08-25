(function attachDiscoveryReviewWorkflow(root) {
  const validReviewStates = new Set(['needs_review', 'approved', 'deferred', 'ignored']);
  const validDeviceModes = new Set(['production', 'maintenance', 'legacy_dev']);

  function reviewState(endpoint) {
    if (endpoint && endpoint._review_staged) {
      return 'staged';
    }
    const explicit = String(endpoint && endpoint.discovery_review_state || '').trim().toLowerCase();
    return validReviewStates.has(explicit) ? explicit : 'needs_review';
  }

  function deviceMode(endpoint) {
    const explicit = String(endpoint && endpoint.device_mode || '').trim().toLowerCase();
    if (validDeviceModes.has(explicit)) {
      return explicit;
    }
    if (endpoint && endpoint.dev) {
      return 'legacy_dev';
    }
    if (['approved', 'staged'].includes(reviewState(endpoint))) {
      return 'production';
    }
    return 'unassigned';
  }

  function indexedByReview(items, filter) {
    const selectedFilter = String(filter || 'needs_review').trim().toLowerCase();
    return (items || [])
      .map((endpoint, index) => ({ endpoint, index }))
      .filter(({ endpoint }) => selectedFilter === 'all' || reviewState(endpoint) === selectedFilter);
  }

  function unassignedSelection(items, indexes) {
    return (indexes || []).filter((index) => items[index] && deviceMode(items[index]) === 'unassigned');
  }

  function markStaged(endpoint, reviewedAt, note) {
    const candidate = { ...(endpoint || {}) };
    candidate.discovery_review_state = 'approved';
    candidate.discovery_reviewed_at = reviewedAt;
    candidate.discovery_review_note = String(note || '').trim();
    return candidate;
  }

  root.PingMonitorDiscoveryReview = {
    reviewState,
    deviceMode,
    indexedByReview,
    unassignedSelection,
    markStaged,
  };
}(typeof globalThis === 'undefined' ? window : globalThis));
