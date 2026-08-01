#!/usr/bin/env bash
# Exercise the image-build skill generator against a local manifest so a
# malicious id cannot escape the generated skills root.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/source/docs/skills/nested-skill"
cat >"$tmpdir/source/docs/skills/valid-skill.md" <<'EOF'
---
name: valid-skill
description: "A valid skill."
---

# Valid Skill
EOF

cat >"$tmpdir/source/docs/skills/nested-skill/SKILL.md" <<'EOF'
---
name: nested-skill
description: "A nested skill."
---

# Nested Skill
EOF

cat >"$tmpdir/index.json" <<EOF
{
  "skills": [
    {
      "id": "valid-skill",
      "name": "valid-skill",
      "description": "A valid skill.",
      "entry_point": "docs/skills/valid-skill.md"
    },
    {
      "id": "nested-skill",
      "name": "nested-skill",
      "description": "A nested skill.",
      "entry_point": "docs/skills/nested-skill/SKILL.md"
    },
    {
      "id": "$tmpdir/escaped",
      "name": "escaped",
      "description": "Must not escape the output directory.",
      "entry_point": "docs/skills/escaped.md"
    },
    {
      "id": "invalid-entry",
      "name": "invalid-entry",
      "description": "Must stay below the skills directory.",
      "entry_point": "../outside.md"
    }
  ]
}
EOF

python3 "$repo_root/scripts/generate-skills.py" \
  --index "$tmpdir/index.json" \
  --raw-base "$tmpdir/source" \
  --out "$tmpdir/out" \
  2>"$tmpdir/stderr"

test -f "$tmpdir/out/valid-skill/SKILL.md"
test -f "$tmpdir/out/nested-skill/SKILL.md"
test ! -e "$tmpdir/escaped"
grep -Fq "skipped $tmpdir/escaped: invalid id" "$tmpdir/stderr"
grep -Fq 'skipped invalid-entry: invalid entry_point' "$tmpdir/stderr"

echo "✓ skill generator rejects unsafe manifest paths."
