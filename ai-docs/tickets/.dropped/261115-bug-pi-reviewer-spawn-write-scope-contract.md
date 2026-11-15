---
title: "Expose the Pi reviewer spawn write-scope requirement before dispatch"
---

# Expose the Pi reviewer spawn write-scope requirement before dispatch

## Background

During 777 integrated-harness review dogfooding on 2026-11-15, a root-rendered `reviewer` prompt was passed to `ws-agent-spawn` with a model, alias, working directory and review task. The tool schema described `write_scopes` as optional. Dispatch refused before allocation with `ws-pi-agent: code reviewer requires exactly one file write scope`.

The root then allocated one report path through `ws/path.generate` and retried with exactly that file grant; dispatch succeeded with one bound scope. The report-only restriction is useful. The surprise is that callers discover its mandatory shape only after a refused call, although the advertised generic spawn contract makes the field optional and the review procedure tells callers that reviewers allocate report artifacts themselves.

## Evidence

- Refused dispatch: `ws-pi-agent: code reviewer requires exactly one file write scope`.
- Successful retry: reviewer alias `effects777-integrated-review`, agent `cb6b0287-525d-499f-a6f4-9b6488707d19`, response `write_scopes: {status: bound, count: 1}`.
- The first request allocated no agent; the retry used the same rendered prompt and one lead-allocated report path. No permission guard was bypassed.
- This is observed tool-contract friction, not evidence of a review defect or failed browser acceptance.

## Constraints

- Preserve the report-only reviewer write boundary. Do not relax it merely to make the original request succeed.
- Ticket capture authorizes no implementation, schema change, or change to reviewer permissions.
- Before implementation, inspect the actual adapter guard, rendered reviewer instructions, spawn schema and lead review procedure. Choose the smallest consistent contract and classify any exposed cross-version change.

## Phases

### Phase 1: Align reviewer dispatch guidance with the enforced scope contract

Make the enforced reviewer report grant discoverable before dispatch and reconcile who allocates the report path. Add focused coverage for missing, multiple and exactly-one report grants, plus the documented lead-to-reviewer handoff. Preserve generic non-reviewer spawn behavior. Settle the concrete surface after investigation rather than assuming that every spawn must require a scope.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 리뷰어 spawn 쓰기 범위 안내 부족. 같은 내용의 다른 티켓과 중복; 261115-bug-pi-reviewer-write-scope-discoverability.
