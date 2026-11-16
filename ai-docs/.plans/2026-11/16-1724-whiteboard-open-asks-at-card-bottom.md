# Whiteboard: open asks out of the history, stacked at the card bottom

## Context

Owner request 2026-11-16 16:42 KST, understanding confirmed (1254)=ok; (1255)=b
sequenced after panel-first Phase 2, which merged as develop 0b1d06db8.

An earlier change renders open ask cards (checkbox / x / pen) inline in the
topic history at the position where the ask was made, and uses the folded
history's "load more above / load more below" mid-expand to reveal folded
history around an open ask. The owner wants that inline placement reverted.

## Outcome

1. Remove the topic-card header chip summarizing check answers (the
   "확인 요청 답변 0/1" style chip under the topic header).
2. Open ask cards are no longer rendered inside the history/timeline.
3. Keep the mid-expand mechanism ("load more above / below" expanding only
   the region around a target) and use it for navigation to an item inside
   folded history: following an item reference such as `{{1248}}` / `(1248)`
   link, or jumping from a bottom ask card to the moment the ask was made,
   expands only the surroundings of that target and scrolls/rings it.
4. Open asks stack at the bottom of the topic card, after the history, in
   creation order. The history keeps only the ask-time log line (the existing
   "(1248) 확인 … open" style event line) at its original position. Each
   bottom card offers a jump to its ask-time line (Outcome 3).
5. Everything else in the history stays as it is today: answered-ask logs,
   panels (including Phase 2 version rows), agent/system lines. Only the
   position of currently open ask cards changes. When an ask is answered, it
   leaves the bottom stack and its history lines remain as today.

Also fix (dogfood surprise reported by the Phase 2 worker, same code path):
a hidden answered check at the live edge of the history has zero height, so
`stream.js` `readyHeight` waits on it forever and "load above" stays blocked.
Zero-height (hidden) rows must not block readiness.

## Approach notes

- Find the inline-open-ask change with `git log --grep` / `git log -S` over
  `agents-plugin-tool/internal/wswhiteboard` and `internal/wsdashboard/assets`
  (stream.js and friends). Revert it surgically on top of Phase 2, not a blind
  `git revert` if Phase 2 has since built on the same code.
- Applies to both the standalone whiteboard page and the dashboard board view
  if both render the same stream.
- View-only change: no store, MCP, or command change (T0). State the tier in
  the commit `## AI Context`.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/` (AGENTS.md Implementation Conventions).
- i18n: any new/removed UI string updated in all locale tables.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` (owner-edited).

## Verification

- Go tests ONLY on remote w5090 (`D:/sys_lock.sh remote w5090 run ...`, note
  remote-build-w5090). NO local `go test`, no test exes, no ws/ws-mcp exes,
  no launcher on this PC. Node/Playwright locally is fine (Phase 2 used
  `testdata/*.browser.cjs` driven locally against a page rendered on w5090).
- Known base failures (excluded, being fixed separately): wsdashboard
  TestCSSClassesDoNotCollideAcrossModules (.board-updated/.dw/.oq-pill),
  TestDependencyPopoverCSSScope, TestPagesCarryTheTracker,
  TestRefreshCacheNonBoardWake, TestPagesCarryTheToastStackAndTitleHost;
  wswhiteboard TestTopicRailBarCapsuleMotion.
- LOOK: dark theme captures at ~806 and ~420 px of a topic with history,
  two open asks (one check, one policy), one answered ask, and a folded
  history: (a) asks at the bottom in creation order, (b) no header chip,
  (c) jump from a bottom card to its folded ask-time line expands only that
  region, (d) an item reference link does the same. Temp captures under
  D:/Temp/open-asks-bottom/, deleted after.
- R1 review; fixes-only R2 for substantive findings.
</content>
</invoke>
