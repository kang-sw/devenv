# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 8: browser-level notification

## Relevant Ticket Contract

- Phase 8 text (`ai-docs/tickets/ready/260725-feat-dashboard-pty-agent-attention-notification.md:1484-1500`):
  Tier 1 (default, zero-permission) is `document.title` flashing plus a favicon
  badge — the only tier that works over plain-http LAN access (page is not a
  secure context there, so `Notification` is unavailable). Tier 2 is
  `Notification` as an explicit opt-in requested from a user gesture (a
  Settings toggle in `settingsSections.tsx`), never on load, with Settings
  copy stating plainly that OS-level notification requires localhost or TLS.
- Verification boundary (same section): Tier 1 asserted in browser
  acceptance (Playwright); Tier 2 verified **manually** and recorded — do not
  build harness automation to drive a real permission prompt.
- BINDING carry-forward from Phase 7's Result forward-note
  (`:1475-1479`): the notification must derive from the **same**
  `pendingAttentionStateFor` predicate rather than adding its own
  acknowledgement watermark, "for the reason recorded above — a second
  watermark is what the acknowledgement pin exists to prevent, and a
  notification with independent state would disagree with both the tab
  badge and the nav badge."
- SETTLED (do not re-open) — Phase 7's Result also left open "whether [the
  hidden-worktree nav silence] should extend to an OS-level notification"
  (`:1480-1482`). The lead has settled this: yes, a hidden root is silent
  everywhere, consistent with the `App.tsx:7596` `CONTRACT:` comment that the
  nav-attention id set and the rendered child-row list must mirror each
  other.
- Phase 7's pinned rule the flash mechanism must extend (`:1391-1394`): "The
  flash is level-driven off a data attribute... An edge-triggered flash with
  its own timer would BE a second acknowledgement watermark, which
  `## Constraints` pins against." Phase 8's title/favicon cue must be the
  same shape: level-driven off the derived tone, not an independently timed
  or persisted state.
- Deferred scope (`:287-289`): Web Push / VAPID / service-worker push is
  explicitly out of scope; the existing `sw.js` stays an 11-line
  installability stub. Do not touch it.
- Constraints (`:245-259`): any new CSS-driven flash affecting
  `.resource-row` must be an overlay layer, not a `background` write — not
  directly applicable here (title/favicon are not CSS), noted for context
  only.
- Spec Impact (`:343-344`): Phase 8 owns the
  `#260722-ws-dashboard-settings-panel` spec entry — add a
  notification-permission section entry alongside the existing Terminal
  section description (`ai-docs/spec/ws-web-dashboard/index.md:906-955`).
- Evidence rule (discovered Phase 7, mental model `## Common Mistakes`
  `ai-docs/mental-model/ws-web-dashboard/index.md:247`): Playwright serves
  the prebuilt `frontend/dist` — `playwright.config.ts` has no `webServer`/
  `globalSetup`, and `e2e/daemonHarness.ts:216-217` points `--static-dir` at
  it. Only `test:browser` (`frontend/package.json:25`) chains `npm run
  build` ahead of `playwright test`. Any `frontend/src` mutation used as
  non-vacuity evidence must be followed by `npm run build` first.
- Known-unrelated failures, judge by site not exit code:
  `dashboard-acceptance.spec.ts:3779` (fitNow short-viewport,
  `260725-bug-dashboard-fitnow-short-viewport-shrink`) with `:4020` skipping
  behind it in serial mode; `cargo test --test routes` has the same
  pre-existing pair at `routes.rs:1066`/`routes.rs:1383`.

## Out of Scope

- Phases 1-7 (all merged) — not touched.
- Web Push / service-worker push delivery while the tab is closed (Deferred
  scope, `:287-289`).
- Any daemon-side change. Phase 8 is 100% frontend: it consumes the
  Phase 5 stream's already-derived browser state (`attentionByKey`,
  `agentAttentionByRoot`), it does not add a route or touch
  `agent_attention.rs`.
- Driving a real OS permission-grant prompt inside Playwright. Playwright's
  `context.grantPermissions`/`clearPermissions` APIs exist and could force a
  `granted` state without a real native dialog, but the ticket's verification
  boundary explicitly prices a real prompt as "not worth its cost" — using
  the grant API would still only prove the app calls `Notification`
  correctly, not that the real browser permission flow behaves as expected,
  and the ticket already assigns that half to manual+recorded. Not proposed;
  flagging only so the choice isn't silently reopened later.
- A distinct favicon/title treatment per tone (`working` vs `ready`). See
  Implementation Plan step 3 for the single-badge decision and its rationale.
- Extending `sw.js` or `manifest.webmanifest` icons.

## Codebase Findings

- `frontend/src/agentAttention.ts:99-108` — `pendingAttentionStateFor(entry,
  acknowledgedUpdatedAtMs, sessionStatus)` is the one predicate every tier
  must read through, per the binding carry-forward. It already folds in
  liveness (`sessionStatus === "running"`), the `idle` no-badge rule, and the
  ack watermark — Phase 8 must not add a second watermark alongside it.
- `frontend/src/agentAttention.ts:201-226` — `aggregateNavAttentionCounts`
  iterates **panes** (not `attentionByKey` directly) and classifies each
  through `pendingAttentionStateFor`, producing `Record<rootKey,
  NavAttentionCounts>`. This is already computed at `App()` level and stored
  as `agentAttentionByRoot` state (`App.tsx:574` declaration, refreshed via
  the signature-gated effect at `App.tsx:4356-4379`).
- `frontend/src/agentAttention.ts:249-264` — `aggregateNavAttentionTone(countsByRoot,
  rootKeys)` returns the pinned `ready > working > none` priority over a
  caller-supplied list of root keys. `App.tsx:3189-3199` (inside `ServerRows`)
  already calls this per-server, feeding it a **filtered** root-key list built
  from `navAttentionWorkRootIds(workspace, hiddenIds)` — this is the exact,
  already-filtered mechanism requirement 4 asks for. Global browser-level tone
  should reuse `aggregateNavAttentionTone` fed with the UNION of that same
  filtered root-key list across every server in `resourcesByServer`, not a
  new predicate.
- `frontend/src/App.tsx:7610-7626` — `navAttentionWorkRootIds(workspace,
  hiddenIds)` is module-scope but **not exported** (same-file use only, at
  `App.tsx:3192-3197`). It already excludes hidden worktrees via
  `applyHiddenWorktrees` and includes the workspace-presentation base root
  per Phase 7's review-cycle-2 fix. Because it is unexported, the global
  tone derivation must live inside `App.tsx` itself (not in `agentAttention.ts`),
  reusing this function directly — this also avoids the barrel back-import
  trap below.
- `frontend/src/App.tsx:7596-7601` — the `CONTRACT:` comment pins
  `navAttentionWorkRootIds`'s output set to exactly match what `WorkspaceRows`
  renders with an `agentCounts` prop. The global browser-level tone must walk
  the *same* set (all servers' workspaces, same hidden-id lookup via
  `workNavOrder.hiddenWorktreesByWorkspace`), so a hidden worktree's agent
  contributes to neither the nav nor the notification — this is the concrete
  mechanism for the lead-settled hidden-worktree decision.
- `frontend/src/App.tsx:484` — `resourcesByServer: Record<string,
  DashboardResourcesView>` (all connected servers) is already `App()`-level
  state; iterating its `.workspaces` the same way `ServerRows` does per-server
  is available with no new plumbing.
- `frontend/index.html:6,16` — `<title>ws dashboard</title>` and
  `<link rel="icon" type="image/png" href="/icon-192.png" />`. Confirmed via
  `grep -rn "document.title" frontend/src` (no hits): **no existing
  `document.title` writer exists anywhere in the frontend**, so a new title-flash
  effect has no other writer to coordinate with or clobber.
- `frontend/src/main.tsx:16-19` — the only existing "act automatically on
  page load" DOM precedent (`sw.js` registration). Contrast case: Tier 2 must
  NOT mirror this — `Notification.requestPermission()` may only be called
  from the Settings toggle's own click/change handler, never from a
  mount-time effect.
- `frontend/src/settingsSections.tsx` (whole file, 92 lines) — the section
  registry pattern to replicate: a settings-scoped React Context
  (`SettingsTerminalContext`) carrying `{ prefs, onChange }`, a zero-prop
  section component reading that context (`TerminalStyleSection`), and one
  entry appended to the exported `SETTINGS_SECTIONS` array. A Notification
  section follows the identical shape (own context, own zero-prop component,
  appended descriptor) — do not thread props through `SettingsModal`.
- `frontend/src/settingsStore.ts:24-63` — `loadNamespacedPrefs`/
  `saveNamespacedPrefs`, the shared versioned-JSON `"ws-dashboard.<feature>.v<N>"`
  persistence helper over `browserStorage()`. `frontend/src/terminalPrefs.ts`
  (whole file) is the concrete wiring example: a typed prefs shape, a
  `parse*` defensive-parse function, `load*`/`save*` wrappers, and
  `App.tsx:2046-2057`'s `handleTerminalPrefsChange` + memoized context value —
  the new Notification-enabled pref follows this exact shape (e.g.
  `"ws-dashboard.settings.notifications.v1"`).
- `frontend/src/App.tsx:2059-2061,2200` — `TerminalPrefsContext.Provider`/
  `SettingsTerminalContext.Provider` wrap the app shell; a new
  `SettingsNotificationContext.Provider` nests alongside them the same way.
- `frontend/src/gitWorktreeRemoveModal.tsx:184-193` — existing checkbox
  markup pattern (`<label><input type="checkbox" .../><span>...</span></label>`)
  to follow for the toggle's JSX shape; no dedicated "settings toggle"
  component exists yet, this is the closest sibling.
- `frontend/e2e/agent-attention-indicator.spec.ts` — the harness Phase 6/7
  already built and the file's own header comments recording why: a POST
  straight at the Phase 4 callback endpoint (reading the on-disk token with
  Node `fs`, never through `page.evaluate`/URL/log, `:55-60`) synthesizes a
  turn boundary without a real vendor CLI. Phase 7 added a **second** `test()`
  to this same file rather than a new sibling spec specifically to reuse the
  daemon/workRoot/token-read module-locals (`:110-125` comment) — Phase 8
  should default to the same move (a third `test()` here) rather than
  standing up a fourth daemon, unless the Settings-panel interaction proves
  awkward to co-locate.
- `frontend/package.json:24-25,31` — `test:agent-attention` runs
  `agentAttention.test.js`; `test:settings` runs `settingsStore.test.js`,
  `terminalPrefs.test.js`, `settingsSections.test.js`; `test:browser` is the
  only script that chains `npm run build` ahead of `playwright test`.
  `frontend/tsconfig.route-tests.json`'s `include` array is an explicit file
  list (not a glob for top-level `src/*.ts`) — a new pure module and its
  `*.test.ts` must both be added to that list, and its test file added to
  either `test:agent-attention`, `test:settings`, or a new npm script.
- `frontend/tsconfig.app.json:6` declares `"lib": ["ES2022", "DOM",
  "DOM.Iterable"]`; `tsconfig.route-tests.json` declares no `lib` override,
  so it gets TypeScript's default-for-target set, which for an `ES2022`
  target already includes `DOM` — confirmed existing route-tests code
  already relies on DOM types (`settingsStore.ts`'s `Pick<Storage,
  "getItem">`). A new pure module using `Notification`/`NotificationPermission`
  types compiles under both programs with no config change.
- `ai-docs/mental-model/ws-web-dashboard/index.md:240` — the two-tsconfig-program
  trap: `workbench/index.ts` is a barrel: any `workbench/*` module
  back-importing from `../App.js` drags all of `App.tsx` into the NodeNext
  route-tests program. Not directly triggered here since the new pure
  module(s) should live as siblings to `agentAttention.ts` (imported by
  `App.tsx`, not importing it), never inside `workbench/`.
- `ai-docs/mental-model/ws-web-dashboard/index.md:247` — the stale-`dist`
  Playwright trap (restated above in Relevant Ticket Contract; the item-6
  evidence rule is this mental-model entry, not new information).
- A same-word false lead, worth recording so it is not re-tripped: mental-model
  entry `:242` mentions a prior "`Notification`-hook negative" — that is the
  Claude Code vendor CLI's own hook **event name** `Notification` (a
  `SessionStart`/`Stop`/`Notification` hook subsystem), unrelated to the
  browser `Notification` API this phase implements. No actual coupling; flagged
  only because the string match cost a few minutes during survey.

## Implementation Plan

1. **Global attention tone, computed in `App.tsx` (not a new predicate).**
   Add a `useMemo`-derived `globalAttentionTone: "ready" | "working" | null`
   near the existing `agentAttentionByRoot` derivation
   (`App.tsx:4356-4379`). Build it by flat-mapping `resourcesByServer`'s
   entries the same way `ServerRows` does for one server
   (`App.tsx:3189-3199`): for each `(serverId, resources)`, for each
   `resources.workspaces`, call the existing unexported
   `navAttentionWorkRootIds(workspace, workNavOrder.hiddenWorktreesByWorkspace[serverScopedIdentity(serverId, workspace.id)])`,
   map to `serverScopedIdentity(serverId, rootId)`, concatenate across all
   servers/workspaces into one root-key array, then call
   `aggregateNavAttentionTone(agentAttentionByRoot, thatArray)`. This is the
   literal mechanism for requirement 4 (hidden roots excluded because they
   never enter `navAttentionWorkRootIds`'s output) and requirement 3 (no new
   predicate — reuses `aggregateNavAttentionTone` → `navAttentionTone` →
   `NavAttentionCounts`, all of which bottom out in `pendingAttentionStateFor`
   via `aggregateNavAttentionCounts`).

2. **New pure module `frontend/src/browserAttentionCue.ts`** (sibling to
   `agentAttention.ts`, not under `workbench/`, imported by `App.tsx` only —
   avoids the barrel back-import trap). Exports:
   - `attentionTitleFor(baseTitle: string, active: boolean, flashOn: boolean): string`
     — pure string builder: returns `baseTitle` when `!active` or `!flashOn`,
     else an attention-labeled variant (e.g. `"● Attention needed"`). No DOM
     access, unit-testable.
   - `buildAttentionFaviconHref(active: boolean): string` — pure SVG
     data-URI string builder (a plain circle/dot glyph, not a canvas
     rasterization of `icon-192.png`). See step 3 for the "why SVG over
     canvas" call. No DOM/Image/canvas access, unit-testable as a string
     comparison.
   - `shouldFireAttentionNotification(previousTone: "ready" | "working" | null,
     currentTone: "ready" | "working" | null): boolean` — pure edge detector,
     true only on a transition INTO `"ready"` (`previousTone !== "ready" &&
     currentTone === "ready"`). This is the "no second watermark" mechanism:
     the only state it needs is the previous render's `globalAttentionTone`
     value (held in a plain `useRef` in `App.tsx`, not persisted, not keyed by
     terminal id), so it cannot diverge from the tab/nav badges — it is
     driven by the exact same derived tone in the exact same render.
   Add `src/browserAttentionCue.ts` and its `.test.ts` to
   `tsconfig.route-tests.json`'s `include` array.

3. **Decision: single badge/flash for both `working` and `ready`, edge-fire
   only `ready` for the OS notification.** Title flash and favicon badge
   activate whenever `globalAttentionTone !== null` (mirrors the nav row,
   which shows a tone-colored badge for both states) — these are passive,
   non-interrupting cues. The OS `Notification` (Tier 2, genuinely
   interrupting) fires only on the edge into `"ready"` specifically, since
   that is the pinned "needs the user" state (`working` is normal background
   progress, not an actionable interruption). Recorded as a plain
   implementation decision, not an escalation, because it directly follows
   the tone vocabulary Phase 7 already pinned (`ready` = orange bell = needs
   you; `working` = spinner = informational) and does not add any new
   per-entity state.
   **Favicon mechanism: SVG data URI, not canvas.** Canvas would need an
   `Image` load of `icon-192.png` (async, needs onload/race handling, and the
   badge state can only be observed from inside a DOM effect with no
   unit-testable pure core) or a pre-built PNG asset per state (asset
   duplication for no real benefit, since the badge is a simple dot, not a
   redraw of the whole icon). A plain SVG string is a pure function of one
   boolean, testable without any DOM/Canvas/Image API — consistent with this
   repo's "no jsdom harness" constraint (mental model, `## Common Mistakes`)
   and the Testability code standard. It does not need to reproduce
   `icon-192.png` pixel-for-pixel; a simple monochrome glyph with a badge dot
   is enough.
   **Flash keeps running regardless of tab focus/visibility**, stopping only
   when `globalAttentionTone` itself returns to `null` (i.e., the user
   acknowledges the last pending tab through the existing per-pane
   mechanism). Stopping on `visibilitychange`/focus alone while the tone
   stays non-null would make the title agree with a glance at the tab but
   disagree with the still-orange nav/tab badges — exactly the divergence
   the binding carry-forward (requirement 3) rules out.

4. **Wire the cue in `App.tsx`.** One `useEffect` keyed on
   `globalAttentionTone`:
   - Maintains a flash-tick `setInterval` (e.g. ~1s) toggling a local `flashOn`
     boolean while `globalAttentionTone !== null`; clears the interval and
     resets `document.title = "ws dashboard"` (the literal base title from
     `index.html:6`) when it becomes `null`. Each tick sets
     `document.title = attentionTitleFor(...)`.
   - On every tick/tone-change, sets the `<link rel="icon">` element's `href`
     to `buildAttentionFaviconHref(globalAttentionTone !== null)` — find the
     existing link via `document.querySelector('link[rel="icon"]')` (the one
     `index.html:16` renders) rather than creating a duplicate node.
   - Holds `previousToneRef = useRef<Tone>(null)`; on each tone recomputation,
     if `shouldFireAttentionNotification(previousToneRef.current,
     globalAttentionTone)` and the Settings pref is enabled and
     `typeof Notification !== "undefined" && Notification.permission ===
     "granted"`, calls `new Notification(...)` with a short, non-sensitive
     body (no terminal id, no path — same identity-privacy posture the
     ticket already applies to the callback token). Updates
     `previousToneRef.current` unconditionally after the check.

5. **Settings section: `frontend/src/settingsSections.tsx`.** Add
   `SettingsNotificationContext` (`{ enabled: boolean; onChange: (next:
   boolean) => void }`) and a zero-prop `NotificationSection` component,
   mirroring `SettingsTerminalContext`/`TerminalStyleSection`. Render a single
   checkbox (pattern: `gitWorktreeRemoveModal.tsx:184-193`) plus copy text
   stating plainly that OS-level notification requires `localhost` or TLS (a
   plain-http LAN page cannot use it), and — since `window.isSecureContext` is
   readable without a permission prompt — show the current
   `Notification.permission` state (or "unavailable, insecure context") so
   the limitation is visible rather than surprising, per the ticket text.
   Append `{ id: "notifications", title: "Notifications", Component:
   NotificationSection }` to `SETTINGS_SECTIONS`
   (`settingsSections.tsx:90-92`).
   The checkbox's own `onChange` handler — a real user gesture — is the ONLY
   call site for `Notification.requestPermission()`. Guard it: do nothing if
   `typeof Notification === "undefined"` (not `window.isSecureContext` alone —
   a plain-http LAN page lacks the whole API, not just permission). Persist
   `enabled` via `loadNamespacedPrefs`/`saveNamespacedPrefs`
   (`"ws-dashboard.settings.notifications.v1"`, version 1), following
   `terminalPrefs.ts`'s exact shape (typed prefs object, `parse*` defensive
   parser, `load*`/`save*` wrappers).

6. **Wire the new context/state into `App.tsx`** the same way
   `terminalPrefs`/`settingsTerminalContextValue` are wired
   (`App.tsx:535-537,2046-2061`): a `useState` seeded from
   `loadNotificationPrefs()`, a `handleNotificationPrefsChange` callback that
   sets state then persists, a memoized context value, and a
   `SettingsNotificationContext.Provider` nested alongside the existing two
   Providers (`App.tsx:2059-2061,2200`).

7. **Spec.** Extend `## Dashboard Settings Panel {#260722-ws-dashboard-settings-panel}`
   (`ai-docs/spec/ws-web-dashboard/index.md:906-955`) with a paragraph for the
   second registered section: what it stores, that permission is requested
   only from the toggle's own click (never on load), and the plain
   localhost/TLS-requirement statement the Settings copy must carry. Follow
   the existing section's descriptive style (see the Terminal-section
   paragraph at `:946-955` for the tone/level of detail expected).

## Verification Plan

- Unit (pure logic, no browser): add `browserAttentionCue.test.ts` covering
  `attentionTitleFor`, `buildAttentionFaviconHref`, and
  `shouldFireAttentionNotification`'s edge-only behavior (in particular:
  `working` → `ready` fires, `ready` → `ready` does NOT re-fire, `ready` →
  `working` → `ready` fires again — proving there is no independent
  "already notified" watermark beyond the one-slot previous-tone ref).
  Extend `settingsSections.test.ts` for the new descriptor (mirrors the
  existing registry-shape assertions at `settingsSections.test.ts:22-76`: the
  `SETTINGS_SECTIONS` length grows to 2, the new descriptor's `id`/`title`
  are stable, its `Component` is the module-scope reference and takes zero
  props). Add both new/changed test files to
  `tsconfig.route-tests.json`'s `include` array and to a package.json test
  script (extend `test:agent-attention` or `test:settings`, or add a new
  `npm run test:<name>` entry following the existing per-suite convention).
- `cargo test -p ws-dashboard-daemon --lib` and `--test routes` — expect no
  new failures; the routes suite's 2 pre-existing failures
  (`routes.rs:1066`, `routes.rs:1383`) are known and unrelated. This phase
  makes no daemon change, so this is a regression check, not new coverage.
- `npm run build` (frontend) — MUST run before any Playwright pass used as
  evidence, per the item-6/mental-model-`:247` rule: `playwright.config.ts`
  has no `webServer`, and the daemon harness serves whatever sits in
  `frontend/dist`. A bare `npx playwright test` after a `src/` edit with no
  rebuild is indistinguishable from a pass against the stale pre-edit bundle.
- Browser acceptance (Tier 1 only, per the ticket's verification boundary):
  add a third `test()` to `frontend/e2e/agent-attention-indicator.spec.ts`,
  reusing its existing daemon/workRoot/token-read module-locals (per that
  file's own `:110-125` precedent-setting comment for Phase 7's second
  test). Synthesize a turn boundary the same way the existing two tests do
  (direct POST to the Phase 4 callback endpoint with the on-disk token),
  then assert: (a) `document.title` changes away from `"ws dashboard"` while
  the tone is non-null (poll across at least one flash-tick interval so a
  first-tick coincidence isn't mistaken for a lack of toggling — assert both
  the flashed and the base string are each observed at least once); (b) the
  `<link rel="icon">` element's `href` changes to a non-default value; (c)
  acknowledging the last pending tab (existing mechanism from Phase 6/7)
  returns both to their base values. Run via
  `npx playwright test agent-attention-indicator.spec.ts` (rebuilt first per
  the rule above) and separately confirm `dashboard-acceptance.spec.ts`'s
  failure stays isolated to the known `:3779` site with `:4020` skipping
  behind it — no new failure site introduced.
- Tier 2 (`Notification` permission tier): **manual only**, recorded in the
  Phase 8 Result, per the ticket's explicit verification boundary. Do not
  attempt to automate a real permission-prompt grant in Playwright (see Out
  of Scope on `context.grantPermissions`). Manual check: toggle the Settings
  checkbox on `localhost`, accept the real browser permission prompt,
  synthesize a `ready` transition, confirm a real OS notification appears;
  then confirm the Settings copy visibly states the localhost/TLS
  requirement when the same toggle is viewed over a plain-http LAN origin
  (`Notification` truly `undefined`/insecure-context there).
- `ws/spec_index.verify` — confirm the settings-panel spec entry edit stays
  clean against the index.

## Escalations

- None.
