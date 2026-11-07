;(function () {
  // Runs inside a served page's topic-body frame, a sandboxed srcdoc document in an
  // opaque origin (wswhiteboard framedDoc), after the body's own markup. The frame
  // cannot touch the host page, so it tells the host what the host acts on
  // through postMessage: the content height, so the host sizes the frame
  // (again whenever the host asks with {wbDoc: "size"}); a
  // marked link (data-wl) click, so the host opens its link menu; and an
  // item-reference click (href="#item-nn"), so the host scrolls to the item.
  // A plain link needs no help: <base target="_blank"> opens it in a new tab.
  var d = document, body = d.querySelector(".doc-body");

  // The frame's mermaid blocks draw through this once the frame is laid
  // out: inside a collapsed body the frame has no width, and mermaid would
  // measure every label as empty. Until then mermaid's own draw on page
  // load is turned off.
  window.wbDocMermaid = function (draw) {
    if (window.innerWidth) { draw(); return; }
    if (window.mermaid) window.mermaid.initialize({ startOnLoad: false });
    window.addEventListener("resize", function once() {
      if (!window.innerWidth) return;
      window.removeEventListener("resize", once);
      draw();
    });
  };

  if (!body || window.parent === window) return;
  function post(m) { window.parent.postMessage(m, "*"); }

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
  // it then asks for the height again.
  window.addEventListener("message", function (e) {
    if (e.source === window.parent && e.data && e.data.wbDoc === "size") { last = -1; size(); }
  });
  size();

  function link(a, e) {
    var r = a.getBoundingClientRect(), key = e.type === "keydown" || e.detail === 0;
    post({
      wbDoc: "link",
      kind: a.getAttribute("data-wl"),
      href: a.getAttribute("href") || "",
      rel: a.getAttribute("data-rel") || "",
      abs: a.getAttribute("data-abs") || "",
      x: key ? r.left : e.clientX,
      y: key ? r.bottom : e.clientY
    });
  }
  d.addEventListener("click", function (e) {
    var t = e.target, a = t && t.closest && t.closest("a");
    if (!a || e.button !== 0) return;
    var href = a.getAttribute("href") || "";
    if (href.charAt(0) === "#") {
      // The frame never scrolls, so an in-page anchor only means something
      // on the host's board; a footnote's anchor names an element of this
      // frame and has nowhere to go.
      e.preventDefault();
      if (d.getElementById(href.slice(1))) return;
      post({ wbDoc: "jump", id: href.slice(1) });
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
})();
