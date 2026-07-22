# Changelog

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
