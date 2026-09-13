---
title: "Refactor Pi Explore onto one persistent spawn path and intent-tier modes"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: prerequisite for an honest web-search intent mode
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: established worker-owned persistent children, subtree waiting, and depth ceilings
  260912-feat-ws-explore-explicit-search-modes: superseded initial feature framing
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: fc33228416be2bf9
sage-review-design-reviewed: fc33228416be2bf9
completed: 2026-09-13
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

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/process-role.ts, agents-plugin-pi/src/agent-sidecar.ts, agents-plugin-pi/src/agent-storage.ts, and Pi guides/tests |
| scope.surface | public-interface | the existing public explore tool schema changes from deep_research to intent modes in agents-plugin-pi/src/spawner.ts |
| scope.new_public_symbol | no | no new tool name or exported caller symbol; explore remains the existing public tool |
| scope.new_type_contract | yes | ExploreMode and the persisted exploreMode/sidecar tuple change in agents-plugin-pi/src/process-role.ts and agents-plugin-pi/src/agent-sidecar.ts |
| scope.test_surface | existing | agents-plugin-pi/test/persistent-explore.test.ts, agents-plugin-pi/test/agent-sidecar.test.ts, and agents-plugin-pi/test/spawner.test.ts |
| complexity.reuse_points | confirmed | reuse the existing RPC-backed spawnAgent, RpcAgentRegistry, sidecar recovery, and delegation envelope in agents-plugin-pi/src/spawner.ts |
| complexity.side_effect_risk | high | deleting the unreachable leaf changes process lifecycle, shutdown, persistence, and nested child dispatch paths |
| risk.correctness | high | incorrect role or mode routing can lose continuation, select the wrong tier, or re-enable a blocking path |
| risk.fit | high | public guidance, schema, persisted records, and worker depth/capability behavior must remain aligned |
| risk.test | high | role registration, tier resolution, rejection-before-allocation, recovery, and subtree completion require existing integration coverage |
| risk.security_or_contract | high | the public argument removal and child capability/profile unification must not widen authority or strand persisted conversations |

## Phases

### Phase 1: Collapse Explore onto persistent intent-tier routing

Remove the unreachable blocking implementation and its dedicated storage, lifecycle, shutdown, and test seams. Simplify registration and dispatch so every eligible lead/worker caller reaches the same persistent `spawnAgent` path, receives `{agent_id, alias}`, participates in subtree obligations, and resumes through `ws-agent-send`.

Replace `simple/deep` and `deep_research` with the settled intent-mode vocabulary and tier mapping. Regenerate Pi-only guidance and schema, update runtime/spec language, persist only new modes, and provide the bounded legacy sidecar read migration without accepting the removed public argument. Verify dead-symbol absence; default and every mode-to-tier mapping; authentication refusal before allocation; lead/worker surface identity; terminal-depth omission; same-session follow-up; compaction and restart recovery; legacy sidecar normalization; unknown/removed argument rejection; web-profile identity after its prerequisite lands; and no regression in recursive subtree completion or capability monotonicity.

### Result (c4f78aab) - 2026-09-12

- Replaced the role-keyed blocking/one-shot implementation with one RPC-backed Explore path for every eligible lead, fork, worker, and researcher caller. Persistent children now share ordinary continuation, recovery, transcript, stop, subtree, and push behavior.
- Replaced the public boolean and simple/deep vocabulary with the eight closed intent modes and their fixed small/medium/large mappings. Omission defaults to `code-search`; removed and unknown arguments fail before tier lookup or allocation.
- Removed the obsolete recon/no-session process machinery and aligned the Pi guides, live model-tier advisory, runtime specification, role metadata, ownership, sidecars, and affected tests. Legacy stored simple/deep records normalize only at read boundaries; obsolete recon records are no longer accepted.
- Strengthened the behavioral contract with literal mode/tier oracles, actual RPC prompt/client and resumed-session assertions, and inspection of the terminal child's real `--tools` allowlist.
- Verification: the affected 19-file test selection passed 606 tests with one intentional skip; the focused bridge/sidecar/Explore selection passed 132 tests; the final Explore selection passed 7 tests; `npm pack --dry-run` succeeded; and `git diff --check` passed. The full `npm test -- --test-reporter=dot` run retained only six pre-existing `fork-lifecycle.integration.test.ts` matrix failures for missing `ws-queue-question`; the same six failures reproduced on an isolated clean `a075db31` worktree.
- Review: round-one correctness passed. Fit and test review raised five Important findings, all fixed. Round-two fit passed; round-two test verification found one remaining Important false-positive assertion, which was fixed by requiring the argv flags themselves. No Critical findings remained, and the two-round cap was respected.


## Resolution (2026-09-13)

Completed the single persistent Explore lifecycle, closed intent-mode tier mapping, legacy persisted-mode normalization, authoritative guidance/spec alignment, and the behavioral coverage required by Phase 1.
