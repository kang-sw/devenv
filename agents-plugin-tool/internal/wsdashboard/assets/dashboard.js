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
  var rid = root.getAttribute("data-rid");

  // --- browser storage: guarded, since a locked-down browser throws on access
  var store = null;
  try { store = window.localStorage; store.getItem("ws.dashboard.layout"); } catch (e) { store = null; }
  function load(k) { try { return store ? store.getItem(k) : null; } catch (e) { return null; } }
  function save(k, v) {
    try { if (store) { if (v === null) store.removeItem(k); else store.setItem(k, v); } } catch (e) { /* full or denied */ }
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
  function fmtTimes(el) {
    var now = Date.now();
    q("time[data-ms]", el).forEach(function (t) {
      var ms = +t.getAttribute("data-ms");
      if (!ms) return;
      var f = t.getAttribute("data-f");
      if (f === "since") t.textContent = dur(now - ms);
      else if (f === "ago") t.textContent = wsT("js.ago", dur(now - ms));
      else if (f === "wd") t.textContent = new Date(ms).toLocaleDateString(wsI18n.lang, { weekday: "short" });
      else { var d = new Date(ms); t.textContent = two(d.getHours()) + ":" + two(d.getMinutes()); }
      t.title = new Date(ms).toLocaleString(wsI18n.lang);
    });
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
  setInterval(function () { fmtTimes(doc); }, 1000);

  function region(name) { return $("r-" + name); }
  function visible(el) { return !!el && el.getClientRects().length > 0; }

  // --- open details: data-key and panel state in memory, peers in storage ---
  // The toggle listener records what the user opened or closed; a swap only
  // re-applies it to the new elements.
  var openState = {};
  var PEER_KEY = "ws.dashboard.peer.";
  function isPeer(d) { return d.classList.contains("peer") && d.hasAttribute("data-peer"); }
  function openKey(d) {
    if (d.hasAttribute("data-key")) return "k:" + d.getAttribute("data-key");
    if (d.classList.contains("panel") && d.id) return "p:" + d.id;
    return null;
  }
  doc.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d || d.tagName !== "DETAILS") return;
    if (isPeer(d)) { save(PEER_KEY + d.getAttribute("data-peer"), d.open ? "1" : null); return; }
    var k = openKey(d);
    if (k) openState[k] = d.open;
  }, true);
  function restoreOpenOne(d) {
    if (isPeer(d)) { d.open = load(PEER_KEY + d.getAttribute("data-peer")) === "1"; return; }
    var k = openKey(d);
    if (k && Object.prototype.hasOwnProperty.call(openState, k)) d.open = openState[k];
  }
  function restoreOpen(el) {
    if (el.tagName === "DETAILS") restoreOpenOne(el);
    q("details", el).forEach(restoreOpenOne);
  }

  // --- scroll offsets -------------------------------------------------------
  // The panes that scroll on their own; the page itself scrolls on / and on
  // the board-first page. A pane hidden by the tabs view loses its offset, so
  // each pane's last visible offset is kept and put back when it shows again.
  // The drawer's body (drbody) is only clipped while the drawer is closed
  // (hidden only while the frame shows the git sheet), so a panels swap
  // keeps its offset whether the drawer is open or closed.
  var PANES = ["bcol", "r-tree", "convo", "drbody"];
  var paneScroll = {};
  PANES.forEach(function (id) {
    var el = $(id);
    if (el) el.addEventListener("scroll", function () { if (visible(el)) paneScroll[id] = el.scrollTop; });
  });
  function scrollState() {
    var s = [];
    PANES.forEach(function (id) { var el = $(id); if (visible(el)) s.push([el, el.scrollTop]); });
    if (doc.scrollingElement) s.push([doc.scrollingElement, doc.scrollingElement.scrollTop]);
    return s;
  }
  function restoreScroll(s) { s.forEach(function (p) { if (p[0].scrollTop !== p[1]) p[0].scrollTop = p[1]; }); }

  // The conversation stays at the bottom while the user has it there. Its
  // two regions (entries, convlive) scroll together inside #convo, which
  // owns the scroll and the pin.
  var convo = $("convo"), stick = true;
  function atBottom() { return convo.scrollTop + convo.clientHeight >= convo.scrollHeight - 24; }
  function pinned() { if (visible(convo)) stick = atBottom(); return stick; }
  function pinConvo() { if (stick && visible(convo)) convo.scrollTop = convo.scrollHeight; }
  if (convo) {
    convo.addEventListener("scroll", function () { if (visible(convo)) stick = atBottom(); });
    // A resize (another pane grew, the window changed, the tab came back)
    // keeps a pinned conversation at the bottom.
    if (window.ResizeObserver) new ResizeObserver(pinConvo).observe(convo);
  }

  // --- layout and tabs (session page with a live adapter only) -------------
  var grid = $("grid"), gmenu = $("gmenu"), live = !!$("tabbar");
  var LAYOUT_KEY = "ws.dashboard.layout";
  // TAB maps a region to the tab that shows it; PANE names the element whose
  // visibility tells whether a tab is shown in the current view. The panels
  // live in the drawer, whose handle carries their dot instead.
  var TAB = { tree: "convo", entries: "convo", convlive: "convo", board: "board" };
  var PANE = { convo: "convo", board: "bcol" };
  function tabShown(t) { return visible($(PANE[t])); }
  function tabDot(t) { var b = doc.querySelector('#tabbar [data-tab="' + t + '"]'); return b && b.querySelector(".tdot"); }
  function revealPanes() {
    PANES.forEach(function (id) {
      var el = $(id);
      if (visible(el) && Object.prototype.hasOwnProperty.call(paneScroll, id)) el.scrollTop = paneScroll[id];
    });
    pinConvo();
    nearTop(); // a conversation shown too short to scroll still loads older entries
    Object.keys(PANE).forEach(function (t) { var d = tabDot(t); if (d && tabShown(t)) d.classList.remove("on"); });
  }
  function setLayout(v) {
    if (v !== "split" && v !== "tabs") v = "auto";
    grid.setAttribute("data-layout", v);
    q(".gmenu [data-layout]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-layout") === v)); });
    revealPanes();
  }
  // The tree and the conversation share their column at a fixed ratio; the
  // saved value is the tree's share. The grid reads it as two fr shares, so a
  // region swap (which only rewrites a pane's contents) never moves it.
  var SPLIT_KEY = "ws.dashboard.split", SPLIT_DEF = 0.25, SPLIT_MIN = 0.05, SPLIT_MAX = 0.95;
  var splitter = $("splitter"), splitShare = SPLIT_DEF;
  function clampShare(f) { return Math.min(SPLIT_MAX, Math.max(SPLIT_MIN, f)); }
  function setSplit(f) {
    splitShare = isFinite(f) ? clampShare(f) : SPLIT_DEF;
    // fr shares summing below 1 would leave the column short, so scale up
    grid.style.setProperty("--sa", (splitShare * 1000).toFixed(1) + "fr");
    grid.style.setProperty("--sb", ((1 - splitShare) * 1000).toFixed(1) + "fr");
  }
  function savedSplit() { var v = parseFloat(load(SPLIT_KEY)); return isFinite(v) ? v : SPLIT_DEF; }
  // shareAt turns a pointer's y into the tree's share of the room the two
  // panes divide (their span less the splitter), with the same pixel floors
  // the stylesheet sets.
  function shareAt(y) {
    var t = $("r-tree"), c = $("convo");
    if (!visible(t) || !visible(c) || !splitter) return null;
    var top = t.getBoundingClientRect().top, room = c.getBoundingClientRect().bottom - top - splitter.offsetHeight;
    if (room <= 0) return null;
    var f = (y - top - splitter.offsetHeight / 2) / room, lo = 56 / room, hi = 1 - 96 / room;
    if (lo < hi) f = Math.min(hi, Math.max(lo, f));
    return f;
  }
  if (splitter) {
    var dragging = false;
    splitter.addEventListener("pointerdown", function (e) {
      if (e.button !== undefined && e.button !== 0) return;
      dragging = true;
      splitter.classList.add("drag");
      try { splitter.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the move still lands */ }
      e.preventDefault();
    });
    splitter.addEventListener("pointermove", function (e) {
      if (!dragging) return;
      var f = shareAt(e.clientY);
      if (f !== null) setSplit(f);
    });
    function endDrag() {
      if (!dragging) return;
      dragging = false;
      splitter.classList.remove("drag");
      save(SPLIT_KEY, splitShare.toFixed(4));
    }
    splitter.addEventListener("pointerup", endDrag);
    splitter.addEventListener("pointercancel", endDrag);
    splitter.addEventListener("dblclick", function () { setSplit(SPLIT_DEF); save(SPLIT_KEY, null); });
    splitter.addEventListener("keydown", function (e) {
      var d = e.key === "ArrowUp" ? -0.03 : e.key === "ArrowDown" ? 0.03 : 0;
      if (!d) return;
      e.preventDefault();
      setSplit(splitShare + d);
      save(SPLIT_KEY, splitShare.toFixed(4));
    });
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
    if (t) t.placeholder = HINT[inputMode];
  }
  function setTab(t) {
    grid.setAttribute("data-tab", t);
    q("#tabbar [data-tab]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-tab") === t)); });
    revealPanes();
  }
  if (live) {
    // "auto" crosses its breakpoint on a resize without a click.
    window.addEventListener("resize", revealPanes);
  }
  // The settings are on every page (the index page's gear menu, a session
  // page's drawer card; the index and board-first pages hold only the theme
  // and language items), so the gear menu's Escape and the language setting
  // do not wait for a live layout. The item selectors read .gmenu, which
  // both carry.
  if (gmenu) doc.addEventListener("keydown", function (e) { if (e.key === "Escape" && !gmenu.hidden) gmenu.hidden = true; });
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
    if (holdsDraft()) banner(wsT("js.banner.lang"));
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
  function setTheme(v) {
    if (v !== "light" && v !== "dark") v = "";
    doc.cookie = THEME_COOKIE + "=" + v + "; Path=/; SameSite=Lax; Max-Age=" + (v ? 31536000 : 0);
    if (v) root.setAttribute("data-theme", v);
    else root.removeAttribute("data-theme");
    markTheme();
    q("iframe.doc-frame").forEach(postTheme);
    if (window.mermaid && q("pre.mermaid[data-processed]").length) window.wbMermaid();
  }
  function postTheme(f) {
    if (f.contentWindow) f.contentWindow.postMessage({ wbDoc: "theme", theme: root.getAttribute("data-theme") || "" }, "*");
  }
  markTheme();

  // --- board answers --------------------------------------------------------
  // A board swap replaces the answer form; picks, notes, and checked
  // acknowledgements carry over by item number to the items still on the new
  // board, and the whiteboard form script rebuilds the reply line from them.
  function pickKey(b) { return (b.getAttribute("data-opt") || "") + "|" + (b.getAttribute("data-v") || ""); }
  function readAnswers(el) {
    var out = {};
    q(".card[data-id], .arow[data-id]", el).forEach(function (item) {
      var s = {}, note = item.querySelector(".note"), box = item.querySelector(".ack input");
      var pick = item.querySelector('.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]');
      if (pick) s.pick = pickKey(pick);
      if (note && note.value) s.note = note.value;
      if (note && !note.hidden) s.noteOpen = true;
      if (box && box.checked) s.checked = true;
      if (s.pick || s.note || s.noteOpen || s.checked) out[item.getAttribute("data-id")] = s;
    });
    // A topic note carries over by topic name, keyed apart from item numbers.
    q(".tnote[data-topic]", el).forEach(function (note) {
      if (note.value || !note.hidden) out["t:" + note.getAttribute("data-topic")] = { note: note.value, noteOpen: !note.hidden };
    });
    return out;
  }
  function openNote(note, s) {
    note.value = s.note || "";
    note.hidden = false;
    note.style.height = "auto";
    if (note.scrollHeight) note.style.height = note.scrollHeight + "px";
  }
  function writeAnswers(el, saved) {
    q(".card[data-id], .arow[data-id]", el).forEach(function (item) {
      var s = saved[item.getAttribute("data-id")];
      if (!s) return;
      var note = item.querySelector(".note"), box = item.querySelector(".ack input");
      if (s.pick) q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) { if (pickKey(b) === s.pick) b.setAttribute("aria-pressed", "true"); });
      if (note && (s.note || s.noteOpen)) openNote(note, s);
      if (box && s.checked) box.checked = true;
    });
    q(".tnote[data-topic]", el).forEach(function (note) {
      var s = saved["t:" + note.getAttribute("data-topic")];
      if (s) openNote(note, s);
    });
  }
  // A board swap waits while the user is typing in the answer form, and
  // lands when the focus leaves it.
  var deferred = null;
  function editing() {
    var a = doc.activeElement, b = region("board");
    return !!(a && b && b.contains(a) && (a.tagName === "INPUT" || a.tagName === "TEXTAREA"));
  }
  doc.addEventListener("focusout", function () {
    if (!deferred) return;
    setTimeout(function () {
      if (!deferred || editing()) return;
      var d = deferred;
      deferred = null;
      swap("board", d.hash, d.html);
    }, 0);
  });

  // --- region swap -----------------------------------------------------------
  // The mode pseudo-region's hash is unknown to the page until the first
  // event carries it; an empty hash makes the server send it once.
  var modeHash = "";
  function held() {
    return q("[data-region]").map(function (el) {
      return el.getAttribute("data-region") + "." + el.getAttribute("data-h");
    }).concat(mode !== "index" ? ["mode." + modeHash] : []).join(",");
  }
  function swap(name, hash, html) {
    var el = region(name);
    if (!el) return;
    var board = name === "board";
    if (el.getAttribute("data-h") === hash) { if (board) deferred = null; return; }
    if (board && editing()) { deferred = { hash: hash, html: html }; return; }
    if (board) deferred = null;
    var pin = (name === "entries" || name === "convlive") && pinned();
    var scroll = scrollState();
    var answers = board ? readAnswers(el) : null;
    var frameH = board ? frameHeights(el) : null;
    // A finished reply's mermaid fences draw once merged; the streaming
    // reply (convlive) keeps them code blocks.
    if (name === "entries") { mergeEntriesRegion(el, html); mermaid(); }
    else {
      el.innerHTML = html;
      restoreOpen(el);
      fmtTimes(el);
    }
    el.setAttribute("data-h", hash);
    sweepOutImgs(el);
    if (board) {
      setFrameHeights(el, frameH);
      writeAnswers(el, answers);
      if (window.wbForm) window.wbForm();
      mermaid();
      syncChips();
      syncNote();
      refetchClosed(el);
    }
    if (board || name === "convlive") applySent();
    if (name === "panels") drawerSync();
    if (name === "git") gitSwapped();
    restoreScroll(scroll);
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
    if (name === "panels") drawerSwapped();
    if (name === "tree" || name === "convlive") drawerWait();
    if (name === "convlive") syncBadge();
    if (name === "head" && !sseOn) connected(false);
  }

  // --- conversation entries: merged by key, never re-rendered ----------------
  // Each entry element carries data-entry, its ring entry name; names sort in
  // time order. Only the entries container is searched: held bubbles in the
  // convlive region carry data-entry too (their inbox id) and are not
  // entries.
  function entriesBox() { var r = region("entries"); return r && r.querySelector(".centries"); }
  function frag(html) { var t = doc.createElement("template"); t.innerHTML = html; return t.content; }
  // mergeEntries inserts each node of src whose name box lacks at its sorted
  // place and returns how many it inserted; an entry already shown is kept
  // as it is.
  function mergeEntries(box, src) {
    var have = {}, added = 0;
    Array.prototype.forEach.call(box.children, function (c) { have[c.getAttribute("data-entry")] = true; });
    Array.prototype.slice.call(src.children).forEach(function (n) {
      var k = n.getAttribute("data-entry");
      if (!k || have[k]) return;
      // New entries are usually the newest (live) or the oldest (history),
      // so walk back from the end only past entries that sort after k.
      var c = box.lastElementChild;
      while (c && (c.getAttribute("data-entry") || "") > k) c = c.previousElementSibling;
      box.insertBefore(n, c ? c.nextSibling : box.firstChild);
      have[k] = true;
      added++;
      restoreOpen(n);
      fmtTimes(n);
    });
    return added;
  }
  // mergeEntriesRegion applies an entries region event: the title bar (its
  // counts) is replaced, and the entries are merged.
  function mergeEntriesRegion(el, html) {
    var src = frag(html), box = entriesBox(), head = el.querySelector(".thead"), nhead = src.querySelector(".thead");
    var nbox = src.querySelector(".centries");
    if (!box || !nbox) { el.innerHTML = html; restoreOpen(el); fmtTimes(el); return; }
    if (head && nhead) { head.parentNode.replaceChild(nhead, head); fmtTimes(nhead); }
    mergeEntries(box, nbox);
  }

  // --- older history: loaded a page at a time when scrolled to the top -------
  // The region draws the newest entries; older ones come from the history
  // route, are prepended with the scroll position kept, and are never
  // evicted. histBefore is the next page's cursor ("" asks for the newest
  // page, so a page that drew no entries can still start loading).
  var histBefore = null, histEnd = false, histBusy = false;
  function loadHistory() {
    if (histEnd || histBusy || !window.fetch || !convo || !visible(convo)) return;
    var box = entriesBox();
    if (!box) return;
    if (histBefore === null) {
      var first = box.firstElementChild;
      histBefore = first ? first.getAttribute("data-entry") || "" : "";
    }
    histBusy = true;
    var url = "/s/" + rid + "/history?n=50" + (histBefore ? "&before=" + encodeURIComponent(histBefore) : "");
    fetch(url, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      histBusy = false;
      var h = convo.scrollHeight, top = convo.scrollTop;
      mergeEntries(box, frag(res.html || ""));
      // Keep what the user was reading in place: the prepended height goes
      // above the scroll offset.
      convo.scrollTop = top + (convo.scrollHeight - h);
      mermaid();
      if (res.before) histBefore = res.before;
      if (res.end || !res.before) histEnd = true;
      nearTop();
    }, function () {
      // A failed fetch (daemon replaced, network) is retried on the next
      // scroll to the top, after a pause.
      setTimeout(function () { histBusy = false; }, 3000);
    });
  }
  // nearTop loads older entries when the conversation is scrolled to (or
  // too short to scroll away from) its top.
  function nearTop() { if (convo && visible(convo) && convo.scrollTop < 160) loadHistory(); }
  if (convo) convo.addEventListener("scroll", nearTop);

  // mermaid draws every undrawn mermaid block, loading the shared script
  // once; a call while it loads is covered by the draw its onload runs, and
  // a failed load (daemon restarting) is retried by the next call.
  var mermaidLoading = false;
  function mermaid() {
    if (!q("pre.mermaid:not([data-processed])").length || !window.wbMermaidFile) return;
    if (window.mermaid) { window.wbMermaid(); return; }
    if (mermaidLoading) return;
    mermaidLoading = true;
    var s = doc.createElement("script");
    s.src = window.wbMermaidFile; // relative: the server serves it beside the page
    s.onload = function () { window.wbMermaid(); };
    s.onerror = function () { mermaidLoading = false; s.parentNode.removeChild(s); };
    doc.head.appendChild(s);
  }

  // --- closed topics past the board's cap: fetched on request ---------------
  // The board renders the most recently closed topics up to its cap, then a
  // show-more control naming the next index (data-closed-next); a click
  // fetches the next page from the closed route into the control's place,
  // ending with the next control when more remain. closedUpTo is the index
  // past the last topic fetched, so a board swap, which renders the capped
  // board again, fetches the same topics back in one request.
  var closedUpTo = 0;
  function loadClosed(btn, n) {
    if (!window.fetch || btn.disabled) return;
    btn.disabled = true;
    var from = +btn.getAttribute("data-closed-next");
    fetch("/s/" + rid + "/closed?from=" + from + (n ? "&n=" + n : ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      var page = frag(res.html || "").firstElementChild;
      // A board swap meanwhile replaced the control, and refetches itself.
      if (!btn.parentNode || !page) { btn.disabled = false; return; }
      btn.parentNode.replaceChild(page, btn);
      closedUpTo = Math.max(closedUpTo, from + page.querySelectorAll(".topic").length);
      restoreOpen(page);
      if (window.wbBindBoard) window.wbBindBoard(page);
      if (window.wbAgo) window.wbAgo(page);
      mermaid();
    }, function () { btn.disabled = false; });
  }
  function refetchClosed(el) {
    var btn = el.querySelector("button.closed-more[data-closed-next]");
    var from = btn ? +btn.getAttribute("data-closed-next") : 0;
    if (btn && closedUpTo > from) loadClosed(btn, closedUpTo - from);
  }

  // --- composer: never inside a region, so a swap never touches it ---------
  // With a live host adapter the composer sends: the typed text plus one
  // answer chip per board item. Clicking an option, ok/no, or an
  // acknowledgement on a card attaches (or replaces) that item's chip; the
  // chips live here, so a chip outlives its card leaving the board.
  var ctext = $("ctext"), cnote = $("cnote"), cchips = $("cchips");
  var chips = {}, chipOrder = [];
  function chipList() { return chipOrder.map(function (id) { return chips[id]; }); }
  function putChip(c) { if (!chips[c.id]) chipOrder.push(c.id); chips[c.id] = c; }
  function dropChip(id) { delete chips[id]; chipOrder = chipOrder.filter(function (x) { return x !== id; }); }
  // Reply grammar, as the board's copy-answers line and the server's answer
  // block build it: "(nn) <answer>[, note: <text>]", a note alone as
  // "(nn) note: <text>", items answered ok without a note grouped last as
  // "(nn), (mm) ok", parts joined by "; ".
  function answerBlock(list) {
    var parts = [], acked = [];
    list.forEach(function (c) {
      var note = (c.note || "").trim(), s = "";
      if (!note && c.answer === "ok") { acked.push("(" + c.id + ")"); return; }
      if (c.answer) s = "(" + c.id + ") " + c.answer;
      if (note) s = s ? s + ", note: " + note : "(" + c.id + ") note: " + note;
      if (s) parts.push(s);
    });
    if (acked.length) parts.push(acked.join(", ") + " ok");
    return parts.join("; ");
  }
  function boardItem(id) {
    var b = region("board");
    return b ? b.querySelector('.card[data-id="' + id + '"], .arow[data-id="' + id + '"]') : null;
  }
  // readItem is a board item's answer as a chip; null when it has none.
  function readItem(item) {
    var pick = item.querySelector('.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]');
    var box = item.querySelector(".ack input"), note = item.querySelector(".note");
    var c = { id: item.getAttribute("data-id"), answer: "", note: note ? note.value.trim() : "" };
    if (pick) c.answer = pick.getAttribute("data-opt") || pick.getAttribute("data-v");
    else if (box && box.checked) c.answer = "ok";
    return c.answer || c.note ? c : null;
  }
  // setItem puts a chip's answer (or none) on the item's card and lets the
  // board's form script rebuild its reply line.
  function setItem(id, c) {
    var item = boardItem(id);
    if (!item) return;
    q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) {
      var v = b.getAttribute("data-opt") || b.getAttribute("data-v");
      b.setAttribute("aria-pressed", String(!!c && v === c.answer));
    });
    var box = item.querySelector(".ack input"), note = item.querySelector(".note");
    if (box) box.checked = !!c && c.answer === "ok";
    if (note) {
      note.value = c && c.note ? c.note : "";
      if (note.value) note.hidden = false;
    }
    var fire = note || box;
    if (fire) fire.dispatchEvent(new Event("change", { bubbles: true }));
  }
  // syncChips takes the answers of the items on the board into the chips.
  function syncChips() {
    var b = region("board");
    if (!b || !cchips) return;
    q(".card[data-id], .arow[data-id]", b).forEach(function (item) {
      var c = readItem(item), id = item.getAttribute("data-id");
      if (c) putChip(c); else if (chips[id]) dropChip(id);
    });
    renderChips();
  }
  function renderChips() {
    if (!cchips) return;
    cchips.textContent = "";
    chipList().forEach(function (c) {
      var el = doc.createElement("span"), b = doc.createElement("b"), x = doc.createElement("button");
      el.className = "achip";
      b.textContent = "(" + c.id + ")";
      el.appendChild(b);
      el.appendChild(doc.createTextNode(" " + (c.answer || "note")));
      if (c.note) {
        var n = doc.createElement("span");
        n.className = "anote";
        n.textContent = c.note;
        el.appendChild(n);
      }
      x.className = "unchip";
      x.setAttribute("data-unchip", c.id);
      x.title = wsT("js.chip.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
  }
  function boardAnswers() {
    if (cchips) return answerBlock(chipList());
    var b = region("board"), r = b && b.querySelector("#reply-text");
    return r ? r.value.trim() : "";
  }
  function syncNote(text) {
    if (!cnote) return;
    cnote.textContent = text || "";
  }
  function grow() { ctext.style.height = "auto"; ctext.style.height = Math.min(160, ctext.scrollHeight) + "px"; }
  // post sends a write route's JSON body; done(ok, answer) gets the parsed
  // answer, or the server's refusal text.
  function post(route, body, done) {
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch("/s/" + rid + "/" + route, {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        if (!r.ok) { done(false, s.trim() || "HTTP " + r.status); return; }
        var v = {};
        try { v = JSON.parse(s); } catch (e) { v = {}; }
        done(true, v);
      });
    }, function () { done(false, wsT("js.post.unreachable")); });
  }
  // --- composer images -------------------------------------------------------
  // A pasted or dropped image is a thumbnail chip held in the page and
  // uploaded only with the send, so an abandoned paste never touches disk. A
  // chip a cancel returned has a ref instead of data: the file the daemon
  // already wrote, which a re-send names rather than uploads again.
  //
  // Each image has a number n, and the text names it with an "[Image #n]"
  // marker inserted at the cursor. Marker and chip stay in sync: deleting the
  // marker drops the chip, removing the chip deletes the marker. A dropped
  // image waits in imgTrash, so an undo that brings its marker back brings
  // the chip back too. Numbers are never reused or rewritten within a draft
  // (rewriting text the user is editing shakes it); the server renumbers a
  // send that merges into a held message, so the page never numbers across
  // sends.
  var cimgs = $("cimgs"), imgs = [], imgSeq = 0, imgTrash = {};
  var IMG_TYPES = { "image/png": 1, "image/jpeg": 1, "image/gif": 1, "image/webp": 1 };
  var IMG_MAX = 8, IMG_BYTES = 10 * 1024 * 1024;
  var MARKER = "\\[Image #([1-9][0-9]{0,5})\\]";
  function attachSrc(name) { return "/s/" + rid + "/attachment/" + encodeURIComponent(name); }
  function marker(n) { return "[Image #" + n + "]"; }
  // markerNums is the set of image numbers text's markers name.
  function markerNums(text) {
    var out = {}, re = new RegExp(MARKER, "g"), m;
    while ((m = re.exec(text))) out[+m[1]] = true;
    return out;
  }
  // nextNum is a number no image, dropped image, or marker of this draft
  // has used.
  function nextNum() {
    Object.keys(markerNums(ctext.value)).forEach(function (k) { imgSeq = Math.max(imgSeq, +k); });
    return ++imgSeq;
  }
  // editText replaces [start, end) of the composer's text with s through the
  // browser's editing command, so the undo stack keeps the edit, else
  // through setRangeText; either way an input event follows.
  function editText(start, end, s) {
    if (doc.activeElement !== ctext) ctext.focus();
    ctext.setSelectionRange(start, end);
    var done = false;
    try { done = s ? doc.execCommand("insertText", false, s) : start === end || doc.execCommand("delete"); } catch (e) { done = false; }
    if (!done) {
      ctext.setRangeText(s, start, end, "end");
      ctext.dispatchEvent(new Event("input", { bubbles: true }));
    }
  }
  // insertMarkers puts the markers for nums at the cursor, spaced from the
  // text around them.
  function insertMarkers(nums) {
    if (!nums.length) return;
    var v = ctext.value, a = ctext.selectionStart, b = ctext.selectionEnd, s = nums.map(marker).join(" ");
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = " " + s;
    if (b >= v.length || !/\s/.test(v.charAt(b))) s += " ";
    editText(a, b, s);
  }
  // deleteMarkers deletes each marker naming n, with the space after it.
  function deleteMarkers(n) {
    var re = new RegExp("\\[Image #" + n + "\\] ?", "g"), v = ctext.value, spans = [], m;
    while ((m = re.exec(v))) spans.push([m.index, m.index + m[0].length]);
    spans.reverse().forEach(function (sp) { editText(sp[0], sp[1], ""); });
  }
  function addFiles(files) {
    var refused = "", added = [];
    files.forEach(function (f) {
      if (!IMG_TYPES[f.type]) { refused = wsT("js.img.type", f.name || wsT("js.img.thefile")); return; }
      if (f.size > IMG_BYTES) { refused = wsT("js.img.big", f.name || wsT("js.img.theimage")); return; }
      if (imgs.length >= IMG_MAX) { refused = wsT("js.img.max", IMG_MAX); return; }
      var it = { name: f.name || "pasted image", url: URL.createObjectURL(f), data: null, ref: "", n: nextNum() };
      imgs.push(it);
      added.push(it.n);
      var rd = new FileReader();
      rd.onload = function () { var v = String(rd.result); it.data = v.slice(v.indexOf(",") + 1); };
      rd.onerror = function () { dropImg(it); release(it); deleteMarkers(it.n); renderImgs(); syncNote(wsT("js.img.readfail", it.name)); };
      rd.readAsDataURL(f);
    });
    renderImgs();
    insertMarkers(added);
    syncNote(refused ? wsT("js.img.refused", refused) : "");
  }
  function dropImg(it) {
    var i = imgs.indexOf(it);
    if (i >= 0) imgs.splice(i, 1);
  }
  function release(it) { if (!it.ref) URL.revokeObjectURL(it.url); }
  // unattach removes a chip the user took off: its markers go, and it waits
  // in imgTrash for an undo.
  function unattach(it) {
    if (!it) return;
    dropImg(it);
    imgTrash[it.n] = it;
    renderImgs();
    deleteMarkers(it.n);
  }
  // clearTrash forgets the dropped images once no undo can bring them back.
  function clearTrash() {
    Object.keys(imgTrash).forEach(function (k) { release(imgTrash[k]); });
    imgTrash = {};
  }
  // syncMarkers follows the text: an image whose marker is gone is dropped
  // (into imgTrash), and a dropped one whose marker is back returns.
  function syncMarkers() {
    if (!imgs.length && !Object.keys(imgTrash).length) return;
    var have = markerNums(ctext.value), changed = false;
    imgs.slice().forEach(function (it) {
      if (!have[it.n]) { dropImg(it); imgTrash[it.n] = it; changed = true; }
    });
    Object.keys(imgTrash).forEach(function (k) {
      var it = imgTrash[k];
      if (have[it.n]) { delete imgTrash[k]; imgs.push(it); changed = true; }
    });
    if (!changed) return;
    imgs.sort(function (a, b) { return a.n - b.n; });
    renderImgs();
  }
  function renderImgs() {
    if (!cimgs) return;
    cimgs.textContent = "";
    imgs.forEach(function (it, i) {
      var el = doc.createElement("span"), im = doc.createElement("img"), x = doc.createElement("button"), num = doc.createElement("span");
      el.className = "ichip";
      im.src = it.url;
      im.alt = it.name;
      el.title = wsT("js.img.title", it.n, it.name);
      el.appendChild(im);
      num.className = "inum";
      num.textContent = "#" + it.n;
      el.appendChild(num);
      x.className = "unimg";
      x.setAttribute("data-unimg", String(i));
      x.title = wsT("js.img.remove");
      x.textContent = "×";
      el.appendChild(x);
      cimgs.appendChild(el);
    });
  }
  // imageFiles is the image files a clipboard or drop carries.
  function imageFiles(dt) {
    var out = [];
    if (!dt) return out;
    if (dt.items && dt.items.length) {
      Array.prototype.forEach.call(dt.items, function (item) {
        if (item.kind !== "file") return;
        var f = item.getAsFile();
        if (f) out.push(f);
      });
    } else if (dt.files) {
      out = Array.prototype.slice.call(dt.files);
    }
    return out;
  }
  var composer = $("composer");
  if (ctext && cimgs && composer) {
    ctext.addEventListener("paste", function (e) {
      var files = imageFiles(e.clipboardData).filter(function (f) { return f.type.indexOf("image/") === 0; });
      if (!files.length) return;
      // A paste that carries text as well (a copied web page) still pastes
      // its text; a screenshot carries only the image. The markers go in
      // once the paste is done, after any pasted text.
      var types = Array.prototype.slice.call(e.clipboardData.types || []);
      if (types.indexOf("text/plain") < 0) e.preventDefault();
      setTimeout(function () { addFiles(files); }, 0);
    });
    var hasFiles = function (e) { return e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0; };
    composer.addEventListener("dragover", function (e) {
      if (!hasFiles(e)) return;
      e.preventDefault();
      composer.classList.add("drop");
    });
    composer.addEventListener("dragleave", function (e) {
      if (!composer.contains(e.relatedTarget)) composer.classList.remove("drop");
    });
    composer.addEventListener("drop", function (e) {
      composer.classList.remove("drop");
      if (!hasFiles(e)) return;
      e.preventDefault();
      ctext.focus();
      addFiles(imageFiles(e.dataTransfer));
    });
  }
  // --- composer file mentions ------------------------------------------------
  // Typing @ opens the file popup (mention.js) on the files route: paths in
  // the session root by fuzzy search, or, for a browser on this machine, an
  // absolute path one directory at a time. A pick is recorded as a mention:
  // its range in the text and the text it inserted. An edit before a mention
  // moves it; an edit inside it, or one that glues text to either end, drops
  // it, so a mention is only ever what a pick inserted, and typed text such
  // as "@mary" is never one. The layer behind the textarea draws each mention
  // as a pill, and a send strips each one's @, so the session reads a plain
  // path (a code span when the path holds spaces). Held messages a cancel
  // returns come back as plain text: they were sent without the @.
  var chl = $("chl"), mentions = [], mentionWas = "", mention = null;
  function liveMention(m, v) {
    var before = v.charAt(m.start - 1), after = v.charAt(m.end);
    return v.slice(m.start, m.end) === m.text && (!before || /\s/.test(before)) && (!after || /\s/.test(after));
  }
  // trackMentions follows an edit, found as the span between the common
  // prefix and suffix of the text before and after it.
  function trackMentions() {
    var v = ctext.value, was = mentionWas, a = 0, s = 0;
    mentionWas = v;
    if (!mentions.length) return;
    while (a < was.length && a < v.length && was.charCodeAt(a) === v.charCodeAt(a)) a++;
    while (s < was.length - a && s < v.length - a && was.charCodeAt(was.length - 1 - s) === v.charCodeAt(v.length - 1 - s)) s++;
    var end = was.length - s, d = v.length - was.length;
    mentions = mentions.filter(function (m) {
      if (end <= m.start) { m.start += d; m.end += d; } else if (a < m.end) return false;
      return liveMention(m, v);
    });
  }
  // shiftMentions moves every mention by d after the text was set whole.
  function shiftMentions(d) {
    var v = ctext.value;
    mentions = mentions.filter(function (m) { m.start += d; m.end += d; return liveMention(m, v); });
    mentionWas = v;
    paintMentions();
  }
  function clearMentions() {
    mentions = [];
    mentionWas = ctext.value;
    paintMentions();
  }
  function paintMentions() {
    if (!chl || !window.wsMention) return;
    // The textarea's scrollbar narrows its text; the layer narrows alike.
    chl.style.paddingRight = (parseFloat(getComputedStyle(ctext).paddingRight) + ctext.offsetWidth - ctext.clientWidth) + "px";
    window.wsMention.paint(chl, ctext.value, mentions);
    chl.scrollTop = ctext.scrollTop;
  }
  // sendText is the composer's text with each mention's @ stripped.
  function sendText() {
    var v = ctext.value, out = "", at = 0;
    mentions.slice().sort(function (x, y) { return x.start - y.start; }).forEach(function (m) {
      if (m.start < at || !liveMention(m, v)) return;
      out += v.slice(at, m.start) + m.text.slice(1);
      at = m.end;
    });
    return out + v.slice(at);
  }
  // fileCandidates asks the files route for the candidates of q.
  function fileCandidates(q, done) {
    if (!window.fetch) { done([]); return; }
    fetch("/s/" + rid + "/files?q=" + encodeURIComponent(q), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.ok ? r.json() : { items: [] };
    }).then(function (v) { done((v && v.items) || []); }, function () { done([]); });
  }
  if (ctext && rid && window.wsMention) {
    mention = window.wsMention.attach(ctext, {
      label: wsT("js.mention.label"),
      source: fileCandidates,
      insert: editText,
      onPick: function (p) {
        mentions.push({ start: p.start, end: p.end, text: p.text });
        paintMentions();
      }
    });
    ctext.addEventListener("scroll", function () { if (chl) chl.scrollTop = ctext.scrollTop; });
    window.addEventListener("resize", paintMentions);
  }
  var sending = false;
  // topicNotes are the board's non-empty topic notes. A send carries them
  // as "[topic] text" lines after the typed text, the lines the board's
  // copy-answers adds; they are no item's answer, so they are no chip.
  function topicNotes() {
    var b = region("board");
    return (b ? q(".tnote[data-topic]", b) : []).map(function (el) {
      return { el: el, value: el.value, line: "[" + el.getAttribute("data-topic") + "] " + el.value.trim() };
    }).filter(function (n) { return n.value.trim(); });
  }
  function sendMessage() {
    if (sending) return;
    var raw = ctext.value.trim(), t = sendText().trim(), list = chipList(), pics = imgs.slice(), notes = topicNotes();
    if (!t && !list.length && !pics.length && !notes.length) { syncNote(wsT("js.send.empty")); return; }
    if (pics.some(function (p) { return !p.ref && p.data === null; })) { syncNote(wsT("js.send.reading")); return; }
    var lines = notes.map(function (n) { return n.line; }).join("\n");
    var body = { text: t + (t && lines ? "\n\n" : "") + lines, chips: list };
    var up = pics.filter(function (p) { return !p.ref; }), refs = pics.filter(function (p) { return !!p.ref; });
    if (up.length) body.images = up.map(function (p) { return { name: p.name, data: p.data, n: p.n }; });
    if (refs.length) {
      body.refs = refs.map(function (p) { return p.ref; });
      body.refNums = refs.map(function (p) { return p.n; });
    }
    sending = true;
    post("message", body, function (ok, res) {
      sending = false;
      if (!ok) { syncNote(wsT("js.send.fail", res)); return; }
      if (ctext.value.trim() === raw) { ctext.value = ""; grow(); clearMentions(); }
      pics.forEach(function (p) { dropImg(p); release(p); });
      clearTrash();
      // A fresh draft numbers its images from 1 again.
      if (!imgs.length && !ctext.value) imgSeq = 0;
      renderImgs();
      // Compare by value: a board event during the send, or clearing one
      // card, re-reads the chips into new objects. Drop every sent chip
      // before clearing any card, since clearing one re-syncs the rest.
      var sent = list.filter(function (c) { var cur = chips[c.id]; return cur && cur.answer === c.answer && cur.note === c.note; });
      sent.forEach(function (c) { dropChip(c.id); });
      sent.forEach(function (c) { setItem(c.id, null); });
      // A topic note edited during the send stays for the next one.
      notes.forEach(function (n) {
        if (n.el.value !== n.value) return;
        n.el.value = "";
        n.el.dispatchEvent(new Event("input", { bubbles: true }));
      });
      renderChips();
      syncNote(wsT(res.merged ? "js.send.merged" : "js.send.ok"));
    });
  }
  // restoreHeld puts held messages a cancel or an interrupt took back into
  // the composer, oldest first, ahead of the draft: their texts joined as
  // paragraphs, their answer chips, and their images as chips that reference
  // the written files, with their markers in the returned text. Each
  // message's numbers move above the highest the draft and the messages
  // before it use, the markers with them, as the server numbers a merge.
  function restoreHeld(list) {
    var cur = ctext.value.trim(), top = imgSeq, backs = [], back;
    // The draft's mentions move with it, past its leading whitespace's trim.
    var lead = ctext.value.length - ctext.value.replace(/^\s+/, "").length;
    Object.keys(markerNums(cur)).forEach(function (k) { top = Math.max(top, +k); });
    imgs.forEach(function (p) { top = Math.max(top, p.n); });
    list.forEach(function (res) {
      var text = res.text || "", names = res.images || [], nums = res.nums || [], renum = {}, offset = top;
      names.forEach(function (name, i) {
        var n = nums[i] || i + 1, have = imgs.filter(function (p) { return p.ref === name; })[0];
        renum[n] = have ? have.n : n + offset;
        if (!have) imgs.push({ name: name, url: attachSrc(name), data: null, ref: name, n: renum[n] });
        top = Math.max(top, renum[n]);
      });
      text = text.replace(new RegExp(MARKER, "g"), function (m, k) { return renum[+k] ? marker(renum[+k]) : m; });
      // A message held before images were numbered has no markers: give
      // each image one, or the next edit would drop it.
      var marked = markerNums(text), missing = [];
      names.forEach(function (name, i) { var n = renum[nums[i] || i + 1]; if (!marked[n]) missing.push(marker(n)); });
      if (missing.length) text += (text ? "\n" : "") + missing.join(" ");
      if (text) backs.push(text);
    });
    imgSeq = top;
    back = backs.join("\n\n");
    ctext.value = back + (back && cur ? "\n\n" : "") + cur;
    grow();
    shiftMentions(back.length + (back && cur ? 2 : 0) - lead);
    imgs.sort(function (a, b) { return a.n - b.n; });
    // A later message's answer replaces an earlier one's, and a chip
    // attached since keeps its newer answer.
    var got = {}, order = [];
    list.forEach(function (res) {
      (res.chips || []).forEach(function (c) { if (!got[c.id]) order.push(c.id); got[c.id] = c; });
    });
    order.forEach(function (id) { if (!chips[id]) { putChip(got[id]); setItem(id, got[id]); } });
    renderChips();
    renderImgs();
  }
  function cancelHeld(btn) {
    btn.disabled = true;
    post("cancel", { id: btn.getAttribute("data-cancel") }, function (ok, res) {
      if (!ok) { btn.disabled = false; syncNote(wsT("js.cancel.fail", res)); return; }
      restoreHeld([res]);
      syncNote(wsT("js.cancel.ok"));
      ctext.focus();
    });
  }
  // --- interrupt badge ---------------------------------------------------------
  // The badge on the border between the conversation and the composer shows
  // while the session has a live adapter and its lead works on a turn (the
  // convlive region's data-phase). One click stops the turn and takes every
  // held message back into the composer. It then stays disabled until the
  // phase leaves the working set, so a second click cannot cut the next turn.
  var ibadge = $("ibadge"), ilabel = ibadge && ibadge.querySelector(".ilabel");
  var WORKING = { thinking: 1, replying: 1, tool: 1, permission: 1 };
  var liveMode = mode === "live", interrupting = false;
  function leadWorking() {
    var c = doc.querySelector("#r-convlive .clive");
    return !!c && WORKING[c.getAttribute("data-phase")] === 1;
  }
  function syncBadge() {
    subBadge(sbadge, doc.querySelector("#r-convlive .clive"), liveMode);
    if (!ibadge) return;
    var on = liveMode && leadWorking();
    if (!on) interrupting = false;
    ibadge.hidden = !on;
    ibadge.disabled = interrupting;
    ibadge.classList.toggle("busy", interrupting);
    var label = wsT(interrupting ? "js.interrupt.busy" : "js.interrupt");
    if (ilabel && ilabel.textContent !== label) ilabel.textContent = label;
  }
  // --- subagent badge --------------------------------------------------------
  // Beside the interrupt badge: a spinner, the robot icon, and "N(+M)
  // subagents" while the session runs subagents, whether or not its lead
  // works. N is a convlive region's data-sub (its running direct children),
  // M its data-subdeep (every running agent below them), counted by the
  // server; "(+M)" shows only when M is above zero. on is live mode.
  var sbadge = $("sbadge");
  function subBadge(badge, clive, on) {
    if (!badge) return;
    var n = clive ? Number(clive.getAttribute("data-sub")) || 0 : 0;
    var m = clive ? Number(clive.getAttribute("data-subdeep")) || 0 : 0;
    badge.hidden = !(on && n + m > 0);
    if (badge.hidden) return;
    var label = wsTn("js.subagents", n + m, m > 0 ? n + "(+" + m + ")" : String(n));
    var el = badge.querySelector(".slabel");
    if (el && el.textContent !== label) el.textContent = label;
  }
  function interruptTurn() {
    if (interrupting) return;
    interrupting = true;
    syncBadge();
    post("interrupt", {}, function (ok, res) {
      if (!ok) { interrupting = false; syncBadge(); syncNote(wsT("js.interrupt.fail", res)); return; }
      var list = res.held || [];
      if (!list.length) { syncNote(wsT("js.interrupt.ok")); return; }
      restoreHeld(list);
      syncNote(wsT("js.interrupt.restored"));
      ctext.focus();
    });
  }
  // steerHeld promotes a held message to steering: the session receives it
  // at its next tool call instead of when its turn ends. The convlive
  // region's next swap shows the steer marker in place of the button.
  function steerHeld(btn) {
    btn.disabled = true;
    post("steer", { id: btn.getAttribute("data-steer") }, function (ok, res) {
      if (!ok) { btn.disabled = false; syncNote(wsT("js.steer.fail", res)); return; }
      syncNote(wsT("js.steer.ok"));
    });
  }
  // applySent marks each board item the session was sent an answer for, or
  // holds one for, from the inbox records the conversation region carries,
  // and dims the item's answer area (sent-sent, sent-held) until the board
  // re-renders it answered; each run recomputes both from scratch.
  function applySent() {
    var b = region("board");
    if (!b) return;
    q(".sentmark", b).forEach(function (m) { m.parentNode.removeChild(m); });
    q(".sent-sent, .sent-held", b).forEach(function (el) { el.classList.remove("sent-sent", "sent-held"); });
    q(".sentdata i[data-id]").forEach(function (i) {
      var item = boardItem(i.getAttribute("data-id")), head = item && item.querySelector(".head");
      if (!head) return;
      var state = i.getAttribute("data-state"), m = doc.createElement("span");
      item.classList.add(state === "held" ? "sent-held" : "sent-sent");
      m.className = "sentmark " + state;
      m.textContent = state + " · " + (i.getAttribute("data-answer") || "note");
      var note = i.getAttribute("data-note");
      m.title = wsT(state === "held" ? "js.sent.held.title" : "js.sent.sent.title") + (note ? ": " + note : "");
      var id = head.querySelector(".id");
      head.insertBefore(m, id ? id.nextSibling : head.firstChild);
    });
  }
  // indentLines indents by two spaces: at the cursor, or every line a
  // multi-line selection touches (blank lines stay blank). out removes up to
  // two leading spaces from each line the cursor or selection touches
  // instead. The selection follows the lines it covered.
  function indentLines(out) {
    var v = ctext.value, a = ctext.selectionStart, b = ctext.selectionEnd;
    var multi = v.slice(a, b).indexOf("\n") >= 0;
    if (!out && !multi) { editText(a, b, "  "); return; }
    var last = b > a && v.charAt(b - 1) === "\n" ? b - 1 : b;
    var ls = a > 0 ? v.lastIndexOf("\n", a - 1) + 1 : 0, le = v.indexOf("\n", last);
    if (le < 0) le = v.length;
    var first = 0, total = 0;
    var lines = v.slice(ls, le).split("\n").map(function (line, i) {
      var d = 0;
      if (out) { d = -/^ {0,2}/.exec(line)[0].length; line = line.slice(-d); }
      else if (line) { d = 2; line = "  " + line; }
      if (i === 0) first = d;
      total += d;
      return line;
    });
    if (!total) return;
    editText(ls, le, lines.join("\n"));
    // A selection that starts at its line's start keeps starting there.
    var na = a === ls ? ls : Math.max(ls, a + first);
    ctext.setSelectionRange(na, a === b ? na : Math.max(na, b + total));
  }
  if (ctext) {
    var csend = $("csend");
    if (csend) csend.addEventListener("click", function () { sendMessage(); ctext.focus(); });
    ctext.addEventListener("input", function () { grow(); syncNote(); syncMarkers(); trackMentions(); paintMentions(); });
    // Tab indents rather than leaving the composer; Escape, then Tab,
    // leaves it (the keyboard way out).
    var tabOut = false;
    ctext.addEventListener("blur", function () { tabOut = false; });
    ctext.addEventListener("keydown", function (e) {
      // A key that confirms an IME composition (Korean, Japanese, Chinese)
      // must never send or indent: browsers report it as isComposing, and
      // Safari reports the first keydown after compositionend as keyCode 229.
      if (e.isComposing || e.keyCode === 229) return;
      // While the file popup shows, it takes Up/Down, Tab and Enter (which
      // pick, never send), and Escape (which closes it without arming
      // Tab-out).
      if (mention && mention.key(e)) { tabOut = false; return; }
      if (e.key === "Tab") {
        if (e.ctrlKey || e.metaKey || e.altKey) return;
        if (tabOut) { tabOut = false; return; }
        e.preventDefault();
        indentLines(e.shiftKey);
        return;
      }
      if (e.key === "Escape") { tabOut = true; return; }
      if (e.key !== "Shift") tabOut = false;
      if (e.key !== "Enter") return;
      var mod = e.ctrlKey || e.metaKey;
      if (inputMode === "enter" ? !e.shiftKey && !e.altKey : mod) { e.preventDefault(); sendMessage(); }
    });
    // The form script's own listeners have rebuilt the reply line by the
    // time an event bubbles here.
    ["input", "change"].forEach(function (type) {
      doc.addEventListener(type, function (e) {
        if (!e.target.closest || !e.target.closest("#r-board")) return;
        syncChips();
        syncNote();
      });
    });
  }

  // --- clicks: delegated, so they survive every swap -----------------------
  doc.addEventListener("click", function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (cnote && t.closest("#r-board")) { syncChips(); syncNote(); }
    var u = t.closest("[data-unchip]");
    if (u) { var uid = u.getAttribute("data-unchip"); dropChip(uid); setItem(uid, null); renderChips(); syncNote(); return; }
    u = t.closest("[data-unimg]");
    if (u) { unattach(imgs[+u.getAttribute("data-unimg")]); syncNote(); return; }
    u = t.closest("#r-board button.closed-more[data-closed-next]");
    if (u) { loadClosed(u); return; }
    u = t.closest("#r-convlive [data-cancel]");
    if (u) { if (!u.disabled) cancelHeld(u); return; }
    u = t.closest("#r-convlive [data-steer]");
    if (u) { if (!u.disabled) steerHeld(u); return; }
    if (ibadge && t.closest("#ibadge")) { if (!ibadge.disabled) interruptTurn(); return; }
    if (gmenu && t.closest("#gear")) { gmenu.hidden = !gmenu.hidden; return; }
    if (gmenu && !gmenu.hidden && !t.closest("#gmenu")) gmenu.hidden = true;
    var th = t.closest(".gmenu [data-theme-set]");
    if (th) { setTheme(th.getAttribute("data-theme-set")); return; }
    if (!live) return;
    var b = t.closest(".gmenu [data-input]");
    if (b) { save(INPUT_KEY, b.getAttribute("data-input")); setInput(b.getAttribute("data-input")); return; }
    b = t.closest(".gmenu [data-layout]");
    if (b) { save(LAYOUT_KEY, b.getAttribute("data-layout")); setLayout(b.getAttribute("data-layout")); return; }
    b = t.closest("#tabbar [data-tab]");
    if (b) { setTab(b.getAttribute("data-tab")); return; }
  });

  // --- link actions: the menu a served page's marked links open ------------
  // The renderer marks a served link data-wl="file" (href: the file view, or
  // no href outside the session root; data-rel, data-abs) or data-wl="url";
  // no other anchor is touched. An image file link is also marked data-img;
  // an absolute image path outside the root is marked data-out, with the
  // image route as its href, which the daemon serves only to a browser on
  // its own machine. A plain click opens the menu; Cmd/Ctrl+click on a link
  // the page can open is left to the browser, which opens a new tab, and
  // opens the menu otherwise. The system-browser and native-app items show
  // only on a page the server marked data-sysopen (a browser on the daemon's
  // machine), and a data-out link opens at all only there; the daemon checks
  // both again. The markup is the same for every viewer: entries render once
  // for all of them.
  var lmenu = null, lmenuFrom = null, ltoast = null, ltoastTimer = 0;
  function closeLinkMenu(refocus) {
    if (!lmenu) return;
    lmenu.parentNode.removeChild(lmenu);
    lmenu = null;
    if (refocus && lmenuFrom && lmenuFrom.isConnected) lmenuFrom.focus();
    lmenuFrom = null;
  }
  function toast(text) {
    if (!ltoast) {
      ltoast = doc.createElement("div");
      ltoast.className = "ltoast";
      ltoast.setAttribute("role", "status");
      doc.body.appendChild(ltoast);
    }
    ltoast.textContent = text;
    ltoast.hidden = false;
    clearTimeout(ltoastTimer);
    ltoastTimer = setTimeout(function () { ltoast.hidden = true; }, 2200);
  }
  // copyText copies s: the Clipboard API where the page is a secure context,
  // else (a LAN address over plain http has no navigator.clipboard) a hidden
  // textarea and the copy command.
  function copyText(s) {
    function fallback() {
      var t = doc.createElement("textarea");
      t.value = s;
      t.setAttribute("readonly", "");
      t.style.cssText = "position:fixed;top:0;left:0;width:1px;height:1px;opacity:0";
      doc.body.appendChild(t);
      t.select();
      var ok = false;
      try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
      doc.body.removeChild(t);
      toast(wsT(ok ? "js.toast.copied" : "js.toast.copyfail", s));
    }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(s).then(function () { toast(wsT("js.toast.copied", s)); }, fallback);
    } else fallback();
  }
  // linkOpens reports whether this page can open a marked link's href: a
  // data-out link only on a data-sysopen page.
  function linkOpens(a) {
    return !!a.getAttribute("href") && (!a.hasAttribute("data-out") || root.hasAttribute("data-sysopen"));
  }
  function openLinkMenu(a, x, y) {
    closeLinkMenu(false);
    var kind = a.getAttribute("data-wl"), href = a.getAttribute("href");
    var rel = a.getAttribute("data-rel"), abs = a.getAttribute("data-abs"), url = a.href;
    var sys = root.hasAttribute("data-sysopen"), out = a.hasAttribute("data-out");
    var items = [];
    if (linkOpens(a)) items.push([wsT("js.menu.newtab"), function () { window.open(url, "_blank", "noopener"); }]);
    if (href && sys && !out) {
      items.push([wsT("js.menu.sysopen"), function () {
        // A file link sends its file view path; the daemon opens it on the
        // address this page was loaded from.
        post("open", { target: kind === "url" ? url : href }, function (ok, v) { toast(ok ? wsT("js.toast.opened") : wsT("js.toast.openfail", v)); });
      }]);
    }
    if (kind === "file" && abs && sys && a.hasAttribute("data-img")) {
      items.push([wsT("js.menu.native"), function () {
        post("open", { native: abs }, function (ok, v) { toast(ok ? wsT("js.toast.openednative") : wsT("js.toast.openfail", v)); });
      }]);
    }
    if (kind === "url") items.push([wsT("js.menu.copyurl"), function () { copyText(url); }]);
    else {
      if (rel) items.push([wsT("js.menu.copyrel"), function () { copyText(rel); }]);
      if (abs) items.push([wsT("js.menu.copyabs"), function () { copyText(abs); }]);
    }
    if (!items.length) return;
    var m = doc.createElement("div");
    m.className = "lmenu";
    m.setAttribute("role", "menu");
    items.forEach(function (it) {
      var b = doc.createElement("button");
      b.type = "button";
      b.setAttribute("role", "menuitem");
      b.textContent = it[0];
      b.addEventListener("click", function (e) { e.stopPropagation(); closeLinkMenu(true); it[1](); });
      m.appendChild(b);
    });
    doc.body.appendChild(m);
    var w = m.offsetWidth, h = m.offsetHeight;
    m.style.left = Math.max(4, Math.min(x, window.innerWidth - w - 4)) + "px";
    m.style.top = Math.max(4, y + h + 4 > window.innerHeight ? y - h - 4 : y + 4) + "px";
    lmenu = m;
    lmenuFrom = a;
    m.firstChild.focus();
  }
  // linkAt opens the menu for a marked link: at the pointer, or below the
  // link for a keyboard activation.
  function linkAt(a, e) {
    var r = a.getBoundingClientRect();
    if (e.detail === 0 || e.type === "keydown") openLinkMenu(a, r.left, r.bottom);
    else openLinkMenu(a, e.clientX, e.clientY);
  }
  doc.addEventListener("click", function (e) {
    var t = e.target;
    if (lmenu && !(t && t.closest && t.closest(".lmenu"))) closeLinkMenu(false);
    if (e.button !== 0 || !t || !t.closest) return;
    var a = t.closest("a[data-wl]");
    if (!a) return;
    if (linkOpens(a) && (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) return;
    e.preventDefault();
    linkAt(a, e);
  }, true);
  // An image outside the session root (img[data-out]) loads only for a
  // browser on the daemon's machine; where it fails (a LAN viewer, a missing
  // file) its alt text and its absolute path, as a copy-only link, take its
  // place, the shape a path the page does not serve takes. The page CSP
  // forbids inline onerror, so a capture listener catches the error, and a
  // sweep after each render catches an image that failed before it: decode()
  // rejects only a broken image, where naturalWidth alone is also 0 for a
  // loaded SVG with no intrinsic size.
  function outImgFailed(img) {
    var abs = img.getAttribute("data-abs") || "";
    var span = doc.createElement("span");
    span.className = "img-text";
    if (img.alt) span.appendChild(doc.createTextNode(img.alt + " "));
    var code = doc.createElement("code");
    code.className = "fpath";
    code.textContent = abs;
    // Inside a link already (an image linked to itself), the path stays
    // text: an anchor in an anchor has no defined click target.
    if (img.parentNode && img.parentNode.closest && img.parentNode.closest("a")) span.appendChild(code);
    else {
      var a = doc.createElement("a");
      a.setAttribute("tabindex", "0");
      a.setAttribute("data-wl", "file");
      a.setAttribute("data-abs", abs);
      a.setAttribute("title", abs);
      a.appendChild(code);
      span.appendChild(a);
    }
    if (img.parentNode) img.parentNode.replaceChild(span, img);
  }
  function sweepOutImgs(el) {
    q("img[data-out]", el).forEach(function (img) {
      if (!img.complete || img.naturalWidth) return;
      if (img.decode) img.decode().then(null, function () { if (img.isConnected) outImgFailed(img); });
      else outImgFailed(img);
    });
  }
  doc.addEventListener("error", function (e) {
    var t = e.target;
    if (t && t.tagName === "IMG" && t.hasAttribute("data-out")) outImgFailed(t);
  }, true);
  sweepOutImgs(doc);
  doc.addEventListener("keydown", function (e) {
    var inMenu = !!lmenu && lmenu.contains(doc.activeElement);
    if (lmenu && e.key === "Escape") { if (inMenu) e.preventDefault(); closeLinkMenu(inMenu); return; }
    if (inMenu && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      var bs = Array.prototype.slice.call(lmenu.children), i = bs.indexOf(doc.activeElement);
      e.preventDefault();
      bs[(i + (e.key === "ArrowDown" ? 1 : bs.length - 1)) % bs.length].focus();
      return;
    }
    // An action link with no href is not natively activatable.
    var a = e.target && e.target.closest && e.target.closest("a[data-wl]:not([href])");
    if (a && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); linkAt(a, e); }
  });
  // A user's scroll closes the menu; the conversation's own pinning to the
  // newest reply scrolls too, and must not.
  doc.addEventListener("wheel", function (e) { if (lmenu && !lmenu.contains(e.target)) closeLinkMenu(false); }, { capture: true, passive: true });
  doc.addEventListener("touchmove", function (e) { if (lmenu && !lmenu.contains(e.target)) closeLinkMenu(false); }, { capture: true, passive: true });
  window.addEventListener("resize", function () { closeLinkMenu(false); });
  window.addEventListener("blur", function () { closeLinkMenu(false); });

  // --- drawer: the side panels pulled down from the top --------------------
  // A frame of registered entries over the page. Each entry is { id, title,
  // icon, count, badge, content }: functions read at every sync, content
  // returning the element the entry's card is or null when the entry is
  // absent, and badge (optional) a count for the handle icon or null. The
  // four panels are the first entries: views onto the one panels region,
  // read again after each swap, so a later entry registers here without
  // reworking the frame. The sheet shows every present entry's card at once
  // and the handle holds one icon per present entry. Open state and height
  // are this browser's (ws.dashboard.drawer*).
  //
  // The git handle (the git region, left of the handle) opens the same frame
  // on the git drawer's sheet (#gbody) instead of the panels: drMode is the
  // sheet the frame shows, "panels" or "git", and only the handle of the
  // shown sheet rides its bottom edge. The git sheet's open state is never
  // kept, and it shares the panels' height; only the panels handle drags.
  var drawer = $("drawer"), drclip = $("drclip"), drsheet = $("drsheet"), drbody = $("drbody");
  var drhandle = $("drhandle"), drheader = doc.querySelector(".dwrap > .top");
  var gbody = $("gbody"), gitRegion = region("git");
  var DR_OPEN = "ws.dashboard.drawer", DR_H = "ws.dashboard.drawer.h";
  var DR_MIN = 160, DR_DEF = 320, DR_SHUT = 80;
  var drRoomW = -1, drGhOff = -1, drEntries = [], drWant = false, drOpen = false, drHas = false, drH = DR_DEF, drTop = 0;
  var drMode = "panels", drShown = "";
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
  cardTab("settings");
  // The worktrees card's nodes (wtLoad, below): set here, ahead of the
  // drawer's first drApply, which a drawer left open calls on load.
  var wtlist = $("wtlist"), wtcount = $("wtcount"), wtnote = $("wtnote"), wtclean = $("wtclean"), wtSeq = 0;

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
    drRoom();
    drFill();
  }
  // drFill stretches cards into the empty cells of the panels grid's last
  // row: the card directly above each empty cell spans two rows. Cards keep
  // their DOM order (grid-auto-flow: dense would reorder them); a spanning
  // card sits in the full row above the last, right of every last-row card,
  // so auto-placement puts the last row's cards in the same cells as before.
  // The column count is the computed track list (auto-fit's collapsed tracks
  // read 0px). Spans clear first, and only while the body is a grid: the
  // narrow layout (a flex column) and the git sheet (body hidden) get none.
  // A card already placed or spanned by a stylesheet skips the fill.
  var drFilled = [];
  function drFill() {
    if (!drbody) return;
    drFilled.forEach(function (el) { el.style.gridRow = ""; });
    drFilled = [];
    var cs = getComputedStyle(drbody);
    if (drbody.hidden || cs.display !== "grid") return;
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
    if (n < 2 || k <= n || !rem) return;
    var placed = cards.some(function (c) {
      var s = getComputedStyle(c);
      return [s.gridRowStart, s.gridRowEnd, s.gridColumnStart, s.gridColumnEnd].some(function (v) { return v !== "auto"; });
    });
    if (placed) return;
    var above = (Math.floor(k / n) - 1) * n;
    for (var c = rem; c < n; c++) {
      cards[above + c].style.gridRow = "span 2";
      drFilled.push(cards[above + c]);
    }
  }
  function drDot() {
    var d = drhandle.querySelector(".tdot");
    if (d) d.classList.toggle("on", drBadge || drSwap);
  }
  // drRoom keeps the header and the tracker's handle row clear of both
  // handles: --drhw is their width together (the header's padding and the
  // tracker row read it), --ghr the git handle's offset from the right edge
  // (the panels handle's width and the gap, 0 without that handle).
  function drRoom() {
    var w = drHas ? Math.ceil(drhandle.getBoundingClientRect().width) : 0;
    var gw = ghandle() ? Math.ceil(gitRegion.getBoundingClientRect().width) : 0;
    var off = w && gw ? w + 6 : 0, all = gw ? off + gw : w;
    if (all !== drRoomW || off !== drGhOff) {
      drRoomW = all;
      drGhOff = off;
      doc.documentElement.style.setProperty("--drhw", all + "px");
      doc.documentElement.style.setProperty("--ghr", off + "px");
    }
    if (drheader) drheader.classList.toggle("dr-room", all > 0);
  }
  function drTitle() {
    var t = wsT(drOpen && drMode === "panels" ? "js.drawer.close" : "js.drawer.open"), tip = drhandle.getAttribute("data-tip");
    if (tip) t += " (" + tip + ")";
    if (drhandle.classList.contains("wait")) t += "; " + wsT("js.drawer.waiting");
    drhandle.title = t;
    drhandle.setAttribute("aria-label", t);
  }
  // drawerWait marks the handle while an approval waits, open or closed:
  // an open drawer can cover the agents tree's approval row.
  function drawerWait() {
    if (!drhandle) return;
    drhandle.classList.toggle("wait", !!doc.querySelector("#r-convlive .permwait, #r-tree .phase.permission, #r-tree .brow.perm"));
    drTitle();
  }
  // A panels change the drawer is closed over lights the handle's dot.
  function drawerSwapped() {
    if (drhandle && !drOpen && drHas) { drSwap = true; drDot(); }
  }
  function drHeaderBottom() {
    return drheader ? Math.max(0, Math.round(drheader.getBoundingClientRect().bottom)) : 0;
  }
  function drMeasure() {
    drTop = drHeaderBottom();
    drawer.style.setProperty("--drtop", drTop + "px");
  }
  // drMax is the tallest sheet: 60% of the window, and short of the
  // composer with the handle below it.
  function drMax() {
    var m = Math.round(window.innerHeight * 0.6), c = $("composer");
    if (visible(c)) m = Math.min(m, Math.round(c.getBoundingClientRect().top) - drTop - drhandle.offsetHeight - 6);
    return Math.max(0, m);
  }
  function drClamp(h) { var hi = drMax(); return Math.max(Math.min(DR_MIN, hi), Math.min(hi, h)); }
  // drApply lays the drawer out for its state; anim animates a click's
  // toggle only.
  function drApply(anim) {
    if (!drawer) return;
    drMeasure();
    var h = drClamp(drH), git = drMode === "git";
    drawer.classList.toggle("anim", !!anim);
    drawer.classList.toggle("open", drOpen);
    drbody.hidden = git;
    if (gbody) gbody.hidden = !git;
    drsheet.style.height = h + "px";
    drclip.style.height = (drOpen ? h : 0) + "px";
    drhandle.style.top = (drOpen && !git ? drTop + h : 0) + "px";
    if (gitRegion) gitRegion.style.top = (drOpen && git ? drTop + h : 0) + "px";
    drhandle.setAttribute("aria-expanded", String(drOpen && !git));
    ghSync();
    drTitle();
    drFill();
    // Each opening of a sheet reads its data again: the worktree pool for
    // the panels, the changed files for the git sheet.
    var shown = drOpen ? drMode : "";
    if (shown !== drShown) {
      drShown = shown;
      if (shown === "panels") wtLoad();
      else if (shown === "git") gitOpened();
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
    if (open && !(mode === "git" ? ghandle() : drHas)) return;
    if (open && !drOpen) drAnnounce();
    if (!open && drsheet.contains(doc.activeElement)) (drMode === "git" && ghandle() || drhandle).focus();
    drMode = mode;
    drOpen = open;
    drWant = open && mode === "panels";
    save(DR_OPEN, drWant ? "1" : null);
    if (drWant) { drSwap = false; drDot(); }
    drApply(true);
  }
  if (drawer) {
    drWant = load(DR_OPEN) === "1";
    var savedH = parseFloat(load(DR_H));
    if (isFinite(savedH) && savedH > 0) drH = savedH;
    drawerSync();
    drawerWait();
    // drag: the handle follows the pointer; past the header's bottom the
    // sheet comes with it, and the release sets the height or, short of
    // DR_SHUT, closes. A press that never moves is a click.
    var drDrag = null, drDragged = false;
    drhandle.addEventListener("pointerdown", function (e) {
      if (e.button !== undefined && e.button !== 0) return;
      drMeasure();
      drDrag = { y0: e.clientY, off: e.clientY - drhandle.getBoundingClientRect().top, moved: false, ext: 0 };
      try { drhandle.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the moves still land */ }
    });
    drhandle.addEventListener("pointermove", function (e) {
      if (!drDrag) return;
      if (!drDrag.moved && Math.abs(e.clientY - drDrag.y0) < 5) return;
      if (!drDrag.moved && !drOpen) drAnnounce();
      // Dragging the panels handle over the open git sheet brings the
      // panels in its place.
      if (!drDrag.moved && drMode !== "panels") { drMode = "panels"; drApply(false); }
      drDrag.moved = true;
      var y = e.clientY - drDrag.off, max = drMax(), ext = Math.max(0, Math.min(max, y - drTop));
      drDrag.ext = ext;
      drawer.classList.remove("anim");
      drawer.classList.add("drag");
      drsheet.style.height = Math.max(Math.min(DR_MIN, max), ext) + "px";
      drclip.style.height = ext + "px";
      drhandle.style.top = (ext > 0 ? drTop + ext : Math.max(0, Math.min(y, drTop))) + "px";
    });
    function drEnd() {
      var d = drDrag;
      drDrag = null;
      if (!d || !d.moved) return; // a press that stayed put is a click
      drawer.classList.remove("drag");
      drDragged = true;
      setTimeout(function () { drDragged = false; }, 0);
      if (d.ext < DR_SHUT) { drSetOpen(false); return; }
      drH = drClamp(d.ext);
      save(DR_H, String(drH));
      drWant = drOpen = true;
      save(DR_OPEN, "1");
      drSwap = false; drDot();
      drApply(false);
    }
    drhandle.addEventListener("pointerup", drEnd);
    drhandle.addEventListener("pointercancel", drEnd);
    drhandle.addEventListener("click", function () {
      if (drDragged) return;
      drSetOpen(!(drOpen && drMode === "panels"), "panels");
    });
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
      if (ctext && doc.activeElement === ctext) return;
      drSetOpen(false);
    }, true);
    // A click outside the drawer closes it; the link menu a drawer link
    // opens counts as inside.
    doc.addEventListener("click", function (e) {
      var t = e.target;
      if (!drOpen || drDragged || !t || !t.closest) return;
      if (t.closest("#drclip, #drhandle, #r-git, .lmenu")) return;
      drSetOpen(false);
    }, true);
    // The header can wrap or change height (a chip appears): the sheet
    // follows its bottom edge. A resize re-clamps the height without saving.
    window.addEventListener("resize", function () { drApply(false); });
    if (window.ResizeObserver && drheader) new ResizeObserver(function () { drApply(false); }).observe(drheader);
    // The handles' widths follow their contents; the header pads clear of
    // them.
    if (window.ResizeObserver) {
      var drRo = new ResizeObserver(drRoom);
      drRo.observe(drhandle);
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
        if (!drDrag && drHeaderBottom() !== drTop) drApply(false);
      });
    }, { passive: true });
    drOpen = drHas && drWant;
    drApply(false);
  }

  // --- the board's topic-body frames -------------------------------------------
  // The server renders each topic body on the board in its own sandboxed
  // frame with an opaque origin (wswhiteboard framedDoc), so its scripts
  // cannot reach this page or the daemon's routes. The frame's script
  // (doc-frame.js) posts its content height, its marked-link clicks, and its
  // item-reference clicks. A message counts only when its source is a body
  // frame on this page, and its
  // fields are data, checked as such: a link opens the same menu a board
  // link does, with an href kept only in the shapes the renderer gives one
  // (an http(s) URL for a url link, a scheme-less daemon path for a file
  // link), so a forged message can at most offer a menu the user must pick
  // from.
  function docFrame(win) {
    return q("iframe.doc-frame").filter(function (f) { return f.contentWindow === win; })[0];
  }
  function str(v) { return typeof v === "string" ? v : ""; }
  function num(v) { return typeof v === "number" && isFinite(v) ? v : 0; }
  function frameLink(m) {
    var kind = str(m.kind), href = str(m.href);
    if (kind === "url") { if (!/^https?:\/\//i.test(href)) return null; }
    else if (kind === "file") { if (!/^\/[^\/\\\x00-\x20\x7f][^\\\x00-\x20\x7f]*$/.test(href)) href = ""; }
    else return null;
    var a = doc.createElement("a");
    a.setAttribute("data-wl", kind);
    if (href) a.setAttribute("href", href);
    if (str(m.rel)) a.setAttribute("data-rel", str(m.rel));
    if (str(m.abs)) a.setAttribute("data-abs", str(m.abs));
    if (kind === "file" && m.img === true) a.setAttribute("data-img", "");
    return a;
  }
  window.addEventListener("message", function (e) {
    var f = docFrame(e.source), m = e.data;
    if (!f || !m || typeof m !== "object") return;
    if (m.wbDoc === "height") {
      var h = num(m.height);
      if (h >= 0 && h < 100000) f.style.height = Math.ceil(h) + "px";
      // A frame rendered before a theme switch (a board swap on a stream
      // opened earlier) takes the current theme when it first reports.
      postTheme(f);
    } else if (m.wbDoc === "jump") {
      var item = /^item-\d+$/.test(str(m.id)) && $(m.id);
      if (item) item.scrollIntoView({ block: "start" });
    } else if (m.wbDoc === "link") {
      var a = frameLink(m);
      if (!a) return;
      var r = f.getBoundingClientRect();
      openLinkMenu(a, r.left + num(m.x), r.top + num(m.y));
    }
  });
  // A swap re-creates every frame, which starts at no height until it posts
  // one; it takes the old frame of the same topic's height meanwhile.
  function frameHeights(el) {
    var h = {};
    q("iframe.doc-frame", el).forEach(function (f) { if (f.style.height) h[f.getAttribute("data-doc")] = f.style.height; });
    return h;
  }
  function setFrameHeights(el, h) {
    if (!h) return;
    q("iframe.doc-frame", el).forEach(function (f) {
      var k = f.getAttribute("data-doc");
      if (Object.prototype.hasOwnProperty.call(h, k)) f.style.height = h[k];
    });
  }

  // --- pooled worktrees: the drawer card over the worktrees route ---------
  // Each drawer opening reads the pool (worktrees.go): the list draws at
  // once, and while sizes are still measured a second read waits for them.
  // Leased worktrees come first, shown active; idle ones group by profile.
  // The cleanup button asks the session to clean the pool up, then closes
  // the drawer. The dashboard itself never removes a worktree.
  function wtGet(wait, done) {
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch("/s/" + rid + "/worktrees" + (wait ? "?wait=1" : ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (!r.ok) { done(false, s.trim() || "HTTP " + r.status); return; }
        try { done(true, JSON.parse(s)); } catch (e) { done(false, "malformed answer"); }
      });
    }, function () { done(false, wsT("js.post.unreachable")); });
  }
  function wtLoad() {
    if (!wtlist) return;
    var seq = ++wtSeq;
    if (!wtlist.firstChild) wtlist.textContent = wsT("js.wt.loading");
    wtGet(false, function (ok, pool) {
      if (seq !== wtSeq) return;
      if (!ok) { wtFail(pool); return; }
      wtDraw(pool);
      if (pool.sizing) wtGet(true, function (ok2, sized) { if (seq === wtSeq && ok2) wtDraw(sized); });
    });
  }
  function wtFail(msg) {
    wtlist.textContent = "";
    wtlist.appendChild(mk("div", "wtempty", wsT("js.wt.fail", msg)));
  }
  // mk makes an element of class cls holding text.
  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  // wtSize is a byte count in decimal units, as the cleanup message gives it.
  function wtSize(n) {
    if (n < 1000) return n + " B";
    var v = n / 1000, i = 0;
    while (v >= 1000 && i < 3) { v /= 1000; i++; }
    return v.toFixed(1) + " " + "kMGT".charAt(i) + "B";
  }
  // wtAge is a coarse age: its largest unit only.
  function wtAge(ms) {
    var m = Math.max(0, Math.floor((Date.now() - ms) / 60000));
    if (m < 60) return m + "m";
    if (m < 48 * 60) return Math.floor(m / 60) + "h";
    return Math.floor(m / 1440) + "d";
  }
  function wtStamp(ms) { return new Date(ms).toLocaleString(wsI18n.lang); }
  function wtRow(w) {
    var row = mk("div", "wtrow" + (w.leased ? " on" : "") + (w.quarantined ? " quar" : ""));
    row.title = w.path + (w.quarantined ? "\n" + wsT("js.wt.quarantined") : "");
    row.appendChild(mk("span", "wtdot"));
    row.appendChild(mk("b", "wtname", w.name));
    if (w.leased) {
      var br = mk("span", "wtbr", w.detached ? wsT("js.wt.detached") : w.branch);
      if (!w.detached) br.title = w.branch; // a long branch ellipsizes
      row.appendChild(br);
      if (w.holder) {
        var who = mk("span", "wtwho", w.holder);
        who.title = wsT("js.wt.holder", w.holder);
        row.appendChild(who);
      }
    }
    var at = w.leased ? w.acquired_at || w.active_at : w.active_at;
    if (at) {
      var age = mk("span", "wtage", wtAge(at));
      age.title = wsT(w.leased ? "js.wt.held" : "js.wt.unused", wtStamp(at));
      row.appendChild(age);
    }
    row.appendChild(mk("span", "wtsize", w.size == null ? "…" : wtSize(w.size)));
    return row;
  }
  function wtDraw(pool) {
    var list = pool.worktrees || [], leased = [], groups = {}, order = [];
    list.forEach(function (w) {
      if (w.leased) { leased.push(w); return; }
      var p = w.profile || "?";
      if (!groups[p]) { groups[p] = []; order.push(p); }
      groups[p].push(w);
    });
    wtlist.textContent = "";
    wtlist.title = wsT("js.wt.pool", pool.pool || "");
    if (!list.length) wtlist.appendChild(mk("div", "wtempty", wsT("js.wt.empty")));
    if (leased.length) {
      var sec = mk("div", "wtsec on");
      leased.forEach(function (w) { sec.appendChild(wtRow(w)); });
      wtlist.appendChild(sec);
    }
    order.sort().forEach(function (p) {
      var sec = mk("div", "wtsec"), head = mk("div", "wtgrp");
      head.appendChild(mk("span", "wtprof", p));
      head.appendChild(mk("span", "chip", String(groups[p].length)));
      sec.appendChild(head);
      groups[p].forEach(function (w) { sec.appendChild(wtRow(w)); });
      wtlist.appendChild(sec);
    });
    if (wtcount) {
      wtcount.hidden = !list.length;
      wtcount.textContent = leased.length + "/" + list.length;
      wtcount.title = wsT("js.wt.count", leased.length, list.length);
    }
  }
  if (wtclean) wtclean.addEventListener("click", function () {
    if (wtclean.disabled) return;
    wtclean.disabled = true;
    if (wtnote) wtnote.textContent = "";
    post("worktrees/cleanup", {}, function (ok, res) {
      wtclean.disabled = false;
      if (!ok) { if (wtnote) wtnote.textContent = wsT("js.wt.cleanfail", res); return; }
      drSetOpen(false);
    });
  });

  // --- git drawer: the action panel over the git routes ---------------------
  // Each opening of the git sheet asks the daemon to read the handle again
  // and reads the changed files (gitdrawer.go): the standing line, then the
  // staged, unstaged, and untracked groups, each file with its status and
  // line delta. Fetch, pull, and push post to their routes (gitpush.go), one
  // at a time: the pressed button shows busy and the others wait; the
  // daemon's refusal or git's failure shows on the panel's message line, a
  // pull's "nothing to pull" as a plain note. Every answer reads the files
  // again; the handle's own re-read arrives through the stream.
  var gref = $("gref"), gfiles = $("gfiles"), gmsg = $("gmsg"), gSeq = 0, gBusy = false;
  function gitOpened() {
    if (!gbody || !window.fetch) return;
    fetch("/s/" + rid + "/git", { method: "POST", credentials: "same-origin", cache: "no-store" }).catch(function () {});
    gitLoad();
  }
  function gitLoad() {
    var seq = ++gSeq;
    if (!gfiles.firstChild) gfiles.appendChild(mk("div", "gempty", wsT("js.git.loading")));
    fetch("/s/" + rid + "/git/status", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gSeq) return;
        if (!r.ok) { gitFail(s.trim() || "HTTP " + r.status); return; }
        try { gitDraw(JSON.parse(s)); } catch (e) { gitFail("malformed answer"); }
      });
    }, function () { if (seq === gSeq) gitFail(wsT("js.post.unreachable")); });
  }
  // gitFail shows a failed read in place of the files; the standing line
  // goes with them, since it is as old as the failed read says.
  function gitFail(msg) {
    gref.textContent = "";
    gfiles.textContent = "";
    gfiles.appendChild(mk("div", "gempty err", wsT("js.git.fail", msg)));
  }
  // gitRef draws the standing line: branch, upstream, and the counts.
  function gitRef(v) {
    gref.textContent = "";
    if (v.detached) { gref.appendChild(mk("b", "gbr", wsT("js.git.detached", v.branch))); return; }
    gref.appendChild(mk("b", "gbr", v.branch));
    if (!v.upstream) { gref.appendChild(mk("span", "gnoup", wsT("js.git.noupstream"))); return; }
    gref.appendChild(mk("span", "gto", "\u2192"));
    gref.appendChild(mk("span", "gup", v.upstream));
    if (v.ahead) gref.appendChild(mk("b", "up", "\u2191" + v.ahead));
    if (v.behind) gref.appendChild(mk("b", "dn", "\u2193" + v.behind));
  }
  function gitRow(f) {
    var row = mk("div", "gf"), slash = f.path.lastIndexOf("/");
    row.title = f.path;
    row.appendChild(mk("b", "gst st-" + (f.status === "?" ? "q" : f.status), f.status));
    var p = mk("span", "gp");
    if (slash >= 0) p.appendChild(mk("span", "gdir", f.path.slice(0, slash + 1)));
    p.appendChild(mk("span", "gname", f.path.slice(slash + 1)));
    row.appendChild(p);
    var d = mk("span", "gd");
    if (f.add < 0) d.appendChild(mk("i", "bin", "bin"));
    else if (f.status !== "?") {
      if (f.add) d.appendChild(mk("i", "add", "+" + f.add));
      if (f.del) d.appendChild(mk("i", "del", "\u2212" + f.del));
    }
    row.appendChild(d);
    return row;
  }
  function gitDraw(v) {
    gitRef(v);
    gfiles.textContent = "";
    var groups = [["staged", "js.git.staged"], ["unstaged", "js.git.unstaged"], ["untracked", "js.git.untracked"]], any = false;
    groups.forEach(function (g) {
      var list = v[g[0]] || [];
      if (!list.length) return;
      any = true;
      var sec = mk("div", "gsec"), head = mk("div", "ggrp");
      head.appendChild(mk("span", "", wsT(g[1])));
      head.appendChild(mk("span", "chip", String(list.length)));
      sec.appendChild(head);
      list.forEach(function (f) { sec.appendChild(gitRow(f)); });
      gfiles.appendChild(sec);
    });
    if (!any) gfiles.appendChild(mk("div", "gempty", wsT("js.git.clean")));
    if (v.truncated) gfiles.appendChild(mk("div", "gempty", wsT("js.git.truncated", 300)));
  }
  function gitMsg(text, err) {
    gmsg.textContent = text || "";
    gmsg.classList.toggle("err", !!err);
    gmsg.hidden = !text;
  }
  // gitAct runs one action from its button.
  function gitAct(btn) {
    if (gBusy || !window.fetch) return;
    var label = btn.querySelector("span"), idle = label.textContent;
    gBusy = true;
    q(".gact", gbody).forEach(function (b) { b.disabled = true; });
    btn.classList.add("busy");
    label.textContent = btn.getAttribute("data-busy");
    gitMsg("");
    function done(text, err) {
      gBusy = false;
      q(".gact", gbody).forEach(function (b) { b.disabled = false; });
      btn.classList.remove("busy");
      label.textContent = idle;
      gitMsg(text, err);
      gitLoad();
    }
    fetch("/s/" + rid + "/git/" + btn.getAttribute("data-git"), { method: "POST", credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (t) {
        t = (t || "").trim();
        if (r.status === 204) done("", false);
        else if (r.ok) done(t, false);
        else done(t || "HTTP " + r.status, true);
      });
    }, function () { done(gmsg.getAttribute("data-lost"), true); });
  }
  if (gbody) gbody.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest(".gact");
    if (b && !b.disabled) gitAct(b);
  });

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
  // and at most one queued. Seen marks are only ever added, so a read unites
  // the daemon's marks with the local copy, which may hold marks still being
  // posted.
  var tkApi = null, tkOrder = [], tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSubs = [];
  var tkBusy = false, tkQueued = false, tkNext = [], tkLastRev = null, tkNudgedRev = null;
  function tkUrl(id, op) { return "/tracker/stores/" + encodeURIComponent(id) + "/" + op; }
  // tkFetch calls a tracker route, a POST when body is given; done(status,
  // answer) gets the HTTP status (0: the daemon was not reached) and the
  // parsed JSON answer ({} when there is none).
  function tkFetch(path, body, done, keepalive) {
    var o = { method: body === undefined ? "GET" : "POST", credentials: "same-origin", cache: "no-store" };
    if (body !== undefined) { o.headers = { "Content-Type": "application/json" }; o.body = JSON.stringify(body); }
    if (keepalive) o.keepalive = true;
    fetch(path, o).then(function (r) {
      return r.text().then(function (s) {
        var v = null;
        try { v = s ? JSON.parse(s) : null; } catch (e) { v = null; }
        done(r.status, v && typeof v === "object" ? v : {});
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
  function tkNotify() { tkSubs.forEach(function (fn) { fn(); }); }
  // tkLoadView reads store id's view and seen marks into the cache.
  function tkLoadView(id, done) {
    tkFetch(tkUrl(id, "view"), undefined, function (status, v) {
      if (status === 200 && v.view) {
        tkViews[id] = v.view;
        tkErrs[id] = null;
        tkAddMarks(id, v.seen && v.seen.badge, v.seen && v.seen.read);
      } else {
        tkViews[id] = null;
        tkErrs[id] = tkErrText(status, v);
      }
      done();
    });
  }
  // tkReadAll reads the store list, calls listed(status), and with a list
  // reads every store's view in parallel before done().
  function tkReadAll(listed, done) {
    tkFetch("/tracker/stores", undefined, function (status, v) {
      if (status === 503) { tkHostErr = tkErrText(status, v); tkOrder = []; }
      if (status === 200) {
        tkHostErr = null;
        if (typeof v.rev === "number") tkLastRev = v.rev;
        tkOrder = (Array.isArray(v.stores) ? v.stores : []).map(function (s) { return String(s.id); });
        var keep = {};
        tkOrder.forEach(function (id) { keep[id] = true; });
        [tkViews, tkErrs, tkSeen].forEach(function (m) { Object.keys(m).forEach(function (id) { if (!keep[id]) delete m[id]; }); });
      }
      listed(status);
      var left = status === 200 ? tkOrder.length : 0;
      if (!left) { done(); return; }
      tkOrder.forEach(function (id) { tkLoadView(id, function () { if (--left === 0) done(); }); });
    });
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
    var thens = tkNext;
    tkNext = [];
    tkReadAll(function (status) {
      if (!tkApi && (status === 200 || status === 503)) tkMount();
    }, function () {
      tkBusy = false;
      if (tkNudgedRev !== null) {
        if (tkNudgedRev !== tkLastRev) tkQueued = true;
        tkNudgedRev = null;
      }
      if (tkApi) tkNotify();
      thens.forEach(function (fn) { fn(); });
      if (tkQueued) tkReload();
    });
  }
  var tkAdapter = {
    stores: function () { return tkOrder.slice(); },
    view: function (id) { return tkViews[id] || null; },
    error: function (id) { return tkViews[id] ? null : tkErrs[id] || null; },
    hostError: function () { return tkHostErr; },
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
          var byId = {};
          ((tkViews[id] && tkViews[id].events) || []).forEach(function (e) { byId[e.id] = e; });
          done({ ok: true, results: ids.map(function (eid, i) {
            var e = byId[eid];
            if (e) return { id: e.id, alias: e.alias, type: e.type, status: e.status, reason: e.reason || "" };
            return { id: eid, alias: String(eid).replace("#", "-"), type: events[i] ? events[i].type : "", status: "applied", reason: "" };
          }) });
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
  // session drawer's handle; the store page takes the page's place.
  function tkMount() {
    var header = doc.querySelector(".dwrap > .top"), slot = $("qsslot"), wrap = doc.querySelector(".dwrap");
    if (!header || !slot || !wrap) return;
    header.classList.add("tk-room");
    tkApi = window.WsTracker.mount(tkAdapter, { header: header, qsSlot: slot, pageParent: wrap, storagePrefix: "ws.tracker." });
  }
  function trackerStart() {
    if (window.WsTracker && window.fetch) tkReload();
  }
  // trackerNudge handles the stream's tracker event: a revision other than
  // the last one handled reloads. While a reload runs (the page load's, which
  // the stream's first event usually meets), the revision waits for its end
  // and reloads again only when the store list it read brought another one,
  // so a page loads once. trackerLost forgets the revision when the stream
  // drops, since a restarted daemon counts from 0 again.
  function trackerNudge(rev) {
    if (!window.WsTracker || !window.fetch) return;
    if (tkBusy) { tkNudgedRev = rev; return; }
    if (rev === tkLastRev) return;
    tkLastRev = rev;
    tkReload();
  }
  function trackerLost() { tkLastRev = null; }

  // --- stream ----------------------------------------------------------------
  // The stream state shows on every indicator in the top bar: the bare dot,
  // and the harness badge's LED, which a head region swap redraws lit, so
  // the swap reapplies the state (swap above).
  var sseOn = true;
  function connected(on) {
    sseOn = on;
    q(".dwrap > .top .sse").forEach(function (el) {
      el.classList.toggle("off", !on);
      el.title = wsT(on ? "js.sse.title.on" : "js.sse.title.off");
    });
  }
  function connect() {
    if (!window.EventSource) return;
    var path = mode === "index" ? "/events" : "/s/" + rid + "/events";
    var es = new EventSource(path + "?h=" + encodeURIComponent(held()));
    es.onopen = function () { connected(true); };
    es.onerror = function () {
      connected(false);
      trackerLost();
      // EventSource would retry with the URL it opened; reopen with the
      // hashes held now, so a reconnect sends only what changed since.
      es.close();
      setTimeout(connect, 3000);
    };
    es.addEventListener("region", function (e) {
      var ev = JSON.parse(e.data);
      if (ev.name === "mode") {
        if (ev.html !== mode) modeChanged(ev.html);
        modeHash = ev.hash;
        liveMode = ev.html === "live";
        syncBadge();
        return;
      }
      swap(ev.name, ev.hash, ev.html);
    });
    es.addEventListener("tracker", function (e) {
      var ev = null;
      try { ev = JSON.parse(e.data); } catch (x) { return; }
      if (ev && typeof ev.rev === "number") trackerNudge(ev.rev);
    });
    es.addEventListener("gone", function () {
      es.close();
      connected(false);
      banner(wsT("js.banner.gone"));
    });
  }
  function banner(text) {
    var b = $("dash-banner");
    if (!b) return;
    b.textContent = text;
    b.hidden = false;
  }
  // holdsDraft reports unsent work a reload would drop: composer text,
  // answers picked on the board, or an attached image.
  function holdsDraft() { return !!((ctext && ctext.value) || boardAnswers() || imgs.length || topicNotes().length); }
  function modeChanged(next) {
    // The other mode has other regions, and a reload renders them; text typed
    // in the composer or answers picked on the board are never dropped for it.
    if (holdsDraft()) {
      banner(wsT(next === "live" ? "js.banner.appeared" : "js.banner.wentaway"));
      return;
    }
    location.reload();
  }

  if (live) {
    setLayout(load(LAYOUT_KEY));
    setSplit(savedSplit());
    setInput(load(INPUT_KEY));
    setTab("convo");
  }
  restoreOpen(doc);
  // A body frame that posted its height before the listener above existed
  // posts it again.
  q("iframe.doc-frame").forEach(function (f) { if (f.contentWindow) f.contentWindow.postMessage({ wbDoc: "size" }, "*"); });
  if (window.wbForm) window.wbForm();
  applySent();
  syncBadge();
  fmtTimes(doc);
  mermaid();
  pinConvo();
  nearTop();
  trackerStart();
  connect();
})();
