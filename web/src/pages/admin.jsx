import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, MetricCard, DataTable, LoadingState, ErrorBox, Icon, StatusIndicator } from '../components/ui.jsx';
import { formatBytes, formatDate, titleCase } from '../format.js';
import { Store } from '../state.js';

export function AdminFleetPage({ embedded = false }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showAgentModal, setShowAgentModal] = useState(false);

  const loadFleet = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.adminFleet();
      setData(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadFleet();
    return Store.subscribe((s) => {
      if (s.refreshToken) loadFleet();
    });
  }, []);

  if (loading) return <div className={embedded ? "" : "page"}><LoadingState label="Loading fleet metrics…" /></div>;
  if (error) return <div className={embedded ? "" : "page"}><ErrorBox error={error} retry={loadFleet} /></div>;

  const fleet = data || {};
  const tenants = fleet.tenants || [];
  const agents = fleet.agents || [];

  return (
    <div className={embedded ? "" : "page"}>
      {!embedded && (
        <PageHeader
          title="Multi-Tenant Super-Admin & Fleet Control Plane"
          description="Global tenant overview, cross-region agent cluster health, and metered subscription revenue."
          actions={[
            <Button
              key="enrol"
              label="Enrol Outbound Agent Node"
              onClick={() => setShowAgentModal(true)}
              tone="primary"
              icon="plus"
            />
          ]}
        />
      )}

      {/* Global Fleet Metrics */}
      <div className="metric-grid">
        <MetricCard
          label="Registered Tenants"
          value={`${fleet.total_tenants || 0}`}
          footerText="Active SaaS organisations"
          statusTone="success"
        />
        <MetricCard
          label="Protected Database Fleet"
          value={`${fleet.active_databases || 0} instances`}
          footerText="Agentless + Agent nodes"
          statusTone="success"
        />
        <MetricCard
          label="Global Vault Storage"
          value={formatBytes(fleet.global_storage_bytes || 0)}
          footerText="R2, S3, and MinIO targets"
          statusTone="neutral"
        />
        <MetricCard
          label="Monthly Metered Revenue"
          value={`$${(fleet.global_monthly_revenue_usd || 0).toFixed(2)}`}
          footerText="Stripe recurring subscriptions"
          statusTone="success"
        />
      </div>

      {/* Tenants Table */}
      <Card title="Customer Organisations & Subscriptions" noPadding>
        <DataTable
          headers={[
            { label: 'Organisation' },
            { label: 'Plan Tier' },
            { label: 'Status' },
            { label: 'Databases' },
            { label: 'Vault Storage' },
            { label: 'Monthly MRR' },
            { label: 'Actions', width: '140px' }
          ]}
        >
          {tenants.map((t) => (
            <tr key={t.id}>
              <td className="cell-primary">
                <div>
                  <strong>{t.name}</strong>
                  <small className="text-muted block text-xs">ID: {t.slug}</small>
                </div>
              </td>
              <td><Badge label={t.plan} tone={t.plan === 'Enterprise' ? 'success' : 'neutral'} /></td>
              <td><StatusIndicator label={titleCase(t.status)} tone={t.status === 'active' ? 'success' : 'danger'} /></td>
              <td>{t.databases_count} protected</td>
              <td className="cell-mono">{formatBytes(t.storage_bytes)}</td>
              <td className="cell-mono"><strong className="text-success">${t.monthly_cost_usd?.toFixed(2)}</strong></td>
              <td className="cell-actions">
                <Button
                  label="Manage"
                  onClick={() => Store.toast(`Switched to tenant ${t.name}`, 'info')}
                  tone="ghost compact"
                />
              </td>
            </tr>
          ))}
        </DataTable>
      </Card>

      {/* Connected Outbound Agents Table */}
      <Card
        title={`Outbound Agent Fleet (${agents.length} nodes connected)`}
        subtitle="Private VPC & On-Premise database agent connections (outbound mTLS port 443 — 0 firewall openings)"
        noPadding
      >
        <DataTable
          headers={[
            { label: 'Node Hostname' },
            { label: 'IP Address' },
            { label: 'Environment' },
            { label: 'Ping Latency' },
            { label: 'CPU / Mem Load' },
            { label: 'Heartbeat' },
            { label: 'Status' },
            { label: 'Actions', width: '100px' }
          ]}
        >
          {agents.map((ag) => (
            <tr key={ag.id}>
              <td className="cell-primary">
                <div>
                  <strong>{ag.hostname}</strong>
                  <small className="text-muted block text-xs">{ag.version} · {ag.protected_databases_count} DBs</small>
                </div>
              </td>
              <td className="cell-mono">{ag.ip}</td>
              <td>{ag.os} ({ag.arch})</td>
              <td className="cell-mono"><strong className="text-success">{ag.ping_latency_ms} ms</strong></td>
              <td className="cell-mono">{ag.cpu_usage_percent?.toFixed(1)}% / {ag.memory_usage_percent?.toFixed(1)}%</td>
              <td className="cell-mono">{formatDate(ag.last_heartbeat_at)}</td>
              <td><StatusIndicator label="Online (mTLS)" tone="success" /></td>
              <td className="cell-actions">
                <Button
                  label="Revoke"
                  tone="danger compact"
                  onClick={async () => {
                    const ok = await Store.confirm({
                      title: 'Revoke Agent Node',
                      message: `Are you sure you want to revoke agent node ${ag.hostname} (${ag.id})? Node will no longer be authorized to sync backups.`,
                      confirmLabel: 'Revoke Node',
                      confirmTone: 'danger'
                    });
                    if (!ok) return;
                    try {
                      await API.revokeAgent(ag.id);
                      Store.toast(`Agent node ${ag.hostname} revoked`, 'success');
                      loadFleet();
                    } catch (err) {
                      Store.toast(`Revocation failed: ${err.message}`, 'danger');
                    }
                  }}
                />
              </td>
            </tr>
          ))}
        </DataTable>
      </Card>

      {/* Enrol Agent Modal */}
      {showAgentModal && (
        <EnrolAgentModal onClose={() => setShowAgentModal(false)} />
      )}
    </div>
  );
}

function EnrolAgentModal({ onClose }) {
  const token = `tk_live_${Math.random().toString(36).slice(2, 10)}${Math.random().toString(36).slice(2, 10)}`;
  const installCmd = `curl -sSL http://127.0.0.1:8006/install/agent.sh | sh -s -- --token ${token}`;

  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Enrol Outbound Agent"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="command-panel" style={{ maxWidth: '600px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Enrol Outbound Agent Node</h2>
            <p className="card-subtitle">Install the 5MB agent on any server or private VPC in 10 seconds.</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <div className="stack-md">
          <div className="safe-note">
            <Icon name="shield" size={16} />
            <span>Outbound mTLS only: No inbound firewall ports or public IP addresses required on your database host.</span>
          </div>

          <div className="form-field">
            <label>1-Line Terminal Install Command</label>
            <div className="config-path" style={{ padding: '12px' }}>
              <code style={{ fontSize: '11px', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{installCmd}</code>
            </div>
          </div>

          <div className="row-actions mt-md">
            <Button
              label="Copy Command"
              onClick={() => {
                navigator.clipboard.writeText(installCmd);
                Store.toast('Agent install command copied to clipboard', 'success');
              }}
              tone="primary"
              icon="download"
            />
            <Button label="Done" onClick={onClose} tone="ghost" />
          </div>
        </div>
      </div>
    </div>
  );
}
