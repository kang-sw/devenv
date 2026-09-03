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
(package-local, hand-sync-noted) rather than reaching across roots. The spike
below settles Forks A/B and gap 6; the one remaining publish blocker is the
skills-path fix.

## Spike resolution (gap #6 + Forks A/B) — 2026-09-03

An empirical spike against the installed Pi build (`@earendil-works/pi-coding-agent@0.84.4`)
settled the two forks and gap #6. Evidence is `pi --help`, `docs/packages.md`,
`docs/extensions.md`, `dist/core/extensions/loader.js`,
`dist/core/package-manager.js`, and live `session_start` probes.

- **gap #6 — installed-extension consumption: RESOLVED.** Pi has a first-class
  package manager: `pi install npm:<name>` / `git:` / URL / local path, writing a
  `packages` array into user settings `~/.pi/agent/settings.json` (or project
  `.pi/settings.json` with `-l`, auto-installed on trust). A published package
  declares resources via a `package.json` `"pi": { extensions, skills, prompts,
  themes }` key (glob + `!exclude`), or via auto-scanned convention dirs
  (`extensions/`, `skills/`, `prompts/`, `themes/`). Installs run
  `npm install --omit=dev`. `-e` stays the "try without installing" path.
- **Entry-contract parity `-e` vs installed: IDENTICAL.** Both routes converge on
  the same jiti loader and the same default-exported `function(pi)` factory;
  `registerCommand`/`registerTool`/`on`/`resources_discover` behave identically,
  and every `pluginDir`-relative asset (`bin/`, `runtime.json`,
  `model-catalog.json`, `rsrc/`) resolves the same when installed
  (`import.meta.url` points at the real on-disk file either way). **The single
  divergence is the skills path** (`src/index.ts:57-58`
  `repoRoot/agents-plugin/skills`), which walks *out of* the package to a sibling
  repo dir that does not exist in an installed tarball → the #1 blocker, as
  gap-list item 1 predicted.
- **Fork B — TS vs build: SETTLED = ship raw `.ts`.** Pi loads every extension
  through jiti with type-stripping (`docs/extensions.md:185`; `loader.js` uses
  `createJiti`), same path for `-e`/manifest/convention. No build step. Caveat:
  runtime third-party deps must be in `dependencies` (installs are `--omit=dev`),
  and Pi core stays a `peerDependencies: "*"`, not bundled.
- **Fork A — skills copy vs depend: SETTLED = copy (package-local).** Bundle a
  package-local `skills/` populated at pack time from `agents-plugin/skills`
  (joining the existing hand-synced `bin/`/`runtime.json`/`rsrc/` copies), and
  expose it either by declaring `"pi": { "skills": [...] }` in `package.json`
  (auto-discovered, zero code) or by pointing `resources_discover`'s `skillPaths`
  at the `pluginDir` copy instead of `repoRoot`. Depend-on-a-separate-package is
  unnecessary complexity and is dropped.
- **pi CLI resolution for spawned children: current pattern already correct.** The
  spawner re-invokes the CLI via `process.argv[1]` (the running pi launcher) with
  `process.execPath` (`src/spawner.ts:270-279`) — install-safe for both `-e` and
  installed. `require.resolve("@earendil-works/pi-coding-agent")` FAILS from an
  installed extension (probe-confirmed), so never resolve the CLI that way. If a
  later slice switches to `RpcClient`, its default `cliPath` is the literal
  `"dist/cli.js"` (won't resolve when installed) — pass an explicit `cliPath`
  derived from `argv[1]`, or use the package's `./client` / `/rpc-entry` exports.
  Minor: the reference example's Bun-virtual-script (`/$bunfs/root/`) guard was
  dropped from the current spawner as dead code; restore it only if a
  compiled-binary pi distribution is ever targeted.

### Remaining publish work (mechanical, no open design)

1. **Skills bundling (the one blocker). — DONE (impl/track/pi-agent, commit
   279f501a; spec d327de64).** Shipped as a `pluginDir`-relative package-local-first
   resolver (`src/skills-dir.ts::resolveSkillsDir`) + a pack-time `prepack`/`prepare`
   copy script (`scripts/copy-skills.mjs`) that generates a gitignored
   `agents-plugin-pi/skills/`, plus a `files` whitelist that ships it. The
   `"pi": { "skills": [...] }` manifest alternative was not used; the resolver path
   keeps dev `-e` working via the canonical-tree fallback.
2. **Publish metadata (partly landed).** The `files` whitelist landed with item 1
   (bin/, runtime.json, rsrc/, skills/, model-catalog.json, src/). Still to do for
   an actual publish: move any runtime deps to `dependencies`, Pi core to
   `peerDependencies: "*"`, drop `private: true` (deliberately kept for now — no
   publish until features 2/3 are discussed), add `license`/`repository`/`engines`.
3. **Hand-sync drift guard — DONE for skills.** The pack-time copy script
   regenerates `skills/` on every pack, so it cannot drift; the three original
   hand-synced copies (`bin/`/`runtime.json`/`rsrc/`) remain prose-note-guarded as
   before.

Nothing in gaps/forks now forces a feature-code rewrite: the entry contract is
install-identical and the CLI-resolution pattern is already correct, so
`260903-feat-ws-pi-subagent-rpc-ux` and `260903-feat-ws-pi-goal-loop-compaction-hook`
can be built under `-e` and will run unchanged once installed, provided the
package-local-paths discipline holds.

## Consumption model + publish path — decided npm-first (2026-09-03)

A follow-up read of Pi `docs/packages.md` (git-source section) surfaced a
repo-layout constraint the gap #6 spike did not cover:

- **`pi install git:<host>/<user>/<repo>` targets the repo ROOT.** Pi clones the
  whole repo to `~/.pi/agent/git/<host>/<path>` and runs `npm install` only if a
  **root `package.json`** exists; the git spec supports refs (`@tag`/`@commit`)
  and `git@`/`ssh://` shorthand but **no subdirectory / subpath / `#fragment`**
  syntax. This package's `package.json` lives at `agents-plugin-pi/`, a monorepo
  **subdirectory**, so a bare `pi install git:github.com/<user>/<devenv-repo>`
  cannot discover it.

Three consumption paths follow, with the decision:

- **npm publish → `pi install npm:<name>` — CHOSEN target.** Cleanest canonical
  path; independent of monorepo layout. Deferred until after features 2/3 land
  (no rush to publish).
- **Dedicated repo whose root IS the package → `pi install git:...` — rejected
  for now.** Would need a repo split / subtree / mirror; heavier than npm publish
  and buys nothing npm publish doesn't.
- **Local path → `pi install ./agents-plugin-pi` (or abs path) — works today**
  (in-place reference; the package-local-first resolver's canonical-tree fallback
  finds the sibling `agents-plugin/skills` from the checkout). This is the
  dogfooding install path until publish.

### npm publish prerequisites (for the eventual publish slice)

- Drop `private: true` (npm refuses to publish a private-marked package).
- Auth: an npm account + `npm login` (web flow) **or** a publish token in
  `.npmrc` (`//registry.npmjs.org/:_authToken=`); publish typically needs
  2FA/OTP, or an **automation token** to bypass it in CI.
- Name policy: unscoped `ws-pi-bridge` must be globally unique (verify
  availability); a scoped name (`@<scope>/ws-pi-bridge`) avoids collision but is
  private-by-default, so publishing it publicly needs `--access public` /
  `publishConfig.access: "public"`. Scoped is the safer default.
- Optional: `npm publish --provenance` via GitHub Actions OIDC for supply-chain
  attestation.
- These join the still-pending publish-metadata items (deps→`dependencies`, Pi
  core→`peerDependencies: "*"`, `license`/`repository`/`engines`) under
  remaining-work item 2 above.
