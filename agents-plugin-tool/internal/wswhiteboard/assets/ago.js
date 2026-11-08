;(function () {
  // Updated-ago: each topic bar's latest change as relative time, refreshed
  // every minute. The full page and the served page write the change time as
  // data-at (Unix ms) beside a local HH:MM fallback; the embed, a frozen
  // snapshot, writes the HH:MM alone and does not run this. The dashboard
  // re-runs this after every board swap, so it keeps a single timer.
  function ago(root) {
    var now = Date.now();
    Array.prototype.forEach.call(root.querySelectorAll("time.tago[data-at]"), function (t) {
      var m = Math.floor((now - +t.getAttribute("data-at")) / 60000);
      t.textContent = m < 1 ? wsT("js.wb.ago.now") : m < 60 ? wsT("js.wb.ago.min", m) :
        m < 1440 ? wsT("js.wb.ago.hour", Math.floor(m / 60)) : wsT("js.wb.ago.day", Math.floor(m / 1440));
    });
  }
  if (!window.wsT) return;
  window.wbAgo = ago;
  ago(document);
  if (!window.wbAgoTimer) window.wbAgoTimer = setInterval(function () { window.wbAgo(document); }, 60000);
})();
