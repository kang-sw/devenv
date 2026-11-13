---
title: Werkschaft project identity and the devenv rename
---

# Werkschaft project identity and the devenv rename

## Background

The project is being reworked from a personal `devenv` (dotfiles plus the
`ws` workflow) into a product identity of its own. A 2026-11-13 lead session
settled the name and the corporate identity (CI), reframed the README around
it (`b1d2c59a6`), and unified the license declarations (`9f6032cbf`). The
GitHub repository rename and the dotfile split remain with the owner; this
ticket records what was decided and what the rename will touch.

## Naming

`ws` expands to **Werkschaft**: German `Werk` (work) plus the collective
suffix `-schaft` (as in `Mannschaft`, `Gesellschaft`), read as "work
collective". It keeps the existing `ws` / `wsflow` package names valid
without a rename of the packages themselves.

The name came out of a "Wotan-shaft" sound the owner liked. The Wotan
lore (Wotan's spear shaft carrying carved contracts; a lead bound by its own
contracts acting through free agents) and an angular Sowilo rune as the `S`
were explored and then dropped wholesale: a German name plus Wotan plus an
angular lightning rune can read as far-right iconography (the SS "Sig rune";
Wotanism / Wotansvolk). The identity is work-themed instead.

## Corporate Identity

Assets live in `ai-docs/ref/brand/`:

- `werkschaft-mark.svg` - the symbol: a navy rounded tile with a cream `W`
  and `S`, where every down-right (`\`) stroke is a detached vermilion piece
  (the two `\` strokes of the `W` and the spine of the `S`).
- `werkschaft-light.svg` / `werkschaft-dark.svg` - the horizontal lockup
  (symbol plus monoline `WERKSCHAFT` wordmark) with a transparent background;
  the dark variant only switches the wordmark ink from navy to cream.
- Wordmark rule: the lower-right legs of `R`, `K`, and `A` are detached
  vermilion strokes at one shared angle ("letters under assembly"). The `K`
  leg is extended upward along that angle so the letter still reads as `K`;
  `R` and `A` keep the shorter leg.
- Palette: navy `#1E2235`, cream `#F2EBDD`, mustard `#F3A712`, vermilion
  `#E4572E`.

The README shows the lockup through `<picture>` with a
`prefers-color-scheme: dark` source, because a baked-in background reads as a
bright band in GitHub dark mode.

## Outcome Ledger

### Verified Findings

- The Go module path is `github.com/kang-sw/devenv`; `kang-sw/devenv`
  appears in 406 tracked files outside tickets, plans, archive, and
  CHANGELOG, almost all of them Go imports.
- The marketplace id `kang-sw-devenv` appears in 16 tracked files, including
  both marketplace manifests, the three `ws-mcp-launcher.py` copies
  (`agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-pi/`),
  `agents-plugin-tool/cmd/ws-mcp/proxy.go`, and
  `agents-plugin-tool/internal/wsstate/paths.go`. It is part of installed
  plugin ids (`ws@kang-sw-devenv`) and of per-plugin cache paths such as
  `~/.cache/ws@kang-sw-devenv/`.
- `~/devenv` appears in 3 tracked files (the workstation install path).
- Pi installs pin `git:github.com/kang-sw/devenv@<tag>`; a GitHub repository
  rename redirects git and web URLs from the old path.
- Before `9f6032cbf`, `LICENSE` was Apache-2.0, the four ws/wsflow
  `plugin.json` manifests declared MIT, and the README said "Personal
  configuration. Use freely"; all now declare Apache-2.0.

### Confirmed Decisions

- The project identity is **Werkschaft**, and `ws` stands for Werkschaft.
- No Wotan, Odin, or rune motifs anywhere in the identity, including hidden
  easter eggs.
- The CI is the current `ai-docs/ref/brand/` set described under Corporate
  Identity, kept in `ai-docs/ref/brand/` rather than a new root directory.
- The README leads with the Werkschaft identity; workstation configuration is
  a `Workstation Setup` subsection until the dotfiles move out.
- The README logo is a transparent light/dark pair switched by `<picture>`.
- The project is licensed Apache-2.0, matching `LICENSE`.
- The GitHub repository will be renamed `devenv` -> `werkschaft`, relying on
  GitHub's redirect for existing install references. The owner performs the
  rename on their own schedule.
- The dotfiles (including the Neovim config) will move out of this
  repository entirely; the owner decides when.

### Proposals

- Use `werkschaft-mark.svg` as the dashboard favicon and plugin icon.

### Open Questions

- Whether the marketplace id `kang-sw-devenv` changes with the repository
  rename. Changing it changes installed plugin ids and cache paths, so it is
  a cross-version surface that needs a compatibility tier (T1 with an alias
  path, or T2) before it is scheduled.
- Whether the Go module path follows the rename; GitHub redirects keep the
  old import path resolvable, so it can stay.
- Where the dotfiles go, and what `install.sh` becomes once they leave.

### Rejected Alternatives

- **Wotanschaft as a hidden easter-egg name** (logo file name and `<title>`
  tooltip): dropped with all Wotan motifs because of the Wotanism
  association.
- **"Wotan's Shaft" as the expansion**: English slang "get the shaft" and the
  same Wotanism association.
- **`Speer` (spear) in the name**: Albert Speer association.
- **An angular Sowilo rune as the `S`**, in the wordmark or the mark: same
  form as the SS sig rune.
- **Workstream as the expansion**: reads clearly but is a generic business
  term (a same-named company was recalled but not verified); the owner
  preferred Werkschaft.
- **Woven Staff**: kept the sound but lost the work meaning.
- **Mark candidate with a plain mustard `S` and no vermilion**: legible at
  16 px but carried none of the wordmark's detached-leg idea.
- **Mark candidate with three detached legs as a hatch under `WS`**: the
  hatch collapses into dots at 16 px.
- **Mark candidate detaching the last `W` stroke**: read as `W/S`.
