"""Guard: shipped surfaces must resolve in a downstream project that holds only
what bootstrap installs.

This test is the mechanical form of AGENTS.md Architecture Rule 4 ("Shipped
surfaces are downstream-first"), whose enumeration lives in
ai-docs/manuals/shipped-surface-boundary.md. This file does not restate it.
It scans the shipped trees for tokens that would fail
to resolve for a reader in a project that has never heard of this repository:
real ticket stems, ai-docs file paths bootstrap does not install, bare
ticket-number citations, commit hashes, and this repository's own layout /
tooling names and migration vocabulary.

There is deliberately no allowlist: an allowlist is where the next leak hides.
When this test fails it prints file, line, offending token, and matched rule so
a real future leak is diagnosable from CI output alone.
"""

import re
import subprocess
import unittest
from pathlib import Path
from typing import Optional


REPO_ROOT = Path(__file__).resolve().parents[2]

# The four non-Go shipped text trees, relative to REPO_ROOT. Every git-tracked
# text file under these is scanned line by line.
TEXT_TREES = (
    "agents-plugin/rsrc",
    "agents-plugin/skills",
    "agents-plugin-wsflow/rsrc",
    "agents-plugin-wsflow/skills",
    "agents-plugin-tool/internal/wsdoc/conventions",
)

# Go source root: every non-test .go file under it is scanned, but only the
# string-literal content (comments excluded).
GO_ROOT = "agents-plugin-tool"

TICKET_STATUS_DIRS = ("idea", "todo", "ready", ".done", ".dropped")

# rule 1: a full ticket-stem-shaped token.
STEM_RE = re.compile(r"26[0-9]{4}-[a-z][a-z0-9-]+")
# rule 2: an ai-docs path token.
AI_DOCS_PATH_RE = re.compile(r"ai-docs/[A-Za-z0-9_./<>-]+")
# rule 3: a bare six-digit token.
BARE_NUM_RE = re.compile(r"26[0-9]{4}")
# spec anchors are {#slug}.
SPEC_ANCHOR_RE = re.compile(r"\{#([A-Za-z0-9_-]+)\}")

# rule 4 name-boundary: a repo name is a whole token only when not glued to an
# identifier char on either side. '-' is a boundary char here (unlike \b) so
# `lead-skill-authoring` does not match `skill-authoring`.
NAME_BOUNDARY = set(
    "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
)

# rule 4: this repository's own layout / tooling names. Longer names first so
# the specific token is reported before its prefix.
REPO_NAMES = (
    "agents-plugin-tool",
    "agents-plugin-wsflow",
    "agents-plugin",
    "claude-plugin/",
    "install.sh",
    "skill-authoring",
    "wsflow-mirroring",
)

# rule 4: this repository's migration vocabulary (matched case-insensitively).
MIGRATION_PHRASES = (
    "migration anchor",
    "native-subagent pivot",
    "spawn-removal",
    "host-neutral migration",
    "adapter boundar",
    "retired claude tree",
    "codex-first",
)


def git_tracked_files(repo_root: Path):
    out = subprocess.run(
        ["git", "ls-files"],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    return set(line for line in out.splitlines() if line)


def iter_ticket_stems(tracked):
    stems = set()
    for path in tracked:
        p = Path(path)
        parts = p.parts
        if (
            len(parts) >= 4
            and parts[0] == "ai-docs"
            and parts[1] == "tickets"
            and parts[2] in TICKET_STATUS_DIRS
            and p.suffix == ".md"
        ):
            stems.add(p.stem)
    return stems


# The spec layer was retired; its corpus lives under the tracked archive, and
# closed tickets still cite its anchors. Rule 1 resolves stem-shaped tokens
# against those anchors, so it reads them where they now are - scanning only the
# retired live path would leave that half of the rule matching nothing and
# silently stop catching anchor citations in shipped text.
SPEC_ANCHOR_SOURCES = ("ai-docs/spec/", "ai-docs/.old/spec/")


def iter_spec_anchors(tracked, repo_root: Path):
    anchors = set()
    for path in tracked:
        if path.startswith(SPEC_ANCHOR_SOURCES) and path.endswith(".md"):
            try:
                text = (repo_root / path).read_text(encoding="utf-8")
            except (UnicodeDecodeError, OSError):
                continue
            anchors.update(SPEC_ANCHOR_RE.findall(text))
    return anchors


def bootstrap_installed_ai_docs_files(repo_root: Path):
    """Derive the set of ai-docs files bootstrap installs from the migration
    scaffold block, rather than hardcoding a literal list disconnected from it.

    Inside the `<!-- MIGRATION: ... -->` block, entries listed under the
    `ai-docs/` heading that are concrete filenames (no trailing `/`, no
    `<placeholder>`) become installed files.
    """
    tpl = (
        repo_root / "agents-plugin/skills/lead-bootstrap/AGENTS.template.md"
    ).read_text(encoding="utf-8")
    start = tpl.find("<!-- MIGRATION:")
    if start < 0:
        return set()
    end = tpl.find("-->", start)
    block = tpl[start:end if end >= 0 else len(tpl)]

    installed = set()
    in_ai_docs = False
    for raw in block.splitlines():
        stripped = raw.strip()
        if stripped == "ai-docs/":
            in_ai_docs = True
            continue
        if not in_ai_docs:
            continue
        if not stripped:
            # blank line ends the indented tree listing
            in_ai_docs = False
            continue
        if not raw.startswith(" "):
            # dedented line ends the tree listing
            in_ai_docs = False
            continue
        entry = stripped.split()[0]
        if entry.endswith("/"):
            continue  # directory scaffold
        if "<" in entry or ">" in entry:
            continue  # placeholder like tickets/<status>/
        installed.add(f"ai-docs/{entry}")
    return installed


def extract_go_source_lines(source_text: str):
    """Yield (line_no, text) for the string-literal content of Go source, with
    comments removed.

    A hand-rolled character scanner tracks in-string / in-comment state so that
    a `//` or `/*` inside a real string literal is not treated as a comment
    start, and a `"` inside a comment does not open a fake literal. Multi-line
    raw (backtick) string literals yield one entry per contained source line, so
    each is classified at line granularity like a text file line.
    """
    results = []
    i = 0
    n = len(source_text)
    line = 1
    state = "normal"  # normal | line_comment | block_comment | string | raw
    buf = []
    buf_start_line = 0

    def flush():
        text = "".join(buf)
        for offset, part in enumerate(text.split("\n")):
            results.append((buf_start_line + offset, part))

    while i < n:
        c = source_text[i]

        if state == "normal":
            if c == "/" and i + 1 < n and source_text[i + 1] == "/":
                state = "line_comment"
                i += 2
                continue
            if c == "/" and i + 1 < n and source_text[i + 1] == "*":
                state = "block_comment"
                i += 2
                continue
            if c == '"':
                state = "string"
                buf = []
                buf_start_line = line
                i += 1
                continue
            if c == "`":
                state = "raw"
                buf = []
                buf_start_line = line
                i += 1
                continue
            if c == "'":
                # rune literal: skip to closing ', honoring escapes
                i += 1
                while i < n and source_text[i] != "'":
                    if source_text[i] == "\\":
                        i += 2
                    else:
                        if source_text[i] == "\n":
                            line += 1
                        i += 1
                i += 1
                continue
            if c == "\n":
                line += 1
            i += 1
            continue

        if state == "line_comment":
            if c == "\n":
                line += 1
                state = "normal"
            i += 1
            continue

        if state == "block_comment":
            if c == "*" and i + 1 < n and source_text[i + 1] == "/":
                state = "normal"
                i += 2
                continue
            if c == "\n":
                line += 1
            i += 1
            continue

        if state == "string":
            if c == "\\":
                if i + 1 < n:
                    nxt = source_text[i + 1]
                    buf.append(nxt)
                    if nxt == "\n":
                        line += 1
                    i += 2
                    continue
                i += 1
                continue
            if c == '"':
                flush()
                state = "normal"
                i += 1
                continue
            if c == "\n":
                # not valid in a Go interpreted string; be defensive
                line += 1
                buf.append(c)
                i += 1
                continue
            buf.append(c)
            i += 1
            continue

        if state == "raw":
            if c == "`":
                flush()
                state = "normal"
                i += 1
                continue
            if c == "\n":
                line += 1
            buf.append(c)
            i += 1
            continue

    return results


def classify_line(
    text: str,
    ticket_stems,
    spec_anchors,
    tracked_files,
    installed_files,
    ticket_date_prefixes,
) -> Optional[str]:
    """Return a failure reason for the first Decision-4 rule the line trips, or
    None. Rules are checked in order 1..4."""

    # rule 1: a full ticket-stem-shaped token that resolves to a real ticket or
    # a spec anchor. One that resolves to neither is an example and passes.
    for m in STEM_RE.finditer(text):
        tok = m.group(0)
        if tok in ticket_stems or tok in spec_anchors:
            return f"{tok} (rule 1: resolves to a real ticket/spec anchor)"

    # rule 2: an ai-docs path naming a specific git-tracked file bootstrap does
    # not install. A bare directory or a <placeholder> passes; a gitignored
    # local file (not tracked) passes.
    for m in AI_DOCS_PATH_RE.finditer(text):
        path = m.group(0).rstrip(".,;:!?)\"'`]}")
        if "<" in path or ">" in path:
            continue
        if path.endswith("/"):
            continue
        if path in installed_files:
            continue
        if path in tracked_files:
            return (
                f"{path} (rule 2: names a repo-specific file "
                f"bootstrap does not install)"
            )

    # rule 3: a bare six-digit citation (not a stem head already judged by rule
    # 1) whose date prefix matches a real ticket in this repo.
    for m in BARE_NUM_RE.finditer(text):
        s, e = m.start(), m.end()
        if s > 0 and (text[s - 1].isdigit() or text[s - 1] == "-"):
            continue  # part of a longer number or a stem tail
        if e < len(text) and text[e].isdigit():
            continue  # part of a longer number
        if (
            e + 1 < len(text)
            and text[e] == "-"
            and text[e + 1].isalpha()
            and text[e + 1].islower()
        ):
            continue  # a stem head (rule 1's job), not a bare citation
        tok = m.group(0)
        if tok in ticket_date_prefixes:
            return (
                f"{tok} (rule 3: bare ticket-number citation resolving to a "
                f"real ticket date)"
            )

    # rule 4: migration vocabulary.
    low = text.lower()
    for phrase in MIGRATION_PHRASES:
        if phrase in low:
            return f"{phrase!r} (rule 4: migration vocabulary)"

    # rule 4: this repository's own layout / tooling names as whole tokens.
    for name in REPO_NAMES:
        idx = 0
        while True:
            idx = text.find(name, idx)
            if idx < 0:
                break
            before = text[idx - 1] if idx > 0 else ""
            after_pos = idx + len(name)
            after = text[after_pos] if after_pos < len(text) else ""
            if before in NAME_BOUNDARY:
                idx = after_pos
                continue
            if not name.endswith("/") and after in NAME_BOUNDARY:
                idx = after_pos
                continue
            return f"{name} (rule 4: repo layout/tooling name)"

    # rule 4: a commit hash introduced by the word "commit" on the same line.
    if re.search(r"\bcommit\w*\b", low):
        for hm in re.finditer(r"\b[0-9a-f]{7,40}\b", text):
            htok = hm.group(0)
            if any(ch.isdigit() for ch in htok) and any(
                ch in "abcdef" for ch in htok
            ):
                return f"{htok} (rule 4: commit hash)"

    return None


class ShippedSurfacesDownstreamNeutralTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tracked = git_tracked_files(REPO_ROOT)
        cls.ticket_stems = iter_ticket_stems(cls.tracked)
        cls.ticket_date_prefixes = {
            s[:6]
            for s in cls.ticket_stems
            if len(s) >= 6 and s[:6].isdigit()
        }
        cls.spec_anchors = iter_spec_anchors(cls.tracked, REPO_ROOT)
        cls.installed = bootstrap_installed_ai_docs_files(REPO_ROOT)

    def _classify(self, text):
        return classify_line(
            text,
            self.ticket_stems,
            self.spec_anchors,
            self.tracked,
            self.installed,
            self.ticket_date_prefixes,
        )

    def test_bootstrap_installed_set_is_current(self):
        # Derived from the migration scaffold block; asserted so a template edit
        # that silently changes the set is caught. ai-docs/WORKFLOW.md is the
        # only concrete filename the scaffold lists; everything else there is a
        # directory. ai-docs/mental-model.md was the second entry until the
        # template stopped scaffolding the spec and mental-model layers.
        self.assertEqual(self.installed, {"ai-docs/WORKFLOW.md"})

    def test_go_string_extractor_excludes_comments(self):
        # Comment-exclusion is load-bearing: real comments naming ticket stems
        # and spec anchors are present in the tree (this mirrors one from
        # internal/wsdoc/legacy_marker.go). The extractor must yield the string
        # literal but never the tokens sitting in comments.
        src = (
            "package x\n"
            "// 260726-refactor-retire-spec-planned-marker-mechanism requires the\n"
            'var msg = "downstream copy lives under ai-docs/manuals/"\n'
            "// 260605 migration anchor note\n"
        )
        texts = [t for _, t in extract_go_source_lines(src)]
        blob = "\n".join(texts)
        self.assertIn("downstream copy lives under ai-docs/manuals/", blob)
        self.assertNotIn("260726", blob)
        self.assertNotIn("260605", blob)
        # the surviving literal is downstream-neutral -> classify passes
        self.assertIsNone(self._classify("downstream copy lives under ai-docs/manuals/"))

    def test_allowed_forms_pass(self):
        # The three forms the ticket names as must-pass:
        #   1. an example stem that resolves to nothing,
        #   2. a bare directory mention of ai-docs/manuals/,
        #   3. (covered above) a // 260605 Go comment.
        cases = [
            "Reference tickets by stem only (e.g., `260115-feat-foo-bar`), never by full path.",
            "Route procedures and how-to content to `ai-docs/manuals/`.",
            "See ai-docs/manuals/ for procedures.",
            "Example epic stem 260401-epic-auth-rewrite in a template.",
        ]
        for text in cases:
            self.assertIsNone(
                self._classify(text), f"unexpectedly flagged: {text}"
            )

    def test_known_pre_fix_leak_is_flagged(self):
        # The exact guardrail sentence removed in Phase 1 (recovered from
        # 83b6653a^). Reinserting it into session_state.go must make the guard
        # fail; here the classifier itself flags it (rule 3 bare 260605 first).
        leak = (
            "Before edits or dispatch, run mental-model lookup, read returned "
            "docs ancestors first, read the 260605 migration anchor when target "
            "touches plugin architecture, host-neutral migration, spawn-removal, "
            'or adapter boundaries, and read infra.read("impl-playbook"). '
        )
        self.assertIsNotNone(self._classify(leak))

    def test_rule2_nonbootstrap_ai_docs_path_is_flagged(self):
        # Positive control for rule 2 (the rule behind most cited point-leak
        # sites): naming a git-tracked ai-docs file bootstrap does not install
        # must trip, and be attributed to rule 2. Both files below are tracked
        # today and absent from the bootstrap-installed set. The ref file trips
        # rule 2 only (no repo-name token), so it isolates the rule cleanly.
        for line in (
            "See ai-docs/ref/worktree-ticket-scope.md for the sparse-checkout hazard.",
            "Read ai-docs/manuals/skill-authoring.md before editing skills.",
        ):
            reason = self._classify(line)
            self.assertIsNotNone(reason, f"rule 2 failed to trip on: {line}")
            self.assertIn("rule 2", reason, f"wrong rule attributed for: {line}")

    def test_text_trees_downstream_neutral(self):
        failures = []
        for tree in TEXT_TREES:
            prefix = tree + "/"
            for path in sorted(self.tracked):
                if not path.startswith(prefix):
                    continue
                try:
                    text = (REPO_ROOT / path).read_text(encoding="utf-8")
                except (UnicodeDecodeError, OSError):
                    continue
                for lineno, line in enumerate(text.splitlines(), 1):
                    reason = self._classify(line)
                    if reason:
                        failures.append(f"{path}:{lineno}: {reason}")
        self.assertEqual(failures, [], "\n" + "\n".join(failures))

    def test_go_string_literals_downstream_neutral(self):
        failures = []
        prefix = GO_ROOT + "/"
        for path in sorted(self.tracked):
            if not path.startswith(prefix):
                continue
            if not path.endswith(".go") or path.endswith("_test.go"):
                continue
            try:
                src = (REPO_ROOT / path).read_text(encoding="utf-8")
            except (UnicodeDecodeError, OSError):
                continue
            for lineno, line in extract_go_source_lines(src):
                reason = self._classify(line)
                if reason:
                    failures.append(f"{path}:{lineno}: {reason}")
        self.assertEqual(failures, [], "\n" + "\n".join(failures))


if __name__ == "__main__":
    unittest.main()
