---
title: "Clarify the supported completion path for a conflicted ws/git.merge"
---

# Clarify the supported completion path for a conflicted ws/git.merge

## Background

During a lead-owned merge of exact `a21b115ce` into the TASK3 implementation
branch, the lead delegated four conflict resolutions. After resolution,
`ws/git.commit(paths: <four resolved files>)` refused an already-staged incoming
path: `refusing to commit unrelated staged path "agents-plugin-tool/cmd/ws-mcp/dashboard.go"`.
That path belonged to the approved merge index, not an unrelated worker edit.
The existing merge was completed natively with exact HEAD/MERGE_HEAD guards
and AI Context as `ab2b73cd5`; no second merge was started.

## Phases

### Phase 1: Make safe merge completion discoverable

Provide or document a supported completion path for an approved in-progress
merge while preserving unrelated-staging and branch-identity protections.
The refusal should explain how to finish that merge, rather than requiring
the worker to infer a native fallback. No implementation or API choice is
settled by this capture.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 같은 문제(충돌 병합 마무리 불가)의 중복 티켓; 261115-bug-ws-git-commit-cannot-conclude-merge.
