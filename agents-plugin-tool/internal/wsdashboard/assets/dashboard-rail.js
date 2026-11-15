  // --- session rail ------------------------------------------------------------
  // The rail (r-rail) is one render shared by every session page, in the
  // order, tree, and with the stars of the daemon's rail layout (a drag or
  // a star posts a layout op, and the rail swap shows it): the page marks
  // its own row, collapses the workspaces and parent rows the user
  // collapsed (rail.js keeps those in storage), and opens the add popovers
  // (the blocks after the drag's).
  // From 1600px the rail is a column, except below 2200px while the pinned
  // column shows (data-pincol on the root, the board cards block): the
  // pinned column takes its place. Otherwise the top-left arrow slides it
  // in over the page, and Esc, the scrim, or picking a session closes it.
  // The arrow stays a link to the sessions index for a modified click.
  var railEl = $("rail"), railBtn = $("railbtn"), rscrim = $("rscrim"), WsRail = window.WsRail;
  var railHoverUpdates = region("rail") && WsRail ? WsRail.hoverUpdates(region("rail")) : null;
  var memoPad = railEl && $("memo-pad") && window.WsMemo ? window.WsMemo.mount(doc, railEl, $("memo-pad"), function (open) { railOpen(open === true); }) : null;
  var railStore = { load: load, save: save }, railSets = WsRail ? WsRail.state(railStore) : null;
  var colSet = railSets && railSets.collapsed, kidSet = railSets && railSets.kids;
  // rdrag is the press or drag in the rail (the drag block below);
  // railSaving counts the layout ops in flight, and railMinGen is the
  // layout generation the last one applied answered (a rail render below
  // it is stale); railGate holds the rail swaps back while a press lasts,
  // a row's rename is open, or an op posts.
  var rdrag = null, railSaving = 0, railMinGen = 0;
  var railGate = WsRail ? WsRail.swapGate(function () { return !!rdrag || railEditing() || railSaving > 0; }) : null;
  var RAIL_WIDE = window.matchMedia ? window.matchMedia("(min-width: 1600px)") : null;
  var RAIL_XWIDE = window.matchMedia ? window.matchMedia("(min-width: 2200px)") : null;
  var RAIL_SCROLL_KEY = "ws.dashboard.railscroll";
  // rp is the open rail popover (the next blocks): its controller P, kind,
  // what it concerns (ws: a workspace, the folder a new session starts in,
  // the viewed session, or a row menu's session), and the button it hangs
  // from (a row menu's: the row's link; at, where in the row it opened).
  var rp = { P: null, kind: "", ws: "", btn: null }, rpWs = null, rpWt = null, rpLead = null, rpSess = null, rpRow = null, rpUsage = null, rpIds = 0;
  var usageGutter = railEl && window.WsUsage ? window.WsUsage.mount(doc, railEl) : null;
  function railWide() { return !!(RAIL_WIDE && RAIL_WIDE.matches) && (!root.hasAttribute("data-pincol") || !!(RAIL_XWIDE && RAIL_XWIDE.matches)); }
  function railShown() { return root.classList.contains("rail-open"); }
  // Ctrl+` toggles rail + memos in overlay mode, memos alone beside a column;
  // a framed page hands it to the shell, which holds the rail.
  function railKey() {
    if (window.WsOverview && window.WsOverview.holding()) { window.WsOverview.after(railKey); return; }
    if (framed) { shPost("key", { key: "rail" }); return; }
    if (!railEl || !WsRail) return;
    railOpen(!railShown());
  }
  function railOpen(open) {
    if (!railEl) return;
    root.classList.toggle("rail-open", !!open);
    if (memoPad) memoPad.open(!!open, railWide());
    if (railBtn) railBtn.setAttribute("aria-expanded", String(!!open || railWide()));
  }
  function railScrollTop() { var l = $("rlist"); return l ? l.scrollTop : null; }
  function railScrollTo(top) { var l = $("rlist"); if (l && top !== null && top !== undefined && l.scrollTop !== top) l.scrollTop = top; }
  // railMarks applies the page's marks to the rail as rendered: after
  // load and after every rail swap.
  function railMarks() {
    var r = region("rail");
    if (!r || !WsRail) return;
    WsRail.marks(r, railSets, rid);
    var selectedPath = location.pathname.indexOf("/w/") === 0 ? decodeURIComponent(location.pathname.slice(3)) : "";
    q(".wthd", r).forEach(function (hd) {
      var path = hd.closest(".rwt").getAttribute("data-g").replace(/\\/g, "/").replace(/^\//, "");
      hd.classList.toggle("worktree-selected", !!selectedPath && path === selectedPath);
    });
    if (memoPad) memoPad.sync(rid);
    WsRail.markRevive(q(".rs", r), revBusy);
    if (usageGutter) usageGutter.sync();
    railPopSync();
  }
  // railFocus is what holds the focus in the rail before a swap replaces
  // every row: a row's link or one of its buttons, by the row's reply-id;
  // railRefocus puts it back on the new row after the swap, unless the
  // focus went elsewhere meanwhile. A keyboard move (Alt and an arrow)
  // lands its swap on the row it moved, so the next key still finds it.
  var RAIL_FOCUS = [".rl", ".star", ".kidtog", ".drren", ".drcopy", ".rrev"];
  function railFocus() {
    var a = doc.activeElement, row = a && railEl && WsRail && railEl.contains(a) && a.closest ? a.closest(".rs") : null;
    if (!row) return null;
    for (var i = 0; i < RAIL_FOCUS.length; i++) if (a.matches(RAIL_FOCUS[i])) return { rid: row.getAttribute("data-rid") || "", sel: RAIL_FOCUS[i] };
    return null;
  }
  function railRefocus(f) {
    var a = doc.activeElement, r = region("rail");
    if (!f || !r || (a && a !== doc.body)) return;
    var row = WsRail.refind(r, { cls: "rs", attr: "data-rid", val: f.rid }), n = row && row.querySelector(f.sel);
    if (n && n.getClientRects().length) n.focus({ preventScroll: true });
  }
  // railSay says text in the rail's live line (rlive, made once outside
  // the rail region): what a keyboard or menu move did.
  var rliveEl = null;
  function railSay(text) { if (rliveEl) rliveEl.textContent = text; }
  // railName is a session row's name as shown: its label, else its
  // reply-id.
  function railName(row) {
    var nm = row && row.querySelector(".rid");
    return nm ? nm.textContent : "";
  }
  // kidFold collapses (fold) or expands the parent row's children, a view
  // preference kept per browser.
  function kidFold(row, fold) {
    var id = row.getAttribute("data-rid") || "";
    if (id && kidSet.has(id) !== fold) kidSet.toggle(id);
    railMarks();
  }
  // railRelease lands the rail swap held while a press, a rename, or a
  // layout op lasted, once none does.
  function railRelease() {
    var h = railGate && railGate.release();
    if (h) swap("rail", h.hash, h.html);
  }
  function railEditing() { return !!(railEl && railEl.querySelector(".drinp")); }
  // layoutPost posts body, one rail layout op, to POST /rail/layout;
  // done(status, answer) gets the HTTP status (0: the daemon was not
  // reached) and the parsed answer, {} when it is not JSON. The route
  // answers JSON ({applied, reason, gen}) for a refusal as for a success.
  // A post that has not answered in LAYOUT_WAIT ms is given up (as
  // unreachable): while one is in flight every rail swap waits, so a hung
  // request must not hold the rail.
  var LAYOUT_WAIT = 15000;
  function layoutPost(body, done) {
    if (!window.fetch) { done(0, {}); return; }
    var ac = window.AbortController ? new AbortController() : null;
    var timer = setTimeout(function () { finish(0, {}); if (ac) ac.abort(); }, LAYOUT_WAIT);
    function finish(status, v) {
      if (!timer) return;
      clearTimeout(timer);
      timer = 0;
      done(status, v);
    }
    fetch("/rail/layout", {
      method: "POST", credentials: "same-origin", cache: "no-store", signal: ac ? ac.signal : undefined,
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        var v = {};
        try { v = s ? JSON.parse(s) : {}; } catch (e) { v = {}; }
        finish(r.status, v && typeof v === "object" ? v : {});
      });
    }).catch(function () { finish(0, {}); });
  }
  // layoutSave posts a layout op the page already shows: the rail swaps
  // wait meanwhile, an applied op's generation marks every older render
  // stale and runs done, and a refused one runs undo (which puts the page
  // back) and toasts the reason.
  function layoutSave(body, undo, done) {
    railSaving++;
    layoutPost(body, function (status, v) {
      railSaving--;
      if (v.applied === true) {
        if (+v.gen > railMinGen) railMinGen = +v.gen;
        if (done) done();
      } else {
        if (undo) undo();
        toast(status ? wsT(WsRail.layoutRefusal(v.reason)) : wsT("js.post.unreachable"));
      }
      railRelease();
    });
  }
  // layoutImport runs once on load: a browser that kept the rail's order
  // and stars locally before the daemon kept them imports them into an
  // empty layout, and drops them once the daemon answered (rail.js
  // legacyImport, legacyDone).
  function layoutImport() {
    var list = $("rlist"), body = list && WsRail.legacyImport(railStore, list.hasAttribute("data-layout-empty"));
    if (body) layoutPost(body, function (status, v) { WsRail.legacyDone(railStore, v); });
  }
  // railRename runs the head's rename (labelStart) on a rail row: the name,
  // its hosted mark and branch tag, the star, and a parent's toggle give
  // way to the input (before the star, or the row's tail on a child, which
  // has none), which posts to
  // that session's label route; a save shows the label in the row and, for
  // the viewed session, in the head. The rail swap waits while it is open.
  function railRename(row) {
    if (!row || railEditing()) return;
    var id = row.getAttribute("data-rid") || "", s1 = row.querySelector(".s1"), nm = row.querySelector(".rid");
    var pen = row.querySelector(".drren"), cp = row.querySelector(".drcopy"), star = row.querySelector(".star"), list = $("rlist");
    var at = star || row.querySelector(".drtail");
    if (!id || !s1 || !nm || !pen || !at) return;
    labelStart({
      box: s1, at: at, cls: "drinp", hide: [nm, row.querySelector(".hosted"), row.querySelector(".rbt"), star, row.querySelector(".kidtog")], route: "/s/" + encodeURIComponent(id) + "/label",
      label: pen.getAttribute("data-label") || "", max: +(list && list.getAttribute("data-max")) || 64,
      ph: id, aria: pen.getAttribute("aria-label") || "",
      saved: function (label) {
        pen.setAttribute("data-label", label);
        nm.textContent = label || id;
        nm.classList.toggle("lbl", !!label);
        if (label) nm.setAttribute("title", id); else nm.removeAttribute("title");
        if (cp) labelCopy(cp, label);
        var h = region("head"), sn = id === rid && h ? h.querySelector(".sname") : null;
        if (sn) labelShow(sn, label);
      },
      ended: railRelease
    });
  }
  // railMove shows the session row's move z (WsRail.keyMove's, or the row
  // menu's) at once, the focus kept on the row's link, and posts its op;
  // once the op applied the live line says what it did ("EXEC → under
  // 메인"), and a refused one goes back with the reason in a toast. A
  // collapsed parent the row goes under opens. A detach the page has no
  // row to show against (no z.zone) shows with the daemon's render.
  function railMove(row, z) {
    var o = z.op, name = railName(row), undo = null, link = row.querySelector(".rl");
    var ref = z.ref ? WsRail.refind(railEl, { cls: "rs", attr: "data-rid", val: z.ref }) : null;
    var text = o.op === "attach" ? wsT("js.rail.moved.under", name, railName(WsRail.refind(railEl, { cls: "rs", attr: "data-rid", val: o.parent })))
      : o.op === "detach" ? wsT("js.rail.moved.top", name)
      : o.before ? wsT("js.rail.moved.before", name, railName(ref)) : wsT("js.rail.moved.after", name, railName(ref));
    if (ref && z.zone) {
      if (z.zone === "child" && ref.classList.contains("kcol")) kidFold(ref, false);
      undo = WsRail.moveRow(row, ref, z.zone, function () {
        var b = doc.createElement("div");
        b.className = "rkids";
        return b;
      });
      railMarks();
    }
    if (link) link.focus();
    layoutSave(o, function () {
      if (!undo) return;
      undo();
      railMarks();
      if (link && link.isConnected && (!doc.activeElement || doc.activeElement === doc.body)) link.focus();
    }, function () { railSay(text); });
  }
  if (railEl && WsRail) {
    // The live line the keyboard path and the row menu say their moves in.
    rliveEl = doc.body.appendChild(mk("div", "rlive"));
    rliveEl.setAttribute("role", "status");
    rliveEl.setAttribute("aria-live", "polite");
    if (railBtn) railBtn.addEventListener("click", function (e) {
      if (e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      e.preventDefault();
      railOpen(!railShown());
    });
    if (rscrim) rscrim.addEventListener("click", function () { railOpen(false); });
    doc.addEventListener("keydown", function (e) { if (e.key === "Escape" && railShown()) railOpen(false); });
    // Ctrl+` (the physical key, so any layout) also opens memos beside
    // the permanent rail column.
    // A topic body frame holding the focus hands the key over ({wbDoc:
    // "rail"}, the frame listener above).
    doc.addEventListener("keydown", function (e) {
      if (e.code !== "Backquote" || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.isComposing) return;
      e.preventDefault();
      railKey();
    });
    [RAIL_WIDE, RAIL_XWIDE].forEach(function (m) { if (m && m.addEventListener) m.addEventListener("change", function () { railOpen(false); }); });
    railEl.addEventListener("click", function (e) {
      var t = e.target;
      // A star shows pressed at once; the rail swap after the op moves the
      // row.
      var star = t.closest && t.closest(".star");
      if (star) {
        e.preventDefault();
        var row = star.closest(".rs"), on = star.getAttribute("aria-pressed") !== "true";
        if (!row) return;
        star.setAttribute("aria-pressed", String(on));
        layoutSave({ op: on ? "star" : "unstar", rid: row.getAttribute("data-rid") || "" }, function () {
          star.setAttribute("aria-pressed", String(!on));
        });
        return;
      }
      var kt = t.closest && t.closest(".kidtog");
      if (kt) {
        e.preventDefault();
        var kr = kt.closest(".rs");
        if (kr) kidFold(kr, !kr.classList.contains("kcol"));
        return;
      }
      var settings = t.closest && t.closest(".settings-toggle");
      if (settings && gmenu) { e.preventDefault(); railPopOpen("settings", settings); return; }
      var usage = t.closest && t.closest(".usage-toggle");
      if (usage && usageGutter) { e.preventDefault(); railPopOpen("usage", usage); return; }
      var add = t.closest && t.closest(".radd");
      if (add) {
        e.preventDefault();
        railPopOpen(add.getAttribute("data-add"), add);
        return;
      }
      var forget = t.closest && t.closest(".rforget");
      if (forget) {
        e.preventDefault();
        railForget(forget);
        return;
      }
      var rev = t.closest && t.closest(".rrev");
      if (rev) {
        e.preventDefault();
        railRevive(rev);
        return;
      }
      var fresh = t.closest && t.closest(".rnew");
      if (fresh) {
        e.preventDefault();
        railPopOpen("lead", fresh);
        return;
      }
      var del = t.closest && t.closest(".rdel");
      if (del) {
        e.preventDefault();
        railDelete(del);
        return;
      }
      var ren = t.closest && t.closest(".drren");
      if (ren) {
        e.preventDefault();
        railRename(ren.closest(".rs"));
        return;
      }
      var cp = t.closest && t.closest(".drcopy");
      if (cp) {
        e.preventDefault();
        copyText(cp.getAttribute("data-copy") || "");
        return;
      }
      // The whole bar toggles the workspace's collapse; its toggle button
      // (.wstog) is the keyboard's way to it.
      var bar = t.closest && t.closest(".wsbar");
      if (bar) {
        var ws = bar.closest(".rws");
        if (ws) colSet.toggle(ws.getAttribute("data-ws") || "");
        railMarks();
        return;
      }
      var ref = t.closest && t.closest("#railref");
      if (ref) {
        if (!window.fetch || ref.classList.contains("busy")) return;
        ref.classList.add("busy");
        fetch("/rail/refresh", { method: "POST", credentials: "same-origin" }).then(function () { ref.classList.remove("busy"); }, function () { ref.classList.remove("busy"); });
        return;
      }
      // Picking a session navigates; the draft is saved first and the
      // overlay closes, so a back navigation finds the page as it was. The
      // shell keeps its page (and the memo pad's state) and shows the
      // session's frame (the shell's link clicks, below). Picking the open
      // session again closes its view (the session keeps running) and goes
      // to the index's guide (/#guide, the landing with its how-to); a
      // modified click keeps the link's own behaviour.
      var pick = t.closest && t.closest("a[href]");
      if (pick) {
        var prow = pick.closest(".rs");
        if (prow && rid && prow.getAttribute("data-rid") === rid && !e.button && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey) {
          e.preventDefault();
          if (!shell) { saveDraft(); if (memoPad) memoPad.remember(); }
          railOpen(false);
          go("/#guide");
          return;
        }
        if (!shell) { saveDraft(); if (memoPad) memoPad.remember(); }
        railOpen(false);
      }
    });
    // Left and Right on a focused parent row collapse and expand it (with
    // Alt they are the keyboard path's, not this).
    railEl.addEventListener("keydown", function (e) {
      if ((e.key !== "ArrowLeft" && e.key !== "ArrowRight") || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || e.isComposing) return;
      var t = e.target, row = t.closest && !t.closest("input, textarea") ? t.closest(".rs") : null;
      if (!row || !row.querySelector(".kidtog")) return;
      e.preventDefault();
      kidFold(row, e.key === "ArrowLeft");
    });
    // The keyboard path: Alt and an arrow on a focused row move it
    // (WsRail.keyMove; railMove): Up and Down past the next row of its
    // level, Right under the top-level row above it as its last child,
    // Left out of its parent. One move posts at a time (another key
    // meanwhile does nothing, as two posts in flight could land out of
    // order), and a refused one toasts the route's reason. Alt+Left and
    // Alt+Right are also the browser's Back and Forward (Windows, Linux;
    // under the iframe shell a Back would pop the shell's history), so the
    // row takes them even when they move nothing. Shift+F10 and the
    // ContextMenu key open the row's menu (railRowMenu).
    var RAIL_KEYS = { ArrowUp: "up", ArrowDown: "down", ArrowRight: "in", ArrowLeft: "out" };
    railEl.addEventListener("keydown", function (e) {
      var t = e.target, row = t.closest && !t.closest("input, textarea") ? t.closest(".rs") : null;
      if (!row || e.ctrlKey || e.metaKey || e.isComposing) return;
      if (!e.altKey && ((e.key === "F10" && e.shiftKey) || e.key === "ContextMenu")) {
        e.preventDefault();
        railRowMenu(row, null);
        return;
      }
      var dir = RAIL_KEYS[e.key];
      if (!dir || !e.altKey || e.shiftKey) return;
      e.preventDefault();
      if (railSaving > 0 || rdrag || railEditing()) return;
      var z = WsRail.keyMove(row, dir);
      if (z && z.refusal) toast(wsT(WsRail.layoutRefusal(z.refusal)));
      else if (z) railMove(row, z);
    });
    // The rail list keeps its scroll across navigations.
    var savedRailTop = parseInt(load(RAIL_SCROLL_KEY), 10);
    window.addEventListener("pagehide", function () { var top = railScrollTop(); if (top !== null) save(RAIL_SCROLL_KEY, String(Math.round(top))); });
    railMarks();
    if (isFinite(savedRailTop)) railScrollTo(savedRailTop);
    railOpen(!!(memoPad && memoPad.resume()));
    layoutImport();
  }

  // --- rail drag: sessions in their tree, workspaces among themselves ---
  // A live session row drags by its grip (the gutter left of its harness
  // icon), a parent with its children; a workspace section drags by its
  // bar among the sections. Over a live row the row's zones decide
  // (WsRail.dropZone): the middle half of a valid parent attaches the row
  // as its last child (the parent outlined, no line), its quarters and
  // any other row's halves are a line before or after it at its level,
  // which reorders, attaches among a parent's children, or detaches a
  // child to the top level of its own group. Anywhere else in its own
  // group the drop takes the nearest place among its top-level peers on
  // its side of the stars. A line shows where it lands, clamped to the
  // list's edge when that place is scrolled out of view, and the drop
  // shows at once and posts its op to the rail layout (layoutSave), which
  // the next rail swap shows; a refused op goes back to where it stood,
  // with the reason in a toast. Over a place it cannot land (another
  // workspace or worktree, the other side of the stars, an ended row, too
  // deep) the cursor says no and a hint says why. A press that never moves past
  // RD_SLOP stays a click. The rest of a row is its link and never drags; a
  // touch on a grip drags with no hold, while a touch on a bar drags only
  // after holding still for RD_HOLD, so a swipe still scrolls the list. No
  // drag starts from a button in a bar (+, forget); the bar's own toggle is
  // the bar. A link's native drag would take the pointer away, so it is
  // cancelled. Every rail swap waits from the press until it ends
  // (railGate), and a node replaced anyway is found again by its reply-id
  // or path.
  var RD_SLOP = 5, RD_HOLD = 380, RD_EDGE = 48;
  var rdragged = false, rdLineEl = null, rdHintEl = null, rdKidEl = null, rdRaf = 0;
  function rdPeers(item) {
    var ws = item.classList.contains("rws"), wt = item.classList.contains("rwt");
    return Array.prototype.filter.call(item.parentNode.children, function (c) {
      return ws ? c.classList.contains("rws") : wt ? c.classList.contains("rwt") : c.classList.contains("rs") && !c.classList.contains("ended") && WsRail.starred(c) === WsRail.starred(item);
    });
  }
  function rdLine(box) {
    if (!box) { if (rdLineEl) rdLineEl.hidden = true; return; }
    if (!rdLineEl) {
      rdLineEl = doc.body.appendChild(doc.createElement("div"));
      rdLineEl.className = "rdrop";
      rdLineEl.setAttribute("aria-hidden", "true");
    }
    rdLineEl.style.left = box.left + "px";
    rdLineEl.style.width = box.width + "px";
    rdLineEl.style.top = box.top + "px";
    rdLineEl.hidden = false;
  }
  // rdHint shows why a drop is refused (why, a WsRail.dropRefusal value)
  // beside the pointer, with the not-allowed cursor; an empty why hides it.
  function rdHint(why, x, y) {
    root.classList.toggle("drno", !!why);
    if (!why) { if (rdHintEl) rdHintEl.hidden = true; return; }
    if (!rdHintEl) {
      rdHintEl = doc.body.appendChild(doc.createElement("div"));
      rdHintEl.className = "drhint";
      rdHintEl.setAttribute("role", "status");
    }
    rdHintEl.textContent = wsT(WsRail.layoutRefusal(why));
    rdHintEl.style.left = Math.round(x + 14) + "px";
    rdHintEl.style.top = Math.round(y + 16) + "px";
    rdHintEl.hidden = false;
  }
  // rdKid outlines row, the parent a drop would attach to (the child
  // zone); none clears it.
  function rdKid(row) {
    if (rdKidEl === row) return;
    if (rdKidEl) rdKidEl.classList.remove("rdkid");
    rdKidEl = row || null;
    if (rdKidEl) rdKidEl.classList.add("rdkid");
  }
  // rdBottom is the bottom of row's unit: its children's box while it shows.
  function rdBottom(row) {
    var k = row.nextElementSibling, b = row.getBoundingClientRect().bottom;
    return k && k.classList.contains("rkids") && !row.classList.contains("kcol") ? Math.max(b, k.getBoundingClientRect().bottom) : b;
  }
  // rdHomePeers are a dragged row's top-level peers for a drop over no
  // row: the live rows of its own group's list (WsRail.homeRows), those on
  // its side of the stars for a top-level row.
  function rdHomePeers(item) {
    var box = WsRail.homeRows(item), kid = item.parentNode.classList.contains("rkids");
    return box ? Array.prototype.filter.call(box.children, function (c) {
      return c.classList.contains("rs") && !c.classList.contains("ended") && (kid || WsRail.starred(c) === WsRail.starred(item));
    }) : [];
  }
  // rdRow places a session row's drop (rdOver): d.z the zone and op, or
  // none with no line (no drop there, or one that changes nothing) or the
  // hint (refused).
  function rdRow(d, over, x, y, lr) {
    var row = over && over.closest ? over.closest(".rs") : null, z = null, ref = null;
    // Left of a child row (a grip held at the top-level indent) the
    // pointer is over its indent: the row is the one at its height.
    if (!row && lr) {
      var mid = doc.elementFromPoint(lr.left + lr.width / 2, y);
      row = mid && mid.closest ? mid.closest(".rs") : null;
    }
    if (row && (!railEl.contains(row) || row.classList.contains("ended"))) row = null;
    if (row) {
      var rr = row.getBoundingClientRect();
      z = WsRail.dropZone(WsRail.rowFacts(d.item), WsRail.rowFacts(row), rr.height ? (y - rr.top) / rr.height : 0.5);
      ref = row;
      if (z && z.ref !== row.getAttribute("data-rid")) ref = row.nextElementSibling && row.nextElementSibling.firstElementChild;
    } else {
      var why = WsRail.dropRefusal(d.item, over);
      if (why) z = { refusal: why };
      else {
        var peers = rdHomePeers(d.item), at = peers.length, i;
        for (i = 0; i < peers.length; i++) {
          var pr = peers[i].getBoundingClientRect();
          if (y < pr.top + pr.height / 2) { at = i; break; }
        }
        if (peers.length) {
          ref = peers[Math.min(at, peers.length - 1)];
          var tf = WsRail.rowFacts(ref);
          tf.first = "";
          z = WsRail.dropZone(WsRail.rowFacts(d.item), tf, at < peers.length ? 0 : 1);
        }
      }
    }
    rdHint(z && z.refusal ? z.refusal : "", x, y);
    if (!z || z.refusal || !ref || WsRail.dropNoop(d.item, z)) { rdLine(null); return; }
    d.z = z;
    d.noop = false;
    if (z.zone === "child") { rdKid(ref); rdLine(null); return; }
    var box = ref.getBoundingClientRect(), top = Math.round((z.zone === "after" ? rdBottom(ref) : box.top) - 1);
    if (lr) top = Math.max(Math.round(lr.top), Math.min(Math.round(lr.bottom) - 2, top));
    rdLine({ left: box.left, width: box.width, top: top });
  }
  // rdItem keeps the press on its item: a node a swap replaced anyway is
  // found again (WsRail.refind) and takes the drag's marks. False when it is
  // gone.
  function rdItem(d) {
    if (d.item.isConnected) return true;
    var n = WsRail.refind(railEl, d.key);
    if (!n) return false;
    d.item = n;
    if (d.on) {
      n.classList.add("rdrag");
      try { n.setPointerCapture(d.id); } catch (x) { /* no capture: the moves still land on the document */ }
    }
    return true;
  }
  // rdOver places the drop at the pointer (x, y): a session row's by
  // rdRow, a workspace's before the first section whose middle is below y,
  // else after the last. A drop that would leave the item where it stands
  // shows no line, and one the pointer's place refuses shows the hint
  // instead. Near the list's top or bottom edge the list scrolls, and
  // keeps scrolling while the pointer stays there.
  function rdOver(x, y) {
    var d = rdrag, list = $("rlist");
    if (!d || !d.on) return;
    d.x = x;
    d.y = y;
    var lr = list ? list.getBoundingClientRect() : null;
    d.edge = lr ? (y < lr.top + RD_EDGE ? -1 : y > lr.bottom - RD_EDGE ? 1 : 0) : 0;
    if (d.edge && !rdRaf) rdRaf = requestAnimationFrame(rdScroll);
    d.noop = true;
    d.z = null;
    rdKid(null);
    if (!rdItem(d)) { rdHint(""); rdLine(null); return; }
    if (d.item.classList.contains("rs")) { rdRow(d, doc.elementFromPoint(x, y), x, y, lr); return; }
    var wt = d.item.classList.contains("rwt");
    var why = wt ? WsRail.dropRefusal(d.item, doc.elementFromPoint(x, y)) : "";
    rdHint(why, x, y);
    if (why) { rdLine(null); return; }
    var peers = rdPeers(d.item), at = peers.length, i;
    for (i = 0; i < peers.length; i++) {
      var r = (wt ? peers[i].querySelector(".wthd") : peers[i]).getBoundingClientRect();
      if (y < r.top + r.height / 2) { at = i; break; }
    }
    var me = peers.indexOf(d.item);
    d.noop = at === me || at === me + 1;
    d.ref = at < peers.length ? peers[at] : peers[peers.length - 1];
    d.after = at === peers.length;
    if (d.noop) { rdLine(null); return; }
    var rr = d.ref.getBoundingClientRect(), top = d.after ? rr.bottom : rr.top;
    if (!d.after && at > 0) top = (peers[at - 1].getBoundingClientRect().bottom + rr.top) / 2;
    top = Math.round(top - 1);
    if (lr) top = Math.max(Math.round(lr.top), Math.min(Math.round(lr.bottom) - 2, top));
    rdLine({ left: rr.left, width: rr.width, top: top });
  }
  function rdScroll() {
    rdRaf = 0;
    var d = rdrag, list = $("rlist");
    if (!d || !d.on || !d.edge || !list) return;
    var was = list.scrollTop;
    list.scrollTop += d.edge * 8;
    if (list.scrollTop === was) return;
    rdOver(d.x, d.y);
  }
  function rdStart(e) {
    if (rdrag || !e.isPrimary || (e.button !== undefined && e.button !== 0)) return;
    var t = e.target, item, grip = t.closest && t.closest(".drgrip");
    if (!t.closest) return;
    if (grip) {
      item = grip.closest(".rs");
      if (!item || !railEl.contains(item) || item.classList.contains("ended") || item.parentNode.closest(".rended")) return;
    } else {
      var b = t.closest("button"), hd = t.closest(".wthd"), bar = hd || t.closest(".wsbar");
      if ((b && (hd || !b.classList.contains("wstog"))) || !bar || !railEl.contains(bar)) return;
      item = bar.closest(hd ? ".rwt" : ".rws");
      if (!item || rdPeers(item).length < 2) return;
    }
    var touch = e.pointerType === "touch";
    rdrag = { item: item, key: WsRail.itemKey(item), id: e.pointerId, x0: e.clientX, y0: e.clientY, x: e.clientX, y: e.clientY, on: false, touch: touch, hold: touch && !grip, timer: 0, ref: null, after: false, z: null, noop: true, edge: 0 };
    if (rdrag.hold) rdrag.timer = setTimeout(function () { if (rdrag && !rdrag.on) rdBegin(); }, RD_HOLD);
  }
  function rdBegin() {
    var d = rdrag;
    if (!rdItem(d)) { rdEnd(null); return; }
    d.on = true;
    try { d.item.setPointerCapture(d.id); } catch (x) { /* no capture: the moves still land on the document */ }
    d.item.classList.add("rdrag");
    root.classList.add("rdragging");
    var sel = window.getSelection && window.getSelection();
    if (sel && sel.removeAllRanges) sel.removeAllRanges();
    rdOver(d.x, d.y);
  }
  function rdMove(e) {
    var d = rdrag;
    if (!d || e.pointerId !== d.id) return;
    // A mouse move with the button up is a release this page never heard.
    if (e.pointerType === "mouse" && e.buttons !== undefined && (e.buttons & 1) === 0) { rdEnd(null); return; }
    if (!d.on) {
      if (Math.abs(e.clientX - d.x0) <= RD_SLOP && Math.abs(e.clientY - d.y0) <= RD_SLOP) return;
      // A touch on a bar that moves before its hold is a scroll.
      if (d.hold) { rdEnd(null); return; }
      d.x = e.clientX;
      d.y = e.clientY;
      rdBegin();
      if (!rdrag) return;
    }
    e.preventDefault();
    rdOver(e.clientX, e.clientY);
  }
  // rdEnd ends the press or drag; only a release drops (a cancel, or a
  // release this page never heard, leaves the rail as it stood). A drop
  // moves the item at once and posts its op (rdDrop for a session row,
  // rdSave for a workspace). A drag's click is
  // swallowed (the capture listener below), and the rail swap held since
  // the press lands after it, or after the move's answer.
  function rdEnd(e) {
    var d = rdrag;
    if (!d || (e && e.pointerId !== d.id)) return;
    if (d.on && !d.item.isConnected) rdOver(d.x, d.y);
    rdrag = null;
    clearTimeout(d.timer);
    if (d.on) {
      d.item.classList.remove("rdrag");
      root.classList.remove("rdragging");
      rdLine(null);
      rdHint("");
      rdKid(null);
      rdragged = true;
      setTimeout(function () { rdragged = false; }, 0);
      if (e && e.type === "pointerup" && !d.noop && d.item.isConnected) {
        if (d.z) rdDrop(d.item, d.z); else rdSave(d.item, d.ref, d.after);
      }
      railMarks();
    }
    // The press's click lands first, on the nodes it pressed.
    setTimeout(railRelease, 0);
  }
  // rdDrop shows a session row's drop (z, from WsRail.dropZone) at once
  // (WsRail.moveRow) and posts its op, with an undo that puts the rows back.
  function rdDrop(item, z) {
    var ref = WsRail.refind(railEl, { cls: "rs", attr: "data-rid", val: z.ref });
    if (!ref) return;
    var undo = WsRail.moveRow(item, ref, z.zone, function () {
      var b = doc.createElement("div");
      b.className = "rkids";
      return b;
    });
    layoutSave(z.op, function () { undo(); railMarks(); });
  }
  // rdSave drops a workspace or complete worktree group next to ref and
  // posts wsmove/wtmove, with an undo that restores the whole subtree.
  function rdSave(item, ref, after) {
    var box = item.parentNode, next = item.nextSibling;
    if (!WsRail.drop(item, ref, after)) return;
    var wt = item.classList.contains("rwt"), attr = wt ? "data-g" : "data-ws";
    var body = { op: wt ? "wtmove" : "wsmove", path: item.getAttribute(attr) || "" };
    if (wt) body.workspace = item.closest(".rws").getAttribute("data-ws") || "";
    body[after ? "after" : "before"] = ref.getAttribute(attr) || "";
    layoutSave(body, function () {
      if (box.isConnected && item.isConnected) box.insertBefore(item, next && next.parentNode === box ? next : null);
      railMarks();
    });
  }
  if (railEl && WsRail) {
    function openWorktree(e) {
      if (!root.hasAttribute("data-sysopen") || e.defaultPrevented || rdragged || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
      if (e.type === "keydown" && e.key !== "Enter") return;
      var hd = e.target.closest && e.target.closest(".wthd");
      if (!hd || e.target.closest("button, a") || (e.type === "keydown" && e.target !== hd)) return;
      var path = hd.closest(".rwt").getAttribute("data-g").replace(/\\/g, "/").replace(/^\//, "");
      go("/w/" + path.split("/").map(encodeURIComponent).join("/").replace(/^([A-Za-z])%3A/, "$1:"));
      e.preventDefault();
    }
    railEl.addEventListener("click", openWorktree);
    railEl.addEventListener("keydown", openWorktree);
    railEl.addEventListener("pointerdown", rdStart);
    doc.addEventListener("pointermove", rdMove);
    doc.addEventListener("pointerup", rdEnd);
    doc.addEventListener("pointercancel", rdEnd);
    // A press the window loses (another window took the focus) ends there,
    // so a release it never hears does not hold the rail's swaps.
    window.addEventListener("blur", function () { rdEnd(null); });
    railEl.addEventListener("click", function (e) { if (rdragged) { e.preventDefault(); e.stopPropagation(); } }, true);
    railEl.addEventListener("dragstart", function (e) { if (e.target.closest && e.target.closest(".rs, .wsbar, .wthd")) e.preventDefault(); });
    // A dragged touch is a drag, not a scroll or the long-press menu.
    railEl.addEventListener("touchmove", function (e) { if (rdrag && rdrag.on) e.preventDefault(); }, { passive: false });
    railEl.addEventListener("contextmenu", function (e) { if (rdrag && rdrag.touch) e.preventDefault(); });
  }

