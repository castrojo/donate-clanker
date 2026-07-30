# donate-clanker Skill Router

Read [`AGENTS.md`](../AGENTS.md) first. Load only the skill relevant to the
task, then validate with the repository's existing checks.

## Skill index

| Task | Skill |
| --- | --- |
| Change Hive credentials, Goose execution, mounts, or assignment prompts | [`worker-credential-boundary.md`](skills/worker-credential-boundary.md) |
| Onboard or audit this repository against the factory | [`factory-onboarding.md`](skills/factory-onboarding.md) |
| Record a durable workaround or repository convention | [`skill-improvement.md`](skills/skill-improvement.md) |

Factory-wide policy lives in
[`projectbluefin/common`](https://github.com/projectbluefin/common). This
repository links to common policy rather than copying it; local repository
rules remain authoritative for launcher, worker, and release behavior.
