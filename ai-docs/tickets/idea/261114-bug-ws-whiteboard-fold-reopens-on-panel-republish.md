---
title: Collapsed whiteboard message panels reopen after a panel is republished
related:
  261114-bug-ws-dashboard-panel-frames-reload-on-topic-update: Distinguish disclosure state from unchanged iframe reload/graft behavior
  261114-bug-ws-dashboard-history-paint-resizeobserver-loop: Neighboring geometry/observer investigation; common cause unproven
---

# Collapsed whiteboard message panels reopen after a panel is republished

## Background

On 2026-11-14 MAIN relayed the owner's report that previously collapsed
whiteboard message panels keep reopening while the board receives frequent
log/panel updates (topic `panel-reopen`). MAIN requested a deployed-source
scenario analysis and coordination with EXEC2's TASK3 fold/graft rewrite,
not an unreviewed change to folding policy.

The deployed source baseline is `9854ff350fa1b0eccfb5c71649c5190c405925b9`.
The 649/652/670 candidates and TASK3 are not treated as deployed. A source-only
Pi worker identified the scenario below. The owner incident has not been
reproduced in a browser or attributed to that scenario.

## Verified source findings

All citations refer to that deployed pin under `agents-plugin-tool/internal/`.

- `wsdashboard/assets/dashboard.js:144-200` remembers folds in page-local
  `openState` and browser localStorage `ws.dashboard.panelshut`; persistent
  keys combine session and entry identity, with a 200-fold cap. In-memory
  state takes precedence. Storage failures are tolerated (`:27-32`).
- `wswhiteboard/entries.go:62-67,90-138` preserves panel `pN` when overwriting
  it, but appends a new immutable entry `eN` for each accepted overwrite.
- `wswhiteboard/assets/page.tmpl:63` keys panel disclosures as `entry:eN`,
  with `:history` added for historical versions, not by stable panel ID.
- `wswhiteboard/stream_view.go:219-264` defaults the newest three distinct
  latest panel versions to open in an open topic.

Thus: collapse a panel's current entry, republish that same panel, and the
new entry has a different fold key and normally opens under the latest-panel
default. Deployed comments explicitly describe new-version folding semantics;
this is a source-proven scenario, not proof of the owner's exact trigger.

Unrelated log-only updates should preserve a same-entry fold: board morphing
is followed by history/disclosure restoration
(`wsdashboard/assets/dashboard.js:599-612`), and grafting retargets that
restoration to the retained live section (`:900-910`). Restoration is not
simply omitted for grafted disclosures. Board transport is WebSocket; full
and incremental board updates converge on `swap()` (`:6239-6257,6279-6318`).

## Decisions

- Capture and investigate the owner-visible reopening scenario separately
  from the existing unchanged-frame reload issue. Share evidence with EXEC2
  before TASK3 verification to avoid duplicate or contradictory fixes.
- Owner decision (754)=a, relayed by MAIN on 2026-11-14, changes the policy:
  a user fold follows stable panel identity across republishing, and the
  updated panel receives a conspicuous `updated` badge instead of reopening.
  MAIN assigned implementation to EXEC2's TASK3 rewrite or its immediate
  follow-up. This ticket supplies baseline evidence, not a duplicate EXEC3
  implementation. The exact badge presentation and storage mechanics remain
  with that implementation's design settlement.
- Compatibility tier for a future fix is not settled. The capture changes
  no runtime/storage/API surface; inspect fold-key persistence and mixed
  readers before claiming a tier for a proposed change.

## Constraints

- Preserve the distinction between unchanged entry identity, republished
  panel identity, historical-entry disclosures and iframe document identity.
  The neighboring frame-reload ticket deliberately reloads a panel on its
  own overwrite; preserving fold state must not silently freeze old content.
- A common cause with vertical jitter is unproven. Grafting copies incoming
  `open` attributes before fold restoration (`dashboard.js:1081-1105,608`),
  which may trigger toggle/measurement work, but visibility across paint and
  causal oscillation have not been measured.
- The owner video was inaccessible under macOS Desktop permissions; no
  permission bypass, extracted frames or visual period is claimed. Await
  MAIN's confirmation of an owner-readable external copy.
- No product edits, browser/daemon runs, storage migration or live owner
  interaction is authorized by this capture. Browser reproduction requires
  MAIN's stated-plan/slot coordination.
- Before product edits, read `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Hand off the baseline scenario to the policy-owning rewrite

Trace a bounded sequence of user collapse, same-topic unrelated log append,
other-topic update, and same-panel republish against the deployed baseline.
Keep observed incident evidence distinct from the source-proven new-entry
scenario. Coordinate exact fold-key/graft behavior with EXEC2's TASK3 branch
before choosing the implementation owner or changing semantics.

Report whether the actual incident is same-entry state loss or the baseline
new-entry auto-opening scenario without treating the policy change as proof
of causality. Apply (754)=a through EXEC2's TASK3 or immediate follow-up;
return any still-unsettled persistence or badge-design choice through MAIN.
That work's verification must separately cover retained user folds and an
updated badge across panel overwrite, unchanged-entry folds, history
disclosures, and fresh content with retained document identity under unrelated
updates. No implementation Result or separate EXEC3 fix is claimed.
