---
title: ws dashboard terminal focus browser gate regression
related:
  260517-bug-ws-dashboard-editor-scroll-ime-verification: previous terminal focus retention hotfix
  260517-feat-ws-dashboard-workroot-activity: dogfood run where Phase 2 browser evidence was blocked by this baseline failure
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-24
---

# ws dashboard terminal focus browser gate regression

## Background

During `260517-feat-ws-dashboard-workroot-activity` Phase 2 dogfood, the
dashboard browser gate passed the new WorkRoot Activity badge assertions but
failed later in the existing terminal input focus step. The focused terminal
helper textarea became `BODY` after typed input.

The same failure reproduced on the pre-Phase-2 baseline commit `0bb994e`, so it
was not caused by the activity badge UI. A narrow watchdog hotfix was applied on
the WorkRoot Activity branch to restore `.xterm-helper-textarea` focus after
ordinary terminal input/output churn and unblock the Phase 2 browser gate.

This ticket remains as follow-up because the hotfix was made under a gate
pressure path rather than a dedicated terminal-focus slice. Stabilize the
terminal focus model and its Playwright coverage so the watchdog cannot steal
focus after intentional outside focus movement, and so future terminal changes
do not rely on an incidental acceptance-gate fix.

## Closeout

Closed after current-main verification that
`481a404e fix(ws-dashboard): stabilize terminal focus watchdog` is an ancestor
of `HEAD`. Reopen as a new focused bug if the terminal browser gate regresses
again.
