// Interleaved history admission, lazy hydration, and image refresh.
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
    copy.removeAttribute('data-lazy');
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
  // Lazy history. The served dashboard board leaves gap payloads, earlier
  // panel bodies and capped closed topics out, each marked data-lazy: an
  // empty gap template, an empty .entry-body, an empty deferred topic
  // template. Its host defines window.wbHistoryFetch(query), which resolves
  // to the complete sections [{name, html}] of the topics a "name=..." or
  // "target=..." query names. hydrateWith fills the marked parts in place
  // from them, so every synchronous path here runs unchanged on the result;
  // a section rendered from another revision fills only what still matches.
  function lazy(n) { return !!(n && n.hasAttribute && n.hasAttribute('data-lazy')); }
  function lazyIn(root) { return lazy(root) || !!(root.querySelector && root.querySelector('[data-lazy]')); }
  var fetching = new Map(), fetchFailures = 0;
  function fetchTopics(query) {
    if (typeof window.wbHistoryFetch !== 'function') return Promise.resolve([]);
    if (!fetching.has(query)) {
      var requestedImages = imageStamp;
      fetching.set(query, Promise.resolve().then(function () { return window.wbHistoryFetch(query); }).then(function (topics) {
        fetching.delete(query);
        // Any query (target=, repeated name=, or prefetch) can straddle an
        // image edit. Do not let its old response hydrate after that edit.
        if (requestedImages !== imageStamp) return fetchTopics(query);
        return Array.isArray(topics) ? topics : [];
      }, function () { fetching.delete(query); fetchFailures++; return []; }));
    }
    return fetching.get(query);
  }
  function named(list, attr, name) { return Array.from(list).filter(function (n) { return n.getAttribute(attr) === name; }); }
  function fillTopic(section, full) {
    var filled = 0;
    var lazyGaps = Array.from(section.querySelectorAll('.history-gap[data-lazy]')), fullGaps = Array.from(full.querySelectorAll('.history-gap'));
    var same = lazyGaps.length === fullGaps.length && lazyGaps.every(function (g, i) {
      return ['data-count', 'data-from', 'data-to'].every(function (a) { return g.getAttribute(a) === fullGaps[i].getAttribute(a); });
    });
    if (same) lazyGaps.forEach(function (g, i) {
      var held = payload(g), rows = payload(fullGaps[i]);
      while (rows.firstChild) held.appendChild(rows.firstChild);
      g.removeAttribute('data-lazy');
      filled++;
    });
    var entries = Array.from(full.querySelectorAll('.wb-entry[data-entry]'));
    section.querySelectorAll('.entry-body[data-lazy]').forEach(function (body) {
      var entry = body.closest('.wb-entry'), id = entry && entry.getAttribute('data-entry');
      var match = id && named(entries, 'data-entry', id)[0], src = match && match.querySelector('.entry-panel > .entry-body');
      if (!src || lazy(src)) return;
      while (src.firstChild) body.appendChild(src.firstChild);
      body.removeAttribute('data-lazy');
      filled++;
    });
    return filled;
  }
  function hydrateWith(root, topics) {
    var filled = 0;
    topics.forEach(function (t) {
      if (!t || typeof t.name !== 'string' || typeof t.html !== 'string') return;
      var parsed = document.createElement('template');
      parsed.innerHTML = t.html;
      var full = parsed.content.querySelector('section.topic');
      if (!full) return;
      var sections = named(root.querySelectorAll('section.topic[data-topic]'), 'data-topic', t.name);
      if (root.matches && root.matches('section.topic') && root.getAttribute('data-topic') === t.name) sections.unshift(root);
      sections.forEach(function (section) {
        if (!lazyIn(section)) return;
        var n = fillTopic(section, full);
        if (!n) return;
        histories(section).forEach(admitted);
        filled += n;
      });
      named(root.querySelectorAll('template[data-history-topic][data-lazy]'), 'data-history-topic', t.name).forEach(function (deferred) {
        deferred.content.appendChild(document.importNode(full, true));
        deferred.removeAttribute('data-lazy');
        filled++;
      });
    });
    return filled;
  }
  function hydrateTopic(section) {
    if (!section || !lazyIn(section)) return Promise.resolve(0);
    return fetchTopics('name=' + encodeURIComponent(section.getAttribute('data-topic') || '')).then(function (topics) {
      return section.isConnected === false ? 0 : hydrateWith(section, topics);
    });
  }
  // targets holds the lazy loadTarget misses in flight by id, one per root
  // asked (a jump tries every board box). loading(id), asked right after
  // loadTarget returned null, is the promise of those loads (true once the
  // target's topic arrived in some root, null otherwise) and hands the
  // landing to the caller, which then runs its own jump again; unasked, a
  // load lands by itself.
  var targets = new Map();
  function loading(id) {
    var list = targets.get(id);
    if (!list) return null;
    list.forEach(function (t) { t.claimed = true; });
    return Promise.all(list.map(function (t) { return t.promise; })).then(function (r) { return r.some(Boolean) || null; });
  }
  function hydrateTarget(root, id) {
    return fetchTopics('target=' + encodeURIComponent(id)).then(function (topics) {
      return root.isConnected === false ? 0 : hydrateWith(root, topics);
    });
  }
  // hydrateAll fills every lazy topic under root in one fetch (the command
  // palette's complete entry search).
  function hydrateAll(root) {
    root = root || document;
    var names = new Set();
    Array.from(root.querySelectorAll('section.topic[data-topic]')).forEach(function (t) { if (lazyIn(t)) names.add(t.getAttribute('data-topic')); });
    Array.from(root.querySelectorAll('template[data-history-topic][data-lazy]')).forEach(function (t) { names.add(t.getAttribute('data-history-topic')); });
    if (!names.size) return Promise.resolve(0);
    // The route answers at most 512 names a request.
    var list = Array.from(names), queries = [];
    for (var i = 0; i < list.length; i += 256) queries.push(list.slice(i, i + 256).map(function (n) { return 'name=' + encodeURIComponent(n); }).join('&'));
    return Promise.all(queries.map(fetchTopics)).then(function (all) { return hydrateWith(root, [].concat.apply([], all)); });
  }
  // A lazy board's deterministic file-image stamp changes even when its
  // retained topic sections do not. Refresh only hydrated image frames, not
  // history membership or disclosures; unchanged frames keep their identity.
  var imageStamp = null, imageRefreshing = null, imageRefreshed = null, imageRetry = 0;
  // querySelectorAll does not enter template.content. Retained gaps and
  // hydrated deferred topics (including their gaps) need the same refresh
  // as live rows, without materializing or binding their inert contents.
  function retainedImageNodes(root, selector) {
    var nodes = Array.from(root.querySelectorAll(selector));
    root.querySelectorAll('template[data-history-gap], template[data-history-topic]').forEach(function (t) {
      nodes = nodes.concat(retainedImageNodes(t.content, selector));
    });
    return nodes;
  }
  function refreshImageTopics(root) {
    var sections = retainedImageNodes(root, 'section.topic[data-topic]').filter(function (s) { return retainedImageNodes(s, '.doc-frame[data-relay], img[src]').length > 0; });
    var names = Array.from(new Set(sections.map(function (s) { return s.getAttribute('data-topic'); })));
    var queries = [];
    for (var i = 0; i < names.length; i += 256) queries.push(names.slice(i, i + 256).map(function (n) { return 'name=' + encodeURIComponent(n); }).join('&'));
    return Promise.all(queries.map(fetchTopics)).then(function (all) {
      [].concat.apply([], all).forEach(function (t) {
        if (!t || typeof t.name !== 'string' || typeof t.html !== 'string') return;
        var parsed = document.createElement('template'); parsed.innerHTML = t.html;
        var full = parsed.content.querySelector('section.topic');
        if (!full) return;
        var frames = retainedImageNodes(full, '.doc-frame[data-doc-key]'), images = new Map();
        function imagePath(src) { return /^\/s\/[^/?]+\/image\?path=/.test(src || '') ? src.replace(/&v=[0-9a-z.]+$/, '') : ''; }
        retainedImageNodes(full, 'img[src]').forEach(function (img) {
          var src = img.getAttribute('src'), path = imagePath(src);
          if (path) images.set(path, src);
        });
        // Resolve destinations again: a deferred topic may have materialized
        // (or disappeared) while the fetch was in flight. Its inert section
        // has isConnected=false even while its owning template is retained.
        retainedImageNodes(root, 'section.topic[data-topic]').filter(function (s) { return s.getAttribute('data-topic') === t.name; }).forEach(function (s) {
          var changed = false, kept = s.isConnected !== false ? kit.keep(s) : null;
          retainedImageNodes(s, '.doc-frame[data-relay]').forEach(function (old) {
            var fresh = frames.find(function (f) { return f.getAttribute('data-doc-key') === old.getAttribute('data-doc-key'); });
            if (!fresh || fresh.getAttribute('data-srcdoc') === old.getAttribute('data-srcdoc') && fresh.getAttribute('data-relay') === old.getAttribute('data-relay')) return;
            var copy = fresh.cloneNode(true);
            copy.style.height = old.style.height; copy.wbKeep = true;
            old.parentNode.insertBefore(copy, old); old.remove(); changed = true;
          });
          // Item/history images outside body frames keep their node and
          // disclosures; only their file-version URL changes.
          retainedImageNodes(s, 'img[src]').forEach(function (img) {
            var src = img.getAttribute('src'), fresh = images.get(imagePath(src));
            if (fresh && fresh !== src) { img.setAttribute('src', fresh); changed = true; }
          });
          if (changed && kept) admitted(s);
          if (kept) kept.apply();
        });
      });
    });
  }
  function refreshImages(root) {
    root = root || document;
    var marker = root.querySelector('template[data-history-images]'), stamp = marker && marker.getAttribute('data-history-images');
    if (stamp === null || stamp === imageRefreshed) return imageRefreshing || Promise.resolve();
    if (imageStamp === null) { imageStamp = imageRefreshed = stamp; return Promise.resolve(); }
    imageStamp = stamp;
    // Serialize refreshes: if another edit arrives during a fetch, run once
    // more after it settles rather than joining an older name= response.
    if (imageRefreshing) return imageRefreshing;
    function update() {
      var requested = imageStamp, failures = fetchFailures;
      return refreshImageTopics(root).then(function () {
        if (fetchFailures !== failures) {
          if (!imageRetry) imageRetry = window.setTimeout(function () { imageRetry = 0; refreshImages(root); }, 2000);
          return;
        }
        if (imageStamp !== requested) return update();
        imageRefreshed = requested;
      });
    }
    imageRefreshing = update().then(function () { imageRefreshing = null; });
    return imageRefreshing;
  }
  // unfilled holds, by history key, the snapshot a rehydrate is still
  // fetching for: a capture taken meanwhile (another update) merges it, so
  // rows the reader had loaded survive a second swap before the fill.
  var unfilled = new Map();
  function rehydrate(root, saved) {
    var scope = root || document;
    unfilled.set(saved.key, saved);
    fetchTopics('name=' + encodeURIComponent(saved.key)).then(function (topics) {
      if (unfilled.get(saved.key) === saved) unfilled.delete(saved.key);
      if (scope.isConnected === false) return;
      // The first restore already ran this snapshot's pending loads.
      if (hydrateWith(scope, topics)) restore(scope, [Object.assign({}, saved, { pending: [] })]);
    });
  }
  function carried(record, h) {
    var prev = unfilled.get(record.key);
    if (!prev || !h.querySelector('.history-gap[data-lazy]')) return record;
    var union = function (a, b) { return Array.from(new Set((a || []).concat(b || []))); };
    var folds = new Map(prev.folds || []);
    (record.folds || []).forEach(function (f) { folds.set(f[0], f[1]); });
    return Object.assign({}, record, { shown: record.shown || prev.shown, rows: union(prev.rows, record.rows), entries: union(prev.entries, record.entries),
      loaded: union(prev.loaded, record.loaded), folds: Array.from(folds) });
  }
  // An earlier panel version's body arrives with its topic when its fold
  // opens, by the reader or a restored fold; its frame mounts on arrival
  // (admitted's wbhistoryload).
  document.addEventListener('toggle', function (e) {
    var d = e.target;
    if (!d || d.tagName !== 'DETAILS' || !d.open || !d.querySelector(':scope > .entry-body[data-lazy]')) return;
    hydrateTopic(d.closest('section.topic'));
  }, true);
