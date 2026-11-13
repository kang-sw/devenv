(function () {
  // The web card (window.WsWeb), a kind on the board card framework
  // (cards.js). A card shows a page of a localhost dev server, or an HTML
  // file under the session root, through the daemon's proxy: its iframe
  // loads the card's wrapper frame, which frames the proxied page on the
  // proxy's own origin and relays postMessage between that page's injected
  // script and this card. The routes and the message protocol are in
  // webcard_routes.go's file comment.
  //
  // Only a browser on the daemon's machine is offered web cards (the page's
  // data-sysopen): without it this module registers nothing, and no web
  // control shows anywhere.
  //
  // A card's record is its target, {url} (a dev server's page) or {path}
  // (a workspace file), never the registration id: every open registers the
  // target again and takes the id it answers, so a restored card works after
  // a daemon restart. The record follows the page the card shows, so a
  // restored card comes back to it.
  //
  // The card takes a message only when it comes from its own iframe's
  // window on this page's origin: the proxied page is the wrapper's inner
  // frame, whose own posts (another source) are dropped, and every post
  // this card makes names this page's origin. A pick or a console block is
  // taken only as the answer to the user's own toggle or attach, so a page
  // cannot put elements into the message on its own.
  var doc = document, root = doc.documentElement, cards = window.WsCards;
  if (!cards || !root.hasAttribute("data-sysopen")) return;
  var KIND = "web", hooks = {}, ui = {};
  // BASE is the body's base height in px. The byte limits are the inbox's
  // (wsinbox): a message over one is refused whole, so every value the
  // card hands the composer stays within them.
  var BASE = 420, URL_BYTES = 2048, SELECTOR_BYTES = 512, TEXT_BYTES = 1024, LINE_BYTES = 1024, LINES = 50;
  // SETTLE is how long after the frame's load the card waits for the
  // wrapper's frame message before it takes the load for a failed one, in ms.
  var SETTLE = 150;

  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function rid() { return hooks.rid || root.getAttribute("data-rid") || ""; }
  function toast(s) { if (hooks.toast) hooks.toast(s); }
  function str(v) { return typeof v === "string" ? v : ""; }
  function baseName(p) { var s = p.replace(/[\\/]+$/, ""); return s.slice(Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\")) + 1) || p; }

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
  function oneLine(s) { return s.replace(/[\r\n]+/g, " "); }

  // --- targets --------------------------------------------------------------------
  function parse(href) {
    try { return new URL(href); } catch (e) { return null; }
  }
  function defaultPort(u) { return u.port || (u.protocol === "https:" ? "443" : "80"); }
  function loopbackHost(h) { h = h.toLowerCase(); return h === "localhost" || h === "127.0.0.1" || h === "[::1]"; }
  // originKey is a URL's origin with its port made explicit, as the
  // registration route writes a dev target's base.
  function originKey(u) { return u.protocol + "//" + u.hostname.toLowerCase() + ":" + defaultPort(u); }
  // localLink reports a link a click opens in a web card: http or https on
  // localhost, 127.0.0.1, or [::1] exactly, and not this page's own port
  // (the dashboard), which keeps the ordinary link behaviour.
  function localLink(href) {
    var u = parse(str(href));
    if (!u || (u.protocol !== "http:" && u.protocol !== "https:") || u.username || u.password || !loopbackHost(u.hostname)) return false;
    var here = location.port || (location.protocol === "https:" ? "443" : "80");
    return defaultPort(u) !== here;
  }
  // target reads what the "+ Web" input or a card's address bar holds: an
  // http(s) URL, or a bare loopback host and port, is a dev server's page
  // ({url}); anything else is a workspace path ({path}).
  function target(text) {
    var s = str(text).trim();
    if (!s) return null;
    if (/^https?:\/\//i.test(s)) return { url: s };
    if (/^(localhost|127\.0\.0\.1|\[::1\]):[0-9]+(?:[\/?#]|$)/i.test(s)) return { url: "http://" + s };
    return { path: s };
  }
  function sameTarget(a, b) { return a.url ? a.url === b.url : !!a.path && a.path === b.path; }

  // pageURL is the address a pick or a console block names for the page at
  // p (its path, search, and hash on the proxy origin): a dev server's page
  // is its own URL; a static page is its workspace path, the registered
  // directory (relative to the session root) and the decoded file path. A
  // decoded name holding a control character stays encoded.
  function pageURL(w, p) {
    if (!w.reg) return "";
    if (w.reg.kind === "dev") return cut(oneLine(w.reg.base + p), URL_BYTES);
    var m = /^([^?#]*)(.*)$/.exec(p), name = m[1].replace(/^\//, "");
    try {
      var d = decodeURIComponent(name);
      if (!/[\x00-\x1f\x7f]/.test(d)) name = d;
    } catch (e) { /* kept encoded */ }
    return cut(oneLine((w.reg.base ? w.reg.base + "/" : "") + name + m[2]) || ".", URL_BYTES);
  }
  // record is the card's record for the page at p: a dev page's URL; a
  // static page's workspace path when it is a file at the top of the
  // registered directory (a deeper one would register a narrower directory
  // on restore, on another proxy origin), else null (the record stays).
  function record(w, p) {
    if (w.reg.kind === "dev") return { url: pageURL(w, p) };
    var name = /^\/([^?#\/]+)(?:[?#]|$)/.exec(p);
    if (!name) return null;
    return { path: pageURL(w, "/" + name[1]) };
  }

  // --- the card ---------------------------------------------------------------------
  function head(card) {
    var w = card.w, d = card.data, name = "";
    if (w.reg && w.reg.kind === "dev") { var b = parse(w.reg.base); name = b ? b.host : w.reg.base; }
    else if (d.url) { var u = parse(d.url); name = u ? u.host : d.url; }
    else name = baseName(d.path || "");
    card.title({ icon: "globe", name: name, sub: w.cur && w.reg ? pageURL(w, w.cur) : d.url || d.path || "", badge: wsT("js.web.kind") });
  }
  // paint draws the card's own state: the address bar (unless it is being
  // edited), the pick toggle and attach button (usable once the current
  // page's script answered), the error badge, and the note.
  function paint(card) {
    var w = card.w, here = w.cur && w.reg ? pageURL(w, w.cur) : card.data.url || card.data.path || "";
    if (doc.activeElement !== w.bar) w.bar.value = here;
    w.frame.title = here;
    if (w.pick) {
      var off = wsT("js.web.unavailable");
      w.pick.disabled = w.att.disabled = !w.ready;
      w.pick.title = w.ready ? wsT("js.web.pick") : off;
      w.pick.setAttribute("aria-label", w.pick.title);
      w.pick.setAttribute("aria-pressed", String(!!w.picking));
      w.att.title = w.ready ? wsT("js.web.attach") : off;
      w.att.setAttribute("aria-label", w.att.title);
      w.badge.hidden = !w.errors;
      w.badge.textContent = w.errors > 99 ? "99+" : String(w.errors);
    }
    w.tab.disabled = !tabHref(card);
    head(card);
  }
  function note(card, text) {
    card.w.note.textContent = text || "";
    card.w.note.hidden = !text;
  }
  function send(card, m) {
    var win = card.w.frame.contentWindow;
    if (win) win.postMessage(m, location.origin);
  }
  // register asks the registration route for the card's target (or t, a
  // new one, which becomes the record once it is taken) and loads the
  // page it answers; again (a reload or a retry) keeps the page the card
  // shows while the registration is the same. A refusal shows in the card
  // instead of the page.
  function register(card, t, again) {
    var w = card.w, turn = ++w.turn, want = t || card.data;
    if (!hooks.post) return;
    note(card, "");
    hooks.post("web", want.url ? { url: want.url } : { path: want.path }, function (ok, v) {
      if (w.dead || w.turn !== turn) return;
      if (!ok || !v || typeof v.id !== "string" || !v.id || (v.kind !== "dev" && v.kind !== "static") || typeof v.path !== "string" || v.path.charAt(0) !== "/") {
        fail(card, ok ? wsT("js.web.lost") : wsT("js.web.failed", str(v)));
        return;
      }
      var p = again && w.reg && w.reg.id === v.id && w.cur ? w.cur : v.path;
      w.reg = { id: v.id, kind: v.kind, base: str(v.base) };
      if (t) {
        card.data = want.url ? { url: want.url } : { path: want.path };
        card.save();
      }
      load(card, p);
    });
  }
  // fail shows text in place of the page.
  function fail(card, text) {
    var w = card.w;
    w.want = false;
    w.ready = w.picking = w.armed = w.asked = false;
    w.frame.hidden = true;
    w.frame.src = "about:blank";
    note(card, text);
    paint(card);
  }
  // load points the card's frame at the wrapper for the page at p.
  function load(card, p) {
    var w = card.w;
    w.cur = p;
    w.ready = w.picking = w.armed = w.asked = w.framed = false;
    w.errors = 0;
    w.proxy = "";
    w.want = true;
    w.loads++;
    w.frame.hidden = false;
    paint(card);
    w.frame.src = "/s/" + encodeURIComponent(rid()) + "/web/" + encodeURIComponent(w.reg.id) + "?p=" + encodeURIComponent(p);
  }
  // retry registers again, once, when the wrapper says its registration is
  // gone or did not come up at all (a daemon restart); a second failure in
  // a row shows in the card. A wrapper that comes up resets it.
  function retry(card) {
    var w = card.w;
    if (w.retried) {
      note(card, wsT("js.web.lost"));
      return;
    }
    w.retried = true;
    register(card, null, true);
  }
  // loaded checks, SETTLE after the frame's load, that the wrapper came up.
  // A load since then owns the frame flag, so this check stands down: it
  // would take that load's flag and leave its check none, which registers
  // again and reloads the page.
  function loaded(card) {
    var w = card.w, n = w.loads;
    if (!w.want) return;
    setTimeout(function () {
      if (w.dead || w.loads !== n) return;
      var framed = w.framed;
      w.framed = false;
      if (!framed && w.want) retry(card);
    }, SETTLE);
  }
  function reload(card) { register(card, null, true); }
  function tabHref(card) {
    var w = card.w;
    if (w.reg && w.reg.kind === "static") return w.proxy && w.cur ? w.proxy + w.cur : "";
    return card.data.url || "";
  }
  function openTab(card) {
    var href = tabHref(card);
    if (href) window.open(href, "_blank", "noopener");
  }
  function togglePick(card) {
    var w = card.w;
    if (!w.ready) return;
    w.armed = !w.picking;
    send(card, { wsWeb: "pick", on: w.armed });
  }
  function attach(card) {
    var w = card.w;
    if (!w.ready) return;
    w.asked = true;
    send(card, { wsWeb: "attach" });
  }
  // go opens what the address bar holds: a page on the dev server the card
  // shows only moves the wrapper; anything else registers again.
  function go(card) {
    var w = card.w, t = target(w.bar.value);
    if (!t) { paint(card); return; }
    if (t.url && w.reg && w.reg.kind === "dev") {
      var u = parse(t.url), b = parse(w.reg.base);
      if (u && b && originKey(u) === originKey(b)) {
        card.data = { url: t.url };
        card.save();
        load(card, u.pathname + u.search + u.hash);
        return;
      }
    }
    register(card, t);
  }

  // take handles a message from the card's own frame.
  function take(card, m) {
    var w = card.w, type = m.wsWeb;
    if (type === "frame") {
      if (!/^https?:\/\/[^\/?#\s]+$/.test(str(m.proxy))) return;
      w.proxy = m.proxy;
      w.framed = true;
      w.retried = false;
      note(card, "");
      paint(card);
    } else if (type === "gone") {
      retry(card);
    } else if (type === "ready") {
      var p = str(m.path);
      if (p.charAt(0) !== "/") return;
      w.cur = p;
      w.ready = true;
      w.picking = w.armed = w.asked = false;
      w.errors = 0;
      var r = w.reg && record(w, p);
      if (r && !sameTarget(r, card.data)) {
        card.data = r;
        card.save();
      }
      paint(card);
    } else if (type === "picking") {
      w.picking = m.on === true;
      if (!w.picking) w.armed = false;
      paint(card);
    } else if (type === "pick") {
      var sel = str(m.selector), path = str(m.path);
      if (!w.armed || !sel.trim() || bytes(sel) > SELECTOR_BYTES || /[\r\n]/.test(sel) || path.charAt(0) !== "/") return;
      w.armed = false;
      if (hooks.addPick) hooks.addPick({ url: pageURL(w, path), selector: sel.trim(), text: cut(oneLine(str(m.text)).trim(), TEXT_BYTES) });
    } else if (type === "errors") {
      var n = m.count;
      if (typeof n !== "number" || !(n >= 0) || n !== Math.floor(n)) return;
      w.errors = n;
      paint(card);
    } else if (type === "console") {
      if (!w.asked || str(m.path).charAt(0) !== "/") return;
      w.asked = false;
      var lines = (Array.isArray(m.lines) ? m.lines : []).filter(function (l) { return typeof l === "string"; }).map(function (l) {
        return cut(oneLine(l), LINE_BYTES);
      }).filter(function (l) { return l.trim(); }).slice(-LINES);
      w.errors = 0;
      paint(card);
      if (!lines.length) toast(wsT("js.web.noerrors"));
      else if (hooks.addConsole) hooks.addConsole({ url: pageURL(w, m.path), lines: lines });
    }
  }
  window.addEventListener("message", function (e) {
    var m = e.data;
    if (e.origin !== location.origin || !e.source || !m || typeof m !== "object" || typeof m.wsWeb !== "string") return;
    var card = cards.list(KIND).filter(function (c) { return c.w && !c.w.dead && c.w.frame.contentWindow === e.source; })[0];
    if (card) take(card, m);
  });

  function build(card) {
    var w = card.w = { turn: 0, loads: 0, reg: null, cur: "", proxy: "", errors: 0 };
    card.body.textContent = "";
    var bar = mk("div", "wc-bar");
    w.bar = mk("input", "fpin wc-url");
    w.bar.type = "text";
    w.bar.spellcheck = false;
    w.bar.setAttribute("autocomplete", "off");
    w.bar.setAttribute("aria-label", wsT("js.web.bar"));
    bar.appendChild(w.bar);
    w.note = mk("div", "wc-note");
    w.note.setAttribute("role", "alert");
    w.note.hidden = true;
    w.frame = mk("iframe", "wc-frame");
    [bar, w.note, w.frame].forEach(function (n) { card.body.appendChild(n); });
    w.bar.addEventListener("keydown", function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter") { e.preventDefault(); go(card); }
      else if (e.key === "Escape") { e.preventDefault(); w.bar.blur(); paint(card); }
    });
    w.bar.addEventListener("blur", function () { paint(card); });
    w.frame.addEventListener("load", function () { loaded(card); });
    // Pick and attach go to the composer: a page without one gets neither.
    var acts = [];
    if (hooks.addPick) {
      acts.push({ icon: "square-mouse-pointer", title: wsT("js.web.pick"), cls: "wc-pick", run: togglePick });
      acts.push({ icon: "bug", title: wsT("js.web.attach"), cls: "wc-att", run: attach });
    }
    acts.push({ icon: "refresh-cw", title: wsT("js.web.reload"), run: reload });
    acts.push({ icon: "external-link", title: wsT("js.web.tab"), cls: "wc-tab", run: openTab });
    var btns = card.actions(acts) || [];
    if (hooks.addPick) {
      w.pick = btns[0];
      w.att = btns[1];
      w.badge = mk("span", "wc-badge");
      w.badge.setAttribute("aria-hidden", "true");
      w.badge.hidden = true;
      w.att.appendChild(w.badge);
      // Escape inside the page ends pick mode there; focus left on the
      // toggle (it was just clicked) ends it here, before a maximized
      // card's Escape takes the key.
      w.pick.addEventListener("keydown", function (e) {
        if (e.key !== "Escape" || !(w.picking || w.armed)) return;
        e.preventDefault();
        w.armed = false;
        send(card, { wsWeb: "pick", on: false });
      });
    }
    w.tab = btns[btns.length - 1];
    paint(card);
    register(card);
  }

  cards.register(KIND, {
    base: BASE,
    open: build,
    close: function (card) {
      if (!card.w) return;
      card.w.dead = true;
      card.w.turn++;
    }
  });

  // --- the "+ Web" button and its popover --------------------------------------------
  // The button stands in the composer's status-line slot beside "+ Terminal"
  // (terminal.js), or in the slot fixed at the page's corner where the page
  // has none. Its popover takes a URL or a workspace path.
  function closePop(refocus) {
    if (!ui.pop || ui.pop.hidden) return;
    ui.pop.hidden = true;
    ui.btn.setAttribute("aria-expanded", "false");
    if (refocus) ui.btn.focus();
  }
  function placePop() {
    var pop = ui.pop;
    pop.style.right = "";
    var out = 8 - pop.getBoundingClientRect().left;
    if (out > 0) pop.style.right = (parseFloat(window.getComputedStyle(pop).right) || 0) - out + "px";
  }
  function openPop() {
    ui.in.value = "";
    ui.pop.hidden = false;
    placePop();
    ui.btn.setAttribute("aria-expanded", "true");
    ui.in.focus();
  }
  function button() {
    var slot = doc.getElementById("tslot") || doc.querySelector(".tslot.tfix");
    if (!slot) {
      slot = mk("div", "tslot tfix");
      doc.body.appendChild(slot);
    }
    ui.btn = mk("button", "btn tnew wnew");
    ui.btn.type = "button";
    ui.btn.innerHTML = cards.icon("globe");
    ui.btn.title = wsT("js.web.new.title");
    ui.btn.setAttribute("aria-label", wsT("js.web.new"));
    ui.btn.setAttribute("aria-haspopup", "dialog");
    ui.btn.setAttribute("aria-expanded", "false");
    slot.appendChild(ui.btn);
    var pop = ui.pop = mk("div", "tpop wpop");
    pop.setAttribute("role", "dialog");
    pop.setAttribute("aria-label", wsT("js.web.pop.label"));
    pop.hidden = true;
    pop.appendChild(mk("div", "lb", wsT("js.web.pop.label")));
    var row = mk("div", "tprow");
    ui.in = mk("input", "fpin");
    ui.in.type = "text";
    ui.in.spellcheck = false;
    ui.in.setAttribute("autocomplete", "off");
    ui.in.placeholder = wsT("js.web.pop.placeholder");
    ui.in.setAttribute("aria-label", wsT("js.web.pop.placeholder"));
    ui.go = mk("button", "btn pr", wsT("js.web.pop.open"));
    ui.go.type = "button";
    row.appendChild(ui.in);
    row.appendChild(ui.go);
    pop.appendChild(row);
    slot.appendChild(pop);

    ui.btn.addEventListener("click", function () { if (pop.hidden) openPop(); else closePop(true); });
    ui.go.addEventListener("click", function () {
      var t = target(ui.in.value);
      if (!t) { ui.in.focus(); return; }
      closePop(false);
      open(t);
    });
    ui.in.addEventListener("keydown", function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter") { e.preventDefault(); ui.go.click(); }
    });
    pop.addEventListener("keydown", function (e) {
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); closePop(true); }
    });
    doc.addEventListener("mousedown", function (e) {
      if (!pop.hidden && !pop.contains(e.target) && !ui.btn.contains(e.target)) closePop(false);
    }, true);
  }

  // open shows t ({url} or {path}) in a web card: the card already on that
  // target registers and loads again, else a new card opens.
  function open(t) {
    if (!t || !t.url === !t.path) return null;
    var data = t.url ? { url: String(t.url) } : { path: String(t.path) };
    var card = cards.list(KIND).filter(function (c) { return sameTarget(data, c.data); })[0];
    if (card) reload(card);
    else card = cards.open(KIND, data);
    if (!card) return null;
    card.fold(false);
    card.show();
    return card;
  }

  window.WsWeb = {
    // setup hands the kind the page's own pieces: post (a session write
    // route, which passes a refusal's text through), toast, rid, and the
    // composer's addPick({url, selector, text}) and addConsole({url,
    // lines}); a page without a composer hands neither, and its cards
    // offer no pick or attach. The page calls it before it mounts the
    // card framework, which restores the stored cards.
    setup: function (h) { hooks = h || {}; },
    // mount adds the "+ Web" button. The page mounts the card framework
    // (and the terminal kind, whose button stands first) before it.
    mount: function (col) {
      if (ui.btn || !col) return;
      button();
    },
    open: open,
    localLink: localLink,
    target: target
  };
})();
