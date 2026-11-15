---
title: Diagnose undefined mode in the rail draft-restore test
related:
  261110-feat-ws-dashboard-session-rail: existing rail and per-session draft restore
---

# Diagnose undefined mode in the rail draft-restore test

## Background

MAIN reported on 2026-11-15 at 01:20:29 UTC that `go test ./internal/wsdashboard/ -run TestRailDraftRestore` fails on develop with the node-harness JavaScript error `ReferenceError: mode is not defined` at `rail.js:807`. MAIN verified the identical failure on the base before `2c5d918d4`; this is a pre-existing failure, not evidence that the pooled-worktree rail hotfix introduced it. These are MAIN-reported observations, not a reproduction by this capture session.

## Constraints

- Capture only now, at lower priority than TASK5 and asset-split 793. No diagnosis, fix, test execution or runtime action is authorized by this capture.
- Determine whether the production `rail.js` code or its node harness owns the missing `mode` binding before proposing a correction. Root cause and the correct fix remain unverified.
- Preserve draft-restore coverage; do not treat removing assertions or bypassing the affected path as a diagnosis.
- Before any later source edit, read the applicable implementation conventions: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Locate the missing mode binding

When separately scheduled, reproduce the focused failure and trace `mode` through the rail implementation and node harness. Record the exact revision, command, complete output and actual exit code; identify which side violates its expected execution context. Report the evidence and proposed correction before implementation. This idea ticket does not settle a production-versus-harness fix or authorize one.

## Resolution
Triage 2026-11-15 (owner (1057)): DONE - 테스트 fixture 에 mode 전역이 없던 문제를 9474c5ec9 에서 고침; 9474c5ec9.
