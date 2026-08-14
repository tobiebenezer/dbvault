import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Button, Badge, PageHeader, SegmentedNav, EmptyState, LoadingState, ErrorBox } from '../components/ui.jsx';
import { Store } from '../state.js';
import { ProductActions } from '../actions.js';
import { formatRelative } from '../format.js';

export function AlertsPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState(Store.state.alertsTab || 'active');
  const [selectedAlertId, setSelectedAlertId] = useState(Store.state.selectedAlert);

  const loadAlerts = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.alerts();
      setData(res);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAlerts();
    return Store.subscribe((s) => {
      setSelectedAlertId(s.selectedAlert);
      if (s.alertsTab) setActiveTab(s.alertsTab);
    });
  }, []);

  if (loading) return <div className="page"><LoadingState label="Loading incidents & alerts…" /></div>;
  if (error) return <div className="page"><ErrorBox error={error} retry={loadAlerts} /></div>;

  const all = data?.alerts || [];
  const active = all.filter((alert) => alert.status !== 'resolved');
  const resolved = all.filter((alert) => alert.status === 'resolved');
  const list = activeTab === 'active' ? active : resolved;
  const lastCheck = data?.last_checked_at || null;

  return (
    <div className="page">
      <PageHeader
        title="Alerts & Incidents"
        description="Active protection warnings, recovery chain gap alerts, and storage quota notifications."
        actions={[
          <Button key="doc" label="Run system doctor" onClick={() => ProductActions.doctor()} tone="secondary" />
        ]}
      />

      <SegmentedNav
        tabs={[
          { id: 'active', label: 'Active', count: active.length },
          { id: 'resolved', label: 'Resolved History', count: resolved.length }
        ]}
        activeId={activeTab}
        onSelect={(tab) => {
          setActiveTab(tab);
          Store.set({ alertsTab: tab });
        }}
      />

      {list.length > 0 ? (
        <div className="stack-sm">
          {list.map((alert) => {
            const tone = alert.severity === 'critical' ? 'danger' : alert.severity === 'action_required' ? 'warning' : 'neutral';
            const action = (alert.suggested_actions || [])[0];
            const expanded = selectedAlertId === alert.id;
            const isResolved = activeTab === 'resolved';

            return (
              <article key={alert.id} className={`alert-row-wrap ${tone}`}>
                <div className="alert-row">
                  <div className="alert-row-copy">
                    <div className="row-sm">
                      <strong>{alert.title}</strong>
                      <Badge
                        label={isResolved ? 'Resolved' : alert.status === 'acknowledged' ? 'Acknowledged' : alert.severity === 'action_required' ? 'Action Required' : alert.severity}
                        tone={isResolved ? 'success' : alert.status === 'acknowledged' ? 'neutral' : tone}
                      />
                    </div>
                    <p>{alert.safety_impact || alert.summary}</p>
                  </div>
                  <div className="row-actions">
                    {!isResolved && action && (
                      <Button
                        label={action.label}
                        onClick={() => ProductActions.executeAlertAction(alert, action)}
                        tone="secondary compact"
                      />
                    )}
                    <Button
                      label={expanded ? 'Hide details' : 'Investigate'}
                      onClick={() => Store.set({ selectedAlert: expanded ? null : alert.id })}
                      tone="ghost compact"
                    />
                  </div>
                </div>

                {expanded && (
                  <div className="alert-details">
                    <div className="stack-sm">
                      <small>Root Cause</small>
                      <span>{alert.likely_cause || 'Automated inspection in progress'}</span>
                    </div>
                    <div className="stack-sm">
                      <small>Safety & Recovery Impact</small>
                      <span>{alert.safety_impact || alert.summary}</span>
                    </div>
                    <div className="stack-sm">
                      <small>Diagnostic Evidence</small>
                      <span>{(alert.evidence || []).map((item) => item.summary).join(' · ') || 'No additional evidence recorded'}</span>
                    </div>
                    {alert.occurred_at && (
                      <div className="stack-sm">
                        <small>Occurred</small>
                        <span>{formatRelative(alert.occurred_at)}</span>
                      </div>
                    )}
                    {!isResolved && (
                      <div className="row-actions mt-sm">
                        {alert.status !== 'acknowledged' && (
                          <Button
                            label="Acknowledge"
                            onClick={async () => {
                              await ProductActions.acknowledgeAlert(alert.id);
                              loadAlerts();
                            }}
                            tone="secondary compact"
                          />
                        )}
                        <Button
                          label="Mark as resolved"
                          onClick={async () => {
                            await ProductActions.resolveAlert(alert.id);
                            loadAlerts();
                          }}
                          tone="ghost compact"
                        />
                      </div>
                    )}
                  </div>
                )}
              </article>
            );
          })}
        </div>
      ) : activeTab === 'active' ? (
        <EmptyState
          title="All Systems Clear"
          text={lastCheck ? `No active alerts. Last checked ${formatRelative(lastCheck)}.` : 'No active alerts. Continuous protection checks are passing.'}
        />
      ) : (
        <EmptyState
          title="No Resolved Alerts"
          text="Resolved incidents will appear here for audit and review."
        />
      )}
    </div>
  );
}
