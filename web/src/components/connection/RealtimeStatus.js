import { h } from 'preact';
import { Store } from '../../state.js';
import { ConnectionState } from '../../state/connection.js';

export function RealtimeStatus() {
  const status = Store.state.connection.status;
  const label = ConnectionState.label();
  return h('div', { class: `realtime-status ${status}`, role: 'status', 'aria-live': 'polite' }, [
    h('span', { class: 'realtime-dot', 'aria-hidden': 'true' }),
    h('span', { text: label })
  ]);
}
