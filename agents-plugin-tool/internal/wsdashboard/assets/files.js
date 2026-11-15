(function () {
  // The file explorer (window.WsFiles): independent card, conversation-tab
  // and toolbar-sheet browsers over the guarded read-only fs routes. Each
  // inner tab owns its root/directory, selection, view mode and scroll.
  // The lazy tree stays beside non-column directory views; files open in the
  // board's preview slot (preview.js).
  //
  // The page script owns the tab switch and hands the module its pieces
  // once (create), then calls shown each time the pane comes into view: the
  // first time loads the root, every later time puts the tree's scroll back
  // and refreshes the current directory and expanded folders' git marks.
  //
  // The browsing root is the session's project root unless a browser on the
  // daemon's machine changed it (the header's change root); the changed
  // root is kept per session in browser storage, and dropped when the
  // server refuses it. The View toggles (ignored and dot entries) are kept
  // per root, for a bounded number of roots.
  var doc = document, cards = window.WsCards;
  if (!cards) return;
  var VIEW_KEY = "ws.dashboard.files.view", ROOT_KEY = "ws.dashboard.files.root.", VIEW_MAX = 24;
  var TABS_KEY = "ws.dashboard.files.tabs.", viewModes = ["details", "list", "icon", "gallery", "column"], cardHooks = {};
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

  // Each owner gets its own DOM, requests, search token and storage adapter.
  function create(node, options) {
  var el = node, h = options || {}, rid = h.rid || doc.documentElement.getAttribute("data-rid") || "";
  var directoriesOnly = !!h.directoriesOnly, routeBase = h.routeBase || "/s/" + rid + "/", rootPending = false;
  var gen = 0, rootAttempt = 0, started = false, destroyed = false, listeners = [];
  var lists = {}, open = {}, sel = null, cur = null, info = null, rootAbs = "";
  var view = { ign: false, dot: false }, rows = [], rowEls = {};
  var mode = "name", caseOn = false, regexOn = false, timer = null, ctrl = null, searchResult = null, searchPaused = false;
  var treeTop = 0, hitsTop = 0, pop = null, popFrom = null, menu = null, menuFrom = null;
  var ui = {}, api, tabs = [], active = -1, tabSeq = 0, dir = "", viewMode = "details", contentTop = 0, navAttempt = 0;
  var subscribers = { selection: [], navigation: [] }, detachAttachment = null;
  var columnLeft = 0, columnScroll = {}, columnLoads = {};

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
  function toast(s) { if (!destroyed && h.toast) h.toast(s); }
  function listen(target, type, fn, options) {
    target.addEventListener(type, fn, options);
    listeners.push(function () { target.removeEventListener(type, fn, options); });
  }
  function editable(t) {
    return !!t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName || ""));
  }

  // Tabs retain live trees separately; only the small browser record is persisted.
  function snapshot() {
    if (!tabs[active]) return;
    tabs[active].state = { root: rootAbs, dir: dir, selection: sel, scroll: contentTop, treeScroll: treeTop, view: viewMode,
      columnLeft: columnLeft, columnScroll: Object.assign({}, columnScroll) };
    tabs[active].live = { lists: lists, open: open, cur: cur, info: info,
      search: { q: ui.q.value, mode: mode, caseOn: caseOn, regexOn: regexOn, result: searchResult, scroll: hitsTop } };
  }
  function persistTabs() {
    if (destroyed || !started) return;
    snapshot();
    save(TABS_KEY + rid, JSON.stringify({ active: active, tabs: tabs.map(function (t) { return t.state; }) }));
  }
  function cleanLocation(p) {
    return typeof p === "string" && !/(^|\/)\.\.(\/|$)|^\/|^[a-z]:|\\|\u0000/i.test(p) ? p : "";
  }
  function newTab(location, background) {
    if (destroyed) return;
    snapshot();
    var state = location || { root: rootAbs, dir: dir, view: viewMode };
    tabs.push({ id: ++tabSeq, state: { root: local() && typeof state.root === "string" ? state.root : "", dir: cleanLocation(state.dir),
      selection: state.selection == null ? null : cleanLocation(state.selection), scroll: +state.scroll || 0, treeScroll: +state.treeScroll || 0,
      view: viewModes.indexOf(state.view) >= 0 ? state.view : "details", columnLeft: +state.columnLeft || 0,
      columnScroll: state.columnScroll && typeof state.columnScroll === "object" ? Object.assign({}, state.columnScroll) : {} } });
    if (!background) switchTab(tabs.length - 1);
    else { paintTabs(); persistTabs(); }
  }
  function paintTabs() {
    ui.tabs.textContent = "";
    tabs.forEach(function (t, i) {
      var wrap = mk("span", "ftab"), b = mk("button", "ftab-select", t.state.dir.split("/").pop() || t.state.root.split(/[\\/]/).pop() || wsT("js.files.root.session"));
      b.type = "button"; b.setAttribute("role", "tab"); b.setAttribute("aria-selected", String(i === active));
      b.addEventListener("click", function () { switchTab(i); });
      var x = iconBtn("x", wsT("js.files.tab.close")); x.classList.add("ftab-close");
      x.disabled = tabs.length === 1;
      x.addEventListener("click", function () { closeTab(i); });
      wrap.appendChild(b); wrap.appendChild(x); ui.tabs.appendChild(wrap);
    });
    var plus = mk("button", "ibtn ftab-new", "+"); plus.type = "button"; plus.title = wsT("js.files.tab.new"); plus.setAttribute("aria-label", plus.title);
    plus.addEventListener("click", function () { newTab({ root: rootAbs, dir: dir, view: viewMode }); });
    ui.tabs.appendChild(plus);
  }
  function switchTab(index) {
    if (destroyed || !tabs[index] || index === active) return;
    snapshot();
    gen++; rootAttempt++; navAttempt++; rootPending = false;
    clearSearch(); closeMenu(false); closePop(false);
    active = index;
    var t = tabs[active], s = t.state, live = t.live;
    rootAbs = s.root; dir = s.dir; sel = s.selection; cur = sel;
    contentTop = s.scroll; treeTop = s.treeScroll; viewMode = s.view;
    columnLeft = s.columnLeft; columnScroll = Object.assign({}, s.columnScroll);
    ui.tree.scrollTop = treeTop;
    lists = live ? live.lists : {}; open = live ? live.open : {}; info = live ? live.info : null;
    if (live && info) { readView(); paintHead(); render(); notify("navigation"); }
    else {
      // Never leave another tab's actionable rows under this tab's root.
      rows = []; rowEls = {}; ui.tree.textContent = "";
      paintHead(); renderDirectory(); notify("navigation");
      loadRoot(true);
    }
    restoreSearch(live && live.search);
    paintTabs(); persistTabs();
  }
  function closeTab(index) {
    if (tabs.length < 2 || !tabs[index]) return;
    snapshot();
    var was = active;
    tabs.splice(index, 1);
    active = index < was ? was - 1 : was;
    if (index === was) { active = -1; switchTab(Math.min(index, tabs.length - 1)); }
    else { paintTabs(); persistTabs(); }
  }
  function location() { return { root: rootAbs, dir: dir }; }
  function selection() {
    if (!info || (directoriesOnly && rootPending)) return null;
    var p = sel === null ? dir : sel, sep = info.sep || "/";
    if (directoriesOnly) {
      var parent = lists[parentRel(p)];
      if (!lists[dir] || (p !== dir && !(parent && parent.entries.some(function (e) { return joinRel(parentRel(p), e.name) === p && opens(e); })))) return null;
    }
    return { root: info.root, session: !!info.session, path: p || ".", absolute: p ? absPath(info.abs || info.root, sep, p.split("/").join(sep)) : info.abs || info.root };
  }
  function notify(type) {
    var value = selection();
    if (ui.send) ui.send.disabled = !value || !h.send;
    if (ui.copy) ui.copy.disabled = !value || !h.copy;
    if (!value && !directoriesOnly) return;
    subscribers[type].slice().forEach(function (fn) { fn(value, api); });
    if (h[type === "selection" ? "onSelection" : "onNavigate"]) h[type === "selection" ? "onSelection" : "onNavigate"](value, api);
  }
  function setAttachment(mount) {
    if (detachAttachment) detachAttachment();
    detachAttachment = null; ui.attachment.textContent = ""; ui.send = ui.copy = null;
    if (mount) { detachAttachment = mount(ui.attachment, api) || null; return; }
    ui.send = mk("button", "btn pr", wsT("js.files.send"));
    ui.copy = mk("button", "btn", wsT("js.files.copy"));
    [ui.send, ui.copy].forEach(function (b) { b.type = "button"; ui.attachment.appendChild(b); });
    ui.send.addEventListener("click", function () { var s = selection(); if (s && h.send) h.send(s.session ? s.path : s.absolute); });
    ui.copy.addEventListener("click", function () { var s = selection(); if (s && h.copy) h.copy(s.session ? s.path : s.absolute); });
    notify("selection");
  }
  function navigate(path, selected, focusPath) {
    if (destroyed) return;
    path = cleanLocation(path);
    clearSearch();
    var g = gen, attempt = ++navAttempt;
    function accept(a) {
      if (g !== gen || attempt !== navAttempt) return;
      lists[path] = a; dir = path; sel = cur = selected == null ? null : selected; contentTop = 0;
      render();
      if (viewMode === "column" && focusPath != null) {
        focusColumn(focusPath);
        if (selected === path) {
          var columns = ui.directory.querySelectorAll(".fcolumn");
          if (columns.length) columns[columns.length - 1].scrollIntoView({ block: "nearest", inline: "nearest" });
        }
        columnLeft = ui.directory.scrollLeft;
      }
      persistTabs(); paintTabs(); notify("navigation");
      if (selected != null) notify("selection");
    }
    if (lists[path]) accept(lists[path]);
    else listDir(path).then(accept, function () { if (g === gen && attempt === navAttempt) toast(wsT("js.files.failed")); });
  }

  // fsGet asks a file browser route; it rejects with {status, text} on a
  // refusal, or the fetch's own error (an abort).
  function fsGet(route, params, signal) {
    var qs = Object.keys(params).filter(function (k) { return params[k] !== "" && params[k] != null; })
      .map(function (k) { return k + "=" + encodeURIComponent(params[k]); }).join("&");
    return fetch(routeBase + "fs/" + route + "?" + qs, { credentials: "same-origin", cache: "no-store", signal: signal }).then(function (r) {
      if (r.ok) return r.json();
      return r.text().then(function (t) { throw { status: r.status, text: (t || "").trim() }; });
    });
  }
  function listDir(dir, root) {
    return fsGet("list", { dir: dir, root: root === undefined ? rootAbs : root }).then(function (a) {
      // The picker never puts files, symlinks or vanished folders into any view.
      return directoriesOnly ? Object.assign({}, a, { entries: (a.entries || []).filter(opens) }) : a;
    });
  }

  // --- View toggles, per root ------------------------------------------------
  function viewMap() {
    try { var m = JSON.parse(load(VIEW_KEY) || "{}"); return m && typeof m === "object" ? m : {}; } catch (e) { return {}; }
  }
  function readView() {
    var v = info ? viewMap()[info.root] : null;
    view = directoriesOnly ? { ign: true, dot: false } : { ign: !!(v && v.ign), dot: !!(v && v.dot) };
    ui.vIgn.checked = view.ign;
    ui.vDot.checked = view.dot;
  }
  function saveView() {
    if (info) save(VIEW_KEY, JSON.stringify(rememberView(viewMap(), info.root, view, Date.now(), VIEW_MAX)));
  }

  // --- the root --------------------------------------------------------------
  function paintHead() {
    ui.rootBtn.hidden = directoriesOnly || !local();
    if (ui.roots && info) ui.roots.value = rootAbs;
    ui.root.textContent = info ? info.name || info.root : "";
    ui.root.title = info ? info.root : "";
    ui.root.classList.toggle("alt", !!rootAbs);
    ui.location.textContent = dir ? "/" + dir : "/";
    ui.up.disabled = !dir;
  }
  // useRoot makes a listing of a root's top folder the tree's root, with a
  // fresh tree.
  function useRoot(a, restore) {
    var saved = restore ? { dir: dir, selection: sel, scroll: contentTop, treeScroll: treeTop, columnLeft: columnLeft, columnScroll: columnScroll } : null;
    gen++; navAttempt++;
    clearTimeout(timer);
    abort();
    searchResult = null;
    ui.hits.textContent = "";
    hitsTop = 0;
    rootAbs = a.session ? "" : a.root;
    save(ROOT_KEY + rid, rootAbs || null);
    info = a;
    lists = { "": a };
    open = {};
    sel = cur = null;
    treeTop = 0; dir = ""; contentTop = 0;
    columnLeft = saved ? saved.columnLeft : 0; columnScroll = saved ? saved.columnScroll : {}; columnLoads = {};
    readView();
    paintHead();
    render();
    function complete() { persistTabs(); paintTabs(); notify("navigation"); }
    if (saved && saved.dir) {
      var g = gen, attempt = navAttempt;
      dir = saved.dir; sel = saved.selection; contentTop = saved.scroll; treeTop = saved.treeScroll;
      paintHead();
      listDir(saved.dir).then(function (listing) {
        if (g !== gen || attempt !== navAttempt) return;
        lists[saved.dir] = listing; dir = saved.dir; sel = saved.selection; contentTop = saved.scroll; treeTop = saved.treeScroll;
        ui.tree.scrollTop = treeTop; render(); complete();
      }, function () { if (g === gen && attempt === navAttempt) { dir = ""; sel = null; render(); complete(); } });
    } else {
      if (saved) { sel = saved.selection; contentTop = saved.scroll; treeTop = saved.treeScroll; ui.tree.scrollTop = treeTop; render(); }
      complete();
    }
    if (input().value) search(true);
  }
  // loadRoot lists the current root. A changed root the server refuses is
  // dropped for the session root.
  function loadRoot(restore) {
    var g = ++gen;
    status(wsT("js.files.loading"));
    listDir("").then(function (a) { if (g === gen) useRoot(a, restore); }, function (err) {
      if (g !== gen) return;
      if (rootAbs && !directoriesOnly) {
        rootAbs = "";
        save(ROOT_KEY + rid, null);
        toast(wsT("js.files.root.dropped"));
        dir = ""; sel = null;
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
    // A candidate must not invalidate the active tree until accepted:
    // refusal still leaves its initial load and expansions usable.
    var attempt = ++rootAttempt;
    if (directoriesOnly) { rootPending = true; notify("navigation"); }
    listDir("", path).then(function (a) { if (attempt !== rootAttempt) return; rootPending = false; closePop(false); useRoot(a); }, function (err) {
      if (attempt !== rootAttempt) return;
      rootPending = false;
      ui.rErr.textContent = err && err.status === 404 ? wsT("js.files.root.refused") : wsT("js.files.failed");
      if (directoriesOnly) { paintHead(); notify("navigation"); toast(ui.rErr.textContent); }
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
    if (directoriesOnly && e.name.charAt(0) === ".") d.classList.add("fdot");
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
    // The side tree navigates folders; files belong to the directory pane.
    rows = info ? flatRows(lists, open, view).filter(function (r) { return r.more || r.e.dir; }) : [];
    var nav = navRows();
    if (sel !== null && sel !== dir && !rowOf(sel) && !(lists[dir] && (lists[dir].entries || []).some(function (e) { return joinRel(dir, e.name) === sel; }))) sel = null;
    if (cur === null || !rowOf(cur)) cur = sel !== null && rowOf(sel) ? sel : nav.length ? nav[0].path : null;
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
    paintHead(); renderDirectory();
  }
  function renderDirectory() {
    var top = contentTop, listing = lists[dir], box = ui.directory;
    var focused = box.contains(doc.activeElement) ? doc.activeElement.getAttribute("data-entry") : null;
    box.textContent = ""; box.setAttribute("data-view", viewMode);
    ui.modes.forEach(function (b, i) { b.setAttribute("aria-pressed", String(viewMode === viewModes[i])); });
    ui.workspace.setAttribute("data-view", viewMode);
    // Layout hides the sidebar independently of search's tree/hits visibility.
    ui.tree.inert = viewMode === "column";
    box.setAttribute("role", viewMode === "column" ? "group" : "listbox");
    if (viewMode === "column") { renderColumns(focused); return; }
    if (viewMode === "details") {
      var hd = mk("div", "fdetail-head");
      hd.appendChild(mk("span", "fn", wsT("js.files.column.name")));
      hd.appendChild(mk("span", "ftype", wsT("js.files.column.type"))); box.appendChild(hd);
    }
    var entries = listing ? (listing.entries || []).filter(function (e) { return showsEntry(e, view); }) : [];
    entries.forEach(function (e) { box.appendChild(directoryEntry(e, dir)); });
    if (listing && !entries.length) box.appendChild(mk("div", "fnote", wsT("js.files.empty")));
    if (listing && listing.more) box.appendChild(mk("div", "fnote", wsT("js.files.more")));
    box.scrollLeft = 0; box.scrollTop = top;
  }
  function directoryEntry(e, folder, column) {
    var p = joinRel(folder, e.name), selected = sel === p || (column && opens(e) && (dir === p || dir.indexOf(p + "/") === 0));
    var row = mk("div", "fentry" + (selected ? " sel" : "") + (e.ign ? " ign" : "") + (directoriesOnly && e.name.charAt(0) === "." ? " fdot" : ""));
    row.setAttribute("data-entry", p); row.setAttribute("role", "option"); row.setAttribute("aria-selected", String(selected)); row.tabIndex = 0;
    var ic = mk("span", "fi");
    ic.innerHTML = licon(e.dir ? "folder" : e.link ? "file-symlink" : kindIcon(e.name));
    if (viewMode === "gallery" && !e.dir && !e.link && !e.gone && kindIcon(e.name) === "image") {
      var img = mk("img", "fthumb"); img.alt = e.name; img.loading = "lazy";
      img.src = routeBase + "fs/raw?path=" + encodeURIComponent(p) + (rootAbs ? "&root=" + encodeURIComponent(rootAbs) : ""); row.appendChild(img);
    } else row.appendChild(ic);
    row.appendChild(mk("span", "fn", e.name));
    if (viewMode === "details") row.appendChild(mk("span", "ftype", e.dir ? wsT("js.files.folder") : e.name.indexOf(".") >= 0 ? e.name.split(".").pop().toUpperCase() : wsT("js.files.file")));
    if (column && opens(e)) {
      var ch = mk("span", "fcolumn-chevron"); ch.innerHTML = licon("chevron-right"); row.appendChild(ch);
    }
    row.title = p;
    function choose() {
      if (column && (opens(e) || folder !== dir)) { navigate(opens(e) ? p : folder, p, p); return; }
      sel = p;
      Array.prototype.forEach.call(ui.directory.querySelectorAll(".fentry"), function (n) {
        var path = n.getAttribute("data-entry"), on = path === sel || (column && (dir === path || dir.indexOf(path + "/") === 0));
        n.classList.toggle("sel", on); n.setAttribute("aria-selected", String(on));
      });
      Object.keys(rowEls).forEach(function (path) {
        rowEls[path].classList.toggle("sel", path === sel); rowEls[path].setAttribute("aria-selected", String(path === sel));
      });
      persistTabs(); notify("selection");
    }
    row.addEventListener("click", choose);
    row.addEventListener("dblclick", function () { if (!column && opens(e)) navigate(p); else if (!e.dir && !e.link && !e.gone) preview(p); });
    row.addEventListener("keydown", function (ev) {
      if (column && columnKey(ev, row, choose)) return;
      if (ev.key === "Enter") { ev.preventDefault(); choose(); if (!column && opens(e)) navigate(p); else if (!e.dir && !e.link && !e.gone) preview(p); }
    });
    row.addEventListener("auxclick", function (ev) { if (ev.button === 1 && opens(e)) { ev.preventDefault(); newTab({ root: rootAbs, dir: p, view: viewMode }, true); } });
    row.addEventListener("contextmenu", function (ev) { ev.preventDefault(); choose(); openMenu({ e: e, path: p, dir: folder }, ev.clientX, ev.clientY); });
    return row;
  }
  function columnDirs() {
    var chain = [""], path = "";
    dir.split("/").forEach(function (part) { if (part) { path = joinRel(path, part); chain.push(path); } });
    return chain;
  }
  function focusColumn(path) {
    var all = ui.directory.querySelectorAll(".fentry");
    Array.prototype.some.call(all, function (row) {
      if (row.getAttribute("data-entry") !== path) return false;
      row.focus({ preventScroll: true }); row.scrollIntoView({ block: "nearest", inline: "nearest" }); return true;
    });
  }
  function renderColumns(focused) {
    var box = ui.directory;
    columnDirs().forEach(function (folder) {
      var col = mk("div", "fcolumn"), listing = lists[folder], pending = columnLoads[folder];
      col.setAttribute("data-column", folder); col.setAttribute("role", "listbox");
      col.setAttribute("aria-label", folder || (info && (info.name || info.root)) || wsT("js.files.root.session"));
      var entries = listing ? (listing.entries || []).filter(function (e) { return showsEntry(e, view); }) : [];
      entries.forEach(function (e) { col.appendChild(directoryEntry(e, folder, true)); });
      if (!listing) {
        col.appendChild(mk("div", "fnote", wsT(pending && pending.gen === gen && pending.error ? "js.files.failed" : "js.files.loading")));
        if (info && (!pending || pending.gen !== gen)) {
          var request = { gen: gen }; columnLoads[folder] = request;
          listDir(folder).then(function (a) {
            if (request.gen !== gen) return;
            lists[folder] = a;
            if (viewMode === "column") renderDirectory();
          }, function () {
            if (request.gen !== gen) return;
            request.error = true;
            if (viewMode === "column") renderDirectory();
          });
        }
      } else {
        if (!entries.length) col.appendChild(mk("div", "fnote", wsT("js.files.empty")));
        if (listing.more) col.appendChild(mk("div", "fnote", wsT("js.files.more")));
      }
      box.appendChild(col); col.scrollTop = +columnScroll[folder] || 0;
      col.addEventListener("scroll", function () { if (shows(col)) { columnScroll[folder] = col.scrollTop; persistTabs(); } });
    });
    box.scrollTop = 0; box.scrollLeft = columnLeft;
    if (focused != null) focusColumn(focused);
  }
  function columnKey(ev, row, choose) {
    if (ev.altKey || ev.ctrlKey || ev.metaKey || ev.shiftKey || !/^Arrow(Up|Down|Left|Right)$/.test(ev.key)) return false;
    ev.preventDefault();
    var col = row.parentNode, all = Array.prototype.slice.call(col.querySelectorAll(".fentry")), i = all.indexOf(row), target;
    if (ev.key === "ArrowUp" || ev.key === "ArrowDown") {
      target = all[Math.max(0, Math.min(all.length - 1, i + (ev.key === "ArrowUp" ? -1 : 1)))];
      if (target) { target.click(); focusColumn(target.getAttribute("data-entry")); }
    } else {
      var cols = Array.prototype.slice.call(ui.directory.querySelectorAll(".fcolumn")), next = cols[cols.indexOf(col) + (ev.key === "ArrowLeft" ? -1 : 1)];
      if (next) {
        target = next.querySelector('.fentry[aria-selected="true"]') || next.querySelector(".fentry");
        if (target) focusColumn(target.getAttribute("data-entry"));
      } else if (ev.key === "ArrowRight") choose();
    }
    return true;
  }
  function moveTo(path) {
    if (!rowEls[path]) return;
    if (cur !== null && rowEls[cur]) rowEls[cur].tabIndex = -1;
    cur = path;
    rowEls[path].tabIndex = 0;
    rowEls[path].focus();
  }
  function toggle(path, browse) {
    var g = gen, navigation = browse ? ++navAttempt : navAttempt;
    if (open[path]) { delete open[path]; render(); if (browse) navigate(path); return; }
    open[path] = true;
    render();
    if (lists[path]) { if (browse) navigate(path); return; }
    listDir(path).then(function (a) {
      if (g !== gen) return;
      lists[path] = a;
      render();
      if (browse && navigation === navAttempt) navigate(path);
    }, function () {
      if (g !== gen) return;
      delete open[path];
      toast(wsT("js.files.failed"));
      render();
    });
  }
  function preview(path, line) {
    if (directoriesOnly) return;
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
    if (opens(r.e)) { toggle(r.path, true); return; }
    if (r.e.dir || r.e.gone || r.e.link) return;
    sel = r.path;
    render(); persistTabs(); notify("selection");
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
    columnLoads = {};
    var chain = viewMode === "column" ? columnDirs() : [];
    var g = gen, dirs = Object.keys(lists).filter(function (d) { return d === "" || d === dir || open[d] || chain.indexOf(d) >= 0; });
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
        else { delete lists[x[0]]; delete open[x[0]]; if (dir === x[0]) { dir = ""; sel = null; contentTop = 0; } }
      });
      if (lost) { lists = {}; loadRoot(); return; }
      Object.keys(open).forEach(function (p) { if (!present(p)) delete open[p]; });
      Object.keys(lists).forEach(function (p) { if (p !== dir && !present(p)) delete lists[p]; });
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
        navigate(path);
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
    if (directoriesOnly) return;
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
  // Search state stays live on its inner tab, never in another owner's
  // browser record. Pending work is restarted on return, not reused across tabs.
  function restoreSearch(s) {
    caseOn = !!(s && s.caseOn); regexOn = !!(s && s.regexOn);
    ui.caseBtn.setAttribute("aria-pressed", String(caseOn));
    ui.regexBtn.setAttribute("aria-pressed", String(regexOn));
    setMode(s && s.mode);
    ui.q.value = s ? s.q : "";
    searchResult = s && s.result;
    if (!query().trim()) return;
    if (!searchResult) { search(true); return; }
    showHits();
    if (mode === "name") drawNames(searchResult.items || []);
    else drawMatches(searchResult);
    hitsTop = s.scroll || 0;
    ui.hits.scrollTop = hitsTop;
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
    ui.workspace.hidden = false;
    ui.tree.scrollTop = treeTop;
  }
  function showHits() {
    if (!ui.hits.hidden) return;
    if (shows(ui.tree)) treeTop = ui.tree.scrollTop;
    ui.tree.hidden = true;
    ui.workspace.hidden = true;
    ui.hits.hidden = false;
  }
  function clearSearch() {
    clearTimeout(timer);
    abort();
    ui.q.value = "";
    searchResult = null;
    ui.hits.textContent = "";
    showTree();
    hitsTop = 0;
  }
  // Invalidate immediately, before debounce: even a fetch that ignores abort
  // cannot repaint or leave actionable results for a superseded query/mode.
  function search(now) {
    if (directoriesOnly) return;
    clearTimeout(timer);
    abort();
    searchResult = null;
    ui.hits.textContent = "";
    hitsTop = 0;
    if (!query().trim()) { showTree(); return; }
    if (searchPaused) return;
    showHits();
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
      searchResult = a;
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
    hitsTop = 0;
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
    el.classList.toggle("fpicker", directoriesOnly);
    var head = mk("div", "fhd");
    ui.root = mk("span", "froot");
    (h.rootSlot || head).appendChild(ui.root);
    ui.rootBtn = iconBtn("folder-input", wsT("js.files.root.change"));
    ui.collapse = iconBtn("chevrons-down-up", wsT("js.files.collapse"));
    ui.refresh = iconBtn("refresh-cw", wsT("js.files.refresh"));
    ui.viewBtn = iconBtn("sliders-horizontal", wsT("js.files.view"));
    [ui.rootBtn, ui.viewBtn].forEach(function (b) { b.setAttribute("aria-haspopup", "dialog"); b.setAttribute("aria-expanded", "false"); });
    [ui.rootBtn, ui.collapse, ui.refresh, ui.viewBtn].forEach(function (b) { head.appendChild(b); });
    ui.openCard = iconBtn("external-link", wsT("js.files.open.card"));
    ui.openCard.classList.add("fopen-card");
    ui.openCard.hidden = directoriesOnly;
    ui.viewBtn.hidden = directoriesOnly;
    ui.openCard.addEventListener("click", function () { if (h.openCard) h.openCard(location()); else openCard(location()); });
    head.appendChild(ui.openCard);
    el.appendChild(head);
    ui.tabs = mk("div", "ftabs"); ui.tabs.setAttribute("role", "tablist");
    ui.tabs.setAttribute("aria-label", wsT("js.files.tabs")); el.appendChild(ui.tabs);
    var nav = mk("div", "fnav");
    ui.up = iconBtn("chevron-up", wsT("js.files.up")); ui.up.addEventListener("click", function () { navigate(parentRel(dir)); });
    ui.location = mk("span", "flocation"); nav.appendChild(ui.up); nav.appendChild(ui.location); el.appendChild(nav);
    if (directoriesOnly && h.roots) {
      ui.roots = mk("select", "froot-choices");
      ui.roots.setAttribute("aria-label", wsT("js.files.root.label"));
      [h.roots.home].concat(h.roots.volumes || []).filter(function (p, i, all) { return p && all.indexOf(p) === i; }).forEach(function (path) {
        var option = mk("option", "", h.roots.host + " · " + path); option.value = path; ui.roots.appendChild(option);
      });
      ui.roots.addEventListener("change", function () { applyRoot(ui.roots.value); });
      nav.appendChild(ui.roots);
    }

    var seg = mk("div", "fseg");
    seg.setAttribute("role", "group");
    seg.setAttribute("aria-label", wsT("js.files.mode"));
    ui.mName = mk("button", "", wsT("js.files.mode.name"));
    ui.mText = mk("button", "", wsT("js.files.mode.content"));
    [ui.mName, ui.mText].forEach(function (b) { b.type = "button"; seg.appendChild(b); });
    el.appendChild(seg);
    seg.hidden = directoriesOnly;

    var row = mk("div", "fsr");
    row.hidden = directoriesOnly;
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
    var modes = mk("div", "fview-modes"); ui.modes = [];
    viewModes.forEach(function (value) {
      var labels = [wsT("js.files.view.details"), wsT("js.files.view.list"), wsT("js.files.view.icon"), wsT("js.files.view.gallery"), wsT("js.files.view.column")];
      var label = labels[viewModes.indexOf(value)], b = mk("button", value === "column" ? "fview-column" : "", value === "column" ? "\u25a5" : label); b.type = "button";
      if (value === "column") { b.title = label; b.setAttribute("aria-label", label); }
      b.addEventListener("click", function () { viewMode = value; renderDirectory(); persistTabs(); });
      modes.appendChild(b); ui.modes.push(b);
    }); el.appendChild(modes);
    ui.workspace = mk("div", "fworkspace");
    ui.directory = mk("div", "fdirectory"); ui.directory.setAttribute("role", "listbox");
    ui.directory.setAttribute("aria-label", wsT("js.files.directory"));
    ui.workspace.appendChild(ui.tree); ui.workspace.appendChild(ui.directory);
    el.appendChild(ui.workspace); el.appendChild(ui.hits);
    ui.attachment = mk("div", "fattachment"); el.appendChild(ui.attachment);

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
    ui.tree.addEventListener("scroll", function () { if (info && shows(ui.tree)) { treeTop = ui.tree.scrollTop; persistTabs(); } });
    ui.directory.addEventListener("scroll", function () {
      if (!info || !lists[dir] || !shows(ui.directory)) return;
      if (viewMode === "column") columnLeft = ui.directory.scrollLeft;
      else contentTop = ui.directory.scrollTop;
      persistTabs();
    });
    ui.tree.addEventListener("auxclick", function (e) {
      var d = e.target.closest && e.target.closest(".fr[data-path]"), r = d && rowOf(d.getAttribute("data-path"));
      if (e.button === 1 && r && opens(r.e)) { e.preventDefault(); newTab({ root: rootAbs, dir: r.path, view: viewMode }, true); }
    });

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
    listen(doc, "keydown", function (e) {
      var pane = e.target.closest && e.target.closest(".fpane"), sheet = doc.querySelector(".bcard[data-max] .fpane, .fpane[data-max]");
      if ((pane && pane !== el) || (sheet && sheet !== el) || (!pane && !sheet)) return;
      if (directoriesOnly || e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || e.defaultPrevented || editable(e.target) || !shows(el)) return;
      e.preventDefault();
      closeMenu(false);
      ui.q.focus();
      ui.q.select();
    });
    listen(doc, "mousedown", function (e) {
      var t = e.target;
      if (menu && !menu.contains(t)) closeMenu(false);
      if (pop && !pop.contains(t) && !(popFrom && popFrom.contains(t))) closePop(false);
    }, true);
    listen(doc, "wheel", function (e) { if (menu && !menu.contains(e.target)) closeMenu(false); }, { capture: true, passive: true });
    listen(window, "resize", function () { closeMenu(false); });
    listen(window, "blur", function () { closeMenu(false); });
  }

  build();
  wire();
  setMode("name");
  paintHead();
  api = {
    location: location,
    selection: selection,
    navigate: navigate,
    newTab: newTab,
    setAttachment: setAttachment,
    on: function (type, fn) {
      if (!subscribers[type]) return function () {};
      subscribers[type].push(fn);
      return function () { subscribers[type] = subscribers[type].filter(function (f) { return f !== fn; }); };
    },
    // shown is called each time the pane comes into view: the first time
    // it loads the root (a changed root kept for the session, if this
    // browser may use one), later it restores the scroll and refreshes.
    shown: function () {
      if (destroyed) return;
      searchPaused = false;
      paintHead();
      if (!started) {
        started = true;
        var stored = null;
        try { stored = JSON.parse(load(TABS_KEY + rid) || "null"); } catch (e) { /* legacy root/View records still load */ }
        if (stored && Array.isArray(stored.tabs) && stored.tabs.length) {
          stored.tabs.forEach(function (s) { newTab(s || {}, true); });
          switchTab(Math.max(0, Math.min(tabs.length - 1, Math.floor(+stored.active) || 0)));
        } else newTab(h.location || { root: local() ? load(ROOT_KEY + rid) || "" : "", dir: "" });
        return;
      }
      if (!ui.tree.hidden) ui.tree.scrollTop = treeTop;
      else ui.hits.scrollTop = hitsTop;
      if (query().trim() && !searchResult) search(true);
      refresh();
    },
    // Leaving a window pauses only its own search; returning restarts any
    // unfinished query. Other visible explorers keep their requests.
    hidden: function () {
      if (destroyed) return;
      searchPaused = true;
      clearTimeout(timer);
      abort();
      closeMenu(false);
      closePop(false);
      persistTabs();
    },
    // The owner calls destroy before discarding its pane. Late list/root
    // responses are invalidated; search and global handlers are retired.
    destroy: function () {
      if (destroyed) return;
      persistTabs();
      destroyed = true;
      if (detachAttachment) detachAttachment();
      subscribers = { selection: [], navigation: [] };
      gen++;
      rootAttempt++;
      clearTimeout(timer);
      abort();
      closeMenu(false);
      closePop(false);
      listeners.forEach(function (off) { off(); });
      listeners = [];
      if (ui.root.parentNode) ui.root.parentNode.removeChild(ui.root);
      el.textContent = "";
    }
  };
  setAttachment(h.attachment);
  return api;
  }

  function openCard(location) {
    var c = cards.open("explorer", location || {});
    if (c) c.show();
    return c;
  }
  if (cards.register) cards.register("explorer", {
    base: 480,
    open: function (c) {
      var hooks = cardHooks, key = "ws.dashboard.explorer." + (hooks.rid || "") + "." + c.id, record = {};
      try { record = JSON.parse(hooks.load && hooks.load(key) || "{}"); } catch (e) { /* a bad browser record starts clean */ }
      if (!record || typeof record !== "object") record = {};
      var pane = doc.createElement("section"); pane.className = "fpane fcard"; c.body.appendChild(pane);
      c.title({ icon: "folder-open", name: wsT("js.board.toolbar.files") });
      c.explorerKey = key;
      c.clearExplorerState = function () { if (hooks.save) hooks.save(key, null); };
      c.explorer = create(pane, Object.assign({}, hooks, { location: c.data,
        load: function (k) { return record[k]; },
        save: function (k, v) { if (v == null) delete record[k]; else record[k] = v; if (hooks.save) hooks.save(key, JSON.stringify(record)); },
        onNavigate: function (s) { if (s) c.title({ icon: "folder-open", name: wsT("js.board.toolbar.files"), sub: c.explorer.location().dir || s.root }); }
      }));
      c.actions([{ icon: "external-link", cls: "bc-float", title: wsT("js.files.float"), run: function () { cards.float(c); } }]);
      c.explorer.shown();
    },
    close: function (c) { c.explorer.destroy(); c.clearExplorerState(); }
  });

  // Each owner's load/save adapter isolates root, View and tabs records.
  // Options also include location {root,dir}, send(path), openCard(location),
  // onSelection/onNavigate(payload, api), and attachment(slot, api) -> disposer.
  // A directoriesOnly consumer supplies roots {host,home,volumes}, an initial
  // location and an absolute routeBase (e.g. /s/host0/) when outside a session.
  // It has no file search/preview/actions and may notify null while loading.
  // on(type, listener) returns an unsubscribe; selection/navigation carry
  // {root,session,path,absolute}. setAttachment replaces/disposes the slot.
  // Existing root and bounded View records remain readable.
  window.WsFiles = { create: create, setup: function (hooks) { cardHooks = hooks || {}; }, open: openCard };
})();
