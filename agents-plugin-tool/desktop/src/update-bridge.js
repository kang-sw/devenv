function installWerkschaftUpdateBridge(origin, channel, phase) {
  if (window !== window.top || location.origin !== origin ||
      document.documentElement.hasAttribute('data-login-gate') || window.werkschaftDesktop) return;
  const mac = window.webkit?.messageHandlers?.werkschaftUpdate;
  const windows = window.chrome?.webview;
  const transport = mac || windows;
  if (!transport) return;
  const post = transport.postMessage.bind(transport);
  const send = (command) => {
    if (navigator.userActivation?.isActive === false) return;
    post('werkschaft:update:' + command);
  };
  let status = { state: 'phase', phase }, listener;
  Object.defineProperty(window, '__werkschaftUpdateStatus', { value: (next) => {
    status = next;
    if (listener) listener(status);
  } });
  const update = Object.freeze({
    channel: () => Object.freeze(channel),
    check: () => send('check'),
    checkCanary: () => send('canary'),
    backToStable: () => send('stable-return'),
    download: () => send('download'),
    skip: () => send('skip'),
    restart: () => send('restart'),
    onStatus: (callback) => {
      if (typeof callback !== 'function') return;
      listener = callback;
      callback(status);
    }
  });
  Object.defineProperty(window, 'werkschaftDesktop', { value: Object.freeze({ update }) });
  window.dispatchEvent(new Event('werkschaft-update-ready'));
}
