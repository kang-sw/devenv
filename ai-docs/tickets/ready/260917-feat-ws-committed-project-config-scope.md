---
title: Committed project-wide ws config scope (.ws-workflow/config.json)
related:
  260917-feat-ticket-assignee-awareness: first consumer (needs a shared, tool-read project flag)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 474dfe33c67aeda3
sage-review-completeness-reviewed: 474dfe33c67aeda3
---

# Committed project-wide ws config scope (.ws-workflow/config.json)

## Background

ws has no project-wide configuration that is simultaneously (a) committed /
version-tracked so it is shared across all contributors, (b) read
deterministically by the MCP tools, and (c) effective without an agent having to
read a prose doc. Each existing option misses one of these:

- `config.tune` scopes are `session`, `project`, `global`, `builtin`. The
  "project" scope is per-machine — `~/.ws@<id>/config.json` (evidence:
  `agents-plugin-tool/internal/wsconfig/scope.go:10`) — so it is NOT committed
  and cannot carry a team-wide decision.
- Committed config that does exist (`ai-docs/ship/*.md`, AGENTS.md
  `### Review Policy` / `### Binding Anchor` blocks) is agent-read prose/markdown,
  not tool-parsed deterministic config.

This gap blocks any feature whose switch must be a shared project decision
honored by tools without agent mediation. The immediate driver is
`260917-feat-ticket-assignee-awareness`, whose `ticket-assignee-aware` flag must
be identical for every contributor and read by `tickets.query` / `create`.

## Decisions

- New committed config file at repo root: `.ws-workflow/config.json`
  (git-tracked; each downstream project owns its own). Rejected: extending
  AGENTS.md blocks (tools do not parse it; agents do not reliably read it) and
  reusing the machine-local `project` scope (not shared across contributors).
- New config scope, tentatively named `repo` (committed), layered into the
  existing wsconfig resolution. Proposed precedence:
  `session > project(machine) > repo(committed) > global > builtin` — the
  committed repo value is the shared baseline, a per-machine override can still
  win for local experimentation.
- Shipped-surface clean: the shipped MCP server reads a conventional path; the
  file lives in the downstream repo, so nothing depends on this repository alone
  (Architecture Rule 4 / `shipped-surface-boundary.md`).
- Scope minimally to what the assignee feature needs. Do not pre-migrate
  review-policy / binding-anchor into it; add keys only as a consumer requires
  them.

## Constraints

- `agents-plugin-tool/internal/mcp/` edits read `ai-docs/manuals/ws-mcp.md`.
- Config surface changes touch `agents-plugin-tool/internal/wsconfig/`; extend
  the existing scope resolver (`resolver.go`) rather than adding a parallel path.
- This is an observable, shipped workflow behavior change; host-neutral and
  mirror obligations apply to any playbook text that references the new scope.
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Open Questions

Settle at `todo`/`ready` grounding, not now:

- Exact scope name (`repo` vs other).
- Whether the committed `repo` value is overridable by the machine `project`
  scope, or non-overridable for safety-relevant flags. The
  `260917-feat-ticket-assignee-awareness` consumer constrains this: its
  `ticket-assignee-aware` flag is meant to be a shared, deterministic gate, so
  if the machine `project` scope can override the committed value a contributor
  could silently disable the gate locally. Settle whether such flags are marked
  non-overridable (or whether local override is accepted as intended, since the
  feature is opt-in coordination, not adversarial enforcement).
- Whether `config.tune` may write the `repo` scope (mutating a committed file)
  or the file is edited by hand only.
- Allowed-key schema and validation; `.gitignore` implications.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | .ws-workflow/config.json, agents-plugin-tool/internal/wsconfig/resolver.go, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin-tool/internal/mcp/server.go |
| scope.surface | public-interface | config.list resolved-scope output gains a repo scope value (agents-plugin-tool/internal/mcp/server.go:694-717) |
| scope.new_public_symbol | yes | wsconfig.ScopeRepo, mirroring wsconfig.ScopeProject/ScopeGlobal (agents-plugin-tool/internal/wsconfig/scope.go:9-15) |
| scope.new_type_contract | no | extends Resolver.Get/Set's existing inline session/project/global/builtin chain with one more loader in the same pattern as loadGlobalConfig; no new interface (agents-plugin-tool/internal/wsconfig/resolver.go:80-131) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsconfig/scope_test.go and config_test.go already cover scope-resolution ordering |
| complexity.reuse_points | confirmed | Resolver.Get's project/global load-and-lookup pattern (resolver.go:92-112) is the template for the repo-scope load |
| complexity.side_effect_risk | moderate | a new file read is added to every config resolution call, and precedence-chain edits touch every existing config item's read path, not just the new key |
| risk.correctness | moderate | Resolver.Get/Set/Unset hardcode the scope chain inline; a wrong insertion point or repo-root discovery bug (e.g. worktrees) would silently mis-resolve values for all config items |
| risk.fit | low | the design slots into the existing session/project/global/builtin resolver architecture already documented in resolver.go and scope.go |
| risk.test | moderate | needs new precedence and missing-file cases beyond the existing scope_test.go/config_test.go coverage, including repo-root discovery under nested worktrees |
| risk.security_or_contract | moderate | a committed, shared file can gate contributor-visible behavior (e.g. the assignee feature's flag) for everyone from a single commit; write-path and `.gitignore` handling are still open questions |

## Phases

Phasing deferred to grounding. Provisional single slice: add the `repo` scope
backed by `.ws-workflow/config.json`, wired into the resolver and surfaced in
`config.list` scope resolution (the current combined config tool; it subsumes
the former standalone `config.show` resolved-scope view —
`agents-plugin-tool/internal/mcp/server.go:694-705`), with missing-file = no
overrides (never an error), delivering the minimum read/resolution path a
consumer (assignee) needs to read a project flag deterministically. Verify with
new resolver tests covering precedence ordering (committed `repo` vs each other
scope), missing-file = no-override, and repo-root discovery under nested
worktrees, plus a `config.list` case showing the `repo` scope value in resolved
output.
