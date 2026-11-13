(function () {
  // The web card's injected script: served by each proxy listener at its
  // reserved path (webcard_proxy.go webInjectPath) and injected as
  // <script src> into every HTML page the proxy serves. The listener bakes
  // the daemon's port into PORT. The protocol is in webcard_routes.go's
  // file comment.
  var PORT = "__WS_DASHBOARD_PORT__";
  if (window.__wsWeb || window.parent === window) return;
  window.__wsWeb = true;
  var DASH = "http://" + location.hostname + ":" + PORT;
  function here() { return location.pathname + location.search + location.hash; }
  function post(m) { window.parent.postMessage(m, DASH); }
  post({ wsWeb: "ready", path: here() });
})();
