#!/usr/bin/env python3
"""Run the directional worker-holder fixture against the pinned old daemon source."""
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

BASE = "f2234e573"
ROOT = Path(__file__).resolve().parents[2]
FIXTURE = ROOT / "agents-plugin-tool/internal/wsagent/testdata/worker-holder-legacy/downgrade_test.go.txt"


def main():
    # Extract a disposable source tree. No checkout, daemon, or installed holder
    # is changed; the Go fixture uses only its in-memory fake holder host.
    with tempfile.TemporaryDirectory(prefix="ws-worker-downgrade-") as directory:
        target = Path(directory)
        archive = target / "source.tar"
        with archive.open("wb") as stream:
            subprocess.run(["git", "archive", BASE, "agents-plugin-tool"], cwd=ROOT, stdout=stream, check=True)
        with tarfile.open(archive) as source:
            source.extractall(target, filter="data")
        package = target / "agents-plugin-tool"
        shutil.copyfile(FIXTURE, package / "internal/wsagent/worker_holder_legacy_test.go")
        shutil.copyfile(ROOT / "agents-plugin-tool/internal/wsagent/worker_holder_io_test.go", package / "internal/wsagent/worker_holder_io_test.go")
        env = dict(os.environ, GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local")
        subprocess.run(["go", "test", "./internal/wsagent", "-run", "^(TestLegacyWorkerHolder|TestWorkerHolderFileOperations)", "-count=1", "-timeout", "5m", "-v"], cwd=package, env=env, check=True)


if __name__ == "__main__":
    main()
