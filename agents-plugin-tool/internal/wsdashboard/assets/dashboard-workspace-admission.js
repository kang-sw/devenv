  // Workspace drafts are independent of worktree evidence. A missing row may
  // return on a later rail render, so only rendered rows are actionable.
  var WS_ADMISSION_KEY = "ws.dashboard.workspace-admission.drafts";
  var wsAdmissionDraft = [];
  try {
    var wsSaved = JSON.parse(load(WS_ADMISSION_KEY) || "[]");
    if (Array.isArray(wsSaved)) wsAdmissionDraft = wsSaved.filter(function (d) {
      return d && typeof d.path === "string" && (d.action === "approve" || d.action === "reject");
    });
  } catch (_) {}
  var wsAdmissionBusy = false, wsUnregFilter = "", wsUnregScroll = 0;
  var wsUnregExpanded = new Set(), wsUnregFocus = null;

  function wsAdmissionSave() {
    save(WS_ADMISSION_KEY, wsAdmissionDraft.length ? JSON.stringify(wsAdmissionDraft) : null);
    admissionSync();
  }
  function wsAdmissionClear() { wsAdmissionDraft = []; save(WS_ADMISSION_KEY, null); }
  function wsAdmissionSelections() {
    var r = region("rail");
    if (!r) return [];
    var paths = new Set(q(".unreg-row[data-unreg-path]", r).map(function (row) { return row.getAttribute("data-unreg-path"); }));
    return wsAdmissionDraft.filter(function (d) { return paths.has(d.path); });
  }
  function wsAdmissionCapture() {
    var r = region("rail"), list = r && r.querySelector(".unreg-list");
    if (list) wsUnregScroll = list.scrollTop;
    var input = r && r.querySelector("#unreg-filter");
    wsUnregFocus = input && doc.activeElement === input ? [input.selectionStart, input.selectionEnd] : null;
    if (input) wsUnregFilter = input.value;
  }
  function wsAdmissionFilter(r) {
    var input = r.querySelector("#unreg-filter"), needle = wsUnregFilter.trim().toLowerCase(), visible = 0;
    q(".unreg-row[data-unreg-path]", r).forEach(function (row) {
      var info = row.querySelector(".unreg-info");
      row.hidden = !!needle && !((info && info.textContent || "") + " " + row.getAttribute("data-unreg-path")).toLowerCase().includes(needle);
      if (!row.hidden) visible++;
    });
    var empty = r.querySelector(".unreg-filter-empty");
    if (empty) empty.hidden = !needle || visible > 0;
    var all = r.querySelector("#unreg-reject-all");
    if (all) {
      var shown = q(".unreg-row[data-unreg-path]", r).filter(function (row) { return !row.hidden; });
      var rejected = shown.length > 0 && shown.every(function (row) {
        return wsAdmissionDraft.some(function (d) { return d.path === row.getAttribute("data-unreg-path") && d.action === "reject"; });
      });
      all.disabled = shown.length === 0;
      all.setAttribute("aria-pressed", String(rejected));
      all.textContent = all.getAttribute(rejected ? "data-clear-label" : "data-reject-label");
    }
    if (input && input.value !== wsUnregFilter) input.value = wsUnregFilter;
  }
  function wsAdmissionSync() {
    var r = region("rail"), counts = { approve: 0, reject: 0 };
    if (!r) return counts;
    q(".unreg-row[data-unreg-path]", r).forEach(function (row) {
      var path = row.getAttribute("data-unreg-path");
      var draft = wsAdmissionDraft.find(function (d) { return d.path === path; });
      if (draft) counts[draft.action]++;
      row.classList.toggle("m-ap", !!draft && draft.action === "approve");
      row.classList.toggle("m-rj", !!draft && draft.action === "reject");
      q("[data-ws-admission-action]", row).forEach(function (b) {
        b.setAttribute("aria-pressed", String(!!draft && draft.action === b.getAttribute("data-ws-admission-action")));
        b.disabled = wsAdmissionBusy;
      });
      var pill = row.querySelector(".unreg-scount"), sessions = row.querySelector(".unreg-sessions");
      var expanded = wsUnregExpanded.has(path);
      if (pill) pill.setAttribute("aria-expanded", String(expanded));
      if (sessions) sessions.hidden = !expanded;
      q(".unreg-slink", row).forEach(function (link) {
        var current = link.getAttribute("href") === "/s/" + rid;
        link.classList.toggle("cur", current);
        if (current) link.setAttribute("aria-current", "page"); else link.removeAttribute("aria-current");
      });
    });
    var details = r.querySelector(".rws-unreg"), badge = details && details.querySelector(".unreg-draft-pill");
    if (badge) {
      badge.hidden = !details || details.open || counts.approve + counts.reject === 0;
      badge.textContent = wsT("js.ws.admission.drafts", counts.approve + counts.reject);
    }
    wsAdmissionFilter(r);
    var list = r.querySelector(".unreg-list");
    if (list) list.scrollTop = wsUnregScroll;
    if (wsUnregFocus) {
      var input = r.querySelector("#unreg-filter");
      if (input) {
        input.focus({ preventScroll: true });
        input.setSelectionRange(wsUnregFocus[0], wsUnregFocus[1]);
      }
      wsUnregFocus = null;
    }
    return counts;
  }
  function wsAdmissionToast(message, ok) {
    if (!railEl) return;
    if (!rtoastEl) {
      rtoastEl = mk("div", "rtoast");
      rtoastEl.setAttribute("role", "status");
      rtoastEl.setAttribute("aria-live", "polite");
    }
    if (rtoastEl.parentNode !== railEl) railEl.appendChild(rtoastEl);
    rtoastEl.textContent = message;
    rtoastEl.className = "rtoast" + (ok ? " ok" : "");
    var r = region("rail"), anchor = r && (r.querySelector(".rcap:not([hidden])") || r.querySelector(".rfoot"));
    if (anchor) rtoastEl.style.bottom = Math.round(railEl.getBoundingClientRect().bottom - anchor.getBoundingClientRect().top + 8) + "px";
    void rtoastEl.offsetWidth;
    rtoastEl.classList.add("on");
    clearTimeout(rtoastTimer);
    rtoastTimer = setTimeout(function () { rtoastEl.classList.remove("on"); }, 5000);
  }
  function wsAdmissionPost(selections, quietFailure) {
    if (wsAdmissionBusy || !selections.length) return Promise.resolve(false);
    wsAdmissionBusy = true; railSaving++; admissionSync();
    var ctl = window.AbortController ? new AbortController() : null;
    var timer = setTimeout(function () { if (ctl) ctl.abort(); }, LAYOUT_WAIT);
    return fetch("/rail/workspaces/admission", {
      method: "POST", credentials: "same-origin", cache: "no-store",
      signal: ctl ? ctl.signal : undefined,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ selections: selections })
    }).then(function (res) {
      return res.json().then(function (value) { return { status: res.status, value: value }; });
    }).then(function (answer) {
      var value = answer.value, applied = answer.status === 200 && value.applied === true;
      if (applied) {
        wsAdmissionDraft = wsAdmissionDraft.filter(function (d) {
          return !selections.some(function (s) { return s.path === d.path && s.action === d.action; });
        });
        save(WS_ADMISSION_KEY, wsAdmissionDraft.length ? JSON.stringify(wsAdmissionDraft) : null);
        railMinGen = Math.max(railMinGen, +value.gen || 0);
      }
      clearTimeout(timer); wsAdmissionBusy = false; railSaving--;
      if (applied && value.html) swap("rail", "workspace-admission:" + value.gen + ":" + Date.now(), value.html);
      admissionSync(); railRelease();
      if (!applied && !quietFailure) wsAdmissionToast(wsT(answer.status === 409 ? "js.ws.admission.stale" : "js.ws.admission.failed"), false);
      return applied;
    }).catch(function () {
      clearTimeout(timer);
      if (wsAdmissionBusy) { wsAdmissionBusy = false; railSaving--; admissionSync(); railRelease(); }
      if (!quietFailure) wsAdmissionToast(wsT("js.ws.admission.failed"), false);
      return false;
    });
  }
  function wsAdmissionApply(worktrees) {
    var selections = wsAdmissionSelections();
    if (!selections.length) return;
    var approve = selections.filter(function (s) { return s.action === "approve"; }).length;
    var reject = selections.length - approve;
    wsAdmissionPost(selections, worktrees.length > 0).then(function (applied) {
      if (!applied) {
        if (worktrees.length) wsAdmissionToast(wsT("js.ws.admission.toast.none", wsT("js.ws.admission.group.workspaces"), wsT("js.ws.admission.group.worktrees")), false);
        return;
      }
      if (worktrees.length) {
        admissionSubmit(worktrees, function (ok) {
          wsAdmissionToast(wsT(ok ? "js.ws.admission.toast.both" : "js.ws.admission.toast.partial",
            wsT("js.ws.admission.group.workspaces"), wsT("js.ws.admission.group.worktrees")), ok);
        });
      } else wsAdmissionToast(approve && reject ? wsT("js.ws.admission.toast.applied", approve, reject) :
        approve ? wsT("js.ws.admission.toast.approved", approve) : wsT("js.ws.admission.toast.rejected", reject), true);
    });
  }
  function wsAdmissionClick(target) {
    var b = target.closest && target.closest("[data-ws-admission-action], [data-ws-undo], [data-tog-sess], #unreg-reject-all");
    if (!b) return false;
    if (wsAdmissionBusy) return true;
    if (b.hasAttribute("data-tog-sess")) {
      var path = b.getAttribute("data-tog-sess");
      if (wsUnregExpanded.has(path)) wsUnregExpanded.delete(path); else wsUnregExpanded.add(path);
      admissionSync();
    } else if (b.id === "unreg-reject-all") {
      var rows = q(".unreg-row[data-unreg-path]", region("rail")).filter(function (row) { return !row.hidden; });
      var allRejected = rows.length > 0 && rows.every(function (row) {
        return wsAdmissionDraft.some(function (d) { return d.path === row.getAttribute("data-unreg-path") && d.action === "reject"; });
      });
      rows.forEach(function (row) {
        var path = row.getAttribute("data-unreg-path");
        wsAdmissionDraft = wsAdmissionDraft.filter(function (d) { return d.path !== path; });
        if (!allRejected) wsAdmissionDraft.push({ path: path, action: "reject" });
      });
      wsAdmissionSave();
    } else if (b.hasAttribute("data-ws-undo")) {
      wsAdmissionPost([{ path: b.getAttribute("data-ws-undo"), action: "undo" }]).then(function (ok) {
        if (ok) wsAdmissionToast(wsT("js.ws.admission.toast.undone"), true);
      });
    } else {
      var row = b.closest(".unreg-row"), rowPath = row.getAttribute("data-unreg-path");
      var action = b.getAttribute("data-ws-admission-action");
      var old = wsAdmissionDraft.find(function (d) { return d.path === rowPath; });
      wsAdmissionDraft = wsAdmissionDraft.filter(function (d) { return d.path !== rowPath; });
      if (!old || old.action !== action) wsAdmissionDraft.push({ path: rowPath, action: action });
      wsAdmissionSave();
    }
    return true;
  }
  if (railEl) {
    railEl.addEventListener("input", function (e) {
      if (e.target.id !== "unreg-filter") return;
      wsUnregFilter = e.target.value;
      wsUnregFocus = [e.target.selectionStart, e.target.selectionEnd];
      wsAdmissionFilter(region("rail"));
    });
    railEl.addEventListener("scroll", function (e) {
      if (e.target.classList && e.target.classList.contains("unreg-list")) wsUnregScroll = e.target.scrollTop;
    }, true);
    railEl.addEventListener("toggle", function (e) {
      if (e.target.matches && e.target.matches("details.rws-unreg")) admissionSync();
    }, true);
    admissionSync();
  }
