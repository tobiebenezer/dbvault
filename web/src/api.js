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
    status: () => request('/api/v1/status'),
    overview: () => request('/api/v1/overview'),
    inventory: () => request('/api/v1/inventory'),
    protection: (sourceId = 'production-postgres') => request(`/api/v1/protection-summary?source_id=${encodeURIComponent(sourceId)}`),
    setup: () => request('/api/v1/setup'),
    saveSetupStep: (step, draft = {}) => request(`/api/v1/setup/steps/${encodeURIComponent(step)}`, { method: 'POST', body: JSON.stringify(draft) }),
    finishSetup: () => request('/api/v1/setup/finish', { method: 'POST', body: '{}' }),
    setSetupStep: (step) => request('/api/v1/setup/current-step', { method: 'POST', body: JSON.stringify({ step }) }),
    timeline: (sourceId = 'production-postgres') => request(`/api/v1/sources/${encodeURIComponent(sourceId)}/recovery-timeline`),
    doctor: () => request('/api/v1/doctor/run', { method: 'POST', body: '{}' }),
    simulatePolicy: (draft) => request('/api/v1/policies/simulate', { method: 'POST', body: JSON.stringify(draft) }),
    discover: (agentId = 'local', roots = []) => request(`/api/v1/agents/${encodeURIComponent(agentId)}/discover`, { method: 'POST', body: JSON.stringify({ roots }) }),
    discoveries: () => request('/api/v1/discoveries'),
    adoptDiscovery: (id) => request(`/api/v1/discoveries/${encodeURIComponent(id)}/adopt`, { method: 'POST', body: '{}' }),
    ignoreDiscovery: (id) => request(`/api/v1/discoveries/${encodeURIComponent(id)}/ignore`, { method: 'POST', body: '{}' }),
    alerts: () => request('/api/v1/alerts'),
    acknowledgeAlert: (id) => request(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`, { method: 'POST', body: '{}' }),
    resolveAlert: (id) => request(`/api/v1/alerts/${encodeURIComponent(id)}/resolve`, { method: 'POST', body: '{}' }),
    createApproval: (payload) => request('/api/v1/approvals', { method: 'POST', body: JSON.stringify(payload) }),
    createSandbox: (payload) => request('/api/v1/sandboxes', { method: 'POST', body: JSON.stringify(payload) }),
    recoveryBundle: () => request('/api/v1/recovery-bundles', { method: 'POST', body: '{}' }),
    supportBundle: () => request('/api/v1/support-bundles', { method: 'POST', body: '{}' }),
    releaseManifest: () => request('/install/releases/current/manifest.json'),
    jobs: (params = {}) => request(`/api/v1/jobs${query(params)}`),
    job: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}`),
    createJob: (payload = {}) => request('/api/v1/jobs', { method: 'POST', body: JSON.stringify(payload) }),
    cancelJob: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: '{}' }),
    retryJob: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/retry`, { method: 'POST', body: '{}' }),
    jobLogs: (id) => request(`/api/v1/jobs/${encodeURIComponent(id)}/logs`)
  };
  function query(params) {
    const pairs = Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== '');
    if (!pairs.length) return '';
    return '?' + pairs.map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`).join('&');
  }
})();
