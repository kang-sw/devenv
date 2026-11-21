// Composer board selection mentions.
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
  // An assumed check's history controls use check-<ID>; item-<ID> stays on
  // their summary.
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
  function putSelectionCapsule(s, update) {
    if (!inlineEditor() || !s.mentions.length) return;
    if (!s.key) s.key = "selection-" + Date.now() + "-" + Math.random().toString(36).slice(2);
    var first = s.mentions[0], label = (first.entry || first.id || first.topic) + " · " + first.text.replace(/\s+/g, " ").slice(0, 38);
    var payload = { label: label, anchor: "[mention:" + (first.entry || first.topic) + "]", plain: s.mentions.map(function (m) { return "[" + m.topic + "] " + m.text; }).join(" "), selectionKey: s.key, selection: { mentions: s.mentions, comment: s.comment, images: s.images } };
    if (update) inlineEditor().upsert("mention", s.key, payload);
    else inlineEditor().insertCapsule("mention", payload, { key: s.key });
  }
  function addMention(keepFocus) {
    var p = mpend;
    if (!p || p.rid !== rid || mcomposing) return;
    if (!p.ms.length) return;
    var s = { mentions: validMentions(p.ms), comment: mnote.value, images: p.images.slice() }, index = bselections.indexOf(p.edit);
    s.key = index >= 0 ? p.edit.key : "selection-" + Date.now() + "-" + Math.random().toString(36).slice(2);
    if (index >= 0) { s.row = p.edit.row; bselections[index] = s; }
    else bselections.push(s);
    putSelectionCapsule(s, index >= 0);
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
  if (inlineEditor()) inlineEditor().editor.addEventListener("click", function (e) {
    var cap = e.target.closest && e.target.closest(".wsc-capsule");
    if (!cap) return;
    if (cap.getAttribute("data-wsc-kind") === "topic-note") { focusTopicNote(cap.getAttribute("data-wsc-key")); return; }
    if (cap.getAttribute("data-wsc-kind") !== "mention") return;
    var key = cap.getAttribute("data-wsc-key"), at = bselections.findIndex(function (s) { return s.key === key; });
    if (at >= 0) editSelection(at);
  });
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
