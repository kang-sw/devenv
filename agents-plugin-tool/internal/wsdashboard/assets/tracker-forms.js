  // ------------------------------------------------------- the markdown editor

  // One editor serves the item body edit (iw-ebody), the comment box (cbox), and a comment's
  // edit in place (tkcedit): a Write / Preview toggle over the textarea, which carries the @ / #
  // suggestion popup (data-mention), and a preview pane in its place while previewing. Each
  // place keeps its own save and cancel keys (the keydown handler goes by the textarea id).
  // The preview is the adapter's render of the text, the renderer the saved view uses, drawn
  // like the saved block (o.pv: its class); without render, the plain text. The textarea stays
  // in the page while hidden, so a save reads it as before.
  // o: { value, rows, label, placeholder, cls (the wrapper's extra class), pv, foot (HTML after
  // the text, inside the wrapper), src (source only: no Write / Preview toggle, no preview pane,
  // no @/# suggestions; edImages refuses images for it) }
  function editorHtml(id, o) {
    var on = !o.src && !!ui.edpv[id];
    if (o.src) {
      return '<div class="tked' + (o.cls ? ' ' + o.cls : '') + '"' + attr('ed', id) + ' data-src>' +
        '<textarea id="' + id + '" data-keep rows="' + (o.rows || 3) + '" spellcheck="false"' + (o.label ? ' aria-label="' + o.label + '"' : '') + (o.placeholder ? ' placeholder="' + o.placeholder + '"' : '') + '>' + esc(o.value || '') + '</textarea>' +
        (o.foot || '') + '</div>';
    }
    return '<div class="tked' + (o.cls ? ' ' + o.cls : '') + '"' + attr('ed', id) + '>' +
      '<div class="tked-tabs" role="group" aria-label="' + wsT("js.tk.ed.mode") + '">' +
      ['write', 'preview'].map(function (m) {
        return '<button type="button" class="tked-tab"' + attr('act', 'ed-mode') + attr('ed', id) + attr('mode', m) + ' aria-pressed="' + ((m === 'preview') === on) + '">' + (m === 'write' ? wsT("js.tk.ed.write") : wsT("js.tk.ed.preview")) + '</button>';
      }).join('') + '</div>' +
      '<textarea id="' + id + '" data-keep data-mention rows="' + (o.rows || 3) + '" aria-autocomplete="list" aria-controls="tkmpop" aria-expanded="false"' +
      (o.label ? ' aria-label="' + o.label + '"' : '') + (o.placeholder ? ' placeholder="' + o.placeholder + '"' : '') + (on ? ' hidden' : '') + '>' + esc(o.value || '') + '</textarea>' +
      '<div id="' + id + '-pv" class="' + pvClass(id, o.pv) + '"' + attr('pv', o.pv || '') + ' aria-live="polite"' + (on ? '' : ' hidden') + '>' + pvHtml(id) + '</div>' +
      (o.foot || '') + '</div>';
  }
  function pvClass(id, cls) { var st = ui.edpv[id]; return 'tked-pv' + (cls ? ' ' + cls : '') + (st && st.html != null && !st.err ? ' md' : ''); }
  function pvHtml(id) {
    var st = ui.edpv[id];
    if (!st) return '';
    if (!st.text.trim()) return '<span class="muted">' + wsT("js.tk.ed.empty") + '</span>';
    if (st.err != null) return '<span class="muted">' + wsT("js.tk.ed.failed", esc(st.err)) + '</span>';
    if (st.busy) return '<span class="muted">' + wsT("js.tk.ed.loading") + '</span>';
    return st.html != null ? refLinksHtml(st.html) : refLinks(esc(st.text));
  }
  // Switches editor id to mode ('write' or 'preview') in place, so the toggle keeps focus and a
  // re-render is not needed; a render that lands meanwhile draws the same state from ui.edpv.
  function edMode(id, mode) {
    var box = $(id), wrap = box && box.closest('.tked');
    if (!wrap || (mode === 'preview') === !!ui.edpv[id]) return;
    if (mode === 'preview') {
      if (MP) mentionClose();
      var st = ui.edpv[id] = { text: box.value, html: null, err: null, busy: false };
      if (AD.render && st.text.trim()) {
        st.busy = true;
        AD.render(ui.store, { text: st.text, attachments: edAtts(id), wiki: id === 'wk-ebody' ? wkRenderName() : undefined }, function (res) {
          if (ui.edpv[id] !== st) return; // back to write, or the editor closed
          st.busy = false;
          if (res && res.ok) st.html = String(res.html || ''); else st.err = String((res && res.error) || '');
          pvPaint(id);
        });
      }
    } else delete ui.edpv[id];
    var on = !!ui.edpv[id];
    box.hidden = on;
    $(id + '-pv').hidden = !on;
    Array.prototype.forEach.call(wrap.querySelectorAll('.tked-tab'), function (b) { b.setAttribute('aria-pressed', String((b.getAttribute('data-mode') === 'preview') === on)); });
    pvPaint(id);
    if (!on) box.focus();
  }
  function pvPaint(id) {
    var p = $(id + '-pv');
    if (!p) return;
    p.className = pvClass(id, p.getAttribute('data-pv'));
    p.innerHTML = pvHtml(id);
  }
  // An editor textarea's text kept in ui, so a re-render draws it back (the comment box keeps
  // its own draft across a render instead).
  function edKeep(t) {
    if (t.id === 'tkcedit' && ui.cedit) ui.cedit.body = t.value;
    else if (/^iw-e(title|body|proj)$/.test(t.id) && ui.iedit) { ui.iedit[{ 'iw-etitle': 'title', 'iw-ebody': 'body', 'iw-eproj': 'project' }[t.id]] = t.value; keepItemDraft(); }
    else if (t.id === 'wk-ebody' && wk().edit) wk().edit.text = t.value;
    else if (t.id === 'wk-epath' && wk().edit) wkPathKind(wk().edit, t.value);
  }

  // An image pasted or dropped into an editor (png, jpeg, gif, or webp, within the view's
  // attachments.max_bytes, checked here before any request) is uploaded at once through the
  // adapter's upload, which stages it until a write names it, then written into the text at the
  // caret as ![name](attachment:<oid>) and kept in ui.eatt[editor id]. A save sends the entries
  // whose attachment:<oid> is still in the text (savedAtts), and the preview's render request
  // carries them, so a pasted image shows before it is saved. Any other file, an oversized
  // image, and every image without upload (the mockup) get a notice and nothing else. An upload
  // that lands after its editor ended is dropped; its staged copy expires on the server.
  var IMG_TYPES = { 'image/png': 1, 'image/jpeg': 1, 'image/gif': 1, 'image/webp': 1 };
  function edImages(id, files, named) {
    var max = VS && VS.attachments ? VS.attachments.max_bytes : 0, store = ui.store;
    if (id === 'wk-ebody' && wkEditHtml(wk().edit)) return toast(wsT("js.tk.ed.img.off")); // an HTML page cannot show images
    files.forEach(function (f) {
      var label = esc((named && f.name) || wsT("js.tk.ed.img.this"));
      if (!AD.upload) return toast(wsT("js.tk.ed.img.off"));
      if (!IMG_TYPES[f.type]) return toast(wsT("js.tk.ed.img.type", label));
      if (max && f.size > max) return toast(wsT("js.tk.ed.img.big", label, fmtSize(max)));
      var list = ui.eatt[id] || (ui.eatt[id] = []);
      toast(wsT("js.tk.ed.img.busy", label));
      AD.upload(store, f, named ? f.name || '' : '', function (res) {
        var box = $(id);
        if (ui.store !== store || ui.eatt[id] !== list || !box) return; // the editor ended meanwhile
        if (!res || !res.ok || !res.attachment || !res.attachment.oid) return toast(wsT("js.tk.ed.img.fail", label, esc((res && res.error) || '')));
        var a = res.attachment;
        if (!list.some(function (x) { return x.oid === a.oid; })) list.push({ oid: a.oid, name: a.name, size: a.size, mime: a.mime });
        if (ui.edpv[id]) edMode(id, 'write');
        var alt = String(a.name || 'image').replace(/[\r\n]+/g, ' ').replace(/[\\\[\]]/g, '\\$&');
        edInsert(box, '![' + alt + '](attachment:' + a.oid + ')');
      });
    });
  }
  // Puts s at box's caret (its selection while it is not focused), spaced from the words around it.
  function edInsert(box, s) {
    var v = box.value, a = box.selectionStart, b = box.selectionEnd;
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = ' ' + s;
    if (b < v.length && !/\s/.test(v.charAt(b))) s += ' ';
    box.setRangeText(s, a, b, 'end');
    edKeep(box); // setRangeText fires no input event
    if (box.id === 'cbox' || document.activeElement !== box) box.focus();
  }
  // The attachments editor id's text already had: an edit's stored ones, the composer's
  // hand-drafted ones (caps.attach, the mockup).
  function edBase(id) {
    var o = id === 'iw-ebody' ? ui.iedit && VS.items[ui.iedit.item] : id === 'tkcedit' ? ui.cedit && VS.comments[ui.cedit.cid] : null;
    return id === 'cbox' ? ui.adraft : o ? listOf(o.attachments) : [];
  }
  // edBase plus the editor's uploads, each oid once: the list a preview renders against.
  function edAtts(id) {
    var out = edBase(id).slice();
    listOf(ui.eatt[id]).forEach(function (a) { if (!out.some(function (x) { return x.oid === a.oid; })) out.push(a); });
    return out;
  }
  // The attachments a save of editor id with text sends: those whose attachment:<oid> is still in
  // the text, as a mention lasts while its @id does. Under caps.attach the base entries are
  // metadata drafted by hand, which no text names, so they stay as before.
  function savedAtts(id, text) {
    var base = edBase(id);
    return edAtts(id).filter(function (a) { return String(text).indexOf('attachment:' + a.oid) >= 0 || (CAPS.attach && base.indexOf(a) >= 0); });
  }

  // ------------------------------------------ @-mention and #-reference suggestion popup

  // An @ or a # at the start or after whitespace in an editor's textarea opens a list anchored at it,
  // filtered by the text after it: members for @, items (by alias and title) for #. It is driven
  // by input events, which fire during an IME composition as well, so a Hangul name or title
  // filters while it is still being composed.
  var MP = null; // the open popup: { box: the textarea id, ch: '@' or '#', start: index of ch, q: the text after it, idx: highlighted row, ids }
  // the focused editor textarea t, when it is one
  function mentionBox(t) { return t && t.hasAttribute && t.hasAttribute('data-mention') && own(t) ? t : null; }
  function mentionMatches(q) {
    q = q.toLowerCase();
    var pre = [], sub = [];
    Object.keys(VS.members).forEach(function (id) {
      var m = VS.members[id], pr = m.profile || {};
      if (m.removed) return;
      var keys = [id, m.name, pr.given, pr.family].filter(Boolean).map(function (x) { return String(x).toLowerCase(); });
      if (keys.some(function (x) { return x.indexOf(q) === 0; })) pre.push(id);
      else if (keys.some(function (x) { return x.indexOf(q) >= 0; })) sub.push(id);
    });
    return pre.concat(sub);
  }
  // items for a # token: alias prefix, then title prefix, then either containing it; hidden items
  // and the open item itself left out
  function refMatches(q) {
    q = q.toLowerCase();
    var a = [], b = [], c = [];
    VS.itemOrder.forEach(function (id) {
      var it = VS.items[id], al = it.alias.toLowerCase(), ti = it.title.toLowerCase();
      if (it.hidden || id === ui.item) return;
      if (al.indexOf(q) === 0) a.push(id); else if (ti.indexOf(q) === 0) b.push(id); else if (ti.indexOf(q) >= 0 || al.indexOf(q) >= 0) c.push(id);
    });
    return a.concat(b, c).slice(0, 30);
  }
  function mentionToken(box) {
    if (box.selectionStart !== box.selectionEnd) return null;
    var m = /(^|\s)([@#])([^\s@#]*)$/.exec(box.value.slice(0, box.selectionStart));
    return m ? { ch: m[2], start: box.selectionStart - m[3].length - 1, q: m[3] } : null;
  }
  // Opens, refilters, re-places, or closes the popup after the box's text or caret changed.
  function mentionSync() {
    var box = mentionBox(document.activeElement), tok = box && mentionToken(box);
    if (!tok || (MP && MP.box !== box.id)) mentionClose();
    if (!tok) return;
    var same = MP && MP.ch === tok.ch && MP.start === tok.start && MP.q === tok.q;
    MP = { box: box.id, ch: tok.ch, start: tok.start, q: tok.q, idx: same ? MP.idx : 0, ids: tok.ch === '#' ? refMatches(tok.q) : mentionMatches(tok.q) };
    if (MP.idx >= MP.ids.length) MP.idx = 0;
    mentionDraw(box); mentionPlace(box);
  }
  function mentionDraw(box) {
    var p = $('tkmpop');
    var ref = MP.ch === '#';
    p.innerHTML = MP.ids.map(function (id, i) {
      var it = ref && VS.items[id];
      return '<li class="mopt' + (i === MP.idx ? ' on' : '') + '" role="option" id="tkmopt-' + i + '" aria-selected="' + (i === MP.idx) + '"' + attr('mp', i) + '>' +
        (ref ? '<span class="sti">' + SI.svg(stKey(it)) + '</span><span class="mn">' + esc(it.title) + '</span><span class="muted">' + esc(it.alias) + '</span>' :
          avHtml(id) + '<span class="mn">' + whoName(id) + '</span><span class="muted">' + esc(id) + '</span>') + '</li>';
    }).join('') || '<li class="qnone" role="presentation">' + (ref ? wsT("js.tk.ref.none") : wsT("js.tk.mention.none")) + '</li>';
    p.setAttribute('aria-label', ref ? wsT("js.tk.ref.tip") : wsT("js.tk.composer.mention.tip"));
    p.hidden = false;
    box.setAttribute('aria-expanded', 'true');
    box.setAttribute('aria-activedescendant', MP.ids.length ? 'tkmopt-' + MP.idx : '');
    var on = p.querySelector('.mopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
  }
  // The @'s (or #'s) position in the viewport, measured on a hidden copy of the box laid out alike.
  function mentionAnchor(box) {
    var cs = getComputedStyle(box), d = document.createElement('div'), mark = document.createElement('span');
    ['boxSizing', 'width', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
      'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing', 'lineHeight', 'textTransform', 'wordSpacing', 'tabSize'].forEach(function (k) { d.style[k] = cs[k]; });
    d.style.cssText += ';position:absolute;visibility:hidden;top:0;left:-9999px;white-space:pre-wrap;overflow-wrap:break-word;border-style:solid';
    d.textContent = box.value.slice(0, MP.start);
    mark.textContent = MP.ch; d.appendChild(mark);
    box.parentNode.appendChild(d);
    var r = box.getBoundingClientRect(), x = r.left + mark.offsetLeft - box.scrollLeft, y = r.top + mark.offsetTop - box.scrollTop, lh = mark.offsetHeight;
    d.remove();
    return { x: x, top: y, bottom: y + lh };
  }
  // above the @ when it fits, so the box's text and send button stay visible; else below it
  function mentionPlace(box) {
    var p = $('tkmpop'), a = mentionAnchor(box), w = p.offsetWidth, h = p.offsetHeight;
    p.style.left = Math.max(8, Math.min(a.x - 4, innerWidth - w - 8)) + 'px';
    p.style.top = (a.top - h - 4 >= 8 ? a.top - h - 4 : Math.min(a.bottom + 4, innerHeight - h - 8)) + 'px';
  }
  function mentionClose() {
    if (!MP) return;
    var box = $(MP.box);
    MP = null; $('tkmpop').hidden = true;
    if (box) { box.setAttribute('aria-expanded', 'false'); box.removeAttribute('aria-activedescendant'); }
  }
  // Picking replaces the @query token with "@<id> " at the caret and records the id for the
  // editor's next write (the comment box's draft, or the edit's mpick); a #query token becomes
  // "#<alias> ", which the write reads back as a ref (refsIn).
  function mentionPick(i) {
    var box = MP && $(MP.box), id = MP && MP.ids[i];
    if (!box || !id) return;
    // The pick replaces the whole token, the part after the caret too (`@ma|ry`).
    var end = box.selectionStart + /^[^\s@#]*/.exec(box.value.slice(box.selectionStart))[0].length;
    var spaced = /\s/.test(box.value.charAt(end)), ref = MP.ch === '#';
    box.setRangeText((ref ? '#' + VS.items[id].alias : '@' + id) + (spaced ? '' : ' '), MP.start, end, 'end');
    if (spaced) box.selectionStart = box.selectionEnd = box.selectionEnd + 1;
    var picks = box.id === 'iw-ebody' ? ui.iedit && ui.iedit.mpick : box.id === 'tkcedit' ? ui.cedit && ui.cedit.mpick : ui.mdraft;
    if (!ref && picks && picks.indexOf(id) < 0) picks.push(id);
    edKeep(box); // setRangeText fires no input event
    mentionClose(); box.focus();
  }
  // The @ button: an @ at the caret (after a space when it would touch a word), then the popup.
  function mentionInsert() {
    var box = $('cbox');
    if (!box) return;
    edMode('cbox', 'write');
    box.focus();
    var s = box.selectionStart;
    box.setRangeText((s && !/\s/.test(box.value.charAt(s - 1)) ? ' ' : '') + '@', s, box.selectionEnd, 'end');
    mentionSync();
  }

  // --------------------------------------------------------- dialog forms

  function optsHtml(list, empty, cur) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    return list.map(function (x) { return '<option value="' + esc(x[0]) + '"' + (String(x[0]) === String(cur) ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
  }
  function memberOpts(cur, filter) {
    return optsHtml(Object.keys(VS.members).filter(function (id) { return !filter || filter(VS.members[id]); }).map(function (id) { return [id, nmo(id) + ' (' + id + ')']; }), wsT("js.tk.common.nobody"), cur);
  }
  function reachTag(v, id) { return id === v ? wsT("js.tk.ev.self") : VS.viewer.reach[id] ? wsT("js.tk.dlg.forced", VS.viewer.reach[id]) : wsT("js.tk.st.pending"); }
  function fld(label, inner) { return '<div class="fld"><span>' + label + '</span>' + inner + '</div>'; }
  // A person's earlier profiles are machine-local, never in the ledger: the adapter keeps
  // them (profiles / saveProfile). Editing your own profile saves the result as one.
  function saveProf(pr) {
    var one = { org: String(pr.org || '').trim(), family: String(pr.family || '').trim(), given: String(pr.given || '').trim() };
    if (one.given && AD.saveProfile) AD.saveProfile(one);
  }
  function profileText(pr) { return (pr.org ? pr.org + ' ' : '') + displayName({ org: pr.org, family: pr.family, given: pr.given }); }
  function profileFields(pr) {
    return '<div class="fld inline">' + fld(wsT("js.tk.profile.org"), '<input type="text" name="org" value="' + esc(pr.org) + '">') +
      fld(wsT("js.tk.profile.family"), '<input type="text" name="family" value="' + esc(pr.family) + '">') +
      fld(wsT("js.tk.profile.given"), '<input type="text" name="given" value="' + esc(pr.given) + '">') + '</div>' +
      '<div class="hint" id="pview"></div>';
  }
  function profileView(form) {
    var el = form.elements, pv = form.querySelector('#pview');
    if (!pv || !el.given) return;
    var pr = { org: el.org.value.trim(), family: el.family.value.trim(), given: el.given.value.trim() };
    pv.classList.toggle('warn', !pr.given);
    pv.textContent = pr.given ? (pr.org ? wsT("js.tk.profile.view.org", displayName(pr), pr.org, avatar(pr)) : wsT("js.tk.profile.view", displayName(pr), avatar(pr))) : wsT("js.tk.profile.required");
  }
  // The roles-event dialog's types, each under its readable name. The store settings
  // (priority levels, attachment limit) have their own dialogs (settingsHtml).
  var ROLE_TYPES = ['roles.add_member', 'roles.set_lead', 'roles.add_admin', 'roles.remove_admin', 'roles.add_key', 'roles.revoke_key', 'roles.remove_member'];
  var ROLE_NAME = { 'roles.add_member': "js.tk.rtype.addmember", 'roles.set_lead': "js.tk.rtype.setlead", 'roles.add_admin': "js.tk.rtype.addadmin", 'roles.remove_admin': "js.tk.rtype.removeadmin",
    'roles.add_key': "js.tk.rtype.addkey", 'roles.revoke_key': "js.tk.rtype.revokekey", 'roles.remove_member': "js.tk.rtype.removemember" };
  // a roles-event field, shown only while the type select names one of its types (rtypeShow)
  function rfld(types, label, inner) { return '<div class="fld"' + attr('rt', types) + '><span>' + label + '</span>' + inner + '</div>'; }
  function rtypeShow(form) {
    var t = form.elements.rtype && form.elements.rtype.value;
    if (!t) return;
    Array.prototype.forEach.call(form.querySelectorAll('[data-rt]'), function (el) { el.style.display = el.getAttribute('data-rt').split(' ').indexOf(t) >= 0 ? '' : 'none'; });
  }

  // the dialog kinds a data-act opens (a host adds its own through ext.dialogs); every item
  // action is the context menu's or inline, so only the store-wide forms stay modal
  // one priority-level row; labels count from 1, while the radio value and the field name stay 0-based
  var PRIO_MAX = 9;
  function prioRow(pi, name, checked) {
    return '<div class="fld inline plevel"><input type="radio" name="pdef" value="' + pi + '"' + (checked ? ' checked' : '') + ' aria-label="' + esc(wsT("js.tk.form.pdefault", pi + 1)) + '"' + tipAttr(wsT("js.tk.form.pdefault", pi + 1)) + '>' +
      '<input type="text" name="pname' + pi + '" value="' + esc(name) + '" placeholder="' + esc(wsT("js.tk.form.plevel", pi + 1)) + '" aria-label="' + esc(wsT("js.tk.form.plevel", pi + 1)) + '"></div>';
  }
  var DIALOGS = { profile: 1, admin: 1, sprio: 1, satt: 1, slabel: 1 };
  function dialogHtml(kind, d) {
    if (kind === 'register') return formHtml(kind, d, registerT(d)); // these need no store view
    if (kind === 'relabel') return formHtml(kind, d, relabelT(d));
    if (kind === 'unregister') return formHtml(kind, d, unregT(d));
    var v = memberOf(viewerOf(ui.store));
    var T;
    switch (kind) {
      case 'profile': {
        var pw = d.who || v, pm = VS.members[pw];
        T = [pw === v ? wsT("js.tk.dlg.profile.title") : wsT("js.tk.dlg.profile.title.admin", esc(nm(pw))),
          pw === v ? wsT("js.tk.dlg.profile.intro") : wsT("js.tk.dlg.profile.intro.admin"),
          profileFields(pm.profile) + '<input type="hidden" name="who" value="' + esc(pw) + '">', wsT("js.tk.dlg.save")];
        break;
      }
      case 'admin': {
        var mt = 'roles.set_lead roles.add_admin roles.remove_admin roles.add_key roles.revoke_key roles.remove_member';
        T = [wsT("js.tk.dlg.admin.title"), wsT("js.tk.dlg.admin.intro", esc(viewerOf(ui.store))),
          fld(wsT("js.tk.form.type"), '<select name="rtype">' + ROLE_TYPES.filter(function (t) { return CAPS.addMember || t !== 'roles.add_member'; }).map(function (t) { return '<option value="' + esc(t) + '">' + wsT(ROLE_NAME[t]) + '</option>'; }).join('') + '</select>') +
          rfld(mt, wsT("js.tk.form.member"), '<select name="member">' + memberOpts('') + '</select>') +
          rfld('roles.add_member roles.set_lead', wsT("js.tk.form.lead"), '<select name="lead"><option value="">' + wsT("js.tk.form.lead.none") + '</option>' + memberOpts(v) + '</select>') +
          rfld('roles.add_member roles.add_key roles.revoke_key', wsT("js.tk.form.key"), '<input type="text" name="key" placeholder="' + wsT("js.tk.form.key.ph") + '">') +
          (CAPS.addMember ? rfld('roles.add_member', wsT("js.tk.form.newmember"), '<div class="fld inline"><input type="text" name="nid" placeholder="' + wsT("js.tk.prop.id") + '"><input type="text" name="norg" placeholder="' + wsT("js.tk.field.org") + '"><input type="text" name="nfamily" placeholder="' + wsT("js.tk.field.family") + '"><input type="text" name="ngiven" placeholder="' + wsT("js.tk.field.given") + '"></div>') : ''),
          wsT("js.tk.dlg.admin.submit")];
        break;
      }
      case 'sprio': {
        // one row per level, lowest first, plus blank rows up to the 9-level cap; blank rows
        // are dropped on save, and the radio picks the default
        var pn = VS.priority.names, rows = '';
        for (var pi = 0; pi < Math.min(PRIO_MAX, pn.length + 2); pi++) rows += prioRow(pi, pn[pi] || '', pi === VS.priority.default);
        T = [wsT("js.tk.dlg.prio.title"), wsT("js.tk.dlg.prio.intro"), rows, wsT("js.tk.dlg.save")];
        break;
      }
      case 'slabel': {
        // add (no label) or rename / recolour one; the colour input gives #rrggbb in lowercase
        var el0 = d.label ? labelOf(d.label) : null;
        T = [wsT(el0 ? "js.tk.dlg.label.edit" : "js.tk.dlg.label.add"), wsT("js.tk.dlg.label.intro"),
          (el0 ? '<input type="hidden" name="label" value="' + esc(el0.id) + '">' : '') +
          fld(wsT("js.tk.form.lname"), '<input type="text" name="lname" autocomplete="off" value="' + esc(el0 ? el0.name : '') + '">') +
          fld(wsT("js.tk.form.lcolor"), '<input type="color" name="lcolor" value="' + esc(el0 ? el0.color : '#6b7280') + '">'), wsT("js.tk.dlg.save")];
        break;
      }
      case 'satt':
        T = [wsT("js.tk.dlg.att.title"), wsT("js.tk.dlg.att.intro", esc(fmtSize(VS.attachments.max_bytes))),
          fld(wsT("js.tk.form.maxbytes"), '<input type="number" name="maxbytes" min="1" step="1" value="' + esc(VS.attachments.max_bytes) + '">'), wsT("js.tk.dlg.save")];
        break;
      default: {
        // a host dialog (ext.dialogs)
        var xd = X.dialogs[kind].html(ui.store, VS, d, H);
        T = [xd.title, xd.intro, xd.body || '', xd.submit];
      }
    }
    return formHtml(kind, d, T);
  }
  // T = [title, intro, body, submit label]; no submit label leaves only a Close button.
  function formHtml(kind, d, T) {
    var btns = T[3] ? '<button type="button" class="btn" data-act="dlg-cancel">' + wsT("js.tk.dlg.cancel") + '</button><button type="submit" class="btn primary">' + T[3] + '</button>'
      : '<button type="button" class="btn primary" data-act="dlg-cancel">' + wsT("js.tk.dlg.close") + '</button>';
    return '<form' + attr('kind', kind) + attr('fitem', d.item || '') + attr('fcid', d.cid || '') + (d.store ? attr('fstore', d.store) : '') + '><h2>' + T[0] + '</h2><p>' + T[1] + '</p>' + T[2] +
      '<div class="btns">' + btns + '</div></form>';
  }

  // The register dialog ("+ store"): create a store (no remote, or an empty repository) or
  // join the store a remote holds. d = {profiles: {last, profiles}} as adapter.profiles
  // reads them, or {hostErr} when the host is unavailable (then it only says why). The
  // member profile defaults to the saved one used last; the host saves what is submitted
  // as the one used last (an edited profile becomes a new entry), so the dialog saves
  // nothing itself. No lead field: a self-join cannot set relations.
  var ID_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
  function registerT(d) {
    if (d.hostErr) return [wsT("js.tk.dlg.reg.title"), wsT("js.tk.dlg.reg.unavailable"), '<div class="hint warn">' + esc(d.hostErr) + '</div>', null];
    var ps = (d.profiles && d.profiles.profiles) || [], last = d.profiles && ps[d.profiles.last] ? d.profiles.last : -1;
    var p0 = last >= 0 ? ps[last] : { org: '', family: '', given: '' };
    var saved = '<select name="saved">' + ps.map(function (x, i) {
      var t = profileText(x);
      return '<option value="' + i + '" data-p="' + esc(JSON.stringify({ org: x.org || '', family: x.family || '', given: x.given || '' })) + '"' + (i === last ? ' selected' : '') + '>' + esc(i === last ? wsT("js.tk.dlg.reg.saved.last", t) : t) + '</option>';
    }).join('') + '<option value=""' + (last < 0 ? ' selected' : '') + '>' + (ps.length ? wsT("js.tk.dlg.reg.saved.edited") : wsT("js.tk.dlg.reg.saved.none")) + '</option></select>';
    return [wsT("js.tk.dlg.reg.title"), wsT("js.tk.dlg.reg.intro"),
      '<div class="fld inline">' + fld(wsT("js.tk.dlg.reg.id"), '<input type="text" name="sid" autocomplete="off" spellcheck="false">') +
        fld(wsT("js.tk.dlg.reg.label"), '<input type="text" name="label" autocomplete="off">') + '</div>' +
      fld(wsT("js.tk.dlg.reg.remote"), '<input type="text" name="remote" autocomplete="off" spellcheck="false" placeholder="' + esc(wsT("js.tk.dlg.reg.remote.ph")) + '">') +
      fld(wsT("js.tk.dlg.reg.saved"), saved) + profileFields(p0) +
      fld(wsT("js.tk.dlg.reg.member"), '<input type="text" name="nid" data-auto="1" autocomplete="off" spellcheck="false" value="' + esc(asciiId(p0.given)) + '">') +
      '<div class="hint" id="regmsg" role="status" hidden></div>' + regHiddenHtml(),
      wsT("js.tk.dlg.reg.submit")];
  }
  function openRegister() {
    var he = AD.hostError && AD.hostError();
    if (he) { openDialog('register', { hostErr: he }); return; }
    if (!AD.profiles) { openDialog('register', {}); return; }
    AD.profiles(function (p) { openDialog('register', { profiles: p }); });
  }
  function regMsg(form, text, warn) {
    var m = form.querySelector('#regmsg');
    m.textContent = text; m.hidden = !text; m.classList.toggle('warn', !!warn);
  }
  function submitRegister(form) {
    var el = form.elements, bt = form.querySelector('button[type="submit"]');
    if (!bt || bt.disabled) return;
    function val(n) { return el[n] ? el[n].value.trim() : ''; }
    var req = { id: val('sid'), label: val('label'), remote: val('remote'), member: val('nid'), profile: { org: val('org'), family: val('family'), given: val('given') } };
    var bad = !ID_RE.test(req.id) ? ['sid', wsT("js.tk.dlg.reg.bad.id")] : req.id === 'keys' ? ['sid', wsT("js.tk.dlg.reg.bad.keys")]
      : !req.profile.given ? ['given', wsT("js.tk.dlg.reg.bad.given")] : !ID_RE.test(req.member) ? ['nid', wsT("js.tk.dlg.reg.bad.member")] : null;
    if (bad) { regMsg(form, bad[1], true); el[bad[0]].focus(); return; }
    // a join clones the remote, which can take a while: the button says so until the answer
    var label = bt.textContent;
    bt.disabled = true; bt.textContent = wsT("js.tk.dlg.reg.busy");
    regMsg(form, wsT("js.tk.dlg.reg.wait"), false);
    AD.register(req, function (res) {
      var here = form.isConnected && $('tkdlg').open; // the dialog may have been closed (Escape) meanwhile
      if (!res || !res.ok) {
        if (!here) { toast(esc((res && res.error) || wsT("js.tk.toast.refused"))); return; }
        bt.disabled = false; bt.textContent = label;
        regMsg(form, (res && res.error) || wsT("js.tk.toast.refused"), true);
        return;
      }
      if (here) closeDialog();
      var id = res.store || req.id;
      toast(res.creator ? wsT("js.tk.toast.reg.created", esc(id)) : wsT("js.tk.toast.reg.joined", esc(id), esc(req.member)));
      // the adapter answers after its store list holds the new store
      if (!ui.page && ui.open !== id && (AD.stores() || []).indexOf(id) >= 0) openStore(id);
    });
  }

  // The store registry on this machine (adapter.unregister, relabel, setHidden, hidden, info).
  // The store menu (the "⋯" in a store's title row) relabels, hides, or unregisters the store
  // shown; the "+ store" dialog lists the hidden stores, each with show and unregister. None of
  // these writes to the store itself: relabel and hide change this machine's registry entry, and
  // unregister deletes the local copy (the remote keeps the store). Unregister never syncs
  // first: a sync in flight would make the host refuse it as busy. A store that leaves the list
  // (hidden or removed, here or by another process) is dropped from every state naming it on the
  // next render (dropGone), with no sync, toast, or retry.
  function regInfo(id) { return (AD.info && AD.info(id)) || { id: id, label: '', remote: '', hidden: false }; }
  function regTitle(id) { var s = regInfo(id); return s.label && s.label !== id ? s.label + ' (' + id + ')' : id; }
  function regKnown(id) { return order.indexOf(id) >= 0 || (AD.hidden ? AD.hidden() || [] : []).indexOf(id) >= 0; }
  function resError(res) { return (res && res.error) || wsT("js.tk.toast.refused"); }
  function storeMenuBtn(name) {
    if (!AD.unregister && !AD.relabel && !AD.setHidden) return '';
    return '<button type="button" class="ia"' + attr('act', 'store-menu') + attr('store', name) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.store.menu")) + '"' +
      tipAttr(wsT("js.tk.store.menu")) + '><svg class="i"><use href="#tki-more"/></svg></button>';
  }
  function storeMenuHtml() {
    var h = '<div class="mh"><b>' + esc(regTitle(MN.store)) + '</b></div>';
    if (AD.relabel) h += mi('m-s-label', esc(wsT("js.tk.store.relabel")), {}, { icon: 'pen' });
    if (AD.setHidden) h += mi('m-s-hide', esc(wsT("js.tk.store.hide")), {}, { icon: 'minus' });
    if (AD.unregister) h += msep() + mi('m-s-unreg', esc(wsT("js.tk.store.unreg")), {}, { icon: 'x', cls: 'danger' });
    return h;
  }
  function storeAct(k) {
    var id = MN.store;
    closeMenu(false);
    if (k === 'm-s-hide') hideStore(id);
    else if (k === 'm-s-label') openDialog('relabel', { store: id });
    else if (k === 'm-s-unreg') openDialog('unregister', { store: id });
  }
  // Hiding the store on screen leaves it the way a collapse or leaving its page does (the
  // apply path), then the reload drops its handle.
  function hideStore(id) {
    if (ui.page === id) leavePage();
    else if (ui.open === id) collapse();
    AD.setHidden(id, true, function (res) {
      if (!res || !res.ok) { toast(esc(resError(res))); return; }
      toast(wsT("js.tk.toast.hidden", esc(id)));
    });
  }
  // "show" in the register dialog's hidden list: the handle comes back and the list redraws.
  function showStore(id) {
    AD.setHidden(id, false, function (res) {
      if (!res || !res.ok) { toast(esc(resError(res))); return; }
      var box = $('tkdlg').querySelector('.tkhid');
      if (box) box.outerHTML = regHiddenHtml();
    });
  }
  function regHiddenHtml() {
    var hs = AD.hidden ? AD.hidden() || [] : [];
    if (!hs.length) return '<div class="tkhid" hidden></div>';
    function b(act, label, id) { return '<button type="button" class="btn mini"' + attr('act', act) + attr('store', id) + '>' + label + '</button>'; }
    return '<div class="tkhid"><div class="tkhid-h">' + wsT("js.tk.dlg.reg.hidden") + '</div>' + hs.map(function (id) {
      return '<div class="tkhid-row"><span class="grow">' + esc(regTitle(id)) + '</span>' +
        (AD.setHidden ? b('store-show', wsT("js.tk.dlg.reg.show"), id) : '') + (AD.unregister ? b('store-unreg', wsT("js.tk.dlg.unreg.submit"), id) : '') + '</div>';
    }).join('') + '</div>';
  }
  // d.store, as the dialog's data-fstore; the label field starts at the registry label (empty: the id shows).
  function relabelT(d) {
    var s = regInfo(d.store);
    return [wsT("js.tk.dlg.relabel.title", esc(d.store)), wsT("js.tk.dlg.relabel.intro"),
      fld(wsT("js.tk.dlg.reg.label"), '<input type="text" name="label" maxlength="80" autocomplete="off" value="' + esc(s.label || '') + '">') +
      '<div class="hint" id="regmsg" role="status" hidden></div>', wsT("js.tk.dlg.save")];
  }
  function submitRelabel(form) {
    var id = form.getAttribute('data-fstore'), bt = form.querySelector('button[type="submit"]');
    if (!bt || bt.disabled) return;
    var label = form.elements.label.value.trim();
    if (label === (regInfo(id).label || '')) { closeDialog(); return; }
    bt.disabled = true;
    AD.relabel(id, label, function (res) {
      var here = form.isConnected && $('tkdlg').open;
      if (res && res.ok) { if (here) closeDialog(); return; }
      if (!here) { toast(esc(resError(res))); return; }
      bt.disabled = false;
      regMsg(form, resError(res), true);
    });
  }
  // A store with no remote holds its only copy here: its data is lost for good, so the store id
  // must be typed before the button enables, and goes to the host as the confirmation.
  function unregT(d) {
    var id = d.store, s = regInfo(id), t = esc(regTitle(id));
    var body = s.remote ? '' : '<div class="hint warn">' + wsT("js.tk.dlg.unreg.lost", t) + '</div>' +
      fld(wsT("js.tk.dlg.unreg.confirm", esc(id)), '<input type="text" name="confirm" autocomplete="off" spellcheck="false">');
    return [wsT("js.tk.dlg.unreg.title", t), s.remote ? wsT("js.tk.dlg.unreg.intro", t) : wsT("js.tk.dlg.unreg.intro.local", t),
      body + '<div class="hint" id="regmsg" role="status" hidden></div>', wsT("js.tk.dlg.unreg.submit")];
  }
  function unregReady(form) {
    var bt = form.querySelector('button[type="submit"]'), c = form.elements.confirm;
    if (bt && !form.hasAttribute('data-busy')) bt.disabled = !!c && c.value.trim() !== form.getAttribute('data-fstore');
  }
  // A refusal (busy, unpushed events, a stuck store) shows the host's sentence in the dialog.
  function submitUnregister(form) {
    var id = form.getAttribute('data-fstore'), bt = form.querySelector('button[type="submit"]'), c = form.elements.confirm;
    unregReady(form);
    if (!bt || bt.disabled) return;
    var label = bt.textContent;
    form.setAttribute('data-busy', '1');
    bt.disabled = true; bt.textContent = wsT("js.tk.dlg.unreg.busy");
    regMsg(form, '', false);
    AD.unregister(id, c ? c.value.trim() : '', function (res) {
      var here = form.isConnected && $('tkdlg').open;
      if (res && res.ok) { if (here) closeDialog(); toast(wsT("js.tk.toast.unreg", esc(id))); return; }
      if (!here) { toast(esc(resError(res))); return; }
      form.removeAttribute('data-busy');
      bt.textContent = label;
      unregReady(form);
      regMsg(form, resError(res), true);
    });
  }
  // dropGone runs first in every render, once order is the store list: a store no longer in it
  // stops being the one shown, open, or routed (the #/ route is cleared without a history entry,
  // so no "unknown store" notice follows), its menu and dialogs close, and its page-lifetime state
  // goes, so its wiki and search answers are dropped instead of retried. Nothing syncs: a hidden
  // store was applied by its hide, an unregistered one has nothing left to sync.
  var GONE_BY_STORE = ['wiki', 'psr', 'nrow', 'knew', 'tab', 'panel'], GONE_BY_KEY = ['tabSeen', 'loaded', 'mdet', 'ofold', 'wfold'];
  function dropGone() {
    function gone(n) { return !!n && order.indexOf(n) < 0; }
    var lost = gone(ui.store);
    if (lost) {
      keepItemDraft();
      ui.store = order[0] || null;
      ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.sub = null; ui.iedit = null; ui.cedit = null; ui.replyTo = null; ui.itemChanged = true;
    } else if (!ui.store) ui.store = order[0] || null;
    if (gone(ui.open)) ui.open = null;
    if (gone(ui.page)) {
      ui.page = null;
      if (/^#\/./.test(location.hash)) history.replaceState(null, '', location.pathname + location.search);
    }
    if (MN && (MN.kind === 'store' ? gone(MN.store) : lost)) closeMenu(false, true);
    if (typeof PM !== 'undefined' && PM && lost) pmClose(false);
    var dl = $('tkdlg'), f = dl && dl.open && dl.querySelector('form');
    if (f) {
      var fs = f.getAttribute('data-fstore');
      if (fs ? !regKnown(fs) : lost && f.getAttribute('data-kind') !== 'register') closeDialog();
    }
    GONE_BY_STORE.forEach(function (k) {
      Object.keys(ui[k]).forEach(function (n) {
        if (!gone(n)) return;
        if (k === 'psr' && ui.psr[n]) clearTimeout(ui.psr[n].timer);
        delete ui[k][n];
      });
    });
    GONE_BY_KEY.forEach(function (k) {
      Object.keys(ui[k]).forEach(function (key) { if (gone(key.slice(0, key.indexOf('|')))) delete ui[k][key]; });
    });
  }

  function asciiId(given) { return String(given || '').toLowerCase().replace(/[^a-z0-9]+/g, '') || 'newcomer'; }
  function openDialog(kind, d) {
    var dl = $('tkdlg');
    dl.innerHTML = dialogHtml(kind, d || {});
    var f = dl.querySelector('form');
    profileView(f);
    rtypeShow(f);
    if (kind === 'unregister') unregReady(f);
    if (!dl.open) dl.showModal();
    var first = f.querySelector('input[type="text"], textarea, select');
    if (first) first.focus();
  }
  function closeDialog() { var d = $('tkdlg'); if (d.open) d.close(); }

  // ---------------------------------------------------------------- actions

  var toastTimer = null;
  function toast(html) {
    if (window.WsToast) { window.WsToast.show(html, { html: true }); return; }
    var t = $('tktoast');
    if (!t) document.body.appendChild(t = el('div', { class: 'tk tk-toast', id: 'tktoast', hidden: '' }));
    t.innerHTML = html; t.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.hidden = true; }, 6000);
  }
  // One write: events go to the viewer clone's journal (open batch). Events are not
  // pre-validated, so a form can deliberately produce a flagged event. An event of
  // the batch names an earlier one's id as "$N" (its 1-based position); the adapter
  // resolves it. `then(results)` runs after a written batch; a refused one keeps the
  // dialog, the menu, or the inline input open and toasts why.
  // `prep(results)` runs on success before the re-render (the composer drafts clear only then).
  function commit(events, then, prep, fail) {
    AD.dispatch(ui.store, events.map(function (e) { return { type: e[0], payload: e[1], origin: e[2] || 'human' }; }), function (res) {
      if (!res || !res.ok) { toast(esc((res && res.error) || wsT("js.tk.toast.refused"))); if (fail) fail(); return; }
      if (prep) prep(res.results || []);
      wrote(res.results || []);
      if (then) then(res.results || []);
    });
  }
  // A written batch: the last-write line, the dialog or the item menu closed, a re-render, and
  // one toast line per event (a host write, such as a simulated join, reports through this too).
  function wrote(results) {
    ui.last = results;
    closeDialog();
    // the labels step stays open across toggles: it draws again over the written item; the range
    // picker stays open across a late write (an earlier close's), unless the write is its own back row's
    var keep = !!(MN && MN.kind === 'item' && (MN.step === 'labels' || (MN.step === 'due' && !MN.rsent)));
    var mb = MN && MN.kind === 'item' && !keep ? MN.back : null; // an item menu closes; focus goes back to its row once redrawn
    if (MN && MN.kind === 'item' && !keep) closeMenu(false);
    render();
    if (keep && MN && MN.step === 'labels') labelStepRedraw();
    var me = mb && document.querySelector(mb);
    if (me && !me.matches('a[href], button, input, [tabindex]')) me = me.querySelector('button, a[href], [tabindex]');
    if (me && !(document.activeElement && document.activeElement.hasAttribute('data-keep'))) me.focus({ preventScroll: true });
    toast(results.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ': ' + (n.status === 'applied' ? wsT("js.tk.activity.applied") : '<span class="tf">' + esc(String(n.status).toUpperCase()) + '</span> ' + esc(n.reason));
    }).join('<br>') + '<br><span class="muted">' + wsT("js.tk.toast.batch") + '</span>');
  }

  function submit(form) {
    if (form.getAttribute('data-kind') === 'register') return submitRegister(form);
    if (form.getAttribute('data-kind') === 'relabel') return submitRelabel(form);
    if (form.getAttribute('data-kind') === 'unregister') return submitUnregister(form);
    var kind = form.getAttribute('data-kind'), el = form.elements, v = memberOf(viewerOf(ui.store));
    function val(n) { return el[n] ? el[n].value : ''; }
    var evs = [];
    switch (kind) {
      case 'profile': {
        var who = val('who') || v, pm = VS.members[who].profile, pp = {};
        PROFILE_FIELDS.forEach(function (f) { if (val(f).trim() !== pm[f]) pp[f] = val(f).trim(); });
        if (!Object.keys(pp).length) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        var pe = { profile: pp };
        if (who !== v) pe.member = who;
        if (who === v) saveProf({ org: val('org'), family: val('family'), given: val('given') });
        evs.push(['member.profile', pe]);
        break;
      }
      case 'admin': {
        var t = val('rtype'), rp = {};
        if (t === 'roles.add_member' && !CAPS.addMember) return;
        if (t === 'roles.add_member') rp.member = { id: val('nid'), profile: { org: val('norg'), family: val('nfamily'), given: val('ngiven') }, keys: val('key') ? [val('key')] : [], leads: val('lead') ? [val('lead')] : [] };
        else {
          rp.member = val('member');
          if (t === 'roles.set_lead') rp.leads = val('lead') ? [val('lead')] : [];
          if (t === 'roles.add_key' || t === 'roles.revoke_key') rp.key = val('key');
        }
        evs.push([t, rp]);
        break;
      }
      case 'sprio': {
        var names = [], def = -1, picked = form.querySelector('input[name="pdef"]:checked');
        for (var i = 0; el['pname' + i]; i++) {
          var pname = val('pname' + i).trim();
          if (!pname) continue;
          if (picked && picked.value === String(i)) def = names.length;
          names.push(pname);
        }
        if (!names.length) return toast(wsT("js.tk.toast.prio.none"));
        if (def < 0) return toast(wsT("js.tk.toast.prio.default"));
        if (names.join('\n') === VS.priority.names.join('\n') && def === VS.priority.default) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.set_priority_levels', { names: names, default: def }]);
        break;
      }
      case 'slabel': {
        var lid = val('label'), ln = val('lname').trim(), lc = val('lcolor').toLowerCase(), cl = lid ? labelOf(lid) : null;
        if (!ln) return toast(wsT("js.tk.toast.label.name"));
        if (!lid) { evs.push(['roles.add_label', { name: ln, color: lc }]); break; }
        if (!cl) { closeDialog(); return; }
        var lp = { label: lid };
        if (ln !== cl.name) lp.name = ln;
        if (lc !== cl.color) lp.color = lc;
        if (!('name' in lp) && !('color' in lp)) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.edit_label', lp]);
        break;
      }
      case 'satt': {
        var mb = Number(val('maxbytes'));
        if (mb === VS.attachments.max_bytes) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.set_attachment_limit', { max_bytes: mb }]);
        break;
      }
      default: if (X.dialogs && X.dialogs[kind]) X.dialogs[kind].submit(ui.store, form, H); return;
    }
    commit(evs);
  }

  function quick(k, a) {
    var item = a.getAttribute('data-item'), who = a.getAttribute('data-who'), cid = a.getAttribute('data-cid');
    var box = $('cbox'), origin = CAPS.origin && $('corigin') ? $('corigin').value : 'human';
    // a comment clears the composer once written
    function put(evs, extra) { commit(evs, null, extra); }
    switch (k) {
      case 'confirm': put([['item.confirm', { item: item, assignee: who }]]); break;
      case 'unassign': put([['item.unassign', a.hasAttribute('data-rule') ? { item: item, root: a.getAttribute('data-root'), rule: a.getAttribute('data-rule') } : { item: item, assignee: who }]]); break;
      case 'accept': put([['item.accept', { item: item }]]); break;
      case 'claim': put([['item.claim', { item: item }]]); break;
      case 'unopen': put([['item.unopen', { item: item }]]); break;
      case 'approve': put([['item.approve', { item: item, request: a.getAttribute('data-req') }]]); break;
      case 'unrequest': put([['item.unrequest', { item: item, request: a.getAttribute('data-req') }]]); break;
      case 'uncc': put([['item.uncc', { item: item, member: who }]]); break;
      case 'ihide': put([['item.hide', { item: item }]]); break;
      case 'chide': put([['comment.hide', { comment: cid }]]); break;
      case 'comment': {
        if (!box || !box.value.trim()) return toast(wsT("js.tk.toast.nocomment"));
        var cp = { item: item, body: box.value.trim() };
        if (ui.replyTo) cp.reply_to = ui.replyTo;
        var ms = keptMentions(ui.mdraft, cp.body), crs = refsIn(cp.body);
        if (ms.length) cp.mentions = ms;
        if (crs.length) cp.refs = crs;
        var cas = savedAtts('cbox', cp.body);
        if (cas.length) cp.attachments = cas;
        put([['comment.add', cp, origin]], function () { ui.clearDraft = true; ui.replyTo = null; ui.mdraft = []; ui.adraft = []; delete ui.eatt.cbox; });
        break;
      }
    }
  }

