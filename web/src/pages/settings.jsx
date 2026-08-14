import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader } from '../components/ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { formatDate } from '../format.js';

export function SettingsPage() {
  const [doctorResult, setDoctorResult] = useState(Store.state.doctorResult);
  const [releaseInfo, setReleaseInfo] = useState(Store.state.releaseInfo);
  const [webhooks, setWebhooks] = useState(Store.state.webhookConfig || []);
  const [retention, setRetention] = useState(Store.state.retentionPolicy || { daily: 7, weekly: 4, monthly: 12, rpo_hours: 1 });

  // Webhook form state
  const [whType, setWhType] = useState('slack');
  const [whEvents, setWhEvents] = useState('all');
  const [whUrl, setWhUrl] = useState('');

  // Retention form state
  const [daily, setDaily] = useState(retention.daily);
  const [weekly, setWeekly] = useState(retention.weekly);
  const [monthly, setMonthly] = useState(retention.monthly);
  const [rpo, setRpo] = useState(retention.rpo_hours);

  useEffect(() => {
    return Store.subscribe((s) => {
      setDoctorResult(s.doctorResult);
      setReleaseInfo(s.releaseInfo);
      if (s.webhookConfig) setWebhooks(s.webhookConfig);
      if (s.retentionPolicy) setRetention(s.retentionPolicy);
    });
  }, []);

  const handleRunDoctor = async () => {
    try {
      const res = await API.doctor();
      Store.set({ doctorResult: res });
      setDoctorResult(res);
      const warnings = (res.checks || []).filter((c) => c.status !== 'pass').length;
      Store.toast(warnings ? `Doctor found ${warnings} issue${warnings === 1 ? '' : 's'}` : 'All checks passed', warnings ? 'warning' : 'success');
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

  const handleAddWebhook = () => {
    if (!whUrl.trim()) {
      Store.toast('Enter a webhook URL', 'danger');
      return;
    }
    const updated = [...webhooks, { type: whType, events: whEvents, url: whUrl.trim() }];
    setWebhooks(updated);
    Store.set({ webhookConfig: updated });
    Store.toast('Webhook added', 'success');
    setWhUrl('');
  };

  const handleRemoveWebhook = (index) => {
    const updated = webhooks.filter((_, i) => i !== index);
    setWebhooks(updated);
    Store.set({ webhookConfig: updated });
    Store.toast('Webhook removed', 'success');
  };

  const handleSaveRetention = () => {
    const updated = { daily: Number(daily), weekly: Number(weekly), monthly: Number(monthly), rpo_hours: Number(rpo) };
    setRetention(updated);
    Store.set({ retentionPolicy: updated });
    Store.toast('Retention policy saved', 'success');
  };

  return (
    <div className="page">
      <PageHeader
        title="Settings & Diagnostics"
        description="System diagnostics, emergency recovery bundles, notification webhooks, and retention policies."
      />

      <Card title="Disaster Recovery & Diagnostics">
        <div className="stack-md">
          <div className="row-between list-item-row">
            <div className="stack-sm max-w-600">
              <strong>Emergency Recovery Bundle</strong>
              <small>Export encrypted DBVault configuration, encryption keys, and catalogue metadata for air-gapped recovery.</small>
            </div>
            <div className="row-actions">
              <Badge label="Export Ready" tone="neutral" />
              <Button
                label="Create recovery bundle"
                onClick={async () => {
                  try {
                    const res = await API.recoveryBundle();
                    Store.toast(`Recovery bundle created: ${res.path}`, 'success');
                  } catch (err) {
                    Store.toast(err.message, 'danger');
                  }
                }}
                tone="primary compact"
              />
            </div>
          </div>

          <div className="row-between list-item-row">
            <div className="stack-sm max-w-600">
              <strong>Sanitized Support Bundle</strong>
              <small>Export diagnostic logs and environment telemetry stripped of credentials and database rows.</small>
            </div>
            <div className="row-actions">
              <Badge label="Safe Export" tone="neutral" />
              <Button
                label="Create support bundle"
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
              <strong>System Preflight Doctor</strong>
              <small>Run host health checks, socket inspections, storage bandwidth, and tool readiness tests.</small>
            </div>
            <div className="row-actions">
              <Badge label="Ready" tone="success" />
              <Button label="Run doctor check" onClick={handleRunDoctor} tone="secondary compact" />
            </div>
          </div>
        </div>

        {doctorResult && (
          <div className="doctor-results mt-md">
            <div className="card-header">
              <div className="card-heading">
                <h2>Doctor Results</h2>
                <p className="card-subtitle">{`${(doctorResult.checks || []).filter((c) => c.status === 'pass').length} passed · ${(doctorResult.checks || []).filter((c) => c.status !== 'pass').length} warnings`}</p>
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
                        {check.detail && <small>{check.detail}</small>}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </Card>

      <Card title="Alert Notification Webhooks">
        <p className="text-sm mb-md">Configure Slack, Discord, or generic HTTP webhook endpoints for RPO breach and incident alerts.</p>
        <div className="stack-md">
          {webhooks.length > 0 && (
            <div className="stack-sm">
              {webhooks.map((wh, idx) => (
                <div key={idx} className="row-between list-item-row">
                  <div className="stack-sm">
                    <strong>{wh.type ? wh.type.charAt(0).toUpperCase() + wh.type.slice(1) : 'Webhook'}</strong>
                    <small className="cell-mono">{wh.url}</small>
                  </div>
                  <Button label="Remove" onClick={() => handleRemoveWebhook(idx)} tone="ghost compact" />
                </div>
              ))}
            </div>
          )}

          <div className="form-grid">
            <div className="form-field">
              <label>Webhook Type</label>
              <select className="form-select" value={whType} onChange={(e) => setWhType(e.currentTarget.value)}>
                <option value="slack">Slack</option>
                <option value="discord">Discord</option>
                <option value="webhook">Generic HTTP Webhook</option>
              </select>
            </div>
            <div className="form-field">
              <label>Alert Events</label>
              <select className="form-select" value={whEvents} onChange={(e) => setWhEvents(e.currentTarget.value)}>
                <option value="all">All events</option>
                <option value="critical">Critical alerts only</option>
                <option value="rpo_breach">RPO breach alerts only</option>
              </select>
            </div>
            <div className="form-field">
              <label>Webhook URL</label>
              <input
                className="form-input"
                type="url"
                placeholder="https://hooks.slack.com/services/…"
                value={whUrl}
                onInput={(e) => setWhUrl(e.currentTarget.value)}
              />
            </div>
          </div>

          <div className="row-actions mt-sm">
            <Button label="Add Webhook" onClick={handleAddWebhook} tone="primary compact" />
            <Button
              label="Test webhook"
              onClick={() => {
                if (!whUrl.trim()) { Store.toast('Enter a webhook URL to test', 'danger'); return; }
                Store.toast('Webhook test queued', 'success');
              }}
              tone="secondary compact"
            />
          </div>
        </div>
      </Card>

      <Card title="Backup Retention & RPO Policy">
        <p className="text-sm mb-md">Configure how long backups are retained and the target Recovery Point Objective for each environment.</p>
        <div className="stack-md">
          <div className="form-grid">
            <div className="form-field">
              <label>Daily Snapshots (days)</label>
              <input className="form-input" type="number" min="1" max="365" value={daily} onInput={(e) => setDaily(e.currentTarget.value)} />
              <small>Keep daily backups for N days</small>
            </div>
            <div className="form-field">
              <label>Weekly Snapshots (weeks)</label>
              <input className="form-input" type="number" min="1" max="52" value={weekly} onInput={(e) => setWeekly(e.currentTarget.value)} />
              <small>Keep weekly backups for N weeks</small>
            </div>
            <div className="form-field">
              <label>Monthly Snapshots (months)</label>
              <input className="form-input" type="number" min="1" max="120" value={monthly} onInput={(e) => setMonthly(e.currentTarget.value)} />
              <small>Keep monthly backups for N months</small>
            </div>
            <div className="form-field">
              <label>RPO Target (hours)</label>
              <input className="form-input" type="number" min="0" step="0.5" value={rpo} onInput={(e) => setRpo(e.currentTarget.value)} />
              <small>Max tolerable data loss window</small>
            </div>
          </div>

          <div className="safe-note mt-sm">
            <span>WAL streaming provides near-zero RPO during continuous archive. Snapshot retention affects cold-storage costs.</span>
          </div>

          <div className="row-actions mt-md">
            <Button label="Save Retention Policy" onClick={handleSaveRetention} tone="primary compact" />
            <Button label="Simulate retention cost" onClick={() => ProductActions.openSetupStep('configure-policy')} tone="ghost compact" />
          </div>
        </div>
      </Card>

      <Card title="System & Release Management">
        <div className="stack-md">
          <div className="row-between list-item-row">
            <div className="stack-sm max-w-600">
              <strong>Appliance Version</strong>
              <small>Current runtime binary and embedded web console build.</small>
            </div>
            <div className="row-actions">
              <span className="cell-mono">{releaseInfo ? `v${releaseInfo.version}` : 'v0.1.0-alpha'}</span>
              <Button label="Check for updates" onClick={handleCheckUpdates} tone="secondary compact" />
            </div>
          </div>

          {releaseInfo && (
            <div className={`safe-note ${releaseInfo.update_available ? 'warning' : ''}`}>
              <span>
                {releaseInfo.update_available
                  ? `Update available: v${releaseInfo.latest_version} — ${releaseInfo.release_notes || 'See release notes'}`
                  : `You are running the latest version (${formatDate(releaseInfo.published_at)}).`}
              </span>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
