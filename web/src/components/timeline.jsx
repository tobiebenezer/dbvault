import { Badge } from './ui.jsx';
import { formatDate } from '../format.js';

export function RecoveryTimeline({ data = {} }) {
  const events = (data.events || []).slice(-6);
  const start = data.earliest ? new Date(data.earliest).getTime() : Date.now() - 86400000;
  const end = data.latest ? new Date(data.latest).getTime() : Date.now();
  const span = Math.max(1, end - start);

  const posClasses = ['marker-pos-10', 'marker-pos-25', 'marker-pos-40', 'marker-pos-55', 'marker-pos-70', 'marker-pos-85', 'marker-pos-95'];

  return (
    <div className="timeline-card">
      <div className="timeline-summary">
        <span>Recovery Range: {shortDate(data.earliest)} – Now</span>
        <Badge
          label={data.continuous ? 'Continuous Archive' : `${(data.gaps || []).length} Log Gaps`}
          tone={data.continuous ? 'success' : 'danger'}
        />
      </div>
      <div
        className={`timeline-track ${data.continuous ? 'continuous' : 'has-gaps'}`}
        role="img"
        aria-label={`Recovery coverage from ${formatDate(data.earliest)} to ${formatDate(data.latest)}`}
      >
        <div className="timeline-line" />
        {events.map((event, idx) => {
          const occurred = new Date(event.occurred_at || event.OccurredAt).getTime();
          const ratio = Math.max(0, Math.min(1, (occurred - start) / span));
          const posClassIndex = Math.min(posClasses.length - 1, Math.floor(ratio * (posClasses.length - 1)));
          const posClass = posClasses[posClassIndex] || 'marker-pos-55';

          return (
            <button
              key={idx}
              className={`timeline-marker ${event.status || ''} ${event.type || ''} ${posClass}`}
              title={event.description || event.label}
              aria-label={`${event.label}: ${event.status}`}
              type="button"
            >
              <span className="timeline-marker-dot" />
              <span className="timeline-marker-label">
                <strong>{shortLabel(event.label)}</strong>
                <small>{shortDate(event.occurred_at || event.OccurredAt)}</small>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function shortDate(value) {
  if (!value) return 'Unknown';
  return new Date(value).toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function shortLabel(value = '') {
  return value.replace(' backup', '').replace('Restore drill', 'Restore test').slice(0, 20);
}
