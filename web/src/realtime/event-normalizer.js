export function normalizeJobEvent(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const event = { ...raw };
  event.event_id = event.event_id || event.id || '';
  event.event_type = event.event_type || raw.type || 'job.progress';
  event.job_id = event.job_id || event.jobId || '';
  event.percentage = clampPercent(event.percentage);
  event.stage_index = Number(event.stage_index || 0);
  event.stage_count = Math.max(1, Number(event.stage_count || 1));
  if (event.stage_index > event.stage_count) event.stage_index = event.stage_count;
  event.bytes_processed = Number(event.bytes_processed || 0);
  event.bytes_total = Number(event.bytes_total || 0);
  event.throughput_bps = Number(event.throughput_bps || 0);
  return event;
}
function clampPercent(v) { return Math.max(0, Math.min(100, Number(v) || 0)); }
