---
title: Tier 2 notification - automate the reachable gate, fix the insecure-context guard
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 8 shipped this tier and left its permission half undischarged; this ticket discharges it
spec:
  - 260726-dashboard-browser-level-attention-cue
  - 260722-ws-dashboard-settings-panel
sage-review-design: completed
sage-review-completeness: completed
---

# Tier 2 notification - automate the reachable gate, fix the insecure-context guard

The stem still reads `...-manually` because ticket stems are immutable. The
scope is no longer manual-only: the premise that made it manual-only was tested
directly and did not survive. See Background.

## Background

`260725-feat-dashboard-pty-agent-attention-notification` Phase 8 shipped both
tiers of the browser-level attention cue. Its verification boundary read: "the
title/favicon tier asserted in browser acceptance; the permission tier verified
manually and recorded, since driving a real permission prompt in the harness is
not worth its cost." That Phase 8 Result closes with an explicit OUTSTANDING
note: the permission tier has never been observed, only reasoned about.

### The premise that was wrong

The parent plan put `context.grantPermissions` in Out of Scope with this
reasoning: "using the grant API would still only prove the app calls
`Notification` correctly, not that the real browser permission flow behaves as
expected, and the ticket already assigns that half to manual+recorded."

The first clause assumes "the app calls `Notification` correctly" is already
covered. It is not covered anywhere, at any level:

- `browserAttentionCue.test.ts` covers `shouldFireAttentionNotification` as a
  pure two-argument function. It never touches the App.tsx effect.
- The App.tsx Tier 2 effect - the conjunction of that edge detector,
  `notificationPrefs.enabled`, `typeof Notification !== "undefined"`,
  `Notification.permission === "granted"`, and the guarded constructor - has
  zero coverage. Search `new Notification(` to land on it.
- `NotificationSection`'s `onChange` handler - the `requestPermission()` call,
  the `Promise.resolve(...)` legacy-form wrapper, the denied-reconciliation
  `onChange(false)`, and the force-re-render - has zero coverage.
  `settingsSections.test.ts` asserts descriptor shape only (id, title,
  `Component` identity, component arity); it never renders the section.
- `currentNotificationAvailability()` is module-private with three branches and
  zero coverage.

So the grant API would not be re-proving something reasoning already covered.
It is the only way to reach a genuinely unverified conjunction, and that
conjunction is the whole feature. Reopening the choice was correct.

### What was measured

Probed directly against this repo's installed Playwright 1.60 / Chromium
(macOS, standalone probe scripts driving a throwaway `node:http` origin - not
through this project's `playwright.config.ts`). Results, all reproducible:

| condition | `isSecureContext` | `typeof Notification` | `.permission` | `requestPermission()` | `new Notification(...)` |
| --- | --- | --- | --- | --- | --- |
| localhost, default headless, granted | true | defined | **denied** | granted | constructs |
| localhost, `channel: "chromium"`, granted | true | defined | **granted** | granted | constructs |
| localhost, `channel: "chromium"`, not granted | true | defined | denied | **denied** | n/a |
| plain-http LAN IP, either channel | **false** | **defined** | denied | denied | constructs |

Four findings follow, and each one changes the ticket.

1. **The default Playwright browser cannot host this gate.** Playwright's
   default Chromium is `chromium-headless-shell`, which hard-denies
   notifications: `Notification.permission` stays `"denied"` even after a
   successful `grantPermissions(["notifications"])`. The app's own guard reads
   that exact property, so under the default browser Tier 2 can never fire.
2. **`channel: "chromium"` fixes it, headless.** The full Chromium build
   (already present in the local Playwright cache alongside the headless shell,
   same 1223/1228 revisions - no extra download step) reports `"granted"` and
   constructs successfully with `headless: true`. No headed window, no xvfb.
3. **Both branches of the toggle are drivable.** With the permission pre-granted
   `requestPermission()` resolves `granted`; without it, it resolves `denied` -
   deterministically, from a real click, with no dialog and no hang (measured
   with an 8s settle timeout). The denied-reconciliation path is therefore
   automatable too, which the parent ticket had listed as an aside "worth a look
   while there".
4. **The insecure-context guard is wrong, and its spec sentence with it.** See
   below. This is the ticket's most consequential finding.

### The defect this investigation surfaced

`currentNotificationAvailability()` in `settingsSections.tsx` tests
`typeof Notification === "undefined"` FIRST and only consults
`window.isSecureContext` inside that branch. Its comment justifies the order by
asserting that "a plain-http LAN page lacks the whole `Notification` global, not
merely a granted permission - `window.isSecureContext` alone would not
distinguish 'insecure' from 'secure but denied'."

Measured in Chromium, that is backwards. On a plain-http LAN origin the
`Notification` global **is defined**; `isSecureContext` is the property that
distinguishes the two cases, and `typeof Notification` is the one that does not.
The consequences on the LAN - the dashboard's routine access mode, and the
reason Tier 1 exists at all:

- The insecure-context copy is unreachable. The section falls through and
  renders "Current permission: denied", which reads as "you denied this / fix
  your browser settings" rather than "this page is not secure; use localhost or
  TLS".
- The checkbox is still offered and still clickable. Clicking it calls
  `requestPermission()`, which resolves `denied`, which fires `onChange(false)`,
  which silently unchecks the box with no explanation - precisely the "surprising
  the user after a click does nothing" outcome the section's own comment says the
  eager availability check exists to prevent.
- The spec sentence under `#260726-dashboard-browser-level-attention-cue` -
  "the page is not a secure context and the browser's `Notification` API is
  absent entirely - not merely un-permissioned" - is false for Chromium.

Step 6 of this ticket's original manual checklist is exactly the step that would
have caught this. It was found by an agent instead, before any human ran it.

Scope of the measurement: Chromium 148 (bundled) and Chrome 150 (local channel),
macOS. Safari and Firefox may genuinely omit the global on insecure origins, so
the fix is a reorder that stays correct for both browser classes, never a swap.

## Decisions

Settled here, under unattended goal-run posture, as reversible and local:

- **The gate runs under `channel: "chromium"`, not headed and not the default
  headless shell.** Headed would work but needs a display and pops a window
  mid-run; the channel switch is one line and keeps the gate CI-shaped.
- **The gate lives in a NEW sibling spec file, not inside
  `agent-attention-indicator.spec.ts`.** Measured against the installed
  Playwright 1.60, not reasoned: `channel` is registered
  `{ scope: "worker", option: true }` (`playwright/lib/index.js:197`), and a
  worker-scoped option inside a `test.describe` is a hard LOAD ERROR -
  "Cannot use({ channel }) in a describe group, because it forces a new
  worker. Make it top-level in the test file or put in the configuration
  file." (`playwright/lib/common/index.js:1601`). Reproduced on a throwaway
  probe config: the file fails to load and contributes zero tests. So the
  originally planned shape - a channel-switched `test.describe` co-located
  with the existing tests - does not merely split workers, it does not run at
  all. The two shapes that do work are file-level `test.use` on the existing
  file (which silently moves four unrelated, currently-green gates onto a
  different browser build, and makes the whole file fail on any cache holding
  only `chromium-headless-shell`) or a sibling file with its own file-level
  `test.use`. The sibling wins: it scopes the channel requirement to the one
  gate that needs it, and it follows the `agent-spawn-profile.spec.ts`
  precedent the existing file's own CONTRACT comment cites. The cost argument
  that once favoured co-location - "avoid a second daemon" - is void, because
  a distinct worker hash forces a fresh worker process, a fresh module
  instance, and therefore a second `beforeAll` daemon either way.
- **The gate pairs its own owner session; it does not reuse `ownerCookies`.**
  Follows from the above: the sibling file has its own daemon and its own
  unconsumed one-time pairing URL, so it calls `pairOwner` in its first test
  exactly as `agent-attention-indicator.spec.ts` does. Reaching for the other
  file's `ownerCookies` would be a cross-module read that does not exist.
- **The `Notification` global is observed through a `Proxy`, never replaced by a
  stub.** A `construct` trap records the arguments and then
  `Reflect.construct`s the real constructor, so the real platform path still
  runs and a browser that refuses to construct still throws into the app's
  existing `catch`. Measured: proxied and unproxied reads of
  `Notification.permission` agree in every mode tested, so the wrapper does not
  perturb the property the app gates on. Rejected: assigning a plain recording
  class or object over `window.Notification`, which would make the gate assert
  only that the app called a test double.
- **The recorder is installed with `page.addInitScript`,** which must be
  installed before the first `goto`. Precise about what survives a reload: the
  init SCRIPT re-runs on every navigation, so the `Proxy` is always in place;
  the recorded construction LIST does not survive, because a reload builds a
  fresh JS realm and the array starts empty again. Every construction count
  must therefore be read inside the same document as the transitions it counts.
  See Phase 1 for the assertion-placement rule this forces.
- **Non-vacuity is a precondition assertion, not an assumption.** The gate must
  assert in-page that `Notification.permission === "granted"` before asserting a
  fire. Without it, a wrong-channel run makes every negative assertion ("toggle
  off fires nothing") pass for the wrong reason, with output indistinguishable
  from a real pass - the same failure shape as the stale-`dist` trap.
- **The insecure-context fix ships in this ticket rather than a separate bug
  ticket.** It is the direct product of the verification this ticket exists to
  perform, and it is a guard reorder plus a copy claim. Split it out if the owner
  prefers; nothing in Phase 1 depends on it.

Rejected alternatives:

- **A Playwright gate driving a real native permission dialog.** Still correctly
  out of scope: Playwright answers the permission request programmatically and
  never renders the native dialog, so there is nothing to observe. This is the
  irreducible human residue.
- **A browser gate on a real plain-http LAN origin.** Needs a routable LAN
  address and `--bind-mode public`, and Chromium refuses `grantPermissions`
  there outright ("Permission can't be granted in current context"), so the
  interesting states are unreachable anyway. Phase 2's pure-function unit
  coverage reaches every branch more cheaply; the LAN observation stays human.
- **Asserting that an OS notification is displayed.** Chromium constructs it;
  whether the OS paints a banner is outside the browser and outside Playwright.
  Human residue.

## Constraints

- **The harness serves the prebuilt bundle.** Any `frontend/src` change used as
  evidence - including every mutation run for non-vacuity - must be followed by
  `npm run build` before Playwright runs. Only `npm run test:browser` chains the
  build; a bare `npx playwright test` serves whatever bundle is on disk and
  produces output indistinguishable from a genuine pass. Full account in the
  `ws-web-dashboard` mental model's Common Mistakes; do not re-derive it the
  hard way. `*.spec.ts` files themselves are always read fresh.
- `test.use({ channel: "chromium" })` is legal at FILE scope only. `channel` is
  a worker-scoped option, and Playwright rejects worker-scoped options in a
  `test.describe` with a load error rather than honouring them
  (`playwright/lib/common/index.js:1601`); inside a single `test()` it is
  invalid for the ordinary reason. Verified on 1.60 against a probe config, not
  read off the docs.
- A file-level channel switch forces a distinct worker hash, and grouping is
  keyed on that hash before anything else
  (`playwright/lib/runner/index.js:2239-2295`), so the channel-switched file
  gets its own worker process. Module state is not shared with any other spec
  file: no daemon, no `workRoot`, no `ownerCookies`. `test.beforeAll` runs
  again there. Budget for a second daemon; it is unavoidable, not a choice.
- The existing spec file is serial and reuses `ownerCookies` captured by its
  first test because that daemon's owner pairing URL is one-time. The new file
  gets its own daemon and therefore its own unconsumed pairing URL, so it pairs
  fresh in its first test instead of borrowing a cookie.
- Playwright permission grants are per-context and every `test()` gets a fresh
  context, so a granted test and a denied test coexist in one file without
  cross-contamination.

## Phases

Phase 1 and Phase 2 are independent and may be taken in either order.

### Phase 1: browser gate for the Tier 2 notification path

Add Tier 2 coverage as a NEW sibling spec under `e2e/` (suggested
`agent-attention-notification.spec.ts`), carrying a FILE-level
`test.use({ channel: "chromium" })`. See Decisions for why this is not a
`test.describe` inside `agent-attention-indicator.spec.ts`: that shape is a
Playwright load error, and the "avoid a second daemon" argument that motivated
co-locating is void because the channel switch forces a separate worker and a
second `beforeAll` regardless.

The new file therefore stands up its own daemon, `WS_DASHBOARD_STATE_HOME`,
workRoot, and owner pairing, following `agent-spawn-profile.spec.ts` and the
`beforeAll`/`afterAll` shape of `agent-attention-indicator.spec.ts`. The
helpers both files now need - the on-disk callback-token read and the direct
turn-state POST - are module-local and unexported today. Lift them into a
shared module under `e2e/` alongside `daemonHarness.ts` rather than
copy-pasting them into a second file; duplicate only if the extraction ripples
further than this phase should reach, and say so if you do.

Completed behavior - the gate proves, at browser level, what no test currently
reaches:

1. Precondition, asserted not assumed: in-page `Notification.permission` is
   `"granted"`. A run that cannot satisfy this must fail here, loudly, rather
   than pass every later negative assertion vacuously.
2. Turning the Settings > Notifications checkbox on from a real click leaves it
   checked and the pref persisted, with permission pre-granted on the context.
3. A `null|working -> ready` transition, driven by the existing turn-state
   callback POST, constructs exactly one `Notification`, with the shipped title
   and body strings asserted literally.
4. `ready -> ready` (a second agent reaching ready while one already is)
   constructs nothing more - the aggregate never left `ready`.
5. `ready -> working -> ready` constructs a second one - the edge detector is
   not a one-shot latch.
6. With the toggle off and permission still granted, a `ready` edge constructs
   nothing. This is the assertion that pins the pref as a real gate rather than
   a decoration.
7. In a context with no grant, clicking the toggle reconciles it back to
   unchecked and leaves the persisted pref disabled, and a following `ready`
   edge constructs nothing.

Assertion placement, load-bearing for 4 and 6: the recorded construction list
is per-document and a `page.reload()` empties it (see Decisions). Any counted
window - most of all the two "constructs nothing more" assertions - must open
and close inside ONE document, with no reload between the transition and the
count. A reload placed mid-sequence makes those two assertions pass with an
empty array no matter what the app did, which is the same indistinguishable
false green the non-vacuity rule exists to catch. A reload also perturbs the
count in the other direction: per
`#260726-dashboard-browser-level-attention-cue`, reloading while the aggregate
is already `ready` re-fires Tier 2 by design, so a reload can ADD an
unexpected construction to an "exactly one" count. The existing indicator spec
reloads to resync React state after a direct fetch; if this gate needs the
same, do it before the counted window opens, never inside it - and if a test
deliberately counts a reload's own notification, say so at the assertion.

Deferred scope: no assertion that the OS displays anything; no native dialog; no
LAN origin; no service-worker/push path.

Verification boundary for this phase: Playwright green under
`npm run test:browser`, plus a per-assertion non-vacuity run - mutate the guard
each assertion targets, rebuild, observe that assertion fail at its own site.
Assertions 4 and 6 are the ones most likely to be vacuous; treat their mutation
runs as mandatory, not sampled. Confirm the four tests in
`agent-attention-indicator.spec.ts` and `dashboard-acceptance.spec.ts`
introduce no new failure site - they stay on the default channel, so a
regression there would come from the extracted helpers, not the browser build.

### Phase 2: correct the insecure-context guard, its comment, and its spec claim

Reorder `currentNotificationAvailability()` to consult `window.isSecureContext`
before `typeof Notification`, so a plain-http LAN page reports the
insecure-context state instead of falling through to "denied". Keep the
`typeof Notification === "undefined"` branch: it is still the correct answer for
a browser that genuinely omits the global, which the reorder must not stop
reaching.

Extract the decision as an exported pure function of its three inputs
(secure-context flag, global presence, permission value) so all four states are
assertable from `settingsSections.test.ts`, which already imports this module
under the NodeNext route-test program and today asserts only registry shape.
This is the cheap substitute for a LAN browser gate.

Also settle whether the checkbox should be offered at all on an insecure origin.
The current behavior - offer it, then silently uncheck it after
`requestPermission()` resolves `denied` - is the surprise the section was written
to avoid. Disabling the control on an insecure context is the recommendation;
it is a caller-visible change, so record the choice either way.

Spec amendment, required and load-bearing, in two parts.

First, the correction. The `#260726-dashboard-browser-level-attention-cue`
sentence claiming the `Notification` API is "absent entirely" on a non-secure
context is false for Chromium. Restate it as un-permissioned-and-ungrantable
rather than absent, and keep the conclusion that Tier 2 cannot work there.
`#260722-ws-dashboard-settings-panel` carries the same claim ("a plain-http LAN
page lacks the whole `Notification` global, not merely a granted permission");
amend it in lockstep.

Second, the new behavior. Whichever way the disabled-control question above is
settled, the settings-panel anchor must state it. Its current text only
promises that the section "shows the current state ... rather than only
surprising the user after an unresponsive click" - which says nothing about
whether the control is offered. If the control becomes disabled on an insecure
origin, that is caller-visible and belongs in that paragraph next to the
corrected availability claim; if it stays offered, say that instead, so the
next reader does not have to re-derive it from the code. Do not land Phase 2
with a caller-visible change no spec sentence covers.

Run `ws/spec_index.verify` after editing.

Deferred scope: no change to Tier 1, to the edge detector, or to the
`new Notification(...)` call site and its load-bearing `catch`.

Verification boundary for this phase: unit assertions for all four availability
states including both insecure variants; `npm run build`; if the control's
disabled state changes, one browser assertion for it - and per the constraint
above, rebuild before running it.

## Human verification residue

What no harness can reach. Not a phase - a person performs this once and records
it. Best done after Phase 2, so the LAN half checks the corrected behavior.

On `localhost` or a TLS origin:

1. Open Settings > Notifications with permission still at its browser default.
   Pass: the copy names the secure-context requirement and shows a live
   permission state.
2. Check the toggle. Pass: a native browser permission prompt appears **on the
   click** and not on page load. This is the single observation nothing
   automated can make - Playwright answers this request without ever drawing it.
3. Accept it. Pass: the section shows the granted state without a reopen or an
   unrelated interaction.
4. Drive a `ready` transition (the callback-token POST the e2e spec uses, or a
   real agent). Pass: an OS notification is actually displayed. On macOS, a
   missing banner here is most likely the browser lacking authorization in
   System Settings > Notifications rather than a dashboard defect - check there
   before filing anything.
5. Reload with an agent still pending. Pass: it notifies again. This is recorded
   spec behavior, not a defect.

On a plain-http LAN origin:

6. Open the same section. Pass: it reports the insecure-context state and does
   not present a control that cannot work. Before Phase 2 this step fails,
   reporting "Current permission: denied" - that is the known defect, not a new
   finding.

## Done when

Phases 1 and 2 are landed, and the six numbered steps of the
`## Human verification residue` section above are recorded durably - an
`#### Edition` on the parent ticket's Phase 8 Result is the natural home,
since that Result is what currently carries the OUTSTANDING note. A step that
disagrees with the spec after Phase 2 is a new bug ticket, not an edit to this
one.

Rule for an implementer who cannot discharge the residue, which is the normal
case under unattended goal-run posture: land both phases, do NOT close this
ticket, and surface the residue explicitly in the final response and in the
landing commit's `## Ticket Updates`. Steps 1-5 need a person watching a native
prompt and an OS banner; no agent may mark them observed, infer them from a
green Playwright run, or record an `#### Edition` on their behalf. Leaving the
ticket open with only the residue outstanding is the intended terminal state
for the automated half - closing it silently is exactly the move that produced
this ticket out of
`260725-feat-dashboard-pty-agent-attention-notification` Phase 8.
