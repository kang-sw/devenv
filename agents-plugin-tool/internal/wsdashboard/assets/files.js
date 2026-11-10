(function () {
  // The file explorer (window.WsFiles): the Files tab of the session page's
  // conversation column. It browses one root read-only through the file
  // browser routes (fs_routes.go, fs_search.go): a lazy tree, one listing
  // per folder on its first expand, a name search and a content search, and
  // a row menu. A file opens in the board's preview slot (preview.js).
  //
  // The page script owns the tab switch and hands the module its pieces
  // once (mount), then calls shown each time the pane comes into view: the
  // first time loads the root, every later time puts the tree's scroll back
  // and lists every loaded folder again, which refreshes the git marks.
  //
  // The browsing root is the session's project root unless a browser on the
  // daemon's machine changed it (the header's change root); the changed
  // root is kept per session in browser storage, and dropped when the
  // server refuses it. The View toggles (ignored and dot entries) are kept
  // per root, for a bounded number of roots.
  var doc = document, cards = window.WsCards;
  if (!cards) return;
  var VIEW_KEY = "ws.dashboard.files.view", ROOT_KEY = "ws.dashboard.files.root.", VIEW_MAX = 24;
  // NAME_WAIT and TEXT_WAIT are the search debounces in ms; INDENT a tree
  // level's indent in px.
  var NAME_WAIT = 120, TEXT_WAIT = 300, INDENT = 14;

  // --- pure parts (files_test.go runs them under node) ---------------------
  // showsEntry reports whether the View toggles show the listing entry e:
  // an ignored entry only while view.ign is on, a dot entry unless view.dot
  // (hide dot entries) is on.
  function showsEntry(e, view) {
    return (!e.ign || !!view.ign) && !(view.dot && e.name.charAt(0) === ".");
  }
  // opens reports a folder the tree can expand: on disk and no symlink.
  function opens(e) {
    return !!e.dir && !e.gone && !e.link;
  }
  function joinRel(dir, name) {
    return dir ? dir + "/" + name : name;
  }
  function parentRel(p) {
    var i = p.lastIndexOf("/");
    return i < 0 ? "" : p.slice(0, i);
  }
  // absPath is an entry's absolute path from its folder's listing: the
  // folder's absolute path (abs) in the host's spelling, the host's
  // separator (sep), and the entry's name. A root folder ("/", "C:\")
  // already ends in the separator.
  function absPath(abs, sep, name) {
    if (!abs) return name;
    return abs.slice(-sep.length) === sep ? abs + name : abs + sep + name;
  }
  // flatRows is the tree's rows as the screen shows them, top to bottom:
  // the root's entries, and right after an open folder its own entries one
  // level deeper. lists maps a loaded folder (its path relative to the
  // root, "" for the root) to its listing; open holds the folders shown
  // open; view the View toggles. A folder cut at the listing cap ends with
  // a "more" row. The keyboard walks these rows.
  function flatRows(lists, open, view) {
    var out = [];
    (function walk(dir, depth) {
      var l = lists[dir];
      if (!l) return;
      (l.entries || []).forEach(function (e) {
        if (!showsEntry(e, view)) return;
        var p = joinRel(dir, e.name), isOpen = opens(e) && !!open[p];
        out.push({ path: p, dir: dir, depth: depth, e: e, open: isOpen });
        if (isOpen) walk(p, depth + 1);
      });
      if (l.more) out.push({ path: dir + "/\u0000more", dir: dir, depth: depth, more: true });
    })("", 0);
    return out;
  }
  // rememberView returns the stored View map with key's toggles set at time
  // now, keeping at most max roots: the root set longest ago goes first.
  function rememberView(map, key, view, now, max) {
    var out = {};
    Object.keys(map || {}).forEach(function (k) { if (k !== key && map[k] && typeof map[k] === "object") out[k] = map[k]; });
    out[key] = { ign: !!view.ign, dot: !!view.dot, t: now };
    var keys = Object.keys(out).sort(function (a, b) { return (out[a].t || 0) - (out[b].t || 0); });
    while (keys.length > max) delete out[keys.shift()];
    return out;
  }
  // kindIcon is a file's Lucide icon by its extension.
  function kindIcon(name) {
    var i = name.lastIndexOf("."), x = i > 0 ? name.slice(i + 1).toLowerCase() : "";
    if (/^(png|jpe?g|gif|webp|avif|bmp|ico)$/.test(x)) return "image";
    if (/^(mp4|webm|ogg|ogv|mov|mkv|avi)$/.test(x)) return "film";
    if (/^(diff|patch)$/.test(x)) return "file-diff";
    if (/^(md|markdown|txt|rst|log|csv|tsv|adoc)$/.test(x)) return "file-text";
    if (/^(html?|svg|xml|css|scss|less|js|mjs|cjs|jsx|ts|tsx|json|ya?ml|toml|go|py|rb|rs|c|h|cc|cpp|hpp|cs|java|kt|swift|php|lua|sh|bash|zsh|ps1|sql|vue|svelte|tmpl|mod|sum)$/.test(x)) return "file-code";
    return "file";
  }

  // --- the module ----------------------------------------------------------
  var el = null, h = {}, rid = "", gen = 0, started = false;
  var lists = {}, open = {}, sel = null, cur = null, info = null, rootAbs = "";
  var view = { ign: false, dot: false }, rows = [], rowEls = {};
  var mode = "name", caseOn = false, regexOn = false, timer = null, ctrl = null;
  var treeTop = 0, hitsTop = 0, pop = null, popFrom = null, menu = null, menuFrom = null;
  var ui = {};

  function licon(name) { return cards.icon(name); }
  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function iconBtn(icon, title) {
    var b = mk("button", "ibtn");
    b.type = "button";
    b.innerHTML = licon(icon);
    b.title = title;
    b.setAttribute("aria-label", title);
    return b;
  }
  function shows(n) { return !!n && n.getClientRects().length > 0; }
  function local() { return typeof h.local === "function" ? !!h.local() : !!h.local; }
  function load(k) { return h.load ? h.load(k) : null; }
  function save(k, v) { if (h.save) h.save(k, v); }
  function toast(s) { if (h.toast) h.toast(s); }
  function editable(t) {
    return !!t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName || ""));
  }

  // fsGet asks a file browser route; it rejects with {status, text} on a
  // refusal, or the fetch's own error (an abort).
  function fsGet(route, params, signal) {
    var qs = Object.keys(params).filter(function (k) { return params[k] !== "" && params[k] != null; })
      .map(function (k) { return k + "=" + encodeURIComponent(params[k]); }).join("&");
    return fetch("/s/" + rid + "/fs/" + route + "?" + qs, { credentials: "same-origin", cache: "no-store", signal: signal }).then(function (r) {
      if (r.ok) return r.json();
      return r.text().then(function (t) { throw { status: r.status, text: (t || "").trim() }; });
    });
  }
  function listDir(dir, root) { return fsGet("list", { dir: dir, root: root === undefined ? rootAbs : root }); }

  // --- View toggles, per root ------------------------------------------------
  function viewMap() {
    try { var m = JSON.parse(load(VIEW_KEY) || "{}"); return m && typeof m === "object" ? m : {}; } catch (e) { return {}; }
  }
  function readView() {
    var v = info ? viewMap()[info.root] : null;
    view = { ign: !!(v && v.ign), dot: !!(v && v.dot) };
    ui.vIgn.checked = view.ign;
    ui.vDot.checked = view.dot;
  }
  function saveView() {
    if (info) save(VIEW_KEY, JSON.stringify(rememberView(viewMap(), info.root, view, Date.now(), VIEW_MAX)));
  }

  // --- the root --------------------------------------------------------------
  function paintHead() {
    ui.rootBtn.hidden = !local();
    ui.root.textContent = info ? info.name || info.root : "";
    ui.root.title = info ? info.root : "";
    ui.root.classList.toggle("alt", !!rootAbs);
  }
  // useRoot makes a listing of a root's top folder the tree's root, with a
  // fresh tree.
  function useRoot(a) {
    gen++;
    rootAbs = a.session ? "" : a.root;
    save(ROOT_KEY + rid, rootAbs || null);
    info = a;
    lists = { "": a };
    open = {};
    sel = cur = null;
    treeTop = 0;
    readView();
    paintHead();
    render();
    if (input().value) search(true);
  }
  // loadRoot lists the current root. A changed root the server refuses is
  // dropped for the session root.
  function loadRoot() {
    var g = ++gen;
    status(wsT("js.files.loading"));
    listDir("").then(function (a) { if (g === gen) useRoot(a); }, function (err) {
      if (g !== gen) return;
      if (rootAbs) {
        rootAbs = "";
        save(ROOT_KEY + rid, null);
        toast(wsT("js.files.root.dropped"));
        loadRoot();
        return;
      }
      status(err && err.status === 404 ? wsT("js.files.missing") : wsT("js.files.failed"));
    });
  }
  // applyRoot tries another root and keeps the current one when the server
  // refuses it ("" is the session root).
  function applyRoot(path) {
    ui.rErr.textContent = "";
    listDir("", path).then(function (a) { closePop(false); useRoot(a); }, function (err) {
      ui.rErr.textContent = err && err.status === 404 ? wsT("js.files.root.refused") : wsT("js.files.failed");
    });
  }

  // --- the tree --------------------------------------------------------------
  function status(text) {
    ui.tree.textContent = "";
    ui.tree.appendChild(mk("div", "fnote", text));
  }
  function rowEl(r) {
    var d = mk("div", "fr");
    d.style.paddingLeft = 4 + r.depth * INDENT + "px";
    if (r.more) {
      d.className = "fr fmore";
      d.setAttribute("role", "treeitem");
      d.setAttribute("aria-disabled", "true");
      d.setAttribute("aria-level", String(r.depth + 1));
      d.appendChild(mk("span", "ch"));
      d.appendChild(mk("span", "fn", wsT("js.files.more")));
      return d;
    }
    var e = r.e;
    d.setAttribute("role", "treeitem");
    d.setAttribute("aria-level", String(r.depth + 1));
    d.setAttribute("aria-selected", String(sel === r.path));
    d.setAttribute("data-path", r.path);
    d.tabIndex = cur === r.path ? 0 : -1;
    d.title = r.path;
    if (opens(e)) d.setAttribute("aria-expanded", String(r.open));
    if (e.ign) d.classList.add("ign");
    if (e.st) d.classList.add("gm");
    if (e.gone) d.classList.add("gone");
    if (sel === r.path) d.classList.add("sel");
    var ch = mk("span", "ch");
    if (opens(e)) ch.innerHTML = licon("chevron-right");
    d.appendChild(ch);
    var ic = mk("span", "fi");
    ic.innerHTML = e.dir ? licon(r.open ? "folder-open" : "folder") : e.link ? licon("file-symlink") : licon(kindIcon(e.name));
    d.appendChild(ic);
    d.appendChild(mk("span", "fn", e.name));
    if (e.st) {
      var m = mk("span", e.st === "*" ? "gmk dot" : "gmk", e.st === "*" ? "" : e.st);
      m.title = stTitle(e.st);
      d.appendChild(m);
      d.setAttribute("aria-description", stTitle(e.st));
    }
    return d;
  }
  function stTitle(s) {
    if (s === "M") return wsT("js.files.st.m");
    if (s === "A") return wsT("js.files.st.a");
    if (s === "U") return wsT("js.files.st.u");
    if (s === "D") return wsT("js.files.st.d");
    return wsT("js.files.st.dir");
  }
  function navRows() { return rows.filter(function (r) { return !r.more; }); }
  function rowOf(path) { return rows.filter(function (r) { return r.path === path; })[0] || null; }
  // render draws the tree's rows again, keeping its scroll and, when a row
  // had the focus, the focus on the current row.
  function render() {
    var t = ui.tree, had = t.contains(doc.activeElement), top = shows(t) ? t.scrollTop : treeTop;
    rows = info ? flatRows(lists, open, view) : [];
    var nav = navRows();
    if (sel !== null && !rowOf(sel)) sel = null;
    if (cur === null || !rowOf(cur)) cur = sel !== null ? sel : nav.length ? nav[0].path : null;
    var frag = doc.createDocumentFragment();
    rowEls = {};
    rows.forEach(function (r) {
      var d = rowEl(r);
      if (!r.more) rowEls[r.path] = d;
      frag.appendChild(d);
    });
    if (info && !rows.length) frag.appendChild(mk("div", "fnote", wsT("js.files.empty")));
    t.textContent = "";
    t.appendChild(frag);
    if (shows(t)) t.scrollTop = top;
    if (had && cur !== null && rowEls[cur]) rowEls[cur].focus({ preventScroll: true });
  }
  function moveTo(path) {
    if (!rowEls[path]) return;
    if (cur !== null && rowEls[cur]) rowEls[cur].tabIndex = -1;
    cur = path;
    rowEls[path].tabIndex = 0;
    rowEls[path].focus();
  }
  function toggle(path) {
    var g = gen;
    if (open[path]) { delete open[path]; render(); return; }
    open[path] = true;
    render();
    if (lists[path]) return;
    listDir(path).then(function (a) {
      if (g !== gen) return;
      lists[path] = a;
      render();
    }, function () {
      if (g !== gen) return;
      delete open[path];
      toast(wsT("js.files.failed"));
      render();
    });
  }
  function preview(path, line) {
    if (!window.WsPreview) return;
    var o = {};
    if (rootAbs) o.root = rootAbs;
    if (line > 0) o.line = line;
    window.WsPreview.open(path, o);
  }
  // activate is a click or Enter on a row: a folder opens or closes, a file
  // is selected and previewed; a deleted entry or a symlink only takes the
  // focus.
  function activate(r) {
    if (!r || r.more) return;
    if (opens(r.e)) { toggle(r.path); return; }
    if (r.e.dir || r.e.gone || r.e.link) return;
    sel = r.path;
    render();
    preview(r.path);
  }
  function collapseAll() {
    open = {};
    if (cur !== null) cur = cur.split("/")[0];
    render();
  }
  // present reports whether a folder is still listed, all the way up.
  function present(p) {
    if (p === "") return true;
    var d = parentRel(p), l = lists[d], name = d ? p.slice(d.length + 1) : p;
    return !!l && (l.entries || []).some(function (e) { return e.name === name && opens(e); }) && present(d);
  }
  // refresh lists the root and every open folder again (the git marks come
  // with the listings), keeping expansion, selection, focus, and scroll; a
  // folder that is gone collapses away, and a loaded but closed folder is
  // listed afresh when it opens next.
  function refresh() {
    if (!lists[""]) { loadRoot(); return; }
    var g = gen, dirs = Object.keys(lists).filter(function (d) { return d === "" || open[d]; });
    Object.keys(lists).forEach(function (d) { if (dirs.indexOf(d) < 0) delete lists[d]; });
    Promise.all(dirs.map(function (d) {
      return listDir(d).then(function (a) { return [d, a, null]; }, function (err) { return [d, null, err || {}]; });
    })).then(function (res) {
      if (g !== gen) return;
      var lost = false;
      res.forEach(function (x) {
        if (x[1]) { lists[x[0]] = x[1]; if (x[0] === "") info = x[1]; return; }
        if (x[2].status !== 404) return;
        if (x[0] === "") lost = true;
        else { delete lists[x[0]]; delete open[x[0]]; }
      });
      if (lost) { lists = {}; loadRoot(); return; }
      Object.keys(open).forEach(function (p) { if (!present(p)) delete open[p]; });
      Object.keys(lists).forEach(function (p) { if (!present(p)) delete lists[p]; });
      render();
    });
  }
  // revealDir leaves the search and opens a folder and every folder above
  // it, then focuses it.
  function revealDir(path) {
    clearSearch();
    var g = gen, chain = [], acc = "";
    path.split("/").forEach(function (p) { if (p) { acc = joinRel(acc, p); chain.push(acc); } });
    (function step(i) {
      if (g !== gen) return;
      if (i >= chain.length) {
        sel = cur = path;
        render();
        if (rowEls[path]) { rowEls[path].scrollIntoView({ block: "nearest" }); moveTo(path); }
        return;
      }
      var p = chain[i];
      open[p] = true;
      if (lists[p]) { step(i + 1); return; }
      // An answer for a root the explorer has left is dropped, not kept
      // as the new root's listing of the same path.
      listDir(p).then(function (a) { if (g !== gen) return; lists[p] = a; step(i + 1); }, function () { if (g !== gen) return; delete open[p]; render(); });
    })(0);
  }

  // --- the row menu ----------------------------------------------------------
  function closeMenu(refocus) {
    if (!menu) return;
    menu.parentNode.removeChild(menu);
    menu = null;
    if (refocus && menuFrom && rowEls[menuFrom]) rowEls[menuFrom].focus();
    menuFrom = null;
  }
  function act(path, action) {
    if (!h.post) return;
    h.post("fs/act", { path: path, action: action, root: rootAbs }, function (ok, v) {
      toast(ok ? (action === "open" ? wsT("js.toast.openednative") : wsT("js.files.revealed")) : wsT("js.toast.openfail", v));
    });
  }
  // openMenu opens a row's menu at x, y: preview (a file), the two host
  // actions (a browser on the daemon's machine only; the daemon checks
  // again), a terminal in a folder (a page that offers terminals,
  // terminal.js), and the two copy entries. A deleted entry offers only the
  // copy entries.
  function openMenu(r, x, y) {
    closeMenu(false);
    closePop(false);
    var e = r.e, path = r.path, l = lists[r.dir], items = [];
    if (!e.gone) {
      if (!e.dir && !e.link) items.push([wsT("js.menu.preview"), function () { sel = path; render(); preview(path); }]);
      if (local()) {
        items.push([wsT("js.preview.openext"), function () { act(path, "open"); }]);
        items.push([wsT("js.files.menu.reveal"), function () { act(path, "reveal"); }]);
      }
      if (e.dir && l && l.abs && window.WsTerm) {
        var dirAbs = absPath(l.abs, l.sep || "/", e.name);
        items.push([wsT("js.files.menu.terminal"), function () {
          window.WsTerm.create({ folder: "path", path: dirAbs }, function (err) { if (err) toast(err); });
        }]);
      }
    }
    if (items.length) items.push(null);
    items.push([wsT("js.menu.copyrel"), function () { if (h.copy) h.copy(path); }]);
    if (l && l.abs) items.push([wsT("js.menu.copyabs"), function () { if (h.copy) h.copy(absPath(l.abs, l.sep || "/", e.name)); }]);
    var m = mk("div", "lmenu fmenu");
    m.setAttribute("role", "menu");
    items.forEach(function (it) {
      if (!it) { var s = mk("div", "msep"); s.setAttribute("role", "separator"); m.appendChild(s); return; }
      var b = mk("button", "", it[0]);
      b.type = "button";
      b.setAttribute("role", "menuitem");
      b.addEventListener("click", function (ev) { ev.stopPropagation(); closeMenu(true); it[1](); });
      m.appendChild(b);
    });
    m.addEventListener("keydown", function (ev) {
      var bs = Array.prototype.slice.call(m.querySelectorAll("button")), i = bs.indexOf(doc.activeElement);
      if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); closeMenu(true); return; }
      if (ev.key === "Tab") { ev.preventDefault(); closeMenu(true); return; }
      var j = ev.key === "ArrowDown" ? (i + 1) % bs.length : ev.key === "ArrowUp" ? (i + bs.length - 1) % bs.length :
        ev.key === "Home" ? 0 : ev.key === "End" ? bs.length - 1 : -1;
      if (j < 0) return;
      ev.preventDefault();
      bs[j].focus();
    });
    doc.body.appendChild(m);
    var w = m.offsetWidth, ht = m.offsetHeight;
    m.style.left = Math.max(4, Math.min(x, window.innerWidth - w - 4)) + "px";
    m.style.top = Math.max(4, y + ht + 4 > window.innerHeight ? y - ht - 4 : y + 4) + "px";
    menu = m;
    menuFrom = path;
    m.querySelector("button").focus();
  }
  function menuAtRow(r) {
    var d = rowEls[r.path], b = d.getBoundingClientRect(), n = d.querySelector(".fn").getBoundingClientRect();
    openMenu(r, n.left, b.bottom);
  }

  // --- popovers: View and change root -----------------------------------------
  function closePop(refocus) {
    if (!pop) return;
    pop.hidden = true;
    if (popFrom) popFrom.setAttribute("aria-expanded", "false");
    if (refocus && popFrom) popFrom.focus();
    pop = popFrom = null;
  }
  function togglePop(p, btn) {
    if (pop === p) { closePop(true); return; }
    closePop(false);
    closeMenu(false);
    p.hidden = false;
    btn.setAttribute("aria-expanded", "true");
    pop = p;
    popFrom = btn;
    if (p === ui.rootPop) {
      ui.rErr.textContent = "";
      ui.rIn.value = info ? info.root : "";
      ui.rIn.focus();
      ui.rIn.select();
    } else ui.vIgn.focus();
  }

  // --- search ------------------------------------------------------------------
  function input() { return ui.q; }
  function setMode(m) {
    mode = m === "content" ? "content" : "name";
    el.setAttribute("data-fm", mode);
    ui.mName.setAttribute("aria-pressed", String(mode === "name"));
    ui.mText.setAttribute("aria-pressed", String(mode === "content"));
    ui.q.placeholder = mode === "name" ? wsT("js.files.find.name") : wsT("js.files.find.content");
    ui.q.setAttribute("aria-label", ui.q.placeholder);
    if (ui.q.value) search(true);
  }
  function abort() {
    if (ctrl && ctrl.c) ctrl.c.abort();
    ctrl = null;
  }
  function query() { return mode === "name" ? ui.q.value.trim() : ui.q.value; }
  function showTree() {
    if (!ui.tree.hidden) return;
    if (shows(ui.hits)) hitsTop = ui.hits.scrollTop;
    ui.hits.hidden = true;
    ui.tree.hidden = false;
    ui.tree.scrollTop = treeTop;
  }
  function showHits() {
    if (!ui.hits.hidden) return;
    if (shows(ui.tree)) treeTop = ui.tree.scrollTop;
    ui.tree.hidden = true;
    ui.hits.hidden = false;
  }
  function clearSearch() {
    clearTimeout(timer);
    abort();
    ui.q.value = "";
    ui.hits.textContent = "";
    showTree();
  }
  // search runs the search box's query after its debounce (now: at once).
  // A newer query aborts the request of an older one; the results shown
  // stay until the new ones are here. An empty query shows the tree again.
  function search(now) {
    clearTimeout(timer);
    if (!query().trim()) { abort(); ui.hits.textContent = ""; showTree(); return; }
    timer = setTimeout(runSearch, now ? 0 : mode === "name" ? NAME_WAIT : TEXT_WAIT);
  }
  function runSearch() {
    abort();
    var tok = { c: window.AbortController ? new AbortController() : null }, m = mode, q = query();
    ctrl = tok;
    showHits();
    var p = m === "name" ? fsGet("find", { q: q, root: rootAbs }, tok.c && tok.c.signal) :
      fsGet("search", { q: q, "case": caseOn ? "1" : "", regex: regexOn ? "1" : "", root: rootAbs }, tok.c && tok.c.signal);
    p.then(function (a) {
      if (ctrl !== tok) return;
      ctrl = null;
      if (m === "name") drawNames(a.items || []);
      else drawMatches(a);
    }, function (err) {
      if (ctrl !== tok) return;
      ctrl = null;
      ui.hits.textContent = "";
      ui.hits.appendChild(mk("div", "hnote err", err && err.status === 400 && err.text ? err.text : wsT("js.files.failed")));
    });
  }
  function hit(cls, path, line) {
    var d = mk("div", cls + " hit");
    d.setAttribute("role", "option");
    d.setAttribute("data-path", path);
    if (line) d.setAttribute("data-line", String(line));
    d.tabIndex = -1;
    return d;
  }
  function endHits(nodes, note) {
    var frag = doc.createDocumentFragment();
    nodes.forEach(function (n) { frag.appendChild(n); });
    frag.appendChild(mk("div", "hnote", note));
    ui.hits.textContent = "";
    ui.hits.appendChild(frag);
    var first = ui.hits.querySelector(".hit");
    if (first) first.tabIndex = 0;
    ui.hits.scrollTop = 0;
  }
  function drawNames(items) {
    var nodes = items.map(function (it) {
      var d = hit("fr", it.path);
      if (it.dir) d.setAttribute("data-dir", "");
      var ic = mk("span", "fi");
      ic.innerHTML = it.dir ? licon("folder") : licon(kindIcon(it.path));
      d.appendChild(ic);
      d.appendChild(mk("span", "fn", it.path + (it.dir ? "/" : "")));
      d.title = it.path;
      return d;
    });
    endHits(nodes, (items.length ? "" : wsT("js.files.nohits") + " \u00b7 ") + wsT("js.files.find.ign"));
  }
  function drawMatches(a) {
    var nodes = [], total = 0, files = a.files || [];
    files.forEach(function (f) {
      var hd = hit("hf", f.path), ic = mk("span", "fi");
      ic.innerHTML = licon(kindIcon(f.path));
      hd.appendChild(ic);
      hd.appendChild(mk("span", "fn", f.path));
      hd.appendChild(mk("span", "hc", String((f.matches || []).length)));
      hd.title = f.path;
      nodes.push(hd);
      (f.matches || []).forEach(function (m) {
        total++;
        var ln = hit("hl", f.path, m.line), tx = mk("span", "tx");
        ln.appendChild(mk("span", "ln", String(m.line)));
        tx.appendChild(doc.createTextNode(m.pre || ""));
        tx.appendChild(mk("mark", "", m.hit || ""));
        tx.appendChild(doc.createTextNode(m.post || ""));
        ln.appendChild(tx);
        nodes.push(ln);
      });
    });
    var note = files.length ? [wsTn("js.files.hits.files", files.length), wsTn("js.files.hits.matches", total)] : [wsT("js.files.nohits")];
    if (a.more) note.push(wsT("js.files.hits.more"));
    note.push(wsT("js.files.hits.ign"));
    endHits(nodes, note.join(" \u00b7 "));
  }
  function openHit(d) {
    var path = d.getAttribute("data-path");
    if (d.hasAttribute("data-dir")) { revealDir(path); return; }
    preview(path, +d.getAttribute("data-line") || 0);
  }

  // --- building the pane ---------------------------------------------------------
  function build() {
    el.textContent = "";
    el.setAttribute("data-fm", mode);
    var head = mk("div", "fhd");
    ui.root = mk("span", "froot");
    head.appendChild(ui.root);
    ui.rootBtn = iconBtn("folder-input", wsT("js.files.root.change"));
    ui.collapse = iconBtn("chevrons-down-up", wsT("js.files.collapse"));
    ui.refresh = iconBtn("refresh-cw", wsT("js.files.refresh"));
    ui.viewBtn = iconBtn("sliders-horizontal", wsT("js.files.view"));
    [ui.rootBtn, ui.viewBtn].forEach(function (b) { b.setAttribute("aria-haspopup", "dialog"); b.setAttribute("aria-expanded", "false"); });
    [ui.rootBtn, ui.collapse, ui.refresh, ui.viewBtn].forEach(function (b) { head.appendChild(b); });
    el.appendChild(head);

    var seg = mk("div", "fseg");
    seg.setAttribute("role", "group");
    seg.setAttribute("aria-label", wsT("js.files.mode"));
    ui.mName = mk("button", "", wsT("js.files.mode.name"));
    ui.mText = mk("button", "", wsT("js.files.mode.content"));
    [ui.mName, ui.mText].forEach(function (b) { b.type = "button"; seg.appendChild(b); });
    el.appendChild(seg);

    var row = mk("div", "fsr");
    var si = mk("span", "fi");
    si.innerHTML = licon("search");
    row.appendChild(si);
    ui.q = mk("input", "fq");
    ui.q.type = "text";
    ui.q.spellcheck = false;
    ui.q.setAttribute("autocomplete", "off");
    row.appendChild(ui.q);
    var opts = mk("span", "fopts");
    ui.caseBtn = iconBtn("case-sensitive", wsT("js.files.case"));
    ui.regexBtn = iconBtn("regex", wsT("js.files.regex"));
    [ui.caseBtn, ui.regexBtn].forEach(function (b) { b.setAttribute("aria-pressed", "false"); opts.appendChild(b); });
    row.appendChild(opts);
    el.appendChild(row);

    ui.tree = mk("div", "flist ftree");
    ui.tree.setAttribute("role", "tree");
    ui.tree.setAttribute("aria-label", wsT("js.files.tree"));
    ui.hits = mk("div", "flist fhits");
    ui.hits.setAttribute("role", "listbox");
    ui.hits.setAttribute("aria-label", wsT("js.files.results"));
    ui.hits.hidden = true;
    el.appendChild(ui.tree);
    el.appendChild(ui.hits);

    ui.viewPop = mk("div", "fpop");
    ui.viewPop.setAttribute("role", "dialog");
    ui.viewPop.setAttribute("aria-label", wsT("js.files.view"));
    ui.viewPop.hidden = true;
    ui.viewPop.appendChild(mk("div", "lb", wsT("js.files.view")));
    function opt(text) {
      var l = mk("label", "opt"), c = mk("input");
      c.type = "checkbox";
      l.appendChild(c);
      l.appendChild(doc.createTextNode(text));
      ui.viewPop.appendChild(l);
      return c;
    }
    ui.vIgn = opt(wsT("js.files.view.ign"));
    ui.vDot = opt(wsT("js.files.view.dot"));
    el.appendChild(ui.viewPop);

    ui.rootPop = mk("div", "fpop froot-pop");
    ui.rootPop.setAttribute("role", "dialog");
    ui.rootPop.setAttribute("aria-label", wsT("js.files.root.label"));
    ui.rootPop.hidden = true;
    ui.rootPop.appendChild(mk("div", "lb", wsT("js.files.root.label")));
    ui.rIn = mk("input", "fpin");
    ui.rIn.type = "text";
    ui.rIn.spellcheck = false;
    ui.rIn.setAttribute("autocomplete", "off");
    ui.rIn.placeholder = wsT("js.files.root.placeholder");
    ui.rIn.setAttribute("aria-label", wsT("js.files.root.placeholder"));
    ui.rootPop.appendChild(ui.rIn);
    ui.rErr = mk("div", "ferr");
    ui.rErr.setAttribute("role", "alert");
    ui.rootPop.appendChild(ui.rErr);
    var go = mk("div", "fgo");
    ui.rHome = mk("button", "btn", wsT("js.files.root.session"));
    ui.rApply = mk("button", "btn pr", wsT("js.files.root.apply"));
    [ui.rHome, ui.rApply].forEach(function (b) { b.type = "button"; go.appendChild(b); });
    ui.rootPop.appendChild(go);
    el.appendChild(ui.rootPop);
  }

  function wire() {
    ui.rootBtn.addEventListener("click", function () { togglePop(ui.rootPop, ui.rootBtn); });
    ui.viewBtn.addEventListener("click", function () { togglePop(ui.viewPop, ui.viewBtn); });
    ui.collapse.addEventListener("click", collapseAll);
    ui.refresh.addEventListener("click", refresh);
    ui.vIgn.addEventListener("change", function () { view.ign = ui.vIgn.checked; saveView(); render(); });
    ui.vDot.addEventListener("change", function () { view.dot = ui.vDot.checked; saveView(); render(); });
    ui.rApply.addEventListener("click", function () { if (ui.rIn.value.trim()) applyRoot(ui.rIn.value.trim()); });
    ui.rHome.addEventListener("click", function () { applyRoot(""); });
    ui.rIn.addEventListener("keydown", function (e) {
      if (e.key === "Enter") { e.preventDefault(); ui.rApply.click(); }
    });
    [ui.viewPop, ui.rootPop].forEach(function (p) {
      p.addEventListener("keydown", function (e) {
        if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); closePop(true); }
      });
    });
    ui.mName.addEventListener("click", function () { setMode("name"); });
    ui.mText.addEventListener("click", function () { setMode("content"); });
    [[ui.caseBtn, function (v) { caseOn = v; }], [ui.regexBtn, function (v) { regexOn = v; }]].forEach(function (p) {
      p[0].addEventListener("click", function () {
        var v = p[0].getAttribute("aria-pressed") !== "true";
        p[0].setAttribute("aria-pressed", String(v));
        p[1](v);
        if (ui.q.value) search(true);
      });
    });
    ui.q.addEventListener("input", function () { search(false); });
    ui.q.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        if (!ui.q.value) return;
        e.preventDefault();
        e.stopPropagation();
        clearSearch();
      } else if (e.key === "ArrowDown") {
        var first = ui.hits.hidden ? null : ui.hits.querySelector(".hit");
        if (first) { e.preventDefault(); first.focus(); }
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (mode === "name" && !ctrl && !ui.hits.hidden && ui.hits.querySelector(".hit")) openHit(ui.hits.querySelector(".hit"));
        else search(true);
      }
    });

    // the tree: a click, the right click, and the keys
    ui.tree.addEventListener("click", function (e) {
      var d = e.target.closest && e.target.closest(".fr[data-path]");
      if (!d) return;
      var r = rowOf(d.getAttribute("data-path"));
      moveTo(r.path);
      activate(r);
    });
    ui.tree.addEventListener("contextmenu", function (e) {
      var d = e.target.closest && e.target.closest(".fr[data-path]");
      if (!d) return;
      e.preventDefault();
      var r = rowOf(d.getAttribute("data-path"));
      moveTo(r.path);
      openMenu(r, e.clientX, e.clientY);
    });
    ui.tree.addEventListener("keydown", function (e) {
      var nav = navRows(), r = rowOf(cur), i = r ? nav.indexOf(r) : -1;
      if (!nav.length || e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
        if (r) { e.preventDefault(); menuAtRow(r); }
        return;
      }
      if (e.shiftKey) return;
      var to = null;
      if (e.key === "ArrowDown") to = nav[Math.min(nav.length - 1, i + 1)];
      else if (e.key === "ArrowUp") to = nav[Math.max(0, i - 1)];
      else if (e.key === "Home") to = nav[0];
      else if (e.key === "End") to = nav[nav.length - 1];
      else if (e.key === "ArrowRight" && r) {
        if (opens(r.e) && !r.open) { e.preventDefault(); toggle(r.path); return; }
        if (r.open && nav[i + 1] && nav[i + 1].dir === r.path) to = nav[i + 1];
        else { e.preventDefault(); return; }
      } else if (e.key === "ArrowLeft" && r) {
        if (r.open) { e.preventDefault(); toggle(r.path); return; }
        to = rowOf(r.dir);
        if (!to) { e.preventDefault(); return; }
      } else if (e.key === "Enter" && r) {
        e.preventDefault();
        activate(r);
        return;
      } else return;
      e.preventDefault();
      if (to) moveTo(to.path);
    });
    ui.tree.addEventListener("scroll", function () { if (shows(ui.tree)) treeTop = ui.tree.scrollTop; });

    // the results: a click, and the keys
    ui.hits.addEventListener("click", function (e) {
      var d = e.target.closest && e.target.closest(".hit");
      if (d) openHit(d);
    });
    ui.hits.addEventListener("keydown", function (e) {
      var all = Array.prototype.slice.call(ui.hits.querySelectorAll(".hit")), i = all.indexOf(doc.activeElement);
      if (i < 0) return;
      if (e.key === "Enter") { e.preventDefault(); openHit(all[i]); return; }
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); ui.q.focus(); return; }
      var j = e.key === "ArrowDown" ? Math.min(all.length - 1, i + 1) : e.key === "ArrowUp" ? i - 1 :
        e.key === "Home" ? 0 : e.key === "End" ? all.length - 1 : -2;
      if (j === -2) return;
      e.preventDefault();
      if (j < 0) ui.q.focus();
      else all[j].focus();
    });
    ui.hits.addEventListener("scroll", function () { if (shows(ui.hits)) hitsTop = ui.hits.scrollTop; });

    // "/" focuses the search box while the pane shows; a click elsewhere,
    // a scroll, a resize, or leaving the window closes a menu or popover.
    doc.addEventListener("keydown", function (e) {
      if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || e.defaultPrevented || editable(e.target) || !shows(el)) return;
      e.preventDefault();
      closeMenu(false);
      ui.q.focus();
      ui.q.select();
    });
    doc.addEventListener("mousedown", function (e) {
      var t = e.target;
      if (menu && !menu.contains(t)) closeMenu(false);
      if (pop && !pop.contains(t) && !(popFrom && popFrom.contains(t))) closePop(false);
    }, true);
    doc.addEventListener("wheel", function (e) { if (menu && !menu.contains(e.target)) closeMenu(false); }, { capture: true, passive: true });
    window.addEventListener("resize", function () { closeMenu(false); });
    window.addEventListener("blur", function () { closeMenu(false); });
  }

  window.WsFiles = {
    // mount builds the explorer in el. o: rid (the session), load and save
    // (the page's guarded storage), post (a session write route), toast,
    // copy (copies a text and says so), local (a function: whether this
    // browser is on the daemon's machine).
    mount: function (node, o) {
      if (el) return;
      el = node;
      h = o || {};
      rid = h.rid || doc.documentElement.getAttribute("data-rid") || "";
      build();
      wire();
      setMode("name");
      paintHead();
    },
    // shown is called each time the pane comes into view: the first time
    // it loads the root (a changed root kept for the session, if this
    // browser may use one), later it restores the scroll and refreshes.
    shown: function () {
      if (!el) return;
      paintHead();
      if (!started) {
        started = true;
        rootAbs = local() ? load(ROOT_KEY + rid) || "" : "";
        loadRoot();
        return;
      }
      if (!ui.tree.hidden) ui.tree.scrollTop = treeTop;
      else ui.hits.scrollTop = hitsTop;
      refresh();
    },
    // hidden is called when the pane leaves the view: an open menu or
    // popover closes.
    hidden: function () {
      closeMenu(false);
      closePop(false);
    }
  };
})();
