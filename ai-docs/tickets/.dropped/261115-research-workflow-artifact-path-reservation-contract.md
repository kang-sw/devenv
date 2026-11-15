---
title: "Clarify generated artifact path reservation contracts"
---

# Clarify generated artifact path reservation contracts

## Background

During the authorized TASK4 framed-setup verification preparation, the worker
attempted `recipe.open('xb')` on a path returned by `ws/path.generate` and
reported `FileExistsError`. It stopped without replacing the file or starting
the verification batch. The lead subsequently read the returned file and found
it empty. Investigate whether the caller-facing allocation contract makes
reservation versus absent-path behavior clear enough to avoid this trap.

## Evidence boundary

Returned artifact:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@ddf3f974/prompt-paths/7bc88858-01-bodytime-677-framed-setup-order-20261114-01.md`.
The worker's complete preparation failure is in its retained session transcript
(`body-time4-worker`, agent `678502af-7639-460a-967b-18b1c3540e8f`).
This is a preparation failure, not evidence of a product or browser-test failure.
No overwrite, replacement allocation, or verification rerun was performed.

## Outcome Ledger

### Verified Findings

- The worker reported an exclusive-create failure on the returned artifact path
  and no verification batch or child process launch from preparation.
- A direct lead read of that exact path returned empty content.
- The caller-facing tool description says it generates writable artifact paths;
  its short contract does not explicitly state whether each returned path is an
  already-created reservation or a filename the caller must create.

### Confirmed Decisions

- None. This research capture authorizes no tool, allocation, or workflow change.

### Proposals

- Non-authoritative: inspect the implementation and tests for each artifact kind,
  then consider stating reservation and safe-population semantics in the result
  or tool contract without changing allocation behavior.

### Open Questions

- Which artifact kinds reserve files, and which return absent paths?
- How should a caller verify ownership and safely populate an empty reservation
  without risking an overwrite of another caller's content?
- Would clearer result text suffice, or is a separately reviewed API change needed?

### Rejected Alternatives

- None settled. Do not infer permission for blind overwrites or automatic retries.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - path.generate 경로가 이미 존재해 xb 열기가 실패한 조사 메모. 구체적 결함 없음.
