# donate-clanker

`ujust donate-clanker` — run the [KubeStellar Hive](https://github.com/kubestellar/hive)
contributor workload (donate CLI/API tokens to the Bluefin agent swarm) as a
proper rootless Podman Quadlet, in the foreground, with nothing left running
when you're not watching it.

Source of truth for the contributor workflow this wraps:
https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute
(and the `contribute-setup` / `contribute-hive` recipes in
[kubestellar/hive](https://github.com/kubestellar/hive)'s `Justfile`, branch `v2`).

## Scope

This repo covers running it on your machine right now: a single
self-contained `just` recipe file that installs the quadlet unit into
`~/.config/containers/systemd/`. Packaging this into a Bluefin image build
(CI, `sync-templates`-style propagation, etc.) is future work and
deliberately out of scope here.

## What the reference workflow actually does

Per the contribute page and the hive `Justfile`:

1. **Host-side, one-time setup** (`just contribute-setup <cli>`): GitHub
   device-code auth via `gh`, registers with the hub over HTTPS, writes
   `~/.config/hive/{contributor.env,gh-auth.env}`. **This repo does not
   reimplement this step** — it's orchestration/auth logic the page owns,
   not container plumbing, and re-deriving it risked inventing steps the
   page doesn't describe. Run it once from a clone of `kubestellar/hive`
   before using `ujust donate-clanker`.
2. **Containerized run** (`just contribute-hive`): `docker`/`podman run -d
   --rm` of `ghcr.io/kubestellar/hive-contributor:latest`, mounting the
   config above (read-only) plus CLI-specific credential dirs (e.g.
   `~/.claude`, `~/.copilot`), then blocking the invoking terminal on
   `podman logs -f` until Ctrl-C.

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

A fresh machine with none of this configured hits three gaps in order —
`ujust donate-clanker` now handles all three instead of failing deep inside
a container log:

1. **No CLI installed/authenticated at all — or too many.** `TOOL` is no
   longer a silent `claude` default. The Justfile's embedded detection
   logic checks every supported CLI (`claude`, `copilot`, `goose`, `codex`):
   - **Zero** installed-and-authenticated → fails fast on the host with an
     install/auth hint for each, before touching podman at all.
   - **Exactly one** ready → auto-picked silently, no prompt.
   - **More than one** ready → since this is already an attended,
     foreground-only workflow, it prompts interactively with `gum choose`
     instead of guessing. If `gum` isn't installed or there's no terminal
     attached, it fails fast and asks for an explicit `TOOL=<name>`.

   Override explicitly with `TOOL=<name>` at any time to skip detection
   entirely; an explicit `TOOL=` that isn't installed or isn't
   authenticated also fails fast with the same actionable one-liner.

   Once `TOOL` is resolved, an optional model (and, for `goose`, provider)
   prompt follows the same rule: only asked when not already given via
   `AGENT_MODEL=`/`GOOSE_PROVIDER=`/`GOOSE_MODEL=` env vars and `gum` +
   a terminal are available; leaving it blank falls back to the tool's own
   default. The choice is written to `~/.config/donate-clanker/secrets.env`
   (`chmod 600`) and re-derived fresh on every run rather than accumulating.
2. **Upstream `contribute-setup` never run.** If
   `~/.config/hive/contributor.env` doesn't exist, the recipe
   clones `kubestellar/hive` (branch `v2`) into
   `~/.local/state/donate-clanker/hive-src` and runs **its own**
   `just contribute-setup <tool>` — we still never reimplement the
   registration/GitHub-auth logic ourselves, we just make sure it has run.
3. **"Is my machine even ready?"** `ujust donate-clanker-doctor` is a
   read-only preflight: rootless podman, the quadlet generator, `gh` auth,
   `gum` (needed only when multiple CLIs are ready), whether upstream setup
   has completed, and which CLIs are installed/authenticated — including
   whether `TOOL` would auto-pick one or prompt because several qualify. It
   never starts the container.

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

**Adding another tool**: drop a new `quadlet/tools/<name>.conf` with an
`[Container]` section (`Environment=AGENT_BACKEND=<name>` plus whatever
`Volume=` lines that CLI needs), then run `TOOL=<name> ujust
donate-clanker`. No other file changes needed.

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

## Assumptions flagged for correction

- Defaulting the quadlet's `Network=` to the sandboxed rootless network
  instead of upstream's `--network host` (commented alternative left in the
  unit file) — the Justfile doesn't explain why host networking is needed
  for an outbound-only WebSocket relay.
- The `CLANKER_SRC`/workspace mount is a convenience addition with no
  upstream equivalent; the hive-contributor image's actual handling of a
  pre-seeded `/home/dev/workspace` is unverified.
- Local iteration on `/usr/share/ublue-os/just/` assumes `rpm-ostree
  usroverlay` is the right tool for testing before this ships in an image
  build — flag if Bluefin's actual workflow differs.
