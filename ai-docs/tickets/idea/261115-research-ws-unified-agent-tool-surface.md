---
title: "Research unified ws-mcp agent tools versus per-harness tool adaptation"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-refactor-ws-common-session-interface: "preserve distinct entry modes, native integrations, authority and parity boundaries; independent research, not a new prerequisite"
---

# Unified agent tool surface

## Research Intent

MAIN relayed owner (853)=a on 2026-11-15 at 01:16:53 UTC: capture a
comparison of a complete ws-mcp agent toolset (read/grep/find/edit/exec,
with harness-native tools disabled) against per-harness tool-surface adaptation.
The owner needs evidence and decision-ready options, not a selected architecture.

Cover Codex, agy, Claude and Pi, recording exact harness and model versions.
Preserve the owner's name `agy`; relate it to Antigravity only if identity is
evidenced. The expectation that Pi may mostly work already is unverified and
should receive a cheap confirmation check before deeper investigation.

This idea captures future research only. TASK3 remains top-priority independent
active work; neither this capture nor future research adds a foundation
implementation prerequisite or delays TASK3. No research execution, experiment,
source/runtime workload, tool restriction or implementation is authorized here.

## Foundation and Interface Boundaries

- Parent `261114-epic-ws-common-session-foundation`, Scope and Cross-Child
  Decisions: minimum interface -> current behavior/state parity -> extensions;
  distinct CLI-plugin and MCP-only hosted entry, existing Runner/adapter owners,
  preserved identity/authority/lineage and one input/mail delivery owner.
  Common access does not make MCP attachment native integration or process control.
- Related `261114-refactor-ws-common-session-interface`, Decisions 1–5 and
  Constraints: preserve native integrations, approvals, delegation ownership,
  recovery, existing keys/stores and backend capability differences. Profile
  labels and mailbox reachability are not permission proofs.
- That interface's Phase 1 owns minimum shared access and backend
  admission/registration/unsupported-operation contracts; Phase 2 owns current
  context/state parity adoption. Research may inform later choices but neither
  changes those contracts nor asserts their completion. Audit still consumes
  interface Phase 2; its UI still consumes audit-backend Phase 2.
- No new MCP API, serialized field, store/protocol shape, universal runtime,
  approval power or root-key issuance restriction is approved. The existing
  foundation targets additive T0; any later cross-version change needs its own
  compatibility-tier settlement, and T2 needs explicit owner approval.

## Expected Research Deliverables

Acceptance of the future investigation requires:

- An exact-version evidence matrix for each harness/tool surface, separating
  official documented support, source evidence and runtime verification. Give
  citations, commands/configuration, conditions and explicit unverified gaps;
  absence of evidence is not proof of impossibility.
- A reproducible paired experiment design, not fabricated results, with task
  fixtures, grading, controls, repeatability, safety bounds and unresolved
  model/tooling differences. Execution and thresholds remain unapproved.
- A safety/UX and trust-boundary comparison, an existing-profile mapping, and
  enforceability/bypass analysis addressing all five questions below.
- Decision-ready options for the owner: trade-offs, evidence strength,
  limitations and further authorization needed. Findings do not themselves
  select a surface or authorize implementation.

## Outcome Ledger

### Verified Findings

- VERIFIED [parent, Scope / Non-Scope / Cross-Child Decisions]: the foundation
  ticket preserves native behavior and state, distinguishes CLI and hosted
  entry, and does not authorize a replacement universal runtime or new powers.
- VERIFIED [related interface, Decisions / Constraints / Phases 1–2]: the
  interface separates minimum access/admission contracts from parity adoption
  and preserves native capability differences and existing authority. These
  are documentary boundaries, not runtime capability verification.
- No harness tool-substitution, profile-enforcement or policy-enforcement
  capability has been verified by this capture.

### Confirmed Decisions

- Owner (853)=a, as relayed by MAIN: capture this comparison as independent
  research alongside TASK3 without delaying it. This approves the research
  ticket, not a surface choice, experiment execution or implementation.

### Proposals

- Compare native and ws-mcp toolsets on identical small, isolated repository
  tasks per model: inspection/search, a localized edit and a command-assisted
  repair. Specify fixtures and machine-checkable expected diffs/tests, plus
  human grading where semantics cannot be checked automatically.
- Pair runs by exact model/version and harness/version; hold prompts, starting
  tree, context, task budget, permissions and environment constant where
  possible. Reset between runs, counterbalance order, repeat runs with recorded
  sampling settings, and retain failures as well as successes. Report residual
  tool-schema, model availability, prompt, caching and mediation differences
  rather than treating them as controlled.
- Measure edit success (correctness and unintended changes), tool-choice
  errors (invalid, unavailable or inappropriate calls), tokens and latency.
  Specify token-accounting sources, missing telemetry, timing boundaries and
  overhead separately. Propose sample sizes, grading rules and any thresholds
  for owner review; none is settled here.
- Bound any later authorized experiment to disposable fixtures, least
  privilege, no secrets, no external mutation/network by default and explicit
  stop/cleanup criteria. Separate safe negative-policy probes from normal
  accuracy tasks. Obtain execution approval before running either.

### Open Questions

1. **Native-tool substitution, per exact harness/version.** Can native tools
   be disabled or allowlisted and MCP tools substituted for read/grep/find/edit/
   exec? Identify official docs, CLI flags and config for each mode, then
   separately label source evidence and runtime verification. Distinguish
   advertised tool visibility from enforced denial. What remains unsupported
   or unverified? First cheaply check the owner's Pi expectation; it is not a
   finding. Establish agy's identity before borrowing Antigravity evidence.
2. **Paired accuracy experiment.** How should the same tasks compare native
   versus ws-mcp surfaces per model using edit success, tool-choice errors,
   tokens and latency? Refine the proposed controls, repeatability, grading,
   safety bounds and unresolved differences into a reproducible design.
   Which comparisons are invalid or confounded? No particular thresholds or
   execution are approved by this capture.
3. **Safety and UX.** Which native protections and affordances are retained or
   lost, including Codex sandboxing, approval prompts and diff rendering?
   Trace execution location, credentials, filesystem/network authority,
   approval mediation and result presentation. Distinguish MCP server
   authority from harness execution mediation: an attached MCP server does
   not automatically inherit a native execution sandbox. What alternative
   mediation or UX could cover gaps, and what would remain unverified?
4. **Existing profile restrictions.** What do wsagent `Profile` `tools` and
   `ws_tools` actually restrict today for each candidate surface? Ground the
   map in existing definitions, validation, launch wiring, dispatch checks
   and relevant tests, distinguishing intended configuration from actual
   trust/enforcement boundaries. Do not invent profile semantics or treat a
   profile label, hidden tool or advertised allowlist as an authority proof.
5. **Enforceable policy and bypasses.** Could ws-owned exec reject an
   out-of-pool `git worktree add`, and under what exact trust assumptions?
   Enumerate alternate execution and mutation paths: remaining native tools,
   other MCP servers, subprocesses/interpreters, delegated agents and direct
   filesystem/Git metadata mutation. Identify which are mediated, denied or
   outside the boundary, and whether path/argument inspection is sufficient.
   Owning exec alone does not establish arbitrary policy enforcement. Which
   options are enforceable, advisory or unverifiable under each configuration?

### Rejected Alternatives

No tool-surface alternative has been rejected. Surface selection and any new
mechanism remain open pending evidence and owner decisions.
