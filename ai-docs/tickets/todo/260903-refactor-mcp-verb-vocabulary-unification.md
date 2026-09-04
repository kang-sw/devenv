---
title: "MCP read-surface verb-vocabulary unification (deterministic scripted rename)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  260903-refactor-mcp-read-surface-collapse: sibling layer ③ — collapse depends on this layer's `query` survivor naming; for tickets/specs rename and collapse coincide
sage-review-design: completed
sage-review-completeness: required
sage-review-design-reviewed: ae7feeadf1d08c7b
---

# MCP read-surface verb-vocabulary unification (deterministic scripted rename)

## Background

Layer ④ of `260903-epic-mcp-tool-surface-affordance-reduction`. The read/query
surface spreads a single "read/query" intent across `list`/`status`/`read`/
`find`/`search`/`info`/`print`, and `status` names both a corpus lookup and a
live-state read. Rename to a small canonical verb set so one intent maps to one
verb. See the epic for the canonical verb table, cost model, and cross-child
invariants; this ticket carries ④'s frozen name map and method.

Canonical verbs: `query` (search/resolve/enumerate a searchable corpus), `read`
(fetch one item's body by id/key), `list` (enumerate a short bounded set — kept,
NOT every list becomes query), `status` (live runtime/vcs/process state only).

## Decisions — frozen name map

Classifications verified against the handlers in
`agents-plugin-tool/internal/mcp/server.go` and `ai-docs/spec/mcp-tools.md`.

| tool | old verb | → new | why |
|---|---|---|---|
| `tickets.find` | find | **query** | free-text/stem/mentions search over the ticket corpus |
| `specs.find` | find | **query** | free-text/stem/ticket-ref search over spec files |
| `mental_models.find` | find | **query** | query/stem/domain search over mental-model docs |
| `note.search` | search | **query** | key-glob + date-range search across note layers |
| `playbook.print` | print | **read** | fetches one playbook body by name |
| `runtime.info` | info | **read** | static build metadata (version + source_commit); **NOT** live state |

**Two corrections to the epic's illustrative map, from source evidence:**

- `runtime.info → read`, not `status`. `runtimeInfo` returns a fixed
  `{version, source_commit}` build stamp — no live runtime/process/vcs state — so
  it is a fetch-one-record, i.e. `read`. The genuine live-state member of the
  `runtime.*` family is `runtime.debug_events`, which already reads correctly and
  is left unchanged.
- `mental_models.find → query` only; `mental_models.list`/`status` are **not**
  renamed or collapsed here (③'s audit found the triple is not a clean superset).
  They are left as an explicit, noted exception, per the epic's corpus-triple
  rule, rather than a silent half-rename.

**Kept as-is (not renamed):** bounded-set lists (`agenda.list`, `api.list`,
`config.list`, `todo.list`, plus `tickets.list`/`specs.list` which ③ removes),
already-canonical reads (`infra.read`, `convention.read`, `todo.read`), and true
live-state `status` (`git.status`, `runtime.debug_events`). The gated `exec.*`/
`mercenary.*` families are **excluded** from this pass — permanently hidden /
config-gated, unstable surface; they adopt the vocabulary later but are not this
epic's landing gate.

## Method — deterministic, not hand-edited

Once the map above is frozen, apply it by **script** across: Go tool
registration + dispatch switch, `runtime.json`, the MCP/workflow specs, playbook
tokens (`agents-plugin/rsrc/` and the wsflow mirror via the mirror script), and
tests. Verify by diff + full test run. Only a thin prose-cleanup tail
(surrounding sentences like "find the ticket…") needs a human pass. Hand-editing
name-by-name is out of method.

## Deprecation posture

One-shot hard cut, inherited from the epic. Old names have no persisted
cross-version consumer — Go dispatch, `runtime.json`, specs, playbook tokens,
workflow manual, and the wsflow mirror all ship in-package and are swept by this
ticket's own script — so the rename is atomic, not a compat break: no
alias/transition window. The script removes every old-name reference in the same
pass.

## Ordering / dependencies

- Runs **first** in the epic's execution order (locks canonical names for ③/②).
- Collision with ① (`enter.*` affordance rewrite): ④'s script edits *tokens* in
  `lead-proceed`/`lead-implement`; ① owns their *authoring* pass. Default order
  ④-script → ①-authoring so those two skills are not authored twice.
- ③ acts on the `query` survivor this layer names; for tickets/specs the rename
  and the collapse coincide on the same survivor.

## Phases

### Phase 1: Freeze the map and apply the scripted rename

Apply the frozen old→new map (Decisions) by script across every in-package
surface: Go tool registration + dispatch switch and any name constants, the tool
descriptions, `runtime.json`, `mcp-tools.md` + `workflow-skills.md`, playbook
tokens in `agents-plugin/rsrc/` and the wsflow mirror (through the mirror script,
not hand-edited), and tests. Then a thin human prose-cleanup pass for surrounding
sentences. Rename `mental_models.find→query` only, leaving `list`/`status` as the
noted exception; run before ①'s authoring pass so `lead-proceed`/`lead-implement`
are authored once.

Acceptance: no old read-surface name remains in-package (a `references.trace` /
grep sweep is clean), the full test suite is green, and `tools/list` shows the
canonical verbs. Behavior is byte-unchanged — this is a rename only, no handler
logic touched.

## Spec Impact

Tool-name renames within existing anchors only — no new spec stem, and no heading
`{#slug}` changes (so no `renamed-spec`):

- `mcp-tools.md`: `{#260505-ticket-discovery-tools}` (`tickets.find→query`),
  `{#260505-spec-discovery-tools}` (`specs.find→query`),
  `{#260505-mental-model-discovery-tools}` (`mental_models.find→query`),
  `{#260810-note-tools}` (`note.search→query`),
  `{#260609-playbook-tools}` (`playbook.print→read`),
  `{#260505-runtime-debug-metadata-tools}` (`runtime.info→read`).
- `workflow-skills.md`: wherever the renamed tools are named in skill procedure
  text (the `references.trace` sweep at implementation enumerates the sites).
