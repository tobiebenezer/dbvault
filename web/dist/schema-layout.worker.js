self.onmessage = (event) => {
  const { type, tables = [] } = event.data || {};
  if (type !== 'LAYOUT_SCHEMA') return;
  const nodes = tables.map((table, index) => ({ ...table, x: (index % 4) * 260, y: Math.floor(index / 4) * 180 }));
  self.postMessage({ type: 'SCHEMA_LAYOUT_READY', nodes, edges: [] });
};
