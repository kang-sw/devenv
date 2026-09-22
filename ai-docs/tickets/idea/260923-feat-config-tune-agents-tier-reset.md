---
title: config.tune agents.tier reset and shadowed-write warning
---

# config.tune agents.tier reset and shadowed-write warning

## Background

Dogfood surprise while tuning global Claude tier mappings through
`ws:lead-tune`:

- `config.tune(key: "agents.tier", scope: "global", harness: "claude", ...)`
  succeeded, but the devenv project config
  (`~/.cache/ws@<project>/config.json`) already carried explicit
  `model_aliases.<tier>.claude` entries, so the project scope shadowed the new
  global values. `config.resolve_agent` still returned the old mapping, and the
  write result gave no hint of the shadowing.
- `agents.tier` exposes no `reset` in the `config.list` catalog (unlike
  `prompt.*`, `workflow.prefer_subagent`, `sage_review`), so there is no
  catalog path to drop a project-scope tier alias back to the inherited global
  value. The workaround was hand-editing the project config JSON, which the
  `lead-tune` playbook otherwise steers away from.

## Phases

### Phase 1: agents.tier reset and shadow warning

- Support `reset: true` for `agents.tier`, selected by `tier` + `harness` +
  `scope`, removing that alias entry so resolution falls through to the next
  scope.
- When a write lands in a scope that a narrower scope overrides for the same
  `(tier, harness)`, return a warning naming the shadowing scope (applies to
  any layered knob, not only `agents.tier`).
- Advertise the reset in the `config.list` catalog entry so `lead-tune` can
  offer it.
