---
title: ws web dashboard UI acceptance recovery
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-epic-ws-web-dashboard-workroot-io-substrate: completed substrate whose UI acceptance remains merge-blocked
  260516-bug-ws-web-dashboard-live-resource-api-connection: live resource connection branch where the user-visible regressions were observed
  260516-feat-ws-web-terminal-session-substrate: terminal substrate that needs browser-terminal acceptance recovery
  260516-feat-ws-web-workroot-file-navigation: file explorer substrate that needs conventional navigation recovery
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard UI acceptance recovery

## Background

The workRoot IO branch connected real opened workRoots to the dashboard resource
model and exercised backend/file/terminal APIs, but user dogfood exposed
product-level UI failures that code and API tests did not catch. This is a
merge blocker for the dashboard branch, not a cosmetic follow-up.

Observed failures:

- Terminal tab labels respond only for the active terminal; inactive terminal
  tabs do not behave like selectable tabs.
- Terminals created through `new terminal` show a live terminal, but the
  terminal visible when a workRoot is first opened still appears to be mock or
  placeholder state.
- The file explorer UI is non-obvious and does not match familiar tree/list
  navigation expectations.
- The terminal surface does not behave like a real xterm.js terminal: ANSI
  color/control sequences appear raw, stdio/input feels disconnected, and the
  prompt appears to be a separate non-terminal prompt.
- The terminal does not fill its panel.
- Opening a workRoot through raw path typing is accepted as a temporary
  compromise for this recovery ticket; a richer picker/browser UI may remain
  deferred.

Current evidence shows that visual/browser acceptance testing is not a real
automated gate. The frontend package has TypeScript/node pure tests and build
checks, but no Playwright, Puppeteer, Vitest browser, screenshot, or visual
test script. Existing dogfood artifacts record interactive browser/screenshot
tooling as unavailable and use HTTP or browser-equivalent evidence instead.

## Decisions

- Treat this ticket as a product-flow recovery pass before merging the current
  workRoot IO implementation branch.
- Do not count API/curl dogfood as sufficient evidence for UI completion.
- Keep the raw path-input opener acceptable for now. Do not spend this ticket
  on a full filesystem picker unless it is necessary to verify the recovered
  flow.
- Terminal acceptance means browser-terminal behavior, not log-view behavior.
  xterm.js must receive terminal byte output in a way that renders ANSI/control
  sequences correctly, accepts input through the terminal surface, and fits the
  pane.
- Mock or placeholder surfaces must not be visible in the production default
  workRoot flow once a real workRoot is opened.
- This ticket uses an exceptional workflow: after the main implementation pass
  and before the normal correctness/fit/test review cycle, run one ws named
  agent pass on a Sonnet model for frontend design verification and bounded
  autonomous tweaks.

## Constraints

- Preserve the existing daemon-owned terminal lifecycle: create/list/output/
  input/resize/close remain daemon APIs, and explicit close terminates the
  session.
- Preserve opened workRoot resource identity and host-path privacy. Do not make
  browser routes or raw paths authoritative over resource ids.
- Add browser-level acceptance evidence. If a persistent visual test framework
  is not added, the ticket must record manual browser evidence with exact
  viewport, steps, and pass/fail notes; pure helper tests alone are not enough.
- Keep UI fixes scoped to workRoot file/terminal usability. Do not add agent
  presets, broad file-manager verbs, write-back editing, or named-agent
  controls here.

## Workflow Override

After the primary implementation pass completes and before the standard review
partitions run, register a ws named agent on the Sonnet model for a single
frontend design verification and tweak pass. This is an intentional exception
to the usual implement-then-review relay order for this ticket.

The Sonnet pass should:

- Inspect the daemon-served browser flow using the browser/visual acceptance
  gate from Phase 1.
- Judge the visible frontend experience against the user-reported failures and
  the dashboard visual-system/domain rules.
- Make bounded UI tweaks directly when they are necessary to pass the browser
  acceptance criteria.
- Commit its changes as one logical checkpoint before the normal
  correctness/fit/test reviewers run.
- Report exact evidence, changed files, and any design concerns that remain.

The pass must not expand scope into a full root picker, agent presets,
write-back editing, broad file-manager verbs, or unrelated visual redesign. If
the Sonnet pass finds a product-direction question that cannot be resolved by
the ticket's acceptance criteria, it should stop and report that blocker rather
than inventing a new interaction model.

## Phases

### Phase 1: Establish browser acceptance gate

Add a minimal browser-level verification path for the dashboard workRoot flow.
The gate must be capable of catching the reported issues: inactive terminal tab
click behavior, visible mock/placeholder terminals after opening a real
workRoot, terminal render/input/fill behavior, and file explorer affordance
regressions.

Preferred approach: add a lightweight Playwright-based check or equivalent
browser automation that can run against the daemon-served production frontend.
If persistent tooling is deferred, record manual browser evidence in a dogfood
artifact and make the lack of automation an explicit blocker for future UI
completion claims.

Success means the implementation can no longer close a UI-facing dashboard
ticket based only on Rust route tests, pure TypeScript tests, Vite build, and
curl evidence.

### Phase 2: Recover terminal tab and initial terminal state

Fix workbench terminal tabs so every visible terminal tab label is selectable
and focuses the corresponding terminal pane. Opening a real workRoot must not
leave a mock or placeholder terminal as the initial visible terminal surface.
If the default workRoot view should start empty instead of showing a terminal,
make that state explicit and non-mock.

Success means browser evidence shows multiple terminal tabs can be selected,
the focused pane changes correctly, and no mock terminal remains visible after
opening a real workRoot.

### Phase 3: Recover terminal rendering, input, and sizing

Make the browser terminal behave like a real terminal surface. ANSI color and
control sequences must be interpreted by the terminal emulator rather than
displayed raw. Keyboard input must flow through the focused terminal surface to
the daemon PTY. The terminal must fill its workbench panel and fit or resize
within the pane without leaving unusable dead space.

Success means browser evidence shows a real shell command with colored/control
output rendering correctly, typed input reaching the PTY, output returning to
the same xterm surface, and the terminal occupying the available pane body.

### Phase 4: Recover file explorer affordance

Revise the workRoot file explorer into a conventional, inspectable tree/list
surface. It should clearly distinguish directories and files, expose expansion
and refresh behavior through familiar controls, keep the selected workRoot
identity visible, and avoid surprising custom interaction patterns. It must
remain read-only and must not become a broad file manager.

Success means browser evidence shows a user can understand how to expand a
directory, refresh, and open a previewable file without relying on hidden or
nonstandard affordances.

### Phase 5: Sonnet design verification and autonomous tweak pass

Run the workflow override after the primary implementation for Phases 1-4 and
before the standard review cycle. The Sonnet named agent must use the
browser-level acceptance gate to verify the frontend design, make bounded
tweaks when needed, and commit one logical checkpoint before normal reviewers
assess the final implementation.

Success means the Sonnet pass reports browser evidence for the dashboard flow,
lists any tweaks it made, and leaves no unresolved design blocker that would
make ordinary correctness/fit/test review premature.

### Phase 6: Product-flow dogfood and merge decision

Run a daemon-served browser dogfood from first load through opening a real
workRoot, browsing files, opening a read-only file, creating and switching
between terminals, verifying terminal rendering/input/fill behavior, and
refreshing without reintroducing mock surfaces.

Success means the dogfood artifact includes browser-level evidence, the known
user-reported failures are explicitly checked off, and the branch can be
reconsidered for merge only after those checks pass.
