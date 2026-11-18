  // --- awaiting-approval notifications -----------------------------------------
  // A row of the rail (the index's list on /) carries data-ap while its
  // hosted session waits on a tool approval. Each read of those rows, from
  // the live region or a hidden tab's held update, notifies of a wait the
  // read before did not have; the first read only sets the baseline. The
  // notification is the browser's when the user granted it (asked on a
  // click that starts a session, answers an approval, or sends a message
  // on a hosted page, never on load), else a toast once the tab shows. One
  // tag per wait keeps several open tabs to one notification. A page not
  // on the daemon's machine cannot answer, and its notification says so.
  var apSeen = null, apHeld = [];
  function notifAsk() {
    try {
      if (window.Notification && window.Notification.permission === "default") window.Notification.requestPermission();
    } catch (e) { /* no notifications in this browser */ }
  }
  function apKey(r) { return r.rid + "\n" + r.tool + "\n" + r.args; }
  function apRows(el) {
    return q("[data-ap]", el).map(function (row) {
      var g = row.closest(".rws, .project"), nm = g && g.querySelector(".nm, .pname");
      return { rid: row.getAttribute("data-rid") || "", tool: row.getAttribute("data-ap-tool") || "", args: row.getAttribute("data-ap-args") || "", project: nm ? nm.textContent : "" };
    });
  }
  function apScan(el) {
    // A framed page's shell notifies, from its rail.
    if (!el || framed) return;
    var now = {}, fresh = [];
    apRows(el).forEach(function (r) {
      var k = apKey(r);
      if (apSeen && !apSeen[k] && !now[k]) fresh.push(r);
      now[k] = true;
    });
    apSeen = now;
    apHeld = apHeld.filter(function (r) { return now[apKey(r)]; });
    fresh.forEach(apNotify);
  }
  // apText is a wait's notification: the session in the title; the tool,
  // its arguments, and (away from the daemon's machine) where to answer in
  // the body's lines.
  function apText(r) {
    var lines = [toolLabel(r.tool) + (r.tool && r.args ? " · " : "") + r.args];
    if (!root.hasAttribute("data-sysopen")) lines.push(wsT("js.notif.lan"));
    return { title: wsT("js.notif.title", (r.project ? r.project + "/" : "") + r.rid), lines: lines.filter(Boolean) };
  }
  function apGo(r) {
    if (r.rid === rid) return;
    saveDraft();
    go("/s/" + encodeURIComponent(r.rid));
  }
  function apNotify(r) {
    // the viewed session's own wait, in a tab the user is looking at, is
    // the card in front of them
    if (r.rid === rid && !doc.hidden && (!doc.hasFocus || doc.hasFocus())) return;
    var t = apText(r);
    try {
      if (window.Notification && window.Notification.permission === "granted") {
        var n = new window.Notification(t.title, { body: t.lines.join("\n"), tag: "ws-approval:" + apKey(r) });
        n.onclick = function () { window.focus(); n.close(); apGo(r); };
        return;
      }
    } catch (e) { /* a browser that notifies only through a service worker */ }
    if (doc.hidden) apHeld.push(r);
    else apToast(r);
  }
  function apToast(r) {
    if (!window.WsToast) return;
    var t = apText(r);
    window.WsToast.show([t.title].concat(t.lines).join(" · "), { kind: "notice", onClick: function () { apGo(r); } });
  }
  doc.addEventListener("visibilitychange", function () {
    if (doc.hidden) return;
    var held = apHeld;
    apHeld = [];
    held.forEach(apToast);
  });
  if (hostedPage) {
    doc.addEventListener("click", function (e) {
      var t = e.target, chip = t && t.closest && t.closest(".host.hbtn");
      if (chip && (railEl || framed) && WsRail) { e.preventDefault(); railPopOpen("sess", chip); }
      else if (t && t.closest && t.closest("#csend")) notifAsk();
    });
    doc.addEventListener("keydown", function (e) {
      var t = e.target;
      if ((e.key !== "Enter" && e.key !== " ") || !t || !t.classList || !t.classList.contains("hbtn") || !(railEl || framed) || !WsRail) return;
      e.preventDefault();
      railPopOpen("sess", t);
    });
  }

  // --- per-session drafts ---------------------------------------------------------
  // What the user typed or picked on this session's page and has not sent,
  // and the page's view state around it, is kept per session in storage
  // (rail.js drafts), so switching sessions in the rail and coming back
  // finds it: the composer's text, mentions, and selection, its slash
  // command selection, the board answers and topic notes, the answer chips, the web picks, the images that reference
  // written attachments, the open details, and the tab. Pasted images are
  // not kept: their markers leave the composer text and the notes, a
  // restored draft says so, and while the page holds one, leaving it asks
  // first (the browser's leave-page confirmation).
  // The shell holds no draft: its frames keep their own.
  var draftStore = WsRail && rid && !shell && mode !== "history" ? WsRail.drafts(railStore) : null, draftTimer = null, restoring = false, draftUnsaved = false;
  function readDraft() {
    var b = region("board"), open = {};
    Object.keys(openState).forEach(function (k) { if (openState[k] && k.indexOf("k:history:") !== 0) open[k] = true; });
    var prefix = selectionPrefix(bselections);
    var shift = function (r) { return Object.assign({}, r, { start: r.start + prefix.length, end: r.end + prefix.length }); };
    var d = {
      text: prefix + (ctext ? ctext.value : ""),
      capsules: ctext && ctext.wsc ? ctext.wsc.snapshot().map(shift) : [],
      cmd: cmdSel ? { name: cmdSel.name, args: cmdSel.args } : null,
      mentions: mentions.map(function (m) { return shift({ start: m.start, end: m.end, text: m.text }); }),
      answers: b ? boardRead(readAnswers) : {},
      chips: chipList(),
      bmentions: bmentions.slice(),
      closeRequests: closeRequests.map(function (r) { return { topic: r.topic, reason: r.reason }; }),
      wpicks: wpicks.slice(),
      wconsoles: wconsoles.slice(),
      refs: imgs.filter(function (p) { return !!p.ref; }).map(function (p) { return { ref: p.ref, name: p.name, n: p.n }; }),
      lost: imgs.filter(function (p) { return !p.ref; }).map(function (p) { return p.n; }),
      open: open,
      tab: live ? grid.getAttribute("data-tab") || "" : ""
    };
    if (ctext && d.text) d.sel = [ctext.selectionStart + prefix.length, ctext.selectionEnd + prefix.length];
    return d;
  }
  // While a send is unanswered the saved draft stays the one from before
  // it, so a page lost meanwhile restores the message rather than drop it.
  function saveDraft() {
    if (!draftStore || restoring || outboxUnsent()) return;
    clearTimeout(draftTimer);
    draftTimer = null;
    var d = readDraft();
    // View state alone is not unsent content requiring a native prompt.
    var saved = draftStore.save(rid, d);
    draftUnsaved = saved === false && !WsRail.emptyDraft(Object.assign({}, d, { open: {}, tab: "" }));
  }
  function queueDraft() {
    if (!draftStore || restoring) return;
    clearTimeout(draftTimer);
    draftTimer = setTimeout(saveDraft, 400);
  }
  function clearDraft() {
    if (!draftStore) return;
    draftStore.clear(rid);
    queueDraft();
  }
  function restoreDraft() {
    var d = draftStore && draftStore.load(rid);
    if (!d) return;
    restoring = true;
    try {
      var lost = Array.isArray(d.lost) ? d.lost : [], named = "";
      // Keep bundles in existing text storage. Extract before stripping lost
      // markers so quoted source and memo markers are never touched.
      var split = splitSelectionText(typeof d.text === "string" ? d.text : "");
      d.text = split.text;
      d.mentions = selectionRanges(d.mentions, split.cuts);
      d.capsules = selectionRanges(d.capsules, split.cuts);
      if (Array.isArray(d.sel)) {
        var caret = selectionRanges([{ start: d.sel[0], end: d.sel[1] }], split.cuts)[0];
        d.sel = caret ? [caret.start, caret.end] : [0, 0];
      }
      if (!bselections.length) bselections = split.groups.map(function (s) {
        s.images = s.images.filter(function (n) { return lost.indexOf(n) < 0; });
        return s;
      });
      bselections.forEach(function (s) { named += "\n" + s.images.map(marker).join(" "); });
      if (cchips) renderChips();
      // A lost image's marker leaves the notes as it leaves the text; what
      // the restored notes still name may bring a reference image back.
      var unlost = function (note) { return typeof note === "string" && note ? WsRail.stripMarkers(note, [], lost).text : note; };
      if (d.answers && typeof d.answers === "object") Object.keys(d.answers).forEach(function (k) {
        var a = d.answers[k];
        if (a && a.note) {
          if (a.capsules && window.WsCapsules) a.capsules = window.WsCapsules.stripMarkers(a.note, a.capsules, lost).ranges;
          a.note = unlost(a.note); named += "\n" + a.note;
        }
      });
      if (Array.isArray(d.chips)) d.chips.forEach(function (c) { if (c && c.note) { c.note = unlost(c.note); named += "\n" + c.note; } });
      if (d.open && typeof d.open === "object") {
        Object.keys(d.open).forEach(function (k) {
          // A panel folded by hand outranks a draft saved while it was open.
          if (k.indexOf("k:history:") !== 0 && !Object.prototype.hasOwnProperty.call(openState, k) && !((k.indexOf("k:entry:") === 0 || k.indexOf("k:panel:") === 0) && shutHas(rid + "/" + k.slice(2)))) openState[k] = true;
        });
        restoreOpen(doc);
      }
      var b = region("board");
      if (b && d.answers && typeof d.answers === "object") {
        boardBoxes().forEach(function (box) { writeAnswers(box, d.answers); });
        if (window.wbForm) window.wbForm();
      }
      if (cchips && Array.isArray(d.chips)) {
        d.chips.forEach(function (c) { if (c && c.id && !chips[c.id]) { putChip(c); setItem(c.id, c); } });
        syncChips();
      }
      if (cchips && !closeRequests.length && validCloseRequests(d.closeRequests).length) {
        closeRequests = validCloseRequests(d.closeRequests);
        renderChips();
      }
      if (cchips && !bmentions.length && validMentions(d.bmentions).length) {
        bmentions = validMentions(d.bmentions);
        renderChips();
      }
      if (cchips && !wpicks.length && !wconsoles.length && (validPicks(d.wpicks).length || validConsoles(d.wconsoles).length)) {
        wpicks = validPicks(d.wpicks);
        wconsoles = validConsoles(d.wconsoles);
        renderChips();
      }
      if (ccmd && !cmdSel && d.cmd && typeof d.cmd.name === "string" && d.cmd.name) setCommand(d.cmd.name, d.cmd.args);
      if (ctext && typeof d.text === "string" && d.text && !ctext.value) {
        var s = WsRail.stripMarkers(d.text, Array.isArray(d.mentions) ? d.mentions : [], lost);
        ctext.value = s.text;
        if (ctext.wsc) ctext.wsc.restore(window.WsCapsules.stripMarkers(d.text, d.capsules, lost).ranges);
        mentions = s.mentions.filter(function (m) { return liveMention(m, s.text); });
        mentionWas = ctext.value;
        named += "\n" + s.text;
        grow();
        paintMentions();
        if (Array.isArray(d.sel)) { try { ctext.setSelectionRange(d.sel[0], d.sel[1]); } catch (e) { /* out of range */ } }
      }
      if (ctext) {
        (Array.isArray(d.refs) ? d.refs : []).forEach(function (p) {
          if (!p || !p.ref || !markerNums(named)[p.n] || imgs.some(function (q) { return q.n === p.n; })) return;
          imgs.push({ name: p.name || p.ref, url: attachSrc(p.ref), data: null, ref: p.ref, n: p.n });
          imgSeq = Math.max(imgSeq, p.n);
        });
        imgs.sort(function (a, b) { return a.n - b.n; });
        renderImgs();
      }
      if (live && d.tab === "board") setTab("board");
      if (lost.length) noteWarn(wsT("js.draft.imgslost", lost.length));
    } finally {
      restoring = false;
    }
  }
  if (draftStore) {
    ["input", "change", "click", "toggle"].forEach(function (t) { doc.addEventListener(t, queueDraft, true); });
    window.addEventListener("pagehide", saveDraft);
    doc.addEventListener("visibilitychange", function () { if (doc.hidden) saveDraft(); });
    window.addEventListener("beforeunload", WsRail.leaveGuard(function () {
      saveDraft();
      var unsaved = imgs.some(function (p) { return !p.ref; }) || draftUnsaved || outboxUnsent();
      if (unsaved && framed && typeof window.wsShellLeave === "number") {
        var attempt = window.wsShellLeave;
        // Survival suggests cancellation, but is not proof: an accepted
        // blank load remains authoritative if this timer races navigation.
        setTimeout(function () { shPost("stay", { attempt: attempt }); }, 0);
      }
      return unsaved;
    }));
  }
  if (framed) {
    window.wsSaveSessionDraft = saveDraft;
    window.wsCanCacheSession = function () {
      saveDraft();
      // Arbitrary web/tool runtimes cannot be frozen by ordinary page code.
      // Keep their existing guarded fresh navigation instead of caching them.
      return !!rid && !draftUnsaved && !outboxUnsent() && !imgs.some(function (p) { return !p.ref; }) &&
        !q('iframe:not(.doc-frame), .bcard, .fpane[data-max]').length && !visible(fpane) && !tkBusy &&
        (!window.WsTerm || (window.WsTerm.cacheSafe && window.WsTerm.cacheSafe())) && !Object.keys(supChats).length;
    };
  }

  // --- iframe shell -------------------------------------------------------------
  // Safe, loaded session pages are retained up to shell.js's code bound.
  // Unsafe/tool/non-session pages still navigate through their native leave
  // guard; a cancelled destructive leave keeps the current page. The
  // address bar names the shown page (pushState; popstate shows the page it
  // goes back to). The shell's header mirrors the shown frame's (the head
  // message): its head region, its search button, and the header's padding
  // and height, so both headers lay out alike and the frame's own items
  // stay invisible under the shell's. The rail marks the session a page
  // opens on at once (shRid). A page the shell frames (framed) hands its
  // header, title, layout, scroll, and the shell's hotkeys to the shell,
  // and every link it would load as a page to the shell (go); shell.js
  // lists the messages.
  var shFrames = [], shOn = null, shWant = null, shLeaving = null, shHeadHtml = null, shSeq = 0, shTrackerRev = null;
  var shIndex = 0, shRestoring = null, shRestoreNav = null, shRestoreTimer = null, shLeaveSeq = 0;
  var shTop = doc.querySelector(".dwrap > .top"), shBody = doc.querySelector(".dwrap > .dbody");
  function shHistoryIndex() { return history.state && Number.isSafeInteger(history.state.wsShellIndex) ? history.state.wsShellIndex : shIndex; }
  function shPush(f) {
    shIndex = shHistoryIndex() + 1;
    history.pushState({ wsShellIndex: shIndex }, "", f.path + f.hash);
  }
  function shRestoreDone() {
    clearTimeout(shRestoreTimer); shRestoreTimer = null;
    shRestoring = null;
    shIndex = shHistoryIndex();
    var nav = shRestoreNav; shRestoreNav = null;
    if (nav) shOpen(nav.href, nav.push);
  }
  function shRestoreTo(index) {
    shRestoring = index;
    clearTimeout(shRestoreTimer);
    // A traversal consumed by joint frame history may never emit a shell
    // popstate. Do not leave all subsequent navigation queued indefinitely.
    shRestoreTimer = setTimeout(shRestoreDone, 1000);
    var delta = index - shHistoryIndex();
    if (delta) history.go(delta);
  }
  function shStay(ent, attempt) {
    var leave = shLeaving;
    if (!leave || leave.ent !== ent || leave.attempt !== attempt) return;
    leave.stayed = true;
    ent.el.inert = false;
    shLeaving = null;
    shIndex = leave.from;
    // A ready but never-shown destination has no reader edits. Do not keep
    // its runtime after Stay; ent.leave still lets an accepted blank win.
    if (shWant && shWant !== ent) shDrop(shWant);
    // Back has already moved history. Restore the position, never its URL:
    // replaceState here would erase the destination entry from Back/Forward.
    if (shHistoryIndex() !== leave.from) {
      shRestoreTo(leave.from);
    }
  }
  // shPost sends a framed page's message of kind to its shell.
  function shPost(kind, m) {
    if (!framed) return;
    m = m || {};
    m.wsShell = kind;
    try { window.parent.postMessage(m, location.origin); } catch (x) { /* the shell left */ }
  }
  function shParentDoc() { try { return window.parent.document; } catch (x) { return doc; } }
  // go shows the page href names: a framed page asks its shell, the shell
  // shows it in a frame, and any other page, or a page the shell does not
  // frame, loads at the top level.
  function go(href) {
    var u = new URL(href, location.href);
    if (shell) { shOpen(u.href, true); return; }
    if (framed) {
      if (window.WsShell.framed(u, location.origin)) shPost("nav", { href: u.href });
      else window.top.location.href = u.href;
      return;
    }
    location.href = u.href;
  }
  // A plain click on a link the page would load goes through go, but a
  // link to the page's own document (a hash route) and one that opens
  // elsewhere (a target, a download, a modified click).
  if (shell || framed) window.addEventListener("click", function (e) {
    if (e.defaultPrevented || e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var a = e.target && e.target.closest && e.target.closest("a[href]");
    if (!a || (a.target && a.target !== "_self" && a.target !== "_top") || a.hasAttribute("download")) return;
    var u;
    try { u = new URL(a.href, location.href); } catch (x) { return; }
    if (u.protocol !== "http:" && u.protocol !== "https:") return;
    if (u.origin === location.origin && u.pathname === location.pathname && u.search === location.search && u.hash) return;
    if (shell && !window.WsShell.framed(u, location.origin)) return;
    e.preventDefault();
    go(u.href);
  });

  // shHolds reports a frame holding unsent work; shDrafts any frame, which
  // an assets reload of the shell would drop.
  function shHolds(x) {
    try { return !!(x.el.contentWindow.wsHoldsDraft && x.el.contentWindow.wsHoldsDraft()); } catch (e) { return false; }
  }
  function shDrafts() { return shFrames.some(shHolds); }
  function shSend(ent, kind, m) {
    m = m || {};
    m.wsShell = kind;
    try { ent.el.contentWindow.postMessage(m, location.origin); } catch (x) { /* unloading */ }
  }
  function shEnt(win) { return shFrames.filter(function (x) { return x.el.contentWindow === win; })[0] || null; }
  function shCacheable(ent) {
    if (!window.WsShell.cacheLimit || !ent || !ent.loaded || !ent.f.rid || ent.leave) return false;
    try { return !!(ent.el.contentWindow.wsCanCacheSession && ent.el.contentWindow.wsCanCacheSession()); } catch (e) { return false; }
  }
  function shDrop(ent) {
    try {
      if (ent.el.contentWindow.wsSaveSessionDraft) ent.el.contentWindow.wsSaveSessionDraft();
      if (ent.el.contentWindow.wsSaveSessionView) ent.el.contentWindow.wsSaveSessionView();
    } catch (e) { /* only safe or never-shown documents are discarded */ }
    shFrames.splice(shFrames.indexOf(ent), 1);
    if (shWant === ent) shWant = null;
    ent.el.remove();
  }
  function shPrune() {
    var limit = Math.max(1, window.WsShell.cacheLimit || 0);
    while (shFrames.length > limit) {
      var old = shFrames.filter(function (x) { return x !== shOn && x !== shWant; })[0];
      if (!old) break;
      shDrop(old);
    }
  }
  // A blank navigation fires load only after the existing beforeunload
  // confirmation accepted it. Removing an iframe directly would bypass that
  // guard. Late loads/messages from a removed frame have no shell authority.
  function shAdd(el, f) {
    var ent = { el: el, f: f, head: null, title: "", pincol: false, dim: 0, y: 0, loaded: false };
    el.addEventListener("load", function () {
      if (shFrames.indexOf(ent) < 0) return;
      // Retain the latest attempt on the frame even after a tentative Stay.
      // If its blank navigation commits, acceptance wins over that signal.
      if (ent.leave) {
        var blank = false;
        try { blank = el.contentWindow.location.href === "about:blank"; } catch (x) { /* navigating */ }
        if (blank) {
          var next = ent.leave;
          ent.leave = null; shLeaving = null;
          shFrames.splice(shFrames.indexOf(ent), 1);
          if (shOn === ent) shOn = null;
          if (shWant === ent) shWant = null;
          el.remove();
          if (!next.push) shIndex = next.index;
          shStart(next.f, next.push);
          // A tentative Stay may already have restored (or be restoring)
          // Back's position. Reconcile it with the accepted destination.
          if (!next.push && next.stayed && (shRestoring !== null || shHistoryIndex() !== next.index)) shRestoreTo(next.index);
          return;
        }
      }
      ent.loaded = true;
      shSettings(ent);
      if (shWant === ent) shShow(ent);
      else shSend(ent, "shown", { shown: ent === shOn });
    });
    shFrames.push(ent);
    return ent;
  }
  // shOpen shows the page href names (a page the shell frames; any other
  // loads at the top level); push adds a history entry for it.
  function shOpen(href, push) {
    // history.go is asynchronous; a pick after Stay must wait until Back's
    // position is restored, rather than pushing from its destination entry.
    if (shRestoring !== null) { shRestoreNav = { href: href, push: push }; return; }
    var u = new URL(href, location.href), f = window.WsShell.framed(u, location.origin);
    if (!f) { location.href = u.href; return; }
    // A destination that never became visible owns no reader edits. A rapid
    // pick supersedes it without parking an extra loading document.
    if (window.WsShell.cacheLimit && shOn && shWant && shWant !== shOn) shDrop(shWant);
    var ent = shOn || shWant;
    if (ent && ent.f.path === f.path && !shLeaving) {
      ent.el.inert = false;
      if (push && location.pathname + location.hash !== f.path + f.hash) shPush(f);
      if (!push) shIndex = shHistoryIndex();
      if (f.hash !== ent.f.hash) {
        try { ent.el.contentWindow.location.hash = f.hash; } catch (x) { /* unloading */ }
      }
      ent.f = f;
      return;
    }
    if (!ent) { shStart(f, push); return; }
    if (shLeaving && shLeaving.ent === ent) {
      // A rapid second Back/rail pick replaces only the destination, not the
      // history entries or the native navigation already awaiting acceptance.
      shLeaving.f = f; shLeaving.push = push; shLeaving.index = shHistoryIndex();
      return;
    }
    if (shCacheable(ent)) {
      try { if (ent.el.contentWindow.wsSaveSessionView) ent.el.contentWindow.wsSaveSessionView(); } catch (e) {}
      // The outgoing page may remain painted during a cold load, but cannot
      // acquire a new unsaved edit after the cache-safety check.
      ent.el.inert = true;
      if (ent.el.blur) ent.el.blur();
      shStart(f, push, shIndex);
      return;
    }
    shLeave(ent, f, push);
  }
  function shLeave(ent, f, push, from) {
    ent.el.inert = false;
    shLeaving = { ent: ent, f: f, push: push, from: from === undefined ? shIndex : from, index: shHistoryIndex(), attempt: ++shLeaveSeq };
    ent.leave = shLeaving;
    // Save even a draft whose debounce has not fired. The native navigation,
    // not a custom confirm or a remove(), asks before losing pasted images.
    try {
      if (ent.el.contentWindow.wsSaveSessionDraft) ent.el.contentWindow.wsSaveSessionDraft();
      if (ent.el.contentWindow.wsSaveSessionView) ent.el.contentWindow.wsSaveSessionView();
      ent.el.contentWindow.wsShellLeave = shLeaving.attempt;
      ent.el.contentWindow.location.replace("about:blank");
    } catch (x) { ent.leave = null; shLeaving = null; }
  }
  function shStart(f, push, from) {
    if (push && location.pathname + location.hash !== f.path + f.hash) shPush(f);
    if (!push) shIndex = shHistoryIndex();
    var cached = window.WsShell.cacheLimit && shFrames.filter(function (x) { return x.loaded && x.f.rid && x.f.path === f.path && !x.leave; })[0];
    if (cached) {
      if (cached.f.hash !== f.hash) try { cached.el.contentWindow.location.hash = f.hash; } catch (e) {}
      cached.f = f;
      shShow(cached, from);
      return;
    }
    var el = doc.createElement("iframe");
    el.className = shOn ? "sframe" : "sframe on";
    el.title = doc.title;
    shWant = shAdd(el, f);
    shWant.from = from;
    el.src = window.WsShell.src(f);
    $("frames").appendChild(el);
    if (shOn) return; // Keep the previous pixels until the target loads.
    shRid(f.rid);
    shHead({ head: null });
    shLayout({ pincol: false, dim: 0 });
    shScroll({ y: 0 });
  }
  // shRid marks the session the shell shows (rid, the rail's viewed row,
  // the memo pad's workspace).
  function shRid(id) {
    rid = id;
    root.setAttribute("data-rid", id);
    railMarks();
  }
  function shShow(ent, from) {
    // load can precede the frame's first rAF/header message. Keep the old
    // page and header together until the destination's chrome is ready.
    if (!ent.loaded || !ent.head) return;
    var was = shOn;
    if (was === ent) return;
    // Async tool/draft callbacks can change eligibility during a cold load.
    // Keep the ready destination, but obtain native leave acceptance before
    // hiding or destroying an outgoing document that is no longer safe.
    if (was && !shCacheable(was)) {
      shSend(ent, "shown", { shown: false });
      if (!shLeaving) shLeave(was, ent.f, false, from === undefined ? ent.from : from);
      return;
    }
    shWant = null;
    // A rename in progress names the session it was started for.
    var sn = region("head") && region("head").querySelector(".sname");
    if (sn && labelEditing()) labelEnd(sn);
    headHeld = null;
    shOn = ent;
    shFrames.splice(shFrames.indexOf(ent), 1); shFrames.push(ent);
    if (ent.cached) {
      shSettings(ent);
      shSend(ent, "theme", { theme: root.getAttribute("data-theme") || "" });
      ent.cached = false;
    }
    ent.el.inert = false;
    ent.el.classList.add("on");
    if (was) {
      was.cached = true;
      was.el.classList.remove("on");
      shSend(was, "shown", { shown: false });
    }
    shPrune();
    shSend(ent, "shown", { shown: true });
    shRid(ent.f.rid);
    shHead(ent);
    shLayout(ent);
    shScroll(ent);
    if (ent.title) doc.title = ent.title;
    try { ent.el.contentWindow.focus(); } catch (x) { /* unloading */ }
  }
  // Private UI state only: no daemon protocol or persistence contract changes.
  function shSettings(ent) { shSend(ent, "settings", { layout: layoutMode, input: inputMode }); }
  function shReload() { if (shOn) try { shOn.el.contentWindow.location.reload(); } catch (x) { /* unloading */ } }
  // shHead shows the shown frame's header: its head region (none while the
  // frame has not named one) and search button, or with the head null the
  // frame's own header (sh-own) beside the shell's rail button.
  function shHead(ent) {
    var m = ent.head, html = m ? m.html : "", qs = $("qsslot");
    shTop.classList.toggle("sh-own", html === null);
    if (html === null) html = "";
    if (html !== shHeadHtml) {
      shHeadHtml = html;
      shSeq++;
      swap("head", "f" + shSeq, html);
    }
    if (qs && qs.innerHTML !== (m ? m.qs : "")) qs.innerHTML = m ? m.qs : "";
    shTop.style.paddingRight = m ? m.pad + "px" : "";
    shTop.style.minHeight = m ? m.top + "px" : "";
    railRenameNew();
  }
  // shLayout takes the shown frame's pinned column (the rail overlays
  // beside it below 2200px) and its backdrop's dim.
  function shLayout(ent) {
    if (ent.pincol !== root.hasAttribute("data-pincol")) {
      if (ent.pincol) root.setAttribute("data-pincol", ""); else root.removeAttribute("data-pincol");
      railOpen(false);
    }
    if (ent.popover) root.setAttribute("data-fpopover", ""); else root.removeAttribute("data-fpopover");
    if (ent.dim > 0) {
      root.style.setProperty("--fdim", String(Math.max(0, 1 - ent.dim)));
      root.setAttribute("data-fdim", "");
    } else root.removeAttribute("data-fdim");
  }
  // shScroll moves the header and the rail with a frame whose page scrolls
  // as a whole, as its own would move: the header scrolls away, and the
  // rail rises with it until it stands at the wrap's top padding.
  function shScroll(ent) {
    var y = Math.max(0, ent.y || 0);
    shTop.style.transform = y ? "translateY(" + -y + "px)" : "";
    if (shBody) shBody.style.marginTop = y ? -Math.min(y, shTop.offsetHeight + 10) + "px" : "";
  }
  function shSearch() {
    if (!shOn) return;
    try { shOn.el.focus(); } catch (x) { /* unloading */ }
    shSend(shOn, "search");
  }
  function overviewButton(d) {
    var b = d && d.querySelector(".board-overview");
    return b && b.getClientRects().length ? b : null;
  }
  // Keyboard events do not cross the session iframe. The rail forwards to
  // the shown frame; both paths activate the same visible toolbar control.
  doc.addEventListener("keydown", function (e) {
    if (e.defaultPrevented || e.isComposing || e.keyCode === 229 || !e.ctrlKey || e.metaKey || e.altKey || e.shiftKey || (e.key || "").toLowerCase() !== "q") return;
    var t = e.target;
    if (t && (t.isContentEditable || (t.closest && t.closest("input, textarea, select, [contenteditable]:not([contenteditable=\"false\"]), .xterm, .tm-host")))) return;
    var b;
    if (shell) {
      try { b = shOn && overviewButton(shOn.el.contentDocument); } catch (x) { return; }
      if (!b) return;
      shSend(shOn, "overview");
    } else {
      if (shHidden) return;
      b = overviewButton(doc);
      if (!b) return;
      b.click();
    }
    e.preventDefault();
  });
  if (shell && window.WsShell) {
    window.addEventListener("message", function (e) {
      if (e.origin !== location.origin) return;
      var m = window.WsShell.message(e.data), ent = m && shEnt(e.source), on = !!ent && ent === shOn;
      if (!ent) return;
      switch (m.wsShell) {
        case "head":
          ent.head = m;
          if (on) shHead(ent);
          else if (ent === shWant) shShow(ent);
          break;
        case "title": ent.title = m.title; if (on) doc.title = m.title; break;
        case "nav": if (on) shOpen(m.href, true); break;
        case "stay": shStay(ent, m.attempt); break;
        case "key":
          if (!on) break;
          if (m.key === "rail") railKey();
          else if (m.key === "esc" && railShown()) railOpen(false);
          break;
        case "layout": ent.pincol = m.pincol; ent.dim = m.dim; ent.popover = m.popover === true; if (on) shLayout(ent); break;
        case "scroll": ent.y = m.y; if (on) shScroll(ent); break;
        case "hash": if (on && !shLeaving && shRestoring === null && location.hash !== m.hash) history.replaceState(history.state, "", ent.f.path + m.hash); ent.f.hash = m.hash; break;
        case "theme":
          setTheme(m.theme, true);
          shFrames.forEach(function (x) { if (x !== ent) shSend(x, "theme", { theme: m.theme }); });
          break;
      }
    });
    window.addEventListener("popstate", function () {
      if (shRestoring !== null) {
        var delta = shRestoring - shHistoryIndex();
        if (delta) history.go(delta);
        else shRestoreDone();
        return;
      }
      shOpen(location.pathname + location.hash, false);
    });
    // The shell's hotkeys besides the rail's: the search palette's keys
    // (tracker.js qShortcut) open the shown frame's palette; Ctrl+K,
    // Ctrl+Space and Cmd+K stay with a terminal by qShortcut's target rule.
    doc.addEventListener("keydown", function (e) {
      if (e.isComposing || e.keyCode === 229 || e.altKey || e.defaultPrevented) return;
      var t = e.target, typing = !!(t && t.closest && t.closest("input, textarea, select, [contenteditable]:not([contenteditable=\"false\"])"));
      var term = !!(t && t.closest && t.closest(".xterm")), k = (e.key || "").toLowerCase() === "k";
      if (e.ctrlKey || e.metaKey ? !term && !e.shiftKey && (e.metaKey ? !e.ctrlKey && k : k || e.code === "Space" || e.key === " ") : e.key === "/" && !typing) {
        e.preventDefault();
        shSearch();
      }
    });
    if (shTop) shTop.addEventListener("click", function (e) {
      var t = e.target;
      if (!t || !t.closest) return;
      if (t.closest("#tkqs")) { e.preventDefault(); shSearch(); return; }
      if (t.closest(".host.hbtn") && shOn) { e.preventDefault(); try { shOn.el.focus(); } catch (x) {} shSend(shOn, "menu"); }
    });
    if (shTop) shTop.addEventListener("keydown", function (e) {
      var t = e.target;
      if ((e.key !== "Enter" && e.key !== " ") || !t || !t.classList || !t.classList.contains("hbtn") || !shOn) return;
      e.preventDefault();
      try { shOn.el.focus(); } catch (x) {}
      shSend(shOn, "menu");
    });
  }
  // shFrameInit wires a framed page to its shell: the header, the layout,
  // the scroll, and hash routes go up as they change, and the shell's
  // messages come down.
  function shFrameInit() {
    var sent = {}, queued = null, head = null, headDirty = true;
    function send(kind, m) {
      var k = JSON.stringify(m);
      if (sent[kind] === k) return;
      sent[kind] = k;
      shPost(kind, m);
    }
    // inline gives the search slot's icons their own paths: the shell
    // holds none of the tracker's symbols.
    function inline(el) {
      var c = el.cloneNode(true);
      q("svg use", c).forEach(function (u) {
        var id = (u.getAttribute("href") || "").replace(/^#/, ""), sym = id ? doc.getElementById(id) : null, svg = u.parentNode;
        if (!sym || !svg) return;
        if (sym.getAttribute("viewBox")) svg.setAttribute("viewBox", sym.getAttribute("viewBox"));
        svg.innerHTML = sym.innerHTML;
      });
      return c.innerHTML;
    }
    // dim is the alpha of the backdrop the page shows over the whole
    // window: a modal dialog's, or an overlay's (the item and git windows,
    // the image viewer, a maximized card's scrim).
    function dim() {
      var a = window.WsOverview && window.WsOverview.active() ? 0.72 : 0;
      q("dialog[open]").forEach(function (d) {
        var modal = false;
        try { modal = d.matches(":modal"); } catch (x) { modal = false; }
        if (modal) a = Math.max(a, window.WsShell.alpha(getComputedStyle(d, "::backdrop").backgroundColor));
      });
      q(".tk-iwin-back:not([hidden]), .gwin-back:not([hidden]), .wbv:not([hidden]), .bc-scrim:not([hidden])").forEach(function (el) {
        if (el.getClientRects().length) a = Math.max(a, window.WsShell.alpha(getComputedStyle(el).backgroundColor));
      });
      return Math.round(a * 100) / 100;
    }
    function post() {
      queued = null;
      if (shHidden) return;
      // Board/root mutations only need the open-overlay state. Re-measure
      // the mirrored header when it changes or resizes, not on every post.
      if (headDirty) {
        headDirty = false;
        var top = doc.querySelector(".dwrap > .top"), h = region("head"), qs = $("qsslot");
        head = top ? {
          html: h ? h.innerHTML : null, qs: qs ? inline(qs) : "",
          pad: (parseFloat(getComputedStyle(top).paddingRight) || 0) + Math.max(0, window.innerWidth - root.clientWidth),
          top: top.offsetHeight
        } : null;
      }
      // A link menu's local z-index cannot outrank the shell header either;
      // lift its frame through the same layer as the other popovers. The
      // read-only hover card (rail.js, .shp) is a tooltip and never lifts.
      // While lifted for a popover the page drops its own background
      // (data-lifted, dashboard-shell.css), so the shell's header and rail
      // stay visible under the raised frame instead of going blank.
      var popover = q(".rpop:not([hidden]), .lmenu").some(function (el) { return !(el.classList && el.classList.contains("shp")) && el.getClientRects().length > 0; });
      var layout = { pincol: root.hasAttribute("data-pincol"), dim: dim(), popover: popover };
      if (root.hasAttribute("data-lifted") !== popover) {
        if (popover) root.setAttribute("data-lifted", ""); else root.removeAttribute("data-lifted");
      }
      if (head) send("head", head);
      send("layout", layout);
    }
    function queue() { if (!shHidden && !queued) queued = requestAnimationFrame(post); }
    function queueHead() { headDirty = true; queue(); }
    doc.addEventListener("ws-overview", queue);
    var top = doc.querySelector(".dwrap > .top");
    if (window.ResizeObserver && top) new ResizeObserver(queueHead).observe(top);
    if (window.MutationObserver) {
      var mo = new MutationObserver(queue);
      if (top) new MutationObserver(queueHead).observe(top, { subtree: true, childList: true, attributes: true, characterData: true });
      mo.observe(root, { attributes: true, attributeFilter: ["style", "data-pincol"] });
      mo.observe(doc.body, { subtree: true, attributes: true, attributeFilter: ["open", "hidden"] });
      // A second observe() of the same node on one observer replaces its
      // options, so the body's direct children (a popover appended already
      // shown) need their own observer.
      new MutationObserver(queue).observe(doc.body, { childList: true });
    }
    window.addEventListener("resize", queueHead);
    window.addEventListener("scroll", function () { send("scroll", { y: Math.round(window.scrollY) }); });
    // A hash route moves by hashchange or by the page's own history calls
    // (tracker.js's store pages), which fire none: either way the shell's
    // address takes the hash.
    function hash() { send("hash", { hash: location.hash }); }
    window.addEventListener("hashchange", hash);
    window.addEventListener("popstate", hash);
    ["pushState", "replaceState"].forEach(function (k) {
      var orig = history[k];
      if (typeof orig !== "function") return;
      history[k] = function () {
        var r = orig.apply(history, arguments);
        hash();
        return r;
      };
    });
    // Escape and the rail hotkey reach the shell from inside the page (the
    // rail's own handler is the shell's).
    doc.addEventListener("keydown", function (e) {
      if (e.code === "Backquote" && e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && !e.isComposing) { e.preventDefault(); railKey(); }
      else if (e.key === "Escape" && !e.defaultPrevented) shPost("key", { key: "esc" });
    });
    window.addEventListener("message", function (e) {
      if (e.source !== window.parent || e.origin !== location.origin) return;
      var m = window.WsShell.message(e.data);
      if (!m) return;
      if (m.wsShell === "shown") {
        shHidden = !m.shown;
        if (shHidden && queued) { cancelAnimationFrame(queued); queued = null; }
        syncPageVisibility();
        if (shHidden) {
          // Opaque author scripts have no safe general pause API. Retain
          // their placeholder heights, not their running child documents.
          suspendDocFrames();
          return;
        }
        showRegions();
        mountFramesPaced(doc);
        tickTimes();
        if (shTrackerRev !== null) { var rev = shTrackerRev; shTrackerRev = null; trackerNudge(rev); }
        queue();
      } else if (m.wsShell === "search") {
        // Open, do not toggle a hidden header button via a synthetic click.
        tkSearch();
      } else if (m.wsShell === "overview") {
        var b = !shHidden && overviewButton(doc);
        if (b) b.click();
      } else if (m.wsShell === "menu") {
        var h = region("head"), chip = h && h.querySelector(".host.hbtn");
        if (chip && WsRail) railPopOpen("sess", chip);
      } else if (m.wsShell === "settings") { setLayout(m.layout); setInput(m.input); }
      else if (m.wsShell === "theme") setTheme(m.theme, true);
      else if (m.wsShell === "sync") { sent = {}; queueHead(); shPost("title", { title: doc.title }); }
    });
    queue();
    shPost("title", { title: doc.title });
    if (window.scrollY) send("scroll", { y: Math.round(window.scrollY) });
  }
  // shShellInit takes the frame the shell page loaded with: the page its
  // address names (its path and hash, which never reached the daemon; a
  // query, such as a LAN token, is not the page's) and asks it for its
  // header, in case it posted before this script listened. A frame already
  // loaded fires no load for shAdd to see.
  function shShellInit() {
    var el = doc.querySelector("#frames > iframe"), f = window.WsShell.framed(new URL(location.pathname + location.hash, location.origin), location.origin);
    if (!el || !f) return;
    shIndex = shHistoryIndex();
    history.replaceState(Object.assign({}, history.state, { wsShellIndex: shIndex }), "", location.href);
    var ent = shAdd(el, f);
    shOn = ent;
    if (f.hash) { el.src = window.WsShell.src(f); return; }
    try { ent.loaded = el.contentDocument.readyState === "complete"; } catch (x) { /* not yet */ }
    shSend(ent, "sync");
    if (ent.loaded) shSettings(ent);
  }

  if (live || gmenu) {
    setLayout(load(LAYOUT_KEY));
    setInput(load(INPUT_KEY));
  }
  if (worktree) revealPanes();
  if (live) {
    setTab("convo");
    setCtab("convo");
  }
  restoreOpen(doc);
  // A body frame that posted its height before the listener above existed
  // posts it again.
  q("iframe.doc-frame").forEach(function (f) { postDoc(f, { wbDoc: "size" }); });
  if (window.wbForm) window.wbForm();
  bindCapsuleNotes();
  // The served board's frames mount once the cards have moved the pinned
  // topics into the pin list, so no frame loads and then moves, paced so the
  // whole board's mounting never lengthens one long task, and once the
  // board's script (wbForm) has set up wbHistory, whose visibility source
  // the viewport bands watch through.
  boardBoxes().forEach(bandBoard);
  mountFramesPaced(doc);
  if (cardBoard) mountBoardOverview();
  boardRestoreReload();
  applySent();
  restoreDraft();
  tlCount();
  apScan(region(mode === "index" ? "sessions" : "rail"));
  if (hostedPage) { lead.key = leadKey(); leadFetch(); }
  else if (handedPage) leadFetch();
  syncBadge();
  fmtTimes(doc);
  mermaid();
  pinConvo();
  restoreSessionView();
  nearTop();
  if (framed) shFrameInit();
  if (shell && window.WsShell) shShellInit();
  // --- desktop app update
  // Only the Werkschaft desktop shell's preload defines werkschaftDesktop;
  // a browser tab (or an older shell) never shows the row or the toasts.
  // Status: { state, version, message, skipped, phase: { phase, version,
  // skipped } }; state drives the toast, phase (unchecked | none |
  // available | ready) the settings line. downloadPage, when the shell sends
  // it, adds [Open download page] (replacing [Update] on available: a shell
  // that does not update itself); the link opens in the system browser
  // through the shell's window-open handler.
  function desktopUpdate() {
    var api = window.werkschaftDesktop && window.werkschaftDesktop.update;
    var card = gmenu && gmenu.querySelector(".gmenu");
    if (!api || typeof api.onStatus !== "function" || !card || !window.WsToast) return;
    var note = card.querySelector(".gn");
    function item(id, mark, label) {
      var b = doc.createElement("button");
      b.type = "button";
      b.id = id;
      b.innerHTML = '<span class="gk"></span><span><span class="dupd-label"></span><span class="gs"></span></span>';
      b.querySelector(".gk").textContent = mark;
      b.querySelector(".dupd-label").textContent = label;
      card.insertBefore(b, note);
      return b;
    }
    var head = doc.createElement("span");
    head.className = "gt gt2";
    head.textContent = wsT("js.update.title");
    card.insertBefore(head, note);
    var check = item("dupd-check", "↻", wsT("js.update.check"));
    var restart = item("dupd-restart", "⏻", wsT("js.update.restart"));
    restart.hidden = true;
    check.addEventListener("click", function () { api.check(); });
    restart.addEventListener("click", function () { api.restart(); });
    function phaseLine(p) {
      check.querySelector(".gs").textContent = !p ? "" :
        p.phase === "none" ? wsT("js.update.state.none") :
        p.phase === "available" ? wsT("js.update.state.available", p.version) + (p.skipped ? " " + wsT("js.update.skipped") : "") :
        p.phase === "ready" ? wsT("js.update.state.ready", p.version) : "";
      restart.hidden = !p || p.phase !== "ready";
    }
    var open = null;
    function toast(text, opts) { if (open) open.close(); open = WsToast.show(text, opts); }
    var close = wsT("js.card.close");
    api.onStatus(function (s) {
      if (!s || typeof s.state !== "string") return;
      if (s.phase) phaseLine(s.phase);
      var v = String(s.version || ""), m = String(s.message || "");
      var page = typeof s.downloadPage === "string" && /^https:\/\//.test(s.downloadPage) ? s.downloadPage : "";
      var pageAct = page ? [{ label: wsT("js.update.downloadpage"), run: function () { window.open(page, "_blank", "noopener"); } }] : [];
      if (s.state === "available") {
        toast(wsT("js.update.available", v) + (s.skipped ? " " + wsT("js.update.skipped") : ""), { sticky: true, dismiss: close, actions:
          (page ? pageAct : [{ label: wsT("js.update.install"), run: function () { api.download(); } }]).concat(
          [{ label: wsT("js.update.skip"), run: function () { api.skip(v); } }]) });
      } else if (s.state === "downloading") toast(wsT("js.update.downloading", v), { sticky: true });
      else if (s.state === "ready") {
        toast(wsT("js.update.ready", v), { sticky: true, dismiss: close, actions: [
          { label: wsT("js.update.restart"), run: function () { api.restart(); } }] });
      } else if (s.state === "current") toast(wsT("js.update.current", v));
      else if (s.state === "error") toast(wsT("js.update.failed"), { kind: "warn" });
      else if (s.state === "download-error") toast(wsT("js.update.downloadfailed", m), { kind: "warn", sticky: true, dismiss: close, actions: pageAct });
      else if (s.state === "apply-error") toast(wsT("js.update.applyfailed", m), { kind: "warn", sticky: true, dismiss: close, actions: pageAct });
    });
  }
  if (!shell) trackerStart();
  desktopUpdate();
  connect();
})();
