---
title: ws dashboard terminal focus browser gate regression
related:
  260517-bug-ws-dashboard-editor-scroll-ime-verification: previous terminal focus retention hotfix
  260517-feat-ws-dashboard-workroot-activity: dogfood run where Phase 2 browser evidence was blocked by this baseline failure
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard terminal focus browser gate regression

## Background

During `260517-feat-ws-dashboard-workroot-activity` Phase 2 dogfood, the
dashboard browser gate passed the new WorkRoot Activity badge assertions but
failed later in the existing terminal input focus step. The focused terminal
helper textarea became `BODY` after typed input.

The same failure reproduced on the pre-Phase-2 baseline commit `0bb994e`, so it
is not caused by the activity badge UI. It still blocks UI-facing dashboard
verification because `npm run test:browser` cannot pass while this regression is
present.

Investigate why the earlier terminal focus retention behavior regressed or
remains flaky in the daemon-served Playwright gate, then restore stable
`.xterm-helper-textarea` focus across ordinary input/output turns.
