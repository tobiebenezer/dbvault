import { useState, useEffect } from 'preact/hooks';
import { Icon, IconButton, Button } from './ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { API } from '../api.js';

const navGroups = [
  { label: 'Protect', items: [['/', 'Overview'], ['/databases', 'Databases'], ['/repositories', 'Storage'], ['/recovery', 'Recovery']] },
  { label: 'Operate', items: [['/jobs', 'Jobs'], ['/alerts', 'Alerts']] },
  { label: 'System', items: [['/setup', 'Setup Wizard'], ['/settings', 'Settings']] }
];

export function Layout({ children }) {
  const [state, setState] = useState(Store.state);

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
      <Sidebar state={state} />
      <main className="main" id="main">
        <Topbar state={state} />
        <div className="page-wrap">{children}</div>
      </main>
      <Toast state={state} />
      <CommandPalette open={Boolean(state.commandOpen)} />
    </div>
  );
}

function Sidebar({ state }) {
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
          <span className="brand-title">DBVault</span>
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
            <small>Active Workspace</small>
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
              <span>Manage workspaces</span>
            </button>
          </div>
        )}
      </div>

      <nav className="nav-groups">
        {navGroups.map((group) => (
          <section key={group.label} className="nav-group">
            <span className="nav-label">{group.label}</span>
            {group.items.map(([path, label]) => {
              const active = isLinkActive(path);
              return (
                <a
                  key={path}
                  className={`nav-link ${active ? 'active' : ''}`}
                  href={path}
                  onClick={navClick(path)}
                >
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
        <span>DBVault Appliance · v0.1-alpha</span>
      </div>
    </aside>
  );
}

function Topbar({ state }) {
  const connectionStatus = state.connection?.status || 'disconnected';

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
          <span>Search commands & databases…</span>
          <kbd>⌘K</kbd>
        </button>
      </div>

      <div className="topbar-actions">
        <div className={`realtime-status ${connectionStatus}`}>
          <span className="realtime-dot" />
          <span>{connectionStatus === 'connected' ? 'Live Stream' : connectionStatus === 'reconnecting' ? 'Reconnecting' : 'Offline'}</span>
        </div>
        <Button
          label="Back up now"
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
    { label: 'Back up database', description: 'Start a production backup', run: () => ProductActions.backup() },
    { label: 'Run restore drill', description: 'Verify recovery safely in sandbox', run: () => ProductActions.restoreDrill() },
    { label: 'Restore to sandbox', description: 'Open the recovery planner', run: () => Store.navigate('/recovery') },
    { label: 'Add database', description: 'Connect a new database source', run: () => ProductActions.openSetupStep('discover-or-add-database') },
    { label: 'Add storage destination', description: 'Connect Cloudflare R2, S3, or local storage', run: () => ProductActions.openSetupStep('add-storage-destination') },
    { label: 'Run system doctor', description: 'Check readiness and host diagnostics', run: () => ProductActions.doctor() },
    { label: 'Create recovery bundle', description: 'Export disaster recovery bundle', run: async () => {
      try {
        const res = await API.recoveryBundle();
        Store.toast(`Recovery bundle created: ${res.path}`, 'success');
      } catch (err) {
        Store.toast(err.message, 'danger');
      }
    }}
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
            placeholder="Type a command…"
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
            <div className="command-empty-box">No matches found</div>
          )}
        </div>
      </div>
    </div>
  );
}
