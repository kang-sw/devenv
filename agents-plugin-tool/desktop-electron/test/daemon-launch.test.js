'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const { EventEmitter } = require('node:events');

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-launch-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const entry = { exports: {} }, child = new EventEmitter();
  let call, unrefs = 0;
  child.unref = () => unrefs++;
  const file = path.resolve(__dirname, '../daemon-controller.js');
  vm.runInNewContext(fs.readFileSync(file, 'utf8'), {
    module: entry, Buffer, process, console, setTimeout,
    require: name => name === 'node:child_process' ? { spawn: (binary, args, options) => {
      call = { binary, args, options }; return child;
    } } : require(name.startsWith('./') ? path.resolve(path.dirname(file), name) : name),
  });
  return { launch: entry.exports.launch, child, root, call: () => call, unrefs: () => unrefs };
}
test('Windows native starter spawns directly, hidden and detached with file-backed logs; waits for successful exit', async t => {
  const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
  const args = ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s'];
  let done = false;
  const result = f.launch('C:\\stage\\ws-mcp.exe', { cwd: f.root, env: { FIXTURE: '1' }, logPath }, args, true).then(() => { done = true; });
  const call = f.call();
  assert.equal(call.binary, 'C:\\stage\\ws-mcp.exe');
  assert.deepEqual(call.args, args);
  assert.equal(call.options.cwd, f.root);
  assert.equal(call.options.detached, true);
  assert.equal(call.options.windowsHide, true);
  assert.equal(call.options.shell, undefined, 'no cmd.exe or Python launcher');
  assert.equal(call.options.stdio[0], 'ignore');
  assert.equal(typeof call.options.stdio[1], 'number');
  assert.equal(call.options.stdio[1], call.options.stdio[2]);
  assert.equal(call.options.logPath, undefined, 'do not pass our option to child_process');
  f.child.emit('spawn');
  await Promise.resolve();
  assert.equal(done, false, 'spawn is not readiness');
  assert.equal(f.unrefs(), 1);
  f.child.emit('exit', 0);
  await result;
});
test('starter exit shows only this attempt\'s bounded native failure; spawn errors propagate', async t => {
  const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
  fs.writeFileSync(logPath, 'old attempt must not appear\n');
  const failed = f.launch('not-executed.exe', { logPath }, [], true);
  fs.appendFileSync(logPath, 'daemon-start: independent lifetime refused by Windows');
  f.child.emit('spawn');
  f.child.emit('exit', 1);
  await assert.rejects(failed, error => {
    assert.match(error.message, /independent lifetime refused by Windows/);
    assert(!error.message.includes('old attempt'));
    return true;
  });
  const refused = f.launch('not-executed.exe', { logPath }, [], true);
  f.child.emit('error', new Error('spawn EACCES'));
  await assert.rejects(refused, /spawn EACCES/);
});
