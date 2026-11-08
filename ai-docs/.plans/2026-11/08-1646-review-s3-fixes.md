# Plan: review sweep S3 fixes

Branch `delegate/review-s3-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here, one commit per logical unit; do not merge or push.

Source: review sweep segment S3 (`8e21ac7a5..895c62c90`, board (446) b).
Full findings: `/tmp/review-sweep/S3-correctness.md`, `S3-test.md`,
`S3-fit.md` (read the Important entries; they carry file:line detail). Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`, and `ai-docs/manuals/ws-mcp.md` for `internal/mcp/`.
Authorities: plan `ai-docs/.plans/2026-11/08-1019-*` (dashboard theme),
`ai-docs/tickets/.done/*daemon-agent-runner*` (runner, decision 318 lazy
resume), the tracker data-structure ticket and `ai-docs/ref/tracker-mockup/`
(engine.js is the reference engine; golden parity via
`testdata/gen-golden.js`). Do not change the ledger or tracker wire format.

## Fix (each behavior fix with a test that fails before it)

1. **Theme does not reach topic-body frames.** `internal/wswhiteboard/render.go`
   `framedDoc` (~L426) emits no `data-theme`, and `dashboard.js` `setTheme`
   changes only the host root, so a pinned theme opposite to the OS gives
   light-on-light (or dark-on-dark) body text. Make frames follow the pinned
   theme at first render and on switch (e.g. the frame reads the theme from the
   host / the host posts it to frames). Same fix: mermaid diagrams
   (`mermaid-init.js` ~L22) take the pinned theme and re-render on switch
   (re-graded from Minor; plan 08-1019 decision 3 says it applies
   immediately). Go test for the first-byte frame theme; verify the switch in
   headless Chromium (cached Playwright under
   `~/.npm/_npx/*/node_modules/playwright`).
2. **`max_bytes` verdict depends on CPU architecture.** `obj.intOf`
   (`internal/wstracker/check.go` ~L632, uses at `state.go` ~206/314/1093,
   `payload.go` ~232) converts with `int(f)`; out-of-range float-to-int is
   platform-defined (1e19: arm64 MaxInt64 accepted, amd64 MinInt64 rejected).
   Bound the conversion explicitly so the Go fold matches the JS reference
   engine on every platform (check engine.js for its exact rule: clamping,
   integer check). Add reasons.json cases for a huge value, regenerate goldens
   through gen-golden.js, and a Go unit test on intOf at the boundaries.
3. **Prune can delete open agent conversations** (re-graded from Minor: data
   loss with a plausible trigger, the daemon down for over 7 days).
   `internal/mcp/dashboard_sessions.go` ~L177 -> `wsdashboard/registry.go`
   ~L233 can delete a runner conversation directory (`agent.json`, held sends,
   the pi session file) before the daemon refreshes its registration. Make
   prune spare agent conversations (registration `Agent: true`, or a session
   directory holding `agent.json`); their lifetime is the runner's. Test it.
4. **Scanner error stalls the turn.** `internal/wsagent/stream.go` ~L93
   ignores `sc.Err()`; an over-long line or read error ends the reader
   silently and the child blocks. Surface it as Fatal (kill the child, end the
   turn with the error). Covered by item 5's tests.

## Test gaps (Important)

5. `wsagent` stream lifecycle (stream.go ~63-130, backend.go ~133-160):
   non-zero exit surfaces `<cmd> exited: <stderr tail>` as Fatal; Fatal ->
   Kill; `stopChild` escalation close-stdin -> SIGTERM -> SIGKILL after
   closeGrace; `request` timeout / exited-during-request / "unsupported" ->
   ErrUnsupported. Use stand-in child scripts (one ignoring stdin EOF, one
   exiting 1 with stderr, one silent), with short timeouts injected so the
   tests are not load-flaky.
6. `ClaudeBackend.Start`, `claudeProtocol.userLine/requestLine`,
   `claudeProcess.Interrupt/SetModel` (claude.go ~221-274): a fake `claude`
   script logging stdin, as pi_test.go does for pi; pin command line, cwd, env
   scrub, lookPath failure, the user line, and the control_request payloads.
7. Attachment limit boundaries (check.go ~164, state.go ~207/315):
   reasons.json cases for size == limit (accepted), size 0, negative size, and
   an invalid genesis `attachments.max_bytes` (rolesProblems message and the
   DefaultAttachmentMax fallback); regenerate goldens.

## Fit (Important)

8. `ai-docs/manuals/ws-mcp.md`: one pass over the Dashboard daemon section
   and the env table: runner registrations (written by the runner, not only
   a serving ws-mcp; `Agent`, `Unlisted`), `/agents` REST routes and their
   JSON-body + own-origin POST rule, `agent.json` / `instructions.md` in the
   session directory, the `agent.profile.<name>` family (reserved `agent.`
   prefix), `WS_MCP_NO_DASHBOARD`, `WS_MCP_ALLOWED_TOOLS`, and the
   `ws_dashboard_theme` cookie beside `ws_dashboard_lang`.
9. Deduplicate `hashLiveStores`: one exported function in `wsdashboard`
   (state dir resolution and clock as arguments) used by both
   `wsdashboard/agent_sessions.go` ~78 and `mcp/dashboard_sessions.go` ~512;
   existing tests keep passing.

## Out of scope

Every other Minor in the three reports (the lead records them as
follow-ups), including the pi tool-check fail-open, the duplicate
`sameOrigin`, duplicate env-name constants, and doc nits.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./... && go test -count=1 ./internal/wsagent/ ./internal/wstracker/ ./internal/wswhiteboard/ ./internal/wsdashboard/ && go test -count=1 -run 'Dashboard|Whiteboard|Session' ./internal/mcp/`,
`node --check` on changed JS, the golden regeneration clean, and the
headless theme-switch check.

## Return

Commit hashes, changed files, test output (which new tests failed before
their fix), screenshot or check output for item 1, and anything left open.
