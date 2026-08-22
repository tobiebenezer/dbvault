export const API = (() => {
  const jsonHeaders = { 'content-type': 'application/json' };
  async function request(path, options = {}) {
    const response = await fetch(path, { headers: jsonHeaders, credentials: 'same-origin', ...options });
    const contentType = response.headers.get('content-type') || '';
    const body = contentType.includes('application/json') ? await response.json() : await response.text();
    if (!response.ok) {
      const error = new Error(typeof body === 'string' ? body : body.error || `Request failed: ${response.status}`);
      error.status = response.status;
      error.body = body;
      throw error;
    }
    return body;
  }
  return {
    request,
    // Core
    status: () => request('/api/v1/status'),
    overview: () => request('/api/v1/overview'),
    inventory: () => request('/api/v1/inventory'),
    protection: (sourceId) => sourceId
      ? request(`/api/v1/protection-summary?source_id=${encodeURIComponent(sourceId)}`)
      : Promise.resolve({ primary_status: { status: 'protected', score: 100, summary: 'No databases configured yet' }, summary: { protected_databases: 0, at_risk_databases: 0, unprotected_databases: 0 } }),
    // Setup
    setup: () => request('/api/v1/setup'),
    saveSetupStep: (step, draft = {}) => request(`/api/v1/setup/steps/${encodeURIComponent(step)}`, { method: 'POST', body: JSON.stringify(draft) }),
    finishSetup: () => request('/api/v1/setup/finish', { method: 'POST', body: '{}' }),
    setSetupStep: (step) => request('/api/v1/setup/current-step', { method: 'POST', body: JSON.stringify({ step }) }),
    // Master Key & Disaster Recovery Custody
    masterKeyStatus: () => request('/api/v1/keys/master'),
    masterKeyReveal: () => request('/api/v1/keys/master/reveal', { method: 'POST', body: '{}' }),
    masterKeySet: (payload) => request('/api/v1/keys/master/set', { method: 'POST', body: JSON.stringify(payload) }),
    masterKeyGenerate: () => request('/api/v1/keys/master/generate', { method: 'POST', body: '{}' }),
    // Recovery, Timeline & WAL Telemetry
    timeline: (sourceId) => sourceId
      ? request(`/api/v1/sources/${encodeURIComponent(sourceId)}/recovery-timeline`)
      : Promise.resolve({ snapshots: [], events: [], windows: [], continuous: false }),
    walStatus: (sourceId) => sourceId
      ? request(`/api/v1/sources/${encodeURIComponent(sourceId)}/wal-status`)
      : Promise.resolve({ status: 'no_source', continuous: false, active_slots: 0 }),
    replayEstimate: (sourceId, targetTime) => request(`/api/v1/sources/${encodeURIComponent(sourceId)}/replay-estimate`, { method: 'POST', body: JSON.stringify({ target_time: targetTime }) }),
    complianceCertificate: (sourceId) => sourceId
      ? request(`/api/v1/pitr/compliance-certificate?source_id=${encodeURIComponent(sourceId)}`)
      : Promise.resolve({}),
    downloadComplianceCertificate: (sourceId) => {
      if (!sourceId) {
        return;
      }
      window.open(`/api/v1/pitr/compliance-certificate?source_id=${encodeURIComponent(sourceId)}&format=markdown`, '_blank');
    },
    downloadDecryptedSQL: (databaseId) => {
      if (!databaseId) return;
      window.open(`/api/v1/databases/${encodeURIComponent(databaseId)}/export-sql`, '_blank');
    },
    // Data Warehouse & Analytical Lakehouse
    warehouseCatalog: () => request('/api/v1/warehouse/catalog'),
    warehouseQuery: (query, engine = 'auto', database = '') => request('/api/v1/warehouse/query', { method: 'POST', body: JSON.stringify({ query, engine, database: database || undefined }) }),
    warehouseSync: (databaseId, connectorId) => request('/api/v1/warehouse/sync', { method: 'POST', body: JSON.stringify({ database_id: databaseId, connector_id: connectorId }) }),
    warehouseConnectors: () => request('/api/v1/warehouse/connectors'),
    biConnections: () => request('/api/v1/bi/connections'),
    createBIConnection: (payload) => request('/api/v1/bi/connections', { method: 'POST', body: JSON.stringify(payload) }),
    rotateBIConnection: (id) => request(`/api/v1/bi/connections/${encodeURIComponent(id)}/rotate`, { method: 'POST', body: '{}' }),
    testBIConnection: (id) => request(`/api/v1/bi/connections/${encodeURIComponent(id)}/test`, { method: 'POST', body: '{}' }),
    revokeBIConnection: (id) => request(`/api/v1/bi/connections/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    downloadWarehouseExport: async (query, format = 'csv', engine = 'auto', database = '') => {
      const response = await fetch('/api/v1/warehouse/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query, format, engine, database: database || undefined })
      });
      if (!response.ok) {
        const err = await response.json().catch(() => ({ error: 'Export failed' }));
        throw new Error(err.error || 'Export failed');
      }
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `query_export_${Date.now()}.${format}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
    },
    // Storage Garbage Collection
    gcPlan: () => request('/api/v1/gc/plan', { method: 'POST', body: '{}' }),
    gcRun: () => request('/api/v1/gc/run', { method: 'POST', body: '{}' }),
    // Diagnostics
    doctor: () => request('/api/v1/doctor/run', { method: 'POST', body: '{}' }),
    simulatePolicy: (draft) => request('/api/v1/policies/simulate', { method: 'POST', body: JSON.stringify(draft) }),
    // Discovery & Engine Introspection
    discover: (agentId = 'local', roots = []) => request(`/api/v1/agents/${encodeURIComponent(agentId)}/discover`, { method: 'POST', body: JSON.stringify({ roots }) }),
    discoveries: () => request('/api/v1/discoveries'),
    adoptDiscovery: (id) => request(`/api/v1/discoveries/${encodeURIComponent(id)}/adopt`, { method: 'POST', body: '{}' }),
    ignoreDiscovery: (id) => request(`/api/v1/discoveries/${encodeURIComponent(id)}/ignore`, { method: 'POST', body: '{}' }),
    probeEngine: (payload) => request('/api/v1/databases/probe', { method: 'POST', body: JSON.stringify(payload) }),
    probeDatabaseEngine: (payload) => request('/api/v1/databases/probe', { method: 'POST', body: JSON.stringify(payload) }),
    adoptBatch: (payload) => request('/api/v1/databases/adopt-batch', { method: 'POST', body: JSON.stringify(payload) }),
    adoptDatabaseBatch: (payload) => request('/api/v1/databases/adopt-batch', { method: 'POST', body: JSON.stringify(payload) }),
    createDatabase: (payload) => request('/api/v1/databases', { method: 'POST', body: JSON.stringify(payload) }),
    deleteDatabase: (id) => request(`/api/v1/databases/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    // Alerts & Incident Management
    alerts: () => request('/api/v1/alerts'),
    acknowledgeAlert: (id) => request(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`, { method: 'POST', body: '{}' }),
    resolveAlert: (id) => request(`/api/v1/alerts/${encodeURIComponent(id)}/resolve`, { method: 'POST', body: '{}' }),
    // Approvals
    approvals: () => request('/api/v1/approvals'),
    createApproval: (payload) => request('/api/v1/approvals', { method: 'POST', body: JSON.stringify(payload) }),
    approveRequest: (id) => request(`/api/v1/approvals/${encodeURIComponent(id)}/approve`, { method: 'POST', body: '{}' }),
    rejectRequest: (id) => request(`/api/v1/approvals/${encodeURIComponent(id)}/reject`, { method: 'POST', body: '{}' }),
    decideApproval: (id, payload) => request(`/api/v1/approvals/${encodeURIComponent(id)}/decide`, { method: 'POST', body: JSON.stringify(payload) }),
    createSandbox: (payload) => request('/api/v1/sandboxes', { method: 'POST', body: JSON.stringify(payload) }),
    // Disaster Recovery & Support Bundles
    recoveryBundle: () => request('/api/v1/recovery-bundles', { method: 'POST', body: '{}' }),
    supportBundle: () => request('/api/v1/support-bundles', { method: 'POST', body: '{}' }),
    releaseManifest: () => request('/install/releases/current/manifest.json'),
    // Operations & Jobs
    jobs: (params = {}) => request(`/api/v1/jobs${query(params)}`),
    job: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}`),
    createJob: (payload = {}) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify(payload) }),
    createBackup: (sourceId) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ job_type: 'backup', resource_id: sourceId, resource_name: sourceId }) }),
    createDrill: (sourceId) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ job_type: 'restore_drill', resource_id: sourceId, resource_name: sourceId }) }),
    retryJob: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/retry`, { method: 'POST', body: '{}' }),
    cancelJob: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: '{}' }),
    jobEvents: () => new EventSource('/api/v1/events/jobs'),
    jobLogs: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/logs`),
    // Automation Schedules
    schedules: () => request('/api/v1/schedules'),
    createSchedule: (payload) => request('/api/v1/schedules', { method: 'POST', body: JSON.stringify(payload) }),
    updateSchedule: (id, payload) => request(`/api/v1/schedules/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(payload) }),
    deleteSchedule: (id) => request(`/api/v1/schedules/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    triggerSchedule: (id) => request(`/api/v1/schedules/${encodeURIComponent(id)}/trigger`, { method: 'POST', body: '{}' }),
    // Security & Immutability
    immutability: () => request('/api/v1/security/immutability'),
    updateImmutability: (payload) => request('/api/v1/security/immutability', { method: 'POST', body: JSON.stringify(payload) }),
    toggleLegalHold: (payload) => request('/api/v1/security/legal-hold', { method: 'POST', body: JSON.stringify(payload) }),
    // Usage & Billing
    billingUsage: () => request('/api/v1/billing/usage'),
    // Audit Logs
    auditEvents: (limit = 100) => request(`/api/v1/audit/events?limit=${limit}`),
    auditExport: () => { window.open('/api/v1/audit/export', '_blank'); },
    // Team & RBAC
    teamMembers: () => request('/api/v1/team/members'),
    inviteTeamMember: (payload) => request('/api/v1/team/members', { method: 'POST', body: JSON.stringify(payload) }),
    updateTeamMember: (id, payload) => request(`/api/v1/team/members/${encodeURIComponent(id)}/role`, { method: 'PATCH', body: JSON.stringify(payload) }),
    updateTeamMemberRole: (id, role) => request(`/api/v1/team/members/${encodeURIComponent(id)}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),
    removeTeamMember: (id) => request(`/api/v1/team/members/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    // Notification Channels
    notificationChannels: () => request('/api/v1/notifications/channels'),
    createNotificationChannel: (payload) => request('/api/v1/notifications/channels', { method: 'POST', body: JSON.stringify(payload) }),
    updateNotificationChannel: (id, payload) => request(`/api/v1/notifications/channels/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(payload) }),
    deleteNotificationChannel: (id) => request(`/api/v1/notifications/channels/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    testNotification: (channelId, url) => request('/api/v1/notifications/test', { method: 'POST', body: JSON.stringify({ channel_id: channelId, url }) }),
    // Outbound Agents & Fleet
    agents: () => request('/api/v1/agents'),
    revokeAgent: (id) => request(`/api/v1/agents/${encodeURIComponent(id)}/revoke`, { method: 'POST', body: '{}' }),
    adminFleet: () => request('/api/v1/admin/fleet'),
    // Remote Database Test
    testRemoteURI: (engine, uri) => request('/api/v1/remote/test', { method: 'POST', body: JSON.stringify({ engine, uri }) }),
    // Database schema & table exclusions
    databaseSchema: (id) => sourceIdOrParam(id),
    setTableExclusions: (sourceId, tables) => request('/api/v1/databases/schema/exclusions', { method: 'POST', body: JSON.stringify({ source_id: sourceId, tables }) }),
    maskingRules: () => request('/api/v1/privacy/masking-rules'),
    powerBICatalog: () => request('/api/v1/bi/powerbi/catalog'),
    // Destination management & live testing
    createDestination: (payload) => request('/api/v1/destinations', { method: 'POST', body: JSON.stringify(payload) }),
    deleteDestination: (id) => request(`/api/v1/destinations/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    testDestination: (payload) => request('/api/v1/destinations/test', { method: 'POST', body: JSON.stringify(payload) }),
    benchmarkDestination: (destId) => request(`/api/v1/destinations/${encodeURIComponent(destId)}/benchmark`, { method: 'POST', body: '{}' }),
    // Snapshot listing per source
    listSnapshots: (sourceId) => request(`/api/v1/sources/${encodeURIComponent(sourceId)}/snapshots`),
    // Trigger specific backup operations
    triggerBackup: (sourceId) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ job_type: 'backup', resource_id: sourceId, resource_name: sourceId }) }),
    verifySnapshot: (sourceId, snapId) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ job_type: 'verification', resource_id: snapId, resource_name: sourceId }) }),
    triggerReplication: (sourceId, snapId) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ job_type: 'replication', resource_id: snapId, resource_name: sourceId }) }),
  };
  function sourceIdOrParam(id) {
    if (!id) return Promise.resolve({ tables: [], engine: 'postgres', database: '' });
    return request(`/api/v1/databases/schema?id=${encodeURIComponent(id)}`);
  }
  function query(params) {
    const pairs = Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== '');
    if (!pairs.length) return '';
    return '?' + pairs.map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`).join('&');
  }
})();
