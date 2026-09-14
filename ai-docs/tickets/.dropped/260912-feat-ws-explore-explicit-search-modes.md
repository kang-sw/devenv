---
title: "Pi Explore: replace deep-research attention bias with intent modes and tier mapping"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: prerequisite for an honest web-search intent mode
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: persistent worker-owned Explore continuation and inherited capability ceilings
dropped: 2026-09-12
---

# Pi Explore: replace deep-research attention bias with intent modes and tier mapping

## Background

The public Pi `explore` schema currently exposes `deep_research?: boolean`. Omitted or false selects the configured `small` tier; true copies the dispatching process's concrete model and thinking level. The high-salience `deep-research` label is selected too readily even for bounded repository inspection that the normal researcher can handle, causing avoidable cost and obscuring the intended difference between evidence-gathering tasks.

Replace the boolean intensity switch with intent names that help the lead select an appropriate tier. This is an attention-alias schema layer, not a family of researcher implementations.

## Decisions

- **One researcher implementation.** Every mode uses the same researcher guide, read-only tool profile, persistent RPC/session lifecycle, continuation behavior, and child-management contract. Do not branch the playbook, prompt, tool authority, or collection topology by mode.
- **Mode selects only a tier.** Resolve the mapped `small`, `medium`, or `large` alias through the existing harness-keyed `agents.tier` configuration. Do not inherit the dispatching lead's concrete model and effort and never choose `xlarge` automatically.
- **Default to code search.** Omitted mode selects `code-search`; ordinary repository exploration remains the cheap and capable path.
- **Use intent rather than difficulty language.** Remove `deep-research` from the public vocabulary. `synthesis` is selected only when the task requires combining conflicting or cross-source evidence into a decision, not merely because the topic sounds important.
- **Initial modes and mappings:**

  | mode | intent | tier |
  |---|---|---|
  | `lookup` | locate a known symbol, path, or single direct fact | `small` |
  | `code-search` | trace bounded repository code and tests; default | `small` |
  | `history-search` | trace Git, ticket, or decision history | `small` |
  | `docs-search` | inspect repository, installed-package, or local API documentation | `medium` |
  | `web-search` | collect current external evidence through the separately supplied Pi web tools | `medium` |
  | `diagnosis` | connect code, tests, logs, and runtime observations into a cause | `medium` |
  | `comparison` | compare multiple implementations or alternatives against evidence | `medium` |
  | `synthesis` | reconcile conflicting evidence or make an architecture-level cross-source conclusion | `large` |

- **Web mode requires real capability.** Do not land `web-search` as a name-only promise. Its prerequisite provides search and fetch to the common Explore profile; all modes still receive the same profile.
- **Continuation retains its initial selection.** `ws-agent-send` follow-ups reuse the existing child, session, resolved model, effort, mode, and authority envelope. Changing mode on an existing child is out of scope.
- **Retire deep-only collection semantics.** The current intended deep nested-collector path is not end-to-end active because persistent Explore routing intercepts it. Do not reproduce or repair a mode-specific collector as part of this feature; recursive worker delegation supplies the general child mechanism independently.
- **Make selection guidance contrastive.** Tool schema and Pi lead guidance state when each neighboring mode does and does not apply, especially `code-search` versus `diagnosis` and `comparison` versus `synthesis`.

## Open Decision Queue

- During migration, should `deep_research` be rejected immediately after all shipped Pi guidance is regenerated, or accepted temporarily as a deprecated runtime alias (`false → code-search`, `true → synthesis`)? Keeping it in the model-visible schema would preserve the attention problem, so any compatibility path must not keep advertising the boolean.

## Phases

### Phase 1: Introduce intent-mode tier routing

Replace the boolean schema with the settled mode vocabulary, defaulting to `code-search`, and resolve each mode through its mapped tier. Persist and report the selected mode with the child record and recovery sidecar while preserving the exact `{agent_id, alias}` asynchronous result and same-session continuation behavior. Update Pi-only guidance, runtime types, validation, telemetry, and tests without branching the researcher prompt or tool profile.

Verify every mode-to-tier mapping, config override resolution, authentication refusal before allocation, default behavior, unknown-mode rejection, continuation/restart stability, and the contrastive tool description. Resolve the migration decision before ready promotion and remove stale simple/deep role validation without conflating Explore modes with legacy one-shot spawn-operation names.


## Resolution (2026-09-12)

Superseded before implementation by `260912-refactor-ws-pi-unify-persistent-explore-modes`. The recursive-worker bootstrap already made the blocking Explore fallback unreachable, so the remaining work is a refactor: delete the dead one-shot path, immediately remove the public `deep_research` boolean, and unify eligible lead/worker callers on persistent intent-to-tier routing.
