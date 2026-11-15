;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // every minute. The full page and the served page write the change time as
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
      t.textContent = text(+t.getAttribute("data-at"), now);
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-body-age]"), function (t) {
      if (t.wbBodyAgePinned) return;
      var topic = t.classList.contains("body-age") && t.closest(".topic");
      var frame = topic && topic.querySelector(".tdoc iframe.doc-frame");
      var held = frame && frame.wbBodyAgeHold;
      t.textContent = held ? held.text : bodyText(t.getAttribute("data-body-at"), now);
    });
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
  if (!window.wbAgoTimer) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 60000);
})();
