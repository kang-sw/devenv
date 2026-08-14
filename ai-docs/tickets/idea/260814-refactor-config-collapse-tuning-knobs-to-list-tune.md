---
title: "Collapse per-knob config MCP tools into config.list + config.tune"
related-mental-model:
  - mcp-runtime
---

# Collapse per-knob config MCP tools into `config.list` + `config.tune`

> Status: **TBD / needs discussion.** Captured as a direction, not an accepted
> plan. The read and write paths both need design work before this is actionable.

## Problem

The `config.*` surface exposes each tuning knob as its own MCP tool. Today that
is roughly: `config.show`, `config.tuning`, `config.agents_tier`,
`config.bootstrap_alarm`, `config.doc_coverage_alarm`, `config.prompt`,
`config.prompt_set`, `config.prompt_unset`, `config.workflow_prefer_subagent`,
and `config.workflow_prefer_mercenary`. Every knob is a separate function with
its own schema.

This bloats the MCP tool surface and burns context: every one of these schemas
ships in `tools/list` and competes for the model's attention, even though the
underlying operation is almost always "read the config catalog" or "set one key
to a value." The manuals-tier retirement (260814) is the same shape of problem
at smaller scale — a surface wider than the operation it serves.

## Proposed direction

Collapse the write knobs (and possibly the read knobs) into **two** tools:

- `config.list` — one read surface returning the full tunable catalog: keys,
  current resolved values, scope, and (where relevant) allowed values /
  override-point metadata. Subsumes what `config.show` and `config.tuning`
  return today.
- `config.tune("<key>", <object>[, "<harness>"])` — one write surface: set a
  single key to a value, with an optional harness qualifier for the
  harness-scoped prompt-override points. Subsumes `config.agents_tier`,
  `config.bootstrap_alarm`, `config.doc_coverage_alarm`,
  `config.prompt_set`/`config.prompt_unset`, and the
  `config.workflow_prefer_*` writers.

Net effect: ~10 tools → 2, with the per-knob validation living behind a single
dispatch keyed on `<key>`.

## Open questions (why this is TBD, not ready)

- **Scope routing.** The layered scope model is `session > project > global >
  builtin` (`{#260619-layered-config-scope-model}`), and some items are
  global-only and reject non-global writes (`workflow.prefer_subagent`,
  `workflow.prefer_mercenary`), while others are session/project/global. A single
  `config.tune` must carry scope intent per key without letting a caller write a
  global-only item at the wrong scope. Does scope become a third argument, or is
  it inferred per-key from a registry?
- **Per-key value schema / validation.** Each knob has its own value domain
  (tier enums, `on|off|hide`, alarm booleans/thresholds, free-form prompt text).
  A generic `<object>` value loses the per-tool schema-level validation the
  current separate tools give the model. Where does per-key validation and the
  model-facing "what values are legal" hint live — in `config.list` output?
- **Harness qualifier.** Only the prompt-override points
  (`prompt.<pointId>.<harness>`) are harness-scoped; the optional `<harness>`
  arg is meaningless for the other keys. Decide whether that is a soft "ignored
  when irrelevant" arg or a per-key contract.
- **Authority gate.** `config.*` is lead-only via the prefix gate; global writers
  additionally require lead-key authority. That gate must survive the collapse
  (the `session_key` stays authority-only for global-only keys — not a scope
  selector; see the mental-model note against treating it as one).
- **`config.agents_tier` is not routed through the resolver today** — folding it
  into a resolver-agnostic `config.tune` needs care.
- **CLI mirror + runtime.json contract.** Collapsing tools changes the launcher
  contract surface (`runtimeCapabilityCommandNames`, both `runtime.json` files,
  wsflow visibility of `workflow.prefer_mercenary` vs `workflow.prefer_subagent`)
  and the CLI mirror — the same multi-surface sweep the 260814 removal touched.
- **Backward compatibility.** Whether the per-knob tools are removed outright
  (API-surface removal, "Always ask" tier) or kept as thin deprecated aliases
  for a transition.

## Prior art / reference

- `mcp-runtime` mental model: `config.tuning` is already a read-only projection
  over writer schemas + live prompt override-point discovery; it omits
  full-ws-only knobs from its own no-agent catalog
  (`{#260625-tuning-catalog}`). It is the closest existing thing to the proposed
  `config.list` and a natural starting point.
- `wsconfig.Resolver` and the layered scope model (`{#260619-layered-config-scope-model}`).
- The `ws:lead-tune` skill is the lead-facing entry for workflow tuning and would
  need its guidance re-pointed at the two-tool surface.

## Notes

Raised by the user right after the 260814 manuals.list/find retirement, as the
next instance of "MCP tool surface wider than the operation." Explicitly parked
for further discussion before any implementation planning.
