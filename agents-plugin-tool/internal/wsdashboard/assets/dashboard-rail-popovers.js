  // --- rail popovers: add workspace, add worktree, forget ----------------------
  // On a daemon that keeps stored workspaces the rail header's + adds a
  // workspace and a repository bar's + adds a worktree, each in a popover;
  // a stored workspace bar's forget takes it off the stored list at once
  // (never its files). The rail region is replaced on every update, so the
  // popovers live outside it: made once, placed fixed against the + that
  // opened it, and kept open across a swap, re-anchored to the new + by
  // railMarks. One popover at a time; Esc, an outside click, and cancel close
  // it. A route's error text is already in the page's language; the rail
  // shows what an add or a forget changed through the stream.
  // railPost posts body to a rail route; done(status, answer, text) gets the
  // HTTP status (0: the daemon was not reached), the parsed JSON answer of a
  // success ({} when there is none), and the body text of a failure.
  // timeout (ms, optional) ends a request that never answers: done runs
  // once with status 0 and the timeout text, and the fetch is aborted, so a
  // caller's busy state can never stay on. The server may still have acted.
  function railPost(route, body, done, timeout) {
    if (!window.fetch) { done(0, {}, wsT("js.post.nobrowser")); return; }
    var ended = false, ctl = timeout && window.AbortController ? new window.AbortController() : null, timer = 0;
    function end(st, v, s) {
      if (ended) return;
      ended = true;
      if (timer) clearTimeout(timer);
      done(st, v, s);
    }
    if (timeout) timer = setTimeout(function () { if (ctl) ctl.abort(); end(0, {}, wsT("js.post.timeout")); }, timeout);
    fetch(route, {
      method: "POST", credentials: "same-origin", cache: "no-store", signal: ctl ? ctl.signal : undefined,
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        var v = {};
        if (r.ok) { try { v = s ? JSON.parse(s) : {}; } catch (e) { v = {}; } }
        end(r.status, v && typeof v === "object" ? v : {}, r.ok ? "" : s.trim());
      });
    }).then(null, function () { end(0, {}, ""); });
  }
  // railErr is a failed rail route's line: its own text, except a 404, which
  // names a workspace the rail no longer shows.
  function railErr(status, text) {
    if (status === 404) return wsT("js.rail.unknown");
    return text || (status ? "HTTP " + status : wsT("js.post.unreachable"));
  }
  function rpId() { return "rpop-" + (++rpIds); }
  function rpPop(title) {
    var el = mk("div", "rpop");
    el.hidden = true;
    el.setAttribute("role", "dialog");
    el.setAttribute("aria-label", title);
    return el;
  }
  function rpBtn(cls, text) { var b = mk("button", cls, text); b.type = "button"; return b; }
  function rpInput(cls, label) {
    var i = mk("input", cls);
    i.type = "text";
    i.spellcheck = false;
    i.setAttribute("autocomplete", "off");
    if (label) i.setAttribute("aria-label", label);
    return i;
  }
  // rpLive reports that popover P is still open as it was when gen was
  // taken: an answer that comes later closes nothing, and shows as a toast.
  function rpLive(P, gen) { return rp.P === P && P.gen === gen; }
  function rpEnter(el, fn) { el.addEventListener("keydown", function (e) { if (e.key === "Enter" && !e.isComposing) { e.preventDefault(); fn(); } }); }

  // The add-workspace popover: a pasted absolute path, read by the daemon's
  // preview while the user types (debounced; a stale answer is dropped).
  // "Add" is off while the input is empty or the latest preview refuses it;
  // a preview still on its way leaves it on, as the add reads the path again.
  function rpWsBrowse(P, inp, preview, from) {
    if (P.picker || P.busy || !root.hasAttribute("data-sysopen") || !window.WsFiles) return;
    var D = P.picker = { instance: null }, gen = P.gen;
    var dl = D.el = mk("dialog", "fworkspace-picker");
    dl.setAttribute("aria-label", wsT("js.rail.ws.browse"));
    var pane = dl.appendChild(mk("section", "fpane"));
    var note = pane.appendChild(mk("div", "fnote", wsT("js.files.loading")));
    var row = pane.appendChild(mk("div", "fattachment"));
    row.appendChild(rpBtn("btn", wsT("js.rail.cancel"))).addEventListener("click", function () { D.close(true); });
    D.close = function (focus) {
      if (P.picker !== D) return;
      P.picker = null;
      if (D.instance) D.instance.destroy();
      dl.close(); dl.remove();
      if (focus) from.focus();
    };
    dl.addEventListener("cancel", function (e) { e.preventDefault(); D.close(true); });
    P.el.appendChild(dl);
    dl.showModal();
    fetch("/s/host0/fs/roots", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("roots refused");
      return r.json();
    }).then(function (roots) {
      if (P.picker !== D || P.gen !== gen) return;
      var start = roots.home || (roots.volumes || [])[0];
      if (!start) throw new Error("no browse roots");
      D.instance = window.WsFiles.create(pane, {
        rid: "host0", routeBase: "/s/host0/", local: true, directoriesOnly: true,
        roots: roots, location: { root: start, dir: "" }, toast: toast,
        attachment: function (slot, api) {
          var select = slot.appendChild(rpBtn("btn pr", wsT("js.rail.ws.select")));
          select.disabled = true;
          var cancel = slot.appendChild(rpBtn("btn", wsT("js.rail.cancel")));
          function sync() { select.disabled = !api.selection(); }
          var offNav = api.on("navigation", sync), offSel = api.on("selection", sync);
          select.addEventListener("click", function () {
            var chosen = api.selection();
            if (!chosen || !rpLive(P, gen) || P.busy) return;
            inp.value = chosen.absolute;
            D.close(false); preview(); inp.focus();
          });
          cancel.addEventListener("click", function () { D.close(true); });
          return function () { offNav(); offSel(); };
        }
      });
      D.instance.shown();
    }).catch(function () { if (P.picker === D && P.gen === gen) note.textContent = wsT("js.files.failed"); });
  }

  function rpWsMake() {
    var P = { seq: 0, gen: 0, timer: 0, ok: null, busy: false };
    var el = P.el = rpPop(wsT("js.rail.ws.title"));
    el.appendChild(mk("div", "sphd", wsT("js.rail.ws.title")));
    var sec = el.appendChild(mk("div", "spsec"));
    var lbl = sec.appendChild(mk("label", "splbl", wsT("js.rail.ws.label")));
    var line = sec.appendChild(mk("div", "spline"));
    var inp = line.appendChild(rpInput("spq mono"));
    var browse = root.hasAttribute("data-sysopen") && window.WsFiles ? line.appendChild(rpBtn("pbtn", wsT("js.rail.ws.browse"))) : null;
    if (browse) browse.addEventListener("click", function () { rpWsBrowse(P, inp, preview, browse); });
    var go = line.appendChild(rpBtn("pbtn go", wsT("js.rail.ws.add")));
    var res = sec.appendChild(mk("div", "spres"));
    inp.id = rpId();
    lbl.htmlFor = inp.id;
    inp.placeholder = wsT("js.rail.ws.placeholder");
    res.id = rpId();
    res.setAttribute("aria-live", "polite");
    inp.setAttribute("aria-describedby", res.id);
    function show(cls, parts) {
      res.className = "spres" + (cls ? " " + cls : "");
      res.textContent = "";
      parts.forEach(function (p) { if (p) res.appendChild(p); });
    }
    function hint() { show("", [mk("span", "spnote", wsT("js.rail.ws.hint"))]); }
    function bad(text) { show("bad", [mk("span", "k", text)]); }
    function sync() {
      go.disabled = P.busy || !inp.value.trim() || P.ok === false;
      if (browse) browse.disabled = P.busy;
    }
    function preview() {
      var text = inp.value, n = ++P.seq;
      clearTimeout(P.timer);
      P.ok = null;
      if (!text.trim()) { hint(); sync(); return; }
      sync();
      P.timer = setTimeout(function () {
        railPost("/rail/workspaces/preview", { path: text }, function (status, v, s) {
          if (n !== P.seq) return;
          if (status !== 200) bad(railErr(status, s));
          else if (!v.ok) { P.ok = false; bad(String(v.note || "")); }
          else {
            P.ok = true;
            show("", [mk("span", "k", wsT("js.rail.ws.will")), mk("code", null, String(v.root || v.path || "")), v.note ? mk("span", "spnote", String(v.note)) : null]);
          }
          sync();
        });
      }, 200);
    }
    function submit() {
      sync();
      if (go.disabled) return;
      var gen = P.gen;
      P.busy = true;
      P.seq++;
      clearTimeout(P.timer);
      sync();
      railPost("/rail/workspaces/add", { path: inp.value }, function (status, v, s) {
        var live = rpLive(P, gen);
        if (status === 200) {
          if (live) railPopClose(false);
          toast(String(v.message || ""));
          return;
        }
        if (!live) { toast(railErr(status, s)); return; }
        P.busy = false;
        if (status === 409) P.ok = false;
        bad(railErr(status, s));
        sync();
      });
    }
    inp.addEventListener("input", preview);
    rpEnter(inp, submit);
    go.addEventListener("click", submit);
    P.open = function () { P.gen++; inp.value = ""; P.ok = null; P.busy = false; hint(); sync(); };
    P.focus = function () { inp.focus(); };
    P.back = function () { if (!P.picker) return false; P.picker.close(true); return true; };
    P.close = function () { P.seq++; clearTimeout(P.timer); if (P.picker) P.picker.close(false); };
    return P;
  }

  // The add-worktree popover for one repository workspace: a new branch
  // (from a base, the root's current branch preselected), an existing one
  // no worktree has checked out, or a detached HEAD from that base, and the folder (the daemon's default, or a
  // custom absolute path). The options load when it opens; a failure, and a
  // refused or failed add, show in its error line and keep it open.
  function rpWtMake() {
    var P = { seq: 0, gen: 0, opts: null, mode: "new", pick: "", custom: false, busy: false, ws: "" };
    var el = P.el = rpPop(wsT("js.rail.wt.title"));
    var hd = el.appendChild(mk("div", "sphd", wsT("js.rail.wt.title")));
    var ctx = hd.appendChild(mk("span", "spctx"));
    var bsec = el.appendChild(mk("div", "spsec"));
    bsec.appendChild(mk("div", "splbl", wsT("js.rail.wt.branch")));
    var seg = bsec.appendChild(mk("div", "spseg"));
    seg.setAttribute("role", "radiogroup");
    seg.setAttribute("aria-label", wsT("js.rail.wt.branch"));
    var modes = { "new": rpBtn("", wsT("js.rail.wt.new")), old: rpBtn("", wsT("js.rail.wt.old")), detached: rpBtn("", wsT("js.rail.wt.detached")) };
    Object.keys(modes).forEach(function (k) { modes[k].setAttribute("role", "radio"); modes[k].setAttribute("data-mode", k); seg.appendChild(modes[k]); });
    var lnew = bsec.appendChild(mk("div", "spline"));
    var name = lnew.appendChild(rpInput("spq mono", wsT("js.rail.wt.name")));
    lnew.appendChild(mk("span", "spfrom", wsT("js.rail.wt.from")));
    var base = lnew.appendChild(mk("select", "spsel"));
    base.setAttribute("aria-label", wsT("js.rail.wt.base"));
    var lold = bsec.appendChild(mk("div", "spsec"));
    var find = lold.appendChild(rpInput("spq mono", wsT("js.rail.wt.old")));
    find.type = "search";
    find.placeholder = wsT("js.rail.wt.search");
    var list = lold.appendChild(mk("ul", "splist"));
    list.id = rpId();
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-label", wsT("js.rail.wt.old"));
    find.setAttribute("aria-controls", list.id);
    var fsec = el.appendChild(mk("div", "spsec"));
    var flbl = fsec.appendChild(mk("div", "splbl", wsT("js.rail.wt.folder")));
    var tog = flbl.appendChild(rpBtn("spreset", wsT("js.rail.wt.custom")));
    var def = fsec.appendChild(mk("div", "spdir"));
    var defPath = def.appendChild(mk("code"));
    def.appendChild(mk("span", "spnote", wsT("js.rail.wt.default")));
    var own = fsec.appendChild(mk("div", "spown"));
    var folder = own.appendChild(rpInput("spq mono", wsT("js.rail.wt.folderpath")));
    own.appendChild(mk("span", "spnote", wsT("js.rail.wt.customnote")));
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var row = el.appendChild(mk("div", "sprow"));
    var status = row.appendChild(mk("span", "spnote"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn go", wsT("js.rail.wt.create")));
    function setErr(text) { err.textContent = text || ""; err.hidden = !text; }
    function sync() {
      Object.keys(modes).forEach(function (k) { modes[k].setAttribute("aria-checked", String(P.mode === k)); });
      lnew.hidden = P.mode === "old";
      name.hidden = P.mode === "detached";
      lold.hidden = P.mode !== "old";
      def.hidden = P.custom;
      own.hidden = !P.custom;
      tog.textContent = wsT(P.custom ? "js.rail.wt.usedefault" : "js.rail.wt.custom");
      find.disabled = !P.opts || !P.opts.free.length;
      go.disabled = P.busy || !P.opts || (P.mode === "new" ? !name.value.trim() : P.mode === "old" && !P.pick);
    }
    // drawList lists the free branches matching the search, keeping the
    // pick when it still matches, else picking the first match.
    function drawList() {
      var free = P.opts ? P.opts.free : [], hits = WsRail.matchBranches(free, find.value);
      if (hits.indexOf(P.pick) < 0) P.pick = hits[0] || "";
      list.textContent = "";
      find.removeAttribute("aria-activedescendant");
      if (P.opts && !hits.length) {
        var none = list.appendChild(mk("li", "spnone", wsT(free.length ? "js.rail.wt.nomatch" : "js.rail.wt.nofree")));
        none.setAttribute("aria-disabled", "true");
      }
      hits.forEach(function (b, i) {
        var li = list.appendChild(mk("li", null, b));
        li.id = list.id + "-" + i;
        li.setAttribute("role", "option");
        li.setAttribute("aria-selected", String(b === P.pick));
        li.setAttribute("data-branch", b);
        if (b === P.pick) find.setAttribute("aria-activedescendant", li.id);
      });
      sync();
    }
    function step(d) {
      var hits = WsRail.matchBranches(P.opts ? P.opts.free : [], find.value), i = hits.indexOf(P.pick) + d;
      if (!hits.length) return;
      P.pick = hits[Math.max(0, Math.min(hits.length - 1, i))];
      drawList();
      var cur = list.querySelector('[aria-selected="true"]');
      if (cur && cur.scrollIntoView) cur.scrollIntoView({ block: "nearest" });
    }
    // edited drops an earlier add's error once the form changed.
    function edited() { if (!P.busy) setErr(""); sync(); }
    function setMode(m) {
      if (P.mode === m) return;
      P.mode = m;
      edited();
      (m === "new" ? name : m === "old" ? find : base).focus();
    }
    function submit() {
      sync();
      if (go.disabled) return;
      var gen = P.gen;
      P.busy = true;
      setErr("");
      sync();
      var body = WsRail.worktreeBody(P.ws, { mode: P.mode, name: name.value, base: base.value, pick: P.pick, custom: P.custom, folder: folder.value, shown: P.opts.folder });
      railPost("/rail/worktrees/add", body, function (st, v, s) {
        var live = rpLive(P, gen);
        if (st === 200) {
          if (live) railPopClose(false);
          toast(String(v.message || ""));
          return;
        }
        if (!live) { toast(railErr(st, s)); return; }
        P.busy = false;
        setErr(railErr(st, s));
        sync();
      });
    }
    seg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-mode]"); if (b && !b.disabled) setMode(b.getAttribute("data-mode")); });
    name.addEventListener("input", edited);
    rpEnter(name, submit);
    find.addEventListener("input", function () { drawList(); edited(); });
    find.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); step(e.key === "ArrowDown" ? 1 : -1); }
    });
    rpEnter(find, submit);
    list.addEventListener("click", function (e) {
      var li = e.target.closest && e.target.closest("li[data-branch]");
      if (!li) return;
      P.pick = li.getAttribute("data-branch");
      drawList();
      edited();
      find.focus();
    });
    tog.addEventListener("click", function () {
      P.custom = !P.custom;
      if (P.custom && !folder.value && P.opts) folder.value = P.opts.folder;
      edited();
      (P.custom ? folder : tog).focus();
    });
    folder.addEventListener("input", edited);
    rpEnter(folder, submit);
    cancel.addEventListener("click", function () { railPopClose(true); });
    go.addEventListener("click", submit);
    P.open = function (ws, label) {
      var n = ++P.seq;
      P.gen++;
      P.ws = ws; P.opts = null; P.mode = "new"; P.pick = ""; P.custom = false; P.busy = false;
      ctx.textContent = label || ws;
      ctx.title = ws;
      name.value = ""; find.value = ""; folder.value = "";
      base.textContent = "";
      defPath.textContent = "";
      status.textContent = wsT("js.rail.wt.loading");
      setErr("");
      drawList();
      railPost("/rail/worktrees/options", { workspace: ws }, function (st, v, s) {
        if (n !== P.seq) return;
        status.textContent = "";
        if (st !== 200) { setErr(railErr(st, s)); sync(); return; }
        var arr = function (x) { return Array.isArray(x) ? x.filter(function (b) { return typeof b === "string"; }) : []; };
        P.opts = { current: String(v.current || ""), bases: arr(v.bases), free: arr(v.free), folder: String(v.folder || "") };
        // A detached root has no current branch: the base then defaults to
        // its HEAD, sent as an empty base.
        if (!P.opts.current) base.appendChild(mk("option", null, wsT("js.rail.wt.head"))).value = "";
        P.opts.bases.forEach(function (b) { base.appendChild(mk("option", null, b)).value = b; });
        base.value = P.opts.current;
        defPath.textContent = P.opts.folder;
        drawList();
        railPopPlace();
      });
    };
    P.focus = function () { name.focus(); };
    P.close = function () { P.seq++; };
    return P;
  }

  // rpKey is what a popover of kind opened from btn concerns: the folder of
  // a new session, the viewed session, else the button's workspace.
  function rpKey(kind, btn) {
    if (kind === "sess") return rid;
    if (kind === "lead") return btn.getAttribute("data-root") || "";
    var ws = btn.closest(".rws");
    return ws ? ws.getAttribute("data-ws") || "" : "";
  }
  // railPopBtn is the button the open popover belongs to: the one that
  // opened it, or after a swap the button of the same kind and concern
  // (null when the page shows none any more).
  function railPopBtn() {
    if (!rp.P) return null;
    if (rp.btn && rp.btn.isConnected) return rp.btn;
    var r = region("rail"), h = region("head"), found = null;
    if (rp.kind === "settings") found = r && r.querySelector(".settings-toggle");
    else if (rp.kind === "usage") found = topEl && topEl.querySelector(".usage-toggle");
    else if (rp.kind === "sess") found = h && h.querySelector(".host.hbtn");
    else if (rp.kind === "row") {
      var row = r && WsRail.refind(r, { cls: "rs", attr: "data-rid", val: rp.ws });
      found = row && row.querySelector(".rl");
    } else if (r) q(rp.kind === "lead" ? ".rnew" : '.radd[data-add="' + rp.kind + '"]', r).forEach(function (b) {
      if (rpKey(rp.kind, b) === rp.ws) found = b;
    });
    rp.btn = found;
    return found;
  }
  // A row menu stands at the place in its row it opened at (the pointer),
  // following the row, else under the whole row.
  function railPopPlace() {
    var b = railPopBtn();
    if (!b) return;
    var el = rp.P.el, a = b.getBoundingClientRect();
    if (rp.kind === "row") {
      var rr = b.closest(".rs").getBoundingClientRect(), o = rp.at;
      a = o ? { top: rr.top + o.dy, bottom: rr.top + o.dy, left: rr.left + o.dx, right: rr.left + o.dx + el.offsetWidth } : rr;
    } else if (rp.kind === "usage") {
      // plan usage hangs centered under the header's center handle
      var c = (a.left + a.right) / 2;
      a = { top: a.top, bottom: a.bottom, left: c - el.offsetWidth / 2, right: c + el.offsetWidth / 2 };
    }
    var at = WsRail.popPlace(a, el.offsetWidth, el.offsetHeight, root.clientWidth || window.innerWidth, root.clientHeight || window.innerHeight);
    el.style.top = at.top + "px";
    el.style.left = at.left + "px";
  }
  // railPopSync runs after a rail swap: the new + shows its popover open, and
  // a popover whose + is gone (its workspace left the rail) closes, as does
  // a row menu whose row is gone or hidden.
  function railPopSync() {
    if (!rp.P) return;
    var b = railPopBtn();
    if (!b || (rp.kind === "row" && !b.getClientRects().length)) { railPopClose(false); return; }
    if (rp.kind !== "row") b.setAttribute("aria-expanded", "true");
    railPopPlace();
  }
  function railPopClose(refocus) {
    if (!rp.P) return;
    var P = rp.P, b = railPopBtn(), row = rp.kind === "row";
    P.close();
    P.el.hidden = true;
    rp = { P: null, kind: "", ws: "", btn: null };
    if (b) {
      if (!row) b.setAttribute("aria-expanded", "false");
      if (refocus) b.focus();
    }
  }
  // railPopOpen opens kind's popover ("ws", "wt", "lead": a new session, or
  // "sess": the viewed session's menu) against the button btn; the open
  // popover's own button closes it instead.
  function railPopOpen(kind, btn) {
    var bar = btn.closest(".rws"), ws = rpKey(kind, btn);
    if (rp.P && rp.kind === kind && rp.ws === ws) { railPopClose(true); return; }
    railPopClose(false);
    var P = kind === "settings" ? {
      el: gmenu,
      open: function () { setLayout(layoutMode); setInput(inputMode); markTheme(); markLang(); },
      close: function () {},
      focus: function () { gmenu.focus(); }
    } : kind === "usage" ? rpUsage || (rpUsage = usageGutter.makePopover(railPopPlace, function () { railPopClose(true); })) : kind === "wt" ? rpWt || (rpWt = rpWtMake()) : kind === "lead" ? rpLead || (rpLead = rpLeadMake()) : kind === "sess" ? rpSess || (rpSess = rpSessMake()) : rpWs || (rpWs = rpWsMake());
    if (!P.el.parentNode) doc.body.appendChild(P.el);
    rp = { P: P, kind: kind, ws: ws, btn: btn };
    btn.setAttribute("aria-expanded", "true");
    var nm = bar && bar.querySelector(".nm");
    P.open(ws, nm ? nm.textContent : ws, btn);
    P.el.hidden = false;
    railPopPlace();
    P.focus();
  }
  // railRevive brings an ended row's session back by its entry's move (take
  // over a session a terminal ran, reopen a closed hosted one) and opens its
  // page. One post at a time: revBusy is the session posting, kept across
  // the rail's updates (railMarks marks its entry), and kept after a success
  // until the page goes. A blocked entry (aria-disabled) posts nothing; a
  // refusal shows the route's own text.
  var revBusy = "";
  function railRevive(btn) {
    var row = btn.closest(".rs"), id = row ? row.getAttribute("data-rid") || "" : "";
    var route = WsRail.reviveRoute(btn.getAttribute("data-revive") || "", id);
    if (!route || revBusy || btn.getAttribute("aria-disabled") === "true") return;
    revBusy = id;
    railMarks();
    railPost(route, {}, function (status, v, s) {
      if (status === 200) {
        saveDraft();
        if (id === rid) { if (shell) shReload(); else window.location.reload(); }
        else go("/s/" + encodeURIComponent(id));
        return;
      }
      revBusy = "";
      railMarks();
      toast(leadErr(status, s));
    });
  }
  // railArchive archives ("hide") or unarchives ("unhide") the row's hosted
  // lead through its agent route. The row moves only when the rail swap
  // shows the new state, so a refusal leaves it where it is and says why. A
  // 409 on archive is the runner still checking the session; the user may
  // try again, and nothing retries here.
  var archBusy = "";
  function railArchive(btn) {
    var row = btn.closest(".rs"), id = row ? row.getAttribute("data-rid") || "" : "", act = btn.getAttribute("data-act") || "";
    if (!id || archBusy || (act !== "hide" && act !== "unhide")) return;
    archBusy = id;
    railMarks();
    railPost("/agents/" + encodeURIComponent(id) + "/" + act, {}, function (status, v, s) {
      archBusy = "";
      railMarks();
      if (status !== 200) toast(archErr(act, status, s));
    });
  }
  // archErr is the message for a refused archive or unarchive. A 409 archive
  // is the runner's retryable "holder scan pending"; a 500 one is a holder
  // that refused to stop, after the runner rolled visibility back. Either
  // way the row stays where it was, and nothing retries.
  function archErr(act, status, text) {
    if (act === "hide" && status === 409) return wsT("js.rail.archive.busy");
    if (act === "hide" && status === 500) return wsT("js.rail.archive.failed");
    return leadErr(status, text);
  }
  // railForget takes the bar's stored workspace off the list.
  function railForget(btn) {
    var bar = btn.closest(".rws"), path = bar ? bar.getAttribute("data-ws") || "" : "", nm = bar && bar.querySelector(".nm");
    if (!path || btn.disabled) return;
    btn.disabled = true;
    railPost("/rail/workspaces/forget", { path: path }, function (status, v, s) {
      btn.disabled = false;
      toast(status >= 200 && status < 300 ? wsT("js.rail.forgot", nm ? nm.textContent : path) : railErr(status, s));
    });
  }

  // --- rail row menu --------------------------------------------------------------
  // Every session row has a menu: a right click on the row, a long touch on
  // it (RM_HOLD; a touch on its grip drags at once instead), or Shift+F10
  // or the ContextMenu key on it (the rail's keydown) opens it. It renames
  // and copies the address as the row's own buttons do, toggles favorites
  // on eligible rows, moves the row under
  // another session (the rows WsRail.underRows allows, a list that opens in
  // place; shown disabled, with the reason, when there is none), detaches a
  // child from its parent (where Alt+Left puts it), and revives an ended
  // row as its own button does; a hosted row adds the session menu of that
  // row's session (rpSessMake(true)), shown once its state says this viewer
  // holds the controls. It is a rail popover (kind "row", rp.ws the row's
  // reply-id) at the pointer, or under the row from the keyboard, kept on
  // the row across a rail swap; the arrows, Home, and End move among its
  // items, Right and Left open and close the list, and Escape closes the
  // list, then the menu, the focus back on the row's link. The items are
  // made at each open, from the row as it was then; each one acts on the
  // row as the rail shows it when picked.
  var RM_HOLD = 500, rmHold = null, rmHeld = false;
  function rpRowMake() {
    var P = { gen: 0, rid: "", sess: null };
    var el = P.el = mk("div", "rpop rmenu");
    el.hidden = true;
    var list = el.appendChild(mk("div", "rmlist")), under = null, sub = null;
    list.setAttribute("role", "menu");
    function rowNow() {
      var r = region("rail");
      return r ? WsRail.refind(r, { cls: "rs", attr: "data-rid", val: P.rid }) : null;
    }
    // item adds a menu item under box; a pick runs fn(row), or toasts why
    // a disabled item does nothing.
    function item(box, icon, text, fn) {
      var b = box.appendChild(rpBtn("rmi"));
      b.setAttribute("role", "menuitem");
      b.appendChild(icon ? gIcon(icon) : mk("span", "i"));
      b.appendChild(mk("span", "rmt", text));
      b.addEventListener("click", function () {
        if (b.getAttribute("aria-disabled") === "true") { if (b.title) toast(b.title); return; }
        var row = rowNow();
        if (row) fn(row);
        else railPopClose(false);
      });
      return b;
    }
    function items(box) {
      return Array.prototype.filter.call(box.children, function (b) { return b.getAttribute("role") === "menuitem" && !b.hidden; });
    }
    function subShow(on) {
      sub.hidden = !on;
      under.setAttribute("aria-expanded", String(on));
    }
    list.addEventListener("keydown", function (e) {
      var t = e.target, box = t.parentNode;
      if (e.altKey || e.ctrlKey || e.metaKey || e.isComposing || !box) return;
      var bs = items(box), i = bs.indexOf(t), n = -1;
      if (e.key === "ArrowDown") n = (i + 1) % bs.length;
      else if (e.key === "ArrowUp") n = i < 0 ? bs.length - 1 : (i + bs.length - 1) % bs.length;
      else if (e.key === "Home") n = 0;
      else if (e.key === "End") n = bs.length - 1;
      else if (e.key === "ArrowRight" && t === under && under.getAttribute("aria-disabled") !== "true") {
        e.preventDefault();
        if (sub.hidden) under.click();
        else if (items(sub)[0]) items(sub)[0].focus();
        return;
      } else if (e.key === "ArrowLeft" && sub && box === sub) {
        e.preventDefault();
        P.back();
        return;
      } else return;
      e.preventDefault();
      if (bs[n]) bs[n].focus();
    });
    // moveUnder moves row under the row pid, as a drop on its middle would.
    function moveUnder(row, pid) {
      var r = region("rail"), p = r && WsRail.refind(r, { cls: "rs", attr: "data-rid", val: pid });
      var z = p ? WsRail.dropZone(WsRail.rowFacts(row), WsRail.rowFacts(p), 0.5) : { refusal: "unknown" };
      railPopClose(true);
      if (z && z.zone === "child") railMove(row, z);
      else toast(wsT(WsRail.layoutRefusal((z && z.refusal) || "invalid")));
    }
    P.open = function (id, row) {
      P.gen++;
      P.rid = id;
      list.textContent = "";
      under = sub = null;
      list.setAttribute("aria-label", railName(row));
      var d = WsRail.rowFacts(row), pen = row.querySelector(".drren"), cp = row.querySelector(".drcopy"), rev = row.querySelector(".rrev");
      if (pen) item(list, "pencil", pen.getAttribute("aria-label") || "", function (r) {
        railPopClose(false);
        railRename(r);
      });
      if (cp) item(list, "copy", cp.getAttribute("aria-label") || "", function (r) {
        var c = r.querySelector(".drcopy");
        railPopClose(true);
        if (c) copyText(c.getAttribute("data-copy") || "");
      });
      if (row.hasAttribute("data-star")) {
        var on = WsRail.starred(row);
        var favorite = item(list, "pin", wsT(on ? "js.rail.menu.unstar" : "js.rail.menu.star"), function (r) {
          railPopClose(true);
          railStar(r, !WsRail.starred(r));
        });
        favorite.setAttribute("data-on", String(on));
        favorite.querySelector(".i").classList.add("rmstar");
      }
      if (d.live) {
        var u = WsRail.underRows(row), chev = gIcon("chev");
        under = item(list, "", wsT("js.rail.menu.under"), function () {
          subShow(sub.hidden);
          railPopPlace();
          var f = items(sub)[0];
          if (!sub.hidden && f) f.focus();
        });
        chev.setAttribute("class", "i rmgo");
        under.appendChild(chev);
        sub = list.appendChild(mk("div", "rmsub"));
        sub.id = rpId();
        sub.setAttribute("role", "menu");
        sub.setAttribute("aria-label", wsT("js.rail.menu.under"));
        under.setAttribute("aria-haspopup", "menu");
        under.setAttribute("aria-controls", sub.id);
        subShow(false);
        u.rows.forEach(function (k) {
          var pid = k.getAttribute("data-rid") || "";
          item(sub, "", railName(k), function (r) { moveUnder(r, pid); }).title = pid;
        });
        if (!u.rows.length) {
          under.setAttribute("aria-disabled", "true");
          under.title = wsT(u.refusal ? WsRail.layoutRefusal(u.refusal) : "js.rail.menu.nounder");
        }
      }
      if (d.live && d.parent) item(list, "back", wsT("js.rail.menu.detach"), function (r) {
        var z = WsRail.keyMove(r, "out");
        railPopClose(true);
        if (z && z.refusal) toast(wsT(WsRail.layoutRefusal(z.refusal)));
        else if (z) railMove(r, z);
      });
      if (rev) {
        var rb = item(list, "", rev.textContent, function (r) {
          var b = r.querySelector(".rrev");
          railPopClose(true);
          if (b) railRevive(b);
        });
        if (rev.title) rb.title = rev.title;
        if (rev.getAttribute("aria-disabled") === "true") rb.setAttribute("aria-disabled", "true");
      }
      if (row.querySelector(".hosted")) {
        if (!P.sess) P.sess = rpSessMake(true);
        el.appendChild(P.sess.el);
        P.sess.open(id);
      } else if (P.sess) P.sess.close();
    };
    // back closes the open "move under" list, the focus back on its item;
    // false when none is open.
    P.back = function () {
      if (!sub || sub.hidden) return false;
      subShow(false);
      railPopPlace();
      under.focus();
      return true;
    };
    P.focus = function () {
      var f = items(list)[0];
      if (f) f.focus();
    };
    P.close = function () {
      P.gen++;
      if (sub) subShow(false);
      if (P.sess) P.sess.close();
    };
    return P;
  }
  // railRowMenu opens row's menu at pt (the pointer, {x, y}) or, with
  // none, under the row. On the row whose menu is open it only moves it to
  // pt: a long touch and the contextmenu event a browser sends for it, or
  // Shift+F10 and the one some browsers send after it, open it once.
  function railRowMenu(row, pt) {
    var id = row.getAttribute("data-rid") || "", link = row.querySelector(".rl");
    if (!id || !link) return;
    var rr = row.getBoundingClientRect(), at = pt ? { dx: pt.x - rr.left, dy: pt.y - rr.top } : null;
    if (rp.P && rp.kind === "row" && rp.ws === id) {
      if (at) { rp.at = at; railPopPlace(); }
      return;
    }
    railPopClose(false);
    var P = rpRow || (rpRow = rpRowMake());
    if (!P.el.parentNode) doc.body.appendChild(P.el);
    rp = { P: P, kind: "row", ws: id, btn: link, at: at };
    P.open(id, row);
    P.el.hidden = false;
    railPopPlace();
    P.focus();
  }
  function rmCancel(e) {
    if (!rmHold || (e && e.pointerId !== rmHold.id)) return;
    clearTimeout(rmHold.timer);
    rmHold = null;
  }
  if (railEl && WsRail) {
    // A right click on a row opens its menu at the pointer (one the
    // keyboard sends, with no point in the row, under the row). A press on
    // a grip is the drag's (its long touch opens nothing).
    railEl.addEventListener("contextmenu", function (e) {
      var t = e.target, row = t.closest && !t.closest("input, textarea") ? t.closest(".rs") : null;
      if (!row || rdrag) return;
      e.preventDefault();
      var rr = row.getBoundingClientRect(), x = e.clientX, y = e.clientY;
      railRowMenu(row, x >= rr.left && x <= rr.right && y >= rr.top && y <= rr.bottom ? { x: x, y: y } : null);
    });
    // A right click anywhere else closes a row menu (inside it, it opens
    // no browser menu over it).
    doc.addEventListener("contextmenu", function (e) {
      var t = e.target;
      if (!rp.P || rp.kind !== "row" || !t || !t.closest) return;
      if (rp.P.el.contains(t)) { e.preventDefault(); return; }
      var row = t.closest(".rs");
      if (!row || !railEl.contains(row) || row.getAttribute("data-rid") !== rp.ws) railPopClose(false);
    }, true);
    // A touch held still on a row for RM_HOLD opens its menu there; one
    // that moves past RD_SLOP (a scroll) or lifts first does not. The
    // click the held touch may end with is the menu's, not the link's (a
    // keyboard's click, detail 0, never is).
    railEl.addEventListener("pointerdown", function (e) {
      rmHeld = false;
      rmCancel(null);
      if (e.pointerType !== "touch" || !e.isPrimary) return;
      var t = e.target, row = t.closest && !t.closest(".drgrip, input, textarea") ? t.closest(".rs") : null;
      if (!row) return;
      var id = row.getAttribute("data-rid") || "", h = { id: e.pointerId, x: e.clientX, y: e.clientY, timer: 0 };
      h.timer = setTimeout(function () {
        var r = region("rail"), now = r && WsRail.refind(r, { cls: "rs", attr: "data-rid", val: id });
        rmHold = null;
        if (!now || rdrag || railEditing()) return;
        rmHeld = true;
        railRowMenu(now, { x: h.x, y: h.y });
      }, RM_HOLD);
      rmHold = h;
    });
    doc.addEventListener("pointermove", function (e) {
      if (rmHold && e.pointerId === rmHold.id && (Math.abs(e.clientX - rmHold.x) > RD_SLOP || Math.abs(e.clientY - rmHold.y) > RD_SLOP)) rmCancel(e);
    });
    doc.addEventListener("pointerup", rmCancel);
    doc.addEventListener("pointercancel", rmCancel);
    window.addEventListener("click", function (e) {
      if (!rmHeld || !e.detail || !railEl.contains(e.target)) return;
      rmHeld = false;
      e.preventDefault();
      e.stopPropagation();
    }, true);
  }

  // --- plain-folder git init --------------------------------------------------
  // The modal lives outside the streamed rail, like the delete modal. Replies
  // to a closed/reopened instance must not close or overwrite the newer one.
  var rgit = null;
  function rgitMake() {
    var D = { seq: 0, path: "", busy: false, btn: null };
    var dl = D.el = mk("dialog", "rdeldlg rgitdlg");
    var hd = dl.appendChild(mk("h2", "rdhd"));
    hd.id = rpId();
    dl.setAttribute("aria-labelledby", hd.id);
    hd.appendChild(gIcon("gitinit"));
    hd.appendChild(mk("span", null, wsT("js.rail.gitinit.title")));
    var where = dl.appendChild(mk("code", "rdpath"));
    var lead = dl.appendChild(mk("p", "rdlead"));
    wsT("js.rail.gitinit.body", "\u0001").split("\u0001").forEach(function (part, i) {
      if (i) lead.appendChild(mk("code", null, wsT("js.rail.gitinit.submit")));
      if (part) lead.appendChild(doc.createTextNode(part));
    });
    var note = dl.appendChild(mk("p", "rdnote bad"));
    note.setAttribute("role", "alert");
    var row = dl.appendChild(mk("div", "rdrow"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn go", wsT("js.rail.gitinit.submit")));
    function button() {
      if (D.btn && D.btn.isConnected) return D.btn;
      var found = null;
      q(".rgit", region("rail") || railEl).forEach(function (b) {
        var ws = b.closest(".rws");
        if (ws && ws.getAttribute("data-ws") === D.path) found = b;
      });
      return found;
    }
    go.addEventListener("click", function () {
      if (D.busy) return;
      var n = D.seq;
      D.busy = go.disabled = true;
      note.textContent = "";
      note.hidden = true;
      railPost("/rail/workspaces/git-init", { path: D.path }, function (st, v, s) {
        if (st === 200) {
          if (n === D.seq) D.close();
          toast(String(v.message || ""));
          return;
        }
        if (n !== D.seq) { toast(railErr(st, s)); return; }
        D.busy = go.disabled = false;
        note.textContent = railErr(st, s);
        note.hidden = false;
      });
    });
    cancel.addEventListener("click", function () { D.close(); });
    dl.addEventListener("cancel", function (e) { e.preventDefault(); D.close(); });
    dl.addEventListener("close", function () {
      D.seq++;
      var b = button();
      if (b) { b.setAttribute("aria-expanded", "false"); b.focus(); }
    });
    D.open = function (btn, path) {
      D.seq++;
      D.path = path;
      D.btn = btn;
      D.busy = go.disabled = false;
      where.textContent = where.title = path;
      note.textContent = "";
      note.hidden = true;
      btn.setAttribute("aria-expanded", "true");
      if (!dl.parentNode) doc.body.appendChild(dl);
      if (!dl.open) dl.showModal();
      cancel.focus();
    };
    D.close = function () { if (dl.open) dl.close(); };
    return D;
  }
  function railGitInit(btn) {
    if (!root.hasAttribute("data-sysopen")) return;
    var ws = btn.closest(".rws"), path = ws && ws.getAttribute("data-ws");
    if (!path) return;
    railPopClose(false);
    rgit = rgit || rgitMake();
    rgit.open(btn, path);
  }

  // --- rail worktree delete ----------------------------------------------------
  // A linked worktree row's trash button (.rdel) opens the delete modal: a
  // <dialog> shown modal, made once outside the rail region (which every
  // update replaces). It reads the daemon's check (the preview route) and
  // shows it: the folder, the branch the delete keeps, the changed files, a
  // detached HEAD's unreached commits, submodules, the lock, and the
  // sessions and terminals in the worktree (a live one blocks, an ended one
  // is listed only); the refusal's note when there is one; and, when the
  // check needs force, that force will be used and why. Delete turns on
  // once the folder name is typed exactly against the current open's check
  // (WsRail.deleteReady): an answer to an earlier open is dropped. The
  // submit carries force and the check's token only when the check needs
  // force (WsRail.deleteBody). A 409 with a check (refused, or force needed
  // or moved since the check) shows that check in place and clears the
  // typed name, so the next confirmation is made against what is shown now;
  // a failure's text shows inline. A success closes the modal and toasts its
  // message; the stream redraws the rail. Escape and Cancel close it.
  var rdel = null;
  function rdelMake() {
    var D = { seq: 0, ws: "", path: "", shown: null, busy: false, btn: null };
    var dl = D.el = mk("dialog", "rdeldlg");
    var hd = dl.appendChild(mk("h2", "rdhd"));
    hd.id = rpId();
    dl.setAttribute("aria-labelledby", hd.id);
    hd.appendChild(gIcon("trash"));
    hd.appendChild(mk("span", null, wsT("js.rail.wt.del.title")));
    var where = dl.appendChild(mk("code", "rdpath"));
    var facts = dl.appendChild(mk("div", "rdfacts"));
    facts.setAttribute("aria-live", "polite");
    var note = dl.appendChild(mk("p", "rdnote"));
    var force = dl.appendChild(mk("div", "rdforce"));
    var conf = dl.appendChild(mk("label", "rdconf"));
    var confText = conf.appendChild(mk("span", "rdconft"));
    var inp = conf.appendChild(rpInput("rdq"));
    var msg = dl.appendChild(mk("p", "rdmsg"));
    msg.setAttribute("aria-live", "polite");
    var row = dl.appendChild(mk("div", "rdrow"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn danger", wsT("js.rail.wt.del.submit")));
    function setMsg(text, err) { msg.textContent = text || ""; msg.className = "rdmsg" + (err ? " err" : ""); msg.hidden = !text; }
    function sync() { go.disabled = D.busy || !WsRail.deleteReady(D.shown, inp.value, D.seq); }
    // fact is one line of the check: an icon, its text, and an optional tone.
    function fact(icon, text, tone) {
      var f = facts.appendChild(mk("div", "rdfact" + (tone ? " " + tone : "")));
      f.appendChild(gIcon(icon));
      f.appendChild(mk("span", null, text));
      return f;
    }
    // draw shows the check c. The worktree's state (changes, commits,
    // submodules, lock, holders) shows only once the daemon read it (the
    // token marks that): a refusal before the read (unknown, primary,
    // pooled, missing) has none to show.
    function draw(c) {
      var r = c.refusal ? String(c.refusal) : "", blocked = !!r && r !== "force" && r !== "changed", read = !!c.token;
      var folder = String(c.folder || "");
      where.textContent = String(c.path || D.path);
      where.title = where.textContent;
      facts.textContent = "";
      if (c.detached) fact("branch", wsT("js.rail.wt.del.detached"));
      else if (c.branch) fact("branch", wsT("js.rail.wt.del.branch", String(c.branch)));
      if (read) {
        var paths = Array.isArray(c.dirty_paths) ? c.dirty_paths : [], dirty = +c.dirty || 0;
        if (dirty > 0) {
          fact("pencil", wsTn("js.rail.wt.del.dirty", dirty), "warn");
          var ul = facts.appendChild(mk("ul", "rdpaths"));
          paths.forEach(function (p) { var li = ul.appendChild(mk("li", null, String(p))); li.title = li.textContent; });
          if (dirty > paths.length) ul.appendChild(mk("li", "more", wsT("js.rail.wt.del.more", dirty - paths.length)));
        } else fact("check", wsT("js.rail.wt.del.clean"));
        if (c.detached && +c.unreached > 0) fact("branch", wsTn("js.rail.wt.del.unreached", +c.unreached), "warn");
        if (c.submodules) fact("repo", wsT("js.rail.wt.del.submodules"), "warn");
        if (c.locked) fact("lock", c.lock_reason ? wsT("js.rail.wt.del.lockedwhy", String(c.lock_reason)) : wsT("js.rail.wt.del.locked"), "bad");
        var holders = Array.isArray(c.holders) ? c.holders : [];
        if (holders.length) {
          facts.appendChild(mk("div", "rdsub", wsT("js.rail.wt.del.holders")));
          var hl = facts.appendChild(mk("ul", "rdholders"));
          holders.forEach(function (h) {
            var li = hl.appendChild(mk("li", h.live ? "live" : null));
            var name = String(h.label || h.id || "");
            if (h.kind === "terminal") name = h.label ? wsT("js.rail.wt.del.terminalfrom", String(h.id || ""), String(h.label)) : wsT("js.rail.wt.del.terminal", String(h.id || ""));
            else if (h.harness) name += " (" + h.harness + ")";
            var nm = li.appendChild(mk("span", "nm", name));
            nm.title = nm.textContent;
            var state = h.live ? "js.rail.wt.del.live" : h.kind === "terminal" ? "js.rail.wt.del.exited" : "js.rail.wt.del.ended";
            li.appendChild(mk("span", "rdst", wsT(state)));
          });
        }
      }
      note.textContent = String(c.note || "");
      note.className = "rdnote" + (blocked ? " bad" : " warn");
      note.hidden = !c.note;
      force.textContent = "";
      force.hidden = blocked || !c.force;
      if (c.force && !blocked) {
        force.appendChild(mk("div", "rdforcehd", wsT("js.rail.wt.del.force")));
        var why = force.appendChild(mk("ul"));
        if (+c.dirty > 0) why.appendChild(mk("li", null, wsT("js.rail.wt.del.why.dirty")));
        if (+c.unreached > 0) why.appendChild(mk("li", null, wsT("js.rail.wt.del.why.unreached")));
        if (c.submodules) why.appendChild(mk("li", null, wsT("js.rail.wt.del.why.submodules")));
      }
      conf.hidden = blocked || !folder;
      confText.textContent = "";
      wsT("js.rail.wt.del.confirm", "\u0001").split("\u0001").forEach(function (part, i) {
        if (i) confText.appendChild(mk("code", null, folder));
        if (part) confText.appendChild(doc.createTextNode(part));
      });
      go.textContent = wsT(c.force ? "js.rail.wt.del.forcesubmit" : "js.rail.wt.del.submit");
    }
    // show displays the check c as request n's answer; again clears the
    // typed name, so a check that changed is confirmed afresh.
    function show(c, n, again) {
      D.shown = { key: n, check: c };
      draw(c);
      if (again) {
        inp.value = "";
        if (!conf.hidden) setMsg(wsT("js.rail.wt.del.again"), false);
      }
      sync();
      if (!conf.hidden) inp.focus();
    }
    function submit() {
      sync();
      if (go.disabled) return;
      var n = D.seq, c = D.shown.check;
      D.busy = true;
      setMsg("", false);
      go.textContent = wsT("js.rail.wt.del.busy");
      sync();
      railPost("/rail/worktrees/delete", WsRail.deleteBody(D.ws, D.path, c), function (st, v, s) {
        if (st === 200) {
          if (n === D.seq) D.close();
          toast(String(v.message || ""));
          return;
        }
        // A 409 answers with the fresh check, or with text while another
        // git action of the repository runs.
        var fresh = null;
        if (st === 409) { try { fresh = JSON.parse(s); } catch (e) { fresh = null; } }
        if (!fresh || typeof fresh !== "object" || Array.isArray(fresh)) fresh = null;
        if (n !== D.seq) { toast(fresh && fresh.note ? String(fresh.note) : railErr(st, fresh ? "" : s)); return; }
        D.busy = false;
        if (fresh) { show(fresh, n, true); return; }
        draw(c);
        setMsg(railErr(st, s), true);
        sync();
      });
    }
    inp.addEventListener("input", function () { if (!D.busy) setMsg("", false); sync(); });
    rpEnter(inp, submit);
    go.addEventListener("click", submit);
    cancel.addEventListener("click", function () { D.close(); });
    // The native Escape (cancel) closes through close too.
    dl.addEventListener("cancel", function (e) { e.preventDefault(); D.close(); });
    dl.addEventListener("close", function () {
      D.seq++;
      D.busy = false;
      var b = D.btn;
      if (!b || !b.isConnected) {
        b = null;
        q(".rdel", region("rail") || railEl).forEach(function (x) { if (x.getAttribute("data-path") === D.path) b = x; });
      }
      if (b) b.focus();
    });
    D.open = function (btn) {
      var n = ++D.seq;
      D.btn = btn;
      D.ws = btn.getAttribute("data-ws") || "";
      D.path = btn.getAttribute("data-path") || "";
      D.shown = null;
      D.busy = false;
      inp.value = "";
      setMsg("", false);
      draw({ path: D.path, folder: btn.getAttribute("data-folder") || "" });
      conf.hidden = true;
      facts.appendChild(mk("div", "rdfact", wsT("js.rail.wt.del.loading")));
      sync();
      if (!dl.parentNode) doc.body.appendChild(dl);
      if (!dl.open) dl.showModal();
      cancel.focus();
      railPost("/rail/worktrees/delete/preview", { workspace: D.ws, path: D.path }, function (st, v, s) {
        if (n !== D.seq) return;
        if (st !== 200) {
          facts.textContent = "";
          setMsg(railErr(st, s), true);
          sync();
          return;
        }
        show(v, n, false);
      });
    };
    D.close = function () { if (dl.open) dl.close(); };
    return D;
  }
  function railDelete(btn) {
    if (!btn.getAttribute("data-path")) return;
    rdel = rdel || rdelMake();
    rdel.open(btn);
  }
  // --- rail: put the idle sessions to sleep -----------------------------------
  // The header's moon reads the runner's idle list (GET /agents/idle), shows
  // it in a modal beside the delete modal's look, and on confirm posts the
  // listed ids (POST /agents/sleep-idle): the runner checks each is still
  // idle and stops it as the session menu's Stop does, skipping one that
  // became busy. The stream redraws the rows. Replies to an earlier open
  // are dropped by the sequence number.
  var rsleep = null;
  function rsleepMake() {
    var D = { seq: 0, ids: [], busy: false, btn: null };
    var dl = D.el = mk("dialog", "rdeldlg rsleepdlg");
    var hd = dl.appendChild(mk("h2", "rdhd"));
    hd.id = rpId();
    dl.setAttribute("aria-labelledby", hd.id);
    hd.appendChild(gIcon("moon"));
    var title = hd.appendChild(mk("span"));
    var lead = dl.appendChild(mk("p", "rdlead"));
    wsT("js.rail.sleep.body", "\u0001").split("\u0001").forEach(function (part, i) {
      if (i) lead.appendChild(mk("b", null, wsT("js.rail.sleep.body.em")));
      if (part) lead.appendChild(doc.createTextNode(part));
    });
    var list = dl.appendChild(mk("ul", "rdsleeplist"));
    var note = dl.appendChild(mk("p", "rdnote"));
    var msg = dl.appendChild(mk("p", "rdmsg err"));
    msg.setAttribute("aria-live", "polite");
    msg.hidden = true;
    var row = dl.appendChild(mk("div", "rdrow"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var go = row.appendChild(rpBtn("pbtn go"));
    go.appendChild(gIcon("moon"));
    go.appendChild(doc.createTextNode(wsT("js.rail.sleep.submit")));
    // rowOf is the rail row of session id: it names the workspace and the
    // branch, and carries the last activity.
    function rowOf(id) {
      var found = null;
      q(".rs[data-rid]", region("rail") || railEl).forEach(function (r) { if (!found && r.getAttribute("data-rid") === id) found = r; });
      return found;
    }
    function text(el) { return el ? el.textContent : ""; }
    function entry(s) {
      var id = String(s.id || ""), r = rowOf(id), li = mk("li");
      li.appendChild(mk("span", "hb")).setAttribute("data-h", String(s.backend || ""));
      var who = li.appendChild(mk("span", "who"));
      who.appendChild(mk("span", "nm", text(r && r.querySelector(".rid")) || String(s.label || id)));
      var ws = r && r.closest(".rws"), wt = r && r.closest(".rwt");
      var pj = [text(ws && ws.querySelector(".wsbar .nm")), text(wt && wt.querySelector(".wthd .br"))].filter(Boolean).join(" · ");
      if (pj) who.appendChild(mk("span", "pj", pj));
      var when = r && r.querySelector("time.when[data-ms]");
      if (when) {
        var t = li.appendChild(mk("time", "ago"));
        t.setAttribute("data-ms", when.getAttribute("data-ms"));
        t.setAttribute("data-f", "rel");
      }
      return li;
    }
    cancel.addEventListener("click", function () { D.close(); });
    // The native Escape (cancel) closes through close too.
    dl.addEventListener("cancel", function (e) { e.preventDefault(); D.close(); });
    dl.addEventListener("close", function () {
      D.seq++;
      D.busy = go.disabled = false;
      var b = D.btn;
      if (!b || !b.isConnected) b = q("#railsleep", region("rail") || railEl)[0];
      if (b && !b.disabled) b.focus();
    });
    go.addEventListener("click", function () {
      if (D.busy) return;
      var n = D.seq;
      D.busy = go.disabled = true;
      msg.hidden = true;
      railPost("/agents/sleep-idle", { ids: D.ids }, function (st, v, s) {
        if (st === 200) {
          if (n === D.seq) D.close();
          var slept = Array.isArray(v.slept) ? v.slept.length : 0, skipped = Array.isArray(v.skipped) ? v.skipped.length : 0;
          var line = wsTn("js.rail.sleep.done", slept) + (skipped ? " " + wsT("js.rail.sleep.skipped", skipped) : "");
          if (window.WsToast) window.WsToast.show(line, { kind: "summary" });
          return;
        }
        if (n !== D.seq) { toast(railErr(st, s)); return; }
        D.busy = go.disabled = false;
        msg.textContent = railErr(st, s);
        msg.hidden = false;
      });
    });
    D.open = function (btn) {
      var n = ++D.seq;
      D.btn = btn;
      btn.classList.add("busy");
      tkFetch("/agents/idle", undefined, function (st, v) {
        btn.classList.remove("busy");
        if (n !== D.seq) return;
        if (st !== 200) { toast(railErr(st, "")); return; }
        var sessions = Array.isArray(v.sessions) ? v.sessions : [];
        if (!sessions.length) { toast(wsT("js.rail.sleep.empty")); return; }
        D.ids = sessions.map(function (s) { return String(s.id); });
        D.busy = go.disabled = false;
        title.textContent = wsTn("js.rail.sleep.title", sessions.length);
        list.textContent = "";
        sessions.forEach(function (s) { list.appendChild(entry(s)); });
        var kept = +v.kept || 0, out = +v.uncontrollable || 0;
        note.textContent = [kept ? wsTn("js.rail.sleep.kept", kept) : "", out ? wsTn("js.rail.sleep.excluded", out) : ""].filter(Boolean).join(" ");
        note.hidden = !note.textContent;
        msg.hidden = true;
        if (!dl.parentNode) doc.body.appendChild(dl);
        if (!dl.open) dl.showModal();
        cancel.focus();
      });
    };
    D.close = function () { if (dl.open) dl.close(); };
    return D;
  }
  function railSleep(btn) {
    if (btn.disabled || btn.classList.contains("busy") || !root.hasAttribute("data-sysopen")) return;
    railPopClose(false);
    rsleep = rsleep || rsleepMake();
    rsleep.open(btn);
  }
  if (railEl && WsRail) {
    // Escape closes the delete modal before anything else hears it (the
    // rail overlay would close on it too).
    window.addEventListener("keydown", function (e) {
      var modal = rgit && rgit.el.open ? rgit : rdel && rdel.el.open ? rdel : rsleep && rsleep.el.open ? rsleep : null;
      if (e.key === "Escape" && modal) { e.preventDefault(); e.stopPropagation(); modal.close(); }
    }, true);
  }
  // A framed page opens the session menu (the shell's menu message) against
  // its own head's chip, which stands where the shell shows its copy.
  if ((railEl || framed) && WsRail) {
    // Escape closes the popover before anything else hears it (the rail
    // overlay and the drawer would close on it too); in a row menu it first
    // closes the open "move under" list.
    // The full-permission warning a popover opened closes first, as its
    // cancel: the popover stays.
    window.addEventListener("keydown", function (e) {
      if (e.key !== "Escape" || !rp.P) return;
      e.preventDefault();
      e.stopPropagation();
      if (permFullOpen()) { permFullDrop(); return; }
      if (!(rp.P.back && rp.P.back())) railPopClose(true);
    }, true);
    // A click outside the popover and its + closes it (the warning it
    // opened, and the head's permission chip, count as inside). Capture sees
    // the target before the popover's own handlers redraw it.
    doc.addEventListener("click", function (e) {
      var t = e.target;
      if (!rp.P || !t || !t.closest) return;
      if (rp.P.el.contains(t) || t.closest(".radd, .rnew, .hbtn, .usage-toggle, .settings-toggle, .pmdlg, .pmhead")) return;
      railPopClose(false);
    }, true);
    // A press inside an iframe never reaches the listener above (the shell's
    // own frames post "press"; a frame of a frame, a card's web page, does
    // not): the window blurs when focus moves into one, which closes it.
    // Checked a tick later, once activeElement names the frame; a blur
    // to another application leaves the popover.
    window.addEventListener("blur", function () {
      setTimeout(function () {
        var a = doc.activeElement;
        if (rp.P && a && a.tagName === "IFRAME") railPopClose(false);
      }, 0);
    });
    window.addEventListener("resize", railPopPlace);
    doc.addEventListener("scroll", function () { if (rp.P) railPopPlace(); }, true);
    if (RAIL_WIDE && RAIL_WIDE.addEventListener) RAIL_WIDE.addEventListener("change", function () { railPopClose(false); });
  }

