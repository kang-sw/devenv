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
    tlHeld[name] = { hash: hash, root: name === "board" && typeof html !== "string" ? boardInputFragment(html) : frag(name === "tree" ? treeInputHTML(html) : html) };
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
  // Immutable tree inputs keep each nested row independent of its children.
  // Slots are private transport comments; materialization removes every slot
  // before inserting server markup. Live, formatted DOM is never a baseline.
  var TREE_SLOT = /<!--ws-tree-row-(\d+)-->/g;
  function treeRowKey(n) {
    if (n.matches("div.agent")) {
      var d = n.querySelector(":scope > details[data-key]");
      var k = d && d.getAttribute("data-key");
      return k && k.indexOf("agent-") === 0 && k.length > 6 ? k : null;
    }
    return n.matches("details.peer[data-peer]") && n.getAttribute("data-peer") ? "peer-" + n.getAttribute("data-peer") : null;
  }
  function treeSlot(i) { return "<!--ws-tree-row-" + i + "-->"; }
  function treeSlots(html) { return Array.from(html.matchAll(TREE_SLOT), function (m) { return +m[1]; }); }
  function treeInputFull(html) {
    if (typeof html !== "string" || html.indexOf("<!--ws-tree-row-") >= 0) return null;
    var box = doc.createElement("div"), rows = Object.create(null), order = [], parents = [];
    box.innerHTML = html;
    var nodes = q("div.agent, details.peer[data-peer]", box);
    if (nodes.some(function (n, i) {
      var key = treeRowKey(n), p = n.parentElement && n.parentElement.closest("div.agent, details.peer[data-peer]");
      if (!key || rows[key]) return true;
      order.push(key); parents.push(p ? nodes.indexOf(p) : -1); rows[key] = {};
      return false;
    })) return null;
    nodes.slice().reverse().forEach(function (n) {
      var i = nodes.indexOf(n);
      rows[order[i]] = { html: n.outerHTML };
      n.replaceWith(doc.createComment("ws-tree-row-" + i));
    });
    return { shell: box.innerHTML, order: order, parents: parents, rows: rows };
  }
  function treeRowNode(html, key) {
    if (typeof html !== "string") return null;
    var box = doc.createElement("div"); box.innerHTML = html;
    var n = box.firstElementChild;
    return box.childNodes.length === 1 && n && treeRowKey(n) === key && !n.querySelector("div.agent, details.peer[data-peer]") ? n : null;
  }
  function keyedSame(a, b) { return a.length === b.length && a.every(function (v, i) { return v === b[i]; }); }
  function keyedInputPatch(base, ev, rowNode, validate) {
    if (!base || !ev || typeof ev.hash !== "string" || !Array.isArray(ev.order) || !keyedSame(base.order, ev.order) || !Array.isArray(ev.rows)) return null;
    var rows = Object.create(null), changed = Object.create(null);
    Object.keys(base.rows).forEach(function (key) { rows[key] = base.rows[key]; });
    if (ev.rows.some(function (r) {
      if (!r || typeof r.key !== "string" || !rows[r.key] || changed[r.key] || typeof r.hash !== "string") return true;
      var n = rowNode(r.html, r.key);
      if (!n || (validate && !validate(n, r.key))) return true;
      changed[r.key] = true; rows[r.key] = { html: n.outerHTML };
      return false;
    })) return null;
    var next = { shell: base.shell, order: base.order, rows: rows };
    if (base.parents) next.parents = base.parents;
    return next;
  }
  function keyedInputHTML(input, slot, recursive) {
    if (typeof input === "string") return input;
    function fill(html) {
      return html.replace(slot, function (_, i) {
        var row = input.rows[input.order[+i]].html;
        return recursive ? fill(row) : row;
      });
    }
    return fill(input.shell);
  }
  function treeInputPatch(base, ev) {
    return keyedInputPatch(base, ev, treeRowNode, function (n, key) {
      return keyedSame(treeSlots(n.outerHTML), treeSlots(base.rows[key].html));
    });
  }
  function treeInputHTML(input) { return keyedInputHTML(input, TREE_SLOT, true); }
  // Validate every replacement before touching the live tree. Moving retained
  // children into a changed parent's slots preserves their node identities,
  // disclosure state, spinner and focus, even across simultaneous row changes.
  function mergeTreeRows(root, previous, next) {
    if (!previous || !next || previous.shell !== next.shell || !keyedSame(previous.order, next.order) || !keyedSame(previous.parents, next.parents)) return null;
    var nodes = q("div.agent, details.peer[data-peer]", root), live = Object.create(null), updates = Object.create(null), inserted = [];
    if (nodes.length !== previous.order.length || nodes.some(function (n, i) {
      var key = treeRowKey(n), p = n.parentElement && n.parentElement.closest("div.agent, details.peer[data-peer]");
      if (key !== previous.order[i] || (p ? nodes.indexOf(p) : -1) !== previous.parents[i]) return true;
      live[key] = n; return false;
    })) return null;
    if (next.order.some(function (key) {
      if (previous.rows[key].html === next.rows[key].html) return false;
      var n = treeRowNode(next.rows[key].html, key);
      if (!n || !keyedSame(treeSlots(n.outerHTML), treeSlots(previous.rows[key].html))) return true;
      updates[key] = n; return false;
    })) return null;
    next.order.slice().reverse().forEach(function (key) {
      var n = updates[key];
      if (!n) return;
      var walk = doc.createTreeWalker(n, 128), slots = [], slot;
      while ((slot = walk.nextNode())) if (/^ws-tree-row-\d+$/.test(slot.data)) slots.push(slot);
      slots.forEach(function (s) { s.replaceWith(live[next.order[+s.data.slice(12)]]); });
      live[key].replaceWith(n); live[key] = n; inserted.push(n);
    });
    return inserted;
  }

  // Panel transport rows are the existing top-level cards, not the nested
  // agenda disclosures. Canonical inputs never include reader folds, formatted
  // times, drawer styles or scroll offsets from the live DOM.
  var PANEL_SLOT = /<!--ws-panel-row-(\d+)-->/g;
  function panelRowKey(n) { return n.matches("details.panel") && /^p-.+/.test(n.id) ? n.id : null; }
  function panelInputFull(html) {
    if (typeof html !== "string" || html.indexOf("<!--ws-panel-row-") >= 0) return null;
    var box = doc.createElement("div"), rows = Object.create(null), order = [];
    box.innerHTML = html;
    if (Array.from(box.childNodes).some(function (n) { return n.nodeType !== 1 && n.nodeType !== 8 && (n.nodeType !== 3 || n.textContent.trim()); })) return null;
    if (Array.from(box.children).some(function (n) {
      var key = panelRowKey(n);
      if (!key || rows[key] || n.querySelector(".panel")) return true;
      rows[key] = { html: n.outerHTML }; order.push(key);
      n.replaceWith(doc.createComment("ws-panel-row-" + (order.length - 1)));
      return false;
    })) return null;
    return { shell: box.innerHTML, order: order, rows: rows };
  }
  function panelRowNode(html, key) {
    if (typeof html !== "string" || html.indexOf("<!--ws-panel-row-") >= 0) return null;
    var box = doc.createElement("div"); box.innerHTML = html;
    var n = box.firstElementChild;
    return box.childNodes.length === 1 && n && panelRowKey(n) === key && !n.querySelector(".panel") ? n : null;
  }
  function panelInputPatch(base, ev) { return keyedInputPatch(base, ev, panelRowNode); }
  function panelInputHTML(input) { return keyedInputHTML(input, PANEL_SLOT); }
  function mergePanelRows(root, previous, next) {
    if (!previous || !next || previous.shell !== next.shell || !keyedSame(previous.order, next.order)) return null;
    var live = Object.create(null), updates = [], nodes = Array.from(root.children);
    if (nodes.length !== previous.order.length || nodes.some(function (n, i) {
      var key = panelRowKey(n);
      if (key !== previous.order[i]) return true;
      live[key] = n; return false;
    })) return null;
    if (next.order.some(function (key) {
      if (previous.rows[key].html === next.rows[key].html) return false;
      var n = panelRowNode(next.rows[key].html, key);
      if (!n) return true;
      updates.push(n); return false;
    })) return null;
    updates.forEach(function (n) { live[n.id].replaceWith(n); });
    return updates;
  }
  // Cards are themselves scrollers on wide screens. Preserve their offsets
  // after drawerSync lays out replacements, and remap a changed card's focused
  // keyed disclosure without touching an unchanged card's content/frame nodes.
  function panelReaderState(root) {
    var active = doc.activeElement, focus = null;
    if (active && root.contains(active)) {
      var card = active.closest("details.panel"), anchor = active.closest("[data-key]") || card, path = [], n = active;
      if (card && anchor) {
        while (n !== anchor) { path.unshift(Array.prototype.indexOf.call(n.parentElement.children, n)); n = n.parentElement; }
        focus = { old: active, card: card.id, key: anchor.getAttribute("data-key"), path: path, tag: active.tagName, href: active.getAttribute("href") };
      }
    }
    return { cards: Array.from(root.children).map(function (n) { return { key: n.id, top: n.scrollTop, left: n.scrollLeft }; }), focus: focus };
  }
  function restorePanelReader(root, state) {
    var cards = Array.from(root.children);
    state.cards.forEach(function (s) { var n = cards.find(function (c) { return c.id === s.key; }); if (n) { n.scrollTop = s.top; n.scrollLeft = s.left; } });
    var f = state.focus;
    if (!f || f.old.isConnected) return;
    var card = cards.find(function (c) { return c.id === f.card; }), n = card;
    if (card && f.key) n = q("[data-key]", card).find(function (d) { return d.getAttribute("data-key") === f.key; });
    f.path.forEach(function (i) { n = n && n.children[i]; });
    if (n && n.tagName === f.tag && n.getAttribute("href") === f.href) n.focus({ preventScroll: true });
  }

  var ASSETS_TRIED = "ws.dashboard.assetsreload";
  function connect(withoutBoard) {
    if (!window.WebSocket) return;
    var boardInput = null, railInput = null, treeInput = null, entryInput = null, panelInput = null;
    // The shell streams the rail alone; a framed session page streams its
    // session without the rail.
    var path = shell ? "/rail/events" : mode === "index" || worktree ? "/events" : "/s/" + rid + "/events";
    var st = openStream(path + "?h=" + encodeURIComponent(held(withoutBoard)) + (framed && mode !== "index" ? "&frame=1" : "") + (!shell && mode !== "index" ? "&topics=1" : "") + (shell || (!framed && mode !== "index") ? "&railrows=1" : "") + (!shell && !worktree && mode !== "index" ? "&treerows=1&entryrows=1&panelrows=1" : ""), {
      open: function () { connected(true); },
      // Reopen with the hashes held now, so a reconnect sends only what
      // changed since.
      drop: function () {
        connected(false);
        trackerLost();
        setTimeout(connect, 3000);
      },
      events: { assets: onAssets, region: onRegion, "board-topics": onBoardTopics, "rail-rows": onRailRows, "tree-rows": onTreeRows, "entry-rows": onEntryRows, "panel-rows": onPanelRows, tracker: onTracker, gone: onGone }
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
      if (ev.name === "rail" && WsRail) {
        railInput = WsRail.rowInputFull(ev.html, doc);
        if (railInput) ev = { name: "rail", hash: ev.hash, html: railInput };
      }
      if (ev.name === "tree") {
        treeInput = treeInputFull(ev.html);
        if (treeInput) ev = { name: "tree", hash: ev.hash, html: treeInput };
      }
      if (ev.name === "entries") {
        entryInput = entryInputFull(ev.html);
        if (entryInput) ev = { name: "entries", hash: ev.hash, html: entryInput };
      }
      if (ev.name === "panels") {
        panelInput = panelInputFull(ev.html);
        if (panelInput) ev = { name: "panels", hash: ev.hash, html: panelInput };
      }
      receiveRegion(ev);
    }
    function onPanelRows(ev) {
      var next = panelInputPatch(panelInput, ev);
      if (!next) {
        st.close(); connected(false);
        setTimeout(connect, 0);
        return;
      }
      panelInput = next;
      receiveRegion({ name: "panels", hash: ev.hash, html: next });
    }
    function onEntryRows(ev) {
      var next = entryInputPatch(entryInput, ev);
      if (!next) {
        st.close(); connected(false);
        setTimeout(connect, 0);
        return;
      }
      entryInput = next;
      receiveRegion({ name: "entries", hash: ev.hash, html: next });
    }
    function onTreeRows(ev) {
      var next = treeInputPatch(treeInput, ev);
      if (!next) {
        st.close(); connected(false);
        setTimeout(connect, 0);
        return;
      }
      treeInput = next;
      receiveRegion({ name: "tree", hash: ev.hash, html: next });
    }
    function onRailRows(ev) {
      var next = WsRail && WsRail.rowInputPatch(railInput, ev, doc);
      if (!next) {
        // Close before reopening: the new connection always sends a full rail.
        st.close(); connected(false);
        setTimeout(connect, 0);
        return;
      }
      railInput = next;
      receiveRegion({ name: "rail", hash: ev.hash, html: next });
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
        if (ev.name === "rail" || ev.name === "sessions") apScan(frag(ev.name === "rail" && WsRail ? WsRail.rowInputHTML(ev.html) : ev.html));
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
      // Only the active frame returns with the shell reload; inactive
      // frames would leave unreachable entries in the tab's storage.
      if (shell && shOn) {
        try { if (shOn.el.contentWindow.wsSaveBoardReload) shOn.el.contentWindow.wsSaveBoardReload(); } catch (e) {}
      }
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
  // answers picked on the board, an attached image, or a topic close request.
  function holdsDraft() { return !!((ctext && ctext.value) || cmdSel || boardAnswers() || imgs.length || topicNotes().length || closeRequests.length); }
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

