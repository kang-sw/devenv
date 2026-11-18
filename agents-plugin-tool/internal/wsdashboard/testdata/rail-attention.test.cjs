// Harness-free: node internal/wsdashboard/testdata/rail-attention.test.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../assets/rail.js'), 'utf8'), { window });

function row(id, asks) {
  const attrs = { 'data-rid': id };
  if (asks !== undefined) attrs['data-asks'] = asks;
  const classes = new Set(['rs']);
  const link = {
    setAttribute: (k, v) => { link[k] = v; },
    removeAttribute: k => { delete link[k]; }
  };
  return {
    getAttribute: k => attrs[k] ?? null,
    setAttribute: (k, v) => { attrs[k] = v; },
    querySelector: () => link,
    classList: {
      contains: k => classes.has(k),
      add: k => classes.add(k),
      remove: k => classes.delete(k),
      toggle: (k, on) => on ? classes.add(k) : classes.delete(k)
    }
  };
}
const state = window.WsRail.attention();
let a = row('away', '12'), current = row('shown', ''), old = row('old');
const sync = () => state.sync([a, current, old], 'shown', 'New question');
const flagged = r => r.classList.contains('attn');
const flashing = r => r.classList.contains('attn-flash');
sync();
assert.equal(flagged(a), false, 'first observation silently baselines');
a.setAttribute('data-asks', '12 15');
sync();
assert.equal(flagged(a), true, 'new ID away flags');
assert.equal(flashing(a), true, 'new flag flashes');
assert.equal(a.querySelector().title, 'New question', 'flag has accessible hint');
sync();
assert.equal(flashing(a), true, 'same node keeps its running animation');
a = row('away', '12 15'); // Server-rendered row replacement.
sync();
assert.equal(flagged(a), true, 'row patch keeps flag');
assert.equal(flashing(a), false, 'row patch never replays flash');
state.sync([a, current, old], 'away', 'New question');
assert.equal(flagged(a), false, 'entering session clears flag');
assert.equal(a.querySelector().title, undefined, 'entering removes hint');
a.setAttribute('data-asks', '12 40');
sync();
assert.equal(flagged(a), true, 'equal-count replacement is new');
state.sync([a, current, old], 'away', 'New question');
a.setAttribute('data-asks', '12');
sync();
assert.equal(flagged(a), false, 'closing does not flag');
a.setAttribute('data-asks', '12 40');
sync();
assert.equal(flagged(a), true, 'settled ID reopening is new');
state.sync([a, current, old], 'away', 'New question');
sync();
assert.equal(flagged(a), false, 're-asking an already-open ID is not new');
current.setAttribute('data-asks', '100');
sync();
assert.equal(flagged(current), false, 'current session never flags');
assert.equal(flagged(old), false, 'old server without attribute does nothing');
const added = row('new-row', '1 2');
state.sync([a, current, old, added], 'shown', 'New question');
assert.equal(flagged(added), false, 'new rows silently baseline');
a.setAttribute('data-asks', '12 40 99');
state.observe([a, current, old], 'shown', true); // First full rail after reconnect.
sync();
assert.equal(flagged(a), false, 'reconnect silently baselines offline arrivals');
a.setAttribute('data-asks', '12 40 99 101');
sync();
state.observe([a, current, old], 'shown', true);
a = row('away', '12 40 99 101');
sync();
assert.equal(flagged(a), true, 'reconnect retains an existing flag');
assert.equal(flashing(a), false, 'reconnect replacement is settled');
const reset = window.WsRail.attention();
reset.sync([a], 'shown', 'New question');
assert.equal(flagged(a), false, 'reload resets memory-only state');
const held = window.WsRail.attention();
let heldRow = row('held', '');
held.sync([heldRow], 'shown', 'New question');
held.observe([row('held', '7')], 'shown');
held.observe([row('held', '')], 'shown');
heldRow = row('held', '');
held.mark([heldRow], 'shown', 'New question');
assert.equal(flagged(heldRow), true, 'hidden/gated open then close still flags');
assert.equal(flashing(heldRow), true, 'held flag flashes when first mounted');
heldRow = row('held', '');
held.mark([heldRow], 'shown', 'New question');
assert.equal(flashing(heldRow), false, 'later patches do not replay held flash');
held.observe([row('held', '8')], 'shown');
held.mark([heldRow], 'held', 'New question');
assert.equal(flagged(heldRow), false, 'successful show clears a held flag');
console.log('PASS: rail attention baseline, ID deltas, current/show, patch, reconnect, old server, held updates');
