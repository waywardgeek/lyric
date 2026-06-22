#!/bin/bash
# split.sh — split the-lyric-book.md into per-chapter files in this dir.
# Run from the repo root: ./cr/book-chapters/split.sh
# Boundaries are discovered dynamically from "^## (Preface|Chapter N|Appendix X)" headers.
set -e
cd "$(dirname "$0")/../.."
BOOK="the-lyric-book.md"
OUT="cr/book-chapters"

# Build list of "linenum:slug" entries.
mapfile -t HEADERS < <(grep -nE '^## (Preface|Chapter [0-9]+|Appendix [A-Z])' "$BOOK")
TOTAL=$(wc -l < "$BOOK")

slug_of() {
  # "## Chapter 8: Relations..." -> "ch08"
  # "## Appendix A: ..."         -> "appA"
  # "## Preface"                 -> "preface"
  local s="$1"
  if [[ "$s" =~ Preface ]]; then echo preface
  elif [[ "$s" =~ Chapter\ ([0-9]+) ]]; then printf 'ch%02d' "${BASH_REMATCH[1]}"
  elif [[ "$s" =~ Appendix\ ([A-Z]) ]]; then echo "app${BASH_REMATCH[1]}"
  fi
}

rm -f "$OUT"/preface.md "$OUT"/ch*.md "$OUT"/app*.md
n=${#HEADERS[@]}
for ((i=0; i<n; i++)); do
  IFS=: read -r start rest <<<"${HEADERS[i]}"
  if (( i+1 < n )); then
    IFS=: read -r next_start _ <<<"${HEADERS[i+1]}"
    end=$((next_start - 1))
  else
    end=$TOTAL
  fi
  slug=$(slug_of "$rest")
  out="$OUT/${slug}.md"
  sed -n "${start},${end}p" "$BOOK" > "$out"
  printf "%-12s lines %4d..%-4d -> %s\n" "$slug" "$start" "$end" "$out"
done
