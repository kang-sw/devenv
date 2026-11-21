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

> **Session invariant:** keep `session_key` verbatim across compaction. After
> compaction or continuation, before any other workflow action, invoke
> `{{.SkillNamespace}}:lead-revive` once with that key.

## On: invoke

Load this manual as the workflow reference, then continue the invoking skill
without a standalone acknowledgement.

---

## Available workflow primitives

- Arguments and response fields: the MCP tool schema is the authority.
- A tool result the host truncated or saved to a file: read all of it before
  acting.

### Session setup

- One key = one workflow track in one Git worktree. Reuse it for every
  root-aware call and after compaction.

### User preferences

<!-- ws:override:UserPreferenceSection desc="user standing preferences for communication, terminology, and workflow behavior" -->
Use the project's terminology and communication style.
<!-- ws:/override:UserPreferenceSection -->
{{.WorkflowLang}}

### Asking the user

Each item you put to the user, in discussion or in a ticket's Open Decision
Queue, has one class, so the user answers only what depends on them:

- Announced default: one answer, and you can cite why the alternatives lose:
  a language or platform constraint, a prior commit or ticket decision, a
  project convention, or a consequence of a confirmed decision.
- Technical decision: no such citation, and the answer does not hinge on the
  user's preferences. Pick it on technical grounds (simpler, fewer moving
  parts, clearer failure) and state the reasoning. Uncertain → still
  technical; state the uncertainty.
- Policy question: the answer depends on the user's preferences, priorities,
  or product direction: audience or scale, scope in or out, cost or risk
  tolerance, a downstream-visible trade-off.
- Check (a gate, never in an Open Decision Queue): ask the user to look at,
  verify, or confirm a finished artifact or an already-settled action ("OK
  this draft and I'll proceed"). One verifiable thing per check.

Rules:
- "Should we do X?" or "which one?", yes/no included → a policy question with
  options, never a check.
- You raise an item that reverses a prior decision → policy question, even
  with a reason. A reversal the user states is their answer.
- Asks a skill or the project reserves for the user stay asks: the
  `{{.SkillNamespace}}:lead-review` next-step handoff, the
  `{{.SkillNamespace}}:lead-ship` publish confirmation, and the project's
  Approval Protocol "Ask first" and "Always ask" items.

IDs, unique within the conversation so the user can cite any item:
- Format `(nn)`, zero-padded to two digits (three past 99).
- New item → one past the highest ID used in the conversation, counting
  every ticket queue you have read (its IDs and its last-ID line); `(01)`
  when none. An item keeps its ID when it moves into a ticket queue.
- Discussion keeps no counter. After compaction, continue from the highest ID
  the summary shows; a collision with an ID the summary dropped is accepted.
- A ticket queue from an earlier conversation keeps its IDs. One collides
  with an ID used in this conversation → renumber that ticket's still-open
  items to fresh IDs in every ticket carrying them, update the last-ID line,
  and state the mapping once: `(04) -> (15), (05) -> (16)`.

### Spawning helpers

- Bounded evidence gathering that benefits from an independent read →
  {{.ExploreAgent}} at the tier the question needs (`### Choosing a tier`).
  English prompt with the scoped question; require cited evidence, gaps, and
  follow-ups. Run independent questions in parallel; synthesize once the
  decision has enough evidence.
- Bundled delegate prompt → `{{.McpNamespace}}/playbook.render` with the lead
  `session_key`; give the subagent the returned path and spawn it with the
  tier and bindings the render returns. Delegate works in another worktree →
  select that worktree at render time.

### Choosing a tier

Pass the tier to `{{.McpNamespace}}/config.resolve_agent(tier)` rather than
naming a model.

- A rendered playbook → the tier its render recommends.
- small: well-specified work with no design calls — mechanical edits,
  running and reading commands, lookups, straightforward investigation.
- medium: ordinary engineering with a clear target and approach — a scoped
  change or investigation, across modules if needed.
- large: demanding work — ambiguous diagnosis, design-sensitive changes or
  drafting, broad cross-cutting changes.
- xlarge: research-grade work — an open problem with no known approach, or
  novel design where a wrong call reshapes the system.
- Unsure between small, medium, and large → the higher one; xlarge only when
  the task plainly meets its bar.

### Lookups and artifacts

- Large generated workflow artifact → `{{.McpNamespace}}/path.generate`;
  relay the path with a brief conclusion and the next action.
- A step depends on an optional runtime or harness capability →
  `{{.McpNamespace}}/runtime.read`; otherwise do not call it.
- Ticket path, stem, or status unknown → `{{.McpNamespace}}/tickets.query`
  before native search; known path → read it directly. Include terminal
  inventory only when the question needs history.
- Local API documentation → `{{.McpNamespace}}/api.list`. External API
  question → official documentation for the exact library and version
  (scoped exploration when sources must be combined); cite it.
- Workflow knobs or delegation posture → `{{.SkillNamespace}}:lead-tune`.

### Notes and agenda

- Agenda (`{{.McpNamespace}}/agenda.set`): this session's state: a pending
  question, a next step, a finding the current work will consume. Restored
  with the key; cleared when the work ends.
- Note: a non-obvious fact a future session would otherwise re-derive, with
  no better home. Persists until erased.
- Note layer → the narrowest that reaches its readers: `worktree` by
  default; `clone` only if a session in another worktree would fail without
  it; `machine` only if a session in another project would.
- Spawned worker → writes no notes; puts the fact in its final report for
  the lead to write.
- Not notes: repository facts (the source tree), commit rationale
  (`## AI Context`), ticket state (the ticket).
- The ambient notes block lacks what you need → query notes.

### Git

- An operation has a {{.McpNamespace}} Git primitive → use it. Unsupported
  operation or read-only diagnosis → native Git.
- Branch integration (impl → parent, epic → review track) is lead-owned,
  through `{{.McpNamespace}}/git.merge`. Apply the project's release gate at
  its boundary.
- Root `HEAD` is on an `impl/*` branch → it is the worker's until the lead
  restores it (the occupied-root banner names the parent branch); a commit or
  ticket move there lands on the worker's branch. Housekeeping meanwhile:
  1. `{{.McpNamespace}}/worktree.acquire(base: <parent branch>, profile:
     "housekeeping")` → a pooled ai-docs-only checkout of the parent branch,
     and the key to commit with there.
  2. Release it with `{{.McpNamespace}}/worktree.release(key: <that key>)`
     before anything checks the parent branch out elsewhere;
     `{{.McpNamespace}}/git.merge` into a branch still held there is refused.
- Every `{{.McpNamespace}}/worktree.acquire` names a profile from the
  resolved list below. Worker → `full`, unless a listed project profile
  describes the work: the builtin `housekeeping` materializes only
  `ai-docs`, so a worker there cannot build. A pooled worktree is reused only
  by an acquire naming the same profile.
- An acquire result carries `nudge` entries → relay each to the user.
  Removing one (`{{.McpNamespace}}/worktree.remove(path)`) is the
  user's call.

### Worktree-based work mode

Only when the user asks (the housekeeping worktree above excepted):
- Per unit → resolve the target branch (for a ticket, with
  `{{.McpNamespace}}/git.resolve_impl_branch`), then either spawn the worker
  directly with `worktree: <destination>` or acquire that worktree and use
  the destination-bound key the acquisition returns. A key remains bound to
  the worktree where it was minted and is never rebound; handing an existing
  child another worktree without a destination-bound key cannot authorize ws
  writes there.
- Release that worktree before merging its branch, or when the unit ends
  unmerged.

#### Parallel mode

- Order units only by a strict dependency: `blocked-by:`, a prerequisite
  `related:` or `parent:`, or a body-stated need-first. Overlapping files
  never defer a unit; the merge step below checks interactions.
- Merge finished units one at a time.
- Earlier merges moved the target → first diff the target against the tree
  `git -c merge.directoryRenames=false merge-tree --write-tree <target>
  <branch>` prints, looking for interactions. The option matches
  `{{.McpNamespace}}/git.merge`, so a status
  directory the branch emptied raises no false conflict.
- An interaction or a merge conflict → `{{.SkillNamespace}}:lead-delegate`,
  which reruns the affected verification; the user sees it only when the
  delegate cannot settle it.
