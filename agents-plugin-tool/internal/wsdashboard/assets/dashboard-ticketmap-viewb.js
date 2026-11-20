  // --- ticket map View B: the swimlane matrix and click-focus --------------
  // A view on the ticket map's host contract (dashboard-ticketmap.js). This
  // placeholder lists each shown cluster's tickets and a minimal focus with
  // Back and Full View; the swimlane matrix, the 3-tier hierarchy, the
  // semantic grid, and the inspector replace it.
  ticketMap.viewB = (function () {
    var tm = ticketMap, box = null, list = null, focusEl = null, sel = null;
    function mount(el) {
      box = el;
      box.addEventListener("click", function (e) {
        var t = e.target, c = t.closest && t.closest("[data-i]");
        if (c && box.contains(c)) tm.select(+c.getAttribute("data-i"));
      });
    }
    function render() {
      var g = tm.data();
      box.textContent = "";
      sel = null;
      focusEl = null;
      list = tm.el("div", "d-tm-ph");
      g.clusters.forEach(function (c) {
        var lane = tm.hue(tm.el("section", "d-tm-ph-lane"), c);
        lane.setAttribute("data-cl", c.id);
        lane.appendChild(tm.el("h4", "", tm.clusterLabel(c)));
        g.nodes.forEach(function (n) {
          if (n.cluster !== c.id) return;
          var b = tm.button("d-tm-ph-card");
          b.setAttribute("data-i", n.i);
          b.appendChild(tm.statusDot(n));
          b.appendChild(tm.el("span", "", n.title));
          lane.appendChild(b);
        });
        list.appendChild(lane);
      });
      box.appendChild(list);
      filter();
    }
    function filter() {
      if (!list) return;
      var g = tm.data();
      Array.prototype.forEach.call(list.children, function (lane) {
        var any = false;
        Array.prototype.forEach.call(lane.querySelectorAll("[data-i]"), function (b) {
          var n = g.nodes[+b.getAttribute("data-i")], on = tm.shown(n);
          b.hidden = !on;
          b.classList.toggle("d-tm-ph-hit", tm.isHit(n.i));
          if (on) any = true;
        });
        lane.hidden = !any;
      });
    }
    function focus(i) {
      var n = tm.data().nodes[i];
      sel = i;
      if (list && list.parentNode) box.removeChild(list);
      if (focusEl) box.removeChild(focusEl);
      focusEl = tm.el("div", "d-tm-ph-focus");
      var bar = tm.el("div", "d-tm-ph-bar");
      var bk = tm.button("btn d-tm-btn", wsT("js.tmap.focus.back"));
      bk.appendChild(tm.kbd("Esc"));
      bk.addEventListener("click", function () { tm.unselect(); });
      bar.appendChild(bk);
      var full = tm.button("btn d-tm-btn", wsT("js.tmap.focus.full"));
      full.addEventListener("click", function () { tm.openFull(i); });
      bar.appendChild(full);
      focusEl.appendChild(bar);
      focusEl.appendChild(tm.el("h3", "", n.title));
      focusEl.appendChild(tm.el("code", "", n.stem));
      box.appendChild(focusEl);
    }
    function unfocus() {
      sel = null;
      if (focusEl) { box.removeChild(focusEl); focusEl = null; }
      if (list && !list.parentNode) box.appendChild(list);
    }
    function isFocus() { return sel !== null; }
    return { mount: mount, render: render, filter: filter, focus: focus, unfocus: unfocus, isFocus: isFocus };
  })();
