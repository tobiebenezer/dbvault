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
      <Sidebar state={state} onOpenShortcuts={() => setShowShortcuts(true)} />
      <main className="main" id="main">
        <Topbar state={state} onOpenShortcuts={() => setShowShortcuts(true)} />
        <div className="page-wrap">{children}</div>
      </main>
      <Toast state={state} />
      <CommandPalette open={Boolean(state.commandOpen)} />
      {showShortcuts && <ShortcutsModal onClose={() => setShowShortcuts(false)} />}
      {state.confirmModal && <ConfirmationModal config={state.confirmModal} />}
    </div>
  );
}

function Sidebar({ state, onOpenShortcuts }) {
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
          <span className="brand-mark">DB</span>
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

      <div className="workspace-switcher">
        <button
          className="project-card"
          type="button"
          aria-expanded={workspaceOpen ? 'true' : 'false'}
          onClick={() => Store.set({ workspaceOpen: !workspaceOpen })}
        >
          <div className="project-card-info">
            <strong>Default Project</strong>
            <small>Active Vault Appliance</small>
          </div>
          <Icon name="chevron" size={12} />
        </button>

        {workspaceOpen && (
          <div className="workspace-menu">
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
        <button type="button" className="footer-shortcut-btn" onClick={onOpenShortcuts}>
          <span>Shortcuts</span>
          <kbd>?</kbd>
        </button>
        <span className="version-tag">v0.1-alpha · Production Ready</span>
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

export function CommandPalette({ open }) {
  const [query, setQuery] = useState('');

  const commands = [
    { label: 'Back up primary database', description: 'Start full production snapshot & WAL sync', run: () => ProductActions.backup() },
    { label: 'Run restore drill', description: 'Verify recovery in isolated sandbox container', run: () => ProductActions.restoreDrill() },
    { label: 'Open Recovery Studio & PITR', description: 'Point-in-time recovery & time-travel planner', run: () => Store.navigate('/recovery') },
    { label: 'Open Data Warehouse & Analytics', description: 'Run analytical OLAP queries over Parquet lakehouse', run: () => Store.navigate('/warehouse') },
    { label: 'Explore Databases & Tables', description: 'Live schema inspector, table sizes & exclusions', run: () => Store.navigate('/databases') },
    { label: 'Connect & Probe Database', description: 'Discover databases on PostgreSQL or MySQL servers', run: () => Store.navigate('/databases') },
    { label: 'Manage Storage Targets & WORM', description: 'Connect Cloudflare R2, AWS S3, or MinIO', run: () => Store.navigate('/repositories') },
    { label: 'Security & Compliance Center', description: 'ISO 27001, ISO 27040 & SOC2 audit certificates', run: () => Store.navigate('/trust') },
    { label: 'Run preflight system doctor', description: 'Check readiness, storage, and host diagnostics', run: () => ProductActions.doctor() },
    { label: 'Create emergency recovery bundle', description: 'Export zero-knowledge disaster recovery bundle', run: async () => {
      try {
        const res = await API.recoveryBundle();
        Store.toast(`Recovery bundle created: ${res.path}`, 'success');
      } catch (err) {
        Store.toast(err.message, 'danger');
      }
    }},
    { label: 'View all operations & jobs', description: 'Monitor running tasks and logs', run: () => Store.navigate('/jobs') },
    { label: 'Check alerts & incident notifications', description: 'Inspect active warnings and health checks', run: () => Store.navigate('/alerts') },
    { label: 'Configure Notification Webhooks', description: 'Slack, Discord, and Teams alerting', run: () => Store.navigate('/settings') }
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

