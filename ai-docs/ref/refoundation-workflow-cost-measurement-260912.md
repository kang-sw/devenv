### Workflow-cost measurement: before/after refoundation, SIZE=51

Measured and classified on 2026-09-12 (Asia/Seoul). Both complete records use the same corrected manual, pinned commits, SIZE=51, FLOOR=5, selector, exclusions, and four-reader judgment protocol. The after-window contains 37 new stems and 14 shared stems. This complete record supersedes the earlier SIZE=20 blocker artifact.

#### Outcome and epic Dropped criterion

Typed corrective commits rise from **10 of 162** post-implementation commits before to **19 of 114** after, with **19 of 78** in the new after partition. Restricting to plausibly anchored rows gives **6 of 123**, **15 of 92**, and **15 of 60**, respectively. Manual reading separately marks six additional repair references before, four after, and three in the new partition. The after sample therefore contains more visible repair work, including review-driven revisions to refoundation children.

Recorded abort indicators do not increase: blocked headings remain 10; dropped phases remain four; dropped tickets with real implementation remain four of nine printed candidate rows. No vocabulary-matched Result line was judged to record an actual stop. These are recorded lower bounds, not proof that no stops occurred.

The evidence **does not establish the epic’s Dropped criterion**. It exposes more visible re-work in this sample, but commit ownership, granularity and ticket-name recording changed; topic mix and observation spans differ. The new partition has 23 unavailable implementation anchors plus five defective selected anchors among 37 tickets, leaving only nine plausible anchors. These counts cannot establish that worker-interpreter runs generally re-work more than the old pipeline. Nor is there evidence for the criterion’s second requirement, that no child can bring the result below parity. Do not infer parity or improvement either. The measurement requirement is complete; this record does not complete the epic’s other criteria.

#### Run headers and window contract

| Field | Before | After |
|---|---|---|
| Pinned commit | 84b1f825f5858716833d2f8094ab2848826c6011 | cb0458151aecf4979dd05b6c7e702ebbcbf755a2 |
| Recorded branch | develop | develop |
| Commit timestamp | 2026-09-09T16:40:56+09:00 | 2026-09-12T15:41:24+09:00 |
| Run | retroactive, AT_TIP=no | retroactive, AT_TIP=no |
| SIZE / FLOOR | 51 / 5 | 51 / 5 |
| Eligible dated actionable closures | 305 | 342 |
| Missing-completed-date skips | 48 | 51 |
| Window verdict | full (51) | full (51) |
| skipped > SIZE | false | false |
| Selected closure span | 2026-07-28–2026-09-09 | 2026-08-31–2026-09-12 |

The before hash is the exact pre-removal input from the corrected baseline in 260909-bug-workflow-cost-measurement-manual-round-three-findings. The after hash was pinned by the lead after the epic landing and release-path follow-up fixes. The branch subsequently advanced; only immutable hashes feed history and archive reads. Branch revision resolution is confined to the manual’s AT_TIP check.

The initial SIZE=20 windows exceeded the skip limit and had disjoint closure-calendar spans. The lead then explicitly authorized rerunning both complete records at SIZE=51. The manual permits changing size when both halves change together. SIZE=51 is the smallest integer avoiding skipped > SIZE at both hashes, since max(48,51)=51; equality does not trigger. Both runs remain above FLOOR and full. The revised windows also share August 31–September 9, though the unrepresentative restriction no longer fires.

The selector remains exactly the manual’s: .done/ presence and completed: frontmatter, excluding category fields epic/workset/research/idea/design, ordered by completed date then stem. No alternative commit, exclusion, proxy, score or new threshold was introduced. The original SIZE=20 baseline is historical context and is not compared directly against these SIZE=51 figures. The corrected manual at d0cb62b5 and at the after hash has identical git blob 75d1e59ec7a66c826604ad430ae37332b6f624f2; both runs use that text.

#### Commit convention and indicator-6 flags

Before, the corrected baseline records lead-authored commits, approximately one per phase plus a docs(ticket) closure, with impl/goal no-ff merges. The expanded history also contains delegated authorship and multiple product commits per phase. After, pinned AGENTS.md assigns one whole-ticket worker per invocation and one commit per logical unit; separate implementation and chore(ticket) closures appear in the log. Ownership and granularity are therefore not held constant. Both windows contain unprefixed subjects, product-typed administrative changes, unrelated ticket mentions, and implementations that omit their ticket stem. This confound comes before interpreting indicators 1, 2, 3 and 6.

All four USES_* flags were explicitly exported yes in each whole-tree run. Sources were checked at each pinned hash:

| Flag | Before | After |
|---|---|---|
| USES_BLOCKED | lead-drain-ready-queue/SKILL.md blocker selection and dated-note terminal; wsdoc/tickets_sage.go writer | lead-run/lead-run.md lines 162–165, ticket-selector, and tickets_sage.go writer |
| USES_DROPPED_PHASE | wsdoc/conventions/ticket-conventions.md line 53 | same convention line 98 |
| USES_GOAL_BRANCH | lead-drain-ready-queue/SKILL.md goal/* staging/terminal | lead-run/lead-run.md lines 47–51 and 149–150 |
| USES_GOAL_MERGE | corrected baseline’s checked convention, corroborated by reachable e4e3d8ea and other merge(goal) subjects | retained convention and newer f04eb1be / 124041a3 subjects |

Literal merge(goal) is not centralized in ticket-conventions.md: the affirmative answer is corroborated by actual project merge convention, not inferred from a zero. Not every landing uses that spelling. Both commits are now retroactive, so the live branch listing is unavailable, exactly as prescribed.

#### Comparison by indicator

| Indicator | Before 51 | After 51 | New after 37 |
|---|---:|---:|---:|
| 1: days min / median / max | 0 / 0 / 79 | 0 / 0 / 77 | 0 / 0 / 77 |
| 1: first-parent gap min / median / max | 0 / 4 / 488 | 0 / 0 / 550 | 0 / 0 / 550 |
| 2: summed ticket-commit references | 352 | 277 | 179 |
| 2 / 5: distinct commits | 311 | 241 | 157 |
| 3: typed corrective pair | 10 of 162 | 19 of 114 | 19 of 78 |
| 3: measurable / unavailable / n/a | 32 / 16 / 3 | 21 / 29 / 1 | 13 / 23 / 1 |
| 3: plausible / defective selected anchors | 28 / 7 | 16 / 6 | 9 / 5 |
| 3: plausible-anchor pair (measurable rows) | 6 of 123 (25) | 15 of 92 (16) | 15 of 60 (9) |
| 3: extra manually marked repairs | 6 | 4 | 3 |
| 4: matching lines / judged stop lines | 11 / 0 | 24 / 0 | 14 / 0 |
| 4: available / unavailable rows | 46 / 5 | 49 / 2 | 36 / 1 |
| 5: bullets read | 1055 | 715 | 495 |
| 5: judgment / narration | 754 / 301 | 483 / 232 | 351 / 144 |

1. Calendar medians stay zero; old backlog tickets set the maxima. The first-parent median falls to zero, but 32 of 37 new stems have gap zero because creation, work and closure co-integrated onto develop. This describes merge topology, not free execution. Long-lived stems and cross-ticket references inflate the maximum. Days and first-parent commits are separate distributions, neither a measure of effort.

2. Total references and distinct commits fall; docs references fall from 204 to 150, with 87 in the new partition. Different calendar spans, topic mix, shared references and missing stem links prevent interpreting this as time or token savings. Per-ticket totals may count one commit more than once.

3. The after increase is concentrated in the new partition and includes actual review repairs. Missing and defective anchors are separately reported, not hidden behind zero corrective rows. The plausible subset also rises, but its populations are unequal and selectively observed, so no general re-work rate is claimed.

4. All judged stop distributions are min/median/max 0/0/0, with 46, 49 and 36 observations respectively. After’s extra vocabulary comes chiefly from measurement Results quoting historical escalation language and playbook names containing escalated. None is an actual stop for that execution.

5. Bullet volume and judged-item volume fall. This does not measure decision quality. The four-reader protocol was copied, but among 233 exactly identical before/after extracted bullets, 19 receive different classes under their assigned partitions. Reader and context variability is visible and prevents attributing the movement to better or worse judgment.

6. Cumulative whole-tree differences apply to all work between the hashes, not just the window. The +2 other goal merges describe merge traffic, not abandoned runs. No goal completion rate is calculated.

#### Missing-value accounting

| Indicator | Before | After | New partition |
|---|---|---|---|
| 1, both values | 0 unavailable, 0 inverted, 0 n/a | same | same |
| 2 | 0 unavailable rows, 0 n/a | same | same |
| 3 | 16 unavailable, 3 n/a | 29 unavailable, 1 n/a | 23 unavailable, 1 n/a |
| 4 | 5 unavailable, 0 n/a | 2 unavailable, 0 n/a | 1 unavailable, 0 n/a |
| 5 | available; 0 unavailable windows | same | same |
| 6 | live branch listing unavailable | same | not a window indicator |

Indicator-3 unavailable means no eligible product-typed stem-matching implementation; n/a means no later stem-matching commit. Defective anchors are judgment flags in addition to those raw counts. Indicator-4 unavailable means no exact ### Result, even if a Phase heading exists. Indicator 5 has no per-ticket n/a concept.

#### Before record: complete per-ticket record

Stems oldest first, prefixed with completed date:
```text
2026-07-28 260726-chore-retire-sprint-salvage-relocate-skill-authoring
2026-07-28 260726-refactor-retire-spec-planned-marker-mechanism
2026-07-28 260728-feat-lead-backfill-docs-entry-skill
2026-07-30 260730-bug-wsflow-tier-config-access-missing
2026-07-30 260730-feat-odq-batch-interview
2026-07-30 260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice
2026-08-01 260730-chore-ws-dashboard-drop-sweep
2026-08-06 260805-chore-mcp-repair-skill-trigger-attention
2026-08-07 260806-feat-worktree-ticket-scope
2026-08-10 260523-bug-implement-merge-target-discovery
2026-08-10 260523-bug-worktree-local-index-missing
2026-08-10 260807-feat-manuals-doc-tier
2026-08-10 260807-feat-note-memory-layers
2026-08-10 260810-feat-idea-ticket-attention-policy
2026-08-12 260810-feat-repo-tracked-note-layer
2026-08-12 260811-feat-note-visibility-mute
2026-08-13 260807-refactor-dissolve-project-index
2026-08-14 260814-bug-notes-block-substring-collision-test
2026-08-14 260814-bug-ship-preflight-go-test-cache-masks-drift
2026-08-14 260814-bug-wsflow-runtime-contract-missing-note-mute-unmute
2026-08-14 260814-feat-manuals-always-on-authoring-anchor
2026-08-14 260814-feat-note-project-local-untracked-layer
2026-08-14 260814-feat-note-search-optional-multi-layer
2026-08-20 260820-feat-ready-closure-bulk-promotion
2026-08-22 260822-refactor-dissolve-index-local
2026-08-23 260814-refactor-config-collapse-tuning-knobs-to-list-tune
2026-08-23 260823-feat-notes-postit-discipline
2026-08-25 260824-feat-per-phase-review-floor
2026-08-25 260825-feat-impl-branch-single-ticket-scope-merge-timing
2026-08-25 260825-refactor-ws-wsflow-bootstrap-artifact-convergence
2026-08-30 260626-feat-session-key-format-and-retention
2026-08-30 260824-feat-lead-review-range-scenario
2026-08-30 260824-feat-review-release-gate-policy
2026-08-30 260824-feat-review-watermark-ledger
2026-08-30 260828-refactor-per-slice-review-relay
2026-08-30 260830-bug-review-nudge-trackless-bootstrap-gap
2026-08-30 260830-feat-sage-freshness-content-baseline
2026-08-31 260831-bug-survey-plan-unilateral-scope-reduction
2026-08-31 260831-refactor-severity-graded-per-slice-review-relay
2026-09-01 260901-feat-note-oversize-layer-aware-clone-path
2026-09-04 260903-refactor-mcp-read-surface-collapse
2026-09-04 260903-refactor-mcp-todo-signature-merge
2026-09-04 260903-refactor-mcp-verb-vocabulary-unification
2026-09-04 260904-bug-windows-parent-watch-pid-reuse-flake
2026-09-04 260904-refactor-enter-affordance-rename-route-opaque
2026-09-06 260906-bug-route-opaque-params-handler-mismatch
2026-09-07 260907-feat-ws-project-tree-parent-nested-ticket-render
2026-09-08 260908-feat-survey-plan-is-route-not-contract
2026-09-09 260908-bug-sage-gate-stale-completed-has-no-rerun-path
2026-09-09 260908-bug-shipped-surfaces-carry-devenv-only-content
2026-09-09 260908-feat-implement-skip-survey-for-localized-ticket-target
```


Days frequencies: 0 × 26; 1 × 10; 2 × 5; 3 × 2; 6 × 4; 9 × 1; 65 × 1; 79 × 2. First-parent-gap frequencies: 0 × 13; 1 × 5; 2 × 3; 3 × 3; 4 × 4; 5 × 1; 6 × 3; 7 × 4; 9 × 3; 11 × 1; 12 × 3; 21 × 1; 23 × 1; 24 × 1; 31 × 3; 370 × 1; 488 × 1. Commit-type totals: chore=32, docs=204, feat=30, fix=16, merge=43, plan=2, refactor=17, test=8.

| Ticket | Days | FP gap | Commit types | Typed pair | Anchor | Extra repairs | Result matching / judged |
|---|---:|---:|---|---|---|---:|---|
| 260726-chore-retire-sprint-salvage-relocate-skill-authoring | 2 | 0 | chore=1, docs=3, merge=3, refactor=3 | 0 of 6 | 1a96ba1e | 0 | 0 / 0 |
| 260726-refactor-retire-spec-planned-marker-mechanism | 2 | 24 | chore=2, docs=15, feat=2, merge=2, refactor=2 | 0 of 15 | 3b4afa52 | 0 | 0 / 0 |
| 260728-feat-lead-backfill-docs-entry-skill | 0 | 0 | chore=1, feat=1, merge=2, refactor=1 | 0 of 4 | 707236ae | 1 | 0 / 0 |
| 260730-bug-wsflow-tier-config-access-missing | 0 | 0 | chore=1, docs=1, fix=1, merge=1, test=1 | 0 of 4 | af3fa165 | 0 | 0 / 0 |
| 260730-feat-odq-batch-interview | 0 | 4 | chore=1, docs=1, feat=1 | 0 of 1 | 766281e6 | 0 | 0 / 0 |
| 260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice | 0 | 5 | chore=1, docs=2, feat=1, refactor=1 | 0 of 3 | d2865ff4 | 0 | 0 / 0 |
| 260730-chore-ws-dashboard-drop-sweep | 2 | 7 | chore=3, docs=9, fix=3 | 2 of 11 | 73fc63eb | 1 | 0 / 0 |
| 260805-chore-mcp-repair-skill-trigger-attention | 1 | 0 | docs=1 | unavailable | — | 0 | 0 / 0 |
| 260806-feat-worktree-ticket-scope | 1 | 3 | docs=5, feat=2, fix=3, merge=2 | 3 of 9 | e55d7a29 | 0 | 0 / 0 |
| 260523-bug-implement-merge-target-discovery | 79 | 370 | docs=4, feat=1, test=1 | 0 of 3 | 523054ba | 0 | 0 / 0 |
| 260523-bug-worktree-local-index-missing | 79 | 488 | chore=1, docs=6, merge=1 | unavailable | — | 0 | unavailable |
| 260807-feat-manuals-doc-tier | 3 | 7 | docs=4, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260807-feat-note-memory-layers | 3 | 12 | docs=5, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260810-feat-idea-ticket-attention-policy | 0 | 1 | docs=6, feat=3, merge=1, test=1 | 0 of 10 | 863f4014 | 2 | 0 / 0 |
| 260810-feat-repo-tracked-note-layer | 2 | 12 | docs=5, fix=1, merge=2 | 0 of 2 | d0cf6b80 | 0 | 0 / 0 |
| 260811-feat-note-visibility-mute | 1 | 4 | docs=3 | unavailable | — | 0 | 0 / 0 |
| 260807-refactor-dissolve-project-index | 6 | 21 | chore=1, docs=14, fix=1, merge=1, refactor=2 | 1 of 8 | e39e2018 | 0 | 0 / 0 |
| 260814-bug-notes-block-substring-collision-test | 0 | 0 | test=1 | n/a | 4985faa0 | 0 | unavailable |
| 260814-bug-ship-preflight-go-test-cache-masks-drift | 0 | 0 | docs=1 | unavailable | — | 0 | unavailable |
| 260814-bug-wsflow-runtime-contract-missing-note-mute-unmute | 0 | 2 | docs=1, feat=1, fix=1 | 1 of 2 | fbec365f | 0 | 0 / 0 |
| 260814-feat-manuals-always-on-authoring-anchor | 0 | 3 | docs=2, feat=2 | 0 of 3 | 525064f4 | 0 | 0 / 0 |
| 260814-feat-note-project-local-untracked-layer | 0 | 0 | docs=1, feat=1 | 0 of 1 | b9c71861 | 0 | 0 / 0 |
| 260814-feat-note-search-optional-multi-layer | 0 | 0 | docs=3, plan=1 | unavailable | — | 0 | 0 / 0 |
| 260820-feat-ready-closure-bulk-promotion | 0 | 2 | docs=2, feat=1 | n/a | 801cbe2e | 0 | 0 / 0 |
| 260822-refactor-dissolve-index-local | 0 | 0 | docs=1, merge=1, refactor=1 | 0 of 2 | 7104f091 | 0 | 0 / 0 |
| 260814-refactor-config-collapse-tuning-knobs-to-list-tune | 9 | 1 | docs=3, merge=3 | unavailable | — | 0 | 1 / 0 |
| 260823-feat-notes-postit-discipline | 0 | 0 | docs=3, feat=2, merge=2 | 0 of 5 | 76a15996 | 0 | 0 / 0 |
| 260824-feat-per-phase-review-floor | 1 | 6 | chore=2, docs=2, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260825-feat-impl-branch-single-ticket-scope-merge-timing | 0 | 0 | docs=3, feat=1, merge=1 | 0 of 3 | 3db94261 | 0 | 0 / 0 |
| 260825-refactor-ws-wsflow-bootstrap-artifact-convergence | 0 | 0 | docs=13, feat=1, fix=2, merge=1, refactor=2, test=1 | 2 of 13 | 52fbf0bc | 1 | 0 / 0 |
| 260626-feat-session-key-format-and-retention | 65 | 23 | chore=1, docs=4, refactor=1 | 0 of 5 | fd84f0a4 | 0 | 0 / 0 |
| 260824-feat-lead-review-range-scenario | 6 | 31 | chore=3, docs=2, feat=2 | 0 of 3 | 01fd2fe3 | 0 | 0 / 0 |
| 260824-feat-review-release-gate-policy | 6 | 31 | chore=4, docs=4, fix=1 | 0 of 2 | f3ac6a20 | 0 | 0 / 0 |
| 260824-feat-review-watermark-ledger | 6 | 31 | chore=3, docs=8, feat=2, fix=1 | 1 of 8 | df34264a | 0 | 0 / 0 |
| 260828-refactor-per-slice-review-relay | 2 | 11 | chore=1, docs=2 | unavailable | — | 0 | 0 / 0 |
| 260830-bug-review-nudge-trackless-bootstrap-gap | 0 | 0 | fix=1 | n/a | 257b9e6d | 0 | unavailable |
| 260830-feat-sage-freshness-content-baseline | 0 | 3 | chore=2, docs=2, feat=1 | 0 of 3 | 29cb2795 | 0 | 0 / 0 |
| 260831-bug-survey-plan-unilateral-scope-reduction | 0 | 2 | chore=1, docs=3, feat=1, merge=1 | 0 of 4 | 4e795761 | 0 | 4 / 0 |
| 260831-refactor-severity-graded-per-slice-review-relay | 0 | 7 | chore=2, docs=1, merge=1, plan=1 | unavailable | — | 0 | 0 / 0 |
| 260901-feat-note-oversize-layer-aware-clone-path | 0 | 1 | chore=1, docs=3, feat=2 | 0 of 4 | 58599fb8 | 0 | 0 / 0 |
| 260903-refactor-mcp-read-surface-collapse | 1 | 9 | docs=6, merge=1, refactor=1 | 0 of 3 | bd335c7f | 0 | 0 / 0 |
| 260903-refactor-mcp-todo-signature-merge | 1 | 9 | docs=7 | unavailable | — | 0 | 0 / 0 |
| 260903-refactor-mcp-verb-vocabulary-unification | 1 | 9 | docs=6, merge=1, refactor=1 | 0 of 3 | f8101dc6 | 0 | 0 / 0 |
| 260904-bug-windows-parent-watch-pid-reuse-flake | 0 | 1 | docs=1, fix=1 | 0 of 1 | 22c51b15 | 0 | unavailable |
| 260904-refactor-enter-affordance-rename-route-opaque | 0 | 6 | docs=7, merge=2, refactor=2 | 0 of 7 | 9860c89c | 1 | 0 / 0 |
| 260906-bug-route-opaque-params-handler-mismatch | 0 | 1 | docs=2 | unavailable | — | 0 | 0 / 0 |
| 260907-feat-ws-project-tree-parent-nested-ticket-render | 0 | 4 | docs=4, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260908-feat-survey-plan-is-route-not-contract | 0 | 12 | docs=9, feat=1, merge=2 | 0 of 4 | 10d38f80 | 0 | 6 / 0 |
| 260908-bug-sage-gate-stale-completed-has-no-rerun-path | 1 | 7 | docs=5, merge=2 | unavailable | — | 0 | 0 / 0 |
| 260908-bug-shipped-surfaces-carry-devenv-only-content | 1 | 4 | docs=5, feat=1, merge=4, test=3 | 0 of 10 | 83b6653a | 0 | 0 / 0 |
| 260908-feat-implement-skip-survey-for-localized-ticket-target | 1 | 6 | docs=4, merge=2 | unavailable | — | 0 | 0 / 0 |

Anchor judgment for every row, against the pinned phase count and subject:

| Ticket | Phase count | Judgment |
|---|---:|---|
| 260726-chore-retire-sprint-salvage-relocate-skill-authoring | 4 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260726-refactor-retire-spec-planned-marker-mechanism | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260728-feat-lead-backfill-docs-entry-skill | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260730-bug-wsflow-tier-config-access-missing | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260730-feat-odq-batch-interview | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260730-chore-ws-dashboard-drop-sweep | 3 | Defective repair anchor: Phase 1 recovery-verification repair, original archive work predates it. |
| 260805-chore-mcp-repair-skill-trigger-attention | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260806-feat-worktree-ticket-scope | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260523-bug-implement-merge-target-discovery | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260523-bug-worktree-local-index-missing | 0 | Unavailable: no product-typed stem-matching implementation selected. |
| 260807-feat-manuals-doc-tier | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260807-feat-note-memory-layers | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260810-feat-idea-ticket-attention-policy | 2 | Defective administrative anchor: feat(ticket) creates the ready ticket; it implements no product behavior. |
| 260810-feat-repo-tracked-note-layer | 1 | Defective repair anchor: review repair relocates an existing repo-note flock. |
| 260811-feat-note-visibility-mute | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260807-refactor-dissolve-project-index | 2 | Defective partial-phase anchor: self-migration follows the Phase 1 bootstrap-template implementation f652a0d8. |
| 260814-bug-notes-block-substring-collision-test | 0 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260814-bug-ship-preflight-go-test-cache-masks-drift | 0 | Unavailable: no product-typed stem-matching implementation selected. |
| 260814-bug-wsflow-runtime-contract-missing-note-mute-unmute | 1 | Defective wrong-ticket anchor: note-capture documentation names this runtime-contract bug only as a pre-existing failure. |
| 260814-feat-manuals-always-on-authoring-anchor | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260814-feat-note-project-local-untracked-layer | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260814-feat-note-search-optional-multi-layer | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260820-feat-ready-closure-bulk-promotion | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260822-refactor-dissolve-index-local | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260814-refactor-config-collapse-tuning-knobs-to-list-tune | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260823-feat-notes-postit-discipline | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260824-feat-per-phase-review-floor | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260825-feat-impl-branch-single-ticket-scope-merge-timing | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260825-refactor-ws-wsflow-bootstrap-artifact-convergence | 4 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260626-feat-session-key-format-and-retention | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260824-feat-lead-review-range-scenario | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260824-feat-review-release-gate-policy | 2 | Defective repair/late anchor: repairs Phase 2 release-gate behavior after earlier implementation. |
| 260824-feat-review-watermark-ledger | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). Date-only scope: yes. |
| 260828-refactor-per-slice-review-relay | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260830-bug-review-nudge-trackless-bootstrap-gap | 0 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260830-feat-sage-freshness-content-baseline | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260831-bug-survey-plan-unilateral-scope-reduction | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). Date-only scope: yes. |
| 260831-refactor-severity-graded-per-slice-review-relay | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260901-feat-note-oversize-layer-aware-clone-path | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). Date-only scope: yes. |
| 260903-refactor-mcp-read-surface-collapse | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260903-refactor-mcp-todo-signature-merge | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260903-refactor-mcp-verb-vocabulary-unification | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260904-bug-windows-parent-watch-pid-reuse-flake | 0 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260904-refactor-enter-affordance-rename-route-opaque | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260906-bug-route-opaque-params-handler-mismatch | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260907-feat-ws-project-tree-parent-nested-ticket-render | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260908-feat-survey-plan-is-route-not-contract | 2 | Defective late anchor: Phase 2 reviewer handoff, omitting Phase 1 planner/implementer work. |
| 260908-bug-sage-gate-stale-completed-has-no-rerun-path | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260908-bug-shipped-surfaces-carry-devenv-only-content | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260908-feat-implement-skip-survey-for-localized-ticket-target | 1 | Unavailable: no product-typed stem-matching implementation selected. |

Further repair judgments after reading all post-implementation subjects:

| Ticket | Commit | Reading |
|---|---|---|
| 260728-feat-lead-backfill-docs-entry-skill | efe795eb | Review repair: replace per-group mental-model dispatch with one whole-window sweep. |
| 260730-chore-ws-dashboard-drop-sweep | 7364c9a0 | Verification-contract repair: fix self-contradictory dashboard teardown scan requirement. |
| 260810-feat-idea-ticket-attention-policy | 6845444b | Review repair: remove duplicated procedural invariant from shipped skill. |
| 260810-feat-idea-ticket-attention-policy | 476f1562 | Review repair: de-duplicate fixture and replace tautological gate test with ground-truth assertions. |
| 260825-refactor-ws-wsflow-bootstrap-artifact-convergence | b119c658 | Review repair: close the no-tag-silent test coverage finding. |
| 260904-refactor-enter-affordance-rename-route-opaque | d1270d13 | Workflow-record repair: recover Phase 1 Result committed against the wrong detached parent. |

Raw indicator 3, including every post-implementation subject:
```text
260726-chore-retire-sprint-salvage-relocate-skill-authoring anchor=1a96ba1e refactor(skills): retire the lead-sprint and lead-salvage skills
  corrective=0 of 6
    e7669bb7 merge(goal): land brisk-lantern-quill — ws 0.36.16 -> 0.36.29
    3496ac4a merge(impl): retire lead-sprint, lead-salvage, and their enter tools
    76c96923 chore(tickets): close the sprint/salvage retirement ticket
    3dc87a56 refactor(mcp): retire the enter.sprint and enter.salvage tools
    cd84927d docs(tickets): cascade the sprint/salvage retirement through the ticket graph
    726cfde4 refactor(docs): relocate the skill-authoring manual out of the distribution surface
260726-refactor-retire-spec-planned-marker-mechanism anchor=3b4afa52 feat(wsdoc): advise on legacy spec planned markers
  corrective=0 of 15
    328f581b merge(impl): retire the spec planned-marker mechanism
    93e608ac chore(tickets): close the planned-marker retirement and capture index drift
    98421c05 docs(mental-model): record the guard-tier, JSON-key, and ephemeral-source rules
    26c8ad34 docs(tickets): record the phase 2 result for the planned-marker retirement
    2180088f chore(tickets): drop the planned-marker ready-ticket cycle ticket
    629d8d2e feat(bootstrap): ratchet downstream projects off spec planned markers
    ea587b4f docs(spec): convert the workspace-root prune policy to an implemented entry
    33820249 docs(spec): remove the planned-marker mechanism from the repo spec corpus
    46ea558f refactor(rsrc): retire the planned-marker playbook surface and judge: contract-first-spec
    9c48fcaa docs(conventions): retire the planned-marker rules from the embedded conventions
    e33ab6b2 refactor(mcp): drop planned-marker reporting from tool output
    d8d03fdb docs(plans): survey plan for retiring the planned-marker mechanism
    1c889258 merge(impl): add the legacy planned-marker compat advisory
    fd7912ae docs(tickets): record the legacy-marker compat note phase result
    073b6325 docs(mental-model): record the advisory-asymmetry rule and the specs.* output recipe
260728-feat-lead-backfill-docs-entry-skill anchor=707236ae feat(skills): add lead-backfill-docs entry skill and doc-gap-discovery delegate
  corrective=0 of 4
    e7669bb7 merge(goal): land brisk-lantern-quill — ws 0.36.16 -> 0.36.29
    1d977a63 merge(impl): add lead-backfill-docs, the retroactive documentation entry point
    2523aa08 chore(tickets): close the lead-backfill-docs entry-skill ticket
    efe795eb refactor(skills): sweep mental model once per backfill window, not per group
260730-bug-wsflow-tier-config-access-missing anchor=af3fa165 fix(wsflow): expose config.agents_tier as a shared tuning knob, not a mercenary-only surface
  corrective=0 of 4
    4fe73074 merge(wsflow): expose config.agents_tier as a shared tuning knob
    379912b8 chore(tickets): close 260730-bug-wsflow-tier-config-access-missing
    15e31a44 docs(mental-model): correct stale wsflow-hidden claims for config.agents_tier
    41b593d3 test(mcp): assert config.agents_tier tool text stays mercenary-free in wsflow
260730-feat-odq-batch-interview anchor=766281e6 feat(write-ticket): ask the Open Decision Queue as one batch interview
  corrective=0 of 1
    243375e7 chore(tickets): close the ODQ batch interview ticket
260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice anchor=d2865ff4 feat(wsrsrc): splice skill bodies at build time and use it for prefer-subagent
  corrective=0 of 3
    2cf83094 chore(tickets): close the drain rename and prefer-subagent splice ticket
    9018c193 docs(spec): record the two-layer drain contract and build-time skill composition
    a0d857c3 refactor(skills): rename lead-goal-step to lead-drain-ready-queue
260730-chore-ws-dashboard-drop-sweep anchor=73fc63eb fix(archive): verify dashboard recovery evidence
  corrective=2 of 11
    6e445963 chore(tickets): close dashboard drop sweep
    3e1ae3ba docs(ticket): record dropped-ticket metadata edition
    8b594246 fix(tickets): date dropped dashboard records
    41157331 docs(ticket): record dashboard drop sweep Phase 3
    61535fc5 chore(dashboard): remove documentation and reconcile board
    aac9cb72 docs(ticket): record archive verifier correction
    956ee469 docs(ticket): record dashboard teardown result
    7364c9a0 docs(ticket): clarify dashboard teardown scan boundary
    038cc631 docs(ticket): correct archive verification result
    6a2c696e fix(archive): compare archive to snapshot main
    0327d9e7 docs(ticket): record dashboard archive completion
260805-chore-mcp-repair-skill-trigger-attention unavailable (no implementation commit)
260806-feat-worktree-ticket-scope anchor=e55d7a29 feat(wsdoc): resolve tickets hidden by a worktree sparse-checkout scope
  corrective=3 of 9
    7f43a195 merge(main): land ws:lead-scope-worktree skill, manual, and workflow_manual scope gate
    a19f8c15 docs(ticket): close 260806-feat-worktree-ticket-scope (both phases done)
    8755dcab docs(ticket): record Phase 2 result for worktree ticket scope
    abf0859a fix(skills): correct the widen-then-retry remedy for a vanished status dir
    503d6a30 feat(skills): add ws:lead-scope-worktree skill and its reference manual
    8248b813 merge(main): land index-aware ticket board resolution for scoped worktrees
    ef82596d docs(spec): document worktree sparse-checkout ticket scope
    0daa9b74 fix(wsdoc): deliver the write-then-reject notice and keep the filter-off path byte-identical
    a738355a fix(wsdoc): make scope refusals true no-ops and keep the filter-off gate free
260523-bug-implement-merge-target-discovery anchor=523054ba feat(mcp): encode impl/<merge-root>/<stem> in the implement branch resolver
  corrective=0 of 3
    56a3cca7 docs(ticket): close 260523 impl merge-target discovery — Phase 1 Result
    e1a83399 test(mcp): exercise the real git-backed D/F ref-conflict detector
    2ef8cddc docs(spec): update impl/<merge-root>/<stem> prose in spec, mental-model, and fan-out step
260523-bug-worktree-local-index-missing unavailable (no implementation commit)
260807-feat-manuals-doc-tier unavailable (no implementation commit)
260807-feat-note-memory-layers unavailable (no implementation commit)
260810-feat-idea-ticket-attention-policy anchor=863f4014 feat(ticket): add idea-ticket attention policy at ready
  corrective=0 of 10
    de9ac8f4 Merge goal/main/copper-lantern-marsh: note-memory + manuals tier + idea-attention + impl merge-root (ws 0.39.6)
    947562a8 docs(ticket): record 260810 Phase 2 result and close ticket
    3a6451ac feat(wsdoc): fold orphan idea tickets in project_tree rendering
    e0425483 docs(ticket): record 260810 Phase 1 result — idea/ worktree topic-scope
    585af568 docs(spec): trim lead-scope-worktree roster paragraph to a cross-reference
    6845444b docs(lead-scope-worktree): de-duplicate the idea/ scope Invariant against step 3
    476f1562 test(git.commit): dedupe fixture helper, ground-truth gate pin, deletion-safety guardrail pin
    3e5b069f docs(spec): amend workflow_manual, git.commit, and lead-scope-worktree for idea/ scope
    d9218a8e feat(git.commit): stage idea/ captures under an active sparse scope
    b34ff4b3 docs(lead-scope-worktree): bring idea/ into the worktree scope uniformly
260810-feat-repo-tracked-note-layer anchor=d0cf6b80 fix(wsnote): move repo-layer flock out of the tracked note dir
  corrective=0 of 2
    d9e6c0d8 Merge impl repo-tracked-note-layer into goal/main/amber-willow-cinder
    1336ac5e docs(ticket): close 260810 repo-tracked note layer (Phase 1 landed)
260811-feat-note-visibility-mute unavailable (no implementation commit)
260807-refactor-dissolve-project-index anchor=e39e2018 refactor(devenv): dissolve ai-docs/_index.md, validate lead-bootstrap v0046 on this repo
  corrective=1 of 8
    0ca22332 refactor(tickets): ready — dissolve _index.local.md into manuals/*.local.md + clone notes
    ac2aa24a chore(ticket): close 260807-refactor-dissolve-project-index (both phases landed)
    1f4e1eed docs(ticket): record 260807 Phase 2 result — v0046 dissolution dogfooded live on a fixture
    1ec90d06 docs(ticket): capture 260807 Phase 2 run (a) — bootstrap idempotency clean on devenv
    4bf9b447 docs(ticket): record 260807 Phase 1 result — versioned _index dissolution shipped and dogfooded
    a9844005 docs(mental-model): reflect _index.md dissolution and coexistence gating
    1494e62e docs(spec): describe the dissolved _index.md project-memory model
    b844635a fix(dissolve-index): extend if-present degrade to shared rsrc/conventions surface, drop dead bump-script regex
260814-bug-notes-block-substring-collision-test anchor=4985faa0 test(mcp): match # Notes block as a whole line, fixing prose substring collision
  n/a (no post-implementation commits)
260814-bug-ship-preflight-go-test-cache-masks-drift unavailable (no implementation commit)
260814-bug-wsflow-runtime-contract-missing-note-mute-unmute anchor=fbec365f feat(workflow-manual): teach note.* durable-memory capture surface
  corrective=1 of 2
    b70c0669 docs(note-clone-layer): record Result, close tickets, capture SharedDir invariant
    d8f09205 fix(runtime): add note.mute/note.unmute to both runtime.json tool contracts
260814-feat-manuals-always-on-authoring-anchor anchor=525064f4 feat(manuals): make # Manuals an always-on authoring anchor
  corrective=0 of 3
    04edb3b1 docs(ticket): record 260814 Phase 2 result and close manuals-anchor ticket
    68f691c6 feat(mcp): retire manuals.list/manuals.find tools and CLI mirror
    e133d20c docs(ticket): record manuals-anchor Phase 1 result
260814-feat-note-project-local-untracked-layer anchor=b9c71861 feat(wsnote): add clone note layer (project-scoped, worktree-agnostic)
  corrective=0 of 1
    b70c0669 docs(note-clone-layer): record Result, close tickets, capture SharedDir invariant
260814-feat-note-search-optional-multi-layer unavailable (no implementation commit)
260820-feat-ready-closure-bulk-promotion anchor=801cbe2e feat(rsrc): relax ready gate to dependency closure + add bulk ready promotion
  n/a (no post-implementation commits)
260822-refactor-dissolve-index-local anchor=7104f091 refactor(bootstrap): dissolve _index.local.md as local project memory store
  corrective=0 of 2
    7cc035d7 Merge impl/develop/dissolve-index-local: dissolve _index.local.md
    b61b6fd5 docs(ticket): record Phase 1 result and close 260822-refactor-dissolve-index-local
260814-refactor-config-collapse-tuning-knobs-to-list-tune unavailable (no implementation commit)
260823-feat-notes-postit-discipline anchor=76a15996 feat(mcp): nudge note.write to relocate/erase oversize notes
  corrective=0 of 5
    e7f48e8f Merge goal/develop/amber-willow-crest: 260823 notes post-it discipline
    c8f30e92 Merge impl/goal/develop/amber-willow-crest/notes-postit-discipline: 260823 notes post-it discipline
    dfd05528 docs(ticket): record 260823 notes post-it discipline Phase 1 result and close to done
    8dbc21a4 docs(spec): note-tools oversize challenge + always-render notes injection
    79cdbdf9 feat(mcp): always render the # Notes block as a standing post-it hint
260824-feat-per-phase-review-floor unavailable (no implementation commit)
260825-feat-impl-branch-single-ticket-scope-merge-timing anchor=3db94261 feat(mcp): stop enter.implement rename over unmerged cross-ticket work
  corrective=0 of 3
    66beac80 merge(goal): converge harbor-willow-quartz goal run into develop
    71da0fca docs(ticket): close 260825 impl-branch merge-timing (Phase 2 done)
    3c87cd96 docs(ticket): record 260825 Phase 1 result (relation-aware start gate landed)
260825-refactor-ws-wsflow-bootstrap-artifact-convergence anchor=52fbf0bc refactor(bootstrap): converge ws/wsflow bootstrap artifacts to package-neutral form
  corrective=2 of 13
    783b54cd docs(ticket): close 260825 bootstrap-artifact convergence (done)
    5b295f91 merge(bootstrap): converge ws/wsflow bootstrap artifact + unify migration counter (260825)
    1956edc0 docs(ticket): record 260825 Phase 4 result (guard inverted + docs reconciled)
    b30406eb docs(plan): add 260825 Phase 4 invert-test + reconcile-docs plan
    08ed349c docs(ticket): record 260825 Phase 3 result + skew-guard follow-ups
    b119c658 test(bootstrap): harden no-tag-silent test against skew false-fire
    a0267b1f fix(bootstrap): regenerate skills manifest for Phase 1/2 template edits
    21055a6f feat(lead-bootstrap): fire honest version-skew guard on above-head/unknown tags
    c5ae91b5 docs(plan): add Phase 3 skew-guard survey plan for 260825
    72b646e3 docs(ticket): record 260825 Phase 2 result + branch-strategy note
    ee4bc6a5 refactor(lead-bootstrap): unify wsflow migration ordinal onto ws lineage
    c1acb8cf docs(ticket): record Phase 1 result for bootstrap-artifact convergence
    103014f7 fix(bootstrap): convert L86 write-ticket-skill pointer to on-disk phrasing
260626-feat-session-key-format-and-retention anchor=fd84f0a4 refactor(wskey): mint 3-word session keys, drop numeric suffix
  corrective=0 of 5
    2a291f46 docs(idea): capture stale-done-target proceed replay after compaction
    bad73e92 chore(260626): close session-key touch/prune ticket to done
    67e65cdf docs(260626): record Phase 2 touch/prune result
    bf403a97 docs(ticket): open 260827 bug — deterministic word-key impl-branch stem
    a914e967 docs(ticket): refocus 260626 session-key ticket on retention, promote to ready
260824-feat-lead-review-range-scenario anchor=01fd2fe3 feat(lead-review): add range scenario diff selection (Phase 1)
  corrective=0 of 3
    4aa2a83b docs(260824): record Phase 2 landing-lens result and close ticket
    cb777b56 feat(lead-review): add landing lens as range-scenario-only required check
    0cfc692a docs(260824): record Phase 1 range-scenario result
260824-feat-review-release-gate-policy anchor=f3ac6a20 fix(lead-ship): stop for explicit decision on an empty/no-marker ledger
  corrective=0 of 2
    3ca577eb chore(260824): close review-release-gate ticket to done
    bedb99b1 docs(260824): record Phase 2 decoupled release-gate result
260824-feat-review-watermark-ledger anchor=df34264a feat(260824): add wsreview ledger package (Phase 1 primitives)
  corrective=1 of 8
    077c92e0 docs(260824): record Phase 3 result and complete ticket
    fc1d727c docs(mental-model): author review-watermark-ledger domain (Phase 3)
    1aa68886 docs(plan): add ledger P3 banner + canary + no-squash plan
    8c6ccb69 docs(260824): record Phase 2 result for review-watermark ledger
    b7815be4 feat(260824): add ledger P2 checkpoint nudge + sweep-stamp tools
    e404c24c docs(plan): add ledger P2 checkpoint-nudge + sweep plan
    e48a4a42 docs(260824): record Phase 1 result for review watermark ledger
    30e94921 fix(260824): validate Ref shape and cover banner-only Bootstrap branch
260828-refactor-per-slice-review-relay unavailable (no implementation commit)
260830-bug-review-nudge-trackless-bootstrap-gap anchor=257b9e6d fix(wsreview): surface review baseline nudge on trackless projects
  n/a (no post-implementation commits)
260830-feat-sage-freshness-content-baseline anchor=29cb2795 feat(sage-freshness): record body-digest baseline on completed stamp
  corrective=0 of 3
    e9985ba8 chore(260830): close sage freshness content baseline to done
    2beeedf7 docs(260830): record Phase 1 result for sage freshness content baseline
    3b33136e docs(mcp-tools): describe digest-primary sage-review freshness contract
260831-bug-survey-plan-unilateral-scope-reduction anchor=4e795761 feat(260831): forbid unilateral scope reduction in survey/research planners
  corrective=0 of 4
    a64bb684 chore(260831): close survey-plan scope-reduction ticket to .done
    f175e85d Merge impl/develop/elf-jam-claw: forbid unilateral scope reduction in survey/research planners (260831)
    2e6d24df docs(ticket): record 260831 survey-plan scope-reduction Phase 1 result
    5cad50c0 docs(mental-model): reconcile survey/research escalation + reviewer coverage
260831-refactor-severity-graded-per-slice-review-relay unavailable (no implementation commit)
260901-feat-note-oversize-layer-aware-clone-path anchor=58599fb8 feat(260901): add path.generate clone kind allocator
  corrective=0 of 4
    436bf363 chore(260901): close note oversize layer-aware clone path to .done
    32e56317 docs(260901): record Phase 1 result for note oversize layer-aware clone path
    351628de docs(spec): document layer-aware note oversize nudge and clone path kind
    f6a02028 feat(260901): make note.write oversize nudge layer-aware
260903-refactor-mcp-read-surface-collapse anchor=bd335c7f refactor(mcp): collapse tickets/specs list+status into query survivor
  corrective=0 of 3
    17eabe4f Merge layer ③ MCP read-surface collapse into goal branch
    5f88a349 docs(ticket): close layer ③ read-surface-collapse (done)
    d7524118 docs(ticket): record layer ③ Phase 1 result (read-surface collapse landed)
260903-refactor-mcp-todo-signature-merge unavailable (no implementation commit)
260903-refactor-mcp-verb-vocabulary-unification anchor=f8101dc6 refactor(mcp): unify verb vocabulary for six MCP tool names
  corrective=0 of 3
    af25e95c Merge layer ④ MCP verb-vocabulary rename into goal branch
    958fdfe9 docs(ticket): close layer ④ verb-vocabulary-unification (done)
    91616eda docs(ticket): record layer ④ Phase 1 result (verb-vocabulary rename landed)
260904-bug-windows-parent-watch-pid-reuse-flake anchor=22c51b15 fix(ws-mcp): guard Windows parent-watch against PID-reuse spurious fire
  corrective=0 of 1
    6b768b28 docs(ticket): close Windows parent-watch PID-reuse flake as done
260904-refactor-enter-affordance-rename-route-opaque anchor=9860c89c refactor(mcp): rename enter.implement/enter.proceed to route.resolve_*
  corrective=0 of 7
    a4288b73 merge(develop): land epic 260903 MCP tool-surface affordance reduction
    773fee94 merge(goal): layer ① enter.* → route.* rename + opaque schema (Phase 1+2)
    38960d4a docs(ticket): record layer ① Phase 2 result and close (done)
    dd6d34df docs(plan): survey plan for layer ① Phase 2 opaque-params schema hollowing
    d1270d13 docs(ticket): recover layer ① Phase 1 result onto goal branch
    e50cc8bf docs(ticket): record layer ① Phase 1 route.resolve_* rename result
    7e35db10 refactor(rsrc): rename enter.implement/enter.proceed in playbooks
260906-bug-route-opaque-params-handler-mismatch unavailable (no implementation commit)
260907-feat-ws-project-tree-parent-nested-ticket-render unavailable (no implementation commit)
260908-feat-survey-plan-is-route-not-contract anchor=10d38f80 feat(agents-plugin): stop dispatching plan artifact to reviewers
  corrective=0 of 4
    e4e3d8ea merge(goal): drain ready queue — survey-plan route, shipped-surface leak guard, sage-gate answer, delegated survey-skip
    fe205f69 merge(goal): survey-plan-is-route-not-contract Phase 2 into goal
    313a74d9 docs(ticket): close survey-plan-is-route-not-contract (Phase 2 Result)
    fe22347e docs(spec): reconcile reviewer/plan spec sentences with 260908 Phase 2
260908-bug-sage-gate-stale-completed-has-no-rerun-path unavailable (no implementation commit)
260908-bug-shipped-surfaces-carry-devenv-only-content anchor=83b6653a feat(ws-mcp): replace shipped migration-anchor constant with declared binding-anchor hook
  corrective=0 of 10
    e4e3d8ea merge(goal): drain ready queue — survey-plan route, shipped-surface leak guard, sage-gate answer, delegated survey-skip
    167a44cc merge(goal): shipped-surfaces devenv-leak Phase 3 into goal (ticket complete)
    ffa07c53 docs(ticket): close shipped-surfaces-devenv-leak (Phase 3 Result)
    4f587a04 test(shipped-surfaces): add rule-2 positive-trip case
    199e6d81 test(shipped-surfaces): add downstream-neutral guard
    95e10a76 merge(goal): shipped-surfaces devenv-leak Phase 2 into goal
    6ca45f65 docs(260908-shipped-surfaces-devenv-leak): Phase 2 Result
    57924aa7 merge(goal): shipped-surfaces devenv-leak Phase 1 into goal
    4aa28661 docs(260908-shipped-surfaces-devenv-leak): Phase 1 Result + spec framing
    eceadf11 test(ws-mcp): pin binding-anchor production wiring end-to-end
260908-feat-implement-skip-survey-for-localized-ticket-target unavailable (no implementation commit)
```


Raw indicator 4, including every matched line:
```text
260726-chore-retire-sprint-salvage-relocate-skill-authoring escalation_lines=0
260726-refactor-retire-spec-planned-marker-mechanism escalation_lines=0
260728-feat-lead-backfill-docs-entry-skill escalation_lines=0
260730-bug-wsflow-tier-config-access-missing escalation_lines=0
260730-feat-odq-batch-interview escalation_lines=0
260730-refactor-drain-ready-queue-rename-and-prefer-subagent-splice escalation_lines=0
260730-chore-ws-dashboard-drop-sweep escalation_lines=0
260805-chore-mcp-repair-skill-trigger-attention escalation_lines=0
260806-feat-worktree-ticket-scope escalation_lines=0
260523-bug-implement-merge-target-discovery escalation_lines=0
260523-bug-worktree-local-index-missing unavailable (no phase Result)
260807-feat-manuals-doc-tier escalation_lines=0
260807-feat-note-memory-layers escalation_lines=0
260810-feat-idea-ticket-attention-policy escalation_lines=0
260810-feat-repo-tracked-note-layer escalation_lines=0
260811-feat-note-visibility-mute escalation_lines=0
260807-refactor-dissolve-project-index escalation_lines=0
260814-bug-notes-block-substring-collision-test unavailable (no phase Result)
260814-bug-ship-preflight-go-test-cache-masks-drift unavailable (no phase Result)
260814-bug-wsflow-runtime-contract-missing-note-mute-unmute escalation_lines=0
260814-feat-manuals-always-on-authoring-anchor escalation_lines=0
260814-feat-note-project-local-untracked-layer escalation_lines=0
260814-feat-note-search-optional-multi-layer escalation_lines=0
260820-feat-ready-closure-bulk-promotion escalation_lines=0
260822-refactor-dissolve-index-local escalation_lines=0
260814-refactor-config-collapse-tuning-knobs-to-list-tune escalation_lines=1
    Review: partitioned correctness/fit/test all at opus (risk-based escalation for the
260823-feat-notes-postit-discipline escalation_lines=0
260824-feat-per-phase-review-floor escalation_lines=0
260825-feat-impl-branch-single-ticket-scope-merge-timing escalation_lines=0
260825-refactor-ws-wsflow-bootstrap-artifact-convergence escalation_lines=0
260626-feat-session-key-format-and-retention escalation_lines=0
260824-feat-lead-review-range-scenario escalation_lines=0
260824-feat-review-release-gate-policy escalation_lines=0
260824-feat-review-watermark-ledger escalation_lines=0
260828-refactor-per-slice-review-relay escalation_lines=0
260830-bug-review-nudge-trackless-bootstrap-gap unavailable (no phase Result)
260830-feat-sage-freshness-content-baseline escalation_lines=0
260831-bug-survey-plan-unilateral-scope-reduction escalation_lines=4
    new token, the survey delegate gained a lead-directed `[escalate-to-lead]`
    scope-reduction signal (distinct from the un-widened `[escalate-to-research]`),
    and the research delegate's existing `[escalate-to-lead]` was broadened to cover
    `[escalate-to-research]` stays uncertainty-scoped, and forbid the removed vague
260831-refactor-severity-graded-per-slice-review-relay escalation_lines=0
260901-feat-note-oversize-layer-aware-clone-path escalation_lines=0
260903-refactor-mcp-read-surface-collapse escalation_lines=0
260903-refactor-mcp-todo-signature-merge escalation_lines=0
260903-refactor-mcp-verb-vocabulary-unification escalation_lines=0
260904-bug-windows-parent-watch-pid-reuse-flake unavailable (no phase Result)
260904-refactor-enter-affordance-rename-route-opaque escalation_lines=0
260906-bug-route-opaque-params-handler-mismatch escalation_lines=0
260907-feat-ws-project-tree-parent-nested-ticket-render escalation_lines=0
260908-feat-survey-plan-is-route-not-contract escalation_lines=6
    Decision 6 settled-vs-open `[escalate-to-lead]` rule with the
    todo instruction (`session_state.go` survey case) names `[escalate-to-lead]`
    beside `[escalate-to-research]`. Spec `{#260505-implementation-workflow-skills}`
    ruling — only strings that already named `[escalate-to-research]` as a plan
    exit signal gained `[escalate-to-lead]`; (2) the shared `[escalate]` Output
    the ticket-target/inline split and the escalation rules.
260908-bug-sage-gate-stale-completed-has-no-rerun-path escalation_lines=0
260908-bug-shipped-surfaces-carry-devenv-only-content escalation_lines=0
260908-feat-implement-skip-survey-for-localized-ticket-target escalation_lines=0
```


Every available indicator-4 row is judged zero. Indicator 5: 311 distinct commits; 1055 top-level bullets read; 754 judgment, 301 narration. SHA-256 of the exact newline-terminated bullet stream: c76e9a362e86852c9e98ad777f37c3d36944c74cde03285bd9e800bd758b85a9.

#### After record: complete per-ticket record

Stems oldest first, prefixed with completed date:
```text
2026-08-31 260831-bug-survey-plan-unilateral-scope-reduction
2026-08-31 260831-refactor-severity-graded-per-slice-review-relay
2026-09-01 260901-feat-note-oversize-layer-aware-clone-path
2026-09-04 260903-refactor-mcp-read-surface-collapse
2026-09-04 260903-refactor-mcp-todo-signature-merge
2026-09-04 260903-refactor-mcp-verb-vocabulary-unification
2026-09-04 260904-bug-windows-parent-watch-pid-reuse-flake
2026-09-04 260904-refactor-enter-affordance-rename-route-opaque
2026-09-06 260906-bug-route-opaque-params-handler-mismatch
2026-09-07 260907-feat-ws-project-tree-parent-nested-ticket-render
2026-09-08 260908-feat-survey-plan-is-route-not-contract
2026-09-09 260908-bug-sage-gate-stale-completed-has-no-rerun-path
2026-09-09 260908-bug-shipped-surfaces-carry-devenv-only-content
2026-09-09 260908-feat-implement-skip-survey-for-localized-ticket-target
2026-09-09 260909-chore-ws-refoundation-git-history-measurement-manual
2026-09-09 260909-refactor-drain-ready-queue-worker-spawner
2026-09-09 260909-refactor-lead-surface-collapse-worker-stop-protocol
2026-09-09 260909-refactor-route-resolve-implement-reads-ticket-facts
2026-09-10 260726-refactor-retire-workset-convention
2026-09-10 260909-bug-workflow-cost-measurement-manual-round-three-findings
2026-09-10 260909-chore-retire-mercenary-surface
2026-09-10 260909-feat-bootstrap-refoundation-template-migration
2026-09-10 260909-refactor-retire-spec-mental-model-layers
2026-09-10 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope
2026-09-10 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest
2026-09-10 260910-chore-design-review-ready-inventory-contradiction-anchor
2026-09-10 260910-chore-prune-dead-workflow-remnants
2026-09-10 260910-chore-session-children-scope-unnoted-filters
2026-09-10 260910-feat-lead-delegate-session-local-executor
2026-09-10 260910-refactor-ready-only-actionable-ticket-gates
2026-09-10 260910-refactor-risk-route-worker-tier-medium-large
2026-09-11 260626-bug-sage-review-config-setter-missing
2026-09-11 260910-bug-implementation-review-todo-retains-retired-relays
2026-09-11 260910-bug-result-heading-guidance-omits-required-date
2026-09-11 260910-bug-route-ticket-absolute-path-symlink-alias
2026-09-11 260911-bug-lead-run-goal-branch-staging-not-created
2026-09-11 260911-bug-worker-stop-none-with-unfinished-phase
2026-09-11 260911-feat-impl-derivation-hardening-branch-aware-select
2026-09-11 260911-feat-research-outcome-ledger-derivation-contract
2026-09-11 260911-feat-ws-git-merge-lead-owned-merge-authority
2026-09-11 260911-feat-wsflow-lead-proceed-tombstone-alias
2026-09-11 260911-refactor-epic-ready-exception-reduction
2026-09-11 260911-refactor-lead-delegate-executor-tier-resolution
2026-09-11 260911-refactor-lead-run-ticket-only-delegate-implementer
2026-09-12 260906-bug-review-ledger-tools-missing-session-key-schema
2026-09-12 260912-bug-fact-populator-fixture-leaks-repo-paths
2026-09-12 260912-bug-git-merge-release-target-diagnostics
2026-09-12 260912-bug-ticket-fact-populator-drops-manual-constraints
2026-09-12 260912-bug-workflow-manual-ticket-query-schema
2026-09-12 260912-feat-git-merge-generic-branch-promotion
2026-09-12 260912-feat-sage-design-autonomous-exploration
```


Days frequencies: 0 × 36; 1 × 12; 6 × 1; 46 × 1; 77 × 1. First-parent-gap frequencies: 0 × 32; 1 × 3; 2 × 2; 3 × 1; 4 × 3; 6 × 2; 7 × 2; 9 × 3; 12 × 1; 285 × 1; 550 × 1. Commit-type totals: chore=25, docs=150, feat=10, fix=24, merge=44, plan=1, refactor=18, test=5.

| Ticket | Days | FP gap | Commit types | Typed pair | Anchor | Extra repairs | Result matching / judged |
|---|---:|---:|---|---|---|---:|---|
| 260831-bug-survey-plan-unilateral-scope-reduction | 0 | 2 | chore=1, docs=3, feat=1, merge=1 | 0 of 4 | 4e795761 | 0 | 4 / 0 |
| 260831-refactor-severity-graded-per-slice-review-relay | 0 | 7 | chore=2, docs=1, merge=1, plan=1 | unavailable | — | 0 | 0 / 0 |
| 260901-feat-note-oversize-layer-aware-clone-path | 0 | 1 | chore=1, docs=3, feat=2 | 0 of 4 | 58599fb8 | 0 | 0 / 0 |
| 260903-refactor-mcp-read-surface-collapse | 1 | 9 | docs=6, merge=1, refactor=1 | 0 of 3 | bd335c7f | 0 | 0 / 0 |
| 260903-refactor-mcp-todo-signature-merge | 1 | 9 | docs=7 | unavailable | — | 0 | 0 / 0 |
| 260903-refactor-mcp-verb-vocabulary-unification | 0 | 9 | docs=6, merge=1, refactor=1 | 0 of 3 | f8101dc6 | 0 | 0 / 0 |
| 260904-bug-windows-parent-watch-pid-reuse-flake | 0 | 1 | docs=1, fix=1 | 0 of 1 | 22c51b15 | 0 | unavailable |
| 260904-refactor-enter-affordance-rename-route-opaque | 0 | 6 | docs=7, merge=2, refactor=2 | 0 of 7 | 9860c89c | 1 | 0 / 0 |
| 260906-bug-route-opaque-params-handler-mismatch | 0 | 1 | docs=2 | unavailable | — | 0 | 0 / 0 |
| 260907-feat-ws-project-tree-parent-nested-ticket-render | 0 | 4 | docs=4, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260908-feat-survey-plan-is-route-not-contract | 0 | 12 | docs=9, feat=1, merge=2 | 0 of 4 | 10d38f80 | 0 | 6 / 0 |
| 260908-bug-sage-gate-stale-completed-has-no-rerun-path | 1 | 7 | docs=5, merge=2 | unavailable | — | 0 | 0 / 0 |
| 260908-bug-shipped-surfaces-carry-devenv-only-content | 1 | 4 | docs=5, feat=1, merge=4, test=3 | 0 of 10 | 83b6653a | 0 | 0 / 0 |
| 260908-feat-implement-skip-survey-for-localized-ticket-target | 1 | 6 | docs=4, merge=2 | unavailable | — | 0 | 0 / 0 |
| 260909-chore-ws-refoundation-git-history-measurement-manual | 0 | 0 | docs=5 | unavailable | — | 0 | 6 / 0 |
| 260909-refactor-drain-ready-queue-worker-spawner | 0 | 0 | docs=5, fix=4, merge=2, refactor=2 | 4 of 10 | d1e13fab | 0 | 0 / 0 |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 0 | 0 | docs=5, feat=1, fix=2, merge=1, refactor=1 | 2 of 8 | 924d473e | 2 | 0 / 0 |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 0 | 0 | chore=1, docs=4, feat=1, fix=4, merge=2, refactor=2 | 4 of 12 | a894d3db | 1 | 0 / 0 |
| 260726-refactor-retire-workset-convention | 46 | 285 | docs=4, feat=1 | 0 of 3 | 444bcb8a | 0 | 0 / 0 |
| 260909-bug-workflow-cost-measurement-manual-round-three-findings | 1 | 0 | docs=2 | unavailable | — | 0 | 3 / 0 |
| 260909-chore-retire-mercenary-surface | 1 | 0 | docs=6, fix=2, merge=2, refactor=2 | 2 of 9 | 232752e2 | 0 | 0 / 0 |
| 260909-feat-bootstrap-refoundation-template-migration | 1 | 0 | docs=4, feat=1, fix=2, merge=2, refactor=1 | 2 of 8 | 835c9d85 | 0 | 0 / 0 |
| 260909-refactor-retire-spec-mental-model-layers | 1 | 0 | docs=5, fix=3, merge=3, refactor=5 | 3 of 14 | 91621687 | 0 | 0 / 0 |
| 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope | 0 | 0 | docs=1 | unavailable | — | 0 | 0 / 0 |
| 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest | 0 | 0 | docs=2 | unavailable | — | 0 | 0 / 0 |
| 260910-chore-design-review-ready-inventory-contradiction-anchor | 0 | 0 | docs=2, fix=1 | 0 of 1 | a5a7ec48 | 0 | 0 / 0 |
| 260910-chore-prune-dead-workflow-remnants | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-chore-session-children-scope-unnoted-filters | 0 | 0 | docs=1 | unavailable | — | 0 | 0 / 0 |
| 260910-feat-lead-delegate-session-local-executor | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-refactor-ready-only-actionable-ticket-gates | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-refactor-risk-route-worker-tier-medium-large | 0 | 0 | docs=2 | unavailable | — | 0 | 1 / 0 |
| 260626-bug-sage-review-config-setter-missing | 77 | 550 | chore=2, docs=3, fix=3, merge=2, test=1 | 2 of 4 | 73ee3609 | 0 | 0 / 0 |
| 260910-bug-implementation-review-todo-retains-retired-relays | 1 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 2 / 0 |
| 260910-bug-result-heading-guidance-omits-required-date | 1 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 1 / 0 |
| 260910-bug-route-ticket-absolute-path-symlink-alias | 1 | 0 | docs=2, merge=1, refactor=1 | 0 of 3 | f262e488 | 0 | 0 / 0 |
| 260911-bug-lead-run-goal-branch-staging-not-created | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260911-bug-worker-stop-none-with-unfinished-phase | 0 | 0 | chore=1, docs=1 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-impl-derivation-hardening-branch-aware-select | 0 | 0 | chore=1, docs=3 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-research-outcome-ledger-derivation-contract | 0 | 0 | chore=1, docs=1 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-ws-git-merge-lead-owned-merge-authority | 0 | 0 | chore=1, docs=2, feat=1 | 0 of 2 | f6255a1f | 0 | 0 / 0 |
| 260911-feat-wsflow-lead-proceed-tombstone-alias | 0 | 0 | chore=1, docs=2 | unavailable | — | 0 | 0 / 0 |
| 260911-refactor-epic-ready-exception-reduction | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260911-refactor-lead-delegate-executor-tier-resolution | 0 | 0 | chore=1, merge=1, test=1 | n/a | 0c884ce4 | 0 | 0 / 0 |
| 260911-refactor-lead-run-ticket-only-delegate-implementer | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 1 / 0 |
| 260906-bug-review-ledger-tools-missing-session-key-schema | 6 | 2 | chore=1, docs=2, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-bug-fact-populator-fixture-leaks-repo-paths | 0 | 0 | chore=1 | unavailable | — | 0 | unavailable |
| 260912-bug-git-merge-release-target-diagnostics | 0 | 0 | chore=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-bug-ticket-fact-populator-drops-manual-constraints | 0 | 3 | chore=1, docs=2, fix=1, merge=1 | 0 of 2 | bc5bdfc7 | 0 | 0 / 0 |
| 260912-bug-workflow-manual-ticket-query-schema | 0 | 4 | chore=1, docs=2, fix=1, merge=1 | 0 of 2 | 2b112390 | 0 | 0 / 0 |
| 260912-feat-git-merge-generic-branch-promotion | 0 | 0 | chore=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-feat-sage-design-autonomous-exploration | 0 | 0 | chore=1, docs=2, merge=1 | unavailable | — | 0 | 0 / 0 |

Anchor judgment for every row, against the pinned phase count and subject:

| Ticket | Phase count | Judgment |
|---|---:|---|
| 260831-bug-survey-plan-unilateral-scope-reduction | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). Date-only scope: yes. |
| 260831-refactor-severity-graded-per-slice-review-relay | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260901-feat-note-oversize-layer-aware-clone-path | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). Date-only scope: yes. |
| 260903-refactor-mcp-read-surface-collapse | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260903-refactor-mcp-todo-signature-merge | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260903-refactor-mcp-verb-vocabulary-unification | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260904-bug-windows-parent-watch-pid-reuse-flake | 0 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260904-refactor-enter-affordance-rename-route-opaque | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260906-bug-route-opaque-params-handler-mismatch | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260907-feat-ws-project-tree-parent-nested-ticket-render | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260908-feat-survey-plan-is-route-not-contract | 2 | Defective late anchor: Phase 2 reviewer handoff, omitting Phase 1 planner/implementer work. |
| 260908-bug-sage-gate-stale-completed-has-no-rerun-path | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260908-bug-shipped-surfaces-carry-devenv-only-content | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260908-feat-implement-skip-survey-for-localized-ticket-target | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260909-chore-ws-refoundation-git-history-measurement-manual | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260909-refactor-drain-ready-queue-worker-spawner | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260726-refactor-retire-workset-convention | 1 | Defective wrong-ticket anchor: July ready-gate/epic-reference change mentions future workset retirement. |
| 260909-bug-workflow-cost-measurement-manual-round-three-findings | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260909-chore-retire-mercenary-surface | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-feat-bootstrap-refoundation-template-migration | 2 | Defective wrong-ticket anchor: layer-retirement sibling explicitly leaves bootstrap migration untouched. |
| 260909-refactor-retire-spec-mental-model-layers | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-chore-design-review-ready-inventory-contradiction-anchor | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260910-chore-prune-dead-workflow-remnants | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-chore-session-children-scope-unnoted-filters | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-feat-lead-delegate-session-local-executor | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-refactor-ready-only-actionable-ticket-gates | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-refactor-risk-route-worker-tier-medium-large | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260626-bug-sage-review-config-setter-missing | 1 | Defective administrative anchor: fix(tickets) promotes/redesigns the ticket; no config implementation. |
| 260910-bug-implementation-review-todo-retains-retired-relays | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-result-heading-guidance-omits-required-date | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-route-ticket-absolute-path-symlink-alias | 1 | Defective wrong-ticket anchor: workset retirement reports the symlink bug as pre-existing. |
| 260911-bug-lead-run-goal-branch-staging-not-created | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-bug-worker-stop-none-with-unfinished-phase | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-impl-derivation-hardening-branch-aware-select | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-research-outcome-ledger-derivation-contract | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-ws-git-merge-lead-owned-merge-authority | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260911-feat-wsflow-lead-proceed-tombstone-alias | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-refactor-epic-ready-exception-reduction | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-refactor-lead-delegate-executor-tier-resolution | 1 | Defective repair anchor: repairs golden left stale by implementation 61f6351c; raw row is n/a. |
| 260911-refactor-lead-run-ticket-only-delegate-implementer | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260906-bug-review-ledger-tools-missing-session-key-schema | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-fact-populator-fixture-leaks-repo-paths | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-git-merge-release-target-diagnostics | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-ticket-fact-populator-drops-manual-constraints | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260912-bug-workflow-manual-ticket-query-schema | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260912-feat-git-merge-generic-branch-promotion | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-feat-sage-design-autonomous-exploration | 1 | Unavailable: no product-typed stem-matching implementation selected. |

Further repair judgments after reading all post-implementation subjects:

| Ticket | Commit | Reading |
|---|---|---|
| 260904-refactor-enter-affordance-rename-route-opaque | d1270d13 | Workflow-record repair: recover Phase 1 Result committed against the wrong detached parent. |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 11cd2603 | Fresh-reader audit repairs to four shipped lead skill bodies. |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | fa490ada | Round-2 observation repair: correct Result commit citations. |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 92389a0f | Fresh-reader audit repairs to route-fact syntax, edit scope, and tier instruction. |

Raw indicator 3, including every post-implementation subject:
```text
260831-bug-survey-plan-unilateral-scope-reduction anchor=4e795761 feat(260831): forbid unilateral scope reduction in survey/research planners
  corrective=0 of 4
    a64bb684 chore(260831): close survey-plan scope-reduction ticket to .done
    f175e85d Merge impl/develop/elf-jam-claw: forbid unilateral scope reduction in survey/research planners (260831)
    2e6d24df docs(ticket): record 260831 survey-plan scope-reduction Phase 1 result
    5cad50c0 docs(mental-model): reconcile survey/research escalation + reviewer coverage
260831-refactor-severity-graded-per-slice-review-relay unavailable (no implementation commit)
260901-feat-note-oversize-layer-aware-clone-path anchor=58599fb8 feat(260901): add path.generate clone kind allocator
  corrective=0 of 4
    436bf363 chore(260901): close note oversize layer-aware clone path to .done
    32e56317 docs(260901): record Phase 1 result for note oversize layer-aware clone path
    351628de docs(spec): document layer-aware note oversize nudge and clone path kind
    f6a02028 feat(260901): make note.write oversize nudge layer-aware
260903-refactor-mcp-read-surface-collapse anchor=bd335c7f refactor(mcp): collapse tickets/specs list+status into query survivor
  corrective=0 of 3
    17eabe4f Merge layer ③ MCP read-surface collapse into goal branch
    5f88a349 docs(ticket): close layer ③ read-surface-collapse (done)
    d7524118 docs(ticket): record layer ③ Phase 1 result (read-surface collapse landed)
260903-refactor-mcp-todo-signature-merge unavailable (no implementation commit)
260903-refactor-mcp-verb-vocabulary-unification anchor=f8101dc6 refactor(mcp): unify verb vocabulary for six MCP tool names
  corrective=0 of 3
    af25e95c Merge layer ④ MCP verb-vocabulary rename into goal branch
    958fdfe9 docs(ticket): close layer ④ verb-vocabulary-unification (done)
    91616eda docs(ticket): record layer ④ Phase 1 result (verb-vocabulary rename landed)
260904-bug-windows-parent-watch-pid-reuse-flake anchor=22c51b15 fix(ws-mcp): guard Windows parent-watch against PID-reuse spurious fire
  corrective=0 of 1
    6b768b28 docs(ticket): close Windows parent-watch PID-reuse flake as done
260904-refactor-enter-affordance-rename-route-opaque anchor=9860c89c refactor(mcp): rename enter.implement/enter.proceed to route.resolve_*
  corrective=0 of 7
    a4288b73 merge(develop): land epic 260903 MCP tool-surface affordance reduction
    773fee94 merge(goal): layer ① enter.* → route.* rename + opaque schema (Phase 1+2)
    38960d4a docs(ticket): record layer ① Phase 2 result and close (done)
    dd6d34df docs(plan): survey plan for layer ① Phase 2 opaque-params schema hollowing
    d1270d13 docs(ticket): recover layer ① Phase 1 result onto goal branch
    e50cc8bf docs(ticket): record layer ① Phase 1 route.resolve_* rename result
    7e35db10 refactor(rsrc): rename enter.implement/enter.proceed in playbooks
260906-bug-route-opaque-params-handler-mismatch unavailable (no implementation commit)
260907-feat-ws-project-tree-parent-nested-ticket-render unavailable (no implementation commit)
260908-feat-survey-plan-is-route-not-contract anchor=10d38f80 feat(agents-plugin): stop dispatching plan artifact to reviewers
  corrective=0 of 4
    e4e3d8ea merge(goal): drain ready queue — survey-plan route, shipped-surface leak guard, sage-gate answer, delegated survey-skip
    fe205f69 merge(goal): survey-plan-is-route-not-contract Phase 2 into goal
    313a74d9 docs(ticket): close survey-plan-is-route-not-contract (Phase 2 Result)
    fe22347e docs(spec): reconcile reviewer/plan spec sentences with 260908 Phase 2
260908-bug-sage-gate-stale-completed-has-no-rerun-path unavailable (no implementation commit)
260908-bug-shipped-surfaces-carry-devenv-only-content anchor=83b6653a feat(ws-mcp): replace shipped migration-anchor constant with declared binding-anchor hook
  corrective=0 of 10
    e4e3d8ea merge(goal): drain ready queue — survey-plan route, shipped-surface leak guard, sage-gate answer, delegated survey-skip
    167a44cc merge(goal): shipped-surfaces devenv-leak Phase 3 into goal (ticket complete)
    ffa07c53 docs(ticket): close shipped-surfaces-devenv-leak (Phase 3 Result)
    4f587a04 test(shipped-surfaces): add rule-2 positive-trip case
    199e6d81 test(shipped-surfaces): add downstream-neutral guard
    95e10a76 merge(goal): shipped-surfaces devenv-leak Phase 2 into goal
    6ca45f65 docs(260908-shipped-surfaces-devenv-leak): Phase 2 Result
    57924aa7 merge(goal): shipped-surfaces devenv-leak Phase 1 into goal
    4aa28661 docs(260908-shipped-surfaces-devenv-leak): Phase 1 Result + spec framing
    eceadf11 test(ws-mcp): pin binding-anchor production wiring end-to-end
260908-feat-implement-skip-survey-for-localized-ticket-target unavailable (no implementation commit)
260909-chore-ws-refoundation-git-history-measurement-manual unavailable (no implementation commit)
260909-refactor-drain-ready-queue-worker-spawner anchor=d1e13fab refactor(skills): replace lead-drain-ready-queue with lead-run worker spawner
  corrective=4 of 10
    8ffa7157 docs(ticket): file the reviewer branch-switch worktree hazard as an idea
    a2f5018a merge(epic): Phase 2 — retire the fan-out entry point and its serve-time hook
    cefcf79a docs(ticket): record Phase 2 result and close the fan-out retirement ticket
    5d6cc2f3 fix(mcp): close the minor review findings on the fan-out retirement
    f2294816 refactor(skills): retire the fan-out entry point and its serve-time hook
    8275e12e merge(epic): Phase 1 — lead-run replaces the drainer as the worker spawner
    cdd6c376 docs(ticket): record Phase 1 result for the lead-run worker spawner
    14c183f4 fix(rsrc): identify the worker key by its missing note, not by recency
    6976f113 fix(skills): close round-1 review findings on the lead-run placement
    0a9ee632 fix(rsrc): close the three fix-class fresh-reader findings on lead-run
260909-refactor-lead-surface-collapse-worker-stop-protocol anchor=924d473e feat(rsrc): place the worker stop protocol and ticket-worker playbook
  corrective=2 of 8
    53540c70 merge(epic): Phase 2 — lead surface collapsed to five working skills
    f7e208a1 docs(ticket): record the lead-surface collapse Phase 2 result and complete
    b9b0627d fix(mcp): repoint the review todo off the retired skill's prompt sections
    11cd2603 docs(skills): apply the fresh-reader audit fixes to the four placed lead bodies
    ce5f30ef refactor(skills): collapse the lead surface to five working skills plus housekeeping
    fa490ada docs(ticket): correct the Phase 1 Result's commit citations
    a909c29b docs(ticket): record Phase 1 result for the worker stop protocol placement
    27d32dd2 fix(rsrc): restore the goal-branch scope of the worker's autonomous merge
260909-refactor-route-resolve-implement-reads-ticket-facts anchor=a894d3db refactor(mcp): collapse route.resolve_implement to one execution mode
  corrective=4 of 12
    fb2d7602 merge(impl): route.resolve_implement reads its facts from the ticket
    21f58c52 docs(ticket): record route-facts relocation Phase 2 result and close
    7991d938 fix(rsrc): repopulate on an incomplete route-facts section too
    8f0093a0 fix(mcp): an incomplete route-facts table is a gap, not a cautious verdict
    92389a0f docs(rsrc): make the route-facts schema unmistakable to its reader
    408b32fe feat(rsrc): the fact populator writes the route facts the run reads
    2a2809de refactor(mcp): route implementation facts from the ticket, not the caller
    ca416b7b merge(impl): collapse route.resolve_implement to one execution mode
    ea274166 docs(ticket): record route-resolve-implement collapse Phase 1 result
    407b12a8 fix(rsrc): repoint the fix-cycle relays from the retired plan to the target
    2f96d79b fix(mcp): drop the delegated edit-title qualifier and format the collapse
    6ea093ed chore(rsrc): retire the plan-populator playbooks with their dispatcher
260726-refactor-retire-workset-convention anchor=444bcb8a feat(write-ticket): keep dependency-blocked tickets out of ready, and plan absent epic children
  corrective=0 of 3
    cd14e7c7 docs(ticket): record workset retirement verification and close
    a1e9fae2 docs(ticket): promote settled workflow gates to ready
    db1b5b27 docs(ticket): settle ready gates and retire worksets
260909-bug-workflow-cost-measurement-manual-round-three-findings unavailable (no implementation commit)
260909-chore-retire-mercenary-surface anchor=232752e2 refactor(mcp): remove the mercenary tool family, config knob, and render branch
  corrective=2 of 9
    7178eba9 merge(impl): delete the mercenary runtime, CLI, and marker plumbing
    54d8a039 docs(ticket): record the mercenary retirement Phase 2 result and close
    99573fea fix(review): pin the CLI verb set and say how the process round trip is driven
    65b1a812 docs(rsrc): retire the delegate orientation prompt with its only reader
    3d36decd refactor(mcp): remove the mercenary runtime, CLI, and marker plumbing
    f5a44217 merge(impl): retire the caller-visible mercenary surface
    832ab893 docs(ticket): record the Phase 1 result on the mercenary retirement ticket
    86aca803 fix(review): pin the delegate-prompt handoff to the path and cover the agentless catalog cut
    09c055b7 docs(rsrc): rewrite the delegation guidance as native delegation on its own terms
260909-feat-bootstrap-refoundation-template-migration anchor=835c9d85 refactor(rsrc): delete the spec and mental-model playbook surface
  corrective=2 of 8
    68e5c3dd merge(impl): dogfood the v0048 upgrade and close the bootstrap migration ticket
    1c69267e docs(ticket): record the bootstrap refoundation Phase 2 result and close
    18acad38 merge(impl): ship the reduced ai-docs layout downstream at template v0048
    1a8237be docs(ticket): record the bootstrap refoundation Phase 1 result
    7b19c4d2 fix(bootstrap): keep the optional template sections from parsing as declared
    32e74bfa fix(bootstrap): resolve the placed template and guide against a fresh reader
    e85856e7 feat(bootstrap): ship the reduced ai-docs layout downstream at template v0048
    18ac9934 docs(ticket): populate route facts on the bootstrap template-migration ticket
260909-refactor-retire-spec-mental-model-layers anchor=91621687 refactor(wsdoc): retire the ready spec-address gate
  corrective=3 of 14
    d98b2e2f merge(impl): archive the spec and mental-model corpus and close the retirement ticket
    691ec8c5 docs(ticket): record Phase 3 result and close the spec/mental-model retirement
    a20cc4e6 merge(impl): retire the spec and mental-model tool, playbook, and convention surface
    9fa7ae48 docs(ticket): record Phase 2 result for the spec and mental-model retirement
    0fd8fb33 fix(rsrc): close the round-1 review findings on the retirement sweep
    835c9d85 refactor(rsrc): delete the spec and mental-model playbook surface
    a937b8dc docs(ticket): populate route facts on the retire-spec ticket
    28c0dcba merge(impl): retire the spec/mental-model gates, alarm, and doc passes
    b9351cfb docs(ticket): record retire-spec-mental-model Phase 1 result
    e2810a33 fix(wsdoc): state the VerifyResult warning invariant without naming a producer
    fd50a281 fix(mcp): stop promising a documentation pass the route no longer installs
    9997d584 refactor(rsrc): replace the correctness reviewer's spec-drift item
    1ba8c89e refactor(mcp): remove the doc-coverage alarm and its config knob
    07facc32 refactor(mcp): drop the write-time documentation todos from the implement route
260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope unavailable (no implementation commit)
260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest unavailable (no implementation commit)
260910-chore-design-review-ready-inventory-contradiction-anchor anchor=a5a7ec48 fix(rsrc): anchor design review on the ready inventory
  corrective=0 of 1
    eb6512fa docs(ticket): record design anchor verification and close
260910-chore-prune-dead-workflow-remnants unavailable (no implementation commit)
260910-chore-session-children-scope-unnoted-filters unavailable (no implementation commit)
260910-feat-lead-delegate-session-local-executor unavailable (no implementation commit)
260910-refactor-ready-only-actionable-ticket-gates unavailable (no implementation commit)
260910-refactor-risk-route-worker-tier-medium-large unavailable (no implementation commit)
260626-bug-sage-review-config-setter-missing anchor=73ee3609 fix(tickets): ready sage review tuning setter
  corrective=2 of 4
    31134ca3 chore(ticket): close Sage review tuning setter
    b0248e51 test(config): cover no-agent Sage tuning
    6376480e fix(config): report session-effective Sage posture
    c1699e1e fix(config): expose Sage review tuning
260910-bug-implementation-review-todo-retains-retired-relays unavailable (no implementation commit)
260910-bug-result-heading-guidance-omits-required-date unavailable (no implementation commit)
260910-bug-route-ticket-absolute-path-symlink-alias anchor=f262e488 refactor(tickets): retire workset authoring with legacy reads intact
  corrective=0 of 3
    c5b45c28 merge(wsdoc): accept symlink-aliased absolute ticket paths
    27cb93b6 docs(tickets): close symlink-aliased ticket path bug
    7a09924c docs(tickets): promote three refound-audit bug tickets to ready
260911-bug-lead-run-goal-branch-staging-not-created unavailable (no implementation commit)
260911-bug-worker-stop-none-with-unfinished-phase unavailable (no implementation commit)
260911-feat-impl-derivation-hardening-branch-aware-select unavailable (no implementation commit)
260911-feat-research-outcome-ledger-derivation-contract unavailable (no implementation commit)
260911-feat-ws-git-merge-lead-owned-merge-authority anchor=f6255a1f feat(git): constrain impl merges to lead-owned no-ff operation
  corrective=0 of 2
    ec3aa001 chore(ticket): close lead-owned merge authority
    70117c27 docs(ticket): record reviewed git merge tool phase
260911-feat-wsflow-lead-proceed-tombstone-alias unavailable (no implementation commit)
260911-refactor-epic-ready-exception-reduction unavailable (no implementation commit)
260911-refactor-lead-delegate-executor-tier-resolution anchor=0c884ce4 test(skills): reconcile lead-delegate exact-prose golden with the landed tier-resolution change
  n/a (no post-implementation commits)
260911-refactor-lead-run-ticket-only-delegate-implementer unavailable (no implementation commit)
260906-bug-review-ledger-tools-missing-session-key-schema unavailable (no implementation commit)
260912-bug-fact-populator-fixture-leaks-repo-paths unavailable (no implementation commit)
260912-bug-git-merge-release-target-diagnostics unavailable (no implementation commit)
260912-bug-ticket-fact-populator-drops-manual-constraints anchor=bc5bdfc7 fix(tickets): retain fact-populator constraints
  corrective=0 of 2
    37b9735e merge(tickets): preserve fact-populator constraints
    6fa8eb1f chore(ticket): close fact-populator constraint retention
260912-bug-workflow-manual-ticket-query-schema anchor=2b112390 fix(workflow): align ticket query examples with schema
  corrective=0 of 2
    e9531ffd merge(workflow): align ticket query guidance
    ba8de108 chore(ticket): close ticket query schema fix
260912-feat-git-merge-generic-branch-promotion unavailable (no implementation commit)
260912-feat-sage-design-autonomous-exploration unavailable (no implementation commit)
```


Raw indicator 4, including every matched line:
```text
260831-bug-survey-plan-unilateral-scope-reduction escalation_lines=4
    new token, the survey delegate gained a lead-directed `[escalate-to-lead]`
    scope-reduction signal (distinct from the un-widened `[escalate-to-research]`),
    and the research delegate's existing `[escalate-to-lead]` was broadened to cover
    `[escalate-to-research]` stays uncertainty-scoped, and forbid the removed vague
260831-refactor-severity-graded-per-slice-review-relay escalation_lines=0
260901-feat-note-oversize-layer-aware-clone-path escalation_lines=0
260903-refactor-mcp-read-surface-collapse escalation_lines=0
260903-refactor-mcp-todo-signature-merge escalation_lines=0
260903-refactor-mcp-verb-vocabulary-unification escalation_lines=0
260904-bug-windows-parent-watch-pid-reuse-flake unavailable (no phase Result)
260904-refactor-enter-affordance-rename-route-opaque escalation_lines=0
260906-bug-route-opaque-params-handler-mismatch escalation_lines=0
260907-feat-ws-project-tree-parent-nested-ticket-render escalation_lines=0
260908-feat-survey-plan-is-route-not-contract escalation_lines=6
    Decision 6 settled-vs-open `[escalate-to-lead]` rule with the
    todo instruction (`session_state.go` survey case) names `[escalate-to-lead]`
    beside `[escalate-to-research]`. Spec `{#260505-implementation-workflow-skills}`
    ruling — only strings that already named `[escalate-to-research]` as a plan
    exit signal gained `[escalate-to-lead]`; (2) the shared `[escalate]` Output
    the ticket-target/inline split and the escalation rules.
260908-bug-sage-gate-stale-completed-has-no-rerun-path escalation_lines=0
260908-bug-shipped-surfaces-carry-devenv-only-content escalation_lines=0
260908-feat-implement-skip-survey-for-localized-ticket-target escalation_lines=0
260909-chore-ws-refoundation-git-history-measurement-manual escalation_lines=6
    **4. Escalations recorded in `### Result`.** 18 measurable; 2
    was read: all are the tickets' own design vocabulary (`[escalate-to-lead]`,
    `[escalate-to-research]`) discussed in the abstract, not a recorded stop. The
    honest baseline figure is therefore **zero recorded escalations in 20 tickets**,
    high, no escalate-to-research."* → judgment, because it names a routing
      already named `[escalate-to-research]` …") records a lead ruling the ticket
260909-refactor-drain-ready-queue-worker-spawner escalation_lines=0
260909-refactor-lead-surface-collapse-worker-stop-protocol escalation_lines=0
260909-refactor-route-resolve-implement-reads-ticket-facts escalation_lines=0
260726-refactor-retire-workset-convention escalation_lines=0
260909-bug-workflow-cost-measurement-manual-round-three-findings escalation_lines=3
    **4. Escalations recorded in `### Result`.** 18 measurable, 2 `unavailable (no
    actual stop** — all 10 are the tickets' own `[escalate-to-lead]` /
    `[escalate-to-research]` design vocabulary discussed in the abstract.
260909-chore-retire-mercenary-surface escalation_lines=0
260909-feat-bootstrap-refoundation-template-migration escalation_lines=0
260909-refactor-retire-spec-mental-model-layers escalation_lines=0
260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope escalation_lines=0
260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest escalation_lines=0
260910-chore-design-review-ready-inventory-contradiction-anchor escalation_lines=0
260910-chore-prune-dead-workflow-remnants escalation_lines=0
260910-chore-session-children-scope-unnoted-filters escalation_lines=0
260910-feat-lead-delegate-session-local-executor escalation_lines=0
260910-refactor-ready-only-actionable-ticket-gates escalation_lines=0
260910-refactor-risk-route-worker-tier-medium-large escalation_lines=1
      `ticket-worker-escalated` playbooks whose bodies differ only by tier
260626-bug-sage-review-config-setter-missing escalation_lines=0
260910-bug-implementation-review-todo-retains-retired-relays escalation_lines=2
      `implementer-elevated` escalation). `implementReviewInstruction` assembles the
    Decisions (recorded, not escalated): dropped the disposition-marker vocabulary
260910-bug-result-heading-guidance-omits-required-date escalation_lines=1
    (`agents-plugin/rsrc/ticket-worker/`, `ticket-worker-escalated/`,
260910-bug-route-ticket-absolute-path-symlink-alias escalation_lines=0
260911-bug-lead-run-goal-branch-staging-not-created escalation_lines=0
260911-bug-worker-stop-none-with-unfinished-phase escalation_lines=0
260911-feat-impl-derivation-hardening-branch-aware-select escalation_lines=0
260911-feat-research-outcome-ledger-derivation-contract escalation_lines=0
260911-feat-ws-git-merge-lead-owned-merge-authority escalation_lines=0
260911-feat-wsflow-lead-proceed-tombstone-alias escalation_lines=0
260911-refactor-epic-ready-exception-reduction escalation_lines=0
260911-refactor-lead-delegate-executor-tier-resolution escalation_lines=0
260911-refactor-lead-run-ticket-only-delegate-implementer escalation_lines=1
    Decisions taken during execution (recorded, not escalated):
260906-bug-review-ledger-tools-missing-session-key-schema escalation_lines=0
260912-bug-fact-populator-fixture-leaks-repo-paths unavailable (no phase Result)
260912-bug-git-merge-release-target-diagnostics escalation_lines=0
260912-bug-ticket-fact-populator-drops-manual-constraints escalation_lines=0
260912-bug-workflow-manual-ticket-query-schema escalation_lines=0
260912-feat-git-merge-generic-branch-promotion escalation_lines=0
260912-feat-sage-design-autonomous-exploration escalation_lines=0
```


Every available indicator-4 row is judged zero. Indicator 5: 241 distinct commits; 715 top-level bullets read; 483 judgment, 232 narration. SHA-256 of the exact newline-terminated bullet stream: e07526054230308c9dbe4d47cad3f8a8955001ff859ac9e6e964007b69cc10c6.

#### New after partition: complete per-ticket record

Stems oldest first, prefixed with completed date:
```text
2026-09-09 260909-chore-ws-refoundation-git-history-measurement-manual
2026-09-09 260909-refactor-drain-ready-queue-worker-spawner
2026-09-09 260909-refactor-lead-surface-collapse-worker-stop-protocol
2026-09-09 260909-refactor-route-resolve-implement-reads-ticket-facts
2026-09-10 260726-refactor-retire-workset-convention
2026-09-10 260909-bug-workflow-cost-measurement-manual-round-three-findings
2026-09-10 260909-chore-retire-mercenary-surface
2026-09-10 260909-feat-bootstrap-refoundation-template-migration
2026-09-10 260909-refactor-retire-spec-mental-model-layers
2026-09-10 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope
2026-09-10 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest
2026-09-10 260910-chore-design-review-ready-inventory-contradiction-anchor
2026-09-10 260910-chore-prune-dead-workflow-remnants
2026-09-10 260910-chore-session-children-scope-unnoted-filters
2026-09-10 260910-feat-lead-delegate-session-local-executor
2026-09-10 260910-refactor-ready-only-actionable-ticket-gates
2026-09-10 260910-refactor-risk-route-worker-tier-medium-large
2026-09-11 260626-bug-sage-review-config-setter-missing
2026-09-11 260910-bug-implementation-review-todo-retains-retired-relays
2026-09-11 260910-bug-result-heading-guidance-omits-required-date
2026-09-11 260910-bug-route-ticket-absolute-path-symlink-alias
2026-09-11 260911-bug-lead-run-goal-branch-staging-not-created
2026-09-11 260911-bug-worker-stop-none-with-unfinished-phase
2026-09-11 260911-feat-impl-derivation-hardening-branch-aware-select
2026-09-11 260911-feat-research-outcome-ledger-derivation-contract
2026-09-11 260911-feat-ws-git-merge-lead-owned-merge-authority
2026-09-11 260911-feat-wsflow-lead-proceed-tombstone-alias
2026-09-11 260911-refactor-epic-ready-exception-reduction
2026-09-11 260911-refactor-lead-delegate-executor-tier-resolution
2026-09-11 260911-refactor-lead-run-ticket-only-delegate-implementer
2026-09-12 260906-bug-review-ledger-tools-missing-session-key-schema
2026-09-12 260912-bug-fact-populator-fixture-leaks-repo-paths
2026-09-12 260912-bug-git-merge-release-target-diagnostics
2026-09-12 260912-bug-ticket-fact-populator-drops-manual-constraints
2026-09-12 260912-bug-workflow-manual-ticket-query-schema
2026-09-12 260912-feat-git-merge-generic-branch-promotion
2026-09-12 260912-feat-sage-design-autonomous-exploration
```


These 37 stems are exactly the lexical comm -13 difference, restored to after-window order. They are not a separately selected window. Closure span: September 9–12. Shared stems: the first 14 after-window stems, through 260908-feat-implement-skip-survey-for-localized-ticket-target. Shared/new membership describes the sample, not proof of a ticket’s entire execution topology.

Days frequencies: 0 × 27; 1 × 7; 6 × 1; 46 × 1; 77 × 1. First-parent-gap frequencies: 0 × 32; 2 × 1; 3 × 1; 4 × 1; 285 × 1; 550 × 1. Commit-type totals: chore=21, docs=87, feat=5, fix=23, merge=27, refactor=14, test=2.

| Ticket | Days | FP gap | Commit types | Typed pair | Anchor | Extra repairs | Result matching / judged |
|---|---:|---:|---|---|---|---:|---|
| 260909-chore-ws-refoundation-git-history-measurement-manual | 0 | 0 | docs=5 | unavailable | — | 0 | 6 / 0 |
| 260909-refactor-drain-ready-queue-worker-spawner | 0 | 0 | docs=5, fix=4, merge=2, refactor=2 | 4 of 10 | d1e13fab | 0 | 0 / 0 |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 0 | 0 | docs=5, feat=1, fix=2, merge=1, refactor=1 | 2 of 8 | 924d473e | 2 | 0 / 0 |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 0 | 0 | chore=1, docs=4, feat=1, fix=4, merge=2, refactor=2 | 4 of 12 | a894d3db | 1 | 0 / 0 |
| 260726-refactor-retire-workset-convention | 46 | 285 | docs=4, feat=1 | 0 of 3 | 444bcb8a | 0 | 0 / 0 |
| 260909-bug-workflow-cost-measurement-manual-round-three-findings | 1 | 0 | docs=2 | unavailable | — | 0 | 3 / 0 |
| 260909-chore-retire-mercenary-surface | 1 | 0 | docs=6, fix=2, merge=2, refactor=2 | 2 of 9 | 232752e2 | 0 | 0 / 0 |
| 260909-feat-bootstrap-refoundation-template-migration | 1 | 0 | docs=4, feat=1, fix=2, merge=2, refactor=1 | 2 of 8 | 835c9d85 | 0 | 0 / 0 |
| 260909-refactor-retire-spec-mental-model-layers | 1 | 0 | docs=5, fix=3, merge=3, refactor=5 | 3 of 14 | 91621687 | 0 | 0 / 0 |
| 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope | 0 | 0 | docs=1 | unavailable | — | 0 | 0 / 0 |
| 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest | 0 | 0 | docs=2 | unavailable | — | 0 | 0 / 0 |
| 260910-chore-design-review-ready-inventory-contradiction-anchor | 0 | 0 | docs=2, fix=1 | 0 of 1 | a5a7ec48 | 0 | 0 / 0 |
| 260910-chore-prune-dead-workflow-remnants | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-chore-session-children-scope-unnoted-filters | 0 | 0 | docs=1 | unavailable | — | 0 | 0 / 0 |
| 260910-feat-lead-delegate-session-local-executor | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-refactor-ready-only-actionable-ticket-gates | 0 | 0 | docs=4 | unavailable | — | 0 | 0 / 0 |
| 260910-refactor-risk-route-worker-tier-medium-large | 0 | 0 | docs=2 | unavailable | — | 0 | 1 / 0 |
| 260626-bug-sage-review-config-setter-missing | 77 | 550 | chore=2, docs=3, fix=3, merge=2, test=1 | 2 of 4 | 73ee3609 | 0 | 0 / 0 |
| 260910-bug-implementation-review-todo-retains-retired-relays | 1 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 2 / 0 |
| 260910-bug-result-heading-guidance-omits-required-date | 1 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 1 / 0 |
| 260910-bug-route-ticket-absolute-path-symlink-alias | 1 | 0 | docs=2, merge=1, refactor=1 | 0 of 3 | f262e488 | 0 | 0 / 0 |
| 260911-bug-lead-run-goal-branch-staging-not-created | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260911-bug-worker-stop-none-with-unfinished-phase | 0 | 0 | chore=1, docs=1 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-impl-derivation-hardening-branch-aware-select | 0 | 0 | chore=1, docs=3 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-research-outcome-ledger-derivation-contract | 0 | 0 | chore=1, docs=1 | unavailable | — | 0 | 0 / 0 |
| 260911-feat-ws-git-merge-lead-owned-merge-authority | 0 | 0 | chore=1, docs=2, feat=1 | 0 of 2 | f6255a1f | 0 | 0 / 0 |
| 260911-feat-wsflow-lead-proceed-tombstone-alias | 0 | 0 | chore=1, docs=2 | unavailable | — | 0 | 0 / 0 |
| 260911-refactor-epic-ready-exception-reduction | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260911-refactor-lead-delegate-executor-tier-resolution | 0 | 0 | chore=1, merge=1, test=1 | n/a | 0c884ce4 | 0 | 0 / 0 |
| 260911-refactor-lead-run-ticket-only-delegate-implementer | 0 | 0 | chore=1, docs=1, merge=1 | unavailable | — | 0 | 1 / 0 |
| 260906-bug-review-ledger-tools-missing-session-key-schema | 6 | 2 | chore=1, docs=2, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-bug-fact-populator-fixture-leaks-repo-paths | 0 | 0 | chore=1 | unavailable | — | 0 | unavailable |
| 260912-bug-git-merge-release-target-diagnostics | 0 | 0 | chore=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-bug-ticket-fact-populator-drops-manual-constraints | 0 | 3 | chore=1, docs=2, fix=1, merge=1 | 0 of 2 | bc5bdfc7 | 0 | 0 / 0 |
| 260912-bug-workflow-manual-ticket-query-schema | 0 | 4 | chore=1, docs=2, fix=1, merge=1 | 0 of 2 | 2b112390 | 0 | 0 / 0 |
| 260912-feat-git-merge-generic-branch-promotion | 0 | 0 | chore=1, merge=1 | unavailable | — | 0 | 0 / 0 |
| 260912-feat-sage-design-autonomous-exploration | 0 | 0 | chore=1, docs=2, merge=1 | unavailable | — | 0 | 0 / 0 |

Anchor judgment for every row, against the pinned phase count and subject:

| Ticket | Phase count | Judgment |
|---|---:|---|
| 260909-chore-ws-refoundation-git-history-measurement-manual | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260909-refactor-drain-ready-queue-worker-spawner | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260726-refactor-retire-workset-convention | 1 | Defective wrong-ticket anchor: July ready-gate/epic-reference change mentions future workset retirement. |
| 260909-bug-workflow-cost-measurement-manual-round-three-findings | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260909-chore-retire-mercenary-surface | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260909-feat-bootstrap-refoundation-template-migration | 2 | Defective wrong-ticket anchor: layer-retirement sibling explicitly leaves bootstrap migration untouched. |
| 260909-refactor-retire-spec-mental-model-layers | 3 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-chore-design-review-ready-inventory-contradiction-anchor | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260910-chore-prune-dead-workflow-remnants | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-chore-session-children-scope-unnoted-filters | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-feat-lead-delegate-session-local-executor | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-refactor-ready-only-actionable-ticket-gates | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-refactor-risk-route-worker-tier-medium-large | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260626-bug-sage-review-config-setter-missing | 1 | Defective administrative anchor: fix(tickets) promotes/redesigns the ticket; no config implementation. |
| 260910-bug-implementation-review-todo-retains-retired-relays | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-result-heading-guidance-omits-required-date | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260910-bug-route-ticket-absolute-path-symlink-alias | 1 | Defective wrong-ticket anchor: workset retirement reports the symlink bug as pre-existing. |
| 260911-bug-lead-run-goal-branch-staging-not-created | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-bug-worker-stop-none-with-unfinished-phase | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-impl-derivation-hardening-branch-aware-select | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-research-outcome-ledger-derivation-contract | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-feat-ws-git-merge-lead-owned-merge-authority | 2 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260911-feat-wsflow-lead-proceed-tombstone-alias | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-refactor-epic-ready-exception-reduction | 2 | Unavailable: no product-typed stem-matching implementation selected. |
| 260911-refactor-lead-delegate-executor-tier-resolution | 1 | Defective repair anchor: repairs golden left stale by implementation 61f6351c; raw row is n/a. |
| 260911-refactor-lead-run-ticket-only-delegate-implementer | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260906-bug-review-ledger-tools-missing-session-key-schema | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-fact-populator-fixture-leaks-repo-paths | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-git-merge-release-target-diagnostics | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-bug-ticket-fact-populator-drops-manual-constraints | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260912-bug-workflow-manual-ticket-query-schema | 1 | Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix). |
| 260912-feat-git-merge-generic-branch-promotion | 1 | Unavailable: no product-typed stem-matching implementation selected. |
| 260912-feat-sage-design-autonomous-exploration | 1 | Unavailable: no product-typed stem-matching implementation selected. |

Further repair judgments after reading all post-implementation subjects:

| Ticket | Commit | Reading |
|---|---|---|
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | 11cd2603 | Fresh-reader audit repairs to four shipped lead skill bodies. |
| 260909-refactor-lead-surface-collapse-worker-stop-protocol | fa490ada | Round-2 observation repair: correct Result commit citations. |
| 260909-refactor-route-resolve-implement-reads-ticket-facts | 92389a0f | Fresh-reader audit repairs to route-fact syntax, edit scope, and tier instruction. |

Raw indicator 3, including every post-implementation subject:
```text
260909-chore-ws-refoundation-git-history-measurement-manual unavailable (no implementation commit)
260909-refactor-drain-ready-queue-worker-spawner anchor=d1e13fab refactor(skills): replace lead-drain-ready-queue with lead-run worker spawner
  corrective=4 of 10
    8ffa7157 docs(ticket): file the reviewer branch-switch worktree hazard as an idea
    a2f5018a merge(epic): Phase 2 — retire the fan-out entry point and its serve-time hook
    cefcf79a docs(ticket): record Phase 2 result and close the fan-out retirement ticket
    5d6cc2f3 fix(mcp): close the minor review findings on the fan-out retirement
    f2294816 refactor(skills): retire the fan-out entry point and its serve-time hook
    8275e12e merge(epic): Phase 1 — lead-run replaces the drainer as the worker spawner
    cdd6c376 docs(ticket): record Phase 1 result for the lead-run worker spawner
    14c183f4 fix(rsrc): identify the worker key by its missing note, not by recency
    6976f113 fix(skills): close round-1 review findings on the lead-run placement
    0a9ee632 fix(rsrc): close the three fix-class fresh-reader findings on lead-run
260909-refactor-lead-surface-collapse-worker-stop-protocol anchor=924d473e feat(rsrc): place the worker stop protocol and ticket-worker playbook
  corrective=2 of 8
    53540c70 merge(epic): Phase 2 — lead surface collapsed to five working skills
    f7e208a1 docs(ticket): record the lead-surface collapse Phase 2 result and complete
    b9b0627d fix(mcp): repoint the review todo off the retired skill's prompt sections
    11cd2603 docs(skills): apply the fresh-reader audit fixes to the four placed lead bodies
    ce5f30ef refactor(skills): collapse the lead surface to five working skills plus housekeeping
    fa490ada docs(ticket): correct the Phase 1 Result's commit citations
    a909c29b docs(ticket): record Phase 1 result for the worker stop protocol placement
    27d32dd2 fix(rsrc): restore the goal-branch scope of the worker's autonomous merge
260909-refactor-route-resolve-implement-reads-ticket-facts anchor=a894d3db refactor(mcp): collapse route.resolve_implement to one execution mode
  corrective=4 of 12
    fb2d7602 merge(impl): route.resolve_implement reads its facts from the ticket
    21f58c52 docs(ticket): record route-facts relocation Phase 2 result and close
    7991d938 fix(rsrc): repopulate on an incomplete route-facts section too
    8f0093a0 fix(mcp): an incomplete route-facts table is a gap, not a cautious verdict
    92389a0f docs(rsrc): make the route-facts schema unmistakable to its reader
    408b32fe feat(rsrc): the fact populator writes the route facts the run reads
    2a2809de refactor(mcp): route implementation facts from the ticket, not the caller
    ca416b7b merge(impl): collapse route.resolve_implement to one execution mode
    ea274166 docs(ticket): record route-resolve-implement collapse Phase 1 result
    407b12a8 fix(rsrc): repoint the fix-cycle relays from the retired plan to the target
    2f96d79b fix(mcp): drop the delegated edit-title qualifier and format the collapse
    6ea093ed chore(rsrc): retire the plan-populator playbooks with their dispatcher
260726-refactor-retire-workset-convention anchor=444bcb8a feat(write-ticket): keep dependency-blocked tickets out of ready, and plan absent epic children
  corrective=0 of 3
    cd14e7c7 docs(ticket): record workset retirement verification and close
    a1e9fae2 docs(ticket): promote settled workflow gates to ready
    db1b5b27 docs(ticket): settle ready gates and retire worksets
260909-bug-workflow-cost-measurement-manual-round-three-findings unavailable (no implementation commit)
260909-chore-retire-mercenary-surface anchor=232752e2 refactor(mcp): remove the mercenary tool family, config knob, and render branch
  corrective=2 of 9
    7178eba9 merge(impl): delete the mercenary runtime, CLI, and marker plumbing
    54d8a039 docs(ticket): record the mercenary retirement Phase 2 result and close
    99573fea fix(review): pin the CLI verb set and say how the process round trip is driven
    65b1a812 docs(rsrc): retire the delegate orientation prompt with its only reader
    3d36decd refactor(mcp): remove the mercenary runtime, CLI, and marker plumbing
    f5a44217 merge(impl): retire the caller-visible mercenary surface
    832ab893 docs(ticket): record the Phase 1 result on the mercenary retirement ticket
    86aca803 fix(review): pin the delegate-prompt handoff to the path and cover the agentless catalog cut
    09c055b7 docs(rsrc): rewrite the delegation guidance as native delegation on its own terms
260909-feat-bootstrap-refoundation-template-migration anchor=835c9d85 refactor(rsrc): delete the spec and mental-model playbook surface
  corrective=2 of 8
    68e5c3dd merge(impl): dogfood the v0048 upgrade and close the bootstrap migration ticket
    1c69267e docs(ticket): record the bootstrap refoundation Phase 2 result and close
    18acad38 merge(impl): ship the reduced ai-docs layout downstream at template v0048
    1a8237be docs(ticket): record the bootstrap refoundation Phase 1 result
    7b19c4d2 fix(bootstrap): keep the optional template sections from parsing as declared
    32e74bfa fix(bootstrap): resolve the placed template and guide against a fresh reader
    e85856e7 feat(bootstrap): ship the reduced ai-docs layout downstream at template v0048
    18ac9934 docs(ticket): populate route facts on the bootstrap template-migration ticket
260909-refactor-retire-spec-mental-model-layers anchor=91621687 refactor(wsdoc): retire the ready spec-address gate
  corrective=3 of 14
    d98b2e2f merge(impl): archive the spec and mental-model corpus and close the retirement ticket
    691ec8c5 docs(ticket): record Phase 3 result and close the spec/mental-model retirement
    a20cc4e6 merge(impl): retire the spec and mental-model tool, playbook, and convention surface
    9fa7ae48 docs(ticket): record Phase 2 result for the spec and mental-model retirement
    0fd8fb33 fix(rsrc): close the round-1 review findings on the retirement sweep
    835c9d85 refactor(rsrc): delete the spec and mental-model playbook surface
    a937b8dc docs(ticket): populate route facts on the retire-spec ticket
    28c0dcba merge(impl): retire the spec/mental-model gates, alarm, and doc passes
    b9351cfb docs(ticket): record retire-spec-mental-model Phase 1 result
    e2810a33 fix(wsdoc): state the VerifyResult warning invariant without naming a producer
    fd50a281 fix(mcp): stop promising a documentation pass the route no longer installs
    9997d584 refactor(rsrc): replace the correctness reviewer's spec-drift item
    1ba8c89e refactor(mcp): remove the doc-coverage alarm and its config knob
    07facc32 refactor(mcp): drop the write-time documentation todos from the implement route
260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope unavailable (no implementation commit)
260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest unavailable (no implementation commit)
260910-chore-design-review-ready-inventory-contradiction-anchor anchor=a5a7ec48 fix(rsrc): anchor design review on the ready inventory
  corrective=0 of 1
    eb6512fa docs(ticket): record design anchor verification and close
260910-chore-prune-dead-workflow-remnants unavailable (no implementation commit)
260910-chore-session-children-scope-unnoted-filters unavailable (no implementation commit)
260910-feat-lead-delegate-session-local-executor unavailable (no implementation commit)
260910-refactor-ready-only-actionable-ticket-gates unavailable (no implementation commit)
260910-refactor-risk-route-worker-tier-medium-large unavailable (no implementation commit)
260626-bug-sage-review-config-setter-missing anchor=73ee3609 fix(tickets): ready sage review tuning setter
  corrective=2 of 4
    31134ca3 chore(ticket): close Sage review tuning setter
    b0248e51 test(config): cover no-agent Sage tuning
    6376480e fix(config): report session-effective Sage posture
    c1699e1e fix(config): expose Sage review tuning
260910-bug-implementation-review-todo-retains-retired-relays unavailable (no implementation commit)
260910-bug-result-heading-guidance-omits-required-date unavailable (no implementation commit)
260910-bug-route-ticket-absolute-path-symlink-alias anchor=f262e488 refactor(tickets): retire workset authoring with legacy reads intact
  corrective=0 of 3
    c5b45c28 merge(wsdoc): accept symlink-aliased absolute ticket paths
    27cb93b6 docs(tickets): close symlink-aliased ticket path bug
    7a09924c docs(tickets): promote three refound-audit bug tickets to ready
260911-bug-lead-run-goal-branch-staging-not-created unavailable (no implementation commit)
260911-bug-worker-stop-none-with-unfinished-phase unavailable (no implementation commit)
260911-feat-impl-derivation-hardening-branch-aware-select unavailable (no implementation commit)
260911-feat-research-outcome-ledger-derivation-contract unavailable (no implementation commit)
260911-feat-ws-git-merge-lead-owned-merge-authority anchor=f6255a1f feat(git): constrain impl merges to lead-owned no-ff operation
  corrective=0 of 2
    ec3aa001 chore(ticket): close lead-owned merge authority
    70117c27 docs(ticket): record reviewed git merge tool phase
260911-feat-wsflow-lead-proceed-tombstone-alias unavailable (no implementation commit)
260911-refactor-epic-ready-exception-reduction unavailable (no implementation commit)
260911-refactor-lead-delegate-executor-tier-resolution anchor=0c884ce4 test(skills): reconcile lead-delegate exact-prose golden with the landed tier-resolution change
  n/a (no post-implementation commits)
260911-refactor-lead-run-ticket-only-delegate-implementer unavailable (no implementation commit)
260906-bug-review-ledger-tools-missing-session-key-schema unavailable (no implementation commit)
260912-bug-fact-populator-fixture-leaks-repo-paths unavailable (no implementation commit)
260912-bug-git-merge-release-target-diagnostics unavailable (no implementation commit)
260912-bug-ticket-fact-populator-drops-manual-constraints anchor=bc5bdfc7 fix(tickets): retain fact-populator constraints
  corrective=0 of 2
    37b9735e merge(tickets): preserve fact-populator constraints
    6fa8eb1f chore(ticket): close fact-populator constraint retention
260912-bug-workflow-manual-ticket-query-schema anchor=2b112390 fix(workflow): align ticket query examples with schema
  corrective=0 of 2
    e9531ffd merge(workflow): align ticket query guidance
    ba8de108 chore(ticket): close ticket query schema fix
260912-feat-git-merge-generic-branch-promotion unavailable (no implementation commit)
260912-feat-sage-design-autonomous-exploration unavailable (no implementation commit)
```


Raw indicator 4, including every matched line:
```text
260909-chore-ws-refoundation-git-history-measurement-manual escalation_lines=6
    **4. Escalations recorded in `### Result`.** 18 measurable; 2
    was read: all are the tickets' own design vocabulary (`[escalate-to-lead]`,
    `[escalate-to-research]`) discussed in the abstract, not a recorded stop. The
    honest baseline figure is therefore **zero recorded escalations in 20 tickets**,
    high, no escalate-to-research."* → judgment, because it names a routing
      already named `[escalate-to-research]` …") records a lead ruling the ticket
260909-refactor-drain-ready-queue-worker-spawner escalation_lines=0
260909-refactor-lead-surface-collapse-worker-stop-protocol escalation_lines=0
260909-refactor-route-resolve-implement-reads-ticket-facts escalation_lines=0
260726-refactor-retire-workset-convention escalation_lines=0
260909-bug-workflow-cost-measurement-manual-round-three-findings escalation_lines=3
    **4. Escalations recorded in `### Result`.** 18 measurable, 2 `unavailable (no
    actual stop** — all 10 are the tickets' own `[escalate-to-lead]` /
    `[escalate-to-research]` design vocabulary discussed in the abstract.
260909-chore-retire-mercenary-surface escalation_lines=0
260909-feat-bootstrap-refoundation-template-migration escalation_lines=0
260909-refactor-retire-spec-mental-model-layers escalation_lines=0
260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope escalation_lines=0
260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest escalation_lines=0
260910-chore-design-review-ready-inventory-contradiction-anchor escalation_lines=0
260910-chore-prune-dead-workflow-remnants escalation_lines=0
260910-chore-session-children-scope-unnoted-filters escalation_lines=0
260910-feat-lead-delegate-session-local-executor escalation_lines=0
260910-refactor-ready-only-actionable-ticket-gates escalation_lines=0
260910-refactor-risk-route-worker-tier-medium-large escalation_lines=1
      `ticket-worker-escalated` playbooks whose bodies differ only by tier
260626-bug-sage-review-config-setter-missing escalation_lines=0
260910-bug-implementation-review-todo-retains-retired-relays escalation_lines=2
      `implementer-elevated` escalation). `implementReviewInstruction` assembles the
    Decisions (recorded, not escalated): dropped the disposition-marker vocabulary
260910-bug-result-heading-guidance-omits-required-date escalation_lines=1
    (`agents-plugin/rsrc/ticket-worker/`, `ticket-worker-escalated/`,
260910-bug-route-ticket-absolute-path-symlink-alias escalation_lines=0
260911-bug-lead-run-goal-branch-staging-not-created escalation_lines=0
260911-bug-worker-stop-none-with-unfinished-phase escalation_lines=0
260911-feat-impl-derivation-hardening-branch-aware-select escalation_lines=0
260911-feat-research-outcome-ledger-derivation-contract escalation_lines=0
260911-feat-ws-git-merge-lead-owned-merge-authority escalation_lines=0
260911-feat-wsflow-lead-proceed-tombstone-alias escalation_lines=0
260911-refactor-epic-ready-exception-reduction escalation_lines=0
260911-refactor-lead-delegate-executor-tier-resolution escalation_lines=0
260911-refactor-lead-run-ticket-only-delegate-implementer escalation_lines=1
    Decisions taken during execution (recorded, not escalated):
260906-bug-review-ledger-tools-missing-session-key-schema escalation_lines=0
260912-bug-fact-populator-fixture-leaks-repo-paths unavailable (no phase Result)
260912-bug-git-merge-release-target-diagnostics escalation_lines=0
260912-bug-ticket-fact-populator-drops-manual-constraints escalation_lines=0
260912-bug-workflow-manual-ticket-query-schema escalation_lines=0
260912-feat-git-merge-generic-branch-promotion escalation_lines=0
260912-feat-sage-design-autonomous-exploration escalation_lines=0
```


Every available indicator-4 row is judged zero. Indicator 5: 157 distinct commits; 495 top-level bullets read; 351 judgment, 144 narration. SHA-256 of the exact newline-terminated bullet stream: f5b3f7d1ed445b157e1106a3e183868a15605741d91098b89c2df706808f59a7.

#### Indicator 6: whole-tree raw results and judgments

Before:
```text
blocked notes: 10
dropped phases: 4
  260405-research-marathon-delegation-hardening <- 5ea1f4e2 refactor(skills): apply Phase 1 delegation hardening to marathon skill
  260421-feat-rebuild-spec-skill <- cd878e98 feat(workflow): spec-driven workflow — ticket, survey plan, rebuild-spec idea
  260425-chore-mental-model-index-migration <- 101aa201 revert: roll back mental-model.md → mental-model/index.md migration
  260429-feat-api-deps <- 68c02000 feat(ticket): 260429-feat-api-deps — ws-ask-api 2-layer API doc cache system
  260505-bug-plugin-managed-default-root-discovery <- 0221f6fe fix(wsagent): tolerate trailing Codex stdout noise
  260513-feat-async-exec-output-reader <- af90f0bc feat(ws-dashboard): implement WorkRoot Activity pane
  260524-bug-wsstore-ci-sqlite-busy <- 6334093e refactor(wsagent): remove Gemini runner impl + dispatch + config alias
  260626-bug-prefer-subagent-fork-executor-narration <- 4f98440c fix(prefer-subagent): lead with strong composed fork directive, drop weak trailing line
  260626-bug-prefer-subagent-recursive-delegate-escape <- 2497e5cc fix(prefer-subagent): block recursive fork delegation
dropped with implementation commits: 9 (read the rows above)
goal runs landed on the measured line: 5
other merge(goal) merges reachable: 9
goal branches not merged: unavailable (live ref set, run is retroactive)
```

After:
```text
blocked notes: 10
dropped phases: 4
  260405-research-marathon-delegation-hardening <- 5ea1f4e2 refactor(skills): apply Phase 1 delegation hardening to marathon skill
  260421-feat-rebuild-spec-skill <- cd878e98 feat(workflow): spec-driven workflow — ticket, survey plan, rebuild-spec idea
  260425-chore-mental-model-index-migration <- 101aa201 revert: roll back mental-model.md → mental-model/index.md migration
  260429-feat-api-deps <- 68c02000 feat(ticket): 260429-feat-api-deps — ws-ask-api 2-layer API doc cache system
  260505-bug-plugin-managed-default-root-discovery <- 0221f6fe fix(wsagent): tolerate trailing Codex stdout noise
  260513-feat-async-exec-output-reader <- af90f0bc feat(ws-dashboard): implement WorkRoot Activity pane
  260524-bug-wsstore-ci-sqlite-busy <- 6334093e refactor(wsagent): remove Gemini runner impl + dispatch + config alias
  260626-bug-prefer-subagent-fork-executor-narration <- 4f98440c fix(prefer-subagent): lead with strong composed fork directive, drop weak trailing line
  260626-bug-prefer-subagent-recursive-delegate-escape <- 2497e5cc fix(prefer-subagent): block recursive fork delegation
dropped with implementation commits: 9 (read the rows above)
goal runs landed on the measured line: 5
other merge(goal) merges reachable: 11
goal branches not merged: unavailable (live ref set, run is retroactive)
```


| Indicator | Before | After | After minus before |
|---|---:|---:|---:|
| Blocked headings | 10 | 10 | 0 |
| Dropped phase headings | 4 | 4 | 0 |
| Dropped implementation candidates printed | 9 | 9 | 0 |
| Dropped implementation candidates judged real | 4 | 4 | 0 |
| First-parent goal merges | 5 | 5 | 0 |
| Other reachable goal merges | 9 | 11 | +2 |
| Unmerged local goal branches | unavailable | unavailable | unavailable |

All existing ticket files in visible statuses plus .done/ and .dropped/ were checked using the manual’s fence-toggle pattern; zero files had odd fence counts. The ten Blocked heading identities and four dropped-phase heading identities are unchanged. One Blocked heading explicitly says RESOLVED; this is a count of recorded headings, not current blockers.

| Dropped ticket | Anchor | Real? | Judgment |
|---|---|---|---|
| 260405-research-marathon-delegation-hardening | 5ea1f4e2 | yes | Implements the named Phase 1 hardening checklist. |
| 260421-feat-rebuild-spec-skill | cd878e98 | no | Adds the idea ticket beside another workflow ticket and plan. |
| 260425-chore-mental-model-index-migration | 101aa201 | yes | Reverts seven implementation commits and preserves the attempt as dropped. |
| 260429-feat-api-deps | 68c02000 | no | Authors a four-phase ticket, not its implementation. |
| 260505-bug-plugin-managed-default-root-discovery | 0221f6fe | no | Fixes another ticket’s trailing stdout; explicitly captures root discovery for follow-up. |
| 260513-feat-async-exec-output-reader | af90f0bc | no | Implements Activity pane; Running Commands stays empty pending this ticket. |
| 260524-bug-wsstore-ci-sqlite-busy | 6334093e | no | Removes unrelated Gemini runner and drops obsolete persistence bug. |
| 260626-bug-prefer-subagent-fork-executor-narration | 4f98440c | yes | Implements the named fork-prompt mitigation. |
| 260626-bug-prefer-subagent-recursive-delegate-escape | 2497e5cc | yes | Implements the named recursive-fork mitigation. |

The same nine rows and judgments apply to both trees. Both runs correctly omit today’s branch listing. The five unmerged branches in the original baseline were an observation made then and are not substituted into these retroactive runs.

#### Mandatory judgment protocol and ambiguities

Indicator 3: the measurer read every printed anchor and later subject, the pinned phase headings for every ticket, and suspicious anchor/repair commit bodies plus relevant Results. Plausible does not prove that no unnamed work existed. e39e2018 is partial-phase work: the Result names earlier bootstrap-template implementation f652a0d8, while this anchor performs the subsequent self-migration. 0c884ce4 explicitly repairs a golden left stale by implementation 61f6351c; its raw n/a conceals a repair.

Additional repair marks include concrete review re-work and workflow-record repairs (detached Result recovery and citation correction). Mere closure, merge, phase-result narration or an Edition recording an already-counted repair is not a second repair. Thus 038cc631, aac9cb72 and 3e1ae3ba narrate dashboard fixes; 15e31a44 is ordinary closeout reconciliation. Planned test additions are not assumed repairs; b119c658 is included because its body explicitly closes a review coverage finding. These are stated judgment boundaries, not changes to the automatic typed rule.

The manual both notes that fix is how a bug lands and later calls fix/revert anchors unavailable in disguise. This record follows the required category/phase reading and corrected-baseline precedent: af3fa165, 257b9e6d, 22c51b15, a5a7ec48, bc5bdfc7 and 2b112390 plausibly implement their first bug-fix phase. No earlier work is inferred only from the prefix. Repair anchors with evidence of prior work are marked defective. Raw subjects preserve the typed signal.

Date-only anchors: before has three, df34264a (260824 watermark-ledger), 4e795761 (260831 scope-reduction), and 58599fb8 (260901 clone-path). After has the latter two; new partition has zero. Subjects/bodies align with their tickets, so no observed misattribution is due to date-only scope. The corrected SIZE=20 baseline also said three but described all three as 260824 tickets; that description does not reproduce: 01fd2fe3 has scope lead-review and f3ac6a20 has scope lead-ship. Actual scopes are recorded here without editing frozen history.

Indicator 4: before’s extra config-collapse line describes risk-based reviewer-tier escalation, not a stop; its other ten hits are design vocabulary. After’s 24 hits are ten inherited vocabulary lines, nine quoted prior-measurement lines, one escalated-playbook name in risk routing, two retired-relay lines (design vocabulary and explicitly not escalated), one escalated directory path, and one explicitly not-escalated decisions heading. None states an actual stop in that execution. Unmatched or unrecorded stops remain invisible.

Indicator 5: four fresh independent blind readers, /root/measure_after_refoundation/blind_reader_1 through _4, each read one near-equal contiguous partition from each window. No history, other files, aggregate outcomes or cross-talk were permitted. Their prompt supplied this rule verbatim: “a bullet is a judgment item when it names an alternative that was not taken, or a constraint that forced the choice; a bullet that only narrates the diff is not.” The measurer did not override their classes. The new partition reuses exact already-classified after bullet instances. All 1770 IDs were classified exactly once.

| Reader | Before range | Before J / N | After range | After J / N |
|---|---|---:|---|---:|
| 1 | A0001–A0263 | 203 / 60 | B0001–B0178 | 114 / 64 |
| 2 | A0264–A0527 | 191 / 73 | B0179–B0357 | 133 / 46 |
| 3 | A0528–A0791 | 191 / 73 | B0358–B0536 | 151 / 28 |
| 4 | A0792–A1055 | 169 / 95 | B0537–B0715 | 85 / 94 |

Two borderline cases: A0059, “Wrote the unguarded inventory as measured fact, not estimate.” → judgment, because estimation is named as the rejected alternative. B0029, “documenting last (after both code commits landed and passed) keeps the spec from drifting ahead of verified behavior.” → narration, because the reader saw a benefit rather than an alternative or forcing constraint. A because-clause alone is not sufficient. The original SIZE=20 reader identities are unavailable, so these rerun classifications are not a claim of reader identity continuity.

Other ambiguities: the SIZE=20 shared-calendar restriction was read as closure-date overlap because that defines the selector. SIZE=51 resolves both its flag and overlap concerns. Output capture and classification require additional derived scratch files beyond the archive; all stayed inside mktemp trees and are removed with them. This explicit task authorizes the persistent external record. No repository file was used for scratch. An analysis-helper substring check briefly misread n/a inside a branch name; it was replaced by an exact n/a-line prefix before reporting. Manual raw output was unaffected, and final row/pair validation passes.

#### Complete classification ledger

A IDs refer to the before extracted bullet stream; B to after. Order is exactly the manual’s: window stem order, newest commit first, wrapping folded, duplicate commit+bullet strings removed. Every listed ID is judgment; every other valid ID in A0001–A1055 or B0001–B0715 is narration. This preserves all decisions without repeating 1770 long bullets. Stream hashes and exact commands reconstruct their inputs.

A judgment IDs:
```text
A0001 A0002 A0005 A0006 A0007 A0008 A0009 A0010 A0011 A0012 A0014 A0015 A0017 A0018 A0019 A0020 A0021 A0023 A0024 A0025 A0026 A0027 A0028 A0029 A0030 A0031 A0032 A0033 A0035 A0036 A0037 A0038 A0039 A0040 A0043 A0044 A0045 A0046 A0047 A0048 A0049 A0052 A0053 A0054 A0055 A0057 A0058 A0059 A0060 A0063 A0064 A0066 A0067 A0068 A0069 A0070 A0071 A0072 A0073 A0075 A0076 A0077 A0080 A0081 A0082 A0083 A0084 A0086 A0087 A0088 A0089 A0090 A0091 A0092 A0094 A0096 A0099 A0100 A0101 A0103 A0104 A0105 A0106 A0107 A0108 A0111 A0112 A0114 A0115 A0116 A0117 A0118 A0119 A0120 A0121 A0122 A0123 A0124 A0125 A0126 A0127 A0128 A0130 A0131 A0132 A0133 A0134 A0135 A0136 A0137 A0138 A0139 A0140 A0142 A0144 A0145 A0146 A0148 A0149 A0150 A0151 A0152 A0153 A0154 A0155 A0156 A0157 A0158 A0159 A0160 A0161 A0164 A0165 A0166 A0167 A0168 A0169 A0170 A0171 A0172 A0173 A0174 A0175 A0176 A0177 A0178 A0179 A0180 A0181 A0182 A0184 A0185 A0187 A0188 A0189 A0192 A0193 A0194 A0195 A0196 A0198 A0199 A0200 A0201 A0203 A0205 A0210 A0211 A0212 A0213 A0215 A0216 A0217 A0218 A0220 A0221 A0222 A0225 A0226 A0227 A0228 A0229 A0231 A0232 A0233 A0235 A0236 A0237 A0238 A0242 A0243 A0244 A0245 A0246 A0247 A0250 A0251 A0252 A0253 A0255 A0256 A0257 A0258 A0265 A0266 A0268 A0269 A0271 A0272 A0275 A0276 A0277 A0279 A0280 A0281 A0282 A0283 A0284 A0285 A0286 A0287 A0289 A0290 A0291 A0292 A0293 A0294 A0295 A0296 A0297 A0298 A0299 A0300 A0302 A0303 A0304 A0305 A0306 A0308 A0309 A0310 A0311 A0312 A0313 A0314 A0315 A0316 A0317 A0318 A0319 A0321 A0326 A0327 A0328 A0329 A0330 A0332 A0333 A0334 A0335 A0336 A0337 A0338 A0340 A0341 A0342 A0343 A0344 A0345 A0346 A0347 A0348 A0349 A0350 A0351 A0352 A0355 A0357 A0358 A0359 A0360 A0361 A0362 A0363 A0365 A0366 A0367 A0368 A0369 A0371 A0373 A0375 A0376 A0378 A0379 A0381 A0382 A0384 A0386 A0387 A0389 A0390 A0391 A0394 A0396 A0397 A0398 A0399 A0403 A0404 A0406 A0407 A0408 A0409 A0410 A0411 A0413 A0414 A0415 A0419 A0420 A0421 A0424 A0425 A0430 A0431 A0432 A0434 A0435 A0436 A0437 A0440 A0442 A0444 A0445 A0446 A0447 A0448 A0449 A0450 A0451 A0452 A0453 A0455 A0456 A0458 A0462 A0463 A0464 A0465 A0466 A0468 A0469 A0470 A0471 A0473 A0475 A0476 A0478 A0479 A0480 A0481 A0483 A0485 A0486 A0487 A0488 A0489 A0492 A0498 A0499 A0500 A0502 A0503 A0504 A0505 A0507 A0508 A0510 A0511 A0512 A0513 A0514 A0516 A0517 A0518 A0519 A0520 A0521 A0522 A0523 A0524 A0526 A0527 A0529 A0531 A0532 A0533 A0534 A0535 A0536 A0537 A0538 A0540 A0542 A0545 A0546 A0547 A0548 A0550 A0551 A0552 A0555 A0556 A0558 A0559 A0560 A0561 A0562 A0563 A0564 A0565 A0567 A0568 A0569 A0574 A0576 A0578 A0580 A0581 A0582 A0583 A0584 A0586 A0588 A0589 A0592 A0593 A0595 A0598 A0599 A0600 A0601 A0602 A0604 A0606 A0607 A0608 A0609 A0611 A0614 A0615 A0616 A0618 A0619 A0620 A0621 A0622 A0623 A0625 A0626 A0628 A0629 A0630 A0631 A0632 A0633 A0634 A0635 A0636 A0637 A0639 A0641 A0642 A0644 A0646 A0647 A0649 A0650 A0651 A0652 A0653 A0654 A0655 A0656 A0658 A0659 A0661 A0663 A0664 A0665 A0667 A0668 A0669 A0671 A0672 A0673 A0674 A0675 A0676 A0677 A0680 A0681 A0682 A0683 A0684 A0686 A0687 A0688 A0692 A0693 A0694 A0695 A0696 A0697 A0698 A0701 A0703 A0704 A0705 A0706 A0708 A0709 A0710 A0712 A0714 A0716 A0717 A0718 A0719 A0723 A0724 A0725 A0726 A0728 A0730 A0731 A0732 A0733 A0736 A0737 A0738 A0739 A0740 A0741 A0744 A0745 A0746 A0747 A0748 A0749 A0751 A0752 A0753 A0754 A0755 A0756 A0757 A0758 A0759 A0760 A0761 A0762 A0763 A0766 A0767 A0770 A0771 A0772 A0773 A0774 A0775 A0776 A0777 A0778 A0780 A0781 A0782 A0783 A0784 A0785 A0788 A0789 A0790 A0791 A0792 A0793 A0794 A0795 A0796 A0797 A0798 A0800 A0801 A0802 A0804 A0805 A0806 A0809 A0810 A0811 A0812 A0813 A0814 A0816 A0818 A0819 A0820 A0823 A0825 A0829 A0830 A0831 A0832 A0834 A0836 A0840 A0841 A0843 A0844 A0845 A0846 A0847 A0853 A0856 A0857 A0858 A0859 A0860 A0862 A0863 A0864 A0865 A0868 A0869 A0870 A0872 A0873 A0875 A0876 A0877 A0879 A0880 A0881 A0882 A0883 A0885 A0886 A0887 A0888 A0890 A0892 A0893 A0894 A0897 A0899 A0902 A0905 A0906 A0907 A0908 A0909 A0910 A0911 A0912 A0913 A0914 A0915 A0916 A0920 A0922 A0924 A0925 A0926 A0927 A0929 A0930 A0931 A0933 A0934 A0935 A0936 A0937 A0940 A0942 A0945 A0946 A0948 A0949 A0950 A0951 A0953 A0954 A0956 A0957 A0958 A0960 A0961 A0965 A0967 A0968 A0970 A0971 A0973 A0974 A0975 A0976 A0977 A0978 A0979 A0981 A0982 A0983 A0985 A0987 A0988 A0989 A0990 A0991 A0992 A0993 A0994 A0995 A0996 A0997 A0998 A0999 A1000 A1001 A1002 A1004 A1005 A1008 A1009 A1012 A1017 A1018 A1019 A1020 A1023 A1024 A1027 A1028 A1030 A1032 A1036 A1038 A1042 A1043 A1049 A1050 A1052 A1053 A1054
```


B judgment IDs:
```text
B0001 B0005 B0006 B0008 B0009 B0010 B0011 B0012 B0018 B0022 B0024 B0025 B0027 B0028 B0030 B0033 B0034 B0035 B0037 B0038 B0039 B0040 B0041 B0042 B0044 B0045 B0046 B0047 B0048 B0050 B0051 B0052 B0053 B0055 B0058 B0059 B0062 B0066 B0067 B0069 B0070 B0071 B0072 B0073 B0074 B0075 B0076 B0077 B0078 B0079 B0080 B0081 B0085 B0087 B0089 B0090 B0091 B0092 B0094 B0095 B0096 B0097 B0098 B0099 B0100 B0101 B0102 B0105 B0107 B0110 B0111 B0113 B0116 B0117 B0118 B0119 B0121 B0122 B0123 B0125 B0127 B0130 B0133 B0135 B0136 B0138 B0139 B0140 B0141 B0142 B0143 B0144 B0146 B0147 B0148 B0150 B0152 B0153 B0154 B0155 B0156 B0157 B0158 B0160 B0161 B0162 B0164 B0165 B0166 B0167 B0169 B0170 B0173 B0174 B0182 B0183 B0184 B0185 B0188 B0189 B0192 B0193 B0195 B0197 B0201 B0203 B0207 B0208 B0215 B0217 B0218 B0219 B0221 B0222 B0223 B0224 B0225 B0227 B0228 B0229 B0231 B0232 B0234 B0235 B0236 B0237 B0239 B0240 B0241 B0242 B0243 B0244 B0246 B0249 B0250 B0251 B0252 B0253 B0254 B0255 B0256 B0258 B0259 B0260 B0261 B0262 B0263 B0264 B0265 B0266 B0267 B0268 B0269 B0270 B0271 B0272 B0273 B0274 B0275 B0276 B0277 B0278 B0279 B0280 B0281 B0282 B0283 B0284 B0286 B0287 B0288 B0289 B0290 B0291 B0293 B0294 B0296 B0297 B0298 B0299 B0300 B0301 B0302 B0303 B0304 B0305 B0307 B0309 B0310 B0311 B0312 B0313 B0314 B0315 B0318 B0321 B0322 B0323 B0325 B0326 B0327 B0328 B0329 B0330 B0331 B0332 B0333 B0334 B0335 B0336 B0337 B0338 B0339 B0340 B0343 B0344 B0345 B0348 B0349 B0350 B0351 B0352 B0353 B0354 B0355 B0356 B0357 B0359 B0360 B0361 B0364 B0365 B0366 B0367 B0369 B0370 B0371 B0372 B0373 B0374 B0375 B0376 B0377 B0378 B0379 B0380 B0381 B0383 B0384 B0385 B0386 B0388 B0389 B0391 B0393 B0394 B0395 B0396 B0397 B0398 B0399 B0400 B0401 B0402 B0403 B0404 B0405 B0406 B0407 B0408 B0410 B0411 B0412 B0413 B0414 B0415 B0416 B0417 B0418 B0419 B0420 B0421 B0422 B0423 B0424 B0425 B0426 B0427 B0428 B0429 B0430 B0432 B0433 B0434 B0435 B0436 B0437 B0438 B0439 B0440 B0441 B0442 B0443 B0444 B0445 B0446 B0447 B0448 B0449 B0450 B0451 B0452 B0453 B0454 B0455 B0456 B0458 B0459 B0460 B0461 B0462 B0463 B0464 B0465 B0466 B0467 B0468 B0470 B0471 B0472 B0473 B0474 B0475 B0476 B0477 B0479 B0480 B0481 B0482 B0484 B0485 B0486 B0487 B0488 B0489 B0490 B0491 B0492 B0493 B0494 B0495 B0496 B0497 B0498 B0499 B0500 B0501 B0502 B0503 B0504 B0505 B0506 B0507 B0508 B0509 B0512 B0513 B0514 B0516 B0521 B0524 B0525 B0529 B0530 B0531 B0532 B0534 B0536 B0538 B0540 B0541 B0544 B0545 B0546 B0549 B0551 B0552 B0554 B0556 B0559 B0564 B0565 B0566 B0567 B0569 B0570 B0573 B0574 B0575 B0576 B0578 B0581 B0582 B0583 B0584 B0587 B0588 B0591 B0593 B0594 B0595 B0596 B0597 B0598 B0599 B0601 B0602 B0606 B0610 B0611 B0612 B0616 B0617 B0622 B0623 B0625 B0626 B0629 B0630 B0631 B0635 B0636 B0641 B0642 B0645 B0646 B0647 B0648 B0649 B0651 B0653 B0656 B0659 B0660 B0661 B0663 B0673 B0674 B0675 B0678 B0680 B0681 B0687 B0688 B0692 B0695 B0697 B0702 B0704 B0707 B0709 B0712 B0714
```


New partition’s exact B-ID membership:
```text
B0221 B0222 B0223 B0224 B0225 B0226 B0227 B0228 B0229 B0230 B0231 B0232 B0233 B0234 B0235 B0236 B0237 B0238 B0239 B0240 B0241 B0242 B0243 B0244 B0245 B0246 B0247 B0248 B0249 B0250 B0251 B0252 B0253 B0254 B0255 B0256 B0257 B0258 B0259 B0260 B0261 B0262 B0263 B0264 B0265 B0266 B0267 B0268 B0269 B0270 B0271 B0272 B0273 B0274 B0275 B0276 B0277 B0278 B0279 B0280 B0281 B0282 B0283 B0284 B0285 B0286 B0287 B0288 B0289 B0290 B0291 B0292 B0293 B0294 B0295 B0296 B0297 B0298 B0299 B0300 B0301 B0302 B0303 B0304 B0305 B0306 B0307 B0308 B0309 B0310 B0311 B0312 B0313 B0314 B0315 B0316 B0317 B0318 B0319 B0320 B0321 B0322 B0323 B0324 B0325 B0326 B0327 B0328 B0329 B0330 B0331 B0332 B0333 B0334 B0335 B0336 B0337 B0338 B0339 B0340 B0341 B0342 B0343 B0344 B0345 B0346 B0347 B0348 B0349 B0350 B0351 B0352 B0353 B0354 B0355 B0356 B0357 B0358 B0359 B0360 B0361 B0362 B0363 B0364 B0365 B0366 B0367 B0368 B0369 B0370 B0371 B0372 B0373 B0374 B0375 B0376 B0377 B0378 B0379 B0380 B0381 B0382 B0383 B0384 B0385 B0386 B0387 B0388 B0389 B0390 B0391 B0392 B0393 B0394 B0395 B0396 B0397 B0398 B0399 B0400 B0401 B0402 B0403 B0404 B0405 B0406 B0407 B0408 B0409 B0410 B0411 B0412 B0413 B0414 B0415 B0416 B0417 B0418 B0419 B0420 B0421 B0422 B0423 B0424 B0425 B0426 B0427 B0428 B0429 B0430 B0431 B0432 B0433 B0434 B0435 B0436 B0437 B0438 B0439 B0440 B0441 B0442 B0443 B0444 B0445 B0446 B0447 B0448 B0449 B0450 B0451 B0452 B0453 B0454 B0455 B0456 B0457 B0458 B0459 B0460 B0461 B0462 B0463 B0464 B0465 B0466 B0467 B0468 B0469 B0470 B0471 B0472 B0473 B0474 B0475 B0476 B0477 B0478 B0479 B0480 B0481 B0482 B0483 B0484 B0485 B0486 B0487 B0488 B0489 B0490 B0491 B0492 B0493 B0494 B0495 B0496 B0497 B0498 B0499 B0500 B0501 B0502 B0503 B0504 B0505 B0506 B0507 B0508 B0509 B0510 B0511 B0512 B0513 B0514 B0515 B0516 B0517 B0518 B0519 B0520 B0521 B0522 B0523 B0524 B0525 B0526 B0527 B0528 B0529 B0530 B0531 B0532 B0533 B0534 B0535 B0536 B0537 B0538 B0539 B0540 B0541 B0542 B0543 B0544 B0545 B0546 B0547 B0548 B0549 B0550 B0551 B0552 B0553 B0554 B0555 B0556 B0557 B0558 B0559 B0560 B0561 B0562 B0563 B0564 B0565 B0566 B0567 B0568 B0569 B0570 B0571 B0572 B0573 B0574 B0575 B0576 B0577 B0578 B0579 B0580 B0581 B0582 B0583 B0584 B0585 B0586 B0587 B0588 B0589 B0590 B0591 B0592 B0593 B0594 B0595 B0596 B0597 B0598 B0599 B0600 B0601 B0602 B0603 B0604 B0605 B0606 B0607 B0608 B0609 B0610 B0611 B0612 B0613 B0614 B0615 B0616 B0617 B0618 B0619 B0620 B0621 B0622 B0623 B0624 B0625 B0626 B0627 B0628 B0629 B0630 B0631 B0632 B0633 B0634 B0635 B0636 B0637 B0638 B0639 B0640 B0641 B0642 B0643 B0644 B0645 B0646 B0647 B0648 B0649 B0650 B0651 B0652 B0653 B0654 B0655 B0656 B0657 B0658 B0659 B0660 B0661 B0662 B0663 B0664 B0665 B0666 B0667 B0668 B0669 B0670 B0671 B0672 B0673 B0674 B0675 B0676 B0677 B0678 B0679 B0680 B0681 B0682 B0683 B0684 B0685 B0686 B0687 B0688 B0689 B0690 B0691 B0692 B0693 B0694 B0695 B0696 B0697 B0698 B0699 B0700 B0701 B0702 B0703 B0704 B0705 B0706 B0707 B0708 B0709 B0710 B0711 B0712 B0713 B0714 B0715
```


#### Commands as run

Repository /Users/kang-sw/devenv. Primary manual execution used sh. The exact setup and indicator blocks were extracted from the unchanged manual; stdout/stderr were captured separately, SIZE was set to 51 after the default setup and before window, and cleanup was delayed until required reads finished. first_impl, fp_pos, completed_date, epoch, count_marker, window and indicator logic were not edited.

Whole-window commands:
```sh
awk '/^```sh$/{n++;inside=1;if(n==2) {print "SIZE=51"; print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"BRANCH=$BRANCH COMMIT=$COMMIT AT_TIP=$AT_TIP TREE=$TREE SIZE=$SIZE FLOOR=$FLOOR\"";} if(n>=2 && n<=8) print "{";next} /^```$/{if(inside && n>=2 && n<=8) {print "} > \"$TREE/block-" n ".txt\" 2> \"$TREE/block-" n ".stderr\"";if(n==2)print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$STEMS\" > \"$TREE/stems.txt\"";}inside=0;next} inside && n<=8{print} END{print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$bullets\" > \"$TREE/bullets.txt\"";print "for stem in $STEMS; do echo $(completed_date \"$TICKETS/.done/$stem.md\") $stem; done > \"$TREE/window-dates.txt\"";print "echo DATA_READY=$TREE"}' ai-docs/manuals/workflow-cost-measurement.md | COMMIT=84b1f825f5858716833d2f8094ab2848826c6011 USES_BLOCKED=yes USES_DROPPED_PHASE=yes USES_GOAL_MERGE=yes USES_GOAL_BRANCH=yes sh

awk '/^```sh$/{n++;inside=1;if(n==2) {print "SIZE=51"; print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"BRANCH=$BRANCH COMMIT=$COMMIT AT_TIP=$AT_TIP TREE=$TREE SIZE=$SIZE FLOOR=$FLOOR\"";} if(n>=2 && n<=8) print "{";next} /^```$/{if(inside && n>=2 && n<=8) {print "} > \"$TREE/block-" n ".txt\" 2> \"$TREE/block-" n ".stderr\"";if(n==2)print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$STEMS\" > \"$TREE/stems.txt\"";}inside=0;next} inside && n<=8{print} END{print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$bullets\" > \"$TREE/bullets.txt\"";print "for stem in $STEMS; do echo $(completed_date \"$TICKETS/.done/$stem.md\") $stem; done > \"$TREE/window-dates.txt\"";print "echo DATA_READY=$TREE"}' ai-docs/manuals/workflow-cost-measurement.md | COMMIT=cb0458151aecf4979dd05b6c7e702ebbcbf755a2 USES_BLOCKED=yes USES_DROPPED_PHASE=yes USES_GOAL_MERGE=yes USES_GOAL_BRANCH=yes sh
```


New partition command, repeating unchanged indicators 1–5 on the set difference:
```sh
awk '/^```sh$/{n++;inside=1;if(n==2) print "SIZE=51";if(n>=2 && n<=7) print "{";next} /^```$/{if(inside && n>=2 && n<=7) {if(n==2){print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$STEMS\" | sort > \"$TREE/after-stems\""; print "sort /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM/stems.txt > \"$TREE/before-stems\"";print "comm -13 \"$TREE/before-stems\" \"$TREE/after-stems\" > \"$TREE/new-stems\"";print "STEMS=$(printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$STEMS\" | awk '\''\'\'''\''NR==FNR{keep[$0]=1;next} $0 in keep'\''\'\'''\'' \"$TREE/new-stems\" -)";print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$STEMS\" > \"$TREE/stems.txt\"";}print "} > \"$TREE/block-" n ".txt\" 2> \"$TREE/block-" n ".stderr\"";}inside=0;next} inside && n<=7{print} END{print "printf '\''\'\'''\''%s\\n'\''\'\'''\'' \"$bullets\" > \"$TREE/bullets.txt\"";print "for stem in $STEMS; do echo $(completed_date \"$TICKETS/.done/$stem.md\") $stem; done > \"$TREE/window-dates.txt\"";print "echo DATA_READY=$TREE"}' ai-docs/manuals/workflow-cost-measurement.md | COMMIT=cb0458151aecf4979dd05b6c7e702ebbcbf755a2 sh
```


Resolved TREE values: before /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM; after /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5; new partition /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.oL2shGui4E. For each TICKETS=$TREE/ai-docs/tickets and FPCHAIN=$TREE/first-parent-chain. BRANCH=develop, SIZE=51, FLOOR=5; both pinned hashes derived AT_TIP=no. The new partition does not repeat indicator 6 because its whole tree is identical to the measured after tree.

Exact mechanical partitioning and summary scripts, executed with node. They format printed data and classifications, not substitute measurement rules:
```javascript
const fs=require('fs');
const dirs=['/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM','/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5'];
let parts=Array.from({length:4},()=>[]);
dirs.forEach((dir,wi)=>{
const bullets=fs.readFileSync(dir+'/bullets.txt','utf8').trimEnd().split('\n');
console.log(wi,bullets.length);
for(let p=0;p<4;p++){
 const start=Math.floor(p*bullets.length/4),end=Math.floor((p+1)*bullets.length/4);
 parts[p].push(...bullets.slice(start,end).map((b,i)=>(wi?'B':'A')+String(start+i+1).padStart(4,'0')+' '+b));
 console.log('partition',p+1,start+1,end);
}
});
parts.forEach((rows,p)=>fs.writeFileSync(dirs[0]+'/reader-'+(p+1)+'.txt',rows.join('\n')+'\n'));
```

```javascript
const fs=require('fs');
const roots={before:'/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM',after:'/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5',fresh:'/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.oL2shGui4E'};
const bad={ '73fc63eb':'Defective repair anchor: Phase 1 recovery-verification repair, original archive work predates it.',
'863f4014':'Defective administrative anchor: feat(ticket) creates the ready ticket; it implements no product behavior.',
'd0cf6b80':'Defective repair anchor: review repair relocates an existing repo-note flock.',
'e39e2018':'Defective partial-phase anchor: self-migration follows the Phase 1 bootstrap-template implementation f652a0d8.',
'fbec365f':'Defective wrong-ticket anchor: note-capture documentation names this runtime-contract bug only as a pre-existing failure.',
'f3ac6a20':'Defective repair/late anchor: repairs Phase 2 release-gate behavior after earlier implementation.',
'10d38f80':'Defective late anchor: Phase 2 reviewer handoff, omitting Phase 1 planner/implementer work.',
'444bcb8a':'Defective wrong-ticket anchor: July ready-gate/epic-reference change mentions future workset retirement.',
'835c9d85':'Defective wrong-ticket anchor: layer-retirement sibling explicitly leaves bootstrap migration untouched.',
'73ee3609':'Defective administrative anchor: fix(tickets) promotes/redesigns the ticket; no config implementation.',
'f262e488':'Defective wrong-ticket anchor: workset retirement reports the symlink bug as pre-existing.',
'0c884ce4':'Defective repair anchor: repairs golden left stale by implementation 61f6351c; raw row is n/a.'
};
const repairs={efe795eb:'Review repair: replace per-group mental-model dispatch with one whole-window sweep.',
'476f1562':'Review repair: de-duplicate fixture and replace tautological gate test with ground-truth assertions.',
'6845444b':'Review repair: remove duplicated procedural invariant from shipped skill.',
'b119c658':'Review repair: close the no-tag-silent test coverage finding.',
'7364c9a0':'Verification-contract repair: fix self-contradictory dashboard teardown scan requirement.',
'd1270d13':'Workflow-record repair: recover Phase 1 Result committed against the wrong detached parent.',
'11cd2603':'Fresh-reader audit repairs to four shipped lead skill bodies.',
'fa490ada':'Round-2 observation repair: correct Result commit citations.',
'92389a0f':'Fresh-reader audit repairs to route-fact syntax, edit scope, and tier instruction.'};
function dist(xs){xs=xs.filter(x=>typeof x==='number').sort((a,b)=>a-b);return {n:xs.length,min:xs[0]??null,median:xs.length?(xs[Math.floor((xs.length-1)/2)]+xs[Math.floor(xs.length/2)])/2:null,max:xs.at(-1)??null,frequencies:xs.reduce((o,x)=>(o[x]=(o[x]||0)+1,o),{})}}
function load(name,root){
 const read=n=>fs.readFileSync(root+'/block-'+n+'.txt','utf8').trimEnd();
 const stems=fs.readFileSync(root+'/stems.txt','utf8').trim().split('\n');
 const dates=fs.readFileSync(root+'/window-dates.txt','utf8').trim().split('\n');
 const latency=read(3).split('\n').map(s=>{const [stem,d,c]=s.split(' ');return {stem,days:/^days=\d+$/.test(d)?+d.slice(5):d.slice(5),gap:/^commits=\d+$/.test(c)?+c.slice(8):c.slice(8)}});
 const typeRows=read(4).split('\n').map(s=>{const [stem,...rest]=s.split(' ');return {stem,counts:Object.fromEntries(rest.filter(Boolean).map(x=>{let [k,v]=x.split('=');return[k,+v]}))}});
 const typeTotal={};for(const r of typeRows)for(const[k,v]of Object.entries(r.counts))typeTotal[k]=(typeTotal[k]||0)+v;
 const i3=[];let row; for(const line of read(5).split('\n')){if(/^\d/.test(line)){row={stem:line.split(' ')[0],raw:line,posts:[]};const m=line.match(/anchor=(\w+) (.*)/);if(m){row.anchor=m[1];row.subject=m[2];row.verdict=bad[row.anchor]||'Plausible first implementation: subject aligns with the first listed phase (or one-phase bug fix).';row.sound=!bad[row.anchor];row.dateScope=/^[a-z]+\(\d{6}\):/.test(row.subject)}else{row.unavailable=true;row.verdict='Unavailable: no product-typed stem-matching implementation selected.'}i3.push(row)}else{const p=line.match(/corrective=(\d+) of (\d+)/);if(p){row.corrective=+p[1];row.total=+p[2]}else if(line.startsWith('  n/a ('))row.na=true;else if(/^    /.test(line)){const hash=line.trim().split(' ')[0];row.posts.push({hash,subject:line.trim().slice(hash.length+1),manualRepair:repairs[hash]||null})}}}
 const esc=read(6).split('\n').filter(s=>/^\d/.test(s)).map(s=>({stem:s.split(' ')[0],printed:s.includes('escalation_lines=')?+s.split('=').at(-1):null,judged:s.includes('escalation_lines=')?0:null}));
 const bullets=fs.readFileSync(root+'/bullets.txt','utf8').trimEnd().split('\n');
 const sumPair=arr=>({corrective:arr.reduce((a,r)=>a+(r.corrective||0),0),post:arr.reduce((a,r)=>a+(r.total||0),0),measurable:arr.filter(r=>r.total!==undefined).length});
 return {name,root,stems,dates,latency,typeRows,typeTotal,i3,esc,bullets,raw:Object.fromEntries([2,3,4,5,6,7,...(name!=='fresh'?[8]:[])].map(n=>[n,read(n)])),stats:{N:stems.length,days:dist(latency.map(r=>r.days)),gaps:dist(latency.map(r=>r.gap)),types:typeTotal,references:Object.values(typeTotal).reduce((a,b)=>a+b,0),distinct:+read(7).split('\n')[0].split(': ').at(-1),pair:sumPair(i3),soundPair:sumPair(i3.filter(r=>r.sound)),i3Unavailable:i3.filter(r=>r.unavailable).length,i3NA:i3.filter(r=>r.na).length,defective:i3.filter(r=>r.anchor&&!r.sound).length,sound:i3.filter(r=>r.sound).length,dateScope:i3.filter(r=>r.dateScope).map(r=>[r.stem,r.anchor]),manualRepairs:i3.flatMap(r=>r.posts.filter(p=>p.manualRepair).map(p=>[r.stem,p.hash,p.manualRepair])),escPrinted:esc.reduce((a,r)=>a+(r.printed||0),0),escUnavailable:esc.filter(r=>r.printed===null).length,escDistribution:dist(esc.map(r=>r.judged)),bullets:bullets.length}};
}
const data=Object.fromEntries(Object.entries(roots).map(([name,root])=>[name,load(name,root)]));
fs.writeFileSync(roots.before+'/data.json',JSON.stringify(data,null,2));
console.log(JSON.stringify(Object.fromEntries(Object.entries(data).map(([k,v])=>[k,v.stats])),null,2));
```

```javascript
const fs=require('fs'),r='/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM',d=require(r+'/data.json');
const cls={};let readerCounts=[];
for(let p=1;p<=4;p++){const input=fs.readFileSync(r+'/reader-'+p+'.txt','utf8').trim().split('\n').map(l=>l.split(' ')[0]);const rows=fs.readFileSync(r+'/classes-'+p+'.tsv','utf8').trim().split('\n');const counts={p,A:{J:0,N:0},B:{J:0,N:0}};if(rows.length!==input.length)throw Error('length');for(const l of rows){const[id,c]=l.trim().split(/\s+/);if(!input.includes(id)||cls[id]||!['J','N'].includes(c))throw Error('bad class '+l);cls[id]=c;counts[id[0]][c]++;}readerCounts.push(counts);}
let bulletClass={};for(const[name,prefix]of [['before','A'],['after','B']]){const v=d[name];v.classifications=v.bullets.map((b,i)=>({id:prefix+String(i+1).padStart(4,'0'),bullet:b,class:cls[prefix+String(i+1).padStart(4,'0')]}));v.stats.J=v.classifications.filter(x=>x.class==='J').length;v.stats.N=v.bullets.length-v.stats.J;if(name==='after')bulletClass=Object.fromEntries(v.classifications.map(x=>[x.bullet,x]));}
d.fresh.classifications=d.fresh.bullets.map(b=>{if(!bulletClass[b])throw Error('new bullet not in after');return bulletClass[b]});
d.fresh.stats.J=d.fresh.classifications.filter(x=>x.class==='J').length;d.fresh.stats.N=d.fresh.bullets.length-d.fresh.stats.J;
let identical=0,discordant=0;const beforeMap=Object.fromEntries(d.before.classifications.map(x=>[x.bullet,x.class]));for(const x of d.after.classifications){if(beforeMap[x.bullet]){identical++;if(beforeMap[x.bullet]!==x.class)discordant++;}}
d.readerCounts=readerCounts;d.identicalBullets={identical,discordant};d.classes=cls;
for(const v of [d.before,d.after,d.fresh]){if(v.i3.length!==v.stems.length||v.stats.i3Unavailable+v.stats.i3NA+v.stats.pair.measurable!==v.stems.length)throw Error('rows count');for(const row of v.i3)if(row.total!==undefined&&row.posts.length!==row.total)throw Error('posts count '+row.stem);if(v.stats.J+v.stats.N!==v.bullets.length)throw Error('bullet count');}
fs.writeFileSync(r+'/data.json',JSON.stringify(d,null,2));
console.log(JSON.stringify({readers:readerCounts,sharedBulletClasses:d.identicalBullets,windows:Object.fromEntries(['before','after','fresh'].map(k=>[k,{J:d[k].stats.J,N:d[k].stats.N,bullets:d[k].stats.bullets}]))},null,2));
```


Additional evidence commands used pinned git show/git grep for the manual, AGENTS.md and convention sources above; git log -1 --format="%H %cI %s" at both hashes; git log <hash> --merges --format="%h %s" --grep="merge(goal)"; phase-heading rg reads from archived tickets; and git show -s --format="%h %s%n%b" for named suspicious anchors and repair candidates. The identical manual blob was verified using git show <hash>:ai-docs/manuals/workflow-cost-measurement.md | git hash-object --stdin at d0cb62b5 and the after hash.

Fence parity command:
```sh
for tree in /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5; do echo "$tree"; for file in "$tree"/ai-docs/tickets/*/*.md "$tree"/ai-docs/tickets/.done/*.md "$tree"/ai-docs/tickets/.dropped/*.md; do test -f "$file" || continue; awk '/^[ \t]*(```|~~~)/{n++} END{if(n%2)print FILENAME,n}' "$file"; done; done
```


#### Verification and limits

All three extraction processes exited 0. Window stderr contained exactly 48 before skips, 51 after skips and 51 new-partition setup skips; every indicator stderr file was empty. Both whole windows printed full (51) and no unrepresentative flag. Validation checked per-ticket row counts, post-subject counts against denominators, all classification IDs, category totals and exact new-partition bullet membership. No source test suite ran because this is read-only history analysis.

Before raw window:
```text
window: full (51)
skipped (no completed:): 48
```

After raw window:
```text
window: full (51)
skipped (no completed:): 51
```


Before skipped stems:
```text
skipped (no completed:): 260421-feat-delegate-implement-feature-branch
skipped (no completed:): 260422-chore-rename-delegate-implement-to-implement
skipped (no completed:): 260422-chore-rename-implement-to-edit
skipped (no completed:): 260422-chore-workflow-chain-drift
skipped (no completed:): 260422-chore-write-ticket-workflow-drift
skipped (no completed:): 260422-feat-proceed-full-pipeline
skipped (no completed:): 260422-feat-write-ticket-review
skipped (no completed:): 260423-feat-doc-system-gap-fixes
skipped (no completed:): 260423-feat-proceed-mandatory-ticket
skipped (no completed:): 260424-feat-discuss-on-demand-survey
skipped (no completed:): 260424-feat-domain-rules-layering
skipped (no completed:): 260424-feat-infra-path-portability
skipped (no completed:): 260424-feat-polish-plugin-docs
skipped (no completed:): 260424-feat-project-survey-agent
skipped (no completed:): 260424-refactor-implement-file-based-review
skipped (no completed:): 260424-refactor-proceed-gate-suppression
skipped (no completed:): 260424-refactor-team-free-orchestration
skipped (no completed:): 260425-chore-implementation-gap-staleness-flagging
skipped (no completed:): 260426-feat-claude-dash
skipped (no completed:): 260513-feat-is-finished-yet-workflow-check
skipped (no completed:): 260513-feat-ticket-result-editions
skipped (no completed:): 260516-bug-ws-web-terminal-cross-platform-portability
skipped (no completed:): 260517-feat-workflow-single-phase-contract-skeleton
skipped (no completed:): 260523-bug-ws-mcp-launcher-runtime-repair-race
skipped (no completed:): 260523-feat-ws-dashboard-linked-worktree-discovery
skipped (no completed:): 260523-feat-ws-dashboard-readonly-file-pane-restore
skipped (no completed:): 260523-feat-ws-dashboard-tool-output-safe-summary
skipped (no completed:): 260524-feat-ws-dashboard-workspace-forget-remove-ui
skipped (no completed:): 260524-feat-ws-dashboard-workspace-root-prune-policy
skipped (no completed:): 260525-bug-lead-implement-delegation-pre-edit-guard
skipped (no completed:): 260525-bug-local-runtime-contract-marker
skipped (no completed:): 260616-refactor-remove-agent-backed-api-tools
skipped (no completed:): 260617-refactor-mcp-stateless-subagent-context
skipped (no completed:): 260617-refactor-ws-session-bootstrap-obscurity
skipped (no completed:): 260618-bug-ws-prefer-mercenary-one-way-flip
skipped (no completed:): 260619-feat-ws-config-prompt-tool-self-doc
skipped (no completed:): 260619-feat-ws-layered-config-scope-substrate
skipped (no completed:): 260619-feat-ws-lead-tune-skill
skipped (no completed:): 260619-feat-ws-prompt-override-marker-engine
skipped (no completed:): 260619-feat-ws-session-lineage-children
skipped (no completed:): 260620-bug-ws-tier-vocabulary-split-undocumented
skipped (no completed:): 260620-chore-pre-shipping-windows-surface-verification
skipped (no completed:): 260624-feat-tickets-template-tool-and-convention-diet
skipped (no completed:): 260624-feat-workflow-lead-language-config
skipped (no completed:): 260627-bug-enter-implement-direct-edit-policy-gap
skipped (no completed:): 260724-bug-windows-mcp-mid-session-disconnect
skipped (no completed:): 260725-feat-ws-cli-mcp-fallback-surface
skipped (no completed:): 260827-bug-impl-branch-stem-word-key
```


After has those 48 plus 260909-bug-code-reviewer-delegate-switches-shared-worktree-branch, 260909-bug-git-commit-emits-updated-tickets-heading, and 260909-bug-sage-stamp-pass-leaves-stale-blocked-section.

No repository file was edited, staged or committed. Before assembly, git status --porcelain=v1 printed nothing and git diff --quiet plus git diff --cached --quiet passed. Final exact scratch removal and cleanliness verification are recorded in the final appended paragraph.

The manual does not measure wall-clock time, tokens, lead turns, quality or causal abort/re-work rates. New stems need not have begun under the new topology. More repair commits may mean harder work or better review; zero recorded stop lines may mean changed recording. This complete comparison supplies qualitative evidence, not authority to drop, close, merge or release the epic.

#### Final cleanup verification

At 2026-09-12T16:10:17+09:00, all three scratch trees were absent, git status --porcelain=v1 printed no entries, and git diff --quiet plus git diff --cached --quiet exited 0. The external report remains the sole persistent artifact; repository files and the repository index are unchanged.

The execution safety check rejected the manual's rm -rf spelling because forced removal commands are not permitted. Cleanup succeeded with the safer non-forced rm -r on the three exact validated mktemp paths. This is the sole teardown deviation; no scratch remains.

```sh
test -f /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM/first-parent-chain &&
test -f /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5/first-parent-chain &&
test -f /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.oL2shGui4E/first-parent-chain &&
rm -r /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5 /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.oL2shGui4E

test ! -e /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.BVpBI1QLtM &&
test ! -e /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.zn3n7jEmm5 &&
test ! -e /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/tmp.oL2shGui4E &&
git status --porcelain=v1 &&
git diff --quiet &&
git diff --cached --quiet &&
date -Iseconds
```

No blocker or required judgment remains omitted. The unavailable values described above are prescribed measurement outputs, not unfinished work.
