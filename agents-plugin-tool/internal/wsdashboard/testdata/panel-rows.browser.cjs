// Production parser, merger, page and streams. Disposable loopback; no captures.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.PANEL_ROWS_BASE, updates = process.env.PANEL_ROWS_UPDATES;
for (const address of [base, updates]) {
  const u = new URL(address);
  assert(['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname) && u.port && u.port !== '29417', 'disposable loopback only');
}
(async () => {
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US', reducedMotion: 'reduce' });
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    await context.addInitScript(() => {
      const Native = window.WebSocket;
      window.panelSockets = [];
      window.WebSocket = class extends Native {
        constructor(...args) { super(...args); window.panelSockets.push(this); }
      };
    });
    const page = await context.newPage(), errors = [], events = [], unexpected = [];
    page.on('pageerror', e => errors.push(e.message));
    await context.route('**/*', route => {
      const u = new URL(route.request().url());
      if (['http:', 'https:'].includes(u.protocol) && ![new URL(base).origin, new URL(updates).origin].includes(u.origin)) {
        unexpected.push(u.href); return route.abort();
      }
      return route.continue();
    });
    page.on('websocket', socket => {
      if (!socket.url().includes('/s/livesession/events')) return;
      assert(socket.url().includes('panelrows=1'), 'session opts in');
      socket.on('framereceived', ({ payload }) => {
        const e = JSON.parse(payload.toString());
        if (e.event === 'panel-rows' || (e.event === 'region' && e.data?.name === 'panels')) events.push(e);
      });
    });
    await page.goto(base + '/s/livesession');
    const frame = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await frame.locator('#p-agenda details[data-key="agenda/plan/step"]').waitFor({ state: 'attached' });
    await eventually(() => events.length > 0, 'first full');
    assert.equal(events[0].event, 'region');
    await frame.locator('iframe.doc-frame').first().waitFor({ state: 'attached' });

    const validation = await frame.evaluate(source => {
      const doc = document, q = (selector, root) => Array.from(root.querySelectorAll(selector));
      const api = new Function('doc', 'q', source + ';return {panelInputFull,panelInputPatch,panelInputHTML,panelRowNode,mergePanelRows,panelReaderState,restorePanelReader};')(doc, q);
      const card = (key, text, attr = '') => `<details class="panel" id="${key}" ${attr}><summary>${text}</summary><details data-key="nested/${key}"><summary>nested</summary><p>body</p></details></details>`;
      const raw = card('p-todo', 'Tasks') + '\n' + card('p-agenda', 'Agenda');
      const base = api.panelInputFull(raw), before = JSON.stringify(base);
      ok(base && base.order.join(',') === 'p-todo,p-agenda', 'native cards only');
      const row = (key, html) => ({ key, hash: 'complete-render-hash', html });
      const valid = row('p-todo', card('p-todo', 'changed task'));
      const patch = (rows, order = base.order) => ({ hash: 'new-region-hash', order, rows });
      const invalid = [null, {}, { ...patch([valid]), hash: 3 }, { ...patch([valid]), order: null },
        patch([valid], ['p-todo', 'p-todo']), patch([valid], ['p-agenda', 'p-todo']), patch([valid], ['p-todo']), patch([valid], ['p-todo', 'unknown']),
        { ...patch([valid]), rows: {} }, patch([null]), patch([{ ...valid, hash: null }]), patch([valid, valid]),
        patch([row('unknown', card('unknown', 'x'))]), patch([row('p-todo', card('p-agenda', 'wrong key'))]),
        patch([row('p-todo', card('p-todo', 'one') + card('p-todo', 'two'))]), patch([row('p-todo', ' ' + card('p-todo', 'x'))]),
        patch([row('p-todo', card('p-todo', 'x') + 'trailing')]), patch([row('p-todo', '<div class="panel" id="p-todo"></div>')]),
        patch([row('p-todo', '<details class="panel" id="p-todo"><details class="panel" id="p-inner"></details></details>')]),
        patch([row('p-todo', card('p-todo', '<!--ws-panel-row-0-->'))]), patch([row('p-todo', null)]),
        patch([valid, row('p-agenda', '<section id="p-agenda">invalid second replacement</section>')]),
        patch([row('p-todo', '<details id="p-todo"></details>')]), patch([row('p-todo', card('p-todo', 'x') + '<!--extra-->')])];
      invalid.forEach((event, i) => {
        ok(api.panelInputPatch(base, event) === null, 'malformed rejected ' + i);
        ok(JSON.stringify(base) === before, 'immutable rejection ' + i);
      });
      ok(api.panelInputPatch(null, patch([valid])) === null, 'missing baseline');
      for (const html of [null, 'text', '<section>' + raw + '</section>', card('p-todo', 'a') + card('p-todo', 'b'),
        '<details class="panel" id="p-"></details>', raw + '<!--ws-panel-row-0-->', '<details class="panel" id="p-a"><div class="panel"></div></details>']) {
        ok(api.panelInputFull(html) === null, 'invalid full');
      }
      const empty = api.panelInputFull('');
      ok(empty && empty.order.length === 0 && api.panelInputHTML(empty) === '', 'empty full');
      const one = api.panelInputPatch(base, patch([valid]));
      const two = api.panelInputPatch(one, patch([row('p-agenda', card('p-agenda', 'changed agenda'))]));
      ok(api.panelInputHTML(two).includes('changed task') && api.panelInputHTML(two).includes('changed agenda'), 'hidden accumulation');
      ok(JSON.stringify(base) === before, 'old canonical retained');
      const root = doc.createElement('div'); root.innerHTML = raw;
      const sibling = root.children[1], descendant = sibling.querySelector('p');
      sibling.open = true; descendant.textContent = 'client formatting only';
      let parses = 0;
      const create = doc.createElement;
      doc.createElement = function(name, ...args) { if (name === 'div') parses++; return create.call(doc, name, ...args); };
      try {
        const inserted = api.mergePanelRows(root, base, one);
        ok(parses === 1 && inserted.length === 1, 'parse changed card only');
        ok(root.children[1] === sibling && sibling.querySelector('p') === descendant && sibling.open, 'unchanged card and descendants retained');
        ok(!api.panelInputHTML(one).includes('client formatting'), 'DOM is not canonical');
        // Bypass the event parser to prove the merger itself validates every
        // replacement before making any live mutation.
        const refs = Array.from(root.children), oldHTML = root.innerHTML;
        const forged = { ...two, rows: { ...two.rows, 'p-todo': { html: card('p-todo', 'would replace first') }, 'p-agenda': { html: '<div>invalid last</div>' } } };
        ok(api.mergePanelRows(root, one, forged) === null && root.innerHTML === oldHTML && refs.every((n, i) => root.children[i] === n), 'all replacements validate atomically');
        for (const next of [{ ...two, shell: 'different' }, { ...two, order: [...two.order].reverse() }]) {
          ok(api.mergePanelRows(root, one, next) === null && root.innerHTML === oldHTML, 'shell/order fallback is atomic');
        }
        root.appendChild(doc.createElement('section'));
        ok(api.mergePanelRows(root, one, two) === null && refs.every((n, i) => root.children[i] === n), 'uncertain live container refused');
      } finally { doc.createElement = create; }
      return { rejected: invalid.length, changedOnlyParsing: true, atomicMerge: true, canonicalClean: true };
      function ok(value, label) { if (!value) throw Error(label); }
    }, process.env.PANEL_ROWS_FUNCTIONS);

    const hash = () => frame.locator('#r-panels').getAttribute('data-h');
    const waitHash = h => frame.waitForFunction(h => document.getElementById('r-panels').getAttribute('data-h') !== h, h);
    const update = async (op, value) => {
      const count = events.length;
      assert.equal((await fetch(updates + '?' + new URLSearchParams({ op, value: value || '' }))).status, 204);
      await eventually(() => events.length > count, op + ' event');
      return events.slice(count);
    };
    const delta = (events, keys) => {
      assert.equal(events.length, 1); assert.equal(events[0].event, 'panel-rows');
      assert.deepEqual(events[0].data.rows.map(r => r.key), keys);
    };
    await frame.locator('#drhandle').click();
    await frame.waitForFunction(() => document.getElementById('drhandle').getAttribute('aria-expanded') === 'true');
    const step = frame.locator('#p-agenda details[data-key="agenda/plan/step"]');
    assert.equal(await step.evaluate(n => n.open), false, 'nested disclosure initially closed');
    // This real summary click is the required regression: open a closed
    // nested fold, then change that same agenda transport card.
    await step.locator(':scope > summary').click();
    await frame.waitForFunction(() => document.querySelector('[data-key="agenda/plan/step"]').open);
    const other = frame.locator('#p-agenda details[data-key="agenda/extra00"]');
    assert(await other.evaluate(n => n.open), 'another nested disclosure initially open');
    await other.locator(':scope > summary').click();
    await frame.waitForFunction(() => !document.querySelector('[data-key="agenda/extra00"]').open);
    await frame.evaluate(() => {
      const agenda = document.getElementById('p-agenda'), todo = document.getElementById('p-todo');
      const summary = document.querySelector('[data-key="agenda/plan/step"] > summary');
      agenda.scrollTop = 85; todo.scrollTop = 60; summary.focus({ preventScroll: true });
      window.panelRefs = { agenda, todo, todoChild: todo.querySelector('li'), tickets: document.getElementById('p-tickets'),
        entries: document.getElementById('r-entries').firstElementChild, board: document.getElementById('r-board').firstElementChild,
        frames: Array.from(document.querySelectorAll('iframe.doc-frame')), sheet: document.getElementById('drsheet'),
        agendaTop: agenda.scrollTop, todoTop: todo.scrollTop, agendaY: agenda.getBoundingClientRect().top };
      panelRefs.windows = panelRefs.frames.map(n => n.contentWindow);
      window.panelFrameLoads = 0;
      panelRefs.frames.forEach(n => n.addEventListener('load', () => panelFrameLoads++));
    });
    assert((await frame.evaluate(() => panelRefs.agendaTop)) > 0, 'card genuinely scrolls');
    let oldHash = await hash();
    delta(await update('agenda', 'changed agenda'), ['p-agenda']); await waitHash(oldHash);
    assert.deepEqual(await frame.evaluate(() => {
      const agenda = document.getElementById('p-agenda');
      return { agendaReplaced: !panelRefs.agenda.isConnected, todoKept: document.getElementById('p-todo') === panelRefs.todo,
        descendantKept: panelRefs.todoChild.isConnected, ticketsKept: panelRefs.tickets.isConnected,
        openFold: document.querySelector('[data-key="agenda/plan/step"]').open,
        closedFold: !document.querySelector('[data-key="agenda/extra00"]').open,
        focus: document.activeElement === document.querySelector('[data-key="agenda/plan/step"] > summary'),
        scroll: agenda.scrollTop === panelRefs.agendaTop, siblingScroll: panelRefs.todo.scrollTop === panelRefs.todoTop,
        geometry: agenda.getBoundingClientRect().top === panelRefs.agendaY,
        nativeOpen: Array.from(document.querySelectorAll('#r-panels > details.panel')).every(n => n.open),
        pageKept: panelRefs.entries.isConnected && panelRefs.board.isConnected && panelRefs.sheet.isConnected,
        framesKept: panelRefs.frames.length > 0 && panelFrameLoads === 0 && panelRefs.frames.every((n, i) => n.isConnected && n.contentWindow === panelRefs.windows[i]) };
    }), { agendaReplaced: true, todoKept: true, descendantKept: true, ticketsKept: true, openFold: true, closedFold: true, focus: true,
      scroll: true, siblingScroll: true, geometry: true, nativeOpen: true, pageKept: true, framesKept: true });

    await frame.evaluate(() => {
      const todo = document.getElementById('p-todo');
      todo.querySelector(':scope > summary').focus({ preventScroll: true });
      window.todoReader = { old: todo, top: todo.scrollTop, y: todo.getBoundingClientRect().top, agenda: document.getElementById('p-agenda') };
    });
    oldHash = await hash(); delta(await update('todo', 'changed tasks'), ['p-todo']); await waitHash(oldHash);
    assert(await frame.evaluate(() => {
      const todo = document.getElementById('p-todo');
      return !todoReader.old.isConnected && todo.scrollTop === todoReader.top && todo.getBoundingClientRect().top === todoReader.y &&
        document.activeElement === todo.querySelector(':scope > summary') && todoReader.agenda === document.getElementById('p-agenda');
    }), 'a different changed card retains its own focus and scroll without replacing agenda');

    const shown = async value => {
      // Registered after production's shell listener: observing this message
      // acknowledges that the hidden gate has already processed it.
      await frame.evaluate(() => {
        window.panelShownAck = null;
        window.addEventListener('message', function ack(e) {
          if (e.source !== parent || e.origin !== location.origin || e.data?.wsShell !== 'shown') return;
          window.panelShownAck = e.data.shown;
          window.removeEventListener('message', ack);
        });
      });
      await page.evaluate(shown => document.querySelector('iframe.sframe.on').contentWindow.postMessage({ wsShell: 'shown', shown }, location.origin), value);
      await frame.waitForFunction(shown => window.panelShownAck === shown, value);
    };
    await shown(false);
    oldHash = await hash();
    delta(await update('agenda', 'hidden agenda'), ['p-agenda']);
    delta(await update('todo', 'hidden task'), ['p-todo']);
    assert.equal(await hash(), oldHash, 'hidden gate holds DOM');
    await shown(true); await waitHash(oldHash);
    assert((await frame.locator('#p-agenda').textContent()).includes('hidden agenda'));
    assert((await frame.locator('#p-todo').textContent()).includes('hidden task'), 'hidden deltas accumulated across card kinds');
    assert(await step.evaluate(n => n.open), 'hidden replacement retains user-open fold');
    assert.equal(await other.evaluate(n => n.open), false, 'hidden replacement retains user-closed fold');

    // A shell change uses a real full event. Canonical equal card HTML must
    // not be confused with live drawer styling or user disclosure attributes.
    oldHash = await hash();
    assert.equal((await update('fallback'))[0].event, 'region'); await waitHash(oldHash);
    assert(await step.evaluate(n => n.open), 'full fallback retains nested open state');
    assert.equal(await other.evaluate(n => n.open), false, 'full fallback retains nested closed state');
    assert(await frame.evaluate(() => panelRefs.board.isConnected && panelRefs.entries.isConnected && panelFrameLoads === 0 && panelRefs.frames.every((n, i) => n.isConnected && n.contentWindow === panelRefs.windows[i])), 'unaffected page and frames survive full panel fallback');
    await frame.locator('#drfold').click();
    await frame.waitForFunction(() => document.getElementById('drhandle').getAttribute('aria-expanded') === 'false');
    oldHash = await hash();
    // Returning from the injected shell also takes a full event; the next
    // stable-shell update must resume changed-card transport.
    assert.equal((await update('agenda', 'closed drawer agenda'))[0].event, 'region'); await waitHash(oldHash);
    assert(await frame.locator('#drhandle .tdot').evaluate(n => n.classList.contains('on')), 'closed drawer marks panel update');
    await frame.locator('#drhandle').click();
    await frame.waitForFunction(() => document.getElementById('drhandle').getAttribute('aria-expanded') === 'true');
    assert.equal(await frame.locator('#drhandle .tdot').evaluate(n => n.classList.contains('on')), true, 'pending todo badge remains independent of swap badge');
    assert(await step.evaluate(n => n.open), 'drawer lifecycle retains nested fold');

    const count = events.length;
    await frame.evaluate(() => {
      const socket = panelSockets.filter(s => s.url.includes('/s/livesession/events') && s.readyState === WebSocket.OPEN).at(-1);
      window.invalidPanelSocket = socket;
      window.reconnectCard = document.getElementById('p-agenda');
      socket.dispatchEvent(new MessageEvent('message', { data: JSON.stringify({ event: 'panel-rows', data: { hash: 'bad', order: ['unknown'], rows: [] } }) }));
    });
    await eventually(() => events.length > count, 'invalid event reconnect full');
    assert.equal(events[count].event, 'region');
    assert(await frame.evaluate(() => invalidPanelSocket.readyState === WebSocket.CLOSED && reconnectCard === document.getElementById('p-agenda')), 'invalid closes and same-hash first full does not remount');
    oldHash = await hash(); delta(await update('agenda', 'repaired agenda'), ['p-agenda']); await waitHash(oldHash);
    assert(await step.evaluate(n => n.open), 'recovered delta retains reader fold');
    assert.equal(await other.evaluate(n => n.open), false);
    await frame.locator('#p-agenda > summary').click();
    assert(await frame.locator('#p-agenda').evaluate(n => n.open), 'top-level summary is deliberately not a user toggle');
    // Disturb native state explicitly, rather than inventing a user-close
    // affordance that the drawer intentionally prevents.
    await frame.locator('#p-agenda').evaluate(n => new Promise(resolve => { n.addEventListener('toggle', resolve, { once: true }); n.open = false; }));
    assert.equal(await frame.locator('#p-agenda').evaluate(n => n.open), false, 'native state disturbed before swap');
    oldHash = await hash(); delta(await update('agenda', 'native panel policy'), ['p-agenda']); await waitHash(oldHash);
    assert(await frame.locator('#p-agenda').evaluate(n => n.open), 'drawerSync overrides top-level closed state');
    assert(await step.evaluate(n => n.open), 'top-level policy does not override nested user-open fold');
    assert.equal(await other.evaluate(n => n.open), false, 'top-level policy does not override nested user-closed fold');
    assert.deepEqual(errors, [], 'no page errors'); assert.deepEqual(unexpected, [], 'no external requests');
    console.log(JSON.stringify({ validation, events: events.map(e => e.event), nestedReaderFolds: 'retained across changed agenda card', focusAndCardScroll: 'retained', hiddenAccumulation: 'both cards', reconnect: 'mandatory full', pageErrors: errors }));
    await context.close();
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
async function eventually(check, label) {
  const end = Date.now() + 10000;
  while (!await check()) { assert(Date.now() < end, label + ' timed out'); await new Promise(resolve => setTimeout(resolve, 20)); }
}
