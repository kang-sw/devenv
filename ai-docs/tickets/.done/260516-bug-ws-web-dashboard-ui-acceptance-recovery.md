---
title: ws web dashboard UI acceptance recovery
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-epic-ws-web-dashboard-workroot-io-substrate: completed substrate whose UI acceptance remains merge-blocked
  260516-bug-ws-web-dashboard-live-resource-api-connection: live resource connection branch where the user-visible regressions were observed
  260516-feat-ws-web-terminal-session-substrate: terminal substrate that needs browser-terminal acceptance recovery
  260516-feat-ws-web-workroot-file-navigation: file explorer substrate that needs conventional navigation recovery
spec:
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
  - 260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
  - 260516-ws-web-dashboard-file-explorer-conventional-affordance
  - 260516-ws-web-dashboard-browser-workroot-io-dogfood-evidence
completed: 2026-05-16
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
- Prefer an explicit empty workbench plus a clear `New Terminal` affordance over
  auto-creating a terminal when a workRoot opens. Auto terminal creation is a
  daemon process lifecycle side effect and should not be introduced merely to
  hide placeholder UI.
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
- Browser acceptance must run against the daemon-served production frontend
  after owner pairing. Vite-only component or dev-server checks are not enough
  for this recovery because the observed failures involve the real daemon
  resource, workbench, and terminal flow.
- Screenshots or traces may be generated as verification artifacts, but do not
  commit bulky generated images by default. Record paths, viewport sizes, and
  pass/fail observations in the dogfood artifact unless a later convention says
  otherwise.
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

The gate should expose a runnable command such as `npm run test:browser`,
`npm run test:e2e`, or the closest project-local equivalent. It should cover
pairing, opening a real workRoot, file explorer interaction, terminal creation,
terminal tab switching, terminal input/output/rendering, and terminal pane fill
against the daemon-served app. Start by proving the current branch fails at
least the known reported behaviors or recording why fail-first evidence cannot
be captured.

Success means the implementation can no longer close a UI-facing dashboard
ticket based only on Rust route tests, pure TypeScript tests, Vite build, and
curl evidence.

### Result (e6982b5) - 2026-05-16

Added a Playwright browser acceptance gate exposed as `npm run test:browser`.
The gate builds the production frontend, starts the dashboard daemon on an
ephemeral port, pairs through the owner pairing URL, opens a real temporary
workRoot, and records textual evidence plus gitignored screenshots. Review
cycle fixes strengthened the gate so terminal tab selection settles past the
500ms poll cycle, terminal pane fill is compared against its actual container,
bounded resize is asserted, and pairing URL scraping is newline-safe.

### Phase 2: Recover terminal tab and initial terminal state

Fix workbench terminal tabs so every visible terminal tab label is selectable
and focuses the corresponding terminal pane. Opening a real workRoot must not
leave a mock or placeholder terminal as the initial visible terminal surface.
If the default workRoot view should start empty instead of showing a terminal,
make that state explicit and non-mock.

Success means browser evidence shows multiple terminal tabs can be selected,
the focused pane changes correctly, terminal input/output does not cross between
sessions, and no mock terminal remains visible after opening a real workRoot.

### Result (e6982b5) - 2026-05-16

Removed the unconditional bodyless `persistent-terminal` placeholder pane and
kept the empty workbench state explicit with the existing `New terminal`
affordance. Terminal focus requests now use a sequence guard so output-poll
reconciliation no longer steals focus back from a user-selected terminal tab.
Browser evidence verifies multiple terminal tabs, per-session input/output
isolation, close-as-terminate, and reload reconstruction without mock surfaces.

### Phase 3: Recover terminal rendering, input, and sizing

Make the browser terminal behave like a real terminal surface. ANSI color and
control sequences must be interpreted by the terminal emulator rather than
displayed raw. Keyboard input must flow through the focused terminal surface to
the daemon PTY. The terminal must fill its workbench panel and fit or resize
within the pane without leaving unusable dead space.

First verify whether the current pane is actually using xterm.js correctly. If
not, rebuild the pane around the established xterm package pattern, such as
`@xterm/xterm` plus a fit addon or the project-local equivalent. PTY output
must be written into the terminal emulator (`terminal.write(...)` or equivalent)
rather than rendered as raw text nodes. Browser input must come from the
terminal emulator's data/input callback (`onData(...)` or equivalent) and flow
to the daemon input route. Pane sizing should use a real container measurement
path such as `ResizeObserver` plus fit-addon behavior, and any daemon resize
updates must remain bounded rather than continuously rewriting logical PTY size
during visual drag.

CSS/layout work is part of this phase: the terminal container, xterm viewport,
and workbench body must use stable flex or grid sizing with `min-height: 0`,
`height: 100%`, or equivalent rules so the terminal fills the available pane.

Success means browser evidence shows a real shell command with colored/control
output rendering correctly, typed input reaching the PTY, output returning to
the same xterm surface, and the terminal occupying the available pane body.

### Result (e6982b5) - 2026-05-16

Replaced raw `<pre>` output and line-buffered input with an `@xterm/xterm`
terminal surface. PTY output deltas stream into the emulator, input flows from
the emulator to the daemon input route, and each pane owns an isolated emulator
instance. `FitAddon` plus `ResizeObserver` makes the surface fill the pane while
clamping to the daemon PTY bounds before resize forwarding. Review added direct
unit coverage for the clamp boundary behavior.

### Phase 4: Recover file explorer affordance

Revise the workRoot file explorer into a conventional, inspectable tree/list
surface. It should clearly distinguish directories and files, expose expansion
and refresh behavior through familiar controls, keep the selected workRoot
identity visible, and avoid surprising custom interaction patterns. It must
remain read-only and must not become a broad file manager.

Success means browser evidence shows a user can understand how to expand a
directory, refresh, and open a previewable file without relying on hidden or
nonstandard affordances.

### Result (e6982b5) - 2026-05-16

Reworked the workRoot file explorer rows into conventional read-only tree/list
controls. Directories and files are visually distinct, directory rows toggle
through familiar disclosure affordances, previewable files open read-only panes,
and existing command ids for select, expand, open, and refresh remain present.
Browser evidence covers expansion, refresh, selection, and read-only preview.

### Phase 5: Sonnet design verification and autonomous tweak pass

Run the workflow override after the primary implementation for Phases 1-4 and
before the standard review cycle. The Sonnet named agent must use the
browser-level acceptance gate to verify the frontend design, make bounded
tweaks when needed, and commit one logical checkpoint before normal reviewers
assess the final implementation.

Success means the Sonnet pass reports browser evidence for the dashboard flow,
lists any tweaks it made, and leaves no unresolved design blocker that would
make ordinary correctness/fit/test review premature.

### Result (e6982b5) - 2026-05-16

Ran the required ws named-agent Sonnet design verification pass before normal
review. The pass used `npm run test:browser`, inspected the generated desktop
and narrow screenshots, and committed a bounded visual fix changing the active
tab underline from an undefined CSS token to the dashboard action color. It
reported no unresolved design blocker; remaining notes were out-of-scope
pre-existing toolbar/agent placeholder behaviors.

### Phase 6: Product-flow dogfood and merge decision

Run a daemon-served browser dogfood from first load through opening a real
workRoot, browsing files, opening a read-only file, creating and switching
between terminals, verifying terminal rendering/input/fill behavior, and
refreshing without reintroducing mock surfaces.

The dogfood must include owner pairing and the same production frontend path
used by the browser acceptance gate. It should record the command used to start
the daemon, the browser automation or manual browser steps, viewport sizes,
terminal commands used to verify color/control handling, and whether screenshot
or trace files were generated.

Success means the dogfood artifact includes browser-level evidence, the known
user-reported failures are explicitly checked off, and the branch can be
reconsidered for merge only after those checks pass.

### Result (e6982b5) - 2026-05-16

Recorded browser-level dogfood evidence in
`ai-docs/.plans/2026-05/16-260516-bug-ws-web-dashboard-ui-acceptance-recovery.dogfood.md`.
The artifact includes the daemon-served command path, owner pairing flow,
desktop and narrow viewports, terminal commands used to verify ANSI/color and
session isolation, screenshot artifact paths, and a pass table for every
user-reported failure. Generated screenshots and Playwright artifacts remain
gitignored and regenerable.
