'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const tick = () => new Promise(setImmediate);

test('start button ignores repeat clicks and restores a failure reason', async () => {
  const elements = Object.fromEntries(['start-daemon', 'start-status', 'reason', 'log'].map(id => [id, {
    disabled: true, textContent: '', addEventListener(_name, fn) { this.click = fn; },
  }]));
  let calls = 0, reject;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../connection.js'), 'utf8'), {
    URLSearchParams, location: { search: '' }, navigator: { language: 'ko-KR' },
    document: { getElementById: id => elements[id] },
    window: { werkschaftDesktop: { daemon: {
      availability: async () => ({ available: true }),
      start: () => { calls++; return new Promise((_resolve, fail) => { reject = fail; }); },
    } } },
  });
  await tick();
  const button = elements['start-daemon'];
  assert.equal(button.textContent, '데몬 시작');
  const pending = button.click();
  button.click();
  assert.equal(calls, 1);
  assert.equal(button.disabled, true);
  assert.equal(button.textContent, '시작하는 중…');
  reject(new Error('timeout'));
  await pending;
  assert.equal(button.disabled, false);
  assert.equal(elements['start-status'].textContent, 'timeout');
});
