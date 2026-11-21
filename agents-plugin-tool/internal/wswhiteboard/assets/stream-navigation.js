// History target navigation, snapshot restore, initialization, and folding.
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
    // A bottom-window fill may be waiting for an offscreen frame's height
    // in this same gap. Navigation must admit its known target immediately;
    // the pending window still owns the remaining rows and resumes normally.
    if (h._wbHistoryPending.some(function (p) { return p.gap === gap || p.fallback === gap; })) {
      take(gap, 'down'); loadedRows.add(row); refresh(gap);
      h._wbHistoryOut = false;
      admitted(h);
    } else load(h, gap, 'down', prefix);
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
  function loadTarget(root, id, beforeGeometry, fetched) {
    if (!valid(id)) return null;
    var node = materialize(root || document, id, beforeGeometry);
    if (!node) {
      // A target in history the page has not fetched yet resolves once its
      // topic arrives; this call returns null, as for an unknown target.
      var list = targets.get(id) || [], scope = root || document;
      if (!fetched && lazyIn(scope) && !list.some(function (t) { return t.root === scope; })) {
        var t = { root: scope };
        t.promise = hydrateTarget(scope, id).then(function () {
          var rest = (targets.get(id) || []).filter(function (x) { return x !== t; });
          if (rest.length) targets.set(id, rest); else targets.delete(id);
          if (scope.isConnected === false) return null;
          // A concurrent prefetch may already have filled every placeholder.
          // Resolve the actual target, not this response's fill count; a fill
          // of some other content likewise does not prove the target exists.
          return t.claimed ? (materialize(scope, id, beforeGeometry) ? true : null) : loadTarget(scope, id, beforeGeometry, true);
        });
        targets.set(id, list.concat([t]));
      }
      return null;
    }
    openTarget(node);
    var panel = node.matches('.wb-entry.panel') ? node.querySelector('.entry-panel') : node.matches('.entry-panel') ? node : null;
    if (panel) { panel.open = true; if (panelState) panelState.opened(panel); }
    var mark = panel || node;
    mark.classList.toggle('flash', false); void mark.offsetWidth; mark.classList.toggle('flash', true);
    navigate(node);
    return node;
  }
  function navigate(node) {
    // On the dashboard a topic may stand on a docked gallery page that is
    // not shown: that page shows first. A docked topic scrolls in its
    // pane's body (.gal-pbody) under a head that does not stick.
    if (window.WsCards && window.WsCards.activate) window.WsCards.activate(node);
    var topic = node.closest('.topic'), head = topic && topic.querySelector('.tophead'), offset = 12;
    var style = head && window.getComputedStyle(head);
    var sticky = style && style.position === 'sticky';
    if (sticky) offset += (parseFloat(style.top) || 0) + head.offsetHeight;
    var host = topic && (topic.closest('.gal-pane') || topic.closest('#bcol') || topic.closest('.wrap'));
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
      var seen = kit.seen(h.closest('.topic') || h);
      return carried({ key: h.getAttribute('data-history'), shown: !!(seen && seen.visible),
        panels: panelState ? panelState.capture(h) : [],
        rows: Array.from(h.querySelectorAll('.timeline-row')).map(function (n) { return n.getAttribute('data-row'); }),
        entries: Array.from(h.querySelectorAll('.wb-entry')).map(function (n) { return n.id; }),
        systems: Array.from(h.querySelectorAll('.system-run .wb-entry')).map(function (n) { return [n.id, n.closest('.system-run').open]; }),
        loaded: Array.from(h.querySelectorAll('.timeline-row')).filter(function (n) { return loadedRows.has(n); }).map(function (n) { return n.getAttribute('data-row'); }),
        pending: h._wbHistoryPending.map(function (p) {
          var list = p.gap && payload(p.gap).children, next = list && (p.direction === 'up' ? list[list.length - 1] : list[0]);
          var before = p.fallback && payload(p.fallback).children, edge = before && before[before.length - 1];
          return { row: p.row && p.row.getAttribute('data-row'), next: next && next.getAttribute('data-row'), before: edge && edge.getAttribute('data-row'), direction: p.direction, height: p.height, loaded: !!p.loaded };
        }),
        folds: Array.from(h.querySelectorAll('details[data-key]')).map(function (d) { return [d.getAttribute('data-key'), d.open]; }) }, h);
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
        if (deferred && lazy(deferred)) { rehydrate(root, saved); return; }
        if (deferred) {
          var fragment = deferred.content.cloneNode(true), topic = fragment.querySelector('.topic');
          deferred.parentElement.insertBefore(fragment, deferred); deferred.remove(); init(topic); admitted(topic);
          h = histories(topic)[0];
        }
      }
      if (!h) return;
      if (panelState) panelState.snapshot(h, saved);
      var kept = kit.keep(h), changed = restoreRows(h, saved);
      // Rows a new lazy render left out come back with the topic's complete
      // history when the reader had loaded them, or saw the topic: a new
      // batch must not fold away the rows just read.
      var present = new Set(Array.from(h.querySelectorAll('.timeline-row')).map(function (n) { return n.getAttribute('data-row'); }));
      if (h.querySelector('.history-gap[data-lazy]') && (saved.shown ? saved.rows || [] : saved.loaded || []).some(function (k) { return !present.has(k); })) rehydrate(root, saved);
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
      // Owner-loaded rows stay foldable across a swap; rows the initial
      // window admitted again (the new card's own fill) included.
      var loaded = new Set(saved.loaded || []);
      if (loaded.size) h.querySelectorAll('.history-live > .timeline-row').forEach(function (n) {
        if (loaded.has(n.getAttribute('data-row'))) loadedRows.add(n);
      });
      (saved.pending || []).forEach(function (p) {
        if (!valid(p.row)) return;
        var row = h.querySelector('[data-row="' + p.row + '"]');
        if (!row) return;
        var gap = gaps(h).find(function (g) { return Array.from(payload(g).children).some(function (n) { return n.getAttribute('data-row') === p.next; }); });
        var fallback = gaps(h).find(function (g) { return Array.from(payload(g).children).some(function (n) { return n.getAttribute('data-row') === p.before; }); });
        var op = { row: row, gap: gap, fallback: fallback, direction: p.direction, height: p.height, loaded: !!p.loaded };
        if (op.loaded) loadedRows.add(row);
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
        // The window resumes past lazy history once its topic, near the
        // view, has fetched it.
        if (lazy(n)) { fillLater(h); break; }
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
  // fillLater fetches the history of h's topic when the topic comes within
  // 800px of the view, then lets fill() take the rest of its window, so a
  // topic shows the rows an eager page would while a page far from the
  // view fetches nothing.
  function fillLater(h) {
    if (h._wbFillWatch) return;
    var topic = h.closest('section.topic') || h;
    h._wbFillWatch = kit.watch(topic, { margin: '800px', onChange: function (r) {
      if (!r.visible || !h._wbFillWatch) return;
      kit.unwatch(h._wbFillWatch);
      hydrateTopic(topic).then(function () {
        h._wbFillWatch = null;
        // Another path may have fetched the topic first; fill once no gap
        // is still lazy.
        if (h.isConnected === false || h._wbHistoryPending.length || h.querySelector('.history-gap[data-lazy]')) return;
        h._wbHistoryFilling = true;
        fill(h);
      });
    } });
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
      // Kept before any load can exhaust and remove the last gap, so a fold
      // always has a shell to return rows into.
      var first = h.querySelector('.history-live > .history-gap');
      h._wbGapShell = first ? shell(first) : null;
      fresh.push(h);
    });
    // The server folds every row except the current state and latest batch;
    // fill() then cuts the visible bottom window client-side. Batch disclosure
    // writes before any result reads and share one capture per scroller.
    var kept = new Map();
    fresh.forEach(function (h) {
      var scroller = kit.keep(h, { capture: false }).scroller;
      if (!kept.has(scroller)) kept.set(scroller, kit.keep(h));
    });
    if (panelState) fresh.forEach(function (h) { h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore); });
    fresh.forEach(function (h) { h._wbHistoryHeight = Math.ceil(h.querySelector('.history-live').getBoundingClientRect().height); });
    // Fill inside the shared capture: each fill's own keep() joins it, so
    // N histories cost one capture per scroller, not one per history.
    initialOps = [];
    fresh.forEach(function (h) { h._wbHistoryFilling = true; fill(h); });
    var pending = initialOps; initialOps = null;
    progressBatch(pending);
    kept.forEach(function (k) { k.apply(); });
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
        var gap = button.closest('.history-gap'), direction = button.getAttribute('data-load');
        if (lazy(gap)) {
          hydrateTopic(h.closest('.topic')).then(function () { if (!lazy(gap) && gap.isConnected) load(h, gap, direction, null, true); });
          return;
        }
        load(h, gap, direction, null, true);
      });
      if (typeof ResizeObserver !== 'undefined') {
        var observer = new ResizeObserver(function () {
          if (h.isConnected === false) { observer.disconnect(); return; }
          if (!h._wbHistoryPending.length && !h._wbHistoryFilling) return;
          resume(h);
        });
        observer.observe(h.querySelector('.history-live'));
        h._wbHistoryObserver = observer;
      }
      arm(h);
    });
  }
  // Auto-fold: once the whole topic card has left the viewport, the rows an
  // owner load admitted (load above/below, a deep link, their swap restore)
  // go back into their gaps, so loaded history does not pile up on screen.
  // The initial window and the always-live anchors stay. onOut fires only on
  // a change from partly visible to fully out, so a card a deep link just
  // opened is not folded by the observer's first report.
  var loadedRows = new WeakSet(), FOLD_RETRY_MS = 250;
  function arm(h) {
    var topic = h.closest('.topic');
    if (!topic || !h._wbGapShell) return;
    h._wbHistoryOut = false;
    kit.watch(topic, {
      onChange: function (r) { if (r.visible) h._wbHistoryOut = false; },
      onOut: function () { h._wbHistoryOut = true; collapse(h); }
    });
  }
  function collapse(h) {
    window.clearTimeout(h._wbHistoryFoldRetry); h._wbHistoryFoldRetry = 0;
    if (!h._wbHistoryOut || !h.isConnected) return;
    var live = h.querySelector('.history-live'), topic = h.closest('.topic'), a = document.activeElement;
    if (!live || !topic || topic.hasAttribute('data-max')) return;
    var rows = Array.from(live.children).filter(function (n) { return loadedRows.has(n); });
    if (!rows.length) return;
    // Focus inside a row the fold would detach, or an answer in progress,
    // keeps the card as it is; focusout retries. The load button the owner
    // clicked keeps focus and does not count.
    if (a && (rows.some(function (n) { return n.contains(a); }) || ((a.tagName === 'INPUT' || a.tagName === 'TEXTAREA') && a.closest('.topic')))) return;
    // An unfinished owner load ends with the fold; a pending initial window
    // owns its gap, so the next exit retries.
    var pending = h._wbHistoryPending.filter(function (p) { return !p.loaded; });
    if (pending.length) return;
    // Wait for the scroller to be idle: no owner scrolling, no navigation
    // jump and no load-above pin (whose settle window must not be ended or
    // fought). Board motion settles first too. keep() then compensates the
    // fold like any other change above the reading line.
    var scroller = kit.keep(h, { capture: false }).scroller;
    var box = topic.closest('#bcol') || topic.closest('.gal-pane') || topic.closest('.wrap') || document.documentElement;
    var motion = box && box.getAnimations && box.getAnimations({ subtree: true }).some(function (m) {
      var n = m.effect && m.effect.target;
      return m.playState === 'running' && n && n.matches && n.matches('.topic, [data-card], iframe.doc-frame');
    });
    if (motion || !kit.idle(scroller)) {
      h._wbHistoryFoldRetry = window.setTimeout(function () { collapse(h); }, FOLD_RETRY_MS);
      return;
    }
    h._wbHistoryPending = pending;
    var kept = kit.keep(h);
    refold(h);
    kept.apply();
  }
  // Every maximal run of gaps and loaded rows becomes one gap again, in
  // chronological order: the first gap of the run, or the kept shell when
  // the run's own gap was exhausted.
  function refold(h) {
    var live = h.querySelector('.history-live'), kids = Array.from(live.children);
    function foldable(n) { return loadedRows.has(n) || n.matches('.history-gap'); }
    for (var i = 0; i < kids.length;) {
      if (!foldable(kids[i])) { i++; continue; }
      var run = [];
      while (i < kids.length && foldable(kids[i])) run.push(kids[i++]);
      if (!run.some(function (n) { return loadedRows.has(n); })) continue;
      var gap = run.find(function (n) { return n.matches('.history-gap'); });
      if (!gap) { gap = h._wbGapShell.cloneNode(true); live.insertBefore(gap, run[0]); }
      var rows = [];
      run.forEach(function (n) {
        if (n.matches('.history-gap')) rows = rows.concat(Array.from(payload(n).children));
        else rows.push(n);
      });
      var held = payload(gap);
      rows.forEach(function (r) { loadedRows.delete(r); held.appendChild(r); });
      run.forEach(function (n) { if (n !== gap && n.matches('.history-gap')) n.remove(); });
      refresh(gap);
    }
  }
