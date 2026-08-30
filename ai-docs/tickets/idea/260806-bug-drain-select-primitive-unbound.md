---
title: lead-drain-ready-queue Select names no discovery primitive, leaving the selector free to shell-search
related:
  260806-feat-worktree-ticket-scope: surfaced this while checking whether a scoped queue would reach the selector; independent of that ticket because a filesystem filter reaches the selector whichever tool it picks
---

# lead-drain-ready-queue Select names no discovery primitive, leaving the selector free to shell-search

## Background

`agents-plugin/skills/lead-drain-ready-queue/SKILL.md`, the `## Select` section,
delegates ticket selection without naming a primitive:

> Spawn a light-tier Explore-style subagent to pick the next ticket. Do not list
> `ready/` or read ticket files yourself - the subagent does that pinpoint read.

The brief that subagent receives (`agents-plugin/rsrc/explore/`) is a generic
codebase-exploration brief whose process step 2 is "Use broad search before
opening specific files". It never mentions the ws ticket tools.

So whether the selector calls `ws/tickets.list(status: "ready")` or globs
`ai-docs/tickets/ready/*.md` is left to the model. That contradicts the workflow
manual's stated rule:

> Use the ws-owned ticket, spec, and mental-model discovery tools for
> path/status/reference lookup **before shell search**.

`ready/` selection is the single highest-traffic discovery call in the workflow -
it runs once per drain cycle - and it is the one place the rule is not applied.

## Direction

Name the primitive in `## Select`. The section already specifies the selection
policy (skip recorded blockers, prefer in-progress, then a prerequisite named by
another ready ticket's `related:`/`parent:`, else FIFO); it needs the lookup
surface stated alongside it.

## Substitution is not a blocker (verified 2026-08-06)

The concern that naming a `ws/` tool would break the wsflow mirror does not
hold. `internal/wsrsrc/skills_mirror.go` substitutes exactly these tokens:

```go
wsColonPattern = regexp.MustCompile(`\bws:`)   // -> wsflow:
wsSlashPattern = regexp.MustCompile(`\bws/`)   // -> wsflow/
```

`ws/tickets.list(...)` is precisely the eligible form. Checked further:

- The skill contains none of the `disqualifyingTokens` (`mercenary`, `ws.`, the
  full-only/wsflow-only markers, the retired `lead-write-*` stems).
- `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` is byte-identical
  to a naive `ws:`/`ws/` substitution of the full-ws source, confirming the skill
  is generated through that mirror today.

## Open

- Whether the fix belongs only in `## Select`, or whether the explore brief
  should also point at the ws discovery tools for ticket-shaped questions in
  general. The narrow fix is safe; the broad one touches every explore consumer.
