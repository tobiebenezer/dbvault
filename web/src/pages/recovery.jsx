import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, DataTable, SegmentedNav, StatusIndicator, EmptyState, LoadingState, ErrorBox, Icon } from '../components/ui.jsx';
import { RecoveryTimeline } from '../components/timeline.jsx';
import { Store } from '../state.js';
import { formatDate, formatRelative, formatBytes, titleCase, databaseHasBackup } from '../format.js';
import { ProductActions } from '../actions.js';

export function RecoveryPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState(Store.state.recoveryTab || 'quick-restore');
  const [restoreReview, setRestoreReview] = useState(Store.state.restoreReview);
  const [targetTime, setTargetTime] = useState('');
  const [targetDbInput, setTargetDbInput] = useState('');
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
      const currentDb = (databases || []).find(d => d.id === currentSource) || databases[0];
      if (currentDb?.name) setTargetDbInput(currentDb.name);
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
    const newDb = (databases || []).find((item) => item.id === newId);
    if (newDb?.name) setTargetDbInput(newDb.name);
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

  const hasBackup = databaseHasBackup(source);

  return (
    <div className="page">
      <PageHeader
        title="Recovery Studio"
        description="Point-in-Time Recovery, direct database restores, verified audit drills, and decrypted SQL exports."
        actions={[
          <Button
            key="sql"
            label="Download Decrypted SQL"
            onClick={() => {
              if (hasBackup) API.downloadDecryptedSQL(currentSource);
            }}
            tone="ghost"
            icon="download"
            disabled={!hasBackup}
            title={!hasBackup ? "No backup available - run a backup first" : "Download decrypted SQL snapshot"}
          />,
          <Button
            key="restore"
            label="Restore Database"
            onClick={() => {
              if (hasBackup) setShowRestoreModal(true);
            }}
            tone="primary"
            icon="shield"
            disabled={!hasBackup}
            title={!hasBackup ? "No backup available - run a backup first" : "Restore database"}
          />
        ]}
      />

      {/* Database Switcher Bar */}
      <div className="source-selector-bar">
        <label className="source-selector-label">Target Database:</label>
        {databases.length > 0 ? (
          <select className="form-select source-select" value={currentSource} onChange={handleSourceChange}>
            {databases.map((db) => (
              <option key={db.id} value={db.id}>
                {db.name} ({titleCase(db.engine)}){!databaseHasBackup(db) ? ' · No backup' : ''}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-muted">No databases protected yet</span>
        )}
      </div>

      {/* Warning banner if selected database has no backup */}
      {!hasBackup && source && (
        <div style={{
          padding: '14px 18px',
          borderRadius: '8px',
          background: 'var(--warning-soft, #fffbeb)',
          border: '1px solid var(--warning, #d97706)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '16px',
          marginTop: '6px',
          marginBottom: '6px',
        }}>
          <div>
            <div style={{ fontWeight: 600, color: 'var(--ink, #0f172a)', fontSize: '13px', marginBottom: '2px' }}>
              No Backup Available for {source.name}
            </div>
            <div style={{ fontSize: '12px', color: 'var(--muted, #64748b)' }}>
              This database has not been backed up yet. You must create at least one backup before point-in-time recovery, direct database restore, or SQL export can be performed.
            </div>
          </div>
          <Button
            label="Back Up Now"
            tone="primary"
            icon="play"
            onClick={() => ProductActions.backup({ id: source.id, name: source.name })}
          />
        </div>
      )}

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
          { id: 'quick-restore', label: 'Restore & SQL Export' },
          { id: 'drills', label: 'Audit Drills & Sandbox', count: drills.length },
          { id: 'time-travel', label: 'Continuous WAL & Timeline' },
          { id: 'masking', label: 'PII Data Masking' },
          { id: 'disaster-plan', label: 'Disaster Recovery Runbook' }
        ]}
        activeId={activeTab}
        onSelect={(tabId) => {
          setActiveTab(tabId);
          Store.set({ recoveryTab: tabId });
        }}
      />

      {activeTab === 'quick-restore' && (
        <div className="stack-md">
          <div className="grid-2" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '20px' }}>
            <Card title="Quick Database Restore" subtitle="Replay snapshot directly into original database or a new side-by-side database">
              <div className="stack-sm">
                <div className="row-between mb-sm" style={{ padding: '10px 14px', background: 'var(--panel-inset)', borderRadius: '6px' }}>
                  <span className="text-xs text-muted font-semibold">Source Snapshot</span>
                  <strong className="cell-mono text-primary font-bold">{source?.name || currentSource}</strong>
                </div>
                <div className="form-field">
                  <div className="row-between mb-xs">
                    <label className="text-xs">Target Database Name</label>
                    {targetDbInput.trim() === (source?.name || '') ? (
                      <Badge label="In-Place Restore" tone="warning" />
                    ) : (
                      <Badge label="New Side-by-Side Database" tone="success" />
                    )}
                  </div>
                  <input
                    className="form-input cell-mono"
                    value={targetDbInput}
                    onInput={(e) => setTargetDbInput(e.currentTarget.value)}
                    placeholder={source?.name || 'database'}
                  />
                  <small className="text-muted mt-xs block">
                    {targetDbInput.trim() === (source?.name || '') ? (
                      <span>Restores directly into live database <code>{source?.name}</code>.</span>
                    ) : (
                      <span>Will create and populate new database <code>{targetDbInput.trim()}</code> and register it in DBVault.</span>
                    )}
                  </small>
                  {targetDbInput.trim() !== (source?.name || '') && (
                    <div className="mt-xs">
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm"
                        style={{ fontSize: '11px', padding: '2px 8px', cursor: 'pointer' }}
                        onClick={() => setTargetDbInput(source?.name || '')}
                      >
                        ↩ Reset to original name ({source?.name})
                      </button>
                    </div>
                  )}
                </div>
                <div className="row-actions mt-md">
                  <Button
                    label={`Execute Restore → ${targetDbInput.trim() || source?.name}`}
                    onClick={() => {
                      if (hasBackup) {
                        ProductActions.restore({ id: source?.id || currentSource, name: targetDbInput.trim() || source?.name });
                      }
                    }}
                    tone="primary"
                    icon="shield"
                    disabled={!hasBackup}
                    title={!hasBackup ? "No backup available - run a backup first" : "Execute database restore"}
                  />
                  <Button
                    label="More Restore Options…"
                    onClick={() => {
                      if (hasBackup) setShowRestoreModal(true);
                    }}
                    tone="ghost"
                    disabled={!hasBackup}
                    title={!hasBackup ? "No backup available - run a backup first" : "More options"}
                  />
                </div>
              </div>
            </Card>

            <Card title="Instant Decrypted SQL Export" subtitle="Download plain SQL dump file for local inspection, DBeaver, or manual replay">
              <div className="stack-sm">
                <div style={{ padding: '12px 14px', background: 'var(--panel-inset)', borderRadius: '6px', fontSize: '12px', color: 'var(--text-muted)' }}>
                  Exports the latest verified snapshot, fully decrypted with your Master Key and decompressed into plain standard SQL.
                </div>
                <div className="row-actions mt-md">
                  <Button
                    label="Download Decrypted .sql File"
                    onClick={() => {
                      if (hasBackup) API.downloadDecryptedSQL(currentSource);
                    }}
                    tone="secondary"
                    icon="download"
                    disabled={!hasBackup}
                    title={!hasBackup ? "No backup available - run a backup first" : "Download plain SQL dump"}
                  />
                  <Button
                    label="Run Restore Drill"
                    onClick={() => {
                      if (hasBackup) ProductActions.restoreDrill(resource);
                    }}
                    tone="ghost"
                    icon="refresh"
                    disabled={!hasBackup}
                    title={!hasBackup ? "No backup available - run a backup first" : "Run automated restore drill"}
                  />
                  <Button
                    label="Launch Sandbox"
                    onClick={() => {
                      if (hasBackup) validateAndSandbox();
                    }}
                    tone="ghost"
                    icon="play"
                    disabled={!hasBackup}
                    title={!hasBackup ? "No backup available - run a backup first" : "Launch ephemeral sandbox"}
                  />
                </div>
              </div>
            </Card>
          </div>
        </div>
      )}

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
              <Button
                label="Restore to Database"
                onClick={() => {
                  if (hasBackup) setShowRestoreModal(true);
                }}
                tone="primary"
                icon="shield"
                disabled={!hasBackup}
                title={!hasBackup ? "No backup available - run a backup first" : "Restore to database"}
              />
              <Button
                label="Launch Sandbox Restore"
                onClick={() => {
                  if (hasBackup) validateAndSandbox();
                }}
                tone="secondary"
                icon="play"
                disabled={!hasBackup}
                title={!hasBackup ? "No backup available - run a backup first" : "Launch sandbox restore"}
              />
              <Button
                label="Download Plain SQL"
                onClick={() => {
                  if (hasBackup) API.downloadDecryptedSQL(currentSource);
                }}
                tone="ghost"
                icon="download"
                disabled={!hasBackup}
                title={!hasBackup ? "No backup available - run a backup first" : "Download plain SQL"}
              />
              <Button
                label={simulating ? 'Simulating…' : 'Preflight Simulation'}
                onClick={handleDryRun}
                tone="ghost"
                disabled={simulating || !hasBackup}
                title={!hasBackup ? "No backup available - run a backup first" : "Run preflight simulation"}
                icon="refresh"
              />
              <Button
                label="Production Replacement"
                disabled={!hasBackup}
                title={!hasBackup ? "No backup available - run a backup first" : "Production replacement"}
                onClick={() => {
                  if (!hasBackup) return;
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
  const originalName = database?.name || 'database';
  const [targetDb, setTargetDb] = useState(originalName);
  const [replaceExisting, setReplaceExisting] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const hasBackup = databaseHasBackup(database);

  const isOriginal = targetDb.trim() === originalName;

  const handleSubmit = (e) => {
    e.preventDefault();
    const finalTarget = targetDb.trim();
    if (!finalTarget) {
      Store.toast('Enter a target database name', 'danger');
      return;
    }
    setSubmitting(true);
    onStartRestore(finalTarget);
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
              <strong className="cell-mono">{originalName}</strong>
            </div>
            <div className="row-between text-xs mb-xs">
              <span className="text-muted">Recovery Point:</span>
              <span>{targetTime ? new Date(targetTime).toLocaleString() : 'Latest Verified Snapshot (Cloudflare R2)'}</span>
            </div>
            <div className="row-between text-xs">
              <span className="text-muted">Decryption & Compression:</span>
              <span className="row-xs" style={{ display: 'flex', gap: '6px' }}>
                <Badge label="AEAD AES-256-GCM" tone="success" />
                <Badge label="Gzip Decompress" tone="info" />
              </span>
            </div>
          </div>

          <div className="form-field">
            <div className="row-between mb-xs">
              <label>Target Database Name</label>
              {isOriginal ? (
                <Badge label="In-Place Restore" tone="warning" />
              ) : (
                <Badge label="New Side-by-Side Database" tone="success" />
              )}
            </div>
            <input
              className="form-input cell-mono"
              value={targetDb}
              onInput={(e) => setTargetDb(e.currentTarget.value)}
              placeholder={`e.g. ${originalName}`}
              required
            />
            <small className="text-muted mt-xs block">
              {isOriginal ? (
                <span>Restores directly into the original database <code>{originalName}</code>.</span>
              ) : (
                <span>Will create and restore into new database <code>{targetDb.trim()}</code> and auto-register it in DBVault.</span>
              )}
            </small>
            {!isOriginal && (
              <div className="mt-xs">
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  style={{ fontSize: '11px', padding: '2px 8px', cursor: 'pointer' }}
                  onClick={() => setTargetDb(originalName)}
                >
                  ↩ Reset to original name ({originalName})
                </button>
              </div>
            )}
          </div>

          <div className="form-field">
            <label className="checkbox-row" style={{ display: 'flex', gap: '8px', alignItems: 'center', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={replaceExisting}
                onChange={(e) => setReplaceExisting(e.currentTarget.checked)}
              />
              <span className="text-xs">Overwrite existing tables if target database already exists</span>
            </label>
          </div>

          <div className="row-actions">
            <Button type="button" label="Cancel" onClick={onClose} tone="ghost" />
            <Button
              type="submit"
              label={submitting ? "Starting Restore…" : (isOriginal ? `Execute In-Place Restore (${originalName})` : `Execute Restore (${targetDb.trim()})`)}
              tone="primary"
              icon="shield"
              disabled={submitting || !hasBackup}
            />
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
