---
title: pi-web-access cannot resolve on a root-only git-install (Explore role breaks)
related:
  260914-chore-ws-pi-root-manifest-runtime-deps: origin of the finding
---

# pi-web-access cannot resolve on a root-only git-install (Explore role breaks)

## Background

`260914-chore-ws-pi-root-manifest-runtime-deps` fixed the reported
`Cannot find module 'yaml'` extension-load failure by mirroring
`agents-plugin-pi/package.json`'s six runtime `dependencies` into the
repo-root `package.json`, on the premise (stated in that ticket's Decisions)
that "Node/jiti module resolution from `agents-plugin-pi/src/*.ts` walks up to
`<root>/node_modules` and resolves them."

That premise holds for five of the six deps (`@anthropic-ai/claude-agent-sdk`,
`ipaddr.js`, `jiti`, `linkedom`, `yaml`) — they are imported as bare
specifiers and ordinary upward Node resolution reaches `<repo-root>/node_modules`.

It does **not** hold for the sixth, `pi-web-access`. A round-1 correctness
review of that ticket found, and this ticket's author independently confirmed
by reading the source, that `agents-plugin-pi/src/web-search.ts` resolves
`pi-web-access` via an **exact filesystem path**, not module resolution:

```ts
// web-search.ts:32-37
export function createWebSearch(options: WebSearchOptions = {}) {
  let packageRoot = options.packageRoot ?? dirname(dirname(fileURLToPath(import.meta.url)));
  try { packageRoot = realpathSync(packageRoot); } catch { /* ... */ }
  const root = join(packageRoot, 'node_modules', 'pi-web-access');
```

Every call site (`web-tools.ts:24`, `spawner.ts:2813`, `spawner.ts:3030`)
passes `packageRoot: dirname(dirname(extensionPath))`, which is always
`agents-plugin-pi/` (the extension entry point is
`agents-plugin-pi/src/index.ts`). The check at `web-search.ts:47-52` is
deliberately exact-directory-only ("Exact local directory only:
require.resolve could walk into a user/global copy") and additionally
requires `realpathSync(root) === root`.

On Pi's git-install flow (clone + `npm install` against the root manifest
only), this creates `<repo-root>/node_modules/pi-web-access`, but
`agents-plugin-pi/node_modules/` is never created — it is `.gitignore`d and
only populated by local dogfooding, and the chosen fix deliberately rejected
a nested `npm install` (postinstall hook) as an alternative. So
`docs.manifest` read at `web-search.ts:49` throws, and `createWebSearch(...).probe()`
fails with `web-search-extension-missing`.

`spawner.ts:2813` awaits that `.probe()` **uncaught** for
`ctx.spawnRole === "explore"`, so on a clean git-install, spawning an Explore
agent throws outright rather than degrading gracefully.

The originally-reported bug (`yaml` missing at extension load) is fixed by
`260914-chore-ws-pi-root-manifest-runtime-deps` and does not depend on this.
This is a distinct, second gap in the same "does git-install actually work
end-to-end" story, scoped to the Explore/web-search codepath, discovered via
static analysis (not yet reproduced against a real clean-machine git-install).

## Phases

### Phase 1: Decide and implement a resolution strategy for pi-web-access under git-install

Open questions for whoever picks this up (not pre-decided):

- Does `web-search.ts` need a second resolution path (e.g. also check
  `<repo-root>/node_modules/pi-web-access` when the exact
  `agents-plugin-pi/node_modules/pi-web-access` path misses), and if so, does
  that weaken the deliberate "exact local directory only" security invariant
  documented at `web-search.ts:48` (avoiding accidental resolution into a
  user/global copy)? A repo-root check is not a "user/global copy" in the
  same sense, but the tradeoff needs a considered decision, not a silent
  patch.
- Alternatively, does this dependency warrant the postinstall-nested-install
  alternative that `260914-chore-ws-pi-root-manifest-runtime-deps` rejected
  for the other five deps — scoped just to this one package?
- Confirm against a real clean-machine `pi install git:...` whether this
  reproduces as described (this ticket's finding is static-analysis-only).

