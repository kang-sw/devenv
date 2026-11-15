  // --- clicks: delegated, so they survive every swap -----------------------
  doc.addEventListener("click", function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (cchips && inBoard(t)) syncChips();
    var u = t.closest("[data-unchip]");
    if (u) { var uid = u.getAttribute("data-unchip"); dropChip(uid); setItem(uid, null); renderChips(); return; }
    u = t.closest("[data-untchip]");
    if (u) { clearTopicNote(u.getAttribute("data-untchip")); renderChips(); return; }
    u = t.closest("[data-unmchip]");
    if (u) { bmentions.splice(+u.getAttribute("data-unmchip"), 1); renderChips(); return; }
    u = t.closest("[data-unwpick]");
    if (u) { wpicks.splice(+u.getAttribute("data-unwpick"), 1); renderChips(); return; }
    u = t.closest("[data-unwcon]");
    if (u) { wconsoles.splice(+u.getAttribute("data-unwcon"), 1); renderChips(); return; }
    u = t.closest("[data-unclose-request]");
    if (u) { removeCloseRequest(u.getAttribute("data-unclose-request")); return; }
    u = t.closest(".t-close-request");
    if (u && inBoard(u)) {
      var topic = u.closest("section.topic[data-topic]");
      if (topic && !topic.classList.contains("closed")) {
        // Restore through the sheet owner so its scrim cannot cover the reason field.
        if (topic.hasAttribute("data-max")) {
          var back = topic.querySelector(".d-min");
          if (back) back.click();
        }
        addCloseRequest(topic.getAttribute("data-topic"));
      }
      return;
    }
    if (t.closest("[data-wbmention]")) { addMention(); return; }
    u = t.closest("[data-tchip]");
    if (u) { focusTopicNote(u.getAttribute("data-tchip")); return; }
    if (t.closest("#ccmd [data-uncmd]")) { clearCommand(); ctext.focus(); return; }
    u = t.closest("[data-unimg]");
    if (u) { unattach(imgs[+u.getAttribute("data-unimg")]); return; }
    u = t.closest("#r-board button.closed-more[data-closed-next]");
    if (u) { loadClosed(u); return; }
    u = t.closest("#r-convlive [data-cancel]");
    if (u) { if (!u.disabled) cancelHeld(u); return; }
    u = t.closest("#r-convlive [data-steer]");
    if (u) { if (!u.disabled) steerHeld(u); return; }
    if (ibadge && t.closest("#ibadge")) { if (!ibadge.disabled) interruptTurn(); return; }
    if (gmenu && t.closest("#settings-close")) { railPopClose(true); return; }
    var th = t.closest(".gmenu [data-theme-set]");
    if (th) { setTheme(th.getAttribute("data-theme-set")); return; }
    var b = t.closest(".gmenu [data-input]");
    if (b) { save(INPUT_KEY, b.getAttribute("data-input")); setInput(b.getAttribute("data-input")); if (shell) shFrames.forEach(shSettings); return; }
    b = t.closest(".gmenu [data-layout]");
    if (b) { save(LAYOUT_KEY, b.getAttribute("data-layout")); setLayout(b.getAttribute("data-layout")); if (shell) shFrames.forEach(shSettings); return; }
    if (!live) return;
    b = t.closest("#tabbar [data-tab]");
    if (b) { setTab(b.getAttribute("data-tab")); return; }
    b = t.closest("#ctabs [data-ctab]");
    if (b) { setCtab(b.getAttribute("data-ctab")); return; }
  });

  // --- link actions: the menu a served page's marked links open ------------
  // The renderer marks a served link data-wl="file" (href: the file view, or
  // no href outside the session root; data-rel, data-abs) or data-wl="url";
  // no other anchor is touched. An image file link is also marked data-img;
  // an absolute image path outside the root is marked data-out, with the
  // image route as its href, which the daemon serves only to a browser on
  // its own machine. A plain click opens the menu, or, on a file link the
  // preview card can show (previews), the preview slot, with the menu on
  // the right click and the keyboard activation instead; Cmd/Ctrl+click on a link
  // the page can open is left to the browser, which opens a new tab, and
  // opens the menu otherwise. The open-with-system item shows only on a
  // page the server marked data-sysopen (a browser on the daemon's
  // machine), and a data-out link opens at all only there; the daemon checks
  // both again. The markup is the same for every viewer: entries render once
  // for all of them.
  var lmenu = null, lmenuFrom = null;
  function closeLinkMenu(refocus) {
    if (!lmenu) return;
    lmenu.parentNode.removeChild(lmenu);
    lmenu = null;
    if (refocus && lmenuFrom && lmenuFrom.isConnected) lmenuFrom.focus();
    lmenuFrom = null;
  }
  function toast(text) { if (window.WsToast) window.WsToast.show(text); }
  // copyText copies s: the Clipboard API where the page is a secure context,
  // else (a LAN address over plain http has no navigator.clipboard) a hidden
  // textarea and the copy command.
  function copyText(s) {
    function fallback() {
      var t = doc.createElement("textarea");
      t.value = s;
      t.setAttribute("readonly", "");
      t.style.cssText = "position:fixed;top:0;left:0;width:1px;height:1px;opacity:0";
      doc.body.appendChild(t);
      t.select();
      var ok = false;
      try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
      doc.body.removeChild(t);
      toast(wsT(ok ? "js.toast.copied" : "js.toast.copyfail", s));
    }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(s).then(function () { toast(wsT("js.toast.copied", s)); }, fallback);
    } else fallback();
  }
  // --- session label: the head's name -----------------------------------------
  // The head's name (sname, dashboard.tmpl) shows the session's display
  // label, or its reply-id when it has none; a click on it copies the
  // session's mailbox address, which the server builds (data-copy). The
  // pencil beside it turns the name into a one-line input: Enter saves
  // through the label route (label_route.go), Escape or leaving the input
  // cancels (the tracker's in-place title edit also cancels on Escape), and
  // an empty value clears the label, which shows the reply-id again. A head
  // swap that arrives while the input is open waits (headHeld) and lands
  // when the edit ends, so a stream update never drops the typing. A rail
  // row's pencil runs the same edit on that row (railRename).
  var headHeld = null;
  function labelEditing() {
    var h = region("head");
    return !!(h && h.querySelector(".sinp"));
  }
  // labelClean is the value the route stores: one line without control
  // characters, trimmed, at most max characters.
  function labelClean(s, max) {
    var v = String(s).replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]+/g, " ").trim();
    return Array.from(v).slice(0, max).join("").trim();
  }
  // labelClick handles a click inside the head: the name copies, the pencil
  // starts the rename.
  function labelClick(t) {
    var sn = t && t.closest ? t.closest(".sname") : null;
    if (!sn) return;
    if (t.closest(".snm")) copyText(t.closest(".snm").getAttribute("data-copy"));
    else if (t.closest(".sren")) labelStart(labelHead(sn));
  }
  // labelHead is the head name's edit. An edit says where its input goes
  // (in box before at, with class cls) and what gives way to it (hide), the
  // label it starts from and the input's limit, placeholder, and name; route
  // is the label route it posts to, saved(label) shows a stored label, and
  // ended() runs once the input is gone.
  function labelHead(sn) {
    var nm = sn.querySelector(".snm"), pen = sn.querySelector(".sren");
    return {
      box: sn, at: nm, cls: "sinp", hide: [nm, pen], route: "label",
      label: sn.getAttribute("data-label") || "", max: +sn.getAttribute("data-max") || 64,
      ph: root.getAttribute("data-rid") || "", aria: pen.getAttribute("aria-label") || "",
      saved: function (label) { labelShow(sn, label); },
      ended: function () {
        if (!headHeld) return;
        var d = headHeld;
        headHeld = null;
        swap("head", d.hash, d.html);
      }
    };
  }
  // labelShow shows label in the head's name (the reply-id when empty), in
  // the label's body face, and points the address the name copies at it.
  function labelShow(sn, label) {
    var nm = sn.querySelector(".snm");
    sn.setAttribute("data-label", label);
    nm.textContent = label || root.getAttribute("data-rid") || "";
    labelCopy(nm, label);
    nm.classList.toggle("lbl", !!label);
  }
  // labelCopy points el's copied address (data-copy) at label: the opening
  // tag the server built (data-mailbox-open) with the label as its body, or
  // closed bare without one.
  function labelCopy(el, label) {
    var open = el.getAttribute("data-mailbox-open");
    el.setAttribute("data-copy", open + (label ? ">" + label.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;") + "</mailbox>" : "/>"));
  }
  function labelStart(ed) {
    if (ed.box.querySelector("." + ed.cls)) return;
    var inp = doc.createElement("input");
    inp.type = "text";
    inp.className = ed.cls;
    inp.value = ed.label;
    inp.placeholder = ed.ph;
    inp.maxLength = ed.max;
    inp.spellcheck = false;
    inp.setAttribute("autocomplete", "off");
    inp.setAttribute("aria-label", ed.aria);
    ed.hide.forEach(function (n) { if (n) n.hidden = true; });
    ed.box.insertBefore(inp, ed.at);
    ed.inp = inp;
    inp.addEventListener("keydown", function (e) { labelKey(ed, inp, e); });
    inp.addEventListener("blur", function () { if (!inp.saving) labelEnd(ed); });
    inp.focus();
    inp.select();
  }
  function labelKey(ed, inp, e) {
    if (e.isComposing || e.keyCode === 229) return;
    if (e.key === "Enter") { e.preventDefault(); labelSave(ed, inp); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); labelEnd(ed); }
  }
  // labelSave posts the input's value; a refusal toasts why and keeps the
  // input open. An unchanged value just closes it.
  function labelSave(ed, inp) {
    if (inp.saving) return;
    var v = labelClean(inp.value, inp.maxLength);
    if (v === ed.label) { labelEnd(ed); return; }
    inp.saving = true;
    post(ed.route, { label: v }, function (ok, ans) {
      inp.saving = false;
      if (!ok) {
        toast(wsT("js.label.fail", ans));
        if (inp.isConnected) inp.focus();
        return;
      }
      ed.saved(typeof ans.label === "string" ? ans.label : v);
      labelEnd(ed);
    });
  }
  // labelEnd closes the input and lands a swap it held (ended). Removing
  // the focused input fires its blur, which comes back here: ending marks
  // the input already on its way out.
  function labelEnd(ed) {
    var inp = ed.inp;
    if (!inp || inp.ending) return;
    inp.ending = true;
    ed.inp = null;
    if (inp.parentNode) inp.parentNode.removeChild(inp);
    ed.hide.forEach(function (n) { if (n) n.hidden = false; });
    ed.ended();
  }
  // labelSwapped retitles the tab after a head swap, from the title the
  // head's name carries.
  function labelSwapped() {
    var h = region("head"), sn = h && h.querySelector(".sname[data-title]");
    if (sn) tlRebase(sn.getAttribute("data-title"));
  }
  doc.addEventListener("click", function (e) {
    var h = region("head");
    if (h && h.contains(e.target)) labelClick(e.target);
  });
  // linkOpens reports whether this page can open a marked link's href: a
  // data-out link only on a data-sysopen page.
  function linkOpens(a) {
    return !!a.getAttribute("href") && (!a.hasAttribute("data-out") || root.hasAttribute("data-sysopen"));
  }
  // desktopShell reports the Werkschaft desktop app, which names itself in
  // its user agent ("Werkschaft/<version>"). Its webview has no tabs, so a
  // modified click on a file link cannot be the browser's new tab there:
  // left to the webview it navigates the clicked frame in place, onto a
  // file view that refuses to be framed. In the app a plain click on a file
  // link opens its menu and a Cmd/Ctrl+click previews (desktopPreview); a
  // browser keeps the behavior below.
  function desktopShell() {
    var n = window.navigator;
    return !!n && /\bWerkschaft\//.test(n.userAgent || "");
  }
  // desktopPreview previews a file link the preview card can show, in the
  // desktop app only, and reports whether it did.
  function desktopPreview(a) {
    if (!desktopShell()) return false;
    if (previews(a)) previewOpen(a, a.getAttribute("data-rel"), false);
    else if (outsideOpens(a)) previewOpen(a, a.getAttribute("data-abs"), true);
    else return false;
    return true;
  }
  // previews reports a file link the preview card (preview.js) can show,
  // decided from what the page already knows: a file inside the session
  // root whose extension is not one no preview shows. What the server then
  // finds binary or too large opens the card in its "not viewable" state.
  function previews(a) {
    var rel = a.getAttribute("data-rel");
    return !!window.WsPreview && !!cardBoard && a.getAttribute("data-wl") === "file" && !!a.getAttribute("href") &&
      !a.hasAttribute("data-out") && !!rel && window.WsPreview.takes(rel);
  }
  // outsideOpens reports a link to a file outside the session root that
  // the preview card can show: a file link with an absolute path and no
  // path under the root, on a data-sysopen page (the outside-link route
  // answers a browser on the daemon's machine only, so any other page keeps
  // the copy-path menu). The daemon decides on the click whether the board
  // or a reply really links the path.
  function outsideOpens(a) {
    var abs = a.getAttribute("data-abs");
    return !!window.WsPreview && !!cardBoard && root.hasAttribute("data-sysopen") && a.getAttribute("data-wl") === "file" &&
      !a.getAttribute("data-rel") && !!abs && window.WsPreview.takes(abs);
  }
  // webLink reports a link a plain click opens in a web card (webcard.js,
  // only on a data-sysopen page): an http(s) link to a localhost page
  // other than this daemon's own port. webFile reports an HTML file under
  // the session root, which the link menu offers to open in a web card.
  function webLink(a) {
    return !!window.WsWeb && !!cardBoard && root.hasAttribute("data-sysopen") && a.getAttribute("data-wl") === "url" &&
      !!a.getAttribute("href") && window.WsWeb.localLink(a.href);
  }
  function webFile(a) {
    var rel = a.getAttribute("data-rel");
    return !!window.WsWeb && !!cardBoard && root.hasAttribute("data-sysopen") && a.getAttribute("data-wl") === "file" &&
      !a.hasAttribute("data-out") && !!rel && /\.html?$/i.test(rel);
  }
  // openOutsideTab opens an outside HTML file as a page in a tab of its own.
  // The tab opens blank inside the click, since the URL comes from a POST
  // (the outside-link route mints its capability) and a tab opened after
  // the answer may be blocked; it is cut from this page before it goes to
  // the file, and closed when the route refuses.
  function openOutsideTab(abs) {
    var w = window.open("", "_blank");
    if (w) w.opener = null;
    post("fs/outside", { path: abs }, function (ok, v) {
      var view = ok && v && typeof v.view === "string" && v.view.indexOf("/s/" + rid + "/fs/oview/") === 0 ? v.view : "";
      if (view && w) w.location.replace(view);
      else if (view) window.open(view, "_blank", "noopener");
      else {
        if (w) w.close();
        toast(wsT("js.preview.gone"));
      }
    });
  }
  function previewOpen(a, path, out) {
    // Frame menus are detached links. Their origin was captured from the
    // source- and document-key-validated host frame, never the message.
    var origin = a.wsPreviewOrigin || (a.closest && a.closest("section.topic[data-topic]"));
    var o = out ? { out: true } : {};
    if (origin) o.origin = origin;
    if (out || origin) window.WsPreview.open(path, o); else window.WsPreview.open(path);
  }
  // hostPath splits abs, an absolute path as the daemon's host spells it
  // (data-abs), into its parent folder and its name, the root and path
  // fs/act takes; null for anything else. A backslash separates only in a
  // drive-letter path, since a POSIX name may hold one.
  function hostPath(abs) {
    if (!abs) return null;
    var win = /^[A-Za-z]:[\\\/]/.test(abs);
    if (!win && abs.charAt(0) !== "/") return null;
    var i = win ? Math.max(abs.lastIndexOf("/"), abs.lastIndexOf("\\")) : abs.lastIndexOf("/");
    var dir = abs.slice(0, i), name = abs.slice(i + 1);
    if (!win && !dir) dir = "/";
    if (win && dir.length === 2) dir += "\\";
    return { dir: dir, name: name };
  }
  function openLinkMenu(a, x, y) {
    closeLinkMenu(false);
    var kind = a.getAttribute("data-wl"), href = a.getAttribute("href");
    var rel = a.getAttribute("data-rel"), abs = a.getAttribute("data-abs"), url = a.href;
    var sys = root.hasAttribute("data-sysopen"), out = a.hasAttribute("data-out");
    var items = [];
    if (webLink(a)) items.push([wsT("js.web.menu.open"), function () { window.WsWeb.open({ url: url }); }]);
    if (previews(a)) items.push([wsT("js.menu.preview"), function () { previewOpen(a, rel, false); }]);
    else if (outsideOpens(a)) {
      items.push([wsT("js.menu.preview"), function () { previewOpen(a, abs, true); }]);
      if (/\.html?$/i.test(abs)) items.push([wsT("js.menu.newtab"), function () { openOutsideTab(abs); }]);
    }
    if (webFile(a)) items.push([wsT("js.web.menu.open"), function () { window.WsWeb.open({ path: rel }); }]);
    if (linkOpens(a)) items.push([wsT("js.menu.newtab"), function () { window.open(url, "_blank", "noopener"); }]);
    // "Open with system" hands the target to the host's default handler: a
    // url link to the system-open route (the default browser), a file or
    // folder (an .app bundle included) to fs/act's open, by its own path.
    var hostAt = kind === "file" && sys ? hostPath(abs) : null;
    if (kind === "url" && href && sys) {
      items.push([wsT("js.menu.sysopen"), function () {
        post("open", { target: url }, function (ok, v) { toast(ok ? wsT("js.toast.opened") : wsT("js.toast.openfail", v)); });
      }]);
    } else if (hostAt) {
      items.push([wsT("js.menu.sysopen"), function () {
        post("fs/act", { path: hostAt.name, action: "open", root: hostAt.dir }, function (ok, v) { toast(ok ? wsT("js.toast.openednative") : wsT("js.toast.openfail", v)); });
      }]);
    }
    if (kind === "url") items.push([wsT("js.menu.copyurl"), function () { copyText(url); }]);
    else {
      if (rel) items.push([wsT("js.menu.copyrel"), function () { copyText(rel); }]);
      if (abs) items.push([wsT("js.menu.copyabs"), function () { copyText(abs); }]);
    }
    if (!items.length) return;
    var m = doc.createElement("div");
    m.className = "lmenu";
    m.setAttribute("role", "menu");
    items.forEach(function (it) {
      var b = doc.createElement("button");
      b.type = "button";
      b.setAttribute("role", "menuitem");
      b.textContent = it[0];
      b.addEventListener("click", function (e) { e.stopPropagation(); closeLinkMenu(true); it[1](); });
      m.appendChild(b);
    });
    doc.body.appendChild(m);
    var w = m.offsetWidth, h = m.offsetHeight;
    m.style.left = Math.max(4, Math.min(x, window.innerWidth - w - 4)) + "px";
    m.style.top = Math.max(4, y + h + 4 > window.innerHeight ? y - h - 4 : y + 4) + "px";
    lmenu = m;
    lmenuFrom = a;
    m.firstChild.focus();
  }
  // linkAt opens the menu for a marked link: at the pointer, or below the
  // link for a keyboard activation.
  function linkAt(a, e) {
    var r = a.getBoundingClientRect();
    if (e.detail === 0 || e.type === "keydown") openLinkMenu(a, r.left, r.bottom);
    else openLinkMenu(a, e.clientX, e.clientY);
  }
  doc.addEventListener("click", function (e) {
    var t = e.target;
    if (lmenu && !(t && t.closest && t.closest(".lmenu"))) closeLinkMenu(false);
    if (e.button !== 0 || !t || !t.closest) return;
    var a = t.closest("a[data-wl]");
    if (!a) return;
    // The desktop app: every click on a file link stays on the page; a
    // pointer Cmd/Ctrl+click previews what the card can show, and any other
    // click, or a file the card cannot show, opens the menu.
    if (a.getAttribute("data-wl") === "file" && desktopShell()) {
      e.preventDefault();
      if (e.detail !== 0 && (e.metaKey || e.ctrlKey) && desktopPreview(a)) return;
      linkAt(a, e);
      return;
    }
    if (linkOpens(a) && (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) return;
    e.preventDefault();
    // A pointer click on a link the preview can show opens the preview
    // slot, and one on a localhost page a web card; their menu is on the
    // right click and on the keyboard activation (a click event with no
    // detail).
    if (e.detail !== 0 && webLink(a)) { window.WsWeb.open({ url: a.href }); return; }
    if (e.detail !== 0 && previews(a)) { previewOpen(a, a.getAttribute("data-rel"), false); return; }
    if (e.detail !== 0 && outsideOpens(a)) { previewOpen(a, a.getAttribute("data-abs"), true); return; }
    linkAt(a, e);
  }, true);
  doc.addEventListener("contextmenu", function (e) {
    var a = e.target && e.target.closest && e.target.closest("a[data-wl]");
    if (!a || !(previews(a) || outsideOpens(a) || webLink(a))) return;
    e.preventDefault();
    openLinkMenu(a, e.clientX, e.clientY);
  });
  // An image outside the session root (img[data-out]) loads only for a
  // browser on the daemon's machine; where it fails (a LAN viewer, a missing
  // file) its alt text and its absolute path, as a copy-only link, take its
  // place, the shape a path the page does not serve takes. The page CSP
  // forbids inline onerror, so a capture listener catches the error, and a
  // sweep after each render catches an image that failed before it: decode()
  // rejects only a broken image, where naturalWidth alone is also 0 for a
  // loaded SVG with no intrinsic size.
  function outImgFailed(img) {
    var abs = img.getAttribute("data-abs") || "";
    var span = doc.createElement("span");
    span.className = "img-text";
    if (img.alt) span.appendChild(doc.createTextNode(img.alt + " "));
    var code = doc.createElement("code");
    code.className = "fpath";
    code.textContent = abs;
    // Inside a link already (an image linked to itself), the path stays
    // text: an anchor in an anchor has no defined click target.
    if (img.parentNode && img.parentNode.closest && img.parentNode.closest("a")) span.appendChild(code);
    else {
      var a = doc.createElement("a");
      a.setAttribute("tabindex", "0");
      a.setAttribute("data-wl", "file");
      a.setAttribute("data-abs", abs);
      a.setAttribute("title", abs);
      a.appendChild(code);
      span.appendChild(a);
    }
    if (img.parentNode) img.parentNode.replaceChild(span, img);
  }
  function sweepOutImgs(el) {
    q("img[data-out]", el).forEach(function (img) {
      if (!img.complete || img.naturalWidth) return;
      if (img.decode) img.decode().then(null, function () { if (img.isConnected) outImgFailed(img); });
      else outImgFailed(img);
    });
  }
  doc.addEventListener("error", function (e) {
    var t = e.target;
    if (t && t.tagName === "IMG" && t.hasAttribute("data-out")) outImgFailed(t);
  }, true);
  sweepOutImgs(doc);
  doc.addEventListener("keydown", function (e) {
    var inMenu = !!lmenu && lmenu.contains(doc.activeElement);
    if (lmenu && e.key === "Escape") { if (inMenu) e.preventDefault(); closeLinkMenu(inMenu); return; }
    if (inMenu && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      var bs = Array.prototype.slice.call(lmenu.children), i = bs.indexOf(doc.activeElement);
      e.preventDefault();
      bs[(i + (e.key === "ArrowDown" ? 1 : bs.length - 1)) % bs.length].focus();
      return;
    }
    // An action link with no href is not natively activatable.
    var a = e.target && e.target.closest && e.target.closest("a[data-wl]:not([href])");
    if (a && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); linkAt(a, e); }
  });
  // A user's scroll closes the menu; the conversation's own pinning to the
  // newest reply scrolls too, and must not.
  doc.addEventListener("wheel", function (e) { if (lmenu && !lmenu.contains(e.target)) closeLinkMenu(false); }, { capture: true, passive: true });
  doc.addEventListener("touchmove", function (e) { if (lmenu && !lmenu.contains(e.target)) closeLinkMenu(false); }, { capture: true, passive: true });
  window.addEventListener("resize", function () { closeLinkMenu(false); });
  window.addEventListener("blur", function () { closeLinkMenu(false); });

