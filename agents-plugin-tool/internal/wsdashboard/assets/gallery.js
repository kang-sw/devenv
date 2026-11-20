(function (G) {
  // The paged docking gallery's model (window.WsGallery): pure data and
  // timing, no DOM. The card framework (cards.js) owns the nodes and asks
  // this module where each docked card or topic stands, which page shows,
  // and what a wheel event or a hover over a rail dot does. Node tests load
  // the same file through module.exports.
  if (typeof module === "object" && module && module.exports) module.exports = G;
  if (typeof window !== "undefined" && window) window.WsGallery = G;
})((function () {
  "use strict";

  // The constants are the approved prototype's (paged-docking-gallery v4).
  var C = {
    WIDE: 1600,          // px: minimum viewport where 4-column docking may activate
    SNAP: 320,           // flip tension that is 100%
    TENSION_MAX: 125,    // tension one wheel event adds at most
    RUBBER_PX: 26,       // rubber band translation at 100%
    RUBBER_K: 2.3,       // its easing: RUBBER_PX * (1 - exp(-RUBBER_K * pct))
    READY: 0.85,         // the pill reads "ready" from here
    IDLE_MS: 240,        // a pull below 100% springs back after this idle
    INERTIA_MS: 170,     // a wheel this close to the last one is momentum
    MICRO_PX: 5,         // the firm micro-bounce
    MICRO_MS: 220,       // the micro-bounce clears after this idle
    LOCK_MS: 420,        // wheel events are ignored this long after a flip
    OVERSHOOT_PX: 28,    // the flip's slide settles from this offset
    SPRING_MS: 300,      // hovering a dragged pane over a rail dot switches pages
    RAIL_WHEEL_MS: 140,  // one wheel tick on the rail is one page, debounced
    EPS: 2,              // px: a scroller within this of its limit is at it
    RATIO_MIN: 0.1,      // a split divider's ratio is clamped to this range
    RATIO_MAX: 0.9,
    CENTER: 0.25         // a drop farther than this (of the pane) from every edge is the center zone
  };

  // --- references and the split tree ---------------------------------------
  // A docked item's reference is "c:<card id>" or "t:<topic name>". A tree
  // node is a leaf, {card: id} or {topic: name}, or a split, {axis, ratio,
  // a, b}: axis "row" puts a left of b, "col" puts a above b, and ratio is
  // a's share.
  function refOf(n) {
    if (!n || typeof n !== "object") return null;
    if (typeof n.card === "string" && n.card) return "c:" + n.card;
    if (typeof n.topic === "string" && n.topic) return "t:" + n.topic;
    return null;
  }
  function leaf(ref) { return ref.charAt(0) === "c" ? { card: ref.slice(2) } : { topic: ref.slice(2) }; }
  function isSplit(n) { return !!n && typeof n === "object" && (n.axis === "row" || n.axis === "col") && "a" in n && "b" in n; }
  function clampRatio(r) {
    r = +r;
    if (!isFinite(r)) return 0.5;
    return Math.min(C.RATIO_MAX, Math.max(C.RATIO_MIN, r));
  }
  // normalize is a clean copy of node: leaves that are no reference, that
  // keep rejects, or that seen already holds go, a split left with one
  // child is that child, ratios are clamped. null when nothing is left.
  function normalize(n, keep, seen) {
    seen = seen || {};
    if (isSplit(n)) {
      var a = normalize(n.a, keep, seen), b = normalize(n.b, keep, seen);
      if (!a) return b;
      if (!b) return a;
      return { axis: n.axis, ratio: clampRatio(n.ratio), a: a, b: b };
    }
    var ref = refOf(n);
    if (ref === null || seen[ref] || (keep && !keep(ref))) return null;
    seen[ref] = true;
    return leaf(ref);
  }
  // leaves is the tree's references depth first, a before b.
  function leaves(n, out) {
    out = out || [];
    if (isSplit(n)) { leaves(n.a, out); leaves(n.b, out); }
    else { var ref = refOf(n); if (ref !== null) out.push(ref); }
    return out;
  }
  function count(n) { return leaves(n).length; }
  function removeRef(n, ref) { return normalize(n, function (r) { return r !== ref; }); }
  // splitAt docks ref beside the leaf target: zone "left"/"right" splits it
  // in a row, "top"/"bottom" in a column, ref on the zone's side.
  function splitAt(n, target, zone, ref) {
    if (isSplit(n)) return { axis: n.axis, ratio: n.ratio, a: splitAt(n.a, target, zone, ref), b: splitAt(n.b, target, zone, ref) };
    if (refOf(n) !== target) return n;
    var axis = zone === "left" || zone === "right" ? "row" : "col";
    return zone === "left" || zone === "top" ? { axis: axis, ratio: 0.5, a: leaf(ref), b: n } : { axis: axis, ratio: 0.5, a: n, b: leaf(ref) };
  }
  function mapLeaves(n, fn) {
    if (isSplit(n)) return { axis: n.axis, ratio: n.ratio, a: mapLeaves(n.a, fn), b: mapLeaves(n.b, fn) };
    var ref = refOf(n);
    return ref === null ? n : leaf(fn(ref));
  }
  // A path names a split from the root: "" the root, then "a"/"b" per step.
  function nodeAt(n, path) {
    for (var i = 0; i < path.length && n; i++) n = isSplit(n) ? n[path.charAt(i)] : null;
    return n || null;
  }
  function withRatio(n, path, r) {
    if (!isSplit(n)) return n;
    if (!path) return { axis: n.axis, ratio: clampRatio(r), a: n.a, b: n.b };
    var k = path.charAt(0), out = { axis: n.axis, ratio: n.ratio, a: n.a, b: n.b };
    out[k] = withRatio(n[k], path.slice(1), r);
    return out;
  }

  // --- the gallery ---------------------------------------------------------
  // {v: 1, active, pages: [{id, root}], rev}. active is the page shown: a
  // page id, "+" (the trailing empty page), or null for home (Page 0, the
  // Main Board, in the narrow view; the first page, or + without one, in the
  // wide view). rev counts changes; new page ids come from it.
  var PLUS = "+";
  function empty() { return { v: 1, active: null, pages: [], rev: 0 }; }
  function copy(g) {
    return { v: 1, active: g.active, rev: +g.rev || 0, pages: g.pages.map(function (p) { return { id: p.id, root: p.root }; }) };
  }
  function pageIndex(g, id) {
    for (var i = 0; i < g.pages.length; i++) if (g.pages[i].id === id) return i;
    return -1;
  }
  function pageOf(g, ref) {
    for (var i = 0; i < g.pages.length; i++) if (leaves(g.pages[i].root).indexOf(ref) >= 0) return i;
    return -1;
  }
  function refs(g) {
    var out = [];
    g.pages.forEach(function (p) { leaves(p.root, out); });
    return out;
  }
  // ordinals maps each docked reference to its flat pin order: 1-based,
  // depth first within a page, pages in order. A previous build reads these
  // as its flat pins (cards[].pinned, topics[].p).
  function ordinals(g) {
    var o = {};
    refs(g).forEach(function (ref, i) { o[ref] = i + 1; });
    return o;
  }
  function newPage(g, root) {
    var id;
    do { g.rev++; id = "p" + g.rev.toString(36); } while (pageIndex(g, id) >= 0);
    var p = { id: id, root: root };
    g.pages.push(p);
    return p;
  }
  // pull takes ref off its page; a page left empty goes, and the shown
  // page moves to the one that took its index (or the last one). With no
  // page left nothing is shown (null), + included: a + kept over zero
  // pages would stand the narrow view off its Main Board.
  function pull(g, ref) {
    var i = pageOf(g, ref);
    if (i < 0) return;
    var root = removeRef(g.pages[i].root, ref);
    if (root) { g.pages[i] = { id: g.pages[i].id, root: root }; return; }
    var gone = g.pages[i].id;
    g.pages.splice(i, 1);
    if (!g.pages.length) g.active = null;
    else if (g.active === gone) g.active = g.pages[Math.min(i, g.pages.length - 1)].id;
  }
  // dock puts ref on a new page before + and shows it (the pin button, a
  // drop on +).
  function dock(g, ref) {
    g = copy(g);
    pull(g, ref);
    g.active = newPage(g, leaf(ref)).id;
    return g;
  }
  // undock takes ref off the gallery (back to the Main Board).
  function undock(g, ref) {
    g = copy(g);
    if (pageOf(g, ref) < 0) return g;
    g.rev++;
    pull(g, ref);
    return g;
  }
  // append docks ref on the page at pos below what it holds, and shows it
  // (a drop on that page's rail dot); pos at or past the pages is a new
  // page. A ref already on that page stays.
  function append(g, ref, pos) {
    if (pos >= g.pages.length) return dock(g, ref);
    if (pos < 0) return g;
    var id = g.pages[pos].id;
    if (pageOf(g, ref) === pos) return g;
    g = copy(g);
    g.rev++;
    pull(g, ref);
    var i = pageIndex(g, id);
    g.pages[i] = { id: id, root: { axis: "col", ratio: 0.5, a: g.pages[i].root, b: leaf(ref) } };
    g.active = id;
    return g;
  }
  // place docks ref at zone of the docked pane target: an edge splits it,
  // the center swaps the two (both docked). It shows target's page.
  function place(g, ref, target, zone) {
    if (ref === target || pageOf(g, target) < 0) return g;
    if (zone === "center") return swap(g, ref, target);
    g = copy(g);
    g.rev++;
    pull(g, ref);
    var i = pageOf(g, target);
    g.pages[i] = { id: g.pages[i].id, root: splitAt(g.pages[i].root, target, zone, ref) };
    g.active = g.pages[i].id;
    return g;
  }
  // swap exchanges two docked panes, across pages too; it shows the page
  // where x lands.
  function swap(g, x, y) {
    if (x === y || pageOf(g, x) < 0 || pageOf(g, y) < 0) return g;
    g = copy(g);
    g.rev++;
    g.pages = g.pages.map(function (p) {
      return { id: p.id, root: mapLeaves(p.root, function (r) { return r === x ? y : r === y ? x : r; }) };
    });
    g.active = g.pages[pageOf(g, x)].id;
    return g;
  }
  // movePage moves the page at from to index to (a rail dot dragged).
  function movePage(g, from, to) {
    if (from < 0 || from >= g.pages.length || to < 0 || to >= g.pages.length || from === to) return g;
    g = copy(g);
    g.rev++;
    var p = g.pages.splice(from, 1)[0];
    g.pages.splice(to, 0, p);
    return g;
  }
  function setRatio(g, id, path, r) {
    var i = pageIndex(g, id);
    if (i < 0 || !isSplit(nodeAt(g.pages[i].root, path))) return g;
    g = copy(g);
    g.rev++;
    g.pages[i] = { id: id, root: withRatio(g.pages[i].root, path, r) };
    return g;
  }

  // --- where the view stands -----------------------------------------------
  // A position is -1 for Page 0 (narrow only), 0..N-1 for the docked pages,
  // and N for +. The narrow view with no docked page is at home even on +:
  // its rail is hidden at rest, so a + there could only hide the board.
  function minPos(narrow) { return narrow ? -1 : 0; }
  function position(g, narrow) {
    var n = g.pages.length;
    if (g.active === PLUS) return narrow && !n ? -1 : n;
    var i = g.active === null ? -1 : pageIndex(g, g.active);
    if (i >= 0) return i;
    return narrow ? -1 : n ? 0 : n;
  }
  function activate(g, pos, narrow) {
    var n = g.pages.length;
    pos = Math.max(minPos(narrow), Math.min(n, pos));
    var a = pos === n ? PLUS : pos < 0 ? null : g.pages[pos].id;
    if (a === g.active) return g;
    g = copy(g);
    g.active = a;
    return g;
  }
  // neighbors tells the bounce-snap gate what lies past the page at pos:
  // true an adjacent page, false an edge (a damped micro-bounce only), null
  // nothing at all (the gate stays inert when there are no docked pages).
  function neighbors(g, pos, narrow) {
    var n = g.pages.length;
    if (!n) return { prev: null, next: null };
    return { prev: pos > minPos(narrow), next: pos < n };
  }

  // --- storage -------------------------------------------------------------
  // reconcile reads a stored gallery against the flat pins (flat: [{ref, p}],
  // every item whose flag says docked, p its pin order). Without a stored
  // gallery each flat pin is a page of its own, in pin order. With one, a
  // leaf whose item lost its flag or is gone leaves (a page left empty
  // goes, a split left one child collapses), and a flat pin no leaf
  // references is appended as a page of its own, in pin order. A previous
  // build that rewrote the record is how the two disagree.
  function reconcile(stored, flat) {
    var want = {}, g = empty();
    (flat || []).forEach(function (f) { if (f && typeof f.ref === "string") want[f.ref] = +f.p || 0; });
    if (stored && typeof stored === "object" && Array.isArray(stored.pages)) {
      var seen = {}, keep = function (r) { return Object.prototype.hasOwnProperty.call(want, r); }, ids = {};
      g.rev = isFinite(+stored.rev) ? Math.max(0, Math.floor(+stored.rev)) : 0;
      stored.pages.forEach(function (p) {
        if (!p || typeof p !== "object" || typeof p.id !== "string" || !p.id || ids[p.id]) return;
        var root = normalize(p.root, keep, seen);
        if (!root) return;
        ids[p.id] = true;
        g.pages.push({ id: p.id, root: root });
      });
      if (stored.active === PLUS || (typeof stored.active === "string" && ids[stored.active])) g.active = stored.active;
    }
    var have = {};
    refs(g).forEach(function (r) { have[r] = true; });
    Object.keys(want).filter(function (r) { return !have[r]; }).sort(function (a, b) { return want[a] - want[b] || (a < b ? -1 : a > b ? 1 : 0); }).forEach(function (r) {
      newPage(g, leaf(r));
    });
    if (!g.pages.length) g.active = null;
    return g;
  }
  // flatPins reads a stored card record's flat pins: cards[].pinned and
  // topics[].p above 0 (cardOk filters cards, e.g. to registered kinds).
  // A record from before topics joined is the bare cards list.
  function flatPins(raw, cardOk) {
    if (Array.isArray(raw)) raw = { cards: raw };
    if (!raw || typeof raw !== "object") return [];
    var out = [];
    (Array.isArray(raw.cards) ? raw.cards : []).forEach(function (c) {
      if (c && typeof c.id === "string" && c.id && +c.pinned > 0 && (!cardOk || cardOk(c))) out.push({ ref: "c:" + c.id, p: +c.pinned });
    });
    (Array.isArray(raw.topics) ? raw.topics : []).forEach(function (t) {
      if (t && typeof t.n === "string" && t.n && +t.p > 0) out.push({ ref: "t:" + t.n, p: +t.p });
    });
    return out;
  }
  // load is the gallery a stored record holds: its gallery field reconciled
  // against its flat pins.
  function load(raw, cardOk) {
    return reconcile(raw && !Array.isArray(raw) && typeof raw === "object" ? raw.gallery : null, flatPins(raw, cardOk));
  }
  // record is what persist writes: the gallery as stored.
  function record(g) {
    return { v: 1, active: g.active, pages: g.pages.map(function (p) { return { id: p.id, root: p.root }; }), rev: g.rev };
  }

  // --- layout --------------------------------------------------------------
  // layout maps the viewport to the gallery's shape. o: width (px), tabs
  // (the layout menu says one column at a time), pages (docked pages), and
  // dragging (a card or topic drag runs). narrow: the Main Board is Page 0
  // of one paged view (viewport < WIDE, tabs mode, or spare width narrower
  // than the board pane). column: the 4-column wide view shows the gallery
  // column beside the board (never an empty one, save while a drag runs).
  // overlay: legacy field kept false for shape stability. rail: the
  // gallery's dot rail shows. empty: in 4-column mode with no docked pages
  // and no drag, the idle dashed slot placeholder shows.
  function layout(o) {
    var four = !o.tabs && o.width >= C.WIDE && !!o.fourCol, any = o.pages > 0, drag = !!o.dragging;
    return { narrow: !four, column: four && (any || drag), overlay: false, rail: any || drag, empty: four && !any && !drag };
  }

  // --- drops ---------------------------------------------------------------
  // zone reads where (x, y) falls in rect: the nearest edge, or "center"
  // when center drops are allowed and the point is farther than CENTER of
  // the pane from every edge.
  function zone(rect, x, y, center) {
    var fx = (x - rect.left) / (rect.width || 1), fy = (y - rect.top) / (rect.height || 1);
    var m = Math.min(fx, 1 - fx, fy, 1 - fy);
    if (center && m > C.CENTER) return "center";
    return m === fx ? "left" : m === 1 - fx ? "right" : m === fy ? "top" : "bottom";
  }

  // --- timing --------------------------------------------------------------
  // Gate is the boundary overscroll gate. Each wheel event reports e: dy,
  // now (ms), canUp and canDown (the scroller under the pointer can still
  // scroll that way; false without one), and prev and next (neighbors). It
  // answers what the view does:
  //   none    nothing (no flip semantics here; leave the event alone)
  //   pass    the scroller consumes it (reset: a pull showing springs back)
  //   ignore  the refractory lock after a flip (swallow the event)
  //   micro   the firm micro-bounce, MICRO_PX, pill damped at 0%: momentum
  //           (edge false) or an edge without a page past it (edge true);
  //           it charges no tension
  //   reset   the direction reversed: the pull springs back
  //   pull    tension charged: pct (0..1), shift (px), ready
  //   flip    100% reached: flip to the page past dir, then lock
  // tick(now) settles idle state: "spring" a pull below 100% idle IDLE_MS,
  // "clear" a micro-bounce idle MICRO_MS, else null; due() is when to ask.
  function Gate(o) {
    this.snap = (o && +o.snap) || C.SNAP;
    this.last = -Infinity;
    this.lock = -Infinity;
    this.inertia = false;
    this.clear();
  }
  Gate.prototype.clear = function () { this.dir = 0; this.acc = 0; this.mode = ""; };
  Gate.prototype.wheel = function (e) {
    var dy = +e.dy || 0;
    if (Math.abs(dy) < 1) return { kind: "none" };
    var dir = dy > 0 ? 1 : -1, now = +e.now, gap = now - this.last, past = dir > 0 ? e.next : e.prev;
    if ((dir > 0 && e.canDown) || (dir < 0 && e.canUp)) {
      this.last = now;
      this.inertia = true;
      var was = this.mode;
      this.clear();
      return { kind: "pass", reset: !!was };
    }
    if (past === null || past === undefined) return { kind: "none" };
    this.last = now;
    if (now < this.lock) return { kind: "ignore" };
    if (this.inertia && gap >= C.INERTIA_MS) this.inertia = false;
    if (this.inertia || past === false) {
      this.dir = 0; this.acc = 0; this.mode = "micro";
      return { kind: "micro", dir: dir, shift: C.MICRO_PX, edge: past === false };
    }
    if (this.dir && this.dir !== dir) { this.clear(); return { kind: "reset" }; }
    this.dir = dir;
    this.mode = "pull";
    this.acc += Math.min(C.TENSION_MAX, Math.abs(dy));
    if (this.acc >= this.snap) {
      this.clear();
      this.lock = now + C.LOCK_MS;
      this.inertia = false;
      return { kind: "flip", dir: dir };
    }
    var pct = Math.min(1, this.acc / this.snap);
    return { kind: "pull", dir: dir, pct: pct, shift: rubber(pct), ready: pct >= C.READY };
  };
  Gate.prototype.due = function () {
    return this.mode === "pull" ? this.last + C.IDLE_MS : this.mode === "micro" ? this.last + C.MICRO_MS : null;
  };
  Gate.prototype.tick = function (now) {
    var d = this.due();
    if (d === null || now < d) return null;
    var was = this.mode;
    this.clear();
    return was === "pull" ? "spring" : "clear";
  };
  function rubber(pct) { return C.RUBBER_PX * (1 - Math.exp(-C.RUBBER_K * pct)); }

  // Hover is the spring-loaded rail dot: over(target, now) starts a hover
  // (true) unless target already is the one hovered; fire(now) answers the
  // target once SPRING_MS passed (and ends the hover), else null.
  function Hover(ms) { this.ms = ms || C.SPRING_MS; this.clear(); }
  Hover.prototype.clear = function () { this.target = null; this.since = 0; };
  Hover.prototype.over = function (target, now) {
    if (target === this.target) return false;
    this.target = target;
    this.since = now;
    return true;
  };
  Hover.prototype.fire = function (now) {
    if (this.target === null || now - this.since < this.ms) return null;
    var t = this.target;
    this.clear();
    return t;
  };
  // railStep tells whether a wheel tick on the rail at now turns a page,
  // the last one having turned at last.
  function railStep(last, now) { return now - last >= C.RAIL_WHEEL_MS; }

  return {
    C: C, PLUS: PLUS,
    refOf: refOf, leaf: leaf, isSplit: isSplit, clampRatio: clampRatio, normalize: normalize,
    leaves: leaves, count: count, removeRef: removeRef, splitAt: splitAt, nodeAt: nodeAt,
    empty: empty, pageIndex: pageIndex, pageOf: pageOf, refs: refs, ordinals: ordinals,
    dock: dock, undock: undock, append: append, place: place, swap: swap, movePage: movePage, setRatio: setRatio,
    position: position, activate: activate, neighbors: neighbors,
    reconcile: reconcile, flatPins: flatPins, load: load, record: record, layout: layout, zone: zone,
    Gate: Gate, Hover: Hover, railStep: railStep, rubber: rubber
  };
})());
