;(function () {
  // Open-question pill. Items awaiting the user (open policy, ack)
  // scrolled out of the board's scroll area show a floating pill at that
  // edge, split into still-open items and items already answered (the form
  // holds a reply, or the dashboard marked one sent or held), and the tab
  // title carries the still-open total. The dashboard
  // re-runs this after every board swap, so it tears down what the previous
  // run attached.
  var d = document;
  if (d.body.classList.contains("embed") || !window.IntersectionObserver) return;
  var host = d.getElementById("bcol") || d.querySelector(".wrap");
  if (!host) return;
  var st = window.wbPill || (window.wbPill = {});
  if (st.off) st.off();

  var items = Array.prototype.slice.call(d.querySelectorAll('.card.policy[id^="item-"], .arow[id^="item-"]'));

  // An item's draft state, read from the same form controls form.js builds
  // the reply from: a pick, a ticked ack, or a non-empty note is a reply
  // segment ("responded"); a note alone also counts as "noted".
  function noted(el) {
    var n = el.querySelector(".note");
    return !!(n && n.value.trim());
  }
  function responded(el) {
    var box = el.querySelector(".ack input");
    return !!(el.querySelector('.opt[aria-pressed="true"]') || (box && box.checked) || noted(el));
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
    d.title = (n ? "(" + n + ") " : "") + base;
  }

  // Two zero-height sticky wrappers, first and last in the host, carry the
  // pills so showing one never moves the board.
  function wrapper(edge) {
    var w = host.querySelector(".oq-wrap." + edge);
    if (!w) {
      w = d.createElement("div");
      w.className = "oq-wrap " + edge;
      var b = d.createElement("button");
      b.type = "button";
      b.className = "oq-pill";
      b.hidden = true;
      w.appendChild(b);
      if (edge === "top") host.insertBefore(w, host.firstChild); else host.appendChild(w);
    }
    return w;
  }
  var wraps = { top: wrapper("top"), bot: wrapper("bot") };
  var pills = { top: wraps.top.firstChild, bot: wraps.bot.firstChild };
  var target = { top: null, bot: null };

  // The host is the scroll root when it scrolls on its own (the dashboard's
  // board column); otherwise the window scrolls.
  var root = /auto|scroll/.test(getComputedStyle(host).overflowY) ? host : null;
  var seen = new Set();

  // list is ordered top to bottom; the nearest item at the top edge is the
  // last one, at the bottom edge the first. A click goes to the nearest
  // still-open item, or the nearest answered one when none is open.
  function show(edge, list) {
    var p = pills[edge];
    p.hidden = !list.length;
    target[edge] = null;
    if (!list.length) return;
    var open = [], done = [], notes = 0;
    list.forEach(function (el) {
      if (!answered(el)) { open.push(el); return; }
      done.push(el);
      if (noted(el)) notes++;
    });
    var pickFrom = open.length ? open : done;
    target[edge] = edge === "top" ? pickFrom[pickFrom.length - 1] : pickFrom[0];
    var parts = [];
    if (open.length) parts.push(wsT("js.pill.open", open.length));
    if (done.length) parts.push(wsT("js.pill.answered", done.length) + (notes ? " " + wsT("js.pill.noted", notes) : ""));
    p.textContent = (edge === "top" ? "↑ " : "↓ ") + parts.join(" / ");
    p.title = wsT(edge === "top" ? "js.pill.up" : "js.pill.down");
  }
  function update() {
    var rt = 0, rb = window.innerHeight;
    if (root) { var rr = root.getBoundingClientRect(); rt = rr.top; rb = rr.bottom; }
    var above = [], below = [];
    items.forEach(function (el) {
      if (seen.has(el)) return;
      var r = el.getBoundingClientRect();
      if (!r.width && !r.height) return; // not rendered, e.g. the board tab is hidden
      (r.top < rt ? above : below).push(el);
    });
    show("top", above);
    show("bot", below);
    retitle();
    // Keep the bottom pill clear of the sticky reply bar and of the topic
    // rail's bottom capsules (rail.js sets --wb-rail-bot on the host).
    var reply = host.querySelector(".reply");
    wraps.bot.style.bottom = "calc(" + (reply && reply.offsetHeight ? reply.offsetHeight + 18 : 10) + "px + var(--wb-rail-bot, 0px))";
  }

  var io = new IntersectionObserver(function (entries) {
    entries.forEach(function (e) { if (e.isIntersecting) seen.add(e.target); else seen.delete(e.target); });
    update();
  }, { root: root });
  items.forEach(function (el) { io.observe(el); });

  // A jump scroll can carry an item past the view without a crossing the
  // observer reports, and a hidden board column changes size without one; a
  // settled-scroll or resize refresh covers both.
  var timer = 0;
  function later() { clearTimeout(timer); timer = setTimeout(update, 120); }
  var scroller = root || window;
  scroller.addEventListener("scroll", later, { passive: true });
  window.addEventListener("resize", later);
  var ro = window.ResizeObserver ? new ResizeObserver(later) : null;
  if (ro) ro.observe(host);

  // A pick, a ticked ack, or a typed note changes an item's draft state; the
  // host-level listeners run after form.js's own control handlers.
  function redraw() { update(); }
  host.addEventListener("click", redraw);
  host.addEventListener("input", redraw);
  host.addEventListener("change", redraw);
  // The dashboard's sent marks land after its region swaps, including a
  // conversation-only swap that does not re-run this script, so watch the
  // items' class attributes rather than wait for a board swap.
  var mo = window.MutationObserver ? new MutationObserver(redraw) : null;
  if (mo) items.forEach(function (el) { mo.observe(el, { attributes: true, attributeFilter: ["class"] }); });
  retitle();

  function click(edge) {
    return function () { if (target[edge]) target[edge].scrollIntoView({ block: "center", behavior: "smooth" }); };
  }
  pills.top.onclick = click("top");
  pills.bot.onclick = click("bot");

  st.off = function () {
    io.disconnect();
    if (ro) ro.disconnect();
    if (mo) mo.disconnect();
    clearTimeout(timer);
    scroller.removeEventListener("scroll", later);
    window.removeEventListener("resize", later);
    host.removeEventListener("click", redraw);
    host.removeEventListener("input", redraw);
    host.removeEventListener("change", redraw);
  };
})();
