'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const conversationSource = fs.readFileSync(__dirname + '/assets/dashboard-conversation.js', 'utf8');
const jumpSection = conversationSource.slice(
  conversationSource.indexOf('  // --- chat jump arrow'),
  conversationSource.indexOf('  // mermaid draws')
);

function makeJumpContext(opts = {}) {
  const listeners = {}, frames = [], scrolled = [];
  class El {
    constructor(tag) {
      this.tag = tag;
      this.children = [];
      this.attrs = {};
      this.hidden = false;
      this.title = '';
      this.isConnected = true;
      this.listeners = {};
    }
    get firstChild() { return this.children[0] || null; }
    appendChild(c) { this.children.push(c); return c; }
    insertBefore(c) { this.children.unshift(c); return c; }
    getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; }
    setAttribute(k, v) { this.attrs[k] = String(v); }
    addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  }
  let msgs = opts.msgs || [];
  let convoRect = opts.convoRect || { top: 100, bottom: 600, width: 400, height: 500 };
  let isVisible = opts.visible !== false;
  const convo = new El('div');
  convo.getBoundingClientRect = () => convoRect;
  convo.querySelectorAll = sel => (sel === '.msg.user' ? msgs : []);
  convo.addEventListener = (t, fn) => { listeners['convo:' + t] = fn; };

  const context = {
    convo,
    stick: true,
    visible: () => isVisible,
    reducedMotion: () => true,
    wsT: k => k,
    doc: { createElement: tag => new El(tag) },
    window: { addEventListener: (t, fn) => { listeners['win:' + t] = fn; } },
    requestAnimationFrame: fn => { frames.push(fn); return frames.length; }
  };
  vm.createContext(context);
  vm.runInContext(
    jumpSection + '\nthis.userJumpTarget = userJumpTarget; this.updateUserJump = updateUserJump; this.getBtn = () => userJumpBtn; this.getStick = () => stick;',
    context
  );
  function flush() { while (frames.length) frames.shift()(); }
  function setMsgs(list) { msgs = list; }
  function setConvoRect(r) { convoRect = r; }
  function setVisible(v) { isVisible = v; }
  function msg(id, top, height = 40, width = 200) {
    return {
      id,
      isConnected: true,
      getBoundingClientRect: () => ({ top, bottom: top + height, width, height }),
      scrollIntoView: arg => scrolled.push([id, { block: arg.block, behavior: arg.behavior }])
    };
  }
  return { context, flush, setMsgs, setConvoRect, setVisible, msg, scrolled, listeners };
}

test('userJumpTarget selects latest off-screen own message, direction, and hides when latest is visible', () => {
  const h = makeJumpContext();
  const { userJumpTarget } = h.context;
  const m1 = h.msg('u1', -200);
  const m2 = h.msg('u2', 20);
  const m3 = h.msg('u3', 300); // visible inside [100, 600]
  const m4 = h.msg('u4', 700); // below [100, 600]
  const hidden = h.msg('uHidden', -500, 0, 0);

  // No messages -> hidden (null).
  assert.equal(userJumpTarget([], 100, 600), null);

  // Latest own message (m3) is visible even though m1, m2 are above -> hidden (null).
  assert.equal(userJumpTarget([m1, m2, m3], 100, 600), null);

  // Latest own message (m2) is above viewport (bottom 60 <= 100) -> points up to m2.
  const up = userJumpTarget([m1, m2], 100, 600);
  assert.equal(up.el.id, 'u2');
  assert.equal(up.dir, 'up');

  // Latest own message (m4) is below viewport (top 700 >= 600) -> points down to m4.
  const down = userJumpTarget([m1, m2, m4], 100, 600);
  assert.equal(down.el.id, 'u4');
  assert.equal(down.dir, 'down');

  // Zero-rect trailing message is skipped so the latest rendered own message governs.
  const skipZero = userJumpTarget([m1, m2, hidden], 100, 600);
  assert.equal(skipZero.el.id, 'u2');
  assert.equal(skipZero.dir, 'up');

  // Partially intersecting latest message counts as visible -> hidden.
  const partialTop = h.msg('uPart', 80, 40); // bottom 120 > 100
  assert.equal(userJumpTarget([m1, partialTop], 100, 600), null);
});

test('chat jump button updates direction/visibility and scrolls to target on click', () => {
  const h = makeJumpContext();
  const u1 = h.msg('u1', -120);
  const u2 = h.msg('u2', 40); // above [100, 600] (bottom = 80 <= 100)
  h.setMsgs([u1, u2]);
  h.flush();

  const btn = h.context.getBtn();
  assert.equal(btn.hidden, false);
  assert.equal(btn.getAttribute('data-dir'), 'up');
  assert.equal(btn.title, 'js.convo.jump.up');

  // Clicking scrolls to u2 and releases bottom stick.
  btn.listeners.click[0]();
  assert.deepEqual(h.scrolled, [['u2', { block: 'center', behavior: 'instant' }]]);
  assert.equal(h.context.getStick(), false);

  // When latest own message is below viewport -> points down.
  const u3 = h.msg('u3', 680);
  h.setMsgs([u1, u2, u3]);
  h.listeners['convo:scroll']();
  h.flush();
  assert.equal(btn.hidden, false);
  assert.equal(btn.getAttribute('data-dir'), 'down');
  assert.equal(btn.title, 'js.convo.jump.down');

  // When latest own message scrolls into view -> hides.
  const u3Visible = h.msg('u3', 260);
  h.setMsgs([u1, u2, u3Visible]);
  h.listeners['convo:scroll']();
  h.flush();
  assert.equal(btn.hidden, true);
});
