;(function () {
  // Runs inside a served page's topic-body frame, a sandboxed srcdoc document in an
  // opaque origin (wswhiteboard framedDoc), after the body's own markup. The frame
  // cannot touch the host page, so it tells the host what the host acts on
  // through postMessage: the content height, so the host sizes the frame
  // (again whenever the host asks with {wbDoc: "size"}); a
  // marked link (data-wl) click, so the host opens its link menu; and an
  // item/panel-reference click (href="#item-nn" or "#pN"), so the host
  // reveals and scrolls to the existing board target; and an
  // image or diagram click, so the host opens its viewer over the whole page
  // ({wbDoc: "view"}: the image's source, or the diagram flattened by
  // wbFlattenSvg, which svg-view.js defines ahead of this script); and, for
  // an image outside the session root, a request for its bytes
  // ({wbDoc: "img"}, see relay below); and a text selection, which the
  // dashboard's live board offers to add to the message as a mention
  // ({wbDoc: "sel"}, see below); and the host's rail key, Ctrl+`
  // ({wbDoc: "rail"}, see the end).
  // A plain link needs no help: <base target="_blank"> opens it in a new tab.
  var d = document, body = d.querySelector(".doc-body");

  // The frame's mermaid blocks draw through this once the frame is laid
  // out: inside a collapsed body the frame has no width, and mermaid would
  // measure every label as empty. Until then mermaid's own draw on page
  // load is turned off. A theme switch draws through it again.
  var redraw = null;
  window.wbDocMermaid = function (draw) {
    redraw = draw;
    if (window.innerWidth) { draw(); return; }
    if (window.mermaid) window.mermaid.initialize({ startOnLoad: false });
    window.addEventListener("resize", function once() {
      if (!window.innerWidth) return;
      window.removeEventListener("resize", once);
      draw();
    });
  };

  if (body) wbWatchChanges(body);
  if (!body || window.parent === window) return;
  var keyMeta = d.querySelector('meta[name="wb-doc-key"]');
  var docKey = keyMeta && keyMeta.getAttribute("content");
  if (!docKey) return;
  // The host compares this key with the source iframe's own identity. It
  // never uses a claimed entry/topic locator to attribute a selection.
  function post(m) { m.docKey = docKey; window.parent.postMessage(m, "*"); }

  var last = -1;
  function size() {
    var h = Math.ceil(body.getBoundingClientRect().height);
    if (h === last) return;
    last = h;
    post({ wbDoc: "height", height: h });
  }
  if (window.ResizeObserver) new ResizeObserver(size).observe(body);
  window.addEventListener("load", size);
  // The host page may register its listener after this frame first posts;
  // it then asks for the height again. It also posts its pinned theme
  // ({wbDoc: "theme"}: "light", "dark", or "" for the OS) on a switch and in
  // answer to each height, since this document follows only its own root:
  // the server pins the theme current when it rendered the frame, and a
  // switch since then reaches the frame only this way. The frame's drawn
  // diagrams redraw in the new theme.
  function theme(v) {
    var root = d.documentElement;
    if (v !== "light" && v !== "dark") v = "";
    if ((root.getAttribute("data-theme") || "") === v) return;
    if (v) root.setAttribute("data-theme", v);
    else root.removeAttribute("data-theme");
    if (redraw && d.querySelector("pre.mermaid[data-processed]")) window.wbDocMermaid(redraw);
  }
  // An image outside the session root: the server leaves its path text
  // marked data-relay with the image route URL (wswhiteboard relayImage),
  // which this opaque origin cannot load (the route refuses it a cross-site
  // request). The frame asks the host page for it by index, and the host,
  // which fetches only a URL listed on this frame's own element, answers
  // with the image as a Blob. The image replaces the path text only once it
  // decodes; with no answer (a LAN viewer, a missing file, a refused fetch)
  // or a broken image the path text stays. A request lost to a host that was
  // not yet listening is asked again when the host asks for the height.
  var relays = Array.prototype.slice.call(d.querySelectorAll("span.img-text[data-relay]"));
  function askRelays() {
    relays.forEach(function (s, i) {
      if (!s.wbRelayed) post({ wbDoc: "img", id: i, href: s.getAttribute("data-relay") });
    });
  }
  function relayed(m) {
    var s = typeof m.id === "number" ? relays[m.id] : null, b = m.blob;
    if (!s || s.wbRelayed || !s.parentNode || !window.Blob || !(b instanceof Blob) || !/^image\//.test(b.type)) return;
    s.wbRelayed = true;
    var alt = s.cloneNode(true);
    Array.prototype.forEach.call(alt.querySelectorAll("a, code"), function (c) { c.parentNode.removeChild(c); });
    var r = new FileReader();
    r.onload = function () {
      var img = d.createElement("img"), code = s.querySelector("code");
      img.alt = alt.textContent.trim();
      if (code) img.title = code.textContent;
      function show() { if (s.parentNode) s.parentNode.replaceChild(img, s); }
      img.src = r.result;
      if (img.decode) img.decode().then(show, function () {});
      else { img.onload = show; }
    };
    r.readAsDataURL(b);
  }
  window.addEventListener("message", function (e) {
    if (e.source !== window.parent || !e.data || e.data.docKey !== docKey) return;
    if (e.data.wbDoc === "size") { last = -1; size(); askRelays(); }
    else if (e.data.wbDoc === "theme") theme(e.data.theme);
    else if (e.data.wbDoc === "img") relayed(e.data);
    else if (e.data.wbDoc === "unsel" && window.getSelection) window.getSelection().removeAllRanges();
  });

  // The selection: its text and where its end stands in this frame, "" once
  // it collapses (each change, but a collapsed one only once). The host
  // clears it after adding it ({wbDoc: "unsel"}).
  var selQueued = false, selLast = "";
  d.addEventListener("selectionchange", function () {
    if (selQueued || !window.getSelection) return;
    selQueued = true;
    requestAnimationFrame(function () {
      selQueued = false;
      var s = window.getSelection(), inside = s && body.contains(s.anchorNode) && body.contains(s.focusNode);
      var text = inside && !s.isCollapsed && s.rangeCount ? s.toString() : "";
      if (!text && !selLast) return;
      selLast = text;
      var r = null;
      if (text) {
        var rs = s.getRangeAt(0).getClientRects();
        r = rs.length ? rs[rs.length - 1] : s.getRangeAt(0).getBoundingClientRect();
      }
      post({ wbDoc: "sel", text: text, x: r ? r.right : 0, y: r ? r.bottom : 0 });
    });
  });
  size();
  askRelays();

  function link(a, e) {
    var r = a.getBoundingClientRect(), key = e.type === "keydown" || e.detail === 0;
    post({
      wbDoc: "link",
      kind: a.getAttribute("data-wl"),
      href: a.getAttribute("href") || "",
      rel: a.getAttribute("data-rel") || "",
      abs: a.getAttribute("data-abs") || "",
      img: a.hasAttribute("data-img"),
      x: key ? r.left : e.clientX,
      y: key ? r.bottom : e.clientY
    });
  }
  // Ahead of the link handling below. An image in a marked link or an
  // in-page anchor keeps that link's behavior; any other link around an image
  // opens the viewer, whose original button then reaches the link.
  d.addEventListener("click", function (e) {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var t = e.target, a = t && t.closest && t.closest("a"), img = t && t.closest && t.closest("img");
    if (!t || !t.closest || (a && (a.hasAttribute("data-wl") || (a.getAttribute("href") || "").charAt(0) === "#"))) return;
    if (img) {
      if (!img.complete || !img.naturalWidth) return;
      e.preventDefault();
      post({ wbDoc: "view", kind: "img", src: img.currentSrc || img.src, alt: img.alt, href: a ? a.href : "" });
      return;
    }
    var s = wbSvgTarget(t), flat = s && wbFlattenSvg(s);
    if (!flat) return;
    e.preventDefault();
    post({ wbDoc: "view", kind: "svg", svg: flat });
  }, true);
  d.addEventListener("click", function (e) {
    var t = e.target, a = t && t.closest && t.closest("a");
    if (!a || e.button !== 0) return;
    var href = a.getAttribute("href") || "";
    if (href.charAt(0) === "#") {
      // The frame never scrolls, so an in-page anchor only means something
      // on the host's board; a footnote's anchor names an element of this
      // frame and has nowhere to go.
      e.preventDefault();
      var id = href.slice(1);
      if (!/^(?:item-[0-9]{1,6}|p[1-9][0-9]*)$/.test(id)) return;
      post({ wbDoc: "jump", id: id });
      return;
    }
    if (!a.hasAttribute("data-wl")) return;
    // As on the host: a modified click on a link with an href is the
    // browser's new tab.
    if (href && (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) return;
    e.preventDefault();
    link(a, e);
  }, true);
  d.addEventListener("keydown", function (e) {
    var a = e.target && e.target.closest && e.target.closest("a[data-wl]:not([href])");
    if (a && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); link(a, e); }
  });
  // Ctrl+` (the host's rail key, same test): a key pressed in this frame
  // reaches only this document, so the frame hands it to the host
  // ({wbDoc: "rail"}), which decides whether the rail toggles.
  d.addEventListener("keydown", function (e) {
    if (e.code !== "Backquote" || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.isComposing) return;
    e.preventDefault();
    post({ wbDoc: "rail" });
  });
})();
