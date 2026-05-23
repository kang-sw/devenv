---
name: lead-workflow-manual
description: Mandatory reference for wsflow workflow orchestration. Use when writing or executing wsflow skills, MCP notation, subagent guidance, or orchestration boundaries matter.
---

# Workflow Manual

> **Session invariant:** Keep this reference active while writing or executing
> wsflow workflow skills. After compaction, re-invoke
> `wsflow:lead-workflow-manual` when primitive names or orchestration
> boundaries matter.

## On: invoke

Reading this file is the invocation; it loads the primitive reference.

---

# wsflow Workflow Primitives

Host-neutral notation reference for wsflow plugin skill text.

Use `wsflow/<tool-name>` for MCP server `wsflow`, tool `<tool-name>`.
Use `wsflow:` only for plugin skills such as `wsflow:lead-write-ticket`.
Write MCP calls as `wsflow/tool.name(arg: value)`.
Show optional arguments only when the skill needs a non-default value.
Omit `root` when the current repository root is intended.
Use `prompt: <block below>` or `question: <block below>` for large text payloads.

When writing wsflow skill text, name only primitives that exist in the wsflow
runtime. If a workflow needs extra context or bounded execution help, use direct
local exploration or a scoped subagent; describe the task scope, permissions,
and expected output.

## How To Document

- Write compressed professional prose: short sentences, exact verbs, no filler.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- Use `Not:` / `Use:` examples only for recurring mistakes.
- Keep procedure text command-shaped; move rationale to one short Doctrine paragraph.
- Use full sentences when compression could blur order, ownership, or safety.

## Available

### Runtime

`wsflow/runtime.info`
`wsflow/runtime.debug_events`
`wsflow/setup`
`wsflow/config.show`

Use `wsflow/runtime.info` for runtime compatibility checks and feature
detection. Use `wsflow/setup(root: "<path>")` to set the current server process
root when plugin-managed startup did not infer the intended repository.

### Project Context

`wsflow/project_tree`
`wsflow/infra.read`
`wsflow/convention.read`

Use `wsflow/project_tree()` for the project map. Use `wsflow/infra.read` for
bundled implementation discipline. Use `wsflow/convention.read` before editing
tickets, specs, or mental models.

### Reference Discovery

`wsflow/tickets.list`
`wsflow/tickets.find`
`wsflow/tickets.status`
`wsflow/specs.list`
`wsflow/specs.find`
`wsflow/specs.status`
`wsflow/mental_models.list`
`wsflow/mental_models.find`
`wsflow/mental_models.status`
`wsflow/references.trace`

Use these for ticket, spec, and mental-model path/status/reference lookup before
shell search. Use native file reads after a discovery tool returns the path to
inspect or edit.

Prefer:
- `wsflow/tickets.list(status: "ready")` for implementation queue discovery.
- `wsflow/tickets.find(ticket_stem: "<stem>")` for ticket lookup by stem.
- `wsflow/tickets.find(mentions_ticket_stem: "<stem>")` for parent/related scans.
- `wsflow/tickets.status(ticket_stem: "<stem>", include_done: true)` for status checks.
- `wsflow/specs.find(spec_stem: "<stem>")` for anchor lookup.
- `wsflow/specs.find(ticket_stem: "<stem>")` for ticket-linked specs.
- `wsflow/specs.status(spec_stem: "<stem>")` for duplicate-safe anchor location.
- `wsflow/mental_models.find(query: "<topic>")` for domain discovery.
- `wsflow/mental_models.status(domain: "<domain>")` for known-domain docs.
- `wsflow/references.trace(ticket_stem: "<stem>")` for ticket/spec/model links.
- `wsflow/references.trace(spec_stem: "<stem>")` for spec/ticket/model links.

### Specs

`wsflow/spec_stem.generate`
`wsflow/spec_index.verify`

Call `wsflow/spec_stem.generate(slug: "<descriptive-slug>")` before inserting
new spec anchors. Call `wsflow/spec_index.verify()` after every spec write.

### Git

`wsflow/git.status`
`wsflow/git.diff`
`wsflow/git.log`
`wsflow/git.merge_base`
`wsflow/git.commit`

Use `wsflow/git.commit` for workflow commits when available. It stages explicit
paths, builds the `## AI Context` message, detects ticket moves and `### Result`
headings, and avoids shell quoting drift.

Prefer:
- `wsflow/git.status()` for branch, staged state, and changed-file discovery.
- `wsflow/git.diff(mode: "stat")` before detailed review.
- `wsflow/git.diff(mode: "full", paths: ["<path>"])` for scoped inspection.
- `wsflow/git.log(range: "<base>..HEAD", include_body: true)` for commit audit.
- `wsflow/git.merge_base(base: "main", head: "HEAD")` for branch ranges.
- `wsflow/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])` for workflow commits.

Use native Git only for operations without an exposed wsflow primitive, such as
branch creation, tag push, merge execution, or path-filtered file history.

### API Documentation Cache

`wsflow/api.list`

Use `wsflow/api.list()` to inspect cached documentation domains. When a
workflow needs API facts, read local cached docs or direct project references.

### Subagents

Use subagents when a task benefits from scoped exploration, implementation,
verification, audit, or review.

Subagent prompts must be self-contained:
- Write subagent prompts in English.
- State the exact question and expected output.
- State scope, permissions, and any writable paths or modules.
- Tell exploration, verification, audit, and review workers to stay read-only.
- For implementation workers, require a changed-file list and verification notes.
- Point the worker at wsflow read tools such as `wsflow/project_tree`,
  `wsflow/convention.read`, `wsflow/infra.read`, `wsflow/specs.*`,
  `wsflow/tickets.*`, `wsflow/mental_models.*`, and `wsflow/git.*`.
- Ask for concise findings with file paths or stems, not broad narratives.

The lead owns integration, verification, final judgment, and commits.

## Usage Pattern

```text
References:
call `wsflow/references.trace(ticket_stem: "<ticket-stem>")` for ticket/spec/model links.
call `wsflow/references.trace(spec_stem: "<spec-stem>")` for spec/ticket/model links.
call domain discovery tools first when only paths or status metadata are needed.

Subagent exploration:
Use a scoped subagent when useful.
Prompt it with the exact question, scope, permissions, wsflow read tools, and concise output format.
If no subagent is useful, use direct local search and file reads.

Commit:
call `wsflow/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])`.
```

## Doctrine

Workflow notation optimizes for **limited execution attention** during
cross-host work. References must survive skill execution and map to each host's
tool display. Skills preserve attention by keeping integration and final
judgment explicit while routing scoped work to direct exploration or subagents.
When ambiguous, preserve execution attention.
