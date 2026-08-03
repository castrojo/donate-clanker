# donate-clanker Skill Router

Agent entry point for `projectbluefin/donate-clanker`. Find the skill that
matches your task, load only that skill, then act.

## Read order

1. [`AGENTS.md`](../AGENTS.md) — repo contract, build commands, boundaries.
2. This file — task→skill mapping.
3. The one skill file named in the table below.

Load one skill. Loading the whole catalog wastes the context budget that the
task itself needs.

## Skill index

| I need to… | Load |
|---|---|
| Change a `just` recipe or the launcher's foreground behavior | [`launcher.md`](skills/launcher.md) |
| Choose between VM mode and container-only mode | [`launcher.md`](skills/launcher.md) |
| Understand how tasks arrive, or debug an attached session | [`hive-runtime.md`](skills/hive-runtime.md) |
| Reason about the 55-minute token or the 168-hour cooldown | [`hive-runtime.md`](skills/hive-runtime.md) |
| Work out why a connected contributor is never handed a task | [`hive-triage.md`](skills/hive-triage.md) |
| Change Goose configuration, Context7, or skill loading | [`goose-context.md`](skills/goose-context.md) |
| Understand why per-repo skills are not auto-discovered | [`goose-context.md`](skills/goose-context.md) |
| Build a Goose Desktop MCP App panel or resource | [`mcp-app.md`](skills/mcp-app.md) |
| Modify the `Containerfile` or what gets layered into the image | [`image-build.md`](skills/image-build.md) |
| Update or verify a pinned digest | [`image-build.md`](skills/image-build.md) |
| Open a pull request, name a branch, or write a commit trailer | [`pr-workflow.md`](skills/pr-workflow.md) |

The machine-readable catalog — every skill's `id`, `category`, `status`, and
one-line purpose — is [`skills/index.json`](skills/index.json). Its values
must match each skill file's frontmatter exactly.

## How to load a skill

Under Goose, org skills installed in `~/.agents/skills/<id>/SKILL.md` load
on demand through the `load_skill` tool, or deterministically when a human
types `/<skill-name>`. Only each skill's `name` and `description` sit in the
system prompt; bodies load when needed.

For skills in this repository, read the file at the `entry_point` path from
the table above or from `index.json`. In a freshly cloned repository, read
`docs/skills/index.json` first and open only the matching `entry_point` —
Goose starts before the clone exists, so nothing here is auto-discovered.

## Writing skills

Every file in `docs/skills/` uses the factory frontmatter: `name`, `version`,
`last_updated`, `id`, `one_line_purpose`, `entry_point`, `category`,
`mcp_compliance_level`, `optimization_status`, `status`, `dependencies`,
`tags`, `description`, and `metadata.type`.

Rules:

- `description` is third person, capability first, then "Use when …", and is
  256 characters or fewer. It is the only body text the model sees before
  loading, so it must be a routing decision on its own.
- 3–6 lowercase tags.
- `id`, `name`, and the filename stem all match, in kebab-case.
- Body skeleton: `# Title`, `## When to Use`, `## Core Process`,
  `## Red Flags`, `## Verification`.
- Under 200 lines. Operational and specific. No filler.

When you add, rename, or retire a skill, update `index.json` in the same pull
request so its entries stay identical to the frontmatter.
