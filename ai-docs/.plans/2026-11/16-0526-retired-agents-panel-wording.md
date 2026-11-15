# Retired agents-panel wording cleanup

## Authority and base

MAIN assigned exactly one bounded task at 2026-11-15 20:25:59Z: resolve review follow-up `parka-tweezers-amusable`. Work on fresh branch `fix/retired-agents-panel-wording` from exact develop `b27e2ce7144c1337218b38461cb4802c82ac253e`. Phase 4 is merged and released. Phase 5 is not assigned.

## Scope and implementation

Inspect the actual current occurrences of retired `agents panel` / `in the panel` wording in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` (reported lines 200–201 and 214) and `assets/dashboard.css` (reported line 156). Some may be comments. First read `git show 8aa2aaed7 -- agents-plugin-tool/internal/mcp/pi_tools.go` and use that previously settled wording as reference. Change only the obsolete wording, with no selector, markup structure, CSS styling, accessibility semantics or behavioral change. If strings are user-visible and use i18n keys, keep Korean and English consistent by updating only the directly associated translation entries. Do not edit `pi_tools.go`, which EXEC3 already fixed, or other unrelated occurrences/packages.

Add or adjust a regression test only when an existing test already pins this wording. Do not invent a new test or broaden scope for a comment-only repair. Read all matching AGENTS/manuals before editing; shipped-surface and skill-authoring/wsflow manuals apply if their declared paths or artifact kinds require them. Any material scope expansion stops for lead disposition.

## Verification and delivery

Find the existing wsdashboard tests that read `dashboard.tmpl` / `dashboard.css` using grep. Run only the relevant scoped `-run` tests with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, `-count=1`; retain exact commands and results. Never run full dashboard/MCP/module suites. No network, installation/download, real provider/harness launch, Windows SSH, runtime/shared-board mutation, process signals, merge, rebase, push or CI. No UI creation or layout changes are authorized.

Commit one logical wording repair plus this unchanged required plan, with `## AI Context`. Resolve the review ID through the repository's structured `resolves: ["parka-tweezers-amusable"]` commit field, matching the previous five follow-up commits' `## Resolves` convention; inspect their format instead of inventing a ticket or silently writing an unrecognized message. Report full HEAD READY SHA, base, changed paths, whether wording was visible or comments, i18n treatment, exact scoped test outcomes, resolution marker and clean status. MAIN alone reviews/merges and swaps the daemon. Retain the lane until its integration report; do not start Phase 5 or any other work.
