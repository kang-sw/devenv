---
title: The e2e build step lives only in the test:browser npm script, so a direct npx playwright test bypasses it and serves a stale bundle
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 7 ran bare npx playwright test as evidence throughout, including reviewers; one mutation run passed against the pre-mutation bundle
related-mental-model:
  - ws-web-dashboard
---

# The e2e build step lives only in the test:browser npm script, so a direct npx playwright test bypasses it and serves a stale bundle

## Background

Found on 2026-07-26. The dashboard e2e suite does have a build step, but it
sits in exactly one place, and the obvious shortcut around it is unguarded.

Verified in `ws-dashboard/frontend`:

- `package.json:25` — `test:browser` is
  `npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test`.
  This is the safe entry point and it works.
- No npm `pretest`/`test` hook exists. (`preview` is a standalone script, not a
  hook — there is no `view` script for it to attach to.)
- `playwright.config.ts` declares no `webServer`, `globalSetup`, or
  `globalTeardown`.
- Nothing under `e2e/` invokes a build. `daemonHarness.ts:216-217`'s
  `startDaemon` just points the daemon at `frontend/dist` via `--static-dir`
  unless `WS_DASHBOARD_STATIC_DIR` overrides it.

So the protection is carried entirely by which command you happen to type.
`npx playwright test <spec>` — the natural move when iterating on a single spec
file, and what was actually used throughout Phase 7 of
`260725-feat-dashboard-pty-agent-attention-notification`, reviewers included —
runs the browser against whatever `dist/` already sits on disk.

The sharp edge is not "the harness never builds." It is that a footgun which
only fires when you take the obvious shortcut is arguably worse than no
protection at all: the presence of `test:browser` makes the suite *look*
build-safe, so nobody re-derives the risk when they drop to a direct
invocation.

The concrete damage is to mutation-based non-vacuity evidence. Mutate
`frontend/src` to prove an assertion catches a regression, run Playwright
directly, and the browser serves the pre-mutation bundle — the run passes, and
a passing assertion and a wrong build look identical from the terminal. Phase 7
hit this: an implementer's first mutation run passed for this reason, and
because the mismatch leaves no trace in output, the bare `npx playwright test`
runs used as evidence throughout that phase cannot be re-established after the
fact.

## Scope

Only the daemon-served production bundle goes stale. Playwright's own runner
reads `*.spec.ts` fresh off disk on every invocation, so edits inside the e2e
test files are never affected. A fix should not be scoped wider than the
`dist/`-vs-`src/` relationship.

## Current mitigation

Procedural and already recorded: `ai-docs/mental-model/ws-web-dashboard/index.md`
carries an entry requiring `npm run build` after any `frontend/src` mutation
used as evidence. This ticket exists because the discipline is unenforced on
the bypass path, not because the hazard is unaddressed.

## Possible directions (none chosen)

Capture only — the tradeoff is deliberately left open:

- A harness-side staleness check: compare `dist/` mtime against `src/` and warn
  or fail loudly. Aimed most directly at the actual failure, since it guards the
  bypass path itself without adding a build to any run.
- A `webServer` entry in `playwright.config.ts` that builds before serving.
- A `pretest`-style npm hook ahead of the Playwright invocation.

The latter two partly duplicate what `test:browser` already does, and they
charge their build cost precisely on the fast single-spec iteration path that
the bypass exists to serve — so they trade the footgun for a slower inner loop
rather than closing a gap. That asymmetry is why the choice is left open here
rather than settled: detect-and-refuse, build unconditionally, or build
conditionally is the open question.
