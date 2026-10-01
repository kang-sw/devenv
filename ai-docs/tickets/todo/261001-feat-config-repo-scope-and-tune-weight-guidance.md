---
title: "Repo-scope config that applies, and lead-tune guidance for workflow weight"
related:
  261001-bug-config-scope-gaps-after-review-opt-in: absorbs its gap 1 (repo-blind resolvers); gaps 2 and 3 stay there
  261001-feat-review-phase-lite-default: its off|lite|full review_phase is one of the levers lead-tune explains
  261001-feat-opt-in-design-and-phase-review: introduced the review knobs whose weight lead-tune now explains
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
    resolver-backed and not global-only; `agents.tier`, `prompt.*`, and
    global-only knobs do not (`Resolver.Get` skips the repo overlay for
    `GlobalOnly` items, and `loadRepoConfig` contributes only `overrides`).
    The flag Decision 2 publishes comes from this rule in the registry.
11. **Scope of the absorbed gaps.** This ticket takes gap 1 of
    `261001-bug-config-scope-gaps-after-review-opt-in`; that idea ticket
    records the move. Gap 2 (inherited session values shown as the child's
    own) and gap 3 (CLI `ticketsMove` without builtin defaults) stay there.

## Constraints

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

## Phases

### Phase 1: repo scope that applies, published by config.list

- Add one constructor that takes a session key, looks up its canonical
  worktree root, and returns a resolver with that root as `RepoRoot`
  (empty for a keyless call or unknown key). Switch every resolver-backed
  reader in `internal/mcp` to it, including the Sage readers in
  `tickets.sage_gate`, `tickets.create_empty`, and `tickets.move`, the
  worktree-pool readers, and the existing `RepoRoot` call sites
  (`resolveReviewPhase`, `assigneeFeature`, `config.list`). Readers that
  intentionally resolve without session config (for example prompt-override
  or language resolvers with no repo-capable item) may stay as they are;
  the worker names each one it leaves and why.
- `config.list`: emit the repo-scope file path resolved from the session
  root (or that none applies), the `{"overrides": {"<knob>": "<value>"}}`
  shape, and a per-knob repo flag from the Decision 10 rule.
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
global-only knob); (vi) existing golden description tests updated. Full
`go test ./...` for `agents-plugin-tool`.

### Phase 2: lead-tune explains the weight knobs

- `lead-tune.md`: replace the Sage-only posture handler and the
  `review_phase` routing line with knob knowledge per Decision 5: how to
  read `config.list` descriptions and current values, explain the knobs a
  request bears on with their exact value words (Decision 6), explain
  scopes (Decision 7), and write only the user's explicit choice through
  the Tuning Proposal. A team-wide choice drafts the
  `.ws-workflow/config.json` edit from `config.list`'s repo information and
  proposes an ordinary commit (Decision 3).
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
