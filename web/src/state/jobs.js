export const JobsStore = (() => {
  const terminal = new Set(['completed', 'failed', 'cancelled', 'dead_letter']);
  const seenEvents = new Set();

  function hydrate(jobs = []) {
    const byId = { ...Store.state.jobs.byId };
    for (const job of jobs) byId[job.id] = normaliseJob(job);
    commit(byId, { loading: false, error: null });
  }

  async function load(params = {}) {
    Store.set({ jobs: { ...Store.state.jobs, loading: true, error: null } });
    try {
      const response = await JobsAPI.list(params);
      hydrate(response.jobs || []);
    } catch (error) {
      Store.set({ jobs: { ...Store.state.jobs, loading: false, error } });
    }
  }

  function applyEvent(event) {
    const normalized = normalizeJobEvent(event);
    if (!normalized || !normalized.job_id) return;
    if (normalized.event_id && seenEvents.has(normalized.event_id)) return;
    if (normalized.event_id) seenEvents.add(normalized.event_id);
    if (normalized.organisation_id && normalized.organisation_id !== 'default') return;

    const byId = { ...Store.state.jobs.byId };
    const old = byId[normalized.job_id] || {};
    if (terminal.has(old.status) && normalized.status === 'running') return;
    const next = normaliseJob({
      ...old,
      id: normalized.job_id,
      organisation_id: normalized.organisation_id || old.organisation_id || 'default',
      project_id: normalized.project_id || old.project_id,
      resource_type: normalized.resource_type || old.resource_type || 'source',
      resource_id: normalized.resource_id || old.resource_id || 'production-postgres',
      resource_name: old.resource_name || titleFromResource(normalized.resource_id),
      job_type: normalized.job_type || old.job_type || 'backup',
      status: normalized.status || old.status || 'running',
      stage: normalized.stage || old.stage || '',
      stage_index: normalized.stage_index ?? old.stage_index ?? 1,
      stage_count: normalized.stage_count ?? old.stage_count ?? 1,
      percentage: clamp(normalized.percentage ?? old.percentage ?? 0),
      bytes_processed: normalized.bytes_processed ?? old.bytes_processed ?? 0,
      bytes_total: normalized.bytes_total ?? old.bytes_total ?? 0,
      throughput_bps: normalized.throughput_bps ?? old.throughput_bps ?? 0,
      message: normalized.message || old.message || '',
      can_cancel: normalized.can_cancel ?? old.can_cancel ?? false,
      updated_at: normalized.created_at || new Date().toISOString(),
      created_at: old.created_at || normalized.created_at || new Date().toISOString()
    });
    byId[next.id] = next;
    commit(byId, {});
    maybeToast(normalized, next);
  }

  function markCancelling(id) {
    const byId = { ...Store.state.jobs.byId };
    if (byId[id]) byId[id] = { ...byId[id], status: 'cancelling', message: 'Cancellation requested.' };
    commit(byId, {});
  }

  function setConnectionStatus(status, error = null) {
    Store.set({ connection: { ...Store.state.connection, status, error } });
  }

  function setLastEventId(lastEventId) {
    Store.set({ connection: { ...Store.state.connection, lastEventId } });
  }

  function commit(byId, patch) {
    const values = Object.values(byId).sort((a, b) => new Date(b.updated_at || b.created_at) - new Date(a.updated_at || a.created_at));
    Store.set({
      jobs: {
        ...Store.state.jobs,
        ...patch,
        byId,
        orderedIds: values.map((job) => job.id),
        activeIds: values.filter((job) => ['queued', 'running', 'cancelling'].includes(job.status)).map((job) => job.id),
        failedIds: values.filter((job) => job.status === 'failed').map((job) => job.id),
        completedIds: values.filter((job) => job.status === 'completed').map((job) => job.id)
      }
    });
  }

  function normaliseJob(job) {
    return { ...job, percentage: clamp(job.percentage || 0), status: job.status || 'queued', can_cancel: !!job.can_cancel };
  }

  function maybeToast(event, job) {
    if (event.event_type === 'job.started') Store.toast(`${labelJob(job)} started`, 'info');
    if (event.event_type === 'job.completed') Store.toast(`${labelJob(job)} completed`, 'success');
    if (event.event_type === 'job.failed') Store.toast(`${labelJob(job)} failed. Existing verified backups remain safe.`, 'danger');
    if (event.event_type === 'job.cancelled') Store.toast(`${labelJob(job)} cancelled`, 'warning');
    const pct = Math.round(job.percentage || 0);
    if (event.event_type === 'job.progress' && [25, 50, 75].includes(pct)) Store.toast(`${labelJob(job)} is ${pct}% complete`, 'info');
  }

  function labelJob(job) { return `${titleCase((job.job_type || 'job').replaceAll('_', ' '))} ${job.resource_name ? 'for ' + job.resource_name : ''}`; }
  function titleFromResource(id) { return (id || 'resource').replaceAll('-', ' '); }
  function clamp(v) { return Math.max(0, Math.min(100, Number(v) || 0)); }

  return { load, hydrate, applyEvent, markCancelling, setConnectionStatus, setLastEventId };
})();
