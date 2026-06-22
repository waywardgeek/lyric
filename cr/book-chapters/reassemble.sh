#!/bin/bash
# reassemble.sh — concat per-chapter files back into the-lyric-book.md.
# Order: preface, ch01..ch99, appA..appZ. Sorted lexicographically thanks
# to zero-padded chapter numbers in split.sh.
# Run from repo root: ./cr/book-chapters/reassemble.sh [output_path]
set -e
cd "$(dirname "$0")/../.."
OUT="${1:-the-lyric-book.md}"
DIR="cr/book-chapters"

FILES=()
[ -f "$DIR/preface.md" ] && FILES+=("$DIR/preface.md")
for f in $(ls "$DIR"/ch*.md 2>/dev/null | sort); do FILES+=("$f"); done
for f in $(ls "$DIR"/app*.md 2>/dev/null | sort); do FILES+=("$f"); done

if [ ${#FILES[@]} -eq 0 ]; then
  echo "reassemble: no chapter files found in $DIR" >&2
  exit 1
fi

echo "Concatenating ${#FILES[@]} files -> $OUT"
cat "${FILES[@]}" > "$OUT"
wc -l "$OUT"
