import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { RecoveryTimeline } from '../components/timeline.jsx';
import { Store } from '../state.js';
import { formatDate, formatRelative, titleCase } from '../format.js';
import { ProductActions } from '../actions.js';

export function RecoveryPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState(Store.state.recoveryTab || 'time-travel');
  const [restoreReview, setRestoreReview] = useState(Store.state.restoreReview);
  const [targetTime, setTargetTime] = useState('');
  const [reason, setReason] = useState('Testing & Verification');
  const [targetEnv, setTargetEnv] = useState('Isolated Sandbox Container (Recommended)');

  const searchParams = new URLSearchParams(window.location.search);
  const paramSource = searchParams.get('source');

  const loadData = async (sourceOverride) => {
    try {
      setLoading(true);
      setError(null);
      const inventory = await API.inventory();
      const databases = inventory.databases || [];
      const currentSource = sourceOverride || paramSource || databases[0]?.id || 'production-postgres';
      const timeline = await API.timeline(currentSource).catch(() => ({
        events: [], continuous: false, gaps: [], earliest: null, latest: null
      }));
      setData({ inventory, databases, currentSource, timeline });
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData(paramSource);
    return Store.subscribe((s) => {
      setRestoreReview(s.restoreReview);
      if (s.recoveryTab) setActiveTab(s.recoveryTab);
    });
  }, [paramSource]);

  if (loading) return <div className="page"><LoadingState label="Loading recovery studio…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={() => loadData(paramSource)} /></div>;

  const { inventory, databases, currentSource, timeline } = data || {};
  const source = (databases || []).find((item) => item.id === currentSource) || {};
  const destinations = (inventory?.destinations || []).filter((item) => (source.destination_ids || []).includes(item.id));
  const drills = [...(timeline?.events || [])].reverse().filter((item) => item.type === 'restore_drill');

  const earliestISO = timeline?.earliest ? new Date(timeline.earliest).toISOString().slice(0, 16) : '';
  const latestISO = timeline?.latest ? new Date(timeline.latest).toISOString().slice(0, 16) : new Date().toISOString().slice(0, 16);

  const resource = { id: currentSource, name: source.name || 'Database' };

  const handleSourceChange = (e) => {
    const newId = e.currentTarget.value;
    Store.navigate(`/recovery?source=${encodeURIComponent(newId)}`);
    loadData(newId);
  };

  const validateAndSandbox = () => {
    if (targetTime) {
      const t = new Date(targetTime).getTime();
      if (earliestISO && t < new Date(earliestISO).getTime()) {
        Store.toast('Recovery point is before earliest available archive.', 'danger');
        return;
      }
      if (latestISO && t > new Date(latestISO).getTime()) {
        Store.toast('Recovery point is in the future.', 'danger');
        return;
      }
    }
    createSandbox(currentSource, targetTime, source.engine || 'postgres');
  };

  return (
    <div className="page">
      <PageHeader
        title="Recovery Studio"
        description="Point-in-Time Recovery (PITR), verified restore drills, and sandbox provisioning."
        actions={[
          <Button key="drill" label="Run restore drill" onClick={() => ProductActions.restoreDrill(resource)} tone="secondary" />,
          <Button key="sandbox" label="Restore to sandbox" onClick={validateAndSandbox} tone="primary" />
        ]}
      />

      <div className="source-selector-bar">
        <label className="source-selector-label">Database:</label>
        {databases.length > 0 ? (
          <select className="form-select source-select" value={currentSource} onChange={handleSourceChange}>
            {databases.map((db) => (
              <option key={db.id} value={db.id}>{db.name}</option>
            ))}
          </select>
        ) : (
          <span className="text-muted">No databases protected yet</span>
        )}
      </div>

      <SegmentedNav
        tabs={[
          { id: 'time-travel', label: 'Time-Travel Recovery & Sandbox' },
          { id: 'drills', label: 'Verified Drill Evidence', count: drills.length },
          { id: 'disaster-plan', label: 'Production Disaster Runbook' }
        ]}
        activeId={activeTab}
        onSelect={(tabId) => {
          setActiveTab(tabId);
          Store.set({ recoveryTab: tabId });
        }}
      />

      {activeTab === 'time-travel' && (
        <div className="stack-lg">
          <Card
            title="Continuous Recovery Timeline"
            action={
              <Badge
                label={timeline?.continuous ? 'Continuous Archive (0 gaps)' : `${(timeline?.gaps || []).length} gap(s) detected`}
                tone={timeline?.continuous ? 'success' : 'danger'}
              />
            }
          >
            <RecoveryTimeline data={timeline} />
          </Card>

          <Card title="Configure Restore Execution">
            <div className="form-grid">
              <div className="form-field">
                <label>Reason for restore</label>
                <select className="form-select" value={reason} onChange={(e) => setReason(e.currentTarget.value)}>
                  <option>Testing & Verification</option>
                  <option>Accidental Table / Row Deletion</option>
                  <option>Bad Application Deployment</option>
                  <option>Data Corruption Recovery</option>
                  <option>Complete Host Loss</option>
                </select>
              </div>

              <div className="form-field">
                <label>Recovery point in time (PITR)</label>
                <input
                  className="form-input"
                  type="datetime-local"
                  min={earliestISO}
                  max={latestISO}
                  value={targetTime}
                  onInput={(e) => setTargetTime(e.currentTarget.value)}
                />
                {earliestISO && (
                  <small className="text-muted">
                    Valid range: {formatDate(timeline.earliest)} → {formatDate(timeline.latest)}
                  </small>
                )}
              </div>

              <div className="form-field">
                <label>Target environment</label>
                <select className="form-select" value={targetEnv} onChange={(e) => setTargetEnv(e.currentTarget.value)}>
                  <option>Isolated Sandbox Container (Recommended)</option>
                  <option>New Database Instance</option>
                  <option>Production Replacement (Requires Approval)</option>
                </select>
              </div>
            </div>

            <div className="safe-note mt-md">
              <span>Sandbox restore creates an isolated, safe instance. Select a time within the valid recovery range above.</span>
            </div>

            <div className="row-actions mt-md">
              <Button label="Launch Sandbox Restore" onClick={validateAndSandbox} tone="primary" />
              <Button
                label="Review Production Plan"
                onClick={() => {
                  Store.set({
                    restoreReview: {
                      sourceID: currentSource,
                      targetTime: targetTime ? new Date(targetTime).toISOString() : '',
                      displayTime: targetTime ? new Date(targetTime).toLocaleString() : 'Latest verified point',
                      reason,
                      target: 'Production replacement'
                    }
                  });
                }}
                tone="secondary"
              />
            </div>
          </Card>
        </div>
      )}

      {activeTab === 'drills' && (
        <Card title="Verified Restore Drills (Audit Trail)" noPadding>
          {drills.length === 0 ? (
            <EmptyState
              title="No Drills Recorded"
              text="Run a restore drill to generate verified evidence for compliance."
            />
          ) : (
            <DataTable
              headers={[
                { label: 'Status' },
                { label: 'Drill Description' },
                { label: 'Verified At' },
                { label: 'Storage Source' },
                { label: 'Actions', width: '120px' }
              ]}
            >
              {drills.map((item, idx) => {
                const passed = item.status === 'passed';
                const drillDest = destinations.map((d) => d.name).join(', ') || 'Primary Storage';
                return (
                  <tr key={idx}>
                    <td><StatusIndicator label={passed ? 'Passed' : 'Failed'} tone={passed ? 'success' : 'danger'} /></td>
                    <td className="cell-primary">{item.description || 'Verified Sandbox Restore'}</td>
                    <td className="cell-mono">{formatRelative(item.occurred_at || item.OccurredAt)}</td>
                    <td>{drillDest}</td>
                    <td className="cell-actions">
                      <Button label="View logs" onClick={() => Store.navigate('/jobs')} tone="ghost compact" />
                    </td>
                  </tr>
                );
              })}
            </DataTable>
          )}
        </Card>
      )}

      {activeTab === 'disaster-plan' && (
        <Card title="Production Disaster Recovery Runbook">
          <div className="stack-md text-sm">
            <div className="safe-note warning">
              <span>Production restore requests overwrite live data, create an immutable audit record, and notify all project owners. Requires two-person approval.</span>
            </div>

            <div className="runbook-steps stack-md mt-md">
              <div className="runbook-step">
                <div className="runbook-step-num">1</div>
                <div className="runbook-step-body">
                  <strong>Assess the incident</strong>
                  <p>Confirm the scope of data loss, affected tables, and whether the primary host is reachable.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">2</div>
                <div className="runbook-step-body">
                  <strong>Verify recovery point</strong>
                  <p>Latest WAL point: {formatDate(timeline?.latest) || 'Unknown'}. Confirm target timestamp is valid.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">3</div>
                <div className="runbook-step-body">
                  <strong>Request two-person approval</strong>
                  <p>Submit an approval request. A second operator must confirm before live data is modified.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">4</div>
                <div className="runbook-step-body">
                  <strong>Validate in sandbox first</strong>
                  <p>Always test restore in an isolated container before modifying production.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">5</div>
                <div className="runbook-step-body">
                  <strong>Production replacement</strong>
                  <p>Once approved and verified, trigger production replacement.</p>
                </div>
              </div>
            </div>

            <div className="rto-strip row-sm mt-md">
              <div className="info-chip"><small>RTO Target</small><strong>&lt; 1 hour</strong></div>
              <div className="info-chip"><small>RPO Target</small><strong>{timeline?.continuous ? 'Seconds (Continuous WAL)' : '&lt; 24h'}</strong></div>
              <div className="info-chip"><small>Latest Point</small><strong>{formatDate(timeline?.latest)}</strong></div>
            </div>

            <div className="row-actions mt-md">
              <Button label="Validate in Sandbox First" onClick={() => createSandbox(currentSource, '', source.engine || 'postgres')} tone="secondary" />
              <Button
                label="Request Production Replacement"
                onClick={() => {
                  Store.set({
                    restoreReview: {
                      sourceID: currentSource,
                      targetTime: targetTime ? new Date(targetTime).toISOString() : '',
                      displayTime: targetTime ? new Date(targetTime).toLocaleString() : 'Latest verified point',
                      reason,
                      target: 'Production replacement'
                    }
                  });
                }}
                tone="danger"
              />
            </div>
          </div>
        </Card>
      )}

      {restoreReview && (
        <Card title="Production Restore Approval Request">
          <div className="grid-2 mb-md text-sm">
            <div><small>Source Database</small><div><strong>{restoreReview.sourceID}</strong></div></div>
            <div><small>Reason</small><div><strong>{restoreReview.reason || '—'}</strong></div></div>
            <div><small>Target Timestamp</small><div><strong>{restoreReview.displayTime}</strong></div></div>
            <div><small>Target</small><div><strong>{restoreReview.target}</strong></div></div>
          </div>
          <div className="safe-note warning">
            <span>Production restore requires verification. No live data has been modified. Submitting creates an immutable audit trail.</span>
          </div>
          <div className="row-actions mt-md">
            <Button
              label="Submit Approval Request"
              onClick={async () => {
                try {
                  const app = await API.createApproval({
                    source_id: restoreReview.sourceID,
                    reason: restoreReview.reason,
                    target_time: restoreReview.targetTime,
                    target: restoreReview.target,
                    requested_by: 'console'
                  });
                  Store.toast(`Approval request ${app.id} submitted — awaiting second operator`, 'success');
                  Store.set({ restoreReview: null });
                } catch (err) {
                  Store.toast(err.message, 'danger');
                }
              }}
              tone="primary"
            />
            <Button label="Cancel" onClick={() => Store.set({ restoreReview: null })} tone="ghost" />
          </div>
        </Card>
      )}
    </div>
  );
}

async function createSandbox(sourceID, value, engine) {
  try {
    const targetTime = value ? new Date(value).toISOString() : '';
    const sandbox = await API.createSandbox({ source_id: sourceID, snapshot_id: 'latest', engine, created_by: 'console', target_time: targetTime });
    const jobId = sandbox.job_id || sandbox.JobID;
    Store.toast(`Sandbox ${sandbox.id || sandbox.ID} is being prepared`, 'success');
    Store.navigate(jobId ? `/jobs/${jobId}` : '/jobs');
  } catch (error) {
    Store.toast(error.message, 'danger');
  }
}
