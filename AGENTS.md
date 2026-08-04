# review — Agent Operating Contract

`review` is a thin, foreground launcher for a QEMU VM or contributor
container running Goose. It owns VM boot, credential handoff, and review
context. Hive owns the WebSocket contributor protocol, task selection, the
`contributor` tmux session, prompt injection, and output capture.

## Read order

1. This file.
2. [`docs/factory/agentic-model.md`](docs/factory/agentic-model.md).
3. [`docs/SKILL.md`](docs/SKILL.md).
4. The one matching file in `docs/skills/`.

## Boundaries

Keep this repository small. Do not add a daemon, service, background
lifecycle, persistent state beyond launcher configuration, task-selection
logic, or a second implementation of the launcher.

`mcp-app/` is a separate, read-only Goose Desktop presentation layer. It may
observe evidence from Hive and GitHub through its foreground stdio MCP server,
but it must never select work, control Hive, read tmux, expose credentials,
persist state, or poll.

The public launcher is foreground-only: no `&`, `nohup`, `--detach`, systemd
unit, or `podman run -d`. Ctrl-C is the stop mechanism. A hard-killed run may
leave a container name behind; the next launch reclaims it with `--replace`.

Do not filter, skip, reorder, prioritize, or decline Hive assignments. Hive is
the sole authority for task selection.

Reporting downstream evidence upstream to `kubestellar/hive` is expected work,
and filed issues are followed up rather than abandoned. Report observations,
reproductions, and options; upstream owns the design decision and the triage
labels. Never add a local workaround for an accepted upstream gap. See
[`docs/skills/upstream-hive.md`](docs/skills/upstream-hive.md).

## Repository layout

- `justfile` is the only shipped launcher artifact. Its
  three public recipes and private helpers intentionally live together.
- `mcp-app/` contains the optional Goose Desktop MCP App; it is not a launcher
  artifact and does not participate in launcher lifecycle.
- `image/` builds the FSDK-derived contributor image and its layered runtime
  configuration.
- `scripts/` contains build-time skill generation and documentation checks.
- `tests/` contains launcher and image contracts.
- `docs/` contains the skill router and catalog.

Hive rewrites `~/.config/goose/config.yaml`. Keep the controlled Goose
configuration under `GOOSE_PATH_ROOT=/opt/bluefin/goose`; do not write it to
the Hive-managed path.

## Permitted changes

Agents may change `justfile`, `mcp-app/`, `image/`,
`tests/`, `docs/`, `README.md`, `AGENTS.md`, and `.github/workflows/`.

Do not modify `ublue-os/*`, or commit generated `.agents/skills/` content.
The generator is the artifact; `projectbluefin/common`'s
`docs/skills/index.json` is the organization-skill source.

When behavior changes, update the matching user documentation. Treat the
launcher, image, and tests as the sources of truth for this repository's
behavior.

## Documentation Is the Model

[`docs/factory/agentic-model.md`](docs/factory/agentic-model.md) is the
canonical local model for the Bluefin Agentic Factory Feedback Loop. It defines
the roles, authority boundaries, and vocabulary that explain this repository.
Code, tests, user documentation, and skills must agree with it.

Every session ships the requested work and records any durable, source-backed
learning in the closest matching document under `docs/skills/`. Update
`docs/skills/index.json` in the same change. Do not commit changelogs, session
notes, implementation plans, design scratchpads, or "append here" documents.
Remove stale records of that kind and route durable guidance to the matching
skill instead.

Local repository contracts take precedence; use `projectbluefin/common` as the
pinned shared factory sidecar, never as a reason to override this repository's
boundaries.

## Validation

```bash
bash scripts/check-skill-frontmatter.sh
bash tests/generate-skills.sh
bash tests/image-contract.sh
bash tests/mcp-app-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --list
pre-commit run --all-files
```

## References

- Hive protocol, contributor runtime, and upstream issue reporting:
  `kubestellar/hive` (default branch `v2`; no contributing guide or issue
  templates, DCO sign-off required on pull requests).
- Organization skills and factory rules: `projectbluefin/common`.
- External API details: Context7 documentation.
