---
name: goose-context
version: "1.4"
last_updated: 2026-08-02
id: goose-context
one_line_purpose: Keep Goose config, Context7, and skills available in the guest.
entry_point: docs/skills/goose-context.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [goose, context7, skills, mcp, config]
description: "Keeps Goose configuration, Context7, and global and repository skill routing available in the guest. Use when Goose loses its config, misses a skill, or lacks Context7."
metadata:
  type: reference
---

# Goose Context

## When to Use

Load this when changing Goose configuration, Context7, skill routing, or the
image-owned agent policy.

## Core Process

1. Keep image-owned Goose configuration under
   `GOOSE_PATH_ROOT=/opt/bluefin/goose`. Hive overwrites
   `~/.config/goose/config.yaml` during startup.
2. Keep the image Copilot-only. `GOOSE_PROVIDER` may be unset or
   `github_copilot`; the entrypoint supplies `gpt-4.1` when no model is set.
3. Use Context7 for current external documentation. If its extension is
   unavailable, continue from local repository evidence rather than inventing
   an API or flag.
4. Treat generated global skills and repository skills differently. The image
   generates global skills at build time from the pinned
   `projectbluefin/common` catalog. After cloning a repository, read its
   `docs/skills/index.json` and load the matching entry point.
5. Keep the image policy short. It is supplied to every Goose turn through
   `GOOSE_MOIM_MESSAGE_FILE`.

Goose discovers native skills at session start, before an assigned repository
exists. Repository skills therefore require the explicit catalog lookup; they
are not automatically discovered. Generated `.agents/skills/` content is
build output and must not be committed.

`CONTEXT_FILE_NAMES` includes `AGENTS.md`, `.goosehints`, and `CLAUDE.md` so
the Hive knowledge export can be read. Keep auto-loaded files concise.

## Red Flags

- Writing controlled configuration to `~/.config/goose`.
- Calling repository skill discovery automatic or guaranteed.
- Committing generated `.agents/skills/` output.
- Expanding the persistent policy with task-specific instructions.
- Treating an unavailable Context7 extension as evidence for an unverified
  external claim.

## Verification

```bash
echo "$GOOSE_PATH_ROOT"                 # /opt/bluefin/goose
ls /home/dev/.agents/skills             # generated global skills
python3 -c "import json; json.load(open('docs/skills/index.json'))"
bash scripts/check-skill-frontmatter.sh
bash tests/image-contract.sh
```
