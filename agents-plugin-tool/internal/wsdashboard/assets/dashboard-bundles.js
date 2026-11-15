  // Client-local views of server-canonical selection text. The Go parser is
  // authoritative: this reader accepts only its canonical serialization, and
  // keeps every failed envelope opaque. No request or inbox metadata is added.
  function selectionEscape(text, attr) {
    var out = String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    if (attr) out = out.replace(/"/g, "&quot;");
    return out.replace(/\[Image #([1-9][0-9]{0,5})\]/g, "&#91;Image #$1]");
  }
  function selectionUnescape(text) {
    var entities = { "&lt;": "<", "&gt;": ">", "&quot;": '"', "&amp;": "&", "&#91;": "[" };
    return text.replace(/&lt;|&gt;|&quot;|&amp;|&#91;/g, function (s) { return entities[s]; });
  }
  function selectionBlock(s) {
    var lines = ["<wb:selection>"].concat(s.mentions.map(function (m) {
      return '<wb:mention topic="' + selectionEscape(m.topic, true) + '"' + (m.id ? ' id="' + m.id + '"' : "") +
        (m.entry ? ' entry="' + m.entry + '"' : "") + ">" + selectionEscape(m.text) + "</wb:mention>";
    }));
    lines.push("<wb:comment>" + selectionEscape(s.comment) + "</wb:comment>");
    (s.images || []).forEach(function (n) { lines.push("[Image #" + n + "]"); });
    return lines.concat(["</wb:selection>"]).join("\n");
  }
  function readSelectionBlock(raw) {
    var text = raw.replace(/\r\n/g, "\n"), body = text.slice(15, -16), ms = [], m;
    if (text.slice(0, 15) !== "<wb:selection>\n" || text.slice(-16) !== "\n</wb:selection>") return null;
    var head = /^<wb:mention topic="([^"]*)"(?: id="([0-9]{1,6})")?(?: entry="(e[1-9][0-9]*)")?>([^<]*)<\/wb:mention>\n/;
    while ((m = head.exec(body))) {
      var mention = { topic: selectionUnescape(m[1]), text: selectionUnescape(m[4]) };
      if (!mention.topic.trim() || /[\r\n]/.test(mention.topic) || new TextEncoder().encode(mention.topic).length > 200 || !mention.text.trim()) return null;
      if (m[2]) mention.id = m[2];
      if (m[3]) mention.entry = m[3];
      ms.push(mention); body = body.slice(m[0].length);
    }
    m = /^<wb:comment>([^<]*)<\/wb:comment>([\s\S]*)$/.exec(body);
    if (!ms.length || ms.length > 50 || !m) return null;
    var s = { mentions: ms, comment: selectionUnescape(m[1]), images: [] }, tail = m[2];
    if (tail) {
      var lines = tail.split("\n");
      if (lines.shift() !== "") return null;
      for (var i = 0; i < lines.length; i++) {
        var image = /^\[Image #([1-9][0-9]{0,5})\]$/.exec(lines[i]);
        if (!image || s.images.indexOf(+image[1]) >= 0) return null;
        s.images.push(+image[1]);
      }
    }
    return selectionBlock(s) === text ? s : null;
  }
  // Locate envelopes with the same opacity boundary as the server lexer:
  // quoted attributes/comments/CDATA/instructions cannot expose nested groups.
  function selectionTags(text) {
    var tags = [], depth = 0, pos = 0;
    while (pos < text.length) {
      var start = text.indexOf("<", pos);
      if (start < 0) break;
      var body = text.slice(start), term = body.indexOf("<!--") === 0 ? "-->" : body.indexOf("<![CDATA[") === 0 ? "]]>" : body.indexOf("<?") === 0 ? "?>" : "";
      if (!term && body.indexOf("<!") === 0) return tags;
      if (term) {
        var stop = body.indexOf(term);
        if (stop < 0) return tags;
        pos = start + stop + term.length; continue;
      }
      var candidate = /^<\/?wb:selection(?:[^A-Za-z0-9_.:-]|$)/.test(body), quote = "", end = start + 1;
      for (; end < text.length; end++) {
        var ch = text.charAt(end);
        if (quote) { if (ch === quote) quote = ""; continue; }
        if (ch === '"' || ch === "'") quote = ch;
        else if (ch === ">" || ch === "<") break;
      }
      if (end === text.length || text.charAt(end) !== ">") {
        if (candidate) { tags.push([start, text.length]); return tags; }
        if (depth > 0) return tags;
        pos = start + 1; continue;
      }
      pos = end + 1;
      if (candidate) {
        tags.push([start, pos]);
        if (text.charAt(start + 1) !== "/") depth++;
        else if (depth > 0) depth--;
      }
    }
    return tags;
  }
  function splitSelectionText(text) {
    var tags = selectionTags(text), groups = [], cuts = [], out = "", last = 0;
    for (var i = 0; i < tags.length; i++) {
      var start = tags[i][0];
      if (text.charAt(start + 1) === "/") continue;
      var depth = 1, end = text.length;
      for (i++; i < tags.length; i++) {
        depth += text.charAt(tags[i][0] + 1) === "/" ? -1 : 1;
        if (!depth) { end = tags[i][1]; break; }
      }
      var s = (start === 0 || text.charAt(start - 1) === "\n") && (end === text.length || /^\r?\n/.test(text.slice(end))) ? readSelectionBlock(text.slice(start, end)) : null;
      if (!s) continue;
      groups.push(s);
      var separator = /^(?:\r?\n){1,2}/.exec(text.slice(end)), cutEnd = end + (separator ? separator[0].length : 0);
      out += text.slice(last, start); last = cutEnd; cuts.push([start, cutEnd]);
    }
    return { text: out + text.slice(last), groups: groups, cuts: cuts };
  }
  function selectionRanges(ranges, cuts) {
    return (Array.isArray(ranges) ? ranges : []).filter(function (r) {
      return r && !cuts.some(function (c) { return c[0] < r.end && c[1] > r.start; });
    }).map(function (r) {
      var shift = 0;
      cuts.forEach(function (c) { if (c[1] <= r.start) shift += c[1] - c[0]; });
      return Object.assign({}, r, { start: r.start - shift, end: r.end - shift });
    });
  }
  function selectionPrefix(groups) { return groups.length ? groups.map(selectionBlock).join("\n\n") + "\n\n" : ""; }
