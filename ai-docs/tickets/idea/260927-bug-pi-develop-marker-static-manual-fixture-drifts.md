---
title: "Keep the opt-in Pi develop-marker manual check valid under current tuning"
---

# Keep the opt-in Pi develop-marker manual check valid under current tuning

## Background

While verifying the Pi mailbox source-build hotfix, `WS_PI_VERIFY_DEVELOP_MARKER=1 npm test -- --test-reporter=dot test/develop-marker.integration.test.ts` failed on its pre-existing exact comparison of `playbook.read("lead-workflow-manual")` to the committed static fixture. The live manual selects `gpt-6-luna` under current model tuning whereas the fixture expects `gpt-5.4-mini` and `gpt-5.5`, and the live manual includes a later worktree ownership paragraph missing from the fixture. A newly isolated source-build mailbox smoke in the same file passes independently. This is a test-fixture drift, not evidence that the mailbox runtime launch failed.

## Phases

### Phase 1: Make opt-in live verification robust to manual changes

Determine whether the static fixture should be regenerated from current shipped defaults or whether this opt-in live probe should validate only stable structural properties while honoring the effective tuning scope. Ensure the test continues to detect a broken source-build/manual path without failing merely because intentional manual text or model mappings changed.
