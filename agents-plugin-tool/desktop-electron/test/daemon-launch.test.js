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
  const entry = { exports: {} }, calls = [];
  let child, unrefs = 0;
  const file = path.resolve(__dirname, '../daemon-controller.js');
  vm.runInNewContext(fs.readFileSync(file, 'utf8'), {
    module: entry, Buffer, process, console, setTimeout,
    require: name => name === 'node:child_process' ? { spawn: (binary, args, options) => {
      calls.push({ binary, args, options });
      child = new EventEmitter();
      child.unref = () => unrefs++;
      return child;
    } } : require(name.startsWith('./') ? path.resolve(path.dirname(file), name) : name),
  });
  return { launch: entry.exports.launch, get child() { return child; }, root, calls,
    call: () => calls.at(-1), unrefs: () => unrefs };
}
test('long-running dashboard and replacement preflight do not inherit the log', async t => {
  const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
  for (const args of [['dashboard'], ['dashboard', '--port', '1234', '--lock-wait', '8s']]) {
    const result = f.launch('not-executed', { logPath }, args);
    assert.equal(f.call().options.stdio, 'ignore');
    assert.equal(f.call().options.logPath, undefined);
    assert.equal(fs.existsSync(logPath), false);
    f.child.emit('spawn');
    await result;
  }
});
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
test('numeric breakaway access-denied retries once with job lifetime, preserving Ensure arguments and hidden detached options', async t => {
  for (const replace of [false, true]) {
    const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
    const args = ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s', ...(replace ? ['--replace'] : [])];
    let done = false;
    const result = f.launch('not-executed.exe', { logPath, cwd: f.root, env: { FIXTURE: 'same', WS_MCP_DAEMON_JOB_LIFETIME: '0' } }, args, true)
      .then(() => { done = true; });
    fs.appendFileSync(logPath, 'daemon spawn mode=breakaway\r\nWS_DAEMON_BREAKAWAY_DENIED=5\r\n');
    f.child.emit('spawn');
    f.child.emit('exit', 1);
    assert.equal(f.calls.length, 2);
    const retry = f.call();
    assert.equal(retry.binary, 'not-executed.exe');
    assert.deepEqual(retry.args, args, 'do not drop prestart or explicit replacement guards');
    assert.equal(retry.options.env.WS_MCP_DAEMON_JOB_LIFETIME, '1');
    assert.equal(retry.options.env.FIXTURE, 'same');
    assert.equal(retry.options.cwd, f.root);
    assert.equal(retry.options.detached, true);
    assert.equal(retry.options.windowsHide, true);
    assert.equal(retry.options.shell, undefined);
    fs.appendFileSync(logPath, 'daemon spawn mode=job-lifetime\n');
    f.child.emit('spawn');
    await Promise.resolve();
    assert.equal(done, false, 'retry must still wait for readiness');
    f.child.emit('exit', 0);
    await result;
    assert.match(fs.readFileSync(logPath, 'utf8'), /mode=breakaway[\s\S]*mode=job-lifetime/);
  }
});
test('arbitrary spawn failures, stale markers and a failed job-lifetime retry never trigger another spawn', async t => {
  for (const detail of ['Access is denied.', 'WS_DAEMON_BREAKAWAY_DENIED=2', 'prefix WS_DAEMON_BREAKAWAY_DENIED=5']) {
    const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
    fs.writeFileSync(logPath, 'WS_DAEMON_BREAKAWAY_DENIED=5\n'); // Previous attempt is not this attempt.
    const result = f.launch('not-executed.exe', { logPath }, ['daemon-start', '--require-independent-lifetime'], true);
    fs.appendFileSync(logPath, detail);
    f.child.emit('exit', 1);
    await assert.rejects(result, /status 1/);
    assert.equal(f.calls.length, 1);
  }
  const f = fixture(t), logPath = path.join(f.root, 'daemon-start.log');
  const result = f.launch('not-executed.exe', { logPath }, ['daemon-start', '--require-independent-lifetime'], true);
  fs.appendFileSync(logPath, 'WS_DAEMON_BREAKAWAY_DENIED=5\n');
  f.child.emit('exit', 1);
  fs.appendFileSync(logPath, 'WS_DAEMON_BREAKAWAY_DENIED=5\nactual fallback failed\n');
  f.child.emit('exit', 1);
  await assert.rejects(result, /actual fallback failed/);
  assert.equal(f.calls.length, 2, 'no retry loop');
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
