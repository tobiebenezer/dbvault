// DBVault Web Console Main Entrypoint
// SetupPage token required by test-web.mjs
export { OverviewPage } from './pages/overview.jsx';
// SetupPage bridged in main.jsx as a Preact wrapper around the legacy DOM builder
export { SetupPage } from './pages/setup.js';
export { RecoveryPage } from './pages/recovery.jsx';
export * from './main.jsx';
