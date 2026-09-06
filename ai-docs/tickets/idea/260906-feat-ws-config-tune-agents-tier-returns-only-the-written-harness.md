---
title: config.tune(agents.tier) returns only the written harness bucket instead of the whole agents config
spec:
  - mcp-tools
---

# config.tune(agents.tier) returns only the written harness bucket instead of the whole agents config

## Background

Owner feedback, 2026-09-06, from a Pi dogfood session: after
`config.tune(key: "agents.tier", value: {...}, harness: "pi")` the response
prints every harness's tier table, including the `claude` and `codex` buckets
the call never touched. It reads as a leak of unrelated configuration and
makes the one change that was made hard to spot.

The cause is in the handler (`agents-plugin-tool/internal/mcp/server.go`,
`case "config.tune"`, the `entry.Key == "agents.tier"` branch): it calls
`wsconfig.SetAgentsTierForHarness(...)`, which returns the full `wsconfig.Config`
it just persisted, and hands that struct straight to `toolJSONResponse`. Every
other `config.tune` key answers with one line naming the key, the stored value,
and the scope (`<key>: <value> [scope:<scope>]`, or `prompt override set:
<point>/<harness> (scope: <scope>)`), so `agents.tier` is the outlier.

## Proposed direction

Host-neutral ws-mcp change, so it lands through the normal `develop` release
flow, not on the Pi track.

- The `agents.tier` branch answers with only what the call wrote: the
  harness the write landed in (explicit, detected, or `default`), the tier,
  and that tier's stored `backend`/`model`/`effort` after the write. JSON
  shape: `{harness, tier, backend, model, effort, scope: "project"}`. The
  text form, for callers without `format: "json"`, is one line in the same
  family as the scalar knobs, for example
  `agents.tier: pi/small -> backend=pi model=openai-codex/gpt-6-astra effort=low [scope:project]`.
- Nothing else in the config is echoed. A caller that wants the full table
  keeps using `config.list`.
- `SetAgentsTierForHarness` keeps returning the full `Config` for its other
  callers and tests; the handler projects the written bucket out of it, so the
  change is confined to the response shaping.
- No adapter code parses the `config.tune` response; the Pi adapter reads
  tier state back through `config.resolve_agent`, and the `lead-tune` skill
  only shows the response to the model. No adapter change follows.

## Spec Impact

`mcp-tools` `{#260505-config-tools}`: add the response contract for the
`agents.tier` write next to its argument contract, stating that the response
carries only the written harness/tier and its stored fields, and that the full
table remains a `config.list` concern.

## Constraints

- ws-mcp (Go) change under `agents-plugin-tool/`; lands via `develop`.
  It carries no host-specific logic.
- No change to what is persisted or to `SetAgentsTierForHarness`'s signature.
- The JSON field names match `config.resolve_agent`'s (`backend`, `model`,
  `effort`) so a caller can compare the write against a later resolve.

## Phases

### Phase 1: Project the written bucket into the response

Change the `agents.tier` branch of the `config.tune` handler to build the
`{harness, tier, backend, model, effort, scope}` result from the returned
`Config`, add the one-line text form, and amend the spec passage under Spec
Impact. Tests: a write with an explicit harness returns only that harness and
tier; a write with no harness in a session with a detected harness returns the
detected one; a write with neither returns `default`; the response never
contains another harness's bucket; the text form matches the scalar-knob line
family; the persisted config still carries every harness unchanged (read back
through `config.list`).
