/* Atomic client-local references over an existing textarea's plain-string contract. */
(function () {
  "use strict";
  var doc = document, topicPickers = [];
  var kinds = { skill: true, topic: true, answer: true, "topic-note": true, mention: true, ticket: true, pick: true, console: true, panel: true, selection: true };
  if (doc.addEventListener) doc.addEventListener("selectionchange", function () {
    var editor = doc.activeElement;
    if (editor && editor.wscField && editor.wscField.wsc) editor.wscField.wsc.rememberCaret();
  });
  function sweep() {
    topicPickers = topicPickers.filter(function (p) {
      if (p.field.isConnected !== false) return true;
      p.popup.destroy(); return false;
    });
  }
  function token(value, caret, sigil) {
    if (!/^\s*$/.test(value.slice(caret))) return null;
    var before = value.slice(0, caret), re = new RegExp("(^|\\s)\\" + sigil + "([^\\s" + sigil + "]*)$"), m = re.exec(before);
    return m ? { start: m.index + m[1].length, end: caret, q: m[2] } : null;
  }
  // Stable subsequence ranking: exact, prefix, contiguous, then compact fuzzy matches.
  function rank(rows, query, label) {
    // Composed and decomposed Hangul (NFC/NFD) must match each other.
    var needle = query.normalize("NFC").toLocaleLowerCase();
    return rows.map(function (row, index) {
      var text = label(row).normalize("NFC").toLocaleLowerCase(), at = -1, first = -1, gaps = 0;
      for (var i = 0; i < needle.length; i++) {
        var next = text.indexOf(needle.charAt(i), at + 1);
        if (next < 0) return null;
        if (first < 0) first = next;
        if (at >= 0) gaps += next - at - 1;
        at = next;
      }
      var score = !needle ? 0 : text === needle ? -3000 : text.indexOf(needle) === 0 ? -2000 : text.indexOf(needle) >= 0 ? -1000 + text.indexOf(needle) : gaps * 10 + first;
      return { row: row, index: index, score: score };
    }).filter(Boolean).sort(function (a, b) { return a.score - b.score || a.index - b.index; }).map(function (r) { return r.row; });
  }
  function valid(text, ranges) {
    var end = 0;
    return (Array.isArray(ranges) ? ranges : []).filter(function (r) {
      if (!r || !Object.prototype.hasOwnProperty.call(kinds, r.kind) || typeof r.id !== "string" || !r.id || typeof r.label !== "string" ||
          !Number.isInteger(r.start) || !Number.isInteger(r.end) || r.start < end || r.end <= r.start || r.end > text.length ||
          text.slice(r.start, r.end) !== marker(r) ||
          (r.kind !== "skill" && r.kind !== "topic" && (typeof r.key !== "string" || !r.key || !r.payload || typeof r.payload !== "object" || typeof r.payload.anchor !== "string"))) return false;
      end = r.end; return true;
    }).map(function (r) {
      var copy = { start: r.start, end: r.end, kind: r.kind, id: r.id, label: r.label };
      if (r.kind !== "skill" && r.kind !== "topic") { copy.key = r.key; copy.payload = Object.assign({}, r.payload); }
      return copy;
    });
  }
  function marker(r) { return r.kind === "skill" ? r.id : r.kind === "topic" ? "[" + r.id + "]" : r.payload && r.payload.anchor; }
  function shifted(was, text, ranges) {
    var a = 0, s = 0;
    while (a < was.length && a < text.length && was.charAt(a) === text.charAt(a)) a++;
    while (s < was.length - a && s < text.length - a && was.charAt(was.length - s - 1) === text.charAt(text.length - s - 1)) s++;
    return valid(text, ranges.map(function (r) {
      r = Object.assign({}, r);
      if (was.length - s <= r.start) { r.start += text.length - was.length; r.end += text.length - was.length; }
      else if (a < r.end) return null;
      return r;
    }));
  }
  function stripMarkers(text, ranges, lost) {
    ranges = valid(text, ranges);
    var cuts = [], re = /\[Image #([1-9][0-9]{0,5})\] ?/g, m;
    while ((m = re.exec(text))) if (lost.indexOf(Number(m[1])) >= 0) cuts.push([m.index, m.index + m[0].length]);
    cuts.reverse().forEach(function (cut) {
      ranges = ranges.filter(function (r) { return r.end <= cut[0] || r.start >= cut[1]; }).map(function (r) {
        r = Object.assign({}, r);
        if (r.start >= cut[1]) { r.start -= cut[1] - cut[0]; r.end -= cut[1] - cut[0]; }
        return r;
      });
      text = text.slice(0, cut[0]) + text.slice(cut[1]);
    });
    return { text: text, ranges: ranges };
  }
  function command(field) {
    var r = field && field.wsc && field.wsc.snapshot()[0];
    return r && r.kind === "skill" && r.start === 0 ? { name: r.id, end: r.end } : null;
  }
  function focused(field) { return doc.activeElement === field || !!(field && field.wsc && doc.activeElement === field.wsc.editor); }
  function fieldFor(node) { return node && node.closest && node.closest(".wsc-editor") ? node.closest(".wsc-editor").wscField : node; }
  function attach(field) {
    if (!field || field.wsc) return field && field.wsc;
    var editor = doc.createElement("div"), text = field.value, ranges = [], selection = [0, 0], seenCaret = false, composing = false, nextKey = 0;
    var nativeValue = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(field), "value");
    // Board snapshots clone DOM, not expando properties or event handlers.
    if (field.nextElementSibling && field.nextElementSibling.classList.contains("wsc-editor")) field.nextElementSibling.remove();
    function cache() { if (nativeValue && nativeValue.set) nativeValue.set.call(field, text); }
    editor.className = "wsc-editor";
    editor.setAttribute("contenteditable", "true");
    editor.setAttribute("role", "textbox"); editor.setAttribute("aria-multiline", "true");
    editor.wscField = field;
    field.classList.add("wsc-backing"); field.insertAdjacentElement("afterend", editor);
    function attrs() {
      ["aria-label", "aria-description", "aria-controls", "aria-expanded", "aria-activedescendant", "aria-autocomplete"].forEach(function (key) {
        var value = field.getAttribute(key);
        if (value === null) editor.removeAttribute(key); else editor.setAttribute(key, value);
      });
      editor.setAttribute("data-placeholder", field.getAttribute("placeholder") || "");
      editor.setAttribute("contenteditable", field.disabled ? "false" : "true");
      editor.setAttribute("aria-disabled", String(!!field.disabled));
      editor.hidden = field.hidden;
    }
    new MutationObserver(attrs).observe(field, { attributes: true }); attrs();
    // Reading the editable DOM emits canonical text, never the displayed label.
    // Block boundaries also handle native IME/undo mutations without rewriting them.
    function read(root, endpoint) {
      var out = "", marks = [], units = [], position = 0;
      function walk(n) {
        var start = out.length;
        if (n.nodeType === 3) {
          out += n.nodeValue;
          if (endpoint && endpoint.node === n) position = start + Math.min(endpoint.offset, n.nodeValue.length);
          units.push({ node: n, start: start, end: out.length }); return;
        }
        if (n.nodeType !== 1 && n !== root) return;
        if (n.nodeType === 1 && n.hasAttribute("data-wsc-kind")) {
          var kind = n.getAttribute("data-wsc-kind"), id = n.getAttribute("data-wsc-id"), label = n.getAttribute("data-wsc-label");
          var mark = { start: start, kind: kind, id: id, label: label };
          if (kind !== "skill" && kind !== "topic") {
            mark.key = n.getAttribute("data-wsc-key");
            try { mark.payload = JSON.parse(n.getAttribute("data-wsc-payload")); } catch (_) { mark.payload = null; }
          }
          out += marker(mark) || ""; mark.end = out.length; marks.push(mark);
          if (endpoint && n.contains(endpoint.node)) position = endpoint.offset ? out.length : start;
          units.push({ node: n, start: start, end: out.length, atomic: true }); return;
        }
        if (n.nodeName === "BR") {
          // A final BR is the native caret filler, not another blank line.
          if (!n.nextSibling && (n.parentNode === root || /^(DIV|P)$/.test(n.parentNode.nodeName))) return;
          out += "\n"; units.push({ node: n, start: start, end: out.length, atomic: true }); return;
        }
        var block = n !== root && /^(DIV|P)$/.test(n.nodeName);
        if (block && n.previousSibling && (/^(DIV|P)$/.test(n.previousSibling.nodeName) || !out || out.slice(-1) !== "\n")) {
          var boundary = out.length; out += "\n";
          units.push({ node: n, start: boundary, end: out.length, atomic: true, boundary: true });
        }
        Array.prototype.forEach.call(n.childNodes, function (child, index) {
          if (endpoint && endpoint.node === n && endpoint.offset === index) position = out.length;
          walk(child);
        });
        if (endpoint && endpoint.node === n && endpoint.offset === n.childNodes.length) position = out.length;
        if (block && n.nextSibling && !/^(DIV|P)$/.test(n.nextSibling.nodeName) && out.slice(-1) !== "\n") out += "\n";
      }
      walk(root); return { text: out, ranges: valid(out, marks), units: units, position: position };
    }
    function offsets() {
      var sel = window.getSelection();
      if (!sel || !sel.rangeCount || !editor.contains(sel.anchorNode) || !editor.contains(sel.focusNode)) return selection;
      seenCaret = true;
      var r = sel.getRangeAt(0);
      function offset(node, at) { return read(editor, { node: node, offset: at }).position; }
      selection = [offset(r.startContainer, r.startOffset), offset(r.endContainer, r.endOffset)];
      return selection;
    }
    function point(at) {
      var units = read(editor).units;
      for (var i = 0; i < units.length; i++) {
        var u = units[i];
        if (at > u.end) continue;
        if (!u.atomic) return [u.node, Math.max(0, at - u.start)];
        if (u.boundary && at > u.start) return [u.node, 0];
        var index = Array.prototype.indexOf.call(u.node.parentNode.childNodes, u.node);
        return [u.node.parentNode, index + (at > u.start ? 1 : 0)];
      }
      return [editor, editor.childNodes.length];
    }
    function select(a, b) {
      selection = [Math.max(0, Math.min(text.length, a)), Math.max(0, Math.min(text.length, b))];
      if (!focused(field)) return;
      var start = point(selection[0]), end = point(selection[1]), r = doc.createRange(), sel = window.getSelection();
      r.setStart(start[0], start[1]); r.setEnd(end[0], end[1]); sel.removeAllRanges(); sel.addRange(r);
    }
    function capsule(kind, id, label, key, payload) {
      var chip = doc.createElement("span"); chip.className = "wsc-capsule wsc-" + kind;
      chip.setAttribute("contenteditable", "false");
      chip.setAttribute("data-wsc-kind", kind); chip.setAttribute("data-wsc-id", id); chip.setAttribute("data-wsc-label", label);
      if (kind !== "skill" && kind !== "topic") {
        chip.setAttribute("data-wsc-key", key); chip.setAttribute("data-wsc-payload", JSON.stringify(payload));
      }
      chip.textContent = (kind === "skill" ? "/" : kind === "topic" ? "#" : "") + label;
      return chip;
    }
    function render() {
      cache(); editor.textContent = ""; var at = 0;
      ranges.forEach(function (r) {
        editor.appendChild(doc.createTextNode(text.slice(at, r.start)));
        editor.appendChild(capsule(r.kind, r.id, r.label, r.key, r.payload)); at = r.end;
      });
      editor.appendChild(doc.createTextNode(text.slice(at))); select(selection[0], selection[1]);
    }
    function changed(before, action) {
      var after = valid(text, ranges), left = before.slice(), added = [], removed = [], updated = [];
      after.forEach(function (r) {
        var index = left.findIndex(function (old) { return old.kind === r.kind && (r.key ? old.key === r.key : old.id === r.id); });
        if (index < 0) { added.push(r); return; }
        var old = left.splice(index, 1)[0];
        if (old.label !== r.label || JSON.stringify(old.payload) !== JSON.stringify(r.payload)) updated.push({ before: old, after: r });
      });
      removed = left;
      if (added.length || removed.length || updated.length) {
        var detail = { action: action || (removed.length ? "remove" : updated.length ? "update" : "insert"), added: added, removed: removed, updated: updated, capsules: after };
        var event = typeof CustomEvent === "function" ? new CustomEvent("wsc:change", { bubbles: true, detail: detail }) : new Event("wsc:change", { bubbles: true });
        if (!event.detail) Object.defineProperty(event, "detail", { value: detail });
        field.dispatchEvent(event);
      }
    }
    function notify(before, action) {
      cache();
      var event = new Event("input", { bubbles: true });
      Object.defineProperty(event, "isComposing", { value: composing });
      field.dispatchEvent(event);
      if (before) changed(before, action);
    }
    function replace(s, a, b, mode) {
      // A partial range through a capsule edits the whole capsule.
      ranges.forEach(function (r) { if (a < r.end && b > r.start) { a = Math.min(a, r.start); b = Math.max(b, r.end); } });
      var next = text.slice(0, a) + s + text.slice(b);
      ranges = ranges.filter(function (r) { return r.end <= a || r.start >= b; }).map(function (r) {
        r = Object.assign({}, r); if (r.start >= b) { r.start += s.length - (b - a); r.end += s.length - (b - a); } return r;
      });
      text = next; selection = mode === "select" ? [a, a + s.length] : [a + s.length, a + s.length]; render();
    }
    function expanded(a, b) {
      ranges.forEach(function (r) {
        if (a === b && a > r.start && a < r.end) { a = r.end; b = r.end; }
        else if (a < r.end && b > r.start) { a = Math.min(a, r.start); b = Math.max(b, r.end); }
      });
      return [a, b];
    }
    function movedSelection(saved, a, b, length) {
      return saved.map(function (pos) {
        if (pos <= a) return pos;
        if (pos >= b) return pos + length - (b - a);
        return a + length;
      });
    }
    function insertPlain(value) {
      var sel = offsets(), before = valid(text, ranges);
      if (!doc.execCommand("insertText", false, value)) { replace(value, sel[0], sel[1]); notify(before); }
    }
    Object.defineProperties(field, {
      value: { configurable: true, get: function () { return text; }, set: function (v) { var before = valid(text, ranges); v = String(v); ranges = shifted(text, v, ranges); text = v; render(); changed(before); } },
      selectionStart: { configurable: true, get: function () { return offsets()[0]; } },
      selectionEnd: { configurable: true, get: function () { return offsets()[1]; } }
    });
    field.setSelectionRange = function (a, b) { seenCaret = true; select(a, b); };
    field.setRangeText = function (s, a, b, mode) { var before = valid(text, ranges); replace(s, a, b, mode); changed(before); };
    field.focus = function () { attrs(); if (!field.hidden && !field.disabled) { if (doc.activeElement === editor) offsets(); seenCaret = true; editor.focus(); select(selection[0], selection[1]); } };
    field.blur = function () { editor.blur(); };
    field.select = function () { field.focus(); select(0, text.length); };
    editor.addEventListener("input", function (e) {
      e.stopPropagation(); var before = valid(text, ranges), state = read(editor); text = state.text; ranges = state.ranges;
      if (editor.childNodes.length === 1 && editor.firstChild.nodeName === "BR") text = "";
      offsets(); notify(before);
    });
    editor.addEventListener("compositionstart", function () { composing = true; field.dispatchEvent(new Event("compositionstart")); });
    // compositionupdate fires before the DOM shows the new text, and an IME
    // may send no input event: read the live text after it, then forward.
    editor.addEventListener("compositionupdate", function () {
      setTimeout(function () {
        if (!composing) return;
        var state = read(editor); text = state.text; ranges = state.ranges; offsets();
        field.dispatchEvent(new Event("compositionupdate"));
      }, 0);
    });
    editor.addEventListener("compositionend", function () { composing = false; field.dispatchEvent(new Event("compositionend")); });
    ["keydown", "keyup"].forEach(function (type) {
      editor.addEventListener(type, function (e) {
        e.stopPropagation();
        if (e.defaultPrevented) return;
        var forwarded = new KeyboardEvent(type, { key: e.key, code: e.code, keyCode: e.keyCode, shiftKey: e.shiftKey, ctrlKey: e.ctrlKey, metaKey: e.metaKey, altKey: e.altKey, isComposing: composing || e.isComposing, bubbles: true, cancelable: true });
        if (!field.dispatchEvent(forwarded)) { e.preventDefault(); return; }
        if (type !== "keydown" || composing || e.isComposing || e.keyCode === 229) return;
        var sel = offsets(), a = sel[0], b = sel[1];
        if (e.key === "Backspace" || e.key === "Delete") {
          var target = ranges.filter(function (r) { return a === b && (e.key === "Backspace" ? r.end === a : r.start === a) || a < r.end && b > r.start; });
          if (target.length) {
            e.preventDefault(); target.forEach(function (r) { a = Math.min(a, r.start); b = Math.max(b, r.end); });
            select(a, b);
            var before = valid(text, ranges);
            if (!doc.execCommand("delete", false)) { replace("", a, b); notify(before, "remove"); }
          }
        } else if (e.key === "Enter") {
          e.preventDefault(); insertPlain("\n");
        }
      });
    });
    ["focus", "blur", "click", "mouseup", "scroll"].forEach(function (type) { editor.addEventListener(type, function () { if (type === "focus") seenCaret = true; offsets(); field.dispatchEvent(new Event(type)); }); });
    editor.addEventListener("paste", function (e) {
      e.stopPropagation();
      var forward = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(forward, "clipboardData", { value: e.clipboardData }); field.dispatchEvent(forward);
      e.preventDefault();
      if (forward.defaultPrevented || !e.clipboardData) return;
      var rich;
      try { rich = JSON.parse(e.clipboardData.getData("application/x-wsc-capsules+json")); } catch (_) { rich = null; }
      if (rich && typeof rich.text === "string" && Array.isArray(rich.ranges) && valid(rich.text, rich.ranges).length === rich.ranges.length) {
        var sel = expanded.apply(null, offsets()), before = valid(text, ranges), start = sel[0];
        replace(rich.text, sel[0], sel[1]);
        ranges = valid(text, ranges.concat(rich.ranges.map(function (r) { return Object.assign({}, r, { start: r.start + start, end: r.end + start }); })).sort(function (a, b) { return a.start - b.start; }));
        render(); notify(before, "paste");
      } else insertPlain(e.clipboardData.getData("text/plain"));
    });
    // Rich HTML drops must not manufacture capsule identity or execute markup.
    editor.addEventListener("drop", function (e) {
      if (e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0) return;
      e.preventDefault(); if (e.dataTransfer) insertPlain(e.dataTransfer.getData("text/plain"));
    });
    editor.addEventListener("copy", copy); editor.addEventListener("cut", copy);
    function copy(e) {
      var sel = offsets(); if (!e.clipboardData || sel[0] === sel[1]) return;
      var a = sel[0], b = sel[1], selected = ranges.filter(function (r) { return a < r.end && b > r.start; });
      selected.forEach(function (r) { a = Math.min(a, r.start); b = Math.max(b, r.end); });
      var raw = text.slice(a, b), local = selected.map(function (r) { return Object.assign({}, r, { start: r.start - a, end: r.end - a }); });
      var plain = raw;
      selected.slice().reverse().forEach(function (r) {
        if (r.payload && typeof r.payload.plain === "string") plain = plain.slice(0, r.start - a) + r.payload.plain + plain.slice(r.end - a);
      });
      e.preventDefault(); e.clipboardData.setData("text/plain", plain);
      if (selected.length) e.clipboardData.setData("application/x-wsc-capsules+json", JSON.stringify({ text: raw, ranges: local }));
      if (e.type === "cut") { var before = valid(text, ranges); replace("", a, b); notify(before, "remove"); }
    }
    function insertGeneric(kind, payload, options) {
      options = options || {};
      if (kind === "skill" || kind === "topic" || !Object.prototype.hasOwnProperty.call(kinds, kind) || !payload || typeof payload.anchor !== "string" || !payload.anchor) return null;
      var key = options.key || payload.key;
      if (!key) do { key = kind + ":" + (++nextKey); } while (ranges.some(function (r) { return r.kind === kind && r.key === key; }));
      key = String(key);
      var label = typeof payload.label === "string" ? payload.label : payload.anchor;
      var saved = options.at, at = saved && typeof saved.start === "number" ? [saved.start, saved.end] : typeof saved === "number" ? [saved, saved] : focused(field) ? offsets() : seenCaret ? selection.slice() : [text.length, text.length];
      at = [Math.max(0, Math.min(text.length, at[0])), Math.max(0, Math.min(text.length, at[1]))];
      at = expanded(at[0], at[1]);
      var before = valid(text, ranges), prefix = at[0] && !/\s/.test(text.charAt(at[0] - 1)) ? " " : "";
      var suffix = !/\s/.test(text.charAt(at[1])) ? " " : "";
      var mark = { start: at[0] + prefix.length, end: at[0] + prefix.length + payload.anchor.length, kind: kind, id: key, key: key, label: label, payload: Object.assign({}, payload, { anchor: payload.anchor }) };
      var kept = options.preserveCaret ? (focused(field) ? offsets().slice() : selection.slice()) : null;
      if (!options.preserveCaret) field.focus();
      replace(prefix + payload.anchor + suffix, at[0], at[1]);
      ranges = valid(text, ranges.concat(mark).sort(function (a, b) { return a.start - b.start; }));
      render(); if (kept) { var moved = movedSelection(kept, at[0], at[1], prefix.length + payload.anchor.length + suffix.length); select(moved[0], moved[1]); }
      notify(before, "insert");
      return mark;
    }
    var api = field.wsc = {
      editor: editor, snapshot: function () { return valid(text, ranges); },
      rememberCaret: offsets,
      ordered: function () { return valid(text, ranges); },
      restore: function (saved) { var before = valid(text, ranges); ranges = valid(text, saved); render(); changed(before, "restore"); },
      insertCapsule: insertGeneric,
      upsert: function (kind, key, payload) {
        key = String(key); var old = ranges.find(function (r) { return r.kind === kind && r.key === key; });
        if (!old) return insertGeneric(kind, payload, { key: key, at: text.length, preserveCaret: true });
        if (!payload || typeof payload.anchor !== "string" || !payload.anchor) return null;
        var before = valid(text, ranges), label = typeof payload.label === "string" ? payload.label : payload.anchor;
        if (marker(old) === payload.anchor && old.label === label && JSON.stringify(old.payload) === JSON.stringify(payload)) return old;
        var kept = focused(field) ? offsets().slice() : selection.slice();
        replace(payload.anchor, old.start, old.end);
        var mark = { start: old.start, end: old.start + payload.anchor.length, kind: kind, id: key, key: key, label: label, payload: Object.assign({}, payload) };
        ranges = valid(text, ranges.concat(mark).sort(function (a, b) { return a.start - b.start; }));
        render(); var moved = movedSelection(kept, old.start, old.end, payload.anchor.length); select(moved[0], moved[1]);
        notify(before, "update"); return mark;
      },
      remove: function (kind, key) {
        var before = valid(text, ranges), removed = before.filter(function (r) { return r.kind === kind && r.key === String(key); });
        removed.slice().reverse().forEach(function (r) { replace("", r.start, r.end + (text.charAt(r.end) === " " ? 1 : 0)); });
        if (removed.length) notify(before, "remove");
        return removed;
      },
      insert: function (kind, id, label, at) {
        var before = valid(text, ranges), value = kind === "skill" ? id : "[" + id + "]", tail = /\s/.test(text.charAt(at.end)) ? "" : " ";
        field.focus(); select(at.start, at.end);
        // A browser editing transaction keeps capsule insertion/deletion undoable.
        if (doc.execCommand("insertHTML", false, capsule(kind, id, label).outerHTML + tail)) return;
        replace(value + tail, at.start, at.end);
        ranges.push({ start: at.start, end: at.start + value.length, kind: kind, id: id, label: label });
        ranges.sort(function (a, b) { return a.start - b.start; }); render(); notify(before, "insert");
      },
      anchor: function (at) { var p = point(at), r = doc.createRange(); r.setStart(p[0], p[1]); r.collapse(true); var box = r.getBoundingClientRect(); return box.height ? { x: box.left, top: box.top, bottom: box.bottom } : { x: editor.getBoundingClientRect().left, top: editor.getBoundingClientRect().top, bottom: editor.getBoundingClientRect().bottom }; }
    };
    render(); return api;
  }
  function topicPicker(field, source) {
    if (!field || !window.wsMention) return null;
    attach(field);
    var popup = window.wsMention.attach(field, {
      label: wsT("js.capsule.topics"), token: function (v, c) { return token(v, c, "#"); },
      source: function (q, done) { source(function (rows) { done(rank(rows, q, function (r) { return r.label; })); }); },
      render: function (li, row) {
        var name = doc.createElement("span"), stem = doc.createElement("span");
        name.className = "wsm-name"; name.textContent = row.label; stem.className = "wsm-desc"; stem.textContent = row.id;
        li.appendChild(name); li.appendChild(stem);
      }, choose: function (row, at) { field.wsc.insert("topic", row.id, row.label, at); }
    });
    topicPickers.push({ field: field, popup: popup });
    field.addEventListener("keydown", function (e) { if (!e.defaultPrevented && !e.isComposing && e.keyCode !== 229 && popup.key(e)) e.stopImmediatePropagation(); });
    return popup;
  }
  window.WsCapsules = { attach: attach, topicPicker: topicPicker, sweep: sweep, token: token, rank: rank, valid: valid, shifted: shifted, stripMarkers: stripMarkers, command: command, focused: focused, fieldFor: fieldFor };
})();
