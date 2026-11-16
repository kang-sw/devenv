import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { HISTORY_PAGE, LOGIC_ONLY, assert, log, openBoard, sleep, waitQuiet } from "../board-headless.mjs";

// Remediation verification, not an independent review: real renderer output,
// native RO/IO deliveries and actual sequential keyboard focus on both hosts.
async function scenarioHistoryRegressions(ctx) {
  const { page, tab, kind } = await openBoard(ctx);
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), `board-history-regressions-${Date.now()}`));
  fs.mkdirSync(dir, { recursive: true });
  const report = { url: ctx.args.url, kind, measurements: [], omissions: [] };
  const measure = (label, data) => { report.measurements.push({ label, ...data }); log(`${label}: ${JSON.stringify(data)}`); };
  const capture = async (name) => { if (LOGIC_ONLY) return; const p = path.join(dir, `${kind}-${name}.png`); await tab.screenshot({ path: p }); log(`capture: ${p}`); };
  const paint = () => page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => setTimeout(resolve, 0))));
  try {
    await page.evaluate(HISTORY_PAGE);
    await page.evaluate(() => { window.__bh.regressionPhase = 'keyboard'; });
    assert(await page.evaluate(() => ['HISTORY', 'LOGCHAIN', 'LOGCANCEL'].every((n) => !!window.__bh.history(n))),
      'seed renderer-produced panel/log histories with serve --history-entries 180');

    // Whole-entry selection intentionally removes clipped tabbable content.
    // Retain the original three identities and the strict 691 hit test: first
    // reveal the real boundary, position it under the actual pill, then reach
    // it only with native Tab. No direct focus() or synthetic focusin on it.
    async function boundaryTab(name, id, selector) {
      await page.evaluate(([name, id, selector]) => {
        const bh = window.__bh, h = bh.history(name), node = window.wbHistory.loadTarget(document, id);
        if (!node || !h.contains(node)) throw new Error(`missing original 691 target ${id}; seed --topics 8 --history-entries 180 --shared-actions`);
        const fold = node.matches(selector) ? node : node.closest(selector);
        if (!fold) throw new Error(`${id} is not the expected ${selector}`);
        fold.open = false;
        bh.foldTab = { summary: fold.querySelector(':scope > summary'), fold };
        const sentinel = document.createElement('button'); sentinel.textContent = 'Tab starting point';
        sentinel.style.cssText = 'position:absolute;height:1px;width:1px;padding:0;border:0;opacity:0';
        fold.before(sentinel); sentinel.focus({ preventScroll: true }); bh.foldTab.sentinel = sentinel;
      }, [name, id, selector]);
      await waitQuiet(page);
      let setupAttempts = 0;
      for (; setupAttempts < 3; setupAttempts++) {
        const positioned = await page.evaluate(() => {
          const bh = window.__bh, s = bh.scroller(), node = bh.foldTab.summary;
          const pill = document.querySelector('.oq-wrap.wb-oq-top .oq-pill:not([hidden])');
          if (!pill) return false;
          const r = node.getBoundingClientRect(), p = pill.getBoundingClientRect();
          if (!r.height || !p.height) return false;
          // The sticky pill can change owners/position during the scroll.
          // Use its real settled rectangle again rather than stale geometry.
          s.scrollTop += (r.top + r.height / 2) - (p.top + p.height / 2);
          return true;
        });
        if (!positioned) break;
        await waitQuiet(page);
        if (await page.evaluate(() => {
          const r = window.__bh.foldTab.summary.getBoundingClientRect();
          return !!document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)?.closest('.oq-pill');
        })) break;
      }
      const before = await page.evaluate((name) => {
        const bh = window.__bh, h = bh.history(name), node = bh.foldTab.summary, r = node.getBoundingClientRect();
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        const wrap = document.querySelector('.oq-wrap.wb-oq-top');
        const rectPill = () => wrap?.querySelector('.oq-pill:not([hidden])')?.getBoundingClientRect().toJSON() || null;
        return { boundary: h._wbHistoryChunks.at(-1), steps: h._wbHistorySteps,
          centerCoveredByPill: !!hit?.closest('.oq-pill'), height: r.height,
          control: r.toJSON(), pill: rectPill(), centerHit: hit && { tag: hit.tagName, classes: hit.className },
          collisionGone: !!wrap && !wrap.classList.contains('top') && parseFloat(getComputedStyle(wrap).marginBottom) === 0,
          whole: h.querySelector('.history-clip').scrollTop === 0 && getComputedStyle(h.querySelector('.history-clip')).maxHeight === 'none' };
      }, name);
      measure(`${id}-before-real-tab`, { ...before, setupAttempts: Math.min(setupAttempts + 1, 3) });
      if (!(before.height > 0 && before.centerCoveredByPill)) {
        const reason = `Skipped: precondition not met for ${id} at ${ctx.width}x${ctx.height}; real pill obstruction could not be established: ` +
          JSON.stringify({ control: before.control, pill: before.pill, centerHit: before.centerHit });
        report.omissions.push(reason); (ctx.skips ||= []).push(reason); log(reason);
        await page.evaluate(() => { window.__bh.foldTab.sentinel.blur(); window.__bh.foldTab.sentinel.remove(); });
        return;
      }
      assert(before.height > 0 && before.centerCoveredByPill && before.collisionGone && before.whole,
        `${id}: setup must prove genuine pill obstruction separately from the removed .top collision`);
      await capture(`${id}-before-tab`);
      await tab.keyboard.press('Tab');
      await paint();
      await waitQuiet(page);
      const after = await page.evaluate((name) => {
        const bh = window.__bh, h = bh.history(name), node = document.activeElement, r = node.getBoundingClientRect(), clip = h.querySelector('.history-clip').getBoundingClientRect(), s = bh.scroller();
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        return { expected: node === bh.foldTab.summary, boundary: h._wbHistoryChunks.at(-1), steps: h._wbHistorySteps,
          centerExposed: !!hit && (hit === node || node.contains(hit)), pillCoverage: !!hit?.closest('.oq-wrap,.oq-pill'),
          centerHit: hit && { tag: hit.tagName, id: hit.id, classes: hit.className },
          top: r.top, bottom: r.bottom, clipTop: clip.top, clipBottom: clip.bottom,
          viewTop: bh.viewTop(s), viewBottom: bh.viewBottom(s), clipScrollTop: h.querySelector('.history-clip').scrollTop,
          hit: hit && `${hit.tagName.toLowerCase()}.${hit.className}`, hitParent: hit?.parentElement && `${hit.parentElement.tagName.toLowerCase()}.${hit.parentElement.className}` };
      }, name);
      measure(`${id}-after-real-tab`, after);
      await capture(`${id}-after-tab`);
      assert(after.expected && after.boundary === before.boundary && after.steps === before.steps && after.centerExposed && !after.pillCoverage &&
        after.top >= after.clipTop - 1 && after.bottom <= after.clipBottom + 1 && after.top >= after.viewTop - 1 && after.bottom <= after.viewBottom + 1 && after.clipScrollTop === 0,
        `${id}: native Tab did not expose the actual boundary control center`);
      await page.evaluate(() => { document.activeElement.blur(); window.__bh.foldTab.sentinel.remove(); });
      await page.waitForFunction((name) => window.wbHistory.seen(window.__bh.history(name).closest('.topic'), '0px')?.visible, name, { timeout: 2000 });
      await page.evaluate((name) => {
        const bh = window.__bh, s = bh.scroller(), topic = bh.history(name).closest('.topic');
        s.scrollTop += topic.getBoundingClientRect().bottom - bh.viewTop(s) + 40;
      }, name);
      await page.waitForFunction((name) => window.__bh.history(name)._wbHistorySteps === 1, name, { timeout: 2000 });
    }
    await boundaryTab('HISTORY', 'e320', '.system-run');
    await boundaryTab('HISTORY', 'p156', '.entry-panel');
    // Log auto-fold is gone; e519 now belongs to the renderer's system chip.
    // Keep its exact identity and the same native Tab/center-hit predicate.
    await boundaryTab('LOGCHAIN', 'e519', '.system-run');

    async function settledLog(name) {
      await page.evaluate((name) => {
        const bh = window.__bh, s = bh.scroller(), topic = bh.history(name).closest('.topic');
        s.scrollTop += topic.getBoundingClientRect().top - bh.viewTop(s) - 100;
      }, name);
      await page.waitForFunction((name) => window.__bh.resizeDeliveries.some((d) => d.after?.some((r) => r.name === name && r.steps === 1)), name, { timeout: 2000 });
      await waitQuiet(page);
      // Ensure an initial observer notification cannot be mistaken for a
      // materialization-triggered delivery after our click.
      await sleep(100);
      return page.evaluate((name) => {
        const bh = window.__bh, h = bh.history(name), live = h.querySelector('.history-live');
        return { ...bh.historyState(name), folded: !!live.querySelector(':scope > details.system-run:not([open])'),
          tailRows: live.querySelectorAll(':scope > .wb-entry').length,
          deliveries: bh.resizeDeliveries.filter((d) => d.after?.some((r) => r.name === name)).length };
      }, name);
    }
    const clickLog = async (name) => {
      const result = await page.evaluate((name) => {
        const bh = window.__bh, before = bh.historyState(name);
        bh.history(name).querySelector('.history-older').click();
        return { before, after: bh.historyState(name) };
      }, name);
      await page.waitForFunction((name) => !window.__bh.history(name)._wbHistoryPending, name, { timeout: 2000 });
      result.settled = await page.evaluate((name) => window.__bh.historyState(name), name);
      return result;
    };
    // A chunk stops before 30 log lines, 3 panels or 1000px, whichever binds
    // first. These fixtures are short single-line logs, so 30 lines bind.
    const boundedClick = (r) => {
      assert(r.before.pages - r.after.pages <= 1, 'one click materialized multiple pages');
      const target = Math.min(200, r.before.includedRows + 30), pagesNeeded = target > r.before.rows ? 1 : 0;
      assert(r.settled.pages === r.before.pages - pagesNeeded && r.settled.rows === r.before.rows + pagesNeeded * 50,
        'settled log chunk loaded extra pages/rows or stopped before its exact candidate page');
      assert(r.settled.steps === r.before.steps + 1 && r.settled.includedRows === Math.min(200, r.before.includedRows + 30),
        'log-only click did not add exactly thirty entries (or the final remainder)');
      assert(r.settled.chunks.slice(0, -1).join() === r.before.chunks.join(), 'log click recut prior chunks');
    };

    await page.evaluate(() => { window.__bh.regressionPhase = 'log-thirty'; });
    const initial = await settledLog('LOGCHAIN');
    measure('log-thirty-initial', initial);
    assert(initial.rows === 50 && initial.pages >= 3 && initial.folded && initial.tailRows === 3 && initial.deliveries >= 1 && initial.includedRows === 30 && initial.height < 1000,
      'need a renderer-produced folded history whose first chunk is thirty lines under the 1000px budget');
    await capture('before-log-thirty');
    for (let i = 0; i < 6; i++) {
      const click = await clickLog('LOGCHAIN');
      boundedClick(click);
      await paint();
      measure('log-thirty-click', click);
    }
    const drained = await page.evaluate(() => window.__bh.historyState('LOGCHAIN'));
    measure('log-thirty-drained', drained);
    assert(drained.pages === 0 && drained.rows === 200 && drained.includedRows === 200 && !drained.button,
      'log chunks did not reach the exact final remainder');
    await capture('after-log-thirty');

    const appendSwap = await page.evaluate(() => {
      const bh = window.__bh, h = bh.history('LOGCHAIN'), opened = Array.from(h.querySelectorAll('.system-run')).at(-1);
      opened.open = true;
      const visible = Array.from(opened.querySelectorAll('.wb-entry')).map((e) => e.id);
      const snapshot = window.wbHistory.capture(document), incoming = h.cloneNode(true), live = incoming.querySelector('.history-live');
      // Renderer pages carry unsplit chips: rejoin the client's chip cuts.
      const runs = Array.from(live.querySelectorAll(':scope > .system-run')), first = runs[0];
      runs.slice(1).forEach((run) => { Array.from(run.children).slice(1).forEach((row) => first.appendChild(row)); run.remove(); });
      first.open = false;
      const row = live.lastElementChild.cloneNode(true); row.id = 'e999003'; row.dataset.entry = row.id; row.dataset.historyAnchor = row.id;
      live.appendChild(row); h.replaceWith(incoming);
      window.wbHistory.init(incoming); window.wbHistory.restore(document, snapshot);
      return { state: bh.historyState('LOGCHAIN'), visible,
        restored: visible.map((id) => { const e = incoming.querySelector('#' + id); return { id, present: !!e, open: !!e?.closest('.system-run')?.open }; }) };
    });
    await paint();
    measure('open-log-append-swap', appendSwap);
    assert(appendSwap.visible.length > 0 && appendSwap.restored.every((r) => r.present && r.open),
      'append+swap folded rows that were actually visible in a saved open log run');

    // Native IO still collapses a partially loaded history without loading
    // the rest of its pages. No observer callbacks are called manually.
    await page.evaluate(() => { window.__bh.regressionPhase = 'cancellation'; });
    const cancelInitial = await settledLog('LOGCANCEL');
    assert(cancelInitial.pages >= 8 && cancelInitial.rows === 50, 'need older pages for collapse cancellation');
    const cancelClick = await clickLog('LOGCANCEL');
    boundedClick(cancelClick);
    await page.evaluate(() => {
      const bh = window.__bh; bh.cancelVisible = false;
      bh.cancelWatch = window.wbHistory.watch(bh.history('LOGCANCEL').closest('.topic'), {
        onChange: (r) => { if (r.visible) bh.cancelVisible = true; }
      });
    });
    await page.waitForFunction(() => window.__bh.cancelVisible, null, { timeout: 2000 });
    const exit = await page.evaluate(() => {
      const bh = window.__bh, topic = bh.history('LOGCANCEL').closest('.topic'), s = bh.scroller();
      const before = bh.historyState('LOGCANCEL');
      s.scrollTop += topic.getBoundingClientRect().bottom - bh.viewTop(s) + 40;
      return before;
    });
    assert(exit.pages > 0 && exit.steps === 2, 'cancellation fixture drained before its visible->out transition');
    await page.waitForFunction(() => window.__bh.history('LOGCANCEL')._wbHistorySteps === 1, null, { timeout: 2000 });
    const collapsed = await page.evaluate(() => window.__bh.historyState('LOGCANCEL'));
    await sleep(350);
    const canceled = await page.evaluate(() => {
      const bh = window.__bh; bh.cancelWatch.unwatch();
      return { ...bh.historyState('LOGCANCEL'), pending: !!bh.history('LOGCANCEL')._wbHistoryPending };
    });
    measure('neutral-chain-canceled', { initial: cancelInitial, click: cancelClick, exit, collapsed, settled: canceled });
    assert(collapsed.pages > 0 && canceled.pages === collapsed.pages && canceled.rows === collapsed.rows && canceled.steps === 1 && canceled.includedRows === 30 && canceled.height < 1000 && !canceled.pending,
      'collapsed history continued materializing older pages or lost its first-chunk boundary (thirty short lines)');
    await capture('after-cancellation');
    const observerErrors = await page.evaluate(() => window.__bh.observerErrors);
    assert(observerErrors.length === 0, `native ResizeObserver loop errors: ${observerErrors.join(' | ')}`);
  } catch (e) {
    report.failure = e.message;
    throw e;
  } finally {
    report.errors = ctx.errors;
    report.native = await page.evaluate(() => ({ deliveries: window.__bh.resizeDeliveries,
      loads: window.__bh.historyLoads, observerErrors: window.__bh.observerErrors,
      observerErrorDetails: window.__bh.observerErrorDetails })).catch(() => null);
    fs.writeFileSync(path.join(dir, 'measurements.json'), JSON.stringify(report, null, 2));
    log(`history regression artifacts: ${dir}`);
  }
}

// Real renderer markup and native controls, not the approved design mockup.
async function scenarioPanelHeader(ctx) {
  const { page, tab, kind } = await openBoard(ctx);
  const target = await page.evaluate(() => {
    const h = document.querySelector('[data-history="HISTORY"]');
    if (!h) throw new Error('need serve --history-entries 180');
    const panels = Array.from(h.querySelectorAll('.history-live > .wb-entry > .entry-panel'));
    const panel = panels.at(-1), long = panels.at(-2);
    long.querySelector('.entry-recap').textContent = 'Full naturally wrapping title ' + '아주 긴 제목 LongUnbrokenTitle0123456789'.repeat(12);
    panel.open = true;
    window.__bh.headerPanels = [panel, long];
    window.__bh.headerBodies = [panel, long].map((p) => {
      const body = p.querySelector(':scope > .entry-body'), frame = body.querySelector('iframe');
      return { body, frame, html: frame ? frame.getAttribute('srcdoc') : body.innerHTML };
    });
    window.__bh.headerCache = Array.from(h.querySelectorAll('template[data-history-page]')).map((t) => t.innerHTML);
    window.__bh.headerIDs = Array.from(h.querySelectorAll('[data-entry]')).map((e) => e.dataset.entry);
    return panel.id;
  });
  const panel = page.locator(`#${target}`), summary = panel.locator(':scope > summary');
  // Intrinsic renderer spacing is deliberately unlike the shell's 6px gap.
  const bodyHandle = await panel.locator(':scope > .entry-body').elementHandle();
  const frameHandle = await page.$(`#${target} iframe`);
  const payload = frameHandle ? await frameHandle.contentFrame() : page;
  await payload.evaluate((framed) => {
    const body = framed ? document.querySelector('.doc-body') : window.__bh.headerBodies[0].body;
    const probe = document.createElement('section');
    probe.id = 'bh-panel-body';
    probe.innerHTML = '<p style="margin:23px 0 17px">Arbitrary renderer spacing</p><pre style="max-width:100%;overflow:auto">' + 'unbroken body code '.repeat(60) + '</pre><details open><summary>Inner disclosure</summary><p>Inner body</p></details>';
    body.append(probe);
  }, !!frameHandle);
  // Save after adding the inline fixture; neither layout nor actions may replace it.
  if (!frameHandle) await page.evaluate(() => { const b = window.__bh.headerBodies[0]; b.html = b.body.innerHTML; });
  const measurements = [];
  async function viewport(width) {
    await tab.setViewportSize({ width, height: ctx.height });
    // Responsive adapter tabs may hide the board after crossing a breakpoint.
    if (kind === 'adapter' && !await panel.isVisible()) await page.click('#tabbar [data-tab="board"]');
  }
  async function measure(label) {
    await sleep(220);
    const r = await page.evaluate(() => {
      const bh = window.__bh, h = document.querySelector('[data-history="HISTORY"]');
      const issues = [], rows = [];
      for (const p of bh.headerPanels) {
        const s = p.querySelector(':scope > summary'), title = s.querySelector('.entry-recap'), meta = s.querySelector('.pmeta');
        const sr = s.getBoundingClientRect(), tr = title.getBoundingClientRect(), mr = meta.getBoundingClientRect();
        const ts = getComputedStyle(title), ss = getComputedStyle(s), narrow = p.clientWidth <= 420;
        const controls = Array.from(s.querySelectorAll('a.mref, button')).filter((n) => getComputedStyle(n).display !== 'none');
        const boxes = [title, meta, ...controls.filter((n) => n.tagName === 'BUTTON')].map((n) => n.getBoundingClientRect());
        for (let i = 0; i < boxes.length; i++) {
          const a = boxes[i];
          if (a.left < sr.left - 1 || a.right > sr.right + 1) issues.push('header overflow ' + p.id);
          for (const b of boxes.slice(i + 1)) if (Math.min(a.right, b.right) - Math.max(a.left, b.left) > 1 && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 1) issues.push('overlap ' + p.id);
        }
        if ((mr.top > tr.top + 1) !== narrow) issues.push('wrong metadata row ' + p.id);
        if (Math.abs(Math.max(mr.right, ...controls.map((n) => n.getBoundingClientRect().right)) - sr.right) > 1) issues.push('metadata not right aligned');
        if (ts.textOverflow === 'ellipsis' || ts.overflowY === 'hidden' || +ts.webkitLineClamp > 0 || title.scrollHeight > title.clientHeight + 1) issues.push('title clipped');
        if (ss.padding !== '0px' || sr.height < 24 || parseFloat(ss.columnGap) !== 8 || ss.borderBottomWidth !== '0px') issues.push('header shell');
        for (const c of controls) { const r = c.getBoundingClientRect(); if (r.width < 24 || r.height < 24) issues.push('target <24px'); }
        if (!meta.querySelector('time[title][datetime]') || !meta.querySelector('.hint') || !meta.querySelector('a.mref')) issues.push('missing metadata');
        const ps = getComputedStyle(p);
        if ((!p.hasAttribute('data-max') && ps.boxShadow !== 'none') || ps.borderTopWidth !== '0px') issues.push('enclosing border');
        if (p.hasAttribute('data-max') && (ps.backgroundColor === 'rgba(0, 0, 0, 0)' || ps.backgroundColor !== ss.backgroundColor)) issues.push('maximized sheet is not opaque like its header');
        if (p.open) {
          const body = p.querySelector(':scope > .entry-body'), bs = getComputedStyle(body);
          if (bs.marginTop !== '6px' || bs.marginLeft !== '18px' || bs.paddingLeft !== '12px' || bs.borderLeftWidth !== '1px') issues.push('body shell');
        }
        rows.push({ id: p.id, width: p.clientWidth, height: sr.height, metaY: mr.top - sr.top, titleHeight: tr.height, open: p.open, background: ps.backgroundColor });
      }
      for (const b of bh.headerBodies) if (!b.body.isConnected || (b.frame ? b.frame !== b.body.querySelector('iframe') || b.html !== b.frame.getAttribute('srcdoc') : b.html !== b.body.innerHTML)) issues.push('body replaced');
      if (JSON.stringify(bh.headerCache) !== JSON.stringify(Array.from(h.querySelectorAll('template[data-history-page]')).map((t) => t.innerHTML))) issues.push('cached pages changed');
      if (JSON.stringify(bh.headerIDs) !== JSON.stringify(Array.from(h.querySelectorAll('[data-entry]')).map((e) => e.dataset.entry))) issues.push('history identity/order changed');
      if (document.documentElement.scrollWidth > innerWidth + 1) issues.push('document overflow');
      const natural = { chunks: h._wbHistoryChunks.slice(), cache: { ...h._wbHistoryHeights },
        height: h.querySelector('.history-clip').getBoundingClientRect().height,
        scrollTop: h.querySelector('.history-clip').scrollTop, pending: !!h._wbHistoryPending };
      if (!bh.headerNatural) bh.headerNatural = { chunks: natural.chunks, cache: natural.cache };
      if (JSON.stringify(natural.chunks) !== JSON.stringify(bh.headerNatural.chunks)) issues.push('finalized boundary recut');
      if (Object.keys(bh.headerNatural.cache).some((k) => natural.cache[k] !== bh.headerNatural.cache[k])) issues.push('first-height cache rewritten');
      if (natural.pending || natural.scrollTop !== 0) issues.push('history pending or clip scrolled');
      if (bh.observerErrors.length) issues.push('native ResizeObserver errors: ' + bh.observerErrors.join(' | '));
      return { issues, rows, natural, native: { observerErrors: bh.observerErrors.slice(),
        observerErrorDetails: bh.observerErrorDetails.slice() } };
    });
    measurements.push({ label, ...r });
    if (ctx.args['capture-dir']) {
      const dir = path.resolve(ctx.args['capture-dir']); fs.mkdirSync(dir, { recursive: true });
      // Write before asserting so a failed native delivery keeps its state.
      fs.writeFileSync(path.join(dir, `${kind}-panel-header.json`), JSON.stringify({ kind, measurements }, null, 2));
      await tab.screenshot({ path: path.join(dir, `${kind}-panel-header-${label}.png`) });
    }
    assert(!r.issues.length, `${label}: ${JSON.stringify(r)}`);
    const intrinsic = await payload.evaluate(() => {
      const n = document.querySelector('#bh-panel-body > p'), s = getComputedStyle(n);
      return [s.marginTop, s.marginBottom, !!document.querySelector('#bh-panel-body > details[open]')];
    });
    assert(JSON.stringify(intrinsic) === JSON.stringify(['23px', '17px', true]), 'intrinsic body spacing/disclosure changed');
  }
  for (const width of (LOGIC_ONLY ? [ctx.width] : [1440, 800, 360, 320])) {
    await viewport(width);
    await measure(`viewport-${width}-open`);
    // Native keyboard disclosure, with focus in the real clipped history.
    await summary.focus();
    await tab.keyboard.press('Enter');
    assert(!await panel.evaluate((n) => n.open), 'native keyboard collapse failed');
    await page.evaluate(() => { window.__bh.headerPanels[1].open = false; });
    await measure(`viewport-${width}-closed`);
    await tab.keyboard.press('Enter');
    assert(await panel.evaluate((n) => n.open), 'native keyboard expand failed');
    await page.evaluate(() => { window.__bh.headerPanels[1].open = true; });
  }
  await viewport(1440);
  for (const width of (LOGIC_ONLY ? [] : [420, 421])) {
    await page.evaluate((w) => { document.querySelector('[data-history="HISTORY"]').style.width = w + 'px'; }, width);
    await measure(`wide-host-content-${width}`);
  }
  // Existing hover formatter keeps the exact stamp alongside its relative prefix.
  const tooltip = await panel.locator('time.entry-at').evaluate((t) => {
    t.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }));
    return { title: t.title, stamp: t.dataset.stamp };
  });
  assert(tooltip.stamp && tooltip.title.includes(tooltip.stamp), 'exact-time tooltip lost');
  await page.evaluate(() => window.wbAgo(document));
  await measure('existing-clock-refresh');
  if (kind !== 'standalone') {
    await page.evaluate(() => { document.querySelector('[data-history="HISTORY"]').style.width = '420px'; });
    await panel.locator(':scope > summary > .d-max').click();
    assert(await panel.evaluate((n) => n.hasAttribute('data-max')), 'native maximize failed');
    await measure('maximized-from-narrow-history');
    if (ctx.args['capture-dir']) {
      const dir = path.resolve(ctx.args['capture-dir']);
      fs.mkdirSync(dir, { recursive: true });
      await tab.screenshot({ path: path.join(dir, `${kind}-panel-maximized.png`) });
    }
    assert(await panel.locator(':scope > summary > .d-min').isVisible(), 'native back action missing');
    await tab.keyboard.press('Escape');
    assert(!await panel.evaluate((n) => n.hasAttribute('data-max')), 'Escape failed');
    await measure('after-Escape');
  }
  assert(await bodyHandle.evaluate((n) => n.isConnected), 'body identity lost');
  log(JSON.stringify({ kind, measurements, tooltip, gaps: [
    'Natural wrapping preserves finalized boundaries/first heights; run history/history-regressions for paging and scroll stability.',
    'Uses current branch clock refresh only; unmerged relative-time work remains excluded.',
    ...(kind === 'standalone' ? ['Standalone intentionally has no maximize/minimize controls or body iframe.'] : [])
  ] }, null, 2));
}

// Controlled geometry in the real host, using cloned renderer rows and the
// shipped history implementation. No substitute formatter, timer or observer.
async function scenarioHistoryFit(ctx) {
  const { page, kind } = await openBoard(ctx);
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), `board-history-fit-${Date.now()}`));
  fs.mkdirSync(dir, { recursive: true });
  const results = [];
  const paint = () => page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(r, 0)))));
  try {
    await page.evaluate(HISTORY_PAGE);
    await page.evaluate(() => {
      const bh = window.__bh;
      bh.fitPanel = bh.history().querySelector('.wb-entry.panel').cloneNode(true);
      bh.fitLog = bh.history('LOGCHAIN').querySelector('.wb-entry.agent').cloneNode(true);
      bh.fitFixture = (heights, unknown = false) => {
        const old = bh.history(), h = old.cloneNode(false);
        h.innerHTML = '<div class="mhead"><button type="button" class="history-older">older</button></div><div class="history-clip"><div class="history-live"></div></div>';
        const live = h.querySelector('.history-live');
        let id = 900000;
        const row = (panel, height) => {
          const n = (panel ? bh.fitPanel : bh.fitLog).cloneNode(true);
          n.id = 'e' + (++id); n.dataset.entry = n.id; n.dataset.historyAnchor = n.id;
          n.removeAttribute('data-wb-history-before');
          n.style.cssText = `height:${height}px;margin:0;padding:0;box-sizing:border-box;overflow:hidden`;
          if (panel) {
            n.dataset.panel = 'p' + id;
            const d = n.querySelector('.entry-panel'); d.id = n.dataset.panel; d.open = true; d.dataset.key = 'entry:' + n.id;
            d.querySelectorAll('.entry-body,.panel-versions').forEach((b) => b.remove());
          }
          live.appendChild(n); return n;
        };
        row(false, 20);
        heights.forEach((height) => { row(true, height); row(false, 20); });
        if (unknown) {
          const f = document.createElement('div'); f.className = 'doc-frame';
          const panel = live.querySelectorAll('.entry-panel')[2];
          if (unknown === 'closed') {
            // Native DETAILS with the same historical-disclosure shape as
            // the renderer: the positive active panel is visible, its old
            // version and unsized frame are behind a closed versions fold.
            const versions = document.createElement('details'); versions.className = 'panel-versions';
            versions.innerHTML = '<summary>Earlier versions</summary><article class="wb-entry panel historical"><details class="entry-panel"><summary>Old version</summary><div class="entry-body"></div></details></article>';
            versions.querySelector('.entry-body').appendChild(f); panel.appendChild(versions);
          } else panel.appendChild(f);
        }
        old.replaceWith(h); window.wbHistory.init(h);
        return bh.historyState();
      };
    });
    // Rows are L1 P2 L3 P4 L5 P6 L7 (logs 20px). A chunk walks back from L7
    // and stops before a row would pass 30 lines, 3 panels or 1000px; the
    // newest row always joins, so an oversized P6 leaves L7 alone.
    for (const [heights, boundary, pixels] of [
      [[300, 300, 300], 'e900001', 980],
      [[400, 400, 400], 'e900003', 860],
      [[600, 600, 600], 'e900005', 640],
      [[1500, 1500, 1500], 'e900007', 20],
    ]) {
      const state = await page.evaluate((h) => window.__bh.fitFixture(h), heights);
      await paint();
      results.push({ heights, state });
      assert(state.chunks[0] === boundary && Math.abs(state.height - pixels) <= 1 && !state.pending,
        `wrong panel fit: ${JSON.stringify({ heights, state })}`);
    }
    const hidden = await page.evaluate(() => {
      const bh = window.__bh, state = bh.fitFixture([400, 400, 400], 'closed');
      const cached = bh.history()._wbHistoryHeights['e900006:true'];
      bh.history().querySelector('.history-older').click();
      return { state, cached, expanded: bh.historyState() };
    });
    assert(!hidden.state.pending && hidden.state.chunks[0] === 'e900003' && hidden.cached === 400 && hidden.expanded.steps === 2,
      'unsized closed historical frame blocked a positive active panel or its older-page control');
    const zero = await page.evaluate(() => {
      const bh = window.__bh, state = bh.fitFixture([400, 400, 0]);
      return { state, cached: bh.history()._wbHistoryHeights['e900006:true'] };
    });
    assert(zero.state.pending && zero.cached === undefined, 'zero active height must stay unknown, not enter the cache');
    await page.evaluate(() => window.__bh.history().querySelector('#e900006').style.height = '400px');
    await page.waitForFunction(() => !window.__bh.history()._wbHistoryPending, null, { timeout: 2000 });
    const progressed = await page.evaluate(() => window.__bh.historyState());
    assert(progressed.chunks[0] === 'e900003', 'first nonzero active layout did not progress');
    results.push({ hidden, zero, progressed });
    const unknown = await page.evaluate(() => window.__bh.fitFixture([400, 400, 400], true));
    // Counts alone admit every row provisionally until P6 renders.
    assert(unknown.pending && unknown.chunks[0] === 'e900001', 'unknown panel must wait for its first rendered height');
    await page.evaluate(() => {
      const h = window.__bh.history(), last = h.querySelector('#e900006');
      last.querySelector('.doc-frame').style.height = '100px'; last.style.height = '401px';
    });
    await page.waitForFunction(() => !window.__bh.history()._wbHistoryPending);
    await paint();
    const measured = await page.evaluate(() => window.__bh.historyState());
    assert(measured.chunks[0] === 'e900003', 'first rendered height did not settle the two-panel chunk');
    await page.evaluate(() => window.__bh.history().querySelector('.history-older').click());
    await paint();
    const expanded = await page.evaluate(() => window.__bh.historyState());
    assert(expanded.chunks.join() === 'e900003,e900001' && !expanded.button, 'expansion did not take the remaining panel and logs as one chunk');
    await page.evaluate(() => {
      const h = window.__bh.history();
      // Keep this controlled resize probe distinct from the real off-screen
      // folding checks in history/history-regressions. Existing focus guards
      // hold the expanded fixture while its oversized boxes grow.
      h.querySelector('#e900002 > .entry-panel > summary').focus({ preventScroll: true });
      h.querySelector('#e900003').style.height = '200px';
      h.querySelector('#e900006').style.height = '1600px';
    });
    await paint();
    const wrapped = await page.evaluate(() => window.__bh.historyState());
    results.push({ unknown, measured, expanded, wrapped });
    assert(wrapped.chunks.join() === expanded.chunks.join() && wrapped.height > expanded.height + 1300,
      `native resize changed finalized boundaries or failed to follow growth: ${JSON.stringify({ expanded, wrapped })}`);
    const swapped = await page.evaluate(() => {
      const bh = window.__bh, snapshot = window.wbHistory.capture(document);
      bh.fitFixture([1500, 1500, 1500]); window.wbHistory.restore(document, snapshot);
      return { state: bh.historyState(), cached: bh.history()._wbHistoryHeights['e900006:true'] };
    });
    await paint();
    assert(swapped.state.chunks.join() === expanded.chunks.join() && swapped.cached === 401,
      'swap did not restore exact expanded boundaries and first-height cache');
    results.push({ swapped });
    const errors = await page.evaluate(() => window.__bh.observerErrors);
    assert(errors.length === 0, `native ResizeObserver loop errors: ${errors.join(' | ')}`);
    log(`panel-fit: ${JSON.stringify(results)}`);
  } finally {
    fs.writeFileSync(path.join(dir, 'measurements.json'), JSON.stringify({ kind, results, errors: ctx.errors }, null, 2));
    log(`history fit artifacts: ${dir}`);
  }
}

export { scenarioHistoryRegressions, scenarioPanelHeader, scenarioHistoryFit };
