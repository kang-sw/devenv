---
title: Ticket assignee awareness (opt-in, deterministic ownership gate)
related:
  260917-feat-ws-committed-project-config-scope: prerequisite (home of the ticket-assignee-aware flag)
  260913-bug-ticket-selector-chooses-recorded-blocker: same small-tier selector surface; motivates deterministic (not prose) gating
---

# Ticket assignee awareness (opt-in, deterministic ownership gate)

## Background

In a multi-contributor repo, contributors fork the shared `ready/` inventory and
run tickets independently, so two people can start the same or ordering-sensitive
work concurrently and tangle the sequence. ws has no notion of ticket ownership
today: no assignee/owner field anywhere; the frontmatter parser passes unknown
keys through but nothing surfaces or gates on ownership; the selector's only skip
is the body `## Blocked` note plus the dispatch-time `dispatch_blocked` gate.

This adds an opt-in, deterministic assignee gate: a contributor is steered to
their own tickets and loudly warned before starting someone else's. Design
audience is the downstream multi-contributor project — this repository is
single-maintainer dogfood (see AGENTS.md `## Project Scope`), so its own posture
is not the design target. The feature is off by default.

## Decisions

- **Enablement:** project flag `ticket-assignee-aware: bool`, default **off**,
  stored in the committed project config
  (`260917-feat-ws-committed-project-config-scope`, prerequisite — the flag must
  be shared across contributors and tool-read, so it cannot live in machine-local
  `project` scope or AGENTS.md prose). Off ⇒ the entire feature is inert.
- **Identity key = git `user.email`** (case-insensitive exact match). Rejected:
  git `user.name` (non-unique, contains spaces/unicode, freely mutable) and a
  project handle/roster (more robust but adds config surface — deferred; email is
  the low-cost git-native start). Requires a new "current git user" read
  (`git config user.email`); no such primitive exists in ws today.
- **Storage syntax:** frontmatter `assignee:` as a YAML sequence (one email per
  item). Rejected a `|`-delimited scalar because `|` is legal in a git name and
  would collide; the existing parser already yields `[]string` for `- item`
  lists. Empty/absent `assignee` = **assign-any** (anyone may take it); every
  existing ticket is assign-any ⇒ backward compatible.
- **Auto-fill on create:** `tickets.create_empty` gains
  `set-assignee: true | false | string[]`, default `true`. `true` = stamp the
  current git `user.email`; `false` = leave unassigned; `string[]` = stamp the
  given emails (assign to others/multiple at creation). Applies only when
  `ticket-assignee-aware` is on (subordinate to the project flag). Empty current
  email (CI/bot) ⇒ no stamp ⇒ assign-any. Creating with `set-assignee` is the
  explicit ownership act.
- **Deterministic comparison in `tickets.query` (not model judgment):** the
  server computes current-identity-vs-assignee once and emits an explicit warning
  token in both JSON and compact-text projections, e.g.
  `assignee: bob@example.com   # NOT ASSIGNED TO YOU`. Rationale: the selector
  runs at a small tier; leaving an ownership skip to prose judgment repeats the
  260913 failure (a haiku selector ignored the `## Blocked` prose rule).
- **Selector: hard-skip via a server-side filter.** The selector calls
  `tickets.query` with an assignee filter so others'-assigned tickets are omitted
  from its result entirely (it never sees them) — stronger than a token it must
  honor. assign-any tickets are always included.
- **Visibility split:** outside the selector, a plain `tickets.query` still
  returns others'-assigned tickets WITH the warning token, so design review and
  other contexts continue to consider them. Only the selector applies the
  omit-filter.
- **Router / lead-run warn:** attempting to start someone else's assigned ticket
  triggers a loud warning (consuming the same comparison/token).
- **`not-assigned` override:** a per-invocation lead-run argument (transient — not
  stored in frontmatter or config) that makes the selector query without the
  assignee filter, for when the lead deliberately picks up another's ticket.
- **goal-run terminal:** in an autonomous drain, hard-skip filters the queue;
  when no autonomously-advanceable ticket remains (only others'-assigned left),
  goal-run reaches a terminal reported as "no autonomously-advanceable ticket
  remains" (the existing every-remaining-blocked family) rather than looping.

## Constraints

- Shipped-surface mirror: any `ticket-selector.md` / `lead-run.md` change is
  byte-identical across `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`,
  `agents-plugin-pi/rsrc/` (`wsflow-mirroring.md`; the pi mirror guard) plus the
  documented regen steps.
- MCP changes (`tickets.query` / `create`, the identity read) read
  `ai-docs/manuals/ws-mcp.md`; any skill/playbook text reads
  `ai-docs/manuals/skill-authoring.md`.
- Depends on `260917-feat-ws-committed-project-config-scope`; do not implement
  the flag storage here.

## Open Questions

Settle at grounding / `ready` promotion:

- selector filter param name/shape on `tickets.query`; exact `not-assigned`
  lead-run argument spelling.
- precise warning-token wording and placement in both projections.
- goal-run terminal message wording and composition with the existing
  every-remaining-blocked result.
- multi-assignee match semantics (assumed any-of: current email matches if it is
  in the set).

## Phases

Phasing deferred to grounding. Provisional slices, prerequisite (committed config
scope) first: (1) current-git-identity read + `assignee` parsing/surfacing with
the deterministic warning token in `tickets.query`; (2) `create` auto-fill
(`set-assignee` union); (3) selector omit-filter + `not-assigned` lead-run
override + goal-run terminal — each carrying the 3-way mirror.
