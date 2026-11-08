(function () {
  // The in-page image and diagram viewer: a click on a content image or a
  // rendered diagram opens a modal overlay over the whole page, with wheel and
  // pinch zoom about the pointer, drag to pan, and a small toolbar. One
  // delegated listener on the document serves every region swap; the overlay
  // is built on first use and lives on the body. A topic body renders inside a
  // sandboxed frame (doc-frame.js), which cannot reach this page: it posts the
  // clicked image's source or its flattened SVG (wbFlattenSvg), and the viewer
  // here opens it. A diagram is shown as an image of that flattened document, so
  // it stays vector (it is re-rendered at each zoom, text included) and nothing
  // a frame sends can run script on this page. Needs svg-view.js, which the
  // page script puts before it.
  var d = document, ov = null, stage, content, pct, orig, btns = {};
  var st = { w: 0, h: 0, s: 1, x: 0, y: 0, fit: 1, min: .1, max: 8, svg: false };
  var opener = null, prevFocus = null, urls = [], token = 0, ptrs = {}, drag = null, pinch = null, lastMoved = false;
  // Frame messages are accepted for an image source of these shapes only.
  var okImg = /^(https?:|data:image\/(png|jpe?g|gif|webp|avif|bmp|svg\+xml)[;,])/i;

  function t(key, fallback) {
    var s = typeof window.wsT === "function" ? window.wsT(key) : key;
    return s === key ? fallback : s;
  }
  function icon(name) { return '<svg class="i" aria-hidden="true"><use href="#i-' + name + '"/></svg>'; }
  function button(act, name, title) {
    var b = d.createElement("button");
    b.type = "button"; b.className = "wbv-btn"; b.title = title; b.setAttribute("aria-label", title);
    b.setAttribute("data-act", act); b.innerHTML = icon(name);
    btns[act] = b;
    return b;
  }

  function build() {
    if (ov) return;
    ov = d.createElement("div");
    ov.className = "wbv"; ov.hidden = true; ov.tabIndex = -1;
    ov.setAttribute("role", "dialog"); ov.setAttribute("aria-modal", "true");
    ov.setAttribute("aria-label", t("js.viewer.label", "Image viewer"));
    stage = d.createElement("div"); stage.className = "wbv-stage";
    content = d.createElement("img"); content.className = "wbv-content"; content.draggable = false; content.alt = "";
    stage.appendChild(content);
    var bar = d.createElement("div"); bar.className = "wbv-bar";
    pct = d.createElement("span"); pct.className = "wbv-pct";
    orig = d.createElement("a"); orig.className = "wbv-btn"; orig.target = "_blank"; orig.rel = "noopener noreferrer"; orig.hidden = true;
    orig.title = t("js.viewer.original", "Open the original in a new tab"); orig.setAttribute("aria-label", orig.title);
    orig.innerHTML = icon("ext");
    bar.appendChild(button("out", "zoom-out", t("js.viewer.zoomout", "Zoom out")));
    bar.appendChild(pct);
    bar.appendChild(button("in", "zoom-in", t("js.viewer.zoomin", "Zoom in")));
    bar.appendChild(button("fit", "fit", t("js.viewer.fit", "Fit to window")));
    bar.appendChild(orig);
    bar.appendChild(button("close", "x", t("js.viewer.close", "Close")));
    ov.appendChild(stage); ov.appendChild(bar);
    d.body.appendChild(ov);

    bar.addEventListener("click", function (e) {
      var b = e.target.closest("[data-act]");
      if (!b) return;
      var z = size();
      if (b.getAttribute("data-act") === "in") zoomAt(1.25, z.w / 2, z.h / 2);
      else if (b.getAttribute("data-act") === "out") zoomAt(.8, z.w / 2, z.h / 2);
      else if (b.getAttribute("data-act") === "fit") toFit();
      else close();
    });
    stage.addEventListener("wheel", function (e) {
      e.preventDefault();
      var dy = e.deltaMode === 1 ? e.deltaY * 16 : e.deltaMode === 2 ? e.deltaY * 100 : e.deltaY;
      var p = local(e.clientX, e.clientY);
      zoomAt(Math.exp(-dy * (e.ctrlKey ? .01 : .0018)), p.x, p.y);
    }, { passive: false });
    stage.addEventListener("pointerdown", function (e) {
      if (e.pointerType === "mouse" && e.button !== 0) return;
      ptrs[e.pointerId] = { x: e.clientX, y: e.clientY };
      try { stage.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer */ }
      var ids = Object.keys(ptrs);
      if (ids.length === 1) {
        drag = { x: e.clientX, y: e.clientY, tx: st.x, ty: st.y, moved: false };
        lastMoved = false;
      } else if (ids.length === 2) {
        startPinch();
      }
    });
    stage.addEventListener("pointermove", function (e) {
      if (!ptrs[e.pointerId]) return;
      ptrs[e.pointerId] = { x: e.clientX, y: e.clientY };
      if (pinch) {
        var ids = Object.keys(ptrs), a = ptrs[ids[0]], b = ptrs[ids[1]];
        var dist = Math.hypot(a.x - b.x, a.y - b.y) || 1, mid = local((a.x + b.x) / 2, (a.y + b.y) / 2);
        var ns = Math.min(st.max, Math.max(st.min, pinch.s * dist / pinch.dist));
        st.s = ns; st.x = mid.x - pinch.cx * ns; st.y = mid.y - pinch.cy * ns;
        apply();
      } else if (drag) {
        var dx = e.clientX - drag.x, dy = e.clientY - drag.y;
        if (!drag.moved && Math.hypot(dx, dy) < 4) return;
        drag.moved = lastMoved = true;
        stage.classList.add("drag");
        st.x = drag.tx + dx; st.y = drag.ty + dy;
        apply();
      }
    });
    function release(e) {
      if (!ptrs[e.pointerId]) return;
      delete ptrs[e.pointerId];
      var ids = Object.keys(ptrs);
      pinch = null;
      if (ids.length === 1) {
        // One finger of a pinch is still down: it carries on as a pan.
        drag = { x: ptrs[ids[0]].x, y: ptrs[ids[0]].y, tx: st.x, ty: st.y, moved: true };
        lastMoved = true;
      } else if (!ids.length) {
        drag = null; stage.classList.remove("drag");
      }
    }
    stage.addEventListener("pointerup", release);
    stage.addEventListener("pointercancel", release);
    // The click after a plain press on the backdrop closes; one after a drag does not.
    stage.addEventListener("click", function (e) {
      if (lastMoved) { lastMoved = false; return; }
      if (!onContent(e.clientX, e.clientY)) close();
    });
    stage.addEventListener("dblclick", function (e) {
      if (!onContent(e.clientX, e.clientY)) return;
      var p = local(e.clientX, e.clientY);
      if (Math.abs(st.s - st.fit) < .01) {
        var target = st.svg ? st.fit * 2 : (st.fit < 1 ? 1 : 2);
        zoomAt(target / st.s, p.x, p.y);
      } else {
        toFit();
      }
    });
    stage.addEventListener("dragstart", function (e) { e.preventDefault(); });
    content.addEventListener("error", function () { if (!ov.hidden) close(); });
  }

  function size() { return { w: stage.clientWidth, h: stage.clientHeight }; }
  function local(cx, cy) { var r = stage.getBoundingClientRect(); return { x: cx - r.left, y: cy - r.top }; }
  function onContent(cx, cy) {
    var r = content.getBoundingClientRect();
    return cx >= r.left && cx <= r.right && cy >= r.top && cy <= r.bottom;
  }
  function startPinch() {
    var ids = Object.keys(ptrs), a = ptrs[ids[0]], b = ptrs[ids[1]];
    var mid = local((a.x + b.x) / 2, (a.y + b.y) / 2);
    pinch = { dist: Math.hypot(a.x - b.x, a.y - b.y) || 1, s: st.s, cx: (mid.x - st.x) / st.s, cy: (mid.y - st.y) / st.s };
    drag = null; lastMoved = true;
  }
  // The scale limits follow the content: the window fit, and no element wider
  // than the browser can paint.
  function limits() {
    var z = size(), f = Math.min(z.w * .94 / st.w, z.h * .92 / st.h);
    st.fit = st.svg ? Math.min(f, 4) : Math.min(f, 1);
    st.min = st.fit * .5;
    st.max = Math.max(st.fit * 4, Math.min(32, 24000 / Math.max(st.w, st.h)));
  }
  function apply() {
    var z = size(), cw = st.w * st.s, ch = st.h * st.s;
    // Smaller than the window it sits centred; larger, it cannot be dragged off.
    st.x = cw <= z.w ? (z.w - cw) / 2 : Math.min(0, Math.max(z.w - cw, st.x));
    st.y = ch <= z.h ? (z.h - ch) / 2 : Math.min(0, Math.max(z.h - ch, st.y));
    content.style.width = cw + "px"; content.style.height = ch + "px";
    content.style.transform = "translate(" + st.x + "px," + st.y + "px)";
    pct.textContent = Math.round(st.s * 100) + "%";
  }
  function zoomAt(k, px, py) {
    var ns = Math.min(st.max, Math.max(st.min, st.s * k)), r = ns / st.s;
    st.x = px - (px - st.x) * r; st.y = py - (py - st.y) * r; st.s = ns;
    apply();
  }
  function toFit() { limits(); st.s = st.fit; st.x = st.y = 0; apply(); }

  function open(spec) {
    build();
    if (!ov.hidden) dropUrls();
    var my = ++token, src = spec.src, svg = false;
    prevFocus = d.activeElement; opener = spec.opener || null;
    if (spec.svg) {
      var u = URL.createObjectURL(new Blob([spec.svg], { type: "image/svg+xml" }));
      urls.push(u); src = u; svg = true;
    } else {
      svg = /^data:image\/svg\+xml/i.test(src) || /\.svg([?#]|$)/i.test(src);
    }
    // The original: the link the image sits in, else the image itself. A diagram
    // has none, and an SVG document would run its scripts opened as a page.
    orig.hidden = true; orig.removeAttribute("href");
    var href = spec.href || (spec.svg || /^data:image\/svg/i.test(src) ? "" : src);
    if (/^data:image\//i.test(href) && !/^data:image\/svg/i.test(href)) {
      // A browser will not open a data: URL as a page; its bytes behind a blob: URL it will.
      fetch(href).then(function (r) { return r.blob(); }).then(function (b) {
        if (my !== token) return;
        var bu = URL.createObjectURL(b);
        urls.push(bu); orig.href = bu; orig.hidden = false;
      }).catch(function () { /* the original stays unoffered */ });
    } else if (/^(https?|file):/i.test(href)) {
      orig.href = href; orig.hidden = false;
    }
    content.alt = spec.alt || "";
    content.removeAttribute("style");
    content.style.visibility = "hidden";
    content.onload = function () {
      if (my !== token) return;
      st.w = content.naturalWidth || 1; st.h = content.naturalHeight || 1; st.svg = svg;
      limits(); st.s = st.fit; st.x = st.y = 0;
      apply();
      content.style.visibility = "";
    };
    ov.hidden = false;
    d.documentElement.classList.add("wbv-open");
    content.src = src;
    ov.focus({ preventScroll: true });
  }
  function dropUrls() { token++; urls.forEach(function (u) { URL.revokeObjectURL(u); }); urls = []; }

  function close() {
    if (!ov || ov.hidden) return;
    ov.hidden = true;
    d.documentElement.classList.remove("wbv-open");
    content.onload = null;
    content.removeAttribute("src");
    dropUrls();
    ptrs = {}; drag = pinch = null; stage.classList.remove("drag");
    var o = opener, p = prevFocus;
    opener = prevFocus = null;
    if (o && o.isConnected && typeof o.focus === "function") o.focus({ preventScroll: true });
    if (p && p.isConnected && d.activeElement !== o && typeof p.focus === "function") p.focus({ preventScroll: true });
    // The live page holds a board change that arrived meanwhile until now.
    d.dispatchEvent(new Event("wbviewerclose"));
  }

  var IMG = ".md img, .doc-body img, .thumbs img, .bub img";
  // viewable reads a click: an image or diagram of the page's content, never one
  // in a marked link (data-wl: the link menu) or an in-page anchor.
  function viewable(e) {
    var tg = e.target;
    if (!tg || !tg.closest) return null;
    var a = tg.closest("a"), img = tg.closest("img");
    // A keyboard activation of a link lands on the link, not the image in it.
    if (!img && a && !a.textContent.trim() && a.querySelectorAll("img").length === 1) img = a.querySelector("img");
    if (img) {
      if (!img.matches(IMG) || !img.complete || !img.naturalWidth) return null;
      var link = img.closest("a");
      if (link && (link.hasAttribute("data-wl") || (link.getAttribute("href") || "").charAt(0) === "#")) return null;
      return { src: img.currentSrc || img.src, alt: img.alt, href: link ? link.href : "", opener: link || img };
    }
    var s = wbSvgTarget(tg);
    if (!s) return null;
    var flat = wbFlattenSvg(s);
    return flat ? { svg: flat, opener: s.closest("a, button") || s } : null;
  }
  d.addEventListener("click", function (e) {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (ov && ov.contains(e.target)) return;
    var v = viewable(e);
    if (!v) return;
    // Ahead of the click behavior of whatever holds the image: a link
    // follows, an option picks, a card confirms.
    e.preventDefault(); e.stopPropagation();
    open(v);
  }, true);

  window.addEventListener("message", function (e) {
    var m = e.data;
    if (!m || typeof m !== "object" || m.wbDoc !== "view") return;
    var f = Array.prototype.filter.call(d.querySelectorAll("iframe.doc-frame"), function (x) { return x.contentWindow === e.source; })[0];
    if (!f) return;
    var s = function (v) { return typeof v === "string" ? v : ""; };
    if (m.kind === "img") {
      if (!okImg.test(s(m.src))) return;
      open({ src: s(m.src), alt: s(m.alt).slice(0, 500), href: /^https?:\/\//i.test(s(m.href)) ? s(m.href) : "", opener: f });
    } else if (m.kind === "svg") {
      if (!/^<svg[\s>]/.test(s(m.svg)) || s(m.svg).length > 30000000) return;
      open({ svg: s(m.svg), opener: f });
    }
  });

  d.addEventListener("keydown", function (e) {
    if (!ov || ov.hidden || e.ctrlKey || e.metaKey || e.altKey) return;
    var z = size(), k = e.key, handled = true;
    if (k === "Escape") close();
    else if (k === "+" || k === "=") zoomAt(1.25, z.w / 2, z.h / 2);
    else if (k === "-" || k === "_") zoomAt(.8, z.w / 2, z.h / 2);
    else if (k === "0") toFit();
    else if (k === "ArrowLeft") { st.x += 60; apply(); }
    else if (k === "ArrowRight") { st.x -= 60; apply(); }
    else if (k === "ArrowUp") { st.y += 60; apply(); }
    else if (k === "ArrowDown") { st.y -= 60; apply(); }
    else if (k === "Tab") {
      // The dialog is modal: Tab cycles among its own controls.
      var f = Array.prototype.filter.call(ov.querySelectorAll("button, a[href]"), function (x) { return !x.hidden; });
      var i = f.indexOf(d.activeElement);
      f[i < 0 ? (e.shiftKey ? f.length - 1 : 0) : (i + (e.shiftKey ? f.length - 1 : 1)) % f.length].focus();
    } else handled = false;
    if (handled) { e.preventDefault(); e.stopPropagation(); }
  }, true);
  window.addEventListener("resize", function () {
    if (!ov || ov.hidden || !st.w) return;
    var fitted = Math.abs(st.s - st.fit) < .001;
    limits();
    if (fitted) { st.s = st.fit; st.x = st.y = 0; } else st.s = Math.min(st.max, Math.max(st.min, st.s));
    apply();
  });
})();
