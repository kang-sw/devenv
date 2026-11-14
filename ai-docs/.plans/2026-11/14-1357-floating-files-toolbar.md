# Approved toolbar floating Files correction

## Authority
Owner625 approved Files toolbar floating-panel behavior, relayed by MAIN. Exact reference `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/docs/handoff-ux.md:63-68`. Completed `261114-feat-ws-dashboard-board-toolbar-overview` Files-tab behavior is superseded only for toolbar action, not existing Files-tab access.

## Contract
Toolbar Files toggles a floating Files panel with maximize-like presentation; second click or Esc closes. Preserve existing Files-tab access and reuse the existing file explorer rather than mount another independent browser. Preserve capability gating fpane && WsFiles. Preserve dropdown/search behavior: consumed Esc remains consumed, search Esc clears search before the outer panel closes. Return focus and explorer to its original placement/tab/state when dismissed; preserve preview interaction and established maximize exclusivity. These are preservation criteria, not authority for a redesigned browser or dropdown.

## Scope
Prefer agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, dashboard.css; cards.js only for bounded reuse of existing sheet implementation. Existing files.js interaction is evidence, not authorization to redesign. Tests: files_test.go/cards_test.go and scripts/board-headless.mjs toolbar scenario. Read applicable manuals and source before edits. If genuinely new reusable public structure, an exported API/type/module, or unsettled product behavior is required, stop before expansion for ticket routing.

## Verification and completion
Exercise toolbar toggle, second-click close, Esc dismissal priority with dropdown/search, focus return, original tab/explorer state restoration, previews, other maximized sheet interaction, and capability absence. Isolated test servers/source build allowed; no live shared daemon/snapshot/installed launcher/config change. No dependency network installs, merge/rebase/pull/push/release. Commit verified logical unit plus this plan with AI Context on feat/floating-files-toolbar. Report tests, commit, limitations and independent review needs in concise English complete VERIFIED/INFERRED bullets with file:line citations. If reviewer provenance gate fails request lead review; no bypass/retry prompts. Other worktrees concurrently change dashboard.js: do not wait solely for overlap; lead integrates serially and adjudicates interactions.
