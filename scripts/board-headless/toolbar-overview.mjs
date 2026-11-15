import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createSharedActions } from './shared-actions.mjs';

export function createToolbarOverview({ assert, log, sleep, openBoard, waitQuiet, waitFor, assertChanged, IN_PAGE, loadPlaywright, BROWSERS, run, REPO }) {
// --- toolbar / overview QA ----------------------------------------------------

// Browser-local fixtures deliberately use the real cards framework and stream
// region path. They do not replace overview, keep, placement, or input handlers.
const OVERVIEW_PAGE = () => {
  const bh = window.__bh, root = document.documentElement;
  const items = () => [...new Set([...document.querySelectorAll('#r-board section.topic[data-topic], #r-board .bcard[data-card], #bpins section.topic[data-topic], #bpins .bcard[data-card]')])];
  const id = (n) => n && (n.dataset.card ? 'card:' + n.dataset.card : 'topic:' + n.dataset.topic);
  const rect = (n) => { const r = n.getBoundingClientRect(); return { top: r.top, left: r.left, width: r.width, height: r.height, bottom: r.bottom }; };
  bh.ovItems = items;
  bh.ovId = id;
  bh.ovFind = (key) => items().find((n) => id(n) === key);
  bh.ovScroller = (n) => {
    for (let p = n.parentElement; p && p !== document.body && p !== root; p = p.parentElement) {
      if (/auto|scroll|overlay/.test(getComputedStyle(p).overflowY) && p.scrollHeight > p.clientHeight) return p;
    }
    return document.scrollingElement;
  };
  bh.ovViewport = (s) => s === document.scrollingElement ? { top: 0, bottom: innerHeight } :
    { top: s.getBoundingClientRect().top + s.clientTop, bottom: s.getBoundingClientRect().top + s.clientTop + s.clientHeight };
  bh.ovLandingError = (n) => {
    const s = bh.ovScroller(n), v = bh.ovViewport(s), pad = parseFloat(getComputedStyle(s).scrollPaddingTop) || 0;
    return n.getBoundingClientRect().top - v.top - pad;
  };
  bh.ovHeadState = (n = document.activeElement) => {
    const s = bh.ovScroller(n), v = bh.ovViewport(s), pad = parseFloat(getComputedStyle(s).scrollPaddingTop) || 0;
    const head = n.querySelector('.tophead > .tbar, .bc-hd'), h = head.getBoundingClientRect(), face = document.querySelector('.board-toolbar-face').getBoundingClientRect();
    const owner = document.elementFromPoint((h.left + h.right) / 2, (h.top + h.bottom) / 2);
    const overlapsFaceX = Math.min(h.right, face.right) > Math.max(h.left, face.left);
    return { key: id(n), focused: document.activeElement === n, scroller: s === document.scrollingElement ? 'document' : s.id, viewport: v, pad,
      item: rect(n), head: rect(head), face: rect(document.querySelector('.board-toolbar-face')), owner: owner && { tag: owner.tagName, cls: owner.className },
      ownsCenter: !!owner && (owner === head || head.contains(owner)),
      exposed: h.top >= v.top + pad - 1 && h.bottom <= v.bottom + 1 && (!overlapsFaceX || h.top >= face.bottom - 1) };
  };
  bh.ovState = () => {
    const s = bh.scroller(), t = document.querySelector('.board-toolbar'), v = bh.viewTop(s), b = bh.viewBottom(s), tr = rect(t.querySelector('.board-toolbar-face'));
    const floating = t.hasAttribute('data-floating'), line = floating ? Math.max(v, tr.bottom) : v;
    const anchor = !floating && tr.bottom > v && tr.top < b ? null : items().find((n) => bh.board_().contains(n) && rect(n).bottom > line && rect(n).top < b);
    return { st: s.scrollTop, top: v, bottom: b, toolbar: tr, key: id(anchor), y: anchor && rect(anchor).top,
      width: rect(bh.board_()).width, pinTop: document.querySelector('#bpins').scrollTop,
      pincol: root.hasAttribute('data-pincol'), here: id(document.querySelector('.overview-here')), hereY: document.querySelector('.overview-here') && rect(document.querySelector('.overview-here')).top,
      active: WsOverview.active(), holding: WsOverview.holding(), order: items().map(id),
      storage: JSON.stringify(Object.entries(localStorage).filter(([k]) => k.startsWith('ws.dashboard.cards.'))) };
  };
  bh.ovPosition = (where) => {
    const flow = [...bh.board_().children].filter((n) => n.matches('section.topic:not(.closed)'));
    if (where === 'top') bh.scroller().scrollTop = 0;
    else {
      const n = flow[where === 'bottom' ? flow.length - 2 : Math.floor(flow.length / 2)];
      if (!n) throw new Error('fixture needs at least two unpinned topics');
      bh.scroller().scrollTop += rect(n).top - bh.viewTop(bh.scroller()) + 36;
    }
  };
  bh.ovSeed = (pins = 2, cards = true) => {
    if (cards) {
      for (const kind of ['terminal', 'file', 'preview', 'web']) {
        const name = 'bh-' + kind;
        WsCards.register(name, { base: 180, open(c) { c.title({ name: 'QA ' + kind, icon: 'file' }); c.body.textContent = 'Browser-local ' + kind + ' body'; } });
        WsCards.open(name, {});
      }
    }
    const ts = bh.topics().slice(-pins);
    ts.forEach((n, i) => WsCards.move(n, { pin: true, index: i }));
    // The serving fixture has body-only topics. Add a real-shaped recap to
    // one, so the max-four-line clamp and content-sized header are exercised.
    const n = bh.topics().find((n) => !n.closest('#bpins'));
    if (n && !n.querySelector('.tsub')) {
      const sub = document.createElement('div'); sub.className = 'tsub';
      sub.innerHTML = '<div class="trecap"><div class="rtext md">' + 'Long current-state recap text with several wrapped lines. '.repeat(18) + '</div></div>';
      n.querySelector('.tophead').appendChild(sub);
    }
  };
  bh.ovFold = () => items().map((n) => {
    const head = n.querySelector('.tophead, .bc-hd'), text = n.querySelector('.trecap .rtext'), slot = n.querySelector('.tsub > .trecap');
    const bodies = n.matches('.bcard') ? [...n.querySelectorAll('.bc-body, .bc-grip, .bc-folded')] : [...n.children].filter((x) => x !== head);
    return { id: id(n), h: rect(n).height, head: rect(head).height, cv: getComputedStyle(n).contentVisibility,
      hidden: bodies.every((x) => getComputedStyle(x).display === 'none'),
      recap: text && rect(text).height, line: text && parseFloat(getComputedStyle(text).lineHeight),
      clamp: text && getComputedStyle(text).webkitLineClamp,
      slot: slot && rect(slot).height, slotLine: slot && parseFloat(getComputedStyle(slot).lineHeight),
      slotClamp: slot && getComputedStyle(slot).webkitLineClamp };
  });
  bh.ovRegion = (html) => {
    const hash = 'bhqa' + Date.now().toString(16), ws = bh.sockets.find((s) => s.readyState === 1 && typeof s.onmessage === 'function');
    if (!ws) throw new Error('no live board socket');
    ws.onmessage({ data: JSON.stringify({ event: 'region', data: { name: 'board', hash, html } }) });
    bh.board = { hash, html };
    return hash;
  };
  bh.ovLands = [];
  document.addEventListener('ws-overview-land', (e) => {
    const { target, from, to } = e.detail;
    const r = (x) => ({ top: x.top, left: x.left, width: x.width, height: x.height });
    const land = { id: id(target), from: r(from), to: r(to), connected: e.target.isConnected };
    bh.ovLands.push(land);
    requestAnimationFrame(() => { land.next = rect(target); });
  });
  bh.ovMotion = (entering) => new Promise((resolve) => {
    const before = items().map((n) => ({ id: id(n), r: rect(n) })), samples = [], landsAt = bh.ovLands.length;
    let start;
    const take = (now = performance.now()) => ({ t: Math.max(0, now - start), active: WsOverview.active(), holding: WsOverview.holding(),
      st: bh.scroller().scrollTop, here: document.querySelector('.overview-here') && rect(document.querySelector('.overview-here')).top,
      rows: items().map((n) => ({ id: id(n), r: rect(n), motion: n.hasAttribute('data-overview-motion'), height: n.style.height,
        transition: getComputedStyle(n).transitionDuration })),
      slot: rect(document.querySelector('.overview-slot') || document.querySelector('#bpins')),
      ghosts: [...document.querySelectorAll('.overview-ghost')].map((n) => ({ r: rect(n), target: (() => { const key = JSON.parse(n.dataset.overviewTarget); return rect(bh.ovFind(key.card ? 'card:' + key.card : 'topic:' + key.topic)); })(), opacity: getComputedStyle(n).opacity,
        unsafe: !!n.querySelector('iframe, .bc-body, .xterm, .doc-body, input, textarea'), children: [...n.children].map((x) => x.className) })) });
    const t0 = performance.now(), nativeNow = performance.now;
    // Observe (never change) the implementation clock origin. Sampling with
    // the rAF timestamp below avoids treating callback/layout latency as a
    // different easing, even on a 120Hz browser.
    let clockStart = null;
    performance.now = function () {
      const now = nativeNow.call(performance);
      if (clockStart === null && /at motion\b/.test(new Error().stack || '')) clockStart = now;
      return now;
    };
    try { if (entering) WsOverview.toggle(); else WsOverview.exit(); }
    finally { performance.now = nativeNow; }
    start = clockStart === null ? performance.now() : clockStart;
    const initial = take(); initial.t = 0; samples.push(initial);
    // A click while the one clock runs must not invoke an underlying control.
    if (initial.rows.some((r) => r.motion)) {
      bh.ovDuringClick = 0;
      const b = document.querySelector('.board-search');
      const listener = () => { bh.ovDuringClick++; };
      b.addEventListener('click', listener, { once: true });
      b.click(); b.removeEventListener('click', listener);
    }
    let tail = 0;
    function frame(now) {
      const s = take(now); samples.push(s);
      if (!s.rows.some((r) => r.motion) && !s.ghosts.length && (entering ? s.active : !s.holding)) tail++; else tail = 0;
      if (tail >= 3) resolve({ before, samples, lands: bh.ovLands.slice(landsAt), setupMs: start - t0, clockObserved: clockStart !== null, clicks: bh.ovDuringClick || 0 });
      else if (s.t > 2000) resolve({ before, samples, lands: bh.ovLands.slice(landsAt), timeout: true });
      else requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  });
};

function qa(cond, label, detail) {
  assert(cond, label + (detail === undefined ? '' : ': ' + JSON.stringify(detail)));
  log('ok: ' + label);
}
async function overviewReady(ctx, options = {}) {
  const p = await openBoard(ctx);
  await p.page.evaluate(OVERVIEW_PAGE);
  if (options.seed !== false) { await p.page.evaluate(([pins, cards]) => window.__bh.ovSeed(pins, cards), [options.pins || 2, options.cards !== false]); await waitQuiet(p.page); }
  log(`page kind: ${p.kind}; viewport ${ctx.width}x${ctx.height}; PIN_WIDE=1600`);
  return p;
}
async function ovDone(page, on) {
  await page.waitForFunction((on) => WsOverview.active() === on && !document.querySelector('[data-overview-motion], .overview-ghost') && (on || !WsOverview.holding()), on, { timeout: 4000, polling: 'raf' });
}
async function ovEnter(page) { await page.evaluate(() => WsOverview.toggle()); await ovDone(page, true); }
async function ovExit(page) { await page.evaluate(() => WsOverview.exit()); await ovDone(page, false); }
async function ovAt(page, where) { await page.evaluate((w) => window.__bh.ovPosition(w), where); await waitQuiet(page); return page.evaluate(() => window.__bh.ovState()); }
async function capture(ctx, tab, name, data) {
  if (!ctx.args['capture-dir']) return;
  const dir = path.resolve(ctx.args['capture-dir']); fs.mkdirSync(dir, { recursive: true });
  const stem = `${ctx.args._[0]}-${ctx.width}-${name}`;
  if (data) fs.writeFileSync(path.join(dir, stem + '.json'), JSON.stringify(data, null, 2));
  await tab.screenshot({ path: path.join(dir, stem + '.png') });
}
async function checkRestore(page, before, label) {
  await ovDone(page, false);
  // Frames hidden by the fold can report their restored heights after the
  // instant exit. The keep contract covers those late reports too.
  await waitQuiet(page);
  const after = await page.evaluate(() => window.__bh.ovState());
  qa(Math.abs(before.st - after.st) <= 1, label + ' restores scroll within 1px', { before: before.st, after: after.st });
  qa(before.storage === after.storage, label + ' never persists overview in card storage');
}
async function checkFold(page) {
  const rows = await page.evaluate(() => window.__bh.ovFold());
  qa(rows.length > 0 && rows.every((r) => r.hidden && r.h <= r.head + 2.01 && r.cv === 'visible'), 'all topics/cards (including offscreen) fold to their headers', rows.filter((r) => !r.hidden || r.h > r.head + 2.01 || r.cv !== 'visible'));
  const recaps = rows.filter((r) => r.recap !== null);
  qa(recaps.length > 0 && recaps.every((r) => r.clamp === '4' && r.recap <= r.line * 4 + 1), 'existing recap is clamped to at most four lines', recaps);
  const slots = rows.filter((r) => r.slot !== null);
  qa(slots.length > 0 && slots.every((r) => r.slotClamp === '4' && r.slot > 0 && r.slot <= r.slotLine * 4 + 1), 'header recap uses at most four lines', slots);
}

async function readWebPopover(page) {
  return page.evaluate(() => {
    const p = document.querySelector('.wpop'), r = p.getBoundingClientRect(), b = document.querySelector('.wnew').getBoundingClientRect();
    return { focused: document.activeElement === p.querySelector('input'), left: r.left, right: r.right, top: r.top, bottom: r.bottom,
      buttonBottom: b.bottom, buttonWidth: b.width, menuWidth: r.width, menuHeight: r.height, viewport: [innerWidth, innerHeight] };
  });
}
function webContained(p) {
  return p.focused && p.left >= 0 && p.right <= p.viewport[0] && p.top >= p.buttonBottom && p.bottom <= p.viewport[1];
}
async function baselineWeb(ctx) {
  // Chromium treats route.fulfill documents as non-local address space and
  // blocks their loopback WebSockets. This static containment probe uses no
  // stream/keep/swap assertion; candidate integration still requires a socket.
  const baseline = { ...ctx, assetRef: ctx.args['baseline-ref'], requireStream: false, requireFrameHeights: false, errors: [] };
  const { browser, page, tab, kind } = await overviewReady(baseline);
  try {
    await page.evaluate((width) => {
      const bar = document.querySelector('.board-toolbar');
      if (width) { bar.style.width = width + 'px'; bar.style.maxWidth = '100%'; }
      __bh.scroller().scrollTop = 0;
    }, +(ctx.args['face-width'] || 0));
    await waitQuiet(page);
    await page.focus('.wnew'); await tab.keyboard.press('Enter');
    await page.waitForSelector('.wpop:not([hidden])');
    const p = await readWebPopover(page);
    const replacements = [...new Set(baseline.assetReplacements)];
    assert(replacements.length === 5, 'baseline must substitute every changed production asset: ' + replacements.join(', '));
    assert(!baseline.errors.length, 'baseline page errors: ' + baseline.errors.join(' | '));
    await capture(ctx, tab, kind + '-web-baseline', { ref: baseline.assetRef, replacements, predicate: webContained(p), bounds: p });
    return { ref: baseline.assetRef, replacements, hashes: baseline.assetHashes, predicate: webContained(p), bounds: p,
      limit: 'Baseline-only static Web UI probe; route-fulfilled baseline has no live WebSocket acceptance.' };
  } finally { await browser.close(); }
}
async function assertWebContainment(ctx, p) {
  const label = 'Web Enter opens the real viewport-fitting URL/path popover';
  try { qa(webContained(p), label, p); }
  catch (failure) {
    if (!ctx.args['baseline-ref']) throw failure;
    const baseline = await baselineWeb(ctx), b = baseline.bounds;
    // Exact width/kind/face evidence only. Compare horizontal coordinates,
    // intrinsic menu layout, focus and failing overflow edges. Absolute y is
    // intentionally not equated: the floating face changes that coordinate.
    const fields = ['left', 'right', 'buttonWidth', 'menuWidth', 'menuHeight'];
    const equivalent = !baseline.predicate && p.focused === b.focused &&
      JSON.stringify(p.viewport) === JSON.stringify(b.viewport) && fields.every((k) => Math.abs(p[k] - b[k]) <= .1) &&
      (p.left < 0) === (b.left < 0) && (p.right > p.viewport[0]) === (b.right > b.viewport[0]) &&
      (p.top < p.buttonBottom) === (b.top < b.buttonBottom) && (p.bottom > p.viewport[1]) === (b.bottom > b.viewport[1]);
    const report = { assertion: label, candidatePredicate: false, candidate: p, baseline, comparedFields: fields, equivalent,
      verdict: equivalent ? 'known-baseline-failure' : 'regression-or-unclassified',
      attributionLimit: 'Equivalence establishes only this containment defect, not absence of other regressions.' };
    const dir = ctx.args['capture-dir'];
    assert(dir, '--baseline-ref requires --capture-dir so every classification retains exact evidence');
    fs.mkdirSync(path.resolve(dir), { recursive: true });
    const output = path.join(path.resolve(dir), 'web-containment-comparison.json');
    fs.writeFileSync(output, JSON.stringify(report, null, 2));
    log(`${report.verdict}: ${JSON.stringify(report)}; evidence ${output}`);
    if (!equivalent) throw failure;
    (ctx.knownBaselineFailures ||= []).push(report);
  }
}
async function scenarioWebContainment(ctx) {
  const { page, tab, kind } = await overviewReady(ctx);
  await page.evaluate((width) => {
    if (width) { const bar = document.querySelector('.board-toolbar'); bar.style.width = width + 'px'; bar.style.maxWidth = '100%'; }
    __bh.scroller().scrollTop = 0;
  }, +(ctx.args['face-width'] || 0));
  await waitQuiet(page);
  await page.focus('.wnew'); await tab.keyboard.press('Enter'); await page.waitForSelector('.wpop:not([hidden])');
  const p = await readWebPopover(page);
  await capture(ctx, tab, kind + '-web-current', { predicate: webContained(p), bounds: p });
  await assertWebContainment(ctx, p);
}

// Measurements deliberately use production typography. The reference's 46/28,
// 76/48 and 82/60 are comparison evidence, not asserted production heights.
async function compactToolbar(ctx, { page, tab, kind }) {
  const evidence = { kind, viewport: [ctx.width, ctx.height], faceWidth: ctx.args['face-width'] || 'natural',
    referenceOnly: { desktop: [46, 28], wrap: [76, 48], fragment266: [82, 60] }, measurements: [] };
  try {
    await tab.emulateMedia({ reducedMotion: 'reduce' });
    await page.evaluate((width) => {
      const bar = document.querySelector('.board-toolbar');
      if (width) { bar.style.width = width + 'px'; bar.style.maxWidth = '100%'; }
      __bh.toolbarIdentity = { bar, face: bar.firstElementChild, row: bar.querySelector('.board-tools'), buttons: [...bar.querySelectorAll('.board-action')],
        frames: [...document.querySelectorAll('iframe.doc-frame')].map((n) => [n, n.contentWindow]) };
      __bh.toolbarMeasure = () => {
        const r = (n) => { const v = n.getBoundingClientRect(); return { x: v.x, y: v.y, width: v.width, height: v.height, bottom: v.bottom, right: v.right }; };
        const face = bar.firstElementChild, s = __bh.scroller(), row = bar.querySelector('.board-tools');
        return { floating: bar.hasAttribute('data-floating'), wrappedActions: bar.hasAttribute('data-wrapped-actions'),
          face: r(face), slot: r(bar), scrollTop: s.scrollTop, viewTop: __bh.viewTop(s), scroller: s === document.scrollingElement ? 'document' : s.id,
          stickyInset: parseFloat(getComputedStyle(bar).top) || 0,
          stickyTop: __bh.viewTop(s) + (parseFloat(getComputedStyle(bar).top) || 0) + (s === document.scrollingElement ? 0 : parseFloat(getComputedStyle(s).paddingTop) || 0),
          padding: parseFloat(getComputedStyle(s).scrollPaddingTop), railInset: parseFloat(getComputedStyle(document.querySelector('#bcol')).getPropertyValue('--wb-rail-inset')),
          counts: { text: bar.querySelector('.board-count').textContent, rect: r(bar.querySelector('.board-count')), font: getComputedStyle(bar.querySelector('.board-count')).fontSize },
          buttons: [...bar.querySelectorAll('.board-action')].map((n) => {
            const label = n.querySelector('.board-label'), icon = n.querySelector('.li'), kbd = n.querySelector('kbd');
            return { cls: n.className, ...r(n), label: getComputedStyle(label).display, font: getComputedStyle(label).fontSize,
              kbd: kbd && { display: getComputedStyle(kbd).display, font: getComputedStyle(kbd).fontSize },
              icon: r(icon), iconBox: [getComputedStyle(icon).width, getComputedStyle(icon).height],
              clipped: label.getClientRects().length > 0 && label.getBoundingClientRect().width > n.clientWidth };
          }), overflow: Math.max(document.documentElement.scrollWidth - innerWidth, s.scrollWidth - s.clientWidth),
          transition: [getComputedStyle(bar).transitionDuration, getComputedStyle(face).transitionDuration, getComputedStyle(row).transitionDuration] };
      };
      __bh.toolbarSame = () => {
        const i = __bh.toolbarIdentity;
        return i.bar === document.querySelector('.board-toolbar') && i.face === i.bar.firstElementChild && i.row === i.face.firstElementChild &&
          i.buttons.every((b, k) => b === i.bar.querySelectorAll('.board-action')[k]) && document.querySelectorAll('.board-toolbar').length === 1;
      };
    }, +(ctx.args['face-width'] || 0));
    await ovAt(page, 'top');
    const top = await page.evaluate(() => __bh.toolbarMeasure()); evidence.measurements.push({ phase: 'top', ...top });
    const actionCount = await page.evaluate(() => 2 + Number(document.documentElement.hasAttribute('data-term')) + Number(document.documentElement.hasAttribute('data-sysopen')) + Number(!!document.querySelector('.fpane')));
    qa(top.buttons.length === actionCount, 'source fixture offers every available original action', top);
    qa(top.buttons.every((b) => b.height === 30 && b.icon.height === 14 && b.icon.width === 14 && !b.clipped), 'nonfloating original 30px controls, uniform 14px glyphs, readable labels', top);
    await capture(ctx, tab, kind + '-compact-top', top);
    // Real keyboard traversal in both directions, including conditional slots.
    await page.focus('.board-overview');
    for (let k = 0; k < actionCount; k++) {
      qa(await page.evaluate((k) => document.activeElement === __bh.toolbarIdentity.buttons[k], k), 'forward Tab action ' + k);
      if (k < actionCount - 1) await tab.keyboard.press('Tab');
    }
    for (let k = actionCount - 1; k >= 0; k--) {
      qa(await page.evaluate((k) => document.activeElement === __bh.toolbarIdentity.buttons[k], k), 'reverse Tab action ' + k);
      if (k) await tab.keyboard.press('Shift+Tab');
    }
    await tab.keyboard.press('Tab');
    await ovAt(page, 'middle');
    const floating = await page.evaluate(() => __bh.toolbarMeasure()); evidence.measurements.push({ phase: 'floating', ...floating });
    qa(await page.evaluate(() => document.activeElement.matches('.tnew') && __bh.toolbarSame() && getComputedStyle(document.activeElement).outlineWidth === '2px'), 'floating transition retains mounted controls and visible keyboard focus');
    qa(floating.floating && Math.abs(floating.face.y - floating.stickyTop) <= 1 && Math.abs(floating.slot.height - top.slot.height) <= 1, 'sticky face reserves the original wrapped slot', { top, floating });
    qa(floating.face.height < top.face.height && floating.buttons.every((b, k) => b.height === 24 && Math.abs(b.icon.height - 12) < .01 && Math.abs(b.icon.width - 12) < .01 && b.iconBox.every((x) => x === '14px') && b.font === top.buttons[k].font && JSON.stringify(b.kbd) === JSON.stringify(top.buttons[k].kbd) && b.label === top.buttons[k].label && Math.abs(b.x - top.buttons[k].x) < .1 && Math.abs(b.width - top.buttons[k].width) < .1), 'compact vertical geometry preserves horizontal boxes, typography, labels and shortcuts', { top, floating });
    qa(floating.counts.text === top.counts.text && floating.counts.font === top.counts.font && Math.abs(floating.counts.rect.x - top.counts.rect.x) <= .1 && Math.abs(floating.counts.rect.width - top.counts.rect.width) <= .1 && floating.overflow <= 1 && floating.transition.every((x) => x === '0s'), 'counts/horizontal fit preserved without geometry animations', floating);
    qa(Math.abs(floating.padding - floating.stickyInset - floating.face.height - 10) < .1 && Math.abs(floating.railInset - floating.stickyInset - floating.face.height - 6) < .1, 'live face publishes navigation and rail clearances', floating);
    const hits = await page.evaluate(() => {
      const buttons = __bh.toolbarIdentity.buttons, errors = [], rects = [];
      buttons.forEach((b, i) => {
        const r = b.getBoundingClientRect(), hit = { left: r.left, right: r.right, top: r.top - 1, bottom: r.bottom + 5 }; rects.push(hit);
        if (hit.top < __bh.viewTop(__bh.scroller()) || hit.bottom > __bh.viewBottom(__bh.scroller())) errors.push({ i, outside: hit });
        // Match the approved rounded hit shape, not its excluded corner
        // triangles. Half-pixel edge probes also avoid subpixel rounding
        // outside Chromium's device-pixel hit grid.
        for (const x of [r.left + r.width * .25, (r.left + r.right) / 2, r.left + r.width * .75]) {
          for (const y of [hit.top + .5, r.top, (r.top + r.bottom) / 2, r.bottom + .5, hit.bottom - .5]) {
            const owner = document.elementFromPoint(x, y)?.closest('.board-action');
            if (owner !== b) errors.push({ i, x, y, owner: owner?.className || document.elementFromPoint(x, y)?.className });
          }
        }
        for (const x of [r.left + .5, r.right - .5]) {
          if (document.elementFromPoint(x, (r.top + r.bottom) / 2)?.closest('.board-action') !== b) errors.push({ i, horizontalEdge: x });
        }
      });
      rects.forEach((a, i) => rects.slice(i + 1).forEach((b, k) => {
        if (Math.min(a.right, b.right) > Math.max(a.left, b.left) && Math.min(a.bottom, b.bottom) >= Math.max(a.top, b.top)) errors.push({ overlap: [i, k + i + 1], a, b });
      }));
      const count = document.querySelector('.board-count'), counts = count.getBoundingClientRect().toJSON();
      for (const x of [.25, .5, .75]) {
        const owner = document.elementFromPoint(counts.left + counts.width * x, counts.top + counts.height / 2);
        if (owner !== count && !count.contains(owner)) errors.push({ countsOwner: owner?.className, x });
      }
      const fixed = [...document.querySelectorAll('.tk-handle,.gh,.dr-handle')].filter((n) => n.getClientRects().length).map((n) => ({ cls: n.className, ...n.getBoundingClientRect().toJSON() }));
      for (const handle of fixed) for (const r of rects.concat([counts])) {
        if (Math.min(r.right, handle.right) > Math.max(r.left, handle.left) && Math.min(r.bottom, handle.bottom) > Math.max(r.top, handle.top)) errors.push({ fixedCollision: handle, r });
      }
      return { errors, rects, counts, fixed, samples: buttons.length * 17 };
    }); evidence.hits = hits;
    qa(!hits.errors.length, `all ${hits.samples} edge/interior points belong to viewport-visible disjoint rounded 30px targets; counts clear fixed controls`, hits);
    await capture(ctx, tab, kind + '-compact-floating', floating);
    await page.focus('.board-overview');
    for (let k = 0; k < actionCount; k++) {
      qa(await page.evaluate((k) => document.activeElement === __bh.toolbarIdentity.buttons[k], k), 'floating forward Tab action ' + k);
      if (k < actionCount - 1) await tab.keyboard.press('Tab');
    }
    for (let k = actionCount - 1; k >= 0; k--) {
      qa(await page.evaluate((k) => document.activeElement === __bh.toolbarIdentity.buttons[k], k), 'floating reverse Tab action ' + k);
      if (k) await tab.keyboard.press('Shift+Tab');
    }
    await tab.keyboard.press('Tab'); await tab.keyboard.press('Enter');
    await page.waitForSelector('#board-term-slot .tpop:not([hidden])'); await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => document.activeElement.matches('.tnew')), 'terminal Enter/Escape preserves keyboard opener focus');
    const terminal = floating.buttons[1];
    await tab.mouse.click(terminal.x + terminal.width / 2, terminal.bottom + 4.5);
    await page.waitForSelector('#board-term-slot .tpop:not([hidden])');
    const menu = () => page.evaluate(() => {
      const p = document.querySelector('#board-term-slot .tpop'), r = p.getBoundingClientRect(), b = document.querySelector('.tnew').getBoundingClientRect();
      return { top: r.top, bottom: r.bottom, left: r.left, right: r.right, gap: r.top - b.bottom, height: b.height, viewport: innerHeight };
    });
    const smallMenu = await menu(); evidence.smallMenu = smallMenu;
    qa(smallMenu.gap >= 0 && smallMenu.left >= 0 && smallMenu.right <= ctx.width && smallMenu.bottom <= ctx.height, 'actual lower-hit extension opens contained terminal/profile menu', smallMenu);
    await ovAt(page, 'top');
    await page.waitForFunction(() => { const p = document.querySelector('#board-term-slot .tpop'), b = document.querySelector('.tnew'); return Math.abs(p.getBoundingClientRect().top - b.getBoundingClientRect().bottom - 8) < 1; });
    evidence.restoredMenu = await menu();
    qa(evidence.restoredMenu.height === 30, 'open terminal menu reanchors after its button resizes', evidence.restoredMenu);
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => document.activeElement.matches('.tnew') && document.querySelector('#board-term-slot .tpop').hidden), 'terminal Escape consumes dismissal and restores opener');
    // Actual Files explorer against the isolated project's real filesystem,
    // before the legacy toolbar scenario installs its deterministic fs routes.
    if (await page.locator('.board-files').count()) {
      await page.focus('.board-files'); await tab.keyboard.press('Enter');
      await page.waitForSelector('#files .fr');
      qa(await page.evaluate(() => document.querySelector('#files').hasAttribute('data-max') && document.activeElement.matches('#files .fq')), 'real Files explorer opens and focuses search');
      await page.click('#files .fr[data-path="ai-docs"]');
      await page.waitForSelector('#files .fr[data-path="ai-docs/_index.md"]');
      await page.click('#files .fr[data-path="ai-docs/_index.md"]', { button: 'right' });
      await page.waitForSelector('.fmenu'); await tab.keyboard.press('Escape');
      qa(await page.evaluate(() => document.querySelector('#files').hasAttribute('data-max') && !document.querySelector('.fmenu')), 'real Files row menu consumes Escape before the sheet');
      await page.focus('#files .fq'); await tab.keyboard.press('Escape');
      qa(await page.evaluate(() => !document.querySelector('#files').hasAttribute('data-max') && document.activeElement.matches('.board-files')), 'Files dismissal restores original opener');
    }
    // Full Enter activation of the remaining actions at their real controls.
    await page.focus('.board-search'); await tab.keyboard.press('Enter');
    await page.waitForSelector('#tkqpop[open]'); await tab.keyboard.press('Escape');
    await page.focus('.wnew'); await tab.keyboard.press('Enter');
    await page.waitForSelector('.wpop:not([hidden])'); await tab.keyboard.press('Escape');
    // Focus before the reading-position baseline: native focus() may scroll
    // an ancestor to reveal a previously clipped descendant of a sticky slot.
    // This probe measures Overview's restoration, not that browser focus step.
    await page.focus('.board-overview');
    await ovAt(page, 'middle');
    const beforeOverview = await page.evaluate(() => __bh.ovState());
    evidence.overviewBefore = beforeOverview;
    const anchorMetrics = () => page.evaluate((key) => {
      const n = __bh.ovFind(key), s = __bh.scroller(), top = __bh.viewTop(s);
      return { key, y: n?.getBoundingClientRect().top, offset: n && n.getBoundingClientRect().top - top, keeping: !!__bh.keeping(),
        framesAbove: [...document.querySelectorAll('#r-board .doc-frame')].filter((f) => f.getBoundingClientRect().bottom <= top).map((f) => ({ key: f.dataset.docKey, height: f.getBoundingClientRect().height })) };
    }, beforeOverview.key);
    evidence.overviewAnchorBefore = await anchorMetrics();
    await capture(ctx, tab, kind + '-compact-before-overview', beforeOverview);
    evidence.overviewAfterFocus = await page.evaluate(() => __bh.ovState());
    await tab.keyboard.press('Enter'); await ovDone(page, true);
    evidence.overviewEntered = await page.evaluate(() => ({ state: __bh.ovState(), stored: sessionStorage.getItem('ws.dashboard.overview.' + document.documentElement.dataset.rid) }));
    qa(await page.evaluate(() => !!document.querySelector('.overview-here')), 'floating Overview Enter retains an exposed reading anchor');
    await tab.keyboard.press('Escape'); await ovDone(page, false); await waitQuiet(page);
    evidence.overviewAfter = await page.evaluate(() => __bh.ovState());
    evidence.overviewAnchorAfter = await anchorMetrics();
    qa(evidence.overviewAnchorBefore.key === evidence.overviewAnchorAfter.key && Math.abs(evidence.overviewAnchorBefore.offset - evidence.overviewAnchorAfter.offset) <= 1 && beforeOverview.storage === evidence.overviewAfter.storage,
      'floating overview preserves original reading anchor/offset and card storage (scrollTop may track frame remeasurement)', { before: evidence.overviewAnchorBefore, after: evidence.overviewAnchorAfter, entry: evidence.overviewEntered, scroll: [beforeOverview.st, evidence.overviewAfter.st] });
    // Native browser target arrival honors the currently published padding.
    await page.evaluate(() => __bh.topics()[Math.floor(__bh.topics().length / 2)].scrollIntoView({ block: 'start' }));
    await waitQuiet(page);
    const arrival = await page.evaluate(() => {
      const target = __bh.topics()[Math.floor(__bh.topics().length / 2)], face = document.querySelector('.board-toolbar-face').getBoundingClientRect();
      return { target: target.getBoundingClientRect().top, faceBottom: face.bottom, padding: getComputedStyle(__bh.scroller()).scrollPaddingTop };
    }); evidence.nativeArrival = arrival;
    qa(arrival.target >= arrival.faceBottom, 'native scrollIntoView target remains reachable below the face', arrival);
    await ovAt(page, 'top');
    const restored = await page.evaluate(() => __bh.toolbarMeasure()); evidence.measurements.push({ phase: 'restored', ...restored });
    qa(!restored.floating && Math.abs(restored.face.height - top.face.height) <= 1 && Math.abs(restored.slot.height - top.slot.height) <= 1 && await page.evaluate(() => __bh.toolbarSame()), 'return-to-top restores full original geometry and identity', restored);
    // Unchanged body frames must survive a maximize cycle and board swap.
    const target = await page.evaluate(() => {
      const n = __bh.topics().find((n) => n.querySelector('iframe.doc-frame')); __bh.toolbarFrame = n.querySelector('iframe.doc-frame'); __bh.toolbarWindow = __bh.toolbarFrame.contentWindow; return n.dataset.topic;
    });
    await page.locator(`section.topic[data-topic="${target}"] .d-max`).click(); await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => __bh.toolbarFrame.isConnected && __bh.toolbarFrame.contentWindow === __bh.toolbarWindow), 'maximize/restore preserves iframe identity');
    await page.evaluate(async () => { const base = await __bh.baseBoard(); __bh.ovRegion(base.html); });
    await waitQuiet(page);
    qa(await page.evaluate(() => __bh.toolbarSame() && __bh.toolbarFrame.isConnected && __bh.toolbarFrame.contentWindow === __bh.toolbarWindow), 'live board event preserves face/buttons and unchanged iframe browsing context');
    await ovAt(page, 'top');
    const transitionAnchor = await page.evaluate(() => {
      const bh = __bh, s = bh.scroller(), n = bh.topics().find((n) => !n.closest('#bpins'));
      bh.toolbarTransitionAnchor = n;
      return { natural: n.getBoundingClientRect().top + s.scrollTop, threshold: Math.max(0, bh.board_().getBoundingClientRect().top - bh.viewTop(s) - (s === document.scrollingElement ? parseFloat(getComputedStyle(document.querySelector('.board-toolbar')).top) || 0 : 0)) };
    });
    evidence.transitions = [];
    for (const st of [0, Math.max(0, transitionAnchor.threshold - 1), transitionAnchor.threshold + 1, transitionAnchor.threshold + 20, Math.max(0, transitionAnchor.threshold - 1), 0]) {
      await page.evaluate((st) => { __bh.scroller().scrollTop = st; }, st); await waitQuiet(page);
      const sample = await page.evaluate(() => ({ geometry: __bh.toolbarMeasure(), natural: __bh.toolbarTransitionAnchor.getBoundingClientRect().top + __bh.scroller().scrollTop }));
      evidence.transitions.push(sample);
      qa(Math.abs(sample.natural - transitionAnchor.natural) <= 1 && Math.abs(sample.geometry.slot.height - top.slot.height) <= 1, 'transition keeps reading geometry and reserved wrapped slot', sample);
    }
    qa(evidence.transitions[0].geometry.floating === false && evidence.transitions.some((s) => s.geometry.floating) && evidence.transitions.at(-1).geometry.floating === false,
      'transition precondition actually traverses nonfloating/floating/nonfloating', evidence.transitions.map((s) => ({ st: s.geometry.scrollTop, floating: s.geometry.floating })));
    evidence.pageErrors = ctx.errors;
    log('production compact measurements (not reference parity): ' + JSON.stringify({ kind, viewport: evidence.viewport, faceWidth: evidence.faceWidth,
      measurements: evidence.measurements.map((m) => ({ phase: m.phase, face: m.face, slot: m.slot.height, padding: m.padding, overflow: m.overflow, wrappedActions: m.wrappedActions, counts: m.counts.text })),
      transitions: evidence.transitions.map((s) => ({ scrollTop: s.geometry.scrollTop, floating: s.geometry.floating, slot: s.geometry.slot.height, natural: s.natural })),
      hits: evidence.hits, overviewOffset: [evidence.overviewAnchorBefore.offset, evidence.overviewAnchorAfter.offset], nativeArrival: evidence.nativeArrival }));
  } finally {
    await capture(ctx, tab, kind + '-compact-result', evidence);
  }
}

async function scenarioToolbar(ctx) {
  const { page, tab, kind, browser } = await overviewReady(ctx);
  const first = await page.evaluate(() => {
    const b = __bh.board_(), t = b.querySelector('.board-toolbar');
    return { first: b.firstElementChild === t, next: t.nextElementSibling.id, rail: t.hasAttribute('data-rail'), card: t.hasAttribute('data-card'), pos: getComputedStyle(t).position,
      terminal: !!document.querySelector('.tnew'), capable: document.documentElement.hasAttribute('data-term'), files: !!document.querySelector('.board-files'), pane: !!document.querySelector('.fpane'),
      order: [...t.querySelectorAll('button.board-action')].map((n) => n.className), old: !!document.querySelector('#tslot,.tslot.tfix,.composer .tnew'),
      labels: [...t.querySelectorAll('.board-label')].map((n) => ({ overview: !!n.closest('.board-overview'), shown: getComputedStyle(n).display !== 'none' })),
      width: t.clientWidth, hint: getComputedStyle(t.querySelector('kbd')).display };
  });
  qa(first.first && (!first.capable || first.terminal) && first.terminal === first.capable && first.files === first.pane && !first.old && !first.rail && !first.card && first.pos === 'sticky', 'toolbar is first, non-card/non-rail/sticky, with capability-gated actions', first);
  if (ctx.width < 1600) qa(first.next === 'bpins', 'normal-width pin list follows toolbar');
  const expected = ['board-overview', ...(first.terminal ? ['tnew'] : []), ...(first.order.some((s) => /wnew/.test(s)) ? ['wnew'] : []), 'board-search', ...(first.files ? ['board-files'] : [])];
  qa(first.order.length === expected.length && expected.every((s, i) => first.order[i].includes(s)), 'toolbar action order', first.order);
  qa(first.labels.every((x) => x.shown === (x.overview || first.width > 540)) && (first.width > 540 || first.hint === 'none'), 'label collapse keys on toolbar width, preserving Overview', first);
  await compactToolbar(ctx, { page, tab, kind });
  if (first.files) {
    // File responses are browser-local fixtures; no host files or native
    // actions are opened. Exercise the mounted explorer, not a replacement.
    await tab.route('**/fs/**', async (route) => {
      const u = new URL(route.request().url()), op = u.pathname.split('/').pop();
      let body;
      if (op === 'list') {
        const dir = u.searchParams.get('dir') || '';
        body = { root: '/fixture', name: 'fixture', session: true, abs: '/fixture' + (dir ? '/' + dir : ''), sep: '/',
          entries: dir ? [{ name: 'main.go' }] : [{ name: 'src', dir: true }, ...Array.from({ length: 100 }, (_, i) => ({ name: `file-${i}.txt` }))] };
      } else if (op === 'find') body = { items: [{ path: 'src/main.go', name: 'main.go' }] };
      else if (op === 'preview') body = { kind: 'text', text: 'Files toolbar preview fixture' };
      else return route.continue();
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
    });
    await page.evaluate(() => {
      __bh.filesNode = document.querySelector('#files');
      __bh.filesParent = __bh.filesNode.parentNode;
      __bh.filesTabs = ['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k));
    });
    const opened = () => page.evaluate(() => document.querySelector('#files').hasAttribute('data-max'));
    const restored = () => page.evaluate(() => {
      const n = document.querySelector('#files');
      return n === __bh.filesNode && n.parentNode === __bh.filesParent && !n.hasAttribute('data-max') &&
        JSON.stringify(__bh.filesTabs) === JSON.stringify(['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k))) &&
        document.activeElement.matches('.board-files') && !!document.querySelector('#chead .fsum .froot');
    });
    await page.click('.board-files');
    await page.waitForSelector('#files .fr[data-path="src"]');
    qa(await page.evaluate(() => {
      const n = document.querySelector('#files'), r = n.getBoundingClientRect();
      return n === __bh.filesNode && n.parentNode === __bh.filesParent && getComputedStyle(n).position === 'fixed' &&
        document.activeElement.matches('#files .fq') && document.querySelectorAll('#files').length === 1 &&
        document.querySelector('#files .fhd .froot') && r.left >= 0 && r.right <= innerWidth && r.bottom <= innerHeight && r.height > 100;
    }), 'Files toolbar opens the same explorer as a viewport-fitting floating sheet and focuses search');
    await page.fill('#files .fq', 'main');
    await page.waitForSelector('#files .hit');
    await page.click('.board-files');
    qa(await restored() && await page.inputValue('#files .fq') === 'main', 'second Files click closes, preserves query/tab/placement and returns label/focus');
    await page.click('.board-files');
    await page.click('#files .fhd button:last-child');
    await tab.keyboard.press('Escape');
    qa(await opened() && await page.evaluate(() => !document.querySelector('#files .fpop:not([hidden])')), 'dropdown consumes Escape before Files sheet');
    await page.focus('#files .fq'); await tab.keyboard.press('Escape');
    qa(await opened() && await page.inputValue('#files .fq') === '', 'search Escape clears query before closing Files');
    await page.click('#files .fr[data-path="src"]');
    await page.waitForSelector('#files .fr[data-path="src/main.go"]');
    await page.click('#files .fr[data-path="src/main.go"]', { button: 'right' });
    qa(await page.evaluate(() => {
      const n = document.querySelector('.fmenu'), r = n.getBoundingClientRect();
      return n && n.contains(document.elementFromPoint(r.left + 10, r.top + 10));
    }), 'existing row menu remains above floating Files');
    await tab.keyboard.press('Escape');
    qa(await opened() && await page.evaluate(() => !document.querySelector('.fmenu') && document.activeElement.matches('#files .fr[data-path="src/main.go"]')), 'row-menu Escape consumes the event and restores its row');
    // Let the native scroll event reach the explorer's existing scroll
    // memory before hiding it (a synthetic assignment is not a user scroll).
    await page.evaluate(() => new Promise((resolve) => {
      document.querySelector('#files .ftree').scrollTop = 100;
      requestAnimationFrame(() => requestAnimationFrame(resolve));
    }));
    await tab.keyboard.press('Escape');
    qa(await restored(), 'unconsumed Escape dismisses Files and restores focus and original tabs');
    await page.click('.board-files');
    qa(await page.evaluate(() => document.querySelector('#files .fr[data-path="src"]').getAttribute('aria-expanded') === 'true' && document.querySelector('#files .ftree').scrollTop === 100), 'reopening preserves expanded folders and tree scroll');
    await page.evaluate(() => { document.querySelector('#files .ftree').scrollTop = 0; });
    await page.click('#files .fr[data-path="src/main.go"]');
    await page.waitForSelector('.pv-txt');
    qa(!await opened() && await page.textContent('.pv-txt') === 'Files toolbar preview fixture', 'file preview remains visible in the established board slot, dismissing Files');
    await ovAt(page, 'top');
    await page.evaluate(() => document.querySelector('#ctabs [data-ctab="files"]').click());
    await page.evaluate(() => { __bh.filesTabs = ['data-tab', 'data-ctab'].map((k) => document.querySelector('#grid').getAttribute(k)); });
    await page.click('.board-files'); await tab.keyboard.press('Escape');
    qa(await restored() && await page.evaluate(() => document.querySelector('#grid').getAttribute('data-ctab') === 'files'), 'original Files-tab access and selection survive floating dismissal');
    await page.evaluate(() => document.querySelector('#ctabs [data-ctab="convo"]').click());
    await page.evaluate(() => { document.querySelector('#r-board section.topic .d-max').click(); document.querySelector('.board-files').click(); });
    qa(await opened() && await page.evaluate(() => !document.querySelector('section.topic[data-max]')), 'Files uses shared maximize exclusivity');
    await page.evaluate(() => document.querySelector('#r-board section.topic .d-max').click());
    qa(!await opened() && await page.evaluate(() => !!document.querySelector('section.topic[data-max]')), 'another maximized sheet dismisses and restores Files');
    await tab.keyboard.press('Escape');
    // Board swaps run the same decorate/follow path as production.
    await page.click('.board-files');
    await page.evaluate(async () => { const b = await __bh.baseBoard(); __bh.ovRegion(b.html); });
    await waitQuiet(page);
    qa(await opened(), 'board updates do not dismiss the borrowed explorer as a missing topic');
    await tab.keyboard.press('Escape');
    await tab.unroute('**/fs/**');
    await ovAt(page, 'top');
  }
  await capture(ctx, tab, kind + '-normal');
  await ovAt(page, 'middle');
  qa(await page.evaluate(() => Math.abs(__bh.toolbarMeasure().face.y - __bh.toolbarMeasure().stickyTop) <= 1), 'floating face sticks to the active viewport');
  await ovAt(page, 'top');
  if (first.terminal) {
    await page.click('.tnew');
    await page.waitForFunction(() => document.querySelectorAll('.topts .topt').length >= 1);
    const folders = await page.evaluate(async () => (await (await fetch(location.pathname + '/terminals')).json()).folders);
    const pop = await page.evaluate(() => { const n = document.querySelector('.tpop'), b = document.querySelector('.tnew'), r = n.getBoundingClientRect(); return { open: !n.hidden, below: r.top >= b.getBoundingClientRect().bottom, left: r.left, right: r.right, bottom: r.bottom, options: n.querySelectorAll('.topt').length, path: !!n.querySelector('input') }; });
    qa(pop.open && pop.below && pop.left >= 0 && pop.right <= ctx.width && pop.bottom <= ctx.height && pop.path && pop.options === (folders.root ? 2 : 1), 'terminal popover below toolbar fits viewport with worktree/path and root when available', { pop, folders });
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => document.querySelector('.tpop').hidden && document.activeElement.matches('.tnew')), 'popover Escape restores trigger focus');
    // No PTY is started: test the real UI callback against a terminal-route
    // refusal first, then a valid exited TerminalInfo response. Backend PTY
    // lifecycle is outside this browser harness (and needs process cleanup).
    let created = false, request = null;
    await tab.route('**/terminals', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      request = route.request().postDataJSON();
      if (request.folder === 'path') return route.fulfill({ status: 400, body: 'not_absolute' });
      created = true;
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'bh-exited-terminal', session: new URL(ctx.args.url).pathname.split('/').pop(), cwd: '/tmp', state: 'exited', rows: 14, cols: 80, exit_code: 0 }) });
    });
    await page.click('.tnew'); await page.fill('.tpop input', 'relative'); await page.click('.tprow button');
    await page.waitForFunction(() => document.querySelector('.tpop .ferr').textContent.length > 0);
    qa(await page.evaluate(() => !document.querySelector('.tpop').hidden && document.activeElement.matches('.tpop input')), 'folder refusal stays inline and focuses typed path');
    await page.click('.topts .topt:first-child');
    await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.data.tid === 'bh-exited-terminal'));
    qa(created && request.folder === 'worktree' && await page.evaluate(() => __bh.board_().querySelector(':scope > .bcard, :scope > section.topic').dataset.kind === 'terminal'), 'worktree pick opens terminal at head of unpinned flow (browser-local POST fixture)', request);
  }
  if (first.order.some((x) => /wnew/.test(x))) {
    await page.focus('.wnew'); await tab.keyboard.press('Enter');
    await page.waitForSelector('.wpop:not([hidden])');
    const webPop = await readWebPopover(page);
    await assertWebContainment(ctx, webPop);
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => document.querySelector('.wpop').hidden && document.activeElement.matches('.wnew')), 'Web Escape restores opener focus');
  }
  // Land a board event with a closed section and localized remainder text.
  const base = await page.evaluate(() => __bh.baseBoard());
  await page.evaluate((html) => __bh.ovRegion(html + '<section class="topic closed" data-topic="BHCLOSED"><div class="tophead"><div class="tbar">Closed fixture</div></div></section><button class="closed-more" data-closed-next="1" data-closed-more="7">not a parseable count</button>'), base.html);
  const count = await page.evaluate(() => { const all = __bh.ovItems().filter((n) => n.matches('section.topic')); return { text: document.querySelector('.board-count').textContent, open: all.filter((n) => !n.classList.contains('closed')).length, closed: all.filter((n) => n.classList.contains('closed')).length + 7, first: __bh.board_().firstElementChild.matches('.board-toolbar') }; });
  qa(count.first && count.text === `open ${count.open} · closed ${count.closed}`, 'landed swap count includes pinned, shown closed and data-closed-more remainder (not localized text)', count);
  // Close only browser-local cards, never a real daemon terminal or process.
  await page.evaluate(() => {
    WsCards.list().forEach((c) => c.close());
    [...document.querySelector('#bpins').children].forEach((n) => WsCards.move(n, { pin: false }));
    __bh.ovRegion('<div class="empty">No board items</div>');
    __bh.scroller().scrollTop = 0;
  });
  await waitQuiet(page);
  const empty = await page.evaluate(() => { const e = document.querySelector('#r-board > .empty'), b = __bh.board_(), r = e.getBoundingClientRect(), s = document.querySelector('#tabbar') ? document.querySelector('#bcol') : document.scrollingElement; return { first: b.firstElementChild.matches('.board-toolbar'), selector: e.matches('.r-board:not(:has(> .topic, > .bcard)) > .empty'), display: getComputedStyle(e).display, height: r.height, bottom: r.bottom, max: __bh.viewBottom(s), overflow: s.scrollHeight - s.clientHeight }; });
  qa(empty.first && empty.selector && empty.display !== 'none' && empty.height >= 54 && (kind !== 'adapter' || (Math.abs(empty.bottom - empty.max) <= 16 && empty.overflow <= 1)), 'topicless placeholder fills remaining column without toolbar overflow', empty);
  await capture(ctx, tab, kind + '-empty');
  // Server-rendered no-data-term document, not deleting a mounted button.
  const noTerm = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
  await noTerm.route('**/*', async (route) => {
    if (!route.request().isNavigationRequest()) return route.continue();
    const response = await route.fetch();
    const body = (await response.text()).replace(/\sdata-term="[^"]*"/g, '');
    await route.fulfill({ response, body });
  });
  await noTerm.goto(ctx.args.url); await noTerm.waitForTimeout(500);
  const frame = noTerm.frames().find((f) => /[?&]frame=1/.test(f.url())) || noTerm;
  await frame.waitForSelector('.board-toolbar', { state: 'attached' });
  qa(await frame.evaluate(() => !document.documentElement.hasAttribute('data-term') && !document.querySelector('.tnew,#tslot,.tfix')), 'no data-term means no terminal entry (server-document fixture)');
  await noTerm.close();
  if (first.files) {
    for (const absent of ['pane', 'module']) {
      const p = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
      await p.route('**/*', async (route) => {
        if (!route.request().isNavigationRequest()) return route.continue();
        const response = await route.fetch();
        let body = await response.text();
        body = absent === 'pane' ? body.replace(/<section id="files"[^>]*><\/section>/, '') : body.replace('window.WsFiles = {', 'window.__bhDisabledFiles = {');
        await route.fulfill({ response, body });
      });
      await p.goto(ctx.args.url); await p.waitForTimeout(500);
      const f = p.frames().find((f) => /[?&]frame=1/.test(f.url())) || p;
      await f.waitForSelector('.board-toolbar', { state: 'attached' });
      qa(await f.evaluate(() => !document.querySelector('.board-files')), `Files toolbar absent without ${absent} (server-document fixture)`);
      await p.close();
    }
  }
}

async function scenarioRecapJitter(ctx) {
  const { page, tab, kind } = await overviewReady(ctx, { seed: false });
  if (ctx.args.theme) await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, ctx.args.theme);
  const key = 'LOGCHAIN';
  // Use renderer-produced agent recap and history markup. Only the text changes
  // in browser-local stream events; the server and disclosure logic stay intact.
  const base = await page.evaluate(() => __bh.baseBoard());
  await page.evaluate(({ html, key }) => {
    const t = document.createElement('template'); t.innerHTML = html;
    const n = t.content.querySelector(`section.topic[data-topic="${key}"]`);
    if (!n?.querySelector('.tsub > .trecap .rtext')) throw new Error('serve --history-entries 60 is required');
    __bh.recapLong = 'Long recap with enough wrapped text to exceed four lines, retained in the hover title. '.repeat(30);
    const text = n.querySelector('.rtext'); text.textContent = text.title = __bh.recapLong;
    n.dataset.th = 'recap-seed';
    __bh.ovRegion(t.innerHTML);
  }, { html: base.html, key });
  await waitQuiet(page);
  await page.evaluate((key) => {
    const n = document.querySelector(`section.topic[data-topic="${key}"]`);
    WsCards.move(n, { pin: true, index: 0 });
    // The target's folded log is short; real body topics below it make the
    // pinned column overflow so scroll/stick assertions cannot pass vacuously.
    for (const [index, topic] of ['T18', 'T17'].entries()) {
      const tail = document.querySelector(`section.topic[data-topic="${topic}"]`);
      if (!tail) throw new Error('serve --topics 18 is required');
      WsCards.move(tail, { pin: true, index: index + 1 });
    }
  }, key);
  await waitQuiet(page);
  await page.evaluate((key) => {
    const n = document.querySelector(`section.topic[data-topic="${key}"]`);
    __bh.focus = n;
    __bh.scroller().scrollTop = 0;
  }, key);
  await waitQuiet(page);
  await capture(ctx, tab, kind + '-recap-unstuck');
  const result = await page.evaluate(async (key) => {
    const bh = __bh, samples = [], updates = [], started = performance.now(), s = bh.scroller();
    const find = () => document.querySelector(`section.topic[data-topic="${key}"]`);
    const sample = (phase) => {
      const n = find(), head = n.querySelector('.tophead'), slot = n.querySelector('.tsub > .trecap');
      samples.push({ t: performance.now() - started, phase, scroll: s.scrollTop,
        // Wide pins have CSS-only sticky heads (no rail/wb-stuck class).
        stuck: head.getBoundingClientRect().top > n.getBoundingClientRect().top + 1,
        slot: slot.getBoundingClientRect().height,
        head: head.getBoundingClientRect().height, topic: n.getBoundingClientRect().height,
        line: parseFloat(getComputedStyle(slot).lineHeight), clamp: getComputedStyle(slot).webkitLineClamp,
        title: slot.querySelector('.rtext').title.length,
        fill: getComputedStyle(slot).backgroundColor });
    };
    const update = (index) => {
      const t = document.createElement('template'); t.innerHTML = bh.board.html;
      const n = t.content.querySelector(`section.topic[data-topic="${key}"]`), text = n.querySelector('.rtext');
      text.textContent = text.title = index % 2 ? bh.recapLong : 'Short recap.';
      n.dataset.th = 'recap-update-' + index;
      bh.ovRegion(t.innerHTML); updates.push(index);
    };
    sample('baseline');
    await new Promise((resolve) => {
      let next = 0;
      function tick() {
        const elapsed = performance.now() - started;
        // Exercise both stick transitions without reaching the topic's end.
        s.scrollTop = elapsed < 300 ? 0 : elapsed < 1500 ? 120 : elapsed < 1900 ? 0 : 120;
        if (next < 4 && elapsed >= 550 + next * 650) update(next++);
        sample('raf');
        if (elapsed >= 3300) resolve(); else requestAnimationFrame(tick);
      }
      requestAnimationFrame(tick);
    });
    // Content updates may change height; only equal-content sticky transitions
    // must remain stable. Compare short and long recaps independently.
    const groups = [...new Set(samples.map(r => r.title))].map(title => samples.filter(r => r.title === title));
    const delta = name => Math.max(...groups.map(rows => Math.max(...rows.map(r => r[name])) - Math.min(...rows.map(r => r[name]))));
    return { samples, updates, delta: { slot: delta('slot'), head: delta('head'), topic: delta('topic') },
      stuck: samples.filter((r) => r.stuck).length, unstuck: samples.filter((r) => !r.stuck).length,
      pincol: document.documentElement.hasAttribute('data-pincol'), scroller: s.id || s.tagName, viewHeight: s.clientHeight };
  }, key);
  await capture(ctx, tab, kind + '-recap-stuck', result);
  qa(ctx.width >= 1600 ? result.pincol && result.scroller === 'bpins' : !result.pincol && result.scroller === 'bcol', 'uses the responsive adapter pinned layout', result.scroller);
  qa(result.samples.every((r) => result.viewHeight >= r.head), 'pinned viewport can display the full recap/header', { viewHeight: result.viewHeight, head: result.samples[0].head });
  qa(result.stuck > 0 && result.unstuck > 0 && result.updates.length === 4 && new Set(result.samples.map((r) => r.title)).size === 2,
    'samples stuck/unstuck states and four live region updates with short/long recap text', { stuck: result.stuck, unstuck: result.unstuck, updates: result.updates });
  qa(result.samples.every((r) => r.clamp === '4' && Math.abs(r.slot - (r.title === 'Short recap.'.length ? 1 : 4) * r.line) <= 1), 'short recap shrinks to one line; long recap caps at four');
  qa(result.samples.every(r => r.fill === 'rgba(0, 0, 0, 0)'), 'header recap has no highlight background');
  qa(Object.values(result.delta).every((d) => d <= 1), 'same-content recap/header/topic has no sticky jitter above 1px', result.delta);
  log('recap jitter max deltas (px): ' + JSON.stringify(result.delta));
}

async function setOverviewFace(page, width) {
  if (!width) return;
  await page.evaluate((width) => {
    const bar = document.querySelector('.board-toolbar'); bar.style.width = width + 'px'; bar.style.maxWidth = '100%';
  }, +width);
  await page.waitForFunction(() => {
    const bar = document.querySelector('.board-toolbar');
    return !bar.hasAttribute('data-floating') && Math.abs(bar.getBoundingClientRect().height - bar.firstElementChild.getBoundingClientRect().height) < .1;
  }, null, { polling: 'raf' });
}

async function scenarioOverviewFocus(ctx) {
  const { page, tab, kind } = await overviewReady(ctx);
  await setOverviewFace(page, ctx.args['face-width']);
  const evidence = { kind, width: ctx.width, faceWidth: ctx.args['face-width'] || 'natural', modes: [] };
  try {
    for (const mode of ['reduce', 'no-preference']) {
      await tab.emulateMedia({ reducedMotion: mode });
      await page.focus('.board-overview');
      await page.evaluate(() => {
        const n = __bh.section('T18');
        if (!n) throw new Error('focus fixture requires real T18 topic');
        const s = __bh.ovScroller(n), v = __bh.ovViewport(s);
        s.scrollTop += n.getBoundingClientRect().top - v.top + 36;
      });
      await waitQuiet(page);
      const before = await page.evaluate(() => __bh.ovState());
      assert(before.key === 'topic:T18', 'focus fixture must begin at the genuine T18 reading anchor');
      const run = { mode, before, keys: [] }; evidence.modes.push(run);
      await tab.keyboard.press('Enter'); await ovDone(page, true);
      run.entry = await page.evaluate(() => __bh.ovHeadState());
      qa(run.entry.key === before.key && run.entry.focused && run.entry.exposed && run.entry.ownsCenter,
        mode + ' real Overview entry exposes the focused title head below face/host', run.entry);
      run.saved = await page.evaluate(() => JSON.parse(sessionStorage.getItem('ws.dashboard.overview.' + document.documentElement.dataset.rid)));
      qa(Math.abs(run.saved.offset - (before.y - before.top)) <= 1, mode + ' entry preserves the original return offset');
      for (const key of ['ArrowUp', 'ArrowDown', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowUp']) {
        const expected = await page.evaluate((key) => {
          const list = __bh.ovItems().filter((n) => __bh.board_().contains(n)), i = list.indexOf(document.activeElement);
          return __bh.ovId(list[Math.max(0, Math.min(list.length - 1, i + (key === 'ArrowDown' ? 1 : -1)))]);
        }, key);
        await tab.keyboard.press(key);
        await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => setTimeout(done, 0))));
        const state = await page.evaluate(() => ({ ...__bh.ovHeadState(), active: WsOverview.active(), floating: document.querySelector('.board-toolbar').hasAttribute('data-floating') }));
        run.keys.push({ key, expected, ...state });
        qa(state.key === expected && state.focused && state.active && state.floating && state.exposed && state.ownsCenter,
          mode + ' ' + key + ' exposes the exact focused title head, not the full-item center', state);
      }
      await tab.keyboard.press('Escape'); await ovDone(page, false); await waitQuiet(page);
      run.restored = await page.evaluate((key) => {
        const n = __bh.ovFind(key), s = __bh.ovScroller(n), v = __bh.ovViewport(s);
        return { key: __bh.ovId(n), offset: n.getBoundingClientRect().top - v.top, state: __bh.ovState() };
      }, before.key);
      qa(run.restored.key === before.key && Math.abs(run.restored.offset - (before.y - before.top)) <= 1 && run.restored.state.storage === before.storage,
        mode + ' Escape restores original reading identity/offset and card storage', run.restored);
      await capture(ctx, tab, kind + '-focus-' + mode, run);
    }
  } finally {
    await capture(ctx, tab, kind + '-focus-result', evidence);
    if (ctx.closeTab) await tab.close();
  }
}

// One explicitly allocated heavy-job slot: one Node/Chromium process, serial
// contexts only. No subprocess browser runner, build or broad suite is started.
async function scenarioOverviewFinal(ctx) {
  const dir = path.resolve(ctx.args['capture-dir']); fs.mkdirSync(dir, { recursive: true });
  const sources = ['wsdashboard/assets/cards.js', 'wsdashboard/assets/dashboard.css', 'wsdashboard/assets/overview.js', 'wsdashboard/assets/terminal.js', 'wswhiteboard/assets/rail.js'];
  const urls = { adapter: ctx.args.url, board: ctx.args['other-url'] };
  assert(urls.board, 'overview-final requires --other-url document-scroller fixture');
  const manifest = { candidate: '8a295b2b825cd65af372cfae004659e0aa454fbc', coverage: [], focus: [], owner: [], browserInstances: 1 };
  const save = () => fs.writeFileSync(path.join(dir, 'FINAL8a295-manifest.json'), JSON.stringify(manifest, null, 2));
  try {
    // Fetch/byte-match before Chromium launch. A stale daemon fails rather
    // than quietly rebuilding or switching any runtime under this slot.
    for (const [kind, url] of Object.entries(urls)) {
      const target = new URL(url); target.searchParams.set('frame', '1');
      const html = await (await fetch(target)).text(), hashes = {};
      for (const f of sources) {
        const relative = 'agents-plugin-tool/internal/' + f, ref = f.endsWith('/overview.js') ? manifest.candidate : '1975e3be465d3bde667529b5bc0af497e8fdf3bf';
        const expected = run('git', ['show', ref + ':' + relative], { cwd: REPO });
        assert(html.includes(expected), 'served source prerequisite mismatch (no build authorized): ' + kind + ' ' + relative);
        hashes[relative] = { ref, sha256: createHash('sha256').update(expected).digest('hex') };
      }
      manifest.coverage.push({ kind, url: target.href, hashes });
    }
    save(); log('SOURCE PREFLIGHT PASS: ' + JSON.stringify(manifest.coverage));
    const { chromium } = await loadPlaywright(), browser = await chromium.launch({ headless: true }); BROWSERS.push(browser);
    for (const [kind, url] of Object.entries(urls)) for (const face of [null, 330]) {
      const name = `FINAL8a295-focus-${kind}-${face ? 'wrap330' : 'normal'}`;
      const sub = { ...ctx, browser, closeTab: true, width: 800, errors: [], args: { ...ctx.args, _: ['FINAL8a295-focus'], url, 'face-width': face, 'capture-dir': path.join(dir, name) } };
      manifest.phase = name; save();
      await scenarioOverviewFocus(sub);
      assert(!sub.errors.length, name + ' page errors: ' + sub.errors.join(' | '));
      manifest.focus.push({ name, kind, width: 800, face, modes: ['reduce', 'no-preference'], verdict: 'pass' }); save();
    }
    for (const [kind, candidateURL] of Object.entries(urls)) for (const width of [1280, 800, 320]) for (const revision of ['baseline', 'candidate']) {
      const url = revision === 'baseline' ? ctx.args['baseline-base'] + new URL(candidateURL).pathname : candidateURL;
      const sub = { ...ctx, browser, width, language: 'ko', errors: [], args: { ...ctx.args, url } };
      manifest.phase = `FINAL8a295-owner-${kind}-${width}-${revision}`; save();
      const { page, tab } = await openBoard(sub);
      try {
        const checks = page.locator('#r-board .ckrow[data-id] label.ck > input');
        await page.waitForFunction(() => document.querySelectorAll('#r-board .ckrow[data-id] label.ck > input').length >= 3);
        for (let i = 0; i < 3; i++) await checks.nth(i).check();
        const controls = await checks.evaluateAll((ns) => ns.slice(0, 3).map((n) => ({ id: n.closest('[data-id]').dataset.id, checked: n.checked, topic: n.closest('[data-topic]').dataset.topic })));
        assert(controls.every((n) => n.checked), 'owner capture must use genuinely checked production inputs');
        for (const state of ['top', 'floating']) {
          await page.evaluate((state) => {
            const bh = __bh, s = bh.scroller();
            if (state === 'top') s.scrollTop = 0;
            else {
              const rows = [...document.querySelectorAll('#r-board .ckrow[data-id]')].filter((n) => n.querySelector('label.ck > input')?.checked);
              s.scrollTop += Math.max(...rows.map((n) => n.getBoundingClientRect().bottom)) - bh.viewTop(s) + 200;
            }
          }, state);
          await waitQuiet(page);
          await page.waitForFunction(() => [...document.querySelectorAll('.oq-seg.ans')].some((n) => n.getClientRects().length && /응답/.test(n.textContent) && n.querySelector('.n')?.textContent.trim() === '3'));
          const metrics = await page.evaluate(() => {
            const bar = document.querySelector('.board-toolbar'), face = bar.querySelector('.board-toolbar-face') || bar, f = face.getBoundingClientRect();
            const badges = [...document.querySelectorAll('.oq-seg.ans')].filter((n) => n.getClientRects().length && /응답/.test(n.textContent)).map((n) => {
              const r = n.getBoundingClientRect();
              return { text: n.textContent, edge: n.closest('.oq-wrap').className, rect: r.toJSON(), overlap: { width: Math.max(0, Math.min(f.right, r.right) - Math.max(f.left, r.left)), height: Math.max(0, Math.min(f.bottom, r.bottom) - Math.max(f.top, r.top)) },
                owned: [.25, .5, .75].map((x) => !!document.elementFromPoint(r.left + r.width * x, (r.top + r.bottom) / 2)?.closest('.oq-seg.ans')) };
            });
            return { face: f.toJSON(), slot: bar.getBoundingClientRect().toJSON(), floating: bar.hasAttribute('data-floating'), badges };
          });
          assert(metrics.badges.length && metrics.badges.every((b) => (!b.overlap.width || !b.overlap.height) && b.owned.every(Boolean)), 'owner badge collision/ownership: ' + JSON.stringify(metrics));
          assert(!sub.errors.length, 'owner page errors: ' + sub.errors.join(' | '));
          const stem = `FINAL8a295-owner-${kind}-${width}-${revision}-${state}`;
          const row = { stem, kind, width, revision, state, controls, metrics, errors: sub.errors, capture: path.join(dir, stem + '.png') };
          await tab.screenshot({ path: row.capture }); fs.writeFileSync(path.join(dir, stem + '.json'), JSON.stringify(row, null, 2));
          manifest.owner.push(row); save(); log('OWNER PASS: ' + JSON.stringify({ stem, face: metrics.face, badges: metrics.badges }));
        }
      } finally { await tab.close(); }
    }
    manifest.verdict = 'pass'; delete manifest.phase; save();
  } catch (e) { manifest.verdict = 'fail'; manifest.failure = e.message; save(); throw e; }
}

async function scenarioOverview(ctx) {
  const { page, tab, kind } = await overviewReady(ctx);
  await setOverviewFace(page, ctx.args['face-width']);
  // Capture shell bridge messages, including unexpected Esc forwarding.
  await tab.evaluate(() => { window.__ovMessages = []; addEventListener('message', (e) => { if (e.data && e.data.wsShell) __ovMessages.push(e.data); }); });
  await tab.emulateMedia({ reducedMotion: 'reduce' });
  const selected = ctx.args.case;
  assert(!selected || ['recap', 'anchors', 'rail', 'picker', 'actions', 'palette', 'controls', 'reload'].includes(selected), 'unknown overview --case');
  if (selected) log('focused overview case: ' + selected + ' (other checkpoints not run)');
  if (selected === 'recap') {
    await ovEnter(page);
    await checkFold(page);
    await capture(ctx, tab, kind + '-recap-overview', await page.evaluate(() => __bh.ovFold()));
    await ovExit(page);
    return;
  }
  if (!selected || selected === 'anchors' || selected === 'rail') {
  if (selected !== 'rail') {
  await page.evaluate(() => { __bh.hints = 0; const show = WsToast.show; WsToast.show = function (text, ...args) { if (text === wsT('js.board.overview.hint')) __bh.hints++; return show.call(this, text, ...args); }; });
  for (const where of ['middle', 'bottom']) {
    const before = await ovAt(page, where);
    qa(before.key && before.y < before.top, where + ' entry has a partly-visible negative-offset anchor', before);
    await ovEnter(page);
    const folded = await page.evaluate(() => __bh.ovState());
    const display = await page.evaluate((key) => {
      const n = __bh.ovFind(key), s = __bh.ovScroller(n), v = __bh.ovViewport(s);
      return { top: v.top, pad: parseFloat(getComputedStyle(s).scrollPaddingTop) || 0, head: __bh.ovHeadState(n),
        saved: JSON.parse(sessionStorage.getItem('ws.dashboard.overview.' + document.documentElement.dataset.rid)) };
    }, before.key);
    const shown = display.top + Math.max(before.y - before.top, display.pad);
    qa(folded.here === before.key && Math.abs(folded.hereY - shown) <= 1 && Math.abs(folded.width - before.width) <= 1 &&
      Math.abs(display.saved.offset - (before.y - before.top)) <= 1 && display.head.ownsCenter && display.head.exposed,
      where + ' folded anchor clears the face while retaining original return offset and board width', { before, folded, display, shown });
    // Test the sibling performance rule while folded, independently of
    // scroll restoration: do not change the unfolded board's intrinsic
    // sizes between recording an anchor and entering overview.
    await page.evaluate(() => { __bh.ovItems().filter((n) => n.matches('section.topic')).forEach((n) => { n.dataset.bhCv = n.style.contentVisibility; n.style.contentVisibility = 'auto'; }); });
    await checkFold(page);
    await page.evaluate(() => { __bh.ovItems().filter((n) => n.matches('section.topic')).forEach((n) => { n.style.contentVisibility = n.dataset.bhCv; delete n.dataset.bhCv; }); });
    await capture(ctx, tab, kind + '-' + where + '-overview');
    await tab.keyboard.press('Escape'); await checkRestore(page, before, where + ' Escape');
  }
  qa(await page.evaluate(() => __bh.hints === 1), 'entry hint appears once in this tab, not on second entry');
  const top = await ovAt(page, 'top');
  await ovEnter(page);
  qa(await page.evaluate(() => !document.querySelector('.overview-here')), 'toolbar-visible entry has no here anchor');
  // Actual toolbar action; unlike locator.click on an offscreen toolbar,
  // this does not silently scroll the board and invalidate an anchor test.
  await page.click('.board-overview'); await checkRestore(page, top, 'Overview button');
  qa(Math.abs((await page.evaluate(() => __bh.ovState())).toolbar.top - top.toolbar.top) <= 1, 'no-anchor exit preserves toolbar screen y');
  }
  const outsides = selected === 'rail' ? ['shell-rail'] : kind === 'adapter' ? ['composer', 'shell-header'] : ['shell-header'];
  if (!selected) outsides.push('shell-rail');
  for (const outside of outsides) {
    const before = await ovAt(page, 'middle');
    if (outside === 'shell-rail') { await tab.evaluate(() => document.querySelector('#railbtn').click()); await sleep(300); }
    await ovEnter(page); await sleep(300);
    await page.evaluate(() => { __bh.outsideFired = 0; document.querySelector('#ctext')?.addEventListener('click', () => __bh.outsideFired++); });
    const urls = tab.frames().map((f) => f.url()), requests = [];
    const onRequest = (r) => { if (r.method() !== 'GET') requests.push(r.url()); };
    tab.on('request', onRequest);
    const focus = await page.evaluate(() => __bh.ovId(document.activeElement));
    if (outside === 'composer') {
      const r = await page.locator('#ctext').boundingBox(); assert(r, 'composer must be visible for physical outside click');
      await tab.mouse.click(r.x + 5, r.y + r.height / 2);
    } else if (outside === 'shell-rail') {
      const rail = tab.locator('#rail a[href]').first(), r = await rail.boundingBox();
      assert(r && r.x >= 0, 'scenario needs an open shell rail row');
      qa(await tab.evaluate(() => getComputedStyle(document.querySelector('#rail')).pointerEvents === 'none'), 'dim shell rail passes pointer events through to frame');
      await tab.mouse.click(r.x + r.width / 2, r.y + r.height / 2);
    } else {
      const r = await tab.locator('.dwrap > .top').boundingBox(); assert(r, 'scenario needs the iframe shell header');
      await tab.mouse.click(r.x + r.width - 5, r.y + r.height / 2);
    }
    await checkRestore(page, before, outside + ' outside click');
    tab.off('request', onRequest);
    qa(!requests.length && JSON.stringify(urls) === JSON.stringify(tab.frames().map((f) => f.url())) && await page.evaluate(() => !__bh.outsideFired && document.activeElement.id !== 'ctext'), outside + ' click is swallowed (no activation, navigation, POST or composer focus)', { requests, focus });
    if (outside === 'shell-rail') await tab.evaluate(() => { if (document.documentElement.classList.contains('rail-open')) document.querySelector('#railbtn').click(); });
  }
  }
  if (!selected || selected === 'picker') {
  // Roving focus and Enter use the stack's own DOM order (toolbar excluded).
  await ovAt(page, 'middle'); await ovEnter(page);
  qa(await page.evaluate(() => {
    const n = document.querySelector('.overview-here .tbar'), r = n.getBoundingClientRect(), before = __bh.ovState().order;
    n.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, isPrimary: true, button: 0, pointerId: 42, pointerType: 'mouse', clientX: r.left + 50, clientY: r.top + 10 }));
    document.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, isPrimary: true, pointerId: 42, clientX: r.left + 100, clientY: r.top + 80 }));
    const dragging = WsCards.dragging();
    document.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, isPrimary: true, pointerId: 42 }));
    return !dragging && JSON.stringify(before) === JSON.stringify(__bh.ovState().order);
  }), 'overview prevents drag start and preserves stack order');
  const roving = await page.evaluate(() => { const a = __bh.ovItems().filter((n) => __bh.board_().contains(n)), i = a.indexOf(document.activeElement); return { first: __bh.ovId(a[i]), next: __bh.ovId(a[i + 1]), third: __bh.ovId(a[i + 2]) }; });
  assert(roving.third, 'fixture needs two stack items after anchor');
  await tab.keyboard.press('ArrowDown'); qa(await page.evaluate((k) => __bh.ovId(document.activeElement) === k, roving.next), 'ArrowDown roves to next stack item');
  await tab.keyboard.press('ArrowUp'); qa(await page.evaluate((k) => __bh.ovId(document.activeElement) === k, roving.first), 'ArrowUp roves back to anchor');
  await tab.keyboard.press('ArrowDown'); await tab.keyboard.press('ArrowDown'); await tab.keyboard.press('Enter'); await ovDone(page, false);
  qa(await page.evaluate((k) => { const n = __bh.ovFind(k); return n.classList.contains('overview-flash') && Math.abs(__bh.ovLandingError(n)) <= 1 && getComputedStyle(n).animationName === 'none' && parseFloat(getComputedStyle(n).outlineWidth) >= 2; }, roving.third), 'Enter on third roving item lands top with static reduced-motion flash');
  await sleep(1250); qa(await page.evaluate((k) => !__bh.ovFind(k).classList.contains('overview-flash'), roving.third), 'static landing cue removed after 1.2s');
  // Clicking a faded head control must pick, not invoke its own action.
  await ovAt(page, 'middle'); await ovEnter(page);
  const pick = await page.evaluate(() => { const n = document.querySelector('.overview-here'); const key = __bh.ovId(n); n.querySelector('.tpin').click(); return key; });
  await ovDone(page, false);
  qa(await page.evaluate((k) => { const n = __bh.ovFind(k); return !n.hasAttribute('data-pinned') && n.classList.contains('overview-flash') && Math.abs(__bh.ovLandingError(n)) <= 1; }, pick), 'head-control click picks/lands/flashes topic rather than pinning it');
  // Card header-only folding and landing use a real Card with inert body.
  await ovAt(page, 'top'); await ovEnter(page);
  await page.evaluate(() => __bh.board_().querySelector('.bcard').click()); await ovDone(page, false);
  qa(await page.evaluate(() => { const n = document.querySelector('.bcard.overview-flash'); return n && Math.abs(__bh.ovLandingError(n)) <= 1; }), 'card click lands top and flashes like a topic');
  // Inject through socket onmessage, not a DOM append or direct morph call.
  await ovAt(page, 'middle'); await ovEnter(page);
  const heldBefore = await page.evaluate(() => ({ order: __bh.ovState().order, hash: __bh.board_().dataset.h, count: document.querySelector('.board-count').textContent }));
  const held = await page.evaluate(() => __bh.update([], [{ from: __bh.topics()[0].dataset.topic, name: 'BHHELD' }], { held: true }));
  qa(await page.evaluate((b) => __bh.board_().dataset.h === b.hash && JSON.stringify(__bh.ovState().order) === JSON.stringify(b.order) && !__bh.section('BHHELD'), heldBefore), 'injected board update changes nothing during overview');
  await ovExit(page);
  qa(await page.evaluate((h) => __bh.board_().dataset.h === h && !!__bh.section('BHHELD'), held.hash), 'held update lands after exit');
  await waitQuiet(page);
  qa(await page.evaluate((s) => document.querySelector('.board-count').textContent !== s, heldBefore.count), 'toolbar count updates on released swap');
  }
  if (!selected || selected === 'actions' || selected === 'palette') {
  // Palette stays over overview until a board pick. Both key variants must
  // route through WsOverview, not a raw scroll into folded content.
  for (const keep of [false, true]) {
    await ovAt(page, 'top'); await ovEnter(page);
    if (selected === 'palette') await tab.keyboard.press('Control+k'); else await page.click('.board-search');
    await page.waitForFunction(() => document.querySelector('#tkqpop').open, null, { timeout: 3000 });
    qa(await page.evaluate(() => WsOverview.active()), 'Search keeps overview open');
    // This palette clears a nonempty query on the first Escape; test the
    // top-layer close with an empty query, not against that separate rule.
    await page.fill('#tkqin', '');
    await tab.keyboard.press('Escape');
    qa(await page.evaluate(() => !document.querySelector('#tkqpop').open && WsOverview.active()), 'Escape closes only palette');
    // Let the native dialog close event finish before another user opens it.
    await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));
    if (selected === 'palette') await tab.keyboard.press('Control+k'); else await page.click('.board-search');
    await page.waitForFunction(() => document.querySelector('#tkqpop').open, null, { timeout: 3000 });
    await page.fill('#tkqin', 'T09');
    await page.waitForFunction(() => document.querySelector('#tkqpop [data-qs="board"][role="option"]'));
    await tab.keyboard.press('Alt+Digit1');
    await tab.keyboard.press(keep ? 'Control+Enter' : 'Enter'); await ovDone(page, false);
    // The frame reporter can deliver a restored height after exit returns.
    // Give the shared keep writer its next frame, within the flash window.
    await page.waitForFunction(() => Math.abs(__bh.ovLandingError(__bh.section('T09'))) <= 1, null, { polling: 'raf', timeout: 1000 }).catch(() => {});
    const r = await page.evaluate(() => { const n = __bh.section('T09'); return { palette: document.querySelector('#tkqpop').open, flash: n.classList.contains('overview-flash'), dy: __bh.ovLandingError(n) }; });
    qa(r.palette === keep && r.flash && Math.abs(r.dy) <= 1, (keep ? 'Ctrl+Enter' : 'Enter') + ' palette board pick exits/lands/flashes with correct palette lifetime', r);
    if (keep) { await page.fill('#tkqin', ''); await tab.keyboard.press('Escape'); }
  }
  }
  if (!selected || selected === 'actions' || selected === 'controls' || selected === 'palette') {
  await ovAt(page, 'top'); await ovEnter(page);
  await page.click('.tnew'); await tab.keyboard.press('Escape');
  qa(await page.evaluate(() => document.querySelector('.tpop').hidden && WsOverview.active()), 'Escape closes only terminal popover');
  await ovExit(page);
  await sleep(50);
  const messages = await tab.evaluate(() => __ovMessages);
  qa(messages.some((m) => m.wsShell === 'layout' && Math.abs(m.dim - .72) < .001) && messages.some((m) => m.wsShell === 'layout' && m.dim === 0) && !messages.some((m) => m.wsShell === 'key' && m.key === 'esc'), 'shell receives dim .72 / 0 and no forwarded Escape', messages);
  if (selected !== 'palette') {
    await ovAt(page, 'top');
    await page.evaluate(() => WsCards.list()[0].maxBtn.click());
    qa(await page.evaluate(() => { WsOverview.toggle(); return !WsOverview.holding() && !!document.querySelector('.bcard[data-max]'); }), 'overview entry is blocked while a card is maximized');
    await page.evaluate(() => WsCards.list()[0].minBtn.click());
    let postState = null;
    await tab.route('**/terminals', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      postState = await page.evaluate(() => ({ active: WsOverview.active(), held: WsOverview.holding() }));
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'bh-overview-terminal', cwd: '/tmp', state: 'exited' }) });
    });
    await ovAt(page, 'top'); await ovEnter(page); await page.click('.tnew'); await page.click('.topts .topt:first-child');
    await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.data.tid === 'bh-overview-terminal'));
    qa(postState && !postState.active && !postState.held && await page.evaluate(() => __bh.board_().querySelector(':scope > .bcard, :scope > section.topic').dataset.kind === 'terminal'), 'terminal folder pick exits fully before POST/card creation (browser-local fixture)', postState);
    await ovAt(page, 'top'); await ovEnter(page);
    await tab.keyboard.press('Control+Backquote'); await ovDone(page, false); await sleep(50);
    qa(await tab.evaluate(() => __ovMessages.some((m) => m.wsShell === 'key' && m.key === 'rail')), 'rail hotkey exits overview before forwarding rail action');
    await tab.evaluate(() => { if (document.documentElement.classList.contains('rail-open')) document.querySelector('#railbtn').click(); });
  }
  if (kind === 'adapter') {
    await ovAt(page, 'top'); await ovEnter(page); await page.click('.board-files'); await ovDone(page, false);
    qa(await page.evaluate(() => document.querySelector('.fpane').getClientRects().length && document.activeElement.closest('.fpane') && document.activeElement.matches('input')), 'Files exits overview then reveals Files and focuses filter');
    if (await page.locator('#tabbar').isVisible()) await page.click('#tabbar [data-tab="board"]');
  }
  }
  if (!selected || selected === 'reload') {
  // Restore a genuine server anchor, not the injected BHHELD topic that a
  // reload intentionally discards. Storage is per rid and session-only.
  await page.evaluate(() => { const s = __bh.scroller(), n = __bh.section('T10'); s.scrollTop += n.getBoundingClientRect().top - __bh.viewTop(s) + 36; });
  await waitQuiet(page);
  const beforeReload = await page.evaluate(() => __bh.ovState()); await ovEnter(page);
  const saved = await page.evaluate(() => Object.entries(sessionStorage).filter(([k]) => k.startsWith('ws.dashboard.overview.')));
  qa(saved.some(([k, v]) => k.endsWith(locationRid(ctx.args.url)) && JSON.parse(v).on), 'overview persists in per-session sessionStorage', saved);
  await tab.reload({ waitUntil: 'load' });
  const reloadPage = tab.frames().find((f) => /[?&]frame=1/.test(f.url())) || tab;
  await reloadPage.evaluate(IN_PAGE); await reloadPage.evaluate(OVERVIEW_PAGE);
  await reloadPage.waitForFunction(() => WsOverview.active());
  qa(await reloadPage.evaluate(() => !document.querySelector('[data-overview-motion],.overview-ghost')), 'reload opens folded without motion');
  await reloadPage.evaluate(() => WsOverview.exit()); await ovDone(reloadPage, false);
  await waitQuiet(reloadPage);
  // The injected extra topic changes absolute scrollTop across reload; the
  // actual contract is the stored anchor offset, not stale pixel scroll.
  const reloaded = await reloadPage.evaluate((k) => { const n = __bh.ovFind(k); return n && n.getBoundingClientRect().top - __bh.viewTop(__bh.scroller()); }, beforeReload.key);
  qa(Math.abs(reloaded - (beforeReload.y - beforeReload.top)) <= 1, 'reload exit restores stored anchor offset within 1px', { reloaded, before: beforeReload.y - beforeReload.top });
  await capture(ctx, tab, kind + '-reload-exit');
  if (ctx.args['other-url']) {
    await ovAt(reloadPage, 'top'); await ovEnter(reloadPage);
    const other = new URL(ctx.args['other-url']); other.searchParams.set('frame', '1');
    assert(other.origin === new URL(ctx.args.url).origin && locationRid(other.href) !== locationRid(ctx.args.url), '--other-url must be another session on the same isolated daemon');
    await tab.evaluate((src) => { const f = document.createElement('iframe'); f.id = 'bh-other'; f.src = src; f.style.cssText = 'position:fixed;inset:0;width:100%;height:100%;z-index:-1'; document.body.appendChild(f); }, other.href);
    const second = await waitFor(() => tab.frames().find((f) => f.url() === other.href), 5000, 'second session document');
    await second.waitForLoadState('load');
    qa(await second.evaluate(() => !WsOverview.active()), 'second session does not inherit first session overview state');
    await second.evaluate(() => {
      document.querySelector('#tabbar [data-tab="board"]')?.click();
      window.__otherHints = 0; const show = WsToast.show;
      WsToast.show = function (text, ...args) { if (text === wsT('js.board.overview.hint')) __otherHints++; return show.call(this, text, ...args); };
      WsOverview.toggle();
    });
    await second.waitForFunction(() => WsOverview.active());
    qa(await second.evaluate(() => !window.__otherHints), 'entry hint does not repeat in second session frame in same tab');
    await ovExit(reloadPage);
    qa(await second.evaluate(() => WsOverview.active()), 'exiting first session leaves second session overview open');
    await second.evaluate(() => WsOverview.exit());
    await tab.evaluate(() => document.querySelector('#bh-other').remove());
  } else log('SKIP second session frame: use --other-url for same-tab isolation and shared hint checks');
  log('SKIP deferred reorder-hold release: only Phase 1 is present; no release scheduler exists');
  log('SKIP real terminal PTY lifecycle and private cards.js maximize() call during overview (not exported); UI max entry guard is in controls case');
  await ovAt(reloadPage, 'top'); await ovEnter(reloadPage); await sleep(300);
  const darkroom = await reloadPage.evaluate(() => ({ drawer: document.querySelector('#drawer') && getComputedStyle(document.querySelector('#drawer')).filter, composer: document.querySelector('.composer') && getComputedStyle(document.querySelector('.composer')).filter }));
  log('LOOK diagnostic (owner owns known #drawer omission): ' + JSON.stringify(darkroom));
  await ovExit(reloadPage);
  }
}
function locationRid(url) { return new URL(url).pathname.split('/').filter(Boolean).pop(); }

function overviewEase(p) {
  let lo = 0, hi = 1, t = p;
  for (let i = 0; i < 24; i++) { t = (lo + hi) / 2; const x = .6 * t * (1 - t) + t * t * t; if (x < p) lo = t; else hi = t; }
  return p <= 0 ? 0 : p >= 1 ? 1 : 2.4 * t * (1 - t) ** 2 + 3 * t * t * (1 - t) + t ** 3;
}
function validateMotion(run, entering, anchorY) {
  const samples = run.samples, initial = samples[0], last = samples.at(-1);
  assert(!run.timeout, 'overview motion never completed');
  qa(run.clockObserved, 'sampler observes the real motion clock without changing timestamps');
  const animated = initial.rows.filter((r) => r.motion);
  qa(animated.length > 0, 'motion starts near-viewport rows synchronously');
  qa(run.clicks === 0, 'clicks during motion are swallowed');
  const end = samples.find((s) => !s.rows.some((r) => r.motion));
  const gaps = samples.slice(1).map((s, i) => s.t - samples[i].t).sort((a, b) => a - b), frame = gaps[Math.floor(gaps.length / 2)];
  qa(end.t >= 240 - frame - 2 && end.t <= 240 + frame + 2, 'one 240ms clock completes within one animation frame', { end: end.t, frame, setup: run.setupMs });
  const initialIds = animated.map((r) => r.id);
  qa(samples.every((s) => s.rows.filter((r) => r.motion).every((r) => initialIds.includes(r.id))), 'no stagger / no row starts after initial frame');
  const reference = animated.find((r) => Math.abs(last.rows.find((x) => x.id === r.id).r.height - r.r.height) > 20);
  assert(reference, 'fixture needs a row that changes height');
  const refEnd = last.rows.find((r) => r.id === reference.id).r.height;
  let worstSync = 0, worstCurve = 0, worstAnchor = 0, worstRow = null;
  for (const s of samples) {
    const ref = s.rows.find((r) => r.id === reference.id), e = (ref.r.height - reference.r.height) / (refEnd - reference.r.height);
    const clock = entering ? overviewEase(Math.min(1, s.t / 240)) : 1 - overviewEase(1 - Math.min(1, s.t / 240));
    worstCurve = Math.max(worstCurve, Math.abs(e - clock) * Math.abs(refEnd - reference.r.height));
    for (const a of animated) {
      const b = last.rows.find((r) => r.id === a.id), r = s.rows.find((r) => r.id === a.id);
      const expected = a.r.height + (b.r.height - a.r.height) * e, delta = Math.abs(r.r.height - expected);
      if (delta > worstSync) { worstSync = delta; worstRow = { id: r.id, t: s.t, expected, actual: r.r.height, inline: r.height }; }
      assert(r.transition.split(',').every((x) => parseFloat(x) === 0), 'row height must be driven by rAF, not CSS transitions');
    }
    if (anchorY !== null && s.here !== null) {
      const expectedAnchor = typeof anchorY === 'number' ? anchorY : anchorY.from + (anchorY.to - anchorY.from) * e;
      worstAnchor = Math.max(worstAnchor, Math.abs(s.here - expectedAnchor));
    }
    for (const [i, g] of s.ghosts.entries()) {
      assert(!g.unsafe && g.children.length === 2 && /tophead|bc-hd/.test(g.children[0]) && g.children[1] === 'overview-ghost-body', 'ghost must be header-only plus a plain collapsing body box');
      const from = initial.ghosts[i].r;
      assert(['top', 'left', 'width', 'height'].every((k) => Math.abs(g.r[k] - (from[k] + (g.target[k] - from[k]) * e)) <= 1), 'every ghost tracks its live target on the same curve within 1px');
    }
    if (initial.ghosts.length) assert(Math.abs(s.slot.height - (initial.slot.height + (last.slot.height - initial.slot.height) * e)) <= 1, 'pin slot and folds share the same clock within 1px');
  }
  qa(worstSync <= 1, 'all animated heights share one progress curve within 1px', { entering, worstSync, worstRow });
  qa(worstCurve <= 1, 'clock follows cubic-bezier(.2,.8,.2,1), reversed on exit, within 1px', { worstCurve });
  if (entering && anchorY !== null) qa(worstAnchor <= 1, 'anchor follows its exposed-offset curve within 1px on every animation frame', { worstAnchor, anchorY });
  const outside = initial.rows.filter((r) => !r.motion);
  if (entering) assert(outside.length > 0, 'entry fixture must include far-offscreen rows');
  qa(outside.every((r) => samples.every((s) => Math.abs(s.rows.find((x) => x.id === r.id).r.height - last.rows.find((x) => x.id === r.id).r.height) <= 1)), 'offscreen rows switch instantly, with no animation');
  for (const land of run.lands) {
    const diff = (a, b) => Math.max(...['top', 'left', 'width', 'height'].map((k) => Math.abs(a[k] - b[k])));
    qa(land.connected && diff(land.from, land.to) <= 1 && land.next && diff(land.to, land.next) <= 1, 'pin clone lands exactly on live target and does not jump next frame', land);
  }
  return { animated: initialIds, final: last.rows.map((r) => ({ id: r.id, h: r.r.height })) };
}
async function scenarioOverviewMotion(ctx) {
  const { page, tab, kind } = await overviewReady(ctx, { pins: ctx.width >= 1600 ? 5 : 2, cards: false });
  await tab.emulateMedia({ reducedMotion: 'no-preference' });
  const selected = ctx.args.case;
  assert(!selected || ['enter-top', 'enter-middle', 'enter-bottom', 'pins'].includes(selected), 'unknown overview-motion --case');
  if (selected) log('focused motion case: ' + selected + ' (other checkpoints not run)');
  const positions = selected === 'pins' ? [] : selected ? [selected.slice(6)] : ['middle', 'bottom'];
  for (const where of positions) {
    const before = await ovAt(page, where);
    const shown = before.key && await page.evaluate(([key, offset]) => {
      const n = __bh.ovFind(key), s = __bh.ovScroller(n), v = __bh.ovViewport(s);
      return v.top + Math.max(offset, parseFloat(getComputedStyle(s).scrollPaddingTop) || 0);
    }, [before.key, before.y - before.top]);
    const enter = await page.evaluate(() => __bh.ovMotion(true));
    await capture(ctx, tab, kind + '-' + where + '-enter', enter);
    let result;
    try { result = validateMotion(enter, true, before.key ? { from: before.y, to: shown } : null); }
    catch (e) { console.error('per-frame enter trace:\n' + JSON.stringify(enter, null, 2)); throw e; }
    qa(await page.evaluate((w) => Math.abs(__bh.ovState().width - w) <= 1, before.width), 'board width unchanged while pin list is in overview');
    if (selected && selected.startsWith('enter-')) { log('SKIP exit/end-state equivalence in focused enter-only case'); return; }
    const exit = await page.evaluate(() => __bh.ovMotion(false));
    await capture(ctx, tab, kind + '-' + where + '-exit', exit);
    try { validateMotion(exit, false, null); }
    catch (e) { console.error('per-frame exit trace:\n' + JSON.stringify(exit, null, 2)); throw e; }
    await checkRestore(page, before, where + ' animated exit');
    // At common eased progress, a forward and reversed curve must traverse
    // the same heights within 1px. Endpoints must be exact too.
    const enterStart = enter.samples[0], exitEnd = exit.samples.at(-1);
    qa(result.animated.every((id) => Math.abs(enterStart.rows.find((r) => r.id === id).r.height - exitEnd.rows.find((r) => r.id === id).r.height) <= 1), 'reverse motion returns exact unfolded heights');
    await tab.emulateMedia({ reducedMotion: 'reduce' });
    const instant = await page.evaluate(() => { WsOverview.toggle(); return { active: WsOverview.active(), motion: !!document.querySelector('[data-overview-motion],.overview-ghost'), heights: __bh.ovItems().map((n) => ({ id: __bh.ovId(n), h: n.getBoundingClientRect().height })) }; });
    qa(instant.active && !instant.motion && instant.heights.every((r) => Math.abs(r.h - result.final.find((x) => x.id === r.id).h) <= 1), 'reduced motion switches instantly to identical folded end state');
    await page.evaluate(() => WsOverview.exit());
    qa(await page.evaluate(() => !WsOverview.holding() && !document.querySelector('[data-overview-motion],.overview-ghost')), 'reduced-motion exit completes synchronously without transition');
    await tab.emulateMedia({ reducedMotion: 'no-preference' });
  }
  if (ctx.width >= 1600) {
    if (selected === 'pins') await tab.emulateMedia({ reducedMotion: 'reduce' });
    const wide = await ovAt(page, 'top');
    qa(wide.pincol, '1900px fixture enables actual pinned column');
    await page.evaluate(() => { const p = document.querySelector('#bpins'); p.scrollTop = 160; });
    const scroll = await page.evaluate(() => document.querySelector('#bpins').scrollTop);
    qa(scroll > 0, 'wide pinned list overflows and is independently scrolled', scroll);
    await ovEnter(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'r-board' && document.querySelector('.board-toolbar').nextElementSibling.id === 'bpins' && !document.querySelector('#bpcol').children.length && getComputedStyle(document.querySelector('#bpins'), '::before').display === 'none'), 'wide overview empties column and places pins directly under toolbar without label');
    await ovExit(page); await waitQuiet(page);
    qa(await page.evaluate((s) => document.querySelector('#bpins').parentElement.id === 'bpcol' && Math.abs(document.querySelector('#bpins').scrollTop - s) <= 1, scroll), 'pinned-column scroll restores through overview');
    await ovEnter(page);
    await tab.setViewportSize({ width: 1400, height: ctx.height }); await sleep(150);
    qa(await page.evaluate(() => WsOverview.active() && document.querySelector('#bpins').parentElement.id === 'r-board' && !document.documentElement.hasAttribute('data-pincol')), 'crossing below PIN_WIDE keeps overview pins in board');
    await ovExit(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'r-board'), 'exit uses new narrow pin placement');
    await ovEnter(page); await tab.setViewportSize({ width: 1900, height: ctx.height }); await sleep(150);
    qa(await page.evaluate(() => WsOverview.active() && document.querySelector('#bpins').parentElement.id === 'r-board' && document.documentElement.hasAttribute('data-pincol')), 'crossing above PIN_WIDE still holds pin list in board');
    await ovExit(page);
    qa(await page.evaluate(() => document.querySelector('#bpins').parentElement.id === 'bpcol'), 'exit uses new wide pin placement');
    await ovEnter(page);
    const target = await page.evaluate(() => { const n = document.querySelector('#bpins').lastElementChild; n.click(); return __bh.ovId(n); });
    await ovDone(page, false);
    qa(await page.evaluate((k) => { const n = __bh.ovFind(k), p = document.querySelector('#bpins'), r = n.getBoundingClientRect(), v = p.getBoundingClientRect(); return p.parentElement.id === 'bpcol' && n.classList.contains('overview-flash') && r.top >= v.top - 1 && r.top < v.bottom; }, target), 'pinned pick returns to column, scrolls to target and flashes');
  } else log('SKIP pin flights/column scrolling at this width: PIN_WIDE=1600 (run --width 1900)');
}

const shared = createSharedActions({ overviewReady, assert, qa, capture, assertChanged, waitQuiet, IN_PAGE, OVERVIEW_PAGE });
return { ...shared, qa, scenarioToolbar, scenarioWebContainment, scenarioRecapJitter,
  scenarioOverviewFocus, scenarioOverviewFinal, scenarioOverview, scenarioOverviewMotion };
}
