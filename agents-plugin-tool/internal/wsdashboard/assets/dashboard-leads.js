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
  var lead = { st: null, busy: false, again: false, gone: false, key: "", cards: {}, need: null };
  // toolLabel is a tool's name as the server's rows show it: an MCP tool
  // without its server prefix.
  function toolLabel(name) {
    var m = /^mcp__.*?__(.+)$/.exec(name);
    return m ? m[1] : name;
  }
  function leadErr(status, text) { return text || (status ? "HTTP " + status : wsT("js.post.unreachable")); }
  // Shared by the child header, tree, and HUD. Keep a pending/closed set so
  // repeated clicks cannot send two close requests while a stream catches up.
  var subagentClosing = Object.create(null), subagentDialog = null;
  function markChildCloseButton(el) {
    var closed = wsT("js.subagent.closed");
    el.disabled = true;
    el.title = closed;
    el.setAttribute("aria-label", closed);
    var lbl = el.querySelector && el.querySelector(".tph-close-lbl");
    if (lbl) lbl.textContent = closed;
    else el.textContent = closed;
  }
  function subagentClose(replyId, label, state, trigger) {
    if (!root.hasAttribute("data-can-control") || !replyId || subagentClosing[replyId]) return;
    subagentClosing[replyId] = true;
    function restore() { if (trigger && trigger.isConnected && trigger.focus) trigger.focus(); }
    function cancel() { delete subagentClosing[replyId]; restore(); }
    function send() {
      if (trigger) trigger.disabled = true;
      fetch("/agents/" + encodeURIComponent(replyId) + "/close", {
        method: "POST", credentials: "same-origin", cache: "no-store"
      }).then(function (r) {
        return r.text().then(function (body) {
          if (r.status !== 200) throw new Error(leadErr(r.status, body.trim()));
          document.querySelectorAll("[data-reply-id]").forEach(function (el) {
            if (el.getAttribute("data-reply-id") === replyId) {
              el.setAttribute("data-subagent-closed", "1");
              if (el.hasAttribute("data-child-close")) markChildCloseButton(el);
            }
          });
          subagentClosing[replyId] = "closed";
          document.dispatchEvent(new CustomEvent("ws:subagent-closed", { detail: { replyId: replyId } }));
        });
      }).catch(function (err) {
        delete subagentClosing[replyId];
        if (trigger && trigger.isConnected) trigger.disabled = false;
        toast(wsT("js.subagent.close.error", err.message || wsT("js.post.unreachable")));
      }).then(restore);
    }
    if (state === "idle" || state === "done" || state === "failed") { send(); return; }
    if (subagentDialog && subagentDialog.open) { cancel(); return; }
    var dl = mk("dialog", "rdeldlg"), title = dl.appendChild(mk("h2", "rdhd"));
    subagentDialog = dl;
    title.id = rpId();
    title.textContent = wsT("js.subagent.close.title", label || replyId);
    dl.setAttribute("aria-labelledby", title.id);
    var phase = state === "replying" ? "thinking" : state;
    var phaseKey = { thinking: "js.hud.state.thinking", tool: "js.hud.state.tool", holding: "js.hud.state.holding", permission: "js.hud.state.permission" }[phase] || "js.hud.state.thinking";
    dl.appendChild(mk("div", "rdpath", wsT("js.subagent.close.state", wsT(phaseKey))));
    dl.appendChild(mk("p", "", wsT("js.subagent.close.explain")));
    var row = dl.appendChild(mk("div", "rdrow"));
    var no = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
    var yes = row.appendChild(rpBtn("pbtn danger", wsT("js.subagent.close")));
    function end(confirmed) {
      dl.close(); dl.remove(); subagentDialog = null;
      if (confirmed) send(); else cancel();
    }
    no.addEventListener("click", function () { end(false); });
    yes.addEventListener("click", function () { end(true); });
    dl.addEventListener("cancel", function (e) { e.preventDefault(); end(false); });
    dl.addEventListener("click", function (e) { if (e.target === dl) end(false); });
    document.body.appendChild(dl); dl.showModal(); no.focus();
  }
  if (typeof document !== "undefined") document.addEventListener("click", function (e) {
    var b = e.target && e.target.closest && e.target.closest("[data-child-close]");
    if (!b) return;
    subagentClose(b.getAttribute("data-reply-id"), b.getAttribute("data-label"), b.getAttribute("data-state"), b);
  });
  // leadChoices reads the harness choices the support chat's popover uses,
  // then runs done. It reads them on every open, as that popover does: the
  // daemon's lists change while the page lives (a CLI update adds models). A
  // list already held answers at once, and refresh runs when the newer one
  // arrives.
  // A list read here is stamped with the connection epoch it was read in;
  // only a list of the current epoch can offer common permission levels.
  function leadChoices(done, refresh) {
    var held = !!supChoices && !supChoices.error, epoch = permEpoch;
    if (held) done();
    tkFetch("/support/choices", undefined, function (status, v) {
      if (status === 200 && Array.isArray(v.harnesses)) { v.wsEpoch = epoch; supChoices = v; }
      else if (supChoices) supChoices.wsEpoch = -1;
      if (!held) done();
      else if (refresh) refresh();
    });
  }

  // --- common permission level ------------------------------------------------
  // A hosted lead may carry a common permission level (read-only,
  // workdir-write, full) beside its native permission mode. The level is
  // intent, not a sandbox: the state reports how it is carried (partial,
  // advisory) apart from the native mode the harness reports. A level is
  // offered and sent only on positive support from the current daemon: the
  // harness's permission_levels in a /support/choices list read since the
  // live stream last (re)connected (permEpoch counts the drops; a daemon
  // replaced underneath drops the stream). Every selection-bearing request
  // re-reads the list first, goes only to a /permission route, and a refusal
  // is shown, never resent on a legacy route. Without support the native
  // controls work as before, and nothing selects a level for the user.
  var PERM_LEVELS = ["read-only", "workdir-write", "full"];
  var PERM_LABEL = { "": "js.perm.level.default", "read-only": "js.perm.level.readonly", "workdir-write": "js.perm.level.workdir", full: "js.perm.level.full" };
  var PERM_WHAT = { "": "js.perm.what.default", "read-only": "js.perm.what.readonly", "workdir-write": "js.perm.what.workdir", full: "js.perm.what.full" };
  var PERM_Q = { partial: "js.perm.q.partial", advisory: "js.perm.q.advisory", pending: "js.perm.q.pending", enforced: "js.perm.q.enforced" };
  // The native mode the daemon maps each level to, where it has one.
  var PERM_NATIVE = {
    claude: { "read-only": "plan", "workdir-write": "acceptEdits", full: "bypassPermissions" },
    codex: { "read-only": "read-only", "workdir-write": "workspace-write", full: "danger-full-access" }
  };
  var permEpoch = 0, permDlg = null;
  function permHarness(k) { return k === "claude" ? "Claude" : k === "pi" ? "Pi" : k; }
  // permLevels is harness's offered levels in order: null while unknown (no
  // list of the current epoch), empty when the daemon offers none.
  function permLevels(harness) {
    if (!supChoices || supChoices.error || supChoices.wsEpoch !== permEpoch) return null;
    var h = supHarness(harness), have = h && Array.isArray(h.permission_levels) ? h.permission_levels : [];
    return PERM_LEVELS.filter(function (l) { return have.indexOf(l) >= 0; });
  }
  // permPreflight re-reads the choices and answers whether the daemon that
  // answers now offers level on an available lead-capable harness.
  function permPreflight(harness, level, done) {
    var epoch = permEpoch;
    tkFetch("/support/choices", undefined, function (status, v) {
      if (status === 200 && Array.isArray(v.harnesses)) { v.wsEpoch = epoch; supChoices = v; }
      else if (supChoices) supChoices.wsEpoch = -1;
      var h = supHarness(harness);
      done(!!h && h.lead === true && h.available === true && (permLevels(harness) || []).indexOf(level) >= 0);
    });
  }
  // permRefused reports a selection route the daemon does not have: an
  // older daemon answers 404 or 405 before any creation or change.
  function permRefused(status) { return status === 404 || status === 405; }
  function permQ(kind) {
    var k = String(kind || "");
    return mk("span", "pmq " + (PERM_Q[k] ? k : "pending"), PERM_Q[k] ? wsT(PERM_Q[k]) : k);
  }
  // permCoded appends text to el with its \u0001 marker replaced by the
  // harness name and the native mode as code.
  function permCoded(el, text, harness, mode) {
    text.split("\u0001").forEach(function (part, i) {
      if (i) { el.appendChild(mk("span", null, permHarness(harness))); el.appendChild(mk("code", null, mode)); }
      if (part.trim()) el.appendChild(mk("span", null, part.trim()));
    });
  }
  // permMap fills el with how harness carries level: the native mode it
  // runs in (mode; for no level the native pick) and the quality to expect.
  // Pi has no native mode, so every level is advisory there.
  function permMap(el, harness, level, mode, change) {
    el.textContent = "";
    if (harness === "pi") {
      el.appendChild(mk("span", null, wsT("js.perm.map.pi", permHarness(harness))));
      el.appendChild(permQ("advisory"));
    } else if (PERM_NATIVE[harness] && mode) {
      permCoded(el, wsT(!level ? "js.perm.map.default" : change ? "js.perm.map.switch" : "js.perm.map.run", "\u0001"), harness, mode);
      if (level) {
        el.appendChild(mk("span", null, "·"));
        el.appendChild(permQ(level === "read-only" ? "partial" : "advisory"));
        if (level === "read-only") el.appendChild(mk("span", null, wsT("js.perm.map.partial")));
      }
    }
    el.hidden = !el.children.length;
  }
  // permSeg is the level segment under box; picked(level) runs on a click.
  // set shows levels (a leading "" is the default), cur checked, derived
  // dashed and tagged (a level derived from older settings, not chosen).
  function permSeg(box, picked) {
    var S = { el: box.appendChild(mk("div", "spseg pmseg")), sig: null, btn: {} };
    S.el.setAttribute("role", "radiogroup");
    S.el.setAttribute("aria-label", wsT("js.perm.title"));
    S.el.addEventListener("click", function (e) {
      var b = e.target && e.target.closest && e.target.closest("[data-level]");
      if (b && !b.disabled) picked(b.getAttribute("data-level"));
    });
    S.set = function (levels, cur, derived, off) {
      var sig = levels.join("\n") + "\n" + derived;
      if (S.sig !== sig) {
        S.sig = sig;
        S.el.textContent = "";
        S.btn = {};
        levels.forEach(function (l) {
          var dashed = !!derived && l === derived;
          var b = S.btn[l] = S.el.appendChild(rpBtn([l === "full" ? "full" : "", dashed ? "derived" : ""].join(" ").trim(), wsT(PERM_LABEL[l])));
          b.setAttribute("role", "radio");
          b.setAttribute("data-level", l);
          if (dashed) b.appendChild(mk("span", "pmtag", wsT("js.perm.tag.derived")));
        });
      }
      levels.forEach(function (l) {
        S.btn[l].setAttribute("aria-checked", String(l === cur));
        S.btn[l].disabled = !!off;
      });
    };
    return S;
  }
  // permFullAsk opens the full-permission warning for harness (o.path
  // names the target; o.change a running session's change). Its grant
  // enables only once the box is ticked; ok runs on that grant, no on
  // cancel (button, Escape), which sends nothing.
  function permFullAsk(o, ok, no) {
    var D = permDlg;
    if (!D) {
      D = permDlg = { el: mk("dialog", "rdeldlg pmdlg"), ok: null, no: null };
      var dl = D.el, hd = dl.appendChild(mk("h2", "rdhd"));
      hd.id = rpId();
      dl.setAttribute("aria-labelledby", hd.id);
      hd.appendChild(gIcon("lock")).setAttribute("class", "i pmlock");
      D.title = hd.appendChild(mk("span"));
      D.path = dl.appendChild(mk("div", "rdpath"));
      D.facts = dl.appendChild(mk("div", "rdfacts"));
      var ack = dl.appendChild(mk("label", "pmack"));
      D.box = ack.appendChild(mk("input"));
      D.box.type = "checkbox";
      ack.appendChild(mk("span", null, wsT("js.perm.full.ack")));
      var row = dl.appendChild(mk("div", "rdrow"));
      D.cancel = row.appendChild(rpBtn("pbtn", wsT("js.rail.cancel")));
      D.go = row.appendChild(rpBtn("pbtn danger"));
      D.box.addEventListener("change", function () { D.go.disabled = !D.box.checked; });
      D.cancel.addEventListener("click", function () { D.end(false); });
      dl.addEventListener("cancel", function (e) { e.preventDefault(); D.end(false); });
      D.go.addEventListener("click", function () { if (D.box.checked) D.end(true); });
      D.end = function (yes) {
        var f = yes ? D.ok : D.no;
        D.ok = D.no = null;
        if (D.el.open) D.el.close();
        if (f) f();
      };
    }
    function fact(cls, icon, text, mode) {
      var f = D.facts.appendChild(mk("div", "rdfact" + (cls ? " " + cls : "")));
      f.appendChild(gIcon(icon));
      var s = f.appendChild(mk("span"));
      if (!mode) { s.textContent = text; return; }
      text.split("\u0001").forEach(function (part, i) {
        if (i) s.appendChild(mk("span", "pmc", mode));
        if (part) s.appendChild(mk("span", null, part));
      });
    }
    if (D.no) D.end(false);
    D.facts.textContent = "";
    var native = PERM_NATIVE[o.harness] && PERM_NATIVE[o.harness].full;
    if (native) fact("bad", "lock", wsT("js.perm.full.native", permHarness(o.harness), "\u0001"), native);
    else fact("bad", "lock", wsT("js.perm.full.plain", permHarness(o.harness)));
    fact("warn", "folder", wsT("js.perm.full.outside"));
    fact("", "wrench", wsT("js.perm.full.caps"));
    fact("", "agents", wsT("js.perm.full.agents"));
    D.title.textContent = wsT(o.change ? "js.perm.full.title.change" : "js.perm.full.title");
    D.path.textContent = o.path || "";
    D.path.hidden = !o.path;
    D.go.textContent = wsT(o.change ? "js.perm.full.go.change" : "js.perm.full.go");
    D.box.checked = false;
    D.go.disabled = true;
    D.ok = ok;
    D.no = no;
    if (!D.el.parentNode) doc.body.appendChild(D.el);
    if (!D.el.open) D.el.showModal();
    D.cancel.focus();
  }
  // permStreamSync follows the live stream: a drop advances the epoch (the
  // support read before it no longer counts), and an open lead or session
  // popover redraws, reading the choices again once the stream is back.
  function permStreamSync(on) {
    if (!on) permEpoch++;
    var sp = rp.P && (rp.P.permSync ? rp.P : rp.P.sess && rp.P.sess.permSync ? rp.P.sess : null);
    if (sp) sp.permSync(on);
  }
  // permFullOpen reports the warning open; permFullDrop cancels it.
  function permFullOpen() { return !!(permDlg && permDlg.el.open); }
  function permFullDrop() { if (permDlg && (permDlg.el.open || permDlg.no)) permDlg.end(false); }
  // permObserved reports whether st carries a native mode the running
  // harness reported; a harness without native modes has none to report.
  function permObserved(st) { return !!(st.process && st.active_mode); }
  // permEvidence is the quality pill's kind for st: pending while a native
  // mode is asked for but not yet reported, else the daemon's own grade.
  function permEvidence(st) {
    return st.permission_mode && !permObserved(st) ? "pending" : String(st.permission_enforcement || "");
  }
  // permState fills box with st's permission state: the intent and its
  // source, the native mode (asked for, or as reported), and the evidence,
  // never merged into one claim. note is a closing line (may be empty).
  function permState(box, st, note) {
    box.textContent = "";
    var level = st && String(st.permission_level || "");
    box.hidden = !level;
    if (!level) return;
    var harness = String(st.backend || ""), explicit = st.permission_source === "explicit";
    function row(key) {
      box.appendChild(mk("span", "pmk", wsT(key)));
      return box.appendChild(mk("span", "pmv"));
    }
    var v = row("js.perm.k.intent");
    v.appendChild(mk("b", null, PERM_LABEL[level] ? wsT(PERM_LABEL[level]) : level));
    v.appendChild(mk("span", "pmsrc", wsT(explicit ? "js.perm.src.explicit" : "js.perm.src.derived")));
    v = row("js.perm.k.native");
    var asked = String(st.permission_mode || ""), seen = permObserved(st) ? String(st.active_mode) : "";
    if (!asked && !seen) v.appendChild(mk("span", "pmsrc", wsT("js.perm.native.none", permHarness(harness))));
    else {
      v.appendChild(mk("span", null, permHarness(harness)));
      v.appendChild(mk("code", null, seen || asked));
      if (seen && asked && seen !== asked) v.appendChild(mk("span", "pmq mismatch", wsT("js.perm.native.asked", asked)));
      else v.appendChild(mk("span", "pmsrc", wsT(seen ? "js.perm.native.reported" : st.process ? "js.perm.native.next" : "js.perm.native.waiting")));
    }
    v = row("js.perm.k.evidence");
    var q = permEvidence(st);
    v.appendChild(permQ(q));
    var why = q === "pending" ? "js.perm.ev.pending" : seen && asked && seen !== asked ? "js.perm.ev.mismatch" : q === "partial" ? "js.perm.ev.partial" : !asked && !seen ? "js.perm.ev.plain" : "js.perm.ev.advisory";
    if (q === "pending" || q === "partial" || q === "advisory") v.appendChild(mk("span", "pmsrc", wsT(why, permHarness(harness))));
    if (note) box.appendChild(mk("p", "pmnote", note));
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
  // control right); changed(select) runs on a pick. With adv (the advanced
  // disclosure) inline(false) moves the native mode row there; own(mode,
  // note) shows the mode a permission level decides, the row then fixed,
  // and free() is the user's own native pick either way.
  function leadRows(box, changed, adv) {
    var R = {}, own = "", ownNote = "", free = "", extra = "", inline = true, busyNow = false, cur = { harness: "", active: "", modes: [] };
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
    R.inline = function (on) {
      if (!adv || inline === on) return;
      inline = on;
      if (on) { box.insertBefore(p.row, e.row); box.insertBefore(pd, e.row); }
      else { adv.appendChild(p.row); adv.appendChild(pd); }
    };
    R.own = function (mode, note) {
      mode = mode || "";
      note = note || "";
      if (mode === own && note === ownNote) return;
      if (mode && !own) free = p.sel.value;
      else if (!mode && own) p.sel.value = free;
      own = mode;
      ownNote = note;
      if (own) {
        if (cur.modes.indexOf(own) < 0) { p.sel.appendChild(mk("option", null, own)).value = own; p.sel.wsSig = null; }
        p.sel.value = own;
      }
      if (R.disable) R.disable(busyNow);
      R.notes(cur.harness, cur.active);
    };
    R.free = function () { return own ? free : p.sel.disabled ? "" : p.sel.value; };
    // mark adds text to the native mode's line.
    R.mark = function (text) {
      if ((text || "") === extra) return;
      extra = text || "";
      R.notes(cur.harness, cur.active);
    };
    // set shows harness's choices with v's model, effort, and mode picked.
    R.set = function (harness, v) {
      var h = supHarness(harness), models = leadModels(harness), efforts = leadEfforts(harness), modes = leadModes(harness);
      own = ownNote = extra = "";
      cur.modes = modes;
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
        busyNow = !!busy;
        m.sel.disabled = busy || unavailable || !models.length;
        e.sel.disabled = busy || unavailable || !efforts.length;
        p.sel.disabled = busy || unavailable || !modes.length || !!own;
      };
      R.disable(false);
      R.notes(harness, v.active);
    };
    // notes writes the lines under the mode and the effort; active is the
    // mode a running harness reported, named when it is not the picked one.
    R.notes = function (harness, active) {
      var claude = harness === "claude", mode = p.sel.value;
      cur.harness = harness;
      cur.active = active || "";
      pd.hidden = !mode && harness !== "pi";
      pd.textContent = own ? ownNote : (!mode ? (harness === "pi" ? wsT("js.lead.mode.pi") : "") : (claude && LEAD_MODE_NOTE[mode] ? wsT(LEAD_MODE_NOTE[mode]) : mode) + (active && active !== mode ? " " + wsT("js.lead.mode.active", active) : "")) + (extra && !own ? " " + extra : "");
      pd.classList.toggle("warn", !own && claude && mode === "bypassPermissions");
      ed.textContent = e.sel.value ? (claude ? "--effort " : harness === "pi" ? "--thinking " : "") + e.sel.value : wsT("js.lead.effort.default");
    };
    return R;
  }

  // The spawn popover for one worktree or plain folder: the target as one
  // read-only line (workspace, branch, path), the harness, the permission
  // level, and the option rows. Start spawns the session and opens its
  // page; the first message is sent from that page's composer. A refused or
  // failed spawn shows in the popover's error line and keeps it open.
  // The level starts at the default (today's behaviour: the legacy route
  // with the native mode, under Advanced, no level sent). A picked level
  // decides the native mode; Start re-reads the daemon's support, and
  // "full" first opens the warning, whose grant alone carries
  // permission_full_ack. A re-read that finds no support sends nothing and
  // says so (P.dropped): the user may then start on a native mode by hand.
  // A daemon that refuses the selection route (P.refused) is shown, and
  // Start becomes a re-check; nothing is resent on the legacy route.
  function rpLeadMake() {
    var P = { gen: 0, busy: false, root: "", harness: "claude", iso: "default", level: "", dropped: "", refused: 0 };
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
      permDraw();
    }
    function currentPick() {
      return { model: rows.model.disabled ? "" : rows.model.value, mode: rows.free(), effort: rows.effort.disabled ? "" : rows.effort.value, iso: P.iso };
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
          var k = h.harness, b = hbtn[k] = rpBtn("", permHarness(k));
          b.setAttribute("role", "radio");
          b.setAttribute("data-harness", k);
          seg.appendChild(b);
        });
      }
      if (!hbtn[P.harness] || off(P.harness)) {
        P.harness = hs.filter(function (h) { return !off(h.harness); }).map(function (h) { return h.harness; })[0] || hs[0].harness;
      }
    }
    // The permission level: its segment, what the pick asks for and how
    // this harness carries it, or why no level can be picked here.
    var psec = el.appendChild(mk("div", "spsec pmsec"));
    psec.appendChild(mk("div", "splbl", wsT("js.perm.title")));
    var pseg = permSeg(psec, function (level) {
      if (P.busy || P.level === level) return;
      P.level = level;
      P.refused = 0;
      edited();
    });
    var pwhat = psec.appendChild(mk("p", "pmwhat")), pmap = psec.appendChild(mk("p", "pmmap")), poff = psec.appendChild(mk("p", "pmoff"));
    var opts = el.appendChild(mk("div", "spopts"));
    var adv = el.appendChild(mk("details", "pmadv")), advSum = adv.appendChild(mk("summary", null, wsT("js.perm.adv")));
    var advOwn = advSum.appendChild(mk("span", "pmown", wsT("js.perm.adv.owned")));
    var rows = leadRows(opts, edited, adv);
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
    var pfine = el.appendChild(mk("p", "pmfine", wsT("js.perm.fine.intent")));
    var perr = el.appendChild(mk("p", "pmerr")), perrHd = perr.appendChild(mk("b"));
    perr.appendChild(mk("span", null, wsT("js.perm.refused.create")));
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
    // dropped is the level a re-read found unsupported (or that the daemon
    // no longer offers): it is not sent, and the form says so.
    function dropped() {
      var levels = permLevels(P.harness);
      return P.dropped || (P.level && levels && levels.indexOf(P.level) < 0 ? P.level : "");
    }
    function permDraw() {
      var levels = permLevels(P.harness), supported = !!(levels && levels.length), modes = leadModes(P.harness).length > 0;
      var gone = dropped(), level = gone ? "" : P.level;
      // A level picked before support became unknown stays visible.
      var show = !gone && (supported || (!levels && !!P.level));
      pseg.el.hidden = !show;
      if (show) pseg.set([""].concat(supported ? levels : PERM_LEVELS), level, "", P.busy || !supported);
      var offText = gone ? wsT("js.perm.off.again", wsT(PERM_LABEL[gone])) : show ? "" : !levels ? wsT("js.perm.checking") : modes ? wsT("js.perm.off.create") : wsT("js.perm.off.plain", permHarness(P.harness));
      poff.textContent = offText;
      poff.hidden = !offText;
      pwhat.textContent = show ? wsT(PERM_WHAT[level]) : "";
      pwhat.hidden = !show;
      var mapped = level && PERM_NATIVE[P.harness] ? PERM_NATIVE[P.harness][level] || "" : "";
      if (show) permMap(pmap, P.harness, level, level ? mapped : rows.free(), false);
      else pmap.hidden = true;
      var owned = show && modes ? mapped : "";
      rows.own(owned, owned ? wsT("js.perm.adv.owned.note", wsT(PERM_LABEL[level])) : "");
      advOwn.hidden = !owned;
      rows.inline(!supported || !!gone);
      adv.hidden = !(supported && !gone && modes);
      pfine.hidden = !supported;
      rows.mark(gone && modes ? wsT("js.perm.native.own") : "");
      perr.hidden = !P.refused;
      perrHd.textContent = P.refused ? wsT("js.perm.refused", P.refused) : "";
      start.textContent = wsT(P.refused ? "js.perm.recheck" : gone ? (modes ? "js.perm.start.native" : "js.perm.start.plain") : level === "full" ? "js.perm.start.full" : "js.lead.start");
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
      permDraw();
      sync();
      railPopPlace();
    }
    function setHarness(k) {
      if (P.harness === k) return;
      picks[P.harness] = currentPick();
      P.harness = k;
      P.level = P.dropped = "";
      P.refused = 0;
      showPick(k);
      edited();
    }
    function locText() {
      return [loc.wsName, loc.wsBranch, permHarness(P.harness)].filter(Boolean).join(" · ");
    }
    function submit() {
      sync();
      if (start.disabled) return;
      if (P.refused) { recheck(); return; }
      var level = dropped() ? "" : P.level;
      if (level === "full") {
        var gen = P.gen;
        permFullAsk({ harness: P.harness, path: locText() }, function () { if (rpLive(P, gen)) launch("full", true); }, function () {});
        return;
      }
      launch(level, false);
    }
    // launch spawns with level (none: the legacy route, as before); ack is
    // the full warning's grant.
    function launch(level, ack) {
      notifAsk();
      var gen = P.gen, chosenHarness = P.harness, chosen = currentPick();
      var form = { harness: P.harness, model: rows.model.disabled ? "" : rows.model.value, effort: rows.effort.disabled ? "" : rows.effort.value, mode: level ? "" : rows.mode.disabled ? "" : rows.mode.value, isolation: P.iso, level: level, ack: ack };
      P.busy = true;
      setErr("");
      status.textContent = wsT(level ? "js.perm.checking" : "js.lead.starting");
      permDraw();
      sync();
      if (!level) { post(gen, chosenHarness, chosen, "/agents/leads", form); return; }
      permPreflight(chosenHarness, level, function (ok) {
        if (!rpLive(P, gen)) { P.busy = false; return; }
        if (!ok) {
          P.busy = false;
          status.textContent = "";
          P.dropped = level;
          permDraw();
          sync();
          railPopPlace();
          return;
        }
        status.textContent = wsT("js.lead.starting");
        post(gen, chosenHarness, chosen, "/agents/leads/permission", form);
      });
    }
    function post(gen, chosenHarness, chosen, route, form) {
      var perm = !!form.level;
      railPost(route, WsRail.leadBody(P.root, form), function (st, v, s) {
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
        // A daemon without the selection route created nothing: say so and
        // offer a re-check; the legacy route is never tried instead.
        if (perm && permRefused(st)) {
          if (supChoices) supChoices.wsEpoch = -1;
          if (!rpLive(P, gen)) { toast(wsT("js.perm.refused", st)); return; }
          P.refused = st;
          permDraw();
          sync();
          railPopPlace();
          return;
        }
        if (!rpLive(P, gen)) { toast(leadErr(st, s)); return; }
        permDraw();
        setErr(leadErr(st, s));
        sync();
        railPopPlace();
        // A spawn answers at once (the process starts on the first send), so
        // a request still open after 20s fails instead of staying busy.
      }, 20000);
    }
    // recheck re-reads the daemon's support after a refusal; it sends
    // nothing. Support back clears the refusal, none drops the level.
    function recheck() {
      var gen = P.gen, level = P.level;
      P.busy = true;
      status.textContent = wsT("js.perm.checking");
      sync();
      permPreflight(P.harness, level, function (ok) {
        P.busy = false;
        status.textContent = "";
        if (!rpLive(P, gen)) return;
        P.refused = 0;
        if (!ok) P.dropped = level;
        permDraw();
        sync();
        railPopPlace();
      });
    }
    iseg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-iso]"); if (b) { P.iso = b.getAttribute("data-iso"); sync(); } });
    seg.addEventListener("click", function (e) { var b = e.target.closest && e.target.closest("[data-harness]"); if (b && !b.disabled) setHarness(b.getAttribute("data-harness")); });
    cancel.addEventListener("click", function () { railPopClose(true); });
    start.addEventListener("click", submit);
    P.open = function (key, label, btn) {
      var gen = ++P.gen, name = btn.getAttribute("data-name") || label, branch = btn.getAttribute("data-branch") || "";
      P.busy = false;
      P.root = key;
      P.level = P.dropped = "";
      P.refused = 0;
      saved = readSettings();
      picks = Object.create(null);
      if (saved.harnesses && typeof saved.harnesses === "object" && !Array.isArray(saved.harnesses)) {
        Object.keys(saved.harnesses).forEach(function (k) { picks[k] = saved.harnesses[k]; });
      }
      P.harness = "claude";
      P.iso = "default";
      P.loading = true;
      loc.textContent = "";
      loc.wsName = name;
      loc.wsBranch = branch;
      loc.appendChild(gIcon(branch ? "repo" : "wsfolder"));
      loc.appendChild(mk("span", null, name));
      if (branch) { loc.appendChild(gIcon("branch")); loc.appendChild(mk("b", null, branch)); }
      loc.appendChild(mk("code", null, key)).title = key;
      setErr("");
      status.textContent = "";
      harnessButtons();
      rows.set(P.harness, { model: "", effort: "", mode: "" });
      permDraw();
      sync();
      leadChoices(function () {
        if (!rpLive(P, gen)) return;
        P.loading = false;
        P.harness = typeof saved.harness === "string" ? saved.harness : "claude";
        harnessButtons();
        showPick(P.harness);
        sync();
        railPopPlace();
      }, function () {
        // The newer list arrived over a form the user may have edited: keep
        // the picks, and show them against the new lists.
        if (!rpLive(P, gen)) return;
        picks[P.harness] = currentPick();
        harnessButtons();
        showPick(P.harness);
        sync();
        railPopPlace();
      });
    };
    // permSync redraws the level after the live stream dropped (support is
    // unknown until a new read) and re-reads the choices once it is back.
    P.permSync = function (read) {
      var gen = P.gen, redraw = function () { if (rpLive(P, gen) && !P.loading) { permDraw(); sync(); railPopPlace(); } };
      redraw();
      if (read) leadChoices(redraw, redraw);
    };
    P.focus = function () { (hbtn[P.harness].disabled ? cancel : hbtn[P.harness]).focus(); };
    P.close = function () { permFullDrop(); };
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
  var leadActions = new Set();
  function rpSessMake(inRow) {
    var P = { gen: 0, busy: false, rid: rid, st: null };
    var el = P.el = inRow ? mk("div", "rmsess") : rpPop(wsT("js.lead.menu"));
    if (inRow) {
      el.hidden = true;
      el.setAttribute("role", "group");
      el.setAttribute("aria-label", wsT("js.lead.menu"));
    }
    el.appendChild(mk("div", "sphd", wsT("js.lead.menu")));
    // The session's permission state, then (a lead on a daemon that offers
    // levels for its harness) the level to change it to: a pick re-reads
    // the support and posts to settings/permission at once; "full" first
    // opens the warning, and its cancel leaves the session as it was.
    var pst = el.appendChild(mk("div", "pmstate"));
    var pchg = el.appendChild(mk("div", "spsec pmchg"));
    pchg.appendChild(mk("div", "splbl", wsT("js.perm.title")));
    var pseg = permSeg(pchg, pickLevel);
    var pwhat = pchg.appendChild(mk("p", "pmwhat")), pmap = pchg.appendChild(mk("p", "pmmap")), pfine = pchg.appendChild(mk("p", "pmfine"));
    var opts = el.appendChild(mk("div", "spopts"));
    var adv = el.appendChild(mk("details", "pmadv")), advSum = adv.appendChild(mk("summary", null, wsT("js.perm.adv")));
    var advOwn = advSum.appendChild(mk("span", "pmown", wsT("js.perm.adv.owned")));
    adv.open = true;
    var rows = leadRows(opts, function (sel) {
      var body = {};
      if (sel === rows.model) body.model = sel.value;
      else if (sel === rows.effort) body.effort = sel.value;
      else body.permission_mode = sel.value;
      act("settings", body);
    }, adv);
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
      var target = P.rid, gen = P.gen;
      if (P.busy || leadActions.has(target)) return;
      leadActions.add(target);
      P.busy = true;
      failed = "";
      P.draw();
      railPost("/agents/" + encodeURIComponent(target) + "/" + action, body, function (st, v, s) {
        leadActions.delete(target);
        if (P.rid === target) P.busy = false;
        if (st === 200 && v.id) {
          if (inRow && gen === P.gen) P.st = v;
          if (rid === target && (hostedPage || handedPage)) lead.st = v;
          P.redraw();
          return;
        }
        var msg = action === "hide" || action === "unhide" ? archErr(action, st, s) : leadErr(st, s);
        // A daemon without the selection route changed nothing; its support
        // is read again, and nothing is resent on the legacy route.
        var refused = action === "settings/permission" && permRefused(st);
        if (refused) {
          msg = wsT("js.perm.refused", st) + " " + wsT("js.perm.refused.settings");
          if (supChoices) supChoices.wsEpoch = -1;
        }
        if (P.live(gen)) failed = msg;
        else toast(msg);
        P.redraw();
        P.fetch();
        if (refused) P.permSync(true);
      });
    }
    // pickLevel changes the session's level after a fresh support read.
    function pickLevel(level) {
      var st = state();
      if (P.busy || !st || (st.permission_source === "explicit" && st.permission_level === level)) return;
      var harness = String(st.backend || ""), gen = P.gen;
      function send(ack) {
        if (!P.live(gen)) return;
        P.busy = true;
        failed = "";
        P.draw();
        permPreflight(harness, level, function (ok) {
          P.busy = false;
          if (!ok) {
            if (P.live(gen)) failed = wsT("js.perm.unsent");
            else toast(wsT("js.perm.unsent"));
            P.redraw();
            return;
          }
          if (!P.live(gen)) return;
          var body = { permission_level: level };
          if (ack) body.permission_full_ack = true;
          act("settings/permission", body);
        });
      }
      if (level !== "full") { send(false); return; }
      permFullAsk({ harness: harness, change: true, path: [String(st.label || P.rid || ""), permHarness(harness)].filter(Boolean).join(" · ") }, function () { send(true); }, function () { if (P.live(gen)) P.draw(); });
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
    // The menu closes before asking, so return focus to the menu's opener.
    async function confirmAction(action, button, message, n) {
      var target = P.rid, opener = railPopBtn() || button, page = rid, st = state();
      if (button.disabled || leadGone(st) || !st.can_control || leadActions.has(target) || (!inRow && target !== page)) return;
      railPopClose(false);
      var gen = P.gen;
      if (message) {
        if (!(await window.wsConfirm(message, { opener: opener }))) return;
        st = state();
        if (P.gen !== gen || P.rid !== target || rid !== page || !opener.isConnected ||
            leadGone(st) || !st.can_control || bgTasks() > n || (action === "stop" && !st.process) ||
            (action === "close" && st.is_child)) return;
      }
      act(action, {});
    }
    stop.b.addEventListener("click", function () {
      var n = bgTasks();
      confirmAction("stop", stop.b, n > 0 ? wsTn("js.lead.stop.confirm", n) : "", n);
    });
    archive.b.addEventListener("click", function () {
      var st = state();
      railPopClose(false);
      act(st && st.hidden ? "unhide" : "hide", {});
    });
    restart.b.addEventListener("click", function () {
      var n = bgTasks();
      confirmAction("restart", restart.b, n > 0 ? wsTn("js.lead.restart.confirm", n) : "", n);
    });
    handoff.b.addEventListener("click", function () {
      var n = bgTasks();
      confirmAction("handoff", handoff.b, n > 0 ? wsTn("js.lead.handoff.confirm", n) : "", n);
    });
    close.b.addEventListener("click", function () {
      var st = state(), n = bgTasks(), target = P.rid;
      if (st && st.is_child) {
        railPopClose(false);
        var phase = st.approvals && st.approvals.length ? "permission" : st.holding ? "holding" : st.status === "running" ? "thinking" : "idle";
        subagentClose(target, st.label || target, phase, close.b);
      } else confirmAction("close", close.b, n > 0 ? wsTn("js.lead.close.confirm.bg", n) : wsT("js.lead.close.confirm"), n);
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
      permDraw(st);
      rows.disable(P.busy);
      stop.b.disabled = P.busy || !st.process;
      stop.note.textContent = wsT(st.process ? "js.lead.stop.note" : "js.lead.stopped");
      var broken = Array.isArray(st.mcp_broken) ? st.mcp_broken.map(String) : [];
      restart.b.hidden = !broken.length;
      restart.b.disabled = P.busy;
      handoff.b.hidden = !!st.is_child;
      archive.b.hidden = !!st.is_child;
      handoff.b.disabled = P.busy;
      archive.b.disabled = P.busy;
      if (archive.b.getAttribute("data-hidden") !== String(!!st.hidden)) {
        archive.b.setAttribute("data-hidden", String(!!st.hidden));
        archive.b.replaceChild(gIcon(st.hidden ? "archive-restore" : "archive"), archive.b.firstChild);
        archT.textContent = wsT(st.hidden ? "js.lead.unarchive" : "js.lead.archive");
        archive.note.textContent = wsT(st.hidden ? "js.lead.unarchive.note" : "js.lead.archive.note");
      }
      restart.note.textContent = broken.length ? wsT("js.lead.restart.note", broken.join(", ")) : "";
      close.b.disabled = P.busy || subagentClosing[P.rid] === "closed";
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
      var redraw = function () { if (P.live(gen)) { P.draw(); railPopPlace(); } };
      leadChoices(redraw, redraw);
      P.fetch();
    };
    // permDraw shows st's permission state and, for a lead whose harness
    // the current daemon offers levels for, the change segment. A worker
    // keeps its state line alone: no worker selection support is offered.
    function permDraw(st) {
      var harness = String(st.backend || ""), lead = !!st.lead && !st.is_child, levels = lead ? permLevels(harness) : [];
      var supported = !!(levels && levels.length), level = String(st.permission_level || ""), explicit = st.permission_source === "explicit";
      permState(pst, st, lead && levels && !levels.length && level ? wsT("js.perm.off.settings") : "");
      pchg.hidden = !supported;
      var cur = explicit ? level : "", modes = leadModes(harness).length > 0;
      if (supported) {
        pseg.set(levels, cur, explicit ? "" : level, P.busy);
        pwhat.textContent = cur ? wsT(cur === "full" ? "js.perm.what.full.set" : PERM_WHAT[cur]) : "";
        pwhat.hidden = !cur;
        if (cur) permMap(pmap, harness, cur, String(st.permission_mode || ""), true);
        else pmap.hidden = true;
        pfine.textContent = wsT(cur ? "js.perm.fine.explicit" : "js.perm.fine.derived");
      }
      var owned = supported && cur && modes ? String(st.permission_mode || "") : "";
      rows.own(owned, owned ? wsT("js.perm.adv.owned.note", wsT(PERM_LABEL[cur])) : "");
      advOwn.hidden = !owned;
      rows.inline(!supported);
      adv.hidden = !(supported && modes);
    }
    // permSync redraws after the live stream dropped or a refusal (support
    // unknown) and re-reads the choices when read is set.
    P.permSync = function (read) {
      var gen = P.gen, redraw = function () { if (P.live(gen)) { P.draw(); railPopPlace(); } };
      redraw();
      if (read) leadChoices(redraw, redraw);
    };
    P.focus = function () { rows.model.focus(); };
    P.close = function () {
      permFullDrop();
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
      if (status === 200 && v.id) { lead.st = v; lead.gone = false; }
      else if (status === 404) { lead.st = null; lead.gone = true; }
      if (status === 200 || status === 404) leadDraw();
      if (lead.again) { lead.again = false; leadFetch(); }
    });
  }
  // leadChip makes the head's harness chip the session menu's button for a
  // viewer who holds the controls. The head region is replaced on every
  // update, so this runs again after each.
  function leadChip() {
    permChip();
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
  // permChip keeps the head's permission chip beside the harness chip: the
  // session's level (tagged when derived from older settings) and its
  // evidence, never the native mode. For a viewer who holds the controls
  // it opens the session menu, as the harness chip does.
  function permChip() {
    var h = region("head"), host = h && h.querySelector(".host.sse"), st = lead.st;
    if (!host) return;
    var chip = h.querySelector(".pmhead"), level = st && !leadGone(st) ? String(st.permission_level || "") : "";
    if (!level) { if (chip) chip.remove(); return; }
    if (!chip) {
      chip = mk("span", "chip pmhead");
      chip.addEventListener("click", function () {
        var c = region("head"), hb = c && c.querySelector(".host.hbtn");
        if (hb && leadCan() && (railEl || framed) && WsRail) railPopOpen("sess", hb);
      });
      chip.addEventListener("keydown", function (e) {
        if ((e.key === "Enter" || e.key === " ") && chip.getAttribute("role") === "button") { e.preventDefault(); chip.click(); }
      });
    }
    if (chip.previousSibling !== host) host.parentNode.insertBefore(chip, host.nextSibling);
    var derived = st.permission_source !== "explicit", q = permEvidence(st), sig = [level, derived, q].join("\n");
    if (chip.wsSig !== sig) {
      chip.wsSig = sig;
      chip.textContent = "";
      chip.appendChild(gIcon("lock"));
      chip.appendChild(mk("span", null, PERM_LABEL[level] ? wsT(PERM_LABEL[level]) : level));
      if (derived) chip.appendChild(mk("span", "pmtag", wsT("js.perm.tag.derived")));
      chip.appendChild(permQ(q));
      chip.title = wsT("js.perm.title");
    }
    var can = leadCan();
    if (can) { chip.setAttribute("role", "button"); chip.setAttribute("tabindex", "0"); }
    else { chip.removeAttribute("role"); chip.removeAttribute("tabindex"); }
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
    composerSync(st);
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
    if (name === "head") {
      document.querySelectorAll("[data-child-close]").forEach(function (b) {
        if (subagentClosing[b.getAttribute("data-reply-id")] === "closed") markChildCloseButton(b);
      });
    }
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

  // --- composer footer 2-control strip (Model + Effort only, decision (38)) --
  var composerPills = $("composer-pills"), composerPop = { slot: "", portal: null, busy: false };
  function composerHarness() {
    if (lead.st && lead.st.backend) return String(lead.st.backend);
    var h = region("head"), el = h && h.querySelector(".host.sse");
    return el ? el.textContent.trim() : "";
  }
  function composerHarnessLabel(k) {
    if (k === "claude") return "Claude";
    if (k === "codex") return "Codex";
    if (k === "agy") return "AGY";
    if (k === "pi") return "Pi";
    return k || "";
  }
  function composerCurrent(slot) {
    if (lead.st) return String(slot === "model" ? (lead.st.model || "") : (lead.st.effort || ""));
    var btn = composerPills && composerPills.querySelector('[data-pop="' + slot + '"]');
    if (!btn) return "";
    var v = btn.getAttribute("data-val");
    if (v !== null) return v;
    var pv = btn.querySelector(".pv"), txt = pv ? pv.textContent.trim() : "";
    return txt === wsT("js.sup.default") ? "" : txt;
  }
  function composerEnsurePortal() {
    if (composerPop.portal) return composerPop.portal;
    var el = mk("div", "fpop");
    el.id = "fpop-portal";
    el.setAttribute("role", "listbox");
    el.hidden = true;
    doc.body.appendChild(el);
    composerPop.portal = el;
    return el;
  }
  function composerPlacePop() {
    var pop = composerPop.portal;
    if (!pop || pop.hidden || !composerPop.slot || !composerPills) return;
    var btn = composerPills.querySelector('[data-pop="' + composerPop.slot + '"]');
    if (!btn) return;
    var r = btn.getBoundingClientRect(), pw = pop.offsetWidth, ph = pop.offsetHeight;
    var left = Math.max(8, Math.min(r.left, window.innerWidth - pw - 8));
    var top = r.top - ph - 6;
    if (top < 8) top = Math.min(window.innerHeight - ph - 8, r.bottom + 6);
    pop.style.left = left + "px";
    pop.style.top = Math.max(8, top) + "px";
  }
  function composerClosePop(restoreFocus) {
    var was = composerPop.slot;
    composerPop.slot = "";
    if (composerPills) q("[data-pop]", composerPills).forEach(function (b) { b.setAttribute("aria-expanded", "false"); });
    if (composerPop.portal) { composerPop.portal.hidden = true; composerPop.portal.removeAttribute("data-sig"); composerPop.portal.textContent = ""; }
    if (restoreFocus && was && composerPills) {
      var btn = composerPills.querySelector('[data-pop="' + was + '"]');
      if (btn && !btn.disabled) btn.focus();
    }
  }
  function composerDrawPop() {
    var slot = composerPop.slot;
    if (!slot || !composerPills) return;
    var pop = composerEnsurePortal(), harness = composerHarness(), cur = composerCurrent(slot);
    var list = slot === "model" ? leadModels(harness) : leadEfforts(harness);
    var all = list.slice();
    if (cur && all.indexOf(cur) < 0) all.push(cur);
    var sig = [slot, harness, cur].concat(all).join("\n");
    if (!pop.hidden && pop.getAttribute("data-sig") === sig) {
      composerPlacePop();
      return;
    }
    pop.setAttribute("data-sig", sig);
    pop.textContent = "";
    var hdText = (composerHarnessLabel(harness) ? composerHarnessLabel(harness) + " " : "") + wsT(slot === "model" ? "js.sup.model" : "js.sup.effort");
    pop.appendChild(mk("div", "fpop-hd", hdText));
    var opts = [{ val: "", label: wsT("js.sup.default") }].concat(all.map(function (v) { return { val: v, label: v }; }));
    opts.forEach(function (o) {
      var opt = mk("button", "fpop-opt");
      opt.type = "button";
      opt.setAttribute("role", "option");
      opt.setAttribute("data-slot", slot);
      opt.setAttribute("data-val", o.val);
      opt.setAttribute("aria-selected", String(o.val === cur));
      opt.appendChild(mk("span", null, o.label));
      pop.appendChild(opt);
    });
    pop.hidden = false;
    composerPlacePop();
  }
  function composerPick(slot, val) {
    if (composerPop.busy) return;
    var body = {};
    body[slot] = val;
    composerPop.busy = true;
    composerClosePop(true);
    composerSync(lead.st);
    railPost("/agents/" + encodeURIComponent(rid) + "/settings", body, function (st, v, s) {
      composerPop.busy = false;
      if (st === 200 && v.id) { lead.st = v; lead.gone = false; leadDraw(); return; }
      toast(leadErr(st, s));
      leadDraw();
      leadFetch();
    });
  }
  function composerSync(st) {
    if (!composerPills) return;
    if ((st && leadGone(st)) || (!st && lead.gone)) {
      composerPills.hidden = true;
      composerClosePop(false);
      return;
    }
    composerPills.hidden = false;
    var isChild = !!(root.hasAttribute("data-child") || (st && st.is_child));
    var canEdit = !isChild && !composerPop.busy && (st ? !!st.can_control : root.hasAttribute("data-can-edit-settings"));
    [["model", "js.lead.controls.model"], ["effort", "js.lead.controls.effort"]].forEach(function (pair) {
      var slot = pair[0], tipKey = pair[1], btn = composerPills.querySelector('[data-pop="' + slot + '"]');
      if (!btn) return;
      if (st) {
        var raw = String(slot === "model" ? (st.model || "") : (st.effort || ""));
        btn.setAttribute("data-val", raw);
        var pv = btn.querySelector(".pv");
        if (pv) pv.textContent = raw || wsT("js.sup.default");
      }
      btn.disabled = !canEdit;
      btn.title = isChild ? wsT("js.lead.controls.child") : (!canEdit && !(st ? false : composerPop.busy) ? wsT("js.lead.controls.host") : wsT(tipKey));
      if (!canEdit && composerPop.busy && !isChild && (st ? !!st.can_control : root.hasAttribute("data-can-edit-settings"))) btn.title = wsT(tipKey);
      var tail = btn.lastElementChild, wantLock = isChild;
      if (tail && tail.classList.contains("lock") !== wantLock) {
        var nextTail = luIcon(wantLock ? "lock" : "chevron-down");
        nextTail.setAttribute("class", "i li " + (wantLock ? "lock" : "chev"));
        btn.replaceChild(nextTail, tail);
      }
    });
    if (composerPop.slot) {
      if (!canEdit) composerClosePop(false);
      else composerDrawPop();
    }
  }
  if (composerPills) {
    composerPills.addEventListener("click", function (e) {
      var btn = e.target && e.target.closest && e.target.closest("[data-pop]");
      if (!btn || btn.disabled) return;
      var slot = btn.getAttribute("data-pop");
      if (composerPop.slot === slot) { composerClosePop(true); return; }
      composerClosePop(false);
      composerPop.slot = slot;
      btn.setAttribute("aria-expanded", "true");
      composerDrawPop();
      leadChoices(composerDrawPop, composerDrawPop);
    });
    doc.addEventListener("click", function (e) {
      if (!composerPop.slot) return;
      var t = e.target;
      if (t && t.closest) {
        var opt = t.closest("#fpop-portal .fpop-opt");
        if (opt) { composerPick(opt.getAttribute("data-slot"), opt.getAttribute("data-val")); return; }
        if (t.closest("#fpop-portal") || t.closest("#composer-pills")) return;
      }
      composerClosePop(false);
    });
    doc.addEventListener("keydown", function (e) {
      if (!composerPop.slot) return;
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); composerClosePop(true); return; }
      if ((e.key === "ArrowDown" || e.key === "ArrowUp") && composerPop.portal) {
        var opts = q(".fpop-opt", composerPop.portal);
        if (!opts.length) return;
        e.preventDefault();
        var idx = opts.indexOf(doc.activeElement);
        var next = e.key === "ArrowDown" ? (idx + 1) % opts.length : (idx <= 0 ? opts.length - 1 : idx - 1);
        opts[next].focus();
      }
    });
    window.addEventListener("resize", composerPlacePop);
  }

