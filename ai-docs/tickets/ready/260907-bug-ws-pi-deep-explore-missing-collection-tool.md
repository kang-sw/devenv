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

Split 2026-09-08 after the source trace (see Investigation). Phase 1 is the
structural fix that stands regardless of the exact trigger and can land now;
Phase 2 is blocked on live dogfood evidence and may reduce to a delegation
once that evidence lands.

### Phase 1: Fail loud when a child's custom-tool registration does not complete

Guard the `session_start` seam so a child that fails to register this
extension's custom tools never comes up silently presenting only Pi's
builtin `--tools`. Today `index.ts` awaits `startBridge` with no `try/catch`
(`src/index.ts:350`) ahead of `registerAgentTools` (`src/index.ts:365`), and
Pi's extension runner swallows a thrown `session_start` handler
(`dist/core/extensions/runner.js:631-649`), so a bridge-launch failure drops
every custom tool and the child looks like a healthy simple researcher. Make
that failure visible at its owning adapter boundary: on a `startBridge` (or
`registerAgentTools`) failure in a spawned child, fail the child loudly with
the underlying launch error rather than continuing toolless — for an explore
child specifically, a researcher whose blocking `explore` (deep) or expected
read surface did not register must surface an error to its parent, not a
silent partial surface. Retain all approved permission, lifecycle and
model-selection constraints; do not widen any tool group as a workaround.

Verification:

- A regression fixture drives the real `session_start` registration path (not
  a generic custom-tool allowlist or static group-string assertion) with a
  forced `startBridge` failure, and asserts the child fails loudly / the
  parent sees an error instead of a toolless researcher. It must fail before
  the guard and pass after.
- With `startBridge` succeeding, a fresh simple researcher still shows only
  read/grep/find/ls (no bash, no explore) and a fresh deep researcher shows
  those reads plus a registered blocking `explore`.
- Run the adapter suite with clean and inherited role/mode environments;
  record full output. Ordinary worker/execute-worker recon leaves unchanged.

### Result (55172110) - 2026-09-09

Phase 1 complete on branch
`impl/goal/track/pi-agent/copper-lantern-drift/expel-cycle-patio`, range
`4384ac46..55172110` (single source+test commit).

Guarded the `session_start` seam in `agents-plugin-pi/src/index.ts` with a new
exported helper `bootstrapOrFailLoud<T>(ui, role, bootstrap, exitProcess?)` that
wraps the `startBridge` -> `createApprovalRelay` -> `registerAgentTools`
sequence: on any throw it notifies loudly, and for a spawned child
(`readSpawnRole(process.env) !== undefined`, i.e. `worker`/`explore`/`fork`)
calls `process.exit(1)` so the parent's existing `RpcClient` exit-rejection
surfaces a real spawn/dispatch error — no new IPC/readiness protocol. The
interactive host lead (`role === undefined`, no RPC parent) gets the loud
notify plus an early `return` rather than a process crash. `if
(!sessionBootstrap) return;` short-circuits the rest of the handler so no
partial/toolless registration path remains. Local binding renamed
`bootstrap` -> `sessionBootstrap` to avoid colliding with the existing
`computeSessionBootstrap` result (cosmetic). No `TOOL_GROUPS`/`resolveTools`,
ws-mcp, or shared-rsrc changes; no tool group widened.

New `agents-plugin-pi/test/session-bootstrap-guard.test.ts` (5 cases) drives the
**real** `startBridge` against a broken fake launcher (`sys.exit(1)`, no
JSON-RPC) so `client.initialize()` genuinely rejects — not a mocked allowlist:
spawned-child exit+notify (exitSpy once with 1), host-lead notify-without-exit,
happy-path passthrough, and healthy simple (no `explore`/`bash`) vs deep
(`explore` present, no `bash`) tool-surface parity.

Verification: targeted `env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE node
--test` over the new file + `native-tool-registration` + `persistent-explore`
(+ `execute-gateway`) green across clean / `WS_PI_SPAWN_ROLE=worker` /
`explore+deep` env runs; full `npm test` failing-test-name set byte-identical to
the documented ~130-failure baseline before vs after in every env => zero
regressions. Fails-before/passes-after confirmed by differential (reverting
`index.ts` removes the `bootstrapOrFailLoud` export; a reproduction of the
pre-guard unguarded shape fires neither notify nor exit). Partitioned review:
correctness clean (1 Minor), fit clean, test clean (2 Minor) — no
Critical/Important. Minors recorded, not acted on: (1) a post-`startBridge`
`registerAgentTools` throw on the host-lead path leaves the started bridge
unshut (pre-existing leak pattern, undocumented failure mode, optional
`h.shutdown()`); (2) exit branch asserted only for `role="explore"`, not
per-role; (3) test temp dirs not cleaned (matches existing convention).

Phase 2 remains blocked on owner-run live dogfood (see `## Blocked
(2026-09-09)` below); this structural guard does not close Phase 2.

### Phase 2: confirm the live trigger and restore deep collection if a defect remains

_Status: blocked — pending live dogfood evidence (see `## Blocked (2026-09-09)`)._

Blocked until a live deep researcher failure is reproduced with the Phase 1
guard in place, so the child's **actual** launch error is captured instead of
inferred. The source trace found the deep-group registration chain correct
(`TOOL_GROUPS`, `WS_PI_EXPLORE_MODE` propagation, the role/mode gate), so the
detailed proximate cause is unresolved: candidates are the stale-bootstrap
launcher failure (`260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env`),
a live-vs-tested engine build difference (`RPC_CLI_PATH` re-execs the lead's
`pi`), and a fresh-spawn-vs-dormant-resume difference. If the captured error
shows the trigger is owned by another ticket (most likely the stale-bootstrap
bug), this ticket's remaining work is to delegate there and verify the deep
surface once launch succeeds — there may be no deep-group defect to fix. If a
genuine deep-group registration defect remains after launch succeeds, restore
the approved contract here.

Verification (live, owner-run; not satisfiable by mocked tests or a child's
self-report of unobservable model state):

- Reproduce a deep researcher after a full Pi process exit/reopen of the same
  session; with the Phase 1 guard, capture the child's actual launch error
  and the concrete parent/child executable, extension source path, launch
  allowlist, and deep role/mode environment. Inspect only relevant
  environment keys; do not dump credentials or whole environments.
- Once launch succeeds, invoke deep blocking collection against a small
  read-only repository question: observe the collection leaf's actual tool
  surface (no bash, no recursive explore), small model and effort, and
  confirm its result reaches the researcher for synthesis. Repeat after
  stop/send and full exit/reopen; identity, mode, permissions and frozen
  model/effort retained.
- Record findings back on the prerequisite feature ticket
  (`260907-feat-ws-pi-persistent-explore-deep-research`); its mandatory live
  acceptance remains incomplete until supported by evidence, and this bug's
  capture does not mark it done.

## Blocked (2026-09-09)

Blocked on a human/owner-run gate, not on remaining agent work. Phase 1 (the
structural fail-loud guard that stands regardless of the exact trigger) is
complete, reviewed clean, and recorded above (`### Result (55172110)`). The only
remaining work is **Phase 2**, which the ticket itself scopes as blocked
"pending live dogfood evidence": reproducing a live deep-researcher failure
after a full Pi process exit/reopen — with the Phase 1 guard now in place to
capture the child's actual launch error — is owner-run (a live Pi session,
subscription/runtime login) and not satisfiable by mocked tests or a child's
self-report. Only once that evidence lands can Phase 2 either delegate to the
stale-bootstrap trigger ticket
(`260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env`) or, if a
genuine deep-group registration defect remains, restore the approved contract.

Ticket stays in `ready/` (Phase 2 incomplete, so not moved to `.done/`) and must
not be re-dispatched for implementation until the owner captures the live
evidence. Clear this note when that evidence is recorded on the prerequisite
feature ticket (`260907-feat-ws-pi-persistent-explore-deep-research`) and Phase 2
becomes actionable.
