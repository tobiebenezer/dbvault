import { useState, useEffect } from 'preact/hooks';
import { Icon, LoadingState, ErrorBox } from '../ui.jsx';
import { JobsAPI } from '../../api/jobs.js';
import { formatDate } from '../../format.js';

export function JobLogViewer({ jobId }) {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [query, setQuery] = useState('');
  const [selectedLevel, setSelectedLevel] = useState('all');

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    JobsAPI.logs(jobId)
      .then((res) => {
        if (!active) return;
        setLogs(res.logs || []);
        setLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        setError(err);
        setLoading(false);
      });
    return () => { active = false; };
  }, [jobId]);

  if (loading) return <LoadingState label="Loading sanitised logs…" />;
  if (error) return <ErrorBox error={error} retry={() => JobsAPI.logs(jobId).then((r) => setLogs(r.logs || []))} />;

  const filtered = logs.filter((line) => {
    const matchesQuery = !query || `${line.level} ${line.message} ${line.stage}`.toLowerCase().includes(query.toLowerCase());
    const matchesLevel = selectedLevel === 'all' || line.level === selectedLevel;
    return matchesQuery && matchesLevel;
  });

  return (
    <div className="job-log-viewer">
      <div className="log-toolbar row-between mb-sm">
        <div className="row-sm" style={{ flex: 1, maxWidth: '400px' }}>
          <div className="search-button" style={{ width: '100%' }}>
            <Icon name="search" size={14} />
            <input
              type="text"
              placeholder="Search stage, level, or message…"
              aria-label="Search logs"
              value={query}
              onInput={(e) => setQuery(e.currentTarget.value)}
              style={{ width: '100%', border: 'none', background: 'transparent', outline: 'none', fontSize: '12px' }}
            />
          </div>
        </div>
        <div className="row-sm">
          <select
            className="form-select"
            value={selectedLevel}
            onChange={(e) => setSelectedLevel(e.currentTarget.value)}
            aria-label="Filter log level"
            style={{ height: '32px' }}
          >
            <option value="all">All levels</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
          </select>
          <span className="log-count text-muted text-sm">{filtered.length} of {logs.length} lines</span>
        </div>
      </div>

      <div className="log-viewer">
        {filtered.length > 0 ? (
          filtered.map((line, idx) => (
            <div key={idx} className={`log-line ${line.level}`} style={{ display: 'flex', gap: '12px', padding: '2px 0' }}>
              <time style={{ opacity: 0.6, whiteSpace: 'nowrap' }}>{formatDate(line.created_at)}</time>
              <code style={{ color: line.level === 'error' ? '#f87171' : line.level === 'warning' ? '#fbbf24' : '#38bdf8', minWidth: '45px' }}>[{line.level}]</code>
              <span>{line.message}</span>
            </div>
          ))
        ) : (
          <div style={{ padding: '16px', textAlign: 'center', opacity: 0.6 }}>No log entries match your filter</div>
        )}
      </div>
    </div>
  );
}
