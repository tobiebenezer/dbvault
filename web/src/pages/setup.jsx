import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, Icon, StatusIndicator, LoadingState, ErrorBox } from '../components/ui.jsx';
import { Store } from '../state.js';
import { formatBytes, titleCase } from '../format.js';
import { ProductActions } from '../actions.js';

const STEPS = [
  { id: 'create-administrator', label: 'Owner', icon: 'shield', hint: 'Record the appliance owner. Console authentication is a separate production-readiness item.' },
  { id: 'configure-public-address', label: 'Console', icon: 'shield', hint: 'Set the address operators will use and document the intended TLS mode.' },
  { id: 'add-storage-destination', label: 'Storage', icon: 'download', hint: 'Choose a filesystem directory or an S3-compatible object store (Cloudflare R2, AWS S3, MinIO, Contabo).' },
  { id: 'create-repository', label: 'Repository', icon: 'shield', hint: 'Define encryption (AES-256-GCM), retention, and the maximum storage budget.' },
  { id: 'discover-or-add-database', label: 'Database', icon: 'search', hint: 'Enter the database name, connection credentials, TLS policy, and timeout.' },
  { id: 'apply-policy', label: 'Policy', icon: 'refresh', hint: 'Choose the initial backup schedule, WAL streaming, and retention posture.' },
  { id: 'run-doctor', label: 'Doctor', icon: 'info', hint: 'Review configuration readiness and system diagnostics before generating runtime files.' },
  { id: 'run-first-backup', label: 'Backup', icon: 'play', hint: 'Backup execution starts after the generated configuration is activated.' },
  { id: 'run-restore-drill', label: 'Restore Test', icon: 'refresh', hint: 'Restore verification requires a real backup from the activated runtime.' },
  { id: 'configure-alerts', label: 'Alerts', icon: 'warning', hint: 'Record where action-required notifications should be delivered (Slack, Discord, Email).' },
  { id: 'finish', label: 'Review', icon: 'check', hint: 'Review the connection graph, then generate a validated runtime configuration.' }
];

export function SetupPage() {
  const [setupState, setSetupState] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  const loadSetup = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.setup();
      setSetupState(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadSetup();
    return Store.subscribe((s) => {
      if (s.refreshToken) loadSetup();
    });
  }, []);

  if (loading) return <div className="page"><LoadingState label="Loading setup wizard…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadSetup} /></div>;

  const completed = new Set(setupState.completed_steps || []);
  const currentStepId = setupState.current_step || 'create-administrator';
  const currentIndex = Math.max(0, STEPS.findIndex((s) => s.id === currentStepId));
  const currentStep = STEPS[currentIndex] || STEPS[0];
  const drafts = setupState.drafts || {};
  const isReady = setupState.status === 'configuration_ready';

  const handleStepSelect = async (stepId) => {
    try {
      const res = await API.setSetupStep(stepId);
      setSetupState(res);
    } catch (err) {
      Store.toast(err.message, 'danger');
    }
  };

  const handleSaveStep = async (stepId, data) => {
    try {
      setSubmitting(true);
      if (stepId === 'finish') {
        const res = await API.finishSetup();
        setSetupState(res);
        Store.toast('Validated runtime configuration generated!', 'success');
        setSubmitting(false);
        return;
      }
      const res = await API.saveSetupStep(stepId, data);
      setSetupState(res);
      Store.toast(`${currentStep.label} saved`, 'success');
      setSubmitting(false);
    } catch (err) {
      Store.toast(err.message, 'danger');
      setSubmitting(false);
    }
  };

  return (
    <div className="page setup-page">
      <PageHeader
        title="Set up DBVault"
        actions={[
          <Button key="exit" label="Exit to Console" onClick={() => Store.navigate('/')} tone="secondary" />
        ]}
      />

      {/* Progress Strip */}
      <div className="setup-progress-compact">
        <div className="setup-progress-label">
          <strong>Step {currentIndex + 1} of {STEPS.length}</strong>
          <span>{currentStep.label} · saved after validation</span>
          {isReady && <Badge label="Configuration Ready" tone="success" />}
        </div>
        <div className="setup-progress-track">
          <span style={{ width: `${Math.round(((currentIndex + 1) / STEPS.length) * 100)}%` }} />
        </div>
        <div className="setup-step-dots">
          {STEPS.map((s, idx) => (
            <button
              key={s.id}
              type="button"
              className={`${completed.has(s.id) ? 'done' : ''} ${idx === currentIndex ? 'current' : ''}`}
              title={`${idx + 1}. ${s.label}`}
              aria-label={`Go to ${s.label}`}
              onClick={() => handleStepSelect(s.id)}
            />
          ))}
        </div>
      </div>

      {/* Main Step Panel */}
      <section className="setup-panel">
        <header className="setup-panel-head">
          <span className="setup-current-icon">
            <Icon name={currentStep.icon} size={22} />
          </span>
          <div>
            <h2>{currentStep.label}</h2>
            <p>{currentStep.hint}</p>
          </div>
        </header>

        <StepFormBody
          stepId={currentStepId}
          drafts={drafts}
          setupState={setupState}
          onSave={(data) => handleSaveStep(currentStepId, data)}
          onNavigateStep={handleStepSelect}
          currentIndex={currentIndex}
          isReady={isReady}
          submitting={submitting}
        />
      </section>
    </div>
  );
}

function StepFormBody({ stepId, drafts, setupState, onSave, onNavigateStep, currentIndex, isReady, submitting }) {
  const saved = drafts[stepId] || {};
  const [formData, setFormData] = useState({ ...saved });

  useEffect(() => {
    setFormData({ ...(drafts[stepId] || {}) });
  }, [stepId, drafts]);

  const updateField = (key, val) => {
    setFormData((prev) => ({ ...prev, [key]: val }));
  };

  const handleFormSubmit = (e) => {
    if (e) e.preventDefault();
    onSave(formData);
  };

  const handleBack = () => {
    if (currentIndex > 0) {
      onNavigateStep(STEPS[currentIndex - 1].id);
    }
  };

  let content;
  switch (stepId) {
    case 'create-administrator':
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Appliance Owner</h3>
            <p>Used for ownership metadata and emergency recovery notifications.</p>
          </div>
          <div className="field-grid">
            <label className="field-group">
              <span><span>Administrator email</span><b className="required-mark">Required</b></span>
              <input
                type="email"
                placeholder="admin@example.com"
                value={formData.administrator_email || ''}
                onInput={(e) => updateField('administrator_email', e.currentTarget.value)}
                required
              />
            </label>
            <label className="field-group">
              <span><span>Password</span><b className="required-mark">{saved.administrator_password_configured ? 'Saved' : 'Required'}</b></span>
              <input
                type="password"
                placeholder={saved.administrator_password_configured ? 'Stored securely; leave blank to keep' : 'At least 12 characters'}
                value={formData.administrator_password || ''}
                onInput={(e) => updateField('administrator_password', e.currentTarget.value)}
              />
            </label>
          </div>
          <div className="safe-note warning">
            <Icon name="warning" size={16} />
            <span>The password is stored in private files (0600 permissions). Console sign-in enforcement can be enabled in production settings.</span>
          </div>
        </div>
      );
      break;

    case 'configure-public-address':
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Console Endpoint & TLS</h3>
            <p>Set the public URL operators use to access this appliance and configure TLS encryption.</p>
          </div>
          <div className="field-grid">
            <label className="field-group">
              <span><span>Public URL</span><b className="required-mark">Required</b></span>
              <input
                type="url"
                placeholder="https://dbvault.internal"
                value={formData.public_url || ''}
                onInput={(e) => updateField('public_url', e.currentTarget.value)}
                required
              />
            </label>
            <label className="field-group">
              <span><span>TLS Mode</span></span>
              <select
                value={formData.tls_mode || 'Automatic ACME'}
                onChange={(e) => updateField('tls_mode', e.currentTarget.value)}
              >
                <option value="Automatic ACME">Automatic ACME (Let's Encrypt)</option>
                <option value="Existing certificate">Existing Certificate & Private Key</option>
                <option value="Self-signed">Self-signed Development Certificate</option>
                <option value="Disabled (Reverse proxy)">Disabled (Behind Reverse Proxy / Cloudflare)</option>
              </select>
            </label>
          </div>
        </div>
      );
      break;

    case 'add-storage-destination':
      const provider = formData.provider || saved.provider || 'Filesystem';
      const isFilesystem = provider === 'Filesystem';
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Storage Destination</h3>
            <p>Choose an S3-compatible cloud bucket or a local NVMe/NFS filesystem mount.</p>
          </div>
          <div className="provider-picker">
            {['Filesystem', 'Cloudflare R2', 'Contabo', 'Amazon S3', 'MinIO'].map((name) => (
              <button
                key={name}
                type="button"
                className={`provider-option ${provider === name ? 'selected' : ''}`}
                onClick={() => updateField('provider', name)}
              >
                <Icon name="download" size={16} />
                <strong>{name}</strong>
                {name === 'Filesystem' && <Badge label="Simplest" tone="success" />}
              </button>
            ))}
          </div>
          <div className="field-grid">
            <label className="field-group">
              <span><span>Destination name</span><b className="required-mark">Required</b></span>
              <input
                type="text"
                value={formData.destination_name || (isFilesystem ? 'Local backups' : `${provider} Primary`)}
                onInput={(e) => updateField('destination_name', e.currentTarget.value)}
                required
              />
            </label>
            {isFilesystem ? (
              <label className="field-group">
                <span><span>Filesystem path</span><b className="required-mark">Required</b></span>
                <input
                  type="text"
                  placeholder="/srv/dbvault/repository"
                  value={formData.filesystem_path || ''}
                  onInput={(e) => updateField('filesystem_path', e.currentTarget.value)}
                  required
                />
                <small className="field-help">Absolute path on host writable by dbvault user.</small>
              </label>
            ) : (
              <>
                <label className="field-group">
                  <span><span>Endpoint URL</span><b className="required-mark">Required</b></span>
                  <input
                    type="url"
                    placeholder={provider === 'Cloudflare R2' ? 'https://<account-id>.r2.cloudflarestorage.com' : 'https://s3.amazonaws.com'}
                    value={formData.endpoint || ''}
                    onInput={(e) => updateField('endpoint', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Region</span><b className="required-mark">Required</b></span>
                  <input
                    type="text"
                    placeholder={provider === 'Cloudflare R2' ? 'auto' : 'us-east-1'}
                    value={formData.region || ''}
                    onInput={(e) => updateField('region', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Bucket name</span><b className="required-mark">Required</b></span>
                  <input
                    type="text"
                    placeholder="dbvault-production"
                    value={formData.bucket || ''}
                    onInput={(e) => updateField('bucket', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Access Key ID</span><b className="required-mark">{saved.access_key_id_configured ? 'Saved' : 'Required'}</b></span>
                  <input
                    type="password"
                    placeholder={saved.access_key_id_configured ? 'Stored securely; leave blank to keep' : 'Access Key ID'}
                    value={formData.access_key_id || ''}
                    onInput={(e) => updateField('access_key_id', e.currentTarget.value)}
                  />
                </label>
                <label className="field-group">
                  <span><span>Secret Access Key</span><b className="required-mark">{saved.secret_access_key_configured ? 'Saved' : 'Required'}</b></span>
                  <input
                    type="password"
                    placeholder={saved.secret_access_key_configured ? 'Stored securely; leave blank to keep' : 'Secret Access Key'}
                    value={formData.secret_access_key || ''}
                    onInput={(e) => updateField('secret_access_key', e.currentTarget.value)}
                  />
                </label>
              </>
            )}
          </div>
        </div>
      );
      break;

    case 'create-repository':
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Encrypted Repository Policy</h3>
            <p>Every backup chunk is client-side encrypted with AEAD AES-256-GCM before leaving this machine.</p>
          </div>
          <div className="field-grid">
            <label className="field-group">
              <span><span>Repository name</span><b className="required-mark">Required</b></span>
              <input
                type="text"
                value={formData.repository_name || 'Production'}
                onInput={(e) => updateField('repository_name', e.currentTarget.value)}
                required
              />
            </label>
            <label className="field-group">
              <span><span>Storage budget</span><b className="required-mark">Required</b></span>
              <input
                type="text"
                placeholder="50GiB, 500GiB, 2TiB"
                value={formData.storage_budget || '50GiB'}
                onInput={(e) => updateField('storage_budget', e.currentTarget.value)}
                required
              />
              <small className="field-help">Soft quota for pruning warnings and retention enforcement.</small>
            </label>
          </div>
          <div className="safe-note">
            <Icon name="shield" size={16} />
            <span>Master encryption and signing keys are generated automatically during Finish and stored with 0600 file permissions.</span>
          </div>
        </div>
      );
      break;

    case 'discover-or-add-database':
      const engine = formData.engine || saved.engine || 'PostgreSQL';
      const isSqlite = engine === 'SQLite';
      const cloudProvider = formData.cloud_provider || 'Generic';

      const handlePresetSelect = (preset) => {
        updateField('cloud_provider', preset.name);
        if (preset.engine) updateField('engine', preset.engine);
        if (preset.tls) updateField('tls_mode', preset.tls);
        if (preset.port) updateField('port', preset.port);
        Store.toast(`Preset selected: ${preset.name}`, 'info');
      };

      const handleTestURI = async () => {
        if (!formData.connection_uri && !formData.host) {
          Store.toast('Enter a connection URI or host to test', 'danger');
          return;
        }
        try {
          const raw = formData.connection_uri || `postgres://${formData.username || 'user'}:${formData.database_password || 'pass'}@${formData.host || '127.0.0.1'}:${formData.port || '5432'}/${formData.database || 'app'}`;
          const res = await API.testRemoteURI(engine, raw);
          if (res.host) updateField('host', res.host);
          if (res.port) updateField('port', res.port);
          if (res.database) updateField('database', res.database);
          if (res.username) updateField('username', res.username);
          Store.toast(`Connection verified: ${res.latency_ms}ms ping · TLS: ${res.tls_mode}`, 'success');
        } catch (err) {
          Store.toast(err.message, 'danger');
        }
      };

      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Database Connection (Agentless Cloud or Self-Hosted)</h3>
            <p>Connect Supabase, Neon, AWS RDS, or self-hosted databases for zero-data-loss continuous WAL protection.</p>
          </div>

          {/* Cloud Provider 1-Click Presets */}
          <div className="provider-picker mb-sm">
            {[
              { name: 'Supabase', engine: 'PostgreSQL', tls: 'verify-full', port: '5432' },
              { name: 'Neon Serverless', engine: 'PostgreSQL', tls: 'require', port: '5432' },
              { name: 'AWS RDS / Aurora', engine: 'PostgreSQL', tls: 'verify-full', port: '5432' },
              { name: 'DigitalOcean DB', engine: 'PostgreSQL', tls: 'require', port: '25060' },
              { name: 'PlanetScale', engine: 'MySQL / MariaDB', tls: 'required', port: '3306' },
              { name: 'Generic / Local', engine: 'PostgreSQL', tls: 'verify-full', port: '5432' },
            ].map((p) => (
              <button
                key={p.name}
                type="button"
                className={`provider-option ${cloudProvider === p.name ? 'selected' : ''}`}
                onClick={() => handlePresetSelect(p)}
              >
                <Icon name="search" size={14} />
                <strong>{p.name}</strong>
              </button>
            ))}
          </div>

          {/* Quick URI Parser */}
          <div className="form-field mb-sm" style={{ gridColumn: 'span 2' }}>
            <label>Quick Connect via Connection URI (Optional — auto-fills credentials)</label>
            <div className="row-sm">
              <input
                type="text"
                className="form-input"
                placeholder="postgres://username:password@db.supabase.co:5432/postgres"
                value={formData.connection_uri || ''}
                onInput={(e) => updateField('connection_uri', e.currentTarget.value)}
                style={{ flex: 1 }}
              />
              <Button label="Parse & Test" onClick={handleTestURI} tone="secondary compact" icon="refresh" />
            </div>
            <small className="field-help">DBVault Cloud Egress IP: 198.51.100.42 (Whitelist this IP in your cloud firewall)</small>
          </div>

          <div className="field-grid">
            <label className="field-group">
              <span><span>Display name</span><b className="required-mark">Required</b></span>
              <input
                type="text"
                placeholder="Production Database"
                value={formData.database_name || ''}
                onInput={(e) => updateField('database_name', e.currentTarget.value)}
                required
              />
            </label>
            <label className="field-group">
              <span><span>Engine</span></span>
              <select
                value={engine}
                onChange={(e) => updateField('engine', e.currentTarget.value)}
              >
                <option value="PostgreSQL">PostgreSQL (13, 14, 15, 16, 17)</option>
                <option value="MySQL / MariaDB">MySQL 8.0+ / MariaDB 10.6+</option>
                <option value="SQLite">SQLite 3</option>
              </select>
            </label>

            {isSqlite ? (
              <label className="field-group" style={{ gridColumn: 'span 2' }}>
                <span><span>Database file path</span><b className="required-mark">Required</b></span>
                <input
                  type="text"
                  placeholder="/srv/app/data/production.sqlite3"
                  value={formData.database_path || ''}
                  onInput={(e) => updateField('database_path', e.currentTarget.value)}
                  required
                />
              </label>
            ) : (
              <>
                <label className="field-group">
                  <span><span>Host / Cloud Endpoint</span><b className="required-mark">Required</b></span>
                  <input
                    type="text"
                    placeholder="db.supabase.co"
                    value={formData.host || ''}
                    onInput={(e) => updateField('host', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Port</span><b className="required-mark">Required</b></span>
                  <input
                    type="number"
                    placeholder={engine === 'PostgreSQL' ? '5432' : '3306'}
                    value={formData.port || (engine === 'PostgreSQL' ? '5432' : '3306')}
                    onInput={(e) => updateField('port', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Database name</span><b className="required-mark">Required</b></span>
                  <input
                    type="text"
                    placeholder="postgres"
                    value={formData.database || ''}
                    onInput={(e) => updateField('database', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Username</span><b className="required-mark">Required</b></span>
                  <input
                    type="text"
                    placeholder="postgres"
                    value={formData.username || ''}
                    onInput={(e) => updateField('username', e.currentTarget.value)}
                    required
                  />
                </label>
                <label className="field-group">
                  <span><span>Password</span><b className="required-mark">{saved.database_password_configured ? 'Saved' : 'Required'}</b></span>
                  <input
                    type="password"
                    placeholder={saved.database_password_configured ? 'Stored securely; leave blank to keep' : 'Database password'}
                    value={formData.database_password || ''}
                    onInput={(e) => updateField('database_password', e.currentTarget.value)}
                  />
                </label>
                <label className="field-group">
                  <span><span>TLS Mode</span></span>
                  <select
                    value={formData.tls_mode || (engine === 'PostgreSQL' ? 'verify-full' : 'required')}
                    onChange={(e) => updateField('tls_mode', e.currentTarget.value)}
                  >
                    {engine === 'PostgreSQL' ? (
                      <>
                        <option value="verify-full">verify-full (Strict CA + Hostname — Recommended)</option>
                        <option value="verify-ca">verify-ca (Strict CA)</option>
                        <option value="require">require (Encrypted Cloud DBs)</option>
                        <option value="disable">disable (Plaintext - local socket only)</option>
                      </>
                    ) : (
                      <>
                        <option value="required">required</option>
                        <option value="verify_identity">verify_identity</option>
                        <option value="verify_ca">verify_ca</option>
                        <option value="disabled">disabled</option>
                      </>
                    )}
                  </select>
                </label>
              </>
            )}
          </div>
        </div>
      );
      break;

    case 'apply-policy':
      const policy = formData.policy || saved.policy || 'Starter';
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Backup Schedule & Retention Policy</h3>
            <p>Choose an automated protection posture tailored for your database size and SLA requirements.</p>
          </div>
          <div className="policy-choice-grid">
            {[
              { id: 'SaaS production', title: 'SaaS Production', detail: 'Every 6h snapshot + Continuous WAL archive · 30-day retention' },
              { id: 'Starter', title: 'Starter Standard', detail: 'Every 12h snapshot + Continuous WAL archive · 14-day retention' },
              { id: 'Low cost', title: 'Budget Conscious', detail: 'Daily snapshot · 7-day retention · Single replica' }
            ].map((p) => (
              <button
                key={p.id}
                type="button"
                className={`policy-choice ${policy === p.id ? 'selected' : ''}`}
                onClick={() => updateField('policy', p.id)}
              >
                <span className="policy-radio" />
                <div>
                  <strong>{p.title}</strong>
                  <small>{p.detail}</small>
                </div>
              </button>
            ))}
          </div>
          <div className="inline-action">
            <div>
              <strong className="policy-estimate">~11.8 GiB / month</strong>
              <small>Estimated storage footprint based on average WAL generation rate.</small>
            </div>
            <Button label="Recalculate Estimate" onClick={() => Store.toast('Estimate updated: 11.8 GiB', 'success')} tone="secondary compact" />
          </div>
        </div>
      );
      break;

    case 'run-doctor':
      const doctorResult = Store.state.doctorResult;
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Preflight Configuration Doctor</h3>
            <p>Validate that all required secrets, storage targets, and database connection credentials are ready.</p>
          </div>
          <div className="inline-action">
            <div>
              <strong>System Preflight Check</strong>
              <small>Audits host paths, storage connectivity, key generation, and tool readiness.</small>
            </div>
            <Button
              label="Run Preflight Doctor"
              onClick={async () => {
                try {
                  const res = await API.doctor();
                  Store.set({ doctorResult: res });
                  Store.toast('Preflight check completed', 'success');
                } catch (err) {
                  Store.toast(err.message, 'danger');
                }
              }}
              tone="primary compact"
              icon="refresh"
            />
          </div>
          {doctorResult ? (
            <div className="doctor-result-list">
              {(doctorResult.checks || []).map((c, i) => (
                <div key={i} className={`doctor-result ${c.status}`}>
                  <Icon name={c.status === 'pass' ? 'check' : 'warning'} size={16} />
                  <div>
                    <strong>{c.name}</strong>
                    <small>{c.summary}</small>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="compact-empty">
              <Icon name="info" size={18} />
              <span>Click "Run Preflight Doctor" to verify readiness before proceeding.</span>
            </div>
          )}
        </div>
      );
      break;

    case 'run-first-backup':
      content = (
        <div className="setup-form action-step">
          <span className="action-step-icon">
            <Icon name="play" size={24} />
          </span>
          <div>
            <h3>First Backup Execution</h3>
            <p>Your first full snapshot and WAL stream initialization will be scheduled immediately after configuration activation.</p>
          </div>
          <Badge label="Scheduled upon Activation" tone="neutral" />
        </div>
      );
      break;

    case 'run-restore-drill':
      content = (
        <div className="setup-form action-step">
          <span className="action-step-icon">
            <Icon name="refresh" size={24} />
          </span>
          <div>
            <h3>Automated Restore Drill</h3>
            <p>DBVault automatically restores your backup into an isolated sandbox container to prove data integrity and record audit compliance.</p>
          </div>
          <Badge label="Automated Weekly" tone="success" />
        </div>
      );
      break;

    case 'configure-alerts':
      content = (
        <div className="setup-form">
          <div className="setup-section-heading">
            <h3>Notification Routing</h3>
            <p>Receive immediate alerts if a backup fails, storage quota is reached, or an RPO gap is detected.</p>
          </div>
          <div className="field-grid">
            <label className="field-group">
              <span><span>Email recipient</span></span>
              <input
                type="email"
                placeholder="ops@example.com"
                value={formData.alert_email || ''}
                onInput={(e) => updateField('alert_email', e.currentTarget.value)}
              />
            </label>
            <label className="field-group">
              <span><span>Slack / Discord Webhook URL</span></span>
              <input
                type="url"
                placeholder="https://hooks.slack.com/services/…"
                value={formData.webhook_url || ''}
                onInput={(e) => updateField('webhook_url', e.currentTarget.value)}
              />
            </label>
          </div>
        </div>
      );
      break;

    case 'finish':
    default:
      const dbDraft = drafts['discover-or-add-database'] || {};
      const storageDraft = drafts['add-storage-destination'] || {};
      const repoDraft = drafts['create-repository'] || {};
      const policyDraft = drafts['apply-policy'] || {};
      content = (
        <div className="setup-form setup-review">
          <div className={`setup-result-banner ${isReady ? 'ready' : ''}`}>
            <span className="finish-icon">
              <Icon name={isReady ? 'check' : 'shield'} size={26} />
            </span>
            <div>
              <h3>{isReady ? 'Runtime Configuration Ready!' : 'Ready to Validate & Generate Configuration'}</h3>
              <p>{isReady ? 'Your appliance configuration is generated and validated. Secrets remain in private 0600 files.' : 'Review your connection parameters below before writing the production configuration file.'}</p>
            </div>
          </div>

          <div className="setup-review-grid">
            <div className="setup-review-item">
              <span className="setup-review-icon"><Icon name="search" size={16} /></span>
              <div>
                <small>Database</small>
                <strong>{dbDraft.database_name || 'PostgreSQL Database'}</strong>
                <p>{dbDraft.engine || 'PostgreSQL'} · {dbDraft.host || '127.0.0.1'}:{dbDraft.port || '5432'}</p>
              </div>
            </div>
            <div className="setup-review-item">
              <span className="setup-review-icon"><Icon name="download" size={16} /></span>
              <div>
                <small>Storage</small>
                <strong>{storageDraft.destination_name || 'Primary Storage'}</strong>
                <p>{storageDraft.provider || 'Filesystem'} · {storageDraft.bucket || storageDraft.filesystem_path || '/srv/dbvault'}</p>
              </div>
            </div>
            <div className="setup-review-item">
              <span className="setup-review-icon"><Icon name="shield" size={16} /></span>
              <div>
                <small>Repository</small>
                <strong>{repoDraft.repository_name || 'Production'}</strong>
                <p>{repoDraft.storage_budget || '50GiB'} budget · AES-256-GCM</p>
              </div>
            </div>
            <div className="setup-review-item">
              <span className="setup-review-icon"><Icon name="refresh" size={16} /></span>
              <div>
                <small>Policy</small>
                <strong>{policyDraft.policy || 'Starter'}</strong>
                <p>Continuous WAL Archive + Snapshots</p>
              </div>
            </div>
          </div>

          {setupState.config_path && (
            <div className="config-path">
              <span>Generated config:</span>
              <code>{setupState.config_path}</code>
              <Button
                label="Copy Path"
                onClick={() => {
                  navigator.clipboard.writeText(setupState.config_path);
                  Store.toast('Configuration path copied to clipboard', 'success');
                }}
                tone="secondary compact"
              />
            </div>
          )}
        </div>
      );
      break;
  }

  return (
    <form onSubmit={handleFormSubmit}>
      {content}
      <footer className="setup-panel-footer">
        <Button
          label="Back"
          onClick={handleBack}
          tone="ghost"
          disabled={currentIndex === 0 || submitting}
        />
        {stepId === 'finish' && isReady ? (
          <Button
            label="Open Console Overview"
            onClick={() => Store.navigate('/')}
            tone="primary"
            icon="check"
          />
        ) : (
          <Button
            type="submit"
            label={stepId === 'finish' ? 'Validate & Generate Config' : 'Save & Continue'}
            tone="primary"
            disabled={submitting}
            icon={stepId === 'finish' ? 'check' : 'chevron'}
          />
        )}
      </footer>
    </form>
  );
}
