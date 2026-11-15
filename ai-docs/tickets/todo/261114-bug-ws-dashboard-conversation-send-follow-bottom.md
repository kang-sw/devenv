---
title: "Dashboard conversation send follows the incoming reply at the bottom"
---

# Dashboard conversation send follows the incoming reply at the bottom

## Background

**VERIFIED:** The live session conversation is a single `#convo` scroller. It follows new content only while `stick` remains true (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:253-264`); scrolling up sets `stick` false. The send path posts from `sendMessage` (`dashboard.js:2437-2487`), and both the send button and Enter route through it (`dashboard.js:2778-2805`), but no send path re-arms bottom following. Incoming `entries`/`convlive` swaps call `pinConvo` only when the existing pin is active (`dashboard.js:585-599`, `:645`), so a user-scrolled-up conversation can miss the sent turn and reply.

**VERIFIED (MAIN11:37 relay: 693=ok):** MAIN means dashboard message SEND (Enter/button), not the first keystroke: after sending while scrolled up, move the conversation to the bottom and follow the incoming reply. A later user upward scroll after sending must still cancel follow mode; IME, picker, empty/invalid guards, and unrelated composer outcomes remain unchanged.

Owner words: "사용자가 채팅 입력하는 순간 채팅 대화 패널 가장 아래쪽까지 스크롤 다운되어 내려가게."

## Decisions

- Compatibility tier: **T0** — dashboard asset behavior only; no API, protocol, store, or daemon semantics change.
- Scope: the existing dashboard send/follow path, focused regression coverage, and headless before/after evidence. No browser or daemon mutation is part of this ticket.

## Phases

### Phase 1: Re-arm conversation bottom-following on message send

Keep the change limited to the dashboard conversation send/follow path. Add focused coverage for a conversation initially scrolled up, a send via Enter/button, and the subsequent incoming reply staying visible at the bottom. Existing coverage verifies composer markup (`agents-plugin-tool/internal/wsdashboard/pages_test.go:257-297`), send outcomes (`composer_toast_test.go:57-67`), draft clearing after send (`railscript_test.go:519-520`), and server-side entries/history regions (`conversation_history_test.go:30-69`, `:150-201`), but no test found here asserts the requested send-then-follow scroll behavior.

Implementation is authorized by the explicit MAIN11:37 relay `693=ok`; keep the change bounded to the confirmed scope above.
