// Run by TestShellTopbarBrowser with real pages and disposable stores.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots, mockup] = process.argv.slice(2);

const at = Date.now();
const usage = { warnPercent: 80, critPercent: 95, entries: [
  { harness: 'claude', source: 'fixture', asOf: at - 42 * 60000, windows: [
    { kind: 'five_hour', percent: 26, asOf: at - 42 * 60000, resetsAt: at + 2 * 3600000 },
    { kind: 'seven_day', percent: 53, asOf: at - 42 * 60000, resetsAt: at + 3 * 86400000 }] },
  { harness: 'codex', source: 'fixture', asOf: at, windows: [
    { kind: 'seven_day', percent: 79, asOf: at, resetsAt: at + 2 * 86400000 }] }] };

function shot(page, name, opts) {
  if (!shots) return Promise.resolve();
  fs.mkdirSync(shots, { recursive: true });
  return page.screenshot(Object.assign({ path: path.join(shots, name + '.png') }, opts || {}));
}

// geometry reads the shell's strip, its items, the rail and the framed
// page's right-end handles (the frame fills the window, so its coordinates
// are the shell's).
function geometry(page) {
  return page.evaluate(() => {
    const box = el => {
      if (!el) return null;
      const b = el.getBoundingClientRect(), cs = getComputedStyle(el);
      if (!(b.width || b.height) || cs.visibility === 'hidden' || cs.display === 'none') return null;
      return { l: b.left, t: b.top, r: b.right, b: b.bottom, w: b.width, h: b.height };
    };
    const top = document.querySelector('.dwrap > .top'), cs = getComputedStyle(top);
    const items = [...top.querySelectorAll(':scope > :not(.r-head), :scope > .r-head > *')]
      .map(e => ({ cls: e.className, box: box(e) })).filter(i => i.box);
    const fdoc = document.querySelector('.sframe.on').contentDocument;
    const handles = [...fdoc.querySelectorAll('#drhandle, .tk-handle, #ghandle, .dr-search-handle')].map(box).filter(Boolean);
    const rail = document.querySelector('#rail');
    return { vw: innerWidth, vh: innerHeight, top: box(top), padR: parseFloat(cs.paddingRight), cls: top.className,
      rail: box(rail), railZ: getComputedStyle(rail).zIndex, railbtn: box(document.querySelector('#railbtn')),
      usage: box(document.querySelector('.usage-toggle')), host: box(document.querySelector('.top .host.sse')),
      items, handles, open: document.documentElement.classList.contains('rail-open') };
  });
}

function checkStrip(label, g) {
  assert.ok(g.top, label + ': header shows');
  assert.equal(g.top.t, 0, label + ': header hangs from the top edge');
  assert.equal(g.top.h, 34, label + ': header is one 34px strip');
  const end = g.top.r - g.padR;
  for (const it of g.items) {
    if (!/\b(tph|back|tcen)\b/.test(it.cls)) continue;
    assert.equal(it.box.t, 0, `${label}: ${it.cls} hangs from the top edge`);
    assert.equal(it.box.h, 34, `${label}: ${it.cls} is 34px tall`);
    assert.ok(it.box.r <= end + 0.5, `${label}: ${it.cls} ends at ${it.box.r}, past the strip's room ${end}`);
    for (const h of g.handles) assert.ok(it.box.r <= h.l + 0.5, `${label}: ${it.cls} (${it.box.r}) collides with a right-end handle at ${h.l}`);
  }
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const report = [];
  try {
    if (mockup && shots) {
      // The approved mockup's states, for the side-by-side look.
      const page = await browser.newPage({ viewport: { width: 1680, height: 900 } });
      await page.goto(pathToFileURL(mockup).href);
      const frame = page.locator('.sr4-frame');
      await frame.screenshot({ path: path.join(shots, 'mockup-wide.png') });
      await page.locator('.sr4-h[data-a="usage"]').click();
      await frame.screenshot({ path: path.join(shots, 'mockup-wide-usage.png') });
      await page.locator('.sr4-cb[data-k="narrow"]').click();
      await frame.screenshot({ path: path.join(shots, 'mockup-narrow.png') });
      await page.locator('.sr4-h[data-a="tg"]').click();
      await frame.screenshot({ path: path.join(shots, 'mockup-narrow-open.png') });
      await page.close();
    }
    for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({ viewport: { width: 1920, height: 900 }, colorScheme: theme });
      await context.addInitScript(() => {
        if (self !== top) return;
        const workers = navigator.serviceWorker;
        if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('No service workers in fixture'); };
      });
      await context.route('**/*', route => {
        const url = new URL(route.request().url());
        if (url.origin !== base) return route.abort();
        if (url.pathname === '/rail/usage') return route.fulfill({ json: usage });
        return route.continue();
      });
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(base + '/s/livesession');
      await page.waitForFunction(() => document.querySelector('.dwrap > .top').style.minHeight !== '');
      await page.frameLocator('.sframe.on').locator('#grid').waitFor();
      await page.locator('.usage-number').first().waitFor();
      await page.waitForTimeout(300);

      // Wide: the docked rail runs the full height 12px in from the edges;
      // the strip keeps to the stage column right of it.
      let g = await geometry(page);
      checkStrip(`${theme}/1920`, g);
      assert.deepEqual([g.rail.l, g.rail.t, g.rail.b], [12, 12, g.vh - 12], `${theme}/1920: rail runs the full height`);
      assert.ok(g.top.l >= g.rail.r + 16 - 0.5, `${theme}/1920: header starts right of the rail (${g.top.l} vs ${g.rail.r})`);
      assert.equal(g.railbtn, null, `${theme}/1920: rail button hides beside the docked rail`);
      assert.ok(!/\btu\b/.test(g.cls), `${theme}/1920: plan usage shows in full`);
      // The badge mark and its stale clock stay clear of the columns.
      const marks = await page.evaluate(() => [...document.querySelectorAll('.top .usage-harness')].map(h => {
        const mark = h.querySelector('.usage-mark').getBoundingClientRect(), cols = h.querySelector('.usage-columns').getBoundingClientRect();
        const clock = h.querySelector('.usage-clock'), badge = h.querySelector('.usage-mark .hb');
        return { mark: mark.right, clock: clock ? clock.getBoundingClientRect().right : null, cols: cols.left, badge: [badge.offsetWidth, badge.offsetHeight] };
      }));
      assert.equal(marks.length, 2);
      for (const m of marks) {
        assert.deepEqual(m.badge, [19, 12], `${theme}: usage mark is the small badge`);
        assert.ok(m.mark <= m.cols && (m.clock === null || m.clock <= m.cols), `${theme}: usage mark overlaps its columns ${JSON.stringify(m)}`);
      }
      report.push({ theme, width: 1920, rail: g.rail, top: g.top, items: g.items.map(i => i.cls) });
      await shot(page, `${theme}-wide`);
      await shot(page, `${theme}-wide-strip`, { clip: { x: 0, y: 0, width: 1920, height: 60 } });
      // The usage popover opens under the center handle.
      await page.locator('.usage-toggle').click();
      const dialog = page.locator('.usage-pop');
      await dialog.waitFor({ state: 'visible' });
      const pop = await dialog.boundingBox(), handle = await page.locator('.usage-toggle').boundingBox();
      assert.ok(pop.y >= handle.y + handle.height, `${theme}: usage popover opens below its handle`);
      assert.ok(Math.abs(pop.x + pop.width / 2 - (handle.x + handle.width / 2)) <= 1, `${theme}: usage popover centers under its handle`);
      assert.equal(await page.locator('.usage-toggle').getAttribute('aria-expanded'), 'true');
      await shot(page, `${theme}-wide-usage`);
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });

      if (theme === 'dark') {
        // Beside the pinned column (1600-2199px) the rail overlays again.
        await page.setViewportSize({ width: 1800, height: 900 });
        await page.evaluate(() => document.documentElement.setAttribute('data-pincol', ''));
        await page.waitForTimeout(100);
        g = await geometry(page);
        checkStrip('dark/1800/pincol', g);
        assert.ok(g.railbtn && g.railbtn.l === 16 && g.railbtn.w === 34, 'dark/1800/pincol: rail button shows at the strip start');
        assert.equal(g.railZ, '45', 'dark/1800/pincol: the rail overlays');
        await page.evaluate(() => document.documentElement.removeAttribute('data-pincol'));

        // Narrow: the rail button hangs at the stage's top-left; the open
        // rail and its scrim cover the strip.
        await page.setViewportSize({ width: 1280, height: 900 });
        await page.waitForTimeout(200);
        g = await geometry(page);
        checkStrip('dark/1280', g);
        assert.deepEqual([g.railbtn.l, g.railbtn.t, g.railbtn.w, g.railbtn.h], [16, 0, 34, 34], 'dark/1280: rail button is a 34px handle at the top-left');
        await shot(page, 'dark-narrow');
        await shot(page, 'dark-narrow-strip', { clip: { x: 0, y: 0, width: 1280, height: 60 } });
        await page.locator('#railbtn').click();
        await page.waitForTimeout(400);
        g = await geometry(page);
        assert.ok(g.open, 'dark/1280: rail button opens the rail');
        assert.deepEqual([g.rail.l, g.rail.t], [12, 12], 'dark/1280: overlay rail sits 12px in');
        const covered = await page.evaluate(() => {
          const b = document.querySelector('#railbtn').getBoundingClientRect(), u = document.querySelector('.usage-toggle').getBoundingClientRect();
          const hit = (x, y) => { const e = document.elementFromPoint(x, y); return e && e.closest('#rail, #rscrim, #memo-pad') ? true : (e ? e.tagName + '.' + e.className + '#' + e.id : 'none'); };
          return [hit(b.left + b.width / 2, b.top + b.height / 2), hit(u.left + u.width / 2, u.top + u.height / 2)];
        });
        assert.deepEqual(covered, [true, true], 'dark/1280: the open rail, its scrim and memo pad cover the strip: ' + JSON.stringify(covered));
        await shot(page, 'dark-narrow-open');
        // The rail footer is Settings and Home alone, and fits at 360px.
        await page.setViewportSize({ width: 360, height: 760 });
        await page.waitForTimeout(300);
        const foot = await page.evaluate(() => { const f = document.querySelector('#rail .rfoot'); return { fits: f.scrollWidth <= f.clientWidth, kids: [...f.children].map(c => c.className) }; });
        assert.ok(foot.fits, 'footer fits at 360px');
        assert.deepEqual(foot.kids, ['btn gear settings-toggle', 'btn gear rail-home'], 'footer is Settings then Home');
        await page.keyboard.press('Escape');
        await page.waitForTimeout(300);

        // 760px, the desktop shell's minimum: the center steps down first
        // to its compact trigger; nothing that must stay is clipped.
        await page.setViewportSize({ width: 760, height: 700 });
        await page.waitForTimeout(300);
        g = await geometry(page);
        checkStrip('dark/760', g);
        // Grow the head until the strip runs short: the center compacts
        // first, the identity ellipsizes only after that, the low-priority
        // chips hide last, and the strip never overflows.
        const steps = [
          [0, null, null],
          [1, 'livesession-renamed-with-a-much-longer-label', null],
          [2, 'livesession-renamed-with-a-much-longer-label', 'a-project-folder-with-an-unusually-long-name-for-the-header-strip'],
        ];
        let state = null;
        for (const [step, label, project] of steps) {
          await page.evaluate(({ label, project }) => {
            const top = document.querySelector('.dwrap > .top');
            if (label) top.querySelector('.snm').textContent = label;
            if (project) {
              top.querySelector('.tph-r1').textContent = project;
              top.querySelector('.tph-chips').insertAdjacentHTML('afterbegin', '<span class="nolive">stores only</span><span class="ended-chip">ended</span>');
            }
          }, { label, project });
          await page.waitForTimeout(150);
          g = await geometry(page);
          checkStrip(`dark/760/step${step}`, g);
          state = await page.evaluate(() => {
            const top = document.querySelector('.dwrap > .top'), id = top.querySelector('.tph-id');
            const clip = e => e.scrollWidth > e.clientWidth + 0.5;
            return { tu: top.classList.contains('tu'), tch: top.classList.contains('tch'), clipped: clip(id) || [...id.querySelectorAll('.snm, .tph-r1')].some(clip) };
          });
          if (state.clipped) assert.ok(state.tu, `dark/760/step${step}: identity ellipsizes only after usage compacts`);
          if (state.tch) assert.ok(state.tu, `dark/760/step${step}: chips hide only after usage compacts`);
          for (const [name, b] of [['railbtn', g.railbtn], ['usage', g.usage], ['host', g.host]]) {
            assert.ok(b && b.w > 0 && b.l >= 0 && b.r <= g.vw, `dark/760/step${step}: ${name} shows unclipped`);
          }
          await shot(page, `dark-760-step${step}-strip`, { clip: { x: 0, y: 0, width: 760, height: 60 } });
          report.push({ width: 760, step, state, items: g.items.map(i => [i.cls, Math.round(i.box.l), Math.round(i.box.r)]), handles: g.handles.map(h => Math.round(h.l)) });
        }
        assert.ok(state.tu && state.tch && state.clipped, 'dark/760: the longest head compacts usage, hides chips and ellipsizes the identity ' + JSON.stringify(state));
        assert.equal(g.usage.w, 34, 'dark/760: compact usage trigger is 34px');
        await shot(page, 'dark-760');
        await shot(page, 'dark-760-strip', { clip: { x: 0, y: 0, width: 760, height: 60 } });
        await page.locator('.usage-toggle').click();
        await page.locator('.usage-pop').waitFor({ state: 'visible' });
        await shot(page, 'dark-760-usage');
        await page.keyboard.press('Escape');
        // Widening back restores the full handle.
        await page.setViewportSize({ width: 1920, height: 900 });
        await page.waitForTimeout(300);
        g = await geometry(page);
        assert.ok(!/\btu\b/.test(g.cls), 'dark/1920 again: plan usage shows in full');
      }
      assert.deepEqual(errors, [], theme + ': page errors ' + JSON.stringify(errors));
      await context.close();
    }
    console.log(JSON.stringify(report));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
