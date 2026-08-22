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

  useEffect(() => {
    return Store.subscribe((s) => setRoute(s.route));
  }, []);

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

// Initialise background stores and real-time SSE stream
JobsStore.load();
AlertsStore.refresh();
connectJobEvents();

const appEl = document.getElementById('app');
if (appEl) {
  render(<App />, appEl);
}
