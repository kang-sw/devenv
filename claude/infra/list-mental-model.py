#!/usr/bin/env python3
"""List mental-model domain docs relevant to given source paths.

Usage:
  python .claude/infra/list-mental-model.py [path ...]

  No args  : list all domain docs.
  With args: list docs whose 'sources' frontmatter patterns overlap
             with any of the provided paths (directory-level matching).

Output (stdout): one entry per matched doc —
  <doc-path>  # domain: <name>[, related: <d1>, <d2>]

overview.md is always included when it exists.
Docs with no 'sources' frontmatter are omitted from filtered results.
"""

import re
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Frontmatter parser — handles scalars, maps, and sequences
# ---------------------------------------------------------------------------

def parse_frontmatter(path: Path) -> dict:
    try:
        text = path.read_text(encoding='utf-8')
    except OSError:
        return {}

    if not text.startswith('---'):
        return {}

    lines = text.splitlines()
    end = next(
        (i for i, ln in enumerate(lines[1:], 1) if ln.rstrip() == '---'),
        None,
    )
    if end is None:
        return {}

    result: dict = {}
    cur_key: str | None = None

    for line in lines[1:end]:
        stripped = line.strip()
        if not stripped or stripped.startswith('#'):
            continue

        if not line[0].isspace():
            # Top-level key
            m = re.match(r'^([\w][\w-]*):\s*(.*)', line)
            if not m:
                cur_key = None
                continue
            cur_key = m.group(1)
            rest = m.group(2).rstrip()
            if rest in ('', '[]', '{}', 'null', '~'):
                result[cur_key] = None  # populated by children
            else:
                result[cur_key] = rest.strip('"\'')
        elif cur_key is not None:
            if stripped.startswith('- '):
                # Sequence item
                val = stripped[2:].strip()
                if not isinstance(result.get(cur_key), list):
                    result[cur_key] = []
                result[cur_key].append(val)
            else:
                # Mapping entry
                m = re.match(r'^  (.+?):\s*(.*)', line)
                if m:
                    if not isinstance(result.get(cur_key), dict):
                        result[cur_key] = {}
                    sub_key = m.group(1).strip()
                    sub_val = m.group(2).rstrip().strip('"\'')
                    result[cur_key][sub_key] = sub_val if sub_val not in ('null', '~', '') else None

    return result


# ---------------------------------------------------------------------------
# Matching
# ---------------------------------------------------------------------------

def overlaps(sources: list[str], targets: list[str]) -> bool:
    """Return True if any target path overlaps with any source pattern."""
    for pat in sources:
        pat_norm = pat.rstrip('/')
        for tgt in targets:
            tgt_norm = tgt.rstrip('/')
            if tgt_norm.startswith(pat_norm) or pat_norm.startswith(tgt_norm):
                return True
    return False


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

def format_entry(doc: Path, fm: dict) -> str:
    domain = fm.get('domain') or doc.stem
    related = fm.get('related')
    suffix = ''
    if isinstance(related, dict) and related:
        suffix = ', related: ' + ', '.join(related.keys())
    return f'{doc}  # domain: {domain}{suffix}'


def main() -> None:
    mental_model_dir = Path('ai-docs/mental-model')
    if not mental_model_dir.is_dir():
        print('(no ai-docs/mental-model/ found)', file=sys.stderr)
        sys.exit(1)

    targets = sys.argv[1:]

    # overview.md — always included if present
    overview = mental_model_dir / 'overview.md'
    if overview.exists():
        print(str(overview))

    for doc in sorted(mental_model_dir.glob('*.md')):
        if doc.name == 'overview.md':
            continue
        fm = parse_frontmatter(doc)

        if not targets:
            # No filter: list all
            print(format_entry(doc, fm))
            continue

        sources = fm.get('sources')
        if not isinstance(sources, list) or not sources:
            continue  # no sources indexed — skip in filtered mode

        if overlaps(sources, targets):
            print(format_entry(doc, fm))


if __name__ == '__main__':
    main()
