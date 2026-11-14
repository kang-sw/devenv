/* Account plan usage: a viewer-only cache and the rail's usage popover. */
(function (window) {
  "use strict";
  function active(entry, now) {
    return (entry.windows || []).filter(function (w) { return !w.resetsAt || w.resetsAt > now; });
  }
  function tightest(entry, now) {
    return active(entry, now).reduce(function (n, w) { return Math.max(n, w.percent); }, -1);
  }
  function sorted(entries, now) {
    return entries.slice().sort(function (a, b) { return tightest(b, now) - tightest(a, now); });
  }
  function level(percent, warn, crit) { return percent >= crit ? "critical" : percent >= warn ? "warning" : "normal"; }
  function age(at, now) { return at ? "value from " + Math.max(0, Math.floor((now - at) / 60000)) + " min ago" : "usage unavailable"; }
  function label(kind) { return kind === "five_hour" ? "5-hour" : "Weekly"; }
  function observation(w, entry) { return w.asOf || entry.asOf; }
  function stale(at, now) { return !!at && now - at > 30 * 60000; }
  function pace(w, at) {
    var duration = w.kind === "five_hour" ? 5 * 3600000 : 7 * 86400000;
    if (!w.resetsAt || !at) return null;
    var elapsed = Math.max(0, Math.min(1, (at - (w.resetsAt - duration)) / duration));
    return Math.round(w.percent - elapsed * 100);
  }
  window.WsUsage = { sorted: sorted, level: level, age: age, active: active, mount: function (doc, rail) {
    var data = { entries: [], warnPercent: 80, critPercent: 95 }, pending = null, popover = null, shown = false, place = function () {};
    function el(tag, cls, text) {
      var n = doc.createElement(tag); n.className = cls || "";
      if (text !== undefined) n.textContent = text;
      return n;
    }
    function icon(entry) { return el("span", "hi " + (entry.harness === "claude" ? "claude" : "codex"), entry.harness === "claude" ? "C" : "X"); }
    function clock() {
      var svg = doc.createElementNS("http://www.w3.org/2000/svg", "svg");
      svg.setAttribute("class", "usage-clock"); svg.setAttribute("viewBox", "0 0 16 16"); svg.setAttribute("aria-hidden", "true");
      var circle = doc.createElementNS("http://www.w3.org/2000/svg", "circle");
      circle.setAttribute("cx", "8"); circle.setAttribute("cy", "8"); circle.setAttribute("r", "5.8"); svg.appendChild(circle);
      var hand = doc.createElementNS("http://www.w3.org/2000/svg", "path");
      hand.setAttribute("d", "M8 4.8V8l2.2 1.4"); svg.appendChild(hand);
      return svg;
    }
    function drawFooter() {
      var button = rail.querySelector(".usage-toggle"); if (!button) return;
      var now = Date.now();
      button.textContent = "";
      if (!data.entries.length) button.appendChild(el("span", "usage-empty", "Plan usage"));
      data.entries.forEach(function (entry) {
        var group = el("span", "usage-harness"), windows = active(entry, now);
        var mark = el("span", "usage-mark"), columns = el("span", "usage-columns"), hasStale = false;
        var name = entry.harness === "claude" ? "Claude" : "Codex / GPT";
        mark.appendChild(icon(entry)); group.appendChild(mark);
        ["five_hour", "seven_day"].forEach(function (kind) {
          var w = windows.filter(function (v) { return v.kind === kind; })[0];
          if (!w) return;
          var at = observation(w, entry), old = stale(at, now);
          // Once stale, compare at the last observation, never at an advancing
          // wall clock that would invent headroom from an unchanged value.
          var delta = at ? pace(w, old ? at : now) : null;
          var comparison = delta === null ? "—" : (delta > 0 ? "+" : delta < 0 ? "\u2212" : "") + Math.abs(delta) + "%";
          var column = el("span", "usage-column" + (old ? " usage-stale" : ""));
          var current = el("span", "usage-current"), number = el("span", "usage-number", w.percent);
          number.appendChild(el("span", "usage-percent", "%"));
          current.appendChild(el("span", "usage-key", kind === "five_hour" ? "5h" : "주")); current.appendChild(number);
          column.appendChild(current);
          column.appendChild(el("span", "usage-pace" + (delta > 0 ? " over" : delta < 0 ? " under" : ""), comparison));
          column.title = name + " " + label(kind) + ": " + w.percent + "%; " + (delta === null ? "pace unavailable" : "pace " + comparison + " (percentage points)") + "; " + age(at, now) + (old ? "; stale, pace at last observation" : "");
          column.setAttribute("aria-label", column.title);
          columns.appendChild(column); hasStale = hasStale || old;
        });
        if (hasStale) mark.appendChild(clock());
        group.title = name;
        if (columns.children.length) group.appendChild(columns);
        else group.appendChild(el("span", "usage-empty", "usage unavailable"));
        button.appendChild(group);
      });
    }
    function drawPopover() {
      if (!popover || !shown) return;
      var list = popover.querySelector(".usage-details"); list.textContent = "";
      if (!data.entries.length) list.appendChild(el("p", "usage-status", "not logged in"));
      sorted(data.entries, Date.now()).forEach(function (entry) {
        var row = el("section", "usage-account"), head = el("div", "usage-heading");
        head.appendChild(icon(entry));
        head.appendChild(el("b", "", entry.harness === "claude" ? "Claude" : "Codex / GPT"));
        if (entry.plan) head.appendChild(el("span", "usage-plan", entry.plan));
        row.appendChild(head);
        var windows = active(entry, Date.now());
        windows.sort(function (a, b) { return b.percent - a.percent; }).forEach(function (w) {
          var line = el("div", "usage-window " + level(w.percent, data.warnPercent, data.critPercent));
          line.appendChild(el("span", "", label(w.kind)));
          line.appendChild(el("b", "", w.percent + "%"));
          row.appendChild(line);
          var reset = w.resetsAt ? new Date(w.resetsAt).toLocaleString() : "unknown";
          row.appendChild(el("div", "usage-meta", "Resets " + reset));
          row.appendChild(el("div", "usage-meta", age(w.asOf || entry.asOf, Date.now())));
        });
        if (entry.status || !windows.length) row.appendChild(el("p", "usage-status", entry.status || "usage unavailable"));
        if (!windows.length && entry.asOf) row.appendChild(el("div", "usage-meta", age(entry.asOf, Date.now())));
        if (entry.source) row.appendChild(el("div", "usage-meta", entry.source));
        list.appendChild(row);
      });
      place();
    }
    function refresh(force) {
      if (pending) return pending;
      pending = window.fetch("/rail/usage" + (force ? "?refresh=1" : ""), { credentials: "same-origin", cache: "no-store", headers: force ? { "X-WS-Usage-Refresh": "1" } : {} }).then(function (r) {
        if (!r.ok) throw new Error("usage unavailable");
        return r.json();
      }).then(function (next) {
        data = next; drawFooter(); drawPopover();
      }).catch(function () {
        data.entries.forEach(function (entry) { entry.status = "usage unavailable"; });
        drawPopover();
        if (popover && shown && !data.entries.length) popover.querySelector(".usage-details").textContent = "usage unavailable";
      }).then(function () { pending = null; });
      return pending;
    }
    function makePopover(onPlace, onClose) {
      place = onPlace;
      var box = el("div", "rpop usage-pop"); box.hidden = true; box.tabIndex = -1;
      box.setAttribute("role", "dialog"); box.setAttribute("aria-label", "Plan usage");
      var header = el("div", "usage-heading"); header.appendChild(el("b", "", "Plan usage"));
      var close = el("button", "usage-close", "✕"); close.type = "button"; close.setAttribute("aria-label", "Close plan usage");
      close.addEventListener("click", onClose); header.appendChild(close); box.appendChild(header);
      box.appendChild(el("div", "usage-details")); popover = box;
      return { el: box, open: function () { shown = true; drawPopover(); refresh(true); }, close: function () { shown = false; }, focus: function () { box.focus(); } };
    }
    refresh(false);
    // Only connected, visible pages trigger periodic queries. The daemon
    // coalesces viewers and does no background polling with no page open.
    window.setInterval(function () { if (!doc.hidden) { drawFooter(); drawPopover(); refresh(false); } }, 300000);
    doc.addEventListener("visibilitychange", function () { if (!doc.hidden) refresh(false); });
    return { sync: drawFooter, makePopover: makePopover };
  } };
})(window);
