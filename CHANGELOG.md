# Changelog

## v0.41.1 - 2026-08-20

### Changed
- **Ready-promotion relaxed to dependency closure.** `lead-write-ticket` now
  lands a ticket in `ready/` when the tickets its earliest unfinished phase
  block-depends on are in `ready/`, `.done/`, or the same bulk-promotion action —
  a closed work front that drains in dependency order — instead of refusing
  `ready/` until every dependency had landed in `.done/`. Dependencies count only
  through `related: <stem>: prerequisite` / prerequisite `parent:` edges; a
  prose-only mention or an epic-hierarchy `parent:` does not.
- **Bulk ready promotion.** A dependency chain can be promoted to `ready/` in one
  action via the new `## On: Bulk Ready Promotion` handler: prerequisites first,
  each ticket's spec-address and sage-review gates run, the landed set committed
  as one unit, the promoted prefix committed on a mid-run block. Cascade Edit now
  also runs the sage review gate per `ready/`-entering target, closing a latent
  gap where cascaded promotions skipped it.

## v0.41.0 - 2026-08-14

### Added
- **Note visibility mute (`note.mute` / `note.unmute`).** New MCP verbs plus a
  `visible` field on notes: muted notes are preserved in storage but excluded
  from ambient `# Notes` injection, giving callers a way to park stale memory
  without erasing it.
- **Git-tracked repo note layer.** A `repo` note layer that stores notes inside
  the tracked tree (`ai-docs/ws-notes/`) with its lock kept outside the tracked
  dir, so durable per-project memory can ride the git index and travel with
  clones.
- **Clone note layer.** A project-scoped, worktree-agnostic `clone` layer for
  untracked local clone context, decoupled from the per-worktree layer.
- **Multi-layer `note.search`.** `note.search` accepts an optional/multi-layer
  selector with a shared 3-key ordering, so one query can span note layers.
- **Always-on `# Manuals` authoring anchor.** The `# Manuals` block is now an
  ambient authoring anchor in the workflow manual, and `note.*` durable-memory
  capture is surfaced through the workflow manual.

### Changed
- **`_index.md` dissolution.** `ai-docs/_index.md` is dissolved into
  `AGENTS.md`, the `manuals/` tier, note layers, and generated inventories;
  `lead-bootstrap` writes to those homes with versioned coexistence degrade.

### Removed
- **`manuals.list` / `manuals.find` retired.** The manuals discovery MCP tools
  and their CLI mirror are removed; the ambient `# Manuals` anchor replaces the
  discovery surface. `manuals.list` remains internally as the anchor's data
  source only.

### Fixed
- **wsflow runtime contract drift.** Added `note.mute` / `note.unmute` to both
  `runtime.json` tool contracts.
- **Repo-layer note lock relocation.** Moved the repo-layer flock out of the
  tracked note directory so it never lands in the git index.
- **Dissolve-index degrade coverage.** Extended the if-present degrade to the
  shared `rsrc/` conventions surface and dropped a dead bump-script regex.

## v0.40.0 - 2026-08-11

### Added
- **Note-memory layers (`note.write` / `note.erase` / `note.search`).** A new
  MCP tool family backing non-tracked, per-machine and per-worktree note memory.
  Notes are key-addressed, overwrite-on-write, and ambiently injected into the
  workflow manual under a `# Notes` section; `note.search` supports glob key
  patterns and inclusive `from`/`then` date bounds. This gives a worktree a
  stable scratch memory that survives session churn without riding the git
  index, and closes the worktree-local-index gap.
- **`manuals/` documentation tier.** A dedicated procedure-manual tier with MCP
  discovery and ambient `# Manuals` injection into the workflow manual, distinct
  from static `ref/`. Six live procedure references were migrated `ref/ ->
  manuals/`, and `lead-bootstrap` routes procedure findings into it.
- **Idea-ticket attention policy.** `idea/` tickets are brought into a scoped
  worktree's topic view via add-only sparse staging, and `project_tree` folds
  orphan `idea/` tickets into a single hidden-count line while rendering
  parented epic-children and all `ready/`/`todo/` in full. Full idea bodies stay
  reachable via `tickets.list(status:"idea")`.

### Changed
- **Implement branches encode their merge base.** The implement-branch resolver
  now names branches `impl/<merge-root>/<stem>`, mirroring the goal-branch
  convention so an impl branch declares the base it merges into and merge-target
  discovery is unambiguous.

### Fixed
- **Statusline resolves `jq` explicitly** instead of relying on `PATH`, so it no
  longer breaks under a stripped environment.
- **Skills manifest drift** for `lead-scope-worktree` regenerated; the
  skills-manifest is a third regen surface that had gone stale.
- **`note.search` date bound** `then` is now inclusive of the whole day, and the
  `written_at` re-stamp path is covered via an injectable clock.

## v0.39.0 - 2026-08-07

### Added
- **`ws:lead-scope-worktree`** — a new entry skill that scopes a worktree's
  ticket board to one work line via `git sparse-checkout --no-cone`, hiding
  out-of-topic `ready/`/`todo/` tickets while `idea/` stays visible. It always
  discusses the target with the user first, derives the pattern set from that
  conversation, applies it, verifies by listing the affected directories, and
  offers `git sparse-checkout disable` restore. Its reference manual lives at
  `ai-docs/ref/worktree-ticket-scope.md`.
- **`workflow_manual` sparse-checkout scope announcement.** When
  `core.sparseCheckout` is active, `workflow_manual` now renders the active
  scope and names the hidden ticket stems in both its FRESH-with-root and
  CONTINUE branches; its output is byte-unchanged when the scope is unset.

### Changed
- **Index-aware ticket board resolution for scoped worktrees.** Ticket board
  tools resolve tickets hidden by a worktree sparse-checkout scope from the
  index rather than the working tree, so scoped worktrees no longer under-report
  the board. Cross-scope ticket moves refuse as true atomic no-ops, and the
  filter-off path stays byte-identical to the unscoped behavior.
- **`mcp-server-repair` skill description** retuned to the agent's own failure
  vocabulary so self-invocation triggers on the symptoms an agent actually
  reports.
- **Statusline** consolidates its subprocess forks per render.

### Removed
- **ws-dashboard** source tree and its documentation, completing the dashboard
  drop sweep; the board was reconciled and the archived tree removed.

## v0.38.0 - 2026-07-30

### Added
- **`lead-prefer-subagent` now ships inside `lead-drain-ready-queue`.** Every
  goal run invoked the two together, so the standing directive had to name both.
  The prefer-subagent body is spliced into the drain skill's committed
  `SKILL.md` at build time — verbatim, frontmatter-stripped, wrapped in the same
  `<playbook>` boundary the serve-time hooks use — rather than transcluded at
  serve time, which would have forced the drain skill back into being a
  `playbook.print` shim on the hottest path in the loop.

- **Build-time skill-body composition** (`wsrsrc.ComposeSkillBody`), behind
  `WS_REGEN_COMPOSED_SKILLS`, with a drift guard and an idempotence guard.
  Regeneration replaces the existing region by its delimiter pair instead of
  appending, and runs before the wsflow namespace mirror so both packages derive
  from one composed source.

### Changed
- **`lead-goal-step` is now `lead-drain-ready-queue`.** The rename reverses the
  one `v0.35.0` made, because the mechanism fact that one rested on was wrong.
  The `/goal` Stop-hook's continue-vs-stop judge reads the **skill name plus the
  transcript**, not the skill body — which compaction may already have discarded
  — and it runs on a weaker model than the main agent. Under that mechanism
  `step` is actively harmful: a weak judge seeing `lead-goal-step` and a
  transcript in which one step visibly completed has every reason to call the
  run over. `drain-ready-queue` instead names the object whose exhaustion *is*
  the termination test, so the stop decision reduces to a lookup. The name stops
  at the invariant process shape and deliberately omits the terminal set, which
  keeps changing.

  The `step` framing is gone from the `description:` frontmatter and the body's
  opening line too, not just the stem — the description is what a harness skill
  listing pairs with the name, so leaving it would have reinstalled the same
  inference one layer down.

  Only the name layer changed. The body keeps the goal-run posture, terminal
  states, `goal/*` staging, and curation authority exactly as they were. This is
  a minor bump because it replaces a caller-visible Codex skill entry point, the
  same reason `v0.35.0` took one for the rename in the other direction.

- **The Open Decision Queue is now asked as one batch interview.**
  `lead-write-ticket` previously restated one queued item per response and
  waited for an answer before asking the next. That serialization was the
  defect: queue items co-vary, so a placement decision split across three
  questions commits the user to the first before the third reveals it wrong.
  The downstream field report behind `v0.37.0`'s ledger fix already recorded the
  symptom — two of seven items were materially revised on contact with the
  actual question, which is what serial asking maximizes by deferring contact
  for every item but one.

  One response now restates every open item's full text, each carrying the
  skill's recommendation for it. The user answers across the whole batch in
  prose, weaving items that depend on each other.

  The gate is not weakened. Its invariant was never "ask one at a time" but
  "every item receives an explicit disposition", and that now runs through an
  explicit reconcile step: the skill maps the answer to items, updates the
  visible queue, and re-batches whatever the answer did not reach. A
  recommendation is a proposal and never a default, an unreached item stays
  `open`, and no round limit converts a still-open item into a disposition the
  user never gave.

  Where an answer plausibly reaches an item but not unambiguously, the skill
  states its reading on its own line and continues rather than asking again —
  "reading [1] and [2] as confirmed and [3] as deferred, continuing." The brake
  is that visible declaration. A grammar of what counts as an explicit agreement
  signal, with a re-ask for anything failing it, was drafted and rejected: it
  reinstates a confirmation turn per batch and spends back the round trips
  batching saves.

- **Both `task-list` host variants become the state record, not the channel.**
  `v0.37.0` made response-body prose the load-bearing channel but left the
  includes calling the list "the consent ledger" without mentioning the prose
  they now depend on, so a reader of the include alone still saw the list as the
  mechanism. The list now holds the item set and each item's
  `open`/`confirmed`/`rejected`/`deferred` status — what survives the lead's own
  compaction and what makes "did every item get a disposition?" checkable — and
  their serial-rhythm rules are gone, since there is no "next item" in a batch.
  Recommendations stay out of the list: added length is the input to the render
  truncation that fix addressed, and an agent can regenerate a lost
  recommendation where a truncated ledger is simply unreadable.

  Item composition is unchanged. The item's visible text is still the decision
  itself rather than a label, secondary note and description fields still carry
  nothing load-bearing, and both rules still govern the Markdown-checklist
  fallback.

### Fixed
- **`config.agents_tier` is now reachable in agentless wsflow.** It had been
  grouped with the named-agent surfaces (`mercenary.*`) that `WS_MCP_NO_AGENT=1`
  hides, but model tier is not a named-agent concept — it selects which model
  backs a delegate, which agentless wsflow still does. The effect was that
  wsflow users had no access point for the one tuning knob they were most
  likely to want, and `lead-tune`'s model-tier route pointed at a tool that was
  not advertised.

  The tool is added to the wsflow runtime gate list, and `lead-tune`'s
  `ws:full-only` markers move so that the model-tier handler and its routing
  row fall outside the full-only region. `config.tuning` now lists the
  `agents.tier` knob in both product modes; `workflow.prefer_mercenary` stays
  full-only. The catalog append for `agents.tier` sits before the
  `noAgentMode` early return, which is what makes it survive into the wsflow
  catalog.

## v0.37.3 - 2026-07-30

### Added
- **`lead-write-ticket` now grounds a ticket in facts before the design review
  reads it.** The reported symptom was a coin flip: the lead drafts a ticket
  without research, design review returns a mix of "this is factually wrong" and
  "this is badly designed", and the lead cannot tell which finding is which, so
  it explores by hand or guesses. A new **Ground** stage sits between Verify and
  the sage review gate and delegates claim-checking to a new read-only
  `ticket-fact-populator`, which returns corrections the lead applies itself and
  never writes the ticket. Design review then judges design, having stopped
  arbitrating facts.

  The populator checks the ticket against the **ticket corpus** as well as the
  tree. One `tickets.list` sweep shortlists tickets whose title or unresolved
  phase titles cover work this ticket also claims, and a real overlap is reported
  as a decision gap naming both stems. The same listing supplies the status of
  every ticket this one names as a blocker, predecessor, or landing-order
  constraint, so a dependency sitting behind this ticket's landing status is
  caught mechanically. These checks sit in the populator on a cost argument, not
  a role argument: one listing plus a shortlist read makes them without pulling
  ticket bodies into the lead context.

- **The design reviewer receives a `relations` premise ledger and takes every
  entry as landed.** A ticket written against work that has not shipped yet used
  to read as broken design, because the reviewer could see the premise but not
  the ticket that would satisfy it. The populator now emits `relations` as a fact
  table that stays complete even when nothing about it is wrong, and the reviewer
  sketches its implementation plan with those entries assumed present. A premise
  the table accounts for is a sequencing fact; only an **undeclared** premise is
  a design defect. The reviewer also reads the `parent:` epic body, which owns
  cross-child invariants that child tickets do not restate.

- **`tickets.sage_stamp` routes review issues by `resolution`.** Both reviewer
  playbooks have always emitted `resolution: autonomous|missing`, but nothing in
  the calling playbook ever mentioned the field — the split was produced and then
  discarded, leaving the caller to improvise. The tool's `next_instruction` now
  counts issues by resolution and routes them: `autonomous` to the implementing
  stage, `missing` to the Open Decision Queue that already served as the consent
  ledger for exactly this.

### Changed
- **`missing` is now cut on policy-ness, never on discovery cost.** Both
  reviewers previously let an expensive lookup read as a blocking gap. An issue
  is `missing` only when it is a policy choice the planning or implementation
  stage cannot make — what the system should do, what contract it commits to,
  which of several defensible shapes is correct. A severity floor was considered
  and rejected: it is a proxy that leaks genuine minor-severity policy gaps
  through while still blocking on cheap lookups.

- **A dependency-blocked ticket no longer reaches `ready/`.** `ready/` means
  direct implementation target, and a ticket that cannot be started is not one.
  `judge: initial-status` now refuses `ready/` when the **earliest unfinished
  phase** waits on a ticket that has not landed, however complete the spec
  addressing, and names the blocking stem. Cutting on the earliest unfinished
  phase rather than the whole ticket keeps a ticket startable when only a later
  phase waits.

- **The design reviewer may read a source file the ticket cites**, and only to
  check a claim the ticket makes about it. Without this the populator would be
  the only source-reading agent in the ticket pipeline, unaudited on the very
  axis it exists for.

- **An epic child that does not exist yet is recorded, not deferred.** Populate
  step 6 used to stop the invocation for a separate one scoped to that child;
  when the child had no ticket, that pointed at a ticket nobody could open. The
  constraint now lands as the epic skeleton's `- Planned:` entry and the
  invocation continues.

- **Both release jobs carry `timeout-minutes`** (build 20, windows-smoke 25). The
  v0.37.2 run had a `go test ./...` step sit in progress for 46 minutes before
  the job died without uploading logs, so there was no evidence of where it hung;
  a re-run of the same commit passed in 6m21s. The caps sit well above observed
  normal duration — the goal is turning a silent hang into a legible failure, not
  tightening the budget.

### Fixed
- **Recording a sage-review block no longer deletes later ticket content.**
  `appendOrReplaceBlockedSection` cut from the first `## Blocked (` heading to
  EOF, on the assumption stated in its own doc comment that a Blocked section is
  always last. It is not — a lead records what changed while the ticket waited by
  adding `## ` sections after it. One live ticket in this tree lost 131 lines of
  dependency-landing analysis to a second block verdict, and the caller committed
  the deletion. Excision now runs only up to the next `## ` heading, and every
  prior Blocked section is removed rather than just the first, so a hand-placed
  duplicate cannot survive as a stale blocker beside the fresh one.

- **The block verdict no longer prescribes a recovery loop that cannot run.** It
  told the caller to resolve the issues and re-stamp with fresh verdicts, but
  `resolveStage` returns `stop_blocked` for a blocked posture forever, so
  `sage_gate` never names a reviewer again. The instruction is now
  stop-and-report, matching `sageGateNextInstruction`. Separately, the block
  branch used to return the same "commit, then proceed to handoff" text as a
  pass, so a block at a `todo/` landing was written to frontmatter and then
  dropped — with a later promotion refused by a posture nothing told the caller
  how to clear.

- **An unlanded-dependency claim is no longer silently dropped.** The populator
  routes such claims to `unverified` rather than correcting them, but the handler
  consumed only corrections and decision gaps, so those claims vanished and
  `judge: populator-round-limit`'s continue condition was evaluated against state
  the lead never held. The lead now states the named dependency in the ticket
  where the claim sits.

- **`Reviewer Spawn` can now reach a blocked stage this invocation's edits
  address.** A `stop_blocked` gate names no reviewer, so the playbook had no
  route to the fresh verdicts that `sage_stamp`'s own `next_instruction` names as
  the only exit from a blocked posture.

- **Smaller sage-record and reviewer corrections.** The missing-decision
  escalation sentence is gated on there being a missing issue, since a standalone
  stage can record `concern` with none. A landing-specific "do not land in
  `ready/`" clause was dropped from a tool that has no landing input. The
  issue-routing clause moved ahead of the verdict's own direction so fixes are
  not stated behind the commit. An absent or unrecognized `resolution` value now
  counts as `autonomous` instead of being dropped. The populator's evidence rule
  was widened — a mandatory `path#Lstart-Lend` citation forbade its own primary
  correction shapes, absence findings and multi-site counts. And
  `ticket-reviewer-completeness` finished a half-migrated resolution axis whose
  checklist still carried the old design-shaped wording.

## v0.37.2 - 2026-07-29

### Changed
- **A delegate's own session may now be continued instead of always being
  respawned.** `lead-prefer-subagent` said a standing role "always takes a fresh
  spawn — this is unconditional". That over-reached: the rule it came from
  deleted the *fork* construct, which inherited the **lead's** conversation, and
  never indicted continuing a delegate's **own** session. The absolute was
  conflicting with three live surfaces — the same skill's authoring whitelist,
  which already sanctioned "the delegated subagent's own continuing session";
  the runtime-injected continuity tip, which tells the lead to reuse an agent id
  rather than respawn; and the rule that delegated review fixes return to the
  implementer that wrote the code. "Always takes" is now "opens with", so a
  standing role's *first* spawn stays unconditionally fresh and the anti-fork
  guarantee is untouched, while later turns may continue.

  Continuation is scoped by **work-item identity, not role fit**: continue when
  the instruction is the same work item that delegate already owns — a review
  finding relayed back to its implementer, a widened query to the explorer that
  ran it, a gap filled by the survey agent that produced it. Open a fresh spawn
  when the work item is new, or when the judgment must not inherit the prior
  agent's conclusion, such as an independent review verdict or a re-check of a
  claim that agent itself made. A role-fit test would have allowed handing
  ticket B to the implementer that just finished ticket A, dragging A's context
  across a work boundary — the same contamination class fork was deleted for.

  Deliberately not added: agent-id bookkeeping prose, which the runtime
  continuity tip already delivers at the point of need, and long-running-session
  retirement prose, which has no reliable signal for the lead to act on yet.

### Fixed
- **The workflow-skills spec no longer claims a single delegation carve-out.**
  Its posture paragraph scoped "the sole carve-out" to freshness, which stopped
  being true once continuation landed. It now records two carve-outs on distinct
  axes — durable-artifact authoring governs *who* authors, continuation governs
  *when* an existing delegate is reused — and drops the "two clean delegation
  poles" framing. Collapsing those axes is what let the same absolute read true
  in the skill body and false in the spec.

## v0.37.1 - 2026-07-29

### Fixed
- **A git submodule now resolves as its own ws project instead of failing
  outright.** `wsstate` derived a project's identity from `--git-common-dir` and
  required that path to end in `.git`. A submodule's common dir is
  `<parent>/.git/modules/<path>`, so resolution errored, and because
  `playbook.render` resolves paths unconditionally, a submodule session would
  open normally — `ferrule`, `workflow_manual`, and the doc, ticket, and git
  tools all worked — and then die the moment any delegate prompt was rendered.
  A submodule now resolves to its own `projectKey` with no `@` suffix, sharing
  no cache state with its superproject. Detection is hooked on the existing
  guard's failure path rather than ahead of it, so no layout that resolved
  before can be reclassified. Worktrees created inside a submodule remain
  unsupported and still fail loudly, except at a path the superproject itself
  records as a gitlink.
- **The statusline's last-update pill no longer breaks on macOS or during an
  active turn.** Reading the transcript in reverse depended on `tac`, which
  macOS lacks, and newest-line-first meant a single partially written line —
  routine while the transcript is being appended — aborted the read before any
  timestamp was emitted. Reading forward degrades to the last good timestamp
  instead. The pill also counted tool-result entries, which are typed `user`,
  so it reported activity recency rather than the output-token recency it is
  labelled with. Transcript paths are normalized at the point of use for
  Windows, and the cache-hit `awk` no longer trips gawk's constant folding of
  `0 / 0` on a fresh session.

### Changed
- **`workflow_manual` now names `impl/*` and `goal/*` as workflow-owned
  branches that merge with `--no-ff` by default.** The manual previously said
  only to use native git for merge execution and never mentioned either branch
  prefix, so leads occasionally fast-forwarded an implementation branch and
  flattened its plan, review, and doc-closeout history. This states the default
  and the squash case; it does not change the merge rules in
  `workflow-skills.md`, which already specified them.
- **The statusline's L3 row drops agent think-time and output tokens/sec.**
  Cache hit rate moves next to the total-session-runtime pill, and the freed
  pill now shows when output tokens last changed, derived from the transcript
  the status JSON already points at rather than from new persisted state. The
  indicator reports absolute `HH:MM` only — the relative age cost up to 11
  columns on a row that already competes for width.

## v0.37.0 - 2026-07-28

### Added
- **A new `lead-backfill-docs` skill reconciles documentation for commits that
  never went through an implementation doc pass.** It groups undocumented
  commits, dispatches a read-only `doc-gap-discovery` delegate per group to find
  spec and mental-model gaps, then runs the existing spec and mental-model update
  passes against them. Mental-model coverage is swept once across the whole audit
  window rather than once per group, so two dispatches can no longer edit the
  same document from partial views. `lead-discuss` names the skill whenever a
  stale-docs observation traces back to specific undocumented commits, so it is
  reachable without already knowing it exists.
- **A `review-adjudicator` delegate settles contested review findings.** When an
  implementer disputed a finding, the dispute had no owner — neither side could
  settle it, and the relay loop stalled. The adjudicator judges only whether the
  implementer's stated defense holds and never re-reviews the diff for
  correctness, and `implementer-relay` gained an `[escalate: <reason>]` token so
  a lead can hand a contested finding over instead of looping.
- **`lead-implement` escalates to an elevated implementer after a fix relay fails
  once, or when several findings share a root cause.** Re-review now uses
  symmetric `[resolved]` / `[unresolved: <reason>]` tokens, so a `[fixed]`
  finding that comes back `[unresolved]` is a structural signal rather than prose
  the lead had to infer. The previous relay wording — "only for genuinely new
  findings" — literally forbade re-relaying that carryover case, leaving a failed
  fix nowhere to go but a repeat dispatch of the same approach. The elevated
  implementer reads the prior fix commits and dispositions, names each finding's
  root cause before editing, and must try a different in-plan approach or
  escalate for a plan update rather than shrink the fix to fit.
- **`ws/tickets.verify` and `ws/git.commit` run a cross-file ticket-graph pass.**
  For a verified ticket with a `parent:`, the response shows the ancestor chain,
  each ancestor's status, and an `ACTION:` closure nudge when every child of an
  ancestor epic is closed or only `idea/` children remain; an already-closed
  ancestor gets a plain `NOTE:`. Four cross-reference checks catch problems no
  guardrail previously looked for — a `parent:` or `related:` stem resolving to
  nothing, a `parent:` cycle, and a `parent:` pointing at a non-`epic` ticket —
  reported as `FIX:` or `CHECK:` and capped at 5 per ticket. None of it blocks
  the commit; on the commit path a `FIX:` also carries a
  `git commit --amend --no-edit` recipe.
- **`ws/git.commit` surfaces ticket-verify's non-blocking warnings instead of
  discarding them.** Soft warnings such as `unresolved-phases` on a `.done`
  commit or `spec-address` on a `ready/` ticket were computed correctly but only
  ever visible through a direct `ws/tickets.verify` call. They now render as an
  `advisories:` block in the text-mode response; `OK` and JSON-mode output are
  unchanged, matching the todo-reinjection precedent.
- **Spec tools flag files that still carry a legacy `🚧` planned marker.**
  `specs.list`, `specs.find`, `specs.status`, and `project_tree` emit a
  non-blocking advisory naming the live tickets that reference the marked spec,
  or reporting the marker as orphaned when no ticket claims it. The bootstrap
  migration checklist (v0045 canonical, v0006 wsflow) gained a matching item that
  walks a downstream project off the convention, covering all three marker forms
  the detector recognizes: any heading level, any alphabetic callout keyword with
  no required bracket suffix, and the `features:` list form.

### Changed
- **Spec entries describe only implemented, verified behavior; the `🚧 Planned`
  marker convention is retired.** A spec entry could previously carry a marker
  meaning "not built yet", which `lead-write-spec`, `lead-update-spec`, and
  `lead-forge-spec` all wrote and read, and which gated a ticket-side
  `judge: contract-first-spec` check. The playbook instructions, the
  contract-first gate, and the `## 🚧 Markers` convention section are all gone,
  replaced by an unqualified implemented-behavior-only rule with one named
  exception: a `> [!note] Implementation Gap · YYYY-MM-DD` callout for a
  known-but-unscheduled gap, which a spec-hygiene pass must leave in place rather
  than delete. The ticket skeleton's `## Spec Impact` bullet no longer asks for a
  `Contract-first spec: yes|no` declaration. `lead-forge-spec`'s wrap-up now
  reads a persistent ambiguous-item record written earlier in the run, and can
  reconstruct that list from a resumed session's own progress markers — labelled
  as a reconstruction rather than a complete count.
- **Sage-review enforcement for `ready/` landings happens at commit time, not at
  the move.** `tickets.move` and `tickets.create_empty` no longer hard-block a
  `ready/` landing with an unreviewed or blocked posture; they persist the
  posture and warn. `ws/git.commit` is the single hard enforcement point, and
  `lead-write-ticket` runs its Sage Review Gate before Commit so the posture
  change lands in the same commit as the rest of the promotion instead of forcing
  a second one. Non-`ready/` upward moves that leave a required stage blocked
  still hard-reject.
- **`tickets.sage_gate` no longer proposes or fires a commit for the posture it
  writes.** Its decline path could auto-commit the flip as its own
  `chore(sage): ...` commit — and, in an interim form, surface it as ready-to-
  paste commit metadata — either of which could swallow co-located real edits or
  collide with the caller's own commit step. The posture is written and left
  uncommitted for the caller's ordinary commit to pick up.
- **Open Decision Queue items show the actual decision text instead of a
  placeholder label.** An item could be created with a vague subject while the
  real text sat in a secondary description field that some hosts never render,
  silently losing the decision. An item's visible text must now be the decision
  itself on every host variant, and asking an item restates its full text with a
  one-line roll-up of the rest.
- **`specs.*` and `project_tree` no longer serialize planned-marker context.**
  `specs.list` dropped its `marker:` line, `specs.status` dropped the
  `# <marker context>` suffix on location lines, `project_tree` dropped its
  WIP/planned spec stats, and `format=json` no longer emits `marker_contexts` /
  `marker_context`. The underlying data still feeds `specs.find` match ranking
  internally; only the serialized surface changed.
- **`install.sh` now sets `CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000` and
  `CLAUDE_FORK_SUBAGENT=1`** alongside the existing `ENABLE_TOOL_SEARCH` in the
  project-level `settings.json` it writes.

### Fixed
- **The review-relay loop stops after its budgeted number of cycles.** The cap
  was live in spec but never reached the generated Instruction text a lead
  follows, so it silently vanished and reviews could relay indefinitely. The
  Instruction now states an explicit per-slice budget — 3 cycles for partitioned
  and fallback reviews, 2 for single-reviewer runs, none for lead-only — and the
  final budgeted cycle completes the run, carrying any unresolved findings into
  the completion report instead of leaving the loop open.
- **Mid-procedure skills point at the MCP repair skill instead of dead-ending.**
  `lead-bootstrap`, `lead-forge-spec`, `lead-review`, `lead-ship`,
  `lead-write-ticket`, and `lead-implement` — in both the `ws` and `wsflow` skill
  trees — used to end a dropped MCP connection with a bare "stop and report that
  blocker". Each now names `mcp-server-repair` so the agent can reconnect and
  resume.
- **The warning for a blocked `ready/` landing names an escape that can actually
  clear it.** It pointed at `ws/tickets.sage_gate`, which returns immediately on
  a `blocked` posture and can never resolve one. It now says to address the
  blocker and call `ws/tickets.sage_stamp` with a non-`block` verdict.
- **`tickets.move` again reports which fields changed when a rejected move
  already wrote a partial mutation.** On the `idea/` → `todo/` path that posture
  rules still block, migrated sage-review frontmatter is written before the move
  is rejected, so callers got a rejection with no sign the file had already
  changed. A prior cleanup removed the notice on the mistaken belief that no
  write-then-reject window remained.

### Removed
- **The `lead-sprint` and `lead-salvage` skills, and their `enter.sprint` /
  `enter.salvage` MCP tools, are gone with no deprecation period.** Sprint's
  documentation wrap-up step is superseded by `lead-backfill-docs` above, but
  salvage's recovery workflow has no successor route. A pinned install that
  called `enter.sprint` or `enter.salvage`, or referenced either skill directly,
  will find them missing after this release.
- **`lead-skill-authoring` is no longer invocable and is now a plain reference
  document.** It was only ever this project's own upstream maintenance guide, and
  shipping it as a distributable skill was a packaging mistake. The content is
  unchanged at `ai-docs/ref/skill-authoring.md`.

## v0.36.16 - 2026-07-26

### Changed
- **Ticket completeness review now uses the same severity and verdict vocabulary
  as design review.** The lead reads both verdicts in one flow, and the two
  prompts previously gave the same field names different meanings. Severity is
  anchored to a consequence — a fresh reader who cannot start or cannot tell when
  a phase is done (`critical`), one who proceeds on a wrong reading of the goal,
  approach, or acceptance criteria (`important`), one who proceeds correctly with
  avoidable friction (`minor`) — and the verdict carries the same required
  `sufficiency` line. The rows are adapted rather than copied: completeness reads
  only the ticket file, so a design-review row about contradicting a live spec
  entry would be unreachable in its lane.
- **`resolution: missing` now means the same thing in both ticket reviewers.**
  Completeness read "requires user input or authoring work", which routed any
  writing gap to `block`, while its own scope-boundary check already defined the
  in-lane meaning as a design-shaped gap the reviewer must not fill. Both prompts
  now state it as a user decision or design input the lead or implementer cannot
  supply. The `concern` threshold also drops the same tautology fixed in
  `v0.36.15`.

## v0.36.15 - 2026-07-26

### Changed
- **Ticket design review grades severity by consequence instead of by how much
  the ticket omits.** `critical`, `important`, and `minor` were undefined
  vocabulary — the only defined field was `resolution` — so severity was
  calibrated from the reviewing model's own priors, and the same prompt read
  markedly harsher under a stronger model. Each level is now anchored to a
  nameable outcome for an implementer following the ticket as written: builds
  something that cannot work or contradicts a live contract (`critical`), builds
  the wrong thing or installs a rule that cannot fire (`important`), builds the
  right thing less cleanly (`minor`). An omission is `minor` unless the reviewer
  can name the wrong result it produces.
- **The design verdict now carries a required `sufficiency` line.** The reviewer
  was already asked whether a competent implementer can execute the unfinished
  phases, but the output format had nowhere to record the answer, so the verdict
  was back-derived from the issue list and `pass` existed only as an empty list.
  `sufficiency` states that judgment directly, above the issues. The `concern`
  threshold also drops a tautology: `resolution: missing` already routes to
  `block`, so every remaining `important` issue was autonomous by construction.
- **Design-review doctrine bounds what gets reported.** An unimplemented design
  has unbounded surface — every added sentence creates new implications that can
  themselves be called under-specified — so findings cannot run dry the way they
  do against a finite diff. The doctrine now names the finite resource (a fresh
  implementer's ability to proceed without the ticket's author) and directs the
  reviewer to report what blocks execution rather than everything that could be
  specified further.

## v0.36.14 - 2026-07-26

### Changed
- **Ticket design review now reads the spec area a ticket targets, not only
  `spec:` frontmatter.** A ticket that addresses specs through `## Spec Impact`
  previously got no spec read at all, so the reviewer judged spec coherence
  without opening the spec. The reviewer also lists `ready/` tickets and compares
  their `## Spec Impact` sections for spec-territory conflicts, reporting one only
  when two tickets would define the same behavior differently or one landing would
  invalidate the contract the other states — shared spec files alone are not a
  finding.
- **Ticket conventions gain a `## Content` section**: record the judgment
  implementation cannot re-derive — the choice among workable alternatives, why
  the others lost, agreed interfaces, and what the ticket deliberately leaves
  untouched — and point at code by the search that finds it rather than by
  surveyed coordinates, since enumerated symbol and line lists drift silently
  while the compiler and test suite re-derive them for free.
- **Epic ticket bodies route implementation detail and deliberation out.** Two
  operational rules in the Epic Tickets section: implementation detail moves to an
  implementation child ticket, and deliberation that outgrows a settled decision
  line moves to a `research` ticket. The epic body carries scope, cross-child
  invariants, closure conditions, and settled decisions only.

`0.36.13` was a dev-merge bump and was never tagged; this entry covers the whole
`v0.36.12..v0.36.14` range.

## v0.36.12 - 2026-07-26

### Added
- **ws-cli / wsflow-cli MCP-independent CLI fallback surface.** New `tools` and
  `call` subcommands on the `ws-mcp` runtime, `bin/ws-cli` and `bin/wsflow-cli`
  shims (with `.cmd` companions), and a self-contained `mcp-server-repair` skill
  (`/ws:mcp-server-repair`, `/wsflow:mcp-server-repair`) that keeps work moving
  through the CLI when the MCP server drops, plus a one-line repair pointer on the
  lead front doors.
- **`lead-goal-fan-out-step`** entry skill for batch-parallel worktree fan-out of
  mutually independent ready tickets, mirrored into the wsflow package, with
  goal-step transclusion wiring.
- **`session.note` MCP tool** for lead-to-child session annotations.
- `lead-goal-step` selection now prefers a ticket already in progress before
  untouched ones.

### Changed
- **`tickets.sage_stamp` is stage-only**: it records the sage posture without
  staging or committing, so it no longer swallows concurrent ticket edits or
  pollutes the next commit; callers commit the stamp explicitly.
- **Retired the `_index.md` Ticket Focus section** and its writer/reader/cleaner
  machinery from specs, shipped playbooks, and bootstrap.

### Fixed
- **Launcher no longer drops the MCP server with a -32000 error on local dev
  patch bumps.** Under an active `.local-devenv-runtime` marker the runtime
  version gate is relaxed from exact-patch to same-minor match, so a source build
  that runs ahead of the installed snapshot is accepted; released/downloaded
  runtimes keep the strict exact-match gate.
- **`git.commit`** no longer runs the commit-gate verifier against the vanished
  side of a staged ticket rename/deletion, and its `ai_context` handling stops
  pre-filtering blank entries while reporting an accurate empty/all-blank/absent
  diagnostic.

## v0.36.1 - 2026-07-24

### Fixed
- Windows release CI (`go test ./...`) now passes. Two tests failed only on
  Windows (both green on Linux, so the release build was never blocked): a
  shipped-doc content assertion broke under a CRLF checkout, and an exec.shell
  cwd assertion missed when the CI runner returned an 8.3 short-name temp path.
  Added a repo-root `.gitattributes` (`* text=auto eol=lf`) so text — including
  the plugin's shipped docs — checks out as LF on every platform, and made the
  temp-path assertion tolerant of 8.3 short names and symlinked temp roots.

## v0.36.0 - 2026-07-24

### Added
- **`tickets.verify` MCP tool** and a `git.commit` ticket-verify gate: commits
  that touch a ticket now run guardrail checks (stem/status-dir agreement,
  frontmatter and phase/Result heading well-formedness, ready-landing sage
  posture, close date-field presence) and are vetoed when a hard check fails;
  spec-address and unresolved-phase issues warn without blocking.
- Ticket-system concept documentation folded into the workflow manual, giving a
  single grounding for status/epic/workset/phase vocabulary.
- **Windows MCP process-lifecycle hardening**: the `ws-mcp serve` process now
  self-terminates when its parent launcher dies (preventing an orphaned server
  from holding a stale `state.sqlite` lock that broke the next connection), the
  launcher records a `last-abnormal-exit` breadcrumb on a non-zero Windows child
  exit, and the release CI workflow now runs `go test ./...` on Windows.

### Changed
- Renamed the MCP tools `tickets.create` -> **`tickets.create_empty`** and
  `tickets.sage_record` -> **`tickets.sage_stamp`**, and gated `sage_stamp`
  lead-only so reviewers can no longer write ticket frontmatter directly
  (preserving the single-writer property). This is a minor bump because it
  changes caller-visible MCP tool names and adds a new tool.
- Removed the `fork` delegation path; `lead-prefer-subagent` is reshaped around
  fresh-spawn plus a central capability whitelist.
- SQLite discipline: `journal_mode=WAL` is now re-asserted on every store open,
  and point reads and multi-row scans retry on transient `SQLITE_BUSY`/
  `SQLITE_LOCKED` (previously only writes retried).

### Fixed
- **Windows mid-session MCP disconnect** root cause: a panic in a request
  handler goroutine now recovers into a JSON-RPC error and persists a crash
  trace instead of tearing down the whole `serve` process. An always-on crash
  sink and lifecycle log were added so future disconnects leave evidence.
- `tickets.close` now soft-warns when phases are unresolved.

## v0.35.0 - 2026-07-23

### Changed
- Renamed the `lead-drain-ready-queue` skill to **`lead-goal-step`** and
  repositioned its identity: advancing a goal-pursuit run by one step is now the
  primary framing (with `ready/` as the sole progress gate), and the
  single-cycle drain becomes a degenerate case rather than the headline. The
  `/goal` Stop-hook and playbook resolvers now reference the new name; the skill
  is substitution-mirrored, so `agents-plugin-wsflow` and the skills manifest
  were regenerated in lockstep. This is a minor bump because it replaces a
  caller-visible Codex skill entry point.
- Added body posture to `lead-goal-step`: lead ticket-curation authority, a
  blocked-progress **clean-conclusion** term distinct from a hard-gate pause
  (with anti-abuse guards — it never runs the empty-queue merge-approval flow,
  re-reads the advanceable-now selector, and records before yielding), and
  bounded autonomous in-scope bug capture that files a `ready/` ticket for a
  later loop unless the fix was explicitly deferred.

## v0.34.5 - 2026-07-22

### Changed
- `lead-drain-ready-queue` now opens with a **goal-run posture**: during a goal
  run (current branch `goal/*`, or an active `/goal` Stop-hook reminder), the
  lead assumes the user is away and resolves reversible, local decisions on its
  own stated recommendation — recording the choice in one line and continuing
  without waiting for confirmation — instead of stalling on trivial "which
  option?" prompts. It stops only for genuinely critical decisions (irreversible
  or destructive actions, public-API/cross-module scope expansion, unresolved
  binding decisions, or any AGENTS.md "Always ask" item); the confirmation
  threshold rises but the hard gates never dissolve. Placed as a top-of-body
  lead-in so the `/goal` Stop-hook re-surfaces it every turn, covering all
  downstream sub-skills rather than one gate. Ships the authoritative posture
  only; decision-point reinforcement tips and spec documentation are tracked in
  `260722-feat-goal-run-autonomy-posture`.

## v0.34.3 - 2026-07-22

### Fixed
- `lead-drain-ready-queue` goal-branch staging now encodes the fork parent in
  the branch name (`goal/<parent>/<slug>`, was `goal/<slug>`) and derives the
  completion merge target from it, instead of hardcoding a merge into `main`.
  Previously a `/goal` run staged on a non-main base branch (e.g. a feature
  stream) would silently complete-merge into `main`, bypassing the branch it was
  actually forked from. Creation captures the current branch as the parent and
  guards against a detached HEAD; completion parses the parent by splitting on
  the last `/`, and an old-format `goal/<slug>` branch (no parent segment) falls
  back to `main` so in-flight pre-upgrade runs are unaffected. The merge stays
  user-approval-gated and never pushes.

## v0.34.2 - 2026-07-22

### Added
- `mental-model-updater` now emits a flag-only `## Spec Coverage Gaps` audit
  block: for each affected domain that references a spec stem and had source
  changes in the scoped range whose spec entry was not touched in the spec diff,
  it flags the domain for lead review. It never authors or edits spec content —
  caller-visibility and spec-impact judgment stay with the lead's spec pass. The
  heuristic is deliberately coarse (over-flags), mirroring the `## Stale Rules`
  "flag, do not edit, user resolves" contract.

## v0.34.1 - 2026-07-22

### Added
- `playbook.render` now returns config-resolved native spawn bindings —
  `recommended-model`, and optional `recommended-reasoning-effort` — after the
  existing `recommended-tier` line, so native (Codex) subagent dispatch can bind
  a model and reasoning effort. Resolution failures or empty values omit only
  the affected optional line; the path and `recommended-tier` are unaffected.
  `playbook.print` stays tier-only.

### Changed
- Codex-native delegation guidance maps `recommended-model` to
  `spawn_agent.model` and `recommended-reasoning-effort` to
  `spawn_agent.reasoning_effort` (`fork_turns: "none"` for self-contained
  rendered prompts), and delegate prompt bodies no longer echo their own model
  alias.

## v0.34.0 - 2026-07-21

### Added
- `tickets.checklist` MCP tool — returns a ticket-authoring phase's checklist
  (`intent`/`content` × category) as structured data for installing into a
  single `todo.append`, replacing the skippable static prose in
  `lead-write-ticket`.
- `tickets.sage_gate` and `tickets.sage_record` MCP tools — the sage-review
  gate now resolves posture (legacy `sage-review:` migration, config fallback,
  category×stage matrix, standalone/combined mode) and records verdicts
  (design/completeness aggregation, `resolution: missing` escalation, Go-owned
  Blocked-section rendering, canonical commit) in the runtime instead of ~235
  lines of playbook prose. Neither tool spawns reviewers.

### Changed
- `lead-write-ticket` diet: the sage-review state machine, the three Blocked
  Section Templates, and the phase checklists are ported to the tools above;
  the playbook drops ~268 lines per distribution while keeping doctrine intact.
- `lead-verify-discussion` gains a design-verification escalation step; the
  standalone `lead-verify-design` skill is removed and its references updated.
- Convention text disambiguates relative "user" in worker-readable prose.

### Fixed
- Spec: removed a duplicate anchor in the verify-discussion escalation coverage.

## v0.33.14 - 2026-07-14

### Added
- Four render-resolved playbook template vars — `{{.SmallTierModel}}`,
  `{{.MediumTierModel}}`, `{{.LargeTierModel}}`, `{{.XLargeTierModel}}` — that
  expand to the configured model for each fixed tier under the active harness.
  They are auto-injected via `wsrsrc.ImplicitVariableNames` (no frontmatter
  declaration required) and resolve through the same config seam as
  `{{.RoleModel}}`, falling back to `the <tier>-tier model` when unset. The
  `lead-workflow-manual` Scoped Exploration guidance is the first consumer.

### Changed
- Remap the codex tier defaults to the gpt-5.6 family: `small` =
  `gpt-5.6-luna` (effort `medium`), `medium` = `gpt-5.6-terra` (effort `high`),
  `large` = `gpt-5.6-sol` (effort `high`), `xlarge` = `gpt-5.6-sol` (effort
  `xhigh`). Claude tier defaults (haiku/sonnet/opus) are unchanged.

## v0.33.13 - 2026-07-14

### Changed
- Make the `impl/<stem>` 15-character branch-slug limit advisory instead of
  enforced. `implementTargetBranchName` no longer hard-truncates the scope slug
  to 15 characters; it only trims a trailing `-`, so the `<=15` guidance is now
  a recommendation. The helper stays the single shared branch-name constructor,
  so `enter.implement` observation and branch-plan derivation still agree and
  the `impl/*` auto-delete gate is unaffected. Spec and mental-model wording
  softened to match.

## v0.33.12 - 2026-07-13

### Fixed
- Regenerate `agents-plugin/skills/manifest.json` after the
  `lead-drain-ready-queue` slug hotfix left it stale, which failed
  `TestSkillsManifestDriftIsVisible` in CI on the v0.33.11 tag. v0.33.11 was
  abandoned (never had a published GitHub release) in favor of this release.

## v0.33.11 - 2026-07-13

### Fixed
- Remove the dead `status-report` `NEXT:` route from `enter.proceed`
  (self-designed during the low-ceremony diet, never wired to a routing
  condition).
- Generate an arbitrary random word-word-word slug for goal-staging branches
  in `lead-drain-ready-queue` instead of deriving it from goal text, since a
  goal-text-derived slug can collide across independent concurrent goal runs
  of the same command (Git branches are shared across worktrees of one
  repository).
- Surface a loud, explicit partial-mutation notice in `tickets.move`'s tool
  result whenever its self-healing sage-review frontmatter write persists
  before the move blocks or fails, so a retrying caller cannot mistake a
  blocked move for an unchanged file.

## v0.33.10 - 2026-07-13

### Fixed
- Handle MCP base-protocol `ping` requests with an empty success result while
  preserving raw JSON-RPC request IDs and keeping `ping` out of `tools/list`;
  focused regression coverage and a native-Windows Claude Code 2.1.207 idle
  A/B confirmed the original launcher/runtime processes and connection survive
  beyond the prior approximately 15-minute termination window.

## v0.33.9 - 2026-07-13

### Changed
- Add an explicit, safety-gated low-ceremony implementation path that may keep
  a real current branch for a bounded inline direct edit while preserving
  review, verification, documentation-skip rationale, commit, and no-push
  requirements; labels such as `hotfix` or `tweak` do not activate it alone.
- Make implementation ceremony proportional: bounded work may remain
  ticketless, delegated planning accepts ticket or inline authority, automatic
  review uses one full-scope reviewer unless multiple risk partitions are
  warranted, and unchanged full-suite evidence remains reusable across
  documentation-only closeout.
- Reduce repeated lead, planner, reviewer, and MCP instruction text while
  keeping the authoritative planner Prep contract and workflow diagnostics.

### Fixed
- Reject unborn Git state from current-branch completion, pass complete active
  and inactive planner authority fields, and route generic single review
  through the delegate-grade reviewer wrapper with its shared review contract.

## v0.33.8 - 2026-07-10

### Fixed
- Export the packaged plugin `skills/` tree through `WS_SKILLS_ROOT` from both
  launchers before handing off to runtimes installed under
  `.runtime/<platform>/`, restoring maximum-delegation workflow-manual loading
  for downstream ws and wsflow installations. Add helper and launcher-main
  regression coverage.

## v0.33.7 - 2026-07-10

### Fixed
- Align `lead-write-ticket`'s `tickets.create` example with the public MCP
  schema (`stem` and `initial_state`), regenerate the wsflow rsrc mirror and
  manifests, and add a rendered-playbook regression assertion.

## v0.33.6 - 2026-07-10

### Changed
- `lead-proceed`'s playbook now skips reloading `workflow_manual` when it was
  already loaded this session and no compaction or continuation occurred
  since, calling only `git.status` on repeat invocations (mirrors
  `lead-sprint`'s existing conditional-reload pattern); addresses the
  per-phase reload cost in high-frequency callers like goal-pursue loops.
- `lead-discuss`/`lead-sprint` SKILL.md (both `agents-plugin` and
  `agents-plugin-wsflow`) now say `session_key: <your key, omit if fresh>`
  for the `playbook.print` call, removing the apparent contradiction with a
  fresh session having no key yet.
- `lead-skill-authoring`'s Downstream Consistency Sweep playbook step now
  points to `ai-docs/ref/wsflow-mirroring.md` before any edit that touches a
  mirrored `agents-plugin-wsflow` rsrc surface, closing a gap where its
  generated byte-identical `rsrc/` mirror could be hand-edited by mistake.

### Fixed
- Regenerated `agents-plugin/skills/manifest.json` and both packages'
  `rsrc/manifest.json`, and fixed a stale hardcoded regex in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` left behind by the
  v0.33.5 session-key wording fix.

## v0.33.5 - 2026-07-08

### Changed
- `git.commit`'s text-mode response now appends a `tip: preserve this session
  key: <key> during compaction` trailer after the todo-reinjection block,
  repeating `workflow_manual`'s session-key reminder on a high-frequency,
  lead-scoped call to improve the odds a compaction summary carries the key
  forward.

## v0.33.4 - 2026-07-07

### Fixed
- Regenerate `agents-plugin/skills/manifest.json`, which had drifted from
  `lead-drain-ready-queue/SKILL.md`'s content (v0.33.3's release CI caught
  this before any release assets were published; v0.33.3 stays tagged but
  has no corresponding GitHub release).

## v0.33.3 - 2026-07-07

### Added
- `impl/<stem>` branch naming convention (15-character stem cap) for
  implementation branches, replacing the legacy `implement/*` prefix; branch
  rename now defaults to allowed unless the caller explicitly asks to keep
  the current branch name.
- Branch Cleanup auto-delete for merged `impl/*` branches without asking;
  non-`impl/*` branches (including legacy `implement/*`) still require
  explicit user approval before deletion.
- `policy.branch.merge_confirm` fact on `enter.implement`, letting a caller
  (e.g. a goal-driven queue drain) opt a branch into merging without a
  final user confirmation gate; default posture remains ask.
- `lead-drain-ready-queue` goal-branch-staging awareness: detects an active
  goal context, stages a shared `goal/<slug>` branch across sequentially
  drained tickets with `merge_confirm: skip` handed to each implementation,
  and performs its own single confirmed merge into the target branch at the
  end of the drain.
- `lead-forge-spec`'s per-item classification pass now proceeds
  autonomously instead of asking per item, embedding an inline
  `<!-- AMBIGUOUS: <reason> -->` marker for genuinely unclear cases and
  summarizing them in the wrap-up report; the wrap-up can now optionally
  chain directly into `lead-forge-mental-model`.
- Session-bootstrap staleness warning and `bootstrap_alarm` config item;
  live doc-coverage session-bootstrap warning and matching mute item.
- Sage Review Gate now branches by ticket landing status
  (design/completeness split, staged), including a scope-boundary check
  added to the completeness reviewer checklist.

### Fixed
- Honor `policy.branch.merge_confirm=skip` in the actually-installed
  `enter.implement` todo instructions (final-action gate and merge-step
  instructions) — previously the skip signal reached only the verdict's
  rendered prose, not the runtime todo text the lead executes, silently
  defeating goal-branch-staging's unattended merge behavior.
- Default `policy.branch.allow_rename` to `yes` when unset, matching the
  documented default-allow posture.
- Align the stale `merge_target` warning and docs with the `impl/*` rename.
- Fail-safe silence and tighter test coverage for the bootstrap-alarm
  warning path.
- Stop `echo -e` from reinterpreting dynamic content as shell escapes.
- Fix a duplicate `sage-review-gate` spec anchor introduced by the staged
  design/completeness split.

## v0.33.0 - 2026-07-03

### Added
- `lead-drain-ready-queue` entry skill: delegates ticket selection to a
  light-tier Explore-style subagent that lists `ai-docs/tickets/ready/`,
  prefers a candidate named as a prerequisite in another ready ticket's
  `related:`/`parent:` frontmatter (falling back to oldest-first FIFO), and
  returns exactly one ticket path — then hands that path to `lead-proceed`
  as an explicit target. A minimal, delegate-only shim intended for
  standing `/goal` directives that repeatedly drain the `ready/` backlog;
  mirrored byte-identically into `agents-plugin-wsflow`.

## v0.32.4 - 2026-07-03

### Changed
- Inline `lead-prefer-subagent` and `lead-verify-discussion` bodies directly
  into their `SKILL.md` files instead of routing through the `rsrc`
  playbook-print indirection; `agents-plugin-wsflow` mirrors both via a new
  substitution-mirrored skill generation mechanism (literal `ws:`/`ws/` ->
  `wsflow:`/`wsflow/` namespace substitution over a curated, guarded skill
  list).
- Drop the unconditional `delegates:true` continuity-tip/mercenary-path
  paragraph from `lead-verify-discussion`, since that skill's delegation is
  conditional ("when investigation is useful"), not guaranteed.
- Flip the `sage_review` builtin default from off to `auto` (required
  posture) for `tickets.create`/`tickets.move`, so downstream sessions get a
  review gate by default unless explicitly overridden at project, session, or
  global scope.

### Fixed
- Fix the substitution-mirrored skill generator (`GenerateWsflowSkillBody`)
  matching `ws:`/`ws/` as raw substrings, which could corrupt unrelated words
  containing those characters as a tail (e.g. "shows:" -> "showsflow:",
  "draws/" -> "drawsflow/"); anchor both patterns to a left-side word
  boundary instead.
- Fix a stale test asserting the pre-inlining `playbook.print` shim contract
  for `lead-verify-discussion` after its rsrc tree was deleted.

## v0.32.0 - 2026-07-02

### Added
- Add `agenda.list` and an `all: true` fast path on `agenda.clear`, so leads
  can enumerate current agenda entries and clear them all in one call.
- Add the lead-only `workflow_state` MCP tool: a session-state-only view
  (agenda/todos) that avoids re-dumping the full `workflow_manual` output.

### Changed
- Redefine `config.prompt.unset` and `config.workflow_prefer_subagent` so
  "unset" consistently means reset-to-builtin-default, never clear-to-empty;
  `config.prompt.unset` gains a `session` scope and
  `config.workflow_prefer_subagent` gains a `reset: true` argument. Explicit
  empty overrides now route through `config.prompt.set` instead.
- Fill the previously thin `Session setup` and `User preferences` sections of
  the shipped `workflow_manual`/`lead-workflow-manual` template with concrete
  guidance (ferrule redundant-mint consequences, compaction-recovery
  reminder, and a non-empty default preference sentence).
- `tickets.move` to `ready` now emits an advisory (non-blocking) warning when
  no spec-addressing signal (`spec:`, `spec-remove:`, or `## Spec Impact`) is
  detected on a non-exempt ticket category.
- `enter_implement` now warns when a supplied `policy.branch.merge_target` is
  ignored because the caller isn't yet on an `implement/*` branch.

## v0.31.1 - 2026-07-02

### Fixed
- Fix `TestBuildCodexInvocationUsesStdinPromptForFirstCall` (`internal/wsagent`)
  to match the `filepath.ToSlash` normalization production already applies to
  `model_instructions_file`, so the test passes on native Windows as well as
  Linux/WSL.
- Fix the shared `initGit` test helper (`internal/mcp`) to set a local git
  identity, so git-commit tests no longer depend on the CI runner or
  developer machine having a global git identity configured.

## v0.31.0 - 2026-07-02

### Added
- Add the Codex-visible playbook surface: `playbook.print` and
  `playbook.render` MCP tools resolve and render lead/delegate playbooks from
  the rsrc tree, replacing self-contained skill prose with a shared
  MCP-served source of truth.
- Add `enter.proceed` and `enter.implement`: deterministic verdict-engine MCP
  tools that resolve routing and implementation dispatch decisions from
  caller-supplied facts, returning a `Next:`/`raw`+`next_instruction` handoff
  instead of leaving verdict computation to playbook prose.
- Add the `workflow_manual` MCP tool and the session-bootstrap (`ferrule`)
  tool for minting and restoring lead session keys per working root,
  including session-parent lineage and session-children enumeration.
- Add `tickets.create`, `tickets.close`, `tickets.move`, and
  `tickets.template` MCP tools for ticket lifecycle and skeleton generation,
  and a `sage_review` posture surface on tickets.
- Add a session-state machine (agenda/enter/todo primitives) backing
  installed-todo runbooks for `lead-implement` and `lead-proceed`.
- Add a layered workflow config surface (`config.prompt`, `config.tuning`,
  `config.workflow_prefer_mercenary`, `config.workflow_prefer_subagent`) for
  prompt overrides and delegation-posture tuning.

### Changed
- Drop the `ws.` prefix from all MCP tool names; every tool is now called by
  its bare name (e.g. `ferrule`, `project_tree`, `git.commit`).
- Diet the `lead-proceed`, `lead-discuss`, `lead-implement`,
  `lead-write-ticket`, and `lead-workflow-manual` playbooks: remove
  MCP-schema-restatement prose and legacy manual verdict/routing blocks now
  computed by `enter.proceed`/`enter.implement`.
- Remove the subquery runtime, its MCP tool, and its CLI subcommand.
- Remove setup tool dispatch; reshape the agent-backed API tools surface.
- Rename the docs-discovery pre-invocation agent `project-survey` to
  `reference-discovery`.

## v0.30.0 - 2026-05-29

### Added
- Add the wsflow-only `prompt.render` MCP tool: it loads a bundled delegate
  prompt by stem, applies render-time `ws/` -> `wsflow/` namespace substitution,
  injects caller context, and returns a rendered prompt path. Visibility mirrors
  the agentless hidden-tool gate (advertised only in wsflow; hidden from full
  ws), enforced at `callTool`, `toolAllowed`, and the capability surface.

### Changed
- Converge wsflow `lead-implement` onto the unified ws spine and absorb the
  separate wsflow `lead-edit` skill (removed). The wsflow Edit stage is now lead
  direct edits plus a lead-discretion scoped native subagent, dispatched through
  `prompt.render` for the five render-eligible prompts; ws `lead-implement` and
  the ws named-agent path are unchanged.
- Rename the docs-only pre-invocation survey agent `project-survey` to
  `reference-discovery` to end the selection-layer confusion with
  `plan-populator-survey` (docs discovery vs source reference map); skills,
  runtime prompt bundle, and docs updated accordingly.

## v0.29.4 - 2026-05-26

### Changed
- Make exec MCP lifecycle and raw-reader responses readable text instead of
  JSON-serialized text payloads.
- Add `exec.result(timeout_seconds)` waiting while preserving non-blocking
  running guidance when the timeout is omitted or zero.
- Shorten newly generated exec job keys while preserving legacy key lookup.

### Fixed
- Keep JSON-shaped command stdout unescaped in `exec.result` raw output sections
  and cover readable raw-reader responses with MCP tests.

## v0.29.3 - 2026-05-25

### Added
- Add ws dashboard document viewing/editing, Git worktree/toolbar controls,
  Activity Console improvements, and linked-server gateway flows.

### Changed
- Make `lead-implement` emit an Implementation Verdict before starting work.
- Refine the dashboard server-scoped operation forwarding plan around
  identity-first routing and separate HTTP, SSE, and WebSocket phases.

### Fixed
- Hide stale public `root` schema exposure from `subquery` and keep actor-scoped
  subquery follow-up behavior aligned with named-agent tools.
- Shorten root-omitted actor setup gate errors to compact recovery guidance.
- Make local runtime marker handling explicit so plugin-cache dogfood uses the
  intended local runtime.

## v0.29.2 - 2026-05-25

### Changed
- Preserve named-agent instance history while reconciling active instance
  cleanup through the SQLite-backed registry.
- Persist exec job metadata and runtime migration metadata in `wsstore`.

### Fixed
- Shorten `ws.setup` actor recovery tokens, keep legacy recovery compatible, and
  hide setup-only `format` schema attention while preserving hidden JSON calls.
- Force local Codex plugin-cache dogfood paths to build or install the intended
  local runtime before falling back to compatible cache reuse.
- Harden exec raw-payload errors, actor-scoped agent retention, and cache-wide
  actor token collision coverage.

## v0.29.1 - 2026-05-24

### Fixed
- Avoid same-process SQLite write contention in `wsstore` when multiple handles
  open one state database, including CI release runners that hit `SQLITE_BUSY`
  during concurrent actor setup metadata writes.

## v0.29.0 - 2026-05-24

### Added
- Add durable exec job MCP tools with persisted stdout/stderr readers and
  lifecycle controls.
- Add SQLite-backed ws state-store foundations for actor setup metadata and
  future runtime metadata pruning.

### Changed
- Replace ticket queue project memory with ticket focus, add workset ticket
  categorization, and add design verification workflow guidance.
- Require implementation branch isolation in `lead-implement` and refine merge
  guidance for single-commit, multi-commit, and noisy implementation branches.
- Split static ws MCP reference ownership across specs and mental models while
  keeping the reference document focused on operations.

### Fixed
- Fence `ws.setup` requests so batched setup-then-call sequences observe the
  updated session and actor state.
- Require absolute setup roots instead of the ambiguous `<cwd>` placeholder.
- Harden exec job lifecycle edges and bounded text-reader behavior.

## v0.28.1 - 2026-05-23

### Fixed
- Harden plugin-managed MCP startup when concurrent launcher repair attempts
  share a runtime directory, including contract-addressed cache binaries,
  process-unique temporary files, and compatible-target reuse after replace
  failure.

## v0.28.0 - 2026-05-23

### Changed
- Redefine ready-ticket workflow as spec-addressed readiness while keeping
  planned spec markers only for contract-first exceptions.
- Make `lead-proceed` emit a single `NEXT:` Routing Verdict and stop cleanly on
  route blockers before any implementation handoff.
- Restructure sprint work into episode shells with guarded documentation commits
  and recovery for open edit episodes.
- Simplify `lead-skill-authoring` fresh-reader audits around target-only
  reviewer prompts, lead-side finding classification, and bounded audit loops.
- Require `lead-discuss` to stress-test premises, trade-offs, and failure modes
  before endorsing a direction.

### Fixed
- Hide `root` from public and raw `agents.*` MCP schemas while preserving
  explicit-root dispatch compatibility.
- Clarify fresh-reader and doc-closeout compaction wording across workflow
  guidance.
- Align the main plugin proceed dispatch contract test with the current Routing
  Verdict wording.

## v0.27.0 - 2026-05-21

### Added
- Add explicit wsflow mirroring follow-up tracking for the remaining
  `lead-implement` / `lead-edit` divergence.
- Add libws harness research and MVP planning tickets for the future
  JSONL-first agent run substrate.

### Changed
- Restructure `lead-implement` into the canonical unified implementation
  spine, absorbing full ws `lead-edit` and `lead-write-code` behavior into
  direct and delegated modes with one review stage.
- Remove same-actor carry blocks from lead skill handoffs; skill-to-skill
  transitions now rely on the shared active conversation.
- Make `lead-proceed` route-only for implementation handoff, with lead-owned
  ticket freshness and no pre-application of `lead-implement` judges.
- Add fresh-reader audit and downstream consistency sweep gates to
  `lead-skill-authoring`, including responsibility-based handler structure for
  dense `On:` handlers.
- Tighten `lead-write-ticket` recoverability so tickets preserve enough
  settled product, workflow, API, and verification detail for fresh sessions.
- Document wsflow as intentionally `lead-edit`-mediated until the follow-up
  mirroring gap is resolved.

### Fixed
- Preserve add/delete ticket moves in `git.commit` summaries so ticket status
  changes retain merge evidence.
- Support mental-model commit notes in `git.commit`.
- Make wsflow bundle verification tolerate only the documented wsflow-only
  `lead-edit` surface.

## v0.26.8 - 2026-05-19

### Added
- Add `### Mental Model Notes` prompt context so commits can provide primary
  intent for mental-model updater passes.

### Changed
- Make remaining workflow MCP and CLI command surfaces default to compact
  readable text while preserving explicit JSON output and original Git command
  mirror shapes.
- Make broad `specs.find` and `mental_models.find` queries tolerant candidate
  discovery with document-grouped line evidence, bounded snippets, JSON match
  evidence, zero-result guidance, and common convention aliases.
- Teach `lead-proceed` to derive implementation verdicts from the corresponding
  `lead-implement` route contract before handoff, while removing stale skeleton
  routing language from active workflow skills.

### Fixed
- Keep broad documentation query snippets rune-safe for non-ASCII text and avoid
  synthetic line-zero evidence for metadata-only matches.
- Disable Snacks smooth scrolling in the local Neovim configuration so editor
  movement stays responsive.

## v0.26.7 - 2026-05-19

### Added
- Add the ws dashboard Activity Console ticket cascade for the read model, UI
  shell, backend watch stream, frontend live UX, and transcript expansion.
- Add the planned `### Mental Model Notes` commit annotation convention for
  improving mental-model updater context.

### Changed
- Make `lead-proceed` resolve at most one ticket phase per invocation, defaulting
  to the first unfinished phase and stopping for conservative ticket or phase
  slicing when scope is too broad.
- Restructure `lead-write-ticket` into short handlers and preserve agreed
  API/type/event/UI sketches, reviewable phase shape, and forward-compatibility
  guardrails while keeping epics board-level.
- Deprecate normal workflow routing through generated skeleton artifacts; carry
  public contract and integration-test obligations through `lead-write-code`
  briefs instead.
- Strengthen plan-populator survey and research prompts so shortcut, mock-data,
  fallback, and duplicated-glue risks are escalated before implementation.

### Fixed
- Disable Snacks smooth scrolling in the local Neovim configuration so editor
  movement stays responsive.

## v0.26.6 - 2026-05-18

### Added
- Add the ws web dashboard workRoot IO substrate: file listing, read-only file
  panes, daemon-owned terminal sessions, terminal websocket transport,
  workbench integration, Dockview layout ownership, tab polish, and WorkRoot
  Activity projection, badge, and pane.

### Changed
- Teach `lead-write-ticket` to preserve settled local and cross-ticket
  decisions before pruning ticket length, and to review related-ticket
  decisions by default for actionable ticket creation and edits.
- Record workflow follow-up tickets for single-phase `lead-proceed`
  implementation units, non-working contract skeleton semantics,
  pre-implementation survey guardrails, and named-agent empty-result lifecycle
  recovery.

### Fixed
- Harden dashboard resource routing, terminal session lifecycle, terminal
  websocket behavior, Windows terminal portability, editor scroll and terminal
  input fidelity, Dockview dynamic groups, terminal focus retention, and
  WorkRoot Activity live refresh.

## v0.26.5 - 2026-05-16

### Changed
- Teach `lead-write-ticket` to treat cascade wording as related-ticket
  propagation across parent and child ticket graphs, while keeping epics
  board-level and child tickets detail-bearing.
- Default `lead-proceed` to autonomous cohesive phase-slice selection when the
  user does not name phases, keeping `auto-slice` as compatible wording for
  the same policy.
- Add skill-authoring guidance that local shorthand should be represented as a
  trigger example for a general workflow intent.

## v0.26.4 - 2026-05-16

### Added
- Add the authenticated `ws-dashboard` daemon and visible frontend substrate,
  including resource fixtures, workspace discovery, instance event streams,
  dark visual tokens, stable pairing routes, and the first workRoot workbench
  shell.

### Changed
- Require `lead-write-code` briefs to preserve selected-slice binding decisions
  from tickets, including caller-visible contracts, implementation strategy
  decisions, rejected alternatives, and verification expectations.

### Fixed
- Keep ticket-driven `lead-write-code` fit review responsible for detecting
  ticket-to-brief decision loss while preserving the implementer brief-only
  boundary.
- Polish dashboard workbench editor chrome so internal topology concepts remain
  compact and pane bodies stay dominant on desktop and narrow viewports.

## v0.26.3 - 2026-05-15

### Added
- Add the `ws-dashboard/` Rust workspace scaffold for future ws-aware dashboard
  work.
- Add harness-aware named-agent effort configuration through
  `config.agents_tier`, including Codex `model_reasoning_effort` and Claude
  `--effort` runner application.

### Changed
- Rename the blocker checkpoint workflow skill from `lead-can-we-proceed` to
  `lead-check-blockers` across ws and wsflow.
- Require cached plugin runtime binaries to match the plugin patch version,
  while still accepting matching `X.Y.Z-dev` development binaries.

### Fixed
- Stage explicit `git.commit` path roots that refer to renamed or deleted
  files without passing missing pathspecs to `git add`.
- Prevent plugin reinstall from reusing a stale older patch `ws-mcp` binary
  through a loose compatibility stamp.

## v0.26.2 - 2026-05-14

### Changed
- Add advisory `_index.md` health checks to `lead-bootstrap` in ws and wsflow,
  including user-approved cleanup boundaries that avoid automatic semantic
  migration into specs, mental models, tickets, or references.
- Add the optional project reading-map convention for root
  `ai-docs/mental-model.md` and route stable task/topic document maps out of
  `_index.md` without treating them as current feature inventory.
- Tighten workflow skill guidance for discussion verification, carried context,
  and skill readability, including explicit over-alignment and countercase
  checks in `lead-verify-discussion`.
- Broaden the tolerant documentation lookup backlog to cover convention aliases
  such as `spec`, `ticket`, and `mental-model`.

### Fixed
- Ignore Python cache directories generated by local validation runs.

## v0.26.1 - 2026-05-13

### Fixed
- Expose `wsflow` through the repository Claude marketplace metadata for manual
  installation and include both Codex and Claude marketplace package lists in
  release validation.

## v0.26.0 - 2026-05-13

### Added
- Add the `lead-review` workflow skill for config-first PR/MR review routing
  in both ws and wsflow.
- Add the `lead-is-finished-yet` checkpoint skill for separating blocking
  design questions from autonomous hygiene before proceeding.
- Add wsflow marketplace packaging, sprint support, and the curated wsflow skill
  bundle for the agentless derivative plugin.

### Changed
- Let `lead-proceed` refresh warm related tickets from active conversation
  context before implementation routing.
- Support append-only ticket Result editions through `#### Edition` entries and
  include Edition headings in `git.commit` ticket update detection.
- Reset wsflow bootstrap template lineage to a package-local v0001 baseline and
  normalize wsflow subagent wording.
- Remove the unused `lead-exit-session` workflow skill from ws and wsflow.
- Keep `install.sh` from installing or enabling wsflow in Claude while leaving
  wsflow available through Codex marketplace registration.

### Fixed
- Keep wsflow version metadata aligned with ws release bumps.

## v0.25.2 - 2026-05-13

### Fixed
- Stabilize `TestServeStdioGitToolCalls` by separating the state-mutating
  `git.commit` request from read-only git tool calls that run concurrently
  under stdio serving.

## v0.25.1 - 2026-05-13

### Changed
- Let `lead-implement` continue on existing `implement/*` branches, close docs
  before the final merge/continue/stop gate, and keep follow-up work in a new
  implementation slice or sprint after ticket Results are written.
- Define ticket Result hashes as the commit that made the completed phase
  reviewable on its current branch, preserving merge commits as the
  already-merged case.
- Capture wsflow agentless plugin planning tickets for the first internal
  derivative plugin work.

### Fixed
- Remove obsolete clerk wording from forge-spec workflow guidance.

## v0.25.0 - 2026-05-13

### Added
- Add `ws.setup` as the canonical volatile root setup MCP tool, replacing the
  advertised session root compatibility tools.

### Changed
- Route `lead-proceed` by ready-ticket phase slices, including autonomous
  `todo/` to `ready/` promotion before selecting the implementation slice.
- Teach `lead-verify-discussion` to look for already implemented items that can
  be reused or merged before creating duplicate structures.
- Keep the `lead-edit` documentation pipeline owned by callers so
  implementation wrappers can decide when document updates run.

### Fixed
- Hide `ws.setup` outside lead profile surfaces and add delegate-profile
  coverage for setup gating.
- Use Git for Windows for tmux Git status checks on WSL-mounted Windows paths.

## v0.24.0 - 2026-05-13

### Added
- Add `ws-mcp smoke --root <repo>` as a single-process executable smoke command
  for Windows release verification.

### Changed
- Route explicit discussion implementation intent through `lead-proceed`, while
  keeping discussion-to-ticket persistence flows intact.
- Let `lead-write-ticket` satisfy ready-ticket spec coverage by invoking
  `lead-write-spec` before finalizing the ready queue entry.
- Use symbolic bracket labels for `lead-discuss` Intent Frames so response prose
  follows the user's active conversation language more reliably.

### Fixed
- Route discuss-driven ready promotion through `lead-write-ticket` so ready
  spec-gate checks are not bypassed.
- Stabilize MCP Git tests on Windows by disabling `core.autocrlf` in temporary
  test repositories.

## v0.23.5 - 2026-05-13

### Changed
- Make `config.agents_tier` update the explicit or detected harness alias
  mapping, so host-local agent tier overrides can target a different execution
  backend without editing `config.json` directly.

## v0.23.4 - 2026-05-12

### Changed
- Default read-only MCP tool responses to compact LLM-readable text, with
  `format=json` compatibility output where structured consumers need it.
- Move skeleton workflow ownership under `lead-implement`: `lead-proceed` now
  routes implementation-ready work only, while `lead-implement` decides and
  runs optional skeleton work inside the implementation branch lifecycle.
- Split `lead-write-skeleton` output into a lead draft checkpoint commit and a
  final populated skeleton commit, recording only the final hash in ticket
  `skeletons:` metadata.

## v0.23.3 - 2026-05-12

### Changed
- Add cancelled-agent recovery guidance that points no-result timeout recovery
  toward retrying `agents.call` on the same registered agent.
- Hide `agents.recall` from advertised MCP tools, runtime capability metadata,
  and workflow guidance while keeping the compatibility implementation
  available for manual CLI use.

## v0.23.2 - 2026-05-12

### Fixed
- Run Gemini PowerShell shims directly on Windows so backend cancellation and
  Windows release smoke tests do not stall behind `.cmd` wrapper processes.

## v0.23.1 - 2026-05-12

### Added
- Add Gemini named-agent backend support through the existing `agents.*`
  surfaces, including stream-json parsing, session resume, diagnostics, and
  model inference.

### Changed
- Route API documentation helper agents through harness-aware `light` and
  `core` aliases instead of pinning Codex.
- Add validation checkpoint guidance to discussion workflows.

### Fixed
- Prevent model alias overrides from creating cross-backend backend/model pairs,
  such as a Codex backend with a Claude concrete model.
- Harden Gemini backend parser and test isolation around callback failures and
  ambient fake-backend environment controls.

## v0.23.0 - 2026-05-11

### Added
- Add `agents.recall` as a recovery-only MCP and CLI surface for retrying a
  named agent after a 10-minute result timeout and inactive diagnostics.

### Fixed
- Recover async named-agent worker startup when the parent MCP process was
  launched from a plugin-cache runtime path that has since been replaced.

## v0.22.4 - 2026-05-11

### Changed
- Retire the live `claude-plugin/` source tree after the Codex-first
  `agents-plugin/` transition, preserving the old Claude home guidance as
  historical reference only.
- Stop installer and doctor flows from requiring the removed legacy Claude tree;
  installer cleanup now removes old repo-owned Claude home and hook symlinks.
- Move old spec archive material to `ai-docs/.old/spec/` and document
  `ai-docs/.old/` as the tracked project archive hidden from default listings.

### Fixed
- Make `ws/infra.read` serve bundled runtime infra docs so downstream projects
  without `claude-plugin/` can resolve executor infra such as
  `executor-wrapup`.

## v0.22.3 - 2026-05-11

### Fixed
- Relax the async API manager-start test wait so Windows smoke runs are not
  blocked by scheduler delay before the background manager goroutine reaches
  fake work.

## v0.22.2 - 2026-05-11

### Fixed
- Clarify `lead-write-code` reviewer partition registration by naming the
  reviewer agents and exact embedded prompt stems for correctness, fit, and
  test review partitions.

## v0.22.1 - 2026-05-11

### Fixed
- Start the `agents-plugin` Claude-facing MCP entrypoint through the
  cross-platform Python launcher so native Windows startup no longer depends on
  the POSIX shell wrapper.

## v0.22.0 - 2026-05-10

### Added
- Add `lead-salvage` for premise-collapse recovery: evidence preservation,
  blast-radius survey fanout, salvage reports, recovery epics, child tickets,
  and affected-ticket disposition before destructive cleanup.

### Changed
- Align skill-authoring guidance with active `ws:` plugin skill invocation
  syntax while preserving host-specific slash forms.

## v0.21.2 - 2026-05-10

### Changed
- Rework `lead-write-skeleton` around lead-owned `CONTRACT:`, `HINT:`, and
  `HOLE:` source drafts, delegated `skeleton-populator` normalization, and a
  lightweight read-only `skeleton-reviewer` loop.
- Remove the obsolete `skeleton-writer` compatibility prompt stem from the
  active runtime bundle.
- Add premise-aware intent framing and interview traversal to discussion
  workflows, and keep discussion responses in the user's active conversation
  language.
- Mark the legacy `claude-plugin/` tree as frozen fallback while keeping
  Codex-first workflow changes in `agents-plugin/` and `agents-plugin-tool/`.

## v0.21.1 - 2026-05-08

### Fixed
- Relax async API MCP test polling deadlines so Windows release smoke tests are
  not blocked by slow runner scheduling.

## v0.21.0 - 2026-05-08

### Added
- Add asynchronous API documentation jobs through `api.ask_async`,
  `api.status`, `api.result`, and `api.cancel`.

### Changed
- Resolve `light`, `core`, and `deep` as portable model aliases with
  harness-aware Codex/Claude defaults while preserving legacy `tier`
  compatibility.
- Keep epic tickets lightweight as milestone boards and route detailed
  implementation discussion into child tickets.
- Strengthen top-level workflow skill descriptions while keeping derived
  primitive descriptions lighter and requiring explicit persistence intent for
  rule storage.

### Fixed
- Deliver Codex named-agent prompts through stdin with the Codex CLI `-` marker
  so multiline first-call and resumed prompts avoid Windows/argv delivery
  issues.
- Ignore downstream `ai-docs/.deps/` API documentation cache data during
  bootstrap-generated `.gitignore` setup.

## v0.20.2 - 2026-05-07

### Changed
- Load `lead-workflow-manual` from routing and delegated implementation skills
  when workflow primitive context is absent.
- Normalize embedded named-agent prompt tier frontmatter to the host-neutral
  `light`, `core`, and `deep` vocabulary while preserving runtime alias
  compatibility.
- Document MCP-first workflow language for general-purpose named-agent
  registration and role-specific prompt usage.

## v0.20.1 - 2026-05-06

### Changed
- Add bootstrap-managed `ai-docs/WORKFLOW.md` guidance so downstream projects
  retain plugin-less workflow maintenance instructions while plugin/runtime
  semantics remain canonical.
- Clarify workflow primitive guidance around native ticket status moves and
  `ws/git.commit`.

## v0.20.0 - 2026-05-06

### Changed
- Split ticket lifecycle semantics so `todo/` is accepted backlog and `ready/`
  is the spec-gated implementation queue across ticket discovery,
  `project_tree`, workflow skills, conventions, bootstrap migration, and Claude
  compatibility guidance.
- Tighten `lead-proceed` and Claude `/proceed` so `todo/` tickets route through
  ready promotion before skeleton or implementation work begins.
- Make `lead-write-code` and `lead-edit` review directives more risk-scoped by
  focusing reviewer prompts and limiting re-review to material findings.

### Fixed
- Teach `git.commit` ticket move expansion to recognize `ready/` paths,
  including `todo/` -> `ready/` promotion moves.

## v0.19.0 - 2026-05-06

### Added
- Add `ws-mcp runtime capabilities`, a single-process JSON probe that reports
  runtime version, MCP protocol, prompt bundle metadata, full lead MCP tool
  names, and the public CLI command surface for launcher compatibility checks.

### Changed
- Teach the Python MCP launcher to prefer the runtime capabilities probe before
  falling back to legacy full validation, reducing first-run validation
  sensitivity to command-surface growth.
- Resolve root-omitted MCP tool calls through host workspace metadata and
  explicit non-dot server startup roots before `WS_MCP_PROJECT_ROOT`; invalid
  explicit startup roots now fail closed.

### Fixed
- Avoid repeated launcher compatibility fanout on steady-state startup by using
  a fail-closed compatibility stamp keyed to the runtime contract and binary
  identity.

## v0.18.1 - 2026-05-06

### Fixed
- Remove unreliable worktree-lock MCP tool authority detection so
  plugin-managed sessions default to the lead tool surface; document
  `WS_MCP_TOOL_PROFILE` as an optional containment filter rather than an
  authority boundary.

## v0.18.0 - 2026-05-05

### Changed
- Re-link Claude plugin install from `claude-plugin/` to `agents-plugin/` so
  the Codex-first candidate is now the single source for both Claude and Codex
  consumers.
- Wire inline `mcpServers` in `agents-plugin/.claude-plugin/plugin.json` using
  `${CLAUDE_PLUGIN_ROOT}` so the MCP server is accessible from Claude sessions
  without touching the Codex-facing `.mcp.json`.
- Update `.claude-plugin/marketplace.json` external source to `./agents-plugin`,
  completing the remote-install path migration.

## v0.17.6 - 2026-05-05

### Fixed
- Prefer the sibling Claude PowerShell shim on Windows so multiline system
  prompts are passed as argv instead of being reinterpreted by `cmd`.

## v0.17.5 - 2026-05-05

### Fixed
- Invoke Windows Claude `.cmd` and `.bat` backend shims through `cmd /c call`
  so quoted batch paths execute correctly.

## v0.17.4 - 2026-05-05

### Fixed
- Quote Claude `.cmd` and `.bat` backend arguments on Windows so JSON settings
  and prompts survive command-shell execution.

## v0.17.3 - 2026-05-05

### Added
- Add a Claude backend adapter for ws named agents while preserving the shared
  named-agent lifecycle.

### Fixed
- Generate Claude session ids in the UUID format required by the Claude CLI.
- Resume Claude sessions after hook-delivered mailbox messages so active hook
  interrupts produce a final result instead of empty output.

## v0.17.2 - 2026-05-05

### Fixed
- Fix the remaining Windows CI session default-root status test to compare
  decoded JSON fields instead of escaped response text.

## v0.17.1 - 2026-05-05

### Fixed
- Fix the Windows CI session default-root test to compare decoded JSON paths
  instead of escaped response text.

## v0.17.0 - 2026-05-05

### Added
- Add `session.set_default_root` and `session.get_default_root` MCP tools for
  volatile per-server repository root recovery.

### Changed
- Resolve root-omitted MCP tool calls through explicit `root`, session default,
  `WS_MCP_PROJECT_ROOT`, unambiguous Codex workspace metadata, and startup root.
- Rename the Codex workflow primitive reference skill from `lead-workflow` to
  `lead-workflow-manual`.

## v0.16.3 - 2026-05-05

### Changed
- Add bounded named-agent backend failure diagnostics that preserve the raw
  backend error and include PATH-detected backend/configuration hints.

### Fixed
- Keep completed Codex named-agent results when Windows process-control text
  appears on stdout after valid JSONL output.

## v0.16.2 - 2026-05-05

### Changed
- Run the Codex plugin-managed MCP server through the Python launcher so native
  Windows plugin installs do not try to execute the POSIX launcher.
- Limit the release workflow trigger to `v*` tag pushes instead of every main
  branch push.

### Fixed
- Keep lead MCP tools visible when startup project-root detection leaves the
  orchestrator lock unavailable; callers can still pass an explicit `root`.

## v0.16.1 - 2026-05-05

### Fixed
- Normalize embedded prompt bundle hash inputs across LF and CRLF checkouts so
  Windows GitHub Actions runners validate the same `runtime.json` contract as
  macOS/Linux builds.

## v0.16.0 - 2026-05-05

### Added
- Codex-first `agents-plugin/` package with `lead-*` workflow skills and plugin-managed MCP configuration.
- Native Go `ws-mcp` runtime for Codex plugin installs, including project docs, tickets, specs, mental models, Git, API docs, generated paths, runtime metadata, and named-agent MCP tools.
- Plugin-local runtime launcher with release download, checksum verification, compatibility checks, project-root detection, and local development repair.
- Embedded prompt bundle for reviewers, implementers, surveys, skeleton writing, API docs, and delegate orientation.
- Async named-agent runtime with register/call/wait/result/status/tail/debug/cancel/erase surfaces and async `subquery` fan-out.
- GitHub Actions release workflow that builds cross-platform `ws-mcp-*` assets, verifies checksums, uploads artifacts, and publishes release assets for `v*` tags.

### Changed
- Shifted the active workflow authority to `AGENTS.md` while keeping `CLAUDE.md` as a compatibility shim.
- Rebuilt the active spec corpus and mental-model domains around the Codex-first plugin/runtime surface.
- Updated workflow skills to prefer host-neutral MCP primitives and ws named-agent delegation.
- Split agent readiness from result consumption: `agents.wait` reports readiness and `agents.result` is the result retrieval point.
- Changed `git.diff` to default to stat output, with full diff content available by parameter.
- Relaxed todo ticket spec gating for `epic` and `research` tickets.
- Reworked `ws` shipping docs around Codex/GitHub plugin install while preserving the stable Claude compatibility package.

### Fixed
- Increased default agent wait/result timeout guidance to 10 minutes for long-running workflow agents.
- Hardened named-agent lifecycle, diagnostics, nonblocking orchestration, worktree authority locking, cancellation, interrupt delivery, and large Codex JSONL reads.
- Fixed MCP Git commit staging for deleted paths and ticket moves.
- Fixed Windows runtime smoke failures in Go tests and executable validation.
- Reduced context-heavy agent tail output while keeping raw debug diagnostics available.
- Kept runtime contract metadata synchronized with embedded prompts and shortened generated subquery keys.

## v0.15.0 — 2026-04-29

### Added
- `/exit-session` skill — 4-phase session handoff: commit staged work, write context note to `_index.md ## Session Notes` from memory only (no tool calls), user approval, commit `_index.md`. Optimizes next-session orientation cost via file-path citations and `(uncertain)` markers.

### Changed
- `ws-subquery`: Explore-level tool access — Bash (read-only), Read, Glob, Grep, WebFetch, WebSearch permitted; Edit, Write, NotebookEdit, Agent prohibited. Matches the native `Explore` subagent type.
- `ws-subquery`: stdin/heredoc support — accepts inline string, `-` sentinel (reads stdin), or piped stdin. Enables multi-line prompts via `ws-subquery --deep-research - <<'PROMPT' ... PROMPT`.
- `forge-spec`: all native `Agent()` calls replaced with `ws-subquery --deep-research` (survey dispatches) and `ws-oneshot-agent -p clerk` (ticket association). No native Agent tool dependency.
- `forge-mental-model`: all native `Agent()` calls replaced with `ws-subquery --deep-research`. No native Agent tool dependency.
- `clerk.md`: moved from `claude-plugin/agents/` to `claude-plugin/infra/prompts/`. Invoked via `ws-oneshot-agent -p clerk --model sonnet`.

## v0.14.0 — 2026-04-29

### Added
- `ws-ask-api` — new bin tool: queries a per-project `ai-docs/.deps/` external API documentation cache. 2-layer routing: Haiku pre-router resolves canonical domain names; persistent `api-doc-<domain>` named-agent sessions handle fetch, cache, and answer. Supports `--refresh`, `--check-stale`, `--list`. Parallel dispatch for multi-domain queries. Exit code propagated from all call sites.
- `api-doc-manager` infra prompt — per-domain executor agent: bootstraps `l1–l3.md` + scripts on first use, answers queries from cache, re-fetches on stale detection.
- `pre-router` infra prompt — Haiku one-shot agent: maps free-text prompts to canonical `.deps/` domain names with fuzzy matching and exact-match bypass.
- `--prompt-cond BINARY[=PROMPT]` flag on `ws-new-named-agent` — appends a named prompt to the system prompt only when the specified binary is present in PATH at registration time.
- `cargo-brief.md` infra prompt — injected via `--prompt-cond cargo-brief`; instructs agents to use `cargo brief` for Rust API exploration.
- `ws-ask-api` entry in `ws:workflow` skill primitives reference.

### Changed
- `workflow-for-agent.md`: added `## API Documentation` section — agents must use `ws-ask-api` for external library API lookup; direct `WebSearch`/`WebFetch` for API docs prohibited.

### Fixed
- `ws-ask-api-internal`: flock timeout removed — kernel releases fd on any exit including crash; Windows mkdir fallback replaced with PID-based stale lock detection (no timeout, crash-safe).
- `ws-ask-api-internal`: `api-doc-manager` now registered with `--no-doc-system` to prevent recursive `ws-ask-api` invocation from within the cache agent.

## v0.13.2 — 2026-04-29

### Fixed
- Remove all `timeout: 600000` mentions from skill and mental-model docs — blanket timeout instruction was causing downstream agents to insert incorrect timeout values into Bash calls, triggering 127 errors.

## v0.13.1 — 2026-04-29

### Changed
- `sprint` skill: inject project map at skill invocation via `!`ws-proj-tree`` — mirrors the pattern already present in `discuss`.

## v0.13.0 — 2026-04-29

### Added
- `ws-oneshot-agent` — new bin tool: registers, calls, and erases a named agent in one invocation. Accepts `-p <stem>` (multi-flag), `--model`, `--no-doc-system`. Doc-system injected by default. Stdin or inline positional prompt. EXIT trap guarantees cleanup.
- `ws-named-agent erase` — removes a named agent's registry entry and its associated Claude session file.
- `/workflow` skill — loads the WS orchestration primitives reference into session context; survives compaction via the Skill tool mechanism.
- `claude-plugin/infra/prompts/subquery.md` — extracted subquery worker prompt; standard agent layout (Identity/Constraints/Process/Output/Doctrine).

### Changed
- `ws-named-agent new`: accepts `-p <stem>` multi-flag; resolves against `infra/prompts/` → `infra/` → cwd; concatenates bodies with `---`; first frontmatter `model:` sets tier. A leading `ws:` prefix on `-p` values is silently stripped.
- `ws-named-agent new`: removed `--agent` and `--agent-type` flags (all call sites migrated to `-p`). `--system-prompt` retained as internal flag for compression re-registration only.
- `ws-subquery`: delegates to `ws-oneshot-agent -p subquery`; full tool access replaces prior `--allowed-tools` restriction; doc-system injected by default.
- Agent prompts consolidated from `claude/agents/` and `claude/infra/` into `claude-plugin/infra/prompts/` (single resolution root for `-p`).
- Plugin directory renamed `claude/` → `claude-plugin/`.
- `doc-system.md` renamed to `workflow-for-agent.md` (more accurate name for the orientation doc).

### Fixed
- `install.sh`: plugin snapshot correctly generates `marketplace.json` so `claude plugin install` discovery works on fresh machines.
- `install.sh`: purge stale registry entry before reinstall to bypass version no-op.

## v0.12.0 — 2026-04-28

### Added
- `/update-spec` skill — lead-driven spec audit: loads `spec-conventions.md` and `write-spec/SKILL.md`, scans a commit range for caller-visible behavior changes (`judge: spec-impact`), adds missing entries, strips `🚧` markers, and handles removals. No subagent delegation. Wired into `/edit` (after cleanup), `/implement` (doc pre-pass step 1), and `/sprint` (wrap-up step 2).
- `claude/infra/doc-system.md` — orientation doc for 3rd-party subagents: explains the three doc layers (spec/mental-model/tickets), `{#YYMMDD-slug}` stems, and `🚧` = planned-but-unimplemented. Auto-injected by `ws-named-agent new` into every agent system prompt.

### Changed
- `ws-named-agent new`: prepends `doc-system.md` to stored system prompt automatically. Pass `--no-doc-system` to suppress (for narrow-role agents such as sprint-survey, project-survey, compression helpers).
- `/sprint` wrap-up spec-update pass: replaced inline 11-line procedure with `Invoke ws:update-spec`.
- `/implement` doc pre-pass: replaced `ws:spec-updater` dispatch with `ws:update-spec` Skill invocation.
- `/edit`: added step 6 — invoke `ws:update-spec` on the edit's commit range; adds `Spec:` line to completion report.
- `ws-named-agent` (`codex` backend): compression disabled — multiple interacting bugs made it unreliable; token count still tracked for observability.
- `ws-named-agent`: `_subrun` now defaults `stdin=subprocess.DEVNULL` when neither `stdin` nor `input` is provided — prevents child claude/codex processes from inheriting the caller's stdin fd and blocking on a heredoc pipe.

### Fixed
- `ws-named-agent`: reconfigure `stdin` to UTF-8 on Windows — non-ASCII characters in heredoc prompts (e.g. `×`, `—`) were read via CP949-encoded stdin, producing surrogates that caused `UnicodeEncodeError` when passed as subprocess input.

## v0.11.4 — 2026-04-28

### Fixed
- `ws-named-agent`: reconfigure `stdout`/`stderr` to UTF-8 on Windows at module load — prevents `UnicodeEncodeError` and mojibake on non-UTF-8 locales (e.g. CP949). Uses `None` guard so the path under `pythonw.exe` (hook invocation) silently skips.

## v0.11.3 — 2026-04-28

### Added
- `ws-print-infra` now accepts bare stems (no `.md` suffix): probes exact match first, then appends `.md`; consistent with `ws-named-agent --system-prompt` resolution.

### Fixed
- `ws-named-agent`: PostToolBatch/PostToolUse hook uses `pythonw.exe` on Windows — suppresses per-tool-call console window flashes when running inside a PTY (e.g. claude-dash).
- `ws-named-agent` (codex, Windows): prompt now delivered via stdin (`-`) to bypass `cmd.exe /c` newline truncation that silently cut multi-line prompts to their first line.
- `ws-named-agent` (codex, Windows): compression re-registration now uses `sys.executable + SCRIPT_DIR` path — bare name `ws-named-agent` was unresolvable via `CreateProcess` on Windows.
- `ws-named-agent` (codex): `_codex_tokens()` no longer double-counts `cached_input_tokens`; OpenAI reports it as a subset of `input_tokens`, not an additive field — was inflating compression threshold checks by ~3×.

## v0.11.2 — 2026-04-28

### Added
- `claude/infra/searcher.md` — resident codebase-search agent with domain accumulation; lead agents spawn once per domain and reset via `ws-new-named-agent` on domain shift

### Changed
- `ws-new-named-agent --system-prompt` now accepts bare stems and bare names: probe order is `infra/<name>`, `infra/<name>.md`, `cwd/<name>`, `cwd/<name>.md`, then error; explicit paths pass through unchanged. Removes the `$(ws-infra-path xxx)` boilerplate from all call sites.

## v0.11.1 — 2026-04-28

### Fixed
- `ws-named-agent`: CLAUDE.md injection moved from claude backend to codex backend — claude CLI reads CLAUDE.md natively; codex does not, so injecting it into `model_instructions_file` is required for codex agents to observe project behavioral constraints

## v0.11.0 — 2026-04-28

### Added
- `/write-code` skill — new delegated-implementation primitive: brief → judge: plan-depth (as-is/survey/research) → implementer named-agent → 3-reviewer loop (correctness, fit, test) with won't-fix disposition system and 3-cycle cap with lead adjudication at cycle 2

### Changed
- `/edit` recast as direct-edit primitive: lead edits directly, one named-agent reviewer covering correctness+fit (temp-file concatenation), 2-cycle relay cap, self-cleanup, no doc pipeline
- `/implement` recast as harness: `judge: execution-mode` routes to `ws:edit` or `ws:write-code`; doc pre-pass (spec-updater then mental-model-updater, each committed separately); approval gate; merge
- `/sprint` routing table updated: calls `ws:edit` | `ws:write-code` directly; Delegation Cycle template removed; wrap-up auto-merges via `ws-merge-branch`
- `/proceed` simplified: always routes to `/implement`; `judge: direct-edit` and `judge: execution-mode` removed (now owned by `/implement`)

### Removed
- `/write-plan` skill — brief writing and `judge: plan-depth` absorbed into `/implement`; plan-populator infra docs moved to `claude/infra/`

## v0.10.6 — 2026-04-27

### Fixed
- `ws-named-agent` (compression): replaced `ws-infra-path` subprocess call with direct `PLUGIN_DIR / "infra" / "agent-compression.md"` read in both claude and codex backends; on Windows, Git Bash's `pwd` returns a POSIX path (`/c/Users/...`) that Python's `pathlib` cannot resolve, causing `FileNotFoundError` at every compression handoff

## v0.10.5 — 2026-04-27

### Fixed
- `ws-named-agent`: added `_WIN_NO_WINDOW = getattr(subprocess, "CREATE_NO_WINDOW", 0)` and `_subrun()` wrapper; all 11 `subprocess.run` call sites replaced — suppresses the brief console window that appears per-spawn for `.cmd` shims on Windows
- `ws-named-agent tail` (codex): added `last_agent_message` fallback in `task_complete` handler — when no inline `agent_message` events are present in a turn, the final message is read from `payload.last_agent_message`; also added explicit `uuid`/`pattern`/`search-dir` diagnostics when no session file is found

## v0.10.4 — 2026-04-27

### Added
- `.cmd` shims for all 16 scripts in `claude/bin/` — Python scripts call `python "%~dp0<name>" %*`; bash scripts call `bash "%~dp0<name>" %*`; cmd.exe selects `.cmd` via PATHEXT, Git Bash selects the shebang file; no conflict
- `ws-named-agent`: `_inject_git_bash()` runs at module load on Windows — reads Git installation path from `HKLM\SOFTWARE\GitForWindows` registry key, falls back to `C:\Program Files\Git\bin`; injects into `PATH` so all subsequent subprocess calls (including hooks and `.cmd` shims) can resolve `bash`

### Fixed
- `ws-named-agent override`: local config path moved from `<git-root>/.claude/kang-sw-devenv-ws.json` to `~/.claude/kang-sw-devenv/ws/<escaped-proj>.json`; eliminates git tracking of machine-local config

## v0.10.3 — 2026-04-27

### Added
- `ws-named-agent override [-g]` — two-layer config: global (`~/.claude/kang-sw-devenv-ws.json`) and local (`<git-root>/.claude/kang-sw-devenv-ws.json`); `-g` writes to global, default writes to local; `override show` displays both layers separately; local wins on merge conflict

### Fixed
- `.gitignore`: added `.claude/kang-sw-devenv-ws.json` (local config is per-machine, not committed)

## v0.10.2 — 2026-04-27

### Added
- `ws-named-agent override default <model>` — sets a per-repo fallback model in the config file; used by `ws-named-agent new` when no `--model`, agent frontmatter, or `--agent-type` is present. Config file only — `WS_OVERRIDE_*` env vars are not involved.

### Fixed
- `ws-named-agent`: all `Path.read_text()` / `Path.write_text()` calls now pass `encoding='utf-8'` explicitly; on Windows systems with a non-UTF-8 locale (e.g. CP949), output files were saved in the system code page
- `ws-named-agent tail` (codex): rewrote `_tail_codex` for the actual session file format — `{"timestamp","type","payload"}` envelope with `event_msg{task_started/task_complete/agent_message}` and `response_item{function_call}` events; old parser was written against the `--json` stdout format and found zero turns
- `ws-named-agent` (codex compression): `_call_codex` haiku intent step used hardcoded `"claude"` instead of `_claude_exe()`; fails on Windows where claude is `claude.cmd`
- `ws-named-agent` (codex): `_run()` now uses `_codex_exe()` wrapper; `codex.cmd` on Windows cannot be invoked without `cmd /c`

## v0.10.1 — 2026-04-27

### Added
- `ws-named-agent` (claude backend) — injects CLAUDE.md into the system prompt between the agent definition and the registered system prompt; walks from git root to CWD collecting all CLAUDE.md files in outer-to-inner order

### Fixed
- `ws-named-agent` Windows portability: `claude.cmd` now invoked via `cmd /c` (resolved with `shutil.which`); `tmp.replace()` replaces `tmp.rename()` to handle existing destination; `find` subprocess calls replaced with `Path.glob`/`Path.rglob`; all `text=True` subprocess calls and temp file opens now specify `encoding="utf-8"`

## v0.10.0 — 2026-04-27

### Added
- `ws-named-agent` — unified Python CLI entry point for named agent management (subcommands: `new`, `call`, `interrupt`, `print`, `check-mailbox`, `tail`, `override`); multi-backend routing (claude, codex, gemini stub); replaces the standalone bash scripts which are now one-liner shims
- `ws-named-agent tail` — reads last N assistant turns from the live session JSONL on disk (safe while agent is running); supports claude and codex session formats
- `ws-named-agent override` — persists tier-to-model overrides in `~/.claude/<repo>-ws.json`; resolution order: `WS_OVERRIDE_<TIER>` env var > config file > stored model
- codex backend — full feature parity with claude backend: session routing via `~/.codex/sessions`, `PostToolUse` hook for outbox drain, system prompt via `model_instructions_file`, compression handoff
- `ai-docs/ref/codex-integration.md` — probed codex CLI behavior reference (invocation, JSONL format, session management, hook config, model flag behavior, PATH inheritance)

### Changed
- `ws-call-named-agent`, `ws-new-named-agent`, `ws-interrupt-named-agent`, `ws-print-named-agent-output`, `ws-agent-check-mailbox` — converted to one-liner shims delegating to `ws-named-agent`
- Model resolution: frontmatter `model:` field read at `ws-named-agent new` to set initial tier; `--model` overrides frontmatter; backend shorthands (`claude`, `codex`, `gemini`) stored as model but never passed as `--model` to their CLIs
- `/sprint` Delegation Cycle — `ws-call-named-agent` calls use `run_in_background: true`
- `/implement` — feature-branch auto-merge mode reverted; approval gate now unconditional

### Fixed
- `ws-call-named-agent` — replaced path construction with `find` for Windows Git Bash portability
- `ws-agent-check-mailbox` hook path — now absolute so it resolves correctly regardless of hook shell's working directory

## v0.9.0 — 2026-04-27

### Added
- `claude-dash` — Rust PTY TUI multiplexer for worktree-scoped Claude sessions: tabbed interface, named-agent panel, prefix-key bindings (`<prefix>+q/w/e/r…`), mouse navigation, `--dangerously-skip-permissions` flag, and `claude --worktree` tab spawning

### Changed
- `ws-call-named-agent` — retry-with-backoff on "Session already in use"; carry pending interrupts across compaction handoff
- `ws-interrupt-named-agent` — removed dead argument-count guard branch

## v0.8.0 — 2026-04-27

### Added
- `ws-interrupt-named-agent` — queue a mid-task message into a named agent's outbox; PostToolBatch hook (`ws-agent-check-mailbox`) stops the agent at the next tool boundary; `ws-call-named-agent` drain loop delivers the message on resume
- `ws-agent-check-mailbox` — PostToolBatch hook script used internally by `ws-call-named-agent` to stop running agents when outbox is non-empty

### Changed
- `ws-call-named-agent` — hook settings now inlined as raw JSON via `--settings` flag (eliminates per-agent settings.json temp files); applies to all call paths including compression handoff
- `claude-watch` session discovery — also scans `~/.claude/projects/` for subdirectory names starting with the escaped main worktree path, surfacing sub-project sessions (e.g. `tools/claude-dash`)
- `/sprint` branch naming — infers a kebab-case slug from context when topic is clear; falls back to random `<adjective>-<noun>-<noun>` name; never prompts the user
- `/sprint` wrap-up discipline — each doc updater (spec-updater, mental-model-updater) commits its output immediately after completion; no batching

## v0.7.0 — 2026-04-24

### Added
- `ws-call-agent` — `claude -p` wrapper with permission bypass; `--agent` flag for deterministic UUID sessions (create-or-resume), `--session-id`, `--uuid`, `--system-prompt`
- `ws-agent` — deterministic UUID v5 from repo-root + branch + name
- `ws-declare-agent` — clears session files to scope agent slots to a run
- `ws-call-agent` formatted output: `[info]`/`[warn]` context window line (shown when `--agent` used and fill ≥50%; warn at ≥70% of 150K)

### Changed
- `ws:implement` internal orchestration rewritten — `TeamCreate`/`SendMessage`/`TeamDelete` replaced with `ws-call-agent`/`ws-declare-agent`
- `ws-*` scripts moved from `claude/infra/` to `claude/bin/` (PATH-accessible)

### Removed
- `/parallel-implement` skill — split-scope work now handled via split tickets + `/implement`
- `/team-lead` skill — no longer needed without `TeamCreate` machinery

## v0.6.0 — 2026-04-24

### Added

- `/add-rule` skill: classify an incoming rule as cross-cutting (→ `CLAUDE.md ## Architecture Rules`) or domain-scoped (→ `ai-docs/mental-model/<domain>.md ## Domain Rules`). Autonomous when clear, interactive when ambiguous. Append-only — never modifies existing rules.
- Ship config (`ai-docs/ship/ws.md`): version strategy, CHANGELOG procedure, pre-flight, tag, and push steps for the `ws` plugin.

### Changed

- **2-layer Architecture Rules split**: `CLAUDE.md ## Architecture Rules` is now scoped to cross-cutting invariants only. Domain-scoped rules belong in `ai-docs/mental-model/<domain>.md ## Domain Rules`.
- `mental-model-conventions.md`: added `## Directory Hierarchy` (flat vs `<domain>/index.md` + children), ancestor loading invariant, and `## Domain Rules` section with authorship and modification constraints.
- `ai-docs/mental-model.md`: updated index with directory hierarchy and Domain Rules sections.
- `claude/bin/list-mental-model`: rewritten with tree output (`├─`/`└─` glyphs); ancestor `index.md` auto-emitted alongside matching direct-child sub-domain in filtered mode.
- `mental-model-updater`: gains `/forge-mental-model` authority — creates new domain docs and splits flat docs to `<domain>/index.md` + children when diff shows code-structure change. Domain Rules promotion-only (upward during splits, never downward, never content-modified). Stale rules flagged in `## Stale Rules` output block; never edited autonomously.
- `executor-wrapup.md`: added `§Ancestor Loading` contract (3-step procedure) and Invariant bullet for one-level hierarchy reads.
- `edit`, `implement`, `parallel-implement` skills: propagate ancestor-loading contract to Invariants and spawn prompts, bounded to one-level hierarchies (`<domain>/<sub>.md` only).
- `CLAUDE.template.md` v0028: tightened `## Architecture Rules` inclusion test (explicitly excludes domain-scoped rules; directs authors to `/add-rule`); added v0028 migration item (reclassify existing Architecture Rules entries that are domain-scoped).
- `_index.md`: stale per-commit version-bump rule removed (ship config is now the authority); `/add-rule` added to skill inventory.

## v0.5.0 — 2026-04-24

### Added

- `project-survey` agent: pre-invocation context survey; returns `[Must|Maybe]`-tiered spec/mental-model/ticket reference list for a given brief.
- Auto-invoke integration: `edit`, `implement`, `parallel-implement`, `discuss` each spawn `project-survey` at step 0.
