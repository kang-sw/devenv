  // ------------------------------------------------------------ context menu

  // One context menu serves every item: list rows, kanban cards, calendar entries, gantt rows,
  // and the item window header open it alike, by right-click, the "⋯" button, or Shift+F10 /
  // the menu key on a focused row; a click on a status icon opens its status part alone. It is
  // one fixed panel (#tkmenu) over everything, the item window included. MN is its state:
  //   { kind: 'item' | 'sync', item, step, op, who, from (the step it came from, for Back),
  //     statusOnly, back (selector of the opener) and backEl (the opener), rk (the opener's row key,
  //     where "add subtask" puts its input row), pt (the pointer, for a right-click) or rect, pick }
  // The item menu's steps: 'main' (status radios, note, and the actions the viewer may take),
  // 'pri' and 'due' (priority radios, a date), 'handoff' (a people list), 'note' (decline,
  // reject, reopen, return, deny a take request with a note), 'opening' (open for claiming: the
  // scope), 'request' (a take request: whose assignment, with a note), 'assign' (the assign form: the anchored assign
  // popover). 'sync' is the Items card ribbon's status popover; 'store' is the store menu of a
  // store's title row (MN.store: its id; see storeMenuHtml). Arrow keys move, Enter or
  // Space applies, ArrowRight opens a step, ArrowLeft or Esc goes back, Esc on the first step
  // closes it, and focus returns to the opener; Tab cycles inside it.
  var MN = null;
  function openMenu(o, anchor, pt) {
    if (MN && !menuApplyClose(false)) return; // a menu opened over another closes it first, applying its pick
    MN = o; MN.step = MN.step || 'main';
    MN.backEl = anchor || null; MN.back = anchor ? returnSel(anchor) : null;
    var rk = anchor && anchor.closest('[data-rk]');
    MN.rk = rk ? rk.getAttribute('data-rk') : null;
    MN.pt = pt || null; MN.rect = anchor ? anchor.getBoundingClientRect() : null;
    hideTip();
    if (anchor && anchor.hasAttribute('aria-haspopup')) anchor.setAttribute('aria-expanded', 'true');
    document.body.classList.add('tk-menuon');
    drawMenu();
  }
  // opts: { step, op, who, statusOnly }; a step opened from inside the menu keeps its place
  function itemMenu(id, anchor, pt, opts) {
    if (!VS || !VS.items[id]) return;
    opts = opts || {};
    if (MN && MN.kind === 'item' && anchor && $('tkmenu').contains(anchor)) {
      MN.from = MN.step; MN.step = opts.step || 'main'; MN.op = opts.op || null; MN.who = opts.who || null; MN.req = opts.req || null; MN.q = ''; MN.as = null; MN.mv = null;
      drawMenu(); return;
    }
    openMenu({ kind: 'item', item: id, step: opts.step || 'main', op: opts.op || null, who: opts.who || null, req: opts.req || null, statusOnly: !!opts.statusOnly, from: null, q: '' }, anchor, pt);
  }
  // Whether the item menu's opener left the place the menu was put at: gone, or moved. A render's
  // scroll restore fires a scroll with the redrawn opener where it was, which keeps the menu (the
  // labels step, open across toggles, redraws its list row under it).
  function openerMoved() {
    var el = MN.backEl, r = el && el.isConnected ? el.getBoundingClientRect() : null;
    return !r || !MN.rect || Math.abs(r.top - MN.rect.top) > 1 || Math.abs(r.left - MN.rect.left) > 1;
  }
  // Every close applies the open picker's pick once: the labels step's toggles, the range picker's
  // ends (an item's as one item.edit, a row's into its draft, which then draws again unless quiet:
  // a close from inside a render). This close cannot stay open, so an inverted range drops unwritten
  // (menuApplyClose refuses it where the menu can stay).
  function closeMenu(refocus, quiet) {
    if (!MN) return;
    var m = MN, el = m.backEl && m.backEl.isConnected ? m.backEl : m.back && document.querySelector(m.back);
    var lw = labelPending(m), rw = rangePending(m); // the picks, written once as the menu closes
    if (rw && rw.bad) rw = null;
    if (m.rg && m.rg.drag) clearInterval(m.rg.drag.tm); // a drag cut short: its edge timer stops too
    MN = null;
    $('tkmenu').hidden = true; $('tkmenu').innerHTML = '';
    $('tkcap').hidden = true;
    document.body.classList.remove('tk-menuon');
    if (m.backEl && m.backEl.hasAttribute('aria-haspopup')) m.backEl.setAttribute('aria-expanded', 'false');
    if (rw && rw.draft) {
      rw.draft.start = m.rg.s; rw.draft.due = m.rg.d;
      if (!quiet) { render(); el = m.back && document.querySelector(m.back); }
    }
    if (refocus && el) {
      if (!el.matches('a[href], button, input, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]');
      if (el) el.focus({ preventScroll: true });
    }
    if (lw) labelWrite(lw);
    if (rw && rw.ev) commit(rw.ev);
  }
  // The back row (and ArrowLeft) of a step opened from the main menu. On the range picker it applies
  // the pick: an inverted one is refused, a changed one written (the written batch closes the item
  // menu), and with nothing changed it steps back.
  function menuBack() {
    if (!MN.from) { closeMenu(true); return; }
    if (MN.step === 'due' && MN.rg) {
      var p = rangePending(MN);
      if (p && p.bad) { rangeRefused(); return; }
      if (p && p.ev) { var m = MN; m.rsent = true; commit(p.ev, null, null, function () { m.rsent = false; }); return; }
    }
    MN.step = MN.from; MN.from = null; MN.op = null; MN.who = null; MN.req = null; MN.q = ''; MN.as = null; MN.mv = null; MN.rg = null;
    drawMenu();
  }
  function drawMenu() {
    var m = $('tkmenu');
    m.innerHTML = MN.kind === 'sync' ? syncPopHtml() : MN.kind === 'new' ? newMenuHtml() : MN.kind === 'store' ? storeMenuHtml() : menuHtml();
    m.className = 'tk tk-menu' + (MN.kind === 'sync' ? ' spopw' : MN.step === 'assign' ? ' wide' : MN.step === 'due' && MN.rg && MN.rg.rm ? ' rwide' : '');
    m.setAttribute('role', MN.kind === 'sync' || MN.step === 'assign' || MN.step === 'due' ? 'dialog' : 'menu');
    m.setAttribute('aria-label', MN.kind === 'sync' ? wsT("js.tk.rib.tip") : MN.kind === 'store' ? wsT("js.tk.store.menu") : wsT("js.tk.menu.aria", mnItem() ? mnItem().title : ''));
    m.hidden = false;
    placeMenu();
    var first = m.querySelector('[data-autofocus]') || m.querySelector('[data-mi]:not([disabled]), button:not([disabled]), input');
    if (first) first.focus({ preventScroll: true });
  }
  // At the pointer for a right-click, else under the opener (above it when below does not fit),
  // kept inside the window.
  function placeMenu() {
    var m = $('tkmenu'), w = m.offsetWidth, h = m.offsetHeight, W = window.innerWidth, Hh = window.innerHeight, x, y;
    if (MN.pt) { x = MN.pt.x; y = MN.pt.y; if (y + h > Hh - 6) y = Math.max(6, MN.pt.y - h); }
    else if (MN.rect) {
      var r = MN.rect;
      x = r.left; y = r.bottom + 4;
      if (y + h > Hh - 6) y = r.top - h - 4 >= 6 ? r.top - h - 4 : Math.max(6, Hh - h - 6);
    } else { x = (W - w) / 2; y = (Hh - h) / 2; }
    x = Math.max(6, Math.min(x, W - w - 6));
    m.style.left = Math.round(x) + 'px'; m.style.top = Math.round(Math.max(6, y)) + 'px';
    placeCap();
  }
  function mi(act, label, data, o) {
    o = o || {};
    return '<button type="button" class="mi' + (o.cls ? ' ' + o.cls : '') + '" role="menuitem" data-mi' + attr('act', act) + dataAttrs(data) + (o.sub ? ' data-sub aria-haspopup="menu"' : '') + '>' +
      (o.icon ? '<svg class="i"><use href="#tki-' + o.icon + '"/></svg>' : '<span class="i"></span>') + '<span class="ml">' + label + '</span>' +
      (o.val ? '<span class="mv">' + o.val + '</span>' : '') + (o.sub ? '<svg class="i mch"><use href="#tki-chev"/></svg>' : '') + '</button>';
  }
  function msep() { return '<div class="msep" role="separator"></div>'; }
  // the priority step's radios (an item's, or the quick-add row's draft), cur the checked level
  function priGroup(cur) {
    return '<div role="group" aria-label="' + wsT("js.tk.menu.pri") + '">' + VS.priority.names.map(function (x, i) {
      var on = i === cur;
      return '<button type="button" class="mi mr" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-pri') + attr('p', i) + (on ? ' data-autofocus' : '') + '><span class="rdo"></span><span class="ml">' + esc(x) + '</span>' +
        (i === VS.priority.default ? '<span class="mv">' + wsT("js.tk.menu.default") + '</span>' : '') + '</button>';
    }).join('') + '</div>';
  }
  function noteField(ph) {
    return '<label class="mnote"><input type="text" id="tkmnote" data-mi autocomplete="off" placeholder="' + esc(ph) + '" aria-label="' + esc(wsT("js.tk.form.note.opt")) + '"><span class="mk" aria-hidden="true">&#9166;</span></label>';
  }
  function menuHtml() {
    var it = VS.items[MN.item], v = VS.viewer.member, h = '';
    if (!it) return '';
    h += '<div class="mh">' + idCopyBtn(it) + '<b>' + esc(it.title) + '</b></div>';
    var back = MN.from ? mi('m-back', wsT("js.tk.menu.back"), {}, { icon: 'back', cls: 'mback' }) + msep() : '';
    var mineR = mineLive(it), step = MN.step;
    if (step === 'pri') return h + back + priGroup(it.priority);
    if (step === 'labels') return h + back + labelStepHtml(it);
    if (step === 'due') return h + back + rangeHtml(rangeOf(MN));
    if (step === 'handoff') {
      var hfrom = MN.who || v, hrec = it.assignments.map(A).filter(function (r) { return r.to === hfrom && r.state === 'live'; })[0] || {};
      return h + back + '<div class="mtitle">' + (hfrom === v ? wsT("js.tk.dlg.handoff.title", esc(it.alias)) : wsT("js.tk.dlg.handoff.title.admin", esc(nm(hfrom)), esc(it.alias))) + '</div>' +
        '<p class="mintro">' + (hfrom === v ? wsT("js.tk.dlg.handoff.intro", esc(nm(hrec.by))) : wsT("js.tk.dlg.handoff.intro.admin", esc(nm(hfrom)), esc(nm(hrec.by)))) + '</p>' +
        '<div class="mrow"><input type="text" id="tkmq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.menu.search") + '" aria-label="' + wsT("js.tk.menu.search") + '" value="' + esc(MN.q || '') + '"></div>' +
        '<div class="mlist" role="group">' + handoffList(it, hfrom) + '</div>' + noteField(wsT("js.tk.form.note.opt"));
    }
    if (step === 'cc') {
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.cc.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.cc.intro") + '</p>' +
        '<div class="mrow"><input type="text" id="tkmq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.menu.search") + '" aria-label="' + wsT("js.tk.menu.search") + '" value="' + esc(MN.q || '') + '"></div>' +
        '<div class="mlist" role="group">' + ccPickList(it) + '</div>';
    }
    if (step === 'opening') {
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.open.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.open.intro") + '</p>' +
        '<div role="group">' + mi('m-opening', esc(wsT("js.tk.open.scope.team")), { scope: 'team' }, { icon: 'team', val: esc(wsT("js.tk.open.scope.team.sub")) }) +
        mi('m-opening', esc(wsT("js.tk.open.scope.store")), { scope: 'store' }, { icon: 'tasks', val: esc(wsT("js.tk.open.scope.store.sub")) }) + '</div>';
    }
    if (step === 'request') {
      var tg = reqTargets(it).filter(function (r) { return !MN.who || r.to === MN.who; });
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.request.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.request.intro") + '</p>' +
        '<div class="mlist" role="group">' + (tg.length ? tg.map(function (r) {
          return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-request') + attr('to', r.to) + '>' + avHtml(r.to) + '<span class="ml">' + whoName(r.to) + '</span><span class="mv">' + esc(stLabel(r.status)) + '</span></button>';
        }).join('') : '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>') + '</div>' + noteField(wsT("js.tk.form.note.opt"));
    }
    if (step === 'wrap') {
      // another member's assignment: an org superior's wrap-up set, or the assigner's drop (wrapAuth)
      var wrec = it.assignments.map(A).filter(function (r) { return r.to === MN.who && r.state === 'live'; })[0], wa = wrapAuth(it, wrec);
      if (!wa) return h + back + '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>';
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.wrap.title", esc(nm(MN.who)), esc(it.alias)) + '</div><p class="mintro">' + wsT(wa === 'sup' ? "js.tk.dlg.wrap.intro" : "js.tk.dlg.wrap.intro.drop", esc(nm(MN.who))) + '</p>' +
        noteField(wsT("js.tk.form.note.opt")) + '<div role="group">' + WRAP_SET[wa].map(function (s, i) {
          return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-wrap') + attr('status', s) + (i ? '' : ' data-autofocus') + '>' + SI.svg(s) + '<span class="ml">' + esc(stLabel(s)) + '</span></button>';
        }).join('') + '</div>';
    }
    if (step === 'note') {
      var L = { 'item.reject': ["js.tk.dlg.reject.title", "js.tk.dlg.reject.intro"], 'item.reopen': ["js.tk.dlg.reopen.title", "js.tk.dlg.reopen.intro"],
        'item.decline': ["js.tk.dlg.decline.title", "js.tk.dlg.decline.intro"], 'item.return': ["js.tk.dlg.return.title", "js.tk.dlg.return.intro"],
        'item.deny': ["js.tk.dlg.deny.title", "js.tk.dlg.deny.intro"] }[MN.op];
      return h + back + '<div class="mtitle">' + wsT(L[0], esc(nm(MN.who || v)), esc(it.alias)) + '</div><p class="mintro">' + wsT(L[1]) + '</p>' +
        '<div class="mrow">' + noteField(wsT("js.tk.form.note")) + btn('m-note-send', wsT("js.tk.dlg.note.submit"), {}, 'primary') + '</div>';
    }
    if (step === 'assign') return h + back + asgHtml(it);
    if (step === 'move') return h + back + moveStepHtml(it);
    // the main step: the status part (radios that apply on click, a note that rides along), then the actions
    var parts = [];
    if (canStatus(it)) {
      var cur = mineR.status, pick = MN.pick || cur, whys = [];
      parts.push('<div class="mst" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + ownStatusOpts(mineR).map(function (o) {
        var on = o.s === cur;
        if (o.dis) whys.push(o.why);
        return '<button type="button" class="mr' + (o.s === pick && !on ? ' pick' : '') + '" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-status') + attr('status', o.s) + (o.dis ? ' disabled' : '') + (on ? ' data-autofocus' : '') + '>' +
          SI.svg(o.s) + '<span>' + esc(o.label) + '</span></button>';
      }).join('') + '</div>' + (whys.length ? '<p class="mintro mwhy">' + whys.map(esc).join('<br>') + '</p>' : '') + noteField(wsT("js.tk.menu.note")));
    }
    if (MN.statusOnly) return h + (parts.join('') || '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>');
    var acts = actList(it).map(function (x) {
      return mi(x[0], esc(x[2]), x[3], { icon: x[1], cls: x[4] ? 'go' : '' });
    });
    if (mineR && wrapped(mineR.status) && !it.hidden) acts.push(mi('note', esc(wsT("js.tk.asg.reopen")), { item: it.id, who: v, op: 'item.reopen' }, { icon: 'return' }));
    if (acts.length) parts.push(acts.join(''));
    // the item window's own header menu: "add subtask" and "hand off" sit in the header beside it
    var inWin = ui.win && ui.item === it.id && MN.backEl && MN.backEl.closest && !!MN.backEl.closest('.iw-head');
    var edit = canEdit(it), task = it.kind !== 'topic', more = [];
    if (canSplit(it) && !inWin) more.push(mi('m-sub', wsT("js.tk.menu.sub"), { item: it.id }, { icon: 'plus' }));
    if (edit && task) {
      more.push(mi('pri', wsT("js.tk.menu.pri"), { item: it.id }, { icon: 'flag', sub: 1, val: esc(it.priorityName) }));
      more.push(mi('due', wsT("js.tk.menu.due"), { item: it.id }, { icon: 'calendar', sub: 1, val: esc(rangeText(it.start, it.due)) }));
    }
    if (mineR && !wrapped(mineR.status) && !mineR.fallback && !it.hidden && !inWin) more.push(mi('handoff', esc(wsT("js.tk.act.handoff")), { item: it.id }, { icon: 'handoff', sub: 1 }));
    // a topic's holder releases it: item.unassign of their own record
    if (mineR && !task && !it.hidden) more.push(mi('unassign', esc(wsT("js.tk.act.release")), { item: it.id, who: v }, { icon: 'withdraw' }));
    if (canAssign(it, v)) more.push(mi('assign', esc(wsT("js.tk.act.assign")), { item: it.id }, { icon: 'assign', sub: 1 }));
    if (canOpen(it)) more.push(mi('opening', esc(wsT("js.tk.act.open")), { item: it.id }, { icon: 'team', sub: 1 }));
    if (canUnopen(it)) more.push(mi('unopen', esc(wsT("js.tk.act.unopen")), { item: it.id }, { icon: 'withdraw' }));
    if (reqTargets(it).length) more.push(mi('request', esc(wsT("js.tk.act.request")), { item: it.id }, { icon: 'handoff', sub: 1 }));
    var mvr = mvMenuRoots(it.id);
    if (mvr.some(function (r) { return canMove(VS.items[r]); })) more.push(mi('move', esc(mvr.length > 1 ? wsTn("js.tk.mv.menu.sel", mvr.length) : wsT("js.tk.mv.menu")), { item: it.id }, { icon: 'tree', sub: 1 }));
    if (more.length) parts.push(more.join(''));
    var last = [];
    // the admin wording only where the creator-gone fallback is the sole reason the viewer may act
    if (edit) last.push(mi('edit', esc(it.author === v || mineLive(it) ? wsT("js.tk.act.edit") : wsT("js.tk.act.edit.admin")), { item: it.id }, { icon: 'pen' }));
    if (!(ui.win && ui.item === it.id)) last.push(mi('m-open', esc(wsT("js.tk.menu.open")), { item: it.id }, { icon: 'page' }));
    if (canHide(it)) last.push(mi('ihide', esc(it.author === v ? wsT("js.tk.act.hide") : wsT("js.tk.act.hide.admin")), { item: it.id }, { icon: 'x', cls: 'danger' }));
    if (last.length) parts.push(last.join(''));
    return h + (parts.length ? parts.join(msep()) : '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>');
  }
  // The handoff step's people: anyone but the holder and removed members, filtered by the search.
  function handoffList(it, from) {
    var q = String(MN.q || '').trim().toLowerCase();
    var ids = Object.keys(VS.members).filter(function (id) { var m = VS.members[id]; return id !== from && !m.removed && (!q || (id + ' ' + nmo(id)).toLowerCase().indexOf(q) >= 0); });
    if (!ids.length) return '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>';
    return ids.map(function (id) {
      return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-handoff') + attr('to', id) + '>' + avHtml(id) + '<span class="ml">' + whoName(id) + '</span><span class="mv">' + esc(id) + '</span></button>';
    }).join('');
  }
  // The CC step's people: every member not removed and not CC'd yet, the viewer included, filtered by the search.
  function ccPickList(it) {
    var q = String(MN.q || '').trim().toLowerCase(), cc = listOf(it.cc);
    var ids = Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed && cc.indexOf(id) < 0 && (!q || (id + ' ' + nmo(id)).toLowerCase().indexOf(q) >= 0); });
    if (!ids.length) return '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>';
    return ids.map(function (id) {
      return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-cc') + attr('to', id) + '>' + avHtml(id) + '<span class="ml">' + whoName(id) + '</span><span class="mv">' + esc(id) + '</span></button>';
    }).join('');
  }
  // the people list a search box (#tkmq) filters: the CC step's or the handoff step's
  function mqList(it) { return MN.step === 'cc' ? ccPickList(it) : handoffList(it, MN.who || VS.viewer.member); }
  // "Move to..." (ticket decision 8): the item menu's move step, the keyboard and touch path of the
  // move drag. A searchable tree of the destinations the store takes (moveWhy) plus the top level;
  // an item that takes none shows only as the context line of one that does, so the item's own
  // subtree stays out. A pick puts what moves last under the choice. Opened from a selected list
  // row it moves the selection's roots: a destination shows when it takes one of them, with "N of
  // M" and the reasons when not all; the roots no destination takes are named up front.
  function mvMenuRoots(id) {
    var row = MN && MN.backEl && MN.backEl.closest && MN.backEl.closest('#card-items .irow');
    return row && mselHas(id) ? moveRoots(mselIds()) : [id];
  }
  function moveStepHtml(it) {
    var roots = MN.mv || (MN.mv = mvMenuRoots(it.id)), n = roots.length;
    var stuck = roots.map(function (r) { return [r, moveWhy(r, VS.items[r] ? VS.items[r].parent || null : null)]; }).filter(function (x) { return x[1]; });
    return '<div class="mtitle">' + (n > 1 ? esc(wsTn("js.tk.mv.title.sel", n)) : esc(wsT("js.tk.mv.title", it.alias))) + '</div><p class="mintro">' + wsT("js.tk.mv.intro") + '</p>' +
      (stuck.length ? '<p class="mintro mvstuck">' + stuck.map(function (x) { return wsT("js.tk.mv.stays", esc(VS.items[x[0]] ? VS.items[x[0]].alias : alias(x[0])), esc(x[1])); }).join('<br>') + '</p>' : '') +
      '<div class="mrow"><input type="text" id="tkmvq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.mv.search") + '" aria-label="' + wsT("js.tk.mv.search") + '" value="' + esc(MN.q || '') + '"></div>' +
      '<div class="mlist" role="group">' + moveList(roots) + '</div>';
  }
  function moveList(roots) {
    var q = String(MN.q || '').trim().toLowerCase(), kids = {}, h = '';
    function takes(dest) { return roots.filter(function (r) { return !moveWhy(r, dest); }); }
    function hit(it) { return !q || [it.title, it.alias, it.id, it.project].some(function (x) { return String(x == null ? '' : x).toLowerCase().indexOf(q) >= 0; }); }
    function opt(id, depth, label, ok) {
      var cur = roots.every(function (r) { return (VS.items[r].parent || '') === id; }), part = ok.length < roots.length;
      var tip = part ? roots.filter(function (r) { return ok.indexOf(r) < 0; }).map(function (r) { return wsT("js.tk.mv.stays", VS.items[r].alias, moveWhy(r, id || null)); }).join('; ') : '';
      return '<button type="button" class="mi mvo" role="menuitem" data-mi' + attr('act', 'm-move') + attr('to', id) + ' style="--ind:' + depth + '"' + (tip ? tipAttr(tip) : '') + '><span class="ml">' + esc(label) + '</span>' +
        '<span class="mv">' + [cur ? esc(wsT("js.tk.mv.cur")) : '', part ? esc(wsT("js.tk.mv.part", ok.length, roots.length)) : '', id ? esc(VS.items[id].alias) : ''].filter(Boolean).join(' &middot; ') + '</span></button>';
    }
    var top = takes(null);
    if (top.length && (!q || wsT("js.tk.mv.top").toLowerCase().indexOf(q) >= 0)) h += opt('', 0, wsT("js.tk.mv.top"), top);
    // hidden items, and so their subtrees, are left out, as the lists leave them
    VS.itemOrder.forEach(function (id) { var it = VS.items[id]; if (it && !it.hidden) (kids[it.parent || ''] = kids[it.parent || ''] || []).push(id); });
    function walk(id, depth) {
      var it = VS.items[id], ok = takes(id), on = ok.length > 0 && hit(it), sub = (kids[id] || []).map(function (k) { return walk(k, depth + 1); }).join('');
      if (!on && !sub) return '';
      return (on ? opt(id, depth, it.title, ok) : '<div class="mvctx" style="--ind:' + depth + '">' + esc(it.title) + ' <span class="mv">' + esc(it.alias) + '</span></div>') + sub;
    }
    h += (kids[''] || []).map(function (id) { return walk(id, 0); }).join('');
    return h || '<p class="mempty">' + wsT("js.tk.mv.none") + '</p>';
  }
  // The assign popover (530): the item menu's assign step, opened from the menu or anchored at
  // the Assignees "+" (openAssign). A search field over people and subtree rules, the picked
  // targets as chips, then the completion mode and the apply button, disabled while nothing is
  // picked. Rules: "My team" (everyone within L1 / L2 below me, or my whole subtree) while the
  // search is empty, and, for a member the search finds who leads anyone, the whole subtree below
  // them, pending when they are outside my reach (a rule request they accept). People: avatar,
  // name, id, and whether the assignment is live at once or pending. One rule per root: picking
  // another drops the first. A person already holding the item, or a rule one of my live rules
  // there covers, shows as assigned and is not picked. Focus stays in the search field: arrows
  // move, Enter toggles (Cmd/Ctrl+Enter applies), Esc goes back or closes (menuKey).
  // compl stays null until the completion radio is touched; until then asgCompl derives it (58).
  // A topic takes one holder: people only, picking one replaces the last, and no completion
  // mode (the store keeps none for a topic; the payload still carries the derived one).
  function asgState() { return MN.as || (MN.as = { q: '', sel: [], compl: null, idx: 0, opts: [] }); }
  // The default completion of one target (the writers' rule, as the MCP writer's completionClasses):
  // a member below the viewer in the org tree, or a rule rooted at the viewer or below them, takes
  // assigner, so a close goes through review; the viewer themself or a peer takes assignee.
  function complClass(o) {
    var id = o.t === 'r' ? o.root : o.id;
    return (o.t === 'r' && id === VS.viewer.member) || VS.viewer.reach[id] ? 'assigner' : 'assignee';
  }
  // The completion one target is written with: assigner for a target below the viewer whatever the
  // pick, since the store refuses assignee there (check.go checkWrite: the viewer confirms that
  // wrap-up); for anyone else the user's pick, else the default class.
  function asgClass(o, compl) { return complClass(o) === 'assigner' ? 'assigner' : compl || 'assignee'; }
  // whether a pick is below the viewer: the completion radio then offers no assignee
  function asgBelow(S) { return S.sel.some(function (o) { return complClass(o) === 'assigner'; }); }
  // The picked targets as the item.assign events they write, one per completion class (asgClass),
  // the first target's class first, each with its completion stored explicitly. Within an event,
  // people come first, then rules ({root, levels?}; the whole subtree without levels).
  function asgEvents(item, sel, compl) {
    var groups = [], by = {};
    sel.forEach(function (o) {
      var c = asgClass(o, compl);
      if (!by[c]) groups.push(by[c] = { c: c, sel: [] });
      by[c].sel.push(o);
    });
    return groups.map(function (g) {
      var people = g.sel.filter(function (o) { return o.t === 'p'; }).map(function (o) { return o.id; });
      var rules = g.sel.filter(function (o) { return o.t === 'r'; }).map(function (o) { return o.levels == null ? { root: o.root } : { root: o.root, levels: o.levels }; });
      return ['item.assign', { item: item, targets: people.concat(rules), completion: g.c }];
    });
  }
  // The completion mode the radio shows: the picks' classes (asgClass); 'mixed' when they fall in
  // both (written as one assign per class); the user's pick, or null, while nothing is picked.
  function asgCompl(S) {
    var cs = {};
    S.sel.forEach(function (o) { cs[asgClass(o, S.compl)] = true; });
    var k = Object.keys(cs);
    return k.length > 1 ? 'mixed' : k[0] || S.compl || null;
  }
  // everyone below root (removed members left out), by depth
  function subtreeOf(root) {
    if (root === VS.viewer.member) return VS.viewer.reach;
    var out = {}, q = [[root, 0]];
    while (q.length) {
      var c = q.shift(), m = VS.members[c[0]];
      (m ? m.children : []).forEach(function (k) {
        if (k in out || k === root || !VS.members[k] || VS.members[k].removed) return;
        out[k] = c[1] + 1; q.push([k, c[1] + 1]);
      });
    }
    return out;
  }
  function inRange(sub, levels) { return Object.keys(sub).filter(function (k) { return levels == null || sub[k] <= levels; }).length; }
  // the store's own cover test (engine ruleCovers) for a rule without include_root
  function ruleCovered(it, root, levels) {
    var v = VS.viewer.member;
    return vals(VS.rules || {}).some(function (y) {
      if (y.item !== it.id || y.by !== v || y.state !== 'live') return false;
      var d = y.root === root ? 0 : subtreeOf(y.root)[root];
      if (d == null) return false;
      return y.levels == null || (levels != null && d + levels <= y.levels);
    });
  }
  function asgKey(o) { return o.t === 'p' ? 'p:' + o.id : 'r:' + o.root + ':' + (o.levels == null ? '*' : o.levels); }
  function asgLabel(o) {
    var v = VS.viewer.member;
    if (o.t === 'p') return esc(nm(o.id));
    if (o.root !== v) return wsT("js.tk.ap.rule.of", esc(nm(o.root)));
    return o.levels == null ? wsT("js.tk.ap.rule.all") : wsT("js.tk.ap.rule.lv", o.levels);
  }
  // the options in list order, with the group headings in between
  function asgOpts(it) {
    var S = asgState(), v = VS.viewer.member, R = VS.viewer.reach, q = S.q.trim(), out = [];
    var held = {};
    it.assignments.map(A).forEach(function (r) { if (r.state === 'live' || r.state === 'pending') held[r.to] = true; });
    function rule(root, levels) {
      var sub = subtreeOf(root);
      return { t: 'r', root: root, levels: levels, n: inRange(sub, levels), pend: root !== v && !R[root], off: ruleCovered(it, root, levels) };
    }
    var topic = it.kind === 'topic', norule = topic || it.draft; // the quick-add row assigns people only
    if (!q && !norule) {
      var depth = Math.max.apply(null, [0].concat(vals(R)));
      if (depth) {
        out.push({ g: wsT("js.tk.ap.myteam") }, rule(v, 1));
        if (depth >= 2) out.push(rule(v, 2));
        out.push(rule(v, null));
      }
    }
    var ids = q ? mentionMatches(q) : Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).sort(function (a, b) {
      if (a === v || b === v) return a === v ? -1 : 1;
      return (R[a] || 99) - (R[b] || 99) || (a < b ? -1 : 1);
    });
    if (q && !norule) {
      var leads = ids.filter(function (id) { return id !== v && inRange(subtreeOf(id), null); });
      if (leads.length) out.push({ g: wsT("js.tk.ap.teams") });
      leads.forEach(function (id) { out.push(rule(id, null)); });
    }
    if (ids.length) out.push({ g: wsT("js.tk.ap.people") });
    ids.forEach(function (id) { out.push({ t: 'p', id: id, pend: id !== v && !R[id], off: !!held[id] }); });
    return out;
  }
  function asgListHtml(it) {
    var S = asgState(), sel = {};
    S.sel.forEach(function (o) { sel[asgKey(o)] = true; });
    S.opts = asgOpts(it);
    var pickable = S.opts.filter(function (o) { return !o.g && !o.off; });
    if (S.idx >= pickable.length) S.idx = Math.max(0, pickable.length - 1);
    var n = -1;
    var h = S.opts.map(function (o) {
      if (o.g) return '<li class="agrp" role="presentation">' + esc(o.g) + '</li>';
      var on = !!sel[asgKey(o)], i = o.off ? -1 : ++n, mark = o.off ? '<span class="amk">' + wsT("js.tk.ap.has") + '</span>' :
        o.t === 'p' ? '<span class="amk' + (o.pend ? ' pend' : '') + '">' + reachTag(VS.viewer.member, o.id) + '</span>' : o.pend ? '<span class="amk pend">' + wsT("js.tk.st.pending") + '</span>' : '';
      var lead = o.t === 'p' ? avHtml(o.id) + '<span class="an">' + whoName(o.id) + '</span><code class="aid">' + esc(o.id) + '</code>' :
        '<svg class="i"><use href="#tki-team"/></svg><span class="an">' + asgLabel(o) + '</span><span class="aid">' + esc(wsTn("js.tk.ap.n", o.n)) + '</span>';
      return '<li class="aopt' + (i === S.idx ? ' on' : '') + (o.off ? ' off' : '') + '" role="option"' + (i >= 0 ? ' id="tkaopt-' + i + '"' + attr('act', 'm-atog') + attr('ai', i) : ' aria-disabled="true"') +
        ' aria-selected="' + on + '"><span class="ack" aria-hidden="true"></span>' + lead + mark + '</li>';
    }).join('');
    return h || '<li class="qnone" role="presentation">' + wsT("js.tk.ap.none") + '</li>';
  }
  function asgChipsHtml() {
    return asgState().sel.map(function (o) {
      return '<span class="achip">' + (o.t === 'p' ? avHtml(o.id) : '<svg class="i"><use href="#tki-team"/></svg>') + '<span>' + asgLabel(o) + '</span>' +
        '<button type="button" class="ax"' + attr('act', 'm-ax') + attr('k', asgKey(o)) + ' aria-label="' + esc(wsT("js.tk.ap.rm", o.t === 'p' ? nm(o.id) : asgLabel(o).replace(/<[^>]*>/g, ''))) + '"><svg class="i"><use href="#tki-x"/></svg></button></span>';
    }).join('');
  }
  function asgHtml(it) {
    var S = asgState(), list = asgListHtml(it), any = S.opts.some(function (o) { return !o.g && !o.off; }), topic = it.kind === 'topic';
    return '<div class="ap"><div class="ap-chips" id="tkachips">' + asgChipsHtml() + '</div>' +
      '<label class="ap-q"><svg class="i"><use href="#tki-search"/></svg><input type="text" id="tkaq" data-autofocus autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="tkalist" aria-autocomplete="list"' +
      (any ? ' aria-activedescendant="tkaopt-' + S.idx + '"' : '') + ' placeholder="' + wsT("js.tk.ap.search") + '" aria-label="' + wsT("js.tk.ap.search") + '" value="' + esc(S.q) + '"></label>' +
      '<ul class="ap-list" id="tkalist" role="listbox" aria-multiselectable="' + !topic + '" aria-label="' + esc(wsT("js.tk.act.assign")) + '">' + list + '</ul>' +
      '<div class="ap-foot">' + (topic ? '<span class="muted">' + wsT("js.tk.ap.topic") + '</span>' : '<span class="ap-cm" role="radiogroup" aria-label="' + wsT("js.tk.form.completion") + '">' + ['assignee', 'assigner'].map(function (c) {
        return '<button type="button" role="radio" aria-checked="' + (asgCompl(S) === c) + '"' + attr('act', 'm-acm') + attr('c', c) + (c === 'assignee' && asgBelow(S) ? ' hidden' : '') + '>' + esc(complText(c)) + '</button>';
      }).join('') + '</span><span class="ap-mix muted" id="tkamix"' + tipAttr(wsT("js.tk.ap.compl.mixed.tip")) + (asgCompl(S) === 'mixed' ? '' : ' hidden') + '>' + wsT("js.tk.ap.compl.mixed") + '</span>') + '<button type="button" class="btn primary" id="tkaapply"' + attr('act', 'm-aapply') + (S.sel.length ? '' : ' disabled') + '>' + wsT("js.tk.dlg.assign.submit") + '</button></div></div>';
  }
  // the list, the chips, and the apply button again; the search field (and an IME composition in it) stays
  function asgDraw() {
    var it = mnItem(), S = asgState();
    if (!it || !$('tkalist')) return;
    $('tkalist').innerHTML = asgListHtml(it);
    $('tkachips').innerHTML = asgChipsHtml();
    $('tkaapply').disabled = !S.sel.length;
    $('tkaq').setAttribute('aria-activedescendant', $('tkaopt-' + S.idx) ? 'tkaopt-' + S.idx : '');
    var on = $('tkalist').querySelector('.aopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
    Array.prototype.forEach.call(document.querySelectorAll('#tkmenu .ap-cm [role="radio"]'), function (b) {
      b.setAttribute('aria-checked', String(b.getAttribute('data-c') === asgCompl(S)));
      b.hidden = b.getAttribute('data-c') === 'assignee' && asgBelow(S); // a pick below the viewer: assigner only
    });
    if ($('tkamix')) $('tkamix').hidden = asgCompl(S) !== 'mixed';
  }
  function asgToggle(i) {
    var S = asgState(), o = S.opts.filter(function (x) { return !x.g && !x.off; })[i];
    if (!o) return;
    var k = asgKey(o), had = S.sel.some(function (x) { return asgKey(x) === k; });
    var one = mnItem() && mnItem().kind === 'topic';
    S.sel = one ? [] : S.sel.filter(function (x) { return asgKey(x) !== k && !(o.t === 'r' && x.t === 'r' && x.root === o.root); });
    if (!had) S.sel.push({ t: o.t, id: o.id, root: o.root, levels: o.levels });
    S.idx = i;
    asgDraw();
  }
  // the picks as item.assign events (asgEvents): one, or one per completion class
  function asgApply() {
    var S = asgState(), it = mnItem();
    if (!it || !S.sel.length) return;
    // a chip's picker: the picks go to the row, sent with its create
    if (it.draft) { var n = nd(); n.who = S.sel.map(function (o) { return o.id; }); n.compl = S.compl; return newPicked(); }
    commit(asgEvents(it.id, S.sel, S.compl));
  }
  function asgMenuKey(e) {
    var t = e.target, S = asgState(), k = e.key;
    if (t.id === 'tkaq') {
      if (e.isComposing || e.keyCode === 229) return true;
      var n = S.opts.filter(function (x) { return !x.g && !x.off; }).length;
      if (k === 'ArrowDown' || k === 'ArrowUp') { e.preventDefault(); if (n) { S.idx = (S.idx + (k === 'ArrowDown' ? 1 : n - 1)) % n; asgDraw(); } return true; }
      if (k === 'Enter') { e.preventDefault(); if (e.metaKey || e.ctrlKey) asgApply(); else asgToggle(S.idx); return true; }
      return false;
    }
    if ((k === 'ArrowLeft' || k === 'ArrowRight') && t.closest('.ap-cm')) {
      e.preventDefault();
      var nx = t.parentNode.querySelector('[data-c]:not([hidden]):not([data-c="' + t.getAttribute('data-c') + '"])');
      if (nx) { S.compl = nx.getAttribute('data-c'); asgDraw(); nx.focus(); }
      return true;
    }
    return false;
  }
  function menuNote() { var n = $('tkmnote'); return n ? n.value.trim() : ''; }
  // A status from the menu: done closes (it waits for the assigner when the completion says so).
  function applyStatus(s) {
    if (!setStatus(VS.items[MN.item], s, menuNote())) closeMenu(true);
  }
  // Writes the viewer's status on it with an optional note; false when there is nothing to write.
  function setStatus(it, s, note) {
    var origin = CAPS.origin && $('corigin') ? $('corigin').value : 'human';
    var r = it && mineLive(it);
    if (!r) return false;
    if (s === r.status && !note) return false;
    var p = { item: it.id };
    if (note) p.note = note;
    if (s === 'done') commit([['item.close', p, origin]]);
    else { p.status = s; commit([['item.status', p, origin]]); }
    return true;
  }
  // The menu's own acts (m-*); the rest of a menu item's data-act goes through the shared handlers.
  function menuAct(k, a) {
    if (MN.kind === 'store') return storeAct(k);
    var id = MN.item, it = mnItem();
    if (k === 'm-back') return menuBack();
    if (!it) return closeMenu(true);
    if (k === 'm-rpage' || k === 'm-rclr') return rangeAct(k, a);
    if (MN.kind === 'new' && !/^m-(atog|ax|acm|aapply)$/.test(k)) return newAct(k, a);
    switch (k) {
      case 'm-atog': asgToggle(Number(a.getAttribute('data-ai'))); return $('tkaq').focus();
      case 'm-ax': {
        var S = asgState(), ak = a.getAttribute('data-k');
        S.sel = S.sel.filter(function (x) { return asgKey(x) !== ak; });
        asgDraw(); return $('tkaq').focus();
      }
      case 'm-acm': asgState().compl = a.getAttribute('data-c'); return asgDraw();
      case 'm-aapply': return asgApply();
      case 'm-status': return applyStatus(a.getAttribute('data-status'));
      case 'm-pri': {
        var pv = Number(a.getAttribute('data-p'));
        if (pv === it.priority) return menuBack();
        return commit([['item.edit', { item: id, priority: pv }]]);
      }
      case 'm-ltog': MN.lab = a.getAttribute('data-label'); MN.lset = labelFlip(MN.lset, labelBase(it), MN.lab); return labelStepRedraw();
      case 'm-ledit': return labelDialog({ label: a.getAttribute('data-label') });
      case 'm-lrm': return labelRemove(a.getAttribute('data-label'), function () { if (MN) MN.lab = null; });
      case 'm-ladd': return labelDialog({});
      case 'm-cc': return commit([['item.cc', { item: id, member: a.getAttribute('data-to') }]]);
      case 'm-handoff': {
        var hp = { item: id, to: a.getAttribute('data-to') }, note = menuNote();
        if (MN.who && MN.who !== VS.viewer.member) hp.assignee = MN.who;
        if (note) hp.note = note;
        return commit([['item.handoff', hp]]);
      }
      case 'm-note-send': {
        var np = { item: id }, nt = menuNote();
        if (MN.op === 'item.deny') np.request = MN.req;
        else if (MN.who && MN.op !== 'item.decline') np.assignee = MN.who;
        if (nt) np.note = nt;
        return commit([[MN.op, np]]);
      }
      case 'm-opening': return commit([['item.open', { item: id, scope: a.getAttribute('data-scope') }]]);
      case 'm-wrap': {
        var wp = { item: id, assignee: MN.who, status: a.getAttribute('data-status') }, wn = menuNote();
        if (wn) wp.note = wn;
        return commit([['item.status', wp]]);
      }
      case 'm-request': {
        var rp = { item: id, assignee: a.getAttribute('data-to') }, rn = menuNote();
        if (rn) rp.note = rn;
        return commit([['item.request', rp]]);
      }
      case 'm-sub': {
        // under a list, tree, gantt, or subtask row; a kanban card or a calendar entry has no room
        // for an input row under it, so those (and the window header) use the item window's Subtasks
        var rk = MN.rk && !/^(iw|k|c):/.test(MN.rk) && document.querySelector('.tk [data-rk="' + CSS.escape(MN.rk) + '"]') ? MN.rk : 'iw:' + id;
        closeMenu(false);
        return startSub(id, rk);
      }
      case 'm-open': closeMenu(false); return showItem(id);
      case 'm-move': {
        var mroots = MN.mv || mvMenuRoots(id), slot = { parent: a.getAttribute('data-to') || null, after: undefined };
        if (mroots.length === 1 && moveNoop(mroots[0], slot)) return closeMenu(true);
        return moveApply(mroots, slot);
      }
    }
  }
  // The inline "add subtask" row under rk ('iw:<id>': the item window's Subtasks section).
  function startSub(id, rk) {
    if (rk === 'iw:' + id && !(ui.win && ui.item === id)) showItem(id);
    ui.sub = { item: id, rk: rk, val: '' };
    render();
    if ($('tksub')) $('tksub').focus();
  }
  // Keys inside the menu (see MN).
  function menuKey(e) {
    var m = $('tkmenu'), t = e.target, k = e.key, field = t.matches('input[type="text"], input[type="date"], input:not([type]), textarea, select');
    if (MN.step === 'assign' && asgMenuKey(e)) return;
    if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); menuDiscard(); return; }
    if (k === 'Tab') {
      var fs = Array.prototype.filter.call(m.querySelectorAll('button:not([disabled]), input:not([disabled]), select'), function (x) { return x.offsetParent !== null; });
      if (!$('tkcap').hidden) fs.push($('tkcap')); // the discard capsule, a fixed element beside the menu, comes last
      if (!fs.length) return;
      var i = fs.indexOf(t);
      e.preventDefault();
      fs[(i + (e.shiftKey ? fs.length - 1 : 1)) % fs.length].focus();
      return;
    }
    if (k === 'Enter' && !e.isComposing) {
      if (t.id === 'tkmnote' && MN.step === 'main') { e.preventDefault(); applyStatus(MN.pick || (VS.items[MN.item] && (function (r) { return r ? r.status : ''; })(mineLive(VS.items[MN.item])))); return; }
      if (t.id === 'tkmnote' && MN.step === 'note') { e.preventDefault(); menuAct('m-note-send', t); return; }
      if (t.id === 'tkmnote' && MN.step === 'request') { e.preventDefault(); var p0 = m.querySelector('[data-act="m-request"]'); if (p0) p0.click(); return; }
      if (t.hasAttribute('data-rin')) { e.preventDefault(); menuApplyClose(true); return; }
      if (t.id === 'tkmq') { e.preventDefault(); var p1 = m.querySelector('[data-act="m-handoff"], [data-act="m-cc"]'); if (p1) p1.click(); return; }
      if (t.id === 'tkmvq') { e.preventDefault(); var p2 = m.querySelector('[data-act="m-move"]'); if (p2) p2.click(); return; }
      return; // a button: its click; a form field: the form's submit
    }
    var items = Array.prototype.filter.call(m.querySelectorAll('[data-mi]'), function (x) { return !x.disabled; }), at = items.indexOf(t);
    if ((k === 'ArrowDown' || k === 'ArrowUp') && t.type !== 'date') {
      e.preventDefault();
      if (!items.length) return;
      var n = at < 0 ? (k === 'ArrowDown' ? 0 : items.length - 1) : (at + (k === 'ArrowDown' ? 1 : items.length - 1)) % items.length;
      items[n].focus();
      return;
    }
    if (field) return;
    if (k === 'ArrowRight' || k === 'ArrowLeft') {
      if (t.classList.contains('mr') && t.parentNode.classList.contains('mst')) {
        e.preventDefault();
        // arrowing over the radios picks without applying: the note's Enter then applies the picked status
        var rs = Array.prototype.filter.call(t.parentNode.children, function (x) { return !x.disabled; }), j = rs.indexOf(t), nx = rs[(j + (k === 'ArrowRight' ? 1 : rs.length - 1)) % rs.length];
        MN.pick = nx.getAttribute('data-status');
        rs.forEach(function (x) { x.classList.toggle('pick', x === nx && x.getAttribute('aria-checked') !== 'true'); });
        nx.focus();
      } else if (k === 'ArrowRight' && t.hasAttribute('data-sub')) { e.preventDefault(); t.click(); }
      else if (k === 'ArrowLeft' && MN.from) { e.preventDefault(); menuBack(); }
      return;
    }
    if (k === 'Home' || k === 'End') { e.preventDefault(); if (items.length) items[k === 'Home' ? 0 : items.length - 1].focus(); }
  }

  // ------------------------------------------------------- the date range picker

  // The item menu's 'due' step and the due chip of the new row and the edit row: one picker over
  // an item's start and due dates (MN.rg = { s, d: the picked ends, '' for none; s0, d0: what it
  // opened on; lm, rm: the left and right panel months, rm null with one panel; err: the inline
  // error shows; last: the end last changed, 's' or 'd'; drag: a handle drag in progress }).
  // The span runs from the start's left half-circle handle to the due's right one; dragging a
  // handle moves that end, clamped at the other end. One end alone shows its handle and a dashed
  // ghost for the other, which a drag pulls out into that end. Two panels show when the ends fall
  // in different months, each paged on its own; the panels refit only on an open, a typed date, a
  // clear, or a drag end. Under the grid, a date input and a clear button per end.
  // A plain click on a day moves an end too (rangeClick), so an end whose day no panel shows can
  // still be set; it never inverts the pair. Two panels on different months: the left sets the
  // start, the right the due. Else (one panel, or both on one month) the end nearer the day moves,
  // the due on a tie; with one end set, a day on or after the start sets the due, a day before it
  // moves the start, and mirrored for a lone due; with none set, the click sets the due.
  // Any close applies the pick once (closeMenu): an item's changed ends as one item.edit, a row's
  // draft fields; Esc and the discard capsule drop it (menuDiscard). An inverted pair is refused
  // by every close that can stay open (menuApplyClose).
  function monAdd(m, n) { var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)) - 1 + n; return new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7); }
  function rangeBad(r) { return !!(r.s && r.d && r.s > r.d); }
  // the picker's state for menu m, set up on first use from what it edits: the item, or the row's draft
  function rangeOf(m) {
    if (m.rg) return m.rg;
    var src = m.kind === 'new' ? nd() : VS.items[m.item], s = (src && src.start) || '', d = (src && src.due) || '';
    var r = m.rg = { s: s, d: d, s0: s, d0: d, lm: null, rm: null, err: false, last: 's', drag: null };
    rangeFit(r);
    r.err = rangeBad(r);
    return r;
  }
  // the panels for the ends as they are: the earlier end's month left and the later one's right
  // when they differ, else one panel on the month of an end that is set (else as it was, else today's)
  function rangeFit(r) {
    var ms = r.s && r.s.slice(0, 7), md = r.d && r.d.slice(0, 7);
    if (ms && md && ms !== md) { r.lm = ms < md ? ms : md; r.rm = ms < md ? md : ms; return; }
    r.lm = ms || md || r.lm || today().slice(0, 7); r.rm = null;
  }
  function rangeHtml(r) {
    return '<div class="rgp' + (r.rm ? ' two' : '') + (r.err ? ' tk-rbad' : '') + '"><div class="rgcal" id="tkrcal">' + rangeCal(r) + '</div>' +
      '<div class="rgin">' + rangeField(r, 's') + rangeField(r, 'd') + '</div>' +
      '<p class="rgerr" id="tkrerr" role="alert"' + (r.err ? '' : ' hidden') + '>' + wsT("js.tk.range.err") + '</p></div>';
  }
  function rangeField(r, side) {
    var lab = wsT(side === 's' ? "js.tk.range.start" : "js.tk.range.due"), clr = wsT(side === 's' ? "js.tk.range.clear.start" : "js.tk.menu.due.clear");
    return '<div class="rgf"><label for="tkr' + side + '">' + lab + '</label><input type="date" id="tkr' + side + '"' + attr('rin', side) + (side === 'd' ? ' data-autofocus' : '') +
      ' value="' + esc(side === 's' ? r.s : r.d) + '"' + (r.err ? ' aria-invalid="true" aria-describedby="tkrerr"' : '') + '>' +
      '<button type="button" class="ia rgx"' + attr('act', 'm-rclr') + attr('side', side) + ' aria-label="' + esc(clr) + '"' + tipAttr(clr) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  function rangeCal(r) { return rangeMonth(r, r.lm, 'l') + (r.rm ? rangeMonth(r, r.rm, 'r') : ''); }
  // One month panel (side 'l' or 'r'): its arrows page it alone. Day cells are not buttons, so
  // they stay out of the menu's Tab ring; the keyboard path is the inputs.
  function rangeMonth(r, m, side) {
    var first = dayNum(m + '-01'), dow = new Date(first * 86400000).getUTCDay(), y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7));
    var days = new Date(Date.UTC(y, mo, 0)).getUTCDate(), lo = r.s && r.d && r.s <= r.d ? r.s : '', t = today();
    function nav(dir) {
      var l = wsT(dir < 0 ? "js.tk.range.prev" : "js.tk.range.next");
      return '<button type="button" class="ia rgnav"' + attr('act', 'm-rpage') + attr('side', side) + attr('dir', dir) + ' aria-label="' + esc(l) + '"' + tipAttr(l) + '>' + (dir < 0 ? '&lsaquo;' : '&rsaquo;') + '</button>';
    }
    var h = '<div class="rgm"><div class="rghd">' + nav(-1) + '<b>' + esc(m) + '</b>' + nav(1) + '</div><div class="rgg"' + attr('rgg', side) + '>' +
      calDow().map(function (d) { return '<div class="rgw">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="rgd off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, k = '';
      if (ds === r.s) k += '<span class="rgk ks" data-rh="s"></span>';
      if (ds === r.d) k += '<span class="rgk kd" data-rh="d"></span>';
      if (ds === r.d && !r.s) k += '<span class="rgk ks ghost" data-rh="s"></span>';
      if (ds === r.s && !r.d) k += '<span class="rgk kd ghost" data-rh="d"></span>';
      h += '<div class="rgd' + (lo && ds >= lo && ds <= r.d ? ' in' : '') + (ds === r.s ? ' s' : '') + (ds === r.d ? ' d' : '') + (ds === t ? ' now' : '') + '"' + attr('day', ds) + '>' + k + '<span class="rgn">' + d + '</span></div>';
    }
    return h + '</div></div>';
  }
  // The open picker after a change: the grid draws again (a focused arrow keeps its focus), the
  // inputs other than src (the one being typed in, if any) take the ends, and the error, the panel
  // count, and the menu's width follow. A drag or a clear leaves no src: a focused input still
  // follows it.
  function rangeDraw(src) {
    var r = MN && MN.rg, c = $('tkrcal');
    if (!r || !c) return;
    var f = document.activeElement, fa = f && c.contains(f) && f.getAttribute('data-act') === 'm-rpage' ? [f.getAttribute('data-side'), f.getAttribute('data-dir')] : null;
    c.innerHTML = rangeCal(r);
    c.parentNode.classList.toggle('two', !!r.rm);
    c.parentNode.classList.toggle('tk-rbad', !!r.err);
    $('tkmenu').classList.toggle('rwide', !!r.rm);
    ['s', 'd'].forEach(function (side) {
      var i = $('tkr' + side);
      if (!i) return;
      if (i !== src && i.value !== r[side]) i.value = r[side];
      if (r.err) { i.setAttribute('aria-invalid', 'true'); i.setAttribute('aria-describedby', 'tkrerr'); } else { i.removeAttribute('aria-invalid'); i.removeAttribute('aria-describedby'); }
    });
    $('tkrerr').hidden = !r.err;
    placeMenu();
    if (fa) { var b = c.querySelector('[data-act="m-rpage"][data-side="' + fa[0] + '"][data-dir="' + fa[1] + '"]'); if (b) b.focus({ preventScroll: true }); }
  }
  // a typed date or an emptied input: that end in the pick
  function rangeInput(t) {
    var r = MN && MN.step === 'due' && MN.rg, side = t.getAttribute('data-rin');
    if (!r) return;
    r[side] = t.value || ''; r.last = side; r.err = rangeBad(r);
    rangeFit(r); rangeDraw(t);
  }
  // the month arrows (view only) and the clear buttons (that end in the pick; the picker stays open)
  function rangeAct(k, a) {
    var r = rangeOf(MN), side = a.getAttribute('data-side');
    if (k === 'm-rclr') { r[side] = ''; r.last = side; r.err = rangeBad(r); rangeFit(r); }
    else {
      var dir = Number(a.getAttribute('data-dir'));
      if (side === 'r' && r.rm) r.rm = monAdd(r.rm, dir); else r.lm = monAdd(r.lm, dir);
    }
    rangeDraw();
  }
  // an end moved by a drag, clamped at the other end (the ends never swap or push each other)
  function rangeClamp(r, side, day) {
    if (side === 's' && r.d && day > r.d) return r.d;
    if (side === 'd' && r.s && day < r.s) return r.s;
    return day;
  }
  function rangeSet(r, side, day) {
    day = rangeClamp(r, side, day);
    if (r[side] === day) return;
    r[side] = day; r.last = side; r.err = rangeBad(r);
    rangeDraw();
  }
  // A drag past the dragged end's panel edge: with one panel, the neighbouring month of that end
  // opens beside it (start: before, due: after) while the other end is set, else the panel pages;
  // with two panels, that end's own panel pages, never past the other panel.
  function rangeEdge(r, side, dir) {
    if (!r.rm) {
      if (!(side === 's' ? r.d : r.s)) r.lm = monAdd(r.lm, dir);
      else if (side === 's' && dir < 0) { r.rm = r.lm; r.lm = monAdd(r.lm, -1); }
      else if (side === 'd' && dir > 0) r.rm = monAdd(r.lm, 1);
      else return;
    } else if (side === 's') { var l = monAdd(r.lm, dir); if (l > r.rm) return; r.lm = l; }
    else { var m = monAdd(r.rm, dir); if (m < r.lm) return; r.rm = m; }
    rangeDraw();
  }
  // Pointer drags (mouse and touch) on a handle, captured by the menu so a redrawn grid keeps the
  // drag. A day under the pointer moves the end there; outside the end's panel the edge pages
  // (rangeEdge, then every RG_EDGE_MS while it stays out). A plain click on a day moves an end (rangeClick).
  var RG_EDGE_MS = 450;
  // the end a click on day moves, per the rules above the picker's functions; pane is the clicked panel, 'l' or 'r'
  function rangeClickEnd(r, day, pane) {
    if (!r.s && !r.d) return 'd';
    if (r.rm && r.rm !== r.lm) return pane === 'r' ? 'd' : 's';
    if (!r.d) return day >= r.s ? 'd' : 's';
    if (!r.s) return day <= r.d ? 's' : 'd';
    return Math.abs(dayNum(day) - dayNum(r.s)) < Math.abs(dayNum(day) - dayNum(r.d)) ? 's' : 'd';
  }
  function rangeClick(r, day, pane) {
    var side = rangeClickEnd(r, day, pane);
    day = rangeClamp(r, side, day);
    if (r[side] === day) return;
    r[side] = day; r.last = side; r.err = rangeBad(r);
    rangeFit(r); rangeDraw();
  }
  function rangeDown(e) {
    // On a picker with the capsule, a press on anything but a control (a day, a header, the panel)
    // keeps focus where it was: focus left on the body would take Esc and Tab away from menuKey.
    if (capOn() && !e.target.closest('input, button, select, textarea, label, [tabindex]')) e.preventDefault();
    var r = MN && MN.step === 'due' && MN.rg, k = r && e.target.closest('[data-rh]');
    if (!r || e.button > 0) return;
    if (!k) {
      var cell = e.target.closest('[data-day]');
      var pn = cell && cell.closest('[data-rgg]');
      if (cell) rangeClick(r, cell.getAttribute('data-day'), pn && pn.getAttribute('data-rgg'));
      return;
    }
    e.preventDefault();
    r.drag = { side: k.getAttribute('data-rh'), id: e.pointerId, x: e.clientX, y: e.clientY, at: 0, tm: setInterval(rangeTick, RG_EDGE_MS) };
    $('tkmenu').setPointerCapture(e.pointerId);
  }
  function rangeMove(e) {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g || e.pointerId !== g.id) return;
    g.x = e.clientX; g.y = e.clientY;
    rangeTick();
  }
  function rangeTick() {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g) return;
    var at = document.elementFromPoint(g.x, g.y), cell = at && at.closest && at.closest('#tkmenu [data-day]');
    if (cell) { rangeSet(r, g.side, cell.getAttribute('data-day')); return; }
    var grid = $('tkmenu').querySelector('[data-rgg="' + (r.rm && g.side === 'd' ? 'r' : 'l') + '"]'), b = grid && grid.getBoundingClientRect();
    if (!b) return;
    var dir = g.x < b.left || g.y < b.top ? -1 : g.x > b.right || g.y > b.bottom ? 1 : 0, now = Date.now();
    if (!dir || now - g.at < RG_EDGE_MS) return;
    g.at = now;
    rangeEdge(r, g.side, dir);
  }
  function rangeUp(e) {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g || (e.pointerId != null && e.pointerId !== g.id)) return;
    clearInterval(g.tm); r.drag = null;
    if ($('tkmenu').hasPointerCapture && $('tkmenu').hasPointerCapture(g.id)) $('tkmenu').releasePointerCapture(g.id);
    rangeFit(r); r.err = rangeBad(r); rangeDraw();
  }
  // What closing menu m owes from its range picker: { bad } for an inverted pair; for an item,
  // { ev } with one item.edit of the ends the user moved off what the picker opened on that still
  // differ from the item (so an end another viewer wrote meanwhile and the user left alone is kept,
  // and a changed start never carries an unchanged due); for a row's draft, { draft } when an end
  // moved; null for nothing owed, or when the item or the viewer's edit right is gone.
  function rangePending(m) {
    var r = m && m.step === 'due' && m.rg;
    // a back-row write in flight (rsent) already carries this pick: its own close must not resend it
    if (!r || m.rsent) return null;
    if (m.kind === 'new') {
      var n = nd();
      if (!n) return null;
      if (rangeBad(r)) return { bad: true };
      return r.s === r.s0 && r.d === r.d0 ? null : { draft: n };
    }
    var it = m.kind === 'item' && VS && VS.items[m.item];
    if (!it || !canEdit(it)) return null;
    if (rangeBad(r)) return { bad: true };
    var ep = { item: it.id };
    if (r.s !== r.s0 && (r.s || null) !== (it.start || null)) ep.start = r.s || null;
    if (r.d !== r.d0 && (r.d || null) !== (it.due || null)) ep.due = r.d || null;
    return Object.keys(ep).length > 1 ? { ev: [['item.edit', ep]] } : null;
  }
  // An inverted pick refuses a close that can stay open: the error shows and focus goes to the
  // input of the end last changed. Returns whether it refused.
  function rangeRefused() {
    var p = MN && rangePending(MN);
    if (!p || !p.bad) return false;
    MN.rg.err = true;
    rangeDraw();
    var f = $('tkr' + MN.rg.last);
    if (f) f.focus({ preventScroll: true });
    return true;
  }
  // A close that applies and could keep the menu open (an outside click, focus loss, Enter in a
  // range input, a menu opened over this one): refused while the range picker holds an inverted
  // pair. Returns whether the menu closed.
  function menuApplyClose(refocus) {
    if (rangeRefused()) return false;
    closeMenu(refocus);
    return true;
  }
  // Esc and the discard capsule: the range or label pick drops and nothing is written (a row's
  // label toggles, which edit the draft live, go back to what the picker opened on); a step opened
  // from the main menu goes back to it, a picker without one closes and focus goes to its opener.
  function menuDiscard() {
    if (!MN) return;
    MN.rg = null; MN.lset = null;
    if (MN.lsnap) { var n = nd(); if (n) n.labels = MN.lsnap; MN.lsnap = null; render(); }
    menuBack();
  }
  // the menu and its discard capsule, one place for every "inside the menu" test
  function menuHas(t) { return !!t && ($('tkmenu').contains(t) || $('tkcap').contains(t)); }
  // the pickers that show the discard capsule
  function capOn() { return !!MN && (MN.kind === 'item' || MN.kind === 'new') && (MN.step === 'due' || MN.step === 'labels'); }
  // The capsule floats just below the menu, above it when below has no room, else over the menu's
  // bottom edge, kept inside the window.
  function placeCap() {
    var c = $('tkcap');
    c.hidden = !capOn();
    if (c.hidden) return;
    var mr = $('tkmenu').getBoundingClientRect(), cw = c.offsetWidth, ch = c.offsetHeight, W = window.innerWidth, Hh = window.innerHeight, y = mr.bottom + 6;
    if (y + ch > Hh - 6) y = mr.top - ch - 6 >= 6 ? mr.top - ch - 6 : Math.max(6, Math.min(Hh - ch - 6, mr.bottom - ch / 2));
    c.style.left = Math.round(Math.max(6, Math.min(mr.left + (mr.width - cw) / 2, W - cw - 6))) + 'px';
    c.style.top = Math.round(y) + 'px';
  }
  // A click outside the menu and its capsule closes the menu, applying its pick; a click on its
  // opener or the item window's backdrop is spent on that. A refused close keeps the menu and
  // spends the click, so another item's chip cannot open over the kept pick. Returns whether the
  // click is spent.
  function menuOutside(e, t) {
    if (!MN || (t.closest && t.closest('#tkmenu, #tkcap'))) return false;
    var spent = !!(MN.backEl && MN.backEl.contains(t)) || t === $('tkwin');
    if (!menuApplyClose(spent)) spent = true;
    if (spent) e.preventDefault();
    return spent;
  }
  // Focus leaving the labels step or the range picker for somewhere outside the menu, its capsule,
  // and its opener (whose click toggles it shut) applies the pick. Focus a redraw dropped (a month
  // arrow, a panel opening) has no new target and is not a focus loss.
  function menuFocusOut(e) {
    var to = e.relatedTarget;
    if (!MN || !to || menuHas(to) || (MN.backEl && MN.backEl.contains(to))) return;
    if ((MN.kind === 'item' && MN.step === 'labels') || ((MN.kind === 'item' || MN.kind === 'new') && MN.step === 'due')) menuApplyClose(false);
  }

