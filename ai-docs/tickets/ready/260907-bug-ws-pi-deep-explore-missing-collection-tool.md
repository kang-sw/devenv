---
title: Deep exploration researcher lacks its blocking collection tool in live Pi
related:
  260907-feat-ws-pi-persistent-explore-deep-research: prerequisite; implementation present, live acceptance still unfinished
spec:
  - 260903-pi-explore-recon-leaf
sage-review-design: skipped
sage-review-completeness: skipped
---

# Deep exploration researcher lacks its blocking collection tool in live Pi

## Background

Live dogfood after the owner fully exited Pi and reopened the same session reproduced a contract failure: `explore({ query, deep_research: true })` created a researcher that reported only `read`, `grep`, `find`, `ls` and the parallel wrapper, with no blocking `explore`. It could inspect files directly but could not delegate collection to small. The owner requested a ready bug ticket and explicitly skipped sage review. Merge was subsequently postponed; ticket capture only is authorized now.

Observed deep researcher: `18d88dd1-48ee-4f76-9603-717e63199dd0` / `explore-2`. Its answer explicitly reported that blocking exploration was unavailable; no collection leaf ran. The simple researcher `07f8574e-a616-4df4-b1bc-49c2c53b9cbe` / `explore-1` returned an initial answer and a follow-up on the same identity with the intended read-only surface. These are live observations, not model/effort verification: neither researcher could directly observe its model or thinking level.

The implementation and correction commits are `048a9cb2` and `6998a5b0`; the feature checkpoint is `d796a30`. Automated tests and a generic dynamic allowlist probe passed, but did not prevent this actual deep-child registration failure.

## Decisions

- Restore the already-approved contract: default researchers have no delegation tool; deep researchers have read-only inspection plus blocking `explore`; their small collection leaves have read-only inspection without bash or recursive delegation.
- Preserve the existing public boolean-only interface and persistent lifecycle. Do not expose arbitrary tool-list overrides, widen researchers to full workers, or change ordinary worker/execute-worker recon permissions as a workaround.
- Keep spawn-time model/effort inheritance and fail-closed small selection unchanged.
- Root cause is unconfirmed. Source inspection found the expected deep group, mode environment propagation and role/mode registration gate in `agents-plugin-pi/src/spawner.ts` and `src/process-role.ts`; that does not prove which source, argv or environment the failing live child used.
- A prior investigator inferred a stale parent from the durable session creation timestamp. That inference was withdrawn: the owner confirms full process exit and reopening the same session. A durable session timestamp is not process uptime. Do not prescribe repeated restarts without actual process/loaded-source evidence.
- Adapter-only correction; no ws-mcp or shared rsrc semantic changes on this track. If diagnosis requires a new public contract, stop for owner approval instead of inventing one.

## Investigation (2026-09-08, source trace)

A second source trace (sonnet explore over `spawner.ts`, `process-role.ts`,
`index.ts`, and the vendored Pi engine) reproduces the earlier finding that
the deep-group chain is correct, and adds a strongly-supported mechanism for
why the failure is invisible to both the tests and the earlier trace. Two
adapter-side facts are verified in-source here:

1. **`session_start` registers tools only *after* an unguarded
   `startBridge`.** `index.ts` awaits `handle = await startBridge(...)` with
   no `try/catch` (`src/index.ts:350`), and only on success reaches
   `registerAgentTools(...)` (`src/index.ts:365`), which is what registers
   `explore`, the `ws-agent-*` tools, and `ws-report-to-lead`. `startBridge`
   re-throws launch failures (`src/bridge.ts` tail, `wrapLaunchErrorWith
   LocalDevenvContext`).
2. **Pi's extension runner swallows a thrown `session_start` handler and
   keeps running.** Each handler is wrapped in `try { await handler } catch
   { emitError }` (vendored `dist/core/extensions/runner.js:631-649`), so a
   throw from `startBridge` aborts the rest of the handler — `register
   AgentTools` never runs — but the child process survives.

Consequence: if the explore child's bridge launch throws, that child comes up
with **only Pi's CLI `--tools` builtins** (`read`, `grep`, `find`, `ls`, plus
the parallel wrapper) and none of this extension's custom tools. That is
exactly the reported deep-researcher symptom, and — critically — it is
**indistinguishable from a healthy simple researcher**, whose intended
surface is also read/grep/find/ls. So the simple researcher behaving
correctly is not evidence against a bridge-launch failure, and a fully-mocked
unit test (which fakes `pi`/`RpcClient` and never launches a real bridge)
cannot see it.

**Likely proximate trigger, same day, same call:**
`260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env` documents that
at 21:59 on 2026-09-07 the lead called `explore`; the child inherited a stale
shell `WS_MCP_BOOTSTRAP_BINARY`, took the forced-bootstrap path, and its
launcher failed with `incompatible ws-mcp runtime after repair`. A launcher
failure is precisely what makes `startBridge` throw. The missing-collection
symptom is therefore plausibly a *downstream surface* of that bootstrap bug,
routed through the unguarded `await` above — not a deep-group registration
bug at all.

Not confirmed from source: which engine build the live child ran
(`RPC_CLI_PATH = process.argv[1]` re-execs the *lead's* `pi`; the machine's
global `pi` is 0.85.1 while the package tests against 0.84.4, though the
tool-registration and RPC-bootstrap files diff byte-identical between them
except one unrelated `loader.js` refactor), and whether the observed
`explore-2` was a fresh spawn or a dormant resume. Phase 1's live repro must
still capture the actual launch error of the failing child to nail the
trigger; the two verified facts above only explain why *any* bridge failure
presents as "deep researcher missing `explore`".

Design implication for Phase 1: guarding the `startBridge`/`register
AgentTools` seam (fail loud in the child, or refuse to present a researcher
whose custom tools failed to register) would convert this whole class of
silent, symptom-shifted failures into a visible error — independent of which
bootstrap or version fault first made the launch throw.

## Spec Impact

Restore `260903-pi-explore-recon-leaf` in `ai-docs/spec/pi-adapter-runtime.md`, including mode-specific tool availability and bounded collection. Update affected documentation only if investigation establishes an inaccurate implemented description; do not weaken the spec to accept missing deep delegation.

## Phases

### Phase 1: Restore actual deep-child blocking collection and prove the launch path

Reproduce with a newly created deep researcher after a full Pi process restart while resuming the same session. Establish actual parent/child executable and extension source paths, process start times, launch allowlist, deep role/mode environment and registration timing. Inspect only relevant environment keys; do not dump credentials or whole environments. Distinguish a source/runtime registration bug from a loaded-copy mismatch with concrete evidence. A parked child may require a fresh controlled probe rather than inferring its lost launch environment from source.

Correct the demonstrated failure at its owning adapter/launch boundary, retaining all approved permission, lifecycle and model-selection constraints. If the issue is deployment/source resolution rather than implementation, document and verify the concrete correction rather than making speculative code changes.

Verification:

- Exercise the actual extension launch/registration path used by deep researchers, not merely a generic custom-tool allowlist or static group string assertion. A regression fixture must fail for the reproduced cause and pass after the correction.
- Fresh simple researcher: only read/grep/find/ls; no bash or explore. Fresh deep researcher: those reads plus functional query-only blocking explore.
- Invoke deep blocking collection against a small read-only repository question. Observe the collection's actual tool surface, small model and effort; no bash or recursive explore. Confirm its result reaches the researcher for synthesis.
- Repeat after stop/send and full process exit/reopen of the same session; retain identity, mode, permissions and frozen model/effort. Verify ordinary worker recon leaves remain unchanged.
- Run the adapter suite with clean and inherited role/mode environments. Record full output and actual live evidence separately. Never claim a passed provider/restart gate from mocked tests or child self-report of unobservable model state.
- Record findings back on the prerequisite feature ticket. Its mandatory live acceptance remains incomplete until supported by evidence; this bug's capture does not mark it done.
