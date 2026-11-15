---
title: "Dashboard conversation send follows the incoming reply at the bottom"
related:
  261115-refactor-ws-dashboard-send-latency-followups: "landed integration prerequisite; already confirmed in Constraints"
  261115-feat-ws-dashboard-agent-badge-popover: "landed integration prerequisite; already confirmed in Constraints"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 955956e06d13ddee
sage-review-completeness-reviewed: 955956e06d13ddee
completed: 2026-11-16
---

# Dashboard conversation send follows the incoming reply at the bottom

## Background

**VERIFIED (source only):** The live conversation uses one `#convo` scroller. `followConvo()` already sets `stick = true` and pins to the bottom; visible scroll events recompute `stick` using a 24 px bottom tolerance (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L284-L297`). `sendMessage` already calls it after the sending/empty/unready-image guards and before POST (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js#L1213-L1246`). Button and eligible Enter share that function; IME/default-prevented events and picker-owned keys return first (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js#L1600-L1629`). Incoming entries/convlive swaps sample the pin before updating and pin afterward (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L64-L79`, #L125-L137). The helper, send re-arm and regression test were introduced by `a06a1c436`, merged by `a3d61df63`, which is an ancestor of this checkout (read-only `git merge-base --is-ancestor a3d61df63 HEAD`, exit 0). The former missing-mechanism claim is stale.

**VERIFIED (source/history):** Re-arm occurs on valid send intent, not server acknowledgment: a failed POST warns and returns without undoing the jump (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js#L1241-L1246`). Merge `a3d61df63` explicitly accepted this failure outcome. Legacy selected-command and inline-command sends also re-arm after their image guards; preserve those existing outcomes (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js#L895-L908`, #L925-L935). Later visible upward scrolling beyond the bottom tolerance clears follow through the existing scroll listener (#L294 of dashboard.js).

**INFERRED:** The existing source matches the confirmed send/follow/cancel scope; no product defect is established by this inspection. Real browser geometry, asynchronous scroll/SSE ordering and preservation of composer outcomes remain unexecuted, not proven by source or historical test receipts.

**VERIFIED (MAIN11:37 relay: 693=ok):** MAIN means dashboard message SEND (Enter/button), not the first keystroke: after sending while scrolled up, move the conversation to the bottom and follow the incoming reply. A later user upward scroll after sending must still cancel follow mode; IME, picker, empty/invalid guards, and unrelated composer outcomes remain unchanged.

Owner words: "사용자가 채팅 입력하는 순간 채팅 대화 패널 가장 아래쪽까지 스크롤 다운되어 내려가게."

## Decisions

- Compatibility tier: **T0** — dashboard asset behavior only; no API, protocol, store, or daemon semantics change.
- Scope: preserve the existing dashboard send/follow implementation, add focused real-browser proof and paired before/after-send evidence. No shared/live browser or daemon mutation is part of this ticket; authorized isolated test fixtures are permitted only after the execution gate below.
- **VERIFIED (MAIN disposition, 2026-11-15 18:18:48Z):** Remaining delivery is real-browser proof with focused count-10 verification and paired captures. If proof finds no behavioral gap, deliver test plus ticket closure/evidence only, with no product change. If a real gap is found, stop and report it with captures before fixing; do not duplicate or remove the existing mechanism. This is confirmed scope, not an open product/UI decision.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- **VERIFIED (MAIN 2026-11-15 18:16:30Z):** SEND means Enter/button, not typing: start scrolled up, jump bottom on valid send, follow incoming reply, then allow a later upward scroll to cancel. Preserve IME, picker, empty/invalid guards and every other composer outcome. T0 only; keep layout/style unchanged and introduce no product/UI design decisions.
- **VERIFIED (MAIN 2026-11-15 18:19:35Z):** Both landing prerequisites are confirmed: `261115-refactor-ws-dashboard-send-latency-followups` C/D/E plus Home seam (`dd5bb3d26`) merged to develop `ea80c8103`; `261115-feat-ws-dashboard-agent-badge-popover` is present via `ad9c2c79f`. Read-only develop history also shows both merge commits. Rebase target is `ea80c8103` before real-browser tests/final source commit. Root sequences the normal ready gate/preparation commit and bounded rebase; do not rebase with these ticket edits uncommitted. Another lead alone integrates.
- **VERIFIED (MAIN fixture boundary):** Any authorized fixture daemon must use fresh HOME, CLAUDE_CONFIG_DIR, CODEX_HOME, WS_CACHE_HOME and WS_CONFIG_HOME, plus explicit Home when the seam is available. No shared/live runtime mutation, provider/credential access or installation is authorized.

## Prior Decisions

- a3d61df6 (2026-11-15, commit): "Accepted LOW: follow re-arms on send intent (before server ack), so a refused send also jumps to bottom where the failure toast shows" — bearing: constrains
- 261114-refactor-ws-dashboard-whiteboard-asset-split (2026-11-15, commit 0c46ba10): "Separator-free fragment joins preserve every upstream asset byte, one dashboard IIFE, evaluation order, CSS cascade" — bearing: constrains
- 261115-feat-ws-composer-inline-skill-and-topic-capsules (2026-11-15, commit 44d01be7): "Preserve editor IME/keyboard ownership, native insert/delete undo transactions when supported, plain-text clipboard/drop boundaries" — bearing: constrains
- 261114-feat-ws-selection-bundle-comment (2026-11-15, commit 549f3e06): "Client-local bselections owns copied ordered sources, one editable memo and explicit image numbers." — bearing: constrains
- 261115-feat-ws-dashboard-user-message-envelope (2026-11-15, commit 697c9508): "Consume the landed asset split by symbol and edit only owning fragments." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Existing assets/dashboard.js, dashboard-composer.js, dashboard-board.js and conversation_follow_test.go under agents-plugin-tool/internal/wsdashboard; remaining browser proof and ticket evidence. |
| scope.surface | internal | Existing dashboard client behavior and isolated test coverage; no API/protocol/store changes authorized. |
| scope.new_public_symbol | no | None; preserve existing followConvo/sendMessage. |
| scope.new_type_contract | no | No new signature, persisted shape or transport contract authorized. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/conversation_follow_test.go#L18-L81 and existing browser harnesses; destination of added browser proof is not yet selected. |
| complexity.reuse_points | confirmed | Read followConvo, sendMessage, both command sends, keyboard guards, region pinning and TestConversationSendRearmsBottomFollow. |
| complexity.side_effect_risk | moderate | INFERRED: browser fixtures may invoke plan-usage/runtime paths unless HOME and daemon Home are isolated. |
| risk.correctness | moderate | INFERRED: real scroll-event timing and streamed reply growth are not modeled by the current stubbed test. |
| risk.fit | moderate | INFERRED: proof must use the existing scroll owner and composer guards without changing layout/style or send outcomes. |
| risk.test | high | VERIFIED: existing test stubs geometry/events; real Enter/button count-10 proof and paired captures remain unexecuted. |
| risk.security_or_contract | moderate | INFERRED: fixture isolation must prevent real-home/provider access; T0 forbids API/store/daemon-semantic changes. |

## Phases

### Phase 1: Re-arm conversation bottom-following on message send

Keep the change limited to the dashboard conversation send/follow path. Add focused coverage for a conversation initially scrolled up, a send via Enter/button, and the subsequent incoming reply staying visible at the bottom. **VERIFIED (source only):** Existing coverage verifies composer markup (`agents-plugin-tool/internal/wsdashboard/pages_test.go#L257-L297`), send outcomes (`agents-plugin-tool/internal/wsdashboard/composer_toast_test.go#L57-L78`), draft clearing after send (`agents-plugin-tool/internal/wsdashboard/railscript_test.go#L501-L524`), and server-side entries/history regions (`agents-plugin-tool/internal/wsdashboard/conversation_history_test.go#L30-L69`, #L192-L210). `TestConversationSendRearmsBottomFollow` already asserts send/bottom/reply/later-cancel and empty/image guards (`agents-plugin-tool/internal/wsdashboard/conversation_follow_test.go#L18-L81`, introduced by `a06a1c436`; latest file change `3d2245c2e`). It calls sendMessage directly for both labeled input cases, string-checks handler wiring and simulates incomingReply/userScroll with a plain object; it is not real-browser Enter/button or scroll-event proof. No such proof was found by bounded search for send/follow/bottom/reply/convo in the dashboard browser tests and testdata browser scripts; no tests were run here.

Verification — **VERIFIED requirements confirmed by MAIN 2026-11-15 18:16:30Z and 18:18:48Z**, pending the execution gate:

- Add focused real-browser Enter and button coverage: initially scrolled up, post-send bottom jump, incoming reply stays at bottom, later upward scroll cancels. Retain the existing stubbed regression; preserve all composer guards/outcomes.
- Require focused `go test ./internal/wsdashboard -run '<exact focused selectors>' -count=10` green from `agents-plugin-tool/`; choose actual selectors after adding proof. Never run the full wsdashboard suite. Record exact commands and receipts; this is a required future check, not a pass claim.
- Save paired before/after-send captures at `/tmp/send-follow-shots/{before,after}-*.png` for MAIN comparison. These compare pre-send and post-send state, not an invented product-change baseline.
- Independent review must have no Critical/Important findings. If proof exposes a real behavior gap, stop and return captures before any product fix; otherwise deliver tests and ticket closure/evidence only. Record any missing proof explicitly.

The explicit MAIN11:37 relay `693=ok` confirms the behavioral scope, not permission to rerun or duplicate the already-landed fix. MAIN's 2026-11-15 18:18:48Z disposition narrows remaining delivery to proof/evidence as above; landing is confirmed by MAIN18:19:35, and final source commit/tests still require the root-sequenced rebase to `ea80c8103`.

### Result (528b6f9ba) - 2026-11-15

**VERIFIED:** Added `conversation_follow_browser_test.go` and
`testdata/conversation-follow.browser.cjs`; no production files changed and no
behavioral gap was found. Actual Enter and `#csend` clicks start with a native
wheel-scrolled, overflowing `#convo`. The real message request is held before
transport until measured bottom geometry and the after-send capture prove the
intent-owned jump. It then reaches the disposable daemon and returns HTTP 200.
Two real live-region reply growth stages retain a visible reply tail at the
bottom. Later upward wheel input cancels follow: further growth increases
height without moving the user's scrollTop. Empty/composing Enter is guarded;
existing invalid-image, picker, draft and send-outcome tests remain unchanged.

**Decisions:** Browser proof only (T0), as authorized; keep the interaction and
isolated fixture together because their timing contract is non-decomposable.
No helper invocation, geometry stubbing, layout/style change or asset-owner
change was needed. The fixture creates fresh HOME/provider/config/cache/XDG
directories per invocation and explicitly passes daemon Home; only ephemeral
fixture origins are admitted in Chromium.

**Verification:**

- Before source/tests, clean `impl/develop/stomp-stunt-fox` was rebased from
  `f46aa9b96` onto exact local `ea80c8103`, yielding preparation `9acaab0d7`.
  Both `dd5bb3d26` and `ad9c2c79f` were verified ancestors afterward.
- From `agents-plugin-tool/`, with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`,
  cached Playwright/Chromium and isolated environment roots:
  `go test ./internal/wsdashboard -run '^(TestConversationSendFollowBrowser|TestConversationSendRearmsBottomFollow|TestComposerOutcomesToast|TestSlashPickerListsAndPicks|TestRailDraftRestore)$' -count=10 -v`
  passed all 50 executions in 33.252s; zero failures or skips. Full output:
  `/tmp/send-follow-count10.jESYXZ/test.log`. The initial two-selector smoke
  also passed: `/tmp/send-follow-smoke.9tvJbY/test.log`.
- Captures and measured geometry: `/tmp/send-follow-shots/`. Representative
  Enter pair: `before-enter-27742.png` / `after-enter-27742.png`; button pair:
  `before-button-42373.png` / `after-button-42373.png`. Inspected pairs show
  the expected scrolled-up-to-bottom transition, not a product-change baseline.
  `reply-enter-42373.png` shows the growing reply tail visible;
  `cancelled-enter-42373.png` shows the reader remaining above the tail.
  Count-10 geometry is retained in `evidence-42373.json` (plus nine other runs).
- Owned Go launcher/test, browser-driver Node and Chromium root PIDs were
  recorded at startup; all 22 count-10 PIDs were checked absent after exit in
  `/tmp/send-follow-count10.jESYXZ/processes.log`. Smoke launcher/test/Node/
  Chromium PIDs 26816/27636/27742/27762 were also checked absent. The daemon
  was in-process and closed by fixture cleanup; no unrelated process was
  signaled and no shared runtime was changed. Absence claims cover these
  recorded root PIDs, not an unrecorded process inventory.
- Independent one-pass lite review: no Critical/Important findings. Artifact:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@c4d08aee/review-paths/953632c0-01-send-follow-lite.md`.

Ticket guardrails passed. The stale sage-review digest advisory is accepted:
the approved phase plan and decisions are unchanged; only completion evidence
was appended after independent implementation review.

**Omitted:** Full dashboard/MCP/project suites, other browser engines and
exhaustive browser guard/failure matrices were outside the authorized focused
verification. The existing focused Node contracts supply invalid-image,
picker, draft and outcome coverage. No approved visual reference was linked;
paired captures are supplied for MAIN's comparison. MAIN alone integrates the
retained branch; the lease remains held until its merge acknowledgment.
