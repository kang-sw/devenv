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

## When to reuse vs. extend this doc

Reuse this procedure as-is for any future dashboard chat/session live-UI
bug report. If a future probe needs a selector or endpoint not listed above,
extend this doc in place rather than re-deriving the method from scratch in
a fresh session.
