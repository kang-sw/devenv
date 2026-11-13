(function () {
  // The web card's injected script: served by each proxy listener at its
  // reserved path (webcard_proxy.go webInjectPath) and injected as
  // <script src> into every HTML page the proxy serves. The listener bakes
  // the daemon's port into PORT. The protocol is in webcard_routes.go's
  // file comment.
  var PORT = "__WS_DASHBOARD_PORT__";
  if (window.__wsWeb || window.parent === window) return;
  window.__wsWeb = true;
  var DASH = "http://" + location.hostname + ":" + PORT;
  // The inbox's caps (wsinbox): a message over one is refused, so every
  // value sent stays within them.
  var RING = 50, LINE_BYTES = 1024, SELECTOR_BYTES = 512, TEXT_BYTES = 1024, DEPTH = 8;
  function here() { return location.pathname + location.search + location.hash; }
  function post(m) { try { window.parent.postMessage(m, DASH); } catch (_) {} }

  // UTF-8 length, and a cut to at most max bytes that never splits a code
  // point (a lone surrogate counts as the 3-byte replacement character it
  // becomes on the server).
  function width(s, i) {
    var c = s.charCodeAt(i);
    if (c < 0x80) return 1;
    if (c < 0x800) return 2;
    if (c >= 0xd800 && c < 0xdc00 && i + 1 < s.length && (s.charCodeAt(i + 1) & 0xfc00) === 0xdc00) return 4;
    return 3;
  }
  function bytes(s) {
    var n = 0;
    for (var i = 0; i < s.length; i++) { var w = width(s, i); n += w; if (w === 4) i++; }
    return n;
  }
  function cut(s, max) {
    var n = 0;
    for (var i = 0; i < s.length; i++) {
      var w = width(s, i);
      if (n + w > max) return s.slice(0, i);
      n += w;
      if (w === 4) i++;
    }
    return s;
  }

  // The selector of a picked element: its #id when unique, else its
  // data-testid when unique, else a path of tag:nth-of-type(n) steps headed
  // by the nearest ancestor with a unique id (or html), at most DEPTH steps;
  // a path cut at DEPTH has no head and may match more than one element,
  // and is sent as built. A candidate over SELECTOR_BYTES is skipped; a path
  // over it drops its head and topmost steps until it fits.
  function esc(s) { return window.CSS && typeof CSS.escape === "function" ? CSS.escape(s) : ""; }
  function unique(sel) {
    if (bytes(sel) > SELECTOR_BYTES) return false;
    try { return document.querySelectorAll(sel).length === 1; } catch (_) { return false; }
  }
  function idSel(el) {
    var e = el.id && esc(el.id);
    return e && unique("#" + e) ? "#" + e : "";
  }
  function testidSel(el) {
    var v = el.getAttribute && el.getAttribute("data-testid"), e = v && esc(v);
    return e && unique('[data-testid="' + e + '"]') ? '[data-testid="' + e + '"]' : "";
  }
  function step(el) {
    var n = 1;
    for (var s = el.previousElementSibling; s; s = s.previousElementSibling) if (s.localName === el.localName) n++;
    return el.localName + ":nth-of-type(" + n + ")";
  }
  function pathSel(el) {
    var parts = [];
    for (var node = el; node && node.nodeType === 1; node = node.parentElement) {
      if (node === document.documentElement) { parts.unshift("html"); break; }
      var head = node !== el && idSel(node);
      if (head) { parts.unshift(head); break; }
      if (parts.length === DEPTH) break;
      parts.unshift(step(node));
    }
    while (parts.length > 1 && bytes(parts.join(" > ")) > SELECTOR_BYTES) parts.shift();
    var sel = parts.join(" > ");
    if (bytes(sel) <= SELECTOR_BYTES) return sel;
    return bytes(el.localName) <= SELECTOR_BYTES ? el.localName : "*";
  }
  function selector(el) { return idSel(el) || testidSel(el) || pathSel(el); }
  function pickText(el) {
    return cut(String(el.textContent || "").replace(/\s+/g, " ").trim(), TEXT_BYTES).trim();
  }

  // Pick mode: a highlight box follows the hovered element; a click picks
  // it and Escape cancels. The pointer and click events that make up the
  // pick are swallowed so the page does not act on them. Every change of
  // mode, and every pick command from the host, answers with picking{on}.
  var picking = null;
  var BOX = "all:initial!important;position:fixed!important;z-index:2147483647!important;pointer-events:none!important;" +
    "box-sizing:border-box!important;margin:0!important;padding:0!important;border:2px solid #2f81f7!important;" +
    "border-radius:2px!important;background:rgba(47,129,247,.18)!important;";
  function element(t) {
    while (t && t.nodeType !== 1) t = t.parentNode;
    return t && picking && t !== picking.box ? t : null;
  }
  function place() {
    var el = picking && picking.over;
    if (!picking) return;
    if (!el || el.isConnected === false) { picking.box.style.cssText = BOX + "display:none!important;"; return; }
    var r = el.getBoundingClientRect();
    picking.box.style.cssText = BOX + "display:block!important;left:" + r.left + "px!important;top:" + r.top +
      "px!important;width:" + r.width + "px!important;height:" + r.height + "px!important;";
  }
  function swallow(e) { e.preventDefault(); e.stopImmediatePropagation(); }
  function startPick() {
    if (!picking) {
      var box = document.createElement("div");
      box.setAttribute("aria-hidden", "true");
      picking = { box: box, over: null, on: {} };
      place();
      picking.on.mouseover = function (e) { var el = element(e.target); if (el) { picking.over = el; place(); } };
      picking.on.scroll = picking.on.resize = place;
      picking.on.pointerdown = picking.on.pointerup = picking.on.mousedown = picking.on.mouseup = swallow;
      picking.on.click = function (e) {
        swallow(e);
        var el = element(e.target);
        if (!el) return;
        post({ wsWeb: "pick", path: here(), selector: selector(el), text: pickText(el) });
        stopPick();
      };
      picking.on.keydown = function (e) {
        if (e.key !== "Escape") return;
        swallow(e);
        stopPick();
      };
      for (var k in picking.on) window.addEventListener(k, picking.on[k], true);
      (document.documentElement || document.body).appendChild(box);
    }
    post({ wsWeb: "picking", on: true });
  }
  function stopPick() {
    if (picking) {
      for (var k in picking.on) window.removeEventListener(k, picking.on[k], true);
      if (picking.box.parentNode) picking.box.parentNode.removeChild(picking.box);
      picking = null;
    }
    post({ wsWeb: "picking", on: false });
  }

  // The console ring: the last RING errors from runtime errors, unhandled
  // rejections, and console.error, each one line of at most LINE_BYTES.
  // errors counts every one since the page loaded or the last attach and is
  // posted at most once per tick, when it changed.
  var ring = [], errors = 0, sent = 0, tick = false;
  function isErr(x) { return !!x && typeof x === "object" && typeof x.message === "string" && (x instanceof Error || typeof x.stack === "string"); }
  function errText(x) {
    var s = (x.name ? x.name + ": " : "") + x.message;
    var st = typeof x.stack === "string" ? x.stack.split("\n") : [];
    for (var i = 0; i < st.length; i++) {
      if (/^\s*at\s|@.*:\d+(:\d+)?\s*$/.test(st[i])) return s + " " + st[i].trim();
    }
    return s;
  }
  function show(x) {
    if (typeof x === "string") return x;
    try {
      if (isErr(x)) return errText(x);
      var j = JSON.stringify(x);
      return j === undefined ? String(x) : j;
    } catch (_) {
      try { return String(x); } catch (_2) { return "[unprintable]"; }
    }
  }
  function flush() {
    tick = false;
    if (errors !== sent) { sent = errors; post({ wsWeb: "errors", count: errors }); }
  }
  function record(line) {
    line = cut(String(line).replace(/\r\n|\r|\n/g, " "), LINE_BYTES);
    if (!line.trim()) line = "(no message)";
    ring.push(line);
    if (ring.length > RING) ring.shift();
    errors++;
    if (!tick) { tick = true; setTimeout(flush, 0); }
  }
  function attach() {
    post({ wsWeb: "console", path: here(), lines: ring.slice() });
    ring = [];
    errors = sent = 0;
    post({ wsWeb: "errors", count: 0 });
  }
  window.addEventListener("error", function (e) {
    if (!e || (typeof e.message !== "string" && !e.error)) return;
    try {
      var m = e.message || show(e.error);
      if (e.filename) m += " (" + e.filename + ":" + (e.lineno || 0) + ":" + (e.colno || 0) + ")";
      record(m);
    } catch (_) {}
  });
  window.addEventListener("unhandledrejection", function (e) {
    try { record("Unhandled rejection: " + show(e && e.reason)); } catch (_) {}
  });
  var con = window.console, orig = con && con.error, busy = false;
  if (typeof orig === "function") {
    con.error = function () {
      if (!busy) {
        busy = true;
        try {
          var parts = [];
          for (var i = 0; i < arguments.length; i++) parts.push(show(arguments[i]));
          record(parts.join(" "));
        } catch (_) {}
        busy = false;
      }
      return orig.apply(con, arguments);
    };
  }

  window.addEventListener("message", function (e) {
    var m = e.data;
    if (e.source !== window.parent || e.origin !== DASH || !m || typeof m !== "object" || typeof m.wsWeb !== "string") return;
    if (m.wsWeb === "pick") {
      if (m.on === true) startPick(); else stopPick();
    } else if (m.wsWeb === "attach") {
      attach();
    }
  });
  function ready() { post({ wsWeb: "ready", path: here() }); }
  window.addEventListener("popstate", ready);
  window.addEventListener("hashchange", ready);
  ready();
})();
