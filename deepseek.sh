#!/usr/bin/env bash
# собирает все .go, go.mod, go.sum в один файл
out=project_dump.txt
: > "$out"
for f in $(find . -type f \( -name '*.go' -name '*.sql' -o -name 'go.mod' -o -name 'go.sum' \) \
           -not -path './vendor/*' -not -path './.git/*' | sort); do
  echo "===== FILE: $f =====" >> "$out"
  cat "$f" >> "$out"
  echo >> "$out"
done