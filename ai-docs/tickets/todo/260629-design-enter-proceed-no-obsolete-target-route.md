---
title: enter.proceed has no route for a target already resolved/obsolete
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260627-feat-enter-proceed-deterministic-verdict-engine: deterministic route engine this surfaced against
sage-review: required
---

# enter.proceed has no route for a target already resolved/obsolete

## Surprise

While dogfooding the new `ws.enter.proceed` verdict engine, the target was an
`idea` ticket whose described bug had already been fixed by a separate update
(work shipped; only a doc-sync residual remained). The honest real next action
was "close the ticket as resolved." The engine has no verdict for that:

- I passed `actionable=no` (no concrete change remains). The resolver
  hard-overrode it to `actionable=yes` with the warning
  `actionable normalized to yes for ticket-path target`, per the rule "ticket
  paths are always actionable."
- With `status=idea` + `freshness=missing-settled-decisions`, it routed to
  `ticket-readiness.status-refresh` -> `NEXT: lead-write-ticket`.

So every existing idea ticket routes toward promotion/refresh regardless of
whether it is still relevant. There is no `NEXT` such as `close-ticket` /
`drop-ticket`, and no route vocabulary for "target overtaken by events."

## Why this is partly by-design

`lead-proceed` assumes implementation intent and is forbidden from inspecting
source, so it cannot generally know a target is "already done" — that often
requires reading code/tests. Routing an idea ticket toward `lead-write-ticket`
(where the lead can then decide to close it) is defensible. The
`actionable=no -> yes` override for ticket-path targets is also documented
behavior, not a bug, and it emitted a non-blocking warning.

## Follow-up (design/research, low priority)

Decide whether the proceed boundary should model an "obsolete/resolved target"
outcome, or whether closing stale tickets is intentionally out of proceed's
scope (handled by the lead dropping out of the flow). Options:

- Add a `freshness=obsolete` (or a `scope_blocked`/route value) that yields
  `NEXT: stop` with a "target appears resolved; consider closing" reason, when
  the lead can defensibly assert the described work already shipped.
- Or document explicitly that proceed never closes tickets, and that a
  lead who finds a resolved target should exit proceed and call ticket close
  directly.

No behavior change proposed yet; this records the live friction and the two
candidate directions.

## Decision (260629 sweep)

Fix: Add lead-side phase-completion check to lead-proceed scope resolution. When the lead reads the ticket and all Phase sections already have a `### Result` block, set `scope_blocked=no-unfinished-phase` rather than routing to promotion. This is a prose-only addition to lead-proceed's scope resolution rules — no new Go MCP tool or markdown parser needed. The existing `no-unfinished-phase` verdict already routes to a clean stop.
