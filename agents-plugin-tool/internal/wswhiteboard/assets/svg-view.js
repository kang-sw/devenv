// Shared by the viewer (viewer.js, on the host page) and a topic-body frame
// (doc-frame.js): which inline SVG a click opens in the viewer, and how it is
// flattened into a standalone, inert SVG document the viewer shows as an image.
// Plain function declarations: each consumer wraps them in its own scope.

// wbSvgTarget is the outermost viewable <svg> a click on t lands in, else
// null: a mermaid diagram, or an inline drawing of the board's text (not a
// sprite icon, not part of a link or control, not a small glyph).
function wbSvgTarget(t) {
  var s = t && t.closest && t.closest("svg");
  if (!s) return null;
  for (var o = s.parentNode && s.parentNode.closest && s.parentNode.closest("svg"); o; o = s.parentNode && s.parentNode.closest && s.parentNode.closest("svg")) s = o;
  if (s.classList.contains("i")) return null;
  if (!s.closest("pre.mermaid")) {
    if (!s.closest(".md, .doc-body") || s.closest("a, button, summary, label, [role=button]")) return null;
  }
  var r = s.getBoundingClientRect();
  return r.width >= 48 && r.height >= 48 ? s : null;
}

// wbFlattenSvg serialises src as a standalone SVG document: the computed
// presentation of every element is written inline (the page's stylesheet and
// its var(...) colours do not exist inside an image), scripts and handlers are
// dropped, and the root gets its natural size. It returns "" when src has no
// usable size.
function wbFlattenSvg(src) {
  var SVG = "http://www.w3.org/2000/svg", XHTML = "http://www.w3.org/1999/xhtml";
  var gfx = ["fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity", "stroke-dasharray",
    "stroke-linecap", "stroke-linejoin", "opacity", "color", "font-family", "font-size", "font-weight", "font-style",
    "letter-spacing", "text-anchor", "dominant-baseline", "visibility", "display"];
  var htm = ["color", "font-family", "font-size", "font-weight", "font-style", "line-height", "text-align", "white-space"];
  var skip = /^(style|script|title|desc|metadata|defs|lineargradient|radialgradient|stop|filter|fe[a-z]+)$/i;
  var box = src.getBoundingClientRect(), vb = src.viewBox && src.viewBox.baseVal;
  var w = vb && vb.width > 0 ? vb.width : box.width, h = vb && vb.height > 0 ? vb.height : box.height;
  if (!(w > 0 && h > 0)) return "";
  var clone = src.cloneNode(true);
  var from = [src].concat(Array.prototype.slice.call(src.querySelectorAll("*")));
  var to = [clone].concat(Array.prototype.slice.call(clone.querySelectorAll("*")));
  for (var i = 0; i < from.length && i < to.length; i++) {
    var el = to[i];
    for (var j = el.attributes.length - 1; j >= 0; j--) {
      if (/^on/i.test(el.attributes[j].name)) el.removeAttribute(el.attributes[j].name);
    }
    if (skip.test(el.localName)) continue;
    var cs = getComputedStyle(from[i]), names = el.namespaceURI === XHTML ? htm : gfx, css = "";
    for (var k = 0; k < names.length; k++) {
      var v = cs.getPropertyValue(names[k]);
      if (v) css += names[k] + ":" + v + ";";
    }
    if (el.localName === "rect") {
      var rx = cs.getPropertyValue("rx"), ry = cs.getPropertyValue("ry");
      if (rx && rx !== "auto") css += "rx:" + rx + ";";
      if (ry && ry !== "auto") css += "ry:" + ry + ";";
    }
    var old = el.getAttribute("style");
    el.setAttribute("style", (old ? old.replace(/;?\s*$/, ";") : "") + css);
  }
  Array.prototype.forEach.call(clone.querySelectorAll("script"), function (s) { s.parentNode.removeChild(s); });
  clone.removeAttribute("style");
  clone.removeAttribute("class");
  clone.setAttribute("xmlns", SVG);
  clone.setAttribute("width", String(w));
  clone.setAttribute("height", String(h));
  if (!clone.getAttribute("viewBox")) clone.setAttribute("viewBox", "0 0 " + w + " " + h);
  var out = new XMLSerializer().serializeToString(clone);
  return out.length > 30000000 ? "" : out;
}
