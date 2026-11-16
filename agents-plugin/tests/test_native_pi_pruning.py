"""Producer pruning rules and local runtime target assertions, entirely offline."""
import json
from pathlib import Path
import tempfile
import unittest
import zipfile
from unittest.mock import patch
from test_native_pi_payload import PRODUCER, PAYLOAD


class NativePiPruningTests(unittest.TestCase):
    def test_rules_keep_notices_manifests_and_all_runtime_references(self):
        with tempfile.TemporaryDirectory(prefix="native-prune-") as tmp:
            stage = Path(tmp)
            def put(name, data="fixture"):
                path = stage / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(data)
            roots = ["node_modules/compiled", "node_modules/sourced", "node_modules/typescript"]
            put(roots[0] + "/package.json", json.dumps({"main": "dist/index.js", "module": "dist/esm.js",
                "exports": {".": "./dist/index.js", "./types": "./types/index.d.ts", "./docs": "./docs/runtime.js"},
                "bin": {"x": "./lib/cli.js"}, "browser": "dist/browser.js", "imports": {"#data": "./lib/data.json"}}))
            for name in ("dist/index.js", "dist/esm.js", "types/index.d.ts", "docs/runtime.js", "lib/cli.js", "dist/browser.js", "lib/data.json",
                         "docs/LICENSE.md", "tests/LICENCE.txt", "docs/NOTICE", "docs/COPYING", "docs/AUTHORS", "licenses/OFL.txt",
                         "tests/package.json", "dist/icon.png", "lib/icon.svg"):
                put(roots[0] + "/" + name, "{}" if name.endswith("package.json") else "fixture")
            removed = ("banner.png", "demo.mp4", "docs/manual.html", "example/a.js", "test/x.js", ".github/workflow.yml",
                       "readme.md", "manual.markdown", "dist/index.js.map", "types/private.d.mts", "types/private.d.cts", "build.tsbuildinfo", "src/index.js", "src/unused.js", "src/dead.svg")
            for name in removed:
                put(roots[0] + "/" + name)
            put(roots[0] + "/src/unused.js", 'import "./dead.svg";')
            put(roots[1] + "/package.json", '{"main":"dist/index.js"}')
            put(roots[1] + "/dist/index.js", 'import "../src/live.js";')
            put(roots[1] + "/src/live.js", 'import "./data.json";')
            put(roots[1] + "/src/data.json", "{}")
            put(roots[2] + "/package.json", '{"main":"src/index.ts"}')
            put(roots[2] + "/src/index.ts", "export {}")
            put("src/own.md", "first-party")
            put("dist/own.png", "first-party")
            before = PAYLOAD.tree(stage)
            stats, narrowed = PRODUCER.prune_dependencies(stage, {"packages": {root: {} for root in roots}})
            after = PAYLOAD.tree(stage)
            for name in removed:
                self.assertNotIn(roots[0] + "/" + name, after)
            for name in before:
                if name not in {roots[0] + "/" + n for n in removed}:
                    self.assertEqual(after[name], before[name], name)
            self.assertEqual(set(stats), {"media", "documentation/test directories", "markdown", "source maps", "type declarations", "build metadata", "redundant src"})
            self.assertTrue(all(entry["bytes"] > 0 for entry in stats.values()))
            self.assertTrue(any(x["file"].endswith("types/index.d.ts") for x in narrowed))
            self.assertTrue(any(x["file"].endswith("docs/runtime.js") for x in narrowed))

    def test_missing_declared_runtime_target_fails_before_pruning(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "node_modules/broken"
            root.mkdir(parents=True)
            (root / "package.json").write_text('{"main":"missing.js"}')
            with self.assertRaisesRegex(ValueError, "runtime target missing"):
                PRODUCER.prune_dependencies(Path(tmp), {"packages": {"node_modules/broken": {}}})

    def test_wildcards_and_browser_mappings_protect_every_file(self):
        files = {"node_modules/p/lib/a.js": "", "node_modules/p/lib/b.js": "", "node_modules/p/assets/logo.png": ""}
        pkg = {"exports": {"./*": "./lib/*.js"}, "browser": {"./assets/logo.png": "./assets/logo.png"}}
        self.assertEqual(PAYLOAD.package_targets(pkg, "node_modules/p", files), set(files))

    def test_legal_filename_families_survive_all_prune_rules(self):
        for name in ("LICENSING", "LICENSING-extra.md", "LicensingTerms.map", "ThirdPartyNotice.txt",
                     "thirdpartynotices-extra.html", "THIRD-PARTY-NOTICES.md", "Third_Party_Notices_2026",
                     "COPYRIGHT", "COPYRIGHTauthors.md", "CopyrightTerms.json", "PATENTS", "PATENTS-extra.png",
                     "PatentsGrantDoc.map", "AUTHORS.txt", "NOTICE-extras.md", "ACKNOWLEDGEMENTS.md"):
            with self.subTest(name=name):
                self.assertTrue(PRODUCER.legal_file(name))
                self.assertIsNone(PRODUCER.prune_rule("docs/" + name, True))

    def test_same_key_reuses_committed_encoding_not_compressed_candidate(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            native = repo / "candidate"
            native.mkdir()
            name = "node_modules/p/index.js"
            data = b"same uncompressed runtime content" * 100
            files = {name: PAYLOAD.digest(data)}
            def archive(path, compression, creator):
                with zipfile.ZipFile(path, "w") as z:
                    info = zipfile.ZipInfo(name, (2020, 1, 1, 0, 0, 0))
                    info.create_system = creator
                    z.writestr(info, data, compress_type=compression)
            archive(native / "closure.zip", zipfile.ZIP_STORED, 0)
            different_bytes = (native / "closure.zip").read_bytes()
            for product in ("agents-plugin", "agents-plugin-wsflow"):
                prior = repo / product / "native-pi"
                prior.mkdir(parents=True)
                archive(prior / "closure.zip", zipfile.ZIP_DEFLATED, 3)
                manifest = {"key": "same-key", "files": files, "archive_sha256": PAYLOAD.digest((prior / "closure.zip").read_bytes()), "provenance": "preserve raw JSON too"}
                (prior / "closure-manifest.json").write_text(json.dumps(manifest, indent=4) + "\n")
            old = repo / "agents-plugin/native-pi"
            expected = {leaf: (old / leaf).read_bytes() for leaf in ("closure.zip", "closure-manifest.json")}
            self.assertNotEqual(different_bytes, expected["closure.zip"])
            candidate = {"key": "same-key", "files": files, "archive_sha256": PAYLOAD.digest(different_bytes)}
            retained = PRODUCER.retain_committed_closure(repo, native, candidate)
            self.assertEqual(retained["archive_sha256"], PAYLOAD.digest(expected["closure.zip"]))
            for leaf, content in expected.items():
                self.assertEqual((native / leaf).read_bytes(), content)
                for product in ("agents-plugin", "agents-plugin-wsflow"):
                    self.assertEqual((repo / product / "native-pi" / leaf).read_bytes(), content)
            candidate["files"] = {name: PAYLOAD.digest(b"different CONTENT")}
            with self.assertRaisesRegex(ValueError, "same closure key, different CONTENT: key-input bug or prune change without a version bump"):
                PRODUCER.retain_committed_closure(repo, native, candidate)
            for leaf, content in expected.items():
                self.assertEqual((native / leaf).read_bytes(), content)

    def test_key_change_zip_writer_fixes_windows_creator_and_attributes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            name = "node_modules/p/index.js"
            path = root / name
            path.parent.mkdir(parents=True)
            path.write_bytes(b"export {};\n")
            original = zipfile.ZipInfo
            class WindowsZipInfo(original):
                def __init__(self, *args, **kwargs):
                    super().__init__(*args, **kwargs)
                    self.create_system = 0
            with patch.object(PRODUCER.zipfile, "ZipInfo", WindowsZipInfo):
                PRODUCER.write_zip(root / "new.zip", root, {name: PAYLOAD.digest(path.read_bytes())})
            with zipfile.ZipFile(root / "new.zip") as z:
                info = z.getinfo(name)
                self.assertEqual(info.create_system, 3)
                self.assertEqual(info.external_attr, 0o100644 << 16)
                self.assertEqual(info.date_time, (2020, 1, 1, 0, 0, 0))
                self.assertEqual(z.read(name), path.read_bytes())

    def test_closure_key_ignores_first_party_and_npm_root_bookkeeping(self):
        dependencies = {"p": "1"}
        lock = {"packages": {"": {"name": "a", "private": True}, "node_modules/p": {"version": "1"}}}
        other = {"packages": {"": {"name": "b"}, "node_modules/p": {"version": "1"}}, "requires": True}
        self.assertEqual(PAYLOAD.closure_key(lock, dependencies, 1), PAYLOAD.closure_key(other, dependencies, 1))
        self.assertNotEqual(PAYLOAD.closure_key(lock, dependencies, 1), PAYLOAD.closure_key(lock, dependencies, 2))
