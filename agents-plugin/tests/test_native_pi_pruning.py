"""Producer pruning rules and local runtime target assertions, entirely offline."""
import json
from pathlib import Path
import tempfile
import unittest
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

    def test_closure_key_ignores_first_party_and_npm_root_bookkeeping(self):
        dependencies = {"p": "1"}
        lock = {"packages": {"": {"name": "a", "private": True}, "node_modules/p": {"version": "1"}}}
        other = {"packages": {"": {"name": "b"}, "node_modules/p": {"version": "1"}}, "requires": True}
        self.assertEqual(PAYLOAD.closure_key(lock, dependencies, 1), PAYLOAD.closure_key(other, dependencies, 1))
        self.assertNotEqual(PAYLOAD.closure_key(lock, dependencies, 1), PAYLOAD.closure_key(lock, dependencies, 2))
