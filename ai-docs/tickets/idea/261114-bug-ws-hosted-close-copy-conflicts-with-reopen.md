---
title: "Hosted lead close copy promises irreversibility despite explicit reopen"
---

# Hosted lead close copy promises irreversibility despite explicit reopen

## Background

The hosted-lead settings popover describes close as irreversible, while the
current ended-session rail and runner expose explicit reopen. This can cause
users to avoid closing a session or misunderstand the difference between
process stop, conversation close, and later explicit reopen.

## Static Evidence

- `agents-plugin-tool/internal/wsi18n/tables/ko.json:1701-1703` labels the action
  `닫기` and says `대화를 완전히 끝냅니다. 다시 열 수 없습니다.`; the confirmation
  also says the conversation cannot continue. Background-task confirmations
  repeat that implication (`1737-1738`).
- `internal/wsdashboard/assets/dashboard.js:8217-8220,8279-8283` displays that
  copy and sends the close action. `internal/wsagent/http.go:193-199` routes it
  to `Runner.CloseConversation`.
- Close ends the current lifetime, stamps ClosedAt and stops the process/mail
  watch (`internal/wsagent/runner.go:927-976,1525-1538,3190-3217`), but hosted
  `POST /agents/<id>/reopen` calls `Runner.Reopen`
  (`internal/wsagent/http.go:215-217`). It clears ClosedAt while retaining the
  key/conversation/settings, subject to existing ownership guards
  (`internal/wsagent/handoff.go:231-285`).
- The ended hosted rail offers this route
  (`internal/wsdashboard/pageview_rail.go:219-229`; `assets/rail.js:531-539`),
  labeled `다시 열기` with title `닫힌 이 세션을 대시보드에서 다시 열기`
  (`internal/wsi18n/tables/ko.json:1741-1742`).

Source-only reproduction: trace the settings close note and click handler to
`/close`, then trace the ended hosted row's reopen action to `/reopen` and its
ClosedAt-clearing branch. No browser, daemon, or test run was performed. Paths
abbreviated as `internal/` above are under `agents-plugin-tool/`.

## Constraints and Unknowns

- Capture only: no wording, route, lifecycle, or reopen behavior is selected
  for change. Preserve the distinction between process stop and conversation
  close; do not infer that ordinary mail reopens a closed conversation.
- The mismatch is established statically. Browser visibility under each row's
  ownership conditions and other translations remain unverified; generic
  close-for-good comments may also need reconciliation when scope is settled.
- Open-ticket discovery for `reopen close` and `reopen` found no matching
  discrepancy ticket locally; the query reported cached ownership because
  origin did not answer, so this is not an origin-wide absence claim.

## Phases

### Phase 1: Reconcile hosted close wording with the settled lifecycle contract

Confirm the intended human-facing distinction between stop, close and explicit
hosted reopen, then align the affected copy/documentation without changing
lifecycle semantics implicitly. Verify the settings and ended-rail wording
against existing guarded routes. Exact replacement text and any broader scope
remain unselected; this idea authorizes no implementation.
