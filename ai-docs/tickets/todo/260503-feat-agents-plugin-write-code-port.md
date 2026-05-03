---
title: agents-plugin write-code port
parent: 260503-epic-agents-plugin-skill-porting
related:
  260503-feat-agents-plugin-agent-session-runtime: named agent registry and prompt-chain baseline
  260503-feat-agents-plugin-async-agent-calls: async call, wait, tail, status, cancel primitives
  260503-feat-agents-plugin-edit-port: first core orchestration port with generated review paths and one reviewer
  260503-epic-agents-plugin-skill-porting: parent roadmap for staged skill porting
---

# agents-plugin write-code port

## Background

`write-code` is the core delegated implementation primitive in the ws workflow.
It reads a ticket or inline target, writes a focused brief, optionally expands
that brief into a plan, delegates implementation to an implementer session, fans
out review to partitioned reviewers, relays findings through a bounded loop, and
returns the commit range and verification status to its caller.

The `edit` port proved the smallest reviewer loop, and the forge skill ports
provided a live smoke for persistent Codex-backed named implementer sessions:
one named agent edited files, accepted review feedback in the same session, and
was reused for a second scoped port while the lead retained diff review and
commit ownership. `write-code` should now absorb the Claude `ws-named-agent`
prior art into the shared `agents-plugin` skill text, but it must explicitly
close the remaining runtime and policy gaps before claiming parity.

## Decisions

- Preserve `write-code` as a delegated implementation primitive; lead-owned
  direct edits remain the responsibility of `edit`.
- Keep the workflow on the current branch; branch creation and merge harnesses
  remain out of scope for this ticket.
- Preserve file-backed review findings and the partitioned reviewer model.
- Expand the embedded prompt bundle instead of depending on downstream
  `claude-plugin/infra/prompts/*` paths.
- Use `ws/path.generate` for review files and keep lead-side cleanup by absolute
  paths for this ticket.
- Treat implementer commit ownership as an explicit smoke target before the
  shared skill instructs delegates to commit.
- Keep `implement`, `proceed`, and `sprint` out of scope; they get separate
  harness tickets after `write-code` is runnable.

## Constraints

- Do not mutate `claude-plugin/skills/write-code` during this port.
- Shared skill text must use `ws/<tool>` notation and avoid host-specific
  helper commands as the main contract.
- Shared skill text must not reference repo-local `claude-plugin/` paths.
- Review relay remains capped at three cycles.
- Reviewers write complete findings to files; summaries remain concise.
- Fit reviewer may consult the ticket for architectural headroom; correctness
  and test reviewers should stay scoped to diff, brief, and tests.
- `ws:update-spec`, mental-model update, branch creation, approval gates, and
  merge behavior remain caller or harness responsibilities.

## Prior Art

Claude `write-code`:

- reads a ticket or inline target and writes a brief under `ai-docs/.plans/`
- calls `project-survey` before writing the brief
- optionally calls `plan-populator-survey` or `plan-populator-research`
- registers `implementer`, `reviewer-correctness`, `reviewer-fit`, and
  `reviewer-test` named agents
- allocates `correctness`, `fit`, and `test` review paths
- runs the implementer in the background and lets it commit logical checkpoints
- fans out reviewers in parallel
- relays findings through at most three cycles
- asks reviewers to accept or maintain implementer won't-fix dispositions
- deletes review paths and returns commit range, test status, and brief path

## Phases

### Phase 1: Embedded prompt bundle expansion

Add the prompt presets needed by `write-code` to the `ws-mcp` embedded prompt
bundle and runtime metadata.

Success criteria:

- Embed host-neutral versions of `implementer`, `project-survey`,
  `plan-populator-survey`, `plan-populator-research`, and
  `code-review-test`.
- Preserve prompt-chain compatibility through `prompts` on `ws/agents.register`
  and `ws/agents.oneshot`.
- Normalize prompts away from Claude-only helper names and repo-local paths.
- Preserve workload-tier intent from Claude frontmatter where useful.
- Update `runtime.json` prompt bundle metadata so launcher drift detection
  repairs stale runtimes.
- Tests cover prompt resolution and prompt bundle metadata for the new stems.
- Verification covers Go tests, MCP smoke, plugin validation, runtime JSON
  parsing, and launcher drift behavior where practical.

### Phase 2: Controlled delegated commit smoke

Verify whether Codex-backed named implementer sessions can safely own commits
before the shared `write-code` skill instructs delegates to commit.

Success criteria:

- Register a named implementer with the embedded `implementer` prompt.
- Give it a narrow, reversible task in a disposable or low-risk scope.
- Allow the delegate to edit and commit exactly one logical change.
- Lead verifies the diff, commit message shape, and `## AI Context` quality.
- Lead verifies that `ws/agents.print`, `ws/agents.tail`, and session resume are
  enough to debug the run.
- If delegate commit ownership is unreliable, record a ticket revision that
  keeps lead-owned commits in the first `write-code` port.
- If delegate commit ownership is reliable, keep the prior-art behavior where
  the implementer commits logical checkpoints.

### Phase 3: Port `write-code` skill draft

Create `agents-plugin/skills/write-code/SKILL.md` as a host-neutral port of the
Claude skill.

Success criteria:

- The skill follows `ai-docs/ref/skill-authoring.md`.
- Target parsing, brief writing, optional plan depth, skeleton gate,
  implementer delegation, reviewer fanout, relay loop, cleanup, and completion
  report are preserved.
- All named agent registration uses embedded prompt stems through
  `ws/agents.register`.
- Long-running implementer and reviewer turns use `ws/agents.call_async`,
  `ws/agents.wait`, `ws/agents.status`, `ws/agents.tail`, and
  `ws/agents.print` where appropriate.
- Review paths are allocated through `ws/path.generate` with stems
  `correctness`, `fit`, and `test`.
- The skill states the selected commit ownership policy from Phase 2.
- The skill avoids downstream-breaking references to this repository's
  `claude-plugin/` source paths.

### Phase 4: Runtime smoke and documentation closeout

Smoke the new `write-code` surface enough to prove that the runtime primitives
and prompt bundle can support the skill.

Success criteria:

- `agents-plugin` validates.
- `ws-mcp` tool listing includes every runtime primitive named by the skill.
- Prompt bundle metadata includes every embedded prompt stem named by the skill.
- A controlled Codex run exercises implementer registration, at least one
  reviewer registration, review path allocation, output recovery, and cleanup.
- A host plugin refresh confirms `ws:write-code` is visible when available.
- Any unverified Claude compatibility, Windows behavior, or delegate commit
  limitation is documented rather than implied.
