---
title: xterm.js font ligature support
sage-review-design: required
related:
  260725-feat-prefs-portability: classifies terminal style as `portable` scope, so a
    pref-gated variant rides that registry for free only if the field lands in
    `TerminalStylePrefs` rather than a separate store
  260725-feat-dashboard-terminal-steady-state-stream-throughput: live throughput work
    on the same per-frame render path a character joiner taxes
---

# xterm.js font ligature support

## Background

The dashboard terminal (xterm.js) does not render programming ligatures. Users
with a ligature-capable coding font would benefit from `->`, `=>`, `!=`, `>=`
etc. rendering as ligatures.

**Correction (2026-07-26, grounded in source).** This ticket originally claimed
"the current Nerd Font stack largely includes ligatures." That is false for the
entry that actually resolves: `TERMINAL_FONT_FALLBACK_STACK`
(`frontend/src/terminalPrefs.ts:9-12`) **leads** with `"MesloLGS NF"`, a
Menlo/Bitstream Vera derivative with no programming-ligature `calt` set. CSS
font resolution stops at the first installed family, and MesloLGS NF is exactly
what Powerlevel10k installs, so on a typical dogfood box the resolved font is
the non-ligature one. Only the *later* entries (JetBrainsMono / CaskaydiaCove /
FiraCode NF) carry ligatures. Ligatures are therefore effectively
override-only unless the default stack is reordered.

## Assessment (2026-07-26)

Assessed against the pinned versions on request ("is this a fast hotfix?").
**Answer: small diff, but not a drop-in — it needs owner decisions first, and
one of its two success conditions cannot be established statically.**

Diff size: ~30-45 lines / 2 files unconditional; ~80-110 lines / 6 files
pref-gated.

### Cleared (verified, so do not re-litigate these)

- **The addon's own README is stale.** It claims Node.js/Electron-only, but
  `@xterm/addon-ligatures@0.10.0`'s source resolves fonts through the browser
  Local Font Access API (`src/font.ts:36-85`: `navigator.fonts.query()`, then
  `window.queryLocalFonts()`) and imports `font-finder` nowhere in `src/`.
- **No build break.** The Node-only `font-finder`/`get-system-fonts` code *is*
  physically inlined in the shipped `lib/addon-ligatures.mjs`, but every
  `require("fs"|"path"|"os")` site sits inside an esbuild lazy `__commonJS`
  factory the browser path never invokes, and they are runtime `__require`
  calls rather than static imports — so Rollup/Vite never try to resolve them.
- **No bundler config change.** `vite.config.ts:16-18` already routes
  `/node_modules/@xterm/` into the `xterm` manual chunk.
- **Dependency tree is clean:** 9 pure-JS packages, 0 vulnerabilities, no native
  builds, no `postinstall` (its `prepare: node bin/download-fonts.js` never runs
  — `bin/` is not shipped).
- **Both pinned renderers support character joiners:** `getJoinedCharacters`
  is present in `addon-webgl@0.18` and `addon-canvas@0.7`, and joins are split
  at fg/bg attribute boundaries, so a color boundary mid-`->` is not mispainted.

### Real risks

- **`fontFeatureSettings` is very likely a no-op on the pinned renderers.** The
  addon's only glyph-shaping lever is
  `terminal.element.style.fontFeatureSettings = '"calt" on'`
  (`src/LigaturesAddon.ts:44`). That string occurs **zero** times in
  `@xterm/xterm@5.5.0`, `addon-webgl@0.18`, and `addon-canvas@0.7`. The texture
  atlas rasterizes into a detached `createElement("canvas")` with no
  `getComputedStyle`, so it does not inherit CSS from `terminal.element`.
  Whether ligatures visibly render under the app's active WebGL renderer
  therefore depends on Chromium's default Canvas2D `fillText` shaping. **This is
  not statically determinable and Playwright cannot assert glyph shaping**
  (`playwright.config.ts:22` is headless; every existing terminal assertion is
  `.xterm-rows` text or row counts). It must be verified by eye on the Windows
  dogfood box before this ticket can claim success.
- **Packaging asymmetry / Node-resolution trap.** Unlike every other pinned
  addon, `@xterm/addon-ligatures@0.10.0` ships `main: lib/addon-ligatures.js`
  **which does not exist in the tarball**, plus a `module:` entry that does.
  Bundler resolution works; any *Node-side* resolution hits
  `ERR_MODULE_NOT_FOUND`. This is latent today because
  `tsconfig.route-tests.json`'s explicit include list omits
  `terminalPaneBody.tsx` — but it *includes* `settingsSections.tsx` and
  `terminalPrefs.ts`, which run under `node` via `npm run test:settings`. So the
  pref-gated variant must keep the addon import out of both.
- **Font-blind fallback joiner.** When the Font Access API is unavailable or
  denied, the addon joins a hardcoded 68-pattern Iosevka list regardless of what
  the resolved font supports (`src/LigaturesAddon.ts:26-33`, `src/index.ts:85-97`)
  — including `==`, `!=`, `::`, `/*`, `*/`, `<>`, `->`. Usually visually
  identical, but it is a real change to the render path for very common terminal
  output.
- **Chromium permission prompt.** `window.queryLocalFonts()` triggers a
  user-facing "use your system fonts" prompt on first terminal open. Secure-context
  caveat: the daemon's `127.0.0.1` default *is* a secure context, but the
  LAN/gateway `bind_mode` path (`crates/daemon/src/server.rs:55-61`) over plain
  HTTP is not — there `queryLocalFonts` is undefined and the font-blind fallback
  list applies. Degrades gracefully, no crash.
- **Per-frame CPU on a path with active optimization work.**
  `getJoinedCharacters` runs a cell loop plus the joiner handler per attribute-run
  per line per render; the fallback path is O(text × 68 patterns). This taxes the
  same path `260725-feat-dashboard-terminal-steady-state-stream-throughput`
  (`ready/`) is currently optimizing — sequence the two deliberately.
- **Process gate:** this ticket carries `sage-review-design: required`, so ready
  promotion is blocked until design review runs regardless of how small the diff
  is.

## Decisions

- Use the official `@xterm/addon-ligatures` addon (hooks xterm's character-joiner
  API); it composes with the active renderer. No custom glyph shaping needed.
- Load the addon **after** `terminal.open(container)`
  (`terminalPaneBody.tsx:198`): `activate()` hard-throws
  `'Cannot activate LigaturesAddon before open is called'` when
  `terminal.element` is undefined. Wrap in `try/catch` and dispose in the
  mount-effect cleanup next to the existing renderer/unicode disposals
  (`:567-577`), mirroring the `unicode11` pattern at `:244-251`.

## Open Decisions (owner)

None of these are resolved; each changes the shape of the edit.

1. **Pref-gated or unconditional?** If gated, accept that the toggle is *not* a
   `terminal.options.X =` assignment like the other three terminal prefs
   (`terminalPaneBody.tsx:760-764`): it requires constructing/disposing the addon
   at runtime plus re-running the WebGL reactivation from (2) inside that same
   effect, and it touches `terminalPrefs.ts`'s schema (versioned `v1` storage key,
   tolerant parse guard needed) and its two exhaustive test literals
   (`terminalPrefs.test.ts:70-74`, `:96-100`).
2. **Renderer ordering.** The addon's own typings state that "if webgl is also
   being used, that addon should be reactivated after ligatures is activated in
   order to apply `fontFeatureSettings` to the texture atlas." Either insert
   ligatures between `terminal.open()` (`:198`) and the renderer chain (`:219`),
   or keep the chain and add a WebGL re-activation step — which must also be
   replayed inside the `onContextLoss` handler (`:224-230`), the one block in
   this file explicitly documented as load-bearing for GPU-context recovery.
3. **Suppress the font-blind fallback joiner?** `new LigaturesAddon({
   fallbackLigatures: [] })` makes the feature strictly font-driven, avoiding
   "merges `==`/`/*`/`::` in every font" at the cost of doing nothing when the
   permission is denied or the app is served over LAN HTTP.
4. **Reorder the default font stack?** Ligatures are near-pointless while
   `MesloLGS NF` leads `TERMINAL_FONT_FALLBACK_STACK`. Either reorder (a visible
   default-appearance change for all users) or accept ligatures as
   override-only.

## Constraints

- Ligatures depend on the resolved font actually containing them; this is opt-in
  by font.
- Visible-glyph verification is manual-only on the Windows dogfood box. Do not
  write a Playwright assertion that claims to cover it.

## Phases

### Phase 1: Enable ligatures via the xterm ligatures addon

Add `@xterm/addon-ligatures@^0.10.0` to `frontend/package.json` (alongside the
existing `@xterm/*` pins), load it against the terminal instance in
`terminalPaneBody.tsx` per the Decisions above, and resolve Open Decisions 1-4
before writing code.

Verification boundary: with a ligature-capable font active, `->`/`=>`/`!=`
render as ligatures in the terminal on the Windows dogfood daemon (**manual,
by eye — headless Playwright cannot assert glyph shaping**); non-ligature fonts
render unchanged; if a pref is added, toggling live-applies without a terminal
remount and `npm run test:settings` still passes under `node` (i.e. the addon
import did not leak into the route-tests include list). Explicitly not covered:
whether the WebGL texture atlas honours `fontFeatureSettings` — that is the
open risk this verification exists to settle, and a negative result means the
renderer-ordering decision (2) or a renderer change is required, not that the
phase is done.
