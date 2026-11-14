(function () {
  // window.WsShell: the iframe shell's pure parts, loaded by the shell page
  // and by every page it frames (dashboard.js drives them).
  //
  // The shell (the "shell" template, served at / and /s/<reply-id>) keeps
  // the session rail and the top header across navigations and loads each
  // dashboard page it frames (the index and the session pages) in a
  // same-origin frame of its own: the page's path with ?frame=1, which the
  // daemon lets only its own origin frame.
  // The LIMIT most recently shown frames stay loaded, hidden but for the
  // shown one, so going back to one shows it at once; the address bar keeps
  // the page's own path (history.pushState). Each frame streams its own
  // regions from the daemon; the shell streams the rail alone.
  //
  // Messages between the shell and its frames are UI control only, never
  // data. Each is an object whose wsShell field names its kind; a receiver
  // takes one only from its own origin, the shell only from one of its
  // frames and a frame only from its parent (message, below, checks the
  // shape):
  //
  //   frame to shell
  //     head    {html, qs, pad, top}  the header the shell shows for the
  //             frame: html its head region's markup (null when the page
  //             keeps its own header), qs its search slot's markup, pad its
  //             header's right padding and top its header's height (px)
  //     title   {title}               the frame's tab title
  //     nav     {href}                a page the shell frames was picked in
  //                                   the frame (a link, the palette, a
  //                                   spawn): the shell shows it
  //     key     {key}                 a key the shell acts on, pressed in
  //                                   the frame: "rail" (Ctrl+`), "esc"
  //     layout  {pincol, dim, popover?} the frame shows the pinned column
  //                                   (the rail then overlays below 2200px),
  //                                   and the alpha of the backdrop it shows
  //                                   over the whole window (0: none), which
  //                                   the shell's rail and header take too;
  //                                   popover lifts the frame for its dropdown
  //     scroll  {y}                   the frame's page scrolled as a whole:
  //                                   the shell's header and rail move along
  //     hash    {hash}                the frame's hash route changed: the
  //                                   address bar takes it
  //     theme   {theme}               the frame's theme was picked
  //   shell to frame
  //     shown   {shown}               the frame was shown or hidden: a
  //                                   hidden one holds its region updates,
  //                                   the clock, and tracker reads
  //     search  {}                    open the search palette
  //     menu    {}                    open the session menu (the head's
  //                                   harness chip, which the shell mirrors)
  //     sync    {}                    send the header, layout, and title
  //                                   again (the shell's script started)
  //     theme   {theme}               another page picked the theme
  var LIMIT = 5;
  var KINDS = {
    head: { html: "?string", qs: "string", pad: "number", top: "number" },
    title: { title: "string" },
    nav: { href: "string" },
    key: { key: "string" },
    layout: { pincol: "boolean", dim: "number" },
    scroll: { y: "number" },
    hash: { hash: "string" },
    theme: { theme: "string" },
    shown: { shown: "boolean" },
    search: {},
    menu: {},
    sync: {}
  };

  // message returns m when it is a shell message of a known kind whose
  // fields have their types (a "?" type also takes null), else null.
  function message(m) {
    if (!m || typeof m !== "object" || typeof m.wsShell !== "string" || !Object.prototype.hasOwnProperty.call(KINDS, m.wsShell)) return null;
    var want = KINDS[m.wsShell];
    for (var k in want) {
      var t = want[k], v = m[k];
      if (t.charAt(0) === "?") { if (v === null) continue; t = t.slice(1); }
      if (typeof v !== t) return null;
    }
    return m;
  }

  // SESSION is a session page's path; its reply-id is the daemon's
  // (registry.go ValidReplyID).
  var SESSION = /^\/s\/([a-z0-9]{1,64})$/;

  // framed reads a URL (u, a URL) the shell may frame: {path, hash, rid}
  // for a dashboard page the shell frames on origin (the index, a session
  // page), null for any other (another origin, another route, a query the
  // shell does not pass on). rid is the session's reply-id, empty on the
  // index.
  function framed(u, origin) {
    if (!u || u.origin !== origin) return null;
    var search = u.search || "";
    if (search && search !== "?" && search !== "?frame=1") return null;
    if (u.pathname === "/") return { path: "/", hash: u.hash || "", rid: "" };
    var m = SESSION.exec(u.pathname);
    if (m) return { path: u.pathname, hash: u.hash || "", rid: m[1] };
    return null;
  }

  // src is the frame's path for a framed page: its path, ?frame=1, and its
  // hash.
  function src(f) { return f.path + "?frame=1" + (f.hash || ""); }

  // touch puts key first in order (the frames, most recently shown first)
  // and returns {order, evict}: the new order, kept to limit (LIMIT when
  // not given), and the keys it dropped, oldest last.
  function touch(order, key, limit) {
    var n = limit || LIMIT, next = [key].concat(order.filter(function (k) { return k !== key; }));
    return { order: next.slice(0, n), evict: next.slice(n) };
  }

  // alpha reads a computed CSS colour's alpha: 1 for rgb(), the fourth
  // component for rgba() or rgb(... / a), 0 for anything else (transparent,
  // none).
  function alpha(c) {
    var m = /^rgba?\(([^)]*)\)$/.exec(String(c || "").trim());
    if (!m) return 0;
    var parts = m[1].split(/[\s,/]+/).filter(Boolean);
    if (parts.length < 4) return 1;
    var a = parts[3], v = a.charAt(a.length - 1) === "%" ? parseFloat(a) / 100 : parseFloat(a);
    return isFinite(v) ? Math.max(0, Math.min(1, v)) : 0;
  }

  window.WsShell = { LIMIT: LIMIT, message: message, framed: framed, src: src, touch: touch, alpha: alpha };
})();
