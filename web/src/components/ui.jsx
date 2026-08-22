// ─────────────────────────────────────────────────────────────────────────────
// ui.jsx — Preact JSX components
// Import from ui.jsx directly when using JSX/hooks pages.
// Legacy pages (setup.js) import from ui.js which has real DOM builders.
// ─────────────────────────────────────────────────────────────────────────────

export const ICONS = {
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  chevron: '<path d="m9 18 6-6-6-6"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M20 6v5h-5M4 18v-5h5"/><path d="M18.5 9A7 7 0 0 0 6 6.5L4 11M5.5 15A7 7 0 0 0 18 17.5l2-4.5"/>',
  download: '<path d="M12 3v12M7 10l5 5 5-5M4 21h16"/>',
  shield: '<path d="M12 3 4.5 6v5.3c0 4.6 3.1 8.8 7.5 9.7 4.4-.9 7.5-5.1 7.5-9.7V6z"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  warning: '<path d="M10.3 4.3 2.7 18a2 2 0 0 0 1.8 3h15a2 2 0 0 0 1.8-3L13.7 4.3a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
  play: '<polygon points="5 3 19 12 5 21 5 3"/>',
  database: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5"/><path d="M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
  terminal: '<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>',
  file: '<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/>',
  clock: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
  server: '<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>',
  key: '<path d="m21 2-2 2m-1.5 1.5L14 9l-4-4-6 6a3 3 0 1 0 4.24 4.24l6-6 3.5 3.5 1.5-1.5M19 5l2 2"/>'
};

export function Icon({ name = 'info', size = 16, label = '', className = '' }) {
  const innerSvg = ICONS[name] || ICONS.info;
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`icon ${className}`}
      role={label ? 'img' : undefined}
      aria-label={label || undefined}
      aria-hidden={label ? undefined : 'true'}
      dangerouslySetInnerHTML={{ __html: innerSvg }}
    />
  );
}

export function Card({ title, subtitle, action, noPadding = false, className = '', children }) {
  return (
    <section className={`card ${className}`} aria-label={typeof title === 'string' ? title : undefined}>
      <div className="card-header">
        <div className="card-heading">
          {typeof title === 'string' ? <h2>{title}</h2> : title}
          {subtitle && <p className="card-subtitle">{subtitle}</p>}
        </div>
        {action && <div>{action}</div>}
      </div>
      <div className={`card-body ${noPadding ? 'no-padding' : ''}`}>
        {children}
      </div>
    </section>
  );
}

export function MetricCard({ label, value, footerText, statusTone = 'neutral', className = '' }) {
  return (
    <article className={`metric-card ${className}`}>
      <div className="metric-card-label">{label}</div>
      <div className="metric-card-value">{value}</div>
      {footerText && (
        <div className="metric-card-footer">
          {statusTone !== 'neutral' && <span className={`status-dot ${statusTone}`} />}
          <span>{footerText}</span>
        </div>
      )}
    </article>
  );
}

export function PageHeader({ title, description, actions = [] }) {
  return (
    <header className="page-header">
      <div className="page-header-main">
        <h1>{title}</h1>
        {description && <p className="page-description">{description}</p>}
      </div>
      {actions.length > 0 && <div className="page-header-actions">{actions}</div>}
    </header>
  );
}

export function SegmentedNav({ tabs, activeId, onSelect }) {
  return (
    <div className="segmented-nav" role="tablist">
      {tabs.map((tab) => {
        const active = tab.id === activeId;
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={active ? 'true' : 'false'}
            className={`segmented-tab ${active ? 'active' : ''}`}
            onClick={() => onSelect(tab.id)}
          >
            <span>{tab.label}</span>
            {tab.count !== undefined && <span className="segmented-tab-count">{tab.count}</span>}
          </button>
        );
      })}
    </div>
  );
}

export function DataTable({ headers = [], rows = [], emptyText = 'No records found', children }) {
  return (
    <div className="data-table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            {headers.map((hdr, i) => {
              let thClass = '';
              if (hdr.width === '120px') thClass = 'cell-actions-w120';
              else if (hdr.width === '160px') thClass = 'cell-actions-w160';
              else if (hdr.width === '200px') thClass = 'cell-actions-w200';
              return <th key={i} className={thClass}>{hdr.label || hdr}</th>;
            })}
          </tr>
        </thead>
        <tbody>
          {children || (rows.length > 0 ? rows : (
            <tr>
              <td colSpan={headers.length} className="empty-cell">
                <span>{emptyText}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function StatusIndicator({ label, tone = 'neutral' }) {
  return (
    <span className="status-indicator">
      <span className={`status-dot ${tone}`} />
      <span>{label}</span>
    </span>
  );
}

export function Badge({ label, tone = 'neutral', title }) {
  return (
    <span className={`badge ${tone}`} title={title}>
      <span className="badge-dot" aria-hidden="true" />
      <span>{label}</span>
    </span>
  );
}

export function Button({ label, onClick, tone = 'primary', icon: iconName, disabled = false, type = 'button', className = '', children, title }) {
  return (
    <button
      type={type}
      className={`btn ${tone} ${className}`}
      onClick={onClick}
      disabled={disabled}
      title={title}
    >
      {iconName && <Icon name={iconName} size={14} />}
      {label && <span>{label}</span>}
      {children}
    </button>
  );
}

export function IconButton({ name, label, onClick, tone = '', className = '' }) {
  return (
    <button
      type="button"
      className={`icon-btn ${tone} ${className}`}
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      <Icon name={name} size={16} />
    </button>
  );
}

export function Skeleton({ variant = 'text', className = '' }) {
  return <span className={`skeleton skeleton-${variant} ${className}`} aria-hidden="true" />;
}

export function SkeletonTable({ rows = 5, cols = 5 }) {
  return (
    <div className="data-table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            {Array.from({ length: cols }, (_, i) => <th key={i}><Skeleton variant="text" /></th>)}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, ri) => (
            <tr key={ri}>
              {Array.from({ length: cols }, (_, ci) => <td key={ci}><Skeleton variant="text" /></td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function EmptyState({ title, text, action }) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      {text && <p>{text}</p>}
      {action && <div className="mt-sm">{action}</div>}
    </div>
  );
}

export function LoadingState({ label = 'Loading' }) {
  return (
    <div className="loading" role="status" aria-live="polite">
      <span>{label}</span>
    </div>
  );
}

export function ErrorBox({ error, retry }) {
  return (
    <div className="card alert-box-danger" role="alert">
      <div className="card-header">
        <h2 className="text-danger">System Notice</h2>
      </div>
      <div className="card-body">
        <p>{error?.message || String(error)}</p>
        {retry && (
          <div className="mt-md">
            <Button label="Retry" onClick={retry} tone="secondary compact" />
          </div>
        )}
      </div>
    </div>
  );
}
