---
title: xterm.js font ligature support
sage-review-design: required
---

# xterm.js font ligature support

## Background

The dashboard terminal (xterm.js) does not render programming ligatures. Users
with a ligature-capable coding font (the current Nerd Font stack largely includes
ligatures) would benefit from `->`, `=>`, `!=`, `>=` etc. rendering as ligatures.

## Decisions

- Use the official `@xterm/addon-ligatures` addon (hooks xterm's character-joiner
  API); it composes with the active renderer. No custom glyph shaping needed.

## Constraints

- Verify compatibility with the renderer currently in use (WebGL/canvas) — the
  ligatures addon works via `registerCharacterJoiner`; confirm behavior under the
  active renderer during implementation.
- Ligatures depend on the resolved font actually containing them; this is opt-in
  by font. Consider whether to gate behind a terminal-style pref (see
  `terminalPrefs.ts`) or enable unconditionally.

## Phases

### Phase 1: Enable ligatures via the xterm ligatures addon

Add `@xterm/addon-ligatures`, load it against the terminal instance, and verify
ligature rendering with a ligature-capable font under the active renderer. Decide
during implementation whether to expose an on/off pref in the Terminal settings
section or enable unconditionally.

Verification boundary: with a ligature font active, `->`/`=>`/`!=` render as
ligatures in the terminal; toggling (if a pref is added) works; non-ligature
fonts render unchanged.
