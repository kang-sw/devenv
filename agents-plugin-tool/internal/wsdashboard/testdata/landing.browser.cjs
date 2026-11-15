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

// landing reads the shown frame's landing: which lockup variant shows,
// whether the how-to shows, the lockup's centre against the main panel's,
// the free space above the lockup and below the how-to, and whether
// anything paints a background behind them.
async function landing(page) {
  const frame = page.frameLocator(on);
  await frame.locator('.dlanding').waitFor();
  return frame.locator('body').evaluate(() => {
    const vis = el => !!el && el.getClientRects().length > 0 && getComputedStyle(el).display !== 'none';
    const box = document.querySelector('.dlanding'), pad = parseFloat(getComputedStyle(box).paddingTop);
    const g = document.querySelector('#grid').getBoundingClientRect();
    const l = document.querySelector('.dlogo').getBoundingClientRect();
    const guide = document.querySelector('.dguide');
    const gr = guide.getBoundingClientRect();
    return {
      dark: vis(document.querySelector('.dlogo-d')), light: vis(document.querySelector('.dlogo-l')), guide: vis(guide),
      dx: Math.abs((l.left + l.right) / 2 - (g.left + g.right) / 2), dy: Math.abs((l.top + l.bottom) / 2 - (g.top + g.bottom) / 2),
      above: l.top - g.top - pad, below: g.bottom - gr.bottom - pad, scrolls: box.scrollHeight > box.clientHeight,
      width: l.width, gridWidth: g.width,
      bg: ['.dlanding', '.dlogo', '.dguide'].map(s => getComputedStyle(document.querySelector(s)).backgroundColor),
      steps: document.querySelectorAll('.dguide li').length
    };
  });
}

// root: the lockup alone at the panel's centre, nothing behind it.
function root(r, what) {
  assert.equal(r.guide, false, what + ': the how-to shows at /');
  assert.ok(r.dx < 2 && r.dy < 2, what + ': lockup off centre by ' + r.dx + ',' + r.dy);
  assert.deepEqual(r.bg, [clear, clear, clear], what + ': a background behind the landing');
  assert.ok(r.width <= 420.5 && r.width >= Math.min(220, r.gridWidth) - 0.5, what + ': lockup width ' + r.width);
}

// guide: the lockup over the how-to, centred as one group, nothing behind.
function guide(r, what) {
  assert.equal(r.guide, true, what + ': no how-to at /#guide');
  assert.equal(r.steps, 5, what + ': how-to steps');
  assert.ok(r.dx < 2, what + ': lockup off centre horizontally by ' + r.dx);
  assert.ok(!r.scrolls && Math.abs(r.above - r.below) < 2, what + ': group not centred (above ' + r.above + ', below ' + r.below + ')');
  assert.deepEqual(r.bg, [clear, clear, clear], what + ': a background behind the landing');
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
    root(r, 'root dark');
    await page.waitForTimeout(150);
    await shot(page, 'root-dark-1440.png');

    // Picking a session opens it; picking it again closes its view and
    // shows the guide at /#guide, and the row is no longer marked.
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/s/livesession');
    await page.waitForTimeout(400);
    await openRail(page);
    assert.equal(await cur(page, 'livesession'), true, 'the open session is marked');
    await shot(page, 'selected-row.png');
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/', '#guide');
    r = await landing(page);
    guide(r, 'after reselect');
    assert.equal(await cur(page, 'livesession'), false, 'no row marked after reselect');
    await page.waitForTimeout(300);
    await shot(page, 'after-reselect.png');
    await shot(page, 'guide-dark-1440.png');

    // Another row still opens as before; Enter on the open row closes it
    // like a click; Back returns to the session, and Back again to the guide.
    await openRail(page);
    await page.locator('#rail .rs[data-rid="boardsession"] .rl').click();
    await shows(page, '/s/boardsession');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="boardsession"] .rl').focus();
    await page.keyboard.press('Enter');
    await shows(page, '/', '#guide');
    await page.goBack();
    await shows(page, '/s/boardsession');
    await page.goBack();
    await shows(page, '/', '#guide');
    guide(await landing(page), 'back to the guide');
    await c.close();

    // Light: the dark-ink lockup, at the root and on the guide.
    ({ c, page } = await ctx('light', 1440));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [false, true], 'light theme shows the dark-ink lockup');
    root(r, 'root light');
    await page.waitForTimeout(150);
    await shot(page, 'root-light-1440.png');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/s/livesession');
    await openRail(page);
    await page.locator('#rail .rs[data-rid="livesession"] .rl').click();
    await shows(page, '/', '#guide');
    guide(await landing(page), 'guide light');
    await page.waitForTimeout(300);
    await shot(page, 'guide-light-1440.png');
    await c.close();

    // A picked theme wins over the OS: light picked on a dark OS, and dark
    // picked on a light one.
    ({ c, page } = await ctx('dark', 1440, [['ws_dashboard_theme', 'light']]));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [false, true], 'picked light on a dark OS shows the dark-ink lockup');
    await c.close();
    ({ c, page } = await ctx('light', 1440, [['ws_dashboard_theme', 'dark']]));
    await page.goto(base + '/');
    r = await landing(page);
    assert.deepEqual([r.dark, r.light], [true, false], 'picked dark on a light OS shows the light-ink lockup');
    await c.close();

    // Narrow, opened at /#guide directly.
    ({ c, page } = await ctx('dark', 700));
    await page.goto(base + '/#guide');
    await shows(page, '/', '#guide');
    r = await landing(page);
    assert.equal(r.guide, true, 'narrow guide shows');
    assert.ok(r.dx < 2, 'narrow lockup off centre horizontally by ' + r.dx);
    await page.waitForTimeout(150);
    await shot(page, 'guide-dark-700.png');
    await c.close();

    assert.deepEqual(errors, []);
    console.log('PASS: root lockup centred per theme; reselect closes the view to /#guide and its how-to; other rows open as before');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
