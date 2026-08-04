---
name: goose-context
version: "1.8"
last_updated: 2026-08-04
id: goose-context
one_line_purpose: Keep Goose config and skill routing working in the guest.
entry_point: docs/skills/goose-context.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [goose, context7, skills, mcp, config]
description: "Keeps Goose configuration and global and repository skill routing available in the guest, and records that Context7 is Hive's hub-side capability the image must never configure. Use when Goose loses its config or misses a skill."
metadata:
  type: reference
  context7-sources: [/aaif-goose/goose, /websites/cli_github_manual]
---

# Goose Context

## When to Use

Load this when changing Goose configuration, skill routing, or the
image-owned agent policy, or when asked to add a Context7 extension to the
image.

## When Not to Use

Do not load this for Hive assignment selection, contributor tmux lifecycle, or
task delivery; use the Hive runtime documentation instead.

## Core Process

1. Keep image-owned Goose configuration under
   `GOOSE_PATH_ROOT=/opt/bluefin/goose`. The pinned Hive runtime preserves an
   existing `~/.config/goose/config.yaml`; the controlled root still separates
   image policy, data, and state from that runtime-owned file.
2. Keep the image Copilot-only. `GOOSE_PROVIDER` may be unset or
   `github_copilot`; the entrypoint supplies `gpt-5.6-luna` and
   `GOOSE_THINKING_EFFORT=high` when callers do not override them.
3. Goose follows the upstream `canary` release. Build it with the required
   `github_token` secret so GitHub CLI can verify signed provenance from the
   official `canary.yml` workflow; never put that token in an image layer.
4. Do not configure Context7 in this image. Hive owns it: the hub queries
   Context7's API server-side (`v2/pkg/knowledge/context7.go`) and folds the
   result into its knowledge base, which reaches the agent through Hive's
   knowledge export and the Goose-native `AGENTS.md` and `.goosehints` links.
   `tests/image-contract.sh` forbids Context7 strings in the policy, Goose
   config, and entrypoint so the image never grows a second path to it. When a
   client separately provides a Context7 extension, use it for current
   external documentation; otherwise continue from Hive's knowledge and local
   repository evidence rather than inventing an API or flag.
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

The pinned Hive runtime links its refreshed knowledge export to Goose-native
`AGENTS.md` and `.goosehints`. Do not add `CONTEXT_FILE_NAMES` merely to retain
the legacy `CLAUDE.md` link; keep auto-loaded files concise.

## Common Rationalizations

- "The policy wording is cosmetic." The image contract validates specific
  skill-routing strings and forbids any Context7 configuration in the policy,
  Goose config, and entrypoint; change its assertions with the wording when
  the intended behavior changes.
- "A repository skill will load automatically." Native discovery occurs before
  the assigned repository exists, so repository skill routing needs the
  explicit catalog lookup.
- "The policy can contain every skill body." Persistent instructions consume
  context every turn; route to the relevant document instead.
`GOOSE_NO_CODE_TRUNCATION=true` keeps full code blocks visible during reviews
without increasing the bounded tool response size.

## Red Flags

- Treating Hive's preserved runtime config as the image-policy seam.
- Treating the mutable canary tag as a checksum pin or passing its verification
  token through a build argument.
- Calling repository skill discovery automatic or guaranteed.
- Committing generated `.agents/skills/` output.
- Growing `AGENTS.md` with content only some tasks need.
- Expanding the persistent policy with task-specific instructions.
- Changing policy wording without checking `tests/image-contract.sh`.
- Treating absent Context7 output as evidence for an unverified external
  claim. Context7 arrives through Hive's knowledge export, not the image.

## Verification

```bash
echo "$GOOSE_PATH_ROOT"                 # /opt/bluefin/goose
ls "$GOOSE_PATH_ROOT"                   # config survives Hive startup
ls /home/dev/.agents/skills             # generated global skills
python3 -c "import json; json.load(open('docs/skills/index.json'))"
wc -l AGENTS.md docs/skills/*.md        # each under 200
bash scripts/check-skill-frontmatter.sh
bash tests/image-contract.sh
bash tests/hive-compatibility.sh
```
