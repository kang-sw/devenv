  // --- ticket map: the full-viewport overlay #thandle opens ----------------
  // One outer binding (the asset concatenation is a single IIFE): every
  // declaration below lives in this inner scope. The map reads
  // GET /s/<rid>/tickets/graph on the first open and on Refresh only, keeps
  // the graph and its state in memory across close and reopen, and covers
  // the page at --z-modal while shown.
  //
  // View host contract. A view is an object registered on ticketMap before
  // the first open: ticketMap.viewB (dashboard-ticketmap-viewb.js, right
  // after this file) and ticketMap.graph (View C, dashboard-ticketmap-graph.js,
  // after viewb and its pure math, dashboard-ticketmap-geom.js). The View C
  // tab enables itself when ticketMap.graph exists.
  // The host calls, on the shown view:
  //   mount(el)        once, with its container (#tmvb or #tmvc), at build;
  //   render(o)        after a load or Refresh, and on the first show of a
  //                    view per load: draw the overview from ticketMap.S and
  //                    ticketMap.data(); o.refresh marks a Refresh, and
  //                    o.keepScroll that the cluster count is unchanged. When
  //                    the view was focused and S.sel survives, the host then
  //                    calls focus(S.sel);
  //   filter()         after a status, cluster, query, or keyword change:
  //                    re-apply ticketMap.shown(n) and ticketMap.isHit(i);
  //   focus(i)         show node i selected (B: the click-focus hierarchy);
  //                    S.sel and the history are already updated;
  //   unfocus()        drop the selection (B: back to the matrix);
  //   isFocus()        B: whether the focus mode shows (Esc step 5, arrows);
  //   show(), hide()   optional, on a view switch and on reopen;
  //   layout()         optional, re-measure (the inspector opened or closed);
  //   key(e)           optional, a key the host did not consume; return true
  //                    to consume it;
  //   isEgo(), exitEgo()  View C only (Esc step 4).
  // S.cam is View C's live camera and S.ego its ring's ticket (or null).
  // A view calls back select(i), unselect(), move(d), openFull(i), the
  // builders, and savePrefs(); it never fetches and never adds a key
  // listener of its own (the host's window capture listener isolates the
  // page while the overlay shows).
  var ticketMap = (function () {
    // tm:pure-begin. Pure logic: no DOM and no page helper, so the node
    // tests (testdata/ticketmap-state.test.cjs) run this block as is.
    var TM_ST = ["ready", "todo", "idea", "done", "dropped"];
    var TM_OPEN = ["ready", "todo", "idea"];
    var TM_REL = ["parent", "blocked-by", "child", "blocks", "related"];
    var TM_INV = { "parent": "child", "blocked-by": "blocks", "related": "related" };
    var TM_EDGES = ["parent", "blocked", "related", "sim"];
    function tmDefaults() {
      return {
        simTh: 15, simUnlinked: false, simRefs: true, simEdgeTh: 24,
        st: { ready: true, todo: true, idea: true, done: true, dropped: true },
        edge: { parent: true, blocked: true, related: false, sim: true }
      };
    }
    // tmView is the stored last view: "B" unless "C" was stored.
    function tmView(raw) { return raw === "C" ? "C" : "B"; }
    // tmPrefs reads ws.ticketmap.prefs field by field: a field that is
    // missing, of the wrong type, or out of range keeps its default.
    function tmPrefs(raw) {
      var p = tmDefaults(), o = null;
      try { o = JSON.parse(raw); } catch (e) { o = null; }
      if (!o || typeof o !== "object" || Array.isArray(o)) return p;
      function num(k, lo, hi) {
        var v = o[k];
        if (typeof v === "number" && isFinite(v) && v >= lo && v <= hi) p[k] = Math.round(v);
      }
      function bools(k, keys) {
        var v = o[k];
        if (!v || typeof v !== "object") return;
        keys.forEach(function (s) { if (typeof v[s] === "boolean") p[k][s] = v[s]; });
      }
      num("simTh", 10, 60);
      num("simEdgeTh", 15, 60);
      if (typeof o.simUnlinked === "boolean") p.simUnlinked = o.simUnlinked;
      if (typeof o.simRefs === "boolean") p.simRefs = o.simRefs;
      bools("st", TM_ST);
      bools("edge", TM_EDGES);
      return p;
    }
    // tmDerive builds the client graph from the payload: per node the
    // explicit relations X (j -> {j, rels, notes}, with the inverse
    // child/blocks/related on the other end), the similarity map and its
    // top-25 list, deg (distinct explicit neighbours), blockedOpen, and the
    // lower-cased search haystack. label(cluster) names a cluster.
    function tmDerive(data, label) {
      data = data || {};
      var g = { meta: data.meta || {}, clusters: data.clusters || [], kw: data.clusterKeywords || {}, nodes: [], byStem: {}, byCluster: {}, count: {} };
      TM_ST.forEach(function (s) { g.count[s] = 0; });
      g.clusters.forEach(function (c) { g.byCluster[c.id] = c; });
      (data.nodes || []).forEach(function (r, i) {
        var n = {};
        for (var k in r) if (Object.prototype.hasOwnProperty.call(r, k)) n[k] = r[k];
        n.i = i;
        n.title = r.title || r.stem;
        n.kw = r.kw || [];
        n.open = TM_OPEN.indexOf(r.status) >= 0;
        n.X = new Map();
        n.simMap = new Map();
        g.nodes.push(n);
        g.byStem[n.stem] = i;
        if (g.count[n.status] !== undefined) g.count[n.status]++;
      });
      var N = g.nodes.length;
      function ok(i) { return typeof i === "number" && i >= 0 && i < N; }
      function add(a, b, rel, note, side) {
        var x = g.nodes[a].X.get(b);
        if (!x) { x = { j: b, rels: new Set(), notes: [] }; g.nodes[a].X.set(b, x); }
        x.rels.add(rel);
        if (note) x.notes.push({ note: note, side: side, rel: rel });
      }
      (data.edges || []).forEach(function (e) {
        if (!ok(e[0]) || !ok(e[1]) || e[0] === e[1] || !TM_INV[e[2]]) return;
        add(e[0], e[1], e[2], e[3] || "", "out");
        add(e[1], e[0], TM_INV[e[2]], e[3] || "", "in");
      });
      (data.sim || []).forEach(function (r) {
        if (!ok(r[0]) || !ok(r[1]) || r[0] === r[1]) return;
        var terms = r[3] || [], men = !!r[4];
        g.nodes[r[0]].simMap.set(r[1], { j: r[1], s: +r[2] || 0, terms: terms, men: men });
        g.nodes[r[1]].simMap.set(r[0], { j: r[0], s: +r[2] || 0, terms: terms, men: men });
      });
      g.nodes.forEach(function (n) {
        n.sim = Array.from(n.simMap.values()).sort(function (a, b) {
          var sa = g.nodes[a.j].stem, sb = g.nodes[b.j].stem;
          return b.s - a.s || (sa < sb ? -1 : sa > sb ? 1 : 0);
        }).slice(0, 25);
        n.deg = n.X.size;
        n.blockedOpen = false;
        n.X.forEach(function (x) { if (x.rels.has("blocked-by") && g.nodes[x.j].open) n.blockedOpen = true; });
        var c = g.byCluster[n.cluster];
        n.hay = [n.title, n.stem, n.kw.map(function (k) { return k[0]; }).join(" "), c ? label(c) : "", n.preview || ""].join(" ").toLowerCase();
      });
      return g;
    }
    // tmBucket is the group a linked ticket goes to: the first relation it
    // carries in the order parent, blocked-by, child, blocks, related.
    function tmBucket(rels) {
      for (var k = 0; k < TM_REL.length; k++) if (rels.has(TM_REL[k])) return TM_REL[k];
      return null;
    }
    function tmTerms(q) { return String(q || "").toLowerCase().split(/\s+/).filter(Boolean); }
    // tmHits is the set of nodes whose haystack holds every query term, or
    // null for an empty query.
    function tmHits(nodes, q) {
      var terms = tmTerms(q);
      if (!terms.length) return null;
      var hits = new Set();
      nodes.forEach(function (n) {
        if (terms.every(function (t) { return n.hay.indexOf(t) >= 0; })) hits.add(n.i);
      });
      return hits;
    }
    // tmShown is the filter: status on, cluster not hidden, a search hit,
    // and in the keyword filter's tickets.
    function tmShown(S, n) {
      return !!S.st[n.status] && !S.cl.has(n.cluster) && (!S.hits || S.hits.has(n.i)) && (!S.kw || S.kw.ids.has(n.i));
    }
    // tmFirstHit is the shown search hit first in status order, then node
    // order, or null.
    function tmFirstHit(S, nodes) {
      var best = null;
      nodes.forEach(function (n) {
        if (!S.hits || !S.hits.has(n.i) || !tmShown(S, n)) return;
        if (best === null || TM_ST.indexOf(n.status) < TM_ST.indexOf(nodes[best].status)) best = n.i;
      });
      return best;
    }
    // History: push the ticket unless it is current, truncating forward
    // entries; move returns the entry d steps away, or null at an end.
    function tmHistPush(S, i) {
      if (S.hp >= 0 && S.hist[S.hp] === i) return false;
      S.hist = S.hist.slice(0, S.hp + 1);
      S.hist.push(i);
      S.hp = S.hist.length - 1;
      return true;
    }
    function tmHistMove(S, d) {
      var np = S.hp + d;
      if (np < 0 || np >= S.hist.length) return null;
      S.hp = np;
      return S.hist[np];
    }
    // tmHistRemap carries the history over a Refresh: entries whose stem is
    // gone drop out, a repeat collapses, and hp follows the current entry.
    function tmHistRemap(S, stemOf, indexOf) {
      var out = [], hp = -1;
      S.hist.forEach(function (i, k) {
        var j = indexOf(stemOf(i));
        if (j === undefined || j === null) return;
        if (out[out.length - 1] !== j) out.push(j);
        if (k <= S.hp) hp = out.length - 1;
      });
      S.hist = out;
      S.hp = out.length ? Math.max(0, hp) : -1;
    }
    // tmEscStep is the Esc layering over a snapshot, first match wins:
    // the Full View, the cluster popover, a query in the focused search,
    // View C's ego ring, keyword filter and selection, View B's focus,
    // and the overlay itself.
    function tmEscStep(s) {
      if (s.full) return "full";
      if (s.pop) return "pop";
      if (s.inQuery && s.query) return "query";
      if (s.view === "C") {
        if (s.ego) return "ego";
        if (s.kw) return "kw";
        if (s.sel !== null && s.sel !== undefined) return "deselect";
      }
      if (s.view === "B" && s.focus) return "unfocus";
      return "close";
    }
    // tmMd renders a ticket preview: everything is escaped first, then only
    // fenced code, ## and ###-###### headings, - and * list items, inline
    // code and **bold** become markup, and other lines paragraphs.
    function tmHtml(s) {
      return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
    }
    function tmInline(s) {
      return String(s).split(/(`[^`]+`)/).map(function (part, k) {
        if (k % 2) return "<code>" + tmHtml(part.slice(1, -1)) + "</code>";
        return tmHtml(part).replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>");
      }).join("");
    }
    function tmMd(src) {
      var out = [], pre = null, list = false, m;
      function endList() { if (list) { out.push("</ul>"); list = false; } }
      String(src || "").split("\n").forEach(function (ln) {
        if (/^\s*```/.test(ln)) {
          if (pre) { out.push("<pre>" + tmHtml(pre.join("\n")) + "</pre>"); pre = null; } else { endList(); pre = []; }
          return;
        }
        if (pre) { pre.push(ln); return; }
        if ((m = /^\s*[-*]\s+(.*)$/.exec(ln))) {
          if (!list) { out.push("<ul>"); list = true; }
          out.push("<li>" + tmInline(m[1]) + "</li>");
          return;
        }
        endList();
        if ((m = /^##\s+(.*)$/.exec(ln))) out.push("<h4>" + tmInline(m[1]) + "</h4>");
        else if ((m = /^#{3,6}\s+(.*)$/.exec(ln))) out.push("<h5>" + tmInline(m[1]) + "</h5>");
        else if (ln.trim()) out.push("<p>" + tmInline(ln) + "</p>");
      });
      endList();
      if (pre) out.push("<pre>" + tmHtml(pre.join("\n")) + "</pre>");
      return out.join("");
    }
    // tm:pure-end

    var VIEW_KEY = "ws.ticketmap.view", PREFS_KEY = "ws.ticketmap.prefs";
    var ST_KEY = { ready: "js.tmap.st.ready", todo: "js.tmap.st.todo", idea: "js.tmap.st.idea", done: "js.tmap.st.done", dropped: "js.tmap.st.dropped" };
    var S = {
      view: "B", sel: null, hist: [], hp: -1, q: "", hits: null,
      st: tmDefaults().st, cl: new Set(), kw: null,
      simTh: 15, simUnlinked: false, simRefs: true, inspB: false,
      collapsed: new Set(), edge: tmDefaults().edge, simEdgeTh: 24,
      ego: null, cam: { x: 0, y: 0, k: 1 }
    };
    var G = null, back = null, E = {}, built = false, fetched = false, busy = false, seq = 0;
    var keysOn = false, lastFocus = null, fullFrom = null, fullNode = null, noteTimer = 0, rendered = {};
    var api = {};

    function el(tag, cls, text) {
      var e = doc.createElement(tag);
      if (cls) e.className = cls;
      if (text !== undefined && text !== null) e.textContent = String(text);
      return e;
    }
    function button(cls, text, icon) {
      var b = el("button", cls);
      b.type = "button";
      if (icon) b.appendChild(luIcon(icon));
      if (text) b.appendChild(el("span", "", text));
      return b;
    }
    function kbd(text) { return el("kbd", "d-tm-kbd", text); }
    function clusterLabel(c) {
      if (!c) return "";
      if (c.misc) return wsT("js.tmap.cluster.misc");
      return c.label || wsT("js.tmap.cluster.unnamed", c.id + 1);
    }
    function hue(e, c) { if (c) e.style.setProperty("--d-tm-h", String(c.hue)); return e; }
    function statusDot(n) {
      var d = el("span", "d-tm-sd d-tm-st-" + n.status, n.status === "done" ? "✓" : n.status === "dropped" ? "⊘" : "");
      d.title = wsT(ST_KEY[n.status] || "js.tmap.st.todo");
      return d;
    }
    function statusBadge(n) {
      var b = el("span", "d-tm-badge d-tm-st-" + n.status);
      b.appendChild(statusDot(n));
      b.appendChild(el("span", "", wsT(ST_KEY[n.status] || "js.tmap.st.todo")));
      return b;
    }
    function clusterPill(id) {
      var c = G && G.byCluster[id], p = hue(el("span", "d-tm-cpill"), c);
      p.appendChild(el("i", "d-tm-cdot"));
      p.appendChild(el("span", "", clusterLabel(c)));
      return p;
    }
    function catTag(n) { return el("span", "d-tm-cat d-tm-cat-" + n.cat, n.cat); }
    function chip(term, code) { return el("span", "d-tm-chip" + (code ? " d-tm-chip-code" : ""), term); }

    function cur() { return S.view === "C" ? api.graph : api.viewB; }
    function shown(n) { return tmShown(S, n); }
    function isHit(i) { return !!S.hits && S.hits.has(i); }
    function savePrefs() {
      save(PREFS_KEY, JSON.stringify({ simTh: S.simTh, simUnlinked: S.simUnlinked, simRefs: S.simRefs, st: S.st, simEdgeTh: S.simEdgeTh, edge: S.edge }));
    }
    function select(i, push) {
      if (!G || !G.nodes[i]) return;
      if (push !== false) tmHistPush(S, i);
      S.sel = i;
      var v = cur();
      if (v) v.focus(i);
    }
    function unselect() {
      S.sel = null;
      var v = cur();
      if (v && v.unfocus) v.unfocus();
    }
    function move(d) {
      var i = tmHistMove(S, d);
      if (i === null || !G || !G.nodes[i]) return false;
      S.sel = i;
      var v = cur();
      if (v) v.focus(i);
      return true;
    }

    // --- DOM: built on the first open --------------------------------
    function build() {
      if (built) return;
      built = true;
      // Tests read the state on the first open: the hook attribute may be
      // set any time before it (a framed page's URL is the shell's).
      if (root.hasAttribute("data-test-hooks") || /[?&]tmaptest=1(?:&|$)/.test(location.search)) {
        window.__tmap = S;
        window.__tmapc = api.graph ? api.graph.probe : null;
      }
      var p = tmPrefs(load(PREFS_KEY));
      S.simTh = p.simTh; S.simUnlinked = p.simUnlinked; S.simRefs = p.simRefs;
      S.st = p.st; S.edge = p.edge; S.simEdgeTh = p.simEdgeTh;
      S.view = tmView(load(VIEW_KEY));
      if (S.view === "C" && !api.graph) S.view = "B";
      back = el("div", "d-tm-back");
      back.id = "tmback";
      back.hidden = true;
      back.setAttribute("role", "dialog");
      back.setAttribute("aria-modal", "true");
      back.setAttribute("aria-labelledby", "tmtitle");
      var bar = el("div", "d-tm-bar");
      var brand = el("div", "d-tm-brand");
      brand.appendChild(el("i", "d-tm-logo"));
      var title = el("span", "", wsT("js.tmap.title"));
      title.id = "tmtitle";
      brand.appendChild(title);
      bar.appendChild(brand);
      var seg = el("div", "d-tm-seg");
      seg.setAttribute("role", "group");
      seg.setAttribute("aria-label", wsT("js.tmap.view.group"));
      E.vb = button("d-tm-segb", wsT("js.tmap.view.b"));
      E.vb.id = "tmviewb";
      E.vb.setAttribute("aria-label", wsT("js.tmap.view.b"));
      E.vb.appendChild(kbd("1"));
      E.vc = button("d-tm-segb", wsT("js.tmap.view.c"));
      E.vc.id = "tmviewc";
      E.vc.setAttribute("aria-label", wsT("js.tmap.view.c"));
      E.vc.appendChild(kbd("2"));
      E.vb.addEventListener("click", function () { setView("B"); });
      E.vc.addEventListener("click", function () { setView("C"); });
      seg.appendChild(E.vb);
      seg.appendChild(E.vc);
      bar.appendChild(seg);
      var search = el("label", "d-tm-search");
      search.appendChild(luIcon("search"));
      E.q = el("input");
      E.q.id = "tmq";
      E.q.type = "search";
      E.q.placeholder = wsT("js.tmap.search");
      E.q.setAttribute("aria-label", E.q.placeholder);
      E.q.autocomplete = "off";
      E.q.spellcheck = false;
      E.q.addEventListener("input", function () { setQuery(E.q.value); });
      search.appendChild(E.q);
      var hc = el("span", "d-tm-hc");
      E.qc = el("span", "d-tm-qc");
      E.qc.id = "tmqc";
      hc.appendChild(E.qc);
      hc.appendChild(kbd("/"));
      search.appendChild(hc);
      bar.appendChild(search);
      E.pills = el("div", "d-tm-pills");
      E.pill = {};
      TM_ST.forEach(function (s) {
        var b = button("d-tm-pill d-tm-st-" + s);
        b.setAttribute("data-st", s);
        var name = wsT(ST_KEY[s]), ref = s === "done" || s === "dropped", text = ref ? wsT("js.tmap.pill.ref", name) : name;
        b.appendChild(ref ? el("span", "d-tm-psym", s === "done" ? "✓" : "⊘") : el("i", "d-tm-pdot"));
        b.appendChild(el("span", "d-tm-pl", text));
        b.appendChild(el("span", "d-tm-n", "0"));
        b.title = wsT("js.tmap.pill.tip", text);
        b.setAttribute("aria-label", text);
        b.addEventListener("click", function () {
          S.st[s] = !S.st[s];
          savePrefs();
          filterChanged();
        });
        E.pill[s] = b;
        E.pills.appendChild(b);
      });
      bar.appendChild(E.pills);
      var clwrap = el("div", "d-tm-clwrap");
      E.clb = button("btn d-tm-btn", "", "list-filter");
      E.clb.id = "tmclusters";
      E.clb.setAttribute("aria-haspopup", "true");
      E.clb.setAttribute("aria-expanded", "false");
      E.clb.setAttribute("aria-controls", "tmclpop");
      E.clbl = el("span", "", wsT("js.tmap.clusters"));
      E.clb.appendChild(E.clbl);
      E.clb.setAttribute("aria-label", wsT("js.tmap.clusters"));
      E.clb.addEventListener("click", function () { popOpen(E.pop.hidden); });
      E.pop = el("div", "d-tm-pop");
      E.pop.id = "tmclpop";
      E.pop.hidden = true;
      E.pop.addEventListener("change", popChange);
      clwrap.appendChild(E.clb);
      clwrap.appendChild(E.pop);
      bar.appendChild(clwrap);
      E.kw = el("span", "d-tm-kw");
      E.kw.id = "tmkw";
      E.kw.hidden = true;
      E.kw.setAttribute("role", "status");
      E.kwt = el("span", "");
      E.kw.appendChild(E.kwt);
      var kwx = button("d-tm-kwx", "", "x");
      kwx.title = wsT("js.tmap.kw.clear");
      kwx.setAttribute("aria-label", kwx.title);
      kwx.addEventListener("click", function () { setKw(null); });
      E.kw.appendChild(kwx);
      bar.appendChild(E.kw);
      bar.appendChild(el("span", "d-tm-spacer"));
      E.stats = el("span", "d-tm-stats");
      E.stats.id = "tmstats";
      bar.appendChild(E.stats);
      E.refresh = button("btn d-tm-btn", wsT("js.tmap.refresh"), "refresh-cw");
      E.refresh.id = "tmrefresh";
      E.refresh.addEventListener("click", function () { fetchGraph(true); });
      bar.appendChild(E.refresh);
      E.close = button("btn d-tm-btn d-tm-icon", "", "x");
      E.close.id = "tmclose";
      E.close.title = wsT("js.tmap.close") + " (Esc)";
      E.close.setAttribute("aria-label", wsT("js.tmap.close"));
      E.close.addEventListener("click", function () { close(false); });
      bar.appendChild(E.close);
      back.appendChild(bar);

      var stage = el("div", "d-tm-stage");
      E.views = el("div", "d-tm-views");
      E.state = el("div", "d-tm-state");
      E.state.id = "tmstate";
      E.state.setAttribute("role", "status");
      E.views.appendChild(E.state);
      E.note = el("div", "d-tm-note");
      E.note.id = "tmnote";
      E.note.hidden = true;
      E.note.setAttribute("role", "alert");
      E.views.appendChild(E.note);
      E.vbox = {};
      [["B", "tmvb"], ["C", "tmvc"]].forEach(function (pair) {
        var v = pair[0] === "C" ? api.graph : api.viewB;
        if (!v) return;
        var box = el("section", "d-tm-view");
        box.id = pair[1];
        box.hidden = true;
        E.vbox[pair[0]] = box;
        E.views.appendChild(box);
        v.mount(box);
      });
      stage.appendChild(E.views);
      E.insp = el("aside", "d-tm-insp");
      E.insp.id = "tminsp";
      E.insp.hidden = true;
      var ih = el("div", "d-tm-ih");
      ih.appendChild(el("span", "", wsT("js.tmap.insp")));
      var imin = button("d-tm-imin", "", "chevron-right");
      imin.title = wsT("js.tmap.insp.min");
      imin.setAttribute("aria-label", imin.title);
      imin.addEventListener("click", function () {
        E.insp.classList.toggle("d-tm-insp-min");
        imin.setAttribute("aria-pressed", String(E.insp.classList.contains("d-tm-insp-min")));
        layout();
      });
      ih.appendChild(imin);
      E.insp.appendChild(ih);
      E.inspb = el("div", "d-tm-ib");
      E.inspb.id = "tminspb";
      E.insp.appendChild(E.inspb);
      stage.appendChild(E.insp);
      back.appendChild(stage);
      buildFull();
      back.addEventListener("click", function (e) {
        var t = e.target;
        if (!E.pop.hidden && !(t.closest && t.closest("#tmclpop, #tmclusters"))) popOpen(false);
      });
      E.vc.disabled = !api.graph;
      viewSync();
    }
    function buildFull() {
      E.full = el("div", "d-tm-full");
      E.full.id = "tmfull";
      E.full.hidden = true;
      E.full.setAttribute("role", "dialog");
      E.full.setAttribute("aria-modal", "true");
      E.full.setAttribute("aria-labelledby", "tmfullt");
      var box = el("div", "d-tm-fbox"), head = el("header", "d-tm-fhead");
      E.fullt = el("h3", "d-tm-ftitle");
      E.fullt.id = "tmfullt";
      head.appendChild(E.fullt);
      E.fullCard = button("btn d-tm-btn", wsT("js.tmap.full.card"), "external-link");
      E.fullCard.id = "tmfullcard";
      E.fullCard.addEventListener("click", handOff);
      head.appendChild(E.fullCard);
      E.fullx = button("btn d-tm-btn", wsT("js.tmap.full.close"));
      E.fullx.id = "tmfullx";
      E.fullx.appendChild(kbd("Esc"));
      E.fullx.addEventListener("click", function () { closeFull(true); });
      head.appendChild(E.fullx);
      box.appendChild(head);
      E.fullb = el("div", "d-tm-fbody");
      E.fullb.id = "tmfullb";
      box.appendChild(E.fullb);
      E.full.appendChild(box);
      E.full.addEventListener("click", function (e) { if (e.target === E.full) closeFull(true); });
      back.appendChild(E.full);
    }

    // --- the top bar's state -----------------------------------------
    function viewSync() {
      E.vb.setAttribute("aria-pressed", String(S.view === "B"));
      E.vc.setAttribute("aria-pressed", String(S.view === "C"));
      var ready = !!G && E.state.hidden;
      Object.keys(E.vbox).forEach(function (k) { E.vbox[k].hidden = !ready || k !== S.view; });
    }
    function barSync() {
      TM_ST.forEach(function (s) {
        E.pill[s].setAttribute("aria-pressed", String(!!S.st[s]));
        E.pill[s].querySelector(".d-tm-n").textContent = String(G ? G.count[s] : 0);
      });
      E.qc.textContent = S.hits ? wsTn("js.tmap.hits", S.hits.size) : "";
      var total = G ? G.clusters.length : 0, hidden = 0;
      if (G) G.clusters.forEach(function (c) { if (S.cl.has(c.id)) hidden++; });
      E.clbl.textContent = hidden ? wsT("js.tmap.clusters.some", total - hidden, total) : wsT("js.tmap.clusters");
      E.clb.setAttribute("aria-pressed", String(hidden > 0));
      // View C shows the keyword chip in its own toolbar.
      E.kw.hidden = !S.kw || S.view === "C";
      if (S.kw) {
        E.kwt.textContent = wsT("js.tmap.kw", S.kw.term, S.kw.ids.size);
        hue(E.kw, G && G.byCluster[S.kw.cluster]);
      }
      if (G) {
        var m = G.meta;
        E.stats.textContent = wsT("js.tmap.stats", m.open || 0, m.refs || 0, G.clusters.length);
        E.stats.title = wsT("js.tmap.stats.tip", m.refDone || 0, m.refDropped || 0, m.dangling || 0, m.gamma || 0, m.modularity || 0, m.dfCut || 0);
      } else { E.stats.textContent = ""; E.stats.title = ""; }
    }
    function filterChanged() {
      barSync();
      var v = cur();
      if (G && v && v.filter) v.filter();
    }
    function setQuery(q) {
      S.q = q;
      S.hits = G ? tmHits(G.nodes, q) : null;
      filterChanged();
    }
    // setKw sets the keyword filter: a cluster's soup item
    // ({term, ticketIds}) and its cluster id, or null.
    function setKw(item, cluster) {
      S.kw = item ? { term: item.term, cluster: cluster, ids: new Set(item.ticketIds || []) } : null;
      filterChanged();
    }
    function setView(v) {
      if (!built || !E.vbox[v] || v === S.view) return;
      var old = cur();
      if (v !== S.view && old && old.hide) old.hide();
      S.view = v;
      save(VIEW_KEY, v);
      viewSync();
      barSync();
      showView();
    }
    // showView draws the shown view once per load, then lets it show.
    function showView() {
      var v = cur();
      if (!v || !G || !E.state.hidden) return;
      if (!rendered[S.view]) {
        rendered[S.view] = true;
        v.render({});
        if (S.sel !== null && v.focus) v.focus(S.sel);
      }
      if (v.show) v.show();
    }
    function layout() { var v = cur(); if (v && v.layout) v.layout(); }
    function inspShow(on) {
      if (!built) return;
      E.insp.hidden = !on;
      layout();
    }

    // --- the cluster filter popover ------------------------------------
    function popOpen(on) {
      if (!built) return;
      if (on && G) popRender();
      E.pop.hidden = !on || !G;
      E.clb.setAttribute("aria-expanded", String(!E.pop.hidden));
      if (!E.pop.hidden) { var f = E.pop.querySelector("input"); if (f) f.focus(); }
    }
    function popRow(id, label, count, c) {
      var row = el("label", "d-tm-clrow"), box = el("input");
      box.type = "checkbox";
      box.setAttribute("data-cl", id);
      row.appendChild(box);
      if (c) row.appendChild(hue(el("i", "d-tm-cdot"), c));
      row.appendChild(el("span", "d-tm-cll", label));
      row.appendChild(el("span", "d-tm-n", String(count)));
      return row;
    }
    function popRender() {
      E.pop.textContent = "";
      var count = {};
      G.nodes.forEach(function (n) { count[n.cluster] = (count[n.cluster] || 0) + 1; });
      E.pop.appendChild(popRow("all", wsT("js.tmap.clusters.all"), G.nodes.length, null));
      G.clusters.forEach(function (c) { E.pop.appendChild(popRow(String(c.id), clusterLabel(c), count[c.id] || 0, c)); });
      popSync();
    }
    function popSync() {
      var hidden = 0;
      G.clusters.forEach(function (c) { if (S.cl.has(c.id)) hidden++; });
      Array.prototype.forEach.call(E.pop.querySelectorAll("input"), function (b) {
        var id = b.getAttribute("data-cl");
        if (id === "all") { b.checked = hidden === 0; b.indeterminate = hidden > 0 && hidden < G.clusters.length; }
        else b.checked = !S.cl.has(+id);
      });
    }
    function popChange(e) {
      var b = e.target, id = b && b.getAttribute && b.getAttribute("data-cl");
      if (!id || !G) return;
      if (id === "all") {
        if (b.checked) S.cl.clear();
        else G.clusters.forEach(function (c) { S.cl.add(c.id); });
      } else if (b.checked) S.cl.delete(+id);
      else S.cl.add(+id);
      popSync();
      filterChanged();
    }

    // --- data ---------------------------------------------------------
    function setState(kind) {
      E.state.textContent = "";
      E.state.hidden = !kind;
      if (kind) {
        E.state.className = "d-tm-state d-tm-state-" + kind;
        if (kind === "loading") {
          var sk = el("div", "d-tm-skel");
          for (var k = 0; k < 4; k++) sk.appendChild(el("i"));
          E.state.appendChild(sk);
        }
        E.state.appendChild(el("p", "", wsT(kind === "loading" ? "js.tmap.loading" : kind === "empty" ? "js.tmap.empty" : kind === "unavailable" ? "js.tmap.unavailable" : "js.tmap.error")));
        if (kind === "error") {
          var retry = button("btn d-tm-btn", wsT("js.tmap.retry"), "refresh-cw");
          retry.addEventListener("click", function () { fetchGraph(false); });
          E.state.appendChild(retry);
        }
      }
      viewSync();
    }
    function notice(on) {
      clearTimeout(noteTimer);
      E.note.textContent = "";
      E.note.hidden = !on;
      if (!on) return;
      E.note.appendChild(el("span", "", wsT("js.tmap.error")));
      var retry = button("btn d-tm-btn", wsT("js.tmap.retry"), "refresh-cw");
      retry.addEventListener("click", function () { notice(false); fetchGraph(true); });
      E.note.appendChild(retry);
      noteTimer = setTimeout(function () { notice(false); }, 8000);
    }
    function fetchGraph(refresh) {
      if (busy) return;
      busy = true;
      fetched = true;
      var my = ++seq;
      E.refresh.disabled = true;
      if (!G) setState("loading");
      fetch("/s/" + rid + "/tickets/graph", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
        if (!r.ok) { var err = new Error("ticket graph"); err.status = r.status; throw err; }
        return r.json();
      }).then(function (data) {
        if (my !== seq) return;
        busy = false;
        E.refresh.disabled = false;
        loaded(data, refresh);
      }).catch(function (err) {
        if (my !== seq) return;
        busy = false;
        E.refresh.disabled = false;
        // A loaded graph is never replaced by an error: it stays, under a
        // transient notice.
        if (G) notice(true);
        else setState(err && err.status === 404 ? "unavailable" : "error");
      });
    }
    function loaded(data, refresh) {
      var old = G, v = cur(), wasFocus = !!(v && v.isFocus && v.isFocus());
      var oldSel = old && S.sel !== null && old.nodes[S.sel] ? old.nodes[S.sel].stem : null;
      notice(false);
      if (!data || !data.nodes || !data.nodes.length) {
        G = null;
        S.sel = null; S.hist = []; S.hp = -1; S.hits = null; S.kw = null;
        rendered = {};
        setState("empty");
        barSync();
        return;
      }
      G = tmDerive(data, clusterLabel);
      if (old) {
        tmHistRemap(S, function (i) { return old.nodes[i] && old.nodes[i].stem; }, function (s) { return G.byStem[s]; });
        S.sel = oldSel !== null && G.byStem[oldSel] !== undefined ? G.byStem[oldSel] : null;
        if (S.kw) {
          var kw = S.kw, list = G.kw[String(kw.cluster)] || [], hit = null;
          list.forEach(function (it) { if (it.term === kw.term) hit = it; });
          S.kw = hit ? { term: kw.term, cluster: kw.cluster, ids: new Set(hit.ticketIds || []) } : null;
        }
      }
      S.hits = tmHits(G.nodes, S.q);
      rendered = {};
      setState("");
      barSync();
      if (built && !E.pop.hidden) popRender();
      v = cur();
      if (!v) return;
      rendered[S.view] = true;
      v.render({ refresh: !!refresh, keepScroll: !!old && old.clusters.length === G.clusters.length });
      // render() leaves the focus mode; a focused view whose ticket
      // survived focuses it again (View C keeps any selection).
      if (S.sel !== null && (wasFocus || S.view === "C")) v.focus(S.sel);
      else S.sel = null;
      if (v.show) v.show();
    }

    // --- Full View and the file-card hand-off --------------------------
    function openFull(i) {
      if (!G || !G.nodes[i]) return;
      var n = G.nodes[i];
      fullNode = n;
      fullFrom = doc.activeElement;
      E.fullt.textContent = n.title;
      E.fullb.textContent = "";
      var meta = el("div", "d-tm-fmeta");
      meta.appendChild(statusBadge(n));
      meta.appendChild(clusterPill(n.cluster));
      meta.appendChild(el("span", "d-tm-tag", n.stem + ".md"));
      meta.appendChild(el("span", "d-tm-tag", wsTn("js.tmap.full.lines", n.lines || 0)));
      E.fullb.appendChild(meta);
      var md = el("div", "d-tm-md");
      md.innerHTML = tmMd(n.preview || "");
      E.fullb.appendChild(md);
      if (/…\s*$/.test(n.preview || "")) E.fullb.appendChild(el("p", "d-tm-fnote", wsT("js.tmap.full.truncated", n.lines || 0)));
      E.fullCard.hidden = !(window.WsPreview && cardBoard && n.path && window.WsPreview.takes(n.path));
      E.full.hidden = false;
      E.fullx.focus();
    }
    function closeFull(refocus) {
      if (!built || E.full.hidden) return;
      E.full.hidden = true;
      if (refocus && fullFrom && fullFrom.isConnected && back.contains(fullFrom)) fullFrom.focus();
      fullFrom = null;
    }
    // handOff is decision (26): close the modal, yield the overlay with
    // every state kept, then open or promote the ticket's durable card.
    function handOff() {
      var n = fullNode;
      closeFull(false);
      close(true);
      if (n && window.WsPreview) window.WsPreview.open(n.path, { keep: true });
    }

    // --- open, close, keys ----------------------------------------------
    function isOpen() { return !!back && !back.hidden; }
    // dismiss closes the page's transient popovers, modals, and the
    // drawer, so their load-time Escape listeners stay inert under the map.
    function dismiss() {
      if (lmenu) closeLinkMenu(false);
      if (rp && rp.P) railPopClose(false);
      [rgit, rdel, rsleep].forEach(function (m) { if (m && m.el && m.el.open) m.close(); });
      if (gw && gw.back && !gw.back.hidden) gwClose();
      if (modelHistoryOpen) modelHistoryClose(false);
      if (drawer && drOpen) drSetOpen(false);
    }
    function open() {
      if (isOpen()) return;
      dismiss();
      var first = !built;
      build();
      doc.body.appendChild(back);
      back.hidden = false;
      if (thandle) thandle.setAttribute("aria-expanded", "true");
      if (!keysOn) {
        keysOn = true;
        window.addEventListener("keydown", onKey, true);
        doc.addEventListener("focusin", onFocusIn, true);
      }
      if (!fetched) fetchGraph(false);
      else showView();
      if (first || !lastFocus || !lastFocus.isConnected || !back.contains(lastFocus)) E.q.focus();
      else lastFocus.focus();
    }
    // close hides the overlay and keeps every state; yieldTo (the
    // file-card hand-off) leaves the focus to what opens next.
    function close(yieldTo) {
      if (!isOpen()) return;
      // A click on a non-focusable part of the modal overlay leaves the
      // focus on the body: the overlay still held it.
      var active = doc.activeElement, inside = back.contains(active), held = inside || !active || active === doc.body;
      if (inside) lastFocus = active;
      closeFull(false);
      popOpen(false);
      var v = cur();
      if (v && v.hide) v.hide();
      back.hidden = true;
      if (keysOn) {
        keysOn = false;
        window.removeEventListener("keydown", onKey, true);
        doc.removeEventListener("focusin", onFocusIn, true);
      }
      if (thandle) thandle.setAttribute("aria-expanded", "false");
      if (held && !yieldTo && thandle && !thandle.hidden) thandle.focus();
    }
    function toggle() { if (isOpen()) close(false); else open(); }
    function focusSearch() { E.q.focus(); E.q.select(); }
    function escape() {
      var v = cur();
      var step = tmEscStep({
        full: !E.full.hidden, pop: !E.pop.hidden, inQuery: doc.activeElement === E.q, query: E.q.value,
        view: S.view, ego: !!(v && v.isEgo && v.isEgo()), kw: !!S.kw, sel: S.sel,
        focus: !!(G && v && v.isFocus && v.isFocus())
      });
      if (step === "full") closeFull(true);
      else if (step === "pop") { popOpen(false); E.clb.focus(); }
      else if (step === "query") { E.q.value = ""; setQuery(""); }
      else if (step === "ego") v.exitEgo();
      else if (step === "kw") setKw(null);
      else if (step === "deselect" || step === "unfocus") unselect();
      else close(false);
    }
    function typing(t) {
      if (!t || !t.closest) return false;
      if (t.tagName === "INPUT" && (t.type === "range" || t.type === "checkbox")) return false;
      return !!t.closest("input, textarea, select, [contenteditable]:not([contenteditable=\"false\"])");
    }
    // onKey is the overlay's one key listener (window, capture, while
    // shown): it runs before every page and drawer listener, consumes the
    // map's keys, and stops every key from reaching the page underneath
    // (the default action, typing or a native control, still happens).
    function onKey(e) {
      if (!isOpen()) return;
      e.stopImmediatePropagation();
      if (e.isComposing || e.keyCode === 229) return;
      var key = e.key || "", t = e.target, v = cur(), done = false;
      // The search palette's keys (lifecycle's shell hotkeys, qShortcut).
      var k = key.toLowerCase() === "k", space = e.code === "Space" || key === " ";
      var palette = !e.altKey && !e.shiftKey && (e.metaKey ? !e.ctrlKey && k : e.ctrlKey && (k || space));
      if (key === "Escape") { escape(); done = true; }
      else if (palette) {
        if (E.full.hidden) focusSearch();
        done = true;
      } else if (!E.full.hidden || e.ctrlKey || e.metaKey) done = false;
      else if (t === E.q && key === "Enter") {
        var i = G ? tmFirstHit(S, G.nodes) : null;
        if (i !== null) select(i);
        done = true;
      } else if (typing(t)) done = false;
      else if (!e.altKey && key === "1") { setView("B"); done = true; }
      else if (!e.altKey && key === "2") { setView("C"); done = true; }
      else if (!e.altKey && key === "/") { focusSearch(); done = true; }
      else if ((key === "ArrowLeft" || key === "ArrowRight") && S.view === "B" && G && v && v.isFocus && v.isFocus()) {
        move(key === "ArrowLeft" ? -1 : 1);
        done = true;
      } else if (G && v && v.key) done = !!v.key(e);
      if (done) e.preventDefault();
    }
    // Tab stays inside the overlay (inside the Full View while it shows).
    function onFocusIn(e) {
      if (!isOpen()) return;
      var box = E.full.hidden ? back : E.full, t = e.target;
      if (t && box.contains(t)) return;
      (E.full.hidden ? E.close : E.fullx).focus();
    }

    api.S = S;
    api.ST = TM_ST;
    api.REL = TM_REL;
    api.open = open;
    api.close = function () { close(false); };
    api.toggle = toggle;
    api.isOpen = isOpen;
    api.data = function () { return G; };
    api.el = el;
    api.button = button;
    api.kbd = kbd;
    api.icon = luIcon;
    api.clusterLabel = clusterLabel;
    api.hue = hue;
    api.statusDot = statusDot;
    api.statusBadge = statusBadge;
    api.clusterPill = clusterPill;
    api.catTag = catTag;
    api.chip = chip;
    api.shown = shown;
    api.isHit = isHit;
    api.bucket = tmBucket;
    api.simOf = function (a, b) { var s = G && G.nodes[a] && G.nodes[a].simMap.get(b); return s ? s.s : 0; };
    api.select = select;
    api.unselect = unselect;
    api.move = move;
    api.openFull = openFull;
    api.setKw = setKw;
    api.savePrefs = savePrefs;
    api.setView = setView;
    api.insp = function () { return built ? E.inspb : null; };
    api.inspShow = inspShow;
    api.viewB = null;
    api.graph = null;
    return api;
  })();
