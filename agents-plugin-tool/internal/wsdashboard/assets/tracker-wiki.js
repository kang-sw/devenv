  // ------------------------------------------------------------ the wiki
  // The store's wiki branches (adapter.wiki) in the Items panel under the header's Items |
  // Wiki switch, or as the Wiki tab of the narrow layout: a picker of the store's wikis, the
  // page tree from the page paths, the page as the daemon rendered it, the shared editor in
  // its place, and the refused-save view (both texts, the lines that overlap marked) when a
  // save could not merge. Everything is read on demand and kept per store in ui.wiki; a save
  // re-reads the list, the pages, and the page.
  var WK_NAME = /^[a-z0-9][a-z0-9-]*$/, WK_NAME_MAX = 64;
  function wk() {
    return (ui.wiki[ui.store] = ui.wiki[ui.store] || {
      mode: 'items',        // 'items' | 'wiki': what the Items panel shows (the wide layouts)
      name: null,           // the wiki shown
      list: null, listErr: null, listBusy: false, // the store's wikis [{name, tip, unpushed}], or why they could not be read
      pages: null, pagesErr: null, pagesBusy: false, // {paths, commit} of the wiki shown
      page: null,           // the page path shown
      doc: null, docErr: null, docBusy: false, // {path, kind, content, commit, html} of the page shown; kind 'html' is an HTML page (wkFrameSync)
      edit: null,           // { path, isNew, newWiki, base, text, orig, newPath, note } while the editor is open; orig is the text the edit started from
      conflict: null,       // { c: the adapter's conflict, mine: the refused text } while the refused-save view shows
      saving: false,
      creating: false, wname: '', wfirst: 'index', // the "New wiki" form
      q: '', tree: true, closed: {}, ntree: false, // search text, tree shown (wide), folded dirs "wiki:dir", narrow tree open
      pick: false, more: false, // the picker and the page menu
      flash: false, top: false, full: false // a page just opened flashes its head and starts at the top; the refused view shows the whole text
    });
  }
  function wkIc(id, cls) { return '<svg class="i' + (cls ? ' ' + cls : '') + '"><use href="#tki-' + id + '"/></svg>'; }
  function wkSha(c) { return String(c || '').slice(0, 7); }
  function wkSwitch() {
    var m = wk().mode;
    return '<span class="wk-sw" role="group" aria-label="' + esc(wsT("js.tk.wk.sw.aria")) + '">' +
      '<button type="button"' + attr('act', 'wk-mode') + attr('mode', 'items') + ' aria-pressed="' + (m === 'items') + '">' + wkIc('tasks') + esc(wsT("js.tk.tab.items")) + '</button>' +
      '<button type="button"' + attr('act', 'wk-mode') + attr('mode', 'wiki') + ' aria-pressed="' + (m === 'wiki') + '">' + wkIc('book') + esc(wsT("js.tk.wk.tab")) + '</button></span>';
  }
  // the wiki an editor's preview renders in: the one shown, once it exists
  function wkRenderName() { var st = wk(); return st.edit && !st.edit.newWiki && st.name ? st.name : undefined; }
  function wkHas(st, path) { return !!(st.pages && st.pages.paths.indexOf(wkCanon(path)) >= 0); }
  // A page path as the daemon answers it (CanonicalWikiPath): name.md is the Markdown page
  // name, so a wiki:<name>/x.md link and a typed x.md find page x; any other path is left as
  // it is, the daemon refusing what it does not accept (a.html.md).
  function wkCanon(path) {
    var p = String(path || ''), m = /^(.*?)([^/]+)\.md$/.exec(p);
    return m && !/\.(md|html)$/i.test(m[2]) ? m[1] + m[2] : p;
  }
  // An HTML page's path ends in exactly .html after a non-empty name; the extension decides the kind.
  function wkIsHtml(path) { return /[^/]\.html$/.test(String(path || '')); }
  // Reads land on the state object they started from, for the wiki (and page) still shown; a
  // read for one no longer shown clears its busy flag and reads what is shown now instead.
  // wkDone wraps a read's answer: it clears the busy flag of read k ('list' | 'pages' |
  // 'doc'), drops an answer for a state no longer kept, and runs read k once more when a
  // change signal arrived while it was in flight (wkReread marked it), since the answer
  // predates that change. A store no longer shown is not re-read: its re-read is a soft one,
  // whose 404 for a page not yet read would set docErr and keep wkPull from opening the new
  // page when the store shows again; what it is missing then is read by wkPull.
  function wkDone(st, store, k, fn) {
    return function (res) {
      st[k + 'Busy'] = false;
      if (ui.wiki[store] !== st) return;
      fn(res);
      if (st[k + 'Again']) { st[k + 'Again'] = false; if (ui.store === store) wkReread(st, store, k); }
    };
  }
  // A read the daemon answers 503 found the store busy (a fetch or push holds it) and runs
  // again after the daemon's Retry-After (WK_RETRY_MS: the adapter passes on the status, not
  // the header) instead of drawing the error, up to WK_RETRIES times in a row. Read k stays
  // busy meanwhile, so a change signal marks it to run again (wkReread) rather than starting
  // a second one; again() decides what still to read when the wait ends.
  var WK_RETRY_MS = 1000, WK_RETRIES = 5;
  function wkRetry(st, store, k, res, again) {
    if (res.ok || res.status !== 503 || (st[k + 'Tries'] || 0) >= WK_RETRIES) { st[k + 'Tries'] = 0; return false; }
    st[k + 'Tries'] = (st[k + 'Tries'] || 0) + 1;
    st[k + 'Busy'] = true;
    setTimeout(function () {
      st[k + 'Busy'] = false;
      if (ui.wiki[store] === st && ui.store === store) again();
    }, WK_RETRY_MS);
    return true;
  }
  function wkLoadList(st, store) {
    if (st.listBusy) return;
    st.listBusy = true; st.listAgain = false;
    AD.wiki.list(store, wkDone(st, store, 'list', function (res) {
      if (wkRetry(st, store, 'list', res, function () { wkLoadList(st, store); })) return;
      if (!res.ok) { st.listErr = res.error; if (!st.list) st.list = []; render(); return; }
      st.list = res.wikis; st.listErr = null;
      if (st.name && !st.list.some(function (w) { return w.name === st.name; }) && !(st.edit && st.edit.newWiki)) { st.name = null; st.pages = null; st.page = null; st.doc = null; }
      if (!st.name && st.list.length) st.name = st.list[0].name;
      render();
    }));
  }
  function wkLoadPages(st, store, name) {
    if (st.pagesBusy) return;
    st.pagesBusy = true; st.pagesAgain = false;
    AD.wiki.pages(store, name, wkDone(st, store, 'pages', function (res) {
      if (st.name !== name) { if (ui.store === store) wkPull(st); return; }
      if (wkRetry(st, store, 'pages', res, function () { if (st.name === name) wkLoadPages(st, store, name); else wkPull(st); })) return;
      if (!res.ok) { st.pagesErr = res.error; if (!st.pages) st.pages = { paths: [], commit: '' }; render(); return; }
      st.pages = { paths: res.pages.slice(), commit: res.commit }; st.pagesErr = null;
      if (!st.page && st.pages.paths.length) st.page = wkHas(st, 'index') ? 'index' : st.pages.paths[0];
      render();
    }));
  }
  // soft: a re-read of the page shown; one that is gone shows as an error, not as a new page
  function wkLoadDoc(st, store, name, path, soft) {
    if (st.docBusy) return;
    st.docBusy = true; st.docAgain = false;
    AD.wiki.read(store, name, path, wkDone(st, store, 'doc', function (res) {
      if (st.name !== name || st.page !== path) { if (ui.store === store) wkPull(st); return; }
      if (wkRetry(st, store, 'doc', res, function () { if (st.name === name && st.page === path && !st.edit) wkLoadDoc(st, store, name, path, soft); else wkPull(st); })) return;
      if (!res.ok) {
        // a page that is not there (a wiki: link to it) opens as a new page at that path; the
        // editor opens on the store shown, so a store switched away from meanwhile reads the
        // page again when it shows
        if (res.status === 404 && !st.edit && !soft) { st.doc = null; st.docErr = null; if (ui.store === store) wkStartEdit(path, true, false); return; }
        st.docErr = res.error; render(); return;
      }
      st.doc = { path: res.path, kind: res.kind === 'html' ? 'html' : 'markdown', content: res.content, commit: res.commit, html: res.html }; st.docErr = null;
      render();
    }));
  }
  // What the panel shows but has not read yet is read now; the draw shows what is there.
  function wkPull(st) {
    var store = ui.store;
    if (st.list === null) wkLoadList(st, store);
    if (st.name && st.pages === null) wkLoadPages(st, store, st.name);
    if (st.name && st.page && st.doc === null && !st.docErr && !st.edit) wkLoadDoc(st, store, st.name, st.page);
  }
  function wkReload(st) {
    st.list = null; st.pages = null; st.doc = null; st.docErr = null;
    wkPull(st);
  }
  // A tracker change (a save by an agent, by another browser, or one the host's poll round
  // brought in) re-reads the wiki shown; what arrives replaces what is drawn, the page
  // excepted while it is edited or its refused save shows.
  function wkRefresh() {
    if (!AD.wiki || !ui.store) return;
    var st = ui.wiki[ui.store], store = ui.store;
    if (!st || st.list === null || !(st.mode === 'wiki' || (ui.layout === 'narrow' && tabState().narrow === 'wiki'))) return;
    wkReread(st, store, 'list');
    wkReread(st, store, 'pages');
    wkReread(st, store, 'doc');
  }
  // One read of a change signal; a read k already in flight started before the change, so it
  // is marked to run again when it lands (wkDone) instead of being dropped.
  function wkReread(st, store, k) {
    if (st[k + 'Busy']) { st[k + 'Again'] = true; return; }
    if (k === 'list') wkLoadList(st, store);
    else if (k === 'pages') { if (st.name && !(st.edit && st.edit.newWiki)) wkLoadPages(st, store, st.name); }
    else if (st.name && st.page && !st.edit && !st.conflict) wkLoadDoc(st, store, st.name, st.page, true);
  }

  function wkPicker() {
    var st = wk(), pop = '';
    if (st.pick) {
      pop = '<div class="wk-pop" role="menu">' + (st.list || []).map(function (w) {
        return '<button type="button" class="wk-opt' + (w.name === st.name ? ' on' : '') + '" role="menuitem"' + attr('act', 'wk-pick') + attr('wiki', w.name) + '>' + wkIc('book') +
          '<span class="nm">' + esc(w.name) + '</span><span class="n">' + (w.name === st.name && st.pages ? esc(wsTn("js.tk.wk.pages", st.pages.paths.length)) : '') + '</span>' +
          '<span class="sub">' + esc(wsT("js.tk.wk.pick.tip", wkSha(w.tip))) + (w.unpushed ? ' &middot; ' + esc(wsT("js.tk.wk.pick.unpushed")) : '') + '</span></button>';
      }).join('') + '<div class="wk-popsep"></div><button type="button" class="wk-opt add" role="menuitem"' + attr('act', 'wk-newwiki') + '>' + wkIc('plus') + '<span>' + esc(wsT("js.tk.wk.pick.new")) + '</span></button>' +
        '<div class="wk-pophint">' + wsT("js.tk.wk.pick.hint") + '</div></div>';
    }
    return '<span class="wk-pickw"><button type="button" class="wk-pick"' + attr('act', 'wk-pickopen') + ' aria-haspopup="menu" aria-expanded="' + st.pick + '"' + tipAttr(wsT("js.tk.wk.pick.switch")) + '>' + wkIc('book') +
      '<span class="pre">wiki/</span><span class="nm">' + esc(st.name || '') + '</span>' + wkIc('chev') + '</button>' + pop + '</span>';
  }
  function wkSearchHtml() {
    var st = wk();
    return '<span class="psearch">' + wkIc('search') + '<input id="wksearch" type="text" role="searchbox" autocomplete="off" spellcheck="false" value="' + esc(st.q) + '" placeholder="' + esc(wsT("js.tk.wk.search", st.name || '')) + '" aria-label="' + esc(wsT("js.tk.wk.search.aria")) + '">' +
      (st.q ? '<button type="button" class="psx"' + attr('act', 'wk-qclear') + ' aria-label="' + esc(wsT("js.tk.panel.clearsearch")) + '">' + wkIc('x') + '</button>' : '') + '</span>';
  }
  // the header's tools: the picker, the page count, the tree toggle (wide), "new page", and the search box
  function wkTools(narrow) {
    var st = wk();
    if (st.list && !st.list.length && !(st.edit && st.edit.newWiki)) return '<span class="chip">' + esc(wsT("js.tk.wk.nowiki")) + '</span>';
    if (!st.name) return '<span class="chip">' + esc(wsT("js.tk.loading")) + '</span>';
    return wkPicker() + (st.pages && !narrow ? '<span class="chip">' + esc(wsTn("js.tk.wk.pages", st.pages.paths.length)) + '</span>' : '') + '<span class="grow"></span>' +
      (narrow ? '' : '<button type="button" class="ia"' + attr('act', 'wk-tree') + ' aria-pressed="' + st.tree + '" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '"' + tipAttr(wsT(st.tree ? "js.tk.wk.tree.hide" : "js.tk.wk.tree.show")) + '>' + wkIc('sidebar') + '</button>') +
      '<button type="button" class="btn ib"' + attr('act', 'wk-newpage') + ' aria-label="' + esc(wsT("js.tk.wk.newpage")) + '"' + tipAttr(wsT("js.tk.wk.newpage")) + '>' + wkIc('plus') + '</button>' +
      '<div class="prow2">' + wkSearchHtml() + '</div>';
  }
  // The tree from the page paths: index first, then folders before pages, each folder foldable;
  // the search box filters by path (the texts are not here to search).
  function wkTreeHtml() {
    var st = wk(), q = st.q.trim().toLowerCase(), all = st.pages ? st.pages.paths : [], seen = {}, h = '';
    var paths = all.filter(function (p) { return !q || p.toLowerCase().indexOf(q) >= 0; }).sort(function (a, b) {
      if (a === 'index') return -1; if (b === 'index') return 1;
      var da = a.indexOf('/') >= 0, db = b.indexOf('/') >= 0;
      if (da !== db) return da ? -1 : 1;
      return a < b ? -1 : a > b ? 1 : 0;
    });
    function hl(t) { if (!q) return esc(t); var k = t.toLowerCase().indexOf(q); return k < 0 ? esc(t) : esc(t.slice(0, k)) + '<mark>' + esc(t.slice(k, k + q.length)) + '</mark>' + esc(t.slice(k + q.length)); }
    function folded(segs, upto) { for (var k = 0; k < upto; k++) if (st.closed[st.name + ':' + segs.slice(0, k + 1).join('/')]) return true; return false; }
    if (q) h += '<div class="wk-tcount">' + esc(wsT("js.tk.wk.match", paths.length, all.length)) + '</div>';
    paths.forEach(function (p) {
      var segs = p.split('/');
      for (var d = 0; d < segs.length - 1; d++) {
        var dir = segs.slice(0, d + 1).join('/');
        if (seen[dir]) continue;
        seen[dir] = 1;
        if (folded(segs, d)) continue;
        var open = !st.closed[st.name + ':' + dir];
        h += '<button type="button" class="wk-tn dir" style="--d:' + d + '"' + attr('act', 'wk-fold') + attr('dir', dir) + ' aria-expanded="' + open + '">' + wkIc('chev', 'cv') + wkIc('folder') + '<span class="t">' + esc(segs[d]) + '</span></button>';
      }
      if (folded(segs, segs.length - 1)) return;
      var on = p === st.page && !(st.edit && st.edit.isNew), ed = on && !!(st.edit || st.conflict);
      h += '<button type="button" class="wk-tn' + (on ? ' on' : '') + (ed ? ' ed' : '') + '" style="--d:' + (segs.length - 1) + '"' + attr('act', 'wk-open') + attr('path', p) + tipAttr(st.name + '/' + p) + (on ? ' aria-current="page"' : '') + '>' +
        wkIc('doc') + '<span class="t">' + hl(segs[segs.length - 1]) + '</span></button>';
    });
    if (q && !paths.length) h += '<div class="wk-tcount">' + esc(wsT("js.tk.wk.nomatch")) + '</div>';
    if (st.pagesErr) h += '<div class="wk-tcount err">' + esc(st.pagesErr) + '</div>';
    return h + '<button type="button" class="wk-tadd"' + attr('act', 'wk-newpage') + '>' + wkIc('plus') + esc(wsT("js.tk.wk.newpage")) + '</button>';
  }
  function wkCrumb(path) {
    var st = wk(), segs = String(path).split('/');
    return '<nav class="wk-path" aria-label="' + esc(wsT("js.tk.wk.path.aria")) + '"><a href="#"' + attr('act', 'wk-open') + attr('path', 'index') + '>' + esc(st.name) + '</a>' + segs.map(function (s, k) {
      return '<span class="sep">/</span>' + (k === segs.length - 1 ? '<b>' + esc(s) + '</b>' : '<span>' + esc(s) + '</span>');
    }).join('') + '</nav>';
  }
  function wkMoreMenu() {
    var st = wk();
    if (!st.more) return '';
    var link = 'wiki:' + st.name + '/' + st.page;
    return '<div class="wk-menu" role="menu">' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-copy') + attr('text', link) + '>' + wkIc('link') + '<span>' + esc(wsT("js.tk.wk.copylink")) + '<br><code>' + esc(link) + '</code></span></button>' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-copy') + attr('text', st.page) + '>' + wkIc('copy') + '<span>' + esc(wsT("js.tk.wk.copypath")) + '</span></button>' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-agent') + '>' + wkIc('history') + '<span>' + esc(wsT(XTAB.chat ? "js.tk.wk.history.agent" : "js.tk.wk.history.tools")) + '</span></button></div>';
  }
  function wkPageRead() {
    var st = wk(), d = st.doc;
    var head = '<div class="wk-phead' + (st.flash ? ' wk-flash' : '') + '">' + wkCrumb(st.page) + '<span class="grow"></span>';
    if (d) head += '<span class="wk-meta"' + tipAttr(wsT("js.tk.wk.commit.tip", wkSha(d.commit))) + '><span class="wk-sha">' + esc(wkSha(d.commit)) + '</span></span>' +
      '<button type="button" class="ia"' + attr('act', 'wk-edit') + ' aria-label="' + esc(wsT("js.tk.wk.edit")) + '"' + tipAttr(wsT("js.tk.wk.edit")) + '>' + wkIc('pen') + '</button>' +
      '<span class="wk-more"><button type="button" class="ia"' + attr('act', 'wk-more') + ' aria-label="' + esc(wsT("js.tk.wk.more")) + '" aria-haspopup="menu" aria-expanded="' + st.more + '"' + tipAttr(wsT("js.tk.wk.more")) + '>' + wkIc('more') + '</button>' + wkMoreMenu() + '</span>';
    head += '</div>';
    if (st.docErr) return head + '<div class="wk-empty"><p class="err">' + esc(wsT("js.tk.wk.fail", st.docErr)) + '</p></div>';
    if (!d) return head + '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>';
    // an HTML page's frame is not drawn here: the slot marks where the frame layer shows it
    if (d.kind === 'html') return head + '<div class="wk-fslot"' + attr('fkey', wkFrameKey()) + '></div>';
    return head + '<article class="wk-doc md">' + refLinksHtml(d.html) + '</article>';
  }
  function wkPageEdit() {
    var st = wk(), ed = st.edit, chord = MAC ? '⌘Enter' : 'Ctrl+Enter';
    var head = '<div class="wk-phead">' + (ed.isNew ? '<nav class="wk-path"><span>' + esc(st.name) + '</span><span class="sep">/</span><b>' + esc(wsT("js.tk.wk.new.title")) + '</b></nav>' : wkCrumb(ed.path)) +
      '<span class="grow"></span><span class="chip st-doing">' + esc(wsT("js.tk.wk.editing")) + '</span></div>';
    var path = ed.isNew ? '<label class="wk-epath">' + esc(wsT("js.tk.wk.epath")) + ' <input id="wk-epath" type="text" data-keep value="' + esc(ed.newPath) + '" spellcheck="false" autocomplete="off" aria-label="' + esc(wsT("js.tk.wk.epath.aria")) + '"></label>' : '';
    var base = ed.newWiki ? wsT("js.tk.wk.base.new") : wsT("js.tk.wk.base", wkSha(ed.base));
    var bar = '<div class="tb">' + btn('wk-save', wsT("js.tk.dlg.save"), {}, 'primary' + (st.saving ? ' busy' : '')) + btn('wk-cancel', wsT("js.tk.dlg.cancel"), {}) +
      '<span class="muted">' + esc(wsT("js.tk.wk.hint", chord)) + '</span><span class="wk-base"' + tipAttr(wsT("js.tk.wk.base.tip", wkSha(ed.base))) + '>' + esc(base) + '</span></div>';
    return head + '<div class="wk-edit">' + (ed.note ? '<div class="wk-note">' + wsT("js.tk.wk.note", '<code>' + esc(wkSha(ed.base)) + '</code>') + '</div>' : '') +
      (path ? '<div class="wk-etop">' + path + '</div>' : '') + editorHtml('wk-ebody', { value: ed.text, rows: 14, label: esc(wsT("js.tk.wk.ed.aria")), pv: 'wk-doc', src: wkEditHtml(ed) }) + bar + '</div>';
  }
  // An edit of an HTML page (its path, or a new page's typed path, ends in .html) is source
  // only: no preview (the saved page is its preview) and no images, which it cannot show.
  function wkEditHtml(ed) { return !!ed && wkIsHtml(ed.isNew ? wkNewPath(ed.newPath) : ed.path); }
  function wkNewPath(raw) { return wkCanon(String(raw || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/{2,}/g, '/')); }
  // A new page's seed: a Markdown heading, or an HTML page's document skeleton (the caret goes into its body).
  function wkSeed(path) {
    var name = String(path).split('/').pop();
    if (!wkIsHtml(path)) return '# ' + name.replace(/-/g, ' ') + '\n\n';
    return '<!DOCTYPE html>\n<html>\n<head>\n<meta charset="utf-8">\n<title>' + esc(name.slice(0, -5).replace(/-/g, ' ')) + '</title>\n</head>\n<body>\n\n</body>\n</html>\n';
  }
  // A new page's path typed into another kind changes the editor: an untouched seed becomes the
  // other kind's, and the draw adds or drops the preview.
  function wkPathKind(ed, raw) {
    var was = wkEditHtml(ed);
    ed.newPath = raw;
    if (wkEditHtml(ed) === was) return;
    if (!wkDirty(ed) && ed.seed !== undefined) ed.seed = ed.text = wkSeed(wkNewPath(raw));
    delete ui.edpv['wk-ebody'];
    render();
  }
  // Line regions of side that differ from base (an LCS alignment), as [{b0, b1: the base lines
  // they replace, lines: side's line indexes}]; wkMarks classes each side's lines 'conf' when
  // its region overlaps one of the other side (the lines that refused the merge) or 'chg'.
  function wkRegions(base, side) {
    // the lines both start and end with are aligned as they are; the table covers the rest
    var pre = 0, suf = 0, N = base.length, M = side.length;
    while (pre < N && pre < M && base[pre] === side[pre]) pre++;
    while (suf < N - pre && suf < M - pre && base[N - 1 - suf] === side[M - 1 - suf]) suf++;
    base = base.slice(pre, N - suf); side = side.slice(pre, M - suf);
    var n = base.length, m = side.length, T = [], i, j;
    for (i = 0; i <= n; i++) T.push(new Array(m + 1).fill(0));
    for (i = n - 1; i >= 0; i--) for (j = m - 1; j >= 0; j--) T[i][j] = base[i] === side[j] ? T[i + 1][j + 1] + 1 : Math.max(T[i + 1][j], T[i][j + 1]);
    var regs = [], cur = null;
    i = 0; j = 0;
    function close() { if (cur) { cur.b1 = i + pre; regs.push(cur); cur = null; } }
    while (i < n || j < m) {
      if (i < n && j < m && base[i] === side[j]) { close(); i++; j++; }
      else { if (!cur) cur = { b0: i + pre, lines: [] }; if (j < m && (i >= n || T[i][j + 1] >= T[i + 1][j])) { cur.lines.push(j + pre); j++; } else i++; }
    }
    close();
    return regs;
  }
  function wkMarks(base, a, b) {
    var ra = wkRegions(base, a), rb = wkRegions(base, b), ma = {}, mb = {}, conf = 0, other = 0;
    function hit(r, o) { return (r.b0 < o.b1 && o.b0 < r.b1) || r.b0 === o.b0; }
    ra.forEach(function (r) { var c = rb.some(function (o) { return hit(r, o); }); if (c) conf++; else other++; r.lines.forEach(function (k) { ma[k] = c ? 'conf' : 'chg'; }); });
    rb.forEach(function (r) { var c = ra.some(function (o) { return hit(r, o); }); if (!c) other++; r.lines.forEach(function (k) { mb[k] = c ? 'conf' : 'chg'; }); });
    return { a: ma, b: mb, conf: conf, other: other };
  }
  function wkRefused() {
    var st = wk(), c = st.conflict.c, base = String(st.conflict.orig || '').split('\n'), a = String(st.conflict.mine).split('\n'), b = String(c.theirsText || '').split('\n'), mk = wkMarks(base, a, b);
    // unchanged runs fold to a gap row (two lines of context around a change); a click shows the whole text
    function lines(arr, m) {
      var keep = arr.map(function (t, k) { if (st.full) return true; for (var d = -2; d <= 2; d++) if (m[k + d]) return true; return false; }), h = '', k = 0;
      while (k < arr.length) {
        if (keep[k]) { h += '<span class="wk-ln' + (m[k] ? ' ' + m[k] : '') + '" data-n="' + (k + 1) + '">' + (esc(arr[k]) || ' ') + '</span>'; k++; continue; }
        var j = k;
        while (j < arr.length && !keep[j]) j++;
        h += '<span class="wk-ln gap" data-n=""' + attr('act', 'wk-full') + tipAttr(wsT("js.tk.wk.gap.tip")) + '>&#8943; ' + esc(wsTn("js.tk.wk.gap", j - k)) + '</span>'; k = j;
      }
      return '<pre class="wk-lines">' + h + '</pre>';
    }
    var head = '<div class="wk-phead">' + wkCrumb(c.path || st.page) + '<span class="grow"></span><span class="chip wk-nosave">' + esc(wsT("js.tk.wk.notsaved")) + '</span></div>';
    return head + '<div class="wk-ref">' +
      '<div class="wk-refwhy">' + wkIc('warn') + '<div><b>' + esc(wsT("js.tk.wk.ref.title")) + '</b> ' + esc(c.reason ? String(c.reason).replace(/[\s.]*$/, '.') : wsT("js.tk.wk.ref.moved", wkSha(c.theirs), wkSha(c.base))) + ' ' +
      esc(wsTn("js.tk.wk.ref.conf", mk.conf)) + ', ' + esc(wsTn("js.tk.wk.ref.other", mk.other)) + '. ' + esc(wsT("js.tk.wk.ref.kept")) + '</div></div>' +
      '<div class="wk-refacts">' + btn('wk-copymine', wkIc('copy') + esc(wsT("js.tk.wk.copymine")), {}) + btn('wk-theirs', wkIc('reload') + esc(wsT("js.tk.wk.theirs")), {}) + btn('wk-again', wkIc('pen') + esc(wsT("js.tk.wk.again")), {}, 'primary') +
      '<span class="wk-key"><span><i class="k-conf"></i>' + esc(wsT("js.tk.wk.key.conf")) + '</span><span><i class="k-chg"></i>' + esc(wsT("js.tk.wk.key.chg")) + '</span><a href="#"' + attr('act', 'wk-full') + '>' + esc(wsT(st.full ? "js.tk.wk.changes" : "js.tk.wk.full")) + '</a></span></div>' +
      '<div class="wk-vs"><section class="wk-side"><header>' + esc(wsT("js.tk.wk.yours")) + '<span class="wk-sha">' + esc(wsT("js.tk.wk.from", wkSha(c.base))) + '</span><span class="grow"></span><span>' + esc(wsT("js.tk.wk.unsaved")) + '</span></header>' + lines(a, mk.a) + '</section>' +
      '<section class="wk-side"><header>' + esc(wsT("js.tk.wk.theirs.side")) + '<span class="wk-sha">' + esc(wkSha(c.theirs)) + '</span><span class="grow"></span></header>' + lines(b, mk.b) + '</section></div></div>';
  }
  // The empty store ("no wiki yet") and the "New wiki" form: a wiki is created by its first page.
  function wkEmptyHtml(create) {
    var st = wk(), ok = WK_NAME.test(st.wname) && st.wname.length <= WK_NAME_MAX;
    return '<div class="wk-empty">' + (create ? '' : wkIc('book') + '<h3>' + esc(wsT("js.tk.wk.empty.title")) + '</h3><p>' + wsT("js.tk.wk.empty.text") + '</p>') +
      (st.listErr ? '<p class="err">' + esc(wsT("js.tk.wk.fail", st.listErr)) + '</p>' : '') +
      '<form class="wk-form">' + (create ? '<b>' + esc(wsT("js.tk.wk.newwiki")) + '</b>' : '') +
      '<label class="wk-fld">' + esc(wsT("js.tk.wk.name")) + '<span class="wk-in"><span>wiki/</span><input id="wk-wname" type="text" data-keep value="' + esc(st.wname) + '" spellcheck="false" autocomplete="off"></span><span class="h' + (ok ? '' : ' bad') + '" id="wk-wnameh">' + esc(wsT(ok ? "js.tk.wk.name.ok" : "js.tk.wk.name.bad")) + '</span></label>' +
      '<label class="wk-fld">' + esc(wsT("js.tk.wk.first")) + '<span class="wk-in"><input id="wk-wfirst" type="text" data-keep value="' + esc(st.wfirst) + '" spellcheck="false" autocomplete="off" style="padding-left:8px"></span><span class="h">' + esc(wsT("js.tk.wk.first.hint")) + '</span></label>' +
      '<div class="tb">' + (create ? btn('wk-cancelnew', wsT("js.tk.dlg.cancel"), {}) : '') + btn('wk-create', wkIc('plus') + esc(wsT("js.tk.wk.create")), {}, 'primary') + '</div></form></div>';
  }
  function wkNameInput(el) {
    var st = wk(), h = $('wk-wnameh');
    st.wname = el.value;
    var ok = WK_NAME.test(st.wname) && st.wname.length <= WK_NAME_MAX;
    if (h) { h.className = 'h' + (ok ? '' : ' bad'); h.textContent = wsT(ok ? "js.tk.wk.name.ok" : "js.tk.wk.name.bad"); }
  }
  function wkPageHtml() {
    var st = wk();
    if (st.creating) return wkEmptyHtml(true);
    if (st.conflict) return wkRefused();
    if (st.edit) return wkPageEdit();
    if (!st.page) return st.pages && !st.pagesBusy ? wkEmptyPages() : '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>';
    return wkPageRead();
  }
  function wkEmptyPages() {
    return '<div class="wk-empty">' + wkIc('book') + '<h3>' + esc(wsT("js.tk.wk.nopages")) + '</h3><div class="tb">' + btn('wk-newpage', wkIc('plus') + esc(wsT("js.tk.wk.newpage")), {}, 'primary') + '</div></div>';
  }
  // The wiki as the Items panel's card spec (panelHtml), or the narrow layout's Wiki tab (paneHtml).
  function cardWiki(v, narrow) {
    var st = wk();
    wkPull(st);
    var none = st.list !== null && !st.list.length && !(st.edit && st.edit.newWiki), body;
    if (narrow) {
      body = none ? wkEmptyHtml(false) : !st.name ? '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>' :
        '<div class="wk-nartree"><button type="button" class="wk-tsel"' + attr('act', 'wk-ntree') + ' aria-expanded="' + st.ntree + '">' + wkIc('tree') + '<span>' + esc(wsT("js.tk.wk.tree.aria")) + '</span><code>' + esc(st.edit && st.edit.isNew ? wsT("js.tk.wk.new.title") : st.page || '') + '</code>' + wkIc('chev', 'cv') + '</button>' +
        (st.ntree ? '<nav class="wk-tree" id="wktree" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '">' + wkTreeHtml() + '</nav>' : '') + '</div><div class="wk-page" id="wkpage">' + wkPageHtml() + '</div>';
      return { tools: wkTools(true), body: body, id: 'card-wiki', cls: '', bodyCls: 'wk-nbody' };
    }
    if (none) return { tools: wkTools(false), body: wkEmptyHtml(false), id: 'card-items', cls: 'panel', bodyCls: 'wk-body notree' };
    if (!st.name) return { tools: wkTools(false), body: '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>', id: 'card-items', cls: 'panel', bodyCls: 'wk-body notree' };
    body = '<nav class="wk-tree" id="wktree" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '">' + wkTreeHtml() + '</nav><div class="wk-page" id="wkpage">' + wkPageHtml() + '</div>';
    return { tools: wkTools(false), body: body, id: 'card-items', cls: 'panel', bodyCls: 'wk-body' + (st.tree ? '' : ' notree') + (ui.page ? ' wide' : '') };
  }
  // After a draw: the page and tree keep their scroll (a page just opened starts at the top), a
  // wiki: link in any rendered text gets its mark, and the flash runs once.
  function wkAfterDraw(body, scroll) {
    var st = ui.store ? wk() : null;
    ['wkpage', 'wktree'].forEach(function (k) { if ($(k) && scroll[k] && !(st && st.top && k === 'wkpage')) $(k).scrollTop = scroll[k]; });
    wkMarkLinks(body);
    if (st) { st.flash = false; st.top = false; }
  }
  function wkMarkLinks(root) {
    if (!AD.wiki || !root) return;
    var st = ui.store ? wk() : null;
    Array.prototype.forEach.call(root.querySelectorAll('a[data-wiki][data-page]'), function (a) {
      var name = a.getAttribute('data-wiki'), path = a.getAttribute('data-page'), miss = !!(st && st.name === name && st.pages && !wkHas(st, path));
      a.classList.add('wlink'); a.classList.toggle('miss', miss);
      if (!a.querySelector('svg.i')) a.insertAdjacentHTML('afterbegin', wkIc('book'));
      a.setAttribute('title', 'wiki:' + name + '/' + path + (miss ? ' (' + wsT("js.tk.wk.missing") + ')' : ''));
    });
  }
  // An HTML page (kind html) shows in the daemon's sandboxed iframe markup (doc.html, inserted
  // as given: the sandbox attribute is the host's), kept in the layer #tkframe outside the
  // subtree render() replaces. An iframe reloads whenever it is detached, so a frame drawn into
  // the page would restart the page's scripts on every draw. The read view leaves an empty
  // slot (.wk-fslot) in the page area instead, and after every draw wkFrameSync puts the layer
  // over it. The frame belongs to one key (store, wiki, path, commit): a draw that keeps the
  // key keeps the same iframe element; a draw without the slot (the item window, the editor or
  // refused-save view, the drawer closed, another panel, or the page's own menus open over it)
  // hides the layer and keeps the frame, so coming back does not reload it; a key change
  // empties the layer, and the new frame goes in once a slot for it shows. Nothing listens for
  // the frame's messages: the host takes none from a wiki page.
  var WKF = { key: '', slot: null, ro: null };
  function wkFrameKey() {
    var st = ui.store && ui.wiki[ui.store], d = st && st.doc;
    return d && d.kind === 'html' && d.html ? JSON.stringify([ui.store, st.name, d.path, d.commit]) : '';
  }
  // root: the subtree the draw filled, or null when nothing of the store shows
  function wkFrameSync(root) {
    var L = $('tkframe'), key = wkFrameKey(), st = key ? wk() : null, slot = null;
    if (key && root && !ui.win && !st.more && !st.pick) slot = root.querySelector('.wk-fslot');
    if (slot && slot.getAttribute('data-fkey') !== key) slot = null;
    if (WKF.key !== key && WKF.key) { L.innerHTML = ''; WKF.key = ''; }
    if (slot && !WKF.key) { L.innerHTML = st.doc.html; WKF.key = key; }
    WKF.slot = slot;
    L.hidden = !slot;
    // the slot moves and resizes with the drawer's opening, the layout, and the window
    // (an outer frame resizing this page fires its resize), not only with a draw
    if (WKF.ro) WKF.ro.disconnect();
    if (slot && window.ResizeObserver) {
      WKF.ro = WKF.ro || new ResizeObserver(wkFramePlace);
      for (var n = slot; n && n !== document.body; n = n.parentElement) if (n === slot || wkClips(n)) WKF.ro.observe(n);
    }
    wkFramePlace();
  }
  function wkClips(n) { var cs = getComputedStyle(n); return cs.overflowX !== 'visible' || cs.overflowY !== 'visible'; }
  // The layer covers the slot's box, clipped to what its scrolling and clipping ancestors show
  // (the drawer's clip while it opens).
  function wkFramePlace() {
    var L = $('tkframe'), s = WKF.slot;
    if (!s || L.hidden) return;
    var r = s.getBoundingClientRect(), t = r.top, l = r.left, b = r.bottom, rt = r.right;
    for (var n = s.parentElement; n && n !== document.body; n = n.parentElement) {
      if (!wkClips(n)) continue;
      var q = n.getBoundingClientRect();
      t = Math.max(t, q.top); l = Math.max(l, q.left); b = Math.min(b, q.bottom); rt = Math.min(rt, q.right);
    }
    var S = L.style;
    S.left = r.left + 'px'; S.top = r.top + 'px'; S.width = r.width + 'px'; S.height = r.height + 'px';
    S.clipPath = b > t && rt > l ? 'inset(' + (t - r.top) + 'px ' + (r.right - rt) + 'px ' + (r.bottom - b) + 'px ' + (l - r.left) + 'px)' : 'inset(50%)';
  }
  // The panel shows the wiki: the Items | Wiki switch in the wide layouts, the Wiki tab when narrow.
  function wkShow() { if (ui.layout === 'narrow') { tabState().narrow = 'wiki'; } else wk().mode = 'wiki'; }
  // An edit is dirty when its text left where it started: the page's text, the seeded heading of
  // a new page (seed; orig stays '' as the conflict view's merge base), or the refused-save
  // "theirs" text after Edit again.
  function wkDirty(ed) { return !!ed && ed.text !== (ed.seed !== undefined ? ed.seed : ed.orig); }
  // Asked before a navigation that drops the wiki edit; false keeps the editor, draft, and page.
  // Cancel, Esc, and taking theirs drop the draft on purpose and do not ask.
  function wkLeaveOk() { return !wkDirty(wk().edit) || window.confirm(wsT("js.tk.wk.leave")); }
  function wkOpenPage(name, path) {
    if (wkLeaveOk()) wkGoPage(name, path);
  }
  function wkGoPage(name, path) {
    var st = wk();
    path = wkCanon(path); // the page list and the read answer the canonical path
    st.creating = false; st.edit = null; st.conflict = null; st.more = false; st.pick = false; st.ntree = false;
    if (st.name !== name) { st.name = name; st.pages = null; st.q = ''; }
    st.page = path; st.doc = null; st.docErr = null; st.flash = true; st.top = true;
    // a link into a wiki the store does not have yet starts that wiki with this page
    if (st.list && !st.list.some(function (w) { return w.name === name; })) { st.pages = { paths: [], commit: '' }; wkStartEdit(path, true, true); return; }
    if (st.pages && !wkHas(st, path)) { wkStartEdit(path, true, false); return; }
    wkShow();
    render();
  }
  function wkOpenLink(name, path) {
    if (!wkLeaveOk()) return; // declining leaves the item window open too
    if (ui.win) closeItemWindow();
    wkGoPage(name, path);
  }
  function wkStartEdit(path, isNew, newWiki) {
    var st = wk();
    st.creating = false; st.conflict = null; st.more = false; st.pick = false; st.ntree = false;
    delete ui.edpv['wk-ebody']; delete ui.eatt['wk-ebody'];
    var seed = wkSeed(path);
    if (isNew) st.edit = { path: path, isNew: true, newWiki: !!newWiki, base: st.pages ? st.pages.commit : '', text: seed, orig: '', seed: seed, newPath: path, note: false };
    else st.edit = { path: path, isNew: false, newWiki: false, base: st.doc.commit, text: st.doc.content, orig: st.doc.content, newPath: '', note: false };
    wkShow();
    render();
    var f = $(isNew && (!path || path.slice(-1) === '/') ? 'wk-epath' : 'wk-ebody');
    if (f) {
      f.focus({ preventScroll: true });
      var at = f.value.length, body = isNew && wkIsHtml(path) ? f.value.indexOf('\n</body>') : -1;
      if (body >= 0) at = body;
      if (f.id === 'wk-ebody') { try { f.setSelectionRange(at, at); } catch (x) { /* not a text field */ } }
    }
  }
  function wkCancel() {
    var st = wk();
    if (!st.edit) return;
    var ed = st.edit;
    st.edit = null; st.conflict = null;
    if (ed.newWiki) { st.name = null; st.page = null; st.pages = null; st.doc = null; st.list = null; }
    else if (ed.isNew) { st.page = wkHas(st, 'index') ? 'index' : (st.pages && st.pages.paths[0]) || null; st.doc = null; }
    render();
  }
  function wkCreate() {
    var st = wk(), name = ($('wk-wname') ? $('wk-wname').value : st.wname).trim(), first = ($('wk-wfirst') ? $('wk-wfirst').value : st.wfirst).trim().replace(/^\/+|\/+$/g, '') || 'index';
    st.wname = name; st.wfirst = first;
    if (!WK_NAME.test(name) || name.length > WK_NAME_MAX) return toast(esc(wsT("js.tk.wk.toast.name")));
    if (st.list && st.list.some(function (w) { return w.name === name; })) { st.creating = false; wkOpenPage(name, first); return; }
    st.name = name; st.pages = { paths: [], commit: '' }; st.page = first; st.doc = null; st.docErr = null;
    wkStartEdit(first, true, true);
  }
  function wkSave() {
    var st = wk(), ed = st.edit, store = ui.store, name = st.name;
    if (!ed || st.saving || !name) return;
    var box = $('wk-ebody');
    if (box) ed.text = box.value;
    var path = ed.isNew ? wkNewPath($('wk-epath') ? $('wk-epath').value : ed.newPath) : ed.path;
    if (!path) return toast(esc(wsT("js.tk.wk.toast.path")));
    if (ed.isNew && wkHas(st, path)) return toast(esc(wsT("js.tk.wk.toast.exists", path)));
    st.saving = true; render();
    AD.wiki.save(store, name, { base: ed.newWiki ? '' : ed.base, newWiki: !!ed.newWiki, op: 'write', path: path, content: ed.text, attachments: wkIsHtml(path) ? [] : savedAtts('wk-ebody', ed.text) }, function (res) {
      st.saving = false;
      var live = ui.wiki[store] === st && st.edit === ed; // the editor this save left from still shows
      if (res.ok) {
        toast(esc(wsT(res.merged ? "js.tk.wk.saved.merged" : "js.tk.wk.saved", wkSha(res.commit))) + (res.warnings || []).map(function (w) { return '<br>' + esc(w); }).join(''));
        if (live) { st.edit = null; st.conflict = null; st.page = path; st.flash = true; st.top = true; }
        // what the save changed is re-read on the next draw of this store's wiki
        st.list = null; st.pages = null; st.doc = null; st.docErr = null;
        render(); return;
      }
      if (!live) { if (!res.conflict) toast(esc(wsT("js.tk.wk.toast.fail", res.error))); return; }
      if (res.conflict) { st.conflict = { c: res.conflict, mine: ed.text, orig: ed.orig }; st.full = false; render(); return; }
      toast(esc(wsT("js.tk.wk.toast.fail", res.error))); render();
    });
  }
  function wkAct(k, a) {
    var st = wk();
    if (k === 'wk-mode') { st.mode = a.getAttribute('data-mode'); st.pick = st.more = false; render(); }
    else if (k === 'wk-pickopen') { st.pick = !st.pick; st.more = false; render(); }
    else if (k === 'wk-pick') { st.pick = false; var nm2 = a.getAttribute('data-wiki'); if (nm2 !== st.name && wkLeaveOk()) { st.name = nm2; st.pages = null; st.page = null; st.doc = null; st.docErr = null; st.edit = null; st.conflict = null; st.q = ''; st.creating = false; } render(); }
    else if (k === 'wk-newwiki') { st.pick = false; if (!wkLeaveOk()) { render(); return; } st.creating = true; st.edit = null; st.conflict = null; st.wname = ''; render(); if ($('wk-wname')) $('wk-wname').focus(); }
    else if (k === 'wk-cancelnew') { st.creating = false; render(); }
    else if (k === 'wk-create') wkCreate();
    else if (k === 'wk-open') wkOpenPage(st.name, a.getAttribute('data-path'));
    else if (k === 'wk-fold') { var fk = st.name + ':' + a.getAttribute('data-dir'); st.closed[fk] = !st.closed[fk]; render(); }
    else if (k === 'wk-tree') { st.tree = !st.tree; render(); }
    else if (k === 'wk-ntree') { st.ntree = !st.ntree; render(); }
    else if (k === 'wk-qclear') { st.q = ''; render(); if ($('wksearch')) $('wksearch').focus(); }
    else if (k === 'wk-more') { st.more = !st.more; st.pick = false; render(); }
    else if (k === 'wk-edit') { if (st.doc) wkStartEdit(st.page, false, false); }
    else if (k === 'wk-newpage') { if (st.name) wkStartEdit(st.page && st.page.indexOf('/') >= 0 ? st.page.slice(0, st.page.lastIndexOf('/') + 1) : '', true, false); }
    else if (k === 'wk-save') wkSave();
    else if (k === 'wk-cancel') wkCancel();
    else if (k === 'wk-copy' || k === 'wk-copymine') {
      var text = k === 'wk-copymine' ? st.conflict.mine : a.getAttribute('data-text');
      st.more = false;
      copyText(text, a.parentNode, function (ok) { toast(esc(ok ? wsT("js.toast.copied", k === 'wk-copymine' ? wsT("js.tk.wk.yours") : text) : wsT("js.tk.win.copyfail"))); if (k === 'wk-copy') render(); });
    }
    else if (k === 'wk-agent') {
      st.more = false;
      if (XTAB.chat) { if (ui.layout === 'narrow') tabState().narrow = 'chat'; else tabState().side = 'chat'; }
      else toast(esc(wsT("js.tk.wk.history.tools")));
      render();
    }
    else if (k === 'wk-theirs') { var c = st.conflict.c; st.conflict = null; st.edit = null; st.doc = null; st.docErr = null; toast(esc(wsT("js.tk.wk.reloaded", wkSha(c.theirs)))); wkReload(st); render(); }
    else if (k === 'wk-again') {
      var cf = st.conflict;
      st.conflict = null;
      st.edit = { path: cf.c.path || st.page, isNew: !cf.c.theirs, newWiki: !cf.c.theirs, base: cf.c.theirs || '', text: cf.mine, orig: cf.c.theirsText || '', newPath: cf.c.path || st.page, note: !!cf.c.theirs };
      delete ui.edpv['wk-ebody'];
      render();
      if ($('wk-ebody')) $('wk-ebody').focus();
    }
    else if (k === 'wk-full') { st.full = !st.full; render(); }
  }

  // The store settings: the store-wide values that are not roles, apart from the roles-event
  // dialog so they can be found. Every member reads them here; an admin's Edit opens the
  // setting's own dialog.
  function settingsHtml(admin) {
    function row(label, value, act) {
      return '<div class="srow"><span class="slabel">' + label + '</span><span class="sval">' + value + '</span>' + (admin ? btn(act, wsT("js.tk.act.edit"), {}) : '') + '</div>';
    }
    var prio = VS.priority.names.map(function (x, i) { return i === VS.priority.default ? wsT("js.tk.team.default", esc(x)) : esc(x); }).join(' &lt; ');
    var ls = VS.labels || [];
    var labels = '<div class="srow"><span class="slabel">' + wsT("js.tk.set.labels") + '</span><span class="sval">' + (ls.length ? '' : '<span class="muted">' + wsT("js.tk.set.labels.none") + '</span>') + '</span>' +
      (admin ? btn('slabel', wsT("js.tk.set.labels.add"), {}) : '') + '</div>' + (ls.length ? '<ul class="slabels">' + ls.map(function (l) {
        return '<li>' + labelChip(l) + (admin ? '<span class="grow"></span>' + btn('slabel', wsT("js.tk.act.edit"), { label: l.id }) + btn('slabel-rm', wsT("js.tk.set.labels.remove"), { label: l.id }) : '') + '</li>';
      }).join('') + '</ul>' : '');
    return '<div class="tkset"><h3>' + wsT("js.tk.set.title") + '</h3><p class="muted">' + (admin ? wsT("js.tk.set.intro.admin") : wsT("js.tk.set.intro.member")) + '</p>' +
      row(wsT("js.tk.set.prio"), prio, 'sprio') + row(wsT("js.tk.set.att"), esc(fmtSize(VS.attachments.max_bytes)), 'satt') + labels + '</div>';
  }

  function cardTeam(v) {
    var me = VS.members[v] || {}, R = VS.viewer.reach, levels = {}, seen = {};
    Object.keys(R).forEach(function (k) { levels[R[k]] = (levels[R[k]] || 0) + 1; });
    var summary = wsT("js.tk.team.summary", esc(VS.admins.join(', ')), VS.viewer.lead ? wsT("js.tk.team.lead", esc(VS.viewer.lead)) : wsT("js.tk.team.root"),
      Object.keys(R).length ? wsT("js.tk.team.reach", Object.keys(levels).map(function (d) { return wsT("js.tk.team.reachat", levels[d], d); }).join(', ')) : wsT("js.tk.team.nofollowers"));
    function shortKey(k) {
      var w = String(k).split(/\s+/), b = w[1] || w[0];
      if (String(k).length <= 32) return String(k);
      return (w[1] ? w[0] + ' ' : '') + b.slice(0, 8) + '\u2026' + b.slice(-8);
    }
    function keyLine(k, rev) {
      return '<div class="kline' + (rev ? ' rev' : '') + '"><code' + tipAttr(k) + '>' + esc(shortKey(k)) + '</code>' + (rev ? '<span class="chip flag">' + wsT("js.tk.team.key.revoked") + '</span>' : '') +
        '<button type="button" class="idcopy"' + attr('act', 'idcopy') + attr('id', k) + ' aria-label="' + esc(wsT("js.tk.team.key.copy.aria")) + '"' + tipAttr(k) + '><svg class="i"><use href="#tki-copy"/></svg><span>' + wsT("js.tk.team.key.copy") + '</span></button></div>';
    }
    function node(id) {
      if (seen[id]) return '<li>' + wsT("js.tk.team.cycle", esc(id)) + '</li>';
      seen[id] = true;
      var m = VS.members[id], open = !!ui.mdet[ui.store + '|' + id];
      var chips = (id === v ? '<span class="chip you">' + wsT("js.tk.team.you") + '</span>' : '') + (m.admin ? '<span class="chip admin">' + wsT("js.tk.team.admin") + '</span>' : '') +
        (id === VS.viewer.lead ? '<span class="chip">' + wsT("js.tk.team.yourlead") + '</span>' : '') + (R[id] ? '<span class="chip" title="' + wsT("js.tk.team.levels.tip") + '">L' + R[id] + '</span>' : '') +
        (m.joined ? '<span class="chip" title="' + wsT("js.tk.team.joined.tip", esc(alias(m.joined))) + '">' + wsT("js.tk.team.joined") + '</span>' : '') +
        (m.revoked.length ? '<span class="chip flag" title="' + wsT("js.tk.team.revoked.tip") + '">' + wsT("js.tk.team.revoked", m.revoked.length) + '</span>' : '') +
        ((VS.clones[id] || []).length > 1 ? '<span class="chip" title="' + wsT("js.tk.team.clones.tip") + '">' + wsT("js.tk.team.clones", VS.clones[id].length) + '</span>' : '');
      return '<li><div class="node' + (id === v ? ' me' : '') + '">' + avHtml(id) + (m.profile.org ? '<span class="org">' + esc(m.profile.org) + '</span>' : '') +
        '<span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips +
        '<span class="grow"></span><button type="button" class="ia mdet"' + attr('act', 'mdet') + attr('who', id) + ' aria-expanded="' + (open ? 'true' : 'false') + '" aria-label="' + esc(wsT("js.tk.team.details")) + '"' + tipAttr(wsT("js.tk.team.details")) + '><svg class="i"><use href="#tki-more"/></svg></button></div>' +
        (open ? '<div class="mdetails">' + m.keys.map(function (k) { return keyLine(k, false); }).join('') + m.revoked.map(function (x) { return keyLine(x.key, true); }).join('') +
          (id === v || me.admin ? '<div>' + btn('profile', wsT("js.tk.team.editprofile"), { who: id }) + '</div>' : '') + '</div>' : '') +
        (m.children.length ? '<ul>' + m.children.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(VS.members).filter(function (id) { var m = VS.members[id], l = m.leads; return !m.removed && (!l.length || !VS.members[l[0]] || VS.members[l[0]].removed); });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (VS.rolesProblems.length) h += '<p class="err">' + wsT("js.tk.team.problems", esc(VS.rolesProblems.join('; '))) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var gone = Object.keys(VS.members).filter(function (id) { return VS.members[id].removed; });
    if (gone.length) h += '<p class="r2">' + wsT("js.tk.team.removed", esc(gone.join(', '))) + '</p>';
    var lost = Object.keys(VS.members).filter(function (id) { return !seen[id] && !VS.members[id].removed; });
    if (lost.length) h += '<p class="err">' + wsT("js.tk.team.unreachable", esc(lost.join(', '))) + '</p>';
    h += '<div class="toolbar" style="margin-top:8px">' + btn('admin', wsT("js.tk.team.adminbtn"), {}) + (X.teamTools ? X.teamTools(ui.store, VS, H) : '') +
      '<span class="muted">' + (me.admin ? wsT("js.tk.team.isadmin") : wsT("js.tk.team.notadmin")) + '</span></div>';
    h += settingsHtml(!!me.admin);
    var nMem = Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).length;
    return card(wsT("js.tk.tab.org"), wsTn("js.tk.team.members", nMem), h, '', 'card-org');
  }

  function kindFilter(it) { return ui.kind === 'request' ? !!it.pending.length : ui.kind === 'claimable' ? !!it.claimable : ui.kind === 'all' || it.kind === ui.kind; }
  function itemFilter(it) {
    if (!kindFilter(it)) return false;
    var s = it.status;
    if (ui.status === 'hidden') return it.hidden;
    if (it.hidden) return false;
    if (ui.status === 'open') return !wrapped(s);
    if (ui.status === 'done' || ui.status === 'in-review' || ui.status === 'dropped') return s === ui.status;
    if (ui.status === 'blocked') return s === 'blocked' || it.rollup.blocked.length;
    if (ui.status === 'idea') return s === 'idea';
    return true;
  }
  function sel(id, val, list) {
    return '<select id="' + id + '">' + list.map(function (x) {
      var k = Array.isArray(x) ? x[0] : x, l = Array.isArray(x) ? x[1] : x;
      return '<option value="' + esc(k) + '"' + (k === val ? ' selected' : '') + '>' + esc(l) + '</option>';
    }).join('') + '</select>';
  }
  function itemFilterBar(kindOnly) {
    return wsT("js.tk.filter.kind") + ' ' + sel('f-kind', ui.kind, [['all', wsT("js.tk.filter.all")], ['task', kindName('task')], ['topic', kindName('topic')], ['request', wsT("js.tk.filter.request")], ['claimable', wsT("js.tk.filter.claimable")]]) +
      (kindOnly ? '' : ' ' + wsT("js.tk.filter.status") + ' ' + sel('f-status', ui.status, [['all', wsT("js.tk.filter.all")], ['open', wsT("js.tk.filter.open")], ['idea', stLabel('idea')], ['blocked', stLabel('blocked')], ['in-review', stLabel('in-review')], ['done', stLabel('done')], ['dropped', stLabel('dropped')], ['hidden', wsT("js.tk.chip.hidden")]]));
  }
  function cardActivity() {
    var chip = wsT("js.tk.activity.chip", VS.stats.events, VS.stats.flagged) + (VS.stats.concurrent ? ', ' + wsT("js.tk.row.nconcurrent", VS.stats.concurrent) : ''), m = modeOf('activity');
    if (m !== 'list') return card(wsT("js.tk.tab.activity"), chip, modeBody('activity', m), picker('activity'), 'card-activity', m);
    var rows = VS.events.filter(function (n) { return (!ui.act.flagged || n.status !== 'applied') && (!ui.act.conc || n.markers.length); }), cap = 50, shown = rows.slice(-cap).reverse();
    var h = lastWrite();
    h += '<div class="toolbar">' + actChips() + '<span class="grow"></span><span class="muted">' + wsT("js.tk.activity.shown", shown.length, rows.length) + '</span></div>';
    h += shown.length ? '<ul class="plain">' + shown.map(function (n) { return evLine(n, false); }).join('') + '</ul>' : '<div class="empty">' + wsT("js.tk.activity.empty") + '</div>';
    return card(wsT("js.tk.tab.activity"), chip, h, picker('activity'), 'card-activity', m);
  }
  // The Activity filters "flagged only" and "concurrent only": toggle chips with the look of the
  // card's mode chips (list / activity timeline), the accent when on.
  function actChips() {
    return [['flagged', "js.tk.activity.flagged"], ['conc', "js.tk.activity.conc"]].map(function (x) {
      var on = !!ui.act[x[0]];
      return '<button type="button" class="btn mini' + (on ? ' primary' : '') + '" aria-pressed="' + on + '"' + attr('act', 'actf') + attr('f', x[0]) + attr('fk', 'actf:' + x[0]) + '>' + wsT(x[1]) + '</button>';
    }).join('');
  }
  function lastWrite() {
    if (!ui.last) return '';
    return '<div class="lastact">' + wsT("js.tk.activity.last", ui.last.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ' ' + (n.status === 'applied' ? '<span class="chip st-done">' + wsT("js.tk.activity.applied") + '</span>' : '<span class="chip flag">' + esc(n.status) + '</span> ' + esc(n.reason));
    }).join('; ')) + '</div>';
  }

