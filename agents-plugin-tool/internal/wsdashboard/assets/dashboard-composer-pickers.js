// Composer file mention and command pickers.
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
  // A cancel or interrupt that returns a command restores it (restoreHeld):
  // when nothing else lands in the message box, as a fresh pick's inline
  // capsule at offset zero followed by its arguments; else as the separate
  // selection holding them (cmdSel.args a string), which sends them as they
  // are and leaves the box's text for a later send.
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
