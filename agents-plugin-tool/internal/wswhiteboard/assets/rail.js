;(function () {
  // Topic rail. The topic being read keeps its title bar stuck at the top of
  // the board's scroll area (position: sticky, under .wb-rail), and its
  // neighbours show as jump capsules: up to two previous topics stacked above
  // the stuck bar, and up to two of the next topics not yet in view at the
  // bottom edge. A click scrolls that topic's bar to where it sticks. A host
  // can rail other board nodes beside the topics (items, below). The
  // dashboard re-runs this after every board swap (wbForm), so it tears down
  // what the previous run attached; the two capsule wrappers sit outside the
  // board region and are reused.
  var d = document;
  if (d.body.classList.contains("embed") || !window.requestAnimationFrame) return;
  // host carries the rail class (the dashboard's board column, the full
  // page's wrapper, the same element pill.js puts its wrappers in); box
  // holds the topics and the wrappers. The rail's offsets go on the
  // elements that read them, not on host (offsets, below).
  var host = d.getElementById("bcol"), box = host;
  if (!host) { host = d.querySelector("body.full .wrap"); box = host && host.querySelector("main"); }
  if (!box) return;
  var st = window.wbRail || (window.wbRail = {});
  if (st.off) st.off();
  host.classList.add("wb-rail");

  // Capsule geometry, mirrored by the .wb-rail-stack and .wb-cap rules in
  // page.css: a capsule is CAP tall, capsules are GAP apart, and the top
  // stack pads PAD above and GAP below. capH(n) is the top stack's height
  // with n capsules, which is where the current topic's bar sticks.
  var CAP = 17, GAP = 3, PAD = 4, FADE = 14;
  function capH(n) { return n ? PAD + n * (CAP + GAP) : 0; }
  // The bottom stack's height with both capsules, fade included: a fixed
  // reserve, so the line below does not move with the capsule count. The
  // bottom wrapper's top margin (page.css), the board's bottom scroll
  // margin, is built from it.
  var BOT = FADE + 2 * CAP + GAP + PAD;

  // The edge classes are up and dn, not top and bot: a bare .top is the
  // page header's class on both pages, whose flex and margin rules would
  // give the zero-height wrapper a flow gap.
  function wrapper(edge) {
    var cls = edge === "top" ? "up" : "dn";
    var w = box.querySelector(":scope > .wb-rail-wrap." + cls);
    if (!w) {
      w = d.createElement("div");
      w.className = "wb-rail-wrap " + cls;
      var s = d.createElement("div");
      s.className = "wb-rail-stack";
      s.hidden = true;
      // Top: the farther capsule first, the nearer one right above the bar.
      // Bottom: the nearer one first, the farther one at the very edge.
      (edge === "top" ? ["far", "near"] : ["near", "far"]).forEach(function (k) {
        var b = d.createElement("button");
        b.type = "button";
        b.className = "wb-cap " + k;
        b.hidden = true;
        b.innerHTML = '<span class="ci" hidden></span><span class="cn"></span><span class="cp" hidden></span><span class="cx" hidden></span>';
        s.appendChild(b);
      });
      w.appendChild(s);
      if (edge === "top") box.insertBefore(w, box.firstChild); else box.appendChild(w);
    }
    return w;
  }
  var wraps = { top: wrapper("top"), bot: wrapper("bot") };
  function slot(edge, k) { return wraps[edge].querySelector(".wb-cap." + k); }
  var slots = { top: { near: slot("top", "near"), far: slot("top", "far") }, bot: { near: slot("bot", "near"), far: slot("bot", "far") } };

  // The box scrolls on its own in the dashboard's board column; otherwise
  // (the board-first page, the full page) the window scrolls.
  var root = /auto|scroll/.test(getComputedStyle(box).overflowY) ? box : null;
  var scroller = root || window;
  // A sticky inset counts from the scroll container's padding edge.
  var padTop = root ? parseFloat(getComputedStyle(root).paddingTop) || 0 : 0;

  // inset is the room the host keeps at the top of its scroll area for a
  // sticky row of its own (--wb-rail-inset on the host, in px; the
  // dashboard's board header row): the top capsules and the current bar
  // stick below it. It is read on every run, since the row can come after
  // the rail.
  var topics = [], found = [], dirty = true, fresh = false, last = "", moved = null, stuck = null, raf = 0, inset = 0;
  function readInset() { inset = root ? parseFloat(getComputedStyle(host).getPropertyValue("--wb-rail-inset")) || 0 : 0; }
  // scrollRest is how far the scroller can still scroll down, in px.
  function scrollRest() {
    var e = root || d.scrollingElement || d.documentElement;
    return Math.max(0, e.scrollHeight - e.clientHeight - e.scrollTop);
  }
  function viewTop() { return root ? root.getBoundingClientRect().top + padTop + inset : 0; }

  // The rail's items are the board's topics and any other node the host
  // marks with data-rail (its key, unique among the marked nodes), in board
  // order; an item with no height (hidden) is left out. A marked node's
  // attributes name its bar (data-rail-bar: the header that sticks while
  // it is current, as a topic's head does under the host's own rule), its
  // capsule's text (data-rail-name), and the icon its capsule leads with
  // (data-rail-icon, copied). A topic's bar (.tbar, what a capsule morphs
  // from and into) sits in its head (.tophead: the bar, the note, and the
  // summary line), and the head is what sticks (stickOf); a marked node's
  // bar is both.
  var ITEMS = "section.topic[data-topic], [data-rail]";
  function isTopic(t) { return t.hasAttribute("data-topic"); }
  function barOf(t) { return t.querySelector(isTopic(t) ? ":scope > .tophead > .tbar" : "[data-rail-bar]"); }
  function stickOf(t) { return t.querySelector(isTopic(t) ? ":scope > .tophead" : "[data-rail-bar]"); }
  function keyOf(t) { return isTopic(t) ? t.getAttribute("data-topic") : "\u0001" + t.getAttribute("data-rail"); }

  function fill(b, t, extra) {
    b.hidden = !t;
    b.wbTopic = t || null;
    if (!t) return;
    var name, n = "", icon = "";
    if (isTopic(t)) {
      // The rail shows the topic's visible name (.tname, its label), not
      // data-topic, which carries the key a stemmed topic is addressed by.
      var tn = t.querySelector(":scope > .tophead > .tbar .tname"), pol = t.querySelector(":scope > .tophead > .tbar > .chip.policy");
      name = tn ? tn.textContent : t.getAttribute("data-topic");
      n = pol ? pol.textContent.trim() : "";
    } else {
      var nm = t.querySelector("[data-rail-name]"), ic = t.querySelector("[data-rail-icon]");
      name = (nm && nm.textContent) || t.getAttribute("data-rail");
      icon = ic ? ic.innerHTML : "";
    }
    var ci = b.querySelector(":scope > .ci"), cp = b.querySelector(":scope > .cp"), cx = b.querySelector(":scope > .cx");
    b.classList.toggle("quiet", t.classList.contains("quiet"));
    ci.innerHTML = icon;
    ci.hidden = !icon;
    b.querySelector(":scope > .cn").textContent = name;
    b.title = name;
    b.setAttribute("aria-label", name);
    cp.hidden = !n || n === "0";
    cp.textContent = n;
    cx.hidden = !extra;
    cx.textContent = extra ? "+" + extra : "";
  }

  function run() {
    raf = 0;
    readInset();
    if (dirty) { found = Array.prototype.slice.call(box.querySelectorAll(ITEMS)); dirty = false; fresh = true; }
    var rects = [];
    topics = [];
    found.forEach(function (t) { var r = t.getBoundingClientRect(); if (r.height) { topics.push(t); rects.push(r); } });
    var n = topics.length;
    // Nothing rendered (the board tab is hidden) or nothing to rail.
    if (!n) return draw(null);
    var vt = viewTop(), vb = root ? root.getBoundingClientRect().bottom : window.innerHeight;
    // i is the last topic whose top has reached where its bar would stick;
    // it is current while its card still spans that line, and otherwise
    // (the gap after it, or past the last topic) every topic up to it is
    // above.
    var i = -1;
    for (var k = 0; k < n && rects[k].top <= vt + capH(Math.min(2, k)) + 1; k++) i = k;
    var cur = i >= 0 && rects[i].bottom > vt + capH(Math.min(2, i)) + 1 ? i : -1;
    var above = cur >= 0 ? cur : i + 1, nTop = Math.min(2, above), top = capH(nTop);
    // Bottom: the next topics whose bar's top edge is at or below the
    // bottom stack's top edge (over the reply bar, when it shows), i.e.
    // hidden behind the stack or further down. A bar that shows above the
    // stack gets no capsule, so the stack may hold fewer than two. Nor does
    // a bar in view that the rest of the scroll cannot lift above the line
    // (a short last item at the end of the board): it is as high as it will
    // ever go, and a capsule would only cover it.
    var reply = box.querySelector(".reply"), replyOff = reply && reply.offsetHeight ? reply.offsetHeight + 14 : 0;
    var line = vb - replyOff - BOT, below = [], rest = scrollRest();
    for (k = above + (cur >= 0 ? 1 : 0); k < n; k++) if (rects[k].top >= line && !(rects[k].top < vb && rects[k].top - rest >= line)) below.push(k);
    draw({
      top: top,
      cur: cur,
      // The current head sits at the stick line; it is stuck (and casts its
      // shadow) once its card's top has passed that line.
      bar: cur >= 0 ? stickOf(topics[cur]) : null,
      stuck: cur >= 0 && rects[cur].top < vt + top - 0.5,
      up: [above - 1, above - 2].filter(function (x) { return x >= 0; }),
      upMore: Math.max(0, above - 2),
      down: below.slice(0, 2),
      downMore: Math.max(0, below.length - 2),
      replyOff: replyOff
    });
  }

  function draw(s) {
    var key = s ? [inset, s.top, s.upMore, s.downMore, s.replyOff, s.stuck, names(s.up), names(s.down), names([s.cur])].join("|") : "" + inset;
    // A re-queried board can hold heads that never got the offset, and the
    // current head may have changed height (a note opened, a summary line
    // wrapping), which resizes its topic and so re-queries.
    if (key === last) {
      if (fresh) { if (s && s.bar) vals.h = inset + s.top + s.bar.offsetHeight + "px"; offsets(); }
      return;
    }
    last = key;
    // The motion's reads are skipped when no capsule can move: the same
    // items in the same slots (a change of the stuck shadow or a "+N"
    // count alone), and on a run that re-queried the board (fresh: a size
    // changed), whose items moved under the rail rather than by a scroll.
    var member = s ? [inset, s.replyOff, names(s.up), names(s.down), names([s.cur])].join("|") : "";
    var pre = motion() && !fresh && member !== moved ? before(s) : null;
    moved = member;
    var bar = s && s.bar, shadow = s && s.stuck ? bar : null;
    if (stuck !== shadow) {
      if (lifted && lifted.head !== shadow) drop();
      if (stuck) stuck.classList.remove("wb-stuck");
      if (shadow) shadow.classList.add("wb-stuck");
      stuck = shadow;
    }
    // The head's height is read before the capsule writes below, so the
    // read restyles only what the shadow toggle touched.
    var barH = bar ? bar.offsetHeight : 0;
    var up = s ? s.up : [], down = s ? s.down : [];
    fill(slots.top.near, topics[up[0]], 0);
    fill(slots.top.far, topics[up[1]], s ? s.upMore : 0);
    fill(slots.bot.near, topics[down[0]], 0);
    fill(slots.bot.far, topics[down[1]], s ? s.downMore : 0);
    wraps.top.firstChild.hidden = !up.length;
    wraps.bot.firstChild.hidden = !down.length;
    // The bottom stack stays at the edge and pads its capsules up over the
    // reply bar (which draws above it), so its fill also covers the gaps
    // around the bar and the capsules read as sitting on it, not floating
    // over the board. --wb-rail-bot is the capsule part alone, computed
    // from the stack's geometry rather than read back after the writes:
    // pill.js adds the reply bar's own clearance.
    var replyOff = s ? s.replyOff : 0;
    wraps.bot.firstChild.style.paddingBottom = replyOff ? replyOff + PAD + "px" : "";
    vals.top = inset + (s ? s.top : 0) + "px";
    vals.h = inset + (s ? s.top + barH : 0) + "px";
    vals.bot = (down.length ? FADE + down.length * CAP + (down.length - 1) * GAP + PAD : 0) + "px";
    offsets();
    curTopic = s && s.cur >= 0 ? topics[s.cur] : null;
    if (pre) after(pre);
    drawn = !!s;
  }

  // offsets puts the rail's offsets on the elements that read them: every
  // item's sticking head gets --wb-rail-top (where it sticks, under the
  // host's own sticky rule), the open-question pill's top wrapper
  // --wb-rail-h (the stack and the current head), and its bottom wrapper --wb-rail-bot
  // (the bottom capsules). Never on host: a custom property there is
  // inherited by every node of the board, so each write restyled the whole
  // board. A value an element already holds is not written again.
  var vals = { top: "0px", h: "0px", bot: "0px" };
  function put(el, name, v) {
    if (!el) return;
    var had = el.wbRail || (el.wbRail = {});
    if (had[name] === v) return;
    had[name] = v;
    el.style.setProperty(name, v);
  }
  function offsets() {
    fresh = false;
    found.forEach(function (t) { put(stickOf(t), "--wb-rail-top", vals.top); });
    put(host.querySelector(".oq-wrap.top"), "--wb-rail-h", vals.h);
    put(host.querySelector(".oq-wrap.bot"), "--wb-rail-bot", vals.bot);
  }

  // --- motion: a bar turning into a capsule and back --------------------------
  // A capsule is as wide as its item's bar (page.css), so the two differ
  // only in height, corners, and content, and the motion changes only
  // those. At the top, when the current topic's card scrolls past, its bar
  // (pushed up under the top stack by the card's end: a stuck bar draws
  // below the stack, so a short topic never covers the capsules) turns
  // into the stack's near capsule: a solid copy changes from the part of
  // the bar still showing under the stack (shows) to the capsule's top and
  // height, its corners turning to the capsule's pill, while the bar's
  // content gives way to the capsule's. Scrolling back, a copy of the
  // capsule turns into the bar the same way. The bottom edge mirrors it:
  // when the bottom stack's near capsule's topic scrolls into view above
  // the stack, a copy of the capsule grows into its bar, and when a shown
  // bar sinks behind the stack it collapses into the near capsule.
  // Capsules that stay in a stack slide from their old slot to their new
  // one. The copies stand in their edge's wrapper, out of flow, so nothing
  // below moves, and leave when their motion ends; nothing on the board
  // keeps an animated style. It follows scrolling only: none of it runs
  // under reduced motion, on a run's first draw (a load or a board swap),
  // on the first draw after the board was hidden, or on a draw after a
  // size change in the board. A board swap's own motion (the dashboard's
  // topics growing and shrinking) moves item after item across a stack's
  // line, and each crossing turned a bar into a copy aimed at where the
  // bar stood that frame while the bar itself hid: closed topics pouring
  // in showed as blank rows, one after another.
  var DUR = 180, EASE = "ease-out", curTopic = null, drawn = false, ghosts = { top: null, bot: null };
  function motion() { return !!d.body.animate && !(window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches); }
  function btns(edge) { return [slots[edge].far, slots[edge].near]; }
  function shown(b) { return !b.hidden && b.wbTopic ? b.wbTopic : null; }
  function seen(r) { return r.height > 0 && r.bottom > viewTop() && r.top < (root ? root.getBoundingClientRect().bottom : window.innerHeight); }
  // look reads what a copy of el needs to look like it once it stands in a
  // wrapper, out of its item: its fill, corners, text colour, and the type
  // size of its name. The capsule's is read before the redraw refills it.
  var TN = ".tname, [data-rail-name], .cn";
  function look(el) {
    var cs = getComputedStyle(el), nm = el.querySelector(TN);
    return { copy: el.cloneNode(true), bg: cs.backgroundColor, fg: cs.color, radius: cs.borderRadius, size: nm ? getComputedStyle(nm).fontSize : "" };
  }
  function barGhost(t, visible) {
    var tb = t && barOf(t), r = tb && tb.getBoundingClientRect();
    return tb && (!visible || seen(r)) ? { kind: "bar", t: t, src: look(tb), r: r } : null;
  }
  // shows cuts a bar's box to the part that shows under the top stack: the
  // stack's band covers the rest, and a copy drawn over the band would
  // cover its capsules. A bar that has slid under the band whole (as one
  // pushed up by its card's end has, by the time its card is past) leaves a
  // box of no height at the band's lower edge, so its capsule grows out of
  // that edge, and shrinks back into it when its bar comes out again.
  function shows(r) {
    var s = wraps.top.firstChild, lim = s.hidden ? viewTop() : s.getBoundingClientRect().bottom;
    var top = Math.max(r.top, lim), bottom = Math.max(r.bottom, lim);
    return { left: r.left, width: r.width, top: top, bottom: bottom, height: bottom - top };
  }
  function capGhost(b) { return { kind: "cap", t: b.wbTopic, src: look(b), r: b.getBoundingClientRect() }; }
  // before reads, ahead of a redraw, what the motion starts from: the
  // shown capsules' boxes by edge and topic, and per edge the bar or the
  // capsule that is about to turn into the other.
  function before(s) {
    var now = s && s.cur >= 0 ? topics[s.cur] : null;
    var p = { caps: [], top: null, bot: null };
    ["top", "bot"].forEach(function (e) { btns(e).forEach(function (b) { if (shown(b)) p.caps.push({ e: e, t: b.wbTopic, r: b.getBoundingClientRect() }); }); });
    if (!drawn || !s) return p;
    var near = shown(slots.top.near);
    if (curTopic && curTopic !== now && topics[s.up[0]] === curTopic) {
      p.top = barGhost(curTopic, false);
      if (p.top) p.top.r = shows(p.top.r);
    }
    else if (now && now === near && now !== curTopic) p.top = capGhost(slots.top.near);
    // Bottom: the near capsule's topic leaves the stack (its bar now shows
    // above it), or a topic not in the stack becomes its near capsule
    // while its bar is still in view.
    var bnear = shown(slots.bot.near), next = topics[s.down[0]] || null;
    if (bnear && !s.down.some(function (i) { return topics[i] === bnear; })) p.bot = capGhost(slots.bot.near);
    else if (next && btns("bot").map(shown).indexOf(next) < 0) p.bot = barGhost(next, true);
    return p;
  }
  // after reads every box the motion needs first (one layout, after the
  // redraw's writes) and only then starts the animations, so no animation
  // or copy dirties the layout between two reads.
  function after(p) {
    var slides = [], morphs = [];
    ["top", "bot"].forEach(function (e) {
      var g = p[e];
      btns(e).forEach(function (b) {
        if (!shown(b) || (g && g.kind === "bar" && b.wbTopic === g.t)) return;
        for (var i = 0; i < p.caps.length; i++) {
          if (p.caps[i].e !== e || p.caps[i].t !== b.wbTopic) continue;
          var dy = p.caps[i].r.top - b.getBoundingClientRect().top;
          if (Math.abs(dy) >= 1) slides.push([b, dy]);
          return;
        }
      });
      if (!g) return;
      if (g.kind === "bar") {
        if (shown(slots[e].near) === g.t) morphs.push(measure(g, slots[e].near, e));
        return;
      }
      // A capsule grows into the bar of the topic it stood for: at the top
      // the topic that became current, at the bottom one that left the
      // stack and shows.
      var tb = barOf(g.t);
      if (!tb || (e === "top" ? g.t !== curTopic : btns("bot").map(shown).indexOf(g.t) >= 0 || !seen(tb.getBoundingClientRect()))) return;
      morphs.push(measure(g, tb, e));
    });
    slides.forEach(function (x) { x[0].animate([{ transform: "translateY(" + x[1] + "px)" }, { transform: "none" }], { duration: DUR, easing: EASE }); });
    morphs.forEach(morph);
  }
  function dropGhost(edge) {
    (edge ? [edge] : ["top", "bot"]).forEach(function (e) {
      (ghosts[e] || []).forEach(function (el) { if (el.parentNode) el.parentNode.removeChild(el); });
      ghosts[e] = null;
    });
  }
  // morph stands a solid copy in edge's wrapper and changes it from g.r to
  // into's box by height alone: it keeps into's left edge and width
  // throughout, while its top, height, corners (between the bar's and the
  // capsule's pill), and fill move. Inside it, g's content gives way to
  // into's, both centred and clipped to its height. into hides
  // under it until it ends; nothing else fades. A stuck bar has its item's
  // body scrolling under it, so while it hides, a page-coloured plate takes
  // its box and the copy grows over that, not over the body.
  // measure reads what morph needs (the wrapper's and into's boxes, into's
  // look); morph only writes.
  function measure(g, into, edge) {
    var wrap = wraps[edge], w = wrap.getBoundingClientRect(), from = g.r, to = into.getBoundingClientRect(), bar = g.kind === "bar";
    if (!bar && edge === "top") to = shows(to);
    return { g: g, into: into, edge: edge, wrap: wrap, w: w, from: from, to: to, bar: bar, b: look(into) };
  }
  function morph(m) {
    var g = m.g, into = m.into, edge = m.edge, wrap = m.wrap, w = m.w, from = m.from, to = m.to, bar = m.bar;
    dropGhost(edge);
    var a = g.src, b = m.b, el = d.createElement("div");
    el.className = "wb-rail-ghost";
    el.setAttribute("aria-hidden", "true");
    el.style.left = to.left - w.left + "px";
    el.style.width = to.width + "px";
    [a, b].forEach(function (l) {
      var c = l.copy, nm = c.querySelector(TN);
      c.hidden = false;
      c.removeAttribute("id");
      c.style.color = l.fg;
      if (nm && l.size) nm.style.fontSize = l.size;
      el.appendChild(c);
    });
    ghosts[edge] = [el];
    if (!bar && into.closest(".wb-stuck")) {
      var plate = d.createElement("div");
      plate.className = "wb-rail-ghost";
      plate.style.cssText = "left:" + el.style.left + ";width:" + el.style.width + ";top:" + (to.top - w.top) + "px;height:" + to.height + "px;background:var(--wb-bg)";
      wrap.appendChild(plate);
      ghosts[edge].unshift(plate);
    }
    wrap.appendChild(el);
    var mine = ghosts[edge];
    el.animate([
      { top: from.top - w.top + "px", height: from.height + "px", borderRadius: bar ? a.radius : from.height / 2 + "px", backgroundColor: a.bg },
      { top: to.top - w.top + "px", height: to.height + "px", borderRadius: bar ? to.height / 2 + "px" : b.radius, backgroundColor: b.bg }
    ], { duration: DUR, easing: EASE, fill: "forwards" }).onfinish = function () { if (ghosts[edge] === mine) dropGhost(edge); };
    // The two contents take turns, g's out over the first half and into's in
    // over the second, so their differently placed text never doubles up.
    el.firstChild.animate([{ opacity: 1 }, { opacity: 0, offset: 0.5 }, { opacity: 0 }], { duration: DUR, fill: "forwards" });
    el.lastChild.animate([{ opacity: 0 }, { opacity: 0, offset: 0.5 }, { opacity: 1 }], { duration: DUR, fill: "forwards" });
    into.animate([{ opacity: 0 }, { opacity: 0 }], { duration: DUR });
  }
  // The keys of the items at the given indices: the draw key holds keys,
  // not indices, so a re-queried board (closed topics fetched in, a topic
  // added above) redraws even when the indices match.
  function names(list) { return list.map(function (i) { return topics[i] ? keyOf(topics[i]) : ""; }).join("\u0000"); }

  function kick() { if (!raf) raf = requestAnimationFrame(run); }
  // A size change inside the board (a topic swapped, a body or settled list
  // opened, closed topics fetched in) can change the topic list and every
  // position; the box's own size changes when its tab shows or the window
  // resizes.
  function reflow() { dirty = true; kick(); }
  scroller.addEventListener("scroll", kick, { passive: true });
  window.addEventListener("resize", reflow);
  var ro = window.ResizeObserver ? new ResizeObserver(reflow) : null;
  if (ro) {
    ro.observe(box);
    Array.prototype.forEach.call(box.children, function (c) { if (!c.classList.contains("wb-rail-wrap") && !c.classList.contains("oq-wrap")) ro.observe(c); });
  }

  // Jump: put the topic's top where its bar sticks, under the top capsules
  // it will have once current.
  function jump(e) {
    var b = e.target.closest && e.target.closest(".wb-cap"), t = b && b.wbTopic;
    if (!t || !t.isConnected) return;
    var j = topics.indexOf(t);
    var delta = t.getBoundingClientRect().top - (viewTop() + capH(Math.min(2, Math.max(0, j))));
    var smooth = !(window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches);
    scroller.scrollBy({ top: delta, behavior: smooth ? "smooth" : "auto" });
  }
  wraps.top.addEventListener("click", jump);
  wraps.bot.addEventListener("click", jump);

  // Check pill (button.ckbtn in an open topic's head). At rest it folds the
  // topic's in-flow check list. On a stuck head the list is out of view, so
  // the pill lifts the list itself into the head, where page.css hangs it as
  // a popover over the body; a spacer of the list's height keeps its place,
  // so nothing under it moves. The real list moves rather than a copy: its
  // answer controls are what form.js reads. A second click, the head
  // unsticking, or a teardown puts it back (drop).
  var lifted = null;
  function drop() {
    var l = lifted;
    lifted = null;
    if (!l) return;
    if (l.spacer.parentNode) l.spacer.parentNode.replaceChild(l.list, l.spacer);
    else if (l.list.parentNode) l.list.parentNode.removeChild(l.list);
    l.list.hidden = l.hid;
    l.btn.setAttribute("aria-expanded", l.hid ? "false" : "true");
  }
  function pill(e) {
    var btn = e.target.closest && e.target.closest("button.ckbtn"), head = btn && btn.closest(".tophead");
    if (!head || !box.contains(head)) return;
    if (lifted && lifted.head === head) return drop();
    var list = head.parentNode.querySelector(":scope > .checks");
    if (!list) return;
    if (!head.classList.contains("wb-stuck")) {
      list.hidden = !list.hidden;
      btn.setAttribute("aria-expanded", list.hidden ? "false" : "true");
      return;
    }
    drop();
    var cs = getComputedStyle(list), spacer = d.createElement("div");
    spacer.className = "ckspace";
    spacer.setAttribute("aria-hidden", "true");
    if (!list.hidden) spacer.style.cssText = "height:" + list.offsetHeight + "px;margin:" + cs.marginTop + " 0 " + cs.marginBottom;
    lifted = { head: head, list: list, spacer: spacer, btn: btn, hid: list.hidden };
    list.parentNode.replaceChild(spacer, list);
    list.hidden = false;
    head.appendChild(list);
    btn.setAttribute("aria-expanded", "true");
  }
  box.addEventListener("click", pill);
  kick();

  st.off = function () {
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
    if (ro) ro.disconnect();
    scroller.removeEventListener("scroll", kick);
    window.removeEventListener("resize", reflow);
    wraps.top.removeEventListener("click", jump);
    wraps.bot.removeEventListener("click", jump);
    box.removeEventListener("click", pill);
    drop();
    if (stuck) stuck.classList.remove("wb-stuck");
    stuck = null;
    dropGhost();
  };
})();
