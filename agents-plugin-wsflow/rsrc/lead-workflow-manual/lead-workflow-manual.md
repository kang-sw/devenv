---
kind: print
---
# Workflow Manual

> **Session invariant:** Keep this reference active while writing or executing ws
> workflow skills. After compaction, call `{{.McpNamespace}}/playbook.print(name: "lead-workflow-manual")` and execute the returned reference inline when primitive
> names or orchestration boundaries matter.

## On: invoke

Reading this file is the invocation; it loads the primitive reference.

---

# WS Workflow Primitives

Host-neutral notation reference for {{.McpNamespace}} plugin skill text.

Use `{{.McpNamespace}}/<tool-name>` for the current MCP namespace and tool `<tool-name>`.
Use `{{.SkillNamespace}}:` only for plugin skills such as `{{.SkillNamespace}}:lead-discuss`.
Write MCP calls as `{{.McpNamespace}}/tool.name(arg: value)`.
Show optional arguments only when the skill needs a non-default value.
Omit `root` when the current repository root is intended.
Use `prompt: <block below>` or `question: <block below>` for large text payloads.
Write prompts sent to native Explore-style subagents in English.
<!-- ws:full-only:start -->
Write prompts sent to `ws.mercenary.call` in English.
<!-- ws:full-only:end -->

When writing shared skill text, name only primitives that exist in the {{.McpNamespace}} runtime.
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

`ws.ferrule`

At the start of any lead workflow session, call
`ws.ferrule(root: "<absolute-working-directory>")` to mint your session key.
The name is deliberately non-descriptive and is taught only here: it is the lead
session-bootstrap call, so subagents that share this MCP connection have no
semantic cue to invoke it. Pass the repository's absolute filesystem path as
`root`; the MCP server cannot infer the agent's current directory from
placeholders or relative paths. Each key binds to one canonical repository
root — the git top-level of the path you pass — and a git worktree resolves to
its own top-level, so it counts as a distinct root. Call `ws.ferrule` once per
working root, and thread the matching `session_key` through every subsequent
root-aware {{.McpNamespace}} tool call that targets that root.

### User preferences

<!-- ws:override:UserPreferenceSection desc="user standing preferences for communication, terminology, and workflow behavior" -->
<!-- ws:/override:UserPreferenceSection -->

### Delegation posture

<!-- ws:override:DelegationSection desc="lead delegation eagerness and context-saving stance" -->
Delegate to preserve lead execution context. Hand off parallelizable
fact-finding, multi-file surveys, and self-contained implementation or review
slices to subagents, and keep the lead loop on routing, adjudication, and
synthesis. Keep work that is faster to do than to brief — small, local,
single-step edits — inline. When context budget runs short, lean harder toward
delegation.
<!-- ws:/override:DelegationSection -->

For lead-owned tuning of this posture or other workflow knobs, use the `{{.SkillNamespace}}:lead-tune` skill.

### Scoped Exploration (native Explore)

Use for scoped fact-finding, surveys, and one-turn answers. Pattern: spawn a
host-native exploration worker directly with an English prompt that includes
the scoped question or purpose-specific query block; require cited evidence,
gaps, and follow-up needs; collect the deferred result. For parallel dispatch, spawn
multiple concurrent subagents in a single turn and collect all before
synthesizing. Use a broad-tracing scope for wide structural surveys.

<!-- ws:full-only:start -->
### Persistent agents

`ws.mercenary.register`
`ws.mercenary.call`
`ws.mercenary.wait`
`ws.mercenary.result`
`ws.mercenary.status`
`ws.mercenary.tail`
`ws.mercenary.print`
`ws.mercenary.cancel`
`ws.mercenary.erase`

Register a stable task name with a self-contained system prompt. Registration
takes `name`, optional `backend`, `system_prompt_text`, and `tier`; the removed
`prompts`/`prompt_refs`/`model` fields are gone. Omit `system_prompt_text` for a
general-purpose named agent; registration applies delegate orientation and the
default tier mapping. Call the agent for each continuity turn.
Bundled delegate prompts are not registered by stem — render them. Obtain a
delegate's self-contained prompt with `{{.McpNamespace}}/playbook.render(name: "<delegate>")`
(the tier-derived model-hint var auto-injects; a lead key splices a child-key credential block and
the call returns a `recommended-tier`). Hand the rendered prompt to a native
subagent (default), or pass it as `system_prompt_text` with `tier:
<recommended-tier>` to a mercenary `ws.mercenary.register` + `ws.mercenary.call`, then
collect through `ws.mercenary.result`. `reference-discovery` is such a delegate
playbook, not a workflow skill.
`ws.mercenary.call` starts async and returns promptly. Use
`wait(timeout_seconds: 600)` for readiness metadata, `result(timeout_seconds:
600)` or a longer bound for final output, `status` before waiting,
`tail(lines: 3)` for small diagnostics, `print` only as a compatibility output
alias, `cancel` to stop active work, retry `call` on the same registered agent
with a recovery prompt when cancellation followed a no-result timeout, and
`erase` when task-scoped state should be removed.
<!-- ws:full-only:end -->

### Artifact paths

`{{.McpNamespace}}/path.generate`

Use for generated workflow artifact paths. Capture returned paths. Relay paths,
not large findings, between lead, implementer, and reviewers.

### Runtime metadata

`{{.McpNamespace}}/runtime.info`

Use for runtime compatibility checks and feature detection.

### Reference discovery

`{{.McpNamespace}}/tickets.list`
`{{.McpNamespace}}/tickets.find`
`{{.McpNamespace}}/tickets.status`
`{{.McpNamespace}}/specs.list`
`{{.McpNamespace}}/specs.find`
`{{.McpNamespace}}/specs.status`
`{{.McpNamespace}}/mental_models.list`
`{{.McpNamespace}}/mental_models.find`
`{{.McpNamespace}}/mental_models.status`
`{{.McpNamespace}}/references.trace`

Use these for {{.McpNamespace}}-owned ticket, spec, and mental-model path/status/reference
lookup before shell search. Use native file reads after a discovery tool returns
the path to inspect or edit.

Prefer:
- `{{.McpNamespace}}/tickets.list(status: "ready")` for implementation-ready discovery; use `status: "todo"` for accepted backlog.
- `{{.McpNamespace}}/tickets.find(ticket_stem: "<stem>")` for ticket lookup by stem.
- `{{.McpNamespace}}/tickets.find(mentions_ticket_stem: "<stem>")` for parent/related scans.
- `{{.McpNamespace}}/tickets.status(ticket_stem: "<stem>", include_done: true)` for status checks.
- `{{.McpNamespace}}/specs.find(spec_stem: "<stem>")` for anchor lookup.
- `{{.McpNamespace}}/specs.find(ticket_stem: "<stem>")` for ticket-linked specs.
- `{{.McpNamespace}}/specs.status(spec_stem: "<stem>")` for duplicate-safe anchor location.
- `{{.McpNamespace}}/mental_models.find(query: "<topic>")` for domain discovery.
- `{{.McpNamespace}}/mental_models.status(domain: "<domain>")` for known-domain docs.
- `{{.McpNamespace}}/references.trace(ticket_stem: "<stem>")` for ticket/spec/model links.
- `{{.McpNamespace}}/references.trace(spec_stem: "<stem>")` for spec/ticket/model links.

### Git

`{{.McpNamespace}}/git.status`
`{{.McpNamespace}}/git.diff`
`{{.McpNamespace}}/git.log`
`{{.McpNamespace}}/git.merge_base`
`{{.McpNamespace}}/git.commit`

Use `{{.McpNamespace}}/git.commit` for workflow commits when available. It stages explicit
paths, builds the `## AI Context` message, detects ticket moves plus
`### Result` and `#### Edition` headings, and avoids shell quoting drift.
For ticket status moves, use `{{.McpNamespace}}/tickets.close(stem, status)` to close (done/dropped) or `{{.McpNamespace}}/tickets.move(stem, to)` to transition (idea/todo/ready); both stage atomically with convention guards. Fall back to native `git mv` when MCP tools are unavailable. Commit the staged change with `{{.McpNamespace}}/git.commit`. `ready/` is implementation-ready and `todo/` is accepted backlog.

Prefer:
- `{{.McpNamespace}}/git.status()` for branch, staged state, and changed-file discovery.
- `{{.McpNamespace}}/git.diff(mode: "stat")` before detailed review.
- `{{.McpNamespace}}/git.diff(mode: "full", paths: ["<path>"])` for scoped inspection.
- `{{.McpNamespace}}/git.log(range: "<base>..HEAD", include_body: true)` for commit audit.
- `{{.McpNamespace}}/git.merge_base(base: "main", head: "HEAD")` for branch ranges.
- `{{.McpNamespace}}/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])` for workflow commits.

Use native Git only for operations without an exposed ws primitive, such as
branch creation, tag push, merge execution, or path-filtered file history.

### API documentation

`{{.McpNamespace}}/api.list`

Use `{{.McpNamespace}}/api.list` only to inspect local API documentation cache domains.
For third-party API documentation questions, run scoped host-native exploration
or official documentation lookup directly. Give the worker the exact library,
version or package manager context when known, and require cited evidence plus
any staleness caveats. Do not route API documentation questions through {{.McpNamespace}} MCP
agent-backed tools.

<!-- ws:full-only:start -->
## Planned Or Specialized

Treat active-agent listing and broad message-queue semantics as planned contract
surfaces unless the runtime exposes the exact tool. Basic async cancellation
exists through `ws.mercenary.cancel`; retry the same registered agent with
`ws.mercenary.call` for no-result cancellation recovery. Check runtime before
assuming richer interrupt behavior.
<!-- ws:full-only:end -->

## Usage Pattern

```text
Scoped exploration:
spawn a host-native exploration worker directly with an English scoped task prompt that requests cited evidence, gaps, and follow-up needs.
collect the result when the subagent returns.
for parallel dispatch, spawn multiple concurrent subagents in a single turn; collect all before synthesizing.

<!-- ws:full-only:start -->
Persistent task:
call `ws.mercenary.register(name: "<agent-name>")` for a general-purpose delegate.
for a bundled delegate, render its prompt: `{{.McpNamespace}}/playbook.render(name: "<delegate>")`, then spawn a native subagent or register a mercenary with `system_prompt_text: <rendered>` and `tier: <recommended-tier>`.
call `ws.mercenary.call(name: "<agent-name>", prompt: <block below>)` for the mercenary path.
wait for readiness, read final output with `result(timeout_seconds: 600)`, inspect status, or tail with `lines: 3`.
erase the task-scoped agent when cleanup matters.
<!-- ws:full-only:end -->

Review artifacts:
call `{{.McpNamespace}}/path.generate(kind: "review", stems: ["<stem>"])`.
tell reviewers to write full findings to those paths.
relay file paths, not full findings, to the implementer.

API docs:
call `{{.McpNamespace}}/api.list()` when choosing among cached domains matters.
for external API lookup, spawn a scoped host-native exploration worker or use
official documentation lookup directly; include exact package, version, and
question context, and require cited evidence.

References:
call `{{.McpNamespace}}/references.trace(ticket_stem: "<ticket-stem>")` for ticket/spec/model links.
call `{{.McpNamespace}}/references.trace(spec_stem: "<spec-stem>")` for spec/ticket/model links.
call domain discovery tools first when only paths or status metadata are needed.

Commit:
call `{{.McpNamespace}}/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])`.
```

## Doctrine

Workflow notation optimizes for **limited execution attention** during cross-host
work. References must survive skill execution and map to each host's tool
display. When ambiguous, preserve execution attention.
