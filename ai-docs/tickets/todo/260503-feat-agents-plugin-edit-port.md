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
- Treat review-path allocation as a runtime primitive, not skill-local temp-file
  string construction, so future `write-code`, `implement`, and `sprint` ports
  can reuse the same path manager.

## Constraints

- Do not mutate `claude-plugin/skills/edit` during this port.
- Shared skill text must use `ws/<tool>` notation and avoid host-specific
  `ws-*` helper commands as the main contract.
- The first port may embed reviewer instructions through `system_prompt_text`;
  prompt-bundle resolution can remain a follow-up unless the implementation
  needs it to keep the skill executable.
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

### Phase 1: Review-path primitive

Add a host-neutral review-path allocation surface to `ws-mcp`. The tool should
use the existing project-state path manager instead of `/tmp/claude-reviews` so
paths are scoped under the ws cache layout chosen for this migration.

Success criteria:

- MCP exposes a review-path allocation tool usable from shared skill text.
- CLI fallback exists for local smoke and Claude-compatibility planning.
- Allocation accepts one or more logical stems and returns concrete writable
  paths in stable order.
- Paths are unique per allocation and safe for concurrent workflow invocations.
- Tests cover stem sanitization, multi-path allocation, and root/project scoping.
- `agents-plugin/skills/workflow` marks review-path allocation as available
  after the primitive lands.

### Phase 2: Reviewer prompt materialization

Decide how `edit` should provide reviewer instructions in the first host-neutral
port. Prefer a self-contained `system_prompt_text` assembled in the skill if
prompt-bundle resolution is still too broad for this slice.

Success criteria:

- The reviewer system prompt covers read-only behavior, correctness checks, fit
  checks, severity, scoped findings, and the expected summary line.
- The prompt tells the reviewer to write full findings to the allocated review
  path and return only `[clean|non-clean]: <summary>`.
- The approach is documented as either embedded prompt text or prompt-reference
  resolution, with follow-up gaps preserved.

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
