/* Shared history window. The dashboard calls capture before a board swap,
   init/restore afterwards, and loadTarget before its own validated navigation.
   Loading emits bubbling wbhistoryload so the host can bind newly live frames. */
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
  function valid(id) { return /^(?:p|e)[1-9][0-9]*$/.test(id || ''); }
  function find(root, id) {
    if (!valid(id)) return null;
    var node = root.querySelector('[id="' + id + '"]');
    if (!node) return null;
    if (id[0] === 'e') return node.matches('.wb-entry[data-entry="' + id + '"]') ? node : null;
    return node.matches('.entry-panel') && node.closest('.wb-entry[data-panel="' + id + '"]') ? node : null;
  }
  function nearBottom(h) { return h.scrollHeight - h.clientHeight - h.scrollTop < 12; }
  // A fold (a long log run, or a system-event chip) anchors by its first
  // entry while closed; once open, its entries anchor themselves.
  function isFold(node) { return node.classList.contains('system-run') || node.classList.contains('run-fold'); }
  function position(h) {
    var rect = h.getBoundingClientRect(), anchor = null;
    Array.from(h.querySelectorAll('[data-history-anchor]')).some(function (node) {
      if (isFold(node) && node.open) return false;
      var r = node.getBoundingClientRect();
      if (r.height && r.bottom > rect.top && r.top < rect.bottom) {
        anchor = { id: node.getAttribute('data-history-anchor'), run: isFold(node), offset: r.top - rect.top };
        return true;
      }
      return false;
    });
    return { bottom: nearBottom(h), anchor: anchor, top: h.scrollTop };
  }
  function applyPosition(h, state) {
    if (!state) return;
    if (state.bottom) h.scrollTop = h.scrollHeight;
    else {
      var a = state.anchor;
      var node = a && (a.run ? h.querySelector('details.system-run[data-key="system:' + a.id + '"]') || h.querySelector('details.run-fold[data-key="run:' + a.id + '"]') : find(h, a.id));
      if (!node && a && a.run) {
        // A merged fold is keyed by its new first entry; find the old one inside.
        var event = find(h, a.id);
        node = event && (event.closest('details.run-fold') || event.closest('details.system-run'));
      }
      if (node && node.getBoundingClientRect().height) {
        h.scrollTop += node.getBoundingClientRect().top - h.getBoundingClientRect().top - a.offset;
      } else h.scrollTop = state.top;
    }
    h._wbPosition = position(h);
  }
  // relabel rewrites a fold's key, anchor, and "n entries · from-to" label
  // after a merge, as the server writes them (EntryPage.Segments).
  function relabel(fold) {
    var events = Array.from(fold.querySelectorAll('.wb-entry')), run = fold.classList.contains('run-fold');
    var clock = function (e) { var t = e.querySelector('time.entry-at'); return t ? t.textContent : ''; };
    var from = clock(events[0]), to = clock(events[events.length - 1]), range = from === to ? from : from + '–' + to;
    fold.setAttribute('data-key', (run ? 'run:' : 'system:') + events[0].id);
    fold.setAttribute('data-history-anchor', events[0].id);
    var summary = fold.children[0], label = summary.querySelector('.lbl') || summary;
    label.textContent = window.wsT ? window.wsT(run ? 'js.wb.stream.run' : 'js.wb.stream.system', events.length, range) : (run ? 'log ' : 'system events ') + events.length;
  }
  function logNode(n) { return !!n && (n.matches('details.run-fold') || n.matches('details.system-run') || n.matches('.wb-entry.agent') || n.matches('.wb-entry.system')); }
  // mergeSystems moves an older chip's events to the front of the adjacent,
  // newer chip, which keeps its disclosure node and open state.
  function mergeSystems(older, newer) {
    Array.from(older.children).slice(1).reverse().forEach(function (entry) { newer.insertBefore(entry, newer.children[1] || null); });
    older.remove();
    relabel(newer);
  }
  // A page boundary is not a semantic boundary: a log run, and a system chip
  // inside it, may continue across it. When the newer side opens with a
  // run's fold, the older side's rows join that fold, which keeps its open
  // state; otherwise only a system chip split by the boundary merges.
  function joinBoundary(live, boundary) {
    var kids = Array.from(live.children), i = kids.indexOf(boundary);
    if (i < 1 || !logNode(boundary) || !logNode(kids[i - 1])) return;
    var older = [];
    for (var j = i - 1; j >= 0 && logNode(kids[j]); j--) older.unshift(kids[j]);
    if (boundary.matches('details.run-fold')) {
      var first = boundary.children[1] || null;
      older.forEach(function (n) {
        var fold = n.matches('details.run-fold');
        (fold ? Array.from(n.children).slice(1) : [n]).forEach(function (r) { boundary.insertBefore(r, first); });
        if (fold) n.remove();
      });
      var rows = Array.from(boundary.children), prev = first && rows[rows.indexOf(first) - 1];
      if (prev && prev.matches('details.system-run') && first.matches('details.system-run')) mergeSystems(prev, first);
      relabel(boundary);
      return;
    }
    var last = older[older.length - 1], lastFold = last.matches('details.run-fold') ? last : null;
    var end = lastFold ? lastFold.children[lastFold.children.length - 1] : last;
    if (end.matches('details.system-run') && boundary.matches('details.system-run')) {
      mergeSystems(end, boundary);
      if (lastFold) relabel(lastFold);
    }
  }
  function loadOne(h) {
    var page = h.querySelector('template[data-history-page]');
    if (!page) return false;
    var live = h.querySelector('.history-live'), boundary = live.children[0];
    live.insertBefore(page.content.cloneNode(true), live.firstChild);
    page.remove();
    if (boundary) joinBoundary(live, boundary);
    var button = h.querySelector('.history-older');
    if (button) button.hidden = !h.querySelector('template[data-history-page]');
    if (window.wbBindBoard) window.wbBindBoard(h);
    h.dispatchEvent(new CustomEvent('wbhistoryload', { bubbles: true, detail: { root: h } }));
    return true;
  }
  // Materialize only known targets. Loading all intervening pages preserves
  // the chronological stream; unknown or malformed IDs cannot grow the DOM.
  function materialize(root, id) {
    var node = find(root, id);
    if (node) return node;
    var deferred = Array.from(root.querySelectorAll('template[data-history-topic]')).find(function (t) {
      if (find(t.content, id)) return true;
      return Array.from(t.content.querySelectorAll('template[data-history-page]')).some(function (page) { return !!find(page.content, id); });
    });
    if (deferred) {
      var fragment = deferred.content.cloneNode(true), topic = fragment.querySelector('.topic');
      deferred.parentElement.insertBefore(fragment, deferred);
      deferred.remove();
      init(topic);
      if (window.wbBindBoard) window.wbBindBoard(topic);
      topic.dispatchEvent(new CustomEvent('wbhistoryload', { bubbles: true, detail: { root: topic } }));
      return materialize(root, id);
    }
    var list = histories(root);
    for (var i = 0; i < list.length; i++) {
      var h = list[i];
      var held = Array.from(h.querySelectorAll('template[data-history-page]')).some(function (t) { return !!find(t.content, id); });
      if (!held) continue;
      while (!find(h, id) && loadOne(h)) {}
      return find(h, id);
    }
    return null;
  }
  function loadTarget(root, id) {
    if (!valid(id)) return null;
    var node = materialize(root || document, id);
    if (!node) return null;
    for (var p = node; p; p = p.parentElement) if (p.tagName === 'DETAILS') p.open = true;
    var mark = node;
    if (node.matches('.wb-entry.panel')) {
      var panel = node.querySelector('.entry-panel');
      if (panel) { panel.open = true; mark = panel; }
    }
    // Highlight the reached target once, restarting a running flash.
    mark.classList.toggle('flash', false); void mark.offsetWidth; mark.classList.toggle('flash', true);
    return node;
  }
  function capture(root) {
    return histories(root).map(function (h) {
      var first = h.querySelector('.wb-entry');
      var fold = h.closest('.topic-stream');
      return { key: h.getAttribute('data-history'), first: first && first.id, open: !fold || fold.open, position: position(h),
        folds: Array.from(h.querySelectorAll('details[data-key]')).map(function (d) { return [d.getAttribute('data-key'), d.open]; }),
        systems: Array.from(h.querySelectorAll('.system-run .wb-entry')).map(function (e) { return [e.id, e.closest('.system-run').open]; }) };
    });
  }
  function restore(root, snapshot) {
    // A navigated closed topic may be inert again in the replacement board.
    (snapshot || []).forEach(function (saved) { if (saved.first) materialize(root || document, saved.first); });
    init(root);
    histories(root).forEach(function (h) {
      var saved = (snapshot || []).find(function (s) { return s.key === h.getAttribute('data-history'); });
      if (!saved) return;
      var fold = h.closest('.topic-stream');
      if (fold && saved.open !== undefined) fold.open = saved.open;
      if (saved.first) materialize(h, saved.first);
      var folds = new Map(saved.folds || []);
      h.querySelectorAll('details[data-key]').forEach(function (d) {
        if (folds.has(d.getAttribute('data-key'))) d.open = folds.get(d.getAttribute('data-key'));
      });
      var systems = new Map(saved.systems || []);
      h.querySelectorAll('.system-run').forEach(function (run) {
        var entry = Array.from(run.querySelectorAll('.wb-entry')).find(function (e) { return systems.has(e.id); });
        if (entry) run.open = systems.get(entry.id);
      });
      applyPosition(h, saved.position);
    });
  }
  function init(root) {
    histories(root).forEach(function (h) {
      if (h._wbHistoryBound) return;
      h._wbHistoryBound = true;
      h.scrollTop = h.scrollHeight;
      h._wbPosition = position(h);
      h.addEventListener('scroll', function () { h._wbPosition = position(h); }, { passive: true });
      // Frame grafts keep this container but replace its older-page button.
      h.addEventListener('click', function (e) {
        var button = e.target.closest && e.target.closest('.history-older');
        if (!button || button.closest('.wb-history') !== h) return;
        var saved = position(h);
        loadOne(h);
        applyPosition(h, saved);
      });
      if (typeof ResizeObserver !== 'undefined') {
        var observer = new ResizeObserver(function () {
          if (h.isConnected === false) { observer.disconnect(); return; }
          applyPosition(h, h._wbPosition);
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
  // document) clips it. Each call is its own subscription. onChange gets
  // every report {node, ratio, visible, rect}; onOut runs once per change
  // from ratio > 0 to 0 (a first report of 0 has not left view). seen(node)
  // reads the node's last report without a layout. sweep() drops the
  // subscriptions of nodes no longer on the page.
  //
  // keep(node, {offset, observe, onEnd}) keeps node at its offset from its
  // nearest scroller's top until layout settles: measured now, in layout
  // terms (a running transform, as a slide's, does not count), so it is
  // called before the DOM change it compensates, and apply() right after
  // it. An offset given puts node there instead. A ResizeObserver over the
  // scroller's content (and observe) re-applies on every layout change. The
  // session ends 500 ms after the last change ("quiet"), 5 s after it began
  // ("cap"), on a scroll it did not write ("user"), or on end() ("ended").
  // One session per scroller: a keep on a scroller that has one re-anchors
  // it (node, offset, observed nodes) and restarts both windows, so two
  // callers never take each other's writes for a user scroll. keeping(el)
  // returns the session el has, which other scroll writers leave alone.
  // While a session runs its scroller takes overflow-anchor: none, so the
  // browser's own anchoring does not compensate a second time.
  function viewKit(win) {
    var doc = win.document, observers = {}, reports = new WeakMap(), sessions = new Map();
    var QUIET = 500, CAP = 5000;
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
            var was = sub.ratio;
            sub.ratio = r.ratio;
            if (sub.onChange) sub.onChange(r);
            if (sub.onOut && was > 0 && r.ratio === 0) sub.onOut(r);
          });
        });
      }, { root: null, rootMargin: margin, threshold: [0] });
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
    function keep(node, opt) {
      opt = opt || {};
      var scroller = scrollerOf(node), s = sessions.get(scroller);
      var offset = typeof opt.offset === "number" ? opt.offset : layoutTop(node, scroller) - scrollerTop(scroller);
      if (!s) s = start(scroller);
      s.node = node;
      s.offset = offset;
      if (opt.onEnd) s.ends.push(opt.onEnd);
      if (s.ro) (opt.observe || []).forEach(function (el) { if (el) s.ro.observe(el); });
      s.restart(true);
      if (typeof opt.offset === "number") s.handle.apply();
      return s.handle;
    }
    function start(scroller) {
      var s = { node: null, offset: 0, ends: [], ro: null, quiet: 0, cap: 0, done: false, wrote: scroller.scrollTop };
      var main = scroller === docScroller(), anchorEl = main ? doc.documentElement : scroller, target = main ? win : scroller;
      var was = anchorEl.style.overflowAnchor;
      anchorEl.style.overflowAnchor = "none";
      function apply() {
        if (s.done) return;
        if (!s.node.isConnected) { end("ended"); return; }
        var d = layoutTop(s.node, scroller) - scrollerTop(scroller) - s.offset;
        if (Math.abs(d) >= 0.5) scroller.scrollTop += d;
        s.wrote = scroller.scrollTop;
      }
      function end(reason) {
        if (s.done) return;
        s.done = true;
        sessions.delete(scroller);
        if (s.ro) s.ro.disconnect();
        win.clearTimeout(s.quiet);
        win.clearTimeout(s.cap);
        target.removeEventListener("scroll", onScroll);
        anchorEl.style.overflowAnchor = was;
        s.ends.forEach(function (fn) { fn(reason); });
      }
      function onScroll() {
        if (s.done || Math.abs(scroller.scrollTop - s.wrote) <= 1) return;
        // Content that shrank under the scroll clamps it: not the user's.
        var max = scroller.scrollHeight - scroller.clientHeight;
        if (s.wrote > max + 1 && Math.abs(scroller.scrollTop - max) <= 1) { apply(); return; }
        end("user");
      }
      s.restart = function (both) {
        win.clearTimeout(s.quiet);
        s.quiet = win.setTimeout(function () { end("quiet"); }, QUIET);
        if (!both) return;
        win.clearTimeout(s.cap);
        s.cap = win.setTimeout(function () { end("cap"); }, CAP);
      };
      s.handle = { scroller: scroller, apply: apply, end: function () { end("ended"); } };
      if (win.ResizeObserver) {
        s.ro = new win.ResizeObserver(function () { if (s.done) return; apply(); s.restart(false); });
        (main ? [doc.body] : Array.prototype.slice.call(scroller.children)).forEach(function (el) { if (el) s.ro.observe(el); });
      }
      target.addEventListener("scroll", onScroll, { passive: true });
      sessions.set(scroller, s);
      return s;
    }
    function keeping(scroller) { var s = sessions.get(scroller); return s ? s.handle : null; }
    return { watch: watch, unwatch: unwatch, sweep: sweep, seen: seen, keep: keep, keeping: keeping };
  }
  var kit = viewKit(window);
  window.wbHistory = { init: init, capture: capture, restore: restore, loadTarget: loadTarget,
    watch: kit.watch, unwatch: kit.unwatch, sweep: kit.sweep, seen: kit.seen, keep: kit.keep, keeping: kit.keeping };
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
      if (node) node.scrollIntoView({ block: 'nearest' });
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
