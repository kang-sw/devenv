# Plan: the conversation's tool summary line shows its numbers on a second row

User request (dashboard dogfood, board (37)): in the conversation view, the
tool summary pill (`.toolsline > summary`, `{{define "toolsum"}}` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:236`) puts the
per-tool call counts first and the numbers that matter (failed count,
duration, token usage `.ttok`) after them in one `nowrap` + ellipsis span
(`dashboard.css:315`), so with several tools the numbers are cut off.

## Outcome

- Collapsed: the summary renders as two rows. Row 1: the per-tool call
  counts (and the "rest"/"calls" text), truncated with an ellipsis when too
  long. Row 2: the numbers - failed count, duration, token usage - never
  truncated by row 1.
- Open (`details[open]`): row 1 is hidden, since the batch list below shows
  every tool; row 2 stays.
- When there are no numbers (no failure, duration, or usage), the collapsed
  pill is a single row as today.
- Applies to both uses of `toolsum`: finished tool entries (`entry`, kind
  `tools`) and the live tool gap in `r-convlive`.
- Keep the pill look (muted, rounded, chevron). The live page re-renders
  regions via morphing; keep `data-key`/`data-entry` and the open state
  behaviour intact.

## Settled constraints

- Compatibility tier T0: dashboard asset content only (template and CSS; JS
  only if the open/close state truly needs it). No Go view-model, SSE, MCP,
  or store change beyond what a template split strictly needs.
- Files: `assets/dashboard.tmpl`, `assets/dashboard.css`, and
  `conversation_tools_test.go` (update the markup assertions; add one for
  the two-row split). Stop and report if more is needed.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.

## Verification

- `go test ./internal/wsdashboard/...` through ws-rgo.
- Describe in the commit how the collapsed and open states look (no browser
  available to you); the lead checks it live after a daemon restart.
- `git diff` reviewed against this plan before commit on
  `impl/develop/toolsum-two-rows`.
