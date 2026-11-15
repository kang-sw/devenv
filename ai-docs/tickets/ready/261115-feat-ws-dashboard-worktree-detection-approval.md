---
title: "Dashboard session rail worktree detection and user approval"
related:
  261113-feat-ws-dashboard-session-rail-tree: "extends the landed session-rail worktree listing"
  261115-research-ws-effects-browser-unexpected-codex-child: "adjacent fixture-isolation evidence; no implementation dependency"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7538a0b753e92f3d
sage-review-completeness-reviewed: 7538a0b753e92f3d
---

# Dashboard session rail worktree detection and user approval

## Background

The session rail already lists repository worktrees and excludes ws pool entries through `railPooled` and `worktreePoolRoots` (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L25-L30,#L348-L350,#L474-L485,#L677-L684`; pool exclusion is already landed and remains unchanged). The owner-approved follow-up is to make non-pool worktree discovery explicit and reviewable without turning the dashboard into a shell-prevention mechanism.

Owner decisions `(847)=a` and `(856)=ok`, relayed by MAIN, settle the visible behavior below. The approved reference is tracked at [the HTML mockup](../assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html), with the four accompanying PNG captures in the same folder. Future verification must compare a result capture side by side with that HTML reference.

## Decisions

- The compatibility target is **T0 additive**. Add one optional admission field to the existing daemon-owned `rail-layout.json` document while keeping `schema_version: 1`; its read-modify-write path preserves unknown fields (`agents-plugin-tool/internal/wsdashboard/rail_layout.go:1-53, 342-345, 387-428`).
- A successful `/rail/worktrees/add` records the canonical repository-root/worktree-path identity with `origin: user` and admitted state (`agents-plugin-tool/internal/wsdashboard/workspace_routes.go:346-392`; `railworktreeadd.go:159-232`).
- A worktree found by Git that has no admission record is `origin: detected` and pending. The repository root is implicitly admitted without a record. Pool filtering runs first through the existing `railPooled`/`worktreePoolRoots` rule, so pooled paths never become pending (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L348-L350,#L474-L485,#L677-L684`; `agents-plugin-tool/internal/wsdashboard/railworktreedelete.go#L212-L230`).
- Admission identity is the pair of canonical repository-root path and canonical worktree path.
- Apply uses the existing same-origin dashboard write gate, revalidates the latest rail facts, writes persistent admission state, and wakes the existing rail generation/render path (`agents-plugin-tool/internal/wsdashboard/rail_layout_route.go:70-120`; `rail.go:37-120`).
- If a selected path was removed or its identity was replaced before Apply, Apply rejects that entry. Apply never deletes, removes, or otherwise alters a worktree on disk.
- Pending entries have approve/reject selection toggles; any selection reveals the Apply capsule. Owner `(1112)=a`, relayed by MAIN, confirms all eight recommendations `(1096)` through `(1103)` below.
- `(1096)` Clicking an already-selected toggle clears that selection. Rejected: keeping it selected, because reversible drafts match the approved reference.
- `(1097)` Approve and reject are mutually exclusive; selecting either clears the other. Rejected: allowing contradictory drafts and deferring their error to Apply.
- `(1098)` Include clear-all in the Apply capsule; it clears unsubmitted selections only. Rejected: requiring individual clearing when the reference offers a batch action.
- `(1099)` Include rejected-list undo returning an applied rejection to pending without changing disk. Rejected: irreversible classification or omitting the recovery control.
- `(1100)` Persist unsubmitted selections in browser-local storage across reload and rail refresh, separately from shared admission; revalidate them at Apply. Rejected: shared drafts or reload-discarded drafts.
- `(1101)` Persist successful rejection before copying. Clipboard failure leaves rejection persisted, visibly reports the failure, retains the investigation prompt and exposes explicit copy retry. Rejected: copy-before-write or claiming atomicity/rollback across browser clipboard and daemon persistence.
- `(1102)` Copy the cumulative rejected-worktree list, matching the visible collapsed list, rather than only the current Apply batch.
- `(1103)` A stale, invalid or unverifiable selection refuses the whole Apply batch without admission writes; retain drafts and identify stale rows for correction. Rejected: partial application with easily missed per-row outcomes.
- User-added worktrees remain admitted; unrecorded Git discoveries remain pending until Apply admits them.
- MAIN owner settlement `(888)=a` confirms that a removed and later recreated same path is a new detected/pending worktree; prune the admission record once the old path is confirmed gone.
- Apply admits approved entries and moves rejected entries into a collapsed rejected-worktree list.
- When rejected entries are applied, the dashboard copies the rejected-worktree list together with a prompt asking the agent to investigate unauthorized non-pool creation and report the finding to the user. Rejection does not delete or otherwise alter the worktree on disk.
- Detection and remediation are the scope; shell prevention or interception of worktree creation is not.
- `(1104)` Technical settlement, auto-confirmed from fact population and upheld by MAIN: use daemon-local handle-captured identity observations and fresh Git association checks rather than persisted inode/admin-path claims of durable incarnation. At existing successful viewer-gated reads, after pool filtering, capture directory identity and relevant Git association and maintain an observation revision. At Apply, reread Git membership once per affected repository and probe selected identities against their referenced observation; changed or unverifiable evidence refuses Apply with refresh/retry guidance under `(1103)`.
- Persist admission and confirmed-removal pruning in the optional schema-1 field, not `FileInfo` or purported portable filesystem IDs. Failed reads preserve records and do not prove absence. Restart invalidates old observation revisions without discarding persisted admission; browser drafts cannot silently become fresh proof. Preserve the existing cadence and no-per-worktree-status-read boundary. No absolute continuity guarantee is claimed across unobserved downtime, indistinguishable identity reuse or the non-atomic filesystem-validation/persistence interval. Rejected: durable inode/admin-path provenance and an extra polling loop, because they do not provide the stronger proof and the latter changes the approved cadence.

## Constraints

- Preserve the current rail's read-only worktree listing boundary: it reads repository worktree records and does not run a working-tree status read for every listed worktree (`agents-plugin-tool/internal/wsdashboard/railworktrees.go:7-17`; `pageview_rail.go:94-111`).
- Preserve pool-first filtering and root implicit admission even when the repository has no active session-backed pool source; the current helper derives configured, default, and legacy pool roots (`agents-plugin-tool/internal/wsdashboard/railworktreedelete.go:212-230`).
- Preserve canonical root/worktree identity and distinguish `origin: user` from `origin: detected`.
- Owner `(1112)=a` makes the settled toggle, clear-all and undo behavior normative; the tracked reference remains the visual comparison authority.
- MAIN requires a UI head with browser tests for each of `(1096)` through `(1103)` and 1440px/700px light/dark screenshots in pending, selected, rejected-list-with-undo and stale-refusal states for its pre-merge check. Supply all width/theme/state combinations and compare the result captures side by side with the approved HTML reference.
- Target HEAD READY by 20:45 KST on 2026-11-15. If unfinished, commit WIP on the retained branch, record progress and outstanding verification in this ticket, and report to MAIN; do not claim completion or skip acceptance to meet the deadline.
- Use focused changed-path verification only: never run the whole Go suite or entire `internal/wsdashboard` package. Set `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; all MCP tests require `-timeout 30m`. No network/provider/download/credential or shared runtime operation; MAIN alone integrates and swaps the daemon.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Grounded Evidence

- VERIFIED: The HTML and all four PNGs are tracked under `ai-docs/tickets/assets/261115-feat-ws-dashboard-worktree-detection-approval/` (`git ls-files` on that directory). The inspected captures are `847-1-pending-dark.png`, `847-1-pending-light.png`, `847-2-toggled-dark.png`, and `847-3-applied-dark.png`. They show pending rows, selected approval/rejection and the Apply capsule, then an admitted row, collapsed rejected list and copy toast. They are mockup evidence, not implementation acceptance. The HTML also implements clear-all and rejected-to-pending undo; those controls do not settle product behavior.
- VERIFIED: Admission is new work, not an existing add-route side effect. Successful add currently only rereads the repository, wakes the broker and returns the folder (`agents-plugin-tool/internal/wsdashboard/workspace_routes.go#L346-L392`). The existing `RepoWorktree` carries path, branch, detached and prunable flags, not a worktree incarnation (`agents-plugin-tool/internal/wsdashboard/railworktrees.go#L44-L58`). Path matching currently cleans paths and compares case-insensitively on Windows; that is not proof of same filesystem identity (`agents-plugin-tool/internal/wsdashboard/railworktreedelete.go#L192-L210`).
- VERIFIED: Same-file detection exists for cached files in `FileCache.LoadIdentity`, using `os.SameFile`, mode, size and mtime (`agents-plugin-tool/internal/wsdashboard/file_cache.go#L57-L81`). It is not an existing persisted worktree-replacement detector. The rail layout's own file stamp uses existence, size and mtime only (`agents-plugin-tool/internal/wsdashboard/rail_layout.go#L307-L323`). Neither mechanism settles the worktree incarnation contract.
- VERIFIED: The layout reader preserves unknown top-level and workspace fields, rejects a newer schema, and rereads before each mutation under its store mutex (`agents-plugin-tool/internal/wsdashboard/rail_layout.go#L104-L212,#L282-L304,#L387-L429`). Writes use owner-only temporary files and atomic replacement (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L95-L122`). Existing tests pin schema-1 unknown-field preservation, reopen behavior, malformed/newer-file refusal and sequential two-store rereads (`agents-plugin-tool/internal/wsdashboard/rail_layout_test.go#L598-L679`). These facts support the T0 target; they do not prove compatibility of the unwritten admission field or simultaneous cross-process writes.
- VERIFIED: Existing layout writes check POST and same Origin, validate against `railFactsOf(d.railBaseView(d.pages()))`, and wake on change (`agents-plugin-tool/internal/wsdashboard/rail_layout_route.go#L70-L120`). Those facts use the worktree lists read so far, not a fresh Git or filesystem identity read at submit (`agents-plugin-tool/internal/wsdashboard/rail.go#L46-L84`). The delete path separately rereads Git membership and checks disk presence, then checks again at submit; its force token covers HEAD and work contents, not filesystem incarnation (`agents-plugin-tool/internal/wsdashboard/railworktreedelete.go#L336-L424,#L514-L558`). Reusing destructive delete checks wholesale would violate this ticket's no-delete and no-per-worktree-status-read boundaries.
- VERIFIED: Layout generation enters the render stamp; `WsRail.staleRail` rejects older rendered generations (`agents-plugin-tool/internal/wsdashboard/rail.go#L101-L129`; `agents-plugin-tool/internal/wsdashboard/assets/rail.js#L174-L181`). The lister reads only with viewers, on refresh and at the existing three-minute cadence; failed reads return `Read: false`, not confirmed absence (`agents-plugin-tool/internal/wsdashboard/railworktrees.go#L10-L23,#L291-L308`; `agents-plugin-tool/internal/wsdashboard/gitbadge.go#L50-L51`). Pruning after confirmed disappearance must not confuse read failure with removal.
- VERIFIED: The production `copyText` uses the secure-context Clipboard API with a textarea/`execCommand` fallback and success/failure toasts, but returns no completion result (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-actions.js#L85-L105`). The mockup's `copyText` returns a promise and its Apply handler mutates in-memory state before copying; this is not a persisted transaction or a settled failure policy (`ai-docs/tickets/assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html`, `copyText` and `capgo` handler). Paths and branch names are untrusted display/prompt text, not shell commands.
- VERIFIED: `261114-bug-ws-dashboard-rail-drag-regression` is done. Its absent-path retention applies to `worktree_order`, not this new admission field; preserve that separate ordering contract (`agents-plugin-tool/internal/wsdashboard/rail_layout_worktrees.go#L9-L38`). `261115-feat-ws-session-hire-approval` is ready with Phase 1 recorded; preserve its hiring attach gate (`agents-plugin-tool/internal/wsdashboard/rail_layout_route.go#L93-L107`). `261115-feat-ws-dashboard-board-topic-transport` is ready with rail-row deltas still an unfinished Phase 2; coordinate rail markup/selection survival without assuming those deltas have landed.
- VERIFIED: The named fixture research is dropped, with its Resolution pointing to idea ticket `261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe`. Neither selects a fixture remedy or proves provider/network activity. They are related isolation evidence, not implementation prerequisites. No browser, runtime, network operation or test was run in this preparation.

## Incarnation Evidence

- VERIFIED by the bounded source follow-up: `os.SameFile` uses platform-specific filesystem identity, not a portable durable incarnation ID. On Windows, pathname-based `os.Stat` may defer identity capture until a later reopen; a short-lived open followed by `File.Stat` captures the handle's identity immediately (`/opt/homebrew/opt/go/libexec/src/os/types.go#L62-L77`, `types_windows.go#L43-L82,#L288-L364`, `stat_windows.go#L16-L46,#L109-L125`; inspected local Go 1.26.2 source, not a claim about every supported Go implementation).
- VERIFIED: Git's linked-worktree admin names are basename-derived and reusable; `.git`, `commondir` and `gitdir` links establish association but not permanent generation identity (local `git-worktree.1#L493-L505`, `gitrepository-layout.5#L360-L379`). `worktreeMain` memoizes `.git` interpretation and is unsuitable for fresh association validation (`agents-plugin-tool/internal/wsdashboard/peerfolder.go#L10-L31`). Canonicalization must distinguish symlink resolution failure from success rather than accept the tolerant fallback in `internal/wsstate/paths.go#L304-L316`.
- INFERRED limitation: retained observations can detect changed identity or association while they exist. They cannot prove continuity across daemon downtime, indistinguishable filesystem-ID/admin-path reuse, or the non-atomic interval between filesystem validation and admission persistence. Persisting an opaque token does not create historical evidence. Permission, sharing, I/O and link-read failures are unverifiable, not confirmed absence.

## Prior Decisions

- 8a1b5d7e (2026-11-15, commit): "Owner directive: hide pooled worktrees regardless of whether they hold live or ended sessions; the owner never inspects sessions attached to pooled worktrees." — bearing: constrains
- 261115-feat-ws-session-hire-approval (2026-11-15, commit): "Approved peers reuse Runner.CreateLead with parent-less lead authority." — bearing: constrains
- 261114-bug-ws-dashboard-rail-drag-regression (2026-11-14, Result): "keep group-specific ordering helpers/tests in adjacent `rail_layout_worktrees.go` / `rail_layout_worktrees_test.go` rather than expanding the session-layout core further" — bearing: constrains
- 261113-feat-ws-dashboard-session-rail-tree (2026-11-14, commit): "Ops are validated against the rail as the daemon renders it (railBaseView -> railFactsOf)" — bearing: supports
- 6c5fe1f2 (2026-11-10, commit): "The worktree routes act only on a repository the rail shows (a stored one or a listed session's), so the routes cannot be pointed at an arbitrary folder." — bearing: constrains
- 261113-feat-ws-dashboard-worktree-delete (2026-11-13, Decisions): "git worktree remove run from the primary worktree's root; afterwards the rail re-reads (d.rail.reread, broker.wake) like the add route." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: Phase 1 spans pageview_rail.go, rail_layout.go, workspace_routes.go, rail.go and browser presentation under agents-plugin-tool/internal/wsdashboard/. |
| scope.surface | public-interface | VERIFIED: New persistent admission field in schema-1 rail-layout.json and browser Apply write behavior are observable contracts. |
| scope.new_public_symbol | yes | INFERRED: Apply introduces a serialized browser operation and its handler/view seam; its internal names remain implementation choices, not new MCP tools. |
| scope.new_type_contract | yes | VERIFIED: Optional persisted admission records use repository/worktree identity, origin and admission state; Apply adds a serialized selection contract. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/rail_test.go, rail_layout_test.go, workspace_routes_test.go and railscript_test.go cover reusable rail/store/route seams; Phase 1 adds admission and browser cases. |
| complexity.reuse_points | confirmed | VERIFIED: Read railPooled/worktreePoolRoots, layout store apply, sameOrigin gate, broker wake/render generation and production copyText; references are in Grounded Evidence. |
| complexity.side_effect_risk | moderate | INFERRED: Persistent admission writes, disappearance pruning and clipboard replacement change shared view state but must never mutate worktrees. |
| risk.correctness | high | INFERRED: Cached discovery cannot establish latest incarnation; the settled handle observations, Apply/prune races and whole-batch refusal require explicit validation. |
| risk.fit | high | INFERRED: Admission classification intersects root/pool filtering, retained worktree ordering, session-tree visibility and unfinished rail-row deltas. |
| risk.test | high | INFERRED: Same-path replacement, read failures, two-browser persistence, stale Apply and clipboard failure need new cases beyond existing layout tests and mockup captures. |
| risk.security_or_contract | high | INFERRED: Same-origin writes persist shared admission state and copy untrusted paths into an agent prompt; T0 unknown-field preservation and no-delete guarantees must survive failures. |

## Phases

### Phase 1: Detect, review, and remediate non-pool worktrees

Implement the settled T0 admission contract above while preserving root admission, canonical identity, and pool-first exclusion. Record dashboard-created additions as user/admitted, classify unrecorded Git discoveries as detected/pending, persist Apply outcomes, reject removed or replaced entries without touching disk, and wake the existing rail generation. Preserve rejected entries as a collapsed list plus the investigation prompt. A removed and later recreated same path is new detected/pending state; prune the admission record once the old path is confirmed gone. Do not prevent worktree creation at the shell or Git layer.

Focused verification must cover:

- pure rail classification for root, user/admitted, detected/pending, and pool-filtered worktrees using canonical root/worktree identity;
- schema-1 admission-field round-trip, unknown-field preservation, and persistence across refresh;
- reappearance and pruning: after a recorded path is confirmed gone, recreating the same path is new detected/pending state and the old admission record is pruned; do not invent or assert a timing mechanism;
- successful `/rail/worktrees/add` recording `origin: user` and admitted state;
- same-origin Apply revalidation against latest rail facts, persistent admitted/rejected writes, existing generation wake, and rejection of removed/replaced paths without deletion;
- browser tests for all eight owner-confirmed behaviors: same-toggle clearing `(1096)`, approve/reject exclusion `(1097)`, draft clear-all `(1098)`, rejected-list undo to pending `(1099)`, browser-local draft persistence across reload and rail refresh `(1100)`, persisted rejection plus visible clipboard failure and explicit copy retry `(1101)`, cumulative prompt contents `(1102)`, and whole-batch stale refusal retaining drafts and identifying rows `(1103)`;
- handle-captured replacement and Git-association changes, unreadable paths/read failures preserving admission, confirmed disappearance pruning, stale/restarted observation revisions refusing Apply, and the no-delete guarantee;
- regression coverage that pool entries remain hidden before admission classification through `railPooled`/`worktreePoolRoots`;
- the rail's existing worktree-list cadence and no-per-worktree-status-read boundary; and
- browser result screenshots at both 1440px and 700px, in light and dark themes, for pending, selected, rejected-list-with-undo and stale-refusal states, plus an applied-state comparison against `ai-docs/tickets/assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html`. Provide side-by-side comparison evidence and paths for MAIN's pre-merge check; do not substitute mockup screenshots for production result captures.

## Blocked (2026-11-15)

Implementation is retained at `679b9b9b4d1d779da3055c6d30f82e066f943dcb` on `impl/develop/gush-wince-guru`; the worker and its UI child are quiescent. This is incomplete work, not a Phase Result or HEAD READY.

- VERIFIED: Focused backend passes were reported at 2.101s and 5.391s. The lead read the complete final browser/focused output: all eight owner behaviors and 14 focused tests passed in 10.250s. Exact sandbox environment, commands, outputs and the 16-state capture index are in `/tmp/ws-worktree-admission-evidence/{commands.txt,final-test-output.txt,summary.md}`. These artifacts are local evidence, not shipped runtime dependencies.
- VERIFIED: The lead viewed the production 700px dark applied capture and rejected-list side-by-side comparison. The general copy-success toast displays the entire investigation prompt, obscuring and clipping the rail instead of the reference's concise feedback. This Important visual defect remains unfixed; visual acceptance has not passed.
- VERIFIED: The worker reported stop `(f)` at xlarge. The lead read `test-output.txt`, `diagnostic-output.txt` and `diagnostic-stack-output.txt` in that evidence directory: two diagnostic reruns repeated an initial malformed-fixture failure before correction. Undefined usage/memo entries interrupted rail synchronization. Subsequent passing tests do not erase the literal stop condition.
- Pending MAIN decision `(1113)`: authorize one bounded continuation for the concise-toast fix, focused acceptance recapture and fresh independent Lite review, or retain WIP for later. No further implementation, tests, source or HEAD mutation by the worker is authorized while this decision remains unanswered.
- Independent review, final visual acceptance, Phase Result, ticket closure, integration and runtime acceptance remain outstanding. The owner's 20:45 KST committed-WIP/progress-report fallback remains in force.
