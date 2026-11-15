---
title: Selection-note browser acceptance reports six chips where five are expected
related:
  261114-research-selection-cross-entry-capsule-count: "existing historical evidence and unresolved native keyboard-range question"
  261114-feat-ws-dashboard-selection-mention-note: "selection-note behavior and preserved browser acceptance gap"
  261114-feat-ws-selection-bundle-comment: "overlapping composer/history surface; no cause or prerequisite inferred"
---

# Selection-note browser chip-count failure

## Background

MAIN requested this separate idea bug capture on 2026-11-14 at 15:49:16 UTC after reporting that `TestSelectionNoteBrowser` fails on develop baseline `106eaa861`: `selection-note.browser.cjs:118` observes six chips where the assertion expects five. The report described a repeated-click assertion and supplied this command, relative to `agents-plugin-tool/`:

```sh
go test ./internal/wswhiteboard -run TestSelectionNoteBrowser -count=1 -timeout=180s
```

This is MAIN-reported execution evidence, not a reproduction by this capture. MAIN did not supply the original output artifact or an exact first-bad commit. Root cause remains unknown; proximity to the selection-bundle surface is not proof of a regression introduced by that work.

## Evidence and discrepancies

Static Git inspection at the reported baseline `106eaa861` establishes:

- `agents-plugin-tool/internal/wsdashboard/selection_note_browser_test.go:19` defines `TestSelectionNoteBrowser`. The supplied command names `internal/wswhiteboard`, so its package and the observed browser assertion do not yet reconcile. Preserve the reported command verbatim rather than claiming it executed the identified test. The corresponding source-located invocation would name `./internal/wsdashboard`; it was not run during capture.
- `agents-plugin-tool/internal/wsdashboard/testdata/selection-note.browser.cjs:109-118` establishes a keyboard selection, checks an empty note, clicks attach and expects five chips. The repeated-action duplicate-prevention check is at lines 120-122, after line 118. A failure at line 118 therefore does not itself prove that the repeated action duplicated an attachment.
- The adjacent line 119 also expects that an empty note adds no ordinary composer text. Keep source provenance, note text and chip-count evidence distinct.

Existing research `261114-research-selection-cross-entry-capsule-count` already records the same expected-five/actual-six failure at line 118 in historical commit `9538354ca`. Its Outcome Ledger preserves a selected range containing `Second immutable quote` and the following `Plan` heading, but explicitly leaves the relationship between the intended and actual native keyboard range unresolved. That is historical context, not a diagnosis of the new reported baseline.

The research also records owner `670,675=a`: merge-first acceptance did not authorize another browser diagnosis or changing the preserved assertion. This new idea capture does not expand that authority.

## Decisions

- Capture the reported baseline failure as an idea bug, linked to existing evidence, without executing another browser/test run or implementing a fix.
- Preserve the five-chip assertion and distinguish initial keyboard/empty-note attachment from the subsequent repeated-action check. Do not change the expectation to six merely to obtain a pass.
- Do not infer a first-bad commit, duplicate-attachment cause, selection-bundle dependency, or acceptance regression from the report alone.

## Constraints

- Current authority is documentation only. Diagnosis, assertion changes and implementation require later authorization; no heavy slot is implied.
- Keep the existing historical failure and interrupted acceptance evidence intact. Link follow-up evidence to the original baseline and actual selected source range instead of overwriting it.
- Any future tooling changes read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` first. Read the authoring/mirroring manuals too if shipped guidance changes.
- MAIN owns daemon rebuild/restart/swap, swap smoke and profiling. This capture permits none of them.

## Phases

### Phase 1: Reconcile the reported failure and correct its cause

After explicit diagnostic/implementation authorization, reconcile the supplied command's package with the test definition and recover the exact baseline/output. Establish whether the native keyboard gesture selects one or multiple immutable source entries, when the count first diverges, and whether the later repeated action changes it. Reuse the existing research ledger; do not assume either a duplicate attachment or an incorrect expectation before the evidence distinguishes them. Preserve the source-range and note/provenance contract when correcting the proven cause. Return a requested change to that contract for decision rather than silently relaxing acceptance.

Verification: record candidate and baseline SHAs, the exact package/test/browser command, dependency and browser availability, raw output, native selected range, source entry identities, and chip counts before and after each action. Exercise the preserved five-chip assertion, repeated-action idempotence, empty-note composer-text behavior, and existing narrow/wide cases after a separately granted slot. Keep executed acceptance distinct from static inspection; do not claim a fix, first-bad commit or runtime success from this idea capture.
