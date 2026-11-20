const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function fn(name) {
  const match = source.match(new RegExp('\\n  function ' + name + '\\([^]*?\\n  \\}\\n'));
  assert.ok(match, name); return match[0];
}
const field = { value: '', wsc: null, focus() {} };
const rows = [];
const inserted = [];
const edit = {
  ordered: () => rows,
  snapshot: () => rows.slice(),
  insertCapsule(kind, payload, options) { inserted.push({ kind, payload, options }); return { kind, payload }; }
};
field.wsc = edit;
const topic = { value: 'review topic', dispatchEvent() {} };
const context = vm.createContext({
  window: { WsCapsules: { command: () => null } }, ctext: field, cchips: {},
  bselections: [], bmentions: [], wpicks: [], wconsoles: [], closeRequests: [], imgs: [], mpend: null,
  sending: false, cmdSel: null, boardOwn: {}, boardIncarnation: '', rid: 'r1',
  inlineEditor: () => edit, inlineRows: kind => rows.filter(r => r.kind === kind),
  topicNotes: () => [{ topic: 'topic-a', value: topic.value, el: topic }],
  topicNote: () => topic,
  chipList: () => rows.filter(r => r.kind === 'answer').map(r => r.payload.answer),
  selectionPrefix: () => '', selectionBlock: s => '<wb:selection>' + s.comment + '</wb:selection>',
  outboxUnsent: () => false, noteWarn: s => { throw new Error(s); },
  lastUserEntry: () => '', outboxAdd: m => { context.sent = m.body; },
  saveDraft() {}, grow() {}, clearMentions() {}, dropChip() {}, setItem() {}, closeTopicNotes() {},
  clearTrash() {}, renderChips() {}, renderImgs() {}, dropImg() {}, boardItem: () => null,
  queueDraft() {},
  Date, Math, Event
});
vm.runInContext(fn('inlineSend') + fn('addPanelMention') + fn('addTicketMention') + fn('sendMessage'), context);

function put(kind, key, anchor, payload) {
  const start = field.value.indexOf(anchor);
  assert(start >= 0, anchor);
  rows.push({ kind, key, start, end: start + anchor.length, payload });
  rows.sort((a, b) => a.start - b.start);
}
field.value = 'Please (51)=a compare {{p3}} with [ticket:261120-feat-example] and [pick:#btn].';
put('answer', '51', '(51)=a', { anchor: '(51)=a', answer: { id: '51', answer: 'a', note: '' } });
put('mention', 'panel-1', '{{p3}}', { anchor: '{{p3}}', panel: 'p3', mention: { topic: 'roadmap', text: 'p3 · Recap' } });
put('ticket', 'ticket-1', '[ticket:261120-feat-example]', { anchor: '[ticket:261120-feat-example]', stem: '261120-feat-example' });
put('pick', 'pick-1', '[pick:#btn]', { anchor: '[pick:#btn]', pick: { url: 'https://example.test', selector: '#btn', text: 'Button' } });
const data = context.inlineSend();
assert.equal(data.text, field.value, 'inline anchors remain at their prose positions');
assert.deepEqual(JSON.parse(JSON.stringify(data.mentions)), [{ topic: 'roadmap', text: 'p3 · Recap' }]);
assert.deepEqual(JSON.parse(JSON.stringify(data.picks)), [{ url: 'https://example.test', selector: '#btn', text: 'Button' }]);
context.sendMessage();
assert.equal(context.sent.text, 'Please (51)=a compare {{p3}} with [ticket:261120-feat-example] and [pick:#btn].');
assert.deepEqual(JSON.parse(JSON.stringify(context.sent.chips)), [{ id: '51', answer: 'a', note: '' }]);
assert.deepEqual(JSON.parse(JSON.stringify(context.sent.mentions)), [{ topic: 'roadmap', text: 'p3 · Recap' }]);
assert.deepEqual(JSON.parse(JSON.stringify(context.sent.picks)), [{ url: 'https://example.test', selector: '#btn', text: 'Button' }]);

field.value = '(51)=a '; rows.length = 0;
put('answer', '51', '(51)=a', { anchor: '(51)=a', answer: { id: '51', answer: 'a' } });
assert.equal(context.inlineSend().text, '', 'answer-only send retains the historical empty text field');
field.value = '[note:topic-a] '; rows.length = 0;
put('topic-note', 'topic-a', '[note:topic-a]', { anchor: '[note:topic-a]', topic: 'topic-a', note: 'review topic' });
const noteOnly = context.inlineSend();
assert.equal(noteOnly.text, '', 'topic-note-only send also keeps text empty');
assert.deepEqual(JSON.parse(JSON.stringify(noteOnly.notes.map(n => ({ topic: n.topic, value: n.value })))), [{ topic: 'topic-a', value: 'review topic' }]);

const panel = context.addPanelMention({ topic: 'roadmap', panel: 'p3', entry: 'e7', recap: 'Recap' });
assert.equal(panel.payload.anchor, '{{p3}}');
assert.deepEqual(JSON.parse(JSON.stringify(panel.payload.mention)), { topic: 'roadmap', text: 'p3 · Recap', entry: 'e7' });
const ticket = context.addTicketMention({ stem: '261120-feat-example', title: 'Example', status: 'ready', path: 'ready/261120-feat-example.md' });
assert.equal(ticket.payload.anchor, '[ticket:261120-feat-example]');
assert.equal(ticket.payload.path, 'ready/261120-feat-example.md');
assert.equal(inserted.length, 2);
console.log('composer inline: ordered T0 send fields, answer-only empty text, panel/ticket helper metadata passed');
