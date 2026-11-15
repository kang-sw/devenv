  // --- the board's topic-body frames -------------------------------------------
  // The server renders each topic body on the board in its own sandboxed
  // frame with an opaque origin (wswhiteboard framedDoc), so its scripts
  // cannot reach this page or the daemon's routes. The frame's script
  // (doc-frame.js) posts its content height, its marked-link clicks, its
  // item-reference clicks, and its requests for an image outside the
  // session root (relayImage). A message counts only when its source is a body
  // frame on this page, and its
  // fields are data, checked as such: a link opens the same menu a board
  // link does, with an href kept only in the shapes the renderer gives one
  // (an http(s) URL for a url link, a scheme-less daemon path for a file
  // link), so a forged message can at most offer a menu the user must pick
  // from.
  // Lazy frames: the server writes each frame as an inert placeholder, a
  // div.doc-frame carrying the frame's attributes, its document as
  // data-srcdoc and its sandbox tokens as data-sandbox (wswhiteboard
  // framedDoc). It becomes the iframe (mountFrame) only while every
  // <details> around it is open (frameShown): on page load, on a toggle open
  // (frameToggle, a capture listener, as toggle does not bubble; a
  // programmatic open such as restoreOpen's or boardJump's fires toggle
  // too), and for what a board swap, an adopted closed page, or a history
  // load inserts; where the browser has scroll margins, only near the view
  // as well (frameNear, the viewport bands below), and a frame far from the
  // view unmounts again. A closed fold keeps its frames.
  // Readers that need a live window (docFrame, postTheme, the size request,
  // wbGrow, wbKeep) select iframe.doc-frame and skip placeholders; the
  // height memory and the palette read .doc-frame, either kind.
  function frameShown(f) {
    if (!f.isConnected) return false;
    for (var p = f.parentNode; p && p.nodeType === 1; p = p.parentNode) if (p.tagName === "DETAILS" && !p.open) return false;
    return true;
  }
  // mountFrame replaces placeholder p with its iframe and returns the
  // iframe. sandbox and srcdoc are set before the iframe is connected: a
  // sandbox set later applies only to the next navigation, and an iframe
  // connected without srcdoc commits about:blank at once. data-srcdoc stays
  // for comparisons.
  function mountFrame(p) {
    var f = doc.createElement("iframe");
    Array.prototype.forEach.call(p.attributes, function (a) { if (a.name !== "data-sandbox") f.setAttribute(a.name, a.value); });
    f.setAttribute("sandbox", p.getAttribute("data-sandbox") || "");
    f.setAttribute("srcdoc", p.getAttribute("data-srcdoc") || "");
    // A frame never laid out (no height yet) grows its topic when its first
    // height lands, and a frame mounted again (unmountFrame) may land at
    // another height than the one its placeholder kept (a narrower column,
    // a diagram drawn again): topicResized keeps the view through either
    // (wbFirst). The marks a board swap left for a frame's first height
    // (wbKeep, wbGrow) stay on a frame unmounted before that height.
    if (!p.style.height || p.wbUnmounted) f.wbFirst = true;
    if (p.wbKeep) f.wbKeep = true;
    if (p.wbGrow) f.wbGrow = true;
    if (p.parentNode) p.parentNode.replaceChild(f, p);
    return f;
  }
  function mountFrames(el) {
    q("div.doc-frame[data-srcdoc]", el).forEach(function (p) {
      if (frameShown(p) && frameNear(p)) mountFrame(p);
    });
  }
  // mountFramesPaced is mountFrames over a large part of the board (the
  // page load's whole board, a topic coming near the view), in slices of
  // about 8 ms through one shared list: the first slice runs now unless one
  // is already pending, each later one in its own task, so mounting many
  // frames at once never lengthens the longest task (measured with the
  // board performance harness). A placeholder a swap or a toggle open took
  // meanwhile is skipped (frameShown checks it is still connected).
  function mountFramesPaced(el) {
    mountList = mountList.concat(q("div.doc-frame[data-srcdoc]", el));
    if (!mountTask) mountSlice();
  }
  function mountSlice() {
    var t = performance.now();
    mountTask = true;
    while (mountList.length && performance.now() - t < 8) {
      var p = mountList.shift();
      if (frameShown(p) && frameNear(p)) mountFrame(p);
    }
    mountTask = mountList.length > 0;
    if (mountTask) setTimeout(mountSlice, 0);
  }
  function frameToggle(e) {
    var d = e.target;
    if (d && d.tagName === "DETAILS" && d.open) mountFrames(d);
  }
  doc.addEventListener("toggle", frameToggle, true);
  // --- viewport bands: open frames mount near the view, unmount far ----------
  // Where the view kit has scroll margins (wbHistory.bands), each board
  // topic is watched in two bands (bandTopic): NEAR, a viewport above and
  // below the view, and FAR, four. A topic entering NEAR mounts its shown
  // placeholders, paced; one leaving FAR turns its frames back into
  // placeholders that keep their height (unmountFrames), so nothing moves.
  // Between the two a topic keeps what it has, so scrolling back and forth
  // reloads nothing. The bands are per topic, not per frame. A frame
  // mounts only where its topic is near (frameNear); a topic not reported
  // yet takes the last report for its name (a changed topic's new section),
  // else waits for its first report. A frame in no topic, and every frame
  // where the browser lacks scroll margins, mounts as before.
  function boardBands() { return !!(window.wbHistory && window.wbHistory.bands && window.wbHistory.watch); }
  function frameNear(f) {
    if (!boardBands()) return true;
    var t = f.parentNode ? f.parentNode.closest("section.topic[data-topic]") : null;
    if (!t) return true;
    bandTopic(t);
    var near = t.wbNear !== undefined ? t.wbNear : topicNear[t.getAttribute("data-topic")] === true;
    return near;
  }
  // bandBoard puts the board's topic sections under root (root itself
  // included) under the bands, once each; frameNear bands a topic it meets
  // too.
  function bandBoard(root) {
    if (!boardBands() || !root || root.nodeType !== 1) return;
    (root.matches("section.topic[data-topic]") ? [root] : q("section.topic[data-topic]", root)).forEach(bandTopic);
  }
  function bandTopic(t) {
    if (t.wbBand) return;
    var w = window.wbHistory;
    t.wbBand = [
      w.watch(t, { margin: NEAR, onChange: function (r) { topicBand(t, r, true); } }),
      w.watch(t, { margin: FAR, onChange: function (r) { topicBand(t, r, false); } })
    ];
    if (window.ResizeObserver) (topicRO || (topicRO = new ResizeObserver(topicResized))).observe(t);
  }
  // topicBand takes a band report for topic t. A report with no box (a
  // hidden board column, a topic taken off the page) tells no distance.
  function topicBand(t, r, near) {
    if (!r.rect || (!r.rect.width && !r.rect.height)) return;
    if (!near) { if (!r.visible) unmountFrames(t); return; }
    t.wbNear = r.visible;
    topicNear[t.getAttribute("data-topic")] = r.visible;
    if (r.visible) mountFramesPaced(t);
  }
  // unmountFrames turns topic t's frames back into placeholders, except a
  // frame the page still holds: a graft's pair (wbNext, wbOut), the focused
  // frame, one holding the user's selection (wbHoldSelection), and the one
  // a mention button stands over (mpend).
  function unmountFrames(t) {
    q("iframe.doc-frame[data-srcdoc]", t).forEach(function (f) {
      if (f.wbNext || f.wbOut || f.wbHoldSelection || f === doc.activeElement || (mpend && mpend.frame === f)) return;
      unmountFrame(f);
    });
  }
  // unmountFrame replaces frame f with its placeholder, mountFrame's
  // inverse, and returns it: the same attributes, the sandbox tokens back
  // as data-sandbox (data-srcdoc stayed), the height the frame stood at, or
  // the one it reported last when that has not landed yet, and the marks
  // a board swap left for its first height (wbKeep, wbGrow), which the
  // frame mounted again takes; it is marked as unmounted (wbUnmounted).
  function unmountFrame(f) {
    var p = doc.createElement("div");
    p.wbUnmounted = true;
    if (f.wbKeep) p.wbKeep = true;
    if (f.wbGrow) p.wbGrow = true;
    Array.prototype.forEach.call(f.attributes, function (a) { if (a.name !== "srcdoc" && a.name !== "sandbox") p.setAttribute(a.name, a.value); });
    p.setAttribute("data-sandbox", f.getAttribute("sandbox") || "");
    if (frameSizes.has(f)) { p.style.height = Math.ceil(frameSizes.get(f)) + "px"; frameSizes.delete(f); }
    if (f.parentNode) f.parentNode.replaceChild(p, f);
    return p;
  }
  // The board's scrollers take no native scroll anchoring (dashboard.css),
  // so what the bands themselves change above the view would move what the
  // user reads: a frame mounted as its topic comes near resizes the topic
  // when its first height lands at another height than it stood at
  // (wbFirst, then wbLanded: applyFrameHeights).
  // topicResized scrolls by such a change of a topic standing wholly above
  // the view. A running keep session (a swap, a jump, a history load)
  // compensates by itself, a topic in view or below it moves nothing the
  // user reads, and any other change (a frame already shown reporting a new
  // height) is left as it was before the bands.
  function topicResized(list) {
    var w = window.wbHistory;
    list.forEach(function (e) {
      var t = e.target, was = t.wbSize, box = e.borderBoxSize && e.borderBoxSize[0], h = box ? box.blockSize : e.contentRect.height;
      if (!t.isConnected) { topicRO.unobserve(t); return; }
      t.wbSize = h;
      var banded = topicSettling(t);
      if (!banded || !was || !h || Math.abs(h - was) < 0.5 || t.hasAttribute("data-max")) return;
      var s = topicScroller(t);
      if (w && w.keeping && w.keeping(s)) return;
      var r = t.getBoundingClientRect(), top = s === doc.scrollingElement ? 0 : s.getBoundingClientRect().top + s.clientTop;
      if (r.height && r.bottom - (h - was) <= top) s.scrollTop += h - was;
    });
  }
  // topicSettling tells whether topic t's last size change is the bands'
  // own: it holds a frame whose first height has just landed as a change
  // (each one counts once).
  function topicSettling(t) {
    var any = false;
    q("iframe.doc-frame", t).forEach(function (f) {
      if (!f.wbLanded) return;
      f.wbLanded = false;
      any = true;
    });
    return any;
  }
  // topicScroller is the element scrolling topic t: the pinned column or
  // the board column where it scrolls, else the document.
  function topicScroller(t) {
    var list = [t.closest(".bpins"), $("bcol")];
    for (var i = 0; i < list.length; i++) {
      var s = list[i];
      if (s && s.contains(t) && s.scrollHeight > s.clientHeight && /auto|scroll/.test(getComputedStyle(s).overflowY)) return s;
    }
    return doc.scrollingElement;
  }
  // docFrame resolves through frameWins (contentWindow -> board frame),
  // indexed again by every board swap and on any miss: a frame that moved
  // without Element.moveBefore (a pin, a reorder) took a new window, and a
  // message from a window that is no board frame finds none either way.
  var frameWins = new Map();
  function indexFrames() {
    frameWins = new Map();
    boardQ("iframe.doc-frame").forEach(function (f) { if (f.contentWindow) frameWins.set(f.contentWindow, f); });
  }
  function docFrame(win) {
    var f = frameWins.get(win);
    if (f && f.contentWindow === win && inBoard(f)) return f;
    indexFrames();
    f = frameWins.get(win);
    return f && inBoard(f) ? f : undefined;
  }
  // Only existing board items, latest stable panels and encoded topic anchors
  // may be navigated. Topic chips never materialize unloaded closed topics.
  function boardJump(id) {
    if (typeof id !== "string" || !/^(?:item-[0-9]{1,6}|p[1-9][0-9]*|topic-(?:[0-9a-f]{2})+)$/.test(id)) return;
    var isTopic = id.indexOf("topic-") === 0;
    var target = null, unfoldTopic = window.WsCards && window.WsCards.unfoldTopic;
    if (!isTopic && window.wbHistory) boardBoxes().some(function (b) { target = window.wbHistory.loadTarget(b, id, unfoldTopic); return !!target; });
    if (!target) target = $(id);
    if (!target || target.id !== id || !inBoard(target)) return;
    if (isTopic && (target.tagName !== "SECTION" || !target.hasAttribute("data-topic"))) return;
    function reveal() {
      if (unfoldTopic) unfoldTopic(target);
      for (var p = target; p && inBoard(p); p = p.parentNode) if (p.tagName === "DETAILS") p.open = true;
      // History targets were anchored by the loader; never scroll their clip.
      if (!target.closest(".wb-history")) {
        target.scrollIntoView({ block: "start" });
        var w = window.wbHistory;
        if (w && w.bands && w.keep) w.keep(target);
      }
    }
    if (isTopic && window.WsCards && window.WsCards.revealTopic) window.WsCards.revealTopic(target, reveal);
    else reveal();
    return target;
  }
  function captureBoardHistory() {
    return window.wbHistory ? boardBoxes().reduce(function (all, b) { return all.concat(window.wbHistory.capture(b)); }, []) : null;
  }
  // Native fragments also need to materialize paged panels and open folds.
  doc.addEventListener("click", function (e) {
    var a = e.target.closest && e.target.closest("a[href]");
    if (!a || !inBoard(a) || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var href = a.getAttribute("href") || "";
    if (href.charAt(0) !== "#" || !boardJump(href.slice(1))) return;
    e.preventDefault();
    try { window.history.replaceState(null, "", href); } catch (x) {}
  });
  doc.addEventListener("wbhistoryload", function (e) {
    var el = e.detail && e.detail.root;
    if (!el || !inBoard(el)) return;
    restoreOpen(el);
    mountFrames(el);
    fmtTimes(el);
    sweepOutImgs(el);
    if (window.wbBindBoard) window.wbBindBoard(el);
    mermaid([el]);
  });
  window.addEventListener("hashchange", function () { boardJump(location.hash.slice(1)); });
  if (location.hash) boardJump(location.hash.slice(1));
  function str(v) { return typeof v === "string" ? v : ""; }
  function num(v) { return typeof v === "number" && isFinite(v) ? v : 0; }
  function frameLink(m) {
    var kind = str(m.kind), href = str(m.href);
    if (kind === "url") { if (!/^https?:\/\//i.test(href)) return null; }
    else if (kind === "file") { if (!/^\/[^\/\\\x00-\x20\x7f][^\\\x00-\x20\x7f]*$/.test(href)) href = ""; }
    else return null;
    var a = doc.createElement("a");
    a.setAttribute("data-wl", kind);
    if (href) a.setAttribute("href", href);
    if (str(m.rel)) a.setAttribute("data-rel", str(m.rel));
    if (str(m.abs)) a.setAttribute("data-abs", str(m.abs));
    if (kind === "file" && m.img === true) a.setAttribute("data-img", "");
    return a;
  }
  // relayHref is the URL the host fetches for a frame's image request
  // ({wbDoc: "img"}): href itself, only when it is on list, the frame
  // element's own data-relay (wswhiteboard framedDoc, which the body's markup
  // cannot reach), and only as this session's image route for an image file
  // name with no other query part; "" otherwise.
  function relayHref(list, href) {
    var pre = "/s/" + rid + "/image?path=", path;
    if (!rid || !href || typeof href !== "string" || (" " + list + " ").indexOf(" " + href + " ") < 0) return "";
    if (href.indexOf(pre) !== 0 || /[\s&#]/.test(href)) return "";
    try { path = decodeURIComponent(href.slice(pre.length).replace(/\+/g, " ")); } catch (x) { return ""; }
    return /\.(avif|bmp|gif|ico|jpe?g|png|svg|webp)$/i.test(path) ? href : "";
  }
  // relayImage answers a topic-body frame's image request: the frame's
  // opaque origin cannot load an image outside the session root from the
  // image route, which refuses it a cross-site request, so this page, same
  // origin and served that image only for a loopback viewer, fetches it and
  // posts the frame the bytes as a Blob. This is a security boundary between
  // a body's scripts and this page's origin, and it stays narrow: only a
  // message from one of this page's own body frames (docFrame), only a URL
  // relayHref allows for that frame, only an answer that is an image, and
  // only back to the frame that asked. It is never a generic fetch relay. A
  // failed fetch (a LAN viewer, a missing file) answers nothing, and the
  // frame keeps the path text.
  function relayImage(f, m) {
    var href = relayHref(f.getAttribute("data-relay") || "", m.href), id = m.id;
    if (!href || typeof id !== "number" || !window.fetch) return;
    // A frame asks again when the host asks for its height, in case its
    // first request went out before this listener; each frame element
    // fetches each of its requests once.
    var asked = f.wbRelayAsked || (f.wbRelayAsked = {});
    if (asked[id]) return;
    asked[id] = true;
    fetch(href, { credentials: "same-origin", redirect: "error" }).then(function (r) {
      var type = (r.headers.get("Content-Type") || "").split(";")[0].trim().toLowerCase();
      return r.ok && /^image\/[a-z0-9.+-]+$/.test(type) ? r.blob() : null;
    }).then(function (b) {
      if (b) postDoc(f, { wbDoc: "img", id: id, blob: b });
    }, function () {});
  }
  // A frame's height lands in the next frame callback, with every other
  // height reported meanwhile (applyFrameHeights): one host layout for a
  // board's worth of frames rather than one per message. The height it
  // eases from is its set height (style.height), 0 when it has none, as the
  // stylesheet's iframe.doc-frame starts it, so nothing reads layout.
  var frameSizes = new Map(), frameSizeFrame = 0;
  function queueFrameHeight(f, h) {
    frameSizes.set(f, h);
    if (!frameSizeFrame) frameSizeFrame = requestAnimationFrame(applyFrameHeights);
  }
  function applyFrameHeights() {
    // While overview is active or transitioning, board frames are inside
    // display:none containers and report collapsed heights. Applying those
    // would overwrite the saved inline heights that exit uses to restore the
    // unfolded layout, causing a height jump at the end of the exit motion.
    // Re-queue until overview releases.
    if (window.WsOverview && window.WsOverview.holding()) {
      frameSizeFrame = requestAnimationFrame(applyFrameHeights);
      return;
    }
    var batch = frameSizes, b = region("board"), kept = null;
    frameSizes = new Map();
    frameSizeFrame = 0;
    // A frame a board swap brought (wbKeep) reports its height late, after
    // the swap's keep session may have ended: its first change keeps the
    // board in place as the swap did, one session for the whole batch,
    // started before any height lands. A session still running keeps the
    // anchor the swap picked (the topmost entry in view may be one the swap
    // moved).
    batch.forEach(function (h, f) {
      if (!f.wbKeep || Math.abs(Math.ceil(h) - (parseFloat(f.style.height) || 0)) < 1 || !b || !b.contains(f)) return;
      if (!kept) kept = boardKept() || boardKeep(b);
      if (kept) f.wbKeep = false;
    });
    batch.forEach(function (h, f) {
      var from = parseFloat(f.style.height) || 0, to = Math.ceil(h);
      f.style.height = to + "px";
      // A frame the bands mounted (wbFirst, mountFrame) resizes its topic
      // by the bands' doing once, when its first height changes it.
      if (f.wbFirst) {
        f.wbFirst = false;
        if (Math.abs(to - from) >= 1) f.wbLanded = true;
      }
      // The first height a frame of a moving board swap reports eases
      // from the height the frame stood at (board motion, slideBoard).
      if (f.wbGrow) {
        f.wbGrow = false;
        if (f.animate && Math.abs(to - from) >= 1) f.animate([{ height: from + "px" }, { height: to + "px" }], { duration: 200, easing: "ease-out" });
      }
      // A frame rendered before a theme switch (a board swap on a stream
      // opened earlier) takes the current theme when it first reports.
      postTheme(f);
    });
    if (kept) kept.h.apply();
  }
  window.addEventListener("message", function (e) {
    var f = docFrame(e.source), m = e.data;
    if (!f || !m || typeof m !== "object") return;
    var key = f.getAttribute("data-doc-key");
    if (!key || m.docKey !== key || f.getAttribute("data-doc") !== key) return;
    if (m.wbDoc === "height") {
      var h = num(m.height);
      // A pending frame (frameGraft) keeps the old frame's height: once
      // shown it is asked for its height again.
      if (f.wbNext) { postTheme(f); return; }
      if (h >= 0 && h < 100000) queueFrameHeight(f, h);
      else postTheme(f);
    } else if (m.wbDoc === "jump") {
      boardJump(str(m.id));
    } else if (m.wbDoc === "link") {
      var a = frameLink(m);
      if (!a) return;
      a.wsPreviewOrigin = f.closest("section.topic[data-topic]");
      // A body frame reports a plain click and a keyboard activation alike:
      // a localhost page opens its web card on either, but only while the
      // user is acting (a click in a frame activates this page too), so a
      // body's script cannot open one by posting alone; otherwise its menu
      // offers the card.
      var act = navigator.userActivation;
      if (webLink(a) && act && act.isActive) { window.WsWeb.open({ url: a.href }); return; }
      // In the desktop app a frame's Cmd/Ctrl+click on a file link (mod)
      // previews it, under the same user-activation gate.
      if (m.mod === true && act && act.isActive && desktopPreview(a)) return;
      var r = f.getBoundingClientRect();
      openLinkMenu(a, r.left + num(m.x), r.top + num(m.y));
    } else if (m.wbDoc === "img") {
      relayImage(f, m);
    } else if (m.wbDoc === "sel") {
      if (cchips) frameMention(f, str(m.text), num(m.x), num(m.y));
    } else if (m.wbDoc === "rail") {
      railKey();
    } else if (m.wbDoc === "search") {
      // The palette shortcut pressed in the frame; a body's script posting
      // it alone, with no user key press, opens nothing.
      var sa = navigator.userActivation;
      if (!sa || sa.isActive) tkSearch();
    }
  });
  // A swap re-creates a changed topic's frame (or loads its next one beside
  // the old one, frameGraft), which starts at no height until it posts one;
  // it takes the old frame of the same topic's height meanwhile. A pending
  // next frame's height is not the topic's. A placeholder (mountFrame) keeps
  // and takes a height as its frame does.
  function frameHeights(el) {
    var h = {};
    q(".doc-frame:not(.doc-frame-next)", el).forEach(function (f) { if (f.style.height) h[f.getAttribute("data-doc")] = f.style.height; });
    return h;
  }
  function setFrameHeights(el, h) {
    if (!h) return;
    q(".doc-frame", el).forEach(function (f) {
      var k = f.getAttribute("data-doc");
      if (Object.prototype.hasOwnProperty.call(h, k)) f.style.height = h[k];
    });
  }

