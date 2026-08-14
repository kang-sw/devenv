---
title: wsflow runtime contract omits note.mute / note.unmute (red test on develop)
related:
  260810-note-tools: context — note.mute/unmute are part of the shipped note surface these runtime contracts should cover
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-14
---

# wsflow runtime contract omits note.mute / note.unmute (red test on develop)

## Background

`python3 -m unittest discover agents-plugin-wsflow/tests` fails on `develop` at
`test_runtime_contract_matches_agentless_capabilities`
(`test_wsflow_runtime_contract.py`): `note.mute` and `note.unmute` are in the
expected agentless capability set but absent from
`agents-plugin-wsflow/runtime.json`. The failure pre-exists this session's work
(reproduced on a clean HEAD with all local edits stashed), so it is a standing
drift bug, not a regression.

The MCP server exposes `note.mute` / `note.unmute` (they appear in the live tool
inventory), but both `agents-plugin/runtime.json` and
`agents-plugin-wsflow/runtime.json` list only `note.write` / `note.search` /
`note.erase`. The wsflow package test compares its contract against the full
agentless capability set and catches the gap; the ws package test suite does not
flag it (either it lacks the equivalent assertion or its expected set also omits
the two tools — confirm during the fix).

## Spec Impact

- The note.mute / note.unmute behavior is already specified in `mcp-tools.md`,
  `## Note Tools {#260810-note-tools}` (mute/unmute set `visible` false/true,
  gating the `# Notes` injection). This ticket reconciles the drifted runtime
  contract to that existing spec; it introduces no new caller-visible behavior
  and needs no spec change. If the fix reveals the ws contract intentionally
  excludes either tool, record that exclusion in the spec instead.

## Phases

### Phase 1: Add note.mute / note.unmute to the runtime contract(s)

Add `note.mute` and `note.unmute` to the runtime tool contract so
`test_runtime_contract_matches_agentless_capabilities` passes. Determine whether
the fix belongs in `agents-plugin-wsflow/runtime.json` only or in both packages'
`runtime.json` (both currently omit the tools; decide against the intended
agentless capability surface). Apply version-edition changes through
`agents-plugin-tool/scripts/bump-ws-version.sh` if the contract edit rides a
version-tied surface, never by hand-editing the edition points. Verify: both
`agents-plugin/tests` and `agents-plugin-wsflow/tests` pass.

### Result (baf8788) - 2026-08-14

Decision: added `note.mute` / `note.unmute` to **both** packages'
`runtime.json` (`agents-plugin/` and `agents-plugin-wsflow/`), at the same
`">=0.40.3-dev <0.41.0"` range as their `note.*` siblings. `note.*` is
unconditionally live (no `noAgentHiddenTool` gate), so both packages carry the
identical set; the ws suite did not flag the gap because it lacks the wsflow
suite's full-agentless-capability assertion, not because of an intentional
exclusion — no spec change needed (behavior already in `#260810-note-tools`).

Verification: `test_runtime_contract_matches_agentless_capabilities`
(`agents-plugin-wsflow/tests`) now passes. No version-edition point was
hand-edited and no `bump-ws-version.sh` run this phase — the patch bump rides
the merge into `develop` per the AGENTS.md version rule, not this contract edit.
