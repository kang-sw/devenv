  // --- region swap -----------------------------------------------------------
  // The mode pseudo-region's hash is unknown to the page until the first
  // event carries it; an empty hash makes the server send it once. A region
  // update held back while the tab is hidden (hiddenRegions, see connect)
  // counts as held: a reconnect meanwhile names its hash, and the update
  // still lands when the tab shows.
  var modeHash = "", hiddenRegions = {}, treeRendered = null, entryRendered = null, panelRendered = null;
  // pageHidden tells whether nobody sees the page (a hidden tab, or a frame
  // the shell hides): region updates are held (hiddenRegions) while it
  // holds, and land when it ends (showRegions).
  function pageHidden() { return doc.hidden || shHidden; }
  function held(withoutBoard) {
    return q("[data-region]").filter(function (el) { return !withoutBoard || el.getAttribute("data-region") !== "board"; }).map(function (el) {
      var name = el.getAttribute("data-region"), p = hiddenRegions[name];
      return name + "." + (p ? p.hash : el.getAttribute("data-h"));
    }).concat(mode !== "index" ? ["mode." + modeHash] : []).join(",");
  }
  // The board's lazy history (wswhiteboard stream.js) fetches the complete
  // sections of the topics a "name=..." or "target=..." query names.
  window.wbHistoryFetch = function (query) {
    if (!rid || !window.fetch) return Promise.resolve([]);
    return fetch("/s/" + rid + "/topic?" + query, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) { return res && Array.isArray(res.topics) ? res.topics : []; });
  };
  // A deferred board swap lands still (no board motion), so the board never
  // moves under a just-blurred input.
  function swap(name, hash, html, still) {
    var el = region(name);
    if (!el) return;
    var board = name === "board";
    if (board && boardReturnTimer) still = true;
    if (el.getAttribute("data-h") === hash && !(board && Object.keys(boardPending).length)) { if (board) { deferred = null; boardReturnApply(); } if (name === "rail") { if (railGate) railGate.forget(); if (typeof html !== "string") railRendered = html; } if (name === "tree" && typeof html !== "string") treeRendered = html; if (name === "entries" && typeof html !== "string") entryRendered = html; if (name === "panels" && typeof html !== "string") panelRendered = html; return; }
    if (board && window.WsOverview && window.WsOverview.holding()) { boardOverviewHeld = { hash: hash, html: html }; return; }
    if (board && editing()) { deferred = { hash: hash, html: html }; return; }
    if (board && window.WsCards && window.WsCards.dragging && window.WsCards.dragging()) { boardDragHeld = { hash: hash, html: html }; return; }
    // A rail render from before the layout change this page saw applied
    // would undo it on screen until the next one: it is dropped, and the
    // render after the change follows.
    if (name === "rail" && WsRail && WsRail.staleRail(html, railMinGen)) return;
    // A rail swap waits from a press on a grip or bar until it ends, while
    // a row's rename is open, and while a layout change posts (railGate),
    // and lands when that ends (railRelease).
    if (name === "rail" && railGate && railGate.offer({ hash: hash, html: html })) return;
    if (name === "rail" && WsRail && typeof html !== "string") {
      var rowTop = railScrollTop(), rowFocus = railFocus();
      var railMerged = WsRail.mergeRows(el, railRendered, html);
      railRendered = html;
      if (railMerged !== null) {
        railHoverUpdates = null;
        el.setAttribute("data-h", hash);
        if (railMerged) { fmtTimes(el); railMarks(); railScrollTo(rowTop); railRefocus(rowFocus); apScan(el); }
        return;
      }
      html = WsRail.rowInputHTML(html);
      railHoverUpdates = null;
    }
    if (name === "rail" && railHoverUpdates && railHoverUpdates(html)) {
      el.setAttribute("data-h", hash);
      return;
    }
    if (name === "head") {
      if (labelEditing()) { headHeld = { hash: hash, html: html }; return; }
      headHeld = null;
    }
    if (board) {
      deferred = null;
      var canonical = typeof html === "string" ? boardInputFull(html) : html;
      boardOffered = !boardLatest || boardLatest.hash !== hash ? { hash: hash, previous: boardLatest ? boardLatest.html : boardRendered, input: canonical } : null;
      boardLatest = { hash: hash, html: canonical };
      html = boardStageInput(boardLatest.html || html);
    }
    var pin = (name === "entries" || name === "convlive") && pinned();
    var scroll = scrollState();
    var treeFocus = name === "tree" ? doc.activeElement : null;
    var panelReader = name === "panels" ? panelReaderState(el) : null;
    var answers = board ? boardRead(readAnswers) : null;
    if (name === "rail") wsAdmissionCapture();
    var railTop = name === "rail" ? railScrollTop() : null, railFoc = name === "rail" ? railFocus() : null;
    var frameH = board ? boardRead(frameHeights) : null;
    var history = board ? captureBoardHistory() : null;
    // Growth/FLIP cannot run through a frozen visible interval: their
    // intermediate layout would escape the synchronous owner transaction.
    var before = board && !boardFrozen.length && !still && !reducedMotion() ? boardSpots(el) : null;
    var kept = board ? boardKeep() : null, boardTop = before && kept ? el.getBoundingClientRect().top : null;
    // The entries merge and the board morph prepare only the nodes they
    // insert; tree and panel row merges prepare only changed units too.
    // A finished reply's mermaid
    // fences draw once merged; the streaming reply (convlive) keeps them
    // code blocks.
    var ins = null;
    if (name === "entries") {
      var entryNext = typeof html === "string" ? entryInputFull(html) : html;
      mergeEntriesRegion(el, entryNext || html, pin, entryRendered);
      entryRendered = entryNext;
    }
    else if (name === "tree") {
      var treeNext = typeof html === "string" ? treeInputFull(html) : html;
      ins = treeNext && mergeTreeRows(el, treeRendered, treeNext);
      treeRendered = treeNext;
      if (!ins) {
        el.innerHTML = treeNext ? treeInputHTML(treeNext) : html;
        restoreOpen(el); fmtTimes(el); sweepOutImgs(el);
      } else ins.forEach(function (n) { restoreOpen(n); fmtTimes(n); sweepOutImgs(n); });
    }
    else if (name === "panels") {
      var panelNext = typeof html === "string" ? panelInputFull(html) : html;
      ins = panelNext && mergePanelRows(el, panelRendered, panelNext);
      panelRendered = panelNext;
      if (!ins) {
        el.innerHTML = panelNext ? panelInputHTML(panelNext) : html;
        restoreOpen(el); fmtTimes(el); sweepOutImgs(el);
      } else ins.forEach(function (n) { restoreOpen(n); fmtTimes(n); sweepOutImgs(n); });
    }
    else if (board) {
      ins = morphBoard(el, html, boardFrozen, boardPagedEdits);
      boardPagedEdits = [];
      if (window.wbHistory && window.wbHistory.sweep) window.wbHistory.sweep();
      ins.forEach(bandBoard);
      ins.forEach(function (n) {
        // Materialize the reader's older pages before restoring disclosures
        // and frame heights, including immutable panel versions off the tail.
        if (window.wbHistory) { window.wbHistory.init(n); window.wbHistory.restore(n, history); }
        watchBoard(n);
        restoreOpen(n);
        fmtTimes(n);
        sweepOutImgs(n);
        setFrameHeights(n, frameH);
        mountFrames(n);
        writeAnswers(n, answers);
        // A placeholder a swap brought (the viewport bands mount it later)
        // takes the mark too, and its frame takes it once mounted.
        q(".doc-frame", n).forEach(function (f) { f.wbKeep = true; });
      });
      // The morph retains unchanged topic nodes, including hydrated closed
      // pages. Their image frames still follow the board's external-file stamp.
      if (window.wbHistory && window.wbHistory.refreshImages) window.wbHistory.refreshImages(doc);
      if (before) {
        var moves = topicMotion(before, ins).map(function (m) { return growTopic(m.n, m.from); });
        startMotion(moves.concat(slideBoard(el, before, boardTop)));
        ins.forEach(function (n) { q("iframe.doc-frame", n).forEach(function (f) { if (!f.wbOut) f.wbGrow = true; }); });
      }
    } else {
      el.innerHTML = html;
      if (name === "rail" && WsRail) railHoverUpdates = WsRail.hoverUpdates(el);
      restoreOpen(el);
      fmtTimes(el);
      sweepOutImgs(el);
    }
    el.setAttribute("data-h", hash);
    if (board) {
      indexFrames();
      if (window.wbForm) window.wbForm();
      bindCapsuleNotes();
      mermaid(ins);
      syncChips();
      // The notes came back with their markers (writeAnswers); their image
      // rows go back after them, and a note the swap took drops its images.
      if (ctext) { syncMarkers(); renderImgs(); }
      // An input-held selection is already a value snapshot, even when its
      // source frame leaves the board during an update.
      if (mpend && !mpend.locked && mpend.frame && !doc.contains(mpend.frame)) hideMention();
    }
    if (board || name === "convlive") applySent(board);
    if (name === "convlive" || name === "entries") outboxSync();
    if (name === "panels") { drawerSync(); restorePanelReader(el, panelReader); }
    if (name === "git") gitSwapped();
    if (treeFocus && treeFocus.isConnected && doc.activeElement !== treeFocus) treeFocus.focus({ preventScroll: true });
    restoreScroll(scroll);
    if (kept) kept.h.apply();
    if (board) {
      // An owner correction that clamps at home is not a hand scroll and
      // must not acknowledge the update that caused that correction.
      scroll.forEach(function (p) { if ((p[0] === $('bcol') || p[0] === doc.scrollingElement) && p[1] > 4 && p[0].scrollTop <= 4) boardHomeWrites.add(p[0]); });
      boardRecordPending(); boardPendingBadge(); boardHoldIndicators();
      boardExposedUnits = Object.keys(boardPending).length ? boardWithSnap(boardExposureUnits) : null;
      if (!boardRelease) boardReturnApply();
    }
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
    if (live && TAB[name] === "convo" && !visible(convo) && (visible(fpane) || visible($("cterm-panes")) || (grid && grid.getAttribute("data-ctab") !== "convo")) && ctabDot()) ctabDot().classList.add("on");
    if (name === "panels") drawerSwapped();
    if (name === "tree" || name === "convlive") drawerWait();
    if (name === "tree") agSwapped();
    if (TL_REGIONS[name]) { delete tlHeld[name]; tlCount(); }
    if (name === "convlive") syncBadge();
    // A list that arrives while a / waits in the box opens the picker.
    if (name === "cmds" && slash) slash.sync();
    if (name === "entries") syncChead();
    if (name === "head" && !sseOn) connected(false);
    if (name === "head") labelSwapped();
    if (name === "rail") { railMarks(); railScrollTo(railTop); railRefocus(railFoc); }
    if (name === "head" || name === "convlive") leadSwapped(name);
    if (name === "rail" || name === "sessions") apScan(el);
  }

  // --- board morph: unchanged topics keep their nodes -------------------------
  // Each topic section carries data-topic, its name. A board swap keeps a
  // topic's section, and with it the body frame (no reload, no second
  // mermaid load, no second image relay), when the section's incoming HTML
  // equals the HTML it last arrived with (boardSeen). The comparison is
  // never against the live DOM, which the page changes after a swap (times,
  // sent marks, answers, frame themes and heights). Everything else the
  // board holds (the closed-topic control, the reply bar) is replaced. The
  // initial record is the board as served, read before anything changes it.
  // The server stamps each section with data-th, a hash of its rendered
  // bytes (TopicHTML, render.go), compared in place of the HTML (topicSeen).
  var boardSeen = {}, boardRendered = null, boardLatest = null, boardPending = Object.create(null), boardFrozen = [], boardRelease = false;
  var boardOwn = Object.create(null), boardImmediate = Object.create(null), boardSnap = null, boardExposedUnits = null;
  var boardChanges = [], boardOffered = null, boardReleaseFrame = 0, boardScrollTimer = 0, boardHomeWrites = new WeakSet();
  var boardPaged = Object.create(null), boardPagedEdits = [];
  var boardTopHeld = Object.create(null), boardTopOrder = null;
  var boardReturnTimer = 0, boardReturnFresh = false;
  function topicKey(n) {
    return n.nodeType === 1 && n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic") ? n.getAttribute("data-topic") : null;
  }
  function topicSeen(n) {
    return n.getAttribute("data-th") || n.outerHTML;
  }
  (function () {
    var b = region("board");
    if (b) {
      boardRendered = boardInputFull(b.innerHTML);
      boardLatest = { hash: b.getAttribute("data-h"), html: boardRendered };
      Array.prototype.forEach.call(b.children, function (n) { var k = topicKey(n); if (k !== null) boardSeen["t:" + k] = topicSeen(n); });
    }
  })();
  // --- board cards (cards.js): the page's own cards among the topics --------
  // The framework mounts on the board region once, before any board swap,
  // and restores the cards and the pinned or hand-moved topics this browser
  // kept for the session. Showing a card switches to the board tab where
  // the view shows one column at a time.
  // Terminal cards (terminal.js) exist only on a page the server marked
  // data-term; their module adds the "+ Terminal" button at the end of
  // the composer's status line (or fixed at the page's corner without one).
  // Web cards (webcard.js) exist only on a data-sysopen page; their "+ Web"
  // button stands beside it, and their picks and console errors go to the
  // composer's chips (web picks, below).
  // The paged docking gallery (cards.js #bgal, joining the page body) has
  // its shape from WsGallery.layout (gallery.js) over this page's width
  // and layout menu (mode), and the page maps it (layout): from 1600px
  // (not "tabs"), the session rail stays permanently docked left while the
  // 4-column gallery (empty slot, drag slot, or data-pincol column) opens
  // only when the right-side spare width fits at least the board's width;
  // otherwise (or below 1600px / in tabs), the Main Board is Page 0 and
  // the gallery overlays the board's column (data-gal="narrow", placed by
  // placeGallery). On the live grid the split handle sizes the
  // conversation's column beside the board (split).
  // A board swap waits while a card or topic is dragged (boardDragHeld), and
  // lands at the drag's end.
  var filePreview = null, filePreviewBack = null;
  function splitFiles() {
    return live && !worktree && layoutMode !== "tabs" && window.matchMedia(
      layoutMode === "split" ? "(min-width: 760px)" : "(min-width: 1100px)"
    ).matches;
  }
  function sizeFilePreview() {
    if (!filePreview || !filePreview.current()) return;
    var board = $("bcol").getBoundingClientRect();
    // The closed board's .dwrap is capped at 1680px without a pin column.
    // Reading space reaches the viewport gutter even while that cap remains.
    var right = doc.documentElement.clientWidth - parseFloat(window.getComputedStyle(grid.closest(".dwrap")).paddingRight);
    filePreview.el.style.width = Math.max(0, right - board.left) + "px";
  }
  function sizeFilesSheet() {
    if (!filesSheet || filesSheet.el.hidden) return;
    var sheet = filesSheet.el;
    if (!splitFiles()) { sheet.removeAttribute("data-docked"); return; }
    // The left column includes its header and composer, not just #convo.
    var column = chead.getBoundingClientRect(), bounds = grid.getBoundingClientRect();
    sheet.setAttribute("data-docked", "");
    sheet.style.setProperty("--files-dock-left", column.left + "px");
    sheet.style.setProperty("--files-dock-top", bounds.top + "px");
    sheet.style.setProperty("--files-dock-width", column.width + "px");
    sheet.style.setProperty("--files-dock-height", bounds.height + "px");
  }
  function sizeFileSurfaces() { sizeFilesSheet(); sizeFilePreview(); }
  function syncFilePreview() {
    sizeFilesSheet();
    if (splitFiles()) {
      sizeFilePreview();
    } else if (filePreview && filePreview.current()) {
      var previous = filePreview.close();
      window.WsPreview.open(previous.path, Object.assign({}, previous.options, { keep: false }));
    }
  }
  function previewLeftFile(path, options) {
    if (splitFiles() && filePreview && !options.keep) {
      filePreviewBack = doc.activeElement;
      root.setAttribute("data-file-preview", "");
      filePreview.open(path, options);
      sizeFilePreview();
    } else {
      if (filePreview) filePreview.close();
      window.WsPreview.open(path, options);
    }
  }
  // Two owners, not two views onto one explorer. Keep the tab's existing
  // storage keys; the toolbar uses the same records in its own namespace.
  if ((live || worktree) && fpane && window.WsFiles) {
    var fileHooks = {
      rid: cardIdentity, post: worktree ? browsePost : post, toast: toast, copy: copyText,
      routeBase: worktree ? "/s/host0/" : undefined,
      location: worktree ? { root: worktree, dir: "" } : undefined,
      load: function (k) { return load(worktree && k === "ws.dashboard.files.view" ? k + "." + cardIdentity : k); },
      save: function (k, v) { save(worktree && k === "ws.dashboard.files.view" ? k + "." + cardIdentity : k, v); },
      local: function () { return root.hasAttribute("data-sysopen"); },
      send: worktree ? null : attachExplorerPath,
      openCard: function (location) {
        // Dismiss before constructing/revealing the new card, not afterward.
        window.WsCards.dismissFiles();
        window.WsFiles.open(location);
      }
    };
    // Register adapters before board mount restores explorer card IDs.
    window.WsFiles.setup(fileHooks);
    function createFiles(node, rootSlot, toolbar) {
      return window.WsFiles.create(node, Object.assign({}, fileHooks, {
        rootSlot: rootSlot,
        preview: live && !worktree ? previewLeftFile : undefined,
        load: toolbar ? function (k) { return load("ws.dashboard.toolbar." + k); } : fileHooks.load,
        save: toolbar ? function (k, v) { save("ws.dashboard.toolbar." + k, v); } : fileHooks.save
      }));
    }
    filesTab = createFiles(fpane, chead && chead.querySelector(".fsum"), false);
    var sheet = doc.createElement("section");
    sheet.id = "toolbar-files";
    sheet.className = "fpane";
    sheet.hidden = true;
    sheet.setAttribute("aria-label", fpane.getAttribute("aria-label"));
    doc.body.appendChild(sheet);
    var sheetInstance = null;
    filesSheet = {
      el: sheet,
      shown: function () {
        sizeFilesSheet();
        if (!sheetInstance) sheetInstance = createFiles(sheet, null, true);
        sheetInstance.shown();
      },
      hidden: function () {
        if (sheetInstance) { sheetInstance.cancelPreview(); sheetInstance.hidden(); }
        if (sheet.hasAttribute("data-docked")) {
          // A left preflight may still be pending before any pane exists.
          if (filesTab) filesTab.cancelPreview();
          if (filePreview) filePreview.close();
        }
        sheet.removeAttribute("data-docked");
      }
    };
    // Capture before explorer menus/inputs or pane chrome consume Escape.
    // Sandboxed preview documents intentionally remain outside this handler.
    doc.addEventListener("keydown", function (e) {
      if (e.key !== "Escape" || sheet.hidden || !sheet.hasAttribute("data-docked")) return;
      e.preventDefault(); e.stopPropagation();
      window.WsCards.dismissFiles();
    }, true);
  }
  var cardBoard = mode !== "index" && window.WsCards ? region("board") : null;
  var PIN_WIDE = window.matchMedia ? window.matchMedia("(min-width: 1600px)") : null, boardDragHeld = null, boardOverviewHeld = null;
  function afterOverview(fn) { if (window.WsOverview) window.WsOverview.after(fn); else fn(); }
  // galMode is what the gallery's layout reads: the viewport width and
  // whether the layout menu shows one column at a time.
  function galMode() {
    return { width: window.innerWidth || 0,
      tabs: !!(live && (grid.getAttribute("data-layout") || load(LAYOUT_KEY)) === "tabs") };
  }
  // placeGallery lays the narrow view's gallery over the board's column:
  // the column's box, cut to the window. A hidden column (another tab)
  // keeps the last box and moves the gallery off the window instead, so
  // its panes keep their sizes and scroll offsets.
  var galFrame = 0, galNarrow = false;
  function placeGallery() {
    galFrame = 0;
    var g = window.WsCards && window.WsCards.gallery && window.WsCards.gallery(), col = $("bcol") || cardBoard;
    if (!g || !galNarrow || !col) return;
    var r = col.getBoundingClientRect(), top = Math.max(r.top, 0), bottom = Math.min(r.bottom, window.innerHeight);
    if (!(r.width > 0) || bottom <= top) { g.setAttribute("data-away", ""); return; }
    g.removeAttribute("data-away");
    g.style.setProperty("--gal-left", r.left + "px");
    g.style.setProperty("--gal-top", top + "px");
    g.style.setProperty("--gal-width", r.width + "px");
    g.style.setProperty("--gal-height", (bottom - top) + "px");
  }
  function queueGallery() { if (galNarrow && !galFrame) galFrame = requestAnimationFrame(placeGallery); }
  if (cardBoard) {
    if (window.WsPreview) window.WsPreview.setup({ renderDiff: gwRender, post: worktree ? browsePost : post, toast: toast });
    if (window.WsTerm) window.WsTerm.setup({ post: worktree ? browsePost : post, toast: toast, setCtab: live ? setCtab : null });
    // The composer's chips come later in this script: the hooks are hoisted
    // functions, and cchips is not yet assigned here.
    if (window.WsWeb) window.WsWeb.setup($("cchips") ? { post: post, toast: toast, rid: rid, addPick: addWebPick, addConsole: addWebConsole } : { post: post, toast: toast, rid: rid });
    var dbody = cardBoard.closest(".dbody");
    if (live && !worktree && window.WsPreview) {
      var previewPane = doc.createElement("section");
      previewPane.className = "file-preview-pane";
      previewPane.setAttribute("aria-label", fpane.getAttribute("aria-label"));
      grid.appendChild(previewPane);
      filePreview = window.WsPreview.createPane(previewPane, function (hadFocus) {
        root.removeAttribute("data-file-preview");
        if (filesTab) filesTab.cancelPreview();
        if (sheetInstance) sheetInstance.cancelPreview();
        if (hadFocus && filePreviewBack && filePreviewBack.isConnected) filePreviewBack.focus({ preventScroll: true });
      });
      filePreview.el = previewPane;
      if (window.ResizeObserver) {
        var previewResize = new ResizeObserver(sizeFileSurfaces);
        previewResize.observe(grid);
        if (chead) previewResize.observe(chead);
        if (dbody) previewResize.observe(dbody);
      }
    }
    window.WsCards.mount(cardBoard, {
      key: "ws.dashboard.cards." + cardIdentity, load: load, save: save,
      toolbar: {
        overview: function () { if (window.WsOverview) window.WsOverview.toggle(); },
        filter: function (run) { if (window.WsOverview) window.WsOverview.filter(run); else run(); },
        download: worktree ? null : function () {
          var a = doc.createElement("a");
          a.href = "/s/" + rid + "/board.zip"; a.download = "";
          doc.body.appendChild(a); a.click(); a.remove();
        },
        files: worktree ? null : filesSheet
      },
      reveal: function () {
        // Linked/durable cards must not remain behind either explorer surface.
        if (filesSheet && filesSheet.el.hasAttribute("data-docked")) window.WsCards.dismissFiles();
        if (filePreview) filePreview.close();
        if (live && !tabShown("board")) setTab("board");
      },
      host: dbody || doc.body, mode: galMode, split: grid && grid.classList.contains("live") ? grid : null,
      leading: function (c) { return worktree && c.preview && !(PIN_WIDE && PIN_WIDE.matches); },
      layout: function (s) {
        var col = s.column && s.pages > 0, mode = s.narrow ? (s.rail ? "narrow" : null) : s.column ? (col ? "col" : "drag") : s.empty ? "empty" : null;
        if (col !== root.hasAttribute("data-pincol")) {
          if (col) root.setAttribute("data-pincol", ""); else root.removeAttribute("data-pincol");
          railOpen(false);
        }
        if (mode) root.setAttribute("data-gal", mode); else root.removeAttribute("data-gal");
        galNarrow = mode === "narrow";
        if (galNarrow) placeGallery();
      },
      closed: function (from, done) {
        if (!window.fetch) { done(null); return; }
        fetch("/s/" + rid + "/closed?from=" + from, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
          if (!r.ok) throw new Error("HTTP " + r.status);
          return r.json();
        }).then(function (res) { done(frag(res.html || "").firstElementChild); }, function () { done(null); });
      },
      adopted: function (n) {
        if (window.wbHistory) window.wbHistory.init(n);
        watchBoard(n);
        bandBoard(n);
        restoreOpen(n);
        mountFrames(n);
        if (window.wbBindBoard) window.wbBindBoard(n);
        if (window.wbAgo) window.wbAgo(n);
        mermaid([n]);
      },
      drag: function (on) {
        if (on) return;
        var h = boardDragHeld;
        boardDragHeld = null;
        if (h) swap("board", h.hash, h.html, true);
      }
    });
    if (worktree) {
      var tools = window.WsCards.toolbar().querySelector(".board-tools"), explorer = doc.createElement("button");
      q(".board-overview, .board-state-filter", tools).forEach(function (n) { n.remove(); });
      explorer.type = "button"; explorer.className = "board-action board-explorer";
      explorer.innerHTML = window.WsCards.icon("folder-open");
      var label = doc.createElement("span"); label.className = "board-label";
      label.textContent = wsT("js.worktree.explorer"); explorer.appendChild(label);
      explorer.addEventListener("click", function () { window.WsFiles.open({ root: worktree, dir: "" }); });
      tools.appendChild(explorer);
    }
    // wbForm creates wbHistory near the end of page startup. Mount overview
    // after that, not here while only the card framework exists.
    function mountBoardOverview() {
      if (worktree || !window.WsOverview || !window.wbHistory) return;
      window.WsOverview.mount({
      rid: rid, board: cardBoard, toolbar: window.WsCards.toolbar(),
      keep: window.wbHistory.place, place: window.WsCards.overview, toast: toast, rail: railKey,
      reveal: function () { if (live && !tabShown("board")) setTab("board"); },
      frames: function () { return boardRead(frameHeights); },
      restoreFrames: function (h) { var kept = boardKeep(); boardBoxes().forEach(function (b) { setFrameHeights(b, h); }); if (kept) kept.h.apply(); },
      changed: function () { doc.dispatchEvent(new Event("ws-overview")); },
      release: function () {
        var h = boardOverviewHeld;
        boardOverviewHeld = null;
        if (h) swap("board", h.hash, h.html, true);
      }
      });
    }
    if (window.WsTerm) window.WsTerm.mount($("bcol"));
    if (window.WsWeb) window.WsWeb.mount($("bcol"));
    if (PIN_WIDE && PIN_WIDE.addEventListener) PIN_WIDE.addEventListener("change", function () { window.WsCards.relayout(); });
    doc.addEventListener("scroll", queueGallery, { capture: true, passive: true });
    window.addEventListener("resize", queueGallery);
    if (window.ResizeObserver) new ResizeObserver(queueGallery).observe($("bcol") || cardBoard);
  }
  boardBoxes().forEach(watchBoard);
  // morphBoard applies a board region event to el and returns the element
  // nodes it inserted, in order, for the caller to prepare.
  function morphBoard(el, html, frozen, pagedEdits) {
    var seen = {}, want = [], ins = [], live = {}, liveClosed = [], pages = [], more = null, grafts = [], changed = [];
    var docked = window.WsCards.docked();
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n);
      if (k !== null) { live["t:" + k] = n; if (n.classList.contains("closed")) liveClosed.push(n); }
      else if (n.classList.contains("closed-page")) pages.push(n);
    });
    // The docked topics (cards.js) stand in the gallery's panes, on every
    // page. A closed one the last board showed counts as the board's own
    // closed topic for the pages below (pinShift).
    var pinShift = [];
    docked.forEach(function (n) {
      var k = topicKey(n);
      if (k === null) return;
      live["t:" + k] = n;
      if (n.classList.contains("closed") && boardSeen["t:" + k] !== undefined) pinShift.push(n);
    });
    boardNodes(html, live).forEach(function (n) {
      var k = topicKey(n);
      if (k === null) {
        want.push(n);
        if (n.nodeType !== 1) return;
        ins.push(n);
        if (n.matches("button.closed-more[data-closed-next]")) more = n;
        return;
      }
      var h = topicSeen(n), key = "t:" + k;
      seen[key] = h;
      // A topic that closes while the page shows it folds once: the
      // server folds a closed topic's body and settled rows under the same
      // keys an open topic uses, so the user's earlier choices for them are
      // forgotten here, and later toggles are kept again.
      if (live[key] && n.classList.contains("closed") && !live[key].classList.contains("closed")) { forgetOpen(live[key]); forgetOpen(n); }
      if (live[key] && boardSeen[key] === h) { want.push(live[key]); delete live[key]; }
      else {
        // A docked topic's live section wraps its body (cards.js wrap): the
        // incoming one takes the same wrapper first, so the graft's paths
        // match and its body frame stays live.
        if (live[key] && window.WsCards.isDocked(live[key])) window.WsCards.wrap(n);
        var g = live[key] && frameGraftable(live[key], n);
        if (g) grafts.push(g);
        if (live[key]) changed.push({ old: live[key], n: n, g: g });
        want.push(n); ins.push(n);
      }
    });
    boardSeen = seen;
    // Closed topics fetched past the cap (closed pages, each nested in the
    // one before it) stay across swaps in the new control's place, since the
    // board renders only its capped section again. Closed-topic indices
    // follow close order, so a topic that newly closes pushes the oldest
    // capped one past the cap: that one moves to the head of the pages, a
    // fetched topic the new board shows itself leaves them, and the pages'
    // own control counts on from what is shown. No topic shows twice, and
    // none drops out between the capped section and the pages. A board with
    // no control holds every closed topic, and the pages go. A docked topic
    // (cards.js) stays in the gallery: in the pages a slot (closed-slot)
    // stands in its place and counts as it would.
    if (pages.length && more) {
      var shown = {}, shifted = liveClosed.filter(function (n) { return !seen["t:" + topicKey(n)]; }), count = shifted.length;
      var slots = pinShift.filter(function (n) { return !seen["t:" + topicKey(n)]; });
      count += slots.length;
      Object.keys(seen).forEach(function (key) { shown[key] = true; });
      shifted.concat(slots).forEach(function (n) { shown["t:" + topicKey(n)] = true; });
      pages.forEach(function (p) {
        q(".closed-page > section.topic[data-topic], .closed-page > .closed-slot[data-slot]", p).forEach(function (t) {
          var key = "t:" + (t.getAttribute("data-topic") || t.getAttribute("data-slot"));
          if (shown[key]) t.parentNode.removeChild(t);
          else { shown[key] = true; count++; }
        });
      });
      var anchor = pages[0].firstChild;
      shifted.forEach(function (n) { pages[0].insertBefore(n, anchor); });
      slots.forEach(function (n) {
        var s = doc.createElement("span");
        s.className = "closed-slot";
        s.hidden = true;
        s.setAttribute("data-slot", topicKey(n));
        pages[0].insertBefore(s, anchor);
      });
      var tail = q("button.closed-more[data-closed-next]", pages[pages.length - 1]).pop();
      if (tail) {
        tail.setAttribute("data-closed-next", String(+more.getAttribute("data-closed-next") + count));
        tail.setAttribute("data-closed-more", String(Math.max(0, (+more.getAttribute("data-closed-more") || 0) - count)));
      }
      want.splice.apply(want, [want.indexOf(more), 1].concat(pages));
      ins.splice(ins.indexOf(more), 1);
    }
    // The page's own cards (cards.js) stand among the topics: they are
    // woven into the wanted nodes at their places, and the board settles
    // around them. Stale nodes go first, so the kept ones that stay in
    // order are never moved (moving a frame reloads it), and a card is
    // never moved at all. A changed topic whose old section stays in place
    // keeps that section and its body frame (frameGraft); where the browser
    // has Element.moveBefore, a kept node moves without reloading its
    // frames, so every changed topic keeps its old section (frameGrafts).
    // The docked topics go to their panes, where an update moves none of
    // them, so a changed one whose old section stands there keeps it, the
    // wrapper (.gal-pbody) its graft target.
    var plan = window.WsCards.weave(want);
    (frozen || []).forEach(function (f) {
      var list = f.box === el ? plan.board : plan.pins;
      var staged = boardStageOrder(Array.prototype.slice.call(f.box.childNodes), list, f.nodes);
      if (f.box === el) plan.board = staged; else plan.pins = staged;
    });
    var woven = plan.board;
    // A changed topic's unchanged entry frames are kept, moved aside before
    // the grafts and the settle would detach them (frameKeep), and moved
    // into their placeholders' places once the board has settled (frameKept).
    var reuse = frameKeep(changed);
    var pinGrafts = grafts.filter(function (g) { return plan.pins.indexOf(g.n) >= 0 && window.WsCards.isDocked(g.old); });
    frameGrafts(el, woven, grafts.filter(function (g) { return plan.pins.indexOf(g.n) < 0; })).concat(pinGrafts).forEach(function (g) {
      frameGraft(g);
      var list = woven.indexOf(g.n) >= 0 ? woven : plan.pins;
      list[list.indexOf(g.n)] = g.old;
      ins[ins.indexOf(g.n)] = g.old;
    });
    window.WsCards.settle(el, plan);
    frameKept(reuse);
    (pagedEdits || []).forEach(function (edit) {
      var old = edit.live, n = edit.source.cloneNode(true), g = frameGraftable(old, n);
      var keptFrames = frameKeep([{ old: old, n: n, g: g }]);
      if (g) { frameGraft(g); n = old; }
      else old.parentNode.replaceChild(n, old);
      frameKept(keptFrames);
      ins.push(n);
    });
    return ins;
  }
  // --- incremental board inputs ---------------------------------------------
  // These are immutable incoming snapshots, not the live DOM. Hidden pages,
  // editing, overview and dragging may defer several events; the newest input
  // must include all earlier changes even though none has reached morphBoard.
  // Canonical nodes are parsed once, then cloned only for changed sections.
  function boardInputFull(html) {
    var nodes = Array.prototype.slice.call(frag(html).childNodes), topics = Object.create(null), order = [], before = [], after = [], past = false;
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i], key = topicKey(n);
      if (key === null) { (order.length ? after : before).push(n); if (order.length && n.nodeType === 1) past = true; continue; }
      if (past || !n.getAttribute("data-th") || Object.prototype.hasOwnProperty.call(topics, key)) return null;
      topics[key] = n; order.push(key);
    }
    return { topics: topics, order: order, before: before, after: after };
  }
  function boardInputPatch(prev, ev) {
    if (!prev || !ev || typeof ev.hash !== "string" || !Array.isArray(ev.order) || !Array.isArray(ev.topics)) return null;
    var topics = Object.create(null), changes = Object.create(null);
    for (var i = 0; i < ev.topics.length; i++) {
      var t = ev.topics[i];
      if (!t || typeof t.key !== "string" || typeof t.hash !== "string" || typeof t.html !== "string" || Object.prototype.hasOwnProperty.call(changes, t.key)) return null;
      var parsed = frag(t.html), n = parsed.firstElementChild;
      if (!n || parsed.childNodes.length !== 1 || topicKey(n) !== t.key || n.getAttribute("data-th") !== t.hash) return null;
      changes[t.key] = n;
    }
    for (var j = 0; j < ev.order.length; j++) {
      var key = ev.order[j];
      if (typeof key !== "string" || Object.prototype.hasOwnProperty.call(topics, key)) return null;
      var node = changes[key] || prev.topics[key];
      if (!node) return null;
      topics[key] = node;
    }
    if (Object.keys(changes).some(function (key) { return !Object.prototype.hasOwnProperty.call(topics, key); })) return null;
    return { topics: topics, order: ev.order.slice(), before: prev.before, after: prev.after };
  }
  function boardNodes(input, live) {
    if (typeof input === "string") return Array.prototype.slice.call(frag(input).childNodes);
    function clone(n) { return n.cloneNode(true); }
    return input.before.map(clone).concat(input.order.map(function (key) {
      var n = input.topics[key], old = live && live["t:" + key];
      return old && boardSeen["t:" + key] === topicSeen(n) ? old : clone(n);
    }), input.after.map(clone));
  }
  function boardInputFragment(input) {
    var f = doc.createDocumentFragment();
    boardNodes(input).forEach(function (n) { f.appendChild(n); });
    return f;
  }
  // --- keyed frame reuse: a changed topic keeps its unchanged entry frames --
  // An entry frame (data-doc-key entry:eN) is immutable per entry, so inside
  // a changed topic an incoming entry placeholder takes the old topic's
  // mounted frame of the same data-doc-key and the same data-srcdoc (the
  // same key need not mean the same document: image inlining and relay
  // lists change it); each old frame pairs once, so duplicate keys pair off
  // one by one. Only Element.moveBefore keeps a frame's document across a
  // move, and only between connected nodes: frameKeep moves the paired old
  // frames into a hidden holder on the page before anything detaches them,
  // and frameKept moves each into its placeholder's place, the placeholder
  // dropped, once the new section stands on the page. A placeholder that
  // stays unpaired is mounted as usual (mountFrames). A frame a graft holds
  // (frameGraft's chain) and the topic-body frame (topic:, a stable key over
  // changing content) are never paired. Without moveBefore nothing is kept.
  function frameKeep(changed) {
    if (!changed.length || !doc.body || !doc.body.moveBefore) return null;
    var pairs = [];
    changed.forEach(function (c) {
      // Timeline anchors can become inert gaps after an append. Replay only
      // already-admitted rows before looking for their frame placeholders;
      // the normal post-morph restore still owns folds, mounting and layout.
      if (window.wbHistory) window.wbHistory.prepare(c.n, window.wbHistory.capture(c.old));
      var held = c.g ? c.g.oc[0] : null, olds = {};
      q('iframe.doc-frame[data-doc-key^="entry:"]:not(.doc-frame-next)', c.old).forEach(function (f) {
        if (f === held || !f.hasAttribute("data-srcdoc")) return;
        var k = f.getAttribute("data-doc-key") + "\n" + f.getAttribute("data-srcdoc");
        (olds[k] = olds[k] || []).push(f);
      });
      q('div.doc-frame[data-doc-key^="entry:"]', c.n).forEach(function (p) {
        if (c.g && p === c.g.nc[0]) return;
        var list = olds[p.getAttribute("data-doc-key") + "\n" + p.getAttribute("data-srcdoc")];
        if (list && list.length) pairs.push({ f: list.shift(), p: p });
      });
    });
    if (!pairs.length) return null;
    var holder = doc.createElement("div");
    holder.hidden = true;
    doc.body.appendChild(holder);
    pairs.forEach(function (x) { holder.moveBefore(x.f, null); });
    return { holder: holder, pairs: pairs };
  }
  function frameKept(r) {
    if (!r) return;
    r.pairs.forEach(function (x) {
      var at = x.p.parentNode;
      if (!at || !x.p.isConnected) return;
      at.moveBefore(x.f, x.p);
      at.removeChild(x.p);
    });
    r.holder.parentNode.removeChild(r.holder);
  }
  // --- a changed topic's body frame, double-buffered --------------------------
  // A changed topic's new section would bring a new body frame, blank until
  // its document loads and reports a height. Instead, when the topic stays
  // open and its old section would stay where it stands, or the browser
  // can move it without reloading its frame (frameGrafts), the old section
  // is kept: its content becomes the new section's, except the
  // path down to the old body frame, and the new frame loads beside the old
  // one, unseen and out of the layout (.doc-frame-next), at the old one's
  // height. Once the new frame has loaded and drawn, it takes the old one's
  // place in the same frame (frameShow, frameReveal); one that never loads
  // shows after frameRevealMs anyway.
  var frameRevealMs = 1500;
  // frameChain is the path from frame f up to section sec, f first, or null.
  function frameChain(f, sec) {
    var c = [];
    for (var x = f; x && x !== sec; x = x.parentNode) c.push(x);
    return x === sec ? c.concat(sec) : null;
  }
  // frameGraftable returns the graft of new section n into old section old
  // (both open, each with a body frame of the same topic document on paths
  // of the same shape), or null.
  function frameGraftable(old, n) {
    if (old.classList.contains("closed") || n.classList.contains("closed")) return null;
    var of = q(".doc-frame:not(.doc-frame-next)", old)[0], nf = q(".doc-frame", n)[0];
    if (!of || !nf || of.getAttribute("data-doc") !== nf.getAttribute("data-doc")) return null;
    // A panel-only topic's first frame is an entry, not a body to reload:
    // leave it to frameKeep's key-and-document match like every other panel.
    if (!/^topic:/.test(of.getAttribute("data-doc-key") || "")) return null;
    var oc = frameChain(of, old), nc = frameChain(nf, n);
    if (!oc || !nc || oc.length !== nc.length) return null;
    // The frames themselves may differ in kind (a mounted iframe, an
    // incoming placeholder: frameGraft mounts it).
    for (var i = 1; i < oc.length; i++) if (oc[i].tagName !== nc[i].tagName) return null;
    return { old: old, n: n, oc: oc, nc: nc };
  }
  // frameGrafts returns the grafts that move nothing settling el into want
  // would not move anyway, each graft's new section standing for its old
  // one: moving a frame reloads it, so a topic whose old section would move
  // (an update moves its topic to the head), or would push kept topics
  // around it, takes its new section as before. When the grafts together
  // move something, each one that alone moves nothing is kept, if those
  // together still move nothing. Where the browser has Element.moveBefore,
  // settle moves a kept node without reloading its frames (cards.js), so
  // every graft is kept, whatever moves.
  function frameGrafts(el, want, grafts) {
    if (!grafts.length || el.moveBefore) return grafts;
    var base = settleMoves(el, want);
    function still(gs) {
      var by = new Map(gs.map(function (g) { return [g.n, g.old]; })), ok = true;
      settleMoves(el, want.map(function (n) { return by.get(n) || n; })).forEach(function (n) { if (!base.has(n)) ok = false; });
      return ok;
    }
    if (still(grafts)) return grafts;
    var each = grafts.filter(function (g) { return still([g]); });
    return each.length && still(each) ? each : [];
  }
  // settleMoves returns the nodes already in el that settling el into want
  // moves. It replays WsCards.settle (cards.js) on a list: keep the two in
  // step.
  function settleMoves(el, want) {
    var keep = new Set(want), moved = new Set(), i = 0;
    var cur = Array.prototype.filter.call(el.childNodes, function (n) { return keep.has(n); });
    want.forEach(function (n) {
      if (cur[i] === n) { i++; return; }
      var j = cur.indexOf(n);
      if (j >= 0 && n.nodeType === 1 && n.hasAttribute("data-card")) {
        var past = cur.splice(i, j - i);
        past.forEach(function (p) { moved.add(p); });
        cur.splice.apply(cur, [i + 1, 0].concat(past));
      } else {
        if (j >= 0) { moved.add(n); cur.splice(j, 1); }
        cur.splice(i, 0, n);
      }
      i++;
    });
    return moved;
  }
  // frameGraft moves g's new section content into its old section, level by
  // level down the frame's path: each kept element takes the new element's
  // attributes and children, except its child on the path, which stays.
  // At the bottom an unchanged document stays live; a changed frame goes
  // in after the old one, pending.
  function frameGraft(g) {
    var of = g.oc[0], mounted = of.tagName === "IFRAME";
    var same = of.getAttribute("data-doc-key") === g.nc[0].getAttribute("data-doc-key") && of.getAttribute("data-srcdoc") === g.nc[0].getAttribute("data-srcdoc");
    if (same) of.wbOut = false;
    // The next frame of a mounted frame is mounted before it goes in, and
    // before anything holds it (expandos, the load listener): a placeholder
    // would show nothing to reveal. The next frame of an unmounted one (a
    // closed fold, a topic far from the view) is a placeholder that simply
    // takes the old one's place: the topic mounts it once it comes near
    // (viewport bands).
    if (!same && mounted && g.nc[0].tagName !== "IFRAME") g.nc[0] = mountFrame(g.nc[0]);
    var nf = g.nc[0];
    if (!same && mounted) frameNext(nf, of);
    for (var i = g.oc.length - 1; i > 0; i--) {
      var oa = g.oc[i], na = g.nc[i], oc = g.oc[i - 1], nc = g.nc[i - 1], before = true;
      Array.prototype.slice.call(oa.attributes).forEach(function (a) { if (!na.hasAttribute(a.name)) oa.removeAttribute(a.name); });
      Array.prototype.forEach.call(na.attributes, function (a) { if (oa.getAttribute(a.name) !== a.value) oa.setAttribute(a.name, a.value); });
      Array.prototype.slice.call(oa.childNodes).forEach(function (c) { if (c !== oc) oa.removeChild(c); });
      Array.prototype.slice.call(na.childNodes).forEach(function (c) {
        if (c === nc) { before = false; if (c === nf && !same) oa.appendChild(c); }
        else if (before) oa.insertBefore(c, oc);
        else oa.appendChild(c);
      });
    }
    if (!same && !mounted) of.parentNode.removeChild(of);
  }
  // frameNext makes nf the pending next frame of mounted frame of.
  function frameNext(nf, of) {
    nf.classList.add("doc-frame-next");
    nf.setAttribute("tabindex", "-1");
    nf.setAttribute("aria-hidden", "true");
    nf.wbNext = true;
    nf.wbOld = of;
    of.wbOut = true;
    nf.addEventListener("load", function () { frameShow(nf); });
    setTimeout(function () { frameReveal(nf); }, frameRevealMs);
  }
  // frameShow reveals pending frame f once it has loaded and drawn: a
  // frame revealed in the frame it loads in can still show blank for a
  // frame, so the reveal waits two frames more.
  function frameShow(f) {
    if (f.wbShowing) return;
    f.wbShowing = true;
    requestAnimationFrame(function () { requestAnimationFrame(function () { frameReveal(f); }); });
  }
  // frameReveal shows pending frame f in its old frame's place, at the old
  // frame's height, and asks it for its height, which the frame message
  // handler sets (easing to it in a moving swap, wbGrow). A frame no longer
  // on the page (a later swap replaced it) is left alone.
  function frameReveal(f) {
    if (!f.wbNext || !f.isConnected) return;
    var w = window.wbHistory, kept = w && w.keep ? w.keep(f) : null;
    f.wbNext = false;
    f.classList.remove("doc-frame-next");
    f.removeAttribute("tabindex");
    f.removeAttribute("aria-hidden");
    var o = f.wbOld;
    f.wbOld = null;
    if (o && o.parentNode) o.parentNode.removeChild(o);
    if (kept) kept.apply();
    postDoc(f, { wbDoc: "size" });
  }
  // Board motion: a board swap's inserted topic sections grow from the
  // height they had before (a topic the board did not hold grows from 0)
  // to their natural height, and every topic section and card standing in
  // the board slides from where it stood to where it stands now (topics
  // are ordered by their last update, so an update moves its topic to the
  // head and shifts the rest). The spots are read before the morph; a
  // section still animating reads at its rendered height and place, so a
  // new swap starts from there. Topics in the closed pages (moved there or
  // out of there by the closed-page reconciliation) do not move. A changed
  // topic's frame starts at its old height and eases to the first height
  // it reports (the frame message handler, wbGrow).
  function reducedMotion() { return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches); }
  // boardSpots reads each topic section's height and offset from the
  // board's top, keyed "t:" + its topic, and each card's offset, keyed
  // "c:" + its id. A docked topic (in the gallery) counts as paged: an
  // update never moves it.
  function boardSpots(el) {
    var h = {}, top = el.getBoundingClientRect().top;
    boardQ("section.topic[data-topic]").forEach(function (n) {
      var r = n.getBoundingClientRect();
      h["t:" + n.getAttribute("data-topic")] = { h: r.height, y: r.top - top, paged: n.parentNode !== el };
    });
    Array.prototype.forEach.call(el.children, function (n) {
      if (n.hasAttribute("data-card")) h["c:" + n.getAttribute("data-card")] = { y: n.getBoundingClientRect().top - top };
    });
    return h;
  }
  // topicMotion returns, in order, the inserted topic sections that animate
  // and the height each starts from. A large topic (cards.js) does not
  // grow: its height is the sheet's.
  function topicMotion(before, ins) {
    var out = [];
    ins.forEach(function (n) {
      var k = topicKey(n);
      if (k === null || n.hasAttribute("data-max")) return;
      var b = before["t:" + k];
      if (b && b.paged) return;
      out.push({ n: n, from: b ? b.h : 0 });
    });
    return out;
  }
  // growTopic runs after the caller prepared the section (frame heights
  // restored), so its natural height is the target. The animation fills
  // nothing: once done the section has no fixed height, and a later size
  // change (a frame posting its height, a details toggle) is never clipped.
  // A section clips its overflow itself (.topic), and a new one also fades
  // in with its bottom margin growing.
  function growTopic(n, from) {
    if (!n.animate) return null;
    var to = n.getBoundingClientRect().height;
    if (Math.abs(to - from) < 1) return null;
    var a = { height: from + "px" }, b = { height: to + "px" };
    if (!from) {
      a.opacity = 0; b.opacity = 1;
      a.marginBottom = "0px"; b.marginBottom = getComputedStyle(n).marginBottom;
    }
    return n.animate([a, b], { duration: 200, easing: "ease-out" });
  }
  // slideBoard moves each topic section and card standing in the board from
  // its spot before the swap to its place now (FLIP): the layout is
  // already final, and a transform that starts at the old offset eases to
  // none. It runs after the growth started, so the places it reads are the
  // growth's first frame and slide and growth add up to one movement.
  // Nothing moves in the DOM (moving a frame reloads it), and the
  // animation fills nothing, so no transform stays behind. A maximized card,
  // a large topic, and a topic holding a large panel stay put: a transform
  // would anchor the fixed box to the moving node.
  // With from, the board's top when the spots were read, a slide runs in
  // screen terms (a swap compensated the scroll between, boardKeep):
  // an element standing where it stood on screen does not move.
  function slideBoard(el, before, from) {
    var top = el.getBoundingClientRect().top, shift = typeof from === "number" ? from - top : 0, out = [];
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n), card = k === null && n.hasAttribute("data-card");
      var b = k !== null ? before["t:" + k] : card ? before["c:" + n.getAttribute("data-card")] : null;
      if (!b || b.paged || !n.animate || n.hasAttribute("data-max") || (k !== null && n.querySelector("details[data-max]"))) return;
      var dy = b.y + shift - (n.getBoundingClientRect().top - top);
      if (Math.abs(dy) < 1) return;
      out.push(n.animate([{ transform: "translateY(" + dy + "px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }));
    });
    return out;
  }
  // startMotion starts the swap's animations at the swap's first frame. An
  // animation's start time is the timeline's time, which holds still at the
  // last frame's through the tasks after it, up to the next frame's own
  // callbacks; a board swap is a long task (a large board's parse and
  // morph, its new topics' frames), so started as created the animations
  // were over when the swap's first frame drew: it showed the old heights
  // (a closing topic's whole open height, empty, and the board under it
  // blank) and the next frame jumped to the end. The start is set to the
  // clock at the first frame's callbacks instead, and until then (that
  // frame itself, whose timeline time is older) the animations hold their
  // first keyframe.
  function startMotion(list) {
    list = list.filter(Boolean);
    if (!list.length) return;
    list.forEach(function (a) { a.effect.updateTiming({ fill: "backwards" }); });
    requestAnimationFrame(function () {
      var t = performance.now();
      list.forEach(function (a) { if (a.playState === "running") a.startTime = t; });
    });
  }
  // Short owner captures, once per actual scroller, including independent
  // pins. No post-morph target selection or ordinary resize following.
  function boardKeep(nodes) {
    var w = window.wbHistory, handles = [], scrollers = new Set(), resolved = new Map();
    if (!w || !w.keep) return null;
    var lookup = w.snapshot && w.snapshot();
    (nodes || boardBoxes().map(function (b) { return b.firstElementChild || b; })).forEach(function (n) {
      if (w.rendered && !w.rendered(n)) return;
      // A height batch shares ancestor/scroller lookup; inspecting keep() for
      // every frame repeated style/scrollHeight walks through the same topic.
      var s = lookup ? lookup.scroller(n) : w.keep(n, { capture: false }).scroller;
      resolved.set(n, s);
      if (scrollers.has(s)) return;
      scrollers.add(s);
      handles.push(w.keep(n));
    });
    return { scroller: function (n) { return resolved.get(n); }, h: { apply: function () { handles.forEach(function (h) { h.apply(); }); }, end: function () { handles.forEach(function (h) { h.end(); }); } } };
  }
  // Asset reloads cross a document boundary. Keep only tab-local, short-lived
  // identities; placement and late frame layout still belong to wbHistory.
  var BOARD_RELOAD = "ws.dashboard.board-reload." + location.pathname + location.search;
  var boardReloadCanceled = false, boardReloadStop = null;
  function boardReloadInput(e) {
    // Revealing the hidden board is navigation, not a scroll override.
    if ((e.type === "pointerdown" || e.type === "touchstart" || (e.type === "keydown" && /^( |Spacebar)$/.test(e.key))) &&
        e.target.closest && e.target.closest('#tabbar [data-tab="board"]')) return;
    if (e.type === "scroll" && e.target !== doc && e.target !== doc.scrollingElement && e.target.id !== "bcol" && !(e.target.classList && e.target.classList.contains("gal-pbody"))) return;
    if (e.type === "keydown" && !/^(ArrowUp|ArrowDown|PageUp|PageDown|Home|End| |Spacebar)$/.test(e.key)) return;
    boardReloadCanceled = true;
    if (boardReloadStop) boardReloadStop();
  }
  ["wheel", "touchstart", "pointerdown", "keydown", "scroll"].forEach(function (type) {
    doc.addEventListener(type, boardReloadInput, { capture: true, passive: true });
  });
  function boardSaveReload() {
    // A shell and its frame can both receive the asset event. A second
    // reload before placement must carry the original reference forward.
    try {
      var pending = JSON.parse(window.sessionStorage.getItem(BOARD_RELOAD));
      if (!boardReloadCanceled && pending && Number.isFinite(pending.at) && pending.page === location.pathname + location.search &&
          Date.now() >= pending.at && Date.now() - pending.at < 60000 && Array.isArray(pending.records)) return;
    } catch (e) {}
    var w = window.wbHistory, records = [], seen = new Set();
    if (!w || !w.keep) return;
    boardBoxes().forEach(function (box) {
      var h = w.keep(box.firstElementChild || box, { capture: false }), s = h.scroller;
      if (seen.has(s)) return;
      seen.add(s);
      var view = h.viewport(), line = view.top + 1;
      var panels = boardWithSnap(function () { return boardViewedPanels(s); });
      panels.sort(function (a, b) { return a.getBoundingClientRect().top - b.getBoundingClientRect().top; });
      var n = panels.find(function (p) { var r = p.getBoundingClientRect(); return r.top <= line && r.bottom > line; }) ||
        panels.find(function (p) { return p.getBoundingClientRect().top > line; });
      var topic = n && n.closest(".topic");
      records.push({ scroller: s === doc.scrollingElement ? "document" : s.id, top: s.scrollTop, key: n && boardPanelKey(n), topic: topic && topicKey(topic),
        offset: n ? n.getBoundingClientRect().top - line : 0 });
    });
    try { window.sessionStorage.setItem(BOARD_RELOAD, JSON.stringify({ at: Date.now(), page: location.pathname + location.search, records: records })); } catch (e) {}
  }
  if (framed) window.wsSaveBoardReload = boardSaveReload;
  function boardRestoreReload() {
    var saved = null;
    try {
      saved = JSON.parse(window.sessionStorage.getItem(BOARD_RELOAD));
    } catch (e) {}
    var observer = null, expiry = null, done = false;
    function stop() {
      done = true;
      if (observer) observer.disconnect();
      clearTimeout(expiry);
      boardReloadStop = null;
      try { window.sessionStorage.removeItem(BOARD_RELOAD); } catch (e) {}
      ["wheel", "touchstart", "pointerdown", "keydown", "scroll"].forEach(function (type) { doc.removeEventListener(type, boardReloadInput, true); });
    }
    function restore() {
      if (done) return;
      if (boardReloadCanceled || !saved || saved.page !== location.pathname + location.search || !Number.isFinite(saved.at) ||
          Date.now() - saved.at < 0 || Date.now() - saved.at >= 60000 || !Array.isArray(saved.records)) { stop(); return; }
      var w = window.wbHistory;
      if (!w || !w.place) { stop(); return; }
      saved.records = saved.records.filter(function (r) {
        if (!r || !Number.isFinite(r.top) || !Number.isFinite(r.offset)) return false;
        var s = r.scroller === "document" ? doc.scrollingElement : $(r.scroller);
        // Hidden panes have no scroller geometry: handing their nodes to
        // place now would incorrectly select the document as scroll owner.
        if (!s || !visible(s) || !s.clientHeight || !s.clientWidth || !boardBoxes().some(function (b) {
          return visible(b) && b.getBoundingClientRect().height > 0 && (s === doc.scrollingElement || s.contains(b));
        })) return true;
        var topic = boardBoxes().flatMap(function (b) { return q("section.topic[data-topic]", b); }).find(function (n) { return topicKey(n) === r.topic; });
        var n = topic && boardLivePanels(topic).find(function (p) { return boardPanelKey(p) === r.key; });
        if ((!n || !n.getBoundingClientRect().height) && /^(p|e)[1-9][0-9]*$/.test(r.key || "") && w.loadTarget) {
          // The history owner also resolves topics served in inert templates.
          (topic ? [topic] : boardBoxes()).some(function (b) {
            n = w.loadTarget(b, r.key, window.WsCards && window.WsCards.unfoldTopic);
            return !!n;
          });
        }
        n = n || topic;
        if (n) {
          w.place(n, { readingOffset: r.offset });
        } else {
          // Missing identities fall back numerically through the SAME owner.
          var ref = s.firstElementChild;
          if (!ref) return false;
          var top = s === doc.scrollingElement ? 0 : s.getBoundingClientRect().top + s.clientTop;
          w.place(ref, { offset: ref.getBoundingClientRect().top - top + s.scrollTop - r.top });
        }
        return false;
      });
      if (!saved.records.length) { stop(); return; }
      try { window.sessionStorage.setItem(BOARD_RELOAD, JSON.stringify(saved)); } catch (e) {}
      if (!observer && window.ResizeObserver) {
        observer = new ResizeObserver(restore);
        boardBoxes().concat([$("bcol")]).forEach(function (b) { if (b) observer.observe(b); });
      }
      if (!expiry) expiry = setTimeout(stop, 60000 - (Date.now() - saved.at));
    }
    boardReloadStop = stop;
    requestAnimationFrame(restore);
  }
  // A staging pass reads exposure through one wbHistory.snapshot: nothing
  // writes the DOM inside it, so each scroller's viewport (a scan of every
  // sticky head) is measured once rather than once per node checked.
  function boardWithSnap(fn) {
    var outer = boardSnap, w = window.wbHistory;
    if (!outer && w && w.snapshot) boardSnap = w.snapshot();
    try { return fn(); } finally { boardSnap = outer; }
  }
  function boardExposed(n) {
    if (!n || n.isConnected === false) return false;
    var w = window.wbHistory;
    if (w && w.rendered && !w.rendered(n)) return false;
    if (boardSnap) return boardSnap.visible(n);
    return !!(w && w.keep && w.keep(n, { capture: false }).visible(n));
  }
  function boardScroller(n) {
    var w = window.wbHistory;
    if (!n || (w && w.rendered && !w.rendered(n))) return null;
    return boardSnap ? boardSnap.scroller(n) : w && w.keep ? w.keep(n, { capture: false }).scroller : null;
  }
  // Freeze the interval, not just the reading reference. A single scroll
  // delta cannot repair an insertion/reorder between two visible units.
  // Units outside the interval retain canonical order and render normally.
  function boardStageOrder(current, want, exposed) {
    var first = current.findIndex(function (n) { return exposed.indexOf(n) >= 0; });
    if (first < 0) return want;
    var last = current.length - 1;
    while (exposed.indexOf(current[last]) < 0) last--;
    var block = current.slice(first, last + 1).map(function (n) {
      var k = topicKey(n);
      return k === null ? n : want.find(function (w) { return topicKey(w) === k; }) || n;
    }), held = new Set(block);
    var index = want.findIndex(function (n) { return held.has(n); });
    if (index < 0) index = Math.min(first, want.length);
    var out = want.filter(function (n) { return !held.has(n); });
    index = want.slice(0, index).filter(function (n) { return !held.has(n); }).length;
    out.splice.apply(out, [index, 0].concat(block));
    if (out.length !== want.length || out.some(function (n, i) { return n !== want[i]; })) {
      block.concat(want).forEach(function (n) {
        var k = topicKey(n);
        if (k !== null && out.indexOf(n) !== want.indexOf(n)) boardPending[k] = true;
      });
    }
    return out;
  }
  // Canonical panel/article bytes, including inert history pages. Live DOM
  // is used only for exposure/identity, never as an incoming snapshot.
  function boardPanelNodes(n) {
    var nodes = q(".entry-panel, .wb-entry:not(.panel), .doc-frame", n).filter(function (p) {
      return !p.classList.contains("doc-frame") || !p.closest(".entry-panel");
    });
    q("template", n).forEach(function (t) { if (t.content) nodes = nodes.concat(boardPanelNodes(t.content)); });
    return nodes;
  }
  // The mounted panels only: inert history pages are never exposed. admit,
  // when given, rejects a panel before any visibility read of it.
  function boardLivePanels(n, admit) {
    var w = window.wbHistory;
    return q(".entry-panel, .wb-entry:not(.panel), .doc-frame", n).filter(function (p) {
      return (!admit || admit(p)) && (!w || !w.rendered || w.rendered(p)) && (!p.classList.contains("doc-frame") || !p.closest(".entry-panel"));
    });
  }
  function boardPanelKey(n) { return n.id || n.getAttribute("data-key") || n.getAttribute("data-entry") || n.getAttribute("data-doc-key"); }
  function boardPanelMap(list) {
    var m = new Map();
    list.forEach(function (p) { var k = boardPanelKey(p); if (!m.has(k)) m.set(k, p); });
    return m;
  }
  // The exposed panels on scroller, in document order, once per snapshot.
  function boardViewedPanels(scroller) {
    var memo = boardSnap && (boardSnap.viewed = boardSnap.viewed || new Map());
    if (memo && memo.has(scroller)) return memo.get(scroller);
    var list = boardLivePanels(scroller).filter(function (p) { return boardExposed(p) && boardScroller(p) === scroller; });
    if (memo) memo.set(scroller, list);
    return list;
  }
  function boardRenderBlockers(live, old, incoming) {
    var exposed = boardLivePanels(live).filter(boardExposed);
    if (!incoming) return exposed.length ? exposed : boardExposed(live) ? [live] : [];
    var before = boardPanelNodes(old), after = boardPanelNodes(incoming), was = boardPanelMap(before), now = boardPanelMap(after);
    var changed = exposed.filter(function (p) {
      var key = boardPanelKey(p), a = was.get(key), b = now.get(key);
      return !a || !b || a.outerHTML !== b.outerHTML;
    });
    // The frozen interval crosses topic boundaries, but never scrollers.
    // Extend this topic's canonical span toward other exposed units.
    var leading = false, trailing = false;
    if (exposed.length) {
      var viewed = boardViewedPanels(boardScroller(live));
      var first = viewed.indexOf(exposed[0]), last = viewed.indexOf(exposed[exposed.length - 1]);
      leading = first > 0;
      trailing = last >= 0 && last < viewed.length - 1;
    }
    if (exposed.length > 1 || leading || trailing) {
      function span(list) {
        var keys = exposed.map(boardPanelKey), indices = list.map(function (p, i) { return keys.indexOf(boardPanelKey(p)) >= 0 ? i : -1; }).filter(function (i) { return i >= 0; });
        return indices.length ? list.slice(leading ? 0 : Math.min.apply(Math, indices), trailing ? list.length : Math.max.apply(Math, indices) + 1).map(function (p) { return p.outerHTML; }).join("\n") : "";
      }
      if (span(before) !== span(after)) return exposed;
    }
    return changed;
  }
  function boardRecordPending() {
    boardChanges = boardChanges.filter(function (c) { return c.names.some(function (k) { return boardPending[k]; }); });
    if (boardRelease) { boardChanges = []; return; }
    var offer = boardOffered;
    boardOffered = null;
    if (!offer || !offer.input) return;
    var previous = offer.previous, input = offer.input, content = [];
    Object.keys(boardPending).forEach(function (k) {
      var old = previous && previous.topics[k], next = input.topics[k];
      if (!old || !next || topicSeen(old) !== topicSeen(next)) content.push(k);
    });
    content.forEach(function (k) { boardChanges.push({ names: [k] }); });
    if (!content.length && previous && previous.order.join("\n") !== input.order.join("\n") && Object.keys(boardPending).length)
      boardChanges.push({ names: Object.keys(boardPending) });
  }
  // A send grants no blanket exemption to its topic: a simultaneous agent
  // rewrite is still external. Server-rendered owner receipts identify the
  // new rows. Admit them alone when the same snapshot also changes panels.
  function boardOwnerSnapshot(old, next) {
    if (!old || !next) return null;
    var known = new Set(q('.wb-entry.owner', old).map(function (n) { return n.id; }));
    var arrivals = q('.wb-entry.owner', next).filter(function (n) { return !known.has(n.id); });
    if (!arrivals.length) return null;
    function external(n) { return !n.classList.contains('owner'); }
    var before = boardPanelNodes(old).filter(external), after = boardPanelNodes(next).filter(external);
    var changed = before.length !== after.length || before.some(function (n, i) { return boardPanelKey(n) !== boardPanelKey(after[i]) || n.outerHTML !== after[i].outerHTML; });
    if (!changed) return { source: next, external: false };
    var partial = old.cloneNode(true), admitted = [];
    arrivals.forEach(function (n) {
      var h = n.closest('.wb-history'), key = h && h.getAttribute('data-history');
      var target = q('.wb-history', partial).find(function (p) { return p.getAttribute('data-history') === key; });
      if (target) { target.querySelector('.history-live').appendChild(n.cloneNode(true)); admitted.push(n.id); }
    });
    if (!admitted.length) return null;
    partial.setAttribute('data-th', topicSeen(old) + ':owner:' + admitted.join(','));
    return { source: partial, external: true };
  }
  // Fetched closed-page topics are not canonical top-level members yet.
  // Keep an exposed transfer in its page while admitting eligible rendering
  // in place. The original immutable fetched bytes, not mounted frame DOM,
  // are the diff source. Placement releases when that card fully leaves.
  function boardStagePaged(input, topics, order) {
    boardPagedEdits = [];
    boardBoxes().forEach(function (box) {
      q('.closed-page section.topic[data-topic]', box).forEach(function (live) {
        var k = topicKey(live), next = topics[k], old = boardPaged[k];
        if (!next || boardRelease || (boardImmediate[k] && !boardPending[k]) || !boardExposed(live)) return;
        var blockers = old ? boardRenderBlockers(live, old, next) : [live];
        if (!blockers.length && topicSeen(old) !== topicSeen(next)) {
          boardPagedEdits.push({ live: live, source: next });
          boardPaged[k] = next;
        }
        delete topics[k];
        var at = order.indexOf(k); if (at >= 0) order.splice(at, 1);
        boardPending[k] = blockers.length ? blockers : [live];
      });
    });
  }
  // Top-origin changes are click-only, independently of ordinary viewed-panel
  // blockers. Keep immutable old bytes/absence and their old open-flow slots;
  // subsequent scroll releases can still admit unrelated down-origin panels.
  function boardStageTop(input, topics, order) {
    if (boardRelease) { boardTopHeld = Object.create(null); boardTopOrder = null; return order; }
    var box = boardBoxes()[0], ref = box && (box.firstElementChild || box);
    var s = ref && boardScroller(ref), offer = boardOffered, previous = offer && offer.previous;
    if (s && s.scrollTop <= 4 && offer && previous && boardRendered) {
      var changed = Array.from(new Set(previous.order.concat(input.order))).filter(function (k) {
        var a = previous.topics[k], b = input.topics[k];
        if (boardImmediate[k] || (a && b && topicSeen(a) === topicSeen(b))) return false;
        if (!a || !b || previous.order.indexOf(k) !== input.order.indexOf(k)) return true;
        var live = Array.prototype.find.call(box.children, function (n) { return topicKey(n) === k; });
        return !!live && boardExposed(live);
      });
      if (!changed.length && previous.order.join('\n') !== input.order.join('\n'))
        changed = input.order.filter(function (k, i) { return previous.order[i] !== k && !boardImmediate[k]; });
      if (changed.length && !boardTopOrder) {
        var shown = Array.prototype.map.call(box.children, topicKey).filter(function (k) { return k !== null; });
        boardTopOrder = shown.concat(boardRendered.order.filter(function (k) { return shown.indexOf(k) < 0; }));
      }
      changed.forEach(function (k) {
        if (!Object.prototype.hasOwnProperty.call(boardTopHeld, k))
          boardTopHeld[k] = { node: boardRendered.topics[k] || null };
      });
    }
    var held = Object.keys(boardTopHeld);
    held.forEach(function (k) {
      if (boardImmediate[k]) boardTopHeld[k].node = topics[k] || boardTopHeld[k].node;
      var old = boardTopHeld[k].node;
      if (old) topics[k] = old; else delete topics[k];
      boardPending[k] = true;
    });
    order = order.filter(function (k) { return !Object.prototype.hasOwnProperty.call(boardTopHeld, k); });
    (boardTopOrder || []).slice().reverse().forEach(function (k, i, reversed) {
      if (!boardTopHeld[k] || !boardTopHeld[k].node) return;
      var next = reversed.slice(0, i).reverse().find(function (n) { return order.indexOf(n) >= 0; });
      order.splice(next === undefined ? order.length : order.indexOf(next), 0, k);
    });
    return order;
  }
  function boardStageInput(input) { return boardWithSnap(function () { return boardStageSnap(input); }); }
  function boardStageSnap(input) {
    boardPending = Object.create(null);
    boardImmediate = Object.create(null);
    boardFrozen = [];
    if (!input || typeof input === "string") return input;
    var topics = Object.assign(Object.create(null), input.topics), order = input.order.slice();
    Object.keys(boardOwn).forEach(function (k) {
      var old = boardRendered && boardRendered.topics[k] || boardPaged[k];
      var own = boardOwnerSnapshot(old, topics[k]);
      if (!own) return;
      topics[k] = own.source; boardImmediate[k] = true; delete boardOwn[k];
      if (own.external) boardPending[k] = true;
    });
    order = boardStageTop(input, topics, order);
    boardBoxes().forEach(function (box) {
      var nodes = Array.prototype.filter.call(box.children, function (n) {
        var k = topicKey(n);
        var group = n.classList.contains('closed-page');
        return (k !== null || n.hasAttribute("data-card") || group) && !boardRelease && !boardImmediate[k] && boardExposed(n);
      });
      if (nodes.length) boardFrozen.push({ box: box, nodes: nodes });
      nodes.forEach(function (n) {
        var k = topicKey(n), old = boardRendered && boardRendered.topics[k];
        if (k === null || !old || (topics[k] && topicSeen(topics[k]) === topicSeen(old))) return;
        var blockers = boardRenderBlockers(n, old, topics[k]);
        if (!blockers.length) return;
        topics[k] = old;
        if (order.indexOf(k) < 0) order.push(k);
        boardPending[k] = blockers;
      });
    });
    boardStagePaged(input, topics, order);
    // Docked units keep their panes whatever the board does, so only their
    // content follows the same per-panel rendering blockers.
    var docked = window.WsCards && window.WsCards.docked ? window.WsCards.docked() : [];
    if (!boardRelease) {
      docked.filter(function (n) { return !boardImmediate[topicKey(n)] && boardExposed(n); }).forEach(function (n) {
        var k = topicKey(n), old = boardRendered && boardRendered.topics[k];
        if (k === null || !old || !topics[k] || topicSeen(old) === topicSeen(topics[k])) return;
        var blockers = boardRenderBlockers(n, old, topics[k]);
        if (blockers.length) { topics[k] = old; boardPending[k] = blockers; }
      });
    }
    // A partial input records only bytes actually admitted, never held hashes.
    boardRendered = { topics: topics, order: order, before: input.before, after: input.after };
    return boardRendered;
  }
  function boardPendingNames() {
    var names = new Set();
    boardChanges.forEach(function (c) { c.names.forEach(function (k) { if (boardPending[k]) names.add(k); }); });
    return Array.from(names);
  }
  function boardPendingBadge() {
    var pill = window.wbPill || (window.wbPill = {});
    pill.pending = boardPendingNames().length;
    pill.home = boardHome;
    // Explicit board navigation acknowledges the same staged snapshot as home,
    // without starting a competing scroll-to-top operation.
    pill.apply = boardApplyPending;
    if (pill.update) pill.update();
  }
  function boardApplyPending(retry) {
    if (!Object.keys(boardPending).length) return true;
    var overview = window.WsOverview;
    if (overview && overview.holding()) {
      if (retry) overview.after(retry);
      return false;
    }
    boardFlushViewed(true);
    return true;
  }
  // Only a cached shell frame's return gets this exemption, not a browser
  // tab visibility change. Restore may apply old held bytes first; keep the
  // window for the reconnect's first snapshot (or a bounded no-change wait).
  function boardReturnShown() {
    boardReturnEnd();
    boardReturnTimer = setTimeout(boardReturnEnd, 1500);
    boardReturnApply();
  }
  function boardReturnEnd() {
    clearTimeout(boardReturnTimer);
    boardReturnTimer = 0;
    boardReturnFresh = false;
  }
  function boardReturnApply() {
    if (!boardReturnTimer) return;
    if (boardApplyPending(boardReturnApply) && boardReturnFresh && !Object.keys(boardPending).length) boardReturnEnd();
  }
  function boardFlash(names, inView) {
    boardBoxes().forEach(function (box) {
      q('section.topic[data-topic]', box).forEach(function (n) {
        if (names.indexOf(topicKey(n)) < 0 || (inView && !boardExposed(n))) return;
        n.classList.remove('board-fresh');
        void n.offsetWidth;
        n.classList.add('board-fresh');
        clearTimeout(n.wbFreshTimer);
        n.wbFreshTimer = setTimeout(function () { n.classList.remove('board-fresh'); }, 650);
      });
    });
  }
  function boardHome() {
    var names = boardPendingNames(), host = $('bcol') || doc.querySelector('.wrap');
    var ref = host && host.querySelector('.oq-wrap.wb-oq-top'), w = window.wbHistory;
    if (!ref || !w || !w.jump) return;
    // The existing navigation owner cancels captures/pins BEFORE the release
    // morph, suppresses new captures during it, then owns the native to-top.
    w.jump(ref, { top: 0, behavior: 'smooth', prepare: function () {
      boardFlushViewed(true);
      boardFlash(names, false);
    } });
  }
  function boardHoldIndicators() {
    var names = boardPendingNames(), input = boardLatest && boardLatest.html;
    boardBoxes().forEach(function (box) {
      q(names.length ? 'section.topic[data-topic]' : 'section.topic[data-board-held]', box).forEach(function (n) {
        var k = topicKey(n), held = names.indexOf(k) >= 0 || !!(boardPending[k] && n.hasAttribute('data-board-held'));
        if (!held && !n.hasAttribute('data-board-held')) return;
        var tag = n.querySelector('.board-hold-tag');
        if (!held) {
          n.removeAttribute('data-board-held'); n.classList.remove('board-hold-nudge');
          if (n.wbHoldNudge) cancelAnimationFrame(n.wbHoldNudge);
          if (tag) tag.remove();
          delete n.wbHoldRevision;
          return;
        }
        var incoming = input && input.topics[k], closed = !incoming || incoming.classList.contains('closed');
        n.setAttribute('data-board-held', closed ? 'closed' : 'moved');
        if (!tag) {
          var bar = n.querySelector('.tbar');
          if (!bar) return;
          tag = doc.createElement('span'); tag.className = 'board-hold-tag'; bar.appendChild(tag);
        }
        tag.textContent = closed ? wsT("js.board.hold.closed") : wsT("js.board.hold.moved");
        var revision = incoming ? topicSeen(incoming) : 'closed';
        if (n.wbHoldRevision !== revision) {
          n.wbHoldRevision = revision;
          n.classList.remove('board-hold-nudge');
          if (n.wbHoldNudge) cancelAnimationFrame(n.wbHoldNudge);
          // Restart after a painted reset, not a forced whole-board layout
          // inside the synchronous stream admission transaction.
          n.wbHoldNudge = requestAnimationFrame(function () {
            n.wbHoldNudge = requestAnimationFrame(function () {
              n.wbHoldNudge = 0;
              if (n.isConnected && n.hasAttribute('data-board-held')) n.classList.add('board-hold-nudge');
            });
          });
        }
      });
    });
  }
  function boardFlushViewed(explicit) {
    boardReleaseFrame = 0;
    var pending = Object.keys(boardPending);
    if (!pending.length || (!explicit && pending.every(function (k) { return Object.prototype.hasOwnProperty.call(boardTopHeld, k); }))) return;
    boardRelease = !!explicit;
    try {
      if (boardLatest) swap("board", boardLatest.hash, boardLatest.html, true);
      boardPendingBadge();
    } finally { boardRelease = false; }
  }
  // The exposed units a staged release can depend on: top-level topics,
  // cards and groups, the pin list's topics and fetched closed-page topics.
  function boardExposureUnits() {
    var units = window.WsCards && window.WsCards.docked ? window.WsCards.docked() : [], panels = [];
    boardBoxes().forEach(function (box) {
      units = units.concat(Array.prototype.slice.call(box.children), q(".closed-page section.topic[data-topic]", box));
      panels = panels.concat(boardLivePanels(box));
    });
    // Exposed panels set the frozen interval across topics (the leading and
    // trailing span in boardRenderBlockers), so another topic's panel leaving
    // can release a block whose own panels are all still in view.
    var w = window.wbHistory;
    return units.concat(panels).filter(function (n) { return !w || !w.rendered || w.rendered(n); }).filter(boardExposed);
  }
  // A scroll or resize re-stages only when a release can follow: a pending
  // panel left the view (its topic's rendering may release), or the exposed
  // unit or panel set changed (a frozen interval or staged placement may
  // release).
  // Re-staging on every scroll frame repeated a whole board swap per frame
  // while anything was pending.
  function boardReleaseDue() {
    return boardWithSnap(function () {
      var gone = Object.keys(boardPending).some(function (k) {
        var b = boardPending[k];
        return Array.isArray(b) && b.some(function (n) { return !boardExposed(n); });
      });
      if (gone || !boardExposedUnits) return true;
      var units = boardExposureUnits();
      return units.length !== boardExposedUnits.length || units.some(function (n, i) { return n !== boardExposedUnits[i]; });
    });
  }
  function boardViewedScroll(e) {
    if (e && e.type === 'scroll') {
      var s = e.target === doc ? doc.scrollingElement : e.target;
      if (s && boardHomeWrites.has(s)) {
        boardHomeWrites.delete(s);
        if (s.scrollTop <= 4) return;
      }
      // Read scrollTop only while changes are staged: the read forces a
      // layout on every scroll event, and with nothing staged the home
      // branch has nothing to keep (pill.js redraws on its own scroll).
      if (s && (s === $('bcol') || s === doc.scrollingElement) && Object.keys(boardPending).length && s.scrollTop <= 4) {
        // Raw scrolls include owner corrections and ongoing user gestures;
        // do not take over their scroll owner with an explicit home jump.
        // Nothing is applied here, so every staged change keeps its signal.
        boardPendingBadge();
        if (boardReleaseFrame) cancelAnimationFrame(boardReleaseFrame);
        boardReleaseFrame = 0;
        return;
      }
    }
    // IO uses the raw viewport, not the inset below sticky heads. One idle
    // check per gesture restores that exact boundary without per-scroll scans.
    if (Object.keys(boardPending).length && !("onscrollend" in doc)) {
      clearTimeout(boardScrollTimer);
      boardScrollTimer = setTimeout(function () { boardViewedTrailing(e); }, 120);
    }
    // Modern hosts get exposure changes from IO, not from scanning the whole
    // board on every raw scroll. Old engines retain one coalesced fallback.
    if (!window.IntersectionObserver) boardViewedExposure();
  }
  function boardViewedTrailing(e) {
    if (e && e.target && e.target !== doc && e.target !== doc.scrollingElement &&
        e.target !== $('bcol') && !inBoard(e.target)) return;
    clearTimeout(boardScrollTimer);
    boardScrollTimer = 0;
    boardViewedExposure();
  }
  function boardViewedExposure(report) {
    if (report && report.node) {
      if (report.node.wbViewedExposure === report.visible) return;
      report.node.wbViewedExposure = report.visible;
    }
    if (!boardReleaseFrame && Object.keys(boardPending).length) boardReleaseFrame = requestAnimationFrame(function () {
      boardReleaseFrame = 0;
      if (Object.keys(boardPending).length && boardReleaseDue()) boardFlushViewed(false);
    });
  }
  doc.addEventListener("scroll", boardViewedScroll, { capture: true, passive: true });
  doc.addEventListener("scrollend", boardViewedTrailing, { capture: true, passive: true });
  window.addEventListener("resize", function () { if (!window.IntersectionObserver) boardViewedExposure(); });
  doc.addEventListener("toggle", function (e) {
    var n = e.target;
    if (!n || n.tagName !== "DETAILS" || !inBoard(n)) return;
    if (n.open) watchBoard(n);
    boardViewedExposure();
  }, true);
  // Watch topics, panels and placement groups through the shared IO source.
  // A swap sweeps removed subscriptions. Observing closed content is safe:
  // watch() skips its scroller inspection, and IO reports when it opens.
  // Exposure transitions, board disclosures and a trailing gesture check gate
  // release; ordinary modern scroll events do no per-unit geometry work.
  function watchBoard(root) {
    if (!window.wbHistory || !window.wbHistory.watch || !root || root.nodeType !== 1) return;
    var w = window.wbHistory;
    var list = root.matches("section.topic[data-topic]") ? [root] : q("section.topic[data-topic]", root);
    list = list.concat(q(".entry-panel, .wb-entry:not(.panel), .doc-frame, [data-card], .closed-page", root));
    list.forEach(function (n) {
      if (!n.wbWatch) n.wbWatch = w.watch(n, { onChange: boardViewedExposure });
    });
  }

