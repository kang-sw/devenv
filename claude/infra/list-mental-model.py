#!/usr/bin/env python3
"""List mental-model domain docs relevant to given source paths.

Usage:
  python3 ~/.claude/infra/list-mental-model.py [path ...]

  No args  : list all domain docs.
  With args: list docs whose 'sources' frontmatter patterns overlap
             with any of the provided paths (directory-level matching).

Output (stdout): YAML map keyed by domain name.
  All paths are relative to ai-docs/mental-model/.
  overview is always included when it exists.
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
            m = re.match(r'^([\w][\w-]*):\s*(.*)', line)
            if not m:
                cur_key = None
                continue
            cur_key = m.group(1)
            rest = m.group(2).rstrip()
            if rest in ('', '[]', '{}', 'null', '~'):
                result[cur_key] = None
            else:
                result[cur_key] = rest.strip('"\'')
        elif cur_key is not None:
            if stripped.startswith('- '):
                val = stripped[2:].strip()
                if not isinstance(result.get(cur_key), list):
                    result[cur_key] = []
                result[cur_key].append(val)
            else:
                m = re.match(r'^  (.+?):\s*(.*)', line)
                if m:
                    if not isinstance(result.get(cur_key), dict):
                        result[cur_key] = {}
                    sub_key = m.group(1).strip()
                    sub_val = m.group(2).rstrip().strip('"\'')
                    result[cur_key][sub_key] = sub_val if sub_val not in ('null', '~', '') else None

    return result


def extract_first_line(path: Path) -> str:
    """Return the first non-heading, non-empty content line (for overview.md)."""
    try:
        text = path.read_text(encoding='utf-8')
    except OSError:
        return ''
    in_frontmatter = False
    for line in text.splitlines():
        s = line.strip()
        if s == '---':
            in_frontmatter = not in_frontmatter
            continue
        if in_frontmatter:
            continue
        if s and not s.startswith('#'):
            return s[:120]
    return ''


# ---------------------------------------------------------------------------
# Matching
# ---------------------------------------------------------------------------

def overlaps(sources: list[str], targets: list[str]) -> bool:
    for pat in sources:
        pat_norm = pat.rstrip('/')
        for tgt in targets:
            tgt_norm = tgt.rstrip('/')
            if tgt_norm.startswith(pat_norm) or pat_norm.startswith(tgt_norm):
                return True
    return False


# ---------------------------------------------------------------------------
# YAML rendering
# ---------------------------------------------------------------------------

def qs(s: str) -> str:
    """Double-quoted YAML scalar with minimal escaping."""
    return '"' + s.replace('\\', '\\\\').replace('"', '\\"') + '"'


def render_domain(domain: str, fm: dict, doc_name: str) -> None:
    print(f'{domain}:')
    print(f'  path: {qs(doc_name)}')

    desc = (fm.get('description') or '').strip()
    if desc:
        print(f'  description: {qs(desc)}')

    related = fm.get('related')
    if isinstance(related, dict) and related:
        print('  related:')
        for rel_domain, note in related.items():
            note_str = (note or '').strip()
            print(f'    {rel_domain}: {qs(note_str)}')

    sources = fm.get('sources')
    if isinstance(sources, list) and sources:
        print('  sources:')
        for src in sources:
            print(f'    - {qs(src)}')


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> None:
    sys.stdout.reconfigure(encoding='utf-8')
    mental_model_dir = Path('ai-docs/mental-model')
    if not mental_model_dir.is_dir():
        print('(no ai-docs/mental-model/ found)', file=sys.stderr)
        sys.exit(1)

    targets = sys.argv[1:]

    print('# Mental-model docs — paths relative to ai-docs/mental-model/ (overview: ../mental-model.md)')

    # mental-model.md — index, always included, no frontmatter
    overview = Path('ai-docs/mental-model.md')
    if overview.exists():
        desc = extract_first_line(overview)
        fm_ov: dict = {}
        if desc:
            fm_ov['description'] = desc
        render_domain('overview', fm_ov, '../mental-model.md')

    for doc in sorted(mental_model_dir.glob('*.md')):
        fm = parse_frontmatter(doc)
        domain = (fm.get('domain') or doc.stem).strip()

        if targets:
            sources = fm.get('sources')
            if not isinstance(sources, list) or not sources:
                continue
            if not overlaps(sources, targets):
                continue

        render_domain(domain, fm, doc.name)


if __name__ == '__main__':
    main()
