# Dashboard board pane: flicker when off-screen folded topics pour in

## Outcome

User report (2026-11-12, browsing the dashboard after reinstall): in the
right-hand board pane, when off-screen collapsed (closed/folded) topics
"pour in" all at once, they render black first and then appear, with odd
flicker. It was observed right after the lead closed 8 topics in one
whiteboard update, so a batch of topics changed from open to closed (folded)
at once and the board morphed.

Find the cause and fix it so that a batch of topics changing (newly closed,
folded, or appearing off-screen) shows no black or blank frames and no
flicker. Keep the intended motion: newly appearing topics slide in, updated
topics interpolate height, no animation on first load, while editing, or
under prefers-reduced-motion.

Suspects to check (evidence first, do not assume):
- topic motion in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
  `morphBoard` (height animation from 0, interaction with folded topics);
- the closed-topic fold (closed topics go last, collapsed);
- topic body frames: a changed topic's frame blanks while it reloads (a known
  unchecked item from an earlier round), with a dark background before load;
- `content-visibility` / containment, or the rail/orbit perf changes
  (rail.js offsets, `--wb-rail-*` vars, capsule) repainting off-screen items;
- the whiteboard page assets under `agents-plugin-tool/internal/wswhiteboard/assets/`
  if the board pane renders through them.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` first (assets ship).
- Quiet dashboard tone; `--wb-*` tokens only.
- A peer session shares the main checkout /home/swkang/devenv and edits the
  topic-note code in dashboard.js (~L770-810 on develop); avoid that region
  unless the fix requires it, and say so in the report if it does.
- Never run go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool -m 20 -- test -count=1 <pkgs>`;
  run at least ./internal/wsdashboard/... and ./internal/wswhiteboard/...
- `node --check` on every changed JS file.

## Verification

- Reproduce first: headless Chromium against a board page (mockup or a page
  rendered from a test store) with many topics, some off-screen, then apply
  an update that closes several topics at once; capture frames (e.g. a
  screencast or rAF-timed screenshots) in light and dark, before and after the
  fix, and state what the frames show. If you cannot reproduce, say so and
  report the most likely cause with evidence instead of guessing a fix.
- Tests above pass; add a test where the cause is testable in Go (asset/CSS
  pins as in rail_offsets_test.go, orbit_css_test.go).

## Return

Commit on `delegate/board-fold-flicker` (this plan rides the commit). Do not
merge or push. Report the cause with evidence, the fix, the commit range,
verification output (including what the captured frames show), and anything
unverified.
