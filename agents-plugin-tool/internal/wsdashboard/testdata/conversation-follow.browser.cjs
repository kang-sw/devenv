// Actual Enter/button sends and wheel scrolling on the production session page.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.FOLLOW_BASE, updates = process.env.FOLLOW_UPDATES;
const shots = process.env.FOLLOW_SHOTS;
console.log(`OWN node_pid=${process.pid}`);
const geometry = frame => frame.evaluate(() => {
  const c = document.getElementById('convo');
  const tail = document.querySelector('#r-convlive .md') || document.getElementById('r-convlive');
  const last = tail.lastElementChild || tail;
  const r = c.getBoundingClientRect(), t = last.getBoundingClientRect();
  return { top: c.scrollTop, height: c.scrollHeight, client: c.clientHeight,
    gap: c.scrollHeight - c.clientHeight - c.scrollTop, tailTop: t.top, tailBottom: t.bottom, viewportTop: r.top, viewportBottom: r.bottom };
});
const bottom = async frame => {
  await frame.waitForFunction(() => { const c = document.getElementById('convo'); return c.scrollHeight - c.clientHeight - c.scrollTop <= 2; });
  const g = await geometry(frame);
  assert(g.tailTop >= g.viewportTop - 2 && g.tailBottom <= g.viewportBottom + 2, `reply tail visible: ${JSON.stringify(g)}`);
  return g;
};
(async () => {
  const server = await chromium.launchServer({ headless: true });
  const pid = server.process().pid;
  console.log(`OWN chromium_pid=${pid}`);
  const browser = await chromium.connect(server.wsEndpoint());
  const evidence = {};
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const shot = async (page, name) => {
    if (shots) await page.screenshot({ path: `${shots}/${name}-${process.pid}.png` });
  };
  try {
    for (const method of ['enter', 'button']) {
      await fetch(updates + '?stage=initial');
      const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US' });
      const errors = [], blocked = [];
      // Only the two disposable fixture origins may be contacted.
      await context.route('**/*', route => {
        const origin = new URL(route.request().url()).origin;
        if (origin === base || origin === updates) return route.continue();
        blocked.push(route.request().url());
        return route.abort();
      });
      const page = await context.newPage();
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(base + '/s/livesession');
      const frame = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
      await frame.waitForFunction(() => document.getElementById('r-convlive').textContent.includes('TAIL-initial'));
      const editor = frame.locator('#composer .wsc-editor');
      await editor.waitFor();
      await bottom(frame);
      const c = frame.locator('#convo');
      await c.hover();
      const initial = await geometry(frame);
      await page.mouse.wheel(0, -650);
      await frame.waitForFunction(top => document.getElementById('convo').scrollTop < top - 300, initial.top);
      const up = await geometry(frame);
      assert(up.gap > 300 && up.height > up.client * 2, 'real overflowing conversation scrolled up');

      let requests = 0, release, arrived;
      const pending = new Promise(resolve => { arrived = resolve; });
      const gate = new Promise(resolve => { release = resolve; });
      // Delay transport, not the UI: prove the jump precedes any server ack.
      await page.route('**/s/livesession/message', async route => {
        requests++;
        assert.equal(route.request().postDataJSON().text, `follow-${method}`);
        arrived();
        await gate;
        await route.continue();
      });
      let responsePromise;
      try {
        // Empty and composing Enter must leave follow cancelled and send nothing.
        await editor.click();
        await page.keyboard.press('Enter');
        await editor.fill('composition');
        await editor.dispatchEvent('keydown', { key: 'Enter', code: 'Enter', isComposing: true, keyCode: 229 });
        assert.equal(requests, 0, 'empty/IME guards do not send');
        assert((await geometry(frame)).gap > 300, 'guarded intent does not jump');
        await editor.fill(`follow-${method}`);
        await shot(page, `before-${method}`);
        evidence[method] = { before: await geometry(frame) };
        responsePromise = page.waitForResponse(r => r.url() === base + '/s/livesession/message' && r.request().method() === 'POST');
        if (method === 'enter') await page.keyboard.press('Enter');
        else await frame.locator('#csend').click();
        await pending;
        evidence[method].intent = await bottom(frame);
        await shot(page, `after-${method}`);
      } finally { release(); }
      const response = await responsePromise;
      assert.equal(response.status(), 200, 'real fixture accepts message');
      await frame.waitForFunction(() => document.getElementById('ctext').value === '');
      await frame.waitForFunction(text => document.getElementById('r-convlive').textContent.includes(text), `follow-${method}`);
      for (const stage of ['reply', 'growth']) {
        const prior = await geometry(frame);
        assert.equal((await fetch(updates + '?stage=' + stage)).status, 204);
        await frame.waitForFunction(s => document.getElementById('r-convlive').textContent.includes('TAIL-' + s), stage);
        const next = await bottom(frame);
        assert(next.height > prior.height + 100, 'incoming SSE reply really grows');
        evidence[method][stage] = next;
      }
      await shot(page, `reply-${method}`);
      const following = await geometry(frame);
      await c.hover();
      await page.mouse.wheel(0, -650);
      await frame.waitForFunction(top => document.getElementById('convo').scrollTop < top - 300, following.top);
      const cancelled = await geometry(frame);
      await fetch(updates + '?stage=cancelled');
      await frame.waitForFunction(() => document.getElementById('r-convlive').textContent.includes('TAIL-cancelled'));
      // Two animation frames settle the browser's native scroll/layout events.
      await frame.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const after = await geometry(frame);
      assert(after.height > cancelled.height + 100, 'further incoming growth after upward scroll');
      assert(Math.abs(after.top - cancelled.top) <= 2 && after.gap > 300, `upward wheel cancels follow: ${JSON.stringify({ cancelled, after })}`);
      evidence[method].cancelled = { before: cancelled, after };
      assert.equal(requests, 1, 'exactly one real send');
      assert.deepEqual(errors, [], 'no page errors');
      assert.deepEqual(blocked, [], 'no external requests');
      await shot(page, `cancelled-${method}`);
      await context.close();
    }
    console.log(JSON.stringify(evidence));
    if (shots) fs.writeFileSync(`${shots}/evidence-${process.pid}.json`, JSON.stringify(evidence, null, 2));
  } catch (error) {
    for (const context of browser.contexts()) for (const page of context.pages()) await shot(page, 'failure');
    throw error;
  } finally {
    await browser.close();
    await server.close();
    assert.equal(server.process().exitCode === null && server.process().signalCode === null, false, 'owned Chromium exited');
    console.log(`OWN chromium_pid=${pid} exited`);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
