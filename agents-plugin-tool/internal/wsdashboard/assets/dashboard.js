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
    var f = t.getAttribute("data-f"), rel = f === "since" || f === "ago" || f === "rel";
    if (rel) {
      var s = f === "since" ? dur(now - ms) : f === "rel" ? relAge(now - ms) : wsT("js.ago", dur(now - ms));
      if (t.textContent !== s) t.textContent = s;
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
  // again catches up at once rather than on the next tick.
  function tickTimes() {
    if (doc.hidden) return;
    var now = Date.now();
    q('time[data-ms][data-f="since"], time[data-ms][data-f="ago"], time[data-ms][data-f="rel"]').forEach(function (t) { fmtTime(t, now); });
    q("em.u-pace[data-reset]").forEach(function (e) { pace(e, now); });
  }
  setInterval(tickTimes, 1000);
  doc.addEventListener("visibilitychange", tickTimes);

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
  var PANES = ["bcol", "r-tree", "convo", "drbody"];
  // A scroll only marks its pane; the offset is read once per frame, in a
  // frame callback beside the board rail's, so the visibility check and
  // the scrollTop read share the frame's layout rather than force one of
  // their own from inside every scroll event.
  var paneScroll = {}, paneMoved = {}, paneFrame = 0;
  function notePanes() {
    paneFrame = 0;
    PANES.forEach(function (id) {
      if (!paneMoved[id]) return;
      paneMoved[id] = false;
      var el = $(id);
      if (visible(el)) paneScroll[id] = el.scrollTop;
    });
  }
  PANES.forEach(function (id) {
    var el = $(id);
    if (el) el.addEventListener("scroll", function () {
      paneMoved[id] = true;
      if (!paneFrame) paneFrame = requestAnimationFrame(notePanes);
    }, { passive: true });
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
  // The conversation panel's header (chead) switches "Conversation |
  // Files" (ctabs): the Files pane (files.js) stands in for the
  // conversation below the header, which is only hidden meanwhile, so its
  // scroll and pin survive the switch; the agents tree and the composer
  // stay. The conversation tab counts as shown while either of the two
  // shows.
  var TAB = { tree: "convo", entries: "convo", convlive: "convo", board: "board" };
  var PANE = { convo: "convo", board: "bcol" };
  var fpane = $("files"), chead = $("chead"), filesSeen = false;
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
      if (visible(el) && Object.prototype.hasOwnProperty.call(paneScroll, id)) el.scrollTop = paneScroll[id];
    });
    pinConvo();
    nearTop(); // a conversation shown too short to scroll still loads older entries
    Object.keys(PANE).forEach(function (t) { var d = tabDot(t); if (d && tabShown(t)) d.classList.remove("on"); });
    if (visible(convo) && ctabDot()) ctabDot().classList.remove("on");
    // The Files pane hears each time it comes into view (a tab, a layout,
    // or a resize showed it) and each time it leaves.
    var fs = visible(fpane);
    if (fs !== filesSeen && window.WsFiles) {
      if (fs) window.WsFiles.shown();
      else window.WsFiles.hidden();
    }
    filesSeen = fs;
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
  // The conversation's header stands between the splitter and the pane
  // below it (the conversation, or the Files pane under its tab), outside
  // the shares.
  function shareAt(y) {
    var t = $("r-tree"), c = visible($("convo")) ? $("convo") : fpane;
    if (!visible(t) || !visible(c) || !splitter) return null;
    var top = t.getBoundingClientRect().top, room = c.getBoundingClientRect().bottom - top - splitter.offsetHeight - (visible(chead) ? chead.offsetHeight : 0);
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
  // setCtab switches the conversation column between the conversation and
  // the Files pane; it is not kept across a reload.
  function setCtab(t) {
    if (t !== "files" || !fpane || !window.WsFiles) t = "convo";
    grid.setAttribute("data-ctab", t);
    q("#ctabs [data-ctab]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-ctab") === t)); });
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
    q(".card[data-id], .arow[data-id], .srow[data-id]", el).forEach(function (item) {
      var s = {}, note = item.querySelector("textarea.note"), box = item.querySelector("label.ack > input");
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
    q(".card[data-id], .arow[data-id], .srow[data-id]", el).forEach(function (item) {
      var s = saved[item.getAttribute("data-id")];
      if (!s) return;
      var note = item.querySelector("textarea.note"), box = item.querySelector("label.ack > input");
      if (s.pick) q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) { if (pickKey(b) === s.pick) b.setAttribute("aria-pressed", "true"); });
      if (note && (s.note || s.noteOpen)) openNote(note, s);
      if (box && s.checked) box.checked = true;
    });
    q(".tnote[data-topic]", el).forEach(function (note) {
      var s = saved["t:" + note.getAttribute("data-topic")];
      // A note a send closed (closeTopicNotes) keeps its text closed.
      if (s && s.noteOpen) openNote(note, s);
      else if (s) note.value = s.note || "";
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
      swap("board", d.hash, d.html, true);
    }, 0);
  });

  // --- region swap -----------------------------------------------------------
  // The mode pseudo-region's hash is unknown to the page until the first
  // event carries it; an empty hash makes the server send it once. A region
  // update held back while the tab is hidden (hiddenRegions, see connect)
  // counts as held: a reconnect meanwhile names its hash, and the update
  // still lands when the tab shows.
  var modeHash = "", hiddenRegions = {};
  function held() {
    return q("[data-region]").map(function (el) {
      var name = el.getAttribute("data-region"), p = hiddenRegions[name];
      return name + "." + (p ? p.hash : el.getAttribute("data-h"));
    }).concat(mode !== "index" ? ["mode." + modeHash] : []).join(",");
  }
  // A deferred board swap lands still (no board motion), so the board never
  // moves under a just-blurred input.
  function swap(name, hash, html, still) {
    var el = region(name);
    if (!el) return;
    var board = name === "board";
    if (el.getAttribute("data-h") === hash) { if (board) deferred = null; return; }
    if (board && editing()) { deferred = { hash: hash, html: html }; return; }
    if (name === "head") {
      if (labelEditing()) { headHeld = { hash: hash, html: html }; return; }
      headHeld = null;
    }
    if (board) deferred = null;
    var pin = (name === "entries" || name === "convlive") && pinned();
    var scroll = scrollState();
    var answers = board ? readAnswers(el) : null;
    var railTop = name === "rail" ? railScrollTop() : null;
    var frameH = board ? frameHeights(el) : null;
    var before = board && !still && !reducedMotion() ? boardSpots(el) : null;
    // The entries merge and the board morph prepare only the nodes they
    // insert; any other region is replaced whole. A finished reply's mermaid
    // fences draw once merged; the streaming reply (convlive) keeps them
    // code blocks.
    var ins = null;
    if (name === "entries") mergeEntriesRegion(el, html, pin);
    else if (board) {
      ins = morphBoard(el, html);
      ins.forEach(function (n) {
        restoreOpen(n);
        fmtTimes(n);
        sweepOutImgs(n);
        setFrameHeights(n, frameH);
        writeAnswers(n, answers);
      });
      if (before) {
        var moves = topicMotion(before, ins).map(function (m) { return growTopic(m.n, m.from); });
        startMotion(moves.concat(slideBoard(el, before)));
        ins.forEach(function (n) { q("iframe.doc-frame", n).forEach(function (f) { if (!f.wbOut) f.wbGrow = true; }); });
      }
    } else {
      el.innerHTML = html;
      restoreOpen(el);
      fmtTimes(el);
      sweepOutImgs(el);
    }
    el.setAttribute("data-h", hash);
    if (board) {
      if (window.wbForm) window.wbForm();
      mermaid(ins);
      syncChips();
      syncNote();
      // A mention button over a body frame the swap removed has nothing left
      // to add.
      if (mpend && mpend.frame && !doc.contains(mpend.frame)) hideMention();
    }
    if (board || name === "convlive") applySent(board);
    if (name === "panels") drawerSync();
    if (name === "git") gitSwapped();
    restoreScroll(scroll);
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
    if (live && TAB[name] === "convo" && !visible(convo) && visible(fpane) && ctabDot()) ctabDot().classList.add("on");
    if (name === "panels") drawerSwapped();
    if (name === "tree" || name === "convlive") drawerWait();
    if (name === "tree") agentTipSwapped();
    if (TL_REGIONS[name]) { delete tlHeld[name]; tlCount(); }
    if (name === "convlive") syncBadge();
    if (name === "entries") syncChead();
    if (name === "head" && !sseOn) connected(false);
    if (name === "head") labelSwapped();
    if (name === "rail") { railMarks(); railScrollTo(railTop); }
    if (name === "head" || name === "convlive") leadSwapped(name);
    if (name === "rail" || name === "sessions") apScan(el);
  }

  // --- board morph: unchanged topics keep their nodes -------------------------
  // Each topic section carries data-topic, its name. A board swap keeps a
  // topic's section, and with it the body frame (no reload, no second
  // mermaid load, no second image relay), when the section's incoming HTML
  // equals the HTML it last arrived with (boardSeen). The comparison is
  // never against the live DOM, which the page changes after a swap (times,
  // sent marks, answers, frame themes and heights). Everything else the
  // board holds (the closed-topic control, the reply bar) is replaced. The
  // initial record is the board as served, read before anything changes it.
  var boardSeen = {};
  function topicKey(n) {
    return n.nodeType === 1 && n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic") ? n.getAttribute("data-topic") : null;
  }
  (function () {
    var b = region("board");
    if (b) Array.prototype.forEach.call(b.children, function (n) { var k = topicKey(n); if (k !== null) boardSeen["t:" + k] = n.outerHTML; });
  })();
  // --- board cards (cards.js): the page's own cards among the topics --------
  // The framework mounts on the board region once, before any board swap,
  // and restores the cards this browser kept for the session. Showing a
  // card switches to the board tab where the view shows one column at a
  // time.
  // Terminal cards (terminal.js) exist only on a page the server marked
  // data-term; their module adds the board column's header row.
  var cardBoard = mode !== "index" && window.WsCards ? region("board") : null;
  if (cardBoard) {
    if (window.WsPreview) window.WsPreview.setup({ renderDiff: gwRender, post: post, toast: toast });
    if (window.WsTerm) window.WsTerm.setup({ post: post, toast: toast });
    window.WsCards.mount(cardBoard, {
      key: "ws.dashboard.cards." + rid, load: load, save: save,
      reveal: function () { if (live && !tabShown("board")) setTab("board"); }
    });
    if (window.WsTerm) window.WsTerm.mount($("bcol"));
  }
  // --- the Files pane (files.js): mounted once, loaded when first shown -------
  if (live && fpane && window.WsFiles) {
    window.WsFiles.mount(fpane, {
      rid: rid, load: load, save: save, post: post, toast: toast, copy: copyText,
      rootSlot: chead && chead.querySelector(".fsum"),
      local: function () { return root.hasAttribute("data-sysopen"); }
    });
  }
  // morphBoard applies a board region event to el and returns the element
  // nodes it inserted, in order, for the caller to prepare.
  function morphBoard(el, html) {
    var src = frag(html), seen = {}, want = [], ins = [], live = {}, liveClosed = [], pages = [], more = null, grafts = [];
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n);
      if (k !== null) { live["t:" + k] = n; if (n.classList.contains("closed")) liveClosed.push(n); }
      else if (n.classList.contains("closed-page")) pages.push(n);
    });
    Array.prototype.slice.call(src.childNodes).forEach(function (n) {
      var k = topicKey(n);
      if (k === null) {
        want.push(n);
        if (n.nodeType !== 1) return;
        ins.push(n);
        if (n.matches("button.closed-more[data-closed-next]")) more = n;
        return;
      }
      var h = n.outerHTML, key = "t:" + k;
      seen[key] = h;
      // A topic that closes while the page shows it folds once: the
      // server folds a closed topic's body and settled rows under the same
      // keys an open topic uses, so the user's earlier choices for them are
      // forgotten here, and later toggles are kept again.
      if (live[key] && n.classList.contains("closed") && !live[key].classList.contains("closed")) { forgetOpen(live[key]); forgetOpen(n); }
      if (live[key] && boardSeen[key] === h) { want.push(live[key]); delete live[key]; }
      else {
        var g = live[key] && frameGraftable(live[key], n);
        if (g) grafts.push(g);
        want.push(n); ins.push(n);
      }
    });
    boardSeen = seen;
    // Closed topics fetched past the cap (closed pages, each nested in the
    // one before it) stay across swaps in the new control's place, since the
    // board renders only its capped section again. Closed-topic indices
    // follow close order, so a topic that newly closes pushes the oldest
    // capped one past the cap: that one moves to the head of the pages, a
    // fetched topic the new board shows itself leaves them, and the pages'
    // own control counts on from what is shown. No topic shows twice, and
    // none drops out between the capped section and the pages. A board with
    // no control holds every closed topic, and the pages go.
    if (pages.length && more) {
      var shown = {}, shifted = liveClosed.filter(function (n) { return !seen["t:" + topicKey(n)]; }), count = shifted.length;
      Object.keys(seen).forEach(function (key) { shown[key] = true; });
      shifted.forEach(function (n) { shown["t:" + topicKey(n)] = true; });
      pages.forEach(function (p) {
        q(".closed-page > section.topic[data-topic]", p).forEach(function (t) {
          var key = "t:" + t.getAttribute("data-topic");
          if (shown[key]) t.parentNode.removeChild(t);
          else { shown[key] = true; count++; }
        });
      });
      var anchor = pages[0].firstChild;
      shifted.forEach(function (n) { pages[0].insertBefore(n, anchor); });
      var tail = q("button.closed-more[data-closed-next]", pages[pages.length - 1]).pop();
      if (tail) tail.setAttribute("data-closed-next", String(+more.getAttribute("data-closed-next") + count));
      want.splice.apply(want, [want.indexOf(more), 1].concat(pages));
      ins.splice(ins.indexOf(more), 1);
    }
    // The page's own cards (cards.js) stand among the topics: they are
    // woven into the wanted nodes at their places, and the board settles
    // around them. Stale nodes go first, so the kept ones that stay in
    // order are never moved (moving a frame reloads it), and a card is
    // never moved at all. A changed topic whose old section stays in place
    // keeps that section and its body frame (frameGraft); where the browser
    // has Element.moveBefore, a kept node moves without reloading its
    // frames, so every changed topic keeps its old section (frameGrafts).
    var woven = window.WsCards.weave(want);
    frameGrafts(el, woven, grafts).forEach(function (g) {
      frameGraft(g);
      woven[woven.indexOf(g.n)] = g.old;
      ins[ins.indexOf(g.n)] = g.old;
    });
    window.WsCards.settle(el, woven);
    return ins;
  }
  // --- a changed topic's body frame, double-buffered --------------------------
  // A changed topic's new section would bring a new body frame, blank until
  // its document loads and reports a height. Instead, when the topic stays
  // open and its old section would stay where it stands, or the browser
  // can move it without reloading its frame (frameGrafts), the old section
  // is kept: its content becomes the new section's, except the
  // path down to the old body frame, and the new frame loads beside the old
  // one, unseen and out of the layout (.doc-frame-next), at the old one's
  // height. Once the new frame has loaded and drawn, it takes the old one's
  // place in the same frame (frameShow, frameReveal); one that never loads
  // shows after frameRevealMs anyway.
  var frameRevealMs = 1500;
  // frameChain is the path from frame f up to section sec, f first, or null.
  function frameChain(f, sec) {
    var c = [];
    for (var x = f; x && x !== sec; x = x.parentNode) c.push(x);
    return x === sec ? c.concat(sec) : null;
  }
  // frameGraftable returns the graft of new section n into old section old
  // (both open, each with a body frame of the same topic document on paths
  // of the same shape), or null.
  function frameGraftable(old, n) {
    if (old.classList.contains("closed") || n.classList.contains("closed")) return null;
    var of = q("iframe.doc-frame:not(.doc-frame-next)", old)[0], nf = q("iframe.doc-frame", n)[0];
    if (!of || !nf || of.getAttribute("data-doc") !== nf.getAttribute("data-doc")) return null;
    var oc = frameChain(of, old), nc = frameChain(nf, n);
    if (!oc || !nc || oc.length !== nc.length) return null;
    for (var i = 0; i < oc.length; i++) if (oc[i].tagName !== nc[i].tagName) return null;
    return { old: old, n: n, oc: oc, nc: nc };
  }
  // frameGrafts returns the grafts that move nothing settling el into want
  // would not move anyway, each graft's new section standing for its old
  // one: moving a frame reloads it, so a topic whose old section would move
  // (an update moves its topic to the head), or would push kept topics
  // around it, takes its new section as before. When the grafts together
  // move something, each one that alone moves nothing is kept, if those
  // together still move nothing. Where the browser has Element.moveBefore,
  // settle moves a kept node without reloading its frames (cards.js), so
  // every graft is kept, whatever moves.
  function frameGrafts(el, want, grafts) {
    if (!grafts.length || el.moveBefore) return grafts;
    var base = settleMoves(el, want);
    function still(gs) {
      var by = new Map(gs.map(function (g) { return [g.n, g.old]; })), ok = true;
      settleMoves(el, want.map(function (n) { return by.get(n) || n; })).forEach(function (n) { if (!base.has(n)) ok = false; });
      return ok;
    }
    if (still(grafts)) return grafts;
    var each = grafts.filter(function (g) { return still([g]); });
    return each.length && still(each) ? each : [];
  }
  // settleMoves returns the nodes already in el that settling el into want
  // moves. It replays WsCards.settle (cards.js) on a list: keep the two in
  // step.
  function settleMoves(el, want) {
    var keep = new Set(want), moved = new Set(), i = 0;
    var cur = Array.prototype.filter.call(el.childNodes, function (n) { return keep.has(n); });
    want.forEach(function (n) {
      if (cur[i] === n) { i++; return; }
      var j = cur.indexOf(n);
      if (j >= 0 && n.nodeType === 1 && n.hasAttribute("data-card")) {
        var past = cur.splice(i, j - i);
        past.forEach(function (p) { moved.add(p); });
        cur.splice.apply(cur, [i + 1, 0].concat(past));
      } else {
        if (j >= 0) { moved.add(n); cur.splice(j, 1); }
        cur.splice(i, 0, n);
      }
      i++;
    });
    return moved;
  }
  // frameGraft moves g's new section content into its old section, level by
  // level down the frame's path: each kept element takes the new element's
  // attributes and children, except its child on the path, which stays.
  // At the bottom the new frame goes in after the old one, pending.
  function frameGraft(g) {
    var of = g.oc[0], nf = g.nc[0];
    nf.classList.add("doc-frame-next");
    nf.setAttribute("tabindex", "-1");
    nf.setAttribute("aria-hidden", "true");
    nf.wbNext = true;
    nf.wbOld = of;
    of.wbOut = true;
    nf.addEventListener("load", function () { frameShow(nf); });
    setTimeout(function () { frameReveal(nf); }, frameRevealMs);
    for (var i = g.oc.length - 1; i > 0; i--) {
      var oa = g.oc[i], na = g.nc[i], oc = g.oc[i - 1], nc = g.nc[i - 1], before = true;
      Array.prototype.slice.call(oa.attributes).forEach(function (a) { if (!na.hasAttribute(a.name)) oa.removeAttribute(a.name); });
      Array.prototype.forEach.call(na.attributes, function (a) { if (oa.getAttribute(a.name) !== a.value) oa.setAttribute(a.name, a.value); });
      Array.prototype.slice.call(oa.childNodes).forEach(function (c) { if (c !== oc) oa.removeChild(c); });
      Array.prototype.slice.call(na.childNodes).forEach(function (c) {
        if (c === nc) { before = false; if (c === nf) oa.appendChild(c); }
        else if (before) oa.insertBefore(c, oc);
        else oa.appendChild(c);
      });
    }
  }
  // frameShow reveals pending frame f once it has loaded and drawn: a
  // frame revealed in the frame it loads in can still show blank for a
  // frame, so the reveal waits two frames more.
  function frameShow(f) {
    if (f.wbShowing) return;
    f.wbShowing = true;
    requestAnimationFrame(function () { requestAnimationFrame(function () { frameReveal(f); }); });
  }
  // frameReveal shows pending frame f in its old frame's place, at the old
  // frame's height, and asks it for its height, which the frame message
  // handler sets (easing to it in a moving swap, wbGrow). A frame no longer
  // on the page (a later swap replaced it) is left alone.
  function frameReveal(f) {
    if (!f.wbNext || !f.isConnected) return;
    f.wbNext = false;
    f.classList.remove("doc-frame-next");
    f.removeAttribute("tabindex");
    f.removeAttribute("aria-hidden");
    var o = f.wbOld;
    f.wbOld = null;
    if (o && o.parentNode) o.parentNode.removeChild(o);
    if (f.contentWindow) f.contentWindow.postMessage({ wbDoc: "size" }, "*");
  }
  // Board motion: a board swap's inserted topic sections grow from the
  // height they had before (a topic the board did not hold grows from 0)
  // to their natural height, and every topic section and card standing in
  // the board slides from where it stood to where it stands now (topics
  // are ordered by their last update, so an update moves its topic to the
  // head and shifts the rest). The spots are read before the morph; a
  // section still animating reads at its rendered height and place, so a
  // new swap starts from there. Topics in the closed pages (moved there or
  // out of there by the closed-page reconciliation) do not move. A changed
  // topic's frame starts at its old height and eases to the first height
  // it reports (the frame message handler, wbGrow).
  function reducedMotion() { return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches); }
  // boardSpots reads each topic section's height and offset from the
  // board's top, keyed "t:" + its topic, and each card's offset, keyed
  // "c:" + its id.
  function boardSpots(el) {
    var h = {}, top = el.getBoundingClientRect().top;
    q("section.topic[data-topic]", el).forEach(function (n) {
      var r = n.getBoundingClientRect();
      h["t:" + n.getAttribute("data-topic")] = { h: r.height, y: r.top - top, paged: n.parentNode !== el };
    });
    Array.prototype.forEach.call(el.children, function (n) {
      if (n.hasAttribute("data-card")) h["c:" + n.getAttribute("data-card")] = { y: n.getBoundingClientRect().top - top };
    });
    return h;
  }
  // topicMotion returns, in order, the inserted topic sections that animate
  // and the height each starts from.
  function topicMotion(before, ins) {
    var out = [];
    ins.forEach(function (n) {
      var k = topicKey(n);
      if (k === null) return;
      var b = before["t:" + k];
      if (b && b.paged) return;
      out.push({ n: n, from: b ? b.h : 0 });
    });
    return out;
  }
  // growTopic runs after the caller prepared the section (frame heights
  // restored), so its natural height is the target. The animation fills
  // nothing: once done the section has no fixed height, and a later size
  // change (a frame posting its height, a details toggle) is never clipped.
  // A section clips its overflow itself (.topic), and a new one also fades
  // in with its bottom margin growing.
  function growTopic(n, from) {
    if (!n.animate) return null;
    var to = n.getBoundingClientRect().height;
    if (Math.abs(to - from) < 1) return null;
    var a = { height: from + "px" }, b = { height: to + "px" };
    if (!from) {
      a.opacity = 0; b.opacity = 1;
      a.marginBottom = "0px"; b.marginBottom = getComputedStyle(n).marginBottom;
    }
    return n.animate([a, b], { duration: 200, easing: "ease-out" });
  }
  // slideBoard moves each topic section and card standing in the board from
  // its spot before the swap to its place now (FLIP): the layout is
  // already final, and a transform that starts at the old offset eases to
  // none. It runs after the growth started, so the places it reads are the
  // growth's first frame and slide and growth add up to one movement.
  // Nothing moves in the DOM (moving a frame reloads it), and the
  // animation fills nothing, so no transform stays behind. A maximized card
  // stays put: a transform would anchor its fixed inner box to the card.
  function slideBoard(el, before) {
    var top = el.getBoundingClientRect().top, out = [];
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n), card = k === null && n.hasAttribute("data-card");
      var b = k !== null ? before["t:" + k] : card ? before["c:" + n.getAttribute("data-card")] : null;
      if (!b || b.paged || !n.animate || (card && n.hasAttribute("data-max"))) return;
      var dy = b.y - (n.getBoundingClientRect().top - top);
      if (Math.abs(dy) < 1) return;
      out.push(n.animate([{ transform: "translateY(" + dy + "px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }));
    });
    return out;
  }
  // startMotion starts the swap's animations at the swap's first frame. An
  // animation's start time is the timeline's time, which holds still at the
  // last frame's through the tasks after it, up to the next frame's own
  // callbacks; a board swap is a long task (a large board's parse and
  // morph, its new topics' frames), so started as created the animations
  // were over when the swap's first frame drew: it showed the old heights
  // (a closing topic's whole open height, empty, and the board under it
  // blank) and the next frame jumped to the end. The start is set to the
  // clock at the first frame's callbacks instead, and until then (that
  // frame itself, whose timeline time is older) the animations hold their
  // first keyframe.
  function startMotion(list) {
    list = list.filter(Boolean);
    if (!list.length) return;
    list.forEach(function (a) { a.effect.updateTiming({ fill: "backwards" }); });
    requestAnimationFrame(function () {
      var t = performance.now();
      list.forEach(function (a) { if (a.playState === "running") a.startTime = t; });
    });
  }

  // --- conversation entries: merged by key, never re-rendered ----------------
  // Each entry element carries data-entry, its ring entry name; names sort in
  // time order. Only the entries container is searched: held bubbles in the
  // convlive region carry data-entry too (their inbox id) and are not
  // entries. A box keeps the names it holds (entryKeys) across merges, so a
  // merge reads only the incoming entries.
  function entriesBox() { var r = region("entries"); return r && r.querySelector(".centries"); }
  function frag(html) { var t = doc.createElement("template"); t.innerHTML = html; return t.content; }
  function entryKeys(box) {
    if (!box.wsKeys) {
      box.wsKeys = Object.create(null);
      Array.prototype.forEach.call(box.children, function (c) { var k = c.getAttribute("data-entry"); if (k) box.wsKeys[k] = true; });
    }
    return box.wsKeys;
  }
  function resetEntries(box) { box.textContent = ""; box.wsKeys = Object.create(null); }
  // mergeEntries inserts each node of src whose name box lacks at its sorted
  // place and returns the inserted nodes, in order; an entry already shown
  // is kept as it is.
  function mergeEntries(box, src) {
    var have = entryKeys(box), added = [];
    Array.prototype.slice.call(src.children).forEach(function (n) {
      var k = n.getAttribute("data-entry");
      if (!k || have[k]) return;
      // New entries are usually the newest (live) or the oldest (history),
      // so walk back from the end only past entries that sort after k.
      var c = box.lastElementChild;
      while (c && (c.getAttribute("data-entry") || "") > k) c = c.previousElementSibling;
      box.insertBefore(n, c ? c.nextSibling : box.firstChild);
      have[k] = true;
      added.push(n);
      restoreOpen(n);
      fmtTimes(n);
    });
    return added;
  }
  // gapped reports a newest window (nbox) that does not reach what box
  // holds: its oldest entry is newer than box's newest, so entries between
  // them, missed while a stream was down, are in neither.
  function gapped(box, nbox) {
    var last = box.lastElementChild, first = nbox.firstElementChild;
    return !!(last && first && (first.getAttribute("data-entry") || "") > (last.getAttribute("data-entry") || ""));
  }
  // ENTRY_CAP bounds a conversation box while the user keeps it at the
  // bottom: three of the server's newest windows (its ring size, 50).
  // evictEntries drops the oldest entries past it and reports whether it
  // dropped any.
  var ENTRY_CAP = 150;
  function evictEntries(box) {
    var n = box.children.length - ENTRY_CAP, keys = entryKeys(box);
    if (n <= 0) return false;
    for (; n > 0; n--) {
      var c = box.firstElementChild;
      delete keys[c.getAttribute("data-entry")];
      box.removeChild(c);
    }
    return true;
  }
  // mergeEntriesRegion applies an entries region event: the title bar (its
  // counts) is replaced, and the entries are merged. A window past a gap
  // starts the box over; a box at the bottom (pin) sheds its oldest entries
  // past the cap. Either way the older history is loaded again from what the
  // box then holds.
  function mergeEntriesRegion(el, html, pin) {
    var src = frag(html), box = entriesBox(), head = el.querySelector(".thead"), nhead = src.querySelector(".thead");
    var nbox = src.querySelector(".centries");
    if (!box || !nbox) { el.innerHTML = html; restoreOpen(el); fmtTimes(el); sweepOutImgs(el); mermaid(el); histReset(); return; }
    if (head && nhead) { head.parentNode.replaceChild(nhead, head); fmtTimes(nhead); }
    if (gapped(box, nbox)) { resetEntries(box); histReset(); }
    var added = mergeEntries(box, nbox);
    added.forEach(sweepOutImgs);
    mermaid(added);
    if (pin && evictEntries(box)) histReset();
  }

  // --- older history: loaded a page at a time when scrolled to the top -------
  // The region draws the newest entries; older ones come from the history
  // route and are prepended with the scroll position kept. histBefore is the
  // next page's cursor ("" asks for the newest page, so a page that drew no
  // entries can still start loading); null takes it from the box's oldest
  // entry. histReset forgets the cursor and the end once the box's oldest
  // entry changed under it (evicted, or the box started over), and its epoch
  // drops a page still in flight, which would no longer join what the box
  // holds.
  var histBefore = null, histEnd = false, histBusy = false, histEpoch = 0;
  function histReset() { histBefore = null; histEnd = false; histBusy = false; histEpoch++; }
  function loadHistory() {
    if (histEnd || histBusy || !window.fetch || !convo || !visible(convo)) return;
    var box = entriesBox();
    if (!box) return;
    if (histBefore === null) {
      var first = box.firstElementChild;
      histBefore = first ? first.getAttribute("data-entry") || "" : "";
    }
    histBusy = true;
    var epoch = histEpoch;
    var url = "/s/" + rid + "/history?n=50" + (histBefore ? "&before=" + encodeURIComponent(histBefore) : "");
    fetch(url, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      if (epoch !== histEpoch) return;
      histBusy = false;
      var h = convo.scrollHeight, top = convo.scrollTop;
      var added = mergeEntries(box, frag(res.html || ""));
      added.forEach(sweepOutImgs);
      // Keep what the user was reading in place: the prepended height goes
      // above the scroll offset.
      convo.scrollTop = top + (convo.scrollHeight - h);
      mermaid(added);
      if (res.before) histBefore = res.before;
      if (res.end || !res.before) histEnd = true;
      nearTop();
    }, function () {
      // A failed fetch (daemon replaced, network) is retried on the next
      // scroll to the top, after a pause.
      if (epoch === histEpoch) setTimeout(function () { if (epoch === histEpoch) histBusy = false; }, 3000);
    });
  }
  // nearTop loads older entries when the conversation is scrolled to (or
  // too short to scroll away from) its top.
  function nearTop() { if (convo && visible(convo) && convo.scrollTop < 160) loadHistory(); }
  if (convo) convo.addEventListener("scroll", nearTop);

  // mermaid draws every undrawn mermaid block, loading the shared script
  // once; a call while it loads is covered by the draw its onload runs, and
  // a failed load (daemon restarting) is retried by the next call. roots
  // (an element or a list of them: what a swap just inserted) limits the
  // look for undrawn blocks to them; omitted, the whole page is looked at.
  var mermaidLoading = false;
  function mermaid(roots) {
    var list = roots == null ? [doc] : Array.isArray(roots) ? roots : [roots];
    if (!window.wbMermaidFile || !list.some(function (r) { return !!r.querySelector("pre.mermaid:not([data-processed])"); })) return;
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
  // ending with the next control when more remain. The fetched pages stay
  // across board swaps (morphBoard).
  function loadClosed(btn) {
    if (!window.fetch || btn.disabled) return;
    btn.disabled = true;
    var from = +btn.getAttribute("data-closed-next");
    fetch("/s/" + rid + "/closed?from=" + from, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      var page = frag(res.html || "").firstElementChild;
      // A board swap meanwhile replaced the control, or moved the index it
      // names (a topic closed or reopened): the page fetched may not follow
      // what shows now, and the control is clicked again.
      if (!btn.parentNode || !page || +btn.getAttribute("data-closed-next") !== from) { btn.disabled = false; return; }
      btn.parentNode.replaceChild(page, btn);
      restoreOpen(page);
      if (window.wbBindBoard) window.wbBindBoard(page);
      if (window.wbAgo) window.wbAgo(page);
      mermaid(page);
    }, function () { btn.disabled = false; });
  }

  // --- composer: never inside a region, so a swap never touches it ---------
  // With a live host adapter the composer sends: the typed text plus one
  // answer chip per board item. Clicking an option, ok/no, or an
  // acknowledgement on a card attaches (or replaces) that item's chip; the
  // chips live here, so a chip outlives its card leaving the board.
  var ctext = $("ctext"), cnote = $("cnote"), cchips = $("cchips");
  var chips = {}, chipOrder = [];
  // bmentions are the board mentions the composer holds, in the order added
  // (board mentions, below).
  var bmentions = [];
  function chipList() { return chipOrder.map(function (id) { return chips[id]; }); }
  function putChip(c) { if (!chips[c.id]) chipOrder.push(c.id); chips[c.id] = c; }
  function dropChip(id) { delete chips[id]; chipOrder = chipOrder.filter(function (x) { return x !== id; }); }
  function boardItem(id) {
    var b = region("board");
    return b ? b.querySelector('.card[data-id="' + id + '"], .arow[data-id="' + id + '"], .srow[data-id="' + id + '"]') : null;
  }
  // readItem is a board item's answer as a chip; null when it has none.
  function readItem(item) {
    var pick = item.querySelector('.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]');
    var box = item.querySelector("label.ack > input"), note = item.querySelector("textarea.note");
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
    var box = item.querySelector("label.ack > input"), note = item.querySelector("textarea.note");
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
    q(".card[data-id], .arow[data-id], .srow[data-id]", b).forEach(function (item) {
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
    // A topic note is no item's answer, but a send carries it, so it shows
    // here too: "[topic] text", read from the board on every render.
    topicNotes().forEach(function (n) {
      var el = doc.createElement("span"), b = doc.createElement("b"), t = doc.createElement("span"), x = doc.createElement("button");
      el.className = "achip tchip";
      el.setAttribute("data-tchip", n.topic);
      el.title = wsT("js.chip.tnote.open");
      b.textContent = "[" + n.topic + "]";
      el.appendChild(b);
      t.className = "anote";
      t.textContent = n.value.trim();
      el.appendChild(t);
      x.className = "unchip";
      x.setAttribute("data-untchip", n.topic);
      x.title = wsT("js.chip.tnote.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    // A board mention: "[topic] (id)" and the selected text, clipped.
    bmentions.forEach(function (m, i) {
      var el = doc.createElement("span"), b = doc.createElement("b"), t = doc.createElement("span"), x = doc.createElement("button");
      el.className = "achip mchip";
      el.title = m.text;
      b.textContent = "[" + m.topic + "]" + (m.id ? " (" + m.id + ")" : "");
      el.appendChild(b);
      t.className = "anote";
      t.textContent = m.text.replace(/\s+/g, " ");
      el.appendChild(t);
      x.className = "unchip";
      x.setAttribute("data-unmchip", String(i));
      x.title = wsT("js.wbmention.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
  }
  // topicNote is the board's note for a topic, matched by attribute value so
  // a topic name needs no selector escaping.
  function topicNote(name) {
    var b = region("board");
    return (b ? q(".tnote[data-topic]", b) : []).filter(function (el) { return el.getAttribute("data-topic") === name; })[0] || null;
  }
  // clearTopicNote empties a topic's note through the input path a typed
  // edit takes, so the reply line, the chips, and holdsDraft follow.
  function clearTopicNote(name) {
    var n = topicNote(name);
    if (!n) return;
    n.value = "";
    n.dispatchEvent(new Event("input", { bubbles: true }));
  }
  // closeTopicNotes closes every open topic note, as its pen does: the text
  // stays, and the note's chip or pen opens it again. A focused note is
  // blurred first, so a board swap deferred while it was edited lands.
  function closeTopicNotes() {
    var b = region("board");
    (b ? q(".tnote[data-topic]", b) : []).forEach(function (el) {
      if (el.hidden) return;
      if (doc.activeElement === el) el.blur();
      el.hidden = true;
    });
  }
  // focusTopicNote brings a topic's note into view on the board, opening it
  // through the topic's pen when it is closed.
  function focusTopicNote(name) {
    var n = topicNote(name), topic = n && n.closest(".topic");
    if (!n) return;
    if (live && !tabShown("board")) setTab("board");
    if (topic) topic.scrollIntoView({ block: "nearest" });
    var pen = topic && topic.querySelector(".tnote-toggle");
    if (n.hidden && pen) pen.click();
    else n.focus();
  }
  // boardAnswers is the answer block the chips make (answers.js, the
  // grammar the board's copy-answers and the server's delivered message use).
  function boardAnswers() {
    if (cchips) return window.wbAnswers.block(chipList(), []);
    var b = region("board"), r = b && b.querySelector("#reply-text");
    return r ? r.value.trim() : "";
  }
  function syncNote(text) {
    if (!cnote) return;
    cnote.textContent = text || "";
  }
  function grow() { ctext.style.height = "auto"; ctext.style.height = Math.min(160, ctext.scrollHeight) + "px"; }
  // post sends a write route's JSON body; done(ok, answer) gets the parsed
  // answer, or the server's refusal text. A route is the session's own
  // unless it starts with "/", a daemon-wide route.
  function post(route, body, done) {
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch(route.charAt(0) === "/" ? route : "/s/" + rid + "/" + route, {
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
  // --- board mentions: selected board text added to the message ------------
  // On the live board, selecting text inside one topic card shows a floating
  // mention button by the selection's end; clicking it adds a mention
  // capsule (renderChips), which a send carries and the delivered message
  // lists after its answer block (wsinbox mentions). A topic body is a
  // sandboxed frame whose selection this page cannot read, so the frame
  // reports it ({wbDoc: "sel"}, doc-frame.js) and the button stands over it.
  var mbtn = null, mpend = null, mraf = 0;
  // mentionOf is the mention a selection makes: the topic card's key, the
  // item ID when both ends of the selection lie in the same item card ("" for
  // an end outside every item), and the selected text; null for a blank
  // selection, since a mention points at content.
  function mentionOf(topic, startId, endId, text) {
    text = (text || "").trim();
    if (!topic || !text) return null;
    var m = { topic: topic, text: text };
    if (startId && startId === endId) m.id = startId;
    return m;
  }
  function selEl(n) { return n && (n.nodeType === 1 ? n : n.parentNode); }
  // itemIdAt is the ID of the item card n lies in, "" outside every card; the
  // card's element id carries it on every kind (a settled row has no data-id).
  function itemIdAt(n) {
    var el = selEl(n), item = el && el.closest && el.closest(".card[id], .arow[id], .srow[id]");
    var m = item && /^item-([0-9]{1,6})$/.exec(item.getAttribute("id") || "");
    return m ? m[1] : "";
  }
  // endRect is where a range ends on screen: its last line box.
  function endRect(r) {
    var rs = r.getClientRects();
    return rs.length ? rs[rs.length - 1] : r.getBoundingClientRect();
  }
  // hostMention is the pending mention of this page's own selection, null
  // unless it lies in one topic card of the board, outside a form field.
  function hostMention() {
    var sel = window.getSelection ? window.getSelection() : null, b = region("board");
    if (!b || !sel || sel.isCollapsed || !sel.rangeCount) return null;
    var a = selEl(sel.anchorNode), f = selEl(sel.focusNode), act = doc.activeElement;
    if (!a || !f || !b.contains(a) || !b.contains(f)) return null;
    if (act && /^(TEXTAREA|INPUT)$/.test(act.tagName) && b.contains(act)) return null;
    var ta = a.closest("section.topic[data-topic]");
    if (!ta || ta !== f.closest("section.topic[data-topic]")) return null;
    var m = mentionOf(ta.getAttribute("data-topic"), itemIdAt(sel.anchorNode), itemIdAt(sel.focusNode), sel.toString());
    if (!m) return null;
    var r = endRect(sel.getRangeAt(0));
    return { m: m, x: r.right, y: r.bottom };
  }
  function hideMention() {
    mpend = null;
    if (mbtn) mbtn.hidden = true;
  }
  // placeMention shows the button for p by its selection's end, kept on
  // screen; a frame's selection stands where its frame stands now.
  function placeMention(p) {
    var x = p.x, y = p.y;
    if (p.frame) {
      if (!doc.contains(p.frame)) { hideMention(); return; }
      var fr = p.frame.getBoundingClientRect();
      x += fr.left; y += fr.top;
    }
    if (!mbtn) {
      mbtn = doc.createElement("button");
      mbtn.type = "button";
      mbtn.className = "wbmention";
      mbtn.setAttribute("data-wbmention", "");
      mbtn.textContent = wsT("js.wbmention.add");
      mbtn.title = wsT("js.wbmention.add.title");
      // Pressing the button must not move the selection it adds.
      mbtn.addEventListener("mousedown", function (e) { e.preventDefault(); });
      doc.body.appendChild(mbtn);
    }
    mpend = p;
    mbtn.hidden = false;
    var w = mbtn.offsetWidth, h = mbtn.offsetHeight;
    mbtn.style.left = Math.max(4, Math.min(window.innerWidth - w - 4, x + 4)) + "px";
    mbtn.style.top = Math.max(4, Math.min(window.innerHeight - h - 4, y + 6)) + "px";
  }
  function syncHostMention() {
    mraf = 0;
    var p = hostMention();
    if (p) placeMention(p);
    else if (mpend && !mpend.frame) hideMention();
  }
  function addMention() {
    var p = mpend;
    hideMention();
    if (!p) return;
    bmentions.push(p.m);
    if (p.frame) { if (p.frame.contentWindow) p.frame.contentWindow.postMessage({ wbDoc: "unsel" }, "*"); }
    else if (window.getSelection) window.getSelection().removeAllRanges();
    renderChips();
    syncNote();
  }
  // frameMention takes a body frame's selection report: its text and where
  // its end stands inside the frame.
  function frameMention(f, text, x, y) {
    var t = f.closest("section.topic[data-topic]"), m = t && mentionOf(t.getAttribute("data-topic"), "", "", text);
    if (m) placeMention({ m: m, frame: f, x: x, y: y });
    else if (mpend && mpend.frame === f) hideMention();
  }
  if (cchips) {
    doc.addEventListener("selectionchange", function () {
      if (!mraf) mraf = requestAnimationFrame(syncHostMention);
    });
    // A press anywhere on this page leaves a frame's selection behind.
    doc.addEventListener("mousedown", function (e) {
      if (mpend && mpend.frame && !(e.target.closest && e.target.closest("[data-wbmention]"))) hideMention();
    }, true);
    // The button follows its selection when the board scrolls or the
    // window resizes.
    var follow = function () {
      if (!mpend) return;
      if (mpend.frame) { placeMention(mpend); return; }
      var p = hostMention();
      if (p) placeMention(p); else hideMention();
    };
    window.addEventListener("scroll", follow, true);
    window.addEventListener("resize", follow);
  }
  var sending = false;
  // topicNotes are the board's non-empty topic notes. A send carries them
  // as its topics, which the delivered message's answer block lists after
  // the item answers; they are no item's answer, so they are no answer
  // chip, and renderChips shows each as a topic-note chip.
  function topicNotes() {
    var b = region("board");
    return (b ? q(".tnote[data-topic]", b) : []).map(function (el) {
      return { el: el, topic: el.getAttribute("data-topic"), value: el.value };
    }).filter(function (n) { return n.value.trim(); });
  }
  function sendMessage() {
    if (sending) return;
    var raw = ctext.value.trim(), t = sendText().trim(), list = chipList(), pics = imgs.slice(), notes = topicNotes(), said = bmentions.slice();
    if (!t && !list.length && !pics.length && !notes.length && !said.length) { syncNote(wsT("js.send.empty")); return; }
    if (pics.some(function (p) { return !p.ref && p.data === null; })) { syncNote(wsT("js.send.reading")); return; }
    var body = { text: t, chips: list, topics: notes.map(function (n) { return { topic: n.topic, note: n.value.trim() }; }), mentions: said };
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
      // A mention added during the send stays for the next one.
      bmentions = bmentions.filter(function (m) { return said.indexOf(m) < 0; });
      // A topic note edited during the send stays for the next one.
      notes.forEach(function (n) {
        if (n.el.value !== n.value) return;
        n.el.value = "";
        n.el.dispatchEvent(new Event("input", { bubbles: true }));
      });
      // A send closes the open notes; one edited during it keeps its text.
      closeTopicNotes();
      renderChips();
      // The sent draft is gone; what was typed during the send is saved
      // again as the next draft.
      clearDraft();
      syncNote(wsT(res.merged ? "js.send.merged" : "js.send.ok"));
    });
  }
  // validMentions keeps the well-formed mentions of a list a cancel or a
  // stored draft returned.
  function validMentions(list) {
    return (Array.isArray(list) ? list : []).filter(function (m) {
      return m && typeof m.topic === "string" && m.topic && typeof m.text === "string" && m.text;
    }).map(function (m) { return m.id ? { topic: m.topic, id: String(m.id), text: m.text } : { topic: m.topic, text: m.text }; });
  }
  // restoreHeld puts held messages a cancel or an interrupt took back into
  // the composer, oldest first, ahead of the draft: their texts joined as
  // paragraphs, their answer chips, their mentions, their topic notes back
  // on the board's topics (restoreTopics), and their images as chips that
  // reference the written files, with their markers in the returned text. Each
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
      var stray = restoreTopics(res.topics || []);
      if (stray.length) text += (text ? "\n\n" : "") + stray.join("\n");
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
    // Their mentions go back ahead of the draft's, oldest message first.
    var held = [];
    list.forEach(function (res) { held = held.concat(validMentions(res.mentions)); });
    bmentions = held.concat(bmentions);
    renderChips();
    renderImgs();
  }
  // restoreTopics puts returned topic notes back on their topics' notes,
  // after any text a note holds now, through the input path a typed edit
  // takes; it returns a "[topic] note" line for each topic no longer on the
  // board, which the caller keeps in the composer text so no note is lost.
  function restoreTopics(list) {
    var stray = [];
    list.forEach(function (tn) {
      var n = topicNote(tn.topic), note = (tn.note || "").trim();
      if (!note) return;
      if (!n) { stray.push("[" + tn.topic + "] " + note); return; }
      var cur = n.value.trim();
      if (cur === note || cur.indexOf(note) >= 0) return;
      openNote(n, { note: cur ? cur + "\n" + note : note });
      n.dispatchEvent(new Event("input", { bubbles: true }));
    });
    return stray;
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
  // held message back into the composer. It then stays disabled until that
  // turn ends, so a second click cannot cut the next turn: until the phase
  // leaves the working set or the region's data-turn (the server's
  // turn-start stamp) changes from the one seen at the click. The stamp is
  // what catches a next turn that starts at once (held input or mail
  // delivered at turn end), which no page swap ever shows as idle.
  var ibadge = $("ibadge"), ilabel = ibadge && ibadge.querySelector(".ilabel");
  var WORKING = { thinking: 1, replying: 1, tool: 1, permission: 1 };
  var liveMode = mode === "live", interrupting = false, interruptedTurn = "";
  function leadWorking() {
    var c = doc.querySelector("#r-convlive .clive");
    return !!c && WORKING[c.getAttribute("data-phase")] === 1;
  }
  function leadTurn() {
    var c = doc.querySelector("#r-convlive .clive");
    return (c && c.getAttribute("data-turn")) || "";
  }
  function syncBadge() {
    subBadge(sbadge, doc.querySelector("#r-convlive .clive"), liveMode);
    bgBadge(doc.querySelector("#r-convlive .clive"));
    if (!ibadge) return;
    var on = liveMode && leadWorking();
    if (!on || leadTurn() !== interruptedTurn) interrupting = false;
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
  // server; "(+M)" shows only when M is above zero. on gates it: the
  // session page's live mode, or the support chat card's open conversation.
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
  // --- background-task badge ---------------------------------------------------
  // Beside the subagent badge: the clock and the count while the harness of
  // a session the daemon runs reports running background tasks (its shells,
  // monitors, and background subagents), from the convlive region's
  // data-bg; zero, or a session with no such report, hides it. The column
  // it shares with the subagent badge is too narrow for both full labels,
  // so "N background tasks" is its title and accessible name (the agents
  // panel head shows it in full).
  var bgbadge = $("bgbadge"), bgTitle = bgbadge ? bgbadge.title : "";
  function bgBadge(clive) {
    if (!bgbadge) return;
    var n = clive ? Number(clive.getAttribute("data-bg")) || 0 : 0;
    bgbadge.hidden = !(liveMode && n > 0);
    if (bgbadge.hidden) return;
    var label = wsTn("js.bgtasks", n), el = bgbadge.querySelector(".blabel");
    if (el && el.textContent !== String(n)) el.textContent = String(n);
    if (bgbadge.getAttribute("aria-label") !== label) {
      bgbadge.setAttribute("aria-label", label);
      bgbadge.title = label + (bgTitle ? " \u00b7 " + bgTitle : "");
    }
  }
  function interruptTurn() {
    if (interrupting) return;
    interrupting = true;
    interruptedTurn = leadTurn();
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
  // re-renders it answered; each run recomputes both from scratch. A run
  // over the same records as the last (sentSig) leaves the board alone unless
  // a board swap asks (force): putting back the same classes would only wake
  // the open-question pill's class observer.
  var sentSig = null;
  function applySent(force) {
    var b = region("board");
    if (!b) return;
    var sig = q(".sentdata i[data-id]").map(function (i) {
      return [i.getAttribute("data-id"), i.getAttribute("data-state"), i.getAttribute("data-answer"), i.getAttribute("data-note")].join("\u0001");
    }).join("\u0002");
    if (!force && sig === sentSig) return;
    sentSig = sig;
    q(".sentmark", b).forEach(function (m) { m.parentNode.removeChild(m); });
    q(".sent-sent, .sent-held", b).forEach(function (el) { el.classList.remove("sent-sent", "sent-held"); });
    q(".sentdata i[data-id]").forEach(function (i) {
      var item = boardItem(i.getAttribute("data-id")), head = item && item.querySelector(".head, .stext");
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
    u = t.closest("[data-untchip]");
    if (u) { clearTopicNote(u.getAttribute("data-untchip")); renderChips(); syncNote(); return; }
    u = t.closest("[data-unmchip]");
    if (u) { bmentions.splice(+u.getAttribute("data-unmchip"), 1); renderChips(); syncNote(); return; }
    if (t.closest("[data-wbmention]")) { addMention(); return; }
    u = t.closest("[data-tchip]");
    if (u) { focusTopicNote(u.getAttribute("data-tchip")); return; }
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
    b = t.closest("#ctabs [data-ctab]");
    if (b) { setCtab(b.getAttribute("data-ctab")); return; }
  });

  // --- link actions: the menu a served page's marked links open ------------
  // The renderer marks a served link data-wl="file" (href: the file view, or
  // no href outside the session root; data-rel, data-abs) or data-wl="url";
  // no other anchor is touched. An image file link is also marked data-img;
  // an absolute image path outside the root is marked data-out, with the
  // image route as its href, which the daemon serves only to a browser on
  // its own machine. A plain click opens the menu, or, on a file link the
  // preview card can show (previews), the preview slot, with the menu on
  // the right click and the keyboard activation instead; Cmd/Ctrl+click on a link
  // the page can open is left to the browser, which opens a new tab, and
  // opens the menu otherwise. The system-browser and native-app items show
  // only on a page the server marked data-sysopen (a browser on the daemon's
  // machine), and a data-out link opens at all only there; the daemon checks
  // both again. The markup is the same for every viewer: entries render once
  // for all of them.
  var lmenu = null, lmenuFrom = null;
  function closeLinkMenu(refocus) {
    if (!lmenu) return;
    lmenu.parentNode.removeChild(lmenu);
    lmenu = null;
    if (refocus && lmenuFrom && lmenuFrom.isConnected) lmenuFrom.focus();
    lmenuFrom = null;
  }
  function toast(text) { if (window.WsToast) window.WsToast.show(text); }
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
  // --- session label: the head's name -----------------------------------------
  // The head's name (sname, dashboard.tmpl) shows the session's display
  // label, or its reply-id when it has none; a click on it copies the
  // session's mailbox address, which the server builds (data-copy). The
  // pencil beside it turns the name into a one-line input: Enter saves
  // through the label route (label_route.go), Escape or leaving the input
  // cancels (the tracker's in-place title edit also cancels on Escape), and
  // an empty value clears the label, which shows the reply-id again. A head
  // swap that arrives while the input is open waits (headHeld) and lands
  // when the edit ends, so a stream update never drops the typing.
  var headHeld = null;
  function labelEditing() {
    var h = region("head");
    return !!(h && h.querySelector(".sinp"));
  }
  // labelClean is the value the route stores: one line without control
  // characters, trimmed, at most max characters.
  function labelClean(s, max) {
    var v = String(s).replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]+/g, " ").trim();
    return Array.from(v).slice(0, max).join("").trim();
  }
  // labelClick handles a click inside the head: the name copies, the pencil
  // starts the rename.
  function labelClick(t) {
    var sn = t && t.closest ? t.closest(".sname") : null;
    if (!sn) return;
    if (t.closest(".snm")) copyText(t.closest(".snm").getAttribute("data-copy"));
    else if (t.closest(".sren")) labelStart(sn);
  }
  function labelStart(sn) {
    if (sn.querySelector(".sinp")) return;
    var nm = sn.querySelector(".snm"), pen = sn.querySelector(".sren"), inp = doc.createElement("input");
    inp.type = "text";
    inp.className = "sinp";
    inp.value = sn.getAttribute("data-label") || "";
    inp.placeholder = root.getAttribute("data-rid") || "";
    inp.maxLength = +sn.getAttribute("data-max") || 64;
    inp.spellcheck = false;
    inp.setAttribute("autocomplete", "off");
    inp.setAttribute("aria-label", pen.getAttribute("aria-label") || "");
    nm.hidden = pen.hidden = true;
    sn.insertBefore(inp, nm);
    inp.addEventListener("keydown", function (e) { labelKey(sn, inp, e); });
    inp.addEventListener("blur", function () { if (!inp.saving) labelEnd(sn); });
    inp.focus();
    inp.select();
  }
  function labelKey(sn, inp, e) {
    if (e.isComposing || e.keyCode === 229) return;
    if (e.key === "Enter") { e.preventDefault(); labelSave(sn, inp); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); labelEnd(sn); }
  }
  // labelSave posts the input's value; a refusal toasts why and keeps the
  // input open. An unchanged value just closes it.
  function labelSave(sn, inp) {
    if (inp.saving) return;
    var v = labelClean(inp.value, inp.maxLength);
    if (v === (sn.getAttribute("data-label") || "")) { labelEnd(sn); return; }
    inp.saving = true;
    post("label", { label: v }, function (ok, ans) {
      inp.saving = false;
      if (!ok) {
        toast(wsT("js.label.fail", ans));
        if (inp.isConnected) inp.focus();
        return;
      }
      var label = typeof ans.label === "string" ? ans.label : v;
      sn.setAttribute("data-label", label);
      sn.querySelector(".snm").textContent = label || root.getAttribute("data-rid") || "";
      labelEnd(sn);
    });
  }
  // labelEnd closes the input and lands a head swap it held. Removing the
  // focused input fires its blur, which comes back here: ending marks the
  // input already on its way out.
  function labelEnd(sn) {
    var inp = sn.querySelector(".sinp");
    if (!inp || inp.ending) return;
    inp.ending = true;
    sn.removeChild(inp);
    sn.querySelector(".snm").hidden = sn.querySelector(".sren").hidden = false;
    if (headHeld) {
      var d = headHeld;
      headHeld = null;
      swap("head", d.hash, d.html);
    }
  }
  // labelSwapped retitles the tab after a head swap, from the title the
  // head's name carries.
  function labelSwapped() {
    var h = region("head"), sn = h && h.querySelector(".sname[data-title]");
    if (sn) tlRebase(sn.getAttribute("data-title"));
  }
  doc.addEventListener("click", function (e) {
    var h = region("head");
    if (h && h.contains(e.target)) labelClick(e.target);
  });
  // linkOpens reports whether this page can open a marked link's href: a
  // data-out link only on a data-sysopen page.
  function linkOpens(a) {
    return !!a.getAttribute("href") && (!a.hasAttribute("data-out") || root.hasAttribute("data-sysopen"));
  }
  // previews reports a file link the preview card (preview.js) can show,
  // decided from what the page already knows: a file inside the session
  // root whose extension is not one no preview shows. What the server then
  // finds binary or too large opens the card in its "not viewable" state.
  function previews(a) {
    var rel = a.getAttribute("data-rel");
    return !!window.WsPreview && !!cardBoard && a.getAttribute("data-wl") === "file" && !!a.getAttribute("href") &&
      !a.hasAttribute("data-out") && !!rel && window.WsPreview.takes(rel);
  }
  function openLinkMenu(a, x, y) {
    closeLinkMenu(false);
    var kind = a.getAttribute("data-wl"), href = a.getAttribute("href");
    var rel = a.getAttribute("data-rel"), abs = a.getAttribute("data-abs"), url = a.href;
    var sys = root.hasAttribute("data-sysopen"), out = a.hasAttribute("data-out");
    var items = [];
    if (previews(a)) items.push([wsT("js.menu.preview"), function () { window.WsPreview.open(rel); }]);
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
    // A pointer click on a link the preview can show opens the preview
    // slot; its menu is on the right click and on the keyboard activation
    // (a click event with no detail).
    if (e.detail !== 0 && previews(a)) { window.WsPreview.open(a.getAttribute("data-rel")); return; }
    linkAt(a, e);
  }, true);
  doc.addEventListener("contextmenu", function (e) {
    var a = e.target && e.target.closest && e.target.closest("a[data-wl]");
    if (!a || !previews(a)) return;
    e.preventDefault();
    openLinkMenu(a, e.clientX, e.clientY);
  });
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
  // kept. Both handles drag (drDragStart, below).
  //
  // One height for every drawer: drH is the height the panels, the git
  // sheet, and each tracker store's drawer open at (the tracker reads and
  // sets it through its drawerHeight option), so dragging any of their
  // handles sets it for all. Each drawer clamps it with its own limits when
  // it shows; the stored value is the drag's result.
  //
  // While a sheet shows, the fold button (#drfold, built below) sits in the
  // slot its riding handle left at the top, at that handle's width (--drw,
  // --ghw), and closes the sheet as a click on the handle does.
  var drawer = $("drawer"), drclip = $("drclip"), drsheet = $("drsheet"), drbody = $("drbody");
  var drhandle = $("drhandle"), drheader = doc.querySelector(".dwrap > .top");
  var gbody = $("gbody"), gitRegion = region("git");
  var DR_OPEN = "ws.dashboard.drawer", DR_H = "ws.dashboard.drawer.h";
  var DR_MIN = 160, DR_DEF = 320, DR_SHUT = 80;
  var drRoomW = -1, drGhOff = -1, drEntries = [], drWant = false, drOpen = false, drHas = false, drH = DR_DEF, drTop = 0;
  var drMode = "panels", drShown = "", drfold = null, drFoldW = -1, drFoldGw = -1;
  // The stored height is read with or without the frame: a page with no
  // session drawer still shares it with the tracker's store drawers.
  (function () { var h = parseFloat(load(DR_H)); if (isFinite(h) && h > 0) drH = h; })();
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
  cardTab("settings");
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
  // drMax is the tallest sheet: 80% of the window, and short of the
  // composer with the handle below it.
  function drMax() {
    var m = Math.round(window.innerHeight * 0.8), c = $("composer");
    if (visible(c)) m = Math.min(m, Math.round(c.getBoundingClientRect().top) - drTop - drhandle.offsetHeight - 6);
    return Math.max(0, m);
  }
  function drClamp(h) { var hi = drMax(); return Math.max(Math.min(DR_MIN, hi), Math.min(hi, h)); }
  // drSetH sets the one drawer height (a drag's release, here or on a
  // tracker store's handle) and keeps it.
  function drSetH(h) { drH = h; save(DR_H, String(h)); }
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
      if (shown === "panels") { wtLoad(); hdLoad(); }
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
  // drFold is a click on the fold button: the shown sheet closes as a click
  // on its handle closes it, and keyboard focus goes to that handle, since
  // the button hides.
  function drFold() {
    if (drDragged || !drOpen) return;
    var back = drMode === "git" && ghandle() || drhandle, kb = doc.activeElement === drfold;
    drSetOpen(false);
    if (kb) back.focus();
  }
  if (drawer) {
    // The fold button, ahead of the handles so a riding handle passes over it.
    drfold = doc.createElement("button");
    drfold.type = "button";
    drfold.className = "dr-fold";
    drfold.id = "drfold";
    drfold.hidden = true;
    drfold.title = wsT("js.drawer.fold");
    drfold.setAttribute("aria-label", drfold.title);
    drfold.setAttribute("aria-controls", "drsheet");
    drfold.appendChild(luIcon("chevron-up"));
    drhandle.parentNode.insertBefore(drfold, drhandle);
    drfold.addEventListener("click", drFold);
    drWant = load(DR_OPEN) === "1";
    drawerSync();
    drawerWait();
    // drag: either handle follows the pointer (the git handle with its
    // region); past the header's bottom the sheet comes with it, and the
    // release sets the one height or, short of DR_SHUT, closes. A press that
    // never moves is a click. The git handle is found per press, since a
    // region swap replaces it, and the moves and the release are heard on
    // the document.
    var drDrag = null, drDragged = false;
    function drDragStart(e) {
      var h = e.target.closest && e.target.closest("#drhandle, #ghandle");
      if (!h || (e.button !== undefined && e.button !== 0)) return;
      var git = h.id === "ghandle", rider = git ? gitRegion : drhandle;
      drMeasure();
      drDrag = { mode: git ? "git" : "panels", rider: rider, id: e.pointerId, y0: e.clientY, off: e.clientY - rider.getBoundingClientRect().top, moved: false, ext: 0 };
      if (!git) try { h.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the moves still land */ }
    }
    function drDragMove(e) {
      var d = drDrag;
      if (!d) return;
      // A move with the primary button up is a release this document never
      // heard: a flick on the git handle, before its drag takes capture, can
      // send the first move and the release to a topic frame. The drag ends
      // there and the sheet goes back as it stood, since that release's
      // position is unknown.
      if (e.buttons !== undefined && (e.buttons & 1) === 0) { drAbort(); return; }
      if (!d.moved && Math.abs(e.clientY - d.y0) < 5) return;
      // The git drag captures on its region once it moves: the region
      // outlives a swap of the handle, which would drop a capture on the
      // handle and leave the moves to whatever is under the pointer (a topic
      // body's frame keeps them). A press that never moves keeps its click
      // on the handle.
      if (!d.moved && d.mode === "git") try { d.rider.setPointerCapture(d.id); } catch (x) { /* no capture: the moves still land */ }
      if (!d.moved && !drOpen) drAnnounce();
      // Dragging one sheet's handle over the other open sheet brings its
      // own sheet in that one's place.
      if (!d.moved && drMode !== d.mode) { drMode = d.mode; drApply(false); }
      if (!d.moved) drFoldShow(d.mode);
      d.moved = true;
      var y = e.clientY - d.off, max = drMax(), ext = Math.max(0, Math.min(max, y - drTop));
      d.ext = ext;
      drawer.classList.remove("anim");
      drawer.classList.add("drag");
      drsheet.style.height = Math.max(Math.min(DR_MIN, max), ext) + "px";
      drclip.style.height = ext + "px";
      d.rider.style.top = (ext > 0 ? drTop + ext : Math.max(0, Math.min(y, drTop))) + "px";
    }
    // drAbort ends a drag without its release: a moved one restores the
    // layout drApply draws for the state before the drag.
    function drAbort() {
      var d = drDrag;
      drDrag = null;
      if (!d || !d.moved) return;
      drawer.classList.remove("drag");
      drApply(false);
    }
    function drEnd() {
      var d = drDrag;
      drDrag = null;
      if (!d || !d.moved) return; // a press that stayed put is a click
      drawer.classList.remove("drag");
      drDragged = true;
      setTimeout(function () { drDragged = false; }, 0);
      if (d.ext < DR_SHUT) { drSetOpen(false); return; }
      drSetH(drClamp(d.ext));
      drMode = d.mode;
      drOpen = true;
      drWant = d.mode === "panels";
      save(DR_OPEN, drWant ? "1" : null);
      if (drWant) { drSwap = false; drDot(); }
      drApply(false);
    }
    doc.addEventListener("pointerdown", drDragStart);
    doc.addEventListener("pointermove", drDragMove);
    doc.addEventListener("pointerup", drEnd);
    doc.addEventListener("pointercancel", drEnd);
    drhandle.addEventListener("click", function () {
      if (drDragged) return;
      drSetOpen(!(drOpen && drMode === "panels"), "panels");
    });
    // The git handle toggles the git sheet, or brings it in place of the
    // panels; it is looked up per click, since a swap replaces it.
    doc.addEventListener("click", function (e) {
      if (drDragged || !e.target.closest || !e.target.closest("#ghandle")) return;
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
      if (t.closest("#drclip, #drhandle, #drfold, #r-git, .lmenu, .gwin-back")) return;
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
  // (doc-frame.js) posts its content height, its marked-link clicks, its
  // item-reference clicks, and its requests for an image outside the
  // session root (relayImage). A message counts only when its source is a body
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
  // relayHref is the URL the host fetches for a frame's image request
  // ({wbDoc: "img"}): href itself, only when it is on list, the frame
  // element's own data-relay (wswhiteboard framedDoc, which the body's markup
  // cannot reach), and only as this session's image route for an image file
  // name with no other query part; "" otherwise.
  function relayHref(list, href) {
    var pre = "/s/" + rid + "/image?path=", path;
    if (!rid || !href || typeof href !== "string" || (" " + list + " ").indexOf(" " + href + " ") < 0) return "";
    if (href.indexOf(pre) !== 0 || /[\s&#]/.test(href)) return "";
    try { path = decodeURIComponent(href.slice(pre.length).replace(/\+/g, " ")); } catch (x) { return ""; }
    return /\.(avif|bmp|gif|ico|jpe?g|png|svg|webp)$/i.test(path) ? href : "";
  }
  // relayImage answers a topic-body frame's image request: the frame's
  // opaque origin cannot load an image outside the session root from the
  // image route, which refuses it a cross-site request, so this page, same
  // origin and served that image only for a loopback viewer, fetches it and
  // posts the frame the bytes as a Blob. This is a security boundary between
  // a body's scripts and this page's origin, and it stays narrow: only a
  // message from one of this page's own body frames (docFrame), only a URL
  // relayHref allows for that frame, only an answer that is an image, and
  // only back to the frame that asked. It is never a generic fetch relay. A
  // failed fetch (a LAN viewer, a missing file) answers nothing, and the
  // frame keeps the path text.
  function relayImage(f, m) {
    var href = relayHref(f.getAttribute("data-relay") || "", m.href), id = m.id;
    if (!href || typeof id !== "number" || !window.fetch) return;
    // A frame asks again when the host asks for its height, in case its
    // first request went out before this listener; each frame element
    // fetches each of its requests once.
    var asked = f.wbRelayAsked || (f.wbRelayAsked = {});
    if (asked[id]) return;
    asked[id] = true;
    fetch(href, { credentials: "same-origin", redirect: "error" }).then(function (r) {
      var type = (r.headers.get("Content-Type") || "").split(";")[0].trim().toLowerCase();
      return r.ok && /^image\/[a-z0-9.+-]+$/.test(type) ? r.blob() : null;
    }).then(function (b) {
      if (b && f.contentWindow) f.contentWindow.postMessage({ wbDoc: "img", id: id, blob: b }, "*");
    }, function () {});
  }
  window.addEventListener("message", function (e) {
    var f = docFrame(e.source), m = e.data;
    if (!f || !m || typeof m !== "object") return;
    if (m.wbDoc === "height") {
      var h = num(m.height);
      // A pending frame (frameGraft) keeps the old frame's height: once
      // shown it is asked for its height again.
      if (f.wbNext) { postTheme(f); return; }
      if (h >= 0 && h < 100000) {
        var from = f.getBoundingClientRect().height, to = Math.ceil(h);
        f.style.height = to + "px";
        // The first height a frame of a moving board swap reports eases
        // from the height the frame stood at (board motion, slideBoard).
        if (f.wbGrow) {
          f.wbGrow = false;
          if (f.animate && Math.abs(to - from) >= 1) f.animate([{ height: from + "px" }, { height: to + "px" }], { duration: 200, easing: "ease-out" });
        }
      }
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
    } else if (m.wbDoc === "img") {
      relayImage(f, m);
    } else if (m.wbDoc === "sel") {
      if (cchips) frameMention(f, str(m.text), num(m.x), num(m.y));
    }
  });
  // A swap re-creates a changed topic's frame (or loads its next one beside
  // the old one, frameGraft), which starts at no height until it posts one;
  // it takes the old frame of the same topic's height meanwhile. A pending
  // next frame's height is not the topic's.
  function frameHeights(el) {
    var h = {};
    q("iframe.doc-frame:not(.doc-frame-next)", el).forEach(function (f) { if (f.style.height) h[f.getAttribute("data-doc")] = f.style.height; });
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

  // --- session holders: the drawer card over the holders route ------------
  // Each drawer opening reads the holders (holders.go): a row for each
  // holder no daemon is attached to and each one that is not restart-safe,
  // then an error row for each failed holder start. An unattached row has a
  // stop button when the route lets this viewer stop one (a browser on this
  // machine); it ends the holder and its child, then reads the list again.
  // The daemon itself never ends a holder.
  //
  // On a page that offers terminals (terminal.js), the card also lists the
  // live terminals whose opening session is gone from the index: attach
  // opens one as a card on this page, stop ends it after a confirm.
  var HD_CAUSE = { no_answer: "js.hd.cause.noanswer", protocol: "js.hd.cause.protocol", token: "js.hd.cause.token" };
  function hdLoad() {
    if (!hdlist || !window.fetch) return;
    var seq = ++hdSeq;
    if (!hdlist.firstChild) hdlist.textContent = wsT("js.hd.loading");
    fetch("/holders", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== hdSeq) return;
        if (!r.ok) { hdFail(s.trim() || "HTTP " + r.status); return; }
        try { hdDraw(JSON.parse(s)); } catch (e) { hdFail("malformed answer"); return; }
        hdTerms(seq);
      });
    }, function () { if (seq === hdSeq) hdFail(wsT("js.post.unreachable")); });
  }
  function hdTerms(seq) {
    if (!window.WsTerm) return;
    window.WsTerm.list(function (err, v) {
      if (seq !== hdSeq || err || !v || !(v.orphans || []).length) return;
      var box = mk("div", "hdterms");
      box.appendChild(mk("div", "hdsub", wsT("js.hd.term.title")));
      v.orphans.forEach(function (t) { box.appendChild(hdTermRow(t)); });
      hdlist.appendChild(box);
    });
  }
  function hdTermRow(t) {
    var row = mk("div", "wtrow hdrow hdterm"), meta = mk("span", "hdmeta"), acts = mk("span", "hdbtns");
    row.title = t.id;
    row.appendChild(mk("span", "wtdot"));
    var name = t.cwd ? t.cwd.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || t.cwd : t.id;
    row.appendChild(mk("b", "wtname", name));
    if (t.started_at) {
      var age = mk("span", "wtage", wtAge(t.started_at));
      age.title = wsT("js.hd.started", wtStamp(t.started_at));
      row.appendChild(age);
    }
    var cwd = mk("span", "hdcwd", t.cwd || "");
    cwd.title = t.cwd || "";
    meta.appendChild(cwd);
    if (t.state === "exited") meta.appendChild(mk("span", null, wsT("js.term.state.exited")));
    row.appendChild(meta);
    var at = mk("button", "btn hdstop", wsT("js.hd.term.attach"));
    at.type = "button";
    at.title = wsT("js.hd.term.attach.title");
    at.addEventListener("click", function () { window.WsTerm.attach(t); });
    var b = mk("button", "btn hdstop", wsT("js.hd.stop"));
    b.type = "button";
    b.title = wsT("js.hd.term.stop.title");
    b.addEventListener("click", function () {
      if (b.disabled || (t.state !== "exited" && !window.confirm(wsT("js.term.confirm")))) return;
      b.disabled = true;
      if (hdnote) hdnote.textContent = "";
      post("terminals/" + encodeURIComponent(t.id) + "/close", {}, function (ok, res) {
        b.disabled = false;
        if (!ok) { if (hdnote) hdnote.textContent = wsT("js.hd.stopfail", res); return; }
        hdLoad();
      });
    });
    acts.appendChild(at);
    acts.appendChild(b);
    row.appendChild(acts);
    return row;
  }
  function hdFail(msg) {
    hdlist.textContent = "";
    hdlist.appendChild(mk("div", "wtempty", wsT("js.hd.fail", msg)));
  }
  // hdLabel is a holder's session label, or its id without one.
  function hdLabel(h) {
    var l = h.session && typeof h.session.label === "string" ? h.session.label.trim() : "";
    return l || h.id;
  }
  function hdRow(h, canStop) {
    var row = mk("div", "wtrow hdrow" + (h.attached ? "" : " lost")), meta = mk("span", "hdmeta");
    row.title = h.id;
    row.appendChild(mk("span", "wtdot"));
    row.appendChild(mk("b", "wtname", hdLabel(h)));
    if (h.started_at) {
      var age = mk("span", "wtage", wtAge(h.started_at));
      age.title = wsT("js.hd.started", wtStamp(h.started_at));
      row.appendChild(age);
    }
    meta.appendChild(mk("span", null, wsT("js.hd.pid", h.pid)));
    if (!h.attached) meta.appendChild(mk("span", "hdcause", wsT(HD_CAUSE[h.cause] || "js.hd.cause.unknown", h.protocol)));
    if (h.restart_safe === false) {
      var rs = mk("span", "hdunsafe", wsT("js.hd.unsafe"));
      rs.title = wsT("js.hd.unsafe.title");
      meta.appendChild(rs);
    }
    row.appendChild(meta);
    if (!h.attached && canStop) {
      var b = mk("button", "btn hdstop", wsT("js.hd.stop"));
      b.type = "button";
      b.title = wsT("js.hd.stop.title");
      b.addEventListener("click", function () { hdStop(h.id, b); });
      row.appendChild(b);
    }
    return row;
  }
  function hdFailRow(f) {
    var row = mk("div", "wtrow hdrow hdfail");
    row.appendChild(mk("span", "wtdot"));
    var msg = mk("span", "hdmsg", f.message);
    msg.title = f.message;
    row.appendChild(msg);
    if (f.at) {
      var age = mk("span", "wtage", wtAge(f.at));
      age.title = wsT("js.hd.failed", wtStamp(f.at));
      row.appendChild(age);
    }
    return row;
  }
  function hdDraw(list) {
    var holders = list.holders || [], failures = list.failures || [], lost = 0;
    hdlist.textContent = "";
    holders.forEach(function (h) {
      if (!h.attached) lost++;
      if (!h.attached || h.restart_safe === false) hdlist.appendChild(hdRow(h, !!list.can_stop));
    });
    if (!lost) hdlist.insertBefore(mk("div", "wtempty", wsT("js.hd.empty")), hdlist.firstChild);
    failures.forEach(function (f) { hdlist.appendChild(hdFailRow(f)); });
    if (hdcount) {
      hdcount.hidden = !lost;
      hdcount.textContent = String(lost);
      hdcount.title = wsTn("js.hd.count", lost);
    }
  }
  function hdStop(id, b) {
    if (b.disabled) return;
    b.disabled = true;
    if (hdnote) hdnote.textContent = "";
    post("/holders/" + encodeURIComponent(id) + "/stop", {}, function (ok, res) {
      b.disabled = false;
      if (!ok) { if (hdnote) hdnote.textContent = wsT("js.hd.stopfail", res); return; }
      hdLoad();
    });
  }

  // --- git drawer: the action panel over the git routes ---------------------
  // Each opening of the git sheet asks the daemon to read the handle again
  // and reads the changed files (gitdrawer.go): the standing line, then the
  // staged, unstaged, and untracked groups, each file with its status and
  // line delta. Fetch, pull, and push post to their routes (gitpush.go), one
  // at a time: the pressed button shows busy and the others wait; the
  // daemon's refusal or git's failure shows on the panel's message line, a
  // pull's "nothing to pull" as a plain note. Every answer reads the files
  // again, and the graph with them; the handle's own re-read arrives
  // through the stream.
  var gref = $("gref"), gfiles = $("gfiles"), gmsg = $("gmsg"), gSeq = 0, gBusy = false, gitLast = null;
  function gitOpened() {
    if (!gbody || !window.fetch) return;
    fetch("/s/" + rid + "/git", { method: "POST", credentials: "same-origin", cache: "no-store" }).catch(function () {});
    gitLoad();
    glLoad(false);
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
    gitLast = v;
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
      list.forEach(function (f, i) {
        var row = gitRow(f);
        row.setAttribute("data-side", g[0]);
        row.setAttribute("data-i", String(i));
        sec.appendChild(row);
      });
      gfiles.appendChild(sec);
    });
    // A clean tree shows a quiet empty state centred in the list's room.
    gfiles.classList.toggle("clean", !any);
    if (!any) {
      var clean = mk("div", "gclean");
      clean.innerHTML = '<svg class="i" aria-hidden="true"><use href="#i-check"/></svg>';
      clean.appendChild(mk("span", "", wsT("js.git.clean")));
      gfiles.appendChild(clean);
    }
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
      glLoad(false);
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
    // A changed file opens the commit window on the working tree.
    var f = e.target.closest && e.target.closest(".gf[data-side]");
    if (f) gwWorktree(f.getAttribute("data-side"), +f.getAttribute("data-i"));
  });

  // --- git drawer: the commit graph and the commit window --------------------
  // The graph reads the session root's log a page at a time (gitlog.go, GET
  // git/log): every local branch, the upstreams they track, and detached
  // worktree HEADs, with the lanes the daemon laid out. A row's down
  // segments run from its node to the next row, so pages stack; the last
  // loaded row's run on dashed into the "more" row while a page remains.
  // The graph is read-only. It loads with the git sheet and after each
  // action, never while hidden (a narrow screen shows the panel alone). A
  // row's hover shows its tooltip; a click (or Enter) opens the commit
  // window (GET git/commit/<oid>), a centred window over the page: header,
  // message, author and committer, copy-hash, parents (a parent opens in the
  // same window), the changed-file list, and the diff viewer below; the
  // working-tree file list opens the same window.
  var glog = $("glog"), glrows = $("glrows"), glscroll = $("glscroll"), glcount = $("glcount");
  var GL_ROW = 26, GL_DX = 14, GL_X0 = 12, GL_MORE = 34;
  var gl = { commits: [], by: {}, width: 1, more: 0, page: 0, head: "", seq: 0, busy: false, loaded: false, tipRow: null };
  function svgEl(tag, attrs) {
    var e = doc.createElementNS(SVGNS, tag);
    for (var k in attrs) if (Object.prototype.hasOwnProperty.call(attrs, k)) e.setAttribute(k, attrs[k]);
    return e;
  }
  function gIcon(name) {
    var s = svgEl("svg", { "class": "i", "aria-hidden": "true" });
    s.appendChild(svgEl("use", { href: "#i-" + name }));
    return s;
  }
  // luIcon is a vendored Lucide icon (the page's l- sprite), sized as gIcon's.
  function luIcon(name) {
    var s = svgEl("svg", { "class": "i li", "aria-hidden": "true" });
    s.appendChild(svgEl("use", { href: "#l-" + name }));
    return s;
  }
  function glX(col) { return GL_X0 + col * GL_DX; }
  function glY(i) { return i * GL_ROW + GL_ROW / 2; }
  function glLane(color) { return "var(--gl-" + (color % 8) + ")"; }
  // glAgo is a short age: minutes, hours, days, then the date.
  function glAgo(ms) {
    var m = Math.max(0, Math.round((Date.now() - ms) / 60000));
    if (m < 60) return m + "m";
    var h = Math.round(m / 60);
    if (h < 48) return h + "h";
    var d = Math.round(h / 24);
    return d < 60 ? d + "d" : new Date(ms).toLocaleDateString(wsI18n.lang);
  }
  function glRel(ms) {
    var a = glAgo(ms);
    return /^\d+[mhd]$/.test(a) ? wsT("js.ago", a) : a;
  }
  function glWhen(ms) { return new Date(ms).toLocaleString(wsI18n.lang); }
  // glHandle is the drawer handle's abbreviation (gitdrawer.go branchHandle)
  // for an upstream's branch part; the daemon sends it for local branches.
  function glHandle(name) {
    var segs = name.split("/"), h = { text: name };
    if (segs.length < 2 || (segs[0] !== "impl" && segs[0] !== "goal") || !segs[segs.length - 1]) return h;
    h.text = segs[segs.length - 1];
    segs.slice(0, -1).forEach(function (s) { if (s === "impl") h.impl = true; if (s === "goal") h.goal = true; });
    return h;
  }
  function glIcons(el, h) {
    if (h.goal) { var g = mk("span", "gh-goal"); g.appendChild(gIcon("trophy")); el.appendChild(g); }
    if (h.impl) { var m = mk("span", "gh-impl"); m.appendChild(gIcon("hammer")); el.appendChild(m); }
    if (!h.goal && !h.impl) el.appendChild(gIcon("branch"));
  }
  function glBase(path) {
    var p = path.replace(/[\\/]+$/, "");
    return p.slice(Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\")) + 1) || p;
  }
  function glWt(el) { var w = mk("span", "wt"); w.appendChild(gIcon("folder")); el.appendChild(w); }
  // glPills draws a commit's ref labels: a local branch in its lane's colour
  // with its upstream folded in when that is on the same commit, an upstream
  // of its own otherwise, and a detached worktree HEAD.
  function glPills(c) {
    var out = [], refs = c.refs || [], folded = {};
    refs.forEach(function (r) {
      if (r.kind !== "branch" || !r.upstream) return;
      refs.forEach(function (u) { if (u.kind === "upstream" && u.name === r.upstream) folded[u.name] = r.name; });
    });
    refs.forEach(function (r) {
      var p, title = [];
      if (r.kind === "branch") {
        p = mk("span", "rf" + (r.head ? " head" : "") + (r.merged ? " merged" : ""));
        if (r.head) p.appendChild(mk("span", "hd", "HEAD"));
        glIcons(p, r);
        p.appendChild(mk("span", "t", r.text || r.name));
        if (r.ahead || r.behind) {
          var ab = mk("span", "ab");
          if (r.ahead) ab.appendChild(mk("b", "up", "↑" + r.ahead));
          if (r.behind) ab.appendChild(mk("b", "dn", "↓" + r.behind));
          p.appendChild(ab);
        }
        if (r.worktree) glWt(p);
        if (r.upstream && folded[r.upstream] === r.name) p.appendChild(mk("span", "ups", r.upstream.split("/")[0]));
        title.push(r.name);
        if (r.gone) title.push(wsT("js.git.ref.gone", r.upstream));
        else if (r.upstream) title.push(wsT("js.git.ref.upstream", r.upstream) + (r.ahead || r.behind ? " " + wsT("js.git.ref.ab", r.ahead || 0, r.behind || 0) : ""));
        else title.push(wsT("js.git.noupstream"));
        if (r.merged) title.push(wsT("js.git.ref.merged"));
      } else if (r.kind === "upstream") {
        if (folded[r.name]) return;
        var parts = r.name.split("/"), h = glHandle(parts.slice(1).join("/"));
        p = mk("span", "rf remote");
        glIcons(p, h);
        p.appendChild(mk("span", "t", parts[0] + "/" + h.text));
        title.push(r.name);
        if (r.of) title.push(wsT("js.git.ref.upof", r.of));
      } else {
        p = mk("span", "rf wtd" + (r.head ? " head" : ""));
        if (r.head) p.appendChild(mk("span", "hd", "HEAD"));
        glWt(p);
        p.appendChild(mk("span", "t", r.worktree ? glBase(r.worktree) : c.oid.slice(0, 7)));
        title.push(wsT("js.git.detached", c.oid.slice(0, 9)));
      }
      if (r.worktree) title.push(wsT("js.git.ref.worktree", r.worktree));
      if (r.head) title.push(wsT("js.git.ref.head"));
      p.style.setProperty("--lc", glLane(c.color));
      p.title = title.join("\n");
      out.push(p);
    });
    return out;
  }
  function glFail(msg, append) {
    // A failed further page keeps the rows already drawn and says so where
    // its "Load more" was.
    var more = append && gl.commits.length && q(".glmore", glrows)[0];
    if (more) { q(".glmore-b", more).forEach(function (b) { b.disabled = false; }); more.lastChild.textContent = wsT("js.git.log.fail", msg); return; }
    glcount.hidden = true;
    glrows.textContent = "";
    glrows.appendChild(mk("div", "gempty err", wsT("js.git.log.fail", msg)));
  }
  // glDraw draws every loaded row and one SVG of lanes and dots under them.
  function glDraw() {
    var cs = gl.commits, n = cs.length, keep = glscroll.scrollTop;
    glTipHide();
    glrows.textContent = "";
    gl.by = {};
    if (!n) { glcount.hidden = true; glrows.appendChild(mk("div", "gempty", wsT("js.git.log.empty"))); return; }
    var w = glX(gl.width - 1) + 12, H = n * GL_ROW;
    // The column stays wide enough for its header on a one-lane graph.
    glscroll.style.setProperty("--gw", Math.max(w, 52) + "px");
    glcount.hidden = false;
    glcount.textContent = n + (gl.more ? "+" : "");
    var svg = svgEl("svg", { "class": "glsvg", width: w, height: H + GL_MORE, "aria-hidden": "true" });
    var lines = svgEl("g", {}), dots = svgEl("g", {}), frag = doc.createDocumentFragment();
    cs.forEach(function (c, i) {
      gl.by[c.oid] = c;
      var last = i === n - 1;
      (c.down || []).forEach(function (s) {
        var x1 = glX(s.from), y1 = glY(i), x2 = glX(s.to), y2 = last ? H + GL_MORE / 2 : glY(i + 1), m = (y1 + y2) / 2;
        var a = { d: x1 === x2 ? "M" + x1 + " " + y1 + "V" + y2 : "M" + x1 + " " + y1 + "C" + x1 + " " + m + " " + x2 + " " + m + " " + x2 + " " + y2, stroke: glLane(s.color) };
        if (last) a["class"] = "cont";
        lines.appendChild(svgEl("path", a));
      });
      var x = glX(c.col), y = glY(i), lc = glLane(c.color), merge = c.parents.length > 1;
      if (c.oid === gl.head) dots.appendChild(svgEl("circle", { "class": "head", cx: x, cy: y, r: 8 }));
      if (c.remote) dots.appendChild(svgEl("circle", { "class": "rem", cx: x, cy: y, r: 4, stroke: lc }));
      else if (merge) dots.appendChild(svgEl("circle", { "class": "mg", cx: x, cy: y, r: 4, stroke: lc }));
      else dots.appendChild(svgEl("circle", { "class": "dot", cx: x, cy: y, r: 5, fill: lc }));
      var row = mk("div", "glr" + (merge ? " mg" : "") + (c.remote ? " rem" : "") + (c.oid === gw.oid && gw.back && !gw.back.hidden ? " sel" : ""));
      row.tabIndex = 0;
      row.setAttribute("data-i", String(i));
      row.appendChild(mk("span"));
      var sub = mk("span", "gls");
      glPills(c).forEach(function (p) { sub.appendChild(p); });
      sub.appendChild(mk("span", "glsub", c.subject));
      row.appendChild(sub);
      row.appendChild(mk("span", "gla", c.author));
      var t = mk("span", "glt", glAgo(c.time));
      t.title = glWhen(c.time);
      row.appendChild(t);
      row.appendChild(mk("span", "glh", c.oid.slice(0, 7)));
      frag.appendChild(row);
    });
    if (gl.more) {
      var more = mk("div", "glmore"), b = mk("button", "btn glmore-b", wsT("js.git.log.more"));
      b.type = "button";
      b.disabled = gl.busy;
      more.appendChild(b);
      more.appendChild(mk("span", "", wsT("js.git.log.morenote", gl.page)));
      frag.appendChild(more);
    }
    svg.appendChild(lines);
    svg.appendChild(dots);
    frag.appendChild(svg);
    glrows.appendChild(frag);
    glscroll.scrollTop = keep;
  }
  // glLoad reads the first page again, or (append) the next one.
  function glLoad(append) {
    if (!glog || !window.fetch) return;
    // Hidden (narrow layout): read it again once it shows.
    if (!glog.offsetWidth) { if (!append) gl.loaded = false; return; }
    if (append && (!gl.more || gl.busy)) return;
    gl.loaded = true;
    var seq = ++gl.seq, offset = append ? gl.more : 0;
    gl.busy = true;
    if (append) q(".glmore-b", glrows).forEach(function (b) { b.disabled = true; });
    else if (!gl.commits.length) { glrows.textContent = ""; glrows.appendChild(mk("div", "gempty", wsT("js.git.log.loading"))); }
    fetch("/s/" + rid + "/git/log" + (offset ? "?offset=" + offset : ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gl.seq) return;
        gl.busy = false;
        if (!r.ok) { glFail(s.trim() || "HTTP " + r.status, append); return; }
        var v;
        try { v = JSON.parse(s); } catch (e) { glFail("malformed answer", append); return; }
        if (!append) { gl.commits = []; gl.width = 1; gl.page = (v.commits || []).length; }
        gl.commits = gl.commits.concat(v.commits || []);
        gl.width = Math.max(gl.width, v.width || 1);
        gl.more = v.more || 0;
        gl.head = v.head || "";
        glDraw();
      });
    }, function () { if (seq === gl.seq) { gl.busy = false; glFail(wsT("js.post.unreachable"), append); } });
  }
  // --- agent stats hover card ----------------------------------------------
  // A narrow agents panel hides a subagent row's stats (template astat, its
  // container query); hovering the row then shows them in a card built like
  // the git graph's tooltip, one line per stat from its title. A tree swap
  // rebuilds the card from the same agent's new row, so it keeps live values.
  var atip = null, atipKey = null;
  function agentTipHide() { if (atip) atip.hidden = true; atipKey = null; }
  function agentTip(head) {
    var st = head.querySelector(".astat"), key = head.parentNode.getAttribute("data-key");
    if (!st || !key || getComputedStyle(st).display !== "none") { agentTipHide(); return; }
    if (key === atipKey && atip && !atip.hidden) return;
    if (!atip) { atip = mk("div", "gltip atip"); atip.hidden = true; doc.body.appendChild(atip); }
    atipKey = key;
    atip.textContent = "";
    var name = head.querySelector(".aname");
    atip.appendChild(mk("div", "ts", name ? name.textContent : ""));
    var tm = mk("div", "tm");
    Array.prototype.forEach.call(st.children, function (s) { tm.appendChild(mk("span", "", s.getAttribute("title") || s.textContent)); });
    atip.appendChild(tm);
    atip.hidden = false;
    var r = head.getBoundingClientRect(), y = r.bottom + 4, th = atip.offsetHeight;
    if (y + th > window.innerHeight - 8) y = r.top - th - 4;
    atip.style.left = Math.max(8, Math.min(r.left + 20, window.innerWidth - atip.offsetWidth - 8)) + "px";
    atip.style.top = y + "px";
  }
  function agentTipSwapped() {
    if (!atipKey) return;
    var key = atipKey, heads = doc.querySelectorAll("#r-tree details > summary.ahead");
    atipKey = null;
    for (var i = 0; i < heads.length; i++) {
      if (heads[i].parentNode.getAttribute("data-key") === key) { agentTip(heads[i]); return; }
    }
    agentTipHide();
  }
  (function () {
    var tree = $("r-tree");
    if (!tree) return;
    tree.addEventListener("mouseover", function (e) {
      var head = e.target.closest && e.target.closest("summary.ahead");
      if (head && head.querySelector(".astat")) agentTip(head); else agentTipHide();
    });
    tree.addEventListener("mouseleave", agentTipHide);
    tree.addEventListener("scroll", agentTipHide);
  })();
  // The tooltip: subject, refs, author, age, short hash, under the row.
  var gltip = null;
  function glTipHide() { if (gltip) gltip.hidden = true; gl.tipRow = null; }
  function glTip(row) {
    if (row === gl.tipRow && gltip && !gltip.hidden) return;
    var c = gl.commits[+row.getAttribute("data-i")];
    if (!c) return;
    if (!gltip) { gltip = mk("div", "gltip"); gltip.hidden = true; doc.body.appendChild(gltip); }
    gl.tipRow = row;
    gltip.textContent = "";
    gltip.appendChild(mk("div", "ts", c.subject));
    var pills = glPills(c);
    if (pills.length) { var tr = mk("div", "tr"); pills.forEach(function (p) { tr.appendChild(p); }); gltip.appendChild(tr); }
    var tm = mk("div", "tm");
    [c.author, "·", glRel(c.time), "·"].forEach(function (s) { tm.appendChild(mk("span", "", s)); });
    tm.appendChild(mk("code", "", c.oid.slice(0, 9)));
    gltip.appendChild(tm);
    gltip.hidden = false;
    var r = row.getBoundingClientRect(), x = r.left + glX(c.col) + 14, y = r.bottom + 4, th = gltip.offsetHeight;
    if (y + th > window.innerHeight - 8) y = r.top - th - 4;
    gltip.style.left = Math.max(8, Math.min(x, window.innerWidth - gltip.offsetWidth - 8)) + "px";
    gltip.style.top = y + "px";
  }
  if (glrows) {
    glrows.addEventListener("mouseover", function (e) { var row = e.target.closest && e.target.closest(".glr"); if (row) glTip(row); else glTipHide(); });
    glrows.addEventListener("mouseleave", glTipHide);
    glscroll.addEventListener("scroll", glTipHide);
    glrows.addEventListener("click", function (e) {
      if (e.target.closest(".glmore-b")) { glLoad(true); return; }
      var row = e.target.closest(".glr");
      if (row) { glTipHide(); gwOpen(gl.commits[+row.getAttribute("data-i")].oid); }
    });
    glrows.addEventListener("keydown", function (e) {
      var row = e.target.closest && e.target.closest(".glr");
      if (row && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); gwOpen(gl.commits[+row.getAttribute("data-i")].oid); }
    });
    // A screen widened past the narrow layout shows the graph: read it then.
    window.addEventListener("resize", function () { if (!gl.loaded && drOpen && drMode === "git" && glog.offsetWidth) glLoad(false); });
  }

  // The commit window, built on first use and kept on the body (outside the
  // drawer's clip, which a fixed element inside it would not escape).
  var gw = { back: null, oid: "", seq: 0, lastFocus: null, down: false, list: [], pin: -1, io: null };
  function gwBuild() {
    if (gw.back) return;
    var back = mk("div", "gwin-back"), win = mk("div", "gwin"), top = mk("div", "gw-top"), head = mk("div", "gw-head");
    back.hidden = true;
    win.setAttribute("role", "dialog");
    win.setAttribute("aria-modal", "true");
    win.setAttribute("aria-labelledby", "gwsubj");
    win.tabIndex = -1;
    var h2 = mk("h2");
    h2.id = "gwsubj";
    var acts = mk("div", "gw-acts"), copy = mk("button", "idcopy"), close = mk("button", "ia");
    copy.type = close.type = "button";
    copy.title = wsT("js.git.win.copy.title");
    copy.appendChild(gIcon("copy"));
    copy.appendChild(mk("span", "", wsT("js.git.win.copy")));
    close.title = wsT("js.git.win.close");
    close.setAttribute("aria-label", close.title);
    close.appendChild(gIcon("x"));
    acts.appendChild(copy);
    acts.appendChild(close);
    head.appendChild(h2);
    head.appendChild(acts);
    var chips = mk("div", "gw-chips"), meta = mk("dl", "gw-meta"), msg = mk("pre", "gw-msg");
    [head, chips, meta, msg].forEach(function (e) { top.appendChild(e); });
    var body = mk("div", "gw-body"), diff = mk("div", "gw-diff"), files = mk("aside", "gw-files");
    files.setAttribute("aria-label", wsT("js.git.win.fileslabel"));
    body.appendChild(diff);
    body.appendChild(files);
    win.appendChild(top);
    win.appendChild(body);
    back.appendChild(win);
    doc.body.appendChild(back);
    gw.back = back; gw.win = win; gw.h2 = h2; gw.copy = copy; gw.chips = chips; gw.meta = meta; gw.msg = msg; gw.diff = diff; gw.files = files;
    close.addEventListener("click", gwClose);
    // Outside: a press and its release both on the backdrop, so a text
    // selection dragged out of the window keeps it open.
    back.addEventListener("mousedown", function (e) { gw.down = e.target === back; });
    back.addEventListener("click", function (e) { if (e.target === back && gw.down) gwClose(); gw.down = false; });
    copy.addEventListener("click", function () {
      var full = gw.oid, span = copy.querySelector("span");
      gwCopy(full, copy, function (ok) {
        if (!ok) return;
        copy.classList.add("done");
        copy.querySelector("use").setAttribute("href", "#i-check");
        span.textContent = wsT("js.git.win.copied");
        setTimeout(function () {
          copy.classList.remove("done");
          copy.querySelector("use").setAttribute("href", "#i-copy");
          span.textContent = wsT("js.git.win.copy");
        }, 1400);
      });
    });
    meta.addEventListener("click", function (e) { var p = e.target.closest(".gw-par"); if (p) gwOpen(p.getAttribute("data-oid")); });
    files.addEventListener("click", function (e) {
      var r = e.target.closest(".gwf");
      if (r) gwGo(+r.getAttribute("data-i"));
    });
    diff.addEventListener("click", function (e) {
      var b = e.target.closest(".load-b"), f = e.target.closest(".fold-b");
      if (b) gwLoad(b.closest(".dfile"));
      if (f) f.closest(".dfile").classList.toggle("fold");
    });
    diff.addEventListener("scroll", gwSpy, { passive: true });
    // A clicked file stays pinned in view while the sections above it load
    // and grow, until the user scrolls the viewer.
    ["wheel", "touchstart", "keydown", "mousedown"].forEach(function (ev) {
      diff.addEventListener(ev, function () { gw.pin = -1; }, { passive: true });
    });
  }
  function gwCopy(text, near, done) {
    function legacy() {
      var ta = doc.createElement("textarea");
      ta.value = text; ta.setAttribute("readonly", ""); ta.style.position = "fixed"; ta.style.opacity = "0";
      near.appendChild(ta); ta.select();
      var ok = false;
      try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
      ta.remove(); near.focus();
      done(ok);
    }
    if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(function () { done(true); }, legacy);
    else legacy();
  }
  function gwSel() { q(".glr", glrows || doc).forEach(function (r) { var c = gl.commits[+r.getAttribute("data-i")]; r.classList.toggle("sel", !!c && c.oid === gw.oid && !gw.back.hidden); }); }
  function gwClose() {
    if (!gw.back || gw.back.hidden) return;
    gw.back.hidden = true;
    gw.seq++;
    if (gw.io) gw.io.disconnect();
    gwSel();
    gw.oid = "";
    if (gw.lastFocus && gw.lastFocus.focus && doc.contains(gw.lastFocus)) gw.lastFocus.focus();
  }
  // Escape closes the window before anything else hears it (the drawer
  // would close on it too).
  window.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && gw.back && !gw.back.hidden) { e.preventDefault(); e.stopPropagation(); gwClose(); }
  }, true);
  function gwNote(text, err) { return mk("div", "gempty" + (err ? " err" : ""), text); }
  function gwOpen(oid) {
    if (!oid || !window.fetch) return;
    gwBuild();
    var seq = ++gw.seq;
    gw.oid = oid;
    if (gw.back.hidden) { gw.lastFocus = doc.activeElement; gw.back.hidden = false; }
    gwSel();
    gw.win.focus();
    gw.h2.textContent = oid.slice(0, 12);
    gw.copy.hidden = true;
    gw.chips.textContent = gw.meta.textContent = gw.files.textContent = gw.diff.textContent = "";
    gw.msg.hidden = true;
    gw.files.appendChild(gwNote(wsT("js.git.win.loading")));
    fetch("/s/" + rid + "/git/commit/" + encodeURIComponent(oid), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gw.seq) return;
        var c = null;
        if (r.ok) { try { c = JSON.parse(s); } catch (e) { s = "malformed answer"; } }
        if (!c) { gw.files.textContent = ""; gw.files.appendChild(gwNote(wsT("js.git.win.fail", s.trim() || "HTTP " + r.status), true)); return; }
        gwFill(c);
      });
    }, function () { if (seq === gw.seq) { gw.files.textContent = ""; gw.files.appendChild(gwNote(wsT("js.post.unreachable"), true)); } });
  }
  function gwPerson(dd, name, email, ms) {
    dd.appendChild(doc.createTextNode(name + " "));
    dd.appendChild(mk("span", "em", "<" + email + ">"));
    dd.appendChild(doc.createTextNode(" · " + glRel(ms) + " "));
    dd.appendChild(mk("span", "abs", "(" + glWhen(ms) + ")"));
  }
  function gwPath(p) {
    var el = mk("span", "gp"), slash = p.lastIndexOf("/");
    if (slash >= 0) el.appendChild(mk("span", "gdir", p.slice(0, slash + 1)));
    el.appendChild(mk("span", "gname", p.slice(slash + 1)));
    return el;
  }
  function gwDelta(add, del) {
    var d = mk("span", "gd");
    if (add < 0) d.appendChild(mk("i", "bin", "bin"));
    else {
      if (add) d.appendChild(mk("i", "add", "+" + add));
      if (del) d.appendChild(mk("i", "del", "−" + del));
    }
    return d;
  }
  // gwFill draws a commit read (GitCommit) into the window.
  function gwFill(c) {
    gw.oid = c.oid;
    var lines = (c.message || "").split("\n"), body = lines.slice(1).join("\n").replace(/^\n+/, "");
    gw.h2.textContent = lines[0] || c.oid.slice(0, 12);
    gw.copy.hidden = false;
    gw.chips.textContent = "";
    var hash = mk("code", "hash", c.oid.slice(0, 12));
    hash.title = c.oid;
    gw.chips.appendChild(hash);
    if (gl.by[c.oid]) glPills(gl.by[c.oid]).forEach(function (p) { gw.chips.appendChild(p); });
    if (c.parents.length > 1) gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.merge")));
    else if (!c.parents.length) gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.root")));
    gw.meta.textContent = "";
    var dd;
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.author")));
    gw.meta.appendChild(dd = mk("dd"));
    gwPerson(dd, c.author, c.authorEmail, c.authorTime);
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.committer")));
    gw.meta.appendChild(dd = mk("dd"));
    if (c.committer === c.author && c.committerEmail === c.authorEmail && c.commitTime === c.authorTime) dd.appendChild(mk("span", "em", wsT("js.git.win.same")));
    else gwPerson(dd, c.committer, c.committerEmail, c.commitTime);
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.parents")));
    gw.meta.appendChild(dd = mk("dd"));
    if (!c.parents.length) dd.appendChild(mk("span", "em", wsT("js.git.win.noparents")));
    c.parents.forEach(function (p, i) {
      if (i) dd.appendChild(doc.createTextNode(" "));
      var b = mk("button", "gw-par", p.slice(0, 9));
      b.type = "button";
      b.setAttribute("data-oid", p);
      b.title = gl.by[p] ? gl.by[p].subject : p;
      dd.appendChild(b);
      if (i === 0 && c.parents.length > 1) { dd.appendChild(doc.createTextNode(" ")); dd.appendChild(mk("span", "em", wsT("js.git.win.base"))); }
    });
    gw.msg.textContent = body;
    gw.msg.hidden = !body;
    var base = "/s/" + rid + "/git/commit/" + c.oid + "/diff?path=";
    gwBody([{ files: c.files.map(function (f) { return { path: f.path, status: f.status, from: f.from, add: f.add, del: f.del, url: base + encodeURIComponent(f.path) }; }) }], -1,
      c.truncated ? wsT("js.git.win.truncated", c.files.length) : "");
  }
  // gwWorktree opens the window on the working tree's changed files (the
  // git sheet's last status read), the file at index i of side in view.
  function gwWorktree(side, i) {
    var v = gitLast;
    if (!v || !window.fetch) return;
    gwBuild();
    var seq = ++gw.seq;
    gw.oid = "";
    if (gw.back.hidden) { gw.lastFocus = doc.activeElement; gw.back.hidden = false; }
    gwSel();
    gw.win.focus();
    gw.h2.textContent = wsT("js.git.win.worktree");
    gw.copy.hidden = true;
    gw.chips.textContent = gw.meta.textContent = "";
    gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.wtnote")));
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.branch")));
    gw.meta.appendChild(mk("dd", "", v.detached ? wsT("js.git.detached", v.branch) : v.branch + (v.upstream ? " \u2192 " + v.upstream : "")));
    gw.msg.hidden = true;
    var focus = -1, n = 0, groups = [];
    [["staged", "js.git.staged"], ["unstaged", "js.git.unstaged"], ["untracked", "js.git.untracked"]].forEach(function (g) {
      var list = v[g[0]] || [];
      if (g[0] === side) focus = n + i;
      n += list.length;
      if (!list.length) return;
      var base = "/s/" + rid + "/git/worktree/diff?side=" + g[0] + "&path=";
      groups.push({ label: wsT(g[1]), files: list.map(function (f) { return { path: f.path, status: f.status, add: f.add, del: f.del, untracked: g[0] === "untracked", url: base + encodeURIComponent(f.path) }; }) });
    });
    if (seq === gw.seq) gwBody(groups, focus, v.truncated ? wsT("js.git.truncated", 300) : "");
  }

  // --- the commit window's diff viewer ---------------------------------------
  // One section per changed file (gitdiff.go serves each file's diff). A
  // section reads its diff when it nears the viewport; a file with more than
  // GW_MANUAL changed lines, a binary file, or a deleted file waits behind
  // "Load diff". Clicking a file in the list scrolls to its section and loads
  // it, and keeps it pinned in view until the user scrolls (gwState).
  var GW_MANUAL = 400;
  function gwStatusClass(st) { return "gst st-" + (st === "?" ? "q" : st); }
  function gwManual(f) {
    if (f.add < 0) return wsT("js.git.diff.binarywait");
    if (f.status === "D") return wsTn("js.git.diff.deleted", f.del);
    if (f.add + f.del > GW_MANUAL) return wsT("js.git.diff.large", f.add + f.del);
    return "";
  }
  // gwBody fills the file list (groups of {label, files}) and the viewer's
  // sections, and brings file focus (an index over every group) into view.
  function gwBody(groups, focus, note) {
    var seq = gw.seq, add = 0, del = 0, n = 0;
    gw.list = [];
    gw.pin = -1;
    if (gw.io) gw.io.disconnect();
    gw.files.textContent = gw.diff.textContent = "";
    groups.forEach(function (g) { g.files.forEach(function (f) { gw.list.push(f); if (f.add > 0) add += f.add; if (f.del > 0) del += f.del; }); });
    var sum = mk("div", "gw-fsum");
    sum.appendChild(mk("span", "", wsTn("js.git.win.files", gw.list.length)));
    sum.appendChild(gwDelta(add, del));
    gw.files.appendChild(sum);
    groups.forEach(function (g) {
      if (g.label) {
        var head = mk("div", "ggrp");
        head.appendChild(mk("span", "", g.label));
        head.appendChild(mk("span", "chip", String(g.files.length)));
        gw.files.appendChild(head);
      }
      g.files.forEach(function (f) {
        var row = mk("div", "gf gwf");
        row.setAttribute("data-i", String(n++));
        row.title = f.from ? f.from + " \u2192 " + f.path : f.path;
        row.appendChild(mk("b", gwStatusClass(f.status), f.status));
        row.appendChild(gwPath(f.path));
        row.appendChild(f.untracked ? mk("span", "gd") : gwDelta(f.add, f.del));
        if (f.from) row.appendChild(mk("span", "ren", "\u2190 " + f.from.slice(f.from.lastIndexOf("/") + 1)));
        gw.files.appendChild(row);
      });
    });
    if (note) gw.files.appendChild(gwNote(note));
    if (!gw.list.length) gw.diff.appendChild(gwNote(wsT("js.git.win.nofiles")));
    gw.list.forEach(function (f, i) { gw.diff.appendChild(gwSection(f, i, seq)); });
    gw.diff.scrollTop = 0;
    gw.io = "IntersectionObserver" in window ? new IntersectionObserver(function (es) {
      es.forEach(function (e) { if (e.isIntersecting && e.target.getAttribute("data-state") === "auto") gwLoad(e.target); });
    }, { root: gw.diff, rootMargin: "200px 0px" }) : null;
    q(".dfile", gw.diff).forEach(function (sec) {
      if (gw.io) gw.io.observe(sec);
      else if (sec.getAttribute("data-state") === "auto") gwLoad(sec);
    });
    if (focus >= 0) gwGo(focus);
    else gwSpy();
  }
  function gwSection(f, i, seq) {
    var sec = mk("section", "dfile"), h = mk("header", "dfh"), fold = mk("button", "ia fold-b");
    sec.setAttribute("data-i", String(i));
    sec.gwSeq = seq;
    fold.type = "button";
    fold.title = wsT("js.git.win.fold");
    fold.setAttribute("aria-label", fold.title);
    fold.appendChild(gIcon("chev"));
    h.appendChild(fold);
    h.appendChild(mk("b", gwStatusClass(f.status), f.status));
    h.appendChild(gwPath(f.path));
    if (f.from) h.appendChild(mk("span", "dfren", "\u2190 " + f.from));
    h.appendChild(f.untracked ? mk("span", "gd") : gwDelta(f.add, f.del));
    sec.appendChild(h);
    sec.appendChild(mk("div", "dfb"));
    gwState(sec, gwManual(f) ? "manual" : "auto");
    return sec;
  }
  function gwSkeleton(widths) {
    var sk = mk("div", "dskel");
    widths.forEach(function (w) { var i = mk("i"); i.style.width = w + "%"; sk.appendChild(i); });
    return sk;
  }
  // gwState draws a section's body for its state: auto and loading (a
  // skeleton), manual (why, and "Load diff"), fail, or ready (the diff).
  function gwState(sec, state, data) {
    var f = gw.list[+sec.getAttribute("data-i")], b = sec.querySelector(".dfb");
    sec.setAttribute("data-state", state);
    b.textContent = "";
    if (state === "auto") b.appendChild(gwSkeleton([46, 72, 58]));
    else if (state === "loading") b.appendChild(gwSkeleton([60, 38, 66, 50]));
    else if (state === "manual" || state === "fail") {
      var note = mk("div", "dnote" + (state === "fail" ? " err" : ""));
      note.appendChild(mk("span", "", state === "fail" ? wsT("js.git.diff.fail", data) : gwManual(f)));
      var lb = mk("button", "btn load-b", wsT("js.git.win.loaddiff"));
      lb.type = "button";
      note.appendChild(lb);
      b.appendChild(note);
    } else gwRender(b, data);
    // Sections above the pinned file grow as they load: keep it in place.
    var pin = gw.pin >= 0 && q('.dfile[data-i="' + gw.pin + '"]', gw.diff)[0];
    if (pin) gw.diff.scrollTop = pin.offsetTop - 10;
  }
  function gwLoad(sec) {
    var st = sec.getAttribute("data-state"), seq = sec.gwSeq, f = gw.list[+sec.getAttribute("data-i")];
    if (st === "ready" || st === "loading" || seq !== gw.seq || !f) return;
    gwState(sec, "loading");
    fetch(f.url, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gw.seq) return;
        var d = null;
        if (r.ok) { try { d = JSON.parse(s); } catch (e) { s = "malformed answer"; } }
        if (d) gwState(sec, "ready", d);
        else gwState(sec, "fail", s.trim() || "HTTP " + r.status);
      });
    }, function () { if (seq === gw.seq) gwState(sec, "fail", wsT("js.post.unreachable")); });
  }
  function gwDnote(text, cls) { return mk("div", "dnote" + (cls ? " " + cls : ""), text); }
  // gwRender draws one file's diff (GitFileDiff): numbered unified lines
  // with their highlight classes and intra-line marks, or a notice.
  function gwRender(b, d) {
    var notices = { dir: wsT("js.git.diff.notice.dir"), gitlink: wsT("js.git.diff.notice.gitlink"), unmerged: wsT("js.git.diff.notice.unmerged"), special: wsT("js.git.diff.notice.special"), outside: wsT("js.git.diff.notice.outside") };
    if (d.notice) b.appendChild(gwDnote(notices[d.notice] || d.notice));
    else if (d.binary) b.appendChild(gwDnote(wsT("js.git.diff.binary")));
    else if (!d.hunks || !d.hunks.length) b.appendChild(gwDnote(wsT("js.git.diff.nochange")));
    else {
      var tab = mk("div", "dtab chroma"), frag = doc.createDocumentFragment();
      d.hunks.forEach(function (h) {
        var o = h.old, n = h.new;
        frag.appendChild(gwRow("hk", "", "", "", [{ s: h.header }]));
        h.lines.forEach(function (l) {
          if (l.k === "+") frag.appendChild(gwRow("add", "", n++, "+", l.t));
          else if (l.k === "-") frag.appendChild(gwRow("del", o++, "", "\u2212", l.t));
          else if (l.k === "\\") frag.appendChild(gwRow("nl", "", "", "", l.t));
          else frag.appendChild(gwRow("", o++, n++, "", l.t));
        });
      });
      tab.appendChild(frag);
      b.appendChild(tab);
    }
    if (d.truncated) b.appendChild(gwDnote(wsT("js.git.diff.cut"), "cut"));
  }
  function gwRow(cls, o, n, sign, pieces) {
    var row = mk("div", "dl" + (cls ? " " + cls : "")), cd = mk("span", "cd");
    row.appendChild(mk("span", "no", String(o)));
    row.appendChild(mk("span", "no", String(n)));
    row.appendChild(mk("span", "sg", sign));
    (pieces || []).forEach(function (p) {
      if (!p.c && !p.x) { cd.appendChild(doc.createTextNode(p.s)); return; }
      cd.appendChild(mk("span", (p.c ? "t-" + p.c : "") + (p.x ? (p.c ? " dx" : "dx") : ""), p.s));
    });
    row.appendChild(cd);
    return row;
  }
  // gwGo scrolls to file i's section, loads it, and pins it.
  function gwGo(i) {
    var sec = q('.dfile[data-i="' + i + '"]', gw.diff)[0];
    if (!sec) return;
    gw.pin = i;
    sec.classList.remove("fold");
    gwLoad(sec);
    gw.diff.scrollTop = sec.offsetTop - 10;
    gwMark(i);
  }
  function gwMark(i) { q(".gwf", gw.files).forEach(function (r) { r.classList.toggle("on", +r.getAttribute("data-i") === i); }); }
  // gwSpy marks the file whose section is at the top of the viewer.
  function gwSpy() {
    if (gw.pin >= 0) { gwMark(gw.pin); return; }
    var top = gw.diff.scrollTop + 40, on = 0;
    q(".dfile", gw.diff).forEach(function (sec) { if (sec.offsetTop <= top) on = +sec.getAttribute("data-i"); });
    gwMark(gw.list.length ? on : -1);
  }

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
  // posted. A store hidden on this machine is left out of tkOrder (the
  // handles) and its view is not read; tkHidden lists it for the "+ store"
  // dialog, and tkInfo holds every listed store's registry entry.
  var tkApi = null, tkOrder = [], tkHidden = [], tkInfo = {}, tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSubs = [];
  var tkSupport = false, tkBusy = false, tkQueued = false, tkNext = [], tkLastRev = null, tkNudgedRev = null;
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
  // tkNotify tells the renderer that the cache changed; more marks a change
  // more of which lands shortly (see tkReadAll), which the renderer may hold
  // back briefly to draw together with the rest.
  function tkNotify(more) { tkSubs.forEach(function (fn) { fn(!!more); }); }
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
  // reads every store's view in parallel before done(). The renderer, once
  // mounted, is told as each view but the last lands (as a change with more
  // to come), so a slow store holds back only its own rows (it keeps its
  // previous view, or the loading state, meanwhile); the last view is left to
  // done's caller to announce.
  function tkReadAll(listed, done) {
    tkFetch("/tracker/stores", undefined, function (status, v) {
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
        [tkViews, tkErrs, tkSeen].forEach(function (m) { Object.keys(m).forEach(function (id) { if (!keep[id]) delete m[id]; }); });
      }
      listed(status);
      var left = status === 200 ? tkOrder.length : 0;
      if (!left) { done(); return; }
      tkOrder.forEach(function (id) {
        tkLoadView(id, function () {
          if (--left === 0) done();
          else if (tkApi) tkNotify(true);
        });
      });
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
    search: function (id, q, done) {
      tkFetch(tkUrl(id, "search?q=" + encodeURIComponent(q)), undefined, function (status, v) {
        if (status === 200 && Array.isArray(v.ids)) done({ ok: true, ids: v.ids });
        else done({ ok: false, error: tkErrText(status, v) });
      });
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
  // the rail's column on a wide screen. The store drawers open at the one
  // drawer height (drH) and a drag of a store's handle sets it.
  function tkMount() {
    var header = doc.querySelector(".dwrap > .top"), slot = $("qsslot"), wrap = doc.querySelector(".dwrap");
    if (!header || !slot || !wrap) return;
    header.classList.add("tk-room");
    if (tkSupport) tkAdapter.ext = supExt();
    tkApi = window.WsTracker.mount(tkAdapter, { header: header, qsSlot: slot, pageParent: wrap.querySelector(":scope > .dbody") || wrap, storagePrefix: "ws.tracker.", onRender: tlTracker,
      drawerHeight: { get: function () { return drH; }, set: drSetH } });
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

  // --- the selection popover ---
  function supPop(c, open) {
    c.pop = open; c.pending = null;
    c.popEl.hidden = !open;
    c.chip.setAttribute("aria-expanded", String(open));
    if (!open) { c.popEl.innerHTML = ""; return; }
    c.q = ""; c.qi = -1;
    supDrawPop(c);
    var f = c.popEl.querySelector(".spq");
    if (f) f.focus({ preventScroll: true });
    if (!supChoicesBusy) {
      supChoicesBusy = true;
      tkFetch("/support/choices", undefined, function (status, v) {
        supChoicesBusy = false;
        if (status === 200 && Array.isArray(v.harnesses)) supChoices = v;
        else if (!supChoices) supChoices = { harnesses: [], error: tkErrText(status, v) };
        Object.keys(supChats).forEach(function (id) { if (supChats[id].pop) supDrawPop(supChats[id]); });
      });
    }
  }
  function supHarness(name) {
    var hs = (supChoices && supChoices.harnesses) || [];
    for (var i = 0; i < hs.length; i++) if (hs[i].harness === name) return hs[i];
    return null;
  }
  // supFuzzy scores id against query q: every character of q in order, a
  // run of adjacent ones and a start at a word boundary scoring higher; -1
  // when q is not a subsequence.
  function supFuzzy(id, qq) {
    if (!qq) return 0;
    var s = id.toLowerCase(), qs = qq.toLowerCase().replace(/\s+/g, ""), at = -1, score = 0, run = 0;
    for (var i = 0; i < qs.length; i++) {
      var j = s.indexOf(qs[i], at + 1);
      if (j < 0) return -1;
      run = j === at + 1 ? run + 1 : 0;
      score += 1 + run * 2 + (j === 0 || /[^a-z0-9]/.test(s[j - 1]) ? 3 : 0);
      at = j;
    }
    return score - s.length / 100;
  }
  function supModels(c) {
    var h = supHarness(supCur(c).harness), list = [{ id: "", label: wsT("js.sup.default") }];
    ((h && h.models) || []).forEach(function (m) { list.push({ id: m.id, label: m.id, thinking: m.thinking }); });
    if (!c.q) return list;
    return list.filter(function (m) { return m.id; }).map(function (m) { return { m: m, s: supFuzzy(m.id, c.q) }; })
      .filter(function (x) { return x.s >= 0; }).sort(function (a, b) { return b.s - a.s; }).map(function (x) { return x.m; });
  }
  function supDrawPop(c) {
    if (!c.pop) return;
    var cur = supCur(c), h = supHarness(cur.harness), hs = (supChoices && supChoices.harnesses) || [];
    var html = '<div class="spsec"><div class="splbl">' + wsT("js.sup.harness") + "</div>";
    if (!supChoices) html += '<p class="spnote">' + wsT("js.sup.loading") + "</p>";
    else if (supChoices.error) html += '<p class="spnote err">' + supEsc(supChoices.error) + "</p>";
    else {
      html += '<div class="spseg" role="radiogroup" aria-label="' + supEsc(wsT("js.sup.harness")) + '">' + hs.map(function (x) {
        var on = x.harness === cur.harness;
        return '<button type="button" role="radio" aria-checked="' + on + '" data-harness="' + supEsc(x.harness) + '"' + (x.available ? "" : " disabled") +
          (x.available ? "" : ' title="' + supEsc(wsT("js.sup.unavail", x.harness, x.error || "")) + '"') + ">" + supEsc(x.harness) + "</button>";
      }).join("") + "</div>";
    }
    if (c.pending) {
      html += '<div class="spwarn" role="alert"><span>' + supEsc(wsT("js.sup.switch.warn", c.pending)) + '</span><span class="spacts">' +
        '<button type="button" class="btn mini primary" data-sp="switch">' + wsT("js.sup.switch.ok") + "</button>" +
        '<button type="button" class="btn mini" data-sp="cancel">' + wsT("js.sup.switch.cancel") + "</button></span></div>";
    }
    html += "</div>";
    html += '<div class="spsec"><div class="splbl">' + wsT("js.sup.model") + '</div><input class="spq" type="search" autocomplete="off" spellcheck="false" placeholder="' +
      supEsc(wsT("js.sup.search")) + '" value="' + supEsc(c.q) + '"><ul class="splist" role="listbox" aria-label="' + supEsc(wsT("js.sup.model")) + '"></ul></div>';
    var levels = (h && h.efforts) || [], m = null;
    ((h && h.models) || []).forEach(function (x) { if (x.id === cur.model) m = x; });
    var noEffort = !levels.length || (m && m.thinking === false);
    html += '<div class="spsec"><div class="splbl">' + wsT("js.sup.effort") + '<span class="spval">' + supEsc(cur.effort || wsT("js.sup.default")) + "</span>" +
      (cur.effort && !noEffort ? '<button type="button" class="spreset" data-sp="effort-reset">' + wsT("js.sup.effort.reset") + "</button>" : "") + "</div>";
    if (noEffort) html += '<p class="spnote">' + wsT(levels.length ? "js.sup.noeffort" : "js.sup.noefforts") + "</p>";
    else {
      var at = levels.indexOf(cur.effort);
      html += '<div class="knob" role="slider" tabindex="0" aria-label="' + supEsc(wsT("js.sup.effort")) + '" aria-valuemin="0" aria-valuemax="' + (levels.length - 1) +
        '" aria-valuenow="' + Math.max(at, 0) + '" aria-valuetext="' + supEsc(cur.effort || wsT("js.sup.default")) + '"' + (at < 0 ? ' data-none=""' : "") + '><div class="ktrack">' +
        levels.map(function (lv, i) { return '<span class="kdot' + (i === at ? " on" : "") + '" style="left:' + supPct(i, levels.length) + '%" title="' + supEsc(lv) + '"></span>'; }).join("") +
        (at >= 0 ? '<span class="kthumb" style="left:' + supPct(at, levels.length) + '%"></span>' : "") + '</div><div class="kends"><span>' + supEsc(levels[0]) + "</span><span>" +
        supEsc(levels[levels.length - 1]) + "</span></div></div>";
    }
    if (c.conv) html += '<p class="spnote">' + wsT("js.sup.next") + "</p>";
    if (c.err) html += '<p class="spnote err">' + supEsc(c.err) + "</p>";
    var fq = doc.activeElement && doc.activeElement.classList && doc.activeElement.classList.contains("spq") && c.popEl.contains(doc.activeElement);
    var fk = doc.activeElement && doc.activeElement.classList && doc.activeElement.classList.contains("knob") && c.popEl.contains(doc.activeElement);
    var caret = fq ? [doc.activeElement.selectionStart, doc.activeElement.selectionEnd] : null;
    c.popEl.innerHTML = html;
    // the popover opens upward over the log and never past the chat's top
    var comp = c.popEl.parentNode;
    if (comp.offsetTop > 0) c.popEl.style.maxHeight = Math.max(160, comp.offsetTop - 12) + "px";
    supDrawModels(c);
    var kn = c.popEl.querySelector(".knob");
    if (kn) supKnob(c, kn, levels);
    if (fq) { var nq = c.popEl.querySelector(".spq"); nq.focus({ preventScroll: true }); try { nq.setSelectionRange(caret[0], caret[1]); } catch (x) { /* search fields */ } }
    if (fk && kn) kn.focus({ preventScroll: true });
  }
  function supPct(i, n) { return n < 2 ? 50 : i / (n - 1) * 100; }
  function supDrawModels(c) {
    var ul = c.popEl.querySelector(".splist");
    if (!ul) return;
    var cur = supCur(c).model, list = supModels(c);
    if (c.qi >= list.length) c.qi = list.length - 1;
    ul.innerHTML = list.length ? list.map(function (m, i) {
      return '<li role="option" aria-selected="' + (m.id === cur) + '" class="' + (i === c.qi ? "hi" : "") + '" data-model="' + supEsc(m.id) + '">' +
        (m.id ? supEsc(m.label) : "<i>" + supEsc(m.label) + "</i>") + "</li>";
    }).join("") : '<li class="spnone">' + wsT("js.sup.nomatch") + "</li>";
    var hi = ul.querySelector("li.hi");
    if (hi) hi.scrollIntoView({ block: "nearest" });
  }
  function supPopClick(c, e) {
    var t = e.target, b;
    if ((b = t.closest("[data-harness]"))) {
      var name = b.getAttribute("data-harness");
      if (name === supCur(c).harness) { c.pending = null; supDrawPop(c); return; }
      // switching an open conversation's harness replaces the conversation
      if (c.conv) { c.pending = name; supDrawPop(c); return; }
      supSelect(c, { harness: name });
      return;
    }
    if ((b = t.closest("[data-sp]"))) {
      var k = b.getAttribute("data-sp");
      if (k === "switch" && c.pending) { var p = c.pending; c.pending = null; supSelect(c, { harness: p }); }
      else if (k === "cancel") { c.pending = null; supDrawPop(c); }
      else if (k === "effort-reset") supSelect(c, { effort: "" });
      return;
    }
    if ((b = t.closest("li[data-model]"))) supPickModel(c, b.getAttribute("data-model"));
  }
  function supPickModel(c, id) {
    c.q = ""; c.qi = -1;
    if (id === supCur(c).model) { supDrawPop(c); return; }
    supSelect(c, { model: id });
  }
  function supPopKey(c, e) {
    if (e.key === "Escape") { e.preventDefault(); supPop(c, false); c.chip.focus(); return; }
    if (!e.target.classList.contains("spq") || e.isComposing) return;
    var list = supModels(c);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!list.length) return;
      c.qi = c.qi < 0 ? (e.key === "ArrowDown" ? 0 : list.length - 1) : (c.qi + (e.key === "ArrowDown" ? 1 : list.length - 1)) % list.length;
      supDrawModelsHi(c, list);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (c.qi >= 0 && list[c.qi]) supPickModel(c, list[c.qi].id);
    }
  }
  function supDrawModelsHi(c, list) {
    q(".splist li", c.popEl).forEach(function (li, i) { li.classList.toggle("hi", i === c.qi && !!list[i]); });
    var hi = c.popEl.querySelector(".splist li.hi");
    if (hi) hi.scrollIntoView({ block: "nearest" });
  }
  // supKnob wires the effort knob: a drag moves the thumb, snapping to the
  // nearest level, and its release (or an arrow key) selects that level.
  function supKnob(c, kn, levels) {
    var track = kn.querySelector(".ktrack"), drag = null;
    function at(x) {
      var r = track.getBoundingClientRect();
      return Math.round(Math.max(0, Math.min(1, (x - r.left) / (r.width || 1))) * (levels.length - 1));
    }
    function show(i) {
      var th = track.querySelector(".kthumb");
      if (!th) { th = doc.createElement("span"); th.className = "kthumb"; track.appendChild(th); }
      th.style.left = supPct(i, levels.length) + "%";
      q(".kdot", track).forEach(function (d, j) { d.classList.toggle("on", j === i); });
      kn.removeAttribute("data-none");
      kn.setAttribute("aria-valuenow", String(i));
      kn.setAttribute("aria-valuetext", levels[i]);
      var v = c.popEl.querySelector(".spval");
      if (v) v.textContent = levels[i];
    }
    function pick(i) { if (levels[i] !== supCur(c).effort) supSelect(c, { effort: levels[i] }); }
    kn.addEventListener("pointerdown", function (e) {
      if (e.button) return;
      drag = { i: at(e.clientX) };
      try { kn.setPointerCapture(e.pointerId); } catch (x) { /* the moves still land */ }
      show(drag.i);
      e.preventDefault();
      kn.focus({ preventScroll: true });
    });
    kn.addEventListener("pointermove", function (e) {
      if (!drag) return;
      var i = at(e.clientX);
      if (i !== drag.i) { drag.i = i; show(i); }
    });
    function end() { if (!drag) return; var i = drag.i; drag = null; pick(i); }
    kn.addEventListener("pointerup", end);
    kn.addEventListener("pointercancel", function () { drag = null; supDrawPop(c); });
    kn.addEventListener("keydown", function (e) {
      var cur = levels.indexOf(supCur(c).effort), i = cur;
      if (e.key === "ArrowRight" || e.key === "ArrowUp") i = Math.min(levels.length - 1, cur + 1);
      else if (e.key === "ArrowLeft" || e.key === "ArrowDown") i = cur < 0 ? 0 : Math.max(0, cur - 1);
      else if (e.key === "Home") i = 0;
      else if (e.key === "End") i = levels.length - 1;
      else return;
      e.preventDefault();
      if (i !== cur) { show(i); pick(i); }
    });
  }
  // A click outside an open popover (and outside its chip) closes it.
  doc.addEventListener("click", function (e) {
    Object.keys(supChats).forEach(function (id) {
      var c = supChats[id];
      if (c.pop && !c.popEl.contains(e.target) && !c.chip.contains(e.target) && e.target.isConnected) supPop(c, false);
    });
  });

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
  var TL_ASK = '.card.policy[id^="item-"], .arow[id^="item-"]';
  var tlHeld = {}, tlPill = 0, tlTk = 0, tlAsks = -1, tlPerm = 0, tlN = -1, tlBase = null;
  function tlRetitle() {
    var n = tlTk + tlPerm + (tlAsks >= 0 ? tlAsks : tlPill);
    if (n === tlN) return;
    tlN = n;
    if (tlBase === null) {
      tlBase = root.getAttribute("data-wb-title");
      if (tlBase === null) { tlBase = doc.title.replace(/^\(\d+\) /, ""); root.setAttribute("data-wb-title", tlBase); }
    }
    doc.title = (n ? "(" + n + ") " : "") + tlBase;
  }
  window.wbTitleCount = function (n) { tlPill = n; tlRetitle(); };
  // tlRebase replaces the label's base (a rename's new tab title, which a
  // head swap brings).
  function tlRebase(base) {
    if (!base || base === tlBase) return;
    tlBase = base;
    root.setAttribute("data-wb-title", base);
    tlN = -1;
    tlRetitle();
  }
  function tlRoot(name) { return tlHeld[name] ? tlHeld[name].root : region(name); }
  // tlDrafted: the pill's reply rule (a pick, a ticked ack, or a note).
  function tlDrafted(el) {
    var box = el.querySelector("label.ack > input"), n = el.querySelector("textarea.note");
    return !!(el.querySelector('.opt[data-opt][aria-pressed="true"]') || (box && box.checked) || (n && n.value.trim()));
  }
  // tlHeldAsks counts a held board the pill's way without its DOM: its asks,
  // less those drafted in the live board and those the newest sent records
  // (in the held conversation when one waits, else the live one) name.
  function tlHeldAsks() {
    var b = tlRoot("board"), live = region("board"), c = tlRoot("convlive"), drafted = {}, sent = {};
    if (!b) return 0;
    if (live) q(TL_ASK, live).forEach(function (el) { if (tlDrafted(el)) drafted[el.getAttribute("data-id")] = 1; });
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
    tlHeld[name] = { hash: hash, root: frag(html) };
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
          else tlNotice(x);
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
    q(".dwrap > .top .sse").forEach(function (el) {
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
  function connect() {
    if (!window.WebSocket) return;
    var path = mode === "index" ? "/events" : "/s/" + rid + "/events";
    var st = openStream(path + "?h=" + encodeURIComponent(held()), {
      open: function () { connected(true); },
      // Reopen with the hashes held now, so a reconnect sends only what
      // changed since.
      drop: function () {
        connected(false);
        trackerLost();
        setTimeout(connect, 3000);
      },
      events: { assets: onAssets, region: onRegion, tracker: onTracker, gone: onGone }
    });
    function onRegion(ev) {
      if (!ev) return;
      if (ev.name === "mode") {
        if (ev.html !== mode) modeChanged(ev.html);
        modeHash = ev.hash;
        liveMode = ev.html === "live";
        syncBadge();
        return;
      }
      // A hidden tab keeps only each region's newest update and applies none;
      // showing the tab applies them (showRegions). The stream stays open, so
      // the light events (tracker) still arrive and act.
      if (doc.hidden) {
        delete hiddenRegions[ev.name];
        hiddenRegions[ev.name] = { hash: ev.hash, html: ev.html };
        tlHold(ev.name, ev.hash, ev.html);
        if (ev.name === "rail" || ev.name === "sessions") apScan(frag(ev.html));
        return;
      }
      swap(ev.name, ev.hash, ev.html);
    }
    function onTracker(ev) {
      if (ev && typeof ev.rev === "number") trackerNudge(ev.rev);
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
      if (holdsDraft() || tried === ev.id) { banner(wsT("js.banner.updated")); return; }
      try { window.sessionStorage.setItem(ASSETS_TRIED, ev.id); } catch (x) {}
      location.reload();
    }
    // The session left: the close that follows is final.
    function onGone() {
      st.close();
      connected(false);
      banner(wsT("js.banner.gone"));
    }
  }
  // showRegions applies, once each and in arrival order, the region updates
  // a hidden tab held back.
  function showRegions() {
    if (doc.hidden) return;
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

  // --- session rail ------------------------------------------------------------
  // The rail (r-rail) is one render shared by every session page: the page
  // marks its own row, pins the starred rows, and collapses the workspaces
  // the user collapsed (rail.js keeps those in storage), and opens the add
  // popovers (the next block). From 1600px the
  // rail is a column; below it the top-left arrow slides it in over the
  // page, and Esc, the scrim, or picking a session closes it. The arrow
  // stays a link to the sessions index for a modified click.
  var railEl = $("rail"), railBtn = $("railbtn"), rscrim = $("rscrim"), WsRail = window.WsRail;
  var railStore = { load: load, save: save };
  var starSet = WsRail ? WsRail.stars(railStore) : null, colSet = WsRail ? WsRail.collapsed(railStore) : null;
  var RAIL_WIDE = window.matchMedia ? window.matchMedia("(min-width: 1600px)") : null;
  var RAIL_SCROLL_KEY = "ws.dashboard.railscroll";
  // rp is the open rail popover (the next blocks): its controller P, kind,
  // what it concerns (ws: a workspace, the folder a new session starts in,
  // or the viewed session), and the button it hangs from.
  var rp = { P: null, kind: "", ws: "", btn: null }, rpWs = null, rpWt = null, rpLead = null, rpSess = null, rpIds = 0;
  function railWide() { return !!(RAIL_WIDE && RAIL_WIDE.matches); }
  function railShown() { return root.classList.contains("rail-open"); }
  function railOpen(open) {
    if (!railEl) return;
    root.classList.toggle("rail-open", !!open);
    if (railBtn) railBtn.setAttribute("aria-expanded", String(!!open || railWide()));
  }
  function railScrollTop() { var l = $("rlist"); return l ? l.scrollTop : null; }
  function railScrollTo(top) { var l = $("rlist"); if (l && top !== null && top !== undefined && l.scrollTop !== top) l.scrollTop = top; }
  // railMarks applies the page's marks to the rail as rendered: after
  // load and after every rail swap.
  function railMarks() {
    var r = region("rail");
    if (!r || !WsRail) return;
    q(".rsessions", r).forEach(function (box) { WsRail.pin(box, starSet); });
    WsRail.markCurrent(q(".rs", r), rid);
    WsRail.markRevive(q(".rs", r), revBusy);
    q(".rws", r).forEach(function (ws) {
      var col = colSet.has(ws.getAttribute("data-ws") || ""), tog = ws.querySelector(".wstog");
      ws.classList.toggle("col", col);
      if (tog) tog.setAttribute("aria-expanded", String(!col));
    });
    railPopSync();
  }
  if (railEl && WsRail) {
    if (railBtn) railBtn.addEventListener("click", function (e) {
      if (e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      e.preventDefault();
      railOpen(!railShown());
    });
    if (rscrim) rscrim.addEventListener("click", function () { railOpen(false); });
    doc.addEventListener("keydown", function (e) { if (e.key === "Escape" && railShown()) railOpen(false); });
    // Ctrl+` (the physical key, so any layout) toggles the overlay rail; at
    // 1600px and up the rail is a permanent column and the key is left alone.
    doc.addEventListener("keydown", function (e) {
      if (e.code !== "Backquote" || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.isComposing || railWide()) return;
      e.preventDefault();
      railOpen(!railShown());
    });
    if (RAIL_WIDE && RAIL_WIDE.addEventListener) RAIL_WIDE.addEventListener("change", function () { railOpen(false); });
    railEl.addEventListener("click", function (e) {
      var t = e.target;
      var star = t.closest && t.closest(".star");
      if (star) {
        e.preventDefault();
        var row = star.closest(".rs");
        if (row) starSet.toggle(row.getAttribute("data-rid") || "");
        railMarks();
        return;
      }
      var add = t.closest && t.closest(".radd");
      if (add) {
        e.preventDefault();
        railPopOpen(add.getAttribute("data-add"), add);
        return;
      }
      var forget = t.closest && t.closest(".rforget");
      if (forget) {
        e.preventDefault();
        railForget(forget);
        return;
      }
      var rev = t.closest && t.closest(".rrev");
      if (rev) {
        e.preventDefault();
        railRevive(rev);
        return;
      }
      var fresh = t.closest && t.closest(".rnew");
      if (fresh) {
        e.preventDefault();
        railPopOpen("lead", fresh);
        return;
      }
      // The whole bar toggles the workspace's collapse; its toggle button
      // (.wstog) is the keyboard's way to it.
      var bar = t.closest && t.closest(".wsbar");
      if (bar) {
        var ws = bar.closest(".rws");
        if (ws) colSet.toggle(ws.getAttribute("data-ws") || "");
        railMarks();
        return;
      }
      var ref = t.closest && t.closest("#railref");
      if (ref) {
        if (!window.fetch || ref.classList.contains("busy")) return;
        ref.classList.add("busy");
        fetch("/rail/refresh", { method: "POST", credentials: "same-origin" }).then(function () { ref.classList.remove("busy"); }, function () { ref.classList.remove("busy"); });
        return;
      }
      // Picking a session navigates; the draft is saved first and the
      // overlay closes, so a back navigation finds the page as it was.
      if (t.closest && t.closest("a[href]")) { saveDraft(); railOpen(false); }
    });
    // The rail list keeps its scroll across navigations.
    var savedRailTop = parseInt(load(RAIL_SCROLL_KEY), 10);
    window.addEventListener("pagehide", function () { var top = railScrollTop(); if (top !== null) save(RAIL_SCROLL_KEY, String(Math.round(top))); });
    railMarks();
    if (isFinite(savedRailTop)) railScrollTo(savedRailTop);
    railOpen(false);
  }

  // --- rail popovers: add workspace, add worktree, forget ----------------------
  // On a daemon that keeps stored workspaces the rail header's + adds a
  // workspace and a repository bar's + adds a worktree, each in a popover;
  // a stored workspace bar's forget takes it off the stored list at once
  // (never its files). The rail region is replaced on every update, so the
  // popovers live outside it: made once, placed fixed against the + that
  // opened it, and kept open across a swap, re-anchored to the new + by
  // railMarks. One popover at a time; Esc, an outside click, and cancel close
  // it. A route's error text is already in the page's language; the rail
  // shows what an add or a forget changed through the stream.
  // railPost posts body to a rail route; done(status, answer, text) gets the
  // HTTP status (0: the daemon was not reached), the parsed JSON answer of a
  // success ({} when there is none), and the body text of a failure.
  function railPost(route, body, done) {
    if (!window.fetch) { done(0, {}, wsT("js.post.nobrowser")); return; }
    fetch(route, {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        var v = {};
        if (r.ok) { try { v = s ? JSON.parse(s) : {}; } catch (e) { v = {}; } }
        done(r.status, v && typeof v === "object" ? v : {}, r.ok ? "" : s.trim());
      });
    }, function () { done(0, {}, ""); });
  }
  // railErr is a failed rail route's line: its own text, except a 404, which
  // names a workspace the rail no longer shows.
  function railErr(status, text) {
    if (status === 404) return wsT("js.rail.unknown");
    return text || (status ? "HTTP " + status : wsT("js.post.unreachable"));
  }
  function rpId() { return "rpop-" + (++rpIds); }
  function rpPop(title) {
    var el = mk("div", "rpop");
    el.hidden = true;
    el.setAttribute("role", "dialog");
    el.setAttribute("aria-label", title);
    return el;
  }
  function rpBtn(cls, text) { var b = mk("button", cls, text); b.type = "button"; return b; }
  function rpInput(cls, label) {
    var i = mk("input", cls);
    i.type = "text";
    i.spellcheck = false;
    i.setAttribute("autocomplete", "off");
    if (label) i.setAttribute("aria-label", label);
    return i;
  }
  // rpLive reports that popover P is still open as it was when gen was
  // taken: an answer that comes later closes nothing, and shows as a toast.
  function rpLive(P, gen) { return rp.P === P && P.gen === gen; }
  function rpEnter(el, fn) { el.addEventListener("keydown", function (e) { if (e.key === "Enter" && !e.isComposing) { e.preventDefault(); fn(); } }); }

  // The add-workspace popover: a pasted absolute path, read by the daemon's
  // preview while the user types (debounced; a stale answer is dropped).
  // "Add" is off while the input is empty or the latest preview refuses it;
  // a preview still on its way leaves it on, as the add reads the path again.
  function rpWsMake() {
    var P = { seq: 0, gen: 0, timer: 0, ok: null, busy: false };
    var el = P.el = rpPop(wsT("js.rail.ws.title"));
    el.appendChild(mk("div", "sphd", wsT("js.rail.ws.title")));
    var sec = el.appendChild(mk("div", "spsec"));
    var lbl = sec.appendChild(mk("label", "splbl", wsT("js.rail.ws.label")));
    var line = sec.appendChild(mk("div", "spline"));
    var inp = line.appendChild(rpInput("spq mono"));
    var go = line.appendChild(rpBtn("pbtn go", wsT("js.rail.ws.add")));
    var res = sec.appendChild(mk("div", "spres"));
    inp.id = rpId();
    lbl.htmlFor = inp.id;
    inp.placeholder = wsT("js.rail.ws.placeholder");
    res.id = rpId();
    res.setAttribute("aria-live", "polite");
    inp.setAttribute("aria-describedby", res.id);
    function show(cls, parts) {
      res.className = "spres" + (cls ? " " + cls : "");
      res.textContent = "";
      parts.forEach(function (p) { if (p) res.appendChild(p); });
    }
    function hint() { show("", [mk("span", "spnote", wsT("js.rail.ws.hint"))]); }
    function bad(text) { show("bad", [mk("span", "k", text)]); }
    function sync() { go.disabled = P.busy || !inp.value.trim() || P.ok === false; }
    function preview() {
      var text = inp.value, n = ++P.seq;
      clearTimeout(P.timer);
      P.ok = null;
      if (!text.trim()) { hint(); sync(); return; }
      sync();
      P.timer = setTimeout(function () {
        railPost("/rail/workspaces/preview", { path: text }, function (status, v, s) {
          if (n !== P.seq) return;
          if (status !== 200) bad(railErr(status, s));
          else if (!v.ok) { P.ok = false; bad(String(v.note || "")); }
          else {
            P.ok = true;
            show("", [mk("span", "k", wsT("js.rail.ws.will")), mk("code", null, String(v.root || v.path || "")), v.note ? mk("span", "spnote", String(v.note)) : null]);
          }
          sync();
        });
      }, 200);
    }
    function submit() {
      sync();
      if (go.disabled) return;
      var gen = P.gen;
      P.busy = true;
      P.seq++;
      clearTimeout(P.timer);
      sync();
      railPost("/rail/workspaces/add", { path: inp.value }, function (status, v, s) {
        var live = rpLive(P, gen);
        if (status === 200) {
          if (live) railPopClose(false);
          toast(String(v.message || ""));
          return;
        }
        if (!live) { toast(railErr(status, s)); return; }
        P.busy = false;
        if (status === 409) P.ok = false;
        bad(railErr(status, s));
        sync();
      });
    }
    inp.addEventListener("input", preview);
    rpEnter(inp, submit);
    go.addEventListener("click", submit);
    P.open = function () { P.gen++; inp.value = ""; P.ok = null; P.busy = false; hint(); sync(); };
    P.focus = function () { inp.focus(); };
    P.close = function () { P.seq++; clearTimeout(P.timer); };
    return P;
  }

  // The add-worktree popover for one repository workspace: a new branch
  // (from a base, the root's current branch preselected) or an existing one
  // no worktree has checked out, and the folder (the daemon's default, or a
  // custom absolute path). The options load when it opens; a failure, and a
  // refused or failed add, show in its error line and keep it open.
  function rpWtMake() {
    var P = { seq: 0, gen: 0, opts: null, mode: "new", pick: "", custom: false, busy: false, ws: "" };
    var el = P.el = rpPop(wsT("js.rail.wt.title"));
    var hd = el.appendChild(mk("div", "sphd", wsT("js.rail.wt.title")));
    var ctx = hd.appendChild(mk("span", "spctx"));
    var bsec = el.appendChild(mk("div", "spsec"));
    bsec.appendChild(mk("div", "splbl", wsT("js.rail.wt.branch")));
    var seg = bsec.appendChild(mk("div", "spseg"));
    seg.setAttribute("role", "radiogroup");
    seg.setAttribute("aria-label", wsT("js.rail.wt.branch"));
    var modes = { "new": rpBtn("", wsT("js.rail.wt.new")), old: rpBtn("", wsT("js.rail.wt.old")) };
    Object.keys(modes).forEach(function (k) { modes[k].setAttribute("role", "radio"); modes[k].setAttribute("data-mode", k); seg.appendChild(modes[k]); });
    var lnew = bsec.appendChild(mk("div", "spline"));
    var name = lnew.appendChild(rpInput("spq mono", wsT("js.rail.wt.name")));
    lnew.appendChild(mk("span", "spfrom", wsT("js.rail.wt.from")));
    var base = lnew.appendChild(mk("select", "spsel"));
    base.setAttribute("aria-label", wsT("js.rail.wt.base"));
    var lold = bsec.appendChild(mk("div", "spsec"));
    var find = lold.appendChild(rpInput("spq mono", wsT("js.rail.wt.old")));
    find.type = "search";
    find.placeholder = wsT("js.rail.wt.search");
    var list = lold.appendChild(mk("ul", "splist"));
    list.id = rpId();
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-label", wsT("js.rail.wt.old"));
    find.setAttribute("aria-controls", list.id);
    var fsec = el.appendChild(mk("div", "spsec"));
    var flbl = fsec.appendChild(mk("div", "splbl", wsT("js.rail.wt.folder")));
    var tog = flbl.appendChild(rpBtn("spreset", wsT("js.rail.wt.custom")));
    var def = fsec.appendChild(mk("div", "spdir"));
    var defPath = def.appendChild(mk("code"));
    def.appendChild(mk("span", "spnote", wsT("js.rail.wt.default")));
    var own = fsec.appendChild(mk("div", "spown"));
    var folder = own.appendChild(rpInput("spq mono", wsT("js.rail.wt.folderpath")));
    own.appendChild(mk("span", "spnote", wsT("js.rail.wt.customnote")));
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var row = el.appendChild(mk("div", "sprow"));
    var status = row.appendChild(mk("span", "spnote"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn go", wsT("js.rail.wt.create")));
    function setErr(text) { err.textContent = text || ""; err.hidden = !text; }
    function sync() {
      Object.keys(modes).forEach(function (k) { modes[k].setAttribute("aria-checked", String(P.mode === k)); });
      lnew.hidden = P.mode !== "new";
      lold.hidden = P.mode !== "old";
      def.hidden = P.custom;
      own.hidden = !P.custom;
      tog.textContent = wsT(P.custom ? "js.rail.wt.usedefault" : "js.rail.wt.custom");
      find.disabled = !P.opts || !P.opts.free.length;
      go.disabled = P.busy || !P.opts || (P.mode === "new" ? !name.value.trim() : !P.pick);
    }
    // drawList lists the free branches matching the search, keeping the
    // pick when it still matches, else picking the first match.
    function drawList() {
      var free = P.opts ? P.opts.free : [], hits = WsRail.matchBranches(free, find.value);
      if (hits.indexOf(P.pick) < 0) P.pick = hits[0] || "";
      list.textContent = "";
      find.removeAttribute("aria-activedescendant");
      if (P.opts && !hits.length) {
        var none = list.appendChild(mk("li", "spnone", wsT(free.length ? "js.rail.wt.nomatch" : "js.rail.wt.nofree")));
        none.setAttribute("aria-disabled", "true");
      }
      hits.forEach(function (b, i) {
        var li = list.appendChild(mk("li", null, b));
        li.id = list.id + "-" + i;
        li.setAttribute("role", "option");
        li.setAttribute("aria-selected", String(b === P.pick));
        li.setAttribute("data-branch", b);
        if (b === P.pick) find.setAttribute("aria-activedescendant", li.id);
      });
      sync();
    }
    function step(d) {
      var hits = WsRail.matchBranches(P.opts ? P.opts.free : [], find.value), i = hits.indexOf(P.pick) + d;
      if (!hits.length) return;
      P.pick = hits[Math.max(0, Math.min(hits.length - 1, i))];
      drawList();
      var cur = list.querySelector('[aria-selected="true"]');
      if (cur && cur.scrollIntoView) cur.scrollIntoView({ block: "nearest" });
    }
    // edited drops an earlier add's error once the form changed.
    function edited() { if (!P.busy) setErr(""); sync(); }
    function setMode(m) {
      if (P.mode === m) return;
      P.mode = m;
      edited();
      (m === "new" ? name : find).focus();
    }
    function submit() {
      sync();
      if (go.disabled) return;
      var gen = P.gen;
      P.busy = true;
      setErr("");
      sync();
      var body = WsRail.worktreeBody(P.ws, { mode: P.mode, name: name.value, base: base.value, pick: P.pick, custom: P.custom, folder: folder.value, shown: P.opts.folder });
      railPost("/rail/worktrees/add", body, function (st, v, s) {
        var live = rpLive(P, gen);
        if (st === 200) {
          if (live) railPopClose(false);
          toast(String(v.message || ""));
          return;
        }
        if (!live) { toast(railErr(st, s)); return; }
        P.busy = false;
        setErr(railErr(st, s));
        sync();
      });
    }
    seg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-mode]"); if (b && !b.disabled) setMode(b.getAttribute("data-mode")); });
    name.addEventListener("input", edited);
    rpEnter(name, submit);
    find.addEventListener("input", function () { drawList(); edited(); });
    find.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); step(e.key === "ArrowDown" ? 1 : -1); }
    });
    rpEnter(find, submit);
    list.addEventListener("click", function (e) {
      var li = e.target.closest && e.target.closest("li[data-branch]");
      if (!li) return;
      P.pick = li.getAttribute("data-branch");
      drawList();
      edited();
      find.focus();
    });
    tog.addEventListener("click", function () {
      P.custom = !P.custom;
      if (P.custom && !folder.value && P.opts) folder.value = P.opts.folder;
      edited();
      (P.custom ? folder : tog).focus();
    });
    folder.addEventListener("input", edited);
    rpEnter(folder, submit);
    cancel.addEventListener("click", function () { railPopClose(true); });
    go.addEventListener("click", submit);
    P.open = function (ws, label) {
      var n = ++P.seq;
      P.gen++;
      P.ws = ws; P.opts = null; P.mode = "new"; P.pick = ""; P.custom = false; P.busy = false;
      ctx.textContent = label || ws;
      ctx.title = ws;
      name.value = ""; find.value = ""; folder.value = "";
      base.textContent = "";
      defPath.textContent = "";
      status.textContent = wsT("js.rail.wt.loading");
      setErr("");
      drawList();
      railPost("/rail/worktrees/options", { workspace: ws }, function (st, v, s) {
        if (n !== P.seq) return;
        status.textContent = "";
        if (st !== 200) { setErr(railErr(st, s)); sync(); return; }
        var arr = function (x) { return Array.isArray(x) ? x.filter(function (b) { return typeof b === "string"; }) : []; };
        P.opts = { current: String(v.current || ""), bases: arr(v.bases), free: arr(v.free), folder: String(v.folder || "") };
        // A detached root has no current branch: the base then defaults to
        // its HEAD, sent as an empty base.
        if (!P.opts.current) base.appendChild(mk("option", null, wsT("js.rail.wt.head"))).value = "";
        P.opts.bases.forEach(function (b) { base.appendChild(mk("option", null, b)).value = b; });
        base.value = P.opts.current;
        defPath.textContent = P.opts.folder;
        drawList();
        railPopPlace();
      });
    };
    P.focus = function () { name.focus(); };
    P.close = function () { P.seq++; };
    return P;
  }

  // rpKey is what a popover of kind opened from btn concerns: the folder of
  // a new session, the viewed session, else the button's workspace.
  function rpKey(kind, btn) {
    if (kind === "sess") return rid;
    if (kind === "lead") return btn.getAttribute("data-root") || "";
    var ws = btn.closest(".rws");
    return ws ? ws.getAttribute("data-ws") || "" : "";
  }
  // railPopBtn is the button the open popover belongs to: the one that
  // opened it, or after a swap the button of the same kind and concern
  // (null when the page shows none any more).
  function railPopBtn() {
    if (!rp.P) return null;
    if (rp.btn && rp.btn.isConnected) return rp.btn;
    var r = region("rail"), h = region("head"), found = null;
    if (rp.kind === "sess") found = h && h.querySelector(".host.hbtn");
    else if (r) q(rp.kind === "lead" ? ".rnew" : '.radd[data-add="' + rp.kind + '"]', r).forEach(function (b) {
      if (rpKey(rp.kind, b) === rp.ws) found = b;
    });
    rp.btn = found;
    return found;
  }
  function railPopPlace() {
    var b = railPopBtn();
    if (!b) return;
    var el = rp.P.el, at = WsRail.popPlace(b.getBoundingClientRect(), el.offsetWidth, el.offsetHeight, root.clientWidth || window.innerWidth, root.clientHeight || window.innerHeight);
    el.style.top = at.top + "px";
    el.style.left = at.left + "px";
  }
  // railPopSync runs after a rail swap: the new + shows its popover open, and
  // a popover whose + is gone (its workspace left the rail) closes.
  function railPopSync() {
    if (!rp.P) return;
    var b = railPopBtn();
    if (!b) { railPopClose(false); return; }
    b.setAttribute("aria-expanded", "true");
    railPopPlace();
  }
  function railPopClose(refocus) {
    if (!rp.P) return;
    var P = rp.P, b = railPopBtn();
    P.close();
    P.el.hidden = true;
    rp = { P: null, kind: "", ws: "", btn: null };
    if (b) {
      b.setAttribute("aria-expanded", "false");
      if (refocus) b.focus();
    }
  }
  // railPopOpen opens kind's popover ("ws", "wt", "lead": a new session, or
  // "sess": the viewed session's menu) against the button btn; the open
  // popover's own button closes it instead.
  function railPopOpen(kind, btn) {
    var bar = btn.closest(".rws"), ws = rpKey(kind, btn);
    if (rp.P && rp.kind === kind && rp.ws === ws) { railPopClose(true); return; }
    railPopClose(false);
    var P = kind === "wt" ? rpWt || (rpWt = rpWtMake()) : kind === "lead" ? rpLead || (rpLead = rpLeadMake()) : kind === "sess" ? rpSess || (rpSess = rpSessMake()) : rpWs || (rpWs = rpWsMake());
    if (!P.el.parentNode) doc.body.appendChild(P.el);
    rp = { P: P, kind: kind, ws: ws, btn: btn };
    btn.setAttribute("aria-expanded", "true");
    var nm = bar && bar.querySelector(".nm");
    P.open(ws, nm ? nm.textContent : ws, btn);
    P.el.hidden = false;
    railPopPlace();
    P.focus();
  }
  // railRevive brings an ended row's session back by its entry's move (take
  // over a session a terminal ran, reopen a closed hosted one) and opens its
  // page. One post at a time: revBusy is the session posting, kept across
  // the rail's updates (railMarks marks its entry), and kept after a success
  // until the page goes. A blocked entry (aria-disabled) posts nothing; a
  // refusal shows the route's own text.
  var revBusy = "";
  function railRevive(btn) {
    var row = btn.closest(".rs"), id = row ? row.getAttribute("data-rid") || "" : "";
    var route = WsRail.reviveRoute(btn.getAttribute("data-revive") || "", id);
    if (!route || revBusy || btn.getAttribute("aria-disabled") === "true") return;
    revBusy = id;
    railMarks();
    railPost(route, {}, function (status, v, s) {
      if (status === 200) {
        saveDraft();
        if (id === rid) window.location.reload();
        else window.location.href = "/s/" + encodeURIComponent(id);
        return;
      }
      revBusy = "";
      railMarks();
      toast(leadErr(status, s));
    });
  }
  // railForget takes the bar's stored workspace off the list.
  function railForget(btn) {
    var bar = btn.closest(".rws"), path = bar ? bar.getAttribute("data-ws") || "" : "", nm = bar && bar.querySelector(".nm");
    if (!path || btn.disabled) return;
    btn.disabled = true;
    railPost("/rail/workspaces/forget", { path: path }, function (status, v, s) {
      btn.disabled = false;
      toast(status >= 200 && status < 300 ? wsT("js.rail.forgot", nm ? nm.textContent : path) : railErr(status, s));
    });
  }
  if (railEl && WsRail) {
    // Escape closes the popover before anything else hears it (the rail
    // overlay and the drawer would close on it too).
    window.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && rp.P) { e.preventDefault(); e.stopPropagation(); railPopClose(true); }
    }, true);
    // A click outside the popover and its + closes it. Capture sees the
    // target before the popover's own handlers redraw it.
    doc.addEventListener("click", function (e) {
      var t = e.target;
      if (!rp.P || !t || !t.closest) return;
      if (rp.P.el.contains(t) || t.closest(".radd, .rnew, .hbtn")) return;
      railPopClose(false);
    }, true);
    window.addEventListener("resize", railPopPlace);
    doc.addEventListener("scroll", function () { if (rp.P) railPopPlace(); }, true);
    if (RAIL_WIDE && RAIL_WIDE.addEventListener) RAIL_WIDE.addEventListener("change", function () { railPopClose(false); });
  }

  // --- daemon-hosted lead sessions ----------------------------------------------
  // A lead session the daemon spawned and feeds (the lead routes under
  // /agents) starts from the rail: the "+ session" at the end of a worktree
  // row or a plain-folder bar opens the spawn popover, a rail popover like
  // the two above. On such a session's page (the server marks it
  // data-hosted) the page reads the session's state from GET
  // /agents/<reply-id> on load, after a head update, and after a
  // conversation update that changed the permission line (the daemon
  // restamps that line on every approval change); no timer polls. From the
  // state it draws the approval cards, in a box under the conversation and
  // outside the regions so a card's typed deny message survives an update,
  // and makes the head's harness chip the button of the session menu (a
  // fourth rail popover). The controls show only when the state says this
  // viewer holds them (can_control: a browser on the daemon's machine); the
  // routes refuse anyone else whatever the page shows. A session handed to
  // a terminal (status handed_off) has no process here: it shows no
  // controls, only a notice with the command that resumes it there. The
  // server marks the page of such a session nothing runs now data-handed;
  // that page reads the state too, on load and after a head update.
  var hostedPage = mode !== "index" && root.hasAttribute("data-hosted");
  var handedPage = mode !== "index" && root.hasAttribute("data-handed");
  var LEAD_MODES = ["auto", "default", "acceptEdits", "plan", "dontAsk", "bypassPermissions"];
  var LEAD_MODE_NOTE = {
    auto: "js.lead.mode.auto", "default": "js.lead.mode.default", acceptEdits: "js.lead.mode.acceptedits",
    plan: "js.lead.mode.plan", dontAsk: "js.lead.mode.dontask", bypassPermissions: "js.lead.mode.bypass"
  };
  // The effort levels when the daemon's choices name none.
  var LEAD_EFFORTS = { claude: ["low", "medium", "high", "xhigh", "max"], pi: ["off", "minimal", "low", "medium", "high", "xhigh", "max"] };
  // lead is the viewed hosted session: its last read state, the read in
  // flight, the permission line the state was last read for, and the
  // approval cards by request id.
  var lead = { st: null, busy: false, again: false, key: "", cards: {}, need: null };
  // toolLabel is a tool's name as the server's rows show it: an MCP tool
  // without its server prefix.
  function toolLabel(name) {
    var m = /^mcp__.*?__(.+)$/.exec(name);
    return m ? m[1] : name;
  }
  function leadErr(status, text) { return text || (status ? "HTTP " + status : wsT("js.post.unreachable")); }
  // leadChoices reads the harness choices the support chat's popover uses
  // (its cache), then runs done.
  function leadChoices(done) {
    if (supChoices && !supChoices.error) { done(); return; }
    tkFetch("/support/choices", undefined, function (status, v) {
      if (status === 200 && Array.isArray(v.harnesses)) supChoices = v;
      done();
    });
  }
  function leadModels(harness) {
    var h = supHarness(harness);
    return ((h && h.models) || []).map(function (m) { return String(m.id); });
  }
  function leadEfforts(harness) {
    var h = supHarness(harness);
    return h && Array.isArray(h.efforts) && h.efforts.length ? h.efforts.map(String) : LEAD_EFFORTS[harness] || [];
  }
  // leadOptions lists "default" and list in sel with cur picked (added when
  // the list lacks it). An unchanged list is left alone, so a redraw never
  // closes an open select.
  function leadOptions(sel, list, cur) {
    var all = list.slice();
    if (cur && all.indexOf(cur) < 0) all.push(cur);
    var sig = all.join("\n");
    if (sel.wsSig !== sig) {
      sel.wsSig = sig;
      sel.textContent = "";
      sel.appendChild(mk("option", null, wsT("js.sup.default"))).value = "";
      all.forEach(function (v) { sel.appendChild(mk("option", null, v)).value = v; });
    }
    if (sel.value !== (cur || "")) sel.value = cur || "";
  }
  // leadRows builds, under box, the three option rows the spawn popover and
  // the session menu share (model, permission mode, effort; label left,
  // control right); changed(select) runs on a pick.
  function leadRows(box, changed) {
    var R = {};
    function row(label, cls) {
      var r = box.appendChild(mk("div", "sprc")), l = r.appendChild(mk("label", null, label)), s = r.appendChild(mk("select", "spsm" + (cls ? " " + cls : "")));
      s.id = rpId();
      l.htmlFor = s.id;
      s.addEventListener("change", function () { changed(s); });
      return { row: r, sel: s };
    }
    var m = row(wsT("js.sup.model"), "mono");
    var p = row(wsT("js.lead.mode"));
    var pd = box.appendChild(mk("p", "spdesc"));
    var e = row(wsT("js.sup.effort"));
    var ed = box.appendChild(mk("p", "spdesc"));
    LEAD_MODES.forEach(function (v) { p.sel.appendChild(mk("option", null, v)).value = v; });
    pd.id = rpId();
    p.sel.setAttribute("aria-describedby", pd.id);
    R.model = m.sel; R.mode = p.sel; R.effort = e.sel;
    // set shows harness's choices with v's model, effort, and mode picked.
    // Only a Claude session has a permission mode.
    R.set = function (harness, v) {
      var claude = harness === "claude";
      leadOptions(m.sel, leadModels(harness), v.model);
      leadOptions(e.sel, leadEfforts(harness), v.effort);
      p.row.classList.toggle("off", !claude);
      p.sel.disabled = !claude;
      p.sel.value = claude ? v.mode || "auto" : "";
      R.notes(harness, v.active);
    };
    // notes writes the lines under the mode and the effort; active is the
    // mode a running harness reported, named when it is not the picked one.
    R.notes = function (harness, active) {
      var claude = harness === "claude", mode = p.sel.value;
      pd.textContent = !claude ? wsT("js.lead.mode.pi") : wsT(LEAD_MODE_NOTE[mode] || "js.lead.mode.auto") + (active && active !== mode ? " " + wsT("js.lead.mode.active", active) : "");
      pd.classList.toggle("warn", claude && mode === "bypassPermissions");
      ed.textContent = e.sel.value ? (claude ? "--effort " : "--thinking ") + e.sel.value : wsT("js.lead.effort.default");
    };
    return R;
  }

  // The spawn popover for one worktree or plain folder: the target as one
  // read-only line (workspace, branch, path), the harness, and the option
  // rows. Start spawns the session and opens its page; the first message
  // is sent from that page's composer. A refused or failed spawn shows in
  // the popover's error line and keeps it open.
  function rpLeadMake() {
    var P = { gen: 0, busy: false, root: "", harness: "claude" };
    var el = P.el = rpPop(wsT("js.lead.title"));
    el.appendChild(mk("div", "sphd", wsT("js.lead.title")));
    var loc = el.appendChild(mk("div", "sploc"));
    var hsec = el.appendChild(mk("div", "spsec"));
    var seg = hsec.appendChild(mk("div", "spseg"));
    seg.setAttribute("role", "radiogroup");
    seg.setAttribute("aria-label", wsT("js.sup.harness"));
    var hbtn = { claude: rpBtn("", "Claude"), pi: rpBtn("", "Pi") };
    Object.keys(hbtn).forEach(function (k) { hbtn[k].setAttribute("role", "radio"); hbtn[k].setAttribute("data-harness", k); seg.appendChild(hbtn[k]); });
    var rows = leadRows(el.appendChild(mk("div", "spopts")), edited);
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var row = el.appendChild(mk("div", "sprow"));
    var status = row.appendChild(mk("span", "spnote"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn go", wsT("js.lead.start")));
    function setErr(text) { err.textContent = text || ""; err.hidden = !text; }
    function off(k) { var h = supHarness(k); return !!h && h.available === false; }
    function sync() {
      Object.keys(hbtn).forEach(function (k) {
        var h = supHarness(k);
        hbtn[k].setAttribute("aria-checked", String(P.harness === k));
        hbtn[k].disabled = off(k);
        hbtn[k].title = off(k) ? wsT("js.sup.unavail", k, h.error || "") : "";
      });
      go.disabled = P.busy || off(P.harness);
    }
    // edited drops an earlier spawn's error once the form changed.
    function edited() {
      if (!P.busy) setErr("");
      rows.notes(P.harness);
      sync();
      railPopPlace();
    }
    function setHarness(k) {
      if (P.harness === k) return;
      P.harness = k;
      rows.set(k, { model: "", effort: "", mode: "auto" });
      edited();
    }
    function submit() {
      sync();
      if (go.disabled) return;
      notifAsk();
      var gen = P.gen;
      P.busy = true;
      setErr("");
      status.textContent = wsT("js.lead.starting");
      sync();
      railPost("/agents/leads", WsRail.leadBody(P.root, { harness: P.harness, model: rows.model.value, effort: rows.effort.value, mode: rows.mode.value }), function (st, v, s) {
        if (st === 201 && v.id) {
          saveDraft();
          location.href = "/s/" + encodeURIComponent(String(v.id));
          return;
        }
        P.busy = false;
        status.textContent = "";
        if (!rpLive(P, gen)) { toast(leadErr(st, s)); return; }
        setErr(leadErr(st, s));
        sync();
        railPopPlace();
      });
    }
    seg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-harness]"); if (b && !b.disabled) setHarness(b.getAttribute("data-harness")); });
    cancel.addEventListener("click", function () { railPopClose(true); });
    go.addEventListener("click", submit);
    P.open = function (key, label, btn) {
      var gen = ++P.gen, name = btn.getAttribute("data-name") || label, branch = btn.getAttribute("data-branch") || "";
      P.busy = false;
      P.root = key;
      P.harness = "claude";
      loc.textContent = "";
      loc.appendChild(gIcon(branch ? "repo" : "wsfolder"));
      loc.appendChild(mk("span", null, name));
      if (branch) { loc.appendChild(gIcon("branch")); loc.appendChild(mk("b", null, branch)); }
      loc.appendChild(mk("code", null, key)).title = key;
      setErr("");
      status.textContent = "";
      rows.set(P.harness, { model: "", effort: "", mode: "auto" });
      sync();
      leadChoices(function () {
        if (!rpLive(P, gen)) return;
        // Claude is the first pick; Pi when only Pi can run.
        if (off(P.harness) && !off("pi")) P.harness = "pi";
        rows.set(P.harness, { model: rows.model.value, effort: rows.effort.value, mode: rows.mode.value || "auto" });
        sync();
        railPopPlace();
      });
    };
    P.focus = function () { (hbtn[P.harness].disabled ? cancel : hbtn[P.harness]).focus(); };
    P.close = function () {};
    return P;
  }

  // The session menu of the viewed hosted session, on the head's harness
  // chip: model, permission mode, and effort (a pick is sent at once and
  // applies from the next turn), stop (the process ends; the next message
  // or mail starts it again), restart (shown while an MCP server stays
  // broken: the process ends and a new one resumes the conversation), hand
  // to terminal (the process ends and the page gives the command that
  // resumes the conversation in a terminal), and close (the conversation
  // ends for good, after a confirmation). Stop, restart, and hand to
  // terminal confirm first while the harness runs background tasks, which
  // they end, and close's confirmation then names their count too. A
  // refused change shows in the menu's error line, and the rows return to
  // what the session has.
  function rpSessMake() {
    var P = { gen: 0, busy: false };
    var el = P.el = rpPop(wsT("js.lead.menu"));
    el.appendChild(mk("div", "sphd", wsT("js.lead.menu")));
    var rows = leadRows(el.appendChild(mk("div", "spopts")), function (sel) {
      var body = {};
      if (sel === rows.model) body.model = sel.value;
      else if (sel === rows.effort) body.effort = sel.value;
      else body.permission_mode = sel.value;
      act("settings", body);
    });
    el.appendChild(mk("p", "spnote", wsT("js.lead.menu.note")));
    var acts = el.appendChild(mk("div", "spacts"));
    function actBtn(icon, cls, title) {
      var b = acts.appendChild(rpBtn("spact" + cls));
      b.appendChild(typeof icon === "string" ? gIcon(icon) : icon);
      var s = b.appendChild(mk("span", null, title));
      return { b: b, note: s.appendChild(mk("span", "gs")) };
    }
    var stop = actBtn("stop", "", wsT("js.lead.stop")), restart = actBtn("reload", "", wsT("js.lead.restart"));
    var handoff = actBtn(luIcon("square-terminal"), "", wsT("js.lead.handoff")), close = actBtn("power", " danger", wsT("js.lead.close"));
    handoff.note.textContent = wsT("js.lead.handoff.note");
    close.note.textContent = wsT("js.lead.close.note");
    // bgTasks is the background task count of the last read state.
    function bgTasks() { return Number(lead.st && lead.st.background_tasks) || 0; }
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var failed = "";
    function act(action, body) {
      if (P.busy) return;
      var gen = P.gen;
      P.busy = true;
      failed = "";
      P.draw();
      railPost("/agents/" + encodeURIComponent(rid) + "/" + action, body, function (st, v, s) {
        P.busy = false;
        if (st === 200 && v.id) {
          lead.st = v;
          if (action === "close" && rpLive(P, gen)) railPopClose(false);
          leadDraw();
          return;
        }
        if (rpLive(P, gen)) failed = leadErr(st, s);
        else toast(leadErr(st, s));
        leadDraw();
        leadFetch();
      });
    }
    stop.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.stop.confirm", n))) act("stop", {});
    });
    restart.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.restart.confirm", n))) act("restart", {});
    });
    handoff.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.handoff.confirm", n))) act("handoff", {});
    });
    close.b.addEventListener("click", function () {
      var n = bgTasks();
      if (window.confirm(n > 0 ? wsTn("js.lead.close.confirm.bg", n) : wsT("js.lead.close.confirm"))) act("close", {});
    });
    // draw shows the session's state; it runs when the menu opens and after
    // every read of the state while it is open.
    P.draw = function () {
      var st = lead.st;
      if (!st) return;
      var text = failed || (st.error ? String(st.error) : "");
      rows.set(String(st.backend || ""), { model: String(st.model || ""), effort: String(st.effort || ""), mode: String(st.permission_mode || ""), active: st.process ? String(st.active_mode || "") : "" });
      rows.model.disabled = rows.effort.disabled = P.busy;
      if (st.backend === "claude") rows.mode.disabled = P.busy;
      stop.b.disabled = P.busy || !st.process;
      stop.note.textContent = wsT(st.process ? "js.lead.stop.note" : "js.lead.stopped");
      var broken = Array.isArray(st.mcp_broken) ? st.mcp_broken.map(String) : [];
      restart.b.hidden = !broken.length;
      restart.b.disabled = P.busy;
      handoff.b.disabled = P.busy;
      restart.note.textContent = broken.length ? wsT("js.lead.restart.note", broken.join(", ")) : "";
      close.b.disabled = P.busy;
      err.textContent = text;
      err.hidden = !text;
    };
    P.open = function () {
      var gen = ++P.gen;
      P.busy = false;
      failed = "";
      P.draw();
      leadChoices(function () { if (rpLive(P, gen)) { P.draw(); railPopPlace(); } });
      leadFetch();
    };
    P.focus = function () { rows.model.focus(); };
    P.close = function () {};
    return P;
  }

  // leadGone reports a state with no process to control: closed for good,
  // or handed to a terminal.
  function leadGone(st) { return !st || st.status === "closed" || st.status === "handed_off"; }
  function leadCan() { var st = lead.st; return !!(st && st.can_control && !leadGone(st)); }
  // leadFetch reads the viewed session's state and draws it; a read asked
  // for while one is in flight runs once after it.
  function leadFetch() {
    if ((!hostedPage && !handedPage) || !window.fetch) return;
    if (lead.busy) { lead.again = true; return; }
    lead.busy = true;
    tkFetch("/agents/" + encodeURIComponent(rid), undefined, function (status, v) {
      lead.busy = false;
      // A daemon that no longer runs the session answers 404: the page then
      // shows it as any session it only watches.
      if (status === 200 && v.id) lead.st = v;
      else if (status === 404) lead.st = null;
      if (status === 200 || status === 404) leadDraw();
      if (lead.again) { lead.again = false; leadFetch(); }
    });
  }
  // leadChip makes the head's harness chip the session menu's button for a
  // viewer who holds the controls. The head region is replaced on every
  // update, so this runs again after each.
  function leadChip() {
    var h = region("head"), chip = h && h.querySelector(".host.sse"), can = leadCan();
    if (!chip || can === chip.classList.contains("hbtn")) return;
    chip.classList.toggle("hbtn", can);
    if (can) {
      chip.setAttribute("role", "button");
      chip.setAttribute("tabindex", "0");
      chip.setAttribute("aria-haspopup", "dialog");
      chip.setAttribute("aria-label", wsT("js.lead.menu"));
      chip.setAttribute("aria-expanded", String(!!rp.P && rp.kind === "sess"));
      chip.appendChild(gIcon("chev"));
    } else {
      ["role", "tabindex", "aria-haspopup", "aria-label", "aria-expanded"].forEach(function (a) { chip.removeAttribute(a); });
      q("svg", chip).forEach(function (s) { s.remove(); });
    }
  }
  // apCard builds the card of approval request a: the tool, the request's
  // description, a readable preview of the input, and Allow and Deny (Deny
  // opens an optional message to send with it). A viewer without the
  // controls (can false) gets the card with one line saying where it is
  // answered. An answer that comes too late (409: another page answered, or
  // the harness withdrew the request) only reads the state again.
  function apCard(a, can) {
    var C = { busy: false };
    var el = C.el = mk("div", "apcard");
    var h = el.appendChild(mk("div", "aph"));
    h.appendChild(gIcon("lock"));
    h.appendChild(mk("b", null, wsT("js.ap.title")));
    h.appendChild(mk("code", null, toolLabel(String(a.tool || "")))).title = String(a.tool || "");
    if (a.agent_id) h.appendChild(mk("span", "apsub", wsT("js.ap.agent")));
    C.n = h.appendChild(mk("span", "apn"));
    if (a.started_at) {
      var t = h.appendChild(mk("time", "t"));
      t.setAttribute("data-ms", String(a.started_at));
      t.setAttribute("data-f", "since");
    }
    var b = el.appendChild(mk("div", "apb")), text = WsRail.approvalText(a.input);
    if (a.description) b.appendChild(mk("div", "apdesc", String(a.description)));
    if (text) b.appendChild(mk("pre", "apin", text));
    if (!can) {
      var ro = el.appendChild(mk("div", "apro"));
      ro.appendChild(gIcon("lock"));
      ro.appendChild(mk("span", null, wsT("js.ap.readonly")));
      return C;
    }
    function btn(box, cls, icon, label) {
      var x = box.appendChild(rpBtn(cls));
      if (icon) x.appendChild(gIcon(icon));
      x.appendChild(mk("span", null, label));
      return x;
    }
    var act = el.appendChild(mk("div", "apact"));
    var allow = btn(act, "btn allow", "check", wsT("js.ap.allow")), deny = btn(act, "btn deny", "x", wsT("js.ap.deny"));
    deny.setAttribute("aria-expanded", "false");
    var dn = el.appendChild(mk("div", "apdeny"));
    dn.hidden = true;
    var msg = dn.appendChild(mk("textarea", "apmsg"));
    msg.rows = 2;
    msg.placeholder = wsT("js.ap.deny.placeholder");
    msg.setAttribute("aria-label", wsT("js.ap.deny.placeholder"));
    var dr = dn.appendChild(mk("div", "aprow"));
    dr.appendChild(mk("span", "apnote", wsT("js.ap.deny.note")));
    var back = btn(dr, "btn", "", wsT("js.rail.cancel")), send = btn(dr, "btn go deny", "", wsT("js.ap.deny.send"));
    var err = el.appendChild(mk("p", "aperr"));
    err.hidden = true;
    err.setAttribute("aria-live", "polite");
    function sync() { [allow, deny, back, send, msg].forEach(function (x) { x.disabled = C.busy; }); }
    function denying(on) {
      var pin = pinned();
      dn.hidden = !on;
      act.hidden = on;
      deny.setAttribute("aria-expanded", String(on));
      (on ? msg : deny).focus();
      if (pin) pinConvo();
    }
    function answer(behavior) {
      if (C.busy) return;
      notifAsk();
      var body = { behavior: behavior }, why = msg.value.trim();
      if (behavior === "deny" && why) body.message = why;
      C.busy = true;
      err.hidden = true;
      sync();
      railPost("/agents/" + encodeURIComponent(rid) + "/approvals/" + encodeURIComponent(String(a.id)), body, function (st, v, s) {
        C.busy = false;
        sync();
        if (st === 200 && v.id) { lead.st = v; leadDraw(); return; }
        if (st === 409) { leadFetch(); return; }
        err.textContent = leadErr(st, s);
        err.hidden = false;
      });
    }
    allow.addEventListener("click", function () { answer("allow"); });
    deny.addEventListener("click", function () { denying(true); });
    back.addEventListener("click", function () { denying(false); });
    send.addEventListener("click", function () { answer("deny"); });
    msg.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !e.isComposing) { e.preventDefault(); answer("deny"); }
      else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); denying(false); }
    });
    return C;
  }
  // leadDraw shows the read state: the chip, the open session menu, one
  // card per waiting approval (oldest first, numbered when several wait),
  // and, when the harness waits on an answer no card can give, a notice in
  // their place. While the box shows anything, the conversation's own
  // permission line (the same wait, as text) is hidden.
  function leadDraw() {
    var st = lead.st, box = $("apbox");
    leadChip();
    if (rp.P && rp.kind === "sess") {
      if (leadCan()) rp.P.draw();
      else railPopClose(false);
    }
    handedDraw(st);
    if (!box) return;
    var open = !leadGone(st), pin = pinned(), keep = {};
    var list = open && Array.isArray(st.approvals) ? st.approvals.filter(function (a) { return a && a.id; }) : [];
    var need = open && !list.length && st.action_needed ? String(st.action_needed) : "";
    list.forEach(function (a) { keep[a.id] = true; });
    Object.keys(lead.cards).forEach(function (id) {
      if (keep[id]) return;
      lead.cards[id].el.remove();
      delete lead.cards[id];
    });
    list.forEach(function (a, i) {
      var c = lead.cards[a.id] || (lead.cards[a.id] = apCard(a, !!st.can_control));
      if (box.children[i] !== c.el) box.insertBefore(c.el, box.children[i] || null);
      c.n.textContent = (i + 1) + "/" + list.length;
      c.n.hidden = list.length < 2;
    });
    if (need && !lead.need) {
      lead.need = mk("div", "apneed");
      lead.need.appendChild(gIcon("lock"));
      lead.need.appendChild(mk("span", null, wsT("js.ap.needed")));
    }
    if (lead.need) {
      // the daemon's own wording, in English, stays readable on hover
      lead.need.title = need;
      if (need) box.appendChild(lead.need);
      else lead.need.remove();
    }
    box.hidden = !list.length && !need;
    if (convo) convo.classList.toggle("ap-on", !box.hidden);
    fmtTimes(box);
    if (pin) pinConvo();
  }
  // handedDraw shows, while st says the session was handed to a terminal,
  // the notice that says so under the banner, with the command that resumes
  // it there and its copy button when the state carries one; any other
  // state takes the notice away. The notice is outside the regions, so an
  // update leaves it.
  var handedEl = null;
  function handedDraw(st) {
    var n = WsRail ? WsRail.handedNotice(st) : null;
    if (!n) { if (handedEl) { handedEl.remove(); handedEl = null; } return; }
    if (handedEl && handedEl.wsKey === n.command) return;
    var banner = $("dash-banner");
    if (!banner) return;
    var el = mk("div", "handed");
    el.wsKey = n.command;
    el.setAttribute("role", "status");
    el.appendChild(luIcon("square-terminal"));
    var body = el.appendChild(mk("div", "hdbody"));
    body.appendChild(mk("span", "hdtext", wsT("js.lead.handed")));
    if (n.command) {
      body.appendChild(mk("span", "hdlbl", wsT("js.lead.handed.resume")));
      var line = body.appendChild(mk("span", "hdcmd"));
      line.appendChild(mk("code", null, n.command));
      var copy = line.appendChild(rpBtn("hdcopy"));
      copy.title = wsT("js.lead.handed.copy");
      copy.setAttribute("aria-label", wsT("js.lead.handed.copy"));
      copy.appendChild(gIcon("copy"));
      copy.addEventListener("click", function () { copyText(n.command); });
    }
    if (handedEl) handedEl.remove();
    banner.parentNode.insertBefore(el, banner.nextSibling);
    handedEl = el;
  }
  // leadKey is the conversation's permission line as the state follows it:
  // its tool, arguments, and stamp; empty when the session waits on none.
  function leadKey() {
    var c = region("convlive"), slot = c && c.querySelector(".apslot");
    if (!slot) return "";
    var t = slot.querySelector("time[data-ms]"), code = slot.querySelector("code"), args = slot.querySelector(".args");
    return [t ? t.getAttribute("data-ms") : "", code ? code.title : "", args ? args.textContent : ""].join("\n");
  }
  // leadSwapped runs after an update of a region the state follows.
  function leadSwapped(name) {
    if (handedPage && !hostedPage) {
      if (name === "head") leadFetch();
      return;
    }
    if (!hostedPage) return;
    if (name === "head") {
      leadChip();
      if (rp.P && rp.kind === "sess") railPopSync();
      leadFetch();
      return;
    }
    var key = leadKey();
    if (key === lead.key) return;
    lead.key = key;
    leadFetch();
  }

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
    if (!el) return;
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
    location.href = "/s/" + encodeURIComponent(r.rid);
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
      if (chip && railEl && WsRail) { e.preventDefault(); railPopOpen("sess", chip); }
      else if (t && t.closest && t.closest("#csend")) notifAsk();
    });
    doc.addEventListener("keydown", function (e) {
      var t = e.target;
      if ((e.key !== "Enter" && e.key !== " ") || !t || !t.classList || !t.classList.contains("hbtn") || !railEl || !WsRail) return;
      e.preventDefault();
      railPopOpen("sess", t);
    });
  }

  // --- per-session drafts ---------------------------------------------------------
  // What the user typed or picked on this session's page and has not sent,
  // and the page's view state around it, is kept per session in storage
  // (rail.js drafts), so switching sessions in the rail and coming back
  // finds it: the composer's text, mentions, and selection, the board
  // answers and topic notes, the answer chips, the images that reference
  // written attachments, the open details, and the tab. Pasted images are
  // not kept: a restored draft says so, and while the page holds one,
  // leaving it asks first (the browser's leave-page confirmation).
  var draftStore = WsRail && rid ? WsRail.drafts(railStore) : null, draftTimer = null, restoring = false;
  function readDraft() {
    var b = region("board"), open = {};
    Object.keys(openState).forEach(function (k) { if (openState[k]) open[k] = true; });
    var d = {
      text: ctext ? ctext.value : "",
      mentions: mentions.map(function (m) { return { start: m.start, end: m.end, text: m.text }; }),
      answers: b ? readAnswers(b) : {},
      chips: chipList(),
      bmentions: bmentions.slice(),
      refs: imgs.filter(function (p) { return !!p.ref; }).map(function (p) { return { ref: p.ref, name: p.name, n: p.n }; }),
      lost: imgs.filter(function (p) { return !p.ref; }).map(function (p) { return p.n; }),
      open: open,
      tab: live ? grid.getAttribute("data-tab") || "" : ""
    };
    if (ctext && d.text) d.sel = [ctext.selectionStart, ctext.selectionEnd];
    return d;
  }
  function saveDraft() {
    if (!draftStore || restoring) return;
    clearTimeout(draftTimer);
    draftTimer = null;
    draftStore.save(rid, readDraft());
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
      if (d.open && typeof d.open === "object") {
        Object.keys(d.open).forEach(function (k) { if (!Object.prototype.hasOwnProperty.call(openState, k)) openState[k] = true; });
        restoreOpen(doc);
      }
      var b = region("board");
      if (b && d.answers && typeof d.answers === "object") {
        writeAnswers(b, d.answers);
        if (window.wbForm) window.wbForm();
      }
      if (cchips && Array.isArray(d.chips)) {
        d.chips.forEach(function (c) { if (c && c.id && !chips[c.id]) { putChip(c); setItem(c.id, c); } });
        syncChips();
      }
      if (cchips && !bmentions.length && validMentions(d.bmentions).length) {
        bmentions = validMentions(d.bmentions);
        renderChips();
      }
      var lost = Array.isArray(d.lost) ? d.lost : [];
      if (ctext && typeof d.text === "string" && d.text && !ctext.value) {
        var s = WsRail.stripMarkers(d.text, Array.isArray(d.mentions) ? d.mentions : [], lost);
        ctext.value = s.text;
        mentions = s.mentions.filter(function (m) { return liveMention(m, s.text); });
        mentionWas = ctext.value;
        (Array.isArray(d.refs) ? d.refs : []).forEach(function (p) {
          if (!p || !p.ref || !markerNums(s.text)[p.n]) return;
          imgs.push({ name: p.name || p.ref, url: attachSrc(p.ref), data: null, ref: p.ref, n: p.n });
          imgSeq = Math.max(imgSeq, p.n);
        });
        imgs.sort(function (a, b) { return a.n - b.n; });
        grow();
        paintMentions();
        renderImgs();
        if (Array.isArray(d.sel)) { try { ctext.setSelectionRange(d.sel[0], d.sel[1]); } catch (e) { /* out of range */ } }
      }
      if (live && d.tab === "board") setTab("board");
      if (lost.length) syncNote(wsT("js.draft.imgslost", lost.length));
    } finally {
      restoring = false;
    }
  }
  if (draftStore) {
    ["input", "change", "click", "toggle"].forEach(function (t) { doc.addEventListener(t, queueDraft, true); });
    window.addEventListener("pagehide", saveDraft);
    doc.addEventListener("visibilitychange", function () { if (doc.hidden) saveDraft(); });
    window.addEventListener("beforeunload", WsRail.leaveGuard(function () { return imgs.some(function (p) { return !p.ref; }); }));
  }

  if (live) {
    setLayout(load(LAYOUT_KEY));
    setSplit(savedSplit());
    setInput(load(INPUT_KEY));
    setTab("convo");
    setCtab("convo");
  }
  restoreOpen(doc);
  // A body frame that posted its height before the listener above existed
  // posts it again.
  q("iframe.doc-frame").forEach(function (f) { if (f.contentWindow) f.contentWindow.postMessage({ wbDoc: "size" }, "*"); });
  if (window.wbForm) window.wbForm();
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
  nearTop();
  trackerStart();
  connect();
})();
