import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, MetricCard, DataTable, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { RecoveryTimeline } from '../components/timeline.jsx';
import { formatDate, formatRelative, titleCase } from '../format.js';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';

export function OverviewPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [jobsState, setJobsState] = useState(Store.state.jobs);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [overview, inventory] = await Promise.all([
        API.overview(),
        API.inventory()
      ]);
      const databases = inventory.databases || [];
      const primaryDb = databases[0] || null;
      const timeline = primaryDb
        ? await API.timeline(primaryDb.id).catch(() => ({ events: [], continuous: false, gaps: [] }))
        : { events: [], continuous: false, gaps: [] };

      setData({ overview, inventory, timeline, primaryDb });
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    return Store.subscribe((s) => setJobsState({ ...s.jobs }));
  }, []);

  if (loading) return <div className="page"><LoadingState label="Loading overview metrics…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadData} /></div>;

  const { overview, inventory, timeline, primaryDb } = data || {};
  const summary = overview?.summary || {};
  const databases = inventory?.databases || [];
  const destinations = inventory?.destinations || [];
  const protectedCount = databases.filter((db) => db.protection === 'protected').length;
  const healthyDests = destinations.filter((d) => d.status === 'healthy').length;
  const statusTone = protectedCount === databases.length && databases.length > 0 ? 'success' : protectedCount > 0 ? 'warning' : 'danger';

  const jobs = jobsState.orderedIds.map((id) => jobsState.byId[id]).filter(Boolean);
  const latestJobs = jobs.slice(0, 5);
  const primaryResource = primaryDb ? { id: primaryDb.id, name: primaryDb.name } : null;

  return (
    <div className="page">
      <PageHeader
        title="Overview"
        description="Disaster recovery readiness, continuous archive state, and verified backup operations."
        actions={[
          <Button
            key="drill"
            label="Run restore drill"
            onClick={() => ProductActions.restoreDrill(primaryResource)}
            tone="secondary"
            disabled={!primaryResource}
          />,
          <Button
            key="backup"
            label="Back up now"
            onClick={() => ProductActions.backup(primaryResource)}
            tone="primary"
            disabled={!primaryResource}
          />
        ]}
      />

      <div className="metric-grid">
        <MetricCard
          label="Protected Databases"
          value={`${protectedCount} of ${databases.length}`}
          footerText={databases.length === 0 ? 'No databases added' : 'All systems verified'}
          statusTone={statusTone}
        />
        <MetricCard
          label="Recovery Readiness"
          value={primaryDb ? `${primaryDb.score ?? '—'} / 100` : '—'}
          footerText={timeline.continuous ? 'Continuous WAL stream active' : primaryDb ? 'Archive gap detected' : 'No databases protected'}
          statusTone={timeline.continuous ? 'success' : 'danger'}
        />
        <MetricCard
          label="Latest Restore Drill"
          value={formatRelative(primaryDb?.last_drill_at || summary.latest_verified_restore)}
          footerText="Sandbox restore verified"
          statusTone="success"
        />
        <MetricCard
          label="Storage Health"
          value={`${healthyDests} / ${destinations.length || 0}`}
          footerText={destinations.length === 0 ? 'No storage configured' : `${healthyDests} destination${healthyDests === 1 ? '' : 's'} healthy`}
          statusTone={healthyDests === destinations.length && destinations.length > 0 ? 'success' : 'warning'}
        />
      </div>

      <Card
        title="Protected Databases"
        noPadding
        action={
          <Button
            label="Add database"
            onClick={() => ProductActions.openSetupStep('discover-or-add-database')}
            tone="ghost compact"
          />
        }
      >
        {databases.length === 0 ? (
          <EmptyState
            title="No Databases Protected"
            text="Add a database to start continuous backup and recovery."
            action={
              <Button
                label="Add database"
                onClick={() => ProductActions.openSetupStep('discover-or-add-database')}
                tone="primary"
              />
            }
          />
        ) : (
          <DataTable
            headers={[
              { label: 'Database' },
              { label: 'Engine' },
              { label: 'Environment' },
              { label: 'Readiness' },
              { label: 'Last Backup' },
              { label: 'Last Restore Drill' },
              { label: 'Actions', width: '160px' }
            ]}
          >
            {databases.map((db) => {
              const dbTone = db.protection === 'protected' ? 'success' : db.protection === 'unprotected' ? 'danger' : 'warning';
              return (
                <tr key={db.id}>
                  <td className="cell-primary">
                    <button
                      className="link-cell"
                      onClick={() => Store.navigate(`/databases/${encodeURIComponent(db.id)}`)}
                      type="button"
                    >
                      <strong>{db.name}</strong>
                    </button>
                  </td>
                  <td>{`${titleCase(db.engine)}${db.version ? ` ${db.version}` : ''}`}</td>
                  <td>{titleCase(db.environment || 'production')}</td>
                  <td><Badge label={`${db.score ?? '—'}/100`} tone={dbTone} /></td>
                  <td className="cell-mono">{formatRelative(db.last_backup_at)}</td>
                  <td className="cell-mono">{formatRelative(db.last_drill_at)}</td>
                  <td className="cell-actions">
                    <Button
                      label="Back up"
                      onClick={() => ProductActions.backup({ id: db.id, name: db.name })}
                      tone="secondary compact"
                    />
                    <Button
                      label="Restore"
                      onClick={() => Store.navigate(`/recovery?source=${encodeURIComponent(db.id)}`)}
                      tone="ghost compact"
                    />
                  </td>
                </tr>
              );
            })}
          </DataTable>
        )}
      </Card>

      {primaryDb && (
        <Card
          title="Continuous Recovery Timeline"
          action={
            <Button
              label="Open Recovery Studio"
              onClick={() => Store.navigate(`/recovery?source=${encodeURIComponent(primaryDb.id)}`)}
              tone="secondary compact"
            />
          }
        >
          <RecoveryTimeline data={timeline} />
        </Card>
      )}

      <Card
        title="Recent Operations"
        noPadding
        action={
          <Button
            label="All jobs"
            onClick={() => Store.navigate('/jobs')}
            tone="ghost compact"
          />
        }
      >
        {latestJobs.length === 0 ? (
          <EmptyState
            title="No Operations Yet"
            text="Backup and restore jobs will appear here as they run."
          />
        ) : (
          <DataTable
            headers={[
              { label: 'Status' },
              { label: 'Operation' },
              { label: 'Resource' },
              { label: 'Triggered' },
              { label: 'Actions', width: '120px' }
            ]}
          >
            {latestJobs.map((job) => {
              const jobTone = job.status === 'completed' ? 'success' : job.status === 'failed' ? 'danger' : 'warning';
              return (
                <tr key={job.id}>
                  <td><StatusIndicator label={titleCase(job.status || 'queued')} tone={jobTone} /></td>
                  <td className="cell-primary">{titleCase((job.job_type || 'job').replaceAll('_', ' '))}</td>
                  <td>{job.resource_name || 'DBVault resource'}</td>
                  <td className="cell-mono">{formatRelative(job.updated_at || job.created_at)}</td>
                  <td className="cell-actions">
                    <Button
                      label="View logs"
                      onClick={() => Store.navigate(`/jobs/${job.id}`)}
                      tone="ghost compact"
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
