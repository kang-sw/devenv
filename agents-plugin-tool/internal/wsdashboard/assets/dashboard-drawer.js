  // --- drawer: the side panels pulled down from the top --------------------
  // A frame of registered entries over the page. Each entry is { id, title,
  // icon, count, badge, content }: functions read at every sync, content
  // returning the element the entry's card is or null when the entry is
  // absent, and badge (optional) a count for the handle icon or null. The
  // four panels are the first entries: views onto the one panels region,
  // read again after each swap, so a later entry registers here without
  // reworking the frame. The sheet shows every present entry's card at once
  // and the handle holds one icon per present entry. Open state is this
  // browser's (ws.dashboard.drawer*); height always fills the available room.
  //
  // The git handle (the git region, left of the handle) opens the same frame
  // on the git drawer's sheet (#gbody) instead of the panels: drMode is the
  // sheet the frame shows, "panels" or "git", and only the handle of the
  // shown sheet is hidden. The git sheet's open state is never kept.
  //
  // Every drawer fills from the header bottom to above the composer (or
  // the viewport bottom without one), including the tracker's store sheets.
  // While a sheet shows, the fold button (#drfold, built below) replaces its
  // opener at the top, at that handle's width (--drw, --ghw), and closes it.
  var drawer = $("drawer"), drclip = $("drclip"), drsheet = $("drsheet"), drbody = $("drbody");
  var drhandle = $("drhandle"), drheader = doc.querySelector(".dwrap > .top");
  var gbody = $("gbody"), gitRegion = region("git");
  var DR_OPEN = "ws.dashboard.drawer", DR_SEARCH = "ws.dashboard.drawer.search";
  var drRoomW = -1, drGhOff = -1, drEntries = [], drWant = false, drOpen = false, drHas = false, drTop = 0;
  var drMode = "panels", drShown = "", drfold = null, drFoldW = -1, drFoldGw = -1;
  var sbody = null, shandle = null;
  // thandle opens the ticket map (dashboard-ticketmap.js), left of the
  // search handle; thandleSync shows it while a ticket panel exists.
  var thandle = null;
  if (drawer) {
    sbody = doc.createElement("div"); sbody.id = "sbody"; sbody.className = "dr-search"; sbody.hidden = true;
    drsheet.appendChild(sbody);
    shandle = doc.createElement("button"); shandle.type = "button"; shandle.id = "shandle"; shandle.className = "dr-handle dr-search-handle";
    shandle.title = wsT("js.tk.qs.tip") + " (Ctrl+K)"; shandle.setAttribute("aria-label", shandle.title); shandle.setAttribute("aria-controls", "drsheet");
    shandle.appendChild(luIcon("search")); drhandle.parentNode.insertBefore(shandle, drhandle);
    thandle = doc.createElement("button"); thandle.type = "button"; thandle.id = "thandle"; thandle.className = "dr-handle d-tm-handle"; thandle.hidden = true;
    thandle.title = wsT("js.tmap.open"); thandle.setAttribute("aria-label", thandle.title); thandle.setAttribute("aria-controls", "tmback");
    thandle.setAttribute("aria-haspopup", "dialog"); thandle.setAttribute("aria-expanded", "false");
    thandle.appendChild(luIcon("network")); drhandle.parentNode.insertBefore(thandle, shandle);
  }
  // ghandle is the git handle now: a region swap replaces it.
  function ghandle() { return $("ghandle"); }
  var drBadge = false, drSwap = false; // the handle's dot: a section badge, or a panels change while closed
  function drawerTab(entry) { drEntries.push(entry); }
  // panelTab registers a panel of the panels region (#p-<id>) as an entry: its
  // title and count chip come from the panel's own title bar.
  function panelTab(id, badge) {
    function el() { var r = region("panels"); return r ? r.querySelector('[id="p-' + id + '"]') : null; }
    function head() { var p = el(); return p ? p.querySelector("summary") : null; }
    drawerTab({
      id: id,
      title: function () {
        var h = head();
        return h ? Array.prototype.filter.call(h.childNodes, function (n) { return n.nodeType === 3; }).map(function (n) { return n.nodeValue; }).join("").trim() : id;
      },
      count: function () { var h = head(), c = h && h.querySelector(".chip"); return c ? c.textContent.trim() : ""; },
      badge: badge,
      content: el
    });
  }
  // The todo chip reads "done/total" and the mailbox chip "N unread".
  panelTab("todo", function (count) {
    var m = /^(\d+)\/(\d+)$/.exec(count), n = m ? +m[2] - +m[1] : 0;
    return n > 0 ? { n: n, title: wsTn("js.drawer.todo", n) } : null;
  });
  panelTab("agenda");
  panelTab("tickets");
  panelTab("mailbox", function (count) {
    var m = /^\D*(\d+)/.exec(count), n = m ? +m[1] : 0;
    return n > 0 ? { n: n, title: wsT("js.drawer.unread", n) } : null;
  });

  // cardTab registers one of the drawer's own cards (#p-<id>, drcards):
  // no region swap touches them.
  function cardTab(id) {
    drawerTab({
      id: id,
      title: function () {
        var h = $("p-" + id), sm = h && h.querySelector("summary");
        return sm ? Array.prototype.filter.call(sm.childNodes, function (n) { return n.nodeType === 3; }).map(function (n) { return n.nodeValue; }).join("").trim() : id;
      },
      count: function () { return ""; },
      content: function () { return $("p-" + id); }
    });
  }
  cardTab("worktrees");
  cardTab("holders");
  cardTab("notes");
  // The worktrees and holders cards' nodes (wtLoad, hdLoad, below): set
  // here, ahead of the drawer's first drApply, which a drawer left open
  // calls on load.
  var wtlist = $("wtlist"), wtcount = $("wtcount"), wtnote = $("wtnote"), wtclean = $("wtclean"), wtSeq = 0;
  var hdlist = $("hdlist"), hdcount = $("hdcount"), hdnote = $("hdnote"), hdSeq = 0;

  function drPresent() { return drEntries.filter(function (t) { return !!t.content(); }); }
  // drawerSync re-reads the entries: each card kept open, the handle's
  // tooltip (the section titles, with what is new in each) and its dot, and
  // whether there is a drawer at all. A sync never changes the open state or
  // the scroll.
  function drawerSync() {
    if (!drawer) return;
    var present = drPresent(), tips = [];
    drBadge = false;
    present.forEach(function (t) {
      var el = t.content();
      if (el.tagName === "DETAILS" && !el.open) el.open = true;
      var bd = t.badge && t.badge(t.count());
      if (bd) drBadge = true;
      tips.push(bd ? t.title() + ": " + bd.title : t.title());
    });
    drhandle.setAttribute("data-tip", tips.join(", "));
    drDot();
    drTitle();
    var has = present.length > 0;
    if (has !== drHas) {
      drHas = has;
      drhandle.hidden = !has;
      if (drMode === "panels") {
        drOpen = has && drWant;
        drApply(false);
      }
    }
    ticketIdeaSync();
    thandleSync();
    drFill(true);
  }
  // The ticket map's handle follows the same #p-tickets gate as the idea
  // section. A map left open over a panels swap that drops the panel stays
  // usable until it closes.
  function thandleSync() {
    if (!thandle) return;
    var on = !!$("p-tickets");
    if (thandle.hidden === on) { thandle.hidden = !on; drRoom(); }
  }
  // Ideas belong to this page's demand state, not the live panels region.
  // Reattach after a panels swap without fetching again or replacing focus
  // in an unchanged card. A failed read leaves a retry control.
  var ideaRows = null, ideaLoading = false, ideaError = false;
  function ticketIdeaSync() {
    var p = $("p-tickets"), lanes = p && p.querySelector(".lanes");
    if (!lanes || lanes.querySelector(".d-ticket-ideas")) return;
    var section = doc.createElement("section"); section.className = "lane d-ticket-ideas";
    var h = doc.createElement("h4"); h.textContent = wsT("js.drawer.idea");
    if (ideaRows !== null) {
      var count = doc.createElement("span"); count.className = "chip"; count.textContent = String(ideaRows.length); h.appendChild(count);
    }
    section.appendChild(h);
    if (ideaRows !== null) {
      var list = doc.createElement("ul"); list.className = "kcards";
      ideaRows.forEach(function (t) {
        var row = doc.createElement("li"); row.className = "kcard";
        var title = doc.createElement("span"); title.className = "ctitle";
        var cat = doc.createElement("span"); cat.className = "chip cat"; cat.textContent = t.category;
        var link = doc.createElement("a"); link.textContent = t.title || t.stem;
        link.href = "/s/" + rid + "/file?path=" + encodeURIComponent(t.path); link.title = t.path; link.setAttribute("data-wl", "file"); link.setAttribute("data-rel", t.path);
        if (t.abs) link.setAttribute("data-abs", t.abs);
        title.appendChild(cat); title.appendChild(link); row.appendChild(title);
        var stem = doc.createElement("span"); stem.className = "csub"; stem.textContent = t.stem; stem.title = t.stem; row.appendChild(stem);
        list.appendChild(row);
      });
      if (!ideaRows.length) { var empty = doc.createElement("li"); empty.className = "none"; empty.textContent = wsT("js.drawer.idea.empty"); list.appendChild(empty); }
      section.appendChild(list);
    } else {
      var button = doc.createElement("button"); button.type = "button"; button.className = "btn d-ticket-more";
      button.disabled = ideaLoading;
      button.textContent = wsT(ideaLoading ? "js.drawer.idea.loading" : ideaError ? "js.drawer.idea.retry" : "js.drawer.idea.more");
      button.addEventListener("click", ticketIdeaLoad); section.appendChild(button);
    }
    lanes.appendChild(section);
  }
  function ticketIdeaLoad() {
    if (ideaLoading || ideaRows !== null) return;
    ideaLoading = true; ideaError = false;
    ticketIdeaRedraw();
    fetch("/s/" + rid + "/tickets/idea", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("idea tickets");
      return r.json();
    }).then(function (data) {
      ideaRows = data.tickets || []; ideaLoading = false; ticketIdeaRedraw();
    }).catch(function () { ideaLoading = false; ideaError = true; ticketIdeaRedraw(); });
  }
  function ticketIdeaRedraw() {
    var p = $("p-tickets"), old = p && p.querySelector(".d-ticket-ideas");
    if (old) old.remove();
    ticketIdeaSync();
  }
  // Notes are fetched only on a panels opening or an explicit refresh. A wait
  // joins the server's existing worker rather than starting another read.
  var notesSeq = 0;
  function notesLoad(refresh) {
    var list = $("noteslist"), button = $("notesrefresh");
    if (!list) return;
    var seq = ++notesSeq, url = "/s/" + rid + "/notes";
    if (button) button.disabled = true;
    function request(wait) {
      return fetch(url + (wait ? "?wait=1" : ""), { method: refresh && !wait ? "POST" : "GET", credentials: "same-origin", cache: "no-store" }).then(function (r) {
        if (!r.ok) throw new Error("notes");
        return r.json();
      }).then(function (data) {
        if (seq !== notesSeq) return;
        notesRender(data);
        if (data.loading) return request(true);
        if (button) button.disabled = false;
      });
    }
    request(false).catch(function () {
      if (seq !== notesSeq) return;
      if (button) button.disabled = false;
      notesRender({ layers: [], error: true });
    });
  }
  function notesRender(data) {
    var list = $("noteslist"); if (!list) return;
    list.textContent = "";
    function line(text, cls, parent) { var el = doc.createElement("div"); el.className = cls; el.textContent = text; (parent || list).appendChild(el); return el; }
    if (data.local_only) line(wsT("js.notes.local"), "notes-hint");
    if (data.error) line(wsT("js.notes.error"), "notes-hint");
    if (data.loading) line(wsT("js.notes.loading"), "notes-hint");
    var names = { machine: wsT("js.notes.machine"), clone: wsT("js.notes.clone"), repo: wsT("js.notes.repo"), worktree: wsT("js.notes.worktree") };
    (data.layers || []).forEach(function (layer) {
      var section = doc.createElement("section"); section.className = "notes-layer"; list.appendChild(section);
      var rows = layer.notes || [];
      line((names[layer.layer] || layer.layer) + " · " + rows.length, "notes-heading", section);
      if (layer.error) line(wsT("js.notes.error"), "notes-hint", section);
      else if (!rows.length) line(wsT("js.notes.empty"), "notes-hint", section);
      rows.forEach(function (note) {
        var row = doc.createElement("div"); row.className = "notes-row" + (note.visible === false ? " notes-dim" : ""); section.appendChild(row);
        line(String(note.priority), "notes-priority" + (note.priority === 0 ? " zero" : ""), row);
        var content = doc.createElement("div"); content.className = "notes-content"; row.appendChild(content);
        var head = doc.createElement("div"); head.className = "notes-row-head"; content.appendChild(head);
        line(note.key, "notes-key", head);
        if (note.visible === false) line(wsT("js.notes.muted"), "notes-muted", head);
        var at = Date.parse(note.written_at);
        if (isFinite(at)) { var age = line(glRel(at), "notes-age", head); age.title = new Date(at).toLocaleString(wsI18n.lang); }
        var value = doc.createElement("button"); value.type = "button"; value.className = "notes-value"; value.textContent = note.value; value.setAttribute("aria-expanded", "false");
        value.addEventListener("click", function () { var expanded = value.getAttribute("aria-expanded") !== "true"; value.setAttribute("aria-expanded", String(expanded)); });
        content.appendChild(value);
      });
    });
  }
  var notesRefresh = $("notesrefresh");
  if (notesRefresh) notesRefresh.addEventListener("click", function (e) { e.preventDefault(); notesLoad(true); });
  // drFill stretches cards into the empty cells of the panels grid's last
  // row: the card directly above each empty cell spans two rows. Cards keep
  // their DOM order (grid-auto-flow: dense would reorder them); a spanning
  // card sits in the full row above the last, right of every last-row card,
  // so auto-placement puts the last row's cards in the same cells as before.
  // The column count is the computed track list (auto-fit's collapsed tracks
  // read 0px). Spans clear first, and only while the body is a grid: the
  // narrow layout (a flex column) and the git sheet (body hidden) get none.
  // A card already placed or spanned by a stylesheet skips the fill.
  var drFilled = [], drFillFrame = 0, drFillRoom = false;
  function drFill(room) {
    if (room === true) drFillRoom = true;
    if (drFillFrame) return;
    drFilled.forEach(function (el) { el.style.gridRow = ""; });
    drFilled = [];
    drFillFrame = requestAnimationFrame(function () {
      drFillFrame = 0;
      var fill = [], cs = drbody && getComputedStyle(drbody);
      if (drbody && !drbody.hidden && cs.display === "grid") {
        var n = cs.gridTemplateColumns.split(/\s+/).filter(function (w) { return parseFloat(w) > 0; }).length;
        var cards = [];
        (function items(el) {
          Array.prototype.forEach.call(el.children, function (c) {
            var d = getComputedStyle(c).display;
            if (d === "contents") items(c);
            else if (d !== "none") cards.push(c);
          });
        })(drbody);
        var k = cards.length, rem = n > 0 ? k % n : 0;
        if (n >= 2 && k > n && rem && !cards.some(function (c) {
          var s = getComputedStyle(c);
          return [s.gridRowStart, s.gridRowEnd, s.gridColumnStart, s.gridColumnEnd].some(function (v) { return v !== "auto"; });
        })) {
          var above = (Math.floor(k / n) - 1) * n;
          for (var c = rem; c < n; c++) fill.push(cards[above + c]);
        }
      }
      if (drFillRoom) { drFillRoom = false; drRoom(); }
      fill.forEach(function (el) { el.style.gridRow = "span 2"; });
      drFilled = fill;
    });
  }
  function drDot() {
    var d = drhandle.querySelector(".tdot");
    if (d) d.classList.toggle("on", drBadge || drSwap);
  }
  // drRoom keeps the header and the tracker's handle row clear of the
  // handles: --drhw is their width together (the header's padding and the
  // tracker row read it), --shr, --thr and --ghr the search, ticket map and
  // git handles' offsets from the right edge (the handles right of each and
  // their 6px gaps, 0 when nothing is right of it or it is absent).
  function drRoom() {
    var w = drHas ? Math.ceil(drhandle.getBoundingClientRect().width) : 0;
    var gw = ghandle() ? Math.ceil(gitRegion.getBoundingClientRect().width) : 0;
    var sw = shandle ? Math.ceil(shandle.getBoundingClientRect().width) : 0;
    var tw = thandle && !thandle.hidden ? Math.ceil(thandle.getBoundingClientRect().width) : 0;
    var so = w && sw ? w + 6 : 0, afterS = sw ? so + sw : w;
    var to = tw && afterS ? afterS + 6 : 0, beforeGit = tw ? to + tw : afterS;
    var off = beforeGit && gw ? beforeGit + 6 : 0, all = gw ? off + gw : beforeGit;
    doc.documentElement.style.setProperty("--shr", so + "px");
    doc.documentElement.style.setProperty("--thr", to + "px");
    if (all !== drRoomW || off !== drGhOff) {
      drRoomW = all;
      drGhOff = off;
      doc.documentElement.style.setProperty("--drhw", all + "px");
      doc.documentElement.style.setProperty("--ghr", off + "px");
    }
    if (drheader) drheader.classList.toggle("dr-room", all > 0);
    if (w !== drFoldW || gw !== drFoldGw) {
      drFoldW = w;
      drFoldGw = gw;
      doc.documentElement.style.setProperty("--drw", w + "px");
      doc.documentElement.style.setProperty("--ghw", gw + "px");
    }
  }
  // drFoldShow shows the fold button in mode's handle slot, or hides it
  // (mode "").
  function drFoldShow(mode) {
    if (!drfold) return;
    drfold.hidden = !mode;
    drfold.classList.toggle("git", mode === "git");
    drfold.classList.toggle("search", mode === "search");
  }
  function drTitle() {
    var t = wsT(drOpen && drMode === "panels" ? "js.drawer.close" : "js.drawer.open"), tip = drhandle.getAttribute("data-tip");
    if (tip) t += " (" + tip + ")";
    if (drhandle.classList.contains("wait")) t += "; " + wsT("js.drawer.waiting");
    drhandle.title = t;
    drhandle.setAttribute("aria-label", t);
    // The fold button announces the wait it shows (in the panels slot only).
    if (!drfold) return;
    var f = wsT("js.drawer.fold");
    if (drMode === "panels" && drfold.classList.contains("wait")) f += "; " + wsT("js.drawer.waiting");
    drfold.title = f;
    drfold.setAttribute("aria-label", f);
  }
  // drawerWait keeps the opener's approval signal current even while hidden;
  // the fold button mirrors it, since it shows in place of the hidden panels
  // handle while the sheet covers the waiting rows. The tree region's hidden
  // compact roster (.ag-source) does not count: it repeats this tree's
  // running rows and adds linked delegate sessions' agents, whose approvals
  // are not this session's to wait on.
  function drawerWait() {
    if (!drhandle) return;
    var wait = q("#r-convlive .permwait, #r-tree .phase.permission, #r-tree .brow.perm").some(function (el) { return !el.closest(".ag-source"); });
    drhandle.classList.toggle("wait", wait);
    if (drfold) drfold.classList.toggle("wait", wait);
    drTitle();
  }
  // A panels change the drawer is closed over lights the handle's dot.
  function drawerSwapped() {
    if (drhandle && !drOpen && drHas) { drSwap = true; drDot(); }
  }
  function drHeaderBottom() {
    return drheader ? Math.max(0, Math.round(drheader.getBoundingClientRect().bottom)) : 0;
  }
  // One maximum for every sheet. Retain the composer's existing clearance;
  // without a visible composer the sheet reaches the viewport bottom.
  function drMax() {
    var top = drHeaderBottom(), m = window.innerHeight - top, c = $("composer");
    if (visible(c)) m = Math.min(m, Math.round(c.getBoundingClientRect().top) - top - drhandle.offsetHeight - 6);
    return Math.max(0, m);
  }
  // drApply lays the drawer out for its state; anim animates a click's
  // toggle only.
  function drApply(anim) {
    if (!drawer) return;
    drTop = drHeaderBottom();
    var h = drMax(), git = drMode === "git", search = drMode === "search";
    drawer.style.setProperty("--drtop", drTop + "px");
    drawer.classList.toggle("anim", !!anim);
    drawer.classList.toggle("open", drOpen);
    ["panels", "git", "search"].forEach(function (mode) { drawer.classList.toggle(mode, drMode === mode); });
    drbody.hidden = git || search;
    if (gbody) gbody.hidden = !git;
    if (sbody) sbody.hidden = !search;
    drsheet.setAttribute("aria-label", wsT(search ? "js.tk.qs.tip" : "js.drawer.open"));
    drsheet.style.height = h + "px";
    drclip.style.height = (drOpen ? h : 0) + "px";
    if (shandle) shandle.setAttribute("aria-expanded", String(drOpen && search));
    drhandle.setAttribute("aria-expanded", String(drOpen && !git && !search));
    drFoldShow(drOpen ? drMode : "");
    ghSync();
    drTitle();
    drFill();
    // Each opening of a sheet reads its data again: the worktree pool and
    // the session holders for the panels, the changed files for the git
    // sheet.
    var shown = drOpen ? drMode : "";
    if (shown !== drShown) {
      drShown = shown;
      if (shown === "panels") { wtLoad(); hdLoad(); notesLoad(false); }
      else if (shown === "git") gitOpened();
      if (tkApi) { if (shown === "search") tkApi.openSearch(); else tkApi.closeSearch(); }
    }
  }
  // ghSync marks the git handle open or closed; a swap draws it closed.
  function ghSync() {
    var g = ghandle(), on = drOpen && drMode === "git";
    if (!g) return;
    g.setAttribute("aria-expanded", String(on));
    g.classList.toggle("open", on);
  }
  // gitSwapped follows a git region swap: the new handle's open mark and
  // width, and a closed git sheet when the handle went away.
  function gitSwapped() {
    if (!drawer) return;
    ghSync();
    drRoom();
    if (drOpen && drMode === "git" && !ghandle()) drSetOpen(false);
  }
  // The session drawer and a tracker store's drawer never show at once:
  // opening this one tells the tracker (which collapses its open store), and
  // the tracker opening a store closes this one (tk:drawer-open, below).
  function drAnnounce() { doc.dispatchEvent(new CustomEvent("ws:drawer-open")); }
  // drSetOpen opens the frame on mode's sheet (the shown one when omitted),
  // or closes it. Only the panels sheet's open state is kept.
  function drSetOpen(open, mode) {
    mode = mode || drMode;
    if (open && !(mode === "git" ? ghandle() : mode === "search" ? shandle : drHas)) return;
    var opener = mode === "search" && shandle || mode === "git" && ghandle() || drhandle, kb = open && doc.activeElement === opener;
    if (open && !drOpen) drAnnounce();
    if (!open && drsheet.contains(doc.activeElement)) (drMode === "search" && shandle || drMode === "git" && ghandle() || drhandle).focus();
    drMode = mode;
    drOpen = open;
    drWant = open && mode === "panels";
    save(DR_OPEN, drWant ? "1" : null);
    save(DR_SEARCH, open && mode === "search" ? "1" : null);
    if (drWant) { drSwap = false; drDot(); }
    drApply(true);
    // The focused opener hides while its sheet shows: focus moves to the fold
    // button in its slot instead of falling to the body, only while it is
    // still on that opener (opening search focuses its input in drApply).
    if (kb && drfold && doc.activeElement === opener) drfold.focus();
  }
  // drFold is a click on the fold button: the shown sheet closes as a click
  // on its handle closes it, and keyboard focus goes to that handle, since
  // the button hides.
  function drFold() {
    if (!drOpen) return;
    var back = drMode === "search" && shandle || drMode === "git" && ghandle() || drhandle, kb = doc.activeElement === drfold;
    drSetOpen(false);
    if (kb) back.focus();
  }
  if (drawer) {
    // The fold button replaces the active sheet's opener.
    drfold = doc.createElement("button");
    drfold.type = "button";
    drfold.className = "dr-fold";
    drfold.id = "drfold";
    drfold.hidden = true;
    drfold.title = wsT("js.drawer.fold");
    drfold.setAttribute("aria-label", drfold.title);
    drfold.setAttribute("aria-controls", "drsheet");
    var drFoldWait = doc.createElement("span");
    drFoldWait.className = "dr-wait";
    drFoldWait.title = wsT("js.drawer.waiting");
    drFoldWait.appendChild(gIcon("lock"));
    drfold.appendChild(drFoldWait);
    drfold.appendChild(luIcon("chevron-up"));
    drhandle.parentNode.insertBefore(drfold, drhandle);
    drfold.addEventListener("click", drFold);
    drWant = load(DR_OPEN) === "1";
    drawerSync();
    drawerWait();
    drhandle.addEventListener("click", function () {
      drSetOpen(!(drOpen && drMode === "panels"), "panels");
    });
    shandle.addEventListener("click", function () {
      drSetOpen(!(drOpen && drMode === "search"), "search");
    });
    thandle.addEventListener("click", function () { ticketMap.toggle(); });
    // The git handle toggles the git sheet, or brings it in place of the
    // panels; it is looked up per click, since a swap replaces it.
    doc.addEventListener("click", function (e) {
      if (!e.target.closest || !e.target.closest("#ghandle")) return;
      drSetOpen(!(drOpen && drMode === "git"), "git");
    });
    // A card's title bar is not a toggle: every card stays open.
    drbody.addEventListener("click", function (e) {
      if (e.target.closest && e.target.closest("details.panel > summary")) e.preventDefault();
    });
    // A store drawer opened: this one closes, and a saved open state not
    // applied yet (no panel present) is dropped, so it never opens over it.
    doc.addEventListener("tk:drawer-open", function () {
      if (drOpen) drSetOpen(false);
      else drWant = false;
    });
    // Escape closes the drawer after any open menu (link menu, gear menu)
    // has had it, and never while the composer has focus: there Escape
    // starts the composer's Escape-then-Tab way out. Capture runs this
    // before the menus' own listeners close them.
    doc.addEventListener("keydown", function (e) {
      if (e.key !== "Escape" || !drOpen || e.defaultPrevented) return;
      if (lmenu || (gmenu && !gmenu.hidden)) return;
      if (ctext && (doc.activeElement === ctext || ctext.wsc && doc.activeElement === ctext.wsc.editor)) return;
      if (drMode === "search" && sbody.contains(doc.activeElement)) return;
      drSetOpen(false);
    }, true);
    // A click outside the drawer closes it; the link menu a drawer link
    // opens counts as inside.
    doc.addEventListener("click", function (e) {
      var t = e.target;
      if (!drOpen || !t || !t.closest) return;
      if (t.closest("#drclip, #drhandle, #shandle, #thandle, #drfold, #r-git, .lmenu, .gwin-back")) return;
      drSetOpen(false);
    }, true);
    // The header can wrap or change height (a chip appears): the sheet
    // follows its bottom edge, and composer growth keeps the clearance.
    window.addEventListener("resize", function () { drApply(false); });
    if (window.ResizeObserver) {
      var drGeometry = new ResizeObserver(function () { drApply(false); });
      if (drheader) drGeometry.observe(drheader);
      if ($("composer")) drGeometry.observe($("composer"));
    }
    // The handles' widths follow their contents; the header pads clear of
    // them. Padding changes the shallower header's observed content box:
    // write it next frame, not during this native resize delivery.
    if (window.ResizeObserver) {
      var drRoomFrame = 0;
      var drRo = new ResizeObserver(function () {
        if (drRoomFrame) return;
        drRoomFrame = requestAnimationFrame(function () {
          drRoomFrame = 0;
          if (drawer.isConnected) drRoom();
        });
      });
      drRo.observe(drhandle);
      if (thandle) drRo.observe(thandle);
      if (gitRegion) drRo.observe(gitRegion);
    }
    // The panels grid's column count follows the body's width: the last
    // row's fill is laid out again (drFill).
    if (window.ResizeObserver) new ResizeObserver(drFill).observe(drbody);
    // The board-first page scrolls the document, and the header with it: the
    // sheet follows the header's bottom (the live layout never scrolls, so
    // this never fires there). Once a frame, and only when the bottom moved,
    // so a click's animation is not cut short.
    var drScrollFrame = 0;
    window.addEventListener("scroll", function () {
      if (drScrollFrame || !(drHas || ghandle())) return;
      drScrollFrame = requestAnimationFrame(function () {
        drScrollFrame = 0;
        if (drHeaderBottom() !== drTop) drApply(false);
      });
    }, { passive: true });
    drOpen = drHas && drWant;
    drApply(false);
  }

