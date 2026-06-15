#!/usr/bin/env python3
"""
verify_book_examples.py — Extract and verify Lyric code examples from the book manuscript.

Usage:
    python3 scripts/verify_book_examples.py [manuscript_path]

Extracts all ```lyric fenced code blocks, attempts to compile each one,
and reports pass/fail with chapter/line references.

Examples are categorized as:
  - complete: has func main() → compile and run directly
  - snippet: no func main() → wrap in func main() { ... } and try
  - fragment: contains "// error:" or is clearly not compilable → skip

Exit code: 0 if all compilable examples pass, 1 otherwise.
"""

import sys
import os
import re
import subprocess
import tempfile
import shutil

LYRIC_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LYRIC_BIN = os.path.join(LYRIC_DIR, "lyric")
RUNTIME_DIR = os.path.join(LYRIC_DIR, "runtime")
STDLIB_DIR = os.path.join(LYRIC_DIR, "stdlib")

DEFAULT_MANUSCRIPT = os.path.expanduser("~/singularity/lyric-book/manuscript.md")


def extract_examples(manuscript_path):
    """Extract all ```lyric code blocks with their line numbers and chapter context."""
    examples = []
    current_chapter = "Preamble"

    with open(manuscript_path) as f:
        lines = f.readlines()

    i = 0
    while i < len(lines):
        line = lines[i]

        # Track chapter headings
        m = re.match(r'^#{1,3}\s+(Chapter \d+|Appendix [A-Z])[:.]?\s*(.*)', line)
        if m:
            current_chapter = m.group(1)
            if m.group(2):
                current_chapter += ": " + m.group(2).strip()

        # Look for ```lyric blocks
        if line.strip() == '```lyric':
            start_line = i + 1  # 1-indexed
            code_lines = []
            i += 1
            while i < len(lines) and lines[i].strip() != '```':
                code_lines.append(lines[i])
                i += 1
            code = ''.join(code_lines)
            examples.append({
                'chapter': current_chapter,
                'line': start_line,
                'code': code,
            })
        i += 1

    return examples


def classify_example(code):
    """Classify an example as 'complete', 'snippet', or 'fragment'."""
    # Fragment: contains deliberate error markers
    if '// error:' in code.lower() or '// Error:' in code:
        return 'fragment'

    # Fragment: just type declarations, no executable code
    stripped = code.strip()
    if not stripped:
        return 'fragment'

    # Complete: has func main()
    if re.search(r'func\s+main\s*\(\s*\)', code):
        return 'complete'

    # Snippet: try wrapping in main
    return 'snippet'


def wrap_snippet(code):
    """Wrap a snippet in func main() { ... }."""
    # If it has top-level declarations (func, struct, class, enum, interface),
    # put them outside main and add a minimal main
    top_level_pattern = re.compile(
        r'^(pub\s+)?(func|struct|class|enum|interface|relation|impl|embed|import)\s',
        re.MULTILINE
    )

    if top_level_pattern.search(code):
        # Has declarations — add a main if there's none
        return code + '\nfunc main() {}\n'
    else:
        # Pure statements — wrap in main
        indented = '\n'.join('    ' + line if line.strip() else '' for line in code.splitlines())
        return f'func main() {{\n{indented}\n}}\n'


def compile_example(code, tmpdir, example_id):
    """Try to compile a Lyric example. Returns (success, error_message)."""
    ly_path = os.path.join(tmpdir, f"ex{example_id}.ly")
    c_path = os.path.join(tmpdir, f"ex{example_id}.c")
    bin_path = os.path.join(tmpdir, f"ex{example_id}")

    with open(ly_path, 'w') as f:
        f.write(code)

    # Step 1: Lyric → C
    result = subprocess.run(
        [LYRIC_BIN, "compile", ly_path, "-o", c_path],
        capture_output=True, text=True, timeout=30,
        cwd=LYRIC_DIR
    )
    if result.returncode != 0:
        return False, f"lyric compile failed:\n{result.stderr.strip()}"

    # Step 2: C → binary
    result = subprocess.run(
        ["gcc", "-std=gnu11", "-O2", "-w", "-o", bin_path, c_path,
         "-I", RUNTIME_DIR, "-lm", "-lpthread"],
        capture_output=True, text=True, timeout=30
    )
    if result.returncode != 0:
        return False, f"gcc failed:\n{result.stderr.strip()}"

    return True, None


def main():
    manuscript_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_MANUSCRIPT

    if not os.path.exists(manuscript_path):
        print(f"ERROR: Manuscript not found: {manuscript_path}")
        sys.exit(1)

    if not os.path.exists(LYRIC_BIN):
        print(f"ERROR: Lyric compiler not found: {LYRIC_BIN}")
        sys.exit(1)

    examples = extract_examples(manuscript_path)
    print(f"Found {len(examples)} lyric code blocks\n")

    tmpdir = tempfile.mkdtemp(prefix="lyric_book_")

    stats = {'complete': 0, 'snippet': 0, 'fragment': 0, 'pass': 0, 'fail': 0}
    failures = []

    try:
        for idx, ex in enumerate(examples):
            kind = classify_example(ex['code'])
            stats[kind] += 1

            if kind == 'fragment':
                print(f"  SKIP  [{ex['chapter']}] line {ex['line']} (fragment)")
                continue

            if kind == 'complete':
                code = ex['code']
            else:
                code = wrap_snippet(ex['code'])

            ok, err = compile_example(code, tmpdir, idx)
            if ok:
                stats['pass'] += 1
                print(f"  PASS  [{ex['chapter']}] line {ex['line']} ({kind})")
            else:
                stats['fail'] += 1
                failures.append((ex, kind, err, code))
                print(f"  FAIL  [{ex['chapter']}] line {ex['line']} ({kind})")

    finally:
        shutil.rmtree(tmpdir, ignore_errors=True)

    # Summary
    print(f"\n{'='*60}")
    print(f"Total: {len(examples)} examples")
    print(f"  Complete programs: {stats['complete']}")
    print(f"  Snippets (wrapped): {stats['snippet']}")
    print(f"  Fragments (skipped): {stats['fragment']}")
    print(f"  PASS: {stats['pass']}")
    print(f"  FAIL: {stats['fail']}")

    if failures:
        print(f"\n{'='*60}")
        print("FAILURES:\n")
        for ex, kind, err, code in failures:
            print(f"--- [{ex['chapter']}] line {ex['line']} ({kind}) ---")
            print(f"Code:\n{code}")
            print(f"Error:\n{err}\n")

    sys.exit(1 if failures else 0)


if __name__ == '__main__':
    main()
