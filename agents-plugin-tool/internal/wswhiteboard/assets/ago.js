;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // with the topic-state clock. The full page and the served page write the change time as
  // data-at (Unix ms) beside a local HH:MM fallback; the embed, a frozen
  // snapshot, writes the HH:MM alone and does not run this. The dashboard
  // re-runs this after every board swap, so it keeps a single timer. Body
  // ages share this refresh; opaque frames load their own copy for tooltips.
  // text is the relative time of at (Unix ms); a stream entry's clock shows
  // it on hover (stream.js).
  function text(at, now) {
    var m = Math.floor(((now === undefined ? Date.now() : now) - at) / 60000);
    return m < 1 ? wsT("js.wb.ago.now") : m < 60 ? wsT("js.wb.ago.min", m) :
      m < 1440 ? wsT("js.wb.ago.hour", Math.floor(m / 60)) : wsT("js.wb.ago.day", Math.floor(m / 1440));
  }
  function put(n, value) { if (n.textContent !== value) n.textContent = value; }
  function span(cls, value) {
    var n = document.createElement("span");
    n.className = cls;
    if (value) n.textContent = value;
    return n;
  }
  // Date.parse alone normalizes impossible dates (e.g. February 30). Accept
  // complete RFC3339 instants only; missing/diagnostic data is not a new run.
  function instant(value) {
    var m = /^(\d{4})-(\d{2})-(\d{2})T([01]\d|2[0-3]):([0-5]\d):([0-5]\d)(?:\.\d+)?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.exec(value || "");
    if (!m) return NaN;
    var day = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]));
    if (day.getUTCFullYear() !== +m[1] || day.getUTCMonth() !== +m[2] - 1 || day.getUTCDate() !== +m[3]) return NaN;
    return Date.parse(value);
  }
  var states = {
    "awaiting-confirmation": ["confirm", "js.wb.state.awaiting-confirmation"],
    waiting: ["wait", "js.wb.state.waiting"],
    discussing: ["discuss", "js.wb.state.discussing"],
    implementing: ["impl", "js.wb.state.implementing"]
  };
  function badge(topic, now) {
    var state = topic.getAttribute("data-state"), spec = Object.prototype.hasOwnProperty.call(states, state) && states[state];
    var bar = topic.querySelector(".tbar"), box = bar && bar.querySelector(".topic-state");
    if (!bar) return;
    if (!spec || topic.classList.contains("closed") || topic.hasAttribute("data-state-diagnostic")) {
      if (box) box.parentNode.removeChild(box);
      return;
    }
    if (!box) {
      box = span("topic-state");
      var lead = bar.querySelector(".tlead");
      bar.insertBefore(box, lead ? lead.nextSibling : bar.firstChild);
    }
    var owner = topic.getAttribute("data-state-owner") || "", run = topic.getAttribute("data-state-run") || "";
    var own = state === "implementing" && owner.trim() && run.trim() ? owner : "";
    // Only rebuild state text when authority changes, not on periodic ticks.
    var key = JSON.stringify([state, own]);
    if (box.wbStateKey !== key) {
      box.textContent = "";
      var pill = span("stb " + spec[0]);
      var dot = span("d");
      dot.setAttribute("aria-hidden", "true");
      pill.appendChild(dot);
      pill.appendChild(span("state-label", wsT(spec[1]) + (own ? " · " : "")));
      if (own) pill.appendChild(span("own", own));
      box.appendChild(pill);
      box.wbStateKey = key;
    }
    var raw = topic.getAttribute("data-state-eta-minutes") || "", eta = Number(raw);
    var start = instant(topic.getAttribute("data-state-started-at")), deadline = instant(topic.getAttribute("data-state-deadline-at"));
    var timer = box.querySelector(".eta");
    if (!own || !/^\d+$/.test(raw) || !Number.isSafeInteger(eta) || eta <= 0 || !Number.isFinite(start) || !Number.isFinite(deadline) || deadline - start !== eta * 60000) {
      if (timer) timer.parentNode.removeChild(timer);
      return;
    }
    if (!timer) { timer = span("eta"); box.appendChild(timer); }
    var elapsed = Math.max(0, Math.floor((now - start) / 60000)), over = elapsed > eta;
    put(timer, wsT("js.wb.state.eta", eta, elapsed) + (over ? " (+" + (elapsed - eta) + ")" : ""));
    timer.classList.toggle("wb-state-over", over);
  }
  // BodyUpdatedAt is already validated by BuildView. No activity/clock
  // fallback: an absent body time stays unknown in the header and gutter.
  function bodyText(at, now) {
    var ms = at ? Date.parse(at) : NaN;
    if (!isFinite(ms)) return wsT("js.wb.body.unknown");
    if (now === undefined) now = Date.now();
    var days = Math.floor((now - ms) / 86400000);
    var age = days >= 365 ? wsT("js.wb.body.year", Math.floor(days / 365)) :
      days >= 30 ? wsT("js.wb.body.month", Math.floor(days / 30)) : text(ms, now);
    return wsT("js.wb.body.updated", age);
  }
  function ago(root) {
    var now = Date.now();
    Array.prototype.forEach.call(root.querySelectorAll("time.tago[data-at]"), function (t) {
      put(t, text(+t.getAttribute("data-at"), now));
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-body-age]"), function (t) {
      if (t.wbBodyAgePinned) return;
      var topic = t.classList.contains("body-age") && t.closest(".topic");
      var frame = topic && topic.querySelector(".tdoc iframe.doc-frame");
      var held = frame && frame.wbBodyAgeHold;
      put(t, held ? held.text : bodyText(t.getAttribute("data-body-at"), now));
    });
    if (root.matches && root.matches("section.topic[data-topic]")) badge(root, now);
    Array.prototype.forEach.call(root.querySelectorAll("section.topic[data-topic]"), function (topic) { badge(topic, now); });
  }
  if (!window.wsT) return;
  window.wbAgo = ago;
  window.wbAgoText = text;
  window.wbBodyTimeText = bodyText;
  if (!window.wbBodyAgeHover) {
    var meta = document.querySelector('meta[name="wb-doc-key"]'), key = meta && meta.getAttribute("content");
    var pending = null, serial = 0, nonce = Math.random().toString(36).slice(2);
    function post(m) { m.wbDoc = "body-age"; m.docKey = key; window.parent.postMessage(m, "*"); }
    window.wbBodyAgeHover = function (tip, show) {
      if (!key || !window.parent || window.parent === window) {
        window.wbAgo(document);
        show(tip.textContent);
        return;
      }
      if (pending) pending.tip.wbBodyAgeEnd();
      var id = nonce + ":" + (++serial), at = tip.getAttribute("data-body-at") || "";
      pending = { id: id, at: at, tip: tip, show: show };
      tip.wbBodyAgePinned = true;
      tip.wbBodyAgeEnd = function () {
        // Hide before releasing: the host may then advance its header safely.
        tip.hidden = true;
        if (!pending || pending.id !== id) return;
        pending = null;
        tip.wbBodyAgePinned = false;
        tip.wbBodyAgeEnd = null;
        post({ op: "hide", id: id, at: at });
      };
      post({ op: "show", id: id, at: at });
    };
    window.addEventListener("pagehide", function () { if (pending) pending.tip.wbBodyAgeEnd(); });
    window.addEventListener("message", function (e) {
      var m = e.data;
      if (!m || m.wbDoc !== "body-age" || typeof m.id !== "string" || m.id.length > 80) return;
      if (key) {
        if (e.source !== window.parent || m.docKey !== key || m.op !== "value" || !pending ||
            m.id !== pending.id || m.at !== pending.at || typeof m.text !== "string" || m.text.length > 200) return;
        pending.show(m.text);
        return;
      }
      if (m.op !== "show" && m.op !== "hide") return;
      var frames = document.querySelectorAll(".tdoc iframe.doc-frame"), f = null;
      for (var i = 0; i < frames.length; i++) if (frames[i].contentWindow === e.source) { f = frames[i]; break; }
      if (!f || !m.docKey || m.docKey !== f.getAttribute("data-doc-key") || m.docKey !== f.getAttribute("data-doc")) return;
      var topic = f.closest(".topic"), header = topic && topic.querySelector(".body-age");
      if (!header) return;
      if (m.op === "hide") {
        if (!f.wbBodyAgeHold || f.wbBodyAgeHold.id !== m.id) return;
        delete f.wbBodyAgeHold;
        window.wbAgo(document);
        return;
      }
      if (m.at !== (header.getAttribute("data-body-at") || "")) return;
      var value = window.wbBodyTimeText(m.at);
      // Two opaque documents cannot paint an atomic clock tick. Hold this one
      // displayed value until the child has hidden its tooltip, not on a timer.
      // Keeping the hold on the iframe also survives a board's frame graft.
      f.wbBodyAgeHold = { id: m.id, text: value };
      header.textContent = value;
      e.source.postMessage({ wbDoc: "body-age", docKey: m.docKey, op: "value", id: m.id, at: m.at, text: value }, "*");
    });
  }
  ago(document);
  // One document-owned clock, shared with updated-ago. Query live nodes each
  // tick, including pinned/adopted topics; no captured list retains old boards.
  // A second-level tick handles run-relative minute boundaries, not wall minutes.
  if (!window.wbAgoTimer) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 1000);
})();
