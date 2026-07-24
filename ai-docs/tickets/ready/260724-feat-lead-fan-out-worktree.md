---
title: lead-fan-out-worktree — lead-cognition parallel worktree orchestration overlay + session.note scratchpad
related:
  260605-epic-ws-playbook-factory-pivot: rides on the epic's landed session-auth + native-subagent architecture; not a pivot-migration milestone itself
  260605-research-ws-native-subagent-pivot: source of the session-key/scope/render-mint decisions this feature builds on (delegate scope, playbook.render root_override, session.children lineage)
sage-review-design: required
sage-review-completeness: required
---

# lead-fan-out-worktree — lead-cognition parallel worktree orchestration overlay + session.note scratchpad

## Background

A lead running a goal-style pass over `ready/` tickets today drains them one at a
time (`lead-goal-step`): select one ticket → dispatch → serial merge → repeat.
When the `ready/` tickets are independent, this leaves parallelism on the table.

This ticket adds a **lead-cognition-driven** overlay that lets one lead hold
several worktree-bound delegate session keys and juggle multiple parallel
mini-leads from its own context, coordinated with a lightweight per-child
scratchpad. The intelligence is the lead's context-switching, not a formal
scheduler — modeled on the observation that current models handle ADHD-style
parallel context-switching well. Auxiliary tooling assists; it does not own
control flow.

The session-key isolation this relies on is already landed (v0.36.1): the ws MCP
server keys `{session_key → root + capability scope}` with parent→child lineage
(`session_auth.go` `sessionStore`, `sessionEntry.parent`, `children()`), and the
worktree child-minting path (`ferrule`, `playbook.render` with `root_override`)
plus the status surface (`session.children`) already exist. The only genuinely
missing primitive is a per-child scratchpad note.

## Decisions

**Delegation model — mini-lead fan-out over native recursive dispatch.**
- Each parallel worker is a **mini-lead**: a native subagent holding a
  **worktree-bound `lead`-scope key** minted by the lead (`ferrule`,
  `capability: "lead"`). It bootstraps like any lead — `workflow_manual` then
  `playbook.print("lead-proceed")` — runs `lead-proceed` on its pinned ticket to a
  committed branch, edits the ai-docs it touches directly (a legitimate document
  owner), then holds at the merge gate without merging. The key must be `lead`, not
  `delegate`: `lead-proceed`/`lead-implement` call lead-only tools
  (`workflow_manual`, `enter.proceed`, …) refused for lower scopes at the role gate.
- **The enabling capability is native recursive subagent dispatch; ws-key depth is
  not the gate.** Native dispatch goes **depth-2** (a worker spawns its own
  implementer/reviewer), confirmed available in this harness (a subagent spawned a
  sub-subagent and returned its result). ws mint depth is **not** 1: a `lead`-scope
  worker running `lead-implement` mints its own implementer/reviewer keys (top lead
  → mini-lead → implementer). Containment is therefore by **playbook doctrine + role
  structure**, not a depth-1 gate — a mini-lead mints only `leaf`/`delegate`
  sub-workers (which cannot mint), so recursion terminates one level down, and there
  is no lead→lead→lead chain in the normal flow. A spawn-depth backstop is
  defense-in-depth, not a hard requirement.
- The lead centrally owns the shared-git operations that must serialize: `ready/`
  ticket **selection** (lead pins one ticket per worktree — no claim race on the
  git-tracked ticket tree) and **merges** to the shared parent branch (physically
  serial — git forbids the same branch checked out in two worktrees), each
  delegated to a per-branch **merge subagent** so conflict/diff noise (recurring
  `_index.md` overlap between doc-owning workers) stays out of the lead window.
  Review lives
  **inside** each mini-lead (its own reviewer sub-subagent; the verdict stays in
  the child), so review noise never reaches the lead window — the lead juggles N
  homogeneous "done, branch X" units, not interleaved implement/review threads.

**Single model, hard precondition gate — no degradation rung.** This skill exists
only for the flat mini-lead juggling above. There is **no** contained-implementer
fallback: if the harness cannot do recursive native subagent dispatch, the skill
**aborts** and directs the user to the serial `lead-goal-step` path instead — a
half-flat model (lead owning implement + review dispatch) is subagent-juggling
that defeats the skill's only purpose, so "unsupported" should refuse, not limp.
Detection is not a forced per-run probe: the capability is a stable harness fact
(recorded here as available for Claude Code); at runtime, if a mini-lead's first
nested spawn errors, the lead unwinds the fan-out cleanly and falls back to serial.
exec/mercenary isolation is **entirely out of scope** for this feature — the worker
surface is 100% native subagents.

**Deliverables (2):**
- **Skill (1):** `lead-fan-out-worktree` — a lead **entry** overlay skill (thin
  `SKILL.md` shim guiding `workflow_manual` + `playbook.print`, body in
  `rsrc/lead-fan-out-worktree/lead-fan-out-worktree.md`), same shape as
  `lead-discuss`/`lead-goal-step`. Nothing else depends on it (overlay, like
  `lead-prefer-subagent`).
- **MCP tool (1):** `session.note` — a per-child scratchpad the lead writes about
  a child session; surfaced in `session.children` output.

**`session.note` contract (sketch, exact fields finalize in implementation):**
- `session.note(session_key, child_session_key, text)` — the **lead** annotates a
  child key with a free-form one-line note. `session_key` is the lead's key
  (authorizes); `child_session_key` is the target.
- Note text is stored as an **additive field on the child's own per-session
  record** (`<cache-root>/keys/<child_session_key>.json`, the same store that backs
  agenda/todo/overrides) — keyed by `child_session_key`, not the lead's record, so
  the `session.children` handler reads each child's `Note` directly. No new
  schema; durable across process restart / compaction. Pin this locus in Phase 1.
- The note is **surfaced in `session.children` output** alongside each child's
  existing `key/scope/parent/depth/live/root`.
- **Lead-only gating is correct and free:** `session.*` is already rejected for
  `delegate`/`leaf` keys at the keyed-call handler, so only the orchestrator lead
  can write/read notes. The note is written *by the lead about the child*, so
  lead-only is the intended semantics (not a child self-note).

**Board value of the note = post-compaction / attention re-hydration, not
observability.** The board is a **lead scratchpad**: it is self-reported and can
go stale (a child may finish/die while its note still says "waiting"). It is NOT
child liveness observability — that comes from the harness agent handle. Its real
value: (a) surviving lead-context compaction (pairs with `lead-revive` — a
revived lead recovers its key, then `session.children` re-discovers children via
lineage), and (b) current models' attention concentrating on recent context, so
an early-spawned child falling out of focus is re-findable from the durable board
rather than the warm window.

**Re-attach boundary (recovery, not resume).** Re-discovering a child session
*key* via lineage is not the same as recovering the *harness agent handle* needed
to rejoin that child's in-flight run — the handle lives in the lead transcript
and may not survive compaction. So the durable board tells a revived lead that a
child exists and its last self-reported note, but not how to resume a live turn.
The recovery procedure for an orphaned-but-live child is therefore
**branch-state reconciliation at merge time** (inspect the child's worktree branch
— commits present, tests) and treat the ticket as complete/incomplete from that
git evidence, rather than attempting to resume the subagent. The playbook body
must encode this; do not promise live-run resume across compaction.

**Ship to both ws and wsflow (option B).** fan-out rides **100% native
subagents + session-auth core** (no `mercenary.*`, no `exec.*` — exec/mercenary is
out of scope). No high-risk spawn/exec surface is involved, so this
native-dependent shared surface belongs in wsflow too. Rendering across `ws/` ↔ `wsflow/` is handled by the rsrc
loader's namespace substitution (`{{.McpNamespace}}` / `{{.SkillNamespace}}`) —
one templated source, not a hand-maintained mirror.

**Name.** `lead-fan-out-worktree`. "fan-out" is the standard term for one
coordinator dispatching many parallel workers; `-worktree` disambiguates this
specific (git-worktree-isolated) variant and names the defining substrate. This
stem fixes the skill dir, `playbook.print(name:)`, and rsrc dir.

## Rejected Alternatives

- **Autonomous per-worktree workers racing a shared `ready/` queue.** Rejected:
  claim-race on the git-tracked ticket tree (git is not a lock server) and
  distributed merge contention. Selection + merge stay lead-central instead.
- **Contained-implementer models (thin worker + lead-owned review dispatch).**
  Rejected once native recursive dispatch was confirmed: they make the lead juggle
  interleaved implement and review threads — the exact subagent-juggling this skill
  exists to avoid. Review lives inside each mini-lead instead. (These were the
  earlier "(1-min)/(1b)" rungs; collapsed away.)
- **Delegate-scope workers (to keep ws mint depth at 1).** Rejected — impossible
  for this worker procedure: `lead-proceed`/`lead-implement` call lead-only tools
  (`workflow_manual`, `enter.proceed`), so a `delegate` key is refused at the role
  gate before the worker can even bootstrap. Workers must hold `lead` scope;
  containment shifts from the role gate to playbook doctrine. (An earlier draft of
  this ticket assumed delegate scope + ws-depth-1; that was wrong.)
- **Graceful degradation to a contained-implementer when recursion is absent.**
  Rejected in favor of a hard abort to serial `lead-goal-step`: a degraded
  half-flat model is subagent-juggling that defeats the skill's only purpose, so
  "unsupported" should refuse, not limp.
- **Inline review in the lead's own context.** Rejected: pollutes the lead window
  with diffs/test output; review belongs inside each mini-lead (verdict stays in
  the child), keeping the lead window clean while juggling.
- **A new `orchestra.*` namespace / new status-tool.** Rejected: status already
  exists as `session.children` (with working-dir `root` + `live`); note reuses
  the existing session record store under the existing `session.*` namespace.
- **In-memory-only note.** Moot/rejected: the session store is already
  file-backed per key, so notes are durable for free.
- **A new capability/role tier for the worker.** Rejected as unnecessary: the
  worker runs `lead-proceed`, so it needs full `lead` scope, which the existing
  `lead` capability already grants — no new intermediate tier.

## Constraints

- **Hard precondition: recursive native subagent dispatch.** The mini-lead model
  requires a worker (native subagent) to spawn its own implement/review
  sub-subagents. If a host cannot, the skill aborts to serial `lead-goal-step` (no
  degradation rung). The body states this host-neutrally as a **capability
  requirement**. *[adapter: Claude Code satisfies it — spawn workers in an
  `Agent`-tool-retaining form (general-purpose / claude), NOT Explore or Plan,
  which strip the spawn tool.]*
- **Workers must hold `lead` scope.** `lead-proceed`/`lead-implement` call
  lead-only tools (`workflow_manual`, `enter.proceed`, `enter.implement`,
  `playbook.render` mint), all refused for `delegate`/`leaf` at the role gate. A
  worker that runs `lead-proceed` therefore needs a `lead`-capable key minted with
  `ferrule(root: <worktree>, capability: "lead", parent_session_key: <lead key>)`.
- **Depth is bounded by playbook structure, not a role-gate depth cap.** Because
  workers hold `lead` scope they mint their own implementer/reviewer keys (top lead
  → mini-lead → implementer). This is safe because a mini-lead mints only
  `leaf`/`delegate` sub-workers, which cannot mint, so recursion terminates one
  level down. A spawn-depth backstop is defense-in-depth, not required.
- **Serial merge is a hard git constraint, unenforceable at the ws gate.** There
  is no `git.merge` MCP tool and workers have their own harness shell, so "worker
  must not merge to parent" is enforced by **playbook doctrine**, not the role
  gate. The overlay body must encode it.
- **Worktree topology is stated as intent-prose, not scripted git.** The body
  describes the isolation intent (implementers/reviewers isolated in worktrees;
  selection / doc + ticket updates / merges anchored to the lead's worktree) and
  lets the agent realize it with native worktree tooling (harness worktree
  isolation / `EnterWorktree`), rather than a hardcoded `git worktree add`
  sequence. There is no ws worktree tool and this ticket does not add one.

## Prior Art

- `lead-goal-step` — nearest analog (subagent ticket selection, single-ticket
  dispatch, confirmed serial merge); fan-out generalizes it to multiple parallel
  workers. Adjacent in the workflow-skills spec `## Planning Workflow Skills`.
- `lead-prefer-subagent` — the overlay-posture shape (no other skill depends on
  it; inline-body entry skill).
- `session.children` (`mcp-tools` `#260619-session-key-lineage-children`) — status
  surface reused verbatim; `ferrule` + `playbook.render` `root_override`
  (`#260610-*`) — worktree child-key minting reused; `roleDelegate` gating
  (`#260505-tool-profile-gating`) — the containment the model depends on.

## Phases

### Phase 1: `session.note` MCP tool

Add `session.note(session_key, child_session_key, text)` storing a per-child note
as an additive field on the existing per-session record store, surfaced in
`session.children` output. Lead-only via the existing `session.*` prefix gate.
Finalize exact field name / empty-omit discipline / clear-verb during
implementation. Tests: lead writes/updates a child note; note appears in
`session.children`; delegate/leaf key is rejected; note survives a re-read
(restart/compaction persistence). Deferred: any child-self-note path.

### Phase 2: `lead-fan-out-worktree` entry skill (mini-lead model)

Thin `SKILL.md` shim (workflow_manual + `playbook.print` guidance, like
`lead-discuss`/`lead-goal-step`) + `rsrc/lead-fan-out-worktree/lead-fan-out-worktree.md`
playbook body, templated for ws + wsflow. Body encodes the mini-lead procedure and
doctrine: (a) **precondition gate** — require recursive native subagent dispatch;
abort to serial `lead-goal-step` if a first nested spawn errors (adapter note:
spawn workers in an `Agent`-retaining form, not Explore/Plan); (b) lead pins one
independent `ready/` ticket per worktree; (c) lead mints one `delegate` key per
worker bound to that worktree root (`ferrule` / `playbook.render` `root_override`)
and dispatches a native mini-lead running `lead-implement` unchanged (its own
implement→review sub-subagents; review verdict stays in the child); (d) workers
never merge to the parent — lead serial-merges returned branches; (e) worktree
topology as intent-prose (native worktree tooling), shared-git ops anchored to the
lead's worktree; (f) `session.note` scratchpad + `session.children` for status
re-hydration; (g) one aggregate verification pass at the end. Register as an entry
skill (namespace list + directly-invocable count). Depends on Phase 1.

Verification: the shim resolves through `workflow_manual` + `playbook.print(name:
"lead-fan-out-worktree")` — body renders with includes resolved and correct
ws/wsflow namespace substitution; entry-skill registration is reflected in the
namespace list and the bumped directly-invocable count; and one end-to-end dry run
— lead creates a worktree, mints a `delegate` key, dispatches a native mini-lead
that itself spawns an implement/review sub-subagent and commits to its branch,
round-trips `session.note` + `session.children`, and serial-merges the returned
branch — completes without a wrong-root, precondition-gate, or containment
surprise.

### Phase 3: wsflow exposure verification + mirror

First probe the wsflow reduced runtime for exposure of `ferrule`,
`session.children`, `session.note`, and `playbook.render` mint. This probe fixes
the phase's remaining scope: for each tool found stripped, opening its wsflow
exposure is in-scope for this phase (mechanical exposure of an already-existing
tool per the established mirroring pattern, not new design). Completion boundary:
all four exposed in wsflow **and** the wsflow mirror-drift tests
(`python3 -m unittest discover agents-plugin-wsflow/tests`) green for the new
lead-* skill + session-tool surface, following `ai-docs/ref/wsflow-mirroring.md`.
Depends on Phases 1-2.

## Spec Impact

Ready addressing is via this section (implementation determines exact spec text;
`Contract-first spec: no` — behavior refined during the mini-lead prototype, no
externally-frozen contract needed before implementation).

- `ai-docs/spec/mcp-tools.md` — add a `session.note` entry beside
  `#260619-session-key-lineage-children` (its output-surface home) reusing the
  `#260625-session-state-tools` record store; pin lead-only `session.*` gating per
  `#260505-tool-profile-gating`.
- `ai-docs/spec/workflow-skills.md` — add a `lead-fan-out-worktree` entry under
  `## Planning Workflow Skills` (adjacent to the `lead-goal-step` cluster);
  register in `#260505-lead-skill-namespace-surface` and bump the
  directly-invocable count at `#260610-entry-skill-surface-reduction`.
- `ai-docs/spec/plugin-runtime.md` — note the new tool on the advertised
  capability surface (`#260506-runtime-capabilities-single-probe`) and record-file
  persistence for the note (`#260626-post-compaction-session-restoration`).
