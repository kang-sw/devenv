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
  // Keep the legacy text projection readable by older dashboard pages.
  // Blobs and delivery records live separately in browser-owned IndexedDB.
  var draftStore = WsRail && rid && !shell && mode !== "history" ? WsRail.drafts(railStore) : null;
  var draftTimer = null, restoring = false, draftUnsaved = false, draftDetached = false;
  var draftDB = null, draftReady = null, draftLoaded = false;
  var draftLegacy = null, draftCommitted = null, draftWrite = null, draftWriteKey = "";
  var draftDeliveryState = new Map();

  function draftDatabase() {
    return new Promise(function (resolve) {
      var request;
      try { request = window.indexedDB.open("ws.dashboard.recovery", 1); }
      catch (e) { resolve(null); return; }
      request.onupgradeneeded = function () { request.result.createObjectStore("drafts"); };
      request.onerror = request.onblocked = function () { resolve(null); };
      request.onsuccess = function () { resolve(request.result); };
    });
  }
  function draftTransaction(write, update) {
    return new Promise(function (resolve, reject) {
      var tx = draftDB.transaction("drafts", write ? "readwrite" : "readonly");
      var store = tx.objectStore("drafts"), request = store.get(rid), result;
      request.onsuccess = function () {
        result = request.result || null;
        if (!write) return;
        try {
          result = update(result);
          if (result) store.put(result, rid);
          else store.delete(rid);
        } catch (e) { tx.abort(); }
      };
      tx.oncomplete = function () { resolve(result); };
      tx.onabort = function () { reject(tx.error || new Error("Draft transaction aborted")); };
    });
  }
  function draftProjection(d) { return WsRail.emptyDraft(d) ? null : JSON.stringify(d); }
  function savedImage(p) { return { n: p.n, name: p.name, ref: p.ref, blob: p.blob }; }
  function loadDraftImage(p) {
    return p.ref ? { n: p.n, name: p.name, ref: p.ref, url: attachSrc(p.ref), data: null } :
      { n: p.n, name: p.name, ref: "", blob: p.blob, url: URL.createObjectURL(p.blob), data: null };
  }
  function imageData(blob) {
    return new Promise(function (resolve, reject) {
      var reader = new FileReader();
      reader.onload = function () { var value = String(reader.result); resolve(value.slice(value.indexOf(",") + 1)); };
      reader.onerror = reject;
      reader.readAsDataURL(blob);
    });
  }
  function saveLegacyDraft(d, projection) {
    if (projection === draftLegacy) return true;
    if (draftStore.save(rid, d) === false) return false;
    draftLegacy = projection;
    return true;
  }
  function readDraft() {
    var b = region("board"), open = {};
    Object.keys(openState).forEach(function (k) { if (openState[k] && k.indexOf("k:history:") !== 0) open[k] = true; });
    var prefix = inlineEditor() ? "" : selectionPrefix(bselections);
    var shift = function (r) { return Object.assign({}, r, { start: r.start + prefix.length, end: r.end + prefix.length }); };
    var d = {
      boardIncarnation: typeof boardIncarnation === "string" ? boardIncarnation : null,
      text: prefix + (ctext ? ctext.value : ""),
      capsules: ctext && ctext.wsc ? ctext.wsc.snapshot().map(shift) : [],
      selections: inlineEditor() ? bselections.map(function (s) { return { key: s.key, mentions: s.mentions, comment: s.comment, images: s.images }; }) : [],
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
  function draftRecoveryKey(record, states) {
    var images = record ? record.images : [];
    return JSON.stringify([record && record.projection, images.map(function (p) { return [p.n, p.name]; }), [...states]]);
  }
  function saveDraft() {
    if (!draftStore || !draftLoaded || restoring || draftDetached) return Promise.resolve(!draftUnsaved);
    clearTimeout(draftTimer);
    draftTimer = null;
    var d = readDraft(), projection = draftProjection(d), previous = draftLegacy;
    var images = imgs.filter(function (p) { return !p.ref && p.blob; }).map(savedImage);
    var deliveries = outbox.filter(function (m) { return !m.acked; }).map(function (m) {
      var body = Object.assign({}, m.body);
      delete body.images;
      return { id: m.recoveryID, state: m.recoveryState === "queued" ? "queued" : "unknown",
        body: body, back: m.back, pics: m.pics.map(savedImage) };
    });
    // Keep the old pre-send projection until the recovery record commits.
    // A POST starts only after this transaction (including its mirror).
    var mirrorNow = !deliveries.length && !draftDeliveryState.size;
    var textSaved = mirrorNow && saveLegacyDraft(d, projection);
    var missingBlob = (d.lost || []).length !== images.length;
    var hasDraft = !WsRail.emptyDraft(Object.assign({}, d, { open: {}, tab: "" }));
    var rich = !!(images.length || deliveries.length || mirrorNow && !textSaved && hasDraft);
    var record = rich ? {
      base: previous, projection: projection, draft: d, images: images
    } : null;
    var states = new Map(deliveries.map(function (m) { return [m.id, m.state]; }));
    var key = draftRecoveryKey(record, states);
    var clean = draftRecoveryKey(null, new Map());
    if (key === draftCommitted && !draftWrite) {
      draftUnsaved = missingBlob || !(textSaved || rich || !hasDraft || saveLegacyDraft(d, projection));
      return Promise.resolve(!draftUnsaved);
    }
    if (draftWrite) {
      if (key === draftWriteKey) return draftWrite;
      return draftWrite.then(saveDraft);
    }
    if (key === clean && draftCommitted === null && !draftWrite) {
      draftUnsaved = missingBlob || !textSaved && hasDraft;
      return Promise.resolve(!draftUnsaved);
    }
    draftUnsaved = true;
    if (!draftDB) return Promise.resolve(false);
    var before = draftDeliveryState;
    draftDeliveryState = states;
    var write = draftTransaction(true, function (existing) {
      var merged = Object.assign({}, existing && existing.deliveries);
      before.forEach(function (_, id) { if (!states.has(id)) delete merged[id]; });
      deliveries.forEach(function (m) { if (m.state !== before.get(m.id)) merged[m.id] = m; });
      return record || Object.keys(merged).length ? { active: record, deliveries: merged } : null;
    }).then(function () {
      if (draftWrite !== write) return true;
      draftWrite = null;
      draftCommitted = key;
      draftUnsaved = missingBlob || !saveLegacyDraft(d, projection) && !rich && hasDraft;
      return !draftUnsaved;
    }, function () {
      if (draftWrite === write) {
        draftWrite = null;
        draftDeliveryState = before;
      }
      return false;
    });
    draftWrite = write;
    draftWriteKey = key;
    return write;
  }
  function queueDraft() {
    if (!draftStore || restoring || draftDetached) return;
    clearTimeout(draftTimer);
    draftTimer = setTimeout(saveDraft, 400);
  }
  function restoreDraftMarkers(nums) {
    if (!nums.length) return;
    var start = ctext.selectionStart, end = ctext.selectionEnd, length = ctext.value.length;
    ctext.setSelectionRange(length, length);
    insertMarkers(nums);
    ctext.setSelectionRange(start, end);
  }
  function startDraftRecovery() {
    if (!draftStore) return;
    var legacy = draftStore.load(rid);
    draftLegacy = draftProjection(legacy);
    if (grid) grid.inert = true;
    draftReady = draftDatabase().then(function (db) {
      draftDB = db;
      return db ? draftTransaction(false).catch(function () { draftDB = null; return null; }) : null;
    }).then(function (record) {
      var active = record && record.active;
      var useActive = active && (draftLegacy === active.base || draftLegacy === active.projection);
      restoreDraft(useActive ? active.draft : legacy, useActive ? active.images : []);
      if (active && !useActive && ctext) {
        // An older page used the legacy draft in the meantime. Keep its
        // current text and put the retained blobs back under new markers.
        var nums = active.images.map(function (p) {
          var image = loadDraftImage(p);
          image.n = nextNum();
          imgs.push(image);
          return image.n;
        });
        restoreDraftMarkers(nums);
        renderImgs();
      }
      var deliveries = ctext ? Object.values(record && record.deliveries || {}) : [];
      deliveries.forEach(function (m) { draftDeliveryState.set(m.id, m.state); });
      restoreDeliveries(deliveries);
      if (record) draftCommitted = draftRecoveryKey(active || null, draftDeliveryState);
    }).finally(function () {
      draftLoaded = true;
      if (grid) grid.inert = false;
      if (!draftDetached) saveDraft();
    });
  }
  function boardDraft(d) {
    if (d && typeof d.boardIncarnation === "string" && d.boardIncarnation !== boardIncarnation) {
      // Keep free text and image references, never reattach replaced IDs.
      var text = d.text || "", ranges = Array.isArray(d.capsules) ? d.capsules.slice() : [];
      ranges.filter(function (r) { return /^(answer|topic-note|mention)$/.test(r.kind); }).reverse().forEach(function (cut) {
        text = text.slice(0, cut.start) + text.slice(cut.end);
        ranges = ranges.filter(function (r) { return r !== cut && (r.end <= cut.start || r.start >= cut.end); }).map(function (r) {
          return r.start >= cut.end ? Object.assign({}, r, { start: r.start - (cut.end - cut.start), end: r.end - (cut.end - cut.start) }) : r;
        });
      });
      return Object.assign({}, d, { boardIncarnation: boardIncarnation, text: text, capsules: ranges, selections: [], answers: {}, chips: [], bmentions: [], closeRequests: [] });
    }
    return d;
  }
  function restoreDraft(saved, recoveredImages) {
    var d = boardDraft(saved);
    recoveredImages = recoveredImages || [];
    if (!d) return;
    restoring = true;
    try {
      var available = new Set(recoveredImages.map(function (p) { return p.n; }));
      var lost = (Array.isArray(d.lost) ? d.lost : []).filter(function (n) { return !available.has(n); });
      var named = "";
      // Keep bundles in existing text storage. Extract before stripping lost
      // markers so quoted source and memo markers are never touched.
      var split = splitSelectionText(typeof d.text === "string" ? d.text : "");
      d.text = split.text;
      d.mentions = selectionRanges(d.mentions, split.cuts);
      d.capsules = selectionRanges(d.capsules, split.cuts);
      if (inlineEditor() && Array.isArray(d.selections)) bselections = d.selections.filter(function (s) { return s && s.key && Array.isArray(s.mentions) && Array.isArray(s.images); });
      if (Array.isArray(d.sel)) {
        var caret = selectionRanges([{ start: d.sel[0], end: d.sel[1] }], split.cuts)[0];
        d.sel = caret ? [caret.start, caret.end] : [0, 0];
      }
      if (!bselections.length) bselections = split.groups.map(function (s) {
        s.images = s.images.filter(function (n) { return lost.indexOf(n) < 0; });
        return s;
      });
      bselections.forEach(function (s) { named += "\n" + s.images.map(marker).join(" "); });
      // Restore the editor before board controls reconcile their answers;
      // otherwise their upserts would consume the empty composer caret.
      if (ctext && typeof d.text === "string" && d.text && inlineEditor()) {
        ctext.value = WsRail.stripMarkers(d.text, [], lost).text;
        ctext.wsc.restore(window.WsCapsules.stripMarkers(d.text, d.capsules, lost).ranges);
      }
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
      if (ctext && typeof d.text === "string" && d.text && (!ctext.value || inlineEditor())) {
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
        recoveredImages.forEach(function (p) {
          if (markerNums(named)[p.n]) {
            imgs.push(loadDraftImage(p));
            imgSeq = Math.max(imgSeq, p.n);
          }
        });
        // A paste may still belong to an open selection editor. Reattach
        // those blobs to the composer when restoring the session.
        var loose = recoveredImages.filter(function (p) {
          return !imgs.some(function (image) { return image.n === p.n; });
        });
        if (loose.length) {
          loose.forEach(function (p) { imgs.push(loadDraftImage(p)); imgSeq = Math.max(imgSeq, p.n); });
          restoreDraftMarkers(loose.map(function (p) { return p.n; }));
        }
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
      return draftUnsaved;
    }));
  }
  if (framed) {
    window.wsSaveSessionDraft = saveDraft;
    window.wsPrepareSessionLeave = async function () {
      await draftReady;
      await saveDraft();
      return draftUnsaved || outboxInFlight();
    };
    window.wsFinishSessionLeave = function () {
      draftDetached = true;
      clearTimeout(draftTimer);
    };
    window.wsCanCacheSession = function () {
      saveDraft();
      // Tool runtimes cannot be frozen by ordinary page code.
      return !!rid && draftLoaded && !draftUnsaved && !outboxInFlight() &&
        !q('iframe:not(.doc-frame), .bcard, .fpane[data-max]').length && !visible(fpane) && !tkBusy &&
        (!window.WsTerm || (window.WsTerm.cacheSafe && window.WsTerm.cacheSafe())) && !Object.keys(supChats).length;
    };
  }

  // --- iframe shell -------------------------------------------------------------
  // Safe, loaded session pages are retained up to shell.js's code bound.
  // Unsafe/tool/non-session pages are replaced after their draft decision. The
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
  var shIndex = 0, shRestoring = null, shRestoreNav = null, shRestoreTimer = null;
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
  function shStay(leave) {
    leave.ent.el.inert = false;
    if (leave.focus && leave.focus.isConnected) leave.focus.focus({ preventScroll: true });
    shIndex = leave.from;
    if (shWant && shWant !== leave.ent) shDrop(shWant);
    // Back has already traversed history. Restore the original position.
    if (shHistoryIndex() !== leave.from) shRestoreTo(leave.from);
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
    if (!window.WsShell.cacheLimit || !ent || !ent.loaded || !ent.f.rid) return false;
    try { return !!(ent.el.contentWindow.wsCanCacheSession && ent.el.contentWindow.wsCanCacheSession()); } catch (e) { return false; }
  }
  function shDrop(ent, leaving) {
    clearTimeout(ent.headTimer); ent.headTimer = null;
    try {
      if (leaving && ent.el.contentWindow.wsFinishSessionLeave) ent.el.contentWindow.wsFinishSessionLeave();
      else if (ent.el.contentWindow.wsSaveSessionDraft) ent.el.contentWindow.wsSaveSessionDraft();
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
  function shAdd(el, f) {
    var ent = { el: el, f: f, head: null, title: "", pincol: false, dim: 0, y: 0, loaded: false };
    el.addEventListener("load", function () {
      if (shFrames.indexOf(ent) < 0) return;
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
    if (shLeaving) {
      // An in-progress decision belongs to its original destination. A
      // subsequent traversal or pick gets its own decision.
      if (f.path !== shLeaving.f.path || f.hash !== shLeaving.f.hash || push !== shLeaving.push ||
          shHistoryIndex() !== shLeaving.index) shLeaving.next = { href: href, push: push };
      else shLeaving.next = null;
      return;
    }
    // A destination that never became visible owns no reader edits. A rapid
    // pick supersedes it without parking an extra loading document.
    if (window.WsShell.cacheLimit && shOn && shWant && shWant !== shOn) shDrop(shWant);
    var ent = shOn || shWant;
    if (ent && ent.f.path === f.path) {
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
  async function shLeave(ent, f, push, from) {
    var leave = { ent: ent, f: f, push: push, from: from === undefined ? shIndex : from, index: shHistoryIndex() };
    shLeaving = leave;
    if (doc.activeElement === ent.el) {
      try { leave.focus = ent.el.contentDocument.activeElement; } catch (e) { /* unavailable */ }
    }
    // Freeze editing while a pending write settles or the user decides.
    ent.el.inert = true;
    var needsDecision = false;
    try {
      var source = ent.el.contentWindow;
      if (source.wsPrepareSessionLeave) needsDecision = await source.wsPrepareSessionLeave();
      else if (source.wsHoldsDraft) needsDecision = source.wsHoldsDraft();
    } catch (e) { needsDecision = true; }
    var accepted = !needsDecision;
    if (!leave.next && needsDecision) {
      accepted = await window.wsConfirm(wsT("js.draft.leave"), {
        accept: wsT("js.draft.leave.accept"), cancel: wsT("js.draft.leave.stay")
      });
    }
    if (shLeaving !== leave) return;
    shLeaving = null;
    if ((shOn || shWant) !== ent) return;
    if (leave.next) {
      // Preserve the history entry reached by a later Back/Forward.
      ent.el.inert = false;
      if (leave.next.push && shHistoryIndex() !== leave.from) {
        shRestoreNav = leave.next;
        shStay(leave);
      } else shOpen(leave.next.href, leave.next.push);
      return;
    }
    if (!accepted || shHistoryIndex() !== leave.index) { shStay(leave); return; }
    shDrop(ent, true);
    if (shOn === ent) shOn = null;
    if (shWant && shWant.f.path !== f.path) shDrop(shWant);
    shStart(f, push);
  }
  function shStart(f, push, from) {
    if (push && location.pathname + location.hash !== f.path + f.hash) shPush(f);
    if (!push) shIndex = shHistoryIndex();
    var cached = window.WsShell.cacheLimit && shFrames.filter(function (x) { return x.loaded && x.f.rid && x.f.path === f.path; })[0];
    if (cached) {
      if (cached.f.hash !== f.hash) try { cached.el.contentWindow.location.hash = f.hash; } catch (e) {}
      cached.f = f;
      shWant = cached;
      shShow(cached, from);
      return;
    }
    var el = doc.createElement("iframe");
    el.className = shOn ? "sframe" : "sframe on";
    el.title = doc.title;
    if (shWant) shDrop(shWant);
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
    // page and header together until the destination's chrome is ready,
    // but never strand navigation on an error/login page or broken script.
    if (!ent.loaded) return;
    if (!ent.head && !ent.headWaitDone) {
      if (!ent.headTimer) ent.headTimer = setTimeout(function () {
        ent.headTimer = null;
        ent.headWaitDone = true;
        if (shWant === ent) shShow(ent, from);
      }, 400);
      return;
    }
    clearTimeout(ent.headTimer); ent.headTimer = null;
    var was = shOn;
    if (was === ent || shLeaving && shLeaving.ent === was) return;
    // Async tool/draft callbacks can change eligibility during a cold load.
    // Keep the ready destination, but settle the draft decision before
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
    var side = m && typeof m.side === "number" && isFinite(m.side) && m.side > 0 ? m.side : 0;
    shTop.style.setProperty("--sh-stage-side", side ? side + "px" : "");
    // Headerless fallback clears stale session controls, not their box.
    if (m) shTop.style.minHeight = m.top + "px";
    railRenameNew();
  }
  // shLayout takes the shown frame's pinned column and its backdrop's dim.
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
    // The rail no longer sits under the header: docked it runs the full
    // height beside it (the framed page's placeholder stays sticky at 12px),
    // and overlaid it is fixed, so it never moves with the page.
    if (shBody) shBody.style.marginTop = "";
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
        case "key":
          if (!on) break;
          if (m.key === "rail") railKey();
          else if (m.key === "esc" && railShown()) railOpen(false);
          break;
        case "press": if (on) railPopClose(false); break;
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
      // The mirrored permission chip opens the same session menu.
      if (t.closest(".host.hbtn, .pmhead[role=button]") && shOn) { e.preventDefault(); try { shOn.el.focus(); } catch (x) {} shSend(shOn, "menu"); }
    });
    if (shTop) shTop.addEventListener("keydown", function (e) {
      var t = e.target;
      if ((e.key !== "Enter" && e.key !== " ") || !t || !t.classList || !(t.classList.contains("hbtn") || (t.classList.contains("pmhead") && t.getAttribute("role") === "button")) || !shOn) return;
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
    // the image viewer, a maximized card's scrim, the ticket map).
    function dim() {
      var a = window.WsOverview && window.WsOverview.active() ? 0.72 : 0;
      q("dialog[open]").forEach(function (d) {
        var modal = false;
        try { modal = d.matches(":modal"); } catch (x) { modal = false; }
        if (modal) a = Math.max(a, window.WsShell.alpha(getComputedStyle(d, "::backdrop").backgroundColor));
      });
      q(".tk-iwin-back:not([hidden]), .gwin-back:not([hidden]), .wbv:not([hidden]), .bc-scrim:not([hidden]), .d-tm-back:not([hidden])").forEach(function (el) {
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
        var topCS = top ? getComputedStyle(top) : null;
        head = top ? {
          html: h ? h.innerHTML : null, qs: qs ? inline(qs) : "",
          pad: (parseFloat(topCS.paddingRight) || 0) + Math.max(0, window.innerWidth - root.clientWidth),
          side: Math.round((parseFloat(topCS.marginRight) || 0) * 100) / 100,
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
    window.addEventListener("resize", queueHead);
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
    // A press in the page reaches the shell, whose rail popovers and row
    // menu close on one (capture: the page's own handlers may stop it).
    doc.addEventListener("pointerdown", function () { shPost("press"); }, true);
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
  startDraftRecovery();
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
  // through the shell's window-open handler. A current shell adds an
  // optional action (canary | stable-return) to statuses and phases of a
  // manual channel selection, and limited to a canary miss at the page cap.
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
    // Manual channel actions: each control appears only when this shell's
    // bridge has its member, so an older shell keeps the original row. The
    // canary pill shares the check row; Back to stable shows only once the
    // shell reports a packaged canary (a stable relaunch drops it).
    if (typeof api.checkCanary === "function") {
      var row = doc.createElement("div");
      row.className = "dupd-row";
      card.insertBefore(row, check);
      row.appendChild(check);
      var canary = doc.createElement("button");
      canary.type = "button";
      canary.id = "dupd-canary";
      canary.className = "dupd-canary";
      canary.textContent = wsT("js.update.canary");
      canary.title = wsT("js.update.canary.hint");
      row.appendChild(canary);
      canary.addEventListener("click", function () { api.checkCanary(); });
    }
    var stable = null, running = "";
    if (typeof api.backToStable === "function" && typeof api.channel === "function") {
      stable = item("dupd-stable", "↩", wsT("js.update.stable"));
      stable.hidden = true;
      stable.addEventListener("click", function () { api.backToStable(); });
      Promise.resolve().then(function () { return api.channel(); }).then(function (c) {
        if (!c || c.canary !== true) return;
        running = String(c.version || "");
        stable.querySelector(".gs").textContent = wsT("js.update.stable.hint", running);
        stable.hidden = false;
      }, function () {});
    }
    var restart = item("dupd-restart", "⏻", wsT("js.update.restart"));
    restart.hidden = true;
    check.addEventListener("click", function () { api.check(); });
    restart.addEventListener("click", function () { api.restart(); });
    function phaseLine(p) {
      check.querySelector(".gs").textContent = !p ? "" :
        p.phase === "none" ? wsT("js.update.state.none") :
        p.phase === "available" ? (p.action === "stable-return" ? wsT("js.update.stable.state", p.version) :
          p.action === "canary" ? wsT("js.update.canary.state", p.version) :
          wsT("js.update.state.available", p.version)) + (p.skipped && p.action !== "stable-return" ? " " + wsT("js.update.skipped") : "") :
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
      // action names a manual channel selection (canary | stable-return);
      // an older shell never sends it, so those statuses read as before.
      var back = s.action === "stable-return", canaryAct = s.action === "canary";
      if (s.state === "available" && back) {
        // An explicit downgrade: name the stable target and the running
        // canary before anything installs. No skip: it is a one-off choice.
        toast(wsT("js.update.stable.available", v, String(s.current || running || "")), { sticky: true, dismiss: close, actions:
          page ? pageAct : [{ label: wsT("js.update.stable.install", v), run: function () { api.download(); } }] });
      } else if (s.state === "available") {
        toast(wsT(canaryAct ? "js.update.canary.available" : "js.update.available", v) + (s.skipped ? " " + wsT("js.update.skipped") : ""), { sticky: true, dismiss: close, actions:
          (page ? pageAct : [{ label: wsT("js.update.install"), run: function () { api.download(); } }]).concat(
          [{ label: wsT("js.update.skip"), run: function () { api.skip(v); } }]) });
      } else if (s.state === "downloading") toast(wsT("js.update.downloading", v), { sticky: true });
      else if (s.state === "ready") {
        toast(wsT(back ? "js.update.stable.ready" : "js.update.ready", v), { sticky: true, dismiss: close, actions: [
          { label: wsT("js.update.restart"), run: function () { api.restart(); } }] });
      } else if (s.state === "current" && canaryAct) toast(wsT(s.limited ? "js.update.canary.limited" : "js.update.canary.none"));
      else if (s.state === "current" && back) toast(wsT("js.update.stable.current"));
      else if (s.state === "current") toast(wsT("js.update.current", v));
      else if (s.state === "error") toast(wsT(canaryAct ? "js.update.canary.failed" : back ? "js.update.stable.failed" : "js.update.failed"), { kind: "warn" });
      else if (s.state === "download-error") toast(wsT("js.update.downloadfailed", m), { kind: "warn", sticky: true, dismiss: close, actions: pageAct });
      else if (s.state === "apply-error") toast(wsT("js.update.applyfailed", m), { kind: "warn", sticky: true, dismiss: close, actions: pageAct });
    });
  }
  if (!shell) trackerStart();
  desktopUpdate();
  syncSpinnerPhases(doc);
  connect();
})();
