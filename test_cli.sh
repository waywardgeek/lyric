#!/bin/bash
# test_cli.sh - Integration tests for the self-hosted Lyric CLI.

set -euo pipefail

cd "$(dirname "$0")"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local want="$2"
  if ! grep -Fq "$want" "$file"; then
    echo "Expected $file to contain: $want" >&2
    echo "--- $file ---" >&2
    cat "$file" >&2
    fail "missing expected text"
  fi
}

make -s lyric
LYRIC="./lyric"
TMPDIR=$(mktemp -d -t lyric_cli_XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

"$LYRIC" help >"$TMPDIR/help.out" 2>"$TMPDIR/help.err"
assert_contains "$TMPDIR/help.out" "Commands:"
assert_contains "$TMPDIR/help.out" "compile"
assert_contains "$TMPDIR/help.out" "test"
assert_contains "$TMPDIR/help.out" "fmt"

if "$LYRIC" does-not-exist >"$TMPDIR/badcmd.out" 2>"$TMPDIR/badcmd.err"; then
  fail "unknown command unexpectedly succeeded"
fi
assert_contains "$TMPDIR/badcmd.err" "unknown command"

cat >"$TMPDIR/hello.ly" <<'EOF'
lyric cli_hello {
  func main() {
    println("cli compile ok")
  }
}
EOF

"$LYRIC" compile "$TMPDIR/hello.ly" -o "$TMPDIR/hello.c" >"$TMPDIR/compile.out" 2>"$TMPDIR/compile.err"
test -s "$TMPDIR/hello.c" || fail "compile did not write C output"
assert_contains "$TMPDIR/hello.c" "cli compile ok"
gcc -std=gnu11 -O2 -w -I runtime -o "$TMPDIR/hello" "$TMPDIR/hello.c" -lm -lpthread
"$TMPDIR/hello" >"$TMPDIR/hello.out"
assert_contains "$TMPDIR/hello.out" "cli compile ok"

cat >"$TMPDIR/bad.ly" <<'EOF'
lyric cli_bad {
  func main() {
    let value: string = 42
    println(value)
  }
}
EOF

if "$LYRIC" compile "$TMPDIR/bad.ly" -o "$TMPDIR/bad.c" >"$TMPDIR/bad.out" 2>"$TMPDIR/bad.err"; then
  fail "bad program unexpectedly compiled"
fi
if [ -e "$TMPDIR/bad.c" ]; then
  fail "failed compile wrote output C file"
fi

cat >"$TMPDIR/unit.ly" <<'EOF'
lyric cli_unit {
  func test_addition() {
    assert_eq(2 + 2, 4, "addition")
  }
}
EOF

"$LYRIC" test "$TMPDIR/unit.ly" >"$TMPDIR/test.out" 2>"$TMPDIR/test.err"
assert_contains "$TMPDIR/test.out" "PASS"

cat >"$TMPDIR/sample.lyric" <<'EOF'
lyric Sample {
struct Point {
x: i32
}
}
EOF

"$LYRIC" fmt "$TMPDIR/sample.lyric" >"$TMPDIR/fmt.out" 2>"$TMPDIR/fmt.err"
assert_contains "$TMPDIR/sample.lyric" "  struct Point {"
assert_contains "$TMPDIR/sample.lyric" "    x: i32"

echo "CLI integration tests passed"
