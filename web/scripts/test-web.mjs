import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const required = [
  'src/main.js',
  'src/api.js',
  'src/pages/overview.js',
  'src/pages/setup.js',
  'src/pages/recovery.js',
  'src/components/timeline.js',
  'src/styles/app.css',
  'src/realtime/job-events.js',
  'src/actions.js',
  'src/state/jobs.js',
  'src/components/jobs/JobCard.js',
  'src/workers/log-search.worker.js',
  'src/workers/timeline.worker.js'
];
for (const file of required) {
  const text = await readFile(resolve(process.cwd(), file), 'utf8');
  if (text.trim().length < 100) throw new Error(`${file} looks incomplete`);
}
const main = await readFile(resolve(process.cwd(), 'src/main.js'), 'utf8');
const setup = await readFile(resolve(process.cwd(), 'src/pages/setup.js'), 'utf8');
const layout = await readFile(resolve(process.cwd(), 'src/components/layout.js'), 'utf8');
for (const token of ['OverviewPage', 'SetupPage', 'RecoveryPage']) {
  if (!main.includes(token)) throw new Error(`main.js missing ${token}`);
}
if (!layout.includes('CommandPalette')) throw new Error('layout.js missing CommandPalette');
for (const field of ['database_password', 'access_key_id', 'secret_access_key', 'tls_mode', 'connect_timeout', 'config_path']) {
  if (!setup.includes(field)) throw new Error(`setup.js missing connection field ${field}`);
}
const jobsState = await readFile(resolve(process.cwd(), 'src/state/jobs.js'), 'utf8');
if (!jobsState.includes('applyEvent')) throw new Error('jobs store missing applyEvent');
const live = await readFile(resolve(process.cwd(), 'src/realtime/job-events.js'), 'utf8');
if (!live.includes('EventSource')) throw new Error('job events client missing EventSource');
const css = await readFile(resolve(process.cwd(), 'src/styles/app.css'), 'utf8');
for (const cls of ['.card', '.job-card', '.toast', '.empty-state']) {
  const idx = css.indexOf(cls);
  const block = css.slice(idx, idx + 300);
  if (idx === -1 || !block.includes('border-radius')) throw new Error(`${cls} must define rounded-md-or-greater border radius`);
}
console.log('DBVault web source checks passed');

const actions = await readFile(resolve(process.cwd(), 'src/actions.js'), 'utf8');
for (const token of ['backup:', 'restoreDrill:', 'destinationTest:', 'discover()', 'doctor()']) {
  if (!actions.includes(token)) throw new Error(`actions.js missing ${token}`);
}
const forbiddenAccentTokens = ['#2563eb', '#1d4ed8', '#0ea5e9', '#38bdf8', '#7c3aed'];
for (const token of forbiddenAccentTokens) {
  if (css.toLowerCase().includes(token)) throw new Error(`legacy blue/purple accent remains: ${token}`);
}
if (!css.includes('--primary: #0f766e')) throw new Error('unified emerald primary accent is missing');


if (!css.includes('--bg: #ffffff')) throw new Error('white application background is missing');
if (!css.includes('--nav: #ffffff')) throw new Error('white navigation surface is missing');
if (!css.includes('Phase 8I final UI cleanup')) throw new Error('final UI cleanup layer is missing');
if (css.includes('background: #202823')) throw new Error('legacy dark workspace menu remains');
