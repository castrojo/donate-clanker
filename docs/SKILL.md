# review Skill Router

After reading [`AGENTS.md`](../AGENTS.md) and the local
[agentic model](factory/agentic-model.md), choose the one task-specific
document below. Load only the matching skill.

| Task | Skill |
|---|---|
| Change a launcher recipe, VM mode, or container-only mode | [`launcher.md`](skills/launcher.md) |
| Investigate the contributor runtime, task delivery, or token lifetime | [`hive-runtime.md`](skills/hive-runtime.md) |
| Investigate an assigned-task or connection problem | [`hive-triage.md`](skills/hive-triage.md) |
| Report evidence to or follow up on a `kubestellar/hive` issue | [`upstream-hive.md`](skills/upstream-hive.md) |
| Change Goose, Context7, or skill loading | [`goose-context.md`](skills/goose-context.md) |
| Build a Goose Desktop MCP App panel or resource | [`mcp-app.md`](skills/mcp-app.md) |
| Change the contributor image or pinned image inputs | [`image-build.md`](skills/image-build.md) |
| Prepare a branch, commit, or pull request | [`pr-workflow.md`](skills/pr-workflow.md) |
| Maintain documentation, skills, or factory compliance | [`skill-improvement.md`](skills/skill-improvement.md) |

`docs/skills/index.json` is the machine-readable catalog. Its entries mirror
the frontmatter in each skill file. When changing a skill, update its catalog
entry in the same change.
