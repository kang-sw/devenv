;(function () {
  // Topic rail. The topic being read keeps its title bar stuck at the top of
  // the board's scroll area (position: sticky, under .wb-rail), and its
  // neighbours show as jump capsules: up to two previous topics stacked above
  // the stuck bar, and up to two of the next topics not yet in view at the
  // bottom edge. A click scrolls that topic's bar to where it sticks. The
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
  // The bottom stack's height with both capsules: a topic whose bar does not
  // clear it counts as not yet in view.
  var BOT = FADE + 2 * CAP + GAP + PAD;

  function wrapper(edge) {
    var w = box.querySelector(":scope > .wb-rail-wrap." + edge);
    if (!w) {
      w = d.createElement("div");
      w.className = "wb-rail-wrap " + edge;
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
        b.innerHTML = '<span class="cn"></span><span class="cp" hidden></span><span class="cx" hidden></span>';
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

  var topics = [], dirty = true, last = "", stuck = null, raf = 0;
  function viewTop() { return root ? root.getBoundingClientRect().top + padTop : 0; }

  function fill(b, t, extra) {
    b.hidden = !t;
    b.wbTopic = t || null;
    if (!t) return;
    var name = t.getAttribute("data-topic"), pol = t.querySelector(":scope > .tbar > .chip.policy");
    var n = pol ? pol.textContent.trim() : "";
    b.classList.toggle("quiet", t.classList.contains("quiet"));
    b.firstChild.textContent = name;
    b.title = name;
    b.setAttribute("aria-label", name);
    b.children[1].hidden = !n || n === "0";
    b.children[1].textContent = n;
    b.children[2].hidden = !extra;
    b.children[2].textContent = extra ? "+" + extra : "";
  }

  function run() {
    raf = 0;
    if (dirty) { topics = Array.prototype.slice.call(box.querySelectorAll("section.topic[data-topic]")); dirty = false; }
    var n = topics.length, rects = topics.map(function (t) { return t.getBoundingClientRect(); });
    // Nothing rendered (the board tab is hidden) or nothing to rail.
    if (!n || (!rects[0].width && !rects[0].height)) return draw(null);
    var vt = viewTop(), vb = root ? root.getBoundingClientRect().bottom : window.innerHeight;
    // i is the last topic whose top has reached where its bar would stick;
    // it is current while its card still spans that line, and otherwise
    // (the gap after it, or past the last topic) every topic up to it is
    // above.
    var i = -1;
    for (var k = 0; k < n && rects[k].top <= vt + capH(Math.min(2, k)) + 1; k++) i = k;
    var cur = i >= 0 && rects[i].bottom > vt + capH(Math.min(2, i)) + 1 ? i : -1;
    var above = cur >= 0 ? cur : i + 1, nTop = Math.min(2, above), top = capH(nTop);
    // Bottom: the next topics whose bar does not clear the bottom stack (and
    // the reply bar, when it shows).
    var reply = box.querySelector(".reply"), replyOff = reply && reply.offsetHeight ? reply.offsetHeight + 14 : 0;
    var line = vb - replyOff - BOT, below = [];
    for (k = above + (cur >= 0 ? 1 : 0); k < n; k++) if (rects[k].top + 24 > line) below.push(k);
    draw({
      top: top,
      cur: cur,
      // The current bar sits at the stick line; it is stuck (and casts its
      // shadow) once its card's top has passed that line.
      bar: cur >= 0 ? topics[cur].querySelector(":scope > .tbar") : null,
      stuck: cur >= 0 && rects[cur].top < vt + top - 0.5,
      up: [above - 1, above - 2].filter(function (x) { return x >= 0; }),
      upMore: Math.max(0, above - 2),
      down: below.slice(0, 2),
      downMore: Math.max(0, below.length - 2),
      replyOff: replyOff
    });
  }

  function draw(s) {
    var key = s ? [s.top, s.upMore, s.downMore, s.replyOff, s.stuck, names(s.up), names(s.down), names([s.cur])].join("|") : "";
    if (key === last) return;
    last = key;
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
    wraps.bot.firstChild.style.bottom = (s ? s.replyOff : 0) + "px";
    var barH = bar ? bar.offsetHeight : 0;
    host.style.setProperty("--wb-rail-top", (s ? s.top : 0) + "px");
    host.style.setProperty("--wb-rail-h", (s ? s.top + barH : 0) + "px");
    host.style.setProperty("--wb-rail-bot", (down.length ? wraps.bot.firstChild.offsetHeight : 0) + "px");
  }
  // The names of the topics at the given indices: the draw key holds names,
  // not indices, so a re-queried board (closed topics fetched in, a topic
  // added above) redraws even when the indices match.
  function names(list) { return list.map(function (i) { return topics[i] ? topics[i].getAttribute("data-topic") : ""; }).join("\u0000"); }

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
  };
})();
