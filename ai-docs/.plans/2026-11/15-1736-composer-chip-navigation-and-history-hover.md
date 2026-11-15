# Composer chip navigation and full history-note hover

## Authority
MAIN's 2026-11-15 08:22:37Z message relays owner decision 1069. Implement as a separate small head after conversation-row unification. The old addMention reveal/focus/caret and note-toggle-to-composer proposals are withdrawn.

## Contract
- In the dashboard composer, only the small right-hand remove control deletes a board-mention chip. Clicking its body reuses existing ID-chip navigation to open the selected session's board, reveal the referenced card/entry and briefly highlight it.
- Conversation-history answer chips expose their full untruncated original note in an escaped title/tooltip. Keep the compact visible chip.
- Preserve chip payloads, command/IME guards, draft persistence, session isolation, mail/envelope and unified conversation rows. Do not alter note-toggle behavior or add composer reveal/focus/caret behavior.

## Scope
Inspect the existing mention chip renderer/actions in agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js and existing board navigation code; reuse that path rather than introducing a navigation subsystem. Change only directly required related dashboard assets, dashboard.tmpl and focused tests. AnswerLine.Note is already full text; ParseAnswerBlock does not clip it. Follow current AGENTS.md and applicable path manuals before editing.

## Verification and handoff
Run relevant scoped regressions and full wsi18n, offline first with isolated fixtures. At most one bounded logic review after the implementation commit, blocking only Important/Critical findings. No browser matrix, screenshots, shared runtime, downloads or broad suites. Return a clean committed checkpoint for a lead-owned current-develop merge immediately before the head. The same worker then runs only integration-affected checks and returns a clean quiescent final head. MAIN owns develop integration and owner-screen acceptance.
