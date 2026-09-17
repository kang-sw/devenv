---
title: Committed project-wide ws config scope (.ws-workflow/config.json)
related:
  260917-feat-ticket-assignee-awareness: first consumer (needs a shared, tool-read project flag)
---

# Committed project-wide ws config scope (.ws-workflow/config.json)

## Background

ws has no project-wide configuration that is simultaneously (a) committed /
version-tracked so it is shared across all contributors, (b) read
deterministically by the MCP tools, and (c) effective without an agent having to
read a prose doc. Each existing option misses one of these:

- `config.tune` scopes are `session`, `project`, `global`, `builtin`. The
  "project" scope is per-machine — `~/.ws@<id>/config.json` (evidence:
  `internal/wsconfig/scope.go:10`) — so it is NOT committed and cannot carry a
  team-wide decision.
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

## Open Questions

Settle at `todo`/`ready` grounding, not now:

- Exact scope name (`repo` vs other).
- Whether the committed `repo` value is overridable by the machine `project`
  scope, or non-overridable for safety-relevant flags.
- Whether `config.tune` may write the `repo` scope (mutating a committed file)
  or the file is edited by hand only.
- Allowed-key schema and validation; `.gitignore` implications.

## Phases

Phasing deferred to grounding. Provisional single slice: add the `repo` scope
backed by `.ws-workflow/config.json`, wired into the resolver and surfaced in
`config.show` scope resolution, with missing-file = no overrides (never an
error), delivering the minimum read/resolution path a consumer (assignee) needs
to read a project flag deterministically.
