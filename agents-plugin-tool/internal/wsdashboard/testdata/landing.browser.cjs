// Run by TestLandingGuideBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

const host = new URL(base).hostname;
const on = '#frames iframe.on';
const clear = 'rgba(0, 0, 0, 0)';

async function shot(page, name) {
  if (shots) await page.screenshot({ path: path.join(shots, name) });
}

// fullShot saves the whole landing: the viewport grows to the landing's
// scroll height (its inner scroll otherwise clips it), every screenshot
// loads, and the viewport returns.
async function fullShot(page, name) {
  if (!shots) return;
  const size = page.viewportSize();
  const extra = await page.frameLocator(on).locator('.dlanding').evaluate(el => el.scrollHeight - el.clientHeight);
  await page.setViewportSize({ width: size.width, height: size.height + extra });
  await page.waitForTimeout(200);
  await loaded(page);
  await page.screenshot({ path: path.join(shots, name) });
  await page.setViewportSize(size);
}

// loaded scrolls the landing through, so every lazy screenshot of the
// shown theme loads, and waits for them.
async function loaded(page) {
  const frame = page.frameLocator(on);
  await frame.locator('.dlanding').evaluate(async el => {
    for (let y = 0; y <= el.scrollHeight; y += el.clientHeight / 2) {
      el.scrollTop = y;
      await new Promise(r => setTimeout(r, 60));
    }
    el.scrollTop = 0;
  });
  await frame.locator('body').evaluate(() => Promise.all([...document.querySelectorAll('.dshot img')]
    .filter(img => getComputedStyle(img).display !== 'none')
    .map(img => img.complete && img.naturalWidth ? null : img.decode().catch(() => {}))));
}

// landing reads the shown frame's landing: which lockup variant and which
// screenshot theme show, the lockup's and the how-to's centres against the
// main panel's, their order, whether the page scrolls (down, never
// across), whether every step's screenshot loaded and fits the column, which
// guide images the frame fetched, and whether anything paints a background
// behind them.
async function landing(page) {
  const frame = page.frameLocator(on);
  await frame.locator('.dlanding').waitFor();
  await loaded(page);
  return frame.locator('body').evaluate(() => {
    const vis = el => !!el && el.getClientRects().length > 0 && getComputedStyle(el).display !== 'none';
    const box = document.querySelector('.dlanding');
    const g = document.querySelector('#grid').getBoundingClientRect();
    const l = document.querySelector('.dlogo').getBoundingClientRect();
    const guide = document.querySelector('.dguide');
    const gr = guide.getBoundingClientRect();
    const steps = [...document.querySelectorAll('.dguide .dstep')];
    const shown = steps.map(li => [...li.querySelectorAll('.dshot img')].filter(vis));
    const fetched = performance.getEntriesByType('resource').map(e => new URL(e.name).pathname).filter(p => p.startsWith('/s/guide-'));
    return {
      dark: vis(document.querySelector('.dlogo-d')), light: vis(document.querySelector('.dlogo-l')), guide: vis(guide),
      dx: Math.abs((l.left + l.right) / 2 - (g.left + g.right) / 2), gx: Math.abs((gr.left + gr.right) / 2 - (g.left + g.right) / 2),
      order: l.bottom <= gr.top, scrolls: box.scrollHeight > box.clientHeight, across: box.scrollWidth > box.clientWidth,
      width: l.width, gridWidth: g.width, guideWidth: gr.width,
      bg: ['.dlanding', '.dlogo', '.dguide'].map(s => getComputedStyle(document.querySelector(s)).backgroundColor),
      steps: steps.length, titles: steps.map(li => vis(li.querySelector('h3')) && li.querySelector('h3').textContent.trim() !== ''),
      shown: shown.map(imgs => imgs.length),
      themes: shown.flat().map(img => /-(dark|light)-/.exec(img.currentSrc || img.src)[1]),
      loaded: shown.flat().every(img => img.complete && img.naturalWidth > 0),
      fits: shown.flat().every(img => img.getBoundingClientRect().right <= gr.right + 0.5 && img.getBoundingClientRect().width > 0),
      fetched
    };
  });
}

// root: the lockup over the illustrated how-to with nothing behind them,
// both centred across, seven titled steps each with one loaded screenshot of
// the theme shown (the other theme's never fetched) that fits the column,
// and a page that scrolls down but never across.
function root(r, what, theme) {
  assert.equal(r.guide, true, what + ': no how-to');
  assert.equal(r.steps, 7, what + ': how-to steps');
  assert.deepEqual(r.titles, Array(7).fill(true), what + ': untitled step');
  assert.deepEqual(r.shown, Array(7).fill(1), what + ': screenshots shown per step');
  assert.deepEqual([...new Set(r.themes)], [theme], what + ': screenshot theme');
  assert.ok(r.loaded, what + ': a screenshot did not load');
  assert.ok(r.fits, what + ': a screenshot overflows the column');
  assert.ok(r.fetched.every(p => p.includes('-' + theme + '-')), what + ': fetched ' + r.fetched.join(' '));
  assert.ok(r.dx < 2, what + ': lockup off centre horizontally by ' + r.dx);
  assert.ok(r.gx < 2, what + ': how-to off centre horizontally by ' + r.gx);
  assert.ok(r.order, what + ': the lockup is not above the how-to');
  assert.ok(r.scrolls, what + ': the landing does not scroll as a page');
  assert.ok(!r.across, what + ': the landing scrolls across');
  assert.ok(r.guideWidth <= 800.5, what + ': how-to column ' + r.guideWidth);
  assert.deepEqual(r.bg, [clear, clear, clear], what + ': a background behind the landing');
  assert.ok(r.width <= 420.5 && r.width >= Math.min(220, r.gridWidth) - 0.5, what + ': lockup width ' + r.width);
}

async function openRail(page) {
  await page.locator('#railbtn').click();
  await page.locator('#rail').evaluate(el => new Promise(r => {
    const t = () => (getComputedStyle(el).visibility === 'visible' && el.getBoundingClientRect().left >= 0 ? r() : requestAnimationFrame(t));
    t();
  }));
  await page.waitForTimeout(260);
}

// shows waits for the shell to show the page at pathname (and hash) in its
// shown frame, its address bar naming it.
async function shows(page, pathname, hash = '') {
  await page.waitForFunction(([p, h]) => {
    const el = document.querySelector('#frames iframe.on');
    if (location.pathname !== p || location.hash !== h || !el) return false;
    try { return el.contentWindow.location.pathname === p && el.contentWindow.location.hash === h; } catch (e) { return false; }
  }, [pathname, hash]);
}

const cur = (page, id) => page.locator('#rail .rs[data-rid="' + id + '"]').evaluate(el => el.classList.contains('cur'));

(async () => {
  const browser = await chromium.launch({ headless: true });
  const errors = [];
  try {
    const ctx = async (scheme, width, cookies) => {
      const c = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, locale: 'ko-KR' });
      if (cookies) await c.addCookies(cookies.map(([name, value]) => ({ name, value, domain: host, path: '/' })));
      const page = await c.newPage();
      page.on('pageerror', e => errors.push(e.message));
      return { c, page };
    };

    // The root, dark: the light-ink lockup alone at the panel's centre.
    let { c, page } = await ctx('dark', 1440);
    await page.goto(base + '/');
    let r = await landing(page);
    assert.deepEqual([r.dark, r.light], [true, false], 'dark theme shows the light-ink lockup');
    root(r, 'root dark', 'dark');
    await fullShot(page, 'root-dark-1440.png');

    // Picking a session opens it; picking it again closes its view and
    // shows the landing at /, and the row is no longer marked.
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/s/livesession');
    await page.waitForTimeout(400);
    await openRail(page);
    assert.equal(await cur(page, 'livesession'), true, 'the open session is marked');
    await shot(page, 'selected-row.png');
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/');
    r = await landing(page);
    root(r, 'after reselect', 'dark');
    assert.equal(await cur(page, 'livesession'), false, 'no row marked after reselect');
    await page.waitForTimeout(300);
    await shot(page, 'after-reselect.png');

    // Another row still opens as before; Enter on the open row closes it
    // like a click; Back returns to the session, and Back again to /.
    await openRail(page);
    await page.locator('#rail .rs[data-rid="boardsession"] .rl').click();
    await shows(page, '/s/boardsession');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="boardsession"] .rl').focus();
    await page.keyboard.press('Enter');
    await shows(page, '/');
    await page.goBack();
    await shows(page, '/s/boardsession');
    await page.goBack();
    await shows(page, '/');
    root(await landing(page), 'back to the landing', 'dark');
    await c.close();

    // Light: the dark-ink lockup, at the root and after a reselect.
    ({ c, page } = await ctx('light', 1440));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [false, true], 'light theme shows the dark-ink lockup');
    root(r, 'root light', 'light');
    await fullShot(page, 'root-light-1440.png');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/s/livesession');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/');
    root(await landing(page), 'landing light after reselect', 'light');
    await c.close();

    // A picked theme wins over the OS: light picked on a dark OS, and dark
    // picked on a light one.
    ({ c, page } = await ctx('dark', 1440, [['ws_dashboard_theme', 'light']]));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [false, true], 'picked light on a dark OS shows the dark-ink lockup');
    root(r, 'picked light', 'light');
    await c.close();
    ({ c, page } = await ctx('light', 1440, [['ws_dashboard_theme', 'dark']]));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [true, false], 'picked dark on a light OS shows the light-ink lockup');
    root(r, 'picked dark', 'dark');
    await c.close();

    // Narrow: the same page, nothing clipped or scrolled across.
    ({ c, page } = await ctx('dark', 700));
    await page.goto(base + '/');
    root(await landing(page), 'root dark 700', 'dark');
    await fullShot(page, 'root-dark-700.png');
    // An earlier /#guide address lands on the same page.
    await page.goto(base + '/#guide');
    await shows(page, '/', '#guide');
    root(await landing(page), '/#guide', 'dark');
    await c.close();

    assert.deepEqual(errors, []);
    console.log('PASS: illustrated landing per theme at / and 700px, only the shown theme fetched; reselect closes the view to /; other rows open as before');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
