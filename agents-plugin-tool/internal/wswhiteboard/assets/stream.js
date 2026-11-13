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
  function position(h) {
    var rect = h.getBoundingClientRect(), anchor = null;
    Array.from(h.querySelectorAll('[data-history-anchor]')).some(function (node) {
      if (node.classList.contains('system-run') && node.open) return false;
      var r = node.getBoundingClientRect();
      if (r.height && r.bottom > rect.top && r.top < rect.bottom) {
        anchor = { id: node.getAttribute('data-history-anchor'), run: node.classList.contains('system-run'), offset: r.top - rect.top };
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
      var node = a && (a.run ? h.querySelector('[data-key="system:' + a.id + '"]') : find(h, a.id));
      if (!node && a && a.run) {
        var event = find(h, a.id);
        node = event && event.closest('.system-run');
      }
      if (node && node.getBoundingClientRect().height) {
        h.scrollTop += node.getBoundingClientRect().top - h.getBoundingClientRect().top - a.offset;
      } else h.scrollTop = state.top;
    }
    h._wbPosition = position(h);
  }
  function loadOne(h) {
    var page = h.querySelector('template[data-history-page]');
    if (!page) return false;
    var live = h.querySelector('.history-live');
    live.insertBefore(page.content.cloneNode(true), live.firstChild);
    page.remove();
    // A page boundary is not a semantic boundary. Keep the existing run's
    // disclosure node (and open state), moving older events into it in order.
    Array.from(live.children).forEach(function (run) {
      var next = run.nextElementSibling;
      if (!run.classList.contains('system-run') || !next || !next.classList.contains('system-run')) return;
      Array.from(run.children).slice(1).reverse().forEach(function (entry) { next.insertBefore(entry, next.children[1] || null); });
      run.remove();
      var events = next.querySelectorAll('.wb-entry');
      next.setAttribute('data-key', 'system:' + events[0].id);
      next.setAttribute('data-history-anchor', events[0].id);
      next.children[0].textContent = 'system events ' + events.length;
    });
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
    if (node.matches('.wb-entry.panel')) {
      var panel = node.querySelector('.entry-panel');
      if (panel) panel.open = true;
    }
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
  window.wbHistory = { init: init, capture: capture, restore: restore, loadTarget: loadTarget };
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
