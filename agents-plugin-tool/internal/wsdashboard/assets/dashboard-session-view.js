  // Fresh frames retain only tab-local reading metadata, never DOM/output.
  var SESSION_VIEWS = "ws.dashboard.session-views", sessionViewReady = false;
  function sessionViews() {
    try {
      var rows = JSON.parse(window.sessionStorage.getItem(SESSION_VIEWS) || "[]");
      return Array.isArray(rows) ? rows.filter(function (r) { return r && typeof r.page === "string" && r.panes && typeof r.panes === "object"; }).slice(-50) : [];
    } catch (e) { return []; }
  }
  // The parts content-visibility may skip (dashboard-board.css).
  var BOARD_CV_PARTS = "section.topic[data-topic] > :not(.tophead), .bcard > .bc-in > .bc-body";
  // A board's anchor is its first panel by top whose bottom passes the
  // reading line. A rect or visibility read of a node inside a skipped part
  // forces that part's layout and leaves layout dirty for the next read: on
  // a session switch that was ~120 full layouts (~730 ms) before the next
  // frame could load. So the first pass reads only panels whose parts reach
  // the view. Such a panel starting above the view's bottom beats every
  // panel in a part outside it; only when none does are all panels read.
  function sessionBoardAnchors() {
    var w = window.wbHistory, records = [], seen = new Set();
    if (!w || !w.keep) return records;
    return boardWithSnap(function () {
      boardBoxes().forEach(function (box) {
        if (!visible(box)) return;
        var h = w.keep(box.firstElementChild || box, { capture: false }), s = h.scroller;
        if (seen.has(s)) return;
        seen.add(s);
        var view = h.viewport(), line = view.top + 1, reach = new Map();
        function reaches(n) {
          var part = n.parentElement && n.parentElement.closest(BOARD_CV_PARTS);
          if (!part || !box.contains(part)) return true;
          if (!reach.has(part)) {
            var r = reaches(part) && part.getBoundingClientRect();
            reach.set(part, !!r && r.bottom > line && r.top < view.bottom);
          }
          return reach.get(part);
        }
        function first(admit) {
          return boardLivePanels(box, admit).filter(function (n) { return boardScroller(n) === s; })
            .map(function (n) { return { node: n, rect: n.getBoundingClientRect() }; })
            .filter(function (p) { return p.rect.height > 0 && p.rect.bottom > line; })
            .sort(function (a, b) { return a.rect.top - b.rect.top; })[0];
        }
        var p = first(reaches);
        if (!p || p.rect.top >= view.bottom) p = first();
        var topic = p && p.node.closest(".topic");
        records.push({ scroller: s === doc.scrollingElement ? "document" : s.id, top: s.scrollTop,
          topic: topic && topicKey(topic), key: p && boardPanelKey(p.node), offset: p ? p.rect.top - line : 0 });
      });
      return records;
    });
  }
  function sessionConversationAnchor() {
    if (!convo || !visible(convo)) return null;
    if (atBottom()) return { bottom: true };
    var box = entriesBox(), edge = convo.getBoundingClientRect().top + convo.clientTop;
    var entries = box ? Array.from(box.children) : [];
    var n = entries.find(function (entry) { return entry.getBoundingClientRect().bottom > edge; });
    return n ? { entry: n.getAttribute("data-entry"), offset: n.getBoundingClientRect().top - edge } : null;
  }
  function saveSessionView() {
    if (shell || !sessionViewReady) return;
    var panes = {};
    PANES.forEach(function (id) {
      var el = $(id), top = visible(el) ? el.scrollTop : paneScroll[id];
      if (Number.isFinite(top) && top >= 0) panes[id] = top;
    });
    var row = { page: location.pathname, panes: panes, top: doc.scrollingElement ? doc.scrollingElement.scrollTop : 0,
      board: sessionBoardAnchors(), conversation: sessionConversationAnchor() };
    var rows = sessionViews().filter(function (r) { return r.page !== row.page; });
    rows.push(row);
    try { window.sessionStorage.setItem(SESSION_VIEWS, JSON.stringify(rows.slice(-50))); } catch (e) { /* view state is optional */ }
  }
  function placeSessionScroll(el, top) {
    if (!el || !Number.isFinite(top) || top < 0) return;
    var w = window.wbHistory;
    if (el.wbScrollOwner || (w && w.keeping && w.keeping(el))) {
      // Identity missing: numeric fallback through the owner, not a content
      // anchor. This fallback cannot preserve a reading panel as layout grows.
      var ref = el.firstElementChild;
      if (!ref || !w || !w.place) return;
      var edge = el === doc.scrollingElement ? 0 : el.getBoundingClientRect().top + el.clientTop;
      w.place(ref, { offset: ref.getBoundingClientRect().top - edge + el.scrollTop - top });
    } else el.scrollTop = top;
  }
  function sessionBoardReloadPending() {
    try {
      var r = JSON.parse(window.sessionStorage.getItem(BOARD_RELOAD));
      return !!(r && r.page === location.pathname + location.search && Number.isFinite(r.at) &&
        Date.now() >= r.at && Date.now() - r.at < 60000 && Array.isArray(r.records));
    } catch (e) { return false; }
  }
  function restoreSessionBoard(records) {
    var w = window.wbHistory;
    if (!w || !w.place || !Array.isArray(records)) return;
    records.forEach(function (r) {
      if (!r || !Number.isFinite(r.offset)) return;
      var s = r.scroller === "document" ? doc.scrollingElement : $(r.scroller);
      if (!visible(s)) return;
      var topic = boardBoxes().flatMap(function (box) { return q("section.topic[data-topic]", box); }).find(function (n) { return topicKey(n) === r.topic; });
      var n = topic && boardLivePanels(topic).find(function (p) { return boardPanelKey(p) === r.key; });
      if ((!n || !n.getBoundingClientRect().height) && /^(p|e)[1-9][0-9]*$/.test(r.key || "") && w.loadTarget) {
        (topic ? [topic] : boardBoxes()).some(function (box) { n = w.loadTarget(box, r.key, window.WsCards && window.WsCards.unfoldTopic); return !!n; });
      }
      n = n || topic;
      if (n) w.place(n, { readingOffset: r.offset });
      else placeSessionScroll(s, r.top);
    });
  }
  function restoreSessionConversation(saved) {
    if (!convo) return;
    var box = entriesBox(), n = saved && !saved.bottom && box && Array.from(box.children).find(function (entry) { return entry.getAttribute("data-entry") === saved.entry; });
    if (n && Number.isFinite(saved.offset)) {
      var edge = convo.getBoundingClientRect().top + convo.clientTop;
      convo.scrollTop += n.getBoundingClientRect().top - edge - saved.offset;
      paneScroll.convo = convo.scrollTop;
      stick = false;
    } else {
      // Missing/older window or bottom on leave: retain Phase 1's bottom follow.
      stick = true;
      pinConvo();
    }
  }
  function restoreSessionView() {
    if (shell) return;
    var saved = sessionViews().find(function (r) { return r.page === location.pathname; });
    // Explicit hash navigation outranks an old reading position.
    if (!saved || location.hash) { sessionViewReady = true; return; }
    // Snapshot this before boardRestoreReload's earlier rAF consumes its record.
    var reload = sessionBoardReloadPending(), cancelled = false;
    function cancel() { cancelled = true; }
    ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (type) { doc.addEventListener(type, cancel, { capture: true, passive: true }); });
    requestAnimationFrame(function () {
      ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (type) { doc.removeEventListener(type, cancel, true); });
      if (!cancelled) {
        Object.keys(saved.panes).forEach(function (id) { if (id !== "convo" && PANES.indexOf(id) >= 0 && Number.isFinite(saved.panes[id])) paneScroll[id] = saved.panes[id]; });
        PANES.forEach(function (id) {
          if (id === "convo" || id === "bcol" || id === "bpins") return;
          var el = $(id); if (visible(el)) placeSessionScroll(el, saved.panes[id]);
        });
        if (!reload) {
          restoreSessionBoard(saved.board);
          if (!Array.isArray(saved.board) || !saved.board.some(function (r) { return r && r.scroller === "document"; })) placeSessionScroll(doc.scrollingElement, saved.top);
        }
        restoreSessionConversation(saved.conversation);
      }
      sessionViewReady = true;
    });
  }
  if (!shell) window.addEventListener("pagehide", saveSessionView);
  if (framed) window.wsSaveSessionView = saveSessionView;
