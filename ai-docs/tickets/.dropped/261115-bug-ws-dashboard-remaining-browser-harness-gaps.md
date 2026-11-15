---
title: Resolve remaining dashboard browser-harness gaps without weakening predicates
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: scoped source verification completed with these browser gaps deferred
  261115-bug-ws-shared-card-browser-harness-bounded-readiness: related fixture/readiness investigation
---

# Resolve remaining dashboard browser-harness gaps without weakening predicates

## Background

The asset-split verification compared immutable BEFORE
`aac39c69f381a82677f0ee6857e8f8fc5bc89322` with AFTER
`2c81f39c60285040b8d822d9a5cff6b29739ccfa`. Complete approved Node commands
and both full dashboard/whiteboard Go packages passed offline. Both capture
jobs completed, but this is **not full browser or visual acceptance**.

MAIN's 2026-11-15 dispositions authorized retaining known-failing browser
commands and ending discovery at the third new browser cell, rather than
repeatedly chasing unrelated harness failures. This is the single combined
follow-up for the remaining gaps; original failed runs have not been regraded.

## Observed cells

1. **Composer outside-click restoration (new cell 1).** Both frozen pins fail
   `composer outside click restores scroll within 1px`: absolute scroll changes
   from 11821 to 8572. Both retain reading/target `T13`, target offset -36px,
   width 394px and unchanged card storage. The associated click-swallow
   diagnostic is false on both pins; its subsequent original assertion was
   not reached, so this diagnostic is not counted as another executed cell.
   Classify the actual geometry/event contract before changing an oracle.
2. **Wide overview pin placement (new cell 2).** The baseline fails
   `wide overview empties column and places pins directly under toolbar without label`
   in `overview-motion --case pins` at 1900px. The original child exit is 1.
   Candidate pairing was not performed after the discovery cap was reached.
3. **Shared-action fixture precondition (new cell 3).** The baseline fails
   `fixture uses renderer-owned ticket, question, history and panel regions`
   in `shared-actions --case persistence` at 420px. The original child exit is
   1. This does not establish the downstream action assertions or their cause.

Find these predicates in `scripts/board-headless/toolbar-overview.mjs` and
`scripts/board-headless/shared-actions.mjs`; older unsplit revisions keep them
in `scripts/board-headless.mjs`.

## Exact coverage gaps

- `overview --case anchors` is a paired known-failing command, not a pass.
  The corrected middle restoration, bottom restoration and top/no-anchor
  checks have retained successful readouts, but the composer failure and the
  command's remaining shell-header/unvisited checks remain unresolved.
- In the final scoped full allocation, baseline keep, history-fit, panel-header
  and toolbar commands exited zero; overview-motion and shared-actions failed.
- All seven candidate browser commands were omitted once the third distinct
  new cell appeared. Prior short-run toolbar evidence is not a replacement
  for that missing full-run coverage.
- `shared-controls` remains omitted without its separately approved coherent
  exited-terminal fixture.
- Paired captures were retained but not compared side by side; capture-job
  completion is not visual acceptance.

Private, unshipped evidence is retained in review-cache roots
`d35aa21c-793-composer-ab-artifacts`,
`084e2d29-793-composer-ab-candidate-artifacts`, and
`51b53804-793-full-scoped-artifacts`. The candidate-only completion used the
original A/B allocation epoch, without repeating the baseline or resetting its
1320-second ceiling. Raw programs, logs, screenshots, process tables and
receipt packets remain outside Git.

## Constraints

- Diagnose implementation, fixture and oracle responsibility before changing
  tests. Expected behavior must come from an independent baseline/contract,
  never the candidate's own observed landing.
- Preserve original failures, 1px tolerances, identity/offset/width/storage,
  exposure and easing assertions. A skipped or known-failing cell is not PASS.
- Keep product fixes separate from fixture/oracle corrections; neither the
  asset split nor newer develop behavior is established as this issue's cause.
- This idea ticket records gaps, not authority for another runtime attempt,
  shared-daemon action, source re-pin or broader product change.

## Phases

### Phase 1: Establish and repair the remaining browser contracts

Populate bounded implementation facts and independent expected behavior for
these cells. Supply coherent required fixtures, distinguish layout-preserving
scroll changes from genuine restoration defects, and correct only the
responsible layer. Then allocate focused verification and the omitted
candidate/visual coverage explicitly. Retain failed evidence and report any
remaining omissions without a manufactured green verdict.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 브라우저 테스트 하네스의 여러 잔여 실패를 묶은 막연한 목록. 구체적 다음 단계 없음.
