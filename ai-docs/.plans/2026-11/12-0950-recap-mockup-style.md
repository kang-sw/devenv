# Whiteboard topic recap: match the agreed mockup

## Outcome

The topic recap (261111-feat-ws-whiteboard-topic-recap, merged ea7de3f5e)
renders differently from the mockup the user approved, and the user noticed
(2026-11-12). The mockup is at
`/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/recap-mockup.html`
(light and dark, `.rc` is the recap). Today's implementation is
`agents-plugin-tool/internal/wswhiteboard/assets/page.css` `.trecap` (around
L301-308) and the `recap` template in `assets/page.tmpl` (L27-31), plus any
dashboard handling in `agents-plugin-tool/internal/wsdashboard/assets/cards.js`.

Make the recap match the mockup:
- Inset block inside the card, not a full-width strip: margin about 10px 12px 0
  (aligned to the card's content gutter, whatever the real page's gutter is),
  padding about 7px 10px, 3px left accent border in the accent colour, wash
  background, rounded right corners (0 8px 8px 0), no bottom border line.
- Accent colour: the mockup uses the policy orange (`--wb-policy`-family, the
  lighter accent in dark). Use an existing `--wb-*` token; add one for both
  themes only if none fits.
- Time label on the right: "recap <relative time>" (e.g. "recap 12분 전"),
  with the absolute local date-time as the hover title, refreshed while the
  page is open (decision (297)). Today it prints the clock (HH:MM) like the
  topic bar. Put the "recap" word through wsT/i18n (add a key to every table
  under `internal/wsi18n/tables/`, keeping all tables on the same key set,
  and regenerate any generated subset the tests check). If the topic bar's
  relative-time refresher (cards.js / the `tago` + `data-at` path) already
  does relative time on the dashboard, reuse it; the static file page may keep
  a non-refreshing fallback.
- Inside a closed topic's fold (RecapFolded), keep the same look above the body.
- The embed layout keeps working (recap always included for open topics).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` first (assets ship).
- Quiet tone; `--wb-*` tokens only.
- Another delegate is fixing a board flicker on branch
  delegate/board-fold-flicker, possibly in dashboard.js/cards.js/page.css;
  keep this change to the recap rules so merges stay small.
- A peer session shares /home/swkang/devenv; work only in this worktree.
- Never run go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone/agents-plugin-tool -m 20 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/... ./internal/wsi18n/...`
  and update goldens or snapshots that pin the recap markup.
- `node --check` on changed JS.

## Verification

- Tests above pass.
- Headless Chromium: render a board page with an open topic carrying a recap
  and a closed topic with a folded recap, light and dark, and compare against
  the mockup screenshots side by side; say what still differs, if anything.

## Return

Commit on `delegate/recap-mockup-style` (this plan rides the commit). Do not
merge or push. Report the commit range, what changed, verification output,
and anything unverified.
