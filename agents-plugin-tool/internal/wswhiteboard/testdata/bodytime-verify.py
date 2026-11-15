#!/usr/bin/env python3
"""Finite local-only verification batch; run only inside an explicit MAIN slot.

No installation, daemon, launcher, live endpoint, merge or release operation.
Every subprocess gets its own group. Chromium's separate group is registered by
its exact PID/start identity, with a descendant snapshot covering launch failure.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[4]
OUT = Path('/tmp/ws-bodytime-677-undecided-spruce-pointing')
MODULE = '/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright'
W = 'agents-plugin-tool/internal/wswhiteboard/'
GO_FILES = [W + name for name in [
    'store.go', 'topics.go', 'view.go', 'bodytime.go', 'bodytime_test.go',
    'bodytime_folders_test.go', 'render.go', 'regions_test.go',
    'bodytime_render_test.go', 'bodytime_ui_test.go', 'testdata/bodytime-hover.go',
]]
START = time.monotonic()
ENV = dict(os.environ, WS_PLAYWRIGHT_MODULE=MODULE,
           BODYTIME_PLAYWRIGHT_MODULE=MODULE, PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD='1')
OWNERS = OUT / 'captures' / 'browser-owner.json'
CLEANUP_DEADLINE = None


def diagnostic_timeout():
    remaining = 2 if CLEANUP_DEADLINE is None else CLEANUP_DEADLINE - time.monotonic()
    if remaining <= 0:
        raise TimeoutError('bounded cleanup diagnostic budget exhausted')
    return min(2, remaining)


def identity(pid, issues):
    try:
        result = subprocess.run(['/bin/ps', '-p', str(pid), '-o', 'lstart='],
                                capture_output=True, text=True, timeout=diagnostic_timeout())
        if result.returncode == 1 and not result.stdout.strip():
            return None  # ps confirmed that this identity is no longer present.
        if result.returncode != 0 or not result.stdout.strip():
            raise RuntimeError('ps did not establish presence or absence')
        return result.stdout.strip()
    except Exception as err:
        issues.append('cannot identify PID %s: %r' % (pid, err))
        return None


def descendants(root_pid, issues):
    # Read-only, bounded process inventory. Never select by executable name.
    try:
        data = subprocess.run(['/bin/ps', '-axo', 'pid=,ppid=,pgid='],
                              capture_output=True, text=True, check=True, timeout=diagnostic_timeout())
        rows = [tuple(map(int, line.split())) for line in data.stdout.splitlines()]
        owned = {root_pid}
        for _ in range(len(rows)):
            added = {pid for pid, parent, _ in rows if parent in owned} - owned
            if not added:
                break
            owned.update(added)
        return {pid: (pgid, identity(pid, issues)) for pid, _, pgid in rows
                if pid in owned and pid != root_pid}
    except Exception as err:
        issues.append('descendant inventory unavailable: %r' % err)
        return {}


def browser_owner(issues, required=False):
    try:
        if not OWNERS.exists():
            if required:
                issues.append('browser ownership record missing; launch/termination unconfirmed')
            return {}
        record = json.loads(OWNERS.read_text())
        pid = record['pid']
        if not isinstance(pid, int) or pid <= 1 or not record['start']:
            raise RuntimeError('invalid browser ownership record')
        if identity(pid, issues) != record['start']:
            return {}
        try:
            group = os.getpgid(pid)
        except ProcessLookupError:
            return {}
        if group != pid:
            raise RuntimeError('registered Chromium is not its recorded group leader')
        return {pid: (group, record['start'])}
    except Exception as err:
        issues.append('browser ownership unavailable: %r' % err)
        return {}


def signal_owned(owners, sig, issues):
    for pid, (group, born) in owners.items():
        if born is None or identity(pid, issues) != born:
            continue
        try:
            # Signal groups only through their observed, still-matching leader.
            # Nonleaders in the snapshot receive only a PID-targeted signal.
            if pid == group and os.getpgid(pid) == group:
                os.killpg(group, sig)
            else:
                os.kill(pid, sig)
        except ProcessLookupError:
            pass
        except Exception as err:
            issues.append('cannot signal owned PID %s: %r' % (pid, err))


def signal_child(proc, sig, issues):
    try:
        # Popen owns this unreaped child directly. If poll says it is still
        # live, its PID cannot be reused before we signal its new-session group.
        # This guard exists even when diagnostic ps/start-time reading failed.
        if proc.poll() is None:
            if os.getpgid(proc.pid) != proc.pid:
                raise RuntimeError('owned direct child changed process group')
            os.killpg(proc.pid, sig)
    except ProcessLookupError:
        pass
    except Exception as err:
        issues.append('cannot signal owned child %s: %r' % (proc.pid, err))


def stop_child(proc, issues, browser_command):
    owners = descendants(proc.pid, issues)
    owners.update(browser_owner(issues, required=browser_command))
    signal_owned(owners, signal.SIGTERM, issues)
    signal_child(proc, signal.SIGTERM, issues)
    try:
        proc.wait(timeout=max(0, min(5, CLEANUP_DEADLINE - time.monotonic())))
    except subprocess.TimeoutExpired:
        pass
    except Exception as err:
        issues.append('owned child TERM wait: %r' % err)
    signal_owned(owners, signal.SIGKILL, issues)
    signal_child(proc, signal.SIGKILL, issues)
    try:
        proc.wait(timeout=max(0, min(3, CLEANUP_DEADLINE - time.monotonic())))
    except Exception as err:
        issues.append('owned child termination unconfirmed: %r' % err)
    for pid, (_, born) in owners.items():
        if born is not None and identity(pid, issues) == born:
            issues.append('owned descendant termination unconfirmed: PID %s' % pid)


def run(argv, timeout, browser_command=False):
    global CLEANUP_DEADLINE
    remaining = 720 - (time.monotonic() - START) - 15
    if remaining <= 0:
        raise TimeoutError('12-minute batch budget exhausted before next command')
    timeout = min(timeout, remaining)
    index = len(list(OUT.glob('command-*.log'))) + 1
    log = OUT / ('command-%02d.log' % index)
    print('RUN', argv, 'timeout=', timeout, 'log=', log, flush=True)
    with log.open('w') as output:
        command_start = time.monotonic()
        proc = subprocess.Popen(argv, cwd=ROOT, env=ENV, stdout=output,
                                stderr=subprocess.STDOUT, start_new_session=True)
        issues = []
        try:  # Guard begins immediately after Popen, before all diagnostics.
            identity(proc.pid, issues)
            code = proc.wait(timeout=max(0, timeout - (time.monotonic() - command_start)))
        except BaseException:
            CLEANUP_DEADLINE = min(START + 720, time.monotonic() + 12)
            # Diagnostic failures never bypass known-owned cleanup and never
            # replace the original command/timeout exception.
            try:
                stop_child(proc, issues, browser_command)
            except BaseException as err:
                issues.append('cleanup incomplete: %r' % err)
            raise
        finally:
            if CLEANUP_DEADLINE is None:
                CLEANUP_DEADLINE = min(START + 720, time.monotonic() + 12)
            try:
                # Unlike gofmt/tests, this command must account for its
                # detached browser even on an early nonzero/signal exit.
                owner = browser_owner(issues, required=browser_command)
                signal_owned(owner, signal.SIGTERM, issues)
                signal_owned(owner, signal.SIGKILL, issues)
                for pid, (_, born) in owner.items():
                    if identity(pid, issues) == born:
                        issues.append('browser termination unconfirmed: PID %s' % pid)
            except BaseException as err:
                issues.append('browser cleanup incomplete: %r' % err)
            for issue in issues:
                print('CLEANUP BLOCK:', issue, file=sys.stderr, flush=True)
            CLEANUP_DEADLINE = None
    print(log.read_text(), end='', flush=True)
    if code:
        raise subprocess.CalledProcessError(code, argv)
    if issues:
        raise RuntimeError('cleanup BLOCK; ownership/termination was not fully established')


def main():
    if os.environ.get('BODYTIME_MAIN_SLOT') != 'START':
        raise RuntimeError('explicit MAIN START is required; this is not a slot allocator')
    if not (Path(MODULE) / 'package.json').is_file():
        raise RuntimeError('MAIN-supplied existing Playwright module is absent')
    OUT.mkdir(mode=0o700, exist_ok=False)
    print('ARTIFACTS', OUT, flush=True)
    run(['gofmt', '-w', *GO_FILES], 30)
    run(['git', 'diff', '--check'], 30)
    run(['node', '--check', W + 'testdata/bodytime-hover.cjs'], 15)
    run(['go', '-C', 'agents-plugin-tool', 'test', '-count=1', '-timeout=120s',
         './internal/wswhiteboard', './internal/wsi18n', './internal/wsdashboard'], 120)
    run(['go', '-C', 'agents-plugin-tool', 'run',
         './internal/wswhiteboard/testdata/bodytime-hover.go',
         '-out', str(OUT / 'fixture')], 90)
    run(['node', W + 'testdata/bodytime-hover.cjs', str(OUT / 'fixture'),
         str(OUT / 'captures')], 360, browser_command=True)
    print('BATCH COMPLETE', 'elapsed=%.1fs' % (time.monotonic() - START), flush=True)


if __name__ == '__main__':
    try:
        main()
    except BaseException as err:
        print('FIRST FAILURE; NO RETRY:', repr(err), file=sys.stderr, flush=True)
        # Preserve complete first-failure output, including timeout diagnostics.
        if OUT.exists():
            for log in sorted(OUT.glob('command-*.log')):
                print('\n' + str(log) + '\n' + log.read_text(), file=sys.stderr)
        raise
