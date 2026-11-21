  // --- pooled worktrees: the drawer card over the worktrees route ---------
  // Each drawer opening reads the pool (worktrees.go): the list draws at
  // once, and while sizes are still measured a second read waits for them.
  // Leased worktrees come first, shown active; idle ones group by profile.
  // The cleanup button asks the session to clean the pool up, then closes
  // the drawer. The dashboard itself never removes a worktree.
  function wtGet(wait, done) {
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch("/s/" + rid + "/worktrees" + (wait ? "?wait=1" : ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (!r.ok) { done(false, s.trim() || "HTTP " + r.status); return; }
        try { done(true, JSON.parse(s)); } catch (e) { done(false, "malformed answer"); }
      });
    }, function () { done(false, wsT("js.post.unreachable")); });
  }
  function wtLoad() {
    if (!wtlist) return;
    var seq = ++wtSeq;
    if (!wtlist.firstChild) wtlist.textContent = wsT("js.wt.loading");
    wtGet(false, function (ok, pool) {
      if (seq !== wtSeq) return;
      if (!ok) { wtFail(pool); return; }
      wtDraw(pool);
      if (pool.sizing) wtGet(true, function (ok2, sized) { if (seq === wtSeq && ok2) wtDraw(sized); });
    });
  }
  function wtFail(msg) {
    wtlist.textContent = "";
    wtlist.appendChild(mk("div", "wtempty", wsT("js.wt.fail", msg)));
  }
  // mk makes an element of class cls holding text.
  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  // wtSize is a byte count in decimal units, as the cleanup message gives it.
  function wtSize(n) {
    if (n < 1000) return n + " B";
    var v = n / 1000, i = 0;
    while (v >= 1000 && i < 3) { v /= 1000; i++; }
    return v.toFixed(1) + " " + "kMGT".charAt(i) + "B";
  }
  // wtAge is a coarse age: its largest unit only.
  function wtAge(ms) {
    var m = Math.max(0, Math.floor((Date.now() - ms) / 60000));
    if (m < 60) return m + "m";
    if (m < 48 * 60) return Math.floor(m / 60) + "h";
    return Math.floor(m / 1440) + "d";
  }
  function wtStamp(ms) { return new Date(ms).toLocaleString(wsI18n.lang); }
  function wtRow(w) {
    var row = mk("div", "wtrow" + (w.leased ? " on" : "") + (w.quarantined ? " quar" : ""));
    row.title = w.path + (w.quarantined ? "\n" + wsT("js.wt.quarantined") : "");
    row.appendChild(mk("span", "wtdot"));
    row.appendChild(mk("b", "wtname", w.name));
    if (w.leased) {
      var br = mk("span", "wtbr", w.detached ? wsT("js.wt.detached") : w.branch);
      if (!w.detached) br.title = w.branch; // a long branch ellipsizes
      row.appendChild(br);
      if (w.holder) {
        var who = mk("span", "wtwho", w.holder);
        who.title = wsT("js.wt.holder", w.holder);
        row.appendChild(who);
      }
    }
    var at = w.leased ? w.acquired_at || w.active_at : w.active_at;
    if (at) {
      var age = mk("span", "wtage", wtAge(at));
      age.title = wsT(w.leased ? "js.wt.held" : "js.wt.unused", wtStamp(at));
      row.appendChild(age);
    }
    row.appendChild(mk("span", "wtsize", w.size == null ? "…" : wtSize(w.size)));
    return row;
  }
  function wtDraw(pool) {
    var list = pool.worktrees || [], leased = [], groups = {}, order = [];
    list.forEach(function (w) {
      if (w.leased) { leased.push(w); return; }
      var p = w.profile || "?";
      if (!groups[p]) { groups[p] = []; order.push(p); }
      groups[p].push(w);
    });
    wtlist.textContent = "";
    wtlist.title = wsT("js.wt.pool", pool.pool || "");
    if (!list.length) wtlist.appendChild(mk("div", "wtempty", wsT("js.wt.empty")));
    if (leased.length) {
      var sec = mk("div", "wtsec on");
      leased.forEach(function (w) { sec.appendChild(wtRow(w)); });
      wtlist.appendChild(sec);
    }
    order.sort().forEach(function (p) {
      var sec = mk("div", "wtsec"), head = mk("div", "wtgrp");
      head.appendChild(mk("span", "wtprof", p));
      head.appendChild(mk("span", "chip", String(groups[p].length)));
      sec.appendChild(head);
      groups[p].forEach(function (w) { sec.appendChild(wtRow(w)); });
      wtlist.appendChild(sec);
    });
    if (wtcount) {
      wtcount.hidden = !list.length;
      wtcount.textContent = leased.length + "/" + list.length;
      wtcount.title = wsT("js.wt.count", leased.length, list.length);
    }
  }
  if (wtclean) wtclean.addEventListener("click", function () {
    if (wtclean.disabled) return;
    wtclean.disabled = true;
    if (wtnote) wtnote.textContent = "";
    post("worktrees/cleanup", {}, function (ok, res) {
      wtclean.disabled = false;
      if (!ok) { if (wtnote) wtnote.textContent = wsT("js.wt.cleanfail", res); return; }
      drSetOpen(false);
    });
  });

  // --- session holders: the drawer card over the holders route ------------
  // Each drawer opening reads the holders (holders.go): a row for each
  // holder no daemon is attached to and each one that is not restart-safe,
  // then an error row for each failed holder start. An unattached row has a
  // stop button when the route lets this viewer stop one (a browser on this
  // machine); it ends the holder and its child, then reads the list again.
  // The daemon itself never ends a holder.
  //
  // On a page that offers terminals (terminal.js), the card also lists the
  // live terminals whose opening session is gone from the index: attach
  // opens one as a card on this page, stop ends it after a confirm.
  var HD_CAUSE = { no_answer: "js.hd.cause.noanswer", protocol: "js.hd.cause.protocol", token: "js.hd.cause.token" };
  function hdLoad() {
    if (!hdlist || !window.fetch) return;
    var seq = ++hdSeq;
    if (!hdlist.firstChild) hdlist.textContent = wsT("js.hd.loading");
    fetch("/holders", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== hdSeq) return;
        if (!r.ok) { hdFail(s.trim() || "HTTP " + r.status); return; }
        try { hdDraw(JSON.parse(s)); } catch (e) { hdFail("malformed answer"); return; }
        hdTerms(seq);
      });
    }, function () { if (seq === hdSeq) hdFail(wsT("js.post.unreachable")); });
  }
  function hdTerms(seq) {
    if (!window.WsTerm) return;
    window.WsTerm.list(function (err, v) {
      if (seq !== hdSeq || err || !v || !(v.orphans || []).length) return;
      var box = mk("div", "hdterms");
      box.appendChild(mk("div", "hdsub", wsT("js.hd.term.title")));
      v.orphans.forEach(function (t) { box.appendChild(hdTermRow(t)); });
      hdlist.appendChild(box);
    });
    fetch("/s/host0/terminals", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) return null;
      return r.json();
    }).then(function (v) {
      if (seq !== hdSeq || !v || !v.anchor || !(v.terminals || []).length) return;
      var box = mk("div", "hdterms");
      box.appendChild(mk("div", "hdsub", wsT("js.hd.term.anchor")));
      v.terminals.forEach(function (t) { box.appendChild(hdTermRow(t)); });
      hdlist.appendChild(box);
    }).catch(function () {});
  }
  var hdTermClosing = new Set();
  function hdTermRow(t) {
    var row = mk("div", "wtrow hdrow hdterm"), meta = mk("span", "hdmeta"), acts = mk("span", "hdbtns");
    row.title = t.id;
    row.appendChild(mk("span", "wtdot"));
    var name = t.cwd ? t.cwd.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || t.cwd : t.id;
    row.appendChild(mk("b", "wtname", name));
    if (t.started_at) {
      var age = mk("span", "wtage", wtAge(t.started_at));
      age.title = wsT("js.hd.started", wtStamp(t.started_at));
      row.appendChild(age);
    }
    var cwd = mk("span", "hdcwd", t.cwd || "");
    cwd.title = t.cwd || "";
    meta.appendChild(cwd);
    if (t.state === "exited") meta.appendChild(mk("span", null, wsT("js.term.state.exited")));
    row.appendChild(meta);
    var at = mk("button", "btn hdstop", wsT("js.hd.term.attach"));
    at.type = "button";
    at.title = wsT("js.hd.term.attach.title");
    at.addEventListener("click", function () { window.WsTerm.attach(t); });
    var b = mk("button", "btn hdstop", wsT("js.hd.stop"));
    b.type = "button";
    b.title = wsT("js.hd.term.stop.title");
    b.addEventListener("click", async function () {
      var target = t.id, page = rid, key = JSON.stringify([page, target]);
      if (hdTermClosing.has(key)) return;
      if (t.state !== "exited" && !(await window.wsConfirm(wsT("js.term.confirm"), { opener: b }))) return;
      if (hdTermClosing.has(key) || !b.isConnected || rid !== page) return;
      hdTermClosing.add(key);
      b.disabled = true;
      if (hdnote) hdnote.textContent = "";
      post("terminals/" + encodeURIComponent(target) + "/close", {}, function (ok, res) {
        hdTermClosing.delete(key);
        b.disabled = false;
        if (rid !== page) return;
        if (!ok) { if (hdnote) hdnote.textContent = wsT("js.hd.stopfail", res); return; }
        hdLoad();
      });
    });
    acts.appendChild(at);
    acts.appendChild(b);
    row.appendChild(acts);
    return row;
  }
  function hdFail(msg) {
    hdlist.textContent = "";
    hdlist.appendChild(mk("div", "wtempty", wsT("js.hd.fail", msg)));
  }
  // hdLabel is a holder's session label, or its id without one.
  function hdLabel(h) {
    var l = h.session && typeof h.session.label === "string" ? h.session.label.trim() : "";
    return l || h.id;
  }
  function hdRow(h, canStop) {
    var row = mk("div", "wtrow hdrow" + (h.attached ? "" : " lost")), meta = mk("span", "hdmeta");
    row.title = h.id;
    row.appendChild(mk("span", "wtdot"));
    row.appendChild(mk("b", "wtname", hdLabel(h)));
    if (h.started_at) {
      var age = mk("span", "wtage", wtAge(h.started_at));
      age.title = wsT("js.hd.started", wtStamp(h.started_at));
      row.appendChild(age);
    }
    meta.appendChild(mk("span", null, wsT("js.hd.pid", h.pid)));
    if (!h.attached) meta.appendChild(mk("span", "hdcause", wsT(HD_CAUSE[h.cause] || "js.hd.cause.unknown", h.protocol)));
    if (h.restart_safe === false) {
      var rs = mk("span", "hdunsafe", wsT("js.hd.unsafe"));
      rs.title = wsT("js.hd.unsafe.title");
      meta.appendChild(rs);
    }
    row.appendChild(meta);
    if (!h.attached && canStop) {
      var b = mk("button", "btn hdstop", wsT("js.hd.stop"));
      b.type = "button";
      b.title = wsT("js.hd.stop.title");
      b.addEventListener("click", function () { hdStop(h.id, b); });
      row.appendChild(b);
    }
    return row;
  }
  function hdFailRow(f) {
    var row = mk("div", "wtrow hdrow hdfail");
    row.appendChild(mk("span", "wtdot"));
    var msg = mk("span", "hdmsg", f.message);
    msg.title = f.message;
    row.appendChild(msg);
    if (f.at) {
      var age = mk("span", "wtage", wtAge(f.at));
      age.title = wsT("js.hd.failed", wtStamp(f.at));
      row.appendChild(age);
    }
    return row;
  }
  function hdDraw(list) {
    var holders = list.holders || [], failures = list.failures || [], lost = 0;
    hdlist.textContent = "";
    holders.forEach(function (h) {
      if (!h.attached) lost++;
      if (!h.attached || h.restart_safe === false) hdlist.appendChild(hdRow(h, !!list.can_stop));
    });
    if (!lost) hdlist.insertBefore(mk("div", "wtempty", wsT("js.hd.empty")), hdlist.firstChild);
    failures.forEach(function (f) { hdlist.appendChild(hdFailRow(f)); });
    if (hdcount) {
      hdcount.hidden = !lost;
      hdcount.textContent = String(lost);
      hdcount.title = wsTn("js.hd.count", lost);
    }
  }
  function hdStop(id, b) {
    if (b.disabled) return;
    b.disabled = true;
    if (hdnote) hdnote.textContent = "";
    post("/holders/" + encodeURIComponent(id) + "/stop", {}, function (ok, res) {
      b.disabled = false;
      if (!ok) { if (hdnote) hdnote.textContent = wsT("js.hd.stopfail", res); return; }
      hdLoad();
    });
  }

  // --- git drawer: the action panel over the git routes ---------------------
  // Each opening of the git sheet asks the daemon to read the handle again
  // and reads the changed files (gitdrawer.go): the standing line, then the
  // staged, unstaged, and untracked groups, each file with its status and
  // line delta. Fetch, pull, and push post to their routes (gitpush.go), one
  // at a time: the pressed button shows busy and the others wait; the
  // daemon's refusal or git's failure shows on the panel's message line, a
  // pull's "nothing to pull" as a plain note. Every answer reads the files
  // again, and the graph with them; the handle's own re-read arrives
  // through the stream.
  var gref = $("gref"), gfiles = $("gfiles"), gmsg = $("gmsg"), gSeq = 0, gBusy = false, gitLast = null;
  function gitOpened() {
    if (!gbody || !window.fetch) return;
    fetch("/s/" + rid + "/git", { method: "POST", credentials: "same-origin", cache: "no-store" }).catch(function () {});
    gitLoad();
    glLoad(false);
  }
  function gitLoad() {
    var seq = ++gSeq;
    if (!gfiles.firstChild) gfiles.appendChild(mk("div", "gempty", wsT("js.git.loading")));
    fetch("/s/" + rid + "/git/status", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gSeq) return;
        if (!r.ok) { gitFail(s.trim() || "HTTP " + r.status); return; }
        try { gitDraw(JSON.parse(s)); } catch (e) { gitFail("malformed answer"); }
      });
    }, function () { if (seq === gSeq) gitFail(wsT("js.post.unreachable")); });
  }
  // gitFail shows a failed read in place of the files; the standing line
  // goes with them, since it is as old as the failed read says.
  function gitFail(msg) {
    gref.textContent = "";
    gfiles.textContent = "";
    gfiles.appendChild(mk("div", "gempty err", wsT("js.git.fail", msg)));
  }
  // gitRef draws the standing line: branch, upstream, and the counts.
  function gitRef(v) {
    gref.textContent = "";
    if (v.detached) { gref.appendChild(mk("b", "gbr", wsT("js.git.detached", v.branch))); return; }
    gref.appendChild(mk("b", "gbr", v.branch));
    if (!v.upstream) { gref.appendChild(mk("span", "gnoup", wsT("js.git.noupstream"))); return; }
    gref.appendChild(mk("span", "gto", "\u2192"));
    gref.appendChild(mk("span", "gup", v.upstream));
    if (v.ahead) gref.appendChild(mk("b", "up", "\u2191" + v.ahead));
    if (v.behind) gref.appendChild(mk("b", "dn", "\u2193" + v.behind));
  }
  function gitRow(f) {
    var row = mk("div", "gf"), slash = f.path.lastIndexOf("/");
    row.title = f.path;
    row.appendChild(mk("b", "gst st-" + (f.status === "?" ? "q" : f.status), f.status));
    var p = mk("span", "gp");
    if (slash >= 0) p.appendChild(mk("span", "gdir", f.path.slice(0, slash + 1)));
    p.appendChild(mk("span", "gname", f.path.slice(slash + 1)));
    row.appendChild(p);
    var d = mk("span", "gd");
    if (f.add < 0) d.appendChild(mk("i", "bin", "bin"));
    else if (f.status !== "?") {
      if (f.add) d.appendChild(mk("i", "add", "+" + f.add));
      if (f.del) d.appendChild(mk("i", "del", "\u2212" + f.del));
    }
    row.appendChild(d);
    return row;
  }
  function gitDraw(v) {
    gitLast = v;
    gitRef(v);
    gfiles.textContent = "";
    var groups = [["staged", "js.git.staged"], ["unstaged", "js.git.unstaged"], ["untracked", "js.git.untracked"]], any = false;
    groups.forEach(function (g) {
      var list = v[g[0]] || [];
      if (!list.length) return;
      any = true;
      var sec = mk("div", "gsec"), head = mk("div", "ggrp");
      head.appendChild(mk("span", "", wsT(g[1])));
      head.appendChild(mk("span", "chip", String(list.length)));
      sec.appendChild(head);
      list.forEach(function (f, i) {
        var row = gitRow(f);
        row.setAttribute("data-side", g[0]);
        row.setAttribute("data-i", String(i));
        sec.appendChild(row);
      });
      gfiles.appendChild(sec);
    });
    // A clean tree shows a quiet empty state centred in the list's room.
    gfiles.classList.toggle("clean", !any);
    if (!any) {
      var clean = mk("div", "gclean");
      clean.innerHTML = '<svg class="i" aria-hidden="true"><use href="#i-check"/></svg>';
      clean.appendChild(mk("span", "", wsT("js.git.clean")));
      gfiles.appendChild(clean);
    }
    if (v.truncated) gfiles.appendChild(mk("div", "gempty", wsT("js.git.truncated", 300)));
  }
  function gitMsg(text, err) {
    gmsg.textContent = text || "";
    gmsg.classList.toggle("err", !!err);
    gmsg.hidden = !text;
  }
  // gitAct runs one action from its button.
  function gitAct(btn) {
    if (gBusy || !window.fetch) return;
    var label = btn.querySelector("span"), idle = label.textContent;
    gBusy = true;
    q(".gact", gbody).forEach(function (b) { b.disabled = true; });
    btn.classList.add("busy");
    label.textContent = btn.getAttribute("data-busy");
    gitMsg("");
    function done(text, err) {
      gBusy = false;
      q(".gact", gbody).forEach(function (b) { b.disabled = false; });
      btn.classList.remove("busy");
      label.textContent = idle;
      gitMsg(text, err);
      gitLoad();
      glLoad(false);
    }
    fetch("/s/" + rid + "/git/" + btn.getAttribute("data-git"), { method: "POST", credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (t) {
        t = (t || "").trim();
        if (r.status === 204) done("", false);
        else if (r.ok) done(t, false);
        else done(t || "HTTP " + r.status, true);
      });
    }, function () { done(gmsg.getAttribute("data-lost"), true); });
  }
  if (gbody) gbody.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest(".gact");
    if (b && !b.disabled) gitAct(b);
    // A changed file opens the commit window on the working tree.
    var f = e.target.closest && e.target.closest(".gf[data-side]");
    if (f) gwWorktree(f.getAttribute("data-side"), +f.getAttribute("data-i"));
  });

  // --- git drawer: the commit graph and the commit window --------------------
  // The graph reads the session root's log a page at a time (gitlog.go, GET
  // git/log): every local branch, the upstreams they track, and detached
  // worktree HEADs, with the lanes the daemon laid out. A row's down
  // segments run from its node to the next row, so pages stack; the last
  // loaded row's run on dashed into the "more" row while a page remains.
  // The graph is read-only. It loads with the git sheet and after each
  // action, never while hidden (a narrow screen shows the panel alone). A
  // row's hover shows its tooltip; a click (or Enter) opens the commit
  // window (GET git/commit/<oid>), a centred window over the page: header,
  // message, author and committer, copy-hash, parents (a parent opens in the
  // same window), the changed-file list, and the diff viewer below; the
  // working-tree file list opens the same window.
  var glog = $("glog"), glrows = $("glrows"), glscroll = $("glscroll"), glcount = $("glcount");
  var GL_ROW = 26, GL_DX = 14, GL_X0 = 12, GL_MORE = 34;
  var gl = { commits: [], by: {}, width: 1, more: 0, page: 0, head: "", seq: 0, busy: false, loaded: false, tipRow: null };
  function svgEl(tag, attrs) {
    var e = doc.createElementNS(SVGNS, tag);
    for (var k in attrs) if (Object.prototype.hasOwnProperty.call(attrs, k)) e.setAttribute(k, attrs[k]);
    return e;
  }
  function gIcon(name) {
    var s = svgEl("svg", { "class": "i", "aria-hidden": "true" });
    s.appendChild(svgEl("use", { href: "#i-" + name }));
    return s;
  }
  // luIcon is a vendored Lucide icon (the page's l- sprite), sized as gIcon's.
  function luIcon(name) {
    var s = svgEl("svg", { "class": "i li", "aria-hidden": "true" });
    s.appendChild(svgEl("use", { href: "#l-" + name }));
    return s;
  }
  function glX(col) { return GL_X0 + col * GL_DX; }
  function glY(i) { return i * GL_ROW + GL_ROW / 2; }
  function glLane(color) { return "var(--gl-" + (color % 8) + ")"; }
  // glAgo is a short age: minutes, hours, days, then the date.
  function glAgo(ms) {
    var m = Math.max(0, Math.round((Date.now() - ms) / 60000));
    if (m < 60) return m + "m";
    var h = Math.round(m / 60);
    if (h < 48) return h + "h";
    var d = Math.round(h / 24);
    return d < 60 ? d + "d" : new Date(ms).toLocaleDateString(wsI18n.lang);
  }
  function glRel(ms) {
    var a = glAgo(ms);
    return /^\d+[mhd]$/.test(a) ? wsT("js.ago", a) : a;
  }
  function glWhen(ms) { return new Date(ms).toLocaleString(wsI18n.lang); }
  // glHandle is the drawer handle's abbreviation (gitdrawer.go branchHandle)
  // for an upstream's branch part; the daemon sends it for local branches.
  function glHandle(name) {
    var segs = name.split("/"), h = { text: name };
    if (segs.length < 2 || (segs[0] !== "impl" && segs[0] !== "goal") || !segs[segs.length - 1]) return h;
    h.text = segs[segs.length - 1];
    segs.slice(0, -1).forEach(function (s) { if (s === "impl") h.impl = true; if (s === "goal") h.goal = true; });
    return h;
  }
  function glIcons(el, h) {
    if (h.goal) { var g = mk("span", "gh-goal"); g.appendChild(gIcon("trophy")); el.appendChild(g); }
    if (h.impl) { var m = mk("span", "gh-impl"); m.appendChild(gIcon("hammer")); el.appendChild(m); }
    if (!h.goal && !h.impl) el.appendChild(gIcon("branch"));
  }
  function glBase(path) {
    var p = path.replace(/[\\/]+$/, "");
    return p.slice(Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\")) + 1) || p;
  }
  function glWt(el) { var w = mk("span", "wt"); w.appendChild(gIcon("folder")); el.appendChild(w); }
  // glPills draws a commit's ref labels: a local branch in its lane's colour
  // with its upstream folded in when that is on the same commit, an upstream
  // of its own otherwise, and a detached worktree HEAD.
  function glPills(c) {
    var out = [], refs = c.refs || [], folded = {};
    refs.forEach(function (r) {
      if (r.kind !== "branch" || !r.upstream) return;
      refs.forEach(function (u) { if (u.kind === "upstream" && u.name === r.upstream) folded[u.name] = r.name; });
    });
    refs.forEach(function (r) {
      var p, title = [];
      if (r.kind === "branch") {
        p = mk("span", "rf" + (r.head ? " head" : "") + (r.merged ? " merged" : ""));
        if (r.head) p.appendChild(mk("span", "hd", "HEAD"));
        glIcons(p, r);
        p.appendChild(mk("span", "t", r.text || r.name));
        if (r.ahead || r.behind) {
          var ab = mk("span", "ab");
          if (r.ahead) ab.appendChild(mk("b", "up", "↑" + r.ahead));
          if (r.behind) ab.appendChild(mk("b", "dn", "↓" + r.behind));
          p.appendChild(ab);
        }
        if (r.worktree) glWt(p);
        if (r.upstream && folded[r.upstream] === r.name) p.appendChild(mk("span", "ups", r.upstream.split("/")[0]));
        title.push(r.name);
        if (r.gone) title.push(wsT("js.git.ref.gone", r.upstream));
        else if (r.upstream) title.push(wsT("js.git.ref.upstream", r.upstream) + (r.ahead || r.behind ? " " + wsT("js.git.ref.ab", r.ahead || 0, r.behind || 0) : ""));
        else title.push(wsT("js.git.noupstream"));
        if (r.merged) title.push(wsT("js.git.ref.merged"));
      } else if (r.kind === "upstream") {
        if (folded[r.name]) return;
        var parts = r.name.split("/"), h = glHandle(parts.slice(1).join("/"));
        p = mk("span", "rf remote");
        glIcons(p, h);
        p.appendChild(mk("span", "t", parts[0] + "/" + h.text));
        title.push(r.name);
        if (r.of) title.push(wsT("js.git.ref.upof", r.of));
      } else {
        p = mk("span", "rf wtd" + (r.head ? " head" : ""));
        if (r.head) p.appendChild(mk("span", "hd", "HEAD"));
        glWt(p);
        p.appendChild(mk("span", "t", r.worktree ? glBase(r.worktree) : c.oid.slice(0, 7)));
        title.push(wsT("js.git.detached", c.oid.slice(0, 9)));
      }
      if (r.worktree) title.push(wsT("js.git.ref.worktree", r.worktree));
      if (r.head) title.push(wsT("js.git.ref.head"));
      p.style.setProperty("--lc", glLane(c.color));
      p.title = title.join("\n");
      out.push(p);
    });
    return out;
  }
  function glFail(msg, append) {
    // A failed further page keeps the rows already drawn and says so where
    // its "Load more" was.
    var more = append && gl.commits.length && q(".glmore", glrows)[0];
    if (more) { q(".glmore-b", more).forEach(function (b) { b.disabled = false; }); more.lastChild.textContent = wsT("js.git.log.fail", msg); return; }
    glcount.hidden = true;
    glrows.textContent = "";
    glrows.appendChild(mk("div", "gempty err", wsT("js.git.log.fail", msg)));
  }
  // glDraw draws every loaded row and one SVG of lanes and dots under them.
  function glDraw() {
    var cs = gl.commits, n = cs.length, keep = glscroll.scrollTop;
    glTipHide();
    glrows.textContent = "";
    gl.by = {};
    if (!n) { glcount.hidden = true; glrows.appendChild(mk("div", "gempty", wsT("js.git.log.empty"))); return; }
    var w = glX(gl.width - 1) + 12, H = n * GL_ROW;
    // The column stays wide enough for its header on a one-lane graph.
    glscroll.style.setProperty("--gw", Math.max(w, 52) + "px");
    glcount.hidden = false;
    glcount.textContent = n + (gl.more ? "+" : "");
    var svg = svgEl("svg", { "class": "glsvg", width: w, height: H + GL_MORE, "aria-hidden": "true" });
    var lines = svgEl("g", {}), dots = svgEl("g", {}), frag = doc.createDocumentFragment();
    cs.forEach(function (c, i) {
      gl.by[c.oid] = c;
      var last = i === n - 1;
      (c.down || []).forEach(function (s) {
        var x1 = glX(s.from), y1 = glY(i), x2 = glX(s.to), y2 = last ? H + GL_MORE / 2 : glY(i + 1), m = (y1 + y2) / 2;
        var a = { d: x1 === x2 ? "M" + x1 + " " + y1 + "V" + y2 : "M" + x1 + " " + y1 + "C" + x1 + " " + m + " " + x2 + " " + m + " " + x2 + " " + y2, stroke: glLane(s.color) };
        if (last) a["class"] = "cont";
        lines.appendChild(svgEl("path", a));
      });
      var x = glX(c.col), y = glY(i), lc = glLane(c.color), merge = c.parents.length > 1;
      if (c.oid === gl.head) dots.appendChild(svgEl("circle", { "class": "head", cx: x, cy: y, r: 8 }));
      if (c.remote) dots.appendChild(svgEl("circle", { "class": "rem", cx: x, cy: y, r: 4, stroke: lc }));
      else if (merge) dots.appendChild(svgEl("circle", { "class": "mg", cx: x, cy: y, r: 4, stroke: lc }));
      else dots.appendChild(svgEl("circle", { "class": "dot", cx: x, cy: y, r: 5, fill: lc }));
      var row = mk("div", "glr" + (merge ? " mg" : "") + (c.remote ? " rem" : "") + (c.oid === gw.oid && gw.back && !gw.back.hidden ? " sel" : ""));
      row.tabIndex = 0;
      row.setAttribute("data-i", String(i));
      row.appendChild(mk("span"));
      var sub = mk("span", "gls");
      glPills(c).forEach(function (p) { sub.appendChild(p); });
      sub.appendChild(mk("span", "glsub", c.subject));
      row.appendChild(sub);
      row.appendChild(mk("span", "gla", c.author));
      var t = mk("span", "glt", glAgo(c.time));
      t.title = glWhen(c.time);
      row.appendChild(t);
      row.appendChild(mk("span", "glh", c.oid.slice(0, 7)));
      frag.appendChild(row);
    });
    if (gl.more) {
      var more = mk("div", "glmore"), b = mk("button", "btn glmore-b", wsT("js.git.log.more"));
      b.type = "button";
      b.disabled = gl.busy;
      more.appendChild(b);
      more.appendChild(mk("span", "", wsT("js.git.log.morenote", gl.page)));
      frag.appendChild(more);
    }
    svg.appendChild(lines);
    svg.appendChild(dots);
    frag.appendChild(svg);
    glrows.appendChild(frag);
    glscroll.scrollTop = keep;
  }
  // glLoad reads the first page again, or (append) the next one.
  function glLoad(append) {
    if (!glog || !window.fetch) return;
    // Hidden (narrow layout): read it again once it shows.
    if (!glog.offsetWidth) { if (!append) gl.loaded = false; return; }
    if (append && (!gl.more || gl.busy)) return;
    gl.loaded = true;
    var seq = ++gl.seq, offset = append ? gl.more : 0;
    gl.busy = true;
    if (append) q(".glmore-b", glrows).forEach(function (b) { b.disabled = true; });
    else if (!gl.commits.length) { glrows.textContent = ""; glrows.appendChild(mk("div", "gempty", wsT("js.git.log.loading"))); }
    fetch("/s/" + rid + "/git/log" + (offset ? "?offset=" + offset : ""), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gl.seq) return;
        gl.busy = false;
        if (!r.ok) { glFail(s.trim() || "HTTP " + r.status, append); return; }
        var v;
        try { v = JSON.parse(s); } catch (e) { glFail("malformed answer", append); return; }
        if (!append) { gl.commits = []; gl.width = 1; gl.page = (v.commits || []).length; }
        gl.commits = gl.commits.concat(v.commits || []);
        gl.width = Math.max(gl.width, v.width || 1);
        gl.more = v.more || 0;
        gl.head = v.head || "";
        glDraw();
      });
    }, function () { if (seq === gl.seq) { gl.busy = false; glFail(wsT("js.post.unreachable"), append); } });
  }
  // The tooltip: subject, refs, author, age, short hash, under the row.
  var gltip = null;
  function glTipHide() { if (gltip) gltip.hidden = true; gl.tipRow = null; }
  function glTip(row) {
    if (row === gl.tipRow && gltip && !gltip.hidden) return;
    var c = gl.commits[+row.getAttribute("data-i")];
    if (!c) return;
    if (!gltip) { gltip = mk("div", "gltip"); gltip.hidden = true; doc.body.appendChild(gltip); }
    gl.tipRow = row;
    gltip.textContent = "";
    gltip.appendChild(mk("div", "ts", c.subject));
    var pills = glPills(c);
    if (pills.length) { var tr = mk("div", "tr"); pills.forEach(function (p) { tr.appendChild(p); }); gltip.appendChild(tr); }
    var tm = mk("div", "tm");
    [c.author, "·", glRel(c.time), "·"].forEach(function (s) { tm.appendChild(mk("span", "", s)); });
    tm.appendChild(mk("code", "", c.oid.slice(0, 9)));
    gltip.appendChild(tm);
    gltip.hidden = false;
    var r = row.getBoundingClientRect(), x = r.left + glX(c.col) + 14, y = r.bottom + 4, th = gltip.offsetHeight;
    if (y + th > window.innerHeight - 8) y = r.top - th - 4;
    gltip.style.left = Math.max(8, Math.min(x, window.innerWidth - gltip.offsetWidth - 8)) + "px";
    gltip.style.top = y + "px";
  }
  if (glrows) {
    glrows.addEventListener("mouseover", function (e) { var row = e.target.closest && e.target.closest(".glr"); if (row) glTip(row); else glTipHide(); });
    glrows.addEventListener("mouseleave", glTipHide);
    glscroll.addEventListener("scroll", glTipHide);
    glrows.addEventListener("click", function (e) {
      if (e.target.closest(".glmore-b")) { glLoad(true); return; }
      var row = e.target.closest(".glr");
      if (row) { glTipHide(); gwOpen(gl.commits[+row.getAttribute("data-i")].oid); }
    });
    glrows.addEventListener("keydown", function (e) {
      var row = e.target.closest && e.target.closest(".glr");
      if (row && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); gwOpen(gl.commits[+row.getAttribute("data-i")].oid); }
    });
    // A screen widened past the narrow layout shows the graph: read it then.
    window.addEventListener("resize", function () { if (!gl.loaded && drOpen && drMode === "git" && glog.offsetWidth) glLoad(false); });
  }

  // The commit window, built on first use and kept on the body (outside the
  // drawer's clip, which a fixed element inside it would not escape).
  var gw = { back: null, oid: "", seq: 0, lastFocus: null, down: false, list: [], pin: -1, io: null };
  function gwBuild() {
    if (gw.back) return;
    var back = mk("div", "gwin-back"), win = mk("div", "gwin"), top = mk("div", "gw-top"), head = mk("div", "gw-head");
    back.hidden = true;
    win.setAttribute("role", "dialog");
    win.setAttribute("aria-modal", "true");
    win.setAttribute("aria-labelledby", "gwsubj");
    win.tabIndex = -1;
    var h2 = mk("h2");
    h2.id = "gwsubj";
    var acts = mk("div", "gw-acts"), copy = mk("button", "idcopy"), close = mk("button", "ia");
    copy.type = close.type = "button";
    copy.title = wsT("js.git.win.copy.title");
    copy.appendChild(gIcon("copy"));
    copy.appendChild(mk("span", "", wsT("js.git.win.copy")));
    close.title = wsT("js.git.win.close");
    close.setAttribute("aria-label", close.title);
    close.appendChild(gIcon("x"));
    acts.appendChild(copy);
    acts.appendChild(close);
    head.appendChild(h2);
    head.appendChild(acts);
    var chips = mk("div", "gw-chips"), meta = mk("dl", "gw-meta"), msg = mk("pre", "gw-msg");
    [head, chips, meta, msg].forEach(function (e) { top.appendChild(e); });
    var body = mk("div", "gw-body"), diff = mk("div", "gw-diff"), files = mk("aside", "gw-files");
    files.setAttribute("aria-label", wsT("js.git.win.fileslabel"));
    body.appendChild(diff);
    body.appendChild(files);
    win.appendChild(top);
    win.appendChild(body);
    back.appendChild(win);
    doc.body.appendChild(back);
    gw.back = back; gw.win = win; gw.h2 = h2; gw.copy = copy; gw.chips = chips; gw.meta = meta; gw.msg = msg; gw.diff = diff; gw.files = files;
    close.addEventListener("click", gwClose);
    // Outside: a press and its release both on the backdrop, so a text
    // selection dragged out of the window keeps it open.
    back.addEventListener("mousedown", function (e) { gw.down = e.target === back; });
    back.addEventListener("click", function (e) { if (e.target === back && gw.down) gwClose(); gw.down = false; });
    copy.addEventListener("click", function () {
      var full = gw.oid, span = copy.querySelector("span");
      gwCopy(full, copy, function (ok) {
        if (!ok) return;
        copy.classList.add("done");
        copy.querySelector("use").setAttribute("href", "#i-check");
        span.textContent = wsT("js.git.win.copied");
        setTimeout(function () {
          copy.classList.remove("done");
          copy.querySelector("use").setAttribute("href", "#i-copy");
          span.textContent = wsT("js.git.win.copy");
        }, 1400);
      });
    });
    meta.addEventListener("click", function (e) { var p = e.target.closest(".gw-par"); if (p) gwOpen(p.getAttribute("data-oid")); });
    files.addEventListener("click", function (e) {
      var r = e.target.closest(".gwf");
      if (r) gwGo(+r.getAttribute("data-i"));
    });
    diff.addEventListener("click", function (e) {
      var b = e.target.closest(".load-b"), f = e.target.closest(".fold-b");
      if (b) gwLoad(b.closest(".dfile"));
      if (f) f.closest(".dfile").classList.toggle("fold");
    });
    diff.addEventListener("scroll", gwSpy, { passive: true });
    // A clicked file stays pinned in view while the sections above it load
    // and grow, until the user scrolls the viewer.
    ["wheel", "touchstart", "keydown", "mousedown"].forEach(function (ev) {
      diff.addEventListener(ev, function () { gw.pin = -1; }, { passive: true });
    });
  }
  function gwCopy(text, near, done) {
    function legacy() {
      var ta = doc.createElement("textarea");
      ta.value = text; ta.setAttribute("readonly", ""); ta.style.position = "fixed"; ta.style.opacity = "0";
      near.appendChild(ta); ta.select();
      var ok = false;
      try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
      ta.remove(); near.focus();
      done(ok);
    }
    if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(function () { done(true); }, legacy);
    else legacy();
  }
  function gwSel() { q(".glr", glrows || doc).forEach(function (r) { var c = gl.commits[+r.getAttribute("data-i")]; r.classList.toggle("sel", !!c && c.oid === gw.oid && !gw.back.hidden); }); }
  function gwClose() {
    if (!gw.back || gw.back.hidden) return;
    gw.back.hidden = true;
    gw.seq++;
    if (gw.io) gw.io.disconnect();
    gwSel();
    gw.oid = "";
    if (gw.lastFocus && gw.lastFocus.focus && doc.contains(gw.lastFocus)) gw.lastFocus.focus();
  }
  // Escape closes the window before anything else hears it (the drawer
  // would close on it too).
  window.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && gw.back && !gw.back.hidden) { e.preventDefault(); e.stopPropagation(); gwClose(); }
  }, true);
  function gwNote(text, err) { return mk("div", "gempty" + (err ? " err" : ""), text); }
  function gwOpen(oid) {
    if (!oid || !window.fetch) return;
    gwBuild();
    var seq = ++gw.seq;
    gw.oid = oid;
    if (gw.back.hidden) { gw.lastFocus = doc.activeElement; gw.back.hidden = false; }
    gwSel();
    gw.win.focus();
    gw.h2.textContent = oid.slice(0, 12);
    gw.copy.hidden = true;
    gw.chips.textContent = gw.meta.textContent = gw.files.textContent = gw.diff.textContent = "";
    gw.msg.hidden = true;
    gw.files.appendChild(gwNote(wsT("js.git.win.loading")));
    fetch("/s/" + rid + "/git/commit/" + encodeURIComponent(oid), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gw.seq) return;
        var c = null;
        if (r.ok) { try { c = JSON.parse(s); } catch (e) { s = "malformed answer"; } }
        if (!c) { gw.files.textContent = ""; gw.files.appendChild(gwNote(wsT("js.git.win.fail", s.trim() || "HTTP " + r.status), true)); return; }
        gwFill(c);
      });
    }, function () { if (seq === gw.seq) { gw.files.textContent = ""; gw.files.appendChild(gwNote(wsT("js.post.unreachable"), true)); } });
  }
  function gwPerson(dd, name, email, ms) {
    dd.appendChild(doc.createTextNode(name + " "));
    dd.appendChild(mk("span", "em", "<" + email + ">"));
    dd.appendChild(doc.createTextNode(" · " + glRel(ms) + " "));
    dd.appendChild(mk("span", "abs", "(" + glWhen(ms) + ")"));
  }
  function gwPath(p) {
    var el = mk("span", "gp"), slash = p.lastIndexOf("/");
    if (slash >= 0) el.appendChild(mk("span", "gdir", p.slice(0, slash + 1)));
    el.appendChild(mk("span", "gname", p.slice(slash + 1)));
    return el;
  }
  function gwDelta(add, del) {
    var d = mk("span", "gd");
    if (add < 0) d.appendChild(mk("i", "bin", "bin"));
    else {
      if (add) d.appendChild(mk("i", "add", "+" + add));
      if (del) d.appendChild(mk("i", "del", "−" + del));
    }
    return d;
  }
  // gwFill draws a commit read (GitCommit) into the window.
  function gwFill(c) {
    gw.oid = c.oid;
    var lines = (c.message || "").split("\n"), body = lines.slice(1).join("\n").replace(/^\n+/, "");
    gw.h2.textContent = lines[0] || c.oid.slice(0, 12);
    gw.copy.hidden = false;
    gw.chips.textContent = "";
    var hash = mk("code", "hash", c.oid.slice(0, 12));
    hash.title = c.oid;
    gw.chips.appendChild(hash);
    if (gl.by[c.oid]) glPills(gl.by[c.oid]).forEach(function (p) { gw.chips.appendChild(p); });
    if (c.parents.length > 1) gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.merge")));
    else if (!c.parents.length) gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.root")));
    gw.meta.textContent = "";
    var dd;
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.author")));
    gw.meta.appendChild(dd = mk("dd"));
    gwPerson(dd, c.author, c.authorEmail, c.authorTime);
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.committer")));
    gw.meta.appendChild(dd = mk("dd"));
    if (c.committer === c.author && c.committerEmail === c.authorEmail && c.commitTime === c.authorTime) dd.appendChild(mk("span", "em", wsT("js.git.win.same")));
    else gwPerson(dd, c.committer, c.committerEmail, c.commitTime);
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.parents")));
    gw.meta.appendChild(dd = mk("dd"));
    if (!c.parents.length) dd.appendChild(mk("span", "em", wsT("js.git.win.noparents")));
    c.parents.forEach(function (p, i) {
      if (i) dd.appendChild(doc.createTextNode(" "));
      var b = mk("button", "gw-par", p.slice(0, 9));
      b.type = "button";
      b.setAttribute("data-oid", p);
      b.title = gl.by[p] ? gl.by[p].subject : p;
      dd.appendChild(b);
      if (i === 0 && c.parents.length > 1) { dd.appendChild(doc.createTextNode(" ")); dd.appendChild(mk("span", "em", wsT("js.git.win.base"))); }
    });
    gw.msg.textContent = body;
    gw.msg.hidden = !body;
    var base = "/s/" + rid + "/git/commit/" + c.oid + "/diff?path=";
    gwBody([{ files: c.files.map(function (f) { return { path: f.path, status: f.status, from: f.from, add: f.add, del: f.del, url: base + encodeURIComponent(f.path) }; }) }], -1,
      c.truncated ? wsT("js.git.win.truncated", c.files.length) : "");
  }
  // gwWorktree opens the window on the working tree's changed files (the
  // git sheet's last status read), the file at index i of side in view.
  function gwWorktree(side, i) {
    var v = gitLast;
    if (!v || !window.fetch) return;
    gwBuild();
    var seq = ++gw.seq;
    gw.oid = "";
    if (gw.back.hidden) { gw.lastFocus = doc.activeElement; gw.back.hidden = false; }
    gwSel();
    gw.win.focus();
    gw.h2.textContent = wsT("js.git.win.worktree");
    gw.copy.hidden = true;
    gw.chips.textContent = gw.meta.textContent = "";
    gw.chips.appendChild(mk("span", "gw-note", wsT("js.git.win.wtnote")));
    gw.meta.appendChild(mk("dt", "", wsT("js.git.win.branch")));
    gw.meta.appendChild(mk("dd", "", v.detached ? wsT("js.git.detached", v.branch) : v.branch + (v.upstream ? " \u2192 " + v.upstream : "")));
    gw.msg.hidden = true;
    var focus = -1, n = 0, groups = [];
    [["staged", "js.git.staged"], ["unstaged", "js.git.unstaged"], ["untracked", "js.git.untracked"]].forEach(function (g) {
      var list = v[g[0]] || [];
      if (g[0] === side) focus = n + i;
      n += list.length;
      if (!list.length) return;
      var base = "/s/" + rid + "/git/worktree/diff?side=" + g[0] + "&path=";
      groups.push({ label: wsT(g[1]), files: list.map(function (f) { return { path: f.path, status: f.status, add: f.add, del: f.del, untracked: g[0] === "untracked", url: base + encodeURIComponent(f.path) }; }) });
    });
    if (seq === gw.seq) gwBody(groups, focus, v.truncated ? wsT("js.git.truncated", 300) : "");
  }

  // --- the commit window's diff viewer ---------------------------------------
  // One section per changed file (gitdiff.go serves each file's diff). A
  // section reads its diff when it nears the viewport; a file with more than
  // GW_MANUAL changed lines, a binary file, or a deleted file waits behind
  // "Load diff". Clicking a file in the list scrolls to its section and loads
  // it, and keeps it pinned in view until the user scrolls (gwState).
  var GW_MANUAL = 400;
  function gwStatusClass(st) { return "gst st-" + (st === "?" ? "q" : st); }
  function gwManual(f) {
    if (f.add < 0) return wsT("js.git.diff.binarywait");
    if (f.status === "D") return wsTn("js.git.diff.deleted", f.del);
    if (f.add + f.del > GW_MANUAL) return wsT("js.git.diff.large", f.add + f.del);
    return "";
  }
  // gwBody fills the file list (groups of {label, files}) and the viewer's
  // sections, and brings file focus (an index over every group) into view.
  function gwBody(groups, focus, note) {
    var seq = gw.seq, add = 0, del = 0, n = 0;
    gw.list = [];
    gw.pin = -1;
    if (gw.io) gw.io.disconnect();
    gw.files.textContent = gw.diff.textContent = "";
    groups.forEach(function (g) { g.files.forEach(function (f) { gw.list.push(f); if (f.add > 0) add += f.add; if (f.del > 0) del += f.del; }); });
    var sum = mk("div", "gw-fsum");
    sum.appendChild(mk("span", "", wsTn("js.git.win.files", gw.list.length)));
    sum.appendChild(gwDelta(add, del));
    gw.files.appendChild(sum);
    groups.forEach(function (g) {
      if (g.label) {
        var head = mk("div", "ggrp");
        head.appendChild(mk("span", "", g.label));
        head.appendChild(mk("span", "chip", String(g.files.length)));
        gw.files.appendChild(head);
      }
      g.files.forEach(function (f) {
        var row = mk("div", "gf gwf");
        row.setAttribute("data-i", String(n++));
        row.title = f.from ? f.from + " \u2192 " + f.path : f.path;
        row.appendChild(mk("b", gwStatusClass(f.status), f.status));
        row.appendChild(gwPath(f.path));
        row.appendChild(f.untracked ? mk("span", "gd") : gwDelta(f.add, f.del));
        if (f.from) row.appendChild(mk("span", "ren", "\u2190 " + f.from.slice(f.from.lastIndexOf("/") + 1)));
        gw.files.appendChild(row);
      });
    });
    if (note) gw.files.appendChild(gwNote(note));
    if (!gw.list.length) gw.diff.appendChild(gwNote(wsT("js.git.win.nofiles")));
    gw.list.forEach(function (f, i) { gw.diff.appendChild(gwSection(f, i, seq)); });
    gw.diff.scrollTop = 0;
    gw.io = "IntersectionObserver" in window ? new IntersectionObserver(function (es) {
      es.forEach(function (e) { if (e.isIntersecting && e.target.getAttribute("data-state") === "auto") gwLoad(e.target); });
    }, { root: gw.diff, rootMargin: "200px 0px" }) : null;
    q(".dfile", gw.diff).forEach(function (sec) {
      if (gw.io) gw.io.observe(sec);
      else if (sec.getAttribute("data-state") === "auto") gwLoad(sec);
    });
    if (focus >= 0) gwGo(focus);
    else gwSpy();
  }
  function gwSection(f, i, seq) {
    var sec = mk("section", "dfile"), h = mk("header", "dfh"), fold = mk("button", "ia fold-b");
    sec.setAttribute("data-i", String(i));
    sec.gwSeq = seq;
    fold.type = "button";
    fold.title = wsT("js.git.win.fold");
    fold.setAttribute("aria-label", fold.title);
    fold.appendChild(gIcon("chev"));
    h.appendChild(fold);
    h.appendChild(mk("b", gwStatusClass(f.status), f.status));
    h.appendChild(gwPath(f.path));
    if (f.from) h.appendChild(mk("span", "dfren", "\u2190 " + f.from));
    h.appendChild(f.untracked ? mk("span", "gd") : gwDelta(f.add, f.del));
    sec.appendChild(h);
    sec.appendChild(mk("div", "dfb"));
    gwState(sec, gwManual(f) ? "manual" : "auto");
    return sec;
  }
  function gwSkeleton(widths) {
    var sk = mk("div", "dskel");
    widths.forEach(function (w) { var i = mk("i"); i.style.width = w + "%"; sk.appendChild(i); });
    return sk;
  }
  // gwState draws a section's body for its state: auto and loading (a
  // skeleton), manual (why, and "Load diff"), fail, or ready (the diff).
  function gwState(sec, state, data) {
    var f = gw.list[+sec.getAttribute("data-i")], b = sec.querySelector(".dfb");
    sec.setAttribute("data-state", state);
    b.textContent = "";
    if (state === "auto") b.appendChild(gwSkeleton([46, 72, 58]));
    else if (state === "loading") b.appendChild(gwSkeleton([60, 38, 66, 50]));
    else if (state === "manual" || state === "fail") {
      var note = mk("div", "dnote" + (state === "fail" ? " err" : ""));
      note.appendChild(mk("span", "", state === "fail" ? wsT("js.git.diff.fail", data) : gwManual(f)));
      var lb = mk("button", "btn load-b", wsT("js.git.win.loaddiff"));
      lb.type = "button";
      note.appendChild(lb);
      b.appendChild(note);
    } else gwRender(b, data);
    // Sections above the pinned file grow as they load: keep it in place.
    var pin = gw.pin >= 0 && q('.dfile[data-i="' + gw.pin + '"]', gw.diff)[0];
    if (pin) gw.diff.scrollTop = pin.offsetTop - 10;
  }
  function gwLoad(sec) {
    var st = sec.getAttribute("data-state"), seq = sec.gwSeq, f = gw.list[+sec.getAttribute("data-i")];
    if (st === "ready" || st === "loading" || seq !== gw.seq || !f) return;
    gwState(sec, "loading");
    fetch(f.url, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.text().then(function (s) {
        if (seq !== gw.seq) return;
        var d = null;
        if (r.ok) { try { d = JSON.parse(s); } catch (e) { s = "malformed answer"; } }
        if (d) gwState(sec, "ready", d);
        else gwState(sec, "fail", s.trim() || "HTTP " + r.status);
      });
    }, function () { if (seq === gw.seq) gwState(sec, "fail", wsT("js.post.unreachable")); });
  }
  function gwDnote(text, cls) { return mk("div", "dnote" + (cls ? " " + cls : ""), text); }
  // gwRender draws one file's diff (GitFileDiff): numbered unified lines
  // with their highlight classes and intra-line marks, or a notice.
  function gwRender(b, d) {
    var notices = { dir: wsT("js.git.diff.notice.dir"), gitlink: wsT("js.git.diff.notice.gitlink"), unmerged: wsT("js.git.diff.notice.unmerged"), special: wsT("js.git.diff.notice.special"), outside: wsT("js.git.diff.notice.outside") };
    if (d.notice) b.appendChild(gwDnote(notices[d.notice] || d.notice));
    else if (d.binary) b.appendChild(gwDnote(wsT("js.git.diff.binary")));
    else if (!d.hunks || !d.hunks.length) b.appendChild(gwDnote(wsT("js.git.diff.nochange")));
    else {
      var tab = mk("div", "dtab chroma"), frag = doc.createDocumentFragment();
      d.hunks.forEach(function (h) {
        var o = h.old, n = h.new;
        frag.appendChild(gwRow("hk", "", "", "", [{ s: h.header }]));
        h.lines.forEach(function (l) {
          if (l.k === "+") frag.appendChild(gwRow("add", "", n++, "+", l.t));
          else if (l.k === "-") frag.appendChild(gwRow("del", o++, "", "\u2212", l.t));
          else if (l.k === "\\") frag.appendChild(gwRow("nl", "", "", "", l.t));
          else frag.appendChild(gwRow("", o++, n++, "", l.t));
        });
      });
      tab.appendChild(frag);
      b.appendChild(tab);
    }
    if (d.truncated) b.appendChild(gwDnote(wsT("js.git.diff.cut"), "cut"));
  }
  function gwRow(cls, o, n, sign, pieces) {
    var row = mk("div", "dl" + (cls ? " " + cls : "")), cd = mk("span", "cd");
    row.appendChild(mk("span", "no", String(o)));
    row.appendChild(mk("span", "no", String(n)));
    row.appendChild(mk("span", "sg", sign));
    (pieces || []).forEach(function (p) {
      if (!p.c && !p.x) { cd.appendChild(doc.createTextNode(p.s)); return; }
      cd.appendChild(mk("span", (p.c ? "t-" + p.c : "") + (p.x ? (p.c ? " dx" : "dx") : ""), p.s));
    });
    row.appendChild(cd);
    return row;
  }
  // gwGo scrolls to file i's section, loads it, and pins it.
  function gwGo(i) {
    var sec = q('.dfile[data-i="' + i + '"]', gw.diff)[0];
    if (!sec) return;
    gw.pin = i;
    sec.classList.remove("fold");
    gwLoad(sec);
    gw.diff.scrollTop = sec.offsetTop - 10;
    gwMark(i);
  }
  function gwMark(i) { q(".gwf", gw.files).forEach(function (r) { r.classList.toggle("on", +r.getAttribute("data-i") === i); }); }
  // gwSpy marks the file whose section is at the top of the viewer.
  function gwSpy() {
    if (gw.pin >= 0) { gwMark(gw.pin); return; }
    var top = gw.diff.scrollTop + 40, on = 0;
    q(".dfile", gw.diff).forEach(function (sec) { if (sec.offsetTop <= top) on = +sec.getAttribute("data-i"); });
    gwMark(gw.list.length ? on : -1);
  }

