---
title: "Hosted Pi EXEC cannot follow run-worker routing without a private CLI bridge"
related:
  261114-bug-design-review-requires-unavailable-researcher-spawn: "related missing-delegation capability, but reviewer-depth admission differs from this peer-lead incident"
  261115-bug-ws-pi-bootstrap-elides-undelivered-manual: "inspect native-versus-fallback capabilities without assuming the same cause"
---

# Hosted Pi EXEC cannot follow run-worker routing without a private CLI bridge

## Background

Dogfood finding #10, 2026-11-16. A hosted Pi EXEC (a peer lead, not a nested
reviewer) had no native spawn tool for run-worker routing. It built a private
attach-only bridge script so its CLI worker could access ws primitives. The
advertised workflow therefore depended on a capability absent from that host.

This is owner-reported evidence. The EXEC identity, resolved profile/tool list,
rendered route, native-extension/fallback status and private bridge transcript
were not supplied. Do not assume a recursion ceiling, payload failure or
permission policy caused the omission. The existing reviewer-spawn idea covers
a different role/depth incident and is related rather than duplicated here.

## Reproduction

In an isolated hosted Pi peer-lead session, record its resolved profile and
native tool inventory, render the normal run-worker route, and compare required
spawn/send capabilities with what the EXEC can actually call. Compare the
native-extension and fallback lanes and a root lead. Preserve the private
bridge as incident evidence if available; do not deploy it as the product fix.

## Candidate fix directions

- Correct capability/profile/resource admission if the peer lead should already
  have the native delegation surface.
- Make routing capability-aware, with a supported bounded worker bridge only if
  that lane is deliberately approved.
- Refuse unsupported routing explicitly instead of requiring an improvised
  script or an unauthorized capability escalation.

## Decisions

Compatibility target: **T0** for restoring promised capabilities or additive
capability-aware handling while preserving existing MCP calls and permission
ceilings. The exact surface and tier remain provisional until the host/profile
trace is recovered. No new spawn permission, depth allowance or CLI bridge
policy is selected here.

## Open questions

- Which profile and native/fallback lane did this EXEC actually run?
- Should peer leads have native spawn, or should unsupported hosts stop before
  worker dispatch?
- If a CLI-worker lane is supported, who owns attachment, key delivery,
  permissions, cleanup and completion reporting?

## Phases

### Phase 1: Align hosted peer-lead worker routing with admitted capabilities

Recover the incident inventory, settle the supported route, and correct either
admission or routing without weakening authority/depth ceilings. Future isolated
verification covers root versus peer lead, native versus fallback extension,
missing spawn, rejected escalation, key binding and bounded worker completion.
No worker, bridge, test or executable probe is launched by this docs capture.
