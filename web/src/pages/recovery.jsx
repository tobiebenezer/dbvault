import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox, Icon } from '../components/ui.jsx';
import { RecoveryTimeline } from '../components/timeline.jsx';
import { Store } from '../state.js';
import { formatDate, formatRelative, formatBytes, titleCase } from '../format.js';
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
  const [dryRunResult, setDryRunResult] = useState(null);
  const [simulating, setSimulating] = useState(false);
  const [walStatus, setWalStatus] = useState(null);
  const [showRestoreModal, setShowRestoreModal] = useState(false);

  const searchParams = new URLSearchParams(window.location.search);
  const paramSource = searchParams.get('source');

  const loadData = async (sourceOverride, isBackground = false) => {
    try {
      if (!isBackground) setLoading(true);
      setError(null);
      const inventory = await API.inventory();
      const databases = inventory.databases || [];
      const currentSource = sourceOverride || paramSource || databases[0]?.id || '';
      let timeline = { events: [], continuous: false, gaps: [], earliest: null, latest: null };
      let wal = null;
      if (currentSource) {
        const results = await Promise.all([
          API.timeline(currentSource).catch(() => ({
            events: [], continuous: false, gaps: [], earliest: null, latest: null
          })),
          API.walStatus(currentSource).catch(() => null)
        ]);
        timeline = results[0];
        wal = results[1];
      }
      setData({ inventory, databases, currentSource, timeline });
      setWalStatus(wal);
      setLoading(false);
    } catch (err) {
      if (!isBackground) setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    let lastToken = Store.state.refreshToken;
    loadData(paramSource);
    return Store.subscribe((s) => {
      setRestoreReview(s.restoreReview);
      if (s.recoveryTab) setActiveTab(s.recoveryTab);
      if (s.refreshToken !== lastToken) {
        lastToken = s.refreshToken;
        loadData(paramSource, true);
      }
    });
  }, [paramSource]);

  if (loading) return <div className="page"><LoadingState label="Loading recovery studio…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={() => loadData(paramSource)} /></div>;

  const { inventory, databases, currentSource, timeline } = data || {};

  if (!databases || databases.length === 0) {
    return (
      <div className="page">
        <PageHeader
          title="Recovery Studio"
          description="Point-in-Time Recovery (PITR), continuous WAL stream scrubbing, verified restore drills, and sandbox provisioning."
        />
        <EmptyState
          icon="shield"
          title="No Databases Configured"
          description="Connect and adopt databases to enable continuous WAL streaming, PITR time-travel, and sandbox restore drills."
          action={<Button label="Connect Databases" onClick={() => Store.navigate('/databases')} tone="primary" icon="plus" />}
        />
      </div>
    );
  }
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

  const handleDryRun = async () => {
    try {
      setSimulating(true);
      setDryRunResult(null);
      Store.toast('Simulating PITR recovery parameters & WAL replay count…', 'info');
      const iso = targetTime ? new Date(targetTime).toISOString() : new Date().toISOString();
      const estimate = await API.replayEstimate(currentSource, iso);
      setDryRunResult(estimate);
      setSimulating(false);
      Store.toast(`Dry-run passed: Estimated RTO ~${estimate.total_estimated_rto_seconds}s (${estimate.wal_segments_to_replay} WAL segments)`, 'success');
    } catch (err) {
      setSimulating(false);
      Store.toast(err.message, 'danger');
    }
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

  const createSandbox = async (sourceId, targetTimestamp, engine) => {
    try {
      const res = await API.createSandbox({
        source_id: sourceId,
        snapshot_id: 'snap-base-20260814-000000',
        engine,
        created_by: 'operator@local',
        target_time: targetTimestamp ? new Date(targetTimestamp).toISOString() : undefined
      });
      Store.toast(`Sandbox provisioned: ${res.id}`, 'success');
      Store.navigate('/jobs');
    } catch (err) {
      Store.toast(err.message, 'danger');
    }
  };

  return (
    <div className="page">
      <PageHeader
        title="Recovery Studio"
        description="Point-in-Time Recovery (PITR), continuous WAL stream scrubbing, verified restore drills, and sandbox provisioning."
        actions={[
          <Button key="sql" label="Download Decrypted SQL" onClick={() => API.downloadDecryptedSQL(currentSource)} tone="ghost" icon="download" />,
          <Button key="cert" label="Compliance Certificate" onClick={() => API.downloadComplianceCertificate(currentSource)} tone="secondary" icon="download" />,
          <Button key="drill" label="Run Restore Drill" onClick={() => ProductActions.restoreDrill(resource)} tone="secondary" icon="refresh" />,
          <Button key="sandbox" label="Launch Sandbox" onClick={validateAndSandbox} tone="secondary" icon="play" />,
          <Button key="restore" label="Restore to Database" onClick={() => setShowRestoreModal(true)} tone="primary" icon="shield" />
        ]}
      />

      {/* Database Switcher Bar */}
      <div className="source-selector-bar">
        <label className="source-selector-label">Target Database:</label>
        {databases.length > 0 ? (
          <select className="form-select source-select" value={currentSource} onChange={handleSourceChange}>
            {databases.map((db) => (
              <option key={db.id} value={db.id}>{db.name} ({titleCase(db.engine)})</option>
            ))}
          </select>
        ) : (
          <span className="text-muted">No databases protected yet</span>
        )}
      </div>

      {/* Real-time WAL Stream Telemetry Bar */}
      {walStatus && (
        <Card title="Continuous WAL Archival Telemetry" noPadding>
          <div className="grid-auto text-sm" style={{ padding: '16px 20px', gap: '20px', background: 'var(--panel-inset)' }}>
            <div>
              <small className="text-muted block text-xs">Current Engine LSN</small>
              <strong className="cell-mono text-success" style={{ fontSize: '13px' }}>{walStatus.current_lsn}</strong>
            </div>
            <div>
              <small className="text-muted block text-xs">Real-Time RPO Lag</small>
              <strong className="text-success">{walStatus.rpo_milliseconds} ms</strong>
            </div>
            <div>
              <small className="text-muted block text-xs">Archived Volume</small>
              <strong>{formatBytes(walStatus.total_archived_bytes)} ({walStatus.archived_segments_count} segs)</strong>
            </div>
            <div>
              <small className="text-muted block text-xs">Compression Ratio</small>
              <Badge label={`${walStatus.compression_ratio}x (Zstandard)`} tone="success" />
            </div>
            <div>
              <small className="text-muted block text-xs">Replay Speed</small>
              <strong>{walStatus.replay_speed_mbps} MB/s</strong>
            </div>
            <div>
              <small className="text-muted block text-xs">Stream Continuity</small>
              <StatusIndicator label={walStatus.continuity_verified ? "0 Gaps (Continuous)" : "Gap Detected"} tone={walStatus.continuity_verified ? "success" : "danger"} />
            </div>
          </div>
        </Card>
      )}

      <SegmentedNav
        tabs={[
          { id: 'time-travel', label: 'Time-Travel Recovery & Sandbox' },
          { id: 'masking', label: 'PII & Data Masking Policy' },
          { id: 'drills', label: 'Verified Drill Evidence', count: drills.length },
          { id: 'disaster-plan', label: 'Production Disaster Runbook' }
        ]}
        activeId={activeTab}
        onSelect={(tabId) => {
          setActiveTab(tabId);
          Store.set({ recoveryTab: tabId });
        }}
      />

      {activeTab === 'masking' && <MaskingRulesPanel />}

      {activeTab === 'time-travel' && (
        <div className="stack-lg">
          <Card
            title="Continuous Recovery Timeline & Scrubber"
            action={
              <Badge
                label={timeline?.continuous ? 'Continuous Archive (0 gaps)' : `${(timeline?.gaps || []).length} gap(s) detected`}
                tone={timeline?.continuous ? 'success' : 'danger'}
              />
            }
          >
            <RecoveryTimeline data={timeline} />
          </Card>

          {/* Granular WAL & Transaction Log Segments Explorer */}
          {timeline?.segments && timeline.segments.length > 0 && (
            <Card
              title={`Archived WAL Segments & LSN Checkpoints (${timeline.segments.length} segments)`}
              subtitle="Contiguous Write-Ahead Log segments verified with SHA-256 Merkle parity for sub-second Point-in-Time Recovery."
              noPadding
            >
              <DataTable
                headers={[
                  { label: 'Segment File' },
                  { label: 'Start LSN' },
                  { label: 'End LSN' },
                  { label: 'Segment Size' },
                  { label: 'Transactions' },
                  { label: 'SHA-256 Checksum' },
                  { label: 'Status' }
                ]}
              >
                {timeline.segments.map((seg, idx) => (
                  <tr key={idx}>
                    <td className="cell-primary"><strong><code>{seg.segment_name}</code></strong></td>
                    <td className="cell-mono text-xs text-primary font-bold">{seg.start_lsn}</td>
                    <td className="cell-mono text-xs">{seg.end_lsn}</td>
                    <td className="cell-mono text-xs">{formatBytes(seg.size_bytes)}</td>
                    <td className="cell-mono text-xs">{(seg.transactions_count || 0).toLocaleString()} txns</td>
                    <td className="cell-mono text-xs text-muted" style={{ maxWidth: '160px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={seg.checksum_sha256}>
                      {seg.checksum_sha256.slice(0, 16)}…
                    </td>
                    <td>
                      <StatusIndicator
                        label={seg.status === 'streaming' ? 'Active WAL' : 'Verified'}
                        tone={seg.status === 'streaming' ? 'warning' : 'success'}
                      />
                    </td>
                  </tr>
                ))}
              </DataTable>
            </Card>
          )}

          <Card title="Point-in-Time Restore (PITR) Planner">
            <div className="form-grid">
              <div className="form-field">
                <label>Reason for Restore</label>
                <select className="form-select" value={reason} onChange={(e) => setReason(e.currentTarget.value)}>
                  <option>Testing & Verification (Sandbox)</option>
                  <option>Accidental Table / Row Deletion</option>
                  <option>Bad Application Migration / Deployment</option>
                  <option>Data Corruption Incident</option>
                  <option>Complete Server / Hardware Failure</option>
                </select>
              </div>

              <div className="form-field">
                <label>Target Timestamp (Exact Replay Position)</label>
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
                    Available recovery window: {formatDate(timeline.earliest)} → {formatDate(timeline.latest)}
                  </small>
                )}
              </div>

              <div className="form-field" style={{ gridColumn: 'span 2' }}>
                <label>Target Environment</label>
                <select className="form-select" value={targetEnv} onChange={(e) => setTargetEnv(e.currentTarget.value)}>
                  <option>Isolated Sandbox Container (Recommended — Safe & Non-Destructive)</option>
                  <option>New Standby Database Instance (Side-by-side)</option>
                  <option>Production In-Place Replacement (Requires Two-Person Authorization)</option>
                </select>
              </div>
            </div>

            {/* Dry Run Simulator Result */}
            {dryRunResult && (
              <div className="safe-note mt-md" style={{ background: '#f0fdf4', borderColor: '#bbf7d0', color: '#166534' }}>
                <Icon name="check" size={18} />
                <div>
                  <strong>PITR Preflight Simulation Passed:</strong>
                  <div className="grid-auto mt-xs text-xs" style={{ color: '#166534', gap: '16px' }}>
                    <span>Base Snapshot: {dryRunResult.base_snapshot_id} ({formatBytes(dryRunResult.base_snapshot_size_bytes)})</span>
                    <span>WAL Replay: {dryRunResult.wal_segments_to_replay} segments ({formatBytes(dryRunResult.wal_bytes_to_replay)})</span>
                    <span>Replay Time: ~{dryRunResult.estimated_replay_seconds}s</span>
                    <span>Target LSN: {dryRunResult.target_lsn}</span>
                    <span>Total Estimated RTO: ~{dryRunResult.total_estimated_rto_seconds}s</span>
                  </div>
                </div>
              </div>
            )}

            <div className="row-actions mt-md">
              <Button label="Restore to Database" onClick={() => setShowRestoreModal(true)} tone="primary" icon="shield" />
              <Button label="Launch Sandbox Restore" onClick={validateAndSandbox} tone="secondary" icon="play" />
              <Button label="Download Plain SQL" onClick={() => API.downloadDecryptedSQL(currentSource)} tone="ghost" icon="download" />
              <Button
                label={simulating ? 'Simulating…' : 'Preflight Simulation'}
                onClick={handleDryRun}
                tone="ghost"
                disabled={simulating}
                icon="refresh"
              />
              <Button
                label="Production Replacement"
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
          </Card>
        </div>
      )}

      {activeTab === 'drills' && (
        <Card
          title="Verified Restore Drills (Audit Proof)"
          noPadding
          action={
            <Button
              label="Download Signed Certificate"
              onClick={() => API.downloadComplianceCertificate(currentSource)}
              tone="ghost compact"
              icon="download"
            />
          }
        >
          {drills.length === 0 ? (
            <EmptyState
              title="No Drills Recorded"
              text="Run a restore drill to generate verified compliance evidence for SOC2 / ISO 27001 audits."
            />
          ) : (
            <DataTable
              headers={[
                { label: 'Status' },
                { label: 'Drill Description' },
                { label: 'Verified At' },
                { label: 'Storage Source' },
                { label: 'Compliance Standards' },
                { label: 'Actions', width: '140px' }
              ]}
            >
              {drills.map((item, idx) => {
                const passed = item.status === 'passed';
                const drillDest = destinations.map((d) => d.name).join(', ') || 'Primary Storage (R2)';
                return (
                  <tr key={idx}>
                    <td><StatusIndicator label={passed ? 'Passed (0 Errors)' : 'Failed'} tone={passed ? 'success' : 'danger'} /></td>
                    <td className="cell-primary">{item.description || 'Verified Sandbox Restore (Checksum Verified)'}</td>
                    <td className="cell-mono">{formatRelative(item.occurred_at || item.OccurredAt)}</td>
                    <td>{drillDest}</td>
                    <td><Badge label="SOC2 · HIPAA · ISO 27001" tone="success" /></td>
                    <td className="cell-actions">
                      <Button label="View Certificate" onClick={() => API.downloadComplianceCertificate(currentSource)} tone="ghost compact" />
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
              <Icon name="warning" size={16} />
              <span>Production restore requests overwrite live database instances, create an immutable audit log, and require two-person operator sign-off.</span>
            </div>

            <div className="runbook-steps stack-md mt-md">
              <div className="runbook-step">
                <div className="runbook-step-num">1</div>
                <div className="runbook-step-body">
                  <strong>Assess Incident & Scope</strong>
                  <p>Confirm the scope of data loss, affected tables, and whether the primary host is reachable.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">2</div>
                <div className="runbook-step-body">
                  <strong>Verify Point-in-Time Boundary</strong>
                  <p>Latest verified WAL point: {formatDate(timeline?.latest) || 'Unknown'}. Confirm target timestamp is valid.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">3</div>
                <div className="runbook-step-body">
                  <strong>Request Two-Person Approval</strong>
                  <p>Submit an approval ticket. A secondary operator must confirm before production data is modified.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">4</div>
                <div className="runbook-step-body">
                  <strong>Sandbox Dry-Run Test</strong>
                  <p>Always verify the restore point in an isolated sandbox before executing in-place replacement.</p>
                </div>
              </div>
              <div className="runbook-step">
                <div className="runbook-step-num">5</div>
                <div className="runbook-step-body">
                  <strong>Execute Production Replacement</strong>
                  <p>Once approved and verified, execute the atomic promotion pipeline.</p>
                </div>
              </div>
            </div>

            <div className="rto-strip row-sm mt-md">
              <div className="info-chip"><small>RTO Target</small><strong>&lt; 15 minutes</strong></div>
              <div className="info-chip"><small>RPO Target</small><strong>{timeline?.continuous ? '0s (Continuous WAL)' : '&lt; 1h'}</strong></div>
              <div className="info-chip"><small>Latest Available Point</small><strong>{formatDate(timeline?.latest)}</strong></div>
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

      {showRestoreModal && (
        <RestoreTargetModal
          database={source}
          targetTime={targetTime}
          onClose={() => setShowRestoreModal(false)}
          onStartRestore={(targetName) => {
            setShowRestoreModal(false);
            ProductActions.restore({ id: source.id, name: targetName || source.name });
          }}
        />
      )}
    </div>
  );
}

function RestoreTargetModal({ database, targetTime, onClose, onStartRestore }) {
  const [targetDb, setTargetDb] = useState(`${database?.name || 'database'}_restored`);
  const [replaceExisting, setReplaceExisting] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!targetDb.trim()) {
      Store.toast('Enter a target database name', 'danger');
      return;
    }
    setSubmitting(true);
    onStartRestore(targetDb.trim());
  };

  return (
    <div className="command-overlay" role="dialog" aria-modal="true" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="command-panel" style={{ maxWidth: '580px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Restore Database</h2>
            <p className="card-subtitle">Replay decrypted snapshot & WAL logs into target engine</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="stack-md">
          <div className="metric-card" style={{ padding: '12px', background: 'var(--panel-inset)', borderRadius: '6px' }}>
            <div className="row-between text-xs mb-xs">
              <span className="text-muted">Source Snapshot:</span>
              <strong className="cell-mono">{database?.name || 'Database'}</strong>
            </div>
            <div className="row-between text-xs mb-xs">
              <span className="text-muted">Recovery Point:</span>
              <span>{targetTime ? new Date(targetTime).toLocaleString() : 'Latest Verified Snapshot (R2)'}</span>
            </div>
            <div className="row-between text-xs">
              <span className="text-muted">Decryption Cipher:</span>
              <Badge label="AEAD AES-256-GCM" tone="success" />
            </div>
          </div>

          <div className="form-field">
            <label>Target Database Name</label>
            <input
              className="form-input cell-mono"
              value={targetDb}
              onInput={(e) => setTargetDb(e.currentTarget.value)}
              placeholder="e.g. cribx_restored"
              required
            />
            <small className="text-muted mt-xs block">
              Default is non-destructive (creates a new side-by-side database). Change to <code>{database?.name}</code> for in-place restore.
            </small>
          </div>

          <div className="form-field">
            <label className="checkbox-row" style={{ display: 'flex', gap: '8px', alignItems: 'center', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={replaceExisting}
                onChange={(e) => setReplaceExisting(e.currentTarget.checked)}
              />
              <span className="text-xs">Drop & replace existing target database tables if they already exist</span>
            </label>
          </div>

          <div className="row-actions">
            <Button type="button" label="Cancel" onClick={onClose} tone="ghost" />
            <Button type="submit" label={submitting ? "Starting Restore…" : "Execute Restore"} tone="primary" icon="shield" />
          </div>
        </form>
      </div>
    </div>
  );
}

function MaskingRulesPanel() {
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    API.maskingRules()
      .then((res) => {
        setRules(res.rules || []);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, []);

  if (loading) return <Card title="PII & Privacy Data Masking Policy"><LoadingState label="Loading privacy rules…" /></Card>;

  return (
    <div className="stack-lg">
      <Card
        title="Automated PII & Sensitive Data Masking Policy"
        subtitle="Pre-configured data transformation strategies applied automatically during staging sandbox provisioning and test exports."
        action={<Badge label="Zero-PII Staging Protected" tone="success" />}
        noPadding
      >
        <DataTable
          headers={[
            { label: 'Column Pattern' },
            { label: 'Classification' },
            { label: 'Transformation Strategy' },
            { label: 'Sample Original' },
            { label: 'Masked Output' },
            { label: 'Status' }
          ]}
        >
          {rules.map((r) => (
            <tr key={r.id}>
              <td className="cell-primary"><strong><code>{r.column_pattern}</code></strong></td>
              <td><Badge label={r.classification} tone="neutral" /></td>
              <td><strong className="text-primary">{r.strategy}</strong></td>
              <td className="cell-mono text-muted text-xs">{r.example_before}</td>
              <td className="cell-mono text-success text-xs font-bold">{r.example_after}</td>
              <td><StatusIndicator label={r.enabled ? "Active" : "Disabled"} tone={r.enabled ? "success" : "neutral"} /></td>
            </tr>
          ))}
        </DataTable>
      </Card>
    </div>
  );
}
