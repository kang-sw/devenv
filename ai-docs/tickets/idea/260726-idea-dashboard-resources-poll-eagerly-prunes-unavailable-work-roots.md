---
title: "`/api/dashboard/resources` eagerly unregisters an unavailable, no-active-child
  work root on every poll, which a caller expecting a stable 409 does not expect"
related:
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: Phase 2 implementation
    of that ticket's plan hit this while writing an e2e test for the moved-root
    409 path through the git toolbar
---

## Symptom

Found while dogfooding the Phase 2 implementation of
`260726-refactor-ws-dashboard-git-fs-watch-invalidation` (2026-07-26), authoring
an e2e test that removes an opened git work root's directory on disk and expects
the daemon's live 409 `"workRoot unavailable"` to surface through
`GET /git/status`.

The first version of the test forced a UI remount by switching the active
workbench selection away from and back to the moved root. Switching triggers a
client-side `fetch("/api/dashboard/resources")` (via
`resourceIdsForWorkRootLabel` in the frontend). That single fetch turned the
expected 409 into a 404: `dashboard_resources_with_registry_sync` on the server
side does more than report state — it **prunes/unregisters** a workspace's work
root from `opened_work_roots` entirely once the root becomes unavailable and the
workspace has no other active child root. After that prune, the git-toolbar
route's `resolve_online_available_work_root` gate correctly reports `Unknown`
(404) because the id is genuinely gone from the registry, not because the
directory is merely inaccessible.

Nothing about `/api/dashboard/resources` being a read/reporting endpoint
suggests to a caller that it also has this side effect. The dashboard's own 5s
polling loop hits this same route continuously, so the same prune happens
automatically and quickly (well under 5s of walltime) whenever an opened root
goes unavailable with no active sibling — not only through the resources poll's
documented staleness window.

## Why it matters

- **Surprising for API consumers.** A route named `resources` (a query/report
  shape) silently mutating persisted registry state (`opened_work_roots`) is a
  side effect callers reasonably do not expect from what reads like a GET-only
  status query.
- **Timing-sensitive tests must route around it.** The Phase 2 e2e test had to
  `page.route("**/api/dashboard/resources", (route) => route.abort())` for its
  duration and trigger a different, independent refresh path (the git toolbar's
  own `focus`-triggered scheduler) to observe the 409 before the prune fires.
  Any other UI action that happens to call the resources endpoint (a poll tick,
  a workspace switch, a manual refresh) will race this prune and can silently
  flip a `409 Unavailable` into a `404 Unknown` for the same underlying
  condition, from the client's point of view.
- **Overlaps, but is distinct from,**
  `260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance`: that
  ticket is about how a `Moved` root's *state* is presented once it exists in
  the registry; this one is about the registry entry itself disappearing out
  from under a still-open UI reference, which is a different, faster-acting
  hazard (the reference the UI/tests hold can become `Unknown` instead of
  merely `Unavailable`).

## Contract consequence (added by the Phase 2 lead, cycle-1 review)

The Phase 2 correctness review established that this prune is not only a testing
nuisance — it makes the parent ticket's accepted Phase 2 delta ("an unavailable
git work root returns 409 instead of 404") untrue in the steady state. Moving the
git routes off `live_dashboard_resources` removed the *route's own* prune; the one
that actually masks the answer lives here and still fires on the frontend's 5 s
poll. So the observable sequence is 409 for less than one poll interval, then 404
thereafter.

The `ws-web-dashboard` spec now records both answers under
`{#260726-dashboard-shared-git-probe-memo-and-per-root-git-context}` and carries an
`Implementation Gap · 2026-07-26` callout pointing at this behavior, on the
grounds that discarding a work root the user explicitly opened — on the strength
of one failed availability read, with no affordance to recover it other than
re-opening it by path — is a defect rather than a contract. Whatever this ticket
decides, that callout and the spec paragraph are the two places to close out.

## Directions to explore (nothing decided here)

- Confirm whether this pruning is intentional self-healing (verified elsewhere
  in the plan's Non-Goals / Verification Boundary sections?) or an
  under-documented side effect; if intentional, document it explicitly next to
  `dashboard_resources_with_registry_sync` and in the route's own doc comment,
  since right now nothing at the call site signals "this mutates the registry."
- Consider whether pruning belongs on a read route at all, versus a dedicated
  maintenance path (e.g. a background sweep, or an explicit "forget" action) so
  a plain status GET stays side-effect-free for callers (including future
  automated tests) that assume it.
- If pruning stays on this route, consider whether the 404-vs-409 flip it causes
  for a root the user/UI still holds a reference to is itself worth smoothing
  over (e.g. distinguishing "never existed" from "existed and was just pruned"
  in the error payload).

## Reporter Context

Discovered and worked around during implementation of Phase 2 of
`260726-refactor-ws-dashboard-git-fs-watch-invalidation`
(`ai-docs/.plans/2026-07/26-1717-per-root-git-context.md`) on branch
`impl/per-root-git-context`, while authoring
`frontend/e2e/dashboard-acceptance.spec.ts`'s "moved git workRoot surfaces the
daemon's live 409 through the git toolbar error chip" step. The workaround
(route-blocking `/api/dashboard/resources` and using the toolbar's own
`focus`-triggered refresh) is committed alongside that step; this ticket exists
so the underlying server-side eager-prune behavior gets a decision instead of
staying only as a code comment.
