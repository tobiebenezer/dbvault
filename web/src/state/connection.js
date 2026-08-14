export const ConnectionState = {
  live: () => Store.state.connection.status === 'connected',
  label: () => ({ connected: 'Live', reconnecting: 'Reconnecting', disconnected: 'Offline', error: 'Offline' }[Store.state.connection.status] || 'Offline')
};
