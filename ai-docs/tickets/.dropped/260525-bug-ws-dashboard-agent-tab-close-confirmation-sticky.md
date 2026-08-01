---
title: ws dashboard agent tab close confirmation stays open
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-markdown-renderer-polish: repeated browser gate blocker while verifying Markdown viewer polish
spec:
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
---

# ws dashboard agent tab close confirmation stays open

## Background

While verifying Markdown viewer polish on 2026-05-25, the full dashboard
Playwright acceptance gate repeatedly failed before reaching the Markdown step
at `agent tab close confirmation when a live agent tab exists`.

The failing assertion was:

```text
e2e/dashboard-acceptance.spec.ts:1625
Expected agent tab count 0, received 1
locator('.dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]').filter({ hasText: 'Agent' })
```

One earlier run of the same implementation branch passed the full gate, but
two later runs on merged `ws-dashboard-dev` failed at this same agent-tab close
confirmation step. This suggests a flaky or regressed confirmation/close path
outside the Markdown renderer scope.

## Investigation Notes

- Confirm whether the `workbench.tab.close.confirm` command is dispatched.
- Check whether the confirmation target keeps the correct visible pane id and
  captured workRoot id through the confirm click.
- Compare agent tab behavior with terminal close confirmation, which has a
  similar browser acceptance path later in the same gate.
- Decide whether the acceptance test needs a stronger post-confirm wait target
  or whether the UI command path is genuinely failing to detach the live agent
  surface.
