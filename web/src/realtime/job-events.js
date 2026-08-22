import { EventsAPI } from '../api/events.js';
import { JobsStore } from '../state/jobs.js';
import { AlertsStore } from '../state/alerts.js';
import { Store } from '../state.js';
import { nextReconnectDelay } from './reconnect.js';

let activeSource = null;
let isClosed = false;
let reconnectTimer = null;
let reconnectAttempt = 0;

export function connectJobEvents() {
  isClosed = false;

  function open() {
    if (isClosed) return;
    if (activeSource) {
      try { activeSource.close(); } catch (_) {}
    }

    const last = Store.state.connection.lastEventId;
    const url = last ? `${EventsAPI.jobsPath()}?last_event_id=${encodeURIComponent(last)}` : EventsAPI.jobsPath();
    
    try {
      activeSource = new EventSource(url, { withCredentials: true });
    } catch (e) {
      JobsStore.setConnectionStatus('error', e.message);
      scheduleReconnect();
      return;
    }

    activeSource.onopen = async () => {
      reconnectAttempt = 0;
      JobsStore.setConnectionStatus('connected');
      await JobsStore.load();
      await AlertsStore.refresh();
    };

    activeSource.addEventListener('hello', () => {
      JobsStore.setConnectionStatus('connected');
    });

    const eventTypes = [
      'job.created', 'job.queued', 'job.started', 'job.progress',
      'job.stage_changed', 'job.retrying', 'job.cancelling', 'job.cancelled',
      'job.failed', 'job.completed', 'job.log', 'job.warning'
    ];

    for (const type of eventTypes) {
      activeSource.addEventListener(type, (message) => receive(message));
    }

    activeSource.onerror = () => {
      if (isClosed) return;
      JobsStore.setConnectionStatus('reconnecting');
      try { activeSource.close(); } catch (_) {}
      scheduleReconnect();
    };
  }

  function scheduleReconnect() {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    const delay = nextReconnectDelay(reconnectAttempt++);
    reconnectTimer = setTimeout(async () => {
      await JobsStore.load().catch(() => {});
      open();
    }, Math.min(delay, 5000));
  }

  function receive(message) {
    try {
      JobsStore.setConnectionStatus('connected');
      const data = JSON.parse(message.data);
      if (message.lastEventId || data.event_id) {
        JobsStore.setLastEventId(message.lastEventId || data.event_id);
      }
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
    isClosed = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (activeSource) {
      try { activeSource.close(); } catch (_) {}
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

export function forceReconnect() {
  if (activeSource) {
    try { activeSource.close(); } catch (_) {}
  }
  reconnectAttempt = 0;
  JobsStore.setConnectionStatus('reconnecting');
  connectJobEvents();
}
