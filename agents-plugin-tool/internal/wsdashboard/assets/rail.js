(function () {
  // The session rail's browser-side state, window.WsRail, for the session
  // page's script (dashboard.js): per-session drafts, the starred sessions,
  // the collapsed workspaces, and the rail's client-side marks (the viewed
  // session's row, the starred rows pinned first, the drag order of the
  // sessions and workspaces, the revive entry in flight). The stored state is in localStorage under ws.dashboard.,
  // through the store the caller passes ({ load(key), save(key, value or
  // null) }, which must not throw), so the page works the same with storage
  // absent; nothing reaches the server. The DOM helpers touch only the
  // elements they are given; the revive route and hand-off notice, the
  // popover helpers at the end, and the approval card's input preview after
  // them, are pure.
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
  // stars are the starred sessions' reply-ids; collapsed the collapsed
  // workspaces' paths.
  function stars(store) { return stringSet(store, STARS); }
  function collapsed(store) { return stringSet(store, COLLAPSED); }

  // orderList is a drag order: ids kept as one JSON array. list() reads it;
  // place(group, present) stores group (the ids of one group, in their new
  // order) in place of that group's ids and drops each stored id present
  // (every id the rail shows) does not hold, returning what it stored. The
  // groups' ids never overlap, so one array holds every group's order. A
  // store that throws reads as no order and keeps nothing: the rail shows
  // the server's order.
  function orderList(store, key) {
    function list() { try { return strings(parse(store.load(key), [])); } catch (e) { return []; } }
    return {
      list: list,
      place: function (group, present) {
        var g = strings(group), keep = Object.create(null);
        strings(present).forEach(function (x) { keep[x] = true; });
        var l = list().filter(function (x) { return keep[x] && g.indexOf(x) < 0; }).concat(g);
        try { store.save(key, l.length ? JSON.stringify(l) : null); } catch (e) { /* not kept: the server's order stands */ }
        return l;
      }
    };
  }
  // sessionOrder is the sessions' drag order by reply-id, wsOrder the
  // workspaces' by path.
  function sessionOrder(store) { return orderList(store, ORDER); }
  function wsOrder(store) { return orderList(store, WSORDER); }

  function hasClass(el, c) { return (" " + (el.className || "") + " ").indexOf(" " + c + " ") >= 0; }
  function kids(box, c) {
    return Array.prototype.slice.call(box.children).filter(function (el) { return hasClass(el, c); });
  }
  // settle puts els (box's children, sorted) in their order before after,
  // touching nothing when they already stand in it: a moved node loses the
  // focus, and the rail re-sorts on every click that marks it.
  function settle(box, els, after) {
    var same = true;
    for (var i = 1; i < els.length && same; i++) same = els[i - 1].nextSibling === els[i];
    if (same && (els[els.length - 1].nextSibling || null) === after) return;
    els.forEach(function (el) { box.insertBefore(el, after); });
  }
  function ord(el, i) {
    if (el.getAttribute("data-ord") === null) el.setAttribute("data-ord", String(i));
    return +el.getAttribute("data-ord");
  }

  // pin marks each row of box (a rail sessions list) starred or not from
  // starred (a stars set) and orders the starred rows first. Within each
  // part the rows follow order (the drag order's ids, for a list of live
  // rows): a row it does not hold (a session new since the last drag)
  // first, then the rest as it lists them; without order, and among the
  // new rows, in the order the server sent (data-ord, set on first sight).
  // Unstarring a row returns it to its place. Anything after the rows (the
  // ended fold) stays after them.
  function pin(box, starred, order) {
    var rows = kids(box, "rs");
    if (!rows.length) return;
    var after = rows[rows.length - 1].nextSibling || null;
    rows.forEach(function (r, i) {
      var id = r.getAttribute("data-rid") || "", s = starred.has(id);
      r.wsOrd = ord(r, i);
      r.wsRank = order ? order.indexOf(id) : -1;
      r.wsStar = s;
      var b = r.querySelector(".star");
      if (b) b.setAttribute("aria-pressed", String(s));
    });
    rows.sort(function (a, b) { return (b.wsStar - a.wsStar) || (a.wsRank - b.wsRank) || (a.wsOrd - b.wsOrd); });
    settle(box, rows, after);
  }

  // arrange orders the workspace sections of list (the rail's list) by
  // order (the drag order's paths); a section it does not hold goes after
  // the ordered ones, in the order the server sent (data-ord). Anything
  // after the sections stays after them.
  function arrange(list, order) {
    var ws = kids(list, "rws");
    if (!ws.length) return;
    var after = ws[ws.length - 1].nextSibling || null;
    ws.forEach(function (w, i) {
      var k = (order || []).indexOf(w.getAttribute("data-ws") || "");
      w.wsOrd = ord(w, i);
      w.wsRank = k < 0 ? Infinity : k;
    });
    ws.sort(function (a, b) { return (a.wsRank - b.wsRank) || (a.wsOrd - b.wsOrd); });
    settle(list, ws, after);
  }

  // drop moves item (a live session row or a workspace section) next to
  // ref, a sibling of its kind, before it or (after) after it, and reports
  // whether it moved. It refuses a ref in another list (a session never
  // leaves its own group), a ref of another kind or on the other side of
  // the stars, an ended row, and the item itself.
  function drop(item, ref, after) {
    if (!item || !ref || item === ref || !item.parentNode || ref.parentNode !== item.parentNode) return false;
    var kind = hasClass(item, "rws") ? "rws" : "rs";
    if (!hasClass(ref, kind) || hasClass(item, "ended") || hasClass(ref, "ended") || !!item.wsStar !== !!ref.wsStar) return false;
    item.parentNode.insertBefore(item, after ? ref.nextSibling : ref);
    return true;
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
  // the form f: the harness, and a model, effort, and permission mode only
  // when one was picked (none leaves the harness's own). Only a Claude
  // session takes a permission mode.
  function leadBody(root, f) {
    var body = { root: root, harness: String(f.harness || "") };
    if (f.model) body.model = String(f.model);
    if (f.effort) body.effort = String(f.effort);
    if (f.mode && body.harness === "claude") body.permission_mode = String(f.mode);
    return body;
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
    stars: stars,
    collapsed: collapsed,
    sessionOrder: sessionOrder,
    wsOrder: wsOrder,
    pin: pin,
    arrange: arrange,
    drop: drop,
    markCurrent: markCurrent,
    reviveRoute: reviveRoute,
    markRevive: markRevive,
    handedNotice: handedNotice,
    leaveGuard: leaveGuard,
    popPlace: popPlace,
    matchBranches: matchBranches,
    worktreeBody: worktreeBody,
    leadBody: leadBody,
    approvalText: approvalText
  };
})();
