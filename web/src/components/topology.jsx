import { Icon, Badge, StatusIndicator } from './ui.jsx';
import { titleCase } from '../format.js';

export function StorageTopologyMap({ inventory = {}, timeline = {} }) {
  const databases = inventory.databases || [];
  const destinations = inventory.destinations || [];
  const repositories = inventory.repositories || [];
  const repo = repositories[0] || { name: 'Production Vault', encrypted: true };

  return (
    <div className="topology-map-container">
      <div className="topology-grid">
        {/* Source Databases Column */}
        <div className="topology-column">
          <div className="topology-column-header">
            <Icon name="search" size={14} />
            <span>Active Sources ({databases.length})</span>
          </div>
          <div className="topology-nodes">
            {databases.map((db) => (
              <div key={db.id} className="topology-node database-node">
                <div className="topology-node-head">
                  <strong>{db.name}</strong>
                  <Badge label={titleCase(db.engine)} tone="neutral" />
                </div>
                <div className="topology-node-meta">
                  <span className="text-muted text-xs">{db.host || '127.0.0.1'} · WAL stream active</span>
                  <StatusIndicator label="Protected" tone="success" />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* DBVault Core Engine Column */}
        <div className="topology-column center-column">
          <div className="topology-column-header">
            <Icon name="shield" size={14} />
            <span>DBVault Core Appliance</span>
          </div>
          <div className="topology-node core-node">
            <div className="core-node-brand">
              <span className="brand-mark">DB</span>
              <div>
                <strong>{repo.name || 'Production Vault'}</strong>
                <small className="text-muted">Zero-Knowledge Appliance</small>
              </div>
            </div>
            <div className="core-pipeline-badges">
              <span className="pipeline-badge"><Icon name="shield" size={12} /> AEAD AES-256-GCM</span>
              <span className="pipeline-badge"><Icon name="check" size={12} /> BLAKE3 Dedup</span>
              <span className="pipeline-badge"><Icon name="download" size={12} /> Zstandard / Gzip</span>
            </div>
            <div className="core-status-row">
              <StatusIndicator label="Continuous Archive (0 gaps)" tone="success" />
            </div>
          </div>
        </div>

        {/* Encrypted Destinations Column */}
        <div className="topology-column">
          <div className="topology-column-header">
            <Icon name="download" size={14} />
            <span>Storage Targets ({destinations.length})</span>
          </div>
          <div className="topology-nodes">
            {destinations.map((dest) => {
              const isPrimary = dest.role === 'primary';
              const isHealthy = dest.status === 'healthy';
              return (
                <div key={dest.id} className={`topology-node destination-node ${isPrimary ? 'primary-dest' : 'replica-dest'}`}>
                  <div className="topology-node-head">
                    <strong>{dest.name}</strong>
                    <Badge label={titleCase(dest.role || 'primary')} tone={isPrimary ? 'success' : 'neutral'} />
                  </div>
                  <div className="topology-node-meta">
                    <span className="text-muted text-xs">{titleCase(dest.provider)} · {dest.region || 'global'}</span>
                    <StatusIndicator
                      label={dest.lag_seconds ? `${dest.lag_seconds}s lag` : 'In Sync (0s)'}
                      tone={isHealthy ? 'success' : 'warning'}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
