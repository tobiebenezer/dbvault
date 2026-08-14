import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, PageHeader, DataTable, StatusIndicator, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { formatDate, titleCase } from '../format.js';

export function RepositoriesPage() {
  const [inventory, setInventory] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedDestId, setSelectedDestId] = useState(Store.state.selectedDestination);
  const [editingDestId, setEditingDestId] = useState(Store.state.editingDestination);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.inventory();
      setInventory(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    return Store.subscribe((s) => {
      setSelectedDestId(s.selectedDestination);
      setEditingDestId(s.editingDestination);
    });
  }, []);

  if (loading) return <div className="page"><LoadingState label="Loading storage destinations…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadData} /></div>;

  const repositories = inventory?.repositories || [];
  const destinations = inventory?.destinations || [];
  const selected = destinations.find((item) => item.id === selectedDestId);
  const editing = destinations.find((item) => item.id === editingDestId);

  return (
    <div className="page">
      <PageHeader
        title="Storage & Repositories"
        description="Encrypted object storage targets (Cloudflare R2, AWS S3, MinIO) and multi-region replication mirrors."
        actions={[
          <Button key="test" label="Test all targets" onClick={() => destinations.forEach((d) => ProductActions.destinationTest(d))} tone="secondary" />,
          <Button key="add" label="Add storage destination" onClick={() => ProductActions.openSetupStep('add-storage-destination')} tone="primary" />
        ]}
      />

      {repositories.length > 0 && (
        <Card title="Repository Encryption Standards">
          <div className="stack-md">
            {repositories.map((repo) => (
              <div key={repo.id || repo.name} className="grid-auto text-sm">
                <div><small>Repository ID</small><div className="mt-sm"><strong>{repo.name || 'production-vault'}</strong></div></div>
                <div><small>Encryption</small><div className="mt-sm"><strong>{repo.encrypted ? 'AEAD AES-256-GCM' : 'Disabled'}</strong></div></div>
                <div><small>Deduplication</small><div className="mt-sm"><strong>Content-Addressed (BLAKE3)</strong></div></div>
                <div><small>Compression</small><div className="mt-sm"><strong>Zstandard / Gzip</strong></div></div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card title="Storage Destinations" noPadding>
        {destinations.length === 0 ? (
          <EmptyState
            title="No Storage Destinations"
            text="Add an S3, Cloudflare R2, or MinIO destination to begin replication."
            action={<Button label="Add destination" onClick={() => ProductActions.openSetupStep('add-storage-destination')} tone="primary" />}
          />
        ) : (
          <DataTable
            headers={[
              { label: 'Destination Name' },
              { label: 'Provider' },
              { label: 'Role' },
              { label: 'Region' },
              { label: 'Health Status' },
              { label: 'Replication Lag' },
              { label: 'Last Verified' },
              { label: 'Actions', width: '200px' }
            ]}
          >
            {destinations.map((dest) => {
              const isHealthy = dest.status === 'healthy';
              const lag = dest.lag_seconds;
              const lagTone = lag === undefined || lag === null ? 'neutral' : lag < 60 ? 'success' : lag < 600 ? 'warning' : 'danger';
              const lagLabel = lag === undefined || lag === null ? '—' : lag < 60 ? `${lag}s (In Sync)` : `${Math.round(lag / 60)}m lag`;

              return (
                <tr key={dest.id}>
                  <td className="cell-primary"><strong>{dest.name}</strong></td>
                  <td>{titleCase(dest.provider || 'S3 Compatible')}</td>
                  <td>{titleCase(dest.role || 'primary')}</td>
                  <td className="cell-mono">{dest.region || 'default'}</td>
                  <td><StatusIndicator label={titleCase(dest.status || 'healthy')} tone={isHealthy ? 'success' : 'warning'} /></td>
                  <td><StatusIndicator label={lagLabel} tone={lagTone} /></td>
                  <td className="cell-mono">{formatDate(dest.last_checked_at)}</td>
                  <td className="cell-actions cell-actions-w200">
                    <Button label="Test" onClick={() => ProductActions.destinationTest(dest)} tone="secondary compact" />
                    <Button
                      label={selectedDestId === dest.id ? 'Close' : 'Details'}
                      onClick={() => Store.set({ selectedDestination: selectedDestId === dest.id ? null : dest.id, editingDestination: null })}
                      tone="ghost compact"
                    />
                  </td>
                </tr>
              );
            })}
          </DataTable>
        )}
      </Card>

      {selected && !editing && (
        <Card title={`Details: ${selected.name}`}>
          <div className="grid-3 mb-md text-sm">
            <div><small>Provider</small><div className="mt-sm"><strong>{titleCase(selected.provider)}</strong></div></div>
            <div><small>Role</small><div className="mt-sm"><strong>{titleCase(selected.role)}</strong></div></div>
            <div><small>Region</small><div className="mt-sm"><strong>{selected.region || '—'}</strong></div></div>
            <div><small>Status</small><div className="mt-sm"><strong>{titleCase(selected.status)}</strong></div></div>
            <div><small>Replication Lag</small><div className="mt-sm"><strong>{selected.lag_seconds ? `${selected.lag_seconds}s` : '0s (In Sync)'}</strong></div></div>
            <div><small>Last Check</small><div className="mt-sm"><strong>{formatDate(selected.last_checked_at)}</strong></div></div>
          </div>
          <div className="row-actions">
            <Button label="Test Destination" onClick={() => ProductActions.destinationTest(selected)} tone="primary compact" />
            <Button label="Edit Configuration" onClick={() => Store.set({ editingDestination: selected.id })} tone="secondary compact" />
            <Button label="Close" onClick={() => Store.set({ selectedDestination: null })} tone="ghost compact" />
          </div>
        </Card>
      )}

      {editing && (
        <Card title={`Edit Configuration: ${editing.name}`}>
          <div className="safe-note warning">
            <span>Saving new credentials will trigger a connectivity test. Existing backup data is not affected.</span>
          </div>
          <div className="form-grid mt-md">
            <div className="form-field"><label>Display Name</label><input className="form-input" defaultValue={editing.name || ''} /></div>
            <div className="form-field"><label>Endpoint URL</label><input className="form-input" defaultValue={editing.endpoint || ''} /></div>
            <div className="form-field"><label>Bucket</label><input className="form-input" defaultValue={editing.bucket || ''} /></div>
            <div className="form-field"><label>Region</label><input className="form-input" defaultValue={editing.region || ''} /></div>
          </div>
          <div className="row-actions mt-md">
            <Button
              label="Save & Test"
              onClick={() => {
                Store.toast('Connectivity test queued', 'success');
                ProductActions.destinationTest(editing);
                Store.set({ editingDestination: null });
              }}
              tone="primary compact"
            />
            <Button label="Cancel" onClick={() => Store.set({ editingDestination: null })} tone="ghost compact" />
          </div>
        </Card>
      )}
    </div>
  );
}
