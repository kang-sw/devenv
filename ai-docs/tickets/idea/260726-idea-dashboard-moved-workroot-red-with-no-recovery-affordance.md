---
title: "A work-root whose directory was deleted goes red with no explanation and no
  usable recovery action; `moved` is also mis-toned as error instead of the muted
  tone its own classifier intends"
related:
  260721-idea-dashboard-worktree-label-alias-split: touches the same work-root
    identity/registration surface this ticket wants a recovery affordance on
---

## Symptom

Found during dogfooding (2026-07-26). The top-left work-root/workspace selector
label for workspace `InspectTGV_AIDriven` renders with a **red background**,
while the user is working normally in that workspace with healthy terminals and
a local, fully accessible primary root. Nothing in the UI says what is wrong or
what to do about it.

Live evidence from the running daemon (`GET /api/dashboard/resources`):

```text
WS: InspectTGV_AIDriven | status=degraded stale=True err=one or more workRoots need refresh
    - InspectTGV_AIDriven  kind=gitPrimaryRoot     avail=available ready
    - jpeg                 kind=gitLinkedWorktree  avail=available ready
    - strata               kind=gitLinkedWorktree  avail=available ready
    - InspectTGV_wt_jpeg   kind=plainDirectory     avail=moved  err=moved   <- cause
    - InspectTGV_wt_trackA kind=plainDirectory     avail=moved  err=moved   <- cause
```

All other workspaces (`nDDC`, `PipelineDevProj`, `libhbs`) are `ready`. The two
offending entries are **manually registered plain-directory work roots whose
directories the user deleted by hand**; the registration outlived the directory.
So the underlying state is legitimate and correctly detected — the problem is
purely how it is surfaced and how (not) it can be resolved.

## Two distinct problems

### 1. `moved` is rendered red, contradicting the classifier's own intent

`frontend/src/resourcePresentation.tsx:117-123` checks `state.error` **before**
the branch that is supposed to handle `moved`:

```ts
if (state.error || availability === "inaccessible" || availability === "missing") {
  return "error";   // -> .resource-row-error, red background
}
if (state.stale || availability === "moved" || activation === "offline") {
  return "muted";   // <- the intended tone for `moved`, unreachable in practice
}
```

But the backend sets *both* fields for a moved root
(`crates/daemon/src/discovery.rs:331-337`): `availability: Moved` **and**
`error: Some("moved")`. The first branch therefore always wins, so the
`availability === "moved"` clause in the second branch is dead code. The
classifier declares `moved` a warning-level state and then never renders it as
one. Either the backend should not set `error` for `Moved`, or the frontend
should order/scope the checks so `moved` reaches its muted branch — decide which
side owns the invariant.

Note this also means the red carries no severity information: a deleted
scratch directory looks exactly as alarming as a genuinely inaccessible or
permission-denied root.

### 2. Red is the entire user-facing response — no explanation, no working recovery

The only actions offered on the moved root are:

```json
"actions": [ { "id": "reconnect", "label": "Reconnect", "enabled": false },
             { "id": "workRoot.activation.offline", "label": "Go offline", "enabled": true } ]
```

`reconnect` — the one action that names the problem — is **disabled**. The user
is left with a red label, no message, and no offered path forward except
manually hunting down which child root is stale. The workspace-level rollup text
(`"one or more workRoots need refresh"`) exists in the payload but is not
surfaced anywhere the user naturally looks; and "needs refresh" misdescribes the
situation, since refreshing cannot fix a directory that no longer exists.

For context on how detection works today: `state.error` is set **only** by
filesystem probes in `discovery.rs:320-372` (`fs::metadata` / `fs::read_dir`) —
`NotFound` with a surviving parent yields `Moved`, without yields `Missing`,
plus permission/read variants yielding `Inaccessible`. Git failures never set it
(`GitDiscovery::discover` returns `None` on failure and the root degrades to a
plain directory with `error: None`). So the signal is reliable and cheap; it is
only the presentation and remediation that are missing.

## Directions to explore (nothing decided here)

Framed as "tell the user, then offer a safe fix" — auto-remediation must stay
conservative because a `Moved` root can also mean a temporarily unmounted volume
or a renamed-but-wanted directory, not only a deliberate deletion.

- **Explain, at minimum.** Surface *which* child root is bad and *why* on the
  red/muted affordance itself (tooltip, inline row detail, or a workspace-level
  notice), instead of only tinting the parent label. Replace the
  "need refresh" rollup wording with something accurate per availability class.
- **Distinguish the classes visually.** `missing`/`inaccessible`/`permission
  denied` are actionable-now problems; `moved` on a plain-directory registration
  is usually just stale bookkeeping. They should not share one red.
- **Offer an explicit, reversible cleanup.** For a `plainDirectory` root whose
  path is gone, an "Unregister this work root" (or "Forget") action is the safe
  remediation, and is meaningfully different from the disabled `reconnect`.
  Consider also "Relocate…" for the genuine rename case.
- **Consider bounded auto-resolution, opt-in.** Candidates worth evaluating:
  auto-`git worktree prune`-equivalent bookkeeping for registrations the daemon
  itself derived, or auto-demoting a long-`Moved` root to `offline` after N
  consecutive probes so it stops poisoning the workspace rollup. Anything that
  *removes* user-registered state should require confirmation; anything that
  merely stops it from alarming can be automatic.
- **Fix or remove the disabled `reconnect`.** A permanently disabled action that
  names the exact user goal is worse than no action; either enable it with real
  re-probe semantics or replace it with the cleanup action above.

## Reporter Context

Observed on the Windows dogfood daemon (pid serving `127.0.0.1:4300`, local dist)
while working in `InspectTGV_AIDriven`. Initially misdiagnosed as a WSL/UNC
availability problem, then as a git-command failure; tracing
`resourceRowTone` -> `discovery.rs` established that only filesystem probes
drive the flag, and querying the live resources endpoint identified the two
stale plain-directory registrations as the actual cause. The user confirmed
having deleted those directories manually.
