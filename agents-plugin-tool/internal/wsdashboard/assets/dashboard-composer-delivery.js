// Composer sends and outbox delivery.
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
    if (restoring || draftStore && !draftLoaded) return;
    var inline = !cmdSel && window.WsCapsules && window.WsCapsules.command(ctext);
    // A command waits until the queued messages before it are held, so the
    // inbox keeps the order typed.
    if ((cmdSel || inline) && outboxInFlight()) return;
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
    if (pics.some(function (p) { return !p.ref && !p.blob && p.data === null; })) { noteWarn(wsT("js.send.reading")); return; }
    var body = { utcOffsetMinutes: -new Date().getTimezoneOffset(), text: t, chips: list, topics: notes.map(function (n) { return { topic: n.topic, note: n.value.trim() }; }), mentions: said };
    if (typeof boardIncarnation === "string") body.boardIncarnation = boardIncarnation;
    if (closing.length) body.closeRequests = closing.map(function (r) { return { topic: r.topic, reason: r.reason }; });
    if (wp.length) body.picks = wp;
    if (wc.length) body.consoles = wc;
    var refs = pics.filter(function (p) { return !!p.ref; });
    if (refs.length) {
      body.refs = refs.map(function (p) { return p.ref; });
      body.refNums = refs.map(function (p) { return p.n; });
    }
    // A send arms only receipt matching for its topics, never a blanket
    // exemption for a concurrent external panel rewrite.
    var ownTopics = notes.map(function (n) { return n.topic; });
    list.forEach(function (c) { var item = boardItem(c.id), topic = item && item.closest("section.topic[data-topic]"); if (topic) ownTopics.push(topicKey(topic)); });
    ownTopics.forEach(function (k) { boardOwn[k] = true; });
    // Save the active draft before handing it off. Its recovery record
    // commits before the POST can start.
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
  // --- outbox -------------------------------------------------------------------
  // Only an acknowledged POST gets a held bubble. Restored deliveries stay
  // in the composer until the user chooses to send or discard each one.
  var outbox = [], outboxPosting = false, outboxRows = null, recoveredRows = null, deliverySequence = 0, OUTBOX_LINGER = 30000, OUTBOX_WAIT = 30000;
  function outboxUnsent() { return outbox.some(function (m) { return !m.acked; }); }
  function outboxInFlight() { return outbox.some(function (m) { return !m.acked && !m.paused; }); }
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
    outbox.forEach(function (m) { if (!m.acked && !m.paused && !m.cancel && m.row.parentNode !== outboxRows) outboxRows.appendChild(m.row); });
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
  function nextDeliveryID() {
    return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2) + "-" + ++deliverySequence;
  }
  function outboxAdd(m) {
    m.recoveryID = nextDeliveryID();
    m.recoveryState = "queued";
    m.row = outboxRow(m);
    outbox.push(m);
    outboxShowRows();
    followConvo();
    if (outboxPosting) saveDraft();
    else outboxPump();
  }
  function restoreDeliveries(deliveries) {
    if (!ctext || !deliveries.length) return;
    deliveries.forEach(function (m) {
      m.pics = m.pics.map(loadDraftImage);
      m.pics.forEach(function (p) { imgSeq = Math.max(imgSeq, p.n); });
      outbox.push({ recoveryID: m.id, recoveryState: m.state, body: m.body,
        pics: m.pics, back: m.back, own: [], user: lastUserEntry(), paused: true, row: outboxRow(m) });
    });
    showRecoveredDeliveries();
  }
  function showRecoveredDeliveries() {
    var paused = outbox.filter(function (m) { return m.paused; });
    if (!recoveredRows && !paused.length) return;
    if (!recoveredRows) {
      var row = ctext.closest(".crow");
      if (!row) return;
      recoveredRows = doc.createElement("div");
      recoveredRows.className = "d-outbox d-outbox-recovery";
      row.parentNode.insertBefore(recoveredRows, row);
    }
    recoveredRows.replaceChildren();
    recoveredRows.hidden = !paused.length;
    paused.forEach(function (m) {
      var row = doc.createElement("div"), title = doc.createElement("strong"), text = doc.createElement("span");
      row.className = "d-outbox-row";
      title.textContent = wsT(m.recoveryState === "queued" ? "js.delivery.unsent" : "js.delivery.unknown");
      text.className = "d-outbox-text";
      text.textContent = m.back.text || m.pics.map(function (p) { return marker(p.n); }).join(" ") ||
        (m.back.chips || []).map(function (c) { return c.note || c.answer || c.id; }).join(" · ") ||
        (m.back.topics || []).map(function (n) { return n.note || n.topic; }).join(" · ");
      row.append(title, text);
      function action(label, click) {
        var button = doc.createElement("button");
        button.type = "button";
        button.textContent = wsT(label);
        button.addEventListener("click", click);
        row.appendChild(button);
      }
      action(m.recoveryState === "queued" ? "js.delivery.send" : "js.delivery.resend", async function () {
        if (!m.paused || outbox.indexOf(m) < 0) return;
        if (m.recoveryState !== "queued" && !await window.wsConfirm(wsT("js.delivery.confirm"), {
          accept: wsT("js.delivery.resend"), opener: doc.activeElement
        })) return;
        if (!m.paused || outbox.indexOf(m) < 0) return;
        m.paused = false;
        m.cancel = false;
        m.sent = false;
        if (m.recoveryState !== "queued") m.recoveryID = nextDeliveryID();
        m.user = lastUserEntry();
        showRecoveredDeliveries();
        outboxShowRows();
        outboxPump();
        ctext.focus();
      });
      action("js.delivery.discard", function () {
        if (!m.paused || outbox.indexOf(m) < 0) return;
        outboxDrop(m);
        showRecoveredDeliveries();
        saveDraft();
        ctext.focus();
      });
      recoveredRows.appendChild(row);
    });
  }
  function pauseDeliveries(m, state) {
    m.recoveryState = state;
    outbox.forEach(function (entry) {
      if (entry.acked) return;
      entry.paused = true;
      outboxUnrow(entry);
    });
    showRecoveredDeliveries();
    saveDraft();
  }
  function prepareDelivery(m, submit, fail) {
    var uploads = m.pics.filter(function (p) { return !p.ref; });
    if (!uploads.length) { submit(m.body); return; }
    Promise.all(uploads.map(async function (p) {
      return { name: p.name, data: p.data === null ? await imageData(p.blob) : p.data, n: p.n };
    })).then(function (images) {
      submit(Object.assign({}, m.body, { images: images }));
    }, function () { fail(false, wsT("js.send.reading")); });
  }
  function persistOutbox(done) { saveDraft().then(done); }
  function outboxPump() {
    if (outboxPosting || draftDetached) return;
    var m = outbox.find(function (x) { return !x.acked && !x.paused; });
    if (!m) return;
    outboxPosting = true;
    var previous = m.recoveryState, settled = false, wait = null;
    m.recoveryState = "posting";
    m.cancelPreparation = function () {
      settled = true;
      outboxPosting = false;
      outboxPump();
    };
    function settle(ok, res, unknown) {
      if (settled || draftDetached) return;
      settled = true;
      clearTimeout(wait);
      outboxPosting = false;
      if (unknown || !ok && previous !== "queued") {
        pauseDeliveries(m, "unknown");
        if (!unknown) noteWarn(wsT("js.send.fail", res));
        return;
      }
      if (!ok) { outboxFail(res); return; }
      if (!res || typeof res.id !== "string" || !/^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{8}$/.test(res.id)) {
        pauseDeliveries(m, "unknown");
        return;
      }
      outboxAcked(m, res.id);
      outboxPump();
    }
    persistOutbox(function (saved) {
      if (settled || draftDetached) return;
      if (m.recoveryConflict) {
        outboxUnrow(m);
        outboxDrop(m);
        releaseTopics(m);
        saveDraft();
        m.cancelPreparation();
        return;
      }
      if (!saved) {
        outboxPosting = false;
        pauseDeliveries(m, previous);
        noteWarn(wsT("js.delivery.savefailed"));
        return;
      }
      prepareDelivery(m, function (body) {
        if (settled || draftDetached) return;
        m.sent = true;
        m.live = doc.querySelector("#r-convlive .clive");
        wait = setTimeout(function () { settle(false, wsT("js.post.unreachable"), true); }, OUTBOX_WAIT);
        post("message", body, settle);
      }, settle);
    });
  }
  function outboxAcked(m, id) {
    m.acked = true;
    m.id = id;
    m.at = Date.now();
    outboxUnrow(m);
    if (m.body.command) outboxDrop(m);
    else {
      var live = doc.querySelector("#r-convlive .clive");
      // A replacement empty live region already reflects an authoritative
      // render; do not resurrect the held message after that render.
      var emptyRender = m.live && live && m.live !== live && !Array.prototype.some.call(live.children, function (el) { return el.classList.contains("held"); });
      if (lastUserEntry() !== m.user || emptyRender) outboxDrop(m);
      else {
        m.el = outboxBubble(m);
        m.lingerTimer = setTimeout(function () { outboxDrop(m); }, OUTBOX_LINGER);
      }
      boardFlushViewed(false);
    }
    // A fresh draft numbers its images from 1 again once nothing, sent or
    // not, could name a number.
    if (!outboxUnsent() && !imgs.length && !ctext.value && !chipList().length && !topicNotes().length) imgSeq = 0;
    saveDraft();
    outboxSync();
    followConvo();
    if (m.cancel) cancelEntry(id, function () { m.cancel = false; outboxSync(); followConvo(); });
  }
  function outboxDrop(m) {
    var i = outbox.indexOf(m);
    if (i < 0) return;
    outbox.splice(i, 1);
    clearTimeout(m.lingerTimer);
    if (m.el && m.el.parentNode) m.el.parentNode.removeChild(m.el);
    m.pics.forEach(function (p) { if (imgs.indexOf(p) < 0 && imgTrash[p.n] !== p) release(p); });
  }
  function outboxFail(res) {
    var saved = outboxReturn(outbox.filter(function (m) { return !m.acked && !m.paused; }));
    noteWarn(wsT("js.send.fail", res));
    if (res === "board-replaced") {
      // Preserve returned text; boardDraft strips stale IDs after reload.
      saved.then(function (ok) { if (ok && !draftDetached) window.location.reload(); });
    }
  }
  // outboxReturn takes unanswered sends out of the outbox and back into the
  // composer, in order, ahead of what was typed since.
  function releaseTopics(m) {
    m.own.forEach(function (k) { if (!outbox.some(function (o) { return o.own.indexOf(k) >= 0; })) delete boardOwn[k]; });
  }
  function outboxReturn(back) {
    back.forEach(function (m) { outbox.splice(outbox.indexOf(m), 1); });
    back.forEach(function (m) {
      outboxUnrow(m);
      // A topic a send still in the outbox names stays its own.
      releaseTopics(m);
      m.pics.forEach(function (p) { if (imgs.indexOf(p) < 0) imgs.push(p); });
    });
    restoreHeld(back.map(function (m) { return m.back; }));
    return Promise.resolve(saveDraft());
  }
  // outboxCancel is an unanswered send's cancel: one not posted yet comes
  // straight back into the composer; one whose post is out leaves its row
  // and cancels on the daemon once the OK names its id (outboxAcked), or
  // comes back with the failure if the post fails.
  function outboxCancel(m) {
    if (m.acked || m.cancel || outbox.indexOf(m) < 0) return;
    if (m.sent) { m.cancel = true; outboxUnrow(m); return; }
    outboxReturn([m]);
    if (m.cancelPreparation) m.cancelPreparation();
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
