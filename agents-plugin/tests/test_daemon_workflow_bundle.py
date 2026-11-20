"""Offline package/start-context contracts; never starts a daemon or a harness.

The Go reader test imports reader_fixture below. SDK module/class identity is
covered by internal/wsagent/testdata/pi-native-payload-identity.test.mjs; this
suite checks that the old sibling seam keeps the original module location.
"""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import threading
import unittest
import uuid
import zipfile
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import patch

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
LEGACY_REV = "f3efb8510"
LEGACY_SHA256 = "11e81de2435faf81d2905e13dbdfabc213f12f0ead526b269a2e9246b2b72bce"
PRODUCTS = {"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PAYLOAD = load(REPO / "agents-plugin/bin/ws-pi-payload.py", "workflow_payload")
PRODUCER = load(REPO / "agents-plugin-pi/scripts/prepare-workflow-bundles.py", "workflow_producer")


def historical_source(path, expected):
    # Immutable source fixture works in a source sync without .git or network.
    fixture = json.loads((Path(__file__).parent / "fixtures/native-pi-f3efb8510.json").read_bytes())
    if fixture["revision"] != LEGACY_REV:
        raise AssertionError("frozen materializer revision changed")
    record = fixture["files"][path]
    source = record["text"].encode()
    if record["sha256"] != expected or hashlib.sha256(source).hexdigest() != expected:
        raise AssertionError("frozen materializer source changed")
    return source


def legacy_payload(base):
    source = historical_source("agents-plugin/bin/ws-pi-payload.py", LEGACY_SHA256)
    target = base / "legacy-materializer.py"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(source)
    return load(target, "frozen_workflow_payload")


def copy_owner(base, identity):
    owner = base / PRODUCTS[identity]
    for name in ("native-pi", "rsrc", "skills", "bin", "runtime.json", "config-manifest.json", "agy-config-manifest.json",
                 "dashboard-config-manifest.json", ".claude-plugin", ".codex-plugin", ".mcp.json",
                 "hooks", "claude-agents"):
        source, dest = REPO / PRODUCTS[identity] / name, owner / name
        dest.parent.mkdir(parents=True, exist_ok=True)
        if source.is_dir():
            shutil.copytree(source, dest, ignore=shutil.ignore_patterns("__pycache__"))
        else:
            shutil.copyfile(source, dest)
    return owner


def copy_plain_pi(base):
    source = REPO / "agents-plugin-pi"
    pi = base / "standalone-pi"
    package = json.loads((source / "package.json").read_bytes())
    for name in ("package.json", *package["files"]):
        src, dest = source / name, pi / name
        # Source-only Go syncs omit the gitignored pack-time skills copy.
        # Assemble those same bytes from its canonical input without Node/npm.
        if name.rstrip("/") == "skills" and not src.exists():
            src = REPO / "agents-plugin/skills"
        dest.parent.mkdir(parents=True, exist_ok=True)
        if src.is_dir():
            shutil.copytree(src, dest, ignore=shutil.ignore_patterns("__pycache__", "node_modules", ".runtime"))
        else:
            shutil.copyfile(src, dest)
    # Offline installed production deps, not checkout/host node_modules. The
    # canonical closure is already checked by test_native_pi_payload.py.
    with zipfile.ZipFile(REPO / "agents-plugin/native-pi/closure.zip") as archive:
        archive.extractall(pi)
    return pi


def reader_fixture(base, lane):
    """Copied package-only start env, shared with the Go old-reader test."""
    base = Path(base).resolve()
    identity = "wsflow" if lane.endswith("wsflow") else "ws"
    if lane == "plain":
        pi = copy_plain_pi(base)
    else:
        owner = copy_owner(base, identity)
        helper = legacy_payload(base) if lane.startswith("legacy-") else PAYLOAD
        pi = helper.materialize(owner, base / "native-cache")
    launcher = load(pi / "bin/ws-mcp-launcher.py", "copied_workflow_launcher")
    env = {"WS_MCP_CONFIG_MANIFESTS": str(pi / "config-manifest.json"),
           "WS_MCP_HARNESS": "pi", "WS_CACHE_HOME": str(base / "cache"),
           "WS_CONFIG_HOME": str(base / "config"), "HOME": str(base / "home"),
           "USERPROFILE": str(base / "home"), "UNRELATED": "retained"}
    # Normal startup with roots initially unset uses these existing caller
    # helpers before constructing the distinct subprocess environment.
    launcher.apply_rsrc_root_env(pi, env)
    launcher.apply_skills_root_env(pi, env)
    start = launcher.daemon_start_env(env, str(base))
    return {"pi_root": str(pi), "identity": identity, "start_env": start,
            "hello_env": launcher.hello_env(env, str(base))}


class DaemonWorkflowBundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="daemon-workflow-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name).resolve()

    def test_ws_launch_configs_list_exact_three_existing_manifests(self):
        root = REPO / PRODUCTS["ws"]
        expected = ["config-manifest.json", "agy-config-manifest.json", "dashboard-config-manifest.json"]
        for config_path, prefix in ((root / ".mcp.json", "./"),
                                    (root / ".claude-plugin/plugin.json", "${CLAUDE_PLUGIN_ROOT}/")):
            with self.subTest(config=str(config_path)):
                config = json.loads(config_path.read_bytes())
                paths = config["mcpServers"]["ws"]["env"]["WS_MCP_CONFIG_MANIFESTS"].split(";")
                self.assertEqual(paths, [prefix + name for name in expected])
                for name in expected:
                    self.assertTrue((root / name).is_file(), name)

    def assert_package(self, root, identity):
        source = REPO / PRODUCTS[identity]
        plugin = json.loads((root / ".claude-plugin/plugin.json").read_bytes())
        self.assertEqual(plugin, json.loads((source / ".claude-plugin/plugin.json").read_bytes()))
        self.assertEqual(plugin["name"], identity)
        self.assertEqual(json.loads((root / "runtime.json").read_bytes())["plugin"], identity)
        # Compare the actual config bytes, not just manifest names or marker
        # identity. wsflow must not inherit ws's permissive Claude defaults.
        for name in ("config-manifest.json", "agy-config-manifest.json", "dashboard-config-manifest.json", "hooks/hooks.json"):
            self.assertEqual((root / name).read_bytes(), (source / name).read_bytes(), name)
        defaults = {item["key"]: item["default"] for item in json.loads((root / "config-manifest.json").read_bytes())["keys"]}
        self.assertEqual(defaults["claude.compaction_advisory_enabled"], identity == "ws")
        self.assertTrue(defaults["claude.manual_injection_enabled"])
        self.assertEqual(defaults["claude.mailbox_drain"], "direct" if identity == "ws" else "notice")
        for agent in plugin["agents"]:
            self.assertTrue((root / agent).is_file(), agent)
        server = plugin["mcpServers"][identity]
        for arg in server["args"]:
            if "${CLAUDE_PLUGIN_ROOT}/" in arg:
                self.assertTrue(Path(arg.replace("${CLAUDE_PLUGIN_ROOT}", str(root))).is_file(), arg)
        for manifest in server["env"]["WS_MCP_CONFIG_MANIFESTS"].split(";"):
            self.assertTrue(Path(manifest.replace("${CLAUDE_PLUGIN_ROOT}", str(root))).is_file())
        hooks = json.loads((root / "hooks/hooks.json").read_bytes())
        pending = [root / "hooks" / name for name in hooks["modules"]]
        self.assertIn(root / "hooks/claude-mod/register.ts", pending)
        # Pin the command hook AND the transitive relative module closure.
        self.assertIn("${CLAUDE_PLUGIN_ROOT}/hooks/compact-revive-nudge.py", json.dumps(hooks))
        self.assertTrue((root / "hooks/compact-revive-nudge.py").is_file())
        scanned = set()
        while pending:
            path = pending.pop().resolve()
            if path in scanned:
                continue
            scanned.add(path)
            self.assertTrue(path.is_relative_to(root), path)
            self.assertTrue(path.is_file(), path)
            self.assertEqual(path.read_bytes(), (source / path.relative_to(root)).read_bytes())
            for name in re.findall(r'''(?:from\s*|import\s*\()\s*["'](\.[^"']+)["']''', path.read_text()):
                target = path.parent / name
                candidates = [target, target.with_suffix(".ts"), target.with_suffix(".tsx"), target / "index.ts"]
                resolved = next((p for p in candidates if p.is_file()), None)
                self.assertIsNotNone(resolved, f"missing relative hook import {path}: {name}")
                pending.append(resolved)
        self.assertGreater(len(scanned), 1, "test must follow imports beyond register.ts")
        self.assertFalse((root / "native-pi").exists(), "workflow archive must not recursively embed native payloads")

    def assert_start(self, fixture):
        pi = Path(fixture["pi_root"])
        identity = fixture["identity"]
        start, hello = fixture["start_env"], fixture["hello_env"]
        root = Path(start["WS_RSRC_ROOT"]).parent
        self.assertEqual(root.name, identity)
        self.assertEqual(root.parent.parent, pi / ".runtime/workflow")
        self.assertEqual(start["WS_SKILLS_ROOT"], str(root / "skills"))
        self.assertEqual(hello["WS_RSRC_ROOT"], str(pi / "rsrc"))
        self.assertEqual(hello["WS_MCP_CONFIG_MANIFESTS"], str(pi / "config-manifest.json"))
        self.assertEqual(start["UNRELATED"], "retained")
        self.assertEqual(start["WS_MCP_HARNESS"], "pi")
        self.assertEqual(start["WS_CACHE_HOME"], hello["WS_CACHE_HOME"])
        plugin = json.loads((root / ".claude-plugin/plugin.json").read_bytes())
        expected_env = {key: value.replace("${CLAUDE_PLUGIN_ROOT}", str(root))
                        for key, value in plugin["mcpServers"][identity]["env"].items()}
        for key, value in expected_env.items():
            self.assertEqual(start[key], value)
        sibling = root.parent / "agents-plugin-pi"
        self.assertEqual(json.loads((sibling / "package.json").read_bytes())["pi"]["extensions"], ["./index.mjs"])
        self.assertEqual((sibling / "index.mjs").read_text(),
                         "import { createPiForwarder } from " + json.dumps((pi / "bin/ws-pi-forward.mjs").as_uri()) + ";\n" +
                         "export default createPiForwarder(" + json.dumps((pi / "dist/index.js").as_uri()) + ");\n")
        self.assert_package(root, identity)
        return root

    def test_ci_and_release_gates_check_workflow_source_freshness(self):
        for path in (".github/workflows/ws-mcp-ci.yml", ".github/workflows/ws-mcp-release.yml",
                     "agents-plugin-tool/scripts/build-release-assets.sh"):
            with self.subTest(path=path):
                text = (REPO / path).read_text(encoding="utf-8")
                self.assertRegex(text, r"prepare-workflow-bundles\.py[\"']? --check")

    def test_version_bump_refreshes_workflows_before_native_first_party_offline(self):
        script = (REPO / "agents-plugin-tool/scripts/bump-ws-version.sh").read_text(encoding="utf-8")
        # The bump must produce (not just check) workflow archives before
        # snapshotting them into native Pi, without reinstalling dependencies.
        self.assertIn("set -eu\n", script)
        self.assertRegex(script, r'PY\n\n(?:#[^\n]*\n)*'
                         r'python3 "\$repo_root/agents-plugin-pi/scripts/prepare-workflow-bundles\.py"\n'
                         r'python3 "\$repo_root/agents-plugin-pi/scripts/prepare-native-payload\.py" --first-party-only\n')

    def test_producer_inventory_and_common_first_party_are_complete(self):
        records = PRODUCER.prepare(REPO / "agents-plugin-pi", check=True)
        self.assertEqual((REPO / "agents-plugin-pi/bundle-inputs/lock.json").read_bytes(),
                         (REPO / "agents-plugin-pi/package-lock.json").read_bytes())
        self.assertEqual(set(records), {"ws", "wsflow"})
        for identity, product in PRODUCTS.items():
            record = records[identity]
            source = PRODUCER.package_files(REPO / product)
            self.assertEqual(record["files"], {name: hashlib.sha256(data).hexdigest() for name, data in source.items()})
            bundle = REPO / "agents-plugin-pi/workflow-bundles" / (identity + ".zip")
            self.assertEqual(hashlib.sha256(bundle.read_bytes()).hexdigest(), record["archive_sha256"])
            with zipfile.ZipFile(bundle) as archive:
                self.assertEqual(sorted(archive.namelist()), sorted(source))
                for name, data in source.items():
                    self.assertEqual(archive.read(name), data, f"{identity}/{name}")
        for product in PRODUCTS.values():
            first = REPO / product / "native-pi/first-party"
            inventory = json.loads((first.parent / "first-party-manifest.json").read_bytes())["files"]
            for name in PAYLOAD.WORKFLOW_BUNDLE_FILES:
                self.assertIn(name, inventory)
                self.assertEqual((first / name).read_bytes(), (REPO / "agents-plugin-pi" / name).read_bytes())
        # The producer also supports validation from a supplied package alone.
        pi = copy_plain_pi(self.base)
        self.assertEqual(PRODUCER.prepare(pi, check=True), records)
        (pi / "workflow-bundles/ws.zip").write_bytes(b"damaged")
        with self.assertRaises((ValueError, zipfile.BadZipFile)):
            PRODUCER.prepare(pi, check=True)

    def test_copied_plain_and_native_start_context_and_real_package(self):
        for lane in ("plain", "native-ws", "native-wsflow"):
            with self.subTest(lane=lane):
                fixture = reader_fixture(self.base / lane, lane)
                root = self.assert_start(fixture)
                self.assertFalse((self.base / lane / "agents-plugin-pi").exists())
                pi = Path(fixture["pi_root"])
                launcher = load(pi / "bin/ws-mcp-launcher.py", "start_context_launcher")
                # Poison every product-specific setting: selecting ws must
                # clear wsflow flags, and selecting wsflow must set its own.
                env = {**fixture["hello_env"], "WS_MCP_NO_AGENT": "wrong", "WS_MCP_NAMESPACE": "wrong",
                       "WS_MCP_SETUP_TOOL": "wrong", "WS_MCP_CLI_NAME": "wrong"}
                before = dict(env)
                start = launcher.daemon_start_env(env, str(self.base))
                self.assertEqual(env, before)
                if lane != "native-wsflow":
                    for name in ("WS_MCP_NO_AGENT", "WS_MCP_NAMESPACE", "WS_MCP_SETUP_TOOL"):
                        self.assertNotIn(name, start)
                # Real start_daemon call, but staging and subprocess execution
                # are intercepted. No launcher entrypoint/executable is run.
                fake = self.base / "never-executed-runtime"
                fake.write_bytes(b"fixture only")
                metadata = {"version": "0.50.1"}
                with patch.object(launcher, "read_runtime_metadata", return_value=metadata), \
                     patch.object(launcher, "stage_daemon_binary", return_value=fake), \
                     patch.object(launcher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", "")) as run:
                    link = launcher.DaemonLink(fake, "0.50.1", "linux", env)
                    self.assertTrue(link.start_daemon())
                    self.assertEqual(run.call_args.kwargs["env"]["WS_RSRC_ROOT"], str(root / "rsrc"))
                    self.assertEqual(link.env, before)
                    hello = launcher.proxy_hello("mcp", ["serve", "--stdio"], link.env, "0.50.1")
                    self.assertEqual(hello["env"]["WS_RSRC_ROOT"], str(pi / "rsrc"))
                    self.assertEqual(hello["env"]["WS_MCP_NAMESPACE"], "wrong")

    def test_explicit_relative_invalid_and_legacy_roots_keep_their_meaning(self):
        pi = copy_plain_pi(self.base)
        launcher = load(pi / "bin/ws-mcp-launcher.py", "root_precedence_launcher")
        for identity in PRODUCTS:
            owner = copy_owner(self.base / identity, identity)
            for rsrc in (str(owner / "rsrc"), os.path.relpath(owner / "rsrc", self.base)):
                env = {"WS_RSRC_ROOT": rsrc, "WS_SKILLS_ROOT": str(owner / "skills"), "WS_MCP_NAMESPACE": identity}
                self.assertEqual(launcher.daemon_start_env(env, str(self.base)), launcher.hello_env(env, str(self.base)))
        relative = {"WS_RSRC_ROOT": os.path.relpath(pi / "rsrc", self.base),
                    "WS_SKILLS_ROOT": os.path.relpath(pi / "skills", self.base)}
        selected = launcher.daemon_start_env(relative, str(self.base))
        self.assertEqual(Path(selected["WS_RSRC_ROOT"]).parent.name, "ws")
        self.assertNotEqual(selected, launcher.hello_env(relative, str(self.base)))
        for root in ("missing/rsrc", "missing/alternate", ""):
            env = {"WS_RSRC_ROOT": root, "WS_SKILLS_ROOT": "caller/skills"}
            self.assertEqual(launcher.daemon_start_env(env, str(self.base)), launcher.hello_env(env, str(self.base)))
        env = {"WS_RSRC_ROOT": str(pi / "rsrc"), "WS_SKILLS_ROOT": str(pi / "skills")}
        for damage in ("missing-archive", "checksum-drift", "invalid-archive", "missing-bundles"):
            with self.subTest(damage=damage):
                archive = pi / "workflow-bundles/ws.zip"
                original = archive.read_bytes()
                manifest = pi / "workflow-bundles/manifest.json"
                original_manifest = manifest.read_bytes()
                if damage == "missing-archive":
                    archive.unlink()
                elif damage in ("checksum-drift", "invalid-archive"):
                    archive.write_bytes(b"not a zip")
                    if damage == "invalid-archive":
                        record = json.loads(original_manifest)
                        record["ws"]["archive_sha256"] = hashlib.sha256(archive.read_bytes()).hexdigest()
                        manifest.write_text(json.dumps(record))
                else:
                    shutil.rmtree(pi / "workflow-bundles")
                with patch.object(launcher, "note"):
                    self.assertEqual(launcher.daemon_start_env(env, str(self.base)), env)
                if damage != "missing-bundles":
                    archive.write_bytes(original)
                    manifest.write_bytes(original_manifest)

    def test_old_helper_new_ws_payload_uses_real_ws_defaults(self):
        self.assert_start(reader_fixture(self.base / "old-new-ws", "legacy-ws"))

    def test_old_helper_new_payload_wsflow_and_new_helper_old_payload(self):
        fixture = reader_fixture(self.base / "old-new", "legacy-wsflow")
        root = self.assert_start(fixture)
        owner = self.base / "old-new/agents-plugin-wsflow"
        old = legacy_payload(self.base)
        cache = self.base / "old-new/native-cache"
        entry = Path(fixture["pi_root"])
        self.assertEqual(PAYLOAD.materialize(owner, cache), entry)
        retained = {name: (entry / name).read_bytes() for name in PAYLOAD.WORKFLOW_BUNDLE_FILES}
        self.assertEqual(old.materialize(owner, cache), entry)
        self.assertEqual(retained, {name: (entry / name).read_bytes() for name in retained})
        self.assertTrue(root.is_dir())
        # Make the OLD writer actually reconstruct the new payload, not merely
        # reuse its directory. It must retain both supplied bundle owners.
        (entry / "workflow-bundles/wsflow.zip").write_bytes(b"warm corruption")
        self.assertEqual(old.materialize(owner, cache), entry)
        self.assertEqual(retained, {name: (entry / name).read_bytes() for name in retained})
        launcher = load(entry / "bin/ws-mcp-launcher.py", "old_writer_repaired_launcher")
        fixture["start_env"] = launcher.daemon_start_env(fixture["hello_env"], str(self.base))
        self.assert_start(fixture)
        # Historical schema-1/package shape with today's valid extension bytes.
        # The base checkout's source dist was newer than its prepared native
        # dist, so this is an old-format fixture, not a historical release load.
        # The immutable old helper validates it before the new helper consumes it.
        owner = copy_owner(self.base / "new-old", "wsflow")
        native = owner / "native-pi"
        first = native / "first-party"
        manifest_path = native / "first-party-manifest.json"
        manifest = json.loads(manifest_path.read_bytes())
        for name in PAYLOAD.WORKFLOW_BUNDLE_FILES:
            manifest["files"].pop(name)
            (first / name).unlink()
        (first / "workflow-bundles").rmdir()
        # Notices, like workflow archives, are additive to this older format.
        (first / "dist/THIRD_PARTY_NOTICES").unlink(missing_ok=True)
        (first / "bin/ws-pi-forward.mjs").unlink(missing_ok=True)
        for relative, target, expected in (
                ("agents-plugin/native-pi/first-party-manifest.json", manifest_path,
                 "9dda90585e6e312d1ab366510be2a7ef54fcd15968951394d1bbe6652456ac9a"),
                ("agents-plugin/native-pi/first-party/package.json", first / "package.json",
                 "f499af7f31021e6d6e813eda418ef9ef0c988d1f99a35b29f6a396378101c03e")):
            target.write_bytes(historical_source(relative, expected))
        historical = json.loads(manifest_path.read_bytes())
        self.assertEqual(set(historical["files"]), set(PAYLOAD.tree(first)))
        historical["files"] = PAYLOAD.tree(first)
        historical["bundle_input_hash"] = json.loads((first / "bundle-inputs.json").read_bytes())["sha256"]
        manifest_path.write_text(json.dumps(historical))
        manifest = historical
        self.assertTrue(set(old.FIRST_PARTY_FILES).issubset(manifest["files"]))
        old.validate(owner)
        PAYLOAD.validate(owner)
        entry = PAYLOAD.materialize(owner, self.base / "new-old-cache")
        self.assertFalse((entry / "workflow-bundles").exists())
        launcher = load(entry / "bin/ws-mcp-launcher.py", "legacy_payload_new_launcher")
        env = {"WS_RSRC_ROOT": str(entry / "rsrc"), "WS_SKILLS_ROOT": str(entry / "skills")}
        self.assertEqual(launcher.daemon_start_env(env, str(self.base)), env)
        self.assertEqual(json.loads((entry / "runtime.json").read_bytes())["plugin"], "wsflow")

    def test_checksum_warm_drift_and_cold_recovery(self):
        pi = copy_plain_pi(self.base)
        launcher = load(pi / "bin/ws-mcp-launcher.py", "recovery_launcher")
        root = launcher.pi_daemon_workflow_root(pi)
        original = (root / "hooks/claude-mod/register.ts").read_bytes()
        for damage in ("modified", "extra", "missing"):
            with self.subTest(damage=damage):
                target = root / "hooks/claude-mod/register.ts"
                if damage == "modified":
                    target.write_bytes(b"corrupt")
                elif damage == "extra":
                    (root / "unexpected").write_bytes(b"extra")
                else:
                    target.unlink()
                self.assertEqual(launcher.pi_daemon_workflow_root(pi), root)
                self.assertEqual(target.read_bytes(), original)
                self.assertFalse((root / "unexpected").exists())
        archive = pi / "workflow-bundles/ws.zip"
        supplied = archive.read_bytes()
        archive.write_bytes(b"checksum drift")
        with self.assertRaisesRegex(ValueError, "checksum"):
            launcher.pi_daemon_workflow_root(pi)
        self.assertEqual((root / "hooks/claude-mod/register.ts").read_bytes(), original)
        archive.write_bytes(supplied)
        self.assertEqual(launcher.pi_daemon_workflow_root(pi), root)
        shutil.rmtree(pi / ".runtime/workflow")
        self.assertEqual(launcher.pi_daemon_workflow_root(pi), root)
        # Changing the original module creates a new key and preserves the old
        # entry; the shim must still import the original, not a relocated copy.
        bundle = pi / "dist/index.js"
        bundle.write_bytes(bundle.read_bytes() + b"\n// fixture drift\n")
        updated = launcher.pi_daemon_workflow_root(pi)
        self.assertNotEqual(updated, root)
        self.assertTrue(root.is_dir())
        self.assertIn(bundle.as_uri(), (updated.parent / "agents-plugin-pi/index.mjs").read_text())
        bootstrap = pi / "bin/ws-pi-forward.mjs"
        bootstrap.write_bytes(bootstrap.read_bytes() + b"\n// bootstrap drift\n")
        self.assertNotEqual(launcher.pi_daemon_workflow_root(pi), updated,
                            "forwarding bootstrap bytes participate in the private cache key")

    def write_archive(self, pi, files, modes=None):
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for name, data in files.items():
                info = zipfile.ZipInfo(name)
                info.create_system = 3
                info.external_attr = (modes or {}).get(name, stat.S_IFREG | 0o644) << 16
                archive.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED)
        data = stream.getvalue()
        dest = pi / "workflow-bundles"
        (dest / "ws.zip").write_bytes(data)
        (dest / "manifest.json").write_text(json.dumps({"ws": {
            "archive_sha256": hashlib.sha256(data).hexdigest(),
            "files": {name: hashlib.sha256(body).hexdigest() for name, body in files.items()}}}))

    def test_archive_path_type_size_inventory_and_checksum_guards(self):
        pi = copy_plain_pi(self.base)
        launcher = load(pi / "bin/ws-mcp-launcher.py", "safety_launcher")
        for name in ("../escape", "/absolute", "C:/drive", "back\\slash", "x/../escape", "x//odd", "directory/"):
            with self.subTest(name=name):
                self.write_archive(pi, {name: b"unsafe"})
                with self.assertRaisesRegex(ValueError, "unsafe"):
                    launcher.pi_daemon_workflow_root(pi)
        self.assertFalse((self.base / "escape").exists())
        self.write_archive(pi, {"linked": b"../../outside"}, {"linked": stat.S_IFLNK | 0o777})
        with self.assertRaisesRegex(ValueError, "unsafe"):
            launcher.pi_daemon_workflow_root(pi)
        self.write_archive(pi, {"huge": b"x" * (8 * 1024 * 1024 + 1)})
        with self.assertRaisesRegex(ValueError, "unsafe"):
            launcher.pi_daemon_workflow_root(pi)
        self.write_archive(pi, {str(i): b"x" for i in range(9)})
        original_infos = launcher.zipfile.ZipFile.infolist
        def oversized(archive):
            infos = original_infos(archive)
            for info in infos:
                info.file_size = 8 * 1024 * 1024
            return infos
        with patch.object(launcher.zipfile.ZipFile, "infolist", oversized):
            with self.assertRaisesRegex(ValueError, "too large"):
                launcher.pi_daemon_workflow_root(pi)
        self.write_archive(pi, {"file": b"data"})
        manifest = pi / "workflow-bundles/manifest.json"
        good = json.loads(manifest.read_bytes())
        for mutation, error in (("checksum", "checksum"), ("inventory", "inventory"), ("count", "inventory")):
            record = json.loads(json.dumps(good))
            if mutation == "checksum":
                record["ws"]["files"]["file"] = "0" * 64
            elif mutation == "inventory":
                record["ws"]["files"]["absent"] = "0" * 64
            else:
                record["ws"]["files"] = {str(i): "0" * 64 for i in range(4097)}
            manifest.write_text(json.dumps(record))
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, error):
                launcher.pi_daemon_workflow_root(pi)
        manifest.write_bytes(b" " * (2 * 1024 * 1024 + 1))
        with self.assertRaisesRegex(ValueError, "manifest too large"):
            launcher.pi_daemon_workflow_root(pi)
        manifest.write_text(json.dumps(good))
        (pi / "workflow-bundles/ws.zip").write_bytes(b"x" * (16 * 1024 * 1024 + 1))
        with self.assertRaisesRegex(ValueError, "size"):
            launcher.pi_daemon_workflow_root(pi)
        self.assertFalse(list((pi / ".runtime/workflow").glob("*/ws")))

    def test_symlink_cache_is_not_followed(self):
        pi = copy_plain_pi(self.base)
        launcher = load(pi / "bin/ws-mcp-launcher.py", "symlink_launcher")
        outside = self.base / "outside"
        outside.mkdir()
        runtime = pi / ".runtime"
        try:
            runtime.symlink_to(outside, target_is_directory=True)
        except OSError as exc:
            self.skipTest(f"symlinks unavailable: {exc}")
        with self.assertRaisesRegex(ValueError, "link"):
            launcher.pi_daemon_workflow_root(pi)
        self.assertEqual(list(outside.iterdir()), [])
        runtime.unlink()
        root = launcher.pi_daemon_workflow_root(pi)
        target = root / "hooks/compact-revive-nudge.py"
        target.unlink()
        sentinel = outside / "sentinel"
        sentinel.write_bytes(b"do not change")
        target.symlink_to(sentinel)
        self.assertEqual(launcher.pi_daemon_workflow_root(pi), root)
        self.assertFalse(target.is_symlink())
        self.assertEqual(sentinel.read_bytes(), b"do not change")

    def test_atomic_publish_failure_and_concurrent_cold_and_warm_recovery(self):
        pi = copy_plain_pi(self.base)
        launcher = load(pi / "bin/ws-mcp-launcher.py", "atomic_launcher")
        rename = Path.rename
        def failed_publish(path, target):
            if path.name == "ready":
                raise OSError("fixture publish failure")
            return rename(path, target)
        with patch.object(Path, "rename", failed_publish):
            with self.assertRaisesRegex(OSError, "publish failure"):
                launcher.pi_daemon_workflow_root(pi)
        self.assertEqual([path.name for path in (pi / ".runtime/workflow").iterdir()], ["launcher-start.lock"])
        real_uuid = uuid.uuid4
        for temperature in ("cold", "warm"):
            if temperature == "warm":
                (root / "hooks/compact-revive-nudge.py").write_bytes(b"corrupt")
            barrier = threading.Barrier(4)
            def together():
                barrier.wait(timeout=30)
                return real_uuid()
            with patch.object(launcher.uuid, "uuid4", together), ThreadPoolExecutor(max_workers=4) as pool:
                results = list(pool.map(lambda _: launcher.pi_daemon_workflow_root(pi), range(4)))
            root = results[0]
            self.assertEqual(results, [root] * 4)
            self.assert_package(root, "ws")
            self.assertEqual(list((pi / ".runtime/workflow").glob(".stage-*")), [])
        target = root / "hooks/compact-revive-nudge.py"
        target.write_bytes(b"pre-existing corruption")
        with patch.object(Path, "rename", failed_publish):
            with self.assertRaisesRegex(OSError, "publish failure"):
                launcher.pi_daemon_workflow_root(pi)
        self.assertEqual(target.read_bytes(), b"pre-existing corruption", "failed replacement must roll back")
        self.assertEqual(launcher.pi_daemon_workflow_root(pi), root)
        self.assert_package(root, "ws")


if __name__ == "__main__":
    unittest.main()
