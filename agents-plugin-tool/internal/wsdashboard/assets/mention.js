(function () {
  // The @ file-mention popup, a module any page textarea can attach: typing
  // @ at a line start or right after whitespace opens a listbox of the
  // candidates a caller-supplied source returns for the text after the @,
  // anchored at the @ and placed above it when it fits. Up/Down move, Tab or
  // Enter picks, Escape closes; a row picks on mousedown, so the textarea
  // keeps its focus and caret. It binds no fixed element id: attach takes
  // the textarea and its options, and the popup element is its own.
  //
  // A mention is `@path`, or, when the path holds whitespace (or starts
  // with a backtick), `@` and the path as a markdown code span: a backtick
  // run one longer than the path's longest, so the reader sees one path.
  // Picking a directory inserts it with a trailing slash and keeps the popup
  // open on what is inside it; picking a file completes it, spaced from the
  // text after it, and closes the popup.
  var doc = document, popSeq = 0;

  // fenceFor is the backtick run that wraps path in a code span.
  function fenceFor(path) {
    var n = 0;
    (path.match(/`+/g) || []).forEach(function (r) { n = Math.max(n, r.length); });
    return new Array(n + 2).join("`");
  }
  // mentionParts is the composer text of a mention of path: the text, and
  // how many of its last characters close it (the code span's fence).
  function mentionParts(path) {
    if (!/\s/.test(path) && path.charAt(0) !== "`") return { text: "@" + path, close: 0 };
    var f = fenceFor(path), pad = /^`|`$/.test(path) ? " " : "";
    return { text: "@" + f + pad + path + pad + f, close: pad.length + f.length };
  }

  // tokenAt is the mention token the caret at caret in v is in: start (the
  // @), end (where a pick's replacement ends: the rest of a bare token, or a
  // code span's closing fence), and q, the text the source is asked about.
  // A token starts at an @ at a line start or after whitespace. A bare one
  // runs to whitespace. A code-span one may hold spaces; with the caret
  // right after its closing fence (and anything typed on there) it is still
  // one token, so continuing a mention reopens the popup on it. Null when
  // the caret is in no token.
  function tokenAt(v, caret) {
    var ls = v.lastIndexOf("\n", caret - 1) + 1, le = v.indexOf("\n", caret);
    if (le < 0) le = v.length;
    var line = v.slice(ls, le), c = caret - ls;
    for (var i = line.lastIndexOf("@", c - 1); i >= 0; i = i > 0 ? line.lastIndexOf("@", i - 1) : -1) {
      if (i > 0 && !/\s/.test(line.charAt(i - 1))) continue;
      var t = parseAt(line, i, c);
      if (t) return { start: ls + i, end: ls + t.end, q: t.q };
    }
    return null;
  }
  function parseAt(line, i, c) {
    var m = /^`+/.exec(line.slice(i + 1));
    if (!m) {
      var bare = line.slice(i + 1, c);
      if (/\s/.test(bare)) return null;
      return { q: bare, end: c + /^\S*/.exec(line.slice(c))[0].length };
    }
    var f = m[0], from = i + 1 + f.length;
    if (c < from) return null;
    var body = line.slice(from, c), close = body.indexOf(f);
    if (close < 0) {
      var k = line.slice(c).indexOf(f);
      return { q: body, end: k >= 0 ? c + k + f.length : c };
    }
    var tail = body.slice(close + f.length);
    if (/[\s`]/.test(tail)) return null;
    return { q: body.slice(0, close) + tail, end: c + /^\S*/.exec(line.slice(c))[0].length };
  }

  // anchor is the viewport position of the character at index at in the
  // textarea, measured on a hidden copy laid out alike.
  function anchor(input, at) {
    var cs = getComputedStyle(input), d = doc.createElement("div"), mark = doc.createElement("span");
    ["boxSizing", "width", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft", "borderTopWidth", "borderRightWidth", "borderBottomWidth", "borderLeftWidth",
      "fontFamily", "fontSize", "fontWeight", "fontStyle", "letterSpacing", "lineHeight", "textTransform", "wordSpacing", "tabSize"].forEach(function (k) { d.style[k] = cs[k]; });
    d.style.cssText += ";position:absolute;visibility:hidden;top:0;left:-9999px;white-space:pre-wrap;overflow-wrap:break-word;border-style:solid";
    d.textContent = input.value.slice(0, at);
    mark.textContent = "@";
    d.appendChild(mark);
    doc.body.appendChild(d);
    var r = input.getBoundingClientRect(), x = r.left + mark.offsetLeft - input.scrollLeft, y = r.top + mark.offsetTop - input.scrollTop, lh = mark.offsetHeight;
    d.parentNode.removeChild(d);
    return { x: x, top: y, bottom: y + lh };
  }

  // attach wires the popup to input. opts:
  //   source(q, done): asks for the candidates of q, calling done(items)
  //     with [{path, dir}]; a late answer to an older q is dropped;
  //   insert(start, end, text): replaces [start, end) of input's text and
  //     fires its input event (default: setRangeText);
  //   onPick({start, end, text, path, dir}): after a pick inserted a mention
  //     at [start, end);
  //   label: the listbox's accessible name.
  // It returns {key(e), close(), sync()}: the caller's keydown handler calls
  // key(e) first after its IME guard, and stops when it returns true.
  function attach(input, opts) {
    var id = "wsm" + (++popSeq), pop = doc.createElement("ul");
    var st = null, ask = 0, timer = 0;
    pop.className = "wsm-pop";
    pop.id = id;
    pop.setAttribute("role", "listbox");
    if (opts.label) pop.setAttribute("aria-label", opts.label);
    pop.hidden = true;
    doc.body.appendChild(pop);
    input.setAttribute("aria-autocomplete", "list");
    input.setAttribute("aria-controls", id);
    input.setAttribute("aria-expanded", "false");
    var insert = opts.insert || function (start, end, text) {
      input.setRangeText(text, start, end, "end");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    };

    function shown() { return !!st && st.items.length > 0 && !pop.hidden; }
    function hide() {
      pop.hidden = true;
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
    }
    function close() {
      st = null;
      ask++;
      clearTimeout(timer);
      hide();
    }
    // sync follows the text and caret: opens, refilters, or closes.
    function sync() {
      var tok = doc.activeElement === input && input.selectionStart === input.selectionEnd && tokenAt(input.value, input.selectionStart);
      if (!tok) { close(); return; }
      var keep = st && st.start === tok.start;
      // The rows of the last answer stay up while the next is asked for;
      // itemsQ is the query they answer.
      st = { start: tok.start, end: tok.end, q: tok.q, items: keep ? st.items : [], itemsQ: keep ? st.itemsQ : null, idx: keep ? st.idx : 0, pickNext: false };
      if (st.items.length) draw(); else hide();
      request(tok.q, 60);
    }
    function request(q, delay) {
      clearTimeout(timer);
      var n = ++ask;
      timer = setTimeout(function () {
        opts.source(q, function (items) {
          if (n !== ask || !st) return;
          st.items = (items || []).slice(0, 50);
          st.itemsQ = q;
          st.idx = 0;
          if (!st.items.length) { hide(); return; }
          if (st.pickNext) { pick(0); return; }
          draw();
        });
      }, delay);
    }
    function draw() {
      pop.textContent = "";
      st.items.forEach(function (it, i) {
        var li = doc.createElement("li"), name = doc.createElement("span"), dir = doc.createElement("span");
        var p = String(it.path || ""), cut = p.lastIndexOf("/") + 1;
        li.className = "wsm-opt" + (i === st.idx ? " on" : "");
        li.id = id + "-" + i;
        li.setAttribute("role", "option");
        li.setAttribute("aria-selected", String(i === st.idx));
        li.setAttribute("data-i", String(i));
        name.className = "wsm-name";
        name.textContent = p.slice(cut) + (it.dir ? "/" : "");
        dir.className = "wsm-dir";
        dir.textContent = p.slice(0, cut);
        li.appendChild(name);
        li.appendChild(dir);
        li.title = p + (it.dir ? "/" : "");
        pop.appendChild(li);
      });
      pop.hidden = false;
      input.setAttribute("aria-expanded", "true");
      input.setAttribute("aria-activedescendant", id + "-" + st.idx);
      place();
      var on = pop.querySelector(".wsm-opt.on");
      if (on && on.scrollIntoView) on.scrollIntoView({ block: "nearest" });
    }
    function place() {
      if (!shown()) return;
      var a = anchor(input, st.start), w = pop.offsetWidth, h = pop.offsetHeight;
      pop.style.left = Math.max(8, Math.min(a.x - 4, innerWidth - w - 8)) + "px";
      pop.style.top = (a.top - h - 4 >= 8 ? a.top - h - 4 : Math.min(a.bottom + 4, innerHeight - h - 8)) + "px";
    }
    function move(d) {
      st.idx = (st.idx + d + st.items.length) % st.items.length;
      draw();
    }
    function pick(i) {
      var it = st && st.items[i];
      if (!it) return;
      var path = String(it.path || "");
      if (it.dir && path.slice(-1) !== "/") path += "/";
      var parts = mentionParts(path), start = st.start, end = st.end, v = input.value;
      var spaced = /\s/.test(v.charAt(end)), tail = it.dir || spaced ? "" : " ";
      insert(start, end, parts.text + tail);
      var mend = start + parts.text.length;
      // A directory leaves the caret inside its mention, before a code
      // span's closing fence, so typing goes on refining it; a file leaves
      // it past the space after the mention, but never past a line break.
      var caret = it.dir ? mend - parts.close : spaced && v.charAt(end) !== " " ? mend : mend + 1;
      input.setSelectionRange(caret, caret);
      if (opts.onPick) opts.onPick({ start: start, end: mend, text: parts.text, path: path, dir: !!it.dir });
      if (!it.dir) { close(); return; }
      // The parent directory's rows answer nothing inside it.
      if (st) st.items = [];
      sync();
    }
    // key handles a keydown while the popup shows; true when it took it.
    function key(e) {
      if (!shown()) return false;
      var k = e.key;
      if (k === "ArrowDown" || k === "ArrowUp") { e.preventDefault(); move(k === "ArrowDown" ? 1 : -1); return true; }
      if (k === "Enter" || (k === "Tab" && !e.ctrlKey && !e.metaKey && !e.altKey)) {
        e.preventDefault();
        // Rows still answering an older query are not what was typed: ask
        // for the current one now and pick its first row when it answers.
        if (st.itemsQ !== st.q) { st.pickNext = true; request(st.q, 0); } else pick(st.idx);
        return true;
      }
      if (k === "Escape") { e.preventDefault(); close(); return true; }
      return false;
    }
    input.addEventListener("input", sync);
    input.addEventListener("blur", close);
    // A caret moved by a click or the arrow keys may leave the token.
    function follow() { if (st) sync(); }
    input.addEventListener("click", follow);
    input.addEventListener("keyup", function (e) {
      if (/^(ArrowLeft|ArrowRight|Home|End)$/.test(e.key)) follow();
    });
    input.addEventListener("scroll", place);
    window.addEventListener("resize", place);
    pop.addEventListener("mousedown", function (e) {
      e.preventDefault();
      var o = e.target.closest && e.target.closest("[data-i]");
      if (o) pick(+o.getAttribute("data-i"));
    });
    pop.addEventListener("mousemove", function (e) {
      var o = e.target.closest && e.target.closest("[data-i]"), i = o ? +o.getAttribute("data-i") : -1;
      if (i >= 0 && st && i !== st.idx) { st.idx = i; draw(); }
    });
    return { key: key, close: close, sync: sync };
  }

  // paint fills layer, a box laid out like the textarea and drawn behind it,
  // with value's text, each of ranges ([{start, end}], not overlapping) in
  // a pill. The text is the textarea's own, so only the pills show.
  function paint(layer, value, ranges) {
    layer.textContent = "";
    var at = 0;
    ranges.slice().sort(function (a, b) { return a.start - b.start; }).forEach(function (r) {
      if (r.start < at) return;
      if (r.start > at) layer.appendChild(doc.createTextNode(value.slice(at, r.start)));
      var s = doc.createElement("span");
      s.className = "wsm-pill";
      s.textContent = value.slice(r.start, r.end);
      layer.appendChild(s);
      at = r.end;
    });
    // A trailing newline needs a character after it to take its line.
    layer.appendChild(doc.createTextNode(value.slice(at) + "​"));
  }

  window.wsMention = { attach: attach, paint: paint, tokenAt: tokenAt, mentionParts: mentionParts };
})();
