// Production pages and transport on disposable loopback fixtures. No captures.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.ENTRY_ROWS_BASE, updates = process.env.ENTRY_ROWS_UPDATES;
for (const address of [base, updates]) {
  const u = new URL(address);
  assert(['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname) && u.port && u.port !== '29417', 'disposable loopback only');
}

(async () => {
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US', reducedMotion: 'reduce' });
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    // Retain the native connection solely to inject malformed daemon events;
    // all validation, close/reconnect and mandatory first-full handling is real.
    await context.addInitScript(() => {
      const Native = window.WebSocket;
      window.entrySockets = [];
      window.WebSocket = class extends Native {
        constructor(...args) { super(...args); window.entrySockets.push(this); }
      };
    });
    const page = await context.newPage(), errors = [], events = [], histories = [], unexpected = [];
    page.on('pageerror', e => errors.push(e.message));
    page.on('response', r => { if (new URL(r.url()).pathname === '/s/livesession/history') histories.push(r.url()); });
    await context.route('**/*', route => {
      const u = new URL(route.request().url());
      if (['http:', 'https:'].includes(u.protocol) && ![new URL(base).origin, new URL(updates).origin].includes(u.origin)) {
        unexpected.push(u.href); return route.abort();
      }
      return route.continue();
    });
    page.on('websocket', socket => {
      if (!socket.url().includes('/s/livesession/events')) return;
      assert(socket.url().includes('entryrows=1'), 'session opts in');
      socket.on('framereceived', ({ payload }) => {
        const e = JSON.parse(payload.toString());
        if (e.event === 'entry-rows' || (e.event === 'region' && e.data?.name === 'entries')) events.push(e);
      });
    });
    await page.goto(base + '/s/livesession');
    const frame = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await frame.locator('#r-entries .centries > [data-entry]').first().waitFor({ state: 'attached' });
    await eventually(() => events.length > 0, 'mandatory first full');
    assert.equal(events[0].event, 'region');

    const validation = await frame.evaluate(source => {
      const doc = document;
      const root = doc.createElement('div'), formatted = [];
      let resets = 0, parses = 0;
      const region = () => root, restoreOpen = () => {}, fmtTimes = n => formatted.push(n), sweepOutImgs = () => {}, mermaid = () => {}, histReset = () => resets++;
      const api = new Function('doc', 'region', 'restoreOpen', 'fmtTimes', 'sweepOutImgs', 'mermaid', 'histReset', source + ';return {entryInputFull,entryInputPatch,entryInputHTML,mergeEntriesRegion,entryKeys};')(doc, region, restoreOpen, fmtTimes, sweepOutImgs, mermaid, histReset);
      // Count actual template parsing without changing the parser's behavior.
      const create = doc.createElement;
      doc.createElement = function(name, ...args) { if (name === 'template') parses++; return create.call(doc, name, ...args); };
      try {
        const row = (key, text, attr = '') => `<details data-entry="${key}" data-key="${key}" ${attr}><summary>${text}</summary><p>body</p></details>`;
        const raw = '<div class="thead">title<span class="sum"><button class="modelhistory-trigger"></button><template class="modelhistory-source">models</template></span></div><div class="centries">' + row('a', 'A') + row('b', 'B') + '</div>';
        const base = api.entryInputFull(raw), before = JSON.stringify(base);
        assertLocal(!!base, 'canonical full accepted');
        const changed = (key, html) => ({ key, hash: 'rendered-row', html });
        const valid = changed('a', row('a', 'available peer A'));
        const patch = (rows, order = base.order) => ({ hash: 'next', order, rows });
        const invalid = [null, {}, { ...patch([valid]), hash: 3 }, { ...patch([valid]), order: null },
          patch([valid], ['a', 'a']), patch([valid], ['b', 'a']), patch([valid], ['a']), patch([valid], ['a', 'unknown']),
          { ...patch([valid]), rows: {} }, patch([null]), patch([{ ...valid, hash: null }]), patch([valid, valid]),
          patch([changed('unknown', row('unknown', 'x'))]), patch([changed('a', row('b', 'wrong key'))]),
          patch([changed('a', row('a', 'one') + row('a', 'two'))]), patch([changed('a', ' ' + row('a', 'x'))]),
          patch([changed('a', row('a', 'x') + 'trailing')]), patch([changed('a', '<div class="centries" data-entry="a"></div>')]),
          patch([changed('a', '<div data-entry="a"><div class="centries"></div></div>')]),
          patch([changed('a', row('a', '<!--ws-entry-row-0-->'))]), patch([changed('a', null)]),
          patch([valid, changed('unknown', row('unknown', 'x'))])];
        invalid.forEach((event, i) => {
          assertLocal(api.entryInputPatch(base, event) === null, 'malformed event rejected ' + i);
          assertLocal(JSON.stringify(base) === before, 'atomic immutable rejection ' + i);
        });
        assertLocal(api.entryInputPatch(null, patch([valid])) === null, 'missing baseline rejected');
        for (const html of ['', '<div class="centries">text</div>', '<div class="centries"><div></div></div>',
          '<div class="centries"></div><div class="centries"></div>', raw.replace(row('b', 'B'), row('a', 'duplicate')), raw + '<!--ws-entry-row-0-->']) {
          assertLocal(api.entryInputFull(html) === null, 'invalid full rejected');
        }
        const one = api.entryInputPatch(base, patch([valid]));
        const two = api.entryInputPatch(one, patch([changed('b', row('b', 'new B'))]));
        assertLocal(api.entryInputHTML(two).includes('available peer A') && api.entryInputHTML(two).includes('new B'), 'hidden inputs accumulate');
        assertLocal(JSON.stringify(base) === before && !api.entryInputHTML(base).includes('new B'), 'held inputs immutable');
        root.innerHTML = raw;
        const box = root.querySelector('.centries'), sibling = box.children[1], head = root.querySelector('.thead'), models = root.querySelector('.modelhistory-source');
        const older = doc.createElement('div'); older.setAttribute('data-entry', '0'); older.textContent = 'older history only'; box.prepend(older);
        sibling.open = true; sibling.querySelector('summary').textContent = 'client formatted B';
        const oldA = box.querySelector('[data-entry="a"]');
        assertLocal(oldA.getAttribute('data-mail-wake') === null, 'mail association initially absent');
        parses = 0;
        api.mergeEntriesRegion(root, one, false, base);
        assertLocal(parses === 1, 'stable delta parses only changed row');
        assertLocal(formatted.length === 1 && formatted[0] === box.querySelector('[data-entry="a"]'), 'only changed row formatted');
        assertLocal(box.querySelector('[data-entry="a"]') !== oldA && box.textContent.includes('available peer A'), 'same-key canonical HTML replaces despite unchanged mail-wake');
        assertLocal(box.querySelector('[data-entry="b"]') === sibling && sibling.open && box.firstElementChild === older, 'unchanged disclosure/history nodes retained');
        assertLocal(root.querySelector('.thead') === head && root.querySelector('.modelhistory-source') === models, 'stable shell nodes retained');
        assertLocal(!api.entryInputHTML(one).includes('older history only') && !api.entryInputHTML(one).includes('client formatted'), 'live DOM never becomes canonical input');
        parses = 0;
        api.mergeEntriesRegion(root, two, false, one);
        assertLocal(parses === 1 && box.firstElementChild === older, 'second delta parses its changed row alone');
        // Full old-daemon events still take gap detection and reset history.
        const full = '<div class="thead">new title</div><div class="centries">' + row('z', 'after gap') + '</div>';
        api.mergeEntriesRegion(root, full, false, two);
        assertLocal(resets === 1 && box.children.length === 1 && box.firstElementChild.getAttribute('data-entry') === 'z', 'legacy full gap resets loaded history');
        assertLocal(!root.querySelector('.modelhistory-source'), 'full shell removes model history');
        return { rejected: invalid.length, changedOnlyParsing: true, canonicalClean: true, gapReset: true };
      } finally { doc.createElement = create; }
      function assertLocal(ok, label) { if (!ok) throw Error(label); }
    }, process.env.ENTRY_ROWS_FUNCTIONS);

    const update = async (op, value) => {
      const n = events.length;
      assert.equal((await fetch(updates + '?' + new URLSearchParams({ op, value: value || '' }))).status, 204);
      await eventually(() => events.length > n, op + ' stream event');
      return events.slice(n);
    };
    const hash = () => frame.locator('#r-entries').getAttribute('data-h');
    const waitHash = h => frame.waitForFunction(h => document.querySelector('#r-entries').getAttribute('data-h') !== h, h);
    const deltaKeys = ev => { assert.equal(ev.length, 1); assert.equal(ev[0].event, 'entry-rows'); return ev[0].data.rows.map(r => r.key); };
    await frame.evaluate(() => {
      const box = document.querySelector('#r-entries .centries');
      const find = text => [...box.children].find(n => n.textContent.includes(text));
      window.entryRefs = { box, a: find('entry-row-mail-a'), b: find('entry-row-mail-b'), effect: find('entry-row-effect-marker'), sibling: box.children[3], head: document.querySelector('#r-entries .thead') };
      window.entryKeysFixture = { a: entryRefs.a.getAttribute('data-entry'), b: entryRefs.b.getAttribute('data-entry') };
    });
    const keys = await frame.evaluate(() => entryKeysFixture);
    // A reconnect first full at the current hash establishes canonical state
    // without remounting the already displayed initial entries.
    let count = events.length;
    await frame.evaluate(() => entrySockets.find(s => s.url.includes('/s/livesession/events')).close());
    await eventually(() => events.length > count, 'samehash reconnect first full');
    assert.equal(events[count].event, 'region');
    assert(await frame.evaluate(() => entryRefs.a.isConnected && entryRefs.b.isConnected && entryRefs.head.isConnected), 'samehash first full retains DOM');

    // Real Older paging: the initial server window excludes fixture mail.
    assert.equal(await frame.getByText('from the board', { exact: true }).count(), 0);
    const olderResponse = page.waitForResponse(r => new URL(r.url()).pathname === '/s/livesession/history' && r.status() === 200);
    await frame.locator('#convo').evaluate(el => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
    await olderResponse;
    await frame.getByText('from the board', { exact: true }).waitFor({ state: 'attached' });
    await frame.evaluate(() => {
      const box = document.querySelector('#r-entries .centries');
      entryRefs.older = box.firstElementChild;
      entryRefs.disclosure = [...box.children].find(n => n.matches('details.mailline') && n.textContent.includes('mail second line'));
      entryRefs.disclosure.open = true;
    });
    // Let native toggle reach the existing disclosure-state owner.
    await frame.evaluate(() => new Promise(resolve => setTimeout(resolve, 0)));
    await frame.evaluate(() => {
      const pane = document.getElementById('convo'); pane.scrollTop = Math.floor(pane.scrollHeight / 2);
      entryRefs.disclosure.querySelector('summary').focus({ preventScroll: true });
      window.readerTop = pane.scrollTop;
      window.readerNode = entryRefs.sibling;
      window.readerY = readerNode.getBoundingClientRect().top;
    });
    assert((await frame.evaluate(() => readerTop)) > 160, 'reader fixture is away from both edges');
    let oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('a', 'off')), [keys.a]);
    await waitHash(oldHash);
    assert.deepEqual(await frame.evaluate(() => ({
      aReplaced: !entryRefs.a.isConnected, bKept: entryRefs.b.isConnected,
      siblingKept: entryRefs.sibling.isConnected, olderKept: entryRefs.older.isConnected,
      open: entryRefs.disclosure.open, focus: document.activeElement === entryRefs.disclosure.querySelector('summary'),
      top: document.getElementById('convo').scrollTop === readerTop, anchor: readerNode.getBoundingClientRect().top === readerY,
      shellKept: entryRefs.head.isConnected
    })), { aReplaced: true, bKept: true, siblingKept: true, olderKept: true, open: true, focus: true, top: true, anchor: true, shellKept: true });
    assert.equal(await frame.locator('[data-entry="' + keys.a + '"]').getAttribute('data-mail-wake'), null, 'canonical replacement did not require mail-wake attribute change');
    oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('a', 'on')), [keys.a]); await waitHash(oldHash);
    oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('peer', 'Entry Peer Renamed')), [keys.a]); await waitHash(oldHash);
    assert((await frame.locator('[data-entry="' + keys.a + '"]').textContent()).includes('Entry Peer Renamed'), 'same-key peer presentation refreshes');

    const browserHidden = value => frame.evaluate(hidden => {
      window.entryTestHidden = hidden;
      if (!Object.prototype.hasOwnProperty.call(document, 'hidden')) {
        Object.defineProperty(document, 'hidden', { configurable: true, get: () => window.entryTestHidden });
      }
      document.dispatchEvent(new Event('visibilitychange'));
    }, value);
    await browserHidden(true);
    oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('a', 'off')), [keys.a]);
    assert.deepEqual(deltaKeys(await update('b', 'off')), [keys.b]);
    assert.equal(await hash(), oldHash, 'hidden gate does not touch live DOM');
    await browserHidden(false); await waitHash(oldHash);
    assert.equal(await frame.locator('[data-entry="' + keys.a + '"] .mbody').count(), 0);
    assert.equal(await frame.locator('[data-entry="' + keys.b + '"] .mbody').count(), 0, 'both hidden deltas materialize, not only last changed list');
    assert(await frame.evaluate(() => entryRefs.older.isConnected && entryRefs.sibling.isConnected && entryRefs.effect.isConnected), 'hidden updates retain history and unrelated siblings');

    // Effect/deep-link navigation keeps its existing validated target/focus
    // owner; a later conversation delta cannot steal that board position.
    const jump = frame.locator('#r-entries .wbupdate-topic a[data-wb-jump]').first();
    const target = await jump.getAttribute('data-wb-jump');
    await jump.click();
    await frame.waitForFunction(id => document.activeElement?.id === id, target);
    const boardState = await frame.evaluate(id => ({ top: document.getElementById('bcol').scrollTop, y: document.getElementById(id).getBoundingClientRect().top }), target);
    oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('b', 'on')), [keys.b]); await waitHash(oldHash);
    assert.deepEqual(await frame.evaluate(id => ({ focus: document.activeElement?.id, top: document.getElementById('bcol').scrollTop, y: document.getElementById(id).getBoundingClientRect().top }), target), { focus: target, ...boardState }, 'entry update retains effect deep-link focus and board anchor');

    // Model history uses full regions, without replacing loaded keyed history.
    oldHash = await hash();
    assert.equal((await update('model', 'entry-model-one'))[0].event, 'region'); await waitHash(oldHash);
    const modelTrigger = frame.locator('.modelhistory-trigger');
    await modelTrigger.click();
    const models = frame.locator('.modelhistory-popover');
    await models.waitFor({ state: 'visible' });
    oldHash = await hash();
    assert.equal((await update('model', 'entry-model-two'))[0].event, 'region'); await waitHash(oldHash);
    assert(await models.isVisible(), 'full shell preserves open model popover');
    assert((await models.textContent()).includes('entry-model-two'));
    assert(await frame.evaluate(() => entryRefs.older.isConnected && entryRefs.disclosure.isConnected && entryRefs.effect.isConnected), 'full shell retains loaded history and effect nodes');

    // The actual stream lifecycle rejects a malformed delta, closes its native
    // socket and reconnects for full, even with the current held hash.
    count = events.length;
    await frame.evaluate(() => {
      const socket = entrySockets.filter(s => s.url.includes('/s/livesession/events') && s.readyState === WebSocket.OPEN).at(-1);
      window.invalidSocket = socket;
      socket.dispatchEvent(new MessageEvent('message', { data: JSON.stringify({ event: 'entry-rows', data: { hash: 'bad', order: ['unknown'], rows: [] } }) }));
    });
    await eventually(() => events.length > count, 'invalid delta reconnect full');
    assert.equal(events[count].event, 'region');
    assert(await frame.evaluate(() => invalidSocket.readyState === WebSocket.CLOSED), 'invalid baseline closes old connection');
    oldHash = await hash();
    assert.deepEqual(deltaKeys(await update('a', 'on')), [keys.a]); await waitHash(oldHash);
    assert(await frame.evaluate(() => entryRefs.older.isConnected), 'repaired baseline retains old history');

    // A genuinely disjoint newest window resets loaded history and its cursor;
    // scrolling Older again must fetch the page adjacent to the new window.
    oldHash = await hash();
    assert.equal((await update('gap'))[0].event, 'region'); await waitHash(oldHash);
    assert(await frame.evaluate(() => !entryRefs.older.isConnected && !entryRefs.effect.isConnected), 'gap resets old window and loaded history');
    const historyCount = histories.length;
    const gapHistory = page.waitForResponse(r => new URL(r.url()).pathname === '/s/livesession/history' && r.status() === 200);
    await frame.locator('#convo').evaluate(el => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
    await gapHistory;
    await frame.getByText('entry-row-effect-marker', { exact: true }).waitFor({ state: 'attached' });
    assert(histories.length > historyCount, 'gap resets Older end/cursor state');
    assert.deepEqual(errors, [], 'no page errors'); assert.deepEqual(unexpected, [], 'no external network');
    console.log(JSON.stringify({ validation, transportEvents: events.map(e => e.event), historyRequests: histories.length, identity: 'retained', readerAnchor: 'retained', effectJump: 'retained', hiddenAccumulation: 'ok', reconnect: 'full recovery', pageErrors: errors }));
    await context.close();
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });

async function eventually(check, label) {
  const end = Date.now() + 10000;
  while (!await check()) { assert(Date.now() < end, label + ' timed out'); await new Promise(resolve => setTimeout(resolve, 20)); }
}
