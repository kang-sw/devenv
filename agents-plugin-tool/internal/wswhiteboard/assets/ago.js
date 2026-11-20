;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // with the topic-state clock. The full page and the served page write the change time as
  // data-at (Unix ms) beside a local HH:MM fallback; a frozen
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
    researching: ["research", "js.wb.state.researching"],
    implementing: ["impl", "js.wb.state.implementing"]
  };
  function glyph(name) {
    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", "i");
    svg.setAttribute("aria-hidden", "true");
    var use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", "#i-st-" + name);
    svg.appendChild(use);
    return svg;
  }
  function child(parent, tag, cls, value) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (value !== undefined) n.textContent = value;
    parent.appendChild(n);
    return n;
  }
  function badge(topic, now) {
    var state = topic.getAttribute("data-state"), spec = Object.prototype.hasOwnProperty.call(states, state) && states[state];
    var bar = topic.querySelector(".tbar"), box = bar && bar.querySelector(".topic-state");
    if (!bar) return;
    if (!spec || topic.classList.contains("closed") || topic.hasAttribute("data-state-diagnostic")) {
      if (box) box.parentNode.removeChild(box);
      return;
    }
    if (!box) {
      box = span("topic-state dw");
      bar.insertBefore(box, bar.firstChild);
    }
    var owner = topic.getAttribute("data-state-owner") || "", run = topic.getAttribute("data-state-run") || "";
    var own = owner.trim() && (state === "researching" || state === "implementing" && run.trim()) ? owner : "";
    var raw = topic.getAttribute("data-state-eta-minutes") || "", eta = Number(raw);
    var estimate = state === "researching" && /^\d+$/.test(raw) && Number.isSafeInteger(eta) && eta > 0 ? eta : 0;
    var start = instant(topic.getAttribute("data-state-started-at")), deadline = instant(topic.getAttribute("data-state-deadline-at"));
    var clock = state === "implementing" && own && /^\d+$/.test(raw) && Number.isSafeInteger(eta) && eta > 0 && Number.isFinite(start) && Number.isFinite(deadline) && deadline - start === eta * 60000;
    var elapsed = clock ? Math.max(0, Math.floor((now - start) / 60000)) : 0, over = clock && elapsed > eta;
    var initial = Number(topic.getAttribute("data-state-initial-eta-minutes")), extensions = Number(topic.getAttribute("data-state-extensions"));
    var extended = clock && Number.isSafeInteger(initial) && initial > 0 && initial < eta && Number.isSafeInteger(extensions) && extensions > 0;
    // Authority changes rebuild the pill; ticks keep its focused node and SVG.
    var key = JSON.stringify([state, own, estimate, !!clock, eta, initial, extensions, run, start, deadline]);
    var pill = box.querySelector(".wb-state-pill"), tip = box.querySelector(".sb-tip");
    if (box.wbStateKey !== key && !(box.wbStateKey === undefined && pill && tip)) {
      box.textContent = "";
      pill = child(box, "span", "wb-state-pill " + spec[0] + (!clock && !estimate ? " solo" : ""));
      pill.setAttribute("tabindex", "0");
      pill.setAttribute("role", "img");
      pill.appendChild(glyph(spec[0]));
      if (clock || estimate) {
        var time = child(pill, "span", "wb-state-time" + (estimate ? " est" : ""));
        if (clock) child(time, "b", "", elapsed);
        child(time, "small", "", wsT(clock ? "js.wb.state.eta.max" : "js.wb.state.estimate.short", eta));
      }
      tip = child(box, "span", "pop sb-tip " + spec[0]);
      tip.setAttribute("role", "tooltip");
      tip.setAttribute("popover", "manual");
      tip.hidden = true;
      var head = child(tip, "span", "h");
      head.appendChild(glyph(spec[0]));
      head.appendChild(document.createTextNode(wsT(spec[1])));
      if (own) child(tip, "span", "who", own);
      if (clock) {
        child(tip, "span", "tl");
        if (extended) child(tip, "span", "ext", wsT("js.wb.state.extended", extensions, initial));
        child(child(tip, "span", "bar"), "i");
      }
      if (estimate) child(tip, "span", "tl", wsT("js.wb.state.estimate", estimate));
    }
    box.wbStateKey = key;
    var parts = [wsT(spec[1])];
    if (own) parts.push(own);
    if (clock) {
      parts.push(wsT("js.wb.state.eta.tip", eta, elapsed));
      if (over) parts.push(wsT("js.wb.state.over", elapsed - eta));
      if (extended) parts.push(wsT("js.wb.state.extended", extensions, initial));
      put(pill.querySelector("b"), String(elapsed));
      // The hidden tip is not a live region. Refresh its clock only per minute.
      if (tip.wbMinute !== elapsed) {
        var line = tip.querySelector(".tl");
        line.textContent = wsT("js.wb.state.eta.tip", eta, elapsed);
        if (over) {
          line.appendChild(document.createTextNode(" · "));
          child(line, "span", "ov", wsT("js.wb.state.over", elapsed - eta));
        }
        tip.querySelector("i").setAttribute("style", "width:" + Math.min(100, Math.floor(elapsed * 100 / eta)) + "%");
        tip.wbMinute = elapsed;
      }
    }
    if (estimate) parts.push(wsT("js.wb.state.estimate", estimate));
    var label = parts.join(" · ");
    if (pill.getAttribute("aria-label") !== label) pill.setAttribute("aria-label", label);
    pill.classList.toggle("wb-state-over", !!over);
    tip.classList.toggle("wb-state-over", !!over);
  }
  // BodyUpdatedAt is already validated by BuildView. No activity/clock
  // fallback: an absent body time stays unknown in the gutter.
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
    if (document.hidden || document.documentElement.hasAttribute("data-animation-paused")) return;
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
    // History entry times: relative within the first hour, then the server
    // clock cached on first sight. data-at/data-stamp stay the exact time.
    Array.prototype.forEach.call(root.querySelectorAll("time.entry-at[data-at]"), function (t) {
      var raw = t.getAttribute("data-at"), at = +raw;
      if (!raw || !isFinite(at)) return;
      if (t._wbClock === undefined) t._wbClock = t.textContent;
      put(t, now - at < 3600000 ? text(at, now) : t._wbClock);
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
      var topic = f.closest(".topic"), source = topic && topic.querySelector(".tdoc[data-body-at]");
      if (!source) return;
      if (m.op === "hide") {
        if (!f.wbBodyAgeHold || f.wbBodyAgeHold.id !== m.id) return;
        delete f.wbBodyAgeHold;
        window.wbAgo(document);
        return;
      }
      if (m.at !== (source.getAttribute("data-body-at") || "")) return;
      var value = window.wbBodyTimeText(m.at);
      // Two opaque documents cannot paint an atomic clock tick. Hold this one
      // displayed value until the child has hidden its tooltip, not on a timer.
      // Keeping the hold on the iframe also survives a board's frame graft.
      f.wbBodyAgeHold = { id: m.id, text: value };
      e.source.postMessage({ wbDoc: "body-age", docKey: m.docKey, op: "value", id: m.id, at: m.at, text: value }, "*");
    });
  }
  ago(document);
  // One document-owned clock, shared with updated-ago. Query live nodes each
  // tick, including pinned/adopted topics; no captured list retains old boards.
  // A second-level tick handles run-relative minute boundaries, not wall minutes.
  if (!window.wbAgoClockInit) {
    window.wbAgoClockInit = true;
    function syncAgoClock() {
      var hidden = document.hidden || document.documentElement.hasAttribute("data-animation-paused");
      if (hidden) {
        if (window.wbAgoTimer != null) { clearInterval(window.wbAgoTimer); window.wbAgoTimer = null; }
        return;
      }
      if (window.wbAgoTimer == null) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 1000);
      window.wbAgo(document);
    }
    document.addEventListener("visibilitychange", syncAgoClock);
    document.addEventListener("ws-page-visibility", syncAgoClock);
    syncAgoClock();
  }
})();
