// FirstRunSetupPage — shown when the appliance has no administrator yet.
// Collects the one-time setup token (printed by the server on startup) and the
// initial administrator credentials, then calls POST /api/v1/auth/bootstrap.
// On success the server sets the session cookie, so the caller can treat this
// exactly like a login.
import { useState } from 'preact/hooks';

export function FirstRunSetupPage({ onLogin, onSwitchToLogin }) {
  const [setupToken, setSetupToken] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    setError('');
    if (password.length < 12) {
      setError('Password must be at least 12 characters.');
      return;
    }
    if (password !== confirm) {
      setError('Passwords do not match.');
      return;
    }
    setLoading(true);
    try {
      const res = await fetch('/api/v1/auth/bootstrap', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ setup_token: setupToken.trim(), username: username.trim(), password }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(body.error || 'Setup failed. Check the setup token and try again.');
        setLoading(false);
        return;
      }
      // Session cookie is set by the server on successful bootstrap.
      if (onLogin) onLogin();
    } catch (err) {
      setError(err.message || 'Network error. Is the server running?');
      setLoading(false);
    }
  }

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      background: 'var(--bg-base, #fff)',
      padding: '24px',
    }}>
      <div style={{
        width: '100%',
        maxWidth: '400px',
        border: '1px solid var(--line-subtle, #e5e7eb)',
        borderRadius: '8px',
        padding: '32px',
        background: 'var(--panel-base, #fff)',
      }}>
        <div style={{ textAlign: 'center', marginBottom: '24px' }}>
          <div style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '12px',
            marginBottom: '8px',
          }}>
            <img src="/logo.png" alt="DBVault" style={{ width: '40px', height: '40px', borderRadius: '10px', objectFit: 'cover', boxShadow: '0 4px 12px rgba(16, 185, 129, 0.25)' }} />
            <span style={{ fontWeight: 800, fontSize: '20px', letterSpacing: '-0.3px' }}>DBVault</span>
          </div>
          <p style={{ fontSize: '13px', color: 'var(--text-muted, #6b7280)', margin: 0 }}>
            First run — create the appliance administrator
          </p>
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '14px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '5px', color: 'var(--text-strong, #111)' }}>
              Setup token
            </label>
            <input
              type="text"
              autoComplete="off"
              required
              value={setupToken}
              onInput={e => setSetupToken(e.currentTarget.value)}
              placeholder="XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXX-XXXX"
              style={{
                width: '100%',
                boxSizing: 'border-box',
                padding: '8px 10px',
                border: '1px solid var(--line-subtle, #d1d5db)',
                borderRadius: '6px',
                fontSize: '12px',
                fontFamily: 'monospace',
                outline: 'none',
                background: 'var(--bg-base, #fff)',
                color: 'var(--text-strong, #111)',
              }}
            />
            <small style={{ display: 'block', fontSize: '11px', color: 'var(--text-muted, #9ca3af)', marginTop: '4px' }}>
              Printed by the server on startup and stored at the data directory (<code>setup-token</code>).
            </small>
          </div>

          <div style={{ marginBottom: '14px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '5px', color: 'var(--text-strong, #111)' }}>
              Administrator username or email
            </label>
            <input
              type="text"
              autoComplete="username"
              required
              value={username}
              onInput={e => setUsername(e.currentTarget.value)}
              placeholder="admin or admin@dbvault.local"
              style={{
                width: '100%',
                boxSizing: 'border-box',
                padding: '8px 10px',
                border: '1px solid var(--line-subtle, #d1d5db)',
                borderRadius: '6px',
                fontSize: '13px',
                outline: 'none',
                background: 'var(--bg-base, #fff)',
                color: 'var(--text-strong, #111)',
              }}
            />
          </div>

          <div style={{ marginBottom: '14px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '5px', color: 'var(--text-strong, #111)' }}>
              Password
            </label>
            <input
              type="password"
              autoComplete="new-password"
              required
              value={password}
              onInput={e => setPassword(e.currentTarget.value)}
              placeholder="At least 12 characters"
              style={{
                width: '100%',
                boxSizing: 'border-box',
                padding: '8px 10px',
                border: '1px solid var(--line-subtle, #d1d5db)',
                borderRadius: '6px',
                fontSize: '13px',
                outline: 'none',
                background: 'var(--bg-base, #fff)',
                color: 'var(--text-strong, #111)',
              }}
            />
          </div>

          <div style={{ marginBottom: '20px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '5px', color: 'var(--text-strong, #111)' }}>
              Confirm password
            </label>
            <input
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onInput={e => setConfirm(e.currentTarget.value)}
              placeholder="Repeat password"
              style={{
                width: '100%',
                boxSizing: 'border-box',
                padding: '8px 10px',
                border: '1px solid var(--line-subtle, #d1d5db)',
                borderRadius: '6px',
                fontSize: '13px',
                outline: 'none',
                background: 'var(--bg-base, #fff)',
                color: 'var(--text-strong, #111)',
              }}
            />
          </div>

          {error && (
            <div style={{
              marginBottom: '14px',
              padding: '9px 12px',
              borderRadius: '6px',
              background: 'var(--color-danger-bg, #fef2f2)',
              border: '1px solid var(--color-danger, #fca5a5)',
              fontSize: '12px',
              color: 'var(--color-danger-text, #b91c1c)',
            }}>
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              padding: '9px',
              borderRadius: '6px',
              border: 'none',
              background: loading ? 'var(--color-emerald-muted, #6ee7b7)' : 'var(--color-emerald, #10b981)',
              color: '#fff',
              fontWeight: 600,
              fontSize: '13px',
              cursor: loading ? 'not-allowed' : 'pointer',
              letterSpacing: '0.1px',
            }}
          >
            {loading ? 'Creating administrator…' : 'Create administrator & sign in'}
          </button>

          {onSwitchToLogin && (
            <div style={{ marginTop: '16px', textAlign: 'center' }}>
              <button
                type="button"
                onClick={onSwitchToLogin}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--color-emerald, #10b981)',
                  fontSize: '12px',
                  cursor: 'pointer',
                  textDecoration: 'underline',
                  padding: 0,
                }}
              >
                Already set up? Sign in with existing credentials
              </button>
            </div>
          )}
        </form>

        <p style={{ textAlign: 'center', fontSize: '11px', color: 'var(--text-muted, #9ca3af)', marginTop: '20px', marginBottom: 0 }}>
          DBVault Enterprise Backup Engine · v0.1-alpha
        </p>
      </div>
    </div>
  );
}
