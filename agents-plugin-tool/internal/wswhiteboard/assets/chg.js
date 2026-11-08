// Draws a topic body's change marks as thin bars in the gutter left of its
// text column: green beside a new block, blue beside a rewritten one, and a
// short red tick where a block was deleted. The server leaves the marks in
// the body as comments (changes.go): <!--wbchg:new-->, <!--wbchg:mod-->, and
// <!--wbchg:del--> go before the block they mark, <!--wbchg:delafter--> after
// the last block. The bars live in a zero-height layer after the body, so the
// body's own layout and :first-child/:last-child rules stay as written.
function wbChangeBars(body) {
  var marks = [], walk = document.createTreeWalker(body, NodeFilter.SHOW_COMMENT), c;
  while ((c = walk.nextNode())) if (c.data.indexOf("wbchg:") === 0) marks.push(c);
  var layer = body.nextElementSibling;
  if (layer && !layer.classList.contains("wb-chg-layer")) layer = null;
  if (!marks.length) { if (layer) layer.remove(); return; }
  if (!layer) {
    layer = document.createElement("div");
    layer.className = "wb-chg-layer";
    body.parentNode.insertBefore(layer, body.nextSibling);
  }
  layer.textContent = "";
  var origin = layer.getBoundingClientRect(), box = body.getBoundingClientRect();
  if (!box.width) return;
  // 9px left of the text column; the body's padding is the gutter.
  var x = box.left + parseFloat(getComputedStyle(body).paddingLeft || "0") - 9 - origin.left;
  function bar(kind, top, height, wide) {
    var b = document.createElement("div");
    b.className = "wb-chg-bar " + kind;
    b.style.left = (wide ? x - 3 : x) + "px";
    b.style.top = top + "px";
    if (height) b.style.height = height + "px";
    layer.appendChild(b);
  }
  marks.forEach(function (m) {
    var kind = m.data.slice(6), el = kind === "delafter" ? m.previousElementSibling : m.nextElementSibling;
    if (!el) return;
    var r = el.getBoundingClientRect();
    if (kind === "new" || kind === "mod") {
      // A list item's bar stops where its nested list starts: the nested
      // items carry their own marks.
      var h = r.height, nested = el.querySelector(":scope > ul, :scope > ol");
      if (nested) h = nested.getBoundingClientRect().top - r.top;
      if (h > 0) bar(kind, r.top - origin.top, h, false);
    } else {
      bar("del", (kind === "del" ? r.top : r.bottom) - origin.top - 1, 0, true);
    }
  });
}

// wbWatchChanges keeps body's bars in step with its layout: a body that opens,
// resizes, or loads an image lays out again.
function wbWatchChanges(body) {
  wbChangeBars(body);
  if (window.ResizeObserver) new ResizeObserver(function () { wbChangeBars(body); }).observe(body);
  window.addEventListener("load", function () { wbChangeBars(body); });
}
