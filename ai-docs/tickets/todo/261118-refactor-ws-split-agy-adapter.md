---
title: "Split the agy adapter file below the size guard"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "its Phase 2 merge (f50bc2625) pushed agy.go past 1000 lines; its Phase 3 also edits agy.go, so sequence the split around it"
---

# Split the agy adapter file below the size guard

## Background

Merging the agy worker role (`f50bc2625`) pushed
`agents-plugin-tool/internal/wsagent/agy.go` to 1124 lines, past the
1000-line soft size guard of `git.merge`. Owner (163)=a, 2026-11-18: capture
the split as a todo ticket and do it later; do not raise the guard in
`.gitattributes` (consistent with the earlier (94)=a).

## Decisions

- (163)=a, 2026-11-18: split later through this ticket; no
  `ws-size-guard` override for `agy.go`. Rejected: splitting before Phase 3
  of `261118-feat-ws-agy-tool-names-and-worker-role` on its own schedule
  (both touch the same file), and raising the guard to 1400 lines.

## Constraints

- Behavior-preserving: no change to the ACP wire handling, the lead or
  worker start paths, or any caller-visible output; existing agy tests pass
  unchanged.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md`.

## Phases

### Phase 1: Split agy.go by responsibility

- Split `agy.go` into files by responsibility so each stays under the size
  guard; the file boundaries are settled at promotion to `ready/`.
- Sequence with Phase 3 of `261118-feat-ws-agy-tool-names-and-worker-role`
  so the two do not edit the file concurrently.
- Verification: `go build ./...`, `go vet` and `go test ./internal/wsagent`
  pass with no test edits beyond moved helpers.
