#!/usr/bin/env bash
# Adapted from stock-flow-context at 4c81bd45ff27cca31bd911e5e429b46c273265d2.
# Fails when a relative link in this repo's Markdown points at a file that doesn't exist.
#
# Checks inline links, [text](path). Skipped, because they are examples rather than
# links: fenced code blocks (``` or ~~~) and inline code spans. Not checked: external
# URLs (they need the network and fail for reasons outside the repo), #anchors,
# reference-style links ([text][ref]), and paths containing parentheses.
set -euo pipefail

cd "$(dirname "$0")/.."

files=0
links=0
broken=0

while IFS= read -r -d '' f; do
  files=$((files + 1))
  dir="$(dirname "$f")"
  while IFS= read -r target; do
    target="${target%% \"*}"      # drop a "title"
    target="${target#<}"; target="${target%>}"
    target="${target%%#*}"        # drop an #anchor
    case "$target" in
      ''|http://*|https://*|mailto:*) continue ;;
    esac
    links=$((links + 1))
    case "$target" in
      /*) path=".$target" ;;
      *) path="$dir/$target" ;;
    esac
    if [ ! -e "$path" ]; then
      echo "check-links: $f -> $target (not found)" >&2
      broken=$((broken + 1))
    fi
  done < <(awk '/^[[:space:]]*(```|~~~)/ { fenced = !fenced; next } !fenced' "$f" \
             | sed -E 's/`[^`]*`//g' \
             | grep -oE '\]\([^)]+\)' \
             | sed -E 's/^\]\(//; s/\)$//' || true)
done < <(git ls-files --cached --others --exclude-standard -z -- '*.md' ':!CLAUDE.md' ':!.agents/**' ':!.claude/**')

# A check that scanned nothing proves nothing.
if [ "$files" -eq 0 ]; then
  echo "check-links: no Markdown files found" >&2
  exit 1
fi

if [ "$broken" -ne 0 ]; then
  echo "check-links: $broken broken of $links links in $files files" >&2
  exit 1
fi

echo "check-links: $links links in $files files, none broken"
