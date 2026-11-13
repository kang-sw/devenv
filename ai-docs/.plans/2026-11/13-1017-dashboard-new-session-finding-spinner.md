# Dashboard: a just-created session shows "Finding session" instead of "unknown session"

## Problem

Owner report 2026-11-13: right after adding a new session from the dashboard,
the session view sometimes goes fully dark with "unknown session". It looks
like a timing issue: the page (or one of its fetches) asks the daemon for the
session before the daemon has registered/indexed the new row, gets a 404
("unknown session", e.g. `agents-plugin-tool/internal/wsdashboard/daemon.go`
~756 and the per-route handlers), and treats that as final.

## Outcome

- First establish which request turns into the dark "unknown session" view
  right after a new session is created (the page route itself, or a client
  fetch inside the SPA) and why it can precede registration. State it in the
  commit's `## AI Context`.
- While a session the client navigated to is not yet known to the daemon, the
  session view shows a full-panel "Finding session…" spinner (i18n string in
  every `internal/wsi18n/tables/*.json`) and retries every 1–2 s. When the
  session appears, the view loads normally without a manual reload.
- After ~10 s without the session, show the final not-found state (today's
  message, or a clearer one in the same place).
- A session that genuinely does not exist ends in that final state after the
  same window; no infinite spinner.
- If the root cause is a server-side ordering bug that can simply be fixed
  (e.g. the create response returns before the row is visible), fix that too
  and keep the client retry as the safety net; say which.
- T0 (dashboard asset content, and/or an additive server change).

## Out of scope

- Other session lifecycle states (stopped, ended) and their views.
- Restarting the dashboard daemon or Claude sessions (lead does the live
  check).

## Verification

- `go test ./internal/wsdashboard/... ./internal/wsi18n/... -count=1` on the
  remote w5090 (clone note `remote-build-w5090`), plus a test for the retry
  window if the JS is covered by the existing JS-extraction tests.
- Headless render or equivalent of the spinner state if cheap; state what was
  not checked visually.
