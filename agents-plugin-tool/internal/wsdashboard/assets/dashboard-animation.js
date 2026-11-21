  // A shared clock keeps newly mounted spinners at the phase their predecessors
  // would have reached. Reading CSS duration here would force style resolution.
  var ANIMATION_PERIOD_MS = { thinking: 1150, tool: 900, holding: 1800, permission: 1150, hbr: 1100, spin: 800 };
  function animationPhaseOffset(seedKey, periodMs) {
    if (!seedKey) return 0;
    var hash = 2166136261;
    for (var i = 0; i < seedKey.length; i++) {
      hash ^= seedKey.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return (hash >>> 0) % periodMs;
  }
  function animationTimelineNow() {
    if (doc.documentElement && doc.documentElement.hasAttribute("data-animation-paused")) return 0;
    if (doc.timeline && typeof doc.timeline.currentTime === "number") return doc.timeline.currentTime;
    return typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
  }
  function animationPhaseDelay(el, periodMs, seedKey, stateKey) {
    var stamp = stateKey + "|" + seedKey + "|" + periodMs;
    if (el.wsAnimationStamp === stamp) return;
    var now = animationTimelineNow(), offset = animationPhaseOffset(seedKey, periodMs);
    var phase = ((now + offset) % periodMs + periodMs) % periodMs;
    el.style.animationDelay = (-phase).toFixed(1) + "ms";
    el.wsAnimationStamp = stamp;
  }
  function animationSeed(el, selector) {
    var owner = el.closest(selector);
    if (!owner) return "";
    return owner.getAttribute("data-sid") || owner.getAttribute("data-rid") || owner.getAttribute("data-id") || owner.getAttribute("data-key") || owner.getAttribute("href") || "";
  }
  function syncSpinnerPhases(root) {
    if (!root || !root.querySelectorAll) return;
    function visit(selector, apply) {
      if (root.matches && root.matches(selector)) apply(root);
      root.querySelectorAll(selector).forEach(apply);
    }
    visit(".hb .hbr", function (el) {
      animationPhaseDelay(el, ANIMATION_PERIOD_MS.hbr, animationSeed(el, "[data-sid], [data-rid], [data-id], [data-key], a[href]"), "hbr");
    });
    visit(".spin", function (el) {
      animationPhaseDelay(el, ANIMATION_PERIOD_MS.spin, animationSeed(el, "[data-key], [data-sid], [data-rid], [data-id], a[href]"), "spin");
    });
  }
