# Review sweep fixes: pill selectors, playbook example, git handle drag, worktree name

## Goal

Review sweep 00fd2e5d9..62304312b findings I2, I3, I6, I7 (findings:
/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/769fe701-01-findings.md; partition reports /home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/d169a2be-06-sweep-whiteboard-landing.md,
/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/d169a2be-02-sweep-dash-assets.md, /home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/d169a2be-01-sweep-dash-go.md).
User-approved local fixes (board 379, 2026-11-12).

## Fixes

- I2 `agents-plugin-tool/internal/wswhiteboard/assets/pill.js:22,26`: answer
  controls are found by class only (`.note`, `.ack input`); item-field HTML
  keeps `class` (29feba493), so `<span class="note">` in an option/why
  throws in `noted()` and stops the scripts bundled with pill.js (rail.js,
  ago.js; live.js and chg.js on the static page), and `class="ack"` around a
  task-list checkbox counts an unanswered item as answered. Use the
  structural selectors form.js/live.js/dashboard.js got in 0ba8b4fc2; extend
  TestMarkdownItemFieldCannotPoseAsAControl to cover pill.js.
- I3 `agents-plugin/rsrc/whiteboard/whiteboard.md:70` (+ the identical wsflow
  and pi copies): the example `<wb:why>A body may hold <details>, & and
  <option> as is.</wb:why>` now renders the `<details>` (a why renders HTML
  since 29feba493) and contradicts the rule a few lines later. Rewrite the
  example so it stays true (backticks, or a different raw-body example).
  Read ai-docs/manuals/skill-authoring.md, shipped-surface-boundary.md and
  wsflow-mirroring.md first; keep the copies byte-identical where they are
  today and update any test that pins them.
- I6 `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:2271-2290`
  (88c5ea709): a press on the git handle takes no pointer capture until it
  moves 5px, so a fast flick can send the first move and the release to a
  topic iframe; the drag then stays active and buttonless moves drag the
  drawer. In the drag move handler, end the drag when `(e.buttons & 1) ===
  0` (as a release would, without opening/setting height from a stale
  press). Extend drawer_height_test.go.
- I7 `agents-plugin-tool/internal/wsdashboard/railworktreeadd.go:95,:218`
  (6c5fe1f2a): the default worktree folder name comes from the current
  minute and is computed twice (form and add), so the worktree can land
  elsewhere than the form showed. Compute it once and carry the shown value
  (or have the add use exactly the form's default). Go test.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/... ./internal/mcp/...`
  plus whatever package pins the playbook copies; never go build/test/vet
  locally. Compatibility tier T0 (assets, playbook text, daemon form value).

## Constraints

- Commit on branch delegate/sweep-ui-fixes in this worktree (one commit per
  fix is fine); do not merge or push.
