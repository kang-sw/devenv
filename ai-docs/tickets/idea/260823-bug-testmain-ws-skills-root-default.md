---
title: "TestMain does not default WS_SKILLS_ROOT, so `go test ./...` fails bare"
related:
  260814-refactor-config-collapse-tuning-knobs-to-list-tune: discovered-during — surfaced while verifying Phase 1; out of scope there
---

# TestMain does not default `WS_SKILLS_ROOT`, so `go test ./...` fails bare

## Background

Running `cd agents-plugin-tool && go test ./...` with no environment setup fails
several `workflow_manual` / `session_state` tests in `cmd/ws-mcp` and
`internal/mcp` with errors like `read skill lead-prefer-subagent: ... no such
file or directory`.

Root cause: `TestMain` (`agents-plugin-tool/internal/mcp/server_test.go`, ~line
35) defaults `WS_RSRC_ROOT` for the package but does **not** default
`WS_SKILLS_ROOT`. Some tests set `WS_SKILLS_ROOT` themselves via `t.Setenv(...)`,
but the tests that rely on skill resolution without doing so fail unless the
caller exports `WS_SKILLS_ROOT` (e.g. `agents-plugin/skills`) before invoking the
suite. So the suite is green only for someone who already knows to set that
variable, and red for a clean checkout / CI invocation that does not.

This was confirmed pre-existing: it reproduces identically on base commit
`9261d87a`, independent of the 260814 Phase 1 change that surfaced it.

## Notes / direction (TBD)

Likely fix: have `TestMain` default `WS_SKILLS_ROOT` to the repo's
`agents-plugin/skills` (mirroring how it already defaults `WS_RSRC_ROOT`) when the
variable is unset, so `go test ./...` is green from a clean environment. Confirm
the correct default path and whether both `cmd/ws-mcp` and `internal/mcp`
`TestMain`s need it. Verify no test depends on the variable being unset.
