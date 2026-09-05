export function formatBytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return `${n} B`;
  const units = ['KiB', 'MiB', 'GiB', 'TiB'];
  let current = n / 1024;
  for (const unit of units) {
    if (Math.abs(current) < 1024) return `${current.toFixed(current >= 10 ? 1 : 2)} ${unit}`;
    current /= 1024;
  }
  return `${current.toFixed(1)} PiB`;
}

export function formatDate(value) {
  if (!value) return 'Not available';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

export function titleCase(value) {
  return String(value || '').replace(/[_-]/g, ' ').replace(/\b\w/g, (m) => m.toUpperCase());
}

export function safeText(value, fallback = 'Not available') {
  if (value === null || value === undefined || value === '') return fallback;
  return String(value);
}

export function formatRelative(value) {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getFullYear() <= 1970) return 'Never';
  const seconds = Math.round((date.getTime() - Date.now()) / 1000);
  const ranges = [[60, 'second'], [60, 'minute'], [24, 'hour'], [7, 'day'], [4.345, 'week'], [12, 'month'], [Infinity, 'year']];
  let amount = seconds;
  for (const [limit, unit] of ranges) {
    if (Math.abs(amount) < limit) return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(Math.round(amount), unit);
    amount /= limit;
  }
  return formatDate(value);
}

export function databaseHasBackup(db) {
  if (!db) return false;
  if (typeof db.has_backup === 'boolean') return db.has_backup;
  if (db.backup_count && db.backup_count > 0) return true;
  if (!db.last_backup_at) return false;
  const date = new Date(db.last_backup_at);
  return !Number.isNaN(date.getTime()) && date.getFullYear() > 2000;
}

export function getJobTone(status) {
  const s = String(status || '').toLowerCase();
  if (['completed', 'finished', 'succeeded', 'healthy', 'success'].includes(s)) return 'success';
  if (['failed', 'error', 'dead_letter'].includes(s)) return 'danger';
  if (['cancelled', 'cancelling'].includes(s)) return 'warning';
  if (['running', 'in_progress'].includes(s)) return 'primary';
  return 'neutral';
}

export function formatDurationSeconds(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const remS = s % 60;
  if (m < 60) return remS > 0 ? `${m}m ${remS}s` : `${m}m`;
  const h = Math.floor(m / 60);
  const remM = m % 60;
  return remM > 0 ? `${h}h ${remM}m` : `${h}h`;
}

export function formatJobDuration(job) {
  if (!job) return '—';
  if (typeof job.duration_seconds === 'number' && job.duration_seconds >= 0) {
    return formatDurationSeconds(job.duration_seconds);
  }
  const start = job.started_at || job.created_at;
  if (!start) return '—';
  const startTime = new Date(start).getTime();
  if (Number.isNaN(startTime) || startTime <= 0) return '—';

  const end = job.completed_at || job.finished_at;
  if (end) {
    const endTime = new Date(end).getTime();
    if (!Number.isNaN(endTime) && endTime >= startTime) {
      const sec = Math.round((endTime - startTime) / 1000);
      return formatDurationSeconds(sec);
    }
  }

  if (['running', 'queued', 'cancelling'].includes(job.status)) {
    const elapsedSec = Math.max(0, Math.round((Date.now() - startTime) / 1000));
    return `${formatDurationSeconds(elapsedSec)} (elapsed)`;
  }

  return '—';
}

