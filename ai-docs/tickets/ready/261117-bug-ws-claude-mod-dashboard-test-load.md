---
title: "claude-mod dashboard.test.ts fails to load under the mod test kit (node:fs import)"
related:
  261115-feat-ws-dashboard-observed-session-model: "introduced the capture hook; its Result already notes dashboard.test.ts could not run merge-side"
  261115-feat-ws-claude-reasoning-summary-producer: "overlap: edits dashboard.ts and adds dashboard.test.ts tests; landing this ticket first gives them native coverage; expect a textual conflict in dashboard.test.ts"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 04e0c6b0f49ca33b
sage-review-completeness-reviewed: 04e0c6b0f49ca33b
---

# claude-mod dashboard.test.ts fails to load under the mod test kit

## Background

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
  If a latent-failure fix edits production code that changes the events the
  adapter emits to daemon intake, re-check the tier against
  `ai-docs/manuals/release-compatibility.md` and record it here.
- Owner (1250)=a: delete the `WS_CLAUDE_RETRY_CAPTURE` capture branch and the
  `node:fs` import from `dashboard.test.ts`; keep the TypeScript
  `observed(events)` assertion, and keep the Go embedded literal in
  `live_observed_test.go` as the reducer fixture, with a comment on both sides
  that they pin the same event shape. Live capture (an offline manual aid) is
  retired.
  Rejected: (b) emitting events through the kit's log output for a manual Go
  feed - depends on unverified kit output behavior; (c) moving the capture
  into a Go-side or script harness - largest change with no evidence it is
  needed.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
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
- Out of scope: any new guard or manual note against future test-file load
  failures; not settled with the owner.

## Prior Decisions

- 7d3e6933 (2026-11-15, commit): "Scoped offline tests passed on B without C: Pi helper/rate/display 19 plus dashboard/recursive evidence 9; Claude dashboard 80 using the bounded Node testing shim; captured rejected-call recovery events passed actual Go intake/reducer" — bearing: constrains
- 4dc1e7c3 (2026-11-15, commit): "Compatible with the CURRENT unchanged Claude/Pi adapters: optional missing response evidence degrades to unknown and legacy telemetry/usage meanings remain. No protocol/schema/version/MCP argument changes." — bearing: supports
- 261110-chore-ws-claude-mod-dashboard-engine-test-load-flake (2026-11-13, commit a8059b9a): "chore-ws-claude-mod-dashboard-engine-test-load-flake: investigation recorded, open" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/dashboard.test.ts, agents-plugin-wsflow/hooks/claude-mod/dashboard.test.ts (mirror), agents-plugin-tool/internal/mcp/live_observed_test.go |
| scope.surface | internal | test files only; no exported symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/dashboard.test.ts, agents-plugin-tool/internal/mcp/live_observed_test.go, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | existing observed(events) assertion and embedded Go literal fixture are kept (dashboard.test.ts:1109, live_observed_test.go:15) |
| complexity.side_effect_risk | moderate | once the file loads, never-run dashboard tests may expose latent failures to fix in the same phase |
| risk.correctness | moderate | about 80 dashboard tests have had no native-harness run since 7d3e69331; real failures may surface |
| risk.fit | low | decision (1250)=a is explicit and small; mirror resync is mechanical |
| risk.test | moderate | verification needs two roughly 2.5 minute package suite runs; the kit's process.env reachability is unverified |
| risk.security_or_contract | low | test-only change, no cross-version surface |

## Phases

### Phase 1: Make dashboard.test.ts load and pass in both packages

Apply the (1250)=a decision:

- `agents-plugin/hooks/claude-mod/dashboard.test.ts`: delete the `node:fs`
  import (line 2) and the capture block (lines 1105-1108: the "Offline
  cross-language verification" comment, the `WS_CLAUDE_RETRY_CAPTURE` read and
  the `writeFileSync` call); keep the `observed(events)` assertion.
- `agents-plugin-tool/internal/mcp/live_observed_test.go`: delete the
  `os.Getenv("WS_CLAUDE_RETRY_CAPTURE")` / `os.ReadFile` branch (lines 18-24)
  so the embedded literal is the only fixture, and replace the comment at
  lines 14-15 that says the variable is emitted by the adapter test. Keep the
  `os` import: other tests in the file still use `os.WriteFile`.
- Add a cross-reference comment on both sides. It states that the TS
  assertion and the Go literal pin the same `observed` payload shape (`id`,
  `model`, `source`, `at`, `effort`, `effortSource`, `outputTps`) and turn
  ordering. It must not claim the event streams are identical: the Go literal
  is a hand-reduced sequence (reset, turn_start, status with observed, reply,
  turn_end), while the TS recovered `events` array has more entries.
- Resync the wsflow copy with the rsync command in Constraints.

Verification (read full output):

- `claude plugin test agents-plugin` and `claude plugin test agents-plugin-wsflow`
  each report 0 failures and a test count higher than 764 (the previously
  unloaded file now runs); the run takes about 2.5 minutes per package. In
  each output, the `hooks/claude-mod/dashboard.test.ts:` section lists its
  tests as passing and no longer shows `(fail) the file did not load`.
- `go test ./internal/mcp -run TestObservedClaudeRetryAdapterToReducer` and
  `go test ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate` pass from
  `agents-plugin-tool`.
- If any non-test claude-mod file changed, regenerate Pi workflow bundles and
  native payload and commit them with the change.
