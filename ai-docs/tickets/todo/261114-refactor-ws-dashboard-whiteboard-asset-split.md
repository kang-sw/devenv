---
title: "Mechanically split dashboard and whiteboard assets at the UI cut line"
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: "Landed 652 source and oversized-file advisories; preserve its visual behavior"
---

# Mechanically split dashboard and whiteboard assets at the UI cut line

## Background

Owner decision (793)=a authorizes a behavior-preserving split of oversized dashboard and whiteboard assets. Large files make concurrent UI changes and mechanical rebases expensive. This ticket captures accepted backlog only; MAIN supplies the start signal and assigns the implementer at the cut line.

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
- **Cut line, not a far-branch barrier.** Start immediately after both 649 and MAIN's test batch have merged: 758, 773 plus sticky work, close-copy, usage meter and UI polish. MAIN confirms that frontier and explicitly starts this work. Do not wait for TASK3, TASK5, 723, 675 or other distant branches; those continue and mechanically rebase onto the new layout afterward.
- **Pause new UI starts during the split.** Worktree view, file explorer, 781 and selection-bundle UI do not newly start while the split is active. This is a start pause, not cancellation of their tickets or a request to stop already-running distant branches.
- **Publish a move map.** Record each old file and recognizable region/symbol group against its new file, plus the changed embedding/template sites and load order. Ongoing workers use this map to rebase without re-deriving the extraction.
- **Compatibility: T0 (asset content).** This work changes internal asset organization, not MCP tools/arguments, protocol behavior, tracker events, store shape or configuration fields. Existing caller-visible behavior remains unchanged.
- **Acceptance includes tests and before/after screenshots.** Capture the actual cut-line baseline, not an older visual reference, and compare the result side by side at matching routes, viewports, state and motion settings. Neither a file move nor syntax checks alone prove behavior preservation.

## Constraints

- This ticket remains `todo/` until the normal implementation-readiness procedure and MAIN's cut-line start signal. Authoring it does not allocate a heavy slot or authorize implementation, release, a daemon rebuild/restart/swap, profiling or an installed-owner test.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before editing `agents-plugin-tool/internal/`; the split must preserve their downstream and compatibility contracts. If a planned edit reaches a skill, playbook, prompt or convention, read `ai-docs/manuals/skill-authoring.md` and stop before expanding this asset-only scope.
- Update the actual `//go:embed` declarations and rendering/template registration sites: dashboard assets are registered in `agents-plugin-tool/internal/wsdashboard/pages.go`; whiteboard assets in `agents-plugin-tool/internal/wswhiteboard/render.go`. Preserve their emitted asset order and the current whiteboard-before-dashboard CSS order.
- Keep existing JavaScript state ownership, closures, event-handler registration, delegated events, frame identities and browser storage semantics. Extraction must not create duplicate handlers, implicit new globals, asynchronous loading races or a new shared API.
- Preserve CSS selector specificity, relative cascade order, URLs and layer/stacking semantics. Split boundaries must not change geometry, typography, animation, focus visibility or hit ownership.
- Tests currently reading a single embedded file must continue to verify the assembled behavior after extraction; do not remove assertions because their old source-file locations moved.
- Preserve all shared-controls/shared-actions and 652 compact-toolbar/overview harness scenarios and command syntax. Known acceptance limitations in the cut-line source stay explicit; they are not silently fixed, erased or converted to passes by this refactor.
- Choose final extraction filenames from the existing responsibilities at the cut line and record them in the move map; this ticket does not introduce a new exported module or caller-visible interface.

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
