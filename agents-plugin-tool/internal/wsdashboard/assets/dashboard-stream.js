  // --- stream ----------------------------------------------------------------
  // The stream state shows on every indicator in the top bar: the bare dot,
  // and the harness badge's LED, which a head region swap redraws lit, so
  // the swap reapplies the state (swap above).
  // --- tab label: the count of work waiting on the user ---------------------
  // This page owns document.title (the html element's data-wb-titlehost tells
  // the board's open-question pill script to report its count here instead
  // of writing the title). The label is "(N) <base>", no prefix at 0, where N
  // sums three sources, each cached where it changes so a recount is a sum:
  //   - the rendered tracker store's unread notifications (tlTk, from the
  //     renderer's onRender);
  //   - board asks: on a session page the pill's open set (tlPill), on the
  //     index the sessions' policy chips;
  //   - permission prompts: one per agent (or peer) in the tree that shows
  //     one, the conversation's wait folded into the lead; on the index the
  //     sessions' approval phases.
  // N drops only when an item is handled (read, answered, resolved), since
  // every source is recounted, never decremented by focus. A hidden tab holds
  // region updates back (connect), so a held update of a counted region is
  // parsed detached instead, once per new hash (tlHeld), and the counts read
  // it in place of the live region until the tab shows again.
  var TL_REGIONS = mode === "index" ? { sessions: 1 } : { board: 1, convlive: 1, tree: 1 };
  var TL_ASK = '.card.policy[id^="item-"], .arow[id^="item-"], .ckrow[data-id]:not([data-assumed])';
  var tlHeld = {}, tlPill = 0, tlTk = 0, tlAsks = -1, tlPerm = 0, tlN = -1, tlBase = null;
  function tlRetitle() {
    // The shell's title is its shown frame's (the title message).
    if (shell) return;
    var n = tlTk + tlPerm + (tlAsks >= 0 ? tlAsks : tlPill);
    if (n === tlN) return;
    tlN = n;
    if (tlBase === null) {
      tlBase = root.getAttribute("data-wb-title");
      if (tlBase === null) { tlBase = doc.title.replace(/^\(\d+\) /, ""); root.setAttribute("data-wb-title", tlBase); }
    }
    doc.title = (n ? "(" + n + ") " : "") + tlBase;
    shPost("title", { title: doc.title });
  }
  window.wbTitleCount = function (n) { tlPill = n; tlRetitle(); };
  // tlRebase replaces the label's base (a rename's new tab title, which a
  // head swap brings).
  function tlRebase(base) {
    if (shell || !base || base === tlBase) return;
    tlBase = base;
    root.setAttribute("data-wb-title", base);
    tlN = -1;
    tlRetitle();
  }
  function tlRoot(name) { return tlHeld[name] ? tlHeld[name].root : region(name); }
  // tlDrafted: the pill's reply rule (a pick, a ticked ack, or a note).
  function tlDrafted(el) {
    var box = el.querySelector("label.ack > input, label.ck > input"), n = el.querySelector("textarea.note");
    return !!(el.querySelector('.opt[data-opt][aria-pressed="true"]') || (box && box.checked) || el.hasAttribute("data-no") || (n && n.value.trim()));
  }
  // tlHeldAsks counts a held board the pill's way without its DOM: its asks,
  // less those drafted in the live board and those the newest sent records
  // (in the held conversation when one waits, else the live one) name.
  function tlHeldAsks() {
    var b = tlRoot("board"), live = region("board"), c = tlRoot("convlive"), drafted = {}, sent = {};
    if (!b) return 0;
    if (live) boardQ(TL_ASK).forEach(function (el) { if (tlDrafted(el)) drafted[el.getAttribute("data-id")] = 1; });
    if (c) q(".sentdata i[data-id]", c).forEach(function (i) { sent[i.getAttribute("data-id")] = 1; });
    return q(TL_ASK, b).filter(function (el) { var id = el.getAttribute("data-id"); return !drafted[id] && !sent[id]; }).length;
  }
  function tlPerms() {
    var t = tlRoot("tree"), c = tlRoot("convlive"), units = [];
    function add(u) { if (units.indexOf(u) < 0) units.push(u); }
    if (t) q(".phase.permission, .brow.perm", t).forEach(function (el) { add(el.closest(".agent, .peer") || "lead"); });
    if (c && c.querySelector(".permwait")) add((t && t.querySelector(".agent.root")) || "lead");
    return units.length;
  }
  // tlCount recounts the region-borne sources from the live regions, or the
  // held parse of one.
  function tlCount() {
    if (mode === "index") {
      var r = tlRoot("sessions");
      tlAsks = r ? q(".sess .chip.policy", r).reduce(function (a, el) { return a + (parseInt(el.textContent, 10) || 0); }, 0) : 0;
      tlPerm = r ? q(".sess .phase.permission", r).length : 0;
    } else {
      tlAsks = tlHeld.board || tlHeld.convlive ? tlHeldAsks() : -1;
      tlPerm = tlPerms();
    }
    tlRetitle();
  }
  // tlHold parses a held update of a counted region, once per new hash.
  function tlHold(name, hash, html) {
    if (!TL_REGIONS[name]) return;
    var was = tlHeld[name], el = region(name);
    if (was ? was.hash === hash : !el || el.getAttribute("data-h") === hash) return;
    tlHeld[name] = { hash: hash, root: name === "board" && typeof html !== "string" ? boardInputFragment(html) : frag(html) };
    tlCount();
  }
  // The tracker term and its new arrivals: each store's first loaded render
  // sets its baseline of unread keys silently; a later key not in it is new.
  // A new one toasts while the tab shows, and waits in tlHidden for one
  // summary toast while it is hidden (tlShown).
  var tlBaseKeys = {}, tlHidden = [], tlLastU = null;
  function tlTracker(u) {
    tlLastU = u;
    // a store no longer listed forgets its baseline and waiting arrivals;
    // listed again, it sets a new baseline silently
    Object.keys(tlBaseKeys).forEach(function (s) { if (tkOrder.indexOf(s) < 0) delete tlBaseKeys[s]; });
    tlHidden = tlHidden.filter(function (x) { return tkOrder.indexOf(x.store) >= 0; });
    tlTk = u && u.ready ? u.entries.length : 0;
    if (u && u.ready) {
      var base = tlBaseKeys[u.store];
      if (!base) {
        base = tlBaseKeys[u.store] = {};
        u.entries.forEach(function (x) { base[x.key] = 1; });
      } else {
        u.entries.forEach(function (x) {
          if (base[x.key]) return;
          base[x.key] = 1;
          if (doc.hidden) tlHidden.push(x);
          else if (!shHidden) tlNotice(x);
        });
      }
    }
    tlRetitle();
  }
  function tlNotice(x) {
    if (!window.WsToast) return;
    var text = x.label + " · " + x.title + (x.author ? " · " + x.author : "");
    window.WsToast.show(text, { kind: "notice", onClick: function () { if (tkApi) tkApi.openNotice(x); } });
  }
  function tlShown() {
    if (doc.hidden) return;
    var u = tlLastU, still = {};
    if (u && u.ready) u.entries.forEach(function (x) { still[x.store + "\n" + x.key] = 1; });
    var n = tlHidden.filter(function (x) { return still[x.store + "\n" + x.key]; }).length;
    tlHidden = [];
    if (!n || !window.WsToast) return;
    var store = u.store;
    window.WsToast.show(wsTn("js.toast.newnotes", n), { kind: "summary", onClick: function () { if (tkApi) tkApi.openNotify(store); } });
  }

  var sseOn = true;
  function connected(on) {
    sseOn = on;
    q(".dwrap > .top #sse").forEach(function (el) {
      el.classList.toggle("off", !on);
      el.title = wsT(on ? "js.sse.title.on" : "js.sse.title.off");
    });
  }
  // openStream opens a live stream (the daemon's sse.go and stream_ws.go)
  // over WebSocket: a browser counts WebSocket connections apart from the six
  // HTTP/1.1 connections it keeps per host, so open streams (one per tab, one
  // more for an open support chat) never hold up the page GETs, tracker
  // reads, and sends that share those six. Each frame is {event, data}, the
  // SSE stream's event name and data; a ping is answered with a pong, which
  // keeps the daemon from dropping the connection as dead. on.open runs when
  // the connection opens, on.events[name](data) per event, and on.drop once
  // when the connection fails or closes, never after the returned close().
  function openStream(path, on) {
    var ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + path), over = false;
    function drop() {
      if (over) return;
      over = true;
      try { ws.close(); } catch (x) {}
      if (on.drop) on.drop();
    }
    ws.onopen = function () { if (!over && on.open) on.open(); };
    ws.onerror = drop;
    ws.onclose = drop;
    ws.onmessage = function (e) {
      if (over) return;
      var m = null;
      try { m = JSON.parse(e.data); } catch (x) { return; }
      if (!m || typeof m.event !== "string") return;
      if (m.event === "ping") { try { ws.send('{"event":"pong"}'); } catch (x) {} return; }
      var fn = on.events && Object.prototype.hasOwnProperty.call(on.events, m.event) && on.events[m.event];
      if (typeof fn === "function") fn(m.data);
    };
    return { close: function () { over = true; try { ws.close(); } catch (x) {} } };
  }
  var ASSETS_TRIED = "ws.dashboard.assetsreload";
  function connect(withoutBoard) {
    if (!window.WebSocket) return;
    var boardInput = null;
    // The shell streams the rail alone; a framed session page streams its
    // session without the rail.
    var path = shell ? "/rail/events" : mode === "index" || worktree ? "/events" : "/s/" + rid + "/events";
    var st = openStream(path + "?h=" + encodeURIComponent(held(withoutBoard)) + (framed && mode !== "index" ? "&frame=1" : "") + (!shell && mode !== "index" ? "&topics=1" : ""), {
      open: function () { connected(true); },
      // Reopen with the hashes held now, so a reconnect sends only what
      // changed since.
      drop: function () {
        connected(false);
        trackerLost();
        setTimeout(connect, 3000);
      },
      events: { assets: onAssets, region: onRegion, "board-topics": onBoardTopics, tracker: onTracker, gone: onGone }
    });
    function onRegion(ev) {
      if (!ev) return;
      if (worktree && ev.name !== "rail") return;
      if (ev.name === "mode") {
        if (ev.html !== mode) modeChanged(ev.html);
        modeHash = ev.hash;
        liveMode = ev.html === "live";
        syncBadge();
        return;
      }
      if (ev.name === "board") {
        boardInput = boardInputFull(ev.html);
        if (boardInput) ev = { name: "board", hash: ev.hash, html: boardInput };
      }
      receiveRegion(ev);
    }
    function onBoardTopics(ev) {
      var next = boardInputPatch(boardInput, ev);
      if (!next) {
        // This connection's baseline cannot be trusted. Close it before
        // reopening without the board hash; other regions keep their hashes.
        st.close();
        connected(false);
        setTimeout(function () { connect(true); }, 0);
        return;
      }
      boardInput = next;
      receiveRegion({ name: "board", hash: ev.hash, html: next });
    }
    function receiveRegion(ev) {
      // A hidden tab keeps only each region's newest update and applies none;
      // showing the tab applies them (showRegions). The stream stays open, so
      // the light events (tracker) still arrive and act.
      if (pageHidden()) {
        delete hiddenRegions[ev.name];
        hiddenRegions[ev.name] = { hash: ev.hash, html: ev.html };
        tlHold(ev.name, ev.hash, ev.html);
        if (ev.name === "rail" || ev.name === "sessions") apScan(frag(ev.html));
        return;
      }
      swap(ev.name, ev.hash, ev.html);
    }
    // A frame the shell hides reads the tracker once it shows (shShown).
    function onTracker(ev) {
      if (!ev || typeof ev.rev !== "number") return;
      if (shHidden) shTrackerRev = ev.rev;
      else trackerNudge(ev.rev);
    }
    // Every (re)connect first names the page assets the daemon serves now:
    // other ones than this page runs (a replaced daemon) reload it, as
    // modeChanged does, or banner while a draft is held. A daemon that names
    // none, or a page rendered without one, never acts. A reload already
    // tried for the same assets (per tab) banners instead, so a page can
    // never reload in a loop.
    function onAssets(ev) {
      var mine = root.getAttribute("data-assets"), tried = null;
      if (!ev || !ev.id || !mine || ev.id === mine) return;
      try { tried = window.sessionStorage.getItem(ASSETS_TRIED); } catch (x) {}
      if (holdsDraft() || shDrafts() || tried === ev.id) { banner(wsT("js.banner.updated")); return; }
      try { window.sessionStorage.setItem(ASSETS_TRIED, ev.id); } catch (x) {}
      boardSaveReload();
      // A shell reload also replaces its same-origin session documents.
      if (shell) shFrames.forEach(function (x) {
        try { if (x.el.contentWindow.wsSaveBoardReload) x.el.contentWindow.wsSaveBoardReload(); } catch (e) {}
      });
      location.reload();
    }
    // A WebSocket handshake hides HTTP status. Recheck the stream once after
    // the usual retry delay before calling the session gone; never replace
    // the held DOM (or its drafts) with the probe's response. Transient probe
    // failures keep retrying at that cadence until absence is confirmed or
    // the stream can reopen with the hashes held then.
    function onGone() {
      st.close();
      connected(false);
      trackerLost();
      setTimeout(recheckGone, 3000);
    }
    function recheckGone() {
      fetch(path + (framed && mode !== "index" ? "?frame=1" : ""), { cache: "no-store" }).then(function (res) {
        // The GET is only a status probe, not another long-lived SSE stream.
        if (res.body) res.body.cancel().catch(function () {});
        if (res.status === 404) { banner(wsT("js.banner.gone")); return; }
        if (res.ok) { connect(); return; }
        setTimeout(recheckGone, 3000);
      }).catch(function () { setTimeout(recheckGone, 3000); });
    }
  }
  // showRegions applies, once each and in arrival order, the region updates
  // a hidden tab held back.
  function showRegions() {
    if (pageHidden()) return;
    var p = hiddenRegions;
    hiddenRegions = {};
    Object.keys(p).forEach(function (name) { swap(name, p[name].hash, p[name].html); });
    tlHeld = {};
    tlCount();
    tlShown();
  }
  doc.addEventListener("visibilitychange", showRegions);
  function banner(text) {
    var b = $("dash-banner");
    if (!b) return;
    b.textContent = text;
    b.hidden = false;
  }
  // holdsDraft reports unsent work a reload would drop: composer text,
  // answers picked on the board, or an attached image.
  function holdsDraft() { return !!((ctext && ctext.value) || cmdSel || boardAnswers() || imgs.length || topicNotes().length); }
  // The shell asks its frames before an assets reload, which reloads them
  // too (shDrafts).
  if (framed) window.wsHoldsDraft = holdsDraft;
  function modeChanged(next) {
    // The other mode has other regions, and a reload renders them; text typed
    // in the composer or answers picked on the board are never dropped for it.
    if (holdsDraft()) {
      banner(wsT(next === "live" ? "js.banner.appeared" : "js.banner.wentaway"));
      return;
    }
    location.reload();
  }

