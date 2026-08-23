self.onmessage = (event) => {
  const { type, query, lines } = event.data || {};
  if (type !== 'SEARCH_LOGS') return;
  const q = String(query || '').toLowerCase();
  const matches = (lines || []).filter((line) => `${line.level || ''} ${line.stage || ''} ${line.message || ''}`.toLowerCase().includes(q));
  self.postMessage({ type: 'SEARCH_RESULTS', matches });
};
