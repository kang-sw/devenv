# Capture unified agent-tool-surface research

## Confirmed request

MAIN relayed owner (853)=a on 2026-11-15 at 01:16:53 UTC: author a research ticket, parallel to TASK3 but without delaying it, comparing a complete ws-mcp agent toolset (read/grep/find/edit/exec, with harness-native tools disabled) against per-harness tool-surface adaptation. This captures research, not an architecture decision or implementation authorization.

## Deliverable

Populate existing stub ai-docs/tickets/idea/261115-research-ws-unified-agent-tool-surface.md in English as a no-phase research ticket with the standard Outcome Ledger. Link parent261114-epic-ws-common-session-foundation and related261114-refactor-ws-common-session-interface after reading those exact known tickets for boundary constraints. Research proceeds as independent evidence, not a new foundation implementation prerequisite. Do not modify those other tickets.

Scope Codex, agy (preserve the owner's exact harness name; relate Antigravity only where identity is evidenced), Claude and Pi. Record the owner's expectation that Pi may mostly work already as an unverified expectation to confirm cheaply, not a finding. Cover all five required questions:
1. Per exact harness/version, can native tools be disabled/allowlisted and MCP tools substituted? Require official docs/CLI flags/config and separate documented support, source evidence and runtime verification, with gaps explicitly unverified.
2. Design a small paired accuracy experiment using the same tasks for native versus ws-mcp toolsets, per model, measuring edit success, tool-choice errors, tokens and latency. The research should propose controls, repeatability, task grading, safety bounds and unresolved model/tooling differences; this capture approves neither particular thresholds nor execution.
3. Assess native safety/UX lost or retained, including Codex sandbox, approval prompts and diff rendering, and alternatives; distinguish MCP server authority from harness execution mediation.
4. Map existing wsagent Profile tools/ws_tools restrictions and their actual trust/enforcement boundary to each candidate surface; do not invent profile semantics.
5. Assess enforceable policy interaction, such as rejecting out-of-pool git worktree add when exec is ws-owned, including alternate execution/mutation paths and bypass limits. Do not claim owning exec alone makes arbitrary policy enforceable.

Acceptance for the future research is an evidence matrix, reproducible experiment design (not fabricated results), safety/trust-boundary comparison, profile mapping, enforceability/gap analysis and decision-ready options for the owner. Keep all unchosen mechanisms under Proposals/Open Questions, not Confirmed Decisions. Do not favor a surface prematurely, create phase plans, add a hard dependency, approve new MCP APIs/store/protocol shape or implement/restrict tools.

## Work limits and verification

This assignment authors the ticket only. Read ticket conventions, the two linked foundation tickets, and directly necessary repo evidence; no broad research run, external network/provider/download, experiments, code tests/builds, daemons, process signals or native harness behavior changes. If inspecting Pi harness capability claims, read relevant installed Pi docs/examples in full first, and mark anything not checked as unverified. A sparse housekeeping checkout is intentional; do not widen it or allocate another checkout. Use precise edit for the stub and add this write-once plan to one docs-only commit with AI Context. Verify ticket guardrails and diff; expected branch docs/exec3-tool-surface-research. Do not merge or push or release the checkout yourself; return the exact head and evidence so root can consume it and immediately release. MAIN alone mints decision IDs; cite existing (853) only. Report in plain English, complete sentences and short VERIFIED/INFERRED citations.
