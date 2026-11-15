// Production-renderer fixture checks for TASK3. This module runs only through
// board-headless.mjs, with an explicitly owned isolated server/browser slot.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

export async function scenarioHistoryUnified(ctx, { openBoard, waitQuiet, waitFor, bindPage, log, assert }) {
  assert(ctx.args.fixtures, 'history-unified requires --fixtures from TestHistoryCaptureFixtures');
  const fixtureDir = path.resolve(ctx.args.fixtures);
  const fixture = JSON.parse(fs.readFileSync(path.join(fixtureDir, 'fixtures.json'), 'utf8'));
  const language = ctx.args.lang || 'ko';
  const dir = path.resolve(ctx.args.artifacts || path.join(os.tmpdir(), 'history-unified-captures'));
  fs.mkdirSync(dir, { recursive: true });
  const opened = await openBoard(ctx);
  let page = opened.page;
  const { tab, browser, kind } = opened;
  const report = { kind, language, width: ctx.width, beforeOnly: !!ctx.args.before, measurements: [], captures: [], comparisons: [], omitted: [] };
  const measure = (name, data) => { report.measurements.push({ name, ...data }); log(`${name}: ${JSON.stringify(data)}`); };
  const capture = async (name, selector) => {
    const file = path.join(dir, `${kind}-${language}-${ctx.width}-${name}.png`);
    if (selector) await page.locator(selector).screenshot({ path: file });
    else await tab.screenshot({ path: file, fullPage: true });
    report.captures.push(file); log(`capture: ${file}`); return file;
  };
  const paint = () => page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => setTimeout(resolve, 0))));
  const density = 'section.topic[data-topic="DENSITY"]';
  let standaloneFile = null;
  if (kind === 'standalone') {
    standaloneFile = new URL(ctx.args.url);
    assert(standaloneFile.protocol === 'file:', 'standalone fixture must use an owned generated file URL');
  }
  async function reloadDashboard() {
    await tab.reload({ waitUntil: 'load' });
    page = tab.mainFrame();
    if (await tab.evaluate(() => document.documentElement.hasAttribute('data-shell'))) {
      page = await waitFor(() => tab.frames().find((f) => f.parentFrame() === tab.mainFrame() && /[?&]frame=1(&|$)/.test(f.url())), 10000, 'fixture session frame after reload');
    }
    await page.waitForLoadState('load');
    await page.evaluate(bindPage);
    await page.waitForFunction(() => window.__bh.sockets.some((s) => s.readyState === 1 && typeof s.onmessage === 'function'), null, { timeout: 10000 });
    if (kind === 'adapter' && !await page.locator('#bcol').isVisible()) await page.click('#tabbar [data-tab="board"]');
    await waitQuiet(page);
  }
  async function inject(stage) {
    const specimen = stage.languages[language];
    assert(specimen, `missing ${stage.name}/${language} specimen`);
    if (kind === 'standalone') {
      // Only the disposable generated live artifact changes, not source or
      // authoritative board data. Its path/reply identity stay stable.
      const target = fileURLToPath(standaloneFile), source = path.join(fixtureDir, specimen.standalone);
      assert(path.dirname(target) === fixtureDir && /^live-.*\.html$/.test(path.basename(target)) && target !== source,
        'standalone --url must be a disposable live-*.html copy inside --fixtures, not a retained specimen');
      fs.copyFileSync(source, target);
      await page.reload({ waitUntil: 'load' });
      await page.evaluate(bindPage);
    } else {
      await page.evaluate(({ board, name }) => {
        const bh = window.__bh, ws = bh.sockets.find((s) => s.readyState === 1 && typeof s.onmessage === 'function');
        if (!ws) throw new Error('no production board stream handler');
        const hash = 'history-fixture-' + name;
        const before = document.querySelector('section.topic[data-topic="DENSITY"]');
        ws.onmessage({ data: JSON.stringify({ event: 'region', data: { name: 'board', hash, html: board } }) });
        bh.board = { hash, html: board };
        if (bh.board_().getAttribute('data-h') !== hash) throw new Error('renderer fixture board swap was held');
        bh.fixtureGraft = !!before && before === document.querySelector('section.topic[data-topic="DENSITY"]');
      }, { board: specimen.board, name: stage.name });
    }
    await waitQuiet(page);
  }
  async function exhaust(name) {
    for (let n = 0; n < 100; n++) {
      const more = await page.evaluate((name) => {
        const h = document.querySelector(`.wb-history[data-history="${name}"]`), b = h?.querySelector('.history-older');
        if (!b || b.hidden) return false;
        b.click(); return true;
      }, name);
      if (!more) return;
      await page.waitForFunction((name) => !document.querySelector(`.wb-history[data-history="${name}"]`)?._wbHistoryPending, name, { timeout: 3000 });
      await paint();
    }
    throw new Error(`${name}: paging did not exhaust within the fixed 100-click budget`);
  }
  async function panelState() {
    return page.evaluate((id) => {
      const p = document.getElementById(id), article = p?.closest('.wb-entry'), badge = p?.querySelector('.wb-panel-updated');
      return { present: !!p, open: !!p?.open, entry: article?.dataset.entry, pending: !!badge && !badge.hidden,
        badgeText: badge?.textContent, badgeFill: badge && getComputedStyle(badge).backgroundColor,
        sameSection: !!window.__bh.fixtureGraft };
    }, fixture.foldPanel);
  }
  async function verifyData(stage) {
    await exhaust('LEGACY'); await exhaust('DENSITY');
    const data = await page.evaluate(({ legacy, recorded, panelEntries, itemIDs, knownBatchIDs }) => {
      const h = document.querySelector('.wb-history[data-history="LEGACY"]');
      const legacyRows = [...h.querySelectorAll('.wb-entry[data-history-item]')];
      const unknown = legacyRows.every((e) => e.querySelector('.wb-hunknown') && !e.querySelector('time.entry-at'));
      const actualLegacy = legacyRows.map((e) => +e.dataset.historyItem);
      const actualRecorded = {}, clocks = [];
      for (const name of ['DENSITY', 'LEGACY', 'CLOSED']) {
        const history = document.querySelector(`.wb-history[data-history="${name}"]`);
        if (!history) throw new Error('missing shared history ' + name);
        const rows = [...history.querySelectorAll('.history-live .wb-entry[data-entry]')].filter((e) => !e.closest('.panel-versions') && /^e[1-9][0-9]*$/.test(e.dataset.entry));
        actualRecorded[name] = rows.map((e) => e.dataset.entry);
        for (const e of rows) {
          const entry = recorded[name].find((r) => r.id === e.dataset.entry), time = e.querySelector('time.entry-at');
          clocks.push({ id: e.dataset.entry, same: !!entry && time?.getAttribute('datetime') === entry.at });
        }
      }
      const expected = Object.fromEntries(Object.entries(recorded).map(([name, entries]) => [name,
        entries.filter((e) => e.kind !== 'panel' || panelEntries[e.panel_id] === e.id).map((e) => e.id)]));
      const plain = document.getElementById('item-' + itemIDs.plain)?.closest('.wb-entry');
      const rich = document.getElementById('item-' + itemIDs.rich)?.closest('.wb-entry');
      const titleLike = document.getElementById('item-' + itemIDs.titleLike)?.closest('.wb-entry');
      const histories = [...document.querySelectorAll('.wb-history')];
      const batch = recorded.DENSITY.filter((e) => e.kind === 'system' && e.text?.startsWith('Items added:') && knownBatchIDs.every((id) => e.text.includes('{{' + id + '}}')));
      const separate = [itemIDs.rich, itemIDs.plain].every((id) => recorded.DENSITY.some((e) => e.item_id === id && e.text?.startsWith('Item {{' + id + '}} added [')));
      return { knownBatch: batch.length === 1 && batch.every((e) => document.getElementById(e.id)?.textContent.includes('Items added:')), separateEqualTimeCreations: separate,
        actualLegacy, expectedLegacy: legacy, unknown, actualRecorded, expected, clocks,
        wholeHistoryExpanded: !document.querySelector('details.topic-stream,[data-key^="history:"]'),
        wholeEntries: histories.every((x) => { const c = x.querySelector('.history-clip'); return getComputedStyle(c).maxHeight === 'none' && c.scrollTop === 0 && !['auto', 'scroll', 'hidden'].includes(getComputedStyle(c).overflowY); }),
        rich: !!rich?.querySelector('table') && !!rich?.querySelector('pre code') && !!rich?.querySelector('strong') && rich.querySelectorAll('.opt.static').length === 2 && !!rich.querySelector('.rec-line') && !rich.querySelector('[data-opt],[aria-pressed],[role="button"]'),
        plain: !!plain && !plain.querySelector('details,.sbody') && plain.querySelector('.wb-hstatus')?.textContent.trim() === 'ok',
        titleLike: !!titleLike?.querySelector('details.sx .ctx'),
        openPolicy: document.querySelectorAll(`.card.policy[data-id="${String(itemIDs.openPolicy).padStart(2, '0')}"] [data-opt]`).length === 2,
        openCheck: !!document.querySelector(`.ckrow[data-id="${String(itemIDs.openCheck).padStart(2, '0')}"] label.ck > input`),
        noArchiveTime: ![...document.querySelectorAll('.wb-htime,.wb-hanswer-at')].some((n) => /archive/i.test(n.textContent)) };
    }, { legacy: fixture.legacyOrderedIDs, recorded: stage.recorded, panelEntries: stage.panelEntries, itemIDs: fixture.itemIDs, knownBatchIDs: fixture.knownBatchIDs });
    measure(stage.name + '-data', data);
    assert(JSON.stringify(data.actualLegacy) === JSON.stringify(data.expectedLegacy) && data.unknown, 'legacy rows lost their explicit unknown time or stable stored order');
    assert(Object.entries(data.expected).every(([name, ids]) => JSON.stringify(data.actualRecorded[name]) === JSON.stringify(ids)) && data.clocks.every((r) => r.same), 'recorded order/time changed, including equal-time events');
    assert(data.wholeHistoryExpanded && data.wholeEntries && data.rich && data.plain && data.titleLike && data.openPolicy && data.openCheck && data.noArchiveTime && data.knownBatch && data.separateEqualTimeCreations, 'rich/whole-entry/open-control production contract failed');
  }
  try {
    await inject(fixture.stages[0]);
    if (ctx.args.before) {
      await exhaust('DENSITY');
      await page.evaluate(() => document.querySelectorAll('details.settled,details.more,details.sx').forEach((d) => { d.open = true; }));
      await capture('before-density', density);
      await capture('before-closed', 'section.topic[data-topic="CLOSED"]');
      report.omitted.push('Before-only capture is historical evidence, not a TASK3 acceptance verdict.');
    } else {
      const band = await page.evaluate(() => {
        const h = document.querySelector('.wb-history[data-history="DENSITY"] > .mhead'), b = h.querySelector('.history-older'), l = h.querySelector('.wb-mtitle'), r = h.querySelector('.mmeta');
        const hb = h.getBoundingClientRect(), bb = b.getBoundingClientRect(), lb = l.getBoundingClientRect(), rb = r.getBoundingClientRect();
        return { height: hb.height, centered: Math.abs(bb.left + bb.width / 2 - hb.left - hb.width / 2), overlap: lb.right > bb.left || bb.right > rb.left, totals: r.textContent, visible: !b.hidden, overflow: h.scrollWidth > h.clientWidth + 1 };
      });
      measure('available-header', band);
      assert(band.visible && band.centered <= 1 && !band.overlap && !band.overflow, 'available history capsule is not centered or overlaps metadata');
      await capture('available-header', density + ' .wb-history > .mhead');
      await verifyData(fixture.stages[0]);
      const answered = page.locator('#item-' + fixture.itemIDs.answeredCheck + ' .wb-hitem, .wb-hitem#item-' + fixture.itemIDs.answeredCheck);
      const hide = page.locator(density + ' .ckhide');
      assert(await hide.count() === 1 && await answered.count() === 1, 'completed-check control or moved history row missing');
      const beforeHide = await answered.isVisible();
      await hide.click(); await paint();
      assert(await answered.isVisible() !== beforeHide, 'completed-check hide control no longer affects its moved history row');
      await hide.click(); await paint();
      assert(await answered.isVisible() === beforeHide, 'completed-check hide control did not restore its prior state');
      const unknownCheck = await page.locator('#item-' + fixture.legacyOrderedIDs[0] + ' .wb-hitem, .wb-hitem#item-' + fixture.legacyOrderedIDs[0]).isVisible();
      assert(unknownCheck, 'completed-check filter hid an explicitly undated legacy item');
      const exhausted = await page.evaluate(() => {
        const h = document.querySelector('.wb-history[data-history="DENSITY"] > .mhead');
        return { height: h.getBoundingClientRect().height, hidden: h.querySelector('.history-older').hidden, totals: h.querySelector('.mmeta').textContent };
      });
      measure('exhausted-header', exhausted);
      assert(exhausted.hidden && exhausted.height === band.height && exhausted.totals === band.totals, 'exhaustion changed the stable header band or totals');
      await page.evaluate(() => document.querySelectorAll('.wb-hitem details.sx').forEach((d) => { d.open = true; }));
      await capture('after-density', density);
      await capture('after-closed', 'section.topic[data-topic="CLOSED"]');
      await page.evaluate((id) => window.wbHistory.loadTarget(document, id), fixture.foldPanel);
      await waitQuiet(page);
      await page.locator('#' + fixture.foldPanel + ' > summary').click();
      await paint();
      let state = await panelState(); measure('reader-fold', state);
      assert(!state.open && !state.pending && state.entry === fixture.originalEntry, 'initial reader fold failed or invented an update');
      let grafts = 0;
      for (const stage of fixture.stages.slice(1)) {
        await inject(stage);
        state = await panelState(); measure(stage.name + '-panel', state);
        if (state.sameSection) grafts++;
        const changed = stage.panelEntries[fixture.foldPanel] !== fixture.originalEntry;
        assert(!state.open && state.pending === changed && state.entry === stage.panelEntries[fixture.foldPanel], 'stable pN fold or pending update state was lost');
        await verifyData(stage);
        if (stage.name === 'republish-p2') {
          if (kind !== 'standalone') { await reloadDashboard(); await inject(stage); }
          else await inject(stage);
          state = await panelState(); measure('754-pending-after-reload', state);
          assert(!state.open && state.pending && state.entry === fixture.repostEntry, 'reload lost the stable fold or unread badge');
          await verifyData(stage);
          await capture('754-pending', '#' + fixture.foldPanel + ' > summary');
        }
        if (stage.name === 'new-panel') {
          const fresh = await page.evaluate((id) => ({ open: document.getElementById(id)?.open, badge: !document.getElementById(id)?.querySelector('.wb-panel-updated')?.hidden }), fixture.newPanel);
          assert(fresh.open && !fresh.badge, 'a genuinely new panel inherited another panel fold or update');
        }
      }
      if (kind !== 'standalone') assert(grafts > 0, 'no retained-section frame-graft path was actually exercised');
      const p = page.locator('#' + fixture.foldPanel);
      await p.evaluate((d) => { d.open = true; }); await paint();
      state = await panelState(); assert(state.pending, 'programmatic/graft open falsely acknowledged an unread update');
      await p.evaluate((d) => window.wbHistory.restorePanel(d));
      await p.locator(':scope > summary').click(); await paint();
      state = await panelState(); measure('754-actual-open', state);
      assert(state.open && !state.pending, 'actual reader open did not acknowledge the update');
      await capture('754-opened', '#' + fixture.foldPanel + ' > summary');
      const legacyStage = fixture.stages.find((s) => s.name === 'republish-p2');
      await page.evaluate((entry) => {
        const html = document.documentElement, scope = html.getAttribute('data-rid') || html.getAttribute('data-wb-reply');
        localStorage.removeItem('ws.whiteboard.panels');
        localStorage.setItem('ws.dashboard.panelshut', JSON.stringify([scope + '/entry:' + entry]));
      }, fixture.originalEntry);
      if (kind !== 'standalone') await reloadDashboard();
      await inject(legacyStage);
      state = await panelState(); measure('754-legacy-entry-key-migration', state);
      assert(!state.open && state.pending && state.entry === fixture.repostEntry, 'legacy entry-key migration did not retain the old panel fold and mark its known new version');
      await verifyData(legacyStage);
    }
    for (const [name, relative] of [
      ['centered-header', 'ai-docs/tickets/assets/261114-feat-ws-whiteboard-history-centered-header/approved-centered-header.fragment.html'],
      ['decision-density', 'ai-docs/tickets/assets/261114-feat-ws-whiteboard-history-unified-decisions/approved-density.fragment.html']
    ]) {
      const reference = path.resolve(relative), ref = await browser.newPage({ viewport: { width: ctx.width, height: ctx.height } });
      await ref.setContent('<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">' + fs.readFileSync(reference, 'utf8'));
      const file = path.join(dir, `${name}-${ctx.width}-reference.png`);
      await ref.screenshot({ path: file, fullPage: true }); await ref.close();
      report.comparisons.push({ capture: report.captures.find((p) => p.includes('density')), reference, referenceCapture: file, verdict: 'pending manual side-by-side comparison' });
      log(`reference capture: ${file}`);
    }
    const observerErrors = await page.evaluate(() => window.__bh?.observerErrors || []);
    assert(ctx.errors.length === 0 && observerErrors.length === 0, `native browser errors: ${JSON.stringify({ errors: ctx.errors, observerErrors })}`);
  } finally {
    report.errors = ctx.errors;
    report.native = await page.evaluate(() => ({ observerErrors: window.__bh?.observerErrors || [] })).catch(() => null);
    fs.writeFileSync(path.join(dir, `${kind}-${language}-${ctx.width}-measurements.json`), JSON.stringify(report, null, 2));
  }
}
