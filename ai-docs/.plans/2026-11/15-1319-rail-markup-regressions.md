# Repair three pre-existing rail test failures

MAIN 2026-11-15 04:19:02Z authorizes a parallel light-lane worker to resolve TestRailMarkupRevive (rail_test.go:988), TestRailMarkupTree (rail_test.go:1114), and TestRailDraftRestore (railscript_test.go:522). MAIN reports all three fail at ce9f5c9f6 and 1ecde93d4; attribution to Settings is tentative, not a requirement.

Inspect current dashboard.tmpl/rail.js and the named tests. Preserve actual revive, child-order, drafts and Settings behavior. Correct stale fixture/parser expectations if product behavior is sound; fix product markup/logic if genuinely regressed. Do not weaken assertions or change behavior to accommodate stale tests. Keep the fix confined to these test seams and directly responsible product assets.

Work in own full checkout fix/rail-markup-regressions at e8ce32f72. Read matching repository manuals. Run relevant -run Rail tests with offline cached dependencies (GOPROXY=off from the first invocation). Add wsi18n only if strings change. No whole suites, screenshot rounds, downloads/providers/live installs/shared runtime/port29417/foreign keys or signals/forks/base sync/develop merge. One logic/state Lite only if a real product logic change warrants it; no Lite for test expectation or pure markup fixes. Two bounded small UI/test correction rounds are pre-granted.

Commit the unchanged plan and concise AI Context, return exact head, root cause and verification with clean checkout. Raw evidence stays /tmp/cache. Lead skims diff; MAIN handles integration and runtime.
