/* Atomic client-local references over an existing textarea's plain-string contract. */
(function () {
  "use strict";
  var doc = document, topicPickers = [];
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
    var needle = query.toLocaleLowerCase();
    return rows.map(function (row, index) {
      var text = label(row).toLocaleLowerCase(), at = -1, first = -1, gaps = 0;
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
      if (!r || (r.kind !== "skill" && r.kind !== "topic") || typeof r.id !== "string" || !r.id || typeof r.label !== "string" ||
          !Number.isInteger(r.start) || !Number.isInteger(r.end) || r.start < end || r.end <= r.start || r.end > text.length ||
          text.slice(r.start, r.end) !== (r.kind === "skill" ? r.id : "[" + r.id + "]")) return false;
      end = r.end; return true;
    }).map(function (r) { return { start: r.start, end: r.end, kind: r.kind, id: r.id, label: r.label }; });
  }
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
    var editor = doc.createElement("div"), text = field.value, ranges = [], selection = [0, 0], composing = false;
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
          out += kind === "skill" ? id : "[" + id + "]";
          marks.push({ start: start, end: out.length, kind: kind, id: id, label: label });
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
    function capsule(kind, id, label) {
      var chip = doc.createElement("span"); chip.className = "wsc-capsule wsc-" + kind;
      chip.setAttribute("contenteditable", "false");
      chip.setAttribute("data-wsc-kind", kind); chip.setAttribute("data-wsc-id", id); chip.setAttribute("data-wsc-label", label);
      chip.textContent = (kind === "skill" ? "/" : "#") + label;
      return chip;
    }
    function render() {
      cache(); editor.textContent = ""; var at = 0;
      ranges.forEach(function (r) {
        editor.appendChild(doc.createTextNode(text.slice(at, r.start)));
        editor.appendChild(capsule(r.kind, r.id, r.label)); at = r.end;
      });
      editor.appendChild(doc.createTextNode(text.slice(at))); select(selection[0], selection[1]);
    }
    function notify() {
      cache();
      var event = new Event("input", { bubbles: true });
      Object.defineProperty(event, "isComposing", { value: composing });
      field.dispatchEvent(event);
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
    function insertPlain(value) {
      var sel = offsets();
      if (!doc.execCommand("insertText", false, value)) { replace(value, sel[0], sel[1]); notify(); }
    }
    Object.defineProperties(field, {
      value: { configurable: true, get: function () { return text; }, set: function (v) { v = String(v); ranges = shifted(text, v, ranges); text = v; render(); } },
      selectionStart: { configurable: true, get: function () { return offsets()[0]; } },
      selectionEnd: { configurable: true, get: function () { return offsets()[1]; } }
    });
    field.setSelectionRange = select; field.setRangeText = replace;
    field.focus = function () { attrs(); if (!field.hidden && !field.disabled) { if (doc.activeElement === editor) offsets(); editor.focus(); select(selection[0], selection[1]); } };
    field.blur = function () { editor.blur(); };
    field.select = function () { field.focus(); select(0, text.length); };
    editor.addEventListener("input", function (e) {
      e.stopPropagation(); var state = read(editor); text = state.text; ranges = state.ranges;
      if (editor.childNodes.length === 1 && editor.firstChild.nodeName === "BR") text = "";
      offsets(); notify();
    });
    editor.addEventListener("compositionstart", function () { composing = true; field.dispatchEvent(new Event("compositionstart")); });
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
            if (!doc.execCommand("delete", false)) { replace("", a, b); notify(); }
          }
        } else if (e.key === "Enter") {
          e.preventDefault(); insertPlain("\n");
        }
      });
    });
    ["focus", "blur", "click", "scroll"].forEach(function (type) { editor.addEventListener(type, function () { offsets(); field.dispatchEvent(new Event(type)); }); });
    editor.addEventListener("paste", function (e) {
      e.stopPropagation();
      var forward = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(forward, "clipboardData", { value: e.clipboardData }); field.dispatchEvent(forward);
      e.preventDefault();
      if (!forward.defaultPrevented && e.clipboardData) insertPlain(e.clipboardData.getData("text/plain"));
    });
    // Rich HTML drops must not manufacture capsule identity or execute markup.
    editor.addEventListener("drop", function (e) {
      if (e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0) return;
      e.preventDefault(); if (e.dataTransfer) insertPlain(e.dataTransfer.getData("text/plain"));
    });
    editor.addEventListener("copy", copy); editor.addEventListener("cut", copy);
    function copy(e) {
      var sel = offsets(); if (!e.clipboardData || sel[0] === sel[1]) return;
      e.preventDefault(); e.clipboardData.setData("text/plain", text.slice(sel[0], sel[1]));
      if (e.type === "cut" && !doc.execCommand("delete", false)) { replace("", sel[0], sel[1]); notify(); }
    }
    var api = field.wsc = {
      editor: editor, snapshot: function () { return valid(text, ranges); },
      restore: function (saved) { ranges = valid(text, saved); render(); },
      insert: function (kind, id, label, at) {
        var value = kind === "skill" ? id : "[" + id + "]", tail = /\s/.test(text.charAt(at.end)) ? "" : " ";
        field.focus(); select(at.start, at.end);
        // A browser editing transaction keeps capsule insertion/deletion undoable.
        if (doc.execCommand("insertHTML", false, capsule(kind, id, label).outerHTML + tail)) return;
        replace(value + tail, at.start, at.end);
        ranges.push({ start: at.start, end: at.start + value.length, kind: kind, id: id, label: label });
        ranges.sort(function (a, b) { return a.start - b.start; }); render(); notify();
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
