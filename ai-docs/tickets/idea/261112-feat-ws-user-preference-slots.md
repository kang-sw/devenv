---
title: "User preferences as named slots that layer across config levels"
---

# User preferences as named slots that layer across config levels

## Background

User feedback (whiteboard, 2026-11-12); the decisions it led to are in
`## Decisions`.

Today the user preference is a single value that the workflow manual prints
under `### User preferences`. The user asked for it to become a set of named
slots:

- The existing default note moves into a `default` slot.
- Slots override per slot, and keys accumulate across config levels, so each
  layer can add slots of its own.
- A lower layer clears a slot set by a higher one by setting it to the empty
  string; a cleared slot disappears from the manual entirely.
- Existing downstream configs that set the single value stay compatible
  (see `## Decisions`).

## Current State (evidence)

- The "single value" is not a config key of its own. `### User preferences` in
  `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:41-45` is an
  override point (`UserPreferenceSection`) whose seed is `Use the project's
  terminology and communication style.` plus `{{.WorkflowLang}}`. The wsflow
  copy is the same file text (`agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md:41`).
- An override is stored as `prompt.UserPreferenceSection.<harness>` and
  written by `config.tune` through `promptKnobEntry`
  (`agents-plugin-tool/internal/mcp/config_registry.go:427-455`); its value
  must be non-empty. The bucket read order is harness, then `all`
  (`config_registry.go:142-157`).
- Render lookup is `buildOverrideLookup` (`mcp/playbook_tools.go:696-714`): a
  stored non-empty value replaces the whole seed block, including the
  `WorkflowLang` line; an empty value counts as unset and falls to the seed, not
  to a lower layer (`applyOverrideMarkers`, `playbook_tools.go:734+`).
- `WorkflowLang` is generated from `workflow.lang` (`playbook_tools.go:231-238`,
  `scope.go:99-101`).
- Layer walk per scalar key is session, project, repo (committed), global,
  builtin; the first layer holding the key wins (`wsconfig/resolver.go:82-127`).
  `config.tune reset` deletes the key at one scope (`resolver.go:238+`,
  `mcp/server.go:~1099-1110`), so it falls through to lower layers; there is no
  way today to hide a lower layer's value.
- No scalar prefix listing exists. `ListStructured` (`wsconfig/structured.go:27-60`)
  lists a key family across layers per key but covers only JSON values and no
  session scope.
- `reservedConfigPrefixes` (`mcp/config_manifest.go:48`) bars manifests from
  `prompt.`, `agents.`, `worktree.`, `agent.`.

## Design

- Key shape: scalar key family `preference.<slot>`, slot name
  `^[a-z0-9][a-z0-9_-]*$`; `preference.` added to the reserved prefixes. Default
  scope global; `config.tune` takes `scope` like `workflow.lang`; no harness
  bucket.
- Merge: per slot, the first layer holding the key wins, using the existing
  session > project > repo > global walk; slots accumulate as the union of keys
  over all layers (new `ListScalars(prefix)` mirroring `ListStructured`, with
  session scope).
- Set / clear: `config.tune key=preference.<slot> value=<text>` sets; `value=""`
  stores an empty tombstone at the chosen scope, which hides the slot from lower
  layers and from the render; `reset:true` removes the entry at that scope so the
  next layer shows through.
- Render: inside the `UserPreferenceSection` point, `default` first, then other
  slots by name ascending, each non-empty slot text as its own paragraph, no slot
  labels. The `workflow.lang` directive renders whenever `workflow.lang` is set,
  as its own line independent of every slot.
- Migration: slot `default` reads `preference.default`, falling back to the
  legacy `prompt.UserPreferenceSection.<harness>` override and then the seed;
  existing configs keep working untouched.
- Tier: T0 per `ai-docs/manuals/release-compatibility.md` (new optional key
  family, legacy key still honored; older binaries ignore unknown override keys
  and render the legacy point). Not T2: nothing removed, no store shape change.

## Phases

### Phase 1: Slotted user preferences

- Add `wsconfig` scalar prefix listing across session, project, repo, and global,
  with tombstone-aware winner selection.
- Register the `preference.<slot>` family for `config.tune` and `config.list`;
  validate slot names; reserve the prefix.
- Render slots inside `UserPreferenceSection` in the order settled in
  `## Decisions`, with the legacy `default` fallback.
- Move the `{{.WorkflowLang}}` line out of the point's seed so the language
  directive renders from `workflow.lang` alone, independent of any slot, a
  cleared `default`, and the legacy override; edit
  `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` and mirror
  the same text into `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md`
  (`ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/skill-authoring.md`).
- Tests: cross-layer accumulation, per-slot override, tombstone clear, reset
  fall-through, ordering, legacy fallback, and language line present with
  `default` cleared, with a legacy override set, and absent when `workflow.lang`
  is unset.

## Decisions

Compatibility tier: T0 (additive key family, legacy key still honored; no
removal, no store shape change), per `ai-docs/manuals/release-compatibility.md`.

- Layer precedence is session > project > repo > global: the more specific
  layer wins per slot, using the existing resolver walk
  (`wsconfig/resolver.go:82-127`).
- Preferences are a new `preference.<slot>` scalar key family. Rejected:
  extending `prompt.UserPreferenceSection` with a slot selector, because that
  knob is harness-bucketed and requires a non-empty value, which conflicts with
  tombstone clearing and a per-slot key.
- Clear is a stored empty-string tombstone at the target scope; `reset:true`
  removes that scope's entry. Rejected: making reset the clear, because reset
  falls through to lower layers and cannot hide a slot.
- Render order is `default` first, then slot names ascending. Rejected: layer or
  insertion order, because config files are maps with no order, so only a name
  sort is deterministic.
- Add `ListScalars(prefix)` to the resolver, session scope included. Rejected:
  reusing `ListStructured`, because structured listing is JSON-only and excludes
  session scope.
- Slot names are lowercase `[a-z0-9_-]` and the `preference.` prefix joins
  `reservedConfigPrefixes`, which keeps manifest namespaces from colliding and
  keeps keys valid as `config.tune` arguments.
- [technical] Existing single-value configs stay compatible by read-alias:
  `default` falls back to the legacy `prompt.UserPreferenceSection.<harness>`
  override when no `preference.default` exists; no data rewrite, legacy writes
  keep working. Rejected: one-shot migration (rewrites a user's config files on
  first read) and hard cut (T2, minor bump only).
- [policy] The `workflow.lang` language directive renders whenever
  `workflow.lang` is set, independent of any preference slot. Rejected:
  rendering it inside the `default` slot text, where hiding or overriding
  `default` would drop the language line. Knock-on: a user who overrode the old
  single value (which replaced the seed, language line included) now sees one
  extra language line.
