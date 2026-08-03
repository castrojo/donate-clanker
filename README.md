# donate-clanker

`donate-clanker` is a thin, foreground launcher for a self-owned
FSDK-derived contributor image. It boots a QEMU VM or runs that image directly
with Goose and Project Bluefin review context.

It owns only VM boot, credential handoff, and review context. Hive owns the
contributor protocol, task selection, the `contributor` tmux session, prompt
injection, and output capture.

## Scope

`ujust donate-clanker` is only available after this recipe has been imported
into a custom image at `/usr/share/ublue-os/just/`. That is the system-wide
path Bluefin's root Justfile loads.

For a local checkout, use `just --justfile just/61-donate-clanker.just ...` or
import that file from your own personal Justfile.

## Installing this into your own setup

For a checkout, run the recipes directly:

```bash
just --justfile just/61-donate-clanker.just --list
just --justfile just/61-donate-clanker.just donate-clanker
```

## Commands

`just/61-donate-clanker.just` is the installable artifact and exposes exactly
three public recipes:

| Command | Purpose |
|---|---|
| `ujust donate-clanker` | Run the contributor through a foreground QEMU VM. |
| `ujust donate-clanker-container` | Run the contributor container directly, without a VM. |
| `ujust donate-clanker-doctor` | Perform read-only launch diagnostics. |

Every run remains attached to its originating terminal. Ctrl-C or closing that
terminal stops it; the launcher provides no lifecycle commands or daemon.
Detaching tmux (`prefix`, then `d`) detaches the view only—the originating
terminal remains responsible for the foreground run.

## Bluefin Ops Control Panel

`mcp-app/` is an optional, read-only Goose Desktop MCP App. It observes Hive
and GitHub evidence in a single Tactical Ledger view; it does not replace the
launcher, select work, control Hive, read tmux, persist data, or poll.

Build it, then register `node mcp-app/dist/server.js` as a stdio MCP server in
Goose Desktop:

```bash
npm --prefix mcp-app install
npm --prefix mcp-app run build
```

The app fetches one evidence snapshot when opened and only fetches again when
the user activates **Refresh all evidence**. Its server uses
`DONATE_CLANKER_GH_TOKEN`, then `GH_TOKEN`, for GitHub API reads when
available; it never uses or displays a Copilot credential. See
[`mcp-app/README.md`](mcp-app/README.md) for Hive endpoint configuration and
Goose resource details.

## Requirements and credentials

- `gh auth login --web --hostname github.com --scopes repo,read:org` is a hard
  prerequisite for both launch modes.
- `podman` for either launch mode.
- Readable, writable `/dev/kvm` for VM mode.
- For a local raw VM: `qemu-system-<host-arch>`, `qemu-img`, matching UEFI
  firmware, `curl`, and `zstd`.
- Goose configured for GitHub Copilot, or `GITHUB_COPILOT_TOKEN`.
- For container-only Git operations, a separate GitHub token via
  `DONATE_CLANKER_GH_TOKEN`.

Goose is the only agent backend and GitHub Copilot is the only supported
provider. `GOOSE_PROVIDER` may be unset or `github_copilot`; `GOOSE_MODEL`
optionally overrides the `gpt-4.1` default. A `gh auth token` does not
authenticate Copilot inference.

The container recipe inherits the Copilot and GitHub tokens by environment
variable name, so token values are not placed on Podman's command line. The
agent can use every scope on its GitHub token; prefer a
`DONATE_CLANKER_GH_TOKEN` limited to `public_repo` or `repo`.

The VM guest has no GitHub identity mapping. Use
`donate-clanker-container` when fork, push, or pull-request access is needed;
a host `gh` login or PAT cannot add that identity to the VM.

Run `ujust donate-clanker-doctor` to check the selected VM path, including
local tools, firmware, raw-artifact availability, contributor image, Hive
setup, and credentials. It never starts a VM or container. A normal attended
launch runs Hive's upstream setup when `~/.config/hive/contributor.env` is
absent; doctor only reports that condition.

## Configuration

All configuration is read at launch.

| Variable | Purpose |
|---|---|
| `DONATE_CLANKER_VM_RAW` | Verified local raw disk; its `.sha256` sidecar is required. |
| `DONATE_CLANKER_VM_RUNNER_IMAGE` | Immutable QEMU runner image used when no raw disk is selected. |
| `DONATE_CLANKER_VM_VERSION` | Raw-release version used when neither VM override is set. |
| `DONATE_CLANKER_CONTRIBUTOR_IMAGE` | Contributor image; defaults to `ghcr.io/projectbluefin/donate-clanker:stable`. |
| `DONATE_CLANKER_HIVE_COMMIT` | Full Hive commit used for contributor setup. |
| `DONATE_CLANKER_GH_TOKEN` | Optional GitHub token override for container-only mode. |
| `GOOSE_PROVIDER` | Unset or `github_copilot`. |
| `GOOSE_MODEL` | Optional GitHub Copilot model override. |
| `GITHUB_COPILOT_TOKEN` | Optional Copilot credential override. |
| `TOOL` | Agent backend selector; only `goose` is accepted. |

`~/.config/donate-clanker/last-selections.env` stores launcher configuration
state such as the last Goose/provider selection between runs.

`~/.local/state/donate-clanker/` stores the pinned Hive checkout and verified
VM artifact cache. No other launcher state persists.

VM selection prefers an explicit raw disk, then a configured runner image,
then an exact version-and-architecture raw release. Raw images are checksum
verified, boot through disposable overlays, and cached by version and
architecture. Once the requested raw image is verified, older caches for that
architecture are removed.

`stable` is the default contributor-image tag and is pulled at each launch.
Use an immutable `sha-<commit>` tag or digest with
`DONATE_CLANKER_CONTRIBUTOR_IMAGE` when a reproducible image is required.

## Image and context

The image derives from the digest-pinned Project Bluefin FSDK lab runner and
layers the pinned Hive runtime at `835448c3cbef9f06d34dd3802548e1d1e16dbd2f`,
Goose, GitHub CLI, tmux, hooks, and generated organization skills.

Hive overwrites `~/.config/goose/config.yaml` at startup. The image therefore
uses `GOOSE_PATH_ROOT=/opt/bluefin/goose` for its controlled Goose
configuration, including Context7.

Organization skills are generated at image build time from
`projectbluefin/common`'s `docs/skills/index.json` into Goose's global skill
directory. Repositories may route agents to their own skill catalog, but
per-repository skills are not automatically discovered at session startup.

Git hooks at `/opt/bluefin/git-hooks` are ergonomics only; GitHub rulesets and
required checks enforce repository policy.

## Development

```bash
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --justfile just/61-donate-clanker.just --list
pre-commit run --all-files
```

See [`AGENTS.md`](AGENTS.md) for contributor boundaries and
[`docs/SKILL.md`](docs/SKILL.md) for task-specific documentation.

## License

Licensed under the [Apache License 2.0](LICENSE).
