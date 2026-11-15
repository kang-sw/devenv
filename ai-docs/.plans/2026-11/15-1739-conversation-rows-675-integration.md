# Conversation rows and 675 summary integration

## Authority
MAIN 2026-11-15 08:36:56Z reported the delivered B head 989c8c878 was NOT merged. Current develop 7b047f682 adds EXEC2's 675 live/history reasoning-summary insertion and history keys. This bounded integration repair is the top priority.

## Contract
Complete the existing lead-started merge of develop7b047f68228d14065ba57243df87e0c76b83ec0f into impl/develop/tabby-foe-blade at989c8c878e7ffe3a02596ac2b25c8e7ed9dee152. Preserve both unified conversation rows and all 675 live-summary/history insertion and history keys. Preserve incoming mail-wake, USER/envelope, typed report folding, batch nudge fallback, both-direction mail icons, the 1043 command-only effects fix and shared memo editor CSS. Do not implement withdrawn composer focus/caret or note-toggle changes, queued chip navigation/hover or IME work.

## Verification
Run only integration-relevant scoped tests, full wsi18n and one exact native TestSelectionNoteBrowser run with cached Playwright. Prior keyboard-test repair is already included: bounded Shift+ArrowRight instead of Shift+End. Do not repeat review, browser matrices or broad suites. No shared runtime, downloads or provider calls. Stop on a new unexplained failure rather than retry. Report a blocker exceeding 20 minutes with exact log paths for MAIN's Claude diagnosis.

## Handoff
Return a clean committed quiescent full head after completing this same merge. Do not self-sync again, merge to develop, reopen/close the already-completed ticket, or edit frozen Results except a narrowly necessary integration Edition. Include merge rationale and MAIN's explicit authorization in AI Context. MAIN owns final develop integration and owner-screen acceptance.
