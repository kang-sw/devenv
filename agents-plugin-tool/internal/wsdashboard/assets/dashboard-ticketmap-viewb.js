  // --- ticket map View B: the swimlane matrix and click-focus --------------
  // A view on the ticket map's host contract (dashboard-ticketmap.js). The
  // matrix shows one lane per cluster and one column per status (closed refs,
  // Ready, Todo, Idea) of excerpt-free cards. A card click focuses its ticket:
  // the matrix leaves the DOM and the 3-tier hierarchy takes its place
  // (upstream parent and blocked-by, the focal card with its related column,
  // downstream children and blocks) with measured SVG connectors, then the
  // semantic-similarity grid. The inspector opens only on its toggle.
  //
  // Test hooks the browser test reads: .d-tm-card[data-i] (children exactly
  // .d-tm-ctop and .d-tm-ctt), #tmhero, #tmlat, .d-tm-xc[data-pivot][data-g],
  // <svg id="tmhwire"> paths .d-tm-w[data-from, data-fs, data-to, data-ts,
  // data-g] (from/to are node indexes or "focal"), .d-tm-gmore (+N),
  // #tmsem, #tmsemth, #tmsemgrid, .d-tm-sc[data-pivot][data-pct],
  // .d-tm-lk-old, #tmfinsp, #tmfback, #tmfprev, #tmfnext, [data-hist].
  ticketMap.viewB = (function () {
    // tm:hier-begin. Pure: layoutHier's column solver, no DOM, so the node
    // test (testdata/ticketmap-hier.test.cjs) runs this block as is.
    var HIER_CW = 232, HIER_GAP = 16, HIER_GG = 40, HIER_CAP = 99;
    // tmHierCols fits one tier's groups into W1, the measured width of the
    // hierarchy's first grid column; ns holds each group's visible card
    // count (capped at 99). Every group starts at one column of width cw;
    // the group with the most rows grows a column while the tier still fits,
    // then each group rebalances to ceil(n / ceil(n / cols)) columns, so five
    // cards become 3+2 rather than 4+1.
    function tmHierCols(W1, ns) {
      var g = Math.max(1, ns.length);
      var n = ns.map(function (k) { return Math.max(0, Math.min(HIER_CAP, Math.floor(k) || 0)); });
      var cw = Math.max(1, Math.min(HIER_CW, Math.floor((W1 - HIER_GG * (g - 1)) / g)));
      var c = n.map(function () { return 1; });
      function width() {
        var s = HIER_GG * (g - 1);
        c.forEach(function (k) { s += k * cw + (k - 1) * HIER_GAP; });
        return s;
      }
      for (;;) {
        var best = -1, rows = 0;
        for (var ix = 0; ix < n.length; ix++) {
          var r = Math.ceil(n[ix] / c[ix]);
          if (c[ix] < n[ix] && r >= rows) { rows = r; best = ix; }
        }
        if (best < 0) break;
        c[best]++;
        if (width() > W1) { c[best]--; break; }
      }
      c = c.map(function (k, ix) { return n[ix] > k ? Math.ceil(n[ix] / Math.ceil(n[ix] / k)) : Math.max(1, Math.min(k, n[ix])); });
      return { cw: cw, cols: c };
    }
    // tm:hier-end

    var tm = ticketMap, SVGNS = "http://www.w3.org/2000/svg";
    var CAP = { "parent": 4, "blocked-by": 4, "child": 8, "blocks": 4, "related": 4 };
    var MARK = { "parent": "tmmkp", "child": "tmmkp", "blocked-by": "tmmkw", "blocks": "tmmkw", "related": "tmmkr" };
    var REFCAP = 5, COLS = ["ref", "ready", "todo", "idea"];
    var box = null, mx = null, cards = [], lanes = [], hovI = null, scrollMem = 0;
    var fx = null, cur = null, F = {}, raf = 0;

    function el(tag, cls, text) { return tm.el(tag, cls, text); }
    function data() { return tm.data(); }
    function clusterOf(n) { var G = data(); return G && G.byCluster[n.cluster]; }
    function stOrd(s) { return tm.ST.indexOf(s); }
    function relOrd(r) { return tm.REL.indexOf(r); }
    function cmp(a, b) { return a < b ? -1 : a > b ? 1 : 0; }
    function shownEl(e) { return e.offsetParent !== null; }
    function phase(n) { return n.phases && n.phases[1] ? n.phases : null; }
    function phaseBar(ph, wide) {
      var w = el("span", wide ? "d-tm-phbar" : "d-tm-ph"), i = el("i"), b = el("b");
      b.style.width = Math.round(ph[0] / ph[1] * 100) + "%";
      i.appendChild(b);
      w.appendChild(i);
      w.appendChild(doc.createTextNode(ph[0] + "/" + ph[1]));
      w.title = wsT("js.tmap.ind.phase", ph[0], ph[1]);
      return w;
    }
    function kidsOf(m) {
      var k = 0;
      m.X.forEach(function (x) { if (x.rels.has("child")) k++; });
      return k;
    }

    function mount(e) {
      box = e;
      box.classList.add("d-tm-vb");
      box.addEventListener("click", onClick);
      box.addEventListener("pointerover", onOver);
      box.addEventListener("pointerout", onOut);
      if (window.ResizeObserver) new window.ResizeObserver(schedule).observe(box);
    }
    function onClick(e) {
      var t = e.target, c;
      if (!t || !t.closest) return;
      if ((c = t.closest("[data-lanes]"))) setAll(c.getAttribute("data-lanes") === "1");
      else if ((c = t.closest(".d-tm-lh"))) toggleLane(c.parentNode);
      else if ((c = t.closest(".d-tm-rmore"))) refMore(c.closest(".d-tm-lane"));
      else if ((c = t.closest(".d-tm-gmore"))) groupMore(c);
      else if ((c = t.closest("[data-hist]"))) tm.move(+c.getAttribute("data-hist") - tm.S.hp);
      else if ((c = t.closest("[data-pivot]"))) tm.select(+c.getAttribute("data-pivot"));
      else if ((c = t.closest("[data-i]"))) tm.select(+c.getAttribute("data-i"));
    }

    // --- the matrix -------------------------------------------------------
    // Card order in a cell: explicit degree, then phase total, then stem;
    // refs list done before dropped.
    function cardOrder(a, b) {
      return (b.deg - a.deg) || ((b.phases ? b.phases[1] : 0) - (a.phases ? a.phases[1] : 0)) || cmp(a.stem, b.stem);
    }
    function render(o) {
      o = o || {};
      var G = data(), keep = o.keepScroll ? (isFocus() ? scrollMem : box.scrollTop) : 0;
      // render leaves the focus mode; the host focuses again when it should.
      if (isFocus()) tm.inspShow(false);
      fx = null; cur = null; F = {};
      box.textContent = "";
      cards = []; lanes = []; hovI = null;
      if (!G) return;
      mx = el("div", "d-tm-mx");
      var by = {};
      G.nodes.forEach(function (n) {
        var l = by[n.cluster] || (by[n.cluster] = { ref: [], ready: [], todo: [], idea: [] });
        l[n.open ? n.status : "ref"].push(n);
      });
      var total = { ref: 0, ready: 0, todo: 0, idea: 0 };
      Object.keys(by).forEach(function (k) { COLS.forEach(function (col) { total[col] += by[k][col].length; }); });
      mx.appendChild(header(total));
      // Cluster order, with the Misc lane last.
      var order = G.clusters.slice().sort(function (a, b) { return (a.misc ? 1 : 0) - (b.misc ? 1 : 0); });
      order.forEach(function (c) { if (by[c.id]) mx.appendChild(lane(c, by[c.id])); });
      box.appendChild(mx);
      filter();
      box.scrollTop = keep;
      scrollMem = keep;
    }
    function colName(col) {
      return col === "ref" ? wsT("js.tmap.col.ref") : wsT(col === "ready" ? "js.tmap.st.ready" : col === "todo" ? "js.tmap.st.todo" : "js.tmap.st.idea");
    }
    function header(total) {
      var h = el("div", "d-tm-mhead"), tools = el("div", "d-tm-mtools");
      [["1", "js.tmap.lane.expandall"], ["0", "js.tmap.lane.collapseall"]].forEach(function (p) {
        var b = tm.button("d-tm-tbtn", wsT(p[1]));
        b.setAttribute("data-lanes", p[0]);
        tools.appendChild(b);
      });
      h.appendChild(tools);
      COLS.forEach(function (col) {
        var d = el("div", "d-tm-mcol d-tm-mcol-" + col);
        d.appendChild(el("span", "", colName(col)));
        d.appendChild(el("b", "", String(total[col])));
        h.appendChild(d);
      });
      return h;
    }
    function lane(c, by) {
      var S = tm.S, L = { el: tm.hue(el("section", "d-tm-lane"), c), nodes: [], dots: {}, xtra: [], more: null, id: c.id };
      L.el.setAttribute("data-cl", c.id);
      L.el.classList.toggle("d-tm-col", S.collapsed.has(c.id));
      var lh = tm.button("d-tm-lh");
      lh.setAttribute("aria-expanded", String(!S.collapsed.has(c.id)));
      var ln = el("span", "d-tm-ln");
      ln.appendChild(el("span", "d-tm-chev", "▼"));
      ln.appendChild(el("span", "d-tm-lname", tm.clusterLabel(c)));
      lh.appendChild(ln);
      var cnt = el("span", "d-tm-cnt");
      cnt.appendChild(el("span", "", wsT("js.tmap.lane.open", c.open || 0)));
      cnt.appendChild(el("span", "", wsT("js.tmap.lane.ref", c.ref || 0)));
      if (c.ready) cnt.appendChild(el("span", "d-tm-cr", wsT("js.tmap.lane.ready", c.ready)));
      lh.appendChild(cnt);
      var kw = el("span", "d-tm-lkw");
      (c.terms || []).slice(0, 6).forEach(function (t) { kw.appendChild(tm.chip(t[0], !!t[1])); });
      lh.appendChild(kw);
      L.el.appendChild(lh);
      by.ref.sort(function (a, b) { return (stOrd(a.status) - stOrd(b.status)) || cardOrder(a, b); });
      ["ready", "todo", "idea"].forEach(function (s) { by[s].sort(cardOrder); });
      COLS.forEach(function (col) {
        var list = by[col], cell = el("div", "d-tm-cell" + (col === "ref" ? " d-tm-cref" : ""));
        cell.setAttribute("data-col", col);
        cell.appendChild(el("span", "d-tm-ccap", colName(col)));
        list.forEach(function (n, ix) {
          var xtra = col === "ref" && ix >= REFCAP, cd = card(n, xtra);
          cards[n.i] = cd;
          L.nodes.push(n);
          if (xtra) L.xtra.push(n);
          cell.appendChild(cd);
        });
        if (!list.length) cell.appendChild(el("span", "d-tm-cempty", "—"));
        if (col === "ref" && list.length > REFCAP) {
          L.more = tm.button("d-tm-rmore");
          L.more.setAttribute("aria-expanded", "false");
          cell.appendChild(L.more);
        }
        var dots = el("span", "d-tm-dots");
        list.forEach(function (n) {
          var d = el("i", "d-tm-dot d-tm-st-" + n.status);
          d.setAttribute("data-i", n.i);
          d.title = n.title;
          L.dots[n.i] = d;
          dots.appendChild(d);
        });
        cell.appendChild(dots);
        L.el.appendChild(cell);
      });
      lanes.push(L);
      return L.el;
    }
    function indicators(n) {
      var ind = el("span", "d-tm-ind"), ph = phase(n), e;
      if (n.blockedOpen) { e = el("span", "d-tm-bk", "⛔"); e.title = wsT("js.tmap.ind.blocked"); ind.appendChild(e); }
      if (n.blockedHeading) { e = el("span", "d-tm-bk", "B!"); e.title = "## Blocked"; ind.appendChild(e); }
      if (n.odq) { e = el("span", "d-tm-odq", "ODQ"); e.title = "## Open Decision Queue"; ind.appendChild(e); }
      if (ph) ind.appendChild(phaseBar(ph, false));
      if (n.deg) { e = el("span", "", "↔" + n.deg); e.title = wsT("js.tmap.ind.deg", n.deg); ind.appendChild(e); }
      return ind;
    }
    // card is a matrix card: exactly the top row and the title, no excerpt.
    function card(n, xtra) {
      var c = tm.hue(tm.button("d-tm-card d-tm-st-" + n.status + (n.open ? "" : " d-tm-ref") + (xtra ? " d-tm-xtra" : "")), clusterOf(n));
      c.setAttribute("data-i", n.i);
      c.title = n.stem;
      var top = el("span", "d-tm-ctop");
      top.appendChild(tm.statusDot(n));
      top.appendChild(tm.catTag(n));
      top.appendChild(indicators(n));
      c.appendChild(top);
      c.appendChild(el("span", "d-tm-ctt", n.title));
      return c;
    }
    function filter() {
      if (!mx) return;
      var searching = !!tm.S.hits;
      mx.classList.toggle("d-tm-searching", searching);
      lanes.forEach(function (L) {
        var any = false, more = 0;
        L.nodes.forEach(function (n) {
          var on = tm.shown(n), hit = on && tm.isHit(n.i);
          cards[n.i].hidden = !on;
          cards[n.i].classList.toggle("d-tm-hit", hit);
          L.dots[n.i].hidden = !on;
          L.dots[n.i].classList.toggle("d-tm-hit", hit);
          if (on) any = true;
        });
        L.xtra.forEach(function (n) { if (tm.shown(n)) more++; });
        L.el.hidden = !any;
        if (L.more) {
          L.more.hidden = !more;
          L.more.textContent = L.el.classList.contains("d-tm-refall") ? wsT("js.tmap.less") : wsT("js.tmap.refs.more", more);
        }
      });
    }
    function toggleLane(laneEl) {
      var S = tm.S, id = +laneEl.getAttribute("data-cl"), on = !S.collapsed.has(id);
      if (on) S.collapsed.add(id); else S.collapsed.delete(id);
      laneEl.classList.toggle("d-tm-col", on);
      laneEl.querySelector(".d-tm-lh").setAttribute("aria-expanded", String(!on));
    }
    function setAll(open) {
      lanes.forEach(function (L) {
        if (open) tm.S.collapsed.delete(L.id); else tm.S.collapsed.add(L.id);
        L.el.classList.toggle("d-tm-col", !open);
        L.el.querySelector(".d-tm-lh").setAttribute("aria-expanded", String(open));
      });
    }
    function refMore(laneEl) {
      var on = laneEl.classList.toggle("d-tm-refall");
      laneEl.querySelector(".d-tm-rmore").setAttribute("aria-expanded", String(on));
      filter();
    }
    // Hover lights a card's explicit neighbours; touch has no hover.
    function onOver(e) {
      if (e.pointerType === "touch" || !mx || !mx.parentNode) return;
      var c = e.target.closest && e.target.closest(".d-tm-card");
      if (c) hover(+c.getAttribute("data-i"));
    }
    function onOut(e) {
      if (!mx || hovI === null) return;
      var c = e.target.closest && e.target.closest(".d-tm-card");
      if (c && !c.contains(e.relatedTarget)) hover(null);
    }
    function hover(i) {
      var G = data();
      if (i === hovI || !G) return;
      if (hovI !== null && G.nodes[hovI]) {
        if (cards[hovI]) cards[hovI].classList.remove("d-tm-hov");
        G.nodes[hovI].X.forEach(function (x, j) { if (cards[j]) cards[j].classList.remove("d-tm-lit"); });
      }
      hovI = i;
      if (mx) mx.classList.toggle("d-tm-hovering", i !== null);
      if (i === null || !cards[i]) return;
      cards[i].classList.add("d-tm-hov");
      G.nodes[i].X.forEach(function (x, j) { if (cards[j]) cards[j].classList.add("d-tm-lit"); });
    }

    // --- click-focus ------------------------------------------------------
    function isFocus() { return !!fx && !!fx.parentNode; }
    function focus(i) {
      var G = data(), n = G && G.nodes[i];
      if (!n) return;
      if (!isFocus()) scrollMem = box.scrollTop;
      hover(null);
      if (mx && mx.parentNode) box.removeChild(mx);
      if (fx && fx.parentNode) box.removeChild(fx);
      cur = i;
      fx = buildFocus(n);
      box.appendChild(fx);
      box.scrollTop = 0;
      if (tm.S.inspB) renderInsp();
      tm.inspShow(!!tm.S.inspB);
      layout();
    }
    function unfocus() {
      if (fx && fx.parentNode) box.removeChild(fx);
      fx = null; cur = null; F = {};
      tm.inspShow(false);
      if (mx && !mx.parentNode) box.appendChild(mx);
      box.scrollTop = scrollMem;
    }
    function groupsOf(n) {
      var G = data(), grp = { "parent": [], "blocked-by": [], "child": [], "blocks": [], "related": [] };
      n.X.forEach(function (x) { var b = tm.bucket(x.rels); if (b) grp[b].push(x); });
      Object.keys(grp).forEach(function (k) {
        grp[k].sort(function (a, b) {
          var p = G.nodes[a.j], q = G.nodes[b.j];
          return (stOrd(p.status) - stOrd(q.status)) || cmp(p.stem, q.stem);
        });
      });
      return grp;
    }
    // ancestors walks parent links up from the first parent: up to three
    // more, root first; the seen set stops a cycle at its repeat.
    function ancestors(n, grp) {
      var G = data(), out = [];
      if (!grp.parent.length) return out;
      var at = G.nodes[grp.parent[0].j], seen = new Set([n.i, at.i]);
      while (out.length < 3) {
        var up = null;
        at.X.forEach(function (x) { if (!up && x.rels.has("parent") && !seen.has(x.j)) up = x; });
        if (!up) break;
        at = G.nodes[up.j];
        seen.add(at.i);
        out.unshift(at);
      }
      return out;
    }
    function buildFocus(n) {
      var grp = groupsOf(n);
      F = { n: n, grp: grp };
      var root = tm.hue(el("div", "d-tm-focus"), clusterOf(n));
      root.appendChild(focusBar(n));
      var hier = el("div", "d-tm-hier");
      hier.id = "tmhier";
      F.hier = hier;
      hier.appendChild(wireSvg());
      F.lab = el("div", "d-tm-hlab");
      F.lab.setAttribute("aria-hidden", "true");
      hier.appendChild(F.lab);
      var hasUp = grp.parent.length + grp["blocked-by"].length > 0;
      var hasDown = grp.child.length + grp.blocks.length > 0, hasLat = grp.related.length > 0;
      if (hasUp) hier.appendChild(tier("up", ["parent", "blocked-by"], ancestors(n, grp)));
      F.hero = hero(n);
      hier.appendChild(F.hero);
      if (hasLat) {
        hier.classList.add("d-tm-has-lat");
        hier.style.setProperty("--d-tm-latrows", String((hasUp ? 1 : 0) + 1 + (hasDown ? 1 : 0)));
        F.lat = el("div", "d-tm-lat");
        F.lat.id = "tmlat";
        F.lat.setAttribute("data-tier", "lat");
        F.latc = el("div", "d-tm-latc");
        grp.related.forEach(function (x, ix) { F.latc.appendChild(xcard(x, "related", ix >= CAP.related)); });
        F.lat.appendChild(F.latc);
        if (grp.related.length > CAP.related) F.lat.appendChild(moreBtn(grp.related.length - CAP.related));
        hier.appendChild(F.lat);
      }
      if (hasDown) hier.appendChild(tier("down", ["child", "blocks"], null));
      if (!n.X.size) {
        var em = el("div", "d-tm-fempty");
        em.appendChild(el("span", "", "🔗"));
        var codes = el("span", "d-tm-fcodes");
        ["parent", "blocked-by", "related"].forEach(function (f, k) {
          if (k) codes.appendChild(doc.createTextNode(" · "));
          codes.appendChild(el("code", "", f));
        });
        em.appendChild(codes);
        em.appendChild(el("span", "", wsT("js.tmap.focus.nolinks")));
        hier.appendChild(em);
      }
      root.appendChild(hier);
      root.appendChild(semSection());
      return root;
    }
    function focusBar(n) {
      var S = tm.S, G = data(), bar = el("div", "d-tm-fbar");
      var bk = tm.button("btn d-tm-btn", "← " + wsT("js.tmap.focus.back"));
      bk.id = "tmfback";
      bk.appendChild(tm.kbd("Esc"));
      bk.addEventListener("click", function () { tm.unselect(); });
      bar.appendChild(bk);
      [["tmfprev", "←", "js.tmap.focus.prev", -1], ["tmfnext", "→", "js.tmap.focus.next", 1]].forEach(function (p) {
        var b = tm.button("btn d-tm-btn d-tm-icon", p[1]);
        b.id = p[0];
        b.title = wsT(p[2]) + " (" + p[1] + ")";
        b.setAttribute("aria-label", wsT(p[2]));
        b.disabled = p[3] < 0 ? S.hp <= 0 : S.hp >= S.hist.length - 1;
        b.addEventListener("click", function () { tm.move(p[3]); });
        bar.appendChild(b);
      });
      var trail = el("div", "d-tm-trail"), from = Math.max(0, S.hp - 4);
      S.hist.slice(from, S.hp + 1).forEach(function (h, k) {
        var m = G.nodes[h];
        if (!m) return;
        if (k) trail.appendChild(el("em", "", "›"));
        var t = tm.button("d-tm-trb" + (h === n.i ? " d-tm-trcur" : ""), m.title);
        t.setAttribute("data-hist", from + k);
        t.title = m.title;
        trail.appendChild(t);
      });
      bar.appendChild(trail);
      var ins = tm.button("btn d-tm-btn", wsT("js.tmap.insp"));
      ins.id = "tmfinsp";
      ins.setAttribute("aria-pressed", String(!!S.inspB));
      ins.addEventListener("click", function () {
        S.inspB = !S.inspB;
        ins.setAttribute("aria-pressed", String(S.inspB));
        if (S.inspB) renderInsp();
        tm.inspShow(S.inspB);
      });
      bar.appendChild(ins);
      return bar;
    }
    function hero(n) {
      var h = tm.hue(el("div", "d-tm-focal"), clusterOf(n)), main = el("div", "d-tm-fmain"), ph = phase(n);
      h.id = "tmhero";
      h.setAttribute("data-tier", "mid");
      main.appendChild(el("div", "d-tm-eyebrow", wsT("js.tmap.focus.eyebrow")));
      main.appendChild(el("h2", "d-tm-htitle", n.title));
      main.appendChild(el("div", "d-tm-stem", n.stem));
      var meta = el("div", "d-tm-hmeta");
      meta.appendChild(tm.statusBadge(n));
      meta.appendChild(tm.clusterPill(n.cluster));
      meta.appendChild(tm.catTag(n));
      if (ph) meta.appendChild(phaseBar(ph, true));
      if (n.blockedOpen) meta.appendChild(el("span", "d-tm-rp d-tm-rp-blocked-by", "⛔ " + wsT("js.tmap.ind.blocked")));
      if (n.odq) meta.appendChild(el("span", "d-tm-rp d-tm-rp-odq", "ODQ"));
      meta.appendChild(el("span", "d-tm-tag", wsTn("js.tmap.full.lines", n.lines || 0)));
      if (n.missing && n.missing.length) {
        var mi = el("span", "d-tm-chip d-tm-chip-miss", wsTn("js.tmap.missing", n.missing.length));
        mi.title = n.missing.join("\n");
        meta.appendChild(mi);
      }
      main.appendChild(meta);
      var kw = el("div", "d-tm-kwr");
      (n.kw || []).slice(0, 6).forEach(function (t) { kw.appendChild(tm.chip(t[0], !!t[1])); });
      main.appendChild(kw);
      h.appendChild(main);
      var acts = el("div", "d-tm-acts");
      // The View C hand-offs: the constellation keeps the selection; the
      // Ego Ring opens there around this ticket.
      var toc = tm.button("btn d-tm-btn", wsT("js.tmap.focus.toc"));
      toc.id = "tmftoc";
      toc.hidden = !tm.graph;
      toc.addEventListener("click", function () { tm.setView("C"); });
      var ego = tm.button("btn d-tm-btn", wsT("js.tmap.focus.ego"));
      ego.id = "tmfego";
      ego.hidden = !tm.graph;
      ego.addEventListener("click", function () { tm.setView("C"); if (tm.S.view === "C") tm.graph.enterEgo(n.i); });
      var full = tm.button("btn d-tm-btn", wsT("js.tmap.focus.full"), "file-text");
      full.id = "tmffull";
      full.addEventListener("click", function () { tm.openFull(n.i); });
      acts.appendChild(toc);
      acts.appendChild(ego);
      acts.appendChild(full);
      h.appendChild(acts);
      return h;
    }
    function tier(kind, keys, anc) {
      var t = el("div", "d-tm-tier d-tm-" + kind);
      t.setAttribute("data-tier", kind);
      if (anc && anc.length) {
        var a = el("div", "d-tm-anc");
        a.appendChild(el("span", "", "⤒"));
        anc.forEach(function (m, k) {
          if (k) a.appendChild(el("em", "", "›"));
          var b = tm.button("d-tm-ancb");
          b.setAttribute("data-pivot", m.i);
          b.title = m.stem;
          b.appendChild(tm.catTag(m));
          b.appendChild(el("span", "", m.title));
          a.appendChild(b);
        });
        a.appendChild(el("em", "", "›"));
        t.appendChild(a);
      }
      var row = el("div", "d-tm-grps");
      keys.forEach(function (k) {
        var list = F.grp[k];
        if (!list.length) return;
        var gr = el("div", "d-tm-grp"), gc = el("div", "d-tm-gcards");
        gr.setAttribute("data-g", k);
        list.forEach(function (x, ix) { gc.appendChild(xcard(x, k, ix >= CAP[k])); });
        gr.appendChild(gc);
        if (list.length > CAP[k]) gr.appendChild(moreBtn(list.length - CAP[k]));
        row.appendChild(gr);
      });
      t.appendChild(row);
      return t;
    }
    function moreBtn(k) {
      var b = tm.button("btn d-tm-btn d-tm-gmore", wsT("js.tmap.more", k));
      b.setAttribute("data-n", k);
      b.setAttribute("aria-expanded", "false");
      return b;
    }
    function groupMore(b) {
      var host = b.parentNode, on = host.classList.toggle("d-tm-all");
      b.textContent = on ? wsT("js.tmap.less") : wsT("js.tmap.more", +b.getAttribute("data-n"));
      b.setAttribute("aria-expanded", String(on));
      layout();
    }
    // xcard is a hierarchy card: relation pills, status, the cluster when
    // it differs, similarity, children, the clamped title, and for related
    // tickets one relation-note line.
    function xcard(x, k, xtra) {
      var G = data(), n = F.n, m = G.nodes[x.j];
      var c = tm.hue(tm.button("d-tm-xc" + (xtra ? " d-tm-xtra" : "")), clusterOf(m));
      c.setAttribute("data-pivot", m.i);
      c.setAttribute("data-g", k);
      var notes = x.notes.filter(function (t) { return t.note; });
      c.title = m.stem + notes.map(function (t) { return "\n· " + t.note; }).join("");
      var r1 = el("span", "d-tm-xr1");
      Array.from(x.rels).sort(function (a, b) { return relOrd(a) - relOrd(b); }).forEach(function (r) {
        r1.appendChild(el("span", "d-tm-rp d-tm-rp-" + r, r));
      });
      r1.appendChild(tm.statusBadge(m));
      if (m.cluster !== n.cluster) r1.appendChild(tm.clusterPill(m.cluster));
      var s = tm.simOf(n.i, m.i);
      if (s >= 0.08) {
        var sp = el("span", "d-tm-tag", "≈ " + Math.round(s * 100) + "%");
        sp.title = wsT("js.tmap.sim.title");
        r1.appendChild(sp);
      }
      var kids = k === "parent" || k === "blocked-by" ? 0 : kidsOf(m);
      if (kids) {
        var kd = el("span", "d-tm-kids", "↓" + kids);
        kd.title = wsT("js.tmap.ind.kids", kids);
        r1.appendChild(kd);
      }
      c.appendChild(r1);
      c.appendChild(el("span", "d-tm-xtt", m.title));
      if (k === "related" && notes.length) {
        var nl = el("span", "d-tm-xnote");
        nl.appendChild(el("em", "", wsT(notes[0].side === "out" ? "js.tmap.note.out" : "js.tmap.note.in")));
        nl.appendChild(doc.createTextNode(notes[0].note));
        c.appendChild(nl);
      }
      return c;
    }

    // --- layout and connectors ------------------------------------------
    function schedule() {
      if (raf) return;
      raf = requestAnimationFrame(function () { raf = 0; layout(); });
    }
    // layout is layoutHier: fit each tier's card columns to the measured
    // first grid column, then route the connectors.
    function layout() {
      if (!isFocus() || !F.hier) return;
      var hier = F.hier, hr = hier.getBoundingClientRect();
      if (!hr.width) return;
      var cols = getComputedStyle(hier).gridTemplateColumns.split(" ").map(parseFloat);
      var W1 = cols[0] || hier.clientWidth;
      Array.prototype.forEach.call(hier.querySelectorAll(".d-tm-tier .d-tm-grps"), function (row) {
        var grs = Array.prototype.slice.call(row.children);
        var ns = grs.map(function (gr) {
          var all = gr.classList.contains("d-tm-all");
          return Array.prototype.filter.call(gr.querySelectorAll(".d-tm-xc"), function (c) { return all || !c.classList.contains("d-tm-xtra"); }).length;
        });
        var r = tmHierCols(W1, ns);
        grs.forEach(function (gr, ix) {
          var gc = gr.querySelector(".d-tm-gcards");
          gc.style.setProperty("--d-tm-gc", String(r.cols[ix]));
          gc.style.setProperty("--d-tm-cw", r.cw + "px");
        });
      });
      wires();
    }
    function wireSvg() {
      var svg = doc.createElementNS(SVGNS, "svg");
      svg.id = "tmhwire";
      svg.setAttribute("class", "d-tm-hwire");
      svg.setAttribute("aria-hidden", "true");
      var defs = doc.createElementNS(SVGNS, "defs");
      [["tmmkp", "d-tm-mkp", 9], ["tmmkw", "d-tm-mkw", 10], ["tmmkr", "d-tm-mkr", 7]].forEach(function (p) {
        var m = doc.createElementNS(SVGNS, "marker"), dot = p[0] === "tmmkr";
        m.id = p[0];
        m.setAttribute("viewBox", "0 0 10 10");
        m.setAttribute("refX", dot ? "5" : "9");
        m.setAttribute("refY", "5");
        m.setAttribute("markerWidth", String(p[2]));
        m.setAttribute("markerHeight", String(p[2]));
        m.setAttribute("markerUnits", "userSpaceOnUse");
        if (!dot) m.setAttribute("orient", "auto");
        var s = doc.createElementNS(SVGNS, dot ? "circle" : "path");
        if (dot) { s.setAttribute("cx", "5"); s.setAttribute("cy", "5"); s.setAttribute("r", "4"); } else s.setAttribute("d", "M0 1L10 5L0 9z");
        s.setAttribute("class", p[1]);
        m.appendChild(s);
        defs.appendChild(m);
      });
      svg.appendChild(defs);
      F.svg = svg;
      F.paths = doc.createElementNS(SVGNS, "g");
      svg.appendChild(F.paths);
      return svg;
    }
    // rounded draws an orthogonal polyline with corners of radius <= 7.
    function rounded(pts) {
      function f(v) { return v.toFixed(1); }
      var d = "M" + f(pts[0][0]) + "," + f(pts[0][1]);
      for (var i = 1; i < pts.length - 1; i++) {
        var px = pts[i - 1][0], py = pts[i - 1][1], x = pts[i][0], y = pts[i][1], nx = pts[i + 1][0], ny = pts[i + 1][1];
        var l1 = Math.hypot(x - px, y - py), l2 = Math.hypot(nx - x, ny - y);
        if (l1 < 0.5 || l2 < 0.5) continue;
        var r = Math.min(7, l1 / 2, l2 / 2);
        d += " L" + f(x - (x - px) / l1 * r) + "," + f(y - (y - py) / l1 * r) + " Q" + f(x) + "," + f(y) + " " + f(x + (nx - x) / l2 * r) + "," + f(y + (ny - y) / l2 * r);
      }
      var L = pts[pts.length - 1];
      return d + " L" + f(L[0]) + "," + f(L[1]);
    }
    function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
    // wires re-measures every card against the hierarchy box and redraws
    // the connectors and their labels.
    function wires() {
      var hier = F.hier, hr = hier.getBoundingClientRect(), svg = F.svg;
      svg.setAttribute("width", String(hr.width));
      svg.setAttribute("height", String(hr.height));
      svg.setAttribute("viewBox", "0 0 " + hr.width + " " + hr.height);
      F.paths.textContent = "";
      F.lab.textContent = "";
      function R(e) {
        var r = e.getBoundingClientRect();
        return { l: r.left - hr.left, t: r.top - hr.top, r: r.right - hr.left, b: r.bottom - hr.top, cx: (r.left + r.right) / 2 - hr.left, cy: (r.top + r.bottom) / 2 - hr.top };
      }
      function add(pts, g, from, fs, to, ts) {
        var p = doc.createElementNS(SVGNS, "path");
        p.setAttribute("class", "d-tm-w d-tm-w-" + g);
        p.setAttribute("d", rounded(pts));
        p.setAttribute("marker-end", "url(#" + MARK[g] + ")");
        p.setAttribute("data-g", g);
        p.setAttribute("data-from", from);
        p.setAttribute("data-fs", fs);
        p.setAttribute("data-to", to);
        p.setAttribute("data-ts", ts);
        F.paths.appendChild(p);
      }
      function label(g, x, y, text, count) {
        var s = el("span", "d-tm-wl d-tm-wl-" + g, text);
        if (count) s.appendChild(el("b", "", String(count)));
        s.style.transform = "translate(" + x.toFixed(1) + "px," + y.toFixed(1) + "px)";
        F.lab.appendChild(s);
      }
      function visible(scope) {
        return Array.prototype.filter.call(scope.querySelectorAll(".d-tm-xc"), shownEl).map(function (c) { return [c, R(c)]; });
      }
      var Fr = R(F.hero), grp = F.grp;
      function trunkX(gR, count, gi) {
        var lo = Math.max(gR.l, Fr.l + 28), hi = Math.min(gR.r, Fr.r - 28), pref = count === 1 ? Fr.cx : gR.cx;
        return lo <= hi ? clamp(pref, lo, hi) : clamp(pref, Fr.l + 28 + gi * 40, Fr.r - 28);
      }
      var name = {
        "parent": ["↓ parent", 2], "blocked-by": ["↓ blocked-by", 2],
        "child": ["↓ " + wsT("js.tmap.rel.children"), 1], "blocks": ["↓ " + wsT("js.tmap.rel.blocks"), 1]
      };
      // Tier 1: each card's bottom (left edge on an upper row) down to a
      // bus midway to the focal card, along it to the trunk, into the top.
      var ups = Array.prototype.slice.call(hier.querySelectorAll(".d-tm-up .d-tm-grp"));
      if (ups.length) {
        var upc = ups.map(visible), tierB = -Infinity;
        upc.forEach(function (l) { l.forEach(function (p) { tierB = Math.max(tierB, p[1].b); }); });
        var busU = (tierB + Fr.t) / 2;
        ups.forEach(function (gr, gi) {
          var k = gr.getAttribute("data-g"), tx = trunkX(R(gr), ups.length, gi), lastB = -Infinity;
          upc[gi].forEach(function (p) { lastB = Math.max(lastB, p[1].b); });
          upc[gi].forEach(function (p) {
            var c = p[0].getAttribute("data-pivot"), r = p[1];
            if (Math.abs(r.b - lastB) < 3) add([[r.cx, r.b], [r.cx, busU], [tx, busU], [tx, Fr.t]], k, c, "bottom", "focal", "top");
            else add([[r.l, r.cy], [r.l - 8, r.cy], [r.l - 8, busU], [tx, busU], [tx, Fr.t]], k, c, "left", "focal", "top");
          });
          var n = grp[k].length;
          label(k, tx + 7, (busU + Fr.t) / 2 - 8.5, name[k][0], n >= name[k][1] ? n : 0);
        });
      }
      // Related: beside the focal card, from its right edge to a dashed bus
      // 26px left of the column, then into each card's left edge; below it
      // (narrow), from its bottom like Tier 3.
      var latB = Fr.b;
      if (F.lat) {
        var rs = visible(F.lat), L = R(F.latc), nrel = grp.related.length;
        if (L.l >= Fr.r - 1) {
          var busX = L.l - 26;
          rs.forEach(function (p) {
            add([[Fr.r, Fr.cy], [busX, Fr.cy], [busX, p[1].cy], [p[1].l, p[1].cy]], "related", "focal", "right", p[0].getAttribute("data-pivot"), "left");
          });
          label("related", Fr.r + 8, Fr.cy - 25, "↔ related ", nrel);
        } else if (rs.length) {
          // Tier 3's bus clears the whole related row, its +N included.
          latB = R(F.lat).b;
          var busR = downWires(rs, [rs], "related", Fr.l + 28, Fr.b);
          label("related", Fr.l + 35, (Fr.b + busR) / 2 - 8.5, "↔ related ", nrel);
        }
      }
      // Tier 3 mirrors Tier 1: the focal bottom down the trunk to a bus,
      // then into each card's top (left edge on a later row).
      var dns = Array.prototype.slice.call(hier.querySelectorAll(".d-tm-down .d-tm-grp")), dnc = dns.map(visible);
      dns.forEach(function (gr, gi) {
        var k = gr.getAttribute("data-g"), tx = trunkX(R(gr), dns.length, gi);
        var bus = downWires(dnc[gi], dnc, k, tx, latB);
        label(k, tx + 7, (latB + bus) / 2 - 8.5, name[k][0] + " ", grp[k].length);
      });
      // downWires routes one group from the focal bottom: the bus sits
      // midway between fromB and the top of the tier (all its groups).
      function downWires(rs, all, k, tx, fromB) {
        var tierT = Infinity, firstT = Infinity;
        all.forEach(function (l) { l.forEach(function (p) { tierT = Math.min(tierT, p[1].t); }); });
        rs.forEach(function (p) { firstT = Math.min(firstT, p[1].t); });
        var bus = (fromB + tierT) / 2;
        rs.forEach(function (p) {
          var c = p[0].getAttribute("data-pivot"), r = p[1];
          if (Math.abs(r.t - firstT) < 3) add([[tx, Fr.b], [tx, bus], [r.cx, bus], [r.cx, r.t]], k, "focal", "bottom", c, "top");
          else add([[tx, Fr.b], [tx, bus], [r.l - 8, bus], [r.l - 8, r.cy], [r.l, r.cy]], k, "focal", "bottom", c, "left");
        });
        return bus;
      }
    }

    // --- the semantic section ---------------------------------------------
    function semSection() {
      var S = tm.S, sec = el("section", "d-tm-sem"), head = el("div", "d-tm-semhead");
      sec.id = "tmsem";
      var h = el("h3", "d-tm-semt");
      h.appendChild(el("span", "", wsT("js.tmap.sim.title")));
      h.appendChild(el("span", "d-tm-semsub", wsT("js.tmap.sim.sub")));
      F.semn = el("small", "d-tm-semn", "0");
      h.appendChild(F.semn);
      head.appendChild(h);
      head.appendChild(el("span", "d-tm-spacer"));
      var ctl = el("label", "d-tm-semctl"), th = el("input");
      ctl.appendChild(el("span", "", wsT("js.tmap.sim.th")));
      th.type = "range";
      th.id = "tmsemth";
      th.min = "10"; th.max = "60"; th.step = "1";
      th.value = String(S.simTh);
      F.semo = el("output", "d-tm-semo", S.simTh + "%");
      th.addEventListener("input", function () {
        S.simTh = +th.value;
        F.semo.textContent = S.simTh + "%";
        tm.savePrefs();
        renderSem();
      });
      ctl.appendChild(th);
      ctl.appendChild(F.semo);
      head.appendChild(ctl);
      var tg = el("div", "d-tm-tg"), modes = [];
      tg.setAttribute("role", "group");
      [[false, "js.tmap.sim.all"], [true, "js.tmap.sim.unlinked"]].forEach(function (p) {
        var b = tm.button("d-tm-tgb", wsT(p[1]));
        b.id = p[0] ? "tmsemunl" : "tmsemall";
        b.setAttribute("aria-pressed", String(S.simUnlinked === p[0]));
        b.addEventListener("click", function () {
          S.simUnlinked = p[0];
          modes.forEach(function (o) { o.setAttribute("aria-pressed", String(o === b)); });
          tm.savePrefs();
          renderSem();
        });
        modes.push(b);
        tg.appendChild(b);
      });
      head.appendChild(tg);
      var chk = el("label", "d-tm-chk"), cb = el("input");
      cb.type = "checkbox";
      cb.id = "tmsemrefs";
      cb.checked = !!S.simRefs;
      cb.addEventListener("change", function () { S.simRefs = cb.checked; tm.savePrefs(); renderSem(); });
      chk.appendChild(cb);
      chk.appendChild(el("span", "", wsT("js.tmap.sim.refs")));
      head.appendChild(chk);
      sec.appendChild(head);
      F.semgrid = el("div", "d-tm-semgrid");
      F.semgrid.id = "tmsemgrid";
      sec.appendChild(F.semgrid);
      renderSem();
      return sec;
    }
    // renderSem lists the focal ticket's similar tickets at or above the
    // threshold, most similar first; a click pivots.
    function renderSem() {
      var G = data(), n = F.n, S = tm.S;
      if (!n || !F.semgrid) return;
      var list = n.sim.filter(function (s) {
        return s.s * 100 >= S.simTh - 1e-9 && (S.simRefs || G.nodes[s.j].open) && (!S.simUnlinked || !n.X.has(s.j));
      }).sort(function (a, b) { return b.s - a.s; });
      F.semn.textContent = String(list.length);
      F.semgrid.textContent = "";
      if (!list.length) {
        F.semgrid.appendChild(el("div", "d-tm-fempty", wsT("js.tmap.sim.none", S.simTh)));
        return;
      }
      list.forEach(function (s) {
        var m = G.nodes[s.j], x = n.X.get(s.j), p = Math.round(s.s * 100);
        var c = tm.hue(tm.button("d-tm-sc"), clusterOf(m));
        c.setAttribute("data-pivot", m.i);
        c.setAttribute("data-pct", String(p));
        c.style.setProperty("--d-tm-p", String(p));
        c.title = m.stem;
        var pct = el("span", "d-tm-pct"), b = el("b", "", String(p)), bar = el("i"), u = el("u");
        b.appendChild(el("small", "", "%"));
        u.style.width = p + "%";
        bar.appendChild(u);
        pct.appendChild(b);
        pct.appendChild(bar);
        c.appendChild(pct);
        var body = el("span", "d-tm-scm"), r1 = el("span", "d-tm-sr1"), chips = el("span", "d-tm-schips");
        r1.appendChild(tm.statusBadge(m));
        r1.appendChild(tm.clusterPill(m.cluster));
        r1.appendChild(x ? el("span", "d-tm-lk d-tm-lk-old", wsT("js.tmap.sim.old", Array.from(x.rels).join("/"))) : el("span", "d-tm-lk d-tm-lk-new", wsT("js.tmap.sim.new")));
        body.appendChild(r1);
        body.appendChild(el("span", "d-tm-stt", m.title));
        if (s.men) chips.appendChild(el("span", "d-tm-chip d-tm-chip-men", wsT("js.tmap.sim.mention")));
        (s.terms || []).forEach(function (t) { chips.appendChild(tm.chip(t[0], !!t[1])); });
        body.appendChild(chips);
        c.appendChild(body);
        F.semgrid.appendChild(c);
      });
    }

    // --- the inspector (opt-in in focus) ------------------------------------
    // renderInsp fills the inspector for node i (default: the focal one);
    // View C calls it too, for its selection.
    function renderInsp(i) {
      if (i === undefined) i = cur;
      var body = tm.insp(), G = data(), n = G && i !== null ? G.nodes[i] : null, ph;
      if (!body || !n) return;
      body.textContent = "";
      var hd = tm.hue(el("div", "d-tm-ihd"), clusterOf(n));
      hd.appendChild(el("h3", "d-tm-ititle", n.title));
      hd.appendChild(el("div", "d-tm-stem", n.stem));
      body.appendChild(hd);
      var kv = el("dl", "d-tm-kv");
      function row(key, v) {
        kv.appendChild(el("dt", "", wsT(key)));
        var dd = el("dd");
        if (typeof v === "string") dd.textContent = v; else dd.appendChild(v);
        kv.appendChild(dd);
      }
      row("js.tmap.insp.status", tm.statusBadge(n));
      row("js.tmap.insp.cluster", tm.clusterPill(n.cluster));
      row("js.tmap.insp.cat", tm.catTag(n));
      row("js.tmap.insp.phase", (ph = phase(n)) ? phaseBar(ph, true) : "—");
      var flags = [];
      if (n.blockedOpen) flags.push("⛔ " + wsT("js.tmap.ind.blocked"));
      if (n.blockedHeading) flags.push("## Blocked");
      if (n.odq) flags.push("ODQ");
      row("js.tmap.insp.flags", flags.join(" · ") || "—");
      row("js.tmap.insp.assignee", (n.assignee || []).join(", ") || "—");
      row("js.tmap.insp.size", wsTn("js.tmap.full.lines", n.lines || 0));
      body.appendChild(kv);
      function sec(key, count) {
        var s = el("div", "d-tm-isec", wsT(key));
        s.appendChild(el("b", "", String(count)));
        body.appendChild(s);
      }
      function go(j, cls) {
        var a = tm.button("d-tm-il" + (cls ? " " + cls : ""));
        a.addEventListener("click", function () { tm.select(j); });
        return a;
      }
      var links = Array.from(n.X.values()).sort(function (a, b) {
        return Math.min.apply(null, Array.from(a.rels).map(relOrd)) - Math.min.apply(null, Array.from(b.rels).map(relOrd)) || cmp(G.nodes[a.j].stem, G.nodes[b.j].stem);
      });
      sec("js.tmap.insp.links", links.length);
      var il = el("div", "d-tm-ilist");
      links.forEach(function (x) {
        var a = tm.hue(go(x.j), clusterOf(G.nodes[x.j]));
        Array.from(x.rels).sort(function (p, q) { return relOrd(p) - relOrd(q); }).forEach(function (r) { a.appendChild(el("span", "d-tm-rp d-tm-rp-" + r, r)); });
        a.appendChild(el("span", "d-tm-ilt", G.nodes[x.j].title));
        a.title = G.nodes[x.j].stem;
        il.appendChild(a);
      });
      if (!links.length) il.appendChild(el("span", "d-tm-inone", wsT("js.tmap.insp.none")));
      body.appendChild(il);
      var sims = n.sim.slice(0, 8);
      sec("js.tmap.insp.similar", sims.length);
      var sl = el("div", "d-tm-ilist");
      sims.forEach(function (s) {
        var a = go(s.j);
        a.appendChild(el("span", "d-tm-ipc", Math.round(s.s * 100) + "%"));
        a.appendChild(el("span", "d-tm-ilt", G.nodes[s.j].title));
        a.title = G.nodes[s.j].stem;
        sl.appendChild(a);
      });
      if (!sims.length) sl.appendChild(el("span", "d-tm-inone", wsT("js.tmap.insp.none")));
      body.appendChild(sl);
      var full = tm.button("btn d-tm-btn", wsT("js.tmap.focus.full"), "file-text");
      full.addEventListener("click", function () { tm.openFull(n.i); });
      body.appendChild(full);
    }

    // show follows the shared selection (it may have changed in View C):
    // focus it, or leave the focus mode when it is gone.
    function show() {
      var sel = tm.S.sel;
      if (sel !== null && (!isFocus() || cur !== sel)) { focus(sel); return; }
      if (sel === null && isFocus()) { unfocus(); return; }
      if (!isFocus()) return;
      if (tm.S.inspB) renderInsp();
      tm.inspShow(!!tm.S.inspB);
      layout();
    }
    function hide() {
      hover(null);
      if (isFocus()) tm.inspShow(false);
    }
    return { mount: mount, render: render, filter: filter, focus: focus, unfocus: unfocus, isFocus: isFocus, show: show, hide: hide, layout: layout, insp: renderInsp };
  })();
