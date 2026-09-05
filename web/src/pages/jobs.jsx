import { useState, useEffect } from 'preact/hooks';
import { Card, Button, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { JobActions } from '../components/jobs/JobActions.jsx';
import { JobStageList } from '../components/jobs/JobStageList.jsx';
import { JobLogViewer } from '../components/jobs/JobLogViewer.jsx';
import { JobsStore } from '../state/jobs.js';
import { JobsAPI } from '../api/jobs.js';
import { Store } from '../state.js';
import { formatRelative, titleCase, formatBytes, getJobTone, formatJobDuration } from '../format.js';
import { API } from '../api.js';
import { AlertsPage } from './alerts.jsx';
import { AuditPage } from './audit.jsx';

export function JobsPage() {
  const [hubTab, setHubTab] = useState('jobs'); // 'jobs' | 'alerts' | 'audit'
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
    : allJobs.filter((job) => {
        if (filter === 'running') return ['queued', 'running', 'cancelling'].includes(job.status);
        if (filter === 'completed') return ['completed', 'finished', 'succeeded', 'healthy', 'success'].includes(job.status);
        if (filter === 'failed') return ['failed', 'dead_letter', 'error'].includes(job.status);
        return job.status === filter;
      });

  if (jobsState.loading && !allJobs.length && hubTab === 'jobs') {
    return <div className="page"><LoadingState label="Loading operations…" /></div>;
  }
  if (jobsState.error && !allJobs.length && hubTab === 'jobs') {
    return <div className="page"><ErrorBox error={jobsState.error} retry={() => JobsStore.load()} /></div>;
  }

  return (
    <div className="page">
      <PageHeader
        title="Activity, Alerts & Audit"
        actions={[
          <Button key="drill" label="Run Restore Drill" onClick={() => createJobFromInventory('restore_drill')} tone="secondary" />,
          <Button key="backup" label="Back Up Now" onClick={() => createJobFromInventory('backup')} tone="primary" icon="play" />
        ]}
      />

      {/* 3 Hub Modes: Jobs, Alerts, Audit */}
      <SegmentedNav
        tabs={[
          { id: 'jobs', label: 'Operations & Tasks', count: allJobs.length },
          { id: 'alerts', label: 'System Alerts' },
          { id: 'audit', label: 'Cryptographic Audit Trail' }
        ]}
        activeId={hubTab}
        onSelect={(id) => setHubTab(id)}
      />

      {hubTab === 'alerts' && <AlertsPage embedded />}
      {hubTab === 'audit' && <AuditPage embedded />}

      {hubTab === 'jobs' && (
        <>
          <SegmentedNav
            tabs={[
              { id: 'all', label: 'All Operations', count: allJobs.length },
              { id: 'running', label: 'Running', count: countJobs(allJobs, 'running') },
              { id: 'failed', label: 'Failed', count: countJobs(allJobs, 'failed') },
              { id: 'completed', label: 'Completed', count: countJobs(allJobs, 'completed') }
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
                title="No Operations Found"
                text={filter === 'all' ? 'No jobs have run yet.' : `No ${filter} jobs recorded.`}
              />
            ) : (
              <DataTable
                headers={[
                  { label: 'Operation' },
                  { label: 'Target / Database' },
                  { label: 'Status' },
                  { label: 'Current Stage' },
                  { label: 'Progress' },
                  { label: 'Started' },
                  { label: 'Duration' },
                  { label: 'Actions', width: '160px' }
                ]}
              >
                {visible.map((job) => {
                  const percentage = Math.round(Math.max(0, Math.min(100, Number(job.percentage || 0))));
                  const isIndeterminate = !job.bytes_total && percentage === 0 && ['running', 'queued'].includes(job.status);
                  return (
                    <tr key={job.id}>
                      <td className="cell-primary">
                        <button
                          type="button"
                          className="link-cell"
                          onClick={() => Store.navigate(`/jobs/${job.id}`)}
                        >
                          <strong>{titleCase((job.job_type || job.type || 'job').replaceAll('_', ' '))}</strong>
                        </button>
                        <div className="text-xs text-muted cell-mono">{job.id}</div>
                      </td>
                      <td className="cell-mono text-xs">
                        {job.resource_name || job.resource_id || job.source_id || 'System'}
                      </td>
                      <td>
                        <StatusIndicator
                          label={titleCase(job.status || 'queued')}
                          tone={getJobTone(job.status)}
                        />
                      </td>
                      <td className="text-sm">
                        {titleCase((job.stage || job.status || 'queued').replaceAll('_', ' '))}
                      </td>
                      <td style={{ minWidth: '130px' }}>
                        <div className="row-between text-xs mb-xs">
                          <span>{isIndeterminate ? 'In progress' : `${percentage}%`}</span>
                          {job.bytes_processed > 0 && <span className="text-muted">{formatBytes(job.bytes_processed)}</span>}
                        </div>
                        <div
                          className={`progress-track ${isIndeterminate ? 'indeterminate' : ''}`}
                          role="progressbar"
                          aria-valuenow={isIndeterminate ? undefined : percentage}
                          aria-valuemin="0"
                          aria-valuemax="100"
                        >
                          <span style={isIndeterminate ? {} : { width: `${percentage}%` }} />
                        </div>
                      </td>
                      <td className="cell-mono text-xs">{formatRelative(job.created_at || job.started_at)}</td>
                      <td className="cell-mono text-xs">{formatJobDuration(job)}</td>
                      <td className="cell-actions">
                        <JobActions job={job} />
                      </td>
                    </tr>
                  );
                })}
              </DataTable>
            )}
          </Card>
        </>
      )}
    </div>
  );
}

function countJobs(jobs, filterType) {
  if (filterType === 'running') {
    return jobs.filter((j) => ['queued', 'running', 'cancelling'].includes(j.status)).length;
  }
  if (filterType === 'completed') {
    return jobs.filter((j) => ['completed', 'finished', 'succeeded', 'healthy', 'success'].includes(j.status)).length;
  }
  if (filterType === 'failed') {
    return jobs.filter((j) => ['failed', 'dead_letter', 'error'].includes(j.status)).length;
  }
  return jobs.filter((j) => j.status === filterType).length;
}

async function createJobFromInventory(type) {
  try {
    const inv = await API.inventory();
    const primary = inv.databases?.[0]?.id;
    if (!primary) {
      Store.toast('No database configured yet. Please configure a database first.', 'warning');
      return;
    }
    if (type === 'backup') {
      await JobsAPI.createBackup({ source_id: primary, type: 'full' });
      Store.toast('Backup job initiated', 'success');
    } else {
      await JobsAPI.createDrill({ source_id: primary });
      Store.toast('Restore drill initiated', 'success');
    }
  } catch (err) {
    Store.toast(err.message, 'danger');
  }
}

function JobDetailPage({ jobId }) {
  const [job, setJob] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const loadJob = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await JobsAPI.get(jobId);
      setJob(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadJob();
    const interval = setInterval(async () => {
      try {
        const res = await JobsAPI.get(jobId);
        if (res && res.id) setJob(res);
      } catch (_) {}
    }, 1000);
    const unsub = Store.subscribe((s) => {
      const live = s.jobs.byId[jobId];
      if (live) setJob(live);
    });
    return () => {
      clearInterval(interval);
      unsub();
    };
  }, [jobId]);

  if (loading) return <div className="page"><LoadingState label={`Loading operation ${jobId}…`} /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadJob} /></div>;
  if (!job) return <div className="page"><EmptyState title="Job Not Found" text={`Operation ${jobId} does not exist.`} /></div>;

  const isRunning = ['queued', 'running'].includes(job.status);

  return (
    <div className="page">
      <PageHeader
        title={job.name || `${titleCase((job.job_type || job.type || 'job').replaceAll('_', ' '))} (${job.id})`}
        description={`Target database: ${job.resource_name || job.resource_id || job.source_id || 'System'} · Run ID: ${job.id}`}
        actions={[
          <Button key="back" label="Back to All Jobs" onClick={() => Store.navigate('/jobs')} tone="secondary" />,
          isRunning && (
            <Button
              key="cancel"
              label="Cancel Operation"
              onClick={async () => {
                try {
                  await JobsAPI.cancel(job.id);
                  Store.toast('Operation cancelled', 'warning');
                } catch (err) {
                  Store.toast(err.message, 'danger');
                }
              }}
              tone="danger"
            />
          )
        ].filter(Boolean)}
      />

      <div className="metric-grid">
        <div className="metric-card">
          <span className="metric-label">Status</span>
          <div className="row-sm mt-xs">
            <StatusIndicator label={titleCase(job.status || 'unknown')} tone={getJobTone(job.status)} />
          </div>
        </div>
        <div className="metric-card">
          <span className="metric-label">Progress</span>
          <strong className="metric-value">{Math.round(job.percentage || 0)}%</strong>
        </div>
        <div className="metric-card">
          <span className="metric-label">Duration</span>
          <strong className="metric-value">{formatJobDuration(job)}</strong>
        </div>
        <div className="metric-card">
          <span className="metric-label">Bytes Processed</span>
          <strong className="metric-value">{job.bytes_processed ? `${Math.round(job.bytes_processed / 1024 / 1024)} MB` : '—'}</strong>
        </div>
      </div>

      <Card title="Execution Stages">
        <JobStageList job={job} />
      </Card>

      <Card title="Live Operation Logs">
        <JobLogViewer jobId={job.id} />
      </Card>
    </div>
  );
}
