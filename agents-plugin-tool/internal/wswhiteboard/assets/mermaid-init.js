(function () {
  // Runs after the shared mermaid script beside the page. A host that cannot
  // load that script (a sandboxed or srcdoc embed) leaves window.mermaid
  // unset, and each <pre class="mermaid"> keeps showing its readable source.
  var m = window.mermaid;
  if (!m) return;
  var doc = document;
  // mermaid's colour math cannot read var(...) strings: resolve every --wb-*
  // colour to the value the page computes for it before initializing.
  var probe = doc.createElement("span");
  probe.style.display = "none";
  doc.body.appendChild(probe);
  function c(name) {
    probe.style.color = "var(" + name + ")";
    return getComputedStyle(probe).color;
  }
  var card = c("--wb-card"), fg = c("--wb-fg"), page = c("--wb-bg"),
    muted = c("--wb-muted"), mutedFg = c("--wb-muted-fg"),
    now = c("--wb-now"), onNow = c("--wb-on-now");
  doc.body.removeChild(probe);
  var dark = !!(window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
  m.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: "base",
    darkMode: dark,
    fontFamily: getComputedStyle(doc.body).fontFamily,
    themeVariables: {
      darkMode: dark,
      background: card,
      fontFamily: getComputedStyle(doc.body).fontFamily,
      primaryColor: muted, primaryTextColor: fg, primaryBorderColor: muted,
      secondaryColor: now, secondaryTextColor: onNow, secondaryBorderColor: now,
      tertiaryColor: page, tertiaryTextColor: fg, tertiaryBorderColor: muted,
      mainBkg: muted, nodeBorder: muted, nodeTextColor: fg, textColor: fg, titleColor: fg,
      lineColor: mutedFg, edgeLabelBackground: card,
      clusterBkg: page, clusterBorder: muted,
      noteBkgColor: page, noteTextColor: fg, noteBorderColor: mutedFg,
      actorBkg: muted, actorBorder: muted, actorTextColor: fg, actorLineColor: mutedFg,
      signalColor: mutedFg, signalTextColor: fg, labelBoxBkgColor: muted, labelTextColor: fg,
      sequenceNumberColor: onNow
    },
    // The look of the page's drawing kit (.box, .store, .edge, .sub): filled
    // rounded boxes without outlines, a "store" class (A:::store) filled with
    // the Now colour, thin muted edges, small muted edge labels. The SVG sits
    // in the page, so var(...) works here even though it cannot above.
    themeCSS:
      ".node rect, .node polygon, .node circle, .node ellipse, .node path { fill: var(--wb-muted); stroke: none; }" +
      ".node rect { rx: 8px; ry: 8px; }" +
      ".node .label, .nodeLabel { color: var(--wb-fg); fill: var(--wb-fg); }" +
      ".node.store rect, .node.store polygon, .node.store circle, .node.store ellipse, .node.store path { fill: var(--wb-now); }" +
      ".node.store .label, .node.store .nodeLabel { color: var(--wb-on-now); fill: var(--wb-on-now); }" +
      ".cluster rect { fill: var(--wb-bg); stroke: var(--wb-muted); rx: 10px; ry: 10px; }" +
      ".flowchart-link, .edgePath .path, .relation, .messageLine0, .messageLine1, .actor-line { stroke: var(--wb-muted-fg); stroke-width: 1.4px; }" +
      ".marker, marker path, .arrowheadPath { fill: var(--wb-muted-fg); stroke: var(--wb-muted-fg); }" +
      ".edgeLabel, .edgeLabel p, .edgeLabel span { color: var(--wb-muted-fg); font-size: 10.5px; background: var(--wb-card); }"
  });
  m.run({ querySelector: "pre.mermaid" });
})();
