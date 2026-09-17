---
title: Ticket assignee awareness (opt-in, deterministic ownership gate)
related:
  260917-feat-ws-committed-project-config-scope: prerequisite (home of the ticket-assignee-aware flag)
  260913-bug-ticket-selector-chooses-recorded-blocker: same small-tier selector surface; motivates deterministic (not prose) gating
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b2be348c6ff09a19
sage-review-completeness-reviewed: b2be348c6ff09a19
completed: 2026-09-17
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Open Questions

Settle at grounding / `ready` promotion:

- selector filter param name/shape on `tickets.query`; exact `not-assigned`
  lead-run argument spelling.
- precise warning-token wording and placement in both projections.
- goal-run terminal message wording and composition with the existing
  every-remaining-blocked result.
- multi-assignee match semantics (assumed any-of: current email matches if it is
  in the set).
- Whether `ticket-assignee-aware` must be non-overridable by the machine
  `project` scope so a contributor cannot silently disable the shared gate
  locally — coupled to the overridability open question in
  `260917-feat-ws-committed-project-config-scope`; settle the two together.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/frontmatter.go, agents-plugin-tool/internal/wsdoc/tickets.go, agents-plugin-tool/internal/mcp/server.go (tickets.query, tickets.create_empty), and the 3-way mirrored agents-plugin/rsrc/ticket-selector/ticket-selector.md and agents-plugin/rsrc/lead-run/lead-run.md (agents-plugin-wsflow/, agents-plugin-pi/) |
| scope.surface | public-interface | tickets.query and tickets.create_empty are MCP tool schemas read by external callers, evidence agents-plugin-tool/internal/mcp/server.go#L3774, #L3852-3862 |
| scope.new_public_symbol | yes | new assignee frontmatter key, ticket-assignee-aware project flag, set-assignee tool param, not-assigned lead-run argument, and a new warning token in tickets.query output |
| scope.new_type_contract | yes | new assignee []string frontmatter field and a new computed identity-comparison field on tickets.query's JSON and compact-text projections |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tickets_scope_test.go, ticket_dispatch_gate_test.go, and server_test.go cover tickets.query; playbook_tools_test.go and session_state_test.go cover tickets.create_empty |
| complexity.reuse_points | confirmed | existing related: dual-shape (map[string]string or []string) frontmatter precedent at agents-plugin-tool/internal/wsdoc/tickets.go#L697-738 (relatedEntries), and the existing computed-field pattern for dispatch_blocked at agents-plugin-tool/internal/wsdoc/tickets_deps.go#L195-217 |
| complexity.side_effect_risk | moderate | changes the candidate set the selector returns from the ready queue and introduces a new external git-identity read with no existing primitive |
| risk.correctness | moderate | new deterministic case-insensitive identity comparison, multi-assignee any-of semantics, and a 3-way mirrored playbook edit must all agree |
| risk.fit | moderate | hard-depends on 260917-feat-ws-committed-project-config-scope, itself unimplemented idea-status work whose scope name, precedence, and mutability are still open |
| risk.test | moderate | needs new selector-filter, warning-token, and goal-run-terminal coverage in addition to the existing 3-way mirror drift-guard suite |
| risk.security_or_contract | moderate | changes the tickets.query and tickets.create_empty tool contracts (new response field, new param) and the selector's default candidate set |

## Phases

Phasing deferred to grounding. Provisional slices, prerequisite (committed config
scope) first: (1) current-git-identity read + `assignee` parsing/surfacing with
the deterministic warning token in `tickets.query`; (2) `create` auto-fill
(`set-assignee` union); (3) selector omit-filter + `not-assigned` lead-run
override + goal-run terminal — each carrying the 3-way mirror. Grounding sets the
stable `### Phase N:` cut lines; each phase's verification expectation is new MCP
tests for the identity comparison / warning token (slice 1), `set-assignee`
auto-fill (slice 2), and selector omit-filter + goal-run terminal behavior
(slice 3), alongside the existing 3-way mirror drift guard.

### Result (07a24ed5) - 2026-09-17

Executed as one unit (no `### Phase N:` cut lines were introduced; the three
provisional slices landed as three commits, matching the committed-config-scope
prerequisite's Result-without-phase-headers precedent).

Delivered, opt-in and off by default via committed repo config
`ticket-assignee-aware` (resolved session > project > repo > global > builtin;
builtin default `off`):

- Slice 1 — identity + surfacing (commit 1685842b). `wsgit.CurrentUserEmail`
  reads git `user.email` (original case for display; any error folds to `""` so
  the gate degrades to assign-any). `wsdoc` parses frontmatter `assignee:`
  (scalar or list) and computes `AssigneeGate` (case-insensitive any-of match;
  empty/absent assignee = assign-any). `tickets.query` surfaces the deterministic
  warning token `# NOT ASSIGNED TO YOU` in both the compact and JSON projections
  for point-resolve and discovery listings, only when the flag is on.
- Slice 2 — create auto-fill (commit 1685842b). `tickets.create_empty` gains
  `set_assignee` (bool | string[], default true): true stamps the current git
  identity (nothing if identity is empty), false omits, list stamps verbatim.
  Inert when the flag is off.
- Slice 3 — selector + lead-run steering (commit a107947b). `tickets.query`
  `assigned_to_me` server-side omit-filter; the selector queries with it (except
  a `not-assigned` run) and, when the filter empties an otherwise-non-empty
  ready set, returns `every remaining ticket blocked` rather than the merge-on-
  goal-branch `ready/ empty`. `lead-run` passes `not-assigned` through and
  surfaces an ownership warning + confirmation before dispatching a directly-
  named others'-assigned ticket. All three carry the byte-identical 3-way mirror
  (`agents-plugin{,-wsflow,-pi}`).

Tests (commits 1685842b, a107947b, and review-fix 07a24ed5): `wsdoc`
gate/parse/omit-filter and create-stamp tables; `wsgit.CurrentUserEmail` trim +
error-fold; `mcp` warning-token (both projections), assign-any/self no-warning,
flag-off inertness, `assigned_to_me` omission with visibility split, create
stamping, pure `resolveSetAssignee` table (incl. empty-identity), and an
end-to-end cleared-identity degrade case. Full `wsdoc`/`wsgit`/`mcp` suites pass
(`WS_MAILBOX= WS_MAILBOX_AUTO=` for the documented ambient-mailbox flake).
Partitioned review: round 1 raised no Critical/Major, one test-partition
Important (empty-identity untested) fixed in 07a24ed5; round-2 verification
PASS, nothing new.
