(function () {
  // The dashboard page script, shared by / and /s/<reply-id>. The server
  // renders every region; this script only swaps a region when an SSE event
  // names it, keeps the browser-side view state across swaps, and formats
  // times. It never builds region HTML itself.
  var doc = document, root = doc.documentElement;
  function q(sel, el) { return Array.prototype.slice.call((el || doc).querySelectorAll(sel)); }
  var mode = root.getAttribute("data-mode");
  var rid = root.getAttribute("data-rid");

  // --- times: the server writes epoch ms; the page shows local or relative time
  function two(n) { return (n < 10 ? "0" : "") + n; }
  function dur(ms) {
    var s = Math.max(0, Math.round(ms / 1000));
    if (s < 60) return s + "s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + "m " + (s % 60) + "s";
    return Math.floor(m / 60) + "h " + (m % 60) + "m";
  }
  function fmtTimes(el) {
    var now = Date.now();
    q("time[data-ms]", el).forEach(function (t) {
      var ms = +t.getAttribute("data-ms");
      if (!ms) return;
      var f = t.getAttribute("data-f");
      if (f === "since") t.textContent = dur(now - ms);
      else if (f === "ago") t.textContent = dur(now - ms) + " ago";
      else { var d = new Date(ms); t.textContent = two(d.getHours()) + ":" + two(d.getMinutes()); }
      t.title = new Date(ms).toLocaleString();
    });
  }
  setInterval(function () { fmtTimes(doc); }, 1000);

  // --- region swap -----------------------------------------------------------
  function region(name) { return doc.getElementById("r-" + name); }
  // The mode pseudo-region's hash is unknown to the page until the first
  // event carries it; an empty hash makes the server send it once.
  var modeHash = "";
  function held() {
    return q("[data-region]").map(function (el) {
      return el.getAttribute("data-region") + "." + el.getAttribute("data-h");
    }).concat(mode !== "index" ? ["mode." + modeHash] : []).join(",");
  }
  function swap(name, hash, html) {
    var el = region(name);
    if (!el || el.getAttribute("data-h") === hash) return;
    el.innerHTML = html;
    el.setAttribute("data-h", hash);
    fmtTimes(el);
    if (name === "board") {
      if (window.wbForm) window.wbForm();
      mermaid();
    }
  }
  function mermaid() {
    if (!q("pre.mermaid:not([data-processed])").length || !window.wbMermaidFile) return;
    if (window.mermaid) { window.wbMermaid(); return; }
    var s = doc.createElement("script");
    s.src = window.wbMermaidFile; // relative: the server serves it beside the page
    s.onload = function () { window.wbMermaid(); };
    doc.head.appendChild(s);
  }

  // --- stream ----------------------------------------------------------------
  var sse = doc.getElementById("sse");
  function connected(on) { if (sse) sse.classList.toggle("off", !on); }
  function connect() {
    if (!window.EventSource) return;
    var path = mode === "index" ? "/events" : "/s/" + rid + "/events";
    var es = new EventSource(path + "?h=" + encodeURIComponent(held()));
    es.onopen = function () { connected(true); };
    es.onerror = function () {
      connected(false);
      // EventSource would retry with the URL it opened; reopen with the
      // hashes held now, so a reconnect sends only what changed since.
      es.close();
      setTimeout(connect, 3000);
    };
    es.addEventListener("region", function (e) {
      var ev = JSON.parse(e.data);
      if (ev.name === "mode") {
        if (ev.html !== mode) modeChanged(ev.html);
        modeHash = ev.hash;
        return;
      }
      swap(ev.name, ev.hash, ev.html);
    });
    es.addEventListener("gone", function () {
      es.close();
      connected(false);
      banner("The dashboard no longer serves this session.");
    });
  }
  function banner(text) {
    var b = doc.getElementById("dash-banner");
    if (!b) return;
    b.textContent = text;
    b.hidden = false;
  }
  function modeChanged(next) {
    // The other mode has other regions, and a reload renders them; text typed
    // in the composer is never dropped for it.
    var ta = doc.getElementById("ctext");
    if (ta && ta.value) {
      banner("The session's live state " + (next === "live" ? "appeared" : "went away") + ". Reload to update the layout.");
      return;
    }
    location.reload();
  }

  if (window.wbForm) window.wbForm();
  fmtTimes(doc);
  mermaid();
  connect();
})();
