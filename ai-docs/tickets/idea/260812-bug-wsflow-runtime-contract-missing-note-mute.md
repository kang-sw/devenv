---
title: "runtime capability contract fixtures drifted — missing note.mute / note.unmute (wsflow runtime.json + two Go main_test.go want-lists)"
related:
  260812-research-reload-plugins-keeps-stale-mcp-binary: sibling — a different symptom of the same note-visibility landing; that one is plugin-cache staleness, this one is a genuine fixture drift
---

# wsflow runtime.json contract drifted — missing note.mute / note.unmute

## Background

`agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py::
test_runtime_contract_matches_agentless_capabilities` fails: the wsflow contract
fixture `agents-plugin-wsflow/runtime.json` declares
`runtime_capabilities: {match: "exact"}` and asserts its `tools` set equals the
live `go run ./cmd/ws-mcp runtime capabilities` payload. The live payload now
carries `note.mute` and `note.unmute` (the note-visibility feature landed in the
Go runtime), but `runtime.json` was never updated, so the two sets diverge:

```text
AssertionError: Items in the second set but not the first:
'note.mute'
'note.unmute'
```

The same drift shows up in **three** fixtures, all one root cause (the
note-visibility tools shipped in the Go runtime without their contract fixtures
being regenerated):

1. `agents-plugin-wsflow/runtime.json` — the python `exact` contract (above).
2. `agents-plugin-tool/cmd/ws-mcp/main_test.go`
   `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` — the lead
   runtime `want` list.
3. same file `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` — the
   wsflow runtime `want` list.

This is a genuine, pre-existing drift — the three fixtures were untouched by the
260807 branch (`git diff --name-only main...HEAD` shows neither file), and the
change does not touch tool registration, so the failures reproduce on `main`. It
surfaced only because the 260807 `_index.md` dissolution verification pass ran the
full suites. It is distinct from
`260812-research-reload-plugins-keeps-stale-mcp-binary`: that ticket is about a
stale MCP *process* hiding the new tools at runtime; this one is a static fixture
that was not regenerated when the tools shipped.

## Phases

### Phase 1: Repair the contract and close the drift window

The immediate fix is to add `note.mute` and `note.unmute` to all three fixtures —
`agents-plugin-wsflow/runtime.json`'s `tools` list and both `want` lists in
`agents-plugin-tool/cmd/ws-mcp/main_test.go` — so the exact-match contracts pass.
Confirm neither belongs in `HIDDEN_TOOLS` (they are first-class note-visibility
tools, so they should be listed, not hidden).

The more important question is *why* the fixture drifted silently: the wsflow
`runtime.json` tool list is maintained by hand (or by a regen step that was not
run) while the Go runtime is the source of truth. Decide whether this contract
should be generated from `runtime capabilities` (removing the drift class
entirely) or kept as a hand-maintained fixture guarded only by this test. If the
latter, note that the test is the sole guard and every new ws tool must remember
to touch this file — the same silent-drift failure mode as
`260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`, so cross-reference or
fold accordingly.
