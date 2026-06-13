#!/bin/bash
# slab_dev.sh — build lyric from source using lyric.sav, then compile+run a test file
set -e
cd /Users/bill/projects/lyric
cp lyric.sav lyric
make update 2>&1 | tail -1
gcc -std=gnu11 -O2 -w -I runtime -o lyric lyric.c -lm
FILE="${1:-testdata/slab_test.ly}"
echo "--- Testing: $FILE ---"
./lyric compile "$FILE" -o /tmp/slab_dev.c 2>/dev/null
gcc -std=gnu11 -O2 -w -I runtime -o /tmp/slab_dev /tmp/slab_dev.c -lm
/tmp/slab_dev
