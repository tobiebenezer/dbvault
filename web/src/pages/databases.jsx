import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { formatDate, formatRelative, titleCase } from '../format.js';

export function DatabasesPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState(Store.state.databasesTab || 'protected');

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [inventory, discoveries] = await Promise.all([
        API.inventory(),
        API.discoveries()
      ]);
      setData({ inventory, discoveries });
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const setTab = (tabId) => {
    setActiveTab(tabId);
    Store.set({ databasesTab: tabId });
  };

  if (loading) return <div className="page"><LoadingState label="Loading databases…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadData} /></div>;

  const databases = data?.inventory?.databases || [];
  const candidates = (data?.discoveries?.discoveries || []).filter((item) => item.status === 'candidate');

  return (
    <div className="page">
      <PageHeader
        title="Databases"
        description="Manage protected database instances, scheduled snapshots, and automatic host discoveries."
        actions={[
          <Button key="scan" label="Scan host" onClick={() => ProductActions.discover()} tone="secondary" />,
          <Button key="add" label="Add database" onClick={() => ProductActions.openSetupStep('discover-or-add-database')} tone="primary" />
        ]}
      />

      <SegmentedNav
        tabs={[
          { id: 'protected', label: 'Protected Databases', count: databases.length },
          { id: 'discovered', label: 'Discovered Candidates', count: candidates.length }
        ]}
        activeId={activeTab}
        onSelect={setTab}
      />

      {activeTab === 'protected' ? (
        <Card title="Protected Databases" noPadding>
          {databases.length === 0 ? (
            <EmptyState
              title="No Protected Databases"
              text="Add a database to begin continuous backup and verified recovery."
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
                { label: 'Name' },
                { label: 'Engine' },
                { label: 'Environment' },
                { label: 'Readiness' },
                { label: 'Last Backup' },
                { label: 'Last Restore Test' },
                { label: 'Storage Targets' },
                { label: 'Schedule' },
                { label: 'Actions', width: '200px' }
              ]}
            >
              {databases.map((db) => {
                const tone = db.protection === 'protected' ? 'success' : db.protection === 'unprotected' ? 'danger' : 'warning';
                const score = db.score ?? null;
                const scheduleLabel = db.backup_schedule || db.schedule || 'Not set';
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
                    <td>
                      <span title={score !== null ? `Readiness score: ${score}/100.` : 'Score not yet calculated'}>
                        <Badge label={score !== null ? `${score}/100` : 'Pending'} tone={tone} />
                      </span>
                    </td>
                    <td className="cell-mono">{formatRelative(db.last_backup_at)}</td>
                    <td className="cell-mono">{formatRelative(db.last_drill_at)}</td>
                    <td>{`${(db.destination_ids || []).length} target${(db.destination_ids || []).length === 1 ? '' : 's'}`}</td>
                    <td>{scheduleLabel}</td>
                    <td className="cell-actions cell-actions-w200">
                      <Button
                        label="Back up"
                        onClick={() => ProductActions.backup({ id: db.id, name: db.name })}
                        tone="secondary compact"
                      />
                      <Button
                        label="Configure"
                        onClick={() => Store.navigate(`/databases/${encodeURIComponent(db.id)}`)}
                        tone="ghost compact"
                      />
                    </td>
                  </tr>
                );
              })}
            </DataTable>
          )}
        </Card>
      ) : (
        <Card
          title="Discovered on this Host / Agent"
          noPadding
          action={
            <Button
              label="Scan again"
              onClick={() => ProductActions.discover()}
              tone="ghost compact"
            />
          }
        >
          {candidates.length === 0 ? (
            <EmptyState
              title="No Candidates Found"
              text="Run a host scan to discover local databases automatically."
              action={<Button label="Scan host" onClick={() => ProductActions.discover()} tone="secondary" />}
            />
          ) : (
            <DataTable
              headers={[
                { label: 'Service / Path' },
                { label: 'Engine' },
                { label: 'Location' },
                { label: 'Database Name' },
                { label: 'Status' },
                { label: 'Actions', width: '160px' }
              ]}
            >
              {candidates.map((item) => {
                const label = item.service_name || item.path || `${titleCase(item.engine || 'Database')} source`;
                const location = item.path || [item.host, item.port].filter(Boolean).join(':') || 'Local host';
                return (
                  <tr key={item.id}>
                    <td className="cell-primary"><strong>{label}</strong></td>
                    <td>{titleCase(item.engine || 'Unknown')}</td>
                    <td className="cell-mono">{location}</td>
                    <td>{item.database_name || item.dbname || '—'}</td>
                    <td><StatusIndicator label="Candidate" tone="warning" /></td>
                    <td className="cell-actions cell-actions-w160">
                      <Button
                        label="Protect"
                        onClick={async () => {
                          try {
                            await API.adoptDiscovery(item.id);
                            Store.toast('Database selected for protection', 'success');
                            await ProductActions.openSetupStep('discover-or-add-database');
                          } catch (err) {
                            Store.toast(err.message, 'danger');
                          }
                        }}
                        tone="primary compact"
                      />
                      <Button
                        label="Ignore"
                        onClick={async () => {
                          try {
                            await API.ignoreDiscovery(item.id);
                            Store.toast('Database ignored', 'success');
                            loadData();
                          } catch (err) {
                            Store.toast(err.message, 'danger');
                          }
                        }}
                        tone="ghost compact"
                      />
                    </td>
                  </tr>
                );
              })}
            </DataTable>
          )}
        </Card>
      )}
    </div>
  );
}

export function DatabaseDetailPage({ dbId }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const loadDetail = async () => {
    try {
      setLoading(true);
      setError(null);
      const [inventory, timeline] = await Promise.all([
        API.inventory(),
        API.timeline(dbId).catch(() => ({ events: [], continuous: false, gaps: [] }))
      ]);
      const db = (inventory.databases || []).find((d) => d.id === dbId);
      const destinations = (inventory.destinations || []).filter((d) => (db?.destination_ids || []).includes(d.id));
      setData({ db, inventory, timeline, destinations });
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadDetail();
  }, [dbId]);

  if (loading) return <div className="page"><LoadingState label="Loading database details…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadDetail} /></div>;

  const { db, timeline, destinations } = data || {};

  if (!db) {
    return (
      <div className="page">
        <PageHeader title="Database Not Found" description={`No database with ID "${dbId}" is currently protected.`} />
        <EmptyState
          title="Not Found"
          text="This database may have been removed or the ID is incorrect."
          action={<Button label="Back to Databases" onClick={() => Store.navigate('/databases')} tone="secondary" />}
        />
      </div>
    );
  }

  const score = db.score ?? null;
  const tone = db.protection === 'protected' ? 'success' : db.protection === 'unprotected' ? 'danger' : 'warning';

  return (
    <div className="page">
      <div className="breadcrumb-row">
        <button className="breadcrumb-link" onClick={() => Store.navigate('/databases')} type="button">
          <span>← Databases</span>
        </button>
      </div>

      <PageHeader
        title={db.name}
        description={`${titleCase(db.engine)}${db.version ? ` ${db.version}` : ''} · ${titleCase(db.environment || 'production')}`}
        actions={[
          <Button key="drill" label="Run restore drill" onClick={() => ProductActions.restoreDrill({ id: db.id, name: db.name })} tone="secondary" />,
          <Button key="backup" label="Back up now" onClick={() => ProductActions.backup({ id: db.id, name: db.name })} tone="primary" />
        ]}
      />

      <div className="metric-grid">
        <article className="metric-card">
          <div className="metric-card-label">Protection Status</div>
          <div className="metric-card-value">{titleCase(db.protection || 'Unknown')}</div>
          <div className="metric-card-footer"><span className={`status-dot ${tone}`} /></div>
        </article>
        <article className="metric-card">
          <div className="metric-card-label">Readiness Score</div>
          <div className="metric-card-value">{score !== null ? `${score}/100` : 'Pending'}</div>
          <div className="metric-card-footer">
            <span className={`status-dot ${score >= 80 ? 'success' : score >= 50 ? 'warning' : 'danger'}`} />
          </div>
        </article>
        <article className="metric-card">
          <div className="metric-card-label">Last Backup</div>
          <div className="metric-card-value">{formatRelative(db.last_backup_at)}</div>
          <div className="metric-card-footer"><span className="status-dot neutral" /></div>
        </article>
        <article className="metric-card">
          <div className="metric-card-label">Last Restore Drill</div>
          <div className="metric-card-value">{formatRelative(db.last_drill_at)}</div>
          <div className="metric-card-footer"><span className="status-dot neutral" /></div>
        </article>
      </div>

      <Card title="Connection & Schedule">
        <div className="grid-3 text-sm">
          <div><small>Engine</small><div className="mt-sm"><strong>{`${titleCase(db.engine)}${db.version ? ` ${db.version}` : ''}`}</strong></div></div>
          <div><small>Environment</small><div className="mt-sm"><strong>{titleCase(db.environment || 'production')}</strong></div></div>
          <div><small>Host / Socket</small><div className="mt-sm"><strong>{db.host ? `${db.host}${db.port ? `:${db.port}` : ''}` : db.socket || 'Agent-managed'}</strong></div></div>
          <div><small>Database Name</small><div className="mt-sm"><strong>{db.database_name || db.dbname || db.name}</strong></div></div>
          <div><small>Backup Schedule</small><div className="mt-sm"><strong>{db.backup_schedule || db.schedule || 'Continuous (WAL stream)'}</strong></div></div>
          <div><small>WAL Streaming</small><div className="mt-sm"><strong>{timeline?.continuous ? 'Active — 0 gaps' : `${(timeline?.gaps || []).length} gap(s) detected`}</strong></div></div>
        </div>
        <div className="row-actions mt-md">
          <Button label="Open Recovery Studio" onClick={() => Store.navigate(`/recovery?source=${encodeURIComponent(db.id)}`)} tone="secondary" />
          <Button label="Configure credentials" onClick={() => ProductActions.openSetupStep('discover-or-add-database')} tone="ghost" />
          <Button
            label="Unprotect database"
            onClick={() => {
              if (confirm(`Remove "${db.name}" from protection? Existing backups are preserved.`)) {
                Store.toast(`Unprotect flow for "${db.name}"`, 'warning');
                ProductActions.openSetupStep('discover-or-add-database');
              }
            }}
            tone="danger compact"
          />
        </div>
      </Card>

      <Card
        title="Recovery Timeline"
        action={
          <Button
            label="Time-Travel Restore"
            onClick={() => Store.navigate(`/recovery?source=${encodeURIComponent(db.id)}`)}
            tone="secondary compact"
          />
        }
      >
        <div className="timeline-card">
          <div className="timeline-summary">
            <span>Coverage: {timeline?.earliest ? formatDate(timeline.earliest) : 'Not available'} → Now</span>
            <Badge
              label={timeline?.continuous ? 'Continuous Archive (0 gaps)' : `${(timeline?.gaps || []).length} gap(s)`}
              tone={timeline?.continuous ? 'success' : 'danger'}
            />
          </div>
          <div className={`timeline-track ${timeline?.continuous ? 'continuous' : ''}`}>
            <div className="timeline-line" />
          </div>
        </div>
      </Card>

      <Card title="Storage Destinations" noPadding>
        {destinations.length === 0 ? (
          <EmptyState
            title="No Destinations"
            text="Assign storage destinations for this database."
            action={<Button label="Manage storage" onClick={() => Store.navigate('/repositories')} tone="secondary" />}
          />
        ) : (
          <DataTable
            headers={[
              { label: 'Destination' },
              { label: 'Provider' },
              { label: 'Region' },
              { label: 'Health' },
              { label: 'Replication Lag' }
            ]}
          >
            {destinations.map((dest) => (
              <tr key={dest.id}>
                <td className="cell-primary">{dest.name}</td>
                <td>{titleCase(dest.provider || 'S3 Compatible')}</td>
                <td className="cell-mono">{dest.region || 'default'}</td>
                <td><StatusIndicator label={titleCase(dest.status || 'healthy')} tone={dest.status === 'healthy' ? 'success' : 'warning'} /></td>
                <td>{dest.lag_seconds ? `${dest.lag_seconds}s` : '0s (In Sync)'}</td>
              </tr>
            ))}
          </DataTable>
        )}
      </Card>
    </div>
  );
}
