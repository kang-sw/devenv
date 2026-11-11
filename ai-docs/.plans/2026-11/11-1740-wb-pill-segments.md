# Whiteboard scroll pill: per-state sub-capsules

## Outcome

The floating "open question" pill (`agents-plugin-tool/internal/wswhiteboard/assets/pill.js`,
styles in `assets/page.css` around `.oq-pill`) today shows one orange capsule
with text like `↓ 미결 3 / 응답 5 (노트 2)`. Replace it with a multi-segment
capsule. User decisions from the whiteboard (2026-11-11):

- (303) a: a card-coloured outer capsule (`--wb-card` background, keeping the
  current `0 0 0 2px var(--wb-bg), var(--wb-shadow-hi)` ring and shadow)
  holding an arrow glyph in a muted colour and inset sub-capsules, one per
  state:
  - open ("미결 N"): `--wb-policy` / `--wb-on-policy` (unchanged orange);
  - answered ("응답 N"): a green family token (the announced/settled green
    already in the token set if one exists; otherwise add a quiet green token
    pair to the `--wb-*` set for light and dark);
  - noted ("노트 N"): a gray family (muted surface and muted foreground).
  Sub-capsules are smaller than the outer capsule (outer 28px, inner about
  22px, outer padding about 3px right and 10px left, gap about 3px), label
  then a tabular-nums count.
- (304) ok: notes stay a subset of answered (the count of answered items with
  a note, as today), drawn as a gray segment right after the answered segment.
  A segment whose count is 0 is not rendered.
- (305) a: each segment is its own click target. The open segment scrolls to
  the nearest still-open item at that edge, the answered segment to the
  nearest answered item, the noted segment to the nearest answered item with
  a note. "Nearest" keeps today's rule (top edge: last in list; bottom edge:
  first). Each segment is focusable (a button) with a visible focus ring.

Reference mockup (light and dark; variant A is the chosen one):
`/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pill-mockup.html`.

## Constraints

- Quiet dashboard tone: `--wb-*` tokens only; define any new token for both
  themes wherever the existing tokens are defined.
- i18n: keep using `wsT`. The keys `js.pill.open` ("미결 {0}"),
  `js.pill.answered`, `js.pill.noted` ("(노트 {0})") embed the count and
  parentheses today. Render label and count separately; add or reword keys
  in every table under `agents-plugin-tool/internal/wsi18n/tables/` (all
  tables keep the same key set), and regenerate any generated subset or
  mockup file the tests check (the dashboard mockup `i18n-en.js` comes from
  `wsi18n.SubsetScript`). Keep `js.pill.up` / `js.pill.down` as tooltips,
  adapting the wording per segment if you add per-segment titles.
- `retitle` and the IntersectionObserver logic stay as they are.
- The whiteboard file page and the dashboard board both use this script and
  page.css; the embed (`body.embed`) returns early and stays unchanged.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (the
  assets ship).
- A peer session shares the main checkout; work only in your worktree.
- Never run go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C <worktree>/agents-plugin-tool -m 20 -- test -count=1 <pkgs>`.
  Run at least ./internal/wswhiteboard/... ./internal/wsi18n/...
  ./internal/wsdashboard/... plus any golden or render test that snapshots
  the pill markup.

## Verification

- The tests above pass; update tests or goldens that pin the old pill text.
- `node --check` on pill.js.
- If a headless browser is available, render a board page with open,
  answered, and noted items scrolled off-screen and capture the pill in light
  and dark; otherwise say so in the report.

## Return

Commit on `delegate/wb-pill-segments` (this plan file rides the commit). Do
not merge or push. Report the commit range, what changed, verification
output, and anything unverified.
