---
title: "Selection browser acceptance: cross-entry selections may yield one capsule per entry"
related:
  261114-feat-ws-dashboard-selection-mention-note: "preserve original keyboard selection failure for later investigation"
---

# Cross-entry selection capsule counts

## Background

A native keyboard selection may legitimately cross immutable entries and yield one capsule per entry. Capture the distinction between a gesture's intended range and its actual range without changing the preserved acceptance assertion.

## Outcome Ledger

### Verified Findings

- VERIFIED historical evidence: `9538354ca` records the repeated keyboard/empty-note failure at `agents-plugin-tool/internal/wsdashboard/testdata/selection-note.browser.cjs:118`: expected five capsules, actual six. Command: `WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright go test ./internal/wsdashboard -run '^TestSelectionNoteBrowser$' -count=1 -v -timeout=3m` from `agents-plugin-tool`. This capture does not rerun it.
- VERIFIED recorded observation: the source ticket's interrupted-implementation section reports a diagnostic selected range containing `Second immutable quote` and the following `Plan` heading with corresponding source capsules; see `479c74e1b` and `9538354ca`.

### Confirmed Decisions

- Owner `670,675=a`, relayed by MAIN on 2026-11-14 at 11:15, authorizes merge-first acceptance after Go tests and independent lite review, not another browser diagnosis or assertion change; see the immutable plan `ai-docs/.plans/2026-11/14-2044-mention-670-merge-first-acceptance.md`.

### Proposals

- INFERRED, not a diagnosis: per-entry capsule splitting may explain a total above a single-entry expectation. Any future investigation should preserve the actual range and provenance as evidence.

### Open Questions

- Whether the failing native keyboard gesture and its expected range agree remains unresolved.

### Rejected Alternatives

- Retrying diagnosis, changing expected five to six, or relaxing the assertion in the merge-first acceptance unit: outside the confirmed authority.
