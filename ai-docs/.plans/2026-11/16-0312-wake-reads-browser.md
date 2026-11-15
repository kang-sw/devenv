# Wake reads browser failure

## Assignment

Diagnose and repair `TestWakeReadsBrowser` in internal/wsdashboard, starting at exact fd09e48dc536f2b0fb7f5e7bd1279bf3c196bdf1. The Claude UI agent saw the same [eval]:23 timeout on develop 806183936 with and without its change. Reproduce on this base, determine stale fixture/markup versus product regression with evidence/history (bisect only if useful), and check whether CI's -short selection runs this test.

No blind timeout increase. Keep assertions unless a merged ticket deliberately changed the asserted behavior; cite that authority. Limit edits to the demonstrated wake-read cause. Agent badge popover session templates/CSS/top-left list and refusal-cost files are excluded: if the cause lies there, STOP and report to lead/MAIN without editing. Follow applicable AGENTS.md/manuals.

Small T0 product fixes are allowed; broader scope, protocol/API changes or T3 stop/report. Product UI changes require before/after headless screenshots /tmp/wake-reads-shots/{before,after}-*.png and visible-change explanation, plus at most one bounded independent logic review. Test-only fixes need no product screenshots/review.

## Verification and safety

Only named test `-run '^TestWakeReadsBrowser$'` with count10 is authorized; never full wsdashboard/mcp or ./... suites. Every Go shell invocation including nested calls explicitly cd to the absolute owned agents-plugin-tool directory and set GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local, isolated HOME/config/runtime/temp and existing warm caches. Cached browser environment: WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright PLAYWRIGHT_BROWSERS_PATH=/Users/kang-sw/Library/Caches/ms-playwright, absolute despite isolated HOME.

Use owned fixtures only. No downloads/providers, native Windows, Mac Electron, shared daemon/29417, runtime swap, CI dispatch, push, merge, foreign keys/signals, forks, or outer Go kills. Keep evidence, controls and honest limitations outside checkout in a unique /tmp directory. Diagnose causally, not by merely retrying until green.

## Deliverable

Commit implementation plus this unchanged plan with AI Context. Report head SHA, exact changed files, reproduced failure and causal evidence/source citations, verification and CI-short inclusion result, gaps, and screenshots only if product UI changed. Keep clean checkout held until MAIN integration report and lead release. Reports: concise complete English with VERIFIED/INFERRED, only material intent/blockers/head/final/ETA slip. Lead skims reports/diff and does not rerun tests. Next task only MAIN.
