---
title: ws playbook surface MVP — rsrc loader and playbook.print/render
spec:
  - 260609-playbook-tools
  - 260609-playbook-harness-rendering
  - 260609-rsrc-playbook-distribution
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction, decisions, evidence, and the settled playbook/rsrc contract
  260523-bug-ws-mcp-launcher-runtime-repair-race: prerequisite — binary/text swap race for rsrc distribution
  260524-bug-codex-plugin-cache-refresh-mcp-startup-race: prerequisite — plugin cache refresh race for rsrc distribution
related-mental-model:
  - prompt-bundle
  - mcp-runtime
  - workflow-skills
---

# ws playbook surface MVP — rsrc loader and playbook.print/render

## Background

First implementation slice of the playbook-factory pivot (epic
`260605-epic-ws-playbook-factory-pivot`, milestone M1). It introduces the
caller-visible playbook surface that later milestones depend on: a plain-text
`rsrc/` prompt tree loaded at call time, and two MCP tools — `playbook.print`
(lead-facing procedure, returned inline) and `playbook.render` (subagent
injection prompt, written to a tmp file, path returned).

This is a net-new contract, not a refactor. `playbook.render` is the direct
promotion of wsflow's existing `prompt.render`
(`mcp-tools.md#260529-prompt-render-tool`) into the full ws distribution; the
main lift is exposing it under ws, generalizing the fixed five-prompt allowlist
into the rsrc tree, and adding harness-aware content selection. The full
direction, rejected alternatives, and empirical evidence live in
`260605-research-ws-native-subagent-pivot` (see the "playbook API", "harness-aware
routing", "prompts as plugin-path rsrc", "agentId continuity", and the
2026-06-08 "convention loading via playbook" decision sections).

## Decisions

Binding decisions inherited from the research ticket and the epic
Cross-Child Decisions. A fresh implementer must not re-derive or contradict
these:

- **Two-command split, no unification.** `playbook.print` returns procedure text
  inline; `playbook.render` writes a context-injected prompt to a worktree-scoped
  tmp file and returns the path. A single tool with output-kind metadata is
  rejected — it loses flexibility and risks full delegate prompts landing in the
  lead's context.
- **Plain-text rsrc tree, loaded at call time.** Prompts/playbooks ship as plain
  text under a plugin-path `rsrc/` tree. Not `go:embed`, not Go raw literals.
  Text-only changes must ship without a binary version bump.
- **Manifest with schema-version compatibility, not hash equality.**
  `rsrc/manifest.json` carries file hashes plus a playbook **schema version**;
  the binary checks schema-version compatibility, not exact hash equality.
- **Loud, partial failure; no embedded fallback.** A manifest/schema mismatch or
  load failure fails the playbook surface loudly. There is no embedded fallback
  copy (split-brain drift risk). An agentless MCP without playbooks still serves
  discovery/git tools — partial death is acceptable and visible.
- **Dev override `WS_RSRC_ROOT`.** Required in the MVP: dogfood MCP reads the
  plugin **cache** copy, so without the override repo edits wait on cache
  refresh and the iteration win evaporates. `WS_RSRC_ROOT=<repo>/.../rsrc`
  overrides the load root.
- **Fully custom playbook schema.** ws is the sole reader/renderer; the schema is
  bound to no agent/MCP-prompt standard. Frontmatter fields, directory layout,
  and manifest format are an autonomous design detail.
- **Harness differences ship as data.** Shared playbook body + a per-harness
  terminology table (`explore_agent`, `spawn_idiom`, `continue_idiom:
  "SendMessage(to: <agentId>)"`, model aliases). Structural divergence only via
  per-harness overlay files (e.g. `subquery.md` + `subquery.codex.md`). Harness
  set is **claude + codex only**; Gemini is excluded.
- **Model-name tables live in config, never in the bundle.** Concrete per-provider
  model names extend `config.agents_tier`-style config so users update them
  without redistribution. The rsrc text and binary never bake concrete model
  names.
- **Unknown-harness fallback.** Render host-neutral text in the current skill
  prose style. Preserves the harness-neutral doctrine even with a 2-way set.
- **Delegation tip is a compact fragment, not the full manual.** Inject the
  retain/spawn continuity tip only into playbooks marked `delegates: true`, as a
  standard fragment at the moment of action — e.g. `tip: after spawning, the
  harness returns an agent id — reuse it for continuation (claude:
  SendMessage(to: <agentId>)) instead of respawning.` No MCP registry, no
  mandated memory-file recording (agentId continuity is tip-only).
- **Frontmatter-declared auto-includes.** A playbook declares its own text
  dependencies in frontmatter (e.g. `includes: [ticket-conventions]`); the rsrc
  loader auto-includes them at print time. Flag-based include selection
  (`playbook.read(name, ["conventions"])`) is rejected — it pushes the decision
  back to the caller. The benefit is atomicity (the procedure cannot be obtained
  without its conventions), fixed at authoring time and CI-validated.
- **`convention.read`/`infra.read` survive as standalone discovery tools.** They
  serve raw access (audits, ad-hoc inspection) and do not compete with the
  execution-path auto-include. Whether they later share the rsrc loader
  internally is an implementation-sequencing call, not an MVP contract change.

## Constraints

- Distribution-race prerequisites: the binary/text swap races
  (`260523-bug-ws-mcp-launcher-runtime-repair-race`,
  `260524-bug-codex-plugin-cache-refresh-mcp-startup-race`) bite at swap time
  regardless of approach and are prerequisites for reliable rsrc distribution.
  MVP work may proceed against the dev override (`WS_RSRC_ROOT`) and the plugin
  cache path, but the cache-refresh story for shipped rsrc edits depends on
  those tickets.
- Open verification item carried from research: confirm Codex plugin
  distribution materializes non-skill `rsrc/` directories into its cache (skills
  are known to work). Note in the Phase 1 result whether this was confirmed.
- This ticket does NOT remove spawn machinery, convert skill bodies, or touch
  the actor/session-auth model — those are M2/M3. The playbook tools are added
  alongside the existing surface; no deletion in this slice.

## Phases

### Phase 1: rsrc plain-text tree, loader, manifest, and validator

Goal: the call-time loading substrate the playbook tools sit on.

- Define the `rsrc/` directory layout and the fully-custom playbook frontmatter
  schema (one directory per playbook; frontmatter fields including at least
  `kind: print|render`, `delegates: bool`, `includes: [<text-dep>]`, declared
  substitution variables, and harness-overlay naming convention such as
  `<name>.codex.md`).
- Implement the loader (reads from the plugin path, or `WS_RSRC_ROOT` when set),
  the substitution engine (declared variables only), and the auto-include
  resolver (frontmatter `includes:` pulled at load time).
- Implement `rsrc/manifest.json` with file hashes + playbook schema version, and
  the schema-version compatibility check (not hash equality). Mismatch ⇒ loud
  failure of the playbook surface, no embedded fallback.
- CI tree validator: required harness variants present, declared substitution
  variables resolvable, declared `includes:` exist. The Go side shrinks to
  loader + validator + substitution + auto-include engine.
- Verification: unit tests over loader/substitution/auto-include and
  schema-version compatibility (compatible, incompatible, missing manifest);
  CI validator catches a missing-variant / undeclared-variable / dangling-include
  tree. Confirm Codex `rsrc/` cache materialization (or record it as still open).

Deliverable boundary: no MCP tool yet; the loader/validator is exercised through
tests and the CI tree check. No skill-body migration in this phase.

### Result (5d4f9d03) - 2026-06-09

Implemented in `bd9f1cae` (feat) + `5d4f9d03` (review fixes). Added the new
`agents-plugin-tool/internal/wsrsrc/` package — a call-time, filesystem-backed
loader deliberately distinct from the `go:embed` `internal/wsprompt` bundle:

- `frontmatter.go` — YAML-subset parser (scalars + `- item` lists + sub-maps),
  modeled on `wsdoc/frontmatter.go` with added `\r\n` normalization on all return
  paths (not `wsprompt.stripFrontmatter`, which is scalar-only).
- `manifest.go` — per-file sha256 (cross-platform via a verbatim copy of
  `wsprompt.normalizePromptHashContent`), `ReadManifest`/`GenerateManifest`/
  `WriteManifest`.
- `loader.go` — `ResolveRoot` (`WS_RSRC_ROOT` env-first; plugin-path default is a
  documented Phase-2 stub for the `os.Executable()→../rsrc` derivation), `Load`
  (base + harness overlay), single-pass declared-variable substitution (undeclared
  or unprovided-but-used → typed error; no double-expansion), flat auto-include
  resolver, `isBareStem` guard applied to name and harness components
  independently.
- `validate.go` — `Validate(root)`: manifest existence/schema/hash, required base
  variant per playbook dir, undeclared-variable scan, dangling-include check,
  bidirectional manifest/tree coverage.
- `wsrsrc.go` — six typed errors (`ErrManifestMissing`, `ErrSchemaMismatch`,
  `ErrHashMismatch`, `ErrFileMissing`, `ErrUndeclaredVar`, `ErrUnprovidedVar`),
  `PlaybookMeta`/`LoadedPlaybook`/`Manifest`, `SupportedSchemaVersion` const.
- `agents-plugin/rsrc/` sample tree: `sample-playbook/{sample-playbook.md,
  sample-playbook.codex.md}` (base + codex overlay with an include and a declared
  `{{.WorktreeID}}` variable), `sample-conventions.md` (flat include),
  `manifest.json` (`schema_version=1`).
- 45 unit tests including `TestValidateRealTree` (the CI tree gate, picked up by
  the existing `go test ./...` workflow with no new script). Compatibility is
  gated on schema-version only; hashes are CI tree-sync + load-time integrity.

Decisions honored: plain-text call-time loading (binary version decoupled from
text edits), loud typed-error failure with no embedded fallback, frontmatter-
declared auto-includes. Partitioned review (correctness/fit/test) returned clean
after one fix cycle.

Open item: Codex plugin distribution materializing non-skill `rsrc/` directories
into its cache is **not yet confirmed** — Phase 1 runs against the `WS_RSRC_ROOT`
dev override; the distribution-race prerequisites (`260523`, `260524-codex-cache`)
were not addressed here.

Forward to Phase 2: wire `playbook.print`/`playbook.render` MCP tools onto the
`Load` seam, add harness-aware selection (config model tables, unknown-harness
fallback, delegation tip), and confirm whether `manifest.json` needs to enter the
`runtime.json` launcher validation contract.

### Phase 2: playbook.print / playbook.render MCP tools and harness-aware selection

Depends on Phase 1 (loader/schema/manifest must exist).

Goal: the two caller-visible MCP tools and harness-aware rendering.

- `playbook.print(name, context?)` — returns the auto-included, substituted
  procedure text inline in the tool result.
- `playbook.render(name, context?)` — writes the context-injected, harness-rendered
  prompt to a worktree-scoped tmp file and returns the path; promotes wsflow
  `prompt.render` behavior (namespace substitution, context injection) into ws
  and generalizes beyond the fixed five-prompt allowlist.
- Harness-aware content selection: detected harness (existing
  `detectHarnessFromRaw`/`observeHarness`) selects the terminology table and any
  per-harness overlay; model aliases resolve from config (never from the
  bundle). Unknown harness ⇒ host-neutral fallback text.
- Delegation tip fragment injected only for `delegates: true` playbooks.
- Tool advertisement/gating follows the full-ws distribution (these are ws-side
  tools; relationship to wsflow `prompt.render` per the spec).
- Verification: golden-render tests for claude / codex / unknown-harness on a
  `delegates: true` and a plain playbook; `print` returns inline with includes
  applied; `render` returns a tmp path whose contents match the rendered
  expectation; config-driven model alias substitution verified without baked
  model names.

**Forward-compat guardrail — keyed render signature (M3 coordination).** The
epic Cross-Child Decisions settle the *final* render signature as
`playbook.render(session_key, name, context?, root_override?)`: render is keyed
like every ws call; when `session_key.role == lead` it mints and injects a fresh
child key; `root_override` rebinds both the auto-include root and the child-key
root for worktree children (the caller passes the path, so
pre-allocate-before-splice makes it known at render time and render never has to
infer lead-in-worktree vs spawn-into-worktree). The session-auth model
(`session_key`, `ws/lead.login`, key roles, in-memory session→root map) does NOT
exist until M3, so **Phase 2 implements `render(name, context?)` as written above
and does NOT mint keys** — but it must leave the seam so M3 retrofits the keyed
signature without reworking the tmp-file / context-injection / path-return core:

- Do not bake in `name` as an immovable first positional or otherwise preclude
  prepending a `session_key` argument later.
- Keep root resolution a parameterizable seam: the `WS_RSRC_ROOT` / auto-include
  root must be overridable from the call site (anticipating `root_override`),
  not hard-wired to a single process-global resolution.

Rejected for Phase 2: implementing the full keyed signature now — session-auth
lands in M3, and render cannot mint a child key before the session model exists.

Deliverable boundary: the playbook surface is callable and harness-aware. Skill
bodies are NOT yet migrated to playbooks (M2). Spawn machinery untouched (M3).

## Spec

Contract-first spec authored in `mcp-tools.md`:

- `260609-playbook-tools` — `playbook.print` / `playbook.render` caller contract,
  delegation continuation tip, frontmatter auto-include.
- `260609-playbook-harness-rendering` — harness-aware content selection
  (claude+codex data tables, config-backed model names, unknown-harness
  fallback).
- `260609-rsrc-playbook-distribution` — plain-text resource tree, manifest +
  schema-version compatibility, loud partial failure with no embedded fallback,
  `WS_RSRC_ROOT` dev override.

Phase 1 implements `260609-rsrc-playbook-distribution`; Phase 2 implements
`260609-playbook-tools` and `260609-playbook-harness-rendering`. Implementation
commits carry a `## Spec` section referencing these stems.
