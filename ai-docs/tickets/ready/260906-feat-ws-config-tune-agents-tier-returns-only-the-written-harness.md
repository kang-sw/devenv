---
title: config.tune(agents.tier) returns only the written harness bucket instead of the whole agents config
related:
  260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model: adapter-side reading of the stored value; independent, not a prerequisite
spec:
  - mcp-tools
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 54015d12c350eea9
sage-review-completeness-reviewed: 54015d12c350eea9
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

Source confirmation, 2026-09-06: the `agents.tier` branch returns
`toolJSONResponse(req.ID, cfg, err)` with the whole `Config`, always as JSON
with no text form, while the scalar and prompt-override branches format one
line each. `SetAgentsTierForHarness` trims `backend`/`model` and stores the
model verbatim; there is no model-shape validation for any harness (a Pi
test writes a slash-less model and asserts success). That absence is out of
this ticket's scope: the Pi adapter will derive the provider from `backend`
once `260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model`
lands (unlanded at the time of writing), so ws-mcp keeps accepting
backend-keyed slugs unchanged.

The CLI `ws-mcp config tune` subcommand (`cmd/ws-mcp/main.go`) calls the
same setter and prints the whole returned config. Whether that human-facing
output should be projected the same way is not decided here; this ticket
changes the MCP tool response only.

## Proposed direction

Host-neutral ws-mcp change, so it lands through the normal `develop` release
flow, not on the Pi track.

- The `agents.tier` branch answers with only what the call wrote: the
  harness the write landed in (explicit, detected, or `default`), the
  canonical tier, and that tier's stored `backend`/`model`/`effort` after
  the write, as one text line in the same family as the scalar knobs:
  `agents.tier: pi/small -> backend=codex model=gpt-5.6-luna effort=high [scope:project]`.
  When the stored effort is empty the `effort=` token is omitted. No JSON
  form is added: `config.tune` advertises no `format` argument and the
  other keys have no JSON form either; the structured read-back is
  `config.resolve_agent(format: "json")`, whose field names (`backend`,
  `model`, `effort`) the text line reuses so the two are comparable.
- The tier in the line is the canonical name (`small`/`medium`/`large`/
  `xlarge`), not the caller's synonym (`opus`, `deep`, ...): the write path
  normalizes synonyms inside `wsconfig`, so the handler projects the bucket
  through an exported normalizer (a new small `wsconfig` helper exposing the
  existing unexported normalization) rather than the raw argument, which
  would read an empty bucket for a synonym.
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
- The line's field names match `config.resolve_agent`'s (`backend`,
  `model`, `effort`) so a caller can compare the write against a later
  resolve.
- Adding an exported tier normalizer to `wsconfig` is allowed; the setter's
  signature and behavior are unchanged.

## Phases

### Phase 1: Project the written bucket into the response

Change the `agents.tier` branch of the `config.tune` handler to build the
one-line text form from the returned `Config`, reading the bucket through
the exported tier normalizer and the already-resolved harness, and amend
the spec passage under Spec Impact. Tests: a write with an explicit harness
returns only that harness and tier; a write with no harness in a session
with a detected harness returns the detected one; a write with neither
returns `default`; a write with a synonym tier (`opus`) answers with the
canonical tier (`large`) and the stored fields; a write without effort
omits the `effort=` token; the response never contains another harness's
bucket; the text form matches the scalar-knob line family; the persisted
config still carries every harness unchanged (read back through
`config.list`).
