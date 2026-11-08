// Tracker mockup: the adapter the shared renderer (tracker.js, WsTracker.mount) reads
// in this page, over the in-page engine (engine.js) and the fixtures (fixtures.js),
// plus every piece of mockup-only chrome: the viewer and clone selects, the clock, Go
// offline / online, the Data tab (raw JSON editor and reset), the Support agent
// placeholder tab, the theme gear, the machine-local profile list, and the join
// simulation. The product's adapter serves the same view-state JSON from the daemon;
// none of this file ships.
(function () {
  'use strict';

  var E = window.TrackerEngine;
  var stores = {}, defs = {}, order = [], listeners = [];
  var viewers = {};   // store -> clone stream the page is viewed as ("<member>.<clone>")
  var clock = {};     // store -> mock "now"; each write advances it by a minute
  var seen = {};      // "store|stream" -> { badge: { key: true }, read: { key: true } }
  var dataOpen = false, openLogs = {}; // the Data tab's open folds
  var api = null, cur = null; // cur: the store the renderer last rendered (the header controls follow it)
  var PROF_KEY = 'tracker-mockup-profiles', THEME_KEY = 'tracker-mockup-theme';

  function $(id) { return document.getElementById(id); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }

  function tick(name, minutes) {
    clock[name] = new Date(Date.parse(clock[name]) + minutes * 60000).toISOString().slice(0, 19) + 'Z';
  }
  function resetStore(name) {
    stores[name].reset();
    clock[name] = defs[name].now || new Date().toISOString().slice(0, 19) + 'Z';
    // a clone only a simulated join created is gone now: view as the fixture's default again
    var v = viewers[name];
    if (v && stores[name].clonesOf(String(v).split('.')[0]).indexOf(v) < 0) viewers[name] = defs[name].defaultViewer;
  }
  window.TRACKER_FIXTURES.stores.forEach(function (d) {
    defs[d.name] = d;
    stores[d.name] = E.openStore(d); order.push(d.name); viewers[d.name] = d.defaultViewer; resetStore(d.name);
  });
  function seenOf(name) { var k = name + '|' + viewers[name]; return (seen[k] = seen[k] || { badge: {}, read: {} }); }

  // The Data tab's textareas, when shown, are the store's data: a write first applies
  // them (so an edit lands before the event), as the mockup always did. '' = applied.
  function applyRaw(name) {
    var ta = $('raw-roles');
    if (!ta) return '';
    var raw = { roles: null, remote: {}, clones: null };
    try { raw.roles = JSON.parse(ta.value); } catch (e) { return 'JSON error in roles file: ' + e.message; }
    var tas = document.querySelectorAll('textarea[data-owner]');
    for (var i = 0; i < tas.length; i++) {
      var o = tas[i].getAttribute('data-owner'), a;
      try { a = JSON.parse(tas[i].value); } catch (e2) { return 'JSON error in ' + o + ': ' + e2.message; }
      if (!Array.isArray(a)) return o + ' must be a JSON list';
      raw.remote[o] = a;
    }
    try { raw.clones = JSON.parse($('raw-clones').value); } catch (e3) { return 'JSON error in clone state: ' + e3.message; }
    try { stores[name].setRaw(raw); } catch (e4) { return e4.message; }
    return '';
  }

  // "$N" in an id field names the N-th event of the same batch: the stream's next ids.
  var REF_FIELDS = ['item', 'parent', 'comment', 'reply_to'];
  function resolveRefs(payload, stream, next) {
    var p = JSON.parse(JSON.stringify(payload || {}));
    REF_FIELDS.forEach(function (f) {
      var m = typeof p[f] === 'string' && /^\$(\d+)$/.exec(p[f]);
      if (m) p[f] = stream + '#' + (next + Number(m[1]) - 1);
    });
    return p;
  }
  function write(name, stream, events) {
    var next = Number(String(stores[name].nextEventId(stream)).split('#').pop());
    var res = stores[name].dispatch(stream, events.map(function (e) {
      return { type: e.type, payload: resolveRefs(e.payload, stream, next), origin: e.origin || 'human' };
    }), clock[name]);
    tick(name, 1);
    return res;
  }

  // Machine-local saved profiles (never in the ledger), the last used first.
  function loadProfiles() {
    try {
      var x = JSON.parse(localStorage.getItem(PROF_KEY));
      return Array.isArray(x) ? x.filter(function (e) { return e && typeof e.given === 'string' && e.given; }).map(function (e) { return { org: String(e.org || ''), family: String(e.family || ''), given: e.given }; }) : [];
    } catch (e) { return []; }
  }
  function rememberProfile(pr) {
    if (!String(pr.given || '').trim()) return;
    var one = { org: String(pr.org || '').trim(), family: String(pr.family || '').trim(), given: String(pr.given).trim() };
    var list = loadProfiles().filter(function (x) { return x.org !== one.org || x.family !== one.family || x.given !== one.given; });
    list.unshift(one);
    try { localStorage.setItem(PROF_KEY, JSON.stringify(list.slice(0, 12))); } catch (e) { /* storage off: not remembered */ }
  }

  // Switching viewer = the previous clone collapses its panel (push) and the new one
  // comes on screen (the renderer's viewerChanged runs the immediate check there).
  function switchViewer(name, stream) {
    var prev = viewers[name];
    if (prev && prev !== stream) stores[name].sync(prev, clock[name]);
    viewers[name] = stream;
    api.viewerChanged(name);
  }

  // ------------------------------------------------------------ mockup tabs

  // A placeholder for the support-agent chat child (a future daemon-side local support
  // agent): placement and a static sample only, with no responder and no events. Entry
  // shapes follow the dashboard conversation view (user text, assistant text, a
  // collapsed tool line) plus tracker card references that open their item window.
  function chatRefs(vs) {
    var pick = [];
    function add(id) { var it = vs.items[id]; if (it && !it.hidden && it.kind !== 'topic' && pick.indexOf(id) < 0) pick.push(id); }
    vs.my.today.work.concat(vs.my.today.pending, vs.my.today.confirms).forEach(function (id) { add(vs.assignments[id].item); });
    vs.my.given.forEach(add);
    vs.itemOrder.forEach(add);
    return pick;
  }
  function cardChat(name, vs, h) {
    function msg(who, label, html) {
      return '<div class="msg ' + who + '"><div class="mh"><b>' + label + '</b> ' + esc(h.fmtTs(clock[name]).slice(6)) + '</div><div class="bub">' + html + '</div></div>';
    }
    var refs = chatRefs(vs), a = refs[0], b = refs[1];
    var log = '<div class="sysline">placeholder: support-agent chat child, a static sample (no responder, no events)</div>' +
      msg('user', 'You', 'What should I look at first?') +
      '<details class="toolsline"><summary><span class="tsum">2 tools &middot; tracker view, tracker items</span><svg class="i"><use href="#i-chev"/></svg></summary>' +
      '<div class="brows"><div class="brow"><span class="tool">tracker view</span> ' + esc(h.nm(vs.viewer.member)) + '</div>' +
      '<div class="brow"><span class="tool">tracker items</span> open, by priority then due</div></div></details>' +
      msg('lead', 'Support agent', a ? 'This one first, by priority and then due:' + h.crefHtml(a) : 'Nothing in this store involves you right now.') +
      msg('user', 'You', 'Anything else?') +
      msg('lead', 'Support agent', b ? 'This one is next:' + h.crefHtml(b) : 'Nothing else right now.');
    var comp = '<div class="chat-comp"><textarea rows="1" placeholder="Ask the support agent... (placeholder)" disabled></textarea>' +
      '<button class="csend" disabled><svg class="i"><use href="#i-send"/></svg></button></div>';
    return h.card('Support agent', 'placeholder', '<div class="chat-log">' + log + '</div>' + comp, '', 'card-chat', '', 'chat');
  }

  function fmtList(a) { return '[\n' + a.map(function (e) { return '  ' + JSON.stringify(e); }).join(',\n') + '\n]'; }
  function fmtRoles(r) {
    return '{\n  "admins": ' + JSON.stringify(r.admins || []) + ',\n' + (r.priority ? '  "priority": ' + JSON.stringify(r.priority) + ',\n' : '') + '  "members": [\n' +
      r.members.map(function (m) { return '    ' + JSON.stringify(m); }).join(',\n') + '\n  ]\n}';
  }
  function cardData(name, vs, h) {
    var raw = stores[name].raw();
    var x = '<details class="data" id="data-details"' + (dataOpen ? ' open' : '') + '><summary>data (raw JSON, editable)</summary>' +
      '<p class="r2">Roles file (genesis), the pushed commits of every clone ref (the remote), and each clone\'s local state (what it fetched, its unpushed commits and open batch). Edit, then apply; edits live in memory until reload.</p>' +
      '<div class="toolbar"><button class="btn mini primary" data-act="apply">apply</button><button class="btn mini" data-act="reset">reset store to fixtures</button></div>' +
      '<div class="sub">roles file</div><textarea id="raw-roles" rows="12" spellcheck="false">' + esc(fmtRoles(raw.roles)) + '</textarea>';
    Object.keys(raw.remote).forEach(function (o) {
      var cs = raw.remote[o];
      x += '<details' + h.attr('owner', o) + (openLogs[o] ? ' open' : '') + '><summary>remote ' + esc(E.refOf(o)) + ' (' + cs.length + ' commits)</summary>' +
        '<textarea' + h.attr('owner', o) + ' rows="' + Math.min(18, cs.length + 3) + '" spellcheck="false">' + esc(fmtList(cs)) + '</textarea></details>';
    });
    x += '<details' + h.attr('owner', '@clones') + (openLogs['@clones'] ? ' open' : '') + '><summary>clone state (fetched counts, local commits, online)</summary>' +
      '<textarea id="raw-clones" rows="14" spellcheck="false">' + esc(JSON.stringify(raw.clones, null, 1)) + '</textarea></details>';
    return h.card('Data', 'debug', x + '</details>', '', 'card-data');
  }

  // ---------------------------------------------------------------- join

  // The chosen profile joins the current store. A join is a self-write (member.join);
  // the mockup has the roles.add_member stand-in, which only an admin writes, so an
  // admin's clone writes it and the new member's clone is created and opened.
  var joinDialog = {
    html: function (name, vs, d, h) {
      if (!vs.store.remote) return { title: 'Join ' + esc(vs.store.name), intro: 'This store has no remote, so nobody can join it. Open a team store first.', body: '', submit: 'Close' };
      var saved = loadProfiles(), p0 = saved[0] || { org: '', family: '', given: '' };
      return { title: 'Join ' + esc(vs.store.name),
        intro: 'Joins this store as a new member. The saved profiles are this machine\'s own list (localStorage), never in the ledger: pick one, or edit the fields, which saves a new entry. The profile goes into the join payload.',
        body: h.fld('saved profiles', '<select name="saved">' + saved.map(function (x, i) { return '<option value="' + i + '" data-p="' + esc(JSON.stringify(x)) + '"' + (i === 0 ? ' selected' : '') + '>' + esc(h.profileText(x)) + (i === 0 ? ' (last used)' : '') + '</option>'; }).join('') +
            '<option value="">' + (saved.length ? '(edited)' : '(none saved)') + '</option></select>') + h.profileFields(p0) +
          '<div class="fld inline">' + h.fld('member id (a handle, no ".")', '<input type="text" name="nid" value="' + esc(h.asciiId(p0.given)) + '">') +
          h.fld('lead', '<select name="lead"><option value="">(none: root)</option>' + h.memberOpts('', function (m) { return !m.removed; }) + '</select>') + '</div>',
        submit: 'Join' };
    },
    submit: function (name, form, h) {
      var vs = adapter.view(name), el = form.elements;
      function val(n) { return el[n] ? el[n].value : ''; }
      if (!vs.store.remote) return h.closeDialog();
      var err = applyRaw(name);
      if (err) return h.toast(esc(err));
      var jp = { org: val('org').trim(), family: val('family').trim(), given: val('given').trim() }, jid = val('nid').trim();
      var adm = vs.admins.filter(function (id) { return vs.members[id] && !vs.members[id].removed; })[0];
      var adminStream = adm && stores[name].clonesOf(adm)[0];
      if (!adminStream) return h.toast('No admin clone to write the stand-in join.');
      var jres = write(name, adminStream, [{ type: 'roles.add_member', payload: { member: { id: jid, profile: jp, keys: [jid + '-k1'], leads: val('lead') ? [val('lead')] : [] } } }]);
      h.wrote(jres);
      if (jres[0].status !== 'applied') return; // flagged: the toast says why; the write stays in the admin clone's journal
      rememberProfile(jp);
      var jclone = jid + '.pc';
      stores[name].sync(adminStream, clock[name]);
      stores[name].addClone(jclone, clock[name]);
      switchViewer(name, jclone);
      h.toast('Joined ' + esc(name) + ' as ' + esc(h.profileText(jp)) + ' (stand-in: ' + esc(jres[0].alias) + ' written by ' + esc(adm) + '). Now viewing as ' + esc(jclone) + '.');
    }
  };

  // ------------------------------------------------------------- adapter

  var adapter = {
    stores: function () { return order.slice(); },
    view: function (id) { return stores[id] ? stores[id].view(viewers[id], clock[id]) : null; },
    error: function (id) { return stores[id] ? null : 'No store named ' + id + '.'; },
    dispatch: function (id, events, done) {
      var err = applyRaw(id);
      if (err) return done({ ok: false, error: err, refusal: null });
      done({ ok: true, results: write(id, viewers[id], events) });
    },
    check: function (id, done) { stores[id].fetch(viewers[id], clock[id]); if (done) done(); },
    sync: function (id, done) {
      var r = stores[id].sync(viewers[id], clock[id]);
      done({ noRemote: !!r.noRemote, offline: !!r.offline, closed: true, pushed: r.pushed, received: r.fetched, unpushed: r.unpushed, error: null });
    },
    seen: function (id) { return seenOf(id); },
    markSeen: function (id, marks) {
      var s = seenOf(id);
      (marks.badge || []).forEach(function (k) { s.badge[k] = true; });
      (marks.read || []).forEach(function (k) { s.read[k] = true; });
    },
    profiles: function (done) { var p = loadProfiles(); done({ last: p.length ? 0 : -1, profiles: p }); },
    saveProfile: function (p) { rememberProfile(p); },
    onChange: function (fn) { listeners.push(fn); },
    caps: { origin: true, attach: true, addMember: true },
    ext: {
      tabs: [
        { key: 'chat', icon: '#i-agent', title: 'Support agent', after: 'notify', make: cardChat },
        { key: 'data', icon: '#i-data', title: 'Data', tip: 'Data (debug): the raw roles file, refs, and clone state', after: 'activity', make: cardData }
      ],
      syncTools: function (id, vs, h) {
        var s = vs.sync;
        return vs.store.remote ? h.btn(s.online ? 'go-offline' : 'go-online', s.online ? 'Go offline' : 'Come back online', {}, s.online ? '' : 'primary') : '';
      },
      teamTools: function (id, vs, h) { return h.btn('join', 'Join a store...', {}); },
      // the join action for the store the page is on (a store with no remote has nothing to join)
      qsActions: function (id, vs, q) {
        if (!vs.store.remote || !(!q || 'join'.indexOf(q) >= 0 || q.indexOf('join') >= 0)) return [];
        return [{ key: 'join', icon: '#i-plus', title: 'Join ' + id, hint: 'with a saved or new profile', tag: 'join' }];
      },
      dialogs: { join: joinDialog },
      action: function (k, el, h) {
        var s = viewers[cur];
        if (k === 'go-offline') { stores[cur].setOnline(s, false); h.refresh(); h.toast('This clone is offline: new writes stay in its journal, and it fetches nothing.'); }
        else if (k === 'go-online') {
          stores[cur].setOnline(s, true);
          var r = stores[cur].sync(s, clock[cur]);
          h.refresh();
          if (r.noRemote) h.toast('No remote: the open batch is closed into a local commit; writes stay local, exactly like a long offline.');
          else h.toast('Synced: pushed ' + r.pushed + ', fetched ' + r.fetched + ' commit' + (r.fetched === 1 ? '' : 's') + (r.fetched ? '. Late arrivals are checked against what their authors had seen; no earlier verdict changes.' : '.'));
        }
        else if (k === 'apply') { var err = applyRaw(cur); if (err) h.toast(esc(err)); else { h.refresh(); h.toast('Raw data applied.'); } }
        else if (k === 'reset') { resetStore(cur); h.refresh(true); h.toast('Store reset to fixtures.'); }
        else return false;
        return true;
      },
      rendered: function (id, vs, h) {
        cur = id;
        if (vs) {
          var v = vs.viewer.member, s = vs.viewer.stream;
          $('viewer').innerHTML = Object.keys(vs.members).map(function (k) {
            var m = vs.members[k];
            return '<option value="' + esc(k) + '"' + (k === v ? ' selected' : '') + '>' + esc(h.nmo(k) + ' (' + k + ', ' + (m.removed ? 'removed' : m.role) + ')') + '</option>';
          }).join('');
          $('vclone').innerHTML = (vs.clones[v] || []).map(function (c) {
            return '<option value="' + esc(c.stream) + '"' + (c.stream === s ? ' selected' : '') + '>' + esc(E.cloneOf(c.stream)) + (c.online ? '' : ' (offline)') + (c.unpushed ? ' *' : '') + '</option>';
          }).join('');
          $('vclock').textContent = h.fmtTs(clock[id]);
        }
        var chat = document.querySelector('.tk .chat-log');
        if (chat) chat.scrollTop = chat.scrollHeight; // newest message at the bottom
        measureStub();
      }
    }
  };

  // ------------------------------------------------------------- frame

  // The panels handle is a static stub standing in for the dashboard's own session-panel
  // handle: its width is --drhw, which the renderer's store handles stack left of.
  function measureStub() {
    var b = document.querySelector('.stub-handle');
    document.documentElement.style.setProperty('--drhw', (b ? b.getBoundingClientRect().width : 0) + 'px');
  }

  // Theme: auto / light / dark, local UI state in localStorage. Applied as
  // data-theme="light|dark" on the root, absent for auto, matching the dashboard's gear
  // menu; index.html applies the saved value before first paint.
  function themeOf() { try { var t = localStorage.getItem(THEME_KEY); return t === 'light' || t === 'dark' ? t : 'auto'; } catch (e) { return 'auto'; } }
  function setTheme(t) {
    try { if (t === 'auto') localStorage.removeItem(THEME_KEY); else localStorage.setItem(THEME_KEY, t); } catch (e) { /* storage off: the choice lasts this page */ }
    if (t === 'auto') document.documentElement.removeAttribute('data-theme'); else document.documentElement.setAttribute('data-theme', t);
    Array.prototype.forEach.call(document.querySelectorAll('#gmenu [data-theme-set]'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-theme-set') === t)); });
  }
  function gearMenu(open) { $('gmenu').hidden = !open; $('gear').setAttribute('aria-expanded', String(open)); }

  window.addEventListener('error', function (e) {
    if (api) api.toast('script error: ' + esc(e.message));
    else { var t = $('tktoast'); if (t) { t.hidden = false; t.textContent = 'script error: ' + e.message; } }
  });
  setTheme(themeOf());
  measureStub();
  document.addEventListener('click', function (e) {
    var t = e.target;
    if (t.closest('#gear')) { gearMenu($('gmenu').hidden); return; }
    var th = t.closest('[data-theme-set]');
    if (th) { setTheme(th.getAttribute('data-theme-set')); return; }
    if (!$('gmenu').hidden && !t.closest('#gmenu')) gearMenu(false);
    if (t.closest('.tk')) return; // the renderer's own
    if (t.closest('.stub-handle')) { api.toast('The side panels handle is a stub in this mockup.'); return; }
    var os = t.closest('[data-open-store]');
    if (os) { var n = os.getAttribute('data-open-store'); if (window.__tracker.ui.open !== n) api.openStore(n); return; }
    var ck = t.closest('[data-act="clock"]');
    if (ck) { e.preventDefault(); tick(cur, Number(ck.getAttribute('data-min'))); api.refresh(); return; }
    if (t.closest('a[href="#"]')) e.preventDefault();
  });
  document.addEventListener('change', function (e) {
    var t = e.target;
    if (t.id === 'viewer') switchViewer(cur, stores[cur].clonesOf(t.value)[0]);
    else if (t.id === 'vclone') switchViewer(cur, t.value);
  });
  // the Data tab's folds stay as they were across re-renders
  document.addEventListener('toggle', function (e) {
    var t = e.target;
    if (t.tagName !== 'DETAILS') return;
    if (t.id === 'data-details') dataOpen = t.open;
    else if (t.getAttribute('data-owner')) openLogs[t.getAttribute('data-owner')] = t.open;
  }, true);
  // Esc closes the gear menu first; the renderer leaves an Esc marked handled alone
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && !$('gmenu').hidden && !$('tkdlg').open) { e.preventDefault(); gearMenu(false); }
  }, true);
  window.addEventListener('resize', measureStub);

  api = window.WsTracker.mount(adapter, {
    header: $('top'), qsSlot: $('qsslot'), pageParent: document.querySelector('.dwrap'), storagePrefix: 'tracker-mockup.'
  });
})();
