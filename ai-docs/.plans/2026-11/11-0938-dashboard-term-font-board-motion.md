# dashboard-term-font-board-motion

## Intent

Three dashboard changes the user asked for on 2026-11-11 (whiteboard, with
screenshots).

1. **Terminal glyphs.** The browser terminal card (`assets/terminal.js`
   `start`, xterm `fontFamily` from `--wb-mono`, defined in
   `internal/wswhiteboard/assets/page.css` as a system monospace stack)
   shows tofu boxes for a powerline / Nerd Font prompt (oh-my-posh style
   segment separators, branch and folder icons). The dashboard embeds no
   font today. The user asked to ship a Nerd Font family font with the
   terminal.
2. **Board card updates are not smooth.** The whiteboard topic cards in the
   board column jump when the board updates. Topic motion already landed
   (`assets/dashboard.js` `topicHeights` / `topicMotion` / `growTopic`, from
   `feat/wb-topic-motion`), and the live daemon runs a develop build that
   includes it, yet the user sees no smoothness.
3. **Conversation | Files tabs move into the conversation panel.** Today a
   tab row at the top of the left column ("대화 | 파일") switches between
   the agents-plus-conversation view and the Files pane. The user wants that
   top row removed, and a "대화 | 파일" tab switch in the conversation
   panel's own header (the panel at the lower left, whose header today
   reads "대화" with the "사용자 N" and mail badges at its right). The
   agents panel above stays visible in both tabs; the Files pane takes the
   conversation panel's body area when its tab is active. The header's
   right-side badges (message counts) are conversation-specific: when the
   Files tab is active they are replaced by a Files-relevant indicator.

## Scope

### 1. Nerd Font symbols for the terminal

- Lead's technical call: ship **Symbols Nerd Font Mono** (the symbols-only
  Nerd Fonts font) as a fallback after the existing mono stack, not a full
  patched text font. Letters keep the user's system monospace; only the
  private-use glyphs come from the embedded font, and the asset is far
  smaller than a patched family.
- Get the font from the official Nerd Fonts release
  (github.com/ryanoasis/nerd-fonts, a pinned release tag, the
  `NerdFontsSymbolsOnly` archive). Prefer woff2; convert from ttf only if a
  woff2 is not published and a converter is available without installing
  heavy tooling; otherwise ship the ttf. Include its license text (and the
  Nerd Fonts license notice) next to it, the way `assets/xterm/` carries
  LICENSE files. Record the release tag and the file's sha256 in a README
  beside it, as `assets/xterm/README.md` does.
- Serve it from the dashboard's existing embedded asset route (find how
  `xterm.js` / `xterm.css` are embedded and served; follow that pattern and
  any asset allow-list, cache header, or CSP `font-src` the page needs).
  The font must load only for pages with a terminal (`data-term`), or at
  least never block the board's first paint: an `@font-face` with
  `font-display: swap` scoped to the terminal is fine.
- xterm measures its cell size from the font at `open`, so the terminal
  must not measure before the font is ready: wait for
  `document.fonts.load(...)` (with a short timeout fallback) before
  `new Terminal` / `open`, or re-measure and refit once it loads.
- Terminal `fontFamily`: the mono stack, then the embedded symbols family.
- Check the binary-size impact and report it.

### 2. Smooth board card updates

- First find out **why** the user sees no smoothness, from the code. Likely
  causes to check (not conclusions):
  - Topics are ordered by latest update (`52d28419d`, wswhiteboard): an
    updated topic moves to the top, so every update reorders cards, and
    `growTopic` animates only height, never position. Kept sections shift
    instantly.
  - An updated topic is a replaced section (`morphBoard` inserts a new node
    when its HTML changed), so a body frame inside it reloads and flashes.
  - The motion may be skipped in practice (`still`, the editing deferral,
    `before` captured after layout already changed, the closed-topic
    pages, or cards woven by `WsCards.settle`).
- Then make updates move smoothly with a FLIP-style position animation
  (record each topic section's and each board card's rect before the swap,
  translate from old to new position after it) combined with the existing
  height growth. Keep the existing rules: no motion on first load, on a
  deferred (`still`) swap, under `prefers-reduced-motion`; never move a kept
  node in the DOM just to animate it (moving a frame reloads it); the
  animation leaves no fixed height or transform behind. Keep it quiet:
  about 200 ms ease-out, as `growTopic` uses.
- If an updated topic's body frame visibly reloads, keep that out of scope
  unless the fix is small and local; report it either way.

### 3. Conversation | Files tabs in the conversation panel header

- Remove the left column's top tab row. Put two tabs, "대화" and "파일"
  (use the existing i18n keys the top row uses; add keys only if needed,
  through the project's i18n catalog, `internal/wsi18n` or wherever the
  current labels live), in the conversation panel header in place of its
  plain "대화" title.
- The agents panel stays where it is in both tabs. The Files pane renders
  in the conversation panel's body area while its tab is active; the
  conversation keeps its state (scroll, pin, unread dot, held bubbles,
  composer draft) while hidden, as it does today when the top-row tab
  switches.
- Keep every behavior the top row carried: the unread dot on the
  conversation tab when it is hidden (`ctabDot`), the Files pane's lazy
  first load, the remembered tab, keyboard shortcuts if any, and the narrow
  (single-column / mobile) layout's tab behavior. Read how `setTab`,
  `TAB`, `tabShown`, `tabDot`, `ctabDot`, `fpane`, and `convo` interact
  before changing them, and keep the narrow layout working.
- Badges: while the conversation tab is active, the header shows today's
  badges unchanged. While the Files tab is active, they are replaced by a
  Files indicator. Lead's technical call: the Files pane's current root
  (folder name, full path in the tooltip) plus its working-tree change
  count when the pane already knows it; reuse what the Files pane header
  already shows rather than computing anything new, and move rather than
  duplicate it if the Files pane header already carries that root line.
  If neither value is available cheaply, show the root name only and say
  so in the report.
- Server-rendered HTML for the panel header lives in the dashboard template
  or Go renderer; change it there if the tabs need server markup, keeping
  region names and SSE region contracts unchanged.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the assets ship
  downstream). Read it before editing.
- A peer session is working in `/home/swkang/devenv` on develop and touches
  the topic-note code in `dashboard.js` (about lines 770-810). Do not edit
  that block.
- Never run heavy go build, test, or vet locally (it froze the machine).
  Use `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test
  ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/wsi18n/
  ./cmd/ws-mcp/`. Light local checks (`node --check`) are fine with
  `nice -n 19`.
- Never push.

## Verification

- Remote Go tests above pass (including any asset embedding, template, or
  route test you add or update).
- `node --check` on every edited JS file.
- Report what you could not verify in a browser (the lead and user check
  visually after merge and reinstall).

## Boundary

Commit on branch `fix/dashboard-term-font-board-motion` in this worktree,
one commit per change. Return to the lead without committing a change if it
needs a new config knob, a change to SSE region names or the wire contract,
or a font asset larger than 2 MB.
