  // Full page only: runs after form.js in the same function scope.
  var store = null; // sessionStorage, guarded: a sandboxed frame throws on access
  try { store = window.sessionStorage; store.getItem("wb"); } catch (e) { store = null; }

  // --- panels: remember open state across reloads ---------------------------
  q("details.panel[id]").forEach(function (d) {
    if (!store) return;
    var v = store.getItem("wb-open-" + d.id);
    if (v !== null) d.open = v === "1";
    d.addEventListener("toggle", function () { store.setItem("wb-open-" + d.id, d.open ? "1" : "0"); });
  });

  // --- change signal ----------------------------------------------------------
  // The stamp script beside the page calls wbStamp("<seq>"); a value other than
  // this page's own seq (data-seq) means the board was rewritten, including by a
  // render that landed between this page's load and its first poll.
  var pending = false;
  var banner = doc.getElementById("wb-banner");
  function formBusy() {
    var a = doc.activeElement;
    return (dirty && !copied) || (a && (a.classList.contains("note") || a.id === "reply-text")) || !!doc.querySelector(".wbv:not([hidden])");
  }
  function reloadNow() {
    if (store) store.setItem("wb-scroll", String(window.scrollY));
    location.reload();
  }
  function changed() {
    if (formBusy()) { pending = true; if (banner) banner.classList.add("show"); }
    else reloadNow();
  }
  onCopied = function () { if (pending) reloadNow(); };
  // The image viewer holds a reload off while it is open (formBusy), and lets it go on close.
  doc.addEventListener("wbviewerclose", function () { if (pending && !formBusy()) reloadNow(); });
  if (banner) banner.addEventListener("click", reloadNow);
  var stampSrc = doc.documentElement.getAttribute("data-stamp");
  var ownSeq = doc.documentElement.getAttribute("data-seq");
  if (stampSrc) {
    window.wbStamp = function (v) {
      if (v !== ownSeq && !pending) changed();
    };
    var poll = function () {
      var s = doc.createElement("script");
      s.src = stampSrc + "?t=" + Date.now();
      s.onload = s.onerror = function () { s.remove(); };
      doc.head.appendChild(s);
    };
    poll();
    setInterval(poll, 1500);
  }

  // --- scroll restore: the very last step --------------------------------------
  // Chrome scroll anchoring shifts a restored position when content above the
  // viewport grows after scrollTo, so this runs after every synchronous fill.
  if (store) {
    var y = store.getItem("wb-scroll");
    if (y !== null) { store.removeItem("wb-scroll"); window.scrollTo(0, +y); }
  }
