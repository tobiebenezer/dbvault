import { useState, useEffect } from 'preact/hooks';
import { Card, Button, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { JobCard } from '../components/jobs/JobCard.jsx';
import { JobStageList } from '../components/jobs/JobStageList.jsx';
import { JobLogViewer } from '../components/jobs/JobLogViewer.jsx';
import { JobsStore } from '../state/jobs.js';
import { JobsAPI } from '../api/jobs.js';
import { Store } from '../state.js';
import { formatRelative, titleCase } from '../format.js';
import { API } from '../api.js';

export function JobsPage() {
  const [jobsState, setJobsState] = useState(Store.state.jobs);
  const [filter, setFilter] = useState(Store.state.jobFilter || 'all');
  const [currentRoute, setCurrentRoute] = useState(Store.state.route);

  useEffect(() => {
    return Store.subscribe((s) => {
      setJobsState({ ...s.jobs });
      setCurrentRoute(s.route);
      if (s.jobFilter) setFilter(s.jobFilter);
    });
  }, []);

  const parts = currentRoute.split('/').filter(Boolean);
  if (parts.length === 2 && parts[0] === 'jobs') {
    return <JobDetailPage jobId={parts[1]} />;
  }

  const allJobs = jobsState.orderedIds.map((id) => jobsState.byId[id]).filter(Boolean);
  const visible = filter === 'all'
    ? allJobs
    : allJobs.filter((job) => filter === 'running'
      ? ['queued', 'running', 'cancelling'].includes(job.status)
      : job.status === filter);

  if (jobsState.loading && !allJobs.length) return <div className="page"><LoadingState label="Loading jobs…" /></div>;
  if (jobsState.error && !allJobs.length) return <div className="page"><ErrorBox error={jobsState.error} retry={() => JobsStore.load()} /></div>;

  return (
    <div className="page">
      <PageHeader
        title="Operations & Jobs"
        description="Real-time background tasks, automated backup streams, restore drills, and storage synchronisation."
        actions={[
          <Button key="drill" label="Run restore drill" onClick={() => createJobFromInventory('restore_drill')} tone="secondary" />,
          <Button key="backup" label="Start backup" onClick={() => createJobFromInventory('backup')} tone="primary" />
        ]}
      />

      <SegmentedNav
        tabs={[
          { id: 'all', label: 'All Jobs', count: allJobs.length },
          { id: 'running', label: 'Running', count: countJobs(allJobs, ['queued', 'running', 'cancelling']) },
          { id: 'failed', label: 'Failed', count: countJobs(allJobs, ['failed']) },
          { id: 'completed', label: 'Completed', count: countJobs(allJobs, ['completed']) }
        ]}
        activeId={filter}
        onSelect={(id) => {
          setFilter(id);
          Store.set({ jobFilter: id });
        }}
      />

      <Card title="Execution History" noPadding>
        {visible.length === 0 ? (
          <EmptyState
            title="No Jobs"
            text={filter === 'all' ? 'No jobs have been run yet.' : `No ${filter} jobs.`}
          />
        ) : (
          <DataTable
            headers={[
              { label: 'Job ID' },
              { label: 'Operation Type' },
              { label: 'Target Resource' },
              { label: 'Status' },
              { label: 'Progress' },
              { label: 'Updated' },
              { label: 'Actions', width: '160px' }
            ]}
          >
            {visible.map((job) => {
              const tone = job.status === 'completed' ? 'success' : job.status === 'failed' ? 'danger' : 'warning';
              const isActive = ['queued', 'running', 'cancelling'].includes(job.status);
              const shortId = `#…${(job.id || '').slice(-8)}`;
              const progressCell = isActive
                ? `${Math.round(job.percentage || 0)}%`
                : job.status === 'failed' ? 'Failed' : job.status === 'cancelled' ? 'Cancelled' : 'Done';

              return (
                <tr key={job.id}>
                  <td className="cell-mono" title={job.id}>{shortId}</td>
                  <td className="cell-primary">{titleCase((job.job_type || 'job').replaceAll('_', ' '))}</td>
                  <td>{job.resource_name || job.resource_id || 'DBVault resource'}</td>
                  <td><StatusIndicator label={titleCase(job.status || 'queued')} tone={tone} /></td>
                  <td>{progressCell}</td>
                  <td className="cell-mono">{formatRelative(job.updated_at || job.created_at)}</td>
                  <td className="cell-actions cell-actions-w160">
                    {isActive && job.can_cancel && (
                      <Button
                        label="Cancel"
                        onClick={async () => {
                          if (!confirm('Cancel this job?')) return;
                          JobsStore.markCancelling(job.id);
                          try { await JobsAPI.cancel(job.id); } catch (err) { Store.toast(err.message, 'danger'); }
                        }}
                        tone="ghost compact"
                      />
                    )}
                    <Button
                      label="Logs"
                      onClick={() => Store.navigate(`/jobs/${job.id}`)}
                      tone="secondary compact"
                    />
                  </td>
                </tr>
              );
            })}
          </DataTable>
        )}
      </Card>
    </div>
  );
}

function JobDetailPage({ jobId }) {
  const [job, setJob] = useState(Store.state.jobs.byId[jobId]);
  const [loading, setLoading] = useState(!job);

  useEffect(() => {
    if (!job) {
      setLoading(true);
      JobsAPI.detail(jobId)
        .then((fresh) => {
          JobsStore.hydrate([fresh]);
          setJob(fresh);
          setLoading(false);
        })
        .catch((err) => {
          Store.toast(err.message, 'danger');
          setLoading(false);
        });
    }
    return Store.subscribe((s) => {
      if (s.jobs.byId[jobId]) setJob(s.jobs.byId[jobId]);
    });
  }, [jobId]);

  if (loading || !job) return <div className="page"><LoadingState label="Loading job details…" /></div>;

  const shortId = `#…${(job.id || '').slice(-8)}`;

  return (
    <div className="page">
      <div className="breadcrumb-row">
        <button className="breadcrumb-link" onClick={() => Store.navigate('/jobs')} type="button">
          <span>← All Jobs</span>
        </button>
      </div>

      <PageHeader
        title={`${titleCase((job.job_type || 'Job').replaceAll('_', ' '))} — ${shortId}`}
        description={`Resource: ${job.resource_name || job.resource_id || 'Unknown'}`}
        actions={[
          <Button key="back" label="Back to all jobs" onClick={() => Store.navigate('/jobs')} tone="secondary" />
        ]}
      />

      <div className="grid-2">
        <Card title="Execution State">
          <JobCard job={job} />
        </Card>
        <Card title="Pipeline Stages">
          <JobStageList job={job} />
        </Card>
      </div>

      <Card title="Live Execution Log">
        <JobLogViewer jobId={job.id} />
      </Card>
    </div>
  );
}

function countJobs(jobs, statuses) {
  return jobs.filter((job) => statuses.includes(job.status)).length;
}

async function createJobFromInventory(type) {
  try {
    const inventory = await API.inventory().catch(() => ({ databases: [] }));
    const primaryDb = (inventory.databases || [])[0];
    const resource = primaryDb
      ? { resource_id: primaryDb.id, resource_name: primaryDb.name }
      : { resource_id: 'default', resource_name: 'Default database' };
    const job = await JobsAPI.create({ job_type: type, ...resource });
    JobsStore.hydrate([job]);
    Store.toast('Job queued successfully', 'success');
    Store.navigate(`/jobs/${job.id}`);
  } catch (error) {
    Store.toast(error.message, 'danger');
  }
}
