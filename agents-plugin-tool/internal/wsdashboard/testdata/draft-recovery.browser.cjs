const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.argv[2], shots = process.env.WS_DRAFT_CAPTURE_DIR;
const before = !!process.env.WS_DRAFT_CAPTURE_BEFORE;
const session = '/s/livesession', other = '/s/boardsession';
const png64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jZ9kAAAAASUVORK5CYII=';

async function frameFor(page, route = session) {
  const handle = await page.locator(`iframe.sframe.on[src^="${route}?"]`).elementHandle();
  const frame = await handle.contentFrame();
  await frame.locator('#grid:not([inert])').waitFor();
  return frame;
}
async function type(frame, value) {
  await frame.evaluate(value => {
    const field = document.querySelector('#ctext');
    field.value = value;
    field.setSelectionRange(value.length, value.length);
    field.dispatchEvent(new Event('input', { bubbles: true }));
  }, value);
}
async function paste(frame) {
  await frame.evaluate(png64 => {
    const bytes = Uint8Array.from(atob(png64), char => char.charCodeAt(0));
    const transfer = new DataTransfer();
    transfer.items.add(new File([bytes], 'draft.png', { type: 'image/png' }));
    document.querySelector('#ctext').dispatchEvent(new ClipboardEvent('paste', {
      clipboardData: transfer, bubbles: true, cancelable: true
    }));
  }, png64);
}
async function send(frame, command) {
  if (command) await frame.evaluate(() => {
    const field = document.querySelector('#ctext');
    field.value = 'compact keep context';
    field.wsc.restore([{ start: 0, end: 7, kind: 'skill', id: 'compact', label: 'compact' }]);
  });
  await frame.locator('#csend').click();
}
async function pickOther(page) {
  if (!await page.locator('html').evaluate(root => root.classList.contains('rail-open'))) {
    await page.locator('#railbtn').click();
  }
  await page.locator('#gear').waitFor({ state: 'visible' });
  await page.locator('.rs[data-rid="boardsession"] .rl').click();
}
async function capture(page, name, theme) {
  if (!shots) return;
  await page.screenshot({ path: path.join(shots, `${before ? 'before' : 'after'}-${name}-${theme}.png`) });
}

async function delivery(browser, { theme, accepted, command, capturePair }) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: theme, locale: 'en-US' });
  await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const native = [];
  page.on('dialog', async dialog => { native.push({ type: dialog.type(), message: dialog.message() }); await dialog.dismiss(); });
  let posted;
  const started = new Promise(resolve => { posted = resolve; });
  let release, replayed, seen = [];
  const replay = new Promise(resolve => { replayed = resolve; });
  const pending = new Promise(resolve => { release = resolve; });
  await context.route('**/s/livesession/message', async route => {
    const body = route.request().postDataJSON();
    seen.push(body);
    if (accepted && seen.length === 1) {
      const response = await route.fetch();
      assert.equal(response.ok(), true, await response.text());
    }
    if (seen.length > 1) {
      await route.fulfill({ json: { id: 'explicit-browser-retry' } });
      replayed();
      return;
    }
    posted();
    await pending;
    await route.abort().catch(() => {});
  });
  try {
    await page.goto(base + session);
    let frame = await frameFor(page);
    if (!command) await type(frame, 'Check the next release draft');
    await send(frame, command);
    await started;
    assert.equal(seen.length, 1);
    if (command) assert.equal(seen[0].command, 'compact');
    if (!command) {
      await type(frame, 'This message was still queued');
      await send(frame, false);
    }
    await type(frame, 'Notes for the next message');
    await frame.evaluate(() => window.wsSaveSessionDraft());
    if (capturePair) {
      if (before) await capture(page, 'inpage-dialog', theme);
      await pickOther(page);
      if (before) {
        await page.waitForFunction(() => location.pathname === '/s/livesession');
        if (shots) fs.writeFileSync(path.join(shots, `before-native-dialog-${theme}.json`), JSON.stringify(native));
      } else {
        const dialog = page.locator('dialog[open]').filter({ hasText: /Leave this session/ });
        await dialog.waitFor();
        await capture(page, 'inpage-dialog', theme);
        await dialog.getByRole('button', { name: 'Stay' }).click();
        assert.equal(new URL(page.url()).pathname, session);
        assert.equal(await (await frameFor(page)).locator('#ctext').inputValue(), 'Notes for the next message');
        await pickOther(page);
        await dialog.getByRole('button', { name: 'Leave' }).click();
        await frameFor(page, other);
      }
    }
    const reopened = await context.newPage();
    reopened.on('pageerror', error => errors.push(error.message));
    await reopened.goto(base + session);
    frame = await frameFor(reopened);
    if (before) {
      await capture(reopened, 'delivery-unknown', theme);
      return;
    }
    const unknown = frame.locator('.d-outbox-recovery .d-outbox-row').filter({ hasText: 'Delivery unknown' });
    await unknown.waitFor();
    assert.match(await unknown.innerText(), command ? /compact keep context/ : /Check the next release draft/);
    assert.equal(await frame.locator('#ctext').inputValue(), 'Notes for the next message');
    if (!command) await frame.locator('.d-outbox-recovery .d-outbox-row').filter({ hasText: 'This message was still queued' }).waitFor();
    assert.equal(seen.length, 1, 'reopen does not replay a send');
    assert.deepEqual(native, [], 'frame swap uses the page dialog');
    if (capturePair) await capture(reopened, 'delivery-unknown', theme);
    if (command && !accepted) {
      await unknown.getByRole('button', { name: 'Resend' }).click();
      const confirmation = frame.locator('dialog[open]');
      await confirmation.getByRole('button', { name: 'Cancel' }).click();
      assert.equal(seen.length, 1, 'Cancel does not resend');
      await unknown.getByRole('button', { name: 'Resend' }).click();
      await confirmation.getByRole('button', { name: 'Resend' }).click();
      await unknown.waitFor({ state: 'hidden' });
      await replay;
      assert.equal(seen.length, 2, 'explicit resend starts one additional POST');
      assert.deepEqual(seen[1], seen[0]);
    } else {
      await unknown.getByRole('button', { name: 'Discard' }).click();
    }
    await frame.evaluate(() => window.wsSaveSessionDraft());
    assert.equal(await frame.locator('#ctext').inputValue(), 'Notes for the next message');
    assert.deepEqual(errors, []);
  } finally {
    release();
    await context.close();
  }
}

async function recoverImage(browser) {
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    await page.goto(base + session);
    let frame = await frameFor(page);
    await type(frame, 'Image draft');
    await paste(frame);
    assert.equal(await frame.evaluate(() => typeof window.wsNativeCloseReady), 'undefined');
    assert.equal(await page.evaluate(() => window.wsNativeCloseReady()), true);
    const native = [];
    page.on('dialog', async dialog => { native.push(dialog.type()); await dialog.dismiss(); });
    await page.reload();
    frame = await frameFor(page);
    const thumbnail = frame.locator('#cimgs img[alt="draft.png"]');
    await thumbnail.waitFor();
    const imageBytes = async () => frame.locator('#cimgs img[alt="draft.png"]').evaluate(async image => {
      const blob = await (await fetch(image.src)).blob();
      return Array.from(new Uint8Array(await blob.arrayBuffer()));
    });
    assert.deepEqual(await imageBytes(), [...Buffer.from(png64, 'base64')]);
    assert.match(await frame.locator('#ctext').inputValue(), /Image draft/);
    assert.deepEqual(native, []);

    // A stale adapter can render the same session's board without the
    // composer. Use the served board-only template with the same draft key.
    await page.route('**/s/livesession?frame=1', async route => {
      const response = await context.request.get(base + '/s/boardsession?frame=1');
      const body = (await response.text()).replaceAll('boardsession', 'livesession');
      await route.fulfill({ body, contentType: 'text/html' });
    }, { times: 1 });
    await page.reload();
    frame = await frameFor(page);
    assert.equal(await frame.locator('#ctext').count(), 0);
    await frame.evaluate(() => window.wsSaveSessionDraft());
    await page.reload();
    frame = await frameFor(page);
    await frame.locator('#cimgs img[alt="draft.png"]').waitFor();
    assert.deepEqual(await imageBytes(), [...Buffer.from(png64, 'base64')]);
    assert.match(await frame.locator('#ctext').inputValue(), /Image draft/);

    // An older page can rewrite its legacy projection. Newer-only bytes
    // still belong to this session after returning to the recovery-capable page.
    await page.goto(base + '/');
    await page.evaluate(() => {
      const key = 'ws.dashboard.draft.livesession';
      const previous = JSON.parse(localStorage.getItem(key));
      localStorage.setItem(key, JSON.stringify({ ...previous, text: 'New input from an older page', lost: [] }));
    });
    await page.goto(base + session);
    frame = await frameFor(page);
    await frame.locator('#cimgs img[alt="draft.png"]').waitFor();
    assert.match(await frame.locator('#ctext').inputValue(), /New input from an older page.*\[Image #1\]/s);
    await frame.locator('#cimgs [data-unimg]').click();
    await frame.evaluate(() => window.wsSaveSessionDraft());
    await page.reload();
    frame = await frameFor(page);
    assert.equal(await frame.locator('#cimgs img[alt="draft.png"]').count(), 0);
    assert.match(await frame.locator('#ctext').inputValue(), /New input from an older page/);
  } finally { await context.close(); }
}

async function failedStorage(browser) {
  const context = await browser.newContext();
  try {
    await context.addInitScript(() => {
      Object.defineProperty(window, 'indexedDB', { value: undefined });
      const set = Storage.prototype.setItem;
      Storage.prototype.setItem = function (key, value) {
        if (key.startsWith('ws.dashboard.draft.')) throw new DOMException('Draft quota fixture', 'QuotaExceededError');
        return set.call(this, key, value);
      };
    });
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    const page = await context.newPage();
    const requests = [];
    page.on('request', request => {
      if (request.method() === 'POST' && /\/message$/.test(request.url())) requests.push(request.url());
    });
    await page.goto(base + session);
    const frame = await frameFor(page);
    await frame.locator('.cfield').click();
    await type(frame, 'This draft could not be saved');
    await paste(frame);
    assert.equal(await page.evaluate(() => window.wsNativeCloseReady()), false);
    const native = new Promise(resolve => page.once('dialog', async dialog => {
      const type = dialog.type();
      await dialog.dismiss();
      resolve(type);
    }));
    void page.reload({ timeout: 2500 }).catch(() => {});
    assert.equal(await native, 'beforeunload', 'real reload retains the browser veto');
    await pickOther(page);
    const dialog = page.locator('dialog[open]');
    await dialog.getByRole('button', { name: 'Stay' }).click();
    assert.match(await frame.locator('#ctext').inputValue(), /This draft could not be saved.*\[Image #1\]/s);
    await send(frame, false);
    await frame.getByText('Unsent', { exact: true }).waitFor();
    assert.deepEqual(requests, [], 'failed durability leaves the send available without posting');
  } finally { await context.close(); }
}

async function ackWithUnavailableLegacyStorage(browser) {
  const context = await browser.newContext();
  try {
    await context.addInitScript(() => {
      const set = Storage.prototype.setItem;
      Storage.prototype.setItem = function (key, value) {
        if (key.startsWith('ws.dashboard.draft.')) throw new DOMException('Draft quota fixture', 'QuotaExceededError');
        return set.call(this, key, value);
      };
    });
    const page = await context.newPage();
    let received, delivered;
    const receivedPost = new Promise(resolve => { received = resolve; });
    const reply = new Promise(resolve => { delivered = resolve; });
    await page.route('**/s/livesession/message', async route => {
      received();
      await reply;
      await route.fulfill({ json: { id: 'legacy-quota-ack' } });
    });
    await page.goto(base + session);
    let frame = await frameFor(page);
    await type(frame, 'This send will be acknowledged');
    await send(frame, false);
    await receivedPost;
    await type(frame, 'Keep this later input after the acknowledgement');
    await frame.evaluate(() => window.wsSaveSessionDraft());
    delivered();
    await frame.locator('#r-convlive [data-entry="legacy-quota-ack"]').waitFor();
    assert.equal(await frame.evaluate(() => window.wsSaveSessionDraft()), true);
    const next = await context.newPage();
    await next.goto(base + session);
    frame = await frameFor(next);
    assert.equal(await frame.locator('#ctext').inputValue(), 'Keep this later input after the acknowledgement');
    assert.equal(await frame.locator('.d-outbox-recovery .d-outbox-row').count(), 0);
  } finally { await context.close(); }
}

async function queueAcrossTabs(browser) {
  const context = await browser.newContext();
  let releaseFirst;
  const firstReply = new Promise(resolve => { releaseFirst = resolve; });
  try {
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    const requests = [];
    let firstPosted;
    const firstRequest = new Promise(resolve => { firstPosted = resolve; });
    await context.route('**/s/livesession/message', async route => {
      const { text } = route.request().postDataJSON();
      requests.push(text);
      if (text === 'First tab in-flight') {
        firstPosted();
        await firstReply;
      }
      const response = await route.fetch();
      assert.equal(response.ok(), true, await response.text());
      await route.fulfill({ response });
    });
    const original = await context.newPage();
    await original.goto(base + session);
    const first = await frameFor(original);
    await type(first, 'First tab in-flight');
    await send(first, false);
    await firstRequest;
    const queued = 'Queued exactly once across tabs';
    await type(first, queued);
    await send(first, false);
    await first.evaluate(() => window.wsSaveSessionDraft());
    const reopened = await context.newPage();
    await reopened.goto(base + session);
    const second = await frameFor(reopened);
    const row = second.locator('.d-outbox-recovery .d-outbox-row').filter({ hasText: queued });
    const sent = reopened.waitForResponse(response => /\/message$/.test(response.url()));
    await row.getByRole('button', { name: 'Send' }).click();
    const { id: queuedID } = await (await sent).json();
    await second.locator(`#r-convlive [data-entry="${queuedID}"]`).first().waitFor();
    await second.evaluate(() => window.wsSaveSessionDraft());
    const acknowledged = original.waitForResponse(response => /\/message$/.test(response.url()));
    releaseFirst();
    const { id: firstID } = await (await acknowledged).json();
    await first.locator(`#r-convlive [data-entry="${firstID}"]`).first().waitFor();
    await first.locator('.d-outbox-row').filter({ hasText: queued }).waitFor({ state: 'hidden' });
    assert.deepEqual(requests, ['First tab in-flight', queued], 'the original queue cannot replay the already claimed item');
  } finally { releaseFirst(); await context.close(); }
}

(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) await delivery(browser, { theme, accepted: theme === 'dark', capturePair: true });
    if (!before) {
      for (const accepted of [false, true]) await delivery(browser, { theme: 'dark', accepted, command: true });
      await recoverImage(browser);
      await failedStorage(browser);
      await ackWithUnavailableLegacyStorage(browser);
      await queueAcrossTabs(browser);
    }
    console.log(before ? 'baseline captures complete' : 'normal and command POST recovery, frame and storage safeguards, and cross-tab send claim passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
