---
title: generalize the word-chain key generator across id-issuing MCP tools
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: prerequisite — introduces the reusable word-chain key generator, wired to session keys only
  260605-epic-ws-playbook-factory-pivot: pivot epic that lands the generator
  260616-refactor-remove-agent-backed-api-tools: removes api_job_key from the candidate surface before this follow-up
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
---

# Generalize the word-chain key generator across id-issuing MCP tools

## Background

M3 (`260609-refactor-ws-spawn-runtime-deletion-session-auth`) introduces a
reusable, generic word-chain key generator — EFF large 7776-word pool vendored
and `go:embed`-ed, 4 words + a 2-digit suffix, mint-time uniqueness check — but
wires it only to `ws.ferrule` session keys to keep that milestone scoped. This
ticket extends the same generator to the remaining id-issuing MCP surfaces so the
fleet has one consistent, LLM-friendly id idiom (an LLM reliably echoes a
word-chain back; opaque hex/UUID handles are more error-prone in prompts).

Deferred deliberately from M3: re-minting these surfaces touches several distinct
tool contracts and widens the review/verification surface beyond M3's session-auth
focus.

## Candidate Surfaces

- `exec_key` — `exec.spawn` / `exec.shell` capability tokens.
- `api_job_key` — removed from the candidate set once M4
  (`260616-refactor-remove-agent-backed-api-tools`) deletes `api.ask_async`; do
  not recreate it for this follow-up.
- `path.generate` artifact stems.
- Mercenary continuation handles — under M3 these are already native-agentId
  shaped; evaluate whether a word-chain handle adds value or is needless churn.

## Decisions

- Reuse the M3 generator package as-is; do NOT fork a second generator.
- Preserve the generator/policy separation established in M3: a word-chain string
  is not an authorization by itself; each surface keeps its own capability
  semantics. The generator only produces the string.
- Opaque-handle contract: callers already treat these ids as opaque, so changing
  the string shape is caller-visible only as "the id now looks like a word-chain".
  Before changing any surface, confirm no caller parses id structure.

## Constraints

- Do not include `api_job_key` unless M4 explicitly leaves an async api job
  surface in place.
- Each surface migration must remain independently reviewable and revertible.

## Phases

> Backlog sketch; slice by tool family at ready promotion.

### Phase 1: per-surface migration

Migrate the candidate surfaces onto the shared generator, one tool family per
reviewable slice. Verification: each migrated surface mints word-chain ids
through the shared generator with no regression in id uniqueness or opacity, and
no caller parses id structure.
