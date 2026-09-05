---
session: ses_fa5f
updated: 2026-09-01T00:54:05.422Z
---

Now the render branch — showing the first-run page instead of login when needed:
[Assistant tool call]: edit({"filePath":"/home/sifu/Documents/dbvault/web/src/main.jsx","newString":"  // Not authenticated → show first-run setup (no admin yet) or login page\n  if (!authed) {\n    if (needsBootstrap) {\n      return (\n        <FirstRunSetupPage onLogin={() => {\n          // Bootstrap set the session cookie — re-check the session.\n          fetch('/api/v1/auth/session', { credentials: 'same-origin' })\n            .then(res => setAuthed(res.ok))\n            .catch(() => setAuthed(false));\n        }} />\n      );\n    }\n    return (\n      <LoginPage onLogin={() => {","oldString":"  // Not authenticated → show login page\n  if (!authed) {\n    return (\n      <LoginPage onLogin={() => {"})
[Tool result]: Edit applied successfully.
