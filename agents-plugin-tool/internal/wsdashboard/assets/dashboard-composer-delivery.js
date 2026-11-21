// Composer sends and outbox delivery.
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
  function inlineSend() {
    var edit = inlineEditor(), rows = edit.ordered(), raw = ctext.value, notes = rows.filter(function (r) { return r.kind === "topic-note"; }).map(function (r) {
      return { el: topicNote(r.key), topic: r.key, value: r.payload.note };
    }), said = [], picks = [], consoles = [], usedSelections = {}, at = 0, text = "";
    var hasProse = false;
    rows.forEach(function (r) { if (raw.slice(at, r.start).trim()) hasProse = true; at = r.end; });
    if (raw.slice(at).trim()) hasProse = true;
    at = 0;
    rows.forEach(function (r) {
      text += raw.slice(at, r.start);
      var p = r.payload || {}, anchor = raw.slice(r.start, r.end);
      if (r.kind === "answer" || r.kind === "topic-note") {
        if (hasProse || rows.some(function (x) { return x.kind !== "answer" && x.kind !== "topic-note"; })) text += anchor;
      } else if (r.kind === "mention" && p.selectionKey) {
        var s = bselections.filter(function (x) { return x.key === p.selectionKey; })[0] || p.selection;
        if (s) { text += "\n\n" + selectionBlock(s) + "\n\n"; usedSelections[p.selectionKey] = true; }
      } else {
        text += anchor;
        if (r.kind === "mention" && p.mention) said.push(p.mention);
        if (r.kind === "pick" && p.pick) picks.push(p.pick);
        if (r.kind === "console" && p.console) consoles.push(p.console);
      }
      at = r.end;
    });
    text += raw.slice(at);
    var legacy = bselections.filter(function (s) { return !usedSelections[s.key]; });
    return { text: selectionPrefix(legacy) + text.trim(), notes: notes, mentions: said.concat(bmentions), picks: picks.concat(wpicks), consoles: consoles.concat(wconsoles) };
  }
  function sendMessage() {
    if (sending) return;
    var inline = !cmdSel && window.WsCapsules && window.WsCapsules.command(ctext);
    // A command waits until the queued messages before it are held, so the
    // inbox keeps the order typed.
    if ((cmdSel || inline) && outboxUnsent()) return;
    if (cmdSel) { sendCommand(); return; }
    if (inline) { sendInlineCommand(inline); return; }
    var groups = bselections.slice(), inlineData = inlineEditor() ? inlineSend() : null;
    var t = inlineData ? inlineData.text : selectionPrefix(groups) + sendText().trim(), list = chipList(), notes = inlineData ? inlineData.notes : topicNotes(), said = inlineData ? inlineData.mentions : bmentions.slice();
    var committed = {};
    groups.forEach(function (s) { s.images.forEach(function (n) { committed[n] = true; }); });
    var pics = imgs.filter(function (p) { return !mpend || (mpend.images || []).indexOf(p.n) < 0 || committed[p.n]; });
    var wp = inlineData ? inlineData.picks : wpicks.slice(), wc = inlineData ? inlineData.consoles : wconsoles.slice();
    var closing = closeRequests.map(function (r) { return { ref: r, topic: r.topic, reason: r.reason }; });
    if (!t && !list.length && !pics.length && !notes.length && !said.length && !wp.length && !wc.length && !closing.length) { noteWarn(wsT("js.send.empty")); return; }
    if (pics.some(function (p) { return !p.ref && p.data === null; })) { noteWarn(wsT("js.send.reading")); return; }
    var body = { utcOffsetMinutes: -new Date().getTimezoneOffset(), text: t, chips: list, topics: notes.map(function (n) { return { topic: n.topic, note: n.value.trim() }; }), mentions: said };
    if (typeof boardIncarnation === "string") body.boardIncarnation = boardIncarnation;
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
    // The composer hands the whole send to the outbox now, which keeps it
    // until the daemon answers: a failed post gives it back (outboxFail),
    // and the saved draft stays the one from before the send until every
    // queued send is held (saveDraft, outboxAcked). The sent images
    // leave first, so emptying the notes trashes none, and keep their
    // thumbnails until the post settles: a failure puts them back under the
    // same numbers, so no fresh draft renumbers from 1 while a send is
    // unsent (outboxAcked). Drop every sent chip before clearing any card,
    // since clearing one re-syncs the rest.
    // Flush the debounced draft while the composer still owns the send.
    // Once it enters the outbox, saveDraft intentionally leaves it untouched.
    saveDraft();
    var inlineBack = inlineEditor() ? { text: ctext.value, capsules: inlineEditor().snapshot(), selections: bselections.map(function (s) { return { key: s.key, mentions: s.mentions, comment: s.comment, images: s.images }; }) } : null;
    pics.forEach(dropImg);
    ctext.value = ""; grow(); clearMentions();
    list.forEach(function (c) { dropChip(c.id); });
    list.forEach(function (c) { setItem(c.id, null); });
    bmentions = []; bselections = []; closeRequests = []; wpicks = []; wconsoles = [];
    notes.forEach(function (n) {
      if (n.el) { n.el.value = ""; n.el.dispatchEvent(new Event("input", { bubbles: true })); }
    });
    closeTopicNotes();
    clearTrash();
    renderChips();
    renderImgs();
    // No toast on success: the held bubble in the conversation, shown on
    // the daemon's OK, is the confirmation.
    outboxAdd({
      body: body, pics: pics, own: ownTopics, user: lastUserEntry(),
      back: { text: t, chips: list, topics: body.topics, mentions: said, closeRequests: body.closeRequests, picks: wp, consoles: wc, inline: inlineBack, sentAt: Date.now() }
    });
  }
  // --- outbox: a message shows the moment the daemon holds it ----------------
  // A send leaves the composer at once (sendMessage) into the outbox: a
  // dimmed row with a spinner above the message box while its post is
  // unanswered. Posts go one at a time in send order, so the inbox holds
  // them as typed. Only the daemon's OK makes a message look sent: the row
  // then becomes a held bubble in the conversation, without waiting for the
  // stream's render, where the server's held bubble will show (after the
  // held ones in the convlive region). A convlive swap replaces the
  // region's nodes, so each swap puts back the bubbles still unmatched
  // (outboxSync): a render from before the inbox read the message must not
  // drop it. A bubble leaves when a render shows the held bubble with the
  // id its post returned, when the conversation shows a newer user entry
  // than the one before the send (the session received it before any
  // render held it), or, as a last resort, at the first swap OUTBOX_LINGER
  // after the OK (a swap, not a timer: a page whose stream is down keeps
  // what the daemon confirmed). A post that fails, or stays unanswered for
  // OUTBOX_WAIT, takes back its message and every later queued one into the
  // composer, in order, ahead of what was typed since; none showed as sent.
  // A row and a bubble each carry the held bubble's cancel (outboxCancel,
  // cancelEntry); a bubble also its steer.
  var outbox = [], outboxPosting = false, outboxRows = null, OUTBOX_LINGER = 30000, OUTBOX_WAIT = 30000;
  function outboxUnsent() { return outbox.some(function (m) { return !m.acked; }); }
  // A batch (sends the session read in one turn) carries its key on its
  // .user-batch wrapper; the bubbles inside it carry none.
  function lastUserEntry() {
    var us = q("#r-entries .msg.user[data-entry], #r-entries .user-batch[data-entry]");
    return us.length ? us[us.length - 1].getAttribute("data-entry") : "";
  }
  // heldAct is a copy of the server held bubble's steer or cancel button
  // (kind) for id, cloned from the composer's heldacts template; none on a
  // page without one.
  function heldAct(kind, id) {
    var tpl = doc.querySelector("#heldacts"), b = tpl && tpl.content.querySelector("[data-" + kind + "]");
    if (!b) return null;
    b = b.cloneNode(true);
    b.setAttribute("data-" + kind, id);
    return b;
  }
  // outboxRow is an unanswered send's place in the composer: its text on
  // one line, or its image markers when it has no text, and its cancel.
  function outboxRow(m) {
    var el = doc.createElement("div"), spin = doc.createElement("span"), text = doc.createElement("span"), cancel = heldAct("cancel", "");
    el.className = "d-outbox-row";
    spin.className = "spin";
    text.className = "d-outbox-text";
    text.textContent = m.back.text || m.pics.map(function (p) { return marker(p.n); }).join(" ");
    el.appendChild(spin);
    el.appendChild(text);
    if (cancel) {
      cancel.addEventListener("click", function () { outboxCancel(m); });
      el.appendChild(cancel);
    }
    return el;
  }
  function outboxShowRows() {
    var row = ctext.closest && ctext.closest(".crow");
    if (!outboxRows && row) {
      outboxRows = doc.createElement("div");
      outboxRows.className = "d-outbox";
      outboxRows.setAttribute("aria-busy", "true");
      row.parentNode.insertBefore(outboxRows, row);
    }
    if (!outboxRows) return;
    outbox.forEach(function (m) { if (!m.acked && !m.cancel && m.row.parentNode !== outboxRows) outboxRows.appendChild(m.row); });
    outboxRows.hidden = !outboxRows.firstChild;
  }
  function outboxUnrow(m) {
    if (m.row.parentNode) m.row.parentNode.removeChild(m.row);
    if (outboxRows) outboxRows.hidden = !outboxRows.firstChild;
  }
  // outboxBubble is the held bubble the server will render, built from what
  // was sent: the page's own origin and held labels (the first rendered
  // ones), the text as typed, the image thumbnails, and the held bubble's
  // steer and cancel buttons for the id its post returned (a send is never
  // steered yet), which the conversation's click wiring handles as the
  // server's.
  function outboxBubble(m) {
    var el = doc.createElement("div"), mh = doc.createElement("div"), bub = doc.createElement("div");
    var src = doc.querySelector("#convo .msg .src.wb"), tag = doc.querySelector("#r-convlive .heldtag:not(.steertag)");
    var steer = heldAct("steer", m.id), cancel = heldAct("cancel", m.id);
    el.className = "msg user held";
    el.setAttribute("data-entry", m.id);
    mh.className = "mh";
    if (src) mh.appendChild(src.cloneNode(true));
    if (tag) mh.appendChild(tag.cloneNode(true));
    if (steer) mh.appendChild(steer);
    if (cancel) mh.appendChild(cancel);
    el.appendChild(mh);
    bub.className = "bub";
    // Match the server's achips presentation using only escaped DOM text.
    // Structured-only sends have no free-text paragraph to confirm receipt.
    var chips = doc.createElement("div"), back = m.back;
    chips.className = "hchips";
    function chip(cls, label, value, note, title) {
      var el = doc.createElement("span"), head = doc.createElement("b");
      el.className = "achip" + (cls ? " " + cls : "");
      head.textContent = label;
      el.appendChild(head);
      if (value) {
        var answer = doc.createElement("span");
        answer.textContent = " " + value;
        el.appendChild(answer);
      }
      if (note) {
        var detail = doc.createElement("span");
        detail.className = "anote";
        detail.textContent = note;
        el.appendChild(detail);
      }
      if (title || note) el.setAttribute("title", title || note);
      chips.appendChild(el);
    }
    (back.chips || []).forEach(function (c) { chip("", "(" + c.id + ")", c.answer || "note", c.note); });
    (back.topics || []).forEach(function (n) { chip("tchip", "[" + n.topic + "]", "", n.note); });
    (back.mentions || []).forEach(function (n) { chip("mchip", "[" + n.topic + "]" + (n.id ? " (" + n.id + ")" : ""), "", n.text); });
    (back.closeRequests || []).forEach(function (r) { chip("close-request-chip", wsT("js.closerequest.label", r.topic), "", r.reason); });
    (back.picks || []).forEach(function (p) { chip("mchip pchip", p.selector, "", p.text, p.url + "\n" + p.selector + (p.text ? "\n" + p.text : "")); });
    (back.consoles || []).forEach(function (c) { chip("mchip cchip", "console ×" + c.lines.length, "", c.url, c.url + "\n" + c.lines.join("\n")); });
    if (chips.firstChild) bub.appendChild(chips);
    if (m.back.text) {
      // The server renders the text as markdown: a paragraph is the
      // nearest plain-text shape.
      var part = doc.createElement("div"), para = doc.createElement("p");
      part.className = "hpart";
      para.textContent = m.back.text;
      part.appendChild(para);
      bub.appendChild(part);
    }
    if (m.pics.length) {
      var th = doc.createElement("div");
      th.className = "thumbs";
      m.pics.forEach(function (p) {
        var a = doc.createElement("span"), im = doc.createElement("img"), num = doc.createElement("span");
        a.className = "thumb";
        im.src = p.url;
        im.alt = p.name;
        num.className = "tnum";
        num.textContent = "#" + p.n;
        a.appendChild(im);
        a.appendChild(num);
        th.appendChild(a);
      });
      bub.appendChild(th);
    }
    if (bub.firstChild) el.appendChild(bub);
    return el;
  }
  function outboxAdd(m) {
    m.row = outboxRow(m);
    outbox.push(m);
    outboxShowRows();
    followConvo();
    outboxPump();
  }
  function outboxPump() {
    if (outboxPosting) return;
    var m = outbox.filter(function (x) { return !x.acked; })[0];
    if (!m) return;
    outboxPosting = true;
    m.sent = true;
    var wait = 0, settled = false;
    var settle = function (ok, res) {
      if (settled) return;
      settled = true;
      clearTimeout(wait);
      outboxPosting = false;
      if (!ok) { outboxFail(res); return; }
      outboxAcked(m, res && res.id ? String(res.id) : "");
      outboxPump();
    };
    wait = setTimeout(function () { settle(false, wsT("js.post.unreachable")); }, OUTBOX_WAIT);
    post("message", m.body, settle);
  }
  function outboxAcked(m, id) {
    m.acked = true;
    m.id = id;
    m.at = Date.now();
    // A row cancelled while its post was out cancels now that it has an
    // id; its bubble shows only if the cancel is refused. An OK without an
    // id leaves nothing to cancel, so the bubble shows.
    if (!id) m.cancel = false;
    outboxUnrow(m);
    m.el = outboxBubble(m);
    boardFlushViewed(false);
    // A fresh draft numbers its images from 1 again once nothing, sent or
    // not, could name a number.
    if (!outboxUnsent() && !imgs.length && !ctext.value && !chipList().length && !topicNotes().length) imgSeq = 0;
    // The sent draft is gone once nothing queued could need it back; what
    // was typed since is saved again as the next draft.
    if (!outboxUnsent()) clearDraft();
    outboxSync();
    followConvo();
    if (m.cancel) cancelEntry(id, function () { m.cancel = false; outboxSync(); followConvo(); });
  }
  function outboxDrop(m) {
    var i = outbox.indexOf(m);
    if (i < 0) return;
    outbox.splice(i, 1);
    if (m.el.parentNode) m.el.parentNode.removeChild(m.el);
    m.pics.forEach(function (p) { if (imgs.indexOf(p) < 0 && imgTrash[p.n] !== p) release(p); });
  }
  function outboxFail(res) {
    outboxReturn(outbox.filter(function (m) { return !m.acked; }));
    noteWarn(wsT("js.send.fail", res));
    if (res === "board-replaced") {
      // Preserve returned text; boardDraft strips stale IDs after reload.
      saveDraft();
      clearTimeout(draftTimer);
      window.location.reload();
    }
  }
  // outboxReturn takes unanswered sends out of the outbox and back into the
  // composer, in order, ahead of what was typed since.
  function outboxReturn(back) {
    back.forEach(function (m) { outbox.splice(outbox.indexOf(m), 1); });
    back.forEach(function (m) {
      outboxUnrow(m);
      // A topic a send still in the outbox names stays its own.
      m.own.forEach(function (k) { if (!outbox.some(function (o) { return o.own.indexOf(k) >= 0; })) delete boardOwn[k]; });
      m.pics.forEach(function (p) { if (imgs.indexOf(p) < 0) imgs.push(p); });
    });
    restoreHeld(back.map(function (m) { return m.back; }));
    queueDraft();
  }
  // outboxCancel is an unanswered send's cancel: one not posted yet comes
  // straight back into the composer; one whose post is out leaves its row
  // and cancels on the daemon once the OK names its id (outboxAcked), or
  // comes back with the failure if the post fails.
  function outboxCancel(m) {
    if (m.acked || m.cancel || outbox.indexOf(m) < 0) return;
    if (m.sent) { m.cancel = true; outboxUnrow(m); return; }
    outboxReturn([m]);
    noteOk(wsT("js.cancel.ok"));
    ctext.focus();
  }
  // cancelEntry takes the held message id back from the daemon into the
  // composer and drops the outbox bubble for it, so no sync puts it back.
  // A refusal (the session already received it) runs failed and warns.
  function cancelEntry(id, failed) {
    post("cancel", { id: id }, function (ok, res) {
      if (!ok) { failed(); noteWarn(wsT("js.cancel.fail", res)); return; }
      outbox.filter(function (m) { return m.acked && m.id === id; }).forEach(outboxDrop);
      restoreHeld([res]);
      noteOk(wsT("js.cancel.ok"));
      ctext.focus();
    });
  }
  // outboxSync drops the bubbles a render now shows (by id) or the session
  // already received, and puts the rest back, in send order, after the
  // held bubbles. Called after each convlive and entries swap.
  function outboxSync() {
    var shown = outbox.filter(function (m) { return m.acked; });
    if (!shown.length) return;
    var c = doc.querySelector("#r-convlive .clive"), kids = c ? Array.prototype.slice.call(c.children) : [];
    var user = lastUserEntry(), now = Date.now();
    var held = function (m) { return m.id && kids.some(function (k) { return k !== m.el && k.classList.contains("held") && k.getAttribute("data-entry") === m.id; }); };
    shown = shown.filter(function (m) {
      if (user === m.user && now - m.at <= OUTBOX_LINGER && !held(m)) return true;
      outboxDrop(m);
      return false;
    });
    if (!c) return;
    // A bubble whose cancel is out stays hidden (outboxAcked).
    shown = shown.filter(function (m) { return !m.cancel; });
    var at = kids.filter(function (k) { return k.classList.contains("sentdata"); })[0] || null;
    if (shown.some(function (m) { return m.el.parentNode !== c; })) shown.forEach(function (m) { c.insertBefore(m.el, at); });
  }
