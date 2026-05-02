---
title: agents-plugin Codex-first port scaffold
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260501-research-agents-bootstrap-root-context: root context direction for host-neutral agents
---

# agents-plugin Codex-first port scaffold

## Background

The host-neutral ws plugin migration should not begin by rewriting
`claude-plugin/` in place. That directory is the stable Claude Code plugin and the
reference implementation for current users. The safer first slice is a parallel
`agents-plugin/` candidate that can be shaped around Codex plugin behavior,
validated in Codex, and then compared against the existing Claude package for
best-effort compatibility.

This ticket creates the initial Codex-first scaffold only. It should establish a
loadable plugin candidate and one minimal usable workflow path without claiming
full skill migration, MCP runtime support, or Claude compatibility completion.

## Decisions

- **Parallel directory**: create `agents-plugin/` rather than renaming
  `claude-plugin/` or introducing the final shared `plugin/` directory now.
  Rejected: in-place porting, because it would mix Codex experiments with the
  known-good Claude package.
- **Codex-first gate**: verify plugin discovery and one minimal workflow in Codex
  before optimizing for Claude compatibility.
- **Claude compatibility as best effort**: use `claude-plugin/` as the file-format
  reference, but leave final Claude validation to a later manual pass in a real
  Claude session.
- **No PATH assumption**: the scaffold must not depend on Claude plugin install
  behavior to make `ws-*` helpers available on the shell PATH.

## Constraints

- Do not delete, rename, or restructure `claude-plugin/`.
- Keep copied content scoped. If existing skill or infra text is copied into
  `agents-plugin/`, normalize obvious Claude-only idioms during the copy instead
  of editing the source package.
- Do not implement the MCP runtime or broad `ws-*` adapter layer in this ticket.
- Do not declare Claude compatibility complete from Codex-only verification.

## Phases

### Phase 1: Scaffold `agents-plugin/`

Create the minimal directory structure and manifests needed for Codex plugin
discovery. Use current Codex plugin documentation and the existing
`claude-plugin/` layout as references.

The scaffold should include only the smallest useful set of files needed to prove
the candidate can load and expose a workflow entry point. Prefer copying a narrow
subset over bulk-importing every current skill.

Success criteria:

- `agents-plugin/` exists with a Codex-recognizable manifest.
- The scaffold includes a clear root context or skill entry point for this repo's
  workflow.
- `claude-plugin/` remains unchanged except for unrelated explicit follow-up work.

### Result (7994a9a) - 2026-05-02

Created `agents-plugin/` as the isolated Codex-first plugin candidate. The
scaffold includes `.codex-plugin/plugin.json`, a `skills/` tree, and the initial
`skill-authoring` skill derived from `ai-docs/ref/skill-authoring.md` so future
skill ports have a local authoring pivot.

Validation completed:

- `python3 -m json.tool agents-plugin/.codex-plugin/plugin.json`
- direct structure check for manifest fields, skill frontmatter, required skill
  sections, and `agents/openai.yaml` default prompt
- `git diff --check`

The generated skill validator could not run because the local Python environment
lacked `PyYAML` (`ModuleNotFoundError: No module named 'yaml'`). Phase 2 still
needs to verify actual Codex discovery/loading and a minimal `$skill-authoring`
invocation path.

### Phase 2: Codex loading and minimal workflow verification

Verify that Codex can discover the local candidate and that one minimal workflow
entry point is usable. Capture exact commands, configuration paths, and observed
results in the ticket result.

The verification should specifically check that helper access does not rely on
Claude plugin PATH mutation. If a helper is needed, use an explicit local path or a
documented adapter placeholder.

Success criteria:

- Codex discovery/loading is verified with command output or an equivalent
  observable result.
- One minimal workflow path is exercised.
- Any Codex-specific limitations are recorded for future tickets.

### Phase 3: Best-effort Claude compatibility alignment

Compare the `agents-plugin/` structure with `claude-plugin/` and current
searchable Claude plugin references. Add compatibility metadata or layout tweaks
only where they do not compromise the verified Codex path.

This phase does not require running Claude. It prepares the candidate for a later
manual Claude closeout by the user.

Success criteria:

- Compatibility differences from `claude-plugin/` are documented.
- Any added Claude-facing metadata is isolated and does not alter the Codex
  verification path.
- Remaining manual Claude checks are listed explicitly.

### Phase 4: Documentation and queue refresh

Update project memory and related research tickets to reflect the new two-plugin
state: `claude-plugin/` as the stable package, `agents-plugin/` as the Codex-first
candidate.

Success criteria:

- `ai-docs/_index.md` reflects the active todo and the new migration shape.
- The research anchor `260429-research-host-neutral-ws-plugin` points to this
  ticket as the first implementation slice.
- Follow-up tickets are identified for MCP exposure, broader skill normalization,
  or Claude manual closeout if needed.
