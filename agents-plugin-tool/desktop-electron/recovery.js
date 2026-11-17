'use strict';

// Keep a single retry outstanding, including while a connection attempt is running.
function createRecovery({ retry, setTimer = setTimeout, clearTimer = clearTimeout }) {
  let delay = 1000, timer = null, hang = null, active = false;
  function schedule() {
    active = true;
    if (timer !== null) return;
    const wait = delay;
    delay = Math.min(delay * 2, 10000);
    timer = setTimer(() => { timer = null; retry(); }, wait);
    timer?.unref?.();
  }
  function success() {
    responsive();
    active = false;
    delay = 1000;
    if (timer !== null) clearTimer(timer);
    timer = null;
  }
  function responsive() {
    if (hang !== null) clearTimer(hang);
    hang = null;
  }
  return {
    failure(isMainFrame, code) { if (isMainFrame && code !== -3) schedule(); },
    gone(reason) { if (reason !== 'clean-exit') schedule(); },
    unresponsive() {
      if (hang !== null) return;
      hang = setTimer(() => { hang = null; schedule(); }, 10000);
      hang?.unref?.();
    },
    responsive, success, schedule,
    retryIfActive() { if (active) schedule(); },
    dispose() { success(); responsive(); },
  };
}

function safeURL(value) {
  try {
    const url = new URL(value);
    url.username = ''; url.password = ''; url.search = ''; url.hash = '';
    return url.href;
  } catch { return '<invalid URL>'; }
}

module.exports = { createRecovery, safeURL };
