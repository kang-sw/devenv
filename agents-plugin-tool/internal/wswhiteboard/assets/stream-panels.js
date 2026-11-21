// Panel fold state, clocks, link menus, and history bindings.
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
  window.wbHistory = { init: init, capture: capture, prepare: prepare, restore: restore, loadTarget: loadTarget, navigate: navigate, resolveTarget: resolveTarget, loading: loading, hydrateAll: hydrateAll, refreshImages: refreshImages,
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
    var mentionButton = t.closest && t.closest('button.pfmention');
    if (mentionButton) {
      e.preventDefault(); e.stopPropagation();
      var article = mentionButton.closest('.wb-entry[data-panel][data-entry]');
      var topic = article && article.closest('section.topic[data-topic]');
      var composer = window.WsComposer;
      if (topic && composer && typeof composer.addPanelMention === 'function') {
        composer.addPanelMention({ topic: topic.getAttribute('data-topic'), panel: article.getAttribute('data-panel'),
          entry: article.getAttribute('data-entry'), recap: (article.querySelector('.entry-recap') || {}).textContent || '' });
      }
      return;
    }
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
  refreshImages(document);
  // A fold that focus held back runs once focus leaves (collapse returns at
  // once for cards still in view or with nothing loaded).
  document.addEventListener('focusout', function () {
    window.setTimeout(function () { histories(document).forEach(collapse); }, 0);
  });
})();
