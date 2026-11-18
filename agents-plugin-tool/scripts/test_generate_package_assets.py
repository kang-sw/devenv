import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).with_name('generate-package-assets.py')
REPO = SCRIPT.resolve().parents[2]


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


generator = load(SCRIPT, 'package_assets')


class GenerationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.output = self.repo / 'agents-plugin-tool/internal/wspackage/generated'
        for product, folder in generator.PACKAGES.items():
            root = self.repo / folder
            for name in generator.FILES + (('bin/ws-pi-payload.py',) if product == 'ws' else ()):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes((product + '/' + name).encode())
            for tree in generator.TREES + (('native-pi',) if product == 'ws' else ()):
                path = root / tree / 'asset.txt'
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes((product + tree).encode())
        self.source = self.repo / 'agents-plugin/rsrc/asset.txt'

    def generate(self, check=False):
        generator.generate(self.repo, self.output, check)

    def test_missing_stale_add_delete_and_extra(self):
        with self.assertRaisesRegex(ValueError, 'missing/stale/extra'):
            self.generate(check=True)
        self.generate()
        self.generate(check=True)
        for action in ('stale', 'add', 'delete', 'extra', 'nested-placeholder', 'missing'):
            with self.subTest(action=action):
                if action == 'stale':
                    self.source.write_bytes(b'edited')
                elif action == 'add':
                    self.source.with_name('added.txt').write_bytes(b'new')
                elif action == 'delete':
                    self.source.with_name('added.txt').unlink()
                elif action == 'extra':
                    (self.output / 'extra').write_bytes(b'junk')
                elif action == 'nested-placeholder':
                    (self.output / 'ws/PLACEHOLDER').write_bytes(b'junk')
                else:
                    (self.output / 'ws/runtime.json').unlink()
                with self.assertRaises(ValueError):
                    self.generate(check=True)
                self.generate()
                self.generate(check=True)

    def test_package_mapping_is_generated_and_drift_checked(self):
        self.generate()
        index = self.output / generator.PLACEHOLDER
        self.assertEqual(json.loads(index.read_bytes()), generator.PACKAGES)
        inventory = json.loads((self.output / 'inventory.json').read_bytes())
        self.assertEqual(inventory['files'][generator.PLACEHOLDER], hashlib.sha256(index.read_bytes()).hexdigest())
        # Move canonical input without changing any package bytes: only the
        # generated directory mapping (and its inventory hash) should drift.
        (self.repo / 'agents-plugin').rename(self.repo / 'relocated-workflow')
        with mock.patch.dict(generator.PACKAGES, {'ws': 'relocated-workflow'}):
            with self.assertRaisesRegex(ValueError, 'PLACEHOLDER'):
                self.generate(check=True)
            self.generate()
            self.generate(check=True)
            self.assertEqual(json.loads(index.read_bytes())['ws'], 'relocated-workflow')
            index.write_bytes(b'{}')
            with self.assertRaisesRegex(ValueError, 'PLACEHOLDER'):
                self.generate(check=True)

    def test_deterministic_no_rewrites_selection_and_junk(self):
        junk = self.source.parent / '__pycache__'
        junk.mkdir()
        (junk / 'junk.pyc').write_bytes(b'junk')
        for name in ('.DS_Store', '.env', 'private.local.md', 'junk.pyc'):
            self.source.with_name(name).write_bytes(b'private')
        if os.name != 'nt':
            self.source.with_name('link').symlink_to(self.source)
        self.generate()
        files = {p.relative_to(self.output).as_posix(): (p.read_bytes(), p.stat().st_mtime_ns)
                 for p in self.output.rglob('*') if p.is_file()}
        self.generate()
        self.assertEqual(files, {p.relative_to(self.output).as_posix(): (p.read_bytes(), p.stat().st_mtime_ns)
                                for p in self.output.rglob('*') if p.is_file()})
        self.assertNotIn('wsflow/native-pi/asset.txt', files)
        self.assertIn('ws/native-pi/asset.txt', files)
        self.assertIn('ws/.claude-plugin/plugin.json', files)
        self.assertFalse(any('junk' in name or 'private' in name or name.endswith('/link') or '.DS_Store' in name for name in files))
        self.assertNotEqual(files['ws/runtime.json'][0], files['wsflow/runtime.json'][0])

    @unittest.skipIf(os.name == 'nt', 'symlink creation requires Windows privileges')
    def test_generated_symlink_is_not_followed(self):
        self.generate()
        victim = self.repo / 'victim'
        victim.write_bytes(b'untouched')
        asset = self.output / 'ws/rsrc/asset.txt'
        asset.unlink()
        asset.symlink_to(victim)
        with self.assertRaises(ValueError):
            self.generate(check=True)
        self.generate()
        self.assertFalse(asset.is_symlink())
        self.assertEqual(victim.read_bytes(), b'untouched')
        self.generate(check=True)
        extra = self.output / 'extra-link'
        extra.symlink_to(victim)
        with self.assertRaises(ValueError):
            self.generate(check=True)
        self.generate()
        self.assertFalse(extra.is_symlink())
        self.generate(check=True)

    def test_missing_canonical_file_fails(self):
        (self.repo / 'agents-plugin/runtime.json').unlink()
        with self.assertRaisesRegex(ValueError, 'missing regular input file'):
            self.generate()

    def test_cli_check_and_inventory_do_not_write(self):
        args = [sys.executable, str(SCRIPT), '--checkout', str(self.repo), '--output', str(self.output)]
        proc = subprocess.run(args + ['--check'], capture_output=True)
        self.assertEqual(proc.returncode, 1)
        self.assertIn(b'generate-package-assets.py', proc.stderr)
        data = subprocess.check_output(args + ['--inventory'])
        self.assertEqual(data, generator.inventory_bytes(generator.package_files(self.repo)))
        self.assertFalse(self.output.exists())
        subprocess.run(args, check=True)
        subprocess.run(args + ['--check'], check=True)

    def test_fingerprint_canonical_before_generation_and_generated_bytes(self):
        launcher = load(REPO / 'agents-plugin/bin/ws-mcp-launcher.py', 'launcher_assets')
        tool = self.repo / 'agents-plugin-tool'
        scripts = tool / 'scripts'
        scripts.mkdir(parents=True)
        (scripts / SCRIPT.name).write_bytes(SCRIPT.read_bytes())
        go = self.repo / 'go'
        go.write_bytes(b'fake toolchain')
        contract = {'tool_dir': tool, 'go': go}
        with mock.patch.object(launcher, 'read_local_devenv_contract', return_value=contract):
            fingerprint = lambda: launcher.local_devenv_source_fingerprint(self.repo, 'linux')
            before = fingerprint()
            self.source.write_bytes(b'changed before generation')
            self.assertNotEqual(before, fingerprint())
            self.generate()
            ready = fingerprint()
            self.generate()
            self.assertEqual(ready, fingerprint())
            asset = self.output / 'ws/rsrc/asset.txt'
            os.utime(asset, ns=(1, 1))
            self.assertEqual(ready, fingerprint(), 'generated mtimes must not churn stamp')
            asset.write_bytes(b'stale generated bytes')
            self.assertNotEqual(ready, fingerprint())

    def test_launcher_generates_before_build_without_running_go(self):
        launcher = load(REPO / 'agents-plugin/bin/ws-mcp-launcher.py', 'launcher_build_assets')
        runtime = self.repo / 'runtime'
        events = []
        def run(argv, **kwargs):
            events.append(argv)
            return subprocess.CompletedProcess(argv, 0 if len(events) == 1 else 1)
        with mock.patch.object(launcher, 'source_runtime_metadata', return_value={}), \
             mock.patch.object(launcher.subprocess, 'run', side_effect=run):
            self.assertFalse(launcher.build_local_devenv_runtime(runtime, runtime / 'ws-mcp', {},
                {'tool_dir': self.repo / 'agents-plugin-tool', 'go': self.repo / 'go'}, 'linux'))
        self.assertEqual(len(events), 2)
        self.assertTrue(events[0][1].endswith('generate-package-assets.py'))
        self.assertEqual(events[1][1], 'build')
        with mock.patch.object(launcher.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, 'generator')) as run:
            with self.assertRaises(subprocess.CalledProcessError):
                launcher.build_local_devenv_runtime(runtime, runtime / 'ws-mcp', {},
                    {'tool_dir': self.repo / 'agents-plugin-tool', 'go': self.repo / 'go'}, 'linux')
        self.assertEqual(run.call_count, 1, 'generation failure must prevent Go compilation')


if __name__ == '__main__':
    unittest.main()
