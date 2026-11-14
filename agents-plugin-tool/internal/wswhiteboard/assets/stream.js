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
  var LIMIT = 1000, PANELS = 3, LINES = 30;
  // A fold (a long log run, or a system-event chip) anchors by its first
  // entry while closed; once open, its entries anchor themselves.
  function isFold(node) { return node.classList.contains('system-run') || node.classList.contains('run-fold'); }
  function rows(h) {
    return Array.from(h.querySelector('.history-live').querySelectorAll('.wb-entry')).filter(function (e) {
      return !e.closest('.panel-versions');
    });
  }
  // Log-only cuts can fall inside a disclosure. Split just that ancestry,
  // keeping its open state and chronological rows on both sides of the cut.
  function splitAt(h, node) {
    var live = h.querySelector('.history-live');
    while (node.parentElement !== live) {
      var fold = node.parentElement;
      if (!isFold(fold)) return fold;
      if (node === fold.children[1]) { node = fold; continue; }
      var newer = fold.cloneNode(false);
      newer.appendChild(fold.children[0].cloneNode(true));
      fold.parentElement.insertBefore(newer, fold.nextElementSibling);
      Array.from(fold.children).slice(Array.from(fold.children).indexOf(node)).forEach(function (c) { newer.appendChild(c); });
      relabel(fold); relabel(newer);
      node = newer;
    }
    return node;
  }
  function topRow(h, node) {
    var live = h.querySelector('.history-live');
    while (node.parentElement !== live) node = node.parentElement;
    return node;
  }
  function boundary(h) { return h._wbHistoryChunks[h._wbHistoryChunks.length - 1]; }
  function fit(h) {
    var id = boundary(h), node = id && find(h, id), live = h.querySelector('.history-live');
    if (!node) return;
    node = splitAt(h, node);
    // Only the saved boundary's geometry changes on resize, never its ID.
    var height = Math.max(0, live.getBoundingClientRect().bottom - node.getBoundingClientRect().top);
    h.style.setProperty('--wb-history-height', Math.ceil(height) + 'px');
  }
  function later(h) {
    if (h._wbHistoryFrame) return;
    h._wbHistoryFrame = true;
    window.requestAnimationFrame(function () {
      h._wbHistoryFrame = false;
      if (h.isConnected) {
        var target = h._wbHistoryTarget;
        if (target && target.isConnected && h.contains(target)) reveal(h, target);
        else if (target) h._wbHistoryTarget = null;
        choose(h); fit(h); updateControl(h);
      }
    });
  }
  // Store each panel's first usable rendered height, keyed by disclosure
  // state. An open dashboard frame with no stored height has not rendered
  // yet; wait for its first height rather than judging its placeholder.
  function panelHeight(h, node) {
    var panel = node.querySelector('.entry-panel'), key = node.id + ':' + !!panel.open;
    if (h._wbHistoryHeights[key] !== undefined) return h._wbHistoryHeights[key];
    if (panel.open && Array.from(panel.querySelectorAll('.doc-frame')).some(function (f) {
      for (var p = f.parentElement; p && p !== panel; p = p.parentElement) if (p.tagName === 'DETAILS' && !p.open) return false;
      return !parseFloat(f.style.height);
    })) return null;
    var height = node.getBoundingClientRect().height;
    if (!height) return null;
    h._wbHistoryHeights[key] = height;
    return height;
  }
  function choose(h) {
    if (!h._wbHistoryPending || h._wbHistoryChoosing) return;
    h._wbHistoryChoosing = true;
    try {
      var all = rows(h), before = h._wbHistoryPending.before;
      var end = before ? all.findIndex(function (e) { return e.id === before; }) : all.length;
      if (end < 0) return;
      var older = all.slice(0, end), panels = older.filter(function (e) { return e.classList.contains('panel'); });
      var pages = Array.from(h.querySelectorAll('template[data-history-page]'));
      var heldPanel = pages.some(function (p) { return !!p.content.querySelector('.wb-entry.panel'); });
      // Materialize one page per frame, and only while the candidate needs
      // more panels (or thirty log lines). No pixel-fill loading loop.
      if (pages.length && ((panels.length < PANELS && heldPanel) || (!panels.length && older.length < LINES))) {
        loadOne(h); later(h); return;
      }
      if (!older.length) { h._wbHistoryPending = null; return; }
      var first;
      if (panels.length) {
        var candidates = panels.slice(-PANELS), heights = candidates.map(function (p) { return panelHeight(h, p); });
        if (heights.some(function (n) { return n === null; })) {
          // Show the provisional candidates so hidden topics can render.
          first = candidates[0];
        } else {
          var count = candidates.length;
          var tail = before ? topRow(h, find(h, before)).getBoundingClientRect().top : h.querySelector('.history-live').getBoundingClientRect().bottom;
          while (count > 1) {
            var start = candidates.length - count, p = candidates[start];
            // Geometry supplies logs, gaps and folded runs. Replace only
            // panel boxes with cached heights for the one-time judgment.
            var height = tail - p.getBoundingClientRect().top;
            for (var i = start; i < candidates.length; i++) height += heights[i] - candidates[i].getBoundingClientRect().height;
            if (height <= LIMIT) break;
            count--;
          }
          first = candidates[candidates.length - count];
          h._wbHistoryPending = null;
        }
      } else {
        first = older[Math.max(0, older.length - LINES)];
        h._wbHistoryPending = null;
      }
      h._wbHistoryChunks[h._wbHistoryChunks.length - 1] = first.id;
    } finally { h._wbHistoryChoosing = false; }
  }
  function reset(h) {
    h._wbHistoryChunks = [null];
    h._wbHistorySteps = 1;
    h._wbHistoryPending = { before: null };
    choose(h); fit(h);
  }
  function updateControl(h) {
    var all = rows(h);
    var more = !!h.querySelector('template[data-history-page]') || (!!boundary(h) && all[0] && all[0].id !== boundary(h));
    var button = h.querySelector('.history-older');
    if ((!button || button.hidden === !more) && h.classList.contains('history-more') === more) return;
    // Hiding the exhausted control resizes shallower ancestors watched by
    // keep/rail. Do that outside this history's resize delivery, including
    // reentrant init calls from loadOne -> wbBindBoard.
    if (h._wbHistoryResizing) {
      if (!h._wbHistoryControlFrame) {
        h._wbHistoryControlFrame = true;
        window.requestAnimationFrame(function () {
          h._wbHistoryControlFrame = false;
          if (h.isConnected) updateControl(h);
        });
      }
      return;
    }
    if (button) button.hidden = !more;
    h.classList.toggle('history-more', more);
  }
  function oldest(h) {
    var clip = h.querySelector('.history-clip').getBoundingClientRect();
    return Array.from(h.querySelectorAll('[data-history-anchor]')).find(function (node) {
      if (isFold(node) && node.open) return false;
      var r = node.getBoundingClientRect();
      return r.height && r.bottom > clip.top && r.top < clip.bottom;
    });
  }
  // A fully out card has no internal anchor. Keep the topmost visible
  // content below it in the same column (or the standalone document).
  function below(topic) {
    var box = topic.closest('#bpins') || topic.closest('#bcol'), root = box || document;
    var r = box ? box.getBoundingClientRect() : { top: 0, bottom: window.innerHeight };
    var top = Math.max(0, r.top), bottom = Math.min(window.innerHeight, r.bottom);
    if (topic.getBoundingClientRect().bottom > top) return null;
    return Array.from(root.querySelectorAll('.topic, [data-card], .reply')).filter(function (node) {
      if (node === topic || topic.contains(node) || !(topic.compareDocumentPosition(node) & 4)) return false;
      var n = node.getBoundingClientRect();
      return n.height && n.bottom > top && n.top < bottom;
    }).sort(function (a, b) { return a.getBoundingClientRect().top - b.getBoundingClientRect().top; })[0];
  }
  function collapse(h) {
    if (!h._wbHistoryOut || (h._wbHistorySteps || 1) === 1) return;
    var topic = h.closest('.topic'), a = document.activeElement;
    if (!topic || !topic.isConnected || topic.hasAttribute('data-max')) return;
    if (a && (h.contains(a) || ((a.tagName === 'INPUT' || a.tagName === 'TEXTAREA') && a.closest('.topic')))) return;
    var root = topic.closest('#bcol') || topic.closest('.wrap') || document;
    var motion = root.getAnimations ? root.getAnimations({ subtree: true }).filter(function (m) {
      var n = m.effect && m.effect.target;
      return m.playState === 'running' && n && n.matches('.topic, [data-card], iframe.doc-frame');
    }) : [];
    if (motion.length) {
      if (!h._wbHistoryWaiting) {
        h._wbHistoryWaiting = true;
        Promise.all(motion.map(function (m) { return m.finished.catch(function () {}); })).then(function () {
          h._wbHistoryWaiting = false; collapse(h);
        });
      }
      return;
    }
    var anchor = below(topic), kept = anchor && kit.keep(anchor, { observe: [h.querySelector('.history-live')] });
    h._wbHistoryOut = false;
    h._wbHistoryTarget = null;
    reset(h);
    updateControl(h);
    if (kept) kept.apply();
  }
  function arm(h) {
    var topic = h.closest('.topic');
    h._wbHistoryOut = false;
    kit.unwatch(h._wbHistoryWatch);
    if (topic) h._wbHistoryWatch = kit.watch(topic, {
      onChange: function (r) { if (r.visible) h._wbHistoryOut = false; },
      onOut: function () { h._wbHistoryOut = true; collapse(h); }
    });
  }
  function reveal(h, node) {
    var all = rows(h), entry = node.closest('.wb-entry'), id = boundary(h);
    // A closed log/system summary has no entry ancestor. Its existing
    // anchor names the first entry represented by that disclosure.
    var fold = !entry && node.closest('.run-fold, .system-run');
    if (fold) entry = find(h, fold.getAttribute('data-history-anchor'));
    if (entry) entry = topRow(h, entry).closest('.wb-entry') || entry;
    if (entry && (!id || all.indexOf(entry) < all.findIndex(function (e) { return e.id === id; }))) {
      h._wbHistoryPending = null;
      h._wbHistoryChunks.push(entry.id);
      h._wbHistorySteps = h._wbHistoryChunks.length;
    }
    fit(h);
    updateControl(h);
  }
  function navigate(h, node) {
    reveal(h, node);
    arm(h);
    h._wbHistoryTarget = node;
    // Page navigation must clear the topic's sticky head AND the rail/host
    // stack above it. A fixed page-top offset can leave keyboard focus
    // inside the clip but hidden behind those capsules on either surface.
    var head = h.closest('.topic').querySelector('.tophead'), offset = 12;
    var style = head && window.getComputedStyle(head);
    if (style && style.position === 'sticky') offset += (parseFloat(style.top) || 0) + head.offsetHeight;
    kit.keep(node, { offset: offset, observe: [h.querySelector('.history-live')], onEnd: function () {
      if (h._wbHistoryTarget === node) h._wbHistoryTarget = null;
    } });
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
    var h = node.closest('.wb-history');
    if (h) navigate(h, node);
    return node;
  }
  function capture(root) {
    return histories(root).map(function (h) {
      var first = h.querySelector('.wb-entry');
      var fold = h.closest('.topic-stream');
      return { key: h.getAttribute('data-history'), first: first && first.id, open: !fold || fold.open, steps: h._wbHistorySteps || 1,
        chunks: h._wbHistoryChunks.slice(), heights: Object.assign({}, h._wbHistoryHeights), pending: h._wbHistoryPending && Object.assign({}, h._wbHistoryPending),
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
      // Discard default cuts made by init on the incoming tail. A new log
      // shifts that cut by one, and it must not leave a closed sub-run inside
      // a previously open saved run. Saved cuts/states are recreated below.
      Array.from(h.querySelectorAll('.run-fold, .system-run')).reverse().forEach(function (run) {
        if (!run.parentElement) return;
        var siblings = Array.from(run.parentElement.children), prev = siblings[siblings.indexOf(run) - 1];
        if (prev && ((run.matches('.run-fold') && prev.matches('.run-fold')) || (run.matches('.system-run') && prev.matches('.system-run')))) mergeSystems(prev, run);
      });
      // Recreate private log cuts before restoring independently toggled
      // disclosures; renderer pages naturally contain the unsplit runs.
      (saved.folds || []).forEach(function (f) {
        var match = /^(?:run|system):(e[1-9][0-9]*)$/.exec(f[0]), entry = match && find(h, match[1]);
        if (entry) splitAt(h, entry);
      });
      var folds = new Map(saved.folds || []);
      h.querySelectorAll('details[data-key]').forEach(function (d) {
        if (folds.has(d.getAttribute('data-key'))) d.open = folds.get(d.getAttribute('data-key'));
      });
      var systems = new Map(saved.systems || []);
      h.querySelectorAll('.system-run').forEach(function (run) {
        var entry = Array.from(run.querySelectorAll('.wb-entry')).find(function (e) { return systems.has(e.id); });
        if (entry) run.open = systems.get(entry.id);
      });
      h._wbHistoryHeights = Object.assign({}, saved.heights || {});
      if (saved.chunks && saved.chunks.length > 1) {
        h._wbHistoryChunks = saved.chunks.slice();
        h._wbHistorySteps = saved.chunks.length;
        h._wbHistoryPending = saved.pending || null;
        saved.chunks.forEach(function (id) { if (id) materialize(h, id); });
        fit(h);
      } else reset(h);
      updateControl(h);
      arm(h);
    });
  }
  function init(root) {
    histories(root).forEach(function (h) {
      if (h._wbHistoryBound) { updateControl(h); return; }
      h._wbHistoryBound = true;
      h._wbHistoryHeights = {};
      reset(h);
      updateControl(h);
      arm(h);
      // Hidden overflow is not keyboard navigation: a clipped summary is
      // still tabbable. Reveal focused controls through the same page keep
      // path as deep links, never by scrolling the clipped box.
      h.addEventListener('focusin', function (e) {
        var live = h.querySelector('.history-live'), node = e.target;
        if (!live.contains(node)) return;
        var r = node.getBoundingClientRect(), clip = h.querySelector('.history-clip').getBoundingClientRect();
        if (r.top < clip.top || r.bottom > clip.bottom) navigate(h, node);
      });
      // Frame grafts keep this container but replace its older-page button.
      h.addEventListener('click', function (e) {
        var button = e.target.closest && e.target.closest('.history-older');
        if (!button || button.closest('.wb-history') !== h) return;
        if (h._wbHistoryPending || button.hidden) return;
        var anchor = oldest(h), live = h.querySelector('.history-live');
        var kept = anchor && kit.keep(anchor, { observe: [live] });
        h._wbHistoryTarget = null;
        var before = boundary(h);
        h._wbHistoryChunks.push(before);
        h._wbHistorySteps = h._wbHistoryChunks.length;
        h._wbHistoryPending = { before: before };
        arm(h);
        choose(h); fit(h);
        updateControl(h);
        if (kept) kept.apply();
      });
      if (typeof ResizeObserver !== 'undefined') {
        var observer = new ResizeObserver(function () {
          if (h.isConnected === false) { observer.disconnect(); return; }
          h._wbHistoryResizing = true;
          try {
            // Changing the clip during resize can resize shallower watched
            // ancestors. Apply it after this delivery, not inside it.
            later(h);
            updateControl(h);
          } finally { h._wbHistoryResizing = false; }
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
  // jump(node, {block, behavior}) gives an explicit navigation priority over
  // keeps until 150 ms / five stable frames, at most 2 s. Scroll intent wins;
  // ordinary keep policy is unchanged once the jump session ends.
  function viewKit(win) {
    var doc = win.document, observers = {}, reports = new WeakMap(), sessions = new Map();
    var QUIET = 500, CAP = 5000, IO = win.IntersectionObserver, bands = !!(IO && IO.prototype && "scrollMargin" in IO.prototype);
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
      if (s && s.follow) {
        if (opt.onEnd) s.ends.push(opt.onEnd);
        return s.handle;
      }
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
    function jump(node, opt) {
      var scroller = scrollerOf(node), s = sessions.get(scroller) || start(scroller);
      s.jump(node, opt || {});
      return s.handle;
    }
    function start(scroller) {
      var s = { node: null, offset: 0, ends: [], ro: null, quiet: 0, cap: 0, done: false, wrote: scroller.scrollTop };
      var main = scroller === docScroller(), anchorEl = main ? doc.documentElement : scroller, target = main ? win : scroller;
      var was = anchorEl.style.overflowAnchor;
      anchorEl.style.overflowAnchor = "none";
      function apply() {
        if (s.done) return;
        if (s.follow) { s.follow.align(); return; }
        if (!s.node.isConnected) { end("ended"); return; }
        var d = layoutTop(s.node, scroller) - scrollerTop(scroller) - s.offset;
        if (Math.abs(d) >= 0.5) scroller.scrollTop += d;
        s.wrote = scroller.scrollTop;
      }
      function end(reason) {
        if (s.done) return;
        s.done = true;
        if (s.follow) s.follow.off();
        sessions.delete(scroller);
        if (s.ro) s.ro.disconnect();
        win.clearTimeout(s.quiet);
        win.clearTimeout(s.cap);
        target.removeEventListener("scroll", onScroll);
        anchorEl.style.overflowAnchor = was;
        s.ends.forEach(function (fn) { fn(reason); });
      }
      function onScroll() {
        if (s.done || s.follow || Math.abs(scroller.scrollTop - s.wrote) <= 1) return;
        // Content that shrank under the scroll clamps it: not the user's.
        var max = scroller.scrollHeight - scroller.clientHeight;
        if (s.wrote > max + 1 && Math.abs(scroller.scrollTop - max) <= 1) { apply(); return; }
        end("user");
      }
      s.restart = function (both) {
        if (s.follow) return;
        win.clearTimeout(s.quiet);
        s.quiet = win.setTimeout(function () { end("quiet"); }, QUIET);
        if (!both) return;
        win.clearTimeout(s.cap);
        s.cap = win.setTimeout(function () { end("cap"); }, CAP);
      };
      s.jump = function (node, opt) {
        if (s.follow) s.follow.off();
        win.clearTimeout(s.quiet);
        win.clearTimeout(s.cap);
        var block = opt.block || "center", raf = 0, cap = 0, stable = 0, quiet = 0, last, position;
        var intents = ["wheel", "touchstart", "touchmove", "pointerdown"];
        function cancel() {
          // Stop an initial native smooth scroll too; pointer/touch intent
          // need not itself produce a scroll event that would interrupt it.
          scroller.scrollTo({ top: scroller.scrollTop, left: scroller.scrollLeft, behavior: "instant" });
          end("user");
        }
        function key(e) { if (/^(ArrowUp|ArrowDown|ArrowLeft|ArrowRight|PageUp|PageDown|Home|End| |Spacebar)$/.test(e.key)) cancel(); }
        function off() {
          win.cancelAnimationFrame(raf);
          win.clearTimeout(cap);
          intents.forEach(function (t) { target.removeEventListener(t, cancel, true); });
          win.removeEventListener("keydown", key, true);
        }
        function measure() {
          var r = node.getBoundingClientRect();
          return r.top - scrollerTop(scroller) + scroller.scrollTop - (block === "center" ? (scroller.clientHeight - r.height) / 2 : 0);
        }
        function align() {
          // A board morph may replace the item but retain its stable ID.
          if (!node.isConnected && node.id) node = doc.getElementById(node.id) || node;
          if (!node.isConnected || scrollerOf(node) !== scroller) { end("ended"); return; }
          var next = measure();
          // Smooth scrolling changes the viewport rect, not this content-space
          // destination. Only layout movement interrupts it with an instant fix.
          if (Math.abs(next - position) >= 0.5) {
            node.scrollIntoView({ block: block, behavior: "instant" });
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
        s.follow = { align: align, off: off };
        position = measure();
        intents.forEach(function (t) { target.addEventListener(t, cancel, { passive: true, capture: true }); });
        win.addEventListener("keydown", key, true);
        var reduced = win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches;
        node.scrollIntoView({ block: block, behavior: reduced ? "instant" : opt.behavior || "instant" });
        cap = win.setTimeout(function () { end("cap"); }, 2000);
        raf = win.requestAnimationFrame(frame);
      };
      // A swap settling on another anchor must not end explicit navigation.
      s.handle = { scroller: scroller, apply: apply, end: function () { if (!s.follow) end("ended"); } };
      if (win.ResizeObserver) {
        s.ro = new win.ResizeObserver(function () { if (s.done) return; apply(); s.restart(false); });
        (main ? [doc.body] : Array.prototype.slice.call(scroller.children)).forEach(function (el) { if (el) s.ro.observe(el); });
      }
      target.addEventListener("scroll", onScroll, { passive: true });
      sessions.set(scroller, s);
      return s;
    }
    function keeping(scroller) { var s = sessions.get(scroller); return s ? s.handle : null; }
    return { watch: watch, unwatch: unwatch, sweep: sweep, seen: seen, keep: keep, keeping: keeping, jump: jump, bands: bands };
  }
  var kit = viewKit(window);
  window.wbHistory = { init: init, capture: capture, restore: restore, loadTarget: loadTarget,
    watch: kit.watch, unwatch: kit.unwatch, sweep: kit.sweep, seen: kit.seen, keep: kit.keep, keeping: kit.keeping, jump: kit.jump, bands: kit.bands };
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
  document.addEventListener('focusout', function () {
    window.setTimeout(function () { histories(document).forEach(collapse); }, 0);
  });

  // The standalone file page navigates its own fragments. The dashboard has
  // its own allowlisted bridge, scroll policy, and region-swap lifecycle.
  if (document.body && document.body.classList.contains('full')) {
    function jump(id) {
      var node = loadTarget(document, id);
      if (node && !node.closest('.wb-history')) node.scrollIntoView({ block: 'nearest' });
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
