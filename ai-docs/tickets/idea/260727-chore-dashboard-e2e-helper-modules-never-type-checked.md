---
title: e2e helper modules and spec files are type-checked by nothing
related:
  260726-chore-dashboard-verify-notification-permission-tier-manually: surfaced by the correctness review of Phase 1
---

# e2e helper modules and spec files are type-checked by nothing

## Background

`ws-dashboard/frontend/tsconfig.e2e-tests.json` has an `include` of exactly two
entries:

```json
"include": ["e2e/daemonHarness.ts", "e2e/daemonHarness.test.ts"]
```

Everything else under `e2e/` is outside every TypeScript program in the repo.
`tsc -b` (the `build` script) covers `src/`. `tsconfig.route-tests.json` covers
the route tests. Playwright transpiles `*.spec.ts` to run them but does not
type-check them. So the `*.spec.ts` files and any helper module that is not
`daemonHarness.ts` are checked by no script in `package.json`.

This surfaced during the correctness review of
`260726-chore-dashboard-verify-notification-permission-tier-manually` Phase 1,
which extracted `readCallbackToken` and `postTurnState` into a new
`e2e/agentTurnState.ts` shared by two spec files. That module is imported by
two callers and validated by neither compiler nor linter.

## Why it matters

A signature change to a shared e2e helper surfaces only as a runtime failure,
inside a Playwright run, far from the edit. `postTurnState` posts and checks for
a 204; a wrong argument order shows up as a non-204 or an `ENOENT` several
hundred lines from the function, in a browser test that takes minutes to reach
that point. The type system would have caught it in a second.

The exposure grows with every helper that gets extracted, and the current
direction is toward more sharing, not less: Phase 1's own ticket text told the
implementer to lift helpers into a shared module rather than copy-paste them.

## First Step

The cheap, obviously-safe half first: add `e2e/agentTurnState.ts` to
`tsconfig.e2e-tests.json`'s `include`. `npm run test:terminals` already runs
`tsc -p tsconfig.e2e-tests.json`, so this costs nothing new and the module gets
checked on every terminal-test run.

Then decide the bigger question deliberately rather than by default: should the
`*.spec.ts` files be type-checked too? Adding `e2e/**/*.ts` to the include is
one line, but it will surface whatever has accumulated in files that have never
been checked, and Playwright's own types must resolve under `module: NodeNext`.
Find out how large that is before committing to it — if it is small, take it; if
it is large, take the helper modules only and record the spec files as a
knowingly-accepted gap rather than leaving it undiscovered.

## Notes

- Filed as `idea/` because it is a real gap with an unmeasured tail, not because
  the first step is uncertain. The first step is a one-line change; it is the
  second that needs a number before anyone commits to it.
- Not a defect in the Phase 1 work that surfaced it. That phase's plan
  explicitly declared the tsconfig addition optional for its scope, and the
  review recorded the consequence rather than treating it as a deviation.
- Worth checking in the same pass whether any lint config covers `e2e/`. If
  neither the compiler nor the linter reaches these files, that is the finding
  to lead with.
