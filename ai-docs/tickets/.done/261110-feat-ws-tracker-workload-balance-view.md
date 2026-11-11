---
title: "Tracker: a lead-side workload balance view of current load per member"
parent: 261110-epic-ws-tracker-team-feedback
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 71d4d8c815e534cd
sage-review-completeness-reviewed: 71d4d8c815e534cd
completed: 2026-11-11
---

# Tracker: workload balance view

## Background

From the team introduction feedback (user, 2026-11-10 whiteboard, topic
"tracker 팀 피드백"): leads and managers asked to monitor the balance of
work across team members. The interest was partly about performance
reviews. A per-member roll-up already exists, but narrower than this
ticket's: the view model's `my.team` list (`Team` struct and the team walk,
`agents-plugin-tool/internal/wstracker/view.go#L310-L320`,
`agents-plugin-tool/internal/wstracker/view.go#L906-L949`) gives each member
below the viewer open, pending, and waiting ids plus done, blocked, and stale
counts, and the Items panel's `team` scope renders it as group-by-person
headers (`listTeam`,
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L1092-L1103`). It
reaches only three levels below the viewer (`reach(v, 3)`, `view.go#L906`),
has no overdue count and no admin-sees-everyone path, and its counts are not
broken out per status. The view model already
derives per-assignment status and a `stale` flag: a live assignment in
doing or blocked with no status write for `DefaultStaleDays` (3) days
(`internal/wstracker/view.go`, `state.go`).

## Decisions

1. **Visibility follows the org tree (user, (613) a).** A lead sees their
   whole subtree, direct and indirect reports. An admin sees every member.
   Rejected: direct reports only (b); every member sees everyone (c).
2. **The first scope is the current load only (user, (614) a).** Per member:
   live assignment counts by status, stale count, and overdue count (due
   date passed and not done). Rejected: historical throughput and cycle-time
   statistics in the same ticket (b). They are deferred, because which
   numbers a review should count is sensitive and needs its own discussion;
   they can be derived later from the ledger.
3. **Extend the existing team roll-up (technical, (260)).** The Items
   panel's `team` scope and the view model's `Team` roll-up grow to the
   whole subtree (an admin sees every member), with live counts per status,
   stale, and overdue. Rejected: a separate view, which computes the same
   roll-up twice. Cost: the `my.team` output and its golden cases change.
4. **idea counts separately (technical, (261)).** idea assignments show as
   their own count and stay out of the load total, matching
   `261110-feat-ws-tracker-idea-status-and-assignment-invariant` Decision 3.
5. **Creator-fallback holdings are not load (user, (262) a).** They show as a
   separate "fallback-held" count, since fallback means nobody has taken the
   work yet. Rejected: counting them as load ((262) b), which inflates a
   lead's load.
6. **Drill-down through filters (technical, (263)).** Clicking a count
   filters the Items list to that member and status. Rejected: a new
   drill-down list.
7. **Count definitions (technical, review-raised).** The load total counts a
   member's live assignments that are neither idea nor done (todo, doing,
   blocked) and not fallback-held (Decision 5); pending assignments are not live and stay out. Stale and
   overdue are subsets of the load total. Overdue uses the item's due date
   (passed, assignment not done). "Admin" is the roles admin of the store.
   Members an admin sees outside their own subtree show in a separate
   group, with depth measured from the org-tree root, so `Team.Depth` keeps
   its meaning (distance from the viewer) inside the viewer's subtree.
   Rejected: counting stale and overdue apart from the load total, which
   double-counts one assignment.

## Constraints

- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- `261110-feat-ws-tracker-superior-settable-status-and-new-statuses` adds
  statuses (dropped, in-review); whichever of the two tickets lands second
  folds them into the per-status counts (dropped stays out of the load
  total, as it is excluded from roll-up there).
- Golden files: view JSON fields carry no `omitempty`, so a new `Team` field
  changes every file under `testdata/golden`; regenerate them from the
  reference engine after each merge, never merge them by hand, and give new
  case files a distinct name prefix.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261110-epic-ws-tracker-team-feedback (2026-11-10, Cross-Child Decisions): "Workload visibility follows the org tree: a lead sees their whole subtree (direct and indirect reports), and an admin sees everyone ((613) a). Any child that exposes per-member work data ... uses the same visibility." — bearing: supports
- 25d11857 (2026-11-10, commit): "Children carry only the settled answers: (613) a subtree visibility, (614) a current load only ... Remaining details are listed as to-settle-at-promotion Constraints" — bearing: supports
- c536206a (2026-11-10, commit): "The team walk excludes topic records from every list and count (open, pending, waiting, done, last) ... since Decision 6 keeps a held topic out of every status-based aggregate" — bearing: constrains
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Decisions): "Idea items are left out of progress roll-ups (user, (551))" — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go ... so the strings cannot drift between the two." (view.go derives under golden parity with the mockup engine) — bearing: constrains
- 76c9201c (2026-11-10, commit): "Self avatars now carry .me at every site (the team tree, team workload groups, menus ...)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/view.go, ai-docs/ref/tracker-mockup/engine.js, testdata/gen-golden.js and golden, agents-plugin-tool/internal/mcp/tracker_render.go (`team` output, L68-L72), agents-plugin-tool/internal/wsdashboard/assets/tracker.js, agents-plugin-tool/internal/wsi18n/tables/*.json |
| scope.surface | public-interface | View JSON my.team / Team struct consumed by the dashboard and tracker.query is the extended surface |
| scope.new_public_symbol | yes | new `Team` fields in view.go (Decision 3); names chosen in planning |
| scope.new_type_contract | yes | the `my.team` view JSON gains per-status, overdue, idea, and fallback-held counts |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/team_test.go, golden_test.go with testdata/golden, internal/wsi18n/keys_test.go |
| complexity.reuse_points | confirmed | Team roll-up and team walk in view.go#L906-L949, listTeam in tracker.js#L1092-L1103, Assignment.Stale in view.go#L497 |
| complexity.side_effect_risk | moderate | widening the team walk beyond reach depth 3 and adding admin scope changes existing my.team output and its goldens |
| risk.correctness | moderate | overdue, idea, topic, and creator-fallback inclusion rules must agree with existing roll-up exclusions |
| risk.fit | low | extends the existing team scope and roll-up (Decision 3) |
| risk.test | moderate | view.go output is pinned by goldens generated from ai-docs/ref/tracker-mockup/engine.js, so parity must be kept |
| risk.security_or_contract | moderate | per-member load data visibility must follow the org-tree rule; admin-sees-all is a new disclosure path |

## Phases

### Phase 1: current-load view

Scope:
- Decisions 1-7 in the view model (`internal/wstracker/view.go` `Team`
  roll-up), the reference engine (`ai-docs/ref/tracker-mockup/engine.js`)
  and its golden cases, the `team` output of `tracker.query`
  (`internal/mcp/tracker_render.go`), and the dashboard's `team` scope
  (per-member counts, the outside-subtree group, and the drill-down).

Verification:
- Tracker tests: a lead sees members beyond three levels below; an admin
  sees every member; a non-lead sees no one else; counts per status, stale,
  overdue (due passed, not done), idea, and fallback-held come out right,
  with idea and fallback-held outside the load total and stale and overdue
  inside it; an admin's outside-subtree members form their own group.
- A render test of the `tracker.query` `team` output; the i18n key test
  (`internal/wsi18n/keys_test.go`) covers the new strings.
- Golden cases from the reference engine agree with the Go domain.
- A browser check of the team scope and a count's drill-down.

### Result (4addb08c1) - 2026-11-11

Landed in eb01cb73f (domain, engine, goldens, `tracker.query` render) and
4addb08c1 (dashboard team scope, i18n).

- `my.team` rows: `{id, depth, outside, open, pending, waiting, counts,
  load, stale, overdue, fallback, last}`; `done` and `blocked` are replaced
  by `counts.done` / `counts.blocked`. The walk covers the viewer's whole
  subtree (depth from the viewer); an admin also gets every other live
  member as the outside group, walked from the org-tree roots (depth from
  the root, a root 0), with a branch no root reaches rooted at depth 0.
- Counts leave out creator-fallback holdings and held topics; load is the
  live counts neither idea nor done (awaiting included, by Decision 7's
  "neither idea nor done" rule); stale and overdue (due before the UTC date
  of now; off with no clock) are inside load; fallback counts live fallback
  holdings not done. Every live, non-done holding lands in exactly one of
  load, idea, or fallback.
- `engine.js` and `view.go` move in lockstep; goldens regenerated from the
  engine; new case `workload-balance` (deep subtree, admin inside the tree,
  second root, every count).
- `tracker.query` team scope: a legend line, per-person load with a
  per-status breakdown, stale/overdue/idea/fallback-held/done, and an
  "outside my subtree" heading.
- Dashboard: group headers show the counts as buttons; a click narrows the
  scope to that person's rows of that count (panel display state, list
  mode), the same count or the clear control clears it; outside members
  render under their own heading with root-depth indent and no `L` label;
  the scope chip sums load, stale, overdue, unaccepted and adds an
  "outside your subtree" count for an admin. 15 new keys in all 11 tables,
  `js.tk.team.done` / `js.tk.team.blocked` removed, mockup `i18n-en.js`
  regenerated.

Verification (remote `ws-rgo`): `test ./internal/wstracker/` ok (golden
parity, new `workload_test.go`, updated scenario/topic tests);
`test ./internal/mcp/ -run Tracker` ok (new `tracker_render_test.go`);
`test ./internal/wsdashboard/ ./internal/wsi18n/` ok (new
`tracker_team_drill_test.go` executed under node, i18n key and mockup-table
tests). Browser check with headless Chromium on the mockup as an admin
lead: team scope and a count's drill-down rendered as intended
(screenshots inspected). Lite review: no Critical or Important findings.

#### Edition (a7c526dae) - 2026-11-11

Lead veto: `load` counts todo, doing, and blocked only (Decision 7's
parenthetical); awaiting keeps only its own per-status count and stays out
of load and overdue, matching the superior-settable-status ticket's rule
that in-review (awaiting's successor) and dropped stay outside the load.
Applied in `view.go` (`teamLoad`), `engine.js` and `tracker.js`
(`TEAM_LOAD`), the `tracker.query` head (awaiting listed after stale and
overdue, not in the load breakdown), the dashboard head and drill
predicate, and the load tooltip in all 11 tables. The workload-balance case
gives the awaiting item a passed due date; goldens regenerated from the
engine; `TestTeamRowCounts` now pins load 4 and overdue excluding awaiting.
Verification (remote `ws-rgo`): `test ./internal/wstracker/
./internal/wsdashboard/ ./internal/wsi18n/` ok; `test ./internal/mcp/ -run
Tracker` ok.
