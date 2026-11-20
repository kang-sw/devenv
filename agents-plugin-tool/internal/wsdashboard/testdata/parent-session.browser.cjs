// Run by TestParentSessionBrowser with real pages and disposable stores.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

// strip reads the shell's header strip, the parent pill and the identity.
function strip(page) {
  return page.evaluate(() => {
    const box = el => {
      if (!el) return null;
      const b = el.getBoundingClientRect(), cs = getComputedStyle(el);
      if (!(b.width || b.height) || cs.display === 'none') return null;
      return { l: b.left, t: b.top, r: b.right, b: b.bottom, w: b.width, h: b.height };
    };
    const top = document.querySelector('.dwrap > .top'), cs = getComputedStyle(top);
    const pill = top.querySelector('.tph-parent'), name = pill && pill.querySelector('.tph-pn');
    const items = [...top.querySelectorAll(':scope > :not(.r-head), :scope > .r-head > *')].map(e => ({ cls: e.className, box: box(e) })).filter(i => i.box);
    return {
      top: box(top), padR: parseFloat(cs.paddingRight), pill: box(pill), id: box(top.querySelector('.tph-id')),
      tag: pill && pill.tagName, href: pill && pill.getAttribute('href'), cls: pill && pill.className,
      title: pill && pill.getAttribute('title'), label: pill && pill.querySelector('.tph-r1').textContent,
      name: name && name.textContent, nameClipped: name ? name.scrollWidth > name.clientWidth : null,
      pillClipped: pill ? pill.scrollWidth > pill.clientWidth + 0.5 : null,
      pillFirst: pill ? [...top.querySelectorAll(':scope > .r-head > *')][0] === pill : null,
      idFirst: [...top.querySelectorAll(':scope > .r-head > *')][0].classList.contains('tph-id'),
      cursor: pill && getComputedStyle(pill).cursor, opacity: pill && getComputedStyle(pill).opacity,
      items,
    };
  });
}

function checkStrip(label, g) {
  assert.ok(g.top, label + ': header shows');
  assert.equal(g.top.h, 34, label + ': header is one 34px strip');
  const end = g.top.r - g.padR;
  for (const it of g.items) {
    if (!/\b(tph|back|tcen)\b/.test(it.cls)) continue;
    assert.equal(it.box.t, 0, `${label}: ${it.cls} hangs from the top edge`);
    assert.equal(it.box.h, 34, `${label}: ${it.cls} is 34px tall`);
    assert.ok(it.box.r <= end + 0.5, `${label}: ${it.cls} ends at ${it.box.r}, past the strip's room ${end}`);
  }
}

async function open(page, rid) {
  await page.goto(base + '/s/' + rid);
  await page.waitForFunction(rid => {
    const f = document.querySelector('.sframe.on');
    return f && f.contentWindow.location.pathname === '/s/' + rid && document.querySelector('.top').style.minHeight !== '';
  }, rid);
  await page.waitForTimeout(250);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of [1280, 760]) {
      for (const theme of ['light', 'dark']) {
        const label = `${theme} ${width}px`;
        const context = await browser.newContext({ viewport: { width, height: 800 }, colorScheme: theme });
        await context.addInitScript(() => {
          const workers = navigator.serviceWorker;
          if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('No service workers in fixture'); };
        });
        await context.route('**/*', route => {
          const url = new URL(route.request().url());
          if (url.origin !== base) return route.abort();
          if (url.pathname === '/rail/usage') return route.fulfill({ json: { entries: [], warnPercent: 80, critPercent: 95 } });
          return route.continue();
        });
        const page = await context.newPage();
        const errors = [];
        // Sandboxed board frames have no service worker; that noise is not the head's.
        page.on('pageerror', e => { if (!/serviceWorker/.test(e.message)) errors.push(e.message); });

        // A child: the pill sits left of the identity, whole, in the strip.
        await open(page, 'kidsession');
        let g = await strip(page);
        checkStrip(label + ' kid', g);
        assert.ok(g.pill, label + ': the child shows the parent pill');
        assert.equal(g.tag, 'A', label + ': a served parent is a link');
        assert.equal(g.href, '/s/parentsession');
        assert.equal(g.label, 'Parent');
        assert.equal(g.name, 'Parent Label');
        assert.equal(g.pill.t, 0); assert.equal(g.pill.h, 34, label + ': pill is 34px tall');
        assert.ok(g.pillFirst, label + ': the pill leads the head');
        assert.ok(g.pill.r <= g.id.l + 0.5, `${label}: pill (${g.pill.r}) is left of the identity (${g.id.l})`);
        assert.ok(!g.nameClipped && !g.pillClipped, label + ': a short parent name is not clipped');
        assert.ok(g.id.w > 40, `${label}: the identity keeps room (${g.id.w}px)`);
        if (shots) { fs.mkdirSync(shots, { recursive: true }); await page.screenshot({ path: path.join(shots, `kid-${theme}-${width}.png`), clip: { x: 0, y: 0, width, height: 60 } }); }

        // The click switches in place: no reload, the URL and frame follow,
        // and the root parent shows no pill.
        await page.evaluate(() => { window.__noReload = 1; });
        await page.locator('.dwrap > .top .tph-parent').click();
        await page.waitForFunction(() => {
          const f = document.querySelector('.sframe.on');
          return location.pathname === '/s/parentsession' && f && f.contentWindow.location.pathname === '/s/parentsession' && !document.querySelector('.top .tph-parent');
        });
        assert.equal(await page.evaluate(() => window.__noReload), 1, label + ': the shell switched in place, not by a reload');
        await page.waitForTimeout(250);
        g = await strip(page);
        checkStrip(label + ' parent', g);
        assert.equal(g.pill, null, label + ': the root parent shows no pill');
        assert.ok(g.idFirst, label + ': the identity leads a root session');
        await page.goBack();
        await page.waitForFunction(() => location.pathname === '/s/kidsession' && !!document.querySelector('.top .tph-parent'));

        // A modified click leaves the shell where it is and opens a tab.
        const popup = context.waitForEvent('page');
        await page.locator('.dwrap > .top .tph-parent').click({ modifiers: ['ControlOrMeta'] });
        const tab = await Promise.race([popup, new Promise(r => setTimeout(() => r(null), 5000))]);
        assert.ok(tab, label + ': a modified click opens a new tab');
        await tab.waitForURL(/\/s\/parentsession$/, { timeout: 5000 }).catch(() => {});
        assert.ok(tab.url().endsWith('/s/parentsession'), label + ': the new tab is the parent: ' + tab.url());
        assert.equal(new URL(page.url()).pathname, '/s/kidsession', label + ': the shell stayed on the child');
        await tab.close();

        // A grandchild with a long label: the immediate parent, the chain in
        // the tooltip, the name ellipsized inside the same strip.
        await open(page, 'deepsession');
        g = await strip(page);
        checkStrip(label + ' deep', g);
        assert.equal(g.href, '/s/kidsession');
        assert.equal(g.name, 'Kid Label');
        assert.ok(g.title.includes('Parent Label › Kid Label'), label + ': the tooltip names the chain: ' + g.title);
        assert.ok(g.pill.r <= g.id.l + 0.5);

        // A pruned parent: the same pill, muted and inert.
        await open(page, 'orphankid');
        g = await strip(page);
        checkStrip(label + ' orphan', g);
        assert.equal(g.tag, 'SPAN', label + ': a pruned parent is not a link');
        assert.ok(/\bdead\b/.test(g.cls) && g.href === null);
        assert.ok(parseFloat(g.opacity) < 1, label + ': the dead pill is muted');
        assert.notEqual(g.cursor, 'pointer');
        await page.locator('.dwrap > .top .tph-parent').click({ force: true });
        await page.waitForTimeout(300);
        assert.equal(new URL(page.url()).pathname, '/s/orphankid', label + ': an inert pill never navigates');
        if (shots) await page.screenshot({ path: path.join(shots, `orphan-${theme}-${width}.png`), clip: { x: 0, y: 0, width, height: 60 } });

        assert.deepEqual(errors, [], label + ': page errors');
        await context.close();
        console.log('ok', label);
      }
    }
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
