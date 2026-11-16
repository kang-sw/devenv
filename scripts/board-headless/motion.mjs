import { Failure, assert, assertChanged, growFrame, log, openBoard, qa, scrollToMiddle, segments, sleep, trace, waitQuiet } from "../board-headless.mjs";

async function scenarioKeep(ctx) {
  const { page, tab, kind } = await openBoard(ctx);
  if (ctx.args['face-width']) await page.evaluate((width) => {
    const bar = document.querySelector('.board-toolbar'); bar.style.width = width + 'px'; bar.style.maxWidth = '100%';
  }, +ctx.args['face-width']);
  await waitQuiet(page);
  // Width changes can leave a cached quiet topic layout while the toolbar's
  // native ResizeObserver is still pending. Establish its real top footprint
  // before taking the reserved-slot baseline, never accept the stale slot.
  await page.waitForFunction(() => {
    const bar = document.querySelector('.board-toolbar');
    return !bar.hasAttribute('data-floating') && Math.abs(bar.getBoundingClientRect().height - bar.firstElementChild.getBoundingClientRect().height) < .1;
  }, null, { polling: 'raf' });
  const originalSlot = await page.evaluate(() => document.querySelector('.board-toolbar').getBoundingClientRect().height);
  log(`page kind: ${kind} (scroller: ${kind === "adapter" ? "#bcol" : "document"})`);
  const scrollerName = await page.evaluate(() => { const s = window.__bh.scroller(); return s === document.scrollingElement ? "document" : "#" + s.id; });
  assert(scrollerName === (kind === "adapter" ? "#bcol" : "document"), `the board scrolls in ${scrollerName} on a ${kind} page`);
  // At most 2.5 viewports down: the topic updated above rises to the
  // head, which must stay within the viewport bands' far band (4 viewports)
  // for its frame to stay mounted and report late.
  let lay;
  if (ctx.args['framed-middle']) {
    // History-heavy fixtures begin with log-only topics. Choose a real T
    // body in the middle, not a pixel distance dominated by those histories.
    await page.evaluate(() => {
      const bh = window.__bh, topics = bh.topics().filter((n) => /^T\d+$/.test(n.dataset.topic));
      const n = topics[Math.floor(topics.length / 2)];
      if (!n) throw new Error('framed-middle needs rendered T topics');
      bh.scroller().scrollTop += n.getBoundingClientRect().top - bh.viewTop(bh.scroller()) + 36;
    });
    await waitQuiet(page);
    lay = await page.evaluate(() => window.__bh.layout());
  } else lay = await scrollToMiddle(page, 2.5);
  const above = lay.filter((t) => t.where === "above"), below = lay.filter((t) => t.where === "below"), inView = lay.filter((t) => t.where === "in");
  assert(above.length >= 3 && below.length >= 1 && inView.length >= 1, `the board must overflow both ways from the middle (above ${above.length}, in view ${inView.length}, below ${below.length}); seed more topics`);
  // The update above is the second topic (it rises past the head, off
  // screen); the update below is the bottom open topic, which rises past
  // the viewport to the head. The frame that grows late is the one the
  // first swap brought for the topic updated above: a swap's frames report
  // their heights late (load, reveal), which is what the compensation has
  // to hold through after the swap's own motion.
  // History seeds also contain log-only topics. Late body-height probes
  // require a real frame, not simply the second topic in board order.
  const framedAbove = await page.evaluate((names) => names.filter((n) => window.__bh.section(n).querySelector('.tdoc iframe.doc-frame')), above.map((t) => t.name));
  assert(framedAbove.length >= 2, 'need two framed topics above for the late-height probe');
  // The framed topic above nearest the view: one far up may be unmounted,
  // and its update brings a placeholder the viewport bands mount only once
  // it comes near, not a frame that reports late.
  const upAbove = framedAbove.at(-1), lateGrow = ctx.args['framed-middle'] ? framedAbove.at(-2) : upAbove, upBelow = below[below.length - 1].name;
  if (ctx.args['framed-middle']) {
    // Real focus is the existing production bands-preservation contract.
    // Grow an unchanged frame, never a replaced/grafted one moved off-band.
    const held = await page.evaluate((name) => {
      const f = __bh.section(name).querySelector('.tdoc iframe.doc-frame');
      f.focus({ preventScroll: true }); __bh.keepHeldFrame = f; __bh.keepHeldWindow = f.contentWindow;
      return { name, focused: document.activeElement === f, above: f.getBoundingClientRect().bottom <= __bh.viewTop(__bh.scroller()) };
    }, lateGrow);
    qa(held.focused && held.above, 'late-height fixture holds a genuinely focused unchanged iframe above the read position', held);
  }
  const tracked = inView.map((t) => t.name);
  log(`in view: ${tracked.join(", ")}; update above: ${upAbove}; update below: ${upBelow}; late frame: ${lateGrow}`);

  // 1. Two swaps, then a late frame height, sampled every animation frame.
  await page.evaluate((names) => window.__bh.startSampler(names), tracked);
  await sleep(50);
  assertChanged(await page.evaluate((n) => window.__bh.update([n]), upAbove), [upAbove]);
  await sleep(300);
  const b = await page.evaluate((n) => window.__bh.update([n]), upBelow);
  assertChanged(b, [upBelow]);
  assert(b.order[0] === upBelow && b.order[1] === upAbove, `the board order after both updates starts ${b.order.slice(0, 3).join(", ")}`);
  await sleep(ctx.args['framed-middle'] ? 150 : 1000);
  if (ctx.args['framed-middle']) qa(await page.evaluate((name) => {
    const f = __bh.section(name).querySelector('.tdoc iframe.doc-frame');
    return f === __bh.keepHeldFrame && f.contentWindow === __bh.keepHeldWindow && document.activeElement === f && !!__bh.keeping();
  }, lateGrow), 'focused late iframe identity and real keep session survive both production swaps');
  const keepAtLate = await page.evaluate((n) => { window.__bh.mark(`late frame height (${n} +300px)`); return !!window.__bh.keeping(); }, lateGrow);
  await growFrame(page, lateGrow, 300);
  await sleep(600);
  const samples = await page.evaluate(() => window.__bh.stopSampler());
  log(`sampled ${samples.filter((s) => s.y).length} frames over ${samples[samples.length - 1].t.toFixed(0)}ms; keep session when the late frame reported: ${keepAtLate ? "running" : "ended"}`);
  const failed = [];
  for (const seg of segments(samples, tracked)) {
    const parts = tracked.map((n) => seg.worst[n] ? `${n} ${seg.worst[n].d.toFixed(2)}px@${seg.worst[n].t.toFixed(0)}ms` : `${n} -`);
    const bad = tracked.filter((n) => seg.worst[n] && seg.worst[n].d > 1);
    log(`  [${seg.label}] from ${seg.from.toFixed(0)}ms: max |dy| ${parts.join(", ")}${bad.length ? "  <-- over 1px" : ""}`);
    if (bad.length) failed.push(`${seg.label}: ${bad.map((n) => `${n} moved ${seg.worst[n].d.toFixed(1)}px`).join(", ")}`);
  }
  if (failed.length || ctx.args.trace) console.error(`trace of ${tracked[0]} (screen y, transforms included):\n${trace(samples, tracked[0])}`);
  ctx.check(!failed.length, `topics in view moved more than 1px: ${failed.join("; ")}`);
  if (!failed.length) log("ok: topics in view held within 1px through both swaps and the late frame height");
  const floatingSlot = await page.evaluate(() => {
    const bar = document.querySelector('.board-toolbar'); return { floating: bar.hasAttribute('data-floating'), slot: bar.getBoundingClientRect().height, face: bar.firstElementChild.getBoundingClientRect().height };
  });
  qa(floatingSlot.floating && Math.abs(floatingSlot.slot - originalSlot) <= 1, 'late-height keep retains the original floating/wrapped toolbar slot', { originalSlot, ...floatingSlot });

  // 2. A user scroll during a session ends it, and the session writes no
  // more.
  await waitQuiet(page);
  const lay2 = await page.evaluate(() => window.__bh.layout());
  const above2 = lay2.filter((t) => t.where === "above");
  assert(above2.length >= 3, "too few topics above the viewport for the user-scroll part");
  const framedAbove2 = await page.evaluate((names) => names.filter((n) => window.__bh.section(n).querySelector('.tdoc iframe.doc-frame')), above2.map((t) => t.name));
  assert(framedAbove2.length >= 1, 'need a framed topic above for the post-user-scroll height probe');
  const up2 = ctx.args['framed-middle'] ? above2.find((n) => n.name !== lateGrow).name : above2[1].name;
  const grow2 = ctx.args['framed-middle'] ? lateGrow : framedAbove2.at(-1);
  assertChanged(await page.evaluate((n) => window.__bh.update([n]), up2), [up2]);
  const running = await page.evaluate(() => !!window.__bh.keeping());
  assert(running, "no keep session runs right after a board swap");
  const box = await page.evaluate(() => { const s = window.__bh.scroller(), t = window.__bh.viewTop(s), b = window.__bh.viewBottom(s); return { x: s === document.scrollingElement ? innerWidth / 2 : s.getBoundingClientRect().left + s.clientWidth / 2, y: (t + b) / 2 }; });
  await tab.mouse.move(box.x, box.y);
  await tab.mouse.wheel(0, 240);
  const ended = await page.waitForFunction(() => !window.__bh.keeping(), null, { timeout: 2000 }).then(() => true, () => false);
  ctx.check(ended, "the keep session still runs 2s after a user wheel scroll");
  // Let the wheel's own scroll finish before measuring.
  await page.waitForFunction(() => {
    const bh = window.__bh, st = bh.scroller().scrollTop;
    if (bh.lastSt === st) bh.stillN = (bh.stillN || 0) + 1; else { bh.lastSt = st; bh.stillN = 0; }
    return bh.stillN >= 10;
  }, null, { timeout: 3000, polling: "raf" });
  const probe = await page.evaluate(() => {
    const bh = window.__bh, s = bh.scroller(), t = bh.viewTop(s);
    const n = bh.topics().find((x) => x.getBoundingClientRect().bottom > t + 1);
    return { name: n.getAttribute("data-topic"), y: n.getBoundingClientRect().top, st: s.scrollTop, writes: bh.writesOn(s), anchorCss: getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor };
  });
  await growFrame(page, grow2, 250);
  await sleep(400);
  const after = await page.evaluate((name) => { const bh = window.__bh; return { y: bh.section(name).getBoundingClientRect().top, st: bh.scroller().scrollTop, writes: bh.writesOn(bh.scroller()), keep: !!bh.keeping() }; }, probe.name);
  log(`after the user scroll: keep session ${after.keep ? "running" : "ended"}; scrollTop writes from script ${probe.writes} -> ${after.writes}; ${probe.name}.y ${probe.y.toFixed(1)} -> ${after.y.toFixed(1)} after ${grow2} grew 250px above (overflow-anchor ${probe.anchorCss})`);
  const okUser = [
    ctx.check(!after.keep, "a keep session started again without a swap"),
    ctx.check(after.writes === probe.writes, `the scroller took ${after.writes - probe.writes} scrollTop writes from script after the user scroll ended the session`),
    // With native anchoring off on the scroller the growth above must
    // show: nothing compensates it any more, so the scroll offset stays
    // (the topic's own y also follows other frames above that re-report
    // their heights meanwhile, so it is logged, not asserted). Where
    // native anchoring is back (the document between sessions) the
    // browser itself may compensate it, so only the absence of script
    // writes is checked.
    probe.anchorCss !== "none" || ctx.check(Math.abs(after.st - probe.st) <= 1 && after.y - probe.y > 100, `the scroll moved ${probe.st} -> ${after.st} (${probe.name}.y ${probe.y.toFixed(1)} -> ${after.y.toFixed(1)}) after a 250px growth above with no session running`),
  ].every(Boolean);
  if (ended && okUser) log("ok: a user scroll ended the session and the session no longer writes");
  await page.evaluate(() => { __bh.scroller().scrollTop = 0; }); await waitQuiet(page);
  qa(await page.evaluate((height) => {
    const bar = document.querySelector('.board-toolbar'); return !bar.hasAttribute('data-floating') && Math.abs(bar.getBoundingClientRect().height - height) <= 1;
  }, originalSlot), 'return from late-height keep restores the original nonfloating wrapped slot');
}

async function scenarioObserver(ctx) {
  const { page, kind } = await openBoard(ctx);
  log(`page kind: ${kind}`);
  const lay = await scrollToMiddle(page);
  const inView = lay.filter((t) => t.where === "in");
  assert(inView.length >= 1, "no topic in view");
  const target = inView[0].name;
  // Subscribe and wait for the first report (in view).
  await page.evaluate((name) => {
    const bh = window.__bh;
    bh.outs = 0;
    bh.obsChanges = 0;
    bh.obsHandle = window.wbHistory.watch(bh.section(name), { onChange: () => { bh.obsChanges++; }, onOut: () => { bh.outs++; } });
  }, target);
  // seen() can still contain the dashboard's earlier report. Wait for OUR
  // subscription's first delivery before scrolling, or onOut is unarmed.
  await page.waitForFunction((name) => { const bh = window.__bh, r = window.wbHistory.seen(bh.section(name)); return bh.obsChanges > 0 && r && r.visible; }, target, { timeout: 2000 })
    .catch(() => { throw new Failure(`no in-view report for ${target} after watch()`); });
  // Scroll it fully out of view (past its bottom), then further.
  await page.evaluate((name) => {
    const bh = window.__bh, s = bh.scroller(), r = bh.section(name).getBoundingClientRect();
    s.scrollTop += r.bottom - bh.viewTop(s) + 40;
  }, target);
  await page.waitForFunction((name) => { const r = window.wbHistory.seen(window.__bh.section(name)); return r && r.ratio === 0; }, target, { timeout: 2000 })
    .catch(() => { throw new Failure(`${target} never reported ratio 0 after scrolling it out of view`); });
  await page.evaluate(() => { window.__bh.scroller().scrollTop += 300; });
  await sleep(300);
  const outs = await page.evaluate(() => { const bh = window.__bh; bh.obsHandle.unwatch(); return bh.outs; });
  log(`onOut fired ${outs} time(s) for ${target}`);
  assert(outs === 1, `onOut fired ${outs} times for one exit from view, expected 1`);
  log("ok: scrolling a topic fully out of view fired onOut once");

  // A topic a swap inserts is watched from that swap on: a new topic
  // (the server's next topic, made from a copy of one in view) lands at the
  // head of the board.
  await waitQuiet(page);
  const lay2 = await page.evaluate(() => window.__bh.layout());
  const from = (lay2.find((t) => t.where === "in") || lay2[0]).name, fresh = "BHNEW";
  const res = await page.evaluate(async ([from, name]) => {
    const bh = window.__bh;
    const r = await bh.update([], [{ name, from }]);
    const n = bh.section(name);
    return { r, present: !!n, watched: !!(n && n.wbWatch && n.wbWatch.node === n), head: bh.topics()[0] === n };
  }, [from, fresh]);
  assertChanged(res.r, [], [fresh]);
  assert(res.present && res.head, `the inserted ${fresh} section is not at the head of the board`);
  assert(res.watched, `the inserted ${fresh} section has no dashboard subscription (section.wbWatch)`);
  await page.waitForFunction((name) => window.wbHistory.seen(window.__bh.section(name)) !== null, fresh, { timeout: 2000 })
    .catch(() => { throw new Failure(`the inserted ${fresh} section has no visibility report after the swap`); });
  const first = await page.evaluate((name) => window.wbHistory.seen(window.__bh.section(name)).visible, fresh);
  await waitQuiet(page);
  // Bring it into view and expect a visible report.
  await page.evaluate(() => { window.__bh.scroller().scrollTop = 0; });
  await page.waitForFunction((name) => { const r = window.wbHistory.seen(window.__bh.section(name)); return r && r.visible; }, fresh, { timeout: 2000 })
    .catch(() => { throw new Failure(`the inserted ${fresh} section reported no visibility after being scrolled into view`); });
  log(`ok: the inserted ${fresh} section is watched (wbWatch; first report ${first ? "in view" : "out of view"}; in view once scrolled in)`);
}

// BANDS_PAGE's bandTopics reads, per open topic, where it stands against the view and
// its body frame: mounted (iframe), a placeholder with its height, or none
// for a bodyless log/history topic. shown excludes deliberately closed folds.
const BANDS_PAGE = () => {
  const bh = window.__bh;
  bh.bandTopics = () => {
    const s = bh.scroller(), top = bh.viewTop(s), bottom = bh.viewBottom(s), h = bottom - top;
    return bh.topics().map((n) => {
      const r = n.getBoundingClientRect(), f = n.querySelector(".tdoc .doc-frame");
      let shown = !!f;
      for (let p = f?.parentElement; p && p !== n; p = p.parentElement) if (p.tagName === 'DETAILS' && !p.open) shown = false;
      return { name: n.getAttribute("data-topic"), top: r.top - top, bottom: r.bottom - top, view: h, win: innerHeight,
        frame: f ? (f.tagName === "IFRAME" ? "iframe" : "placeholder") : "none", shown, near: n.wbNear ?? null, height: f ? f.style.height : "" };
    });
  };
};

async function mermaidDrawn(page, name) {
  const handle = await page.$(`section.topic[data-topic="${name}"] .tdoc iframe.doc-frame`);
  if (!handle) return { frame: null, drawn: false };
  const frame = await handle.contentFrame();
  const drawn = frame ? await frame.waitForFunction(() => !!document.querySelector("pre.mermaid svg"), null, { timeout: 8000 }).then(() => true, () => false) : false;
  return { frame: handle, drawn };
}

async function scenarioBands(ctx) {
  const { page, kind } = await openBoard(ctx);
  log(`page kind: ${kind}`);
  await page.evaluate(BANDS_PAGE);
  assert(await page.evaluate(() => !!(window.wbHistory && window.wbHistory.bands)), "this browser's view kit has no scroll margins (wbHistory.bands): the viewport bands are off");
  assert(await page.evaluate(() => !!window.__bh.section("MERMAID")), "no MERMAID topic on the board: run `serve --mermaid`");
  const M = "MERMAID";
  // 1. At load the bottom topic is far below: a placeholder never mounted.
  const load = await page.evaluate(() => window.__bh.bandTopics());
  const mLoad = load.find((t) => t.name === M);
  ctx.check(mLoad.frame === "placeholder", `at load the far MERMAID topic's body frame is a ${mLoad.frame}, expected a placeholder`);
  // History fixtures also contain bodyless topics (frame: "none"). They
  // are not lazy placeholders; nor should a deliberately closed body mount.
  const inViewPlaceholders = load.filter((t) => t.top < t.view && t.bottom > 0 && t.shown && t.frame === 'placeholder');
  ctx.check(!inViewPlaceholders.length, `at load shown body placeholders remain in view: ${JSON.stringify(inViewPlaceholders)}`);
  const anchoring = await page.evaluate(() => {
    const s = window.__bh.scroller();
    return getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor;
  });
  ctx.check(anchoring === 'none', `native anchoring is ${anchoring} on the ${kind} scroller; it would compete with topicResized`);
  // Topics never laid out (a placeholder with no height) are what grows
  // above the view on the way back up from the jump.
  const unsized = load.filter((t) => t.frame === "placeholder" && !t.height).length;
  assert(unsized >= 4, `only ${unsized} topic(s) were never laid out at load: run \`serve --mermaid --topics 30\``);

  // Select AND prepare a live placeholder in one browser task, before any
  // mount can consume its first height. A late message listener is not a
  // reliable hold: the dashboard's listener is already registered.
  const held = await page.evaluate((name) => {
    const bh = window.__bh, topics = bh.topics(), target = bh.section(name);
    const topic = topics[topics.indexOf(target) - 1], p = topic?.querySelector('.tdoc .doc-frame');
    if (!p || p.tagName !== 'DIV' || p.style.height) return { error: 'immediately preceding body must still be an unsized placeholder', topic: topic?.dataset.topic, tag: p?.tagName, height: p?.style.height };
    const source = p.getAttribute('data-srcdoc'), parsed = new DOMParser().parseFromString(source, 'text/html');
    const script = Array.from(parsed.querySelectorAll('script[src]')).find((s) => /(?:^|\/)wb-frame-[a-f0-9]+\.js$/.test(s.getAttribute('src')));
    if (!script || parsed.querySelector('pre')) return { error: 'need a plain body with the shared frame script', topic: topic.dataset.topic };
    // No synthetic height or MessageEvent: defer execution of the actual
    // reporting script, then start that same asset in the real child frame.
    script.type = 'application/x-board-headless-held';
    script.setAttribute('data-bh-band-script', '');
    p.setAttribute('data-srcdoc', '<!DOCTYPE html>\n' + parsed.documentElement.outerHTML);
    // MERMAID is last in the seeded board. Without tail room, a fragment
    // jump clamps near the bottom and the preceding topic stays IN view.
    const tail = document.createElement('div');
    tail.style.height = 3 * bh.scroller().clientHeight + 'px';
    tail.setAttribute('aria-hidden', 'true');
    bh.board_().appendChild(tail);
    bh.bandProbe = { topic: topic.dataset.topic, source, tail };
    return { topic: topic.dataset.topic };
  }, M);
  assert(!held.error, `cannot prepare first-height probe: ${JSON.stringify(held)}`);
  const heldTopic = held.topic;

  // 2. A jump to the panel under MERMAID lands and holds while the frames
  // around it mount and report their heights.
  const pid = await page.evaluate((n) => { const e = window.__bh.section(n).querySelector(".wb-entry[data-panel]"); return e && e.getAttribute("data-panel"); }, M);
  assert(pid, "the MERMAID topic has no panel to jump to");
  await page.evaluate((id) => { window.__bh.jumpTarget = id; location.hash = "#" + id; }, pid);
  await page.waitForFunction((id) => { const t = document.getElementById(id); return t && t.getBoundingClientRect().height > 0; }, pid, { timeout: 3000 })
    .catch(() => { throw new Failure(`the jump to ${pid} found no target`); });
  await sleep(100);
  const ys = [];
  const t0 = Date.now();
  while (Date.now() - t0 < 2000) {
    ys.push(await page.evaluate((id) => document.getElementById(id).getBoundingClientRect().top, pid));
    await sleep(50);
  }
  const drift = Math.max(...ys) - Math.min(...ys);
  ctx.check(drift <= 1, `the jump target ${pid} moved ${drift.toFixed(1)}px within 2s of landing (from ${ys[0].toFixed(1)})`);
  const land = await page.evaluate((id) => { const bh = window.__bh, s = bh.scroller(); return document.getElementById(id).getBoundingClientRect().top - bh.viewTop(s); }, pid);
  ctx.check(land > -2 && land < 900, `the jump target ${pid} stands ${land.toFixed(1)}px from the view's top`);
  const m1 = await mermaidDrawn(page, M);
  ctx.check(m1.frame && m1.drawn, "after the jump the MERMAID topic's frame did not mount with its diagram drawn");
  log(`jump to ${pid}: landed ${land.toFixed(1)}px from the view top, drift ${drift.toFixed(1)}px over 2s; mermaid ${m1.drawn ? "drawn" : "not drawn"}`);

  await page.waitForFunction((name) => window.__bh.section(name)?.querySelector('.tdoc iframe.doc-frame') && !window.__bh.keeping(), heldTopic, { timeout: 8000 });
  const heldHandle = await page.$(`section.topic[data-topic="${heldTopic}"] .tdoc iframe.doc-frame`);
  const heldFrame = await heldHandle.contentFrame();
  assert(heldFrame, 'held placeholder did not mount a child frame');
  await heldFrame.waitForLoadState('load');
  // Position explicitly, outside keep, with the held topic's bottom 80px
  // above the view. Tail space prevents clamping; verify it did not happen.
  await page.evaluate((name) => {
    const bh = window.__bh, s = bh.scroller();
    s.scrollTop += bh.section(name).getBoundingClientRect().bottom - bh.viewTop(s) + 80;
  }, heldTopic);
  await waitQuiet(page);
  const to = await heldFrame.evaluate(() => {
    if (!document.querySelector('script[data-bh-band-script]')) throw new Error('held reporting script is missing');
    return Math.ceil(document.querySelector('.doc-body').getBoundingClientRect().height);
  });
  const firstHeight = await page.evaluate(({ name, pid, to }) => {
    const bh = window.__bh, s = bh.scroller(), t = bh.section(name), f = t.querySelector('.tdoc iframe.doc-frame'), r = t.getBoundingClientRect();
    const before = { targetY: document.getElementById(pid).getBoundingClientRect().top, st: s.scrollTop,
      topicTop: r.top, topicBottom: r.bottom, viewTop: bh.viewTop(s), viewBottom: bh.viewBottom(s), above: r.bottom <= bh.viewTop(s),
      from: parseFloat(f.style.height) || 0, to, first: !!f.wbFirst, keeping: !!bh.keeping(), writes: bh.writesOn(s),
      anchorInline: (s === document.scrollingElement ? document.documentElement : s).style.overflowAnchor,
      anchoring: getComputedStyle(s === document.scrollingElement ? document.documentElement : s).overflowAnchor };
    bh.samplePostPaint = true;
    bh.startSampler([pid]);
    return before;
  }, { name: heldTopic, pid, to });
  assert(firstHeight.above && firstHeight.first && firstHeight.from === 0 && firstHeight.to > 0 && !firstHeight.keeping &&
    firstHeight.targetY >= firstHeight.viewTop && firstHeight.targetY < firstHeight.viewBottom,
    `delayed first-height probe must grow an above-view mount outside keep with a visible reference: ${JSON.stringify(firstHeight)}`);
  ctx.check(firstHeight.anchoring === 'none' && firstHeight.anchorInline === '',
    `probe must exercise stylesheet anchoring, not keep's inline override: ${JSON.stringify(firstHeight)}`);
  // The original doc-frame.js installs its native ResizeObserver and posts
  // its first real height. Dashboard applyFrameHeights/topicResized handle
  // it normally; no harness code writes frame height or scrollTop here.
  await heldFrame.evaluate(() => new Promise((resolve, reject) => {
    const held = document.querySelector('script[data-bh-band-script]'), live = document.createElement('script');
    live.src = held.getAttribute('src');
    live.onload = () => resolve();
    live.onerror = () => reject(new Error('held frame asset failed to load'));
    held.replaceWith(live);
  }));
  await sleep(800);
  const firstAfter = await page.evaluate(({ pid, name }) => {
    const bh = window.__bh, s = bh.scroller(), samples = bh.stopSampler().filter((v) => v.phase !== 'raf');
    const f = bh.section(name).querySelector('.tdoc iframe.doc-frame');
    bh.samplePostPaint = false;
    return { y: document.getElementById(pid).getBoundingClientRect().top, st: s.scrollTop, writes: bh.writesOn(s),
      height: parseFloat(f.style.height) || 0, first: !!f.wbFirst, keeping: !!bh.keeping(), samples };
  }, { pid, name: heldTopic });
  ctx.check(firstAfter.height === firstHeight.to && !firstAfter.first && !firstAfter.keeping,
    `real first height must land and consume wbFirst outside keep: height=${firstAfter.height}, first=${firstAfter.first}, keeping=${firstAfter.keeping}`);
  const delta = firstHeight.to - firstHeight.from;
  const firstDrift = Math.max(...firstAfter.samples.map((v) => Math.abs(v.y[pid] - firstHeight.targetY)));
  ctx.check(firstDrift <= 1 && Math.abs(firstAfter.y - firstHeight.targetY) <= 1,
    `above-view ${heldTopic} first height moved ${pid} by ${firstDrift.toFixed(1)}px post-paint`);
  ctx.check(Math.abs(firstAfter.st - firstHeight.st - delta) <= 1 && firstAfter.writes > firstHeight.writes,
    `above-view first height must compensate once: height +${delta}, scrollTop +${firstAfter.st - firstHeight.st}, script writes +${firstAfter.writes - firstHeight.writes}`);
  log(`above-view first height (${kind}): ${heldTopic} +${delta}px, scroll +${firstAfter.st - firstHeight.st}px, post-paint drift ${firstDrift.toFixed(1)}px; baseline ${JSON.stringify(firstHeight)}`);
  await page.evaluate((name) => {
    const bh = window.__bh, f = bh.section(name).querySelector('.tdoc iframe.doc-frame');
    // Restore the original payload for subsequent real unmount/remounts.
    f.setAttribute('data-srcdoc', bh.bandProbe.source);
    bh.bandProbe.tail.remove();
    delete bh.bandProbe;
  }, heldTopic);
  await waitQuiet(page);

  // 3. Up to the top a step at a time, then back down: each step keeps the
  // topic in view where the step put it, frames mount before they enter the
  // view, and frames far from the view are placeholders with their height.
  const steps = async (dir) => {
    const bad = [], late = [];
    let worst = 0;
    for (let i = 0; i < 80; i++) {
      const before = await page.evaluate(() => {
        const bh = window.__bh, s = bh.scroller(), top = bh.viewTop(s);
        const ref = bh.topics().find((n) => n.getBoundingClientRect().bottom > top + 40);
        return { st: s.scrollTop, max: s.scrollHeight - s.clientHeight, ref: ref.getAttribute("data-topic"), y: ref.getBoundingClientRect().top, h: s.clientHeight };
      });
      if (dir < 0 ? before.st <= 0 : before.st >= before.max - 1) break;
      const want = Math.round(before.h * 0.6) * dir;
      const moved = await page.evaluate((d) => { const s = window.__bh.scroller(), was = s.scrollTop; s.scrollTop = was + d; return s.scrollTop - was; }, want);
      await sleep(250);
      const after = await page.evaluate((ref) => window.__bh.section(ref).getBoundingClientRect().top, before.ref);
      const off = after - (before.y - moved);
      worst = Math.max(worst, Math.abs(off));
      if (Math.abs(off) > 1) bad.push(`${before.ref} ${off.toFixed(1)}px at scrollTop ${before.st}`);
      const lay = await page.evaluate(() => window.__bh.bandTopics());
      lay.filter((t) => t.top < t.view && t.bottom > 0 && t.shown && t.frame === "placeholder").forEach((t) => late.push(`${t.name} at scrollTop ${before.st + moved}`));
    }
    return { bad, late, worst };
  };
  const up = await steps(-1);
  ctx.check(!up.bad.length, `scrolling up moved the topic in view off its step: ${up.bad.join("; ")}`);
  ctx.check(!up.late.length, `scrolling up showed placeholders in view: ${up.late.join("; ")}`);
  const top = await page.evaluate(() => window.__bh.bandTopics());
  // The far band reaches four window heights past the view's bottom.
  const far = top.filter((t) => t.frame !== 'none' && t.top > t.view + 4 * t.win + 50);
  const mTop = top.find((t) => t.name === M);
  ctx.check(far.length > 0 && far.every((t) => t.frame === "placeholder"), `at the top a topic beyond the far band holds a mounted frame: ${far.filter((t) => t.frame !== "placeholder").map((t) => t.name).join(", ")}`);
  ctx.check(far.every((t) => /px$/.test(t.height) && parseFloat(t.height) > 0), `a far placeholder lost its height: ${far.filter((t) => !/px$/.test(t.height)).map((t) => t.name).join(", ")}`);
  ctx.check(mTop.frame === "placeholder" && parseFloat(mTop.height) > 0, `at the top the MERMAID topic holds ${mTop.frame} (height ${mTop.height || "none"}), expected a placeholder with its height`);
  log(`up: worst step offset ${up.worst.toFixed(2)}px; at the top ${far.length} topic(s) beyond the far band, all placeholders with heights`);
  const down = await steps(1);
  ctx.check(!down.bad.length, `scrolling down moved the topic in view off its step: ${down.bad.join("; ")}`);
  ctx.check(!down.late.length, `scrolling down showed placeholders in view: ${down.late.join("; ")}`);
  const m2 = await mermaidDrawn(page, M);
  ctx.check(m2.frame && m2.drawn, "back at the bottom the MERMAID topic's frame did not remount with its diagram drawn");
  const fresh = m1.frame && m2.frame ? await page.evaluate(([a, b]) => a !== b && !a.isConnected, [m1.frame, m2.frame]) : false;
  ctx.check(fresh, "the MERMAID topic's frame at the bottom is not a new one: it never unmounted");
  log(`down: worst step offset ${down.worst.toFixed(2)}px; MERMAID remounted (${fresh ? "new frame" : "same frame"}), diagram ${m2.drawn ? "drawn" : "not drawn"}`);
}

async function scenarioPill(ctx) {
  const { page, kind } = await openBoard(ctx);
  const expected = +(ctx.args.pending || 11);
  assert(Number.isInteger(expected) && expected > 0, "--pending must be a positive integer");
  assert(await page.evaluate(() => !!window.wbHistory?.bands), "viewport bands must be active for the churn regression");
  const inventory = await page.evaluate(() => {
    const items = Array.from(document.querySelectorAll('.card.policy[id^="item-"], .arow[id^="item-"], .ckrow[data-id]:not([data-assumed])'));
    const topics = [...new Set(items.map((el) => el.closest('section.topic')))];
    return { count: items.length, topic: topics.length === 1 ? topics[0]?.dataset.topic : null };
  });
  assert(inventory.count === expected && inventory.topic,
    `expected ${expected} pending controls in one topic, got ${JSON.stringify(inventory)}; use serve --topics 30 --pending ${expected} --mermaid`);

  await page.evaluate(() => {
    const bh = window.__bh;
    bh.pillChurn = { mounts: 0, unmounts: 0 };
    const frames = (n) => n.nodeType !== 1 ? 0 : (n.matches('iframe.doc-frame') ? 1 : 0) + n.querySelectorAll('iframe.doc-frame').length;
    bh.pillMO = new MutationObserver((records) => {
      for (const r of records) {
        for (const n of r.addedNodes) bh.pillChurn.mounts += frames(n);
        for (const n of r.removedNodes) bh.pillChurn.unmounts += frames(n);
      }
    });
    bh.pillMO.observe(bh.board_(), { childList: true, subtree: true });
  });
  try {
    for (const views of [2, 5]) {
      await page.evaluate(({ name, views }) => {
        const bh = window.__bh, s = bh.scroller();
        s.scrollTop += bh.section(name).getBoundingClientRect().bottom - bh.viewTop(s) + views * s.clientHeight;
      }, { name: inventory.topic, views });
      // Fixed upper bound, not an unbounded wait-until-quiet that could hide a
      // perpetual mount loop. No input, scroll, or data mutations follow.
      await sleep(3000);
      const result = await page.evaluate(async ({ name, expected }) => {
        const bh = window.__bh, s = bh.scroller(), host = document.getElementById('bcol') || document.querySelector('.wrap');
        const pill = host.querySelector('.oq-wrap.wb-oq-top .oq-pill'), seg = pill?.querySelector('.oq-seg.open');
        const count = () => !pill || pill.hidden || !seg || seg.hidden ? 0 : Number(seg.querySelector('.n').textContent);
        bh.pillMO.takeRecords();
        bh.pillChurn = { mounts: 0, unmounts: 0 };
        const start = performance.now(), st = s.scrollTop, counts = new Set(), failures = [];
        let samples = 0, drift = 0;
        await new Promise((resolve) => {
          function sample() {
            samples++;
            counts.add(count());
            drift = Math.max(drift, Math.abs(s.scrollTop - st));
            if (bh.section(name).getBoundingClientRect().bottom > bh.viewTop(s)) failures.push('pending topic entered view');
            if (performance.now() - start < 4000) requestAnimationFrame(sample); else resolve();
          }
          sample();
        });
        const frames = Array.from(bh.board_().querySelectorAll('.doc-frame'));
        return { expected, samples, counts: [...counts], drift, failures: [...new Set(failures)], ...bh.pillChurn,
          mounted: frames.filter((f) => f.tagName === 'IFRAME').length,
          placeholders: frames.filter((f) => f.tagName === 'DIV').length };
      }, { name: inventory.topic, expected });
      log(`pill ${kind}, ${views} viewports past pending topic: ${JSON.stringify(result)}`);
      ctx.check(result.samples > 30 && result.counts.length === 1 && result.counts[0] === expected,
        `static pending count must stay ${expected}: ${JSON.stringify(result)}`);
      ctx.check(result.mounts === 0 && result.unmounts === 0, `static viewport churned frames: ${JSON.stringify(result)}`);
      ctx.check(result.drift <= 1 && !result.failures.length, `static viewport moved: ${JSON.stringify(result)}`);
      ctx.check(result.mounted > 0 && result.placeholders > 0, 'regression must exercise mounted frames and lazy placeholders together');
    }
  } finally {
    await page.evaluate(() => window.__bh.pillMO.disconnect());
  }
}

export { scenarioKeep, scenarioObserver, scenarioBands, scenarioPill };
