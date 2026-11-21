// Composer interrupt, agent, and task status controls.
  // --- interrupt badge ---------------------------------------------------------
  // The badge on the border between the conversation and the composer shows
  // while the session has a live adapter and its lead works on a turn (the
  // convlive region's data-phase). One click stops the turn and takes every
  // held message back into the composer. It then stays disabled until that
  // turn ends, so a second click cannot cut the next turn: until the phase
  // leaves the working set or the region's data-turn (the server's
  // turn-start stamp) changes from the one seen at the click. The stamp is
  // what catches a next turn that starts at once (held input or mail
  // delivered at turn end), which no page swap ever shows as idle.
  var ibadge = $("ibadge"), ilabel = ibadge && ibadge.querySelector(".ilabel");
  var WORKING = { thinking: 1, replying: 1, tool: 1, permission: 1 };
  var liveMode = mode === "live", interrupting = false, interruptedTurn = "";
  function leadWorking() {
    var c = doc.querySelector("#r-convlive .clive");
    return !!c && WORKING[c.getAttribute("data-phase")] === 1;
  }
  function leadTurn() {
    var c = doc.querySelector("#r-convlive .clive");
    return (c && c.getAttribute("data-turn")) || "";
  }
  function syncBadge() {
    // The session page's badge opens the agents island's full view, so it shows there
    // whether or not the session is live and anything runs.
    var popover = !!$("agpop");
    subBadge(sbadge, popover ? doc.querySelector("#r-tree .ag-source") : doc.querySelector("#r-convlive .clive"), liveMode || popover, popover);
    bgBadge(doc.querySelector("#r-convlive .clive"));
    if (!ibadge) return;
    var on = liveMode && leadWorking();
    if (!on || leadTurn() !== interruptedTurn) interrupting = false;
    ibadge.hidden = !on;
    ibadge.disabled = interrupting;
    ibadge.classList.toggle("busy", interrupting);
    var label = wsT(interrupting ? "js.interrupt.busy" : "js.interrupt");
    if (ilabel && ilabel.textContent !== label) ilabel.textContent = label;
  }
  // --- subagent badge --------------------------------------------------------
  // Beside the interrupt badge: a spinner, the robot icon, and "N(+M)"
  // (the word "subagents" is the title and accessible name) while the session runs subagents, whether or not its lead
  // works. N is data-sub (its running direct children), M data-subdeep
  // (every running agent below them), counted by the
  // server. The session reads the tree source; tracker chat reads convlive.
  // "(+M)" shows only when M is above zero. on gates it: the
  // session page's live mode, or the support chat card's open conversation.
  // stay keeps it shown, quiet (idle), while nothing runs.
  var sbadge = $("sbadge");
  function subBadge(badge, clive, on, stay) {
    if (!badge) return;
    var n = clive ? Number(clive.getAttribute("data-sub")) || 0 : 0;
    var m = clive ? Number(clive.getAttribute("data-subdeep")) || 0 : 0;
    badge.hidden = !(on && (stay || n + m > 0));
    if (badge.hidden) return;
    badge.classList.toggle("idle", n + m === 0);
    var count = m > 0 ? n + "(+" + m + ")" : String(n);
    var label = wsTn("js.subagents", n + m, count);
    var el = badge.querySelector(".slabel");
    if (el && el.textContent !== count) el.textContent = count;
    // The word moves to the accessible name and tooltip, like the clock badge.
    if (badge.getAttribute("aria-label") !== label) {
      if (badge.dataset.tip === undefined) badge.dataset.tip = badge.title;
      badge.setAttribute("aria-label", label);
      badge.title = label + (badge.dataset.tip ? " · " + badge.dataset.tip : "");
    }
  }
  // --- background-task badge ---------------------------------------------------
  // Separate harness-owned background and monitor pills follow subagents.
  // The live region carries counts for terminal and hosted sessions alike;
  // a missing count or a zero leaves its pill hidden.
  var bgbadge = $("bgbadge"), bgTitle = bgbadge ? bgbadge.title : "";
  var monbadge = $("monbadge"), monTitle = monbadge ? monbadge.title : "";
  function taskBadge(badge, clive, attr, title) {
    if (!badge) return;
    var n = clive ? Number(clive.getAttribute(attr)) || 0 : 0;
    badge.hidden = !(liveMode && n > 0);
    if (badge.hidden) return;
    var label = attr === "data-bg" ? wsTn("js.bgtasks", n) : wsTn("js.monitors", n);
    var el = badge.querySelector(".blabel");
    if (el && el.textContent !== label) el.textContent = label;
    if (badge.getAttribute("aria-label") !== label) {
      badge.setAttribute("aria-label", label);
      badge.title = label + (title ? " \u00b7 " + title : "");
    }
  }
  function bgBadge(clive) {
    taskBadge(bgbadge, clive, "data-bg", bgTitle);
    taskBadge(monbadge, clive, "data-monitors", monTitle);
  }
  // --- agents island ---------------------------------------------------------
  // The streamed tree is authoritative; the compact HUD and full popover
  // project from it without moving tree nodes, preserving disclosure state.
  var agpop = $("agpop"), agpin = $("agpin"), agisland = $("agisland"), agcompact = $("agcompact"), agidle = $("agidle");
  var agExpand = agcompact ? agcompact.querySelector(".hud-expand-btn") : null;
  var AGPIN_KEY = "ws.dashboard.agpop.pin", agFull = {}, agRunning = 0, agSeen = false, agDismissed = false;
  function agSize() {
    if (!agisland || agisland.hidden) return;
    var mode = agisland.getAttribute("data-mode"), view = mode === "full" ? agpop : mode === "compact" ? agcompact : agidle;
    // Measure at the destination width without painting the temporary auto
    // height. Restore the current interpolated box before setting pixel targets,
    // so both dimensions can transition even during a streaming update.
    var box = agisland.getBoundingClientRect(), transition = agisland.style.transition;
    agisland.style.transition = "none";
    if (mode === "compact" && view && view.style) view.style.width = "max-content";
    agisland.style.width = mode === "full" ? "500px" : "100%";
    agisland.style.height = "auto";
    var width = agisland.offsetWidth;
    if (mode === "idle") width = Math.min(width, view.offsetWidth);
    else if (mode === "compact") {
      width = Math.min(width, view.offsetWidth);
      if (view && view.style) view.style.width = "";
      agisland.style.setProperty("--ag-cw", width + "px");
    } else agisland.style.setProperty("--ag-fw", width + "px");
    var height = view.offsetHeight;
    agisland.style.width = box.width + "px";
    agisland.style.height = box.height + "px";
    void agisland.offsetWidth;
    agisland.style.transition = transition;
    agisland.style.width = width + "px";
    agisland.style.height = height + "px";
  }
  function agOpen(open) {
    if (!agpop) return;
    if (open && typeof hudHideTooltip === "function") hudHideTooltip();
    agpop.hidden = !open;
    if (agisland) {
      agisland.hidden = !open && !agSeen;
      agisland.setAttribute("data-mode", open ? "full" : agRunning ? "compact" : "idle");
    }
    [sbadge, agExpand, agidle].forEach(function (el) { if (el) el.setAttribute("aria-expanded", open ? "true" : "false"); });
    agSize();
    if (!open && agRunning && typeof hudScheduleConnectors === "function") hudScheduleConnectors();
  }
  function agPinned() { return agpin.getAttribute("aria-pressed") === "true"; }
  function agPin(on) {
    agpin.setAttribute("aria-pressed", on ? "true" : "false");
    save(AGPIN_KEY, on ? "1" : null);
  }
  function agProject() {
    if (!agisland || !agcompact || !agidle) return;
    var tree = region("tree");
    if (!tree) return;
    var info = hudProject(tree, agcompact, agidle, agpop, agOpen);
    var firstSeen = !agSeen && (info.visibleCount > 0 || q(".tgroup .agent", tree).length > 0);
    if (firstSeen) agSeen = true;
    agRunning = info.visibleCount;
    if (firstSeen && agPinned() && !agDismissed && agpop.hidden) agOpen(true);
    else if (agpop.hidden) agOpen(false); else agSize();
  }
  function agSwapped() {
    if (!agpop) return;
    q("details[data-key]", agpop).forEach(function (d) { d.classList.toggle("d-full", !!agFull[d.getAttribute("data-key")]); });
    agProject();
    syncBadge();
  }
  if (agpop && sbadge && agpin) {
    sbadge.addEventListener("click", function () { agDismissed = !agpop.hidden; agOpen(agpop.hidden); });
    if (agidle) {
      agidle.addEventListener("click", function () { agOpen(true); });
      agidle.addEventListener("keydown", function (e) { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); agOpen(true); } });
    }
    agpin.addEventListener("click", function () { agPin(!agPinned()); });
    $("agclose").addEventListener("click", function () { agPin(false); agOpen(false); sbadge.focus(); });
    agpop.addEventListener("click", function (e) {
      var b = e.target.closest && e.target.closest("[data-full]"), d = b && b.closest("details[data-key]");
      if (!d) return;
      agFull[d.getAttribute("data-key")] = true;
      d.classList.add("d-full");
      setTimeout(agSize, 0);
    });
    agpop.addEventListener("toggle", function () { setTimeout(agSize, 0); }, true);
    doc.addEventListener("keydown", function (e) {
      if (e.key !== "Escape" || e.defaultPrevented || agpop.hidden) return;
      var inside = agpop.contains(doc.activeElement);
      agDismissed = true;
      agOpen(false);
      if (inside) sbadge.focus();
    });
    doc.addEventListener("pointerdown", function (e) {
      if (agpop.hidden || agPinned() || (agisland || agpop).contains(e.target) || sbadge.contains(e.target)) return;
      agOpen(false);
    }, true);
    window.addEventListener("resize", agSize);
    if (agisland) agisland.hidden = true;
    if (load(AGPIN_KEY) === "1") agPin(true);
    agProject();
    if (!agisland && agPinned()) agOpen(true);
  }
  function interruptTurn() {
    if (interrupting) return;
    interrupting = true;
    interruptedTurn = leadTurn();
    syncBadge();
    post("interrupt", {}, function (ok, res) {
      if (!ok) { interrupting = false; syncBadge(); noteWarn(wsT("js.interrupt.fail", res)); return; }
      var list = res.held || [];
      if (!list.length) { noteOk(wsT("js.interrupt.ok")); return; }
      restoreHeld(list);
      noteOk(wsT("js.interrupt.restored"));
      ctext.focus();
    });
  }
  // steerHeld promotes a held message to steering: the session receives it
  // at its next tool call instead of when its turn ends. The convlive
  // region's next swap shows the steer marker in place of the button.
  function steerHeld(btn) {
    btn.disabled = true;
    post("steer", { id: btn.getAttribute("data-steer") }, function (ok, res) {
      if (!ok) { btn.disabled = false; noteWarn(wsT("js.steer.fail", res)); return; }
      noteOk(wsT("js.steer.ok"));
    });
  }
  // applySent marks each board item the session was sent an answer for, or
  // holds one for, from the inbox records the conversation region carries,
  // and dims the item's answer area (sent-sent, sent-held) until the board
  // re-renders it answered; each run recomputes both from scratch. A run
  // over the same records as the last (sentSig) leaves the board alone unless
  // a board swap asks (force): putting back the same classes would only wake
  // the open-question pill's class observer.
  var sentSig = null;
  function applySent(force) {
    var b = region("board");
    if (!b) return;
    var sig = q(".sentdata i[data-id]").map(function (i) {
      return [i.getAttribute("data-id"), i.getAttribute("data-state"), i.getAttribute("data-answer"), i.getAttribute("data-note")].join("\u0001");
    }).join("\u0002");
    if (!force && sig === sentSig) return;
    sentSig = sig;
    boardQ(".sentmark").forEach(function (m) { m.parentNode.removeChild(m); });
    boardQ(".sent-sent, .sent-held").forEach(function (el) { el.classList.remove("sent-sent", "sent-held"); });
    q(".sentdata i[data-id]").forEach(function (i) {
      var item = boardItem(i.getAttribute("data-id")), head = item && item.querySelector(".head, .stext");
      if (!head) return;
      var state = i.getAttribute("data-state"), m = doc.createElement("span");
      item.classList.add(state === "held" ? "sent-held" : "sent-sent");
      m.className = "sentmark " + state;
      m.textContent = state + " · " + (i.getAttribute("data-answer") || "note");
      var note = i.getAttribute("data-note");
      m.title = wsT(state === "held" ? "js.sent.held.title" : "js.sent.sent.title") + (note ? ": " + note : "");
      var id = head.querySelector(".id");
      head.insertBefore(m, id ? id.nextSibling : head.firstChild);
    });
  }
