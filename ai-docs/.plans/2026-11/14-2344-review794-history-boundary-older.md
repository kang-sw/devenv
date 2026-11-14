# Review 794 UI-C2: historical boundary recovery

## Authority and outcome

MAIN assigned this bounded static bug fix at 2026-11-14T14:36:30Z, after the 649 Preview static fix. The 649 fix is now committed as 8c715c569d321a3b3aaf97a52f7ab80f1efa5577; this assignment is separate.

Fix the Older paging stall when an expanded history boundary panel is republished and its old immutable entry moves into panel-versions. Preserve finalized expanded membership and the existing 3/2/1 selection semantics. Resolve the chronological paging cutoff and whole-entry display anchor even when the obsolete boundary is excluded from rows(). An unresolved boundary must not leave permanent pending state.

## Inputs and scope

- Finding UI-C2 only: /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/c8591ce0-01-sweep794-ui-correctness.md.
- Product edit: agents-plugin-tool/internal/wswhiteboard/assets/stream.js.
- Existing regression harnesses: agents-plugin-tool/internal/wswhiteboard/stream_script_test.go and stream_test.go; edit the relevant existing test file(s) only.
- Read-only context: stream_view.go, assets/page.tmpl and the finding's cited prior plan/commits.
- Branch: fix/review794-history-boundary-older. No UI-C1, 649, TASK3, architecture, protocol, API or unrelated refactor work.

## Execution and return boundary

Source and Git operations only. Read repository path manuals before editing. Author a deterministic regression covering republish of the oldest selected boundary, Older advancement to earlier entries, unchanged restored membership and recovery from unresolvable boundaries. Do not execute formatting, tests, builds, Node, Python, browser, provider, daemon, runtime, dependency installation or diagnostic experiments; MAIN owns the local heavy slot. Static verification and code review are not passing test evidence.

Commit one logical unit with AI Context, explicit verification omissions and this plan. Keep unrelated untracked files out. Return the exact commit, changed paths, rationale, authored regression names, remaining risks and an exact proposed finite later test command with reachable child/resource inventory. Do not merge, push, close tickets or claim executable acceptance. If the existing contract requires a new structure/interface or broader scope, stop and report rather than expanding it.
