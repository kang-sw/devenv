  // --- ticket map View C: the constellation, Keyword Soup and Ego Ring -----
  // A view on the ticket map's host contract (dashboard-ticketmap.js), its
  // math in ticketMap.geom (dashboard-ticketmap-geom.js, right before this
  // file). One deterministic, seeded star layout per payload, grouped into
  // cluster hulls, under a camera {x, y, k} with a 3-stage semantic zoom:
  // far, each cluster's Keyword Soup at its tickets' centroid and no ticket
  // title; mid, the soup fades as hub titles fade in; close, borderless
  // titles. Wheel zoom keeps the point under the cursor, any drag pans,
  // Alt+drag moves a star, and the Ego Ring lays a ticket's links and
  // similar tickets around it.
  //
  // Test hooks: S.cam (the live camera) and S.ego on window.__tmap, and
  // window.__tmapc (probe, below); .d-tm-cnd[data-i] stars (.d-tm-kwh,
  // .d-tm-hvn, .d-tm-qh), .d-tm-ced edges, #tmlbl's .d-tm-lb[data-i],
  // .d-tm-cb, .d-tm-sk[data-j] and .d-tm-sect (.on when placed), .d-tm-calm
  // on #tmsvg, #tmctool (#tmsimth, #tmegob, #tmckw), #tmchint, #tmegobar,
  // #tmlegend, .d-tm-zoom (#tmzrd) and #tmtip (.on).
  ticketMap.graph = (function () {
    var tm = ticketMap, geo = ticketMap.geom, S = ticketMap.S, SVGNS = "http://www.w3.org/2000/svg";
    var ST_KEY = { ready: "js.tmap.st.ready", todo: "js.tmap.st.todo", idea: "js.tmap.st.idea", done: "js.tmap.st.done", dropped: "js.tmap.st.dropped" };
    var PRIO = { ready: 6e3, todo: 2.5e3, idea: 1.2e3 };
    var box = null, vp = null, svg = null, world = null, lbl = null, tipEl = null, lay = {}, U = {};
    var G = null, P = [], Bp = [], R = [], gN = [], lbs = [], cbs = [], soup = [], soupOrder = [], sects = [];
    var edges = [], sims = [], egoEdges = [], hullPoly = {}, hullTop = {}, hovMarked = [];
    var cam = S.cam, W = 0, H = 0, frame = 0, anim = 0, tween = 0, ns = 1, ready = false, fitted = false;
    var stemKey = "", camFor = null, layoutMs = 0, drag = null, lastClick = null, pins = new Map(), pinch = null;
    var hov = null, kwHover = null, kwSet = null, nb = new Set(), stg = 1, soupA = 1, nLabels = 0, nSoup = 0;
    var E = null, pinned = false, egoNext = false, clicking = false, keepCam = false;
    var wcache = new Map(), mctx = null, fam = "", mono = "";

    function el(tag, cls, text) { return tm.el(tag, cls, text); }
    function sv(tag, attrs) {
      var e = doc.createElementNS(SVGNS, tag);
      for (var k in attrs) if (Object.prototype.hasOwnProperty.call(attrs, k)) e.setAttribute(k, String(attrs[k]));
      return e;
    }
    function cl(id) { return G && G.byCluster[id]; }
    // vis is View C's filter: status and cluster. A search fades the rest
    // (.d-tm-qh) and a keyword highlights (.d-tm-kwh); neither hides.
    function vis(n) { return !!n && !!S.st[n.status] && !S.cl.has(n.cluster); }
    function shownI(i) { return vis(G.nodes[i]) && (!E || E.set.has(i)); }
    function tr(p) { return "translate(" + p.x.toFixed(1) + "," + p.y.toFixed(1) + ")"; }
    function cmp(a, b) { return a < b ? -1 : a > b ? 1 : 0; }

    // --- static DOM, built once ------------------------------------------
    // mk appends a new element to parent, with an id when given.
    function mk(parent, tag, cls, text, id) {
      var e = el(tag, cls, text);
      if (id) e.id = id;
      parent.appendChild(e);
      return e;
    }
    function btn(parent, cls, text, id, tip, fn) {
      var b = tm.button(cls, text);
      if (id) b.id = id;
      if (tip) { b.title = tip; b.setAttribute("aria-label", tip); }
      b.addEventListener("click", fn);
      parent.appendChild(b);
      return b;
    }
    function mount(e) {
      box = e;
      box.classList.add("d-tm-vc");
      vp = mk(box, "div", "d-tm-cvp", null, "tmvp");
      svg = sv("svg", { id: "tmsvg", "class": "d-tm-csvg", role: "img", "aria-label": wsT("js.tmap.c.aria") });
      var defs = sv("defs", {});
      [["tmarw", "d-tm-carw", 9], ["tmarwp", "d-tm-carwp", 8]].forEach(function (m) {
        var mk = sv("marker", { id: m[0], viewBox: "0 0 10 10", refX: 9, refY: 5, markerWidth: m[2], markerHeight: m[2], markerUnits: "userSpaceOnUse", orient: "auto" });
        mk.appendChild(sv("path", { d: "M0 1L10 5L0 9z", "class": m[1] }));
        defs.appendChild(mk);
      });
      svg.appendChild(defs);
      world = sv("g", { id: "tmworld" });
      [["hull", ""], ["guide", ""], ["edge", "d-tm-cge"], ["ego", ""], ["node", ""]].forEach(function (p) {
        lay[p[0]] = sv("g", p[1] ? { "class": p[1] } : {});
        world.appendChild(lay[p[0]]);
      });
      lay.hov = world.appendChild(sv("circle", { "class": "d-tm-chov", r: 0 }));
      lay.sel = world.appendChild(sv("circle", { "class": "d-tm-csel", r: 0 }));
      svg.appendChild(world);
      vp.appendChild(svg);
      lbl = mk(vp, "div", "d-tm-clbl", null, "tmlbl");
      tools();
      tipEl = mk(box, "div", "d-tm-ctip", null, "tmtip");
      tipEl.setAttribute("role", "tooltip");
      bind();
      if (window.ResizeObserver) new window.ResizeObserver(resize).observe(vp);
      // A theme switch re-measures the label text and re-tints the soup.
      if (window.MutationObserver) new window.MutationObserver(function () { wcache = new Map(); fam = ""; schedule(); }).observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    }
    function glyph(kind) {
      var g = sv("svg", { viewBox: "0 0 22 8", "class": "d-tm-cgly d-tm-cgly-" + kind, "aria-hidden": "true" });
      g.appendChild(sv("line", { x1: 0, y1: 4, x2: kind === "blocked" ? 17 : 22, y2: 4 }));
      if (kind === "blocked") g.appendChild(sv("path", { d: "M15 0.5L22 4L15 7.5z" }));
      return g;
    }
    // tools builds the overlay controls: the toolbar (edge toggles, the
    // similarity threshold and edge count, the Ego Ring button, the keyword
    // chip), the hint, the ego bar, the legend and the zoom control.
    function tools() {
      var t = U.tool = mk(box, "div", "d-tm-ctool", null, "tmctool");
      t.setAttribute("role", "toolbar");
      t.setAttribute("aria-label", wsT("js.tmap.c.tools"));
      U.et = {};
      // Field names render verbatim; only the similarity toggle is prose.
      [["parent", "parent"], ["blocked", "blocked-by"], ["related", "related"], ["sim", wsT("js.tmap.c.edge.sim")]].forEach(function (p) {
        if (p[0] === "sim") mk(t, "span", "d-tm-csep");
        var b = U.et[p[0]] = btn(t, "d-tm-cet", "", null, null, function () {
          S.edge[p[0]] = !S.edge[p[0]];
          tm.savePrefs();
          edgeSync();
          filter();
        });
        b.setAttribute("data-e", p[0]);
        b.appendChild(glyph(p[0]));
        mk(b, "span", "", p[1]);
        b.title = wsT("js.tmap.c.edge.tip", p[1]);
      });
      U.th = mk(t, "input", "", null, "tmsimth");
      U.th.type = "range";
      U.th.min = "15"; U.th.max = "60"; U.th.step = "1";
      U.th.setAttribute("aria-label", wsT("js.tmap.c.simth"));
      U.th.addEventListener("input", function () {
        S.simEdgeTh = +U.th.value;
        U.tho.textContent = S.simEdgeTh + "%";
        tm.savePrefs();
        if (ready) { buildSim(); highlight(); }
      });
      U.tho = mk(t, "output", "d-tm-cout");
      U.simn = mk(mk(t, "span", "d-tm-ccnt", wsT("js.tmap.c.edges")), "b", "", "0", "tmsimn");
      mk(t, "span", "d-tm-csep");
      U.ego = btn(t, "btn d-tm-btn", "◎ " + wsT("js.tmap.focus.ego"), "tmegob", null, function () { if (S.sel !== null) enterEgo(S.sel); });
      U.ego.title = wsT("js.tmap.c.ego.tip") + " (E)";
      U.ego.appendChild(tm.kbd("E"));
      U.ego.disabled = true;
      // The keyword chip: "<prefix> <term> (n) ×", the × its clear button.
      U.kw = mk(t, "span", "d-tm-ckw", null, "tmckw");
      U.kw.hidden = true;
      U.kw.setAttribute("role", "status");
      mk(U.kw, "span", "", wsT("js.tmap.c.kw"));
      U.kwt = mk(U.kw, "b");
      U.kwn = mk(U.kw, "span");
      btn(U.kw, "d-tm-ckwx", "×", "tmckwx", wsT("js.tmap.kw.clear"), function () { tm.setKw(null); }).title += " (Esc)";
      U.hint = mk(box, "div", "d-tm-chint", wsT("js.tmap.c.hint"), "tmchint");
      U.egobar = mk(box, "div", "d-tm-cego", null, "tmegobar");
      U.egobar.hidden = true;
      mk(U.egobar, "span", "", "◎ " + wsT("js.tmap.focus.ego"));
      U.egot = mk(U.egobar, "b");
      btn(U.egobar, "btn d-tm-btn", wsT("js.tmap.c.egoexit"), "tmegox", null, function () { exitEgo(); }).appendChild(tm.kbd("Esc"));
      U.legend = mk(box, "div", "d-tm-clegend", null, "tmlegend");
      U.legend.setAttribute("aria-label", wsT("js.tmap.c.legend"));
      tm.ST.forEach(function (s) {
        var name = wsT(ST_KEY[s]), text = s === "done" || s === "dropped" ? wsT("js.tmap.pill.ref", name) : name;
        var it = mk(U.legend, "span", "d-tm-clg d-tm-st-" + s), g = sv("svg", { viewBox: "0 0 12 12", "aria-hidden": "true" });
        g.appendChild(sv("circle", { cx: 6, cy: 6, r: s === "todo" ? 4.6 : s === "ready" ? 4.4 : 4.2 }));
        it.appendChild(g);
        mk(it, "span", "", text);
        it.title = text;
      });
      mk(U.legend, "span", "d-tm-clgc", wsT("js.tmap.c.legend.color"));
      var z = U.zoom = mk(box, "div", "d-tm-zoom");
      z.setAttribute("role", "group");
      z.setAttribute("aria-label", wsT("js.tmap.c.zoom"));
      btn(z, "d-tm-zb", "−", "tmzout", wsT("js.tmap.c.zoomout"), function () { animZoom(1 / 1.35); });
      U.zrd = mk(z, "output", "", "100%", "tmzrd");
      U.zrd.setAttribute("aria-label", wsT("js.tmap.c.zoomlevel"));
      btn(z, "d-tm-zb", "+", "tmzin", wsT("js.tmap.c.zoomin"), function () { animZoom(1.35); });
      btn(z, "d-tm-zb d-tm-zs", wsT("js.tmap.c.fit"), "tmzfit", wsT("js.tmap.c.fit.tip"), function () { fit(true); });
      btn(z, "d-tm-zb d-tm-zs", "1:1", "tmz100", wsT("js.tmap.c.reset"), function () { animTo(geo.zoomAt(cam, W / 2, H / 2, 1)); });
      edgeSync();
    }
    function edgeSync() {
      Object.keys(U.et).forEach(function (k) { U.et[k].setAttribute("aria-pressed", String(!!S.edge[k])); });
      U.th.value = String(S.simEdgeTh);
      U.tho.textContent = S.simEdgeTh + "%";
    }

    // --- per payload ------------------------------------------------------
    // render lays out the payload once: the stars, their labels, cluster
    // headers, soup, edges and hulls. A Refresh over the same stem set keeps
    // the camera; any other payload is fitted on show.
    function render(o) {
      o = o || {};
      stopAnims();
      if (E) egoOff(false);
      G = tm.data();
      ["hull", "guide", "edge", "ego", "node"].forEach(function (k) { lay[k].textContent = ""; });
      lbl.textContent = "";
      P = []; Bp = []; R = []; gN = []; lbs = []; cbs = []; soup = []; sects = []; edges = []; sims = []; egoEdges = [];
      hullPoly = {}; hullTop = {}; hovMarked = []; hov = null; kwHover = null; nb = new Set();
      ready = false;
      hideTip();
      if (!G) return;
      var key = G.nodes.map(function (n) { return n.stem; }).join("\n");
      keepCam = !!o.refresh && key === stemKey && fitted;
      if (!keepCam) { fitted = false; camFor = null; }
      stemKey = key;
      settle();
      buildNodes();
      buildSoup();
      buildEdges();
      ready = true;
      edgeSync();
      buildSim();
    }
    function settle() {
      var t0 = performance.now(), cix = {};
      G.clusters.forEach(function (c, ix) { cix[c.id] = ix; });
      var nodes = G.nodes.map(function (n) {
        R[n.i] = geo.radius(n.status, n.deg, n.cat === "epic");
        var x = [];
        n.X.forEach(function (l, j) {
          var r = l.rels;
          x.push([j, r.has("parent") || r.has("blocked-by") || r.has("child") || r.has("blocks") ? 1 : 0.5]);
        });
        return { k: cix[n.cluster] || 0, r: R[n.i], x: x, sim: n.sim.slice(0, 3).map(function (s) { return [s.j, s.s]; }) };
      });
      var res = geo.layout(nodes, Math.max(1, G.clusters.length), 42);
      G.nodes.forEach(function (n) {
        P[n.i] = { x: res.x[n.i], y: res.y[n.i] };
        Bp[n.i] = { x: res.x[n.i], y: res.y[n.i] };
      });
      layoutMs = performance.now() - t0;
    }
    function buildNodes() {
      G.nodes.forEach(function (n) {
        var g = tm.hue(sv("g", { "class": "d-tm-cnd d-tm-st-" + n.status, "data-i": n.i, transform: tr(P[n.i]) }), cl(n.cluster));
        g.appendChild(sv("circle", { "class": "d-tm-ccore", r: R[n.i].toFixed(2) }));
        lay.node.appendChild(g);
        gN[n.i] = g;
        var lb = el("div", "d-tm-lb");
        lb.setAttribute("data-i", n.i);
        lbl.appendChild(lb);
        lbs[n.i] = lb;
      });
      G.clusters.forEach(function (c) {
        var b = tm.hue(el("div", "d-tm-cb"), c), full = tm.clusterLabel(c);
        b.setAttribute("data-cl", c.id);
        b.appendChild(el("i"));
        var name = el("span", "", full);
        b.appendChild(name);
        b.appendChild(el("small", "", String(c.open || 0)));
        lbl.appendChild(b);
        cbs.push({ c: c, el: b, name: name, full: full, short: full.split(" · ")[0] });
      });
      sects = ["js.tmap.c.sect.up", "js.tmap.c.sect.down", "js.tmap.c.sect.rel", "js.tmap.c.sect.rel", "js.tmap.c.sect.sim"].map(function (k) {
        var d = el("div", "d-tm-sect", wsT(k));
        lbl.appendChild(d);
        return d;
      });
    }
    // buildSoup takes each cluster's first 8 keywords outside its name.
    function buildSoup() {
      G.clusters.forEach(function (c) {
        var names = new Set((c.nameTerms || []).map(function (t) { return String(t).toLowerCase(); }));
        var list = (G.kw[String(c.id)] || []).filter(function (x) { return !names.has(String(x.term).toLowerCase()); }).slice(0, 8);
        var max = 1e-6;
        list.forEach(function (x) { max = Math.max(max, +x.weight || 0); });
        list.forEach(function (x, rank) {
          var nw = (+x.weight || 0) / max, fs = geo.soupFont(nw);
          var e = tm.hue(el("div", "d-tm-sk" + (x.code ? " d-tm-code" : ""), x.term), c);
          e.setAttribute("data-j", soup.length);
          e.setAttribute("data-cl", c.id);
          e.style.setProperty("--d-tm-a", (0.55 + 0.33 * nw).toFixed(2));
          e.style.fontSize = fs + "px";
          lbl.appendChild(e);
          soup.push({ j: soup.length, c: c, rank: rank, term: String(x.term), code: !!x.code, fs: fs, nw: nw, ids: x.ticketIds || [], tw: x.tw || [], item: x, el: e, wx: 0, wy: 0, n: 0, on: false });
        });
      });
      soupOrder = soup.slice().sort(function (a, b) { return a.rank - b.rank || b.nw - a.nw; });
    }
    // Structural edges: parent child -> parent, blocked-by blocker ->
    // blocked (arrow), related once per pair.
    function buildEdges() {
      function add(a, b, cls) {
        var p = sv("path", { "class": "d-tm-ced d-tm-ce-" + cls });
        if (cls === "blocked") p.setAttribute("marker-end", "url(#tmarw)");
        lay.edge.appendChild(p);
        edges.push({ a: a, b: b, cls: cls, el: p });
      }
      G.nodes.forEach(function (n) {
        n.X.forEach(function (x, j) {
          if (x.rels.has("parent")) add(n.i, j, "parent");
          if (x.rels.has("blocked-by")) add(j, n.i, "blocked");
          if (x.rels.has("related") && n.i < j) add(n.i, j, "related");
        });
      });
    }
    // buildSim picks the similarity edges at or above the threshold, most
    // similar first, at most 3 per star.
    function buildSim() {
      sims.forEach(function (e) { e.el.remove(); e.hit.remove(); });
      sims = geo.simPick(G.nodes.map(function (n) { return n.sim; }), S.simEdgeTh / 100).map(function (p, ix) {
        var e = sv("path", { "class": "d-tm-ced d-tm-ce-sim" }), hit = sv("path", { "class": "d-tm-cehit", "data-e": ix });
        lay.edge.appendChild(e);
        lay.edge.appendChild(hit);
        return { a: p[1], b: p[2], s: p[0], terms: p[3], cls: "sim", el: e, hit: hit };
      });
      placeEdges();
      filter();
    }
    function epath(e) {
      return geo.edgePath(P[e.a], P[e.b], R[e.a] * ns + 1, R[e.b] * ns + (e.cls === "blocked" ? 2 : 1), e.cls === "sim" || e.cls === "related");
    }
    function setPath(e) {
      var d = epath(e);
      e.el.setAttribute("d", d);
      if (e.hit) e.hit.setAttribute("d", d);
    }
    function placeEdges() { edges.forEach(setPath); sims.forEach(setPath); egoEdges.forEach(setPath); }
    function placeRings() {
      var s = S.sel;
      function ring(c, i, pad) {
        if (i !== null && G && G.nodes[i] && shownI(i)) {
          c.setAttribute("cx", P[i].x.toFixed(1));
          c.setAttribute("cy", P[i].y.toFixed(1));
          c.setAttribute("r", (R[i] * ns + pad).toFixed(2));
        } else c.setAttribute("r", "0");
      }
      ring(lay.sel, s, 5);
      ring(lay.hov, hov !== s ? hov : null, 4);
    }

    // --- filters and highlights -----------------------------------------------
    function filter() {
      if (!ready) return;
      G.nodes.forEach(function (n) {
        gN[n.i].classList.toggle("d-tm-fx", !vis(n));
        gN[n.i].classList.toggle("d-tm-qh", tm.isHit(n.i));
      });
      function ef(e) {
        var hide = !vis(G.nodes[e.a]) || !vis(G.nodes[e.b]) || !S.edge[e.cls];
        e.el.classList.toggle("d-tm-fx", hide);
        if (e.hit) e.hit.classList.toggle("d-tm-fx", hide);
      }
      edges.forEach(ef);
      sims.forEach(ef);
      U.simn.textContent = String(sims.filter(function (e) { return !e.el.classList.contains("d-tm-fx"); }).length);
      buildHulls();
      highlight();
      applyKw();
    }
    // buildHulls draws each cluster's hull from its visible stars' settled
    // positions, then re-anchors the soup on them.
    function buildHulls() {
      lay.hull.textContent = "";
      hullPoly = {}; hullTop = {};
      var pts = {};
      G.nodes.forEach(function (n) {
        if (!vis(n)) return;
        geo.ring(Bp[n.i].x, Bp[n.i].y, R[n.i], pts[n.cluster] || (pts[n.cluster] = []));
      });
      G.clusters.forEach(function (c) {
        var h = pts[c.id] ? geo.convexHull(pts[c.id]) : [];
        if (h.length < 3) return;
        hullPoly[c.id] = h;
        hullTop[c.id] = geo.hullTop(h);
        lay.hull.appendChild(tm.hue(sv("path", { "class": "d-tm-chull", d: geo.hullPath(h) }), c));
      });
      // The soup anchor: the visible carrying tickets' weighted centroid.
      soup.forEach(function (it) {
        var c = geo.centroid(it.ids, it.tw, Bp, function (i) { return vis(G.nodes[i]); });
        it.wx = c.x; it.wy = c.y; it.n = c.n;
      });
      schedule();
    }
    function highlight() {
      if (!ready) return;
      var s = S.sel;
      nb = new Set();
      if (s !== null && G.nodes[s]) {
        nb.add(s);
        G.nodes[s].X.forEach(function (x, j) { nb.add(j); });
        sims.forEach(function (e) { if (e.a === s) nb.add(e.b); if (e.b === s) nb.add(e.a); });
      }
      svg.classList.toggle("d-tm-hassel", s !== null && !E);
      svg.classList.toggle("d-tm-hasq", !!S.hits && s === null);
      G.nodes.forEach(function (n) {
        gN[n.i].classList.toggle("d-tm-nb", nb.has(n.i));
        gN[n.i].classList.toggle("d-tm-sel", n.i === s);
      });
      edges.concat(sims).forEach(function (e) { e.el.classList.toggle("d-tm-hl", s !== null && (e.a === s || e.b === s)); });
      U.ego.disabled = s === null;
      placeRings();
      schedule();
    }
    // applyKw highlights the hovered soup word's or the keyword filter's
    // visible tickets and syncs the chip.
    function applyKw() {
      if (!ready) return;
      var ids = kwHover !== null && soup[kwHover] ? soup[kwHover].ids : S.kw ? Array.from(S.kw.ids) : null;
      kwSet = ids ? new Set(ids.filter(function (i) { return vis(G.nodes[i]); })) : null;
      svg.classList.toggle("d-tm-haskw", !!kwSet && !E);
      G.nodes.forEach(function (n) { gN[n.i].classList.toggle("d-tm-kwh", !!kwSet && kwSet.has(n.i)); });
      soup.forEach(function (x) {
        x.el.classList.toggle("d-tm-kwon", x.j === kwHover || (!!S.kw && S.kw.term === x.term && S.kw.cluster === x.c.id));
      });
      U.kw.hidden = !S.kw;
      if (S.kw) {
        var n = Array.from(S.kw.ids).filter(function (i) { return vis(G.nodes[i]); }).length;
        U.kwt.textContent = S.kw.term;
        U.kwn.textContent = "(" + n + ")";
        tm.hue(U.kw, cl(S.kw.cluster));
        U.kw.title = tm.clusterLabel(cl(S.kw.cluster)) + " · " + wsTn("js.tmap.c.tip.tickets", n);
      }
      schedule();
    }
    // kwSelect is a soup click: the keyword filter replaces the selection
    // and the camera frames its visible tickets.
    function kwSelect(it) {
      if (!it) return;
      if (S.sel !== null) tm.unselect();
      kwHover = null;
      tm.setKw(it.item, it.c.id);
      var ids = it.ids.filter(function (i) { return vis(G.nodes[i]); }), b = bounds(new Set(ids));
      if (b) animTo(geo.fitCam(b, W, H, 70, 54, Math.max(Math.min(cam.k * 1.35, 1.1), 0.9)), 420);
    }
    function kwHov(j) {
      if (kwHover === j) return;
      kwHover = j;
      applyKw();
    }
    function setHover(i) {
      if (hov === i) return;
      hovMarked.forEach(function (e) { e.classList.remove("d-tm-hv", "d-tm-hvn"); });
      hovMarked = [];
      hov = i;
      if (i !== null && !E) {
        edges.concat(sims).forEach(function (e) {
          if (e.a !== i && e.b !== i) return;
          var o = gN[e.a === i ? e.b : e.a];
          e.el.classList.add("d-tm-hv");
          o.classList.add("d-tm-hvn");
          hovMarked.push(e.el, o);
        });
        gN[i].classList.add("d-tm-hvn");
        hovMarked.push(gN[i]);
      }
      placeRings();
      schedule();
    }

    // --- camera -----------------------------------------------------------------
    function schedule() {
      if (!frame) frame = requestAnimationFrame(function () { frame = 0; draw(); });
    }
    function setCam(t) { cam.x = t.x; cam.y = t.y; cam.k = t.k; schedule(); }
    function stopAnims() {
      cancelAnimationFrame(anim);
      cancelAnimationFrame(tween);
      anim = 0; tween = 0;
    }
    function animTo(t, ms) {
      cancelAnimationFrame(anim);
      var f = { x: cam.x, y: cam.y, k: cam.k }, t0 = performance.now();
      ms = ms || 280;
      function step(now) {
        var u = Math.min(1, (now - t0) / ms), e = geo.easeOut(u);
        if (u >= 1) setCam(t);
        else setCam({ x: f.x + (t.x - f.x) * e, y: f.y + (t.y - f.y) * e, k: f.k + (t.k - f.k) * e });
        anim = u < 1 ? requestAnimationFrame(step) : 0;
      }
      anim = requestAnimationFrame(step);
    }
    function animZoom(f) { animTo(geo.zoomAt(cam, W / 2, H / 2, cam.k * f), 200); }
    function bounds(set) { return geo.bounds(P, function (i) { return set ? set.has(i) : shownI(i); }); }
    function fit(animate) {
      if (!ready) return;
      var b = bounds(E ? E.set : null);
      if (!b) return;
      var t = geo.fitCam(b, W, H, 64, 54, 1.6);
      if (animate) animTo(t); else setCam(t);
    }
    function centerOn(i, kLo, kHi, ms) {
      var p = P[i], k = Math.max(kLo, Math.min(cam.k, kHi)), t = { k: k, x: W / 2 - p.x * k, y: H / 2 - p.y * k };
      if (ms) animTo(t, ms); else setCam(t);
      camFor = i;
    }
    function resize() {
      if (!vp) return;
      var r = vp.getBoundingClientRect();
      if (!r.width) return;
      W = r.width; H = r.height;
      // The hint yields to the toolbar when they would meet.
      U.hint.hidden = !!E;
      if (!E) {
        var a = U.tool.getBoundingClientRect(), b = U.hint.getBoundingClientRect();
        if (a.width && b.width && a.right + 8 > b.left && a.bottom > b.top) U.hint.hidden = true;
      }
      schedule();
    }

    // --- drawing: the camera, star scale and every label ---------------------
    function draw() {
      if (!ready || S.view !== "C" || box.hidden || !W) return;
      world.setAttribute("transform", "matrix(" + cam.k + ",0,0," + cam.k + "," + cam.x + "," + cam.y + ")");
      U.zrd.textContent = Math.round(cam.k * 100) + "%";
      var n2 = geo.nodeScale(cam.k);
      if (Math.abs(n2 - ns) > 0.04) {
        ns = n2;
        G.nodes.forEach(function (n) { gN[n.i].firstChild.setAttribute("r", (R[n.i] * ns).toFixed(2)); });
        placeEdges();
        placeRings();
      }
      labels();
    }
    function textW(s, wt, size, isMono) {
      size = size || 12;
      var key = wt + "|" + size + (isMono ? "m" : "") + "|" + s, w = wcache.get(key);
      if (w === undefined) {
        if (!mctx) mctx = doc.createElement("canvas").getContext("2d");
        if (!fam) {
          var cs = getComputedStyle(box);
          fam = cs.fontFamily;
          mono = cs.getPropertyValue("--wb-mono").trim() || "monospace";
        }
        mctx.font = wt + " " + size + "px " + (isMono ? mono : fam);
        w = mctx.measureText(s).width;
        wcache.set(key, w);
      }
      return w;
    }
    function move(e, b) { e.style.transform = "translate(" + b.x.toFixed(1) + "px," + b.y.toFixed(1) + "px)"; }
    // prio is a star's title priority: the selection, the hover, the ring,
    // their neighbours, keyword and search matches, then hubs by status.
    function prio(n, s, hv, ego) {
      if (n.i === s) return 1e7;
      if (n.i === hv) return 9e6;
      if (ego) return 5e6 + (E.rank.get(n.i) || 0);
      if (s !== null && nb.has(n.i)) return 6e6 + tm.simOf(s, n.i) * 1e3;
      if (hv !== null && G.nodes[hv].X.has(n.i)) return 5.5e6;
      if (kwSet && kwSet.has(n.i)) return 4.5e6 + n.deg;
      if (tm.isHit(n.i)) return 4e6 + n.deg;
      return (n.cat === "epic" || n.deg >= 6 ? 2e4 + 200 * n.deg : 0) + (n.open ? PRIO[n.status] || 0 : 400) + 60 * n.deg;
    }
    // labels plans one frame (geo.planLabels, the overlay controls as
    // obstacles) and applies it: titles only from stage 2 or on
    // interaction, the soup while it shows.
    function labels() {
      var k = cam.k, ego = !!E, s = S.sel, hv = hov, blocks = [], stars = [];
      stg = geo.stage(k, ego);
      soupA = geo.soupAlpha(k, ego, s !== null);
      lbl.style.setProperty("--d-tm-soup", soupA.toFixed(3));
      svg.classList.toggle("d-tm-calm", stg === 1);
      var vr = vp.getBoundingClientRect();
      [U.tool, U.hint, U.legend, U.egobar, U.zoom].forEach(function (e) {
        if (e.hidden || !e.offsetParent) return;
        var r = e.getBoundingClientRect();
        if (r.width) blocks.push({ x: r.left - vr.left - 4, y: r.top - vr.top - 4, w: r.width + 8, h: r.height + 8 });
      });
      G.nodes.forEach(function (n) {
        if (!shownI(n.i)) return;
        var x = P[n.i].x * k + cam.x, y = P[n.i].y * k + cam.y, p;
        if (x < -40 || x > W + 40 || y < -40 || y > H + 40) return;
        p = prio(n, s, hv, ego);
        stars.push({
          i: n.i, x: x, y: y, r: R[n.i] * ns * k, p: p, title: n.title, framed: n.i === s || n.i === hv,
          pre: ego && E.pct.has(n.i) ? E.pct.get(n.i) + "%" : "", side: ego ? E.side.get(n.i) || "r" : "r", inner: ego && E.tag.get(n.i) === "x",
          obst: n.i === s || n.i === hv || nb.has(n.i) || ego || p >= 6e3, skip: !!S.hits && !S.hits.has(n.i) && p < 5e6
        });
      });
      var minP = ego ? 0 : stg === 1 ? 4e6 : stg === 2 ? 6e3 : 0;
      var plan = geo.planLabels({
        W: W, H: H, k: k, cx: cam.x, cy: cam.y, ego: E, soupFirst: soupA >= 0.5, blocks: blocks, text: textW,
        maxLen: ego ? 60 : stg === 2 ? 26 : k < 1.8 ? 46 : 70,
        obst: stars.filter(function (n) { return n.obst; }),
        stars: stars.filter(function (n) { return n.p >= minP; }).sort(function (a, b) { return b.p - a.p; }),
        heads: ego ? [] : cbs.filter(function (cb) { return hullTop[cb.c.id]; }).map(function (cb) {
          var h = hullTop[cb.c.id];
          return { id: cb.c.id, x: h.x, y: h.y, by: h.by, full: cb.full, short: cb.short, cnt: String(cb.c.open || 0).length * 6.6 };
        }),
        sects: ego ? E.sect.map(function (a, ix) { return a && { x: a.x, y: a.y, text: sects[ix].textContent }; }) : [],
        soup: soupA <= 0.02 ? [] : soupOrder.filter(function (it) { return hullPoly[it.c.id] && it.n; }).map(function (it) {
          return { j: it.j, cl: it.c.id, x: it.wx * k + cam.x, y: it.wy * k + cam.y, w: Math.ceil(textW(it.term, it.code ? 500 : 540, it.fs, it.code) + 6), h: Math.ceil(it.fs * 1.3), poly: hullPoly[it.c.id] };
        })
      });
      cbs.forEach(function (cb) {
        var h = plan.heads.get(cb.c.id);
        if (h) { cb.name.textContent = h.txt; cb.el.style.width = h.b.w + "px"; move(cb.el, h.b); }
        cb.el.classList.toggle("on", !!h);
      });
      sects.forEach(function (d, ix) {
        var b = plan.sects.get(ix);
        if (b) { d.style.width = b.w + "px"; move(d, b); }
        d.classList.toggle("on", !!b);
      });
      soup.forEach(function (it) {
        var b = plan.soup.get(it.j);
        if (b) { move(it.el, b); it.el.style.width = b.w + "px"; it.el.style.height = it.el.style.lineHeight = b.h + "px"; }
        if (it.on !== !!b) { it.el.classList.toggle("on", !!b); it.on = !!b; }
      });
      var dim = s !== null && !ego;
      G.nodes.forEach(function (n) {
        var e = lbs[n.i], L = plan.stars.get(n.i), key = L && L.pre + "\n" + L.txt;
        if (L) {
          if (e._k !== key) {
            e.textContent = "";
            if (L.pre) e.appendChild(el("b", "", L.pre));
            e.appendChild(doc.createTextNode(L.txt));
            e._k = key;
          }
          move(e, L.b);
          e.style.width = L.b.w.toFixed(1) + "px";
        }
        e.classList.toggle("on", !!L);
        e.classList.toggle("d-tm-sel", n.i === s);
        e.classList.toggle("d-tm-hov", n.i === hv && n.i !== s);
        e.classList.toggle("d-tm-kw", !!kwSet && kwSet.has(n.i));
        e.classList.toggle("d-tm-dim", dim && !!L && !nb.has(n.i));
      });
      nLabels = plan.stars.size;
      nSoup = plan.soup.size;
    }

    // --- Ego Ring -------------------------------------------------------------
    function enterEgo(i) {
      if (!ready || !G.nodes[i]) return;
      egoNext = true;
      clicking = true;
      try { tm.select(i); } finally { egoNext = false; clicking = false; }
    }
    // egoOn rings ticket i: its visible explicit links on the inner ring,
    // up to 10 unlinked similar tickets on the outer one.
    function egoOn(i) {
      var n = G.nodes[i], c0 = Bp[i], up = [], down = [], rel = [];
      var links = Array.from(n.X.values()).filter(function (x) { return vis(G.nodes[x.j]); }).sort(function (a, b) {
        var p = G.nodes[a.j], q = G.nodes[b.j];
        return (tm.ST.indexOf(p.status) - tm.ST.indexOf(q.status)) || cmp(p.stem, q.stem);
      });
      links.forEach(function (x) {
        var r = x.rels;
        (r.has("parent") || r.has("blocked-by") ? up : r.has("child") || r.has("blocks") ? down : rel).push(x.j);
      });
      var th = Math.min(S.simEdgeTh, 12);
      var sim = n.sim.filter(function (s) { return !n.X.has(s.j) && s.s * 100 >= th && vis(G.nodes[s.j]); }).slice(0, 10);
      var g = geo.ego({ i: i, up: up, down: down, rel: rel, sim: sim.map(function (s) { return [s.j, s.s]; }) }, W, c0);
      S.ego = i;
      E = { i: i, set: new Set(g.pos.keys()), side: g.side, pct: g.pct, rank: g.rank, tag: g.tag, R1: g.R1, gap: g.R2 - g.R1, outer: g.outer, sect: g.sect };
      lay.guide.textContent = "";
      lay.ego.textContent = "";
      egoEdges = [];
      if (g.rows) lay.guide.appendChild(sv("ellipse", { "class": "d-tm-cguide", cx: c0.x, cy: c0.y, rx: g.R1, ry: g.Ry1 }));
      if (sim.length) lay.guide.appendChild(sv("ellipse", { "class": "d-tm-cguide", cx: c0.x, cy: c0.y, rx: g.R2, ry: g.Ry2 }));
      links.forEach(function (x) {
        var r = x.rels, cls = r.has("blocked-by") || r.has("blocks") ? "blocked" : r.has("parent") || r.has("child") ? "parent" : "related";
        var p = sv("path", { "class": "d-tm-ced d-tm-ce-" + cls + " d-tm-hl" });
        if (cls === "blocked") p.setAttribute("marker-end", "url(#tmarw)");
        lay.ego.appendChild(p);
        egoEdges.push({ a: r.has("blocked-by") ? x.j : i, b: r.has("blocked-by") ? i : x.j, cls: cls, el: p });
      });
      sim.forEach(function (s) {
        var p = sv("path", { "class": "d-tm-ced d-tm-ce-sim d-tm-hl" }), hit = sv("path", { "class": "d-tm-cehit", "data-x": egoEdges.length });
        lay.ego.appendChild(p);
        lay.ego.appendChild(hit);
        egoEdges.push({ a: i, b: s.j, cls: "sim", s: s.s, terms: s.terms || [], el: p, hit: hit });
      });
      setHover(null);
      G.nodes.forEach(function (m) { gN[m.i].classList.toggle("d-tm-eg", E.set.has(m.i)); });
      svg.classList.add("d-tm-egoon");
      box.classList.add("d-tm-ego");
      U.egobar.hidden = false;
      U.egot.textContent = n.title;
      U.egot.title = n.stem;
      U.hint.hidden = true;
      tweenTo(g.pos, null);
      animTo(geo.egoCam(Array.from(g.pos.values()), c0, W, H), 420);
      highlight();
      inspector(i);
      camFor = i;
    }
    // egoOff puts every star back; recenter (Esc) centers the kept
    // selection at k in [.9, 1.2].
    function egoOff(recenter) {
      S.ego = null;
      E = null;
      lay.guide.textContent = "";
      lay.ego.textContent = "";
      egoEdges = [];
      svg.classList.remove("d-tm-egoon");
      box.classList.remove("d-tm-ego");
      U.egobar.hidden = true;
      resize();
      if (!G) return;
      G.nodes.forEach(function (m) { gN[m.i].classList.remove("d-tm-eg"); });
      if (!recenter) {
        cancelAnimationFrame(tween);
        tween = 0;
        G.nodes.forEach(function (m) { P[m.i] = { x: Bp[m.i].x, y: Bp[m.i].y }; gN[m.i].setAttribute("transform", tr(P[m.i])); });
        placeEdges();
        return;
      }
      var T = new Map();
      G.nodes.forEach(function (m) { T.set(m.i, Bp[m.i]); });
      tweenTo(T, placeEdges);
      highlight();
      if (S.sel !== null && G.nodes[S.sel]) centerOn(S.sel, 0.9, 1.2, 420); else fit(true);
    }
    function exitEgo() { if (E) egoOff(true); }
    function tweenTo(T, done) {
      cancelAnimationFrame(tween);
      var from = new Map(), t0 = performance.now();
      T.forEach(function (t, i) { from.set(i, { x: P[i].x, y: P[i].y }); });
      function step(now) {
        var u = Math.min(1, (now - t0) / 420), e = geo.easeInOut(u);
        T.forEach(function (t, i) {
          var f = from.get(i);
          P[i] = u >= 1 ? { x: t.x, y: t.y } : { x: f.x + (t.x - f.x) * e, y: f.y + (t.y - f.y) * e };
          gN[i].setAttribute("transform", tr(P[i]));
        });
        egoEdges.forEach(setPath);
        placeRings();
        schedule();
        tween = u < 1 ? requestAnimationFrame(step) : 0;
        if (u >= 1 && done) done();
      }
      tween = requestAnimationFrame(step);
    }

    // --- tooltip ------------------------------------------------------------------
    function showTip(fill, mx, my) {
      tipEl.textContent = "";
      fill(tipEl);
      tipEl.classList.add("on");
      var p = geo.tipPos(mx, my, tipEl.offsetWidth, tipEl.offsetHeight, box.clientWidth, box.clientHeight);
      tipEl.style.transform = "translate(" + Math.round(p.x) + "px," + Math.round(p.y) + "px)";
    }
    function hideTip() { if (tipEl) tipEl.classList.remove("on"); }
    function row(c) { return tm.hue(el("div", "d-tm-ttr"), c); }
    function nodeTip(i, mx, my) {
      var n = G.nodes[i];
      showTip(function (t) {
        t.appendChild(el("div", "d-tm-ttt", n.title));
        t.appendChild(el("div", "d-tm-tts", n.stem));
        var r = row(cl(n.cluster));
        r.appendChild(tm.statusBadge(n));
        r.appendChild(tm.clusterPill(n.cluster));
        var d = el("span", "d-tm-tag", "↔ " + n.deg);
        d.title = wsT("js.tmap.ind.deg", n.deg);
        r.appendChild(d);
        t.appendChild(r);
        n.sim.slice(0, 2).forEach(function (s) { t.appendChild(el("div", "d-tm-tts", "≈ " + Math.round(s.s * 100) + "% " + G.nodes[s.j].title.slice(0, 46))); });
      }, mx, my);
    }
    function soupTip(it, mx, my) {
      var ids = it.ids.filter(function (i) { return vis(G.nodes[i]); });
      showTip(function (t) {
        var r = row(it.c);
        r.appendChild(el("span", "d-tm-ttb d-tm-ttk", it.term));
        r.appendChild(el("span", "d-tm-tag", wsTn("js.tmap.c.tip.tickets", ids.length)));
        r.appendChild(tm.clusterPill(it.c.id));
        t.appendChild(r);
        var list = el("div", "d-tm-tts");
        ids.slice(0, 3).forEach(function (i) { list.appendChild(el("div", "", "· " + G.nodes[i].title.slice(0, 54))); });
        if (ids.length > 3) list.appendChild(el("div", "", wsT("js.tmap.c.tip.more", ids.length - 3)));
        t.appendChild(list);
        t.appendChild(el("div", "d-tm-tts", wsT("js.tmap.c.kwtip")));
      }, mx, my);
    }
    function edgeTip(e, mx, my) {
      showTip(function (t) {
        var r = row(null);
        r.appendChild(el("span", "d-tm-ttb", wsT("js.tmap.c.tip.sim", Math.round(e.s * 100))));
        (e.terms || []).forEach(function (x) { r.appendChild(tm.chip(x[0], !!x[1])); });
        t.appendChild(r);
        var s = el("div", "d-tm-tts");
        s.appendChild(el("div", "", G.nodes[e.a].title.slice(0, 50)));
        s.appendChild(el("div", "", "↔ " + G.nodes[e.b].title.slice(0, 50)));
        t.appendChild(s);
      }, mx, my);
    }

    // --- pointer --------------------------------------------------------------------
    function kind(t) {
      if (!t || !t.closest) return { type: "bg" };
      var x = t.closest(".d-tm-cnd");
      if (x) return { type: "node", i: +x.getAttribute("data-i") };
      if ((x = t.closest(".d-tm-lb"))) return { type: "label", i: +x.getAttribute("data-i") };
      if ((x = t.closest(".d-tm-sk"))) return { type: "soup", j: +x.getAttribute("data-j") };
      if (t.classList && t.classList.contains("d-tm-cehit")) {
        var e = t.hasAttribute("data-e") ? sims[+t.getAttribute("data-e")] : egoEdges[+t.getAttribute("data-x")];
        return e ? { type: "edge", e: e } : { type: "bg" };
      }
      return { type: "bg" };
    }
    // hitOf: a star under a soup word wins over the word.
    function hitOf(t, ev) {
      var k0 = kind(t);
      var stack = k0.type === "soup" && ev && doc.elementsFromPoint ? doc.elementsFromPoint(ev.clientX, ev.clientY).map(kind) : [];
      return geo.resolveHit(k0, stack);
    }
    function local(e) { var r = vp.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top }; }
    function bind() {
      vp.addEventListener("wheel", function (e) {
        e.preventDefault();
        if (!ready) return;
        var m = local(e);
        cancelAnimationFrame(anim);
        anim = 0;
        setCam(geo.zoomAt(cam, m.x, m.y, geo.wheelK(cam.k, geo.wheelDy(e.deltaY, e.deltaMode), e.ctrlKey)));
      }, { passive: false });
      vp.addEventListener("pointerdown", down);
      vp.addEventListener("pointermove", moveP);
      vp.addEventListener("pointerup", function (e) { end(e, false); });
      vp.addEventListener("pointercancel", function (e) { end(e, true); });
      vp.addEventListener("lostpointercapture", function (e) { if ((drag && drag.id === e.pointerId) || pins.has(e.pointerId)) end(e, true); });
      vp.addEventListener("pointerleave", function () {
        if (drag || pinch) return;
        setHover(null);
        kwHov(null);
        if (!pinned) hideTip();
      });
      // The context menu and the middle click's auxclick stay inside #tmvp.
      vp.addEventListener("auxclick", function (e) { e.preventDefault(); });
      vp.addEventListener("contextmenu", function (e) { e.preventDefault(); });
    }
    function down(e) {
      if (e.button === 2 || !ready) return;
      if (e.button === 1) e.preventDefault();
      cancelAnimationFrame(anim);
      anim = 0;
      pinned = false;
      hideTip();
      if (e.pointerType === "touch") {
        pins.set(e.pointerId, local(e));
        try { vp.setPointerCapture(e.pointerId); } catch (err) { /* gone */ }
        if (pins.size === 2) {
          // Two fingers: pinch about their midpoint, no click.
          var a = Array.from(pins.values());
          pinch = { d0: Math.hypot(a[1].x - a[0].x, a[1].y - a[0].y) || 1, m0: { x: (a[0].x + a[1].x) / 2, y: (a[0].y + a[1].y) / 2 }, c0: { x: cam.x, y: cam.y, k: cam.k } };
          drag = null;
          return;
        }
        if (pins.size > 2) return;
      }
      var hit = hitOf(e.target, e);
      var nodeDrag = e.altKey && e.button === 0 && hit.type === "node" && !E && e.pointerType !== "touch";
      drag = { id: e.pointerId, sx: e.clientX, sy: e.clientY, cx: cam.x, cy: cam.y, moved: false, hit: hit, mode: nodeDrag ? "node" : "pan", btn: e.button };
      if (nodeDrag) drag.p0 = { x: P[hit.i].x, y: P[hit.i].y };
      try { vp.setPointerCapture(e.pointerId); } catch (err) { /* gone */ }
      vp.classList.add(nodeDrag ? "d-tm-ndrag" : "d-tm-grab");
    }
    function moveP(e) {
      if (pinch && pins.has(e.pointerId)) {
        pins.set(e.pointerId, local(e));
        var a = Array.from(pins.values()), c = pinch.c0;
        if (a.length < 2) return;
        var d = Math.hypot(a[1].x - a[0].x, a[1].y - a[0].y), m = { x: (a[0].x + a[1].x) / 2, y: (a[0].y + a[1].y) / 2 };
        var k2 = geo.clampK(c.k * d / pinch.d0), wx = (pinch.m0.x - c.x) / c.k, wy = (pinch.m0.y - c.y) / c.k;
        setCam({ x: m.x - wx * k2, y: m.y - wy * k2, k: k2 });
        return;
      }
      if (drag && drag.id === e.pointerId) {
        var dx = e.clientX - drag.sx, dy = e.clientY - drag.sy;
        if (!drag.moved && Math.hypot(dx, dy) > 3) drag.moved = true;
        if (!drag.moved) return;
        if (drag.mode === "pan") setCam({ x: drag.cx + dx, y: drag.cy + dy, k: cam.k });
        else {
          var i = drag.hit.i;
          P[i] = { x: drag.p0.x + dx / cam.k, y: drag.p0.y + dy / cam.k };
          Bp[i] = { x: P[i].x, y: P[i].y };
          gN[i].setAttribute("transform", tr(P[i]));
          edges.concat(sims).forEach(function (x) { if (x.a === i || x.b === i) setPath(x); });
          placeRings();
          schedule();
        }
        return;
      }
      if (e.pointerType !== "touch" && !drag && ready) hoverAt(e);
    }
    function end(e, cancel) {
      if (pins.has(e.pointerId)) {
        pins.delete(e.pointerId);
        if (pinch) {
          if (pins.size < 2) pinch = null;
          drag = null;
          vp.classList.remove("d-tm-grab", "d-tm-ndrag");
          return;
        }
      }
      var d = drag;
      if (!d || d.id !== e.pointerId) return;
      drag = null;
      vp.classList.remove("d-tm-grab", "d-tm-ndrag");
      if (d.mode === "node" && d.moved) buildHulls();
      if (cancel || d.moved || d.btn !== 0) return;
      var now = performance.now(), lc = lastClick;
      var dbl = !!lc && now - lc.t < 340 && Math.hypot(e.clientX - lc.x, e.clientY - lc.y) < 8;
      lastClick = dbl ? null : { t: now, x: e.clientX, y: e.clientY };
      click(d.hit, e, dbl);
    }
    function click(hit, e, dbl) {
      var m = local(e);
      if (hit.type === "node" || hit.type === "label") {
        if (dbl || (E && hit.i !== S.ego)) { enterEgo(hit.i); return; }
        clicking = true;
        try { tm.select(hit.i); } finally { clicking = false; }
        return;
      }
      if (hit.type === "soup") { kwSelect(soup[hit.j]); return; }
      if (hit.type === "edge") { pinned = true; edgeTip(hit.e, m.x, m.y); return; }
      if (dbl) { animTo(geo.zoomAt(cam, m.x, m.y, cam.k * 1.8), 260); return; }
      if (!E && S.sel !== null) tm.unselect();
    }
    function hoverAt(e) {
      var hit = hitOf(e.target, e), m = local(e);
      if (hit.type !== "soup") kwHov(null);
      if (hit.type === "node" || hit.type === "label") { setHover(hit.i); nodeTip(hit.i, m.x, m.y); pinned = false; }
      else if (hit.type === "soup") { setHover(null); kwHov(hit.j); soupTip(soup[hit.j], m.x, m.y); pinned = false; }
      else if (hit.type === "edge") { setHover(null); edgeTip(hit.e, m.x, m.y); }
      else { setHover(null); if (!pinned) hideTip(); }
    }

    // --- the host contract ----------------------------------------------------------
    function inspector(i) {
      if (tm.viewB && tm.viewB.insp) tm.viewB.insp(i);
      tm.inspShow(true);
      resize();
    }
    // focus shows node i selected: in the Ego Ring it re-centers the ring;
    // a selection made outside the canvas (search, inspector, history)
    // also centers the camera on it.
    function focus(i) {
      if (!ready || !G.nodes[i]) return;
      if (egoNext || E) { egoOn(i); return; }
      highlight();
      inspector(i);
      if (clicking || keepCam) camFor = i;
      else if (fitted && !box.hidden) centerOn(i, 0.95, Infinity, 280);
      keepCam = false;
    }
    function unfocus() {
      if (!ready) return;
      if (E) egoOff(true);
      highlight();
      tm.inspShow(false);
      resize();
    }
    // show: a payload's first show fits it; a selection carried from View
    // B (or the search) is centered at k >= .95 with its label framed.
    function show() {
      if (!ready) return;
      if (S.sel !== null && G.nodes[S.sel]) inspector(S.sel); else { tm.inspShow(false); resize(); }
      keepCam = false;
      edgeSync();
      filter();
      if (!fitted) { fit(false); fitted = true; }
      if (S.sel !== null && G.nodes[S.sel] && !E && camFor !== S.sel) centerOn(S.sel, 0.95, Infinity, 0);
    }
    // hide keeps the camera, the ring and the selection for the next show.
    function hide() {
      drag = null; pinch = null; pins.clear(); pinned = false;
      setHover(null);
      kwHov(null);
      hideTip();
      tm.inspShow(false);
    }
    function key(e) {
      var k = e.key;
      if (e.altKey) return false;
      if ((k === "e" || k === "E") && S.sel !== null) {
        if (E && S.ego === S.sel) exitEgo(); else enterEgo(S.sel);
        return true;
      }
      if (k === "+" || k === "=") { animZoom(1.35); return true; }
      if (k === "-" || k === "_") { animZoom(1 / 1.35); return true; }
      if (k === "0") { fit(true); return true; }
      return false;
    }

    // probe is for the browser test only (window.__tmapc, behind the same
    // gate as window.__tmap): layout and label stats, positions, the soup's
    // anchors, and a synchronous draw.
    var probe = {
      stats: function () { return { stage: stg, soupA: soupA, soup: nSoup, labels: nLabels, ns: ns, layoutMs: layoutMs, W: W, H: H, simEdges: sims.length }; },
      pos: function () { return P.map(function (p) { return [p.x, p.y]; }); },
      base: function () { return Bp.map(function (p) { return [p.x, p.y]; }); },
      soup: function () { return soup.map(function (s) { return { j: s.j, term: s.term, cluster: s.c.id, wx: s.wx, wy: s.wy, n: s.n, on: s.on }; }); },
      draw: function () { cancelAnimationFrame(frame); frame = 0; draw(); },
      setCam: function (c) { stopAnims(); setCam(c); probe.draw(); }
    };
    return {
      mount: mount, render: render, filter: filter, focus: focus, unfocus: unfocus,
      isFocus: function () { return S.sel !== null; }, show: show, hide: hide, layout: resize, key: key,
      isEgo: function () { return !!E; }, exitEgo: exitEgo, enterEgo: enterEgo, probe: probe
    };
  })();
