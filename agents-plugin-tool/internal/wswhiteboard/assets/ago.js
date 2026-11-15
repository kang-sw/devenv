;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // every minute. The full page and the served page write the change time as
  // data-at (Unix ms) beside a local HH:MM fallback; the embed, a frozen
  // snapshot, writes the HH:MM alone and does not run this. The dashboard
  // re-runs this after every board swap, so it keeps a single timer. Body
  // ages share this refresh; opaque frames load their own copy for tooltips.
  // text is the relative time of at (Unix ms); a stream entry's clock shows
  // it on hover (stream.js).
  function text(at) {
    var m = Math.floor((Date.now() - at) / 60000);
    return m < 1 ? wsT("js.wb.ago.now") : m < 60 ? wsT("js.wb.ago.min", m) :
      m < 1440 ? wsT("js.wb.ago.hour", Math.floor(m / 60)) : wsT("js.wb.ago.day", Math.floor(m / 1440));
  }
  // BodyUpdatedAt is already validated by BuildView. No activity/clock
  // fallback: an absent body time stays unknown in the header and gutter.
  function bodyText(at) {
    var ms = at ? Date.parse(at) : NaN;
    if (!isFinite(ms)) return wsT("js.wb.body.unknown");
    var days = Math.floor((Date.now() - ms) / 86400000);
    var age = days >= 365 ? wsT("js.wb.body.year", Math.floor(days / 365)) :
      days >= 30 ? wsT("js.wb.body.month", Math.floor(days / 30)) : text(ms);
    return wsT("js.wb.body.updated", age);
  }
  function ago(root) {
    Array.prototype.forEach.call(root.querySelectorAll("time.tago[data-at]"), function (t) {
      t.textContent = text(+t.getAttribute("data-at"));
    });
    Array.prototype.forEach.call(root.querySelectorAll("[data-body-age]"), function (t) {
      t.textContent = bodyText(t.getAttribute("data-body-at"));
    });
  }
  if (!window.wsT) return;
  window.wbAgo = ago;
  window.wbAgoText = text;
  window.wbBodyTimeText = bodyText;
  ago(document);
  if (!window.wbAgoTimer) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 60000);
})();
