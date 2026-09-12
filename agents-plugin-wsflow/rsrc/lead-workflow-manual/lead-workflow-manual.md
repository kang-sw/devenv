---
kind: print
includes:
  - native-spawn-binding
variables:
  - WorkflowLang
  - ExploreAgent
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

### API documentation

Use `{{.McpNamespace}}/api.list` to inspect local API documentation cache domains.
For external API questions, consult official documentation for the exact
library and version; use scoped exploration when cross-source evidence is
needed. Cite the evidence.
