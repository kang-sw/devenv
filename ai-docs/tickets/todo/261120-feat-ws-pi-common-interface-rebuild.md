---
title: "Pi 확장 철거와 공통 인터페이스 기반 백엔드 재구축"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-prompt-assembly: "Decision 27 leaves the existing Pi prompt hooks in place until this late-stage rebuild"
  261118-research-ws-pi-hosted-connector: "Pi hosted connector evidence for the replacement adapter"
---

# Rebuild Pi on the common hosted interface

## Background

Owner directive (2026-11-20, verbatim): "pi 확장 철거 + pi를 바닥부터 짜서 이번 공통 인터페이스에 맞추는 티켓을 에픽 후반부 페이즈에 넣는 걸로 합시다. 즉, pi-ws-extension 의식하지 않고 처음부터 짜는 방식으로, claude-codex-agy에서 공통 인터페이스 완성하고 해당 인터페이스 상부를 건드리지 않고 깔끔하게 pi를 끼워넣는 걸 목표로."

This is the epic's late-stage Pi child. The existing native Pi extension is
historical behavior and migration evidence, not the implementation template.

## Decisions

1. **Rebuild Pi from scratch as a backend adapter of the common hosted
   interface (owner, 2026-11-20).** Do not port or preserve the structure of
   the existing `pi-ws-extension` (`agents-plugin-pi` native extension) as the
   new adapter. Rejected: treating the extension as the adapter architecture.
2. **Sequence Pi after the common interface is complete for Claude, Codex, and
   agy (owner, 2026-11-20).** This child is a late epic stage, not a parallel
   redesign of the shared interface. Rejected: implementing Pi before that
   interface has been proven by the other three hosted backends.
3. **Pi plugs in without changing the interface's upper layer (owner,
   2026-11-20).** The acceptance boundary includes the daemon application
   layer, MCP tools, and hosted prompt assembly. Rejected: Pi-specific changes
   to those layers to make the adapter fit.
4. **Retire the existing Pi extension (owner, 2026-11-20).** The replacement
   hosted Pi backend takes its place; retirement timing and installed-user
   compatibility remain open below. Rejected: retaining the extension as a
   permanent parallel hosted path.

## Open Decision Queue

Last ID: (1923)

(1920) [open] [policy] When does the existing Pi extension retire relative to
replacement rollout and downstream installed Pi users?

- a. Keep the old extension available until the new hosted Pi backend passes
  parity and a documented transition gate; retire it in that release.
- b. Remove the extension as soon as the new adapter builds, before parity and
  downstream transition are verified.

> recommendation: a - it makes retirement contingent on a working replacement
> while honoring the owner's directive to remove the extension.

(1921) [open] [compatibility] What tier and migration path apply to removing
the native extension and its packaged entry points?

- a. Inventory the installed surfaces and old/new reader behavior, then retain
  a compatibility shim where T0-T2 is possible; request explicit T3 approval
  only for a demonstrated unavoidable old-version boundary.
- b. Remove all extension surfaces in one release and accept a T3 boundary.

> recommendation: a - the current directive settles retirement, not the
> compatibility tier or old-version behavior.

(1922) [open] [policy] What happens to the Decision 21 Pi hook exceptions in
`261117-feat-ws-hosted-prompt-assembly` once the extension retires?

- a. Define the replacement hosted Pi behavior from the common interface and
  retire the exceptions with the extension, preserving only explicitly chosen
  user-visible capabilities.
- b. Recreate every extension hook as Pi-specific daemon or adapter behavior.

> recommendation: a - copying hook internals would undermine the common
> interface boundary, but user-visible capability decisions need explicit
> settlement before implementation.

(1923) [open] [policy] Which Pi goal-loop and model-prompts capabilities must
the replacement hosted Pi backend preserve?

- a. Inventory observable behavior and settle parity per capability before
  implementation; defer features that are not part of the common hosted
  contract explicitly.
- b. Require exact feature parity with all native extension hooks.
- c. Drop both capabilities without a parity review.

> recommendation: a - the owner chose a clean adapter rebuild but did not
> decide the fate of every extension feature.

## Phases

### Phase 1: Settle replacement contract and retirement gate

After Claude, Codex, and agy complete the common hosted interface, resolve
(1920)-(1923), inventory the existing Pi package and downstream transition
surfaces, and define a checkable hosted Pi parity and compatibility gate.

### Phase 2: Implement the Pi backend and retire the extension

Build the Pi adapter against the settled common interface. Verify hosted lead
and worker behavior through the shared MCP, daemon application, and prompt
assembly contracts without changing their upper layers. Retire the native
extension through the Phase 1 gate and record the migration result.
