;(function () {
  // The dashboard's one toast surface (window.WsToast), shared by the page
  // script and the tracker renderer, each of which also loads without the
  // other on some page. Toasts stack bottom-right, newest on top, at most
  // MAX at once: a new one past the cap drops the oldest. Each leaves after
  // its duration, which a hover pauses and leaving resumes; a toast that
  // leaves is removed from the page.
  //
  // WsToast.show(content, opts) -> { close }
  //   content  text by default; opts.html: true sets it as HTML instead, for
  //            callers that already escaped what they pass
  //   opts.kind      "info" (default), "notice" (a tracker notification: a
  //                  bar on its left), or "summary" (a dot before its text)
  //   opts.duration  ms before it leaves; default 4000, 8000 for "notice"
  //   opts.onClick   makes the toast a button: a click (or Enter / Space)
  //                  runs it and closes the toast
  if (window.WsToast) return;
  var doc = document, MAX = 3, DURATION = { info: 4000, notice: 8000, summary: 8000 };
  var box = null, live = []; // live: the shown toasts' close functions, oldest first

  function stack() {
    if (box && box.parentNode) return box;
    box = doc.createElement("div");
    box.className = "wstoasts";
    box.id = "wstoasts";
    box.setAttribute("role", "status");
    box.setAttribute("aria-live", "polite");
    doc.body.appendChild(box);
    return box;
  }

  function show(content, opts) {
    opts = opts || {};
    var kind = DURATION[opts.kind] ? opts.kind : "info";
    var ms = opts.duration > 0 ? opts.duration : DURATION[kind];
    var s = stack(), el = doc.createElement("div"), body = doc.createElement("span");
    el.className = "wstoast " + kind;
    body.className = "wst-body";
    if (opts.html) body.innerHTML = String(content);
    else body.textContent = String(content);
    el.appendChild(body);

    var timer = 0, left = ms, since = 0, gone = false;
    function close() {
      if (gone) return;
      gone = true;
      clearTimeout(timer);
      live.splice(live.indexOf(close), 1);
      if (el.parentNode) el.parentNode.removeChild(el);
    }
    function run() {
      since = Date.now();
      timer = setTimeout(close, left);
    }
    el.addEventListener("mouseenter", function () {
      clearTimeout(timer);
      left = Math.max(0, left - (Date.now() - since));
    });
    el.addEventListener("mouseleave", function () { if (!gone) run(); });
    if (typeof opts.onClick === "function") {
      el.classList.add("click");
      el.setAttribute("role", "button");
      el.tabIndex = 0;
      var go = function (e) {
        if (e.type === "keydown" && e.key !== "Enter" && e.key !== " ") return;
        if (e.preventDefault) e.preventDefault();
        close();
        opts.onClick();
      };
      el.addEventListener("click", go);
      el.addEventListener("keydown", go);
    }

    s.insertBefore(el, s.firstChild);
    live.push(close);
    while (live.length > MAX) live[0]();
    run();
    return { close: close };
  }

  window.WsToast = { show: show };
})();
