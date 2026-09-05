import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, MetricCard, DataTable, LoadingState, ErrorBox, Icon, StatusIndicator } from '../components/ui.jsx';
import { formatBytes, formatDate, titleCase } from '../format.js';
import { Store } from '../state.js';

export function BillingPage({ embedded = false }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const loadBilling = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.billingUsage();
      setData(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadBilling();
    return Store.subscribe((s) => {
      if (s.refreshToken) loadBilling();
    });
  }, []);

  if (loading) return <div className={embedded ? "" : "page"}><LoadingState label="Loading usage metering and cloud costs…" /></div>;
  if (error) return <div className={embedded ? "" : "page"}><ErrorBox error={error} retry={loadBilling} /></div>;

  const usage = data || {};
  const destinations = usage.storage_by_destination || [];
  const quota = usage.quota || {};
  const ops = usage.operations_this_month || {};

  return (
    <div className={embedded ? "" : "page"}>
      {!embedded && (
        <PageHeader
          title="Usage Metering & Costs"
          actions={[
            <Button
              key="export"
              label="Download Invoice Estimate"
              onClick={() => {
                const text = JSON.stringify(usage, null, 2);
                const blob = new Blob([text], { type: 'application/json' });
                const url = URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `dbvault-billing-estimate-${new Date().toISOString().slice(0, 10)}.json`;
                a.click();
                URL.revokeObjectURL(url);
                Store.toast('Invoice estimate downloaded', 'success');
              }}
              tone="primary"
              icon="download"
            />
          ]}
        />
      )}

      {/* 4 Core Cost & Storage Metric Cards */}
      <div className="metric-grid">
        <MetricCard
          label="Total Vault Storage"
          value={formatBytes(usage.total_storage_bytes)}
          footerText={`${usage.storage_usage_percent?.toFixed(1)}% of ${formatBytes(usage.storage_limit_bytes)} quota`}
          statusTone="success"
        />
        <MetricCard
          label="Estimated Monthly Cost"
          value={`$${(usage.estimated_monthly_cost_usd || 0).toFixed(2)}`}
          footerText="Hot object storage fees"
          statusTone="neutral"
        />
        <MetricCard
          label="AWS RDS Backup Savings"
          value={`~$${(usage.estimated_rds_savings_usd || 0).toFixed(2)}/mo`}
          footerText="87% lower than native AWS snapshots"
          statusTone="success"
        />
        <MetricCard
          label="Billable Databases"
          value={`${usage.protected_databases} / ${quota.maximum_sources || 20}`}
          footerText="Continuous WAL protection"
          statusTone="success"
        />
      </div>

      {/* Savings & Architecture Value Banner */}
      <Card title="Zero-Egress Hot Storage Architecture">
        <div className="grid-3 text-sm">
          <div>
            <small>Client-Side Compression</small>
            <div className="mt-sm"><strong>Zstandard Adaptive (Average 3.4x ratio)</strong></div>
          </div>
          <div>
            <small>Cloudflare R2 Egress Fees</small>
            <div className="mt-sm"><strong className="text-success">$0.00 (Zero Egress Penalty)</strong></div>
          </div>
          <div>
            <small>Current Billing Cycle</small>
            <div className="mt-sm"><strong>{formatDate(usage.billing_period_start)} → {formatDate(usage.billing_period_end)}</strong></div>
          </div>
        </div>
      </Card>

      {/* Storage Cost Breakdown by Destination Table */}
      <Card title="Storage Cost Attribution by Target" noPadding>
        <DataTable
          headers={[
            { label: 'Storage Target' },
            { label: 'Provider' },
            { label: 'Stored Volume' },
            { label: 'Rate ($/GB/mo)' },
            { label: 'Est. Monthly Cost' }
          ]}
        >
          {destinations.map((d) => (
            <tr key={d.destination_id}>
              <td className="cell-primary"><strong>{d.destination_name}</strong></td>
              <td><Badge label={titleCase(d.provider)} tone="neutral" /></td>
              <td className="cell-mono">{formatBytes(d.storage_bytes)}</td>
              <td className="cell-mono">${d.cost_per_gb_usd?.toFixed(3)}</td>
              <td className="cell-mono"><strong className="text-success">${d.estimated_cost_usd?.toFixed(3)}</strong></td>
            </tr>
          ))}
        </DataTable>
      </Card>

      {/* Operations Metering & Quotas */}
      <div className="grid-2">
        <Card title="Operations Metering (Current Month)">
          <div className="stack-sm text-sm">
            <div className="row-between list-item-row">
              <span>Full Base Snapshots</span>
              <strong>{ops.backup_snapshots || 0}</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Continuous WAL Chunks Uploaded</span>
              <strong>{ops.wal_chunks || 0}</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Verified Restore Drills Executed</span>
              <strong>{ops.restore_drills || 0}</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Cross-Region Replications</span>
              <strong>{ops.replications || 0}</strong>
            </div>
          </div>
        </Card>

        <Card title="Tenant Quotas & Limits">
          <div className="stack-sm text-sm">
            <div className="row-between list-item-row">
              <span>Maximum Protected Sources</span>
              <strong>{quota.maximum_sources || 20} databases</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Maximum Storage Quota</span>
              <strong>{formatBytes(quota.maximum_storage_bytes || 100_000_000_000)}</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Max Concurrent Backup Workers</span>
              <strong>{quota.maximum_concurrent_backups || 5} concurrent</strong>
            </div>
            <div className="row-between list-item-row">
              <span>Max Concurrent Sandbox Restores</span>
              <strong>{quota.maximum_concurrent_restores || 2} concurrent</strong>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}
