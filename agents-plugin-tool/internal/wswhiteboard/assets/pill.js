;(function () {
  // Open-question pill. Items awaiting the user (open policy, assumed, ack)
  // scrolled out of the board's scroll area show a floating pill at that
  // edge, and the tab title carries their total. The dashboard re-runs this
  // after every board swap, so it tears down what the previous run attached.
  var d = document;
  if (d.body.classList.contains("embed") || !window.IntersectionObserver) return;
  var host = d.getElementById("bcol") || d.querySelector(".wrap");
  if (!host) return;
  var st = window.wbPill || (window.wbPill = {});
  if (st.off) st.off();

  var items = Array.prototype.slice.call(d.querySelectorAll('.card.policy[id^="item-"], .card.assumed[id^="item-"], .arow[id^="item-"]'));

  // The title prefix is the total, whatever is in view.
  var html = d.documentElement;
  var base = html.getAttribute("data-wb-title");
  if (base === null) { base = d.title.replace(/^\(\d+\) /, ""); html.setAttribute("data-wb-title", base); }
  d.title = (items.length ? "(" + items.length + ") " : "") + base;

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

  function show(edge, list, nearest) {
    var p = pills[edge];
    target[edge] = nearest;
    p.hidden = !list.length;
    if (!list.length) return;
    p.textContent = (edge === "top" ? "↑ " : "↓ ") + list.length + " open";
    p.title = edge === "top" ? "scroll to the nearest open item above" : "scroll to the nearest open item below";
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
    show("top", above, above[above.length - 1]);
    show("bot", below, below[0]);
    // Keep the bottom pill clear of the sticky reply bar.
    var reply = host.querySelector(".reply");
    wraps.bot.style.bottom = (reply && reply.offsetHeight ? reply.offsetHeight + 18 : 10) + "px";
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

  function click(edge) {
    return function () { if (target[edge]) target[edge].scrollIntoView({ block: "center", behavior: "smooth" }); };
  }
  pills.top.onclick = click("top");
  pills.bot.onclick = click("bot");

  st.off = function () {
    io.disconnect();
    if (ro) ro.disconnect();
    clearTimeout(timer);
    scroller.removeEventListener("scroll", later);
    window.removeEventListener("resize", later);
  };
})();
