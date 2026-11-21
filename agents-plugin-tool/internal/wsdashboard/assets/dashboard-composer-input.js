// Composer input sizing and write request helper.
  // The contenteditable capsule editor is a normal block: CSS min/max-height
  // and overflow size it naturally, including its placeholder. Never reset its
  // height and read scrollHeight here: that forces document layout per input.
  // Plain textareas use field-sizing where supported. Only older browsers
  // need a hidden .chl copy, measured once per frame without resetting the
  // visible field's height. Width/font/placeholder changes take the same path.
  var cmeasure = null, ctextFrame = 0;
  var ctextFieldSizing = !!(window.CSS && window.CSS.supports("field-sizing", "content"));
  function ctextHeight() {
    if (!ctext.parentNode) return 0;
    if (!cmeasure) {
      cmeasure = doc.createElement("div");
      cmeasure.className = "chl chm";
      cmeasure.setAttribute("aria-hidden", "true");
      ctext.parentNode.appendChild(cmeasure);
    }
    var paddingRight = chl ? chl.style.paddingRight : "";
    if (cmeasure.style.paddingRight !== paddingRight) cmeasure.style.paddingRight = paddingRight;
    // A trailing newline needs a final line even in a pre-wrap measuring div.
    var text = ctext.value || ctext.getAttribute("placeholder") || "";
    if (text.slice(-1) === "\n" || !text) text += "\u200b";
    if (cmeasure.textContent !== text) cmeasure.textContent = text;
    return cmeasure.offsetHeight;
  }
  function grow() {
    if (ctext.wsc || ctextFieldSizing || ctextFrame) return;
    ctextFrame = requestAnimationFrame(function () {
      ctextFrame = 0;
      var height = Math.min(160, Math.max(38, ctextHeight())) + "px";
      if (ctext.style.height !== height) ctext.style.height = height;
    });
  }
  // CSS-sized fields need no resize observer or font-ready measurement.
  function growCtext() { if (ctext) grow(); }
  if (ctext && !ctext.wsc && !ctextFieldSizing) {
    var ctextW = -1;
    if (window.ResizeObserver) new ResizeObserver(function () {
      var w = ctext.offsetWidth;
      if (w === ctextW) return;
      ctextW = w;
      grow();
    }).observe(ctext);
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(growCtext);
    growCtext();
  }
  // post sends a write route's JSON body; done(ok, answer) gets the parsed
  // answer, or the server's refusal text. The third argument is true when
  // transport failed before a full response. A route is the session's own
  // unless it starts with "/", a daemon-wide route.
  function post(route, body, done) {
    if (route === "steer" && typeof boardIncarnation === "string") body.boardIncarnation = boardIncarnation;
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch(route.charAt(0) === "/" ? route : "/s/" + rid + "/" + route, {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        if (!r.ok) {
          var refusal = s.trim() || "HTTP " + r.status;
          done(false, refusal);
          if (route === "steer" && refusal === "board-replaced") {
            saveDraft(); clearTimeout(draftTimer); window.location.reload();
          }
          return;
        }
        var v = {};
        try { v = JSON.parse(s); } catch (e) { v = {}; }
        done(true, v);
      }, function () { done(false, wsT("js.post.unreachable"), true); });
    }, function () { done(false, wsT("js.post.unreachable"), true); });
  }
