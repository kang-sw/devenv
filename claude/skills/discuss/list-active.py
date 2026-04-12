#!/usr/bin/env python3
"""Project map — pre-injected context for the /discuss skill.

Outputs two sections:
  1. ai-docs/ directory tree  (excluding tickets/)
  2. tickets: grouped by status (wip → todo → idea → done → dropped),
     with frontmatter details for parent / related lookups.
"""

import re
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Minimal frontmatter parser — no external dependencies
# ---------------------------------------------------------------------------

def _strip_inline_comment(s: str) -> str:
    """Strip trailing ' # ...' from an unquoted scalar."""
    idx = s.find(' #')
    return s[:idx].rstrip() if idx != -1 else s.rstrip()


def parse_frontmatter(path: Path) -> dict:
    """Parse YAML frontmatter.

    Handles:
      - Simple  key: value  (plain, single-quoted, double-quoted)
      - Empty   key: [] / {} / null / ~  → stored as {}
      - One-level map  key:\\n  subkey: value
    """
    try:
        text = path.read_text(encoding='utf-8')
    except OSError:
        return {}

    if not text.startswith('---'):
        return {}

    lines = text.splitlines()
    end = next(
        (i for i, l in enumerate(lines[1:], 1) if l.rstrip() == '---'),
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
            m = re.match(r'^([\w][\w-]*):\s*', line)
            if not m:
                cur_key = None
                continue
            cur_key = m.group(1)
            rest = line[m.end():].rstrip()
            if rest in ('', '[]', '{}', 'null', '~'):
                result[cur_key] = {}
            elif len(rest) >= 2 and rest[0] == '"' and rest[-1] == '"':
                result[cur_key] = rest[1:-1]
            elif len(rest) >= 2 and rest[0] == "'" and rest[-1] == "'":
                result[cur_key] = rest[1:-1]
            else:
                result[cur_key] = _strip_inline_comment(rest)
        elif cur_key is not None:
            # One-level indented map entry
            m = re.match(r'^  (.+?):\s*(.*)', line)
            if m:
                if not isinstance(result.get(cur_key), dict):
                    result[cur_key] = {}
                sub_key = m.group(1).strip()
                sub_val = _strip_inline_comment(m.group(2))
                result[cur_key][sub_key] = sub_val if sub_val not in ('null', '~', '') else None

    return result


# ---------------------------------------------------------------------------
# Ticket helpers
# ---------------------------------------------------------------------------

STATUS_ORDER = ['wip', 'todo', 'idea']


def find_ticket(stem: str, tickets_root: Path) -> Path | None:
    """Locate a ticket file by stem across all status directories."""
    for status_dir in tickets_root.iterdir():
        if not status_dir.is_dir():
            continue
        candidate = status_dir / f'{stem}.md'
        if candidate.exists():
            return candidate
    return None


def ticket_title(stem: str, tickets_root: Path) -> str:
    """Return the title of a ticket, or empty string if not found."""
    f = find_ticket(stem, tickets_root)
    if f is None:
        return ''
    return parse_frontmatter(f).get('title', '') or ''


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

def dir_tree(root: Path, indent: int = 1) -> list[str]:
    """Indented tree lines for a directory (dirs before files, both sorted)."""
    prefix = '  ' * indent
    lines: list[str] = []
    try:
        entries = sorted(root.iterdir(), key=lambda p: (p.is_file(), p.name))
    except PermissionError:
        return lines
    for entry in entries:
        if entry.is_dir():
            lines.append(f'{prefix}{entry.name}/')
            lines.extend(dir_tree(entry, indent + 1))
        else:
            lines.append(f'{prefix}{entry.name}')
    return lines


def render_ai_docs(ai_docs: Path) -> None:
    print('ai-docs/')
    try:
        entries = sorted(ai_docs.iterdir(), key=lambda p: (p.is_file(), p.name))
    except PermissionError:
        return
    for entry in entries:
        if entry.name == 'tickets':
            continue
        if entry.is_dir():
            print(f'  {entry.name}/')
            for line in dir_tree(entry, indent=2):
                print(line)
        else:
            print(f'  {entry.name}')


def render_tickets(tickets_root: Path) -> None:
    print('tickets:')
    any_ticket = False

    for status in STATUS_ORDER:
        status_dir = tickets_root / status
        if not status_dir.is_dir():
            continue
        for ticket in sorted(status_dir.glob('*.md')):
            any_ticket = True
            fm = parse_frontmatter(ticket)
            stem = ticket.stem
            tag = f'[{status}]'
            date_str = f"  ({fm['started']})" if status == 'wip' and fm.get('started') else ''
            print(f'  {tag} {stem}{date_str}')

            # parent
            parent = fm.get('parent')
            if parent and isinstance(parent, str):
                title = ticket_title(parent, tickets_root)
                suffix = f'  # {title}' if title else ''
                print(f'      parent: {parent}{suffix}')

            # related (map format: stem → note)
            related = fm.get('related')
            if related and isinstance(related, dict):
                for rel_stem, note in related.items():
                    title = ticket_title(rel_stem, tickets_root)
                    parts = [p for p in (str(note) if note else '', title) if p]
                    suffix = '  # ' + ' · '.join(parts) if parts else ''
                    print(f'      related: {rel_stem}{suffix}')

    if not any_ticket:
        print('  (none)')


def main() -> None:
    ai_docs = Path('ai-docs')
    if not ai_docs.is_dir():
        print('(no ai-docs/ found)', file=sys.stderr)
        return

    render_ai_docs(ai_docs)
    print()

    tickets_root = ai_docs / 'tickets'
    if tickets_root.is_dir():
        render_tickets(tickets_root)


if __name__ == '__main__':
    main()
