import { Button } from '../ui.jsx';
import { JobsStore } from '../../state/jobs.js';
import { JobsAPI } from '../../api/jobs.js';
import { Store } from '../../state.js';

export function JobActions({ job = {} }) {
  const isCancellable = job.can_cancel && ['queued', 'running'].includes(job.status);
  const isFailed = job.status === 'failed';

  const handleCancel = async () => {
    const ok = await Store.confirm({
      title: 'Cancel Active Job',
      message: 'Are you sure you want to cancel this operation at the next safe cancellation point?',
      confirmLabel: 'Cancel Job',
      confirmTone: 'danger'
    });
    if (!ok) return;
    JobsStore.markCancelling(job.id);
    try {
      await JobsAPI.cancel(job.id);
    } catch (error) {
      Store.toast(error.message, 'danger');
    }
  };

  const handleRetry = async () => {
    try {
      const result = await JobsAPI.retry(job.id);
      JobsStore.hydrate([result.job]);
      Store.toast(`Retry created: ${result.new_job_id}`, 'success');
    } catch (error) {
      Store.toast(error.message, 'danger');
    }
  };

  return (
    <div className="job-actions row-actions">
      {isCancellable && (
        <Button
          label="Cancel"
          onClick={handleCancel}
          tone="ghost compact"
          icon="close"
        />
      )}
      {isFailed && (
        <Button
          label="Retry"
          onClick={handleRetry}
          tone="primary compact"
          icon="refresh"
        />
      )}
      <Button
        label="Details"
        onClick={() => Store.navigate(`/jobs/${job.id}`)}
        tone="secondary compact"
      />
    </div>
  );
}
