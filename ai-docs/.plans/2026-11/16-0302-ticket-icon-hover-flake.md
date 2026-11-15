# Ticket-icon hover browser flake

## Assignment

Diagnose and repair intermittent `TestTopicTicketIconBrowser` failure, "hover does not open the popover" at light@700. MAIN measured base a3b5101ec failing 2/3; 28cf9c7bc passed 3/3 once. Inputs: /tmp/p-wb.log, /tmp/p-base.log, /tmp/p-re.log. This owned branch starts at a603b8f989497e8e549c5811fea947cd4c9873bb, a verified descendant of 28cf9c7bc.

Distinguish fixture synchronization from product hover/popover race using source evidence and a deterministic controlled-delay pre-fix reproduction. Fix the cause without blind timeout bumps or weakened/deleted assertions. Scope is wswhiteboard ticket-icon test/assets only. Avoid stream.js/history-fold code unless directly required by the demonstrated cause; avoid agent badge popover files and refusal-cost store/command files. Read applicable AGENTS.md and matching manuals before editing.

A small T0 product JS/CSS repair is allowed. Broader scope, protocol/API behavior changes, T3, or unmeasured material unknowns require stopping and reporting. Test-only repair needs no screenshots or independent logic review. Product repair requires one bounded independent logic review and before/after headless screenshots of hovered popover states at light@700 and dark, saved as /tmp/ticket-icon-shots/{before,after}-*.png, with every visible change explained. Use only owned fixture servers and cached Playwright; never shared daemon/29417.

## Verification

Acceptance: deterministic controlled-delay failure before the repair and pass after it, retaining the original assertions; `-run '^TestTopicTicketIconBrowser$' -count=20` passes with WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright and PLAYWRIGHT_BROWSERS_PATH=/Users/kang-sw/Library/Caches/ms-playwright. Every Go shell invocation, including nested ones, must explicitly cd to this absolute owned agents-plugin-tool directory with GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local and isolated HOME/config/runtime/temp plus warm caches. Explicit absolute browser cache path must remain set when HOME is isolated.

No full wswhiteboard/wsdashboard/mcp or ./... suites, downloads/providers, native Windows, Mac Electron, shared runtime, CI, push, merge, foreign keys/signals, or outer Go kills. Keep diagnostics outside the checkout, with honest limitations and attempted controls.

## Deliverable

Commit implementation and this unchanged write-once plan with AI Context. Report head SHA, exact touched paths, root cause with source citations and causal evidence, verification, screenshot paths only if product changed, and remaining gaps. Keep the clean checkout held until MAIN integration report and lead release. Reports use concise complete English, VERIFIED/INFERRED labels, material updates only. Lead skims the diff/report and does not rerun worker tests. Next assignment comes only from MAIN.
