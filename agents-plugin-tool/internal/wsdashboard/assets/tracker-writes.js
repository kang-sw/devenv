  // ------------------------------------------------------------ inline writes

  // A new item: an input row in the Items list (ui.nrow[store] = { val, kind, st, who, compl, due,
  // parent?, after?, at?, depth? } while it shows). The "+" that ends the panel opens it there (no
  // `at`: it appends a root item); the hover insert opens it in a gap between rows (`at` names the
  // row it sits against, and `parent` / `after` place the item where the row shows). Title + Enter
  // creates the item; the row stays for the next one, which then goes after the one just made;
  // Esc closes it. The icon toggle at its left edge switches the kind (task <-> topic).
  // The row's chips set status, assignee, priority, due, and labels before Enter (nrowChips,
  // newMenu); a topic has no status, priority, or due. Left at their defaults (idea, the viewer,
  // the store's default level, none, none) the row sends the create plus the creator's status
  // idea (asIdea), or for a topic the create plus the creator's assign (heldTopic). The chips keep
  // their values across creates while the row stays open; closing it restores the defaults.
  // n.pri is a level index (null: the store's default, sent as no priority); n.labels label ids.
  function nrowFill(n) {
    if (!n.who) { n.st = 'idea'; n.who = [VS.viewer.member]; n.compl = null; n.start = ''; n.due = ''; n.pri = null; n.labels = []; }
    return n;
  }
  // a row opened from another (the hover insert moving it) keeps its text, kind, and chips
  function nrowNew(o, place) {
    var n = { val: o ? o.val : '', kind: o ? o.kind : 'task' };
    if (o && o.who) { n.st = o.st; n.who = o.who; n.compl = o.compl; n.start = o.start; n.due = o.due; n.pri = o.pri; n.labels = o.labels; }
    Object.keys(place || {}).forEach(function (k) { n[k] = place[k]; });
    return nrowFill(n);
  }
  function nrowMine(n) { return n.who.length === 1 && n.who[0] === VS.viewer.member; }
  // The status chip's choices: the creator's own statuses while the viewer alone is the assignee
  // (sent as item.status on the store's fallback holding); the wrap-up set when the viewer is an
  // org superior of every picked assignee, none of them the viewer (one item.status per assignee
  // after the assign, which is live at once for a subordinate), todo standing for no status write
  // (the live assignment's own start); none otherwise, where anyone else starts in the store's own
  // state (todo within reach, pending outside it). A topic has no status or due: only its one
  // holder shows.
  var NROW_ST = { own: ['idea', 'todo', 'doing', 'blocked'], wrap: ['todo', 'blocked', 'done', 'dropped'] };
  function nrowStMode(n) {
    var v = VS.viewer.member;
    if (n.kind === 'topic') return '';
    // an edit writes the viewer's own status only, and none on a done or dropped holding: the store
    // takes no item.status there (reopen it first), so as the item menu (canStatus) it offers none
    if (n.ed) { var r = mineLive(VS.items[n.ed]); return r && !wrapped(r.status) && (!n.canAsg || nrowMine(n)) ? 'own' : ''; }
    if (nrowMine(n)) return 'own';
    return n.who.length && n.who.every(function (id) { return id !== v && !!VS.viewer.reach[id]; }) ? 'wrap' : '';
  }
  // the chip's status as the mode reads it: a pick the mode does not offer falls back to its first
  function nrowSt(n) { var m = nrowStMode(n); return !m ? '' : nrowStOpts(n).indexOf(n.st) >= 0 ? n.st : NROW_ST[m][0]; }
  // the status chip's choices; an edit also offers the status the item is in (in-review, done, ...)
  function nrowStOpts(n) { var l = NROW_ST[nrowStMode(n)] || []; return n.ed && n.st && l.indexOf(n.st) < 0 ? l.concat(n.st) : l; }
  // the chip's level: the pick while the store's levels still hold it, else the store's default
  function nrowPri(n) { return n.pri != null && n.pri >= 0 && n.pri < VS.priority.names.length ? n.pri : VS.priority.default; }
  // the picked labels the store's list still holds, in the list's order
  function nrowLabels(n) { return (VS.labels || []).filter(function (l) { return (n.labels || []).indexOf(l.id) >= 0; }); }
  function nrowChips(n) {
    var topic = n.kind === 'topic', h = '', st = nrowSt(n);
    function chip(id, step, label, inner, dflt) {
      return '<button type="button" class="nch' + (dflt ? ' dflt' : '') + '" id="' + id + (n.ed ? 'e' : '') + '"' + attr('act', 'nchip') + attr('step', step) + (n.ed ? attr('ed', '1') : '') +
        ' aria-haspopup="' + (step === 'assign' ? 'dialog' : 'menu') + '" aria-expanded="false" aria-label="' + esc(label) + '"' + tipAttr(label) + '>' + inner + '</button>';
    }
    if (st) h += chip('tknst', 'status', wsT("js.tk.new.st", stLabel(st)), SI.svg(st) + '<span>' + esc(stLabel(st)) + '</span>', st === NROW_ST[nrowStMode(n)][0]);
    var one = n.who.length === 1, wl = one ? nm(n.who[0]) : wsTn("js.tk.ap.n", n.who.length);
    if (!n.ed || n.canAsg) h += chip('tknwho', 'assign', wsT("js.tk.new.who", wl), one ? avHtml(n.who[0]) + '<span>' + esc(wl) + '</span>' :
      '<svg class="i"><use href="#tki-assign"/></svg><span>' + esc(wl) + '</span>', nrowMine(n));
    if (!topic) {
      var pv = nrowPri(n), pn = VS.priority.names[pv];
      h += chip('tknpri', 'pri', wsT("js.tk.new.pri", pn), '<svg class="i"><use href="#tki-flag"/></svg><span>' + esc(pn) + '</span>', pv === VS.priority.default);
      var rt = rangeText(n.start, n.due);
      h += chip('tkndue', 'due', wsT("js.tk.new.due", rt || wsT("js.tk.new.due.none")), '<svg class="i"><use href="#tki-calendar"/></svg>' + (rt ? '<span>' + esc(rt) + '</span>' : ''), !rt);
    }
    // labels (any kind: a label is orthogonal to the kind); with an empty list only an admin, who can add one, sees the chip
    var ls = nrowLabels(n);
    if ((VS.labels || []).length || VS.viewer.admin) {
      var ln = ls.map(function (l) { return l.name; }).join(', ') || wsT("js.tk.new.labels.none");
      h += chip('tknlab', 'labels', wsT("js.tk.new.labels", ln), '<svg class="i"><use href="#tki-tag"/></svg>' + ls.slice(0, 3).map(function (l) { return '<span class="ldot" style="background:' + esc(l.color) + '"></span>'; }).join('') +
        (ls.length ? '<span>' + esc(ls.length === 1 ? ls[0].name : wsTn("js.tk.new.labels.n", ls.length)) + '</span>' : ''), !ls.length);
    }
    return h;
  }
  function newRowHtml() {
    var n = ui.nrow[ui.store];
    if (!n) return '';
    nrowFill(n);
    var kl = wsT(n.kind === 'topic' ? "js.tk.new.kind.topic" : "js.tk.new.kind.task");
    return '<div class="nrow"' + (n.depth ? ' style="--ind:' + n.depth + '"' : '') + '><button type="button" class="nkind" id="tknkind"' + attr('act', 'nkind') + ' aria-label="' + esc(kl) + '"' + tipAttr(kl) + '>' + SI.svg(n.kind === 'topic' ? 'topic' : 'todo') + '</button>' +
      '<input type="text" id="tknew" data-keep autocomplete="off" value="' + esc(n.val) + '" placeholder="' + wsT("js.tk.new.ph") + '" aria-label="' + wsT("js.tk.new.aria") + '">' +
      nrowChips(n) +
      '<button type="button" class="ia"' + attr('act', 'new-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // The kind toggle (task <-> topic): a topic takes one holder, so several picked people fall back
  // to the viewer; its status, priority, and due chips hide (createNew leaves them out).
  function nrowKind(kind) {
    var n = ui.nrow[ui.store];
    if (!n) return;
    nrowFill(n).kind = kind;
    if (kind === 'topic' && n.who.length !== 1) n.who = [VS.viewer.member];
    render();
    if ($('tknkind')) $('tknkind').focus({ preventScroll: true });
  }
  // A chip's picker: the item menu's own step (MN.kind 'new') over the row's draft instead of an item.
  // ed: the chip is the edit row's (ui.ned), not the new row's
  function newMenu(step, anchor, ed) {
    var n = ed ? ui.ned : ui.nrow[ui.store];
    if (!n) return;
    nrowFill(n);
    if (step === 'assign') {
      if (MN) closeMenu(false);
      openPicker('assign', { draft: true, ed: !!ed }, anchor);
      return;
    }
    var as = null;
    // the label picker edits the draft live: a discard goes back to this snapshot
    openMenu({ kind: 'new', ed: !!ed, step: step, from: null, q: '', as: as, lsnap: step === 'labels' ? (n.labels || []).slice() : null }, anchor);
  }
  // the draft the open chip picker works on: the edit row's, else the new row's
  function nd() { return MN && MN.kind === 'new' && MN.ed ? ui.ned : ui.nrow[ui.store]; }
  // the item the asg* steps read: the row's draft (no records, no id) for a chip's picker
  function draftItem() {
    var n = nd();
    return n ? { id: '', kind: n.kind, title: n.val.trim() || wsT("js.tk.new.item"), assignments: [], draft: true } : null;
  }
  function mnItem() { return MN.kind === 'new' ? draftItem() : VS.items[MN.item]; }
  // a menu whose subject left closes on the next render (a store menu's store, see dropGone)
  function mnStale() { return MN.kind === 'store' ? false : MN.kind === 'new' ? !nd() : !VS.items[MN.item]; }
  function newMenuHtml() {
    var n = nd(), d = draftItem();
    if (!n) return '';
    var h = '<div class="mh"><b>' + esc(d.title) + '</b></div>';
    if (MN.step === 'status') {
      var nst = nrowSt(n);
      return h + '<div class="mst" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + nrowStOpts(n).map(function (s) {
        var on = s === nst;
        return '<button type="button" class="mr" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-status') + attr('status', s) + (on ? ' data-autofocus' : '') + '>' + SI.svg(s) + '<span>' + esc(stLabel(s)) + '</span></button>';
      }).join('') + '</div>';
    }
    if (MN.step === 'due') return h + rangeHtml(rangeOf(MN));
    if (MN.step === 'pri') return h + priGroup(nrowPri(n));
    if (MN.step === 'labels') return h + newLabelsHtml(n);
    return h + asgHtml(d);
  }
  // The label chip's picker: the store's labels as toggles (the menu stays open across picks).
  // An admin also adds, renames, and removes labels here, through the store settings' own label
  // dialog and remove path (labelRemove), which change the list for every item.
  function newLabelsHtml(n) { return labelChecklist(n.labels || []); }
  // The checklist both pickers share (the quick-add row's and the item menu's labels step): a
  // checkbox, colour dot, and name per label, cur the picked ids; m-ltog toggles.
  function labelChecklist(cur) {
    var ls = VS.labels || [], admin = !!VS.viewer.admin;
    var rows = ls.map(function (l) {
      var on = cur.indexOf(l.id) >= 0;
      return '<div class="mlrow"><button type="button" class="mi mr" role="menuitemcheckbox" aria-checked="' + on + '" data-mi' + attr('act', 'm-ltog') + attr('label', l.id) + '><span class="cbx"></span>' +
        '<span class="ldot" style="background:' + esc(l.color) + '"></span><span class="ml">' + esc(l.name) + '</span></button>' +
        (admin ? '<button type="button" class="ia" data-mi' + attr('act', 'm-ledit') + attr('label', l.id) + ' aria-label="' + esc(wsT("js.tk.new.labels.edit", l.name)) + '"' + tipAttr(wsT("js.tk.new.labels.edit", l.name)) + '><svg class="i"><use href="#tki-pen"/></svg></button>' +
          '<button type="button" class="ia danger" data-mi' + attr('act', 'm-lrm') + attr('label', l.id) + ' aria-label="' + esc(wsT("js.tk.new.labels.rm", l.name)) + '"' + tipAttr(wsT("js.tk.new.labels.rm", l.name)) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</div>';
    }).join('');
    return '<div role="group" aria-label="' + esc(wsT("js.tk.set.labels")) + '">' + (rows || '<p class="mempty">' + wsT("js.tk.set.labels.none") + '</p>') + '</div>' +
      (admin ? msep() + mi('m-ladd', esc(wsT("js.tk.new.labels.add")), {}, { icon: 'plus' }) : '');
  }
  function newAct(k, a) {
    var n = nd();
    if (!n) return closeMenu(false);
    if (k === 'm-status') n.st = a.getAttribute('data-status');
    else if (k === 'm-pri') n.pri = Number(a.getAttribute('data-p'));
    else if (k === 'm-ltog') {
      var lid = a.getAttribute('data-label'), ls = n.labels = (n.labels || []).slice(), li = ls.indexOf(lid);
      if (li >= 0) ls.splice(li, 1); else ls.push(lid);
      return newLabelsRedraw(lid);
    } else if (k === 'm-ladd' || k === 'm-ledit') {
      closeMenu(false);
      return openDialog('slabel', k === 'm-ledit' ? { label: a.getAttribute('data-label') } : {});
    } else if (k === 'm-lrm') {
      var menu = MN;
      return labelRemove(a.getAttribute('data-label'), a, function () { if (MN === menu) newLabelsRedraw(null); });
    } else return;
    newPicked();
  }
  // the label picker after a toggle or a removal: the row's chip and the open picker redraw, and
  // focus stays on the toggled label (the first item when gone)
  function newLabelsRedraw(lid) {
    render();
    if (!MN || MN.kind !== 'new' || MN.step !== 'labels') return;
    var bk = MN.backEl && MN.backEl.id && $(MN.backEl.id);
    if (bk) { MN.backEl = bk; bk.setAttribute('aria-expanded', 'true'); }
    $('tkmenu').innerHTML = newMenuHtml();
    placeMenu();
    var f = (lid && $('tkmenu').querySelector('[data-act="m-ltog"][data-label="' + CSS.escape(lid) + '"]')) || $('tkmenu').querySelector('[data-mi]');
    if (f) f.focus({ preventScroll: true });
  }
  // a chip's pick lands: the row draws again and focus goes back to the chip (the title when gone)
  function newPicked() {
    var back = MN && MN.backEl ? MN.backEl.id : '', ed = MN && MN.ed;
    closeMenu(false);
    render();
    var el = (back && $(back)) || $(ed ? 'tkned' : 'tknew');
    if (el) el.focus({ preventScroll: true });
  }
  function newBtn() {
    return '<button type="button" class="nadd"' + attr('act', 'create') + ' aria-label="' + wsT("js.tk.drawer.new") + '"' + tipAttr(wsT("js.tk.drawer.new.tip")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
  }
  function openNewRow() {
    if (!VS) return;
    ui.nrow[ui.store] = nrowNew(ui.nrow[ui.store]);
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    render();
    if ($('tknew')) $('tknew').focus();
  }
  // after a render: a row opened by the hover insert goes back against its row; with that row
  // gone from the list it ends the list, still placed by its parent and after
  function placeNew() {
    var n = ui.nrow[ui.store], list = n && n.at && document.querySelector('#card-items .plist');
    if (!list) return;
    var at = list.querySelector(n.at.item ? '.irow[data-item="' + CSS.escape(n.at.item) + '"]' : '[data-rk="' + CSS.escape(n.at.rk) + '"]');
    if (at) at.insertAdjacentHTML(n.at.before ? 'beforebegin' : 'afterend', newRowHtml());
    else list.querySelector('.nadd').insertAdjacentHTML('beforebegin', newRowHtml());
  }
  function closeNewRow() { delete ui.nrow[ui.store]; render(); var b = document.querySelector('.tk [data-act="create"]'); if (b) b.focus(); }
  function createNew() {
    var n = ui.nrow[ui.store], title = $('tknew').value.trim(), v = VS.viewer.member;
    if (!n || !title) return;
    nrowFill(n);
    var cp = { kind: n.kind, title: title }, b, ls = nrowLabels(n);
    if (n.parent) cp.parent = n.parent;
    if (n.after != null) cp.after = n.after;
    if (ls.length) cp.labels = ls.map(function (l) { return l.id; });
    if (cp.kind === 'topic') b = heldTopic(cp, n.who.length === 1 ? n.who[0] : v);
    else {
      if (n.due) cp.due = n.due;
      if (n.start) cp.start = n.start;
      if (nrowPri(n) !== VS.priority.default) cp.priority = nrowPri(n);
      // anyone but the viewer alone: the assign (one per completion class unless the picker set
      // one, asgEvents), then for a superior's wrap-up pick one status per assignee
      var st = nrowSt(n);
      if (nrowMine(n)) b = asIdea(cp, st);
      else {
        b = [['item.create', cp]].concat(asgEvents('$1', n.who.map(function (id) { return { t: 'p', id: id }; }), n.compl));
        if (nrowStMode(n) === 'wrap' && st !== 'todo') n.who.forEach(function (id) { b.push(['item.status', { item: '$1', assignee: id, status: st }]); });
      }
    }
    commit(b, null, function (res) {
      n.val = '';
      if (n.at && res[0] && res[0].status === 'applied') { n.after = res[0].id; n.at = { item: res[0].id }; }
    });
  }
  // (560) a quick-added task lands before todo: the create, then the creator's status idea (or the
  // status chip's pick) in the same write. No assign: the store holds a task nobody else holds for
  // its creator.
  function asIdea(cp, s) {
    return [['item.create', cp], ['item.status', { item: '$1', status: s || 'idea' }]];
  }
  // (529) a topic made here is its holder's at once (the creator unless the assignee chip picked
  // another): the create, then the assign that makes them its one holder (a topic has no fallback
  // holder; the store ignores its completion)
  function heldTopic(cp, holder) {
    return [['item.create', cp], ['item.assign', { item: '$1', mode: 'chosen', people: [holder], completion: 'assignee' }]];
  }
  // The "all" kanban's idea column "+": an input row atop that column (ui.knew[store] = { val }).
  // Title + Enter creates a creator-held task at idea (asIdea). The row stays for the next one;
  // Esc closes it.
  function knewRowHtml(n) {
    return '<div class="nrow"><svg class="i"><use href="#tki-plus"/></svg><input type="text" id="tkknew" data-keep autocomplete="off" value="' + esc(n.val) + '" placeholder="' + wsT("js.tk.kanban.new.ph") + '" aria-label="' + wsT("js.tk.kanban.new") + '">' +
      '<button type="button" class="ia"' + attr('act', 'knew-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  function openKnew() {
    if (!VS) return;
    ui.knew[ui.store] = ui.knew[ui.store] || { val: '' };
    render();
    if ($('tkknew')) $('tkknew').focus();
  }
  function closeKnew() { delete ui.knew[ui.store]; render(); var b = document.querySelector('.tk [data-act="knew"]'); if (b) b.focus(); }
  function createKnew() {
    var n = ui.knew[ui.store], title = $('tkknew').value.trim();
    if (!n || !title) return;
    commit(asIdea({ kind: 'task', title: title }), null, function () { n.val = ''; });
  }

  // The hover insert: in a list view, the pointer near the line between two rows (or under the
  // last one) shows a "+" badge at the left edge with a guide line. The places a new item can
  // take there (insOpts) run from b's depth (the row below must stay below) to one level under
  // a (its first child); the pointer's x over the left gutter picks the one whose badge is
  // nearest, left to right = shallow to deep. A marker at every other level slot from the root to
  // one under a (.hdep, insMarks) shows where the badge can move: a dim tick where a place is, a
  // small x where none is (above b's depth, or under a parent that cannot take the item); the slots
  // deeper than the badge are drawn too, and the guide line starts past the deepest. The row that
  // would parent the badge's place carries an outline (.hpar, insPar). A click opens the input row
  // in the gap.
  var IND = 16, INS_EDGE = 5; // a level's indent (tracker.css .ir-main, .hins), the gap's half height
  var HV = null; // the shown insert: { list, a, b, y, opts, pick }
  var HP = null; // the row outlined as the shown insert's parent (.hpar)
  function rowDepth(el) { return el ? Number(el.style.getPropertyValue('--ind')) || 0 : 0; }
  function rowLike(el) { return el && el.matches && el.matches('.irow, .csum, .tmore') ? el : null; }
  // ok(parent id, or null for the top level): may the place's parent take the item; a create's
  // rule (canSplit) by default, the moved items' (mvOk) for a drop
  function insOk(pid) { return !pid || (!!VS.items[pid] && canSplit(VS.items[pid])); }
  function insOpts(a, b, ok) {
    var db = b ? rowDepth(b) : 0, out = [];
    ok = ok || insOk;
    if (!a) return [{ depth: 0, parent: null, after: '' }]; // above the list's first row: the first root item
    var it = VS.items[a.getAttribute('data-item')], da = rowDepth(a);
    if (!it || !a.matches('.irow')) return out;
    if (ok(it.id)) out.push({ depth: da + 1, parent: it.id, after: '' });
    // after a, or after one of its ancestors: the item lands below that ancestor's subtree, which ends at a
    for (var x = it, d = da; x && d >= db; d--) {
      var p = x.parent ? VS.items[x.parent] : null;
      if (ok(x.parent || null)) out.push({ depth: d, parent: x.parent || null, after: x.id });
      x = p;
    }
    return out.sort(function (m, k) { return m.depth - k.depth; });
  }
  function insPick(opts, rel) {
    var best = 0;
    opts.forEach(function (o, i) { if (Math.abs(rel - (12 + o.depth * IND)) < Math.abs(rel - (12 + opts[best].depth * IND))) best = i; });
    return best;
  }
  // the depth markers' markup for the shown insert: one per level slot 0..max but the badge's
  var INS_X = '<svg viewBox="0 0 8 8"><path d="M1.5 1.5l5 5M6.5 1.5l-5 5"/></svg>';
  function insMarks(opts, pick, max) {
    var h = '';
    for (var d = 0; d <= max; d++) {
      if (d === pick) continue;
      var ok = opts.some(function (o) { return o.depth === d; });
      h += '<span class="' + (ok ? 'hk' : 'hx') + '" style="--d:' + d + '">' + (ok ? '' : INS_X) + '</span>';
    }
    return h;
  }
  // the row that would parent place o in the gap below row a: the nearest row above the gap one
  // level shallower than the place, when it shows the place's parent; none for a root place
  function insParRow(a, o) {
    if (!a || !o.parent) return null;
    for (var r = a; r; r = r.previousElementSibling) {
      if (!r.matches('.irow')) continue;
      var d = rowDepth(r);
      if (d < o.depth - 1) return null;
      if (d === o.depth - 1) return r.getAttribute('data-item') === o.parent ? r : null;
    }
    return null;
  }
  function insPar(el) {
    if (HP === el) return;
    if (HP) HP.classList.remove('hpar');
    if (el) el.classList.add('hpar');
    HP = el;
  }
  function insWidth(opts) { return 4 + opts[opts.length - 1].depth * IND + 40; } // the gutter: past the deepest badge by a little
  // pointer at (x, y) over row el in list: the gap it is near, or null
  function insGap(list, el, y) {
    var r = el.getBoundingClientRect(), a, b;
    if (y >= r.bottom - INS_EDGE) { a = el; b = rowLike(el.nextElementSibling); }
    else if (y <= r.top + INS_EDGE) {
      a = rowLike(el.previousElementSibling); b = el;
      if (!a && list.querySelector('.irow') !== el) return null; // the top of a later section: no place a create can name
    } else return null;
    return { a: a, b: b, y: (a ? a.getBoundingClientRect().bottom : r.top) - list.getBoundingClientRect().top };
  }
  function insShow(list, g, x) {
    var opts = insOpts(g.a, g.b), rel = x - list.getBoundingClientRect().left;
    if (!opts.length || rel > insWidth(opts)) return insHide();
    HV = { list: list, a: g.a, b: g.b, y: g.y, opts: opts, pick: insPick(opts, rel) };
    var s = list.querySelector(':scope > .hins');
    if (!s) {
      list.insertAdjacentHTML('beforeend', '<div class="hins" aria-hidden="true"><span class="hdep"></span><span class="hl"></span><span class="hb"><svg class="i"><use href="#tki-plus"/></svg></span>' +
        '<button type="button" class="hhit" tabindex="-1"' + attr('act', 'ins') + ' aria-label="' + wsT("js.tk.ins.aria") + '"></button></div>');
      s = list.querySelector(':scope > .hins');
    }
    var o = opts[HV.pick], max = g.a ? rowDepth(g.a) + 1 : 0, marks = insMarks(opts, o.depth, max), dep = s.querySelector('.hdep');
    s.style.top = Math.round(g.y - INS_EDGE) + 'px';
    s.style.setProperty('--ind', o.depth);
    s.style.setProperty('--hmax', max);
    s.classList.toggle('hdeep', max > o.depth);
    if (dep.innerHTML !== marks) dep.innerHTML = marks;
    s.querySelector('.hhit').style.width = insWidth(opts) + 'px';
    s.hidden = false;
    list.classList.add('hins-on');
    insPar(insParRow(g.a, o));
  }
  function insHide() {
    if (!HV) return;
    var s = HV.list.querySelector(':scope > .hins');
    if (s) s.hidden = true;
    HV.list.classList.remove('hins-on');
    insPar(null);
    HV = null;
  }
  function insMove(e) {
    var t = e.target, list = own(t) && t.closest('#card-items[data-view="list"] .plist');
    if (!list || lastPointer === 'touch') return insHide();
    if (t.closest('.hins')) { if (HV) insShow(list, HV, e.clientX); return; }
    var row = t.closest('.irow, .csum, .tmore'), g = row && list.contains(row) && !t.closest('.ir-more') && insGap(list, row, e.clientY);
    if (g) insShow(list, g, e.clientX); else insHide();
  }
  function insOpen() {
    if (!HV) return;
    var o = HV.opts[HV.pick], at = HV.a ? { rk: HV.a.getAttribute('data-rk') } : { rk: HV.b.getAttribute('data-rk'), before: true }, n = ui.nrow[ui.store];
    insHide();
    ui.nrow[ui.store] = nrowNew(n, { parent: o.parent, after: o.after, at: at, depth: o.depth });
    render();
    if ($('tknew')) $('tknew').focus();
  }
  // The move drag: a list row (or the selection, when a selected row is dragged) dropped on a row of
  // the Items list moves there with its subtree (ticket decisions 4, 5, 7). A row's middle band makes
  // it the row's last child, after the last child the list shows; the top and bottom bands make it
  // the sibling before or after the row: the hover insert's places (insOpts) at the row's depth, or,
  // on the bottom band of a row whose children show, its first child. In a filtered list `after`
  // names the shown row above, so siblings the list leaves out keep their places. Several roots land
  // one after another in tree order; the roots the drop refuses stay and are named (the indicator
  // counts them before the drop), the others move. A drop the store would refuse for every root, or
  // that would change nothing, is not offered. The kanban drag is apart: its cards carry data-drag
  // and text/plain, the rows draggable and MV_TYPE.
  var MV_TYPE = 'application/x-ws-tracker-move';
  var MV = null;  // while a row drags: { roots }
  var MVS = null; // the drop shown: { list, d (mvDrop's answer) }
  function hasMv(e) { return !!MV && !!e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], MV_TYPE) >= 0; }
  // does the item sit in root's subtree (root itself included)
  function underRoot(id, root) {
    for (var cur = id, n = 0; cur && VS.items[cur] && n < 1000; cur = VS.items[cur].parent, n++) if (cur === root) return true;
    return false;
  }
  function mvOk(pid) { return MV.roots.some(function (r) { return !moveWhy(r, pid || null); }); }
  // the input rows a list may hold between rows (an anchor scan steps over them)
  function rowGap(x) { return x && x.matches && x.matches('.nrow, .subrow'); }
  // the shown sibling above row r (same depth, same parent) whose item is not moving, '' when none shows
  function mvPrevSib(r, moving) {
    var d = rowDepth(r), par = VS.items[r.getAttribute('data-item')].parent || null;
    for (var x = r.previousElementSibling; x; x = x.previousElementSibling) {
      if (rowGap(x)) continue;
      if (!rowLike(x) || rowDepth(x) < d) return '';
      var xi = rowDepth(x) === d && x.matches('.irow') && VS.items[x.getAttribute('data-item')];
      if (xi && (xi.parent || null) === par && !moving[xi.id]) return xi.id;
    }
    return '';
  }
  // the last child row shown under row whose item is not moving; undefined when none shows (then the item goes last)
  function mvLastKid(row, moving) {
    var d = rowDepth(row), last;
    for (var x = row.nextElementSibling; x; x = x.nextElementSibling) {
      if (rowGap(x)) continue;
      if (!rowLike(x) || rowDepth(x) <= d) break;
      if (rowDepth(x) === d + 1 && x.matches('.irow') && !moving[x.getAttribute('data-item')]) last = x.getAttribute('data-item');
    }
    return last;
  }
  // An anchor the drop's own roots hold moves with them: step up to the shown sibling above it.
  // from: a row at or below the anchor's row (the nearest row of the item is the one meant).
  function mvAnchor(from, after, moving) {
    for (var r = from; after && moving[after];) {
      while (r && !(r.matches && r.matches('.irow') && r.getAttribute('data-item') === after)) r = r.previousElementSibling;
      after = r ? mvPrevSib(r, moving) : '';
    }
    return after;
  }
  // The place a band of row takes: { parent, after, depth (the indicator's indent), from }, or null.
  function mvSlot(list, row, band) {
    var d = rowDepth(row), opts, o, from = row;
    if (band === 'mid') return { parent: row.getAttribute('data-item'), after: undefined, depth: d + 1, from: row };
    if (band === 'top') {
      var a = rowLike(row.previousElementSibling);
      if (!a && list.querySelector('.irow') !== row) return null; // the top of a later section: no place to name (insGap's rule)
      while (a && !a.matches('.irow')) a = rowLike(a.previousElementSibling); // past a "… N more" or "closed N" row
      opts = insOpts(a, row, mvOk); from = a;
      o = opts.filter(function (x) { return x.depth === d; })[0];
    } else {
      var b = row.nextElementSibling;
      while (rowGap(b)) b = b.nextElementSibling;
      opts = insOpts(row, rowLike(b), mvOk);
      o = opts.filter(function (x) { return x.depth === d; })[0] || opts.filter(function (x) { return x.depth === d + 1; })[0];
    }
    return o ? { parent: o.parent, after: o.after, depth: o.depth, from: from } : null;
  }
  // The drop the pointer at clientY over row of list would make: { el, band, slot, n (roots it moves),
  // total }, or null when it moves nothing.
  function mvDrop(list, row, y) {
    var id = row.getAttribute('data-item'), roots = MV.roots;
    if (!VS.items[id] || roots.some(function (r) { return underRoot(id, r); })) return null; // onto what moves
    var rc = row.getBoundingClientRect(), f = (y - rc.top) / (rc.height || 1), mid = mvOk(id);
    var band = f < (mid ? 0.25 : 0.5) ? 'top' : f >= (mid ? 0.75 : 0.5) ? 'bot' : 'mid';
    var slot = mvSlot(list, row, band);
    if (!slot) return null;
    var moving = {}, n = 0, stuck = [];
    roots.forEach(function (r) {
      var why = moveWhy(r, slot.parent || null);
      if (!why) { moving[r] = true; n++; } else stuck.push(wsT("js.tk.mv.stays", VS.items[r] ? VS.items[r].alias : alias(r), why));
    });
    if (!n) return null;
    slot.after = band === 'mid' ? mvLastKid(row, moving) : mvAnchor(slot.from, slot.after, moving);
    if (roots.length === 1 && moveNoop(roots[0], slot)) return null;
    return { el: row, band: band, slot: { parent: slot.parent || null, after: slot.after, depth: slot.depth }, n: n, total: roots.length, stuck: stuck };
  }
  // The indicator: a line at the gap, indented to the place's depth, or the target row outlined
  // for the middle band; "N of M move" when some roots stay, naming each with its reason before
  // the drop (Decision 7).
  function mvShow(list, d) {
    if (MVS && MVS.list !== list) mvHide();
    var s = list.querySelector(':scope > .mvdrop');
    if (!s) {
      list.insertAdjacentHTML('beforeend', '<div class="mvdrop" aria-hidden="true"><span class="hl"></span><span class="mvn"></span></div>');
      s = list.querySelector(':scope > .mvdrop');
    }
    var lt = list.getBoundingClientRect().top, r = d.el.getBoundingClientRect(), mid = d.band === 'mid';
    s.classList.toggle('in', mid);
    s.style.top = Math.round((mid || d.band === 'top' ? r.top : r.bottom) - lt - (mid ? 0 : 1)) + 'px';
    s.style.height = mid ? Math.round(r.height) + 'px' : '';
    s.style.setProperty('--ind', mid ? rowDepth(d.el) : d.slot.depth);
    s.querySelector('.mvn').textContent = d.n < d.total ? wsT("js.tk.mv.part", d.n, d.total) + ': ' + d.stuck.join('; ') : '';
    s.hidden = false;
    MVS = { list: list, d: d };
  }
  function mvHide() {
    if (!MVS) return;
    var s = MVS.list.querySelector(':scope > .mvdrop');
    if (s) s.hidden = true;
    MVS = null;
  }
  function mvEnd() {
    MV = null;
    mvHide();
    Array.prototype.forEach.call(document.querySelectorAll('.tk .mvsrc, .tk .mv-on'), function (x) { x.classList.remove('mvsrc', 'mv-on'); });
  }
  // dragstart on a row: the selection when the row is selected, else the row's item
  function mvStart(e, row) {
    var id = row.getAttribute('data-item'), roots = mselHas(id) ? moveRoots(mselIds()) : [id];
    var stuck = roots.filter(function (r) { return !VS.items[r] || !canMove(VS.items[r]); });
    if (!VS.items[id] || stuck.length === roots.length) {
      e.preventDefault();
      toast(stuck.map(function (r) { return wsT("js.tk.mv.stays", esc(VS.items[r] ? VS.items[r].alias : alias(r)), esc(moveWhy(r, null))); }).join('<br>'));
      return;
    }
    MV = { roots: roots };
    e.dataTransfer.setData(MV_TYPE, roots.join(' '));
    e.dataTransfer.effectAllowed = 'move';
    // the row is the drag image, also when the drag began on its title link
    var rc = row.getBoundingClientRect();
    if (e.dataTransfer.setDragImage) e.dataTransfer.setDragImage(row, Math.max(0, e.clientX - rc.left), Math.max(0, e.clientY - rc.top));
    insHide(); hideTip();
    var list = row.closest('#card-items');
    // after the drag image is taken: the moving rows (subtrees included) dim, and the row previews stay shut
    setTimeout(function () {
      if (!MV || !list) return;
      list.classList.add('mv-on');
      Array.prototype.forEach.call(list.querySelectorAll('.irow'), function (x) { var xi = x.getAttribute('data-item'); if (roots.some(function (r) { return underRoot(xi, r); })) x.classList.add('mvsrc'); });
    }, 0);
  }
  function mvOver(e) {
    if (!hasMv(e)) return;
    var t = e.target, list = own(t) && t.closest && t.closest('#card-items[data-view="list"] .plist'), row = list && t.closest('.irow');
    var d = row && list.contains(row) && VS ? mvDrop(list, row, e.clientY) : null;
    if (!d) { mvHide(); return; }
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    mvShow(list, d);
  }
  function mvDropped(e) {
    if (!hasMv(e)) return;
    e.preventDefault();
    var t = e.target, list = own(t) && t.closest && t.closest('#card-items[data-view="list"] .plist'), row = list && t.closest('.irow');
    var d = row && list.contains(row) && VS ? mvDrop(list, row, e.clientY) : null, roots = MV.roots;
    mvEnd();
    if (d) moveApply(roots, d.slot);
  }

  // "Add subtask": an input row one level in, right under the row the menu opened from (or atop
  // the item window's Subtasks for the window header); ui.sub = { item: parent, rk: that row's
  // key, val }. Title + Enter creates a task under the parent, its creator's like any task made here
  // (529), and keeps the row for the next sibling; Esc closes it and returns to the row.
  function subRowHtml(ind) {
    var it = VS.items[ui.sub.item];
    return '<div class="subrow" style="--ind:' + ind + '"><span class="ind">&#8627;</span><input type="text" id="tksub" data-keep autocomplete="off" value="' + esc(ui.sub.val) + '" placeholder="' + wsT("js.tk.sub.ph") + '" aria-label="' + esc(wsT("js.tk.sub.aria", it ? it.title : '')) + '">' +
      '<button type="button" class="ia"' + attr('act', 'sub-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.dlg.cancel")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // after a render: the subtask row goes under its anchor (the item window draws its own)
  function placeSub() {
    if (!ui.sub) return;
    if (!VS.items[ui.sub.item]) { ui.sub = null; return; }
    if (ui.sub.rk === 'iw:' + ui.sub.item) return;
    var at = document.querySelector('.tk:not(#tkmenu) [data-rk="' + CSS.escape(ui.sub.rk) + '"]');
    if (!at) return;
    var ie = at.style.getPropertyValue('--ind') ? at : at.querySelector('[style*="--ind"]');
    at.insertAdjacentHTML('afterend', subRowHtml((ie ? Number(ie.style.getPropertyValue('--ind')) || 0 : 0) + 1));
  }
  function closeSub() {
    var rk = ui.sub && ui.sub.rk;
    ui.sub = null; render();
    var el = rk && document.querySelector('.tk [data-rk="' + CSS.escape(rk) + '"]');
    if (el && !el.matches('[tabindex], button, a[href]')) el = el.querySelector('[tabindex], button, a[href]');
    if (el) el.focus();
  }
  function createSub() {
    var s = ui.sub, title = $('tksub').value.trim();
    if (!s || !title) return;
    commit([['item.create', { kind: 'task', title: title, parent: s.item }]], null, function () { s.val = ''; });
  }
  // The edit mode's label picker: every label of the store's list, each a toggle; the picked
  // ids live in ui.iedit.labels so a re-render keeps them.
  function labelPickHtml(ed) {
    var ls = VS.labels || [];
    if (!ls.length) return '';
    return '<div class="lpick" role="group" aria-label="' + esc(wsT("js.tk.form.labels")) + '"><span class="muted">' + wsT("js.tk.form.labels") + '</span>' + ls.map(function (l) {
      var on = ed.labels.indexOf(l.id) >= 0;
      return labelToggle(l, on, 'ilabel', ' aria-pressed="' + on + '"');
    }).join('') + '</div>';
  }
  // One label toggle chip (the edit mode's and the labels step's); aria is its state attribute.
  function labelToggle(l, on, act, aria) {
    return '<button type="button" class="chip lbl' + (on ? ' on' : '') + '"' + attr('act', act) + attr('label', l.id) + aria + '><span class="ldot" style="background:' + esc(l.color) + '"></span>' + esc(l.name) + '</button>';
  }
  // The item menu's labels step (a list row's label icon, the property card's labels button): the
  // quick-add picker's checklist (labelChecklist) over the item's current labels with the step's
  // toggles laid over (MN.lset: label id -> wanted on, labelsOver). A toggle only records the
  // user's pick (m-ltog -> labelStepRedraw); the toggles are one write when the menu closes
  // (labelPending, called by closeMenu), applied to the item's labels as they are then, none
  // without a toggle or when they change nothing. So a label another viewer adds while the step is
  // open, or a write still in flight from the last close, is kept: only the picked labels move. A
  // refused write leaves the item as the server holds it, the row never drew the pick. With no
  // labels in the store it says so, and an admin gets the store settings' label dialog.
  function labelStepHtml(it) {
    if (!(VS.labels || []).length) return '<p class="mempty">' + wsT("js.tk.labels.empty") + '</p>' + (VS.viewer.admin ? msep() + mi('m-ladd', esc(wsT("js.tk.new.labels.add")), {}, { icon: 'plus' }) : '');
    return labelChecklist(labelsOver(labelBase(it), MN.lset));
  }
  // [store, item id] -> each in-flight write's own label picks, laid over the live labels.
  // Settling one write removes only its picks, leaving any others still in flight.
  var LPEND = {};
  function labelBase(it) { return (LPEND[JSON.stringify([ui.store, it.id])] || []).reduce(function (ls, set) { return labelsOver(ls, set); }, it.labels); }
  // A labels step's write (labelPending's events): pending for its item until it lands or is refused.
  function labelWrite(ev, then) {
    var id = ev[0][1].item, key = JSON.stringify([ui.store, id]), set = labelPicks(labelBase(VS.items[id]), ev[0][1].labels);
    var pending = LPEND[key] || (LPEND[key] = []);
    pending.push(set);
    function settled() { pending.splice(pending.indexOf(set), 1); if (!pending.length) delete LPEND[key]; }
    commit(ev, then, settled, settled);
  }
  // Labels cur with the picks set (label id -> wanted on) laid over, in the store's list order
  // (labels no longer listed drop).
  function labelsOver(cur, set) {
    return (VS.labels || []).map(function (l) { return l.id; }).filter(function (x) {
      return set && Object.prototype.hasOwnProperty.call(set, x) ? !!set[x] : (cur || []).indexOf(x) >= 0;
    });
  }
  // Picks set with lid flipped from how the step draws it over labels cur (labelsOver).
  function labelFlip(set, cur, lid) {
    set = set || {};
    set[lid] = labelsOver(cur, set).indexOf(lid) < 0;
    return set;
  }
  // The picks (labelsOver's set) that turn labels from into labels to: the ones added and dropped.
  function labelPicks(from, to) {
    var set = {};
    (from || []).forEach(function (x) { if ((to || []).indexOf(x) < 0) set[x] = false; });
    (to || []).forEach(function (x) { if ((from || []).indexOf(x) < 0) set[x] = true; });
    return set;
  }
  // Whether labels a and b hold the same listed labels.
  function sameLabels(a, b) {
    function key(l) { return (l || []).filter(function (x) { return !!labelOf(x); }).sort().join(' '); }
    return key(a) === key(b);
  }
  // The write closing menu m owes: its toggles over the item's current labels, null when it has
  // none, they change nothing, or the item or the edit right is gone.
  function labelPending(m) {
    var it = m.kind === 'item' && m.step === 'labels' && m.lset && VS && VS.items[m.item];
    if (!it || !canEdit(it)) return null;
    var base = labelBase(it), picked = labelsOver(base, m.lset);
    return sameLabels(picked, base) ? null : [['item.edit', { item: it.id, labels: picked }]];
  }
  // The store label dialog from the labels step: the pick is written first, the dialog opens once
  // that write returns (a written batch closes any open dialog).
  function labelDialog(arg) {
    var ev = labelPending(MN);
    MN.lset = null;
    closeMenu(false);
    if (ev) labelWrite(ev, function () { openDialog('slabel', arg); }); else openDialog('slabel', arg);
  }
  // The labels step after a write: the property card drew again, so the step re-anchors on its new
  // button, redraws over the written item, and keeps focus on the toggled chip. It closes when the
  // item or the viewer's edit right is gone.
  function labelStepRedraw() {
    // the opener drew again: the property card's #iw-plabels, or a list row's label icon inside the row MN.back names
    var it = VS.items[MN.item], bk = MN.back && document.querySelector(MN.back);
    if (bk && !bk.matches('[data-act="labels"]')) bk = bk.querySelector('[data-act="labels"]');
    if (!bk || !it || !canEdit(it)) return closeMenu(false);
    // expanded first: a row's label icon shows only while hovered or expanded, and the rect needs it shown
    bk.setAttribute('aria-expanded', 'true');
    MN.backEl = bk; MN.rect = bk.getBoundingClientRect();
    $('tkmenu').innerHTML = menuHtml();
    placeMenu();
    var f = (MN.lab && $('tkmenu').querySelector('[data-act="m-ltog"][data-label="' + CSS.escape(MN.lab) + '"]')) || $('tkmenu').querySelector('[data-mi]');
    if (f) f.focus({ preventScroll: true });
  }
  // Each draft has its own key so other items and stores cannot overwrite it. Storage may
  // be disabled or full; the cached copy still works for the lifetime of this page.
  function draftBucket(store) { return ui.itemDrafts[store] || (ui.itemDrafts[store] = Object.create(null)); }
  function draftKey(store, item) { return PFX + 'itemDraft.' + encodeURIComponent(JSON.stringify([store, item])); }
  function removeItemDraft(store, item) {
    draftBucket(store)[item] = null;
    try { localStorage.removeItem(draftKey(store, item)); } catch (err) { /* storage unavailable */ }
  }
  function validItemDraft(d, item) {
    return d && d.item === item && typeof d.title === 'string' && typeof d.origTitle === 'string' &&
      typeof d.body === 'string' && typeof d.seen === 'string' && typeof d.project === 'string' &&
      typeof d.origProject === 'string' && Array.isArray(d.base) && Array.isArray(d.labels) &&
      Array.isArray(d.lorig) && Array.isArray(d.mpick) && (!d.attachments || Array.isArray(d.attachments));
  }
  function keepItemDraft() {
    var e = ui.iedit;
    if (!e || !ui.store) return;
    var bucket = draftBucket(ui.store);
    if (e.title !== e.origTitle || e.body !== e.seen || e.project !== e.origProject || !sameLabels(e.labels, e.lorig)) {
      e.attachments = (ui.eatt['iw-ebody'] || []).slice();
      bucket[e.item] = JSON.parse(JSON.stringify(e));
      try { localStorage.setItem(draftKey(ui.store, e.item), JSON.stringify(bucket[e.item])); } catch (err) { /* storage unavailable */ }
    } else removeItemDraft(ui.store, e.item);
  }
  function restoreItemDraft(store, item) {
    var bucket = draftBucket(store);
    if (!Object.prototype.hasOwnProperty.call(bucket, item)) {
      var saved = null;
      try { saved = JSON.parse(localStorage.getItem(draftKey(store, item))); } catch (err) { /* storage unavailable or corrupt */ }
      bucket[item] = validItemDraft(saved, item) ? saved : null;
    }
    var d = bucket[item];
    if (!d) return null;
    var e = JSON.parse(JSON.stringify(d));
    e.restored = true;
    ui.eatt['iw-ebody'] = (e.attachments || []).slice();
    return e;
  }
  // The item window's edit mode and a comment's edit in place.
  function startEdit(id) {
    if (AD.detailReady && !AD.detailReady(ui.store, id)) {
      var store = ui.store;
      showItem(id);
      AD.detail(store, [id], function () {
        if (ui.store === store && ui.win && ui.item === id) { render(); startEdit(id); }
      });
      return;
    }
    var it = VS.items[id];
    if (!it) return;
    if (!(ui.win && ui.item === id)) showItem(id);
    // seen and base pin the body revision the editor started from; body follows the textarea
    ui.iedit = { item: id, title: it.title, origTitle: it.title, body: it.body || '', seen: it.body || '', base: (it.bodyRev || []).slice(), project: it.project || '', origProject: it.project || '', labels: (it.labels || []).slice(), lorig: (it.labels || []).slice(), mpick: [] };
    delete ui.edpv['iw-ebody']; delete ui.eatt['iw-ebody'];
    render();
    if ($('iw-etitle')) $('iw-etitle').focus();
  }
  function endEdit() { if (ui.iedit) removeItemDraft(ui.store, ui.iedit.item); ui.iedit = null; render(); var b = $('tkwinbody').querySelector('.iw-more'); if (b) b.focus(); }
  function saveEdit() {
    var e = ui.iedit, it = e && VS.items[e.item];
    if (!it) return;
    var ep = { item: it.id }, cur = { title: $('iw-etitle').value, body: $('iw-ebody').value, project: $('iw-eproj').value };
    ['title', 'project'].forEach(function (f) { if (cur[f] !== (it[f] || '')) ep[f] = cur[f]; });
    // the body is judged against the revision loaded at startEdit, not the live one a concurrent
    // edit may have moved; sent with base, the store merges it three-way with edits since
    if (cur.body !== e.seen) { ep.body = cur.body; if (e.base.length) ep.base = e.base; }
    // the edit's label picks (what it added and dropped since startEdit) over the item's labels as
    // they are now, as the row edit's save: a label another viewer wrote meanwhile is kept; labels
    // the list no longer holds are dropped, so an edit never writes one
    var picked = labelsOver(it.labels, labelPicks(e.lorig, e.labels));
    if (!sameLabels(picked, it.labels)) ep.labels = picked;
    if (!('title' in ep) && !('body' in ep) && !('project' in ep) && !('labels' in ep)) { toast(wsT("js.tk.toast.unchanged")); return endEdit(); }
    if ('body' in ep) {
      var km = keptMentions(withPicks(it.mentions, e.mpick), ep.body), ers = refsIn(ep.body);
      if (km.length) ep.mentions = km;
      if (ers.length) ep.refs = ers;
      var eas = savedAtts('iw-ebody', ep.body);
      if (eas.length) ep.attachments = eas;
    }
    keepItemDraft();
    var store = ui.store, submitted = JSON.stringify(e);
    commit([['item.edit', ep]], null, function (results) {
      if (!results.length || results.some(function (r) { return r.status !== 'applied'; })) return;
      var bucket = draftBucket(store), d = bucket[e.item];
      if (d && JSON.stringify(d) === submitted) removeItemDraft(store, e.item);
      if (ui.iedit === e && JSON.stringify(e) === submitted) { removeItemDraft(store, e.item); ui.iedit = null; }
    });
  }
  function startCedit(cid) {
    var c = VS.comments[cid];
    if (!c) return;
    ui.cedit = { cid: cid, body: c.body, mpick: [] };
    delete ui.edpv.tkcedit; delete ui.eatt.tkcedit;
    render();
    var t = $('tkcedit');
    if (t) { t.focus(); t.setSelectionRange(t.value.length, t.value.length); }
  }
  function endCedit() {
    var cid = ui.cedit && ui.cedit.cid;
    ui.cedit = null; render();
    var b = cid && $('tkwinbody').querySelector('[data-act="cedit"][data-cid="' + CSS.escape(cid) + '"]');
    if (b) b.focus();
  }
  function saveCedit() {
    var e = ui.cedit, ocm = e && VS.comments[e.cid];
    if (!ocm) return;
    var cep = { comment: e.cid, body: $('tkcedit').value };
    if (cep.body === ocm.body) { toast(wsT("js.tk.toast.unchanged")); return endCedit(); }
    var ckm = keptMentions(withPicks(ocm.mentions, e.mpick), cep.body), cers = refsIn(cep.body);
    if (ckm.length) cep.mentions = ckm;
    if (cers.length) cep.refs = cers;
    var ceas = savedAtts('tkcedit', cep.body);
    if (ceas.length) cep.attachments = ceas;
    commit([['comment.edit', cep]], null, function () { ui.cedit = null; });
  }

  // Kanban drop: only the viewer's own live assignment moves, by the own status menu's rules
  // (ownStatusOpts: done closes, a refused choice says why); an org superior's wrap-up of someone
  // else's card goes through that card's menu.
  function dropOn(item, col) {
    var v = memberOf(viewerOf(ui.store)), r = VS.items[item] && VS.items[item].assignments.map(A).filter(function (x) { return x.to === v && x.state === 'live'; })[0];
    if (!r) return toast(wsT("js.tk.toast.notmine"));
    if (col === r.status || VS.items[item].kind === 'topic') return; // a topic has no status to move
    var o = ownStatusOpts(r).filter(function (x) { return x.s === col; })[0];
    if (!o) return toast(wsT("js.tk.toast.notstatus", esc(stLabel(col))));
    if (o.dis) return toast(esc(o.why));
    if (col === 'done') return commit([['item.close', { item: item }]]);
    commit([['item.status', { item: item, status: col }]]);
  }

  // --------------------------------------------------------------- sync actions

  // The apply path (panel collapse, leaving the page, switching stores, the Sync button):
  // the adapter closes the open batch, pushes, and fetches; `quiet` skips the toast.
  function syncNow(name, quiet, then) {
    AD.sync(name, function (r) {
      r = r || {};
      if (!quiet) {
        var got = r.received || 0;
        if (r.error) toast(wsT("js.tk.toast.syncfail", esc(r.error)));
        else if (r.noRemote) toast(wsT("js.tk.toast.noremote"));
        else if (r.offline) toast(wsT("js.tk.toast.offline"));
        else toast(got ? wsTn("js.tk.toast.synced.late", got, r.pushed || 0) : wsT("js.tk.toast.synced", r.pushed || 0));
      }
      if (then) then(r);
    });
  }
  // The immediate check when a store comes on screen (its drawer opens, its page is entered,
  // its viewer changes); what it brings arrives through onChange, and done() runs once it
  // landed.
  function checkNow(name, done) { AD.check(name, done); }

