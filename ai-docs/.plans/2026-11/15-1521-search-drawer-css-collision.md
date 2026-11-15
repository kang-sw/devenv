# Search drawer CSS collision fast follow-up

MAIN's 2026-11-15 06:15:19 instruction authorizes a tiny separate fix after Search Phase 2 merged as a7340d618. TestCSSClassesDoNotCollideAcrossModules reports the new .tk-palette collision between dashboard-drawer.css and tracker.css, introduced by drawer commits 40ea004f5 / ee2d95201.

Rename the drawer's colliding class and its matching DOM/test references to a drawer-specific name, preserving layout and behavior. Scope: agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.css, the matching drawer JS references, and directly affected search/collision tests. Do not rename unrelated tracker classes or change UI, contracts, tickets, or compatibility behavior. Read applicable project manuals before editing.

Run TestCSSClassesDoNotCollideAcrossModules, the existing search drawer scoped test set, wsagent TestSearchAnswer, wsi18n, and git diff --check. Report exact selectors and outcomes. No broad test/review, screenshots, runtime launches, provider/auth/network/install, daemon access or swaps, merge/rebase/push, cleanup of unrelated artifacts, or changes to 675. Commit the tiny fix and this unchanged plan with AI Context on fix/search-drawer-css-collision, then return head, changed paths, verification and clean/protected state. MAIN owns develop integration.
