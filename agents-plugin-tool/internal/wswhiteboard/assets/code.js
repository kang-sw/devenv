// Code blocks: a fenced block (pre > code) in an item field or a topic body
// gets a copy button and, from two lines on, a line-number gutter. The pre is
// wrapped in div.cb, which carries the button, so the button stays put while a
// wide block scrolls; the numbers are their own element (span.ln, not
// selectable), so a drag selection and the copy take the code text alone.
// A block is wired once (pre.wbCode): a dashboard board swap and a fetched
// closed topic run this over nodes the page kept.
function wbCodeBlocks(root) {
  var d = document, copyIcon = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.2V3a.8.8 0 0 0-.8-.8H3a.8.8 0 0 0-.8.8v6.7c0 .4.4.8.8.8h.2"/></svg>';
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
    var lines = code.textContent.replace(/\n$/, "").split("\n").length;
    var wrap = d.createElement("div"), btn = d.createElement("button"), timer = 0;
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
    btn.setAttribute("title", "복사");
    btn.setAttribute("aria-label", "복사");
    btn.innerHTML = copyIcon;
    btn.addEventListener("click", function (e) {
      // A block inside an option or a summary copies and does nothing else.
      if (e.stopPropagation) e.stopPropagation();
      if (!copyText(code.textContent)) return;
      btn.className = "cb-copy done";
      btn.textContent = "복사됨";
      clearTimeout(timer);
      timer = setTimeout(function () { btn.className = "cb-copy"; btn.innerHTML = copyIcon; }, 1500);
    });
    wrap.appendChild(btn);
  });
}
wbCodeBlocks(document);
if (window.wbBindBoard) {
  (function (bind) {
    window.wbBindBoard = function (root) { bind(root); wbCodeBlocks(root); };
  })(window.wbBindBoard);
}
