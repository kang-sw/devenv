// INIT runs in every document before its scripts. In the page document (the
// top page, or the session page the iframe shell frames, ?frame=1) it wraps
// WebSocket so the harness reaches the page's stream socket, records the
// board region events the server sends, and spies on scrollTop writes from
// script (a keep session, restoreScroll) per element.
const INIT = (options = {}) => {
  // The shell's session frame is a child of the top page. Chromium may run
  // this only in the frame's initial about:blank, whose window the
  // same-origin page then reuses, so the frame's URL is not checked here.
  // The body frames are grandchildren and are skipped.
  if (window.top !== window && window.parent !== window.top) return;
  const bh = (window.__bh = { sockets: [], board: null, writes: [], resizeDeliveries: [], historyLoads: [], observerErrors: [], observerErrorDetails: [], menuHitTargets: [], menuHitTargetsEnabled: !!options.menuHitTargets });
  // Observe actual native deliveries; never invoke a callback or re-observe
  // on the harness's behalf. Filter to the history's own observer, excluding
  // the shared keep observer which also watches history-live.
  window.addEventListener('error', (e) => {
    if (!/ResizeObserver.*loop/i.test(e.message || '')) return;
    bh.observerErrors.push(e.message);
    bh.observerErrorDetails.push({ message: e.message, t: performance.now(), phase: bh.regressionPhase || 'startup',
      lastHistoryDelivery: bh.resizeDeliveries.at(-1)?.id || null });
  });
  const NativeResize = window.ResizeObserver;
  window.ResizeObserver = class extends NativeResize {
    constructor(callback) {
      super((entries, observer) => {
        const targets = entries.filter((e) => e.target.matches?.('.history-live') &&
          e.target.closest('.wb-history')?._wbHistoryObserver === observer);
        const snapshot = (live) => {
          const h = live.closest('.wb-history');
          return { name: h.dataset.history, pages: h.querySelectorAll('template[data-history-page]').length,
            rows: live.querySelectorAll('.wb-entry').length, height: live.scrollHeight, steps: h._wbHistorySteps };
        };
        const record = targets.length ? { id: bh.resizeDeliveries.length + 1, t: performance.now(),
          before: targets.map((e) => snapshot(e.target)) } : null;
        const previous = bh.activeResize;
        if (record) { bh.resizeDeliveries.push(record); bh.activeResize = record.id; }
        try { callback.call(observer, entries, observer); }
        finally {
          if (record) record.after = targets.map((e) => snapshot(e.target));
          bh.activeResize = previous;
        }
      });
    }
  };
  const Native = window.WebSocket;
  class Captured extends Native {
    constructor(...a) {
      super(...a);
      bh.sockets.push(this);
      this.addEventListener("message", (e) => {
        try {
          const m = JSON.parse(e.data);
          if (m && m.event === "region" && m.data && m.data.name === "board") bh.board = { hash: m.data.hash, html: m.data.html };
        } catch {}
      });
    }
  }
  window.WebSocket = Captured;
  const d = Object.getOwnPropertyDescriptor(Element.prototype, "scrollTop");
  Object.defineProperty(Element.prototype, "scrollTop", {
    configurable: true,
    get() { return d.get.call(this); },
    set(v) { bh.writes.push({ el: this, t: performance.now() }); d.set.call(this, v); },
  });
};

// IN_PAGE installs the in-page half of the shared helpers on window.__bh.
const IN_PAGE = () => {
  const bh = window.__bh;
  // The shell can reuse its child's initial about:blank window with a NEW
  // Document. Bind document events here, not in that initial document's INIT.
  if (bh.historyLoadDocument !== document) {
    bh.historyLoadDocument = document;
    document.addEventListener('wbhistoryload', (e) => {
      const h = e.detail?.root;
      if (h?.matches('.wb-history')) bh.historyLoads.push({ t: performance.now(), name: h.dataset.history,
        delivery: bh.activeResize || null, pages: h.querySelectorAll('template[data-history-page]').length,
        rows: h.querySelectorAll('.history-live .wb-entry').length });
    });
  }
  if (bh.menuHitTargetsEnabled && bh.menuHitDocument !== document) {
    bh.menuHitDocument = document;
    // Capture before the native menu handler removes the button, once for
    // this actual Document rather than the shell's initial about:blank.
    document.addEventListener('click', (e) => {
      const button = e.target?.closest?.('.lmenu button');
      if (!button) return;
      const local = document.elementFromPoint(e.clientX, e.clientY);
      const hit = { trusted: e.isTrusted, first: button.matches('.lmenu button:first-child'), text: button.textContent,
        point: { x: e.clientX, y: e.clientY }, localHitIsButton: local === button, framed: window.parent !== window, parentHitIsFrame: null };
      if (hit.framed) {
        try {
          const frame = window.frameElement, r = frame.getBoundingClientRect();
          hit.parentHitIsFrame = window.parent.document.elementFromPoint(r.left + frame.clientLeft + e.clientX, r.top + frame.clientTop + e.clientY) === frame;
        } catch (error) { hit.parentError = String(error); }
      }
      bh.menuHitTargets.push(hit);
      console.debug('__bh-menu-hit-target__' + JSON.stringify(hit));
    }, true);
  }
  const q = (sel, root) => Array.from((root || document).querySelectorAll(sel));
  bh.board_ = () => document.getElementById("r-board") || document.querySelector("body.full .grid > main");
  bh.kind = () => (document.body.classList.contains("full") ? "standalone" : document.getElementById("tabbar") ? "adapter" : "board");
  bh.scroller = () => {
    const pins = bh.focus && bh.focus.closest("#bpins");
    const c = pins && ['auto', 'scroll'].includes(getComputedStyle(pins).overflowY) ? pins : document.getElementById("bcol");
    const oy = c && getComputedStyle(c).overflowY;
    return c && (oy === "auto" || oy === "scroll") && c.scrollHeight > c.clientHeight ? c : document.scrollingElement;
  };
  bh.viewTop = (s) => (s === document.scrollingElement ? 0 : Math.max(0, s.getBoundingClientRect().top + s.clientTop));
  bh.viewBottom = (s) => (s === document.scrollingElement ? innerHeight : Math.min(innerHeight, s.getBoundingClientRect().top + s.clientTop + s.clientHeight));
  bh.topics = () => q("section.topic[data-topic]", bh.board_()).filter((n) => !n.classList.contains("closed"));
  bh.section = (name) => q("section.topic[data-topic]").find((n) => n.getAttribute("data-topic") === name) || null;
  // Topics by where they stand against the scroller's viewport.
  bh.layout = () => {
    const s = bh.scroller(), top = bh.viewTop(s), bottom = bh.viewBottom(s);
    return bh.topics().map((n) => {
      const r = n.getBoundingClientRect();
      return { name: n.getAttribute("data-topic"), top: r.top, bottom: r.bottom, where: r.bottom <= top ? "above" : r.top >= bottom ? "below" : "in" };
    });
  };
  bh.keeping = () => (window.wbHistory && window.wbHistory.keeping ? window.wbHistory.keeping(bh.scroller()) : null);
  bh.writesOn = (s) => bh.writes.filter((w) => w.el === s).length;
  bh.frames = () => q("iframe.doc-frame", bh.board_());
  // baseBoard is the board HTML the page last received, with its hash.
  bh.baseBoard = async () => {
    const live = bh.board_().getAttribute("data-h");
    if (bh.board && bh.board.hash === live) return bh.board;
    const r = await fetch(location.href, { cache: "no-store", credentials: "same-origin" });
    const page = new DOMParser().parseFromString(await r.text(), "text/html");
    const b = page.getElementById("r-board");
    if (!b || b.getAttribute("data-h") !== live) throw new Error(`the page's board (hash ${live}) is not the served one (${b ? b.getAttribute("data-h") : "none"}); reload the page`);
    return { hash: live, html: b.innerHTML };
  };
  // update builds the next board as the server would order it and
  // delivers it through the stream socket's own onmessage. Each topic in
  // names gets its last-change time set to now (so its section differs from
  // the HTML the page last saw) and moves to the head of the open topics,
  // the last named first. Each name in add is a new topic, made from a copy
  // of topic `from` under the new name (its section, keys and body frame
  // document renamed), put at the head. It reports which topics the morph
  // treated as changed: a topic whose section node, or whose body frame
  // element, is not the one it had, or that loads a next frame beside the
  // old one (frameGraft keeps a changed topic's section and loads its new
  // frame beside the old one).
  bh.update = async (names, add, options) => {
    const base = await bh.baseBoard();
    const t = document.createElement("template");
    t.innerHTML = base.html;
    const box = t.content;
    const sectionIn = (name) => Array.from(box.children).find((n) => n.matches && n.matches("section.topic") && n.getAttribute("data-topic") === name);
    const head = () => Array.from(box.children).find((n) => n.matches && n.matches("section.topic[data-topic]:not(.closed)"));
    const stamp = (sec, at) => {
      const time = sec.querySelector(".tbar time.tago[data-at]");
      if (time) { time.setAttribute("data-at", String(at)); time.setAttribute("datetime", new Date(at).toISOString().replace(/\.\d+Z$/, "Z")); }
      else sec.setAttribute("data-bh-at", String(at));
      // The morph compares a topic's server hash (data-th) when it has one,
      // so a changed section needs a new hash, as the server would give it.
      if (sec.hasAttribute("data-th")) sec.setAttribute("data-th", "bh" + at.toString(16));
    };
    let now = Date.now();
    for (const name of names || []) {
      const sec = sectionIn(name);
      if (!sec) throw new Error(`no topic ${name} in the board`);
      if (sec.classList.contains("closed")) throw new Error(`topic ${name} is closed`);
      stamp(sec, ++now);
      if (head() !== sec) box.insertBefore(sec, head());
    }
    for (const a of add || []) {
      const src = sectionIn(a.from);
      if (!src) throw new Error(`no topic ${a.from} in the board`);
      if (sectionIn(a.name)) throw new Error(`topic ${a.name} is already on the board`);
      // Rename every occurrence of the source topic's keys (data-topic,
      // data-key, data-doc, the frame document's wb-doc-key meta) and its
      // shown name.
      const esc = (x) => x.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const html = src.outerHTML.replace(new RegExp(`(["':])${esc(a.from)}(?=["'&])`, "g"), `$1${a.name}`).replace(new RegExp(`>${esc(a.from)}<`, "g"), `>${a.name}<`);
      const tt = document.createElement("template");
      tt.innerHTML = html;
      const sec = tt.content.firstElementChild;
      stamp(sec, ++now);
      box.insertBefore(sec, head());
    }
    const html = t.innerHTML, hash = "bh" + now.toString(16);
    const ws = bh.sockets.find((s) => s.readyState === 1 && typeof s.onmessage === "function");
    if (!ws) throw new Error("no open stream socket on the page");
    // Frames of either kind: a topic far from the view holds placeholders
    // (the viewport bands), and a changed one gets new placeholders.
    const look = (n) => ({ n, frames: q(".doc-frame:not(.doc-frame-next)", n) });
    const before = new Map(q("section.topic[data-topic]", bh.board_()).map((n) => [n.getAttribute("data-topic"), look(n)]));
    bh.mark("swap: " + (names || []).concat((add || []).map((a) => "+" + a.name)).join(", "));
    ws.onmessage({ data: JSON.stringify({ event: "region", data: { name: "board", hash, html } }) });
    bh.board = { hash, html };
    if (options && options.held) {
      if (bh.board_().getAttribute("data-h") === hash) throw new Error("the overview-held swap landed early");
      return { hash, html };
    }
    if (bh.board_().getAttribute("data-h") !== hash) throw new Error("the board swap did not land (held: a hidden tab, an edit, or a drag?)");
    const changed = [], added = [];
    q("section.topic[data-topic]", bh.board_()).forEach((n) => {
      const k = n.getAttribute("data-topic"), was = before.get(k), now = look(n);
      if (!was) { added.push(k); return; }
      const same = was.n === n && was.frames.length === now.frames.length && was.frames.every((f, i) => f === now.frames[i]) && !n.querySelector("iframe.doc-frame-next");
      if (!same) changed.push(k);
    });
    const order = bh.topics().map((n) => n.getAttribute("data-topic"));
    return { changed, added, order };
  };
  // The sampler records, per animation frame, each tracked topic's screen
  // top (getBoundingClientRect: what the user sees, transforms included)
  // and its own running translateY, the scroller's scrollTop, and whether a
  // keep session runs.
  bh.startSampler = (names) => {
    const s = bh.scroller(), t0 = performance.now(), samples = [];
    const state = bh.sampling = { samples, on: true, t0 };
    const take = (phase) => {
      const y = {}, ty = {};
      for (const name of names) {
        const n = bh.section(name) || document.getElementById(name);
        y[name] = n ? n.getBoundingClientRect().top : null;
        const m = n && /^matrix(3d)?\((.*)\)$/.exec(getComputedStyle(n).transform || "");
        ty[name] = m ? +m[2].split(",")[m[1] ? 13 : 5] : 0;
      }
      const h = bh.history && bh.history();
      samples.push({ t: performance.now() - t0, phase, st: s.scrollTop, keep: !!bh.keeping(), y, ty,
        history: h ? { steps: h._wbHistorySteps, target: h._wbHistoryTarget?.id || null } : undefined });
    };
    take('baseline');
    const postPaint = !!bh.samplePostPaint;
    const tick = () => {
      if (!state.on) return;
      take('raf');
      // rAF precedes ResizeObserver delivery. A task queued here runs after
      // that rendering update, including RO's reveal/keep corrections. Keep
      // both phases as evidence, but history asserts the post-paint geometry.
      if (postPaint) setTimeout(() => { if (state.on) take('post-paint'); }, 0);
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  };
  bh.mark = (label) => { if (bh.sampling && bh.sampling.on) bh.sampling.samples.push({ t: performance.now() - bh.sampling.t0, mark: label }); };
  bh.stopSampler = () => { bh.sampling.on = false; return bh.sampling.samples; };
};

export { INIT, IN_PAGE };
