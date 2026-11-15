---
title: "Investigate git.merge profile refusal for a lead-owned worktree integration"
---

# Investigate git.merge profile refusal for a lead-owned worktree integration

## Background

During 649 dogfooding, the top-level Pi lead attempted an authorized integration in a worker-bound worktree using its bound session key. The host advertised git.merge, but dispatch refused it before any merge. Determine whether this is an intended capability restriction, a profile mismatch, or adapter drift; do not assume a product bug or change authority semantics from this observation alone.

## Outcome Ledger

### Verified Findings

- On 2026-11-15 local time, ws/git.merge with session_key `passive-venue-thrill`, source `develop`, and target `feat/compact-panel-header` returned: `ws-mcp JSON-RPC error: tool not available in current ws MCP profile: git.merge`.
- The caller was the root lead; the supplied key was bound to `/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`. The worker had explicitly acknowledged source-read quiescence.
- Immediately before dispatch, ws/git.status reported `feat/compact-panel-header 8c715c5 clean`; develop was `7d95271caa04a57f2587ccbe835c34b743725739`.
- The supplied workflow manual describes branch integration as lead-owned through ws/git.merge. The refusal does not itself establish why this bound-key call was unavailable.

### Confirmed Decisions

None. This ticket captures the surprise under the repository's dogfood rule, not a change to workflow semantics.

### Proposals

None selected.

### Open Questions

- Which profile/capability gate rejected this call, and is passing a worker-bound key from a root lead supported?
- What supported path lets the lead integrate in a separately bound worktree while preserving lead authority and checkout safety?
- Do the exposed schema, operational manual, and profile diagnostics accurately describe that path?

### Rejected Alternatives

None.


## Resolution
Triage 2026-11-15 (owner (1057)): DROP - git.merge 가 프로필에서 거부된 한 건의 조사 메모. 의도된 제한인지도 불명.
