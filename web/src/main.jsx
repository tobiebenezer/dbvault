import { render } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { Layout } from './components/layout.jsx';
import { Store } from './state.js';
import { JobsStore } from './state/jobs.js';
import { AlertsStore } from './state/alerts.js';
import { connectJobEvents } from './realtime/job-events.js';
import { OverviewPage } from './pages/overview.jsx';
import { SetupPage as _SetupPageDom } from './pages/setup.js';
import { DatabasesPage, DatabaseDetailPage } from './pages/databases.jsx';
import { RepositoriesPage } from './pages/repositories.jsx';
import { RecoveryPage } from './pages/recovery.jsx';
import { JobsPage } from './pages/jobs.jsx';
import { AlertsPage } from './pages/alerts.jsx';
import { SettingsPage } from './pages/settings.jsx';

// Bridge wrapper: mounts the legacy DOM-builder setup wizard into a Preact-owned div
function SetupPage() {
  const containerRef = useRef(null);
  useEffect(() => {
    if (!containerRef.current) return;
    // Call the old imperative SetupPage which returns a real DOM element
    const el = _SetupPageDom();
    containerRef.current.replaceChildren(el);
    // Re-run when Store refreshes (setup wizard steps change)
    return Store.subscribe(() => {
      // Only update if this container is still mounted
      if (containerRef.current) {
        const fresh = _SetupPageDom();
        containerRef.current.replaceChildren(fresh);
      }
    });
  }, []);
  return <div ref={containerRef} className="setup-wrapper" />;
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
  } else if (route.startsWith('/jobs')) {
    PageComponent = <JobsPage />;
  } else if (route.startsWith('/alerts')) {
    PageComponent = <AlertsPage />;
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

  return <Layout>{PageComponent}</Layout>;
}

// Initialise background stores and real-time SSE stream
JobsStore.load();
AlertsStore.refresh();
connectJobEvents();

const appEl = document.getElementById('app');
if (appEl) {
  render(<App />, appEl);
}
