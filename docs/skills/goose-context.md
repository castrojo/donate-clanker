---
name: goose-context
version: "1.2"
last_updated: 2026-08-01
id: goose-context
one_line_purpose: Keep Goose config, Context7, and skills loaded in the guest.
entry_point: docs/skills/goose-context.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [goose, context7, skills, mcp, config]
description: "Explains GOOSE_PATH_ROOT, the Context7 MCP extension, build-time skill generation from index.json, per-repo skill lookup, and CONTEXT_FILE_NAMES. Use when Goose loses its config, misses a skill, or lacks Context7."
metadata:
  type: reference
---

# Goose Context

## Overview

Keep image-owned Goose context available in Hive tmux sessions and route
task-specific guidance from each cloned repository's skill catalog.

## When to Use

Load this when Goose in the guest is missing the Context7 extension, when a
skill that should be available is not, when editing `image/config/goose.yaml`,
or when deciding where a new piece of agent context belongs.

## When NOT to Use

Do not use this for Hive task selection, delivery, or tmux lifecycle changes.

## Core Process

1. Keep image-owned configuration under `GOOSE_PATH_ROOT`, not Hive's rewritten default path.
2. Use global skills, then consult the cloned repository's task-specific catalog.
3. Use Context7 only for current external documentation; otherwise use repository evidence.
4. Keep the contributor profile at `GOOSE_MODE=auto`: Hive injects prompts by
   keystroke, so a permission dialog blocks the session. Treat a restricted
   `DONATE_CLANKER_GH_TOKEN` as the compensating control.

### GOOSE_PATH_ROOT is not optional

Hive **unconditionally overwrites** `~/.config/goose/config.yaml` at every
startup. Any configuration written there is destroyed before the agent runs.

donate-clanker therefore sets `GOOSE_PATH_ROOT` to a controlled config root
at `/opt/bluefin/goose`. Our Goose configuration lives there and survives
Hive's rewrite. That configuration is what declares the Context7 MCP
extension.

If Context7 is missing at runtime, check `GOOSE_PATH_ROOT` first. It is
almost always that, not the extension definition.

### Context7

Context7 is an MCP extension declared in our Goose config. It is the
first-lookup path for external tool documentation: look up current docs
before asserting a flag, label, config key, or API shape from memory. This
matches the factory-wide policy in `projectbluefin/common`.

### Skills are directories

Goose v1.45 has native Agent Skills. A skill is a **directory** containing a
`SKILL.md` with YAML frontmatter carrying at minimum `name` and
`description`.

Only `name` and `description` enter the system prompt. The body loads on
demand through the `load_skill` tool, or deterministically when a human types
`/skill-name`. Write descriptions as routing decisions: capability first,
then "Use when …", 256 characters or fewer.

### Org skills: generated at build time

Goose's discovery roots include `~/.agents/skills/` for global skills. Org
skills are generated **at image build time** from `projectbluefin/common`'s
`docs/skills/index.json` into `/home/dev/.agents/skills/<id>/SKILL.md`.

The org keeps `docs/skills/*.md` plus `index.json` as the source of truth.
Nothing in other repositories changes to support this. The generator is the
artifact worth reviewing; the generated tree is not, and must never be
committed here.

### Per-repo skills: model-driven, not guaranteed

Per-repo skills **cannot** be discovered natively. Goose starts in
`/home/dev` before any repository is cloned, and it enumerates skills at
session start. By the time a repository exists, discovery has already run.

The workaround: the agent is instructed to read `docs/skills/index.json`
after cloning and open only the matching `entry_point`. The image keeps this
instruction in `/opt/bluefin/local-agent-policy.md` and supplies it through
`GOOSE_MOIM_MESSAGE_FILE`, so it remains visible on every turn. The agent
still makes the routing decision; it is not native auto-discovery. Say so
plainly rather than describing it as automatic discovery.

### Global routing policy

`/opt/bluefin/local-agent-policy.md` is the concise, image-baked prompt for
every Goose session. It tells the agent to use matching global Agent Skills,
then consult the cloned repository's catalog. Keep it short: persistent
instructions consume context on every turn. Override its path only with
`GOOSE_MOIM_MESSAGE_FILE` when deliberately replacing this global policy.

### CONTEXT_FILE_NAMES

`CONTEXT_FILE_NAMES` controls which files Goose auto-loads into the system
prompt. `AGENTS.md` is auto-loaded on **every request**, so every line in it
costs tokens on every turn. Keep it under 200 lines and factual. Push
anything task-scoped into `docs/skills/` where it loads on demand.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "Hive starts Goose, so image configuration can live in the default config path." | Hive overwrites that path before the agent runs; use `GOOSE_PATH_ROOT`. |
| "Global skill descriptions make repository skills automatically available." | Repository-local skills appear after session startup; the agent must consult the cloned catalog. |
| "The policy can contain every skill body." | Persistent instructions consume context every turn; route to the relevant document instead. |

`GOOSE_NO_CODE_TRUNCATION=true` keeps full code blocks visible during reviews
without increasing the bounded tool response size.

## Red Flags

- Writing agent configuration to `~/.config/goose/config.yaml`. Hive will
  erase it.
- A skill authored as a bare `.md` file where Goose expects a directory
  containing `SKILL.md`.
- A `description` over 256 characters, or one that is not capability-first.
- Committing generated `.agents/skills/` output.
- Documenting per-repo skill loading as guaranteed or automatic.
- Growing `AGENTS.md` with content only some tasks need.

## Verification

Inside a running guest:

```

echo "$GOOSE_PATH_ROOT"                     # expect /opt/bluefin/goose
ls "$GOOSE_PATH_ROOT"                       # config present after Hive start
ls /home/dev/.agents/skills/                # generated org skills
```

In this repository:

```bash
python3 -c "import json;json.load(open('docs/skills/index.json'))"
wc -l AGENTS.md docs/skills/*.md            # each under 200
bash tests/image-contract.sh
pre-commit run --all-files
```

## The Copilot credential

Goose's `github_copilot` provider needs the long-lived OAuth token minted by
the Copilot editor device flow -- a `ghu_` user-to-server token. It is the only
credential its API accepts.

A `gh auth token` (`gho_`) is **not** a substitute. It is a different OAuth
client with different scopes, and Goose fails at the first model call with
`failed to get api info`. Verified against the contributor image; do not
"simplify" the launcher by passing the gh token here.

On a desktop Goose stores the real token in the login keyring, not in
`~/.config/goose/githubcopilot/info.json` -- that file holds only a short-lived
API token plus metadata and is usually expired. Mounting it into the container
therefore does nothing. The launcher reads `GITHUB_COPILOT_TOKEN` from the
keyring (`secret-tool lookup service goose username secrets`), so the guest
receives exactly one secret and no view of the rest of the host's Goose
configuration.

Both launch paths resolve the credential with the same `resolve_copilot_token`
helper in the justfile; only the delivery differs, because they build their
envelope in different places:

| Path | How the secret travels |
|---|---|
| `donate-clanker-container` | `--env GITHUB_COPILOT_TOKEN=` |
| `donate-clanker` (VM) | `provider_secret` in the v2 bootstrap envelope, over the one-shot socket |

Never a command-line argument and never the runner container's environment:
both would put a live credential where `ps` and `podman inspect` can read it.

`resolve_copilot_token` orders its sources `GITHUB_COPILOT_TOKEN`, then the
keyring. Every failure falls through silently -- note the trailing `|| true`,
which matters under `set -euo pipefail`: a locked or empty keyring makes
`secret-tool` exit non-zero, and `pipefail` would otherwise abort the whole
launch over a lookup that is meant to be optional.

This is best-effort: no keyring, no `secret-tool`, or a locked session just
means the agent runs its own device flow, and the pane waits on
`enter code XXXX-XXXX` until a human types one in. When only a `gh auth token`
is available, both paths say so out loud -- `report_missing_copilot_credential`
prints the same four lines in each -- rather than letting a contributor
discover it as `failed to get api info` inside a guest they cannot read.

`donate-clanker-doctor` reports whether a usable credential can be resolved,
without printing it and without starting anything.

## Sources

- Goose configuration and persistent instructions: `/aaif-goose/goose`
- Skill structure guidance: `/addyosmani/agent-skills`
