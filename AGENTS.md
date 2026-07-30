# donate-clanker Agent Contract

Read this file first, then [`docs/SKILL.md`](docs/SKILL.md). For factory-wide
rules, use the pinned shared-contract sidecar:
[`projectbluefin/common/docs/skills/factory-onboarding.md`](https://github.com/projectbluefin/common/blob/main/docs/skills/factory-onboarding.md)
and [`projectbluefin/common/docs/factory/agentic-model.md`](https://github.com/projectbluefin/common/blob/main/docs/factory/agentic-model.md).

## Build and validation

```bash
go test ./...
git diff --check
gofmt -l .
just --justfile just/61-donate-clanker.just --list
```

The repository owns the donate-clanker launcher, Hive/Goose worker protocol,
container boundary, and user-facing onboarding. Keep changes scoped to those
surfaces; shared FSDK image changes belong in `projectbluefin/fsdk-containers`.

## Factory workflow

- Target branch: `main`.
- Verify the assigned repository, issue, and requested scope before editing.
- Use existing repository conventions and validation commands.
- Do not bypass review, merge, security, or publication gates.
- Do not create credentials or secrets.

## Self-Improvement

Every session produces the work and the learning. Update the relevant
`docs/skills/*.md` file in the same change when you discover a durable
workaround, pattern, or convention.

Banned:

- No changelog or session-log files.
- No committed planning or progress notes.
- No “append here” documentation; route durable knowledge to `docs/skills/`.

Before marking work done:

- [ ] Relevant skill documentation was updated or no update was needed.
- [ ] Existing checks were run for the changed surface.
- [ ] Remaining gaps are routed to the owning repository or issue.
