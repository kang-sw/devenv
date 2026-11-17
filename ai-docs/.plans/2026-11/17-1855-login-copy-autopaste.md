# UX: the login gate logs in right after its copy button succeeds

## Context

The owner asked on 2026-11-17 at 18:52 for this flow: on the login gate,
pressing copy makes the daemon put a login link on the clipboard. Once that
REST call returns success, the clipboard surely holds the link, so the page
reads the clipboard itself and logs in, without the user pressing Ctrl+V.
The input field below stays as the fallback.

A read-only survey (file:line on develop `875de30bd`, paths under
`agents-plugin-tool/internal/wsdashboard/`) found most pieces already exist:
- The gate is a daemon page: `login_gate.go:28-84`, with
  `assets/login-gate.tmpl/.css/.js`. Its CSP has `connect-src 'self'`.
- The copy button calls `POST /auth/clipboard` (`localhost_clipboard.go:140-190`).
  That route writes a fresh login link to the OS clipboard and returns
  `{ok:true}`. It is rendered only when `localViewer(r) && d.clipboardAvailable()`
  (`login_gate.go:58`), and it is rate-limited.
- A "Paste from clipboard" button already calls `navigator.clipboard.readText()`
  on click (`login-gate.js:116-120`). A document paste listener
  (`:110-113`) and the `POST /auth/exchange` submit path (`:64`) exist too.

## Outcome

1. After `/auth/clipboard` returns ok, the gate immediately calls
   `navigator.clipboard.readText()` and submits the result through the
   existing exchange path, exactly as the paste button does. Reuse that code
   rather than duplicating it.
2. If the read is refused, throws, or returns text that is not a login link,
   fall back quietly:
   - keep today's state, where the link is on the clipboard and the input,
     paste button and paste listener work;
   - show a short hint that the user can paste.
   - Never loop, and never retry the copy automatically.
3. The input field and the paste button stay as they are.
4. Note in the commit how the read behaves in Chrome/Edge (a permission
   prompt on first use, then allowed) and in Firefox (needs user activation
   and may show a paste menu). Code reasoning is enough; no browser run is
   required.

## Constraints

- **Scope.** Only `login-gate.js`, plus the template, CSS and i18n strings if
  a hint string is needed. No server route changes. Tier T0.
- **Security.**
  - Do not change what `/auth/clipboard` or `/auth/exchange` accept.
  - The page must never receive the login code by any channel other than the
    clipboard read it already uses.
  - Never log clipboard contents.
- **Tests.** Extend the existing login gate tests (find the node or Go
  harness that covers `login-gate.js`). Cover:
  - copy ok, then read, then exchange is called;
  - a read failure falls back without an exchange and shows the hint.
  - Node JS tests may run locally; Go only on w5090.
- **Coordination: do not touch these.**
  - `access.go`, `localhost_auth.go`: another worker is caching the secret
    there;
  - dashboard CSS outside the gate;
  - `dashboard-session-view.js`.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run login-autopaste 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test -count=1 ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/login-autopaste/`.
  - Run it with run_in_background and wait for it to finish. Do not end your
    turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/login-copy-autopaste` with your own
  model's Co-Authored-By trailer. Do not push.
- **Temp files** go only under `D:/Temp/login-autopaste/`.

## Report

- what changed, and where;
- the browser behaviour notes;
- the tests;
- the commit and files;
- the w5090 result;
- anything left alone.
