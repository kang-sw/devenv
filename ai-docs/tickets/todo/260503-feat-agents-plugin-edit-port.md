---
title: agents-plugin edit port
parent: 260503-epic-agents-plugin-skill-porting
related:
  260503-feat-agents-plugin-agent-session-runtime: named agent registry and synchronous delegation baseline
  260503-feat-agents-plugin-async-agent-calls: async reviewer execution and inspection primitives
  260503-feat-agents-plugin-runtime-boundary: ws-mcp retained process and plugin launcher boundary
  260503-epic-agents-plugin-skill-porting: parent roadmap for staged skill porting
---

# agents-plugin edit port

## Background

`edit` should be the first core implementation orchestration skill ported into
`agents-plugin/`. It is narrower than `write-code`, `implement`, `proceed`, and
`sprint`: the lead performs the edit directly on the current branch, then uses a
single reviewer delegate to check correctness and fit. That makes it a useful
bridge between the completed direct-execution skills and the heavier delegated
implementation workflows.

The Claude prior art uses host-specific helpers for implementation playbook
loading, mental-model lookup, named reviewer registration, review-path
allocation, background reviewer execution, output printing, cleanup, and
`update-spec`. The host-neutral port should preserve the direct-edit shape while
replacing helper dependencies with ws MCP primitives and explicit fallback
boundaries.

## Decisions

- Port `edit` before `write-code` because `edit` combines lead-owned
  implementation with one reviewer delegate and therefore validates the smallest
  useful core orchestration loop.
- Keep the lead as the only implementation actor; reviewer agents remain
  read-only and never edit files or create commits.
- Preserve file-backed review findings instead of relying only on reviewer chat
  summaries, because the lead must be able to inspect complete findings after a
  non-clean review.
- Treat generated workflow paths as a runtime primitive, not skill-local
  temp-file string construction. `edit` needs `kind: "review"` first, but future
  workflow slices may need the same primitive for handoff notes, scratch
  artifacts, or other retained text files.
- Resolve prompt chains in the runtime instead of embedding reviewer prompt text
  in the `edit` skill. Prompt presets should be embedded into the single
  `ws-mcp` binary and checked against `runtime.json` metadata for plugin/runtime
  drift.

## Constraints

- Do not mutate `claude-plugin/skills/edit` during this port.
- Shared skill text must use `ws/<tool>` notation and avoid host-specific
  `ws-*` helper commands as the main contract.
- The first port must support runtime-resolved prompt chains before `edit`
  depends on reviewer presets.
- Prompt drift detection should compare prompt bundle content hashes, with source
  commit hashes preserved for provenance and context recovery.
- Review relay remains bounded at two cycles.
- `ws:update-spec` invocation remains a skill-level handoff in Codex unless a
  future runtime primitive replaces it.

## Prior Art

Claude `edit`:

- records the start commit before editing
- loads mental models and `impl-playbook`
- performs direct lead edits and verification
- registers one `reviewer` named agent with `code-reviewer`,
  `code-review-correctness`, and `code-review-fit`
- allocates one review path through `ws-review-path direct`
- runs the reviewer in the background
- reads reviewer summary via named-agent output and full findings from the
  review path
- relays fixes for at most two review cycles
- deletes the review path and runs `update-spec` over the edit commit range

## Phases

### Phase 1: Generated workflow paths

Add a host-neutral generated-path allocation surface to `ws-mcp`. The first
supported path kind should be `review`, using the existing project-state path
manager instead of `/tmp/claude-reviews` so paths are scoped under the ws cache
layout chosen for this migration.

Success criteria:

- MCP exposes `ws/path.generate` with required `kind` and `stems` arguments.
- CLI fallback `ws-mcp path generate` exists for local smoke and
  Claude-compatibility planning.
- Initial supported kind is `review`, mapped to the worktree-local
  `review-paths/` state directory.
- Allocation accepts one or more logical stems and returns concrete writable
  paths in stable order.
- Paths are unique per allocation and safe for concurrent workflow invocations.
- Tests cover path kind validation, stem sanitization, multi-path allocation,
  uniqueness, and root/project scoping.
- `agents-plugin/skills/workflow` marks generated workflow path allocation as
  available after the primitive lands.

### Result (0569982) - 2026-05-03

Implemented generated workflow path allocation as MCP tool `ws/path.generate`
and CLI fallback `ws-mcp path generate`. The first supported kind is `review`,
which maps to the worktree-local `review-paths/` directory managed by
`wsstate`. Each allocation accepts one or more stems, sanitizes them, preserves
input order, reserves empty writable `.md` files, and adds a timestamp plus
random run id so repeated and concurrent allocations do not reuse paths.

Updated runtime metadata, MCP tool listing, CLI smoke, workflow skill
availability text, and ws MCP/runtime references. Tests cover review path
allocation, stem sanitization, unsupported kind errors, missing stem errors,
multi-path stable ordering, uniqueness, and worktree scoping. Validation covered
Go tests, MCP smoke, plugin validation, runtime JSON parsing, shell syntax, and
whitespace checks.

### Phase 2: Embedded prompt bundle resolver

Implement prompt-chain resolution in `ws-mcp` before porting `edit`. The runtime
should embed prompt presets into the Go binary and materialize the same chained
system prompt shape that Claude `ws-named-agent new ... -p ...` used.

Success criteria:

- Add a prompt package with embedded preset files copied from the current Claude
  prompt prior art needed by `edit`: `code-reviewer`,
  `code-review-correctness`, and `code-review-fit`.
- Expose `prompts` on `ws/agents.register` and `ws/agents.oneshot`; keep
  `prompt_refs` as a compatibility alias during migration.
- Resolve bare prompt stems from embedded presets, and resolve absolute paths by
  reading those files directly. Reject ambiguous relative paths unless a later
  ticket defines root-relative behavior.
- Strip YAML frontmatter from resolved prompt files and concatenate prompt
  bodies in input order with the existing `---` separator convention.
- Preserve frontmatter-derived tier/model behavior where it applies to known
  workload tiers, while keeping explicit `tier` or `model` arguments higher
  priority.
- Write the materialized prompt to the agent `system.md` so calls continue to
  use backend-specific system prompt injection.
- Add runtime metadata for prompt compatibility: prompt bundle source commit,
  prompt bundle content SHA-256, and embedded prompt stem list.
- Expose a runtime info surface through MCP or CLI so the launcher and smoke
  tests can compare the installed binary against `agents-plugin/runtime.json`.
- Extend launcher compatibility checks to detect prompt bundle drift in addition
  to tool and command drift.
- Tests cover prompt stem resolution, absolute path resolution, path traversal or
  relative path rejection, frontmatter stripping, prompt ordering, tier/model
  precedence, prompt metadata, and drift detection.

### Result (pending) - 2026-05-03

Implemented the embedded prompt bundle resolver in `ws-mcp`. The runtime now
embeds the prompt presets needed by the first `edit` slice:
`code-reviewer`, `code-review-correctness`, and `code-review-fit`.
`ws/agents.register` and `ws/agents.oneshot` accept `prompts` as the canonical
prompt-chain field while keeping `prompt_refs` as a migration alias. Bare stems
resolve from the embedded bundle; absolute paths read directly; relative and
traversal-like specs are rejected.

Prompt materialization strips YAML frontmatter, concatenates prompt bodies in
input order with the existing `---` separator convention, appends
`system_prompt_text` for compatibility, maps legacy Claude frontmatter model
names to shared tiers when no explicit tier or model is supplied, and writes the
result to each agent's `system.md`.

Added `ws/runtime.info` and `ws-mcp runtime info` so runtime metadata exposes
the prompt bundle source commit, content SHA-256, and embedded prompt list.
`agents-plugin/runtime.json` records the expected bundle hash, and the POSIX
launcher now repairs stale binaries when prompt bundle drift is detected in
addition to tool or command surface drift. Release builds inject the source
commit through `-ldflags`.

Validation covered Go tests, MCP smoke, plugin validation, runtime JSON parsing,
shell syntax checks, launcher positive smoke, launcher prompt-bundle drift
failure smoke, and whitespace checks. The plugin cache still requires the normal
user reinstall/restart path before the new runtime surface is visible in a host
session.

### Phase 3: Port `edit` skill draft

Create `agents-plugin/skills/edit/SKILL.md` as a host-neutral port of the Claude
skill. Preserve the direct-edit lifecycle while replacing helper commands with
MCP calls and current Codex skill handoffs.

Success criteria:

- The skill follows `ai-docs/ref/skill-authoring.md`.
- The lead-owned edit, logical commits, verification loop, two-cycle review
  relay, cleanup, and completion report are preserved.
- Mental-model lookup, convention/playbook loading, named reviewer calls,
  review-path allocation, output recovery, and cleanup use available ws MCP
  primitives or explicitly labeled skill handoffs.
- The skill avoids downstream-breaking references to this repository's
  `claude-plugin/` source paths.

### Phase 4: Codex smoke and documentation

Smoke the new `edit` surface enough to prove that Codex can load the skill and
that the MCP primitives it names are present. Use a narrow no-op or docs-only
scenario if a real code edit would create unnecessary churn.

Success criteria:

- `agents-plugin` validates.
- `ws-mcp` tool listing includes every runtime primitive named by the skill.
- A Codex/plugin smoke confirms `ws:edit` is visible after reinstall/restart
  when host cache refresh is available.
- Any unverified Claude compatibility or Windows behavior is documented rather
  than implied.
