const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// Exercise the real badge functions without booting the rest of the page.
const source = fs.readFileSync(path.join(__dirname, '../assets/dashboard.js'), 'utf8');
const functions = source.slice(source.indexOf('  function taskBadge('), source.indexOf('  function interruptTurn('));
function badge() {
  const label = { textContent: '' };
  const attrs = {};
  return { hidden: true, title: '', label, querySelector: () => label,
    getAttribute: (name) => attrs[name], setAttribute: (name, value) => attrs[name] = value };
}

test('background and monitor pills are separate and zero hides each', () => {
  const context = { bgbadge: badge(), monbadge: badge(), bgTitle: '', monTitle: '', liveMode: true,
    wsTn: (word, n) => `${n} ${word === 'js.bgtasks' ? 'background' : n === 1 ? 'monitor' : 'monitors'}` };
  vm.createContext(context);
  vm.runInContext(functions, context);
  const counts = (bg, mon) => ({ getAttribute: (name) => name === 'data-bg' ? String(bg) : String(mon) });
  context.bgBadge(counts(2, 1));
  assert.equal(context.bgbadge.hidden, false);
  assert.equal(context.bgbadge.label.textContent, '2 background');
  assert.equal(context.monbadge.hidden, false);
  assert.equal(context.monbadge.label.textContent, '1 monitor');
  context.bgBadge(counts(0, 3));
  assert.equal(context.bgbadge.hidden, true);
  assert.equal(context.monbadge.label.textContent, '3 monitors');
  context.bgBadge(null);
  assert.equal(context.monbadge.hidden, true);
  context.liveMode = false;
  context.bgBadge(counts(1, 1));
  assert.equal(context.bgbadge.hidden, true);
  assert.equal(context.monbadge.hidden, true);
});
