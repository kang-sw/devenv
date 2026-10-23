---
kind: print
includes:
  - native-spawn-binding
variables:
  - WorkflowLang
  - ExploreAgent
  - SpawnIdiom
---
# Workflow Manual

> **Session invariant:** Preserve the `session_key` verbatim across compaction.
> Before any other workflow action after compaction or continuation, invoke
> `{{.SkillNamespace}}:lead-revive` once with the preserved key to restore
> session state and reload this manual.

## On: invoke

Load this manual as the current workflow reference, then continue the invoking
skill without a standalone acknowledgement.

---

## Available workflow primitives

Treat MCP tool schemas as the authority for arguments and response fields.

### Session setup

<!-- ws:fresh-only:start -->
You have no session key yet. To mint your lead key, call the MCP primitive
`ferrule(root: "<absolute-working-directory>")` for this Git worktree and retain
the returned value.
<!-- ws:fresh-only:end -->
A workflow track is one independently managed work stream. Its session key
belongs to one Git worktree; reuse it for root-aware calls and after compaction.
`ferrule` creates a new empty session, so call it again only to start an
independent track, including another track in the same worktree.

### User preferences

<!-- ws:override:UserPreferenceSection desc="user standing preferences for communication, terminology, and workflow behavior" -->
Use the project's terminology and communication style.
{{.WorkflowLang}}
<!-- ws:/override:UserPreferenceSection -->

### Asking the user

Every item you put to the user, in discussion or in a ticket's Open Decision
Queue, is one of three classes, so the user answers only the choices that
depend on their preferences and can review the rest at the depth they choose.

- **Announced default**: the item has one answer, and you can cite the reason
  its alternatives lose. The citation is one of: a language or platform
  constraint; a prior commit or ticket decision; an established project
  convention; a direct consequence of an already-confirmed decision.
- **Technical decision**: the item has no such citation, and its answer does
  not hinge on the user's preferences or priorities: you pick it on technical
  reasoning (simpler, fewer moving parts, a clearer failure mode) and state
  that reasoning. Your own uncertainty about a technical call keeps it a
  technical decision: state the reasoning and the uncertainty, since as a
  policy question it would hand the user a call they cannot judge.
- **Policy question**: the answer depends on the user's preferences,
  priorities, or product direction, such as the audience or scale a default
  assumes, scope in or out, cost or risk tolerance, or a trade-off in
  downstream-observable behavior.
- An item you raise that reverses a prior decision is a policy question, even
  when you can cite a reason for the reversal; a reversal the user states is
  their answer, not a new question.
- The classes never turn an ask a skill or the project reserves for the user
  into a default or a technical call: the next-step handoff of
  `{{.SkillNamespace}}:lead-review`, the publish confirmation of
  `{{.SkillNamespace}}:lead-ship`, and the project's Approval Protocol "Ask
  first" and "Always ask" items stay asks.

Every item carries an ID unique within the conversation, so the user can cite
any earlier item by number:

- Write it `(nn)`, zero-padded to two digits (`(03)`; three digits past 99).
  A new item takes one past the highest ID already used in the conversation,
  counting the IDs and the last-ID line of every ticket queue you have read,
  `(01)` when none; an item keeps its ID when it moves from discussion into a
  ticket's queue.
- Discussion keeps no counter. After compaction, continue from the highest ID
  the summary shows; a collision with an ID the summary dropped is accepted,
  since nothing in discussion persists.
- A ticket queue left open from an earlier conversation keeps its IDs. Only
  when one collides with an ID already used in this conversation are the
  items still in that ticket's queue renumbered to fresh IDs, in every ticket
  that carries them, with the last-ID line updated; state the old-to-new
  mapping once, in one line: `(04) -> (15), (05) -> (16)`.

### Workflow tuning

For lead-owned tuning of delegation posture or other workflow knobs, use the `{{.SkillNamespace}}:lead-tune` skill.

### Scoped Exploration (native Explore)

Use {{.ExploreAgent}} when bounded evidence gathering materially benefits from
an independent read. Choose {{.SmallTierModel}} by default and
{{.MediumTierModel}} for ambiguous evidence, trade-off analysis, or
cross-source synthesis; pass the chosen model explicitly. Give it an English
prompt with the scoped question and require cited evidence, gaps, and follow-up
needs. Parallelize independent questions when that reduces elapsed time, and
synthesize once the decision has the evidence it needs.

### Delegate prompts

Render a bundled delegate prompt with `{{.McpNamespace}}/playbook.render` and
give the returned path to the native subagent. Pass the lead `session_key` so
the rendered prompt carries the delegate's session context. Use the dispatch
tier and bindings reported by the render when spawning.
When delegating into another worktree, select that worktree at render time so
the prompt artifact and session context follow the delegate.

### Artifact paths

Use `{{.McpNamespace}}/path.generate` for large generated workflow artifacts.
Relay the returned path with a brief conclusion and requested next action.

### Runtime metadata

Call `{{.McpNamespace}}/runtime.read` only when a step depends on optional
runtime or harness capabilities.

### Reference discovery

When a ticket path, stem, or status is unknown, use
`{{.McpNamespace}}/tickets.query` before native search. Read a known path
directly. Include terminal inventory only when the question requires history.

### Notes / durable memory

A note persists beyond this session until erased; the agenda is this
session's own state, restored with its key and cleared when its work ends. A
pending question, a next step, or a finding the current work will consume goes
in the agenda (`{{.McpNamespace}}/agenda.set`), not a note.

Write a durable note when a future session would otherwise re-derive a
non-obvious fact with no better home. Use the narrowest layer that reaches its
readers. Repository facts, commit rationale, and ticket state belong in the
source tree, `## AI Context`, and the ticket respectively. Query notes when the
ambient block omits needed context.

### Git

Use a {{.McpNamespace}} Git primitive when one exists for the operation. Use
native Git for unsupported operations or read-only diagnosis.

Branch integration is lead-owned through `{{.McpNamespace}}/git.merge`,
including impl-to-parent, goal-to-parent, and epic-to-review-track promotion.
Apply the goal approval gate and the project's release gate at their respective
boundaries.

A root whose `HEAD` is on an `impl/*` branch is a worker's until the lead
restores it (the occupied-root banner names the parent branch): a commit or
ticket move there lands on the worker's branch. Housekeeping meanwhile goes
through `{{.McpNamespace}}/worktree.acquire(base: <parent branch>,
profile: "housekeeping")`, which checks the parent branch out in a pooled
ticket-housekeeping worktree and returns the key to commit with there; release
it with `{{.McpNamespace}}/worktree.release(key: <that key>)` before anything
checks the parent branch out elsewhere, because Git holds one checkout per
branch: a `{{.McpNamespace}}/git.merge` into a branch still held that way is
refused, not stolen.

Every `{{.McpNamespace}}/worktree.acquire` names a profile from the resolved
list below. For a worker, pass `full` unless a listed project profile
describes that work's checkout, because the builtin `housekeeping`
materializes only `ai-docs` and a worker there cannot build; a pooled worktree
is reused only by an acquire naming the same profile.

Relay every `nudge` entry an acquire result carries to the user, because it is
the only report of pooled worktrees left idle or holding large preserved
artifacts; removing one with `{{.McpNamespace}}/worktree.remove(path: ...)` is
the user's call.

### API documentation

Use `{{.McpNamespace}}/api.list` to inspect local API documentation cache domains.
For external API questions, consult official documentation for the exact
library and version; use scoped exploration when cross-source evidence is
needed. Cite the evidence.
