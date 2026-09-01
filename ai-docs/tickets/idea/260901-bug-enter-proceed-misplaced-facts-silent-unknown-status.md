---
title: enter.proceed silently drops misplaced facts and returns an undiagnosable status=unknown
related:
  260830-bug-proceed-continuation-replays-stale-done-target: adjacent proceed target-validation gap; that one is a stale target after compaction, this one is a caller facts-shape/diagnosability defect
---

# enter.proceed silently drops misplaced facts and returns an undiagnosable status=unknown

## Background

A downstream `wsflow` dogfood (v0.44.2) reported `/wsflow:lead-proceed` "fully
blocked": `enter.proceed` returned `Route: terminal-artifact.unknown-status`,
`NEXT: stop`, `Reason: status=unknown` for a ticket that physically existed in
`ai-docs/tickets/ready/`. The report concluded a "server-side derivation
failure" — that five conditions (`status`, `category`, `freshness`, `slice`,
`scope-blocked`) are runtime-derived and ignore caller facts.

That premise is inverted. `enter.proceed` (`proceed_resolver.go`) does **no**
filesystem I/O; it is a pure fact-router. Every one of those five conditions is
read **only** from caller-supplied grouped facts and defaults to `unknown` when
absent (`normalizeProceedFacts`, `proceed_resolver.go:365-464`). The MCP
inputSchema documents a nested shape — `facts.ticket.status`,
`facts.ticket.category`, `facts.ticket.freshness`, `facts.gates.scope_blocked`,
`facts.work.slice`, etc. (`server.go:3611-3645`). The downstream caller passed a
**flat** payload (`{"status":"ready","category":"feat",...}`), so those keys
matched no recognized path, were dropped, and each defaulted to `unknown`;
`status=unknown` on a ticket-path target short-circuits to
`terminal-artifact.unknown-status` (`proceed_resolver.go:474-481`).

The four conditions the report believed were "honored" (`has-ticket=yes`,
`ticket-missing=no`, `actionable=yes`, `migration-anchor=n/a`,
`discussion-needed=no`, `needs-ticket=n/a`) are all resolver
defaults/forced-normalizations for a ticket-path target, not the caller's flat
facts — which is what created the false "4 honored / 5 ignored" asymmetry that
misled the diagnosis.

Root cause is caller-side (wrong facts shape + off-playbook hand-invocation),
but the tool made this failure undiagnosable: it silently swallowed misplaced
top-level keys with no warning, and the terminal `Reason: status=unknown` gives
the caller no path back. Because the router verdict is authoritative and
`NEXT: stop` halts with no in-band recovery, the operator had no signal that the
facts payload — not a server bug — was the problem.

This ticket is diagnosability hardening for the router, not a behavioral change
to routing itself. The correct route for a well-formed call is unaffected.

## Phases

### Phase 1: Make misplaced/absent facts self-diagnosing at the router boundary

Goal: when a ticket-path target resolves to `terminal-artifact.unknown-status`
because `facts.ticket.status` was absent, the output must tell the caller *why*
and *what to do*, and unrecognized/misplaced fact keys must not vanish silently.

Suggested (settle exact shape during implementation):

- Enrich the `status=unknown` reason for a ticket-path target with an
  actionable hint, e.g. "no facts.ticket.status supplied — build route context
  (tickets.status) and pass grouped facts (facts.ticket.status)", instead of the
  bare `status=unknown`.
- Emit a warning when the raw `facts` object carries recognized field names at
  the wrong nesting level (e.g. a top-level `status`/`category`/`slice`, or
  ticket-scoped keys placed directly under `facts`), mirroring the existing
  `warnFactIfMeaningful` pattern already used for inline/ticket-missing cases
  (`proceed_resolver.go:392-394, 416`). Decide whether unknown top-level keys
  warrant a generic "unrecognized facts key" warning too.

Constraints:

- Do not add filesystem I/O to `enter.proceed`; it stays a pure fact-router. The
  status derivation contract (caller builds route context via `tickets.status`
  and passes `facts.ticket.status`) is intentional and unchanged.
- Route selection must not change for well-formed input; this is warnings +
  reason-string enrichment only.
- The strict-JSON decode path may already reject some unknown keys — verify
  whether misplaced keys currently error, warn, or are silently ignored, and
  target only the silent-ignore gap.

Open question for implementation: whether the same enrichment should extend to
the `fallback.insufficient-route-facts` route (repro #2 in the source report),
where a target with no `kind` plus flat facts yields all-unknown.

## Spec Impact

Likely touches the proceed-routing behavior contract (proceed route/verdict and
its warnings surface). Exact spec area to be confirmed at promotion; `idea/`
landing does not require spec addressing.
