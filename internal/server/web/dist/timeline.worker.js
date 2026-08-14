self.onmessage = (event) => {
  const { type, events = [], windows = [], viewportStart, viewportEnd } = event.data || {};
  if (type !== 'BUILD_TIMELINE') return;
  const start = viewportStart ? new Date(viewportStart).getTime() : Math.min(...events.map((e) => new Date(e.occurred_at).getTime()), Date.now() - 86400000);
  const end = viewportEnd ? new Date(viewportEnd).getTime() : Math.max(...events.map((e) => new Date(e.occurred_at).getTime()), Date.now());
  const span = Math.max(1, end - start);
  const points = events
    .slice()
    .sort((a, b) => new Date(a.occurred_at) - new Date(b.occurred_at))
    .map((item) => ({ ...item, x: Math.max(2, Math.min(98, ((new Date(item.occurred_at).getTime() - start) / span) * 100)) }));
  const gaps = [];
  const sortedWindows = windows.slice().sort((a, b) => new Date(a.start) - new Date(b.start));
  for (let i = 1; i < sortedWindows.length; i++) {
    const prevEnd = new Date(sortedWindows[i - 1].end).getTime();
    const nextStart = new Date(sortedWindows[i].start).getTime();
    if (nextStart > prevEnd) gaps.push({ start: sortedWindows[i - 1].end, end: sortedWindows[i].start, reason: 'Window gap' });
  }
  self.postMessage({ type: 'TIMELINE_READY', points, segments: sortedWindows, gaps, labels: [] });
};
