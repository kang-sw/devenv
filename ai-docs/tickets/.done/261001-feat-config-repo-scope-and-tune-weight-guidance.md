---
title: "Repo-scope config that applies, and lead-tune guidance for workflow weight"
related:
  261001-bug-config-scope-gaps-after-review-opt-in: absorbs its gap 1 (repo-blind resolvers); gaps 2 and 3 stay there
  261001-feat-review-phase-lite-default: its off|lite|full review_phase is one of the levers lead-tune explains
  261001-feat-opt-in-design-and-phase-review: introduced the review knobs whose weight lead-tune now explains
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 611b8a39958c3b5d
sage-review-completeness-reviewed: 611b8a39958c3b5d
completed: 2026-10-01
---

# Repo-scope config that applies, and lead-tune guidance for workflow weight

## Background

ws now exposes several knobs that set how heavy the workflow is: design
review (`sage_review_design`), completeness review (`sage_review`),
per-phase code review (`review_phase`), model tier (`agents.tier`), and
delegation posture (`workflow.prefer_subagent`). Downstream users,
especially low-quota ones, need to adjust that weight without knowing the
knob names, and need to know what each lever does and what lowering it
loses. Two problems block this today.

1. **Repo scope is shown but not applied.** The committed repo scope
   (`<repo-root>/.ws-workflow/config.json`) is read only where the caller
   builds its resolver with `wsconfig.Options{RepoRoot: root}`: `config.list`,
   `review_phase`, and `ticket-assignee-aware`. `tickets.sage_gate`,
   `tickets.create_empty`, `tickets.move`, the worktree-pool readers, and
   most other `server.go` readers pass `wsconfig.Options{}`, so the repo
   layer drops out. A committed `sage_review_design: auto` appears in
   `config.list` as the repo value while the gate ignores it. Nothing
   shipped tells a user the repo file's path or shape either; only Go code
   (`internal/wsconfig/repo.go`, `config.go`) defines it as
   `{"overrides": {"<knob>": "<value>"}}`.
2. **lead-tune routes, it does not explain.** `lead-tune.md` maps a request
   to one knob (`judge: tune-target`) and calls anything else unsupported.
   It never tells the user what a lever does, what lowering it costs, or what
   the scopes mean. A vague request such as "the workflow is too heavy" spans
   several knobs and has no handler. The skill description triggers on
   delegation and model-tier wording, not on cost or review wording.

## Decisions

1. **One session-anchored resolver constructor.** A single constructor
   resolves the session key to its canonical worktree root and builds the
   resolver with that root as `RepoRoot`. Every resolver-backed reader in
   the MCP server switches to it. A keyless call or an unknown key leaves
   `RepoRoot` empty, so the repo scope drops out, never an error (the rule
   `config.list` already follows).
   *Rejected: patch only the three Sage call sites* - leaves the other
   repo-blind readers in place, and the per-knob repo flag (Decision 2)
   would not be structurally true.
2. **`config.list` publishes repo-scope information.** It emits the repo
   file path resolved from the session's root, the `overrides` shape, and,
   per knob, whether the repo scope applies to it. lead-tune reads this
   instead of carrying the format itself.
3. **Repo scope stays read-only to `config.tune`.** When the user wants a
   team-wide setting, lead-tune drafts the `.ws-workflow/config.json` edit
   from `config.list`'s repo information and proposes an ordinary commit.
   The committed file goes through commit history and review like any
   other project decision.
   *Rejected: add a repo-scope writer to `config.tune`* - convenient, but
   reopens what `ScopeSchemaEnum` deliberately excludes.
4. **Lever effects are single-sourced in `config.list`.** Each weight-lever
   knob's Go description states what each value does and what lowering it
   loses. lead-tune translates that into the user's language and does not
   restate it.
   *Rejected: effect prose in `lead-tune.md`* - drifts from the code as
   values change.
5. **lead-tune explains; the user chooses.** lead-tune states each
   knob's function and side effects plainly. The lead, a capable model,
   reads the user's request, identifies the knobs that bear on it, and
   explains those: their values, what each does, what lowering it loses,
   and the current value and scope. The aim is a user who can then say a
   concrete choice ("review lite, skip design review"). The lead writes
   only a choice the user states explicitly, through the existing Tuning
   Proposal confirmation, and never infers a knob change from vague
   wording. The playbook carries knob knowledge, not a script for
   particular complaints.
   *Rejected: the lead composes a bundle of changes from vague wording* -
   the user would not know what changed or how to adjust it next time.
   *Rejected: a preset knob (`light`/`heavy`)* - a new stored state that
   shadows the real knobs.
   *Rejected: fixed bundle tables in the playbook* - drift as knobs are
   added, and are a preset in prose.
   *Rejected: a weight-complaint branch in `judge: proactive-propose`* -
   a scripted response to one kind of request; `proactive-propose` is
   unchanged.
6. **Values are named exactly as `config.list` accepts them.** When the
   lead explains a knob it uses the accepted value words (`off|lite|full`;
   `off|ask|auto`, glossed as skip/recommend/require), so the user's
   choice maps to a value without interpretation.
7. **Scopes are explained in plain terms.** session: this work stream only,
   gone when it ends. project: this project on this machine, persists.
   repo: the whole team, through the committed file (Decision 3). global:
   all of the user's projects. When the user names no scope, the lead
   suggests session for a one-off request and project for a stated
   standing preference. A global-only knob (delegation posture) is
   explained as such.
8. **Cost hints are qualitative.** Each lever's `config.list` description
   (Decision 4) says which tier runs and how often (for example, one
   large-tier reviewer per ticket promotion; one medium-tier reviewer per
   phase under `lite`). No numeric token figures: no telemetry exists to
   ground them.
9. **The skill description triggers on weight wording.** `lead-tune`'s
   `SKILL.md` description adds cost and weight phrasing ("workflow too
   heavy/slow/expensive", "turn review on/off", "more/less review").
   `lead-workflow-manual` is unchanged.
   *Rejected: widen the always-on `lead-workflow-manual` "Workflow tuning"
   line* - the owner keeps the always-on manual lean.
10. **Repo-flag derivation.** A knob takes repo scope when it is
    resolver-backed and not global-only; `agents.tier` and global-only
    knobs do not (`Resolver.Get` skips the repo overlay for `GlobalOnly`
    items, and `loadRepoConfig` contributes only `overrides`). `prompt.*`
    overrides follow the general rule: the playbook-render prompt resolver
    (`playbook_tools.go`, today built with `wsconfig.Options{}`) moves to
    the session-anchored constructor, so a committed prompt override applies
    at render as `config.list` already shows it; project scope still beats
    it for a local override. The flag Decision 2 publishes comes from this
    rule in the registry.
    *Rejected: exclude `prompt.*` from repo scope* - an exception the rule
    does not need, and `config.list` would have to stop showing repo values
    it reads today.
11. **Scope of the absorbed gaps.** This ticket takes gap 1 of
    `261001-bug-config-scope-gaps-after-review-opt-in`; that idea ticket
    records the move. Gap 2 (inherited session values shown as the child's
    own) and gap 3 (CLI `ticketsMove` without builtin defaults) stay there.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Read every manual the `AGENTS.md` `### Implementation Conventions` rows
  match before editing (`shipped-surface-boundary.md`, `skill-authoring.md`,
  `wsflow-mirroring.md`, `ws-mcp.md`). Shipped text names no devenv-only
  path or ticket.
- Regenerate `agents-plugin-wsflow/` mirrors and manifests per
  `wsflow-mirroring.md`, and resync the byte-identical `agents-plugin-pi/rsrc/`
  mirror (guarded by `TestPiMirrorUpToDate`), in the phase that edits their
  sources; do not hand-edit mirrors.
- Scope precedence (session > project > repo > global > builtin) and the
  global-only skip in `Resolver.Get` are unchanged.
- `config.tune`'s writable scopes (`ScopeSchemaEnum`: session, project,
  global) are unchanged.
- `lead-workflow-manual` and `judge: proactive-propose` are unchanged.

## Prior Decisions

- 260917-feat-ws-committed-project-config-scope (2026-09-17, Result): "config.tune write of repo: no. Read-only / hand-edited for now." — bearing: supports
- 430ca5d3 (2026-10-01, commit): "repo-scope config is not read by the sage gate/move/create resolvers (pre-existing for sage_review), and config.list labels a parent-inherited session value as session scope" — bearing: supports
- b193a7ee (2026-09-17, commit): "config.tune's echo resolver stays repo-unaware (out of ticket scope, and no tunable key is repo-committed today)" — bearing: constrains
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-23, Result): "the ten per-knob config.* tools are gone; callers read through config.list and write through config.tune" — bearing: supports
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, commit): "fixed round-1 correctness finding that config.list omitted global tier mappings" — bearing: supports
- 260619-feat-ws-lead-tune-skill (2026-06-20, Decisions): "Dedicated entry skill, not workflow-manual prose. Its description is the runtime trigger surface (mental model workflow-skills #260508)" — bearing: supports
- 260625-refactor-workflow-delegation-config (2026-06-26, commit): "lead-tune now routes strict subagent posture through workflow.prefer_subagent / config.workflow_prefer_subagent" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/config_registry.go, agents-plugin-tool/internal/wsconfig/resolver.go, repo.go, scope.go, scoped_show.go, agents-plugin/rsrc/lead-tune/lead-tune.md, agents-plugin/skills/lead-tune/SKILL.md, agents-plugin-wsflow mirrors, agents-plugin-pi/rsrc mirror |
| scope.surface | public-interface | config.list's View/configListView output gains fields (repo path, overrides shape, per-knob repo flag) read by every lead caller (internal/mcp/server.go#L784-L825, internal/wsconfig/scoped_show.go) |
| scope.new_public_symbol | no | the session-anchored resolver constructor follows the existing unexported-helper pattern (s.resolveReviewPhase at internal/mcp/server.go#L2484, s.assigneeFeature at internal/mcp/server.go#L532); no new MCP tool or exported Go symbol is named |
| scope.new_type_contract | yes | wsconfig.View/configListView (internal/wsconfig/scoped_show.go, internal/mcp/server.go) gains fields for the repo path, overrides shape, and per-knob repo flag (Decision 2) |
| scope.test_surface | existing | internal/mcp/server_test.go, internal/wsconfig/scope_test.go, internal/wsconfig/repo_test.go, internal/mcp/playbook_tools_test.go carry the resolver/config.list suites Phase 1 extends; agents-plugin/tests/test_skill_dispatch_contracts.py and wsflow/pi drift tests cover Phase 2 |
| complexity.reuse_points | confirmed | wsconfig.Resolver/Options{RepoRoot}, wsconfig.ScopeSchemaEnum, GlobalOnly/DefaultScope registry (internal/wsconfig/scope.go), and the s.sessions.lookup(key).root pattern already used at internal/mcp/server.go#L803 for config.list |
| complexity.side_effect_risk | moderate | the session-anchored constructor replaces wsconfig.Options{} at roughly 20 call sites across internal/mcp/server.go, each needing a deliberate switch-or-leave judgment |
| risk.correctness | moderate | every resolver-backed reader moves to one constructor; a missed reader stays repo-blind, and prompt.* render gains repo scope (internal/mcp/server.go#L817, #L2207, buildPromptOverrideListing; playbook_tools.go#L654) |
| risk.fit | low | Decisions 1-11 stay within the existing Resolver/Options/ScopeSchemaEnum framework and explicitly reject broader interventions |
| risk.test | moderate | Phase 1 verification names six distinct scenarios (i-vi) across temp-repo sage_gate/create_empty/move/worktree-pool/config.list paths, each needing new or extended Go tests |
| risk.security_or_contract | moderate | a previously-inert committed sage_review_design/sage_review/other repo override becomes live at multiple gates (tickets.sage_gate, tickets.create_empty, tickets.move, worktree pool) — a silent behavior change for any downstream repo with an existing .ws-workflow/config.json |


## Phases

### Phase 1: repo scope that applies, published by config.list

- Add one constructor that takes a session key, looks up its canonical
  worktree root, and returns a resolver with that root as `RepoRoot`
  (empty for a keyless call or unknown key). Switch every resolver-backed
  reader in `internal/mcp` to it, including:
  - the Sage readers in `tickets.sage_gate`, `tickets.create_empty`, and
    `tickets.move` (`server.go` ~L1610, ~L1643, ~L1682);
  - the worktree-pool readers in `git.merge`, `worktree.acquire`,
    `worktree.release`, and `worktree.list` (`server.go` ~L1142, ~L1169,
    ~L1224, ~L1240);
  - the `config.tune` echo readers that print the effective value after a
    set or reset (`server.go` ~L928-L995), so a reset reports the repo value
    that now applies; the writers stay unchanged because repo scope is
    read-only;
  - the existing `RepoRoot` call sites (`resolveReviewPhase`,
    `assigneeFeature`, `config.list`);
  - the playbook-render prompt-override resolver (`playbook_tools.go`
    `buildOverrideLookup`, Decision 10).
  A reader with no repo-capable item (for example a resolver over a
  global-only key) may stay as it is; the worker names each one it leaves
  and why.
- Resolver errors at a repo-anchored reader fail loud. A malformed
  committed `.ws-workflow/config.json` makes the call return an error that
  names the file, as `config.list` already does since `b193a7ee`, instead
  of a dropped error that resolves to an empty value. This matters most at
  the Sage readers, where `""` maps to `skipped` and would silently bypass
  review, and at the pool readers. `resolveReviewPhase`'s builtin fallback
  on error changes to the same rule.
- `config.list`: emit the repo-scope file path resolved from the session
  root (or that none applies), the `{"overrides": {"<knob>": "<value>"}}`
  shape, and a per-knob repo flag from the Decision 10 rule. Repo-capable
  items outside the tuning catalog (`worktree_pool`,
  `ticket-assignee-aware`) are marked the same way in the scoped view, so
  every key a team may commit shows whether repo scope applies.
- Rewrite the descriptions of `sage_review_design`, `sage_review`,
  `review_phase`, `agents.tier`, and `workflow.prefer_subagent` so each
  states what each value does, what lowering it loses, and a qualitative
  cost hint (Decisions 4, 8).

Verification: Go tests showing (i) a committed `sage_review_design: auto`
in a temp repo's `.ws-workflow/config.json` makes `tickets.sage_gate`
require design review under a session rooted there, with no project or
global value; (ii) the same for `tickets.create_empty`/`tickets.move`
posture and a worktree-pool knob; (iii) a project-scope value still beats
the repo value; (iv) a keyless call drops the repo scope without error;
(v) `config.list` reports the repo path, the overrides shape, and the
per-knob flag (true for `sage_review_design`, false for `agents.tier` and a
global-only knob, true for a `prompt.*` knob); (vi) a committed
`prompt.<pointId>.<harness>` override in the temp repo appears in a
rendered playbook under a session rooted there; (vii) a malformed
committed repo file makes `tickets.sage_gate`, `tickets.move`, and a
worktree-pool reader return an error naming the file, never a `skipped`
posture or an empty template; (viii) after a project-scope reset of
`sage_review_design` with a committed repo `auto`, `config.tune` reports
`auto` at scope `repo`; (ix) existing golden description tests updated.
Full `go test ./...` for `agents-plugin-tool`.

### Result (84f3b74a2) - 2026-10-01

- One session-anchored constructor (`sessionConfigOptions` / `sessionResolver`
  in `internal/mcp/server.go`) now serves every resolver-backed reader: the
  Sage readers (`tickets.sage_gate`, `tickets.create_empty`, `tickets.move`),
  the pool readers (`git.merge`, `worktree.acquire/release/list`), the
  `config.tune` reset echo, `resolveReviewPhase`, `assigneeFeature`,
  `config.list`, `buildOverrideLookup`, and the `workflow.lang` readers in
  `playbook.read`, `playbook.render`, and `workflow_manual`.
- Left on `wsconfig.Options{}` (no repo-capable key): `config.resolve_agent`,
  `tuneAgentsTier`, `currentAgentTierMappings`, and the
  `printPlaybook`/`renderPlaybook` config options (agents.tier, not
  resolver-backed); the `bootstrap_alarm` readers in ferrule and
  `workflow_manual` and `workflowPreferSubagentEnabled` (global-only); the
  `config.tune` writers (repo scope is read-only); `wsnote`/mailbox
  `MachinePath` (not a resolver).
- Fail-loud: every repo-anchored reader returns its load error, and
  `loadRepoConfig` errors name the file. `resolveReviewPhase` no longer falls
  back to the builtin. `buildOverrideLookup` returns an error after a new
  `Resolver.Check` probe, because its closure cannot carry one; this makes
  `playbook.read`, `playbook.render`, and `workflow_manual` fail on a malformed
  committed file too. `git.merge` fails on a config error but still degrades to
  "unknown pool" when worktree listing fails.
- `config.list` emits `repo_scope` {path, exists, shape} and a `repo_scope`
  flag per catalog knob (`configKeyEntry.RepoScoped`: resolver-backed and not
  global-only) and per scoped-view item (`wsconfig.RepoScoped`).
  `ticket-assignee-aware` got a declared default scope so the scoped view
  always lists it.
- Added beyond the plan: the `config.tune` scalar set echo appends a
  `shadowed: effective ...` line when the effective value comes from another
  scope (for example a global write under a committed repo value).
- The five weight-lever descriptions state each value's effect, what lowering
  it loses, and a qualitative cost hint.
- Verification: tests in `internal/mcp/config_repo_scope_test.go` (items i-ix,
  4bc9705d5, tightened in 3fa024cc8) and `internal/wsconfig/repo_test.go`.
  `go test ./...` in `agents-plugin-tool` passed on 84f3b74a2.
  `go test ./internal/wsconfig ./internal/mcp -count=1` passed on 4bc9705d5.
  No existing test pinned the old description wording.

### Phase 2: lead-tune explains the weight knobs

- `lead-tune.md`: replace the Sage-only posture handler and the
  `review_phase` routing line with knob knowledge per Decision 5: how to
  read `config.list` descriptions and current values, explain the knobs a
  request bears on with their exact value words (Decision 6), explain
  scopes (Decision 7), and write only the user's explicit choice through
  the Tuning Proposal. A team-wide choice drafts the
  `.ws-workflow/config.json` edit from `config.list`'s repo information and
  proposes an ordinary commit (Decision 3).
- Revise what conflicts with Decisions 3 and 5. The Scope invariant "Tune
  only through catalog writer tools" gains the repo-file commit as the
  one non-catalog path. The Surface invariant "any request that does not
  map to one of this playbook's handlers is not yet supported" and
  `judge: tune-target` no longer send a multi-knob or vague weight request
  to "unsupported axis"; that request goes to the knob-explanation path.
  The Tuning Proposal template gets a form for the repo-file commit,
  because its `writer:` field assumes a catalog tool.
- `agents-plugin/skills/lead-tune/SKILL.md` and its wsflow shim: widen the
  description per Decision 9.
- Apply `skill-authoring.md`'s invariant checklist to every changed line.
- Update the idea ticket `261001-bug-config-scope-gaps-after-review-opt-in`
  with one line recording that gap 1 moved here.

Verification: plugin package tests and skill-shim drift tests pass after
wsflow and pi regeneration; a fresh-reader audit of the revised
`lead-tune.md` (per `skill-authoring.md`) confirms it contains no
complaint-specific script and no restated value semantics that duplicate
`config.list`.

### Result (c561d3328) - 2026-10-01

- `agents-plugin/rsrc/lead-tune/lead-tune.md` rewritten. The Sage-only posture
  handler and the `review_phase` routing line are replaced by `On: explain
  knobs`, which reads `config.list` descriptions and current values, explains
  the knobs a request bears on with their exact value words (keeping the
  off/ask/auto gloss as skip/recommend/require), explains the session,
  project, repo, and global scopes, and suggests one. `On: tune scalar knob`
  uses that scope guidance and confirms the scope with the user. The new
  `On: commit repo-scope setting` drafts the `.ws-workflow/config.json` edit
  from `config.list`'s `repo_scope` block and per-knob `repo_key`, then
  proposes an ordinary commit through the new `Repo-Scope Proposal` template
  (knob/file/change/commit). `On: tune prompt override` routes a repo-scope
  choice to that handler.
- Invariants revised: the Scope invariant admits the repo-file commit as the
  one non-catalog path; confirmation covers storage scope; only an explicitly
  chosen value is written. The Surface invariant and `judge: tune-target` send
  a multi-knob or vague weight request to the explain path instead of
  "unsupported axis", which is now a one-line fallback. `judge:
  proactive-propose` is unchanged. The Doctrine section was removed as
  duplicated value semantics.
- `agents-plugin/skills/lead-tune/SKILL.md` and the wsflow shim descriptions
  widened to weight and review-posture requests. wsflow and pi rsrc mirrors,
  the rsrc manifests, and the skills manifest were regenerated.
- The idea ticket `261001-bug-config-scope-gaps-after-review-opt-in` records
  that gap 1 moved here (Decision 11).
- Fresh-reader audit (per `skill-authoring.md`): complaint-specific scripts,
  duplicated value semantics, and the Doctrine section were fixed; the gloss
  (Decision 6) and the unchanged proactive-propose judge are intentional.
- Review round 1 (correctness + test) raised six findings, all fixed in
  eab5d417d: `assigneeFeature` now returns resolver errors to
  `tickets.query` and `tickets.create_empty`; `config.list` publishes
  `repo_key` (`prompt.<point>.<claude|codex|pi|all>` for prompt knobs) and
  lead-tune drafts from it; a `config.tune` echo failure after a successful
  write is reported as a warning, not an error; the scalar handler suggests a
  scope instead of defaulting to the declared one; new tests cover
  `workflow.lang` via playbook.read, fail-loud at the playbook readers and
  `tickets.query`, and `Resolver.Check`'s project and global branches. Round
  2 verified all six fixed with no new findings.
- Verification: `go test ./... -count=1` in `agents-plugin-tool`,
  `python3 -m unittest discover agents-plugin/tests`, and
  `python3 -m unittest discover agents-plugin-wsflow/tests` passed on
  eab5d417d; the round-2 targeted run
  (`-run 'RepoScope|ConfigTune|ConfigList|LeadTune|WeightLever|ResolverCheck'`)
  passed.
