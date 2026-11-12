# Plan: conversation strip - interrupt badge to the left end, badges to the right end

## Outcome (user-confirmed, board items 347 a + note, 354 ok)

The floating strip `.cbadges` that straddles the composer's top edge (between the
conversation and the input box) keeps its current floating position and look - the user
explicitly likes it exactly where it is. Only the horizontal arrangement changes:

- The interrupt badge `#ibadge` moves from the centre to the **left end** of the strip.
- The badge group `.sbwrap` (subagent badge `.sbadge`, background badge `.bgbadge`, and any
  later sibling badges) moves to the **right end**, badges kept as separate pills (347 a).
  Order inside the group stays: subagents, then background.
- The hidden interrupt badge must still not shift the other badges sideways when the lead
  starts/stops working (today it keeps its slot via `visibility:hidden`; keep an equivalent).
- On narrow widths the right group still shrinks (the subagent label ellipsizes, as today)
  rather than overlapping the interrupt badge.

## Where

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`
  - L320-330 comment + `.cbadges` grid (`grid-template-columns: minmax(0,1fr) auto minmax(0,1fr)`);
    e.g. `auto minmax(0,1fr)` with `.ibadge` in column 1 (`justify-self:start`) and the badge
    group in column 2 (`justify-self:end`). Choose what is simplest and correct.
  - `.ibadge` (L331, `grid-column: 2`), `.sbadge` (L340, `grid-column: 3; justify-self:start`),
    `.sbwrap` (~L1526, `grid-column: 3; justify-self:start`).
  - The support-chat composer (`.composer.supcomp > .cbadges > .sbadge`, built in
    dashboard.js ~L3729, no interrupt badge, CSS override ~L1235): its lone subagent badge
    should also sit at the right end, consistent with the main composer.
  - Update the comment describing the strip layout.
- No markup change is expected (`dashboard.tmpl:21`, `:154`); `pages_test.go:254-255` and
  `support_chat_test.go:90` assert markup and should stay green. If you add a CSS assertion,
  keep it to the behavior-bearing rule.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 6 -- test -count=1 ./internal/wsdashboard/...`
  (never run go test locally).
- Headless visual check: playwright-core at
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pw/node_modules/playwright-core,
  chromium ~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome. Render a static page
  loading the real wswhiteboard/assets/page.css + dashboard.css with the strip markup in
  states: lead working + subagents + background; lead idle + subagents; narrow (360px).
  Save screenshots under
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/strip-check/.

## Boundary

Commit on branch `delegate/conv-strip-layout` in worktree
/home/swkang/.ws-worktrees/devenv/aim-hydration-granite (plan file included), one commit,
repository commit rules with `## AI Context`. Do not merge or push. Event cards, new
monitor badge data, and panel auto-collapse are out of scope (separate tickets).
