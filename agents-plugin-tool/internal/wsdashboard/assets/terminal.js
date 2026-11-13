(function () {
  // The terminal card (window.WsTerm), a kind on the board card framework
  // (cards.js). A card shows one of the daemon's terminals: a shell on a
  // holder's PTY (terminals.go), reached through the terminal routes and
  // one WebSocket per card (terminal_routes.go documents the routes and the
  // frames). The emulator is the vendored xterm.js (assets/xterm), loaded
  // from beside the page when the first card needs it.
  //
  // The page offers terminals only when the server marked it data-term (a
  // browser on the daemon's machine, and a daemon that runs terminals); the
  // attribute's value is the emulator script's name. Without it this module
  // registers nothing, and no terminal control shows anywhere.
  //
  // A card's record is the terminal's id and folder. Its output is the
  // holder's: a card that opens asks for everything the holder's ring
  // holds (from=0), and one that loses its socket connects again from the
  // record after the last it took, so a gap the holder reports is shown at
  // its place in the output. The daemon is the record of which terminals
  // exist: the page reads its list on load and when it becomes visible
  // again, opens a card for each of this session's terminals that has none
  // (at the terminal's start time, so it stands where it stood), and drops
  // the cards whose terminal is gone.
  var doc = document, root = doc.documentElement, cards = window.WsCards;
  var script = root.getAttribute("data-term"), rid = root.getAttribute("data-rid");
  if (!cards || !script || !rid) return;
  var KIND = "terminal";
  // ROWS is the base height in rows; SCROLLBACK the lines the emulator
  // keeps; FIT_WAIT the resize debounce in ms; RETRY the reconnect delays
  // in ms, the last one repeating.
  var ROWS = 14, SCROLLBACK = 5000, FIT_WAIT = 150, RETRY = [300, 700, 1500, 3000, 6000, 10000];
  // IN_MAX is the most text one input frame carries, in UTF-16 units.
  var IN_MAX = 64 * 1024;
  // GUESS is a row's height in px before the emulator has measured one.
  var GUESS = 17;
  // SYMBOLS is the symbols-only Nerd Font the page's inlined stylesheet
  // declares (terminal_routes.go termSymbolsFamily: change the two
  // together). The emulator lists it after the page's monospace stack, so
  // letters keep that font and a prompt's powerline separators and icons
  // come from this one. The emulator's DOM renderer draws a fallback glyph
  // at its font's own width and only fixes its advance to the cell, so the
  // font's glyphs are redrawn to fit a cell (assets/nerdfont/fit.py); a 1em
  // glyph would spill into the next cell and show cut in half under that
  // cell's background. FONT_WAIT bounds the wait for it in ms: the
  // emulator measures its cells when it opens, so it opens once the font
  // is in or the wait is over.
  var SYMBOLS = "WsNerdSymbols", FONT_WAIT = 1500;
  var hooks = {}, loading = null, cellH = 0, ui = {}, folders = null, listSeq = 0;

  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function baseName(p) { var s = (p || "").replace(/[\\/]+$/, ""); return s.slice(Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\")) + 1) || p || ""; }
  function route(rest) { return "/s/" + rid + "/terminals" + (rest || ""); }
  function toast(s) { if (hooks.toast) hooks.toast(s); }
  function bytes(b64) {
    var s = atob(b64 || ""), n = s.length, a = new Uint8Array(n);
    for (var i = 0; i < n; i++) a[i] = s.charCodeAt(i);
    return a;
  }

  // emulator loads the vendored xterm.js and the symbols font once; it
  // resolves when window.Terminal and window.FitAddon are there and the
  // font is in, failed, or past FONT_WAIT.
  function emulator() {
    if (!loading) {
      var code = new Promise(function (ok, fail) {
        if (window.Terminal && window.FitAddon) { ok(); return; }
        var s = doc.createElement("script");
        s.src = "/s/" + encodeURIComponent(script);
        s.onload = function () { if (window.Terminal && window.FitAddon) ok(); else fail(); };
        s.onerror = function () { loading = null; fail(); };
        doc.head.appendChild(s);
      });
      var font = new Promise(function (ok) {
        if (!doc.fonts || !doc.fonts.load) { ok(); return; }
        setTimeout(ok, FONT_WAIT);
        doc.fonts.load('13px "' + SYMBOLS + '"', "\ue0b0").then(ok, ok);
      });
      loading = Promise.all([code, font]);
    }
    return loading;
  }

  // --- theme: the card's own colours, with an ANSI palette for each --------
  var DARK = {
    black: "#2b2d32", red: "#f0797e", green: "#6cc39a", yellow: "#e3b341", blue: "#7aa2ff", magenta: "#c79bf2", cyan: "#5fc4cc", white: "#d4d5d8",
    brightBlack: "#6b7079", brightRed: "#ff9a9e", brightGreen: "#8fd9b3", brightYellow: "#f2cc6b", brightBlue: "#a3c0ff", brightMagenta: "#dcbcfa", brightCyan: "#8adbe1", brightWhite: "#f3f3f0"
  };
  var LIGHT = {
    black: "#1b1d21", red: "#c0262d", green: "#1d7348", yellow: "#8a5a00", blue: "#2457c5", magenta: "#8a3fb5", cyan: "#0e7a83", white: "#8b9097",
    brightBlack: "#5f646b", brightRed: "#d9434a", brightGreen: "#2a8f5c", brightYellow: "#a36b00", brightBlue: "#2b62d9", brightMagenta: "#a052cf", brightCyan: "#138f99", brightWhite: "#b9bdc3"
  };
  // rgb reads a CSS colour as [r, g, b] through the browser's own parser.
  function rgb(color) {
    var probe = mk("span");
    probe.style.color = color;
    probe.style.display = "none";
    doc.body.appendChild(probe);
    var m = getComputedStyle(probe).color.match(/\d+(\.\d+)?/g) || [0, 0, 0];
    probe.parentNode.removeChild(probe);
    return [+m[0], +m[1], +m[2]];
  }
  function hex(c) { return "#" + c.map(function (v) { return ("0" + Math.round(v).toString(16)).slice(-2); }).join(""); }
  function theme() {
    var cs = getComputedStyle(root), bg = rgb(cs.getPropertyValue("--wb-card").trim() || "#fff"), fg = rgb(cs.getPropertyValue("--wb-fg").trim() || "#000");
    var dark = 0.299 * bg[0] + 0.587 * bg[1] + 0.114 * bg[2] < 128, pal = dark ? DARK : LIGHT, t = {};
    Object.keys(pal).forEach(function (k) { t[k] = pal[k]; });
    t.background = hex(bg);
    t.foreground = hex(fg);
    t.cursor = hex(fg);
    t.cursorAccent = hex(bg);
    t.selectionBackground = dark ? "#3a4a6b" : "#c9d8f7";
    return t;
  }
  function retheme() {
    var t = theme();
    cards.list(KIND).forEach(function (c) { if (c.t && c.t.term) c.t.term.options.theme = t; });
  }
  new MutationObserver(retheme).observe(root, { attributes: true, attributeFilter: ["data-theme"] });
  if (window.matchMedia) {
    var mq = window.matchMedia("(prefers-color-scheme: dark)");
    if (mq.addEventListener) mq.addEventListener("change", retheme);
  }

  // --- sizes -----------------------------------------------------------------
  // PAD is what the body holds besides the rows: the emulator's padding
  // (the stylesheet's .tm-host .xterm) and the body's top border.
  var PAD = 8 + 1;
  // base is the body height that fits ROWS rows, from the emulator's
  // measured row height once there is one.
  function base() { return Math.ceil(ROWS * (cellH || GUESS)) + PAD; }
  // measure reads the row height the emulator measured; a changed one
  // sets every card's base again.
  function measure(term) {
    var scr = term.element && term.element.querySelector(".xterm-screen"), h = 0;
    if (scr && term.rows) h = scr.getBoundingClientRect().height / term.rows;
    if (!(h > 0)) {
      try { h = term._core._renderService.dimensions.css.cell.height; } catch (e) { h = 0; }
    }
    if (h > 0 && Math.abs(h - cellH) > 0.01) {
      cellH = h;
      cards.list(KIND).forEach(function (c) { if (!c.folded) c.paint(); });
    }
  }
  function shows(c) { return !c.folded && c.t.host.clientHeight > 0 && c.t.host.clientWidth > 0; }
  // fit fits the emulator to the card's body and sends the size when its
  // rows or columns changed. A folded or hidden card fits nothing: its
  // body has no size, and the PTY keeps the last one.
  function fit(c) {
    var s = c.t;
    clearTimeout(s.fitTimer);
    s.fitTimer = 0;
    if (!s.term || s.gone || !shows(c)) return;
    measure(s.term);
    var d = null;
    try { d = s.fit.proposeDimensions(); } catch (e) { d = null; }
    if (!d || !(d.rows > 0) || !(d.cols > 0)) return;
    if (d.rows !== s.term.rows || d.cols !== s.term.cols) s.term.resize(d.cols, d.rows);
    sendSize(c);
  }
  // sendSize sends the emulator's size unless the daemon has it. A folded
  // or hidden card sends none, on a new connection too: its emulator was
  // never fitted, and its size would become the PTY's.
  function sendSize(c) {
    var s = c.t;
    if (!s.term || s.exited || !shows(c)) return;
    var r = s.term.rows, k = s.term.cols;
    if (s.sent && s.sent.rows === r && s.sent.cols === k) return;
    if (send(c, { t: "size", rows: r, cols: k })) s.sent = { rows: r, cols: k };
  }
  function later(c) {
    clearTimeout(c.t.fitTimer);
    c.t.fitTimer = setTimeout(function () { fit(c); }, FIT_WAIT);
  }

  // --- the socket ------------------------------------------------------------
  function send(c, frame) {
    var ws = c.t.ws;
    if (!ws || ws.readyState !== 1) return false;
    ws.send(JSON.stringify(frame));
    return true;
  }
  function connect(c) {
    var s = c.t;
    clearTimeout(s.retryTimer);
    s.retryTimer = 0;
    if (s.gone || s.ws) return;
    var url = (location.protocol === "https:" ? "wss:" : "ws:") + "//" + location.host + route("/" + encodeURIComponent(c.data.tid) + "/ws?from=" + (s.last ? s.last + 1 : 0));
    var ws;
    try { ws = new WebSocket(url); } catch (e) { retry(c); return; }
    s.ws = ws;
    s.sent = null;
    ws.onopen = function () {
      if (s.ws !== ws) return;
      fit(c);
      sendSize(c);
    };
    ws.onmessage = function (m) {
      if (s.ws !== ws) return;
      var f;
      try { f = JSON.parse(m.data); } catch (e) { return; }
      frame(c, f);
    };
    ws.onclose = function () {
      if (s.ws !== ws) return;
      s.ws = null;
      if (!s.gone) retry(c);
    };
  }
  // retry connects again after a delay that grows with each failure; a
  // second failure in a row also asks the daemon whether the terminal is
  // still there.
  function retry(c) {
    var s = c.t;
    if (s.gone || s.retryTimer) return;
    s.fails++;
    status(c);
    if (s.fails >= 2) reconcile();
    s.retryTimer = setTimeout(function () { s.retryTimer = 0; connect(c); }, RETRY[Math.min(s.fails - 1, RETRY.length - 1)]);
  }
  function frame(c, f) {
    var s = c.t;
    switch (f.t) {
      case "hello":
        s.fails = 0;
        s.live = true;
        if (f.state === "exited") ended(c, f);
        status(c);
        break;
      case "out":
        if (f.seq) s.last = f.seq;
        s.term.write(bytes(f.data));
        break;
      case "exit":
        if (f.seq) s.last = f.seq;
        ended(c, f);
        break;
      case "gap":
        // The marker in the output stands where the records went missing;
        // the line over the emulator says so while the marker may have
        // scrolled away under what followed (a gap is a full ring).
        if (f.to) s.last = f.to - 1;
        s.gaps++;
        s.term.write("\x1b[0m\r\n\x1b[7m " + wsT("js.term.gap") + " \x1b[0m\r\n");
        s.gap.hidden = false;
        break;
      case "closed":
        gone(c);
        break;
      case "ping":
        send(c, { t: "pong" });
        break;
    }
  }
  // ended shows the shell's exit status and a close button in the card,
  // and the terminal takes no more input.
  function ended(c, f) {
    var s = c.t;
    if (s.exited) return;
    s.exited = { code: f.exit_code, signal: f.signal };
    if (s.term) s.term.options.disableStdin = true;
    var text = f.signal != null ? wsT("js.term.exit.signal", f.signal) : f.exit_code != null ? wsT("js.term.exit.code", f.exit_code) : wsT("js.term.exit.done");
    s.endText.textContent = text;
    s.end.hidden = false;
    c.el.setAttribute("data-exited", "");
    status(c);
  }
  // gone drops a card whose terminal no longer exists, without asking.
  function gone(c) {
    c.t.gone = true;
    c.close();
  }
  // status writes the header: the folder, and the kind's word, or the
  // card's state while it is not plainly running.
  function status(c) {
    var s = c.t, word = wsT("js.term.kind");
    if (s.exited) word = wsT("js.term.state.exited");
    else if (s.fails && !(s.ws && s.ws.readyState === 1 && s.live)) word = wsT("js.term.state.wait");
    c.title({ icon: "square-terminal", name: baseName(c.data.cwd) || wsT("js.term.kind"), sub: c.data.cwd || "", badge: word });
  }

  // --- the folded line ---------------------------------------------------------
  // lastLine is the last line of the screen that holds anything: at a
  // prompt, the prompt.
  function lastLine(term) {
    var b = term.buffer.active, y = b.baseY + b.cursorY, stop = Math.max(0, y - term.rows);
    for (; y >= stop; y--) {
      var l = b.getLine(y), t = l ? l.translateToString(true) : "";
      if (t.trim()) return t;
    }
    return "";
  }
  function foldLine(c) {
    var s = c.t;
    if (!s.term) return;
    var t = lastLine(s.term);
    if (s.foldEl.textContent !== t) s.foldEl.textContent = t;
  }

  // --- the card ----------------------------------------------------------------
  function open(c) {
    var d = c.data || {};
    if (!d.tid) { setTimeout(function () { c.close(); }, 0); c.t = { gone: true }; return; }
    var s = c.t = { last: 0, fails: 0, gaps: 0, live: false, exited: null, gone: false, ws: null, term: null, born: Date.now() };
    c.body.textContent = "";
    s.gap = mk("div", "tm-gap");
    s.gap.hidden = true;
    s.gap.setAttribute("role", "status");
    s.gap.appendChild(mk("span", null, wsT("js.term.gap.note")));
    var dismiss = mk("button", "ibtn");
    dismiss.type = "button";
    dismiss.innerHTML = cards.icon("x");
    dismiss.title = wsT("js.term.gap.dismiss");
    dismiss.setAttribute("aria-label", wsT("js.term.gap.dismiss"));
    dismiss.addEventListener("click", function () { s.gap.hidden = true; if (s.term) s.term.focus(); });
    s.gap.appendChild(dismiss);
    s.host = mk("div", "tm-host");
    s.end = mk("div", "tm-end");
    s.end.hidden = true;
    s.endText = mk("span", "tm-endtx");
    var close = mk("button", "btn", wsT("js.card.close"));
    close.type = "button";
    close.addEventListener("click", function () { closing(c); });
    s.end.appendChild(s.endText);
    s.end.appendChild(close);
    c.body.appendChild(s.gap);
    c.body.appendChild(s.host);
    c.body.appendChild(s.end);
    s.foldEl = mk("div", "tm-fold");
    status(c);
    emulator().then(function () {
      if (s.gone) return;
      start(c);
    }, function () {
      if (s.gone) return;
      s.host.appendChild(mk("div", "tm-note", wsT("js.term.loadfail")));
    });
  }
  function start(c) {
    var s = c.t, cs = getComputedStyle(root);
    var term = s.term = new window.Terminal({
      scrollback: SCROLLBACK,
      fontFamily: (cs.getPropertyValue("--wb-mono").trim() || "monospace") + ', "' + SYMBOLS + '"',
      fontSize: 13,
      lineHeight: 1.15,
      cursorBlink: true,
      allowProposedApi: false,
      theme: theme()
    });
    s.fit = new window.FitAddon.FitAddon();
    term.loadAddon(s.fit);
    term.open(s.host);
    measure(term);
    if (s.exited) term.options.disableStdin = true;
    // In the maximize sheet Esc returns the terminal to its card (the
    // framework's Escape); in the card it reaches the shell.
    term.attachCustomKeyEventHandler(function (e) { return !(e.key === "Escape" && c.el.hasAttribute("data-max")); });
    term.onData(function (data) {
      if (s.exited) return;
      // A long paste goes in pieces under the daemon's frame limit, cut
      // between characters.
      for (var i = 0, n; i < data.length; i = n) {
        n = Math.min(i + IN_MAX, data.length);
        if (n < data.length && (data.charCodeAt(n - 1) & 0xfc00) === 0xd800) n--;
        send(c, { t: "in", data: data.slice(i, n) });
      }
    });
    term.onBinary(function (data) { if (!s.exited) send(c, { t: "in", bin: btoa(data) }); });
    // The viewer that last focused the terminal sizes the PTY: a focus, a
    // click into a terminal that already has the focus, and the window
    // coming back to a focused terminal all count.
    var focused = function () { send(c, { t: "focus" }); };
    if (term.textarea) term.textarea.addEventListener("focus", focused);
    s.host.addEventListener("mousedown", function () { if (term.textarea && doc.activeElement === term.textarea) focused(); });
    s.onWinFocus = function () { if (term.textarea && doc.activeElement === term.textarea) focused(); };
    window.addEventListener("focus", s.onWinFocus);
    var foldTimer = 0;
    term.onWriteParsed(function () {
      if (!c.folded || foldTimer) return;
      foldTimer = setTimeout(function () { foldTimer = 0; foldLine(c); }, 100);
    });
    if (window.ResizeObserver) {
      s.ro = new ResizeObserver(function () { later(c); });
      s.ro.observe(s.host);
    }
    fit(c);
    connect(c);
  }
  function stop(c) {
    var s = c.t;
    if (!s) return;
    s.gone = true;
    clearTimeout(s.retryTimer);
    clearTimeout(s.fitTimer);
    if (s.ro) s.ro.disconnect();
    if (s.onWinFocus) window.removeEventListener("focus", s.onWinFocus);
    if (s.ws) { var ws = s.ws; s.ws = null; try { ws.close(); } catch (e) { /* closing */ } }
    if (s.term) { try { s.term.dispose(); } catch (e) { /* gone */ } }
  }
  // closing asks before a running shell ends, then closes the terminal
  // through the daemon; the card goes when the daemon has. An exited
  // shell closes without the question (the close removes its holder).
  function closing(c) {
    var s = c.t;
    if (s.gone) return true;
    if (!s.exited && !window.confirm(wsT("js.term.confirm"))) return false;
    if (!hooks.post) return false;
    hooks.post("terminals/" + encodeURIComponent(c.data.tid) + "/close", {}, function (ok, v) {
      if (ok || /unknown terminal/.test(v)) gone(c);
      else toast(wsT("js.term.closefail", v));
    });
    return false;
  }
  function maximized(c) { return c.el.hasAttribute("data-max"); }

  cards.register(KIND, {
    base: base,
    open: open,
    closing: closing,
    close: stop,
    folded: function (c) {
      if (!c.t || !c.t.foldEl) return null;
      foldLine(c);
      return c.t.foldEl;
    },
    sized: function (c) {
      if (!c.t || !c.t.term) return;
      later(c);
      // A card maximized to work in takes the keyboard.
      if (maximized(c)) setTimeout(function () { if (maximized(c) && c.t.term) c.t.term.focus(); }, 0);
    }
  });

  // --- the daemon's list -------------------------------------------------------
  function cardFor(tid) { return cards.list(KIND).filter(function (c) { return c.data.tid === tid; })[0] || null; }
  function list(done) {
    fetch(route(""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw (t || "").trim() || "HTTP " + r.status; });
      return r.json();
    }).then(function (v) {
      folders = v.folders || null;
      done(null, v);
    }, function (e) { done(typeof e === "string" ? e : wsT("js.post.unreachable"), null); });
  }
  // reconcile opens a card for each of this session's terminals that has
  // none, at its start time, and drops the cards whose terminal the daemon
  // no longer lists. A card opened after the list was asked for stays.
  function reconcile() {
    var seq = ++listSeq, asked = Date.now();
    list(function (err, v) {
      if (err || seq !== listSeq) return;
      var known = {};
      (v.terminals || []).concat(v.orphans || []).forEach(function (t) { known[t.id] = t; });
      cards.list(KIND).forEach(function (c) { if (c.t && c.t.born < asked && !known[c.data.tid]) gone(c); });
      (v.terminals || []).forEach(function (t) { if (!cardFor(t.id)) cards.open(KIND, record(t), t.started_at || undefined); });
    });
  }
  function record(t) { return { tid: t.id, cwd: t.cwd || "" }; }
  // show opens a card for the terminal t, unless it has one, and shows it.
  function show(t) {
    var c = cardFor(t.id) || cards.open(KIND, record(t));
    if (!c) return null;
    c.fold(false);
    c.show();
    return c;
  }
  // REFUSAL maps the create route's folder refusals to their text.
  var REFUSAL = { not_absolute: "js.term.err.relative", not_found: "js.term.err.missing", not_a_folder: "js.term.err.notdir" };
  // create opens a terminal in the folder body names ({folder, path}) and
  // shows its card; done(err) gets the refusal's text, or null.
  function create(body, done) {
    if (!hooks.post) { done(wsT("js.post.nobrowser")); return; }
    var req = { folder: body.folder || "worktree", path: body.path || "", rows: ROWS, cols: guessCols() };
    hooks.post("terminals", req, function (ok, v) {
      if (!ok) { done(REFUSAL[v] ? wsT(REFUSAL[v]) : wsT("js.term.err.failed", v)); return; }
      show(v);
      done(null);
    });
  }
  // guessCols is the columns a new card will likely fit, so the shell
  // starts near its size: an open card's, or the board column's width.
  function guessCols() {
    var open = cards.list(KIND).filter(function (c) { return c.t && c.t.term; })[0];
    if (open) return open.t.term.cols;
    var col = doc.getElementById("bcol"), w = col ? col.clientWidth : 640;
    return Math.max(20, Math.min(400, Math.floor((w - 40) / 7.8)));
  }

  // --- the toolbar's new-terminal button and folder popover -------------------
  // The toolbar survives board swaps, including an open folder popover.
  // The popover opens below its button and stays inside the viewport.
  function closePop(refocus) {
    if (!ui.pop || ui.pop.hidden) return;
    ui.pop.hidden = true;
    ui.btn.setAttribute("aria-expanded", "false");
    if (refocus) ui.btn.focus();
  }
  function placePop() {
    var pop = ui.pop;
    var bounds = { left: 8, right: window.innerWidth - 8, top: 8, bottom: window.innerHeight - 8 };
    // A live board scrolls inside its column, which clips descendants even
    // when the viewport has room (notably beside the pinned column).
    var col = ui.col && ui.col.getClientRects().length ? ui.col.getBoundingClientRect() : null;
    if (col) {
      bounds.left = Math.max(bounds.left, col.left + 8);
      bounds.right = Math.min(bounds.right, col.right - 8);
      bounds.top = Math.max(bounds.top, col.top + 8);
      bounds.bottom = Math.min(bounds.bottom, col.bottom - 8);
    }
    pop.style.left = "0px";
    pop.style.top = "calc(100% + 8px)";
    pop.style.maxWidth = Math.max(0, bounds.right - bounds.left) + "px";
    pop.style.maxHeight = "";
    var r = pop.getBoundingClientRect();
    var left = Math.max(bounds.left - r.left, Math.min(0, bounds.right - r.right));
    pop.style.left = left + "px";
    // Stay below the button where space allows, and scroll rather than
    // clipping options when the toolbar is near the viewport's bottom.
    var top = Math.max(bounds.top, Math.min(r.top, bounds.bottom - 72));
    pop.style.top = (top - r.top + ui.btn.offsetHeight + 8) + "px";
    pop.style.maxHeight = Math.max(0, bounds.bottom - top) + "px";
  }
  function optRow(label, path, run) {
    var b = mk("button", "topt");
    b.type = "button";
    b.appendChild(mk("span", "tnm", label));
    var p = mk("span", "tpath", path || "");
    p.title = path || "";
    b.appendChild(p);
    b.addEventListener("click", run);
    return b;
  }
  function busy(on) {
    ui.pop.classList.toggle("busy", on);
    Array.prototype.forEach.call(ui.pop.querySelectorAll("button, input"), function (b) { b.disabled = on; });
  }
  function pick(body) {
    ui.err.textContent = "";
    busy(true);
    var run = function () {
      create(body, function (err) {
        busy(false);
        if (err) { ui.err.textContent = err; if (body.folder === "path") ui.in.focus(); return; }
        closePop(false);
      });
    };
    if (window.WsOverview) window.WsOverview.after(run); else run();
  }
  function drawOpts() {
    ui.opts.textContent = "";
    var f = folders || {};
    ui.opts.appendChild(optRow(wsT("js.term.pop.worktree"), f.worktree, function () { pick({ folder: "worktree" }); }));
    if (f.root) ui.opts.appendChild(optRow(wsT("js.term.pop.root"), f.root, function () { pick({ folder: "root" }); }));
  }
  function openPop() {
    ui.err.textContent = "";
    ui.in.value = "";
    drawOpts();
    ui.pop.hidden = false;
    placePop();
    ui.btn.setAttribute("aria-expanded", "true");
    var first = ui.opts.querySelector("button");
    if (first) first.focus();
    list(function (err) { if (!err && !ui.pop.hidden) { var a = doc.activeElement, i = Array.prototype.indexOf.call(ui.opts.children, a); drawOpts(); if (i >= 0 && ui.opts.children[i]) ui.opts.children[i].focus(); } });
  }
  function button() {
    var slot = doc.getElementById("board-term-slot");
    if (!slot) return;
    ui.btn = mk("button", "board-action tnew");
    ui.btn.type = "button";
    ui.btn.innerHTML = cards.icon("square-terminal");
    ui.btn.appendChild(mk("span", "board-label", wsT("js.term.new")));
    var arrow = mk("span", "term-arrow", "▾");
    arrow.setAttribute("aria-hidden", "true");
    ui.btn.appendChild(arrow);
    ui.btn.title = wsT("js.term.new.title");
    ui.btn.setAttribute("aria-label", wsT("js.term.new"));
    ui.btn.setAttribute("aria-haspopup", "dialog");
    ui.btn.setAttribute("aria-expanded", "false");
    slot.appendChild(ui.btn);
    var pop = ui.pop = mk("div", "tpop");
    pop.setAttribute("role", "dialog");
    pop.setAttribute("aria-label", wsT("js.term.pop.label"));
    pop.hidden = true;
    pop.appendChild(mk("div", "lb", wsT("js.term.pop.label")));
    ui.opts = mk("div", "topts");
    pop.appendChild(ui.opts);
    pop.appendChild(mk("div", "lb", wsT("js.term.pop.path")));
    var row = mk("div", "tprow");
    ui.in = mk("input", "fpin");
    ui.in.type = "text";
    ui.in.spellcheck = false;
    ui.in.setAttribute("autocomplete", "off");
    ui.in.placeholder = wsT("js.term.pop.placeholder");
    ui.in.setAttribute("aria-label", wsT("js.term.pop.placeholder"));
    ui.go = mk("button", "btn pr", wsT("js.term.pop.open"));
    ui.go.type = "button";
    row.appendChild(ui.in);
    row.appendChild(ui.go);
    pop.appendChild(row);
    ui.err = mk("div", "ferr");
    ui.err.setAttribute("role", "alert");
    pop.appendChild(ui.err);
    slot.appendChild(pop);

    ui.btn.addEventListener("click", function () { if (pop.hidden) openPop(); else closePop(true); });
    ui.go.addEventListener("click", function () {
      var p = ui.in.value.trim();
      if (!p) { ui.in.focus(); return; }
      pick({ folder: "path", path: p });
    });
    ui.in.addEventListener("keydown", function (e) { if (e.key === "Enter") { e.preventDefault(); ui.go.click(); } });
    pop.addEventListener("keydown", function (e) {
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); closePop(true); }
    });
    window.addEventListener("resize", function () { if (!pop.hidden) placePop(); });
    doc.addEventListener("mousedown", function (e) {
      if (!pop.hidden && !pop.contains(e.target) && !ui.btn.contains(e.target)) closePop(false);
    }, true);
  }

  window.WsTerm = {
    // setup hands the kind the page's own pieces: post (a session write
    // route, which passes a refusal's text through) and toast.
    setup: function (h) { hooks = h || {}; },
    // mount adds the toolbar's new-terminal button for cards in the board column
    // col and reads the daemon's list, again each time the page becomes
    // visible. The page mounts the card framework first.
    mount: function (col) {
      if (ui.btn || !col) return;
      ui.col = col;
      button();
      if (!ui.btn) return;
      reconcile();
      doc.addEventListener("visibilitychange", function () {
        if (doc.hidden) return;
        reconcile();
        cards.list(KIND).forEach(function (c) { if (c.t && c.t.term && !c.t.ws && !c.t.gone) connect(c); });
      });
    },
    create: create,
    // attach opens a card for the terminal t (a TerminalInfo), one this
    // session did not open included, and shows it.
    attach: show,
    // list reads the daemon's terminal list: done(err, list).
    list: list
  };
})();
