(function () {
  "use strict";
  var cfg = window.wsLogin, root = document.documentElement, osTheme = window.matchMedia("(prefers-color-scheme: dark)");
  if (!root.hasAttribute("data-theme")) {
    function theme() { root.setAttribute("data-theme", osTheme.matches ? "dark" : "light"); }
    theme(); osTheme.addEventListener("change", theme);
  }
  var input = document.getElementById("lg-input"), go = document.getElementById("lg-go"), clip = document.getElementById("lg-clip"), hint = document.getElementById("lg-hint");
  var native = document.getElementById("lg-native"), copy = document.getElementById("lg-copy"), again = document.getElementById("lg-again"), divider = document.getElementById("lg-or");
  var busy = false, copyBusy = false, raw = "", shown = "";
  document.getElementById("lg-mod").textContent = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl";
  var copiedHint = hint.innerHTML;
  function destination(next) {
    // Defense in depth: the server has already validated this local path.
    if (typeof next !== "string" || next[0] !== "/" || next[1] === "/" || /[\\\r\n]/.test(next)) next = "/";
    return next + location.hash;
  }
  function continueTo(next) {
    var target = new URL(destination(next), location.origin);
    // A gate occupies the requested URL already. Replacing that same URL with
    // a fragment is a same-document navigation, so it would leave the gate up.
    if (target.pathname === location.pathname && target.search === location.search) {
      history.replaceState(null, "", target.href);
      location.reload();
    } else location.replace(target.href);
  }
  function message(text, warning) {
    var msg = document.getElementById("lg-msg");
    msg.hidden = false;
    msg.className = "lg-msg " + (warning ? "warn" : "bad");
    document.getElementById("lg-message").textContent = text;
    input.classList.toggle("bad", !warning);
    input.setAttribute("aria-invalid", warning ? "false" : "true");
  }
  function nativeVisible(on) {
    if (native) {
      native.hidden = !on;
      if (on) {
        document.getElementById("lg-sub").hidden = false;
        document.getElementById("lg-native-msg").hidden = true;
      }
    }
    if (divider) divider.hidden = !on;
  }
  function checking(on) {
    busy = on;
    input.disabled = clip.disabled = on;
    if (copy) copy.disabled = on;
    go.disabled = on || !input.value.trim();
    nativeVisible(!on);
    hint.hidden = !on;
    hint.className = "lg-hint busy";
    hint.innerHTML = on ? '<span class="lg-spin"></span>' : copiedHint;
    if (on) hint.appendChild(document.createTextNode(cfg.checking));
  }
  function submit(text) {
    if (busy || copyBusy || !text.trim()) return;
    raw = text.trim();
    // Never leave a usable credential visible while checking or after refusal.
    shown = /[?&]login=/.test(raw) ? "…/?login=••••••••" : "••••••••";
    input.value = shown;
    document.getElementById("lg-msg").hidden = true;
    checking(true);
    fetch("/auth/exchange", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({code:raw, next:cfg.next})}).then(function (r) {
      return r.json().then(function (body) { return {ok:r.ok, status:r.status, body:body}; });
    }).then(function (r) {
      if (!r.ok) {
        checking(false);
        message(r.status === 429 ? cfg.rate : cfg.invalidText, r.status === 429);
        input.focus(); input.select();
        return;
      }
      document.getElementById("lg-form").hidden = true;
      document.getElementById("lg-done").hidden = false;
      setTimeout(function () { continueTo(r.body.next); }, 800);
    }).catch(function () { checking(false); message(cfg.invalidText, false); input.focus(); input.select(); });
  }
  function copyNative() {
    if (busy || copyBusy) return;
    copyBusy = true; copy.disabled = true;
    if (again) again.disabled = true;
    document.getElementById("lg-native-msg").hidden = true;
    document.getElementById("lg-msg").hidden = true;
    input.classList.remove("bad");
    input.removeAttribute("aria-invalid");
    fetch("/auth/clipboard", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({next:destination(cfg.next)})}).then(function (r) {
      if (!r.ok) throw {rate:r.status === 429};
      nativeVisible(false);
      hint.className = "lg-hint go";
      hint.innerHTML = copiedHint;
      hint.hidden = false;
      document.getElementById("lg-msg").hidden = true;
      input.focus();
    }).catch(function (err) {
      nativeVisible(true); hint.hidden = true;
      document.getElementById("lg-sub").hidden = true;
      var msg = document.getElementById("lg-native-msg");
      msg.hidden = false; msg.className = "lg-msg " + (err.rate ? "warn" : "note");
      msg.querySelector("path").setAttribute("d", err.rate ? "M12 8v4M12 16h.01" : "M12 16v-4M12 8h.01");
      document.getElementById("lg-native-message").textContent = err.rate ? cfg.rate : cfg.unavailable;
    }).finally(function () {
      copyBusy = false; copy.disabled = false;
      var retry = document.getElementById("lg-again");
      if (retry) retry.disabled = false;
    });
  }
  if (copy) copy.addEventListener("click", copyNative);
  // Recreated with the copied hint after checking: delegation keeps retry live.
  hint.addEventListener("click", function (e) { if (e.target.closest("#lg-again")) copyNative(); });
  document.addEventListener("paste", function (e) {
    e.preventDefault();
    if (!busy && !copyBusy && e.clipboardData) submit(e.clipboardData.getData("text/plain"));
  });
  input.addEventListener("input", function () { raw = ""; go.disabled = busy || !input.value.trim(); });
  document.getElementById("lg-row").addEventListener("submit", function (e) { e.preventDefault(); submit(raw && input.value === shown ? raw : input.value); });
  clip.addEventListener("click", function () {
    if (busy || copyBusy) return;
    if (!navigator.clipboard) { message(cfg.clipfail, true); return; }
    navigator.clipboard.readText().then(function (s) { if (s.trim()) submit(s); else message(cfg.clipfail, true); }, function () { message(cfg.clipfail, true); });
  });
  // A cross-site navigation omits Strict cookies; this same-origin probe carries
  // them. No clipboard access occurs without an explicit button or paste gesture.
  fetch("/auth/status", {cache:"no-store"}).then(function (r) {
    if (r.ok && !busy && !copyBusy) continueTo(cfg.next);
  }).catch(function () {});
})();
