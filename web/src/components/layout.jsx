import { useState, useEffect } from 'preact/hooks';
import { Icon, IconButton, Button } from './ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { API } from '../api.js';

const navGroups = [
  {
    label: 'Protection & Recovery',
    items: [
      ['/', 'Overview', 'search'],
      ['/databases', 'Databases & Schedules', 'shield'],
      ['/recovery', 'Recovery & PITR', 'refresh'],
      ['/repositories', 'Storage & WORM', 'download']
    ]
  },
  {
    label: 'Data Lakehouse & OLAP',
    items: [
      ['/warehouse', 'Warehouse & Analytics', 'database']
    ]
  },
  {
    label: 'Operations & Management',
    items: [
      ['/jobs', 'Activity & Audit', 'play'],
      ['/settings', 'Settings & Team', 'info']
    ]
  }
];

export function Layout({ children }) {
  const [state, setState] = useState(Store.state);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showAbout, setShowAbout] = useState(false);

  useEffect(() => {
    const handleGlobalKeys = (e) => {
      // Don't trigger shortcuts if user is typing in an input
      const isInput = ['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName);

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        Store.set({ commandOpen: !Store.state.commandOpen });
        return;
      }
      if (e.key === '?' && !isInput && !e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        setShowShortcuts((prev) => !prev);
        return;
      }
      if (e.key === 'Escape') {
        setShowShortcuts(false);
        setShowAbout(false);
        Store.set({ commandOpen: false, workspaceOpen: false });
      }
    };

    window.addEventListener('keydown', handleGlobalKeys);
    return () => window.removeEventListener('keydown', handleGlobalKeys);
  }, []);

  useEffect(() => {
    return Store.subscribe((next) => setState({ ...next }));
  }, []);

  const navOpen = Boolean(state.mobileNavOpen);

  return (
    <div className={`shell ${navOpen ? 'nav-open' : ''}`}>
      <button
        className="mobile-scrim"
        type="button"
        aria-label="Close menu"
        onClick={() => Store.set({ mobileNavOpen: false })}
      />
      <Sidebar
        state={state}
        onOpenShortcuts={() => setShowShortcuts(true)}
        onOpenAbout={() => setShowAbout(true)}
      />
      <main className="main" id="main">
        <Topbar state={state} onOpenShortcuts={() => setShowShortcuts(true)} />
        <div className="page-wrap">{children}</div>
      </main>
      <Toast state={state} />
      <CommandPalette
        open={Boolean(state.commandOpen)}
        onOpenAbout={() => setShowAbout(true)}
      />
      {showShortcuts && <ShortcutsModal onClose={() => setShowShortcuts(false)} />}
      {showAbout && <AboutModal onClose={() => setShowAbout(false)} />}
      {state.confirmModal && <ConfirmationModal config={state.confirmModal} />}
    </div>
  );
}

function Sidebar({ state, onOpenShortcuts, onOpenAbout }) {
  const currentRoute = state.route;
  const workspaceOpen = Boolean(state.workspaceOpen);
  const alertBadge = state.alerts?.badge;

  const navClick = (path) => (e) => {
    e.preventDefault();
    Store.navigate(path);
  };

  const isLinkActive = (path) => {
    return currentRoute === path || (path !== '/' && currentRoute.startsWith(path));
  };

  return (
    <aside className="sidebar" aria-label="Primary navigation">
      <div className="sidebar-head">
        <a className="brand" href="/" onClick={navClick('/')}>
          <img src="/logo.png" alt="DBVault" className="brand-logo-img" style={{ width: '32px', height: '32px', borderRadius: '8px', objectFit: 'cover', flexShrink: 0, boxShadow: '0 2px 8px rgba(16, 185, 129, 0.25)' }} />
          <div className="brand-copy">
            <span className="brand-title">DBVault</span>
            <span className="brand-sub">Enterprise Backup Engine</span>
          </div>
        </a>
        <IconButton
          name="close"
          label="Close menu"
          onClick={() => Store.set({ mobileNavOpen: false })}
          className="sidebar-close"
        />
      </div>

      <div className="workspace-picker">
        <button
          className="workspace-current"
          onClick={() => Store.set({ workspaceOpen: !workspaceOpen })}
          type="button"
          aria-expanded={workspaceOpen}
        >
          <div className="workspace-icon">
            <Icon name="shield" size={16} />
          </div>
          <div className="workspace-meta">
            <span className="workspace-name">DBVault Engine</span>
            <span className="workspace-tier">Enterprise Appliance</span>
          </div>
          <Icon name="chevron" size={14} className="workspace-chevron" />
        </button>

        {workspaceOpen && (
          <div className="workspace-dropdown">
            <button
              type="button"
              className="workspace-option active"
              onClick={() => Store.set({ workspaceOpen: false })}
            >
              <strong>Default Project</strong>
              <Icon name="check" size={14} />
            </button>
            <button
              type="button"
              className="workspace-option"
              onClick={() => {
                Store.set({ workspaceOpen: false });
                Store.navigate('/settings');
              }}
            >
              <span>Manage workspaces & nodes</span>
            </button>
          </div>
        )}
      </div>

      <nav className="nav-groups">
        {navGroups.map((group) => (
          <section key={group.label} className="nav-group">
            <span className="nav-label">{group.label}</span>
            {group.items.map(([path, label, iconName]) => {
              const active = isLinkActive(path);
              return (
                <a
                  key={path}
                  className={`nav-link ${active ? 'active' : ''}`}
                  href={path}
                  onClick={navClick(path)}
                >
                  <Icon name={iconName} size={15} />
                  <span>{label}</span>
                  {path === '/alerts' && alertBadge ? (
                    <span className="nav-count alert">{alertBadge}</span>
                  ) : null}
                </a>
              );
            })}
          </section>
        ))}
      </nav>

      <div className="sidebar-footer">
        <button
          type="button"
          className="footer-shortcut-btn"
          onClick={async () => {
            try {
              await API.logout();
            } catch (_) {}
            window.dispatchEvent(new CustomEvent('dbvault:unauthenticated'));
          }}
          title="Sign out of DBVault"
        >
          <span>Sign Out</span>
          <Icon name="close" size={12} />
        </button>
        <button
          type="button"
          className="footer-shortcut-btn"
          onClick={onOpenAbout}
          title="About DBVault"
        >
          <span>About</span>
          <Icon name="info" size={12} />
        </button>
        <button type="button" className="footer-shortcut-btn" onClick={onOpenShortcuts}>
          <span>Shortcuts</span>
          <kbd>?</kbd>
        </button>
        <button
          type="button"
          className="version-tag"
          onClick={onOpenAbout}
          style={{ background: 'none', border: 'none', cursor: 'pointer', textAlign: 'left', padding: '2px 0' }}
          title="About DBVault Appliance"
        >
          v0.1-alpha · Production Ready
        </button>
      </div>
    </aside>
  );
}

function Topbar({ state, onOpenShortcuts }) {
  const connectionStatus = state.connection?.status || 'disconnected';

  const handleStatusClick = () => {
    if (connectionStatus !== 'connected') {
      import('../realtime/job-events.js').then(({ forceReconnect }) => {
        forceReconnect();
        Store.toast('Reconnecting live events stream…', 'info');
      });
    }
  };

  return (
    <header className="topbar">
      <div className="row-sm">
        <IconButton
          name="menu"
          label="Open navigation menu"
          onClick={() => Store.set({ mobileNavOpen: true })}
          className="mobile-menu-btn"
        />
        <button
          className="search-button"
          type="button"
          onClick={() => Store.set({ commandOpen: true })}
        >
          <Icon name="search" size={14} />
          <span>Search databases, storage, commands…</span>
          <kbd>⌘K</kbd>
        </button>
      </div>

      <div className="topbar-actions">
        <button
          type="button"
          className={`realtime-status-btn realtime-status ${connectionStatus}`}
          onClick={handleStatusClick}
          title={connectionStatus === 'connected' ? 'Live stream active' : 'Click to reconnect live event stream'}
        >
          <span className="realtime-dot" />
          <span>{connectionStatus === 'connected' ? 'Live Events' : connectionStatus === 'reconnecting' ? 'Reconnecting…' : 'Offline'}</span>
        </button>
        <Button
          label="Back Up Now"
          onClick={() => ProductActions.backup()}
          tone="primary compact"
          icon="play"
        />
      </div>
    </header>
  );
}

function Toast({ state }) {
  const toast = state.toast;
  if (!toast) return <div className="toast-region" aria-live="polite" aria-atomic="true" />;

  return (
    <div className="toast-region" aria-live="polite" aria-atomic="true">
      <div className={`toast ${toast.tone}`}>
        <span>{toast.message}</span>
        <IconButton
          name="close"
          label="Dismiss"
          onClick={() => Store.set({ toast: null })}
          className="toast-close"
        />
      </div>
    </div>
  );
}

export function CommandPalette({ open, onOpenAbout }) {
  const [query, setQuery] = useState('');

  const commands = [
    { label: 'Back Up Primary Database', description: 'Run immediate snapshot & WAL checkpoint', run: () => ProductActions.backup() },
    { label: 'Restore Database (Recovery Studio)', description: 'Point-in-Time Recovery & restore tools', run: () => Store.navigate('/recovery') },
    { label: 'Run Automated Restore Drill', description: 'Validate cryptographic checksums & table consistency', run: () => ProductActions.restoreDrill() },
    { label: 'Add & Discover Databases', description: 'Connect PostgreSQL, MySQL, MariaDB, or SQLite', run: () => Store.navigate('/databases') },
    { label: 'Configure Cloud Storage & WORM', description: 'Connect Cloudflare R2, AWS S3, MinIO, or Contabo', run: () => Store.navigate('/repositories') },
    { label: 'Query Data Lakehouse & Parquet', description: 'SQL analytics on historical database snapshots', run: () => Store.navigate('/warehouse') },
    { label: 'Run Appliance System Doctor', description: 'Self-diagnose storage, engine, and network health', run: () => ProductActions.doctor() },
    { label: 'Create Disaster Recovery Kit', description: 'Export Master Key sheet and offline restore bundle', run: async () => {
      try {
        const res = await API.recoveryBundle();
        Store.toast(`Recovery bundle created: ${res.path}`, 'success');
      } catch (err) {
        Store.toast(err.message, 'danger');
      }
    }},
    { label: 'View all operations & jobs', description: 'Monitor running tasks and logs', run: () => Store.navigate('/jobs') },
    { label: 'Check alerts & incident notifications', description: 'Inspect active warnings and health checks', run: () => Store.navigate('/alerts') },
    { label: 'Configure Notification Webhooks', description: 'Slack, Discord, and Teams alerting', run: () => Store.navigate('/settings') },
    { label: 'About DBVault Appliance', description: 'Appliance version, encryption specs, and system information', run: () => { if (onOpenAbout) onOpenAbout(); } }
  ];

  const matches = commands.filter(({ label, description }) =>
    `${label} ${description}`.toLowerCase().includes(query.trim().toLowerCase())
  );

  if (!open) return <div className="command-overlay hidden" role="dialog" aria-modal="true" aria-label="Command palette" />;

  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
      onClick={(e) => {
        if (e.target === e.currentTarget) Store.set({ commandOpen: false });
      }}
    >
      <div className="command-panel">
        <div className="command-search">
          <Icon name="search" size={16} />
          <input
            className="command-input"
            placeholder="Type a command or jump to page…"
            aria-label="Command search"
            autoFocus
            value={query}
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
          <kbd>Esc</kbd>
        </div>
        <div className="command-list">
          {matches.length > 0 ? (
            matches.map(({ label, description, run }, i) => (
              <button
                key={i}
                className="command-item"
                type="button"
                onClick={() => {
                  Store.set({ commandOpen: false });
                  run();
                }}
              >
                <div>
                  <strong>{label}</strong>
                  <small className="text-muted"> — {description}</small>
                </div>
                <Icon name="chevron" size={14} />
              </button>
            ))
          ) : (
            <div className="command-empty-box">No matching commands</div>
          )}
        </div>
      </div>
    </div>
  );
}

function ShortcutsModal({ onClose }) {
  const shortcuts = [
    { key: '⌘K / Ctrl+K', desc: 'Open Command Palette & Search' },
    { key: '?', desc: 'Toggle Keyboard Shortcuts modal' },
    { key: 'Esc', desc: 'Close open dialog, palette, or menu' }
  ];

  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="command-panel" style={{ maxWidth: '480px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Keyboard Shortcuts</h2>
            <p className="card-subtitle">Quick navigation and operator controls.</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <div className="stack-sm">
          {shortcuts.map((s, idx) => (
            <div key={idx} className="row-between list-item-row" style={{ padding: '8px 12px' }}>
              <span>{s.desc}</span>
              <kbd style={{ background: 'var(--panel-inset)', border: '1px solid var(--line-strong)', borderRadius: '4px', padding: '2px 8px', fontSize: '11px' }}>
                {s.key}
              </kbd>
            </div>
          ))}
        </div>

        <div className="row-actions mt-md">
          <Button label="Close" onClick={onClose} tone="ghost" />
        </div>
      </div>
    </div>
  );
}

function AboutModal({ onClose }) {
  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="About DBVault"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="command-panel" style={{ maxWidth: '520px', padding: '28px' }}>
        <div className="row-between mb-md">
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <img src="/logo.png" alt="DBVault" style={{ width: '36px', height: '36px', borderRadius: '8px', objectFit: 'cover', flexShrink: 0, boxShadow: '0 2px 8px rgba(16, 185, 129, 0.25)' }} />
            <h2 style={{ margin: 0, fontSize: '18px' }}>DBVault Appliance</h2>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <p className="text-sm text-muted" style={{ lineHeight: 1.6, margin: '0 0 16px 0' }}>
          DBVault is a self-contained Go appliance for database backup automation, cryptographic Merkle verification, and point-in-time recovery on VPS servers and self-hosted infrastructure.
        </p>

        <div className="stack-sm mb-md" style={{
          background: 'var(--panel-inset, #f9fafb)',
          padding: '16px',
          borderRadius: '8px',
          border: '1px solid var(--line-subtle, #e5e7eb)',
          gap: '10px'
        }}>
          <div className="row-between text-xs">
            <span className="text-muted">Version:</span>
            <strong className="cell-mono">v0.1-alpha (Production Ready)</strong>
          </div>
          <div className="row-between text-xs">
            <span className="text-muted">Engines Supported:</span>
            <span>PostgreSQL · MySQL · MariaDB · SQLite</span>
          </div>
          <div className="row-between text-xs">
            <span className="text-muted">Envelope Encryption:</span>
            <span>AEAD AES-256-GCM (Master Key Derivation)</span>
          </div>
          <div className="row-between text-xs">
            <span className="text-muted">Integrity Signatures:</span>
            <span>Deterministic Merkle Tree (Ed25519)</span>
          </div>
          <div className="row-between text-xs">
            <span className="text-muted">Vault Storage:</span>
            <span>Local Filesystem Repository & Cloud (R2 / S3 / WORM)</span>
          </div>
        </div>

        <div className="row-actions">
          <Button label="Close" onClick={onClose} tone="primary" />
        </div>
      </div>
    </div>
  );
}

function ConfirmationModal({ config }) {
  if (!config) return null;
  const { title, message, confirmLabel, cancelLabel, confirmTone, resolve } = config;

  return (
    <div
      className="command-overlay"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      aria-describedby="confirm-dialog-desc"
      onClick={(e) => { if (e.target === e.currentTarget) resolve(false); }}
    >
      <div className="command-panel" style={{ maxWidth: '460px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2 id="confirm-dialog-title">{title}</h2>
          </div>
          <button type="button" className="icon-btn" onClick={() => resolve(false)} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <p id="confirm-dialog-desc" className="text-sm text-muted" style={{ lineHeight: '1.5', margin: '14px 0 24px 0' }}>
          {message}
        </p>

        <div className="row-actions" style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <Button label={cancelLabel || 'Cancel'} onClick={() => resolve(false)} tone="ghost" />
          <Button
            label={confirmLabel || 'Confirm'}
            onClick={() => resolve(true)}
            tone={confirmTone || 'danger'}
          />
        </div>
      </div>
    </div>
  );
}

