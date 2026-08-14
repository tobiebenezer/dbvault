import { API } from '../api.js';
import { h, button, badge, mountAsync, pageHeader, icon } from '../components/ui.js';
import { Store } from '../state.js';
import { formatBytes, titleCase } from '../format.js';
import { ProductActions } from '../actions.js';

const steps = [
  ['create-administrator', 'Owner', 'setup'],
  ['configure-public-address', 'Console', 'shield'],
  ['add-storage-destination', 'Storage', 'storage'],
  ['create-repository', 'Repository', 'repository'],
  ['discover-or-add-database', 'Database', 'database'],
  ['apply-policy', 'Policy', 'settings'],
  ['run-doctor', 'Doctor', 'activity'],
  ['run-first-backup', 'Backup', 'play'],
  ['run-restore-drill', 'Restore test', 'recovery'],
  ['configure-alerts', 'Alerts', 'alert'],
  ['finish', 'Review', 'check']
];

export function SetupPage() {
  const root = h('div');
  mountAsync(root, () => API.setup(), renderSetup);
  return root;
}

function renderSetup(state) {
  const completed = new Set(state.completed_steps || []);
  const current = state.current_step || 'create-administrator';
  const currentIndex = Math.max(0, steps.findIndex(([id]) => id === current));
  const step = steps[currentIndex] || steps[0];
  const drafts = state.drafts || {};
  const body = currentStepForm(current, drafts, state);
  const configurationReady = state.status === 'configuration_ready';
  return h('div', { class: 'page setup-page' }, [
    pageHeader({
      eyebrow: 'First-run configuration',
      title: 'Set up DBVault',
      description: 'Connect one database to storage. Sensitive values are written to private files and never returned by this page.',
      actions: [button('Exit setup', () => Store.navigate('/'), 'secondary')]
    }),
    h('div', { class: 'setup-progress-compact' }, [
      h('div', { class: 'setup-progress-label' }, [
        h('strong', { text: `Step ${currentIndex + 1} of ${steps.length}` }),
        h('span', { text: `${step[1]} · saved after validation` })
      ]),
      h('div', { class: 'setup-progress-track' }, h('span', { class: `progress-${Math.round(((currentIndex + 1) / steps.length) * 4) * 25}` })),
      h('div', { class: 'setup-step-dots' }, steps.map(([id, label], index) => h('button', {
        class: `${completed.has(id) ? 'done' : ''} ${index === currentIndex ? 'current' : ''}`,
        title: label,
        type: 'button',
        'aria-label': `Go to ${label}`,
        onclick: () => goToStep(id)
      })))
    ]),
    h('section', { class: 'setup-panel' }, [
      h('header', { class: 'setup-panel-head' }, [
        h('span', { class: 'setup-current-icon' }, icon(step[2], 22)),
        h('div', {}, [h('h2', { text: step[1] }), h('p', { text: stepHint(current) })])
      ]),
      body,
      h('footer', { class: 'setup-panel-footer' }, [
        button('Back', () => goToStep(steps[Math.max(0, currentIndex - 1)][0]), 'ghost', { disabled: currentIndex === 0 }),
        button(
          current === 'finish' ? (configurationReady ? 'Open overview' : 'Validate and generate config') : 'Save and continue',
          () => configurationReady && current === 'finish' ? Store.navigate('/') : completeStep(current, body),
          'primary',
          { trailingIcon: current === 'finish' ? 'check' : 'arrow' }
        )
      ])
    ])
  ]);
}

function stepHint(id) {
  return ({
    'create-administrator': 'Record the appliance owner. Console authentication is a separate production-readiness item.',
    'configure-public-address': 'Set the address operators will use and document the intended TLS mode.',
    'add-storage-destination': 'Choose a filesystem directory or an S3-compatible object store.',
    'create-repository': 'Define encryption, retention, and the maximum storage budget.',
    'discover-or-add-database': 'Enter the database name, login, TLS policy, and connection timeout.',
    'apply-policy': 'Choose the initial backup schedule and retention posture.',
    'run-doctor': 'Review configuration readiness before generating runtime files.',
    'run-first-backup': 'Backup execution starts only after the generated configuration is activated.',
    'run-restore-drill': 'Restore verification requires a real backup from the activated runtime.',
    'configure-alerts': 'Record where action-required notifications should be delivered.',
    finish: 'Review the connection graph, then generate a validated runtime configuration.'
  }[id] || 'Complete this step to continue.');
}

function currentStepForm(current, drafts, state) {
  const saved = drafts[current] || {};
  if (current === 'create-administrator') {
    return formSection('Appliance owner', 'Used for ownership metadata. Authentication is not enabled by this setup pass.', [
      setupField('Administrator email', 'administrator_email', saved.administrator_email || '', { type: 'email', placeholder: 'admin@example.com', autocomplete: 'email', required: true }),
      setupField('Password', 'administrator_password', '', { type: 'password', placeholder: saved.administrator_password_configured ? 'Stored securely; leave blank to keep' : 'At least 12 characters', autocomplete: 'new-password', required: !saved.administrator_password_configured })
    ], warningNote('The password is stored privately, but console sign-in enforcement is not yet production-ready.'));
  }
  if (current === 'configure-public-address') {
    return formSection('Console endpoint', 'These values document the intended deployment endpoint; certificate automation remains installer-managed.', [
      setupField('Public URL', 'public_url', saved.public_url || '', { type: 'url', placeholder: 'https://backup.example.com', required: true }),
      setupSelectField('TLS mode', 'tls_mode', ['Automatic ACME', 'Existing certificate', 'Self-signed'], saved.tls_mode || 'Automatic ACME')
    ]);
  }
  if (current === 'add-storage-destination') return storageForm(saved);
  if (current === 'create-repository') {
    return formSection('Repository policy', 'Every backup object is encrypted before it leaves this appliance.', [
      setupField('Repository name', 'repository_name', saved.repository_name || 'Production', { required: true }),
      setupSelectField('Mode', 'repository_mode', ['Single destination'], saved.repository_mode || 'Single destination'),
      setupSelectField('Encryption', 'encryption', ['Generated local key'], saved.encryption || 'Generated local key'),
      setupField('Storage budget', 'storage_budget', saved.storage_budget || '20GiB', { help: 'Examples: 20GiB, 500GiB, 2TiB', required: true })
    ], h('div', { class: 'safe-note' }, [icon('shield', 17), h('span', { text: 'A repository encryption key and signing key are generated once during Finish.' })]));
  }
  if (current === 'discover-or-add-database') return databaseForm(saved);
  if (current === 'apply-policy') return policyForm(saved);
  if (current === 'run-doctor') {
    return h('div', { class: 'setup-form' }, [
      h('div', { class: 'inline-action' }, [
        h('span', {}, [h('strong', { text: 'Configuration readiness' }), h('small', { text: 'Checks whether required setup values are present. It is not a live database or object-store probe.' })]),
        button('Run readiness check', () => ProductActions.doctor(), 'primary compact', { icon: 'activity' })
      ]),
      Store.state.doctorResult ? doctorResults(Store.state.doctorResult) : h('div', { class: 'compact-empty' }, [icon('activity', 20), h('span', { text: 'Readiness check has not run in this browser session' })])
    ]);
  }
  if (current === 'run-first-backup') return deferredAction('First backup', 'The current web server still uses the product-experience service. Generate and activate the runtime config before relying on backup results.', 'Configuration only in this pass');
  if (current === 'run-restore-drill') return deferredAction('Restore verification', 'A restore drill needs a backup created by the activated runtime. This step records the intended workflow without fabricating a successful restore.', 'Requires a real backup');
  if (current === 'configure-alerts') {
    return formSection('Notification routing', 'Notification delivery is recorded for the generated configuration roadmap; no test message is sent here.', [
      setupField('Email recipient', 'alert_email', saved.alert_email || '', { type: 'email', placeholder: 'ops@example.com' }),
      setupField('Webhook URL', 'webhook_url', saved.webhook_url || '', { type: 'url', placeholder: 'https://example.com/dbvault-webhook' })
    ]);
  }
  return finishReview(drafts, state);
}

function storageForm(saved) {
  const provider = Store.state.setupDraft.provider || saved.provider || 'Filesystem';
  const isFilesystem = provider === 'Filesystem';
  const defaults = storageDefaults(provider);
  const fields = isFilesystem ? [
    setupField('Destination name', 'destination_name', saved.destination_name || 'Local backups', { required: true }),
    setupField('Filesystem path', 'filesystem_path', saved.filesystem_path || '', { placeholder: '/srv/dbvault/repository', help: 'Absolute path writable by the DBVault service user.', required: true })
  ] : [
    setupField('Destination name', 'destination_name', saved.destination_name || `${provider} primary`, { required: true }),
    setupField('Endpoint URL', 'endpoint', saved.endpoint || defaults.endpoint, { type: 'url', placeholder: defaults.endpointPlaceholder, required: true }),
    setupField('Region', 'region', saved.region || defaults.region, { required: true }),
    setupField('Bucket', 'bucket', saved.bucket || '', { placeholder: 'dbvault-production', required: true }),
    setupField('Object prefix', 'prefix', saved.prefix || 'backups/production', { help: 'Optional folder-like prefix inside the bucket.' }),
    setupSelectField('Addressing', 'addressing', ['Virtual hosted', 'Path style'], saved.addressing || defaults.addressing),
    setupField('Access key ID', 'access_key_id', '', { type: 'password', placeholder: saved.access_key_id_configured ? 'Stored securely; leave blank to keep' : 'Enter access key ID', autocomplete: 'off', required: !saved.access_key_id_configured }),
    setupField('Secret access key', 'secret_access_key', '', { type: 'password', placeholder: saved.secret_access_key_configured ? 'Stored securely; leave blank to keep' : 'Enter secret access key', autocomplete: 'new-password', required: !saved.secret_access_key_configured })
  ];
  return h('div', { class: 'setup-form' }, [
    h('input', { type: 'hidden', 'data-field': 'provider', value: provider }),
    h('div', { class: 'setup-section-heading' }, [h('h3', { text: 'Storage provider' }), h('p', { text: 'Credentials are stored as private files and referenced by path from the generated config.' })]),
    h('div', { class: 'provider-picker compact' }, ['Filesystem', 'Cloudflare R2', 'Contabo', 'Amazon S3'].map((name) => h('button', {
      class: `provider-option ${provider === name ? 'selected' : ''}`,
      type: 'button',
      onclick: () => Store.set({ setupDraft: { ...Store.state.setupDraft, provider: name } })
    }, [icon('storage', 18), h('strong', { text: name }), name === 'Filesystem' ? badge('Simplest', 'success') : null]))),
    h('div', { class: 'field-grid setup-fields' }, fields),
    h('div', { class: 'safe-note' }, [icon('shield', 17), h('span', { text: 'A real destination capability probe is required after runtime activation; this screen does not simulate one.' })])
  ]);
}

function databaseForm(saved) {
  const engine = Store.state.setupDraft.engine || saved.engine || 'PostgreSQL';
  const sqlite = engine === 'SQLite';
  const defaults = engine === 'MySQL / MariaDB' ? { port: '3306', tls: 'required' } : { port: '5432', tls: 'verify-full' };
  const fields = [
    setupField('Display name', 'database_name', saved.database_name || '', { placeholder: 'Production database', required: true }),
    setupSelectField('Engine', 'engine', ['PostgreSQL', 'MySQL / MariaDB', 'SQLite'], engine, { onchange: (event) => Store.set({ setupDraft: { ...Store.state.setupDraft, engine: event.target.value } }) }),
    ...(sqlite ? [
      setupField('Database file path', 'database_path', saved.database_path || '', { placeholder: '/srv/app/data/application.sqlite', help: 'Absolute path to the SQLite database file.', required: true })
    ] : [
      setupField('Host', 'host', saved.host || '127.0.0.1', { placeholder: 'db.internal', required: true }),
      setupField('Port', 'port', saved.port || defaults.port, { type: 'number', min: '1', max: '65535', required: true }),
      setupField('Database name', 'database', saved.database || '', { placeholder: 'application_production', required: true }),
      setupField('Username', 'username', saved.username || '', { placeholder: 'dbvault_backup', autocomplete: 'username', required: true }),
      setupField('Password', 'database_password', '', { type: 'password', placeholder: saved.database_password_configured ? 'Stored securely; leave blank to keep' : 'Database password', autocomplete: 'new-password', required: !saved.database_password_configured }),
      setupSelectField('TLS mode', 'tls_mode', engine === 'PostgreSQL' ? ['verify-full', 'verify-ca', 'require', 'disable'] : ['required', 'verify_identity', 'verify_ca', 'disabled'], saved.tls_mode || defaults.tls),
      setupField('Root CA certificate', 'root_certificate', saved.root_certificate || '', { placeholder: '/etc/dbvault/certificates/database-ca.pem', help: 'Recommended for verify modes; leave empty only when system trust is sufficient.' }),
      setupField('Connection timeout', 'connect_timeout', saved.connect_timeout || '15s', { help: 'Examples: 10s, 30s, 1m', required: true })
    ])
  ];
  return h('div', { class: 'setup-form' }, [
    h('div', { class: 'setup-section-heading' }, [h('h3', { text: 'Source connection' }), h('p', { text: sqlite ? 'DBVault reads this local file through its SQLite backup runtime.' : 'Use a dedicated least-privilege backup account. The password is never placed in setup state.' })]),
    h('div', { class: 'field-grid setup-fields' }, fields),
    h('div', { class: 'safe-note warning' }, [icon('warning', 17), h('span', { text: sqlite ? 'The service user must be able to read the database and write to its parent directory for safe snapshots.' : 'PostgreSQL/MySQL connection settings are generated now, but standalone backup execution for these engines remains an alpha integration.' })])
  ]);
}

function policyForm(saved) {
  const selected = Store.state.setupDraft.policy || saved.policy || 'Starter';
  return h('div', { class: 'setup-form' }, [
    h('input', { type: 'hidden', 'data-field': 'policy', value: selected }),
    h('div', { class: 'policy-choice-grid compact' }, ['SaaS production', 'Starter', 'Low cost'].map((policy) => setupPolicyChoice(policy, policyDetail(policy), selected === policy))),
    h('div', { class: 'inline-action' }, [
      h('span', {}, [h('strong', { class: 'policy-estimate', text: formatBytes(Store.state.setupDraft.estimatedBytes || 11800000000) }), h('small', { text: 'Illustrative 30-day estimate; actual usage depends on database change rate.' })]),
      button('Recalculate estimate', recalculatePolicy, 'secondary compact')
    ])
  ]);
}

function finishReview(drafts, state) {
  const storage = drafts['add-storage-destination'] || {};
  const database = drafts['discover-or-add-database'] || {};
  const repository = drafts['create-repository'] || {};
  const policy = drafts['apply-policy'] || {};
  const ready = state.status === 'configuration_ready';
  return h('div', { class: 'setup-form setup-review' }, [
    h('div', { class: `setup-result-banner ${ready ? 'ready' : ''}` }, [
      h('span', { class: 'finish-icon' }, icon(ready ? 'check' : 'shield', 26)),
      h('div', {}, [
        h('h3', { text: ready ? 'Runtime configuration generated' : 'Ready to validate the connection graph' }),
        h('p', { text: ready ? 'Secrets and repository keys remain in private files. Restart DBVault with the generated config to activate it.' : 'Finish validates the complete source → repository → destination graph before writing any runtime config.' })
      ])
    ]),
    h('div', { class: 'setup-review-grid' }, [
      reviewItem('Database', database.database_name || 'Not configured', `${database.engine || 'Unknown engine'}${database.host ? ` · ${database.host}:${database.port}` : database.database_path ? ` · ${database.database_path}` : ''}`, 'database'),
      reviewItem('Storage', storage.destination_name || 'Not configured', `${storage.provider || 'Unknown provider'}${storage.bucket ? ` · ${storage.bucket}` : storage.filesystem_path ? ` · ${storage.filesystem_path}` : ''}`, 'storage'),
      reviewItem('Repository', repository.repository_name || 'Production', `${repository.storage_budget || '20GiB'} budget · encrypted`, 'repository'),
      reviewItem('Schedule', policy.policy || 'Starter', policyDetail(policy.policy || 'Starter'), 'clock')
    ]),
    ready ? h('div', { class: 'config-path' }, [h('span', { text: 'Generated config' }), h('code', { text: state.config_path }), button('Copy path', () => copyText(state.config_path), 'secondary compact', { icon: 'copy' })]) : null,
    h('div', { class: 'safe-note warning' }, [icon('warning', 17), h('span', { text: 'Generating configuration does not prove a backup. Protection status must remain unverified until a real backup and restore drill complete.' })])
  ]);
}

function formSection(title, description, fields, after = null) {
  return h('div', { class: 'setup-form' }, [
    h('div', { class: 'setup-section-heading' }, [h('h3', { text: title }), h('p', { text: description })]),
    h('div', { class: 'field-grid setup-fields' }, fields),
    after
  ]);
}

function deferredAction(title, text, status) {
  return h('div', { class: 'setup-form action-step' }, [
    h('span', { class: 'action-step-icon' }, icon('clock', 24)),
    h('div', {}, [h('h3', { text: title }), h('p', { text })]),
    badge(status, 'warning')
  ]);
}

function setupField(label, name, value = '', options = {}) {
  const attrs = {
    type: options.type || 'text',
    value,
    placeholder: options.placeholder || '',
    'data-field': name,
    autocomplete: options.autocomplete,
    required: options.required || false,
    min: options.min,
    max: options.max
  };
  return h('label', { class: 'field-group' }, [
    h('span', {}, [h('span', { text: label }), options.required ? h('b', { class: 'required-mark', text: 'Required' }) : null]),
    h('input', attrs),
    options.help ? h('small', { class: 'field-help', text: options.help }) : null
  ]);
}

function setupSelectField(label, name, options, selected, attrs = {}) {
  return h('label', { class: 'field-group' }, [
    h('span', { text: label }),
    h('select', { 'data-field': name, onchange: attrs.onchange }, options.map((option) => h('option', { text: option, value: option, selected: option === selected })))
  ]);
}

function setupPolicyChoice(title, detail, selected = false) {
  return h('button', {
    class: `policy-choice ${selected ? 'selected' : ''}`,
    type: 'button',
    onclick: () => Store.set({ setupDraft: { ...Store.state.setupDraft, policy: title } })
  }, [h('span', { class: 'policy-radio' }), h('div', {}, [h('strong', { text: title }), h('small', { text: detail })])]);
}

function policyDetail(title) {
  if (title === 'SaaS production') return 'Every 6h · weekly restore target';
  if (title === 'Starter') return 'Every 12h · monthly restore target';
  return 'Daily backup · budget-aware retention';
}

function reviewItem(label, value, detail, iconName) {
  return h('article', { class: 'setup-review-item' }, [
    h('span', { class: 'setup-review-icon' }, icon(iconName, 18)),
    h('div', {}, [h('small', { text: label }), h('strong', { text: value }), h('p', { text: detail })])
  ]);
}

function warningNote(text) {
  return h('div', { class: 'safe-note warning' }, [icon('warning', 17), h('span', { text })]);
}

function doctorResults(result) {
  return h('div', { class: 'doctor-result-list' }, (result.checks || []).map((check) => h('div', { class: `doctor-result ${check.status}` }, [
    icon(check.status === 'pass' ? 'check' : 'warning', 16),
    h('div', {}, [h('strong', { text: check.name }), h('small', { text: check.summary })])
  ])));
}

function storageDefaults(provider) {
  if (provider === 'Cloudflare R2') return { endpoint: '', endpointPlaceholder: 'https://<account-id>.r2.cloudflarestorage.com', region: 'auto', addressing: 'Virtual hosted' };
  if (provider === 'Contabo') return { endpoint: '', endpointPlaceholder: 'https://<region>.contabostorage.com', region: 'default', addressing: 'Path style' };
  return { endpoint: 'https://s3.amazonaws.com', endpointPlaceholder: 'https://s3.amazonaws.com', region: 'us-east-1', addressing: 'Virtual hosted' };
}

async function goToStep(step) {
  try {
    await API.setSetupStep(step);
    Store.set({ setupDraft: { provider: null, policy: null, engine: null } });
    Store.refresh();
  } catch (error) {
    Store.toast(error.message, 'danger');
  }
}

async function completeStep(current, body) {
  try {
    if (current === 'finish') {
      await API.finishSetup();
      Store.toast('Validated runtime configuration generated', 'success');
      Store.refresh();
      return;
    }
    const draft = collectDraft(body);
    if (current === 'add-storage-destination') draft.provider = Store.state.setupDraft.provider || draft.provider || 'Filesystem';
    if (current === 'apply-policy') draft.policy = Store.state.setupDraft.policy || draft.policy || 'Starter';
    await API.saveSetupStep(current, draft);
    Store.set({ setupDraft: { provider: null, policy: null, engine: null } });
    Store.toast(`${titleCase(current)} saved`, 'success');
    Store.refresh();
  } catch (error) {
    Store.toast(error.message, 'danger');
  }
}

function collectDraft(root) {
  const out = {};
  root.querySelectorAll('[data-field]').forEach((field) => { out[field.getAttribute('data-field')] = field.value; });
  return out;
}

async function recalculatePolicy() {
  try {
    const selected = Store.state.setupDraft.policy || 'Starter';
    const frequency = selected === 'SaaS production' ? 'Every 6 hours' : selected === 'Starter' ? 'Every 12 hours' : 'Every 24 hours';
    const result = await API.simulatePolicy({ policy: { backup_frequency: frequency, replica_count: selected === 'Low cost' ? 1 : 2, monthly_retention: 3 } });
    Store.set({ setupDraft: { ...Store.state.setupDraft, estimatedBytes: result.estimated_thirty_day_bytes } });
    Store.toast(`Estimate: ${formatBytes(result.estimated_thirty_day_bytes)}`, 'success');
  } catch (error) {
    Store.toast(error.message, 'danger');
  }
}

async function copyText(value) {
  try {
    await navigator.clipboard.writeText(value);
    Store.toast('Configuration path copied', 'success');
  } catch {
    Store.toast('Could not access the clipboard', 'danger');
  }
}
