#!/usr/bin/env bash
set -euo pipefail
MAX_LINES=499
violations=0
while IFS= read -r f; do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt "$MAX_LINES" ]; then
    echo "$f:1: file is $lines lines (max $MAX_LINES)"
    violations=$((violations + 1))
  fi
done < <(find . -name '*.go' \
  -not -path './vendor/*' \
  -not -path './.knots/*' \
  -not -path './tasks/*' \
  2>/dev/null | sort)
if [ "$violations" -gt 0 ]; then
  echo
  echo "Found $violations file-size violation(s)."
  exit 1
fi
echo "All Go files are within size thresholds."
