# Subagent badge on the tracker support-agent chat card

User decision (board (80) ok, 2026-11-09): the support-agent chat card in the tracker drawer also shows the running-subagent badge `N(+M)`.

## Context

- The badge landed in `261109-feat-ws-dashboard-subagent-badge` (merge c80edb923): a `subbadge` template piece on the session composer, server counts on the convlive region as `data-sub` / `data-subdeep`, and `subBadge(badgeEl, cliveEl, on)` in `assets/dashboard.js`. The session page calls it with its own `#sbadge` and `#r-convlive .clive`; the worker noted the chat card must call it with its own elements.
- The chat card landed in `261108-feat-ws-tracker-support-agent-chat` Phase 2 (merge a42ded2e5): it builds its own `.composer.supcomp` (reuses the composer's markup and CSS, not its code), and its log streams the session SSE with `?regions=entries,convlive`. That ticket decided the card reuses the composer "without its bottom badges" (the session badges); the subagent badge is added now by the user's (80).

## Outcome

- The chat card shows the subagent badge in the same position as on the session composer (top border of its composer), driven by the convlive region the card already streams, with the same rules: only while something runs, `(+M)` only when M > 0, same i18n label. Reuse `subBadge` and the badge markup/CSS rather than duplicating them; if `subBadge` or the template piece must be generalized to take elements, keep the session page behavior unchanged. No interrupt badge on the card unless it already has one.
- Make sure the card's convlive payload carries `data-sub` / `data-subdeep` (the support conversation's agent snapshot) — verify server-side that the counts are computed for that conversation.

## Verification

From `agents-plugin-tool/`: `go test ./internal/wsdashboard/ ./internal/wsi18n/ -count=1`, `go vet ./internal/...`, `node --check` on changed JS. Add or extend a test that pins the card markup/script hook if a pages test covers the card. A headless browser check against a dev daemon (isolated WS_CACHE_HOME/WS_CONFIG_HOME, as the Phase 2 worker did) showing the badge on the card while a support subagent runs is welcome; if not done, say so.

Return when committed on branch `chat-card-subbadge` with verification passing.
