import { useState, useEffect, useRef } from 'preact/hooks';
import { Icon, Button, LoadingState, ErrorBox } from '../ui.jsx';
import { JobsAPI } from '../../api/jobs.js';
import { formatDate } from '../../format.js';
import { Store } from '../../state.js';

export function JobLogViewer({ jobId }) {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [query, setQuery] = useState('');
  const [selectedLevel, setSelectedLevel] = useState('all');
  const [autoScroll, setAutoScroll] = useState(true);
  const logTerminalRef = useRef(null);

  const fetchLogs = async (silent = false) => {
    try {
      if (!silent) setLoading(true);
      setError(null);
      const res = await JobsAPI.logs(jobId);
      setLogs(res.logs || []);
      if (!silent) setLoading(false);
    } catch (err) {
      if (!silent) {
        setError(err);
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    fetchLogs();
    const interval = setInterval(() => {
      fetchLogs(true);
    }, 2000);
    return () => clearInterval(interval);
  }, [jobId]);

  useEffect(() => {
    if (autoScroll && logTerminalRef.current) {
      logTerminalRef.current.scrollTop = logTerminalRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const handleDownloadLogs = () => {
    const rawText = logs
      .map((l) => `[${formatDate(l.created_at)}] [${(l.level || 'INFO').toUpperCase()}] [${l.stage || 'general'}] ${l.message}`)
      .join('\n');
    const blob = new Blob([rawText], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `dbvault-job-${jobId}.log`;
    a.click();
    URL.revokeObjectURL(url);
    Store.toast('Log file downloaded', 'success');
  };

  if (loading) return <LoadingState label="Loading live execution stream…" />;
  if (error) return <ErrorBox error={error} retry={() => fetchLogs()} />;

  const filtered = logs.filter((line) => {
    const matchesQuery = !query || `${line.level} ${line.message} ${line.stage}`.toLowerCase().includes(query.toLowerCase());
    const matchesLevel = selectedLevel === 'all' || line.level === selectedLevel;
    return matchesQuery && matchesLevel;
  });

  return (
    <div className="job-log-viewer-enhanced">
      {/* Log Console Toolbar */}
      <div className="log-console-toolbar">
        <div className="row-sm" style={{ flex: 1, maxWidth: '360px' }}>
          <div className="search-button" style={{ width: '100%' }}>
            <Icon name="search" size={14} />
            <input
              type="text"
              placeholder="Search stage, level, message…"
              aria-label="Search logs"
              value={query}
              onInput={(e) => setQuery(e.currentTarget.value)}
              style={{ width: '100%', border: 'none', background: 'transparent', outline: 'none', fontSize: '12px' }}
            />
            {query && (
              <button
                type="button"
                className="clear-search-btn"
                onClick={() => setQuery('')}
                aria-label="Clear search"
              >
                <Icon name="close" size={12} />
              </button>
            )}
          </div>
        </div>

        <div className="row-sm">
          <select
            className="form-select text-xs"
            value={selectedLevel}
            onChange={(e) => setSelectedLevel(e.currentTarget.value)}
            aria-label="Filter log level"
            style={{ height: '30px', padding: '0 8px' }}
          >
            <option value="all">All Levels</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
          </select>

          <button
            type="button"
            className={`btn compact ${autoScroll ? 'secondary' : 'ghost'}`}
            onClick={() => setAutoScroll(!autoScroll)}
            title="Auto-scroll log terminal to bottom"
          >
            <span className={`status-dot ${autoScroll ? 'success' : 'neutral'}`} />
            <span>Auto-scroll</span>
          </button>

          <Button
            label="Download Log"
            onClick={handleDownloadLogs}
            tone="secondary compact"
            icon="download"
          />

          <span className="log-count text-muted text-xs">
            {filtered.length} / {logs.length} lines
          </span>
        </div>
      </div>

      {/* Monospace Terminal Body */}
      <div className="log-terminal-window" ref={logTerminalRef}>
        {filtered.length > 0 ? (
          filtered.map((line, idx) => {
            const levelTone = line.level === 'error' ? 'log-error' : line.level === 'warning' ? 'log-warn' : 'log-info';
            return (
              <div key={idx} className={`log-line-row ${levelTone}`}>
                <span className="log-time">{formatDate(line.created_at)}</span>
                <span className={`log-level-badge ${line.level}`}>{(line.level || 'INFO').toUpperCase()}</span>
                {line.stage && <span className="log-stage-tag">{line.stage}</span>}
                <span className="log-message-text">{line.message}</span>
              </div>
            );
          })
        ) : (
          <div className="log-empty-message">
            <span>{logs.length === 0 ? 'No log entries recorded yet for this operation.' : 'No log lines match the current search filters.'}</span>
          </div>
        )}
      </div>
    </div>
  );
}
