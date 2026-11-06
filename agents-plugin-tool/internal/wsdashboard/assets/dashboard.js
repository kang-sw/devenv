(function () {
  // The dashboard page script, shared by / and /s/<reply-id>. The server
  // renders every region; this script only swaps a region when an SSE event
  // names it, keeps the browser-side view state across swaps (open details,
  // scroll offsets, board answers, layout and tab), and formats times. It
  // never builds region HTML itself.
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
      else if (f === "ago") t.textContent = dur(now - ms) + " ago";
      else { var d = new Date(ms); t.textContent = two(d.getHours()) + ":" + two(d.getMinutes()); }
      t.title = new Date(ms).toLocaleString();
    });
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
  function restoreOpen(el) {
    q("details", el).forEach(function (d) {
      if (isPeer(d)) { d.open = load(PEER_KEY + d.getAttribute("data-peer")) === "1"; return; }
      var k = openKey(d);
      if (k && Object.prototype.hasOwnProperty.call(openState, k)) d.open = openState[k];
    });
  }

  // --- scroll offsets -------------------------------------------------------
  // The panes that scroll on their own; the page itself scrolls on / and on
  // the board-first page. A pane hidden by the tabs view loses its offset, so
  // each pane's last visible offset is kept and put back when it shows again.
  var PANES = ["bcol", "r-tree", "r-conversation"];
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

  // The conversation stays at the bottom while the user has it there.
  var convo = region("conversation"), stick = true;
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
  // visibility tells whether a tab is shown in the current view.
  var TAB = { tree: "convo", conversation: "convo", board: "board", panels: "board" };
  var PANE = { convo: "r-conversation", board: "bcol" };
  function tabShown(t) { return visible($(PANE[t])); }
  function tabDot(t) { var b = doc.querySelector('#tabbar [data-tab="' + t + '"]'); return b && b.querySelector(".tdot"); }
  function revealPanes() {
    PANES.forEach(function (id) {
      var el = $(id);
      if (visible(el) && Object.prototype.hasOwnProperty.call(paneScroll, id)) el.scrollTop = paneScroll[id];
    });
    pinConvo();
    Object.keys(PANE).forEach(function (t) { var d = tabDot(t); if (d && tabShown(t)) d.classList.remove("on"); });
  }
  function setLayout(v) {
    if (v !== "split" && v !== "tabs") v = "auto";
    grid.setAttribute("data-layout", v);
    q("#gmenu [data-layout]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-layout") === v)); });
    revealPanes();
  }
  function setTab(t) {
    grid.setAttribute("data-tab", t);
    q("#tabbar [data-tab]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-tab") === t)); });
    revealPanes();
  }
  if (live) {
    // "auto" crosses its breakpoint on a resize without a click.
    window.addEventListener("resize", revealPanes);
    doc.addEventListener("keydown", function (e) { if (e.key === "Escape" && gmenu && !gmenu.hidden) gmenu.hidden = true; });
  }

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
    return out;
  }
  function writeAnswers(el, saved) {
    q(".card[data-id], .arow[data-id]", el).forEach(function (item) {
      var s = saved[item.getAttribute("data-id")];
      if (!s) return;
      var note = item.querySelector(".note"), box = item.querySelector(".ack input");
      if (s.pick) q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) { if (pickKey(b) === s.pick) b.setAttribute("aria-pressed", "true"); });
      if (note && (s.note || s.noteOpen)) {
        note.value = s.note || "";
        note.hidden = false;
        note.style.height = "auto";
        if (note.scrollHeight) note.style.height = note.scrollHeight + "px";
      }
      if (box && s.checked) box.checked = true;
    });
  }
  function boardAnswers() {
    var b = region("board"), r = b && b.querySelector("#reply-text");
    return r ? r.value.trim() : "";
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
    var pin = name === "conversation" && pinned();
    var scroll = scrollState();
    var answers = board ? readAnswers(el) : null;
    el.innerHTML = html;
    el.setAttribute("data-h", hash);
    restoreOpen(el);
    fmtTimes(el);
    if (board) {
      writeAnswers(el, answers);
      if (window.wbForm) window.wbForm();
      mermaid();
      syncNote();
    }
    restoreScroll(scroll);
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
  }
  function mermaid() {
    if (!q("pre.mermaid:not([data-processed])").length || !window.wbMermaidFile) return;
    if (window.mermaid) { window.wbMermaid(); return; }
    var s = doc.createElement("script");
    s.src = window.wbMermaidFile; // relative: the server serves it beside the page
    s.onload = function () { window.wbMermaid(); };
    doc.head.appendChild(s);
  }

  // --- composer: never inside a region, so a swap never touches it ---------
  var ctext = $("ctext"), ccopy = $("ccopy"), cnote = $("cnote");
  function syncNote(text) {
    if (!cnote) return;
    cnote.textContent = text || (boardAnswers() ? "The board answers are copied with the message." : "");
  }
  function grow() { ctext.style.height = "auto"; ctext.style.height = Math.min(160, ctext.scrollHeight) + "px"; }
  // execCommand first: it works without a secure context or a permission;
  // the async clipboard API is the fallback.
  function copyText(s, done) {
    var prev = doc.activeElement, tmp = doc.createElement("textarea"), ok = false;
    tmp.value = s;
    tmp.setAttribute("readonly", "");
    tmp.style.position = "fixed"; tmp.style.top = "0"; tmp.style.opacity = "0";
    doc.body.appendChild(tmp);
    tmp.select();
    try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
    doc.body.removeChild(tmp);
    if (prev && prev.focus) prev.focus();
    if (ok || !navigator.clipboard) { done(ok); return; }
    navigator.clipboard.writeText(s).then(function () { done(true); }, function () { done(false); });
  }
  function copyMessage() {
    var t = ctext.value.trim(), a = boardAnswers(), s = t + (t && a ? "\n\n" : "") + a;
    if (!s) { syncNote("Nothing to copy: write a message or answer the board."); return; }
    copyText(s, function (ok) {
      syncNote(ok ? (a ? "Copied the message and the board answers." : "Copied.") : "Copy failed: select the text and copy it by hand.");
    });
  }
  if (ctext && ccopy) {
    ccopy.addEventListener("click", copyMessage);
    ctext.addEventListener("input", function () { grow(); syncNote(); });
    ctext.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); copyMessage(); }
    });
    // The form script's own listeners have rebuilt the reply line by the
    // time an event bubbles here.
    ["input", "change"].forEach(function (type) {
      doc.addEventListener(type, function (e) { if (e.target.closest && e.target.closest("#r-board")) syncNote(); });
    });
  }

  // --- clicks: delegated, so they survive every swap -----------------------
  doc.addEventListener("click", function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (cnote && t.closest("#r-board")) syncNote();
    if (!live) return;
    var b = t.closest("#gmenu [data-layout]");
    if (b) { save(LAYOUT_KEY, b.getAttribute("data-layout")); setLayout(b.getAttribute("data-layout")); return; }
    b = t.closest("#tabbar [data-tab]");
    if (b) { setTab(b.getAttribute("data-tab")); return; }
    if (t.closest("#gear")) { gmenu.hidden = !gmenu.hidden; return; }
    if (gmenu && !gmenu.hidden && !t.closest("#gmenu")) gmenu.hidden = true;
  });

  // --- stream ----------------------------------------------------------------
  var sse = $("sse"), sseLabel = sse && sse.querySelector(".sse-label");
  function connected(on) {
    if (!sse) return;
    sse.classList.toggle("off", !on);
    if (sseLabel) sseLabel.textContent = on ? "live" : "offline";
    sse.title = on ? "live updates" : "not connected, retrying";
  }
  function connect() {
    if (!window.EventSource) return;
    var path = mode === "index" ? "/events" : "/s/" + rid + "/events";
    var es = new EventSource(path + "?h=" + encodeURIComponent(held()));
    es.onopen = function () { connected(true); };
    es.onerror = function () {
      connected(false);
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
        return;
      }
      swap(ev.name, ev.hash, ev.html);
    });
    es.addEventListener("gone", function () {
      es.close();
      connected(false);
      banner("The dashboard no longer serves this session.");
    });
  }
  function banner(text) {
    var b = $("dash-banner");
    if (!b) return;
    b.textContent = text;
    b.hidden = false;
  }
  function modeChanged(next) {
    // The other mode has other regions, and a reload renders them; text typed
    // in the composer or answers picked on the board are never dropped for it.
    if ((ctext && ctext.value) || boardAnswers()) {
      banner("The session's live state " + (next === "live" ? "appeared" : "went away") + ". Reload to update the layout.");
      return;
    }
    location.reload();
  }

  if (live) {
    setLayout(load(LAYOUT_KEY));
    setTab("convo");
  }
  restoreOpen(doc);
  if (window.wbForm) window.wbForm();
  fmtTimes(doc);
  mermaid();
  pinConvo();
  connect();
})();
