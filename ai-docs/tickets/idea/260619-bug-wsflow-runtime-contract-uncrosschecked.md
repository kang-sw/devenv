---
title: wsflow runtime.json contract is not CI-cross-checked against the live agentless tool surface
---

# wsflow runtime.json contract is not CI-cross-checked against the live agentless tool surface

## Background

`agents-plugin/runtime.json` (the full lead runtime contract) is asserted by
`cmd/ws-mcp` `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`: the
test runs `ws-mcp runtime capabilities` and compares the live tool/command set
against the manifest, so adding or removing a tool fails CI until the manifest
is updated.

`agents-plugin-wsflow/runtime.json` (the agentless product-mode contract,
declared `runtime_capabilities.match: "exact"`) has **no equivalent test**. No
Go test reads that file. It is hand-maintained, so it can silently drift from
the live wsflow tool surface and only fail at launcher runtime (the exact-match
capability check), not in CI.

## Observed

During the session-lineage Phase 3 dogfood (`260619-feat-ws-session-lineage-children`),
adding the `session.children` tool broke the full-surface test (caught in CI)
but left `agents-plugin-wsflow/runtime.json` stale with no test failure.
`session.children` is not in `noAgentHiddenTool`, so it is exposed in wsflow
no-agent mode; the wsflow manifest needed it too and was updated by hand
(`see fix(mcp): add session.children to runtime contract surfaces`). Had the
omission not been noticed, the wsflow launcher's `match: "exact"` check could
have failed at runtime for users while CI stayed green.

## Possible follow-up

- Add a wsflow-mode analogue of
  `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`: run the
  agentless capabilities path (e.g. with the wsflow product-mode env) and assert
  the live tool/command set equals `agents-plugin-wsflow/runtime.json`. This
  closes the silent-drift gap symmetrically with the full surface.
- Alternatively, derive the wsflow manifest from the full manifest minus
  `noAgentHiddenTool`, so the two cannot diverge by construction, and test that
  derivation.

## Notes

- Process learning captured in the fix commit: tool-adding implementation briefs
  should set the verification scope to `go test ./...` (or at least include
  `./cmd/ws-mcp`), not just `./internal/mcp/...`, so contract-surface tests run.
