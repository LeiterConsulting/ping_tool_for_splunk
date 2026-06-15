const state = {
  endpoints: [],
  filter: 'all',
  search: '',
  status: null,
};

const elements = {
  versionText: document.getElementById('version-text'),
  modePill: document.getElementById('mode-pill'),
  endpointPath: document.getElementById('endpoint-path'),
  devRefresh: document.getElementById('dev-refresh'),
  errorBanner: document.getElementById('error-banner'),
  summaryTotal: document.getElementById('summary-total'),
  summaryProduction: document.getElementById('summary-production'),
  summaryDev: document.getElementById('summary-dev'),
  summaryGroups: document.getElementById('summary-groups'),
  endpointRows: document.getElementById('endpoint-rows'),
  devRows: document.getElementById('dev-rows'),
  searchInput: document.getElementById('search-input'),
  refreshButton: document.getElementById('refresh-button'),
  filterButtons: Array.from(document.querySelectorAll('[data-filter]')),
};

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function filterEndpoints() {
  const search = state.search.trim().toLowerCase();

  return state.endpoints.filter((endpoint) => {
    if (state.filter === 'dev' && !endpoint.dev) {
      return false;
    }
    if (state.filter === 'production' && endpoint.dev) {
      return false;
    }
    if (!search) {
      return true;
    }

    return [
      endpoint.ip,
      endpoint.hostname,
      endpoint.group,
      endpoint.description,
      endpoint.entitytype,
      endpoint.device,
      endpoint.vendor,
      endpoint.additional_notes,
    ].some((value) => String(value || '').toLowerCase().includes(search));
  });
}

function renderSummary() {
  const groups = new Set(state.endpoints.map((endpoint) => (endpoint.group || 'default').trim() || 'default'));
  const devCount = state.endpoints.filter((endpoint) => endpoint.dev).length;

  elements.summaryTotal.textContent = String(state.endpoints.length);
  elements.summaryProduction.textContent = String(state.endpoints.length - devCount);
  elements.summaryDev.textContent = String(devCount);
  elements.summaryGroups.textContent = String(groups.size);
}

function renderInventory() {
  const filtered = filterEndpoints();

  if (filtered.length === 0) {
    elements.endpointRows.innerHTML = '<tr><td colspan="7" class="empty-cell">No endpoints match the current filter.</td></tr>';
  } else {
    elements.endpointRows.innerHTML = filtered.map((endpoint) => `
      <tr>
        <td>${escapeHtml(endpoint.ip)}</td>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.entitytype || '-')}</td>
        <td>${escapeHtml(endpoint.device || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
        <td><span class="mode-badge ${endpoint.dev ? 'dev' : 'production'}">${endpoint.dev ? 'Dev' : 'Production'}</span></td>
      </tr>
    `).join('');
  }

  const devEndpoints = state.endpoints.filter((endpoint) => endpoint.dev);
  if (devEndpoints.length === 0) {
    elements.devRows.innerHTML = '<tr><td colspan="4" class="empty-cell">No dev endpoints are currently defined.</td></tr>';
  } else {
    elements.devRows.innerHTML = devEndpoints.map((endpoint) => `
      <tr>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.description || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
      </tr>
    `).join('');
  }
}

function renderStatus() {
  if (!state.status) {
    return;
  }

  elements.versionText.textContent = `${state.status.version} UI preview`;
  elements.modePill.textContent = state.status.mode === 'read-only' ? 'Read-only' : state.status.mode;
  elements.endpointPath.textContent = state.status.endpoints_path;
}

function setError(message) {
  if (!message) {
    elements.errorBanner.classList.add('hidden');
    elements.errorBanner.textContent = '';
    return;
  }

  elements.errorBanner.textContent = message;
  elements.errorBanner.classList.remove('hidden');
}

async function fetchJson(path) {
  const response = await fetch(path, { cache: 'no-store' });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || `Request failed for ${path}`);
  }
  return payload;
}

async function refreshData() {
  elements.refreshButton.disabled = true;
  elements.refreshButton.textContent = 'Refreshing...';

  try {
    const [status, endpoints] = await Promise.all([
      fetchJson('/api/status'),
      fetchJson('/api/endpoints'),
    ]);

    state.status = status;
    state.endpoints = endpoints.items || [];
    renderStatus();
    renderSummary();
    renderInventory();
    elements.devRefresh.textContent = `Updated ${new Date(endpoints.generated_at).toLocaleTimeString()}`;
    setError('');
  } catch (error) {
    setError(error instanceof Error ? error.message : 'Unable to load endpoint data.');
    elements.endpointRows.innerHTML = '<tr><td colspan="7" class="empty-cell">Unable to load endpoints.</td></tr>';
    elements.devRows.innerHTML = '<tr><td colspan="4" class="empty-cell">Unable to load dev endpoints.</td></tr>';
  } finally {
    elements.refreshButton.disabled = false;
    elements.refreshButton.textContent = 'Refresh Data';
  }
}

elements.searchInput.addEventListener('input', (event) => {
  state.search = event.target.value;
  renderInventory();
});

elements.refreshButton.addEventListener('click', () => {
  refreshData();
});

elements.filterButtons.forEach((button) => {
  button.addEventListener('click', () => {
    state.filter = button.dataset.filter || 'all';
    elements.filterButtons.forEach((item) => item.classList.toggle('active', item === button));
    renderInventory();
  });
});

refreshData();