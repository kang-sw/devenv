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
      else if (f === "ago") t.textContent = dur(now - ms) + " ago";
      else if (f === "wd") t.textContent = new Date(ms).toLocaleDateString(undefined, { weekday: "short" });
      else { var d = new Date(ms); t.textContent = two(d.getHours()) + ":" + two(d.getMinutes()); }
      t.title = new Date(ms).toLocaleString();
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
  // The drawer's body (drbody) is never display:none, only clipped, so a
  // panels swap keeps its offset whether the drawer is open or closed.
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
    q("#gmenu [data-layout]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-layout") === v)); });
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
  var HINT = {
    enter: "Message this session. Enter sends, Shift+Enter adds a line. Board answers and pasted images attach.",
    mod: "Message this session. Ctrl/Cmd+Enter sends, Enter adds a line. Board answers and pasted images attach."
  };
  function setInput(v) {
    inputMode = v === "mod" ? "mod" : "enter";
    q("#gmenu [data-input]").forEach(function (b) { b.setAttribute("aria-pressed", String(b.getAttribute("data-input") === inputMode)); });
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
    var frameH = board ? frameHeight(el) : "";
    // A finished reply's mermaid fences draw once merged; the streaming
    // reply (convlive) keeps them code blocks.
    if (name === "entries") { mergeEntriesRegion(el, html); mermaid(); }
    else {
      el.innerHTML = html;
      restoreOpen(el);
      fmtTimes(el);
    }
    el.setAttribute("data-h", hash);
    if (board) {
      setFrameHeight(el, frameH);
      writeAnswers(el, answers);
      if (window.wbForm) window.wbForm();
      mermaid();
      syncChips();
      syncNote();
    }
    if (board || name === "convlive") applySent();
    if (name === "panels") drawerSync();
    restoreScroll(scroll);
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
    if (name === "panels") drawerSwapped();
    if (name === "tree" || name === "convlive") drawerWait();
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
      x.title = "remove this answer";
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
    if (!window.fetch) { done(false, "this browser cannot send"); return; }
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
    }, function () { done(false, "the dashboard is not reachable"); });
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
      if (!IMG_TYPES[f.type]) { refused = (f.name || "the file") + " is not a PNG, JPEG, GIF, or WebP image"; return; }
      if (f.size > IMG_BYTES) { refused = (f.name || "the image") + " is larger than 10 MiB"; return; }
      if (imgs.length >= IMG_MAX) { refused = "a message holds at most " + IMG_MAX + " images"; return; }
      var it = { name: f.name || "pasted image", url: URL.createObjectURL(f), data: null, ref: "", n: nextNum() };
      imgs.push(it);
      added.push(it.n);
      var rd = new FileReader();
      rd.onload = function () { var v = String(rd.result); it.data = v.slice(v.indexOf(",") + 1); };
      rd.onerror = function () { dropImg(it); release(it); deleteMarkers(it.n); renderImgs(); syncNote("Could not read " + it.name + "."); };
      rd.readAsDataURL(f);
    });
    renderImgs();
    insertMarkers(added);
    syncNote(refused ? "Not attached: " + refused + "." : "");
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
      el.title = "Image #" + it.n + ": " + it.name;
      el.appendChild(im);
      num.className = "inum";
      num.textContent = "#" + it.n;
      el.appendChild(num);
      x.className = "unimg";
      x.setAttribute("data-unimg", String(i));
      x.title = "remove this image";
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
  var sending = false;
  function sendMessage() {
    if (sending) return;
    var t = ctext.value.trim(), list = chipList(), pics = imgs.slice();
    if (!t && !list.length && !pics.length) { syncNote("Nothing to send: write a message, answer the board, or attach an image."); return; }
    if (pics.some(function (p) { return !p.ref && p.data === null; })) { syncNote("Still reading an image: send again in a moment."); return; }
    var body = { text: t, chips: list };
    var up = pics.filter(function (p) { return !p.ref; }), refs = pics.filter(function (p) { return !!p.ref; });
    if (up.length) body.images = up.map(function (p) { return { name: p.name, data: p.data, n: p.n }; });
    if (refs.length) {
      body.refs = refs.map(function (p) { return p.ref; });
      body.refNums = refs.map(function (p) { return p.n; });
    }
    sending = true;
    post("message", body, function (ok, res) {
      sending = false;
      if (!ok) { syncNote("Not sent: " + res); return; }
      if (ctext.value.trim() === t) { ctext.value = ""; grow(); }
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
      renderChips();
      syncNote(res.merged ? "Added to the held message." : "Sent.");
    });
  }
  function cancelHeld(btn) {
    btn.disabled = true;
    post("cancel", { id: btn.getAttribute("data-cancel") }, function (ok, res) {
      if (!ok) { btn.disabled = false; syncNote("Not cancelled: " + res); return; }
      var cur = ctext.value.trim(), back = res.text || "";
      // Its images come back as chips that reference the written files,
      // with their markers in the returned text. Numbers the draft already
      // uses are moved above the draft's highest, the markers with them.
      var names = res.images || [], nums = res.nums || [], top = imgSeq, renum = {};
      Object.keys(markerNums(cur)).forEach(function (k) { top = Math.max(top, +k); });
      imgs.forEach(function (p) { top = Math.max(top, p.n); });
      names.forEach(function (name, i) {
        var n = nums[i] || i + 1, have = imgs.filter(function (p) { return p.ref === name; })[0];
        renum[n] = have ? have.n : n + top;
        if (!have) imgs.push({ name: name, url: attachSrc(name), data: null, ref: name, n: renum[n] });
        imgSeq = Math.max(imgSeq, renum[n]);
      });
      back = back.replace(new RegExp(MARKER, "g"), function (m, k) { return renum[+k] ? marker(renum[+k]) : m; });
      // A message held before images were numbered has no markers: give
      // each image one, or the next edit would drop it.
      var marked = markerNums(back), missing = [];
      names.forEach(function (name, i) { var n = renum[nums[i] || i + 1]; if (!marked[n]) missing.push(marker(n)); });
      if (missing.length) back += (back ? "\n" : "") + missing.join(" ");
      ctext.value = back + (back && cur ? "\n\n" : "") + cur;
      grow();
      imgs.sort(function (a, b) { return a.n - b.n; });
      // A chip attached since keeps its newer answer.
      (res.chips || []).forEach(function (c) { if (!chips[c.id]) { putChip(c); setItem(c.id, c); } });
      renderChips();
      renderImgs();
      syncNote("Cancelled: the message is back in the composer.");
      ctext.focus();
    });
  }
  // steerHeld promotes a held message to steering: the session receives it
  // at its next tool call instead of when its turn ends. The convlive
  // region's next swap shows the steer marker in place of the button.
  function steerHeld(btn) {
    btn.disabled = true;
    post("steer", { id: btn.getAttribute("data-steer") }, function (ok, res) {
      if (!ok) { btn.disabled = false; syncNote("Not promoted: " + res); return; }
      syncNote("Promoted: the session receives it at its next tool call.");
    });
  }
  // applySent marks each board item the session was sent an answer for, or
  // holds one for, from the inbox records the conversation region carries.
  function applySent() {
    var b = region("board");
    if (!b) return;
    q(".sentmark", b).forEach(function (m) { m.parentNode.removeChild(m); });
    q(".sentdata i[data-id]").forEach(function (i) {
      var item = boardItem(i.getAttribute("data-id")), head = item && item.querySelector(".head");
      if (!head) return;
      var state = i.getAttribute("data-state"), m = doc.createElement("span");
      m.className = "sentmark " + state;
      m.textContent = state + " · " + (i.getAttribute("data-answer") || "note");
      var note = i.getAttribute("data-note");
      m.title = (state === "held" ? "held until the session's turn ends" : "sent to the session") + (note ? ": " + note : "");
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
    ctext.addEventListener("input", function () { grow(); syncNote(); syncMarkers(); });
    // Tab indents rather than leaving the composer; Escape, then Tab,
    // leaves it (the keyboard way out).
    var tabOut = false;
    ctext.addEventListener("blur", function () { tabOut = false; });
    ctext.addEventListener("keydown", function (e) {
      // A key that confirms an IME composition (Korean, Japanese, Chinese)
      // must never send or indent: browsers report it as isComposing, and
      // Safari reports the first keydown after compositionend as keyCode 229.
      if (e.isComposing || e.keyCode === 229) return;
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
    u = t.closest("#r-convlive [data-cancel]");
    if (u) { if (!u.disabled) cancelHeld(u); return; }
    u = t.closest("#r-convlive [data-steer]");
    if (u) { if (!u.disabled) steerHeld(u); return; }
    if (!live) return;
    var b = t.closest("#gmenu [data-input]");
    if (b) { save(INPUT_KEY, b.getAttribute("data-input")); setInput(b.getAttribute("data-input")); return; }
    b = t.closest("#gmenu [data-layout]");
    if (b) { save(LAYOUT_KEY, b.getAttribute("data-layout")); setLayout(b.getAttribute("data-layout")); return; }
    b = t.closest("#tabbar [data-tab]");
    if (b) { setTab(b.getAttribute("data-tab")); return; }
    if (t.closest("#gear")) { gmenu.hidden = !gmenu.hidden; return; }
    if (gmenu && !gmenu.hidden && !t.closest("#gmenu")) gmenu.hidden = true;
  });

  // --- link actions: the menu a served page's marked links open ------------
  // The renderer marks a served link data-wl="file" (href: the file view, or
  // no href outside the session root; data-rel, data-abs) or data-wl="url";
  // no other anchor is touched. A plain click opens the menu; Cmd/Ctrl+click
  // on a link with an href is left to the browser, which opens a new tab,
  // and opens the menu otherwise. The system-browser item shows only on a
  // page the server marked data-sysopen (a browser on the daemon's machine);
  // the open route checks that again.
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
      toast(ok ? "Copied " + s : "Could not copy: " + s);
    }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(s).then(function () { toast("Copied " + s); }, fallback);
    } else fallback();
  }
  function openLinkMenu(a, x, y) {
    closeLinkMenu(false);
    var kind = a.getAttribute("data-wl"), href = a.getAttribute("href");
    var rel = a.getAttribute("data-rel"), abs = a.getAttribute("data-abs"), url = a.href;
    var items = [];
    if (href) items.push(["Open in new tab", function () { window.open(url, "_blank", "noopener"); }]);
    if (href && root.hasAttribute("data-sysopen")) {
      items.push(["Open in system browser", function () {
        // A file link sends its file view path; the daemon opens it on the
        // address this page was loaded from.
        post("open", { target: kind === "url" ? url : href }, function (ok, v) { toast(ok ? "Opened in the system browser" : "Could not open: " + v); });
      }]);
    }
    if (kind === "url") items.push(["Copy URL", function () { copyText(url); }]);
    else {
      if (rel) items.push(["Copy relative path", function () { copyText(rel); }]);
      if (abs) items.push(["Copy absolute path", function () { copyText(abs); }]);
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
    var href = a.getAttribute("href");
    if (href && (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) return;
    e.preventDefault();
    linkAt(a, e);
  }, true);
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
  // A frame of registered tabs over the page. Each entry is { id, title,
  // icon, count, badge, content }: functions read at every sync, content
  // returning the element the tab shows or null when the tab is absent, and
  // badge (optional) a count for the handle or null. The four panels are the
  // first entries: views onto the one panels region, read again after each
  // swap, so a later tab registers here without reworking the frame. Open
  // state, height, and last tab are this browser's (ws.dashboard.drawer*).
  var drawer = $("drawer"), drclip = $("drclip"), drsheet = $("drsheet"), drtabs = $("drtabs"), drbody = $("drbody");
  var drhandle = $("drhandle"), drcounts = $("drcounts"), drheader = doc.querySelector(".dwrap > .top");
  var DR_OPEN = "ws.dashboard.drawer", DR_H = "ws.dashboard.drawer.h", DR_TAB = "ws.dashboard.drawer.tab";
  var DR_MIN = 160, DR_DEF = 320, DR_SHUT = 80;
  var drEntries = [], drWant = false, drOpen = false, drHas = false, drSel = null, drH = DR_DEF, drTop = 0, drTabScroll = {};
  function drawerTab(entry) { drEntries.push(entry); }
  var SVGNS = "http://www.w3.org/2000/svg";
  function iconEl(href) {
    var svg = doc.createElementNS(SVGNS, "svg"), use = doc.createElementNS(SVGNS, "use");
    svg.setAttribute("class", "i");
    svg.setAttribute("aria-hidden", "true");
    use.setAttribute("href", href);
    svg.appendChild(use);
    return svg;
  }
  // panelTab registers a panel of the panels region (#p-<id>) as a tab: its
  // title, icon, and count chip come from the panel's own title bar.
  function panelTab(id, badge) {
    function el() { var r = region("panels"); return r ? r.querySelector('[id="p-' + id + '"]') : null; }
    function head() { var p = el(); return p ? p.querySelector("summary") : null; }
    drawerTab({
      id: id,
      title: function () {
        var h = head();
        return h ? Array.prototype.filter.call(h.childNodes, function (n) { return n.nodeType === 3; }).map(function (n) { return n.nodeValue; }).join("").trim() : id;
      },
      icon: function () { var h = head(), u = h && h.querySelector("svg.i use"); return u ? u.getAttribute("href") : ""; },
      count: function () { var h = head(), c = h && h.querySelector(".chip"); return c ? c.textContent.trim() : ""; },
      badge: badge,
      content: el
    });
  }
  // The todo chip reads "done/total" and the mailbox chip "N unread".
  panelTab("todo", function (count) {
    var m = /^(\d+)\/(\d+)$/.exec(count), n = m ? +m[2] - +m[1] : 0;
    return n > 0 ? { n: n, title: n + (n === 1 ? " open todo" : " open todos") } : null;
  });
  panelTab("agenda");
  panelTab("tickets");
  panelTab("mailbox", function (count) {
    var m = /^(\d+) unread$/.exec(count), n = m ? +m[1] : 0;
    return n > 0 ? { n: n, title: n + " unread mail" } : null;
  });

  function drPresent() { return drEntries.filter(function (t) { return !!t.content(); }); }
  // drawerSync re-reads the entries: the tab strip, the shown panel, the
  // handle's counts, and whether there is a drawer at all. A sync never
  // changes the open state, the tab, or the scroll.
  function drawerSync() {
    if (!drawer) return;
    var present = drPresent(), ids = {};
    present.forEach(function (t) { ids[t.id] = true; });
    var sel = drSel && ids[drSel] ? drSel : present.length ? present[0].id : null;
    // tab buttons: updated in place, so a focused tab keeps its focus
    q("[data-drtab]", drtabs).forEach(function (b) { if (!ids[b.getAttribute("data-drtab")]) drtabs.removeChild(b); });
    present.forEach(function (t, i) {
      var b = drtabs.querySelector('[data-drtab="' + t.id + '"]');
      if (!b) {
        b = doc.createElement("button");
        b.type = "button";
        b.setAttribute("role", "tab");
        b.setAttribute("data-drtab", t.id);
      }
      if (drtabs.children[i] !== b) drtabs.insertBefore(b, drtabs.children[i] || null);
      var icon = t.icon(), title = t.title(), count = t.count();
      var sig = icon + "\n" + title + "\n" + count;
      if (b.getAttribute("data-sig") !== sig) {
        b.setAttribute("data-sig", sig);
        b.textContent = "";
        if (icon) b.appendChild(iconEl(icon));
        b.appendChild(doc.createTextNode(title));
        if (count) { var c = doc.createElement("span"); c.className = "chip"; c.textContent = count; b.appendChild(c); }
      }
      b.setAttribute("aria-selected", String(t.id === sel));
      b.tabIndex = t.id === sel ? 0 : -1;
    });
    drEntries.forEach(function (t) {
      var el = t.content();
      if (!el) return;
      el.hidden = t.id !== sel;
      if (el.tagName === "DETAILS" && !el.open) el.open = true;
    });
    drSel = sel;
    // handle counts
    drcounts.textContent = "";
    var tips = [];
    present.forEach(function (t) {
      var bd = t.badge && t.badge(t.count());
      if (!bd) return;
      var s = doc.createElement("span"), n = doc.createElement("b"), icon = t.icon();
      s.className = "dr-badge";
      s.title = bd.title;
      if (icon) s.appendChild(iconEl(icon));
      n.textContent = String(bd.n);
      s.appendChild(n);
      drcounts.appendChild(s);
      tips.push(bd.title);
    });
    drhandle.setAttribute("data-tip", tips.join(", "));
    drTitle();
    var has = present.length > 0;
    if (has !== drHas) {
      drHas = has;
      drhandle.hidden = !has;
      if (drheader) drheader.classList.toggle("dr-room", has);
      drOpen = has && drWant;
      drApply(false);
    }
  }
  function drTitle() {
    var t = (drOpen ? "close" : "open") + " the side panels: click or drag", tip = drhandle.getAttribute("data-tip");
    if (tip) t += " (" + tip + ")";
    if (drhandle.classList.contains("wait")) t += "; an approval is waiting";
    drhandle.title = t;
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
    var d = drhandle && drhandle.querySelector(".tdot");
    if (d && !drOpen && drHas) d.classList.add("on");
  }
  function drMeasure() {
    drTop = drheader ? Math.max(0, Math.round(drheader.getBoundingClientRect().bottom)) : 0;
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
    var h = drClamp(drH);
    drawer.classList.toggle("anim", !!anim);
    drawer.classList.toggle("open", drOpen);
    drsheet.style.height = h + "px";
    drclip.style.height = (drOpen ? h : 0) + "px";
    drhandle.style.top = (drOpen ? drTop + h : 0) + "px";
    drhandle.setAttribute("aria-expanded", String(drOpen));
    drTitle();
  }
  function drSetOpen(open) {
    if (!drHas) return;
    if (!open && drsheet.contains(doc.activeElement)) drhandle.focus();
    drWant = drOpen = open;
    save(DR_OPEN, open ? "1" : null);
    if (open) { var d = drhandle.querySelector(".tdot"); if (d) d.classList.remove("on"); }
    drApply(true);
  }
  function drSelect(id) {
    if (id === drSel) return;
    if (drSel) drTabScroll[drSel] = drbody.scrollTop;
    drSel = id;
    save(DR_TAB, id);
    drawerSync();
    drbody.scrollTop = drTabScroll[id] || 0;
  }
  if (drawer) {
    drWant = load(DR_OPEN) === "1";
    var savedH = parseFloat(load(DR_H));
    if (isFinite(savedH) && savedH > 0) drH = savedH;
    drSel = load(DR_TAB);
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
      if (!d || !d.moved) return;
      drawer.classList.remove("drag");
      drDragged = true;
      setTimeout(function () { drDragged = false; }, 0);
      if (d.ext < DR_SHUT) { drSetOpen(false); return; }
      drH = drClamp(d.ext);
      save(DR_H, String(drH));
      drWant = drOpen = true;
      save(DR_OPEN, "1");
      var dot = drhandle.querySelector(".tdot");
      if (dot) dot.classList.remove("on");
      drApply(false);
    }
    drhandle.addEventListener("pointerup", drEnd);
    drhandle.addEventListener("pointercancel", drEnd);
    drhandle.addEventListener("click", function () { if (!drDragged) drSetOpen(!drOpen); });
    drtabs.addEventListener("click", function (e) {
      var b = e.target.closest && e.target.closest("[data-drtab]");
      if (b) drSelect(b.getAttribute("data-drtab"));
    });
    drtabs.addEventListener("keydown", function (e) {
      var d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (!d) return;
      var bs = q("[data-drtab]", drtabs), i = bs.indexOf(doc.activeElement);
      if (i < 0) return;
      e.preventDefault();
      var b = bs[(i + d + bs.length) % bs.length];
      drSelect(b.getAttribute("data-drtab"));
      b.focus();
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
      if (t.closest("#drclip, #drhandle, .lmenu")) return;
      drSetOpen(false);
    }, true);
    // The header can wrap or change height (a chip appears): the sheet
    // follows its bottom edge. A resize re-clamps the height without saving.
    window.addEventListener("resize", function () { drApply(false); });
    if (window.ResizeObserver && drheader) new ResizeObserver(function () { drApply(false); }).observe(drheader);
    drOpen = drHas && drWant;
    drApply(false);
  }

  // --- the board's Now frame ------------------------------------------------
  // The server renders the board's Now slot in a sandboxed frame with an
  // opaque origin (wswhiteboard framedNow), so its scripts cannot reach this
  // page or the daemon's routes. The frame's script (now-frame.js) posts its
  // content height, its marked-link clicks, and its item-reference clicks. A
  // message counts only when its source is a Now frame on this page, and its
  // fields are data, checked as such: a link opens the same menu a board
  // link does, with an href kept only in the shapes the renderer gives one
  // (an http(s) URL for a url link, a scheme-less daemon path for a file
  // link), so a forged message can at most offer a menu the user must pick
  // from.
  function nowFrame(win) {
    return q("iframe.now-frame").filter(function (f) { return f.contentWindow === win; })[0];
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
    return a;
  }
  window.addEventListener("message", function (e) {
    var f = nowFrame(e.source), m = e.data;
    if (!f || !m || typeof m !== "object") return;
    if (m.wbNow === "height") {
      var h = num(m.height);
      if (h >= 0 && h < 100000) f.style.height = Math.ceil(h) + "px";
    } else if (m.wbNow === "jump") {
      var item = /^item-\d+$/.test(str(m.id)) && $(m.id);
      if (item) item.scrollIntoView({ block: "start" });
    } else if (m.wbNow === "link") {
      var a = frameLink(m);
      if (!a) return;
      var r = f.getBoundingClientRect();
      openLinkMenu(a, r.left + num(m.x), r.top + num(m.y));
    }
  });
  // A swap re-creates the frame, which starts at no height until it posts
  // one; it takes the old frame's height meanwhile.
  function frameHeight(el) { var f = el.querySelector("iframe.now-frame"); return f ? f.style.height : ""; }
  function setFrameHeight(el, h) { var f = el.querySelector("iframe.now-frame"); if (f && h) f.style.height = h; }

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
    if ((ctext && ctext.value) || boardAnswers() || imgs.length) {
      banner("The session's live state " + (next === "live" ? "appeared" : "went away") + ". Reload to update the layout.");
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
  // A Now frame that posted its height before the listener above existed
  // posts it again.
  q("iframe.now-frame").forEach(function (f) { if (f.contentWindow) f.contentWindow.postMessage({ wbNow: "size" }, "*"); });
  if (window.wbForm) window.wbForm();
  applySent();
  fmtTimes(doc);
  mermaid();
  pinConvo();
  nearTop();
  connect();
})();
