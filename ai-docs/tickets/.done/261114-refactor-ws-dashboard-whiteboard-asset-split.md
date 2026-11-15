---
title: "Mechanically split dashboard and whiteboard assets at the UI cut line"
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: "Landed 652 source and oversized-file advisories; preserve its visual behavior"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6e9424ad230fd815
sage-review-completeness-reviewed: 6e9424ad230fd815
completed: 2026-11-15
---

# Mechanically split dashboard and whiteboard assets at the UI cut line

## Background

Owner decision (793)=a authorizes a behavior-preserving split of oversized dashboard and whiteboard assets. Large files make concurrent UI changes and mechanical rebases expensive. MAIN supplied the explicit cut-line start signal on 2026-11-14 at 17:27:36Z: 649, MAIN's test batch and 789 are integrated in baseline `d0ad8a9f1`. Promote through design/completeness review, then use a parallel worker for the mechanical split. H CRLF repair retains first priority; tests require a separately allocated heavy slot and a bounded recipe.

The source inventory at develop `cacd771c1ab69c01fbc33ffb1db74df0bb6dc3d9` is:

| Existing file | Lines | Split unit |
|---|---:|---|
| `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` | 9,221 | Existing responsibility sections: storage/time/disclosure and scroll state; board region/morph/frame ownership; conversation/history; composer inputs and attachments; delegated actions and drawer/worktree controls. |
| `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` | 2,113 | Existing palette/base/layout rules; session/conversation/composer chrome; board/card/fold/maximum controls; toolbar/popovers/navigation; frame and layer rules. |
| `agents-plugin-tool/internal/wswhiteboard/assets/page.css` | 776 | Existing palette/base rules; topic headers and linked tickets; stream/body/panel styling; sticky head/capsule geometry. |
| `agents-plugin-tool/internal/wswhiteboard/assets/*.js` | Already separate files | Preserve the existing script responsibilities and registrations while extracting any coupled regions needed by the agreed split; do not consolidate them into a new monolith. |
| `scripts/board-headless.mjs` | 3,627 | The additional 652 size advisory: separate existing shared-action and compact-toolbar/overview scenario groups mechanically, retaining the command entry point and all predicates. |

There is **no** `wswhiteboard/assets/page.js` in this source. The whiteboard JavaScript is already split among `stream.js`, `rail.js`, `form.js`, `pill.js`, `doc-frame.js`, and other registered assets. The owner's shorthand "page.css/js" refers to this actual whiteboard asset surface, not authority to invent or rename a `page.js` entry point. The inventory is a capture-time baseline, not a fixed line-count target or a mandate to change every small script.

## Decisions

- **Mechanical split only — (793)=a.** Move existing behavior into responsibility-sized files and update its loading/embedding and tests. Preserve behavior, selectors, cascade, initialization order, scope and ownership. Reject feature development, redesign, opportunistic fixes, renamed public interfaces and a framework/module-system migration in this refactor.
- **Cut line, not a far-branch barrier.** Start immediately after both 649 and MAIN's test batch have merged: 758; 773 plus sticky work (`c4e033c5f`); close-copy (`2fa254b`); usage meter (`292e20895`); UI polish (`989fdd169` / `76f32c0e6`); and 789 jump-follow. These identifiers are MAIN's relayed batch pointers, not a claim that every item has already merged. MAIN confirms the frontier and explicitly starts this work. Rejected: option d's full join of all in-flight work. Do not wait for TASK3, TASK5, 723, 675 or other distant branches; those continue and mechanically rebase onto the new layout afterward.
- **Pause new UI starts during the split.** Worktree view, file explorer, 781 and selection-bundle UI do not newly start while the split is active. This is a start pause, not cancellation of their tickets or a request to stop already-running distant branches.
- **Publish a move map.** Record each old file and recognizable region/symbol group against its new file, plus the changed embedding/template sites and load order. Ongoing workers use this map to rebase without re-deriving the extraction.
- **Compatibility: T0 (asset content).** This work changes internal asset organization, not MCP tools/arguments, protocol behavior, tracker events, store shape or configuration fields. Existing caller-visible behavior remains unchanged.
- **Acceptance includes tests and before/after screenshots.** Capture the actual cut-line baseline, not an older visual reference, and compare the result side by side at matching routes, viewports, state and motion settings. Neither a file move nor syntax checks alone prove behavior preservation.
- **MAIN's 17:59:12 repin.** Integrate exact develop `aac39c69f381a82677f0ee6857e8f8fc5bc89322` before the split's future slot, preserving its landed UI-C2 history-boundary and UI-C3 topic-unfold/beforeGeometry changes. That exact source is now the actual BEFORE comparison pin, replacing historical `d0ad8a9f1`; unmerged 777 and UI-T5 remain excluded. MAIN owns integration; reconciliation and recipe edits remain static-only while H runs.

- **MAIN's 2026-11-15 exact-099 rework supersedes the earlier verification allocation.** Mechanically reconstruct/re-extract all four originals from `09998d57986f9d0f716036115452e1cb912d1f28`, preserving every upstream edit. MAIN owns the started merge; finish its conflict resolution without re-pinning to newer develop. Run the same seven Node commands and both complete Go packages offline by approximately 06:00Z. No new browser matrix, screenshots, old-baseline harness/Lite chase or shared-runtime work is allocated; old Lite evidence applies only to its old head. Preserve the combined browser-gap ticket and all original failures. Publish the [current map](../assets/261114-refactor-ws-dashboard-whiteboard-asset-split/move-map-09998d579.md) and record fresh command exits separately from historical results.

## Constraints

- MAIN's cut-line start signal is satisfied at baseline `d0ad8a9f1`; complete the normal fact-population and design/completeness gates before promotion and worker implementation. The start signal authorizes move-only T0 implementation in a parallel worktree, not test execution, release, a daemon rebuild/restart/swap, profiling or an installed-owner test. Send the bounded test recipe for a heavy-slot allocation.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before editing `agents-plugin-tool/internal/`; the split must preserve their downstream and compatibility contracts. If a planned edit reaches a skill, playbook, prompt or convention, read `ai-docs/manuals/skill-authoring.md` and stop before expanding this asset-only scope.
- Update the actual `//go:embed` declarations and rendering/template registration sites: dashboard assets are registered in `agents-plugin-tool/internal/wsdashboard/pages.go`; whiteboard assets in `agents-plugin-tool/internal/wswhiteboard/render.go`. Preserve their emitted asset order and the current whiteboard-before-dashboard CSS order.
- Keep existing JavaScript state ownership, closures, event-handler registration, delegated events, frame identities and browser storage semantics. Extraction must not create duplicate handlers, implicit new globals, asynchronous loading races or a new shared API.
- Preserve CSS selector specificity, relative cascade order, URLs and layer/stacking semantics. Split boundaries must not change geometry, typography, animation, focus visibility or hit ownership.
- Tests currently reading a single embedded file must continue to verify the assembled behavior after extraction; do not remove assertions because their old source-file locations moved.
- Preserve all shared-controls/shared-actions and 652 compact-toolbar/overview harness scenarios and command syntax. Known acceptance limitations in the cut-line source stay explicit; they are not silently fixed, erased or converted to passes by this refactor.
- Choose final extraction filenames from the existing responsibilities at the cut line and record them in the move map; this ticket does not introduce a new exported module or caller-visible interface.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 184e0755 (2026-11-15, commit): "Remove fixed/minimum recap height but retain the shared four-line ellipsis clamp and matching maximum height in both sticky states." — bearing: constrains
- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Result): "Linked-ticket badges and ticket-derived live styling are hidden, not the accepted attribute or its validation" — bearing: constrains
- 3a13a369 (2026-11-14, commit): "Mount after wbForm creates wbHistory, rather than the earlier card setup." — bearing: constrains
- 0a438cdb (2026-11-14, commit): "load UsageScript in the shell owning the rail, as the iframe-shell ticket explicitly requires." — bearing: constrains
- 261114-feat-ws-dashboard-worktree-view (2026-11-14, Decisions): "Capturing the idea does not bypass MAIN's cut-line scheduling or start signal." — bearing: supports
- 5695371f (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains
- 562bfeb9 (2026-11-12, commit): "One JS builder instead of two: answers.js is a self-contained asset" — bearing: constrains
- 261108-feat-ws-dashboard-disk-assets-dev-mode (2026-11-08, Decisions): "Daemon-served pages only" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, dashboard.css, pages.go; agents-plugin-tool/internal/wswhiteboard/assets/page.css, existing scripts, render.go; scripts/board-headless.mjs and affected source-reading tests. The historical inventory counts match git show cacd771c1:<path> piped to wc -l. At d0ad8a9f1, wc -l returns 9234, 2116, 805 and 3923 respectively for the four counted files. git merge-base --is-ancestor <ref> d0ad8a9f1 returns 0 for cacd771c1 (652), a33221747 (758), 1771e2140 (773), c4e033c5f, 2fa254b, 292e20895, 989fdd169, 76f32c0e6 and 5573bce92 (789); d0ad8a9f1 itself merges 649. |
| scope.surface | cross-module | VERIFIED: Dashboard assembly consumes wswhiteboard.PageAssets in agents-plugin-tool/internal/wsdashboard/pages.go#L191-L237; whiteboard also emits standalone and frame assets through agents-plugin-tool/internal/wswhiteboard/render.go#L135-L167. No external interface change is approved. |
| scope.new_public_symbol | no | VERIFIED: Decisions and Phase 1 exclude renamed public interfaces, a new shared API and module-system migration; extraction filenames are implementation details. |
| scope.new_type_contract | no | VERIFIED: Phase 1 preserves closure/state ownership and existing loading contracts; no new signature, wire type or persisted shape is requested. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/board_keep_test.go#L15-L38 and #L77-L91 read dashboard.js and extract functions; stream_script_test.go#L192-L215 under agents-plugin-tool/internal/wswhiteboard checks assembled scripts/CSS. Additional readers are found by rg -n -e 'dashboard[.]js' -e 'dashboard[.]css' agents-plugin-tool/internal/wsdashboard --glob '*test.go'. scripts/board-headless.mjs#L3866-L3886 registers shared-controls, shared-actions, toolbar, overview and motion/history scenarios. |
| complexity.reuse_points | confirmed | VERIFIED: Read existing get-based builders, embed lists, assetID, and template order: agents-plugin-tool/internal/wsdashboard/pages.go#L34-L35, #L191-L278; agents-plugin-tool/internal/wswhiteboard/render.go#L33-L34, #L135-L167; agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L5-L46. These are reuse boundaries, not a new extraction mechanism. |
| complexity.side_effect_risk | high | INFERRED: Moving initialization and shared closure regions can reorder handlers, break storage ownership or change CSS cascade across standalone, dashboard and frame consumers. |
| risk.correctness | high | INFERRED: Phase 1 must preserve scroll/focus, frame identity and asynchronous live updates; existing board_keep_test.go#L15-L65 pins ordering across morph, history restore and late frame height compensation. |
| risk.fit | moderate | INFERRED: Existing builders and responsibility sections support mechanical extraction, but closure boundaries must be retained rather than replaced with a new exported API. |
| risk.test | high | INFERRED: Many tests extract source spans from a single file; syntax checks cannot prove assembled behavior or visual equality. The actual comparison baseline, repinned by MAIN to exact aac39c69f381a82677f0ee6857e8f8fc5bc89322, is still to be captured in the split's separately granted heavy slot, not an already-approved mockup. git show -s --format=%B cacd771c1 records owner-accepted open picker/card Important and an incomparable earlier baseline, not full 652 acceptance. |
| risk.security_or_contract | moderate | INFERRED: T0 asset organization must retain trusted frame dispatch, sandboxed document loading and existing action availability; agents-plugin-tool/internal/wswhiteboard/render.go and agents-plugin-tool/internal/wsdashboard/pages.go are assembly boundaries, not authority to change those contracts. |

## Historical Static Preparation — Before MAIN's Heavy-Slot START

Static move-only T0 preparation is retained on `impl/develop/map-yoga-coach`.
MAIN started integration of exact `aac39c69f381a82677f0ee6857e8f8fc5bc89322`
into preparation `961c8e3bf1a243b602e9c541b101747a6df8ea26`; the worker resolves
that existing merge, not a rebase or another integration. The new source pin
includes landed UI-C2/UI-C3. This is not a Phase 1 Result or acceptance.
No tests, syntax/byte checks, imports, formatters, generators, builds, browsers,
network/daemon/owner operations or process signals ran. H's actual slot grants
no split execution. MAIN already acquired the lease; no acquisition was repeated.

- [Move map and peer patch landing points](../assets/261114-refactor-ws-dashboard-whiteboard-asset-split/move-map.md).
- [One bounded offline verification/baseline recipe, not run](../assets/261114-refactor-ws-dashboard-whiteboard-asset-split/verification-recipe.md).
- The merge retains landed UI-C2 history-boundary handling, UI-C3 owner export and
  pre-geometry callback ordering, and all incoming regression assertions. The two
  dashboard changes land in `dashboard-frames.js`/`dashboard-tracker.js`; source
  readers remain assembled. Unmerged 777/UI-T5 changes stay excluded.
- Independent static lite review 8092 covered `b8989519a..961c8e3bf`, found no
  Critical and three Important recipe issues. Static corrections select the
  narrow board tab before visible-topic waiting, mark shared-controls blocked
  without its separately approved coherent exited-terminal fixture, and specify
  cumulative deadlines/operation budgets/cleanup reserve/owned-exit handoff.
  No re-review is requested under lite's one-pass allocation.
- The one combined recipe remains NOT START-ready: its actual enforcement runner
  and exited-terminal prerequisite are not supplied by this asset-only scope.
  Baseline-first failures can leave all candidate coverage explicitly omitted.
  Exact source/behavior preservation remains unverified; executable verification,
  side-by-side comparison, Result and closure are deferred. The UI-start pause
  remains MAIN-owned; peer test receipts are not this ticket's acceptance.

## Phases

### Phase 1: Split the cut-line assets and publish the mechanical rebase map

After MAIN's explicit start signal, pin the actual merged cut-line commit and record the before state. Execute the concrete units above as behavior-preserving extractions: dashboard runtime sections, dashboard stylesheet sections, whiteboard stylesheet/script loading boundaries, and the 652-flagged harness scenario groups. Include each unit's embedding/template/loader and source-reading test updates in the same logical extraction. Do not treat the capture-time regions as permission to redesign their interfaces.

Publish the old-region-to-new-file move map alongside the implementation evidence. Explain any region kept together because splitting it would change closure/state ownership rather than mechanically move code. Keep unrelated UI development outside this ticket.

Verification:

- Run the relevant dashboard and whiteboard Go tests, JavaScript syntax checks, embedded-asset availability/load-order checks, and existing focused behavior tests affected by moved regions. Record exact commands and outputs; no new full-suite or acceptance rounds are implied by ticket authoring.
- Compare matching before/after screenshots side by side for dashboard and standalone whiteboard, normal and narrow layouts, full and floating toolbar states, and affected conversation/composer, card, stream and sticky-header surfaces. Cover relevant light/dark and reduced-motion settings without changing those policies.
- Exercise affected initialization, controls, scroll/focus preservation and frame/card identity paths using the existing harness. Keep all assertions and report any known baseline failure distinctly from a new failure.
- Verify the published move map covers every moved region and all registration changes, so ongoing TASK3/TASK5/723/675 and other branches can rebase mechanically.
- Report the pause boundary and hand control back to MAIN before new UI starts resume. MAIN alone chooses and performs any daemon deployment; a worker does not infer deployment authority from this ticket.

## Integration Result (0c46ba103) - 2026-11-15

This records MAIN's bounded exact-099 rework/handoff, not full browser acceptance.
MAIN merged `81382b093`; fresh wswhiteboard run by merge side, 6 pre-existing reds only.
MAIN authorized ticket closure on 2026-11-15 at 06:01:23Z.

- **Integration:** `0c46ba103ccab58f8c432dbe7f0a2f998e879d91` concludes MAIN's
  pending merge, parents `922a2d81abbbd7c6351f6a782c38c0db533f58a8` and exact
  `09998d57986f9d0f716036115452e1cb912d1f28`. All four originals were rebuilt
  from 099 before re-extraction. The [current map](../assets/261114-refactor-ws-dashboard-whiteboard-asset-split/move-map-09998d579.md)
  records every range; no newer develop head or old split side was substituted.
- **Verified source contract:** the three separator-free assembled assets equal
  exact 099 byte-for-byte; the entire upstream harness is reconstructed by
  inversing only the explicit factory/source-reader adapters. Single-IIFE scope,
  evaluation order, CSS cascade and upstream fixture/history edits are retained.
- **Seven Node commands, all exit 0 at tested head 0c46ba103:**
  `node --test scripts/board-headless/split.test.mjs` (5 tests, none skipped),
  `node --check scripts/board-headless.mjs`,
  `node --check scripts/board-headless/toolbar-overview.mjs`,
  `node --check scripts/board-headless/shared-actions.mjs`,
  `node agents-plugin-tool/internal/wsdashboard/testdata/lead-popover.test.cjs`,
  `node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs`,
  `node --test agents-plugin-tool/internal/wsdashboard/testdata/task_badges.test.cjs`.
  `BOARD_SPLIT_BASELINE` was the full exact-099 hash.
- **Full dashboard:** `go test ./internal/wsdashboard -count=1 -timeout=5m`
  exited 1 (172.457s). Failing tests: `TestCSSClassesDoNotCollideAcrossModules`
  (.d/.over/.wait), `TestLabelRouteSavesShowsAndResets`,
  `TestOverviewPageIntegration`, `TestGearHoldsTheThemeSection`. This is not PASS.
- **Required attribution:** on MAIN's clean exact-099 checkout and candidate
  0c46ba103, `go test ./internal/wsdashboard -run
  '^Test(LabelRouteSavesShowsAndResets|OverviewPageIntegration|GearHoldsTheThemeSection)$'
  -count=1 -timeout=60s` exited 1 on both. All three fail with the same 213
  assertion lines; no candidate-only failure or repair. MAIN's 05:45:57 relay
  independently classifies the CSS failure as pre-split baseline/nonblocking;
  this worker did not rerun or repair CSS.
- **Worker whiteboard omission:** the continuation refused whole-operation
  admission at 05:52:06Z. The worker produced no fresh whiteboard package exit;
  its controller's exit 1 was not a test result. MAIN subsequently reported a
  fresh whole-package merge-side run with only the 6 pre-existing reds.
- **Environment/evidence:** all execution was offline (`GOPROXY=off`,
  `GOSUMDB=off`, `GOTOOLCHAIN=local`). The first launch stopped before its Node
  suite because the private recorder still guarded the old pin pair. Only those
  two private constants were rebound for the fresh run; original failed evidence
  and product/tests were not repaired. Raw records remain private under cache
  roots `3e3bef94`, `d3e55baa` and `615b494e`, not committed log bundles. Complete
  stdout/stderr was read. Every executed controller has complete owned cleanup,
  I/O, external exit and post-scan evidence; retained-identity matches/unknowns
  are empty, not an exhaustive global-process absence claim.
- **Omitted:** whiteboard execution after admission refusal; new browsers,
  screenshots/visual comparison, Lite, resync, runtime actions, phase closure and
  ticket closure. Old Lite proves only its old head; original browser failures
  remain in `261115-bug-ws-dashboard-remaining-browser-harness-gaps`, unchanged.
  MAIN retains merging, UI-start pause, baseline follow-ups and worktree release.



## Resolution (2026-11-15)

MAIN merged 81382b093; fresh wswhiteboard run by merge side, 6 pre-existing reds only. MAIN authorized this docs-only closure at 2026-11-15T06:01:23Z. Merge-side touched dashboard tests retained only the independently baseline CSS collision failure; Node split checks and wsi18n passed. Original browser omissions remain recorded in 261115-bug-ws-dashboard-remaining-browser-harness-gaps; this closure does not claim full browser acceptance.
