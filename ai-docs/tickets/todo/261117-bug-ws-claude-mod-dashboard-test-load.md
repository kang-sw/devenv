---
title: "claude-mod dashboard.test.ts fails to load under the mod test kit (node:fs import)"
related:
  261115-feat-ws-dashboard-observed-session-model: "introduced the capture hook; its Result already notes dashboard.test.ts could not run merge-side"
---

# claude-mod dashboard.test.ts fails to load under the mod test kit

## Context

Dogfood surprise reported by a worker: every full claude-mod suite run in
both packages ends "763 pass / 1 fail", and the one failure is a whole test
file that never loads, not a failing assertion.

Reproduced read-only with the package's own runner (no package.json exists;
the runner is the host's `claude plugin test <plugin dir>`, one child per
`*.test.ts(x)` file, kit imported from `'claude-code/testing'`):

```text
hooks/claude-mod/dashboard.test.ts:
(fail) the file did not load
  ... cannot import "node:fs" (from hooks/claude-mod/dashboard.test.ts): a hooks
  module imports its own files by relative path and "claude-code", nothing else
 763 pass / 1 fail / Ran 764 tests across 18 files.
```

Root cause: `agents-plugin/hooks/claude-mod/dashboard.test.ts:2` imports
`writeFileSync` from `node:fs`; it is used once, at line 1108
(`if (capture?.startsWith('/tmp/')) writeFileSync(capture, JSON.stringify(events))`),
to dump recovered adapter events when `WS_CLAUDE_RETRY_CAPTURE` is set. The
host loader restricts mod and test modules to relative imports plus
`claude-code`; the restriction is host-owned, so there is no allowlist in
this repository to widen. Because the file fails at load, none of its tests
run: the "763 pass" figure excludes all of `dashboard.test.ts`, so the
dashboard adapter tests currently have no native-harness coverage at all
(the earlier ticket's Result also records a merge-side failure to run it).

Introduced by `7d3e69331` (2026-11-15, "feat(adapters): report own completed
response identity with ordered Claude retry"), the only commit that adds the
import (`git log -S"from 'node:fs'"`). The wsflow copy
`agents-plugin-wsflow/hooks/claude-mod/dashboard.test.ts` is byte-identical
(`diff -rq` of both claude-mod directories is clean), so the same failure
occurs there.

Consumer of the capture: `agents-plugin-tool/internal/mcp/live_observed_test.go:14-24`
(`TestObservedClaudeRetryAdapterToReducer`) reads `WS_CLAUDE_RETRY_CAPTURE`
when set and otherwise falls back to an embedded literal fixture. The
fallback is what the default `go test` runs, so the Go side does not need the
file.

Not verified here: whether `process.env` is even reachable inside the kit's
sandbox (the kit has no process access per
`agents-plugin/tests/test_claude_mod_harness_pid_probe.py:4`); once the
import is gone the test body must be run to find out.

## Decisions

- The fix is in the test or its consumer, not in the host loader: the import
  restriction is enforced by the installed host and is not configurable from
  this repository.
- Pi bundles and the native payload do not need regeneration for a
  test-file-only change: `agents-plugin-pi/scripts/prepare-workflow-bundles.py:25`
  excludes `.test.ts` and `.test.tsx`, and `agents-plugin-pi/workflow-bundles/manifest.json`
  lists no test files. If the chosen fix touches a non-test claude-mod file
  (for example `dashboard.ts`), rerun both prepare scripts and commit the
  regenerated output. Compatibility tier: n/a (test-only, no cross-version surface).

## Open Decisions

1. How to keep the Go reducer cross-check without `node:fs`.
   - (a) Recommended: delete the capture branch and the import; keep the
     TypeScript `observed(events)` assertion, and keep the Go embedded
     literal as the reducer fixture, with a comment on both sides that they
     pin the same event shape. Simplest; loses live capture, which was an
     offline manual aid.
   - (b) Emit the events via the kit's console/log output and have a manual
     script feed Go. Keeps live capture but depends on kit output behavior
     that is unverified.
   - (c) Move the fs-dependent capture into a Go-side or script harness that
     drives the adapter. Largest change; no evidence it is needed.
2. Whether to add a guard so a load failure cannot sit as a "known baseline
   failure" again (for example a note in the mod-test manual that the
   baseline is 0 failures). Recommended: yes, as a one-line manual note only;
   no new tooling.

## Constraints

- ws and wsflow `hooks/claude-mod/` copies must stay byte-identical
  (`ai-docs/manuals/wsflow-mirroring.md`, "Claude Mod Mirror"); resync with
  `rsync -a --delete agents-plugin/hooks/claude-mod/ agents-plugin-wsflow/hooks/claude-mod/`.
  `TestWsflowHooksMirrorUpToDate` guards this.
- Read `ai-docs/manuals/shipped-surface-boundary.md` (test files ship with
  the package tree) and `ai-docs/manuals/release-compatibility.md` before
  editing.
- Mod test files may import only `claude-code` and relative files; do not
  introduce any other import to fix this.
- Once `dashboard.test.ts` loads, tests that were never run natively may
  expose latent failures; fix real ones in this phase rather than deferring.
- English only; no session keys, PIDs or absolute local paths in files.

## Phases

### Phase 1: Make dashboard.test.ts load and pass in both packages

Apply the chosen option from Open Decisions 1 to
`agents-plugin/hooks/claude-mod/dashboard.test.ts`, resync the wsflow copy,
and update the comment in `live_observed_test.go` if the capture variable is
retired.

Verification (read full output):

- `claude plugin test agents-plugin` and `claude plugin test agents-plugin-wsflow`
  each report 0 failures and a test count higher than 764 (the previously
  unloaded file now runs); the run takes about 2.5 minutes per package.
- `go test ./internal/mcp -run TestObservedClaudeRetryAdapterToReducer` and
  `go test ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate` pass from
  `agents-plugin-tool`.
- If any non-test claude-mod file changed, regenerate Pi workflow bundles and
  native payload and commit them with the change.
