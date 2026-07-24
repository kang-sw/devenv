---
title: lead-fan-out-worktree — lead-cognition parallel worktree orchestration overlay + session.note scratchpad
related:
  260605-epic-ws-playbook-factory-pivot: rides on the epic's landed session-auth + native-subagent architecture; not a pivot-migration milestone itself
  260605-research-ws-native-subagent-pivot: source of the session-key/scope/render-mint decisions this feature builds on (delegate scope, playbook.render root_override, session.children lineage)
sage-review-design: completed
sage-review-completeness: completed
---

# lead-fan-out-worktree — lead-cognition parallel worktree orchestration overlay + session.note scratchpad

## Background

A lead running a goal-style pass over `ready/` tickets today drains them one at a
time (`lead-goal-step`): select one ticket → dispatch → serial merge → repeat.
When the `ready/` tickets are independent, this leaves parallelism on the table.

This ticket adds a **lead-cognition-driven** overlay that lets one lead hold
several worktree-bound delegate session keys and juggle multiple parallel
implementers from its own context, coordinated with a lightweight per-child
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

**Delegation model — (1b) contained delegate children, lead owns shared state.**
- Each parallel worker is a **`delegate`-scope** session key bound to its own git
  worktree root, minted by the lead. Workers implement one pinned ticket, commit
  to their own branch, and stop.
- The lead centrally owns the operations that touch shared git state and must
  serialize regardless: `ready/` ticket **selection** (lead pins one ticket per
  worktree — no claim race on the shared git-tracked ticket tree) and **merges**
  to the shared parent branch (physically serial — git forbids the same branch
  checked out in two worktrees).
- **Review is delegated, not inline.** The lead spawns a separate reviewer
  delegate on a returned branch; review noise (diffs, test output, reasoning)
  stays in the reviewer's context and only a compact verdict returns to the lead.
  This is what keeps the lead window clean while juggling — the same pattern
  `lead-implement` already uses (mutation and review owned by subagents).

**Starting rung — begin at (1-min), escalate on real data.** Incremental ladder,
each rung additive:
- **(1-min):** thin delegate children direct-edit one ticket → own branch →
  stop; lead serial-merges; one aggregate verification pass at the end (full
  test / CI); no per-ticket review. Fastest test of the core hypothesis (can the
  lead juggle N parallel worktree implementers via a scratchpad).
- **(1b):** add per-branch reviewer delegates when per-ticket review quality
  matters (verdict-only back to lead).
- **(2) [deferred escalation, not this ticket]:** if lead juggling load proves
  too high, fuse implement+review into a fatter mini-lead child — requires
  `lead`-scope children, reopens depth-2 recursion, needs a spawn-depth backstop.
  Escalate to (2) only on measured overload, never speculatively.

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
The (1-min) recovery procedure for an orphaned-but-live child is therefore
**branch-state reconciliation at merge time** (inspect the child's worktree branch
— commits present, tests) and treat the ticket as complete/incomplete from that
git evidence, rather than attempting to resume the subagent. The playbook body
must encode this; do not promise live-run resume across compaction.

**Ship to both ws and wsflow (option B).** orchestra rides **100% native
subagents + session-auth core** (no `mercenary.*`, no `exec.*`). Only the
high-risk spawn/exec surface is ws-isolated; native-dependent shared surface
belongs in wsflow too. Rendering across `ws/` ↔ `wsflow/` is handled by the rsrc
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
- **Fat mini-lead children first (model 2).** Rejected as the starting point:
  needs `lead`-scope children → reopens depth-2 recursion and a spawn-depth
  backstop the pivot deliberately demoted to unnecessary. Kept only as a
  measured-overload escalation.
- **Inline review at the lead.** Rejected: pollutes the lead window with
  diffs/test-output; delegated review (verdict-only) is the clean form of (1).
- **A new `orchestra.*` namespace / new status-tool.** Rejected: status already
  exists as `session.children` (with working-dir `root` + `live`); note reuses
  the existing session record store under the existing `session.*` namespace.
- **In-memory-only note.** Moot/rejected: the session store is already
  file-backed per key, so notes are durable for free.
- **Capability tier build.** Rejected as unnecessary: the needed
  "ticket-move + own-branch commit, no spawn" middle tier already exists as
  `roleDelegate`.

## Constraints

- **Serial merge is a hard git constraint, unenforceable at the ws gate.** There
  is no `git.merge` MCP tool and delegates have their own harness shell, so "child
  must not merge to parent" is enforced by **playbook doctrine**, not the role
  gate. The overlay body must encode it.
- **Depth strictly 1.** `delegate` keys cannot mint/spawn (`ferrule` +
  `playbook.render` mint are lead-gated), so children cannot recurse. Preserve
  this — do not hand children `lead` scope in this ticket's scope.
- **`delegate` child running `lead-implement` must take the direct-edit /
  no-spawn path** — `playbook.render` will not mint a grandchild key for a
  non-lead caller, so a delegated implement that tries to spawn sub-implementers
  is inconsistent. Force direct-edit for the contained worker.
- **worktree creation is out-of-band shell** (`git worktree add`); there is no ws
  worktree tool and this ticket does not add one.

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

### Phase 2: `lead-fan-out-worktree` entry skill at the (1-min) rung

Thin `SKILL.md` shim (workflow_manual + `playbook.print` guidance, like
`lead-discuss`/`lead-goal-step`) + `rsrc/lead-fan-out-worktree/lead-fan-out-worktree.md`
playbook body, templated for ws + wsflow. Body encodes the (1-min) procedure and
the doctrine: (a) lead pins one independent `ready/` ticket per worktree; (b)
mint a `delegate` worktree child per ticket (`ferrule` / `playbook.render`
`root_override`) and dispatch a native subagent; (c) children never merge to the
parent — lead serial-merges returned branches; (d) `delegate` workers run
implement direct-edit / no-spawn; (e) `session.note` scratchpad + `session.children`
for status re-hydration; (f) one aggregate verification pass at the end; (g) name
the (1b)→(2) escalation, don't build it. Register as an entry skill (namespace
list + directly-invocable count). Depends on Phase 1.

Verification: the shim resolves through `workflow_manual` + `playbook.print(name:
"lead-fan-out-worktree")` — body renders with includes resolved and correct
ws/wsflow namespace substitution; entry-skill registration is reflected in the
namespace list and the bumped directly-invocable count; and one end-to-end
(1-min) dry run — lead creates a worktree, mints a `delegate` child, dispatches a
native subagent, round-trips `session.note` + `session.children`, and
serial-merges the returned branch — completes without a wrong-root or containment
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
`Contract-first spec: no` — behavior refined during the (1-min) prototype, no
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
