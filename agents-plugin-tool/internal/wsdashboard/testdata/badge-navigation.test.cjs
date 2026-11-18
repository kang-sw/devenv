// Run locally with node; Go also supplies real template-rendered fixtures.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const fs = require('node:fs');
const path = require('node:path');
const template = fs.readFileSync(path.join(__dirname, '../assets/dashboard.tmpl'), 'utf8');
assert(template.includes('{{range .AnswerChips}}'));
assert(template.includes('{{range .MentionChips}}'));
assert(template.includes('role="link" tabindex="0" data-wb-nav="{{.Target}}" data-wb-topic="{{.TopicTarget}}"'));
assert(template.includes('{{range .Parts}}{{if .Target}}<span class="wbref"'));
if (process.argv[2]) {
  const { chips, card } = JSON.parse(process.argv[2]);
  for (const [id, topic] of [[44,'706c616e'],[45,'6f74686572'],[46,'706c616e'],[51,'6f74686572']]) {
    assert(chips.includes(`data-wb-nav="item-${id}" data-wb-topic="topic-${topic}"`));
    assert(chips.includes(`<b>(${id})</b> ok`));
  }
  assert(!chips.includes('44..46'));
  assert(chips.includes('data-wb-nav="e8" data-wb-topic="topic-6f74686572"'));
  assert(card.includes('data-wb-nav="item-58" data-wb-topic="topic-706c616e">(58)</span>'));
  assert(card.includes('data-wb-nav="item-999" data-wb-topic="">(999)</span>'));
  assert(card.includes('&lt;script&gt;x&lt;/script&gt;'));
  assert(!card.includes('<script>'));
  assert(card.includes('{{x}}'));
}
const src = readAssetSource('wsdashboard', 'dashboard-conversation.js');
const code = src.slice(src.indexOf('  function conversationBoardActivate('), src.indexOf('  // --- chat block actions'));
assert(code.includes('effectJump(topic, target,'));
const calls = [], notices = [], handlers = {};
let found = true, exact = true, owned = true, held = false, selected = false;
const ctx = vm.createContext({
  doc: { addEventListener(k, f) { handlers[k] = f; } },
  entriesBox() { return { contains() { return owned; } }; },
  region() { return { contains() { return held; } }; },
  window: { getSelection() { return { isCollapsed: !selected }; } },
  effectJump(topic, id, done) { calls.push([topic, id]); done(found, exact); },
  wsT(k) { return k; }, toast(s) { notices.push(s); }
});
vm.runInContext(code, ctx);
function event(target = 'item-58', topic = 'topic-706c616e', extra = {}) {
  const link = { getAttribute(k) { return k === 'data-wb-nav' ? target : topic; } };
  return Object.assign({ type: 'click', button: 0, target: { closest() { return link; } }, preventDefault() { this.prevented = true; } }, extra);
}
let e = event(); handlers.click(e);
assert(e.prevented); assert.deepEqual(calls.pop(), ['topic-706c616e', 'item-58']);
e = event('e8','topic-6f74686572',{type:'keydown', key:'Enter'}); handlers.keydown(e);
assert(e.prevented); assert.deepEqual(calls.pop(), ['topic-6f74686572','e8']);
owned = false; held = true; handlers.click(event('item-44')); assert.equal(calls.pop()[1], 'item-44');
held = false; handlers.click(event()); assert.equal(calls.length, 0);
owned = true; handlers.click(event('item-999','')); assert.equal(calls.length, 0);
assert.equal(notices.pop(), 'js.whiteboard.jump.itemmissing');
found = false; handlers.click(event()); assert.equal(notices.pop(), 'js.whiteboard.jump.itemmissing');
found = true; exact = false; handlers.click(event()); assert.equal(notices.pop(), 'js.whiteboard.jump.itemmissing');
calls.length = 0;
handlers.click(event('item-58', 'topic-706c616e', {ctrlKey:true}));
handlers.keydown(event('item-58','topic-706c616e',{type:'keydown',key:'Escape'}));
selected = true; handlers.click(event());
assert.equal(calls.length, 0, 'modified clicks, other keys and selected text stay untouched');
console.log('badge navigation contract ok (rendered fixtures: ' + !!process.argv[2] + ')');
