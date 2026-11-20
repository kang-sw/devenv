const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const { readAssetSource } = require('./asset-source.cjs');
const assets = path.join(__dirname, '../assets');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function fn(name) {
  const m = source.match(new RegExp('\\n  function ' + name + '\\([^]*?\\n  \\}\\n'));
  assert.ok(m, name); return m[0];
}
const listener = source.slice(source.indexOf('  if (ctext) ctext.addEventListener("wsc:change"'), source.indexOf('  function chipList()', source.indexOf('  if (ctext) ctext.addEventListener("wsc:change"')));
assert(listener.includes('e.detail.removed'));

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    await page.setContent('<div class="composer"><textarea id="ctext"></textarea><button id="other">other</button><div id="board"><div class="card" data-id="51"><button class="opt" data-opt="a" aria-pressed="false">a</button><button class="opt" data-opt="b" aria-pressed="false">b</button></div><textarea class="tnote" data-topic="topic-a"></textarea></div></div>');
    await page.addScriptTag({ content: fs.readFileSync(path.join(assets, 'capsules.js'), 'utf8') });
    const setup = `
      var ctext = document.getElementById('ctext'), chips = {}, chipOrder = [], bselections = [], syncingInline = false, restoringInlineBack = false;
      var q = (s, root) => Array.from(root.querySelectorAll(s));
      var boardItem = id => document.querySelector('.card[data-id="' + id + '"]');
      var topicNote = name => document.querySelector('.tnote[data-topic="' + name + '"]');
      var boardQ = s => Array.from(document.querySelectorAll('#board ' + s));
      var topicNotes = () => boardQ('.tnote[data-topic]').map(el => ({el, topic:el.getAttribute('data-topic'), value:el.value})).filter(n => n.value.trim());
      var queueDraft = () => {}, renderChips = () => {}, setCheckNo = () => {}, dropSelection = () => {};
      var inlineEditor = () => ctext.wsc;
      var inlineRows = kind => ctext.wsc.ordered().filter(r => r.kind === kind);
      var windowComposer = window.WsComposer || {};
      WsCapsules.attach(ctext);
      ${fn('answerCapsule')}
      ${fn('topicNoteCapsule')}
      ${fn('syncInlineTopics')}
      ${fn('clearTopicNote')}
      ${fn('putChip')}
      ${fn('dropChip')}
      ${fn('setItem')}
      ${listener}
      ${fn('addPanelMention')}
      ${fn('addTicketMention')}
    `;
    await page.addScriptTag({ content: setup });
    await page.evaluate(() => {
      ctext.value = 'before after'; ctext.focus(); ctext.setSelectionRange(7, 7);
      document.getElementById('other').focus();
      putChip({ id: '51', answer: 'a', note: '' });
    });
    let state = await page.evaluate(() => ({ text: ctext.value, ranges: ctext.wsc.ordered(), boardA: document.querySelector('[data-opt="a"]').getAttribute('aria-pressed') }));
    assert.equal(state.text, 'before (51)=a after');
    assert.equal(state.ranges[0].kind, 'answer');
    await page.evaluate(() => {
      putChip({ id: '51', answer: 'b', note: '' });
      addPanelMention({ topic: 'topic-a', panel: 'p3', recap: 'Recap' });
      addTicketMention({ stem: '261120-feat-example', title: 'Example', status: 'ready', path: 'ready/261120-feat-example.md' });
    });
    state = await page.evaluate(() => ({ text: ctext.value, rows: ctext.wsc.ordered(), badges: Array.from(ctext.wsc.editor.querySelectorAll('.wsc-capsule'), n => n.textContent) }));
    assert.equal(state.rows.filter(r => r.kind === 'answer').length, 1, 'board answer updates in place');
    assert(state.text.includes('(51)=b'));
    assert(state.rows.some(r => r.kind === 'mention' && r.payload.panel === 'p3'));
    assert(state.rows.some(r => r.kind === 'ticket' && r.payload.stem === '261120-feat-example'));

    const clip = await page.evaluate(() => {
      const mention = ctext.wsc.ordered().find(r => r.kind === 'mention');
      ctext.focus(); ctext.setSelectionRange(mention.start, mention.end);
      const data = new DataTransfer();
      ctext.wsc.editor.dispatchEvent(new ClipboardEvent('cut', { clipboardData: data, bubbles: true, cancelable: true }));
      const plain = data.getData('text/plain'), structured = data.getData('application/x-wsc-capsules+json');
      ctext.wsc.editor.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
      return { plain, structured, rows: ctext.wsc.ordered() };
    });
    assert(clip.plain.includes('p3 · Recap'));
    assert(clip.structured.includes('"panel":"p3"'));
    assert(clip.rows.some(r => r.kind === 'mention' && r.payload.panel === 'p3'), 'cut/paste restores metadata');

    await page.evaluate(() => { const answer = ctext.wsc.ordered().find(r => r.kind === 'answer'); ctext.focus(); ctext.setSelectionRange(answer.end, answer.end); });
    await page.keyboard.press('Backspace');
    state = await page.evaluate(() => ({ rows: ctext.wsc.ordered(), boardA: document.querySelector('[data-opt="a"]').getAttribute('aria-pressed'), boardB: document.querySelector('[data-opt="b"]').getAttribute('aria-pressed') }));
    assert.equal(state.rows.filter(r => r.kind === 'answer').length, 0);
    assert.equal(state.boardA, 'false'); assert.equal(state.boardB, 'false');
    await page.evaluate(() => {
      const at = ctext.value.indexOf('after');
      ctext.focus(); ctext.setSelectionRange(at, at);
      document.getElementById('other').focus();
      topicNote('topic-a').value = 'note text'; syncInlineTopics();
    });
    state = await page.evaluate(() => ({ text: ctext.value, note: ctext.wsc.ordered().find(r => r.kind === 'topic-note'), focus: document.activeElement.id }));
    assert(state.text.includes('[note:topic-a] after'));
    assert.equal(state.note.payload.note, 'note text');
    assert.equal(state.focus, 'other', 'board note editing does not steal focus');
    await page.evaluate(() => { ctext.focus(); ctext.setSelectionRange(stateNote().end, stateNote().end); function stateNote() { return ctext.wsc.ordered().find(r => r.kind === 'topic-note'); } });
    await page.keyboard.press('Backspace');
    assert.equal(await page.evaluate(() => topicNote('topic-a').value), '', 'atomic deletion clears the board topic note');
    await page.evaluate(() => { const ticket = ctext.wsc.ordered().find(r => r.kind === 'ticket'); ctext.focus(); ctext.setSelectionRange(ticket.start, ticket.start); });
    await page.keyboard.press('Delete');
    assert.equal(await page.evaluate(() => ctext.wsc.ordered().some(r => r.kind === 'ticket')), false, 'forward Delete removes a ticket atomically');
    console.log('composer inline browser: saved caret, board answer/note update and deletion, panel/ticket, cut/paste metadata passed');
  } finally { await browser.close(); }
})().catch(err => { console.error(err); process.exitCode = 1; });
