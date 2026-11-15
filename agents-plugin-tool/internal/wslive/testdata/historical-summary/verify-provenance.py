#!/usr/bin/env python3
"""Verify retained fixture bytes against original Git blobs in a full clone."""

import hashlib
import json
import pathlib
import subprocess
import tarfile

base = pathlib.Path(__file__).resolve().parent
root = subprocess.check_output(
    ["git", "rev-parse", "--show-toplevel"], cwd=base, text=True
).strip()
for name in ("wslive", "wsagent"):
    manifest = json.loads((base / (name + ".json")).read_text())
    archive = base / manifest["archive"]
    assert hashlib.sha256(archive.read_bytes()).hexdigest() == manifest["archiveSHA256"]
    with tarfile.open(archive) as source:
        members = source.getmembers()
        assert len(members) == len(manifest["files"])
        assert {member.name for member in members} == set(manifest["files"])
        for member in members:
            assert member.isfile()
            retained = source.extractfile(member).read()
            original = subprocess.check_output(
                ["git", "show", manifest["revision"] + ":" + member.name], cwd=root
            )
            assert retained == original, member.name
            assert hashlib.sha256(original).hexdigest() == manifest["files"][member.name]
    print(f"{name}: {len(members)} original blobs verified at {manifest['revision']}; "
          f"{archive.stat().st_size} archive bytes; SHA-256 {manifest['archiveSHA256']}")
