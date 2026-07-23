---
kind: print
includes:
  - native-spawn-binding
variables:
  - WorkflowLang
  - ExploreAgent
---
# Workflow Manual

> **Session invariant:** Must reload after session compaction or continuation. Call
> `{{.McpNamespace}}/playbook.print(name: "lead-workflow-manual")` and execute inline.
> When in doubt, reload — a duplicate load is safe.

## On: invoke

After reading this file, treat the listed primitives and usage patterns as the
current workflow reference. No separate output is required.

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
Write prompts sent to `mercenary.call` in English.
<!-- ws:full-only:end -->

When writing shared skill text, name only primitives that exist in the {{.McpNamespace}} runtime.
If a workflow needs a surface that is still planned, state the required MCP
contract instead of naming a Claude helper command or another host-specific
fallback.
`user` is relative: in a lead session it is the human; in a delegated worker
session it is the lead that spawned the worker. In shared skill text, name `the
lead` for worker-facing escalation targets and reserve `user` for the human
decision at the top.

Centralize primitive usage here. Other skills should name the primitive and
include only local arguments that affect the current step.

## Available

### Session setup

<!-- ws:fresh-only:start -->
You have no session key yet: call `ferrule(root: "<absolute-working-directory>")`
for this root to mint your lead key. The name is deliberately non-descriptive
and is taught only here: it is the lead session-bootstrap call, so subagents
that share this MCP connection have no semantic cue to invoke it. Pass the
repository's absolute filesystem path as `root`; the MCP server cannot infer
the agent's current directory from placeholders or relative paths.
<!-- ws:fresh-only:end -->
Each key binds to one canonical repository root — the git top-level of the path
you pass — and a git worktree resolves to its own top-level, so it counts as a
distinct root. Call `ferrule` once per working root, then reuse the returned
`session_key` for every subsequent root-aware {{.McpNamespace}} tool call that
targets that root, including across context compaction. Calling `ferrule`
again for a root you already hold a key for does not reuse or restore the
existing identity: it mints a brand-new session key with empty state,
stranding any agenda, todo, or session-tree state bound to the earlier key. If
you are recovering after compaction, restore the preserved key instead of
re-minting; only call `ferrule` again when you have genuinely never held a key
for this root.

### User preferences

<!-- ws:override:UserPreferenceSection desc="user standing preferences for communication, terminology, and workflow behavior" -->
No standing user preferences are configured for this project. Use conventional
terminology and default communication style unless project or session
configuration overrides this slot via `config.prompt.set`.
{{.WorkflowLang}}
<!-- ws:/override:UserPreferenceSection -->

### Workflow tuning

For lead-owned tuning of delegation posture or other workflow knobs, use the `{{.SkillNamespace}}:lead-tune` skill.

### Scoped Exploration (native Explore)

For scoped fact-finding, surveys, and one-turn answers, dispatch
{{.ExploreAgent}} as {{.SmallTierModel}} by default; escalate to
{{.MediumTierModel}} only when the exploration requires judgment. Specify the
model explicitly on the spawn call — do not rely on the harness default. Use an
English prompt and require cited evidence, gaps, and follow-up needs. For
parallel dispatch, spawn multiple in one turn; collect all before
synthesizing.

<!-- ws:full-only:start -->
### Persistent agents

Register a stable task name with a self-contained system prompt. Registration
takes `name`, optional `backend`, `system_prompt_text`, and `tier`; the removed
`prompts`/`prompt_refs`/`model` fields are gone. Omit `system_prompt_text` for a
general-purpose named agent; registration applies delegate orientation and the
default tier mapping. Call the agent for each continuity turn.
Bundled delegate prompts are not registered by stem — render them. Obtain a
delegate's self-contained prompt with `{{.McpNamespace}}/playbook.render(name: "<delegate>")`
(a lead key splices a child-key credential block). Hand the rendered prompt to a native
subagent (default), or pass it as `system_prompt_text` with `tier:
<recommended-tier>` to a mercenary `mercenary.register` + `mercenary.call`, then
collect through `mercenary.result`. `reference-discovery` is such a delegate
playbook, not a workflow skill.
`mercenary.call` starts async and returns promptly. Use
`wait(timeout_seconds: 600)` for readiness metadata, `result(timeout_seconds:
600)` or a longer bound for final output, `status` before waiting,
`tail(lines: 3)` for small diagnostics, `print` only as a compatibility output
alias, `cancel` to stop active work, retry `call` on the same registered agent
with a recovery prompt when cancellation followed a no-result timeout, and
`erase` when task-scoped state should be removed.
<!-- ws:full-only:end -->

### Artifact paths

Use `{{.McpNamespace}}/path.generate` for generated workflow artifact paths. Capture
returned paths. Relay paths, not large findings, between lead, implementer, and
reviewers.

### Runtime metadata

Use `{{.McpNamespace}}/runtime.info` for runtime compatibility checks and feature detection.

### Reference discovery

Use the {{.McpNamespace}}-owned ticket, spec, and mental-model discovery tools for
path/status/reference lookup before shell search. Use native file reads after a
discovery tool returns the path to inspect or edit.

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

Use `{{.McpNamespace}}/git.commit` for workflow commits when available. It stages explicit
paths, builds the `## AI Context` message, detects ticket moves plus
`### Result` and `#### Edition` headings, and avoids shell quoting drift.
For ticket status moves, use `{{.McpNamespace}}/tickets.close(stem: "<stem>", status: "done")` to close or `{{.McpNamespace}}/tickets.move(stem: "<stem>", to: "ready")` to transition; both stage atomically with convention guards. Fall back to native `git mv` when MCP tools are unavailable. Commit the staged change with `{{.McpNamespace}}/git.commit`. `ready/` is implementation-ready and `todo/` is accepted backlog.

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

Use `{{.McpNamespace}}/api.list` only to inspect local API documentation cache domains.
For external API questions, run scoped host-native exploration or official docs
lookup directly with exact library/version context and cited evidence; do not
route them through {{.McpNamespace}} MCP tools.

## Ticket System Concepts

Meaning and rationale behind the ticket system's structural concepts. Exact
syntax and hard invariants live in `ticket-conventions.md` (via
`{{.McpNamespace}}/convention.read(name: "ticket-conventions")`) and are
enforced where that document notes; this section explains *why* the shape
exists so those rules read as intentional rather than arbitrary.

### Status directories

A ticket's status is its directory, not a frontmatter field. `idea/` is a
rough capture surface for underspecified or exploratory topics — nothing yet
needs to be actionable. `todo/` is accepted backlog: the intent is
recoverable and worth doing, but implementation has not started and a spec
contract may not exist yet. `ready/` is the implementation-ready status: the
ticket's caller-visible behavior is addressed by a spec (existing or newly
declared), so a fresh session can proceed without inventing product
decisions. `.done/` and `.dropped/` are terminal: completed work and
abandoned scope, respectively.

### Type prefix: feat / bug / refactor / chore

`feat`, `bug`, `refactor`, and `chore` are **mechanically identical** in the
workflow — same phase model, same spec-address gate, same sage-review stage
requirements (see `judge: ticket-category`). The prefix is a categorization
label for human and agent scanning, not a behavioral switch. Pick by
plain-word fit: `feat` introduces a new capability or behavior; `bug`
corrects behavior that deviates from intent; `refactor` restructures
internals with no intended external behavior change; `chore` covers
maintenance, tooling, or housekeeping outside product behavior. Do not read a
different verification depth, phase count, or workflow routing into the
choice.

### Sage review

Sage review is an independent-reviewer gate a ticket passes through before a
stage boundary is treated as settled: a **design** review gates the `todo/`
stage (does the approach still make sense), and a **completeness** review
gates the `ready/` stage (is the ticket's contract and verification story
actually complete). A ticket that reaches `ready/` without a prior `todo/`
design pass runs both stages together. Stage applicability follows category:
`research`/`workset` need neither stage (nothing decompositional to review),
`epic` needs design only (epics never reach implementation, so completeness
never applies), and actionable categories (`bug`/`feat`/`refactor`/`chore`)
need both.

Posture (per-stage, stored in ticket frontmatter) resolves the gate:
`pending` falls back to the project's configured default; `skipped` means
the stage will not run; `blocked` means a prior review found unresolved
issues and the gate stops until they are addressed; `recommended` asks
before running; `required` always runs; `completed` means the stage already
ran and passed.

### Spec addressing

Entering `ready/` (for non-`epic`, non-`research`, non-`workset` tickets)
requires each phase's caller-visible behavior to be addressed by a spec: an
existing confirmed `spec:` stem, a `spec-remove:` entry, or a `## Spec
Impact` section describing what a spec will need to cover. The purpose is to
stop implementation from starting against an undocumented or unstable
contract — either point at an already-addressed spec area, or explicitly
declare the ticket only needs post-implementation closeout documentation.
`idea/` and `todo/` tickets may hold `spec:` links as optional recovery
hints; the check only applies at the `ready/` boundary.

### Phases

A phase is one complete, reviewable, verifiable behavior slice — sized so a
fresh session with no other context can finish it, review it, verify it, and
hand it off cleanly. Setup, API, UI, tests, legacy skeleton artifacts, and
investigation are phase ingredients folded into that slice unless one of them
is itself the reviewable deliverable. A phase write-up states its completed
behavior, deferred scope, and verification boundary. Phases accumulate
`### Result` (and later `#### Edition`) entries as work lands, giving the
ticket a durable record of what actually happened per phase versus what was
planned. Epics and worksets do not carry implementation phases; phase-level
detail belongs in the child or included tickets they reference.

### Epic vs. workset

Both are board artifacts, not implementation targets, and both skip the
ready spec-address gate — but they organize differently. An `epic` is
hierarchical: child tickets collectively deliver one parent outcome, and
cross-child invariant decisions live in the epic body. A `workset` is
non-hierarchical: it groups independent or cross-cutting tickets for
coordination, sequencing, or focus without owning decomposition — included
tickets are listed, never made children. Choose `epic` when the request is a
parent-outcome breakdown; choose `workset` when it is a coordination/focus
grouping with no decomposition ownership.

<!-- ws:full-only:start -->
## Planned Or Specialized

Check `{{.McpNamespace}}/runtime.info` before assuming richer interrupt or
active-agent/message-queue behavior than the runtime exposes; basic async
cancellation exists through `mercenary.cancel`, with retry via `mercenary.call`.
<!-- ws:full-only:end -->
