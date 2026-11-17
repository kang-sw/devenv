---
title: "Antigravity hosted adapter: fill catalog values per harness@version range and implement the backend (stage 3, after Codex)"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-feat-ws-hosted-antigravity-acp-adapter: "dropped prior work (immutable in .dropped); this ticket revives its scope under the hosted-first plan"
  261117-feat-ws-harness-capability-catalog: prerequisite
  261115-feat-ws-hosted-codex-adapter-contract: "stage 3 ordering: Codex first, then Antigravity (25)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5adf930d038dd542
sage-review-completeness-reviewed: 5adf930d038dd542
---

# Antigravity hosted adapter

## Background

The earlier ticket `261114-feat-ws-hosted-antigravity-acp-adapter` was dropped on
2026-11-15 (owner (1057)) as far-future work because no Antigravity account was
available; it lives in `.dropped/` and is immutable, so this is a new stem. What
it planned, in short:

- assess the official ACP path (`agy acp`, ACP v1 JSON-RPC over stdio, with
  headless NDJSON, the Python SDK and the IDE extension as distinct alternatives)
  against the common hosted contract;
- settle an exact-version capability/protocol/fixture matrix (session
  create/load/resume, send/cancel/steer, permission pause/answer correlation,
  ordered events, holder reattach, auth/config home, subagent telemetry);
- then implement the chosen public control path through the existing
  Runner/backend/holder boundary.

Antigravity is revived by (02) (correction: Codex and Antigravity will both be
tier-1 hosted harnesses after hosted common-ization) and (05) (bring Antigravity
back as a stage-3 child of the epic). The ticket's official-documentation links
and capability checklist remain the starting evidence there; they are not
re-verified here.

## Decisions

1. **Stage-3 child, ordered after Codex (25), ahead of the holder ((70)=b).**
   Implementation and verification order is Claude, Codex, Antigravity, then
   Pi. Owner (70)=b, 2026-11-17 22:10, moved Codex and Antigravity ahead of
   `261117-feat-ws-single-session-holder`. Phase 1 starts after the Codex
   ticket's Phase 1 lands; Phase 2 also depends on the catalog's Phases 1-3.
2. **Catalog-based (07).** The work is: fill the capability catalog values per
   `harness@version` range (`since` / `until`), and implement the backend. The
   adapter is range-versioned per the catalog version policy (20)=a, (21)=a, (22):
   startup version and init-shape check, nearest-profile "unverified version"
   warning when out of range, per-version recorded stream fixtures.
3. **Admission conditions apply.** Headless JSON event streaming (the adapter
   converts the format) and HTTP MCP server injection (24). Whether the Antigravity
   path satisfies them is determined in Phase 1.
4. **Delegation is `ws/agent.*` (06).** Hosted sessions delegate through
   `ws/agent.*`. Owner (08)=c: whether a harness's native subagents can be
   removed is decided per harness at stage 3, so Phase 1 probes and records it
   for Antigravity.
5. **Account and access confirmed (owner 2026-11-17 22:10).** The owner logged
   in on the home machine (installed `agy` 1.5.9) and approved provider calls
   for probes, fixtures and smoke tests. Do not log in, log out, switch
   accounts or mutate credentials; keep account details out of tracked files.
   Rejected: keeping the earlier "recheck at stage-3 start" gate, which this
   answer settles.

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; interactive paths never wait on file I/O.
- Keep machine-local credentials and account details out of tracked tickets and
  manuals. Do not mutate the user's agy config home; run probes with an isolated
  or read-only configuration where the CLI allows.
- No main push, version bump or release. Work lands on `track/agent-v2` through
  the lead. Replacing the shared daemon on this machine is the lead's call, not
  a worker's.
- Do not edit the dropped prior ticket; link it by stem only.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Compatibility

Additive adapter and catalog data: target T0. Phase 1 records the concrete tier
verdict for every new surface (descriptor, config, protocol) in its `### Result`,
with source lines, per `ai-docs/manuals/release-compatibility.md`, including an
old daemon reopening a hosted Record whose new Antigravity `Backend` value it
does not know.

## Prior Decisions

- 261114-feat-ws-hosted-antigravity-acp-adapter (2026-11-14, Decisions): "A future adapter must coexist with Pi/Claude without reducing their supported controls or native integrations. MCP attachment alone does not establish sustained agent-process control." — bearing: constrains
- 261114-feat-ws-hosted-antigravity-acp-adapter (2026-11-14, Decisions): "The future design must prefer an additive T0 adapter. Any actual protocol/store/config/MCP change is classified and settled before promotion; T2 is not authorized by this deferred idea." — bearing: constrains
- 449e2602 (2026-11-15, commit): "Keep exact-version support unknown and distinguish ACP from headless NDJSON, SDK and IDE-private transport. Record required session/control/approval/event/identity/holder/config/telemetry evidence" — bearing: constrains
- 261117-feat-ws-exec-tools (2026-11-17, commit 1d04941a): "Owner 19:02: harness+tier strings (pi.small, codex.large) and user aliases (mockup -> agy.large) instead of model/effort args; rejected: spawn_ex(model, effort, harness) and an override object." — bearing: constrains
- 261114-epic-ws-common-session-foundation (2026-11-17, commit 6615ab3f): "(29)=ok extends native subagent removal to hosted workers; owner added a per-harness 'Never use {{NativeToolScheme}}' playbook line because Codex/Antigravity removability is still unprobed ((08)=c)." — bearing: supports
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(02) Codex and Antigravity will both be supported as tier-1 hosted harnesses after hosted common-ization; Antigravity is not dropped (new ticket `261117-feat-ws-hosted-antigravity-adapter`" — bearing: supports
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-15, Decisions): "Stage ordering (25). Claude is the reference, then Codex (this ticket), then Antigravity, then Pi. The Pi hosted backend is a port of this Codex adapter and catalog profile" — bearing: supports
- 261114-refactor-ws-hosted-common-contract-pi-claude-parity (2026-11-14, Decisions): "No Antigravity install, authentication, provider call or live probe" — scoped to that first unit and lifted for this ticket by Decision 5 (owner 2026-11-17 22:10) — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 2 touches the Runner/backend/holder boundary (agents-plugin-tool/internal/wsagent/backend.go, registration.go, holder.go), catalog values and fixtures; no agy/antigravity file exists in wsagent yet (ls grep found none); exact files are settled in Phase 1 |
| scope.surface | cross-module | wsagent backend plus holder, catalog profile and startup version check, possibly MCP injection under agents-plugin-tool/internal/mcp/ |
| scope.new_public_symbol | unknown | the backend registration and catalog profile shape are settled in Phase 1 and the catalog schema has not landed (pending 261117-feat-ws-harness-capability-catalog) |
| scope.new_type_contract | unknown | the Backend interface exists at agents-plugin-tool/internal/wsagent/backend.go#L24 but whether agy needs a new contract depends on the Phase 1 matrix |
| scope.test_surface | new-files | Phase 2 verification names recorded-stream fixture tests and an account-backed smoke; wsagent has existing Claude and fake backend tests but no agy test |
| complexity.reuse_points | unconfirmed | Backend interface and BackendRegistration exist (wsagent/backend.go#L24, registration.go#L14) but the catalog schema they plug into has not landed (pending 261117-feat-ws-harness-capability-catalog) |
| complexity.side_effect_risk | moderate | additive backend behind registration, but it spawns a provider-calling subprocess under the owner's login and runs inside the shared holder |
| risk.correctness | high | agy 1.5.9 protocol, session resume, permission correlation and event ordering are unprobed; the ticket has no recorded matrix yet |
| risk.fit | high | fit to the catalog and Codex-first contract is unsettled; both predecessors have unlanded phases (pending 261115-feat-ws-hosted-codex-adapter-contract) |
| risk.test | high | needs recorded fixtures per version plus provider-backed smoke on a single owner account that automated tests cannot reproduce |
| risk.security_or_contract | high | credential home and account state, permission mapping, HTTP MCP token injection and a version-ranged catalog contract are all in play |

## Phases

### Phase 1: Settle the version-ranged contract and fixtures

With access confirmed (Decision 5): select the target version range from the
installed `agy`; assess the documented public ACP path against the catalog's
admission conditions and items; record `native` / `ws-substitute` / `unsupported`
per item with `since` / `until`; decide native subagent removability (08)=c; plan
the per-version fixtures. Return product or API choices to the lead, and also
which roles (hosted lead, `agent.spawn` worker, or both) the first Phase 2 slice
should deliver, with the evidence for each. If the chosen path does not fit the
existing `Backend` / `Process` shape, describe the needed extension and return
it to the lead; do not extend the common interface in this phase.

Deliverables: the matrix, admission evidence, removability verdict, role
recommendation and compatibility verdict go in this phase's `### Result`;
recorded fixtures go under `agents-plugin-tool/internal/wsagent/testdata/`.

Verification: a recorded matrix where every claimed capability carries a version
and a source; a fixture plan distinguishing planned fixtures from ones run.

### Phase 2: Implement the backend under the settled contract

Before this phase is dispatched, the lead settles the role scope from Phase 1's
recommendation and records it here as an `#### Edition`; a worker does not pick
the roles. A role not in scope must not be advertised: if the worker role is
out, `agy.<tier>` is not spawnable (batch-review coherence C2).

Implement the chosen control path through the Runner/backend/holder boundary with
the catalog values, startup check and loud nearest-profile policy.

Verification: recorded-stream fixture tests, regression of the
Claude and Codex backends, and an account-backed smoke on the owner's login
(authorized, Decision 5). The smoke passes when every capability Phase 1 graded
`native` or `ws-substitute` for the settled roles works end to end on the
installed version, and every `unsupported` one is refused cleanly.
