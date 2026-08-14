import { StatusIndicator } from '../ui.jsx';
import { titleCase } from '../../format.js';
import { JobProgress } from './JobProgress.jsx';
import { JobActions } from './JobActions.jsx';

export function JobCard({ job = {} }) {
  const tone = toneForJob(job.status);
  return (
    <article className={`job-card ${job.status}`}>
      <div className="job-card-head">
        <div className="job-title-copy">
          <strong>{titleCase((job.job_type || 'job').replaceAll('_', ' '))}</strong>
          <small>{job.resource_name || job.resource_id || 'DBVault resource'}</small>
        </div>
        <StatusIndicator label={titleCase(job.status || 'queued')} tone={tone} />
      </div>

      <JobProgress job={job} />

      <div className="job-card-footer">
        <span>{`${relativeTime(job.updated_at || job.created_at)} · Stage ${job.stage_index || 1}/${job.stage_count || 1}`}</span>
        <JobActions job={job} />
      </div>
    </article>
  );
}

function toneForJob(status) {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'danger';
  if (status === 'cancelled' || status === 'cancelling') return 'warning';
  return 'warning';
}

function relativeTime(value) {
  if (!value) return 'Just now';
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return 'Just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ago`;
}
