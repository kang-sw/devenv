---
title: "Auto-approve read-only ws-execute commands with a typed decision model; mutations still go to the lead"
related:
  261003-feat-pi-execute-worker-git-read-tools: prerequisite; structural removal of git inspection approvals lands first, and this ticket's value is measured on the residual
  261003-research-typed-decision-model-new-domains: origin; decision-model survey and the discussion that narrowed this to read-only
  260904-feat-ws-pi-execute-approval-gateway: origin constraints; §2 per-mutation gate invariant kept, §5 no-string-classification partially reversed
---

# Auto-approve read-only ws-execute commands with a typed decision model; mutations still go to the lead

## Background

Every `ws-worker-exec` command an execute-worker issues costs a lead turn,
including commands that change nothing. Once
`261003-feat-pi-execute-worker-git-read-tools` lands, the remaining read-only
requests in the historical sample are:

- 39 non-git inspection requests (`date`, `env`, smoke scripts, version probes);
- 18 cross-worktree `git -C` inspection requests;
- possibly 13 build/test requests.

That is up to ~70 of 327, about 20%.

## Decisions

### Scope and relation to 260904

- **Read-only auto-approval only.**
  - A typed decision model (TypeSafe Jev or equivalent) may auto-approve **only** commands it judges read-only.
  - `risky_edit`, `irreversible` and secrets-touching are vetoes.
  - Every mutation, including non-risky reversible ones, still elevates to the lead.
  - Rejected: auto-approving non-risky mutations. It would reverse `260904` §2 and wave through calls the lead uses as review checkpoints (e.g. a `git commit` denied over a correctness flaw). Routine mutation belongs on an ungated general worker.
- **§2 is kept to the letter; §5 is partially reversed.**
  - §2 holds because non-mutations were never what the per-mutation gate protects.
  - §5's rejection of command-string classification is partially reversed. That is accepted because the execute-worker is cooperative, the residual risk is model error or injection, and layered vetoes bound it.
- **Its own ticket.** It takes `261003-feat-pi-execute-worker-git-read-tools` as prerequisite, so its value is measured on the residual.

### Where and how it runs

- **Pi extension only.**
  - The classifier lives entirely inside the Pi extension and reaches the hosted backend over Pi's existing outbound path.
  - There is no ws-mcp change and no `decision.ask` tool. The research ticket keeps that interface deferred, and ws-mcp has no outbound network (Rules 3–4).
- **Opt-in, fail to the current path.**
  - Default off.
  - Any backend error, timeout or missing configuration falls back to the current lead-approval path, so auto-approval is only an optimization, never a new failure mode.
- **Data egress is stated.** The opt-in setting's description says that enabling it sends each proposed command and its rationale to the configured third-party backend.
- **Visibility.**
  - Auto-approved commands surface to the lead as a compact display row (widget or tool-row), never as an injected turn.
  - They are recorded in the agent's approval log with the decision probabilities.
  - Rejected: silent with an audit log only, which loses supervision of what ran.
  - Rejected: a context-injected notice, which spends the lead context this ticket exists to save.

### What the classifier judges

- **Definition of read-only.**
  - **Read-only:** the command changes no tracked or untracked file in any worktree, no git ref, index, config or stash, nothing on a remote, no running process, and no file outside the OS temp directory.
  - **Still read-only:** writes confined to the OS temp directory and to toolchain caches (Go build cache, npm cache).
  - Rejected: strict read-only where any write disqualifies. It excludes most build/test commands, and historical inspection commands routinely wrote scratch output to `/tmp`.
- **Decision shape fixed; numbers come from the replay.**
  - Auto-approve only when `P(read_only)` clears a high threshold, both `P(risky_edit)` and `P(irreversible)` stay below a low threshold, and a deterministic check finds no secrets reference (`.env`, `*_KEY`, `*TOKEN*`, credential paths).
  - Thresholds come from the Phase 1 replay, because calibration is per decision type and must come from this repo's own labeled data.
  - Rejected: fixing numbers now.

### Backend

- **Backend and credential.**
  - Use Jev through OpenRouter's decisions endpoint, behind a thin in-adapter provider seam so another backend can slot in later.
  - The OpenRouter key comes from Pi provider auth (`ctx.modelRegistry.getProviderAuth("openrouter")`, the same seam `fork-context.ts` already uses).
  - No env-var or config setting holds or names a key.
  - Rejected: an env-var-named key setting. It is redundant now that OpenRouter is a configured Pi provider.
  - Rejected for now: a light-LLM logprob backend.

## Phases

### Phase 1: Offline replay evaluation

- **Labeling.** Label the historical `ws-worker-exec` approval requests
  recoverable from local Pi lead sessions as read-only or not, under the
  definition above. An LLM does the first pass and the lead agent spot-checks
  the labels.
- **Replay.** Replay every labeled request (command plus rationale) through
  Jev. Report the would-auto-approve rate and the false read-only count on
  mutating commands, then propose thresholds.
- **Gate:** Phase 2 proceeds only if the replay shows zero false read-only on
  mutating commands at the proposed thresholds.
- **Data:** the replay data is the user's local session history and is never
  committed. Only aggregate results go into the phase Result.
- **Scope:** Jev only. A light-LLM baseline is added only if Jev fails the
  gate.
- Rejected: skipping the replay and relying on live shadow mode only. The
  replay costs under a dollar and decides whether the feature is worth shipping
  before any adapter code exists.

### Phase 2: Opt-in read-only auto-approval in the execute gateway

- **Classifier and config.** Implement the classifier in the execute gateway's
  approval path with a `pi.*` adapter-config mode `off | shadow | on`
  (default `off`). `shadow` logs decisions without acting.
- **Behavior:** apply the decision shape, the vetoes, the fail-to-lead
  fallback and the visibility rules above.
- **Verification:**
  - With the backend unavailable, every request reaches the lead exactly as today.
  - In `shadow` mode no request is auto-approved.
  - In `on` mode a vetoed or below-threshold command reaches the lead, and an auto-approved command produces a display row and a log entry but no lead turn.
