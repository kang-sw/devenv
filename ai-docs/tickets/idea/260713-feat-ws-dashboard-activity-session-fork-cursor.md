---
title: "ActivitySessionForkRequest needs a cursor/turn-cut-point field"
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: prerequisite
  260711-feat-ws-dashboard-agent-activity-chat-ui: prerequisite
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: related
related-mental-model:
  - ws-dashboard-agent-harness
---

# ActivitySessionForkRequest needs a cursor/turn-cut-point field

## Background

`ActivitySessionForkRequest` (`ws-dashboard/frontend/src/activitySessionApi.ts`,
authored by `260620` Phase 1, commit `852cd0ad`) has no cursor/turn-cut-point
field — as drafted it can only fork an entire session, not fork from a
specific bubble/point in the transcript. `260711`'s per-bubble "fork from
here" affordance (Phase 3) needs exactly this: forking a new session that
starts from a specific transcript cut point, not the whole session.

Discovered during `260711` Phase 3 research planning
(`ai-docs/.plans/2026-07/13-1150-chat-ui-resume-fork-phase3.md`). Since
`260620`'s real `activity.session.fork` wire route does not exist yet (only
a local type-only draft), `260711` Phase 3 worked around this by giving its
local stub `stubForkActivitySession` a second, non-wire parameter carrying
the transcript cut point — this is a legitimate stub-only accommodation, not
a violation of any real wire contract, since no real fetch/route exists yet
to violate. But the shared `ActivitySessionForkRequest` type itself was
deliberately left unedited, per `260620`'s ownership of that contract.

## Phases

### Phase 1: Add a cursor/turn-cut-point field to the real fork contract

When `260620`'s real `activity.session.fork` route is implemented (or if an
earlier phase of `260620` revisits the draft type before that), add a
cursor/turn-cut-point field to `ActivitySessionForkRequest` (and the
corresponding daemon-side fork handler) so a caller can request a fork from
an arbitrary transcript point, not only the session's current end. Use
`260711` Phase 3's stub-side `stubForkActivitySession` cut-point parameter
as the reference shape for what the real field needs to express. When this
lands, `260711`'s stub-only workaround parameter should be reconciled with
the real field (either by adopting the same shape or by threading the
stub's local parameter into the now-real request type).

**Progress note (2026-07-13)**: `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`
Phase 1 (commit `53d420fe`) added the frontend-type half of this field —
`ActivitySessionForkRequest`/`ActivitySessionForkResponse` now carry an
optional `cutCursor`, adopting the stub's `cutBlocks` shape as a single
wire-shaped cursor value. The daemon-side fork handler this phase also
requires does not exist yet; it is scoped into that same ticket's Phase 3
(`CodexControlRequest::Fork` variant and route). This ticket stays open in
`idea/` until Phase 3 lands and the daemon side can be verified end-to-end.
