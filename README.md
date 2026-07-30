# donate-clanker

## Run it

From the repository you want the agent to work on, run:

```bash
just --justfile just/61-donate-clanker.just donate-clanker
```

The launcher verifies GitHub, Goose, and Hive setup in order. If Hive setup is
missing, it runs the pinned upstream interactive setup. It creates or reuses
the `donate-clanker` Lima VM, then attaches its contributor container to this
terminal. Press `Ctrl-C` to stop the foreground container; the VM remains for
the next run. `donate-clanker-stop` removes a stale guest container.

On Bluefin DX, a missing `limactl` is installed with `brew install lima`.
Other hosts must install Lima themselves. A present `limactl` is always used
directly and never triggers Homebrew.

Inside the Lima guest, `ghcr.io/projectbluefin/donate-clanker` is compatibility mode: it is a
small, digest-pinned wrapper around the verified
`ghcr.io/kubestellar/hive-contributor` runtime. It maps `/config` and
`/workspace` to the upstream `/home/dev` paths, runs in the foreground as the
upstream non-root `dev` user, attaches the live Copilot interface to the
invoking terminal, and needs no host container socket. It does **not**
include donate-clanker's native Goose/RamaLama launcher or local inference
helper. Releases always publish immutable `sha-<commit>` and version tags;
the `stable` alias is published only when the repository's explicit stable
channel policy enables it.

Source of truth for the contributor workflow this wraps:
https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute
(and the `contribute-setup` / `contribute-hive` recipes in
[kubestellar/hive @ `e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e`](https://github.com/kubestellar/hive/tree/e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e)'s `Justfile`, the exact `origin/v2` commit pinned here on 2026-07-29).

## Scope

This repo covers the published compatibility image and the optional native
launcher integration. The self-contained `just` recipe is the product launch
path and keeps the compatibility container inside Lima.

## Native contributor runtime (not compatibility mode)

The optional native launcher starts this repository's `contributor` worker and
the FSDK helper separately. The worker owns the Hive connection and handles
one assignment at a time: every assignment gets exactly one fresh, isolated
`goose run --no-session` with an assignment-specific runtime directory. A
token refresh never restarts an active run. Native execution does not create,
select, inject keys into, or supervise tmux sessions, panes, or windows.

The published `ghcr.io/projectbluefin/donate-clanker` compatibility image is
different: it is only a wrapper around the pinned upstream Hive contributor
image. It does not contain the native Goose/RamaLama launcher, native
assignment isolation, or local inference helper; its interactive behavior is
owned by that upstream runtime.

The native path retains bundled Goose/RamaLama configuration under
`image/config/`. It is not configuration for the compatibility image.

### Lima guest boundary

The product container runs only through `limactl shell donate-clanker`, using
Podman installed in the named guest VM rather than a host engine or socket.
Lima receives an explicit writable workspace mount; its Hive/tool mounts rely
on Lima's default read-only behavior. The guest container gets explicit mounts for `/workspace`, `/config`
(read-only), and only its selected tool configuration (read-only).

### Context7 (optional, no credential required)

The configuration registers Context7 (`https://mcp.context7.com/mcp`) as an
optional streamable-HTTP MCP extension. Goose may call it opportunistically
when current external documentation would help a task; it is **never a
prerequisite** for starting or completing work:

- **No token, API key, or account is required** — Context7 is a public MCP
  endpoint.
- **Failed or unavailable lookups fall back silently** — the bundled
  `image/config/local-agent-policy.md` instructs the model to inspect local
  repository evidence first and to continue with that evidence if Context7
  is unreachable or returns nothing useful. Work is never blocked.
- **Offline mode is fully supported** — tasks proceed using only the
  workspace content mounted by the container.

### Profile policy and helper launch boundary

All bundled model profiles (`image/config/models.json`) declare extended
thinking disabled:

- `thinking: false` is declared in every profile's JSON entry.
- `--thinking false` is included in every profile's RamaLama runtime
  arguments as catalog policy.
- Profiles for Qwen3.5 and Qwen3.6 additionally pass
  `--chat-template-kwargs '{"enable_thinking":false}'` to the llama.cpp
  chat template layer as catalog policy.

The FSDK helper image has no versioned launch contract for this catalog's
`context_size` or `runtime_args`. Therefore native `--profile` and
`DONATE_CLANKER_PROFILE` are currently rejected during launcher option parsing,
before auth/setup or pod creation. The launcher does not translate or forward
profile values as helper flags or environment variables, and
`--profile-catalog` remains reserved until that contract exists. A direct
native Goose run still sets `GOOSE_THINKING_EFFORT=off`.

Local pre-commit hooks provide formatting and basic syntax feedback. The
`validate.yml` CI workflow is authoritative for the full static profile-policy
checks and runs without GPU hardware, Podman, or network access.

### Local task observations

After each terminal native Goose result, the contributor writes one
newline-delimited JSON record to local stderr. It contains only task identity,
kind/repository/number, timestamps, duration, and a success/failure/cancelled
outcome—never assignment text, credentials, model output, commands, or raw
errors. This is not telemetry or a transcript: the native runtime does not
persist observations or send them anywhere. Its retention ceiling is the
lifetime and retention policy of the stderr consumer outside this runtime.

## What the reference workflow actually does

Per the contribute page and the hive `Justfile`:

1. **Host-side, one-time setup** (`just contribute-setup goose` for the
   supported path): GitHub device-code auth via `gh`, registers with the
   hub over HTTPS, writes
   `~/.config/hive/{contributor.env,gh-auth.env}`. **This repo does not
   reimplement this step** — it's orchestration/auth logic the page owns,
   not container plumbing, and re-deriving it risked inventing steps the
   page doesn't describe. If `contributor.env` is missing, donate-clanker
   fetches that exact pinned upstream commit into
   `~/.local/state/donate-clanker/hive-src`, verifies `HEAD` before
   execution, and runs upstream `just contribute-setup <tool>` there. If
   you intentionally need a different immutable upstream revision, set
   `DONATE_CLANKER_HIVE_COMMIT=<40-hex sha>` first; branch names are
   rejected.
2. **Containerized run** (`just contribute-hive`): `docker`/`podman run
   --rm -it` of the published compatibility image, mounting `/config`
   read-only and `/workspace` read/write. The wrapper translates those
   portable paths to the upstream runtime's `/home/dev` paths and keeps the
   upstream process attached to the invoking terminal.

This repo runs step 2 through the named Lima guest, retains explicit
workspace/config mounts, and keeps step 1 untouched.

## Layout

```
just/61-donate-clanker.just   # the ONLY file that ships/installs — recipes:
                               #   donate-clanker, donate-clanker-doctor, donate-clanker-stop
```

Everything donate-clanker needs — Lima lifecycle, tool selection, and host
CLI detection — is embedded directly in `61-donate-clanker.just`. There is
deliberately no separate launcher script.

## Native launcher onboarding

`ujust donate-clanker` retains the attended launcher experience while running
the selected contributor only inside Lima:

1. **Tool selection detects every ready CLI.** In the fixed order `claude`,
   `copilot`, `goose`, then `codex`, zero ready tools fails with install/auth
   guidance, one is selected silently, and multiple ready tools open a
   `gum choose` picker. If prompting is unavailable, pass `TOOL=<name>`
   explicitly instead. Explicit `TOOL=` always skips auto-detection.
2. **Provider and model prompts follow the tool picker.** With a terminal and
   `gum`, Goose prompts for its provider and optional model; the other tools
   prompt for an optional `AGENT_MODEL` override. Copilot first attempts its
   live model list and falls back to text input. Environment values
   (`GOOSE_PROVIDER`, `GOOSE_MODEL`, and `AGENT_MODEL`) skip their respective
   prompts.
3. **Selections persist safely.** The next launch preselects values from
   `~/.config/donate-clanker/last-selections.env`, including intentionally
   blank choices. `~/.config/donate-clanker/secrets.env` is re-derived from
   the current run, contains only nonblank launcher-managed model settings,
   and has mode `0600`. The Lima launch passes those resolved values directly
   to its guest container; it does not mount the selections file.
4. **Hive setup remains upstream-owned.** If
   `~/.config/hive/contributor.env` is missing, the recipe fetches the pinned
   `kubestellar/hive` revision and runs its interactive
   `just contribute-setup <tool>` command. Non-interactive runs receive
   pre-seeding guidance instead of fabricated setup.

`ujust donate-clanker-doctor` is a read-only preflight that lists every CLI's
readiness and whether launch would auto-select or ask with `gum`.

## Design rationale

### Foreground-only

The recipe `exec`s `limactl shell donate-clanker -- podman run --rm -it`.
The terminal therefore owns the entire contributor lifecycle: Ctrl-C ends the
container, and `--rm` removes it. No host service, socket, or detached
process is created. `donate-clanker-stop` is only a recovery path for a
container left behind by an interrupted terminal.

### Source flexibility: `CLANKER_SRC`

The `donate-clanker` recipe resolves `CLANKER_SRC` into a single
stable host path, `~/.local/state/donate-clanker/workspace`, every time it
runs:

- **Git URL** (`https://`, `ssh://`, `git://`, or `git@...`): clone once into
  `~/.local/state/donate-clanker/clones/<repo-name>/`, `git pull --ff-only`
  on subsequent runs, then symlink `workspace` to that clone.
- **Local path**: symlink `workspace` directly to `realpath "$CLANKER_SRC"`.
- **Unset**: default to `$PWD` if it's a git repo, else reuse whatever
  `workspace` already points at from a previous run.

The recipe uses the symlink target as the VM's one writable workspace mount,
so the contributor receives no sibling state paths. It recreates the named VM
only when that target or the selected tool configuration changes.

**Adding a third source type** would follow the same pattern: extend the
`is_git_url()`/dispatch logic in the `donate-clanker` recipe with a new branch
(e.g. an OCI artifact reference), resolve it to a real host directory, and
point the same `workspace` symlink at it. No VM recreation is required.

### Tool-agnostic editor/agent layer: `TOOL=`

The launcher passes the selected backend as `AGENT_BACKEND` and adds only that
tool's read-only configuration mount to the guest container. Adding a tool
means updating the matching validation and `tool_container_mounts` case in
`just/61-donate-clanker.just`; do not mount every tool configuration at once.

## Installing this into your own setup

`just/61-donate-clanker.just` is the one file you need — everything else
in this repo is documentation or human-editable reference. This doesn't
touch `/usr` or require `sudo` at all — no image rebuild, no transient
overlay, nothing to redo after a reboot:

```sh
git clone https://github.com/projectbluefin/donate-clanker ~/.local/share/donate-clanker

# one-off, explicit file:
just --justfile ~/.local/share/donate-clanker/just/61-donate-clanker.just donate-clanker-doctor
just --justfile ~/.local/share/donate-clanker/just/61-donate-clanker.just donate-clanker

# recommended: fold it into a personal Justfile so no --justfile flag is
# ever needed again. `just` auto-discovers a `Justfile`/`.justfile` by
# walking up from the current directory, so a single import line in
# ~/Justfile (or ~/.justfile) makes it available from anywhere under $HOME:
echo 'import "'"$HOME"'/.local/share/donate-clanker/just/61-donate-clanker.just"' >> ~/Justfile
cd ~ && just donate-clanker-doctor   # or from any subdirectory of $HOME
```

Verified: with that one `import` line in `~/Justfile`, `just donate-clanker`
/ `just donate-clanker-doctor` / `just donate-clanker-stop` work unmodified
from `$HOME` or any of its subdirectories — no alias, no `--justfile` flag,
no shell function needed.

**Before running `donate-clanker` itself**: it needs a source to donate
against (see "Source flexibility: `CLANKER_SRC`" below) — either run it
from inside the git repo you want to donate cycles to, or pass
`CLANKER_SRC=<path-or-git-url>` explicitly. Running it bare from `$HOME` (or
anywhere else that isn't a git repo, on a first run with no prior
workspace) fails fast with the exact command to use — `donate-clanker-doctor`
also reports this under "Source (CLANKER_SRC)" before you hit it.

Wiring this into `ujust` itself (so plain `ujust donate-clanker` works,
system-wide, no `--justfile` flag) means getting this file under
`/usr/share/ublue-os/just/` — which on a bootc image means baking it into
a custom image build. That's out of scope here (see "Scope" above); this
repo intentionally only documents the no-rebuild, per-user path.

## Verify it locally

```sh
# Confirm the named VM exists and check its guest container:
limactl list donate-clanker
limactl shell donate-clanker -- podman ps -a --filter name=donate-clanker

# After Ctrl-C, the foreground container is removed:
limactl shell donate-clanker -- podman ps -a --filter name=donate-clanker
```

## Troubleshooting

### Receiving tasks that aren't labeled for the agent queue

If the running contributor picks up issues that don't carry the queue label
you expect (e.g. a repo's `N-clanker-queue`-style label), this is **not** a
`donate-clanker`/container problem — it's the hub's own task-selection
config, fixed entirely in the Hive dashboard, not in this repo:

1. Log into the hub's dashboard (e.g.
   `https://<your-hive-instance>.hive.kubestellar.io/`), go to
   **Config → Hub tab → "Contribute Work Queue" → Labels**.
2. The Labels filter has two independent controls: an **"Allow only
   these" / "Deny these"** mode toggle, and a separate list of label
   strings. Adding a label to the list does **not** switch the mode —
   the default mode is **"Deny these"**, so adding your queue label there
   without switching the toggle does the *opposite* of what you want (it
   skips that label and serves everything else). Click **"Allow only
   these"** so it's active/highlighted, keeping your queue label in the
   list.
3. Label matching is **exact** (or wildcard `*`), never a substring match.
   Make sure the string you saved matches the real GitHub label exactly,
   including any numeric/stage prefix your repos use (e.g.
   `3-clanker-queue`, not `clanker-queue`).
4. Save, then confirm with `ujust donate-clanker`: the "Task assigned"
   line in the container logs should now only ever show issues carrying
   that exact label.

There is no API-token path to make this change on your behalf from outside
the browser: the hosted hub sits behind an edge proxy that requires a
logged-in GitHub session for every request (bearer tokens and `?token=`
query params are rejected there before they ever reach the app), so this
one has to be done by hand in the dashboard.

## Compatibility image boundary

The wrapper maps the explicit `/config` and `/workspace` mounts to the
upstream `/home/dev` locations. It does not add a native launcher, local
inference service, or container-engine socket.
