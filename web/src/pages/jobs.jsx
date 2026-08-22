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
    : allJobs.filter((job) => filter === 'running'
      ? ['queued', 'running', 'cancelling'].includes(job.status)
      : job.status === filter);

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
        description="Unified hub for live backup tasks, system health warnings, and cryptographically verified audit records."
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
                title="No Operations Found"
                text={filter === 'all' ? 'No jobs have run yet.' : `No ${filter} jobs recorded.`}
              />
            ) : (
              <DataTable
                headers={[
                  { label: 'Operation' },
                  { label: 'Status' },
                  { label: 'Current Stage' },
                  { label: 'Progress' },
                  { label: 'Started' },
                  { label: 'Duration' }
                ]}
              >
                {visible.map((job) => (
                  <JobCard key={job.id} job={job} />
                ))}
              </DataTable>
            )}
          </Card>
        </>
      )}
    </div>
  );
}

function countJobs(jobs, statuses) {
  return jobs.filter((j) => statuses.includes(j.status)).length;
}

async function createJobFromInventory(type) {
  try {
    const inv = await API.inventory();
    const primary = inv.databases?.[0]?.id || 'production-postgres';
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
        title={job.name || `${titleCase(job.type || 'Job')} (${job.id})`}
        description={`Target database: ${job.resource_id || job.source_id || 'production-postgres'} · Run ID: ${job.id}`}
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
            <StatusIndicator label={titleCase(job.status || 'unknown')} tone={job.status === 'completed' ? 'success' : job.status === 'failed' ? 'danger' : 'warning'} />
          </div>
        </div>
        <div className="metric-card">
          <span className="metric-label">Progress</span>
          <strong className="metric-value">{Math.round(job.percentage || 0)}%</strong>
        </div>
        <div className="metric-card">
          <span className="metric-label">Elapsed Time</span>
          <strong className="metric-value">{job.duration_seconds ? `${job.duration_seconds}s` : formatRelative(job.created_at)}</strong>
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
