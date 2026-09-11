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
> `{{.McpNamespace}}/playbook.read(name: "lead-workflow-manual")` and execute inline.
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
Write prompts sent to delegated subagents in English.

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
re-minting — re-minting when you meant to restore is the footgun above. Holding
several keys on one root is deliberate and supported when they carry distinct
workflow contexts: to run parallel development tracks in one session, mint one
key per track and keep each verbatim, since agenda, todo, and session-tree state
are per-key, not shared across the root. Session keys are lightweight and stale
ones prune automatically, so create one per track without hesitation.

### User preferences

<!-- ws:override:UserPreferenceSection desc="user standing preferences for communication, terminology, and workflow behavior" -->
No standing user preferences are configured for this project. Use conventional
terminology and default communication style unless project or session
configuration overrides this slot via `config.tune`.
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

### Delegate prompts

Bundled delegate prompts are rendered, not named by stem: call
`{{.McpNamespace}}/playbook.render(name: "<delegate>", session_key: <your key>)`
and hand the returned path to a native subagent, spawned at the tier the render
recommends. Pass the session_key: with a lead key the render splices the
delegate's own session credential into the prompt file, so the subagent starts
already authenticated; without one it does not, and the delegate arrives
unkeyed. Hand over the path, not the file's contents.
`reference-discovery` is such a delegate playbook, not a workflow skill.

### Artifact paths

Use `{{.McpNamespace}}/path.generate` for generated workflow artifact paths. Capture
returned paths. Relay paths, not large findings, between lead, implementer, and
reviewers.

### Runtime metadata

Use `{{.McpNamespace}}/runtime.read` for runtime compatibility checks and feature detection.

### Reference discovery

Use the {{.McpNamespace}}-owned ticket discovery tools for path/status lookup
before shell search. Use native file reads after a discovery tool returns the
path to inspect or edit.

Prefer:
- `{{.McpNamespace}}/tickets.query(status: "ready")` for implementation-ready discovery; use `status: "todo"` for accepted backlog.
- `{{.McpNamespace}}/tickets.query(ticket_stem: "<stem>")` for ticket lookup by stem.
- `{{.McpNamespace}}/tickets.query(mentions_ticket_stem: "<stem>")` for parent/related scans.
- `{{.McpNamespace}}/tickets.query(ticket_stem: "<stem>", include_done: true)` for status checks.

### Notes / durable memory

`{{.McpNamespace}}/note.write` records durable cross-session context;
`{{.McpNamespace}}/note.query` reads it back, and the ambient `# Notes` block
surfaces active notes at session start. Write one when a future session would
otherwise re-derive a fact that has no better home — a non-obvious environment
quirk, a standing gotcha, or the live consequence of a past decision. Prefer the
narrowest scope that still reaches the sessions that need it; a note shared
wider than its readers is noise everywhere else.

Do not note what already has a home: a value derivable from the tree at read
time, a change's rationale (`## AI Context` on the commit), or a ticket's status
or plan (the ticket). A note is durable context, not a running log.

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

`impl/*` and `goal/*` are workflow-owned branches carrying plan and review
history: merge them with `git merge --no-ff` by default, and squash instead
only when the branch is one logical change with noisy or dependent commits.

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
recoverable and worth doing, but implementation has not started. `ready/` is
the implementation-ready status: the ticket has passed its completeness sage
review, and the dependencies blocking its earliest unfinished phase are
themselves in `ready/` or `.done/` — so the `ready/` set is a closed work front
that drains in dependency order, and a dependent reaches `ready/` only alongside
or after its prerequisites (recorded as `related:`/`parent:` edges). `.done/` and
`.dropped/` are terminal: completed work and abandoned scope, respectively.

### Type prefix: feat / bug / refactor / chore

`feat`, `bug`, `refactor`, and `chore` are **mechanically identical** in the
workflow — same phase model, same sage-review stage requirements (see
`judge: ticket-category`). The prefix is a categorization
label for human and agent scanning, not a behavioral switch. Pick by
plain-word fit: `feat` introduces a new capability or behavior; `bug`
corrects behavior that deviates from intent; `refactor` restructures
internals with no intended external behavior change; `chore` covers
maintenance, tooling, or housekeeping outside product behavior. Do not read a
different verification depth, phase count, or workflow routing into the
choice.

### Sage review

Actionable tickets (`bug`/`feat`/`refactor`/`chore`) are authored and edited in
`todo/` without fact population or Sage review. Promotion to `ready/` is their
settlement boundary: populate facts first, then run design and completeness
review against that populated body. Existing completed or skipped stages keep
their posture and freshness behavior.

An epic is a living board that is never an execution target, so it never enters
`ready/` (the move is barred in code); a research ticket is likewise barred and
ungated. Epic design review is design-only (completeness never applies) and
lead-judgment-invoked: run it — populating checkable facts first — when the
epic's cross-child design has drifted materially, not as a status boundary.
Ordinary epic edits do not automatically spawn reviewers, and a child relying on
a revised cross-child decision needs that re-review first. `ready/` remains the
actionable implementation queue.

Posture (per-stage, stored in ticket frontmatter) resolves the gate:
`pending` falls back to the project's configured default; `skipped` means
the stage will not run; `blocked` means a prior review found unresolved
issues and the gate stops until they are addressed; `recommended` asks
before running; `required` always runs; `completed` means the stage already
ran and passed.

### Phases

A phase is one complete, reviewable, verifiable behavior slice — sized so a
fresh session with no other context can finish it, review it, verify it, and
hand it off cleanly. Setup, API, UI, tests, legacy skeleton artifacts, and
investigation are phase ingredients folded into that slice unless one of them
is itself the reviewable deliverable. A phase write-up states its completed
behavior, deferred scope, and verification boundary. Phases accumulate
`### Result` (and later `#### Edition`) entries as work lands, giving the
ticket a durable record of what actually happened per phase versus what was
planned. Epics do not carry implementation phases; phase-level detail belongs in
their child tickets.

### Epics and execution scope

An `epic` is hierarchical: child tickets collectively deliver one parent
outcome, and cross-child invariant decisions live in the epic body. Use
`epic` for single-outcome decomposition. The goal loop over the scoped
`ready/` queue handles mixed-parent execution; frontmatter `related:` records
non-hierarchical relationships between tickets.

## Planned Or Specialized

Check `{{.McpNamespace}}/runtime.read` before assuming richer interrupt or
active-agent/message-queue behavior than the runtime exposes. Interrupting a
dispatched subagent, or retrying one, is the harness's affordance rather than a
workflow tool.
