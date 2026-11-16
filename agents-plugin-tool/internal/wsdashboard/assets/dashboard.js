(function () {
  // The dashboard page script, shared by / and /s/<reply-id>. The server
  // renders every region; this script only swaps a region when an SSE event
  // names it (or, for the conversation entries, merges it by entry name),
  // loads older conversation entries when scrolled to the top, keeps the
  // browser-side view state across swaps (open details, scroll offsets,
  // board answers, layout and tab), and formats times. It never builds
  // region HTML itself.
  var doc = document, root = doc.documentElement;
  function q(sel, el) { return Array.prototype.slice.call((el || doc).querySelectorAll(sel)); }
  function $(id) { return doc.getElementById(id); }
  var mode = root.getAttribute("data-mode");
  // svgEl's namespace, set ahead of the drawer, which draws its fold icon at
  // load (luIcon, below).
  var SVGNS = "http://www.w3.org/2000/svg";
  var rid = root.getAttribute("data-rid");
  var worktree = root.getAttribute("data-worktree") || "", cardIdentity = worktree ? "wt:" + worktree : rid;
  // Browser identity and transport authority are deliberately separate.
  function browsePost(route, body, done) { post("/s/host0/" + route, body, done); }
  // The iframe shell (shell.js, the shell block below): shell marks the
  // shell page, which holds the rail and the top header and no page's own
  // content; framed marks a page the shell frames (?frame=1), which leaves
  // the rail and its header's items to the shell; neither holds for a page
  // standing alone. A framed page opened outside a frame goes to its shell.
  // shHidden is a framed page the shell hides: like a hidden tab, it holds
  // its region updates and skips the clock until it shows.
  var shell = root.hasAttribute("data-shell"), framed = root.hasAttribute("data-frame") && !!window.WsShell, shHidden = false;
  if (framed && window.parent === window) { location.replace(location.pathname + location.hash); return; }
  function syncPageVisibility() {
    var paused = doc.hidden || shHidden;
    if (root.hasAttribute("data-animation-paused") === paused) return;
    root.toggleAttribute("data-animation-paused", paused);
    doc.dispatchEvent(new CustomEvent("ws-page-visibility"));
  }
  doc.addEventListener("visibilitychange", syncPageVisibility);
  syncPageVisibility();

  // --- browser storage: guarded, since a locked-down browser throws on access
  var store = null;
  try { store = window.localStorage; store.getItem("ws.dashboard.layout"); } catch (e) { store = null; }
  function load(k) { try { return store ? store.getItem(k) : null; } catch (e) { return null; } }
  function save(k, v) {
    try {
      if (!store) return false;
      if (v === null) store.removeItem(k); else store.setItem(k, v);
      return true;
    } catch (e) { return false; /* full or denied */ }
  }

  // --- times: the server writes epoch ms; the page shows local or relative time
  function two(n) { return (n < 10 ? "0" : "") + n; }
  function dur(ms) {
    var s = Math.max(0, Math.round(ms / 1000));
    if (s < 60) return s + "s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + "m " + (s % 60) + "s";
    return Math.floor(m / 60) + "h " + (m % 60) + "m";
  }
  // relAge is an age in one short unit: "now" under five seconds, then
  // seconds, minutes, hours, days (the session rail's last tool call).
  function relAge(ms) {
    var s = Math.max(0, Math.floor(ms / 1000));
    if (s < 5) return wsT("js.rel.now");
    if (s < 60) return wsT("js.rel.s", s);
    if (s < 3600) return wsT("js.rel.m", Math.floor(s / 60));
    if (s < 86400) return wsT("js.rel.h", Math.floor(s / 3600));
    return wsT("js.rel.d", Math.floor(s / 86400));
  }
  // A clock or weekday time and every time's title never change while the
  // page shows them, so each element formats them once (wsFmt marks it with
  // the language it used; only a language change formats it again). The
  // relative times ("since", "ago", "rel") follow the clock, and are
  // written only when their text changed.
  function fmtTime(t, now) {
    var ms = +t.getAttribute("data-ms");
    if (!ms) return;
    var f = t.getAttribute("data-f"), rel = f === "since" || f === "ago" || f === "rel" || f === "del";
    if (f === "user") {
      var at = t.getAttribute("datetime"), minutes = Math.max(0, Math.floor((now - ms) / 60000));
      // Absolute clocks and "today" use the envelope's sender offset, not
      // the viewer's zone. The full original date/time/offset stays on hover.
      var zone = at.slice(-6), offset = (+zone.slice(1, 3) * 60 + +zone.slice(4)) * (zone[0] === "-" ? -1 : 1);
      var today = new Date(now + offset * 60000).toISOString().slice(0, 10);
      var label = minutes < 60 ? wsT("js.user.minutes", minutes) : (at.slice(0, 10) === today ? "" : at.slice(5, 7) + "/" + at.slice(8, 10) + " ") + at.slice(11, 16);
      if (t.textContent !== label) t.textContent = label;
      t.title = at;
      return;
    }
    if (rel) {
      // "del" is a pending-deletion row's prune time, ahead of now; under
      // a day it takes the warning ink.
      var s = f === "since" ? dur(now - ms) : f === "rel" ? relAge(now - ms) : f === "del" ? (ms > now ? wsT("js.rail.deletes", relAge(ms - now)) : wsT("js.rail.deletes.due")) : wsT("js.ago", dur(now - ms));
      if (t.textContent !== s) t.textContent = s;
      if (f === "del") t.classList.toggle("soon", ms - now < 86400000);
    }
    if (t.wsFmt === wsI18n.lang) return;
    t.wsFmt = wsI18n.lang;
    if (f === "wd") t.textContent = new Date(ms).toLocaleDateString(wsI18n.lang, { weekday: "short" });
    else if (f === "dt") t.textContent = new Date(ms).toLocaleString(wsI18n.lang, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
    else if (!rel) { var d = new Date(ms); t.textContent = two(d.getHours()) + ":" + two(d.getMinutes()); }
    // data-tl names what the time is; the title leads with it.
    var tl = t.getAttribute("data-tl");
    t.title = (tl ? tl + " \u00b7 " : "") + new Date(ms).toLocaleString(wsI18n.lang);
  }
  // fmtTimes formats every time under el, for content just put on the page.
  function fmtTimes(el) {
    var now = Date.now();
    q("time[data-ms]", el).forEach(function (t) { fmtTime(t, now); });
    q("em.u-pace[data-reset]", el).forEach(function (e) { pace(e, now); });
  }
  // A usage limit's pace: its percent used against the straight line from
  // 0% at the window's start to 100% at its reset; negative is under pace,
  // nothing on the line.
  function pace(e, now) {
    var reset = +e.getAttribute("data-reset"), win = +e.getAttribute("data-win") * 1000, used = +e.getAttribute("data-pct");
    var text = "", cls = "u-pace";
    if (reset && win) {
      var elapsed = Math.max(0, Math.min(win, now - (reset - win)));
      var d = Math.trunc(used - elapsed / win * 100);
      if (d > 0) { text = " (+" + d + "%)"; cls += " pos"; }
      else if (d < 0) { text = " (\u2212" + -d + "%)"; cls += " neg"; }
    }
    if (e.textContent !== text) e.textContent = text;
    if (e.className !== cls) e.className = cls;
  }
  // The per-second tick visits only what follows the clock: the relative
  // times and the usage paces. A hidden tab skips it, and showing the tab
  // again catches up at once rather than on the next tick. The two sets are
  // kept (tickSets) and collected again only once the page changed: one
  // observer on the document drops them on any insertion or removal, and on
  // a change to an attribute they select on, wherever it happens (a board
  // swap, a conversation entry, a history load, a node this script builds),
  // except the tick's own text writes inside a time or a pace.
  var tickSets = null;
  function tickDirty(records) {
    if (records.some(function (r) {
      var n = r.target;
      return !(r.type === "childList" && (n.tagName === "TIME" || (n.tagName === "EM" && n.classList.contains("u-pace"))));
    })) tickSets = null;
  }
  if (window.MutationObserver) new MutationObserver(tickDirty).observe(doc, { childList: true, subtree: true, attributes: true, attributeFilter: ["data-ms", "data-f", "data-reset"] });
  function tickTimes() {
    if (doc.hidden || shHidden) return;
    var now = Date.now();
    if (!tickSets || !window.MutationObserver) tickSets = {
      times: q('time[data-ms][data-f="since"], time[data-ms][data-f="ago"], time[data-ms][data-f="rel"], time[data-ms][data-f="del"], time[data-ms][data-f="user"]'),
      paces: q("em.u-pace[data-reset]")
    };
    tickSets.times.forEach(function (t) { fmtTime(t, now); });
    tickSets.paces.forEach(function (e) { pace(e, now); });
  }
  setInterval(tickTimes, 1000);
  doc.addEventListener("visibilitychange", tickTimes);

  function region(name) { return $("r-" + name); }
  // The board frames' paced mount list and the viewport bands' state
  // (mountFramesPaced, frameNear: the lazy frames block), set here, before
  // the page load's first mount pass runs.
  var mountList = [], mountTask = false, NEAR = "100% 0px", FAR = "400% 0px", topicNear = {}, topicRO = null;
  // boardBoxes are the elements holding the board's topics: the board
  // region and, while the pinned column shows, the pin list there
  // (cards.js), whose topics are the board's as much as the region's.
  // boardQ queries both; inBoard tells whether a node is in either.
  function boardBoxes() {
    var b = region("board"), p = window.WsCards && window.WsCards.pinList ? window.WsCards.pinList() : null;
    return (b ? [b] : []).concat(p && p.isConnected && !(b && b.contains(p)) ? [p] : []);
  }
  function boardQ(sel) { return boardBoxes().reduce(function (a, b) { return a.concat(q(sel, b)); }, []); }
  function inBoard(n) { return !!n && boardBoxes().some(function (b) { return b.contains(n); }); }
  function boardRead(read) { var out = {}; boardBoxes().forEach(function (b) { var r = read(b); Object.keys(r).forEach(function (k) { out[k] = r[k]; }); }); return out; }
  function visible(el) { return !!el && el.getClientRects().length > 0; }

  // --- open details: data-key and panel state in memory, peers in storage ---
  // The toggle listener records what the user opened or closed; a swap only
  // re-applies it to the new elements.
  var openState = {};
  var PEER_KEY = "ws.dashboard.peer.";
  // A board panel the user folds by hand stays folded across a reload, a
  // session switch and a frame the shell evicted, none of which keep
  // openState. The folded panels are one capped LRU list in storage
  // (SHUT_KEY), each "<session>/<data-key>": the key names the entry, so an
  // overwrite (a new entry under the same panel id) is not matched and
  // opens by the server's default again. Opening the panel by hand removes
  // it; beyond SHUT_CAP the least recently folded is dropped.
  var SHUT_KEY = "ws.dashboard.panelshut", SHUT_CAP = 200, shutSet = null;
  function isPanel(d) { return d.classList.contains("entry-panel") && d.hasAttribute("data-key"); }
  function shutId(d) {
    var id = d.getAttribute("id");
    return rid + "/" + (/^p[1-9][0-9]*$/.test(id || "") ? "panel:" + id : d.getAttribute("data-key"));
  }
  function shutList() {
    try {
      var v = JSON.parse(load(SHUT_KEY) || "[]");
      return Array.isArray(v) ? v.filter(function (x) { return typeof x === "string"; }) : [];
    } catch (e) { return []; }
  }
  function shutHas(id) {
    if (!shutSet) { shutSet = {}; shutList().forEach(function (x) { shutSet[x] = true; }); }
    return !!shutSet[id];
  }
  function shutPut(id, on) {
    if (shutHas(id) === on && !on) return;
    var l = shutList().filter(function (x) { return x !== id; });
    if (on) l.push(id);
    save(SHUT_KEY, l.length ? JSON.stringify(l.slice(-SHUT_CAP)) : null);
    shutSet = null;
  }
  // Another tab or frame wrote the list; read it again.
  window.addEventListener("storage", function (e) { if (e.key === SHUT_KEY || e.key === null) shutSet = null; });
  function isPeer(d) { return d.classList.contains("peer") && d.hasAttribute("data-peer"); }
  function openKey(d) {
    // The rail's ended folds keep their state in storage (rail.js marks).
    if (d.classList.contains("rended")) return null;
    // Whole HISTORY is no longer a disclosure. Ignore only its obsolete
    // key; item/panel/version/system/run/topic folds keep their own owners.
    if ((d.getAttribute("data-key") || "").indexOf("history:") === 0) return null;
    // Latest panels carry pN as id; historical versions have only their
    // independent entry key. Republish changes eN, not this fold identity.
    if (d.classList.contains("entry-panel") && /^p[1-9][0-9]*$/.test(d.getAttribute("id") || "")) return "k:panel:" + d.getAttribute("id");
    if (d.hasAttribute("data-key")) return "k:" + d.getAttribute("data-key");
    if (d.classList.contains("panel") && d.id) return "p:" + d.id;
    return null;
  }
  doc.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d || d.tagName !== "DETAILS") return;
    if (isPeer(d)) { save(PEER_KEY + d.getAttribute("data-peer"), d.open ? "1" : null); return; }
    if (d.classList.contains("rended") && d.hasAttribute("data-key") && railSets) { railSets.ended.put(d.getAttribute("data-key"), d.open); return; }
    // A graft's temporary open attribute is not an actual reader open.
    // Shared panel policy accepts folds, but acknowledges updates only on
    // explicit summary/navigation intent. Historical folds keep this path.
    if (isPanel(d) && /^p[1-9][0-9]*$/.test(d.getAttribute("id") || "") && window.wbHistory && window.wbHistory.panelToggle && !window.wbHistory.panelToggle(d)) return;
    var k = openKey(d);
    if (k) openState[k] = d.open;
    if (isPanel(d)) shutPut(shutId(d), !d.open);
  }, true);
  function restoreOpenOne(d) {
    if (isPeer(d)) { d.open = load(PEER_KEY + d.getAttribute("data-peer")) === "1"; return; }
    if (d.classList.contains("entry-panel") && typeof window !== "undefined" && window.wbHistory && window.wbHistory.restorePanel && window.wbHistory.restorePanel(d)) return;
    var k = openKey(d);
    if (k && Object.prototype.hasOwnProperty.call(openState, k)) d.open = openState[k];
    else if (isPanel(d) && shutHas(shutId(d))) d.open = false;
  }
  function restoreOpen(el) {
    if (el.tagName === "DETAILS") restoreOpenOne(el);
    q("details", el).forEach(restoreOpenOne);
  }
  // forgetOpen drops what the user chose for the keyed details inside el,
  // so the next restoreOpen leaves the server's markup as it arrived.
  function forgetOpen(el) {
    q("details[data-key]", el).forEach(function (d) { delete openState[openKey(d)]; });
  }

  // --- scroll offsets -------------------------------------------------------
  // The panes that scroll on their own; the page itself scrolls on / and on
  // the board-first page. A pane hidden by the tabs view loses its offset, so
  // each pane's last visible offset is kept and put back when it shows again.
  // The drawer's body (drbody) is only clipped while the drawer is closed
  // (hidden only while the frame shows the git sheet), so a panels swap
  // keeps its offset whether the drawer is open or closed.
  var PANES = ["bcol", "bpins", "r-tree", "convo", "drbody"];
  // A scroll only marks its pane; the offset is read once per frame, in a
  // frame callback beside the board rail's. ResizeObserver caches whether
  // each pane is laid out, so scroll callbacks never probe its geometry.
  var paneScroll = {}, paneMoved = {}, paneVisible = {}, paneFrame = 0;
  var paneObserver = window.ResizeObserver ? new ResizeObserver(function (entries) {
    entries.forEach(function (entry) {
      var el = entry.target, shown = entry.contentRect.width > 0 && entry.contentRect.height > 0;
      paneVisible[el.id] = shown;
      if (shown) paneScroll[el.id] = el.scrollTop;
    });
  }) : null;
  function notePanes() {
    paneFrame = 0;
    PANES.forEach(function (id) {
      if (!paneMoved[id]) return;
      paneMoved[id] = false;
      var el = $(id);
      if (el && (paneObserver ? paneVisible[id] : visible(el))) paneScroll[id] = el.scrollTop;
    });
  }
  PANES.forEach(function (id) {
    var el = $(id);
    if (!el) return;
    if (paneObserver) paneObserver.observe(el);
    el.addEventListener("scroll", function () {
      paneMoved[id] = true;
      if (!paneFrame) paneFrame = requestAnimationFrame(notePanes);
    }, { passive: true });
  });
  function scrollState() {
    // The observer owns visibility. Snapshot offsets before swap writes,
    // without getClientRects/style probes or any reads of owned scrollers.
    var s = [];
    PANES.forEach(function (id) {
      var el = $(id);
      if (el && !el.wbScrollOwner && (paneObserver ? paneVisible[id] : visible(el))) s.push([el, el.scrollTop]);
    });
    if (doc.scrollingElement && !doc.scrollingElement.wbScrollOwner) s.push([doc.scrollingElement, doc.scrollingElement.scrollTop]);
    return s;
  }
  // Restore after the swap's writes; collect all current offsets before any
  // scroll write. Multiple messages share one read phase. Owner input wins.
  var scrollRestore = new Map(), scrollRestoreFrame = 0;
  function restoreScroll(s) {
    s.forEach(function (p) { if (!scrollRestore.has(p[0])) scrollRestore.set(p[0], p[1]); });
    if (scrollRestoreFrame) return;
    scrollRestoreFrame = requestAnimationFrame(function () {
      scrollRestoreFrame = 0;
      var kept = window.wbHistory && window.wbHistory.keeping, writes = [];
      scrollRestore.forEach(function (top, el) {
        if (el.isConnected !== false && !(el === convo && stick) && !el.wbScrollOwner && !(kept && kept(el)) && el.scrollTop !== top) writes.push([el, top]);
      });
      scrollRestore.clear();
      writes.forEach(function (p) { p[0].scrollTop = p[1]; });
    });
  }
  ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (type) {
    window.addEventListener(type, function () { scrollRestore.clear(); }, { capture: true, passive: true });
  });

  // The conversation stays at the bottom while the user has it there. Its
  // two regions (entries, convlive) scroll together inside #convo, which
  // owns the scroll and the pin.
  var convo = $("convo"), stick = true;
  function atBottom() { return convo.scrollTop + convo.clientHeight >= convo.scrollHeight - 24; }
  function pinned() { if (visible(convo)) stick = atBottom(); return stick; }
  function pinConvo() { if (stick && visible(convo)) convo.scrollTop = convo.scrollHeight; }
  // An accepted message starts a new reply, so sending re-arms bottom follow
  // even when the user had scrolled up. A later user scroll still cancels it.
  function followConvo() { stick = true; pinConvo(); }
  if (convo) {
    convo.addEventListener("scroll", function () { if (visible(convo)) stick = atBottom(); });
    // A resize (another pane grew, the window changed, the tab came back)
    // keeps a pinned conversation at the bottom.
    if (window.ResizeObserver) new ResizeObserver(pinConvo).observe(convo);
  }

  // --- layout and tabs (live or saved session conversation) ----------------
  // A saved-history page shares tab/scroll handling, not live controls.
  var grid = $("grid"), gmenu = $("gmenu"), live = !!$("tabbar");
  var LAYOUT_KEY = "ws.dashboard.layout", layoutMode = "auto";
  // TAB maps a region to the tab that shows it; PANE names the element whose
  // visibility tells whether a tab is shown in the current view. The panels
  // live in the drawer, whose handle carries their dot instead.
  // The conversation panel's header (chead) switches "Conversation |
  // Files" (ctabs): the Files pane (files.js) stands in for the
  // conversation below the header, which is only hidden meanwhile, so its
  // scroll and pin survive the switch; the composer, with the agents
  // popover, stays. The conversation tab counts as shown while either of the two
  // shows.
  var TAB = { entries: "convo", convlive: "convo", board: "board" };
  var PANE = { convo: "convo", board: "bcol" };
  var fpane = $("files"), chead = $("chead"), filesSeen = false, filesTab = null, filesSheet = null;
  // syncChead moves the conversation's counts from the entries region's
  // own title bar, which a live page hides, into the panel's header; it
  // runs on load and after each entries swap.
  function syncChead() {
    var slot = chead && chead.querySelector(".csum"), sum = doc.querySelector("#r-entries > .thead > .sum");
    if (!slot || !sum) return;
    slot.textContent = "";
    slot.appendChild(sum);
  }
  syncChead();
  function tabShown(t) { return visible($(PANE[t])) || (t === "convo" && visible(fpane)); }
  function tabDot(t) { var b = doc.querySelector('#tabbar [data-tab="' + t + '"]'); return b && b.querySelector(".tdot"); }
  function ctabDot() { return doc.querySelector('#ctabs [data-ctab="convo"] .tdot'); }
  function revealPanes() {
    PANES.forEach(function (id) {
      var el = $(id);
      var shown = visible(el);
      paneVisible[id] = shown;
      if (shown && Object.prototype.hasOwnProperty.call(paneScroll, id)) el.scrollTop = paneScroll[id];
    });
    pinConvo();
    nearTop(); // a conversation shown too short to scroll still loads older entries
    Object.keys(PANE).forEach(function (t) { var d = tabDot(t); if (d && tabShown(t)) d.classList.remove("on"); });
    if (visible(convo) && ctabDot()) ctabDot().classList.remove("on");
    // The Files pane hears each time it comes into view (a tab, a layout,
    // or a resize showed it) and each time it leaves.
    var fs = visible(fpane);
    if (fs !== filesSeen && filesTab) {
      if (fs) filesTab.shown();
      else filesTab.hidden();
    }
    filesSeen = fs;
  }
  function setLayout(v) {
    if (v !== "split" && v !== "tabs") v = "auto";
    layoutMode = v;
    if (grid) grid.setAttribute("data-layout", v);
    q(".gmenu [data-layout]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-layout") === v)); });
    if (live || worktree) revealPanes();
    if (cardBoard) window.WsCards.relayout();
  }
  // Input mode: which key sends. "enter" (the default) sends on Enter and
  // adds a line on Shift+Enter; "mod" sends on Ctrl/Cmd+Enter and Enter adds
  // a line. The placeholder tells which one is on.
  var INPUT_KEY = "ws.dashboard.input", inputMode = "enter";
  var HINT = { enter: wsT("js.hint.enter"), mod: wsT("js.hint.mod") };
  function setInput(v) {
    inputMode = v === "mod" ? "mod" : "enter";
    q(".gmenu [data-input]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-input") === inputMode)); });
    var t = $("ctext");
    if (t) { t.placeholder = HINT[inputMode]; growCtext(); }
  }
  function setTab(t) {
    grid.setAttribute("data-tab", t);
    q("#tabbar [data-tab]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-tab") === t)); });
    revealPanes();
  }
  // setCtab switches the conversation column between the conversation and
  // the Files pane; it is not kept across a reload.
  function setCtab(t) {
    if (t !== "files" || !fpane || !window.WsFiles) t = "convo";
    grid.setAttribute("data-ctab", t);
    q("#ctabs [data-ctab]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-ctab") === t)); });
    revealPanes();
  }
  if (live || worktree) {
    // "auto" crosses its breakpoint on a resize without a click.
    window.addEventListener("resize", revealPanes);
  }
  // Storage synchronizes other tabs. The shell also sends its in-memory
  // choice to frames, so denied/full storage never prevents immediate use.
  window.addEventListener("storage", function (e) {
    if (e.key === LAYOUT_KEY) setLayout(e.newValue);
    if (e.key === INPUT_KEY) setInput(e.newValue);
  });
  // Language: the choice lives in a cookie the server reads, so every page,
  // its stream, and its history render in it from the first byte. An empty
  // value deletes the cookie and follows the browser again. The page shows
  // the new language after a reload, which waits while a draft is held.
  var LANG_COOKIE = "ws_dashboard_lang";
  function markLang() {
    var pref = root.getAttribute("data-langpref") || "";
    var sel = $("glang");
    if (sel) sel.value = pref;
  }
  function setLang(v) {
    doc.cookie = LANG_COOKIE + "=" + encodeURIComponent(v) + "; Path=/; SameSite=Lax; Max-Age=" + (v ? 31536000 : 0);
    root.setAttribute("data-langpref", v);
    markLang();
    if (holdsDraft() || (shell && shFrames.some(shHolds))) {
      if (shell) toast(wsT("js.banner.lang"));
      else banner(wsT("js.banner.lang"));
    }
    else if (framed) window.parent.location.reload();
    else location.reload();
  }
  markLang();
  if ($("glang")) $("glang").addEventListener("change", function () { setLang(this.value); });

  // Theme: the choice lives in a cookie the server reads, so the root's
  // data-theme, and each topic-body frame's, is right from the first byte; no
  // cookie (auto) follows the OS. A change sets the attribute and the cookie,
  // posts the theme to the body frames (separate documents), and redraws the
  // drawn diagrams: the stylesheets do the rest, with no reload.
  var THEME_COOKIE = "ws_dashboard_theme";
  function markTheme() {
    var cur = root.getAttribute("data-theme") || "";
    q(".gmenu [data-theme-set]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-theme-set") === cur)); });
  }
  // setTheme picks the theme v; a framed page also hands it to the shell,
  // which passes it on to its other frames (quiet: the shell's own pass).
  function setTheme(v, quiet) {
    if (v !== "light" && v !== "dark") v = "";
    if (framed && !quiet) shPost("theme", { theme: v });
    if (shell && !quiet) shFrames.forEach(function (x) { shSend(x, "theme", { theme: v }); });
    doc.cookie = THEME_COOKIE + "=" + v + "; Path=/; SameSite=Lax; Max-Age=" + (v ? 31536000 : 0);
    if (v) root.setAttribute("data-theme", v);
    else root.removeAttribute("data-theme");
    markTheme();
    // The installed app's title bar: each theme-color meta takes the pinned
    // theme's colour, or its own scheme's under auto (pwa.go pwaHead).
    q('meta[name="theme-color"]').forEach(function (m) {
      var c = v ? doc.querySelector('meta[name="theme-color"][data-scheme="' + v + '"]') : m;
      if (c) m.setAttribute("content", c.getAttribute("data-c"));
    });
    q("iframe.doc-frame").forEach(postTheme);
    if (window.mermaid && q("pre.mermaid[data-processed]").length) window.wbMermaid();
  }
  function postDoc(f, message) {
    var key = f.getAttribute("data-doc-key");
    if (!key || key !== f.getAttribute("data-doc") || !f.contentWindow) return;
    message.docKey = key;
    f.contentWindow.postMessage(message, "*");
  }
  function postTheme(f) {
    postDoc(f, { wbDoc: "theme", theme: root.getAttribute("data-theme") || "" });
  }
  markTheme();

  // --- board answers --------------------------------------------------------
  // A board swap replaces the answer form; picks, notes, and checked
  // acknowledgements carry over by item number to the items still on the new
  // board, and the whiteboard form script rebuilds the reply line from them.
  function pickKey(b) { return (b.getAttribute("data-opt") || "") + "|" + (b.getAttribute("data-v") || ""); }
  function readAnswers(el) {
    var out = {};
    q(".card[data-id], .arow[data-id], .ckrow[data-id], .srow[data-id]", el).forEach(function (item) {
      var s = {}, note = item.querySelector("textarea.note"), box = item.querySelector("label.ack > input, label.ck > input");
      var pick = item.querySelector('.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]');
      if (pick) s.pick = pickKey(pick);
      if (note && note.value) s.note = note.value;
      if (note && note.wsc) s.capsules = note.wsc.snapshot();
      if (note && !note.hidden) s.noteOpen = true;
      if (box && box.checked) s.checked = true;
      if (item.hasAttribute("data-no")) s.no = true;
      if (s.pick || s.note || s.noteOpen || s.checked || s.no) out[item.getAttribute("data-id")] = s;
    });
    // A topic note carries over by topic name, keyed apart from item numbers.
    q(".tnote[data-topic]", el).forEach(function (note) {
      if (note.value || !note.hidden) {
        var saved = { note: note.value, noteOpen: !note.hidden };
        if (note.wsc) saved.capsules = note.wsc.snapshot();
        out["t:" + note.getAttribute("data-topic")] = saved;
      }
    });
    return out;
  }
  function openNote(note, s) {
    note.value = s.note || "";
    if (s.capsules && window.WsCapsules) window.WsCapsules.attach(note).restore(s.capsules);
    note.hidden = false;
    note.style.height = "auto";
    if (note.scrollHeight) note.style.height = note.scrollHeight + "px";
  }
  // setCheckNo presses or releases a check row's x, as the whiteboard form
  // script does: the row carries data-no while it stages "no".
  function setCheckNo(item, on) {
    var b = item.querySelector("button.no-toggle");
    if (!b) return;
    b.setAttribute("aria-pressed", on ? "true" : "false");
    if (on) item.setAttribute("data-no", ""); else item.removeAttribute("data-no");
  }
  function writeAnswers(el, saved) {
    q(".card[data-id], .arow[data-id], .ckrow[data-id], .srow[data-id]", el).forEach(function (item) {
      var s = saved[item.getAttribute("data-id")];
      if (!s) return;
      var note = item.querySelector("textarea.note"), box = item.querySelector("label.ack > input, label.ck > input");
      if (s.pick) q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) { if (pickKey(b) === s.pick) b.setAttribute("aria-pressed", "true"); });
      if (note && (s.note || s.noteOpen)) openNote(note, s);
      if (box && s.checked) box.checked = true;
      if (s.no) setCheckNo(item, true);
    });
    q(".tnote[data-topic]", el).forEach(function (note) {
      var s = saved["t:" + note.getAttribute("data-topic")];
      // A note a send closed (closeTopicNotes) keeps its text closed.
      if (s && s.noteOpen) openNote(note, s);
      else if (s) {
        note.value = s.note || "";
        if (s.capsules && window.WsCapsules) window.WsCapsules.attach(note).restore(s.capsules);
      }
    });
  }
  // A board swap waits while the user is typing in the answer form, and
  // lands when the focus leaves it.
  var deferred = null;
  function editing() {
    var a = doc.activeElement;
    return !!(a && inBoard(a) && (a.tagName === "INPUT" || a.tagName === "TEXTAREA" || a.isContentEditable));
  }
  doc.addEventListener("focusout", function () {
    if (!deferred) return;
    setTimeout(function () {
      if (!deferred || editing()) return;
      var d = deferred;
      deferred = null;
      swap("board", d.hash, d.html, true);
    }, 0);
  });

