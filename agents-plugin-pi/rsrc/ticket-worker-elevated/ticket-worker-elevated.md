---
kind: render
delegates: true
role: worker
tier: large
includes:
  - worker-stop-protocol
variables:
  - RoleModel
  - ExploreAgent
  - SpawnIdiom
---
# Ticket Worker

Execute the ticket in your task block starting from the branch named there,
using the session key spliced into this file. You are an orchestrating
mini-lead: drive its remaining phases to a close, keeping the interface and
hard core in your context while pushing decomposable implementation into
fresh-context leaves. The Worker Protocol appended below governs; read it
first.

## Scope

- You own **every phase** without a `### Result` — not one phase (later phases
  leave your scope only when the task block says so). Lifecycle steps 2–4 run
  across your owned phases, recording a `### Result` for each completed phase
  in order.

## Implement

Runs between **Route** and **Review** in the Worker Protocol below:

1. **Decompose** along the natural seam, not the phase list: fix the
   interface/contract (the spine) first, then split what remains into disjoint
   implementation leaves and test contracts (fall back to phase-serial leaves
   when no disjoint seam exists). Decomposition is planning you hold, not work
   you delegate. For a broad read you do not have, spawn {{.ExploreAgent}}
   (read-only, citing paths); its answer is evidence, not your plan.
2. **Own the spine and hard core** in your own context: interfaces, contracts,
   and any non-decomposable hard part. Before leaves build on the spine,
   consider a fresh review of its interface and test-contract prose (same
   Review wrappers below) unless trivial or already covered by design review —
   a recommendation keyed to leverage, not a gate.
3. **Run leaves one at a time** on your single warm worktree, never in
   parallel (prevents concurrent seam-write races and keeps warm-incremental
   builds). Render `delegate-implementer` via
   `{{.McpNamespace}}/playbook.render(name: "delegate-implementer",
   session_key: <your key>)`, resolve its model at the leaf's difficulty tier
   with `{{.McpNamespace}}/config.resolve_agent(tier)`, and spawn by
   {{.SpawnIdiom}} using the rendered path and resolved model with the ticket
   path, branch name, exact files, spine, and verification. Each leaf builds,
   tests, and commits on the shared branch so logs stay in its context (batch
   a heavy build across coherent leaves when that amortizes it).
   Calibrate by difficulty, not a quota: mechanical, disjoint,
   cheaply-verifiable leaves are delegated by default; hard, high-leverage, or
   non-decomposable work stays with you (a ticket with no clean mechanical
   leaf runs direct).
4. **Verify** integrated build and test commands (from project docs, cited
   manuals, or build configuration) and resolve introduced warnings whenever
   builds were batched or you edited code directly.
