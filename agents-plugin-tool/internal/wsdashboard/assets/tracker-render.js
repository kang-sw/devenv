  // ------------------------------------------------------------------ render

  // The handle row's width (plus the 6px gap to the host's handle) and the header's bottom
  // edge, where the drawer hangs. The width first: the host header pads clear of it, which
  // can change how the header wraps and so where it ends.
  function measure() {
    var w = Math.ceil($('tkhandles').getBoundingClientRect().width);
    document.documentElement.style.setProperty('--tkhw', (w ? w + 6 : 0) + 'px');
    $('tkdrawer').style.setProperty('--tktop', Math.max(0, Math.round(O.header.getBoundingClientRect().bottom)) + 'px');
  }
  // Every store fills the available space below the header. A host can supply its
  // maximum (including composer clearance); standalone pages use the viewport.
  function fitDrawer() {
    var h = O.drawerHeight ? O.drawerHeight() : window.innerHeight - Math.max(0, Math.round(O.header.getBoundingClientRect().bottom));
    $('tkdrawer').style.setProperty('--tkh', Math.max(0, h) + 'px');
  }

  // One handle per store, stacking leftward from the host's handle in stores() order, then
  // (with adapter.register) the "+ store" handle, also with no store at all; the row is
  // rebuilt when the store list changes. Each store's handle sits in a slot with its fold
  // button (tkf-<store>), which fills the slot while that store's drawer is open
  // (the handle stays hidden without changing its width) and closes the drawer.
  var handleSig = null;
  function syncHandles() {
    order = (AD.stores() || []).slice();
    var sig = order.join('\n');
    if (sig === handleSig) return;
    handleSig = sig;
    $('tkhandles').innerHTML = order.map(function (n) {
      return '<span class="tk-slot"><button type="button" class="tk-fold" id="tkf-' + esc(n) + '"' + attr('fold', n) + ' aria-controls="tkbody" title="' + esc(wsT("js.tk.handle.fold")) +
        '" aria-label="' + esc(wsT("js.tk.handle.fold")) + '" hidden><svg class="i"><use href="#tki-up"/></svg></button>' +
        '<button class="tk-handle" id="tkh-' + esc(n) + '"' + attr('handle', n) + ' aria-controls="tkbody"><svg class="i"><use href="#tki-tasks"/></svg><span class="hl">' + esc(n) +
        '</span><span class="hoff" title="' + wsT("js.tk.handle.off") + '" hidden>&#8856;</span><span class="tk-badge" hidden></span></button></span>';
    }).join('') + (AD.register ? '<button class="tk-handle tk-add" id="tkadd" data-act="register" aria-haspopup="dialog" title="' + esc(wsT("js.tk.reg.handle.tip")) + '" aria-label="' + esc(wsT("js.tk.reg.handle.tip")) + '"><svg class="i"><use href="#tki-plus"/></svg></button>' : '');
  }
  function renderHandles() {
    order.forEach(function (name) {
      var b = $('tkh-' + name), n = newKeys(name).length, badge = b.querySelector('.tk-badge'), vs = vss[name];
      b.className = 'tk-handle' + (ui.store === name ? ' on' : '') + (ui.open === name ? ' open' : '');
      b.setAttribute('aria-expanded', String(ui.open === name));
      $('tkf-' + name).hidden = !(ui.open === name && !ui.page);
      badge.hidden = !n; badge.textContent = n;
      if (!vs) { b.querySelector('.hoff').hidden = true; b.title = name + ' - ' + (AD.error(name) || wsT("js.tk.loading")); return; }
      b.querySelector('.hoff').hidden = !!(vs.store.remote && vs.sync.online);
      // A store with a display label shows the label, its id in the tooltip.
      b.querySelector('.hl').textContent = vs.store.label || name;
      b.title = storeTitle(vs.store) + (n ? wsT("js.tk.handle.new", n) : '') + (vs.store.remote ? (vs.sync.online ? '' : ' - ' + wsT("js.tk.sync.offline")) : ' - ' + wsT("js.tk.handle.noremote"));
    });
  }

  // The item window makes the rest of the page inert; the renderer remembers which of the
  // body's children it marked, so it releases only those.
  var inerted = [];
  function setInert(on) {
    if (!on) { inerted.forEach(function (el) { el.inert = false; }); inerted = []; return; }
    if (inerted.length) return;
    var keep = [$('tkwin'), $('tkdlg'), $('tkqpop'), $('tktoast'), $('wstoasts'), $('tktip'), $('tkmenu')];
    Array.prototype.forEach.call(document.body.children, function (el) {
      if (keep.indexOf(el) < 0 && !el.inert) { el.inert = true; inerted.push(el); }
    });
  }

  // dropEditorState forgets the previews and uploads of editors that are gone; open says
  // whether the item window shows.
  function dropEditorState(open) {
    // an editor's preview ends with the editor: an edit saved or cancelled, a comment posted, another item
    if (!ui.iedit) { delete ui.edpv['iw-ebody']; delete ui.eatt['iw-ebody']; }
    if (!ui.cedit) { delete ui.edpv.tkcedit; delete ui.eatt.tkcedit; }
    // the wiki editor's preview and uploads go when no store's wiki edit is open (another store's edit keeps them)
    if (!Object.keys(ui.wiki).some(function (k) { return !!ui.wiki[k].edit; })) { delete ui.edpv['wk-ebody']; delete ui.eatt['wk-ebody']; }
    if (!open || ui.itemChanged || ui.clearDraft) { delete ui.edpv.cbox; delete ui.eatt.cbox; }
  }
  // renderDue: a change notification asked for a render that has not run yet (see mount); any
  // render meets it, so a synchronous render() takes the pending one's place.
  var renderDue = false, changeTimer = 0, windowHold = null;
  // UNREADS: the rendered store's unread notifications as api.unread() returns them,
  // rebuilt by every render. afterRender runs the host's hooks once a render is done.
  var UNREADS = { store: null, ready: false, entries: [] };
  // Reconcile the keyed local model into existing DOM. Unchanged rows (and
  // their focus, scroll, editor and disclosure state) are never replaced.
  // Keys are sibling-scoped: the same item may appear in several panels.
  var trackerRowHTML = new WeakMap();
  function patchTrackerHTML(root, html) {
    var t = document.createElement('template'); t.innerHTML = html;
    function key(n) {
      if (n.nodeType !== 1) return '';
      var attrs = ['id', 'data-rk', 'data-nkey', 'data-rec', 'data-ev', 'data-kcol', 'data-tab'];
      for (var i = 0; i < attrs.length; i++) if (n.hasAttribute(attrs[i])) return n.nodeName + ':' + attrs[i] + ':' + n.getAttribute(attrs[i]);
      return '';
    }
    function compatible(a, b) { return a && a.nodeType === b.nodeType && a.nodeName === b.nodeName && key(a) === key(b); }
    function row(n) { return n.nodeType === 1 && (n.hasAttribute('data-rk') || n.hasAttribute('data-nkey')); }
    function remember(n) {
      if (n.nodeType !== 1) return;
      if (row(n)) trackerRowHTML.set(n, n.outerHTML);
      Array.prototype.forEach.call(n.querySelectorAll('[data-rk], [data-nkey]'), function (r) { trackerRowHTML.set(r, r.outerHTML); });
    }
    function children(a, b) {
      var keyed = new Map();
      Array.prototype.forEach.call(a.childNodes, function (n) { var k = key(n); if (k) keyed.set(k, n); });
      var cursor = a.firstChild;
      Array.prototype.forEach.call(b.childNodes, function (fresh) {
        var k = key(fresh), old = k ? keyed.get(k) : compatible(cursor, fresh) ? cursor : null;
        if (!compatible(old, fresh)) old = null;
        if (!old) { old = fresh.cloneNode(true); remember(old); a.insertBefore(old, cursor); }
        else {
          if (old !== cursor) a.insertBefore(old, cursor);
          // Compare canonical row markup, not the live DOM: positioning a
          // hover preview must not make an unchanged row look server-dirty.
          var rowHTML = row(fresh) ? fresh.outerHTML : null;
          if (rowHTML !== null && trackerRowHTML.get(old) === rowHTML) { cursor = old.nextSibling; return; }
          if (!old.isEqualNode(fresh)) {
            if (old.nodeType === 1) {
              Array.prototype.slice.call(old.attributes).forEach(function (x) { if (!fresh.hasAttribute(x.name)) old.removeAttribute(x.name); });
              Array.prototype.forEach.call(fresh.attributes, function (x) { if (old.getAttribute(x.name) !== x.value) old.setAttribute(x.name, x.value); });
              children(old, fresh);
            } else if (old.nodeValue !== fresh.nodeValue) old.nodeValue = fresh.nodeValue;
          }
          if (rowHTML !== null) trackerRowHTML.set(old, rowHTML);
          if (old.nodeType === 1 && /^(INPUT|TEXTAREA|SELECT)$/.test(old.nodeName) && old.value !== fresh.value) old.value = fresh.value;
        }
        cursor = old.nextSibling;
      });
      while (cursor) { var next = cursor.nextSibling; a.removeChild(cursor); cursor = next; }
    }
    children(root, t.content);
  }
  // IntersectionObserver is clipped by the drawer/card scrollports. Merely
  // mounting handles never loads bodies; visible rows batch on the adapter's
  // next tick. Existing observed rows need re-requesting after a version delta.
  var detailObserver = null, detailNodes = new Set(), visibleDetails = new Set();
  function observeDetails(body) {
    if (!AD.detail || !VS || !VS.projection) return;
    if (!detailObserver && typeof IntersectionObserver === 'function') detailObserver = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) { if (entry.isIntersecting) visibleDetails.add(entry.target); else visibleDetails.delete(entry.target); });
      requestVisibleDetails();
    });
    detailNodes.forEach(function (n) { if (!n.isConnected || !body.contains(n)) { if (detailObserver) detailObserver.unobserve(n); detailNodes.delete(n); visibleDetails.delete(n); } });
    Array.prototype.forEach.call(body.querySelectorAll('.irow[data-item], .kcard[data-item], .citem[data-item]'), function (n) {
      if (detailNodes.has(n)) return;
      detailNodes.add(n);
      if (detailObserver) detailObserver.observe(n);
      // Older browsers still fetch an explicitly expanded item's detail.
    });
    requestVisibleDetails();
  }
  function requestVisibleDetails() {
    if (!AD.detail || !VS || !VS.projection || !viewing()) return;
    var ids = {};
    visibleDetails.forEach(function (n) { if (n.isConnected) ids[n.getAttribute('data-item')] = true; });
    AD.detail(ui.store, Object.keys(ids));
  }
  function afterRender(name) {
    if (X.rendered) X.rendered(name, VS, H);
    if (O.onRender) O.onRender(UNREADS);
  }
  function render() {
    renderDue = false;
    syncHandles();
    order.forEach(function (n) { vss[n] = AD.view(n); });
    Object.keys(vss).forEach(function (n) { if (order.indexOf(n) < 0) delete vss[n]; });
    dropGone();
    var name = ui.store;
    VS = name ? vss[name] : null; EV = {};
    UNREADS = { store: name, ready: false, entries: [] };
    renderHandles();
    $('tkdrawer').classList.toggle('open', !!ui.open);

    // the store renders into the store page or the drawer, never both (the card ids are unique)
    var body = ui.page ? $('tkpage') : $('tkbody'), sb = $('tkwinbody'), top1 = body.scrollTop, top2 = sb.scrollTop;
    (ui.page ? $('tkbody') : $('tkpage')).innerHTML = '';
    $('tkpage').hidden = !ui.page; document.body.classList.toggle('tk-paged', !!ui.page);
    if (!VS) { // nothing to show yet: the store is loading, or its view could not be read
      hideTip();
      body.innerHTML = '<div class="tk-wait">' + esc(name ? AD.error(name) || wsT("js.tk.loading") : wsT("js.tk.nostore")) + '</div>';
      ui.win = false; $('tkwin').hidden = true; sb.innerHTML = ''; setInert(false);
      if (!ui.page) fitDrawer();
      afterRender(name);
      measure();
      wkFrameSync(null);
      return;
    }
    var s = viewerOf(name), v = memberOf(s);
    VS.events.forEach(function (e) { EV[e.id] = e; });
    if (ui.item && !VS.items[ui.item]) { ui.item = null; ui.win = false; }
    // per-render row marks: flagged writes in an item's own thread, unread notification entries per item
    FLAGGED = VS.itemFlags || {}; UNREAD = {};
    if (!VS.projection) VS.events.forEach(function (e) { if (e.status === 'flagged' && e.thread && VS.items[e.thread]) FLAGGED[e.thread] = (FLAGGED[e.thread] || 0) + 1; });
    var rm = readMap();
    UNREADS.ready = true;
    feedEntries(v).forEach(function (x) {
      if (rm[x.key]) return;
      UNREAD[x.item] = (UNREAD[x.item] || 0) + 1;
      var src = x.ev && (VS.comments[x.ev] || EV[x.ev]), who = x.author || src && (src.author || src.member);
      UNREADS.entries.push({ store: name, item: x.item, ev: x.ev, key: x.key, kind: x.kind, label: wsT(NK[x.kind]), title: VS.items[x.item].title || alias(x.item), author: who ? nm(who) : '' });
    });
    // With no drawer, store page, or item window open, nothing of the store shows but its
    // handle: the marks above and the host's rendered hook stay current, the drawer is left as
    // it is (the render that opens it draws it), and nothing is measured but the handle row.
    if (!ui.open && !ui.page && !ui.win) {
      hideTip();
      insPar(null); HV = null;
      $('tkwin').hidden = true; setInert(false);
      if (sb.firstChild) sb.innerHTML = '';
      dropEditorState(false);
      if (MN && MN.kind === 'sync') { $('tkmenu').innerHTML = syncPopHtml(); placeMenu(); }
      else if (MN && mnStale()) closeMenu(false, true);
      ui.itemChanged = false; ui.clearDraft = false;
      afterRender(name);
      measure();
      wkFrameSync(null);
      return;
    }

    if (windowHold && (windowHold.store !== name || windowHold.item !== ui.item || ui.itemChanged || ui.clearDraft)) windowHold = null;
    var draft = $('cbox') ? $('cbox').value : windowHold ? windowHold.draft : '';
    // cards scroll inside; keep each card's scroll and the focused row across the re-render
    var scrolls = {}, fa = document.activeElement, focusRk = fa && fa.closest && body.contains(fa) && fa.closest('[data-rk]');
    focusRk = focusRk && focusRk.getAttribute('data-rk');
    var focusSearch = fa && (fa.id === 'psearch' || fa.id === 'wksearch') ? [fa.id, fa.selectionStart, fa.selectionEnd] : null;
    // the wiki's page and tree scroll inside the card body; a page just opened starts at the top
    var wkScroll = {}; ['wkpage', 'wktree'].forEach(function (k) { if ($(k)) wkScroll[k] = $(k).scrollTop; });
    // an inline input (new item, subtask, in-place edit, the composer) keeps focus and its caret
    var focusKeep = fa && fa.id && fa.hasAttribute('data-keep') ? [fa.id, fa.selectionStart, fa.selectionEnd] : null;
    // an editor's Write / Preview toggle keeps focus too (it has focus while the editor previews)
    var focusTab = fa && fa.classList && fa.classList.contains('tked-tab') ? '.tked-tab[data-ed="' + CSS.escape(fa.getAttribute('data-ed')) + '"][data-mode="' + fa.getAttribute('data-mode') + '"]' : null;
    var focusFk = fa && fa.closest && (body.contains(fa) || sb.contains(fa)) && fa.closest('[data-fk]'); // the fold buttons keep keyboard focus across their re-render
    focusFk = focusFk && focusFk.getAttribute('data-fk');
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { scrolls[c.parentNode.id] = c.scrollTop; });
    hideTip();
    insPar(null); HV = null;
    var drawer = drawerHtml(name, v);
    if (VS.projection) patchTrackerHTML(body, drawer); else body.innerHTML = drawer;
    placeSub(); placeNew();
    if (!ui.page) fitDrawer();
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { if (scrolls[c.parentNode.id]) c.scrollTop = scrolls[c.parentNode.id]; });
    body.scrollTop = top1;
    wkAfterDraw(body, wkScroll);
    if (focusSearch && $(focusSearch[0])) { $(focusSearch[0]).focus({ preventScroll: true }); $(focusSearch[0]).setSelectionRange(focusSearch[1], focusSearch[2]); }
    var ff = focusFk && lastInput === 'key' && body.querySelector('[data-fk="' + CSS.escape(focusFk) + '"]');
    if (ff) ff.focus({ preventScroll: true });
    var fr = focusRk && lastInput === 'key' && body.querySelector('[data-rk="' + CSS.escape(focusRk) + '"]');
    if (fr) fr.focus({ preventScroll: true });
    var sr = ui.sel && body.querySelector('[data-rk="' + CSS.escape(ui.sel) + '"]');
    if (sr) placeMore(sr);
    // the item window: one at a time; the rest of the page is inert while it is open
    var open = !!(ui.win && ui.item);
    $('tkwin').hidden = !open;
    $('tkwin').firstElementChild.setAttribute('aria-labelledby', 'iw-title');
    setInert(open);
    dropEditorState(open);
    if (open) {
      var ready = !AD.detail || AD.detail(name, [ui.item]);
      // A summary invalidates the old detail before its replacement arrives.
      // Preserve the draft/caret/scroll across that loading gap, not just a
      // synchronous redraw. Item switches and successful posts still clear it.
      if (!ready && !windowHold) windowHold = { store: name, item: ui.item, draft: draft, focus: focusKeep, top: top2 };
      if (ready && windowHold) { draft = windowHold.draft; focusKeep = focusKeep || windowHold.focus; top2 = windowHold.top; windowHold = null; }
      var windowBody = ready ? windowHtml(v, VS.items[ui.item]) :
        '<h2 id="iw-title">' + esc(VS.items[ui.item].title) + '</h2><p role="status">' + esc(AD.detailError && AD.detailError(name) || wsT('js.tk.loading')) + '</p>' +
        btn('detail-retry', wsT('js.preview.reload'), {}) + btn('win-close', wsT('js.tk.win.close'), {});
      if (VS.projection) patchTrackerHTML(sb, windowBody); else sb.innerHTML = windowBody;
      wkMarkLinks(sb);
      if (!ui.itemChanged && !ui.clearDraft && $('cbox')) $('cbox').value = draft;
      sb.scrollTop = ui.itemChanged ? 0 : top2;
      // the notification's target: a thin mark stays while the window is open; the flash and the scroll happen once
      var fx = ui.focus && focusTarget(sb, ui.focus.ev);
      if (fx) {
        fx.classList.add('fx-mark');
        if (ui.focus.flash) { fx.classList.add('fx-flash'); fx.scrollIntoView({ block: 'center' }); }
      }
      if (ui.focus && ready) ui.focus.flash = false;
      var wf = focusFk && lastInput === 'key' && sb.querySelector('[data-fk="' + CSS.escape(focusFk) + '"]');
      if (wf) wf.focus({ preventScroll: true });
    } else { sb.innerHTML = ''; windowHold = null; }
    if (open) placeSub();
    var fk2 = focusKeep && $(focusKeep[0]);
    if (fk2) { fk2.focus({ preventScroll: true }); try { fk2.setSelectionRange(focusKeep[1], focusKeep[2]); } catch (x) { /* not a text field */ } }
    var ft = focusTab && document.querySelector(focusTab);
    if (ft) ft.focus({ preventScroll: true });
    if (MP) mentionSync(); // the editors were redrawn: the popup follows the new box, or closes with it
    // the menu stays over the redraw: the status popover follows the store, an item menu closes with its item
    if (MN && MN.kind === 'sync') {
      var mIn = $('tkmenu').contains(document.activeElement);
      $('tkmenu').innerHTML = syncPopHtml(); placeMenu();
      if (mIn) { var mf = $('tkmenu').querySelector('button'); if (mf) mf.focus({ preventScroll: true }); }
    } else if (MN && mnStale()) closeMenu(false, true);
    ui.itemChanged = false; ui.clearDraft = false;
    observeDetails(body);
    afterRender(name);
    measure();
    wkFrameSync(body); // after measure: the drawer hangs where the header ends
  }

  // A store comes on screen: the immediate check, then what is new is shown once. What
  // the check brings in is new on this showing too: when the check lands after this
  // returns (a network round trip), those keys join the fresh set and are marked seen
  // then, so they do not come back as a badge on the next open. A check that lands
  // synchronously is already in the view read below.
  function bringUp(name) {
    var now = true, shown;
    checkNow(name, function () {
      if (now || ui.store !== name || ui.fresh !== shown || !viewing()) return;
      vss[name] = AD.view(name);
      markSeen(name, true);
      render();
    });
    now = false;
    vss[name] = AD.view(name);
    markSeen(name);
    shown = ui.fresh;
  }
  // Collapsing the open drawer pushes (the apply path).
  function collapse() { var n = ui.open; ui.open = null; syncNow(n, true); render(); }
  // foldStore is a click on store name's fold button: its open drawer closes as a click on
  // its handle closes it, and keyboard focus goes to that handle, since the button hides.
  function foldStore(name) {
    if (ui.open !== name) return;
    var kb = document.activeElement === $('tkf-' + name);
    openStore(name);
    if (kb && $('tkh-' + name)) $('tkh-' + name).focus();
  }
  function openStore(name) {
    if (ui.open === name) { collapse(); return; }
    var opener = $('tkh-' + name), kb = !!opener && document.activeElement === opener;
    if (ui.open) syncNow(ui.open, true); // switching to another store while one is open applies it
    if (ui.store !== name) { keepItemDraft(); ui.iedit = null; ui.item = null; ui.win = false; ui.last = null; ui.itemChanged = true; $('tkbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    bringUp(name);
    render();
    // the focused handle hides while its drawer is open: focus moves to the fold button in its
    // slot, unless the render already moved it elsewhere
    var fold = $('tkf-' + name);
    if (kb && fold && !fold.hidden && document.activeElement === opener) fold.focus();
    document.dispatchEvent(new CustomEvent('tk:drawer-open', { detail: { store: name } }));
  }
  // openItemWindow(storeId, itemId): the one entry point that opens an item, from a row, a
  // notification, a chat reference, a subtask link, or (later) a URL route. It switches to
  // the store when needed (the drawer stays as it is, or closes when it shows another
  // store), replaces any open window, and moves focus into it. Returns false when the
  // store or the item is unknown to the current viewer.
  // `opts` is { focus: eventId, opener: element } (a bare element still means the opener): the
  // window then scrolls to and marks what the event refers to, and just opens when the
  // window has nothing for it. An event URL route would reuse exactly this.
  function openItemWindow(storeId, itemId, opts) {
    var opener = opts && opts.nodeType ? opts : opts && opts.opener, focus = opts && !opts.nodeType && opts.focus;
    if ((AD.stores() || []).indexOf(storeId) < 0) return false;
    var sv = AD.view(storeId);
    if (!sv || !sv.items[itemId]) return false;
    var switching = !ui.win || ui.store !== storeId || ui.item !== itemId;
    if (switching) { keepItemDraft(); ui.iedit = null; }
    if (ui.page && ui.page !== storeId) { history.pushState(null, '', '#/' + encodeURIComponent(storeId)); applyRoute(); } // on a store page: go to the item's store page
    if (ui.store !== storeId) {
      if (ui.open) { syncNow(ui.open, true); ui.open = null; }
      ui.store = storeId; ui.last = null; ui.sel = null;
    }
    if (!ui.win) ui.winReturn = returnSel(opener || document.activeElement);
    if (ui.item !== itemId) { ui.iedit = null; ui.cedit = null; if (ui.sub && ui.sub.rk.indexOf('iw:') === 0) ui.sub = null; }
    if (MN) closeMenu(false);
    ui.item = itemId; ui.win = true; ui.itemChanged = true; ui.aopen = null; ui.replyTo = null; ui.mdraft = []; ui.adraft = [];
    if (switching) ui.iedit = restoreItemDraft(storeId, itemId);
    ui.focus = focus ? { ev: focus, flash: true } : null;
    render();
    $('tkwin').firstElementChild.focus({ preventScroll: true });
    return true;
  }
  // openNotify(storeId) brings a store's Notify tab up: on its store page when a store page
  // shows, else in its drawer, opened when it is not. The wide layout shows notifications
  // in a column of their own, which the side-tab choice leaves alone.
  function openNotify(storeId) {
    if ((AD.stores() || []).indexOf(storeId) < 0) return false;
    var ts = (ui.tab[storeId] = ui.tab[storeId] || { side: 'notify', narrow: 'items' });
    ts.side = 'notify'; ts.narrow = 'notify';
    if (ui.page) {
      if (ui.page !== storeId) { history.pushState(null, '', '#/' + encodeURIComponent(storeId)); applyRoute(); }
      else render();
    } else if (ui.open !== storeId) openStore(storeId);
    else render();
    return true;
  }
  function showItem(id, opener, focus) { openItemWindow(ui.store, id, { opener: opener, focus: focus }); }
  // uiCommand performs a store support chat's UI command (h.uiCommand) on store id's view,
  // as the user's own operation would: search puts cmd.arg in the Items search box and
  // sends it at once (no typing debounce to wait out; searchFor), focus opens the item whose alias is cmd.arg
  // (at the thread comment whose alias is cmd.comment, when given), scope and view switch
  // the Items scope and view mode. Search, scope, and view bring the Items panel up: the
  // Items tab when narrow (where it replaces the chat's tab), the items side of the
  // Items | Wiki switch when wide. A command for a store other than the one shown, or one
  // the view cannot follow (an unknown item or comment, a scope or mode the panel does not
  // offer now), does nothing. Returns whether it acted.
  function uiCommand(id, cmd) {
    var sv = id === ui.store && AD.view(id);
    if (!sv || !cmd) return false;
    var findAlias = function (ids, a) { var hit = null; ids.some(function (k) { if (alias(k) === a) { hit = k; return true; } return false; }); return hit; };
    if (cmd.op === 'focus') {
      var iid = findAlias(Object.keys(sv.items), cmd.arg);
      if (!iid) return false;
      var cid = cmd.comment ? findAlias(listOf(sv.items[iid].thread).filter(function (k) { return sv.projection || sv.comments[k]; }), cmd.comment) : null;
      return openItemWindow(id, iid, cid ? { focus: cid } : null);
    }
    VS = sv; // scopeList reads the viewer's team from the shown view
    var ps = panelState();
    if (cmd.op === 'search') { ps.q = String(cmd.arg || ''); searchState(id).now = true; }
    else if (cmd.op === 'scope') {
      if (!scopeList().some(function (x) { return x[0] === cmd.arg; })) return false;
      ps.scope = cmd.arg;
    } else if (cmd.op === 'view') {
      var sc = scopeOf();
      if (!SCOPE_MODES[sc].some(function (x) { return x[0] === cmd.arg; })) return false;
      ps.modes[sc] = cmd.arg; dsSave('panel.modes', ui.pmodes);
    } else return false;
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    else if (AD.wiki) wk().mode = 'items';
    ui.sel = null;
    render();
    return true;
  }
  // A selector for the element focus returns to: the row, feed entry, or reference that
  // opened the window survives a re-render under the same data-rk / data-nkey / data-cref.
  function returnSel(el) {
    if (!el || el === document.body) return null;
    var k = ['rk', 'nkey', 'cref', 'handle'];
    for (var i = 0; i < k.length; i++) { var c = el.closest('[data-' + k[i] + ']'); if (c) return '[data-' + k[i] + '="' + CSS.escape(c.getAttribute('data-' + k[i])) + '"]'; }
    return el.id ? '#' + CSS.escape(el.id) : null;
  }
  function closeItemWindow() {
    var back = ui.winReturn;
    keepItemDraft();
    ui.win = false; ui.winReturn = null; ui.focus = null; ui.replyTo = null; ui.mdraft = []; ui.adraft = []; ui.iedit = null; ui.cedit = null;
    if (ui.sub && ui.sub.rk.indexOf('iw:') === 0) ui.sub = null;
    if (MN) closeMenu(false);
    render();
    var el = back && document.querySelector(back);
    if (el && !el.matches('a[href], button, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]'); // a feed entry: its title link
    if (el) el.focus();
  }

  // The store page: the route #/<storeId> shows that store full-page, with the same Items
  // panel and tab pane layout, in place of the conversation and the drawer. It is the only
  // route the renderer keeps. Entering a page checks and shows what is new, like opening
  // the drawer; leaving it pushes, like a collapse. Clearing the hash, the browser back
  // button, or "Back" returns to the host's own view. A hash that is not a #/ route is the
  // host page's and is left alone.
  // The link #/<storeId>/<itemId> (a tracker event hook's url, which points at this
  // machine's own daemon) enters that store page and opens the item once the store's view
  // holds it: applyRoute replaces it with the store page's route at once, so the item is not
  // a route of its own, and back leaves the page.
  function routeStore() {
    var m = /^#\/([^/]+)$/.exec(location.hash), id = m && decodeURIComponent(m[1]);
    return id && (AD.stores() || []).indexOf(id) >= 0 ? id : null;
  }
  function routeItem() {
    var m = /^#\/([^/]+)\/([^/]+)$/.exec(location.hash);
    if (!m) return null;
    try { return { store: decodeURIComponent(m[1]), item: decodeURIComponent(m[2]) }; } catch (e) { return null; }
  }
  // openRouteItem opens the item a link named (ui.routeItem) once its store's view has
  // arrived; an item the view does not hold is dropped.
  function openRouteItem() {
    var r = ui.routeItem, sv = r && AD.view(r.store);
    if (!sv) return;
    ui.routeItem = null;
    if (sv.items[r.item] && ui.page === r.store) openItemWindow(r.store, r.item);
  }
  function applyRoute() {
    var ri = routeItem();
    if (ri && (AD.stores() || []).indexOf(ri.store) >= 0) {
      ui.routeItem = ri;
      history.replaceState(null, '', location.pathname + location.search + '#/' + encodeURIComponent(ri.store));
    }
    var s = routeStore();
    if (/^#\/./.test(location.hash) && !s) toast(wsT("js.tk.toast.nostore", esc(location.hash.slice(2))));
    if (s !== ui.page) {
      if (ui.page) syncNow(ui.page, true);
      if (s) {
        if (ui.open) { syncNow(ui.open, true); ui.open = null; }
        if (ui.store !== s) { keepItemDraft(); ui.iedit = null; ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.itemChanged = true; }
        ui.store = s;
        bringUp(s);
      }
      ui.page = s;
    }
    render();
    openRouteItem();
  }
  function leavePage() { history.pushState(null, '', location.pathname + location.search); applyRoute(); }

  // One palette: five result slots (board, files, content, tracker, tracker
  // content), with an optional host-owned answer column. The host
  // supplies the board rows and their picker (O.searchRows, O.searchPick) and
  // the file hooks (O.fileFind, O.fileSearch, O.fileList, O.filePreview,
  // O.fileOpen), so the renderer never reaches a page's file routes itself.
  // Every query is a generation: a keystroke aborts every request in flight,
  // and a batch from an older generation is dropped. Local slots answer at
  // once; files, content, and tracker content wait out QDEBOUNCE, and their
  // previous rows stay, dimmed, until the new generation's first batch.
  var QSLOTS = ['board', 'files', 'content', 'tracker', 'tnote'], QASYNC = ['files', 'content', 'tnote'];
  // the Tab order of each layout step: medium runs the name column, then the text column
  var QORDER = { narrow: ['all'].concat(QSLOTS), medium: ['board', 'files', 'tracker', 'content', 'tnote'], wide: QSLOTS };
  // QMAX caps the rows a slot draws and navigates; QTNOTE_CAP the tracker content hits per store
  var QDEBOUNCE = 80, QMAX = 100, QTNOTE_CAP = 8;
  function qSlot0() { return { rows: [], state: 'idle', stale: false, more: false, err: '', ms: 0, t0: 0, wait: 0 }; }
  // cur is a result slot or the narrow layout's 'all' tab. Selection is a
  // row key so async batches cannot move an explicitly armed result.
  var qs = { open: false, back: null, rows: [], q: '', gen: 0, timer: 0, ctl: [], step: 'narrow', cur: 'all', sel: {}, path: null, S: {} };
  QSLOTS.forEach(function (k) { qs.S[k] = qSlot0(); });
  // Fuzzy subsequence score. Try each first match so a scattered early match
  // cannot hide a compact later one. Long prose needs a reasonably dense span.
  function qScore(text, query, prose) {
    var s = String(text || '').toLowerCase(), q = query.toLowerCase(), best = null;
    if (!q) return { score: 0, start: 0, end: 0 };
    for (var start = s.indexOf(q[0]); start >= 0; start = s.indexOf(q[0], start + 1)) {
      var pos = start, score = 10 - start * 0.05, run = 1, ok = true;
      for (var i = 1; i < q.length; i++) {
        var next = s.indexOf(q[i], pos + 1);
        if (next < 0) { ok = false; break; }
        if (next === pos + 1) { run++; score += 10 + run * 4; }
        else { run = 1; score += 10 - (next - pos - 1) * 2; }
        pos = next;
      }
      if (ok && (!prose || pos - start + 1 <= Math.max(24, q.length * 6)) && (!best || score > best.score)) best = { score: score, start: start, end: pos + 1 };
    }
    return best;
  }
  // The layout step follows the palette's own width, not the viewport's.
  function qStepOf(w) { return w < 600 ? 'narrow' : w < 1000 ? 'medium' : 'wide'; }
  // The board slot: (n) is exactly item n, one hit; a bare number matches item ids, prefix
  // matches first; other text matches topics, bodies, items, answers and entries. With no query it
  // lists topics, then items, newest first.
  function qBoard(rows, t) {
    var ex = /^\((\d+)\)$/.exec(t), hits = [];
    var items = rows.filter(function (x) { return x.kind === 'board-item'; });
    if (ex) return items.filter(function (x) { return x.id === ex[1]; }).slice(0, 1);
    if (/^\d+$/.test(t)) {
      items.forEach(function (x) { var m = qScore(x.id, t); if (m) hits.push({ row: x, pre: x.id.indexOf(t) === 0, score: m.score }); });
      hits.sort(function (a, b) { return (b.pre - a.pre) || b.score - a.score || Number(a.row.id) - Number(b.row.id); });
      return hits.map(function (x) { return x.row; });
    }
    rows.forEach(function (x, i) {
      if (!x.group || (!t && x.group !== 1 && x.group !== 3)) return;
      var m = qScore(x.text || x.title, t, x.group === 2 || x.group === 4 || x.group === 5);
      if (m) hits.push({ row: x, score: m.score, order: i });
    });
    hits.sort(function (a, b) {
      return (t ? b.score - a.score || a.row.group - b.row.group : a.row.group - b.row.group || (b.row.at || 0) - (a.row.at || 0)) || a.order - b.order;
    });
    return hits.map(function (x) { return x.row; });
  }
  // The tracker slot: a fuzzy title or alias match over the loaded store views, with no
  // search grammar (the tracker content slot sends that); with no query, the items in
  // progress.
  function qTracker(t) {
    var hits = [];
    order.forEach(function (n) {
      var V = vss[n];
      if (!V) return;
      V.itemOrder.forEach(function (id) {
        var it = V.items[id];
        if (!it || it.hidden) return;
        var m = t ? qScore(it.title, t) : it.status === 'doing' ? { score: 0 } : null, a = t && it.alias ? qScore(it.alias, t) : null;
        if (a && (!m || a.score > m.score)) m = a;
        if (m) hits.push({ row: { store: n, item: id, title: it.title, alias: it.alias, st: it.kind === 'topic' ? 'topic' : it.status, key: n + '/' + id }, score: m.score, order: hits.length });
      });
    });
    hits.sort(function (a, b) { return b.score - a.score || a.order - b.order; });
    return hits.map(function (x) { return x.row; });
  }
  function qKeyOf(k, r) {
    if (k === 'board') return r.kind + ':' + (r.entry ? r.topic + ':' + r.entry : r.id || r.topic || r.title);
    return r.key;
  }
  // qView lists one slot's rows or each slot's top three in the 'all' tab.
  function qView(v) {
    var out = [];
    (v === 'all' ? QSLOTS : [v]).forEach(function (k) {
      qs.S[k].rows.slice(0, v === 'all' ? 3 : QMAX).forEach(function (r) { out.push({ slot: k, row: r, key: k + '|' + qKeyOf(k, r) }); });
    });
    return out;
  }
  // Only arrow movement arms a result. Late batches keep that key, never
  // silently select a replacement when the selected row disappeared.
  function qIdx(v) {
    var list = qView(v);
    for (var j = 0; j < list.length; j++) if (list[j].key === qs.sel[v]) return j;
    return -1;
  }
  function qSave() { if (O.searchState) O.searchState.save({ query: qs.q, slot: qs.cur }); }
  function qFixCur() {
    var ord = QORDER[qs.step];
    if (ord.indexOf(qs.cur) < 0) { qs.cur = ord[0]; qs.sel = {}; }
  }
  // qAbsPath mirrors the daemon's root check (fs_routes.go browseRootParam): an absolute
  // POSIX path or a Windows drive path. A network path (//host, \\host), a device
  // namespace (\\?\, \\.\), a Windows device name, ~/, and a relative path are not. The
  // daemon's refusal stays the authority.
  function qAbsPath(t) {
    if (!t || t.indexOf('\u0000') >= 0 || /^[\\/]{2}/.test(t)) return false;
    if (t.charAt(0) === '/') return true;
    if (!/^[A-Za-z]:[\\/]/.test(t)) return false;
    return !t.slice(3).split(/[\\/]/).some(function (e) { return /^(con|prn|aux|nul|com[0-9]|lpt[0-9])\s*([.:].*)?$/i.test(e); });
  }
  // qPathOf: an absolute path typed by a browser on the daemon's machine switches the files
  // slot to path mode, one directory deep: a trailing separator lists that directory, else
  // its parent filtered by the last segment.
  function qPathOf(t) {
    if (!O.fileList || !O.fileLocal || !O.fileLocal() || !qAbsPath(t)) return null;
    var i = Math.max(t.lastIndexOf('/'), t.lastIndexOf('\\'));
    return { dir: t.slice(0, i + 1), leaf: t.slice(i + 1), sep: t.charAt(i) };
  }
  function qPathRows(a, p) {
    var leaf = p.leaf.toLowerCase(), pre = [], fz = [];
    ((a && a.entries) || []).forEach(function (x, i) {
      // a deleted file (gone) has nothing to open; a link is never opened or descended
      if (x.gone || x.link) return;
      var r = { name: x.name, dir: !!x.dir, key: x.name };
      if (x.name.toLowerCase().indexOf(leaf) === 0) pre.push(r);
      else { var m = qScore(x.name, leaf); if (m) fz.push({ row: r, score: m.score, order: i }); }
    });
    fz.sort(function (a, b) { return b.score - a.score || a.order - b.order; });
    return pre.concat(fz.map(function (x) { return x.row; }));
  }
  // qHitRows turns a store search's excerpt hits into tracker content rows: hidden items
  // dropped unless the query asks for them (as the Items panel does), at most QTNOTE_CAP.
  function qHitRows(n, hits, wantHidden) {
    var V = vss[n], rows = [];
    hits.forEach(function (h) {
      var hidden = h.hidden || (V && V.items[h.id] && V.items[h.id].hidden);
      if (hidden && !wantHidden) return;
      rows.push({ store: n, item: h.id, title: h.title, alias: h.alias, st: h.kind === 'topic' ? 'topic' : h.status, field: h.field, comment: h.comment || '', text: h.text || '',
        key: n + '/' + h.id + '/' + (h.comment || h.field) });
    });
    return { rows: rows.slice(0, QTNOTE_CAP), more: rows.length > QTNOTE_CAP };
  }
  function qErrText(err) {
    if (err && (err.text || err.message)) return String(err.text || err.message);
    return err && err.status ? 'HTTP ' + err.status : wsT("js.tk.qs.st.error");
  }
  function qAbort() {
    qs.ctl.forEach(function (c) { c.abort(); });
    qs.ctl = [];
    clearTimeout(qs.timer); qs.timer = 0;
  }
  function qDone(rows) { var s = qSlot0(); s.rows = rows; s.state = 'done'; return s; }
  // qRun starts a generation for the input's text: the local slots answer at
  // once, the async slots after QDEBOUNCE.
  function qRun() {
    var q = $('tkqin').value, t = q.trim(), gen = ++qs.gen, now = Date.now();
    qAbort();
    var wasPath = !!qs.path;
    qs.q = q; qs.sel = {}; qs.path = qPathOf(t);
    // path rows ({name, dir}) and found files ({path}) draw differently: never keep one as the other's stale rows
    if (!!qs.path !== wasPath) qs.S.files = qSlot0();
    qs.S.board = qDone(qBoard(qs.rows, t));
    qs.S.tracker = qDone(qTracker(t));
    var on = { files: !!t && !!(qs.path ? O.fileList : O.fileFind), content: !!t && !qs.path && !!O.fileSearch, tnote: !!t && !!AD.search && order.length > 0 };
    QASYNC.forEach(function (k) {
      var s = qs.S[k];
      if (!on[k]) { qs.S[k] = qSlot0(); return; }
      s.stale = s.rows.length > 0; s.state = 'run'; s.more = false; s.err = ''; s.t0 = now; s.wait = 0;
    });
    if (/^\(\d+\)$/.test(t) && qs.S.board.rows.length) qs.cur = qs.step === 'narrow' ? 'all' : 'board';
    qRender();
    if (on.files || on.content || on.tnote) qs.timer = setTimeout(function () { qs.timer = 0; if (gen === qs.gen) qFetch(gen, t, on); }, QDEBOUNCE);
  }
  // qLocal re-ranks the local slots for the same query, after the host's rows
  // changed (an older board page loaded); the async slots are left as they are.
  function qLocal() {
    var t = $('tkqin').value.trim();
    qs.S.board = qDone(qBoard(qs.rows, t));
    qs.S.tracker = qDone(qTracker(t));
    qs.sel = {};
    qRender();
  }
  function qFetch(gen, t, on) {
    function signal() {
      if (typeof AbortController !== 'function') return undefined;
      var c = new AbortController();
      qs.ctl.push(c);
      return c.signal;
    }
    if (on.files && qs.path) {
      var p = qs.path;
      O.fileList(p.dir, signal()).then(function (a) { qBatch(gen, 'files', qPathRows(a, p), true); }, function (err) { qFail(gen, 'files', qErrText(err)); });
    } else if (on.files) O.fileFind(t, signal()).then(function (a) {
      qBatch(gen, 'files', ((a && a.items) || []).filter(function (x) { return !x.dir; }).map(function (x) { return { path: x.path, key: x.path }; }), true);
    }, function (err) { qFail(gen, 'files', qErrText(err)); });
    if (on.content) O.fileSearch(t, signal()).then(function (a) {
      var rows = [];
      ((a && a.files) || []).forEach(function (f) {
        (f.matches || []).forEach(function (m) { rows.push({ path: f.path, line: m.line, pre: m.pre || '', hit: m.hit || '', post: m.post || '', key: f.path + ':' + m.line }); });
      });
      qBatch(gen, 'content', rows, true, !!(a && a.more));
    }, function (err) { qFail(gen, 'content', qErrText(err)); });
    if (on.tnote) {
      // every registered store, loaded or not, one request each; a 400 is the grammar's
      // refusal, the same for every store, and puts only this slot in its error state
      var wantHidden = /(^|\s)status:hidden(\s|$)/i.test(t), ids = order.slice();
      qs.S.tnote.wait = ids.length;
      ids.forEach(function (n) {
        AD.search(n, t, function (res) {
          var s = qs.S.tnote;
          if (gen !== qs.gen || !qs.open || s.state === 'err') return;
          if (!res.ok && res.status === 400) { qFail(gen, 'tnote', res.error); return; }
          s.wait--;
          var hr = res.ok ? qHitRows(n, res.hits || [], wantHidden) : { rows: [], more: false };
          qBatch(gen, 'tnote', hr.rows, s.wait <= 0, hr.more);
        }, { excerpts: true, signal: signal() });
      });
    }
  }
  function qBatch(gen, k, rows, done, more) {
    if (gen !== qs.gen || !qs.open) return;
    var s = qs.S[k];
    if (s.stale) { s.rows = []; s.stale = false; }
    s.rows = s.rows.concat(rows);
    if (more) s.more = true;
    if (done) { s.state = 'done'; s.ms = Date.now() - s.t0; }
    qRender();
  }
  function qFail(gen, k, msg) {
    if (gen !== qs.gen || !qs.open) return;
    var s = qs.S[k];
    s.rows = []; s.stale = false; s.state = 'err'; s.err = msg;
    qRender();
  }

  function qLabel(k) {
    return k === 'board' ? wsT("js.tk.qs.slot.board") : k === 'files' ? wsT("js.tk.qs.slot.files") : k === 'content' ? wsT("js.tk.qs.slot.content") :
      k === 'tracker' ? wsT("js.tk.qs.tracker") : k === 'tnote' ? wsT("js.tk.qs.slot.tnote") : wsT("js.tk.qs.all");
  }
  function qState(k) {
    var s = qs.S[k];
    if (k === 'content' && qs.path) return wsT("js.tk.qs.st.path");
    if (s.state === 'idle') return wsT("js.tk.qs.st.idle");
    if (s.state === 'run') return wsT("js.tk.qs.st.run", s.stale ? '…' : String(s.rows.length));
    if (s.state === 'err') return wsT("js.tk.qs.st.error");
    if (!s.rows.length) return wsT("js.tk.qs.st.none");
    return s.more ? wsT("js.tk.qs.st.cap", String(s.rows.length)) : wsT("js.tk.qs.st.done", String(s.rows.length), (s.ms / 1000).toFixed(1));
  }
  function qExcerpt(text, query) {
    var m = qScore(text, query.trim(), true), start = m ? Math.max(0, m.start - 35) : 0;
    return (start ? '…' : '') + text.slice(start, start + 150) + (text.length > start + 150 ? '…' : '');
  }
  function qSplit(p) { var i = p.lastIndexOf('/'); return { dir: p.slice(0, i + 1), name: p.slice(i + 1) }; }
  function qRowHtml(k, r) {
    var q = qs.q.trim();
    if (k === 'board') return '<span class="tk-qid">' + esc(r.id ? '(' + r.id + ')' : '#') + '</span><span class="qboard"><b>' + esc(r.group === 2 || r.group === 4 || r.group === 5 ? qExcerpt(r.title, q) : r.title) + '</b><span class="muted">' + esc(r.hint || '') + '</span></span><span class="grow"></span>' +
      (/^\(\d+\)$/.test(q) ? '<span class="qk tk-qexact">' + wsT("js.tk.qs.exact") + '</span>' : '');
    if (k === 'tracker') return '<span class="sti">' + SI.svg(r.st) + '</span><span class="qt">' + esc(r.title) + '</span><span class="grow"></span><code class="al">' + esc(r.alias) + '</code><span class="qk">' + esc(r.store) + '</span>';
    if (k === 'files' && qs.path) return '<svg class="i"><use href="#tki-' + (r.dir ? 'folder' : 'doc') + '"/></svg><span class="qt">' + esc(r.name + (r.dir ? qs.path.sep : '')) + '</span>';
    if (k === 'files') { var p = qSplit(r.path); return '<svg class="i"><use href="#tki-doc"/></svg><span class="qt">' + esc(p.name) + '</span><span class="muted">' + esc(p.dir) + '</span>'; }
    if (k === 'content') return '<span class="muted">' + esc(r.path + ':' + r.line) + '</span><code class="tk-qcode">' + esc(r.pre) + '<mark>' + esc(r.hit) + '</mark>' + esc(r.post) + '</code>';
    var f = r.field === 'comment' ? wsT("js.tk.qs.comment") : r.field === 'body' ? wsT("js.tk.qs.body") : '';
    return '<span class="tk-qline"><span class="sti">' + SI.svg(r.st) + '</span><span class="qt">' + esc(r.title) + '</span><span class="muted">' + esc((f ? f + ' · ' : '') + r.alias + ' · ' + r.store) + '</span></span>' +
      (r.text ? '<span class="muted tk-qx">' + esc(r.text) + '</span>' : '');
  }
  function qOpt(k, r, i, on) {
    return '<li class="qopt' + (k === 'content' || k === 'tnote' ? ' tk-qtall' : '') + (on ? ' on' : '') + '" role="option" id="tkqopt-' + k + '-' + i + '" aria-selected="' + on + '"' + attr('qs', k) + attr('qi', i) + '>' + qRowHtml(k, r) + '</li>';
  }
  // qList draws slot k's rows, at most max; sel is the index of the selected row, or -1.
  function qList(k, max, sel) {
    var s = qs.S[k], h = '';
    if (s.state === 'err') h = '<li class="qnone tk-qerr" role="presentation">' + esc(k === 'files' ? wsT("js.tk.qs.listfail", s.err) : s.err) + '</li>';
    else if (!s.rows.length) h = s.state === 'run' ? '<li class="tk-qskel" role="presentation"></li>' : s.state === 'done' ? '<li class="qnone" role="presentation">' + wsT("js.tk.qs.none") + '</li>' : '';
    else {
      s.rows.slice(0, max).forEach(function (r, i) { h += qOpt(k, r, i, i === sel); });
      if (s.rows.length > max) h += '<li class="tk-qmore" role="presentation">' + wsT("js.tk.qs.more", String(s.rows.length - max)) + '</li>';
    }
    return '<ul role="group" aria-label="' + esc(qLabel(k)) + '"' + (s.stale ? ' class="tk-qstale"' : '') + '>' + h + '</ul>';
  }
  function qHead(k, focus) {
    var s = qs.S[k];
    return '<div class="tk-qsh' + (focus ? ' tk-qcur' : '') + '"' + attr('qslot', k) + '><span>' + esc(qLabel(k)) + '</span><kbd>Alt+' + (QSLOTS.indexOf(k) + 1) + '</kbd><span class="tk-qst">' +
      (s.state === 'run' ? '<span class="tk-qspin"></span>' : '') + esc(qState(k)) + '</span></div>';
  }
  function qEmpty(k) { var s = qs.S[k]; return !s.rows.length && s.state !== 'run' && s.state !== 'err'; }
  function qRender() {
    if (!qs.open) return;
    qFixCur();
    qSave();
    var d = $('tkqpop'), list = $('tkqlist'), st = qs.step, v = qs.cur, h = '', keep = {};
    var si = qIdx(qs.cur), selE = qView(qs.cur)[si];
    function on(k, i) { return !!selE && selE.slot === k && selE.row === qs.S[k].rows[i]; }
    function rows(k, max) { var i = -1; for (var j = 0; j < Math.min(max, qs.S[k].rows.length); j++) if (on(k, j)) i = j; return qList(k, max, i); }
    d.setAttribute('data-step', st);
    $('tkqtabs').hidden = st !== 'narrow';
    if (st === 'narrow') $('tkqtabs').innerHTML = QORDER.narrow.map(function (k) {
      var s = qs.S[k], a = k === v, n = k === 'all' || s.state === 'idle' || s.state === 'err' ? '' : s.stale ? '…' : s.rows.length + (s.more ? '+' : '');
      return '<span class="tk-qtab' + (a ? ' on' : '') + (k !== 'all' && qEmpty(k) && s.state === 'done' ? ' tk-qzero' : '') + '" role="tab" aria-selected="' + a + '"' + attr('qslot', k) + '>' + esc(qLabel(k)) +
        (s && s.state === 'run' ? '<span class="tk-qspin"></span>' : '') + (n !== '' ? '<span class="tk-qcnt">' + n + '</span>' : '') + '</span>';
    }).join('');
    if (st === 'narrow' && v === 'all') {
      var shown = 0, busy = false;
      QSLOTS.forEach(function (k) {
        var s = qs.S[k];
        if (s.state === 'run') busy = true;
        if (qEmpty(k)) return;
        shown++;
        h += '<div class="tk-qsec">' + qHead(k, false) + rows(k, 3) + '</div>';
      });
      if (!shown && !busy) h = '<p class="qnone">' + wsT("js.tk.qs.none") + '</p>';
      h = '<div class="tk-qpane tk-qsingle" data-pane="all"><div class="tk-qscroll">' + h + '</div></div>';
    } else if (st === 'narrow') h = '<div class="tk-qpane tk-qsingle" data-pane="' + v + '">' + qHead(v, true) + '<div class="tk-qscroll">' + rows(v, QMAX) + '</div></div>';
    else if (st === 'medium') h = [['names', ['board', 'files', 'tracker']], ['texts', ['content', 'tnote']]].map(function (p) {
      return '<div class="tk-qpane' + (p[1].indexOf(v) >= 0 ? ' tk-qfocus' : '') + '" data-pane="' + p[0] + '"><div class="tk-qscroll">' + p[1].map(function (k) {
        return '<div class="tk-qsec' + (k === v ? ' tk-qopen' : '') + '">' + qHead(k, k === v) + (qEmpty(k) && k !== v ? '' : rows(k, k === v ? QMAX : 3)) + '</div>';
      }).join('') + '</div></div>';
    }).join('');
    else h = QSLOTS.map(function (k) {
      return '<div class="tk-qpane' + (k === v ? ' tk-qfocus' : '') + '" data-pane="' + k + '">' + qHead(k, k === v) + '<div class="tk-qscroll">' + rows(k, QMAX) + '</div></div>';
    }).join('');
    // a redraw keeps each pane where it was scrolled; the selection then comes into view
    Array.prototype.forEach.call(list.querySelectorAll('.tk-qpane[data-pane]'), function (p) { var sx = p.querySelector('.tk-qscroll'); if (sx) keep[p.getAttribute('data-pane')] = sx.scrollTop; });
    list.innerHTML = h;
    Array.prototype.forEach.call(list.querySelectorAll('.tk-qpane[data-pane]'), function (p) { var sx = p.querySelector('.tk-qscroll'), y = keep[p.getAttribute('data-pane')]; if (sx && y) sx.scrollTop = y; });
    var cur = selE ? 'tkqopt-' + selE.slot + '-' + qs.S[selE.slot].rows.indexOf(selE.row) : '';
    $('tkqin').setAttribute('aria-activedescendant', cur && $(cur) ? cur : '');
    var o = cur && $(cur);
    if (o && o.scrollIntoView) o.scrollIntoView({ block: 'nearest' });
  }
  // Hover is CSS-only: it never arms Enter to open a result.
  function qHover(o) {}
  function qOpener() { return $('tkqs') || $('shandle'); }
  function qOpen() {
    if (qs.open) return;
    qs.open = true; qs.back = document.activeElement;
    qs.rows = O.searchRows ? O.searchRows(function (rows) { if (qs.open) { qs.rows = rows; qLocal(); } }) : [];
    var d = $('tkqpop');
    d.hidden = false;
    if (O.searchHost) O.searchHost.setOpen(true);
    var qsBtn = $('tkqs');
    if (qsBtn) qsBtn.setAttribute('aria-expanded', 'true');
    qs.step = qStepOf(d.getBoundingClientRect ? d.getBoundingClientRect().width : 0);
    if (!qs.loaded && O.searchState) {
      var saved = O.searchState.load() || {};
      qs.q = typeof saved.query === 'string' ? saved.query : '';
      qs.cur = saved.slot || qs.cur;
    }
    qs.loaded = true;
    $('tkqin').value = qs.q; qRun(); $('tkqin').focus();
    if ($('tkqin').select) $('tkqin').select();
  }
  function qClose() {
    if (!qs.open) return;
    qs.open = false; qAbort(); qs.gen++;
    $('tkqpop').hidden = true;
    var qsBtn = $('tkqs');
    if (qsBtn) qsBtn.setAttribute('aria-expanded', 'false');
    if (O.searchHost) O.searchHost.setOpen(false);
    var b = qs.back; qs.back = null;
    if (b && b.isConnected) b.focus({ preventScroll: true });
    else { var op = qOpener(); if (op) op.focus({ preventScroll: true }); }
  }
  // Esc clears a query first and closes the palette on an empty one.
  function qEsc() { if ($('tkqin').value) { $('tkqin').value = ''; qRun(); } else qClose(); }
  function qTab(dir) {
    var ord = QORDER[qs.step], i = ord.indexOf(qs.cur);
    for (var n = 0; n < ord.length; n++) {
      i = (i + dir + ord.length) % ord.length;
      if (ord[i] === 'all' || qs.S[ord[i]].rows.length) { qs.cur = ord[i]; qs.sel = {}; qRender(); return; }
    }
  }
  // a tracker content hit opens its item window at the comment; a store with no loaded view
  // takes the item link's route (#/<store>/<item>), which loads it and opens the item
  function qOpenHit(r) {
    var V = AD.view(r.store);
    if (V && V.items && V.items[r.item]) {
      var op = qOpener();
      if (!openItemWindow(r.store, r.item, r.comment ? { focus: r.comment, opener: op } : { opener: op })) toast(wsT("js.tk.toast.hiddenitem"));
    } else location.hash = '#/' + encodeURIComponent(r.store) + '/' + encodeURIComponent(r.item);
  }
  // qPick opens row i of view v. keep (Ctrl+Enter) opens it under the palette, which stays
  // open with the focus in its input.
  function qPick(v, i, keep) { qPickEntry(qView(v)[i], keep); }
  function qPickEntry(e, keep) {
    if (!e || !e.row) return;
    var k = e.slot, r = e.row, p = qs.path;
    if (k === 'files' && p) {
      if (r.dir) { $('tkqin').value = p.dir + r.name + p.sep; qRun(); return; }
      if (keep) { if (O.fileOpen) O.fileOpen(r.name, p.dir); $('tkqin').focus(); return; }
      qClose();
      if (O.filePreview) O.filePreview(r.name, { root: p.dir });
      return;
    }
    if (!keep) qClose();
    if (k === 'board') { if (ui.win) closeItemWindow(); if (O.searchPick) O.searchPick(r, { keep: !!keep }); }
    else if (k === 'tracker') { if (!openItemWindow(r.store, r.item, { opener: qOpener() })) toast(wsT("js.tk.toast.hiddenitem")); }
    else if (k === 'tnote') qOpenHit(r);
    else if (O.filePreview) O.filePreview(r.path, k === 'content' ? { line: r.line } : {});
    if (keep) $('tkqin').focus();
  }
  // qKey: the input's keys. The focus stays in the input; Alt+digit is matched on e.code,
  // since macOS Option changes e.key.
  function qKey(e) {
    if (e.isComposing || e.keyCode === 229) return;
    var k = e.key, v = qs.cur, d = /^Digit([1-5])$/.exec(e.code || '');
    // AltGr (Ctrl+Alt on Windows) types characters on some layouts; it stays typing
    if (e.altKey && !e.ctrlKey && d) { e.preventDefault(); qs.cur = QSLOTS[d[1] - 1]; qs.sel = {}; qRender(); return; }
    if (k === 'Tab') { e.preventDefault(); qTab(e.shiftKey ? -1 : 1); return; }
    if (k === 'ArrowDown' || k === 'ArrowUp') {
      e.preventDefault();
      var list = qView(v), i = qIdx(v);
      if (!list.length) return;
      i = i < 0 ? 0 : Math.max(0, Math.min(list.length - 1, i + (k === 'ArrowDown' ? 1 : -1)));
      qs.sel[v] = list[i].key; qRender();
      return;
    }
    if (k === 'Enter') {
      e.preventDefault();
      if (!$('tkqin').value.trim()) return;
      var selected = qIdx(v);
      if (selected >= 0) qPick(v, selected, e.ctrlKey || e.metaKey);
      return;
    }
    if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); qEsc(); }
  }
  // Editors keep their own keys: a terminal card (its .xterm helper textarea),
  // the composer, and any textarea or contenteditable never lose Ctrl+K or
  // Ctrl+Space to the palette; / also stays typed in inputs and selects.
  // Ctrl+K, Ctrl+Space and Cmd+K open it from anywhere but a terminal, which
  // keeps its own keys; '/' only outside a text field.
  function qShortcut(e) {
    if (e.isComposing || e.keyCode === 229 || e.altKey) return false;
    var t = e.target && e.target.closest ? e.target : null, k = (e.key || '').toLowerCase() === 'k';
    if (e.ctrlKey || e.metaKey) return !e.shiftKey && (e.metaKey ? !e.ctrlKey && k : k || e.code === 'Space' || e.key === ' ') &&
      !(t && t.closest('.xterm'));
    return e.key === '/' && !(t && t.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'));
  }

  // The immediate custom tooltip: one fixed element shown at once for whatever [data-tip]
  // the pointer or keyboard focus is on (the browser's title waits ~1s).
  var tipFor = null;
  function showTip(el) {
    var t = $('tktip'), up = el.matches('.kcard, .citem') ? parentTip(el) : '';
    tipFor = el; t.textContent = el.getAttribute('data-tip') + (up ? '\n' + up : ''); t.hidden = false;
    var r = el.getBoundingClientRect(), w = t.offsetWidth, h = t.offsetHeight;
    var x = Math.max(6, Math.min(r.left + r.width / 2 - w / 2, window.innerWidth - w - 6)), y = r.bottom + 6;
    if (y + h > window.innerHeight - 6) y = r.top - h - 6;
    t.style.left = Math.round(x) + 'px'; t.style.top = Math.round(y) + 'px';
  }
  function hideTip() { tipFor = null; $('tktip').hidden = true; }
  // Kanban and calendar cards stay independent tasks. Hovering or focusing one marks its direct
  // parent's and direct children's cards in the same view (lk-par, lk-kid); the card's tooltip
  // names the parent when its card is not on screen (another month, or scrolled out of view).
  var LK = null; // the card whose links are marked
  function linkCards(box, id) { return id ? box.querySelectorAll('.kcard[data-item="' + CSS.escape(id) + '"], .citem[data-item="' + CSS.escape(id) + '"]') : []; }
  function linkHi(card) {
    if (card === LK) return;
    if (LK && LK.isConnected) Array.prototype.forEach.call(LK.closest('.kanban, .cal').querySelectorAll('.lk-on, .lk-par, .lk-kid'), function (n) { n.classList.remove('lk-on', 'lk-par', 'lk-kid'); });
    LK = card || null;
    var box = card && card.closest('.kanban, .cal'), it = box && VS && VS.items[card.getAttribute('data-item')];
    if (!it) return;
    card.classList.add('lk-on');
    Array.prototype.forEach.call(linkCards(box, it.parent), function (n) { n.classList.add('lk-par'); });
    it.children.forEach(function (k) { Array.prototype.forEach.call(linkCards(box, k), function (n) { if (n !== card) n.classList.add('lk-kid'); }); });
  }
  function onScreen(n) {
    var r = n.getBoundingClientRect(), v = { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight };
    for (var p = n.parentElement; p && !p.classList.contains('card'); p = p.parentElement) {
      if (getComputedStyle(p).overflow === 'visible') continue;
      var q = p.getBoundingClientRect();
      v = { left: Math.max(v.left, q.left), top: Math.max(v.top, q.top), right: Math.min(v.right, q.right), bottom: Math.min(v.bottom, q.bottom) };
    }
    return r.right > v.left && r.left < v.right && r.bottom > v.top && r.top < v.bottom;
  }
  function parentTip(card) {
    var box = card.closest('.kanban, .cal'), it = box && VS && VS.items[card.getAttribute('data-item')], par = it && VS.items[it.parent];
    if (!par || Array.prototype.some.call(linkCards(box, par.id), onScreen)) return '';
    return wsT("js.tk.link.parent", par.title);
  }
  // A row's detail overlay (the quick preview; the item window holds the full view) is
  // fixed-positioned so a short, content-height card does not clip it: below the row, or
  // above when below would leave the open drawer and above has more room.
  function placeMore(row) {
    var m = row.querySelector(':scope > .ir-more');
    if (!m) return;
    var r = row.getBoundingClientRect(), c = $(ui.page ? 'tkpage' : 'tkclip').getBoundingClientRect(), ind = Number(row.style.getPropertyValue('--ind')) || 0;
    var left = r.left + 28 + ind * 16;
    m.style.left = Math.round(left) + 'px'; m.style.width = Math.round(Math.max(220, r.right - 6 - left)) + 'px';
    var h = m.offsetHeight, lo = Math.min(c.bottom, window.innerHeight) - 4, hi = Math.max(c.top, 0) + 4;
    var down = r.bottom - 3 + h <= lo || lo - r.bottom >= r.top - hi;
    m.style.top = Math.round(down ? r.bottom - 3 : r.top + 3 - h) + 'px';
  }
  function placeShownMore() { Array.prototype.forEach.call(document.querySelectorAll('.tk .irow:hover, .tk .irow:focus-within, .tk .irow.sel'), placeMore); }
  var lastPointer = 'mouse', lastInput = 'pointer'; // the focused row is restored after a re-render only for keyboard use

  var QUICK = { confirm: 1, unassign: 1, accept: 1, ihide: 1, chide: 1, comment: 1, claim: 1, unopen: 1, approve: 1, unrequest: 1, uncc: 1 };
  // the item menu's steps an action button opens (in the menu, or a menu anchored to the button)
  var STEPS = { note: 1, handoff: 1, assign: 1, pri: 1, due: 1, labels: 1, move: 1, opening: 1, request: 1, wrap: 1, cc: 1 };

  // ------------------------------------------------------------- display state

  // Per-card view modes, the recently-closed fold, and the tree folds persist under
  // opts.storagePrefix; everything else lasts the page. Storage may throw (a locked-down
  // browser): then the values last the page.
  function dsLoad(k) {
    try { var m = JSON.parse(localStorage.getItem(PFX + k)); return m && typeof m === 'object' && !Array.isArray(m) ? m : {}; } catch (e) { return {}; }
  }
  function dsSave(k, m) { try { localStorage.setItem(PFX + k, JSON.stringify(m)); } catch (e) { /* storage off: the value lasts this page */ } }

