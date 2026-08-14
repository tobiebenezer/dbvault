import { API } from './api.js';
import { JobsAPI } from './api/jobs.js';
import { JobsStore } from './state/jobs.js';
import { AlertsStore } from './state/alerts.js';
import { Store } from './state.js';

export const ProductActions = {
  backup: (resource = defaultSource()) => queueJob('backup', resource, 'Backup queued'),
  restoreDrill: (resource = defaultSource()) => queueJob('restore_drill', resource, 'Restore drill queued'),
  destinationTest: (destination) => queueJob('destination_test', { id: destination?.id || 'all-destinations', name: destination?.name || 'Storage destinations' }, 'Storage test queued'),
  retryReplication: (destination) => queueJob('replication', { id: destination?.id || 'contabo-replica', name: destination?.name || 'Contabo replica' }, 'Replication retry queued'),
  async discover() {
    try {
      const result = await API.discover('local', []);
      const count = (result.discoveries || []).length;
      Store.toast(`${count} database candidate${count === 1 ? '' : 's'} found`, 'success');
      Store.refresh();
      return result;
    } catch (error) { Store.toast(error.message, 'danger'); throw error; }
  },
  async doctor() {
    try {
      const result = await API.doctor();
      Store.set({ doctorResult: result });
      const warnings = (result.checks || []).filter((check) => check.status !== 'pass').length;
      Store.toast(warnings ? `Doctor found ${warnings} item${warnings === 1 ? '' : 's'} to review` : 'All doctor checks passed', warnings ? 'warning' : 'success');
      return result;
    } catch (error) { Store.toast(error.message, 'danger'); throw error; }
  },
  async openSetupStep(step) {
    try { await API.setSetupStep(step); Store.navigate('/setup'); Store.refresh(); }
    catch (error) { Store.toast(error.message, 'danger'); }
  },
  async executeAlertAction(alert, action) {
    if (!action) return;
    if (action.id === 'retry-replication') return ProductActions.retryReplication({ id: alert.resource_id, name: 'Contabo replica' });
    if (action.id === 'test-destination' || action.id === 'test-contabo') return ProductActions.destinationTest({ id: alert.resource_id, name: 'Contabo Object Storage' });
    if (action.id === 'run-doctor') return ProductActions.doctor();
    Store.set({ selectedAlert: alert.id });
  },
  async acknowledgeAlert(id) {
    try { await API.acknowledgeAlert(id); await AlertsStore.refresh(); Store.toast('Alert acknowledged', 'success'); Store.refresh(); }
    catch (error) { Store.toast(error.message, 'danger'); }
  },
  async resolveAlert(id) {
    try { await API.resolveAlert(id); await AlertsStore.refresh(); Store.toast('Alert resolved', 'success'); Store.refresh(); }
    catch (error) { Store.toast(error.message, 'danger'); }
  }
};

async function queueJob(jobType, resource, message) {
  try {
    const job = await JobsAPI.create({ job_type: jobType, resource_id: resource.id, resource_name: resource.name });
    JobsStore.hydrate([job]);
    Store.toast(message, 'success');
    Store.navigate(`/jobs/${job.id}`);
    return job;
  } catch (error) { Store.toast(error.message, 'danger'); throw error; }
}
function defaultSource() { return { id: 'production-postgres', name: 'Production PostgreSQL' }; }
