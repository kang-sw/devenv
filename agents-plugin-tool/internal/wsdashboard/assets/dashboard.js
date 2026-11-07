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
    var t = $("r-tree"), c = $("r-conversation");
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
      syncChips();
      syncNote();
    }
    if (board || name === "conversation") applySent();
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
  var cimgs = $("cimgs"), imgs = [];
  var IMG_TYPES = { "image/png": 1, "image/jpeg": 1, "image/gif": 1, "image/webp": 1 };
  var IMG_MAX = 8, IMG_BYTES = 10 * 1024 * 1024;
  function attachSrc(name) { return "/s/" + rid + "/attachment/" + encodeURIComponent(name); }
  function addFiles(files) {
    var refused = "";
    files.forEach(function (f) {
      if (!IMG_TYPES[f.type]) { refused = (f.name || "the file") + " is not a PNG, JPEG, GIF, or WebP image"; return; }
      if (f.size > IMG_BYTES) { refused = (f.name || "the image") + " is larger than 10 MiB"; return; }
      if (imgs.length >= IMG_MAX) { refused = "a message holds at most " + IMG_MAX + " images"; return; }
      var it = { name: f.name || "pasted image", url: URL.createObjectURL(f), data: null, ref: "" };
      imgs.push(it);
      var rd = new FileReader();
      rd.onload = function () { var v = String(rd.result); it.data = v.slice(v.indexOf(",") + 1); };
      rd.onerror = function () { dropImg(it); renderImgs(); syncNote("Could not read " + it.name + "."); };
      rd.readAsDataURL(f);
    });
    renderImgs();
    syncNote(refused ? "Not attached: " + refused + "." : "");
  }
  function dropImg(it) {
    var i = imgs.indexOf(it);
    if (i < 0) return;
    imgs.splice(i, 1);
    if (!it.ref) URL.revokeObjectURL(it.url);
  }
  function renderImgs() {
    if (!cimgs) return;
    cimgs.textContent = "";
    imgs.forEach(function (it, i) {
      var el = doc.createElement("span"), im = doc.createElement("img"), x = doc.createElement("button");
      el.className = "ichip";
      im.src = it.url;
      im.alt = it.name;
      el.title = it.name;
      el.appendChild(im);
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
      // its text; a screenshot carries only the image.
      var types = Array.prototype.slice.call(e.clipboardData.types || []);
      if (types.indexOf("text/plain") < 0) e.preventDefault();
      addFiles(files);
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
      addFiles(imageFiles(e.dataTransfer));
      ctext.focus();
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
    if (up.length) body.images = up.map(function (p) { return { name: p.name, data: p.data }; });
    if (refs.length) body.refs = refs.map(function (p) { return p.ref; });
    sending = true;
    post("message", body, function (ok, res) {
      sending = false;
      if (!ok) { syncNote("Not sent: " + res); return; }
      if (ctext.value.trim() === t) { ctext.value = ""; grow(); }
      pics.forEach(dropImg);
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
      ctext.value = back + (back && cur ? "\n\n" : "") + cur;
      grow();
      // A chip attached since keeps its newer answer.
      (res.chips || []).forEach(function (c) { if (!chips[c.id]) { putChip(c); setItem(c.id, c); } });
      renderChips();
      // Its images come back as chips that reference the written files.
      (res.images || []).forEach(function (n) {
        if (!imgs.some(function (p) { return p.ref === n; })) imgs.push({ name: n, url: attachSrc(n), data: null, ref: n });
      });
      renderImgs();
      syncNote("Cancelled: the message is back in the composer.");
      ctext.focus();
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
  if (ctext) {
    var csend = $("csend");
    if (csend) csend.addEventListener("click", function () { sendMessage(); ctext.focus(); });
    ctext.addEventListener("input", function () { grow(); syncNote(); });
    ctext.addEventListener("keydown", function (e) {
      if (e.key !== "Enter") return;
      // An Enter that confirms an IME composition (Korean, Japanese,
      // Chinese) must never send: browsers report it as isComposing, and
      // Safari reports the first keydown after compositionend as keyCode 229.
      if (e.isComposing || e.keyCode === 229) return;
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
    if (u) { dropImg(imgs[+u.getAttribute("data-unimg")]); renderImgs(); syncNote(); return; }
    u = t.closest("#r-conversation [data-cancel]");
    if (u) { if (!u.disabled) cancelHeld(u); return; }
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
  if (window.wbForm) window.wbForm();
  applySent();
  fmtTimes(doc);
  mermaid();
  pinConvo();
  connect();
})();
