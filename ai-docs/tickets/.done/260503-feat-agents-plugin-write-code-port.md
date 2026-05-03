---
title: agents-plugin write-code port
parent: 260503-epic-agents-plugin-skill-porting
related:
  260503-feat-agents-plugin-agent-session-runtime: named agent registry and prompt-chain baseline
  260503-feat-agents-plugin-async-agent-calls: async call, wait, tail, status, cancel primitives
  260503-feat-agents-plugin-edit-port: first core orchestration port with generated review paths and one reviewer
  260503-epic-agents-plugin-skill-porting: parent roadmap for staged skill porting
completed: 2026-05-03
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
- Embed `impl-playbook` so the `write-code` caller can register the implementer
  with `prompts: ["implementer", "impl-playbook"]` instead of making the
  implementer fetch runtime policy at task start.
- Preserve prompt-chain compatibility through `prompts` on `ws/agents.register`
  and `ws/agents.oneshot`.
- Normalize prompts away from Claude-only helper names and repo-local paths.
- Preserve workload-tier intent from Claude frontmatter where useful.
- Update `runtime.json` prompt bundle metadata so launcher drift detection
  repairs stale runtimes.
- Tests cover prompt resolution and prompt bundle metadata for the new stems.
- Verification covers Go tests, MCP smoke, plugin validation, runtime JSON
  parsing, and launcher drift behavior where practical.

### Result (fd3b61f) - 2026-05-03

Expanded the embedded `ws-mcp` prompt bundle with the prompt presets needed by
the upcoming `write-code` port: `implementer`, `project-survey`,
`plan-populator-survey`, `plan-populator-research`, `code-review-test`, and
`impl-playbook`. The source material was copied from the Claude prior art, then
lightly normalized away from host-specific helper command names, explicit
`CLAUDE.md` dependence, and repo-local path assumptions.

`impl-playbook` is now an embedded prompt stem so the `write-code` caller can
materialize implementation policy through a prompt chain such as
`prompts: ["implementer", "impl-playbook"]`, instead of requiring the
implementer delegate to fetch policy through MCP at task start. Runtime metadata
now records prompt bundle hash
`c68893663ac91db1fb5b186bb3f4b62762099defa69ec703302035e67512cb26` and lists
all ten embedded stems for launcher drift detection.

Validation covered stale-helper searches across the embedded prompt bundle, Go
tests, MCP smoke, plugin manifest validation, runtime JSON parsing, and
whitespace checks. Host-loaded plugin sessions still need the normal cache
refresh before the expanded prompt bundle is visible.

### Phase 2: Controlled delegated commit smoke

Verify whether Codex-backed named implementer sessions can safely own commits
before the shared `write-code` skill instructs delegates to commit.

Success criteria:

- Register a named implementer with embedded prompts `implementer` and
  `impl-playbook`.
- Give it a narrow, reversible task in a disposable or low-risk scope.
- Allow the delegate to edit and commit exactly one logical change.
- Lead verifies the diff, commit message shape, and `## AI Context` quality.
- Lead verifies that `ws/agents.print`, `ws/agents.tail`, and session resume are
  enough to debug the run.
- If delegate commit ownership is unreliable, record a ticket revision that
  keeps lead-owned commits in the first `write-code` port.
- If delegate commit ownership is reliable, keep the prior-art behavior where
  the implementer commits logical checkpoints.

### Result (72a20cb) - 2026-05-03

Verified delegated commit ownership on the synchronous named-agent path. A
Codex-backed `smoke-implementer` was registered with embedded prompts
`implementer` and `impl-playbook`, given a single-file test-only task, and
allowed to create exactly one commit. The delegate added
`TestResolveWriteCodeImplementerPolicyChain` to
`agents-plugin-tool/internal/wsprompt/prompts_test.go`, ran
`go test ./internal/wsprompt`, and committed the change as `72a20cb` with a
detailed `## AI Context` body.

Lead-side review confirmed that the diff stayed within scope, the commit message
explained the user intent and verification boundary, `ws/agents.print` and
`ws/agents.tail` recovered useful output, and session state recorded a reusable
Codex session id. This supports keeping the prior-art behavior where the
implementer may commit logical checkpoints.

The same smoke exposed a blocker for the asynchronous path used by `write-code`.
Two `call_async` attempts, one via `go run` and one via a stable temporary
binary, reached `call_async.worker_started` and `call.started`, then the worker
process exited while `current/state.json` and `agent.json` remained `running`
with empty stdout/stderr/output. `ws/agents.wait` timed out and `ws/agents.status`
continued to report the dead pid as active. Before porting `write-code`, the
runtime needs a failure-finalization fix so async worker exits cannot leave
stale-running agent state.

Validation after the delegate commit covered lead inspection of the commit and
diff, `go test ./...`, plugin manifest validation, whitespace checks, and cleanup
of the smoke agent registry entry.

### Phase 3: Async worker failure finalization

Fix the async agent runtime so dead or failed worker processes are reflected in
current-call state and can be diagnosed by `wait`, `status`, and `tail`.

Success criteria:

- Reproduce the stale-running case with a focused test or local smoke.
- Ensure worker process exit without `CompleteCurrentCall` marks the current
  call failed or otherwise non-active.
- Preserve stdout/stderr capture for backend errors.
- Append per-agent async worker lifecycle diagnostics to a JSONL log such as
  `current/runtime.jsonl`.
- Log enough lifecycle points to localize silent exits: worker entry, prompt
  read, backend call start, backend session id, backend exit or error, state
  finalization begin/end, and panic recovery.
- Make `ws/agents.wait` stop waiting when the recorded worker process is dead.
- Make `ws/agents.status` and `ws/agents.tail` expose enough failure detail for
  lead-side diagnosis, including recent runtime lifecycle log lines.
- Re-run the controlled async implementer smoke enough to prove that failure
  states close cleanly, even if the delegate task itself fails.

### Result (5351bb1) - 2026-05-03

Fixed async worker finalization and diagnostics in `ws-mcp`. Each current call
now owns `current/runtime.jsonl`, and `RunCurrent` logs worker entry, prompt
read, backend call start, streamed session id, backend completion or error,
state finalization, and panic recovery. `ws/agents.tail` includes a runtime
section, while `ws/agents.wait` and `ws/agents.status` reconcile active
current-call state before reporting it.

The original stale-running symptom was reproduced with a local CLI smoke: the
recorded worker pid died while current-call state remained active. The fix now
marks dead worker pids as failed with an explicit error and updates agent state,
so `wait` returns failure status instead of timing out indefinitely. The
diagnostics then narrowed the remaining live smoke failure to the automatic
worker launch path: `run-current` succeeded when invoked manually, but the
child process launched by `call-async` could disappear before entering
`RunCurrent`. `SelfWorkerStarter` now starts workers in a separate process group
and redirects both stdout and stderr to current-call stream files.

Validation covered focused unit tests for dead-worker reconciliation, backend
failure diagnostics, panic diagnostics, runtime tail output, and full `go test
./...`. A local `ws-mcp` binary smoke using `agents call-async` with embedded
`implementer` and `impl-playbook` prompts completed successfully: `wait`
returned `async smoke ok`, `tail` showed worker entry through finalization, and
stdout captured Codex JSONL including the streamed thread id.

### Phase 4: Port `write-code` skill draft

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

### Result (39f3fb4) - 2026-05-03

Added `agents-plugin/skills/write-code/SKILL.md` as a host-neutral draft of the
delegated implementation primitive. The skill preserves the Claude prior-art
shape: target parsing, project survey, brief writing, optional plan population,
skeleton gate, async implementer delegation, partitioned reviewer fanout,
file-backed findings, bounded relay loop, cleanup, and completion reporting.

The port uses `ws/agents.oneshot` for project survey and plan population,
`ws/agents.register` with embedded prompt stems for all named agents,
`ws/agents.call_async` plus wait/status/tail/print for long-running turns, and
`ws/path.generate` for correctness, fit, and test review files. It records the
Phase 2 commit policy by allowing the implementer to commit logical checkpoints
on the current branch, while leaving branch creation, approval gates, merge, and
spec updates to the caller or later harness skills.

Direct Git operations for recording the start commit and committing brief/plan
checkpoints remain lead actions because `ws/git.*` tools are now tracked by the
separate `260503-epic-ws-mcp-vcs-reference-tools` roadmap rather than available
runtime primitives. The skill does not reference `claude-plugin/` source paths
or PATH-injected `ws-*` helpers. Validation covered skill-authoring review,
host-specific reference search, ASCII check, and `claude plugin validate
agents-plugin`. A `ws/subquery` audit attempt timed out after 120 seconds, so no
independent delegate audit result was incorporated in this phase.

### Phase 5: Local runtime smoke and documentation closeout

Smoke the new `write-code` surface enough to prove that the local runtime
primitives and prompt bundle can support the skill.

Success criteria:

- `agents-plugin` validates.
- `ws-mcp` tool listing includes every runtime primitive named by the skill.
- Prompt bundle metadata includes every embedded prompt stem named by the skill.
- A controlled Codex run exercises implementer registration, at least one
  reviewer registration, review path allocation, output recovery, and cleanup.
- Any unverified Claude compatibility, Windows behavior, or delegate commit
  limitation is documented rather than implied.

### Result (a386ae1, 1a9f5aa) - 2026-05-03

Completed the local smoke surface for the new `write-code` skill. Validation
covered `claude plugin validate agents-plugin`, `go test ./...`,
`git diff --check`, prompt bundle metadata for every prompt stem named by the
skill, MCP `tools/list` coverage for every runtime primitive named by the
skill, `ws/path.generate` allocation for correctness/fit/test review files, and
Codex-backed async calls for one implementer and one reviewer. The smoke
confirmed `ws/agents.wait`, `ws/agents.print`, and `ws/agents.tail` can recover
the expected output and lifecycle diagnostics, and the smoke agents were erased.

The independent `ws/subquery` audit that previously timed out was retried after
the user restarted the MCP server. A small smoke query completed in about seven
seconds, and a bounded `write-code` skill audit completed in about twenty-two
seconds with no orphaned `codex exec` or nested `ws-mcp` process left behind.
The audit found one completion-report gap: the report recorded agent cleanup but
not generated review-file cleanup. The skill now includes `Review files:
deleted | <remaining cleanup issue>` in `Templates / Completion Report`.

The timeout incident also produced runtime hardening in `a386ae1`: `ws/subquery`
now has a default 90 second backend timeout, synchronous oneshot/call paths can
carry a timeout, and Codex backend processes are launched in a cancellable
process group so timeout cleanup can kill nested child processes without
disabling recursive MCP. A fuller recursion-depth and remaining-budget policy is
still future work rather than part of this phase.

### Phase 6: Host plugin visibility closeout

Confirm the installed host plugin cache sees the new skill and document any
remaining compatibility gaps.

Success criteria:

- A user-performed Codex plugin refresh or restart confirms `ws:write-code` is
  visible in the installed skill list.
- The installed MCP runtime reports the prompt bundle and tool surface expected
  by the local smoke.
- Any unverified Claude compatibility, Windows behavior, recursive MCP budget
  policy, or delegate commit limitation is documented before closing this
  ticket.

### Result (installed cache) - 2026-05-03

Closed the host visibility gate after a user-performed Codex plugin refresh and
session restart. The current Codex session exposes `ws:write-code` in the
installed skill list, and the cached plugin tree contains
`skills/write-code/SKILL.md`.

The installed MCP runtime reports version `0.1.0-dev`, source commit `dev`, and
prompt bundle hash
`c68893663ac91db1fb5b186bb3f4b62762099defa69ec703302035e67512cb26`, matching
the local smoke metadata. Runtime inspection confirmed the embedded stems used
by `write-code`: `implementer`, `impl-playbook`, `project-survey`,
`plan-populator-survey`, `plan-populator-research`, `code-reviewer`,
`code-review-correctness`, `code-review-fit`, and `code-review-test`.

The final closeout also confirmed that the current MCP session can call
`runtime.info` and that the installed binary can allocate a generated review
path through `path generate`. Plugin validation still passes for
`agents-plugin`. Remaining compatibility gaps are intentionally outside this
ticket: real Claude runtime invocation for the candidate plugin, native Windows
plugin-managed launcher behavior, and a fuller recursive-MCP depth or remaining
budget policy. Delegate commit ownership has already been smoke-tested in Phase
2 and remains accepted for this port.
