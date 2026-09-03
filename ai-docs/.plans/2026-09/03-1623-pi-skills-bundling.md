# Plan: bundle ws skills into the agents-plugin-pi tarball (no committed duplicate)

Branch: `impl/track/pi-agent/pi-skills-bundling`. Survey-only plan; design already decided.

## Goal

Make `agents-plugin-pi/` carry the ws skills tree so a published/installed tarball
exposes them, WITHOUT committing a large duplicate. Package-local-first resolver at
runtime; pack-time copy generates a gitignored `agents-plugin-pi/skills/`; `files`
whitelist ships it.

## Current state (evidence)

- `agents-plugin-pi/src/index.ts:55-58` — path wiring:
  - `srcDir = dirname(fileURLToPath(import.meta.url))`
  - `pluginDir = dirname(srcDir)` (= `agents-plugin-pi/`)
  - `repoRoot = dirname(pluginDir)`
  - `skillsDir = join(repoRoot, "agents-plugin", "skills")` ← the line to change.
  - `resources_discover` returns `{ skillPaths: [skillsDir] }` (`index.ts:67-69`).
- `index.ts:48` imports only `{ dirname, join }` from `node:path` — **`existsSync` is NOT imported** in index.ts. Precedent for the import exists: `src/spawner.ts:34` imports `{ existsSync, mkdtempSync } from "node:fs"`.
- `agents-plugin-pi/package.json` — has `name`, `version` (`0.1.0`), `private: true`, `type: module`, `description`, `pi.extensions`, `scripts.test: "node --test"`. No `files`, no `prepack`/`prepare`, no `scripts/` dir (confirmed: none exists).
- Pure-seam test convention: each pure helper lives in its own `src/*.ts` and is unit-tested by importing named exports from `../src/X.ts` under `test/X.test.ts`, run via `node --test` (Node v22+ native TS type-stripping, zero deps). See `test/model-catalog.test.ts`, `test/version-check.test.ts`.
- `.gitignore` is a single **root** file (no per-package one). It already carries per-package lines like `agents-plugin-pi/.runtime/`, `agents-plugin-pi/.local-devenv-runtime`.
- No `agents-plugin-pi/skills/` currently exists.
- Pi package docs (`.../pi-coding-agent/docs/packages.md`): for npm sources Pi downloads the published tarball; for git sources Pi clones and runs `npm install`. `skills/` is a convention directory Pi auto-discovers, but here discovery is driven explicitly via `resources_discover`, so the resolver path is what matters.

## Testable shape (recommendation)

Extract a tiny pure helper into a **new module** `agents-plugin-pi/src/skills-dir.ts`
(mirrors the one-pure-seam-per-module pattern of `model-catalog.ts` / `version-check.ts`).
The resolver in `index.ts` is currently a bare `join(...)` at module top-level — not
unit-testable without executing index.ts's whole import graph. A separate module keeps
the test import cheap and the seam pure (dependency-injected `exists`):

```ts
// src/skills-dir.ts
import { existsSync } from "node:fs";
import { join } from "node:path";

/**
 * Package-local-first skills resolver. Prefers a pack-time-generated
 * `<pluginDir>/skills` (present in published/installed tarballs); falls back to
 * the monorepo canonical `<repoRoot>/agents-plugin/skills` for dev `-e` runs
 * from the source tree. `exists` is injected for unit testing.
 */
export function resolveSkillsDir(
  pluginDir: string,
  repoRoot: string,
  exists: (p: string) => boolean = existsSync,
): string {
  const local = join(pluginDir, "skills");
  if (exists(local)) return local;
  return join(repoRoot, "agents-plugin", "skills");
}
```

## Edit steps

### 1. `agents-plugin-pi/src/skills-dir.ts` (new file)

- Create it with the `resolveSkillsDir` helper above.

### 2. `agents-plugin-pi/src/index.ts`

1. Add import near the top imports (after L48-49):
   `import { resolveSkillsDir } from "./skills-dir.ts";`
2. Replace L58:
   - from: `const skillsDir = join(repoRoot, "agents-plugin", "skills");`
   - to:   `const skillsDir = resolveSkillsDir(pluginDir, repoRoot);`
3. Leave `resources_discover` (L67-69) unchanged — it already returns `[skillsDir]`.
   `join` stays imported (still used for `launcherPath`/`runtimeJsonPath`/`modelCatalogPath`).

### 3. `agents-plugin-pi/package.json`

Add `files` whitelist and the pack-time copy scripts:

```json
"files": [
  "src/",
  "bin/",
  "rsrc/",
  "skills/",
  "runtime.json",
  "model-catalog.json"
],
"scripts": {
  "test": "node --test",
  "copy-skills": "node scripts/copy-skills.mjs",
  "prepack": "npm run copy-skills",
  "prepare": "npm run copy-skills"
}
```

Notes:
- `private: true` is currently set. Publishing a real tarball needs it removed (or the ship flow uses `npm pack` which ignores `private`). `npm pack --dry-run` works with `private:true`, so leave `private` as-is for this implementer commit unless the lead's ship flow says otherwise — flag, don't change.
- Copy helper lives in a new `agents-plugin-pi/scripts/copy-skills.mjs` (no scripts/ dir exists yet — create it). A `.mjs` file is clearer and diffable than an inline `node -e`, and matches the repo's preference for readable helpers.

### 4. `agents-plugin-pi/scripts/copy-skills.mjs` (new file)

Node-builtins-only (so `npm install --omit=dev` can still run it — no devDependency):

```js
#!/usr/bin/env node
// Pack-time copy: mirror the canonical ws skills tree into this package so the
// published/installed tarball carries it. Generated dir is gitignored, never
// committed. Source is the sibling monorepo tree; when packing from outside the
// monorepo (already-vendored tarball), the source is absent and we no-op.
import { cpSync, existsSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const pkgDir = dirname(dirname(fileURLToPath(import.meta.url))); // agents-plugin-pi/
const src = join(dirname(pkgDir), "agents-plugin", "skills");
const dest = join(pkgDir, "skills");

if (!existsSync(src)) {
  console.warn(`[copy-skills] source ${src} absent; leaving ${dest} as-is`);
  process.exit(0);
}
rmSync(dest, { recursive: true, force: true });
cpSync(src, dest, { recursive: true });
console.log(`[copy-skills] copied ${src} -> ${dest}`);
```

### 5. Root `.gitignore`

Add next to the other `agents-plugin-pi/` lines (after `agents-plugin-pi/.local-devenv-runtime`):

```
agents-plugin-pi/skills/
```

### 6. `agents-plugin-pi/test/skills-dir.test.ts` (new file)

Follow `test/model-catalog.test.ts` structure (`node:test` + `assert/strict`, inject `exists`):

```ts
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { join } from "node:path";
import { resolveSkillsDir } from "../src/skills-dir.ts";

describe("resolveSkillsDir", () => {
  const pluginDir = "/pkg/agents-plugin-pi";
  const repoRoot = "/pkg";

  test("prefers package-local skills/ when it exists (installed tarball)", () => {
    const local = join(pluginDir, "skills");
    assert.equal(resolveSkillsDir(pluginDir, repoRoot, (p) => p === local), local);
  });

  test("falls back to repoRoot/agents-plugin/skills when package-local absent (dev -e)", () => {
    assert.equal(
      resolveSkillsDir(pluginDir, repoRoot, () => false),
      join(repoRoot, "agents-plugin", "skills"),
    );
  });
});
```

## Why prepack + prepare (both)

- **`prepack`** is the primary hook: it runs at `npm pack` / `npm publish` time, baking
  `skills/` into the tarball. Consumer-side `npm install --omit=dev` never re-runs it —
  the tarball already contains `skills/`, so the consumer install flag is irrelevant. This
  is the robust path for npm-source installs.
- **`prepare`** additionally covers **git-source** installs: Pi clones the repo and runs
  `npm install` in the clone (docs/packages.md, "git" + "Dependencies" sections), which
  fires `prepare` (not `prepack`) — regenerating `skills/` in the checkout. `prepare` runs
  even under `--omit=dev` because the copy script uses only Node builtins (no dev deps to
  strip).
- Both point at the same `copy-skills` script, so the behavior is identical; the two hooks
  just cover the two install sources (npm tarball vs git clone).

## Verification commands (from `agents-plugin-pi/`)

1. Unit test the resolver + full suite:
   `cd agents-plugin-pi && node --test` (expect the new `resolveSkillsDir` cases green plus existing suites).
2. Run the copy manually and confirm the tree lands:
   `node scripts/copy-skills.mjs && ls skills | head` (should list `lead-proceed/`, `lead-discuss/`, … ; then `git status --porcelain agents-plugin-pi/skills` shows **nothing** — proving gitignore holds).
3. Prove the tarball includes skills/ (prepack fires):
   `npm pack --dry-run` (or `npm pack --dry-run --json`) — grep the file list for `skills/` entries and for the other whitelisted assets (`bin/`, `rsrc/`, `runtime.json`, `model-catalog.json`, `src/`). Confirm NO stray files (e.g. `.runtime/`, `test/`) leak in.
4. Prove dev `-e` fallback still resolves: with `agents-plugin-pi/skills/` removed
   (`rm -rf skills`), a `resolveSkillsDir(pluginDir, repoRoot, existsSync)` call returns
   `<repoRoot>/agents-plugin/skills` — covered by the unit test's fallback case; optionally
   sanity-check by launching Pi `-e` from the monorepo and confirming ws skills still load.

## Spec staleness to flag for the lead (do NOT edit in the implementer commit)

`ai-docs/spec/pi-adapter-runtime.md` — two sections go stale:

1. **Skill exposure** `{#260903-pi-bridge-skill-exposure}` (L85-86): says the adapter
   answers `resources_discover` "with the path to the ws `agents-plugin/skills/` tree …
   pointing at the existing directory directly rather than copying it." After this change the
   resolver is **package-local-first**: it prefers a pack-time-copied `agents-plugin-pi/skills/`
   and only falls back to `agents-plugin/skills/` for dev runs. Needs rewording.
2. **Package topology** `{#260903-pi-adapter-package-topology}` (L262-270): enumerates the
   self-contained copies the package carries (`bin/ws-mcp-launcher.py`, `runtime.json`,
   `rsrc/`) and says these are "kept in sync by hand." The skills tree is now a **fourth**
   carried copy, but generated at pack time (gitignored, not hand-synced) — a distinct
   sync model worth noting so the "by hand" claim isn't read as covering skills/.

Also note: `index.ts:14-17` and `:34-43` header prose describe skills as "pointing at the
existing directory directly rather than copying it" — the implementer should update that
comment to match the new package-local-first behavior (in-scope for the implementer commit,
since it's code-adjacent, unlike the spec which the lead owns).
