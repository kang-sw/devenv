# Usage popup owner feedback 921

## Authorization

MAIN relayed owner feedback (921), pre-granting this light lane: implement, scoped tests including wsi18n and Node usage tests, then head. No lite unless logic changes. Owner on-screen check is visual acceptance. MAIN alone integrates develop and swaps runtime.

## Changes

- Localize every new usage-popup string through existing wsi18n tables for all 11 supported locales, including chart titles/tooltips/accessibility text and legend labels.
- Format X-axis date labels as short weekday names with the dashboard active locale using Intl.DateTimeFormat(locale, {weekday:'short'}), e.g. Korean 월/화/수 and English Mon/Tue. Preserve the existing hourly-window time presentation unless the owner requirement explicitly addresses its date labels.
- Move explanatory history-state notes into title/tooltips instead of visible text.
- Remove visible top legend rows. Add a focusable '?' icon immediately right of the Plan usage heading; hover and focus expose the localized legend.
- Remove bottom-left source text such as Claude push / via Codex CLI from the popup presentation, without changing source identity in storage or API.

## Scope and verification

Work on dashboard usage.js/usage.css, the existing wsi18n locale tables and focused tests. Preserve history geometry, collection, persistence, discontinuities, stale-state semantics, API compatibility and the footer settings gear. Read all matching project manuals before edits. Run scoped wsi18n and Node usage/trend tests; no screenshots, visual matrices, full suites, provider/credential/network/download/shared-daemon actions, pushes or develop merges. No lite unless logic changes; distinguish display formatting from domain logic. Commit with AI Context and this plan; append an Edition/Result to the completed usage-history ticket only if needed without rewriting prior Results. Report exact head, tests, any remaining issue, logic-change assessment and clean status. Root skims diff only; owner screen feedback accepts visuals.
