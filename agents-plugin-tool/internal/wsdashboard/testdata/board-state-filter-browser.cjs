// Opt-in only: TestBoardStateFilterBrowser serves production assets on port 0.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, control, output, reference] = process.argv.slice(2);
// Eight operational topics, one legacy topic, and no research topics or
// framework roots. The asks filter counts topics, not individual items.
const values = ['all', 'asks', 'researching', 'discussing', 'waiting', 'implementing', 'awaiting-confirmation', 'closed', 'cards'];
const labels = ['전체', '연구 중', '논의 중', '대기 중', '진행 중', '확인 대기', '종료', '카드'];
for (const address of [base, control]) {
  const u = new URL(address);
  assert(['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname));
  assert(u.port && u.port !== '29417');
}
const write = (name, data) => fs.writeFileSync(path.join(output, name + '.json'), JSON.stringify(data, null, 2));
const measurements = [], requests = [], errors = [], unexpected = [], checks = [];
async function until(check, message) {
  for (let i = 0; i < 200; i++) { if (await check()) return; await new Promise(r => setTimeout(r, 50)); }
  assert.fail(message);
}
async function action(id, name) {
  const res = await fetch(`${control}?id=${id}&action=${name}`, { method: 'POST' });
  assert.equal(res.status, 200); return res.json();
}
function alive(pid) { try { process.kill(pid, 0); return true; } catch (_) { return false; } }

(async () => {
  fs.mkdirSync(output, { recursive: true });
  const server = await chromium.launchServer({ headless: true });
  const chromiumPID = server.process().pid;
  write('browser-owned-before', { node_pid: process.pid, chromium_pid: chromiumPID, alive: alive(chromiumPID) });
  const browser = await chromium.connect(server.wsEndpoint());
  let context, page, tab;
  const refs = {};
  async function capture(name, locator) {
    const file = path.join(output, name + '.png');
    await (locator || tab).screenshot({ path: file }); return file;
  }
  async function pair(name, production, original) {
    const tab = await browser.newPage({ viewport: { width: 1900, height: 600 } });
    try {
      const image = file => 'data:image/png;base64,' + fs.readFileSync(file).toString('base64');
      await tab.setContent(`<body style="margin:0;background:#888;font:16px system-ui"><div style="display:flex;gap:12px;align-items:flex-start"><div>Production (corrected lifecycle counts)<br><img src="${image(production)}"></div><div>Untouched approved reference<br><img src="${image(original)}"></div></div></body>`);
      await tab.locator('img').evaluateAll(nodes => Promise.all(nodes.map(n => n.decode())));
      await tab.screenshot({ path: path.join(output, name + '-contact.png'), fullPage: true });
    } finally { await tab.close(); }
  }
  try {
    // Capture the original file directly, without rewriting its DOM or CSS.
    const ref = await browser.newPage({ viewport: { width: 1400, height: 1100 } });
    try {
      await ref.goto(pathToFileURL(reference).href);
      for (const theme of ['light', 'dark']) {
        for (const [variant, width] of [['v1', 900], ['v2', 1280], ['v2', 900]]) {
          const key = `${variant}-${theme}-${width}`;
          refs[key] = await capture('reference-' + key, ref.locator('#' + key));
        }
      }
    } finally { await ref.close(); }
    for (const theme of ['dark', 'light']) {
      context = await browser.newContext({ viewport: { width: 900, height: 1000 }, locale: 'ko-KR', colorScheme: theme, reducedMotion: 'reduce', serviceWorkers: 'allow' });
      context.setDefaultTimeout(10000);
      // Match the effects fixture: Playwright's built-in blocker itself reads
      // navigator.serviceWorker in opaque sandboxed topic frames and throws.
      await context.addInitScript(() => {
        if (self.origin === 'null') return;
        const workers = navigator.serviceWorker;
        if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('Service workers forbidden in isolated filter fixture'); };
      });
      await context.route('**/*', route => {
        const u = new URL(route.request().url());
        requests.push({ theme, method: route.request().method(), url: u.href });
        if (u.pathname === '/favicon.ico') return route.fulfill({ status: 204 });
        if (['http:', 'https:'].includes(u.protocol) && ![new URL(base).origin, new URL(control).origin].includes(u.origin)) { unexpected.push(u.href); return route.abort(); }
        return route.continue();
      });
      tab = await context.newPage();
      tab.on('pageerror', e => errors.push({ theme, message: e.message }));
      async function settle() { await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)))); }
      async function active() {
        await until(async () => {
          const handle = await tab.locator('iframe.sframe.on').elementHandle();
          page = handle && await handle.contentFrame();
          return page && await page.locator('.board-toolbar').count() > 0;
        }, 'active production session frame');
      }
      async function load(id) {
        await tab.goto(`${base}/s/${id}`);
        await active();
        await page.locator('.board-toolbar').waitFor({ state: 'attached' });
        if (!(await page.locator('#bcol').isVisible())) await page.locator('[data-tab="board"]').first().click();
        await settle();
      }
      async function width(target) {
        await page.evaluate(() => { document.scrollingElement.scrollTop = 0; const b = document.querySelector('#bcol'); b.scrollTop = 0; });
        await tab.setViewportSize({ width: target + 50, height: 1000 });
        for (let i = 0; i < 6; i++) {
          await settle();
          const actual = (await page.locator('.board-toolbar').boundingBox()).width;
          if (Math.abs(actual - target) < 0.1) break;
          await tab.setViewportSize({ width: Math.round(tab.viewportSize().width + target - actual), height: 1000 });
        }
        await until(async () => Math.abs((await page.locator('.board-toolbar').boundingBox()).width - target) < 0.1, 'actual toolbar outer width ' + target);
        await until(async () => await page.locator('.board-state-trigger').isVisible() === (target < 540), 'responsive presentation at actual width ' + target);
        measurements.push({ theme, requestedToolbarWidth: target, viewport: tab.viewportSize(), outer: await page.locator('.board-toolbar').boundingBox(), inline: await page.locator('.board-state-inline').isVisible(), selected: await page.locator('.board-state-radio[aria-checked="true"]').getAttribute('data-filter') });
        write('measurements', measurements);
      }
      async function select(value) {
        if (await page.locator('.board-state-trigger').isVisible()) {
          await page.locator('.board-state-trigger').click();
          await page.locator(`.board-state-option[data-filter="${value}"]`).click();
        } else await page.locator(`.board-state-radio[data-filter="${value}"]`).click();
        await settle();
        assert.equal(await page.locator('.board-state-radio[aria-checked="true"]').getAttribute('data-filter'), value);
      }
      async function update(id, name, expected) {
        await action(id, name);
        // e64fcea2d AI Context intentionally latches top-origin changes until
        // explicit apply; unexposed content can still be admitted immediately.
        // Verify the same exact counts after either production admission path.
        const apply = page.locator('.wb-oq-top .oq-updates');
        await until(async () => await apply.isVisible() || JSON.stringify(await page.locator('.board-state-radio:not([data-filter="asks"]) .board-state-count').allTextContents()) === JSON.stringify(expected.map(String)), 'stream update admitted or offered for explicit apply');
        if (await apply.isVisible()) await apply.click();
        await counts(expected);
      }
      async function counts(expected) {
        await until(async () => JSON.stringify(await page.locator('.board-state-radio:not([data-filter="asks"]) .board-state-count').allTextContents()) === JSON.stringify(expected.map(String)), 'authoritative counts ' + expected);
        assert.deepEqual(await page.locator('.board-state-option:not([data-filter="asks"]) .board-state-count').allTextContents(), expected.map(String));
      }
      async function eligible(value) {
        const nodes = await page.locator('section.topic[data-topic]').evaluateAll(nodes => nodes.map(n => ({ name: n.dataset.topic, state: n.dataset.state, closed: n.classList.contains('closed'), asks: n.dataset.openAsks === 'true', hidden: n.hasAttribute('data-state-filtered'), display: getComputedStyle(n).display })));
        for (const n of nodes) {
          // 32dec7255 countTopics: Cards excludes every topic, including legacy
          // topics and topic-internal panels; only framework roots are eligible.
          const visible = value !== 'cards' && (value === 'all' || (value === 'asks' ? n.asks : n.closed ? value === 'closed' : value !== 'closed' && n.state === value));
          assert.equal(n.hidden, !visible, 'eligibility ' + n.name + ' under ' + value);
          if (!visible) assert.equal(n.display, 'none');
        }
        assert(nodes.some(n => n.name === 'legacy' && !n.state), 'legacy remains metadata-free and retained');
      }
      async function geometry(name, oneRow) {
        const result = await page.locator('.board-toolbar').evaluate(toolbar => {
          const rect = n => { const r = n.getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height, right: r.right, bottom: r.bottom }; };
          const actions = [...toolbar.querySelectorAll('.board-overview,.tnew,.wnew,.board-download,.board-files')].map(n => {
            const r = rect(n), hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
            return { label: n.getAttribute('aria-label'), rect: r, hit: !!hit && n.contains(hit), clipped: n.scrollWidth > n.clientWidth + 1 };
          });
          const radios = [...toolbar.querySelectorAll('.board-state-radio')].filter(n => n.getClientRects().length).map(n => ({ value: n.dataset.filter, rect: rect(n), clipped: n.scrollWidth > n.clientWidth + 1 }));
          const filter = toolbar.querySelector('.board-state-filter');
          return { outer: rect(toolbar), face: rect(toolbar.firstChild), filter: rect(filter), actions, radios, selected: toolbar.querySelector('.board-state-radio[aria-checked="true"]').dataset.filter, floating: toolbar.hasAttribute('data-floating') };
        });
        measurements.push({ name, theme, ...result }); write('measurements', measurements);
        const live = await page.locator('html').getAttribute('data-mode') === 'live';
        assert.equal(result.actions.length, live ? 5 : 4, name + ' inherited actions (synthetic terminal capability; Files live-only)');
        assert.equal(await page.locator('.board-files').count(), live ? 1 : 0, 'preserve baseline live-only Files gate');
        for (const a of result.actions) {
          assert(a.rect.width > 0 && a.hit, name + ' action hit-testable: ' + a.label);
          assert(!a.clipped, name + ' action clipping: ' + a.label);
          assert(a.rect.x >= result.face.x && a.rect.right <= result.face.right, name + ' action within face');
        }
        if (oneRow) {
          const boxes = [...result.actions.map(a => a.rect), result.filter];
          assert(Math.max(...boxes.map(r => r.y + r.height / 2)) - Math.min(...boxes.map(r => r.y + r.height / 2)) <= 1, name + ' single row');
          const sorted = boxes.sort((a, b) => a.x - b.x);
          for (let i = 1; i < sorted.length; i++) assert(sorted[i - 1].right <= sorted[i].x + 0.5, name + ' no overlap');
          assert(result.filter.right <= result.face.right, name + ' filter fits face');
          for (const r of result.radios) assert(!r.clipped, name + ' segment unclipped ' + r.value);
        }
      }
      for (const id of ['filterordinary', 'filtersaved', 'filterhundred', 'filterlive']) {
        await action(id, 'reset');
        await load(id);
        const history = id === 'filtersaved', closed = id === 'filterhundred' ? 135 : 35;
        assert.equal(await page.locator('html').getAttribute('data-mode'), history ? 'history' : id === 'filterlive' ? 'live' : 'board');
        const capability = await page.evaluate(() => ({ mode: document.documentElement.dataset.mode, terminal: document.documentElement.hasAttribute('data-term'), filesPane: !!document.querySelector('#files'), actions: [...document.querySelectorAll('.board-toolbar .board-action')].map(n => ({ class: n.className, label: n.getAttribute('aria-label') })) }));
        write(`${id}-${theme}-capabilities`, { syntheticTerminalHost: true, syntheticAdapter: id === 'filterlive', actionsExecuted: false, ...capability });
        if (history) {
          assert.equal(await page.locator('.dgrid.history').count(), 1);
          assert.equal(await page.locator('#ctext,#composer').count(), 0, 'saved history has no invented adapter');
          assert((await page.locator('#convo').textContent()).includes('Saved history entry'), 'real AppendRing conversation is present');
        }
        if (id === 'filterlive' && theme === 'dark') {
          for (const lang of ['ko', 'en']) {
            await context.addCookies([{ name: 'ws_dashboard_lang', value: lang, url: base }]);
            await tab.setViewportSize({ width: 1440, height: 1000 });
            await load(id);
            assert.equal(await page.locator('.board-overview .board-label').textContent(), lang === 'ko' ? '한 눈에 보기' : 'Overview');
            const layout = await page.locator('.board-toolbar').evaluate(n => ({
              outerWidth: n.getBoundingClientRect().width,
              containerWidth: n.firstElementChild.clientWidth,
              labels: [...n.querySelectorAll('.board-action .board-label')].map(n => ({ text: n.textContent, visible: !!n.getClientRects().length }))
            }));
            measurements.push({ name: 'default-split-1440', lang, ...layout }); write('measurements', measurements);
            assert.equal(layout.labels.length, 5);
            assert(layout.labels.every(n => n.visible), lang + ' default split retains every action label');
            // The original 760px rule already wraps English at this width;
            // preserve its labels while retaining Korean's single-row layout.
            await geometry('default-split-1440-' + lang, lang === 'ko');
            await capture('filterlive-dark-1440' + (lang === 'en' ? '-en' : ''));
          }
          await context.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
          await load(id);
        }
        await width(800);
        for (const selector of ['.board-state-radio', '.board-state-option']) {
          assert.deepEqual(await page.locator(selector).evaluateAll(nodes => nodes.map(n => n.dataset.filter)), values, selector + ' exact lifecycle order');
          assert.deepEqual(await page.locator(selector + ':not([data-filter="asks"]) .board-state-label').allTextContents(), labels, selector + ' Korean labels follow values');
        }
        await select('discussing');
        const inlineCurrent = () => page.locator('.board-state-radio[aria-checked="true"]');
        for (const [key, value] of [['ArrowRight', 'waiting'], ['ArrowDown', 'implementing'], ['ArrowLeft', 'waiting'], ['ArrowUp', 'discussing'], ['ArrowUp', 'researching'], ['ArrowLeft', 'asks'], ['ArrowLeft', 'all'], ['ArrowRight', 'asks'], ['ArrowRight', 'researching'], ['Home', 'all'], ['ArrowLeft', 'cards'], ['ArrowUp', 'closed'], ['ArrowDown', 'cards'], ['ArrowRight', 'all'], ['End', 'cards'], ['ArrowDown', 'all'], ['ArrowUp', 'cards']]) {
          await inlineCurrent().press(key);
          assert.equal(await inlineCurrent().getAttribute('data-filter'), value);
          assert.equal(await inlineCurrent().evaluate(n => document.activeElement === n), true);
          assert.equal(await page.evaluate(id => localStorage.getItem('ws.dashboard.cards.' + id + '.state-filter'), id), value);
        }
        await select('all');
        await counts([closed + 9, 0, 2, 2, 2, 2, closed, 0]);
        assert.equal(await page.locator('section.topic.closed').count(), 30);
        assert.equal(await page.locator('.closed-more').getAttribute('data-closed-more'), String(closed - 30));
        await page.locator('section.topic').evaluateAll(nodes => nodes.forEach(n => { n.filterOriginalNode = n; const f = n.querySelector('iframe'); if (f) n.filterOriginalFrame = f; }));
        const order = await page.locator('section.topic').evaluateAll(nodes => nodes.map(n => n.dataset.topic));
        for (const value of values) {
          await select(value); await eligible(value);
          const name = `${id}-${theme}-800-${value}`;
          const shot = await capture(name);
          if (value === 'discussing' || value === 'all') await pair(name, shot, refs[`v1-${theme}-900`]);
          await geometry(name, true);
          assert.equal(await page.locator('section.topic').evaluateAll(nodes => nodes.every(n => n.filterOriginalNode === n && (!n.filterOriginalFrame || n.querySelector('iframe') === n.filterOriginalFrame))), true, 'filter retains nodes and body iframes');
        }
        await select('all');
        assert.deepEqual(await page.locator('section.topic').evaluateAll(nodes => nodes.map(n => n.dataset.topic)), order, 'All restores original order');
        // Threshold uses the actual toolbar, not the browser viewport.
        await select('discussing');
        await page.locator('.board-state-radio[data-filter="discussing"]').focus();
        await width(540); await counts([closed + 9, 0, 2, 2, 2, 2, closed, 0]);
        assert.equal(await page.locator('.board-state-inline').isVisible(), true);
        await capture(`${id}-${theme}-540-inline`);
        await width(539);
        assert.equal(await page.locator('.board-state-trigger').evaluate(n => document.activeElement === n), true, 'focus transfers to pill');
        assert.equal(await page.locator('.board-state-trigger').getAttribute('data-filter'), 'discussing');
        await page.locator('.board-state-trigger').click();
        const options = page.locator('.board-state-option');
        assert.equal(await options.count(), values.length);
        for (let i = 0; i < values.length; i++) {
          const o = options.nth(i);
          assert.equal(await o.evaluate(n => n.tagName), 'BUTTON');
          assert.equal(await o.getAttribute('role'), 'radio');
          assert.equal(await o.getAttribute('data-filter'), values[i]);
          const selected = values[i] === 'discussing';
          assert.equal(await o.getAttribute('aria-checked'), String(selected));
          assert.equal(await o.getAttribute('tabindex'), selected ? '0' : '-1');
        }
        let shot = await capture(`${id}-${theme}-539-open`);
        await pair(`${id}-${theme}-539-open`, shot, refs[`v2-${theme}-1280`]);
        const current = () => page.locator('.board-state-option[aria-checked="true"]');
        for (const [key, value] of [['ArrowDown', 'waiting'], ['ArrowUp', 'discussing'], ['ArrowRight', 'waiting'], ['ArrowLeft', 'discussing'], ['ArrowUp', 'researching'], ['ArrowLeft', 'asks'], ['ArrowLeft', 'all'], ['ArrowRight', 'asks'], ['ArrowRight', 'researching'], ['End', 'cards'], ['ArrowUp', 'closed'], ['ArrowDown', 'cards'], ['ArrowRight', 'all'], ['ArrowLeft', 'cards'], ['Home', 'all'], ['ArrowUp', 'cards'], ['ArrowDown', 'all']]) {
          await current().press(key);
          assert.equal(await current().getAttribute('data-filter'), value);
          assert.equal(await current().evaluate(n => document.activeElement === n), true);
          assert.equal(await page.evaluate(id => localStorage.getItem('ws.dashboard.cards.' + id + '.state-filter'), id), value);
        }
        await current().press('Enter');
        assert.equal(await page.locator('.board-state-pop').isVisible(), false);
        assert.equal(await page.locator('.board-state-trigger').evaluate(n => document.activeElement === n), true);
        await page.locator('.board-state-trigger').click(); await current().press('End'); await current().press('Space');
        assert.equal(await page.locator('.board-state-pop').isVisible(), false);
        await page.locator('.board-state-trigger').click(); await current().press('Escape');
        assert.equal(await page.locator('.board-state-trigger').evaluate(n => document.activeElement === n), true);
        await page.locator('.board-state-trigger').click(); await current().press('Tab');
        assert.equal(await page.locator('.board-state-pop').isVisible(), false, 'Tab exits popover without trapping focus');
        await page.locator('.board-state-trigger').click(); await page.locator('.board-overview').focus();
        assert.equal(await page.locator('.board-state-pop').isVisible(), false);
        await page.locator('.board-state-trigger').click(); await page.locator('.board-toolbar-face').click({ position: { x: 2, y: 2 } });
        assert.equal(await page.locator('.board-state-pop').isVisible(), false, 'outside click dismisses');
        await select('discussing');
        await page.locator('.board-state-trigger').focus();
        await width(540);
        assert.equal(await page.locator('.board-state-radio[data-filter="discussing"]').evaluate(n => document.activeElement === n), true, 'focus returns to checked inline radio');
        await width(539);
        await width(360); await geometry(`${id}-${theme}-360`, false);
        shot = await capture(`${id}-${theme}-360-pill`); await pair(`${id}-${theme}-360-pill`, shot, refs[`v2-${theme}-900`]);
        const stored = await page.evaluate(id => localStorage.getItem('ws.dashboard.cards.' + id + '.state-filter'), id);
        assert.equal(stored, 'discussing');
        await tab.reload(); await active(); await page.locator('.board-toolbar').waitFor({ state: 'attached' });
        if (!(await page.locator('#bcol').isVisible())) await page.locator('[data-tab="board"]').first().click();
        await until(async () => await page.locator('.board-state-trigger').getAttribute('data-filter') === 'discussing', 'per-session persistence');
        await eligible('discussing');
        await width(800); await select('waiting');
        await update(id, 'transition', [closed + 9, 0, 4, 0, 2, 2, closed, 0]); await eligible('waiting');
        assert.equal(await page.locator('section.topic:not([data-state-filtered])').count(), 0, 'zero transition has no invented eligibility');
        await select('discussing'); await update(id, 'close', [closed + 9, 0, 3, 0, 2, 2, closed + 1, 0]);
        await eligible('discussing');
        await select('closed');
        const paging = tab.waitForResponse(r => new URL(r.url()).pathname === `/s/${id}/closed` && r.status() === 200);
        await page.locator('.closed-more').click(); await paging;
        await until(async () => await page.locator('section.topic.closed').count() > 30, 'real closed history appended');
        await counts([closed + 9, 0, 3, 0, 2, 2, closed + 1, 0]); await eligible('closed');
        await select('all');
        // Real scroll ownership differs for ordinary and saved-history boards.
        const scroll = await page.locator('#bcol').evaluate(b => {
          let s = b;
          while (s && !(/auto|scroll/.test(getComputedStyle(s).overflowY) && s.scrollHeight > s.clientHeight)) s = s.parentElement;
          s = s || document.scrollingElement;
          s.scrollTop = Math.min(450, s.scrollHeight - s.clientHeight);
          s.dispatchEvent(new Event('scroll', { bubbles: true }));
          return { id: s.id, class: s.className, top: s.scrollTop, height: s.clientHeight, scrollHeight: s.scrollHeight, document: s === document.scrollingElement };
        });
        measurements.push({ theme, id, scroll }); write('measurements', measurements);
        assert(scroll.top > 0, 'owned board actually scrolls');
        if (history) assert.equal(scroll.id, 'bcol', 'saved history uses bounded board scroller');
        await until(async () => await page.locator('.board-toolbar').evaluate(n => n.hasAttribute('data-floating')), 'toolbar floats in actual scroller');
        await geometry(`${id}-${theme}-floating`, true); await capture(`${id}-${theme}-floating`);
        await width(800); await select('implementing');
        await page.locator('.board-overview').click();
        await until(async () => await page.evaluate(() => document.activeElement.matches('section.topic[data-state="implementing"]:not([data-state-filtered])')), 'Overview roves an eligible topic');
        const eligibleNames = await page.locator('section.topic:not([data-state-filtered])').evaluateAll(nodes => nodes.map(n => n.dataset.topic));
        await tab.keyboard.press('ArrowDown');
        assert.equal(await page.evaluate(() => document.activeElement.dataset.topic), eligibleNames[1], 'Overview ArrowDown traverses only eligible nodes');
        await tab.keyboard.press('ArrowUp');
        assert.equal(await page.evaluate(() => document.activeElement.dataset.topic), eligibleNames[0], 'Overview ArrowUp traverses only eligible nodes');
        const overview = await page.evaluate(() => ({ pressed: document.querySelector('.board-overview').getAttribute('aria-pressed'), hiddenFocus: !!document.activeElement.closest('[data-state-filtered]') }));
        assert.equal(overview.pressed, 'true'); assert.equal(overview.hiddenFocus, false, 'Overview keyboard excludes hidden topics');
        await select('awaiting-confirmation');
        assert.equal(await page.locator('.board-overview').getAttribute('aria-pressed'), 'true', 'filter keeps Overview pressed');
        assert.equal(await page.locator('html').evaluate(n => n.hasAttribute('data-overview')), true, 'filter keeps one-line view');
        await eligible('awaiting-confirmation');
        assert.equal(await page.locator('.board-state-radio[data-filter="awaiting-confirmation"]').evaluate(n => document.activeElement === n), true, 'filter retains control focus');
        const filteredNames = await page.locator('section.topic:not([data-state-filtered])').evaluateAll(nodes => nodes.map(n => n.dataset.topic));
        const roved = page.locator('section.topic[tabindex="0"]');
        assert.equal(await roved.count(), 1, 'one eligible roving target after filter');
        assert.equal(await roved.getAttribute('data-topic'), filteredNames[0]);
        await roved.focus();
        await tab.keyboard.press('ArrowDown');
        assert.equal(await page.evaluate(() => document.activeElement.dataset.topic), filteredNames[1], 'filtered Overview ArrowDown skips hidden rows');
        await tab.keyboard.press('ArrowUp');
        assert.equal(await page.evaluate(() => document.activeElement.dataset.topic), filteredNames[0], 'filtered Overview ArrowUp skips hidden rows');
        await tab.keyboard.press('Enter');
        assert.equal(await page.locator('.board-overview').getAttribute('aria-pressed'), 'false', 'topic selection still exits Overview');
        assert.equal(await page.evaluate(() => document.activeElement.dataset.topic), filteredNames[0], 'topic selection expands selected eligible row');
        checks.push({ theme, id, matrix: '800 all eight selected labels; 540 inline; 539 pop keyboard; 360 pill; reload; stream transition/close; real closed paging; floating; Overview' });
      }
      // Real rendered open check/policy/ack, assumed check, and answered check.
      await action('filterordinary', 'reset'); await load('filterordinary'); await width(800);
      for (const [name, expected] of [['discussing0', 'true'], ['discussing1', 'true'], ['waiting0', null], ['waiting1', 'true'], ['implementing0', null]]) {
        assert.equal(await page.locator(`section.topic[data-topic="${name}"]`).getAttribute('data-open-asks'), expected, name + ' rendered ask marker');
      }
      await select('asks'); await eligible('asks');
      assert.equal(await page.locator('.board-state-radio[data-filter="asks"] .board-state-count').textContent(), '3');
      await tab.reload(); await active();
      if (!(await page.locator('#bcol').isVisible())) await page.locator('[data-tab="board"]').first().click();
      await until(async () => await page.locator('.board-state-radio[data-filter="asks"]').getAttribute('aria-checked') === 'true', 'ask filter survives reload');
      await action('filterordinary', 'answer-asks');
      const applyAsks = page.locator('.wb-oq-top .oq-updates');
      await until(async () => await applyAsks.isVisible() || await page.locator('.board-state-radio[data-filter="asks"] .board-state-count').textContent() === '1', 'answer update admitted or offered');
      if (await applyAsks.isVisible()) await applyAsks.click();
      await until(async () => await page.locator('.board-state-radio[data-filter="asks"] .board-state-count').textContent() === '1', 'answered asks recount');
      await eligible('asks');
      // Wide pin column uses real production pin actions and keeps hidden nodes.
      await load('filterordinary'); await tab.setViewportSize({ width: 1900, height: 1000 }); await select('all');
      const pinTopic = page.locator('section.topic[data-topic="implementing0"]');
      const pin = pinTopic.locator('.tpin');
      await pin.click();
      const secondPinTopic = page.locator('section.topic[data-topic="discussing1"]');
      await secondPinTopic.locator('.tpin').click();
      await until(async () => await page.locator('#bgal').isVisible(), 'wide gallery column appears');
      await pinTopic.evaluate(n => { n.pinnedIdentity = n; n.pinnedFrame = n.querySelector('iframe'); });
      const pinOrder = await page.locator('#bgal section.topic').evaluateAll(nodes => nodes.map(n => n.dataset.topic));
      await select('waiting');
      assert.equal(await pinTopic.evaluate(n => n.hasAttribute('data-state-filtered') && n.pinnedIdentity === n && n.pinnedFrame === n.querySelector('iframe')), true);
      assert.deepEqual(await page.locator('#bgal section.topic').evaluateAll(nodes => nodes.map(n => n.dataset.topic)), pinOrder);
      await select('all');
      assert.deepEqual(await page.locator('#bgal section.topic').evaluateAll(nodes => nodes.map(n => n.dataset.topic)), pinOrder, 'All restores multi-pin order');
      await capture(`wide-pins-${theme}`);
      await pin.click(); await secondPinTopic.locator('.tpin').click();
      assert.deepEqual(errors, [], 'no production page errors'); assert.deepEqual(unexpected, [], 'requests isolated to owned listeners');
      console.log(`PASS ${theme}: ${JSON.stringify(checks.filter(c => c.theme === theme))}; wide pin identity/order`);
      await context.close(); context = null;
    }
  } catch (error) {
    if (tab && !tab.isClosed()) { try { await capture('failure'); fs.writeFileSync(path.join(output, 'failure.html'), await page.content()); } catch (_) {} }
    write('failure', { message: error.message, stack: error.stack }); throw error;
  } finally {
    write('checks', checks); write('measurements', measurements); write('requests-and-errors', { requests, errors, unexpected });
    if (context) await context.close();
    await browser.close(); await server.close();
    write('browser-owned-after', { node_pid: process.pid, chromium_pid: chromiumPID, alive: alive(chromiumPID) });
    assert.equal(alive(chromiumPID), false, 'owned Chromium is reaped');
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
