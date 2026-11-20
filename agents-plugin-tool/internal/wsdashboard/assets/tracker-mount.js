  // ---------------------------------------------------------------- mount

  // The icons the renderer draws, under its own ids (tki-*) so a host sprite never collides.
  var SPRITE = '<svg width="0" height="0" style="position:absolute" aria-hidden="true">' +
    '<symbol id="tki-back" viewBox="0 0 16 16"><path d="M10 3.5L5.5 8l4.5 4.5"/></symbol>' +
    '<symbol id="tki-trash" viewBox="0 0 16 16"><path d="M2.5 4.5h11M6.5 4.5V2.8h3v1.7M4 4.5l.7 8.7h6.6l.7-8.7M6.6 7v4M9.4 7v4"/></symbol>' +
    '<symbol id="tki-chev" viewBox="0 0 16 16"><path d="M4 6l4 4 4-4"/></symbol>' +
    '<symbol id="tki-up" viewBox="0 0 16 16"><path d="M4 10l4-4 4 4"/></symbol>' +
    '<symbol id="tki-tasks" viewBox="0 0 16 16"><path d="M2.5 4.2l1.4 1.4 2.4-2.6M2.5 10.2l1.4 1.4 2.4-2.6M8.5 4.6h5M8.5 10.6h5"/></symbol>' +
    '<symbol id="tki-plus" viewBox="0 0 16 16"><path d="M8 3v10M3 8h10"/></symbol>' +
    '<symbol id="tki-minus" viewBox="0 0 16 16"><path d="M4 8h8"/></symbol>' +
    '<symbol id="tki-x" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"/></symbol>' +
    '<symbol id="tki-accept" viewBox="0 0 16 16"><path d="M3.2 8.4l3 3 6.6-7"/></symbol>' +
    '<symbol id="tki-decline" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.6"/><path d="M5.9 5.9l4.2 4.2M10.1 5.9l-4.2 4.2"/></symbol>' +
    '<symbol id="tki-chat" viewBox="0 0 16 16"><path d="M2.8 3.8h10.4v6.8H7.4l-2.8 2.4v-2.4H2.8z"/></symbol>' +
    '<symbol id="tki-confirm" viewBox="0 0 16 16"><path d="M1.8 8.6l2.8 2.8 5.6-6M7.6 11.2l.4.2 6.2-6.6"/></symbol>' +
    '<symbol id="tki-reject" viewBox="0 0 16 16"><path d="M5.6 4L2.8 6.8l2.8 2.8"/><path d="M2.8 6.8h6.6a3.6 3.6 0 0 1 0 7.2H6.6"/></symbol>' +
    '<symbol id="tki-return" viewBox="0 0 16 16"><path d="M13 13V9.4a2.8 2.8 0 0 0-2.8-2.8H3.4M6.4 3.6l-3 3 3 3"/></symbol>' +
    '<symbol id="tki-close" viewBox="0 0 16 16"><rect x="2.4" y="2.4" width="11.2" height="11.2" rx="2.4"/><path d="M5.2 8.2l2 2 3.6-3.8"/></symbol>' +
    '<symbol id="tki-split" viewBox="0 0 16 16"><circle cx="4" cy="3.6" r="1.6"/><circle cx="4" cy="12.4" r="1.6"/><circle cx="12" cy="8" r="1.6"/><path d="M4 5.2v5.6M4 7.2c0 1 1 1.6 2.4 1.2l4-.5"/></symbol>' +
    '<symbol id="tki-handoff" viewBox="0 0 16 16"><circle cx="5.6" cy="5" r="2.2"/><path d="M1.8 13.4c.4-2.6 2-3.8 3.8-3.8 1 0 1.9.3 2.6 1M9.6 9.8h4.8M12.4 7.8l2 2-2 2"/></symbol>' +
    '<symbol id="tki-status" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.6"/><path d="M8 2.4a5.6 5.6 0 0 1 0 11.2z" fill="currentColor"/></symbol>' +
    '<symbol id="tki-assign" viewBox="0 0 16 16"><circle cx="6.4" cy="5" r="2.4"/><path d="M2 13.6c.4-2.8 2.2-4 4.4-4s4 1.2 4.4 4M12.6 4.6v4M10.6 6.6h4"/></symbol>' +
    '<symbol id="tki-withdraw" viewBox="0 0 16 16"><path d="M3.2 8h9.6M6.4 4.8L3.2 8l3.2 3.2"/></symbol>' +
    '<symbol id="tki-warn" viewBox="0 0 16 16"><path d="M8 2.4l6 10.8H2z"/><path d="M8 6.6v3M8 11.4v.05"/></symbol>' +
    '<symbol id="tki-search" viewBox="0 0 16 16"><circle cx="7" cy="7" r="4.4"/><path d="M10.3 10.3l3.4 3.4"/></symbol>' +
    '<symbol id="tki-list" viewBox="0 0 16 16"><path d="M5.6 4h8M5.6 8h8M5.6 12h8M2.3 4h.1M2.3 8h.1M2.3 12h.1"/></symbol>' +
    '<symbol id="tki-tree" viewBox="0 0 16 16"><path d="M2.4 3.4h6M4.6 3.4v9.2M4.6 8h2.6M4.6 12.6h2.6M9 8h4.6M9 12.6h4.6"/></symbol>' +
    '<symbol id="tki-kanban" viewBox="0 0 16 16"><rect x="2" y="3" width="3.2" height="10" rx=".8"/><rect x="6.4" y="3" width="3.2" height="6.4" rx=".8"/><rect x="10.8" y="3" width="3.2" height="8.2" rx=".8"/></symbol>' +
    '<symbol id="tki-timeline" viewBox="0 0 16 16"><path d="M2 2.4v11.2M4.6 4.6h5M7.2 8h6.4M4.6 11.4h4"/></symbol>' +
    '<symbol id="tki-calendar" viewBox="0 0 16 16"><rect x="2.2" y="3.4" width="11.6" height="10.2" rx="1.6"/><path d="M2.2 6.8h11.6M5.4 2v2.8M10.6 2v2.8"/></symbol>' +
    '<symbol id="tki-bell" viewBox="0 0 16 16"><path d="M8 2.6a3.6 3.6 0 0 0-3.6 3.6v2.4L3 10.8h10l-1.4-2.2V6.2A3.6 3.6 0 0 0 8 2.6zM6.6 12.8a1.5 1.5 0 0 0 2.8 0"/></symbol>' +
    '<symbol id="tki-team" viewBox="0 0 16 16"><circle cx="5.6" cy="5.4" r="2.1"/><path d="M1.8 13c.3-2.4 1.8-3.6 3.8-3.6s3.5 1.2 3.8 3.6"/><circle cx="11.2" cy="6" r="1.7"/><path d="M10.6 9.6c1.8-.1 3.2.9 3.6 3"/></symbol>' +
    '<symbol id="tki-activity" viewBox="0 0 16 16"><path d="M1.8 8h3l1.6-4.6 3 9 1.6-4.4h3.2"/></symbol>' +
    '<symbol id="tki-more" viewBox="0 0 16 16"><circle cx="3.4" cy="8" r="1.3" fill="currentColor" stroke="none"/><circle cx="8" cy="8" r="1.3" fill="currentColor" stroke="none"/><circle cx="12.6" cy="8" r="1.3" fill="currentColor" stroke="none"/></symbol>' +
    '<symbol id="tki-send" viewBox="0 0 16 16"><path d="M2.5 8L13.5 2.8 10.6 13.4 7.6 9.1z"/><path d="M7.6 9.1l5.9-6.3"/></symbol>' +
    '<symbol id="tki-flag" viewBox="0 0 16 16"><path d="M3.6 14V2.6M3.6 3h8.4l-1.8 3 1.8 3H3.6"/></symbol>' +
    '<symbol id="tki-tag" viewBox="0 0 16 16"><path d="M2.6 2.6h5.2l5.8 5.8-5.2 5.2-5.8-5.8z"/><circle cx="5.6" cy="5.6" r="1.1"/></symbol>' +
    '<symbol id="tki-pen" viewBox="0 0 16 16"><path d="M10.6 2.8l2.6 2.6-7.6 7.6-3.2.6.6-3.2z"/></symbol>' +
    '<symbol id="tki-copy" viewBox="0 0 16 16"><rect x="5.5" y="5.5" width="8" height="8" rx="1.2"/><path d="M10.5 3.5v-.3a.7.7 0 0 0-.7-.7H3.2a.7.7 0 0 0-.7.7v6.6a.7.7 0 0 0 .7.7h.3"/></symbol>' +
    '<symbol id="tki-page" viewBox="0 0 16 16"><path d="M9.5 2.5h4v4M13.5 2.5L8 8M11.5 9.5v3.2a.8.8 0 0 1-.8.8H3.3a.8.8 0 0 1-.8-.8V5.3a.8.8 0 0 1 .8-.8h3.2"/></symbol>' +
    // the wiki (cardWiki)
    '<symbol id="tki-book" viewBox="0 0 16 16"><path d="M8 4.2C6.6 3 4.6 2.6 2.4 2.8v9.6c2.2-.2 4.2.2 5.6 1.4 1.4-1.2 3.4-1.6 5.6-1.4V2.8C11.4 2.6 9.4 3 8 4.2zM8 4.2v9.6"/></symbol>' +
    '<symbol id="tki-doc" viewBox="0 0 16 16"><path d="M4 2.4h5.2L12 5.2v8.4H4z"/><path d="M9 2.6v2.8h2.8M6 8.4h4M6 10.8h4"/></symbol>' +
    '<symbol id="tki-folder" viewBox="0 0 16 16"><path d="M2 4.2h4l1.4 1.6H14v6.7H2z"/></symbol>' +
    '<symbol id="tki-sidebar" viewBox="0 0 16 16"><rect x="2.2" y="2.8" width="11.6" height="10.4" rx="1.4"/><path d="M6.2 2.8v10.4"/></symbol>' +
    '<symbol id="tki-history" viewBox="0 0 16 16"><path d="M2.8 8a5.2 5.2 0 1 0 1.6-3.8"/><path d="M2.6 2.6v2.6h2.6M8 5.2V8l2 1.4"/></symbol>' +
    '<symbol id="tki-link" viewBox="0 0 16 16"><path d="M6.8 9.2a2.6 2.6 0 0 0 3.7 0l2-2a2.6 2.6 0 0 0-3.7-3.7l-.8.8M9.2 6.8a2.6 2.6 0 0 0-3.7 0l-2 2a2.6 2.6 0 0 0 3.7 3.7l.8-.8"/></symbol>' +
    '<symbol id="tki-reload" viewBox="0 0 16 16"><path d="M13 8a5 5 0 1 1-1.5-3.6"/><path d="M12 2v2.8H9.2"/></symbol>' +
    '</svg>';

  function el(tag, props, html) {
    var n = document.createElement(tag);
    Object.keys(props).forEach(function (k) { n.setAttribute(k, props[k]); });
    if (html) n.innerHTML = html;
    return n;
  }
  function buildDom() {
    var b = document.body;
    b.appendChild(el('div', { class: 'tk-sprite', 'aria-hidden': 'true' }, SPRITE));
    b.appendChild(el('div', { class: 'tk tk-drawer', id: 'tkdrawer' },
      '<div class="tk-dr-in"><div class="tk-dr-clip" id="tkclip"><div class="tk-dr-sheet" role="region" aria-label="' + wsT("js.tk.aria.store") + '"><div class="tk-dr-body" id="tkbody"></div></div></div>' +
      '<div class="tk-handles" id="tkhandles"></div></div>'));
    O.pageParent.appendChild(el('main', { class: 'tk tk-spage', id: 'tkpage', role: 'region', 'aria-label': wsT("js.tk.aria.page"), hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-wkframe', id: 'tkframe', hidden: '' })); // an HTML wiki page's frame (wkFrameSync)
    b.appendChild(el('div', { class: 'tk tk-iwin-back', id: 'tkwin', hidden: '' }, '<div class="iwin" role="dialog" aria-modal="true" tabindex="-1"><div class="iwin-in" id="tkwinbody"></div></div>' +
      '<ul class="tk-mpop" id="tkmpop" role="listbox" aria-label="' + wsT("js.tk.composer.mention.tip") + '" hidden></ul>'));
    b.appendChild(el('dialog', { class: 'tk tk-dlg', id: 'tkdlg' }));
    // a page without the shared toast stack (the tracker mockup) gets the renderer's own
    if (!window.WsToast) b.appendChild(el('div', { class: 'tk tk-toast', id: 'tktoast', hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-tip', id: 'tktip', role: 'tooltip', hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-menu', id: 'tkmenu', role: 'menu', hidden: '' }));
    // the range and label pickers' discard capsule: outside the menu, which clips its children
    b.appendChild(el('button', { type: 'button', class: 'tk tk-mcap', id: 'tkcap', hidden: '' }, '<svg class="i"><use href="#tki-trash"/></svg><span>' + wsT("js.tk.menu.discard") + '</span>'));
    if (O.qsSlot && !O.searchHost) O.qsSlot.appendChild(el('span', { class: 'tk tk-qswrap' },
      '<button class="btn tk-qsbtn" id="tkqs" title="' + wsT("js.tk.qs.tip") + ' (Ctrl+K / Ctrl+Space / /)" aria-label="' + wsT("js.tk.qs.tip") + '" aria-controls="tkqpop" aria-expanded="false"><svg class="i"><use href="#tki-search"/></svg></button>'));
    (O.searchHost ? O.searchHost.parent : b).appendChild(el('section', { class: 'tk tk-palette dr-search-palette', id: 'tkqpop', role: 'region', 'aria-label': wsT("js.tk.qs.tip"), hidden: '' },
      '<label class="tk-qlabel" for="tkqin">' + wsT("js.tk.qs.tip") + '</label>' +
      '<div class="tk-qrow"><input id="tkqin" type="text" role="combobox" aria-controls="tkqlist" aria-expanded="true" aria-autocomplete="list" autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.qs.placeholder") + '">' +
      '</div>' +
      '<div class="tk-qmain"><div class="tk-qresults"><div class="tk-qtabs" id="tkqtabs" role="tablist" hidden></div>' +
      '<div class="tk-qgrid" id="tkqlist" role="listbox" aria-label="' + wsT("js.tk.qs.results") + '"></div></div></div>' +
      '<span class="tk-qhint">' + wsT("js.tk.qs.hint") + '</span>'));
  }
  // Delegated handlers act only on the renderer's own nodes, so the host page's clicks,
  // changes, and keys stay the host's.
  function own(t) { return !!(t && t.closest && t.closest('.tk')); }
  // the editor (editorHtml's wrapper) a drag is over, when it is one
  function edDrop(e) { return e.target.closest ? e.target.closest('.tked[data-ed]') : null; }
  function hasFiles(e) { return !!e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], 'Files') >= 0; }
  // the files a clipboard or a drop carries
  function filesOf(dt) {
    if (!dt) return [];
    if (dt.items && dt.items.length) return Array.prototype.map.call(dt.items, function (it) { return it.kind === 'file' ? it.getAsFile() : null; }).filter(Boolean);
    return Array.prototype.slice.call(dt.files || []);
  }

  // A store image (an attachment, or a wiki's image) fails while its store is busy: the daemon
  // answers 503 while a fetch or push holds the store, and a script never sees an image's
  // status. A failed one loads again after IMG_RETRY_MS, with a query the daemon ignores so
  // the browser does not reuse the failure, up to IMG_RETRIES times per element; an element a
  // redraw replaced meanwhile is left alone.
  var IMG_RETRY_MS = 1500, IMG_RETRIES = 3;
  var IMG_ROUTE = /^\/tracker\/stores\/[^/]+\/(wiki\/[^/]+\/)?attachments\/[^/]+$/;
  function imgRetry(e) {
    var img = e.target;
    if (!img || img.tagName !== 'IMG') return;
    var u;
    try { u = new URL(img.src, location.href); } catch (x) { return; }
    if (u.origin !== location.origin || !IMG_ROUTE.test(u.pathname)) return;
    var n = +(img.getAttribute('data-tk-retry') || 0);
    if (n >= IMG_RETRIES) return;
    img.setAttribute('data-tk-retry', String(n + 1));
    setTimeout(function () {
      if (!img.isConnected) return;
      u.searchParams.set('retry', String(n + 1));
      img.src = u.pathname + u.search;
    }, IMG_RETRY_MS);
  }

  function listen() {
    document.addEventListener('click', function (e) {
      var t = e.target;
      if (qs.open && !(t.closest && t.closest('#tkqpop, #tkqs')) && !(O.searchHost && O.searchHost.contains(t))) qClose();
      if (mentionBox(t)) mentionSync(); // a click into an @token opens it, elsewhere in the box closes it
      else if (MP && !(t.closest && t.closest('#tkmpop, #cmention'))) mentionClose();
      // a click outside the menu closes it; a click on its own opener toggles it shut (menuOutside)
      if (menuOutside(e, t)) return;
      if (!own(t)) return;
      if (t === $('tkwin')) { closeItemWindow(); return; } // the backdrop
      if (t.closest('#tkqs')) { if (qs.open) qClose(); else qOpen(); return; }
      var qo = t.closest('[data-qi]'), qh = t.closest('#tkqpop [data-qslot]');
      if (qo) {
        var qk = qo.getAttribute('data-qs'), qi = Number(qo.getAttribute('data-qi'));
        qPickEntry({ slot: qk, row: qs.S[qk] && qs.S[qk].rows[qi] }, e.ctrlKey || e.metaKey);
        return;
      }
      if (qh) { qs.cur = qh.getAttribute('data-qslot'); qs.sel = {}; qRender(); $('tkqin').focus(); return; }
      var h = t.closest('.tk-handle[data-handle]');
      if (h) { openStore(h.getAttribute('data-handle')); return; }
      var fb = t.closest('.tk-fold[data-fold]');
      if (fb) { foldStore(fb.getAttribute('data-fold')); return; }
      // a wiki: link in rendered text (the daemon marks it data-wiki / data-page) opens that page in the panel
      var wl = t.closest('a[data-wiki][data-page]');
      if (wl && AD.wiki) { e.preventDefault(); wkOpenLink(wl.getAttribute('data-wiki'), wl.getAttribute('data-page')); return; }
      // the wiki's menus close on a click elsewhere in the tracker
      if (AD.wiki && ui.store && wk().pick && !t.closest('.wk-pickw')) { wk().pick = false; render(); }
      if (AD.wiki && ui.store && wk().more && !t.closest('.wk-more')) { wk().more = false; render(); }
      var a = t.closest('[data-act]');
      if (a) {
        e.preventDefault(); e.stopPropagation();
        var k = a.getAttribute('data-act'), d = { item: a.getAttribute('data-item'), who: a.getAttribute('data-who'), op: a.getAttribute('data-op'), req: a.getAttribute('data-req'), cid: a.getAttribute('data-cid'), label: a.getAttribute('data-label') };
        // selection mode: a row part's picker (partHtml) gives way to the row's own click, as the part did before it had one
        var prow = mselOn() && a.classList.contains('rp') && a.closest('#card-items .irow');
        if (prow) mselClick(prow, e);
        else if (k.indexOf('wk-') === 0) wkAct(k, a);
        else if (k === 'win-close') closeItemWindow();
        else if (k === 'detail-retry' && AD.retryDetail) AD.retryDetail(ui.store, ui.item);
        else if (k === 'history-retry' && AD.retryHistory) AD.retryHistory(ui.store);
        else if (k === 'page-back') leavePage();
        else if (k === 'afold') { ui.aopen = ui.aopen === ui.item ? null : ui.item; render(); }
        else if (k === 'menu') itemMenu(d.item, a);
        else if (k === 'mstatus') itemMenu(d.item, a, null, { statusOnly: true });
        else if (k.indexOf('m-') === 0) { if (MN) menuAct(k, a); }
        else if (STEPS[k]) itemMenu(d.item, a, null, { step: k, op: d.op, who: d.who, req: d.req });
        else if (k === 'edit') { closeMenu(false); startEdit(d.item); }
        else if (k === 'iedit-save') saveEdit();
        else if (k === 'ilabel' && ui.iedit) { var il = ui.iedit.labels, ix = il.indexOf(d.label); if (ix >= 0) il.splice(ix, 1); else il.push(d.label); keepItemDraft(); render(); }
        else if (k === 'lsearch') labelSearch(d.label);
        else if (k === 'tedit') startRowEdit(d.item, (a.closest('.irow') || a).getAttribute('data-rk'));
        else if (k === 'claimf') claimableFilter();
        else if (k === 'slabel-rm') labelRemove(d.label);
        else if (k === 'iedit-x' || k === 'iedit-discard') endEdit();
        else if (k === 'cedit') startCedit(d.cid);
        else if (k === 'cedit-save') saveCedit();
        else if (k === 'cedit-x') endCedit();
        else if (k === 'create') openNewRow();
        else if (k === 'ins') insOpen();
        else if (k === 'msel') { var mr = a.closest('.irow'); if (mr) mselClick(mr, e); }
        else if (k === 'msel-clear') { mselClear(); render(); }
        else if (k === 'mdet') { var mk = ui.store + '|' + d.who; ui.mdet[mk] = !ui.mdet[mk]; render(); }
        else if (k === 'ofold') { var ok = ui.store + '|' + d.who; ui.ofold[ok] = !ui.ofold[ok]; render(); }
        else if (k === 'org-edit') { /* Phase 2 opens the org-chart editor modal */ }
        else if (k === 'idcopy') copyId(a);
        else if (k === 'new-x') closeNewRow();
        else if (k === 'nchip') newMenu(a.getAttribute('data-step'), a, a.hasAttribute('data-ed'));
        else if (k === 'ned-x') endRowEdit();
        else if (k === 'nkind') { var nk0 = ui.nrow[ui.store]; if (nk0) nrowKind(nk0.kind === 'topic' ? 'task' : 'topic'); }
        else if (k === 'knew') openKnew();
        else if (k === 'knew-x') closeKnew();
        else if (k === 'sub-x') closeSub();
        else if (k === 'rib') openMenu({ kind: 'sync' }, a);
        else if (k === 'actf') { var af = a.getAttribute('data-f'); ui.act[af] = !ui.act[af]; render(); }
        else if (k === 'tab') { tabState()[ui.layout === 'narrow' ? 'narrow' : 'side'] = a.getAttribute('data-tab'); ui.sel = null; render(); }
        else if (k === 'today') {
          var ps = panelState(), set = a.getAttribute('data-set');
          ps.today = ps.today === set ? null : set;
          if (ps.today && !scopeHasToday(set, scopeOf(), panelMode(scopeOf()), memberOf(viewerOf(ui.store)))) ps.scope = TODAY_HOME[set];
          if (ui.layout === 'narrow') tabState().narrow = 'items';
          ui.sel = null; render();
        }
        else if (k === 'dlg-cancel') closeDialog();
        else if (k === 'open') showItem(d.item);
        else if (k === 'bref') showItem(d.item, a, d.cid);
        else if (k === 'tdrill') { // a team head's count: drill to that person's rows of it; the same count again, or the clear control, clears it
          var dps = panelState(), dset = a.getAttribute('data-set');
          dps.drill = !dset || (dps.drill && dps.drill.who === d.who && dps.drill.key === dset) ? null : { who: d.who, key: dset };
          ui.sel = null; render();
        }
        else if (k === 'scope') { panelState().scope = a.getAttribute('data-scope'); ui.sel = null; render(); }
        else if (k === 'pmode') { panelState().modes[scopeOf()] = a.getAttribute('data-mode'); dsSave('panel.modes', ui.pmodes); render(); }
        else if (k === 'rel-fold') { var rfs = panelState(), rk0 = a.getAttribute('data-set'); rfs.relFold = rfs.relFold || {}; rfs.relFold[rk0] = !rfs.relFold[rk0]; render(); }
        else if (k === 'closed-toggle') { var cps = panelState(); cps.closedOpen = !cps.closedOpen; ui.closedOpen[ui.store] = cps.closedOpen; dsSave('closed', ui.closedOpen); render(); }
        else if (k === 'tree-closed') { var to = panelState().treeOpen, pid = a.getAttribute('data-parent'); to[pid] = !to[pid]; render(); }
        else if (k === 'tfold') { var fk = ui.store + '|' + a.getAttribute('data-node'), fto = a.getAttribute('data-to'); if (fto) ui.tfold[fk] = fto; else delete ui.tfold[fk]; dsSave('tree.fold', ui.tfold); render(); }
        else if (k === 'wfold') { ui.wfold[ui.store + '|' + a.getAttribute('data-node')] = a.getAttribute('aria-expanded') !== 'true'; render(); }
        else if (k === 'psearch-clear') { panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); }
        else if (k === 'cmode') { (ui.cmodes[ui.store] = ui.cmodes[ui.store] || {})[a.getAttribute('data-card')] = a.getAttribute('data-mode'); dsSave('card.modes', ui.cmodes); render(); }
        else if (k === 'cal') { ui.calMonth[a.getAttribute('data-card')] = a.getAttribute('data-month'); render(); }
        else if (k === 'sync') { syncNow(ui.store, false, function () { render(); }); }
        else if (k === 'reply') { ui.replyTo = d.cid; delete ui.edpv.cbox; render(); if ($('cbox')) $('cbox').focus(); }
        else if (k === 'reply-x') { ui.replyTo = null; render(); }
        else if (k === 'mention') mentionInsert();
        else if (k === 'ed-mode') edMode(a.getAttribute('data-ed'), a.getAttribute('data-mode'));
        else if (k === 'hstatus') setStatus(VS.items[d.item], a.getAttribute('data-status'), '');
        else if (k === 'hsub') startSub(d.item, 'iw:' + d.item);
        else if (k === 'asg-add') openAssign(d.item, a);
        else if (k === 'nread-all') { markRead(feedEntries(memberOf(viewerOf(ui.store))).map(function (x) { return x.key; })); render(); }
        else if (k === 'att-load') { if (CAPS.attach) { var lm = (ui.loaded[seenKey(ui.store)] = ui.loaded[seenKey(ui.store)] || {}), oid = a.getAttribute('data-oid'); lm[oid] = !lm[oid]; render(); } }
        else if (k === 'att-add') {
          var an = $('catt-name').value.trim(), asz = $('catt-size').value;
          if (!an || asz === '') return toast(wsT("js.tk.toast.attneeds"));
          ui.adraft.push({ oid: mockOid(an + '|' + asz), name: an, size: Number(asz), mime: mimeOf(an) }); render();
        }
        else if (k === 'att-x') { ui.adraft.splice(Number(a.getAttribute('data-idx')), 1); render(); }
        else if (k === 'register') openRegister();
        else if (k === 'store-menu') openMenu({ kind: 'store', store: a.getAttribute('data-store') }, a);
        else if (k === 'store-show') showStore(a.getAttribute('data-store'));
        else if (k === 'store-unreg') openDialog('unregister', { store: a.getAttribute('data-store') });
        else if (QUICK[k]) quick(k, a);
        else if (DIALOGS[k] || (X.dialogs && X.dialogs[k])) openDialog(k, d);
        else if (X.action) X.action(k, a, H);
        return;
      }
      // selection mode: a click on a list row toggles it, Shift adds a range (mselClick), rather than opening the item window
      var mrow = mselOn() && t.closest('#card-items .irow');
      if (mrow) { e.preventDefault(); mselClick(mrow, e); return; }
      // touch: a tap selects a row, which shows its actions and details as a hover would;
      // a tap on its title opens the item window
      var row = t.closest('.irow');
      if (row && lastPointer === 'touch' && !t.closest('a.title') && ui.sel !== row.getAttribute('data-rk')) {
        e.preventDefault(); ui.sel = row.getAttribute('data-rk'); render(); return;
      }
      if (row && !rowClick(row, t)) { e.preventDefault(); return; }
      var nk = t.closest('[data-nkey]');
      if (nk) markRead([nk.getAttribute('data-nkey')]); // a feed entry is read once clicked
      var it = clickItem(t);
      if (it) { e.preventDefault(); showItem(it.getAttribute('data-item'), t, nk && nk.getAttribute('data-nev')); return; }
      if (t.closest('a[href="#"]')) e.preventDefault();
      // a footnote link in rendered text scrolls to its note; following it would set the
      // hash, which is the store-page route (applyRoute)
      var fn = t.closest('.md a[href^="#"]');
      if (fn) { e.preventDefault(); var to = document.getElementById(decodeURIComponent(fn.getAttribute('href').slice(1))); if (to) to.scrollIntoView({ block: 'nearest' }); }
    });
    document.addEventListener('submit', function (e) {
      var f = e.target;
      if (f.closest && f.closest('#tkdlg, #tkmenu')) { e.preventDefault(); submit(f); }
      else if (f.closest && f.closest('.wk-form')) { e.preventDefault(); wkCreate(); }
    });
    // the search box filters as you type; the render rebuilds the panel, so focus and the caret are put back (see render),
    // and an IME composition (Korean, Japanese) is left alone until it commits
    function searchInput(e) {
      if (e.isComposing || !own(e.target)) return;
      if (e.target.id === 'psearch') { panelState().q = e.target.value; render(); }
      else if (e.target.id === 'wksearch') { wk().q = e.target.value; render(); }
      else if (e.target.id === 'wk-wname') wkNameInput(e.target);
    }
    document.addEventListener('input', searchInput);
    document.addEventListener('compositionend', searchInput);
    document.addEventListener('change', function (e) {
      var t = e.target;
      if (!own(t)) return;
      if (t.id === 'f-kind') { ui.kind = t.value; render(); }
      else if (t.id === 'f-status') { ui.status = t.value; render(); }
      else if (t.id === 'a-family') { ui.act.family = t.value; render(); }
      else if (t.id === 'a-member') { ui.act.member = t.value; render(); }
      else if (t.closest('#tkdlg form')) {
        var df = t.closest('form');
        if (t.name === 'saved') { // picking a saved profile fills the fields
          var o = t.options[t.selectedIndex], sp = o && o.getAttribute('data-p');
          if (sp) { var pr = JSON.parse(sp); df.elements.org.value = pr.org; df.elements.family.value = pr.family; df.elements.given.value = pr.given; if (df.elements.nid) df.elements.nid.value = asciiId(pr.given); }
        }
        if (t.name === 'rtype') rtypeShow(df);
        profileView(df);
      }
    });
    // the inline inputs keep their text in ui, so a re-render draws it back
    document.addEventListener('input', function (e) {
      var t = e.target;
      if (!own(t)) return;
      if (t.id === 'tknew' && ui.nrow[ui.store]) ui.nrow[ui.store].val = t.value;
      else if (t.id === 'tkknew' && ui.knew[ui.store]) ui.knew[ui.store].val = t.value;
      else if (t.id === 'tksub' && ui.sub) ui.sub.val = t.value;
      else if (t.id === 'tkned' && ui.ned) ui.ned.val = t.value;
      else if (t.id === 'tkaq' && MN && MN.step === 'assign') { var as = asgState(); as.q = t.value; as.idx = 0; asgDraw(); }
      else if (t.id === 'tkmq' && MN) { MN.q = t.value; var ml = $('tkmenu').querySelector('.mlist'); if (ml) ml.innerHTML = mqList(VS.items[MN.item]); }
      else if (t.id === 'tkmvq' && MN && MN.mv) { MN.q = t.value; var mvl = $('tkmenu').querySelector('.mlist'); if (mvl) mvl.innerHTML = moveList(MN.mv); }
      else edKeep(t);
      if (mentionBox(t)) mentionSync(); // also mid-composition: the popup filters on the composing text
    });
    document.addEventListener('input', function (e) {
      var t = e.target, df = t.closest && t.closest('#tkdlg form');
      if (!df || !df.elements.given) return;
      if (df.elements.saved && PROFILE_FIELDS.indexOf(t.name) >= 0) df.elements.saved.value = '';
      // the register dialog's member id follows the given name until it is typed in
      if (df.getAttribute('data-kind') === 'register' && df.elements.nid) {
        if (t.name === 'nid') df.elements.nid.removeAttribute('data-auto');
        else if (t.name === 'given' && df.elements.nid.hasAttribute('data-auto')) df.elements.nid.value = asciiId(t.value);
      }
      profileView(df);
    });
    // typing into the last priority row opens the next blank one, up to PRIO_MAX
    document.addEventListener('input', function (e) {
      var pf = e.target.closest && e.target.closest('#tkdlg form[data-kind="sprio"]');
      if (!pf || !/^pname\d+$/.test(e.target.name) || !e.target.value.trim()) return;
      var pn = 0;
      while (pf.elements['pname' + pn]) pn++;
      if (e.target.name !== 'pname' + (pn - 1) || pn >= PRIO_MAX) return;
      e.target.parentNode.insertAdjacentHTML('afterend', prioRow(pn, '', false));
    });
    document.addEventListener('input', function (e) {
      var f = e.target.closest && e.target.closest('#tkdlg form[data-kind="unregister"]');
      if (f) unregReady(f);
    });
    document.addEventListener('dragstart', function (e) {
      var c = own(e.target) && e.target.closest('[data-drag]');
      if (!c) return;
      e.dataTransfer.setData('text/plain', c.getAttribute('data-drag'));
      e.dataTransfer.effectAllowed = 'move';
    });
    document.addEventListener('dragover', function (e) {
      if (own(e.target) && (e.target.closest('[data-kcol]') || (edDrop(e) && hasFiles(e)))) e.preventDefault();
    });
    // the list's move drag (mvStart): its own listeners, apart from the kanban's above
    document.addEventListener('dragstart', function (e) {
      var row = own(e.target) && e.target.closest && e.target.closest('#card-items .irow[draggable="true"]');
      if (row && VS) mvStart(e, row);
    });
    document.addEventListener('dragover', mvOver);
    document.addEventListener('dragleave', function (e) { if (MVS && !(e.relatedTarget && MVS.list.contains(e.relatedTarget))) mvHide(); });
    document.addEventListener('drop', mvDropped);
    document.addEventListener('dragend', function () { if (MV) mvEnd(); });
    // an image pasted into an editor's textarea, or dropped on an editor (edImages); a paste that
    // also carries text (a copied web page) still pastes its text
    document.addEventListener('paste', function (e) {
      var t = mentionBox(e.target), files = t ? filesOf(e.clipboardData) : [];
      if (!files.length) return;
      if (Array.prototype.indexOf.call(e.clipboardData.types || [], 'text/plain') < 0) e.preventDefault();
      edImages(t.id, files, false);
    });
    document.addEventListener('drop', function (e) {
      var wrap = own(e.target) && edDrop(e);
      if (!wrap || !hasFiles(e)) return;
      e.preventDefault();
      edImages(wrap.getAttribute('data-ed'), filesOf(e.dataTransfer), true);
    });
    document.addEventListener('drop', function (e) {
      var col = own(e.target) && e.target.closest('[data-kcol]');
      if (!col) return;
      e.preventDefault();
      var item = e.dataTransfer.getData('text/plain');
      if (item) dropOn(item, col.getAttribute('data-kcol'));
    });
    document.addEventListener('error', imgRetry, true); // an image's error does not bubble
    document.addEventListener('pointerdown', function (e) { lastPointer = e.pointerType || 'mouse'; lastInput = 'pointer'; }, true);
    document.addEventListener('mouseover', function (e) {
      var tt = own(e.target) && e.target.closest('[data-tip]');
      if (tt && tt !== tipFor) showTip(tt); else if (!tt && tipFor) hideTip();
      var row = own(e.target) && e.target.closest('.irow');
      if (row && !row.contains(e.relatedTarget)) placeMore(row);
      linkHi(own(e.target) && e.target.closest('.kcard, .citem'));
    });
    document.addEventListener('focusin', function (e) {
      if (MP && e.target.id !== MP.box) mentionClose();
      var row = own(e.target) && e.target.closest('.irow'), card = own(e.target) && e.target.closest('.kcard, .citem');
      if (row) placeMore(row);
      linkHi(card);
      if (own(e.target) && e.target.matches('button[data-tip]')) showTip(e.target); else if (card) showTip(card); else if (tipFor) hideTip();
    });
    document.addEventListener('mousemove', insMove);
    document.addEventListener('scroll', function (e) {
      if (tipFor) hideTip(); placeShownMore(); insHide();
      if (MP) mentionSync();
      if (MN && MN.kind === 'item' && !(e.target.nodeType === 1 && $('tkmenu').contains(e.target)) && openerMoved()) closeMenu(false); // an item menu does not follow a scrolled row
    }, true);
    // a right-click on a row, card, entry, or the item window header opens its menu at the pointer
    document.addEventListener('contextmenu', function (e) {
      var t = e.target;
      if (!own(t) || t.closest('#tkmenu, #tkdlg, input, textarea, select, a[href]:not([href="#"])')) return;
      var m = t.closest('[data-mitem], [data-item]');
      if (!m) return;
      e.preventDefault();
      itemMenu(m.getAttribute('data-mitem') || m.getAttribute('data-item'), m, { x: e.clientX, y: e.clientY });
    });
    document.addEventListener('mouseout', function (e) { if (!e.relatedTarget && tipFor) hideTip(); });
    document.addEventListener('keydown', function (e) {
      if (!e.defaultPrevented && qShortcut(e)) { e.preventDefault(); e.stopImmediatePropagation(); qOpen(); }
    }, true);
    // Esc from the input is its keydown's (qKey); a close request from elsewhere in the
    // dialog clears then closes the same way, and one the browser forced closes it
    // the list keeps the focus in the input: a click picks or switches without taking it
    $('tkqpop').addEventListener('mousedown', function (e) { if (e.target !== $('tkqin') && e.target.closest('[data-qi], [data-qslot]')) e.preventDefault(); });
    $('tkqin').addEventListener('input', function (e) { if (!e.isComposing) qRun(); });
    $('tkqin').addEventListener('compositionend', function () { qRun(); });
    $('tkqin').addEventListener('keydown', function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      qKey(e);
    });
    // the caret moved without an edit (arrow keys, Home/End): the popup follows the token under it
    document.addEventListener('keyup', function (e) {
      if (mentionBox(e.target) && /^(ArrowLeft|ArrowRight|Home|End)$/.test(e.key)) mentionSync();
    });
    // the assign popover's rows and chip buttons act on click without taking focus from its search field
    $('tkmenu').addEventListener('mousedown', function (e) { if (e.target.closest('.ap-list .aopt, .ap-chips .ax')) e.preventDefault(); });
    // focus leaving the labels step or the range picker applies the pick (menuFocusOut); focus going
    // to the capsule stays inside, and focus leaving the capsule counts as leaving the menu
    $('tkmenu').addEventListener('focusout', menuFocusOut);
    $('tkcap').addEventListener('focusout', menuFocusOut);
    // the capsule discards on its own click (the [data-act] dispatch would apply first) and keeps focus where it is
    $('tkcap').addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); menuDiscard(); });
    $('tkcap').addEventListener('mousedown', function (e) { e.preventDefault(); });
    // the range picker: handle drags, a tap on an empty grid, typed dates
    $('tkmenu').addEventListener('pointerdown', rangeDown);
    $('tkmenu').addEventListener('pointermove', rangeMove);
    $('tkmenu').addEventListener('pointerup', rangeUp);
    $('tkmenu').addEventListener('pointercancel', rangeUp);
    $('tkmenu').addEventListener('input', function (e) { if (e.target.hasAttribute && e.target.hasAttribute('data-rin')) rangeInput(e.target); });
    $('tkmenu').addEventListener('mousemove', function (e) {
      var o = e.target.closest('.aopt[data-ai]'), i = o && Number(o.getAttribute('data-ai'));
      if (!o || !MN || !MN.as || i === MN.as.idx) return;
      MN.as.idx = i;
      Array.prototype.forEach.call($('tkalist').querySelectorAll('.aopt'), function (x) { x.classList.toggle('on', x === o); });
      $('tkaq').setAttribute('aria-activedescendant', o.id);
    });
    // a row is picked on mousedown, so the box keeps focus and its caret
    $('tkmpop').addEventListener('mousedown', function (e) {
      e.preventDefault();
      var o = e.target.closest('[data-mp]');
      if (o) mentionPick(Number(o.getAttribute('data-mp')));
    });
    $('tkmpop').addEventListener('mousemove', function (e) {
      var o = e.target.closest('[data-mp]'), i = o && Number(o.getAttribute('data-mp'));
      if (o && MP && i !== MP.idx) { MP.idx = i; mentionDraw($(MP.box)); }
    });
    $('tkqpop').addEventListener('mousemove', function (e) {
      var o = e.target.closest('[data-qi]');
      if (o) qHover(o);
    });
    document.addEventListener('keydown', function (e) {
      lastInput = 'key';
      var t = e.target, id = t.id, chord = e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !e.isComposing, plain = e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.metaKey && !e.ctrlKey;
      if (MN && menuHas(t)) { menuKey(e); return; }
      // Shift+F10 or the menu key opens the focused row's menu under it
      if ((e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) && own(t) && !t.closest('#tkdlg, input, textarea, select')) {
        var mk = t.closest('[data-mitem], [data-item]');
        if (mk) { e.preventDefault(); itemMenu(mk.getAttribute('data-mitem') || mk.getAttribute('data-item'), t.closest('[data-rk]') && !t.matches('button') ? t.closest('[data-rk]') : t); }
        return;
      }
      // an editor showing its preview (focus on its toggle or the pane): Cmd/Ctrl+Enter saves or
      // sends as in its textarea, and Esc goes back to writing rather than closing the window
      var edw = own(t) && t.closest && t.closest('.tked-tabs, .tked-pv') && t.closest('.tked'), eid = edw && edw.getAttribute('data-ed');
      if (eid && ui.edpv[eid] && (chord || e.key === 'Escape')) {
        e.preventDefault();
        if (e.key === 'Escape') edMode(eid, 'write');
        else if (eid === 'iw-ebody') saveEdit();
        else if (eid === 'tkcedit') saveCedit();
        else if (eid === 'wk-ebody') wkSave();
        else if (eid === 'cbox' && $('tkcsend')) quick('comment', $('tkcsend'));
        return;
      }
      // the inline inputs: Enter (or Cmd/Ctrl+Enter in a multi-line field) writes, Esc leaves
      if (own(t) && t.hasAttribute && t.hasAttribute('data-keep')) {
        var esc1 = e.key === 'Escape';
        if (id === 'tknew') { if (plain) { e.preventDefault(); createNew(); } else if (esc1) { e.preventDefault(); closeNewRow(); } return; }
        if (id === 'tkknew') { if (plain) { e.preventDefault(); createKnew(); } else if (esc1) { e.preventDefault(); closeKnew(); } return; }
        if (id === 'tkned') { if (plain) { e.preventDefault(); saveRowEdit(); } else if (esc1) { e.preventDefault(); endRowEdit(); } return; }
        if (id === 'tksub') { if (plain) { e.preventDefault(); createSub(); } else if (esc1) { e.preventDefault(); closeSub(); } return; }
        if (id === 'iw-etitle' || id === 'iw-eproj') { if (plain || chord) { e.preventDefault(); saveEdit(); } else if (esc1) { e.preventDefault(); endEdit(); } return; }
        if (t.hasAttribute('data-mention')) {
          // an editor: the Enter that commits an IME composition neither picks, saves, nor sends;
          // while the popup is open, its keys come first and Esc closes it, not the editor
          if (e.isComposing || e.keyCode === 229) return;
          var mp = MP && MP.box === id;
          if (mp && MP.ids.length) {
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); MP.idx = (MP.idx + (e.key === 'ArrowDown' ? 1 : MP.ids.length - 1)) % MP.ids.length; mentionDraw(t); return; }
            if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey) { e.preventDefault(); mentionPick(MP.idx); return; }
          }
          if (mp && esc1) { e.preventDefault(); mentionClose(); return; }
        }
        if (id === 'iw-ebody') { if (chord) { e.preventDefault(); saveEdit(); } else if (esc1) { e.preventDefault(); endEdit(); } return; }
        if (id === 'tkcedit') { if (chord) { e.preventDefault(); saveCedit(); } else if (esc1) { e.preventDefault(); endCedit(); } return; }
        if (id === 'wk-ebody') { if (chord) { e.preventDefault(); wkSave(); } else if (esc1) { e.preventDefault(); wkCancel(); } return; }
        if (id === 'wk-epath') { if (plain || chord) { e.preventDefault(); wkSave(); } else if (esc1) { e.preventDefault(); wkCancel(); } return; }
        if (id === 'wk-wname' || id === 'wk-wfirst') { if (plain) { e.preventDefault(); wkCreate(); } else if (esc1 && wk().creating) { e.preventDefault(); wk().creating = false; render(); } return; }
        if (id === 'cbox' && chord) { e.preventDefault(); if ($('tkcsend')) quick('comment', $('tkcsend')); return; }
      }
      // Esc in the search box clears it first; with it empty, Esc goes on to close the window or the drawer
      if (e.key === 'Escape' && e.target.id === 'psearch' && e.target.value && own(e.target)) { e.preventDefault(); panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); return; }
      if (e.key === 'Escape' && e.target.id === 'wksearch' && e.target.value && own(e.target)) { e.preventDefault(); wk().q = ''; render(); if ($('wksearch')) $('wksearch').focus(); return; }
      // Esc closes an open wiki menu before anything else
      if (e.key === 'Escape' && own(t) && AD.wiki && ui.store && (wk().pick || wk().more)) { e.preventDefault(); wk().pick = wk().more = false; render(); return; }
      // keyboard: a focused row shows its actions and details; Enter opens its item window
      if (e.key === 'Enter' && e.target.classList && e.target.classList.contains('irow') && own(e.target)) { e.preventDefault(); showItem(e.target.getAttribute('data-item')); return; }
      // ArrowRight unfolds a focused tree row, ArrowLeft folds it
      if ((e.key === 'ArrowRight' || e.key === 'ArrowLeft') && e.target.classList && e.target.classList.contains('irow') && own(e.target)) {
        var tw = e.target.querySelector(':scope > .ir-main > .tw[data-act]');
        if (tw && (tw.getAttribute('aria-expanded') === 'true') === (e.key === 'ArrowLeft')) { e.preventDefault(); tw.click(); }
        return;
      }
      // a host that handled this Esc itself (its own menu, say) marks it defaultPrevented
      if (e.key !== 'Escape' || e.defaultPrevented || $('tkdlg').open) return;
      if (ui.win) closeItemWindow();
      else if ((ui.open || ui.page) && mselOn()) { e.preventDefault(); mselClear(); render(); }
      else if (ui.open) collapse();
    });
    // the layout follows the width: re-render when it crosses a breakpoint, else refit
    window.addEventListener('resize', function () { if (qs.open) { var qw = qStepOf($('tkqpop').getBoundingClientRect().width); if (qw !== qs.step) { qs.step = qw; qRender(); } } if (MN) closeMenu(false); if (MP) mentionSync(); if (layoutOf() !== ui.layout) render(); else { if (!ui.page) fitDrawer(); measure(); wkFramePlace(); } });
    // Header wrapping and composer resize/display changes need no window resize.
    // Measure outside the observer callback: header padding can itself resize it.
    if (window.ResizeObserver) {
      var drawerFitFrame = 0;
      var drawerObserver = new ResizeObserver(function () {
        if (drawerFitFrame) return;
        drawerFitFrame = requestAnimationFrame(function () {
          drawerFitFrame = 0;
          measure();
          if (!ui.page) fitDrawer();
          wkFramePlace();
        });
      });
      drawerObserver.observe(O.header);
      if (O.drawerBoundary) drawerObserver.observe(O.drawerBoundary);
    }
    window.addEventListener('scroll', wkFramePlace, true); // any scroller moving the wiki frame's slot (capture: scroll does not bubble)
    // the store page route: reload on #/<storeId> restores the page; back and forward follow the hash
    window.addEventListener('pagehide', keepItemDraft);
    window.addEventListener('hashchange', applyRoute);
    window.addEventListener('popstate', function () { if (routeStore() !== ui.page) applyRoute(); });
    // the host opened its own drawer: an open store collapses (and so syncs)
    document.addEventListener('ws:drawer-open', function () { if (ui.open) collapse(); });
  }

  function mount(adapter, opts) {
    if (api) return api;
    AD = adapter; CAPS = adapter.caps || {}; X = adapter.ext || {}; O = opts || {};
    PFX = O.storagePrefix || 'ws.tracker.';
    ui.cmodes = dsLoad('card.modes'); ui.pmodes = dsLoad('panel.modes'); ui.closedOpen = dsLoad('closed'); ui.tfold = dsLoad('tree.fold');
    mergeTabs();
    H = { esc: esc, attr: attr, btn: btn, card: card, fld: fld, fmtTs: fmtTs, fmtSize: fmtSize, nm: nm, nmo: nmo, memberOpts: memberOpts,
      profileFields: profileFields, profileText: profileText, asciiId: asciiId, crefHtml: crefHtml, uiCommand: uiCommand,
      openDialog: openDialog, closeDialog: closeDialog, wrote: wrote, toast: toast,
      refresh: function (forget) { if (forget) ui.last = null; render(); } };
    buildDom();
    listen();
    api = {
      refresh: function () { render(); },
      openStore: openStore,
      openSearch: qOpen,
      closeSearch: qClose,
      closeStore: function () { if (ui.open) collapse(); },
      openItemWindow: openItemWindow,
      leavePage: leavePage,
      viewerChanged: function (id) {
        ui.last = null;
        if (id === ui.store && viewing()) bringUp(id);
        render();
      },
      toast: toast,
      unread: function () { return UNREADS; },
      openNotice: function (x) { if (x && openItemWindow(x.store, x.item, x.ev ? { focus: x.ev } : null)) markRead([x.key]); },
      openNotify: openNotify
    };
    syncHandles();
    ui.store = order[0] || null;
    // A change notification renders (and re-reads the wiki shown) on the next timer tick, so
    // the notifications that arrive meanwhile land as one redraw. The views of a read of
    // several stores land in separate tasks, a tick apart or more, so a change with more to
    // come waits up to MORE_HOLD for the rest; the last one (or any other change) brings the
    // redraw forward to the next tick, and a store slower than the hold still draws the others
    // first. A timer, not an animation frame: a hidden tab pauses frames, and the handle badges
    // should still follow there.
    var MORE_HOLD = 100, changeAt = 0;
    AD.onChange(function (more) {
      renderDue = true;
      var at = Date.now() + (more ? MORE_HOLD : 0);
      if (changeTimer && changeAt <= at) return;
      clearTimeout(changeTimer);
      changeAt = at;
      changeTimer = setTimeout(function () { changeTimer = 0; wkRefresh(); if (renderDue) render(); openRouteItem(); }, at - Date.now());
    });
    applyRoute();
    // read-only hook for scripted checks (headless verification); not part of the model
    window.__tracker = { ui: ui, vs: function () { return VS; }, openItemWindow: openItemWindow };
    // the entry point a later URL route uses to open an item from a link
    window.openItemWindow = openItemWindow;
    return api;
  }

  window.WsTracker = { mount: mount };
  // searchKey is the palette's shortcut test (qShortcut), for a host that
  // must recognize the key before mount installs the palette's listener.
  window.WsTracker.searchKey = qShortcut;
})();
