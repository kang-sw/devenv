  // --- composer: never inside a region, so a swap never touches it ---------
  // With a live host adapter the composer sends: the typed text plus one
  // answer chip per board item. Clicking an option, ok/no, or an
  // acknowledgement on a card attaches (or replaces) that item's chip; the
  // chips live here, so a chip outlives its card leaving the board.
  var ctext = $("ctext"), cchips = $("cchips");
  if (ctext && window.WsCapsules) window.WsCapsules.attach(ctext);
  var chips = {}, chipOrder = [];
  // bmentions are the board mentions the composer holds, in the order added
  // (board mentions, below).
  var bmentions = [];
  // New pushes retain their source, one memo, and image references together.
  // Drafts and requests serialize these views into the existing text field.
  var bselections = [];
  var closeRequests = [];
  function validCloseRequests(list) {
    var out = [];
    (Array.isArray(list) ? list : []).forEach(function (r) {
      if (!r || typeof r.topic !== "string" || !r.topic.trim() || /[\r\n\x00]/.test(r.topic) || typeof r.reason !== "string") return;
      var next = { topic: r.topic.trim(), reason: r.reason }, i = out.findIndex(function (v) { return v.topic === next.topic; });
      if (i < 0) out.push(next); else out[i] = next;
    });
    return out;
  }
  function focusCloseRequest(topic) {
    if (!cchips) return;
    var input = q("[data-close-reason]", cchips).filter(function (el) { return el.getAttribute("data-close-reason") === topic; })[0];
    if (input) input.focus();
  }
  function addCloseRequest(topic) {
    if (!ctext || !cchips || !topic) return;
    if (!closeRequests.some(function (r) { return r.topic === topic; })) {
      closeRequests.push({ topic: topic, reason: "" });
      renderChips();
      queueDraft();
    }
    focusCloseRequest(topic);
  }
  function removeCloseRequest(topic) {
    closeRequests = closeRequests.filter(function (r) { return r.topic !== topic; });
    renderChips();
    queueDraft();
  }
  // wpicks and wconsoles are the web card's picked elements and console
  // blocks the composer holds, in the order added (web picks, below).
  var wpicks = [], wconsoles = [];
  function chipList() { return chipOrder.map(function (id) { return chips[id]; }); }
  function putChip(c) { if (!chips[c.id]) chipOrder.push(c.id); chips[c.id] = c; }
  function dropChip(id) { delete chips[id]; chipOrder = chipOrder.filter(function (x) { return x !== id; }); }
  function boardItem(id) {
    return boardQ('.card[data-id="' + id + '"], .arow[data-id="' + id + '"], .ckrow[data-id="' + id + '"], .srow[data-id="' + id + '"]')[0] || null;
  }
  // readItem is a board item's answer as a chip; null when it has none.
  function readItem(item) {
    var pick = item.querySelector('.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]');
    var box = item.querySelector("label.ack > input, label.ck > input"), note = item.querySelector("textarea.note");
    var c = { id: item.getAttribute("data-id"), answer: "", note: note ? note.value.trim() : "" };
    if (pick) c.answer = pick.getAttribute("data-opt") || pick.getAttribute("data-v");
    else if (item.hasAttribute("data-no")) c.answer = "no";
    else if (box && box.checked) c.answer = "ok";
    return c.answer || c.note ? c : null;
  }
  // setItem puts a chip's answer (or none) on the item's card and lets the
  // board's form script rebuild its reply line.
  function setItem(id, c) {
    var item = boardItem(id);
    if (!item) return;
    q(".opt[data-opt], .btn[data-v]", item).forEach(function (b) {
      var v = b.getAttribute("data-opt") || b.getAttribute("data-v");
      b.setAttribute("aria-pressed", String(!!c && v === c.answer));
    });
    var box = item.querySelector("label.ack > input, label.ck > input"), note = item.querySelector("textarea.note");
    if (box) box.checked = !!c && c.answer === "ok";
    setCheckNo(item, !!c && c.answer === "no");
    if (note) {
      note.value = c && c.note ? c.note : "";
      if (note.value) note.hidden = false;
    }
    var fire = note || box;
    if (fire) fire.dispatchEvent(new Event("change", { bubbles: true }));
  }
  // syncChips takes the answers of the items on the board into the chips.
  function syncChips() {
    if (!region("board") || !cchips) return;
    boardQ(".card[data-id], .arow[data-id], .ckrow[data-id], .srow[data-id]").forEach(function (item) {
      var c = readItem(item), id = item.getAttribute("data-id");
      if (c) putChip(c); else if (chips[id]) dropChip(id);
    });
    renderChips();
  }
  function renderChips() {
    if (!cchips) return;
    var active = doc.activeElement, focused = active && active.getAttribute("data-close-reason");
    var selection = focused != null ? [active.selectionStart, active.selectionEnd] : null;
    cchips.textContent = "";
    chipList().forEach(function (c) {
      var el = doc.createElement("span"), b = doc.createElement("b"), x = doc.createElement("button");
      el.className = "achip";
      b.textContent = "(" + c.id + ")";
      el.appendChild(b);
      el.appendChild(doc.createTextNode(" " + (c.answer || "note")));
      if (c.note) {
        var n = doc.createElement("span");
        n.className = "anote";
        n.textContent = c.note;
        el.appendChild(n);
      }
      x.className = "unchip";
      x.setAttribute("data-unchip", c.id);
      x.title = wsT("js.chip.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    // A topic note is no item's answer, but a send carries it, so it shows
    // here too: "[topic] text", read from the board on every render.
    topicNotes().forEach(function (n) {
      var el = doc.createElement("span"), b = doc.createElement("b"), t = doc.createElement("span"), x = doc.createElement("button");
      el.className = "achip tchip";
      el.setAttribute("data-tchip", n.topic);
      el.title = wsT("js.chip.tnote.open");
      b.textContent = "[" + n.topic + "]";
      el.appendChild(b);
      t.className = "anote";
      t.textContent = n.value.trim();
      el.appendChild(t);
      x.className = "unchip";
      x.setAttribute("data-untchip", n.topic);
      x.title = wsT("js.chip.tnote.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    // A board mention: "[topic] (id)" and the selected text, clipped.
    bmentions.forEach(function (m, i) {
      var el = doc.createElement("span"), open = doc.createElement("button"), b = doc.createElement("b"), t = doc.createElement("span"), x = doc.createElement("button");
      el.className = "achip mchip";
      el.title = m.text;
      open.type = "button";
      open.className = "mchip-open";
      open.addEventListener("click", function () {
        // Use the same session-local navigation as the history's ID chips.
        var topicID = "topic-" + Array.prototype.map.call(new TextEncoder().encode(m.topic), function (n) { return ("0" + n.toString(16)).slice(-2); }).join("");
        effectJump(topicID, m.entry || (m.id ? "item-" + Number(m.id) : topicID));
      });
      b.textContent = "[" + m.topic + "]" + (m.id ? " (" + m.id + ")" : "") + (m.entry ? " " + m.entry : "");
      open.appendChild(b);
      t.className = "anote";
      t.textContent = m.text.replace(/\s+/g, " ");
      open.appendChild(t);
      el.appendChild(open);
      x.type = "button";
      x.className = "unchip";
      x.setAttribute("data-unmchip", String(i));
      x.title = wsT("js.wbmention.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    renderSelectionChips();
    closeRequests.forEach(function (r) {
      var el = doc.createElement("span"), b = doc.createElement("b"), input = doc.createElement("textarea"), x = doc.createElement("button");
      el.className = "achip close-request-chip";
      b.textContent = wsT("js.closerequest.label", r.topic);
      el.appendChild(b);
      input.className = "close-request-reason";
      input.rows = 1;
      input.value = r.reason;
      input.placeholder = wsT("js.closerequest.reason");
      input.setAttribute("aria-label", wsT("js.closerequest.reasonfor", r.topic));
      input.setAttribute("data-close-reason", r.topic);
      input.addEventListener("input", function () { r.reason = input.value; queueDraft(); });
      el.appendChild(input);
      x.type = "button";
      x.className = "unchip";
      x.setAttribute("data-unclose-request", r.topic);
      x.title = wsT("js.closerequest.remove");
      x.setAttribute("aria-label", x.title);
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    // A web card's picked element: its selector and text; a console block:
    // its error count and page. The held message shows the same capsules
    // (dashboard.tmpl achips).
    wpicks.forEach(function (p, i) {
      var el = doc.createElement("span"), b = doc.createElement("b"), x = doc.createElement("button");
      el.className = "achip mchip pchip";
      el.title = p.url + "\n" + p.selector + (p.text ? "\n" + p.text : "");
      b.textContent = p.selector;
      el.appendChild(b);
      if (p.text) {
        var t = doc.createElement("span");
        t.className = "anote";
        t.textContent = p.text;
        el.appendChild(t);
      }
      x.className = "unchip";
      x.setAttribute("data-unwpick", String(i));
      x.title = wsT("js.web.chip.pick.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    wconsoles.forEach(function (c, i) {
      var el = doc.createElement("span"), b = doc.createElement("b"), t = doc.createElement("span"), x = doc.createElement("button");
      el.className = "achip mchip cchip";
      el.title = c.url + "\n" + c.lines.join("\n");
      b.textContent = wsT("js.web.chip.console", c.lines.length);
      el.appendChild(b);
      t.className = "anote";
      t.textContent = c.url;
      el.appendChild(t);
      x.className = "unchip";
      x.setAttribute("data-unwcon", String(i));
      x.title = wsT("js.web.chip.console.remove");
      x.textContent = "×";
      el.appendChild(x);
      cchips.appendChild(el);
    });
    if (focused != null) {
      focusCloseRequest(focused);
      var next = doc.activeElement;
      if (next && next.getAttribute("data-close-reason") === focused) next.setSelectionRange(selection[0], selection[1]);
    }
    if (cimgs) renderImgs();
  }
  function renderSelectionChips() {
    bselections.forEach(function (s, index) {
      var group = doc.createElement("span"), edit = doc.createElement("button"), note = doc.createElement("span"), remove = doc.createElement("button");
      group.className = "selection-bundle achip mchip selection-chip";
      group.setAttribute("role", "group");
      group.setAttribute("aria-label", wsT("js.wbmention.context"));
      edit.type = "button"; edit.className = "mchip-open selection-edit";
      edit.setAttribute("data-selection-edit", String(index));
      edit.title = s.mentions.map(function (m) { return m.text; }).join("\n") + (s.comment ? "\n" + s.comment : "");
      edit.setAttribute("aria-label", wsT("js.wbmention.note") + ": " + edit.title);
      s.mentions.forEach(function (m) {
        var source = doc.createElement("span"), b = doc.createElement("b"), text = doc.createElement("span");
        source.className = "selection-source";
        b.textContent = "[" + m.topic + "]" + (m.id ? " (" + m.id + ")" : "") + (m.entry ? " " + m.entry : "");
        text.className = "anote"; text.textContent = m.text.replace(/\s+/g, " ");
        source.appendChild(b); source.appendChild(text);
        if (m.entry) source.setAttribute("data-wb-entry", m.entry);
        edit.appendChild(source);
      });
      note.className = "selection-note"; note.textContent = "✎";
      selectionNote(s, note);
      edit.appendChild(note);
      remove.type = "button"; remove.className = "unchip";
      remove.setAttribute("data-unselection", String(index));
      remove.title = wsT("js.wbmention.remove"); remove.textContent = "×";
      group.appendChild(edit); group.appendChild(remove);
      s.row = doc.createElement("div"); s.row.className = "selection-images";
      group.appendChild(s.row); cchips.appendChild(group);
    });
  }
  function selectionNote(s, note) {
    note.hidden = !s.comment.trim();
    note.title = s.comment;
    note.setAttribute("aria-label", wsT("js.wbmention.note"));
  }
  function dropSelection(index) {
    var s = bselections[index];
    if (!s) return;
    bselections.splice(index, 1);
    if (mpend && mpend.edit === s) hideMention();
    s.images.slice().forEach(function (n) {
      var it = imgs.filter(function (p) { return p.n === n; })[0];
      if (it) unattach(it);
    });
    renderChips(); queueDraft();
  }
  function editSelection(index) {
    var s = bselections[index];
    if (!s) return;
    hideMention();
    var row = s.row && s.row.parentNode, rect = row ? row.getBoundingClientRect() : { left: 4, bottom: 4 };
    placeMention({ ms: s.mentions, x: rect.left, y: rect.bottom, edit: s, comment: s.comment, images: s.images.slice() });
    lockMention(); mnote.focus();
    mnote.setSelectionRange(mnote.value.length, mnote.value.length);
  }
  // --- web picks: what a web card (webcard.js) attaches -------------------------
  // A picked element and a console block are chips beside the mentions,
  // sent as the message's picks and consoles. The inbox refuses a send over
  // its caps whole (WEB_PICKS picks, WEB_CONSOLES console blocks), so the
  // composer takes no more than that and says so. A cancel or a stored
  // draft brings them back as they were, even past a cap: the send then
  // says why, and nothing is lost.
  var WEB_PICKS = 50, WEB_CONSOLES = 10;
  function addWebPick(p) {
    if (wpicks.length >= WEB_PICKS) { noteWarn(wsT("js.web.cap.picks", WEB_PICKS)); return false; }
    wpicks = wpicks.concat([p]);
    renderChips();
    queueDraft();
    return true;
  }
  function addWebConsole(c) {
    if (wconsoles.length >= WEB_CONSOLES) { noteWarn(wsT("js.web.cap.consoles", WEB_CONSOLES)); return false; }
    wconsoles = wconsoles.concat([c]);
    renderChips();
    queueDraft();
    return true;
  }
  // validPicks and validConsoles keep the well-formed entries of a list a
  // cancel or a stored draft returned.
  function validPicks(list) {
    return (Array.isArray(list) ? list : []).filter(function (p) {
      return p && typeof p.url === "string" && p.url && typeof p.selector === "string" && p.selector && (p.text == null || typeof p.text === "string");
    }).map(function (p) { return { url: p.url, selector: p.selector, text: p.text || "" }; });
  }
  function validConsoles(list) {
    return (Array.isArray(list) ? list : []).filter(function (c) {
      return c && typeof c.url === "string" && c.url && Array.isArray(c.lines);
    }).map(function (c) {
      return { url: c.url, lines: c.lines.filter(function (l) { return typeof l === "string" && l.trim(); }) };
    }).filter(function (c) { return c.lines.length; });
  }
  // topicNote is the board's note for a topic, matched by attribute value so
  // a topic name needs no selector escaping.
  function topicNote(name) {
    return boardQ(".tnote[data-topic]").filter(function (el) { return el.getAttribute("data-topic") === name; })[0] || null;
  }
  // clearTopicNote empties a topic's note through the input path a typed
  // edit takes, so the reply line, the chips, and holdsDraft follow.
  function clearTopicNote(name) {
    var n = topicNote(name);
    if (!n) return;
    n.value = "";
    n.dispatchEvent(new Event("input", { bubbles: true }));
  }
  // closeTopicNotes closes every open topic note, as its pen does: the text
  // stays, and the note's chip or pen opens it again. A focused note is
  // blurred first, so a board swap deferred while it was edited lands.
  function closeTopicNotes() {
    boardQ(".tnote[data-topic]").forEach(function (el) {
      if (el.hidden) return;
      if (doc.activeElement === el || el.wsc && doc.activeElement === el.wsc.editor) el.blur();
      el.hidden = true;
    });
  }
  // focusTopicNote brings a topic's note into view on the board, opening it
  // through the topic's pen when it is closed.
  function focusTopicNote(name) {
    var n = topicNote(name), topic = n && n.closest(".topic");
    if (!n) return;
    if (live && !tabShown("board")) setTab("board");
    if (topic) topic.scrollIntoView({ block: "nearest" });
    var pen = topic && topic.querySelector(".tnote-toggle");
    if (n.hidden && pen) pen.click();
    else n.focus();
  }
  // boardAnswers is the answer block the chips make (answers.js, the
  // grammar the board's copy-answers and the server's delivered message use).
  function boardAnswers() {
    if (cchips) return window.wbAnswers.block(chipList(), []);
    // A keystroke's reply build may still wait for its frame (form.js).
    if (window.wbFlushReply) window.wbFlushReply();
    var b = region("board"), r = b && b.querySelector("#reply-text");
    return r ? r.value.trim() : "";
  }
  // The composer's outcomes show as toasts, so the composer keeps no row
  // for them: noteWarn for a failure or warning, noteOk for a success.
  function noteWarn(text) { if (window.WsToast) window.WsToast.show(text, { kind: "warn" }); }
  function noteOk(text) { if (window.WsToast) window.WsToast.show(text); }
  // grow sizes the message box to its text; an empty box is sized to its
  // wrapped placeholder instead, because an empty textarea's scrollHeight
  // need not count it. The placeholder is measured in a hidden copy of the
  // mention layer (.chl, same font, padding and wrapping), so the two
  // cannot drift. A width change or a placeholder swap grows it again
  // (see growCtext below).
  var cmeasure = null;
  function placeholderHeight() {
    var ph = ctext.getAttribute("placeholder");
    if (!ph || !ctext.parentNode) return 0;
    if (!cmeasure) {
      cmeasure = doc.createElement("div");
      cmeasure.className = "chl chm";
      cmeasure.setAttribute("aria-hidden", "true");
      ctext.parentNode.appendChild(cmeasure);
    }
    cmeasure.style.paddingRight = chl ? chl.style.paddingRight : "";
    cmeasure.textContent = ph;
    return cmeasure.offsetHeight;
  }
  function grow() {
    if (ctext.wsc) {
      var editor = ctext.wsc.editor;
      editor.style.height = "auto";
      editor.style.height = Math.min(160, Math.max(38, editor.scrollHeight)) + "px";
      return;
    }
    ctext.style.height = "auto";
    var h = ctext.scrollHeight;
    if (!ctext.value) h = Math.max(h, placeholderHeight());
    ctext.style.height = Math.min(160, h) + "px";
  }
  // growCtext keeps an empty box fitted to its placeholder: at first render
  // and once the fonts load, after each placeholder swap (setInput,
  // renderCmd call it), and when only the width changes (the observer skips
  // height-only changes, so setting the height cannot loop).
  function growCtext() { if (ctext) grow(); }
  if (ctext) {
    var ctextW = -1;
    if (window.ResizeObserver) new ResizeObserver(function () {
      var w = (ctext.wsc ? ctext.wsc.editor : ctext).offsetWidth;
      if (w === ctextW) return;
      ctextW = w;
      grow();
    }).observe(ctext.wsc ? ctext.wsc.editor : ctext);
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(growCtext);
    growCtext();
  }
  // post sends a write route's JSON body; done(ok, answer) gets the parsed
  // answer, or the server's refusal text. A route is the session's own
  // unless it starts with "/", a daemon-wide route.
  function post(route, body, done) {
    if (!window.fetch) { done(false, wsT("js.post.nobrowser")); return; }
    fetch(route.charAt(0) === "/" ? route : "/s/" + rid + "/" + route, {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    }).then(function (r) {
      return r.text().then(function (s) {
        if (!r.ok) { done(false, s.trim() || "HTTP " + r.status); return; }
        var v = {};
        try { v = JSON.parse(s); } catch (e) { v = {}; }
        done(true, v);
      });
    }, function () { done(false, wsT("js.post.unreachable")); });
  }
  // --- composer images -------------------------------------------------------
  // A pasted or dropped image is a thumbnail chip held in the page and
  // uploaded only with the send, so an abandoned paste never touches disk. A
  // chip a cancel returned has a ref instead of data: the file the daemon
  // already wrote, which a re-send names rather than uploads again.
  //
  // Each image has a number n, and the text names it with an "[Image #n]"
  // marker inserted at the cursor. Marker and chip stay in sync: deleting the
  // marker drops the chip, removing the chip deletes the marker. A dropped
  // image waits in imgTrash, so an undo that brings its marker back brings
  // the chip back too. Numbers are never reused or rewritten within a draft
  // (rewriting text the user is editing shakes it). Each send is its own
  // held message, numbered as the draft numbered it; a fresh draft numbers
  // from 1 again.
  //
  // The board's note pens (item notes and topic notes) take pasted images
  // too: one image list and one number sequence span the composer text and
  // every note, since one send carries them all. An image's chip sits with
  // the field whose text names it: the composer's row, or a row right after
  // that note; a note whose item left the board lives on in its answer chip,
  // and its images show in the composer's row.
  var cimgs = $("cimgs"), imgs = [], imgSeq = 0, imgTrash = {}, holdSync = false, imgLayout = "";
  var IMG_TYPES = { "image/png": 1, "image/jpeg": 1, "image/gif": 1, "image/webp": 1 };
  var IMG_MAX = 8, IMG_BYTES = 10 * 1024 * 1024;
  var MARKER = "\\[Image #([1-9][0-9]{0,5})\\]";
  function attachSrc(name) { return "/s/" + rid + "/attachment/" + encodeURIComponent(name); }
  function marker(n) { return "[Image #" + n + "]"; }
  // markerNums is the set of image numbers text's markers name.
  function markerNums(text) {
    var out = {}, re = new RegExp(MARKER, "g"), m;
    while ((m = re.exec(text))) out[+m[1]] = true;
    return out;
  }
  // boardNotes are the board's note pens: every item note and topic note
  // (a topic note is a textarea.note too).
  function boardNotes() { return boardQ("textarea.note"); }
  // strayChips are the answer chips whose item left the board: their notes
  // still go with the send, so their markers still name images.
  function strayChips() { return chipList().filter(function (c) { return c.note && !boardItem(c.id); }); }
  // allNums is the set of image numbers the markers of the whole draft name:
  // the composer text, every note on the board, and the stray chips' notes.
  function allNums() {
    var text = [ctext.value].concat(boardNotes().map(function (n) { return n.value; }), strayChips().map(function (c) { return c.note; }));
    var nums = markerNums(text.join("\n"));
    bselections.forEach(function (s) { s.images.forEach(function (n) { nums[n] = true; }); });
    if (mpend) (mpend.images || []).forEach(function (n) { nums[n] = true; });
    return nums;
  }
  // nextNum is a number no image, dropped image, or marker of this draft
  // has used.
  function nextNum() {
    Object.keys(allNums()).forEach(function (k) { imgSeq = Math.max(imgSeq, +k); });
    return ++imgSeq;
  }
  // editField replaces [start, end) of a text field's value with s through
  // the browser's editing command, so the undo stack keeps the edit, else
  // through setRangeText; either way an input event follows. A field that
  // cannot take the focus (a closed note) takes setRangeText, so the
  // command never edits whatever else holds the focus.
  function editField(el, start, end, s) {
    if (!(doc.activeElement === el || el.wsc && doc.activeElement === el.wsc.editor)) el.focus();
    el.setSelectionRange(start, end);
    var done = false;
    if (doc.activeElement === el || el.wsc && doc.activeElement === el.wsc.editor) {
      try { done = s ? doc.execCommand("insertText", false, s) : start === end || doc.execCommand("delete"); } catch (e) { done = false; }
    }
    if (!done) {
      el.setRangeText(s, start, end, "end");
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }
  }
  // editText is editField on the composer's text.
  function editText(start, end, s) { editField(ctext, start, end, s); }
  // The explorer has already resolved the path against its active tab root.
  // Use the composer's mention syntax and edit path; this never submits.
  function attachExplorerPath(path) {
    if (!ctext || !window.wsMention) return;
    window.WsCards.dismissFiles();
    var v = ctext.value, a = ctext.selectionStart, b = ctext.selectionEnd;
    var s = window.wsMention.mentionParts(path).text;
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = " " + s;
    if (b >= v.length || !/\s/.test(v.charAt(b))) s += " ";
    editText(a, b, s);
  }
  // insertMarkers puts the markers for nums at el's cursor (the composer's
  // by default), spaced from the text around them.
  function insertMarkers(nums, el) {
    el = el || ctext;
    if (!nums.length) return;
    var v = el.value, a = el.selectionStart, b = el.selectionEnd, s = nums.map(marker).join(" ");
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = " " + s;
    if (b >= v.length || !/\s/.test(v.charAt(b))) s += " ";
    editField(el, a, b, s);
  }
  // deleteMarkers deletes each marker naming n, with the space after it,
  // wherever the draft holds one: the composer, a note, or a stray chip's
  // note. The stray chips go first: each field edit re-syncs the images,
  // which a marker left in a stray chip would bring back.
  function deleteMarkers(n) {
    bselections.forEach(function (s) { s.images = s.images.filter(function (k) { return k !== n; }); });
    if (mpend) mpend.images = (mpend.images || []).filter(function (k) { return k !== n; });
    var src = "\\[Image #" + n + "\\] ?";
    var stray = strayChips().filter(function (c) { return new RegExp(src).test(c.note); });
    stray.forEach(function (c) {
      c.note = c.note.replace(new RegExp(src, "g"), "").trim();
      if (!c.answer && !c.note) dropChip(c.id);
    });
    if (stray.length) renderChips();
    [ctext].concat(boardNotes()).forEach(function (el) {
      var re = new RegExp(src, "g"), v = el.value, spans = [], m;
      while ((m = re.exec(v))) spans.push([m.index, m.index + m[0].length]);
      spans.reverse().forEach(function (sp) { editField(el, sp[0], sp[1], ""); });
    });
  }
  // addFiles attaches image files, their markers going in at el's cursor
  // (the composer's by default).
  function addFiles(files, el) {
    var owner = el === mnote ? mpend : null;
    if (el === mnote && (!owner || owner.rid !== rid)) return;
    if (owner) lockMention();
    var refused = "", added = [];
    files.forEach(function (f) {
      if (!IMG_TYPES[f.type]) { refused = wsT("js.img.type", f.name || wsT("js.img.thefile")); return; }
      if (f.size > IMG_BYTES) { refused = wsT("js.img.big", f.name || wsT("js.img.theimage")); return; }
      if (imgs.length >= IMG_MAX) { refused = wsT("js.img.max", IMG_MAX); return; }
      var it = { name: f.name || "pasted image", url: URL.createObjectURL(f), data: null, ref: "", n: nextNum() };
      imgs.push(it);
      added.push(it.n);
      if (owner) owner.images.push(it.n);
      var rd = new FileReader();
      rd.onload = function () { var v = String(rd.result); it.data = v.slice(v.indexOf(",") + 1); };
      rd.onerror = function () { dropImg(it); release(it); deleteMarkers(it.n); renderImgs(); noteWarn(wsT("js.img.readfail", it.name)); };
      rd.readAsDataURL(f);
    });
    renderImgs();
    if (!owner) insertMarkers(added, el);
    queueDraft();
    if (refused) noteWarn(wsT("js.img.refused", refused));
  }
  function dropImg(it) {
    var i = imgs.indexOf(it);
    if (i >= 0) imgs.splice(i, 1);
  }
  function release(it) { if (!it.ref) URL.revokeObjectURL(it.url); }
  // unattach removes a chip the user took off: its markers go, and it waits
  // in imgTrash for an undo.
  function unattach(it) {
    if (!it) return;
    dropImg(it);
    imgTrash[it.n] = it;
    renderImgs();
    deleteMarkers(it.n);
  }
  // clearTrash forgets the dropped images once no undo can bring them back.
  function clearTrash() {
    Object.keys(imgTrash).forEach(function (k) { release(imgTrash[k]); });
    imgTrash = {};
  }
  // syncMarkers follows the draft's text (allNums): an image whose marker is
  // gone is dropped (into imgTrash), and a dropped one whose marker is back
  // returns. A restore that moves markers between fields (restoreHeld)
  // holds it off until every marker has landed.
  function syncMarkers() {
    if (holdSync || (!imgs.length && !Object.keys(imgTrash).length)) return;
    var have = allNums(), changed = false;
    imgs.slice().forEach(function (it) {
      if (!have[it.n]) { dropImg(it); imgTrash[it.n] = it; changed = true; }
    });
    Object.keys(imgTrash).forEach(function (k) {
      var it = imgTrash[k];
      if (have[it.n]) { delete imgTrash[k]; imgs.push(it); changed = true; }
    });
    if (!changed) return;
    imgs.sort(function (a, b) { return a.n - b.n; });
    renderImgs();
  }
  // imgHomes is each image's field: -1 for the composer, else its index in
  // notes. The composer's text wins a marker both name, and an image no
  // field names (a stray chip's) shows in the composer's row.
  function imgHomes(notes) {
    var own = markerNums(ctext.value), names = notes.map(function (n) { return markerNums(n.value); });
    return imgs.map(function (it) {
      if (mpend && (mpend.images || []).indexOf(it.n) >= 0) return -2;
      for (var s = 0; s < bselections.length; s++) if (bselections[s].images.indexOf(it.n) >= 0) return -3 - s;
      if (own[it.n]) return -1;
      for (var i = 0; i < names.length; i++) if (names[i][it.n]) return i;
      return -1;
    });
  }
  function renderImgs() {
    if (!cimgs) return;
    var notes = boardNotes(), homes = imgHomes(notes), rows = {};
    imgLayout = JSON.stringify(homes);
    cimgs.textContent = "";
    if (mimgs) mimgs.textContent = "";
    bselections.forEach(function (s) { if (s.row) s.row.textContent = ""; });
    boardQ(".nimgs").forEach(function (r) { r.parentNode.removeChild(r); });
    imgs.forEach(function (it, i) {
      var el = doc.createElement("span"), im = doc.createElement("img"), x = doc.createElement("button"), num = doc.createElement("span");
      el.className = "ichip";
      im.src = it.url;
      im.alt = it.name;
      el.title = wsT("js.img.title", it.n, it.name);
      el.appendChild(im);
      num.className = "inum";
      num.textContent = "#" + it.n;
      el.appendChild(num);
      x.className = "unimg";
      x.setAttribute("data-unimg", String(i));
      x.title = wsT("js.img.remove");
      x.textContent = "×";
      el.appendChild(x);
      var h = homes[i];
      if (h === -2) { if (mimgs) mimgs.appendChild(el); return; }
      if (h <= -3) { var group = bselections[-3 - h]; if (group && group.row) group.row.appendChild(el); return; }
      if (h < 0) { cimgs.appendChild(el); return; }
      // A note's row follows the note, so it hides with it (CSS) and
      // leaves with it when a board swap replaces its topic.
      if (!rows[h]) {
        rows[h] = doc.createElement("span");
        rows[h].className = "nimgs";
        var after = notes[h].wsc ? notes[h].wsc.editor : notes[h];
        after.parentNode.insertBefore(rows[h], after.nextSibling);
      }
      rows[h].appendChild(el);
    });
  }
  // followImgs moves the chips after an edit in a note that changed which
  // field names an image, and puts back the rows a board swap took.
  function followImgs() {
    if (!imgs.length && !boardQ(".nimgs").length) return;
    var homes = imgHomes(boardNotes()), rows = boardQ(".nimgs").length;
    var want = {};
    homes.forEach(function (h) { if (h >= 0) want[h] = true; });
    if (JSON.stringify(homes) !== imgLayout || rows !== Object.keys(want).length) renderImgs();
  }
  // imageFiles is the image files a clipboard or drop carries.
  function imageFiles(dt) {
    var out = [];
    if (!dt) return out;
    if (dt.items && dt.items.length) {
      Array.prototype.forEach.call(dt.items, function (item) {
        if (item.kind !== "file") return;
        var f = item.getAsFile();
        if (f) out.push(f);
      });
    } else if (dt.files) {
      out = Array.prototype.slice.call(dt.files);
    }
    return out;
  }
  // pasteImages attaches the images a paste into el carries.
  function pasteImages(e, el) {
    var files = imageFiles(e.clipboardData).filter(function (f) { return f.type.indexOf("image/") === 0; });
    if (!files.length) return;
    // A paste that carries text as well (a copied web page) still pastes
    // its text; a screenshot carries only the image. The markers go in
    // once the paste is done, after any pasted text.
    var types = Array.prototype.slice.call(e.clipboardData.types || []);
    if (types.indexOf("text/plain") < 0) e.preventDefault();
    var owner = el === mnote ? mpend : null;
    setTimeout(function () { if (el !== mnote || owner && owner === mpend && owner.rid === rid) addFiles(files, el); }, 0);
  }
  var composer = $("composer");
  if (ctext && cimgs && composer) {
    ctext.addEventListener("paste", function (e) { pasteImages(e, ctext); });
    // A board note pen pastes into the same draft; one listener on the
    // document covers the notes every board swap brings.
    doc.addEventListener("paste", function (e) {
      var el = e.target;
      if (el && el.tagName === "TEXTAREA" && el.classList.contains("note") && inBoard(el)) pasteImages(e, el);
    });
    var hasFiles = function (e) { return e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0; };
    composer.addEventListener("dragover", function (e) {
      if (!hasFiles(e)) return;
      e.preventDefault();
      composer.classList.add("drop");
    });
    composer.addEventListener("dragleave", function (e) {
      if (!composer.contains(e.relatedTarget)) composer.classList.remove("drop");
    });
    composer.addEventListener("drop", function (e) {
      composer.classList.remove("drop");
      if (!hasFiles(e)) return;
      e.preventDefault();
      ctext.focus();
      addFiles(imageFiles(e.dataTransfer));
    });
  }
  // --- composer file mentions ------------------------------------------------
  // Typing @ opens the file popup (mention.js) on the files route: paths in
  // the session root by fuzzy search, or, for a browser on this machine, an
  // absolute path one directory at a time. A pick is recorded as a mention:
  // its range in the text and the text it inserted. An edit before a mention
  // moves it; an edit inside it, or one that glues text to either end, drops
  // it, so a mention is only ever what a pick inserted, and typed text such
  // as "@mary" is never one. The layer behind the textarea draws each mention
  // as a pill, and a send strips each one's @, so the session reads a plain
  // path (a code span when the path holds spaces). Held messages a cancel
  // returns come back as plain text: they were sent without the @.
  var chl = $("chl"), mentions = [], mentionWas = "", mention = null;
  function liveMention(m, v) {
    var before = v.charAt(m.start - 1), after = v.charAt(m.end);
    return v.slice(m.start, m.end) === m.text && (!before || /\s/.test(before)) && (!after || /\s/.test(after));
  }
  // trackMentions follows an edit, found as the span between the common
  // prefix and suffix of the text before and after it.
  function trackMentions() {
    var v = ctext.value, was = mentionWas, a = 0, s = 0;
    mentionWas = v;
    if (!mentions.length) return;
    while (a < was.length && a < v.length && was.charCodeAt(a) === v.charCodeAt(a)) a++;
    while (s < was.length - a && s < v.length - a && was.charCodeAt(was.length - 1 - s) === v.charCodeAt(v.length - 1 - s)) s++;
    var end = was.length - s, d = v.length - was.length;
    mentions = mentions.filter(function (m) {
      if (end <= m.start) { m.start += d; m.end += d; } else if (a < m.end) return false;
      return liveMention(m, v);
    });
  }
  // shiftMentions moves every mention by d after the text was set whole.
  function shiftMentions(d) {
    var v = ctext.value;
    mentions = mentions.filter(function (m) { m.start += d; m.end += d; return liveMention(m, v); });
    mentionWas = v;
    paintMentions();
  }
  function clearMentions() {
    mentions = [];
    mentionWas = ctext.value;
    paintMentions();
  }
  function paintMentions() {
    if (!chl || !window.wsMention) return;
    if (ctext.wsc) { chl.hidden = true; return; }
    // The textarea's scrollbar narrows its text; the layer narrows alike.
    chl.style.paddingRight = (parseFloat(getComputedStyle(ctext).paddingRight) + ctext.offsetWidth - ctext.clientWidth) + "px";
    window.wsMention.paint(chl, ctext.value, mentions);
    chl.scrollTop = ctext.scrollTop;
  }
  // sendText is the composer's text with each mention's @ stripped.
  function sendText() {
    var v = ctext.value, out = "", at = 0;
    mentions.slice().sort(function (x, y) { return x.start - y.start; }).forEach(function (m) {
      if (m.start < at || !liveMention(m, v)) return;
      out += v.slice(at, m.start) + m.text.slice(1);
      at = m.end;
    });
    return out + v.slice(at);
  }
  // fileCandidates asks the files route for the candidates of q.
  function fileCandidates(q, done) {
    if (!window.fetch) { done([]); return; }
    fetch("/s/" + rid + "/files?q=" + encodeURIComponent(q), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      return r.ok ? r.json() : { items: [] };
    }).then(function (v) { done((v && v.items) || []); }, function () { done([]); });
  }
  if (ctext && rid && window.wsMention) {
    mention = window.wsMention.attach(ctext, {
      label: wsT("js.mention.label"),
      source: fileCandidates,
      insert: editText,
      onPick: function (p) {
        mentions.push({ start: p.start, end: p.end, text: p.text });
        paintMentions();
      }
    });
    ctext.addEventListener("scroll", function () { if (chl) chl.scrollTop = ctext.scrollTop; });
    window.addEventListener("resize", paintMentions);
  }
  // --- composer slash commands ---------------------------------------------
  // A session whose adapter runs slash commands lists them in the composer's
  // cmds region (one <i> each: data-name, data-desc, data-src, and data-layer
  // when the host tells who may invoke the command), which the stream keeps
  // current. A caret-local / token opens their fuzzy picker. Picks are atomic
  // inline capsules: only one at position zero invokes through {command,args};
  // a mid-text capsule sends its literal name, without a slash. The legacy
  // separate selection remains for returned commands holding fixed arguments.
  // A command selection is sent alone:
  // answer chips, topic notes, and board mentions stay in the composer for
  // the next message, and a send with images attached is refused (the
  // markers would read as arguments). Backspace in an empty box, or the
  // capsule's x, removes the selection.
  //
  // A cancel or interrupt that returns a command restores it as the
  // selection (restoreHeld): its arguments go to the message box when nothing
  // else lands there; else they stay on the capsule (cmdSel.args a string),
  // which sends them as they are and leaves the box's text for a later send.
  var ccmd = $("ccmd"), cmdSel = null, slash = null, ctextHint = ctext ? ctext.getAttribute("placeholder") || "" : "";
  function commandList() {
    var r = region("cmds");
    return (r ? q("i[data-name]", r) : []).map(function (el) {
      return { name: el.getAttribute("data-name"), description: el.getAttribute("data-desc") || "", source: el.getAttribute("data-src") || "", layer: el.getAttribute("data-layer") || "" };
    });
  }
  // Fuzzy rows keep stable host order on ties and retain the advertised
  // user/agent layer headings.
  function commandRows(list, q) {
    var rows = window.WsCapsules ? window.WsCapsules.rank(list, q, function (c) { return c.name; }) : list.filter(function (c) { return c.name.toLowerCase().indexOf(q.toLowerCase()) === 0; });
    if (!list.some(function (c) { return c.layer; })) return rows;
    var user = [], agent = [], other = [];
    rows.forEach(function (c) { (c.layer === "user" ? user : c.layer === "agent" ? agent : other).push(c); });
    var out = [];
    [[user, wsT("js.cmd.group.user")], [agent, wsT("js.cmd.group.agent")], [other, wsT("js.cmd.group.rest")]].forEach(function (g) {
      g[0].forEach(function (c) { out.push({ name: c.name, description: c.description, source: c.source, layer: c.layer, group: g[1] }); });
    });
    return out;
  }
  // A slash token is local to the caret; only whitespace may follow it.
  // Sessions without advertised commands never offer an invocation picker.
  function slashToken(v, caret) {
    if (cmdSel || !commandList().length) return null;
    if (window.WsCapsules) return window.WsCapsules.token(v, caret, "/");
    if (!/^\/\S*$/.test(v) || caret < 1) return null;
    return { start: 0, end: v.length, q: v.slice(1, caret) };
  }
  function commandRow(li, c) {
    var name = doc.createElement("span"), desc = doc.createElement("span");
    name.className = "wsm-name";
    name.textContent = "/" + c.name;
    desc.className = "wsm-desc";
    desc.textContent = c.description;
    li.appendChild(name);
    li.appendChild(desc);
    if (c.source) {
      var src = doc.createElement("span");
      src.className = "wsm-src";
      src.textContent = c.source;
      li.appendChild(src);
    }
    li.title = "/" + c.name + (c.description ? " \u2014 " + c.description : "");
  }
  // renderCmd shows the selection: the capsule, its fixed arguments when it
  // holds them, and the message box's hint.
  function renderCmd() {
    if (!ccmd) return;
    var fixed = !!cmdSel && typeof cmdSel.args === "string", name = ccmd.querySelector(".ccmd-name"), args = ccmd.querySelector(".ccmd-args");
    ccmd.hidden = !cmdSel;
    ccmd.parentNode.classList.toggle("hascmd", !!cmdSel);
    if (name) name.textContent = cmdSel ? "/" + cmdSel.name : "";
    if (args) { args.textContent = fixed ? cmdSel.args : ""; args.hidden = !fixed || !cmdSel.args; }
    ctext.setAttribute("placeholder", cmdSel && !fixed ? wsT("js.cmd.args", cmdSel.name) : ctextHint);
    grow();
  }
  // setCommand selects command name; args, when a string, stays on the
  // capsule, and otherwise the message box holds the arguments.
  function setCommand(name, args) {
    cmdSel = { name: name, args: typeof args === "string" ? args : null };
    renderCmd();
  }
  function clearCommand() {
    cmdSel = null;
    renderCmd();
  }
  // uncommand is Backspace in an empty message box: a capsule holding its
  // arguments puts them in the box to edit, an empty one goes.
  function uncommand() {
    var a = cmdSel.args;
    if (typeof a === "string" && a) {
      cmdSel.args = null;
      editText(0, ctext.value.length, a);
      renderCmd();
    } else clearCommand();
  }
  // sendCommand posts the selection; what else the composer holds stays.
  function sendCommand() {
    var sel = cmdSel, fixed = typeof sel.args === "string";
    if (!fixed && imgs.length) { noteWarn(wsT("js.cmd.images")); return; }
    var raw = ctext.value, args = fixed ? sel.args : sendText().trim();
    followConvo();
    sending = true;
    post("message", { command: sel.name, args: args }, function (ok, res) {
      sending = false;
      if (!ok) { noteWarn(wsT("js.send.fail", res)); return; }
      if (cmdSel === sel) clearCommand();
      if (!fixed && ctext.value === raw) { ctext.value = ""; grow(); clearMentions(); }
      clearDraft();
    });
  }
  if (ctext && rid && ccmd && window.wsMention) {
    slash = window.wsMention.attach(ctext, {
      label: wsT("js.cmd.label"),
      max: 500,
      token: slashToken,
      source: function (q, done) { done(commandRows(commandList(), q)); },
      render: commandRow,
      exact: function (c, q) { return c.name === q; },
      choose: function (c, at) {
        if (ctext.wsc) ctext.wsc.insert("skill", c.name, c.name, at);
        else { setCommand(c.name); editText(at.start, at.end, ""); }
        queueDraft();
      }
    });
  }
  function sendInlineCommand(sel) {
    if (imgs.length) { noteWarn(wsT("js.cmd.images")); return; }
    var raw = ctext.value, args = sendText().slice(sel.end).trim();
    followConvo(); sending = true;
    post("message", { command: sel.name, args: args }, function (ok, res) {
      sending = false;
      if (!ok) { noteWarn(wsT("js.send.fail", res)); return; }
      if (ctext.value === raw) { ctext.value = ""; grow(); clearMentions(); }
      clearDraft();
    });
  }
  // Topic-only editors keep their textarea owner and canonical [stem] strings.
  function bindCapsuleNotes() {
    if (!window.WsCapsules) return;
    window.WsCapsules.sweep();
    boardNotes().forEach(function (note) {
      if (note.wscTopicPicker) return;
      note.wscTopicPicker = window.WsCapsules.topicPicker(note, capsuleTopics);
    });
  }
  if (ctext && window.WsCapsules) window.WsCapsules.topicPicker(ctext, capsuleTopics);
  // --- board mentions: selected board text added to the message ------------
  // On the live board, selecting text inside one topic card shows a floating
  // mention button by the selection's end; clicking it adds a mention
  // capsule (renderChips), which a send carries and the delivered message
  // lists after its answer block (wsinbox mentions). A topic body is a
  // sandboxed frame whose selection this page cannot read, so the frame
  // reports it ({wbDoc: "sel"}, doc-frame.js) and the button stands over it.
  var mbtn = null, mnote = null, mimgs = null, mpend = null, mraf = 0, mcomposing = false;
  // mentionOf is the mention a selection makes: the topic card's key, the
  // item ID when both ends lie in one item (or trusted system-event
  // provenance), the optional immutable entry locator, and selected text.
  function mentionOf(topic, startId, endId, text, entry) {
    text = (text || "").trim();
    if (!topic || !text) return null;
    var m = { topic: topic, text: text };
    if (startId && startId === endId) m.id = startId;
    if (entry && /^e[1-9][0-9]*$/.test(entry)) m.entry = entry;
    return m;
  }
  function entryAt(n) {
    var el = selEl(n), entry = el && el.closest && el.closest("[data-entry]");
    return entry && /^e[1-9][0-9]*$/.test(entry.getAttribute("data-entry") || "") ? entry : null;
  }
  function selEl(n) { return n && (n.nodeType === 1 ? n : n.parentNode); }
  // itemIdAt is the ID of the item card n lies in, "" outside every card; the
  // card's element id carries it on every kind (a settled row has no data-id).
  // Lifted check controls use check-<ID>; item-<ID> stays on their summary.
  function itemIdAt(n) {
    var el = selEl(n), entry = entryAt(n), provenance = entry && entry.getAttribute("data-entry-item");
    if (entry) return /^[0-9]{1,6}$/.test(provenance || "") ? provenance : "";
    var item = el && el.closest && el.closest(".card[id], .arow[id], .ckrow[id], .srow[id]");
    var m = item && /^(?:item|check)-([0-9]{1,6})$/.exec(item.getAttribute("id") || "");
    return m ? m[1] : "";
  }
  // endRect is where a range ends on screen: its last line box.
  function endRect(r) {
    var rs = r.getClientRects();
    return rs.length ? rs[rs.length - 1] : r.getBoundingClientRect();
  }
  // Split a cross-entry selection in document order, clipping each part to
  // its selected text nodes. Locator and item provenance come only from
  // host-owned entry wrappers, never authored references or frame messages.
  // A selection outside entries retains the old topic/item-only behavior.
  function selectionMentions(range, ta, tf, text) {
    var entries = boardQ("[data-entry]").filter(function (el) { return range.intersectsNode(el); });
    if (!entries.length) {
      var plain = ta === tf && mentionOf(ta.getAttribute("data-topic"), itemIdAt(range.startContainer), itemIdAt(range.endContainer), text);
      return plain ? [plain] : [];
    }
    var base = range.commonAncestorContainer, parts = [], part = null;
    if (base.nodeType === 3) base = base.parentNode;
    var walk = doc.createTreeWalker(base, 4), node;
    while ((node = walk.nextNode())) {
      var el = selEl(node), topic = el && el.closest("section.topic[data-topic]");
      if (!topic || !inBoard(el) || !range.intersectsNode(node) || el.closest("script, style, template, [hidden]")) continue;
      if (el.getClientRects && !el.getClientRects().length) continue;
      var start = node === range.startContainer ? range.startOffset : 0;
      var end = node === range.endContainer ? range.endOffset : node.nodeValue.length;
      if (end <= start) continue;
      var entry = entryAt(node), id = entry ? entry.getAttribute("data-entry") : "", key = topic.getAttribute("data-topic");
      if (!part || part.topic !== key || part.entry !== id) {
        part = { topic: key, entry: id, text: "", block: null, startId: itemIdAt(node), endId: "" };
        parts.push(part);
      }
      // Accumulate only clipped visible nodes. An enclosing Range.toString()
      // would reintroduce skipped collapsed entries, scripts, or templates.
      var chunk = node.nodeValue.slice(start, end), block = el.closest("p, li, div, summary, article, section, pre, blockquote, h1, h2, h3, h4, h5, h6");
      if (part.text && block !== part.block && !/\s$/.test(part.text) && !/^\s/.test(chunk)) part.text += "\n";
      part.text += chunk;
      part.block = block;
      part.endId = itemIdAt(node);
    }
    return parts.map(function (p) {
      return mentionOf(p.topic, p.startId, p.endId, p.text, p.entry);
    }).filter(Boolean);
  }
  // hostMention is the pending mention(s) of this page's own selection,
  // outside a form field. Entry-spanning selections become ordered parts.
  function hostMention() {
    var sel = window.getSelection ? window.getSelection() : null;
    if (!region("board") || !sel || sel.isCollapsed || !sel.rangeCount) return null;
    var a = selEl(sel.anchorNode), f = selEl(sel.focusNode), act = doc.activeElement;
    if (!a || !f || !inBoard(a) || !inBoard(f)) return null;
    if (act && (/^(TEXTAREA|INPUT)$/.test(act.tagName) || act.isContentEditable) && inBoard(act)) return null;
    var ta = a.closest("section.topic[data-topic]"), tf = f.closest("section.topic[data-topic]");
    if (!ta || !tf) return null;
    var range = sel.getRangeAt(0), mentions = selectionMentions(range, ta, tf, sel.toString());
    if (!mentions.length) return null;
    var r = endRect(range);
    return mentions.length === 1 ? { m: mentions[0], x: r.right, y: r.bottom } : { ms: mentions, x: r.right, y: r.bottom };
  }
  function hideMention(keep, keepImages) {
    var p = mpend;
    mpend = null;
    if (p && !keep) (p.images || []).forEach(function (n) {
      if (p.edit && p.edit.images.indexOf(n) >= 0) return;
      var it = imgs.filter(function (it) { return it.n === n; })[0];
      if (it) { dropImg(it); release(it); }
    });
    mcomposing = false;
    if (mnote) mnote.value = "";
    if (mbtn) mbtn.hidden = true;
    if (cimgs) {
      if (keepImages && p && p.edit && p.edit.row && mimgs) {
        // Blur can occur between pointer-down and click on any image ×.
        // Only move this editor's thumbnails home; rebuilding other rows
        // would detach the pressed control and lose its delegated click.
        while (mimgs.firstChild) p.edit.row.appendChild(mimgs.firstChild);
        imgLayout = JSON.stringify(imgHomes(boardNotes()));
      } else renderImgs();
    }
  }
  // Once the input takes focus, live selection reports may no longer replace
  // this bundle. The copied mentions and owning session outlive DOM updates.
  function lockMention() { if (mpend) mpend.locked = true; }
  // placeMention shows a snapshot beside its selection. An input-held bundle
  // keeps its last screen position if its source disappears.
  function placeMention(p) {
    if (mpend && mpend.locked && p !== mpend) return;
    var x = p.x, y = p.y;
    if (p.frame) {
      if (doc.contains(p.frame)) {
        var fr = p.frame.getBoundingClientRect();
        x += fr.left; y += fr.top;
      } else if (p.locked) { x = p.screenX; y = p.screenY; }
      else { hideMention(); return; }
    }
    if (!mbtn) {
      mbtn = doc.createElement("div");
      mbtn.className = "wbmention";
      mbtn.setAttribute("data-wbmention-pop", "");
      mbtn.setAttribute("role", "group");
      mbtn.setAttribute("aria-label", wsT("js.wbmention.add.title"));
      mnote = doc.createElement("textarea");
      mnote.rows = 2;
      mnote.className = "wbmention-note";
      mnote.placeholder = wsT("js.wbmention.input");
      mnote.setAttribute("aria-label", wsT("js.wbmention.input"));
      var add = doc.createElement("button");
      add.type = "button";
      add.setAttribute("data-wbmention", "");
      add.innerHTML = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M8.5 2.5H2.5v6l5.3 5.3a1 1 0 0 0 1.4 0l4.6-4.6a1 1 0 0 0 0-1.4Z"/><circle cx="5.5" cy="5.5" r=".8"/></svg>';
      add.appendChild(doc.createTextNode(wsT("js.wbmention.add")));
      add.title = wsT("js.wbmention.add.title");
      mbtn.appendChild(mnote);
      mimgs = doc.createElement("div"); mimgs.className = "selection-images";
      mbtn.appendChild(mimgs);
      mbtn.appendChild(add);
      if (window.WsCapsules) window.WsCapsules.topicPicker(mnote, capsuleTopics);
      mnote.addEventListener("paste", function (e) { pasteImages(e, mnote); });
      mbtn.addEventListener("dragover", function (e) {
        if (e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0) e.preventDefault();
      });
      mbtn.addEventListener("drop", function (e) {
        var files = imageFiles(e.dataTransfer);
        if (!files.length) return;
        e.preventDefault(); e.stopPropagation(); addFiles(files, mnote);
      });
      // Capture before focus collapses the host or sandbox selection. Only
      // the action prevents focus; the input must remain normally editable.
      mbtn.addEventListener("mousedown", function (e) { lockMention(); if (e.target === add) e.preventDefault(); });
      mbtn.addEventListener("focusin", lockMention);
      mbtn.addEventListener("compositionstart", function () { mcomposing = true; });
      mbtn.addEventListener("compositionend", function () { mcomposing = false; });
      mbtn.addEventListener("keydown", function (e) {
        if (e.isComposing || e.keyCode === 229 || mcomposing) { if (e.target === add) e.preventDefault(); e.stopPropagation(); return; }
        if (e.key === "Escape") {
          e.preventDefault(); e.stopPropagation();
          if (mpend && mpend.edit) addMention();
          else { hideMention(); ctext.focus(); }
        }
        // Enter edits the note; attachment always uses the explicit action.
      });
      // Saving on blur must not replace the chip DOM under an in-flight
      // pointer press (notably its remove button), or steal the new focus.
      mbtn.addEventListener("focusout", function (e) {
        if (mpend && mpend.edit && !mbtn.contains(e.relatedTarget)) addMention(true);
      });
      doc.body.appendChild(mbtn);
    }
    if (p !== mpend) {
      if (mpend) hideMention();
      p = Object.assign({}, p, { ms: validMentions(p.ms || [p.m]), rid: rid, images: (p.images || []).slice() });
      mnote.value = p.comment || "";
    }
    p.screenX = x; p.screenY = y;
    mpend = p;
    renderImgs();
    mbtn.hidden = false;
    mbtn.querySelector("[data-wbmention]").hidden = !!p.edit;
    var w = mbtn.offsetWidth, h = mbtn.offsetHeight;
    mbtn.style.left = Math.max(4, Math.min(window.innerWidth - w - 4, x + (p.edit ? 0 : 4))) + "px";
    mbtn.style.top = Math.max(4, Math.min(window.innerHeight - h - 4, y + 6)) + "px";
  }
  function syncHostMention() {
    mraf = 0;
    if (mpend && mpend.locked) return;
    var p = hostMention();
    if (p) placeMention(p);
    else if (mpend && !mpend.frame) hideMention();
  }
  function addMention(keepFocus) {
    var p = mpend;
    if (!p || p.rid !== rid || mcomposing) return;
    if (!p.ms.length) return;
    var s = { mentions: validMentions(p.ms), comment: mnote.value, images: p.images.slice() }, index = bselections.indexOf(p.edit);
    if (index >= 0) { s.row = p.edit.row; bselections[index] = s; }
    else bselections.push(s);
    hideMention(true, keepFocus && index >= 0);
    if (p.frame) { if (doc.contains(p.frame)) postDoc(p.frame, { wbDoc: "unsel" }); }
    else if (window.getSelection) window.getSelection().removeAllRanges();
    if (keepFocus && index >= 0 && s.row && s.row.parentNode) {
      var edit = s.row.parentNode.querySelector("[data-selection-edit]");
      edit.title = s.mentions.map(function (m) { return m.text; }).join("\n") + (s.comment ? "\n" + s.comment : "");
      edit.setAttribute("aria-label", wsT("js.wbmention.note") + ": " + edit.title);
      selectionNote(s, edit.querySelector(".selection-note"));
    } else renderChips();
    queueDraft();
    if (!keepFocus) ctext.focus();
  }
  // frameMention takes a body frame's selection report: its text and where
  // its end stands inside the frame.
  function frameMention(f, text, x, y) {
    if (mpend && mpend.locked) return;
    var t = f.closest("section.topic[data-topic]"), entry = f.getAttribute("data-entry") || "";
    var item = f.getAttribute("data-entry-item") || "", parentEntry = entryAt(f.parentNode);
    if (!item && parentEntry) item = parentEntry.getAttribute("data-entry-item") || "";
    if (!/^[0-9]{1,6}$/.test(item)) item = "";
    // Entry identity is the host's iframe attribute, not a claimed eN in
    // the message or inside its sandboxed document.
    var topic = f.getAttribute("data-topic") || (t && t.getAttribute("data-topic"));
    var m = t && topic === t.getAttribute("data-topic") && mentionOf(topic, item, item, text, entry);
    if (m) placeMention({ m: m, frame: f, x: x, y: y });
    else if (mpend && mpend.frame === f) hideMention();
  }
  if (cchips) {
    doc.addEventListener("selectionchange", function () {
      if (!mraf) mraf = requestAnimationFrame(syncHostMention);
    });
    // A press anywhere on this page leaves a frame's selection behind.
    doc.addEventListener("mousedown", function (e) {
      if (mpend && !mpend.locked && mpend.frame && !(e.target.closest && e.target.closest("[data-wbmention-pop]"))) hideMention();
    }, true);
    doc.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !e.isComposing && e.keyCode !== 229 && !mcomposing && mpend) hideMention();
    });
    // The button follows its selection when the board scrolls or the
    // window resizes.
    var follow = function () {
      if (!mpend) return;
      if (mpend.locked || mpend.frame) { placeMention(mpend); return; }
      var p = hostMention();
      if (p) placeMention(p); else hideMention();
    };
    window.addEventListener("scroll", follow, true);
    window.addEventListener("resize", follow);
  }
  var sending = false;
  // topicNotes are the board's non-empty topic notes. A send carries them
  // as its topics, which the delivered message's answer block lists after
  // the item answers; they are no item's answer, so they are no answer
  // chip, and renderChips shows each as a topic-note chip.
  function topicNotes() {
    return boardQ(".tnote[data-topic]").map(function (el) {
      return { el: el, topic: el.getAttribute("data-topic"), value: el.value };
    }).filter(function (n) { return n.value.trim(); });
  }
  function sendMessage() {
    if (sending) return;
    if (cmdSel) { sendCommand(); return; }
    var inline = window.WsCapsules && window.WsCapsules.command(ctext);
    if (inline) { sendInlineCommand(inline); return; }
    var raw = ctext.value.trim(), groups = bselections.slice(), t = selectionPrefix(groups) + sendText().trim(), list = chipList(), notes = topicNotes(), said = bmentions.slice();
    var committed = {};
    groups.forEach(function (s) { s.images.forEach(function (n) { committed[n] = true; }); });
    var pics = imgs.filter(function (p) { return !mpend || (mpend.images || []).indexOf(p.n) < 0 || committed[p.n]; });
    var wp = wpicks.slice(), wc = wconsoles.slice();
    var closing = closeRequests.map(function (r) { return { ref: r, topic: r.topic, reason: r.reason }; });
    if (!t && !list.length && !pics.length && !notes.length && !said.length && !wp.length && !wc.length && !closing.length) { noteWarn(wsT("js.send.empty")); return; }
    if (pics.some(function (p) { return !p.ref && p.data === null; })) { noteWarn(wsT("js.send.reading")); return; }
    var body = { utcOffsetMinutes: -new Date().getTimezoneOffset(), text: t, chips: list, topics: notes.map(function (n) { return { topic: n.topic, note: n.value.trim() }; }), mentions: said };
    if (closing.length) body.closeRequests = closing.map(function (r) { return { topic: r.topic, reason: r.reason }; });
    if (wp.length) body.picks = wp;
    if (wc.length) body.consoles = wc;
    var up = pics.filter(function (p) { return !p.ref; }), refs = pics.filter(function (p) { return !!p.ref; });
    if (up.length) body.images = up.map(function (p) { return { name: p.name, data: p.data, n: p.n }; });
    if (refs.length) {
      body.refs = refs.map(function (p) { return p.ref; });
      body.refNums = refs.map(function (p) { return p.n; });
    }
    // A send arms only receipt matching for its topics, never a blanket
    // exemption for a concurrent external panel rewrite.
    var ownTopics = notes.map(function (n) { return n.topic; });
    list.forEach(function (c) { var item = boardItem(c.id), topic = item && item.closest("section.topic[data-topic]"); if (topic) ownTopics.push(topicKey(topic)); });
    ownTopics.forEach(function (k) { boardOwn[k] = true; });
    followConvo();
    sending = true;
    post("message", body, function (ok, res) {
      sending = false;
      if (!ok) { ownTopics.forEach(function (k) { delete boardOwn[k]; }); noteWarn(wsT("js.send.fail", res)); return; }
      boardFlushViewed(false);
      if (ctext.value.trim() === raw) { ctext.value = ""; grow(); clearMentions(); }
      // Compare by value: a board event during the send, or clearing one
      // card, re-reads the chips into new objects. Drop every sent chip
      // before clearing any card, since clearing one re-syncs the rest.
      var sent = list.filter(function (c) { var cur = chips[c.id]; return cur && cur.answer === c.answer && cur.note === c.note; });
      sent.forEach(function (c) { dropChip(c.id); });
      sent.forEach(function (c) { setItem(c.id, null); });
      // A mention added during the send stays for the next one.
      bmentions = bmentions.filter(function (m) { return said.indexOf(m) < 0; });
      bselections = bselections.filter(function (s) { return groups.indexOf(s) < 0; });
      // Edited, removed/re-added, or newly attached requests belong to the next send.
      closeRequests = closeRequests.filter(function (r) {
        return !closing.some(function (sent) { return sent.ref === r && sent.reason === r.reason; });
      });
      wpicks = wpicks.filter(function (p) { return wp.indexOf(p) < 0; });
      wconsoles = wconsoles.filter(function (c) { return wc.indexOf(c) < 0; });
      // A topic note edited during the send stays for the next one.
      notes.forEach(function (n) {
        if (n.el.value !== n.value) return;
        n.el.value = "";
        n.el.dispatchEvent(new Event("input", { bubbles: true }));
      });
      // A send closes the open notes; one edited during it keeps its text.
      closeTopicNotes();
      var remaining = allNums();
      pics.forEach(function (p) { if (!remaining[p.n]) { dropImg(p); release(p); } });
      clearTrash();
      renderChips();
      // A fresh draft numbers its images from 1 again: nothing is left that
      // a marker or an image could name.
      if (!imgs.length && !ctext.value && !chipList().length && !topicNotes().length) imgSeq = 0;
      renderImgs();
      // The sent draft is gone; what was typed during the send is saved
      // again as the next draft.
      // No toast on success: the held bubble in the conversation is the
      // confirmation.
      clearDraft();
    });
  }
  // validMentions keeps the well-formed mentions of a list a cancel or a
  // stored draft returned.
  function validMentions(list) {
    return (Array.isArray(list) ? list : []).filter(function (m) {
      return m && typeof m.topic === "string" && m.topic && typeof m.text === "string" && m.text &&
        (!m.id || /^[0-9]{1,6}$/.test(String(m.id))) &&
        (m.entry == null || m.entry === "" || typeof m.entry === "string" && /^e[1-9][0-9]*$/.test(m.entry));
    }).map(function (m) {
      var out = m.id ? { topic: m.topic, id: String(m.id), text: m.text } : { topic: m.topic, text: m.text };
      if (m.entry) out.entry = m.entry;
      return out;
    });
  }
  // restoreHeld puts held messages a cancel or an interrupt took back into
  // the composer, in the order given (an interrupt's queue order: steered
  // first, then the rest, oldest first), ahead of the draft: their texts joined as
  // paragraphs, their answer chips, their mentions and web picks, their topic notes back
  // on the board's topics (restoreTopics), and their images as chips that
  // reference the written files, with their markers in the returned text
  // and notes. Each message's numbers move above the highest the draft and
  // the messages before it use, the markers in its text, chip notes, and
  // topic notes with them, so the restored draft never names a number
  // twice. An image no
  // restored text names (a message held before images were numbered, or a
  // note a newer one replaced) gets a marker in the composer, so the next
  // edit does not drop it.
  //
  // A returned command becomes the composer's selection (see the slash
  // commands section). Only one fits: a second, or one returned while
  // another is selected, comes back as its typed line, with a warning.
  function restoreHeld(list) {
    holdSync = true;
    try { restoreHeldNow(list); } finally { holdSync = false; }
    syncMarkers();
    renderImgs();
  }
  function restoreHeldNow(list) {
    var cmd = null, astext = 0;
    list = list.map(function (res) {
      var c = res.command;
      if (!c || !c.name) return res;
      if (!cmdSel && !cmd) { cmd = c; return {}; }
      astext++;
      return { text: "/" + c.name + (c.args ? " " + c.args : "") };
    });
    var cur = ctext.value.trim(), top = imgSeq, backs = [], back, restoredSelections = [];
    // The draft's mentions move with it, past its leading whitespace's trim.
    var lead = ctext.value.length - ctext.value.replace(/^\s+/, "").length;
    Object.keys(allNums()).forEach(function (k) { top = Math.max(top, +k); });
    imgs.forEach(function (p) { top = Math.max(top, p.n); });
    // The newest send's answer wins (its sentAt; a later message on a tie),
    // whatever the list's queue order, and a chip attached since keeps its
    // newer answer.
    var got = {}, gotAt = {}, order = [];
    list.forEach(function (res) {
      var at = +res.sentAt || 0;
      var names = res.images || [], nums = res.nums || [], renum = {}, offset = top;
      names.forEach(function (name, i) {
        var n = nums[i] || i + 1, have = imgs.filter(function (p) { return p.ref === name; })[0];
        renum[n] = have ? have.n : n + offset;
        if (!have) imgs.push({ name: name, url: attachSrc(name), data: null, ref: name, n: renum[n] });
        top = Math.max(top, renum[n]);
      });
      var rn = function (s) { return (s || "").replace(new RegExp(MARKER, "g"), function (m, k) { return renum[+k] ? marker(renum[+k]) : m; }); };
      var split = splitSelectionText(res.text || ""), text = rn(split.text);
      split.groups.forEach(function (s) { s.images = s.images.map(function (n) { return renum[n] || n; }); restoredSelections.push(s); });
      (res.chips || []).forEach(function (c) {
        var o = {};
        Object.keys(c).forEach(function (k) { o[k] = c[k]; });
        if (c.note) o.note = rn(c.note);
        if (!got[c.id]) order.push(c.id);
        else if (at < gotAt[c.id]) return;
        got[c.id] = o;
        gotAt[c.id] = at;
      });
      var stray = restoreTopics((res.topics || []).map(function (tn) { return { topic: tn.topic, note: rn(tn.note) }; }));
      if (stray.length) text += (text ? "\n\n" : "") + stray.join("\n");
      if (text) backs.push(text);
    });
    imgSeq = top;
    order.forEach(function (id) { if (!chips[id]) { putChip(got[id]); setItem(id, got[id]); } });
    back = backs.join("\n\n");
    bselections = restoredSelections.concat(bselections);
    var named = allNums(), inBack = markerNums(back), missing = [];
    imgs.forEach(function (p) { if (!named[p.n] && !inBack[p.n]) missing.push(marker(p.n)); });
    if (missing.length) back += (back ? "\n" : "") + missing.join(" ");
    ctext.value = back + (back && cur ? "\n\n" : "") + cur;
    grow();
    shiftMentions(back.length + (back && cur ? 2 : 0) - lead);
    imgs.sort(function (a, b) { return a.n - b.n; });
    // Their mentions go back ahead of the draft's, in the order given.
    var held = [];
    list.forEach(function (res) { held = held.concat(validMentions(res.mentions)); });
    bmentions = held.concat(bmentions);
    var requests = [];
    list.slice().sort(function (a, b) { return (+a.sentAt || 0) - (+b.sentAt || 0); }).forEach(function (res) { requests = requests.concat(validCloseRequests(res.closeRequests)); });
    requests = validCloseRequests(requests);
    // Keep current capsules intact: an in-flight send compares their identity.
    closeRequests.forEach(function (r) {
      var i = requests.findIndex(function (v) { return v.topic === r.topic; });
      if (i < 0) requests.push(r); else requests[i] = r;
    });
    closeRequests = requests;
    // So do their picked elements and console blocks.
    var hp = [], hc = [];
    list.forEach(function (res) { hp = hp.concat(validPicks(res.picks)); hc = hc.concat(validConsoles(res.consoles)); });
    wpicks = hp.concat(wpicks);
    wconsoles = hc.concat(wconsoles);
    renderChips();
    if (cmd) {
      if (ctext.value.trim()) setCommand(cmd.name, cmd.args || "");
      else { ctext.value = cmd.args || ""; grow(); clearMentions(); setCommand(cmd.name); }
    }
    if (astext) noteWarn(wsT("js.cmd.astext"));
  }
  // restoreTopics puts returned topic notes back on their topics' notes,
  // after any text a note holds now, through the input path a typed edit
  // takes; it returns a "[topic] note" line for each topic no longer on the
  // board, which the caller keeps in the composer text so no note is lost.
  function restoreTopics(list) {
    var stray = [];
    list.forEach(function (tn) {
      var n = topicNote(tn.topic), note = (tn.note || "").trim();
      if (!note) return;
      if (!n) { stray.push("[" + tn.topic + "] " + note); return; }
      var cur = n.value.trim();
      if (cur === note || cur.indexOf(note) >= 0) return;
      openNote(n, { note: cur ? cur + "\n" + note : note });
      n.dispatchEvent(new Event("input", { bubbles: true }));
    });
    return stray;
  }
  function cancelHeld(btn) {
    btn.disabled = true;
    post("cancel", { id: btn.getAttribute("data-cancel") }, function (ok, res) {
      if (!ok) { btn.disabled = false; noteWarn(wsT("js.cancel.fail", res)); return; }
      restoreHeld([res]);
      noteOk(wsT("js.cancel.ok"));
      ctext.focus();
    });
  }
  // --- interrupt badge ---------------------------------------------------------
  // The badge on the border between the conversation and the composer shows
  // while the session has a live adapter and its lead works on a turn (the
  // convlive region's data-phase). One click stops the turn and takes every
  // held message back into the composer. It then stays disabled until that
  // turn ends, so a second click cannot cut the next turn: until the phase
  // leaves the working set or the region's data-turn (the server's
  // turn-start stamp) changes from the one seen at the click. The stamp is
  // what catches a next turn that starts at once (held input or mail
  // delivered at turn end), which no page swap ever shows as idle.
  var ibadge = $("ibadge"), ilabel = ibadge && ibadge.querySelector(".ilabel");
  var WORKING = { thinking: 1, replying: 1, tool: 1, permission: 1 };
  var liveMode = mode === "live", interrupting = false, interruptedTurn = "";
  function leadWorking() {
    var c = doc.querySelector("#r-convlive .clive");
    return !!c && WORKING[c.getAttribute("data-phase")] === 1;
  }
  function leadTurn() {
    var c = doc.querySelector("#r-convlive .clive");
    return (c && c.getAttribute("data-turn")) || "";
  }
  function syncBadge() {
    subBadge(sbadge, doc.querySelector("#r-convlive .clive"), liveMode);
    bgBadge(doc.querySelector("#r-convlive .clive"));
    if (!ibadge) return;
    var on = liveMode && leadWorking();
    if (!on || leadTurn() !== interruptedTurn) interrupting = false;
    ibadge.hidden = !on;
    ibadge.disabled = interrupting;
    ibadge.classList.toggle("busy", interrupting);
    var label = wsT(interrupting ? "js.interrupt.busy" : "js.interrupt");
    if (ilabel && ilabel.textContent !== label) ilabel.textContent = label;
  }
  // --- subagent badge --------------------------------------------------------
  // Beside the interrupt badge: a spinner, the robot icon, and "N(+M)"
  // (the word "subagents" is the title and accessible name) while the session runs subagents, whether or not its lead
  // works. N is a convlive region's data-sub (its running direct children),
  // M its data-subdeep (every running agent below them), counted by the
  // server; "(+M)" shows only when M is above zero. on gates it: the
  // session page's live mode, or the support chat card's open conversation.
  var sbadge = $("sbadge");
  function subBadge(badge, clive, on) {
    if (!badge) return;
    var n = clive ? Number(clive.getAttribute("data-sub")) || 0 : 0;
    var m = clive ? Number(clive.getAttribute("data-subdeep")) || 0 : 0;
    badge.hidden = !(on && n + m > 0);
    if (badge.hidden) return;
    var count = m > 0 ? n + "(+" + m + ")" : String(n);
    var label = wsTn("js.subagents", n + m, count);
    var el = badge.querySelector(".slabel");
    if (el && el.textContent !== count) el.textContent = count;
    // The word moves to the accessible name and tooltip, like the clock badge.
    if (badge.getAttribute("aria-label") !== label) {
      if (badge.dataset.tip === undefined) badge.dataset.tip = badge.title;
      badge.setAttribute("aria-label", label);
      badge.title = label + (badge.dataset.tip ? " · " + badge.dataset.tip : "");
    }
  }
  // --- background-task badge ---------------------------------------------------
  // Separate harness-owned background and monitor pills follow subagents.
  // The live region carries counts for terminal and hosted sessions alike;
  // a missing count or a zero leaves its pill hidden.
  var bgbadge = $("bgbadge"), bgTitle = bgbadge ? bgbadge.title : "";
  var monbadge = $("monbadge"), monTitle = monbadge ? monbadge.title : "";
  function taskBadge(badge, clive, attr, title) {
    if (!badge) return;
    var n = clive ? Number(clive.getAttribute(attr)) || 0 : 0;
    badge.hidden = !(liveMode && n > 0);
    if (badge.hidden) return;
    var label = attr === "data-bg" ? wsTn("js.bgtasks", n) : wsTn("js.monitors", n);
    var el = badge.querySelector(".blabel");
    if (el && el.textContent !== label) el.textContent = label;
    if (badge.getAttribute("aria-label") !== label) {
      badge.setAttribute("aria-label", label);
      badge.title = label + (title ? " \u00b7 " + title : "");
    }
  }
  function bgBadge(clive) {
    taskBadge(bgbadge, clive, "data-bg", bgTitle);
    taskBadge(monbadge, clive, "data-monitors", monTitle);
  }
  function interruptTurn() {
    if (interrupting) return;
    interrupting = true;
    interruptedTurn = leadTurn();
    syncBadge();
    post("interrupt", {}, function (ok, res) {
      if (!ok) { interrupting = false; syncBadge(); noteWarn(wsT("js.interrupt.fail", res)); return; }
      var list = res.held || [];
      if (!list.length) { noteOk(wsT("js.interrupt.ok")); return; }
      restoreHeld(list);
      noteOk(wsT("js.interrupt.restored"));
      ctext.focus();
    });
  }
  // steerHeld promotes a held message to steering: the session receives it
  // at its next tool call instead of when its turn ends. The convlive
  // region's next swap shows the steer marker in place of the button.
  function steerHeld(btn) {
    btn.disabled = true;
    post("steer", { id: btn.getAttribute("data-steer") }, function (ok, res) {
      if (!ok) { btn.disabled = false; noteWarn(wsT("js.steer.fail", res)); return; }
      noteOk(wsT("js.steer.ok"));
    });
  }
  // applySent marks each board item the session was sent an answer for, or
  // holds one for, from the inbox records the conversation region carries,
  // and dims the item's answer area (sent-sent, sent-held) until the board
  // re-renders it answered; each run recomputes both from scratch. A run
  // over the same records as the last (sentSig) leaves the board alone unless
  // a board swap asks (force): putting back the same classes would only wake
  // the open-question pill's class observer.
  var sentSig = null;
  function applySent(force) {
    var b = region("board");
    if (!b) return;
    var sig = q(".sentdata i[data-id]").map(function (i) {
      return [i.getAttribute("data-id"), i.getAttribute("data-state"), i.getAttribute("data-answer"), i.getAttribute("data-note")].join("\u0001");
    }).join("\u0002");
    if (!force && sig === sentSig) return;
    sentSig = sig;
    boardQ(".sentmark").forEach(function (m) { m.parentNode.removeChild(m); });
    boardQ(".sent-sent, .sent-held").forEach(function (el) { el.classList.remove("sent-sent", "sent-held"); });
    q(".sentdata i[data-id]").forEach(function (i) {
      var item = boardItem(i.getAttribute("data-id")), head = item && item.querySelector(".head, .stext");
      if (!head) return;
      var state = i.getAttribute("data-state"), m = doc.createElement("span");
      item.classList.add(state === "held" ? "sent-held" : "sent-sent");
      m.className = "sentmark " + state;
      m.textContent = state + " · " + (i.getAttribute("data-answer") || "note");
      var note = i.getAttribute("data-note");
      m.title = wsT(state === "held" ? "js.sent.held.title" : "js.sent.sent.title") + (note ? ": " + note : "");
      var id = head.querySelector(".id");
      head.insertBefore(m, id ? id.nextSibling : head.firstChild);
    });
  }
  // indentLines indents by two spaces: at the cursor, or every line a
  // multi-line selection touches (blank lines stay blank). out removes up to
  // two leading spaces from each line the cursor or selection touches
  // instead. The selection follows the lines it covered.
  function indentLines(out) {
    var v = ctext.value, a = ctext.selectionStart, b = ctext.selectionEnd;
    var multi = v.slice(a, b).indexOf("\n") >= 0;
    if (!out && !multi) { editText(a, b, "  "); return; }
    var last = b > a && v.charAt(b - 1) === "\n" ? b - 1 : b;
    var ls = a > 0 ? v.lastIndexOf("\n", a - 1) + 1 : 0, le = v.indexOf("\n", last);
    if (le < 0) le = v.length;
    var first = 0, total = 0;
    var lines = v.slice(ls, le).split("\n").map(function (line, i) {
      var d = 0;
      if (out) { d = -/^ {0,2}/.exec(line)[0].length; line = line.slice(-d); }
      else if (line) { d = 2; line = "  " + line; }
      if (i === 0) first = d;
      total += d;
      return line;
    });
    if (!total) return;
    editText(ls, le, lines.join("\n"));
    // A selection that starts at its line's start keeps starting there.
    var na = a === ls ? ls : Math.max(ls, a + first);
    ctext.setSelectionRange(na, a === b ? na : Math.max(na, b + total));
  }
  if (ctext) {
    var csend = $("csend");
    if (csend) csend.addEventListener("click", function () { sendMessage(); ctext.focus(); });
    ctext.addEventListener("input", function () { grow(); syncMarkers(); trackMentions(); paintMentions(); });
    // Tab indents rather than leaving the composer; Escape, then Tab,
    // leaves it (the keyboard way out).
    var tabOut = false;
    ctext.addEventListener("blur", function () { tabOut = false; });
    ctext.addEventListener("keydown", function (e) {
      // A key that confirms an IME composition (Korean, Japanese, Chinese)
      // must never send or indent: browsers report it as isComposing, and
      // Safari reports the first keydown after compositionend as keyCode 229.
      if (e.defaultPrevented || e.isComposing || e.keyCode === 229) return;
      // While the file popup shows, it takes Up/Down, Tab and Enter (which
      // pick, never send), and Escape (which closes it without arming
      // Tab-out).
      if ((mention && mention.key(e)) || (slash && slash.key(e))) { tabOut = false; return; }
      if (e.key === "Backspace" && cmdSel && !ctext.value) { e.preventDefault(); uncommand(); queueDraft(); return; }
      if (e.key === "Tab") {
        if (e.ctrlKey || e.metaKey || e.altKey) return;
        if (tabOut) { tabOut = false; return; }
        e.preventDefault();
        indentLines(e.shiftKey);
        return;
      }
      if (e.key === "Escape") { tabOut = true; return; }
      if (e.key !== "Shift") tabOut = false;
      if (e.key !== "Enter") return;
      var mod = e.ctrlKey || e.metaKey;
      if (inputMode === "enter" ? !e.shiftKey && !e.altKey : mod) { e.preventDefault(); sendMessage(); }
    });
    // The form script's own listeners have rebuilt the reply line by the
    // time an event bubbles here.
    ["input", "change"].forEach(function (type) {
      doc.addEventListener(type, function (e) {
        if (!inBoard(e.target)) return;
        syncChips();
        syncMarkers();
        followImgs();
      });
    });
  }

