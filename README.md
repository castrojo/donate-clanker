# donate-clanker

Donate a terminal to the Bluefin factory: a thin, foreground launcher that
boots a self-contained QEMU VM running a Project Bluefin FSDK-derived
contributor image with Goose, preloaded with Bluefin review context.

## What it does

`donate-clanker` is duct tape in the `brew`/`just` tradition, not a platform.
It owns exactly three things:

1. Booting a pinned QEMU VM in the foreground.
2. Passing your own agent credentials through to Goose inside the guest.
3. Assembling the review context payload (org skills, Context7, git hooks).

Everything else belongs to Hive. Hive owns the WebSocket contributor
protocol, task selection, the `contributor` tmux session, prompt injection,
and output capture. donate-clanker deliberately duplicates none of it.

The result feels like a dedicated terminal that runs review tasks handed to
it by Hive, and that a human can attach to and steer at any time.

## Requirements

- `podman` — runs the VM runner container and the container-only mode.
- `/dev/kvm` readable and writable by your user — the VM runner requires
  hardware virtualization.
- `gh` authenticated against `github.com` (`gh auth login --web --hostname
  github.com --scopes repo,read:org`).
- A credential for a Goose-supported model provider. Goose is the only agent
  backend; supply `GOOSE_PROVIDER` and, optionally, `GOOSE_MODEL`.
- For the default GitHub Copilot provider, a Copilot login on this host
  (`goose configure`, then complete the device flow) or an exported
  `GITHUB_COPILOT_TOKEN`. Both launch paths hand that credential to the agent
  so it never stalls on a device code. A `gh auth token` is *not* a
  substitute — Copilot inference rejects it.

Check all of the above with `ujust donate-clanker-doctor` before filing a
bug.

> **The agent acts as you on GitHub.** Hive's contributor model is fork +
> pull request under the contributor's own identity, so
> `ujust donate-clanker-container` passes your `gh auth token` to the agent as
> `GH_TOKEN`. The agent inherits every scope that token holds — a desktop `gh`
> login often carries `admin:org`, `workflow` and `delete:packages`. The
> launcher prints the scopes (never the value) before handing them over. To
> narrow that, export `DONATE_CLANKER_GH_TOKEN` with a PAT scoped to
> `public_repo` or `repo`, which is enough to fork, push and open a pull
> request.

## Installation

`just/61-donate-clanker.just` is the only file that ships or installs.

Bluefin's root `Justfile` (`/usr/share/ublue-os/just/00-entry.just`) imports
a fixed list of files, not a glob. Installing this file system-wide so that
plain `ujust donate-clanker` works therefore requires baking it into a custom
image build, which is out of scope for this repository.

For local use, run the file directly:

```bash
just --justfile just/61-donate-clanker.just --list
just --justfile just/61-donate-clanker.just donate-clanker
```

Or import it from your own personal `Justfile` so the recipes appear
alongside your own:

```just
import "/absolute/path/to/donate-clanker/just/61-donate-clanker.just"
```

## Usage

| Command | What it does |
|---|---|
| `ujust donate-clanker` | Boot the pinned QEMU VM in the **foreground** and attach to the `contributor` tmux session. Ctrl-C stops it. |
| `ujust donate-clanker-container` | Run only the container, no VM, and attach to the same tmux session. For quick local development. |
| `ujust donate-clanker-doctor` | Read-only preflight diagnostics. Never starts anything. |

The launcher never backgrounds itself. If your terminal is gone, the run is
gone. That is the foreground guarantee, and it is intentional: there is no
daemon, no unit, and no orphaned state to reap.

**There is no stop command, and there will not be one.** Ctrl-C is the stop
button. A `ujust donate-clanker-stop` would only make sense if a run could
outlive its terminal, which is precisely what this launcher refuses to allow —
shipping one would advertise a daemon we do not have. If a previous run was
killed hard enough to leave its container name behind, the next launch
reclaims that name itself (`--replace`); cleanup is a startup concern, never
something you have to remember to do.

Attaching by hand uses Hive's own documented flow:

```bash
podman exec -it <container> tmux attach -t contributor
# docker exec -it <container> tmux attach -t contributor
```

## Reviewing

From a shell in the assigned repository, run `bluefin-review [range]`. It is
a compact header over Goose's native `goose review` command; it is not a Goose
slash command. To inspect earlier output, enter tmux copy-mode with `Ctrl-b [`;
PageUp scrolls, tmux search finds text, and `q` returns to the live pane.
Copy-mode only changes your view; Hive still owns task and output handling.

## Configuration

All configuration is environment variables read at launch.

The attended contributor runtime uses Goose `auto` tool mode. This intentionally
avoids keystroke-injected permission prompts, but allows all tools available to
the contributor's scoped credentials. Use a restricted `DONATE_CLANKER_GH_TOKEN`
when that scope is too broad.

| Variable | Purpose |
|---|---|
| `DONATE_CLANKER_VM_RUNNER_IMAGE` | Signed, immutable VM runner image reference. Required for VM mode. |
| `DONATE_CLANKER_CONTRIBUTOR_IMAGE` | Contributor image to run. Defaults to `ghcr.io/projectbluefin/donate-clanker:stable`. See [Published image tags](#published-image-tags). |
| `DONATE_CLANKER_HIVE_COMMIT` | Override the pinned Hive commit. Defaults to `e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e`. |
| `GOOSE_PROVIDER` | Goose model provider. Passed through to the guest. |
| `GOOSE_MODEL` | Goose model name. Optional; passed through to the guest. |
| `TOOL` | Agent backend selector. Goose is the only supported value. |

The launcher keeps a small amount of state under its config directory:
`last-selections.env` remembers the previous run's selections, and
`secrets.env` (mode `0600`) holds provider and model values so you are not
re-prompted every launch. Neither file is mounted into the guest as a home
or workspace directory; the VM runner receives only its per-run
control/overlay directory.

## Published image tags

`.github/workflows/publish-compat-image.yml` publishes the contributor image
to `ghcr.io/projectbluefin/donate-clanker`:

| Tag | Moves? | Published by | Architectures |
|---|---|---|---|
| `stable` | Yes | every push to `main` and `v*.*.*` tag push | `linux/amd64`, `linux/arm64` |
| `X.Y.Z` and `vX.Y.Z` | No | a `v*.*.*` tag push | `linux/amd64`, `linux/arm64` |
| `sha-<commit>` | No | every build | `linux/amd64`, `linux/arm64` on `main` and release builds; `linux/amd64` on manual dispatches |

There is no `:latest`, and there never will be — pointing at it fails with
`manifest unknown`.

`stable` moves on every merge to `main` and is the launcher's default. A
release tag also updates it alongside immutable version tags. To run a
reproducible build, override the image with its immutable `sha-<commit>` tag:

```bash
DONATE_CLANKER_CONTRIBUTOR_IMAGE=ghcr.io/projectbluefin/donate-clanker:sha-<commit> \
  ujust donate-clanker-container
```

For a reproducible run, pin `sha-<commit>` or an `@sha256:` digest instead of
a moving tag.

## Review context

The image derives `FROM ghcr.io/projectbluefin/lab-runner`, pinned by digest,
then layers in the minimal Hive contributor runtime, Goose, and the Bluefin
review context.

**Goose config survival.** Hive unconditionally overwrites
`~/.config/goose/config.yaml` at every startup. donate-clanker therefore sets
`GOOSE_PATH_ROOT` to a controlled config root at `/opt/bluefin/goose`, so our
Goose configuration — which declares the Context7 MCP extension — survives
Hive's rewrite.

**Org skills.** Goose v1.45 has native Agent Skills: a skill is a *directory*
containing a `SKILL.md` with YAML frontmatter (`name`, `description`). Only
the name and description enter the system prompt; the body loads on demand
through the `load_skill` tool, or deterministically when a human types
`/skill-name`. Goose discovers global skills under `~/.agents/skills/`.

Org skills are generated **at image build time** from
`projectbluefin/common`'s `docs/skills/index.json` into
`/home/dev/.agents/skills/<id>/SKILL.md`. The org keeps `docs/skills/*.md`
plus `index.json` as the source of truth; nothing in other repositories has
to change.

**Per-repo skills.** These cannot be discovered natively. Goose starts in
`/home/dev` before any repository is cloned, and it discovers skills at
session start. Instead, the agent is instructed to read the cloned repo's
`docs/skills/index.json` and open only the matching `entry_point`. This is
model-driven, not guaranteed. Treat it as best effort.

**Git hooks.** Hooks ship at `/opt/bluefin/git-hooks` and are wired through a
global `core.hooksPath`. They are ergonomics only: `git commit --no-verify`
bypasses them entirely. Deterministic enforcement is GitHub rulesets and
required status checks, not hooks.

## Architecture

```
  you                       host                      guest VM
  ---                       ----                      --------
  ujust donate-clanker
        |
        v
  61-donate-clanker.just  --> podman run (VM runner)
                                    |
                                    v
                              qemu-system-* -nographic
                                    |
                                    v
                          fsdk-derived contributor container
                                    |
                                    v
                          tmux session "contributor"
                            + goose (agent backend)
                                    ^
                                    |
                          Hive WebSocket: task in, 15 lines out
```

Foreground the whole way down: your terminal is the process tree's root.

## Scope and non-goals

donate-clanker does **not**:

- Manage Hive sessions. Hive selects tasks, injects prompts, and captures
  output.
- Run local inference. Bring your own model credential.
- Implement the Hive contributor protocol. It boots an image that already
  speaks it.
- Aspire to be a platform, a service, or a daemon.

Hive behaviors you should know about because they will surprise you:
completion is self-reported and applies a 168-hour cooldown to that issue,
while a reported failure applies none at all; and the scoped GitHub token Hive
issues expires 55 minutes after assignment and is never refreshed.

## Development

```bash
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --justfile just/61-donate-clanker.just --list
pre-commit run --all-files
```

Agent-facing rules live in [`AGENTS.md`](AGENTS.md). Task-scoped
documentation is routed from [`docs/SKILL.md`](docs/SKILL.md).

## Testing

`tests/image-contract.sh` asserts the image's Containerfile, Goose config,
entrypoint and git hooks still hold their contract. `tests/just-onboarding.sh`
exercises the launcher's onboarding behavior against a mocked host.
`tests/generate-skills.sh` ensures a skills manifest cannot escape its output
root. `pre-commit run --all-files` runs the YAML/JSON/shell hygiene hooks and
the generator regression test. `git diff --check` must be clean.

## Contributing

- One logical change per pull request.
- Pull request titles must follow Conventional Commits; merges are squash-only
  and the squash commit inherits the PR title.
- Update the matching `docs/skills/*.md` file in the same pull request when
  behavior changes.

## License

See the repository's license file.
