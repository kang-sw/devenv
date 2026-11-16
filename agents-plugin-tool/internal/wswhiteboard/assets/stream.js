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
    if (!h._wbHistoryNotify) {
      h._wbHistoryNotify = true;
      window.requestAnimationFrame(function () {
        h._wbHistoryNotify = false;
        if (h.isConnected !== false) h.dispatchEvent(new CustomEvent('wbhistoryload', { bubbles: true, detail: { root: h } }));
      });
    }
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
    if (height > 0) return height;
    // A laid-out row whose content is all hidden (display:none inside) still
    // has a box: it is ready at zero height and must not block the window.
    // Only a row with no box (a host not laid out yet) waits for a resize.
    return row.getClientRects && row.getClientRects().length ? 0 : null;
  }
  // Initial histories share rounds: write every topic's next batch, then
  // read all rows without intervening writes. Surplus stays inert and never
  // binds or mounts frames. Exact whole-row windows and unknown-height waits
  // remain unchanged; 32 tiny rows cost one layout, not 32 layouts.
  var initialOps = null, resumeHistories = new Set(), resumeFrame = false;
  function resume(h) {
    resumeHistories.add(h);
    if (resumeFrame) return;
    resumeFrame = true;
    window.requestAnimationFrame(function () {
      resumeFrame = false;
      var histories = Array.from(resumeHistories).filter(function (h) { return h.isConnected !== false; });
      resumeHistories.clear();
      initialOps = [];
      histories.forEach(function (h) {
        h._wbHistoryPending.slice().forEach(function (op) { progress(h, op); });
        if (h._wbHistoryFilling) fill(h);
      });
      var pending = initialOps; initialOps = null;
      progressBatch(pending);
    });
  }
  function progress(h, op) {
    if (initialOps) { initialOps.push({ h: h, op: op }); return; }
    progressBatch([{ h: h, op: op }]);
  }
  function progressBatch(items) {
    items = items.filter(function (item) { return !item.op.busy && !item.op.done; });
    items.forEach(function (item) {
      item.op.busy = true; item.kept = kit.keep(item.h);
      item.touched = new Set(); item.changed = false;
    });
    try {
      var active = items.slice();
      while (active.length) {
        active.forEach(function (item) {
          var op = item.op, rows = item.rows = [], pending = op.row;
          if (pending) rows.push(pending);
          while (rows.length < 32 && op.height < WINDOW_PX) {
            var row = take(op.gap, op.direction);
            if (!row) break;
            rows.push(row); item.touched.add(op.gap);
          }
          item.pending = pending;
        });
        // One read phase for the entire board, including all 27 topics.
        active.forEach(function (item) { item.heights = item.rows.map(readyHeight); });
        active = active.filter(function (item) {
          var op = item.op, used = 0, waiting = false;
          while (used < item.rows.length && op.height < WINDOW_PX) {
            var row = item.rows[used], height = item.heights[used++];
            if (row !== item.pending) { item.changed = true; if (op.loaded) loadedRows.add(row); }
            op.row = height === null ? row : null;
            if (height === null) { waiting = true; break; }
            op.height += height;
          }
          // Undo speculative admission in reverse take order.
          var held = op.gap && payload(op.gap);
          for (var i = item.rows.length - 1; i >= used; i--) {
            if (op.direction === 'up') held.appendChild(item.rows[i]);
            else held.insertBefore(item.rows[i], held.children[0] || null);
          }
          if (waiting) return false;
          if (op.height < WINDOW_PX && item.rows.length) return true;
          if (op.height < WINDOW_PX && op.fallback) {
            op.gap = op.fallback; op.fallback = null; op.direction = 'up'; return true;
          }
          op.done = true;
          item.h._wbHistoryPending = item.h._wbHistoryPending.filter(function (p) { return p !== op; });
          if (!op.loaded) {
            if (op.height >= WINDOW_PX) item.h._wbHistoryFilling = false;
            else if (item.h._wbHistoryFilling) resume(item.h);
          }
          return false;
        });
      }
    } finally {
      items.forEach(function (item) { item.touched.forEach(refresh); });
      items.forEach(function (item) { if (item.changed) admitted(item.h); item.op.busy = false; });
      items.forEach(function (item) { item.kept.apply(); });
      items.forEach(function (item) { if (item.changed && item.op.pin) item.op.pin.apply(); });
    }
  }
  function load(h, gap, direction, fallback, pinned) {
    if (h._wbHistoryPending.some(function (p) { return p.gap === gap || p.fallback === gap; })) return;
    // An owner load re-arms the fold: it runs only after the card has been
    // in view since this load and then fully left it.
    h._wbHistoryOut = false;
    var op = { gap: gap, direction: direction, fallback: fallback, height: 0, row: null, loaded: true };
    var below = pinned && direction === 'up' && gap.nextElementSibling;
    if (below && below.matches('.timeline-row')) op.pin = kit.pin(below);
    h._wbHistoryPending.push(op);
    progress(h, op);
  }
  // Clone the small control shell, never the potentially huge payload.
  function shell(gap) {
    var copy = gap.cloneNode(false);
    Array.from(gap.children).forEach(function (n) { copy.appendChild(n.cloneNode(!n.matches('template[data-history-gap]'))); });
    return copy;
  }
  // Split only the inert gap containing a known target. The prefix stays
  // inert; loading starts at the target, not at a distant page boundary.
  function split(gap, row) {
    var held = payload(gap), prefix = shell(gap);
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
        loaded: Array.from(h.querySelectorAll('.timeline-row')).filter(function (n) { return loadedRows.has(n); }).map(function (n) { return n.getAttribute('data-row'); }),
        pending: h._wbHistoryPending.map(function (p) {
          var list = p.gap && payload(p.gap).children, next = list && (p.direction === 'up' ? list[list.length - 1] : list[0]);
          var before = p.fallback && payload(p.fallback).children, edge = before && before[before.length - 1];
          return { row: p.row && p.row.getAttribute('data-row'), next: next && next.getAttribute('data-row'), before: edge && edge.getAttribute('data-row'), direction: p.direction, height: p.height, loaded: !!p.loaded };
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
        load(h, button.closest('.history-gap'), button.getAttribute('data-load'), null, true);
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
    var box = topic.closest('#bcol') || topic.closest('#bpins') || topic.closest('.wrap') || document.documentElement;
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
    var watchNodes = new Set(), watchFrame = false;
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
      if (sub.node.wbWatch === sub.handle) {
        delete sub.node.wbWatch;
        delete sub.node.wbViewedExposure;
      }
      var o = observers[sub.margin], list = o && o.subs.get(sub.node);
      if (!list) return;
      list.splice(list.indexOf(sub), 1);
      if (!list.length) { o.subs.delete(sub.node); o.io.unobserve(sub.node); }
    }
    function watch(node, opt) {
      opt = opt || {};
      var sub = { node: node, margin: opt.margin || "0px", onChange: opt.onChange, onOut: opt.onOut, ratio: undefined, gone: false };
      var handle = { node: node, unwatch: function () { drop(sub); } };
      sub.handle = handle;
      if (!node || !win.IntersectionObserver) { sub.gone = true; return handle; }
      watchNodes.add(node);
      if (!watchFrame) {
        watchFrame = true;
        win.requestAnimationFrame(function () {
          watchFrame = false;
          var scrollers = new Set();
          watchNodes.forEach(function (n) { if (rendered(n)) scrollers.add(scrollerOf(n)); });
          watchNodes.clear();
          scrollers.forEach(owner);
        });
      }
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
    function viewport(scroller, read) {
      read = read || function (n) { return n.getBoundingClientRect(); };
      var main = scroller === docScroller(), r = main ? { top: 0, bottom: win.innerHeight || scroller.clientHeight } : read(scroller);
      var top = Math.max(0, r.top + (main ? 0 : scroller.clientTop || 0));
      var bottom = Math.min(win.innerHeight || scroller.clientHeight, main ? r.bottom : r.top + (scroller.clientTop || 0) + scroller.clientHeight);
      var style = win.getComputedStyle(scroller);
      top += parseFloat(style.scrollPaddingTop) || 0;
      // The rail publishes its clearance on sticky heads, not on the host.
      Array.from((main ? doc : scroller).querySelectorAll('.tophead, .bc-hd, .wb-rail-wrap.up .wb-rail-stack')).filter(rendered).forEach(function (head) {
        var hs = win.getComputedStyle(head), h = read(head);
        if ((hs.position === 'sticky' || head.matches('.wb-rail-stack')) && h.height && h.top <= top + (parseFloat(hs.top) || 0) + 1 && h.bottom > top)
          top = Math.max(top, h.bottom);
      });
      return { top: top, bottom: bottom, left: main ? 0 : Math.max(0, r.left + (scroller.clientLeft || 0)),
        right: Math.min(win.innerWidth || r.right || Infinity, main ? win.innerWidth || Infinity : r.left + (scroller.clientLeft || 0) + scroller.clientWidth) };
    }
    // A geometry read inside closed details can itself force a display-lock
    // update and full-document layout. Reject hidden ancestry BEFORE any rect,
    // client box or scroller inspection; the details' own summary still shows.
    function rendered(node) {
      if (!node || node.isConnected === false) return false;
      for (var p = node, branch = node; p; branch = p, p = p.parentElement) {
        if (p.hidden) return false;
        if (p !== node && p.tagName === 'DETAILS' && !p.open) {
          var summary = Array.from(p.children || []).find(function (n) { return n.tagName === 'SUMMARY'; });
          if (branch !== summary) return false;
        }
      }
      if (node.checkVisibility) return node.checkVisibility({ visibilityProperty: true });
      // Older engines lack checkVisibility. Style reads do not open a hidden
      // details subtree's forced geometry scope (already rejected above).
      for (var p = node; p; p = p.parentElement) {
        var s = win.getComputedStyle(p);
        if (s.display === 'none' || (p === node && (s.visibility === 'hidden' || s.visibility === 'collapse')) || s.contentVisibility === 'hidden') return false;
      }
      return true;
    }
    function exposed(node, scroller, view, read) {
      if (!rendered(node)) return false;
      read = read || function (n) { return n.getBoundingClientRect(); };
      var v = view || viewport(scroller, read), r = read(node), top = Math.max(v.top, r.top), bottom = Math.min(v.bottom, r.bottom);
      var left = Math.max(v.left, r.left), right = Math.min(v.right, r.right);
      if (!(r.height > 0 && bottom - top >= 1 && right - left >= 1)) return false;
      for (var p = node.parentElement; p && p !== scroller && p !== doc.body; p = p.parentElement) {
        if (/hidden|clip|auto|scroll/.test(win.getComputedStyle(p).overflowY)) {
          var box = read(p); top = Math.max(top, box.top); bottom = Math.min(bottom, box.bottom);
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
      var selector = '.entry-panel, .wb-entry:not(.panel), .doc-frame, [data-card]', rects = new Map();
      function read(n) { if (!rects.has(n)) rects.set(n, n.getBoundingClientRect()); return rects.get(n); }
      function eligible(n) {
        if (!rendered(n) || (n.matches('.doc-frame') && n.closest('.entry-panel'))) return false;
        return exposed(n, scroller, view, read) && scrollerOf(n) === scroller;
      }
      // Frame-height batches normally capture the unit under the reading line
      // in constant work, rather than enumerating the entire long board.
      if (doc.elementFromPoint) {
        var hit = doc.elementFromPoint((view.left + view.right) / 2, line), path = [];
        for (var p = hit; p && p !== root; p = p.parentElement) {
          if (p.matches && p.matches(selector) && eligible(p)) path.push(p);
        }
        path.sort(function (a, b) { return read(a).top - read(b).top; });
        var atLine = path.find(function (n) { var r = read(n); return r.top <= line && r.bottom > line; });
        if (atLine) return atLine;
      }
      // Gaps, overlays and old engines retain the exact previous selection
      // rule, but hidden history/system runs are never measured and each
      // candidate rect is read once, including sorting/line selection.
      var nodes = Array.from(root.querySelectorAll(selector)).filter(rendered).filter(eligible);
      nodes.sort(function (a, b) { return read(a).top - read(b).top; });
      return nodes.find(function (n) { var r = read(n); return r.top <= line && r.bottom > line; }) ||
        nodes.find(function (n) { return read(n).top > line; }) || null;
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
        if (o.pin) o.pin.end();
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
    // pin(ref) holds ref's place in its scroller for a settle window: apply()
    // scrolls by however far ref moved since the call, and a ResizeObserver
    // re-applies it before paint while late content (images, panel frames)
    // sizes. The window ends at its non-renewing cap, on owner input, or when
    // a jump takes the scroller; after that apply() is a no-op and ordinary
    // keep() transactions own the scroller again. Writes go through the scroll
    // owner so they are not taken for user input.
    var pins = new Set(), PIN_MS = 1500;
    function pin(ref) {
      var scroller = scrollerOf(ref), o = owner(scroller), top = layoutTop(ref, scroller);
      if (o.pin) o.pin.end();
      var p = { done: false, ro: null, cap: 0 };
      p.apply = function () {
        if (p.done) return;
        if (!ref.isConnected) { p.end(); return; }
        var delta = layoutTop(ref, scroller) - top;
        if (Math.abs(delta) >= 0.5) scroller.scrollTop += delta;
        o.wrote = scroller.scrollTop;
      };
      p.end = function () {
        if (p.done) return;
        p.done = true; pins.delete(p);
        if (o.pin === p) o.pin = null;
        if (p.ro) p.ro.disconnect();
        win.clearTimeout(p.cap);
      };
      o.pin = p; pins.add(p);
      p.cap = win.setTimeout(p.end, PIN_MS);
      if (win.ResizeObserver) {
        p.ro = new win.ResizeObserver(p.apply);
        var content = scroller === docScroller() ? [doc.body] : Array.prototype.slice.call(scroller.children);
        content.concat([ref.parentElement]).forEach(function (el) { if (el) p.ro.observe(el); });
      }
      return p;
    }
    function pinning() { return pins.size; }
    // idle(scroller): no owner input in the last 200 ms, no jump following
    // and no pin holding the scroller, so a keep() transaction would write
    // without cutting a settle window short.
    function idle(scroller) { var o = owner(scroller); return o.ready() && !sessions.get(scroller) && !o.pin; }
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
      if (opt && opt.prepare && s) {
        if (s.cancel) s.cancel(); else s.handle.end();
        s = null;
      }
      if (!s || !s.follow) s = start(scroller);
      if (opt && opt.onEnd) s.ends.push(opt.onEnd);
      if (opt && opt.prepare) opt.prepare();
      s.jump(node, opt || {});
      return s.handle;
    }
    function start(scroller) {
      var o = owner(scroller);
      if (o.transaction) o.transaction.cancel();
      if (o.pin) o.pin.end();
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
          if (typeof opt.top === 'number') return opt.top;
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
          if (typeof opt.top === 'number' || typeof opt.offset === 'number' || typeof opt.readingOffset === 'number') scroller.scrollTo({ top: measure(), behavior: behavior });
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
      var views = new Map(), under = new Map(), rects = new Map(), visibility = new Map();
      function read(node) { if (!rects.has(node)) rects.set(node, node.getBoundingClientRect()); return rects.get(node); }
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
        if (visibility.has(node)) return visibility.get(node);
        if (!rendered(node)) { visibility.set(node, false); return false; }
        var s = scroller(node), v = views.get(s);
        if (!v) { v = viewport(s, read); views.set(s, v); }
        var result = exposed(node, s, v, read);
        visibility.set(node, result);
        return result;
      }
      return { scroller: scroller, visible: visible };
    }
    return { watch: watch, unwatch: unwatch, sweep: sweep, seen: seen, keep: keep, pin: pin, pinning: pinning, idle: idle, place: place, keeping: keeping, jump: jump, bands: bands, snapshot: snapshot, rendered: rendered };
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
      // Earlier versions are sibling rows of the history, live or inert in a gap.
      var history = i.article.closest('.wb-history') || i.article, versions = [i.article];
      [history].concat(Array.from(history.querySelectorAll('template')).map(function (t) { return t.content; })).forEach(function (r) {
        if (r) versions = versions.concat(Array.from(r.querySelectorAll('.wb-entry[data-entry]')));
      });
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
    watch: kit.watch, unwatch: kit.unwatch, sweep: kit.sweep, seen: kit.seen, keep: kit.keep, pin: kit.pin, pinning: kit.pinning, place: kit.place, keeping: kit.keeping, jump: kit.jump, bands: kit.bands, snapshot: kit.snapshot, rendered: kit.rendered };
  // An entry's clock shows its relative time on hover, computed when shown
  // so it never goes stale (ago.js's wording), then the local date and time.
  document.addEventListener('mouseover', function (e) {
    var t = e.target && e.target.closest && e.target.closest('time.entry-at[data-at]');
    if (t && window.wbAgoText) t.title = window.wbAgoText(+t.getAttribute('data-at')) + ' · ' + (t.getAttribute('data-stamp') || '');
  });
  // A panel's link menu copies two links: pN always opens the newest
  // version, eN this exact one. The page URL is kept, since both the file
  // page and the dashboard jump to a fragment on load.
  function linkMenuClose() {
    Array.from(document.querySelectorAll('.pfmenu')).forEach(function (m) {
      var b = m.parentElement && m.parentElement.querySelector('button.pflink');
      if (b) { b.classList.remove('on'); b.setAttribute('aria-expanded', 'false'); }
      m.remove();
    });
  }
  function linkCopy(text) {
    // Through a scratch textarea: the async clipboard API is denied in
    // sandboxed hosts and absent over plain http (code.js does the same).
    var ta = document.createElement('textarea'), ok = false;
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    document.body.appendChild(ta);
    ta.select();
    try { ok = document.execCommand('copy'); } catch (_) { ok = false; }
    ta.remove();
    return ok;
  }
  function linkMenu(btn) {
    var article = btn.closest('.wb-entry.panel'), T = window.wsT || function (k) { return k; };
    var panel = article.getAttribute('data-panel'), entry = article.getAttribute('data-entry');
    var newest = btn.getAttribute('data-newest') || '', ver = btn.getAttribute('data-ver') || '';
    var menu = document.createElement('div'), head = document.createElement('div');
    menu.className = 'pfmenu'; menu.setAttribute('role', 'menu');
    head.className = 'pfmh'; head.textContent = T("js.wb.link.copy");
    menu.appendChild(head);
    [[panel, 'link', T("js.wb.link.latest"), article.classList.contains('historical') ? T("js.wb.link.latest.old", newest) : T("js.wb.link.latest.now", newest)],
      [entry, 'history', T("js.wb.link.this"), T("js.wb.link.this.desc", ver)]].forEach(function (it) {
      var b = document.createElement('button'), text = document.createElement('span'), k = document.createElement('span'), code = document.createElement('code'), d = document.createElement('span');
      b.type = 'button'; b.className = 'pfmi'; b.setAttribute('role', 'menuitem'); b.setAttribute('data-copy', it[0]);
      text.className = 'pfmt'; k.className = 'pfk'; d.className = 'pfd';
      code.textContent = '#' + it[0]; d.textContent = it[3];
      k.append(it[2] + ' ', code); text.append(k, d);
      b.innerHTML = '<svg class="i" aria-hidden="true"><use href="#i-' + it[1] + '"/></svg>';
      b.appendChild(text);
      b.insertAdjacentHTML('beforeend', '<svg class="i" aria-hidden="true"><use href="#i-copy"/></svg>');
      b.addEventListener('mouseenter', function () { b.focus(); });
      menu.appendChild(b);
    });
    article.appendChild(menu);
    btn.classList.add('on'); btn.setAttribute('aria-expanded', 'true');
    menu.querySelector('.pfmi').focus();
  }
  document.addEventListener('click', function (e) {
    var t = e.target, btn = t.closest && t.closest('button.pflink'), item = t.closest && t.closest('.pfmenu .pfmi');
    if (item) {
      e.preventDefault(); e.stopPropagation();
      if (linkCopy(location.href.split('#')[0] + '#' + item.getAttribute('data-copy'))) {
        item.querySelector('.pfd').textContent = window.wsT ? window.wsT("js.wb.copy.done") : 'Copied';
        window.setTimeout(linkMenuClose, 700);
      }
      return;
    }
    if (t.closest && t.closest('.pfmenu')) return;
    var open = btn && btn.getAttribute('aria-expanded') === 'true';
    linkMenuClose();
    if (!btn) return;
    // The button sits in a panel summary: never toggle the panel.
    e.preventDefault(); e.stopPropagation();
    if (!open) linkMenu(btn);
  }, true);
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && document.querySelector('.pfmenu')) linkMenuClose();
  });
  var bind = window.wbBindBoard;
  if (bind) {
    window.wbBindBoard = function (root) { bind(root); init(root); };
    window.wbBindBoard.wbHistoryBound = true;
  }
  init(document);
  // A fold that focus held back runs once focus leaves (collapse returns at
  // once for cards still in view or with nothing loaded).
  document.addEventListener('focusout', function () {
    window.setTimeout(function () { histories(document).forEach(collapse); }, 0);
  });

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
    var topic = recap.closest('.topic'), entry = recap.getAttribute('data-entry'), history = window.wbHistory;
    if (!topic || !history || !history.loadTarget) return null;
    if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(function () { jump(recap); })) return null;
    if (topic.isConnected === false) topic = topic.id ? document.getElementById(topic.id) : null;
    if (!topic) return null;
    // A dashboard card fold hides the whole topic; reveal it before geometry.
    var unfold = window.WsCards && window.WsCards.unfoldTopic;
    var row = history.loadTarget(topic, entry, unfold);
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
