---
title: "hosted 자식 연결 신원 검증: 자식의 부모 session key 사용 거부"
parent: 261114-epic-ws-common-session-foundation
blocked-by: 261117-feat-ws-hosted-prompt-assembly
related:
  261117-feat-ws-hosted-prompt-assembly: "prerequisite; its Decisions 10, 11 and 19 apply the hosted session_key rules to hosted lead connections only, and (124)=a moved the child-side refusal here"
---

# Hosted child-connection identity: refuse a child's use of its parent's session key

## Background

Owner note on (109), 2026-11-18: `session_key` may stay as an argument for
compatibility and child-session override, with a permission check so a session
cannot freely use a peer's or parent's key. `261117-feat-ws-hosted-prompt-assembly`
(Decisions 10, 11, 19) implements that check for hosted lead connections. The
parent clause cannot fire there: a child's ws-mcp connection carries no hosted
identity (`agents-plugin-tool/internal/wsagent/runner_process.go#L140-L163`
versus the lead env at `#L309-L317`), and children are not registered, so
`Server.hostedSessionKey()` is empty on every child connection. Owner (124)=a
kept the prompt-assembly ticket on the hosted-lead key gap and moved the
child-side refusal to this ticket.

## Constraints

- Design is not settled. Promotion to `ready/` needs the identity source for
  a child connection (for example an HTTP token mapped to the child's record
  key) and whether the keyless fallback and the `workflow_manual` gate extend
  to children, settled with the owner, plus a compatibility tier
  (AGENTS.md `### Compatibility Tiers`).

## Phases

### Phase 1: Child-connection identity and parent-key refusal

Scope and mechanism are TBD (see `## Constraints`); this phase is a
placeholder until the design is settled.
