#!/usr/bin/env bash
# Conformance checks for docs/skills/ — enforces that:
#   - index.schema.json exists and is valid JSON
#   - index.md exists
#   - index.json validates against the schema
#   - every .md file (except index.md) has an index.json entry and vice-versa
#
# These checks complement scripts/check-skill-frontmatter.sh; that script owns
# front-matter field-level validation, this one owns structural conformance.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

fail=0

ok() { printf '  ok  %s\n' "$*"; }
fail_msg() { printf '::error::%s\n' "$*"; fail=1; }

# 1. index.schema.json must exist and parse as JSON.
if [[ ! -f docs/skills/index.schema.json ]]; then
  fail_msg "docs/skills/index.schema.json is missing"
else
  python3 -c "import json,sys; json.load(open('docs/skills/index.schema.json'))" \
    2>&1 || { fail_msg "docs/skills/index.schema.json does not parse as JSON"; fail=1; }
  ok "docs/skills/index.schema.json exists and parses"
fi

# 2. index.md must exist.
if [[ ! -f docs/skills/index.md ]]; then
  fail_msg "docs/skills/index.md is missing"
else
  ok "docs/skills/index.md exists"
fi

# 3. index.json must validate against the schema (requires jsonschema).
python3 - <<'PY'
import json, sys, os
try:
    import jsonschema
except ImportError:
    print("  skip schema validation: jsonschema not installed")
    sys.exit(0)

schema_path = "docs/skills/index.schema.json"
index_path  = "docs/skills/index.json"
if not os.path.exists(schema_path) or not os.path.exists(index_path):
    sys.exit(0)

try:
    schema  = json.load(open(schema_path))
    catalog = json.load(open(index_path))
    jsonschema.validate(catalog, schema)
    print("  ok  index.json validates against index.schema.json")
except jsonschema.ValidationError as exc:
    print("::error file=docs/skills/index.json::schema validation: %s" % exc.message)
    sys.exit(1)
except jsonschema.SchemaError as exc:
    print("::error file=docs/skills/index.schema.json::invalid schema: %s" % exc.message)
    sys.exit(1)
PY
[[ $? -eq 0 ]] || fail=1

# 4. Every .md file (not index.md) must have an index.json entry and vice-versa.
python3 - <<'PY'
import json, os, sys
skill_dir = "docs/skills"
catalog = json.load(open(os.path.join(skill_dir, "index.json")))
ids = {e["id"] for e in catalog.get("skills", [])}
stems = {
    os.path.splitext(name)[0]
    for name in os.listdir(skill_dir)
    if name.endswith(".md") and name != "index.md"
}
errors = []
for s in sorted(stems - ids):
    errors.append("docs/skills/%s.md has no index.json entry" % s)
for i in sorted(ids - stems):
    errors.append("index.json entry '%s' has no docs/skills/%s.md" % (i, i))
for msg in errors:
    print("::error file=docs/skills/index.json::" + msg)
if errors:
    sys.exit(1)
print("  ok  index.json entries match .md files (%d skills)" % len(ids))
PY
[[ $? -eq 0 ]] || fail=1

if [[ "$fail" -eq 0 ]]; then
  echo "skill-conformance: all checks passed"
else
  echo "skill-conformance: one or more checks failed"
  exit 1
fi
