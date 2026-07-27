---
title: notification toggle stays enabled with no reconciliation when the Notification API is absent
related:
  260726-chore-dashboard-verify-notification-permission-tier-manually: surfaced by the correctness review of Phase 2
---

# notification toggle stays enabled with no reconciliation when the Notification API is absent

## Background

`NotificationSection` in `ws-dashboard/frontend/src/settingsSections.tsx` binds
the checkbox's `disabled` to `insecureContext` alone
(`disabled={insecureContext}`, line 176, with `insecureContext =
!window.isSecureContext` at line 162). That covers exactly one of the two ways
`notificationAvailability` can report the tier as unusable.

The uncovered state is **secure context, `Notification` global absent** — e.g.
iOS Safari before 16.4 over HTTPS, or an embedded webview without the API. In
that state:

- `notificationAvailability(true, false, ...)` (lines 129-131) correctly
  returns `"unavailable in this browser"`, so the note text is honest.
- The checkbox itself stays enabled, because `insecureContext` is `false`
  here. A click persists `{ enabled: true }` through `onChange(next)` (line
  179), which writes to `localStorage` via `notificationPrefs.ts`
  (`notificationPrefsStorageKey = "ws-dashboard.settings.notifications.v1"`).
- The `onChange` handler's own guard, `if (next && typeof Notification !==
  "undefined")` (line 180), is `false` in this browser, so
  `requestPermission()` is never called and the `.then` block that turns a
  denial back off (lines 189-198, with `onChange(false)` at 196) never runs.
  Nothing reconciles the preference back to `false`.

## Why it matters

`ai-docs/spec/ws-web-dashboard/index.md` lines 1007-1008 state the standard
this section is held to: "an enabled preference guarding a tier the browser
will never allow is a control that lies about its own effect." That sentence
is written about the denied-permission case, and the denied case is handled —
the `requestPermission()` resolution turns the preference back off. The
no-global case reaches the identical end state (a preference that can never
fire) by a path the existing reconciliation does not cover, because
reconciliation only runs after a call that this browser can't make.

## First Step

The one-line candidate is `disabled={insecureContext ||
!hasNotificationGlobal}` (or equivalent), which requires lifting the
`typeof Notification !== "undefined"` read (currently only inside
`currentNotificationAvailability`, line 136, and the `onChange` guard, line
180) up next to the `insecureContext` read in the component body so both
gate the same `disabled` expression.

That one line drags in two more:

- A spec sentence: the existing paragraph around
  `ai-docs/spec/ws-web-dashboard/index.md:1003` only describes the disable
  condition as `window.isSecureContext`; widening the disable to also cover
  the no-global case needs its own sentence, not a silent behavior change.
- A unit assertion on the CONTROL, not the message.
  `settingsSections.test.ts` already pins all four
  `notificationAvailability` states (lines 140, 146, 152, 158), including
  `notificationAvailability(true, false, "default")` at line 152 — that
  covers the message text for this exact state already. What's missing is an
  assertion that the checkbox's `disabled` prop is `true` in that state; the
  message-text gap and the control-state gap are different assertions and
  only the second is open.

Also open, and deliberately not settled here: is disabling the checkbox the
right answer for "browser has no Notification API at all," or should the
section render no checkbox in that state? The insecure-context case has a
standing reason to keep the control visible-but-disabled — the origin can
change (e.g. the user switches from a plain-http LAN address to a TLS one)
without a page reload of the settings modal being guaranteed. A browser that
lacks the `Notification` global entirely has no equivalent "this could still
become true" story within a session; hiding the control instead of graying it
out may be the more honest answer, but that is a UI-shape decision, not
implied by the reconciliation fix.

## Notes

- Filed as `idea/`, not folded into a phase of
  `260726-chore-dashboard-verify-notification-permission-tier-manually`,
  because the phase that produced the `disabled={insecureContext}` binding
  recorded a decision scoped to insecure origins only. Widening that scope is
  caller-visible (it changes what real no-global browsers show and persist)
  and needs its own spec sentence and its own assertion — that is more than a
  review-minor fix folded into a phase whose recorded decision didn't cover
  it.
- Not a defect in Phase 2's own scope: the phase's `disabled` binding does
  exactly what its plan text asked for (gate the insecure-context case). The
  gap is in what the plan text left out, not in what it implemented.
