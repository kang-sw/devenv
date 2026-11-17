'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const u = require('../update');

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-update-windows-'));
  const install = path.join(root, 'installed'), userData = path.join(root, 'userData');
  const stage = path.join(userData, 'update', 'extract', 'new');
  fs.mkdirSync(install);
  fs.mkdirSync(stage, { recursive: true });
  fs.writeFileSync(path.join(install, 'marker'), 'old');
  fs.writeFileSync(path.join(stage, 'marker'), 'new');
  fs.writeFileSync(path.join(userData, 'update', 'staged.json'), JSON.stringify({ version: '0.51.0', app: stage }));
  const sent = [], detached = [];
  const make = (options = {}) => u.createUpdater({ version: '0.50.0', packaged: true, platform: 'win32',
    execPath: path.join(install, 'Werkschaft.exe'), userData, log: () => {}, send: s => sent.push(s),
    feed: 'http://127.0.0.1:1', isApplying: () => false,
    detach: (...args) => { detached.push(args); return { pid: 42, created: '12345' }; }, ...options });
  return { root, userData, install, stage, sent, detached, make,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }) };
}

test('Windows bootstrap is piped, bounded and hidden, with literal PowerShell and Windows argument quoting', () => {
  const f = fixture();
  try {
    const command = 'C:\\Windows\\powershell.exe';
    const args = ['-File', "C:\\O'Brien & friends\\apply.ps1", '-Target', 'C:\\app x\\'];
    let called = false;
    u.spawnDetached(command, args, path.join(f.userData, 'update-apply.log'), 'win32', (exe, argv, options) => {
      called = true;
      assert.equal(exe, command);
      assert.equal(options.detached, undefined);
      assert.equal(options.stdio, undefined, 'spawnSync defaults to pipes, not invalid detached handles');
      assert.equal(options.cwd, f.userData);
      assert.equal(options.windowsHide, true);
      assert.equal(options.timeout, 15000);
      const script = Buffer.from(argv.at(-1), 'base64').toString('utf16le');
      assert.match(script, /Start-Process .* -WindowStyle Hidden -WorkingDirectory/);
      assert.ok(script.includes(u.windowsCommandLine(args).replace(/'/g, "''")));
      return { status: 0, stdout: '{"pid":42,"created":"12345"}' };
    });
    assert.equal(called, true);
    assert.match(fs.readFileSync(path.join(f.userData, 'update-apply.log'), 'utf8'), /Start-Process/);
    assert.throws(() => u.spawnDetached(command, [], path.join(f.userData, 'update-apply.log'), 'win32',
      () => ({ status: 1, stderr: 'Access denied' })), /Access denied/);
  } finally { f.cleanup(); }
});

test('Windows failures survive automatic checks and launches until a manual dashboard action acknowledges them', async () => {
  for (const outcome of ['pending', 'error', 'launch-error']) {
    const f = fixture();
    try {
      const options = outcome === 'launch-error' ? { detach: () => { throw new Error('bootstrap refused'); } } : {};
      assert.equal(f.make(options).applyOnQuit(true), outcome !== 'launch-error');
      if (outcome === 'launch-error') {
        assert.match(fs.readFileSync(path.join(f.userData, 'update-apply.log'), 'utf8'), /bootstrap refused/);
        assert.equal(f.sent.at(-1).state, 'apply-error');
      }
      if (outcome === 'error') {
        fs.writeFileSync(path.join(f.userData, 'update-apply-result.json'), '\uFEFF' + JSON.stringify({
          status: 'error', message: 'could not move the old app aside: Access denied',
        }));
      }
      f.sent.length = 0;
      assert.equal(f.make().applyOnLaunch(), false);
      assert.equal(f.sent.at(-1).state, 'apply-error');
      assert.equal(f.sent.at(-1).downloadPage, u.DOWNLOAD_PAGE);
      assert.match(f.sent.at(-1).message, outcome === 'error' ? /Access denied/
        : outcome === 'launch-error' ? /bootstrap refused/ : /did not complete/);
      assert.equal(fs.existsSync(f.stage), false, 'no failed-update relaunch loop');
      f.sent.length = 0;
      // main.js keeps only the latest pending status until subscription.
      let pending;
      const next = f.make({ send: status => { pending = status; } });
      assert.equal(next.applyOnLaunch(), false);
      assert.equal(await next.check(false), 'apply-error');
      assert.equal(pending.state, 'apply-error', 'automatic check cannot replace the failure pending for a late subscriber');
      assert.match(pending.message, /failed|did not complete|Access denied/);
      await next.check(true); // An explicit connected-dashboard action acknowledges delivery.
      assert.equal(JSON.parse(fs.readFileSync(path.join(f.userData, 'update-state.json'))).applyFailure, null);
      assert.equal(f.make().applyOnLaunch(), false);
      assert.deepEqual(f.sent, []);
    } finally { f.cleanup(); }
  }
});

test('successful Windows apply is consumed without an error; a leftover stage can still apply on launch', () => {
  const f = fixture();
  try {
    assert.equal(f.make().applyOnLaunch(), true);
    const args = f.detached.at(-1)[1];
    assert.equal(args[args.indexOf('-Relaunch') + 1], '1');
    fs.writeFileSync(path.join(f.userData, 'update-apply-result.json'), '\uFEFF' + JSON.stringify({ status: 'success' }));
    assert.equal(f.make({ version: '0.51.0' }).applyOnLaunch(), false);
    assert.deepEqual(f.sent, []);
    assert.equal(JSON.parse(fs.readFileSync(path.join(f.userData, 'update-state.json'))).applyAttempt, null);
  } finally { f.cleanup(); }
});

test('Windows unavailable location reports the reason and download-page fallback without starting a helper', () => {
  const f = fixture();
  try {
    fs.rmSync(f.install, { recursive: true });
    assert.equal(f.make().applyOnLaunch(), false);
    assert.equal(f.sent.at(-1).state, 'apply-error');
    assert.match(f.sent.at(-1).message, /not writable/);
    assert.equal(f.sent.at(-1).downloadPage, u.DOWNLOAD_PAGE);
    assert.equal(f.detached.length, 0);
    assert.equal(fs.existsSync(f.stage), true);
    if (process.platform === 'win32') {
      assert.match(u.applyProblem('win32', 'C:\\Apps\\W', 'D:\\stage'), /different drive/);
    }
  } finally { f.cleanup(); }
});

test('a rapid reopen yields to the identified helper with independent continuation ownership', () => {
  const f = fixture();
  try {
    assert.equal(f.make({ now: () => 1000 }).applyOnQuit(false), true);
    let queried = false;
    const next = f.make({ now: () => 999999, argv: ['--lang=ko'], isApplying: helper => {
      queried = true;
      assert.deepEqual(helper, { pid: 42, created: '12345' });
      return true; // A known live helper is never abandoned merely due to age.
    } });
    assert.equal(next.applyOnLaunch(), true, 'main exits before opening/locking the old app');
    assert.equal(queried, true);
    assert.equal(fs.existsSync(f.stage), true);
    assert.equal(f.detached.length, 2, 'a separate continuation owns the reopen, not a second swap');
    const args = f.detached.at(-1)[1];
    assert.equal(args[args.indexOf('-OwnerPid') + 1], '42');
    assert.equal(args[args.indexOf('-OwnerCreated') + 1], '12345');
    assert.equal(args.includes('-Staged'), false, 'continuation never swaps or removes the stage');
    assert.equal(Buffer.from(args[args.indexOf('-RelaunchArgs') + 1], 'base64').toString('utf8'), '--lang=ko');
    fs.writeFileSync(path.join(f.userData, 'update-apply-result.json'), JSON.stringify({ status: 'success' }));
    assert.equal(f.make({ version: '0.51.0' }).applyOnLaunch(), false);
    assert.deepEqual(f.sent, []);
  } finally { f.cleanup(); }
});

test('completion during liveness observation never fabricates failure or loses the explicit reopen', () => {
  for (const running of [true, false]) {
    for (const version of ['0.50.0', '0.51.0']) {
      const f = fixture();
      try {
        f.make().applyOnQuit(false);
        const next = f.make({ version, isApplying: () => {
          // The original helper passes its final relaunch decision and exits
          // while the separate process inventory query is in flight.
          fs.writeFileSync(path.join(f.userData, 'update-apply-result.json'), JSON.stringify({ status: 'success' }));
          return running;
        } });
        assert.equal(next.applyOnLaunch(), version === '0.50.0');
        assert.deepEqual(f.sent, [], 'successful completion must not create an apply-error');
        if (version === '0.50.0') {
          assert.equal(f.detached.length, 2, 'an independent continuation, not an unacknowledged request, owns relaunch');
          assert.equal(f.detached.at(-1)[1].includes('-OwnerPid'), true);
        } else {
          assert.equal(f.detached.length, 1, 'the new version may open its window directly');
        }
      } finally { f.cleanup(); }
    }
  }
});

test('unknown helper ownership gets a bounded grace period before an abandoned attempt is discarded', () => {
  const f = fixture();
  try {
    f.make({ now: () => 1000 }).applyOnQuit(false);
    assert.equal(f.make({ now: () => 2000, isApplying: () => null }).applyOnLaunch(), true);
    assert.equal(fs.existsSync(f.stage), true);
    assert.equal(f.make({ now: () => 182000, isApplying: () => null }).applyOnLaunch(), false);
    assert.equal(fs.existsSync(f.stage), false);
    assert.equal(f.sent.at(-1).state, 'apply-error');
  } finally { f.cleanup(); }
});

test('helper liveness validates PID and creation identity without signalling any process', () => {
  const helper = { pid: 42, created: '12345' };
  let called = false;
  assert.equal(u.windowsHelperRunning(helper, (command, args, options) => {
    called = true;
    assert.match(command, /powershell.exe$/);
    assert.ok(args.at(-1).includes('Get-Process -Id 42'));
    assert.ok(args.at(-1).includes("Ticks -eq '12345'"));
    assert.equal(options.timeout, 5000);
    return { status: 0 };
  }), true);
  assert.equal(called, true);
  assert.equal(u.windowsHelperRunning(helper, () => ({ status: 1 })), false);
  assert.equal(u.windowsHelperRunning(helper, () => ({ error: new Error('timeout') })), null);
  assert.equal(u.windowsHelperRunning({ pid: 42, created: "';throw 'bad" }, () => assert.fail('must not launch')), null);
});

test('Windows write probe catches effective ACL denial in an existing install and cleans owned probes', () => {
  const f = fixture();
  try {
    assert.equal(u.applyProblem('win32', f.install, f.stage), null);
    assert.deepEqual(fs.readdirSync(f.install), ['marker']);
    assert.ok(!fs.readdirSync(f.root).some(name => name.startsWith('.werkschaft-update-probe-')));
    for (const operation of ['mkdtempSync', 'renameSync']) {
      const denied = { ...fs, [operation]: () => { const error = new Error('EACCES: ACL denies write'); error.code = 'EACCES'; throw error; } };
      assert.match(u.applyProblem('win32', f.install, f.stage, denied), /not writable: EACCES: ACL denies write/);
      assert.deepEqual(fs.readdirSync(f.install), ['marker'], 'no probe remains after a denied operation');
    }
  } finally { f.cleanup(); }
});

// Explicit opt-in: never launch an app, a daemon, or a real exe. The helper
// operates on marker files only, and this host's approved dummy root only.
const helperEnabled = process.platform === 'win32' && process.env.WERKSCHAFT_TEST_UPDATE_HELPER === '1'
  && path.resolve(os.tmpdir()).toLowerCase() === path.resolve('D:/Temp/electron-win-selfupdate').toLowerCase();
test('Windows reopen continuation waits for its Node caller before a marker-only simulated relaunch',
  { skip: !helperEnabled }, async () => {
    const f = fixture();
    try {
      const script = path.join(f.userData, 'reopen.ps1');
      const logFile = path.join(f.userData, 'update-apply.log');
      // Replace only the final app launch with a log marker. No exe is run.
      fs.writeFileSync(script, u.helperScript('win32', true)
        .replace('Start-Process -FilePath $exe -ArgumentList $argLine', 'Add-Content -LiteralPath $Log -Value "fake launch $argLine"')
        .replace('Start-Process -FilePath $exe', 'Add-Content -LiteralPath $Log -Value "fake launch without args"'));
      const opts = { target: f.install, staging: path.join(f.userData, 'update'), reopen: true,
        helper: { pid: 2147483647, created: '0' }, logFile,
        resultFile: path.join(f.userData, 'result.json'), args: ['--lang=ko', '--user-data-dir=C:\\x y'] };
      const parentScript = `const u=require(${JSON.stringify(require.resolve('../update'))});
        const [script,opts]=JSON.parse(process.argv[1]);
        const [cmd,args]=u.helperCommand('win32',script,{...opts,pid:process.pid});
        const helper=u.spawnDetached(cmd,args,opts.logFile);
        if(!u.windowsHelperRunning(helper)) throw new Error('continuation did not wait for its caller');
        console.log(JSON.stringify(helper));`;
      const parent = spawnSync(process.execPath, ['-e', parentScript, JSON.stringify([script, opts])], { encoding: 'utf8', timeout: 20000 });
      assert.equal(parent.status, 0, parent.stderr);
      const helper = JSON.parse(parent.stdout.trim());
      let running = true;
      for (let i = 0; i < 20 && running !== false; i++) {
        running = u.windowsHelperRunning(helper);
        if (running !== false) await new Promise(resolve => setTimeout(resolve, 100));
      }
      assert.equal(running, false, 'continuation exited before cleanup');
      const log = fs.readFileSync(logFile, 'utf8');
      assert.match(log, /reopen helper started/);
      assert.ok(log.includes('fake launch ' + u.windowsCommandLine(opts.args)), log);
      assert.match(log, /reopened /);
      assert.equal(fs.existsSync(f.stage), true, 'continuation never touches the stage');
    } finally { f.cleanup(); }
  });

test('Windows helper outlives its Node parent, swaps dummy folders, and records rollback failure detail',
  { skip: !helperEnabled }, async () => {
    const f = fixture();
    try {
      const resultFile = path.join(f.userData, 'update-apply-result.json');
      const logFile = path.join(f.userData, 'update-apply.log');
      const script = path.join(f.userData, 'apply.ps1');
      fs.writeFileSync(script, u.helperScript('win32'));
      const opts = { target: f.install, staged: f.stage, backup: f.install + '-update-backup',
        staging: path.join(f.userData, 'update'), relaunch: false, logFile, resultFile };
      const parentScript = `const u=require(${JSON.stringify(require.resolve('../update'))});
        const [script, opts]=JSON.parse(process.argv[1]);
        const [command,args]=u.helperCommand('win32',script,{...opts,pid:process.pid});
        const helper=u.spawnDetached(command,args,opts.logFile);
        if(!u.windowsHelperRunning(helper)) throw new Error('dummy helper identity did not remain live');`;
      const parent = spawnSync(process.execPath, ['-e', parentScript, JSON.stringify([script, opts])], { encoding: 'utf8', timeout: 20000 });
      assert.equal(parent.status, 0, parent.stderr);
      const wait = async () => {
        for (let i = 0; i < 100; i++) {
          try { return JSON.parse(fs.readFileSync(resultFile, 'utf8').replace(/^\uFEFF/, '')); } catch {}
          await new Promise(resolve => setTimeout(resolve, 100));
        }
        assert.fail('helper did not finish; ' + fs.readFileSync(logFile, 'utf8'));
      };
      const success = await wait();
      assert.equal(success.status, 'success', success.message);
      assert.equal(fs.readFileSync(path.join(f.install, 'marker'), 'utf8'), 'new');
      assert.match(fs.readFileSync(logFile, 'utf8'), /helper started.*\r?\nupdated /);
      assert.equal(fs.existsSync(opts.backup), false);
      fs.rmSync(resultFile);
      // Missing stage: moving the old app aside succeeds, installation fails,
      // and rollback must restore the old folder without touching other paths.
      const [command, args] = u.helperCommand('win32', script, { ...opts, pid: 2147483647 });
      const helper = u.spawnDetached(command, args, logFile);
      const failed = await wait();
      assert.equal(failed.status, 'error');
      assert.match(failed.message, /restored the previous app/);
      assert.equal(fs.readFileSync(path.join(f.install, 'marker'), 'utf8'), 'new');
      console.log('dummy helper:', success.message, ';', failed.message);
      // Completion is intentionally durable before process exit; its cwd can
      // still lock the dummy root briefly. Reconcile the exact owned identity
      // before cleanup, rather than deleting a live helper's working folder.
      let running = true;
      for (let i = 0; i < 20 && running !== false; i++) {
        running = u.windowsHelperRunning(helper);
        if (running !== false) await new Promise(resolve => setTimeout(resolve, 100));
      }
      assert.equal(running, false, 'dummy helper exited before cleanup');
    } finally { f.cleanup(); }
  });
