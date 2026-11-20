;(function () {
  // Open-question pill. Items awaiting the user (open policy, ack, check)
  // scrolled out of the board's scroll area show a floating pill at that
  // edge, split into still-open items and items already answered (the form
  // holds a reply, or the dashboard marked one sent or held), and the tab
  // title carries the still-open total. The dashboard
  // re-runs this after every board swap, so it tears down what the previous
  // run attached.
  var d = document;
  if (!window.IntersectionObserver) return;
  var host = d.getElementById("bcol") || d.querySelector(".wrap");
  if (!host) return;
  var st = window.wbPill || (window.wbPill = {});
  if (st.off) st.off();

  var items = Array.prototype.slice.call(d.querySelectorAll('.card.policy[id^="item-"], .arow[id^="item-"], .ckrow[data-id]:not([data-assumed])'));

  // An item's draft state, read from the same form controls form.js builds
  // the reply from: a pick, a ticked ack, or a non-empty note is a reply
  // segment ("responded"); a note alone also counts as "noted". Item fields
  // keep their class attribute, so each control is selected by what a field
  // cannot carry (a data-* attribute, a textarea or label element), as
  // form.js does.
  function noted(el) {
    var n = el.querySelector("textarea.note");
    return !!(n && n.value.trim());
  }
  function responded(el) {
    var box = el.querySelector("label.ack > input, label.ck > input");
    return !!(el.querySelector('.opt[data-opt][aria-pressed="true"]') || (box && box.checked) || el.hasAttribute("data-no") || noted(el));
  }
  // The dashboard marks an item it already sent, or holds, a reply for with
  // sent-sent / sent-held; that item is no longer open either. A standalone
  // page never carries these classes.
  function answered(el) {
    return responded(el) || el.classList.contains("sent-sent") || el.classList.contains("sent-held");
  }

  // The title prefix is the still-open total, whatever is in view. A page
  // whose html element carries data-wb-titlehost (the dashboard) owns the
  // title itself: the total goes to its window.wbTitleCount hook instead,
  // and this script never writes the title there, even before the hook
  // exists.
  var html = d.documentElement, hosted = html.hasAttribute("data-wb-titlehost"), base = null;
  if (!hosted) {
    base = html.getAttribute("data-wb-title");
    if (base === null) { base = d.title.replace(/^\(\d+\) /, ""); html.setAttribute("data-wb-title", base); }
  }
  function retitle() {
    var n = items.filter(function (el) { return !answered(el); }).length;
    if (hosted) { if (window.wbTitleCount) window.wbTitleCount(n); return; }
    var title = (n ? "(" + n + ") " : "") + base;
    if (d.title !== title) d.title = title;
  }

  // Two zero-height sticky wrappers, first and last in the host, carry the
  // pills so showing one never moves the board. A pill is a card-coloured
  // capsule holding the edge's arrow and one button per state, in the order
  // open, answered, noted; show() fills their counts and hides an empty one.
  // The buttons are built once, so a redraw never drops a focused one.
  var KINDS = ["open", "ans", "nt"];
  function wrapper(edge) {
    // Edge modifiers must not share the page header's bare .top class.
    var modifier = edge === "top" ? "wb-oq-top" : "wb-oq-bot";
    var w = host.querySelector(".oq-wrap." + modifier);
    if (!w) {
      w = d.createElement("div");
      w.className = "oq-wrap " + modifier;
      var p = d.createElement("div");
      p.className = "oq-pill";
      p.hidden = true;
      var arr = d.createElement(edge === "top" ? "button" : "span");
      arr.className = "oq-arr";
      if (edge === "top") {
        arr.type = "button";
        arr.title = wsT("js.board.hold.pill.title");
        arr.setAttribute("aria-label", arr.title);
      } else arr.setAttribute("aria-hidden", "true");
      arr.textContent = edge === "top" ? "↑" : "↓";
      if (edge === "top") arr.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 19V5M5 12l7-7 7 7"/></svg>';
      p.appendChild(arr);
      if (edge === "top") {
        var updates = d.createElement("button");
        updates.type = "button";
        updates.className = "oq-updates";
        updates.hidden = true;
        updates.title = arr.title;
        p.appendChild(updates);
        var pull = d.createElement("button");
        pull.type = "button";
        pull.className = "oq-pull";
        pull.hidden = true;
        pull.title = wsT("js.board.hold.pull");
        pull.setAttribute("aria-label", pull.title);
        pull.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11M7 11l5 5 5-5M5 20h14"/></svg>';
        p.appendChild(pull);
      }
      KINDS.forEach(function (k) {
        var b = d.createElement("button");
        b.type = "button";
        b.className = "oq-seg " + k;
        b.setAttribute("data-k", k);
        b.hidden = true;
        var label = d.createElement("span");
        label.textContent = k === "open" ? wsT("js.pill.open") : k === "ans" ? wsT("js.pill.answered") : wsT("js.pill.noted");
        var n = d.createElement("span");
        n.className = "n";
        b.appendChild(label);
        b.appendChild(n);
        if (edge === "top") b.title = k === "open" ? wsT("js.pill.up") : k === "ans" ? wsT("js.pill.upanswered") : wsT("js.pill.upnoted");
        else b.title = k === "open" ? wsT("js.pill.down") : k === "ans" ? wsT("js.pill.downanswered") : wsT("js.pill.downnoted");
        p.appendChild(b);
      });
      w.appendChild(p);
      if (edge === "top") host.insertBefore(w, host.firstChild); else host.appendChild(w);
    }
    return w;
  }
  var wraps = { top: wrapper("top"), bot: wrapper("bot") };
  var pills = { top: wraps.top.firstChild, bot: wraps.bot.firstChild };
  var target = { top: {}, bot: {} };

  // The host is the scroll root when it scrolls on its own (the dashboard's
  // board column); otherwise the window scrolls.
  var root = /auto|scroll/.test(getComputedStyle(host).overflowY) ? host : null;
  // Observers only invalidate the display; they are not the pending-item
  // inventory. Their reports can describe layout before a frame height (or
  // a scroll compensation) landed. Mixing that snapshot with current topic
  // rects made static counts depend on observer delivery order.
  var positions = new Set();
  items.forEach(function (el) { positions.add(el.closest("section.topic") || el); });

  // list is ordered top to bottom; the nearest item at the top edge is the
  // last one, at the bottom edge the first. Each segment jumps to the nearest
  // item of its own state: still open, answered, or answered with a note
  // (noted items are a subset of the answered ones).
  function atTop() { return (root ? root.scrollTop : window.scrollY) <= 4; }
  function show(edge, list, width, home, goal) {
    var p = pills[edge], pending = edge === "top" && st.pending > 0;
    // The top capsule has nothing to offer at the very top with no board
    // update pending (no item above either); a latched change keeps it.
    var hide = edge === "top" ? !pending && !list.length && home : !list.length;
    var label = pending ? wsT("js.board.hold.pill") + " " + st.pending : "";
    var groups = { open: [], ans: [], nt: [] };
    list.forEach(function (el) {
      if (!answered(el)) { groups.open.push(el); return; }
      groups.ans.push(el);
      if (noted(el)) groups.nt.push(el);
    });
    target[edge] = {};
    KINDS.forEach(function (k) { var g = groups[k]; if (g.length) target[edge][k] = edge === "top" ? g[g.length - 1] : g[0]; });
    var shape = [hide, label, groups.open.length, groups.ans.length, groups.nt.length].join("|");
    if (shape === p.wbShape) {
      // The invisible copy supplies the new intrinsic width while the real
      // capsule still presents its old width. Both were read before writes.
      if (p.wbMeasure) {
        p.wbMeasure.parentNode.removeChild(p.wbMeasure);
        p.wbMeasure = null;
        p.style.width = "";
        var from = p.wbFrom;
        if (from && goal && from !== goal) {
          p.wbGrow = p.animate([{ width: from + "px" }, { width: goal + "px" }], { duration: 420, easing: "cubic-bezier(.34,1.56,.64,1)" });
          p.wbGrow.onfinish = function () { p.style.overflow = ""; };
        } else p.style.overflow = "";
      }
      return;
    }
    if (p.hidden !== hide) p.hidden = hide;
    p.classList.toggle("board-updated", !!pending);
    if (edge === "top") {
      var updates = p.querySelector(".oq-updates");
      if (updates.hidden !== !pending) updates.hidden = !pending;
      if (updates.textContent !== label) updates.textContent = label;
      var pull = p.querySelector(".oq-pull");
      if (pull.hidden !== !pending) pull.hidden = !pending;
    }
    Array.prototype.forEach.call(p.querySelectorAll(".oq-seg"), function (b) {
      var k = b.getAttribute("data-k"), g = groups[k] || [];
      if (b.hidden !== !g.length) b.hidden = !g.length;
      var count = b.querySelector(".n"), text = String(g.length), label = b.title + " (" + g.length + ")";
      if (count.textContent !== text) count.textContent = text;
      if (b.getAttribute("aria-label") !== label) b.setAttribute("aria-label", label);
    });
    // Measure the destination next frame, without flashing it before the
    // grow starts. Only the real capsule holds a width; the hidden copy is
    // intrinsic and cannot receive focus or duplicate accessible controls.
    if (p.wbGrow) p.wbGrow.cancel();
    if (p.wbMeasure) p.wbMeasure.parentNode.removeChild(p.wbMeasure);
    p.wbMeasure = null;
    p.style.width = "";
    if (p.style.overflow) p.style.overflow = "";
    p.wbShape = shape;
    if (p.animate && width && !hide && !(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches)) {
      var copy = p.cloneNode(true);
      copy.style.visibility = "hidden";
      copy.style.animation = "none";
      copy.style.pointerEvents = "none";
      copy.setAttribute("aria-hidden", "true");
      copy.inert = true;
      wraps[edge].appendChild(copy);
      p.wbMeasure = copy; p.wbFrom = width;
      p.style.width = width + "px";
      p.style.overflow = "clip";
      redrawSoon();
    }
  }
  // Avoid reading every far item's rect: an item in a topic wholly above
  // or below the view takes its side from the topic's rect, read once per
  // update, unless a closed fold hides it. Only an item in a topic crossing
  // the view reads its own rect.
  function folded(el, topic) {
    for (var p = el.parentElement; p && p !== topic; p = p.parentElement) if (p.tagName === "DETAILS" && !p.open) return true;
    return false;
  }
  function update() {
    var rt = 0, rb = window.innerHeight;
    if (root) { var rr = root.getBoundingClientRect(); rt = rr.top; rb = rr.bottom; }
    var above = [], below = [], topics = new Map();
    items.forEach(function (el) {
      var topic = el.closest("section.topic"), tr = topic && topics.get(topic);
      if (topic && !tr) { tr = topic.getBoundingClientRect(); topics.set(topic, tr); }
      if (tr && (tr.bottom <= rt || tr.top >= rb)) {
        if ((tr.width || tr.height) && !folded(el, topic)) (tr.bottom <= rt ? above : below).push(el);
        return;
      }
      var r = el.getBoundingClientRect();
      if (!r.width && !r.height) return; // not rendered, e.g. the board tab is hidden
      if (r.bottom > rt && r.top < rb) return; // currently in view
      (r.top < rt ? above : below).push(el);
    });
    // Read reply clearance before either capsule writes.
    var reply = host.querySelector(".reply"), replyHeight = reply ? reply.offsetHeight : 0;
    var bottom = "calc(" + (replyHeight ? replyHeight + 18 : 10) + "px + var(--wb-rail-bot, 0px))";
    var topWidth = pills.top.offsetWidth, botWidth = pills.bot.offsetWidth, home = atTop();
    var topGoal = pills.top.wbMeasure ? pills.top.wbMeasure.offsetWidth : 0;
    var botGoal = pills.bot.wbMeasure ? pills.bot.wbMeasure.offsetWidth : 0;
    show("top", above, topWidth, home, topGoal);
    show("bot", below, botWidth, home, botGoal);
    retitle();
    if (wraps.bot.style.bottom !== bottom) wraps.bot.style.bottom = bottom;
  }

  // Watch topic positions, not every far control's rendered visibility.
  // Counts always come from the data-backed items and one current layout.
  var io = new IntersectionObserver(redrawSoon, { root: root });
  positions.forEach(function (el) { io.observe(el); });

  // A jump scroll can carry an item past the view without a crossing the
  // observer reports, and a hidden board column changes size without one; a
  // scroll or resize refresh covers both, coalesced to one pending frame.
  function later() { redrawSoon(); }
  var scroller = root || window;
  scroller.addEventListener("scroll", later, { passive: true });
  window.addEventListener("resize", later);
  var ro = window.ResizeObserver ? new ResizeObserver(later) : null;
  if (ro) {
    ro.observe(host);
    // The host's border box need not resize when a mounted body changes
    // its topic's height. Refresh even if no topic crosses an IO threshold.
    positions.forEach(function (el) { ro.observe(el); });
  }

  // A pick, a ticked ack, or a typed note changes an item's draft state; the
  // host-level listeners run after form.js's own control handlers.
  // Typing redraws once a frame at most (redrawSoon): read right after
  // form.js grew the note, every item's rect forced a second layout per
  // keystroke.
  function redraw() { redrawSoon(); }
  var typed = 0;
  function redrawSoon() { if (!typed) typed = requestAnimationFrame(function () { typed = 0; update(); }); }
  host.addEventListener("click", redraw);
  host.addEventListener("input", redrawSoon);
  host.addEventListener("change", redraw);
  // The dashboard's sent marks land after its region swaps, including a
  // conversation-only swap that does not re-run this script, so watch the
  // items' class attributes rather than wait for a board swap.
  var mo = window.MutationObserver ? new MutationObserver(redraw) : null;
  if (mo) items.forEach(function (el) { mo.observe(el, { attributes: true, attributeFilter: ["class"] }); });
  retitle();

  function click(edge) {
    return function activate(e) {
      if (edge === "top" && e.target.closest && e.target.closest(".oq-pull")) {
        if (st.apply) st.apply();
        return;
      }
      if (edge === "top" && e.target.closest && e.target.closest(".oq-arr, .oq-updates")) {
        if (st.home) st.home();
        else if (window.wbHistory && window.wbHistory.jump) window.wbHistory.jump(pills.top, { top: 0, behavior: "smooth" });
        else (root || window).scrollTo({ top: 0, behavior: window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" });
        return;
      }
      var b = e.target.closest && e.target.closest(".oq-seg");
      var el = b && target[edge][b.getAttribute("data-k")];
      if (!el) return;
      if (st.apply) {
        if (!st.apply(function () { activate(e); })) return;
        if (el.isConnected === false) {
          // Applied history can put the old target back in an inert gap.
          var history = window.wbHistory;
          if (el.id && history && history.loadTarget && history.loadTarget(d, el.id, window.WsCards && window.WsCards.unfoldTopic)) return;
          el = el.id ? d.getElementById(el.id) : null;
        }
        if (!el) return;
      }
      var w = window.wbHistory;
      if (w && w.jump) w.jump(el, { block: "center", behavior: "smooth" });
      else el.scrollIntoView({ block: "center", behavior: window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" });
    };
  }
  pills.top.onclick = click("top");
  pills.bot.onclick = click("bot");

  st.update = redrawSoon;
  redrawSoon();
  st.off = function () {
    io.disconnect();
    if (ro) ro.disconnect();
    if (mo) mo.disconnect();
    if (typed) cancelAnimationFrame(typed);
    [pills.top, pills.bot].forEach(function (p) {
      if (p.wbGrow) p.wbGrow.cancel();
      if (p.wbMeasure) p.wbMeasure.parentNode.removeChild(p.wbMeasure);
      p.wbMeasure = null;
      p.style.width = "";
      if (p.style.overflow) p.style.overflow = "";
    });
    scroller.removeEventListener("scroll", later);
    window.removeEventListener("resize", later);
    host.removeEventListener("click", redraw);
    host.removeEventListener("input", redrawSoon);
    host.removeEventListener("change", redraw);
  };
})();
