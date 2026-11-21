// Composer restoration of held messages.
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
    restoringInlineBack = true;
    try { restoreHeldNow(list); } finally { restoringInlineBack = false; holdSync = false; }
    if (inlineEditor()) {
      chipOrder.forEach(function (id) { if (!inlineRows("answer").some(function (r) { return r.key === id; })) putChip(chips[id]); });
      syncInlineTopics();
      bselections.forEach(function (s) { if (!inlineRows("mention").some(function (r) { return r.key === s.key; })) putSelectionCapsule(s, false); });
      bmentions.forEach(function (m) { inlineEditor().insertCapsule("mention", { label: "[" + m.topic + "] " + m.text.replace(/\s+/g, " ").slice(0, 38), anchor: "[mention:" + m.topic + "]", plain: "[" + m.topic + "] " + m.text, mention: m }); });
      bmentions = [];
      wpicks.forEach(function (p) { inlineEditor().insertCapsule("pick", { label: p.selector, anchor: "[pick:" + p.selector + "]", plain: p.selector + " " + (p.text || ""), pick: p }); });
      wpicks = [];
      wconsoles.forEach(function (c) { inlineEditor().insertCapsule("console", { label: "console ×" + c.lines.length, anchor: "[console:" + c.url + "]", plain: "console ×" + c.lines.length + " " + c.url, console: c }); });
      wconsoles = [];
    }
    syncMarkers();
    renderImgs();
  }
  function restoreHeldNow(list) {
    var cmd = null, astext = 0;
    list = list.map(function (res) {
      if (typeof boardIncarnation === "string" && +res.sentAt < Date.parse(boardIncarnation)) {
        // A cancelled pre-load send must not reattach its reused IDs.
        res = Object.assign({}, res, { chips: [], topics: [], mentions: [], closeRequests: [] });
        noteWarn(wsT("js.send.fail", "board-replaced"));
      }
      var c = res.command;
      if (!c || !c.name) return res;
      if (!cmdSel && !cmd) { cmd = c; return {}; }
      astext++;
      return { text: "/" + c.name + (c.args ? " " + c.args : "") };
    });
    var cur = ctext.value.trim(), top = imgSeq, backs = [], back, restoredSelections = [], backRanges = [];
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
      var split = splitSelectionText(res.inline && inlineEditor() ? res.inline.text : res.text || ""), text = rn(split.text);
      if (res.inline && inlineEditor()) {
        var offset = backs.join("\n\n").length + (backs.length ? 2 : 0);
        var rr = window.WsCapsules.shifted(res.inline.text, text, res.inline.capsules || []);
        rr.forEach(function (r) { backRanges.push(Object.assign({}, r, { start: r.start + offset, end: r.end + offset })); });
        (res.inline.selections || []).forEach(function (s) { restoredSelections.push(s); });
      }
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
    if (inlineEditor() && backRanges.length) inlineEditor().restore(backRanges.concat(inlineEditor().snapshot()));
    grow();
    shiftMentions(back.length + (back && cur ? 2 : 0) - lead);
    imgs.sort(function (a, b) { return a.n - b.n; });
    // Their mentions go back ahead of the draft's, in the order given.
    var held = [];
    list.forEach(function (res) { if (!res.inline) held = held.concat(validMentions(res.mentions)); });
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
    list.forEach(function (res) { if (!res.inline) { hp = hp.concat(validPicks(res.picks)); hc = hc.concat(validConsoles(res.consoles)); } });
    wpicks = hp.concat(wpicks);
    wconsoles = hc.concat(wconsoles);
    renderChips();
    if (cmd) {
      if (ctext.value.trim()) setCommand(cmd.name, cmd.args || "");
      else if (ctext.wsc) restoreInlineCommand(cmd);
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
  // restoreInlineCommand writes a returned command into the empty box as a
  // fresh pick leaves it: the skill capsule at offset zero, one space, then
  // its arguments, with the caret at the end.
  function restoreInlineCommand(cmd) {
    var v = cmd.name + " " + (cmd.args || "");
    ctext.value = v;
    ctext.wsc.restore([{ start: 0, end: cmd.name.length, kind: "skill", id: cmd.name, label: cmd.name }]);
    ctext.setSelectionRange(v.length, v.length);
    grow(); clearMentions();
  }
  function cancelHeld(btn) {
    btn.disabled = true;
    cancelEntry(btn.getAttribute("data-cancel"), function () { btn.disabled = false; });
  }
