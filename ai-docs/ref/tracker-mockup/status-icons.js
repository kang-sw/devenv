// Tracker status icons (board (355)): every status differs by shape as well as
// colour, so it reads without colour too, and carries one short description for
// the hover tooltip. This file is the one place the set lives, so the dashboard
// can adopt it as is: shapes are 16x16 inline SVG drawn with currentColor, and
// `tone` names the CSS custom property that colours the icon (the dashboard and
// this mockup share those names).
(function () {
  'use strict';
  var RING = '<circle cx="8" cy="8" r="5.6"/>';
  var SET = {
    todo:       { label: 'todo', desc: 'assigned, not started', tone: 'primary', svg: RING },
    doing:      { label: 'doing', desc: 'in progress', tone: 'primary', svg: RING + '<path d="M8 2.4a5.6 5.6 0 0 1 0 11.2z" fill="currentColor"/>' },
    blocked:    { label: 'blocked', desc: 'stuck; the note says on what', tone: 'fail', svg: RING + '<path d="M4.1 4.1l7.8 7.8"/>' },
    awaiting:   { label: 'awaiting confirm', desc: 'closed by the assignee; waits for the assigner to confirm', tone: 'warn',
      svg: '<path d="M4.5 2.4h7M4.5 13.6h7M5.4 2.4c0 3.2 5.2 3.4 5.2 5.6s-5.2 2.4-5.2 5.6M10.6 2.4c0 3.2-5.2 3.4-5.2 5.6s5.2 2.4 5.2 5.6"/>' },
    done:       { label: 'done', desc: 'finished', tone: 'ok', svg: '<circle cx="8" cy="8" r="6" fill="currentColor" stroke="none"/><path d="M5.3 8.2l1.9 1.9 3.6-3.8" stroke="var(--card)"/>' },
    pending:    { label: 'pending', desc: 'sent, not accepted yet; not counted in status or roll-ups', tone: 'teal',
      svg: '<circle cx="8" cy="8" r="5.6" stroke-dasharray="2.2 1.9"/><path d="M6.5 6.6a1.55 1.55 0 1 1 2.2 1.4c-.5.25-.7.6-.7 1.1M8 10.9v.05"/>' },
    unassigned: { label: 'unassigned', desc: 'nobody holds it', tone: 'muted-fg', svg: '<circle cx="8" cy="8" r="5.6" stroke-dasharray="0.8 2.2"/>' },
    declined:   { label: 'declined', desc: 'the target declined the pending assignment', tone: 'muted-fg', svg: RING + '<path d="M5.9 5.9l4.2 4.2M10.1 5.9l-4.2 4.2"/>' },
    returned:   { label: 'returned', desc: 'the accepter\'s lead returned it; the assigner may send it again', tone: 'muted-fg', svg: RING + '<path d="M10.4 10.4V8.9a1.9 1.9 0 0 0-1.9-1.9H5.6M7.1 5.4L5.5 7l1.6 1.6"/>' },
    handed:     { label: 'handed off', desc: 'the holder handed it to someone who accepted', tone: 'muted-fg', svg: RING + '<path d="M5.2 8h5.2M8.6 6l2 2-2 2"/>' },
    withdrawn:  { label: 'withdrawn', desc: 'the sender withdrew it, or its original ended', tone: 'muted-fg', svg: RING + '<path d="M5.4 8h5.2"/>' },
    topic:      { label: 'topic', desc: 'discussion only; a topic has no status', tone: 'muted-fg', svg: '<path d="M2.8 4h10.4v6.6H7.4l-2.8 2.4v-2.4H2.8z"/>' }
  };
  function info(key) { return SET[key] || SET.unassigned; }
  // The icon as inline SVG; the caller wraps it with its own tooltip trigger.
  function svg(key) {
    var s = info(key);
    return '<svg class="sti-svg" viewBox="0 0 16 16" aria-hidden="true" style="color:var(--' + s.tone + ')">' + s.svg + '</svg>';
  }
  window.TrackerStatusIcons = { set: SET, info: info, svg: svg };
})();
