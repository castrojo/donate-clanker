# donate-clanker

## Run it

Complete the one-time GitHub/Hive setup from a clean
[KubeStellar Hive](https://github.com/kubestellar/hive) checkout first. It
creates the local credentials listed below. Then, from the repository you want
the agent to work on, run this single command:

```bash
podman run --pull=always --rm -it --userns=keep-id -v "$HOME/.config/hive:/config:ro,Z" -v "$PWD:/workspace:Z" ghcr.io/projectbluefin/donate-clanker:stable
```

The command expects these files from the setup step:

- `$HOME/.config/hive/contributor.env`
- `$HOME/.config/hive/gh-auth.env`

The image remains attached to the terminal; press `Ctrl-C` to stop it.

`ghcr.io/projectbluefin/donate-clanker` is compatibility mode: it is a
small, digest-pinned wrapper around the verified
`ghcr.io/kubestellar/hive-contributor` runtime. It maps `/config` and
`/workspace` to the upstream `/home/dev` paths, runs in the foreground as the
upstream non-root `dev` user, and needs no host container socket. It does **not**
include donate-clanker's native Goose/RamaLama launcher or local inference
helper. Releases always publish immutable `sha-<commit>` and version tags;
the `stable` alias is published only when the repository's explicit stable
channel policy enables it.

Source of truth for the contributor workflow this wraps:
https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute
(and the `contribute-setup` / `contribute-hive` recipes in
[kubestellar/hive @ `e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e`](https://github.com/kubestellar/hive/tree/e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e)'s `Justfile`, the exact `origin/v2` commit pinned here on 2026-07-29).

## Scope

This repo covers running it on your machine right now: a single
self-contained `just` recipe file that installs the quadlet unit into
`~/.config/containers/systemd/`. Packaging this into a Bluefin image build
(CI, `sync-templates`-style propagation, etc.) is future work and
deliberately out of scope here.

## Native launcher configuration (not compatibility mode)

The repository retains bundled Goose/RamaLama configuration under
`image/config/` for native launcher integrations. The published
`ghcr.io/projectbluefin/donate-clanker` compatibility image does not copy or
use these files; it delegates to the upstream Hive contributor runtime.

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

### Extended thinking disabled by default

All bundled model profiles (`image/config/models.json`) run with extended
thinking explicitly disabled:

- `thinking: false` is declared in every profile's JSON entry.
- `--thinking false` is included in every profile's RamaLama runtime
  arguments, disabling reasoning at the inference-server level.
- Goose also receives `GOOSE_THINKING_EFFORT=off` as a secondary guard.
- Profiles for Qwen3.5 and Qwen3.6 additionally pass
  `--chat-template-kwargs '{"enable_thinking":false}'` to the llama.cpp
  chat template layer.

These defaults are verified by CI static checks (`validate.yml`) that run
without GPU hardware, Podman, or network access.

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

This repo replaces step 2's ad-hoc `podman run` with a quadlet-managed
systemd user unit, adds a tool-swap mechanism and workspace source
flexibility on top, and keeps step 1 untouched.

## Layout

```
just/61-donate-clanker.just   # the ONLY file that ships/installs — recipes:
                               #   donate-clanker, donate-clanker-doctor, donate-clanker-stop
quadlet/*.conf                # human-editable *source* for the strings embedded
                               # in the Justfile above; not read at runtime (see
                               # the note at the top of 61-donate-clanker.just)
```

Everything donate-clanker needs — the quadlet unit content, per-tool
fragments, and host CLI detection — is embedded directly in
`61-donate-clanker.just` as private (`_`-prefixed style, via `[private]`-like
convention) variables and inline bash, generated to disk on each run. There
is deliberately no separate `bin/` of standalone scripts: a user browsing
the image or this repo only ever finds one file and the three commands it
exposes, not scripts they might run out of context.

## Onboarding

The supported production onboarding path is **GitHub + Goose/local
inference**. `ujust donate-clanker` now fails fast on the host with
actionable, secret-safe checks instead of relying on filesystem presence or
deep container logs:

1. **GitHub auth is authoritative.** Auto-detect only covers `TOOL=goose`,
   and Goose is considered ready only when `gh auth status --hostname
   github.com` succeeds. A `~/.config/gh` directory alone is **never**
   treated as proof of authentication. If auth is missing or invalid, fix it
   with:

   ```sh
   gh auth login --web --hostname github.com --scopes repo,read:org
   ```

2. **Goose local inference must be configured semantically.** The launcher
   validates `~/.config/goose/config.yaml` for the supported path — not just
   the existence of `~/.config/goose/`. The file must contain non-empty
   top-level `provider`, `base_url`, `api_key`, and `model` keys, and the
   supported production path requires `provider: openai` so Goose talks to a
   local OpenAI-compatible endpoint. `goose configure` is the expected way to
   write that file. Optional `GOOSE_PROVIDER=` / `GOOSE_MODEL=` overrides are
   still honored for a run, but they do not replace the base local config
   check.

3. **Hive setup is reused, then validated.** If
   `~/.config/hive/contributor.env` is missing, the recipe fetches
   `kubestellar/hive` into `~/.local/state/donate-clanker/hive-src` at the
   exact pinned commit
   `e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e`, verifies
   `git rev-parse HEAD` matches before execution, and then runs upstream
   `just contribute-setup goose` with your current terminal attached so the
   upstream prompts can read stdin normally. `DONATE_CLANKER_HIVE_COMMIT`
   may override that pin, but it must still be a full 40-character commit
   SHA — mutable refs like `v2` are rejected. If you set
   `--non-interactive` or `DONATE_CLANKER_NON_INTERACTIVE=true`, or if
   stdio is not attached to a terminal, donate-clanker refuses to fake the
   prompts and instead tells you to pre-seed
   `~/.config/hive/contributor.env` yourself from that exact pinned
   checkout. If the file already exists,
   donate-clanker validates it semantically before starting:
   `HIVE_REGISTRATION_TOKEN`, `HIVE_HUB`, `CONTRIBUTOR_ID`,
   `CONTRIBUTOR_USERNAME`, and `AGENT_BACKEND=goose` must all be present,
   and `HIVE_HUB` must be a `wss://` URL ending in `/contribute`. Errors
   name the missing or invalid key, never the secret value.

4. **Doctor mirrors the supported checks.** `ujust donate-clanker-doctor` is
   a read-only preflight for rootless podman, the quadlet generator, GitHub
   auth, Goose local config, and validated upstream Hive setup. It also shows
   that only Goose participates in supported auto-detect; legacy backends
   (`claude`, `copilot`, `codex`) remain manual compatibility paths via
   explicit `TOOL=<name>` and are not part of the production-gated workflow.

Once `TOOL` is resolved, an optional model (and, for `goose`, provider)
prompt follows the same rule as before: only asked when not already given via
`AGENT_MODEL=`/`GOOSE_PROVIDER=`/`GOOSE_MODEL=` env vars and `gum` + a
terminal are available; leaving it blank falls back to the tool's own
default. Every pick (TOOL, model, goose provider/model — including a
deliberate blank) is remembered in
`~/.config/donate-clanker/last-selections.env` and offered back as the
pre-selected default the next time you run it. The final resolved values are
written to `~/.config/donate-clanker/secrets.env` (`chmod 600`) and
re-derived fresh on every run rather than accumulating.

## Design rationale

### Foreground-only, never a lingering background service

The `[Install]` section a normal quadlet would have (`WantedBy=default.target`,
to auto-start at boot/login) is **deliberately omitted** from
`donate-clanker.container` — see the comment at the bottom of that file.
Without it, `systemctl --user enable` has nothing to attach to a target, and
nothing ever starts this unit except an explicit `systemctl --user start`
from the `ujust donate-clanker` recipe.

The recipe itself is the foreground layer: it starts the unit, `trap`s
`EXIT`/`INT`/`TERM` to stop it again, and follows `journalctl --user -u
donate-clanker.service -f` in the invoking terminal so it's visually obvious
the workload is live and burning tokens for as long as that terminal is
open. Ctrl-C, closing the terminal, or the script exiting for any other
reason all run the same `systemctl --user stop`.

**If the terminal is killed out-of-band** (e.g. `kill -9` on the shell, a
crashed terminal emulator) the `trap` never fires, and the unit — with no
linger enabled — keeps running only until the user's systemd instance itself
is torn down (normally at logout), not indefinitely. `ujust
donate-clanker-stop` is the recovery path: it stops the service, resets any
failed state, and force-removes the container by its deterministic name
(`donate-clanker`) even if systemd's bookkeeping got confused, so you're
never stuck without a way back to a clean state.

`loginctl enable-linger` is intentionally **not** configured for this
workload. Linger keeps a user's systemd instance (and anything in it)
running across logout — exactly what would let this unit survive an
unattended session. We rely on the *lack* of linger as an extra backstop:
even in the out-of-band-kill scenario above, logging out fully tears the
unit down.

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

The quadlet's `Volume=` line never changes — it always mounts
`~/.local/state/donate-clanker/workspace`. Podman resolves host-side
symlinks at mount time, so swapping the symlink target is enough to change
what the container sees on every run without touching the unit file.

**Adding a third source type** would follow the same pattern: extend the
`is_git_url()`/dispatch logic in the `donate-clanker` recipe with a new branch
(e.g. an OCI artifact reference), resolve it to a real host directory, and
point the same `workspace` symlink at it. No quadlet or `just` recipe
changes required.

### Tool-agnostic editor/agent layer: `TOOL=`

Per-tool credentials/env are never hardcoded in the base unit. Each
supported tool has its own quadlet drop-in fragment in `quadlet/tools/`
(`claude.conf`, `copilot.conf`, `goose.conf`, `codex.conf` — extend with more as needed, matching the
`CLI_MOUNTS` cases in the hive `Justfile`). The recipe copies
*exactly one* of these into
`~/.config/containers/systemd/donate-clanker.container.d/10-tool.conf`,
deleting any previous selection first — this avoids the standard
systemd-drop-in gotcha where multiple `*.conf` files in the same `.d/`
directory would all merge together and mount every tool's credentials at
once.

**Adding another tool**: the `.conf` files are the human-editable
*reference* for what to add — they aren't read at runtime (see "Layout"
above). To actually add a tool, edit `just/61-donate-clanker.just`'s
`shared_functions` variable: add a case arm for the new tool name in
`tool_installed`/`tool_authenticated`/`tool_fixit_hint`/`tool_install_hint`
and in `tool_fragment_conf` (the `[Container]` fragment — usually just
`Environment=AGENT_BACKEND=<name>` plus whatever `Volume=` lines that CLI
needs). Only add the name to `tool_order` if it becomes part of the
supported production auto-detect path; otherwise keep it manual-only behind
explicit `TOOL=<name>`. Then copy the same fragment text into a new
`quadlet/tools/<name>.conf` for reference. Run `TOOL=<name> ujust
donate-clanker` to try it.

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
# 1. Confirm the quadlet generator understood the unit + drop-in:
/usr/lib/systemd/system-generators/podman-system-generator --user --dryrun

# 2. After starting it via `ujust donate-clanker` (in another terminal):
systemctl --user daemon-reload
systemctl --user status donate-clanker.service
podman ps --filter name=donate-clanker
journalctl --user -u donate-clanker.service -n 100 --no-pager

# 3. Confirm it's really gone after Ctrl-C or `ujust donate-clanker-stop`:
systemctl --user is-active donate-clanker.service   # should print "inactive"/"failed", never "active"
podman ps -a --filter name=donate-clanker           # should show no running container
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

The wrapper deliberately keeps the rootless network default and maps the
portable `/config` and `/workspace` mounts to the upstream `/home/dev`
locations. It does not add a native launcher, local inference service, or
container-engine socket.
