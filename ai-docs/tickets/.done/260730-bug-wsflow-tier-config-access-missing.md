---
title: wsflow has no access point to inspect or configure the tier→model capability mapping
related:
  260620-bug-mercenary-path-visible-when-prefer-off: adjacent mercenary-visibility-in-wsflow precedent; established mercenary dispatch must stay invisible but did not address the shared host-neutral tier→model config surface
  260622-feat-playbook-render-tier-label: introduced the recommended-model render output that wsflow's playbook.render also serves, using the config seam this ticket exposes
  260714-feat-playbook-tier-model-render-vars: shares the resolveTierModel/ResolveAgentForHarnessConfig seam this ticket's config surface backs
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-30
---

# wsflow has no access point to inspect or configure the tier→model capability mapping

## Background

Discovered while dogfooding the wsflow variant of `lead-tune`: there is no
model-tier configuration surface at all in agentless (wsflow) product mode.
Tracing the code confirms `config.agents_tier` is hidden unconditionally in
wsflow — `noAgentHiddenTool` (`agents-plugin-tool/internal/mcp/server.go:4761-4779`)
bundles it with the `mercenary.`-prefix check and `config.workflow_prefer_mercenary`.

This is a functional gap, not just a UX omission: `playbook.render`'s
`recommended-model` output is explicitly documented as "Available in both full
and agentless product modes" (`server.go:4335`), and it is computed from the
exact same `Agents.ModelAliases` table that `config.agents_tier` reads and
writes (`resolveTierModel` → `wsconfig.ResolveAgentForHarnessConfig`,
`wsconfig/config.go:213-256,368-407`). wsflow sessions can observe a
`recommended-model` value with no tool to inspect, understand, or correct the
mapping that produced it.

The gap also has a diagnostic cost, found in the same discussion: the
harness-neutral `"default"` alias bucket (and legacy flat `Agents.Tiers`) can
get silently populated with a Codex/GPT model+backend by any config-write call
path that lacks live harness detection (e.g. the `ws-mcp config agents-tier`
CLI without `--harness`, or `ws-mcp call` JSON-RPC passthrough), and
`playbook.render`'s own tier resolution falls back to that same polluted
bucket whenever its caller also lacks harness detection — even inside a
genuine Claude Code host session. A wsflow user hitting that symptom currently
has no tool available to confirm or fix it themselves. Not fixed by this
ticket; candidate follow-up, not yet created pending separate confirmation.

## Decisions

- Keep all `mercenary.*` tools and `config.workflow_prefer_mercenary` hidden in
  wsflow — mercenary is genuinely absent there and must stay invisible
  (unchanged).
- Un-bundle `config.agents_tier` from the `mercenary.`-prefix / mercenary-tool
  hiding in `noAgentHiddenTool` so wsflow sessions can reach the tier→model
  alias table that `playbook.render` already reads on their behalf.
- Tool text exposed to wsflow must stay free of mercenary vocabulary, per the
  wsflow forbidden-reference list (`ai-docs/ref/wsflow-mirroring.md` Static
  Verification section: no `ws.mercenary.*` mentions in distributed wsflow
  surfaces) — reword the wsflow-visible tool description if the current full-ws
  description leans on mercenary framing.
- Give wsflow's `lead-tune` (and its rsrc/shim counterpart per
  `ai-docs/ref/wsflow-mirroring.md`) an actual entry point to view/set the
  tier→model mapping, matching what full-ws `lead-tune` already exposes.

## Constraints

- Do not expose `mercenary.*` tools, `config.workflow_prefer_mercenary`, or any
  mercenary-specific concept to wsflow as a side effect of this change.
- Preserve `config.agents_tier`'s existing harness-aware resolution behavior
  and output contract for full ws; this ticket only changes wsflow
  *visibility*, not the underlying resolution semantics.
- Follow `ai-docs/ref/wsflow-mirroring.md` for any shared playbook/skill body
  change (shim vs curated body, static verification, rsrc mirror regen
  checklist) if `lead-tune`'s rsrc playbook body changes.

## Phases

### Phase 1: Expose tier→model config in wsflow without mercenary leakage

- Un-bundle `config.agents_tier` from `noAgentHiddenTool`'s mercenary gating
  (`agents-plugin-tool/internal/mcp/server.go:4761-4779`); verify it becomes
  callable (and listed in `tools/list`) under agentless mode while all
  `mercenary.*` tools and `config.workflow_prefer_mercenary` remain hidden.
- Fix the second, independent wsflow gate: `buildTuningCatalog`
  (`server.go:2095`) has its own early return — `if noAgentMode { return
  catalog, nil }` at `server.go:2161` — that executes *before* the
  `"agents.tier"` knob is appended at `server.go:2178`, and does not go through
  `noAgentHiddenTool` at all. `lead-tune`'s playbook body is driven entirely by
  this catalog (`ai-docs/spec/workflow-skills.md:72-85`), so unbundling only
  `noAgentHiddenTool` makes the raw tool callable but still leaves it
  undiscoverable through `lead-tune` in wsflow. Move the `"agents.tier"` knob
  append so it lands outside (before, or after with its own conditional) the
  `noAgentMode` early return, while `"workflow.prefer_mercenary"` (also
  appended after that early return, `server.go:2165-2172`) stays gated behind
  it.
- Audit the tool description/schema text surfaced to wsflow sessions for
  mercenary wording; adjust so the wsflow-visible surface names only the
  host-neutral tier→model concept.
- Add a wsflow-reachable entry point in `lead-tune` (this is the `"agents.tier"`
  catalog knob fixed above — confirm wsflow's `lead-tune` playbook body
  actually surfaces catalog knobs generically, or add the missing wiring if it
  doesn't) so a wsflow user can discover and use this without knowing the raw
  tool name.
- Update `ai-docs/spec/mcp-tools.md`'s Config Tools section to state
  `config.agents_tier`'s product-mode availability explicitly, mirroring how
  `playbook.render`'s own doc already states it.
- Update the existing tests that currently assert the pre-fix (wsflow-hidden)
  behavior, since they will otherwise fail or falsely pass against the new
  contract: `TestPlaybookPrintWsflowLeadTuneOmitsFullWsOnlyCatalogKnobs`
  (`agents-plugin-tool/internal/mcp/playbook_tools_test.go:1011`) currently
  asserts `config.agents_tier`/`agents.tier` are omitted from wsflow's
  `lead-tune` catalog and must be rewritten to assert the opposite while still
  asserting `workflow.prefer_mercenary` stays omitted;
  `TestServeStdioNoAgentModeHidesAgentBackedTools`
  (`agents-plugin-tool/internal/mcp/server_test.go:1292`) currently bundles a
  `config.agents_tier`-hidden assertion together with the mercenary-hidden
  assertions and must be split so only the mercenary ones remain "hidden";
  `agents-plugin-wsflow/runtime.json`, byte-exact-checked by
  `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`
  (`agents-plugin-tool/cmd/ws-mcp/main_test.go:152`), currently has no
  `config.agents_tier` entry and needs one added. `runtime.json` is a
  hand-maintained wsflow manifest (no regen mechanism produces it); add the
  entry by hand, following the version-range pattern already used by every
  other entry in the file.
- Verification: `tools/list` under agentless mode includes `config.agents_tier`
  and excludes all `mercenary.*`/`config.workflow_prefer_mercenary`; the
  `config.agents_tier` description/schema text visible in that agentless
  `tools/list` output contains no mercenary wording; wsflow's `config.tuning`
  catalog includes the `"agents.tier"` knob and still excludes
  `"workflow.prefer_mercenary"`; a wsflow-mode `config.agents_tier` call
  round-trips a tier/model write and is visible in a subsequent
  `playbook.render`'s `recommended-model`; the added `lead-tune` entry point is
  exercised end-to-end (a wsflow session can discover and use it to read/set a
  tier's model without knowing the raw tool name); the three tests named above
  are updated and green; remaining full-ws mercenary-hiding tests
  (`mercenary_surface_test.go` and any other `noAgentHiddenTool` tests) still
  pass; wsflow package tests (`python3 -m unittest discover
  agents-plugin-wsflow/tests`) pass; run the wsflow rsrc/skill mirror regen
  checklist if `lead-tune`'s shared playbook body changed.

### Result (af3fa165) - 2026-07-30

Un-bundled `config.agents_tier` from `noAgentHiddenTool`'s mercenary gating
(one `server.go` case removed) and reordered `buildTuningCatalog` so its
`"agents.tier"` knob append sits before the `noAgentMode` early return while
`"workflow.prefer_mercenary"` stays gated after it. Split `lead-tune`'s shared
`ws:full-only` marker block in two: the mercenary delegation-mode handler and
judge bullet stay gated; the model-tier handler and judge bullet render
unconditionally, giving wsflow an actual `lead-tune` entry point. Added the
`config.agents_tier` entry to `agents-plugin-wsflow/runtime.json`, updated
`mcp-tools.md` and `workflow-skills.md` per Spec Impact, and rewrote/added the
tests named in this phase plus several more surfaced by survey
(`main_test.go`'s two other hidden-tool lists, `prompt_override_test.go`'s
direct `buildTuningCatalog` unit test, and `test_wsflow_runtime_contract.py`'s
own `HIDDEN_TOOLS` set, which the plan's survey missed but which the ticket's
own wsflow-suite verification step exercises directly).

Partitioned review (correctness/fit/test) came back clean on correctness and
fit; the test partition flagged one minor gap — no assertion that
`config.agents_tier`'s tool text stays mercenary-free in the wsflow
`tools/list` output, an explicit verification bullet above that the plan's
test list hadn't covered. Closed in a follow-up commit
(`41b593d3`) with a non-vacuous assertion (verified to fail when mercenary
wording is temporarily injected, pass otherwise).

Deviation: doc closeout also corrected two mental-model bullets
(`mcp-runtime.md`'s `config.tuning` description, `workflow-skills.md`'s "Add a
Codex workflow skill" recipe) that still stated `config.agents_tier`/agent-tier
controls were wsflow-hidden — drift the plan's survey didn't name, found while
checking the landed change against the mental-model corpus (`15e31a44`).

Deferred: the harness-detection-less write path polluting the shared
`"default"` alias bucket (see Background) remains an explicit non-goal,
unticketed pending separate confirmation.

Verification: `go build ./...` clean; targeted and full `internal/mcp`,
`internal/wsrsrc`, `cmd/ws-mcp` package runs green; `python3 -m unittest
discover agents-plugin-wsflow/tests` green (10/10); the new round-trip test
manually confirmed to fail against the pre-fix parent commit and pass
post-fix; `spec_index.verify` clean.

Result commit range: `6efc552d..15e31a44`.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md` Config Tools section
  (`#260513-harness-local-agent-tier-config`), and wherever
  `ai-docs/spec/workflow-skills.md` documents wsflow-only-knob exclusions
  (currently states `workflow.prefer_mercenary` and `config.agents_tier` are
  both full-ws-only around `workflow-skills.md:80-83`; that line becomes
  inaccurate for `config.agents_tier` and needs updating).
- Expected caller-visible change: `config.agents_tier` becomes callable in
  wsflow/agentless mode; wsflow's `lead-tune` gains a tier/model tuning entry
  point; `mercenary.*` and `config.workflow_prefer_mercenary` remain
  wsflow-hidden.
- Contract-first spec: no — implementation confirms exact wording; update the
  spec on landing.


## Resolution (2026-07-30)

Phase 1 shipped on branch impl/wsflow-tier-config-access (range 6efc552d..15e31a44): config.agents_tier un-bundled from wsflow's mercenary-hiding gates (noAgentHiddenTool + buildTuningCatalog), lead-tune given a wsflow model-tier entry point, specs and mental models updated, partitioned review clean. Single-phase ticket; complete. The deferred default-alias-bucket diagnostic-cost problem remains a separate, unticketed candidate follow-up.
