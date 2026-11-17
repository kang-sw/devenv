(function () {
  // The file preview card (window.WsPreview), a kind on the board card
  // framework (cards.js). A card shows one file under the session's browsing
  // root, as the preview route (fs_routes.go) describes it: an image or a
  // video/audio from the raw route, an HTML or SVG file in a sandboxed frame from
  // the capability route, rendered markdown, a .diff or .patch through the
  // commit window's diff renderer, other text as plain text, or a "not
  // viewable" note.
  //
  // Files have three states: one replaceable preview, durable open cards,
  // and pinned cards. Preview is browser-memory-only: the unchanged card
  // record restores as open (or pinned). Opening an already open/pinned
  // file reuses it without demoting it.
  //
  // A card's record is the file's path (and the root, when the explorer
  // changed it), never the capability URL: every load asks the preview
  // route again, so a restored card gets a fresh capability.
  //
  // An outside card (its record has out) shows a file outside the session
  // root that the board or a lead reply links, by its absolute path. Every
  // load posts the outside-link route (fs_outside.go), which checks the link
  // again and mints a fresh capability; when it refuses (the link is gone,
  // or this browser is not on the daemon's machine) the card says the file
  // is no longer available.
  var doc = document, root = doc.documentElement, cards = window.WsCards;
  if (!cards) return;
  var rid = root.hasAttribute("data-worktree") ? "host0" : root.getAttribute("data-rid"), KIND = "preview", hooks = {};
  // Explorer preflight answers are one-shot, never part of a saved card record.
  var answers = new WeakMap();
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
  // place splits an absolute path into its folder and its file name: the
  // host route takes a root and a path under it.
  function place(p) {
    var i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
    return { root: p.slice(0, i + 1), path: p.slice(i + 1) };
  }
  function query(d) {
    return "?path=" + encodeURIComponent(d.path) + (d.root ? "&root=" + encodeURIComponent(d.root) : "") + (d.line ? "&text=1" : "");
  }

  // OPAQUE keeps link handling unchanged, including outside-root PDFs. An
  // explorer explicitly asks the guarded preview route, whose pdf answer
  // alone opts a card into the native viewer.
  var OPAQUE = {};
  ("zip gz tgz bz2 xz zst 7z rar tar jar war pdf doc docx xls xlsx ppt pptx odt ods odp exe dll so dylib a o obj bin dat db sqlite class pyc wasm iso dmg img deb rpm msi apk " +
    "ttf otf woff woff2 eot mp3 wav flac aac m4a mov avi mkv wmv psd heic tif tiff").split(" ").forEach(function (x) { OPAQUE[x] = true; });
  function takes(path) { return !OPAQUE[ext(path)]; }

  var ICON = { image: "image", video: "film", audio: "file", pdf: "file-text", frame: "file-code", markdown: "file-text", diff: "file-diff", text: "file-text", none: "file" };
  function badge(v) {
    if (v.kind === "image") return wsT("js.preview.kind.image");
    if (v.kind === "video") return wsT("js.preview.kind.video");
    if (v.kind === "audio") return wsT("js.preview.kind.audio");
    if (v.kind === "pdf") return wsT("js.preview.kind.pdf");
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
    if (/^(mp4|webm|mov|ogv|m4v)$/.test(x)) return "film";
    if (/^(mp3|wav|ogg|oga|flac|m4a|aac|opus)$/.test(x)) return "file";
    if (/^(html?|svg)$/.test(x)) return "file-code";
    if (/^(diff|patch)$/.test(x)) return "file-diff";
    return "file-text";
  }

  function openExternal(card) {
    if (!hooks.post) return;
    var d = card.data, at = d.out ? place(d.path) : { root: d.root || "", path: d.path };
    hooks.post("fs/act", { path: at.path, action: "open", root: at.root }, function (ok, v) {
      if (hooks.toast) hooks.toast(ok ? wsT("js.toast.openednative") : wsT("js.toast.openfail", v));
    });
  }
  // The header's own action, open in the default app, runs a host command:
  // only a browser on the daemon's machine is offered it, and the daemon
  // checks again.
  // The reload action loads the card again, whatever it shows: the route is
  // asked afresh (a new capability for a frame, so it works for a sandboxed
  // frame, whose window is opaque to the page), and the content is redrawn.
  // An HTML file also opens in a web card (webcard.js, which a page offers
  // a browser on the daemon's machine only), where its scripts run on a
  // proxy origin of its own; the registration route decides whether the
  // file is under the session root.
  function actions(card) {
    var a = local() ? [{ icon: "external-link", title: wsT("js.preview.openext"), run: openExternal }] : [];
    if (window.WsWeb && /^html?$/.test(ext(card.data.path))) a.push({ icon: "globe", title: wsT("js.web.menu.open"), run: openWeb });
    a.push({ icon: "refresh-cw", title: wsT("js.preview.reload"), run: load });
    return a;
  }
  // openWeb opens the card's file in a web card, by the path the
  // registration route takes: relative to the session root, or absolute.
  function openWeb(card) {
    var d = card.data, p = d.path;
    if (!d.out && d.root) p = d.root.replace(/[\\/]+$/, "") + "/" + d.path;
    window.WsWeb.open({ path: p });
  }
  function head(card, icon, text) {
    var d = card.data;
    card.title({ icon: icon, name: baseName(d.path), sub: d.path, badge: text });
    card.actions(actions(card));
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
  // Token text is never HTML. Reject a lexer-normalized/mismatched answer
  // rather than changing the source (and content-search line offsets).
  function codeText(v) {
    var pre = mk("pre", "pv-txt"), lines = v.highlighted_lines, text = v.text || "";
    if (!Array.isArray(lines) || lines.map(function (line) { return (line || []).map(function (p) { return p.s; }).join(""); }).join("\n") !== text) {
      pre.textContent = text;
      return pre;
    }
    pre.className += " chroma";
    lines.forEach(function (line, i) {
      if (i) pre.appendChild(mk("span", "", "\n"));
      (line || []).forEach(function (p) { pre.appendChild(mk("span", p.c ? "t-" + p.c : "", p.s)); });
    });
    return pre;
  }
  function pdfView(card, v) {
    var box = mk("div", "pv-pdf"), link = mk("a", "btn pv-pdf-open", wsT("js.preview.pdf.open"));
    link.href = v.src;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    // Keep an unobtrusive escape hatch inside the preview even when the
    // browser advertises a viewer but a policy/plugin failure leaves it blank.
    box.appendChild(link);
    if (window.navigator && window.navigator.pdfViewerEnabled === true) {
      var frame = mk("iframe");
      frame.title = card.data.path;
      frame.setAttribute("referrerpolicy", "no-referrer");
      frame.addEventListener("error", function () {
        if (frame.parentNode === box) box.removeChild(frame);
        box.removeAttribute("data-viewer");
      });
      frame.src = v.src;
      box.setAttribute("data-viewer", "");
      box.appendChild(frame);
    }
    return box;
  }
  function draw(card, v) {
    var d = card.data, box;
    // Monospace text views are code, regardless of the HTML/SVG source flag;
    // rendered Markdown is prose. Use renderer kinds, not extensions or MIME.
    card.el.setAttribute("data-preview-kind", v.kind === "text" ? "code" : v.kind);
    card.paint();
    head(card, ICON[v.kind] || "file", badge(v));
    if (v.kind === "image") {
      box = mk("div", "pv-img");
      var img = mk("img");
      img.alt = baseName(d.path);
      img.src = v.src;
      box.appendChild(img);
    } else if (v.kind === "video" || v.kind === "audio") {
      box = mk("div", v.kind === "video" ? "pv-vid" : "pv-audio");
      var media = mk(v.kind), turn = card.turn;
      media.controls = true;
      media.preload = "metadata";
      media.addEventListener("error", function () {
        // Ignore a late event from a replaced player or a pending reload.
        if (card.turn === turn && card.body.firstChild === box) {
          if (card.mediaError && card.mediaError(card)) return;
          draw(card, { kind: "none", reason: wsT("js.preview.media.unsupported") });
        }
      });
      media.src = v.src;
      box.appendChild(media);
    } else if (v.kind === "pdf") {
      box = pdfView(card, v);
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
      wrap.appendChild(codeText(v));
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
  // ask requests what the card shows: the preview route, or for an outside
  // card a POST of its path to the outside-link route, whose answer has the
  // same shape.
  function ask(d) {
    if (!d.out) return fetch("/s/" + rid + "/fs/preview" + query(d), { credentials: "same-origin", cache: "no-store" });
    return fetch("/s/" + rid + "/fs/outside", {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path: d.path })
    });
  }
  // load asks for the card's file and draws the answer. The card keeps what
  // it shows until the answer is here; a later load cancels an earlier
  // one's answer.
  function load(card) {
    var d = card.data, turn = card.turn = (card.turn || 0) + 1;
    card.el.removeAttribute("data-preview-kind");
    card.paint();
    head(card, guess(d.path), "");
    var answer = answers.get(d);
    answers.delete(d);
    var request = answer ? Promise.resolve(answer) : ask(d).then(function (r) {
      if (!r.ok) throw r.status;
      return r.json();
    });
    request.then(function (v) {
      if (card.turn === turn) draw(card, v);
    }, function (status) {
      if (card.turn !== turn) return;
      head(card, "file", "");
      note(card, status === 404 ? wsT(d.out ? "js.preview.gone" : "js.preview.missing") : wsT("js.preview.failed"), false);
    });
  }
  function slots() { return cards.list(KIND).filter(function (c) { return c.preview && !c.pinned; }); }
  function state(card) { return card.pinned ? "pinned" : card.preview ? "preview" : "open"; }
  function changed(card) {
    card.el.setAttribute("data-file-state", state(card));
    var label = card.fileState;
    if (label) {
      label.textContent = wsT("js.preview.state.preview");
      label.hidden = state(card) !== "preview";
    }
    doc.dispatchEvent(new Event("ws-file-cards"));
  }
  function promote(card) {
    if (!card.preview) return;
    card.preview = false;
    changed(card);
    cards.relayout();
  }
  function matches(card, data) {
    return card.data.path === data.path && !card.data.out === !data.out && (card.data.root || "") === (data.root || "");
  }

  cards.register(KIND, {
    open: function (card) {
      // The flag and state label never enter card.data or the stored record.
      if (!card.fileState) {
        card.preview = false;
        card.fileState = mk("span", "pv-state");
        card.name.parentNode.insertBefore(card.fileState, card.name.nextSibling);
        card.el.addEventListener("dblclick", function (e) {
          if (e.target.closest(".bc-hd") && !e.target.closest("button")) promote(card);
        });
      }
      changed(card);
      load(card);
    },
    close: function (card) { card.turn = -1; card.mediaError = null; doc.dispatchEvent(new Event("ws-file-cards")); },
    // Both pinning and unpinning end the transient preview state.
    pinned: function (card) { card.preview = false; changed(card); }
  });

  window.WsPreview = {
    // setup hands the kind the page's own pieces: renderDiff (the commit
    // window's file diff renderer), post (a session write route), toast.
    setup: function (h) { hooks = h || {}; },
    takes: takes,
    // A memory-only surface using the very same loaders and renderers as cards.
    // It never enters WsCards, so existing durable/pinned cards stay untouched.
    createPane: function (el, closed) {
      var header = mk("header", "file-preview-head"), name = mk("strong"), path = mk("span", "file-preview-path");
      var keep = mk("button", "btn", wsT("js.preview.keep")), exit = mk("button", "ibtn");
      keep.type = exit.type = "button";
      exit.innerHTML = cards.icon("x");
      exit.title = wsT("js.card.close");
      exit.setAttribute("aria-label", exit.title);
      header.appendChild(name); header.appendChild(path); header.appendChild(keep); header.appendChild(exit);
      var body = mk("div", "file-preview-body"), current = null;
      el.appendChild(header); el.appendChild(body);
      el.hidden = true;
      var view = {
        el: el, body: body, paint: function () {}, actions: function () {},
        title: function (v) { name.textContent = v.name; path.textContent = v.sub; path.title = v.sub; },
        close: function () { pane.close(); }
      };
      var pane = {
        open: function (p, o) {
          current = { path: p, options: o || {} };
          view.data = { path: p };
          if (o && o.root) view.data.root = o.root;
          if (o && o.line) view.data.line = o.line;
          view.mediaError = o && o.mediaError;
          if (o && o.answer) answers.set(view.data, o.answer);
          body.textContent = "";
          el.hidden = false;
          load(view);
        },
        close: function () {
          if (!current) return null;
          var previous = current;
          current = null;
          // Increment, never reset: an old answer must not match a later open.
          view.turn = (view.turn || 0) + 1;
          view.mediaError = null;
          body.textContent = "";
          el.hidden = true;
          if (closed) closed();
          return previous;
        },
        current: function () { return current; }
      };
      keep.addEventListener("click", function () {
        var previous = pane.close();
        if (previous) window.WsPreview.open(previous.path, Object.assign({}, previous.options, { keep: true }));
      });
      exit.addEventListener("click", function () { pane.close(); });
      el.addEventListener("keydown", function (e) {
        if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); pane.close(); }
      });
      return pane;
    },
    state: function (path, o) {
      var data = { path: path, root: o && o.root, out: o && o.out };
      var card = cards.list(KIND).filter(function (c) { return matches(c, data); })[0];
      return card ? state(card) : "";
    },
    // open shows the file at path (relative to the browsing root) in a
    // preview card. o.root is a root the explorer changed to; o.line a line
    // to show, which also asks for the file as plain text; o.out marks path
    // as the absolute path of a linked file outside the session root.
    // o.origin is the exact validated topic node for one-time placement;
    // it is never part of the kind's persisted data or sandbox capability.
    // o.answer reuses explorer preflight; o.mediaError offers its row menu.
    // o.keep opens for real (double click/Enter); default callers preview.
    // None is persisted: restored cards request fresh content as usual.
    open: function (path, o) {
      o = o || {};
      var data = { path: path };
      if (o.out) data.out = true;
      else {
        if (o.root) data.root = o.root;
        if (o.line > 0) data.line = o.line;
      }
      if (o.answer) answers.set(data, o.answer);
      var card = cards.list(KIND).filter(function (c) {
        return matches(c, data);
      })[0];
      if (card) {
        if (o.keep) promote(card);
        card.data = data;
        card.save();
        if (card.preview && !card.pinned) card.bump(o.origin);
        load(card);
      } else if (!o.keep && (card = slots()[0])) {
        card.data = data;
        card.bump(o.origin);
        load(card);
      } else {
        card = cards.open(KIND, data, undefined, o.origin);
        if (card) card.preview = !o.keep;
      }
      if (!card) return null;
      changed(card);
      cards.relayout();
      card.mediaError = o.mediaError || null;
      card.fold(false);
      card.show();
      return card;
    }
  };
})();
