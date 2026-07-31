---
title: ws-dashboard drop sweep — archive tag, git-surface teardown, doc and board removal
sage-review-design: completed
related:
  260730-research-ws-dashboard-drop-for-orca: origin decision this sweep executes; Phase 3 converts its Open Decisions into recorded outcomes and corrects its branch claims
  260730-research-orca-plugin-ws-workflow-surface: successor concern; Phase 3 records four verified plugin-surface findings into it
  260605-epic-ws-playbook-factory-pivot: its M3 non-scope premises dashboard survival; Phase 3 settles the port-vs-remove decision it deferred
  260605-research-ws-native-subagent-pivot: migration anchor carrying three layers of dashboard-disposition history; Phase 3 appends the terminal supersession
  260726-research-spec-planned-marker-management-cost: its 2026-07-26 citation of the dashboard spec goes stale; the marker mechanism itself was already retired 2026-07-28
  260514-epic-ws-web-dashboard-mvp: the board this sweep drops, together with eight of its nine children
  260523-bug-worktree-local-index-missing: the ninth child, kept and re-homed as a ws-core defect rather than dropped
  260726-refactor-retire-spec-planned-marker-mechanism: already retired the planned-marker mechanism on 2026-07-28, which falsifies the original premise for touching the marker research ticket
  260725-idea-retire-ticket-focus-root-regen: overlapping _index.md work, deliberately not waited on
sage-review-completeness: completed
---

# ws-dashboard drop sweep — archive tag, git-surface teardown, doc and board removal

## Background

`260730-research-ws-dashboard-drop-for-orca` records the decision to stop
ws-dashboard development because Orca ADE supersedes its generic half. That
ticket froze forward investment but left disposition open. The user then chose a
full **sweep**: close the pending PRs, delete the code and its documentation, and
clear the board, keeping exactly one consolidated recovery point.

The ws/wsflow plugin is unaffected and remains the project's core deliverable.
Orca runs the Claude Code / Codex CLIs in ordinary terminals, so ws skills load
unchanged inside it.

## Decisions

**Archive as an annotated tag, not a branch.** `archive/ws-dashboard` points at a
commit whose tree is byte-identical to `main` and whose parents are the tips of
every dashboard line. Rejected: an `ws-dashboard-archived` **branch** — a mutable
ref that stays in `git branch -a`, the GitHub branch dropdown, and PR base
candidates, and that stale-branch automation may delete. Rejected: **merging the
dashboard line into `main`** — 1058 commits would flood `git log main` for code
that is no longer developed. A tag's only cost is that `git fetch` does not
auto-follow it (the archive commit is a descendant of `main`, so nothing
reachable from `main` points at it); `git fetch --tags` is required, which the
tag message and the drop ticket both record. The user accepted this explicitly:
"어차피 ws-dashboard를 일부러 되살리려는 사람이 아닌 이상 오히려 태그는
안 딸려들어오는 게 낫기도 하고요."

**Tag-push confirmation is a hard gate.** No PR, branch, file, or ticket may be
deleted until `git ls-remote origin refs/tags/archive/ws-dashboard` returns the
tag. Closing a PR alone leaves commit reachability dependent on GitHub's
`refs/pull/N/head` retention, which is an implementation detail rather than a
guarantee; deleting a branch removes it entirely.

**Artifacts are deleted outright.** This sweep does not move dashboard artifacts
to `ai-docs/.old/`: parking dead documents inside the live document tree keeps
them in doc-coverage alarms and verification crawls. Git plus the archive tag
are the recovery path.

**The harness research is recovered, and moved out of `mental-model/`.** Blob
`7ae574fa` (162 lines, identical on every dashboard branch, last touched
2026-07-13) holds fixture-verified Codex app-server findings obtained against
`codex-cli 0.144.1`: Passthrough `thread/compact/start`, `thread/fork`,
`skills/list`, `turn/steer`, and `thread/goal/*`, with `thread/rollback`
confirmed deprecated, turn-coarse, and non-reverting for file changes. That
research is valid independently of any UI and is expensive to re-derive. It lands
at `ai-docs/ref/agent-harness-capability-tiers.md`: `ref/` because no live domain
remains to modify once `ws-dashboard/` is gone, and renamed off "dashboard"
because the content is `(harness, capability)` tiering, not dashboard behavior.

**No version bump.** This is not an implementation-branch merge into an
integration branch or `main`, and it touches no plugin edition point.

## Constraints

- The archive commit's tree must equal `main`'s exactly; verify with
  `git diff --quiet main <archive-commit>` before tagging.
- `implement/260525-feat-ws-dashboard-sqlite-agent-activity-source` exists
  **locally only** (1 commit, no remote). It must be an archive parent or it is
  lost on local deletion.
- Close PRs **before** deleting their branches; deleting a branch auto-closes its
  PR and forfeits the comment that records the tag name and revive command.
- `260514-epic-ws-web-dashboard-mvp` and the eight `parent:`-linked children being
  dropped must move to `.dropped/` in the same commit so no orphaned `parent:`
  appears. The ninth child, `260523-bug-worktree-local-index-missing`, is kept, so
  its `parent:` must be stripped in that same commit for the same reason.
- Branch discovery must be **path-based**, not name-pattern-based. This session's
  first survey used `*dashboard*` / `*ws-web*` globs and silently missed
  `impl/nav-row-two-line-open-state-phase1`, which carries 209 commits touching
  `ws-dashboard/`. Enumerate candidates with
  `git rev-list --count main..<ref> -- ws-dashboard/` over every ref instead.
- Shell state does not persist between tool calls: capture the four parent SHAs
  from `git rev-parse` output and pass them explicitly to the `commit-tree` call.
- **Snapshot changes stop the sweep.** After Phase 1's fetch, capture the exact
  SHA of every Phase 2 branch-deletion target. Immediately before each
  destructive remote action, refresh and re-enumerate the path-based candidate
  set; delete only through an exact-SHA lease. If a target moved or a new
  non-exempt ref carries `ws-dashboard/` commits, stop before deletion. Do not
  move a pushed `archive/ws-dashboard` tag; a later archive needs a new explicit
  user decision.
- `ws-mcp-release.yml` triggers on tags `v*` only, so pushing `archive/*` does
  not fire the release pipeline. All 108 existing tags are `vX.Y.Z`, and no tag
  named `archive` exists, so there is no D/F ref conflict.

## Prior Art

Branch facts established this session, correcting the drop ticket's own text:

- `origin/impl/helper-liveness-probe` (1058 commits absent from `main`, PR #8
  head) **contains** `velvet-arbor-quill`, `copper-heron-vale`, and
  `ws-dashboard-dev`. It is the single tip covering the whole development line.
- `velvet-arbor-quill` is neither the latest tip nor dangling: PR #4 already
  merged it into `ws-dashboard-dev`. It is remote-only, not a local branch.
- `impl/nav-row-two-line-open-state-phase1` also carries the dashboard line —
  local 730 commits ahead of `main` with 209 touching `ws-dashboard/`, remote
  724/207 — and is contained in `helper-liveness-probe`, so archive reachability
  holds. It is named neither `*dashboard*` nor `*ws-web*`, which is why the
  name-pattern survey missed it.
- Three refs sit outside the archived line:
  `implement/dashboard-server-scoped-forwarding-phase-7` (44 commits, 20 touching
  `ws-dashboard/`, local+remote),
  `implement/260525-feat-ws-dashboard-sqlite-agent-activity-source` (1 commit,
  local-only), and `origin/discuss` (13 commits, 3 touching `ws-dashboard/`).
  `origin/discuss` is ws-core work and is **not** deleted by this sweep, so its
  three dashboard commits stay reachable without being archive parents.
- The local `dashboard` branch is **unrelated** despite its name — ws core work
  through 0.36.1 — and is fully contained in `main`, so deleting it loses
  nothing. It misled this session's first survey and is removed in Phase 2.
- Only one worktree exists (`main`), so nothing blocks branch deletion.
- `ws-dashboard` appears in neither `.github/workflows/` nor
  `agents-plugin-tool/scripts/`, so removal does not affect CI or release.

## Spec Impact

- **Target spec area:** `ai-docs/spec/ws-web-dashboard/` in full — a 1,305-line
  index carrying 72 `{#YYMMDD-slug}` anchors. Enumerating them in `spec-remove:`
  adds no recovery value when the whole domain file is deleted.
- **Expected caller-visible change:** the entire dashboard surface is removed —
  daemon HTTP/WS API, browser UI, linked-server gateway and remote forwarding,
  Activity Console, document viewer/editor. Nothing in ws replaces it; Orca ADE
  covers the generic half, and the ws/wsflow plugin surface is untouched.
- **Contract-first spec:** no. The closeout is deletion of the spec domain
  itself, performed in Phase 3.

## Phases

Phase 2 depends on Phase 1's confirmed remote tag — that gate is the whole safety
design. Phase 3 depends on Phase 2 having removed the code, so documentation is
never left describing a tree that is already gone.

### Phase 1: Archive the dashboard line and recover the harness research

Establish the single recovery point and land the one asset worth keeping. Nothing
is deleted in this phase, so it is fully reversible.

1. `git fetch origin --prune`, enumerate every `refs/heads` and `refs/remotes`
   ref with `ws-dashboard/` commits beyond `main`, and capture exact SHAs for
   every Phase 2 branch-deletion target. Also capture SHAs for `main`,
   `origin/impl/helper-liveness-probe`,
   `implement/dashboard-server-scoped-forwarding-phase-7`, and
   `implement/260525-feat-ws-dashboard-sqlite-agent-activity-source`.
2. Recover
   `origin/impl/helper-liveness-probe:ai-docs/mental-model/ws-dashboard-agent-harness.md`
   to `ai-docs/ref/agent-harness-capability-tiers.md`. Rewrite the frontmatter to
   `ref/` conventions and drop the references that this sweep kills:
   `sources: ws-dashboard/` and `related: ws-web-dashboard`. Preserve the
   four-tier Passthrough/Overlay/Hack/Unavailable definitions and the
   fixture-verified Codex column verbatim.
3. Build the archive commit with `git commit-tree` using `main`'s tree and the
   four captured SHAs as parents, `main` first.
4. Verify `git diff --quiet main <archive-commit>` succeeds.
5. Create annotated tag `archive/ws-dashboard` whose message records why the line
   was archived, the drop ticket stem, and the revive command
   `git fetch --tags && git checkout -b revive archive/ws-dashboard`.
6. Push the tag and confirm with
   `git ls-remote origin refs/tags/archive/ws-dashboard`.

**Verification:** the tree-equality check passes; `ls-remote` shows the tag; a
scratch `git checkout -b` from the tag reaches the dashboard code tree.

### Result (73fc63e) - 2026-07-31

- Published annotated `archive/ws-dashboard` at `bc2e54e` and confirmed its
  remote tag object and peeled commit. Its tree equals pre-sweep `main`, and
  its ordered parents preserve `main`, helper-liveness, server-scoped
  forwarding, and the local-only SQLite activity tip.
- Recovered the reusable four-tier agent-harness taxonomy and fixture-verified
  Codex evidence as `ai-docs/ref/agent-harness-capability-tiers.md`, with the
  retired Dashboard-specific Overlay actor neutralized to an integration layer.
- Added `ai-docs/ref/verify-dashboard-archive-recovery.sh`; it proves the tag
  and parent invariants, remote reachability, fresh checkout, and recovered
  evidence comparison before Phase 2 may delete source refs.

### Phase 2: Tear down the dashboard git surface and code tree

Irreversible. Do not start until Phase 1's `ls-remote` confirmation succeeded.

1. Close PR #8 (`impl/helper-liveness-probe` → `copper-heron-vale`), then PR #7
   (`copper-heron-vale` → `ws-dashboard-dev`). Each closing comment records the
   tag name, the revive command, and the drop ticket stem. No open PR targets
   `main`.
2. Delete remote branches: `impl/helper-liveness-probe`,
   `impl/nav-row-two-line-open-state-phase1`,
   `goal/ws-dashboard-dev/copper-heron-vale`,
   `goal/ws-dashboard-dev/velvet-arbor-quill`, `ws-dashboard-dev`,
   `implement/dashboard-server-scoped-forwarding-phase-7`. Immediately before
   each deletion, fetch and repeat the path-based enumeration; delete with an
   exact-SHA lease from Phase 1. A moved target or a new non-exempt candidate is
   a stop condition, not a reason to force-push or update the archive tag.
3. Delete local branches: `ws-dashboard-dev`,
   `impl/nav-row-two-line-open-state-phase1`,
   `implement/dashboard-server-scoped-forwarding-phase-7`,
   `implement/260525-feat-ws-dashboard-sqlite-agent-activity-source`, and
   `dashboard`.
4. `git rm -r ws-dashboard` (110 tracked files), then remove the ignored bulk
   from disk — `target/` at 9.0G and `frontend/node_modules` at 207M, out of
   9.2G total.

**Verification:** both PRs report closed; `ws-dashboard/` is absent; no workflow
or release script referenced it, so CI needs no change. Verify branch teardown by
**property, not by name list** — iterate `refs/heads` and `refs/remotes` and
confirm `git rev-list --count main..<ref> -- ws-dashboard/` is `0`, with
`origin/discuss` as the only accepted exception. Checking the six/five hardcoded
names would have passed even while `impl/nav-row-two-line-open-state-phase1`
survived, which is exactly how the original survey missed it. Scope the iteration
to those two namespaces deliberately: `git for-each-ref` also walks `refs/tags`,
where `archive/ws-dashboard` carries the entire dashboard line by construction and
would read as a teardown failure. That tag is the one ref that must violate the
property.

### Phase 3: Remove dashboard documentation and reconcile the board

1. Delete `ai-docs/spec/ws-web-dashboard/` and
   `ai-docs/mental-model/ws-web-dashboard.md` (223 lines). Then remove the
   references that would be left describing a deleted tree:
   - `ai-docs/mental-model.md` — the domain-map row pointing at both deleted
     files, and the domain-table row for `ws-web-dashboard`. Its other
     dashboard-sounding row is `developer-environment-tools` and must stay.
   - `ai-docs/mental-model/named-agent-runtime.md` — the `ws-web-dashboard:`
     `related:` key, and the Activity-Console coupling clause in its worktree
     scoping rule. Its Windows `replaceFile` rule cites the dashboard only as an
     example of a process holding a destination file open: keep the rule and its
     retry contract, drop the dashboard as a live example.
   - `README.md` — the prose naming the dashboard scaffold and the tree-listing
     entry for `ws-dashboard/`.
2. In `ai-docs/_index.md`, remove the dashboard scaffold inventory line, the
   dashboard spec table row, and the 13 ticket rows for dropped tickets. The
   14th dashboard row belongs to the kept
   `260523-bug-worktree-local-index-missing`: keep the row but reword its
   description, which currently reads "dashboard-managed propagation". Removing
   rows by hand is deliberate: `260725-idea-retire-ticket-focus-root-regen`
   proposes retiring that section wholesale, but its landing time is unknown.
3. Move 13 dashboard tickets to `.dropped/` — the epic
   `260514-epic-ws-web-dashboard-mvp` plus eight of its nine `parent:`-linked
   children in one commit, and the four unparented dashboard tickets
   (`260514-research-ws-web-dashboard-direction`,
   `260524-research-ws-dashboard-react-aria-ui-primitives`,
   `260524-research-ws-dashboard-visual-design-system-refresh`,
   `260729-bug-dashboard-submodule-workroot-empty-projection`).
   `260525-feat-ws-dashboard-server-scoped-operation-forwarding` drops with the
   rest — its 44-commit implementation now lives only in the archive tag. Per
   ticket conventions, moves need no cross-link updates.

   **Keep three.** `260729-feat-workflow-manual-submodule-detection` (its
   substance is `workflow_manual`, not the dashboard) and
   `260523-bug-implement-merge-target-discovery` (a nested-merge issue that merely
   surfaced during dashboard dogfooding) need no edit — neither is a dashboard
   ticket nor an epic child.

   `260523-bug-worktree-local-index-missing` **is** an epic child and is kept
   deliberately: its `## Background` records a live ws-core defect — ignored local
   workflow context such as `ai-docs/_index.local.md` is not copied into Git
   worktrees, so worktree-based runs lose approved SSH hosts and browser-gate
   notes and agents re-derive setup or pick wrong defaults. Only its `## Direction`
   ("Treat this as a dashboard/workroot management problem") dies with the sweep.
   Re-home it in the same commit, and treat this list as exhaustive:
   - strip `parent:` and `related-mental-model: ws-web-dashboard`;
   - retitle `title:` and the H1 from "dashboard-managed worktree local context"
     to `worktree local context propagation` — otherwise the ticket keeps
     asserting the disposition this step removes, and diverges from the `_index.md`
     row that step 2 rewords. The date-prefixed stem is already surface-neutral
     and does not change;
   - replace `## Direction` with the statement that the mechanism is undecided and
     the dashboard surface it assumed no longer exists;
   - keep `## Background`'s third paragraph but reframe it. As written ("Adding
     broad worktree-management APIs to ws core would also pull workflow
     orchestration into general Git workspace management") it is directional, not
     observational, and once `## Direction` is gone it becomes the only surviving
     direction-shaping text — arguing against the only remaining owner. Retain it
     as a constraint on a *broad* API surface, not as a rejection of ws-core
     ownership of the defect.

   Rejected: dropping it and opening a fresh ws-core ticket — ticket conventions
   reserve that for a fundamental concept change, and here the defect is unchanged
   while only the proposed solution surface disappeared.
4. Reconcile `260605-epic-ws-playbook-factory-pivot`: its M3 non-scope currently
   premises dashboard survival ("retained as a web-tmux surface", "M3 keeps it
   compiling", "port-vs-remove deferred to a dashboard idea ticket"). This sweep
   settles that deferred decision and removes the compile-maintenance burden.
5. Append a terminal supersession section to
   `260605-research-ws-native-subagent-pivot`, following the file's existing
   `### Supersede:` pattern. It carries three layers of dashboard-disposition
   history — deprecation to a TUI, then retention, then "port, not strip" — and
   AGENTS.md directs every architecture and migration task to read it, so a stale
   premise there propagates into future sessions.
6. In `260726-research-spec-planned-marker-management-cost`, correct only the
   stale citation: its `## Measured 2026-07-26` section points at
   `ws-web-dashboard/index.md:231`, which this sweep deletes. Do **not** write that
   the sweep removed the project's last live planned-marker or that it strengthens
   the retirement case — the 🚧 mechanism was already retired on 2026-07-28 by
   `260726-refactor-retire-spec-planned-marker-mechanism` (now in `.done/`),
   `ai-docs/spec/` holds zero live markers today, and that ticket already carries an
   "Answered by implementation" section recording the landing. The original premise
   for this step was a stale reading of a dated measurement.
7. Update `260730-research-ws-dashboard-drop-for-orca`: convert Open Decisions
   into recorded outcomes, correct "It stays dangling and unmerged" (PR #4 had
   already merged `velvet-arbor-quill` into `ws-dashboard-dev`, and two further
   tips sat above it), and record the archive tag name and revive command.
8. Record four verified findings in
   `260730-research-orca-plugin-ws-workflow-surface`: (a) "What Does Not Come
   Back" is over-scoped — the loss covers Orca-owned sessions only, since Codex
   app-server exposes compact/fork/steer/goal as Passthrough for ws-owned
   sessions; (b) `storage` limits of 256 KB per value and 5 MB total cannot hold a
   board of ~100 live tickets plus its `related:` graph in one value; (c)
   `contributes.events` exposes no filesystem-change event, leaving worker
   re-projection and panel refresh timing unspecified — possibly a harder blocker
   than the worktree-path friction the ticket names; (d) `settings:own` with a
   manually configured root is adequate for one to three personal projects, so
   Hack-tier reading of Orca's private state is avoidable. The markdown-board
   alternative raised in discussion is deliberately **not** recorded — it was not
   settled.
9. Remove the machine-local dashboard guidance, which `git rm` does not reach
   because it is ignored: `ai-docs/ref/ws-dashboard-playwright.local.md` (a
   workstation runbook whose canonical gate is
   `cd ws-dashboard/frontend && npm run test:browser`), and the dashboard dogfood
   and Playwright-gate references in `ai-docs/_index.local.md`. This produces no
   commit; it exists so the "artifacts are deleted outright" decision is not
   silently limited to tracked files.

**Verification:** `ws/spec_index.verify` passes; the ticket graph shows no
dangling `parent:`; `_index.md` names no dashboard artifact beyond the kept
ticket's reworded row.

The residual-reference search needs an explicit exemption set, because a bare
repo-wide `ws-web-dashboard` search under `ai-docs/` **cannot** pass:
`ai-docs/.plans/` holds roughly 105 dashboard plan and brief files, and several
live tickets legitimately name the domain — `260605-research-ws-native-subagent-pivot`,
`260605-epic-ws-playbook-factory-pivot`,
`260726-research-spec-planned-marker-management-cost`,
`260730-research-orca-plugin-ws-workflow-surface`,
`260713-workset-workflow-dogfood-bugs`,
`260517-bug-ws-agent-empty-result-after-tool-use`, and
`260523-bug-implement-merge-target-discovery`. Two more must be exempt because
this sweep edits them in place rather than dropping them, so they still name the
domain afterwards by design: `260730-research-ws-dashboard-drop-for-orca` (step 7
records the deleted spec path) and this sweep ticket itself, which is still in
`todo/` when the check runs. Exempt `.plans/`, `.done/`, `.dropped/`, and those
nine stems, then require zero hits elsewhere.

Run the same search for `ws-dashboard` **outside** `ai-docs/` as well — that is
what would have caught `README.md` — exempting `CHANGELOG.md`, whose lines 1232
and 1252 are shipped release entries announcing the daemon and the Rust workspace
scaffold. Those are historical record; rewriting them would falsify a published
changelog. After Phase 3, `README.md` and `CHANGELOG.md` are the only tracked
files outside `ai-docs/` that mention the dashboard, and only the former is
edited.

## Blocked (2026-07-30)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | 260523-bug-worktree-local-index-missing carries a ws-core defect that survives the dashboard, but the keep-list omits it and the 14-count cross-check forbids keeping it | important | missing |
| 2 | Phase 2 branch teardown omits impl/nav-row-two-line-open-state-phase1 (local 730 ahead / 209 dashboard commits; remote 724/207), leaving a mutable ref carrying the dashboard line | important | autonomous |
| 3 | Phase 3 step 6 rests on a falsified premise: the planned-marker mechanism was already retired 2026-07-28 and zero live markers remain in specs | important | autonomous |
| 4 | Doc removal enumeration misses ai-docs/mental-model.md, ai-docs/mental-model/named-agent-runtime.md, and README.md | important | autonomous |
| 5 | Phase 3 verification criterion is unsatisfiable because ai-docs/.plans/ holds 105 dashboard files, so it yields no signal | minor | autonomous |
| 6 | Machine-local dashboard docs (ai-docs/ref/ws-dashboard-playwright.local.md, _index.local.md) are unaddressed | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Dangling '13' self-correction has no referent inside this ticket | minor |
| 2 | Doc-reference verification scoped narrower than the removal itself (nothing checks outside ai-docs/) | minor |
