# Plan: an open dashboard tab notices a daemon serving different page assets

Ticket: `261112-bug-ws-dashboard-open-tab-keeps-stale-assets-after-daemon-upgrade`
(idea/; read it first: background, settled decisions, constraints).

## Outcome

A dashboard tab opened before a daemon replacement notices, when its SSE
stream reconnects to the new daemon, that the daemon serves different page
assets than the ones it runs. It then reloads at once when `holdsDraft()` is
false, and otherwise shows a banner asking for a reload (the `modeChanged` /
`setLang` pattern). A replacement that leaves the page assets unchanged does
nothing visible.

## Settled constraints (from the ticket's Decisions)

- Identity: a content hash of the embedded page assets (script, style,
  template, and any other asset the page inlines). Not the daemon build time.
- Delivery: the SSE stream's first event on every (re)connect carries the
  identity (additive field or event); the page compares it with a value
  rendered into the page (for example a `data-` attribute on the root).
- Mismatch handling reuses `holdsDraft()` and the existing `banner()` helper;
  add a localized banner string beside the existing `js.banner.*` keys (check
  how those are defined for every supported language).
- Dev asset mode (`internal/wsdashboard/devassets.go`): the identity follows
  the files actually served, and the page must never reload in a loop.
- Every page kind that inlines the script and holds an SSE stream gets the
  check (session page, and any index/rail page that does the same); identify
  them, do not guess.
- Compatibility tier T0: asset content plus an additive SSE field. No MCP,
  store, or tracker shape change. An old page that ignores the new field
  keeps working; a new page talking to an old daemon (no field) does nothing.
- Expected files: `internal/wsdashboard/sse.go`, the page view model and
  template, `assets/dashboard.js`, the i18n strings, plus tests. Stop and
  report if the change needs more than that.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.

## Verification

- Go tests: the identity is stable for the same assets and differs when an
  asset changes; the rendered page carries it; the SSE first event carries the
  same value; dev mode follows the served files.
- JS: if the package has a JS test harness, cover reload vs banner; otherwise
  describe the manual check in the commit.
- `go test ./internal/wsdashboard/...` and `go vet` through ws-rgo.
- `git diff` reviewed against this plan before commit, on
  `impl/develop/stale-tab-assets`, with `## Ticket Updates` naming the ticket.
