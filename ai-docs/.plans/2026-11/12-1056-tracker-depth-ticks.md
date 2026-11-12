# Plan: tracker hover-insert depth markers - reachable vs blocked levels

## Outcome (user-confirmed design, board items 350/351/356)

In a tracker list view, the hover insert ("+" badge between rows) currently draws uniform
blue 40% ticks under every level shallower than the badge (`.hdep`, a repeating gradient).
Change it so every level slot in the gap's range is drawn individually:

- **Reachable level** (the badge can move there = a depth present in `insOpts(a, b)`):
  an 8px dim gray tick, `color-mix(in srgb, var(--wb-fg) 34%, transparent)`.
- **Blocked level** (in range but not in `insOpts`): a small "x" mark, 8x8, stroke ~1.5px,
  round caps, colored `color-mix(in srgb, var(--wb-fail) 60%, transparent)` (the existing
  failure token; the mockup used #e06c6c at 60%). Red x was chosen over gray x (356 a).
- The badge's own level gets no marker (the badge sits there).
- **Levels deeper than the badge are drawn too ("A" layout, 350 a):** the slots to the right
  of the badge, up to the deepest slot in range, get the same tick/x markers, and the blue
  guide line `.hl` starts after the deepest slot instead of right after the badge. When the
  badge is already at the deepest slot, `.hl` starts right after the badge as today.

Range of level slots: depths `0 .. max` where `max` is `da + 1` when the gap has a row
above (`a`, an `.irow`; `da` its depth), i.e. the theoretical deepest place (first child of
a). For the top-of-list gap (`!a`) the only slot is depth 0. Everything in `0..max` that is
not in `insOpts` is blocked (this covers both levels shallower than the row below's depth
and parents that cannot take children, e.g. done/hidden items under canSplit).

Geometry stays as today: badge at `left: 4px + depth*16px` (16px wide), tick for level d
at `left: 8px + d*16px`, width 8px (same as the current gradient's tick positions); x mark
centered on the same 8px slot. `.hl`: `left = 4px + max*16px + 16px` when `max > pick`,
otherwise the current `4px + depth*16px + 8px`.

Mockup (reference only): /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/mock-tracker-depth/index.html
(middle column "A안 + 막힌 칸 작은 x (빨강)" is the chosen design).

## Where

- `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` ~L4493-4570: `insOpts`,
  `insPick`, `insShow`. Build the markers in `insShow` (it already knows `opts`, the pick,
  and `g.a`); e.g. fill `.hdep` with one span per level, or replace `.hdep`'s gradient with
  child elements. Keep `insOpts`/`insPick` semantics unchanged; `mvSlot` (move drag) reuses
  `insOpts` and is out of scope. Update the block comment above (it describes `.hdep` as
  "drawn from --ind alone").
- `agents-plugin-tool/internal/wsdashboard/assets/tracker.css` L916-924 (`.hins`, `.hl`,
  `.hdep`). Use tokens on the tracker's existing theme variables so light/dark both work
  (same mix ratios). Pointer-events stay none; the hit button `.hhit` and `insWidth`
  unchanged.
- `agents-plugin-tool/internal/wsdashboard/tracker_quickadd_test.go` L161-170 asserts the
  exact `.hins` markup string and the `.hdep` CSS line; update those assertions to the new
  structure (assert the behavior-bearing parts: per-level markers, ok vs blocked classes,
  `.hl` offset rule).

If the hover insert's markup also exists in a file-mode/static renderer, mirror it; if not,
nothing else.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 6 -- test -count=1 ./internal/wsdashboard/...`
  (never local go test).
- Visual: render a small headless Chromium check if practical (playwright-core at
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pw/node_modules/playwright-core,
  chromium at ~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome) or reason the
  geometry against the mockup; report which you did.

## Boundary

Commit on branch `delegate/tracker-depth-ticks` in worktree
/home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone (plan file included), one commit,
repository commit rules with `## AI Context`. Do not merge or push.
