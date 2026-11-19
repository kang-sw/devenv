  // --- tracker: the store drawers over the daemon's /tracker routes ---------
  // The tracker renderer (tracker.js, window.WsTracker) draws one drawer
  // handle per tracker store, the #/<store> page, the item window, and the
  // quick switcher; this section is its data adapter, on both pages (the
  // adapter contract is tracker.js's header comment). The first store-list
  // read decides whether there is a tracker at all: 404 (the daemon serves
  // none) mounts nothing, 503 (its host is unavailable) mounts with no
  // stores and keeps the reason for hostError, 200 mounts and then reads
  // every store's view. The adapter's reads are synchronous over the cache
  // below; the stream's "tracker" event reloads it, one reload in flight
  // and at most one queued, each read revalidating the answer held (ETag,
  // see tkAnswers). Seen marks are only ever added, so a read unites
  // the daemon's marks with the local copy, which may hold marks still being
  // posted. A store hidden on this machine is left out of tkOrder (the
  // handles) and its view is not read; tkHidden lists it for the "+ store"
  // dialog, and tkInfo holds every listed store's registry entry.
  var tkApi = null, tkOrder = [], tkHidden = [], tkInfo = {}, tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSubs = [];
  var tkSupport = false, tkBusy = false, tkQueued = false, tkNext = [], tkLastRev = null, tkNudgedRev = null;
  function tkUrl(id, op) { return "/tracker/stores/" + encodeURIComponent(id) + "/" + op; }
  // tkFetch calls a tracker route, a POST when body is given; done(status,
  // answer, etag, text) gets the HTTP status (0: the daemon was not
  // reached), the parsed JSON answer ({} when there is none), the answer's
  // ETag ("" when none), and its raw text.
  // signal, when given, aborts the request (done then gets status 0).
  // tag, when given, is the ETag of the answer the page holds: a GET sends
  // it as If-None-Match, and the daemon answers 304 while it still stands.
  function tkFetch(path, body, done, keepalive, signal, tag) {
    var o = { method: body === undefined ? "GET" : "POST", credentials: "same-origin", cache: "no-store" };
    if (body !== undefined) { o.headers = { "Content-Type": "application/json" }; o.body = JSON.stringify(body); }
    else if (tag) o.headers = { "If-None-Match": tag };
    if (keepalive) o.keepalive = true;
    if (signal) o.signal = signal;
    fetch(path, o).then(function (r) {
      return r.text().then(function (s) {
        var v = null;
        try { v = s ? JSON.parse(s) : null; } catch (e) { v = null; }
        done(r.status, v && typeof v === "object" ? v : {}, r.headers.get("ETag") || "", s);
      });
    }, function () { done(0, {}); });
  }
  function tkErrText(status, v) { return v.error ? String(v.error) : status ? "HTTP " + status : wsT("js.post.unreachable"); }
  function tkMarks(s) { return s || { badge: {}, read: {} }; }
  function tkAddMarks(id, badge, read) {
    var s = tkSeen[id] || (tkSeen[id] = { badge: {}, read: {} });
    (badge || []).forEach(function (k) { s.badge[k] = true; });
    (read || []).forEach(function (k) { s.read[k] = true; });
  }
  // tkNotify tells the renderer that the cache changed; more marks a change
  // more of which lands shortly (see tkReadAll), which the renderer may hold
  // back briefly to draw together with the rest.
  function tkNotify(more) { tkSubs.forEach(function (fn) { fn(!!more); }); }
  // The page revalidates what it holds: tkAnswers keeps each store's last
  // view answer ({tag, text}: its ETag and raw text) and tkList the store
  // list's ({tag, v}: its ETag and parsed answer), so a reload of an
  // unchanged store is a 304 with no body. tkDirty records that a reload
  // changed the cache (tkReload tells the renderer only then).
  var tkAnswers = {}, tkList = null, tkDirty = false;
  // tkLent is another session frame's answer for store id, so a frame the
  // shell just opened revalidates it rather than reading the whole view
  // again; null when there is none (or no shell).
  function tkLent(id) {
    try {
      if (window.parent === window) return null;
      var fr = window.parent.frames;
      for (var i = 0; i < fr.length; i++) {
        try {
          var f = fr[i] !== window && fr[i].wsTrackerAnswer, a = typeof f === "function" ? f(id) : null;
          if (a && typeof a.tag === "string" && a.tag && typeof a.text === "string") return { tag: a.tag, text: a.text };
        } catch (e) { /* another origin's frame */ }
      }
    } catch (e) { /* no window */ }
    return null;
  }
  // tkLoadView reads store id's view and seen marks into the cache; done
  // gets whether the cache changed (a 304 for the view held changes nothing).
  // noLend reads without a sibling frame's answer.
  function tkLoadLegacyView(id, done, noLend) {
    var read = tkReads[id] = ++tkReadSeq;
    var own = tkViews[id] && tkAnswers[id] ? tkAnswers[id] : null, held = own || (noLend ? null : tkLent(id));
    function take(v, answer) {
      tkViews[id] = v.view;
      tkErrs[id] = null;
      tkAddMarks(id, v.seen && v.seen.badge, v.seen && v.seen.read);
      if (answer) tkAnswers[id] = answer; else delete tkAnswers[id];
      // Only the installed full view can retire the projection's detail gate.
      // Waiters added while /view was in flight must also see that full view.
      var p = tkProjections[id];
      delete tkProjections[id];
      if (p) Object.keys(p.waiters).forEach(function (iid) { p.waiters[iid].forEach(function (fn) { fn(); }); });
    }
    tkFetch(tkUrl(id, "view"), undefined, function (status, v, tag, text) {
      if (tkReads[id] !== read) { done(false); return; }
      if (status === 304 && own) { done(false); return; }
      if (status === 304 && held) {
        var lent = null;
        try { lent = JSON.parse(held.text); } catch (e) { lent = null; }
        if (lent && lent.view) { take(lent, held); done(true); return; }
        tkLoadLegacyView(id, done, true); // an unreadable lent answer: read the view itself
        return;
      }
      if (status === 200 && v.view) take(v, tag ? { tag: tag, text: text } : null);
      else {
        tkErrs[id] = tkErrText(status, v);
        if (tkProjections[id]) tkProjections[id].detailError = tkErrs[id];
        else tkViews[id] = null;
        delete tkAnswers[id];
      }
      done(true);
    }, false, null, held ? held.tag : "");
  }
  // Projection models live only in memory. Rows own their flattened records;
  // replacing one never copies/reparses the rest of the store. A detail can be
  // ahead of OR behind its row, so equality (not arrival order) is the gate.
  var tkProjections = {}, tkLegacy = {}, tkReads = {}, tkReadSeq = 0;
  function tkProjection(id) {
    return { cursor: '', rows: {}, bundles: {}, held: {}, busy: {}, tried: {}, waiters: {},
      view: { projection: true, items: {}, assignments: {}, requests: {}, rules: {},
        comments: {}, events: [], notices: [], itemTime: {}, itemFlags: {}, html: { items: {}, comments: {} }, feed: [] } };
  }
  function tkDetailReady(id, iid) {
    var p = tkProjections[id], row = p && p.rows[iid], b = p && p.bundles[iid];
    return !p || !!(row && b && row.detailVersion === b.detailVersion);
  }
  function tkDetailOverlay(p, iid) {
    var row = p.rows[iid], b = p.bundles[iid];
    if (!row) return;
    var it = Object.assign({}, row.data.item);
    if (b && b.detailVersion === row.detailVersion) {
      it.body = b.data.body; it.bodyRev = b.data.bodyRev || []; it.attachments = b.data.attachments || [];
    }
    p.view.items[iid] = it;
  }
  function tkDetailIndexes(p) {
    var v = p.view;
    v.comments = {}; v.events = []; v.html = { items: {}, comments: {} };
    var events = {};
    Object.keys(p.bundles).forEach(function (iid) {
      var b = p.bundles[iid], row = p.rows[iid];
      if (!row || row.detailVersion !== b.detailVersion) return;
      var d = b.data;
      Object.assign(v.comments, d.comments || {});
      (d.events || []).forEach(function (e) { if (!events[e.id]) { events[e.id] = true; v.events.push(e); } });
      Object.assign(v.html.items, d.html && d.html.items || {});
      Object.assign(v.html.comments, d.html && d.html.comments || {});
    });
    v.notices = [];
    Object.keys(p.rows).forEach(function (iid) { v.notices.push.apply(v.notices, p.rows[iid].data.notices || []); });
  }
  function tkDetailWaiters(id, p) {
    Object.keys(p.waiters).forEach(function (iid) {
      if (!tkDetailReady(id, iid)) return;
      var fns = p.waiters[iid]; delete p.waiters[iid];
      fns.forEach(function (fn) { fn(); });
    });
  }
  function tkTakeProjection(id, answer) {
    var p = tkProjections[id];
    if (!p || answer.reset) {
      var old = p;
      p = tkProjections[id] = tkProjection(id);
      // Waiters belong to the user's open editor, not the daemon epoch.
      if (old) { p.waiters = old.waiters; p.held = old.held; }
    }
    var v = p.view;
    function drop(iid) {
      var old = p.rows[iid];
      if (old) ['assignments', 'requests', 'rules'].forEach(function (k) {
        Object.keys(old.data[k] || {}).forEach(function (key) { delete v[k][key]; });
      });
      delete p.rows[iid]; delete p.bundles[iid]; delete p.held[iid]; delete p.tried[iid];
      delete v.items[iid]; delete v.itemTime[iid]; delete v.itemFlags[iid];
    }
    (answer.removed || []).forEach(drop);
    Object.keys(answer.items || {}).forEach(function (iid) {
      var row = answer.items[iid], b = p.bundles[iid], held = p.held[iid];
      drop(iid); p.rows[iid] = row;
      ['assignments', 'requests', 'rules'].forEach(function (k) { Object.assign(v[k], row.data[k] || {}); });
      v.itemTime[iid] = row.data.time; v.itemFlags[iid] = row.data.flagged || 0;
      if (held && held.detailVersion === row.detailVersion) p.bundles[iid] = held;
      else if (b && b.detailVersion === row.detailVersion) p.bundles[iid] = b;
      tkDetailOverlay(p, iid);
      tkAddMarks(id, row.seen && row.seen.badge, row.seen && row.seen.read);
    });
    Object.keys(answer.globals || {}).forEach(function (k) {
      if (answer.globals[k] === null) delete v[k]; else v[k] = answer.globals[k];
    });
    if (Object.prototype.hasOwnProperty.call(answer, 'feed')) v.feed = answer.feed || [];
    p.cursor = answer.cursor; v.projectionCursor = answer.cursor;
    delete p.history; delete v.historyEvents;
    tkDetailIndexes(p);
    tkViews[id] = v; tkErrs[id] = null; delete tkAnswers[id];
    tkDetailWaiters(id, p);
  }
  function tkUseLegacy(id, done) {
    tkLegacy[id] = true; delete tkAnswers[id];
    tkLoadLegacyView(id, function (changed) {
      tkNotify(); if (done) done(changed);
    });
  }
  function tkLoadView(id, done) {
    if (tkLegacy[id]) { tkLoadLegacyView(id, done); return; }
    var p = tkProjections[id], cursor = p && p.cursor, read = tkReads[id] = ++tkReadSeq;
    tkFetch(tkUrl(id, cursor ? 'delta?since=' + encodeURIComponent(cursor) : 'summary'), undefined, function (status, v) {
      if (tkReads[id] !== read) { done(false); return; }
      if (tkLegacy[id] || (p ? tkProjections[id] !== p || p.cursor !== cursor : !!tkProjections[id])) {
        // Another check/dispatch/read already advanced this keyed model. An
        // older request may finish last; never regress its opaque cursor.
        done(false); tkReload(); return;
      }
      if (status === 404 || status === 405 || status === 501) { tkUseLegacy(id, done); return; }
      if (status === 304 && p) { done(false); return; }
      if (status === 200 && v.cursor && v.items && v.globals) {
        tkTakeProjection(id, v); done(true); return;
      }
      // Keep a good model on transient failure, just as a busy store keeps
      // its last published server snapshot. A first failure shows an error.
      tkErrs[id] = tkErrText(status, v);
      done(!tkViews[id]);
    });
  }
  // Visible/expanded ids arrive together on the next tick, at most 100 per
  // request. One request per row version; an unmatched future bundle waits
  // for the delta (a nudge also covers a detail overtaking the stream).
  var tkDetailQueue = {}, tkDetailTimer = 0;
  function tkNeedDetails(id, ids, done) {
    var p = tkProjections[id];
    if (!p) { if (done) done(); return true; }
    var ready = true, called = false;
    function settled() {
      if (!called && (ids || []).every(function (iid) { return tkDetailReady(id, iid); })) { called = true; done(); }
    }
    (ids || []).forEach(function (iid) {
      if (!p.rows[iid] || tkDetailReady(id, iid)) return;
      ready = false;
      if (done) (p.waiters[iid] || (p.waiters[iid] = [])).push(settled);
      if (!tkLegacy[id] && !p.busy[iid] && p.tried[iid] !== p.rows[iid].detailVersion) {
        (tkDetailQueue[id] || (tkDetailQueue[id] = {}))[iid] = true;
      }
    });
    if (ready && done) settled();
    if (!tkDetailTimer && Object.keys(tkDetailQueue).length) tkDetailTimer = setTimeout(tkFlushDetails, 0);
    return ready;
  }
  function tkFlushDetails() {
    tkDetailTimer = 0;
    var queue = tkDetailQueue; tkDetailQueue = {};
    Object.keys(queue).forEach(function (id) {
      var p = tkProjections[id]; if (!p || tkLegacy[id]) return;
      var ids = Object.keys(queue[id]).filter(function (iid) { return p.rows[iid] && !p.busy[iid] && !tkDetailReady(id, iid); });
      while (ids.length) {
        (function (batch) {
          batch.forEach(function (iid) { p.busy[iid] = true; p.tried[iid] = p.rows[iid].detailVersion; });
          tkFetch(tkUrl(id, 'detail?' + batch.map(function (iid) { return 'id=' + encodeURIComponent(iid); }).join('&')), undefined, function (status, answer) {
            if (tkProjections[id] !== p || tkLegacy[id]) return; // retired or downgrading while in flight
            batch.forEach(function (iid) { delete p.busy[iid]; });
            if (status === 404 || status === 405 || status === 501) { tkUseLegacy(id); return; }
            var mismatch = false;
            if (status === 200) {
              p.detailError = '';
              Object.keys(answer.items || {}).forEach(function (iid) {
                var b = answer.items[iid], row = p.rows[iid]; if (!row || !b.data) return;
                if (b.detailVersion === row.detailVersion) { p.bundles[iid] = b; tkDetailOverlay(p, iid); }
                else { p.held[iid] = b; mismatch = true; }
              });
              if ((answer.missing || []).length) mismatch = true;
              tkDetailIndexes(p); tkDetailWaiters(id, p);
            } else p.detailError = tkErrText(status, answer);
            tkNotify();
            if (mismatch) tkReload();
          });
        })(ids.splice(0, 100));
      }
    });
  }
  // Activity and sync inspect global ledger events, which the per-item detail
  // contract deliberately omits. Keep the legacy read only for those explicit
  // history surfaces; its events NEVER satisfy an item's detail-version gate.
  function tkHistory(id) {
    var p = tkProjections[id];
    if (!p || p.history === p.cursor) return true;
    if (p.historyBusy || p.historyAttempt === p.cursor) return false;
    p.historyBusy = true; var cursor = p.cursor; p.historyAttempt = cursor; p.historyError = '';
    tkFetch(tkUrl(id, 'view'), undefined, function (status, answer) {
      if (tkProjections[id] !== p) return;
      p.historyBusy = false;
      if (cursor !== p.cursor) { tkNotify(); return; }
      if (status === 200 && answer.view) { p.view.historyEvents = answer.view.events || []; p.history = cursor; }
      else p.historyError = tkErrText(status, answer);
      tkNotify();
    });
    return false;
  }
  // tkReadAll reads the store list, calls listed(status), and with a list
  // reads every store's view in parallel before done(). The renderer, once
  // mounted, is told as each view but the last lands (as a change with more
  // to come), so a slow store holds back only its own rows (it keeps its
  // previous view, or the loading state, meanwhile); the last view is left to
  // done's caller to announce.
  // A 304 for the list held reads as that list's 200.
  function tkReadAll(listed, done) {
    var held = tkList;
    function sig(v) { return JSON.stringify([v.stores, !!v.support]); }
    tkFetch("/tracker/stores", undefined, function (status, v, tag) {
      if (status === 304 && held) { status = 200; v = held.v; }
      else {
        // The list's ETag moves with every revision (the answer carries
        // it); only another list, or none, changes what the renderer shows.
        if (status !== 200 || !held || sig(v) !== sig(held.v)) tkDirty = true;
        tkList = status === 200 && tag ? { tag: tag, v: v } : null;
      }
      if (status === 503) { tkHostErr = tkErrText(status, v); tkOrder = []; tkHidden = []; tkInfo = {}; }
      if (status === 200) {
        tkHostErr = null;
        tkSupport = !!v.support;
        if (typeof v.rev === "number") tkLastRev = v.rev;
        tkOrder = []; tkHidden = []; tkInfo = {};
        (Array.isArray(v.stores) ? v.stores : []).forEach(function (s) {
          var id = String(s.id);
          tkInfo[id] = { id: id, label: String(s.label || ""), remote: String(s.remote || ""), hidden: !!s.hidden };
          (s.hidden ? tkHidden : tkOrder).push(id);
        });
        var keep = {};
        tkOrder.forEach(function (id) { keep[id] = true; });
        [tkViews, tkErrs, tkSeen, tkAnswers, tkProjections, tkLegacy, tkReads].forEach(function (m) { Object.keys(m).forEach(function (id) { if (!keep[id]) delete m[id]; }); });
      }
      listed(status);
      var left = status === 200 ? tkOrder.length : 0;
      if (!left) { done(); return; }
      tkOrder.forEach(function (id) {
        tkLoadView(id, function (changed) {
          if (changed) tkDirty = true;
          if (--left === 0) done();
          else if (tkApi && changed) tkNotify(true);
        });
      });
    }, false, null, held ? held.tag : "");
  }
  // tkReload re-reads everything, then tells the renderer and runs then;
  // a call while a reload runs queues one more, which runs the thens given
  // meanwhile. Until a first list read mounts the renderer, a reload is
  // that first read.
  function tkReload(then) {
    if (then) tkNext.push(then);
    if (tkBusy) { tkQueued = true; return; }
    tkBusy = true;
    tkQueued = false;
    var thens = tkNext, first = !tkApi;
    tkNext = [];
    tkDirty = false;
    tkReadAll(function (status) {
      if (!tkApi) tkMount(); // the palette still works when no tracker route is available
    }, function () {
      tkBusy = false;
      if (tkNudgedRev !== null) {
        if (tkNudgedRev !== tkLastRev) tkQueued = true;
        tkNudgedRev = null;
      }
      // A reload that read nothing new (every answer a 304) redraws nothing.
      if (tkApi && (tkDirty || first)) tkNotify();
      thens.forEach(function (fn) { fn(); });
      if (tkQueued) tkReload();
    });
  }
  // tkRegistryWrite posts one of a store's registry writes (label, hidden,
  // unregister) and answers done after the reload that shows it; a refusal
  // passes on its message and kind.
  function tkRegistryWrite(id, op, body, done) {
    tkFetch(tkUrl(id, op), body, function (status, v) {
      if (status === 200 || status === 204) tkReload(function () { done({ ok: true }); });
      else done({ ok: false, error: tkErrText(status, v), kind: String(v.kind || "") });
    });
  }
  var tkAdapter = {
    stores: function () { return tkOrder.slice(); },
    view: function (id) { return tkViews[id] || null; },
    detail: tkNeedDetails,
    detailReady: tkDetailReady,
    detailError: function (id) { var p = tkProjections[id]; return p && p.detailError || ''; },
    retryDetail: function (id, iid) {
      if (tkLegacy[id]) { tkUseLegacy(id); return; }
      var p = tkProjections[id]; if (p) delete p.tried[iid]; tkNeedDetails(id, [iid]);
    },
    history: tkHistory,
    retryHistory: function (id) { var p = tkProjections[id]; if (p) delete p.historyAttempt; tkHistory(id); },
    historyError: function (id) { var p = tkProjections[id]; return p && p.historyError || ''; },
    error: function (id) { return tkViews[id] ? null : tkErrs[id] || null; },
    hostError: function () { return tkHostErr; },
    hidden: function () { return tkHidden.slice(); },
    info: function (id) { return tkInfo[id] || null; },
    relabel: function (id, label, done) { tkRegistryWrite(id, "label", { label: label }, done); },
    setHidden: function (id, hidden, done) { tkRegistryWrite(id, "hidden", { hidden: !!hidden }, done); },
    unregister: function (id, confirm, done) { tkRegistryWrite(id, "unregister", { confirm: confirm || "" }, done); },
    seen: function (id) { return tkMarks(tkSeen[id]); },
    markSeen: function (id, add) {
      var badge = (add && add.badge) || [], read = (add && add.read) || [];
      tkAddMarks(id, badge, read);
      if (badge.length || read.length) tkFetch(tkUrl(id, "seen"), { badge: badge, read: read }, function () {});
    },
    // A written batch's results come from the store's view read again: each
    // event's alias and its applied or flagged status as the store derived it.
    dispatch: function (id, events, done) {
      tkFetch(tkUrl(id, "dispatch"), { events: events }, function (status, v) {
        if (status === 409) { done({ ok: false, error: tkErrText(status, v), refusal: v.refusal || null }); return; }
        if (status !== 200) { done({ ok: false, error: tkErrText(status, v), refusal: null }); return; }
        var ids = Array.isArray(v.ids) ? v.ids : [];
        tkLoadView(id, function () {
          function finish() {
            var byId = {};
            ((tkViews[id] && tkViews[id].events) || []).forEach(function (e) { byId[e.id] = e; });
            // Global/role writes have no item bundle; read their verdicts from
            // the explicit history view, never invent an applied verdict.
            if (ids.some(function (eid) { return !byId[eid]; })) {
              tkFetch(tkUrl(id, 'view'), undefined, function (code, full) {
                if (code === 200 && full.view) (full.view.events || []).forEach(function (e) { byId[e.id] = e; });
                report(byId);
              });
            } else report(byId);
          }
          function report(byId) {
            done({ ok: true, results: ids.map(function (eid, i) {
              var e = byId[eid];
              return e ? { id: e.id, alias: e.alias, type: e.type, status: e.status, reason: e.reason || '' } :
                { id: eid, alias: String(eid).replace('#', '-'), type: events[i] ? events[i].type : '', status: 'unknown', reason: 'Verdict unavailable' };
            }) });
          }
          var p = tkProjections[id], wanted = [];
          if (p) Object.keys(p.rows).some(function (iid) {
            if ((p.rows[iid].data.item.thread || []).some(function (eid) { return ids.indexOf(eid) >= 0; })) wanted.push(iid);
            return wanted.length === 100;
          });
          if (!wanted.length) finish();
          else tkFetch(tkUrl(id, 'detail?' + wanted.map(function (iid) { return 'id=' + encodeURIComponent(iid); }).join('&')), undefined, function (code, answer) {
            var byId = {};
            if (code === 200) Object.keys(answer.items || {}).forEach(function (iid) {
              (answer.items[iid].data.events || []).forEach(function (e) { byId[e.id] = e; });
            });
            // Verdict reporting is not rendering: read returned event ids even
            // when the bundle overtook the summary. Never install it as detail.
            if (ids.every(function (eid) { return !!byId[eid]; })) report(byId);
            else finish();
          });
        });
      });
    },
    check: function (id, done) {
      tkFetch(tkUrl(id, "check"), {}, function () {
        tkLoadView(id, function () { tkNotify(); if (done) done(); });
      });
    },
    // The route's pushed is a yes/no; the renderer counts commits, so a push
    // counts what the view listed as unpushed (the open batch as one).
    // keepalive: a collapse can be the last thing the page does.
    sync: function (id, done) {
      var vw = tkViews[id], n = vw && vw.sync && Array.isArray(vw.sync.unpushed) ? vw.sync.unpushed.length : 0;
      tkFetch(tkUrl(id, "sync"), {}, function (status, v) {
        if (status !== 200) { done({ error: tkErrText(status, v) }); return; }
        var rep = {};
        Object.keys(v).forEach(function (k) { rep[k] = v[k]; });
        rep.pushed = v.pushed ? n : 0;
        done(rep);
      }, true);
    },
    profiles: function (done) {
      tkFetch("/tracker/profiles", undefined, function (status, v) {
        if (status === 200 && Array.isArray(v.profiles)) done({ last: typeof v.last === "number" ? v.last : -1, profiles: v.profiles });
        else done({ last: -1, profiles: [] });
      });
    },
    saveProfile: function (p) { tkFetch("/tracker/profiles", p, function () {}); },
    // An editor's preview: the daemon renders the text as the view renders a saved one,
    // with the editor's pending attachments as the text's own.
    render: function (id, req, done) {
      var body = { text: req.text, attachments: req.attachments || [] };
      if (req.wiki) body.wiki = req.wiki;
      tkFetch(tkUrl(id, "render"), body, function (status, v) {
        if (status === 200 && typeof v.html === "string") done({ ok: true, html: v.html });
        else done({ ok: false, error: tkErrText(status, v) });
      });
    },
    // The Items search box: the daemon parses the query by the tracker's search grammar
    // (tracker_routes.go: the search route); a malformed query answers 400 with its message.
    // The search palette asks for excerpts (o.excerpts) and aborts through o.signal.
    search: function (id, q, done, o) {
      o = o || {};
      tkFetch(tkUrl(id, "search?q=" + encodeURIComponent(q) + (o.excerpts ? "&excerpts=1" : "")), undefined, function (status, v) {
        if (status === 200 && Array.isArray(v.ids)) done({ ok: true, ids: v.ids, hits: Array.isArray(v.hits) ? v.hits : [] });
        else done({ ok: false, error: tkErrText(status, v), status: status });
      }, false, o.signal);
    },
    // The store's wiki branches (tracker_routes.go: the wiki routes). A save that
    // cannot merge answers 409 with both texts, passed on as res.conflict.
    wiki: {
      list: function (id, done) {
        tkFetch(tkUrl(id, "wiki"), undefined, function (status, v) {
          if (status === 200 && Array.isArray(v.wikis)) done({ ok: true, wikis: v.wikis });
          else done({ ok: false, error: tkErrText(status, v), status: status });
        });
      },
      pages: function (id, wiki, done) {
        tkFetch(tkUrl(id, "wiki/" + encodeURIComponent(wiki) + "/pages"), undefined, function (status, v) {
          if (status === 200 && Array.isArray(v.pages)) done({ ok: true, pages: v.pages, commit: String(v.commit || "") });
          else done({ ok: false, error: tkErrText(status, v), status: status });
        });
      },
      read: function (id, wiki, path, done) {
        tkFetch(tkUrl(id, "wiki/" + encodeURIComponent(wiki) + "/page?path=" + encodeURIComponent(path)), undefined, function (status, v) {
          if (status === 200 && typeof v.content === "string") done({ ok: true, path: String(v.path || path), content: v.content, commit: String(v.commit || ""), html: String(v.html || "") });
          else done({ ok: false, error: tkErrText(status, v), status: status });
        });
      },
      save: function (id, wiki, req, done) {
        tkFetch(tkUrl(id, "wiki/" + encodeURIComponent(wiki) + "/save"), req, function (status, v) {
          if (status === 200 && typeof v.commit === "string") done({ ok: true, commit: v.commit, merged: !!v.merged, created: !!v.created, warnings: v.warnings || [] });
          else done({ ok: false, error: tkErrText(status, v), status: status, conflict: status === 409 && v.conflict && typeof v.conflict === "object" ? v.conflict : null });
        });
      }
    },
    // An image pasted or dropped into an editor: the raw bytes, which the daemon
    // stages until a write names them; a paste sends no name (the daemon's default).
    upload: function (id, file, name, done) {
      var o = { method: "POST", credentials: "same-origin", cache: "no-store", body: file };
      fetch(tkUrl(id, "attachments") + (name ? "?name=" + encodeURIComponent(name) : ""), o).then(function (r) {
        return r.text().then(function (s) {
          var v = null;
          try { v = s ? JSON.parse(s) : null; } catch (e) { v = null; }
          v = v && typeof v === "object" ? v : {};
          if (r.status === 200 && typeof v.oid === "string") done({ ok: true, attachment: { oid: v.oid, name: v.name, size: v.size, mime: v.mime } });
          else done({ ok: false, error: tkErrText(r.status, v) });
        });
      }, function () { done({ ok: false, error: tkErrText(0, {}) }); });
    },
    // done comes after the reload, so the store list already holds the new store.
    register: function (req, done) {
      tkFetch("/tracker/register", req, function (status, v) {
        if (status === 200) tkReload(function () { done({ ok: true, store: v.store, creator: !!v.creator }); });
        else done({ ok: false, error: tkErrText(status, v) });
      });
    },
    onChange: function (fn) { tkSubs.push(fn); },
    caps: { origin: false, attach: false, addMember: false }
  };
  // The drawer hangs from the header and its handle row sits left of the
  // session drawer's handle; the store page takes the page's place. On a
  // session page it goes into the body beside the rail, where it takes the
  // hidden grid's column; appended to the wrap instead, it would stack below
  // the rail's column on a wide screen. Store drawers use the host's maximum
  // height, including its composer clearance, without storing a height.
  // --- search palette: the rendered rail and this session's board only -----
  function paletteText(el) {
    if (!el) return "";
    var copy = el.cloneNode(true);
    q("script, style, template, noscript", copy).forEach(function (n) { n.remove(); });
    return copy.textContent.replace(/\s+/g, " ").trim();
  }
  function paletteBodyText(body) {
    var frame = body && body.querySelector(".doc-frame");
    if (!frame) return paletteText(body);
    // Both mounted frames and inert placeholders retain the source document.
    // A template reads its text without executing scripts or fetching images.
    var parsed = doc.createElement("template");
    parsed.innerHTML = frame.getAttribute("data-srcdoc") || frame.getAttribute("srcdoc") || "";
    return paletteText(parsed.content.querySelector(".doc-body"));
  }
  function paletteEntryText(entry) {
    function own(selector) {
      return q(selector, entry).filter(function (n) { return n.closest(".wb-entry") === entry; })[0];
    }
    var recap = own(".entry-recap"), body = own(".entry-body");
    if (recap || body) return (paletteText(recap) + " " + paletteBodyText(body)).trim();
    var line = own(".entry-text");
    if (!line) return "";
    var copy = line.cloneNode(true);
    q("time", copy).forEach(function (n) { n.remove(); });
    return paletteText(copy).replace(/^—\s*/, "");
  }
  function paletteRows(update) {
    // The board caps its initially rendered closed topics. Reuse its existing
    // show-more path so older hits are searchable and already mounted to jump to.
    if (update) { paletteClosed(update); paletteHistory(update); }
    var rows = [];
    q("#rail .rs[data-rid]", framed ? shParentDoc() : doc).forEach(function (el) {
      var label = el.querySelector(".rid"), link = el.querySelector("a.rl");
      if (label && link) rows.push({ group: 0, kind: "session", title: label.textContent.trim(), href: link.getAttribute("href") });
    });
    boardQ("section.topic[data-topic]").forEach(function (topic) {
      var name = topic.getAttribute("data-topic"), label = paletteText(topic.querySelector(".tname")) || name;
      var time = topic.querySelector(".tbar time"), at = time ? Number(time.getAttribute("data-at")) || Date.parse(time.getAttribute("datetime")) || 0 : 0;
      rows.push({ group: 1, kind: "board-topic", topic: name, title: label, at: at });
      // Only the topic-owned fold can supply a body, never a stream panel.
      var body = topic.querySelector(":scope > .tdoc > .doc-body"), text = paletteBodyText(body);
      var recap = paletteText(topic.querySelector(".rtext"));
      text = (recap + " " + text).trim();
      if (body && text) rows.push({ group: 2, kind: "board-body", topic: name, title: text, hint: label });
      function entries(root) {
        q("article.wb-entry", root).forEach(function (entry) {
          var clock = q("time.entry-at", entry).filter(function (n) { return n.closest(".wb-entry") === entry; })[0];
          rows.push({ group: 5, kind: "board-entry", topic: name, entry: entry.id,
            title: paletteEntryText(entry), hint: label + (clock ? " · " + paletteText(clock) : "") });
        });
      }
      q(".topic-stream", topic).forEach(function (stream) {
        entries(stream);
        q("template[data-history-page], template[data-history-gap]", stream).forEach(function (page) { entries(page.content); });
      });
      q('[id^="item-"]', topic).forEach(function (item) {
        var id = /^item-(\d+)$/.exec(item.id);
        if (!id) return;
        var title = paletteText(item.querySelector(".title, .stext > b"));
        rows.push({ group: 3, kind: "board-item", topic: name, id: id[1], title: title, hint: label, at: at });
        var answer = q(".pick, .ans, .quote, .stamp, .said", item).map(paletteText).join(" ");
        if (item.hasAttribute("data-id")) {
          var draft = readItem(item);
          if (draft) answer += " " + draft.answer + " " + draft.note;
        }
        if (answer.trim()) rows.push({ group: 4, kind: "board-answer", topic: name, id: id[1], title: answer.trim(), hint: label + " · " + title });
      });
    });
    return rows;
  }
  // Capsule candidates share the selected board's labels and closed-page route.
  // Parse older pages inertly rather than opening folds or changing reader history.
  var capsuleTopicLoad = null, capsuleTopicKey = "";
  function capsuleTopics(done) {
    if (!rid || shell) { done([]); return; }
    var board = region("board"), more = boardQ("button.closed-more[data-closed-next]")[0];
    var key = (board && board.getAttribute("data-h") || "") + ":" + (more ? more.getAttribute("data-closed-next") : "end");
    if (!capsuleTopicLoad || key !== capsuleTopicKey) {
      capsuleTopicKey = key;
      var rows = [], seen = {};
      function add(topics) {
        topics.forEach(function (topic) {
          var id = topic.getAttribute("data-topic");
          if (!id || seen[id]) return;
          seen[id] = true; rows.push({ id: id, label: paletteText(topic.querySelector(".tname")) || id });
        });
      }
      add(boardQ("section.topic[data-topic]"));
      function page(next) {
        if (next == null || !window.fetch) return Promise.resolve(rows);
        return fetch("/s/" + rid + "/closed?from=" + encodeURIComponent(next), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
          if (!r.ok) throw new Error("HTTP " + r.status);
          return r.json();
        }).then(function (res) {
          var parsed = frag(res.html || "");
          add(q("section.topic[data-topic]", parsed));
          var button = parsed.querySelector("button.closed-more[data-closed-next]"), after = button && button.getAttribute("data-closed-next");
          return after != null && Number(after) > Number(next) ? page(after) : rows;
        });
      }
      capsuleTopicLoad = page(more ? more.getAttribute("data-closed-next") : null).catch(function () {
        capsuleTopicLoad = null;
        // A failed closed-page load is not a complete topic inventory.
        return [];
      });
    }
    capsuleTopicLoad.then(done);
  }
  // Lazy board history leaves older entries out of the page; fetch them so
  // the entry search stays complete.
  function paletteHistory(update) {
    var w = window.wbHistory;
    if (!w || !w.hydrateAll) return;
    w.hydrateAll(doc).then(function (n) { if (n) update(paletteRows()); });
  }
  function paletteClosed(update) {
    var more = boardQ("button.closed-more[data-closed-next]")[0];
    if (!more || more.disabled) return;
    loadClosed(more).then(function (loaded) {
      if (!loaded) return;
      update(paletteRows());
      paletteHistory(update);
      paletteClosed(update);
    });
  }
  // o.keep (the palette's Ctrl+Enter) scrolls and flashes the target behind
  // the open palette, which keeps the focus.
  function palettePick(row, o) {
    if (row.kind === "session") { go(row.href); return; }
    if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(function () { palettePick(row, o); })) return;
    // Look the target up again: an SSE board swap may have replaced its node
    // while the palette was open. Open every enclosing fold before scrolling.
    var topic = boardQ("section.topic[data-topic]").filter(function (n) { return n.getAttribute("data-topic") === row.topic; })[0];
    var target = row.id ? $("item-" + row.id) : topic;
    if (!target || !topic) return;
    function prepare(owner) {
      if (window.WsCards) window.WsCards.unfoldTopic(owner);
      if (doc.body.classList.contains("tk-paged") && tkApi) tkApi.leavePage();
      if (live && !tabShown("board")) setTab("board");
    }
    var waited = false;
    function entryPick() {
      if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(entryPick)) return;
      // Overview releases held board swaps before after callbacks; look up
      // the live topic again rather than navigating a detached history tree.
      var owner = boardQ("section.topic[data-topic]").filter(function (n) { return n.getAttribute("data-topic") === row.topic; })[0];
      if (!owner) return;
      var prepared = false;
      var entry = window.wbHistory && window.wbHistory.loadTarget(owner, row.entry, function () {
        if (prepared) return;
        prepared = true;
        prepare(owner);
      });
      // An entry in history the page has not fetched yet: pick again once
      // its topic arrives.
      var waiting = !entry && !waited && window.wbHistory && window.wbHistory.loading && window.wbHistory.loading(row.entry);
      if (waiting) { waited = true; waiting.then(entryPick); return; }
      if (!entry) { palettePick({ kind: "board-topic", topic: row.topic }, o); return; }
      // History owns disclosure, flash and viewport landing. In particular,
      // scrollIntoView would scroll its clipped box and break bottom anchoring.
      if (!(o && o.keep)) {
        entry.setAttribute("tabindex", "-1");
        entry.focus({ preventScroll: true });
      }
    }
    if (window.WsOverview && window.WsOverview.pick(topic, o)) {
      if (row.kind === "board-entry") window.WsOverview.after(function () {
        // During entry animation, after is not an exit barrier: hand off
        // again until Overview has finished its queued exit.
        if (window.WsOverview.holding()) palettePick(row, o);
        else entryPick();
      });
      return;
    }
    if (row.kind === "board-entry") { entryPick(); return; }
    prepare(topic);
    if (row.kind === "board-body") target = topic.querySelector(".tdoc") || topic;
    for (var p = target.parentElement; p && p !== topic.parentElement; p = p.parentElement) {
      if (p.tagName === "DETAILS") p.open = true;
    }
    if (target.tagName === "DETAILS") target.open = true;
    if (target.matches(".ckrow.done, .ckrow.no") && doc.documentElement.getAttribute("data-ck-hide") === "1") {
      var toggle = topic.querySelector("button.ckhide[aria-pressed=true]");
      if (toggle) toggle.click();
    }
    if (!(o && o.keep)) {
      target.setAttribute("tabindex", "-1");
      target.focus({ preventScroll: true });
    }
    target.scrollIntoView({ block: "center" });
    target.classList.remove("palette-flash");
    void target.offsetWidth;
    target.classList.add("palette-flash");
    setTimeout(function () { target.classList.remove("palette-flash"); }, 1600);
  }
  // The palette's file hooks (tracker.js keeps no page knowledge): the
  // session's file routes (fs_search.go, fs_routes.go), the preview card
  // (preview.js), and fs/act's open. A page without a session has none.
  function paletteFs(route, params, signal) {
    var qs = Object.keys(params).map(function (k) { return k + "=" + encodeURIComponent(params[k]); }).join("&");
    return fetch("/s/" + rid + "/fs/" + route + "?" + qs, { credentials: "same-origin", cache: "no-store", signal: signal }).then(function (r) {
      if (r.ok) return r.json();
      return r.text().then(function (t) { throw { status: r.status, text: (t || "").trim() }; });
    });
  }
  function paletteFiles(o) {
    if (!rid || !window.fetch) return o;
    o.fileLocal = function () { return root.hasAttribute("data-sysopen"); };
    o.fileScope = function () { var h = region("head"), m = h && h.querySelector(".meta"); return m ? m.textContent.trim() : ""; };
    o.fileFind = function (q, signal) { return paletteFs("find", { q: q }, signal); };
    o.fileSearch = function (q, signal) { return paletteFs("search", { q: q }, signal); };
    // the directory is the browsing root itself; dir equal to an absolute root is refused
    o.fileList = function (dir, signal) { return paletteFs("list", { root: dir }, signal); };
    o.filePreview = function (path, opt) { if (window.WsPreview) window.WsPreview.open(path, opt); };
    o.fileOpen = function (name, dir) {
      post("fs/act", { path: name, action: "open", root: dir }, function (ok, v) { toast(ok ? wsT("js.toast.openednative") : wsT("js.toast.openfail", v)); });
    };
    return o;
  }

  function paletteSearch(o) {
    var prefix = "ws.dashboard.drawer.search";
    o.searchState = {
      load: function () { try { return JSON.parse(load(prefix + ".state") || "null"); } catch (e) { return null; } },
      save: function (v) { save(prefix + ".state", JSON.stringify(v)); }
    };
    if (sbody) o.searchHost = {
      parent: sbody,
      contains: function (t) { return !!(t.closest && t.closest("#shandle, #drfold")); },
      setOpen: function (open) {
        if (open && !(drOpen && drMode === "search")) drSetOpen(true, "search");
        else if (!open && drOpen && drMode === "search") drSetOpen(false);
      }
    };
    return o;
  }

  function tkMount() {
    var header = doc.querySelector(".dwrap > .top"), slot = sbody ? null : $("qsslot"), wrap = doc.querySelector(".dwrap");
    if (!header || (!slot && !sbody) || !wrap) return;
    header.classList.add("tk-room");
    if (tkSupport) tkAdapter.ext = supExt();
    tkApi = window.WsTracker.mount(tkAdapter, paletteSearch(paletteFiles({ header: header, qsSlot: slot, pageParent: wrap.querySelector(":scope > .dbody") || wrap, storagePrefix: "ws.tracker.", onRender: tlTracker,
      drawerHeight: drMax, drawerBoundary: $("composer"), searchRows: paletteRows, searchPick: palettePick })));
    if (load(DR_SEARCH) === "1" || tkWantSearch) tkApi.openSearch();
    tkWantSearch = false;
  }
  // tkSearch opens the palette, or, before the first store list mounts it,
  // opens it at mount: the shortcut is never dropped while the page loads.
  var tkWantSearch = false;
  function tkSearch() {
    if (tkApi) tkApi.openSearch();
    else tkWantSearch = true;
  }
  function trackerStart() {
    window.wsCapsuleTopics = capsuleTopics;
    if (!window.WsTracker || !window.fetch) return;
    // A frame the shell opens later revalidates this frame's answers (tkLent).
    window.wsTrackerAnswer = function (id) { return tkAnswers[id] || null; };
    // Until mount installs the palette's own key listener, the shortcut
    // still counts (the same target rule), as a pending open.
    doc.addEventListener("keydown", function (e) {
      if (tkApi || e.defaultPrevented || !window.WsTracker.searchKey || !window.WsTracker.searchKey(e)) return;
      e.preventDefault();
      tkSearch();
    }, true);
    tkReload();
  }
  // trackerNudge handles the stream's tracker event: a revision other than
  // the last one handled reloads. While a reload runs (the page load's, which
  // the stream's first event usually meets), the revision waits for its end
  // and reloads again only when the store list it read brought another one,
  // so a page loads once. trackerLost forgets the revision when the stream
  // drops, since a restarted daemon counts from 0 again; the reload that
  // follows (a frame shown again, a reconnect) revalidates what the page
  // holds, so an unchanged store answers it with 304s.
  function trackerNudge(rev) {
    if (!window.WsTracker || !window.fetch) return;
    if (tkBusy) { tkNudgedRev = rev; return; }
    if (rev === tkLastRev) return;
    tkLastRev = rev;
    tkReload();
  }
  function trackerLost() { tkLastRev = null; }

  // --- tracker: the store support chat (a drawer tab) ------------------------
  // When the daemon serves the support conversations (the store list's
  // support flag), every store's drawer gets a Chat tab over the store's one
  // ongoing runner conversation (/support/stores/<store>). The renderer
  // redraws the drawer from HTML on every change, so each store's chat is one
  // DOM root kept here, which rendered() moves into the tab's slot: the log,
  // the draft, the scroll offset, and the focus survive a redraw. The log is
  // the conversation's own session regions over its event stream (entries
  // merged by key, convlive replaced), open only while the chat shows. A
  // card line (the agent called tracker.show) becomes the item's mini card
  // from the store's view, which opens the item window as any card
  // reference does. A ui line (the agent called tracker.ui) moves the
  // store's view once, when it arrives live (supActUI). The chip opens the
  // selection popover: the harness (a switch starts a new conversation,
  // after a warning), the model (fuzzy searched), and the effort (a knob
  // snapping to the levels); the runner lists the choices and remembers the
  // last one as the next default.
  var supChats = {}, supChoices = null, supChoicesBusy = false, supH = null, supStore = null, supView = null;
  function supUrl(id, op) { return "/support/stores/" + encodeURIComponent(id) + (op ? "/" + op : ""); }
  function supEsc(s) { return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]; }); }
  function supIcon(href) { return '<svg class="i"><use href="' + href + '"/></svg>'; }
  function supExt() {
    return {
      tabs: [{ key: "chat", icon: "#tki-chat", title: wsT("js.sup.title"), tip: wsT("js.sup.tip"), after: "notify", make: function (id, view, h) {
        return h.card(wsT("js.sup.title"), "", '<div class="supslot"' + h.attr("store", id) + "></div>",
          h.btn("sup-new", supIcon("#tki-plus") + wsT("js.sup.new"), { store: id }, "supnew"), "tk-chat", "", "chat");
      } }],
      rendered: supRendered,
      action: function (k, el) {
        if (k !== "sup-new" || !el) return false;
        supNew(el.getAttribute("data-store"));
        return true;
      }
    };
  }
  // supChat returns store id's chat, building its root on first use.
  function supChat(id) {
    if (supChats[id]) return supChats[id];
    var c = supChats[id] = { id: id, conv: null, sel: null, loaded: false, loading: false, st: null, stFor: null, retry: null,
      stick: true, top: 0, focus: null, sending: false, pop: false, pending: null, q: "", qi: -1, err: "" };
    var r = c.root = doc.createElement("div");
    r.className = "supchat";
    r.innerHTML = '<div class="suplog"><p class="supempty"></p><div class="conv centries"></div><div class="suplive"></div></div>' +
      '<p class="superr" hidden></p>' +
      '<div class="composer supcomp"><div class="cbadges"><span class="sbadge" title="' + supEsc(wsT("js.subagents.title")) + '" hidden>' +
      '<span class="spin"></span><svg class="i" aria-hidden="true"><use href="#i-agents"/></svg><span class="slabel"></span></span></div><div class="supbar"><button type="button" class="supchip" aria-haspopup="dialog" aria-expanded="false"></button>' +
      '<span class="grow"></span></div><div class="crow"><textarea rows="1" aria-label="' + supEsc(wsT("js.sup.title")) + '"></textarea>' +
      '<button type="button" class="csend" aria-label="' + supEsc(wsT("js.sup.send")) + '" title="' + supEsc(wsT("js.sup.send")) + '">' + supIcon("#tki-send") + "</button></div>" +
      '<div class="suppop" role="dialog" aria-label="' + supEsc(wsT("js.sup.chip.tip")) + '" hidden></div></div>';
    c.log = r.querySelector(".suplog"); c.box = r.querySelector(".centries"); c.live = r.querySelector(".suplive");
    c.ta = r.querySelector("textarea"); c.chip = r.querySelector(".supchip"); c.popEl = r.querySelector(".suppop");
    c.sbadge = r.querySelector(".sbadge");
    c.log.addEventListener("scroll", function () {
      if (!c.root.isConnected) return;
      c.top = c.log.scrollTop;
      c.stick = c.log.scrollTop + c.log.clientHeight >= c.log.scrollHeight - 24;
    });
    // The redraw detaches the root, which may or may not fire focusout; the
    // focus counts as lost only when, after the event, it sits elsewhere
    // with the root on the page.
    r.addEventListener("focusin", function (e) { c.focus = e.target; });
    r.addEventListener("focusout", function (e) {
      if (e.relatedTarget && !r.contains(e.relatedTarget)) { c.focus = null; return; }
      setTimeout(function () { if (c.root.isConnected && !c.root.contains(doc.activeElement)) c.focus = null; }, 0);
    });
    c.ta.addEventListener("input", function () { supGrow(c); });
    c.ta.addEventListener("keydown", function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      // Escape leaves the box (the next one, outside it, collapses the drawer)
      if (e.key === "Escape") { e.preventDefault(); if (c.pop) supPop(c, false); else c.ta.blur(); return; }
      if (e.key !== "Enter") return;
      var mod = e.ctrlKey || e.metaKey;
      if (inputMode === "enter" ? !e.shiftKey && !e.altKey : mod) { e.preventDefault(); supSend(c); }
    });
    r.querySelector(".csend").addEventListener("click", function () { supSend(c); c.ta.focus(); });
    c.chip.addEventListener("click", function () { supPop(c, !c.pop); });
    c.popEl.addEventListener("click", function (e) { supPopClick(c, e); });
    c.popEl.addEventListener("input", function (e) {
      if (!e.target.classList.contains("spq")) return;
      c.q = e.target.value; c.qi = c.q ? 0 : -1; supDrawModels(c);
    });
    c.popEl.addEventListener("keydown", function (e) { supPopKey(c, e); });
    supDraw(c);
    return c;
  }
  function supGrow(c) {
    c.ta.style.height = "auto";
    c.ta.style.height = Math.min(c.ta.scrollHeight, 160) + "px";
  }
  // supShown reports whether the chat's slot is on screen: in the store page,
  // or in an open drawer.
  function supShown(slot) {
    if (!slot || !slot.isConnected) return false;
    var page = $("tkpage"), dr = $("tkdrawer");
    return !!((page && page.contains(slot)) || (dr && dr.classList.contains("open") && dr.contains(slot)));
  }
  function supRendered(name, view, h) {
    supH = h; supStore = view ? name : null; supView = view;
    var slot = name && doc.querySelector('.supslot[data-store="' + CSS.escape(name) + '"]');
    Object.keys(supChats).forEach(function (id) {
      var c = supChats[id];
      if (id !== name || !supShown(slot)) { supClose(c); if (c.pop) supPop(c, false); }
      // a store no longer listed (hidden, or unregistered, which ended its
      // conversation) drops its chat with its draft and log
      if (tkOrder.indexOf(id) < 0) {
        if (c.root.parentNode) c.root.parentNode.removeChild(c.root);
        delete supChats[id];
      }
    });
    if (!supShown(slot)) return;
    var c = supChat(name);
    if (c.root.parentNode !== slot) slot.appendChild(c.root);
    c.ta.placeholder = wsT(inputMode === "mod" ? "js.sup.hint.mod" : "js.sup.hint.enter");
    supCards(c);
    c.log.scrollTop = c.stick ? c.log.scrollHeight : c.top;
    if (c.focus && c.root.contains(c.focus) && !c.root.contains(doc.activeElement)) {
      var f = c.focus, s0 = f.selectionStart, s1 = f.selectionEnd;
      f.focus({ preventScroll: true });
      try { if (s0 != null) f.setSelectionRange(s0, s1); } catch (x) { /* not a text field */ }
    }
    if (!c.loaded && !c.loading) supLoad(c); else supOpen(c);
  }
  // supLoad reads the store's conversation and selection.
  function supLoad(c) {
    c.loading = true;
    tkFetch(supUrl(c.id), undefined, function (status, v) {
      c.loading = false;
      if (status !== 200) { c.err = tkErrText(status, v); supDraw(c); return; }
      c.loaded = true; c.err = "";
      c.sel = v.selection || null;
      supSetConv(c, v.conversation || null);
      supDraw(c);
    });
  }
  // supSetConv takes the store's conversation as the routes answered it; a
  // different conversation starts an empty log and its own stream.
  function supSetConv(c, conv) {
    var was = c.conv && c.conv.id, now = conv && conv.status !== "closed" ? conv : null;
    c.conv = now;
    if ((now && now.id) !== was) {
      supClose(c);
      resetEntries(c.box); c.live.innerHTML = ""; c.stick = true;
    }
    supOpen(c);
  }
  // supOpen opens the conversation's region stream while the chat shows.
  function supOpen(c) {
    var slot = c.root.parentNode;
    if (!c.conv || !window.WebSocket || !supShown(slot) || (c.st && c.stFor === c.conv.id)) return;
    supClose(c);
    var primed = false, st;
    // The stream's first entries region is the conversation's history (on a
    // first open, a reload, or a reconnect alike); only the entries regions
    // after it are live, so only they act on the ui lines they bring.
    function onRegion(ev) {
      if (c.st !== st || !ev) return;
      var live = primed;
      if (ev.name === "entries") primed = true;
      supRegion(c, ev.name, ev.html || "", live ? st : null);
    }
    st = c.st = openStream(c.conv.page + "/events?regions=entries,convlive", {
      // A conversation the index does not list yet (just opened) or a
      // dropped stream: try again shortly while the chat still shows.
      drop: function () {
        if (c.st !== st) return;
        supClose(c);
        c.retry = setTimeout(function () { c.retry = null; supOpen(c); }, 2000);
      },
      events: { region: onRegion, gone: function () { if (c.st === st) supClose(c); } }
    });
    c.stFor = c.conv.id;
  }
  function supClose(c) {
    if (c.retry) { clearTimeout(c.retry); c.retry = null; }
    if (c.st) { c.st.close(); c.st = null; c.stFor = null; }
  }
  // supRegion applies a region of the chat's stream; st is that stream when
  // the region arrived live (see supOpen), else null. The entries merge as
  // the session conversation's do (mergeEntriesRegion), with no history to
  // page back to: what a chat at the bottom sheds past the cap is gone until
  // the chat opens again, as a reopen shows only the newest window. A window
  // past a gap starts the log over, and what that puts back is replayed
  // history, never a live arrival: its ui lines do not act.
  function supRegion(c, name, html, st) {
    var stick = c.root.isConnected ? c.log.scrollTop + c.log.clientHeight >= c.log.scrollHeight - 24 : c.stick, added = null, reset = false;
    if (name === "entries") {
      var nbox = frag(html).querySelector(".centries");
      if (nbox) {
        if (gapped(c.box, nbox)) { resetEntries(c.box); reset = true; }
        added = mergeEntries(c.box, nbox);
        added.forEach(sweepOutImgs);
        if (stick) evictEntries(c.box);
      }
      supCards(c);
    } else if (name === "convlive") {
      c.live.innerHTML = html;
      // held messages are answered from the session page, not from here
      q(".hsteer, .hcancel, .sentdata", c.live).forEach(function (n) { n.remove(); });
      restoreOpen(c.live);
      fmtTimes(c.live);
    } else return;
    supDraw(c);
    if (stick && c.root.isConnected) { c.log.scrollTop = c.log.scrollHeight; c.stick = true; }
    if (st && added && !reset) supActUI(c, added, st);
  }
  // supActUI performs, in order, the ui lines (a tracker.ui call: the agent
  // moving this store's view) that a live entries region added; every line,
  // acted on or not, stays the same inert line in the log, so a reload or a
  // reconnect, whose first region is history, never acts again. A command
  // that hides the chat (a search, scope, or view switch in the narrow
  // layout, where Items and the chat share one pane) closes its stream, and
  // the lines after it are left for the next open to meet as history.
  function supActUI(c, added, st) {
    if (!supH || !supH.uiCommand) return;
    added.forEach(function (el) {
      if (c.st !== st || !el.matches(".uiline[data-entry]")) return;
      supH.uiCommand(c.id, { op: el.getAttribute("data-ui-op"), arg: el.getAttribute("data-ui-arg") || "", comment: el.getAttribute("data-ui-comment") || "" });
    });
  }
  // supCards draws each card line as the item's mini card while the store's
  // view holds the item, else as the server drew it. The view is the one
  // the renderer last drew (crefHtml reads it), not the adapter's cache,
  // which can be ahead of it between a store's reload and the redraw.
  function supCards(c) {
    if (!supH || supStore !== c.id || !supView || !supView.items) return;
    var vs = supView, byAlias = null;
    q(".cardref[data-tk-alias]", c.box).forEach(function (el) {
      if (el.getAttribute("data-tk-store") !== c.id) return;
      if (!byAlias) {
        byAlias = {};
        Object.keys(vs.items).forEach(function (iid) { byAlias[vs.items[iid].alias] = iid; });
      }
      var iid = byAlias[el.getAttribute("data-tk-alias")];
      if (!iid) {
        // the item left the view: back to the server's line
        if (el.hasAttribute("data-fb")) { el.innerHTML = el.getAttribute("data-fb"); el.removeAttribute("data-fb"); el.removeAttribute("data-up"); el.classList.remove("up"); }
        return;
      }
      var html = supH.crefHtml(iid);
      if (el.getAttribute("data-up") === html) return;
      if (!el.hasAttribute("data-fb")) el.setAttribute("data-fb", el.innerHTML);
      el.innerHTML = html;
      el.setAttribute("data-up", html);
      el.classList.add("up");
    });
  }
  function supSelText(sel) {
    if (!sel || !sel.harness) return wsT("js.sup.none.short");
    return sel.harness + " · " + (sel.model || wsT("js.sup.default")) + " · " + (sel.effort || wsT("js.sup.default"));
  }
  // supCur is the selection the next message goes with: the open
  // conversation's own settings, else the remembered choice.
  function supCur(c) {
    if (c.conv) return { harness: c.conv.backend, model: c.conv.model || "", effort: c.conv.effort || "" };
    return c.sel || { harness: "", model: "", effort: "" };
  }
  function supDraw(c) {
    var empty = !c.box.firstElementChild && !c.live.textContent.trim();
    var e = c.root.querySelector(".supempty");
    e.hidden = !empty;
    e.textContent = !c.loaded ? (c.err ? "" : wsT("js.sup.loading")) : wsT("js.sup.empty");
    var er = c.root.querySelector(".superr");
    er.hidden = !c.err;
    er.textContent = c.err;
    var cur = supCur(c), none = c.loaded && !cur.harness;
    c.chip.innerHTML = "<span>" + supEsc(supSelText(cur)) + "</span>" + supIcon("#tki-chev");
    c.chip.title = wsT("js.sup.chip.tip");
    c.chip.setAttribute("aria-expanded", String(c.pop));
    // the session composer's subagent badge, over the convlive region the
    // log streams; the card has no interrupt badge
    subBadge(c.sbadge, c.live.querySelector(".clive"), !!c.conv);
    c.root.querySelector(".csend").disabled = c.sending || !c.loaded || none;
    c.ta.disabled = !c.loaded || none;
  }
  function supSend(c) {
    var text = c.ta.value;
    if (c.sending || !text.trim()) return;
    c.sending = true; c.err = ""; supDraw(c);
    tkFetch(supUrl(c.id, "send"), { text: text }, function (status, v) {
      c.sending = false;
      if (status !== 200) { c.err = tkErrText(status, v); supDraw(c); return; }
      if (c.ta.value === text) { c.ta.value = ""; supGrow(c); }
      c.stick = true;
      supSetConv(c, v);
      supDraw(c);
    });
  }
  function supNew(id) {
    var c = supChat(id);
    c.err = ""; supDraw(c);
    tkFetch(supUrl(id, "new"), {}, function (status, v) {
      if (status !== 200) { c.err = tkErrText(status, v); supDraw(c); return; }
      supSetConv(c, v);
      supDraw(c);
      if (c.root.isConnected) c.ta.focus();
    });
  }
  // supSelect saves a choice; a harness switch answers with a new
  // conversation, a model or effort change with the same one.
  function supSelect(c, req) {
    c.err = "";
    tkFetch(supUrl(c.id, "select"), req, function (status, v) {
      if (status !== 200) { c.err = tkErrText(status, v); supDraw(c); supDrawPop(c); return; }
      c.sel = v.selection || c.sel;
      supSetConv(c, v.conversation || null);
      supDraw(c); supDrawPop(c);
    });
  }

