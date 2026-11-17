---
title: "Antigravity hosted adapter: fill catalog values per harness@version range and implement the backend (stage 3, after Codex)"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-feat-ws-hosted-antigravity-acp-adapter: "dropped prior work (immutable in .dropped); this ticket revives its scope under the hosted-first plan"
  261117-feat-ws-harness-capability-catalog: prerequisite
  261115-feat-ws-hosted-codex-adapter-contract: "stage 3 ordering: Codex first, then Antigravity (25)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
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

1. **Stage-3 child, ordered after Codex (25).** Implementation and verification
   order is Claude, Codex, Antigravity, then Pi.
2. **Catalog-based (07).** The work is: fill the capability catalog values per
   `harness@version` range (`since` / `until`), and implement the backend. The
   adapter is range-versioned per the catalog version policy (20)=a, (21)=a, (22):
   startup version and init-shape check, nearest-profile "unverified version"
   warning when out of range, per-version recorded stream fixtures on remote
   w5090.
3. **Admission conditions apply.** Headless JSON event streaming (the adapter
   converts the format) and HTTP MCP server injection (24). Whether the Antigravity
   path satisfies them is determined in Phase 1.
4. **Delegation is `ws/agent.*` (06).** Whether Antigravity's native subagents can
   be removed is decided per harness at stage 3 (08)=c, in Phase 1.
5. **Account and access are rechecked when stage 3 starts.** The earlier blocker
   (no account) is not assumed to still hold or to be resolved; no install, login
   or provider call before it is rechecked and authorized.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; interactive paths never wait on file I/O.
- Keep machine-local credentials and account details out of tracked tickets and
  manuals.
- Do not edit the dropped prior ticket; link it by stem only.

## Compatibility

Additive adapter and catalog data: target T0. Any actual new surface (descriptor,
config, protocol) is classified and recorded before promotion, per
`ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Recheck access, settle the version-ranged contract and fixtures

When stage 3 reaches Antigravity: recheck account/access; select the target
version range; assess the documented public ACP path against the catalog's
admission conditions and items; record `native` / `ws-substitute` / `unsupported`
per item with `since` / `until`; decide native subagent removability (08)=c; plan
the per-version fixtures. Return product or API choices to the lead.

Verification: a recorded matrix where every claimed capability carries a version
and a source; a fixture plan distinguishing planned fixtures from ones run.

### Phase 2: Implement the backend under the settled contract

Implement the chosen control path through the Runner/backend/holder boundary with
the catalog values, startup check and loud nearest-profile policy.

Verification: recorded-stream fixture tests on remote w5090, regression of the
Claude and Codex backends, and a separately authorized account-backed smoke.
