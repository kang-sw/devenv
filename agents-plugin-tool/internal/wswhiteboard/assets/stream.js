/* Interleaved topic history. Payloads stay inert in gaps until owner input;
   admitted whole rows survive swaps and are never evicted. */
(function () {
  if (window.wbHistory) {
    var previous = window.wbBindBoard;
    if (previous && !previous.wbHistoryBound) {
      window.wbBindBoard = function (root) { previous(root); window.wbHistory.init(root); };
      window.wbBindBoard.wbHistoryBound = true;
    }
    window.wbHistory.init(document);
    return;
  }
  function histories(root) {
    root = root || document;
    var out = Array.from(root.querySelectorAll('.wb-history'));
    if (root.matches && root.matches('.wb-history')) out.unshift(root);
    return out;
  }
  function valid(id) { return /^(?:(?:p|e)[1-9][0-9]*|item-[1-9][0-9]*)$/.test(id || ''); }
  function find(root, id) {
    if (!valid(id)) return null;
    var node = root.querySelector('[id="' + id + '"]');
    if (!node) return null;
    if (id[0] === 'e') return node.matches('.wb-entry[data-entry="' + id + '"]') ? node : null;
    if (id[0] === 'p') return node.matches('.entry-panel') && node.closest('.wb-entry[data-panel="' + id + '"]') ? node : null;
    return node.closest('.topic') || root.nodeType === 11 ? node : null;
  }
  var WINDOW_PX = 1000;
  function payload(gap) { return gap.querySelector('template[data-history-gap]').content; }
  function gaps(root) { return Array.from(root.querySelectorAll('.history-gap')); }
  function refresh(gap) {
    var list = Array.from(payload(gap).children);
    if (!list.length) { gap.remove(); return; }
    var count = list.reduce(function (sum, row) { return sum + (+row.getAttribute('data-count') || 1); }, 0);
    var from = list[0].getAttribute('data-from') || '', to = list[list.length - 1].getAttribute('data-to') || '';
    gap.setAttribute('data-count', count); gap.setAttribute('data-from', from); gap.setAttribute('data-to', to);
    gap.querySelector('.gap-label').textContent = window.wsT ? window.wsT('js.wb.timeline.gap', count) : '\u22ef ' + count;
    gap.querySelector('.gap-range').textContent = from === to ? from : from + '\u2013' + to;
    var before = Array.from(gap.parentElement.children).slice(0, Array.from(gap.parentElement.children).indexOf(gap));
    gap.querySelector('[data-load="down"]').hidden = !before.some(function (n) { return n.matches('.timeline-row'); });
  }
  function admitted(h) {
    if (panelState) h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore);
    if (window.wbAgo) window.wbAgo(h);
    if (window.wbBindBoard) window.wbBindBoard(h);
    h.dispatchEvent(new CustomEvent('wbhistoryload', { bubbles: true, detail: { root: h } }));
  }
  function take(gap, direction) {
    if (!gap) return null;
    var list = payload(gap).children, row = direction === 'up' ? list[list.length - 1] : list[0];
    if (!row) return null;
    gap.parentElement.insertBefore(row, direction === 'up' ? gap.nextElementSibling : gap);
    return row;
  }
  function readyHeight(row) {
    var frames = Array.from(row.querySelectorAll('.entry-panel[open] .doc-frame'));
    if (frames.some(function (f) {
      for (var p = f.parentElement; p && p !== row; p = p.parentElement) if (p.tagName === 'DETAILS' && !p.open) return false;
      return !parseFloat(f.style.height);
    })) return null;
    var height = row.getBoundingClientRect().height;
    return height > 0 ? height : null;
  }
  // One pending operation per gap; a late panel height resumes only the
  // unfinished window. Settled membership never changes on resize.
  function progress(h, op) {
    if (op.busy || op.done) return;
    op.busy = true;
    var kept = kit.keep(h), changed = false, touched = new Set();
    try {
      while (op.height < WINDOW_PX) {
        if (!op.row) {
          op.row = take(op.gap, op.direction);
          if (!op.row) {
            if (op.fallback) { op.gap = op.fallback; op.fallback = null; op.direction = 'up'; continue; }
            break;
          }
          touched.add(op.gap);
          changed = true;
        }
        var height = readyHeight(op.row);
        if (height === null) return;
        op.height += height;
        op.row = null;
      }
      op.done = true;
      h._wbHistoryPending = h._wbHistoryPending.filter(function (p) { return p !== op; });
    } finally {
      touched.forEach(refresh);
      if (changed) admitted(h);
      op.busy = false; kept.apply();
    }
  }
  function load(h, gap, direction, fallback) {
    if (h._wbHistoryPending.some(function (p) { return p.gap === gap || p.fallback === gap; })) return;
    var op = { gap: gap, direction: direction, fallback: fallback, height: 0, row: null };
    h._wbHistoryPending.push(op);
    progress(h, op);
  }
  // Split only the inert gap containing a known target. The prefix stays
  // inert; loading starts at the target, not at a distant page boundary.
  function split(gap, row) {
    // Clone the small control shell, never the potentially huge payload.
    var held = payload(gap), prefix = gap.cloneNode(false);
    Array.from(gap.children).forEach(function (n) { prefix.appendChild(n.cloneNode(!n.matches('template[data-history-gap]'))); });
    var before = payload(prefix);
    while (held.children[0] && held.children[0] !== row) before.appendChild(held.children[0]);
    if (before.children.length) { gap.parentElement.insertBefore(prefix, gap); refresh(prefix); }
    refresh(gap);
    return before.children.length ? prefix : null;
  }
  function materialize(root, id, beforeGeometry) {
    var node = find(root, id);
    if (node) { if (beforeGeometry) beforeGeometry(node); return node; }
    var deferred = Array.from(root.querySelectorAll('template[data-history-topic]')).find(function (t) {
      return !!find(t.content, id) || gaps(t.content).some(function (g) { return !!find(payload(g), id); });
    });
    if (deferred) {
      var fragment = deferred.content.cloneNode(true), topic = fragment.querySelector('.topic');
      deferred.parentElement.insertBefore(fragment, deferred);
      if (beforeGeometry) beforeGeometry(topic);
      deferred.remove(); init(topic); admitted(topic);
      return materialize(root, id, beforeGeometry);
    }
    var gap = gaps(root).find(function (g) { return !!find(payload(g), id); });
    if (!gap) return null;
    var h = gap.closest('.wb-history');
    if (beforeGeometry) beforeGeometry(h);
    init(h);
    var target = find(payload(gap), id), row = target.closest('.timeline-row');
    openTarget(target);
    if (!row) return null;
    var kept = kit.keep(h);
    var prefix = split(gap, row);
    load(h, gap, 'down', prefix);
    kept.apply();
    return find(h, id);
  }
  function openTarget(node) {
    for (var p = node; p; p = p.parentElement) if (p.tagName === 'DETAILS') p.open = true;
    if (node.matches('.wb-entry.panel')) {
      var panel = node.querySelector('.entry-panel');
      if (panel) panel.open = true;
    }
  }
  function loadTarget(root, id, beforeGeometry) {
    if (!valid(id)) return null;
    var node = materialize(root || document, id, beforeGeometry);
    if (!node) return null;
    openTarget(node);
    var panel = node.matches('.wb-entry.panel') ? node.querySelector('.entry-panel') : node.matches('.entry-panel') ? node : null;
    if (panel) { panel.open = true; if (panelState) panelState.opened(panel); }
    var mark = panel || node;
    mark.classList.toggle('flash', false); void mark.offsetWidth; mark.classList.toggle('flash', true);
    navigate(node);
    return node;
  }
  function navigate(node) {
    var topic = node.closest('.topic'), head = topic && topic.querySelector('.tophead'), offset = 12;
    var style = head && window.getComputedStyle(head);
    var sticky = style && style.position === 'sticky';
    if (sticky) offset += (parseFloat(style.top) || 0) + head.offsetHeight;
    var host = topic && (topic.closest('#bpins') || topic.closest('#bcol') || topic.closest('.wrap'));
    var wrap = host && host.querySelector('.oq-wrap.wb-oq-top'), pill = wrap && wrap.querySelector('.oq-pill');
    if (pill && !pill.hidden) {
      var pillTop = parseFloat(window.getComputedStyle(wrap).top) || 0;
      offset = Math.max(offset + (sticky ? 10 + pill.offsetHeight : 0), pillTop + pill.offsetHeight + 12);
    }
    kit.jump(node, { offset: offset });
    return node;
  }
  function capture(root) {
    return histories(root).map(function (h) {
      return { key: h.getAttribute('data-history'),
        panels: panelState ? panelState.capture(h) : [],
        rows: Array.from(h.querySelectorAll('.timeline-row')).map(function (n) { return n.getAttribute('data-row'); }),
        entries: Array.from(h.querySelectorAll('.wb-entry')).map(function (n) { return n.id; }),
        systems: Array.from(h.querySelectorAll('.system-run .wb-entry')).map(function (n) { return [n.id, n.closest('.system-run').open]; }),
        pending: h._wbHistoryPending.map(function (p) {
          var list = p.gap && payload(p.gap).children, next = list && (p.direction === 'up' ? list[list.length - 1] : list[0]);
          var before = p.fallback && payload(p.fallback).children, edge = before && before[before.length - 1];
          return { row: p.row && p.row.getAttribute('data-row'), next: next && next.getAttribute('data-row'), before: edge && edge.getAttribute('data-row'), direction: p.direction, height: p.height };
        }),
        folds: Array.from(h.querySelectorAll('details[data-key]')).map(function (d) { return [d.getAttribute('data-key'), d.open]; }) };
    });
  }
  // Restore membership before a host's DOM morph, without binding, mounting
  // frames or reading layout. A former anchor may now live in an inert gap.
  function prepare(root, snapshot) {
    (snapshot || []).forEach(function (saved) {
      var h = histories(root).find(function (n) { return n.getAttribute('data-history') === saved.key; });
      if (h) restoreRows(h, saved);
    });
  }
  function restoreRows(h, saved) {
    var wanted = new Set(saved.rows || []), entries = new Set(saved.entries || []), changed = false;
    gaps(h).forEach(function (gap) {
      Array.from(payload(gap).children).forEach(function (row) {
        if (!wanted.has(row.getAttribute('data-row')) && !Array.from(row.querySelectorAll('.wb-entry')).some(function (n) { return entries.has(n.id); })) return;
        split(gap, row);
        take(gap, 'down'); changed = true;
        refresh(gap);
      });
    });
    return changed;
  }
  function restore(root, snapshot) {
    // Materialize exact previously admitted rows without granting another
    // 1000px window on each update. No append can evict a viewed row.
    init(root);
    (snapshot || []).forEach(function (saved) {
      var h = histories(root).find(function (n) { return n.getAttribute('data-history') === saved.key; });
      if (!h) {
        var deferred = Array.from((root || document).querySelectorAll('template[data-history-topic]')).find(function (t) { return t.getAttribute('data-history-topic') === saved.key; });
        if (deferred) {
          var fragment = deferred.content.cloneNode(true), topic = fragment.querySelector('.topic');
          deferred.parentElement.insertBefore(fragment, deferred); deferred.remove(); init(topic); admitted(topic);
          h = histories(topic)[0];
        }
      }
      if (!h) return;
      if (panelState) panelState.snapshot(h, saved);
      var kept = kit.keep(h), changed = restoreRows(h, saved);
      var folds = new Map(saved.folds || []);
      h.querySelectorAll('details[data-key]').forEach(function (d) {
        if (folds.has(d.getAttribute('data-key'))) d.open = folds.get(d.getAttribute('data-key'));
      });
      // Latest pN identity wins over obsolete latest-version entry keys.
      if (panelState) h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore);
      var systems = new Map(saved.systems || []);
      h.querySelectorAll('.system-run').forEach(function (fold) {
        var entry = Array.from(fold.querySelectorAll('.wb-entry')).find(function (n) { return systems.has(n.id); });
        if (entry) fold.open = systems.get(entry.id);
      });
      if (changed) admitted(h);
      (saved.pending || []).forEach(function (p) {
        if (!valid(p.row)) return;
        var row = h.querySelector('[data-row="' + p.row + '"]');
        if (!row) return;
        var gap = gaps(h).find(function (g) { return Array.from(payload(g).children).some(function (n) { return n.getAttribute('data-row') === p.next; }); });
        var fallback = gaps(h).find(function (g) { return Array.from(payload(g).children).some(function (n) { return n.getAttribute('data-row') === p.before; }); });
        var op = { row: row, gap: gap, fallback: fallback, direction: p.direction, height: p.height };
        h._wbHistoryPending.push(op); progress(h, op);
      });
      kept.apply();
    });
  }
  // The initial view always holds at least the bottom WINDOW_PX of history,
  // cut like the expand button: whole rows from the bottom up, including the
  // row that crosses the line. Server folds mark where a gap may start; only
  // this client-side measure decides how much below it stays visible. A host
  // that is not laid out yet (zero heights) retries from the resize observer.
  function fill(h) {
    var live = h.querySelector('.history-live');
    if (!live || h._wbHistoryPending.length) return;
    var acc = 0, kids = Array.from(live.children);
    for (var i = kids.length - 1; i >= 0; i--) {
      var n = kids[i];
      if (n.matches('.history-gap')) {
        if (acc >= WINDOW_PX) break;
        var op = { gap: n, direction: 'up', fallback: null, height: acc, row: null };
        h._wbHistoryPending.push(op);
        progress(h, op);
        if (op.done && op.height < WINDOW_PX) fill(h); // gap exhausted; continue above it
        return;
      }
      if (n.matches('.timeline-row')) {
        var height = readyHeight(n);
        if (height === null) return;
        acc += height;
        if (acc >= WINDOW_PX) break;
      }
    }
    h._wbHistoryFilling = false;
  }
  function init(root) {
    if (window.wbAgo) window.wbAgo(root || document);
    var fresh = [];
    histories(root).forEach(function (h) {
      if (h._wbHistoryBound) {
        if (panelState) h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore);
        return;
      }
      h._wbHistoryBound = true;
      h._wbHistoryPending = [];
      fresh.push(h);
    });
    // Anchor/gap membership is selected on the server. Batch disclosure
    // writes before any result reads and share one capture per scroller,
    // retaining the landed initialization cost shape without a client cut.
    var kept = new Map();
    fresh.forEach(function (h) {
      var scroller = kit.keep(h, { capture: false }).scroller;
      if (!kept.has(scroller)) kept.set(scroller, kit.keep(h));
    });
    if (panelState) fresh.forEach(function (h) { h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore); });
    fresh.forEach(function (h) { h._wbHistoryHeight = Math.ceil(h.querySelector('.history-live').getBoundingClientRect().height); });
    kept.forEach(function (k) { k.apply(); });
    fresh.forEach(function (h) { h._wbHistoryFilling = true; fill(h); });
    fresh.forEach(function (h) {
      h.addEventListener('focusin', function (e) {
        var node = e.target, live = h.querySelector('.history-live');
        if (!live.contains(node)) return;
        var r = node.getBoundingClientRect();
        var hit = document.elementFromPoint && document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        if (hit && hit !== node && !node.contains(hit)) navigate(node);
      });
      h.addEventListener('click', function (e) {
        var button = e.target.closest && e.target.closest('[data-load]');
        if (!button || button.hidden || button.closest('.wb-history') !== h) return;
        load(h, button.closest('.history-gap'), button.getAttribute('data-load'));
      });
      if (typeof ResizeObserver !== 'undefined') {
        var observer = new ResizeObserver(function () {
          if (h.isConnected === false) { observer.disconnect(); return; }
          if ((!h._wbHistoryPending.length && !h._wbHistoryFilling) || h._wbHistoryFrame) return;
          h._wbHistoryFrame = true;
          window.requestAnimationFrame(function () {
            h._wbHistoryFrame = false;
            if (!h.isConnected) return;
            h._wbHistoryPending.slice().forEach(function (op) { progress(h, op); });
            if (h._wbHistoryFilling) fill(h);
          });
        });
        observer.observe(h.querySelector('.history-live'));
        h._wbHistoryObserver = observer;
      }
    });
  }
  // viewKit holds the visibility source and the keep-in-place helper every
  // board host shares (the dashboard's board, the standalone page's
  // history). win gives the globals it uses, so the kit runs under a fake
  // window too.
  //
  // watch(node, {onChange, onOut, margin}) subscribes to node's viewport
  // visibility: one IntersectionObserver per rootMargin, all with root null,
  // so whichever ancestor scrolls (a column, the pinned column, the
  // document) clips it. A margin reaches past an inner scroller's clip
  // only as a scroll margin (a root margin widens the viewport alone), so
  // each observer takes it as both where the browser has scroll margins;
  // bands tells whether it has, that is whether a margin means the same
  // distance from the view whichever ancestor scrolls. Each call is its own
  // subscription. onChange gets every report {node, ratio, visible, rect};
  // onOut runs once per change from ratio > 0 to 0 (a first report of 0
  // has not left view). seen(node) reads the node's last report without a
  // layout. sweep() drops the subscriptions of nodes no longer on the page.
  //
  // keep(node) captures the reading-line panel on node's actual scroller.
  // Nested synchronous mutations share one transaction; the last apply()
  // resolves that identity and writes at most once. There is no ordinary
  // resize session or substituted reference. capture:false only inspects
  // exposed geometry. Owners retain native-anchor suppression and input
  // listeners; input/momentum suppress captures until 200 ms after scrolling.
  // jump(node, {block, behavior}) alone follows asynchronous layout, until
  // 150 ms / five stable frames or its non-renewing 2 s cap. It has priority
  // over ordinary transactions, and input cancels it immediately.
  function viewKit(win) {
    var doc = win.document, observers = {}, reports = new WeakMap(), sessions = new Map(), owners = new Map();
    var IO = win.IntersectionObserver, bands = !!(IO && IO.prototype && "scrollMargin" in IO.prototype);
    function observer(margin) {
      if (observers[margin]) return observers[margin];
      var o = observers[margin] = { subs: new Map(), io: null };
      o.io = new win.IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          var list = o.subs.get(e.target);
          if (!list) return;
          var r = { node: e.target, ratio: e.intersectionRatio, visible: e.intersectionRatio > 0, rect: e.boundingClientRect };
          var s = reports.get(e.target) || { by: {} };
          s.by[margin] = r;
          s.last = r;
          reports.set(e.target, s);
          list.slice().forEach(function (sub) {
            if (sub.gone) return;
            var was = sub.ratio;
            sub.ratio = r.ratio;
            if (sub.onChange) sub.onChange(r);
            if (sub.onOut && was > 0 && r.ratio === 0) sub.onOut(r);
          });
        });
      }, bands ? { root: null, rootMargin: margin, scrollMargin: margin, threshold: [0] } : { root: null, rootMargin: margin, threshold: [0] });
      return o;
    }
    function drop(sub) {
      if (sub.gone) return;
      sub.gone = true;
      var o = observers[sub.margin], list = o && o.subs.get(sub.node);
      if (!list) return;
      list.splice(list.indexOf(sub), 1);
      if (!list.length) { o.subs.delete(sub.node); o.io.unobserve(sub.node); }
    }
    function watch(node, opt) {
      opt = opt || {};
      var sub = { node: node, margin: opt.margin || "0px", onChange: opt.onChange, onOut: opt.onOut, ratio: undefined, gone: false };
      var handle = { node: node, unwatch: function () { drop(sub); } };
      if (!node || !win.IntersectionObserver) { sub.gone = true; return handle; }
      owner(scrollerOf(node));
      var o = observer(sub.margin), list = o.subs.get(node);
      // A node already observed reports only on a change: observing it
      // again gives the new subscription its first report.
      if (list) { list.push(sub); o.io.unobserve(node); } else o.subs.set(node, [sub]);
      o.io.observe(node);
      return handle;
    }
    function unwatch(handle) { if (handle && handle.unwatch) handle.unwatch(); }
    function sweep() {
      Object.keys(observers).forEach(function (m) {
        var gone = [];
        observers[m].subs.forEach(function (list, node) { if (!node.isConnected) gone = gone.concat(list); });
        gone.forEach(drop);
      });
    }
    function seen(node, margin) {
      var s = reports.get(node);
      return s ? (margin ? s.by[margin] || null : s.last) : null;
    }
    // A node's nearest scroller: the first ancestor whose overflow scrolls
    // and whose content overflows it, else the document's.
    function docScroller() { return doc.scrollingElement || doc.documentElement; }
    function scrollerOf(node) {
      for (var p = node && node.parentElement; p && p !== doc.body && p !== doc.documentElement; p = p.parentElement) {
        var oy = win.getComputedStyle(p).overflowY;
        if ((oy === "auto" || oy === "scroll" || oy === "overlay") && p.scrollHeight > p.clientHeight) return p;
      }
      return docScroller();
    }
    function shiftY(el) {
      var t = win.getComputedStyle(el).transform, m = t && /^matrix(3d)?\((.*)\)$/.exec(t);
      if (!m) return 0;
      var v = m[2].split(",").map(parseFloat);
      return (m[1] ? v[13] : v[5]) || 0;
    }
    // layoutTop is node's top in the viewport with the transforms between
    // it and its scroller taken out; scrollerTop is the scroller's content
    // top in the viewport.
    function layoutTop(node, scroller) {
      var y = node.getBoundingClientRect().top;
      for (var p = node; p && p !== scroller; p = p.parentElement) y -= shiftY(p);
      return y;
    }
    function scrollerTop(scroller) { return scroller === docScroller() ? 0 : scroller.getBoundingClientRect().top + (scroller.clientTop || 0); }
    function viewport(scroller) {
      var main = scroller === docScroller(), r = main ? { top: 0, bottom: win.innerHeight || scroller.clientHeight } : scroller.getBoundingClientRect();
      var top = Math.max(0, r.top + (main ? 0 : scroller.clientTop || 0));
      var bottom = Math.min(win.innerHeight || scroller.clientHeight, main ? r.bottom : r.top + (scroller.clientTop || 0) + scroller.clientHeight);
      var style = win.getComputedStyle(scroller);
      top += parseFloat(style.scrollPaddingTop) || 0;
      // The rail publishes its clearance on sticky heads, not on the host.
      Array.from((main ? doc : scroller).querySelectorAll('.tophead, .bc-hd, .wb-rail-wrap.up .wb-rail-stack')).forEach(function (head) {
        var hs = win.getComputedStyle(head), h = head.getBoundingClientRect();
        if ((hs.position === 'sticky' || head.matches('.wb-rail-stack')) && h.height && h.top <= top + (parseFloat(hs.top) || 0) + 1 && h.bottom > top)
          top = Math.max(top, h.bottom);
      });
      return { top: top, bottom: bottom, left: main ? 0 : Math.max(0, r.left + (scroller.clientLeft || 0)),
        right: Math.min(win.innerWidth || r.right || Infinity, main ? win.innerWidth || Infinity : r.left + (scroller.clientLeft || 0) + scroller.clientWidth) };
    }
    function exposed(node, scroller, view) {
      var v = view || viewport(scroller), r = node.getBoundingClientRect(), top = Math.max(v.top, r.top), bottom = Math.min(v.bottom, r.bottom);
      var left = Math.max(v.left, r.left), right = Math.min(v.right, r.right);
      if (!(r.height > 0 && bottom - top >= 1 && right - left >= 1)) return false;
      for (var p = node.parentElement; p && p !== scroller && p !== doc.body; p = p.parentElement) {
        if (p.tagName === 'DETAILS' && !p.open) return false;
        if (/hidden|clip|auto|scroll/.test(win.getComputedStyle(p).overflowY)) {
          var box = p.getBoundingClientRect(); top = Math.max(top, box.top); bottom = Math.min(bottom, box.bottom);
          left = Math.max(left, box.left); right = Math.min(right, box.right);
        }
      }
      return r.height > 0 && bottom - top >= 1 && right - left >= 1;
    }
    function reading(scroller) {
      // One synchronous capture has one viewport. Re-scanning every sticky
      // head for every candidate makes history initialization/resize cubic
      // in board size; reject offscreen boxes before walking their ancestry.
      var root = scroller === docScroller() ? doc : scroller, view = viewport(scroller), line = view.top + 1;
      var nodes = Array.from(root.querySelectorAll('.entry-panel, .wb-entry:not(.panel), .doc-frame, [data-card]')).filter(function (n) {
        if (!exposed(n, scroller, view) || scrollerOf(n) !== scroller) return false;
        // An opaque frame is represented by its host panel, never its DOM.
        return !(n.matches('.doc-frame') && n.closest('.entry-panel'));
      });
      nodes.sort(function (a, b) { return a.getBoundingClientRect().top - b.getBoundingClientRect().top; });
      return nodes.find(function (n) { var r = n.getBoundingClientRect(); return r.top <= line && r.bottom > line; }) ||
        nodes.find(function (n) { return n.getBoundingClientRect().top > line; }) || null;
    }
    function owner(scroller) {
      var o = owners.get(scroller);
      if (o) return o;
      var main = scroller === docScroller(), target = main ? win : scroller;
      o = { state: 'idle', until: 0, wrote: scroller.scrollTop, transaction: null };
      owners.set(scroller, o);
      scroller.wbScrollOwner = true;
      (main ? doc.documentElement : scroller).style.overflowAnchor = 'none';
      function clock() { return win.performance ? win.performance.now() : Date.now(); }
      function intent() {
        o.state = 'user-scrolling'; o.until = clock() + 200;
        if (o.transaction) o.transaction.cancel();
        var s = sessions.get(scroller);
        if (s && s.follow) { scroller.scrollTo({ top: scroller.scrollTop, left: scroller.scrollLeft, behavior: 'instant' }); s.cancel(); }
      }
      ['wheel', 'touchstart', 'touchmove', 'pointerdown'].forEach(function (type) { target.addEventListener(type, intent, { passive: true, capture: true }); });
      win.addEventListener('keydown', function (e) {
        if (!main && e.target && !scroller.contains(e.target) && doc.activeElement && !scroller.contains(doc.activeElement)) return;
        if (/^(ArrowUp|ArrowDown|ArrowLeft|ArrowRight|PageUp|PageDown|Home|End| |Spacebar)$/.test(e.key)) intent();
      }, true);
      target.addEventListener('scroll', function () {
        if (Math.abs(scroller.scrollTop - o.wrote) <= 1) return;
        var s = sessions.get(scroller);
        if (s && s.follow) return; // Native initial smooth navigation is not input.
        if (o.state === 'user-scrolling') o.until = clock() + 200;
        else intent();
        o.wrote = scroller.scrollTop;
      }, { passive: true });
      o.ready = function () {
        if (o.state === 'user-scrolling' && clock() >= o.until) o.state = 'idle';
        return o.state === 'idle';
      };
      return o;
    }
    function keep(node, opt) {
      opt = opt || {};
      var scroller = scrollerOf(node), o = owner(scroller), s = sessions.get(scroller);
      var inspect = { scroller: scroller, visible: function (n) { return exposed(n, scroller); }, viewport: function () { return viewport(scroller); }, apply: function () {}, end: function () {} };
      if (opt.capture === false) return inspect;
      if (s && s.follow) return s.handle;
      if (!o.ready()) return inspect;
      var t = o.transaction;
      if (!t) {
        var ref = typeof opt.offset === 'number' ? node : reading(scroller);
        var id = ref && ref.id, topic = ref && ref.closest('.topic'), name = topic && topic.getAttribute('data-topic');
        var key = ref && ref.getAttribute('data-doc-key');
        var offset = ref && (typeof opt.offset === 'number' ? opt.offset + scrollerTop(scroller) : layoutTop(ref, scroller));
        t = o.transaction = { depth: 0, done: false };
        t.cancel = function () { t.done = true; o.transaction = null; sessions.delete(scroller); };
        t.apply = function () {
          if (t.done || --t.depth > 0) return;
          t.cancel();
          if (!o.ready() || !ref) return;
          if (!ref.isConnected) {
            ref = id ? doc.getElementById(id) : null;
            if (!ref && key) ref = Array.from(doc.querySelectorAll('.doc-frame[data-doc-key]')).find(function (n) {
              var p = n.closest('.topic'); return n.getAttribute('data-doc-key') === key && p && p.getAttribute('data-topic') === name;
            });
          }
          if (!ref || !ref.isConnected || scrollerOf(ref) !== scroller || (name && (!ref.closest('.topic') || ref.closest('.topic').getAttribute('data-topic') !== name))) return;
          var delta = layoutTop(ref, scroller) - offset;
          if (Math.abs(delta) >= 0.5) scroller.scrollTop += delta;
          o.wrote = scroller.scrollTop;
          if (opt.onEnd) opt.onEnd('settled');
        };
        sessions.set(scroller, { handle: inspect });
      }
      t.depth++;
      var used = false;
      inspect.apply = function () { if (!used) { used = true; t.apply(); } };
      inspect.end = function () { if (!used) { used = true; if (!t.done && --t.depth === 0) t.cancel(); } };
      return inspect;
    }
    // Explicit Overview placement uses the navigation owner, including its
    // late-layout follow and input cancellation, never an ordinary capture.
    // readingOffset follows the sticky reading line as a reloaded board mounts;
    // ordinary offset stays relative to the scroller. Both use the same jump.
    function place(node, opt) {
      return jump(node, { offset: opt && typeof opt.offset === 'number' ? opt.offset : 0,
        readingOffset: opt && opt.readingOffset, behavior: 'instant' });
    }
    function jump(node, opt) {
      var scroller = scrollerOf(node), s = sessions.get(scroller);
      if (!s || !s.follow) s = start(scroller);
      if (opt && opt.onEnd) s.ends.push(opt.onEnd);
      s.jump(node, opt || {});
      return s.handle;
    }
    function start(scroller) {
      var o = owner(scroller);
      if (o.transaction) o.transaction.cancel();
      o.state = 'explicit-jump-follow';
      var s = { ends: [], ro: null, done: false };
      var main = scroller === docScroller();
      function apply() {
        if (s.done) return;
        if (s.follow) s.follow.align();
        o.wrote = scroller.scrollTop;
      }
      function end(reason) {
        if (s.done) return;
        s.done = true;
        if (s.follow) s.follow.off();
        sessions.delete(scroller);
        if (s.ro) s.ro.disconnect();
        if (o.state === 'explicit-jump-follow') o.state = 'idle';
        o.wrote = scroller.scrollTop;
        s.ends.forEach(function (fn) { fn(reason); });
      }
      s.cancel = function () { end('user'); };
      s.jump = function (node, opt) {
        if (s.follow) s.follow.off();
        var block = opt.block || "center", raf = 0, cap = 0, stable = 0, quiet = 0, last, position;
        function off() {
          win.cancelAnimationFrame(raf);
          win.clearTimeout(cap);
        }
        function measure() {
          var r = node.getBoundingClientRect();
          var offset = typeof opt.readingOffset === 'number' ? viewport(scroller).top + 1 - scrollerTop(scroller) + opt.readingOffset :
            typeof opt.offset === 'number' ? opt.offset : block === "center" ? (scroller.clientHeight - r.height) / 2 : 0;
          return r.top - scrollerTop(scroller) + scroller.scrollTop - offset;
        }
        function align() {
          // A board morph may replace the item but retain its stable ID.
          if (!node.isConnected && node.id) node = doc.getElementById(node.id) || node;
          if (!node.isConnected || scrollerOf(node) !== scroller) { end("ended"); return; }
          var next = measure();
          // Smooth scrolling changes the viewport rect, not this content-space
          // destination. Only layout movement interrupts it with an instant fix.
          if (Math.abs(next - position) >= 0.5) {
            land('instant');
            stable = 0; quiet = 0;
            position = next;
          }
        }
        function frame(t) {
          raf = 0;
          align();
          if (s.done) return;
          var current = scroller.scrollTop;
          if (last !== undefined && Math.abs(current - last) < 0.5) {
            if (!stable++) quiet = t;
            if (stable >= 5 && t - quiet >= 150) { end("settled"); return; }
          } else { stable = 0; quiet = t; }
          last = current;
          raf = win.requestAnimationFrame(frame);
        }
        function land(behavior) {
          if (typeof opt.offset === 'number' || typeof opt.readingOffset === 'number') scroller.scrollTo({ top: measure(), behavior: behavior });
          else node.scrollIntoView({ block: block, behavior: behavior });
          o.wrote = scroller.scrollTop;
        }
        s.follow = { align: align, off: off };
        position = measure();
        var reduced = win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches;
        land(reduced ? "instant" : opt.behavior || "instant");
        cap = win.setTimeout(function () { end("cap"); }, 2000);
        raf = win.requestAnimationFrame(frame);
      };
      // A swap settling on another anchor must not end explicit navigation.
      s.handle = { scroller: scroller, apply: apply, end: function () { if (!s.follow) end("ended"); } };
      if (win.ResizeObserver) {
        s.ro = new win.ResizeObserver(function () { if (!s.done) apply(); });
        (main ? [doc.body] : Array.prototype.slice.call(scroller.children)).forEach(function (el) { if (el) s.ro.observe(el); });
      }
      sessions.set(scroller, s);
      return s;
    }
    function keeping(scroller) { var s = sessions.get(scroller); return s ? s.handle : null; }
    // snapshot() is a read-only exposure view for one synchronous pass with
    // no DOM write inside it: each scroller's viewport and each ancestor's
    // scroller are measured once. Per-node keep(capture:false).visible
    // rescans every sticky head, which over a whole board's panels froze
    // every board update for ~1 s (100 topics).
    function snapshot() {
      var views = new Map(), under = new Map();
      function scroller(node) {
        var chain = [], found = null;
        for (var p = node && node.parentElement; p && p !== doc.body && p !== doc.documentElement; p = p.parentElement) {
          if (under.has(p)) { found = under.get(p); break; }
          chain.push(p);
          var oy = win.getComputedStyle(p).overflowY;
          if ((oy === "auto" || oy === "scroll" || oy === "overlay") && p.scrollHeight > p.clientHeight) { found = p; break; }
        }
        if (!found) found = docScroller();
        chain.forEach(function (c) { under.set(c, found); });
        return found;
      }
      function visible(node) {
        if (!node || node.isConnected === false) return false;
        var s = scroller(node), v = views.get(s);
        if (!v) { v = viewport(s); views.set(s, v); }
        return exposed(node, s, v);
      }
      return { scroller: scroller, visible: visible };
    }
    return { watch: watch, unwatch: unwatch, sweep: sweep, seen: seen, keep: keep, place: place, keeping: keeping, jump: jump, bands: bands, snapshot: snapshot };
  }
  // Current panel folds follow pN; historical-version folds still use their
  // independent entry keys. The additive browser record is not board data.
  function panelStateKit(win) {
    var doc = win.document || document, html = doc.documentElement;
    var scope = html && html.getAttribute && (html.getAttribute('data-rid') || html.getAttribute('data-wb-reply'));
    if (!scope) return null;
    var KEY = 'ws.whiteboard.panels', CAP = 200, records = new Map(), loaded = new Set(), storage = null;
    try {
      storage = win.localStorage;
      var saved = JSON.parse(storage.getItem(KEY) || '[]');
      if (Array.isArray(saved)) saved.forEach(function (r) {
        if (r && typeof r.key === 'string' && /\/p[1-9][0-9]*$/.test(r.key) && typeof r.folded === 'boolean' && /^e[1-9][0-9]*$/.test(r.read || '') && /^e[1-9][0-9]*$/.test(r.seen || '')) {
          records.set(r.key, r); loaded.add(r.key);
        }
      });
    } catch (_) {}
    function put(r) {
      records.delete(r.key); records.set(r.key, r);
      // Open-panel read marks must not evict the existing 200-fold budget.
      var same = Array.from(records.values()).filter(function (v) { return v.folded === r.folded; });
      same.slice(0, Math.max(0, same.length - CAP)).forEach(function (v) { records.delete(v.key); });
      try { if (storage) storage.setItem(KEY, JSON.stringify(Array.from(records.values()))); } catch (_) {}
    }
    function info(panel) {
      if (!panel || !panel.classList.contains('entry-panel')) return null;
      var article = panel.closest('.wb-entry[data-panel]');
      if (!article || article.classList.contains('historical')) return null;
      var id = article.getAttribute('data-panel'), entry = article.getAttribute('data-entry');
      if (!/^p[1-9][0-9]*$/.test(id || '') || !/^e[1-9][0-9]*$/.test(entry || '')) return null;
      return { panel: panel, article: article, id: id, entry: entry, key: scope + '/' + id };
    }
    function legacy(i) {
      var shut;
      try { shut = JSON.parse(storage && storage.getItem('ws.dashboard.panelshut') || '[]'); } catch (_) { return null; }
      if (!Array.isArray(shut)) return null;
      var versions = [i.article].concat(Array.from(i.article.querySelectorAll('.wb-entry[data-entry]')));
      var old = versions.find(function (a) {
        return a.getAttribute('data-panel') === i.id && shut.indexOf(scope + '/entry:' + a.getAttribute('data-entry')) >= 0;
      });
      return old && old.getAttribute('data-entry');
    }
    function record(i) {
      var r = records.get(i.key);
      if (!r) {
        var old = legacy(i);
        r = { key: i.key, folded: !!old || !i.panel.open, read: old || i.entry, seen: i.entry };
        put(r);
      }
      return r;
    }
    function badge(i, r) {
      var pending = r.folded && r.read !== i.entry, b = i.panel.querySelector('.wb-panel-updated');
      if (b) b.hidden = !pending;
      i.article.classList.toggle('wb-panel-pending', pending);
    }
    function restorePanel(panel) {
      var i = info(panel);
      if (!i) return false;
      var intent = panel._wbPanelIntent;
      if (panel.open && intent && intent.open && intent.entry === i.entry) toggle(panel);
      var r = record(i), changed = r.seen !== i.entry;
      r.seen = i.entry;
      if (!r.folded && r.read !== i.entry) { r.read = i.entry; changed = true; }
      panel.open = !r.folded;
      badge(i, r);
      if (changed) put(r);
      return true;
    }
    function toggle(panel) {
      var i = info(panel);
      if (!i) return false;
      var r = record(i), intent = panel._wbPanelIntent;
      // Native toggle is also emitted for programmatic attribute changes.
      // Only explicit opening intent may acknowledge a pending new version.
      if (panel.open && (!intent || !intent.open || intent.entry !== i.entry)) { badge(i, r); return false; }
      r.folded = !panel.open; r.seen = i.entry;
      if (panel.open) r.read = i.entry;
      delete panel._wbPanelIntent;
      put(r); badge(i, r);
      return true;
    }
    function opened(panel) {
      var i = info(panel);
      if (!i) return;
      panel._wbPanelIntent = { entry: i.entry, open: true };
      panel.open = true;
      toggle(panel);
    }
    function panels(root) { return Array.from(root.querySelectorAll('.entry-panel[data-key]')).filter(function (p) { return !!info(p); }); }
    function capturePanels(root) {
      return panels(root).map(function (p) {
        var i = info(p), r = record(i);
        if (!p.open && !r.folded) { r.folded = true; put(r); }
        return { key: r.key, folded: r.folded, read: r.read, seen: i.entry };
      });
    }
    function restoreSnapshot(root, saved) {
      if (Array.isArray(saved.panels)) {
        saved.panels.forEach(function (r) {
          if (r && typeof r.key === 'string' && r.key.indexOf(scope + '/') === 0 && !loaded.has(r.key) && typeof r.folded === 'boolean' && /^e[1-9][0-9]*$/.test(r.read || '') && /^e[1-9][0-9]*$/.test(r.seen || '')) put(Object.assign({}, r));
        });
      } else {
        // Old standalone snapshots knew entry keys only. Resolve those
        // against trusted renderer metadata, including earlier versions.
        (saved.folds || []).forEach(function (f) {
          var m = /^entry:(e[1-9][0-9]*)$/.exec(f[0]);
          if (!m || f[1] !== false) return;
          var a = root.querySelector('.wb-entry[data-entry="' + m[1] + '"]'), id = a && a.getAttribute('data-panel');
          panels(root).forEach(function (p) {
            var i = info(p);
            if (i.id === id && !loaded.has(i.key)) {
              put({ key: i.key, folded: true, read: m[1], seen: i.entry }); loaded.add(i.key);
            }
          });
        });
      }
    }
    doc.addEventListener('click', function (e) {
      var summary = e.target.closest && e.target.closest('summary'), panel = summary && summary.parentElement;
      var i = info(panel);
      if (!i || (e.target.closest && e.target.closest('a,button,input,textarea'))) return;
      panel._wbPanelIntent = { entry: i.entry, open: !panel.open };
      // Record a fold before the next observer/host restore can see the
      // native default action. An open still waits for actual open DOM.
      if (panel.open) { var r = record(i); r.folded = true; put(r); }
    }, true);
    // The dashboard already owns the toggle listener and calls this kit.
    // Standalone pages have no host listener, so bind the same policy here.
    if (!html.getAttribute('data-rid')) doc.addEventListener('toggle', function (e) { toggle(e.target); }, true);
    return { restore: restorePanel, toggle: toggle, opened: opened, capture: capturePanels, snapshot: restoreSnapshot };
  }
  var kit = viewKit(window);
  var panelState = panelStateKit(window);
  window.wbHistory = { init: init, capture: capture, prepare: prepare, restore: restore, loadTarget: loadTarget,
    restorePanel: function (p) { return !!panelState && panelState.restore(p); },
    panelToggle: function (p) { return !!panelState && panelState.toggle(p); },
    watch: kit.watch, unwatch: kit.unwatch, sweep: kit.sweep, seen: kit.seen, keep: kit.keep, place: kit.place, keeping: kit.keeping, jump: kit.jump, bands: kit.bands, snapshot: kit.snapshot };
  // An entry's clock shows its relative time on hover, computed when shown
  // so it never goes stale (ago.js's wording), then the local date and time.
  document.addEventListener('mouseover', function (e) {
    var t = e.target && e.target.closest && e.target.closest('time.entry-at[data-at]');
    if (t && window.wbAgoText) t.title = window.wbAgoText(+t.getAttribute('data-at')) + ' · ' + (t.getAttribute('data-stamp') || '');
  });
  var bind = window.wbBindBoard;
  if (bind) {
    window.wbBindBoard = function (root) { bind(root); init(root); };
    window.wbBindBoard.wbHistoryBound = true;
  }
  init(document);

  // The standalone file page navigates its own fragments. The dashboard has
  // its own allowlisted bridge, scroll policy, and region-swap lifecycle.
  if (document.body && document.body.classList.contains('full')) {
    function jump(id) {
      var node = loadTarget(document, id);
      if (node && !node.closest('.wb-history')) kit.jump(node, { block: 'nearest' });
      return node;
    }
    document.addEventListener('click', function (e) {
      var a = e.target.closest && e.target.closest('a[href]');
      var href = a && a.getAttribute('href');
      if (!href || href[0] !== '#' || !jump(href.slice(1))) return;
      e.preventDefault();
      try { window.history.replaceState(null, '', href); } catch (_) {}
    });
    window.addEventListener('hashchange', function () { jump(location.hash.slice(1)); });
    var storageKey = 'wb-history:' + location.pathname, saved;
    try { saved = JSON.parse(sessionStorage.getItem(storageKey) || 'null'); sessionStorage.removeItem(storageKey); } catch (_) {}
    if (saved) restore(document, saved);
    if (location.hash) jump(location.hash.slice(1));
    window.addEventListener('beforeunload', function () {
      try { sessionStorage.setItem(storageKey, JSON.stringify(capture(document))); } catch (_) {}
    });
  }
})();

/* Recap jump: a topic recap's clock chip (.rat, a keyboard button) names the
   log row the recap summarizes (the recap's data-entry, its latest agent
   line). Activating the chip, or a plain click on the recap text, reveals
   that row through the shared history loader (opening its disclosures,
   clearing the sticky head), whose flash is the one highlight. A text click
   waits out a double-click so selecting a word never jumps; a drag, a
   selection, or a modified click is left alone, and controls inside the
   recap keep their own behavior. Document-level, so board swaps need no
   rebinding. */
(function () {
  if (window.wbRecapJump) return;
  // DOUBLE stands in for the platform double-click interval, which a page
  // cannot read; FLASH outlasts page.css .flash (1.6s).
  var CHIP = '.trecap[data-entry] > .rat[role="button"]', DOUBLE = 500, FLASH = 2000, press = null, pending = 0;
  function recapOf(n) {
    var recap = n && n.closest ? n.closest('.trecap[data-entry]') : null;
    return recap && recap.querySelector(':scope > .rat[role="button"]') ? recap : null;
  }
  function cancel() { window.clearTimeout(pending); pending = 0; }
  function selecting(recap) {
    var sel = window.getSelection && window.getSelection();
    return !!(sel && !sel.isCollapsed && sel.rangeCount && sel.getRangeAt(0).intersectsNode(recap));
  }
  function jump(recap) {
    cancel();
    var topic = recap.closest('.topic'), history = window.wbHistory;
    if (!topic || !history || !history.loadTarget) return null;
    // A dashboard card fold hides the whole topic; reveal it before geometry.
    var unfold = window.WsCards && window.WsCards.unfoldTopic;
    var row = history.loadTarget(topic, recap.getAttribute('data-entry'), unfold);
    if (row) {
      // The arrival blink's more specific animation would mask the flash and
      // replay it when the blink ends, so neither may outlive this one.
      row.classList.remove('wb-log-blink');
      window.clearTimeout(row._wbRecapFlash);
      row._wbRecapFlash = window.setTimeout(function () { row.classList.remove('flash'); }, FLASH);
    }
    return row;
  }
  // Every press cancels a pending text jump: it may be a double-click's
  // second press, and its own click decides again.
  document.addEventListener('pointerdown', function (e) {
    cancel();
    press = recapOf(e.target) ? { x: e.clientX, y: e.clientY } : null;
  }, true);
  document.addEventListener('click', function (e) {
    var recap = recapOf(e.target), at = press;
    press = null;
    if (!recap || e.defaultPrevented || e.button || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (e.detail > 1) { cancel(); return; }
    var inner = e.target.closest('a[href], button, input, textarea, select, summary, label, [contenteditable]');
    if (inner && recap.contains(inner)) return;
    if (at && Math.abs(e.clientX - at.x) + Math.abs(e.clientY - at.y) > 4) return;
    if (selecting(recap)) return;
    var chip = e.target.closest('.rat[role="button"]');
    if (chip && recap.contains(chip)) { jump(recap); return; }
    pending = window.setTimeout(function () {
      pending = 0;
      if (recap.isConnected && !selecting(recap)) jump(recap);
    }, DOUBLE);
  });
  document.addEventListener('keydown', function (e) {
    if (e.defaultPrevented || (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar')) return;
    if (e.isComposing || e.repeat || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (!e.target || !e.target.matches || !e.target.matches(CHIP)) return;
    e.preventDefault();
    jump(e.target.parentElement);
  });
  window.wbRecapJump = { jump: jump };
})();

/* Latest agent arrival: state belongs to the page, not to replaceable board
   nodes. Initial content (including inert history) is never an arrival. */
(function () {
  if (!window.wbLatestLog) {
    var started = Date.now(), topics = new Set(), seen = new Set(), watched = new Map();
    // Seeing the header recap must not consume the lower log's first sight.
    var selector = '.wb-entry.agent.current';
    function key(node) {
      var topic = node.closest('.topic');
      return topic && topic.getAttribute('data-topic') + '\n' + node.getAttribute('data-entry');
    }
    function baseline(root) {
      root.querySelectorAll('[data-kind="agent"][data-entry]').forEach(function (node) {
        var k = key(node);
        if (k) seen.add(k);
      });
      root.querySelectorAll('template').forEach(function (t) { if (t.content) baseline(t.content); });
    }
    baseline(document);
    function reduced() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
    var observer = window.IntersectionObserver && new window.IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        var node = entry.target, k = watched.get(node);
        if (!entry.isIntersecting || !entry.intersectionRatio || !k || !node.isConnected) return;
        // The observer accounts for viewport and history-clip intersection;
        // closed disclosures must not consume a pending arrival either.
        for (var p = node.parentElement; p; p = p.parentElement) {
          if (p.tagName === 'DETAILS' && !p.open) return;
        }
        if (!seen.has(k)) {
          seen.add(k);
          if (!reduced()) {
            node.classList.add('wb-log-blink');
            window.setTimeout(function () { node.classList.remove('wb-log-blink'); }, 3000);
          }
        }
        observer.unobserve(node);
        watched.delete(node);
      });
    });
    function init() {
      var candidates = new Map();
      document.querySelectorAll(selector).forEach(function (node) {
        var k = key(node);
        if (!k) return;
        candidates.set(node, k);
        var name = node.closest('.topic').getAttribute('data-topic');
        if (!topics.has(name)) {
          // A closed topic fetched later is old content, not a new log. New
          // topics still qualify when their log was written since page load.
          var time = node.querySelector('time[datetime]');
          if (!time || !(Date.parse(time.getAttribute('datetime')) > started)) seen.add(k);
        }
      });
      candidates.forEach(function (k, node) {
        topics.add(node.closest('.topic').getAttribute('data-topic'));
      });
      watched.forEach(function (k, node) {
        if (!candidates.has(node) || candidates.get(node) !== k || seen.has(k)) {
          observer.unobserve(node);
          watched.delete(node);
          node.classList.remove('wb-log-blink');
        }
      });
      candidates.forEach(function (k, node) {
        if (!observer || seen.has(k) || watched.has(node)) return;
        watched.set(node, k);
        observer.observe(node);
      });
    }
    window.wbLatestLog = { init: init };
    // Board swaps normally bind explicitly; child-list changes also cover
    // history admission and deferred closed-topic insertion. Do not observe
    // classes: our own animation must not trigger another binding pass.
    if (window.MutationObserver) new window.MutationObserver(function (changes) {
      // Ignore unrelated host activity, such as streaming terminal output.
      if (changes.some(function (change) {
        if (change.target.closest && change.target.closest('.topic')) return true;
        return Array.from(change.addedNodes).concat(Array.from(change.removedNodes)).some(function (node) {
          return node.nodeType === 1 && (node.matches('.topic') || node.querySelector('.topic'));
        });
      })) init();
    }).observe(document.body, { childList: true, subtree: true });
  }
  var bind = window.wbBindBoard;
  if (bind && !bind.wbLatestLogBound) {
    window.wbBindBoard = function (root) { bind(root); window.wbLatestLog.init(); };
    window.wbBindBoard.wbLatestLogBound = true;
  }
  window.wbLatestLog.init();
})();
