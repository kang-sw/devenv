;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // with the topic-state clock. The full page and the served page write the change time as
  // data-at (Unix ms) beside a local HH:MM fallback; the embed, a frozen
  // snapshot, writes the HH:MM alone and does not run this. The dashboard
  // re-runs this after every board swap, so it keeps a single timer.
  // text is the relative time of at (Unix ms); a stream entry's clock shows
  // it on hover (stream.js).
  function text(at) {
    var m = Math.floor((Date.now() - at) / 60000);
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
    timer.classList.toggle("over", over);
  }
  function ago(root) {
    Array.prototype.forEach.call(root.querySelectorAll("time.tago[data-at]"), function (t) {
      put(t, text(+t.getAttribute("data-at")));
    });
    var now = Date.now();
    if (root.matches && root.matches("section.topic[data-topic]")) badge(root, now);
    Array.prototype.forEach.call(root.querySelectorAll("section.topic[data-topic]"), function (topic) { badge(topic, now); });
  }
  if (!window.wsT) return;
  window.wbAgo = ago;
  window.wbAgoText = text;
  ago(document);
  // One document-owned clock, shared with updated-ago. Query live nodes each
  // tick, including pinned/adopted topics; no captured list retains old boards.
  // A second-level tick handles run-relative minute boundaries, not wall minutes.
  if (!window.wbAgoTimer) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 1000);
})();
