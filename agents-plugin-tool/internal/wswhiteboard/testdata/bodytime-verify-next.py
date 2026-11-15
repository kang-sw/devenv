#!/usr/bin/env python3
"""Prepared TASK4 batch, not execution authority. Requires a fresh MAIN START.

Scope: full whiteboard/i18n tests, six named dashboard integration tests, and
production-renderer offline fixtures with 96 hover cases/120 captures. This is
not live acceptance or a complete dashboard suite. Holder/terminal, tracker,
filesystem, git, stream, selection-note and dashboard layout/morph/lazy-frame
suites are omitted. No install, shared daemon, launcher, merge or release.

Ownership audit: ordinary Go/compiler/test/Node/Python helpers inherit their
new-session command group. Whiteboard folders_test.go starts ordinary exec.Cmd
children; selected dashboard nodeScript (toast_test.go) does likewise. The
real detached-holder test (holders_live_test.go) is NOT selected. Playwright's
launchServer creates a separately owned browser group: the CJS probe registers
it in browser-owner.json and the receipt journal. The sampler also records
observed descendants. Receipts cover these audited groups/observed identities,
not every process on the machine or an unobserved arbitrary breakaway process.
"""
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import traceback

ROOT = Path(__file__).resolve().parents[4]
OUT = Path('/tmp/ws-bodytime-677-next-undecided-spruce-pointing')
MODULE = '/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright'
W = 'agents-plugin-tool/internal/wswhiteboard/'
SELECTOR = ('^(TestCSSClassesDoNotCollideAcrossModules|'
            'TestDevAssetsShowEditsWithoutARestart|'
            'TestFrameAssetsAreServedBesideThePages|'
            'TestBoardEntryDocFrameKeyEnvelope|'
            'TestSessionPageRendersLiveRegions|'
            'TestTopicFramesCarryThePinnedTheme)$')
BASE = '54943a1831de1a89c5cb06f08088d354da60c896'


class Block(RuntimeError):
    pass


class Tee:
    def __init__(self, original, file):
        self.original, self.file = original, file

    def write(self, text):
        self.original.write(text)
        self.file.write(text)
        self.file.flush()
        return len(text)

    def flush(self):
        self.original.flush()
        self.file.flush()


class Receipts:
    def __init__(self, path):
        self.path = path
        self.blocks = []
        self.child_blocks = set()

    def emit(self, event, **data):
        row = dict(event=event, wall=time.time(), monotonic=time.monotonic(), **data)
        with self.path.open('a') as output:
            output.write(json.dumps(row, ensure_ascii=False) + '\n')
            output.flush()
            os.fsync(output.fileno())

    def block(self, reason):
        self.blocks.append(reason)
        self.emit('cleanup_block', reason=reason)
        print('CLEANUP BLOCK:', reason, flush=True)

    def absorb_child_blocks(self):
        try:
            for line in self.path.read_text().splitlines():
                row = json.loads(line)
                if row.get('source') == 'offline-cjs' and row.get('event') == 'cleanup_block':
                    key = (row.get('wall'), row.get('reason'))
                    if key not in self.child_blocks:
                        self.child_blocks.add(key)
                        self.blocks.append('offline CJS: ' + str(row.get('reason')))
        except Exception as err:
            self.block('receipt journal could not be fully read: %r' % err)


def raw_signal_child(proc, sig, receipts):
    # A live, unreaped Popen child cannot have its PID reused. Never infer
    # ownership from an executable name, port, or a stale numeric PID.
    if proc.poll() is not None:
        return
    try:
        if os.getpgid(proc.pid) != proc.pid:
            raise Block('direct child changed its new-session group')
        os.killpg(proc.pid, sig)
        receipts.emit('signal', pid=proc.pid, pgid=proc.pid,
                      identity_basis='live unreaped direct Popen child',
                      signal=int(sig), result='sent')
    except ProcessLookupError:
        receipts.emit('signal', pid=proc.pid, signal=int(sig), result='already gone')
    except Exception as err:
        receipts.block('cannot signal direct child %s: %r' % (proc.pid, err))


def snapshot(deadline, receipts):
    """Bounded read-only metadata inventory; persist only owned/diagnostic rows."""
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise Block('metadata observation deadline exhausted')
    proc = subprocess.Popen(['/bin/ps', '-axo', 'pid=,ppid=,pgid=,lstart='],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, start_new_session=True)
    try:
        receipts.emit('diagnostic_launch', pid=proc.pid, pgid=proc.pid,
                      argv=['/bin/ps', '-axo', 'pid=,ppid=,pgid=,lstart='])
        stdout, stderr = proc.communicate(timeout=min(1, remaining))
        if proc.returncode:
            raise Block('process metadata unavailable: ' + stderr.strip())
        rows = {}
        for line in stdout.splitlines():
            fields = line.split()
            if len(fields) != 8:
                raise Block('unexpected process metadata row shape')
            pid, ppid, pgid = map(int, fields[:3])
            rows[pid] = dict(pid=pid, ppid=ppid, pgid=pgid,
                             start=' '.join(fields[3:]))
        own = rows.get(proc.pid)
        if own is None or own['pgid'] != proc.pid:
            raise Block('diagnostic start identity was not captured')
        receipts.emit('diagnostic_reap', **own, exit=proc.returncode,
                      absence_basis='direct child communicate reaped; ps has no children')
        return rows
    finally:
        if proc.poll() is None:
            raw_signal_child(proc, signal.SIGKILL, receipts)
            try:
                proc.wait(timeout=max(0, min(1, deadline - time.monotonic())))
                receipts.emit('diagnostic_reap', pid=proc.pid, exit=proc.returncode)
            except Exception as err:
                receipts.block('diagnostic termination unconfirmed: %r' % err)


class Job:
    def __init__(self, number, argv, timeout, env, receipts, wall_deadline, browser=False):
        self.number, self.argv, self.timeout = number, argv, timeout
        self.env, self.receipts, self.wall_deadline = env, receipts, wall_deadline
        self.browser = browser
        self.proc = None
        self.identities = {}  # (PID, start) -> observed owned row.
        self.groups = set()
        self.owner_path = OUT / 'captures' / 'browser-owner.json'
        self.browser_registered = False
        self.root_registered = False
        self.failure = None

    def register(self, row, reason):
        key = (row['pid'], row['start'])
        if key not in self.identities:
            self.identities[key] = row
            self.groups.add(row['pgid'])
            self.receipts.emit('owned_identity', job=self.number, reason=reason, **row)

    def observe(self, deadline, final=False):
        rows = snapshot(deadline, self.receipts)
        # Only a matching observed identity anchors parent or process-group
        # inheritance. A reused PID is never accepted or signalled.
        anchors = {pid for (pid, start) in self.identities
                   if pid in rows and rows[pid]['start'] == start}
        changed = True
        while changed:
            changed = False
            anchored_groups = {rows[pid]['pgid'] for pid in anchors}
            for row in rows.values():
                if row['pid'] not in anchors and (row['ppid'] in anchors or
                                                  row['pgid'] in anchored_groups):
                    self.register(row, 'observed descendant/audited inherited group')
                    anchors.add(row['pid'])
                    changed = True
        if self.browser:
            try:
                record = json.loads(self.owner_path.read_text())
                pid, start = record['pid'], record['start']
                if not isinstance(pid, int) or pid <= 1 or not isinstance(start, str) or not start:
                    raise Block('invalid browser identity record')
                start = ' '.join(start.split())
                row = rows.get(pid)
                if row is not None and row['start'] == start:
                    if row['pgid'] != pid:
                        raise Block('browser is not its registered group leader')
                    self.register(row, 'exclusive CJS BrowserServer.process registration')
                # A record can be accepted after exit only if this exact browser
                # was previously observed. A file alone is not OS identity proof.
                if (pid, start) in self.identities:
                    self.browser_registered = True
                elif final:
                    self.receipts.block('browser exited without an observed matching start identity')
            except Exception as err:
                if final:
                    self.receipts.block('browser ownership/termination unknown: %r' % err)
        return rows

    def residue(self, rows):
        live = [row for (pid, start), row in self.identities.items()
                if pid in rows and rows[pid]['start'] == start]
        members = [row for row in rows.values() if row['pgid'] in self.groups]
        return live, members

    def signal_groups(self, sig, deadline):
        try:
            rows = self.observe(deadline)
        except Exception as err:
            self.receipts.block('cannot refresh identities before signal: %r' % err)
            if self.proc is not None:
                raw_signal_child(self.proc, sig, self.receipts)
            return
        for group in sorted(self.groups):
            members = [r for r in rows.values() if r['pgid'] == group]
            if not members:
                continue
            guards = [r for r in members if (r['pid'], r['start']) in self.identities]
            if not guards:
                self.receipts.block('group %s has no matching owned live identity; not signalled' % group)
                continue
            guard = guards[0]
            try:
                # Refresh the guard immediately before the group signal.
                fresh = snapshot(deadline, self.receipts).get(guard['pid'])
                if fresh != guard:
                    self.receipts.block('signal guard changed for group %s' % group)
                    continue
                os.killpg(group, sig)
                self.receipts.emit('signal', job=self.number, pgid=group,
                                   guard=guard, signal=int(sig), result='sent')
            except ProcessLookupError:
                self.receipts.emit('signal', job=self.number, pgid=group,
                                   signal=int(sig), result='already gone')
            except Exception as err:
                self.receipts.block('owned group signal failed: %r' % err)

    def finish(self, failed):
        deadline = min(self.wall_deadline, time.monotonic() + 12)
        if self.proc is None:
            self.receipts.emit('job_not_launched', job=self.number)
            return
        if not self.root_registered:
            self.receipts.block('root start identity missing; descendant absence is unknown')
            raw_signal_child(self.proc, signal.SIGTERM, self.receipts)
            try:
                self.proc.wait(timeout=max(0, min(3, deadline - time.monotonic())))
            except subprocess.TimeoutExpired:
                raw_signal_child(self.proc, signal.SIGKILL, self.receipts)
                try:
                    self.proc.wait(timeout=max(0, min(2, deadline - time.monotonic())))
                except Exception as err:
                    self.receipts.block('unregistered direct child reap unconfirmed: %r' % err)
            if self.proc.returncode is not None:
                self.receipts.emit('command_reap', job=self.number,
                                   pid=self.proc.pid, exit=self.proc.returncode)
            return  # No group/identity absence or quiescence inference.
        initial_blocks = len(self.receipts.blocks)
        try:
            # Permit normal child teardown briefly. Any remaining owned work
            # after a successful command is a BLOCK, not silent success.
            rows = self.observe(deadline)
            live, members = self.residue(rows)
            if live or members:
                if not failed:
                    time.sleep(min(.2, max(0, deadline - time.monotonic())))
                    rows = self.observe(deadline)
                    live, members = self.residue(rows)
                if live or members:
                    if not failed:
                        self.receipts.block('successful job left owned process/group residue')
                    self.signal_groups(signal.SIGTERM, deadline)
                    if self.proc is not None:
                        try:
                            self.proc.wait(timeout=max(0, min(3, deadline - time.monotonic())))
                        except subprocess.TimeoutExpired:
                            pass
                    self.signal_groups(signal.SIGKILL, deadline)
            if self.proc is not None:
                try:
                    self.proc.wait(timeout=max(0, min(2, deadline - time.monotonic())))
                    self.receipts.emit('command_reap', job=self.number,
                                       pid=self.proc.pid, exit=self.proc.returncode)
                except Exception as err:
                    self.receipts.block('command reap unconfirmed: %r' % err)
            # Poll only this owned cleanup boundary, never another session.
            while time.monotonic() < deadline:
                rows = self.observe(deadline, final=True)
                live, members = self.residue(rows)
                if not live and not members:
                    for (pid, start), row in self.identities.items():
                        self.receipts.emit('identity_absent', job=self.number,
                                           pid=pid, start=start,
                                           observed_replacement=rows.get(pid))
                    for group in sorted(self.groups):
                        self.receipts.emit('group_empty', job=self.number, pgid=group)
                    self.receipts.absorb_child_blocks()
                    if len(self.receipts.blocks) == initial_blocks and (not self.browser or self.browser_registered):
                        self.receipts.emit('job_quiescent', job=self.number,
                                           scope='audited groups and observed owned identities only')
                    return
                time.sleep(min(.1, max(0, deadline - time.monotonic())))
            self.receipts.block('owned process/group final absence unconfirmed at cleanup deadline')
        except BaseException as err:
            self.receipts.block('owned cleanup observation failed: %r' % err)
            if self.proc is not None:
                raw_signal_child(self.proc, signal.SIGKILL, self.receipts)
                try:
                    self.proc.wait(timeout=max(0, min(1, deadline - time.monotonic())))
                    self.receipts.emit('command_reap', job=self.number,
                                       pid=self.proc.pid, exit=self.proc.returncode)
                except Exception as wait_error:
                    self.receipts.block('fallback reap unconfirmed: %r' % wait_error)

    def run(self):
        log = OUT / ('command-%02d.log' % self.number)
        deadline = min(self.wall_deadline - 12, time.monotonic() + self.timeout)
        if deadline <= time.monotonic():
            raise Block('batch budget exhausted before command')
        print('RUN', self.number, self.argv, 'cap=', self.timeout, 'log=', log, flush=True)
        before_blocks = len(self.receipts.blocks)
        with log.open('w') as output:
            try:
                self.proc = subprocess.Popen(self.argv, cwd=ROOT, env=self.env,
                                             stdout=output, stderr=subprocess.STDOUT,
                                             start_new_session=True)
                # The guard and durable launch receipt precede all diagnostics.
                self.receipts.emit('command_launch', job=self.number, pid=self.proc.pid,
                                   pgid=self.proc.pid, start_status='pending observation',
                                   argv=self.argv, cap_seconds=self.timeout)
                rows = snapshot(deadline, self.receipts)
                root = rows.get(self.proc.pid)
                if root is None or root['pgid'] != self.proc.pid:
                    raise Block('launched command start identity was not established')
                self.register(root, 'new-session direct Popen child')
                self.root_registered = True
                while True:
                    if time.monotonic() >= deadline:
                        raise subprocess.TimeoutExpired(self.argv, self.timeout)
                    self.observe(deadline)
                    code = self.proc.poll()
                    if code is not None:
                        self.receipts.emit('command_exit', job=self.number,
                                           pid=self.proc.pid, exit=code)
                        if code:
                            raise subprocess.CalledProcessError(code, self.argv)
                        break
                    if time.monotonic() >= deadline:
                        raise subprocess.TimeoutExpired(self.argv, self.timeout)
                    time.sleep(min(.2, max(0, deadline - time.monotonic())))
            except BaseException as err:
                self.failure = err
                self.receipts.emit('first_command_failure', job=self.number,
                                   error=repr(err))
            finally:
                try:
                    self.finish(failed=self.failure is not None)
                except BaseException as cleanup_error:
                    # A cleanup exception must never replace the first command
                    # failure. The direct unreaped child is the only fallback
                    # target when observation itself is unavailable.
                    self.receipts.block('cleanup raised unexpectedly: %r' % cleanup_error)
                    if self.failure is None:
                        self.failure = cleanup_error
                    if self.proc is not None:
                        raw_signal_child(self.proc, signal.SIGKILL, self.receipts)
                        try:
                            self.proc.wait(timeout=max(0, min(1, self.wall_deadline - time.monotonic())))
                            self.receipts.emit('command_reap', job=self.number,
                                               pid=self.proc.pid, exit=self.proc.returncode)
                        except Exception as reap_error:
                            self.receipts.block('last direct-child reap unconfirmed: %r' % reap_error)
        print(log.read_text(), end='', flush=True)
        self.receipts.absorb_child_blocks()
        if self.failure is not None:
            raise self.failure
        if len(self.receipts.blocks) != before_blocks:
            raise Block('cleanup BLOCK; command success is not batch success')


def source_manifest(receipts, filename='source-manifest.json'):
    files = []
    for package in ('wswhiteboard', 'wsi18n', 'wsdashboard', 'wsholder'):
        directory = ROOT / 'agents-plugin-tool' / 'internal' / package
        files.extend(p for p in directory.rglob('*') if p.is_file() and
                     p.suffix in ('.go', '.js', '.cjs', '.css', '.tmpl', '.json', '.py'))
    files.extend(ROOT / 'agents-plugin-tool' / p for p in ('go.mod', 'go.sum'))
    manifest = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in sorted(files)}
    (OUT / filename).write_text(json.dumps(manifest, indent=2) + '\n')
    receipts.emit('source_manifest', declared_base=BASE,
                  branch='impl/feat/content-change-hover-677/awoke-stir-icon',
                  path=str(OUT / filename), files=len(manifest))


def main():
    if os.environ.get('BODYTIME_MAIN_SLOT') != 'START':
        raise Block('fresh explicit MAIN START is required; preparation is not authority')
    if not (Path(MODULE) / 'package.json').is_file():
        raise Block('MAIN-supplied Playwright module is absent')
    OUT.mkdir(mode=0o700, exist_ok=False)
    supervisor = (OUT / 'supervisor.log').open('w')
    original_out, original_err = sys.stdout, sys.stderr
    sys.stdout, sys.stderr = Tee(original_out, supervisor), Tee(original_err, supervisor)
    receipts = Receipts(OUT / 'receipts.jsonl')
    started = time.monotonic()
    wall_deadline = started + 720
    env = dict(os.environ, WS_PLAYWRIGHT_MODULE=MODULE,
               BODYTIME_PLAYWRIGHT_MODULE=MODULE, PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD='1',
               GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local',
               BODYTIME_RECEIPT_PATH=str(receipts.path))
    commands = [
        (['gofmt', '-w', W + 'bodytime_ui_test.go'], 15, False),
        (['git', 'diff', '--check'], 10, False),
        (['node', '--check', W + 'testdata/bodytime-hover.cjs'], 15, False),
        (['go', '-C', 'agents-plugin-tool', 'test', '-json', '-count=1', '-timeout=110s',
          './internal/wswhiteboard', './internal/wsi18n'], 120, False),
        (['go', '-C', 'agents-plugin-tool', 'test', '-json', '-count=1', '-timeout=80s',
          '-run', SELECTOR, './internal/wsdashboard'], 90, False),
        (['go', '-C', 'agents-plugin-tool', 'run',
          './internal/wswhiteboard/testdata/bodytime-hover.go',
          '-out', str(OUT / 'fixture')], 90, False),
        (['node', W + 'testdata/bodytime-hover.cjs', str(OUT / 'fixture'),
          str(OUT / 'captures')], 360, True),
    ]
    def interrupted(number, _frame):
        raise Block('supervisor interrupted by signal %s' % number)
    previous = {number: signal.signal(number, interrupted)
                for number in (signal.SIGINT, signal.SIGTERM, signal.SIGALRM)}
    signal.setitimer(signal.ITIMER_REAL, max(.001, wall_deadline - time.monotonic()))
    try:
        if os.getpgrp() != os.getpid():
            os.setsid()  # The supervisor itself never shares a caller's group.
        own = snapshot(wall_deadline - 12, receipts).get(os.getpid())
        if own is None or own['pgid'] != os.getpid():
            raise Block('supervisor start/group identity was not established')
        receipts.emit('batch_start', **own, hard_seconds=720,
                      output=str(OUT), omissions=__doc__)
        source_manifest(receipts, 'pre-format-source-manifest.json')
        for number, (argv, cap, browser) in enumerate(commands, 1):
            Job(number, argv, cap, env, receipts, wall_deadline, browser).run()
            if number == 1:
                source_manifest(receipts)  # Exact inputs after the only formatter.
        geometry = json.loads((OUT / 'captures' / 'geometry.json').read_text())
        captures = list((OUT / 'captures').glob('*.png'))
        if len(geometry) != 96 or len(captures) != 120:
            raise Block('offline matrix/capture count is incomplete')
        receipts.emit('batch_pass', elapsed=time.monotonic() - started,
                      hover_cases=96, captures=120, acceptance='offline only')
        print('BATCH PASS: 96 offline hover cases, 120 captures;', OUT, flush=True)
        return 0
    except BaseException as err:
        receipts.emit('batch_failure', error=repr(err), elapsed=time.monotonic() - started,
                      cleanup_block=bool(receipts.blocks), retry=False)
        print('FIRST FAILURE; NO RETRY:', repr(err), file=sys.stderr, flush=True)
        traceback.print_exc()
        return 1
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        for number, handler in previous.items():
            signal.signal(number, handler)
        sys.stdout.flush()
        sys.stderr.flush()
        sys.stdout, sys.stderr = original_out, original_err
        supervisor.close()


if __name__ == '__main__':
    sys.exit(main())
