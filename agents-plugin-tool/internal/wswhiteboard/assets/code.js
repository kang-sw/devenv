// Code blocks: a fenced block (pre > code) in an item field or a topic body
// gets a copy button and, from two lines on, a line-number gutter. The pre is
// wrapped in div.cb, which carries the button, so the button stays put while a
// wide block scrolls; the numbers are their own element (span.ln, not
// selectable), so a drag selection and the copy take the code text alone.
// A second button toggles line wrapping (off by default; the choice is kept in
// localStorage under one key and read by blocks wired later). A wrapped
// numbered block drops the gutter and splits its text into one span.cl per
// line, each carrying its own number (data-n, drawn by CSS), so every number
// sits at the top of its wrapped line whatever the width; unwrapping restores
// the plain text.
// A block is wired once (pre.wbCode): a dashboard board swap and a fetched
// closed topic run this over nodes the page kept.
function wbCodeBlocks(root) {
  var d = document, copyIcon = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.2V3a.8.8 0 0 0-.8-.8H3a.8.8 0 0 0-.8.8v6.7c0 .4.4.8.8.8h.2"/></svg>';
  var wrapIcon = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M2 3.5h12M2 12.5h4M2 8h9.5a2 2 0 0 1 0 4H9.5"/><path d="M11 10.5 9.5 12 11 13.5"/></svg>', WRAP_KEY = "ws.wb.codeWrap";
  function savedWrap() { try { return localStorage.getItem(WRAP_KEY) === "1"; } catch (e) { return false; } }
  function saveWrap(on) { try { localStorage.setItem(WRAP_KEY, on ? "1" : "0"); } catch (e) {} }
  // Copy through a scratch textarea: the async clipboard API is denied in a
  // sandboxed topic-body frame, and a textarea selection leaves the page's own
  // selection (which the dashboard offers as a mention) alone.
  function copyText(text) {
    var ta = d.createElement("textarea"), ok = false;
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.cssText = "position:fixed;top:0;left:0;opacity:0;pointer-events:none";
    d.body.appendChild(ta);
    ta.select();
    try { ok = d.execCommand("copy"); } catch (e) { ok = false; }
    d.body.removeChild(ta);
    return ok;
  }
  Array.prototype.slice.call(root.querySelectorAll(".md pre > code")).forEach(function (code) {
    var pre = code.parentNode;
    if (pre.wbCode) return;
    pre.wbCode = true;
    var src = code.textContent, lines = src.replace(/\n$/, "").split("\n").length;
    var wrap = d.createElement("div"), btn = d.createElement("button"), wbtn = d.createElement("button"), timer = 0;
    wrap.className = "cb" + (lines > 1 ? " cb-ln" : "");
    pre.parentNode.insertBefore(wrap, pre);
    wrap.appendChild(pre);
    if (lines > 1) {
      var ln = d.createElement("span"), nums = [];
      for (var i = 1; i <= lines; i++) nums.push(i);
      ln.className = "ln";
      ln.setAttribute("aria-hidden", "true");
      ln.textContent = nums.join("\n");
      pre.insertBefore(ln, code);
    }
    btn.type = "button";
    btn.className = "cb-copy";
    var label = window.wsT ? wsT("js.wb.copy") : "Copy";
    btn.setAttribute("title", label);
    btn.setAttribute("aria-label", label);
    btn.innerHTML = copyIcon;
    btn.addEventListener("click", function (e) {
      // A block inside an option or a summary copies and does nothing else.
      if (e.stopPropagation) e.stopPropagation();
      if (!copyText(src)) return;
      btn.className = "cb-copy done";
      btn.textContent = window.wsT ? wsT("js.wb.copy.done") : "Copied";
      clearTimeout(timer);
      timer = setTimeout(function () { btn.className = "cb-copy"; btn.innerHTML = copyIcon; }, 1500);
    });
    wrap.appendChild(btn);
    function setWrap(on) {
      wrap.className = "cb" + (lines > 1 ? " cb-ln" : "") + (on ? " cb-wrap" : "");
      wbtn.className = "cb-wrapbtn" + (on ? " on" : "");
      wbtn.setAttribute("aria-pressed", on ? "true" : "false");
      if (lines < 2) return;
      code.innerHTML = "";
      code.textContent = src;
      if (!on) return;
      wrap.setAttribute("style", "--nd:" + String(lines).length);
      code.textContent = "";
      (src.match(/[^\n]*\n|[^\n]+$/g) || []).forEach(function (text, i) {
        var span = d.createElement("span");
        span.className = "cl";
        span.setAttribute("data-n", String(i + 1));
        span.textContent = text;
        code.appendChild(span);
      });
    }
    var wlabel = window.wsT ? wsT("js.wb.copy.wrap") : "Wrap long lines";
    wbtn.type = "button";
    wbtn.setAttribute("title", wlabel);
    wbtn.setAttribute("aria-label", wlabel);
    wbtn.innerHTML = wrapIcon;
    wbtn.addEventListener("click", function (e) {
      if (e.stopPropagation) e.stopPropagation();
      var on = !wrap.classList.contains("cb-wrap");
      setWrap(on);
      saveWrap(on);
    });
    wrap.appendChild(wbtn);
    setWrap(savedWrap());
  });
}
wbCodeBlocks(document);
if (window.wbBindBoard) {
  (function (bind) {
    window.wbBindBoard = function (root) { bind(root); wbCodeBlocks(root); };
  })(window.wbBindBoard);
}
