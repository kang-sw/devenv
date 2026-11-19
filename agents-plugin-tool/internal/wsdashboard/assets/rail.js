(function () {
  // The session rail's browser-side state, window.WsRail, for the session
  // page's script (dashboard.js) and the iframe shell's first paint:
  // per-session drafts, the collapsed workspaces, parents, and ended folds
  // left open, and the rail's client-side marks (the viewed session's row,
  // the revive entry in flight), with the drag's refusal, re-find, and
  // swap-hold rules, the tree's drop zones (which the keyboard path's moves
  // and the context menu's "move under" read too), and the rail layout
  // route's answers. The daemon keeps the rows' order and tree, the stars,
  // and the workspace order (its rail layout, POST /rail/layout) and renders
  // the rail in that order; the page only reads a row's star and place from
  // it. The drafts and the collapsed and open sets are in localStorage
  // under ws.dashboard., through the store the caller passes ({ load(key),
  // save(key, value or null) }, which must not throw), so the page works
  // the same with storage absent; the retired local order and stars are
  // read there once, for the layout's import. The DOM helpers touch only
  // the elements they are given; the revive route and hand-off notice, the
  // popover and delete modal helpers at the end, and the approval card's
  // input preview after them, are pure.
  var DRAFT = "ws.dashboard.draft.", DRAFTS = "ws.dashboard.drafts";
  var STARS = "ws.dashboard.stars", COLLAPSED = "ws.dashboard.railcol";
  var ORDER = "ws.dashboard.railorder", WSORDER = "ws.dashboard.wsorder";
  var ENDED = "ws.dashboard.railended";
  // DRAFT_CAP bounds the sessions whose drafts are kept; the oldest saved
  // goes first.
  var DRAFT_CAP = 50;

  function parse(raw, def) {
    if (raw === null || raw === undefined) return def;
    try { var v = JSON.parse(raw); return v === null || v === undefined ? def : v; } catch (e) { return def; }
  }
  function strings(v) { return Array.isArray(v) ? v.filter(function (x) { return typeof x === "string"; }) : []; }
  function keys(o) { return o && typeof o === "object" ? Object.keys(o) : []; }

  // Admission drafts keep the original observation revision until the user
  // explicitly clears/reselects. A reload or server render is not fresh consent.
  function admissionDrafts(store) {
    var key = "ws.dashboard.worktree-admission.drafts";
    return {
      load: function () {
        var v = parse(store.load(key), []);
        return Array.isArray(v) ? v.filter(function (s) {
          return s && typeof s.repository === "string" && typeof s.path === "string" && typeof s.revision === "string" &&
            (s.action === "approve" || s.action === "reject" || s.action === "undo");
        }) : [];
      },
      save: function (v) { store.save(key, v.length ? JSON.stringify(v) : null); }
    };
  }

  // A draft is one session's unsent input and view state: text and
  // mentions ({start, end, text}) of the composer, sel its selection
  // ([start, end]), cmd its slash command selection ({name, args}: args a
  // string when the capsule holds them, else null), answers the board answers by item (readAnswers' shape,
  // topic notes among them), chips the answer chips, bmentions the board
  // mentions ({topic, id?, text}), wpicks and wconsoles the web card's
  // picked elements ({url, selector, text}) and console blocks ({url,
  // lines}), refs the images that
  // reference written attachment files ({ref, name, n}), lost the numbers
  // of the pasted images it could not keep, open the open details by key, and tab
  // the tab shown. emptyDraft reports one that holds nothing to restore.
  function emptyDraft(d) {
    if (!d || typeof d !== "object") return true;
    return !d.text && !d.cmd && !(d.chips && d.chips.length) && !(d.bmentions && d.bmentions.length) && !(d.closeRequests && d.closeRequests.length) &&
      !(d.wpicks && d.wpicks.length) && !(d.wconsoles && d.wconsoles.length) && !keys(d.answers).length && !(d.refs && d.refs.length) &&
      !(d.lost && d.lost.length) && !keys(d.open).length && (!d.tab || d.tab === "convo");
  }

  // drafts is the draft store: load(rid), save(rid, draft) (an empty draft
  // clears), and clear(rid). Saves return the storage adapter's write result
  // (false means failure). Beyond cap sessions the oldest saved is dropped.
  function drafts(store, cap) {
    cap = cap || DRAFT_CAP;
    function index() { return strings(parse(store.load(DRAFTS), [])); }
    function clear(rid) {
      var saved = store.save(DRAFT + rid, null);
      var ids = index(), rest = ids.filter(function (x) { return x !== rid; });
      if (rest.length !== ids.length) store.save(DRAFTS, rest.length ? JSON.stringify(rest) : null);
      return saved;
    }
    return {
      load: function (rid) {
        var d = parse(store.load(DRAFT + rid), null);
        return d && typeof d === "object" && !Array.isArray(d) ? d : null;
      },
      save: function (rid, d) {
        if (emptyDraft(d)) return clear(rid);
        var ids = index().filter(function (x) { return x !== rid; });
        ids.push(rid);
        while (ids.length > cap) store.save(DRAFT + ids.shift(), null);
        store.save(DRAFTS, JSON.stringify(ids));
        return store.save(DRAFT + rid, JSON.stringify(d));
      },
      clear: clear
    };
  }

  // stripMarkers removes each image marker ("[Image #n]" and the space after
  // it) naming one of nums from text, moving the mentions after a removed
  // marker with the text and dropping one a removal cuts into.
  function stripMarkers(text, mentions, nums) {
    var want = {};
    (nums || []).forEach(function (n) { want[n] = true; });
    var re = /\[Image #([1-9][0-9]{0,5})\] ?/g, m, cuts = [];
    while ((m = re.exec(text))) if (want[+m[1]]) cuts.push([m.index, m.index + m[0].length]);
    if (!cuts.length) return { text: text, mentions: (mentions || []).slice() };
    var out = "", at = 0;
    cuts.forEach(function (c) { out += text.slice(at, c[0]); at = c[1]; });
    out += text.slice(at);
    var kept = [];
    (mentions || []).forEach(function (mn) {
      var shift = 0, cut = false;
      cuts.forEach(function (c) {
        if (c[1] <= mn.start) shift += c[1] - c[0];
        else if (c[0] < mn.end) cut = true;
      });
      if (!cut) kept.push({ start: mn.start - shift, end: mn.end - shift, text: mn.text });
    });
    return { text: out, mentions: kept };
  }

  // A set of strings kept as one JSON array: has(x), toggle(x) returning
  // the new state, put(x, on), list().
  function stringSet(store, key) {
    function list() { return strings(parse(store.load(key), [])); }
    function put(x, on) {
      var l = list(), i = l.indexOf(x);
      if ((i >= 0) === !!on) return;
      if (i >= 0) l.splice(i, 1); else l.push(x);
      store.save(key, l.length ? JSON.stringify(l) : null);
    }
    return {
      list: list,
      has: function (x) { return list().indexOf(x) >= 0; },
      toggle: function (x) {
        var on = list().indexOf(x) < 0;
        put(x, on);
        return on;
      },
      put: put
    };
  }
  // collapsed is the collapsed workspaces' paths; endedOpen the fold rows'
  // open lists, by FOLD_KEYS (marks).
  function collapsed(store) { return stringSet(store, COLLAPSED); }
  function endedOpen(store) { return stringSet(store, ENDED); }

  // LEGACY are the retired local layout's keys (the starred sessions, the
  // sessions' drag order by reply-id, the workspaces' by path), and what
  // the import op calls each.
  var LEGACY = [[ORDER, "order"], [STARS, "stars"], [WSORDER, "wsorder"]];
  function legacyClear(store) { LEGACY.forEach(function (k) { store.save(k[0], null); }); }
  // legacyImport is the rail layout's import of the retired local layout
  // held in store, for a page whose rail reports the server layout empty
  // (empty); null when there is nothing to import. A server layout that
  // holds an entry wins: the local keys are cleared, never imported.
  function legacyImport(store, empty) {
    var body = { op: "import" }, any = false;
    LEGACY.forEach(function (k) {
      var v = strings(parse(store.load(k[0]), []));
      body[k[1]] = v;
      if (store.load(k[0]) !== null) any = true;
    });
    if (!any) return null;
    if (!empty) { legacyClear(store); return null; }
    return body;
  }
  // legacyDone takes the import's answer: an import applied, or one the
  // server did not apply (another browser's came first), clears the local
  // keys; any other answer (retry: the daemon has not read its registry
  // yet; an unreachable daemon) keeps them for a later load.
  function legacyDone(store, answer) {
    var a = answer || {};
    if (a.applied === true || a.reason === "not_applied") legacyClear(store);
  }

  // LAYOUT_REFUSALS maps the rail layout route's refusal reasons (the
  // layout* codes in rail_layout.go) to their strings; the drag's own
  // refusals (dropRefusal) and later the tree's and the keyboard's name
  // the same codes. layoutRefusal is a reason's string key, an unknown or
  // missing reason's the generic one.
  var LAYOUT_REFUSALS = {
    unknown: "js.rail.layout.unknown", ended: "js.rail.layout.ended", self: "js.rail.layout.self",
    depth: "js.rail.layout.depth", workspace: "js.rail.layout.workspace", group: "js.rail.layout.group",
    level: "js.rail.layout.level", stars: "js.rail.layout.stars", child: "js.rail.layout.child",
    toplevel: "js.rail.layout.toplevel", invalid: "js.rail.layout.invalid", retry: "js.rail.layout.retry",
    store: "js.rail.layout.store", hiring: "js.rail.layout.hiring"
  };
  function layoutRefusal(reason) {
    return LAYOUT_REFUSALS.hasOwnProperty(reason) ? LAYOUT_REFUSALS[reason] : LAYOUT_REFUSALS.invalid;
  }
  // staleRail reports a rail render (its HTML) older than the layout
  // generation min: its list's data-lgen is below it. A render without one
  // is never stale.
  function staleRail(html, min) {
    var m = /\bdata-lgen="([0-9]+)"/.exec(typeof html === "string" ? html : html && html.shell ? html.shell.join("") : "");
    return !!m && +m[1] < min;
  }

  function hasClass(el, c) { return (" " + (el.className || "") + " ").indexOf(" " + c + " ") >= 0; }
  // starred reads the favorite state rendered on the session row.
  function starred(el) {
    return !!el && el.getAttribute("data-star") === "1";
  }

  // drop moves item (a live session row, worktree group or workspace section) next to
  // ref, a sibling of its kind, before it or (after) after it, and reports
  // whether it moved. It refuses a ref in another list (a session never
  // leaves its own group), a ref of another kind or on the other side of
  // the stars, an ended row, and the item itself.
  function drop(item, ref, after) {
    if (!item || !ref || item === ref || !item.parentNode || ref.parentNode !== item.parentNode) return false;
    var kind = hasClass(item, "rws") ? "rws" : hasClass(item, "rwt") ? "rwt" : "rs";
    if (!hasClass(ref, kind) || hasClass(item, "ended") || hasClass(ref, "ended") || (kind === "rs" && starred(item) !== starred(ref))) return false;
    item.parentNode.insertBefore(item, after ? ref.nextSibling : ref);
    return true;
  }

  // up is el or its nearest ancestor of class c, or null.
  function up(el, c) {
    for (var n = el; n; n = n.parentNode) if (hasClass(n, c)) return n;
    return null;
  }
  // dropRefusal names why a dragged session row cannot land where the
  // pointer is (over, the element under it), as a layout refusal reason
  // (LAYOUT_REFUSALS): "workspace" over another workspace, "group" over
  // another worktree than its own group's (a child's own is where it would
  // go top-level, not its parent's), "stars" over a live row of its own
  // list on the other side of the stars; "" anywhere else (its own group,
  // a header, the ended fold, or outside the list, where the drop takes the
  // nearest place). A workspace section is never refused.
  function dropRefusal(item, over) {
    if (!item || !over || hasClass(item, "rws")) return "";
    var ws = up(over, "rws");
    if (!ws) return "";
    if (ws !== up(item, "rws")) return "workspace";
    if (hasClass(item, "rwt")) return "";
    var wt = up(over, "rwt");
    if (wt && wt !== homeWt(item)) return "group";
    var row = up(over, "rs");
    if (row && row !== item && row.parentNode === item.parentNode && !hasClass(row, "ended") && starred(row) !== starred(item)) return "stars";
    return "";
  }

  // The tree: a child row sits in its parent's children box (.rkids, the
  // parent row's next sibling, data-parent the parent's reply-id) and
  // carries data-g, the path of its own worktree group; a top-level row's
  // group is its worktree's (.rwt data-g), or its plain-folder workspace's
  // path. data-kids marks a stored parent (of a live or an ended child),
  // which never becomes a child; kcol marks a parent the page shows
  // collapsed (kidsCollapsed, the parents' reply-ids, per browser).
  var KIDS_COLLAPSED = "ws.dashboard.railkids";
  function kidsCollapsed(store) { return stringSet(store, KIDS_COLLAPSED); }
  function attr(el, k) { return el && el.getAttribute ? el.getAttribute(k) : null; }
  function kidsOf(row) {
    var k = row && row.nextSibling;
    return k && hasClass(k, "rkids") ? k : null;
  }
  function rowGroup(row) {
    var g = attr(row, "data-g");
    if (g !== null) return g;
    var wt = up(row, "rwt"), ws = up(row, "rws");
    return wt ? attr(wt, "data-g") || "" : attr(ws, "data-ws") || "";
  }
  // rowFacts is what a drop reads of a session row: its reply-id, its
  // workspace and group, live, its parent (a child's), kids (a stored
  // parent), star (its rendered star), and first, its first child while
  // its children show.
  function rowFacts(row) {
    var box = row.parentNode, kids = kidsOf(row), ws = up(row, "rws");
    return {
      rid: attr(row, "data-rid") || "", ws: attr(ws, "data-ws") || "", group: rowGroup(row),
      live: !hasClass(row, "ended"), parent: box && hasClass(box, "rkids") ? attr(box, "data-parent") || "" : "",
      kids: attr(row, "data-kids") === "1", star: starred(row),
      first: kids && !hasClass(row, "kcol") && kids.children.length ? attr(kids.children[0], "data-rid") || "" : ""
    };
  }
  // dropZone is where the dragged session row d lands over the row t (both
  // rowFacts) with the pointer at frac of t's height (0 its top, 1 its
  // bottom): { zone, ref, op }, op the layout op the drop posts and ref the
  // reply-id the zone is drawn against: the line before or after that row,
  // or (zone "child") the parent outlined. A place the route would refuse
  // is { refusal }, the route's reason code (LAYOUT_REFUSALS); d itself
  // and its own children are no target (null). The middle half of a valid
  // parent (top-level, live, d's workspace, d no parent itself) attaches d
  // as its last child, its quarters are before and after; any other row
  // splits in halves. A line among a parent's children attaches d there
  // (among its own siblings it moves); a line at the top level moves a
  // top-level row (its own group, its side of the stars) or detaches a
  // child (its own group only; the route checks no stars for a detach).
  // The line after a parent whose children show is the line before its
  // first child. An orphan shows top-level, so it is a target as one.
  // With halves every row splits in halves (no child zone): the pointer
  // drag's line, whose child zone arms only after a dwell.
  function dropZone(d, t, frac, halves) {
    if (!d || !t || !d.rid || t.rid === d.rid || t.parent === d.rid) return null;
    var parentOK = !t.parent && t.live && t.ws === d.ws && !d.kids;
    var zone = parentOK && !halves ? (frac < 0.25 ? "before" : frac > 0.75 ? "after" : "child") : (frac < 0.5 ? "before" : "after");
    if (zone === "child") return { zone: zone, ref: t.rid, op: { op: "attach", rid: d.rid, parent: t.rid } };
    if (zone === "after" && parentOK && t.first) {
      return dropZone(d, { rid: t.first, ws: t.ws, group: "", live: true, parent: t.rid, kids: false, star: false, first: "" }, 0);
    }
    if (!t.live) return { refusal: "ended" };
    if (t.ws !== d.ws) return { refusal: "workspace" };
    var op;
    if (t.parent) {
      if (d.kids) return { refusal: "depth" };
      op = d.parent === t.parent ? { op: "move", rid: d.rid } : { op: "attach", rid: d.rid, parent: t.parent };
    } else {
      if (t.group !== d.group) return { refusal: "group" };
      if (!d.parent && t.star !== d.star) return { refusal: "stars" };
      op = { op: d.parent ? "detach" : "move", rid: d.rid };
    }
    op[zone] = t.rid;
    return { zone: zone, ref: t.rid, op: op };
  }
  function rowsIn(box) {
    return box ? Array.prototype.filter.call(box.children, function (c) { return hasClass(c, "rs"); }) : [];
  }
  // dropNoop reports a drop (z, from dropZone) that leaves the dragged row
  // item where it stands: a move right next to it, on the side it is on, or
  // an attach to its own parent while it is already the last child.
  function dropNoop(item, z) {
    var o = z && z.op;
    if (!o) return true;
    var rows = rowsIn(item.parentNode), i = rows.indexOf(item);
    if (o.op === "attach" && !o.before && !o.after) {
      return hasClass(item.parentNode, "rkids") && attr(item.parentNode, "data-parent") === o.parent && i === rows.length - 1;
    }
    if (o.op !== "move") return false;
    var j = -1;
    rows.forEach(function (r, k) { if (attr(r, "data-rid") === (o.before || o.after)) j = k; });
    return j >= 0 && (o.before ? j === i + 1 : j === i - 1);
  }
  // moveRow shows a tree drop at once: item (with its children's box) goes
  // before ref, after ref and its children (zone "after"), or last into
  // ref's children's box (zone "child"), which make() makes when ref has
  // none; a child keeps its own group (data-g), and a children's box it
  // leaves empty goes. It returns the undo, which puts every node back.
  function moveRow(item, ref, zone, make) {
    var undo = [], from = item.parentNode, kids = kidsOf(item), group = rowGroup(item), had = attr(item, "data-g");
    function put(n, box, next) {
      var p = n.parentNode, s = n.nextSibling;
      undo.push(function () { if (p) p.insertBefore(n, s); else if (n.parentNode) n.parentNode.removeChild(n); });
      box.insertBefore(n, next);
    }
    var box = ref.parentNode, next = ref;
    if (zone === "after") next = (kidsOf(ref) || ref).nextSibling;
    else if (zone === "child") {
      box = kidsOf(ref);
      if (!box) {
        box = make();
        box.setAttribute("data-parent", attr(ref, "data-rid") || "");
        put(box, ref.parentNode, ref.nextSibling);
      }
      next = null;
    }
    if (next === item) next = (kids || item).nextSibling;
    put(item, box, next);
    if (kids) put(kids, box, next);
    if (hasClass(box, "rkids")) {
      undo.push(function () { if (had === null) item.removeAttribute("data-g"); else item.setAttribute("data-g", had); });
      item.setAttribute("data-g", group);
    }
    if (from && from !== box && hasClass(from, "rkids") && !from.children.length) {
      var fp = from.parentNode, fs = from.nextSibling;
      undo.push(function () { fp.insertBefore(from, fs); });
      fp.removeChild(from);
    }
    return function () { for (var i = undo.length - 1; i >= 0; i--) undo[i](); };
  }
  // homeWt is the worktree a session row's own group renders in: a
  // top-level row's own, a child's by its data-g; null in a plain-folder
  // workspace.
  function homeWt(item) {
    var box = item.parentNode;
    if (!box || !hasClass(box, "rkids")) return up(item, "rwt");
    var g = attr(item, "data-g") || "";
    return find(up(item, "rws"), function (n) { return hasClass(n, "rwt") && attr(n, "data-g") === g; });
  }
  // homeRows is the list a dragged row's top-level peers stand in, for a
  // drop over no row: a top-level row's own, a child's own group's (null
  // when that group lists no row).
  function homeRows(item) {
    var box = item.parentNode;
    if (!box || !hasClass(box, "rkids")) return box;
    var home = homeWt(item) || find(up(item, "rws"), function (n) { return hasClass(n, "wsbody"); });
    return home ? find(home, function (n) { return hasClass(n, "rsessions"); }, true) : null;
  }
  // find is the first node under root that test passes, depth first; with
  // top, only root's own children are tried.
  function find(root, test, top) {
    var kids = (root && root.children) || [];
    for (var i = 0; i < kids.length; i++) {
      if (test(kids[i])) return kids[i];
      var deep = top ? null : find(kids[i], test);
      if (deep) return deep;
    }
    return null;
  }
  function liveRows(box) { return rowsIn(box).filter(function (r) { return !hasClass(r, "ended"); }); }
  function rowOf(root, rid) { return root && rid ? refind(root, { cls: "rs", attr: "data-rid", val: rid }) : null; }

  // keyMove is the keyboard path's op for the session row item (Alt and an
  // arrow): "up" and "down" reorder it past the next live row of its level
  // (its group's top-level rows, or its parent's children), "in" attaches
  // it to the previous top-level row as its last child, and "out" detaches
  // a child, which the route lands right after its old parent when that is
  // in the child's own group, else at the head of its own group (the page
  // shows it before that group's first unstarred row). It answers
  // dropZone's { zone, ref, op }, read by dropZone's rules where a place is
  // involved (zone and ref empty when the page has no row to show the
  // detach against: the route still places it), or { refusal } with the
  // route's reason for an op it would refuse (an ended row moves no way,
  // a child or a parent never goes one level deeper, a top-level row has
  // no parent to leave); null when there is no row to move past.
  function keyMove(item, dir) {
    var d = rowFacts(item);
    if (!d.rid) return null;
    if (!d.live) return { refusal: "ended" };
    var rows = liveRows(item.parentNode), i = rows.indexOf(item), t;
    if (dir === "up" || dir === "down") {
      t = rows[dir === "up" ? i - 1 : i + 1];
      if (!t) return null;
      var tf = rowFacts(t);
      tf.first = "";
      return dropZone(d, tf, dir === "up" ? 0 : 1);
    }
    if (dir === "in") {
      if (d.parent || d.kids) return { refusal: "depth" };
      t = rows[i - 1];
      return t ? dropZone(d, rowFacts(t), 0.5) : null;
    }
    if (dir !== "out") return null;
    if (!d.parent) return { refusal: "toplevel" };
    var op = { op: "detach", rid: d.rid }, p = rowOf(up(item, "rws"), d.parent);
    if (p && rowGroup(p) === d.group) return { zone: "after", ref: d.parent, op: op };
    var home = liveRows(homeRows(item)), first = home.filter(function (r) { return !starred(r); })[0];
    if (first) return { zone: "before", ref: attr(first, "data-rid") || "", op: op };
    if (home.length) return { zone: "after", ref: attr(home[home.length - 1], "data-rid") || "", op: op };
    return { zone: "", ref: "", op: op };
  }
  // underRows is where the context menu's "move under" can put the session
  // row item: { rows, refusal }, rows every row of its workspace whose
  // middle would take it as a child (dropZone; its own parent aside) in
  // the rail's order, refusal the route's reason when none can (an ended
  // row, or a parent, which never becomes a child), else "".
  function underRows(item) {
    var d = rowFacts(item), out = [];
    if (!d.live) return { rows: out, refusal: "ended" };
    if (d.kids) return { rows: out, refusal: "depth" };
    (function walk(n) {
      var kids = (n && n.children) || [];
      for (var i = 0; i < kids.length; i++) {
        var k = kids[i];
        if (hasClass(k, "rs")) {
          var z = k !== item ? dropZone(d, rowFacts(k), 0.5) : null;
          if (z && z.zone === "child" && z.ref !== d.parent) out.push(k);
        } else walk(k);
      }
    })(up(item, "rws"));
    return { rows: out, refusal: "" };
  }

  // itemKey names a dragged item, a session row by reply-id or a workspace
  // section by path, so refind can find the node a swap put in its place.
  function itemKey(el) {
    var ws = hasClass(el, "rws"), wt = hasClass(el, "rwt"), attr = ws ? "data-ws" : wt ? "data-g" : "data-rid";
    var key = { cls: ws ? "rws" : wt ? "rwt" : "rs", attr: attr, val: el.getAttribute(attr) || "" };
    if (wt) key.ws = (up(el, "rws") || el).getAttribute("data-ws") || "";
    return key;
  }
  // refind is the element under box that key names, or null.
  function refind(box, key) {
    var kids = box.children || [];
    for (var i = 0; i < kids.length; i++) {
      var k = kids[i];
      if (hasClass(k, key.cls) && k.getAttribute(key.attr) === key.val &&
          (key.ws === undefined || attr(up(k, "rws"), "data-ws") === key.ws)) return k;
      var deep = refind(k, key);
      if (deep) return deep;
    }
    return null;
  }

  // swapGate holds the rail's swaps while busy() reports a press, a drag,
  // or an open rename on it, since a swap replaces every row node: offer(s)
  // reports whether it held s (the latest offer replaces an earlier one),
  // release() hands the held swap back once busy() clears (null while busy
  // or with none held), and forget() drops it (its hash turned current).
  function swapGate(busy) {
    var held = null;
    return {
      offer: function (s) {
        if (!busy()) return false;
        held = s;
        return true;
      },
      release: function () {
        if (busy() || !held) return null;
        var s = held;
        held = null;
        return s;
      },
      forget: function () { held = null; }
    };
  }

  // markCurrent marks the row of the viewed session rid among rows: the
  // .cur class and aria-current on its link; any other row loses both.
  function markCurrent(rows, rid) {
    rows.forEach(function (r) {
      var cur = r.getAttribute("data-rid") === rid, a = r.querySelector(".rl");
      if (cur) r.classList.add("cur"); else r.classList.remove("cur");
      if (a) { if (cur) a.setAttribute("aria-current", "page"); else a.removeAttribute("aria-current"); }
    });
  }

  // attention is tab-memory only, independent of the persistent question
  // dot. First observations (including a reconnect's full rail) baseline
  // silently. Replaced rows retain the flag but never replay its flash.
  function attention() {
    var seen = Object.create(null);
    function asks(row) {
      var raw = row.getAttribute("data-asks");
      return raw === null ? null : new Set(raw.trim().split(/\s+/).filter(Boolean));
    }
    function observe(rows, rid, baseline) {
      var kept = Object.create(null);
      rows.forEach(function (row) {
        var id = row.getAttribute("data-rid"), ids = asks(row), prev = seen[id];
        if (!ids) return; // Old server: neither baseline nor flag.
        var fresh = !baseline && !!prev && !prev.flagged && id !== rid && Array.from(ids).some(function (n) { return !prev.asks.has(n); });
        kept[id] = { asks: ids, flagged: id !== rid && (fresh || !!(prev && prev.flagged)), flash: fresh || !!(prev && prev.flash) };
      });
      seen = kept;
    }
    function mark(rows, rid, hint) {
      rows.forEach(function (row) {
        var id = row.getAttribute("data-rid"), ids = asks(row), prev = seen[id];
        if (!ids) { delete seen[id]; prev = null; }
        else if (!prev) prev = seen[id] = { asks: ids, flagged: false, flash: false };
        if (prev && id === rid) { prev.flagged = false; prev.flash = false; }
        var on = !!prev && prev.flagged, link = row.querySelector(".rl");
        // A running flash stays on its original node. A server replacement
        // has no .attn class, so an existing flag goes straight to settled.
        if (!on || !row.classList.contains("attn")) row.classList.remove("attn-flash");
        row.classList.toggle("attn", on);
        if (on && prev.flash) { row.classList.add("attn-flash"); prev.flash = false; }
        if (link) { if (on) link.setAttribute("title", hint); else link.removeAttribute("title"); }
      });
    }
    return {
      observe: observe,
      mark: mark,
      sync: function (rows, rid, hint) { observe(rows, rid); mark(rows, rid, hint); }
    };
  }

  // state is the rail's stored sets over store, as marks reads them: the
  // collapsed workspaces and parents and the ended folds left open. The
  // rows' order and stars come rendered from the daemon's layout.
  function state(store) {
    return { collapsed: collapsed(store), kids: kidsCollapsed(store), ended: endedOpen(store) };
  }
  // marks applies the stored marks (s, state) to the rendered rail region
  // r: marks the viewed session's row (rid), collapses the collapsed
  // workspaces and parents, and opens the ended folds left open. The page
  // script runs it after every rail swap; the iframe shell also runs it as
  // soon as the rail is parsed, before the first paint, so a reload never
  // shows a fold the browser keeps closed open.
  function marks(r, s, rid) {
    function all(sel) { return Array.prototype.slice.call(r.querySelectorAll(sel)); }
    markCurrent(all(".rs"), rid);
    all(".rws").forEach(function (ws) {
      var col = s.collapsed.has(ws.getAttribute("data-ws") || ""), tog = ws.querySelector(".wstog");
      if (col) ws.classList.add("col"); else ws.classList.remove("col");
      if (tog) tog.setAttribute("aria-expanded", String(!col));
    });
    all(".kidtog").forEach(function (tog) {
      var row = tog.closest(".rs"), col = !!row && s.kids.has(row.getAttribute("data-rid") || "");
      if (row) { if (col) row.classList.add("kcol"); else row.classList.remove("kcol"); }
      tog.setAttribute("aria-expanded", String(!col));
    });
    all(".rfold[data-key]").forEach(function (f) {
      var key = f.getAttribute("data-key") || "";
      foldShow(f, s.ended.has(FOLD_KEYS.arch + key) ? "arch" : s.ended.has(FOLD_KEYS.del + key) ? "del" : "");
    });
  }
  // A group's fold row (.rfold, data-key its path) has two halves, archived
  // ("arch") and ended ("del"), acting as one tri-state radio: none, one,
  // or the other list open. The pick is kept in the ended-fold set under
  // FOLD_KEYS' prefix and the path; "rended:" is the key the single ended
  // fold kept before the split, so a fold left open stays open.
  var FOLD_KEYS = { arch: "rarch:", del: "rended:" };
  // foldShow opens the fold row f's list open ("arch", "del", or "" for
  // none); a disabled half (no rows) never opens.
  function foldShow(f, open) {
    var halves = Array.prototype.slice.call(f.querySelectorAll(".rfh")), on = "";
    halves.forEach(function (b) { if (b.getAttribute("data-k") === open && !b.disabled) on = open; });
    halves.forEach(function (b) { b.setAttribute("aria-expanded", String(!!on && b.getAttribute("data-k") === on)); });
    (f.parentNode ? Array.prototype.slice.call(f.parentNode.children) : []).forEach(function (l) {
      if (!l.classList.contains("rended")) return;
      if (on && l.getAttribute("data-k") === on) l.classList.add("on"); else l.classList.remove("on");
    });
  }
  // foldPick is a click on the fold half: it opens its list and closes the
  // other, or closes its own list when open, and keeps the pick (s, state).
  function foldPick(s, half) {
    var f = half.parentNode, key = f.getAttribute("data-key") || "", k = half.getAttribute("data-k") || "";
    var on = !half.disabled && half.getAttribute("aria-expanded") !== "true";
    s.ended.put(FOLD_KEYS.arch + key, on && k === "arch");
    s.ended.put(FOLD_KEYS.del + key, on && k === "del");
    foldShow(f, on ? k : "");
  }

  // REVIVE_MOVES are the ways an ended row brings its session back (its
  // revive entry's data-revive); each is the session's route of that name.
  var REVIVE_MOVES = { takeover: true, reopen: true };
  // reviveRoute is the route the revive entry for move posts to for the
  // session rid; empty for a move the rail does not know or no session.
  function reviveRoute(move, rid) {
    if (!REVIVE_MOVES.hasOwnProperty(move) || !rid) return "";
    return "/agents/" + encodeURIComponent(rid) + "/" + move;
  }
  // markRevive marks the revive entry of the row of session rid among rows
  // as posting (the .busy class and aria-busy); every other entry loses
  // both. An empty rid marks none.
  function markRevive(rows, rid) {
    rows.forEach(function (r) {
      var b = r.querySelector(".rrev");
      if (!b) return;
      var on = !!rid && r.getAttribute("data-rid") === rid;
      if (on) { b.classList.add("busy"); b.setAttribute("aria-busy", "true"); }
      else { b.classList.remove("busy"); b.removeAttribute("aria-busy"); }
    });
  }
  // handedNotice is the hand-off notice a session state st calls for: null
  // unless the session was handed to a terminal, else { command }, the shell
  // command that resumes it there (empty when the state carries none, as
  // for a viewer who does not hold the controls).
  function handedNotice(st) {
    if (!st || typeof st !== "object" || st.status !== "handed_off") return null;
    return { command: typeof st.resume_command === "string" ? st.resume_command.trim() : "" };
  }

  // leaveGuard is a beforeunload listener that asks the browser to confirm
  // leaving while holds() reports input the draft store cannot keep (pasted
  // images or a failed draft write), and does nothing otherwise.
  function leaveGuard(holds) {
    return function (e) {
      if (!holds()) return undefined;
      e.preventDefault();
      e.returnValue = "";
      return "";
    };
  }

  // popPlace is where a rail popover of size w x h goes in a viewport of vw x
  // vh, against the anchor rect a ({top, bottom, left, right}, viewport
  // coordinates): below the anchor, its right edge on the anchor's, else
  // above it when only that fits, kept 8px inside the viewport.
  function popPlace(a, w, h, vw, vh) {
    var m = 8, gap = 4;
    var left = Math.max(m, Math.min(a.right - w, vw - w - m));
    var top = a.bottom + gap;
    if (top + h > vh - m) top = a.top - gap - h >= m ? a.top - gap - h : Math.max(m, vh - h - m);
    return { top: Math.round(top), left: Math.round(left) };
  }

  // matchBranches is the branches of list whose name holds query, ignoring
  // case and surrounding space; an empty query matches every branch.
  function matchBranches(list, query) {
    var qv = String(query || "").trim().toLowerCase();
    return strings(list).filter(function (b) { return !qv || b.toLowerCase().indexOf(qv) >= 0; });
  }

  // worktreeBody is the add-worktree request for the repository workspace
  // ws from the form f: mode "new" (name, from base; an empty base is the
  // root's current HEAD), "old" (pick, an existing branch), or "detached"
  // (from base without a branch), and the folder:
  // the custom one when custom is set, else the default the form shows
  // (shown). The default names the current minute, so the daemon's own
  // default at add time can differ from the one the form showed.
  function worktreeBody(ws, f) {
    var isNew = f.mode === "new", detached = f.mode === "detached";
    var body = {
      workspace: ws,
      branch: detached ? "" : isNew ? String(f.name || "").trim() : String(f.pick || ""),
      "new": isNew,
      base: isNew || detached ? String(f.base || "") : "",
      folder: String((f.custom ? f.folder : f.shown) || "").trim()
    };
    if (detached) body.detached = true;
    return body;
  }

  // leadBody is the spawn request for a hosted lead session in root from
  // the form f: the harness, the plugin isolation (always sent), and a model, effort, and permission mode only
  // when one was picked (none leaves the harness's own). The form supplies
  // admitted permission choices; Pi keeps its existing mode-free request.
  function leadBody(root, f) {
    var body = { root: root, harness: String(f.harness || ""), plugin_isolation: f.isolation === "isolated" ? "isolated" : "default" };
    if (f.model) body.model = String(f.model);
    if (f.effort) body.effort = String(f.effort);
    if (f.mode && body.harness !== "pi") body.permission_mode = String(f.mode);
    return body;
  }

  // deleteReady reports whether the worktree delete modal's Delete is on:
  // shown is the check the modal displays ({key, check}: key the request it
  // answered), key the modal's current request, typed its input. Only the
  // current request's check counts (an answer to an earlier open is stale),
  // the typed text must be the check's folder name exactly, and the check
  // must hold no refusal, except the submit's own two (force: the delete
  // needs force the submit did not carry; changed: what force covers moved
  // since the check): their check is fresh, with a fresh token, and asks
  // only for the name to be typed again. A forced delete needs the token.
  function deleteReady(shown, typed, key) {
    var c = shown && shown.check;
    if (!c || typeof c !== "object" || shown.key !== key) return false;
    var r = c.refusal ? String(c.refusal) : "";
    if (r && r !== "force" && r !== "changed") return false;
    var folder = typeof c.folder === "string" ? c.folder : "";
    if (!folder || typed !== folder) return false;
    return !c.force || !!c.token;
  }
  // deleteBody is the delete submit for the worktree at path of the
  // repository workspace ws, confirmed against check: force and the check's
  // token only when the check needs force.
  function deleteBody(ws, path, check) {
    var force = !!(check && check.force);
    return { workspace: ws, path: path, force: force, token: force ? String(check.token || "") : "" };
  }

  // APPROVAL_CAP bounds an approval card's input preview.
  var APPROVAL_CAP = 4000;
  // approvalText is a tool call's input as an approval card shows it: a
  // shell call's command; a file call's path, then what it writes (an
  // edit's old lines under "-" and new ones under "+", a write's content);
  // a fetch's address; any other input as indented JSON. Capped at
  // APPROVAL_CAP characters.
  function approvalText(input) {
    var text = "";
    function lines(mark, s) { return String(s).split("\n").map(function (l) { return mark + l; }).join("\n"); }
    if (input === null || input === undefined) text = "";
    else if (typeof input !== "object" || Array.isArray(input)) text = typeof input === "string" ? input : JSON.stringify(input, null, 2);
    else if (typeof input.command === "string") text = input.command;
    else if (typeof input.file_path === "string") {
      text = input.file_path;
      if (typeof input.new_string === "string") text += "\n" + (input.old_string ? lines("- ", input.old_string) + "\n" : "") + lines("+ ", input.new_string);
      else if (typeof input.content === "string") text += "\n" + input.content;
    } else if (typeof input.url === "string") text = input.url;
    else if (Object.keys(input).length) text = JSON.stringify(input, null, 2);
    return text.length > APPROVAL_CAP ? text.slice(0, APPROVAL_CAP) + "\u2026" : text;
  }

  // Canonical server inputs are independent of the live DOM (client marks,
  // optimistic layout and time formatting never become the delta baseline).
  // The server sends a full region whenever these inter-row slots change.
  var ROW_SLOT = "<!--ws-rail-row-->";
  function rowInputFull(html, doc) {
    if (typeof html !== "string" || html.indexOf(ROW_SLOT) >= 0) return null;
    var box = doc.createElement("div"), rows = Object.create(null), order = [];
    box.innerHTML = html;
    var nodes = Array.prototype.slice.call(box.querySelectorAll(".rs[data-rid]"));
    if (nodes.some(function (n) {
      var id = n.getAttribute("data-rid");
      if (!id || rows[id] || n.parentElement.closest(".rs")) return true;
      rows[id] = { html: n.outerHTML }; order.push(id);
      return false;
    })) return null;
    nodes.forEach(function (n) { n.replaceWith(doc.createComment("ws-rail-row")); });
    return { shell: box.innerHTML.split(ROW_SLOT), order: order, rows: rows };
  }
  function rowNode(html, id, doc) {
    if (typeof html !== "string") return null;
    var box = doc.createElement("div"); box.innerHTML = html;
    var n = box.firstElementChild;
    return box.childNodes.length === 1 && n && n.matches("div.rs[data-rid]") && n.getAttribute("data-rid") === id && !n.querySelector(".rs") ? n : null;
  }
  function rowInputPatch(base, ev, doc) {
    if (!base || !ev || typeof ev.hash !== "string" || !Array.isArray(ev.order) || !Array.isArray(ev.rows) || ev.order.length + 1 !== base.shell.length) return null;
    var rows = Object.create(null), changed = Object.create(null), seen = Object.create(null);
    Object.keys(base.rows).forEach(function (id) { rows[id] = base.rows[id]; });
    if (ev.rows.some(function (r) {
      if (!r || typeof r.key !== "string" || !r.key || typeof r.hash !== "string" || changed[r.key]) return true;
      var n = rowNode(r.html, r.key, doc);
      if (!n) return true;
      changed[r.key] = true; rows[r.key] = { html: n.outerHTML };
      return false;
    })) return null;
    if (ev.order.some(function (id) {
      if (typeof id !== "string" || !id || seen[id] || !rows[id]) return true;
      seen[id] = true; return false;
    }) || Object.keys(changed).some(function (id) { return !seen[id]; })) return null;
    var kept = Object.create(null);
    ev.order.forEach(function (id) { kept[id] = rows[id]; });
    return { shell: base.shell, order: ev.order.slice(), rows: kept };
  }
  function rowInputHTML(input) {
    if (typeof input === "string") return input;
    return input.order.map(function (id, i) { return input.shell[i] + input.rows[id].html; }).join("") + input.shell[input.order.length];
  }
  // Validate everything before mutating. Only changed rows are parsed; an
  // unchanged row and a hover-only row keep their nodes and running spinners.
  // Return null for the existing full swap, false for hover-only/no-op, true
  // when visible markup changed and client marks must run again.
  function mergeRows(root, previous, next) {
    if (!previous || !next || previous.shell.length !== next.shell.length || previous.shell.some(function (s, i) { return s !== next.shell[i]; })) return null;
    var live = Object.create(null), updates = [], doc = root.ownerDocument;
    Array.prototype.forEach.call(root.querySelectorAll(".rs[data-rid]"), function (n) { live[n.getAttribute("data-rid")] = n; });
    if (Object.keys(live).length !== previous.order.length || previous.order.some(function (id) { return !live[id]; })) return null;
    var reordered = previous.order.some(function (id, i) { return id !== next.order[i]; }), visible = reordered;
    function withoutHover(n) { n = n.cloneNode(true); var src = n.querySelector(":scope > template.shp-source"); if (src) src.remove(); return n.outerHTML; }
    if (next.order.some(function (id) {
      var old = previous.rows[id], value = next.rows[id];
      if (!old || !live[id]) return true;
      if (old.html === value.html) return false;
      var a = rowNode(old.html, id, doc), b = rowNode(value.html, id, doc);
      if (!a || !b) return true;
      var source = b.querySelector(":scope > template.shp-source"), oldSource = live[id].querySelector(":scope > template.shp-source");
      if (source && oldSource && withoutHover(a) === withoutHover(b)) updates.push({ old: oldSource, node: source });
      else { updates.push({ old: live[id], node: b, id: id }); visible = true; }
      return false;
    })) return null;
    updates.forEach(function (u) { u.old.replaceWith(u.node); if (u.id) live[u.id] = u.node; });
    if (reordered) {
      var slots = previous.order.map(function (id) { var slot = doc.createComment("row"); live[id].replaceWith(slot); return slot; });
      slots.forEach(function (slot, i) { slot.replaceWith(live[next.order[i]]); });
    }
    return visible;
  }

  // Remember server markup before client marks/time formatting touch it.
  // A hover-only render replaces inert sources, not rows or their running
  // animations. Any other change uses the existing full rail swap.
  function hoverUpdates(root) {
    function read(box) {
      var sources = [];
      Array.prototype.forEach.call(box.querySelectorAll(".rs > template.shp-source"), function (src) {
        sources.push({ rid: src.parentNode.getAttribute("data-rid"), node: src });
        src.parentNode.removeChild(src);
      });
      return { markup: box.innerHTML, sources: sources };
    }
    var seen = read(root.cloneNode(true)).markup;
    return function (html) {
      var box = root.ownerDocument.createElement("div");
      box.innerHTML = html;
      var next = read(box), same = next.markup === seen;
      seen = next.markup;
      if (!same) return false;
      var sources = Object.create(null);
      Array.prototype.forEach.call(root.querySelectorAll(".rs > template.shp-source"), function (src) {
        sources[src.parentNode.getAttribute("data-rid")] = src;
      });
      // Validate all keys before changing anything; a locally missing source
      // falls back to the normal swap rather than leaving stale evidence.
      if (Object.keys(sources).length !== next.sources.length || next.sources.some(function (s) { return !sources[s.rid]; })) return false;
      next.sources.forEach(function (s) {
        var old = sources[s.rid];
        if (old.outerHTML !== s.node.outerHTML) old.parentNode.replaceChild(s.node, old);
      });
      return true;
    };
  }

  // Keep a read-only hover outside replaced rail/tree regions. The source
  // belongs directly to its row/agent: a parent's model is never a fallback.
  function hoverPlace(anchor, edge, width, height, viewport) {
    var left = edge + 8;
    if (left + width > viewport.width - 8) left = anchor.left - width - 8;
    return { left: Math.max(8, Math.min(left, viewport.width - width - 8)),
      top: Math.max(8, Math.min(anchor.top, viewport.height - height - 8)) };
  }
  function hover(doc, win) {
    var pop = null, active = null, x = 0, y = 0, keyboard = false, html = null;
    function target(n) {
      return n && n.closest ? n.closest(".rs, .amodel, .agent .aname") : null;
    }
    function source(n) {
      var owner = hasClass(n, "rs") ? n : n.closest(".agent");
      if (!owner) return null;
      for (var i = 0; i < owner.children.length; i++) {
        var c = owner.children[i];
        if (c.tagName === "TEMPLATE" && hasClass(c, "shp-source")) return c;
      }
      return null;
    }
    function hide() {
      active = null; html = null;
      if (pop) pop.hidden = true;
    }
    function show(n) {
      var src = n && source(n);
      // A rail drag (rdragging) shows no hover: the rows it passes are targets.
      if (!src || !n.isConnected || !n.getClientRects().length || hasClass(doc.documentElement, "rdragging")) { hide(); return; }
      if (!pop) {
        pop = doc.createElement("div"); pop.className = "rpop shp";
        pop.setAttribute("role", "tooltip");
        doc.body.appendChild(pop);
      }
      active = n;
      pop.className = "rpop shp" + (src.getAttribute("data-stale") ? " stale" : "");
      if (html !== src.innerHTML) { html = src.innerHTML; pop.innerHTML = html; }
      pop.hidden = false;
      var a = n.getBoundingClientRect(), rail = hasClass(n, "rs") && n.closest(".rail");
      var p = hoverPlace(a, rail ? rail.getBoundingClientRect().right : a.right,
        pop.offsetWidth, pop.offsetHeight, { width: win.innerWidth, height: win.innerHeight });
      pop.style.left = p.left + "px"; pop.style.top = p.top + "px";
    }
    doc.addEventListener("mouseover", function (e) {
      keyboard = false; x = e.clientX; y = e.clientY;
      var n = target(e.target); if (n) show(n); else hide();
    });
    doc.addEventListener("mousemove", function (e) { x = e.clientX; y = e.clientY; });
    doc.addEventListener("mouseout", function (e) {
      if (active && target(e.relatedTarget) !== active) hide();
    });
    doc.addEventListener("focusin", function (e) {
      keyboard = true; var n = target(e.target); if (n) show(n); else hide();
    });
    doc.addEventListener("focusout", hide);
    doc.addEventListener("pointerdown", hide);
    doc.addEventListener("keydown", function (e) { if (e.key === "Escape") hide(); });
    doc.addEventListener("scroll", hide, true);
    win.addEventListener("resize", hide); win.addEventListener("blur", hide);
    // SSE can replace the hovered node without a pointer event. Re-hit-test
    // at the last pointer (or focus) rather than retaining detached evidence.
    var observer = new win.MutationObserver(function (changes) {
      if (!active || !changes.some(function (c) { return c.target !== pop && !pop.contains(c.target); })) return;
      show(target(keyboard ? doc.activeElement : doc.elementFromPoint(x, y)));
    });
    observer.observe(doc.body, { childList: true, subtree: true });
    return { hide: hide, refresh: function () { if (active) show(target(keyboard ? doc.activeElement : doc.elementFromPoint(x, y))); } };
  }

  // rail.js is shared by the shell (which owns the rail) and framed pages
  // (which own the agent tree). Each document installs exactly one hover.
  if (typeof document !== "undefined") {
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", function () { hover(document, window); });
    else hover(document, window);
  }

  // topOver tells whether the header strip's items run past its content
  // box (its right padding is the clearance of the drawer's and tracker's
  // handles). The head region's wrapper lays out as its items (display:
  // contents), so they are measured in its place.
  function topOver(top, win) {
    var box = top.getBoundingClientRect(), end = box.right - (parseFloat(win.getComputedStyle(top).paddingRight) || 0), over = false;
    Array.prototype.forEach.call(top.querySelectorAll(":scope > :not(.r-head), :scope > .r-head > *"), function (el) {
      var r = el.getBoundingClientRect();
      if (r.width > 0 && r.right > end + 0.5) over = true;
    });
    return over;
  }
  // topFit steps the header strip (dashboard.css "top bar") down as it runs
  // short, the center first. Measured with the identity at its full width
  // (tmeas), an overflowing strip compacts plan usage to its one trigger
  // (tu); one that still overflows hides the low-priority chips (tch), and
  // the identity then ellipsizes to fit. The strip is one row of fixed
  // height, so no step moves anything under it. Returns the level, 0-2.
  function topFit(top, win) {
    win = win || window;
    if (!top || !top.classList) return 0;
    var cl = top.classList, level = 0;
    cl.add("tmeas"); cl.remove("tu", "tch");
    if (topOver(top, win)) {
      level = 1; cl.add("tu");
      if (topOver(top, win)) { level = 2; cl.add("tch"); }
    }
    cl.remove("tmeas");
    return level;
  }
  // topWatch keeps topFit current: on a resize of the strip (the window,
  // the drawer's and tracker's room it pads clear of) and on a change of its
  // items (a head swap, a usage redraw, a rename). Its own class changes
  // neither resize the strip nor touch its items.
  function topWatch(top, win) {
    win = win || window;
    if (!top) return;
    var fit = function () { topFit(top, win); };
    if (win.ResizeObserver) new win.ResizeObserver(fit).observe(top);
    if (win.MutationObserver) new win.MutationObserver(fit).observe(top, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ["hidden"] });
    if (win.document.fonts && win.document.fonts.ready) win.document.fonts.ready.then(fit);
    fit();
  }
  window.WsRail = {
    admissionDrafts: admissionDrafts,
    hoverUpdates: hoverUpdates,
    rowInputFull: rowInputFull,
    rowInputPatch: rowInputPatch,
    rowInputHTML: rowInputHTML,
    mergeRows: mergeRows,
    hover: hover,
    hoverPlace: hoverPlace,
    DRAFT_CAP: DRAFT_CAP,
    drafts: drafts,
    emptyDraft: emptyDraft,
    stripMarkers: stripMarkers,
    collapsed: collapsed,
    legacyImport: legacyImport,
    legacyDone: legacyDone,
    layoutRefusal: layoutRefusal,
    staleRail: staleRail,
    starred: starred,
    endedOpen: endedOpen,
    state: state,
    marks: marks,
    foldPick: foldPick,
    drop: drop,
    dropRefusal: dropRefusal,
    kidsCollapsed: kidsCollapsed,
    rowFacts: rowFacts,
    dropZone: dropZone,
    dropNoop: dropNoop,
    moveRow: moveRow,
    homeRows: homeRows,
    keyMove: keyMove,
    underRows: underRows,
    itemKey: itemKey,
    refind: refind,
    swapGate: swapGate,
    markCurrent: markCurrent,
    attention: attention,
    reviveRoute: reviveRoute,
    markRevive: markRevive,
    handedNotice: handedNotice,
    leaveGuard: leaveGuard,
    popPlace: popPlace,
    topFit: topFit,
    topWatch: topWatch,
    matchBranches: matchBranches,
    worktreeBody: worktreeBody,
    deleteReady: deleteReady,
    deleteBody: deleteBody,
    leadBody: leadBody,
    approvalText: approvalText
  };
})();
