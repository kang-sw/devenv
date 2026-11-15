import fs from 'node:fs';
import path from 'node:path';

export function createSharedActions({ overviewReady, assert, qa, capture, assertChanged, waitQuiet, IN_PAGE, OVERVIEW_PAGE }) {
async function scenarioSharedControls(ctx) {
  const { page, tab, kind } = await overviewReady(ctx, { seed: false });
  const topic = 'section.topic[data-topic="SHARED"]';
  await page.waitForSelector(topic + ' > div.tdoc');
  const bodyShape = () => page.locator(topic).evaluate((t) => {
    const body = t.querySelector(':scope > .tdoc');
    return { div: body && body.tagName === 'DIV', summary: !!t.querySelector(':scope > details.tdoc,:scope > .tdoc > summary'),
      paddingMarker: t.classList.contains('has-doc'), visible: body && body.getBoundingClientRect().height > 0,
      title: t.querySelector('.tname').textContent, tickets: !!t.querySelector('.ttix, .wbtk'), state: !!t.querySelector('.tbar .chip') };
  });
  const shape = await bodyShape();
  qa(shape.div && !shape.summary && !shape.paddingMarker && shape.visible && shape.title && !shape.tickets && shape.state, 'dashboard body is visible div.tdoc without content-only disclosure or reserved padding; title/state remain without ticket slot', shape);
  // History is always shown: a plain container, never a disclosure.
  const stream = await page.locator(topic).evaluate((t) => {
    const s = [...t.querySelectorAll('.topic-stream')];
    return { count: s.length, tag: s[0]?.tagName, summary: !!s[0]?.querySelector(':scope > summary, :scope > .wb-history > summary'),
      history: !!s[0]?.querySelector(':scope > .wb-history'), visible: !!s[0] && s[0].getBoundingClientRect().height > 0 };
  });
  qa(stream.count === 1 && stream.tag !== 'DETAILS' && !stream.summary && stream.history && stream.visible, 'history is one always-shown non-details .topic-stream without a summary', stream);
  const panels = page.locator(topic + ' details.entry-panel');
  assert(await panels.count() >= 2, 'renderer supplies independent panels');
  if (!await panels.nth(0).evaluate((d) => d.open)) await panels.nth(0).locator(':scope > summary').click();
  if (await panels.nth(1).evaluate((d) => d.open)) await panels.nth(1).locator(':scope > summary').click();
  await panels.nth(0).locator(':scope > summary').click();
  qa(!await panels.nth(0).evaluate((d) => d.open) && !await panels.nth(1).evaluate((d) => d.open), 'panel summary remains independently usable');
  await panels.nth(0).locator(':scope > summary').click();
  qa(await panels.nth(0).evaluate((d) => d.open) && !await panels.nth(1).evaluate((d) => d.open), 'panel toggle preserves independently open/closed panels');
  const disclosures = () => page.locator(topic).evaluate((t) => [...t.querySelectorAll('details.entry-panel')].map((d) => [d.getAttribute('data-key'), d.open]));
  const inner = await disclosures();
  await page.locator(topic + ' a[data-wl="file"][data-rel="docs/a.md"]').first().click();
  await page.waitForSelector('.bcard[data-kind="preview"] .pv-md');
  await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.t && c.t.exited));
  const controls = await page.evaluate(() => {
    const t = document.querySelector('section.topic[data-topic="SHARED"]'), p = WsCards.list('preview')[0], term = WsCards.list('terminal').find((c) => c.t && c.t.exited);
    __bh.polishCards = [p, term];
    const rows = [{ el: t, button: t.querySelector('.t-raise') }].concat(__bh.polishCards.map((c) => ({ el: c.el, button: c.raiseBtn })));
    return rows.map((r) => ({ kind: r.el.dataset.kind || 'topic', icon: r.button.querySelector('use').getAttribute('href'), label: r.button.getAttribute('aria-label'), title: r.button.title }));
  });
  qa(controls.every((c) => c.icon === '#l-arrow-up-to-line' && c.label && c.title === c.label && c.label === controls[0].label), 'topic/preview/exited-terminal raise use upstream arrow-to-line and identical accessible labels', controls);
  const raised = await page.evaluate(() => {
    const t = document.querySelector('section.topic[data-topic="SHARED"]'), cards = __bh.polishCards;
    const rows = [{ el: t, button: t.querySelector('.t-raise'), pin: (on) => WsCards.move(t, { pin: on, index: 0 }) }].concat(cards.map((c) => ({ el: c.el, button: c.raiseBtn, pin: (on) => c.pin(on) })));
    rows.forEach((r) => r.pin(false));
    return rows.map((r) => {
      r.button.click();
      const ordinary = [...__bh.board_().children].find((n) => n.matches('section.topic,.bcard')) === r.el && !r.el.hasAttribute('data-pinned');
      rows.forEach((n) => n.pin(true));
      r.button.click();
      const pinned = WsCards.pinList().firstElementChild === r.el && r.el.hasAttribute('data-pinned');
      rows.forEach((n) => n.pin(false));
      return { ordinary, pinned };
    });
  });
  qa(raised.every((r) => r.ordinary && r.pinned), 'all three raise controls move within ordinary/pinned regions without changing pin state', raised);
  const dir = path.resolve(ctx.args['capture-dir'] || '/tmp/shared-card-polish-artifacts/' + kind + '-' + ctx.width);
  fs.mkdirSync(dir, { recursive: true });
  await page.locator(topic + ' .tbar').screenshot({ path: path.join(dir, 'topic-header-expanded.png') });
  for (const k of ['preview', 'terminal']) await page.locator('.bcard[data-kind="' + k + '"] .bc-hd').first().screenshot({ path: path.join(dir, k + '-header.png') });
  await page.locator(topic + ' .tbar').scrollIntoViewIfNeeded();
  await capture(ctx, tab, kind + '-expanded');
  await page.locator(topic + ' .t-fold').focus(); await tab.keyboard.press('Enter');
  const fold = await page.locator(topic).evaluate((t) => {
    const bar = t.querySelector('.tbar');
    return { size: t.getAttribute('data-size'), focus: document.activeElement === t.querySelector('.t-fold'),
      hidden: [...t.querySelectorAll('*')].every((n) => bar.contains(n) || n.contains(bar) || !n.getClientRects().length || n.getBoundingClientRect().height === 0),
      chrome: !!bar.querySelector('.tname') && !bar.querySelector('.ttix, .wbtk') && !!bar.querySelector('.chip') };
  });
  qa(fold.size === 'fold' && fold.focus && fold.hidden && fold.chrome, 'whole fold masks every body/history/question region while retaining header chrome and focus', fold);
  qa(JSON.stringify(await disclosures()) === JSON.stringify(inner), 'whole fold does not toggle inner disclosures');
  await page.locator(topic + ' .tbar').screenshot({ path: path.join(dir, 'topic-header-folded.png') });
  await capture(ctx, tab, kind + '-folded');
  await page.locator(topic + ' > .tophead > .tbar > .d-max').click();
  const maximum = await page.locator(topic).boundingBox();
  qa(maximum.x >= 0 && maximum.y >= 0 && maximum.x + maximum.width <= ctx.width + 1 && maximum.y + maximum.height <= ctx.height + 1 && (await bodyShape()).visible, 'folded topic maximum reveals new body container within viewport', maximum);
  await capture(ctx, tab, kind + '-maximum');
  await tab.keyboard.press('Escape');
  qa(await page.locator(topic).evaluate((t) => !t.hasAttribute('data-max') && t.getAttribute('data-size') === 'fold' && t.querySelector(':scope > .tdoc').getBoundingClientRect().height === 0), 'maximum dismissal restores whole folded mask');
  assertChanged(await page.evaluate(() => __bh.update(['SHARED'])), ['SHARED']);
  await waitQuiet(page);
  qa(await page.locator(topic).getAttribute('data-size') === 'fold' && JSON.stringify(await disclosures()) === JSON.stringify(inner), 'production topic update preserves whole and inner disclosure states');
  const updated = await bodyShape();
  qa(updated.div && !updated.summary && !updated.paddingMarker, 'topic source update does not reintroduce content-only control');
  await page.locator(topic + ' .t-fold').click();
  // Expansion remounts the updated sandbox body; measure its landed layout.
  await waitQuiet(page);
  const expanded = await bodyShape(), restoredInner = await disclosures();
  qa(expanded.visible && JSON.stringify(restoredInner) === JSON.stringify(inner), 'whole expansion restores visible body and retained independent disclosures', { expanded, inner, restoredInner });
  await page.locator(topic + ' .tbar').scrollIntoViewIfNeeded();
  await capture(ctx, tab, kind + '-updated-expanded');
}

async function scenarioSharedActions(ctx) {
  const { page, tab, kind } = await overviewReady(ctx, { seed: false });
  await tab.route('**/fs/preview?**', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ kind: 'text', text: 'Shared preview transport fixture' }) }));
  const topic = 'section.topic[data-topic="SHARED"]';
  await page.waitForSelector(topic);
  await page.evaluate(async () => { __bh.sharedServerBoard = await __bh.baseBoard(); });
  await page.evaluate(() => {
    const t = document.querySelector('section.topic[data-topic="SHARED"]');
    const ds = [...t.querySelectorAll('details')];
    ds.forEach((d, i) => { d.open = i % 2 === 0; });
    __bh.sharedDisclosures = ds.map((d) => [d.getAttribute('data-key'), d.open]);
    __bh.sharedTopic = t;
    __bh.sharedChrome = t.querySelector('.tbar').textContent;
  });
  const before = await page.locator(topic).boundingBox();
  await page.focus(topic + ' .t-fold');
  await tab.keyboard.press('Enter');
  const folded = await page.evaluate(() => {
    const t = __bh.sharedTopic, bar = t.querySelector('.tbar'), r = t.getBoundingClientRect(), b = bar.getBoundingClientRect();
    return { h: r.height, bar: b.height, text: bar.textContent, focus: document.activeElement.matches('.t-fold'), label: document.activeElement.getAttribute('aria-label'), expanded: document.activeElement.getAttribute('aria-expanded'),
      outside: [...t.querySelectorAll('*')].filter((n) => !bar.contains(n) && !n.contains(bar) && n.getClientRects().length && n.getBoundingClientRect().height > 0).map((n) => n.className) };
  });
  qa(folded.h < before.height && folded.h <= folded.bar + 4 && !folded.outside.length && folded.focus && folded.label && folded.expanded === 'false', 'whole fold removes all rendered body/history/questions, retains keyboard focus and labeled control', folded);
  qa(folded.text === await page.evaluate(() => __bh.sharedChrome), 'title and state text survive fold');
  await tab.keyboard.press('Enter');
  qa(await page.evaluate(() => JSON.stringify([...__bh.sharedTopic.querySelectorAll('details')].map((d) => [d.getAttribute('data-key'), d.open])) === JSON.stringify(__bh.sharedDisclosures)), 'expand retains every independent disclosure');
  qa(await page.locator(topic).evaluate((t) => !t.querySelector('.ttix, .wbtk') && t.textContent.includes('Open question fixture') && t.textContent.includes('History fixture') && t.querySelectorAll('details.entry-panel').length >= 2), 'fixture hides ticket slot and uses renderer-owned question, history and panel regions');
  await capture(ctx, tab, kind + '-expanded');
  const direct = page.locator(topic + ' a[data-wl="file"][data-rel="docs/a.md"]').first();
  assert(await direct.count(), 'server renderer supplies a direct question link');
  await direct.click();
  qa(await page.evaluate(() => WsCards.list('preview')[0].el.nextElementSibling === __bh.sharedTopic), 'direct renderer link opens above its enclosing topic');
  await direct.click({ button: 'right' });
  await page.waitForSelector('.lmenu');
  await page.click('.lmenu button:first-child');
  qa(await page.evaluate(() => WsCards.list('preview').length === 1 && WsCards.list('preview')[0].el.nextElementSibling === __bh.sharedTopic), 'real context menu reuses ordinary preview above origin');
  const bodyFrame = await page.locator(topic + ' iframe[data-doc-key]').first().elementHandle();
  assert(bodyFrame, 'renderer supplies an identity-bound sandbox frame');
  const content = await bodyFrame.contentFrame();
  const docKey = await bodyFrame.getAttribute('data-doc-key');
  await content.evaluate((docKey) => parent.postMessage({ wbDoc: 'link', docKey, kind: 'file', href: '/fixture/file?path=docs%2Fframe.md', rel: 'docs/frame.md', topic: 'forged-topic', x: 0, y: 0 }, '*'), docKey);
  await page.waitForSelector('.lmenu');
  await page.click('.lmenu button:first-child');
  qa(await page.evaluate(() => WsCards.list('preview')[0].el.nextElementSibling === __bh.sharedTopic), 'real sandbox postMessage dispatch uses validated frame topic, ignoring claimed topic');
  await content.evaluate(() => parent.postMessage({ wbDoc: 'link', docKey: 'wrong-key', kind: 'file', rel: 'docs/forged.md' }, '*'));
  await page.waitForTimeout(100);
  qa(await page.locator('.lmenu').count() === 0 && await page.evaluate(() => WsCards.list('preview')[0].data.path === 'docs/frame.md'), 'wrong document identity cannot open a menu or replace preview');
  await page.evaluate(() => WsCards.list('preview')[0].close());
  const placement = await page.evaluate(() => {
    const t = __bh.sharedTopic, other = [...__bh.board_().querySelectorAll('section.topic')].find((n) => n !== t);
    const p = WsPreview.open('docs/a.md', { origin: t }), out = [];
    out.push(p.el.nextElementSibling === t);
    out.push(WsPreview.open('docs/a.md', { origin: other }) === p && p.el.nextElementSibling === other);
    out.push(WsPreview.open('docs/b.md', { origin: t }) === p && p.el.nextElementSibling === t && p.data.path === 'docs/b.md');
    WsCards.move(p.el, { pin: false, before: other, after: t });
    out.push(p.el.nextElementSibling === other && p.el.nextElementSibling !== t);
    WsCards.move(t, { pin: false, before: other, after: null });
    out.push(p.el.nextElementSibling !== t);
    __bh.sharedPreview = p; __bh.sharedOther = other;
    return out;
  });
  qa(placement.every(Boolean), 'new/reuse/replacement use one-time adjacency; manual card and topic reorder remain independent', placement);
  const pins = await page.evaluate(() => {
    const t = __bh.sharedTopic, p = __bh.sharedPreview, o = __bh.sharedOther;
    WsCards.move(t, { pin: true, index: 0 });
    WsPreview.open('docs/c.md', { origin: t });
    const boundary = !p.pinned && p.el === [...__bh.board_().children].find((n) => n.matches('.bcard,section.topic'));
    p.pin(true);
    WsCards.move(o, { pin: true, index: 0 });
    const order = [...WsCards.pinList().children];
    const matching = WsPreview.open('docs/c.md', { origin: t }) === p && p.pinned && order.every((n, i) => WsCards.pinList().children[i] === n);
    p.el.querySelector('.bc-raise').click();
    const raised = WsCards.pinList().firstElementChild === p.el && p.pinned;
    t.querySelector('.t-raise').click();
    const topicRaised = WsCards.pinList().firstElementChild === t && t.hasAttribute('data-pinned');
    p.pin(false); WsCards.move(t, { pin: false }); WsCards.move(o, { pin: false });
    t.querySelector('.t-raise').click();
    const flow = [...__bh.board_().children].find((n) => n.matches('.bcard,section.topic')) === t;
    p.el.querySelector('.bc-raise').click();
    return { boundary, matching, raised, topicRaised, flow, previewFlow: p.el.nextElementSibling === t, emptyPins: !WsCards.pinList().children.length };
  });
  qa(Object.values(pins).every(Boolean), 'pin boundary, matching-pinned reuse and raise preserve each region including empty pins', pins);
  await page.evaluate(() => __bh.sharedTopic.querySelector('.d-max').click());
  const max = await page.locator(topic).boundingBox();
  qa(max.x >= 0 && max.y >= 0 && max.x + max.width <= ctx.width + 1 && max.y + max.height <= ctx.height + 1, 'topic maximum view fits actual browser viewport', max);
  await tab.keyboard.press('Escape');
  qa(await page.locator(topic).evaluate((t) => !t.hasAttribute('data-max')), 'Escape exits shared maximum view');
  // Create only an exited terminal via the existing toolbar transport seam.
  await tab.route('**/terminals', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'bh-shared-exited', cwd: '/tmp', state: 'exited', rows: 14, cols: 80, exit_code: 0 }) });
  });
  await page.click('.tnew');
  await page.waitForSelector('.topts .topt');
  await page.click('.topts .topt:first-child');
  await page.waitForFunction(() => WsCards.list('terminal').some((c) => c.data.tid === 'bh-shared-exited'));
  const terminal = await page.evaluate(() => {
    const c = WsCards.list('terminal').find((c) => c.data.tid === 'bh-shared-exited');
    c.foldBtn.click();
    const folded = c.el.getAttribute('data-size') === 'fold';
    c.foldBtn.click(); c.raiseBtn.click();
    return { folded, expanded: c.el.getAttribute('data-size') === 'base', head: c.el === [...__bh.board_().children].find((n) => n.matches('.bcard,section.topic')), label: c.raiseBtn.getAttribute('aria-label') };
  });
  qa(terminal.folded && terminal.expanded && terminal.head && terminal.label, 'real terminal kind shares fold/expand/raise with exited transport fixture only', terminal);
  await page.evaluate(() => {
    document.querySelectorAll('section.topic').forEach((t) => { if (t.getAttribute('data-size') !== 'fold') t.querySelector('.t-fold').click(); });
    WsCards.list().forEach((c) => c.fold(true));
    __bh.sharedDragAt = __bh.sharedPreview.at;
  });
  const dragHead = page.locator('.bcard[data-kind="preview"] .bc-nm');
  await dragHead.scrollIntoViewIfNeeded();
  const from = await dragHead.boundingBox(), to = await page.locator(topic + ' .tbar').boundingBox();
  assert(from && to, 'drag headers have real browser geometry');
  await tab.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await tab.mouse.down();
  await tab.mouse.move(to.x + 20, to.y + to.height - 2, { steps: 12 });
  await tab.mouse.up();
  qa(await page.evaluate(() => __bh.sharedPreview.at !== __bh.sharedDragAt && !__bh.sharedPreview.pinned && !document.documentElement.hasAttribute('data-bdrag')), 'actual pointer drag independently reorders an opened preview without pinning');
  // Each real kind's own content remains mounted through fold and sheet.
  for (const cardKind of (ctx.args.case === 'persistence' ? [] : ['preview', 'terminal'])) {
    const card = '.bcard[data-kind="' + cardKind + '"]';
    await page.locator(card + ' .bc-fold').focus();
    await tab.keyboard.press('Enter');
    const expanded = await page.locator(card + ' .bc-body').boundingBox();
    assert(expanded && expanded.height > 0, cardKind + ' body expands with real geometry');
    await tab.keyboard.press('Enter');
    qa(await page.locator(card).evaluate((c) => c.querySelector('.bc-body').getBoundingClientRect().height === 0 && document.activeElement === c.querySelector('.bc-fold') && document.activeElement.getAttribute('aria-expanded') === 'false'), cardKind + ' fold hides body and retains keyboard focus');
    await page.locator(card + ' .bc-max').click();
    const geometry = await page.locator(card + ' .bc-in').boundingBox();
    qa(geometry.x >= 0 && geometry.y >= 0 && geometry.x + geometry.width <= ctx.width + 1 && geometry.y + geometry.height <= ctx.height + 1, cardKind + ' maximum chrome fits viewport', geometry);
    qa(await page.locator(card + ' .bc-body').evaluate((n) => n.getBoundingClientRect().height > 0), cardKind + ' maximum view reveals folded body');
    await capture(ctx, tab, kind + '-' + cardKind + '-maximum');
    await tab.keyboard.press('Escape');
    qa(await page.locator(card).evaluate((c) => !c.hasAttribute('data-max') && c.getAttribute('data-size') === 'fold' && document.activeElement === c.querySelector('.bc-max')), cardKind + ' sheet dismissal restores folded state and action focus');
  }
  // Restore SHARED before the single-toggle duplicate-handler check.
  qa(await page.locator(topic).getAttribute('data-size') === 'fold', 'drag/maximum checkpoints leave topic fold intact');
  await page.evaluate(() => __bh.sharedTopic.querySelector('.t-fold').click());
  qa(await page.locator(topic).getAttribute('data-size') === 'base', 'single toggle expands topic before source updates');
  // An actual timestamp-changing update uses the same production morph.
  const disclosureBefore = await page.locator(topic).evaluate((t) => [...t.querySelectorAll('details')].map((d) => [d.getAttribute('data-key'), d.open]));
  const previewAt = await page.evaluate(() => __bh.sharedPreview.at);
  assertChanged(await page.evaluate(() => __bh.update(['SHARED'])), ['SHARED']);
  await waitQuiet(page);
  qa(JSON.stringify(await page.locator(topic).evaluate((t) => [...t.querySelectorAll('details')].map((d) => [d.getAttribute('data-key'), d.open]))) === JSON.stringify(disclosureBefore), 'changed topic source retains internal disclosure states');
  qa(await page.evaluate((at) => __bh.sharedPreview.at === at, previewAt), 'source update never changes manually placed preview snapshot');
  // Inject the real server region through the production morph path twice.
  await page.evaluate(async () => {
    const base = __bh.sharedServerBoard;
    __bh.ovRegion(base.html); __bh.ovRegion(base.html);
  });
  // A changed frame graft settles asynchronously; act on the landed head,
  // not the outgoing head that the region event is replacing.
  await waitQuiet(page);
  const actionsBefore = await page.evaluate(() => {
    WsCards.decorate(); WsCards.decorate();
    const t = document.querySelector('section.topic[data-topic="SHARED"]');
    return { size: t.getAttribute('data-size'), expanded: t.querySelector('.t-fold').getAttribute('aria-expanded'), folds: t.querySelectorAll('.t-fold').length, raises: t.querySelectorAll('.t-raise').length };
  });
  qa(actionsBefore.size === 'base' && actionsBefore.expanded === 'true' && actionsBefore.folds === 1 && actionsBefore.raises === 1, 'repeated updates retain expanded pre-toggle state and exactly one action set', actionsBefore);
  await page.locator(topic + ' .t-fold').evaluate((b) => b.click());
  const updatedActions = await page.locator(topic).evaluate((t) => ({ size: t.getAttribute('data-size'), folds: t.querySelectorAll('.t-fold').length, raises: t.querySelectorAll('.t-raise').length }));
  qa(updatedActions.size === 'fold' && updatedActions.folds === 1 && updatedActions.raises === 1, 'repeated topic updates/decorate retain one handler and one action set', updatedActions);
  await capture(ctx, tab, kind + '-folded');
  // Exclude the transport-only terminal: daemon reconciliation correctly
  // removes that unowned fixture on reload. Preview snapshot is persistent.
  const snapshot = await page.evaluate(() => ({
    at: __bh.sharedPreview.at, rank: __bh.sharedPreview.rank,
    order: [...__bh.board_().children].filter((n) => n.matches('section.topic,.bcard[data-kind="preview"]')).map((n) => n.dataset.topic || 'preview')
  }));
  await tab.reload();
  const reloaded = tab.frames().find((f) => f.url().includes('frame=1')) || tab;
  await reloaded.waitForSelector(topic + '[data-size="fold"]');
  qa(await reloaded.locator(topic + ' .t-fold').getAttribute('aria-expanded') === 'false', 'whole topic fold restores after reload');
  await capture(ctx, tab, kind + '-reload');
  // Reload creates a new __bh through INIT; install both helper layers on
  // that document, as openBoard does, before reading its placement.
  await reloaded.evaluate(IN_PAGE);
  await reloaded.evaluate(OVERVIEW_PAGE);
  const restored = await reloaded.evaluate(() => ({ at: WsCards.list('preview')[0].at, rank: WsCards.list('preview')[0].rank,
    order: [...__bh.board_().children].filter((n) => n.matches('section.topic,.bcard[data-kind="preview"]')).map((n) => n.dataset.topic || 'preview') }));
  qa(JSON.stringify(snapshot) === JSON.stringify(restored), 'manual preview snapshot and independent board order survive reload', { snapshot, restored });
  const invalid = await reloaded.evaluate(() => {
    const t = document.querySelector('section.topic[data-topic="SHARED"]'), p = WsCards.list('preview')[0];
    const head = () => [...__bh.board_().children].find((n) => n.matches('section.topic,.bcard'));
    WsPreview.open('docs/detached.md', { origin: t.cloneNode(true) });
    const detached = head() === p.el;
    WsPreview.open('docs/invalid.md', { origin: document.querySelector('.board-toolbar') });
    const wrongKind = head() === p.el;
    // Remove served topics through the production region path, not DOM surgery.
    __bh.ovRegion('<div class="empty">No ordinary topics</div>');
    WsPreview.open('docs/disappeared.md', { origin: t });
    const disappeared = head() === p.el && !t.isConnected;
    p.close();
    const empty = !head();
    const fresh = WsPreview.open('docs/empty.md');
    fresh.raiseBtn.click();
    const ordinary = head() === fresh.el && !fresh.pinned;
    fresh.pin(true); fresh.raiseBtn.click();
    const pinned = WsCards.pinList().firstElementChild === fresh.el && !!fresh.pinned;
    fresh.pin(false); fresh.raiseBtn.click();
    return { detached, wrongKind, disappeared, empty, ordinary, pinned, unpinned: head() === fresh.el && !fresh.pinned };
  });
  qa(Object.values(invalid).every(Boolean), 'invalid/disappeared origins fall back; empty ordinary and sole pinned raise preserve region state', invalid);
  await capture(ctx, tab, kind + '-empty-regions');
}

return { scenarioSharedControls, scenarioSharedActions };
}
