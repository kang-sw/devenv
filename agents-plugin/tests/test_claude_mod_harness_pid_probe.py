"""The Claude mod's harness PID probe (hooks/claude-mod/hosted.ts
HARNESS_PID_SCRIPT) runs under a real python on this OS.

The mod test kit has no process access, so hosted.test.ts can pin only the
argv's shape and the TS parsing of its output; the python script itself is
exercised here. The script is read from hosted.ts, encoded the way
HARNESS_PID_ARGV encodes it (each character's code as two hex digits), and
run as `python -c "exec(bytes.fromhex('<hex>').decode())"`, the one-line form
the mod runs. Its output must be what parseHarnessPid reads: the bare parent
PID off Windows, `<pid> <parent> <image>` on Windows.
"""

import ast
import os
import re
import subprocess
import sys
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
HOSTED_TS = PLUGIN_ROOT / "hooks" / "claude-mod" / "hosted.ts"


def probe_script():
    """HARNESS_PID_SCRIPT as hosted.ts builds it: its string literals joined by newlines."""
    text = HOSTED_TS.read_text(encoding="utf-8")
    match = re.search(r"^const HARNESS_PID_SCRIPT = \[\n(.*?)^\]\.join\('\\n'\)$", text, re.S | re.M)
    if match is None:
        raise AssertionError("hosted.ts holds no `const HARNESS_PID_SCRIPT = [ ... ].join('\\n')` block")
    lines = []
    for raw in match.group(1).splitlines():
        raw = raw.strip()
        if not raw:
            continue
        lines.append(ast.literal_eval(raw.rstrip(",")))
    return "\n".join(lines)


def probe_code():
    """The `-c` argument of HARNESS_PID_ARGV for the script."""
    script = probe_script()
    # The TS encoding is per UTF-16 code unit, two hex digits each: equal to
    # bytes.hex() only while the script is ASCII (encode raises otherwise).
    return "exec(bytes.fromhex('%s').decode())" % script.encode("ascii").hex()


class HarnessPidProbeTest(unittest.TestCase):
    def run_probe(self, argv, **kwargs):
        done = subprocess.run(argv, capture_output=True, text=True, timeout=30, **kwargs)
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(done.stderr, "")
        return done.stdout.strip()

    def test_the_probe_is_one_quote_free_line(self):
        code = probe_code()
        self.assertNotIn("\n", code)
        self.assertNotRegex(code, r'["%&|<>^]')

    @unittest.skipIf(sys.platform == "win32", "Windows output carries the parent and image")
    def test_off_windows_it_prints_the_bare_parent_pid(self):
        self.assertEqual(self.run_probe([sys.executable, "-c", probe_code()]), str(os.getpid()))

    @unittest.skipUnless(sys.platform == "win32", "the process table lookup runs on Windows only")
    def test_on_windows_it_prints_pid_parent_and_image(self):
        out = self.run_probe([sys.executable, "-c", probe_code()])
        match = re.fullmatch(r"([0-9]+) ([0-9]+) (\S.*)", out)
        self.assertIsNotNone(match, out)
        pid, parent, image = int(match.group(1)), int(match.group(2)), match.group(3)
        self.assertTrue(image.lower().endswith(".exe"), out)
        # The printed id is this test process, or (a venv's python.exe
        # redirector between) a child of it.
        self.assertTrue((pid, parent) == (os.getpid(), os.getppid()) or parent == os.getpid(), out)

    @unittest.skipUnless(sys.platform == "win32", "the cmd.exe wrapper exists on Windows only")
    @unittest.skipIf(sys.prefix != sys.base_prefix, "a venv redirector adds a process level")
    def test_on_windows_a_cmd_wrapper_is_reported_with_its_parent(self):
        # shell=True runs `cmd.exe /c "<line>"`, the wrapper Claude Code runs a
        # host command through; the probe then names cmd.exe and this process.
        out = self.run_probe('"%s" -c %s' % (sys.executable, probe_code()), shell=True)
        match = re.fullmatch(r"([0-9]+) ([0-9]+) (\S.*)", out)
        self.assertIsNotNone(match, out)
        self.assertEqual(match.group(3).lower(), "cmd.exe", out)
        self.assertEqual(int(match.group(2)), os.getpid(), out)


if __name__ == "__main__":
    unittest.main()
