const state = {
  status: null,
  endpoints: [],
  savedEndpoints: [],
  filter: 'all',
  search: '',
  selectedEndpointIndex: -1,
  selectedEndpointIndices: new Set(),
  savedConfig: null,
  tables: {
    endpoint: {
      page: 1,
      pageSize: 10,
      sortKey: 'ip',
      sortDir: 'asc',
    },
    discovery: {
      page: 1,
      pageSize: 10,
      sortKey: 'ip',
      sortDir: 'asc',
    },
  },
  discovery: {
    available: false,
    running: false,
    runState: 'Idle',
    progressSummary: 'No discovery run yet.',
    items: [],
    summary: null,
    logs: '',
    durationMs: 0,
    selectedIndices: new Set(),
    mergeMode: 'skip_existing',
  },
};

let settingsHelpTrigger = null;

const elements = {
  sidebarConfigSource: document.getElementById('sidebar-config-source'),
  sidebarDiscoveryStatus: document.getElementById('sidebar-discovery-status'),
  versionText: document.getElementById('version-text'),
  modePill: document.getElementById('mode-pill'),
  configPathChip: document.getElementById('config-path-chip'),
  endpointPath: document.getElementById('endpoint-path'),
  configSourceLabel: document.getElementById('config-source-label'),
  configSourceCopy: document.getElementById('config-source-copy'),
  discoveryStatusLabel: document.getElementById('discovery-status-label'),
  discoveryStatusCopy: document.getElementById('discovery-status-copy'),
  contentScroll: document.querySelector('.content-scroll'),
  refreshButton: document.getElementById('refresh-button'),
  summaryTotal: document.getElementById('summary-total'),
  summaryProduction: document.getElementById('summary-production'),
  summaryDev: document.getElementById('summary-dev'),
  summaryGroups: document.getElementById('summary-groups'),
  endpointBanner: document.getElementById('endpoint-banner'),
  endpointDirtyPill: document.getElementById('endpoint-dirty-pill'),
  endpointSelectionStatus: document.getElementById('endpoint-selection-status'),
  endpointRows: document.getElementById('endpoint-rows'),
  endpointForm: document.getElementById('endpoint-form'),
  endpointSelectionLabel: document.getElementById('endpoint-selection-label'),
  addEndpointButton: document.getElementById('add-endpoint-button'),
  selectAllEndpointsButton: document.getElementById('select-all-endpoints-button'),
  deselectAllEndpointsButton: document.getElementById('deselect-all-endpoints-button'),
  markSelectedDevButton: document.getElementById('mark-selected-dev-button'),
  markSelectedProductionButton: document.getElementById('mark-selected-production-button'),
  deleteEndpointButton: document.getElementById('delete-endpoint-button'),
  resetEndpointsButton: document.getElementById('reset-endpoints-button'),
  saveEndpointsButton: document.getElementById('save-endpoints-button'),
  endpointPageSize: document.getElementById('endpoint-page-size'),
  endpointPrevPageButton: document.getElementById('endpoint-prev-page'),
  endpointPageStatus: document.getElementById('endpoint-page-status'),
  endpointNextPageButton: document.getElementById('endpoint-next-page'),
  endpointFields: {
    ip: document.getElementById('endpoint-ip'),
    hostname: document.getElementById('endpoint-hostname'),
    group: document.getElementById('endpoint-group'),
    description: document.getElementById('endpoint-description'),
    entitytype: document.getElementById('endpoint-entitytype'),
    device: document.getElementById('endpoint-device'),
    vendor: document.getElementById('endpoint-vendor'),
    additional_notes: document.getElementById('endpoint-notes'),
    dev: document.getElementById('endpoint-dev'),
  },
  devRefresh: document.getElementById('dev-refresh'),
  devRows: document.getElementById('dev-rows'),
  searchInput: document.getElementById('search-input'),
  filterButtons: Array.from(document.querySelectorAll('[data-filter]')),
  navLinks: Array.from(document.querySelectorAll('.nav-item[href^="#"]')),
  discoveryBanner: document.getElementById('discovery-banner'),
  discoveryAvailability: document.getElementById('discovery-availability'),
  discoveryRunState: document.getElementById('discovery-run-state'),
  discoverySummary: document.getElementById('discovery-summary'),
  discoveryLogs: document.getElementById('discovery-logs'),
  discoveryRows: document.getElementById('discovery-rows'),
  runDiscoveryButton: document.getElementById('run-discovery-button'),
  selectAllDiscoveryButton: document.getElementById('select-all-discovery-button'),
  deselectAllDiscoveryButton: document.getElementById('deselect-all-discovery-button'),
  markDiscoveryDevButton: document.getElementById('mark-discovery-dev-button'),
  markDiscoveryProductionButton: document.getElementById('mark-discovery-production-button'),
  addDiscoverySelectedButton: document.getElementById('add-discovery-selected-button'),
  discoverySelectionStatus: document.getElementById('discovery-selection-status'),
  discoveryTableStatus: document.getElementById('discovery-table-status'),
  discoveryMergeMode: document.getElementById('discovery-merge-mode'),
  discoveryPageSize: document.getElementById('discovery-page-size'),
  discoveryPrevPageButton: document.getElementById('discovery-prev-page'),
  discoveryPageStatus: document.getElementById('discovery-page-status'),
  discoveryNextPageButton: document.getElementById('discovery-next-page'),
  discoveryInputs: {
    targetNetwork: document.getElementById('discovery-target-network'),
    subnetMask: document.getElementById('discovery-subnet-mask'),
    timeoutMs: document.getElementById('discovery-timeout-ms'),
    throttleLimit: document.getElementById('discovery-throttle-limit'),
  },
  tableSortButtons: Array.from(document.querySelectorAll('.table-sort')),
  settingsBanner: document.getElementById('settings-banner'),
  outputTestDetails: document.getElementById('output-test-details'),
  settingsSourceChip: document.getElementById('settings-source-chip'),
  settingsForm: document.getElementById('settings-form'),
  testHECButton: document.getElementById('test-hec-button'),
  testMetricsButton: document.getElementById('test-metrics-button'),
  reloadConfigButton: document.getElementById('reload-config-button'),
  resetConfigButton: document.getElementById('reset-config-button'),
  saveConfigButton: document.getElementById('save-config-button'),
  settingsFields: {
    pingsPerCycle: document.getElementById('cfg-pings-per-cycle'),
    cycleInterval: document.getElementById('cfg-cycle-interval'),
    timeoutMs: document.getElementById('cfg-timeout-ms'),
    parallelThreads: document.getElementById('cfg-parallel-threads'),
    emitIndividualPings: document.getElementById('cfg-emit-individual-pings'),
    outputMode: document.getElementById('cfg-output-mode'),
    logPath: document.getElementById('cfg-log-path'),
    logRotation: document.getElementById('cfg-log-rotation'),
    pingMode: document.getElementById('cfg-ping-mode'),
    diagnosticsEnabled: document.getElementById('cfg-diagnostics-enabled'),
    handleProbeMode: document.getElementById('cfg-handle-probe-mode'),
    emitMemoryStats: document.getElementById('cfg-emit-memory-stats'),
    hecEnabled: document.getElementById('cfg-hec-enabled'),
    hecURL: document.getElementById('cfg-hec-url'),
    hecToken: document.getElementById('cfg-hec-token'),
    hecIndex: document.getElementById('cfg-hec-index'),
    hecSourcetype: document.getElementById('cfg-hec-sourcetype'),
    hecVerifySSL: document.getElementById('cfg-hec-verify-ssl'),
    hecSSLProtocol: document.getElementById('cfg-hec-ssl-protocol'),
    hecBatchSize: document.getElementById('cfg-hec-batch-size'),
    hecDropOnFailure: document.getElementById('cfg-hec-drop-on-failure'),
    hecMaxBufferEvents: document.getElementById('cfg-hec-max-buffer-events'),
    hecMaxBufferBytes: document.getElementById('cfg-hec-max-buffer-bytes'),
    hecRetryEnabled: document.getElementById('cfg-hec-retry-enabled'),
    hecMaxAttempts: document.getElementById('cfg-hec-max-attempts'),
    hecBaseDelayMs: document.getElementById('cfg-hec-base-delay-ms'),
    hecJitterPct: document.getElementById('cfg-hec-jitter-pct'),
    hecBackoff: document.getElementById('cfg-hec-backoff'),
    hecRetryCount: document.getElementById('cfg-hec-retry-count'),
    hecRetryDelayMs: document.getElementById('cfg-hec-retry-delay-ms'),
    hecDeadLetterPath: document.getElementById('cfg-hec-dead-letter-path'),
    hecDeadLetterRotation: document.getElementById('cfg-hec-dead-letter-rotation'),
    metricsEnabled: document.getElementById('cfg-metrics-enabled'),
    metricsMode: document.getElementById('cfg-metrics-mode'),
    metricsIndex: document.getElementById('cfg-metrics-index'),
    metricsHECURL: document.getElementById('cfg-metrics-hec-url'),
    metricsToken: document.getElementById('cfg-metrics-token'),
    metricsVerifySSL: document.getElementById('cfg-metrics-verify-ssl'),
    metricsSSLProtocol: document.getElementById('cfg-metrics-ssl-protocol'),
    metricsCompatMode: document.getElementById('cfg-metrics-compat-mode'),
    metricsSourcetype: document.getElementById('cfg-metrics-sourcetype'),
    metricsEventName: document.getElementById('cfg-metrics-event-name'),
    metricsUseMetricsIndex: document.getElementById('cfg-metrics-use-metrics-index'),
    metricsBatchSize: document.getElementById('cfg-metrics-batch-size'),
    metricsMaxBufferEvents: document.getElementById('cfg-metrics-max-buffer-events'),
    metricsMaxBufferBytes: document.getElementById('cfg-metrics-max-buffer-bytes'),
  },
};

const sectionHashes = ['#overview', '#inventory', '#devices', '#discovery', '#settings'];

const checkboxFormat = 'Checked or unchecked.';
const positiveIntegerFormat = 'Whole number, 1 or higher.';
const nonNegativeIntegerFormat = 'Whole number, 0 or higher.';
const filePathFormat = 'File path. Relative paths are resolved from the active deployment/config directory.';
const tlsProfileFormat = 'Text profile. Use Default or a specific TLS version such as Tls12 or Tls13.';
const byteSizeFormat = 'Text size such as 5MB, 256KB, 1GB, or a raw byte count.';

const settingsPanelHelp = {
  'Core Runtime': panelHelp(
    'Core Runtime',
    'Controls how often the monitor runs, how many samples each endpoint receives, and how much concurrency the runtime uses.',
    [
      'Use this card to balance result fidelity against cycle duration and network load.',
      'These values are written back to the active deployment config file the UI is editing.',
    ],
  ),
  'Output and Ping Engine': panelHelp(
    'Output and Ping Engine',
    'Controls where results are written, how local logs are rotated, and which ping strategy the runtime should use.',
    [
      'Choose this card when you need to change delivery mode, raw-versus-exec ping behavior, or local logging.',
      'The ping mode matters most on locked-down hosts where raw ICMP may be unavailable.',
    ],
  ),
  Diagnostics: panelHelp(
    'Diagnostics',
    'Turns on troubleshooting-oriented runtime output. These settings add operational visibility; they do not change ping math or endpoint state.',
    [
      'Enable diagnostics when you need more detail around ping failures, delivery issues, or startup behavior.',
      'Handle Probe Mode is a targeted troubleshooting selector and should normally stay on none.',
      'Emit Memory Stats adds runtime memory snapshots at startup and exit so you can compare resource usage over time.',
    ],
  ),
  'HEC Events': panelHelp(
    'HEC Events',
    'Configures direct event delivery to Splunk HEC, including TLS, buffering, retries, and dead-letter handling.',
    [
      'Use this card when output_mode includes hec or both.',
      'Most production tuning comes from the batch, buffer, retry, and TLS settings here.',
    ],
  ),
  'Metrics Output': panelHelp(
    'Metrics Output',
    'Configures the summary-to-metrics pipeline, including batching, compatibility mode, and native metrics-index behavior.',
    [
      'Use this card when you want mstats-friendly data or a dual event-plus-metrics deployment.',
      'Compatibility and metrics-index mode affect how Splunk should query the resulting payloads.',
    ],
  ),
};

const settingsFieldHelp = {
  'cfg-pings-per-cycle': helpTopic(
    'Pings Per Cycle',
    'Sets how many ping attempts the runtime sends to each endpoint during one monitoring cycle.',
    positiveIntegerFormat,
    ['Higher values improve packet-loss sampling but add traffic and can lengthen each cycle.'],
  ),
  'cfg-cycle-interval': helpTopic(
    'Cycle Interval (s)',
    'Sets the target spacing between monitoring cycles.',
    positiveIntegerFormat,
    ['Use longer intervals to reduce endpoint load on large fleets or slower links.'],
  ),
  'cfg-timeout-ms': helpTopic(
    'Timeout (ms)',
    'Controls how long each ping attempt waits before it is treated as failed.',
    'Whole number milliseconds, 100 or higher in the current UI.',
    ['This applies per ping attempt, so high values can noticeably lengthen a slow cycle.'],
  ),
  'cfg-parallel-threads': helpTopic(
    'Parallel Threads',
    'Sets how many endpoints the runtime can work on concurrently.',
    positiveIntegerFormat,
    ['Higher concurrency speeds up large runs but increases local CPU, socket, and network pressure.'],
  ),
  'cfg-emit-individual-pings': helpTopic(
    'Emit Per-Ping Events',
    'Adds one event per individual ping attempt in addition to the per-endpoint summary event.',
    checkboxFormat,
    ['Unchecked keeps output at summary-only volume, which is the lower-noise default for most long-running deployments.'],
  ),
  'cfg-output-mode': helpTopic(
    'Output Mode',
    'Chooses where the runtime writes monitoring results.',
    'One of: file, hec, both.',
    [],
    [
      'file writes only to the local NDJSON log file.',
      'hec sends only to the Splunk event HEC endpoint.',
      'both keeps local logging and HEC delivery active together.',
    ],
  ),
  'cfg-ping-mode': helpTopic(
    'Ping Mode',
    'Chooses which ping implementation the runtime should use on the host.',
    'One of: auto, raw, exec.',
    [],
    [
      'auto tries raw ICMP first and falls back to the OS ping command if needed.',
      'raw uses only Go raw ICMP.',
      'exec uses only the operating system ping command.',
    ],
  ),
  'cfg-log-path': helpTopic(
    'Log Path',
    'Sets the local file used when output_mode includes file.',
    filePathFormat,
    ['Relative paths are usually the safest choice for a drop-in deployment folder.'],
  ),
  'cfg-log-rotation': helpTopic(
    'Log Rotation (MB)',
    'Sets the maximum local file size before the runtime rotates the file-based output log.',
    positiveIntegerFormat,
    ['Use a lower value when local disk churn matters more than keeping a longer uninterrupted file history.'],
  ),
  'cfg-diagnostics-enabled': helpTopic(
    'Enable Runtime Diagnostics Output',
    'Turns on additional troubleshooting-oriented runtime logs for operational analysis.',
    checkboxFormat,
    ['Use this when you need more detail around failures, retries, or runtime behavior beyond the normal monitoring output.'],
  ),
  'cfg-handle-probe-mode': helpTopic(
    'Handle Probe Mode',
    'Stores the diagnostics probe selector used for targeted troubleshooting workflows.',
    'One of: none, hec_only, metrics_only.',
    [
      'Leave this on none for normal deployments.',
      'Use the output-specific values only when you are intentionally narrowing a diagnostic session to HEC or metrics behavior.',
    ],
  ),
  'cfg-emit-memory-stats': helpTopic(
    'Emit Memory Stats Snapshots',
    'Writes runtime memory and goroutine snapshots at startup and exit for resource troubleshooting.',
    checkboxFormat,
    ['This is most useful when you are investigating growth, leaks, or long-run stability questions.'],
  ),
  'cfg-hec-enabled': helpTopic(
    'Enable Splunk HEC Event Delivery',
    'Turns on direct event delivery to the Splunk event HEC endpoint.',
    checkboxFormat,
    ['This is only used when output_mode includes hec or both.'],
  ),
  'cfg-hec-url': helpTopic(
    'HEC URL',
    'Sets the full Splunk event HEC endpoint used for event delivery.',
    'Full URL ending in /services/collector/event.',
    ['Example: https://splunk.example.com:8088/services/collector/event'],
  ),
  'cfg-hec-token': helpTopic(
    'HEC Token',
    'Supplies the authorization token sent in the Splunk header for event delivery.',
    'Plain token string issued by Splunk HEC.',
    ['The UI masks the token, but the runtime stores and uses the underlying value from the config file.'],
  ),
  'cfg-hec-index': helpTopic(
    'HEC Index',
    'Overrides the target Splunk events index for the HEC event stream.',
    'Index name text.',
    ['Use an index that the HEC token is allowed to write to.'],
  ),
  'cfg-hec-sourcetype': helpTopic(
    'HEC Sourcetype',
    'Sets the sourcetype written with the event HEC stream.',
    'Sourcetype text such as ping_monitor.',
  ),
  'cfg-hec-ssl-protocol': helpTopic(
    'HEC SSL Protocol',
    'Pins the TLS protocol profile used for event HEC connections.',
    tlsProfileFormat,
    [],
    [
      'Default lets Go negotiate the best supported version.',
      'Accepted specific profiles include Tls10, Tls11, Tls12, and Tls13.',
    ],
  ),
  'cfg-hec-batch-size': helpTopic(
    'HEC Batch Size',
    'Sets how many event payloads are grouped into one HEC POST attempt.',
    positiveIntegerFormat,
    ['Larger batches reduce request count but can make retries and dead-letter writes heavier.'],
  ),
  'cfg-hec-max-buffer-events': helpTopic(
    'HEC Max Buffer Events',
    'Caps how many events the in-memory HEC buffer can hold while batching or riding out outages.',
    positiveIntegerFormat,
    ['This limit helps keep memory bounded when HEC is unavailable.'],
  ),
  'cfg-hec-max-buffer-bytes': helpTopic(
    'HEC Max Buffer Bytes',
    'Caps the in-memory HEC buffer size by bytes instead of event count.',
    byteSizeFormat,
    ['Use this with Max Buffer Events so both large payloads and large counts stay bounded.'],
  ),
  'cfg-hec-retry-count': helpTopic(
    'Retry Count',
    'Legacy retry setting for how many additional attempts to make after the first failed send.',
    nonNegativeIntegerFormat,
    ['Used only when structured retry is disabled. If structured retry is enabled, the runtime uses Retry Max Attempts instead.'],
  ),
  'cfg-hec-retry-delay-ms': helpTopic(
    'Retry Delay (ms)',
    'Legacy fixed delay between retry attempts when structured retry is disabled.',
    nonNegativeIntegerFormat,
    ['Ignored when structured retry is enabled.'],
  ),
  'cfg-hec-dead-letter-path': helpTopic(
    'Dead Letter Path',
    'Optional file used to preserve failed HEC payloads when drop_on_failure is enabled.',
    filePathFormat,
    ['Only used when Drop Event Batches When Delivery Fails is checked.'],
  ),
  'cfg-hec-dead-letter-rotation': helpTopic(
    'Dead Letter Rotation (MB)',
    'Rotates the dead-letter file once it reaches the configured size.',
    nonNegativeIntegerFormat,
    ['Only applies when a dead-letter path is set. Zero keeps the file unrotated.'],
  ),
  'cfg-hec-verify-ssl': helpTopic(
    'Verify TLS Certificates',
    'Controls whether the runtime validates the remote HEC server certificate.',
    checkboxFormat,
    ['Turn this off only for controlled troubleshooting or self-signed environments where you accept the risk.'],
  ),
  'cfg-hec-drop-on-failure': helpTopic(
    'Drop Event Batches When Delivery Fails',
    'Controls whether failed HEC batches are discarded from memory after a failed flush attempt.',
    checkboxFormat,
    [
      'Checked means the runtime clears the failed batch from memory after failure.',
      'If a dead-letter path is set, the failed payload is appended there before the in-memory buffer is cleared.',
      'Unchecked keeps the batch buffered for later retry attempts across cycles.',
    ],
  ),
  'cfg-hec-retry-enabled': helpTopic(
    'Enable Structured Retry Policy',
    'Turns on the newer retry policy with max attempts, base delay, jitter, and backoff selection.',
    checkboxFormat,
    ['When enabled, the structured retry settings below take precedence over the legacy Retry Count/Retry Delay fields.'],
  ),
  'cfg-hec-max-attempts': helpTopic(
    'Retry Max Attempts',
    'Sets the total number of send attempts in the structured retry policy, including the first try.',
    positiveIntegerFormat,
  ),
  'cfg-hec-base-delay-ms': helpTopic(
    'Retry Base Delay (ms)',
    'Sets the starting delay used by the structured retry policy before jitter and backoff are applied.',
    nonNegativeIntegerFormat,
  ),
  'cfg-hec-jitter-pct': helpTopic(
    'Retry Jitter %',
    'Reduces synchronized retry spikes by varying the retry delay by the configured percentage.',
    nonNegativeIntegerFormat,
    ['A value of 0 disables jitter.'],
  ),
  'cfg-hec-backoff': helpTopic(
    'Retry Backoff',
    'Chooses how structured retry delays grow between attempts.',
    'One of: fixed, exponential.',
    [],
    [
      'fixed keeps the same delay between attempts.',
      'exponential doubles the delay between attempts up to the runtime cap.',
    ],
  ),
  'cfg-metrics-enabled': helpTopic(
    'Enable Metrics Delivery',
    'Turns on the metrics output pipeline for summary data.',
    checkboxFormat,
    ['Use this when you want Splunk metrics ingestion in addition to or instead of event output.'],
  ),
  'cfg-metrics-mode': helpTopic(
    'Metrics Mode',
    'Chooses whether summary data is emitted as metrics only or as both events and metrics.',
    'One of: dual, metrics_only.',
    [],
    [
      'dual keeps the normal event stream and also emits metrics.',
      'metrics_only suppresses event summaries and emits metrics only.',
    ],
  ),
  'cfg-metrics-index': helpTopic(
    'Metrics Index',
    'Sets the target Splunk index for the metrics stream.',
    'Index name text.',
    ['Use a metrics-type index when Use Splunk Metrics Index Semantics is enabled.'],
  ),
  'cfg-metrics-hec-url': helpTopic(
    'Metrics HEC URL',
    'Sets the full HEC endpoint used for metrics delivery.',
    'Full URL ending in /services/collector.',
    ['Example: https://splunk.example.com:8088/services/collector'],
  ),
  'cfg-metrics-token': helpTopic(
    'Metrics Token',
    'Supplies the authorization token used for the metrics HEC stream.',
    'Plain token string issued by Splunk HEC.',
    ['The UI masks the token, but the runtime stores and uses the underlying value from the config file.'],
  ),
  'cfg-metrics-ssl-protocol': helpTopic(
    'Metrics SSL Protocol',
    'Pins the TLS protocol profile used for metrics HEC connections.',
    tlsProfileFormat,
    [],
    [
      'Default lets Go negotiate the best supported version.',
      'Accepted specific profiles include Tls10, Tls11, Tls12, and Tls13.',
    ],
  ),
  'cfg-metrics-sourcetype': helpTopic(
    'Metrics Sourcetype',
    'Sets the sourcetype associated with the metrics payload.',
    'Sourcetype text such as ping_monitor:metrics.',
  ),
  'cfg-metrics-event-name': helpTopic(
    'Event Name',
    'Sets the event field name used in the metrics payload when compatibility mode is active.',
    'Text value. The runtime uses metric when native metrics-index semantics are enabled.',
    ['If Use Splunk Metrics Index Semantics is checked, the runtime forces this to metric internally.'],
  ),
  'cfg-metrics-batch-size': helpTopic(
    'Metrics Batch Size',
    'Sets how many metric payloads are grouped before the buffer flushes.',
    positiveIntegerFormat,
  ),
  'cfg-metrics-max-buffer-events': helpTopic(
    'Metrics Max Buffer Events',
    'Caps how many metric payloads the in-memory metrics buffer can hold.',
    positiveIntegerFormat,
  ),
  'cfg-metrics-max-buffer-bytes': helpTopic(
    'Metrics Max Buffer Bytes',
    'Caps the metrics buffer by size instead of only by event count.',
    byteSizeFormat,
  ),
  'cfg-metrics-verify-ssl': helpTopic(
    'Verify TLS Certificates For The Metrics Sink',
    'Controls whether the runtime validates the remote certificate for metrics delivery.',
    checkboxFormat,
    ['Turn this off only for controlled troubleshooting or self-signed environments where you accept the risk.'],
  ),
  'cfg-metrics-compat-mode': helpTopic(
    'Preserve Legacy Metrics Payload Compatibility',
    'Keeps the older metrics payload shape so existing dashboards and searches continue to work during migration.',
    checkboxFormat,
    ['This setting is bypassed when Use Splunk Metrics Index Semantics is enabled.'],
  ),
  'cfg-metrics-use-metrics-index': helpTopic(
    'Use Splunk Metrics Index Semantics',
    'Switches the payload into native Splunk metrics-index behavior for mstats-friendly ingestion.',
    checkboxFormat,
    [
      'When enabled, the runtime forces event_name to metric and does not use the legacy compatibility payload shape.',
      'Use this only when the target index is a true Splunk metrics index.',
    ],
  ),
};

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function deepClone(value) {
  return JSON.parse(JSON.stringify(value));
}

function pluralize(count, singular, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function emptyEndpoint() {
  return {
    ip: '',
    hostname: '',
    group: 'default',
    description: '',
    entitytype: '',
    device: '',
    vendor: '',
    additional_notes: '',
    dev: false,
  };
}

function normalizeEndpoint(endpoint) {
  return {
    ip: String(endpoint.ip || '').trim(),
    hostname: String(endpoint.hostname || '').trim(),
    group: String(endpoint.group || 'default').trim() || 'default',
    description: String(endpoint.description || '').trim(),
    entitytype: String(endpoint.entitytype || '').trim(),
    device: String(endpoint.device || '').trim(),
    vendor: String(endpoint.vendor || '').trim(),
    additional_notes: String(endpoint.additional_notes || '').trim(),
    dev: Boolean(endpoint.dev),
  };
}

function normalizeEndpoints(endpoints) {
  return endpoints.map((endpoint) => normalizeEndpoint(endpoint));
}

function readNumberValue(element, fallback = 0) {
  const numeric = Number(element.value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function readTextValue(element) {
  return String(element.value || '').trim();
}

function isBlankText(value) {
  return String(value ?? '').trim() === '';
}

function setMessage(element, tone, message) {
  if (!element) {
    return;
  }
  if (!message) {
    element.className = 'message-banner hidden';
    element.textContent = '';
    return;
  }
  element.className = `message-banner ${tone}`;
  element.textContent = message;
}

function endpointsAreDirty() {
  return JSON.stringify(normalizeEndpoints(state.endpoints)) !== JSON.stringify(normalizeEndpoints(state.savedEndpoints));
}

function configIsDirty() {
  if (!state.savedConfig) {
    return false;
  }
  return JSON.stringify(readConfigForm()) !== JSON.stringify(state.savedConfig);
}

function helpTopic(title, summary, format = '', notes = [], values = []) {
  return { title, summary, format, notes, values };
}

function panelHelp(title, summary, notes = []) {
  return { title, summary, notes };
}

function getSettingsHelpTopic(helpKey) {
  if (!helpKey) {
    return null;
  }
  if (helpKey.startsWith('field:')) {
    return settingsFieldHelp[helpKey.slice(6)] || null;
  }
  if (helpKey.startsWith('panel:')) {
    return settingsPanelHelp[helpKey.slice(6)] || null;
  }
  return null;
}

function renderSettingsHelpSection(title, items) {
  if (!items || items.length === 0) {
    return '';
  }
  const listItems = items.map((item) => `<li>${escapeHtml(item)}</li>`).join('');
  return `
    <section class="settings-help-section">
      <h4>${escapeHtml(title)}</h4>
      <ul>${listItems}</ul>
    </section>
  `;
}

function renderSettingsHelpTopic(topic) {
  if (!topic) {
    return '';
  }
  const fragments = [`<p class="settings-help-summary">${escapeHtml(topic.summary)}</p>`];
  if (topic.format) {
    fragments.push(`
      <section class="settings-help-section">
        <h4>Expected input</h4>
        <p class="settings-help-copy">${escapeHtml(topic.format)}</p>
      </section>
    `);
  }
  fragments.push(renderSettingsHelpSection('Accepted values', topic.values));
  fragments.push(renderSettingsHelpSection('Operational notes', topic.notes));
  return fragments.join('');
}

function createSettingsHelpButton(helpKey, label, variant = 'inline') {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = `help-button help-button-${variant}`;
  button.dataset.helpKey = helpKey;
  button.setAttribute('aria-label', `Show help for ${label}`);
  button.textContent = 'i';
  return button;
}

function injectSettingsPanelHelpButtons() {
  if (!elements.settingsForm) {
    return;
  }
  elements.settingsForm.querySelectorAll('.settings-card').forEach((card) => {
    const titleElement = card.querySelector('.panel-title');
    const title = titleElement?.textContent?.trim();
    if (!titleElement || !title || !settingsPanelHelp[title]) {
      return;
    }
    let titleRow = titleElement.parentElement;
    if (!titleRow || !titleRow.classList.contains('panel-title-row')) {
      titleRow = document.createElement('div');
      titleRow.className = 'panel-title-row';
      titleElement.replaceWith(titleRow);
      titleRow.appendChild(titleElement);
    }
    if (titleRow.querySelector(`[data-help-key="panel:${title}"]`)) {
      return;
    }
    titleRow.appendChild(createSettingsHelpButton(`panel:${title}`, title, 'panel'));
  });
}

function injectSettingsFieldHelpButtons() {
  Object.values(elements.settingsFields).forEach((field) => {
    if (!(field instanceof HTMLElement)) {
      return;
    }
    const help = settingsFieldHelp[field.id];
    if (!help) {
      return;
    }
    const container = field.closest('.field-group, .checkbox-row');
    const textElement = container?.querySelector('span');
    if (!container || !textElement) {
      return;
    }
    const wrapperClass = container.classList.contains('checkbox-row') ? 'checkbox-help-row' : 'field-label-row';
    let textRow = textElement.parentElement;
    if (!textRow || !textRow.classList.contains(wrapperClass)) {
      textRow = document.createElement('div');
      textRow.className = wrapperClass;
      textElement.replaceWith(textRow);
      textRow.appendChild(textElement);
    }
    textElement.classList.add('help-label-text');
    if (textRow.querySelector(`[data-help-key="field:${field.id}"]`)) {
      return;
    }
    textRow.appendChild(createSettingsHelpButton(`field:${field.id}`, help.title, 'inline'));
  });
}

function openSettingsHelp(helpKey, trigger = null) {
  const topic = getSettingsHelpTopic(helpKey);
  if (!topic || !elements.settingsHelpDialog || !elements.settingsHelpTitle || !elements.settingsHelpBody) {
    return;
  }
  settingsHelpTrigger = trigger;
  elements.settingsHelpTitle.textContent = topic.title;
  elements.settingsHelpBody.innerHTML = renderSettingsHelpTopic(topic);
  if (typeof elements.settingsHelpDialog.showModal === 'function') {
    if (!elements.settingsHelpDialog.open) {
      elements.settingsHelpDialog.showModal();
    }
  } else {
    elements.settingsHelpDialog.setAttribute('open', 'open');
  }
  elements.settingsHelpCloseButton?.focus();
}

function closeSettingsHelp() {
  const dialog = elements.settingsHelpDialog;
  if (!dialog) {
    return;
  }
  if (typeof dialog.close === 'function') {
    if (dialog.open) {
      dialog.close();
      return;
    }
  } else {
    dialog.removeAttribute('open');
  }
  if (settingsHelpTrigger instanceof HTMLElement) {
    settingsHelpTrigger.focus();
  }
  settingsHelpTrigger = null;
}

function initializeSettingsHelp() {
  if (!elements.settingsForm || elements.settingsHelpDialog) {
    return;
  }

  const dialog = document.createElement('dialog');
  dialog.id = 'settings-help-dialog';
  dialog.className = 'settings-help-dialog';
  dialog.innerHTML = `
    <div class="settings-help-surface">
      <div class="settings-help-header">
        <div class="settings-help-heading">
          <p class="eyebrow">Settings Help</p>
          <h3 class="settings-help-title" data-help-title>Help</h3>
        </div>
        <button class="secondary-button settings-help-close" type="button" data-help-close>Close</button>
      </div>
      <div class="settings-help-body" data-help-body></div>
    </div>
  `;

  document.body.appendChild(dialog);
  elements.settingsHelpDialog = dialog;
  elements.settingsHelpTitle = dialog.querySelector('[data-help-title]');
  elements.settingsHelpBody = dialog.querySelector('[data-help-body]');
  elements.settingsHelpCloseButton = dialog.querySelector('[data-help-close]');

  elements.settingsHelpCloseButton?.addEventListener('click', closeSettingsHelp);
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) {
      closeSettingsHelp();
    }
  });
  dialog.addEventListener('cancel', (event) => {
    event.preventDefault();
    closeSettingsHelp();
  });
  dialog.addEventListener('close', () => {
    if (settingsHelpTrigger instanceof HTMLElement) {
      settingsHelpTrigger.focus();
    }
    settingsHelpTrigger = null;
  });

  elements.settingsForm.addEventListener('click', (event) => {
    const helpButton = event.target.closest('.help-button[data-help-key]');
    if (!helpButton) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    openSettingsHelp(helpButton.dataset.helpKey, helpButton);
  });

  injectSettingsPanelHelpButtons();
  injectSettingsFieldHelpButtons();
}

function sanitizeIndexSet(indexSet, maxLength) {
  const next = new Set();
  indexSet.forEach((value) => {
    if (Number.isInteger(value) && value >= 0 && value < maxLength) {
      next.add(value);
    }
  });
  return next;
}

function syncEndpointSelectionState() {
  state.selectedEndpointIndices = sanitizeIndexSet(state.selectedEndpointIndices, state.endpoints.length);
  if (state.selectedEndpointIndex >= state.endpoints.length) {
    state.selectedEndpointIndex = state.endpoints.length - 1;
  }
}

function syncDiscoverySelectionState() {
  state.discovery.selectedIndices = sanitizeIndexSet(state.discovery.selectedIndices, state.discovery.items.length);
}

function pruneEndpointSelections() {
  syncEndpointSelectionState();
}

function pruneDiscoverySelections() {
  syncDiscoverySelectionState();
}

function ensureSelectedEndpoint() {
  pruneEndpointSelections();
  if (state.endpoints.length === 0) {
    state.selectedEndpointIndex = -1;
    return;
  }
  if (state.selectedEndpointIndex < 0 || state.selectedEndpointIndex >= state.endpoints.length) {
    state.selectedEndpointIndex = 0;
  }
}

function filterEndpoints() {
  syncEndpointSelectionState();
  const search = state.search.trim().toLowerCase();
  return state.endpoints
    .map((endpoint, index) => ({ endpoint, index }))
    .filter(({ endpoint }) => {
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

function indexedDiscoveryItems() {
  syncDiscoverySelectionState();
  return state.discovery.items.map((endpoint, index) => ({ endpoint, index }));
}

function getTableState(tableKind) {
  return state.tables[tableKind];
}

function getSortValue(endpoint, sortKey) {
  if (sortKey === 'dev') {
    return endpoint.dev ? 1 : 0;
  }
  return String(endpoint[sortKey] || '').trim().toLowerCase();
}

function compareSortValues(left, right) {
  if (typeof left === 'number' && typeof right === 'number') {
    return left - right;
  }
  return String(left).localeCompare(String(right), undefined, {
    numeric: true,
    sensitivity: 'base',
  });
}

function getSortedRows(rows, tableKind) {
  const table = getTableState(tableKind);
  return [...rows].sort((left, right) => {
    const compared = compareSortValues(
      getSortValue(left.endpoint, table.sortKey),
      getSortValue(right.endpoint, table.sortKey),
    );
    if (compared !== 0) {
      return table.sortDir === 'asc' ? compared : -compared;
    }
    return left.index - right.index;
  });
}

function getPaginatedRows(rows, tableKind) {
  const table = getTableState(tableKind);
  const pageSize = Math.max(1, Number(table.pageSize) || 10);
  const totalItems = rows.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize) || 1);
  table.page = Math.min(Math.max(1, Number(table.page) || 1), totalPages);
  table.pageSize = pageSize;
  const startIndex = (table.page - 1) * pageSize;
  const pagedRows = rows.slice(startIndex, startIndex + pageSize);
  return {
    rows: pagedRows,
    page: table.page,
    totalPages,
    totalItems,
    startItem: totalItems === 0 ? 0 : startIndex + 1,
    endItem: totalItems === 0 ? 0 : Math.min(totalItems, startIndex + pagedRows.length),
  };
}

function formatPageStatus(view) {
  if (view.totalItems === 0) {
    return 'Page 1 of 1';
  }
  return `Page ${view.page} of ${view.totalPages} (${view.startItem}-${view.endItem} of ${view.totalItems})`;
}

function setTablePage(tableKind, page) {
  const table = getTableState(tableKind);
  table.page = Math.max(1, Number(page) || 1);
}

function changeTablePage(tableKind, delta) {
  const table = getTableState(tableKind);
  table.page = Math.max(1, (Number(table.page) || 1) + delta);
}

function setTablePageSize(tableKind, value) {
  const table = getTableState(tableKind);
  const nextValue = Number(value);
  table.pageSize = Number.isFinite(nextValue) && nextValue > 0 ? nextValue : 10;
  table.page = 1;
}

function updateTableSort(tableKind, sortKey) {
  const table = getTableState(tableKind);
  if (table.sortKey === sortKey) {
    table.sortDir = table.sortDir === 'asc' ? 'desc' : 'asc';
  } else {
    table.sortKey = sortKey;
    table.sortDir = sortKey === 'dev' ? 'desc' : 'asc';
  }
  table.page = 1;
}

function updateSortButtons() {
  elements.tableSortButtons.forEach((button) => {
    const tableKind = button.dataset.tableKind;
    const sortKey = button.dataset.sortKey;
    const table = getTableState(tableKind);
    const isActive = Boolean(table) && table.sortKey === sortKey;
    button.classList.toggle('is-active', isActive);
    button.dataset.sortState = isActive ? table.sortDir : 'none';
    button.setAttribute('aria-pressed', isActive ? 'true' : 'false');
  });
}

function renderEndpointFilterButtons() {
  elements.filterButtons.forEach((button) => {
    button.classList.toggle('active', (button.dataset.filter || 'all') === state.filter);
  });
}

function syncSelectedEndpointToVisibleRows(rows) {
  if (state.endpoints.length === 0 || rows.length === 0) {
    state.selectedEndpointIndex = -1;
    return;
  }
  const hasVisibleSelection = rows.some(({ index }) => index === state.selectedEndpointIndex);
  if (!hasVisibleSelection) {
    state.selectedEndpointIndex = rows[0].index;
  }
}

function getEndpointActionIndices() {
  if (state.selectedEndpointIndices.size > 0) {
    return Array.from(state.selectedEndpointIndices).sort((left, right) => left - right);
  }
  if (state.selectedEndpointIndex >= 0 && state.selectedEndpointIndex < state.endpoints.length) {
    return [state.selectedEndpointIndex];
  }
  return [];
}

function getDiscoveryActionIndices() {
  pruneDiscoverySelections();
  return Array.from(state.discovery.selectedIndices).sort((left, right) => left - right);
}

function toggleEndpointSelection(index, forceChecked = null) {
  if (forceChecked === null) {
    if (state.selectedEndpointIndices.has(index)) {
      state.selectedEndpointIndices.delete(index);
    } else {
      state.selectedEndpointIndices.add(index);
    }
    return;
  }
  if (forceChecked) {
    state.selectedEndpointIndices.add(index);
  } else {
    state.selectedEndpointIndices.delete(index);
  }
}

function toggleDiscoverySelection(index, forceChecked = null) {
  if (forceChecked === null) {
    if (state.discovery.selectedIndices.has(index)) {
      state.discovery.selectedIndices.delete(index);
    } else {
      state.discovery.selectedIndices.add(index);
    }
    return;
  }
  if (forceChecked) {
    state.discovery.selectedIndices.add(index);
  } else {
    state.discovery.selectedIndices.delete(index);
  }
}

function appendDiscoveryLogLine(line) {
  const trimmed = String(line || '').trim();
  if (!trimmed) {
    return;
  }
  state.discovery.logs = state.discovery.logs ? `${state.discovery.logs}\n${trimmed}` : trimmed;
}

function fetchJson(path, options = {}) {
  return fetch(path, {
    cache: 'no-store',
    ...options,
  }).then(async (response) => {
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || `Request failed for ${path}`);
    }
    return payload;
  });
}

function putJson(path, payload) {
  return fetchJson(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

function postJson(path, payload) {
  return fetchJson(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

function usesInnerScroll() {
  return Boolean(elements.contentScroll) && getComputedStyle(elements.contentScroll).overflowY !== 'visible';
}

function setActiveNav(hash, preferredLink = null) {
  const fallbackLink = elements.navLinks.find((link) => link.getAttribute('href') === '#overview') || null;
  const resolvedLink = preferredLink
    || elements.navLinks.find((link) => link.getAttribute('href') === hash)
    || fallbackLink;
  elements.navLinks.forEach((link) => {
    link.classList.toggle('active', link === resolvedLink);
  });
}

function scrollSectionIntoView(hash, behavior = 'smooth') {
  if (!hash) {
    return;
  }
  const target = document.querySelector(hash);
  if (!(target instanceof HTMLElement)) {
    setActiveNav(hash);
    return;
  }
  if (usesInnerScroll()) {
    const contentTop = elements.contentScroll.getBoundingClientRect().top;
    const targetTop = target.getBoundingClientRect().top;
    const nextTop = elements.contentScroll.scrollTop + (targetTop - contentTop) - 24;
    elements.contentScroll.scrollTo({ top: Math.max(0, nextTop), behavior });
  } else {
    target.scrollIntoView({ behavior, block: 'start' });
  }
  setActiveNav(hash);
}

function updateActiveNavFromScroll() {
  const threshold = 140;
  const activeHash = sectionHashes.reduce((current, hash) => {
    const target = document.querySelector(hash);
    if (!(target instanceof HTMLElement)) {
      return current;
    }
    const top = usesInnerScroll()
      ? target.getBoundingClientRect().top - elements.contentScroll.getBoundingClientRect().top
      : target.getBoundingClientRect().top;
    return top <= threshold ? hash : current;
  }, '#overview');
  setActiveNav(activeHash);
}

function loadEndpointForm(endpoint) {
  const current = endpoint || emptyEndpoint();
  elements.endpointFields.ip.value = current.ip || '';
  elements.endpointFields.hostname.value = current.hostname || '';
  elements.endpointFields.group.value = current.group || 'default';
  elements.endpointFields.description.value = current.description || '';
  elements.endpointFields.entitytype.value = current.entitytype || '';
  elements.endpointFields.device.value = current.device || '';
  elements.endpointFields.vendor.value = current.vendor || '';
  elements.endpointFields.additional_notes.value = current.additional_notes || '';
  elements.endpointFields.dev.checked = Boolean(current.dev);
}

function readEndpointForm() {
  return normalizeEndpoint({
    ip: readTextValue(elements.endpointFields.ip),
    hostname: readTextValue(elements.endpointFields.hostname),
    group: readTextValue(elements.endpointFields.group),
    description: readTextValue(elements.endpointFields.description),
    entitytype: readTextValue(elements.endpointFields.entitytype),
    device: readTextValue(elements.endpointFields.device),
    vendor: readTextValue(elements.endpointFields.vendor),
    additional_notes: readTextValue(elements.endpointFields.additional_notes),
    dev: elements.endpointFields.dev.checked,
  });
}

function updateCurrentEndpointFromForm() {
  if (state.selectedEndpointIndex < 0 || state.selectedEndpointIndex >= state.endpoints.length) {
    return;
  }
  state.endpoints[state.selectedEndpointIndex] = readEndpointForm();
  renderSummary();
  renderEndpointTable();
  renderDevTable();
  renderEndpointButtons();
}

function renderStatus() {
  if (!state.status) {
    return;
  }
  const formatLabel = String(state.status.config_format || '').toUpperCase();
  elements.versionText.textContent = `${state.status.version} deployment UI`;
  elements.modePill.textContent = state.status.mode === 'editable' ? 'Editable' : state.status.mode;
  elements.configPathChip.textContent = state.status.config_path;
  elements.endpointPath.textContent = state.status.endpoints_path;
  elements.sidebarConfigSource.textContent = `Config: ${formatLabel}`;
  elements.sidebarDiscoveryStatus.textContent = state.status.discovery_available ? 'Discovery ready' : 'Discovery unavailable';
  elements.configSourceLabel.textContent = `${formatLabel} settings file`;
  elements.configSourceCopy.textContent = state.status.config_path;
  elements.discoveryStatusLabel.textContent = state.status.discovery_available ? 'Discovery Ready' : 'Discovery Unavailable';
  elements.discoveryStatusCopy.textContent = state.status.discovery_available
    ? (state.status.discovery_script_path || 'Using companion discovery workflow.')
    : 'Discovery needs the companion workflow available in this deployment.';
  elements.discoveryAvailability.textContent = state.status.discovery_available ? 'Discovery Available' : 'Discovery Not Available';
  elements.settingsSourceChip.textContent = `${formatLabel} · ${state.status.config_path}`;
}

function renderSummary() {
  const groups = new Set(state.endpoints.map((endpoint) => (endpoint.group || 'default').trim() || 'default'));
  const devCount = state.endpoints.filter((endpoint) => endpoint.dev).length;
  elements.summaryTotal.textContent = String(state.endpoints.length);
  elements.summaryProduction.textContent = String(state.endpoints.length - devCount);
  elements.summaryDev.textContent = String(devCount);
  elements.summaryGroups.textContent = String(groups.size);
}

function renderEndpointTable() {
  ensureSelectedEndpoint();
  renderEndpointFilterButtons();
  const filtered = filterEndpoints();
  syncSelectedEndpointToVisibleRows(filtered);
  const sorted = getSortedRows(filtered, 'endpoint');
  const view = getPaginatedRows(sorted, 'endpoint');
  const selectedCount = state.selectedEndpointIndices.size;

  updateSortButtons();
  elements.endpointPageSize.value = String(state.tables.endpoint.pageSize);
  elements.endpointPageStatus.textContent = formatPageStatus(view);
  elements.endpointPrevPageButton.disabled = view.page <= 1;
  elements.endpointNextPageButton.disabled = view.page >= view.totalPages;
  elements.endpointSelectionStatus.textContent = `${selectedCount} selected from ${filtered.length} matching endpoint${filtered.length === 1 ? '' : 's'}`;

  if (view.totalItems === 0) {
    elements.endpointRows.innerHTML = '<tr><td colspan="8" class="empty-cell">No endpoints match the current filter.</td></tr>';
    return;
  }

  elements.endpointRows.innerHTML = view.rows.map(({ endpoint, index }) => {
    const rowClasses = [
      index === state.selectedEndpointIndex ? 'selected-row' : '',
      state.selectedEndpointIndices.has(index) ? 'checked-row' : '',
    ].filter(Boolean).join(' ');
    const label = endpoint.ip || endpoint.hostname || `endpoint ${index + 1}`;
    return `
      <tr class="${rowClasses}" data-index="${index}">
        <td class="table-select-col"><input class="table-row-checkbox" type="checkbox" data-index="${index}" ${state.selectedEndpointIndices.has(index) ? 'checked' : ''} aria-label="Select ${escapeHtml(label)}"></td>
        <td>${escapeHtml(endpoint.ip)}</td>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.entitytype || '-')}</td>
        <td>${escapeHtml(endpoint.device || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
        <td><span class="mode-badge ${endpoint.dev ? 'dev' : 'production'}">${endpoint.dev ? 'Dev' : 'Production'}</span></td>
      </tr>
    `;
  }).join('');
}

function renderEndpointButtons() {
  const dirty = endpointsAreDirty();
  const hasEditorSelection = state.selectedEndpointIndex >= 0;
  const actionIndices = getEndpointActionIndices();
  const selectedCount = state.selectedEndpointIndices.size;
  const filteredCount = filterEndpoints().length;

  elements.endpointDirtyPill.textContent = dirty ? 'Unsaved changes' : 'In sync';
  elements.endpointDirtyPill.classList.toggle('is-dirty', dirty);
  elements.saveEndpointsButton.disabled = !dirty;
  elements.resetEndpointsButton.disabled = !dirty;
  elements.selectAllEndpointsButton.disabled = filteredCount === 0;
  elements.deselectAllEndpointsButton.disabled = selectedCount === 0;
  elements.markSelectedDevButton.disabled = actionIndices.length === 0;
  elements.markSelectedProductionButton.disabled = actionIndices.length === 0;
  elements.deleteEndpointButton.disabled = actionIndices.length === 0;

  if (!hasEditorSelection) {
    elements.endpointSelectionLabel.textContent = state.endpoints.length === 0
      ? 'Add a new endpoint or load discovery results into the working draft.'
      : 'No endpoint matches the current filter. Clear the filters or pick another view to edit.';
    loadEndpointForm(emptyEndpoint());
    return;
  }

  if (selectedCount > 0) {
    elements.endpointSelectionLabel.textContent = `Editing endpoint ${state.selectedEndpointIndex + 1} of ${state.endpoints.length}. ${selectedCount} selected row${selectedCount === 1 ? '' : 's'} ready for bulk actions.`;
    return;
  }

  elements.endpointSelectionLabel.textContent = `Editing endpoint ${state.selectedEndpointIndex + 1} of ${state.endpoints.length}. Changes remain local until you save.`;
}

function renderEndpointEditor() {
  ensureSelectedEndpoint();
  const selected = state.selectedEndpointIndex >= 0 ? state.endpoints[state.selectedEndpointIndex] : null;
  loadEndpointForm(selected);
  renderEndpointButtons();
}

function renderDevTable() {
  const devEndpoints = state.endpoints.filter((endpoint) => endpoint.dev);
  if (devEndpoints.length === 0) {
    elements.devRows.innerHTML = '<tr><td colspan="4" class="empty-cell">No dev endpoints are currently defined in the working draft.</td></tr>';
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
  elements.devRefresh.textContent = `${devEndpoints.length} dev endpoint${devEndpoints.length === 1 ? '' : 's'} in draft`;
}

function buildDiscoverySummaryText() {
  if (!state.discovery.available) {
    return 'Discovery is unavailable in this deployment.';
  }
  if (state.discovery.running) {
    return state.discovery.progressSummary || 'Discovery is running.';
  }
  if (state.discovery.summary) {
    return `${state.discovery.summary.total} endpoints found in ${state.discovery.durationMs} ms. Production ${state.discovery.summary.production}, dev ${state.discovery.summary.dev}, groups ${state.discovery.summary.groups}.`;
  }
  return state.discovery.progressSummary || 'No discovery run yet.';
}

function renderDiscovery() {
  pruneDiscoverySelections();
  const hasResults = state.discovery.items.length > 0;
  const selectedCount = state.discovery.selectedIndices.size;
  const sorted = getSortedRows(indexedDiscoveryItems(), 'discovery');
  const view = getPaginatedRows(sorted, 'discovery');

  updateSortButtons();
  elements.discoveryMergeMode.value = state.discovery.mergeMode;
  elements.discoveryPageSize.value = String(state.tables.discovery.pageSize);
  elements.discoveryRunState.textContent = state.discovery.runState;
  elements.discoveryRunState.dataset.state = state.discovery.runState.toLowerCase().replace(/\s+/g, '-');
  elements.discoverySummary.textContent = buildDiscoverySummaryText();
  elements.discoveryLogs.textContent = state.discovery.logs || (state.discovery.running
    ? 'Discovery is running. Progress logs will appear here as they are emitted.'
    : 'Discovery logs will appear here after a run.');
  elements.discoverySelectionStatus.textContent = `${selectedCount} selected`;
  elements.discoveryTableStatus.textContent = hasResults
    ? `Showing ${view.startItem}-${view.endItem} of ${view.totalItems} result${view.totalItems === 1 ? '' : 's'}`
    : (state.discovery.running ? 'Discovery is running. Results will populate when the staged file is written.' : '0 results');
  elements.discoveryPageStatus.textContent = formatPageStatus(view);
  elements.discoveryPrevPageButton.disabled = view.page <= 1 || state.discovery.running;
  elements.discoveryNextPageButton.disabled = view.page >= view.totalPages || state.discovery.running;
  elements.runDiscoveryButton.disabled = !state.discovery.available || state.discovery.running;
  elements.runDiscoveryButton.textContent = state.discovery.running ? 'Running Discovery...' : 'Run Discovery';
  elements.selectAllDiscoveryButton.disabled = !hasResults || state.discovery.running;
  elements.deselectAllDiscoveryButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryDevButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryProductionButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.addDiscoverySelectedButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.discoveryMergeMode.disabled = !hasResults || state.discovery.running;

  if (view.totalItems === 0) {
    const emptyMessage = state.discovery.running
      ? 'Discovery is running. Results will appear here when the staged output file is ready.'
      : (state.discovery.available
        ? 'Discovery results will appear here after a run.'
        : 'Discovery is unavailable because the companion workflow is not present in this deployment.');
    elements.discoveryRows.innerHTML = `<tr><td colspan="8" class="empty-cell">${emptyMessage}</td></tr>`;
    return;
  }

  elements.discoveryRows.innerHTML = view.rows.map(({ endpoint, index }) => {
    const label = endpoint.ip || endpoint.hostname || `discovered endpoint ${index + 1}`;
    return `
      <tr class="${state.discovery.selectedIndices.has(index) ? 'checked-row' : ''}" data-index="${index}">
        <td class="table-select-col"><input class="table-row-checkbox" type="checkbox" data-index="${index}" ${state.discovery.selectedIndices.has(index) ? 'checked' : ''} aria-label="Select ${escapeHtml(label)}"></td>
        <td>${escapeHtml(endpoint.ip)}</td>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.entitytype || '-')}</td>
        <td>${escapeHtml(endpoint.device || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
        <td><span class="mode-badge ${endpoint.dev ? 'dev' : 'production'}">${endpoint.dev ? 'Dev' : 'Production'}</span></td>
      </tr>
    `;
  }).join('');
}

function loadConfigForm(cfg) {
  const ping = cfg.ping || {};
  const diagnostics = cfg.diagnostics || {};
  const debug = cfg.debug || {};
  const hec = cfg.hec || {};
  const retry = hec.retry || {};
  const metrics = cfg.metrics || {};

  elements.settingsFields.pingsPerCycle.value = cfg.pings_per_cycle ?? '';
  elements.settingsFields.cycleInterval.value = cfg.cycle_interval_seconds ?? '';
  elements.settingsFields.timeoutMs.value = cfg.timeout_ms ?? '';
  elements.settingsFields.parallelThreads.value = cfg.parallel_threads ?? '';
  elements.settingsFields.emitIndividualPings.checked = Boolean(cfg.emit_individual_pings);
  elements.settingsFields.outputMode.value = cfg.output_mode || 'file';
  elements.settingsFields.logPath.value = cfg.log_path || '';
  elements.settingsFields.logRotation.value = cfg.log_rotation_size_mb ?? '';
  elements.settingsFields.pingMode.value = ping.mode || 'auto';
  elements.settingsFields.diagnosticsEnabled.checked = Boolean(diagnostics.enabled);
  elements.settingsFields.handleProbeMode.value = diagnostics.handle_probe_mode || 'none';
  elements.settingsFields.emitMemoryStats.checked = Boolean(debug.emit_memory_stats);
  elements.settingsFields.hecEnabled.checked = Boolean(hec.enabled);
  elements.settingsFields.hecURL.value = hec.url || '';
  elements.settingsFields.hecToken.value = hec.token || '';
  elements.settingsFields.hecIndex.value = hec.index || '';
  elements.settingsFields.hecSourcetype.value = hec.sourcetype || '';
  elements.settingsFields.hecVerifySSL.checked = Boolean(hec.verify_ssl);
  elements.settingsFields.hecSSLProtocol.value = hec.ssl_protocol || '';
  elements.settingsFields.hecBatchSize.value = hec.batch_size ?? '';
  elements.settingsFields.hecDropOnFailure.checked = Boolean(hec.drop_on_failure);
  elements.settingsFields.hecMaxBufferEvents.value = hec.max_buffer_events ?? '';
  elements.settingsFields.hecMaxBufferBytes.value = hec.max_buffer_bytes || '';
  elements.settingsFields.hecRetryEnabled.checked = Boolean(retry.enabled);
  elements.settingsFields.hecMaxAttempts.value = retry.max_attempts ?? '';
  elements.settingsFields.hecBaseDelayMs.value = retry.base_delay_ms ?? '';
  elements.settingsFields.hecJitterPct.value = retry.jitter_pct ?? '';
  elements.settingsFields.hecBackoff.value = retry.backoff || '';
  elements.settingsFields.hecRetryCount.value = hec.retry_count ?? '';
  elements.settingsFields.hecRetryDelayMs.value = hec.retry_delay_ms ?? '';
  elements.settingsFields.hecDeadLetterPath.value = hec.dead_letter_path || '';
  elements.settingsFields.hecDeadLetterRotation.value = hec.dead_letter_rotation_size_mb ?? '';
  elements.settingsFields.metricsEnabled.checked = Boolean(metrics.enabled);
  elements.settingsFields.metricsMode.value = metrics.mode || 'dual';
  elements.settingsFields.metricsIndex.value = metrics.index || '';
  elements.settingsFields.metricsHECURL.value = metrics.hec_url || '';
  elements.settingsFields.metricsToken.value = metrics.token || '';
  elements.settingsFields.metricsVerifySSL.checked = Boolean(metrics.verify_ssl);
  elements.settingsFields.metricsSSLProtocol.value = metrics.ssl_protocol || '';
  elements.settingsFields.metricsCompatMode.checked = Boolean(metrics.compat_mode);
  elements.settingsFields.metricsSourcetype.value = metrics.sourcetype || '';
  elements.settingsFields.metricsEventName.value = metrics.event_name || '';
  elements.settingsFields.metricsUseMetricsIndex.checked = Boolean(metrics.use_metrics_index);
  elements.settingsFields.metricsBatchSize.value = metrics.batch_size ?? '';
  elements.settingsFields.metricsMaxBufferEvents.value = metrics.max_buffer_events ?? '';
  elements.settingsFields.metricsMaxBufferBytes.value = metrics.max_buffer_bytes || '';
}

function readConfigForm() {
  return {
    pings_per_cycle: readNumberValue(elements.settingsFields.pingsPerCycle, 4),
    cycle_interval_seconds: readNumberValue(elements.settingsFields.cycleInterval, 60),
    timeout_ms: readNumberValue(elements.settingsFields.timeoutMs, 1000),
    parallel_threads: readNumberValue(elements.settingsFields.parallelThreads, 10),
    output_mode: readTextValue(elements.settingsFields.outputMode) || 'file',
    log_path: readTextValue(elements.settingsFields.logPath),
    log_rotation_size_mb: readNumberValue(elements.settingsFields.logRotation, 50),
    emit_individual_pings: elements.settingsFields.emitIndividualPings.checked,
    ping: {
      mode: readTextValue(elements.settingsFields.pingMode) || 'auto',
    },
    diagnostics: {
      enabled: elements.settingsFields.diagnosticsEnabled.checked,
      handle_probe_mode: readTextValue(elements.settingsFields.handleProbeMode) || 'none',
    },
    debug: {
      emit_memory_stats: elements.settingsFields.emitMemoryStats.checked,
    },
    hec: {
      enabled: elements.settingsFields.hecEnabled.checked,
      url: readTextValue(elements.settingsFields.hecURL),
      token: readTextValue(elements.settingsFields.hecToken),
      index: readTextValue(elements.settingsFields.hecIndex),
      sourcetype: readTextValue(elements.settingsFields.hecSourcetype),
      verify_ssl: elements.settingsFields.hecVerifySSL.checked,
      ssl_protocol: readTextValue(elements.settingsFields.hecSSLProtocol) || 'Default',
      batch_size: readNumberValue(elements.settingsFields.hecBatchSize, 100),
      drop_on_failure: elements.settingsFields.hecDropOnFailure.checked,
      max_buffer_events: readNumberValue(elements.settingsFields.hecMaxBufferEvents, 5000),
      max_buffer_bytes: readTextValue(elements.settingsFields.hecMaxBufferBytes) || '5MB',
      retry: {
        enabled: elements.settingsFields.hecRetryEnabled.checked,
        max_attempts: readNumberValue(elements.settingsFields.hecMaxAttempts, 3),
        base_delay_ms: readNumberValue(elements.settingsFields.hecBaseDelayMs, 250),
        jitter_pct: readNumberValue(elements.settingsFields.hecJitterPct, 20),
        backoff: readTextValue(elements.settingsFields.hecBackoff) || 'exponential',
      },
      retry_count: readNumberValue(elements.settingsFields.hecRetryCount, 0),
      retry_delay_ms: readNumberValue(elements.settingsFields.hecRetryDelayMs, 250),
      dead_letter_path: readTextValue(elements.settingsFields.hecDeadLetterPath),
      dead_letter_rotation_size_mb: readNumberValue(elements.settingsFields.hecDeadLetterRotation, 0),
    },
    metrics: {
      enabled: elements.settingsFields.metricsEnabled.checked,
      mode: readTextValue(elements.settingsFields.metricsMode) || 'dual',
      index: readTextValue(elements.settingsFields.metricsIndex),
      hec_url: readTextValue(elements.settingsFields.metricsHECURL),
      token: readTextValue(elements.settingsFields.metricsToken),
      verify_ssl: elements.settingsFields.metricsVerifySSL.checked,
      ssl_protocol: readTextValue(elements.settingsFields.metricsSSLProtocol) || 'Default',
      compat_mode: elements.settingsFields.metricsCompatMode.checked,
      sourcetype: readTextValue(elements.settingsFields.metricsSourcetype),
      event_name: readTextValue(elements.settingsFields.metricsEventName),
      use_metrics_index: elements.settingsFields.metricsUseMetricsIndex.checked,
      batch_size: readNumberValue(elements.settingsFields.metricsBatchSize, 100),
      max_buffer_events: readNumberValue(elements.settingsFields.metricsMaxBufferEvents, 5000),
      max_buffer_bytes: readTextValue(elements.settingsFields.metricsMaxBufferBytes) || '5MB',
    },
  };
}

function renderConfigButtons() {
  const dirty = configIsDirty();
  elements.saveConfigButton.disabled = !dirty;
  elements.resetConfigButton.disabled = !dirty;
}

function renderOutputTestDetails(result) {
  if (!result) {
    elements.outputTestDetails.textContent = '';
    elements.outputTestDetails.classList.add('hidden');
    return;
  }

  const warnings = (result.warnings || []).map((warning) => `- ${warning}`).join('\n');
  const details = [
    `Target: ${result.target}`,
    `URL: ${result.url}`,
    `HTTP Status: ${result.status_code || 'n/a'}`,
    `Duration (ms): ${result.duration_ms}`,
    `Success: ${result.success ? 'true' : 'false'}`,
  ];
  if (warnings) {
    details.push('', 'Warnings:', warnings);
  }
  if (result.response_body) {
    details.push('', 'Response:', result.response_body);
  }
  elements.outputTestDetails.textContent = details.join('\n');
  elements.outputTestDetails.classList.remove('hidden');
}

async function reloadAllData(showSuccess = false) {
  elements.refreshButton.disabled = true;
  elements.refreshButton.textContent = 'Reloading...';
  try {
    const [status, endpointsPayload, configPayload] = await Promise.all([
      fetchJson('/api/status'),
      fetchJson('/api/endpoints'),
      fetchJson('/api/config'),
    ]);

    state.status = status;
    state.endpoints = deepClone(endpointsPayload.items || []);
    state.savedEndpoints = deepClone(endpointsPayload.items || []);
    state.selectedEndpointIndices.clear();
    loadConfigForm(configPayload.config || {});
    state.savedConfig = readConfigForm();
    state.discovery.available = Boolean(status.discovery_available);
    if (!state.discovery.running && state.discovery.items.length === 0 && !state.discovery.summary) {
      state.discovery.runState = 'Idle';
      state.discovery.progressSummary = state.discovery.available ? 'No discovery run yet.' : 'Discovery is unavailable in this deployment.';
    }
    ensureSelectedEndpoint();
    renderAll();

    if (showSuccess) {
      setMessage(elements.endpointBanner, 'success', 'Reloaded endpoints and config from disk.');
      setMessage(elements.settingsBanner, 'success', 'Config form refreshed from disk.');
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unable to load deployment data.';
    setMessage(elements.endpointBanner, 'error', message);
    setMessage(elements.settingsBanner, 'error', message);
    setMessage(elements.discoveryBanner, 'error', message);
  } finally {
    elements.refreshButton.disabled = false;
    elements.refreshButton.textContent = 'Reload From Disk';
    requestAnimationFrame(() => {
      scrollSectionIntoView(location.hash || '#overview', 'auto');
      updateActiveNavFromScroll();
    });
  }
}

function renderAll() {
  renderStatus();
  renderSummary();
  renderEndpointTable();
  renderEndpointEditor();
  renderDevTable();
  renderDiscovery();
  renderConfigButtons();
}

async function saveEndpoints() {
  if (state.selectedEndpointIndex >= 0) {
    state.endpoints[state.selectedEndpointIndex] = readEndpointForm();
  }
  try {
    const payload = await putJson('/api/endpoints', { items: normalizeEndpoints(state.endpoints) });
    state.endpoints = deepClone(payload.items || []);
    state.savedEndpoints = deepClone(payload.items || []);
    ensureSelectedEndpoint();
    renderAll();
    setMessage(elements.endpointBanner, 'success', `Saved ${state.endpoints.length} endpoints to ${payload.endpoints_path}.`);
  } catch (error) {
    setMessage(elements.endpointBanner, 'error', error instanceof Error ? error.message : 'Unable to save endpoints.');
  }
}

function resetEndpointsDraft() {
  state.endpoints = deepClone(state.savedEndpoints);
  state.selectedEndpointIndices.clear();
  ensureSelectedEndpoint();
  renderAll();
  setMessage(elements.endpointBanner, 'success', 'Reverted endpoint draft to the last saved file state.');
}

function addEndpoint() {
  state.endpoints.push(emptyEndpoint());
  state.selectedEndpointIndex = state.endpoints.length - 1;
  state.selectedEndpointIndices.clear();
  state.tables.endpoint.page = Math.max(1, Math.ceil(state.endpoints.length / state.tables.endpoint.pageSize));
  renderAll();
  setMessage(elements.endpointBanner, 'success', 'Added a new endpoint to the working draft.');
}

function deleteSelectedEndpoint() {
  const indexes = getEndpointActionIndices();
  if (indexes.length === 0) {
    return;
  }
  const removals = new Set(indexes);
  state.endpoints = state.endpoints.filter((_, index) => !removals.has(index));
  state.selectedEndpointIndices.clear();
  ensureSelectedEndpoint();
  renderAll();
  setMessage(elements.endpointBanner, 'warning', `Removed ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} from the working draft. Save endpoints to persist the deletion.`);
}

function selectAllVisibleEndpoints() {
  filterEndpoints().forEach(({ index }) => state.selectedEndpointIndices.add(index));
  renderEndpointTable();
  renderEndpointButtons();
}

function deselectAllEndpoints() {
  state.selectedEndpointIndices.clear();
  renderEndpointTable();
  renderEndpointButtons();
}

function setEndpointModeForSelection(isDev) {
  const indexes = getEndpointActionIndices();
  if (indexes.length === 0) {
    return;
  }
  indexes.forEach((index) => {
    if (state.endpoints[index]) {
      state.endpoints[index].dev = isDev;
    }
  });
  if (indexes.includes(state.selectedEndpointIndex)) {
    loadEndpointForm(state.endpoints[state.selectedEndpointIndex]);
  }
  renderAll();
  setMessage(elements.endpointBanner, 'success', `Marked ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} as ${isDev ? 'dev' : 'production'} in the working draft.`);
}

async function saveConfig() {
  try {
    const payload = await putJson('/api/config', { config: readConfigForm() });
    loadConfigForm(payload.config || {});
    state.savedConfig = readConfigForm();
    renderStatus();
    renderConfigButtons();
    setMessage(elements.settingsBanner, 'success', `Saved ${payload.config_format.toUpperCase()} config to ${payload.config_path}.`);
  } catch (error) {
    setMessage(elements.settingsBanner, 'error', error instanceof Error ? error.message : 'Unable to save config.');
  }
}

async function testOutput(target) {
  const button = target === 'hec' ? elements.testHECButton : elements.testMetricsButton;
  const idleLabel = target === 'hec' ? 'Test Event HEC' : 'Test Metrics HEC';
  const activeLabel = target === 'hec' ? 'Testing Event HEC...' : 'Testing Metrics HEC...';
  button.disabled = true;
  button.textContent = activeLabel;
  try {
    const payload = await postJson('/api/output/test', {
      target,
      config: readConfigForm(),
    });
    renderOutputTestDetails(payload);
    setMessage(elements.settingsBanner, payload.success ? 'success' : 'warning', payload.message);
  } catch (error) {
    renderOutputTestDetails(null);
    setMessage(elements.settingsBanner, 'error', error instanceof Error ? error.message : 'Unable to test output settings.');
  } finally {
    button.disabled = false;
    button.textContent = idleLabel;
  }
}

async function reloadConfig() {
  try {
    const payload = await fetchJson('/api/config');
    loadConfigForm(payload.config || {});
    state.savedConfig = readConfigForm();
    renderConfigButtons();
    setMessage(elements.settingsBanner, 'success', `Reloaded ${payload.config_format.toUpperCase()} config from disk.`);
  } catch (error) {
    setMessage(elements.settingsBanner, 'error', error instanceof Error ? error.message : 'Unable to reload config.');
  }
}

function resetConfigChanges() {
  if (!state.savedConfig) {
    return;
  }
  loadConfigForm(state.savedConfig);
  renderConfigButtons();
  setMessage(elements.settingsBanner, 'success', 'Reverted settings form to the last saved file state.');
}

function selectAllVisibleDiscovery() {
  indexedDiscoveryItems().forEach(({ index }) => state.discovery.selectedIndices.add(index));
  renderDiscovery();
}

function deselectAllDiscovery() {
  state.discovery.selectedIndices.clear();
  renderDiscovery();
}

function setDiscoveryModeForSelection(isDev) {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0) {
    return;
  }
  indexes.forEach((index) => {
    if (state.discovery.items[index]) {
      state.discovery.items[index].dev = isDev;
    }
  });
  renderDiscovery();
}

function mergeEndpointRecords(existingEndpoint, incomingEndpoint, mode) {
  const merged = deepClone(existingEndpoint);
  ['hostname', 'group', 'description', 'entitytype', 'device', 'vendor', 'additional_notes'].forEach((key) => {
    const incomingValue = String(incomingEndpoint[key] || '').trim();
    if (mode === 'overwrite') {
      if (!isBlankText(incomingValue)) {
        merged[key] = incomingValue;
      }
      return;
    }
    if (mode === 'fill_blanks' && isBlankText(merged[key]) && !isBlankText(incomingValue)) {
      merged[key] = incomingValue;
    }
  });
  if (mode === 'overwrite') {
    merged.dev = Boolean(incomingEndpoint.dev);
  } else if (mode === 'fill_blanks' && incomingEndpoint.dev) {
    merged.dev = true;
  }
  return normalizeEndpoint(merged);
}

function addSelectedDiscoveryToEndpoints() {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0) {
    return;
  }
  const knownByIP = new Map(state.endpoints.map((endpoint, index) => [String(endpoint.ip || '').trim().toLowerCase(), index]));
  let addedCount = 0;
  let updatedCount = 0;
  let skippedCount = 0;

  indexes.forEach((index) => {
    const candidate = normalizeEndpoint(state.discovery.items[index]);
    const key = String(candidate.ip || '').trim().toLowerCase();
    if (!key) {
      skippedCount += 1;
      return;
    }
    if (!knownByIP.has(key)) {
      state.endpoints.push(candidate);
      knownByIP.set(key, state.endpoints.length - 1);
      addedCount += 1;
      return;
    }
    if (state.discovery.mergeMode === 'skip_existing') {
      skippedCount += 1;
      return;
    }
    const existingIndex = knownByIP.get(key);
    state.endpoints[existingIndex] = mergeEndpointRecords(state.endpoints[existingIndex], candidate, state.discovery.mergeMode);
    updatedCount += 1;
  });

  state.discovery.selectedIndices.clear();
  ensureSelectedEndpoint();
  renderAll();
  setMessage(
    elements.endpointBanner,
    'success',
    `Applied ${indexes.length} selected discovery result${indexes.length === 1 ? '' : 's'} to the working endpoint draft. Added ${addedCount}, updated ${updatedCount}, skipped ${skippedCount}.`,
  );
  history.replaceState(null, '', '#inventory');
  scrollSectionIntoView('#inventory');
}

function handleDiscoveryStreamEvent(event) {
  if (event.summary_text) {
    state.discovery.progressSummary = event.summary_text;
  }
  if (event.log_line) {
    appendDiscoveryLogLine(event.log_line);
  }

  switch (event.type) {
    case 'started':
      state.discovery.runState = 'Preparing';
      break;
    case 'progress':
      state.discovery.runState = 'Running';
      break;
    case 'complete':
      state.discovery.runState = 'Complete';
      state.discovery.running = false;
      state.discovery.items = deepClone(event.items || []);
      state.discovery.summary = event.summary || null;
      state.discovery.logs = event.logs || state.discovery.logs;
      state.discovery.durationMs = event.duration_ms || 0;
      state.discovery.selectedIndices.clear();
      setMessage(elements.discoveryBanner, 'success', `Discovery completed with ${state.discovery.items.length} endpoint${state.discovery.items.length === 1 ? '' : 's'}. Review and merge the results when ready.`);
      break;
    case 'error':
      state.discovery.runState = 'Error';
      state.discovery.running = false;
      if (event.logs) {
        state.discovery.logs = event.logs;
      }
      renderDiscovery();
      throw new Error(event.error || 'Discovery failed.');
    default:
      break;
  }

  renderDiscovery();
}

async function consumeDiscoveryStream(response) {
  if (!response.body) {
    throw new Error('Discovery stream did not return a readable body.');
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) {
        continue;
      }
      handleDiscoveryStreamEvent(JSON.parse(trimmed));
    }
    if (done) {
      if (buffer.trim()) {
        handleDiscoveryStreamEvent(JSON.parse(buffer.trim()));
      }
      break;
    }
  }
}

async function runDiscovery() {
  if (!state.discovery.available) {
    setMessage(elements.discoveryBanner, 'warning', 'Discovery is unavailable in this deployment because the companion workflow was not found.');
    return;
  }
  state.discovery.running = true;
  state.discovery.runState = 'Starting';
  state.discovery.progressSummary = 'Starting discovery run.';
  state.discovery.items = [];
  state.discovery.summary = null;
  state.discovery.logs = '';
  state.discovery.durationMs = 0;
  state.discovery.selectedIndices.clear();
  setTablePage('discovery', 1);
  renderDiscovery();
  setMessage(elements.discoveryBanner, null, '');
  try {
    const response = await fetch('/api/discovery/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      cache: 'no-store',
      body: JSON.stringify({
        target_network: elements.discoveryInputs.targetNetwork.value.trim(),
        subnet_mask: readNumberValue(elements.discoveryInputs.subnetMask, 24),
        timeout_ms: readNumberValue(elements.discoveryInputs.timeoutMs, 500),
        throttle_limit: readNumberValue(elements.discoveryInputs.throttleLimit, 50),
      }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      throw new Error(payload.error || 'Unable to run discovery.');
    }
    await consumeDiscoveryStream(response);
  } catch (error) {
    state.discovery.running = false;
    if (state.discovery.runState !== 'Error') {
      state.discovery.runState = 'Error';
    }
    renderDiscovery();
    setMessage(elements.discoveryBanner, 'error', error instanceof Error ? error.message : 'Unable to run discovery.');
  }
}

elements.searchInput.addEventListener('input', (event) => {
  state.search = event.target.value;
  setTablePage('endpoint', 1);
  renderEndpointTable();
  renderEndpointEditor();
});

elements.refreshButton.addEventListener('click', () => {
  reloadAllData(true);
});

elements.navLinks.forEach((link) => {
  link.addEventListener('click', (event) => {
    const hash = link.getAttribute('href');
    if (!hash) {
      return;
    }
    event.preventDefault();
    history.replaceState(null, '', hash);
    setActiveNav(hash, link);
    scrollSectionIntoView(hash);
  });
});

elements.filterButtons.forEach((button) => {
  button.addEventListener('click', () => {
    state.filter = button.dataset.filter || 'all';
    setTablePage('endpoint', 1);
    renderEndpointTable();
    renderEndpointEditor();
  });
});

elements.endpointRows.addEventListener('change', (event) => {
  const checkbox = event.target.closest('.table-row-checkbox');
  if (!checkbox) {
    return;
  }
  toggleEndpointSelection(Number(checkbox.dataset.index), checkbox.checked);
  renderEndpointTable();
  renderEndpointButtons();
});

elements.endpointRows.addEventListener('click', (event) => {
  if (event.target.closest('.table-row-checkbox')) {
    return;
  }
  const row = event.target.closest('[data-index]');
  if (!row) {
    return;
  }
  state.selectedEndpointIndex = Number(row.dataset.index);
  renderEndpointEditor();
  renderEndpointTable();
});

elements.endpointForm.addEventListener('input', updateCurrentEndpointFromForm);
elements.endpointForm.addEventListener('change', updateCurrentEndpointFromForm);

elements.addEndpointButton.addEventListener('click', addEndpoint);
elements.selectAllEndpointsButton.addEventListener('click', selectAllVisibleEndpoints);
elements.deselectAllEndpointsButton.addEventListener('click', deselectAllEndpoints);
elements.markSelectedDevButton.addEventListener('click', () => setEndpointModeForSelection(true));
elements.markSelectedProductionButton.addEventListener('click', () => setEndpointModeForSelection(false));
elements.deleteEndpointButton.addEventListener('click', deleteSelectedEndpoint);
elements.endpointPageSize.addEventListener('change', (event) => {
  setTablePageSize('endpoint', event.target.value);
  renderEndpointTable();
});
elements.endpointPrevPageButton.addEventListener('click', () => {
  changeTablePage('endpoint', -1);
  renderEndpointTable();
});
elements.endpointNextPageButton.addEventListener('click', () => {
  changeTablePage('endpoint', 1);
  renderEndpointTable();
});
elements.resetEndpointsButton.addEventListener('click', resetEndpointsDraft);
elements.saveEndpointsButton.addEventListener('click', saveEndpoints);

elements.runDiscoveryButton.addEventListener('click', runDiscovery);
elements.selectAllDiscoveryButton.addEventListener('click', selectAllVisibleDiscovery);
elements.deselectAllDiscoveryButton.addEventListener('click', deselectAllDiscovery);
elements.markDiscoveryDevButton.addEventListener('click', () => setDiscoveryModeForSelection(true));
elements.markDiscoveryProductionButton.addEventListener('click', () => setDiscoveryModeForSelection(false));
elements.addDiscoverySelectedButton.addEventListener('click', addSelectedDiscoveryToEndpoints);
elements.discoveryMergeMode.addEventListener('change', (event) => {
  state.discovery.mergeMode = event.target.value || 'skip_existing';
  renderDiscovery();
});
elements.discoveryPageSize.addEventListener('change', (event) => {
  setTablePageSize('discovery', event.target.value);
  renderDiscovery();
});
elements.discoveryPrevPageButton.addEventListener('click', () => {
  changeTablePage('discovery', -1);
  renderDiscovery();
});
elements.discoveryNextPageButton.addEventListener('click', () => {
  changeTablePage('discovery', 1);
  renderDiscovery();
});
elements.discoveryRows.addEventListener('change', (event) => {
  const checkbox = event.target.closest('.table-row-checkbox');
  if (!checkbox) {
    return;
  }
  toggleDiscoverySelection(Number(checkbox.dataset.index), checkbox.checked);
  renderDiscovery();
});

elements.tableSortButtons.forEach((button) => {
  button.addEventListener('click', () => {
    const tableKind = button.dataset.tableKind;
    const sortKey = button.dataset.sortKey;
    updateTableSort(tableKind, sortKey);
    if (tableKind === 'endpoint') {
      renderEndpointTable();
      return;
    }
    renderDiscovery();
  });
});

initializeSettingsHelp();

elements.settingsForm.addEventListener('input', renderConfigButtons);
elements.settingsForm.addEventListener('change', renderConfigButtons);
elements.testHECButton.addEventListener('click', () => testOutput('hec'));
elements.testMetricsButton.addEventListener('click', () => testOutput('metrics'));
elements.reloadConfigButton.addEventListener('click', reloadConfig);
elements.resetConfigButton.addEventListener('click', resetConfigChanges);
elements.saveConfigButton.addEventListener('click', saveConfig);

elements.contentScroll?.addEventListener('scroll', updateActiveNavFromScroll, { passive: true });
window.addEventListener('scroll', () => {
  if (!usesInnerScroll()) {
    updateActiveNavFromScroll();
  }
}, { passive: true });
window.addEventListener('resize', updateActiveNavFromScroll);
window.addEventListener('hashchange', () => {
  scrollSectionIntoView(location.hash || '#overview', 'auto');
});

reloadAllData();