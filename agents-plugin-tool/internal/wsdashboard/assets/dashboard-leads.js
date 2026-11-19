  // --- daemon-hosted lead sessions ----------------------------------------------
  // A lead session the daemon spawned and feeds (the lead routes under
  // /agents) starts from the rail: the "+ session" at the end of a worktree
  // row or a plain-folder bar opens the spawn popover, a rail popover like
  // the two above. On such a session's page (the server marks it
  // data-hosted) the page reads the session's state from GET
  // /agents/<reply-id> on load, after a head update, and after a
  // conversation update that changed the permission line (the daemon
  // restamps that line on every approval change); no timer polls. From the
  // state it draws the approval cards, in a box under the conversation and
  // outside the regions so a card's typed deny message survives an update,
  // and makes the head's harness chip the button of the session menu (a
  // fourth rail popover). The controls show only when the state says this
  // viewer holds them (can_control: a browser on the daemon's machine); the
  // routes refuse anyone else whatever the page shows. A session handed to
  // a terminal (status handed_off) has no process here: it shows no
  // controls, only a notice with the command that resumes it there. The
  // server marks the page of such a session nothing runs now data-handed;
  // that page reads the state too, on load and after a head update.
  var hostedPage = mode !== "index" && root.hasAttribute("data-hosted");
  var handedPage = mode !== "index" && root.hasAttribute("data-handed");
  var LEAD_MODES = ["auto", "default", "acceptEdits", "plan", "dontAsk", "bypassPermissions"];
  var LEAD_MODE_NOTE = {
    auto: "js.lead.mode.auto", "default": "js.lead.mode.default", acceptEdits: "js.lead.mode.acceptedits",
    plan: "js.lead.mode.plan", dontAsk: "js.lead.mode.dontask", bypassPermissions: "js.lead.mode.bypass"
  };
  // The effort levels when the daemon's choices name none.
  var LEAD_EFFORTS = { claude: ["low", "medium", "high", "xhigh", "max"], pi: ["off", "minimal", "low", "medium", "high", "xhigh", "max"] };
  // lead is the viewed hosted session: its last read state, the read in
  // flight, the permission line the state was last read for, and the
  // approval cards by request id.
  var lead = { st: null, busy: false, again: false, key: "", cards: {}, need: null };
  // toolLabel is a tool's name as the server's rows show it: an MCP tool
  // without its server prefix.
  function toolLabel(name) {
    var m = /^mcp__.*?__(.+)$/.exec(name);
    return m ? m[1] : name;
  }
  function leadErr(status, text) { return text || (status ? "HTTP " + status : wsT("js.post.unreachable")); }
  // leadChoices reads the harness choices the support chat's popover uses
  // (its cache), then runs done.
  function leadChoices(done) {
    if (supChoices && !supChoices.error) { done(); return; }
    tkFetch("/support/choices", undefined, function (status, v) {
      if (status === 200 && Array.isArray(v.harnesses)) supChoices = v;
      done();
    });
  }
  // Older daemons have no lead markers; retain their Claude/Pi controls.
  function leadHarnesses() {
    var hs = ((supChoices && supChoices.harnesses) || []).filter(function (h) { return h.lead === true; });
    return hs.length ? hs : [supHarness("claude") || { harness: "claude" }, supHarness("pi") || { harness: "pi" }];
  }
  function leadModes(harness) {
    var h = supHarness(harness);
    if (leadHarnesses().some(function (x) { return x.lead === true; })) return h && Array.isArray(h.permission_modes) ? h.permission_modes : [];
    return harness === "claude" ? LEAD_MODES : [];
  }
  function leadModels(harness) {
    var h = supHarness(harness);
    return ((h && h.models) || []).map(function (m) { return String(m.id); });
  }
  function leadEfforts(harness) {
    var h = supHarness(harness);
    return h && Array.isArray(h.efforts) && h.efforts.length ? h.efforts.map(String) : (harness === "claude" || harness === "pi" ? LEAD_EFFORTS[harness] : []);
  }
  // leadOptions lists "default" and list in sel with cur picked (added when
  // the list lacks it). An unchanged list is left alone, so a redraw never
  // closes an open select.
  function leadOptions(sel, list, cur) {
    var all = list.slice();
    if (cur && all.indexOf(cur) < 0) all.push(cur);
    var sig = all.join("\n");
    if (sel.wsSig !== sig) {
      sel.wsSig = sig;
      sel.textContent = "";
      sel.appendChild(mk("option", null, wsT("js.sup.default"))).value = "";
      all.forEach(function (v) { sel.appendChild(mk("option", null, v)).value = v; });
    }
    if (sel.value !== (cur || "")) sel.value = cur || "";
  }
  // leadRows builds, under box, the three option rows the spawn popover and
  // the session menu share (model, permission mode, effort; label left,
  // control right); changed(select) runs on a pick.
  function leadRows(box, changed) {
    var R = {};
    function row(label, cls) {
      var r = box.appendChild(mk("div", "sprc")), l = r.appendChild(mk("label", null, label)), s = r.appendChild(mk("select", "spsm" + (cls ? " " + cls : "")));
      s.id = rpId();
      l.htmlFor = s.id;
      s.addEventListener("change", function () { changed(s); });
      return { row: r, sel: s };
    }
    var m = row(wsT("js.sup.model"), "mono");
    var p = row(wsT("js.lead.mode"));
    var pd = box.appendChild(mk("p", "spdesc"));
    var e = row(wsT("js.sup.effort"));
    var ed = box.appendChild(mk("p", "spdesc"));
    pd.id = rpId();
    p.sel.setAttribute("aria-describedby", pd.id);
    R.model = m.sel; R.mode = p.sel; R.effort = e.sel;
    // set shows harness's choices with v's model, effort, and mode picked.
    R.set = function (harness, v) {
      var h = supHarness(harness), models = leadModels(harness), efforts = leadEfforts(harness), modes = leadModes(harness);
      leadOptions(m.sel, models, v.model);
      leadOptions(e.sel, efforts, v.effort);
      var sig = modes.join("\n");
      if (p.sel.wsSig !== sig) {
        p.sel.wsSig = sig;
        p.sel.textContent = "";
        modes.forEach(function (mode) { p.sel.appendChild(mk("option", null, mode)).value = mode; });
      }
      p.row.classList.toggle("off", !modes.length);
      var initial = harness === "claude" && modes.indexOf("auto") >= 0 ? "auto" : modes[0] || "";
      p.sel.value = modes.indexOf(v.mode) >= 0 ? v.mode : initial;
      R.disable = function (busy) {
        var unavailable = !!h && (h.lead === true ? h.available !== true : h.available === false);
        m.sel.disabled = busy || unavailable || !models.length;
        e.sel.disabled = busy || unavailable || !efforts.length;
        p.sel.disabled = busy || unavailable || !modes.length;
      };
      R.disable(false);
      R.notes(harness, v.active);
    };
    // notes writes the lines under the mode and the effort; active is the
    // mode a running harness reported, named when it is not the picked one.
    R.notes = function (harness, active) {
      var claude = harness === "claude", mode = p.sel.value;
      pd.hidden = !mode && harness !== "pi";
      pd.textContent = !mode ? (harness === "pi" ? wsT("js.lead.mode.pi") : "") : (claude && LEAD_MODE_NOTE[mode] ? wsT(LEAD_MODE_NOTE[mode]) : mode) + (active && active !== mode ? " " + wsT("js.lead.mode.active", active) : "");
      pd.classList.toggle("warn", claude && mode === "bypassPermissions");
      ed.textContent = e.sel.value ? (claude ? "--effort " : harness === "pi" ? "--thinking " : "") + e.sel.value : wsT("js.lead.effort.default");
    };
    return R;
  }

  // The spawn popover for one worktree or plain folder: the target as one
  // read-only line (workspace, branch, path), the harness, and the option
  // rows. Start spawns the session and opens its page; the first message
  // is sent from that page's composer. A refused or failed spawn shows in
  // the popover's error line and keeps it open.
  function rpLeadMake() {
    var P = { gen: 0, busy: false, root: "", harness: "claude", iso: "default" };
    var settingsKey = "ws.dashboard.newSession", saved = {}, picks = Object.create(null);
    function readSettings() {
      try {
        var v = JSON.parse(localStorage.getItem(settingsKey));
        return v && typeof v === "object" && !Array.isArray(v) ? v : {};
      } catch (_) { return {}; }
    }
    function pick(k) {
      var v = picks[k];
      return v && typeof v === "object" && !Array.isArray(v) ? v : {};
    }
    function showPick(k) {
      var v = pick(k), models = leadModels(k), efforts = leadEfforts(k);
      rows.set(k, { model: models.indexOf(v.model) >= 0 ? v.model : "", effort: efforts.indexOf(v.effort) >= 0 ? v.effort : "", mode: v.mode });
      P.iso = v.iso === "isolated" ? "isolated" : "default";
    }
    function currentPick() {
      return { model: rows.model.disabled ? "" : rows.model.value, mode: rows.mode.disabled ? "" : rows.mode.value, effort: rows.effort.disabled ? "" : rows.effort.value, iso: P.iso };
    }
    var el = P.el = rpPop(wsT("js.lead.title"));
    el.appendChild(mk("div", "sphd", wsT("js.lead.title")));
    var loc = el.appendChild(mk("div", "sploc"));
    var hsec = el.appendChild(mk("div", "spsec"));
    var seg = hsec.appendChild(mk("div", "spseg"));
    seg.setAttribute("role", "radiogroup");
    seg.setAttribute("aria-label", wsT("js.sup.harness"));
    var hbtn = Object.create(null), harnessSig = "";
    function harnessButtons() {
      var hs = leadHarnesses(), sig = hs.map(function (h) { return h.harness; }).join("\n");
      if (harnessSig !== sig) {
        harnessSig = sig;
        seg.textContent = "";
        hbtn = Object.create(null);
        hs.forEach(function (h) {
          var k = h.harness, b = hbtn[k] = rpBtn("", k === "claude" ? "Claude" : k === "pi" ? "Pi" : k);
          b.setAttribute("role", "radio");
          b.setAttribute("data-harness", k);
          seg.appendChild(b);
        });
      }
      if (!hbtn[P.harness] || off(P.harness)) {
        P.harness = hs.filter(function (h) { return !off(h.harness); }).map(function (h) { return h.harness; })[0] || hs[0].harness;
      }
    }
    var rows = leadRows(el.appendChild(mk("div", "spopts")), edited);
    // Plugin isolation: which plugins the hosted session loads. The pick goes
    // out as plugin_isolation; Pi's standalone binary supports only isolated.
    var isec = el.appendChild(mk("div", "spsec"));
    isec.appendChild(mk("div", "splbl", wsT("js.lead.iso")));
    var iseg = isec.appendChild(mk("div", "spseg"));
    iseg.setAttribute("role", "radiogroup");
    iseg.setAttribute("aria-label", wsT("js.lead.iso"));
    var ibtn = { "default": rpBtn("", wsT("js.lead.iso.default")), isolated: rpBtn("", wsT("js.lead.iso.isolated")) };
    Object.keys(ibtn).forEach(function (k) { ibtn[k].setAttribute("role", "radio"); ibtn[k].setAttribute("data-iso", k); iseg.appendChild(ibtn[k]); });
    var idesc = isec.appendChild(mk("p", "spdesc"));
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var row = el.appendChild(mk("div", "sprow"));
    var status = row.appendChild(mk("span", "spnote"));
    var cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var start = row.appendChild(rpBtn("pbtn go", wsT("js.lead.start")));
    function setErr(text) { err.textContent = text || ""; err.hidden = !text; }
    function off(k) {
      var h = supHarness(k);
      return !hbtn[k] || (!!h && (h.lead === true ? h.available !== true : h.available === false));
    }
    function sync() {
      Object.keys(hbtn).forEach(function (k) {
        var h = supHarness(k);
        hbtn[k].setAttribute("aria-checked", String(P.harness === k));
        hbtn[k].disabled = P.loading || off(k);
        hbtn[k].title = off(k) ? wsT("js.sup.unavail", k, h && h.error || "") : "";
      });
      Object.keys(ibtn).forEach(function (k) { ibtn[k].setAttribute("aria-checked", String(P.iso === k)); });
      idesc.textContent = wsT(P.iso === "isolated" ? "js.lead.iso.isolated.hint" : "js.lead.iso.default.hint") + (P.harness === "pi" ? " " + wsT("js.lead.iso.pi") : "");
      start.disabled = P.busy || P.loading || off(P.harness);
    }
    // edited drops an earlier spawn's error once the form changed.
    function edited() {
      if (!P.busy) setErr("");
      rows.notes(P.harness);
      sync();
      railPopPlace();
    }
    function setHarness(k) {
      if (P.harness === k) return;
      picks[P.harness] = currentPick();
      P.harness = k;
      showPick(k);
      edited();
    }
    function submit() {
      sync();
      if (start.disabled) return;
      notifAsk();
      var gen = P.gen, chosenHarness = P.harness, chosen = currentPick();
      P.busy = true;
      setErr("");
      status.textContent = wsT("js.lead.starting");
      sync();
      railPost("/agents/leads", WsRail.leadBody(P.root, { harness: P.harness, model: rows.model.disabled ? "" : rows.model.value, effort: rows.effort.disabled ? "" : rows.effort.value, mode: rows.mode.disabled ? "" : rows.mode.value, isolation: P.iso }), function (st, v, s) {
        P.busy = false;
        status.textContent = "";
        sync();
        if (st === 201 && v.id) {
          var stored = readSettings(), byHarness = stored.harnesses;
          if (!byHarness || typeof byHarness !== "object" || Array.isArray(byHarness)) byHarness = {};
          byHarness[chosenHarness] = chosen;
          try { localStorage.setItem(settingsKey, JSON.stringify({ harness: chosenHarness, harnesses: byHarness })); } catch (_) { /* storage may be unavailable */ }
          if (rpLive(P, gen)) railPopClose(false);
          railOpen(false);
          saveDraft();
          try { sessionStorage.setItem(RENAME_NEW, JSON.stringify({ id: String(v.id), at: Date.now() })); } catch (_) { /* storage may be unavailable */ }
          go("/s/" + encodeURIComponent(String(v.id)));
          return;
        }
        if (!rpLive(P, gen)) { toast(leadErr(st, s)); return; }
        setErr(leadErr(st, s));
        sync();
        railPopPlace();
        // A spawn answers at once (the process starts on the first send), so
        // a request still open after 20s fails instead of staying busy.
      }, 20000);
    }
    iseg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-iso]"); if (b) { P.iso = b.getAttribute("data-iso"); sync(); } });
    seg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-harness]"); if (b && !b.disabled) setHarness(b.getAttribute("data-harness")); });
    cancel.addEventListener("click", function () { railPopClose(true); });
    start.addEventListener("click", submit);
    P.open = function (key, label, btn) {
      var gen = ++P.gen, name = btn.getAttribute("data-name") || label, branch = btn.getAttribute("data-branch") || "";
      P.busy = false;
      P.root = key;
      saved = readSettings();
      picks = Object.create(null);
      if (saved.harnesses && typeof saved.harnesses === "object" && !Array.isArray(saved.harnesses)) {
        Object.keys(saved.harnesses).forEach(function (k) { picks[k] = saved.harnesses[k]; });
      }
      P.harness = "claude";
      P.iso = "default";
      P.loading = true;
      loc.textContent = "";
      loc.appendChild(gIcon(branch ? "repo" : "wsfolder"));
      loc.appendChild(mk("span", null, name));
      if (branch) { loc.appendChild(gIcon("branch")); loc.appendChild(mk("b", null, branch)); }
      loc.appendChild(mk("code", null, key)).title = key;
      setErr("");
      status.textContent = "";
      harnessButtons();
      rows.set(P.harness, { model: "", effort: "", mode: "" });
      sync();
      leadChoices(function () {
        if (!rpLive(P, gen)) return;
        P.loading = false;
        P.harness = typeof saved.harness === "string" ? saved.harness : "claude";
        harnessButtons();
        showPick(P.harness);
        sync();
        railPopPlace();
      });
    };
    P.focus = function () { (hbtn[P.harness].disabled ? cancel : hbtn[P.harness]).focus(); };
    P.close = function () {};
    return P;
  }

  // The session menu of the viewed hosted session, on the head's harness
  // chip: model, permission mode, and effort (a pick is sent at once and
  // applies from the next turn), stop (the process ends; the next message
  // or mail starts it again), restart (shown while an MCP server stays
  // broken: the process ends and a new one resumes the conversation), hand
  // to terminal (the process ends and the page gives the command that
  // resumes the conversation in a terminal), and close (the conversation
  // ends for good, after a confirmation). Stop, restart, and hand to
  // terminal confirm first while the harness runs background tasks, which
  // they end, and close's confirmation then names their count too. A
  // refused change shows in the menu's error line, and the rows return to
  // what the session has.
  // With inRow it is instead the section a hosted row's menu (the rail row
  // menu) shows for that row's session, which may not be the viewed one:
  // P.rid is that session, its state (P.st) is read from GET
  // /agents/<P.rid> when the row menu opens, its actions post to that
  // session's routes, and the section shows only while its state says
  // this viewer holds the controls. A change to the viewed session updates
  // the page's own state (lead.st) too.
  function rpSessMake(inRow) {
    var P = { gen: 0, busy: false, rid: rid, st: null };
    var el = P.el = inRow ? mk("div", "rmsess") : rpPop(wsT("js.lead.menu"));
    if (inRow) {
      el.hidden = true;
      el.setAttribute("role", "group");
      el.setAttribute("aria-label", wsT("js.lead.menu"));
    }
    el.appendChild(mk("div", "sphd", wsT("js.lead.menu")));
    var rows = leadRows(el.appendChild(mk("div", "spopts")), function (sel) {
      var body = {};
      if (sel === rows.model) body.model = sel.value;
      else if (sel === rows.effort) body.effort = sel.value;
      else body.permission_mode = sel.value;
      act("settings", body);
    });
    el.appendChild(mk("p", "spnote", wsT("js.lead.menu.note")));
    var acts = el.appendChild(mk("div", "spacts"));
    function actBtn(icon, cls, title) {
      var b = acts.appendChild(rpBtn("spact" + cls));
      b.appendChild(typeof icon === "string" ? gIcon(icon) : icon);
      var s = b.appendChild(mk("span", null, title));
      return { b: b, note: s.appendChild(mk("span", "gs")) };
    }
    var stop = actBtn("stop", "", wsT("js.lead.stop")), restart = actBtn("reload", "", wsT("js.lead.restart"));
    // archive hides the session from the rail (no process, mail held until
    // unarchive); on an archived session it is unarchive (back to asleep).
    var archive = actBtn("archive", "", wsT("js.lead.archive")), archT = archive.b.lastChild.firstChild;
    var handoff = actBtn(luIcon("square-terminal"), "", wsT("js.lead.handoff")), close = actBtn("power", " danger", wsT("js.lead.close"));
    handoff.note.textContent = wsT("js.lead.handoff.note");
    close.note.textContent = wsT("js.lead.close.note");
    // state is the session's last read state.
    function state() { return inRow ? P.st : lead.st; }
    // bgTasks is the background task count of the last read state.
    function bgTasks() { var st = state(); return Number(st && st.background_tasks) || 0; }
    var err = el.appendChild(mk("p", "spnote err"));
    err.setAttribute("aria-live", "polite");
    var failed = "";
    function act(action, body) {
      if (P.busy) return;
      var gen = P.gen;
      P.busy = true;
      failed = "";
      P.draw();
      railPost("/agents/" + encodeURIComponent(P.rid) + "/" + action, body, function (st, v, s) {
        P.busy = false;
        if (st === 200 && v.id) {
          if (inRow && gen === P.gen) P.st = v;
          if (!inRow || (P.rid === rid && (hostedPage || handedPage))) lead.st = v;
          if ((action === "close" || action === "hide" || action === "unhide") && P.live(gen)) railPopClose(false);
          P.redraw();
          return;
        }
        var msg = action === "hide" || action === "unhide" ? archErr(action, st, s) : leadErr(st, s);
        if (P.live(gen)) failed = msg;
        else toast(msg);
        P.redraw();
        P.fetch();
      });
    }
    // live reports the menu still open as it was when gen was taken;
    // redraw shows a new state (the head's own menu redraws with the
    // page's state), and fetch reads it again.
    P.live = function (gen) { return inRow ? !!rp.P && rp.P.sess === P && P.gen === gen : rpLive(P, gen); };
    P.redraw = function () {
      if (!inRow) { leadDraw(); return; }
      P.draw();
      if (P.rid === rid && (hostedPage || handedPage)) leadDraw();
    };
    P.fetch = function () {
      if (!inRow) { leadFetch(); return; }
      var gen = P.gen;
      tkFetch("/agents/" + encodeURIComponent(P.rid), undefined, function (status, v) {
        if (gen !== P.gen) return;
        if (status === 200 && v.id) P.st = v;
        else if (status === 404) P.st = null;
        P.draw();
      });
    };
    stop.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.stop.confirm", n))) act("stop", {});
    });
    archive.b.addEventListener("click", function () {
      var st = state();
      act(st && st.hidden ? "unhide" : "hide", {});
    });
    restart.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.restart.confirm", n))) act("restart", {});
    });
    handoff.b.addEventListener("click", function () {
      var n = bgTasks();
      if (n < 1 || window.confirm(wsTn("js.lead.handoff.confirm", n))) act("handoff", {});
    });
    close.b.addEventListener("click", function () {
      var n = bgTasks();
      if (window.confirm(n > 0 ? wsTn("js.lead.close.confirm.bg", n) : wsT("js.lead.close.confirm"))) act("close", {});
    });
    // draw shows the session's state; it runs when the menu opens and after
    // every read of the state while it is open.
    P.draw = function () {
      var st = state();
      if (inRow) {
        var can = !!(st && st.can_control && !leadGone(st));
        if (el.hidden !== !can) {
          el.hidden = !can;
          railPopPlace();
        }
        if (!can) return;
      }
      if (!st) return;
      var text = failed || (st.error ? String(st.error) : "");
      rows.set(String(st.backend || ""), { model: String(st.model || ""), effort: String(st.effort || ""), mode: String(st.permission_mode || ""), active: st.process ? String(st.active_mode || "") : "" });
      rows.disable(P.busy);
      stop.b.disabled = P.busy || !st.process;
      stop.note.textContent = wsT(st.process ? "js.lead.stop.note" : "js.lead.stopped");
      var broken = Array.isArray(st.mcp_broken) ? st.mcp_broken.map(String) : [];
      restart.b.hidden = !broken.length;
      restart.b.disabled = P.busy;
      handoff.b.disabled = P.busy;
      archive.b.disabled = P.busy;
      if (archive.b.getAttribute("data-hidden") !== String(!!st.hidden)) {
        archive.b.setAttribute("data-hidden", String(!!st.hidden));
        archive.b.replaceChild(gIcon(st.hidden ? "archive-restore" : "archive"), archive.b.firstChild);
        archT.textContent = wsT(st.hidden ? "js.lead.unarchive" : "js.lead.archive");
        archive.note.textContent = wsT(st.hidden ? "js.lead.unarchive.note" : "js.lead.archive.note");
      }
      restart.note.textContent = broken.length ? wsT("js.lead.restart.note", broken.join(", ")) : "";
      close.b.disabled = P.busy;
      err.textContent = text;
      err.hidden = !text;
    };
    // open takes the row's session (inRow; the head's menu is the viewed
    // session's).
    P.open = function (id) {
      var gen = ++P.gen;
      P.busy = false;
      failed = "";
      if (inRow) {
        P.rid = id;
        P.st = null;
      }
      P.draw();
      leadChoices(function () { if (P.live(gen)) { P.draw(); railPopPlace(); } });
      P.fetch();
    };
    P.focus = function () { rows.model.focus(); };
    P.close = function () {
      if (!inRow) return;
      P.gen++;
      el.hidden = true;
    };
    return P;
  }

  // leadGone reports a state with no process to control: closed for good,
  // or handed to a terminal.
  function leadGone(st) { return !st || st.status === "closed" || st.status === "handed_off"; }
  function leadCan() { var st = lead.st; return !!(st && st.can_control && !leadGone(st)); }
  // leadFetch reads the viewed session's state and draws it; a read asked
  // for while one is in flight runs once after it.
  function leadFetch() {
    if ((!hostedPage && !handedPage) || !window.fetch) return;
    if (lead.busy) { lead.again = true; return; }
    lead.busy = true;
    tkFetch("/agents/" + encodeURIComponent(rid), undefined, function (status, v) {
      lead.busy = false;
      // A daemon that no longer runs the session answers 404: the page then
      // shows it as any session it only watches.
      if (status === 200 && v.id) lead.st = v;
      else if (status === 404) lead.st = null;
      if (status === 200 || status === 404) leadDraw();
      if (lead.again) { lead.again = false; leadFetch(); }
    });
  }
  // leadChip makes the head's harness chip the session menu's button for a
  // viewer who holds the controls. The head region is replaced on every
  // update, so this runs again after each.
  function leadChip() {
    var h = region("head"), chip = h && h.querySelector(".host.sse"), can = leadCan();
    if (!chip || can === chip.classList.contains("hbtn")) return;
    chip.classList.toggle("hbtn", can);
    if (can) {
      chip.setAttribute("role", "button");
      chip.setAttribute("tabindex", "0");
      chip.setAttribute("aria-haspopup", "dialog");
      chip.setAttribute("aria-label", wsT("js.lead.menu"));
      chip.setAttribute("aria-expanded", String(!!rp.P && rp.kind === "sess"));
      chip.appendChild(gIcon("chev"));
    } else {
      ["role", "tabindex", "aria-haspopup", "aria-label", "aria-expanded"].forEach(function (a) { chip.removeAttribute(a); });
      q("svg", chip).forEach(function (s) { s.remove(); });
    }
  }
  // apCard builds the card of approval request a: the tool, the request's
  // description, a readable preview of the input, and Allow and Deny (Deny
  // opens an optional message to send with it). A viewer without the
  // controls (can false) gets the card with one line saying where it is
  // answered. An answer that comes too late (409: another page answered, or
  // the harness withdrew the request) only reads the state again.
  function apCard(a, can) {
    var C = { busy: false };
    var el = C.el = mk("div", "apcard");
    var h = el.appendChild(mk("div", "aph"));
    h.appendChild(gIcon("lock"));
    h.appendChild(mk("b", null, wsT("js.ap.title")));
    h.appendChild(mk("code", null, toolLabel(String(a.tool || "")))).title = String(a.tool || "");
    if (a.agent_id) h.appendChild(mk("span", "apsub", wsT("js.ap.agent")));
    C.n = h.appendChild(mk("span", "apn"));
    if (a.started_at) {
      var t = h.appendChild(mk("time", "t"));
      t.setAttribute("data-ms", String(a.started_at));
      t.setAttribute("data-f", "since");
    }
    var b = el.appendChild(mk("div", "apb")), text = WsRail.approvalText(a.input);
    if (a.description) b.appendChild(mk("div", "apdesc", String(a.description)));
    if (text) b.appendChild(mk("pre", "apin", text));
    if (!can) {
      var ro = el.appendChild(mk("div", "apro"));
      ro.appendChild(gIcon("lock"));
      ro.appendChild(mk("span", null, wsT("js.ap.readonly")));
      return C;
    }
    function btn(box, cls, icon, label) {
      var x = box.appendChild(rpBtn(cls));
      if (icon) x.appendChild(gIcon(icon));
      x.appendChild(mk("span", null, label));
      return x;
    }
    var act = el.appendChild(mk("div", "apact"));
    var allow = btn(act, "btn allow", "check", wsT("js.ap.allow")), deny = btn(act, "btn deny", "x", wsT("js.ap.deny"));
    deny.setAttribute("aria-expanded", "false");
    var dn = el.appendChild(mk("div", "apdeny"));
    dn.hidden = true;
    var msg = dn.appendChild(mk("textarea", "apmsg"));
    msg.rows = 2;
    msg.placeholder = wsT("js.ap.deny.placeholder");
    msg.setAttribute("aria-label", wsT("js.ap.deny.placeholder"));
    var dr = dn.appendChild(mk("div", "aprow"));
    dr.appendChild(mk("span", "apnote", wsT("js.ap.deny.note")));
    var back = btn(dr, "btn", "", wsT("js.rail.cancel")), send = btn(dr, "btn go deny", "", wsT("js.ap.deny.send"));
    var err = el.appendChild(mk("p", "aperr"));
    err.hidden = true;
    err.setAttribute("aria-live", "polite");
    function sync() { [allow, deny, back, send, msg].forEach(function (x) { x.disabled = C.busy; }); }
    function denying(on) {
      var pin = pinned();
      dn.hidden = !on;
      act.hidden = on;
      deny.setAttribute("aria-expanded", String(on));
      (on ? msg : deny).focus();
      if (pin) pinConvo();
    }
    function answer(behavior) {
      if (C.busy) return;
      notifAsk();
      var body = { behavior: behavior }, why = msg.value.trim();
      if (behavior === "deny" && why) body.message = why;
      C.busy = true;
      err.hidden = true;
      sync();
      railPost("/agents/" + encodeURIComponent(rid) + "/approvals/" + encodeURIComponent(String(a.id)), body, function (st, v, s) {
        C.busy = false;
        sync();
        if (st === 200 && v.id) { lead.st = v; leadDraw(); return; }
        if (st === 409) { leadFetch(); return; }
        err.textContent = leadErr(st, s);
        err.hidden = false;
      });
    }
    allow.addEventListener("click", function () { answer("allow"); });
    deny.addEventListener("click", function () { denying(true); });
    back.addEventListener("click", function () { denying(false); });
    send.addEventListener("click", function () { answer("deny"); });
    msg.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !e.isComposing) { e.preventDefault(); answer("deny"); }
      else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); denying(false); }
    });
    return C;
  }
  // leadDraw shows the read state: the chip, the open session menu, one
  // card per waiting approval (oldest first, numbered when several wait),
  // and, when the harness waits on an answer no card can give, a notice in
  // their place. While the box shows anything, the conversation's own
  // permission line (the same wait, as text) is hidden.
  function leadDraw() {
    var st = lead.st, box = $("apbox");
    leadChip();
    if (rp.P && rp.kind === "sess") {
      if (leadCan()) rp.P.draw();
      else railPopClose(false);
    }
    handedDraw(st);
    harnessDraw(st);
    if (!box) return;
    var open = !leadGone(st), pin = pinned(), keep = {};
    var list = open && Array.isArray(st.approvals) ? st.approvals.filter(function (a) { return a && a.id; }) : [];
    var need = open && !list.length && st.action_needed ? String(st.action_needed) : "";
    list.forEach(function (a) { keep[a.id] = true; });
    Object.keys(lead.cards).forEach(function (id) {
      if (keep[id]) return;
      lead.cards[id].el.remove();
      delete lead.cards[id];
    });
    list.forEach(function (a, i) {
      var c = lead.cards[a.id] || (lead.cards[a.id] = apCard(a, !!st.can_control));
      if (box.children[i] !== c.el) box.insertBefore(c.el, box.children[i] || null);
      c.n.textContent = (i + 1) + "/" + list.length;
      c.n.hidden = list.length < 2;
    });
    if (need && !lead.need) {
      lead.need = mk("div", "apneed");
      lead.need.appendChild(gIcon("lock"));
      lead.need.appendChild(mk("span", null, wsT("js.ap.needed")));
    }
    if (lead.need) {
      // the daemon's own wording, in English, stays readable on hover
      lead.need.title = need;
      if (need) box.appendChild(lead.need);
      else lead.need.remove();
    }
    box.hidden = !list.length && !need;
    if (convo) convo.classList.toggle("ap-on", !box.hidden);
    fmtTimes(box);
    if (pin) pinConvo();
  }
  // handedDraw shows, while st says the session was handed to a terminal,
  // the notice that says so under the banner, with the command that resumes
  // it there and its copy button when the state carries one; any other
  // state takes the notice away. The notice is outside the regions, so an
  // update leaves it.
  var handedEl = null;
  function handedDraw(st) {
    var n = WsRail ? WsRail.handedNotice(st) : null;
    if (!n) { if (handedEl) { handedEl.remove(); handedEl = null; } return; }
    if (handedEl && handedEl.wsKey === n.command) return;
    var banner = $("dash-banner");
    if (!banner) return;
    var el = mk("div", "handed");
    el.wsKey = n.command;
    el.setAttribute("role", "status");
    el.appendChild(luIcon("square-terminal"));
    var body = el.appendChild(mk("div", "hdbody"));
    body.appendChild(mk("span", "hdtext", wsT("js.lead.handed")));
    if (n.command) {
      body.appendChild(mk("span", "hdlbl", wsT("js.lead.handed.resume")));
      var line = body.appendChild(mk("span", "hdcmd"));
      line.appendChild(mk("code", null, n.command));
      var copy = line.appendChild(rpBtn("hdcopy"));
      copy.title = wsT("js.lead.handed.copy");
      copy.setAttribute("aria-label", wsT("js.lead.handed.copy"));
      copy.appendChild(gIcon("copy"));
      copy.addEventListener("click", function () { copyText(n.command); });
    }
    if (handedEl) handedEl.remove();
    banner.parentNode.insertBefore(el, banner.nextSibling);
    handedEl = el;
  }
  // Warn once per distinct harness notice for the lifetime of this page,
  // even when state updates clear a notice and later bring it back.
  var harnessNotices = new Set();
  function harnessDraw(st) {
    var c = !leadGone(st) && st.capabilities, lines = [];
    if (c && c.unverified) {
      var range = c.oldest && c.newest && c.oldest !== c.newest ? c.oldest + "\u2013" + c.newest : String(c.newest || c.oldest || c.profile_version || "");
      lines.push(wsT("js.lead.harness.unverified", c.version ? String(c.version) : wsT("js.lead.harness.unreported"), range, String(c.profile_version || "")));
    }
    if (c && Array.isArray(c.demoted) && c.demoted.length) lines.push(wsT("js.lead.harness.demoted", c.demoted.map(String).join(", ")));
    if (!lines.length || !window.WsToast) return;
    var key = lines.join(" ");
    if (harnessNotices.has(key)) return;
    window.WsToast.show(key, { kind: "warn" });
    harnessNotices.add(key);
  }
  // leadKey is the conversation's permission line as the state follows it:
  // its tool, arguments, and stamp; empty when the session waits on none.
  function leadKey() {
    var c = region("convlive"), slot = c && c.querySelector(".apslot");
    if (!slot) return "";
    var t = slot.querySelector("time[data-ms]"), code = slot.querySelector("code"), args = slot.querySelector(".args");
    return [t ? t.getAttribute("data-ms") : "", code ? code.title : "", args ? args.textContent : ""].join("\n");
  }
  // leadSwapped runs after an update of a region the state follows.
  function leadSwapped(name) {
    if (handedPage && !hostedPage) {
      if (name === "head") leadFetch();
      return;
    }
    if (!hostedPage) return;
    if (name === "head") {
      leadChip();
      if (rp.P && rp.kind === "sess") railPopSync();
      leadFetch();
      return;
    }
    var key = leadKey();
    if (key === lead.key) return;
    lead.key = key;
    leadFetch();
  }

