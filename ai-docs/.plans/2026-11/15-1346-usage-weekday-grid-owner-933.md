# Usage weekly day grid — owner 933

## Authorization and outcome

MAIN relayed owner933, a pre-granted light UI lane queued after TASK3. TASK3 cutoff head has now been reported; implement the weekly usage chart day grid, run scoped wsi18n and usage Node tests, then return a head. MAIN alone integrates develop and swaps runtime. Owner screen check is visual acceptance.

## Requirements

For weekly windows draw a dim vertical gridline at every local midnight between the window start and reset. Use local calendar arithmetic, not fixed 24-hour stepping, so local midnights remain true across DST transitions. Put a weekday label in the active dashboard locale (Intl short weekday) for each day where it fits. Choose day-boundary or day-center positions for readability, not edge-pinned labels; first/last labels may be inset or offset because resets need not occur at midnight. Skip labels that collide or clip off the chart. Vertical gridlines must be subtler than the 0/50/100 horizontal lines. Preserve current 5-hour behavior. Reference image, only if useful: /tmp/usage-xaxis-owner.png.

## Scope and verification

Change dashboard usage.js/usage.css and focused usage tests; update locale tables only if new strings are necessary. Preserve popup921 localization/help/tooltip/source-removal behavior, footer settings gear, observed-series geometry and all storage/API/acquisition/stale-state semantics. Read matching project manuals before edits. Run scoped wsi18n and Node usage/trend tests, with meaningful local-midnight/DST/non-midnight-reset/label-bound checks as appropriate. No screenshots or visual matrices, full suites, network, downloads, providers, credentials, daemon actions, pushes or develop merges. Keep review proportional to this display-only change; no new lite for unchanged domain logic. Commit with AI Context and this plan. Return exact head, tests, remaining issues and clean status. Lead skims diff only.
