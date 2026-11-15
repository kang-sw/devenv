---
title: Validate native model evidence before backfill
related:
  261115-feat-ws-dashboard-observed-session-model: partial Phase 1 collection and proof gaps
---

# Validate native model evidence before backfill

## Background

The partial Phase 1 Result in
`261115-feat-ws-dashboard-observed-session-model` records A/B response-evidence
work without claiming complete native-file collection. MAIN explicitly
authorized this research capture at 2026-11-15T05:35:53Z. This idea is
unscheduled; MAIN must schedule investigation. No research or implementation
was performed for this stub.

## Outcome Ledger

### Verified Findings

- Supplied A/B implementation evidence uses CLI Claude completed-step
  `usage.model`, CLI Pi completed assistant events plus existing own-log
  restoration, and hosted Claude/Pi response parsing in Go. A merged as
  `a83c12c84`; MAIN confirmed B `7d3e69331e924f520268d26afaa60be9c8135a06`
  landed as `3e10e3613` at 2026-11-15T05:36:25Z.
- A/B add no Go native-file backfill. Safe Claude native-file/session
  association and cross-source step-ID versus transcript-message-ID
  deduplication remain unvalidated. Codex response identity is unvalidated;
  this is not proof that a supported native Codex identity exists.
- The source ticket's partial Result carries scoped test evidence and its
  limits. Worker-side Claude tests used a local shim, not native-harness
  coverage; runtime/owner activation remains pending. Those checks do not
  establish native-file association or cross-source deduplication.

### Confirmed Decisions

- Validate Claude native-file/session association and step-ID versus
  transcript-message-ID cross-source deduplication before Go native-file
  backfill. Backfill may follow only after that validation.
- Research Codex response model, effort, rate and child association first;
  collect no speculative evidence and infer no child's identity from a parent.
- MAIN owns scheduling. This stub authorizes no investigation, implementation,
  new API or collection mechanism choice.
- Keep any later change T0 optional/additive only, with no version bumps or
  protocol changes. This capture does not revise the source ticket's phase
  plans or close that ticket; CSS/label C remains pending after 793.

### Proposals

None selected or proposed by this documentation-only capture.

### Open Questions

- What validated association ties a Claude native file to the correct session
  and independent child, including recovery/resume?
- How can completed-step IDs and transcript-message IDs identify the same
  evidence without duplicate history or altered observation timestamps?
- Does supported Codex response evidence expose concrete model, recorded
  effort and eligible rate, and how is each child's evidence independently
  associated? Unsupported or absent evidence must remain unknown.

### Rejected Alternatives

- Speculative native-file collection before association/deduplication
  validation: it could misattribute or double-count response evidence.
- Treating request/configuration, a parent's model, or an unvalidated Codex
  identity as observed response proof: these do not establish the answering
  model for the specific session or child.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 모델 증거 백필 검증 연구 메모. 일정도 구체 작업도 없음.
