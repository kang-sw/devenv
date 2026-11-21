// args: page-url [shots-dir]. The approved panel-first history: write order,
// an earlier version folded in place and expandable with its own payload,
// the link menu's latest/exact copy targets, and #eN / #pN arrival.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const url = process.argv[2], shots = process.argv[3] || '';
const topic = '.topic[data-topic="auth-profile"]';
const errors = [];
async function shot(p, name) {
  if (!shots) return;
  await p.mouse.move(0, 0); // no hover band in a capture
  await p.locator(topic).screenshot({ path: path.join(shots, name + '.png') });
}
async function open(p, hash = '') {
  await p.goto(url + hash);
  // The dashboard mounts body frames and sizes them from their messages.
  await p.evaluate(() => {
    function mount(root) {
      root.querySelectorAll('div.doc-frame[data-srcdoc]').forEach(p => {
        for (let n = p.parentElement; n; n = n.parentElement) if (n.tagName === 'DETAILS' && !n.open) return;
        const f = document.createElement('iframe');
        for (const a of p.attributes) if (a.name !== 'data-sandbox') f.setAttribute(a.name, a.value);
        f.setAttribute('sandbox', p.dataset.sandbox);
        f.srcdoc = p.dataset.srcdoc;
        p.replaceWith(f);
      });
    }
    const previous = window.wbBindBoard;
    window.wbBindBoard = root => { previous(root); mount(root); };
    document.addEventListener('toggle', e => { if (e.target.open) mount(e.target); }, true);
    window.addEventListener('message', e => {
      if (!e.data || e.data.wbDoc !== 'height') return;
      const f = [...document.querySelectorAll('iframe.doc-frame')].find(f => f.contentWindow === e.source);
      if (f) f.style.height = e.data.height + 'px';
    });
    mount(document);
    // The dashboard owns fragment routing; exercise its shared loader here.
    document.addEventListener('click', e => {
      const a = e.target.closest('a[href^="#"]');
      if (!a) return;
      const id = a.getAttribute('href').slice(1);
      if (!/^(?:e|p)[1-9][0-9]*$/.test(id)) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      window.wbHistory.loadTarget(document, id);
    }, true);
  });
  if (hash) await p.evaluate(id => window.wbHistory.loadTarget(document, id), hash.slice(1));
  await p.waitForTimeout(500);
  // Copying goes through execCommand; record what it would copy.
  await p.evaluate(() => {
    document.execCommand = c => { window.copied = c === 'copy' && document.activeElement ? document.activeElement.value : null; return true; };
  });
}
// The page opens on the latest batch; load the earlier rows like a reader.
async function loadAll(p) {
  for (let i = 0; i < 10; i++) {
    const up = p.locator(`${topic} .history-gap [data-load="up"]:not([hidden])`);
    if (!await up.count()) return;
    await up.first().click();
    await p.waitForTimeout(150);
  }
  assert.fail('history gaps never finished loading');
}
const rows = p => p.evaluate(sel => [...document.querySelector(sel).querySelectorAll('.wb-history article.wb-entry')].map(a => ({
  id: a.id, kind: a.getAttribute('data-kind'), historical: a.classList.contains('historical'),
  open: !!(a.querySelector(':scope > .entry-panel') || {}).open,
})), topic);
const menu = p => p.evaluate(() => [...document.querySelectorAll('.pfmenu .pfmi')].map(b => b.textContent.replace(/\s+/g, ' ').trim()));
// A new tab each time: the page restores its own folds per tab.
async function tab(b, width) {
  const p = await b.newPage({ viewport: { width, height: 900 }, colorScheme: 'dark' });
  p.on('pageerror', e => errors.push(e.message));
  return p;
}
(async () => {
  const b = await chromium.launch();
  try {
    for (const width of [806, 420]) {
      let p = await tab(b, width);
      await open(p);
      await loadAll(p);
      const all = await rows(p), panels = all.filter(r => r.kind === 'panel');
      console.log(width, JSON.stringify(all));
      assert.deepEqual(all.filter(r => r.kind !== 'system').map(r => r.kind), ['agent', 'panel', 'agent', 'agent', 'panel', 'agent'], 'one write-ordered line');
      assert.equal(panels.length, 2);
      const [v1, v2] = panels;
      assert(v1.historical && !v1.open && !v2.historical && v2.open, 'the earlier version folds in place; the latest stays open');
      const meta = await p.evaluate(([a, b]) => {
        const q = (id, s) => document.getElementById(id).querySelector(s);
        return {
          ver: [q(a, '.pfver').textContent, q(b, '.pfver').textContent],
          newer: [q(a, '.pfnewer').textContent, q(a, '.pfnewer').getAttribute('href')],
          exact: q(a, '.pfexact').getAttribute('href'), latest: q(b, '.mref').getAttribute('href'),
          prev: [q(b, '.pfprev a').textContent, q(b, '.pfprev a').getAttribute('href')],
          name: q(a, '.pfname').textContent, events: document.body.textContent.includes('re-posted'),
        };
      }, [v1.id, v2.id]);
      assert.deepEqual(meta, { ver: ['v1', 'v2'], newer: ['→ v2 · 09:55', '#' + v2.id], exact: '#' + v1.id, latest: '#p1', prev: ['이전 버전 v1 · 09:05 ↑', '#' + v1.id], name: 'implementation', events: false });
      await shot(p, `mixed-dark-${width}`);

      // Expanding the earlier version recovers its own payload in place.
      await p.locator(`#${v1.id} .entry-recap`).click();
      await p.waitForTimeout(200);
      const old = await p.evaluate(id => {
        const a = document.getElementById(id), d = a.querySelector('.entry-panel');
        return { open: d.open, note: a.querySelector('.pfoldnote').textContent.replace(/\s+/g, ' ').trim() };
      }, v1.id);
      old.body = await p.frameLocator(`#${v1.id} iframe.doc-frame`).locator('.doc-body').innerText();
      assert(old.open && old.note === '이전 버전 v1 · 09:05의 내용입니다. 최신은 v2 · 09:55 ↓' && old.body.includes('생성 UI: 대기'), JSON.stringify(old));
      await shot(p, `mixed-expanded-dark-${width}`);
      await p.locator(`#${v1.id} .entry-recap`).click();

      // The link menu copies pN or the exact eN; the panel never toggles.
      await p.locator(`#${v2.id} .pflink`).click();
      assert.deepEqual(await menu(p), ['최신 패널 #p1항상 가장 새 버전으로 열림 · 지금 v2', `이 버전 #${v2.id}v2 · 09:55 그대로 열림`]);
      await shot(p, `links-latest-dark-${width}`);
      await p.locator('.pfmenu .pfmi').first().click();
      assert((await p.evaluate(() => window.copied)).endsWith('#p1'));
      assert.equal(await p.locator('.pfmenu .pfmi .pfd').first().textContent(), '복사됨');
      await p.waitForTimeout(900);
      assert.equal(await p.locator('.pfmenu').count(), 0, 'the menu closes after copying');
      await p.locator(`#${v1.id} .pflink`).click();
      assert.deepEqual(await menu(p), ['최신 패널 #p1지금은 v2 · 09:55가 열림', `이 버전 #${v1.id}v1 · 09:05 그대로 열림`]);
      await shot(p, `links-old-dark-${width}`);
      await p.locator('.pfmenu .pfmi').nth(1).click();
      assert((await p.evaluate(() => window.copied)).endsWith('#' + v1.id));
      assert.equal((await rows(p)).find(r => r.id === v1.id).open, false, 'the link button does not toggle its panel');
      await p.locator(`#${v1.id} .pflink`).click();
      await p.keyboard.press('Escape');
      assert.equal(await p.locator('.pfmenu').count(), 0, 'Escape closes the menu');

      // Arrival: #eN opens that version in place; #pN the latest. Either flashes.
      await p.close(); p = await tab(b, width);
      await open(p, '#' + v1.id);
      let arrived = await p.evaluate(id => { const d = document.getElementById(id).querySelector('.entry-panel'); return [d.open, d.classList.contains('flash')]; }, v1.id);
      assert.deepEqual(arrived, [true, true], 'arrival on the exact version');
      await shot(p, `arrive-exact-dark-${width}`);
      await p.locator(`#${v1.id} .pfnewer`).click();
      await p.waitForTimeout(200);
      arrived = await p.evaluate(id => { const d = document.getElementById(id).querySelector('.entry-panel'); return [d.open, d.classList.contains('flash')]; }, v2.id);
      assert.deepEqual(arrived, [true, true], 'the newer link reaches the latest version');
      await p.close(); p = await tab(b, width);
      await open(p, '#p1');
      // An earlier version still inside an unloaded gap is folded too.
      arrived = await p.evaluate(([a, b]) => [a, b].map(id => { const r = document.getElementById(id), d = r && r.querySelector('.entry-panel'); return d ? [d.open, d.classList.contains('flash')] : [false, false]; }), [v1.id, v2.id]);
      assert.deepEqual(arrived, [[false, false], [true, true]], 'arrival on pN leaves earlier versions folded');
      await shot(p, `arrive-latest-dark-${width}`);
      await p.locator(`#${v2.id} .pfprev a`).click();
      await p.waitForTimeout(200);
      assert.equal(await p.evaluate(id => document.getElementById(id).querySelector('.entry-panel').open, v1.id), true, 'the previous-version link opens it');
      await p.close();
    }
    assert.deepEqual(errors, [], 'browser script errors');
    console.log('ok');
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
