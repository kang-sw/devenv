// Exercise the actual page's stream dispatch, swap and merge with no canonical
// baseline. Hold only the mandatory first full; production code is unmodified.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.ENTRY_ROWS_BASE;
const address = new URL(base);
assert(['127.0.0.1', 'localhost', '[::1]'].includes(address.hostname) && address.port && address.port !== '29417');
(async () => {
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: 'reduce' });
    await context.route('**/*', route => new URL(route.request().url()).origin === address.origin ? route.continue() : route.abort());
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    await context.addInitScript(() => {
      const Native = window.WebSocket;
      window.WebSocket = class extends Native {
        set onmessage(handler) {
          super.onmessage = event => {
            const message = JSON.parse(event.data);
            if (!window.firstEntryFull && message.event === 'region' && message.data.name === 'entries') {
              window.firstEntryFull = { socket: this, message };
              return;
            }
            handler.call(this, event);
          };
        }
      };
    });
    const page = await context.newPage(), errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto(base + '/s/livesession');
    const frame = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await frame.waitForFunction(() => window.firstEntryFull);
    const result = await frame.evaluate(() => {
      const { socket, message } = firstEntryFull, region = document.getElementById('r-entries');
      const full = message.data;
      const send = data => socket.dispatchEvent(new MessageEvent('message', { data: JSON.stringify({ event: 'region', data }) }));
      const source = document.createElement('template'); source.innerHTML = full.html;
      const input = [...source.content.querySelector('.centries').children].find(n => n.textContent.includes('entry-row-mail-a'));
      if (!input) throw Error('fixture mail row missing');
      const key = input.getAttribute('data-entry');
      const find = () => [...region.querySelector('.centries').children].find(n => n.getAttribute('data-entry') === key);
      const original = find(), box = region.querySelector('.centries');
      if (original.getAttribute('data-mail-wake') !== null) throw Error('fixture must not rely on mail-wake transition');
      const older = document.createElement('div'); older.setAttribute('data-entry', '0'); older.textContent = 'reader-owned older history'; box.prepend(older);
      // The first canonical input differs from server-rendered markup, with
      // exactly the same key and mail-wake association. There was no earlier
      // entry full dispatch, so entryRendered is still null.
      const first = { name: 'entries', hash: 'fixture-first-full-different', html: full.html.replaceAll('entry-row-mail-a', 'first-full-current-mail') };
      send(first);
      if (find() === original || !find().textContent.includes('first-full-current-mail')) throw Error('missing-baseline first full must replace stale same-key entry');
      if (!older.isConnected || box !== region.querySelector('.centries')) throw Error('first full lost reader-owned history/container');
      const current = find(), head = region.querySelector('.thead');
      send(first);
      if (find() !== current || region.querySelector('.thead') !== head) throw Error('equal-hash full must not remount');
      // A full that cannot be canonicalized clears entryRendered. Its next
      // valid differing full must not inherit a fictitious rendered baseline.
      send({ name: 'entries', hash: 'fixture-unparseable-full', html: first.html + '<!--ws-entry-row-0-->' });
      const beforeRecovery = find();
      send({ name: 'entries', hash: 'fixture-recovered-full', html: first.html.replaceAll('first-full-current-mail', 'recovered-current-mail') });
      if (find() === beforeRecovery || !find().textContent.includes('recovered-current-mail')) throw Error('full after unparseable baseline must replace stale same-key entry');
      if (!older.isConnected || box !== region.querySelector('.centries')) throw Error('baseline recovery lost reader-owned history/container');
      return { initialMissingBaseline: 'replaced', unparseableBaseline: 'recovered', equalHash: 'no remount', olderHistory: 'retained' };
    });
    assert.deepEqual(errors, []);
    console.log(JSON.stringify(result));
    await context.close();
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
