const state = {
  status: null,
  endpoints: [],
  savedEndpoints: [],
  endpointsRevision: '',
  endpointsDirty: false,
  filter: 'all',
  search: '',
  selectedEndpointIndex: -1,
  selectedEndpointIndices: new Set(),
  savedConfig: null,
  configRevision: '',
  configDirty: false,
  config: null,
  configSecrets: {},
  classificationRules: [],
  discoverySubnets: [],
  runtimeRefreshPending: false,
  runtimeRestartBusy: false,
  advisor: null,
  advisorProfiles: [],
  advisorBenchmark: null,
  advisorBusy: false,
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
    delta: null,
    logs: '',
    durationMs: 0,
    generatedAt: '',
    selectedIndices: new Set(),
    mergeMode: 'skip_existing',
    reviewFilter: 'needs_review',
    reviewBusy: false,
    abortController: null,
    history: {
      loading: false,
      scans: [],
      schedules: [],
      totalScans: 0,
      retentionScans: 0,
      retentionDays: 0,
      historyPath: '',
    },
  },
};

let settingsHelpTrigger = null;

const elements = {
  sidebarConfigSource: document.getElementById('sidebar-config-source'),
  sidebarDiscoveryStatus: document.getElementById('sidebar-discovery-status'),
  versionText: document.getElementById('version-text'),
  modePill: document.getElementById('mode-pill'),
  runtimeBanner: document.getElementById('runtime-banner'),
  runtimeState: document.getElementById('runtime-state'),
  runtimeUptime: document.getElementById('runtime-uptime'),
  runtimeCycle: document.getElementById('runtime-cycle'),
  runtimeCycleNote: document.getElementById('runtime-cycle-note'),
  runtimeDelivery: document.getElementById('runtime-delivery'),
  runtimeDeliveryNote: document.getElementById('runtime-delivery-note'),
  runtimePending: document.getElementById('runtime-pending'),
  runtimePendingNote: document.getElementById('runtime-pending-note'),
  endpointReloadLabel: document.getElementById('endpoint-reload-label'),
  endpointReloadCopy: document.getElementById('endpoint-reload-copy'),
  configPathChip: document.getElementById('config-path-chip'),
  endpointPath: document.getElementById('endpoint-path'),
  configSourceLabel: document.getElementById('config-source-label'),
  configSourceCopy: document.getElementById('config-source-copy'),
  discoveryStatusLabel: document.getElementById('discovery-status-label'),
  discoveryStatusCopy: document.getElementById('discovery-status-copy'),
  contentScroll: document.querySelector('.content-scroll'),
  refreshButton: document.getElementById('refresh-button'),
  restartCollectorButton: document.getElementById('restart-collector-button'),
  summaryTotal: document.getElementById('summary-total'),
  summaryProduction: document.getElementById('summary-production'),
  summaryDev: document.getElementById('summary-dev'),
  summaryGroups: document.getElementById('summary-groups'),
  advisorBanner: document.getElementById('advisor-banner'),
  advisorProfile: document.getElementById('advisor-profile'),
  advisorAnalyzeButton: document.getElementById('advisor-analyze-button'),
  advisorBenchmarkButton: document.getElementById('advisor-benchmark-button'),
  advisorApplySafeButton: document.getElementById('advisor-apply-safe-button'),
  advisorApplyProfileButton: document.getElementById('advisor-apply-profile-button'),
  advisorBlockers: document.getElementById('advisor-blockers'),
  advisorWarnings: document.getElementById('advisor-warnings'),
  advisorSafeFixes: document.getElementById('advisor-safe-fixes'),
  advisorReadiness: document.getElementById('advisor-readiness'),
  advisorReadinessNote: document.getElementById('advisor-readiness-note'),
  advisorInventoryChip: document.getElementById('advisor-inventory-chip'),
  advisorCurrentSchedule: document.getElementById('advisor-current-schedule'),
  advisorProposedSchedule: document.getElementById('advisor-proposed-schedule'),
  advisorProposalTitle: document.getElementById('advisor-proposal-title'),
  advisorFindings: document.getElementById('advisor-findings'),
  advisorChanges: document.getElementById('advisor-changes'),
  advisorBenchmarkPanel: document.getElementById('advisor-benchmark-panel'),
  advisorBenchmarkResults: document.getElementById('advisor-benchmark-results'),
  endpointBanner: document.getElementById('endpoint-banner'),
  endpointDirtyPill: document.getElementById('endpoint-dirty-pill'),
  endpointSelectionStatus: document.getElementById('endpoint-selection-status'),
  endpointRows: document.getElementById('endpoint-rows'),
  endpointForm: document.getElementById('endpoint-form'),
  endpointValidation: document.getElementById('endpoint-validation'),
  endpointSelectionLabel: document.getElementById('endpoint-selection-label'),
  addEndpointButton: document.getElementById('add-endpoint-button'),
  selectAllEndpointsButton: document.getElementById('select-all-endpoints-button'),
  deselectAllEndpointsButton: document.getElementById('deselect-all-endpoints-button'),
  markSelectedMaintenanceButton: document.getElementById('mark-selected-maintenance-button'),
  markSelectedProductionButton: document.getElementById('mark-selected-production-button'),
  disableSelectedAlertingButton: document.getElementById('disable-selected-alerting-button'),
  enableSelectedAlertingButton: document.getElementById('enable-selected-alerting-button'),
  pauseSelectedButton: document.getElementById('pause-selected-button'),
  resumeSelectedButton: document.getElementById('resume-selected-button'),
  deleteEndpointButton: document.getElementById('delete-endpoint-button'),
  deleteCurrentEndpointButton: document.getElementById('delete-current-endpoint-button'),
  previewEndpointClassificationButton: document.getElementById('preview-endpoint-classification-button'),
  applyEndpointClassificationButton: document.getElementById('apply-endpoint-classification-button'),
  endpointClassificationPreview: document.getElementById('endpoint-classification-preview'),
  resetEndpointsButton: document.getElementById('reset-endpoints-button'),
  saveEndpointsButton: document.getElementById('save-endpoints-button'),
  endpointPageSize: document.getElementById('endpoint-page-size'),
  endpointPrevPageButton: document.getElementById('endpoint-prev-page'),
  endpointPageStatus: document.getElementById('endpoint-page-status'),
  endpointNextPageButton: document.getElementById('endpoint-next-page'),
  endpointFields: {
    ip: document.getElementById('endpoint-ip'),
    hostname: document.getElementById('endpoint-hostname'),
    fqdn: document.getElementById('endpoint-fqdn'),
    group: document.getElementById('endpoint-group'),
    description: document.getElementById('endpoint-description'),
    entitytype: document.getElementById('endpoint-entitytype'),
    device: document.getElementById('endpoint-device'),
    vendor: document.getElementById('endpoint-vendor'),
    asset_id: document.getElementById('endpoint-asset-id'),
    device_mode: document.getElementById('endpoint-device-mode'),
    additional_notes: document.getElementById('endpoint-notes'),
    alerting_enabled: document.getElementById('endpoint-alerting-enabled'),
    alerting_reason: document.getElementById('endpoint-alerting-reason'),
    dynamic_address: document.getElementById('endpoint-dynamic-address'),
    classification_source: document.getElementById('endpoint-classification-source'),
    monitoring_enabled: document.getElementById('endpoint-monitoring-enabled'),
    maintenance_until: document.getElementById('endpoint-maintenance-until'),
    maintenance_reason: document.getElementById('endpoint-maintenance-reason'),
  },
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
  cancelDiscoveryButton: document.getElementById('cancel-discovery-button'),
  discoveryPreflight: document.getElementById('discovery-preflight'),
  selectAllDiscoveryButton: document.getElementById('select-all-discovery-button'),
  deselectAllDiscoveryButton: document.getElementById('deselect-all-discovery-button'),
  markDiscoveryMaintenanceButton: document.getElementById('mark-discovery-maintenance-button'),
  markDiscoveryProductionButton: document.getElementById('mark-discovery-production-button'),
  disableDiscoveryAlertingButton: document.getElementById('disable-discovery-alerting-button'),
  enableDiscoveryAlertingButton: document.getElementById('enable-discovery-alerting-button'),
  markDiscoveryDynamicButton: document.getElementById('mark-discovery-dynamic-button'),
  markDiscoveryStaticButton: document.getElementById('mark-discovery-static-button'),
  deferDiscoverySelectedButton: document.getElementById('defer-discovery-selected-button'),
  ignoreDiscoverySelectedButton: document.getElementById('ignore-discovery-selected-button'),
  resetDiscoveryReviewButton: document.getElementById('reset-discovery-review-button'),
  discoveryReviewNote: document.getElementById('discovery-review-note'),
  discoveryReviewFilterButtons: Array.from(document.querySelectorAll('[data-discovery-review-filter]')),
  discoveryBulkFields: {
    group: document.getElementById('discovery-bulk-group'),
    entitytype: document.getElementById('discovery-bulk-entitytype'),
    device: document.getElementById('discovery-bulk-device'),
    vendor: document.getElementById('discovery-bulk-vendor'),
  },
  applyDiscoveryBulkFieldsButton: document.getElementById('apply-discovery-bulk-fields-button'),
  previewDiscoveryClassificationButton: document.getElementById('preview-discovery-classification-button'),
  applyDiscoveryClassificationButton: document.getElementById('apply-discovery-classification-button'),
  discoveryClassificationPreview: document.getElementById('discovery-classification-preview'),
  exportDiscoveryAllButton: document.getElementById('export-discovery-all-button'),
  exportDiscoverySelectedButton: document.getElementById('export-discovery-selected-button'),
  addDiscoverySelectedButton: document.getElementById('add-discovery-selected-button'),
  discoverySelectionStatus: document.getElementById('discovery-selection-status'),
  discoveryTableStatus: document.getElementById('discovery-table-status'),
  discoveryMergeMode: document.getElementById('discovery-merge-mode'),
  discoveryPageSize: document.getElementById('discovery-page-size'),
  discoveryPrevPageButton: document.getElementById('discovery-prev-page'),
  discoveryPageStatus: document.getElementById('discovery-page-status'),
  discoveryNextPageButton: document.getElementById('discovery-next-page'),
  refreshDiscoveryHistoryButton: document.getElementById('refresh-discovery-history-button'),
  discoveryScheduleStatuses: document.getElementById('discovery-schedule-statuses'),
  discoveryHistoryStatus: document.getElementById('discovery-history-status'),
  discoveryRetentionStatus: document.getElementById('discovery-retention-status'),
  discoveryHistoryRows: document.getElementById('discovery-history-rows'),
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
  addDiscoverySubnetButton: document.getElementById('add-discovery-subnet-button'),
  discoverySubnetRows: document.getElementById('discovery-subnet-rows'),
  addClassificationRuleButton: document.getElementById('add-classification-rule-button'),
  classificationRuleRows: document.getElementById('classification-rule-rows'),
  classificationPreviewHostname: document.getElementById('classification-preview-hostname'),
  classificationPreviewFQDN: document.getElementById('classification-preview-fqdn'),
  previewClassificationSampleButton: document.getElementById('preview-classification-sample-button'),
  classificationPreviewResult: document.getElementById('classification-preview-result'),
  settingsFields: {
    pingsPerCycle: document.getElementById('cfg-pings-per-cycle'),
    cycleInterval: document.getElementById('cfg-cycle-interval'),
    timeoutMs: document.getElementById('cfg-timeout-ms'),
    parallelThreads: document.getElementById('cfg-parallel-threads'),
    emitIndividualPings: document.getElementById('cfg-emit-individual-pings'),
    outputMode: document.getElementById('cfg-output-mode'),
    logPath: document.getElementById('cfg-log-path'),
    logRotation: document.getElementById('cfg-log-rotation'),
    logRetentionFiles: document.getElementById('cfg-log-retention-files'),
    logRetentionDays: document.getElementById('cfg-log-retention-days'),
    logCompressRotated: document.getElementById('cfg-log-compress-rotated'),
    discoveryEnabled: document.getElementById('cfg-discovery-enabled'),
    discoveryID: document.getElementById('cfg-discovery-id'),
    discoveryHistoryPath: document.getElementById('cfg-discovery-history-path'),
    discoveryRetentionScans: document.getElementById('cfg-discovery-retention-scans'),
    discoveryRetentionDays: document.getElementById('cfg-discovery-retention-days'),
    discoveryTargets: document.getElementById('cfg-discovery-targets'),
    discoveryDay: document.getElementById('cfg-discovery-day'),
    discoveryTime: document.getElementById('cfg-discovery-time'),
    discoveryTimezone: document.getElementById('cfg-discovery-timezone'),
    discoveryScheduleTimeout: document.getElementById('cfg-discovery-schedule-timeout'),
    discoveryConcurrency: document.getElementById('cfg-discovery-concurrency'),
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
    hecMaxBufferEvents: document.getElementById('cfg-hec-max-buffer-events'),
    hecMaxBufferBytes: document.getElementById('cfg-hec-max-buffer-bytes'),
    hecRetryEnabled: document.getElementById('cfg-hec-retry-enabled'),
    hecMaxAttempts: document.getElementById('cfg-hec-max-attempts'),
    hecBaseDelayMs: document.getElementById('cfg-hec-base-delay-ms'),
    hecJitterPct: document.getElementById('cfg-hec-jitter-pct'),
    hecBackoff: document.getElementById('cfg-hec-backoff'),
    hecRetryCount: document.getElementById('cfg-hec-retry-count'),
    hecRetryDelayMs: document.getElementById('cfg-hec-retry-delay-ms'),
    hecUseACK: document.getElementById('cfg-hec-use-ack'),
    hecACKTimeout: document.getElementById('cfg-hec-ack-timeout'),
    hecACKPoll: document.getElementById('cfg-hec-ack-poll'),
    hecChannel: document.getElementById('cfg-hec-channel'),
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
    metricsUseACK: document.getElementById('cfg-metrics-use-ack'),
    metricsACKTimeout: document.getElementById('cfg-metrics-ack-timeout'),
    metricsACKPoll: document.getElementById('cfg-metrics-ack-poll'),
    metricsChannel: document.getElementById('cfg-metrics-channel'),
    deliverySpoolPath: document.getElementById('cfg-delivery-spool-path'),
    deliveryMaxBytes: document.getElementById('cfg-delivery-max-bytes'),
    deliveryMaxEnvelopes: document.getElementById('cfg-delivery-max-envelopes'),
    deliveryDrainMax: document.getElementById('cfg-delivery-drain-max'),
  },
};

const sectionHashes = ['#overview', '#advisor', '#inventory', '#discovery', '#settings'];

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
  'Weekly Discovery Schedule': panelHelp(
    'Weekly Discovery Schedule',
    'Runs bounded subnet discovery on a weekly calendar and stores the results for review without automatically changing monitored inventory.',
    [
      'A scheduled result is discovery evidence, not a declaration that an asset should be monitored or removed.',
      'Missed occurrences are eligible for catch-up after collector startup. Schedule status and errors appear in Discovery Operations.',
      'Use retention controls to keep long-running discovery history bounded.',
    ],
  ),
  'Discovery Subnet Catalog': panelHelp(
    'Discovery Subnet Catalog',
    'Pairs a CIDR with operator-owned subnet metadata and the address-allocation model used for discovery identity decisions.',
    [
      'ID is a stable internal label; CIDR controls matching. Name, VLAN, location, and routing domain are carried into discovery evidence.',
      'Static addresses use IP as the default identity. DHCP/dynamic addresses require Asset ID or forward-confirmed FQDN before they count as a durable new or missing asset.',
      'When catalogs overlap, the most-specific CIDR wins.',
    ],
  ),
  'Naming Convention Rules': panelHelp(
    'Naming Convention Rules',
    'Evaluates ordered regular-expression match/assignment pairs against hostname, FQDN, or either name and proposes structured CMDB fields.',
    [
      'Each pair contains a stable rule ID, name source, RE2 pattern, and optional Group, Entity Type, Device, and Vendor assignments.',
      'Named captures such as (?P<site>...) can be reused in assignment templates as ${site}.',
      'Fill blanks is the safe default. Overwrite must be enabled explicitly. Stop on match prevents later rules from running after a match.',
      'Use Test Rule Pairs before saving. Discovery uses only the saved configuration; the preview also supports unsaved draft rules.',
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
  'Durable Delivery': panelHelp(
    'Durable Delivery',
    'Controls the disk-backed outbox that protects unsent event and metrics envelopes during Splunk outages.',
    [
      'The outbox decouples measurement from delivery so a temporary HEC failure does not silently erase observations.',
      'Capacity limits protect the collector host from unbounded disk use. Exhaustion is reported as a delivery fault and should be alerted on.',
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
      'Discovery scan summaries and per-IP evidence use this same event pipeline in v5.10 and later.',
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
  'cfg-log-retention-files': helpTopic(
    'Retain Rotated Files',
    'Caps the number of rotated result-log archives kept beside the active log.',
    nonNegativeIntegerFormat,
    ['0 preserves all rotated archives. That is backward compatible but allows total directory use to grow without a count bound.'],
  ),
  'cfg-log-retention-days': helpTopic(
    'Retain Rotated Logs (days)',
    'Removes rotated result-log archives older than the configured age.',
    nonNegativeIntegerFormat,
    ['0 disables age-based removal. When count and age are both set, an archive is removed when either limit excludes it.'],
  ),
  'cfg-log-compress-rotated': helpTopic(
    'Compress Rotated Logs',
    'Gzip-compresses closed result-log archives after the active writer has safely reopened.',
    checkboxFormat,
    ['Compression reduces disk use and never changes the active NDJSON file.'],
  ),
  'cfg-discovery-enabled': helpTopic(
    'Enable Weekly Discovery',
    'Enables the primary weekly subnet-discovery schedule.',
    checkboxFormat,
    ['Saving this setting requires collector restart before the monitoring runtime uses the new schedule. Scheduled imports always remain review-only.'],
  ),
  'cfg-discovery-id': helpTopic(
    'Discovery Schedule ID',
    'Provides a stable name used to track schedule state, errors, and the source of retained scans.',
    'Non-empty text, unique across schedules.',
    ['Keep the ID stable so operational history remains attributable after configuration edits.'],
  ),
  'cfg-discovery-history-path': helpTopic(
    'Discovery History Path',
    'Stores durable scan snapshots, the compact history index, per-target baselines, and schedule state.',
    filePathFormat,
    ['The service account needs create, read, replace, and delete access when retention is enabled.'],
  ),
  'cfg-discovery-retention-scans': helpTopic(
    'Retain Most Recent Scans',
    'Caps discovery history by the number of completed target scans retained across schedules.',
    nonNegativeIntegerFormat,
    ['0 disables the count limit. On deployments with many targets, choose a value large enough to retain at least the desired number of weekly observations per target.'],
  ),
  'cfg-discovery-retention-days': helpTopic(
    'Retain Scan History (days)',
    'Removes discovery scan snapshots older than the configured number of days.',
    nonNegativeIntegerFormat,
    ['0 disables the age limit. New versioned configurations default to 365 days; legacy files remain unchanged until explicitly configured.'],
  ),
  'cfg-discovery-targets': helpTopic(
    'Discovery Target Networks',
    'Lists the IPv4 CIDR networks scanned by the weekly schedule.',
    'One IPv4 CIDR per line, from /16 through /30.',
    [
      'A whole /8 is intentionally rejected because it contains more than 16 million addresses.',
      'Each target produces independent history and new/missing deltas.',
    ],
  ),
  'cfg-discovery-day': helpTopic(
    'Discovery Day',
    'Selects the weekday for the schedule in its configured timezone.',
    'Sunday through Saturday.',
  ),
  'cfg-discovery-time': helpTopic(
    'Discovery Local Time',
    'Selects the 24-hour wall-clock time in the configured schedule timezone.',
    'HH:MM.',
    ['A missed occurrence is eligible for catch-up after collector startup.'],
  ),
  'cfg-discovery-timezone': helpTopic(
    'Discovery Timezone',
    'Defines which timezone interprets the configured weekday and time.',
    'Local or an IANA timezone such as America/New_York.',
    ['Use an explicit IANA timezone when daylight-saving behavior must follow a specific location.'],
  ),
  'cfg-discovery-schedule-timeout': helpTopic(
    'Scheduled Discovery Timeout',
    'Sets how long each discovery ICMP attempt waits before the address is considered unobserved for that scan.',
    'Whole number milliseconds, 100 or higher.',
    ['An unobserved address is not automatically down or decommissioned; firewalls and transient loss can also suppress a response.'],
  ),
  'cfg-discovery-concurrency': helpTopic(
    'Scheduled Discovery Concurrency',
    'Caps how many target addresses the discovery script probes concurrently within one network.',
    positiveIntegerFormat,
    ['Higher values shorten scans but increase burst load on the collector and network. Scheduled target networks are processed one at a time.'],
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
    ['Existing tokens are write-only and are never returned by the API. Leave this blank to preserve the configured token, or enter a new value to replace it.'],
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
  'cfg-hec-verify-ssl': helpTopic(
    'Verify TLS Certificates',
    'Controls whether the runtime validates the remote HEC server certificate.',
    checkboxFormat,
    ['Turn this off only for controlled troubleshooting or self-signed environments where you accept the risk.'],
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
  'cfg-hec-use-ack': helpTopic(
    'Require HEC Indexer Acknowledgement',
    'Waits for Splunk to confirm that accepted event batches reached the indexing pipeline.',
    checkboxFormat,
    ['This improves delivery certainty but adds HEC channel state, polling, and latency. Splunk HEC acknowledgement must be enabled server-side.'],
  ),
  'cfg-hec-ack-timeout': helpTopic(
    'HEC ACK Timeout',
    'Limits how long the collector waits for indexer acknowledgement before treating delivery as failed.',
    positiveIntegerFormat,
    ['Timeout is measured in seconds. A timeout causes durable retry behavior; it does not change the underlying ping observation.'],
  ),
  'cfg-hec-ack-poll': helpTopic(
    'HEC ACK Poll Interval',
    'Controls how often the collector asks Splunk whether an acknowledged batch has been indexed.',
    'Whole number milliseconds, 50 or higher.',
  ),
  'cfg-hec-channel': helpTopic(
    'HEC ACK Channel',
    'Provides the stable channel identifier required by Splunk indexer acknowledgement.',
    'Text channel identifier.',
    ['Use a channel unique to this collector instance to avoid acknowledgement-state collisions.'],
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
      'Discovery observations have no metrics equivalent. Use dual when the Splunk Discovery Inventory dashboard must receive scan evidence.',
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
    ['Existing tokens are write-only and are never returned by the API. Leave this blank to preserve the configured token, or enter a new value to replace it.'],
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
  'cfg-metrics-use-ack': helpTopic(
    'Require Metrics HEC Acknowledgement',
    'Waits for Splunk to acknowledge metrics batches before they are considered durably delivered.',
    checkboxFormat,
    ['Enable only when the metrics HEC input supports indexer acknowledgement.'],
  ),
  'cfg-metrics-ack-timeout': helpTopic(
    'Metrics ACK Timeout',
    'Limits how long the collector waits for metrics indexer acknowledgement.',
    positiveIntegerFormat,
  ),
  'cfg-metrics-ack-poll': helpTopic(
    'Metrics ACK Poll Interval',
    'Controls how frequently acknowledgement state is checked for metrics batches.',
    'Whole number milliseconds, 50 or higher.',
  ),
  'cfg-metrics-channel': helpTopic(
    'Metrics ACK Channel',
    'Provides the stable channel identifier used for metrics indexer acknowledgement.',
    'Text channel identifier unique to this collector.',
  ),
  'cfg-delivery-spool-path': helpTopic(
    'Durable Outbox Path',
    'Stores unsent delivery envelopes on disk so temporary Splunk outages do not erase observations.',
    filePathFormat,
    ['Place this on reliable local storage and grant the collector service account read, create, replace, and delete permissions.'],
  ),
  'cfg-delivery-max-bytes': helpTopic(
    'Maximum Outbox Size',
    'Caps total disk space used by queued delivery envelopes.',
    byteSizeFormat,
    ['When the cap is exhausted, delivery health becomes unhealthy. This protects the host but means new output cannot be durably queued indefinitely.'],
  ),
  'cfg-delivery-max-envelopes': helpTopic(
    'Maximum Outbox Envelopes',
    'Caps the number of queued event or metrics batches independently of their total byte size.',
    positiveIntegerFormat,
  ),
  'cfg-delivery-drain-max': helpTopic(
    'Drain Limit Per Pass',
    'Limits how many queued envelopes are retried during one outbox drain pass.',
    positiveIntegerFormat,
    ['This prevents recovery traffic from monopolizing the collector after a long Splunk outage.'],
  ),
};

const interfacePanelHelp = {
  'Endpoint Editor': panelHelp(
    'Endpoint Editor',
    'Edits the monitored inventory and its explicit monitoring policy. Saving creates a backup and requests an in-process inventory reload.',
    [
      'Maintenance mode suppresses probes and emits monitoring_control evidence instead of synthetic packet loss.',
      'Legacy Dev/Test remains available only for backward compatibility and continues to probe.',
      'Alerting Disabled keeps measurement active while marking the signal ineligible for supported alert searches.',
      'Discovery provenance remains attached when reviewed results are added to the endpoint inventory.',
    ],
  ),
  'Discovery Controls': panelHelp(
    'Discovery Controls',
    'Runs a bounded, operator-initiated IPv4 ICMP scan and stages responding addresses for review.',
    [
      'Discovery records an observation, latency, and DNS evidence. No response is not proof that an asset does not exist.',
      'Running a scan never changes endpoints.csv by itself.',
    ],
  ),
  'Discovery Results': panelHelp(
    'Discovery Results',
    'Shows the current manual scan or a retained historical result set before CSV export or reviewed import.',
    [
      'Selecting Add to Device List changes only the in-browser endpoint draft; Save Endpoints is still required.',
      'Needs Review is the default for unknown observations. Approved is derived from the saved endpoint inventory; Deferred and Ignored decisions are retained separately from immutable scan evidence.',
      'A newly observed device must be explicitly marked Production or Maintenance before it can be staged into the endpoint inventory.',
      'FQDN is reverse-DNS evidence with forward-confirmation status, not an authoritative asset identity.',
      'CSV exports contain the full discovery provenance schema, quote every field, and neutralize spreadsheet formula prefixes in discovery-controlled text.',
    ],
  ),
  'Bulk Discovery Review': panelHelp(
    'Bulk Discovery Review',
    'Applies reviewed metadata and naming-rule results to selected discovery rows before they enter the endpoint draft.',
    [
      'Blank common fields are ignored, so one field can be updated without clearing the others.',
      'Preview Naming Rules does not mutate results. Apply Naming Rules records the matching rule IDs as classification provenance.',
      'Mark DHCP/Dynamic when an IP can move between devices. Such rows need Asset ID or forward-confirmed FQDN for durable discovery-delta identity.',
      'Defer, Ignore, and Return to Needs Review write the selected metadata and decision to the durable discovery review registry.',
    ],
  ),
  'Discovery Operations': panelHelp(
    'Discovery Operations',
    'Shows weekly schedule health and durable scan history, including new and missing deltas against the prior scan for the same target.',
    [
      'New means observed now but not in the prior retained baseline.',
      'Missing means observed previously but not now. It must not be interpreted as confirmed downtime, deletion, or decommissioning.',
      'All, New, and Missing load a retained evidence set into the normal review/export workflow.',
      'Each completed scan is also accepted by the configured event output pipeline as a scan summary plus per-IP evidence for the Splunk Discovery Inventory dashboard.',
    ],
  ),
  'Current schedule': panelHelp(
    'Current Schedule Capacity',
    'Models whether the configured workers can finish all worst-case endpoint probe budgets inside one monitoring interval.',
    ['Admission is intentionally conservative so a nominal one-minute cadence does not silently become slower under maximum timeouts.'],
  ),
  'Findings and recommendations': panelHelp(
    'Findings and Recommendations',
    'Lists all detectable inventory, capacity, output, retention, and schedule issues instead of stopping at the first error.',
    ['Blockers prevent a safe start or restart. Warnings identify risk that remains operator-selectable.'],
  ),
  'Preview changes before applying': panelHelp(
    'Preview Changes',
    'Shows the exact configuration or inventory mutations proposed by safe fixes or an operating profile.',
    ['Nothing is written until the operator confirms the apply action. A backup and revision check protect concurrent edits.'],
  ),
  'Non-SLA capacity evidence': panelHelp(
    'Non-SLA Capacity Evidence',
    'Benchmarks local parsing, planning, temporary writes, and loopback probes without contacting monitored endpoints or Splunk.',
    ['Use this as host-readiness evidence, not as proof of network latency or contractor SLA performance.'],
  ),
};

const interfaceFieldHelp = {
  'endpoint-ip': helpTopic(
    'Endpoint IP',
    'Defines the literal IPv4 or IPv6 address the collector probes.',
    'A valid IP address; DNS names are not accepted in this field.',
    ['The IP is measurement routing, not necessarily durable asset identity in DHCP environments. Duplicate target IPs are rejected.'],
  ),
  'endpoint-hostname': helpTopic(
    'Endpoint Hostname',
    'Provides the required operator-facing short name carried into events and inventory views.',
    'Non-empty text without tabs or line breaks.',
    ['This label does not change which address is probed.'],
  ),
  'endpoint-fqdn': helpTopic(
    'Endpoint FQDN',
    'Stores the best available fully qualified DNS name for inventory correlation.',
    'Optional DNS name.',
    ['Discovery records whether forward lookup confirmed the original IP. DNS evidence can be stale or absent and should not replace a stable CMDB identity.'],
  ),
  'endpoint-group': helpTopic(
    'Endpoint Group',
    'Provides an operator-defined grouping used in events, filters, and Splunk breakdowns.',
    'Text; blank values normalize to default.',
  ),
  'endpoint-description': helpTopic(
    'Endpoint Description',
    'Stores human-readable context about the monitored target.',
    'Optional text.',
  ),
  'endpoint-entitytype': helpTopic(
    'Entity Type',
    'Classifies the broad asset or service type used for inventory enrichment.',
    'Optional text such as network, server, service, or appliance.',
  ),
  'endpoint-device': helpTopic(
    'Device',
    'Stores a more specific device or platform classification.',
    'Optional text such as router, switch, firewall, VM, or physical host.',
  ),
  'endpoint-vendor': helpTopic(
    'Vendor',
    'Stores an operator- or discovery-supplied vendor label for CMDB correlation.',
    'Optional text.',
    ['ICMP discovery cannot authoritatively determine manufacturer. Treat inferred values as enrichment, not measurement truth.'],
  ),
  'endpoint-notes': helpTopic(
    'Additional Notes',
    'Stores free-form operational context that travels with endpoint metadata.',
    'Optional text.',
  ),
  'endpoint-asset-id': helpTopic(
    'Stable Asset ID',
    'Stores an operator- or CMDB-assigned identity that remains stable when an address changes.',
    'Optional text without tabs or line breaks.',
    ['For DHCP/dynamic devices this is the preferred discovery identity.'],
  ),
  'endpoint-device-mode': helpTopic(
    'Device Mode',
    'Controls the explicit operational role of the endpoint.',
    'Production, Maintenance, or Legacy Dev/Test.',
    [
      'Maintenance sends no ICMP and reports a truthful suppressed state.',
      'Legacy Dev/Test preserves existing dev=true behavior and continues to ping.',
    ],
  ),
  'endpoint-alerting-enabled': helpTopic(
    'Alerting Enabled',
    'Marks measurements as eligible for supported Splunk alert searches without changing collection.',
    checkboxFormat,
    ['When disabled, pings continue and dashboards retain the evidence.'],
  ),
  'endpoint-alerting-reason': helpTopic(
    'Alerting Disabled Reason',
    'Records why supported alerts should ignore this endpoint while measurements continue.',
    'Optional operator-entered text.',
  ),
  'endpoint-dynamic-address': helpTopic(
    'Dynamic Address',
    'Marks the IP as DHCP or otherwise movable so discovery does not mistake ordinary lease churn for a durable new asset.',
    checkboxFormat,
    ['Supply Stable Asset ID when possible. Forward-confirmed FQDN is the fallback identity; unresolved dynamic rows are kept as evidence but excluded from new/missing asset counts.'],
  ),
  'endpoint-classification-source': helpTopic(
    'Classification Source',
    'Shows provenance for fields populated by naming convention rules.',
    'Read-only comma-separated rule identifiers.',
  ),
  'endpoint-monitoring-enabled': helpTopic(
    'Monitoring Enabled',
    'Controls whether the collector schedules ICMP probes for this endpoint.',
    checkboxFormat,
    ['When disabled, no ping is sent. The collector emits a truthful monitoring_control record with measurement_valid=false instead of reporting artificial loss.'],
  ),
  'endpoint-maintenance-until': helpTopic(
    'Maintenance Until',
    'Suppresses probes until the specified instant, then automatically resumes monitoring.',
    'Optional RFC3339 timestamp such as 2026-07-28T04:00:00Z.',
    ['While active, state is maintenance and observation status is suppressed—not down.'],
  ),
  'endpoint-maintenance-reason': helpTopic(
    'Maintenance Reason',
    'Records why an endpoint is intentionally suppressed so dashboards and responders have context.',
    'Optional text.',
  ),
  'discovery-target-network': helpTopic(
    'Discovery Target Network',
    'Selects the IPv4 network for a manual scan.',
    'IPv4 address, IPv4 CIDR, or blank to infer the local subnet.',
    ['CIDRs are bounded to /16 through /30. Large ranges require explicit confirmation and can generate substantial traffic and history.'],
  ),
  'discovery-subnet-mask': helpTopic(
    'Discovery Subnet Mask',
    'Controls how many addresses are considered when the target does not already include a CIDR prefix.',
    'Whole number from 16 through 30.',
    ['A /16 contains up to 65,534 usable host addresses; a /24 contains up to 254.'],
  ),
  'discovery-timeout-ms': helpTopic(
    'Manual Discovery Timeout',
    'Sets the per-address ICMP wait used by a manual discovery scan.',
    'Whole number milliseconds, 100 or higher.',
    ['Timeout is a censoring boundary. No reply may mean filtering, congestion, sleep, or absence; discovery does not label it confirmed down.'],
  ),
  'discovery-throttle-limit': helpTopic(
    'Manual Discovery Throttle',
    'Caps concurrent address probes during a manual scan.',
    positiveIntegerFormat,
    ['Increase carefully: higher concurrency finishes sooner but creates a larger local and network burst.'],
  ),
  'discovery-merge-mode': helpTopic(
    'Duplicate Handling',
    'Controls how reviewed discovery evidence is combined with an endpoint that already has the same IP.',
    'Skip duplicate IPs, fill blank fields, or overwrite existing fields.',
    [
      'Skip preserves the existing record unchanged.',
      'Fill blanks adds missing identity fields while preserving operator-entered values.',
      'Overwrite replaces nonblank identity fields with reviewed discovery values; monitoring policy is not silently disabled.',
    ],
  ),
  'discovery-bulk-group': helpTopic('Bulk Group', 'Applies a reviewed Group value to selected discovery rows.', 'Optional text; blank is ignored.'),
  'discovery-bulk-entitytype': helpTopic('Bulk Entity Type', 'Applies a reviewed Entity Type to selected discovery rows.', 'Optional text; blank is ignored.'),
  'discovery-bulk-device': helpTopic('Bulk Device', 'Applies a reviewed Device classification to selected discovery rows.', 'Optional text; blank is ignored.'),
  'discovery-bulk-vendor': helpTopic('Bulk Vendor', 'Applies a reviewed Vendor to selected discovery rows.', 'Optional text; blank is ignored.'),
  'discovery-review-note': helpTopic(
    'Review Note',
    'Records why selected discovery results were deferred, ignored, or approved into inventory.',
    'Optional single-line text, up to 2,000 characters.',
    ['The note is persisted when a review-state action is used or when the staged endpoint inventory is saved.'],
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

function formatDuration(seconds) {
  const value = Math.max(0, Number(seconds) || 0);
  if (value < 60) {
    return `${Math.floor(value)}s`;
  }
  if (value < 3600) {
    return `${Math.floor(value / 60)}m ${Math.floor(value % 60)}s`;
  }
  const hours = Math.floor(value / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  return `${hours}h ${minutes}m`;
}

function formatTimestamp(value, fallback = 'Not available') {
  if (!value) {
    return fallback;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return fallback;
  }
  return parsed.toLocaleString();
}

function titleCase(value) {
  return String(value || 'unknown')
    .replaceAll('_', ' ')
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function emptyEndpoint() {
  return {
    ip: '',
    hostname: '',
    fqdn: '',
    group: 'default',
    description: '',
    entitytype: '',
    device: '',
    vendor: '',
    asset_id: '',
    additional_notes: '',
    dev: false,
    device_mode: 'production',
    alerting_enabled: true,
    alerting_reason: '',
    dynamic_address: false,
    classification_source: '',
    monitoring_enabled: true,
    maintenance_until: '',
    maintenance_reason: '',
  };
}

function effectiveDeviceMode(endpoint) {
  const explicit = String(endpoint?.device_mode || '').trim().toLowerCase();
  if (['production', 'maintenance', 'legacy_dev'].includes(explicit)) {
    return explicit;
  }
  return endpoint?.dev ? 'legacy_dev' : 'production';
}

function effectiveDiscoveryReviewState(endpoint) {
  return globalThis.PingMonitorDiscoveryReview.reviewState(endpoint);
}

function effectiveDiscoveryDeviceMode(endpoint) {
  return globalThis.PingMonitorDiscoveryReview.deviceMode(endpoint);
}

function normalizeEndpoint(endpoint) {
  const deviceMode = effectiveDeviceMode(endpoint);
  return {
    ...endpoint,
    endpoint_id: String(endpoint.endpoint_id || '').trim(),
    ip: String(endpoint.ip || '').trim(),
    hostname: String(endpoint.hostname || '').trim(),
    fqdn: String(endpoint.fqdn || '').trim(),
    group: String(endpoint.group || 'default').trim() || 'default',
    description: String(endpoint.description || '').trim(),
    entitytype: String(endpoint.entitytype || '').trim(),
    device: String(endpoint.device || '').trim(),
    vendor: String(endpoint.vendor || '').trim(),
    asset_id: String(endpoint.asset_id || '').trim(),
    additional_notes: String(endpoint.additional_notes || '').trim(),
    dev: deviceMode === 'legacy_dev',
    device_mode: deviceMode,
    alerting_enabled: endpoint.alerting_enabled !== false,
    alerting_reason: String(endpoint.alerting_reason || '').trim(),
    dynamic_address: Boolean(endpoint.dynamic_address),
    subnet_id: String(endpoint.subnet_id || '').trim(),
    subnet_name: String(endpoint.subnet_name || '').trim(),
    subnet_vlan: String(endpoint.subnet_vlan || '').trim(),
    subnet_location: String(endpoint.subnet_location || '').trim(),
    addressing_mode: String(endpoint.addressing_mode || '').trim(),
    routing_domain: String(endpoint.routing_domain || '').trim(),
    classification_source: String(endpoint.classification_source || '').trim(),
    discovery_review_state: String(endpoint.discovery_review_state || '').trim().toLowerCase(),
    discovery_reviewed_at: String(endpoint.discovery_reviewed_at || '').trim(),
    discovery_review_note: String(endpoint.discovery_review_note || '').trim(),
    monitoring_enabled: endpoint.monitoring_enabled !== false,
    maintenance_until: String(endpoint.maintenance_until || '').trim(),
    maintenance_reason: String(endpoint.maintenance_reason || '').trim(),
  };
}

function normalizeEndpoints(endpoints) {
  return endpoints.map((endpoint) => normalizeEndpoint(endpoint));
}

function normalizeDiscoveryEndpoint(endpoint) {
  const explicitMode = String(endpoint?.device_mode || '').trim().toLowerCase();
  const normalized = normalizeEndpoint(endpoint || {});
  if (!explicitMode && !endpoint?.dev && effectiveDiscoveryReviewState(endpoint) !== 'approved') {
    normalized.device_mode = '';
  }
  return normalized;
}

function normalizeDiscoveryEndpoints(endpoints) {
  return endpoints.map((endpoint) => normalizeDiscoveryEndpoint(endpoint));
}

function isValidIPAddress(value) {
  const text = String(value || '').trim();
  const octets = text.split('.');
  if (octets.length === 4 && octets.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)) {
    return true;
  }
  if (text.includes(':')) {
    try {
      return new URL(`http://[${text}]/`).hostname.length > 0;
    } catch {
      return false;
    }
  }
  return false;
}

function validateEndpointDraft() {
  const seen = new Map();
  const seenAssetIDs = new Map();
  for (let index = 0; index < state.endpoints.length; index += 1) {
    const endpoint = state.endpoints[index];
    const ip = String(endpoint.ip || '').trim();
    const hostname = String(endpoint.hostname || '').trim();
    if (!isValidIPAddress(ip)) {
      return { index, field: 'ip', message: `Endpoint ${index + 1} needs a valid IP address.` };
    }
    const key = ip.toLowerCase();
    if (seen.has(key)) {
      return { index, field: 'ip', message: `Endpoint ${index + 1} duplicates the IP address from endpoint ${seen.get(key) + 1}.` };
    }
    seen.set(key, index);
    if (!hostname || hostname.length > 253 || /[\r\n\t]/.test(hostname)) {
      return { index, field: 'hostname', message: `Endpoint ${index + 1} needs a valid hostname.` };
    }
    const fqdn = String(endpoint.fqdn || '').trim();
    if (fqdn.length > 253 || /[\r\n\t]/.test(fqdn)) {
      return { index, field: 'fqdn', message: `Endpoint ${index + 1} has an invalid FQDN.` };
    }
    if (endpoint.maintenance_until && Number.isNaN(Date.parse(endpoint.maintenance_until))) {
      return { index, field: 'maintenance_until', message: `Endpoint ${index + 1} maintenance time must use RFC3339, for example 2026-07-28T04:00:00Z.` };
    }
    if (!['production', 'maintenance', 'legacy_dev'].includes(effectiveDeviceMode(endpoint))) {
      return { index, field: 'device_mode', message: `Endpoint ${index + 1} has an invalid device mode.` };
    }
    if (String(endpoint.asset_id || '').length > 253 || /[\r\n\t]/.test(String(endpoint.asset_id || ''))) {
      return { index, field: 'asset_id', message: `Endpoint ${index + 1} has an invalid stable Asset ID.` };
    }
	const assetID = String(endpoint.asset_id || '').trim().toLowerCase();
	if (assetID && seenAssetIDs.has(assetID)) {
	  return { index, field: 'asset_id', message: `Endpoint ${index + 1} duplicates the stable Asset ID from endpoint ${seenAssetIDs.get(assetID) + 1}.` };
	}
	if (assetID) {
	  seenAssetIDs.set(assetID, index);
	}
  }
  return null;
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
  return state.endpointsDirty;
}

function configIsDirty() {
  return state.configDirty;
}

function hasUnsavedChanges() {
  return endpointsAreDirty() || configIsDirty();
}

function confirmDiscardChanges(message = 'Discard unsaved changes and reload from disk?') {
  return !hasUnsavedChanges() || window.confirm(message);
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
    const key = helpKey.slice(6);
    return settingsFieldHelp[key] || interfaceFieldHelp[key] || null;
  }
  if (helpKey.startsWith('panel:')) {
    const key = helpKey.slice(6);
    return settingsPanelHelp[key] || interfacePanelHelp[key] || null;
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

function injectInterfacePanelHelpButtons() {
  document.querySelectorAll('.panel').forEach((card) => {
    const titleElement = card.querySelector('.panel-title');
    const title = titleElement?.textContent?.trim();
    if (!titleElement || !title || !interfacePanelHelp[title]) {
      return;
    }
    let titleRow = titleElement.parentElement;
    if (!titleRow || !titleRow.classList.contains('panel-title-row')) {
      titleRow = document.createElement('div');
      titleRow.className = 'panel-title-row';
      titleElement.replaceWith(titleRow);
      titleRow.appendChild(titleElement);
    }
    if (!titleRow.querySelector(`[data-help-key="panel:${title}"]`)) {
      titleRow.appendChild(createSettingsHelpButton(`panel:${title}`, title, 'panel'));
    }
  });
}

function injectInterfaceFieldHelpButtons() {
  Object.entries(interfaceFieldHelp).forEach(([fieldID, help]) => {
    const field = document.getElementById(fieldID);
    if (!(field instanceof HTMLElement)) {
      return;
    }
    const container = field.closest('.field-group, .checkbox-row, .inline-field');
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
    if (!textRow.querySelector(`[data-help-key="field:${fieldID}"]`)) {
      textRow.appendChild(createSettingsHelpButton(`field:${fieldID}`, help.title, 'inline'));
    }
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

  document.body.addEventListener('click', (event) => {
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
  injectInterfacePanelHelpButtons();
  injectInterfaceFieldHelpButtons();
}

function initializeAdvancedSettings() {
  const groups = [
    {
      anchor: 'cfg-hec-enabled',
      fields: ['cfg-hec-ssl-protocol', 'cfg-hec-batch-size', 'cfg-hec-max-buffer-events', 'cfg-hec-max-buffer-bytes', 'cfg-hec-retry-count', 'cfg-hec-retry-delay-ms', 'cfg-hec-ack-timeout', 'cfg-hec-ack-poll', 'cfg-hec-channel', 'cfg-hec-verify-ssl', 'cfg-hec-retry-enabled', 'cfg-hec-use-ack', 'cfg-hec-max-attempts', 'cfg-hec-base-delay-ms', 'cfg-hec-jitter-pct', 'cfg-hec-backoff'],
    },
    {
      anchor: 'cfg-metrics-enabled',
      fields: ['cfg-metrics-ssl-protocol', 'cfg-metrics-sourcetype', 'cfg-metrics-event-name', 'cfg-metrics-batch-size', 'cfg-metrics-max-buffer-events', 'cfg-metrics-max-buffer-bytes', 'cfg-metrics-ack-timeout', 'cfg-metrics-ack-poll', 'cfg-metrics-channel', 'cfg-metrics-verify-ssl', 'cfg-metrics-compat-mode', 'cfg-metrics-use-metrics-index', 'cfg-metrics-use-ack'],
    },
    {
      anchor: 'cfg-delivery-spool-path',
      fields: ['cfg-delivery-spool-path', 'cfg-delivery-max-bytes', 'cfg-delivery-max-envelopes', 'cfg-delivery-drain-max'],
    },
  ];

  groups.forEach((group) => {
    const anchor = document.getElementById(group.anchor);
    const card = anchor?.closest('.settings-card');
    if (!card || card.dataset.advancedReady === 'true') {
      return;
    }
    const targets = group.fields
      .map((id) => document.getElementById(id)?.closest('.field-group, .checkbox-row'))
      .filter(Boolean);
    if (targets.length === 0) {
      return;
    }
    targets.forEach((target) => target.classList.add('advanced-setting-collapsed'));
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'secondary-button advanced-toggle';
    button.textContent = 'Show Advanced Settings';
    button.setAttribute('aria-expanded', 'false');
    button.addEventListener('click', () => {
      const expanded = button.getAttribute('aria-expanded') === 'true';
      targets.forEach((target) => target.classList.toggle('advanced-setting-collapsed', expanded));
      button.setAttribute('aria-expanded', expanded ? 'false' : 'true');
      button.textContent = expanded ? 'Show Advanced Settings' : 'Hide Advanced Settings';
    });
    card.appendChild(button);
    card.dataset.advancedReady = 'true';
  });
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
      const mode = effectiveDeviceMode(endpoint);
      if (state.filter === 'legacy_dev' && mode !== 'legacy_dev') {
        return false;
      }
      if (state.filter === 'production' && mode !== 'production') {
        return false;
      }
      if (state.filter === 'maintenance' && mode !== 'maintenance') {
        return false;
      }
      if (state.filter === 'alerting_disabled' && endpoint.alerting_enabled !== false) {
        return false;
      }
      if (!search) {
        return true;
      }
      return [
        endpoint.ip,
        endpoint.hostname,
        endpoint.fqdn,
        endpoint.group,
        endpoint.description,
        endpoint.entitytype,
        endpoint.device,
        endpoint.vendor,
        endpoint.asset_id,
        endpoint.alerting_reason,
        endpoint.additional_notes,
      ].some((value) => String(value || '').toLowerCase().includes(search));
    });
}

function indexedDiscoveryItems() {
  syncDiscoverySelectionState();
  return globalThis.PingMonitorDiscoveryReview.indexedByReview(state.discovery.items, state.discovery.reviewFilter);
}

function getTableState(tableKind) {
  return state.tables[tableKind];
}

function getSortValue(endpoint, sortKey) {
  if (sortKey === 'device_mode') {
    return endpoint.discovery_review_state ? effectiveDiscoveryDeviceMode(endpoint) : effectiveDeviceMode(endpoint);
  }
  if (sortKey === 'discovery_review_state') {
    return effectiveDiscoveryReviewState(endpoint);
  }
  if (sortKey === 'dynamic_address') {
    return endpoint.dynamic_address ? 1 : 0;
  }
  if (sortKey === 'alerting_enabled' || sortKey === 'monitoring_enabled') {
    return endpoint[sortKey] === false ? 0 : 1;
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
    const isActive = (button.dataset.filter || 'all') === state.filter;
    button.classList.toggle('active', isActive);
    button.setAttribute('aria-selected', isActive ? 'true' : 'false');
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
  return Array.from(state.selectedEndpointIndices).sort((left, right) => left - right);
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
      const error = new Error(payload.error || `Request failed for ${path}`);
      error.status = response.status;
      error.payload = payload;
      throw error;
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
    const isActive = link === resolvedLink;
    link.classList.toggle('active', isActive);
    if (isActive) {
      link.setAttribute('aria-current', 'page');
    } else {
      link.removeAttribute('aria-current');
    }
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
  elements.endpointFields.fqdn.value = current.fqdn || '';
  elements.endpointFields.group.value = current.group || 'default';
  elements.endpointFields.description.value = current.description || '';
  elements.endpointFields.entitytype.value = current.entitytype || '';
  elements.endpointFields.device.value = current.device || '';
  elements.endpointFields.vendor.value = current.vendor || '';
  elements.endpointFields.asset_id.value = current.asset_id || '';
  elements.endpointFields.device_mode.value = effectiveDeviceMode(current);
  elements.endpointFields.additional_notes.value = current.additional_notes || '';
  elements.endpointFields.alerting_enabled.checked = current.alerting_enabled !== false;
  elements.endpointFields.alerting_reason.value = current.alerting_reason || '';
  elements.endpointFields.dynamic_address.checked = Boolean(current.dynamic_address);
  elements.endpointFields.classification_source.value = current.classification_source || '';
  elements.endpointFields.monitoring_enabled.checked = current.monitoring_enabled !== false;
  elements.endpointFields.maintenance_until.value = current.maintenance_until || '';
  elements.endpointFields.maintenance_reason.value = current.maintenance_reason || '';
}

function readEndpointForm() {
  const current = state.endpoints[state.selectedEndpointIndex] || {};
  const deviceMode = readTextValue(elements.endpointFields.device_mode) || 'production';
  return normalizeEndpoint({
    ...current,
    endpoint_id: current.endpoint_id,
    ip: readTextValue(elements.endpointFields.ip),
    hostname: readTextValue(elements.endpointFields.hostname),
    fqdn: readTextValue(elements.endpointFields.fqdn),
    group: readTextValue(elements.endpointFields.group),
    description: readTextValue(elements.endpointFields.description),
    entitytype: readTextValue(elements.endpointFields.entitytype),
    device: readTextValue(elements.endpointFields.device),
    vendor: readTextValue(elements.endpointFields.vendor),
    asset_id: readTextValue(elements.endpointFields.asset_id),
    additional_notes: readTextValue(elements.endpointFields.additional_notes),
    device_mode: deviceMode,
    dev: deviceMode === 'legacy_dev',
    alerting_enabled: elements.endpointFields.alerting_enabled.checked,
    alerting_reason: readTextValue(elements.endpointFields.alerting_reason),
    dynamic_address: elements.endpointFields.dynamic_address.checked,
    classification_source: readTextValue(elements.endpointFields.classification_source),
    monitoring_enabled: elements.endpointFields.monitoring_enabled.checked,
    maintenance_until: readTextValue(elements.endpointFields.maintenance_until),
    maintenance_reason: readTextValue(elements.endpointFields.maintenance_reason),
  });
}

function updateCurrentEndpointFromForm(renderTable = true) {
  if (state.selectedEndpointIndex < 0 || state.selectedEndpointIndex >= state.endpoints.length) {
    return;
  }
  state.endpoints[state.selectedEndpointIndex] = readEndpointForm();
  state.endpointsDirty = true;
  renderSummary();
  if (renderTable) {
    renderEndpointTable();
  }
  renderEndpointButtons();
}

function renderStatus() {
  if (!state.status) {
    return;
  }
  const formatLabel = String(state.status.config_format || '').toUpperCase();
  const runtime = state.status.runtime || {};
  const delivery = state.status.delivery || {};
  const deliveryState = delivery.state || 'not_started';
  const runtimeState = runtime.state || 'unknown';
  const restartRequired = Boolean(state.status.config_restart_required);
  const reloadPending = Boolean(state.status.endpoints_reload_pending);

  elements.versionText.textContent = `${state.status.version} deployment UI`;
  elements.modePill.textContent = restartRequired ? 'Restart required' : titleCase(runtimeState);
  elements.modePill.dataset.tone = restartRequired || runtimeState === 'failed' ? 'warning' : 'healthy';
  const showRestart = restartRequired && runtime.mode === 'monitor';
  elements.restartCollectorButton.classList.toggle('hidden', !showRestart);
  elements.restartCollectorButton.disabled = state.runtimeRestartBusy || state.configDirty || Boolean(runtime.restarting);
  elements.restartCollectorButton.textContent = state.runtimeRestartBusy || runtime.restarting ? 'Restarting...' : 'Restart Collector';
  elements.configPathChip.textContent = state.status.config_path;
  elements.endpointPath.textContent = state.status.endpoints_path;
  elements.sidebarConfigSource.textContent = `Config: ${formatLabel}`;
  elements.sidebarDiscoveryStatus.textContent = restartRequired
    ? 'Config restart required'
    : (state.status.discovery_available ? 'Discovery ready' : 'Discovery unavailable');

  elements.runtimeState.textContent = titleCase(runtimeState);
  elements.runtimeUptime.textContent = runtime.mode === 'ui_only'
    ? 'Configuration-only mode'
    : `Uptime ${formatDuration(runtime.uptime_seconds)}`;

  if (runtime.cycle_running) {
    elements.runtimeCycle.textContent = `Cycle ${runtime.current_cycle}`;
    elements.runtimeCycleNote.textContent = `In progress since ${formatTimestamp(runtime.current_cycle_started_at)}.`;
  } else if (runtime.last_cycle_completed_at) {
    elements.runtimeCycle.textContent = `Cycle ${runtime.current_cycle}`;
    elements.runtimeCycleNote.textContent = `${runtime.last_production_success || 0} healthy, ${runtime.last_production_partial || 0} partial, ${runtime.last_production_failed || 0} failed · ${runtime.last_cycle_duration_ms || 0} ms.`;
  } else {
    elements.runtimeCycle.textContent = runtime.mode === 'ui_only' ? 'Not running' : 'Starting';
    elements.runtimeCycleNote.textContent = runtime.mode === 'ui_only' ? 'The monitoring engine is disabled in UI-only mode.' : 'Waiting for the first completed cycle.';
  }

  elements.runtimeDelivery.textContent = titleCase(deliveryState);
  elements.runtimeDeliveryNote.textContent = delivery.last_success_at
    ? `Last success ${formatTimestamp(delivery.last_success_at)}.`
    : (delivery.last_error || 'No completed delivery recorded yet.');
  elements.runtimePending.textContent = String(delivery.pending_envelopes || 0);
  elements.runtimePendingNote.textContent = `${delivery.pending_bytes || 0} bytes waiting in the durable outbox.`;

  elements.configSourceLabel.textContent = restartRequired ? 'Restart Required' : 'Active Configuration';
  elements.configSourceCopy.textContent = restartRequired
    ? `${formatLabel} changes are saved on disk but are not active in this collector.`
    : `${formatLabel} on disk matches the running collector.`;
  if (runtime.last_endpoint_reload_error) {
    elements.endpointReloadLabel.textContent = 'Reload Failed';
    elements.endpointReloadCopy.textContent = runtime.last_endpoint_reload_error;
  } else if (reloadPending) {
    elements.endpointReloadLabel.textContent = 'Pending Reload';
    elements.endpointReloadCopy.textContent = 'The endpoint file changed and will be applied between monitoring cycles.';
  } else {
    elements.endpointReloadLabel.textContent = 'In Sync';
    elements.endpointReloadCopy.textContent = runtime.last_endpoint_reload_at
      ? `Last applied ${formatTimestamp(runtime.last_endpoint_reload_at)}.`
      : `${runtime.active_endpoints || state.endpoints.length || 0} endpoints are active.`;
  }

  elements.discoveryStatusLabel.textContent = state.status.discovery_available ? 'Discovery Ready' : 'Discovery Unavailable';
  elements.discoveryStatusCopy.textContent = state.status.discovery_available
    ? (state.status.discovery_script_path || 'Using companion discovery workflow.')
    : 'Discovery needs the companion workflow available in this deployment.';
  elements.discoveryAvailability.textContent = state.status.discovery_available ? 'Discovery Available' : 'Discovery Not Available';
  elements.settingsSourceChip.textContent = `${formatLabel} · ${state.status.config_path}`;

  if (runtime.last_restart_error) {
    setMessage(elements.runtimeBanner, 'error', `Collector restart failed; the last known-good configuration resumed. ${runtime.last_restart_error}`);
  } else if (runtime.fatal_error) {
    setMessage(elements.runtimeBanner, 'error', `Collector failed: ${runtime.fatal_error}`);
  } else if (runtime.last_endpoint_reload_error) {
    setMessage(elements.runtimeBanner, 'error', `Endpoint reload failed; the collector is using its last known-good set. ${runtime.last_endpoint_reload_error}`);
  } else if (restartRequired) {
    setMessage(elements.runtimeBanner, 'warning', 'Configuration is saved on disk but is not active. Restart the collector to apply engine settings.');
  } else if (reloadPending) {
    setMessage(elements.runtimeBanner, 'warning', 'Endpoint changes are saved and waiting for the next between-cycle hot reload.');
  } else if (deliveryState === 'impaired' || deliveryState === 'blocked') {
    setMessage(elements.runtimeBanner, 'error', `Splunk delivery is ${deliveryState}. ${delivery.last_error || 'Review the output configuration and durable outbox.'}`);
  } else {
    setMessage(elements.runtimeBanner, '', '');
  }
}

function renderSummary() {
  const groups = new Set(state.endpoints.map((endpoint) => (endpoint.group || 'default').trim() || 'default'));
  const productionCount = state.endpoints.filter((endpoint) => effectiveDeviceMode(endpoint) === 'production').length;
  const maintenanceCount = state.endpoints.filter((endpoint) => effectiveDeviceMode(endpoint) === 'maintenance').length;
  elements.summaryTotal.textContent = String(state.endpoints.length);
  elements.summaryProduction.textContent = String(productionCount);
  elements.summaryDev.textContent = String(maintenanceCount);
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
    elements.endpointRows.innerHTML = '<tr><td colspan="11" class="empty-cell">No endpoints match the current filter.</td></tr>';
    return;
  }

  elements.endpointRows.innerHTML = view.rows.map(({ endpoint, index }) => {
    const mode = effectiveDeviceMode(endpoint);
    const modeLabel = mode === 'legacy_dev' ? 'Legacy Dev' : titleCase(mode);
    const rowClasses = [
      index === state.selectedEndpointIndex ? 'selected-row' : '',
      state.selectedEndpointIndices.has(index) ? 'checked-row' : '',
    ].filter(Boolean).join(' ');
    const label = endpoint.ip || endpoint.hostname || `endpoint ${index + 1}`;
    return `
      <tr class="${rowClasses}" data-index="${index}" tabindex="0" aria-selected="${index === state.selectedEndpointIndex ? 'true' : 'false'}">
        <td class="table-select-col"><input class="table-row-checkbox" type="checkbox" data-index="${index}" ${state.selectedEndpointIndices.has(index) ? 'checked' : ''} aria-label="Select ${escapeHtml(label)}"></td>
        <td>${escapeHtml(endpoint.ip)}</td>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.fqdn || '-')}</td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.entitytype || '-')}</td>
        <td>${escapeHtml(endpoint.device || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
        <td><span class="mode-badge ${mode === 'production' ? 'production' : 'dev'}">${escapeHtml(modeLabel)}</span></td>
        <td><span class="mode-badge ${endpoint.alerting_enabled === false ? 'dev' : 'production'}">${endpoint.alerting_enabled === false ? 'Disabled' : 'Enabled'}</span></td>
        <td><span class="mode-badge ${endpoint.monitoring_enabled === false || mode === 'maintenance' ? 'dev' : 'production'}">${endpoint.monitoring_enabled === false ? 'Paused' : (mode === 'maintenance' || (endpoint.maintenance_until && Date.parse(endpoint.maintenance_until) > Date.now()) ? 'Maintenance' : 'Active')}</span></td>
      </tr>
    `;
  }).join('');
}

function renderEndpointButtons() {
  const dirty = endpointsAreDirty();
  const validation = validateEndpointDraft();
  const hasEditorSelection = state.selectedEndpointIndex >= 0;
  const actionIndices = getEndpointActionIndices();
  const selectedCount = state.selectedEndpointIndices.size;
  const filteredCount = filterEndpoints().length;

  elements.endpointDirtyPill.textContent = dirty ? 'Unsaved changes' : 'In sync';
  elements.endpointDirtyPill.classList.toggle('is-dirty', dirty);
  elements.saveEndpointsButton.disabled = !dirty || Boolean(validation);
  elements.resetEndpointsButton.disabled = !dirty;
  elements.selectAllEndpointsButton.disabled = filteredCount === 0;
  elements.deselectAllEndpointsButton.disabled = selectedCount === 0;
  elements.markSelectedMaintenanceButton.disabled = actionIndices.length === 0;
  elements.markSelectedProductionButton.disabled = actionIndices.length === 0;
  elements.disableSelectedAlertingButton.disabled = actionIndices.length === 0;
  elements.enableSelectedAlertingButton.disabled = actionIndices.length === 0;
  elements.pauseSelectedButton.disabled = actionIndices.length === 0;
  elements.resumeSelectedButton.disabled = actionIndices.length === 0;
  elements.deleteEndpointButton.disabled = actionIndices.length === 0;
  elements.deleteCurrentEndpointButton.disabled = !hasEditorSelection;
  elements.previewEndpointClassificationButton.disabled = !hasEditorSelection;
  elements.applyEndpointClassificationButton.disabled = !hasEditorSelection;
  Object.values(elements.endpointFields).forEach((field) => field.removeAttribute('aria-invalid'));
  if (validation) {
    setMessage(elements.endpointValidation, 'error', validation.message);
    if (validation.index === state.selectedEndpointIndex && elements.endpointFields[validation.field]) {
      elements.endpointFields[validation.field].setAttribute('aria-invalid', 'true');
    }
  } else {
    setMessage(elements.endpointValidation, '', '');
  }

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

function buildDiscoverySummaryText() {
  if (!state.discovery.available) {
    return 'Discovery is unavailable in this deployment.';
  }
  if (state.discovery.running) {
    return state.discovery.progressSummary || 'Discovery is running.';
  }
  if (state.discovery.summary) {
    const delta = state.discovery.delta
      ? ` New ${state.discovery.delta.new}, missing ${state.discovery.delta.missing}, unchanged ${state.discovery.delta.unchanged}, unresolved dynamic ${state.discovery.delta.unresolved_dynamic || 0}.`
      : '';
    const production = state.discovery.items.filter((endpoint) => effectiveDiscoveryDeviceMode(endpoint) === 'production').length;
    const maintenance = state.discovery.items.filter((endpoint) => effectiveDiscoveryDeviceMode(endpoint) === 'maintenance').length;
    const legacyDev = state.discovery.items.filter((endpoint) => effectiveDiscoveryDeviceMode(endpoint) === 'legacy_dev').length;
    const unassigned = state.discovery.items.filter((endpoint) => effectiveDiscoveryDeviceMode(endpoint) === 'unassigned').length;
    const needsReview = state.discovery.items.filter((endpoint) => effectiveDiscoveryReviewState(endpoint) === 'needs_review').length;
    return `${state.discovery.summary.total} endpoints found in ${state.discovery.durationMs} ms. Needs review ${needsReview}, unassigned mode ${unassigned}, production ${production}, maintenance ${maintenance}, legacy dev ${legacyDev}, groups ${state.discovery.summary.groups}.${delta}`;
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
  elements.discoveryReviewFilterButtons.forEach((button) => {
    const active = button.dataset.discoveryReviewFilter === state.discovery.reviewFilter;
    button.classList.toggle('active', active);
    button.setAttribute('aria-selected', String(active));
  });
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
    ? `Showing ${view.startItem}-${view.endItem} of ${view.totalItems} ${titleCase(state.discovery.reviewFilter)} result${view.totalItems === 1 ? '' : 's'} · ${state.discovery.items.length} total`
    : (state.discovery.running ? 'Discovery is running. Results will populate when the staged file is written.' : '0 results');
  elements.discoveryPageStatus.textContent = formatPageStatus(view);
  elements.discoveryPrevPageButton.disabled = view.page <= 1 || state.discovery.running;
  elements.discoveryNextPageButton.disabled = view.page >= view.totalPages || state.discovery.running;
  elements.runDiscoveryButton.disabled = !state.discovery.available || state.discovery.running;
  elements.runDiscoveryButton.textContent = state.discovery.running ? 'Running Discovery...' : 'Run Discovery';
  elements.cancelDiscoveryButton.disabled = !state.discovery.running;
  elements.selectAllDiscoveryButton.disabled = view.totalItems === 0 || state.discovery.running || state.discovery.reviewBusy;
  elements.deselectAllDiscoveryButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryMaintenanceButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryProductionButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.disableDiscoveryAlertingButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.enableDiscoveryAlertingButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryDynamicButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.markDiscoveryStaticButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.deferDiscoverySelectedButton.disabled = selectedCount === 0 || state.discovery.running || state.discovery.reviewBusy;
  elements.ignoreDiscoverySelectedButton.disabled = selectedCount === 0 || state.discovery.running || state.discovery.reviewBusy;
  elements.resetDiscoveryReviewButton.disabled = selectedCount === 0 || state.discovery.running || state.discovery.reviewBusy;
  elements.applyDiscoveryBulkFieldsButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.previewDiscoveryClassificationButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.applyDiscoveryClassificationButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.exportDiscoveryAllButton.disabled = !hasResults || state.discovery.running;
  elements.exportDiscoverySelectedButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.addDiscoverySelectedButton.disabled = selectedCount === 0 || state.discovery.running;
  elements.discoveryMergeMode.disabled = !hasResults || state.discovery.running;
  renderDiscoveryPreflight();

  if (view.totalItems === 0) {
    const emptyMessage = state.discovery.running
      ? 'Discovery is running. Results will appear here when the staged output file is ready.'
      : (hasResults
        ? `No discovery results are currently ${titleCase(state.discovery.reviewFilter)}.`
        : (state.discovery.available
          ? 'Discovery results will appear here after a run.'
          : 'Discovery is unavailable because the companion workflow is not present in this deployment.'));
    elements.discoveryRows.innerHTML = `<tr><td colspan="15" class="empty-cell">${emptyMessage}</td></tr>`;
    return;
  }

  elements.discoveryRows.innerHTML = view.rows.map(({ endpoint, index }) => {
    const label = endpoint.ip || endpoint.hostname || `discovered endpoint ${index + 1}`;
    const mode = effectiveDiscoveryDeviceMode(endpoint);
    const modeLabel = mode === 'legacy_dev' ? 'Legacy Dev' : titleCase(mode);
    const reviewState = effectiveDiscoveryReviewState(endpoint);
    const reviewClass = reviewState === 'approved' ? 'production' : (reviewState === 'needs_review' || reviewState === 'staged' ? 'warning' : 'dev');
    return `
      <tr class="${state.discovery.selectedIndices.has(index) ? 'checked-row' : ''}" data-index="${index}">
        <td class="table-select-col"><input class="table-row-checkbox" type="checkbox" data-index="${index}" ${state.discovery.selectedIndices.has(index) ? 'checked' : ''} aria-label="Select ${escapeHtml(label)}"></td>
        <td>${escapeHtml(endpoint.ip)}</td>
        <td>${escapeHtml(endpoint.hostname)}</td>
        <td>${escapeHtml(endpoint.fqdn || '-')}</td>
        <td><input class="text-input text-input-compact discovery-asset-input" type="text" data-index="${index}" value="${escapeHtml(endpoint.asset_id || '')}" placeholder="Optional" ${reviewState === 'approved' ? 'disabled' : ''} aria-label="Asset ID for ${escapeHtml(label)}"></td>
        <td><span class="mode-badge ${reviewClass}" title="${escapeHtml(endpoint.discovery_review_note || '')}">${escapeHtml(titleCase(reviewState))}</span></td>
        <td>${escapeHtml(endpoint.group || 'default')}</td>
        <td>${escapeHtml(endpoint.entitytype || '-')}</td>
        <td>${escapeHtml(endpoint.device || '-')}</td>
        <td>${escapeHtml(endpoint.vendor || '-')}</td>
        <td><span class="mode-badge ${mode === 'production' ? 'production' : 'dev'}">${escapeHtml(modeLabel)}</span></td>
        <td><span class="mode-badge ${endpoint.alerting_enabled === false ? 'dev' : 'production'}">${endpoint.alerting_enabled === false ? 'Disabled' : 'Enabled'}</span></td>
        <td>${endpoint.dynamic_address ? 'DHCP / Dynamic' : 'Static'}</td>
        <td>${escapeHtml(endpoint.dns_status || 'unresolved')}</td>
        <td>${endpoint.discovery_latency_ms == null ? '-' : `${escapeHtml(endpoint.discovery_latency_ms)} ms`}</td>
      </tr>
    `;
  }).join('');
}

function summarizeDiscoveryItems(items) {
  const groups = new Set();
  let dev = 0;
  items.forEach((endpoint) => {
    if (endpoint.dev) {
      dev += 1;
    }
    groups.add(String(endpoint.group || 'default').trim() || 'default');
  });
  return {
    total: items.length,
    production: items.length - dev,
    dev,
    groups: groups.size,
  };
}

function renderDiscoveryOperations() {
  const history = state.discovery.history;
  if (!elements.discoveryHistoryRows || !elements.discoveryScheduleStatuses) {
    return;
  }
  elements.refreshDiscoveryHistoryButton.disabled = history.loading;
  elements.refreshDiscoveryHistoryButton.textContent = history.loading ? 'Refreshing...' : 'Refresh History';

  if (history.schedules.length === 0) {
    elements.discoveryScheduleStatuses.innerHTML = `
      <article class="panel info-card discovery-schedule-card" data-state="disabled">
        <p class="summary-label">Weekly Discovery</p>
        <p class="info-value">Not configured</p>
        <p class="summary-note">Create and enable a schedule in Configuration when recurring discovery is required.</p>
      </article>
    `;
  } else {
    elements.discoveryScheduleStatuses.innerHTML = history.schedules.map((schedule) => {
      const stateName = schedule.last_error ? 'error' : (schedule.due ? 'due' : (schedule.enabled ? 'scheduled' : 'disabled'));
      const headline = schedule.last_error
        ? 'Attention required'
        : (schedule.enabled ? (schedule.due ? 'Due / catch-up pending' : 'Scheduled') : 'Disabled');
      const nextRun = schedule.enabled ? formatTimestamp(schedule.next_run_at, 'Schedule unavailable') : 'Disabled';
      const lastRun = formatTimestamp(schedule.last_run_at, 'Never completed');
      const targetCount = (schedule.targets || []).length;
      return `
        <article class="panel info-card discovery-schedule-card" data-state="${escapeHtml(stateName)}">
          <p class="summary-label">${escapeHtml(schedule.id || 'Unnamed schedule')}</p>
          <p class="info-value">${escapeHtml(headline)}</p>
          <p class="summary-note">${pluralize(targetCount, 'target')} · ${escapeHtml(schedule.day || '')} ${escapeHtml(schedule.time || '')} ${escapeHtml(schedule.timezone || '')}</p>
          <p class="summary-note">Last success: ${escapeHtml(lastRun)} · Next/due: ${escapeHtml(nextRun)}</p>
          ${schedule.last_error ? `<p class="summary-note">Last error: ${escapeHtml(schedule.last_error)}</p>` : ''}
        </article>
      `;
    }).join('');
  }

  const shown = history.scans.length;
  elements.discoveryHistoryStatus.textContent = history.loading
    ? 'Loading discovery history...'
    : `${history.totalScans} retained scan${history.totalScans === 1 ? '' : 's'}${shown < history.totalScans ? ` · showing latest ${shown}` : ''}`;
  const retention = [];
  if (history.retentionScans > 0) {
    retention.push(`latest ${history.retentionScans} scans`);
  }
  if (history.retentionDays > 0) {
    retention.push(`${history.retentionDays} days`);
  }
  elements.discoveryRetentionStatus.textContent = retention.length > 0
    ? `Retention: ${retention.join(' or ')}`
    : 'Retention: unbounded';

  if (history.scans.length === 0) {
    elements.discoveryHistoryRows.innerHTML = `
      <tr><td colspan="8" class="empty-cell">${history.loading ? 'Discovery history is loading.' : 'No completed discovery scans are retained yet.'}</td></tr>
    `;
    return;
  }
  elements.discoveryHistoryRows.innerHTML = history.scans.map((scan) => {
    const scanID = escapeHtml(scan.scan_id || '');
    const newCount = Number(scan.delta?.new || 0);
    const missingCount = Number(scan.delta?.missing || 0);
    const source = scan.schedule_id ? `Schedule: ${scan.schedule_id}` : 'Manual';
    const duration = Number(scan.duration_ms || 0);
    return `
      <tr>
        <td>${escapeHtml(formatTimestamp(scan.generated_at))}</td>
        <td>${escapeHtml(scan.target || '-')}</td>
        <td>${escapeHtml(source)}</td>
        <td>${escapeHtml(scan.summary?.total ?? 0)}</td>
        <td>${escapeHtml(newCount)}</td>
        <td>${escapeHtml(missingCount)}</td>
        <td>${duration > 0 ? `${escapeHtml((duration / 1000).toFixed(1))}s` : '-'}</td>
        <td>
          <div class="discovery-history-actions">
            <button class="secondary-button" type="button" data-history-scan-id="${scanID}" data-history-mode="all">All</button>
            <button class="secondary-button" type="button" data-history-scan-id="${scanID}" data-history-mode="new" ${newCount === 0 ? 'disabled' : ''}>New</button>
            <button class="secondary-button" type="button" data-history-scan-id="${scanID}" data-history-mode="missing" ${missingCount === 0 ? 'disabled' : ''}>Missing</button>
          </div>
        </td>
      </tr>
    `;
  }).join('');
}

async function loadDiscoveryHistory(showSuccess = false) {
  state.discovery.history.loading = true;
  renderDiscoveryOperations();
  try {
    const payload = await fetchJson('/api/discovery/history?limit=100');
    state.discovery.history.scans = payload.scans || [];
    state.discovery.history.schedules = payload.schedules || [];
    state.discovery.history.totalScans = Number(payload.total_scans || 0);
    state.discovery.history.retentionScans = Number(payload.retention_scans || 0);
    state.discovery.history.retentionDays = Number(payload.retention_days || 0);
    state.discovery.history.historyPath = payload.history_path || '';
    if (showSuccess) {
      setMessage(elements.discoveryBanner, 'success', 'Discovery schedule state and retained scan history refreshed.');
    }
  } catch (error) {
    setMessage(elements.discoveryBanner, 'error', error instanceof Error ? error.message : 'Unable to load discovery history.');
  } finally {
    state.discovery.history.loading = false;
    renderDiscoveryOperations();
  }
}

async function loadDiscoveryHistoryScan(scanID, mode) {
  try {
    const detail = await fetchJson(`/api/discovery/history?scan_id=${encodeURIComponent(scanID)}`);
    if ((mode === 'new' || mode === 'missing') && !detail.baseline_available) {
      throw new Error('The prior scan needed to calculate this delta is no longer retained.');
    }
    const sourceItems = mode === 'new'
      ? (detail.new_items || [])
      : (mode === 'missing' ? (detail.missing_items || []) : (detail.items || []));
    state.discovery.items = normalizeDiscoveryEndpoints(sourceItems);
    state.discovery.summary = summarizeDiscoveryItems(state.discovery.items);
    state.discovery.delta = mode === 'all' ? (detail.delta || null) : null;
    state.discovery.logs = mode === 'missing'
      ? 'These addresses were observed in the prior scan but not in the selected scan. Absence is not proof of downtime or decommissioning.'
      : `Loaded ${mode === 'new' ? 'newly observed addresses from' : 'all results from'} retained scan ${detail.scan_id}.`;
    state.discovery.durationMs = Number(detail.duration_ms || 0);
    state.discovery.generatedAt = detail.generated_at || '';
    state.discovery.runState = `History: ${titleCase(mode)}`;
    state.discovery.progressSummary = `Loaded ${state.discovery.items.length} ${mode} result${state.discovery.items.length === 1 ? '' : 's'} from ${detail.target}.`;
    state.discovery.selectedIndices.clear();
    setTablePage('discovery', 1);
    renderDiscovery();
    setMessage(
      elements.discoveryBanner,
      mode === 'missing' ? 'warning' : 'success',
      `${state.discovery.progressSummary} Review, export, or explicitly add selected results to the endpoint draft.`,
    );
  } catch (error) {
    setMessage(elements.discoveryBanner, 'error', error instanceof Error ? error.message : 'Unable to load the retained discovery scan.');
  }
}

function selectOption(value, current, label) {
  return `<option value="${escapeHtml(value)}" ${value === current ? 'selected' : ''}>${escapeHtml(label)}</option>`;
}

function renderDiscoverySubnetEditor() {
  if (!elements.discoverySubnetRows) {
    return;
  }
  if (state.discoverySubnets.length === 0) {
    elements.discoverySubnetRows.innerHTML = '<p class="empty-copy">No subnet metadata pairs configured. Scheduled targets remain valid without a catalog.</p>';
    return;
  }
  elements.discoverySubnetRows.innerHTML = state.discoverySubnets.map((subnet, index) => `
    <article class="repeatable-row discovery-subnet-row" data-index="${index}">
      <div class="repeatable-row-header">
        <strong>Subnet pair ${index + 1}</strong>
        <button class="danger-button compact-action" type="button" data-remove-subnet="${index}">Remove</button>
      </div>
      <div class="field-grid">
        <label class="field-group"><span>ID</span><input class="text-input" data-subnet-field="id" value="${escapeHtml(subnet.id || '')}" placeholder="nyc-users"></label>
        <label class="field-group"><span>CIDR</span><input class="text-input" data-subnet-field="cidr" value="${escapeHtml(subnet.cidr || '')}" placeholder="10.20.30.0/24"></label>
        <label class="field-group"><span>Subnet Name</span><input class="text-input" data-subnet-field="name" value="${escapeHtml(subnet.name || '')}" placeholder="NYC User LAN"></label>
        <label class="field-group"><span>VLAN</span><input class="text-input" data-subnet-field="vlan" value="${escapeHtml(subnet.vlan || '')}" placeholder="230"></label>
        <label class="field-group"><span>Location</span><input class="text-input" data-subnet-field="location" value="${escapeHtml(subnet.location || '')}" placeholder="New York HQ"></label>
        <label class="field-group"><span>Routing Domain</span><input class="text-input" data-subnet-field="routing_domain" value="${escapeHtml(subnet.routing_domain || '')}" placeholder="corp"></label>
        <label class="field-group"><span>Addressing</span><select class="text-input" data-subnet-field="addressing_mode">${selectOption('static', subnet.addressing_mode || 'static', 'Static')}${selectOption('dhcp', subnet.addressing_mode || 'static', 'DHCP / Dynamic')}</select></label>
      </div>
    </article>
  `).join('');
}

function readDiscoverySubnetsFromEditor() {
  return Array.from(elements.discoverySubnetRows?.querySelectorAll('.discovery-subnet-row') || []).map((row) => {
    const value = (field) => readTextValue(row.querySelector(`[data-subnet-field="${field}"]`));
    return {
      id: value('id'),
      cidr: value('cidr'),
      name: value('name'),
      vlan: value('vlan'),
      location: value('location'),
      routing_domain: value('routing_domain'),
      addressing_mode: value('addressing_mode') || 'static',
    };
  });
}

function renderClassificationRuleEditor() {
  if (!elements.classificationRuleRows) {
    return;
  }
  if (state.classificationRules.length === 0) {
    elements.classificationRuleRows.innerHTML = '<p class="empty-copy">No naming pairs configured. Existing endpoint fields and discovery behavior remain unchanged.</p>';
    return;
  }
  elements.classificationRuleRows.innerHTML = state.classificationRules.map((rule, index) => {
    const assignments = rule.assignments || {};
    const pairNumber = index + 1;
    const titleID = `classification-rule-title-${index}`;
    return `
      <article class="repeatable-row classification-rule-row" data-index="${index}" aria-labelledby="${titleID}">
        <div class="repeatable-row-header">
          <strong id="${titleID}" data-rule-title>Pair ${pairNumber}: ${escapeHtml(rule.id || 'unnamed')}</strong>
          <div class="button-row compact-button-row">
            <button class="secondary-button compact-action" type="button" data-move-rule="up" data-index="${index}" aria-label="Move Pair ${pairNumber} up" ${index === 0 ? 'disabled' : ''}>Move Up</button>
            <button class="secondary-button compact-action" type="button" data-move-rule="down" data-index="${index}" aria-label="Move Pair ${pairNumber} down" ${index === state.classificationRules.length - 1 ? 'disabled' : ''}>Move Down</button>
            <button class="danger-button compact-action" type="button" data-remove-rule="${index}" aria-label="Remove Pair ${pairNumber}">Remove</button>
          </div>
        </div>
        <div class="field-grid">
          <label class="field-group"><span>Rule ID</span><input class="text-input" data-rule-field="id" value="${escapeHtml(rule.id || '')}" placeholder="site-network-vendor"></label>
          <label class="field-group"><span>Match Source</span><select class="text-input" data-rule-field="source">${selectOption('hostname', rule.source || 'either', 'Hostname')}${selectOption('fqdn', rule.source || 'either', 'FQDN')}${selectOption('either', rule.source || 'either', 'Hostname, then FQDN')}</select></label>
          <label class="field-group field-span-2"><span>Expected Naming Regex</span><input class="text-input code-input" data-rule-field="pattern" value="${escapeHtml(rule.pattern || '')}" placeholder="^(?P&lt;site&gt;[a-z]{3})-(?P&lt;role&gt;sw|fw)-(?P&lt;vendor&gt;[a-z]+)-\\d+$"></label>
          <label class="field-group"><span>Assign Group</span><input class="text-input code-input" data-assignment-field="group" value="${escapeHtml(assignments.group || '')}" placeholder="\${site}"></label>
          <label class="field-group"><span>Assign Entity Type</span><input class="text-input code-input" data-assignment-field="entitytype" value="${escapeHtml(assignments.entitytype || '')}" placeholder="network"></label>
          <label class="field-group"><span>Assign Device</span><input class="text-input code-input" data-assignment-field="device" value="${escapeHtml(assignments.device || '')}" placeholder="\${role}"></label>
          <label class="field-group"><span>Assign Vendor</span><input class="text-input code-input" data-assignment-field="vendor" value="${escapeHtml(assignments.vendor || '')}" placeholder="\${vendor}"></label>
        </div>
        <div class="checkbox-grid">
          <label class="checkbox-row"><input type="checkbox" data-rule-field="enabled" ${rule.enabled !== false ? 'checked' : ''}><span>Enabled</span></label>
          <label class="checkbox-row"><input type="checkbox" data-rule-field="overwrite" ${rule.overwrite ? 'checked' : ''}><span>Overwrite existing values</span></label>
          <label class="checkbox-row"><input type="checkbox" data-rule-field="stop_on_match" ${rule.stop_on_match ? 'checked' : ''}><span>Stop after this match</span></label>
        </div>
      </article>
    `;
  }).join('');
}

function readClassificationRulesFromEditor() {
  return Array.from(elements.classificationRuleRows?.querySelectorAll('.classification-rule-row') || []).map((row) => {
    const value = (field) => readTextValue(row.querySelector(`[data-rule-field="${field}"]`));
    const assignments = {};
    row.querySelectorAll('[data-assignment-field]').forEach((field) => {
      const assignment = readTextValue(field);
      if (assignment) {
        assignments[field.dataset.assignmentField] = assignment;
      }
    });
    return {
      id: value('id'),
      enabled: row.querySelector('[data-rule-field="enabled"]').checked,
      source: value('source') || 'either',
      pattern: value('pattern'),
      assignments,
      overwrite: row.querySelector('[data-rule-field="overwrite"]').checked,
      stop_on_match: row.querySelector('[data-rule-field="stop_on_match"]').checked,
    };
  });
}

async function previewClassificationSample() {
  const hostname = readTextValue(elements.classificationPreviewHostname);
  const fqdn = readTextValue(elements.classificationPreviewFQDN);
  if (!hostname && !fqdn) {
    elements.classificationPreviewResult.textContent = 'Enter a preview hostname or FQDN.';
    return;
  }
  try {
    const payload = await postJson('/api/classification/preview', {
      rules: readClassificationRulesFromEditor(),
      items: [{ hostname, fqdn, group: 'default' }],
    });
    const result = (payload.results || [])[0] || {};
    const endpoint = result.endpoint || {};
    elements.classificationPreviewResult.textContent = [
      `Matched rules: ${(result.matched_rules || []).join(', ') || 'none'}`,
      `Changes: ${Object.entries(result.changes || {}).map(([key, value]) => `${key}=${value}`).join(', ') || 'none'}`,
      `Result: group=${endpoint.group || 'default'}, entitytype=${endpoint.entitytype || '-'}, device=${endpoint.device || '-'}, vendor=${endpoint.vendor || '-'}`,
      `Provenance: ${endpoint.classification_source || 'none'}`,
    ].join('\n');
  } catch (error) {
    elements.classificationPreviewResult.textContent = error instanceof Error ? error.message : 'Unable to evaluate naming rules.';
  }
}

function loadConfigForm(cfg, secrets = {}) {
	state.config = deepClone(cfg || {});
	state.configSecrets = deepClone(secrets || {});
  const ping = cfg.ping || {};
  const diagnostics = cfg.diagnostics || {};
  const debug = cfg.debug || {};
  const hec = cfg.hec || {};
  const retry = hec.retry || {};
  const metrics = cfg.metrics || {};
  const delivery = cfg.delivery || {};
  const discovery = cfg.discovery || {};
  const primarySchedule = (discovery.schedules || [])[0] || {};
  state.discoverySubnets = deepClone(discovery.subnets || []);
  state.classificationRules = deepClone(cfg.classification?.rules || []);
  renderDiscoverySubnetEditor();
  renderClassificationRuleEditor();

  elements.settingsFields.pingsPerCycle.value = cfg.pings_per_cycle ?? '';
  elements.settingsFields.cycleInterval.value = cfg.cycle_interval_seconds ?? '';
  elements.settingsFields.timeoutMs.value = cfg.timeout_ms ?? '';
  elements.settingsFields.parallelThreads.value = cfg.parallel_threads ?? '';
  elements.settingsFields.emitIndividualPings.checked = Boolean(cfg.emit_individual_pings);
  elements.settingsFields.outputMode.value = cfg.output_mode || 'file';
  elements.settingsFields.logPath.value = cfg.log_path || '';
  elements.settingsFields.logRotation.value = cfg.log_rotation_size_mb ?? '';
  elements.settingsFields.logRetentionFiles.value = cfg.log_retention_files ?? 0;
  elements.settingsFields.logRetentionDays.value = cfg.log_retention_days ?? 0;
  elements.settingsFields.logCompressRotated.checked = Boolean(cfg.log_compress_rotated);
  elements.settingsFields.discoveryEnabled.checked = Boolean(primarySchedule.enabled);
  elements.settingsFields.discoveryID.value = primarySchedule.id || 'weekly-network-discovery';
  elements.settingsFields.discoveryHistoryPath.value = discovery.history_path || './data/discovery';
  elements.settingsFields.discoveryRetentionScans.value = discovery.retention_scans ?? 0;
  elements.settingsFields.discoveryRetentionDays.value = discovery.retention_days ?? 0;
  elements.settingsFields.discoveryTargets.value = (primarySchedule.targets || []).join('\n');
  elements.settingsFields.discoveryDay.value = primarySchedule.day || 'sunday';
  elements.settingsFields.discoveryTime.value = primarySchedule.time || '02:00';
  elements.settingsFields.discoveryTimezone.value = primarySchedule.timezone || 'Local';
  elements.settingsFields.discoveryScheduleTimeout.value = primarySchedule.timeout_ms ?? 500;
  elements.settingsFields.discoveryConcurrency.value = primarySchedule.concurrency ?? 25;
  elements.settingsFields.pingMode.value = ping.mode || 'auto';
  elements.settingsFields.diagnosticsEnabled.checked = Boolean(diagnostics.enabled);
  elements.settingsFields.handleProbeMode.value = diagnostics.handle_probe_mode || 'none';
  elements.settingsFields.emitMemoryStats.checked = Boolean(debug.emit_memory_stats);
  elements.settingsFields.hecEnabled.checked = Boolean(hec.enabled);
  elements.settingsFields.hecURL.value = hec.url || '';
  elements.settingsFields.hecToken.value = hec.token || '';
  elements.settingsFields.hecToken.placeholder = secrets.hec_token_configured ? 'Configured — enter to replace' : 'Enter HEC token';
  elements.settingsFields.hecIndex.value = hec.index || '';
  elements.settingsFields.hecSourcetype.value = hec.sourcetype || '';
  elements.settingsFields.hecVerifySSL.checked = Boolean(hec.verify_ssl);
  elements.settingsFields.hecSSLProtocol.value = hec.ssl_protocol || '';
  elements.settingsFields.hecBatchSize.value = hec.batch_size ?? '';
  elements.settingsFields.hecMaxBufferEvents.value = hec.max_buffer_events ?? '';
  elements.settingsFields.hecMaxBufferBytes.value = hec.max_buffer_bytes || '';
  elements.settingsFields.hecRetryEnabled.checked = Boolean(retry.enabled);
  elements.settingsFields.hecMaxAttempts.value = retry.max_attempts ?? '';
  elements.settingsFields.hecBaseDelayMs.value = retry.base_delay_ms ?? '';
  elements.settingsFields.hecJitterPct.value = retry.jitter_pct ?? '';
  elements.settingsFields.hecBackoff.value = retry.backoff || '';
  elements.settingsFields.hecRetryCount.value = hec.retry_count ?? '';
  elements.settingsFields.hecRetryDelayMs.value = hec.retry_delay_ms ?? '';
  elements.settingsFields.hecUseACK.checked = Boolean(hec.use_ack);
  elements.settingsFields.hecACKTimeout.value = hec.ack_timeout_seconds ?? 60;
  elements.settingsFields.hecACKPoll.value = hec.ack_poll_interval_ms ?? 1000;
  elements.settingsFields.hecChannel.value = hec.channel || '';
  elements.settingsFields.metricsEnabled.checked = Boolean(metrics.enabled);
  elements.settingsFields.metricsMode.value = metrics.mode || 'dual';
  elements.settingsFields.metricsIndex.value = metrics.index || '';
  elements.settingsFields.metricsHECURL.value = metrics.hec_url || '';
  elements.settingsFields.metricsToken.value = metrics.token || '';
  elements.settingsFields.metricsToken.placeholder = secrets.metrics_token_configured ? 'Configured — enter to replace' : 'Enter metrics token';
  elements.settingsFields.metricsVerifySSL.checked = Boolean(metrics.verify_ssl);
  elements.settingsFields.metricsSSLProtocol.value = metrics.ssl_protocol || '';
  elements.settingsFields.metricsCompatMode.checked = Boolean(metrics.compat_mode);
  elements.settingsFields.metricsSourcetype.value = metrics.sourcetype || '';
  elements.settingsFields.metricsEventName.value = metrics.event_name || '';
  elements.settingsFields.metricsUseMetricsIndex.checked = Boolean(metrics.use_metrics_index);
  elements.settingsFields.metricsBatchSize.value = metrics.batch_size ?? '';
  elements.settingsFields.metricsMaxBufferEvents.value = metrics.max_buffer_events ?? '';
  elements.settingsFields.metricsMaxBufferBytes.value = metrics.max_buffer_bytes || '';
  elements.settingsFields.metricsUseACK.checked = Boolean(metrics.use_ack);
  elements.settingsFields.metricsACKTimeout.value = metrics.ack_timeout_seconds ?? 60;
  elements.settingsFields.metricsACKPoll.value = metrics.ack_poll_interval_ms ?? 1000;
  elements.settingsFields.metricsChannel.value = metrics.channel || '';
  elements.settingsFields.deliverySpoolPath.value = delivery.spool_path || '';
  elements.settingsFields.deliveryMaxBytes.value = delivery.max_spool_bytes || '512MB';
  elements.settingsFields.deliveryMaxEnvelopes.value = delivery.max_envelopes ?? 10000;
  elements.settingsFields.deliveryDrainMax.value = delivery.drain_max_envelopes ?? 100;
}

function readConfigForm() {
	const preserved = deepClone(state.config || {});
  return {
	...preserved,
    pings_per_cycle: readNumberValue(elements.settingsFields.pingsPerCycle, 4),
    cycle_interval_seconds: readNumberValue(elements.settingsFields.cycleInterval, 60),
    timeout_ms: readNumberValue(elements.settingsFields.timeoutMs, 1000),
    parallel_threads: readNumberValue(elements.settingsFields.parallelThreads, 10),
    output_mode: readTextValue(elements.settingsFields.outputMode) || 'file',
    log_path: readTextValue(elements.settingsFields.logPath),
    log_rotation_size_mb: readNumberValue(elements.settingsFields.logRotation, 50),
    log_retention_files: readNumberValue(elements.settingsFields.logRetentionFiles, 0),
    log_retention_days: readNumberValue(elements.settingsFields.logRetentionDays, 0),
    log_compress_rotated: elements.settingsFields.logCompressRotated.checked,
    emit_individual_pings: elements.settingsFields.emitIndividualPings.checked,
    ping: {
      mode: readTextValue(elements.settingsFields.pingMode) || 'auto',
    },
	health: deepClone(preserved.health || {
	  down_after_failures: 3,
	  recovery_after_successes: 2,
	  stale_after_intervals: 2,
	}),
    diagnostics: {
      enabled: elements.settingsFields.diagnosticsEnabled.checked,
      handle_probe_mode: readTextValue(elements.settingsFields.handleProbeMode) || 'none',
    },
    debug: {
      emit_memory_stats: elements.settingsFields.emitMemoryStats.checked,
    },
    hec: {
	  ...(preserved.hec || {}),
      enabled: elements.settingsFields.hecEnabled.checked,
      url: readTextValue(elements.settingsFields.hecURL),
      token: readTextValue(elements.settingsFields.hecToken),
      index: readTextValue(elements.settingsFields.hecIndex),
      sourcetype: readTextValue(elements.settingsFields.hecSourcetype),
      verify_ssl: elements.settingsFields.hecVerifySSL.checked,
      ssl_protocol: readTextValue(elements.settingsFields.hecSSLProtocol) || 'Default',
      batch_size: readNumberValue(elements.settingsFields.hecBatchSize, 100),
      max_buffer_events: readNumberValue(elements.settingsFields.hecMaxBufferEvents, 5000),
      max_buffer_bytes: readTextValue(elements.settingsFields.hecMaxBufferBytes) || '5MB',
      retry: {
		...(preserved.hec?.retry || {}),
        enabled: elements.settingsFields.hecRetryEnabled.checked,
        max_attempts: readNumberValue(elements.settingsFields.hecMaxAttempts, 3),
        base_delay_ms: readNumberValue(elements.settingsFields.hecBaseDelayMs, 250),
        jitter_pct: readNumberValue(elements.settingsFields.hecJitterPct, 20),
        backoff: readTextValue(elements.settingsFields.hecBackoff) || 'exponential',
      },
      retry_count: readNumberValue(elements.settingsFields.hecRetryCount, 0),
      retry_delay_ms: readNumberValue(elements.settingsFields.hecRetryDelayMs, 250),
      use_ack: elements.settingsFields.hecUseACK.checked,
      ack_timeout_seconds: readNumberValue(elements.settingsFields.hecACKTimeout, 60),
      ack_poll_interval_ms: readNumberValue(elements.settingsFields.hecACKPoll, 1000),
      channel: readTextValue(elements.settingsFields.hecChannel),
    },
    metrics: {
	  ...(preserved.metrics || {}),
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
      use_ack: elements.settingsFields.metricsUseACK.checked,
      ack_timeout_seconds: readNumberValue(elements.settingsFields.metricsACKTimeout, 60),
      ack_poll_interval_ms: readNumberValue(elements.settingsFields.metricsACKPoll, 1000),
      channel: readTextValue(elements.settingsFields.metricsChannel),
    },
    delivery: {
	  ...(preserved.delivery || {}),
	  spool_path: readTextValue(elements.settingsFields.deliverySpoolPath) || './data/outbox',
	  max_spool_bytes: readTextValue(elements.settingsFields.deliveryMaxBytes) || '512MB',
	  max_envelopes: readNumberValue(elements.settingsFields.deliveryMaxEnvelopes, 10000),
	  drain_max_envelopes: readNumberValue(elements.settingsFields.deliveryDrainMax, 100),
	},
    discovery: {
      ...(preserved.discovery || {}),
      history_path: readTextValue(elements.settingsFields.discoveryHistoryPath) || './data/discovery',
      retention_scans: readNumberValue(elements.settingsFields.discoveryRetentionScans, 0),
      retention_days: readNumberValue(elements.settingsFields.discoveryRetentionDays, 0),
      subnets: readDiscoverySubnetsFromEditor(),
      schedules: [{
        ...((preserved.discovery?.schedules || [])[0] || {}),
        id: readTextValue(elements.settingsFields.discoveryID) || 'weekly-network-discovery',
        enabled: elements.settingsFields.discoveryEnabled.checked,
        targets: readTextValue(elements.settingsFields.discoveryTargets).split(/[\r\n,]+/).map((value) => value.trim()).filter(Boolean),
        frequency: 'weekly',
        day: readTextValue(elements.settingsFields.discoveryDay) || 'sunday',
        time: readTextValue(elements.settingsFields.discoveryTime) || '02:00',
        timezone: readTextValue(elements.settingsFields.discoveryTimezone) || 'Local',
        timeout_ms: readNumberValue(elements.settingsFields.discoveryScheduleTimeout, 500),
        concurrency: readNumberValue(elements.settingsFields.discoveryConcurrency, 25),
        import_policy: 'review',
      }, ...((preserved.discovery?.schedules || []).slice(1))],
    },
    classification: {
      ...(preserved.classification || {}),
      rules: readClassificationRulesFromEditor(),
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

function scheduleMarkup(plan) {
  if (!plan) {
    return '<p class="empty-copy">A schedule cannot be modeled until the config and at least one valid endpoint are available.</p>';
  }
  const fitLabel = plan.fits ? 'Fits worst case' : 'Does not fit';
  const fitClass = plan.fits ? 'advisor-fit' : 'advisor-blocked';
  return `
    <div class="advisor-schedule-status ${fitClass}">${escapeHtml(fitLabel)}</div>
    <dl class="advisor-stat-list">
      <div><dt>Endpoints</dt><dd>${escapeHtml(plan.endpoint_count)}</dd></div>
      <div><dt>Probe budget</dt><dd>${(Number(plan.probe_budget_ms || 0) / 1000).toFixed(2)}s</dd></div>
      <div><dt>Modeled cycle</dt><dd>${(Number(plan.worst_case_cycle_ms || 0) / 1000).toFixed(2)}s / ${escapeHtml(plan.interval_seconds)}s</dd></div>
      <div><dt>Workers</dt><dd>${escapeHtml(plan.workers)} current · ${escapeHtml(plan.required_workers)} minimum · ${escapeHtml(plan.recommended_workers)} recommended</dd></div>
      <div><dt>Headroom</dt><dd>${escapeHtml(plan.capacity_headroom_pct)}%</dd></div>
    </dl>`;
}

function renderAdvisor() {
  const report = state.advisor;
  const busy = state.advisorBusy;
  elements.advisorAnalyzeButton.disabled = busy;
  elements.advisorBenchmarkButton.disabled = busy;
  elements.advisorProfile.disabled = busy;
  if (!report) {
    elements.advisorApplySafeButton.disabled = true;
    elements.advisorApplyProfileButton.disabled = true;
    return;
  }

  const summary = report.summary || {};
  const inventory = report.inventory || {};
  elements.advisorBlockers.textContent = summary.blockers ?? 0;
  elements.advisorWarnings.textContent = summary.warnings ?? 0;
  elements.advisorSafeFixes.textContent = summary.safe_fixes ?? 0;
  elements.advisorReadiness.textContent = summary.ready_to_run ? 'Ready' : 'Blocked';
  elements.advisorReadinessNote.textContent = summary.ready_to_run
    ? 'No startup blockers were found in the files on disk.'
    : 'Resolve blockers, then analyze again before starting the service.';
  elements.advisorInventoryChip.textContent = `${inventory.rows || 0} rows · ${inventory.schedulable_endpoints || 0} unique schedulable`;
  elements.advisorCurrentSchedule.innerHTML = scheduleMarkup(report.schedule);

  const proposal = report.proposal;
  elements.advisorProposalTitle.textContent = proposal?.profile?.name || 'Proposed schedule';
  elements.advisorProposedSchedule.innerHTML = scheduleMarkup(proposal?.schedule);
  const findings = report.findings || [];
  elements.advisorFindings.innerHTML = findings.length
    ? findings.map((finding) => `
      <article class="advisor-finding severity-${escapeHtml(finding.severity)}">
        <div class="advisor-finding-heading">
          <span class="advisor-severity">${escapeHtml(finding.severity)}</span>
          <span class="advisor-code">${escapeHtml(finding.code)}</span>
        </div>
        <h4>${escapeHtml(finding.title)}</h4>
        <p>${escapeHtml(finding.message)}</p>
        ${(finding.evidence || []).length ? `<ul>${finding.evidence.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>` : ''}
        ${finding.recommendation ? `<p class="advisor-recommendation"><strong>Recommendation:</strong> ${escapeHtml(finding.recommendation)}</p>` : ''}
      </article>`).join('')
    : '<p class="empty-copy">No findings were returned.</p>';

  const changes = proposal?.changes || [];
  elements.advisorChanges.innerHTML = changes.length
    ? changes.map((change) => `
      <div class="advisor-change-row">
        <code>${escapeHtml(change.path)}</code>
        <span><del>${escapeHtml(change.before)}</del> → <strong>${escapeHtml(change.after)}</strong></span>
        <span>${escapeHtml(change.reason)}</span>
      </div>`).join('')
    : '<p class="empty-copy">The selected profile does not require configuration changes.</p>';

  const hasInventoryBlocker = findings.some((finding) => finding.category === 'inventory' && finding.severity === 'blocker');
  elements.advisorApplySafeButton.disabled = busy || Number(summary.safe_fixes || 0) === 0;
  elements.advisorApplyProfileButton.disabled = busy || !changes.length || hasInventoryBlocker;
}

async function loadAdvisorProfiles() {
  const profiles = await fetchJson('/api/advisor/profiles');
  state.advisorProfiles = Array.isArray(profiles) ? profiles : [];
  const selected = elements.advisorProfile.value || 'standard';
  elements.advisorProfile.innerHTML = state.advisorProfiles
    .map((profile) => `<option value="${escapeHtml(profile.id)}">${escapeHtml(profile.name)}</option>`)
    .join('');
  if (state.advisorProfiles.some((profile) => profile.id === selected)) {
    elements.advisorProfile.value = selected;
  }
}

async function loadAdvisor(showSuccess = false) {
  state.advisorBusy = true;
  renderAdvisor();
  try {
    const profile = elements.advisorProfile.value || 'standard';
    state.advisor = await fetchJson(`/api/advisor?profile=${encodeURIComponent(profile)}`);
    state.configRevision = state.advisor.config_revision || state.configRevision;
    state.endpointsRevision = state.advisor.endpoints_revision || state.endpointsRevision;
    renderAdvisor();
    if (showSuccess) {
      setMessage(elements.advisorBanner, state.advisor.summary?.ready_to_run ? 'success' : 'warning', 'Analysis refreshed from the deployment files on disk.');
    }
  } catch (error) {
    setMessage(elements.advisorBanner, 'error', error instanceof Error ? error.message : 'Advisor analysis failed.');
  } finally {
    state.advisorBusy = false;
    renderAdvisor();
  }
}

async function applyAdvisorFixes(kind) {
  if (!state.advisor || state.advisorBusy) {
    return;
  }
  if (hasUnsavedChanges() && !window.confirm('Applying advisor changes writes directly to disk. Discard the unsaved endpoint/config drafts and continue?')) {
    return;
  }
  const applySafe = kind === 'safe';
  const profileName = state.advisor.proposal?.profile?.name || elements.advisorProfile.value;
  const prompt = applySafe
    ? 'Apply only the listed safe inventory cleanup? A timestamped backup will be retained.'
    : `Apply the ${profileName} settings shown in the preview? The collector must be restarted afterward.`;
  if (!window.confirm(prompt)) {
    return;
  }
  state.advisorBusy = true;
  renderAdvisor();
  try {
    const result = await postJson('/api/advisor/apply', {
      profile: elements.advisorProfile.value || 'standard',
      apply_safe: applySafe,
      apply_profile: kind === 'profile',
      config_revision: state.advisor.config_revision,
      endpoints_revision: state.advisor.endpoints_revision,
    });
    state.advisor = result.report;
    const applied = (result.applied_fixes || []).join(', ') || 'No changes were needed';
    setMessage(elements.advisorBanner, 'success', `${applied}. ${result.restart_required ? 'Restart the collector to activate config changes.' : 'Inventory changes are ready for runtime reload.'}`);
    await reloadAllData(false);
  } catch (error) {
    setMessage(elements.advisorBanner, 'error', error instanceof Error ? error.message : 'Advisor changes could not be applied.');
  } finally {
    state.advisorBusy = false;
    renderAdvisor();
  }
}

async function runAdvisorBenchmark() {
  state.advisorBusy = true;
  renderAdvisor();
  setMessage(elements.advisorBanner, 'warning', 'Running a bounded local benchmark. No monitored devices will be pinged.');
  try {
    const profile = elements.advisorProfile.value || 'standard';
    state.advisorBenchmark = await postJson(`/api/advisor/benchmark?profile=${encodeURIComponent(profile)}`, {});
    const result = state.advisorBenchmark;
    elements.advisorBenchmarkPanel.classList.remove('hidden');
    elements.advisorBenchmarkResults.textContent = [
      `Completed: ${formatTimestamp(result.generated_at)}`,
      `Total duration: ${result.duration_ms} ms`,
      `Config + inventory analysis: ${result.config_inventory_ms} ms`,
      `Planner throughput: ${Math.round(result.planner_ops_per_second || 0).toLocaleString()} operations/second`,
      `Ping backend: ${result.ping_backend || 'unknown'} (${result.loopback_successes || 0}/${result.loopback_attempts || 0} loopback replies in ${result.loopback_duration_ms || 0} ms)`,
      ...(result.ping_fallback ? [`Backend fallback: ${result.ping_fallback}`] : []),
      `Deployment filesystem: ${Number(result.filesystem_mb_per_second || 0).toFixed(1)} MiB/second across ${result.filesystem_writes || 0} temporary writes`,
      `Runtime goroutines observed: ${result.goroutines || 0}`,
      ...(result.warnings || []).map((warning) => `Warning: ${warning}`),
    ].join('\n');
    setMessage(elements.advisorBanner, 'success', 'Benchmark complete. These host measurements are planning evidence, not device latency or SLA evidence.');
  } catch (error) {
    setMessage(elements.advisorBanner, 'error', error instanceof Error ? error.message : 'Benchmark failed.');
  } finally {
    state.advisorBusy = false;
    renderAdvisor();
  }
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
    state.endpoints = normalizeEndpoints(endpointsPayload.items || []);
    state.savedEndpoints = deepClone(state.endpoints);
    state.endpointsRevision = endpointsPayload.revision || '';
    state.endpointsDirty = false;
    state.selectedEndpointIndices.clear();
    loadConfigForm(configPayload.config || {}, configPayload.secrets || {});
    state.savedConfig = readConfigForm();
    state.configRevision = configPayload.revision || '';
    state.configDirty = false;
    state.discovery.available = Boolean(status.discovery_available);
    if (!state.discovery.running && state.discovery.items.length === 0 && !state.discovery.summary) {
      state.discovery.runState = 'Idle';
      state.discovery.progressSummary = state.discovery.available ? 'No discovery run yet.' : 'Discovery is unavailable in this deployment.';
    }
    ensureSelectedEndpoint();
    renderAll();
    await loadDiscoveryHistory(false);

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
    try {
      await loadAdvisorProfiles();
      await loadAdvisor(false);
    } catch (error) {
      setMessage(elements.advisorBanner, 'error', error instanceof Error ? error.message : 'Unable to load advisor data.');
    }
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
  renderDiscovery();
  renderDiscoveryOperations();
  renderConfigButtons();
  renderAdvisor();
}

async function saveEndpoints() {
  if (state.selectedEndpointIndex >= 0) {
    state.endpoints[state.selectedEndpointIndex] = readEndpointForm();
  }
  try {
    const payload = await putJson('/api/endpoints', {
      items: normalizeEndpoints(state.endpoints),
      revision: state.endpointsRevision,
    });
    state.endpoints = normalizeEndpoints(payload.items || []);
    state.savedEndpoints = deepClone(state.endpoints);
    state.endpointsRevision = payload.revision || state.endpointsRevision;
    state.endpointsDirty = false;
	const savedByIP = new Map(state.endpoints.map((endpoint) => [String(endpoint.ip || '').trim().toLowerCase(), endpoint]));
	state.discovery.items.forEach((item) => {
	  if (!item._review_staged) {
		return;
	  }
	  const saved = savedByIP.get(String(item.ip || '').trim().toLowerCase());
	  if (!saved) {
		return;
	  }
	  item.discovery_review_state = 'approved';
	  item.discovery_reviewed_at = saved.discovery_reviewed_at || new Date().toISOString();
	  item.discovery_review_note = saved.discovery_review_note || '';
	  delete item._review_staged;
	});
    ensureSelectedEndpoint();
    renderAll();
    setMessage(elements.endpointBanner, payload.reload_pending ? 'warning' : 'success', payload.reload_pending
      ? `Saved ${state.endpoints.length} endpoints. The running collector will apply them between cycles.`
      : `Saved ${state.endpoints.length} endpoints to ${payload.endpoints_path}.`);
    refreshRuntimeStatus();
  } catch (error) {
    const prefix = error?.status === 409 ? 'Save blocked to protect a newer file. ' : '';
    setMessage(elements.endpointBanner, 'error', `${prefix}${error instanceof Error ? error.message : 'Unable to save endpoints.'}`);
  }
}

function resetEndpointsDraft() {
  state.endpoints = deepClone(state.savedEndpoints);
	state.discovery.items.forEach((item) => {
	  delete item._review_staged;
	});
  state.endpointsDirty = false;
  state.selectedEndpointIndices.clear();
  ensureSelectedEndpoint();
  renderAll();
  setMessage(elements.endpointBanner, 'success', 'Reverted endpoint draft to the last saved file state.');
}

function addEndpoint() {
  state.endpoints.push(emptyEndpoint());
  state.endpointsDirty = true;
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
  if (!window.confirm(`Remove ${pluralize(indexes.length, 'selected endpoint')} from the working draft?`)) {
    return;
  }
  const removals = new Set(indexes);
  state.endpoints = state.endpoints.filter((_, index) => !removals.has(index));
  state.endpointsDirty = true;
  state.selectedEndpointIndices.clear();
  ensureSelectedEndpoint();
  renderAll();
  setMessage(elements.endpointBanner, 'warning', `Removed ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} from the working draft. Save endpoints to persist the deletion.`);
}

function discoveryHostEstimate() {
  const mask = Math.min(30, Math.max(16, readNumberValue(elements.discoveryInputs.subnetMask, 24)));
  return Math.max(1, (2 ** (32 - mask)) - 2);
}

function renderDiscoveryPreflight() {
  const mask = Math.min(30, Math.max(16, readNumberValue(elements.discoveryInputs.subnetMask, 24)));
  const hosts = discoveryHostEstimate();
  const target = elements.discoveryInputs.targetNetwork.value.trim() || 'the detected local subnet';
  elements.discoveryPreflight.textContent = `A /${mask} scan can check up to ${hosts.toLocaleString()} host addresses in ${target}. Review the target before starting.`;
  elements.discoveryPreflight.className = `message-banner discovery-preflight ${hosts > 4094 ? 'warning' : ''}`.trim();
}

async function refreshRuntimeStatus() {
  if (state.runtimeRefreshPending) {
    return;
  }
  state.runtimeRefreshPending = true;
  try {
    state.status = await fetchJson('/api/status');
    renderStatus();
  } catch (error) {
    setMessage(elements.runtimeBanner, 'error', error instanceof Error ? error.message : 'Unable to refresh collector status.');
  } finally {
    state.runtimeRefreshPending = false;
  }
}

async function restartCollector() {
  if (!state.status || state.runtimeRestartBusy) {
    return;
  }
  if (configIsDirty()) {
    setMessage(elements.settingsBanner, 'warning', 'Save or reset the current configuration draft before restarting the collector.');
    return;
  }
  const confirmed = window.confirm('Restart the collector now to activate the saved configuration? Monitoring will pause briefly while the current engine shuts down cleanly and reloads validated files.');
  if (!confirmed) {
    return;
  }
  state.runtimeRestartBusy = true;
  let completionMessage = null;
  renderStatus();
  try {
    await postJson('/api/runtime/restart', {
      config_revision: state.configRevision || state.status.config_revision,
      endpoints_revision: state.endpointsRevision || state.status.endpoints_revision,
      confirmed: true,
    });
    setMessage(elements.runtimeBanner, 'warning', 'Controlled restart accepted. Waiting for the collector to activate the saved configuration...');
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 500));
      state.status = await fetchJson('/api/status');
      const runtime = state.status.runtime || {};
      renderStatus();
      if (runtime.last_restart_error) {
        throw new Error(runtime.last_restart_error);
      }
      if (!runtime.restarting && !state.status.config_restart_required && runtime.state === 'running') {
        await reloadAllData(false);
        completionMessage = { tone: 'success', text: 'Collector restarted successfully. The saved configuration is now active.' };
        return;
      }
    }
    throw new Error('The collector did not report a completed restart within 20 seconds. Monitoring status will continue to refresh.');
  } catch (error) {
    completionMessage = { tone: 'error', text: error instanceof Error ? error.message : 'Collector restart failed.' };
  } finally {
    state.runtimeRestartBusy = false;
    renderStatus();
    if (completionMessage) {
      setMessage(elements.runtimeBanner, completionMessage.tone, completionMessage.text);
    }
  }
}

function deleteCurrentEndpoint() {
  const index = state.selectedEndpointIndex;
  if (index < 0 || index >= state.endpoints.length) {
    return;
  }
  const endpoint = state.endpoints[index];
  const label = endpoint.hostname || endpoint.ip || `endpoint ${index + 1}`;
  if (!window.confirm(`Remove ${label} from the working draft?`)) {
    return;
  }
  state.endpoints.splice(index, 1);
  state.endpointsDirty = true;
  state.selectedEndpointIndices.delete(index);
  state.selectedEndpointIndices = new Set(Array.from(state.selectedEndpointIndices).map((selected) => selected > index ? selected - 1 : selected));
  state.selectedEndpointIndex = Math.min(index, state.endpoints.length - 1);
  renderAll();
  setMessage(elements.endpointBanner, 'warning', `Removed ${label} from the working draft. Save endpoints to persist the deletion.`);
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

function setEndpointModeForSelection(mode) {
  const indexes = getEndpointActionIndices();
  if (indexes.length === 0) {
    return;
  }
  if (mode === 'maintenance' && !window.confirm(`Put ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} into Maintenance Mode? The collector will stop sending ICMP probes and emit explicit suppression evidence.`)) {
    return;
  }
  indexes.forEach((index) => {
    if (state.endpoints[index]) {
      state.endpoints[index].device_mode = mode;
      state.endpoints[index].dev = mode === 'legacy_dev';
    }
  });
  state.endpointsDirty = true;
  if (indexes.includes(state.selectedEndpointIndex)) {
    loadEndpointForm(state.endpoints[state.selectedEndpointIndex]);
  }
  renderAll();
  setMessage(elements.endpointBanner, mode === 'maintenance' ? 'warning' : 'success', `Marked ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} as ${mode === 'legacy_dev' ? 'legacy dev/test' : mode} in the working draft. Save endpoints to apply.`);
}

function setEndpointAlertingForSelection(enabled) {
  const indexes = getEndpointActionIndices();
  if (indexes.length === 0) {
    return;
  }
  if (!enabled && !window.confirm(`Disable supported Splunk alerts for ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'}? Pings and measurements will continue.`)) {
    return;
  }
  indexes.forEach((index) => {
    if (state.endpoints[index]) {
      state.endpoints[index].alerting_enabled = enabled;
      if (enabled) {
        state.endpoints[index].alerting_reason = '';
      }
    }
  });
  state.endpointsDirty = true;
  if (indexes.includes(state.selectedEndpointIndex)) {
    loadEndpointForm(state.endpoints[state.selectedEndpointIndex]);
  }
  renderAll();
  setMessage(elements.endpointBanner, enabled ? 'success' : 'warning', `${enabled ? 'Enabled' : 'Disabled'} alerting for ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} in the working draft. Measurement remains active; save endpoints to apply.`);
}

function setEndpointMonitoringForSelection(enabled) {
  const indexes = getEndpointActionIndices();
  if (indexes.length === 0) {
    return;
  }
  if (!enabled && !window.confirm(`Pause monitoring for ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'}? The collector will emit an explicit disabled control state instead of sending ICMP probes.`)) {
    return;
  }
  indexes.forEach((index) => {
    if (!state.endpoints[index]) {
      return;
    }
    state.endpoints[index].monitoring_enabled = enabled;
    if (enabled) {
      state.endpoints[index].maintenance_until = '';
      state.endpoints[index].maintenance_reason = '';
    }
  });
  state.endpointsDirty = true;
  if (indexes.includes(state.selectedEndpointIndex)) {
    loadEndpointForm(state.endpoints[state.selectedEndpointIndex]);
  }
  renderAll();
  setMessage(elements.endpointBanner, enabled ? 'success' : 'warning', `${enabled ? 'Resumed' : 'Paused'} monitoring for ${indexes.length} endpoint${indexes.length === 1 ? '' : 's'} in the working draft. Save endpoints to apply.`);
}

async function saveConfig() {
  try {
    const payload = await putJson('/api/config', { config: readConfigForm(), revision: state.configRevision });
    loadConfigForm(payload.config || {}, payload.secrets || {});
    state.savedConfig = readConfigForm();
    state.configRevision = payload.revision || state.configRevision;
    state.configDirty = false;
    renderStatus();
    renderConfigButtons();
    setMessage(elements.settingsBanner, payload.restart_required ? 'warning' : 'success', payload.restart_required
      ? `Saved ${payload.config_format.toUpperCase()} config. Restart the collector to apply these engine settings.`
      : `Saved ${payload.config_format.toUpperCase()} config to ${payload.config_path}.`);
    refreshRuntimeStatus();
  } catch (error) {
    const prefix = error?.status === 409 ? 'Save blocked to protect a newer file. ' : '';
    setMessage(elements.settingsBanner, 'error', `${prefix}${error instanceof Error ? error.message : 'Unable to save config.'}`);
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
    loadConfigForm(payload.config || {}, payload.secrets || {});
    state.savedConfig = readConfigForm();
    state.configRevision = payload.revision || '';
    state.configDirty = false;
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
  loadConfigForm(state.savedConfig, state.configSecrets);
  state.configDirty = false;
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

function setDiscoveryModeForSelection(mode) {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0) {
    return;
  }
  indexes.forEach((index) => {
    if (state.discovery.items[index]) {
      state.discovery.items[index].device_mode = mode;
      state.discovery.items[index].dev = mode === 'legacy_dev';
    }
  });
  renderDiscovery();
}

function setDiscoveryAlertingForSelection(enabled) {
  getDiscoveryActionIndices().forEach((index) => {
    if (state.discovery.items[index]) {
      state.discovery.items[index].alerting_enabled = enabled;
      if (enabled) {
        state.discovery.items[index].alerting_reason = '';
      }
    }
  });
  renderDiscovery();
}

function setDiscoveryAddressingForSelection(dynamicAddress) {
  getDiscoveryActionIndices().forEach((index) => {
    if (state.discovery.items[index]) {
      state.discovery.items[index].dynamic_address = dynamicAddress;
    }
  });
  renderDiscovery();
  setMessage(elements.discoveryBanner, dynamicAddress ? 'warning' : 'success', dynamicAddress
    ? 'Selected results are marked DHCP/dynamic. Add a stable Asset ID or retain forward-confirmed FQDN evidence before treating them as durable CMDB identities.'
    : 'Selected results are marked static and will use IP as their default discovery identity.');
}

async function persistDiscoveryReviewState(reviewState) {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0 || state.discovery.reviewBusy) {
    return;
  }
  if (reviewState === 'ignored' && !window.confirm(`Ignore ${indexes.length} selected discovery result${indexes.length === 1 ? '' : 's'}? The observations remain in immutable scan history, but they will leave the Needs Review queue.`)) {
    return;
  }
  state.discovery.reviewBusy = true;
  renderDiscovery();
  try {
    const payload = await postJson('/api/discovery/reviews', {
      state: reviewState,
      note: readTextValue(elements.discoveryReviewNote),
      items: indexes.map((index) => state.discovery.items[index]),
    });
    (payload.items || []).forEach((item, offset) => {
      const index = indexes[offset];
      if (state.discovery.items[index]) {
        state.discovery.items[index] = normalizeDiscoveryEndpoint(item);
      }
    });
    state.discovery.selectedIndices.clear();
    setTablePage('discovery', 1);
    const stateLabel = reviewState === 'needs_review' ? 'returned to Needs Review' : `marked ${titleCase(reviewState)}`;
    setMessage(elements.discoveryBanner, 'success', `${indexes.length} discovery result${indexes.length === 1 ? '' : 's'} ${stateLabel}. The decision is retained in the discovery review registry.`);
  } catch (error) {
    setMessage(elements.discoveryBanner, 'error', error instanceof Error ? error.message : 'Unable to persist discovery review state.');
  } finally {
    state.discovery.reviewBusy = false;
    renderDiscovery();
  }
}

function applyDiscoveryBulkFields() {
  const indexes = getDiscoveryActionIndices();
  const values = Object.fromEntries(Object.entries(elements.discoveryBulkFields)
    .map(([key, field]) => [key, readTextValue(field)])
    .filter(([, value]) => value));
  if (indexes.length === 0 || Object.keys(values).length === 0) {
    setMessage(elements.discoveryClassificationPreview, 'warning', 'Select discovery rows and enter at least one common field.');
    return;
  }
  indexes.forEach((index) => Object.assign(state.discovery.items[index], values));
  renderDiscovery();
  setMessage(elements.discoveryClassificationPreview, 'success', `Applied ${Object.keys(values).join(', ')} to ${indexes.length} selected discovery result${indexes.length === 1 ? '' : 's'}.`);
}

async function classifyDiscoverySelection(applyChanges) {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0) {
    return;
  }
  try {
    const payload = await postJson('/api/classification/preview', {
      rules: readClassificationRulesFromEditor(),
      items: indexes.map((index) => state.discovery.items[index]),
    });
    const results = payload.results || [];
    const matched = results.filter((result) => (result.matched_rules || []).length > 0).length;
    const changed = results.filter((result) => Object.keys(result.changes || {}).length > 0).length;
    if (applyChanges) {
      results.forEach((result, offset) => {
        state.discovery.items[indexes[offset]] = normalizeDiscoveryEndpoint(result.endpoint || state.discovery.items[indexes[offset]]);
      });
      renderDiscovery();
    }
    const changeSummary = applyChanges ? `${changed} rows updated` : `${changed} rows would change`;
    setMessage(elements.discoveryClassificationPreview, changed > 0 ? 'success' : 'warning', `${applyChanges ? 'Applied' : 'Previewed'} naming rules for ${indexes.length} row${indexes.length === 1 ? '' : 's'}: ${matched} matched and ${changeSummary}.`);
  } catch (error) {
    setMessage(elements.discoveryClassificationPreview, 'error', error instanceof Error ? error.message : 'Unable to evaluate naming rules.');
  }
}

async function classifyCurrentEndpoint(applyChanges) {
  const index = state.selectedEndpointIndex;
  if (index < 0 || index >= state.endpoints.length) {
    return;
  }
  const candidate = readEndpointForm();
  try {
    const payload = await postJson('/api/classification/preview', {
      rules: readClassificationRulesFromEditor(),
      items: [candidate],
    });
    const result = (payload.results || [])[0] || {};
    const changes = Object.entries(result.changes || {});
    const ruleSummary = (result.matched_rules || []).join(', ') || 'none';
    if (applyChanges && result.endpoint) {
      state.endpoints[index] = normalizeEndpoint(result.endpoint);
      state.endpointsDirty = true;
      loadEndpointForm(state.endpoints[index]);
      renderAll();
    }
    setMessage(elements.endpointClassificationPreview, changes.length > 0 ? 'success' : 'warning', `${applyChanges ? 'Applied' : 'Previewed'} naming rules. Matched: ${ruleSummary}. ${changes.length > 0 ? changes.map(([field, value]) => `${field}=${value}`).join(', ') : 'No structured fields would change.'}`);
  } catch (error) {
    setMessage(elements.endpointClassificationPreview, 'error', error instanceof Error ? error.message : 'Unable to evaluate naming rules.');
  }
}

function mergeEndpointRecords(existingEndpoint, incomingEndpoint, mode) {
  const merged = deepClone(existingEndpoint);
  ['hostname', 'fqdn', 'group', 'description', 'entitytype', 'device', 'vendor', 'asset_id', 'additional_notes', 'classification_source'].forEach((key) => {
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
  [
    'dns_status',
    'dns_forward_confirmed',
    'discovered_at',
    'discovery_scan_id',
    'discovery_source',
    'discovery_latency_ms',
    'subnet_id',
    'subnet_name',
    'subnet_vlan',
    'subnet_location',
    'addressing_mode',
    'routing_domain',
  ].forEach((key) => {
    if (incomingEndpoint[key] !== undefined && incomingEndpoint[key] !== null && incomingEndpoint[key] !== '') {
      merged[key] = incomingEndpoint[key];
    }
  });
  if (mode === 'overwrite') {
    merged.device_mode = effectiveDeviceMode(incomingEndpoint);
    merged.dev = merged.device_mode === 'legacy_dev';
    merged.alerting_enabled = incomingEndpoint.alerting_enabled !== false;
    merged.alerting_reason = incomingEndpoint.alerting_reason || '';
    merged.dynamic_address = Boolean(incomingEndpoint.dynamic_address);
  }
  return normalizeEndpoint(merged);
}

function addSelectedDiscoveryToEndpoints() {
  const indexes = getDiscoveryActionIndices();
  if (indexes.length === 0) {
    return;
  }
	const unassigned = globalThis.PingMonitorDiscoveryReview.unassignedSelection(state.discovery.items, indexes);
	if (unassigned.length > 0) {
	  setMessage(elements.discoveryBanner, 'warning', `${unassigned.length} selected discovery result${unassigned.length === 1 ? ' has' : 's have'} no reviewed device mode. Mark every selected result Production or Maintenance before adding it to the device list.`);
	  return;
	}
  const knownByIP = new Map(state.endpoints.map((endpoint, index) => [String(endpoint.ip || '').trim().toLowerCase(), index]));
  let addedCount = 0;
  let updatedCount = 0;
  let skippedCount = 0;

	const reviewedAt = new Date().toISOString();

  indexes.forEach((index) => {
    const candidate = normalizeEndpoint(globalThis.PingMonitorDiscoveryReview.markStaged(
	  state.discovery.items[index], reviewedAt, readTextValue(elements.discoveryReviewNote),
	));
    const key = String(candidate.ip || '').trim().toLowerCase();
    if (!key) {
      skippedCount += 1;
      return;
    }
    if (!knownByIP.has(key)) {
      state.endpoints.push(candidate);
      knownByIP.set(key, state.endpoints.length - 1);
      addedCount += 1;
	  state.discovery.items[index]._review_staged = true;
      return;
    }
    if (state.discovery.mergeMode === 'skip_existing') {
      skippedCount += 1;
      return;
    }
    const existingIndex = knownByIP.get(key);
    state.endpoints[existingIndex] = mergeEndpointRecords(state.endpoints[existingIndex], candidate, state.discovery.mergeMode);
	state.endpoints[existingIndex].discovery_review_state = 'approved';
	state.endpoints[existingIndex].discovery_reviewed_at = reviewedAt;
	state.endpoints[existingIndex].discovery_review_note = candidate.discovery_review_note;
    updatedCount += 1;
	state.discovery.items[index]._review_staged = true;
  });

  state.discovery.selectedIndices.clear();
  if (addedCount > 0 || updatedCount > 0) {
    state.endpointsDirty = true;
  }
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

function exportDiscoveryCsv(selectedOnly) {
  const indexes = selectedOnly
    ? getDiscoveryActionIndices()
    : state.discovery.items.map((_, index) => index);
  if (indexes.length === 0) {
    setMessage(elements.discoveryBanner, 'warning', selectedOnly ? 'Select at least one discovery result to export.' : 'Run discovery before exporting results.');
    return;
  }
  const exporter = globalThis.PingMonitorDiscoveryCsv;
  if (!exporter || typeof exporter.build !== 'function') {
    setMessage(elements.discoveryBanner, 'error', 'The CSV export component did not load. Refresh the page and try again.');
    return;
  }
  const csv = exporter.build(state.discovery.items, indexes);
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  const stamp = (state.discovery.generatedAt || new Date().toISOString()).replaceAll(':', '').replaceAll('-', '').replace(/\.\d+Z$/, 'Z');
  anchor.href = url;
  anchor.download = `pingmonitor_discovery_${selectedOnly ? 'selected_' : ''}${stamp}.csv`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
  setMessage(elements.discoveryBanner, 'success', `Exported ${indexes.length} discovery result${indexes.length === 1 ? '' : 's'} to CSV.`);
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
      state.discovery.delta = event.delta || null;
      state.discovery.logs = event.logs || state.discovery.logs;
      state.discovery.durationMs = event.duration_ms || 0;
      state.discovery.generatedAt = event.generated_at || new Date().toISOString();
      state.discovery.selectedIndices.clear();
      setMessage(elements.discoveryBanner, 'success', `Discovery completed with ${state.discovery.items.length} endpoint${state.discovery.items.length === 1 ? '' : 's'}. Review and merge the results when ready.`);
      void loadDiscoveryHistory(false);
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
  const estimatedHosts = discoveryHostEstimate();
  if (estimatedHosts > 4094 && !window.confirm(`This discovery can probe up to ${estimatedHosts.toLocaleString()} addresses. Start the scan?`)) {
    return;
  }
  const abortController = new AbortController();
  state.discovery.abortController = abortController;
  state.discovery.running = true;
  state.discovery.runState = 'Starting';
  state.discovery.progressSummary = 'Starting discovery run.';
  state.discovery.items = [];
  state.discovery.summary = null;
  state.discovery.delta = null;
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
      signal: abortController.signal,
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
    if (error?.name === 'AbortError') {
      state.discovery.runState = 'Canceled';
      state.discovery.progressSummary = 'Discovery was canceled by the operator.';
      renderDiscovery();
      setMessage(elements.discoveryBanner, 'warning', 'Discovery canceled. No endpoint file changes were made.');
      return;
    }
    if (state.discovery.runState !== 'Error') {
      state.discovery.runState = 'Error';
    }
    renderDiscovery();
    setMessage(elements.discoveryBanner, 'error', error instanceof Error ? error.message : 'Unable to run discovery.');
  } finally {
    if (state.discovery.abortController === abortController) {
      state.discovery.abortController = null;
    }
  }
}

function cancelDiscovery() {
  state.discovery.abortController?.abort();
}

elements.searchInput.addEventListener('input', (event) => {
  state.search = event.target.value;
  setTablePage('endpoint', 1);
  renderEndpointTable();
  renderEndpointEditor();
});

elements.refreshButton.addEventListener('click', () => {
  if (confirmDiscardChanges()) {
    reloadAllData(true);
  }
});
elements.restartCollectorButton.addEventListener('click', restartCollector);

elements.advisorAnalyzeButton.addEventListener('click', () => loadAdvisor(true));
elements.advisorBenchmarkButton.addEventListener('click', runAdvisorBenchmark);
elements.advisorApplySafeButton.addEventListener('click', () => applyAdvisorFixes('safe'));
elements.advisorApplyProfileButton.addEventListener('click', () => applyAdvisorFixes('profile'));
elements.advisorProfile.addEventListener('change', () => loadAdvisor(false));

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
elements.endpointRows.addEventListener('keydown', (event) => {
  if (event.key !== 'Enter' && event.key !== ' ') {
    return;
  }
  const row = event.target.closest('[data-index]');
  if (!row || event.target.closest('.table-row-checkbox')) {
    return;
  }
  event.preventDefault();
  state.selectedEndpointIndex = Number(row.dataset.index);
  renderEndpointEditor();
  renderEndpointTable();
});

elements.endpointForm.addEventListener('input', () => updateCurrentEndpointFromForm(false));
elements.endpointForm.addEventListener('change', () => updateCurrentEndpointFromForm(true));

elements.addEndpointButton.addEventListener('click', addEndpoint);
elements.selectAllEndpointsButton.addEventListener('click', selectAllVisibleEndpoints);
elements.deselectAllEndpointsButton.addEventListener('click', deselectAllEndpoints);
elements.markSelectedMaintenanceButton.addEventListener('click', () => setEndpointModeForSelection('maintenance'));
elements.markSelectedProductionButton.addEventListener('click', () => setEndpointModeForSelection('production'));
elements.disableSelectedAlertingButton.addEventListener('click', () => setEndpointAlertingForSelection(false));
elements.enableSelectedAlertingButton.addEventListener('click', () => setEndpointAlertingForSelection(true));
elements.pauseSelectedButton.addEventListener('click', () => setEndpointMonitoringForSelection(false));
elements.resumeSelectedButton.addEventListener('click', () => setEndpointMonitoringForSelection(true));
elements.deleteEndpointButton.addEventListener('click', deleteSelectedEndpoint);
elements.deleteCurrentEndpointButton.addEventListener('click', deleteCurrentEndpoint);
elements.previewEndpointClassificationButton.addEventListener('click', () => classifyCurrentEndpoint(false));
elements.applyEndpointClassificationButton.addEventListener('click', () => classifyCurrentEndpoint(true));
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
elements.cancelDiscoveryButton.addEventListener('click', cancelDiscovery);
elements.refreshDiscoveryHistoryButton.addEventListener('click', () => loadDiscoveryHistory(true));
elements.discoveryHistoryRows.addEventListener('click', (event) => {
  const button = event.target.closest('[data-history-scan-id]');
  if (!button || button.disabled) {
    return;
  }
  void loadDiscoveryHistoryScan(button.dataset.historyScanId || '', button.dataset.historyMode || 'all');
});
Object.values(elements.discoveryInputs).forEach((input) => {
  input.addEventListener('input', renderDiscoveryPreflight);
  input.addEventListener('change', renderDiscoveryPreflight);
});
elements.selectAllDiscoveryButton.addEventListener('click', selectAllVisibleDiscovery);
elements.deselectAllDiscoveryButton.addEventListener('click', deselectAllDiscovery);
elements.markDiscoveryMaintenanceButton.addEventListener('click', () => setDiscoveryModeForSelection('maintenance'));
elements.markDiscoveryProductionButton.addEventListener('click', () => setDiscoveryModeForSelection('production'));
elements.disableDiscoveryAlertingButton.addEventListener('click', () => setDiscoveryAlertingForSelection(false));
elements.enableDiscoveryAlertingButton.addEventListener('click', () => setDiscoveryAlertingForSelection(true));
elements.markDiscoveryDynamicButton.addEventListener('click', () => setDiscoveryAddressingForSelection(true));
elements.markDiscoveryStaticButton.addEventListener('click', () => setDiscoveryAddressingForSelection(false));
elements.deferDiscoverySelectedButton.addEventListener('click', () => persistDiscoveryReviewState('deferred'));
elements.ignoreDiscoverySelectedButton.addEventListener('click', () => persistDiscoveryReviewState('ignored'));
elements.resetDiscoveryReviewButton.addEventListener('click', () => persistDiscoveryReviewState('needs_review'));
elements.applyDiscoveryBulkFieldsButton.addEventListener('click', applyDiscoveryBulkFields);
elements.previewDiscoveryClassificationButton.addEventListener('click', () => classifyDiscoverySelection(false));
elements.applyDiscoveryClassificationButton.addEventListener('click', () => classifyDiscoverySelection(true));
elements.exportDiscoveryAllButton.addEventListener('click', () => exportDiscoveryCsv(false));
elements.exportDiscoverySelectedButton.addEventListener('click', () => exportDiscoveryCsv(true));
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
  const assetInput = event.target.closest('.discovery-asset-input');
  if (assetInput) {
    const index = Number(assetInput.dataset.index);
    if (state.discovery.items[index]) {
      state.discovery.items[index].asset_id = String(assetInput.value || '').trim();
      setMessage(elements.discoveryBanner, 'warning', 'Asset ID updated in the review draft. Defer, ignore, or add and save the device to persist this identity.');
    }
    return;
  }
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
initializeAdvancedSettings();

elements.addDiscoverySubnetButton.addEventListener('click', () => {
  state.discoverySubnets = readDiscoverySubnetsFromEditor();
  state.discoverySubnets.push({
    id: `subnet-${state.discoverySubnets.length + 1}`,
    cidr: '',
    name: '',
    vlan: '',
    location: '',
    routing_domain: '',
    addressing_mode: 'static',
  });
  renderDiscoverySubnetEditor();
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});

elements.discoveryReviewFilterButtons.forEach((button) => {
  button.addEventListener('click', () => {
    state.discovery.reviewFilter = button.dataset.discoveryReviewFilter || 'needs_review';
    state.discovery.selectedIndices.clear();
    setTablePage('discovery', 1);
    renderDiscovery();
  });
});
elements.discoverySubnetRows.addEventListener('click', (event) => {
  const button = event.target.closest('[data-remove-subnet]');
  if (!button) {
    return;
  }
  state.discoverySubnets = readDiscoverySubnetsFromEditor();
  state.discoverySubnets.splice(Number(button.dataset.removeSubnet), 1);
  renderDiscoverySubnetEditor();
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});
elements.addClassificationRuleButton.addEventListener('click', () => {
  state.classificationRules = readClassificationRulesFromEditor();
  state.classificationRules.push({
    id: `naming-rule-${state.classificationRules.length + 1}`,
    enabled: true,
    source: 'either',
    pattern: '',
    assignments: {},
    overwrite: false,
    stop_on_match: false,
  });
  renderClassificationRuleEditor();
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});
elements.classificationRuleRows.addEventListener('click', (event) => {
  const removeButton = event.target.closest('[data-remove-rule]');
  const moveButton = event.target.closest('[data-move-rule]');
  if (!removeButton && !moveButton) {
    return;
  }
  state.classificationRules = readClassificationRulesFromEditor();
  if (removeButton) {
    state.classificationRules.splice(Number(removeButton.dataset.removeRule), 1);
  } else {
    const index = Number(moveButton.dataset.index);
    const nextIndex = moveButton.dataset.moveRule === 'up' ? index - 1 : index + 1;
    if (nextIndex >= 0 && nextIndex < state.classificationRules.length) {
      [state.classificationRules[index], state.classificationRules[nextIndex]] = [state.classificationRules[nextIndex], state.classificationRules[index]];
    }
  }
  renderClassificationRuleEditor();
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});
elements.previewClassificationSampleButton.addEventListener('click', previewClassificationSample);

elements.settingsForm.addEventListener('input', (event) => {
  if (event.target.matches('[data-rule-field="id"]')) {
    const row = event.target.closest('.classification-rule-row');
    const title = row?.querySelector('[data-rule-title]');
    if (row && title) {
      title.textContent = `Pair ${Number(row.dataset.index) + 1}: ${readTextValue(event.target) || 'unnamed'}`;
    }
  }
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});
elements.settingsForm.addEventListener('change', () => {
  state.configDirty = true;
  renderConfigButtons();
  renderStatus();
});
elements.testHECButton.addEventListener('click', () => testOutput('hec'));
elements.testMetricsButton.addEventListener('click', () => testOutput('metrics'));
elements.reloadConfigButton.addEventListener('click', () => {
  if (!configIsDirty() || window.confirm('Discard unsaved configuration changes and reload from disk?')) {
    reloadConfig();
  }
});
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
window.addEventListener('beforeunload', (event) => {
  if (!hasUnsavedChanges()) {
    return;
  }
  event.preventDefault();
  event.returnValue = '';
});

reloadAllData();
window.setInterval(refreshRuntimeStatus, 5000);
