import { EventsAPI } from '../api/events.js';
import { JobsStore } from '../state/jobs.js';
import { AlertsStore } from '../state/alerts.js';
import { Store } from '../state.js';
import { nextReconnectDelay } from './reconnect.js';

export function connectJobEvents() {
  let source = null;
  let closed = false;
  let reconnectAttempt = 0;
  let reconnectTimer = null;

  function open() {
    if (closed) return;
    const last = Store.state.connection.lastEventId;
    const url = last ? `${EventsAPI.jobsPath()}?last_event_id=${encodeURIComponent(last)}` : EventsAPI.jobsPath();
    source = new EventSource(url, { withCredentials: true });
    source.onopen = async () => {
      reconnectAttempt = 0;
      JobsStore.setConnectionStatus('connected');
      await JobsStore.load();
      await AlertsStore.refresh();
    };
    source.addEventListener('hello', () => JobsStore.setConnectionStatus('connected'));
    for (const type of ['job.created', 'job.queued', 'job.started', 'job.progress', 'job.stage_changed', 'job.retrying', 'job.cancelling', 'job.cancelled', 'job.failed', 'job.completed', 'job.log', 'job.warning']) {
      source.addEventListener(type, (message) => receive(message));
    }
    source.onerror = () => {
      if (closed) return;
      JobsStore.setConnectionStatus('reconnecting');
      try { source.close(); } catch (_) {}
      const delay = nextReconnectDelay(reconnectAttempt++);
      reconnectTimer = setTimeout(async () => {
        await JobsStore.load();
        open();
      }, delay);
    };
  }

  function receive(message) {
    try {
      const data = JSON.parse(message.data);
      if (message.lastEventId || data.event_id) JobsStore.setLastEventId(message.lastEventId || data.event_id);
      JobsStore.applyEvent(data);
      if (['job.completed', 'job.failed', 'job.cancelled'].includes(data.event_type)) {
        Store.refresh();
        AlertsStore.refresh();
      }
    } catch (error) {
      JobsStore.setConnectionStatus('error', error.message);
    }
  }

  const handleUnload = () => {
    closed = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (source) {
      try { source.close(); } catch (_) {}
    }
  };

  window.addEventListener('beforeunload', handleUnload, { once: true });
  window.addEventListener('pagehide', handleUnload, { once: true });

  open();
  return () => {
    handleUnload();
    window.removeEventListener('beforeunload', handleUnload);
    window.removeEventListener('pagehide', handleUnload);
    JobsStore.setConnectionStatus('disconnected');
  };
}
