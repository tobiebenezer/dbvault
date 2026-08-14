import { titleCase } from '../../format.js';

export function JobProgress({ job = {} }) {
  const percentage = Math.round(Math.max(0, Math.min(100, Number(job.percentage || 0))));
  const indeterminate = !job.bytes_total && percentage === 0 && ['running', 'queued'].includes(job.status);

  let progressClass = 'progress-0';
  if (percentage >= 100) progressClass = 'progress-100';
  else if (percentage >= 75) progressClass = 'progress-75';
  else if (percentage >= 50) progressClass = 'progress-50';
  else if (percentage >= 25) progressClass = 'progress-25';

  return (
    <div className="job-progress">
      <div className="row-between mb-sm">
        <span className="text-muted">
          {titleCase((job.stage || job.status || 'queued').replaceAll('_', ' '))}
        </span>
        <strong>{indeterminate ? 'In progress' : `${percentage}%`}</strong>
      </div>
      <div
        className={`progress-track ${indeterminate ? 'indeterminate' : ''}`}
        role="progressbar"
        aria-valuemin="0"
        aria-valuemax="100"
        aria-valuenow={indeterminate ? undefined : percentage}
        aria-label={`${job.job_type || 'Job'} progress`}
      >
        <span className={indeterminate ? '' : progressClass} />
      </div>
    </div>
  );
}
