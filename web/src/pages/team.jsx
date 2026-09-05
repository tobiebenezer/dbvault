import { useState, useEffect } from 'preact/hooks';
import { API } from '../api.js';
import { Card, Button, Badge, PageHeader, DataTable, LoadingState, ErrorBox, Icon, StatusIndicator } from '../components/ui.jsx';
import { formatDate, titleCase } from '../format.js';
import { Store } from '../state.js';

export function TeamPage({ embedded = false }) {
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [editingMember, setEditingMember] = useState(null);

  const loadTeam = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await API.teamMembers();
      setMembers(res.members || []);
      setLoading(false);
    } catch (err) {
      setError(err);
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTeam();
    return Store.subscribe((s) => {
      if (s.refreshToken) loadTeam();
    });
  }, []);

  const handleRemoveMember = async (member) => {
    if (member.role === 'organisation_owner') {
      Store.toast('Organisation Owner cannot be removed', 'danger');
      return;
    }
    const ok = await Store.confirm({
      title: 'Revoke Team Member Access',
      message: `Are you sure you want to revoke workspace access for ${member.name} (${member.email})?`,
      confirmLabel: 'Revoke Access',
      confirmTone: 'danger'
    });
    if (!ok) return;
    try {
      await API.removeTeamMember(member.id);
      Store.toast(`Access revoked for ${member.name}`, 'success');
      loadTeam();
    } catch (err) {
      Store.toast(`Failed to remove member: ${err.message}`, 'danger');
    }
  };

  if (loading) return <div className={embedded ? "" : "page"}><LoadingState label="Loading workspace members…" /></div>;
  if (error) return <div className={embedded ? "" : "page"}><ErrorBox error={error} retry={loadTeam} /></div>;

  return (
    <div className={embedded ? "" : "page"}>
      {!embedded && (
        <PageHeader
          title="Team & Access Control"
          actions={[
            <Button
              key="invite"
              label="Invite Member"
              onClick={() => setShowInviteModal(true)}
              tone="primary"
              icon="plus"
            />
          ]}
        />
      )}

      {/* Team Members DataTable */}
      <Card title="Workspace Members" noPadding>
        <DataTable
          headers={[
            { label: 'Member' },
            { label: 'Assigned Role' },
            { label: 'Status' },
            { label: 'Last Active' },
            { label: 'Actions', width: '180px' }
          ]}
        >
          {members.map((m) => (
            <tr key={m.id}>
              <td className="cell-primary">
                <div>
                  <strong>{m.name}</strong>
                  <small className="text-muted block text-xs">{m.email}</small>
                </div>
              </td>
              <td><Badge label={titleCase(m.role.replaceAll('_', ' '))} tone={m.role === 'organisation_owner' ? 'success' : 'neutral'} /></td>
              <td><StatusIndicator label={titleCase(m.status)} tone={m.status === 'active' ? 'success' : 'warning'} /></td>
              <td className="cell-mono">{formatDate(m.last_active_at)}</td>
              <td className="cell-actions">
                <div className="row-sm">
                  <Button
                    label="Edit Role"
                    onClick={() => setEditingMember(m)}
                    tone="ghost compact"
                  />
                  {m.role !== 'organisation_owner' && (
                    <Button
                      label="Remove"
                      onClick={() => handleRemoveMember(m)}
                      tone="danger compact"
                    />
                  )}
                </div>
              </td>
            </tr>
          ))}
        </DataTable>
      </Card>

      {/* RBAC Permission Matrix Card */}
      <Card title="Role Permissions Matrix">
        <div className="stack-sm text-sm">
          <div className="row-between list-item-row">
            <div>
              <strong>Organisation Owner</strong>
              <small className="text-muted block">Full root control over appliances, billing, encryption keys, and membership.</small>
            </div>
            <Badge label="All 15 Permissions" tone="success" />
          </div>
          <div className="row-between list-item-row">
            <div>
              <strong>Backup Administrator (DBA)</strong>
              <small className="text-muted block">Can create backup snapshots, configure databases, and execute sandbox restore drills.</small>
            </div>
            <Badge label="Backup + Drill Active" tone="neutral" />
          </div>
          <div className="row-between list-item-row">
            <div>
              <strong>Restore Approver (Two-Person Rule)</strong>
              <small className="text-muted block">Authorized to review and approve destructive production in-place database restores.</small>
            </div>
            <Badge label="Restore Authorizer" tone="warning" />
          </div>
          <div className="row-between list-item-row">
            <div>
              <strong>External Auditor</strong>
              <small className="text-muted block">Read-only access to tamper-evident audit logs, verification chains, and drill compliance proofs.</small>
            </div>
            <Badge label="Audit Export Only" tone="neutral" />
          </div>
        </div>
      </Card>

      {/* Invite Member Modal */}
      {showInviteModal && (
        <InviteMemberModal
          onClose={() => setShowInviteModal(false)}
          onInvited={() => {
            setShowInviteModal(false);
            loadTeam();
          }}
        />
      )}

      {/* Edit Role Modal */}
      {editingMember && (
        <EditRoleModal
          member={editingMember}
          onClose={() => setEditingMember(null)}
          onSaved={() => {
            setEditingMember(null);
            loadTeam();
          }}
        />
      )}
    </div>
  );
}

function InviteMemberModal({ onClose, onInvited }) {
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [role, setRole] = useState('backup_administrator');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!email.trim()) {
      Store.toast('Email is required', 'danger');
      return;
    }
    try {
      setSubmitting(true);
      await API.inviteTeamMember({
        email: email.trim(),
        name: name.trim() || email.split('@')[0],
        role
      });
      Store.toast(`Invitation dispatched to ${email}`, 'success');
      onInvited();
    } catch (err) {
      Store.toast(`Invitation failed: ${err.message}`, 'danger');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Invite team member"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="command-panel" style={{ maxWidth: '480px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Invite Team Member</h2>
            <p className="card-subtitle">Grant access to this DBVault appliance with tailored permissions.</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="stack-md">
          <div className="form-field">
            <label>Member Email</label>
            <input
              type="email"
              className="form-input"
              placeholder="engineer@company.com"
              value={email}
              onInput={(e) => setEmail(e.currentTarget.value)}
              required
            />
          </div>

          <div className="form-field">
            <label>Display Name</label>
            <input
              type="text"
              className="form-input"
              placeholder="Alex Smith"
              value={name}
              onInput={(e) => setName(e.currentTarget.value)}
            />
          </div>

          <div className="form-field">
            <label>Assigned Role</label>
            <select className="form-select" value={role} onChange={(e) => setRole(e.currentTarget.value)}>
              <option value="backup_administrator">Backup Administrator (DBA)</option>
              <option value="restore_approver">Restore Approver (Two-Person Rule)</option>
              <option value="auditor">SOC2 / Compliance Auditor (Read-Only)</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>

          <div className="row-actions mt-md">
            <Button type="submit" label={submitting ? "Sending..." : "Send Invitation"} tone="primary" />
            <Button label="Cancel" onClick={onClose} tone="ghost" />
          </div>
        </form>
      </div>
    </div>
  );
}

function EditRoleModal({ member, onClose, onSaved }) {
  const [role, setRole] = useState(member.role || 'backup_administrator');
  const [status, setStatus] = useState(member.status || 'active');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      setSubmitting(true);
      await API.updateTeamMember(member.id, { role, status });
      Store.toast(`Role and status updated for ${member.name}`, 'success');
      onSaved();
    } catch (err) {
      Store.toast(`Update failed: ${err.message}`, 'danger');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="command-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Edit Member Role"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="command-panel" style={{ maxWidth: '480px', padding: '24px' }}>
        <div className="row-between mb-md">
          <div className="card-heading">
            <h2>Edit Access & Role</h2>
            <p className="card-subtitle">Update permissions and membership status for {member.name}.</p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Close dialog">
            <Icon name="close" size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="stack-md">
          <div className="form-field">
            <label>Member</label>
            <div className="text-sm font-semibold">{member.name} ({member.email})</div>
          </div>

          <div className="form-field">
            <label>Assigned Role</label>
            <select className="form-select" value={role} onChange={(e) => setRole(e.currentTarget.value)}>
              <option value="organisation_administrator">Organisation Administrator</option>
              <option value="security_administrator">Security Administrator</option>
              <option value="backup_administrator">Backup Administrator (DBA)</option>
              <option value="restore_approver">Restore Approver (Two-Person Rule)</option>
              <option value="operator">Operator</option>
              <option value="auditor">SOC2 / Compliance Auditor (Read-Only)</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>

          <div className="form-field">
            <label>Membership Status</label>
            <select className="form-select" value={status} onChange={(e) => setStatus(e.currentTarget.value)}>
              <option value="active">Active</option>
              <option value="suspended">Suspended</option>
            </select>
          </div>

          <div className="row-actions mt-md">
            <Button type="submit" label={submitting ? "Saving..." : "Save Changes"} tone="primary" />
            <Button label="Cancel" onClick={onClose} tone="ghost" />
          </div>
        </form>
      </div>
    </div>
  );
}
