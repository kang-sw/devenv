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
  function valid(id) { return /^(?:(?:p|e)[1-9][0-9]*|item-[1-9][0-9]*)$/.test(id || ''); }
  function find(root, id) {
    if (!valid(id)) return null;
    var node = root.querySelector('[id="' + id + '"]');
    if (!node) return null;
    if (id[0] === 'e') return node.matches('.wb-entry[data-entry="' + id + '"]') ? node : null;
    if (id.indexOf('item-') === 0) return node.closest('.wb-entry[data-history-item="' + id.slice(5) + '"]') ? node : null;
    return node.matches('.entry-panel') && node.closest('.wb-entry[data-panel="' + id + '"]') ? node : null;
  }
  var LIMIT = 1000, PANELS = 3, LINES = 30;
  // A fold (a system-event chip) anchors by its first entry while closed;
  // once open, its entries anchor themselves.
  function isFold(node) { return node.classList.contains('system-run'); }
  function rows(h) {
    return Array.from(h.querySelector('.history-live').querySelectorAll('.wb-entry')).filter(function (e) {
      return !e.closest('.panel-versions');
    });
  }
  // Republishing moves an immutable boundary into Versions. Its eN still
  // defines the chronological cut, not the newer card that now contains it.
  // Entry IDs increase at append; compare decimal lengths before digits.
  // Undated item-N display anchors are not recorded events or successors.
  function newer(id, before) { return /^e[1-9][0-9]*$/.test(id) && (id.length > before.length || (id.length === before.length && id > before)); }
  function boundaryRow(h, id) {
    var node = id && find(h, id);
    if (!node || !node.closest('.panel-versions')) return node;
    return rows(h).find(function (e) { return newer(e.id, id); }) || null;
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
  // A row's measured top: a row behind a closed chip has no box of its own,
  // so the chip's top stands in for it.
  function topOf(h, node) {
    var live = h.querySelector('.history-live');
    for (var p = node.parentElement; p && p !== live; p = p.parentElement) {
      if (p.tagName === 'DETAILS' && !p.open) return topRow(h, node).getBoundingClientRect().top;
    }
    return node.getBoundingClientRect().top;
  }
  // countsFull: no inert page is left, or the row next to the loaded rows
  // (the newest held page's last row) is one the counts already refuse.
  // Peeking keeps that page's payloads out of the live DOM.
  function countsFull(h, panels, lines) {
    var page = h.querySelector('template[data-history-page]'), kids = page && page.content.children, last = kids && kids[kids.length - 1];
    if (!page) return true;
    return !!last && (last.matches('.wb-entry.panel') ? panels >= PANELS : lines >= LINES);
  }
  function boundary(h) { return h._wbHistoryChunks[h._wbHistoryChunks.length - 1]; }
  function fit(h) {
    var node = boundaryRow(h, boundary(h)), live = h.querySelector('.history-live');
    if (!node) return;
    node = splitAt(h, node);
    // Membership, not pixels: hide only whole top-level rows before the
    // saved boundary. splitAt preserves independently open log/system folds.
    // No height, overflow clip, fade, or partially visible oldest entry.
    var before = true;
    Array.from(live.children).forEach(function (row) {
      if (row === node) before = false;
      if (before && !row.hasAttribute('data-wb-history-before')) row.setAttribute('data-wb-history-before', '');
      if (!before && row.hasAttribute('data-wb-history-before')) row.removeAttribute('data-wb-history-before');
    });
    h._wbHistoryHeight = Math.ceil(live.getBoundingClientRect().height);
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
      // Measure candidates in ordinary flow, then fit hides the excluded
      // prefix synchronously before paint. Disclosure state is not changed.
      h.querySelectorAll('[data-wb-history-before]').forEach(function (row) { row.removeAttribute('data-wb-history-before'); });
      var all = rows(h), before = h._wbHistoryPending.before, node = before && boundaryRow(h, before);
      var end = before ? all.indexOf(node) : all.length;
      if (end < 0) {
        // Abandon only the unfinished Older chunk; keep finalized cuts so a
        // missing entry cannot lock out later clicks or resize retries.
        h._wbHistoryPending = null;
        if (h._wbHistoryChunks.length > 1) h._wbHistoryChunks.pop();
        h._wbHistorySteps = h._wbHistoryChunks.length;
        return;
      }
      // Walk back from the cut one row at a time and stop just before a row
      // would pass any budget: LINES log lines, PANELS panels, LIMIT pixels.
      // The first row always joins, so an oversized single row still shows.
      var older = all.slice(0, end), first = null, lines = 0, panels = 0, adjust = 0, full = false, provisional = false;
      var tail = older.length && (before ? splitAt(h, node).getBoundingClientRect().top : h.querySelector('.history-live').getBoundingClientRect().bottom);
      for (var i = older.length - 1; i >= 0; i--) {
        var row = older[i], panel = row.classList.contains('panel');
        if (first && (panel ? panels + 1 > PANELS : lines + 1 > LINES)) { full = true; break; }
        var known = panel ? panelHeight(h, row) : 0;
        // An unrendered panel cannot be judged yet: from there on only the
        // counts apply, and the rows they admit show provisionally so they
        // can render (hidden topics too) before the pixels decide.
        if (known === null) provisional = true;
        if (!provisional) {
          // Geometry supplies logs, gaps and system chips. Replace only
          // panel boxes with cached heights for the one-time judgment.
          var next = adjust + (panel ? known - row.getBoundingClientRect().height : 0);
          if (first && tail - topOf(h, row) + next > LIMIT) { full = true; break; }
          adjust = next;
        }
        first = row;
        if (panel) panels++; else lines++;
      }
      // Rows ran out before a budget did: materialize one more page per
      // frame, unless its adjacent row could not join anyway.
      if (!full && !countsFull(h, panels, lines)) { loadOne(h); later(h); return; }
      if (!first) { h._wbHistoryPending = null; return; }
      if (!provisional) h._wbHistoryPending = null;
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
    var more = !!h.querySelector('template[data-history-page]') || (!!boundary(h) && all[0] && all[0] !== boundaryRow(h, boundary(h)));
    var button = h.querySelector('.history-older');
    if (!button || button.hidden === !more) return;
    // The header row keeps its height without the capsule, but toggling it
    // still mutates watched ancestors. Do that outside this history's resize
    // delivery, including reentrant init calls from loadOne -> wbBindBoard.
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
    button.hidden = !more;
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
    // A closed system summary has no entry ancestor. Its existing anchor
    // names the first entry represented by that disclosure.
    var fold = !entry && node.closest('.system-run');
    if (fold) entry = find(h, fold.getAttribute('data-history-anchor'));
    if (entry) entry = topRow(h, entry).closest('.wb-entry') || entry;
    if (entry && (!id || all.indexOf(entry) < all.indexOf(boundaryRow(h, id)))) {
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
    // Clear both independent obstructions: the sticky head/rail and the
    // open-question pill below that head. Merely landing within history
    // bounds does not expose a control's center (including a short summary).
    var topic = h.closest('.topic'), head = topic.querySelector('.tophead'), offset = 12;
    var style = head && window.getComputedStyle(head);
    var sticky = style && style.position === 'sticky';
    if (sticky) offset += (parseFloat(style.top) || 0) + head.offsetHeight;
    var host = topic.closest('#bpins') || topic.closest('#bcol') || topic.closest('.wrap');
    var wrap = host && host.querySelector('.oq-wrap.wb-oq-top'), pill = wrap && wrap.querySelector('.oq-pill');
    if (pill && !pill.hidden) {
      // The 10px gap mirrors page.css. Predict this topic's pill position
      // as well as reading the current one: the rail changes its current
      // head during the scroll, so the old pill's geometry alone can lie.
      var pillTop = parseFloat(window.getComputedStyle(wrap).top) || 0;
      offset = Math.max(offset + (sticky ? 10 + pill.offsetHeight : 0), pillTop + pill.offsetHeight + 12);
    }
    kit.keep(node, { offset: offset, observe: [h.querySelector('.history-live')], onEnd: function () {
      if (h._wbHistoryTarget === node) h._wbHistoryTarget = null;
    } });
  }
  // relabel rewrites a chip's key, anchor, and "n entries · from-to" label
  // after a merge or split, as the server writes them (EntryPage.Segments).
  function relabel(fold) {
    var events = Array.from(fold.querySelectorAll('.wb-entry'));
    var clock = function (e) { var t = e.querySelector('time.entry-at'); return t ? t.textContent : ''; };
    var from = clock(events[0]), to = clock(events[events.length - 1]), range = from === to ? from : from + '–' + to;
    fold.setAttribute('data-key', 'system:' + events[0].id);
    fold.setAttribute('data-history-anchor', events[0].id);
    var summary = fold.children[0], label = summary.querySelector('.lbl') || summary;
    label.textContent = window.wsT ? window.wsT('js.wb.stream.system', events.length, range) : 'system events ' + events.length;
  }
  // mergeSystems moves an older chip's events to the front of the adjacent,
  // newer chip, which keeps its disclosure node and open state.
  function mergeSystems(older, newer) {
    Array.from(older.children).slice(1).reverse().forEach(function (entry) { newer.insertBefore(entry, newer.children[1] || null); });
    older.remove();
    relabel(newer);
  }
  // A page boundary is not a semantic boundary: a system chip split by it
  // merges into the newer side's chip.
  function joinBoundary(live, boundary) {
    var kids = Array.from(live.children), end = kids[kids.indexOf(boundary) - 1];
    if (end && end.matches('details.system-run') && boundary.matches('details.system-run')) mergeSystems(end, boundary);
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
  // Navigation's optional callback reveals the owning live topic before
  // admission hooks or init can measure it. It may receive the topic or a
  // live descendant more than once. Snapshot restore has no callback.
  function materialize(root, id, beforeGeometry) {
    var node = find(root, id);
    if (node) { if (beforeGeometry) beforeGeometry(node); return node; }
    var deferred = Array.from(root.querySelectorAll('template[data-history-topic]')).find(function (t) {
      if (find(t.content, id)) return true;
      return Array.from(t.content.querySelectorAll('template[data-history-page]')).some(function (page) { return !!find(page.content, id); });
    });
    if (deferred) {
      var fragment = deferred.content.cloneNode(true), topic = fragment.querySelector('.topic');
      deferred.parentElement.insertBefore(fragment, deferred);
      if (beforeGeometry) beforeGeometry(topic);
      deferred.remove();
      init(topic);
      if (window.wbBindBoard) window.wbBindBoard(topic);
      topic.dispatchEvent(new CustomEvent('wbhistoryload', { bubbles: true, detail: { root: topic } }));
      return materialize(root, id, beforeGeometry);
    }
    var list = histories(root);
    for (var i = 0; i < list.length; i++) {
      var h = list[i];
      var held = Array.from(h.querySelectorAll('template[data-history-page]')).some(function (t) { return !!find(t.content, id); });
      if (!held) continue;
      if (beforeGeometry) beforeGeometry(h);
      while (!find(h, id) && loadOne(h)) {}
      return find(h, id);
    }
    return null;
  }
  function loadTarget(root, id, beforeGeometry) {
    if (!valid(id)) return null;
    var node = materialize(root || document, id, beforeGeometry);
    if (!node) return null;
    for (var p = node; p; p = p.parentElement) if (p.tagName === 'DETAILS') p.open = true;
    var mark = node;
    if (node.matches('.wb-entry.panel')) {
      var panel = node.querySelector('.entry-panel');
      if (panel) { panel.open = true; mark = panel; }
    }
    var openedPanel = node.matches('.entry-panel') ? node : node.matches('.wb-entry.panel') ? node.querySelector('.entry-panel') : null;
    if (panelState && openedPanel) panelState.opened(openedPanel);
    // Highlight the reached target once, restarting a running flash.
    mark.classList.toggle('flash', false); void mark.offsetWidth; mark.classList.toggle('flash', true);
    var h = node.closest('.wb-history');
    if (h) navigate(h, node);
    return node;
  }
  function capture(root) {
    return histories(root).map(function (h) {
      var first = h.querySelector('.wb-entry');
      return { key: h.getAttribute('data-history'), first: first && first.id, steps: h._wbHistorySteps || 1,
        chunks: h._wbHistoryChunks.slice(), heights: Object.assign({}, h._wbHistoryHeights), pending: h._wbHistoryPending && Object.assign({}, h._wbHistoryPending),
        panels: panelState ? panelState.capture(h) : [],
        folds: Array.from(h.querySelectorAll('details[data-key]')).map(function (d) { return [d.getAttribute('data-key'), d.open]; }),
        systems: Array.from(h.querySelectorAll('.system-run .wb-entry')).map(function (e) { return [e.id, e.closest('.system-run').open]; }) };
    });
  }
  function restore(root, snapshot) {
    // A navigated closed topic may be inert again in the replacement board.
    (snapshot || []).forEach(function (saved) { if (saved.first) materialize(root || document, saved.first); });
    if (panelState) histories(root).forEach(function (h) {
      var saved = (snapshot || []).find(function (s) { return s.key === h.getAttribute('data-history'); });
      if (saved) panelState.snapshot(h, saved);
    });
    init(root);
    histories(root).forEach(function (h) {
      var saved = (snapshot || []).find(function (s) { return s.key === h.getAttribute('data-history'); });
      if (!saved) return;
      // Old snapshots' whole-history open flag is obsolete. Descendant
      // disclosures below are still independently restored.
      if (saved.first) materialize(h, saved.first);
      if (saved.chunks && saved.chunks.length > 1) saved.chunks.forEach(function (id) {
        if (!id) return;
        var node = materialize(h, id), held = null;
        if (!node || !node.closest('.panel-versions')) return;
        // Finding the nested version need not load its old chronological
        // page. Restore every surviving row from that cut, even if the
        // saved first entry also moved into the newer card's Versions.
        h.querySelectorAll('template[data-history-page]').forEach(function (page) {
          page.content.querySelectorAll('.wb-entry').forEach(function (e) {
            if (!e.closest('.panel-versions') && newer(e.id, id) && (!held || newer(held.id, e.id))) held = e;
          });
        });
        if (held) materialize(h, held.id);
      });
      // Discard default cuts made by init on the incoming tail. Saved
      // cuts/states are recreated below.
      Array.from(h.querySelectorAll('.system-run')).reverse().forEach(function (run) {
        if (!run.parentElement) return;
        var siblings = Array.from(run.parentElement.children), prev = siblings[siblings.indexOf(run) - 1];
        if (prev && prev.matches('.system-run')) mergeSystems(prev, run);
      });
      // Recreate private chip cuts before restoring independently toggled
      // disclosures; renderer pages naturally contain the unsplit chips.
      (saved.folds || []).forEach(function (f) {
        var match = /^system:(e[1-9][0-9]*)$/.exec(f[0]), entry = match && find(h, match[1]);
        if (entry) splitAt(h, entry);
      });
      var folds = new Map(saved.folds || []);
      h.querySelectorAll('details[data-key]').forEach(function (d) {
        if (folds.has(d.getAttribute('data-key'))) d.open = folds.get(d.getAttribute('data-key'));
      });
      // Latest pN identity wins over an obsolete latest-version entry key.
      // Historical-version/item/system fold keys remain independent.
      if (panelState) h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore);
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
        fit(h);
      } else reset(h);
      updateControl(h);
      arm(h);
    });
  }
  function init(root) {
    histories(root).forEach(function (h) {
      if (panelState) h.querySelectorAll('.entry-panel[data-key]').forEach(panelState.restore);
      if (h._wbHistoryBound) { updateControl(h); return; }
      h._wbHistoryBound = true;
      h._wbHistoryHeights = {};
      reset(h);
      updateControl(h);
      arm(h);
      // Excluded whole rows are not tab stops. For a selected control,
      // native Tab must also clear sticky overlays, not just history bounds.
      // Deep links reveal excluded entries through the same keep path.
      h.addEventListener('focusin', function (e) {
        var live = h.querySelector('.history-live'), node = e.target;
        if (!live.contains(node)) return;
        var r = node.getBoundingClientRect(), clip = h.querySelector('.history-clip').getBoundingClientRect();
        var hit = document.elementFromPoint && document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        if (r.top < clip.top || r.bottom > clip.bottom || (hit && hit !== node && !node.contains(hit))) navigate(h, node);
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
            // Changing membership during resize can resize shallower
            // watched ancestors. Apply it after this delivery, not inside it.
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
  window.wbHistory = { init: init, capture: capture, restore: restore, loadTarget: loadTarget,
    restorePanel: function (p) { return !!panelState && panelState.restore(p); },
    panelToggle: function (p) { return !!panelState && panelState.toggle(p); },
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
