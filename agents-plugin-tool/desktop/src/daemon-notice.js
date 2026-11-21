(origin, reason) => {
  if (window !== window.top || location.origin !== origin) return 0;
  if (!window.WsToast || typeof window.WsToast.show !== "function") return 0;
  if (typeof window.__werkschaftDaemonToast?.close === "function") {
    window.__werkschaftDaemonToast.close();
  }
  const notice = window.WsToast.show(reason, { kind: "warn" });
  if (!notice || typeof notice.close !== "function") return 0;
  window.__werkschaftDaemonToast = notice;
  return 1;
}
