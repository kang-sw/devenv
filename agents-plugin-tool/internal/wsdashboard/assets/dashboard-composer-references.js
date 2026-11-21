// Composer web references and topic note controls.
  // --- web picks: what a web card (webcard.js) attaches -------------------------
  // A picked element and a console block are chips beside the mentions,
  // sent as the message's picks and consoles. The inbox refuses a send over
  // its caps whole (WEB_PICKS picks, WEB_CONSOLES console blocks), so the
  // composer takes no more than that and says so. A cancel or a stored
  // draft brings them back as they were, even past a cap: the send then
  // says why, and nothing is lost.
  var WEB_PICKS = 50, WEB_CONSOLES = 10;
  function addWebPick(p) {
    if (wpicks.length + inlineRows("pick").length >= WEB_PICKS) { noteWarn(wsT("js.web.cap.picks", WEB_PICKS)); return false; }
    if (inlineEditor()) inlineEditor().insertCapsule("pick", { label: p.selector, anchor: "[pick:" + p.selector + "]", plain: p.selector + (p.text ? " " + p.text : ""), pick: p });
    else wpicks = wpicks.concat([p]);
    renderChips();
    queueDraft();
    return true;
  }
  function addWebConsole(c) {
    if (wconsoles.length + inlineRows("console").length >= WEB_CONSOLES) { noteWarn(wsT("js.web.cap.consoles", WEB_CONSOLES)); return false; }
    if (inlineEditor()) inlineEditor().insertCapsule("console", { label: "console ×" + c.lines.length, anchor: "[console:" + c.url + "]", plain: "console ×" + c.lines.length + " " + c.url, console: c });
    else wconsoles = wconsoles.concat([c]);
    renderChips();
    queueDraft();
    return true;
  }
  // Public Pass 2 hooks. The editor remembers the last caret when a board
  // control takes focus; callers pass semantic data, never DOM ranges.
  function addPanelMention(ref) {
    if (!ref || !ref.topic || !/^p[1-9][0-9]*$/.test(ref.panel || "")) return null;
    var label = ref.panel + " · " + (ref.recap || ref.topic);
    var mention = { topic: ref.topic, text: label };
    if (/^e[1-9][0-9]*$/.test(ref.entry || "")) mention.entry = ref.entry;
    var payload = { label: label, anchor: "{{" + ref.panel + "}}", plain: "[" + label + "]", mention: mention,
      panel: ref.panel, entry: ref.entry || "", recap: ref.recap || "" };
    if (inlineEditor()) return inlineEditor().insertCapsule("mention", payload);
    bmentions.push(mention); renderChips(); queueDraft(); return mention;
  }
  function addTicketMention(ref) {
    if (!ref || !/^[0-9]{6}-[a-z0-9][a-z0-9-]*$/.test(ref.stem || "")) return null;
    var payload = { label: "#" + ref.stem, anchor: "[ticket:" + ref.stem + "]", plain: "[" + ref.stem + "] " + (ref.title || ""),
      stem: ref.stem, title: ref.title || "", status: ref.status || "", path: ref.path || "" };
    if (inlineEditor()) return inlineEditor().insertCapsule("ticket", payload);
    ctext.value += (ctext.value ? " " : "") + payload.anchor; queueDraft(); return payload;
  }
  window.WsComposer = Object.assign(window.WsComposer || {}, { addPanelMention: addPanelMention, addTicketMention: addTicketMention });
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
    if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(function () { focusTopicNote(name); })) return;
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
