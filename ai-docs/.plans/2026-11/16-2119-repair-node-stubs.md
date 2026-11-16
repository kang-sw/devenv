# Repair: Node-harness client tests red on CI (B1, ubuntu half)

Goal: CI run on develop b8e434d81 is red on ubuntu because about 25 Go tests that drive dashboard/whiteboard JS assets under Node fail. Their DOM stubs did not follow the panel-first, hub-first polling Phase 3, and `ago.js` refactors. Make every one of them pass against the real current assets, without weakening what they assert.

Failing tests (internal/wsdashboard and internal/wswhiteboard), with the errors seen:
- ReferenceError `shHidden`: TestBoardFrameBands, TestBoardTopicResizeKeepsTheView, TestBoardFramesMountLazily, TestBoardFrameHeightsBatch, TestBoardFramesMountPaced, TestBoardKeepsUnchangedEntryFrames (and likely TestHiddenFrameHoldsItsUpdatesUntilShown, TestPageReloadsOnOtherAssets, TestPageStreamsRideWebSocket, TestTabLabelCount, TestSearchPaletteRenderedEntries, TestBoardHistoryHooksSurroundSSESwap)
- ReferenceError `scrollRestore`: TestBoardKeepUnderNode
- ReferenceError `requestAnimationFrame`: TestToolbarFloatingGeometry
- ReferenceError `stopMainStream`: TestBoardTopicsClientHiddenAndRecovery, TestEntryRowsClientRecovery, TestPanelRowsClientRecovery, TestRailRowsClientRecovery, TestTreeRowsClientRecovery
- ReferenceError `drHeaderBottom`: TestDrawerFoldButtons
- TypeError reading `hasAttribute` of undefined `document.documentElement` (ago.js:92, :179-192): TestTopicStateBadgeResearchingMetadata, TestTopicStateBadgeClockAndReconciliation, TestBodyTimeAgeAcrossMinuteBoundaryUnderNode, TestBodyTimeUIBehaviorUnderNode

Approach: for each test, find how it assembles the JS (asset concatenation / slicing) and why a symbol defined in another asset file or a browser global is now missing. Prefer loading the real asset that defines the symbol, or a minimal browser-global shim in the harness (`requestAnimationFrame`, `document.documentElement` with `hasAttribute`, `document.addEventListener`), over editing production code. Change production JS only if a test exposes a real defect, and say so.

Also correct the misdiagnosis: `ai-docs/ref/261116-baseline-test-repair.md` records the two TestTopicStateBadge* reds as caused by a machine lacking Node. Update that note to state the real cause and that they are fixed.

Verify: run each listed test with `-run` (one regex per package is fine), then `go test ./internal/wswhiteboard` (whole package is allowed) and a `-run` over all wsdashboard tests whose name contains `Board|Rows|Drawer|Toolbar|Page|Tab|Search|Hidden`.

## Common rules (all repair delegates)
- Source of findings: lead range review e93073373..b8e434d81 (release gate, verdict NEEDS FIX). Owner decision (1187)=a: fix the blocking set and the Important set before v0.50.2, then the lead re-reviews the same base.
- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available for the Node-harness tests.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror (owner is editing them).
- Do not ssh anywhere, do not launch Electron or any GUI, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- Mirrors stay in sync: the three `ws-mcp-launcher.py` copies are byte-identical; wsflow mirrors follow `ai-docs/manuals/wsflow-mirroring.md`.
- Any change to a cross-version surface states its compatibility tier in the commit `## AI Context` (expected T0; stop and report before anything above T2).
- The repository is public: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior fix lands with a test that fails before the fix.
- Commit on your branch only; do not merge. If a fix turns out to need a product decision or grows beyond the listed files' area, stop and report instead of expanding.
- Return: commits, tests run with results, anything left unfixed and why.
