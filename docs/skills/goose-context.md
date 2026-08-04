---
name: goose-context
version: "1.6"
last_updated: 2026-08-03
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
  context7-sources: [/aaif-goose/goose, /websites/cli_github_manual]
---

# Goose Context

## When to Use

Load this when changing Goose configuration, Context7, skill routing, or the
image-owned agent policy.

## When Not to Use

Do not load this for Hive assignment selection, contributor tmux lifecycle, or
task delivery; use the Hive runtime documentation instead.

## Core Process

1. Keep image-owned Goose configuration under
   `GOOSE_PATH_ROOT=/opt/bluefin/goose`. Hive overwrites
   `~/.config/goose/config.yaml` during startup.
2. Keep the image Copilot-only. `GOOSE_PROVIDER` may be unset or
   `github_copilot`; the entrypoint supplies `gpt-5.6-luna` and
   `GOOSE_THINKING_EFFORT=high` when callers do not override them.
3. Goose follows the upstream `canary` release. Build it with the required
   `github_token` secret so GitHub CLI can verify signed provenance from the
   official `canary.yml` workflow; never put that token in an image layer.
4. Use Context7 for current external documentation. If its extension is
   unavailable, continue from local repository evidence rather than inventing
   an API or flag.
5. Treat generated global skills and repository skills differently. The image
   generates global skills at build time from the pinned
   `projectbluefin/common` catalog. After cloning a repository, read its
   `docs/skills/index.json` and load the matching entry point.
6. Keep the image policy short. It is supplied to every Goose turn through
   `GOOSE_MOIM_MESSAGE_FILE`.
7. Treat the policy strings asserted by `tests/image-contract.sh` as contract
   anchors. Preserve them when changing policy wording, or intentionally update
   their test assertions in the same change.

Goose discovers native skills at session start, before an assigned repository
exists. Repository skills therefore require the explicit catalog lookup; they
are not automatically discovered. Generated `.agents/skills/` content is
build output and must not be committed.

`CONTEXT_FILE_NAMES` includes `AGENTS.md`, `.goosehints`, and `CLAUDE.md` so
the Hive knowledge export can be read. Keep auto-loaded files concise.

## Common Rationalizations

- "The policy wording is cosmetic." The image contract deliberately validates
  specific routing and Context7 fallback behavior; change its assertions with
  the wording when the intended behavior changes.
- "A repository skill will load automatically." Native discovery occurs before
  the assigned repository exists, so repository skill routing needs the
  explicit catalog lookup.
- "The policy can contain every skill body." Persistent instructions consume
  context every turn; route to the relevant document instead.
`GOOSE_NO_CODE_TRUNCATION=true` keeps full code blocks visible during reviews
without increasing the bounded tool response size.

## Red Flags

- Writing controlled configuration to `~/.config/goose`.
- Treating the mutable canary tag as a checksum pin or passing its verification
  token through a build argument.
- Calling repository skill discovery automatic or guaranteed.
- Committing generated `.agents/skills/` output.
- Growing `AGENTS.md` with content only some tasks need.
- Expanding the persistent policy with task-specific instructions.
- Changing policy wording without checking `tests/image-contract.sh`.
- Treating an unavailable Context7 extension as evidence for an unverified
  external claim.

## Verification

```bash
echo "$GOOSE_PATH_ROOT"                 # /opt/bluefin/goose
ls "$GOOSE_PATH_ROOT"                   # config survives Hive startup
ls /home/dev/.agents/skills             # generated global skills
python3 -c "import json; json.load(open('docs/skills/index.json'))"
wc -l AGENTS.md docs/skills/*.md        # each under 200
bash scripts/check-skill-frontmatter.sh
bash tests/image-contract.sh
```
