  // Fresh session frames retain only tab-local scroll metadata, never DOM,
  // sockets or terminal output. The new key is independent of draft storage.
  var SESSION_VIEWS = "ws.dashboard.session-views", sessionViewReady = false;
  function sessionViews() {
    try {
      var rows = JSON.parse(window.sessionStorage.getItem(SESSION_VIEWS) || "[]");
      return Array.isArray(rows) ? rows.filter(function (r) { return r && typeof r.page === "string" && r.panes && typeof r.panes === "object"; }).slice(-50) : [];
    } catch (e) { return []; }
  }
  function saveSessionView() {
    if (shell || !sessionViewReady) return;
    var panes = {};
    PANES.forEach(function (id) {
      var el = $(id), top = visible(el) ? el.scrollTop : paneScroll[id];
      if (Number.isFinite(top) && top >= 0) panes[id] = top;
    });
    var row = { page: location.pathname, panes: panes, top: doc.scrollingElement ? doc.scrollingElement.scrollTop : 0, stick: stick };
    var rows = sessionViews().filter(function (r) { return r.page !== row.page; });
    rows.push(row);
    try { window.sessionStorage.setItem(SESSION_VIEWS, JSON.stringify(rows.slice(-50))); } catch (e) { /* view state is optional */ }
  }
  function placeSessionScroll(el, top) {
    if (!el || !Number.isFinite(top) || top < 0) return;
    var w = window.wbHistory;
    if (el.wbScrollOwner || (w && w.keeping && w.keeping(el))) {
      // Board placement stays with its existing owner, including late frame
      // heights. Do not bypass it with a numeric scrollTop write.
      var ref = el.firstElementChild;
      if (!ref || !w || !w.place) return;
      var edge = el === doc.scrollingElement ? 0 : el.getBoundingClientRect().top + el.clientTop;
      w.place(ref, { offset: ref.getBoundingClientRect().top - edge + el.scrollTop - top });
    } else el.scrollTop = top;
  }
  function restoreSessionView() {
    if (shell) return;
    var saved = sessionViews().find(function (r) { return r.page === location.pathname; });
    // Explicit hash navigation outranks an old reading position.
    if (!saved || location.hash) { sessionViewReady = true; return; }
    Object.keys(saved.panes).forEach(function (id) { if (PANES.indexOf(id) >= 0 && Number.isFinite(saved.panes[id])) paneScroll[id] = saved.panes[id]; });
    if (typeof saved.stick === "boolean") stick = saved.stick;
    var cancelled = false;
    function cancel() { cancelled = true; }
    ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (type) { doc.addEventListener(type, cancel, { capture: true, passive: true }); });
    requestAnimationFrame(function () {
      ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (type) { doc.removeEventListener(type, cancel, true); });
      if (!cancelled) {
        PANES.forEach(function (id) { var el = $(id); if (visible(el)) placeSessionScroll(el, saved.panes[id]); });
        placeSessionScroll(doc.scrollingElement, saved.top);
        pinConvo();
      }
      sessionViewReady = true;
    });
  }
  if (!shell) window.addEventListener("pagehide", saveSessionView);
  if (framed) window.wsSaveSessionView = saveSessionView;
