---
title: Manually verify the OS notification permission tier, which the harness deliberately does not cover
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 8 shipped this tier; its verification boundary assigns this half to a human and it has not been performed
related-spec:
  - 260726-dashboard-browser-level-attention-cue
  - 260722-ws-dashboard-settings-panel
---

# Manually verify the OS notification permission tier, which the harness deliberately does not cover

## Background

`260725-feat-dashboard-pty-agent-attention-notification` Phase 8 shipped both
tiers of the browser-level attention cue. Its own verification boundary reads:
"the title/favicon tier asserted in browser acceptance; the permission tier
verified manually and recorded, since driving a real permission prompt in the
harness is not worth its cost."

The first half is done. The second is not — no human has driven a real
permission prompt against this build. This ticket exists so that gap is visible
rather than implied by a closed parent ticket.

This is not a suspected defect. Everything automatable about Tier 2 is verified:
`Notification.requestPermission()` has exactly one call site and it is the
Settings checkbox's own `onChange`; the secure-context guards distinguish "API
absent" from "permission denied"; the edge detector fires only on entry into
`ready` and holds no acknowledgement watermark; the constructor is wrapped
against browsers that expose the global but refuse to construct. What is
unobserved is the end-to-end claim that a real OS notification appears.

Playwright's `context.grantPermissions` was considered and rejected in the
parent ticket's plan: it would force a `granted` state without a native dialog,
proving the app calls the API correctly but not that the real permission flow
behaves. That is the half already covered by reasoning; automating it would add
cost without adding evidence.

## Steps

On `localhost` (a secure context — over plain-http LAN the whole API is absent
and there is nothing to grant):

1. Open Settings > Notifications. Confirm the copy states that OS-level
   notification requires a secure context, and that the live permission state
   is shown.
2. Check the toggle. Confirm a real browser permission prompt appears — it must
   appear on the click, not on page load.
3. Accept it. Confirm the section updates to the granted state without needing
   a reopen or an unrelated interaction.
4. Synthesize a turn boundary into `ready` (the callback-token POST the e2e
   spec uses works, or drive a real agent). Confirm a real OS notification
   appears.
5. Reload with an agent still pending. A notification is expected again — this
   is recorded behavior, not a defect; confirm it matches the description in
   the spec rather than surprising you.

Then, separately, on a plain-http LAN origin:

6. Open the same section. Confirm it reads unavailable/insecure-context rather
   than offering a control that cannot work.

Also worth a look while there, though not part of the boundary: whether the
denial path behaves as specified — checking the box and then denying should
leave the toggle off rather than on.

## Done when

The observations above are recorded somewhere durable — an `#### Edition` on
the parent ticket's Phase 8 Result is the natural home. If any step disagrees
with the spec, that is a bug ticket, not an edit to this one.
