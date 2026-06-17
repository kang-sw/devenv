---
title: make MCP subagent context stateless and filesystem-backed
related:
  260605-research-ws-native-subagent-pivot: native-subagent pivot depends on predictable delegate access to workflow context
  260616-refactor-wsflow-product-mode-convergence: dogfood surfaced unknown session keys from rendered delegate prompts
related-mental-model:
  - mcp-runtime
  - prompt-bundle
---

# make MCP subagent context stateless and filesystem-backed

## Background

Dogfooding during wsflow product-mode convergence surfaced an unstable MCP
instance boundary: a native subagent that received a rendered playbook prompt
with a minted session key reported the key as `unknown_session` when it tried to
use ws MCP tools. The likely cause is that subagents sometimes receive a fresh
MCP server instance instead of sharing the lead's in-memory session registry.
The behavior appears harness-dependent or unstable, so in-memory lead-to-delegate
session state is not a reliable delegation contract.

The desired direction is to make MCP delegation context stateless from the
server process perspective. Data needed by subagents should be recoverable from
filesystem-backed artifacts or explicit prompt material, not from an in-memory
registry that assumes a shared MCP process.

## Decisions

- Treat subagent MCP process identity as unstable: a subagent may or may not
  share the lead's MCP server instance.
- Do not rely on render-minted in-memory session keys as the only path for
  subagent access to repository context.
- Prefer filesystem-backed context materialization for delegate prompts,
  credentials, root binding, and workflow handoff data where feasible.
- Keep this as a separate design ticket; do not block narrow runtime-surface
  cleanup work that can proceed without delegated MCP calls.

## Phases

### Phase 1: characterize current subagent MCP instance behavior

Survey Codex and Claude native subagent MCP startup behavior. Record when a
subagent shares the lead MCP instance, when it starts a separate instance, and
which prompt/render flows currently depend on in-memory session continuity.
Verification: a short matrix with reproducible probes for same-instance,
fresh-instance, and unknown-session outcomes.

### Phase 2: design stateless filesystem-backed delegation context

Define the replacement contract for delegate root binding and workflow context.
The design should specify which data is embedded directly in rendered prompts,
which data is written to worktree-scoped files, how delegates discover those
files, and which existing MCP tools stop depending on per-process session
registry continuity. Verification: spec or mental-model updates identify the
new source of truth and rejected alternatives.

### Phase 3: implement stateless delegate context path

Implement the chosen filesystem-backed or prompt-embedded context path and
update playbook rendering, MCP root resolution, tests, and docs accordingly.
Verification: native subagents can use rendered delegate prompts even when they
start with a fresh MCP server instance; existing lead-session behavior remains
compatible or has an explicit migration path.
