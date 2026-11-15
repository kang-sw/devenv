// Opt-in RED repro against the current embedded dashboard assets.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_VIEWED_PERF_PLAYWRIGHT);
const [origin, variantsFile] = process.argv.slice(2);
const variants = JSON.parse(fs.readFileSync(variantsFile, 'utf8'));
const summary = { updates: [], failures: [] };
let browser;
function gate(name, ok) {
  if (!ok) { summary.failures.push(name); console.error('FAIL gate: ' + name); }
}
(async () => {
  try {
    browser = await chromium.launch({ executablePath: process.env.WS_VIEWED_PERF_CHROMIUM, headless: true });
    const context = await browser.newContext({ viewport: { width: 1400, height: 900 }, reducedMotion: 'reduce' });
    context.setDefaultTimeout(10000);
    await context.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
    await context.addInitScript(() => {
      window.__viewedSockets = [];
      window.WebSocket = class {
        constructor(url) {
          this.url = String(url);
          window.__viewedSockets.push(this);
          setTimeout(() => { if (this.onopen) this.onopen(); }, 0);
        }
        send() {} close() {}
      };
    });
    const page = await context.newPage();
    await page.goto(origin + '/s/livesession');
    const active = page.locator('iframe.sframe.on');
    await active.waitFor({ state: 'visible' });
    const frame = await (await active.elementHandle()).contentFrame();
    assert(frame, 'active session contentFrame');
    await frame.waitForFunction(() => window.wbHistory && document.getElementById('r-board') &&
      window.__viewedSockets.some(s => s.url.includes('/s/livesession/events') && s.onmessage));
    if (!(await frame.locator('#bcol').isVisible())) await frame.locator('[data-tab="board"]:visible').first().click();
    await page.waitForTimeout(1500);
    await frame.evaluate(() => {
      window.scroller = () => {
        let n = document.querySelector('#r-board section.topic').parentElement;
        while (n && n !== document.documentElement) {
          if (/auto|scroll/.test(getComputedStyle(n).overflowY) && n.scrollHeight > n.clientHeight) return n;
          n = n.parentElement;
        }
        return document.scrollingElement;
      };
      window.__lt = [];
      window.__ltObserver = new PerformanceObserver(list => {
        for (const e of list.getEntries()) window.__lt.push({ start: e.startTime, duration: e.duration });
      });
      window.__ltObserver.observe({ type: 'longtask', buffered: true });
      window.clearLT = () => {
        window.__ltObserver.takeRecords(); window.__lt = []; window.__ltStart = performance.now();
      };
      window.longTasks = () => window.__lt.filter(e => e.start >= window.__ltStart).map(e => e.duration);
      window.send = (event, data) => {
        const ws = window.__viewedSockets.find(s => s.url.includes('/s/livesession/events') && s.onmessage);
        if (!ws) throw new Error('no events socket');
        const start = performance.now();
        ws.onmessage({ data: JSON.stringify({ event, data }) });
        return performance.now() - start;
      };
      window.patch = (prevHtml, nextHtml, hash) => {
        const parse = html => {
          const t = document.createElement('template'); t.innerHTML = html;
          return new Map([...t.content.querySelectorAll('section.topic[data-topic]')].map(n =>
            [n.dataset.topic, { th: n.dataset.th, html: n.outerHTML }]));
        };
        const prev = parse(prevHtml), next = parse(nextHtml);
        return { hash, order: [...next.keys()], topics: [...next].filter(([key, n]) =>
          !prev.has(key) || prev.get(key).th !== n.th).map(([key, n]) => ({ key, hash: n.th, html: n.html })) };
      };
      window.perfViewport = () => {
        const s = window.scroller(), r = s.getBoundingClientRect();
        return s === document.scrollingElement ? { top: 0, bottom: innerHeight } :
          { top: Math.max(0, r.top), bottom: Math.min(innerHeight, r.bottom) };
      };
    });
    const emit = (prev, name) => frame.evaluate(({ prev, next, name }) => {
      const data = window.patch(prev, next, 'v-' + name);
      window.clearLT();
      return window.send('board-topics', data);
    }, { prev: variants[prev], next: variants[name], name });
    const settle = () => frame.evaluate(() => new Promise(resolve =>
      requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(resolve, 400)))));
    const centerMouse = async () => {
      const host = await (await frame.frameElement()).boundingBox();
      const point = await frame.evaluate(() => {
        const r = window.scroller().getBoundingClientRect(), v = window.perfViewport();
        return { x: Math.max(1, Math.min(innerWidth - 1, r.left + r.width / 2)), y: (v.top + v.bottom) / 2 };
      });
      await page.mouse.move(host.x + point.x, host.y + point.y);
    };

    // A: establish the stream's full baseline before incremental updates.
    await frame.evaluate(html => window.send('region', { name: 'board', hash: 'v-base', html }), variants.base);
    await page.waitForTimeout(1000);
    let prev = 'base';
    // B: one real agent log append and recency bump per update.
    for (const name of variants.order.filter(n => /^u\d+$/.test(n))) {
      const syncMs = await emit(prev, name);
      await settle();
      const observed = await frame.evaluate(k => ({
        longTasks: window.longTasks(),
        firstTopic: document.querySelector('#r-board > section.topic')?.dataset.topic,
        appendedLogApplied: document.querySelector('#r-board section[data-topic="T' + String(k * 7 % 100).padStart(3, '0') + '"]')
          ?.textContent.includes('Agent update ' + k) || false
      }), Number(name.slice(1)));
      const expectedFirst = 'T' + String(Number(name.slice(1)) * 7 % 100).padStart(3, '0');
      summary.updates.push({ name, syncMs, ...observed, expectedFirst });
      assert(observed.appendedLogApplied, name + ': appended agent log must reach the live board');
      assert.equal(observed.firstTopic, expectedFirst, name + ': recency bump must reorder the live board');
      gate(name + ' syncMs < 50', syncMs < 50);
      gate(name + ' no longtask > 50ms', observed.longTasks.every(ms => ms <= 50));
      prev = name;
    }

    // C: start below the viewport, then reach the late-growing panel by wheel.
    summary.lateStart = await frame.evaluate(() => {
      const n = document.querySelector('#r-board section[data-topic="T060"]'), v = window.perfViewport();
      window.scroller().scrollTop += n.getBoundingClientRect().top - (v.bottom + 300);
      return { topicTop: n.getBoundingClientRect().top, viewportBottom: v.bottom };
    });
    await emit(prev, 'img');
    await page.waitForTimeout(300);
    await centerMouse();
    const fullyInside = () => frame.evaluate(() => {
      const panels = document.querySelectorAll('#r-board section[data-topic="T060"] .entry-panel');
      const last = panels[panels.length - 1], r = last?.getBoundingClientRect(), v = window.perfViewport();
      return !!r && r.height > 0 && r.top >= v.top && r.bottom <= v.bottom;
    });
    let wheelSteps = 0;
    while (!(await fullyInside()) && wheelSteps < 60) {
      await page.mouse.wheel(0, 120); wheelSteps++;
      await page.waitForTimeout(30);
    }
    await page.waitForTimeout(1500);
    summary.late = await frame.evaluate(() => {
      const v = window.perfViewport();
      const visible = [...document.querySelectorAll('#r-board .entry-panel[open]')].filter(n => {
        const r = n.getBoundingClientRect(); return r.height > 0 && r.bottom > v.top && r.top < v.bottom;
      }).map(n => ({ id: n.id, topic: n.closest('section.topic').dataset.topic,
        frameHeights: [...n.querySelectorAll('iframe.doc-frame')].map(f => f.style.height) }));
      const panel = document.querySelector('#r-board #p1453');
      return { visible, newFrameHeights: [...(panel?.querySelectorAll('iframe.doc-frame') || [])].map(f => f.style.height),
        newPanelPresent: !!panel, newPanelTop: panel?.getBoundingClientRect().top };
    });
    summary.late.wheelSteps = wheelSteps;
    summary.late.fullyInside = await fullyInside();
    gate('visible open panels have nonzero hydrated frames', summary.late.visible.length > 0 &&
      summary.late.visible.every(p => p.frameHeights.length > 0 && p.frameHeights.every(h => parseFloat(h) > 0)));
    gate('late panel frame height >= 400', summary.late.newFrameHeights.length > 0 &&
      summary.late.newFrameHeights.every(h => parseFloat(h) >= 400));

    // D: keep the user's wheel stream authoritative while a revision is pending.
    await frame.evaluate(() => { window.scroller().scrollTop = 0; });
    await page.waitForTimeout(300);
    await emit('img', 'vis');
    summary.scroll = await frame.evaluate(() => {
      const badge = document.querySelector('.board-viewed-pending');
      const visible = !!badge && !badge.hidden && badge.getBoundingClientRect().height > 0;
      const s = window.scroller();
      const changedPanelVisible = ['T001', 'T002'].map(topic => {
        const n = document.querySelector('section[data-topic="' + topic + '"] .entry-panel'), r = n?.getBoundingClientRect();
        const v = window.perfViewport(); return { topic, visible: !!r && r.bottom > v.top && r.top < v.bottom };
      });
      return { pendingBadgeVisible: visible, changedPanelVisible, initial: s.scrollTop, maxScroll: s.scrollHeight - s.clientHeight, scrollTop: [] };
    });
    await centerMouse();
    await frame.evaluate(() => window.clearLT());
    for (let i = 0; i < 25; i++) {
      await page.mouse.wheel(0, 80);
      await page.waitForTimeout(16);
      summary.scroll.scrollTop.push(await frame.evaluate(() => window.scroller().scrollTop));
    }
    summary.scroll.longTasks = await frame.evaluate(() => window.longTasks());
    const scroll = summary.scroll, series = [scroll.initial, ...scroll.scrollTop];
    scroll.final = series[series.length - 1];
    scroll.requiredDelta = 0.8 * Math.min(25 * 80, Math.max(0, scroll.maxScroll - scroll.initial));
    gate('wheel no longtask > 50ms', scroll.longTasks.every(ms => ms <= 50));
    gate('wheel progress >= 80% clamped distance', scroll.final - scroll.initial >= scroll.requiredDelta);
    gate('wheel scrollTop non-decreasing', series.every((n, i) => !i || n >= series[i - 1]));
  } catch (error) {
    summary.harnessError = error.stack;
    console.error('HARNESS ERROR: ' + error.stack);
    process.exitCode = 1;
  } finally {
    if (browser) await browser.close();
    console.log(JSON.stringify(summary));
    if (summary.failures.length) process.exitCode = 1;
  }
})().catch(error => { console.error(error.stack); process.exitCode = 1; });
