---
title: "Refactor Pi Explore onto one persistent spawn path and intent-tier modes"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: prerequisite for an honest web-search intent mode
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: established worker-owned persistent children, subtree waiting, and depth ceilings
  260912-feat-ws-explore-explicit-search-modes: superseded initial feature framing
---

# Refactor Pi Explore onto one persistent spawn path and intent-tier modes

## Background

The recursive-worker bootstrap made Explore persistent for the host lead, fork, ordinary worker, and execute-worker paths. Current registration and dispatch predicates are now equivalent because the lead-like predicate includes workers, so the fallback blocking `runExploreLeaf` branch is unreachable. The one-shot child process, event buffer, registry, no-session allocation, shutdown path, tests, comments, and spec language remain as dead pre-refoundation machinery.

At the same time, the public `deep_research?: boolean` schema biases leads toward an expensive path for ordinary code inspection. It maps false or omission to the configured `small` tier while true inherits the caller's concrete model and effort. Replace both the dead execution split and this difficulty-oriented model split with one persistent Explore contract whose intent mode selects an existing tier alias.

## Decisions

- **One persistent implementation.** Lead, fork, and every worker with remaining delegation budget use the same RPC-backed Explore path, returning exactly `{agent_id, alias}` and retaining the same child/session for `ws-agent-send` continuation. There is no blocking or ad-hoc Explore process.
- **Depth remains authoritative.** Terminal-depth workers receive no child-management or Explore tool. Tool-surface unification applies to eligible dispatchers and does not bypass recursive-worker depth or capability ceilings.
- **Delete dead machinery.** Remove `runExploreLeaf`, its one-shot process/event/registry helpers, no-session scratch allocation used only by that path, shutdown branches, positional runner seams, and tests that exist solely for blocking execution. Do not preserve parallel implementations behind a feature flag.
- **Remove the boolean immediately.** Delete `deep_research` from the public schema, tool descriptions, guides, prompts, runtime types, role environment, tests, and current persisted output. Do not expose or accept a deprecated runtime argument alias.
- **One-time storage normalization is not an API alias.** When recovering an already persisted child record, normalize legacy `simple → code-search` and `deep → synthesis` values at the sidecar/storage read boundary so a reload does not strand a retained conversation. Never write the legacy values again, and remove the migration after the bounded retention horizon makes old records ineligible.
- **Mode selects only a tier.** Every mode uses the same researcher guide, read-only profile, web capability set, persistence, continuation, subtree lifecycle, and child topology. Resolve the mapped alias through existing harness-keyed `agents.tier`; never inherit the dispatcher's concrete model/effort and never choose `xlarge` automatically.
- **Default to code search.** An omitted mode selects `code-search`.
- **Intent modes and mappings:**

  | mode | intent | tier |
  |---|---|---|
  | `lookup` | locate a known symbol, path, or single fact | `small` |
  | `code-search` | trace bounded repository code and tests; default | `small` |
  | `history-search` | trace Git, ticket, or decision history | `small` |
  | `docs-search` | inspect repository, installed-package, or local API documentation | `medium` |
  | `web-search` | collect current external evidence through the separately supplied Pi web tools | `medium` |
  | `diagnosis` | connect code, tests, logs, and runtime observations into a cause | `medium` |
  | `comparison` | compare implementations or alternatives against evidence | `medium` |
  | `synthesis` | reconcile conflicting evidence or produce an architecture-level cross-source conclusion | `large` |

- **Web mode must be honest.** Do not land `web-search` before the related Pi-local bundled search and bounded-fetch capability is present. Once present, all Explore modes receive the same web-capable read-only profile; mode does not gate tools.
- **Continuation is immutable.** Follow-ups retain the child's original mode, resolved model/effort, prompt, session, and authority envelope. Changing mode on a retained child is out of scope.
- **Contrastive descriptions replace salience.** The schema explains neighboring boundaries, especially `code-search` versus `diagnosis` and `comparison` versus `synthesis`. `synthesis` is selected because evidence must be reconciled, not because a task sounds important.
- **Update authoritative behavior text.** Remove stale one-shot/blocking claims from Pi guides, runtime spec anchors, comments, and tests. Keep caller-visible behavior and the test suite aligned with the refounded persistent-worker contract.

## Phases

### Phase 1: Collapse Explore onto persistent intent-tier routing

Remove the unreachable blocking implementation and its dedicated storage, lifecycle, shutdown, and test seams. Simplify registration and dispatch so every eligible lead/worker caller reaches the same persistent `spawnAgent` path, receives `{agent_id, alias}`, participates in subtree obligations, and resumes through `ws-agent-send`.

Replace `simple/deep` and `deep_research` with the settled intent-mode vocabulary and tier mapping. Regenerate Pi-only guidance and schema, update runtime/spec language, persist only new modes, and provide the bounded legacy sidecar read migration without accepting the removed public argument. Verify dead-symbol absence; default and every mode-to-tier mapping; authentication refusal before allocation; lead/worker surface identity; terminal-depth omission; same-session follow-up; compaction and restart recovery; legacy sidecar normalization; unknown/removed argument rejection; web-profile identity after its prerequisite lands; and no regression in recursive subtree completion or capability monotonicity.
