  // The answer block a reply carries ahead of its text, the grammar the
  // server's delivered message uses (wsinbox.AnswerBlock; a test holds the
  // two to the same cases):
  //   <wb:answers>
  //   123,567,691=b        items sharing a value and no note, ascending
  //   124..129,181=ok      a run of three or more consecutive IDs as a..b
  //   130=ok^note          an item with a note: its own line, note after ^
  //   131=^note            a note alone
  //   [topic]^note         a topic note, after the item lines
  //   </wb:answers>
  // In a note, newline is written \n and backslash \\. Lines follow the order
  // of the first item each covers. Self-contained: the static page runs it
  // ahead of form.js, the dashboard page as its own script.
  window.wbAnswers = (function () {
    function esc(s) {
      return s.replace(/\r\n?/g, "\n").replace(/\\/g, "\\\\").replace(/\n/g, "\\n");
    }
    function ids(list) {
      var a = list.map(function (s) { return { s: s, n: parseInt(s, 10) || 0 }; });
      // A stable sort by number: Array.prototype.sort is stable in every
      // current engine.
      a.sort(function (x, y) { return x.n - y.n; });
      var out = [];
      for (var i = 0; i < a.length;) {
        var j = i;
        while (j + 1 < a.length && a[j + 1].n === a[j].n + 1) j++;
        if (j - i >= 2) out.push(a[i].s + ".." + a[j].s);
        else for (var k = i; k <= j; k++) out.push(a[k].s);
        i = j + 1;
      }
      return out.join(",");
    }
    // block builds the answer block from items ({id, answer, note}) and topic
    // notes ({topic, note}); "" when neither carries anything.
    function block(items, topics) {
      var lines = [], groups = Object.create(null);
      (items || []).forEach(function (c) {
        var note = (c.note || "").trim(), v = c.answer || "";
        if (note) { lines.push({ text: c.id + "=" + v + "^" + esc(note) }); return; }
        if (!v) return;
        if (!groups[v]) { groups[v] = { v: v, ids: [] }; lines.push(groups[v]); }
        groups[v].ids.push(String(c.id));
      });
      var out = lines.map(function (l) { return l.ids ? ids(l.ids) + "=" + l.v : l.text; });
      (topics || []).forEach(function (t) {
        var note = (t.note || "").trim();
        if (note) out.push("[" + t.topic + "]^" + esc(note));
      });
      return out.length ? "<wb:answers>\n" + out.join("\n") + "\n</wb:answers>" : "";
    }
    return { block: block };
  })();
