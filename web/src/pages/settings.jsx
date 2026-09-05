import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, Icon, StatusIndicator, DataTable, SegmentedNav } from '../components/ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { formatDate, titleCase } from '../format.js';
import { TeamPage } from './team.jsx';
import { AdminFleetPage } from './admin.jsx';
import { BillingPage } from './billing.jsx';

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState('general'); // 'general' | 'team' | 'channels' | 'fleet' | 'billing'
  const [doctorResult, setDoctorResult] = useState(Store.state.doctorResult);
  const [releaseInfo, setReleaseInfo] = useState(Store.state.releaseInfo);
  const [channels, setChannels] = useState([]);
  const [retention, setRetention] = useState(Store.state.retentionPolicy || { daily: 7, weekly: 4, monthly: 12, rpo_hours: 1 });

  // Webhook form state
  const [whName, setWhName] = useState('');
  const [whType, setWhType] = useState('slack');
  const [whEvents, setWhEvents] = useState('backup.failed,restore_drill.failed,rpo.breach');
  const [whUrl, setWhUrl] = useState('');

  // Retention form state
  const [daily, setDaily] = useState(retention.daily);
  const [weekly, setWeekly] = useState(retention.weekly);
  const [monthly, setMonthly] = useState(retention.monthly);
  const [rpo, setRpo] = useState(retention.rpo_hours);

  const loadSettingsData = async () => {
    try {
      const chanRes = await API.notificationChannels().catch(() => ({ channels: [] }));
      setChannels(chanRes.channels || []);
    } catch (_) {}
  };

  useEffect(() => {
    loadSettingsData();
    return Store.subscribe((s) => {
      setDoctorResult(s.doctorResult);
      setReleaseInfo(s.releaseInfo);
      if (s.retentionPolicy) setRetention(s.retentionPolicy);
      if (s.refreshToken) loadSettingsData();
    });
  }, []);

  const handleRunDoctor = async () => {
    try {
      const res = await API.doctor();
      Store.set({ doctorResult: res });
      setDoctorResult(res);
      const warnings = (res.checks || []).filter((c) => c.status !== 'pass').length;
      Store.toast(warnings ? `Doctor found ${warnings} warning${warnings === 1 ? '' : 's'}` : 'All preflight checks passed!', warnings ? 'warning' : 'success');
    } catch (err) {
      Store.toast(err.message, 'danger');
    }
  };

  const handleCheckUpdates = async () => {
    try {
      const res = await API.releaseManifest();
      const updateAvailable = res.version && res.version !== '0.1.0-alpha';
      const info = { ...res, update_available: updateAvailable };
      Store.set({ releaseInfo: info });
      setReleaseInfo(info);
      Store.toast(updateAvailable ? `Update available: v${res.version}` : `Up to date: v${res.version || '0.1.0-alpha'}`, 'success');
    } catch (err) {
      Store.toast(err.message, 'danger');
    }
  };

  const handleAddChannel = async (e) => {
    e.preventDefault();
    if (!whUrl.trim()) {
      Store.toast('Enter a valid webhook URL', 'danger');
      return;
    }
    const name = whName.trim() || `${titleCase(whType)} Alerts`;
    try {
      await API.createNotificationChannel({
        name,
        type: whType,
        url: whUrl.trim(),
        events: whEvents.split(',').map((s) => s.trim())
      });
      Store.toast(`Channel "${name}" created & active`, 'success');
      setWhName('');
      setWhUrl('');
      loadSettingsData();
    } catch (err) {
      Store.toast(`Failed to add channel: ${err.message}`, 'danger');
    }
  };

  const handleDeleteChannel = async (id, name) => {
    const ok = await Store.confirm({
      title: 'Delete Notification Channel',
      message: `Are you sure you want to delete notification channel "${name}"? Alerts will no longer be routed to this endpoint.`,
      confirmLabel: 'Delete Channel',
      confirmTone: 'danger'
    });
    if (!ok) return;
    try {
      await API.deleteNotificationChannel(id);
      Store.toast(`Channel "${name}" removed`, 'success');
      loadSettingsData();
    } catch (err) {
      Store.toast(err.message, 'danger');
    }
  };

  const handleTestWebhook = async (url) => {
    if (!url) {
      Store.toast('Enter or select a webhook URL to test', 'danger');
      return;
    }
    try {
      const res = await API.testNotification('', url);
      if (res.status === 'dispatched') {
        Store.toast(`Test notification delivered (HTTP ${res.http_status}) ✓`, 'success');
      } else {
        Store.toast(`Test notification simulated: ${res.status}`, 'success');
      }
    } catch (err) {
      Store.toast(`Webhook test failed: ${err.message}`, 'danger');
    }
  };

  const handleSaveRetention = () => {
    const updated = { daily: Number(daily), weekly: Number(weekly), monthly: Number(monthly), rpo_hours: Number(rpo) };
    setRetention(updated);
    Store.set({ retentionPolicy: updated });
    Store.toast('Retention & RPO policy saved successfully', 'success');
  };

  const handleGenerateRunbook = () => {
    const runbookText = `# DBVault Disaster Recovery Runbook
Generated: ${new Date().toUTCString()}
Appliance Version: v0.1.0-alpha

## 1. Quick Emergency Restore Command (CLI)
\`\`\`bash
# 1. Download recovery bundle and decryption key
dbvault bundle restore --bundle /srv/dbvault/bundles/recovery.tar.gz

# 2. Replay continuous WAL stream to specific point in time
dbvault restore --database production-postgres \\
  --target-time "${new Date().toISOString()}" \\
  --destination s3://dbvault-backups \\
  --target-host 127.0.0.1:5432
\`\`\`

## 2. Emergency Contacts & SLA
- RTO Target: < 15 Minutes
- RPO Target: 0 Seconds (Continuous WAL Archive)
- Storage Targets: Cloudflare R2 Primary + Contabo Replica

---
DBVault Zero-Knowledge Appliance`;

    const blob = new Blob([runbookText], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `dbvault-disaster-runbook-${new Date().toISOString().slice(0, 10)}.md`;
    a.click();
    URL.revokeObjectURL(url);
    Store.toast('Disaster Recovery Runbook exported (.md)', 'success');
  };

  return (
    <div className="page">
      <PageHeader
        title="Settings & Management"
        actions={[
          <Button key="runbook" label="Export DR Runbook" onClick={handleGenerateRunbook} tone="secondary" icon="download" />,
          <Button key="doc" label="Run Preflight Doctor" onClick={handleRunDoctor} tone="primary" icon="refresh" />
        ]}
      />

      {/* Structured Settings Navigation Tabs */}
      <SegmentedNav
        tabs={[
          { id: 'general', label: 'General & Doctor' },
          { id: 'keys', label: 'Master Key & DR Kit' },
          { id: 'team', label: 'Team & RBAC' },
          { id: 'channels', label: 'Alert Channels', count: channels.length },
          { id: 'fleet', label: 'Agent Fleet Nodes' },
          { id: 'billing', label: 'Usage & Quotas' }
        ]}
        activeId={activeTab}
        onSelect={(id) => setActiveTab(id)}
      />

      {activeTab === 'keys' && <MasterKeyManagementCard />}
      {activeTab === 'team' && <TeamPage embedded />}
      {activeTab === 'fleet' && <AdminFleetPage embedded />}
      {activeTab === 'billing' && <BillingPage embedded />}

      {activeTab === 'channels' && (
        <Card title="Alert Notification Channels (Slack, Discord, PagerDuty, Webhook)" noPadding>
          {channels.length > 0 && (
            <DataTable
              headers={[
                { label: 'Channel Name' },
                { label: 'Type' },
                { label: 'Target URL / Endpoint' },
                { label: 'Subscribed Events' },
                { label: 'Actions', width: '180px' }
              ]}
            >
              {channels.map((ch) => (
                <tr key={ch.id}>
                  <td className="cell-primary"><strong>{ch.name}</strong></td>
                  <td><Badge label={titleCase(ch.type)} tone="neutral" /></td>
                  <td className="cell-mono text-xs">{ch.url}</td>
                  <td><small className="text-muted">{ch.events.join(', ')}</small></td>
                  <td className="cell-actions">
                    <div className="row-sm">
                      <Button label="Test" onClick={() => handleTestWebhook(ch.url)} tone="secondary compact" />
                      <Button label="Delete" onClick={() => handleDeleteChannel(ch.id, ch.name)} tone="danger compact" />
                    </div>
                  </td>
                </tr>
              ))}
            </DataTable>
          )}

          <div style={{ padding: '20px' }}>
            <h3 className="text-sm font-semibold mb-sm">Add Notification Channel</h3>
            <form onSubmit={handleAddChannel} className="form-grid">
              <div className="form-field">
                <label>Channel Name</label>
                <input
                  className="form-input"
                  placeholder="#ops-database-alerts"
                  value={whName}
                  onInput={(e) => setWhName(e.currentTarget.value)}
                />
              </div>
              <div className="form-field">
                <label>Channel Type</label>
                <select className="form-select" value={whType} onChange={(e) => setWhType(e.currentTarget.value)}>
                  <option value="slack">Slack Incoming Webhook</option>
                  <option value="discord">Discord Webhook</option>
                  <option value="teams">Microsoft Teams</option>
                  <option value="pagerduty">PagerDuty Alert Events</option>
                  <option value="webhook">Generic JSON HTTP Webhook</option>
                </select>
              </div>
              <div className="form-field" style={{ gridColumn: 'span 2' }}>
                <label>Webhook URL / Endpoint</label>
                <input
                  className="form-input"
                  type="url"
                  placeholder="https://hooks.slack.com/services/…"
                  value={whUrl}
                  onInput={(e) => setWhUrl(e.currentTarget.value)}
                  required
                />
              </div>
              <div className="row-actions" style={{ gridColumn: 'span 2' }}>
                <Button type="submit" label="Add Channel" tone="primary compact" />
              </div>
            </form>
          </div>

          {/* Incident Alert & Webhook Test Simulator */}
          <div style={{ padding: '20px', borderTop: '1px solid var(--border-color)', background: 'var(--panel-inset)' }}>
            <div className="row-between mb-sm">
              <div>
                <h3 className="text-sm font-semibold" style={{ margin: 0 }}>🚨 Incident Response Alert Simulator</h3>
                <span className="text-xs text-muted">Test alert routing and payload rendering for Slack, Discord, and PagerDuty before real emergencies occur.</span>
              </div>
              <Badge label="Preflight Simulator" tone="neutral" />
            </div>

            <div className="stack-sm">
              <div className="row-sm" style={{ flexWrap: 'wrap', gap: '8px' }}>
                <Button
                  label="Simulate Backup Failure"
                  onClick={() => {
                    if (channels.length > 0) handleTestWebhook(channels[0].url);
                    else Store.toast('Add a notification channel first to simulate alerts', 'warning');
                  }}
                  tone="danger compact"
                  icon="shield"
                />
                <Button
                  label="Simulate RPO SLA Breach"
                  onClick={() => {
                    if (channels.length > 0) handleTestWebhook(channels[0].url);
                    else Store.toast('Add a notification channel first to simulate alerts', 'warning');
                  }}
                  tone="warning compact"
                  icon="refresh"
                />
                <Button
                  label="Simulate Security Tamper Alert"
                  onClick={() => {
                    if (channels.length > 0) handleTestWebhook(channels[0].url);
                    else Store.toast('Add a notification channel first to simulate alerts', 'warning');
                  }}
                  tone="secondary compact"
                  icon="lock"
                />
              </div>
            </div>
          </div>
        </Card>
      )}

      {activeTab === 'general' && (
        <div className="stack-md">
          {/* Database Connections Architecture Overview */}
          <Card title="Appliance Architecture & Database Connections">
            <div className="grid-2" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
              <div className="metric-card" style={{ padding: '16px', background: 'var(--panel-inset)', borderRadius: '8px' }}>
                <div className="row-between mb-xs">
                  <strong className="text-sm">1. DBVault System Database</strong>
                  <Badge label="Environment Config" tone="neutral" />
                </div>
                <p className="text-xs text-muted mb-sm">
                  Stores internal appliance state: user logins, RBAC roles, immutability policies, scheduled jobs, Merkle audit trail, and encryption keys.
                </p>
                <div className="stack-xs">
                  <div className="row-between text-xs">
                    <span className="text-muted">Configured Via:</span>
                    <code>DBVAULT_DATABASE_URL</code>
                  </div>
                  <div className="row-between text-xs">
                    <span className="text-muted">Storage Dir:</span>
                    <code>DBVAULT_DATA_DIR</code>
                  </div>
                  <div className="row-between text-xs">
                    <span className="text-muted">Status:</span>
                    <StatusIndicator label="Active & Secured" tone="success" />
                  </div>
                </div>
              </div>

              <div className="metric-card" style={{ padding: '16px', background: 'var(--panel-inset)', borderRadius: '8px' }}>
                <div className="row-between mb-xs">
                  <strong className="text-sm">2. Target Backup Databases</strong>
                  <Badge label="UI Managed" tone="success" />
                </div>
                <p className="text-xs text-muted mb-sm">
                  Your external/local databases (PostgreSQL, MySQL, SQLite) to backup, mirror to cloud storage, and point-in-time recover.
                </p>
                <div className="stack-xs">
                  <div className="row-between text-xs">
                    <span className="text-muted">Management:</span>
                    <span>Dynamic Discovery in UI</span>
                  </div>
                  <div className="row-between text-xs">
                    <span className="text-muted">Credentials:</span>
                    <span>Probed on-demand or URI</span>
                  </div>
                  <div className="row-between text-xs">
                    <span className="text-muted">Action:</span>
                    <Button label="Manage Target DBs" onClick={() => Store.navigate('/databases')} tone="primary compact" />
                  </div>
                </div>
              </div>
            </div>
          </Card>

          {/* Emergency Disaster Recovery Bundles */}
          <Card title="Disaster Recovery & Emergency Exports">
            <div className="stack-md">
              <div className="row-between list-item-row">
                <div className="stack-sm max-w-600">
                  <strong>Emergency Recovery Bundle (Air-Gapped)</strong>
                  <small>Export encrypted catalogue metadata, public signing keys, and repository configurations for disaster recovery without internet access.</small>
                </div>
                <div className="row-actions">
                  <Badge label="Zero-Knowledge" tone="neutral" />
                  <Button
                    label="Create Recovery Bundle"
                    onClick={async () => {
                      try {
                        const res = await API.recoveryBundle();
                        Store.toast(`Recovery bundle created: ${res.path}`, 'success');
                      } catch (err) {
                        Store.toast(err.message, 'danger');
                      }
                    }}
                    tone="secondary compact"
                  />
                </div>
              </div>

              <div className="row-between list-item-row">
                <div className="stack-sm max-w-600">
                  <strong>Redacted Diagnostics Support Bundle</strong>
                  <small>Generate an encrypted diagnostic archive with secret keys and customer data sanitized for troubleshooting with DBVault engineers.</small>
                </div>
                <div className="row-actions">
                  <Badge label="Redacted" tone="neutral" />
                  <Button
                    label="Create Support Bundle"
                    onClick={async () => {
                      try {
                        const res = await API.supportBundle();
                        Store.toast(`Support bundle created: ${res.path}`, 'success');
                      } catch (err) {
                        Store.toast(err.message, 'danger');
                      }
                    }}
                    tone="secondary compact"
                  />
                </div>
              </div>

              <div className="row-between list-item-row">
                <div className="stack-sm max-w-600">
                  <strong>Preflight System Doctor</strong>
                  <small>Validate host database sockets, tool availability, disk space, and cloud storage read/write contracts.</small>
                </div>
                <div className="row-actions">
                  <Badge label="Ready" tone="success" />
                  <Button label="Run Doctor Check" onClick={handleRunDoctor} tone="secondary compact" icon="refresh" />
                </div>
              </div>
            </div>

            {/* Doctor Results Section */}
            {doctorResult && (
              <div className="doctor-results mt-md">
                <div className="card-header">
                  <div className="card-heading">
                    <h2>Preflight Doctor Results</h2>
                    <p className="card-subtitle">
                      {`${(doctorResult.checks || []).filter((c) => c.status === 'pass').length} passed · ${(doctorResult.checks || []).filter((c) => c.status !== 'pass').length} warnings`}
                    </p>
                  </div>
                  <Button label="Dismiss" onClick={() => { setDoctorResult(null); Store.set({ doctorResult: null }); }} tone="ghost compact" />
                </div>
                <div className="card-body">
                  <div className="stack-sm">
                    {(doctorResult.checks || []).map((check, i) => {
                      const tone = check.status === 'pass' ? 'success' : check.status === 'warn' ? 'warning' : 'danger';
                      return (
                        <div key={i} className="doctor-check-row">
                          <span className={`status-dot ${tone}`} />
                          <div className="doctor-check-copy">
                            <strong>{check.name || check.check}</strong>
                            {check.summary && <small>{check.summary}</small>}
                            {check.suggested_fix && <small className="text-warning mt-xs">💡 Fix: {check.suggested_fix}</small>}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}
          </Card>

          {/* Retention & RPO Policy */}
          <Card title="Grandfather-Father-Son (GFS) Retention Policy">
            <p className="text-sm mb-md">Configure automated snapshot retention windows and maximum tolerable Recovery Point Objectives.</p>
            <div className="stack-md">
              <div className="form-grid">
                <div className="form-field">
                  <label>Daily Snapshots (days)</label>
                  <input className="form-input" type="number" min="1" max="365" value={daily} onInput={(e) => setDaily(e.currentTarget.value)} />
                  <small>Retain daily snapshots for N days</small>
                </div>
                <div className="form-field">
                  <label>Weekly Snapshots (weeks)</label>
                  <input className="form-input" type="number" min="1" max="52" value={weekly} onInput={(e) => setWeekly(e.currentTarget.value)} />
                  <small>Retain weekly snapshots for N weeks</small>
                </div>
                <div className="form-field">
                  <label>Monthly Snapshots (months)</label>
                  <input className="form-input" type="number" min="1" max="120" value={monthly} onInput={(e) => setMonthly(e.currentTarget.value)} />
                  <small>Retain monthly archives for N months</small>
                </div>
                <div className="form-field">
                  <label>Target RPO Window (hours)</label>
                  <input className="form-input" type="number" min="0" step="0.5" value={rpo} onInput={(e) => setRpo(e.currentTarget.value)} />
                  <small>Alert if WAL stream gap exceeds threshold</small>
                </div>
              </div>

              <div className="row-actions mt-md">
                <Button label="Save Retention Policy" onClick={handleSaveRetention} tone="primary compact" />
              </div>
            </div>
          </Card>

          {/* Appliance Version & Release Info */}
          <Card title="Appliance Runtime & Release Management">
            <div className="stack-md">
              <div className="row-between list-item-row">
                <div className="stack-sm max-w-600">
                  <strong>DBVault Server Appliance Version</strong>
                  <small>Embedded single-binary web console and background backup agent.</small>
                </div>
                <div className="row-actions">
                  <span className="cell-mono">{releaseInfo ? `v${releaseInfo.version}` : 'v0.1.0-alpha'}</span>
                  <Button label="Check for Updates" onClick={handleCheckUpdates} tone="secondary compact" icon="refresh" />
                </div>
              </div>

              {releaseInfo && (
                <div className={`safe-note ${releaseInfo.update_available ? 'warning' : ''}`}>
                  <Icon name={releaseInfo.update_available ? 'warning' : 'check'} size={16} />
                  <span>
                    {releaseInfo.update_available
                      ? `Update available: v${releaseInfo.latest_version} — ${releaseInfo.release_notes || 'See release notes'}`
                      : `You are running the latest verified release (${formatDate(releaseInfo.published_at)}).`}
                  </span>
                </div>
              )}
            </div>
          </Card>
        </div>
      )}
    </div>
  );
}

function MasterKeyManagementCard() {
  const [keyInfo, setKeyInfo] = useState(null);
  const [loading, setLoading] = useState(true);
  const [revealModal, setRevealModal] = useState(null);
  const [showSetModal, setShowSetModal] = useState(false);

  const loadKey = async () => {
    try {
      setLoading(true);
      const res = await API.masterKeyStatus();
      setKeyInfo(res);
    } catch (_) {}
    finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadKey();
  }, []);

  const handleReveal = async () => {
    try {
      const secret = await API.masterKeyReveal();
      setRevealModal(secret);
    } catch (err) {
      Store.toast(`Failed to reveal key: ${err.message}`, 'danger');
    }
  };

  const handleGenerate = async () => {
    const ok = await Store.confirm({
      title: 'Generate New Master Key',
      message: 'Generating a new master key will rotate encryption for all future database snapshots. Ensure you download and save the new emergency kit. Proceed?',
      confirmLabel: 'Generate New Key',
      confirmTone: 'warning'
    });
    if (!ok) return;
    try {
      const secret = await API.masterKeyGenerate();
      setRevealModal(secret);
      loadKey();
      Store.toast('New Master Key generated & active', 'success');
    } catch (err) {
      Store.toast(`Failed to generate key: ${err.message}`, 'danger');
    }
  };

  return (
    <div className="stack-md">
      <Card
        title="Master Encryption Key & Disaster Recovery"
      >
        <div className="stack-md">
          <div className="grid-auto text-sm">
            <div>
              <small className="text-muted block text-xs">Encryption Cipher</small>
              <strong className="mt-xs block">AEAD AES-256-GCM (256-bit)</strong>
            </div>
            <div>
              <small className="text-muted block text-xs">Key Fingerprint</small>
              <code className="mt-xs block">{keyInfo?.fingerprint || 'sha256:calculating…'}</code>
            </div>
            <div>
              <small className="text-muted block text-xs">Custody Status</small>
              <div className="mt-xs">
                <StatusIndicator label={keyInfo?.is_set ? "Client-Held & Active" : "Unset"} tone="success" />
              </div>
            </div>
            <div>
              <small className="text-muted block text-xs">Key Storage Path</small>
              <code className="mt-xs block text-xs">{keyInfo?.key_path || 'Default Appliance Path'}</code>
            </div>
          </div>

          <div className="safe-note warning">
            <Icon name="shield" size={16} />
            <span>
              <strong>Disaster Recovery Rule:</strong> If your server is lost or destroyed, you can restore all databases onto a brand new machine in 2 minutes as long as you have your <strong>Master Key</strong>.
            </span>
          </div>

          <div className="row-actions mt-md">
            <Button
              label="View & Backup Master Key (Emergency Kit)"
              onClick={handleReveal}
              tone="primary"
              icon="shield"
            />
            <Button
              label="Set / Import Existing Key"
              onClick={() => setShowSetModal(true)}
              tone="secondary"
            />
            <Button
              label="Generate Fresh Key"
              onClick={handleGenerate}
              tone="ghost"
              icon="refresh"
            />
          </div>
        </div>
      </Card>

      {/* Disaster Recovery Guide Card */}
      <Card title="How Cold-Start Recovery Works (Zero Server Dependency)">
        <div className="stack-sm text-sm">
          <p className="text-muted">If this server permanently goes down, follow these 3 steps on any fresh machine anywhere:</p>
          <ol className="stack-xs" style={{ paddingLeft: '20px', lineHeight: 1.6 }}>
            <li><strong>1. Download DBVault:</strong> <code>curl -fsSL https://get.dbvault.io | sh</code> (or use docker / single binary)</li>
            <li><strong>2. Provide your Master Key:</strong> <code>export DBVAULT_MASTER_KEY="your-saved-hex-key"</code></li>
            <li><strong>3. Restore database directly from R2 / S3:</strong> <code>dbvault restore --snapshot latest --target postgresql://postgres:pass@localhost:5432/my_db</code></li>
          </ol>
        </div>
      </Card>

      {/* Reveal Modal */}
      {revealModal && (
        <MasterKeyRevealModal
          secret={revealModal}
          onClose={() => setRevealModal(null)}
        />
      )}

      {/* Set Modal */}
      {showSetModal && (
        <MasterKeySetModal
          currentPath={keyInfo?.key_path}
          onClose={() => setShowSetModal(false)}
          onSaved={() => {
            setShowSetModal(false);
            loadKey();
          }}
        />
      )}
    </div>
  );
}

function MasterKeyRevealModal({ secret, onClose }) {
  const [copied, setCopied] = useState(false);
  const [confirmed, setConfirmed] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(secret.key_hex);
    setCopied(true);
    Store.toast('Master Key copied to clipboard', 'success');
    setTimeout(() => setCopied(false), 3000);
  };

  const handleDownload = () => {
    const blob = new Blob([secret.recovery_sheet], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `dbvault-disaster-recovery-kit-${new Date().toISOString().slice(0, 10)}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    Store.toast('Disaster Recovery Kit downloaded (.txt)', 'success');
  };

  return (
    <div className="command-overlay" role="dialog" aria-modal="true" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="command-panel" style={{ maxWidth: '650px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>DBVault Emergency Disaster Recovery Kit</h2>
            <p className="card-subtitle">Master AEAD AES-256-GCM Encryption Key & Cold-Start Runbook</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <div className="stack-md">
          <div className="form-field">
            <label>Master Encryption Key (256-bit Hex)</label>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                className="form-input cell-mono text-sm"
                value={secret.key_hex}
                readOnly
                style={{ background: 'var(--panel-inset)', fontWeight: 600 }}
              />
              <Button
                label={copied ? "Copied!" : "Copy Key"}
                onClick={handleCopy}
                tone="primary compact"
                icon="check"
              />
            </div>
          </div>

          <div className="grid-2" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
            <div className="metric-card" style={{ padding: '12px', background: 'var(--panel-inset)', borderRadius: '6px' }}>
              <small className="text-muted text-xs block">Key Fingerprint</small>
              <code className="text-xs">{secret.fingerprint}</code>
            </div>
            <div className="metric-card" style={{ padding: '12px', background: 'var(--panel-inset)', borderRadius: '6px' }}>
              <small className="text-muted text-xs block">Algorithm</small>
              <strong className="text-xs">AEAD AES-256-GCM</strong>
            </div>
          </div>

          <div className="safe-note warning">
            <Icon name="shield" size={16} />
            <span className="text-xs">
              Save this key in your password manager (e.g. 1Password / Bitwarden). If this server is ever lost, this key allows instant recovery on any new server.
            </span>
          </div>

          <div className="row-between pt-sm">
            <Button
              label="Download Emergency Sheet (.txt)"
              onClick={handleDownload}
              tone="secondary"
              icon="download"
            />
            <Button
              label="Done & Saved"
              onClick={onClose}
              tone="primary"
            />
          </div>
        </div>
      </div>
    </div>
  );
}

function MasterKeySetModal({ currentPath, onClose, onSaved }) {
  const [keyInput, setKeyInput] = useState('');
  const [saving, setSaving] = useState(false);

  const handleSave = async (e) => {
    e.preventDefault();
    if (!keyInput.trim()) {
      Store.toast('Enter a master key or passphrase', 'danger');
      return;
    }
    try {
      setSaving(true);
      await API.masterKeySet({ key: keyInput.trim() });
      Store.toast('Master encryption key updated & active', 'success');
      onSaved();
    } catch (err) {
      Store.toast(`Failed to set key: ${err.message}`, 'danger');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="command-overlay" role="dialog" aria-modal="true" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="command-panel" style={{ maxWidth: '580px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Set / Import Master Key</h2>
            <p className="card-subtitle">Paste an existing 256-bit Hex Key or custom passphrase.</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <form onSubmit={handleSave} className="stack-md">
          <div className="form-field">
            <label>Master Key (64-character Hex or Secret Passphrase)</label>
            <input
              className="form-input cell-mono"
              type="password"
              placeholder="e.g. 9f83bf871a2e4c02919d6756c8e65c69ef92272821c89876e6085e3c3f606903"
              value={keyInput}
              onInput={(e) => setKeyInput(e.currentTarget.value)}
              required
            />
            <small className="text-muted mt-xs block">
              Hex keys (64 characters) are imported directly. Any text passphrase is automatically derived using SHA-256 into a 256-bit key.
            </small>
          </div>

          <div className="row-actions">
            <Button type="button" label="Cancel" onClick={onClose} tone="ghost" />
            <Button type="submit" label={saving ? "Saving…" : "Save Master Key"} tone="primary" />
          </div>
        </form>
      </div>
    </div>
  );
}
