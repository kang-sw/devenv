---
title: "Research: npm distribution for the ws Pi adapter (agents-plugin-pi)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: design anchor for the Pi adapter this ticket packages
  260902-feat-ws-pi-native-mvp: the MVP whose composition is proven but which ships dev-load (-e) only
related-mental-model:
  - plugin-runtime
  - claude-compatibility
spec:
  - pi-adapter-runtime
---

# Research: npm distribution for the ws Pi adapter (agents-plugin-pi)

## Background

`260902-feat-ws-pi-native-mvp` proved the ws-pi-native composition (bridge +
spawner + model-catalog + `/ws-discuss` PoC) end-to-end, but the package ships
**dev-load only** — it is consumed via `pi -e agents-plugin-pi/src/index.ts` from
a checkout of this monorepo. `package.json` is `"private": true` with no publish
metadata, and one load-bearing path (skills exposure) resolves to a *sibling
package inside this repo*, so the package is not installable standalone.

The user's direction (2026-09-03): fold npm distribution into the epic now and
make subsequent Pi-adapter work **distribution-aware** from here on, rather than
bolting packaging on after the fact. This ticket collects the gaps and the open
design forks so that decision can be made deliberately; it does not itself change
code.

The golden rule still holds throughout: ws-mcp Go source is never modified for
Pi; the dependency stays one-directional (adapter → ws-mcp). Distribution must
not introduce a reverse dependency.

## Current distribution posture (evidence)

- `agents-plugin-pi/package.json`: `name: "ws-pi-bridge"`, `version: "0.1.0"`,
  **`private: true`**; declares only `pi.extensions: ["./src/index.ts"]` and a
  `test` script. No `files`, `exports`, `publishConfig`, `license`, `repository`,
  or `engines`.
- Self-contained (distribution-aware) copies already exist for the runtime
  substrate: `bin/ws-mcp-launcher.py`, `runtime.json`, and `rsrc/` are
  byte-identical hand-synced copies of `agents-plugin/`'s versions, because the
  launcher resolves those trees relative to its own package dir at runtime
  (`src/index.ts` HAND-SYNC NOTE; spec `{#260903-pi-adapter-package-topology}`).
- **NOT self-contained: the ws skills tree.** `src/index.ts:58` computes
  `skillsDir = join(repoRoot, "agents-plugin", "skills")` and `resources_discover`
  returns `{ skillPaths: [skillsDir] }` (`:67-68`). There is no
  `agents-plugin-pi/skills/` copy. The `:14-17` comment records this as a
  deliberate monorepo choice ("pointing at the existing directory directly
  rather than copying it … unlike bin/ws-mcp-launcher.py + runtime.json, which
  have repo precedent for copying"). On a standalone npm install
  `<repoRoot>/agents-plugin/skills` does not exist, so skill exposure — and thus
  `/ws-discuss` and every `/skill:*` — breaks.
- Source is TypeScript consumed via Node native type-stripping (no build step);
  the whole package assumes a Node that supports `--experimental-strip-types`.

## Distribution gaps to resolve

1. **Skills bundling (the #1 blocker).** Decide how a published adapter carries
   the ws skills tree (see Fork A). Whatever the choice, `resources_discover`
   must resolve to a path that exists in the installed package, not a sibling
   repo path.
2. **Tarball contents + publish metadata.** Add a `files` whitelist (must include
   the hand-synced `bin/`, `runtime.json`, `rsrc/`, and — per Fork A — possibly a
   skills copy), plus `license`, `repository`, `engines`, and drop
   `private: true` / add `publishConfig`. Verify `pi.extensions` resolves for an
   installed (not `-e`) package.
3. **Hand-sync integrity becomes a release concern.** The launcher/runtime/rsrc
   copies currently drift-protected only by a prose note and manual `diff`. For a
   published artifact this needs a pre-publish check (or generated sync) so a
   stale copy cannot ship.
4. **TS source vs shipped JS (see Fork B).** Either pin an `engines.node` range
   and ship `.ts` relying on type-stripping, or add a build step emitting `.js` +
   `.d.ts`. Interacts with how Pi consumes an installed extension.
5. **Version + pin coherence.** The adapter pins ws-mcp's `runtime.json`
   `plugin_version` on startup and fails loudly on mismatch
   (`{#260903-pi-bridge-version-pin}`). A published `ws-pi-bridge@X.Y.Z` must ship
   a `runtime.json` matching a real, obtainable ws-mcp build, and needs a
   versioning story relating its own semver to the ws plugin version (`0.43.x`)
   — including how the bundled ws-mcp launcher itself is obtained/pinned at the
   consumer.
6. **Pi's installed-extension consumption model (unknown — verify first).** How
   does an end user install and enable a *published* Pi extension (npm package
   name in Pi config? a Pi marketplace? a global install dir)? The MVP only
   exercised `-e` project-local loading. This determines several answers above
   and should be settled empirically against the installed Pi build before the
   forks are decided.

## Open design forks

- **Fork A — skills: copy vs depend.**
  - *Copy* the skills tree into `agents-plugin-pi/` (the bin/runtime/rsrc
    precedent): self-contained, offline-installable, but adds a fourth
    hand-synced copy and inflates the tarball.
  - *Depend* on a separately published ws-skills/ws-core package: single source
    of truth, but requires publishing that package too and a resolution path Pi
    honors, and must not create a reverse dependency into ws-mcp.
- **Fork B — TS source vs build step.** Ship `.ts` + `engines.node` (zero build,
  matches current dev ergonomics) vs. emit `.js`/`.d.ts` (broader Node support,
  but adds tooling this package has so far avoided).

## Working guidance until this is decided

Make ongoing Pi-adapter changes distribution-aware: prefer package-local paths
over repo-relative sibling references for anything the runtime reads; when adding
a new bundled asset, treat it the way `bin/`/`runtime.json`/`rsrc/` are treated
(package-local, hand-sync-noted) rather than reaching across roots. Do not
publish anything until Forks A/B and gap 6 are settled.
