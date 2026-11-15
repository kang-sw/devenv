# Restrict the toolbar backing to its intended painted box

MAIN 2026-11-15T03:34:27Z relays owner feedback after badge acceptance (903): the opaque backing hides part of the first sticky folded-topic strip below the toolbar. Owner screenshot: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/a640731720b44843-1.png.

Implement the smallest CSS-only fix on base 9c795e410. The opaque backing must cover the gap above the floating toolbar and the toolbar's own painted box, but must not extend below the toolbar bottom onto the following folded strips. Preserve the top-gap bleed fix, approved toolbar order, badge/timer behavior and pointer interactions. Inspect actual box/offset styles before interpreting the earlier 12px extension; do not blindly remove upward coverage and reintroduce the original bleed. A minimal folded-strip offset adjustment is allowed only if it is simpler and preserves the intended boundary.

Scope: agents-plugin-tool/internal/wsdashboard/assets/dashboard.css only, plus this write-once plan and commit rationale. No product JavaScript, protocol, store, settings or telemetry edits. Read matching implementation manuals before editing. No new ticket or alteration of frozen completed Result text.

Run only the existing relevant toolbar/budget -run tests with their native Go timeout and cached dependencies. No Lite, full suite, screenshot-proof pass or viewport matrix. No browser is required. The owner checks the actual screen after MAIN merges and swaps. Up to two bounded fix rounds are pre-granted under light-lane policy. Stop if a non-CSS product change is required.

Use isolated offline test fixtures/cache/config; never port 29417, providers, downloads, shared-daemon build/restart/swap/smoke, owner installation or an outer Go-process kill. Keep raw outputs outside ai-docs. Commit the CSS and this plan with AI Context, then return a head promptly. MAIN owns merge and runtime acceptance; the lead skims the diff without repeating logs or cleanup checks.
