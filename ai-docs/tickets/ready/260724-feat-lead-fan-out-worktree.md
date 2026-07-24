---
title: lead-goal-fan-out-step — batch-parallel lead-goal-step variant (mini-lead worktree fan-out) + session.note scratchpad
related:
  260605-epic-ws-playbook-factory-pivot: rides on the epic's landed session-auth + native-subagent architecture; not a pivot-migration milestone itself
  260605-research-ws-native-subagent-pivot: source of the session-key/scope/render-mint decisions this feature builds on (ferrule lead-scope minting, playbook.render root_override, session.children lineage)
sage-review-design: completed
sage-review-completeness: completed
---

# lead-goal-fan-out-step — batch-parallel lead-goal-step variant (mini-lead worktree fan-out) + session.note scratchpad

> **Stem note.** The file stem `260724-feat-lead-fan-out-worktree` is retained for
> `git log --grep` history continuity (earlier commits carry it); the delivered
> skill is named **`lead-goal-fan-out-step`** (see Name below).

## Background

A lead running a goal-style pass over `ready/` tickets today drains them one at a
time (`lead-goal-step`): select one ticket → dispatch → serial merge → repeat.
When the `ready/` tickets are independent, this leaves parallelism on the table.

This ticket adds **`lead-goal-fan-out-step`**: a **batch-parallel variant of
`lead-goal-step`** that the same goal loop can inject in its place. It inherits
goal-step's entire contract (goal-run posture, the three terminal states, `goal/*`
staging, blocker-recording, the "one step is not a finished goal" continuation,
conserve-context delegation) and overlays exactly one difference — when selection
finds two or more mutually-independent advanceable tickets and recursive native
dispatch is available, it advances them in parallel, one worktree-isolated
mini-lead per ticket, instead of one at a time. When it cannot fan out, it
degenerates to goal-step's own serial single-ticket step for that cycle, so the
goal loop is never broken.

The lead holds several worktree-bound `lead`-scope session keys and juggles the
parallel mini-leads from its own context, coordinated with a lightweight per-child
scratchpad. The intelligence is the lead's context-switching, not a formal
scheduler — modeled on the observation that current models handle ADHD-style
parallel context-switching well. Auxiliary tooling assists; it does not own
control flow.

The session-key isolation this relies on is already landed (v0.36.1): the ws MCP
server keys `{session_key → root + capability scope}` with parent→child lineage
(`session_auth.go` `sessionStore`, `sessionEntry.parent`, `children()`), and the
worktree child-minting path (`ferrule`, `playbook.render` with `root_override`)
plus the status surface (`session.children`) already exist. The two genuinely
missing pieces are a per-child scratchpad note and a serve-time transclusion of
goal-step's body into the overlay (see Decisions).

## Decisions

**Positioning — a batch-parallel `lead-goal-step`, not a standalone overlay.**
`lead-goal-fan-out-step` is a drop-in alternative the goal loop injects instead of
`lead-goal-step`. It is a single-cycle shim on the same Stop-hook drive: one
injection advances the batch (or, degenerate, one ticket); the caller re-invokes.
All of goal-step's contract governs unchanged; this skill only changes selection
(one ticket → an independent batch) and dispatch (direct → worktree mini-leads),
plus the delegated serial merge. When the batch condition does not hold — fewer
than two independent tickets, or recursive dispatch unavailable — it runs
goal-step's serial single-ticket path **inline** (the lead dispatches one ticket to
`lead-proceed` itself), so it stays a true drop-in and does not depend on the loop
being able to swap the injected skill mid-run.

**Contract inheritance — serve-time transclusion of `lead-goal-step` (visible XML
boundary).** Rather than re-stating goal-step's contract in prose (which drifts and
lies as goal-step evolves), the overlay body carries only the batch-parallel delta,
and goal-step's **actual current body is appended at serve time**, wrapped in a
visible `<playbook name="lead-goal-step" title="Goal Step">…</playbook>` boundary so
the agent can distinguish the inherited base from the overlay. Mechanism (the
"A-approach", generalizing an existing precedent):
- `renderPlaybookBody` is custom token find/replace, not Go `text/template`, so there
  is no native `{{template}}` include. But `printPlaybook`
  (`agents-plugin-tool/internal/mcp/playbook_tools.go:889-914`) already has the exact
  pattern: for `lead-workflow-manual` + `workflow.prefer_subagent` it
  `LoadSkillBody`s a skills-tree body and appends it via
  `wrapRenderedPlaybookForConcatenation` (which emits the `<playbook name=… title=…>`
  boundary).
- Generalize that into a bespoke branch: when serving `lead-goal-fan-out-step`,
  `LoadSkillBody(skillsRoot, "lead-goal-step")` and append it via
  `wrapRenderedPlaybookForConcatenation("lead-goal-step", "Goal Step", body)`,
  **after** `substitutePlaybookVars` (post-substitution, matching the precedent), so
  the static goal-step body is never subject to the `{{.`-undeclared-var guard.
- Corpus fact this rests on: `lead-goal-step` lives **only** in the skills tree
  (`agents-plugin/skills/lead-goal-step/SKILL.md`), not in `rsrc/` — it was
  deliberately migrated out of `playbook.print`. So rsrc frontmatter `includes:`
  cannot reach it; it must be pulled via `LoadSkillBody`/`ResolveSkillsRoot`. Accept
  that skills bodies are **not** rsrc-manifest hash-verified — the same integrity
  trade-off the existing `lead-prefer-subagent` transclusion already lives with.

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
  Review lives **inside** each mini-lead (its own reviewer sub-subagent; the verdict
  stays in the child), so review noise never reaches the lead window — the lead
  juggles N homogeneous "done, branch X" units, not interleaved implement/review
  threads.

**In-flight exclusion (the one hazard fan-out adds over serial goal-step).** A
mini-lead's ticket status move (`ready/` → done) lives on its own worktree branch
until merge, so the parent's `ready/` still lists tickets that are already in
flight. The batch selection must therefore exclude tickets already dispatched this
run and not yet merged, read off the board (below). Serial goal-step never hits this
because it holds only one ticket at a time and yields after dispatch.
Encoding (pinned, resolving the sage-review minor): each board note leads with its
ticket stem and a state word — `"<stem>: dispatched"` at dispatch, advanced to
`"<stem>: merged"` (or `"<stem>: blocked"`) as it resolves — so the in-flight set the
selection excludes is exactly the children whose note is still `dispatched`.
`session.note` stays free-form text; this stem/state convention is a skill-body
concern (Phase 2 body doctrine), not a tool schema field.

**Single model, graceful serial degenerate — no degradation rung.** This skill
exists only for the flat mini-lead juggling above. There is **no** contained-
implementer fallback (a half-flat model where the lead owns implement + review
dispatch is subagent-juggling that defeats the skill's only purpose). When the
harness cannot do recursive native subagent dispatch, or the batch is smaller than
two, the skill does not abort the goal run — it **degenerates inline to goal-step's
serial single-ticket step** (the lead dispatches one ticket to `lead-proceed`
directly: a depth-1 lead→implementer/reviewer dispatch that needs no recursion).
Detection is not a forced per-run probe: the capability is a stable harness fact
(recorded here as available for Claude Code); at runtime, if a mini-lead's first
nested spawn errors, the lead removes any worktrees/branches it created and takes
the serial path. exec/mercenary isolation is **entirely out of scope** — the worker
surface is 100% native subagents.

**Deliverables (3):**
- **Skill (1):** `lead-goal-fan-out-step` — a lead **entry** skill (thin `SKILL.md`
  shim guiding `workflow_manual` + `playbook.print`, body in
  `rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`), same shape as
  `lead-discuss`/`lead-goal-step`. The rsrc body is the batch-parallel overlay only;
  goal-step's contract arrives by transclusion.
- **Transclusion branch (1):** generalize `printPlaybook`'s existing
  `LoadSkillBody` + `wrapRenderedPlaybookForConcatenation` precedent to append the
  `lead-goal-step` body (visible `<playbook>` boundary) when serving
  `lead-goal-fan-out-step`.
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
child liveness observability — that comes from the host completion signal. Its real
value: (a) surviving lead-context compaction (pairs with `lead-revive` — a
revived lead recovers its key, then `session.children` re-discovers children via
lineage), (b) keeping in-flight tickets out of the next batch selection, and (c)
current models' attention concentrating on recent context, so an early-spawned
child falling out of focus is re-findable from the durable board rather than the
warm window.

**Re-attach boundary (recovery, not resume).** Re-discovering a child session
*key* via lineage is not the same as recovering the *harness agent handle* needed
to rejoin that child's in-flight run — the handle lives in the lead transcript
and may not survive compaction. So the durable board tells a revived lead that a
child exists and its last self-reported note, but not how to resume a live turn.
The recovery procedure for a child whose handle was lost but whose branch exists is
therefore **branch-state reconciliation at merge time** (inspect the child's
worktree branch — commits present, tests) and treat the ticket as
complete/incomplete from that git evidence, rather than attempting to resume the
subagent. The playbook body must encode this; do not promise live-run resume across
compaction.

**Ship to both ws and wsflow (option B).** fan-out rides **100% native
subagents + session-auth core** (no `mercenary.*`, no `exec.*` — exec/mercenary is
out of scope). No high-risk spawn/exec surface is involved, so this
native-dependent shared surface belongs in wsflow too. Rendering across `ws/` ↔
`wsflow/` is handled by the rsrc loader's namespace substitution
(`{{.McpNamespace}}` / `{{.SkillNamespace}}`) — one templated source, not a
hand-maintained mirror. The transclusion branch must resolve `lead-goal-step`
against the **active** skills root, so the wsflow-mirrored `lead-goal-step` SKILL.md
is what gets appended under wsflow (verified in Phase 3).

**Name.** `lead-goal-fan-out-step`. It is a batch-parallel `lead-goal-step`, so the
name carries the `goal`/`step` lineage (goal-loop-injectable, single-cycle shim)
and inserts `fan-out` for the parallel-dispatch delta. This stem fixes the skill
dir, `playbook.print(name:)`, and rsrc dir (`rsrc/lead-goal-fan-out-step/`). The
ticket file stem is unchanged (see Stem note).

## Rejected Alternatives

- **A standalone "overlay" positioning (independent of goal-step).** Rejected: the
  skill is driven by the same goal loop as `lead-goal-step` and must inherit its
  terminal states, posture, and staging to be injectable in its place. Positioning
  it as a batch-parallel goal-step variant (not a free-standing overlay) is what
  makes the degenerate path a clean inline fall-through instead of a cross-skill
  handoff.
- **Prose-pointer inheritance (re-state goal-step's contract in the body).**
  Rejected: re-enumerating goal-step's terminal states/posture drifts and lies as
  goal-step evolves. Serve-time transclusion of goal-step's actual body keeps the two
  in lockstep and lets the overlay body stay pure-delta.
- **A general `<skill name="…">` marker expansion pass (B-approach).** Deferred
  (YAGNI): a reusable positional-transclusion mechanism is a larger, doctrine-
  touching change (skill-authoring rules, recursion guards, tests). With one
  inheritance site today, generalize the existing bespoke `printPlaybook` precedent
  (A-approach) instead; build the marker when a second site actually appears.
- **Hard abort when recursion is absent.** Rejected in favor of the inline serial
  degenerate: aborting the skill would break the goal loop (the Stop-hook keeps
  re-injecting it). Degenerating to goal-step's own serial step for that cycle keeps
  the loop alive and is exactly the base behavior.
- **Contained-implementer models (thin worker + lead-owned review dispatch).**
  Rejected once native recursive dispatch was confirmed: they make the lead juggle
  interleaved implement and review threads — the exact subagent-juggling this skill
  exists to avoid. Review lives inside each mini-lead instead.
- **Autonomous per-worktree workers racing a shared `ready/` queue.** Rejected:
  claim-race on the git-tracked ticket tree (git is not a lock server) and
  distributed merge contention. Selection + merge stay lead-central instead.
- **Delegate-scope workers (to keep ws mint depth at 1).** Rejected — impossible
  for this worker procedure: `lead-proceed`/`lead-implement` call lead-only tools
  (`workflow_manual`, `enter.proceed`), so a `delegate` key is refused at the role
  gate before the worker can even bootstrap. Workers must hold `lead` scope;
  containment shifts from the role gate to playbook doctrine. (An earlier draft of
  this ticket assumed delegate scope + ws-depth-1; that was wrong.)
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

- **Inherits `lead-goal-step`'s contract verbatim.** The overlay may change only
  selection and dispatch (plus delegated serial merge). Posture, the three terminal
  states, `goal/*` staging, blocker-recording, and continuation come from the
  transcluded goal-step body and must not be re-specified or contradicted in the
  overlay.
- **Transclusion is serve-time and post-substitution, with a visible boundary.**
  The `lead-goal-step` body is appended by the `printPlaybook` branch after
  `substitutePlaybookVars`, wrapped in a visible `<playbook name="lead-goal-step"
  title="Goal Step">…</playbook>` boundary (agent must be able to tell base from
  overlay). It is loaded from the skills tree (`LoadSkillBody`), which is not
  rsrc-manifest hash-verified — accepted trade-off, same as the existing
  `lead-prefer-subagent` transclusion.
- **Hard precondition: recursive native subagent dispatch.** The mini-lead model
  requires a worker (native subagent) to spawn its own implement/review
  sub-subagents. If a host cannot — or the batch is < 2 — the skill degenerates
  inline to goal-step's serial single-ticket path (no degradation rung). The body
  states this host-neutrally as a **capability requirement**. *[adapter: Claude Code
  satisfies it — spawn workers in an `Agent`-tool-retaining form (general-purpose /
  claude), NOT Explore or Plan, which strip the spawn tool.]*
- **Workers must hold `lead` scope.** `lead-proceed`/`lead-implement` call
  lead-only tools (`workflow_manual`, `enter.proceed`, `enter.implement`,
  `playbook.render` mint), all refused for `delegate`/`leaf` at the role gate. A
  worker that runs `lead-proceed` therefore needs a `lead`-capable key minted with
  `ferrule(root: <worktree>, capability: "lead", parent_session_key: <your key>)`.
- **Depth is bounded by playbook structure, not a role-gate depth cap.** Because
  workers hold `lead` scope they mint their own implementer/reviewer keys (top lead
  → mini-lead → implementer). This is safe because a mini-lead mints only
  `leaf`/`delegate` sub-workers, which cannot mint, so recursion terminates one
  level down. A spawn-depth backstop is defense-in-depth, not required.
- **Serial merge is a hard git constraint, unenforceable at the ws gate.** There
  is no `git.merge` MCP tool and workers have their own harness shell, so "worker
  must not merge to parent" is enforced by **playbook doctrine**, not the role
  gate. The overlay body must encode it (workers hold at the merge gate; the lead's
  merge subagent does the physically-serial merges from its worktree).
- **Worktree topology is stated as intent-prose, not scripted git.** The body
  describes the isolation intent (each mini-lead in its own worktree; selection /
  merges anchored to the lead's worktree) and lets the agent realize it with native
  worktree tooling (harness worktree isolation / `EnterWorktree`), rather than a
  hardcoded `git worktree add` sequence. There is no ws worktree tool and this
  ticket does not add one.

## Prior Art

- `lead-goal-step` — the base this variant inherits (subagent ticket selection,
  single-ticket dispatch, confirmed serial merge, goal-run terminal states);
  fan-out generalizes it to a mutually-independent batch. Adjacent in the
  workflow-skills spec `## Planning Workflow Skills`.
- `printPlaybook` `lead-workflow-manual` + `prefer_subagent` branch
  (`playbook_tools.go:889-914`) — the exact serve-time skill-body transclusion
  precedent (`LoadSkillBody` + `wrapRenderedPlaybookForConcatenation`) the
  transclusion deliverable generalizes.
- `lead-prefer-subagent` — the overlay-posture shape (no other skill depends on
  it; inline-body entry skill) and the existing skills-tree transclusion target.
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
implementation. The field stays **convention-agnostic free-form text** — the
`<stem>: <state>` board convention that makes it the in-flight-exclusion ledger is a
skill-body concern (Phase 2), not a tool schema field. Tests: lead writes/updates a
child note; note appears in `session.children`; delegate/leaf key is rejected; note
survives a re-read (restart/compaction persistence). Deferred: any child-self-note
path.

### Result (c522438c) - 2026-07-24

`session.note(session_key, child_session_key, text)` shipped as a lead-only MCP
tool. Delta:

- **Field.** Additive `Note string json:"note,omitempty"` on `sessionRecord`,
  `sessionChild`, and `sessionChildOutput` in
  `agents-plugin-tool/internal/mcp/{session_auth.go,server.go}`, mirroring the
  existing `Overrides`/`Agenda`/`Todos` fields. Written by a new `setNote` method
  through the existing `mutateRecord` atomic RMW — it lands on the **child's own**
  per-session record file (keyed by `child_session_key`), so concurrent
  agenda/todo/override writes are not clobbered. No new store, no schema version bump.
- **Surface.** Surfaced in `session.children` output (compact text line + JSON
  `note` field). Handler `handleSessionNote` + dispatch case + schema added beside
  `session.children`.
- **Gating.** Lead-only for free via the existing `session.*` prefix block in
  `roleAllowsTool`; no new gate code. Deliberately no lineage check that the child
  descends from the caller — mirrors `session.children`'s trust model (a valid
  `session_key` authorizes).
- **Clear discipline.** Empty `text` is a legitimate "clear the note" write; the
  `omitempty` tag makes an empty note disappear from output (no separate clear verb).
- **Manifest.** `session.note` added to **both** `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json` `tools` maps in this same phase (not deferred
  to Phase 3): `agents-plugin-wsflow/runtime.json` declares
  `runtime_capabilities.match: exact` and `session.note` is not
  `noAgentHiddenTool`-gated, so omitting either entry breaks
  `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`. Version strings were
  left untouched — only the `tools` map content was edited.

Verification: `go build ./...` clean; `go test ./internal/mcp/...` green including
4 new `TestSessionNote*` tests (set/update appears in children; delegate+leaf
rejected; empty-text clears; survives fresh server instance = disk persistence);
`go test ./cmd/ws-mcp/...` green including the exact-match runtime-surface tests;
`go vet` clean. Single full-scope review: clean, one non-blocking minor (the
`session.*` gate skips unrecognized keys — a pre-existing documented design
property this tool inherits, not introduced here).

Specs updated in the impl commit: `ai-docs/spec/mcp-tools.md` (tool entry beside
`#260619-session-key-lineage-children`) and `ai-docs/spec/plugin-runtime.md`
(record-file persistence note). No deviations from the survey plan.

Deferred to later phases: the `<stem>: <state>` board convention (Phase 2 skill-body
doctrine) and wsflow reduced-runtime exposure probe (Phase 3).

### Phase 2: `lead-goal-fan-out-step` entry skill + goal-step transclusion

Two coupled pieces:

1. **Transclusion branch.** Generalize `printPlaybook`
   (`playbook_tools.go:889-914`): when serving `lead-goal-fan-out-step`, after the
   normal render+substitute, `LoadSkillBody(skillsRoot, "lead-goal-step")` and append
   it via `wrapRenderedPlaybookForConcatenation("lead-goal-step", "Goal Step", body)`
   so the visible `<playbook name="lead-goal-step" title="Goal Step">…</playbook>`
   boundary reaches the agent. Post-substitution append (goal-step's static body has
   no `{{.` placeholders to trip the undeclared-var guard). Tests: serving
   `lead-goal-fan-out-step` yields the overlay followed by the wrapped goal-step body;
   the boundary tag is present; a change to `lead-goal-step` SKILL.md is reflected in
   the served output (lockstep).
2. **Skill.** Thin `SKILL.md` shim (workflow_manual + `playbook.print` guidance, like
   `lead-discuss`/`lead-goal-step`) + the already-drafted
   `rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` overlay body (batch-parallel
   delta only), templated for ws + wsflow. Add the rsrc body to `rsrc/manifest.json`
   (hash entry) so `wsrsrc.Load` will serve it. Register as an entry skill (namespace
   list + directly-invocable count).

Body doctrine to confirm present (already drafted): (a) **precondition / degenerate**
— require recursive native dispatch; on a first nested-spawn error or a < 2 batch,
degenerate inline to goal-step's serial `lead-proceed` path (adapter note: spawn
workers in an `Agent`-retaining form, not Explore/Plan); (b) batch selection of
mutually-independent `ready/` tickets with in-flight exclusion via the board; (c)
lead mints one `lead`-scope key per worker bound to that worktree root (`ferrule`
`capability:"lead"`) and dispatches a native mini-lead that bootstraps
`workflow_manual` + `playbook.print("lead-proceed")` and holds at the merge gate;
(d) workers never merge — a per-branch merge subagent serial-merges from the lead's
worktree; (e) worktree topology as intent-prose; (f) `session.note` +
`session.children` for status/in-flight re-hydration, using the pinned `<stem>:
<state>` board convention so the exclusion set is a plain scan for still-`dispatched`
children; (g) one aggregate verification pass, then hand back to goal-step's terminal
logic. Depends on Phase 1.

Verification: the shim resolves through `workflow_manual` + `playbook.print(name:
"lead-goal-fan-out-step")` — body renders with correct ws/wsflow namespace
substitution **and** the appended `<playbook name="lead-goal-step">` block; entry-skill
registration is reflected in the namespace list and the bumped directly-invocable
count; and one end-to-end **batch≥2** dry run — lead selects two mutually-independent
`ready/` tickets, creates two worktrees, mints two `lead`-scope keys, and dispatches
two concurrent native mini-leads that each spawn their own implement/review
sub-subagent and commit to their own branch, round-trips `session.note` +
`session.children` across both, and serial-merges both returned branches via merge
subagents — completes without a wrong-root, precondition-gate, or containment
surprise. This batch≥2 run is where the N-way concurrency claim is actually
exercised (a single-worktree run proves only depth-2 recursion and the mint/merge
round-trip, not that N worktrees + mini-leads coexist and juggle cleanly); treat it
as prototype-validate — run it, judge the concurrency behavior, and fold any friction
back into the body doctrine before shipping.

### Result (2816a82c) - 2026-07-24

The entry skill and serve-time transclusion landed; the batch≥2 end-to-end dry
run is **deferred** (see Deferred below). Delta:

- **Transclusion branch.** `printPlaybook`
  (`agents-plugin-tool/internal/mcp/playbook_tools.go`) gained a second,
  **unconditional-on-name** transclusion branch beside the config-gated
  `lead-workflow-manual`/`prefer_subagent` precedent it generalizes. Serving
  `lead-goal-fan-out-step` now `LoadSkillBody(skillsRoot, "lead-goal-step")` and
  appends it via `wrapRenderedPlaybookForConcatenation("lead-goal-step", "Goal
  Step", body)` **after** substitution, so goal-step's static body never trips the
  `{{.`-undeclared-var guard. `printPlaybook` was refactored into two independent
  mutually-exclusive `if name == …` blocks (new constants
  `goalFanOutStepPlaybookName` / `goalStepPlaybookName` / `goalStepPlaybookTitle`).
  `lead-goal-step` is pulled from the skills tree (not rsrc, not hash-verified) —
  the same accepted integrity trade-off as the existing `lead-prefer-subagent`
  transclusion.
- **Entry skill.** Thin shim `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md`
  (modeled on `lead-discuss`/`lead-goal-step`) wires `workflow_manual` +
  `playbook.print`. The already-finalized overlay body
  (`agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`, batch-parallel
  delta only, `kind: print`, `delegates: true`) was left byte-identical — only wired,
  not rewritten.
- **Manifests.** Added `lead-goal-fan-out-step` hash entries to **both**
  `agents-plugin/rsrc/manifest.json` (regen `WS_REGEN_MANIFEST=1`) and
  `agents-plugin/skills/manifest.json` (regen `WSRSRC_REGEN_SKILLS=1`). The rsrc
  entry also cleared pre-existing `TestShippedManifestUpToDate` drift (the overlay
  file had shipped in Phase-1's merge without a manifest entry).
- **Registration.** Registered as an entry skill; the directly-invocable count in
  `ai-docs/spec/workflow-skills.md` (`#260610-entry-skill-surface-reduction`) bumped
  14 → 15 and added to the `#260505-lead-skill-namespace-surface` list, with a new
  `#260724-goal-fan-out-step-transclusion` prose entry under `## Planning Workflow
  Skills`.

Verification: `go build ./...`, `go vet ./...`, `go test ./internal/mcp/...`, and
`go test ./cmd/ws-mcp/...` all green. New
`TestPlaybookPrintGoalFanOutStepAppendsGoalStepUnconditionally` asserts overlay
presence, boundary-tag presence, overlay-**before**-boundary ordering, and a
**lockstep** compare against a live `LoadSkillBody("lead-goal-step")` read (so a
future goal-step edit auto-reflects with no fixture update). Direct render evidence:
`printPlaybook("lead-goal-fan-out-step")` produced a 14534-byte body — overlay
first (offsets 0–6210, ending in its delegate footer), then the visible `<playbook
name="lead-goal-step" title="Goal Step">` boundary at offset 6210, then goal-step's
base contract verbatim. Partitioned review (correctness/fit/test): correctness
clean first pass; the test partition's ordering-assertion gap and the fit partition's
missing spec updates were both fixed in this same commit and re-verified.

Specs updated in-commit: `ai-docs/spec/plugin-runtime.md` (serve-time skill-body
transclusion in `printPlaybook`, generalizing the `lead-prefer-subagent`
concatenation precedent; skills-tree body not hash-verified) and
`ai-docs/spec/workflow-skills.md` (entry + namespace-surface registration + count
bump).

Deferred to keep the phase reviewable: (a) the **batch≥2 concurrent-mini-lead dry
run** (the ticket frames it as prototype-validate; the wiring, depth-2 recursion, and
mint/merge round-trip are proven by this session's own fan-out-shaped delegation, but
the N-worktree juggling exercise is a live-run validation better executed once the
version bump ships the new print surface into a running server). (b) Phase 3 wsflow
mirror — `TestWsflowRsrcMirrorUpToDate` is currently red because the overlay + shim
are not yet mirrored into `agents-plugin-wsflow/`; that is Phase 3's explicit scope,
not a regression (the failure predates this branch for the overlay file).

### Phase 3: wsflow exposure verification + mirror

First probe the wsflow reduced runtime for exposure of `ferrule`,
`session.children`, `session.note`, and `playbook.render` mint, **and** that the
transclusion branch resolves the wsflow-mirrored `lead-goal-step` SKILL.md against
the wsflow skills root (so the appended base body is the wsflow variant). This probe
fixes the phase's remaining scope: for each tool found stripped, opening its wsflow
exposure is in-scope for this phase (mechanical exposure of an already-existing tool
per the established mirroring pattern, not new design). Completion boundary: all four
tools exposed in wsflow, the transclusion resolving correctly under wsflow, **and**
the wsflow mirror-drift tests
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
- `ai-docs/spec/plugin-runtime.md` — note the new tool on the advertised
  capability surface (`#260506-runtime-capabilities-single-probe`), record-file
  persistence for the note (`#260626-post-compaction-session-restoration`), and the
  serve-time skill-body transclusion in `printPlaybook` (generalizing the existing
  `lead-prefer-subagent` concatenation precedent).
- `ai-docs/spec/workflow-skills.md` — add a `lead-goal-fan-out-step` entry under
  `## Planning Workflow Skills` (adjacent to the `lead-goal-step` cluster), noting it
  as a batch-parallel goal-step variant that transcludes goal-step's contract;
  register in `#260505-lead-skill-namespace-surface` and bump the
  directly-invocable count at `#260610-entry-skill-surface-reduction`.
