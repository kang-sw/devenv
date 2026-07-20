# ws Dashboard Headless-Browser Verification

Probed 2026-07-13/14 while diagnosing
`260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`. Documents a
repeatable, no-human-in-the-loop procedure for driving the real
`ws-dashboard` frontend against a real running daemon and a real Codex/Claude
session, without relying on the repo's Playwright acceptance suite
(`ws-dashboard/frontend/e2e/`) or on a human manually reproducing a bug.
Use this whenever a dashboard bug report needs live-UI confirmation and the
existing acceptance spec doesn't already cover the interaction.

## Why not just the existing Playwright suite

`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` is a scripted
acceptance gate with its own `webServer`/fixture lifecycle; it is not meant
for one-off ad hoc reproduction of a specific live-session bug, and at least
one of its later steps has an open, separately-tracked blocker
(`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`). The
procedure below reuses the same installed Playwright/Chromium binaries but
skips the suite's fixture entirely, driving the real no-auth daemon instead.

## Prerequisites

- A running daemon in no-auth debug mode (loopback-only, disables owner
  pairing):
  ```bash
  cd ws-dashboard
  cargo run -p ws-dashboard-daemon -- serve --static-dir frontend/dist --no-auth --port <port>
  ```
  Confirm via the log line `ws-dashboard daemon listening bound_addr=...`.
  See `crates/daemon/src/cli.rs`'s `ServeArgs.no_auth` for the flag's exact
  semantics.
- Playwright's Chromium already installed under
  `~/.cache/ms-playwright/` (installed as part of the frontend's own e2e
  dependency; check with `npx playwright --version` and
  `ls ~/.cache/ms-playwright`).

## Running a headless probe script

Node's ESM loader resolves bare imports (`import { chromium } from
'playwright'`) relative to the *importing file's own directory*, not the
process's cwd — a script placed under a scratch/tmp directory cannot resolve
`playwright` even when run with `cwd` set to `ws-dashboard/frontend`. Write
the probe script directly under `ws-dashboard/frontend/` (e.g.
`.tmp-probe.mjs`), run it with plain `node`, and delete it when done — do not
leave it in the tree or stage it in a commit.

Minimal skeleton:

```js
import { chromium } from 'playwright';

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

// Capture the real API calls the frontend makes — this is the fastest way
// to discover exact work-root/session-id-bearing REST paths without reading
// `activitySessionClient.ts`'s route-building helpers first.
page.on('request', req => {
  if (req.url().includes('/api/dashboard/')) {
    console.log('[REQ]', req.method(), req.url(), req.method() === 'POST' ? req.postData() : '');
  }
});

// `waitUntil: 'networkidle'` never resolves — the dashboard keeps
// long-lived polling connections open, so network never goes idle. Use
// 'load' plus an explicit `waitForTimeout` instead.
await page.goto('http://127.0.0.1:<port>/', { waitUntil: 'load' });
await page.waitForTimeout(2000);
```

## Driving the real UI

Confirmed working selectors/flow as of `260711`/`260713`-series chat UI:

1. Click a work root by its exact label text: `page.getByText('<root-name>', { exact: true })`.
2. Toolbar buttons are reachable by their `title` attribute, not visible
   text — enumerate with
   `page.locator('header button, [class*="toolbar"] button').all()` and read
   each one's `title`/`aria-label` if the exact button is unknown. The new-tab
   entry point is `page.getByTitle('Open new agent tab')`.
3. The harness picker (`Codex` / `OpenCode` / `Claude`) appears as plain
   text buttons: `page.getByText('Codex', { exact: true }).click()`.
4. The chat prompt input is `page.getByTestId('agent-chat-prompt-input')` —
   its placeholder is `"Send a message…"` with a Unicode ellipsis (`…`,
   U+2026), so `getByPlaceholder('Send a message...')` (three ASCII dots)
   silently never matches; prefer the `data-testid`.
5. Submit via the visible `Send` button
   (`page.getByRole('button', { name: 'Send' })`), not `Enter` — the prompt
   input is a plain `<input>`, and pressing Enter was not verified to submit.

## Cross-checking against the daemon directly

The request-logging listener above reveals the exact
`workRootId`/`activityId` pair for the just-created session, e.g.:

```
POST http://127.0.0.1:<port>/api/dashboard/work-roots/<workRootId>/activity/codex-sessions {}
POST http://127.0.0.1:<port>/api/dashboard/work-roots/<workRootId>/activity/codex-sessions/<activityId>/prompt {"text":"hello"}
```

(`<activityId>` must be URL-encoded, e.g. `codex%3A<hex>` for `codex:<hex>`.)
Poll the transcript endpoint directly with `curl` to see the daemon-side
projection independent of frontend rendering/merge logic — this is the
fastest way to tell whether a symptom is a backend projection gap or a
frontend rendering/merge bug:

```bash
curl -s "http://127.0.0.1:<port>/api/dashboard/work-roots/<workRootId>/activity/codex-sessions/<activityId>/transcript" | python3 -m json.tool
```

Key response fields: `live` (true while a turn is in flight; the frontend's
`beginRealStreamingTurn` poll loop, `activitySessionClient.ts`, stops polling
once this goes `false`), and `blocks[].cursor`/`renderKind`/`title`/`text`
(no `role`/`turnId` field exists on the wire as of `260713`-series work —
see `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`).

## Known pitfall this method caught

Driving a fresh Codex session end-to-end this way (create session -> send
"hello" -> poll both the live browser DOM and the daemon transcript
endpoint for 45-90s) revealed that the four transcript blocks appearing
within ~10s of session creation are Codex's own automatic
AGENTS.md-driven session-start orientation turn (triggered by `thread/start`
itself, independent of any explicit prompt), not a reply to the user's
message — the explicit `send_prompt`-issued `turn/start` for "hello" produced
no new transcript blocks even after 90s of polling. Relying on transcript
content alone without also checking wall-clock ordering relative to the
actual `/prompt` POST would have misattributed the orientation turn's reply
as an answer to the user's message.

## Cold-start build+run E2E workflow

Probed 2026-07-20. The manual "Prerequisites" procedure above assumes a
pre-built `frontend/dist` and a manually-launched no-auth daemon. There is
also a scripted cold-start path that builds everything from scratch and runs
the full Playwright acceptance suite, defined as `test:browser` in
`ws-dashboard/frontend/package.json`:

```bash
cd ws-dashboard/frontend
npm run test:browser
# == npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test
```

This is a materially different flow from the ad hoc probe method above, not
just a wrapper around it:

- It rebuilds `frontend/dist` (`npm run build`) and the debug daemon binary
  (`cargo build -p ws-dashboard-daemon`) fresh, then runs
  `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` under Playwright's
  own `webServer`-free fixture lifecycle (`e2e/daemonHarness.ts`'s
  `startDaemon`), not a manually-launched
  `cargo run -p ws-dashboard-daemon -- serve --no-auth ...` instance.
- `daemonHarness.ts` spawns `target/debug/ws-dashboard[.exe]` itself, scrapes
  the one-time owner-pairing URL from its stdout/stderr
  (`owner pairing URL: <url>`), waits on `/healthz`, and the suite performs
  **real owner pairing** through that scraped URL as its first `test.step` —
  it does not run in `--no-auth` mode.
- The suite sets `WS_DASHBOARD_E2E_AGENT_FIXTURE=1` on the spawned daemon's
  env unless already set (`daemonHarness.ts` around line 233), which makes
  `crates/daemon/src/discovery.rs` (`push()`, around line 211) synthesize a
  fixture "main instance" per enabled work root for the resources/discovery
  view. This is unrelated to the Codex/Claude session-launch stub discussed
  below — it only affects what `discovery.rs` reports as already-running
  instances, not which code path a freshly-launched chat session takes.
- The whole file's top-level tests run under
  `test.describe.configure({ mode: "serial" })`, so a failure partway through
  the first test (e.g. the hidden-transcript bug below) aborts everything
  after it in that test *and* skips the second top-level test entirely — a
  failure here can hide unrelated, otherwise-passing coverage.
- `test:browser`'s CONTRACT header comment
  (`dashboard-acceptance.spec.ts` lines 21-25) states this file's coverage
  scope is workbench tab polish evidence (hover-close affordances,
  confirmation popovers, pinned/opened tab presentation, preview-to-pinned
  file behavior) driven against the daemon-served production frontend — it
  is a broad scripted acceptance gate, not a substitute for the ad hoc
  live-bug-reproduction method documented above.

Prefer the manual no-auth + ad hoc probe-script method above for one-off live
bug reproduction (much faster iteration, no full rebuild each time); reserve
`npm run test:browser` for full-suite acceptance runs or when the bug is
specifically about the suite's own fixtures/steps.

## Driving a real (non-stub) harness

Probed 2026-07-20. Everything above (and the existing acceptance suite) can
be driven either against a synthetic in-browser stub or against a real
spawned `codex`/`claude` CLI process — the two look similar in the UI but
exercise completely different code paths. This section documents how to
deliberately reach the real one.

- **Binaries must be on `PATH` and pre-authenticated.** The daemon does not
  bundle or manage CLI auth; it shells out to whatever `codex`/`claude`
  resolves to for the user running it. Check resolution before assuming
  anything: `which codex`, `which claude` (or `command -v` inside a script
  that might run under a shell function/alias). On the machine this was
  probed on, `codex` resolved through a shell function to a brew-installed
  `@openai/codex` npm shim
  (`/home/linuxbrew/.linuxbrew/bin/codex -> .../lib/node_modules/@openai/codex/bin/codex.js`)
  and `claude` resolved to `~/.local/bin/claude ->
  ~/.local/share/claude/versions/<version>`. Exact resolution is
  machine-specific and will differ elsewhere — always re-check, never assume
  a path from this doc still holds.
- **The daemon spawns them directly, with no test-mode substitution.**
  `crates/daemon/src/codex_app_server.rs` and `crates/daemon/src/claude_cli.rs`
  both call `Command::new(&self.config.codex_bin)` /
  `Command::new(&self.config.claude_bin)` (around lines 772 and 753
  respectively), and `codex_bin`/`claude_bin` default to the bare binary name
  (`"codex"` / `"claude"`, resolved via `PATH`). No environment-variable or
  build-flag override path exists in either file — there is no built-in
  daemon-side mock/stub mode to defeat; if the binary resolves and is
  authenticated, the daemon runs the real thing.
- **The frontend's own harness routing matters more than which test file you
  use.** `ws-dashboard/frontend/src/App.tsx`'s `realAgentChatHarness()`
  (around line 430) routes exactly `"codex"` and `"claude"` tile picks
  through the real fetch-based `activitySessionClient.ts` adapter
  unconditionally; only `"opencode"` (no real adapter wired yet) stays on the
  synthetic in-browser `activitySessionStub.ts` provider. So clicking the
  Codex or Claude tile — whether by hand, via an ad hoc probe script, or via
  the acceptance suite — always issues the real REST calls
  (`POST .../activity/codex-sessions`, etc.) to the daemon, which in turn
  spawns a real CLI process, provided that binary is reachable.
- **Do not trust the acceptance spec file's own naming/assertions as proof of
  stub isolation.** `dashboard-acceptance.spec.ts`'s
  `"open new agent tab and launch a stub harness session"` step (around line
  2534) still asserts the transcript contains the literal string
  `"stub provider"` after clicking the Codex tile — that string only exists
  in `activitySessionStub.ts`. This assertion predates the real-adapter
  wiring described above and reads as a stale leftover from before Codex/Claude
  had a real adapter, not a guarantee that the suite runs stub-isolated
  today. Don't reason about real-vs-stub behavior from that spec file's
  comments or test names.
- **Recommended way to reach a real harness deliberately**: don't use
  `dashboard-acceptance.spec.ts` at all (its fixture lifecycle and stale
  stub-flavored assertions make outcomes there ambiguous). Instead:
  1. Launch the daemon standalone exactly as in "Prerequisites" above
     (`--no-auth`, an isolated `WS_DASHBOARD_STATE_HOME`, bound to
     `127.0.0.1:<port>` — not `localhost`, which can resolve to a different
     loopback interface and complicate cookie/origin assumptions).
  2. Drive it with a plain ad hoc Playwright script per "Running a headless
     probe script" / "Driving the real UI" above, clicking the Codex/Claude
     tile using the confirmed selectors.
  3. Cross-check via the transcript endpoint ("Cross-checking against the
     daemon directly" above) and/or network interception (next section) to
     confirm real, non-synthetic content — a real turn takes noticeably
     longer and produces different transcript shapes than the stub's
     near-instant canned text (see "Known pitfall this method caught" above
     for a concrete example of a real Codex turn's actual timing/content).

## Verifying past a hidden-but-functional element

Probed 2026-07-20, while chasing
`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`. That ticket
documents a standing, separately-tracked defect: the
`agent-chat-transcript` div (`[data-testid="agent-chat-transcript"]`,
`App.tsx:7816`, styled by `.agent-chat-pane-transcript` in `styles.css`) can
be computed-`hidden` — via an as-yet-untraced ancestor, likely a Dockview
inactive-tab/pane visibility mechanism — even when the underlying session
and its transcript data are completely healthy on the daemon side. The tab
header updates correctly and the prompt input appears; only the transcript
body area fails to become visible.

This is a landmine for any verification script: `toBeVisible()` assertions
and screenshots will both look "broken" in this state regardless of whether
the actual thing you're investigating has anything to do with visibility.
Do not conclude "the session/data is broken" from a hidden transcript alone.

Instead, verify real data flow through channels that don't depend on CSS
visibility:

- Read DOM `textContent`/`innerHTML` directly — unlike `click()`,
  Playwright's `textContent()`/`innerHTML()` locator methods do not
  auto-wait for visibility, so they still return real content on a
  computed-hidden element.
- Intercept the underlying network traffic (`page.on('response', ...)` or
  route interception) to confirm the daemon actually produced/streamed the
  expected data, independent of whether the frontend chooses to display it.

Reusable idiom combining both:

```js
// Track transcript API responses independent of the DOM's visual state.
const transcriptResponses = [];
page.on('response', async (res) => {
  if (res.request().method() === 'GET' && res.url().includes('/transcript')) {
    const body = await res.json().catch(() => null);
    if (body) transcriptResponses.push(body);
  }
});

// ... click the Codex/Claude tile, send a prompt, wait as needed ...

// Do NOT gate on toBeVisible() here — the transcript div can be
// computed-hidden per 260713-bug-dashboard-acceptance-codex-tile-transcript-hidden
// even when the session is completely healthy. Read the DOM directly instead;
// textContent()/innerHTML() work even while the element is hidden.
const transcript = page.locator('[data-testid="agent-chat-transcript"]');
const transcriptHtml = await transcript.innerHTML();
console.log('[transcript DOM, may be CSS-hidden]', transcriptHtml.slice(0, 500));

// Cross-check against the last polled transcript response.
const last = transcriptResponses.at(-1);
console.log('[transcript API]', 'live:', last?.live, 'blocks:', last?.blocks?.length);
```

If the DOM `innerHTML`/API `blocks` show real content but `toBeVisible()`
still fails, the symptom is the known hidden-transcript bug (or a sibling of
it), not a data/session problem — do not misattribute it as a backend or
session-launch failure.

## Screenshot capture

Probed 2026-07-20. `page.screenshot(...)` at key steps is useful for
after-the-fact visual review of a probe run, e.g.:

```js
await page.screenshot({
  path: '/path/to/session/scratchpad/codex-tile-after-send.png',
  fullPage: true,
});
```

- Save under this session's own scratchpad directory, not `/tmp` directly
  and never inside the repo tree — the existing acceptance suite keeps its
  own screenshots (`.artifacts/`, Playwright's `test-results/`) gitignored
  for the same reason (`ws-dashboard/frontend/e2e/.gitignore`,
  `ws-dashboard/frontend/.gitignore`).
- Treat screenshots as ephemeral, session-scoped debugging aids for the
  current investigation only, not permanent artifacts — do not cite a
  screenshot path in a ticket or writeup as something that will still exist
  later; re-run the probe to regenerate one if needed (mirrors
  `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`'s own
  caveat about its `test-failed-1.png`: "cite only", not moved/copied).
- A screenshot alone is not sufficient evidence of a working (or broken)
  feature — see "Verifying past a hidden-but-functional element" above for
  why a passing/failing visual check can be misleading on its own.

## When to reuse vs. extend this doc

Reuse this procedure as-is for any future dashboard chat/session live-UI
bug report. If a future probe needs a selector or endpoint not listed above,
extend this doc in place rather than re-deriving the method from scratch in
a fresh session.
