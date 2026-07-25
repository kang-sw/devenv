---
title: Prefs portability — export/import to file and web sync
related:
  260725-feat-workspace-workroot-alias: introduces the scope-tagged prefs registry this builds on
---

# Prefs portability — export/import to file and web sync

## Background

Dashboard client customization prefs are accumulating: terminal style
(`terminalPrefs`), nav order + hidden worktrees (`workNavOrder`), and (incoming)
workspace/workroot aliases (`workNavAliases`). Today each is an isolated,
device-local localStorage blob with no way to carry settings to another
machine/browser or back them up. This idea captures the future arm that makes
those prefs portable, built on the scope-tagged prefs registry introduced by
`260725-feat-workspace-workroot-alias` (Phase 1).

## Direction

The registry classifies each store by `scope`:

- **`portable`** (machine-independent): terminal style, and future theme /
  keybindings. These are the real portability/sync targets — meaningful on any
  device.
- **`host`** (PC-bound): nav order, hidden worktrees, aliases. Keyed on local
  path-derived, server-scoped ids, so they are inert on a different machine.

Two deliverables, both riding the registry (kept as one idea for now; split into
actionable tickets when picked up):

1. **Export / import to file.** Serialize all registered stores into one JSON
   blob and re-import it. `portable` prefs apply anywhere; `host`-scoped prefs
   only rehydrate on the machine whose ids match and are otherwise ignored
   (inert), not errored.
2. **Web sync.** Sync `portable` prefs across browsers/devices via some backend
   surface (spec TBD). `host`-scoped prefs stay local (or sync but only rehydrate
   on the matching host).

## Open questions

- Sync backend/transport: where do portable prefs live server-side, and what auth
  gates them? (No spec exists yet for a dashboard prefs sync surface.)
- Import conflict policy: merge vs replace per store; how to surface
  host-scoped entries that did not apply.
- Whether `host`-scoped prefs are worth including in export at all (backup value
  vs noise), given they are inert off-host.
- Versioning/migration across store `version` bumps during import.

## Prior Art

- `260725-feat-workspace-workroot-alias` Phase 1 — the scope-tagged prefs
  registry (`{ key, version, scope, parse, defaults }`) this feature serializes.
- `settingsStore.ts` `load/saveNamespacedPrefs` — the versioned `{version, value}`
  envelope each store already uses.
