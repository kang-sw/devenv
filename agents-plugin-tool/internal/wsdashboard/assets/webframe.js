(function () {
  // The web card's wrapper frame (webcard_routes.go serves it, inline and
  // nonced): it frames the proxied page and relays postMessage between that
  // inner frame and the card on the session page, checking each hop's
  // source and origin, and beats the target's heartbeat while it is open.
  // The protocol is in webcard_routes.go's file comment.
  var doc = document, root = doc.documentElement;
  var proxy = root.getAttribute("data-proxy"), beat = root.getAttribute("data-beat");
  var inner = doc.getElementById("f"), host = location.origin;
  if (!proxy || !inner || window.parent === window) return;
  function msg(m) { return !!m && typeof m === "object" && typeof m.wsWeb === "string"; }
  window.addEventListener("message", function (e) {
    if (!msg(e.data)) return;
    if (e.source === inner.contentWindow && e.origin === proxy) {
      window.parent.postMessage(e.data, host);
    } else if (e.source === window.parent && e.origin === host && inner.contentWindow) {
      inner.contentWindow.postMessage(e.data, proxy);
    }
  });
  window.parent.postMessage({ wsWeb: "frame", proxy: proxy }, host);
  if (!beat || !window.fetch) return;
  setInterval(function () {
    fetch(beat, { method: "POST", credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (r.status === 404) window.parent.postMessage({ wsWeb: "gone" }, host);
    }, function () {});
  }, 60000);
})();
