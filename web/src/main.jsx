import { render, Component } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import { Layout } from './components/layout.jsx';
import { Store } from './state.js';
import { JobsStore } from './state/jobs.js';
import { AlertsStore } from './state/alerts.js';
import { connectJobEvents } from './realtime/job-events.js';
import { OverviewPage } from './pages/overview.jsx';
import { SetupPage } from './pages/setup.jsx';
import { DatabasesPage, DatabaseDetailPage } from './pages/databases.jsx';
import { RepositoriesPage } from './pages/repositories.jsx';
import { RecoveryPage } from './pages/recovery.jsx';
import { WarehousePage } from './pages/warehouse.jsx';
import { JobsPage } from './pages/jobs.jsx';
import { AlertsPage } from './pages/alerts.jsx';
import { SettingsPage } from './pages/settings.jsx';
import { BillingPage } from './pages/billing.jsx';
import { AuditPage } from './pages/audit.jsx';
import { TeamPage } from './pages/team.jsx';
import { AdminFleetPage } from './pages/admin.jsx';
import { TrustPage } from './pages/trust.jsx';
import { LoginPage } from './pages/login.jsx';
import { FirstRunSetupPage } from './pages/setup-admin.jsx';


export class ErrorBoundary extends Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }
  componentDidCatch(error) {
    console.error('Captured UI Error:', error);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div className="page">
          <div className="card" style={{ padding: '24px', borderLeft: '4px solid var(--color-danger, #f85149)' }}>
            <h2>View temporarily unavailable</h2>
            <p className="text-muted text-sm">{this.state.error?.message || String(this.state.error)}</p>
            <button className="btn btn-primary mt-md" onClick={() => { this.setState({ hasError: false }); window.location.reload(); }}>
              Reload Page
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export function App() {
  const [route, setRoute] = useState(Store.state.route);
  // null = loading, true = authenticated, false = unauthenticated
  const [authed, setAuthed] = useState(null);
  // true when the appliance has no administrator yet → show first-run setup
  const [needsBootstrap, setNeedsBootstrap] = useState(false);

  // Check session on mount by probing /api/v1/auth/session
  useEffect(() => {
    fetch('/api/v1/auth/session', { credentials: 'same-origin' })
      .then(res => {
        setAuthed(res.ok);
      })
      .catch(() => setAuthed(false));
  }, []);

  // On unauthenticated, probe whether the appliance still needs its first
  // administrator so we can show the first-run setup screen instead of login.
  useEffect(() => {
    if (authed !== false) return;
    fetch('/api/v1/auth/bootstrap-status', { credentials: 'same-origin' })
      .then(res => (res.ok ? res.json() : { bootstrapped: true }))
      .then(body => setNeedsBootstrap(body.bootstrapped === false))
      .catch(() => setNeedsBootstrap(false));
  }, [authed]);

  // Listen for global 401 events fired by api.js
  useEffect(() => {
    function onUnauthed() { setAuthed(false); }
    window.addEventListener('dbvault:unauthenticated', onUnauthed);
    return () => window.removeEventListener('dbvault:unauthenticated', onUnauthed);
  }, []);

  useEffect(() => {
    return Store.subscribe((s) => setRoute(s.route));
  }, []);

  // Connect background jobs and realtime stream once authed
  useEffect(() => {
    if (authed) {
      JobsStore.load();
      AlertsStore.refresh();
      connectJobEvents();
    }
  }, [authed]);

  // 'auto' | 'login' | 'setup'
  const [authMode, setAuthMode] = useState('auto');

  // Show a blank screen while checking session on first load
  if (authed === null) {
    return <div style={{ minHeight: '100vh', background: 'var(--bg-base, #fff)' }} />;
  }

  // Not authenticated → show first-run setup (no administrator yet) or login
  if (!authed) {
    const showSetup = authMode === 'setup' || (authMode === 'auto' && needsBootstrap && route !== '/login');
    const handleLoginSuccess = () => {
      fetch('/api/v1/auth/session', { credentials: 'same-origin' })
        .then(res => {
          if (res.ok) {
            setAuthed(true);
            if (route === '/login' || route === '/setup-admin') {
              Store.navigate('/');
            }
          } else {
            setAuthed(false);
          }
        })
        .catch(() => setAuthed(false));
    };

    if (showSetup) {
      return (
        <FirstRunSetupPage
          onLogin={handleLoginSuccess}
          onSwitchToLogin={() => setAuthMode('login')}
        />
      );
    }
    return (
      <LoginPage
        onLogin={handleLoginSuccess}
        onSwitchToSetup={() => setAuthMode('setup')}
      />
    );
  }


  const dbDetailMatch = route.match(/^\/databases\/(.+)$/);

  let PageComponent;
  if (dbDetailMatch) {
    const dbId = decodeURIComponent(dbDetailMatch[1]);
    PageComponent = <DatabaseDetailPage dbId={dbId} />;
  } else if (route === '/') {
    PageComponent = <OverviewPage />;
  } else if (route === '/setup' || route.startsWith('/setup')) {
    PageComponent = <SetupPage />;
  } else if (route.startsWith('/databases')) {
    PageComponent = <DatabasesPage />;
  } else if (route.startsWith('/repositories')) {
    PageComponent = <RepositoriesPage />;
  } else if (route.startsWith('/recovery')) {
    PageComponent = <RecoveryPage />;
  } else if (route.startsWith('/warehouse')) {
    PageComponent = <WarehousePage />;
  } else if (route.startsWith('/jobs')) {
    PageComponent = <JobsPage />;
  } else if (route.startsWith('/alerts')) {
    PageComponent = <AlertsPage />;
  } else if (route.startsWith('/billing')) {
    PageComponent = <BillingPage />;
  } else if (route.startsWith('/audit')) {
    PageComponent = <AuditPage />;
  } else if (route.startsWith('/trust')) {
    PageComponent = <TrustPage />;
  } else if (route.startsWith('/team')) {
    PageComponent = <TeamPage />;
  } else if (route.startsWith('/admin')) {
    PageComponent = <AdminFleetPage />;
  } else if (route.startsWith('/settings')) {
    PageComponent = <SettingsPage />;
  } else {
    PageComponent = (
      <div className="page">
        <h1>Page not found</h1>
        <p>The route exists neither in the console nor in the recovery map.</p>
      </div>
    );
  }

  return (
    <ErrorBoundary>
      <Layout>{PageComponent}</Layout>
    </ErrorBoundary>
  );
}


// Initialise background stores and real-time SSE stream only when authenticated
const appEl = document.getElementById('app');
if (appEl) {
  render(<App />, appEl);
}

