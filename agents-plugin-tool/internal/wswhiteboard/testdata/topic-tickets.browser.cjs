// args: board.html shotdir. Topics "none" (no tickets), "one" (one ticket)
// and "three" (three tickets, long label), each waiting behind the amber
// clock. Checks the ticket icon's place right after the clock inside the bar,
// the count badge, no glow, and the popover rows, in both themes at 1440 and
// 700px, and leaves one screenshot per theme and width with the popover open.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, shots] = process.argv.slice(2);

const measure = (p) => p.evaluate(() => {
  const box = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { l: r.left, r: r.right, t: r.top, b: r.bottom, cy: (r.top + r.bottom) / 2 }; };
  const out = {};
  for (const name of ['none', 'one', 'three']) {
    const topic = document.querySelector(`.topic[data-topic="${name}"]`);
    const bar = topic.querySelector(':scope > .tophead > .tbar');
    const lead = bar.querySelector(':scope > .tlead');
    const clock = lead.querySelector(':scope > .dw:not(.wbtk) .dbtn');
    const icon = lead.querySelector(':scope > .dw.wbtk .dbtn');
    out[name] = {
      bar: box(bar), clock: box(clock), icon: box(icon),
      overflow: bar.scrollWidth - bar.clientWidth,
      next: icon ? icon.parentElement.previousElementSibling.className : null,
      badge: icon && icon.querySelector('.bd') ? icon.querySelector('.bd').textContent : null,
      shadow: getComputedStyle(topic).boxShadow,
      live: topic.classList.contains('live'),
    };
  }
  return out;
});

async function run(browser, theme, width) {
  const p = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: theme });
  const fail = [];
  try {
    await p.goto('file://' + file);
    await p.evaluate((theme) => { document.documentElement.dataset.theme = theme; }, theme);
    await p.waitForTimeout(300);
    const m = await measure(p);
    const tag = `${theme}@${width}`;
    if (m.none.icon) fail.push(`${tag}: a topic without tickets shows the icon`);
    for (const name of ['one', 'three']) {
      const t = m[name];
      if (!t.icon) { fail.push(`${tag} ${name}: no ticket icon`); continue; }
      if (t.next !== 'dw') fail.push(`${tag} ${name}: icon follows ${t.next}, not the clock`);
      const gap = t.icon.l - t.clock.r;
      // Same line as the clock, immediately after it.
      if (Math.abs(t.icon.cy - t.clock.cy) > 2 || gap < 0 || gap > 12) fail.push(`${tag} ${name}: icon not beside the clock (gap ${gap}, dy ${t.icon.cy - t.clock.cy})`);
      if (t.icon.r > t.bar.r || t.icon.l < t.bar.l || t.icon.b > t.bar.b || t.icon.t < t.bar.t) fail.push(`${tag} ${name}: icon outside its bar`);
      if (t.overflow > 0) fail.push(`${tag} ${name}: bar overflows by ${t.overflow}px`);
      if (t.live || t.shadow !== m.none.shadow) fail.push(`${tag} ${name}: ticket-derived glow (${t.shadow})`);
    }
    if (m.one.badge !== null) fail.push(`${tag}: one ticket shows a count ${m.one.badge}`);
    if (m.three.badge !== '3') fail.push(`${tag}: three tickets show count ${m.three.badge}`);

    // A click pins the popover open; read its rows.
    const three = p.locator('.topic[data-topic="three"] .dw.wbtk');
    await three.locator('.dbtn').click();
    await p.waitForTimeout(100);
    const pop = await three.evaluate((w) => {
      const pop = w.querySelector('.pop'), r = pop.getBoundingClientRect();
      return {
        visible: !pop.hidden && r.width > 0,
        expanded: w.querySelector('.dbtn').getAttribute('aria-expanded'),
        box: { l: r.left, r: r.right, t: r.top, b: r.bottom },
        label: pop.querySelector('.lb').textContent,
        rows: [...pop.querySelectorAll('.row')].map((row) => ({
          cls: row.className, stem: row.querySelector('.nm').textContent, status: row.querySelector('.st').textContent,
          mark: row.firstElementChild.matches('svg') ? row.firstElementChild.querySelector('use').getAttribute('href') : row.firstElementChild.className,
          strike: getComputedStyle(row.querySelector('.nm')).textDecorationLine,
          dim: getComputedStyle(row.querySelector('.nm')).opacity,
        })),
      };
    });
    if (!pop.visible || pop.expanded !== 'true') fail.push(`${tag}: click does not open the popover`);
    if (pop.box.l < 0 || pop.box.r > width || pop.box.t < 0 || pop.box.b > 900) fail.push(`${tag}: popover leaves the viewport ${JSON.stringify(pop.box)}`);
    assert.equal(pop.label, '연관 티켓 3개', tag);
    assert.deepEqual(pop.rows.map((r) => [r.cls, r.stem, r.status, r.mark]), [
      ['row open', '261114-feat-ws-whiteboard-topic-state-and-lead-nudges', '열림 · ready', 'wb-dot'],
      ['row done', '261113-bug-board-ticket-status-resolver', '완료', '#i-check'],
      ['row dropped', '261112-refactor-retired-badge-glow', '취소됨', '#i-x'],
    ], tag);
    const [open, done, dropped] = pop.rows;
    if (open.dim !== '1' || done.dim === '1' || dropped.dim === '1') fail.push(`${tag}: done/dropped stems not dimmed (${open.dim}/${done.dim}/${dropped.dim})`);
    if (dropped.strike !== 'line-through' || done.strike === 'line-through') fail.push(`${tag}: dropped stem not struck (${dropped.strike}/${done.strike})`);
    await p.screenshot({ path: path.join(shots, `topic-tickets-${theme}-${width}.png`) });

    // Escape closes it; keyboard focus and hover open it again.
    await p.keyboard.press('Escape');
    await p.waitForTimeout(50);
    if (await three.locator('.pop').isVisible()) fail.push(`${tag}: Escape leaves the popover open`);
    await p.locator('.topic[data-topic="one"] .dw.wbtk .dbtn').focus();
    await p.waitForTimeout(50);
    if (!(await p.locator('.topic[data-topic="one"] .dw.wbtk .pop').isVisible())) fail.push(`${tag}: focus does not open the popover`);
    await p.keyboard.press('Escape');
    await p.mouse.move(0, 0);
    await p.locator('.topic[data-topic="one"] .dw.wbtk .dbtn').hover();
    await p.waitForTimeout(250);
    const hover = p.locator('.topic[data-topic="one"] .dw.wbtk .pop');
    if (!(await hover.isVisible())) fail.push(`${tag}: hover does not open the popover`);
    else assert.deepEqual(await hover.locator('.row').evaluateAll((rows) => rows.map((r) => [r.className, r.querySelector('.nm').textContent, r.querySelector('.st').textContent])), [['row open', '261114-feat-history-fold-rule', '열림 · todo']], tag);
    console.log(tag, JSON.stringify({ one: m.one.icon, three: m.three.icon, pop: pop.box }));
  } finally {
    await p.close();
  }
  return fail;
}

(async () => {
  const browser = await chromium.launch({ headless: true, args: ['--disable-background-networking', '--no-proxy-server'] });
  const fail = [];
  try {
    for (const theme of ['dark', 'light']) for (const width of [1440, 700]) fail.push(...await run(browser, theme, width));
  } finally {
    await browser.close();
  }
  if (fail.length) { console.error(fail.join('\n')); process.exit(1); }
  console.log('ok');
})().catch((err) => { console.error(err); process.exit(1); });
