---
kind: print
---
# Workflow Manual

> **Session invariant:** Keep this reference active while writing or executing ws
> workflow skills. After compaction, call `ws/playbook.print(name: "lead-workflow-manual")` and execute the returned reference inline when primitive
> names or orchestration boundaries matter.

## On: invoke

Reading this file is the invocation; it loads the primitive reference.

---

# WS Workflow Primitives

Host-neutral notation reference for ws plugin skill text.

Use `ws/<tool-name>` for MCP server `ws`, tool `<tool-name>`.
Use `ws:` only for plugin skills such as `ws:lead-discuss`.
Write MCP calls as `ws/tool.name(arg: value)`.
Show optional arguments only when the skill needs a non-default value.
Omit `root` when the current repository root is intended.
Use `prompt: <block below>` or `question: <block below>` for large text payloads.
Write prompts sent to native Explore-style subagents and `ws/agents.call` in English.

When writing shared skill text, name only primitives that exist in the ws runtime.
If a workflow needs a surface that is still planned, state the required MCP
contract instead of naming a Claude helper command or another host-specific
fallback.
Centralize primitive usage here. Other skills should name the primitive and
include only local arguments that affect the current step.

## How To Document

- Write compressed professional prose: short sentences, exact verbs, no filler.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- Use `Not:` / `Use:` examples only for recurring mistakes.
- Keep procedure text command-shaped; move rationale to one short Doctrine paragraph.
- Use full sentences when compression could blur order, ownership, or safety.

## Available

### Session setup

`ws.lead.login`

At the start of any lead workflow session, call
`ws.lead.login(root: "<absolute-working-directory>")`. Pass the repository's
absolute filesystem path as `root`; the MCP server cannot infer the agent's
current directory from placeholders or relative paths. Thread the returned
`session_key` through every subsequent root-aware ws tool call.

### Scoped Exploration (native Explore)

`ws/playbook.print`
`ws/playbook.render`

Use for scoped fact-finding, surveys, and one-turn answers. Pattern: render the
`explore` playbook (`ws/playbook.render(name: "explore", context: {...})` to hand a
worker a brief file, or `ws/playbook.print(name: "explore")` for inline); spawn a
native Explore-style subagent with the rendered brief; collect the deferred result.
For parallel dispatch, spawn multiple concurrent subagents in a single turn and
collect all before synthesizing. Use a broad-tracing scope for wide structural surveys.

### Persistent agents

`ws/agents.register`
`ws/agents.call`
`ws/agents.wait`
`ws/agents.result`
`ws/agents.status`
`ws/agents.tail`
`ws/agents.print`
`ws/agents.cancel`
`ws/agents.erase`

Register a stable task name with a self-contained system prompt. Registration
takes `name`, optional `backend`, `system_prompt_text`, and `tier`; the removed
`prompts`/`prompt_refs`/`model` fields are gone. Omit `system_prompt_text` for a
general-purpose named agent; registration applies delegate orientation and the
default model alias. Call the agent for each continuity turn.
Bundled delegate prompts are not registered by stem — render them. Obtain a
delegate's self-contained prompt with `ws/playbook.render(name: "<delegate>")`
(model-alias vars auto-inject; a lead key splices a child-key credential block and
the call returns a `recommended-tier`). Hand the rendered prompt to a native
subagent (default), or pass it as `system_prompt_text` with `tier:
<recommended-tier>` to a mercenary `ws/agents.register` + `ws/agents.call`, then
collect through `ws/agents.result`. `reference-discovery` is such a delegate
playbook, not a workflow skill.
`ws/agents.call` starts async and returns promptly. Use
`wait(timeout_seconds: 600)` for readiness metadata, `result(timeout_seconds:
600)` or a longer bound for final output, `status` before waiting,
`tail(lines: 3)` for small diagnostics, `print` only as a compatibility output
alias, `cancel` to stop active work, retry `call` on the same registered agent
with a recovery prompt when cancellation followed a no-result timeout, and
`erase` when task-scoped state should be removed.

### Artifact paths

`ws/path.generate`

Use for generated workflow artifact paths. Capture returned paths. Relay paths,
not large findings, between lead, implementer, and reviewers.

### Runtime metadata

`ws/runtime.info`

Use for runtime compatibility checks and feature detection.

### Reference discovery

`ws/tickets.list`
`ws/tickets.find`
`ws/tickets.status`
`ws/specs.list`
`ws/specs.find`
`ws/specs.status`
`ws/mental_models.list`
`ws/mental_models.find`
`ws/mental_models.status`
`ws/references.trace`

Use these for ws-owned ticket, spec, and mental-model path/status/reference
lookup before shell search. Use native file reads after a discovery tool returns
the path to inspect or edit.

Prefer:
- `ws/tickets.list(status: "ready")` for implementation-ready discovery; use `status: "todo"` for accepted backlog.
- `ws/tickets.find(ticket_stem: "<stem>")` for ticket lookup by stem.
- `ws/tickets.find(mentions_ticket_stem: "<stem>")` for parent/related scans.
- `ws/tickets.status(ticket_stem: "<stem>", include_done: true)` for status checks.
- `ws/specs.find(spec_stem: "<stem>")` for anchor lookup.
- `ws/specs.find(ticket_stem: "<stem>")` for ticket-linked specs.
- `ws/specs.status(spec_stem: "<stem>")` for duplicate-safe anchor location.
- `ws/mental_models.find(query: "<topic>")` for domain discovery.
- `ws/mental_models.status(domain: "<domain>")` for known-domain docs.
- `ws/references.trace(ticket_stem: "<stem>")` for ticket/spec/model links.
- `ws/references.trace(spec_stem: "<stem>")` for spec/ticket/model links.

### Git

`ws/git.status`
`ws/git.diff`
`ws/git.log`
`ws/git.merge_base`
`ws/git.commit`

Use `ws/git.commit` for workflow commits when available. It stages explicit
paths, builds the `## AI Context` message, detects ticket moves plus
`### Result` and `#### Edition` headings, and avoids shell quoting drift.
For ticket status moves, use native `git mv` between status directories and commit through `ws/git.commit`; `ready/` is implementation-ready and `todo/` is accepted backlog.

Prefer:
- `ws/git.status()` for branch, staged state, and changed-file discovery.
- `ws/git.diff(mode: "stat")` before detailed review.
- `ws/git.diff(mode: "full", paths: ["<path>"])` for scoped inspection.
- `ws/git.log(range: "<base>..HEAD", include_body: true)` for commit audit.
- `ws/git.merge_base(base: "main", head: "HEAD")` for branch ranges.
- `ws/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])` for workflow commits.

Use native Git only for operations without an exposed ws primitive, such as
branch creation, tag push, merge execution, or path-filtered file history.

### API documentation

`ws/api.list`
`ws/api.ask`
`ws/api.ask_async`
`ws/api.status`
`ws/api.result`
`ws/api.cancel`

Use `ws/api.ask(prompt: "<prose question>")` for external API documentation.
Pass the natural-language question directly. Use `ws/api.list` to inspect cached domains.
Use `domain_hint` only when the intended domain is known.
The runtime owns pre-routing, per-domain sessions, stale checks, and cache access.
Use `ws/api.ask_async` for broad or bootstrap-prone lookups that may outlive the
host tool-call timeout. Store the returned `api_job_key`, then use
`ws/api.status`, `ws/api.result`, and `ws/api.cancel` to inspect, collect, or
stop the job.

## Planned Or Specialized

Treat active-agent listing and broad message-queue semantics as planned contract
surfaces unless the runtime exposes the exact tool. Basic async cancellation
exists through `ws/agents.cancel`; retry the same registered agent with
`ws/agents.call` for no-result cancellation recovery. Check runtime before
assuming richer interrupt behavior.

## Usage Pattern

```text
Scoped exploration:
render the explore playbook: `ws/playbook.render(name: "explore", context: {...})`.
spawn a native Explore-style subagent with the rendered brief path.
collect the result when the subagent returns.
for parallel dispatch, spawn multiple concurrent subagents in a single turn; collect all before synthesizing.

Persistent task:
call `ws/agents.register(name: "<agent-name>")` for a general-purpose delegate.
for a bundled delegate, render its prompt: `ws/playbook.render(name: "<delegate>")`, then spawn a native subagent or register a mercenary with `system_prompt_text: <rendered>` and `tier: <recommended-tier>`.
call `ws/agents.call(name: "<agent-name>", prompt: <block below>)` for the mercenary path.
wait for readiness, read final output with `result(timeout_seconds: 600)`, inspect status, or tail with `lines: 3`.
erase the task-scoped agent when cleanup matters.

Review artifacts:
call `ws/path.generate(kind: "review", stems: ["<stem>"])`.
tell reviewers to write full findings to those paths.
relay file paths, not full findings, to the implementer.

API docs:
call `ws/api.list()` when choosing among cached domains matters.
call `ws/api.ask(prompt: "<prose API documentation question>")` for external API lookup.
add `domain_hint: "<optional-domain>"` only when the intended domain is known.
call `ws/api.ask_async(prompt: "<long-running question>")` when cache bootstrap,
slow upstream fetch, or broad multi-domain routing can exceed the host timeout.
store `<api_job_key>`; call `ws/api.status(api_job_key: "<key>")`, then
`ws/api.result(api_job_key: "<key>")`, or `ws/api.cancel(api_job_key: "<key>")`.

References:
call `ws/references.trace(ticket_stem: "<ticket-stem>")` for ticket/spec/model links.
call `ws/references.trace(spec_stem: "<spec-stem>")` for spec/ticket/model links.
call domain discovery tools first when only paths or status metadata are needed.

Commit:
call `ws/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])`.
```

## Doctrine

Workflow notation optimizes for **limited execution attention** during cross-host
work. References must survive skill execution and map to each host's tool
display. When ambiguous, preserve execution attention.
