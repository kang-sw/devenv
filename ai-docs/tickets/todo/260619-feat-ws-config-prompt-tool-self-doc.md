---
title: config.prompt.* setter + self-documenting override listing
parent: 260619-epic-ws-layered-config-prompt-tuning
related:
  260619-feat-ws-layered-config-scope-substrate: prerequisite — setter writes through the layered scope primitive
  260619-feat-ws-prompt-override-marker-engine: prerequisite — listing tree-scans the override markers this defines
  260619-feat-ws-lead-tune-skill: consumer — the ws:lead-tune skill owns the tuning manual and calls this data plane; config.prompt() points users there
related-mental-model:
  - prompt-bundle
---

# config.prompt.* setter + self-documenting override listing

## Background

The prompt-override surface must be discoverable and tunable from inside the MCP
(epic `260619-epic-ws-layered-config-prompt-tuning`): an agent should be able to
see every overridable point and set one without external docs.

## Decisions

- **Own namespace `config.prompt.*`** for this new feature (does not disturb
  `config.agents_tier`/`config.show` or the pending `config.model_alias` rename).
- **`config.prompt.set(pointId, harness, prompt, scope?)`** writes a prompt
  override keyed by `(pointId, harness)` (`harness` ∈ `claude | codex | "*"`),
  stored at the chosen scope via the layered primitive
  (`260619-feat-ws-layered-config-scope-substrate`); overrides live inline in the
  single config file (no split resource-files).
- **No-arg `config.prompt()` = data listing + pointer (scope trimmed).** Tree-scan
  the rsrc playbooks for declared override markers (the A1 grammar from
  `260619-feat-ws-prompt-override-marker-engine`) and return the **data**: the list
  of override-points with their short `desc`, plus current overrides per
  harness/scope. End with a **one-line pointer to `ws:lead-tune`** for the tuning
  manual / how-to. The marker scan is the listing mechanism.
  - **The tuning manual is NOT rendered here.** It moved to the `ws:lead-tune`
    entry skill (`260619-feat-ws-lead-tune-skill`); keeping the manual out of the
    MCP output keeps `config.prompt()` a lean data surface. (Originally this
    ticket inlined a manual from an rsrc infra doc — superseded by the
    discuss-confirmed three-plane split: data here, manual + discovery in the skill.)

## Phases

### Phase 1: config.prompt.set setter

Add the `config.prompt.*` namespace and `config.prompt.set(pointId, harness,
prompt, scope?)` that writes the override through the layered config primitive at
the resolved scope.

Depends on `260619-feat-ws-layered-config-scope-substrate` Phase 1.

Verification: an override set via the tool is honored at render time by the
override engine for the matching `(pointId, harness)` and resolves at the
expected scope.

### Phase 2: config.prompt() data listing

Implement no-arg `config.prompt()`: tree-scan override markers, render the point
list with `desc` and current overrides (harness/scope), and end with a one-line
pointer to `ws:lead-tune`. No tuning manual is rendered here (it lives in the
skill child).

Depends on Phase 1 and `260619-feat-ws-prompt-override-marker-engine` (marker
grammar to scan).

Verification: listing enumerates all declared override-points with descriptions,
reflects an override set in Phase 1 with its scope, and emits the `ws:lead-tune`
pointer.
