(function () {
  // The file preview card (window.WsPreview), a kind on the board card
  // framework (cards.js). A card shows one file under the session's browsing
  // root, as the preview route (fs_routes.go) describes it: an image or a
  // video from the raw route, an HTML or SVG file in a sandboxed frame from
  // the capability route, rendered markdown, a .diff or .patch through the
  // commit window's diff renderer, other text as plain text, or a "not
  // viewable" note.
  //
  // There is one preview slot: opening a file replaces the content of the
  // one unpinned preview card, which counts as an open (the card moves to
  // the head of the unpinned flow). Pinning the slot makes it a pinned
  // card, and the next file opens a new slot. Opening a file that already
  // has a pinned card shows that card and loads it again.
  //
  // A card's record is the file's path (and the root, when the explorer
  // changed it), never the capability URL: every load asks the preview
  // route again, so a restored card gets a fresh capability.
  var doc = document, root = doc.documentElement, cards = window.WsCards;
  if (!cards) return;
  var rid = root.getAttribute("data-rid"), KIND = "preview", hooks = {};
  // LINE is the text view's line height in px (the stylesheet's .pv-txt), which
  // a jump to a line multiplies.
  var LINE = 18;

  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function baseName(p) { var s = p.replace(/[\\/]+$/, ""); return s.slice(Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\")) + 1) || p; }
  function ext(p) { var b = baseName(p), i = b.lastIndexOf("."); return i > 0 ? b.slice(i + 1).toLowerCase() : ""; }
  function local() { return root.hasAttribute("data-sysopen"); }
  function query(d) {
    return "?path=" + encodeURIComponent(d.path) + (d.root ? "&root=" + encodeURIComponent(d.root) : "") + (d.line ? "&text=1" : "");
  }

  // OPAQUE are the extensions no preview shows: a link to such a file keeps
  // the link menu on a plain click, and the explorer still opens the card,
  // in its "not viewable" state. Every other file is asked of the server.
  var OPAQUE = {};
  ("zip gz tgz bz2 xz zst 7z rar tar jar war pdf doc docx xls xlsx ppt pptx odt ods odp exe dll so dylib a o obj bin dat db sqlite class pyc wasm iso dmg img deb rpm msi apk " +
    "ttf otf woff woff2 eot mp3 wav flac aac m4a mov avi mkv wmv psd heic tif tiff").split(" ").forEach(function (x) { OPAQUE[x] = true; });
  function takes(path) { return !OPAQUE[ext(path)]; }

  var ICON = { image: "image", video: "film", frame: "file-code", markdown: "file-text", diff: "file-diff", text: "file-text", none: "file" };
  function badge(v) {
    if (v.kind === "image") return wsT("js.preview.kind.image");
    if (v.kind === "video") return wsT("js.preview.kind.video");
    if (v.kind === "frame") return wsT("js.preview.kind.frame");
    if (v.kind === "markdown") return wsT("js.preview.kind.markdown");
    if (v.kind === "diff") return wsT("js.preview.kind.diff");
    if (v.kind === "text") return v.source ? wsT("js.preview.kind.source") : wsT("js.preview.kind.text");
    return "";
  }
  // guess is the header icon before the server has answered.
  function guess(path) {
    var x = ext(path);
    if (/^(png|jpe?g|gif|webp|avif|bmp|ico)$/.test(x)) return "image";
    if (/^(mp4|webm|ogg|ogv)$/.test(x)) return "film";
    if (/^(html?|svg)$/.test(x)) return "file-code";
    if (/^(diff|patch)$/.test(x)) return "file-diff";
    return "file-text";
  }

  function openExternal(card) {
    if (!hooks.post) return;
    var d = card.data;
    hooks.post("fs/act", { path: d.path, action: "open", root: d.root || "" }, function (ok, v) {
      if (hooks.toast) hooks.toast(ok ? wsT("js.toast.openednative") : wsT("js.toast.openfail", v));
    });
  }
  // The header's own action, open in the default app, runs a host command:
  // only a browser on the daemon's machine is offered it, and the daemon
  // checks again.
  function actions() { return local() ? [{ icon: "external-link", title: wsT("js.preview.openext"), run: openExternal }] : []; }
  function head(card, icon, text) {
    var d = card.data;
    card.title({ icon: icon, name: baseName(d.path), sub: d.path, badge: text });
    card.actions(actions());
  }
  function fill(card, node) {
    card.body.textContent = "";
    card.body.appendChild(node);
    card.body.scrollTop = 0;
  }
  function note(card, text, offerOpen) {
    var box = mk("div", "pv-none");
    box.appendChild(mk("p", "", text));
    if (offerOpen && local()) {
      var b = mk("button", "btn", wsT("js.preview.openext"));
      b.type = "button";
      b.addEventListener("click", function () { openExternal(card); });
      box.appendChild(b);
    }
    fill(card, box);
  }

  // jump scrolls a text view to line and marks it.
  function jump(card, box, line) {
    var mark = mk("div", "pv-mark");
    mark.style.top = (line - 1) * LINE + "px";
    box.appendChild(mark);
    card.body.scrollTop = Math.max(0, (line - 1) * LINE - Math.round(card.body.clientHeight / 3));
  }
  function draw(card, v) {
    var d = card.data, box;
    head(card, ICON[v.kind] || "file", badge(v));
    if (v.kind === "image") {
      box = mk("div", "pv-img");
      var img = mk("img");
      img.alt = baseName(d.path);
      img.src = v.src;
      box.appendChild(img);
    } else if (v.kind === "video") {
      box = mk("div", "pv-vid");
      var vid = mk("video");
      vid.controls = true;
      vid.preload = "metadata";
      vid.src = v.src;
      box.appendChild(vid);
    } else if (v.kind === "frame") {
      // allow-scripts without allow-same-origin: the document runs in an
      // opaque origin, apart from the dashboard page, its cookie, and its
      // routes. The capability route's own policy says the same.
      box = mk("div", "pv-sand");
      box.appendChild(mk("div", "pv-cap", wsT("js.preview.sandbox")));
      var fr = mk("iframe");
      fr.setAttribute("sandbox", "allow-scripts");
      fr.setAttribute("referrerpolicy", "no-referrer");
      fr.title = d.path;
      fr.src = v.frame;
      box.appendChild(fr);
    } else if (v.kind === "markdown") {
      box = mk("article", "md pv-md");
      box.innerHTML = v.html;
    } else if (v.kind === "diff") {
      box = mk("div", "pv-diff");
      (v.diff || []).forEach(function (f) {
        var sec = mk("section", "pv-dfile");
        if (f.path) sec.appendChild(mk("div", "pv-dpath", f.path));
        if (hooks.renderDiff) hooks.renderDiff(sec, f);
        box.appendChild(sec);
      });
    } else if (v.kind === "text") {
      box = mk("div", "pv-txtbox");
      if (v.source) box.appendChild(mk("div", "pv-cap", wsT("js.preview.source.note")));
      var wrap = mk("div", "pv-lines");
      wrap.appendChild(mk("pre", "pv-txt", v.text));
      box.appendChild(wrap);
      fill(card, box);
      if (d.line) jump(card, wrap, d.line);
      return;
    } else {
      note(card, wsT("js.preview.none", v.reason || ""), true);
      return;
    }
    fill(card, box);
  }
  // load asks the preview route for the card's file and draws the answer.
  // The card keeps what it shows until the answer is here; a later load
  // cancels an earlier one's answer.
  function load(card) {
    var d = card.data, turn = card.turn = (card.turn || 0) + 1;
    head(card, guess(d.path), "");
    fetch("/s/" + rid + "/fs/preview" + query(d), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw r.status;
      return r.json();
    }).then(function (v) {
      if (card.turn === turn) draw(card, v);
    }, function (status) {
      if (card.turn !== turn) return;
      head(card, "file", "");
      note(card, status === 404 ? wsT("js.preview.missing") : wsT("js.preview.failed"), false);
    });
  }
  function slots() { return cards.list(KIND).filter(function (c) { return !c.pinned; }); }

  cards.register(KIND, {
    open: load,
    close: function (card) { card.turn = -1; },
    // Unpinning a card makes it the slot again: the slot it replaces goes.
    pinned: function (card) {
      if (card.pinned) return;
      slots().forEach(function (c) { if (c !== card) c.close(); });
    }
  });

  window.WsPreview = {
    // setup hands the kind the page's own pieces: renderDiff (the commit
    // window's file diff renderer), post (a session write route), toast.
    setup: function (h) { hooks = h || {}; },
    takes: takes,
    // open shows the file at path (relative to the browsing root) in a
    // preview card. o.root is a root the explorer changed to; o.line a line
    // to show, which also asks for the file as plain text.
    open: function (path, o) {
      o = o || {};
      var data = { path: path };
      if (o.root) data.root = o.root;
      if (o.line > 0) data.line = o.line;
      var card = cards.list(KIND).filter(function (c) { return c.pinned && c.data.path === path && (c.data.root || "") === (o.root || ""); })[0];
      if (card) {
        card.data = data;
        card.save();
        load(card);
      } else if ((card = slots()[0])) {
        card.data = data;
        card.bump();
        load(card);
      } else card = cards.open(KIND, data);
      if (!card) return null;
      card.fold(false);
      card.show();
      return card;
    }
  };
})();
