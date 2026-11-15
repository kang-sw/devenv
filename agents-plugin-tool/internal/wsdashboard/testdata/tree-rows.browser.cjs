// Disposable Chromium: real stream events and lifecycle, plus the unmodified
// production input validator/merger in a detached DOM for hostile payloads.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.TREE_ROWS_BASE;

(async () => {
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US' });
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: base }]);
    const page = await context.newPage(), errors = [], events = [];
    page.on('pageerror', e => errors.push(e.message));
    page.on('websocket', socket => {
      if (!socket.url().includes('/s/livesession/events')) return;
      assert(socket.url().includes('treerows=1'), 'session opts into tree rows');
      socket.on('framereceived', ({ payload }) => {
        const e = JSON.parse(payload.toString());
        if (e.event === 'tree-rows' || (e.event === 'region' && e.data?.name === 'tree')) events.push(e);
      });
    });
    await page.goto(base + '/s/livesession');
    const frame = await (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
    const parent = '#r-tree details[data-key="agent-parent"]';
    await frame.locator('#sbadge:not([hidden])').waitFor();
    await frame.waitForFunction(() => document.querySelector('#r-tree')?.getAttribute('data-h'));
    // Wait for this connection's mandatory full, not merely the initial page HTML.
    await assertEventually(() => events.length > 0, 'initial full event');
    assert.equal(events[0].event, 'region');

    const validation = await frame.evaluate(source => {
      const doc = document, q = (selector, root) => Array.from(root.querySelectorAll(selector));
      const api = new Function('doc', 'q', source + ';return {treeInputFull,treeInputPatch,treeInputHTML,mergeTreeRows};')(doc, q);
      const row = (key, text, kids = '') => `<div class="agent"><details data-key="agent-${key}"><summary>${text}</summary></details>${kids ? '<div class="kids">' + kids + '</div>' : ''}</div>`;
      const raw = '<section>' + row('a', 'A', row('b', 'B') + row('c', 'C')) + row('d', 'D') + '</section>';
      const baseline = api.treeInputFull(raw), before = JSON.stringify(baseline);
      if (!baseline) throw Error('no detached baseline');
      const patch = (rows, order = baseline.order) => ({ hash: 'next', order, rows });
      const changed = (key, html) => ({ key, hash: 'row-hash', html });
      const a = baseline.rows['agent-a'].html, b = baseline.rows['agent-b'].html;
      const valid = changed('agent-b', b.replace('>B<', '>new B<'));
      const invalid = [
        null, {}, { ...patch([valid]), hash: 4 }, { ...patch([valid]), order: null },
        patch([valid], ['agent-a', 'agent-b', 'agent-b', 'agent-d']),
        patch([valid], ['agent-a', 'agent-b', 'unknown', 'agent-d']),
        patch([valid], ['agent-d', 'agent-a', 'agent-b', 'agent-c']),
        patch([valid], baseline.order.slice(1)), { ...patch([valid]), rows: {} },
        patch([null]), patch([{ ...valid, hash: null }]), patch([valid, valid]),
        patch([changed('unknown', b)]), patch([changed('agent-b', row('c', 'wrong key'))]),
        patch([changed('agent-b', b + b)]), patch([changed('agent-b', ' ' + b)]),
        patch([changed('agent-b', b + 'trailing text')]),
        patch([changed('agent-b', row('b', 'nested', row('z', 'Z')))]),
        patch([changed('agent-a', a.replace('<!--ws-tree-row-1-->', ''))]),
        patch([changed('agent-a', a.replace('<!--ws-tree-row-1-->', '<!--ws-tree-row-2-->'))]),
        patch([changed('agent-a', a.replace('<!--ws-tree-row-1-->', '<!--ws-tree-row-99-->'))]),
        patch([changed('agent-a', a.replace('<!--ws-tree-row-1-->', '<!--ws-tree-row-2--><!--ws-tree-row-1-->'))]),
        patch([changed('agent-b', b.replace('</div>', '<!--ws-tree-row-0--></div>'))]),
        // Validation is atomic even if a good row precedes a bad one.
        patch([valid, changed('unknown', b)])
      ];
      invalid.forEach((ev, i) => {
        if (api.treeInputPatch(baseline, ev) !== null) throw Error('accepted malformed event ' + i);
        if (JSON.stringify(baseline) !== before) throw Error('invalid event mutated baseline ' + i);
      });
      if (api.treeInputPatch(null, patch([valid])) !== null) throw Error('accepted missing baseline');
      if (api.treeInputFull(raw + row('a', 'duplicate')) !== null) throw Error('accepted duplicate full keys');
      if (api.treeInputFull(raw + '<!--ws-tree-row-0-->') !== null) throw Error('accepted full private slot');
      const next = api.treeInputPatch(baseline, patch([valid]));
      const newer = api.treeInputPatch(next, patch([changed('agent-a', a.replace('>A<', '>new A<'))]));
      if (!newer || !api.treeInputHTML(newer).includes('new B') || !api.treeInputHTML(newer).includes('new A')) throw Error('lost accumulated child/parent update');
      if (api.treeInputHTML(newer).includes('<!--ws-tree-row-')) throw Error('materialized slot leaked');
      // Formatting a live node must never become the next canonical input.
      const root = doc.createElement('div'); root.innerHTML = raw;
      const keptChild = root.querySelector('[data-key="agent-b"]').parentElement;
      keptChild.setAttribute('data-client-formatted', 'yes');
      keptChild.querySelector('summary').textContent = 'live-only formatting';
      const parentOnly = api.treeInputPatch(baseline, patch([changed('agent-a', a.replace('>A<', '>parent only<'))]));
      if (!api.mergeTreeRows(root, baseline, parentOnly)) throw Error('parent did not merge');
      if (root.querySelector('[data-key="agent-b"]').parentElement !== keptChild) throw Error('parent lost retained child');
      if (api.treeInputHTML(parentOnly).includes('data-client-formatted') || api.treeInputHTML(parentOnly).includes('live-only formatting') || JSON.stringify(baseline) !== before) throw Error('formatted DOM contaminated canonical input');
      const walker = doc.createTreeWalker(root, NodeFilter.SHOW_COMMENT);
      while (walker.nextNode()) if (walker.currentNode.data.startsWith('ws-tree-row-')) throw Error('live slot leaked');
      const originalRoot = root.innerHTML;
      const incompatible = { ...parentOnly, order: parentOnly.order.slice().reverse() };
      if (api.mergeTreeRows(root, parentOnly, incompatible) !== null || root.innerHTML !== originalRoot) throw Error('incompatible merge mutated DOM');
      return { rejected: invalid.length, slotsRemoved: true, canonicalClean: true };
    }, process.env.TREE_ROWS_FUNCTIONS);

    await frame.click('#sbadge');
    await frame.click(parent + ' > summary');
    await frame.click(parent + ' > .afull');
    await frame.evaluate(() => {
      const find = key => document.querySelector(`#r-tree details[data-key="agent-${key}"]`).parentElement;
      window.treeRefs = { parent: find('parent'), child: find('child'), other: find('other'), lead: find('lead') };
      const child = treeRefs.child.querySelector('summary'); child.focus({ preventScroll: true });
      const pane = document.getElementById('r-tree'); pane.scrollTop = 100;
      window.treeScroll = pane.scrollTop;
    });
    assert((await frame.evaluate(() => treeScroll)) > 0, 'fixture scrolls its tree pane');
    const expanded = async () => frame.evaluate(() => {
      const d = document.querySelector('#r-tree details[data-key="agent-parent"]');
      return { open: d.open, full: d.classList.contains('d-full'), batches: [...d.querySelectorAll(':scope > .brows > li')].filter(n => n.offsetParent).length };
    });
    assert.deepEqual(await expanded(), { open: true, full: true, batches: 7 });
    const update = async (op, text) => {
      const start = events.length;
      const response = await fetch(process.env.TREE_ROWS_UPDATES + '?' + new URLSearchParams({ op, text: text || '' }));
      assert.equal(response.status, 204);
      await assertEventually(() => events.length > start, op + ' tree event');
      return events.slice(start);
    };
    const waitText = text => frame.waitForFunction(text => document.getElementById('r-tree').textContent.includes(text), text);
    const identities = () => frame.evaluate(() => {
      const find = key => document.querySelector(`#r-tree details[data-key="agent-${key}"]`).parentElement;
      return Object.fromEntries(Object.keys(treeRefs).map(k => [k, find(k) === treeRefs[k]]));
    });
    const deltaKeys = ev => {
      assert.equal(ev.length, 1, 'one tree update'); assert.equal(ev[0].event, 'tree-rows');
      return ev[0].data.rows.map(r => r.key);
    };
    assert.deepEqual(deltaKeys(await update('parent', 'parent-one')), ['agent-parent']);
    await waitText('parent-one');
    assert.deepEqual(await identities(), { parent: false, child: true, other: true, lead: true }, 'changed parent keeps unchanged child and siblings');
    assert.deepEqual(await expanded(), { open: true, full: true, batches: 7 }, 'expanded/full survive keyed parent replacement');
    assert.deepEqual(await frame.evaluate(() => ({ focus: document.activeElement === treeRefs.child.querySelector('summary'), scroll: document.getElementById('r-tree').scrollTop === treeScroll, open: !document.getElementById('agpop').hidden })), { focus: true, scroll: true, open: true }, 'focus, tree scroll and popover stay');

    await frame.evaluate(() => { window.latestParent = document.querySelector('#r-tree details[data-key="agent-parent"]').parentElement; });
    assert.deepEqual(deltaKeys(await update('child', 'child-one')), ['agent-child']);
    await waitText('child-one');
    assert.deepEqual(await identities(), { parent: false, child: false, other: true, lead: true });
    assert(await frame.evaluate(() => latestParent === document.querySelector('#r-tree details[data-key="agent-parent"]').parentElement), 'child-only edit retains its parent node');
    assert.deepEqual(deltaKeys(await update('both', 'both-two')), ['agent-parent', 'agent-child']);
    await waitText('both-two');
    assert.deepEqual(await frame.evaluate(() => ['parent', 'child'].map(key => document.querySelector(`#r-tree details[data-key="agent-${key}"] .acap`).textContent)), ['both-two', 'both-two'], 'simultaneous parent and child edits both materialize');
    assert.equal(await frame.locator('#r-tree details[data-key="agent-parent"] ~ .kids details[data-key="agent-child"]').count(), 1, 'changed child remains nested exactly once');
    assert.deepEqual(await expanded(), { open: true, full: true, batches: 7 }, 'simultaneous replacements retain full disclosure');

    // Pin/close controls are outside the replaced region. An ordinary delta
    // keeps their nodes and state; Esc/outside click still follow the pin.
    await frame.click('#agpin');
    assert.deepEqual(deltaKeys(await update('parent', 'pinned-three')), ['agent-parent']);
    await waitText('pinned-three');
    await page.keyboard.press('Escape');
    assert.equal(await frame.locator('#agpin').getAttribute('aria-pressed'), 'true');
    assert(await frame.isVisible('#agpop'), 'pin survives row updates and Esc');
    await frame.click('#agclose');
    assert.deepEqual(deltaKeys(await update('child', 'closed-four')), ['agent-child']);
    await waitText('closed-four');
    assert(await frame.locator('#agpop').isHidden(), 'closed popover stays closed while DOM updates');
    await frame.click('#sbadge');
    assert(await frame.locator('#agpop').textContent().then(t => t.includes('closed-four')), 'reopening shows latest data');

    // Drive the same validated shell-to-frame message used for an inactive
    // cached frame. The stream remains live, but no hidden update touches DOM.
    const shown = value => page.evaluate(shown => document.querySelector('iframe.sframe.on').contentWindow.postMessage({ wsShell: 'shown', shown }, location.origin), value);
    await shown(false);
    await frame.evaluate(() => new Promise(resolve => setTimeout(resolve, 0)));
    const oldHash = await frame.locator('#r-tree').getAttribute('data-h');
    assert.deepEqual(deltaKeys(await update('parent', 'hidden-parent')), ['agent-parent']);
    assert.deepEqual(deltaKeys(await update('child', 'hidden-child')), ['agent-child']);
    assert.equal(await frame.locator('#r-tree').getAttribute('data-h'), oldHash, 'hidden frame did not swap');
    assert(!(await frame.locator('#r-tree').textContent()).includes('hidden-parent'));
    await shown(true);
    await waitText('hidden-child');
    assert((await frame.locator('#r-tree').textContent()).includes('hidden-parent'), 'hidden parent and child accumulate');
    assert.deepEqual(await identities(), { parent: false, child: false, other: true, lead: true }, 'hidden updates keep unrelated nodes');

    // Permission waits still reach the real title-count lifecycle. Clear the
    // wait too: a stale count after a row replacement is a regression.
    await update('permission');
    await frame.waitForFunction(() => /^\(1\) /.test(document.title));
    assert.equal(await frame.locator('#r-tree .phase.permission').count(), 1);
    await update('clear');
    await frame.waitForFunction(() => !/^\(\d+\) /.test(document.title));
    assert.equal(await frame.locator('#r-tree .phase.permission').count(), 0);

    // Shell change deliberately takes the old full-swap path and must retain
    // open/full state, pin and pane scroll just as the row delta path does.
    await frame.click('#agpin');
    const scroll = await frame.evaluate(() => { window.pinNode = document.getElementById('agpin'); const el = document.getElementById('r-tree'); el.scrollTop = 80; return el.scrollTop; });
    const fallback = await update('shell', 'fallback-five');
    assert.equal(fallback.length, 1); assert.equal(fallback[0].event, 'region');
    await waitText('fallback-five');
    assert.deepEqual(await expanded(), { open: true, full: true, batches: 7 }, 'full shell fallback preserves disclosures');
    assert.equal(await frame.locator('#agpin').getAttribute('aria-pressed'), 'true');
    assert(await frame.evaluate(() => document.getElementById('agpin') === pinNode && document.activeElement === pinNode), 'full fallback retains external control identity and focus');
    assert(await frame.isVisible('#agpop'));
    assert.equal(await frame.evaluate(() => document.getElementById('r-tree').scrollTop), scroll);
    assert(await frame.locator('#r-tree .thead .bgt').count(), 'new shell chip rendered');
    const slots = await frame.evaluate(() => { const walk = document.createTreeWalker(document.getElementById('r-tree'), NodeFilter.SHOW_COMMENT), found = []; while (walk.nextNode()) if (walk.currentNode.data.startsWith('ws-tree-row-')) found.push(walk.currentNode.data); return found; });
    assert.deepEqual(slots, [], 'no private slots in final DOM');
    assert.deepEqual(errors, [], 'no page errors');
    console.log(JSON.stringify({ validation, transportEvents: events.map(e => e.event), identity: 'retained', lifecycle: 'ok', permissionCounts: 'ok', hiddenAccumulation: 'ok', pageErrors: errors }));
    await context.close();
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });

async function assertEventually(test, label) {
  const end = Date.now() + 10000;
  while (!test()) { assert(Date.now() < end, label + ' timed out'); await new Promise(resolve => setTimeout(resolve, 20)); }
}
