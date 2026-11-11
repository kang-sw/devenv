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
  // host carries the rail class and its offsets (the dashboard's board
  // column, the full page's wrapper, the same element pill.js uses, so the
  // open-question pill reads the offsets too); box holds the topics and the
  // wrappers.
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
  // reserve, so the line below does not move with the capsule count.
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
  var topics = [], found = [], dirty = true, last = "", stuck = null, raf = 0, inset = 0;
  function readInset() { inset = root ? parseFloat(getComputedStyle(host).getPropertyValue("--wb-rail-inset")) || 0 : 0; }
  function viewTop() { return root ? root.getBoundingClientRect().top + padTop + inset : 0; }

  // The rail's items are the board's topics and any other node the host
  // marks with data-rail (its key, unique among the marked nodes), in board
  // order; an item with no height (hidden) is left out. A marked node's
  // attributes name its bar (data-rail-bar: the header that sticks while
  // it is current, as a topic's .tbar does under the host's own rule), its
  // capsule's text (data-rail-name), and the icon its capsule leads with
  // (data-rail-icon, copied).
  var ITEMS = "section.topic[data-topic], [data-rail]";
  function isTopic(t) { return t.hasAttribute("data-topic"); }
  function barOf(t) { return t.querySelector(isTopic(t) ? ":scope > .tbar" : "[data-rail-bar]"); }
  function keyOf(t) { return isTopic(t) ? t.getAttribute("data-topic") : "\u0001" + t.getAttribute("data-rail"); }

  function fill(b, t, extra) {
    b.hidden = !t;
    b.wbTopic = t || null;
    if (!t) return;
    var name, n = "", icon = "";
    if (isTopic(t)) {
      // The rail shows the topic's visible name (.tname, its label), not
      // data-topic, which carries the key a stemmed topic is addressed by.
      var tn = t.querySelector(":scope > .tbar .tname"), pol = t.querySelector(":scope > .tbar > .chip.policy");
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
    if (dirty) { found = Array.prototype.slice.call(box.querySelectorAll(ITEMS)); dirty = false; }
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
    // stack gets no capsule, so the stack may hold fewer than two.
    var reply = box.querySelector(".reply"), replyOff = reply && reply.offsetHeight ? reply.offsetHeight + 14 : 0;
    var line = vb - replyOff - BOT, below = [];
    for (k = above + (cur >= 0 ? 1 : 0); k < n; k++) if (rects[k].top >= line) below.push(k);
    draw({
      top: top,
      cur: cur,
      // The current bar sits at the stick line; it is stuck (and casts its
      // shadow) once its card's top has passed that line.
      bar: cur >= 0 ? barOf(topics[cur]) : null,
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
    if (key === last) return;
    last = key;
    var pre = motion() ? before(s) : null;
    var bar = s && s.bar, shadow = s && s.stuck ? bar : null;
    if (stuck !== shadow) {
      if (stuck) stuck.classList.remove("wb-stuck");
      if (shadow) shadow.classList.add("wb-stuck");
      stuck = shadow;
    }
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
    // over the board. --wb-rail-bot is the capsule part alone: pill.js adds
    // the reply bar's own clearance.
    var replyOff = s ? s.replyOff : 0;
    wraps.bot.firstChild.style.paddingBottom = replyOff ? replyOff + PAD + "px" : "";
    var barH = bar ? bar.offsetHeight : 0;
    host.style.setProperty("--wb-rail-top", inset + (s ? s.top : 0) + "px");
    host.style.setProperty("--wb-rail-h", inset + (s ? s.top + barH : 0) + "px");
    host.style.setProperty("--wb-rail-bot", (down.length ? wraps.bot.firstChild.offsetHeight - replyOff : 0) + "px");
    curTopic = s && s.cur >= 0 ? topics[s.cur] : null;
    if (pre) after(pre);
  }

  // --- motion: a bar turning into a capsule and back --------------------------
  // When the current topic's card scrolls past, its bar (pushed up over the
  // top stack by the card's end: a stuck bar draws above the stack) turns
  // into the stack's near capsule: a copy of the bar shrinks from where the
  // bar is to the capsule's box, its details fading and sliding out and its
  // title shrinking to the capsule's size, while the capsule fades in under
  // it. Scrolling back, a copy of the capsule grows into the bar's box and
  // fades while the bar fades in. Capsules that stay in the stack slide from
  // their old slot to their new one. The copies stand in the top wrapper,
  // out of flow, so nothing below moves; every animation fills nothing.
  // None of it runs under reduced motion.
  var DUR = 180, EASE = "ease-out", curTopic = null, ghostEl = null;
  function motion() { return !!d.body.animate && !(window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches); }
  function topBtns() { return [slots.top.far, slots.top.near]; }
  // before reads, ahead of a redraw, what the motion starts from: the
  // shown top capsules' boxes by topic, and the bar or the capsule that is
  // about to turn into the other.
  function before(s) {
    var now = s && s.cur >= 0 ? topics[s.cur] : null, near = slots.top.near.hidden ? null : slots.top.near.wbTopic;
    var p = { caps: [], prev: curTopic, now: now, ghost: null };
    topBtns().forEach(function (b) { if (!b.hidden && b.wbTopic) p.caps.push({ t: b.wbTopic, r: b.getBoundingClientRect() }); });
    if (curTopic && curTopic !== now && s && topics[s.up[0]] === curTopic) {
      var tb = barOf(curTopic);
      if (tb) p.ghost = { kind: "bar", src: tb, copy: tb.cloneNode(true), r: tb.getBoundingClientRect() };
    } else if (now && now === near && now !== curTopic) {
      p.ghost = { kind: "cap", src: slots.top.near, copy: slots.top.near.cloneNode(true), r: slots.top.near.getBoundingClientRect() };
    }
    return p;
  }
  function after(p) {
    topBtns().forEach(function (b) {
      if (b.hidden || !b.wbTopic || (p.ghost && p.ghost.kind === "bar" && b.wbTopic === p.prev)) return;
      for (var i = 0; i < p.caps.length; i++) {
        if (p.caps[i].t !== b.wbTopic) continue;
        var dy = p.caps[i].r.top - b.getBoundingClientRect().top;
        if (Math.abs(dy) >= 1) b.animate([{ transform: "translateY(" + dy + "px)" }, { transform: "none" }], { duration: DUR, easing: EASE });
        return;
      }
    });
    var g = p.ghost;
    if (!g) return;
    if (g.kind === "bar") {
      var cap = slots.top.near;
      if (cap.hidden || cap.wbTopic !== p.prev) return;
      morph(g, cap, cap.getBoundingClientRect());
    } else {
      var tb = barOf(p.now);
      if (tb) morph(g, tb, tb.getBoundingClientRect());
    }
  }
  function dropGhost() {
    if (ghostEl && ghostEl.parentNode) ghostEl.parentNode.removeChild(ghostEl);
    ghostEl = null;
  }
  // morph animates the copy g.copy from g.r to the box to, the element
  // into fading in under it.
  function morph(g, into, to) {
    dropGhost();
    var w = wraps.top.getBoundingClientRect(), from = g.r, bar = g.kind === "bar";
    var el = g.copy, cs = getComputedStyle(g.src);
    el.className += " wb-rail-ghost";
    el.hidden = false;
    el.setAttribute("aria-hidden", "true");
    el.setAttribute("tabindex", "-1");
    el.style.margin = "0";
    el.style.background = cs.backgroundColor;
    el.style.color = cs.color;
    wraps.top.appendChild(el);
    ghostEl = el;
    function box(r, radius) { return { left: r.left - w.left + "px", top: r.top - w.top + "px", width: r.width + "px", height: r.height + "px", borderRadius: radius }; }
    var a = box(from, bar ? cs.borderRadius : from.height / 2 + "px"), b = box(to, bar ? to.height / 2 + "px" : getComputedStyle(into).borderRadius);
    a.opacity = 1;
    b.opacity = 0;
    el.animate([a, b], { duration: DUR, easing: EASE }).onfinish = function () { if (ghostEl === el) dropGhost(); };
    into.animate([{ opacity: 0 }, { opacity: 1 }], { duration: DUR, easing: EASE });
    if (!bar) return;
    // The bar's details fade and slide out; its title (and an item's icon)
    // stays, the title shrinking to the capsule's type size.
    Array.prototype.forEach.call(el.children, function (c) {
      if (!c.classList.contains("tlead") && !c.hasAttribute("data-rail-name") && !c.hasAttribute("data-rail-icon")) c.animate([{ opacity: 1, transform: "none" }, { opacity: 0, transform: "translateY(-4px)" }], { duration: DUR * 0.6, easing: EASE, fill: "forwards" });
    });
    var TN = ".tname, [data-rail-name]", tn = el.querySelector(TN), sn = g.src.querySelector(TN), cn = into.querySelector(".cn");
    if (tn && sn && cn) tn.animate([{ fontSize: getComputedStyle(sn).fontSize }, { fontSize: getComputedStyle(cn).fontSize }], { duration: DUR, easing: EASE, fill: "forwards" });
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
  kick();

  st.off = function () {
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
    if (ro) ro.disconnect();
    scroller.removeEventListener("scroll", kick);
    window.removeEventListener("resize", reflow);
    wraps.top.removeEventListener("click", jump);
    wraps.bot.removeEventListener("click", jump);
    if (stuck) stuck.classList.remove("wb-stuck");
    stuck = null;
    dropGhost();
  };
})();
