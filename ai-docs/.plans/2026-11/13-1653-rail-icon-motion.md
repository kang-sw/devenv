# Delegate plan: rail harness icon motion - running orbit line, permission wobble-hop

## Outcome

Follow-up to the rail redesign (merged ca949461a, plan
`ai-docs/.plans/2026-11/13-1609-rail-harness-icon.md`, commits 77505f3fb,
d74f3cc99, f80c69c4e). User feedback on the live rail (2026-11-13): spinning
the whole letter icon like a top is wrong. Settled motion per state:

1. **Running (`.hi.run`)**: the icon itself stays still. Bring back the
   previous running indicator - the blue outline line that travels around
   the badge's border (the old ring that used the `rail-orbit` keyframes,
   removed in the redesign; recover its look from `git show ca949461a^` /
   `77505f3fb^` in `wsdashboard/assets/dashboard.css` and `dashboard.tmpl`),
   now drawn around the square letter icon (rounded ~3px corners) instead of
   the old round badge. Same blue as before.
2. **Waiting on a permission / approval (`perm` state)**: the icon does a
   playful "wobble left-right and hop" loop: a small side-to-side tilt
   (e.g. rotate about +/-10deg) plus a short upward hop (a few px), then a
   rest pause so it reads as an attention nudge, not constant jitter
   (e.g. ~1.2-1.6s cycle with the motion in the first part). Keep the
   existing perm colour cue (the box-shadow outline) unless it visibly
   clashes with the motion.
3. If a row is both running and perm, perm motion wins (it needs the user).
4. Error and other states unchanged.
5. `prefers-reduced-motion: reduce` stops both the orbit and the wobble
   (static outline cues remain).

## Constraints

- Dashboard assets only: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`
  and, only if the orbit line needs an extra element, the rail row in
  `dashboard.tmpl`. Touch only the rail harness icon; keep the diff minimal.
  No Go data-model change.
- The hop must not shift row layout (use transform only; no margin/height
  change) and must not get clipped by the row or rail overflow - check it.
- Main track concurrently edits dashboard assets; do not refactor shared code.
- Do not restart or replace the user's live dashboard daemon. LOOK via a
  rendered fixture / separate dev instance in a headless browser.

## Verification

- Update `orbit_css_test.go` `TestRailHarnessIcon` (and `rail_test.go` if
  markup changes) to assert: the running rule animates an orbit line, not a
  rotation of the icon itself; the perm rule carries the wobble keyframes;
  reduced motion disables both.
- `go test ./internal/wsdashboard/` passes on remote w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/'`
  (Node-dependent tests locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`).
  One passing full check closes the work.
- LOOK gate: screenshots under `D:/Temp/rail-icon-motion/` - a running row
  with the orbit line mid-travel, a perm row captured at the top of a hop
  (or a short frame strip / GIF of the wobble). One retry on an
  environmental capture failure, then report.

## Return boundary

Commit on `impl/develop/rail-icon-wobble` in this worktree (this plan rides
the commit). Do not merge. Stop and report if the old orbit ring cannot be
recovered or needs a non-asset change.

Machine rules: temp files only under `D:/Temp/<purpose>/`; never create
folders in C:/ or D:/ roots; stop if D: free < 20 GB.
