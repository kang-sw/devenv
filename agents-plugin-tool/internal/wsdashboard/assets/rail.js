(function () {
  // The session rail's browser-side state, window.WsRail, for the session
  // page's script (dashboard.js): per-session drafts, the collapsed
  // workspaces, and the rail's client-side marks (the viewed session's row,
  // the revive entry in flight), with the drag's refusal, re-find, and
  // swap-hold rules, and the rail layout route's answers. The daemon keeps
  // the rows' order, the stars, and the workspace order (its rail layout,
  // POST /rail/layout) and renders the rail in that order; the page only
  // reads a row's star from it. The drafts and the collapsed set are in
  // localStorage under ws.dashboard., through the store the caller passes
  // ({ load(key), save(key, value or null) }, which must not throw), so the
  // page works the same with storage absent; the retired local order and
  // stars are read there once, for the layout's import. The DOM helpers
  // touch only the elements they are given; the revive route and hand-off
  // notice, the popover and delete modal helpers at the end, and the
  // approval card's input preview after them, are pure.
  var DRAFT = "ws.dashboard.draft.", DRAFTS = "ws.dashboard.drafts";
  var STARS = "ws.dashboard.stars", COLLAPSED = "ws.dashboard.railcol";
  var ORDER = "ws.dashboard.railorder", WSORDER = "ws.dashboard.wsorder";
  // DRAFT_CAP bounds the sessions whose drafts are kept; the oldest saved
  // goes first.
  var DRAFT_CAP = 50;

  function parse(raw, def) {
    if (raw === null || raw === undefined) return def;
    try { var v = JSON.parse(raw); return v === null || v === undefined ? def : v; } catch (e) { return def; }
  }
  function strings(v) { return Array.isArray(v) ? v.filter(function (x) { return typeof x === "string"; }) : []; }
  function keys(o) { return o && typeof o === "object" ? Object.keys(o) : []; }

  // A draft is one session's unsent input and view state: text and
  // mentions ({start, end, text}) of the composer, sel its selection
  // ([start, end]), cmd its slash command selection ({name, args}: args a
  // string when the capsule holds them, else null), answers the board answers by item (readAnswers' shape,
  // topic notes among them), chips the answer chips, bmentions the board
  // mentions ({topic, id?, text}), refs the images that
  // reference written attachment files ({ref, name, n}), lost the numbers
  // of the pasted images it could not keep, open the open details by key, and tab
  // the tab shown. emptyDraft reports one that holds nothing to restore.
  function emptyDraft(d) {
    if (!d || typeof d !== "object") return true;
    return !d.text && !d.cmd && !(d.chips && d.chips.length) && !(d.bmentions && d.bmentions.length) && !keys(d.answers).length && !(d.refs && d.refs.length) &&
      !(d.lost && d.lost.length) && !keys(d.open).length && (!d.tab || d.tab === "convo");
  }

  // drafts is the draft store: load(rid), save(rid, draft) (an empty draft
  // clears), and clear(rid). Beyond cap sessions the oldest saved is dropped.
  function drafts(store, cap) {
    cap = cap || DRAFT_CAP;
    function index() { return strings(parse(store.load(DRAFTS), [])); }
    function clear(rid) {
      store.save(DRAFT + rid, null);
      var ids = index(), rest = ids.filter(function (x) { return x !== rid; });
      if (rest.length !== ids.length) store.save(DRAFTS, rest.length ? JSON.stringify(rest) : null);
    }
    return {
      load: function (rid) {
        var d = parse(store.load(DRAFT + rid), null);
        return d && typeof d === "object" && !Array.isArray(d) ? d : null;
      },
      save: function (rid, d) {
        if (emptyDraft(d)) { clear(rid); return; }
        var ids = index().filter(function (x) { return x !== rid; });
        ids.push(rid);
        while (ids.length > cap) store.save(DRAFT + ids.shift(), null);
        store.save(DRAFTS, JSON.stringify(ids));
        store.save(DRAFT + rid, JSON.stringify(d));
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
  // the new state, list().
  function stringSet(store, key) {
    function list() { return strings(parse(store.load(key), [])); }
    return {
      list: list,
      has: function (x) { return list().indexOf(x) >= 0; },
      toggle: function (x) {
        var l = list(), i = l.indexOf(x);
        if (i >= 0) l.splice(i, 1); else l.push(x);
        store.save(key, l.length ? JSON.stringify(l) : null);
        return i < 0;
      }
    };
  }
  // collapsed is the collapsed workspaces' paths.
  function collapsed(store) { return stringSet(store, COLLAPSED); }

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
    store: "js.rail.layout.store"
  };
  function layoutRefusal(reason) {
    return LAYOUT_REFUSALS.hasOwnProperty(reason) ? LAYOUT_REFUSALS[reason] : LAYOUT_REFUSALS.invalid;
  }
  // staleRail reports a rail render (its HTML) older than the layout
  // generation min: its list's data-lgen is below it. A render without one
  // is never stale.
  function staleRail(html, min) {
    var m = /\bdata-lgen="([0-9]+)"/.exec(String(html || ""));
    return !!m && +m[1] < min;
  }

  function hasClass(el, c) { return (" " + (el.className || "") + " ").indexOf(" " + c + " ") >= 0; }
  // starred reports a session row whose star the daemon rendered pressed.
  function starred(el) {
    var b = el && el.querySelector ? el.querySelector(".star") : null;
    return !!b && b.getAttribute("aria-pressed") === "true";
  }

  // drop moves item (a live session row or a workspace section) next to
  // ref, a sibling of its kind, before it or (after) after it, and reports
  // whether it moved. It refuses a ref in another list (a session never
  // leaves its own group), a ref of another kind or on the other side of
  // the stars, an ended row, and the item itself.
  function drop(item, ref, after) {
    if (!item || !ref || item === ref || !item.parentNode || ref.parentNode !== item.parentNode) return false;
    var kind = hasClass(item, "rws") ? "rws" : "rs";
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
  // another worktree of its own, "stars" over a live row of its own list on
  // the other side of the stars; "" anywhere else (its own group, a header,
  // the ended fold, or outside the list, where the drop takes the nearest
  // place). A workspace section is never refused.
  function dropRefusal(item, over) {
    if (!item || !over || hasClass(item, "rws")) return "";
    var ws = up(over, "rws");
    if (!ws) return "";
    if (ws !== up(item, "rws")) return "workspace";
    var wt = up(over, "rwt");
    if (wt && wt !== up(item, "rwt")) return "group";
    var row = up(over, "rs");
    if (row && row !== item && row.parentNode === item.parentNode && !hasClass(row, "ended") && starred(row) !== starred(item)) return "stars";
    return "";
  }

  // itemKey names a dragged item, a session row by reply-id or a workspace
  // section by path, so refind can find the node a swap put in its place.
  function itemKey(el) {
    var ws = hasClass(el, "rws"), attr = ws ? "data-ws" : "data-rid";
    return { cls: ws ? "rws" : "rs", attr: attr, val: el.getAttribute(attr) || "" };
  }
  // refind is the element under box that key names, or null.
  function refind(box, key) {
    var kids = box.children || [];
    for (var i = 0; i < kids.length; i++) {
      var k = kids[i];
      if (hasClass(k, key.cls) && k.getAttribute(key.attr) === key.val) return k;
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
  // images), and does nothing otherwise.
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
  // root's current HEAD) or "old" (pick, an existing branch), and the folder:
  // the custom one when custom is set, else the default the form shows
  // (shown). The default names the current minute, so the daemon's own
  // default at add time can differ from the one the form showed.
  function worktreeBody(ws, f) {
    var isNew = f.mode === "new";
    return {
      workspace: ws,
      branch: isNew ? String(f.name || "").trim() : String(f.pick || ""),
      "new": isNew,
      base: isNew ? String(f.base || "") : "",
      folder: String((f.custom ? f.folder : f.shown) || "").trim()
    };
  }

  // leadBody is the spawn request for a hosted lead session in root from
  // the form f: the harness, the plugin isolation (always sent), and a model, effort, and permission mode only
  // when one was picked (none leaves the harness's own). Only a Claude
  // session takes a permission mode.
  function leadBody(root, f) {
    var body = { root: root, harness: String(f.harness || ""), plugin_isolation: f.isolation === "isolated" ? "isolated" : "default" };
    if (f.model) body.model = String(f.model);
    if (f.effort) body.effort = String(f.effort);
    if (f.mode && body.harness === "claude") body.permission_mode = String(f.mode);
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

  window.WsRail = {
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
    drop: drop,
    dropRefusal: dropRefusal,
    itemKey: itemKey,
    refind: refind,
    swapGate: swapGate,
    markCurrent: markCurrent,
    reviveRoute: reviveRoute,
    markRevive: markRevive,
    handedNotice: handedNotice,
    leaveGuard: leaveGuard,
    popPlace: popPlace,
    matchBranches: matchBranches,
    worktreeBody: worktreeBody,
    deleteReady: deleteReady,
    deleteBody: deleteBody,
    leadBody: leadBody,
    approvalText: approvalText
  };
})();
