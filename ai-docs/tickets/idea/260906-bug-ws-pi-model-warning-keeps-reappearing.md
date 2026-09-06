---
title: Model warnings keep reappearing during Pi dogfooding
related:
  260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit: related tier validation and warning work; causal relationship unconfirmed
---

# Model Warnings Keep Reappearing During Pi Dogfooding

## Background

The owner reports that model warnings keep appearing during the current Pi
session and requested a separate ticket. The exact recurring warning text,
emitting surface, and recurrence trigger have not yet been isolated; do not
assume this is duplicate delivery rather than repeated legitimate degradation.

Related observations from this session:

- Delegations requested with `small` and `medium` were reported by
  `ws-agent-list` as running `openai-codex/gpt-6-astra`, the parent model.
- An early workflow-manual advisory said Pi's model tier table had no entries
  and agents would inherit the parent model. A later
  `config.resolve_agent(harness: pi, tier: small)` returned backend `codex`,
  model `gpt-5.6-luna`, effort `high`, resolved from `pi`.
- These observations were made at different points in the session. They do
  not prove a stable configuration, the adapter's effective resolution, or
  the cause of fallback. Actual worker effort and the share of usage due to
  model selection remain unverified.
- Related tier-warning Phase 1 source `e5e09187` passed automated tests and
  review locally, but installed-session behavior and owner-live model/auth/UI
  checks were not verified. Do not attribute the current warnings to that
  newly committed code without confirming what the running adapter loaded.

The owner is concerned that model settings are not taking effect and usage is
high. Warning repetition and effective model selection should be investigated
together without assuming either explains all usage.

## Phases

### Phase 1: Isolate recurring warnings and effective model resolution

Capture the literal warning, its emitter, and the action that repeats it.
Compare the running adapter/version and effective configuration with the live
catalog/auth result and actual spawned provider/model. Distinguish stale
session state, rejected configuration, expected per-spawn warnings, and a
true repetition defect using evidence rather than choosing a cause now.

Use the established contract of the related tier-warning work when judging
cardinality; no new suppression policy, configuration change, or fix strategy
is approved by this capture. Determine whether this belongs as a follow-up to
that ticket or a separate defect before implementation planning.

Ticket-only request: goal draining is stopped. Do not restart implementation,
change model settings, or spawn workers merely to populate this idea.
