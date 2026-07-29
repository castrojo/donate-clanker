# 61-donate-clanker.just — ships to /usr/share/ublue-os/just/61-donate-clanker.just
#
# Bluefin's root Justfile already does:
#   import? '/usr/share/ublue-os/just/*.just'
# so dropping this file there is enough to expose `ujust donate-clanker` —
# no separate import line to add.
#
# ── /usr-immutable note ──
# This repo isn't packaged into the image build yet (that's later work, out
# of scope here — see README "Scope"). To test *right now* on a running
# bootc system without an image rebuild:
#   sudo mkdir -p /usr/share/ublue-os/just
#   sudo cp just/61-donate-clanker.just /usr/share/ublue-os/just/
#   sudo cp -r bin quadlet /usr/share/ublue-os/donate-clanker/
# ASSUMPTION: writing under /usr requires `rpm-ostree usroverlay` (a
# transient, reboot-cleared overlay) since /usr is read-only on Bluefin.
# That's the conventional way to iterate on ublue-os just recipes locally
# before they're baked into an image; flag if your setup differs.
#
# All state this recipe touches lives under $HOME (quadlets, config,
# workspace) — nothing here needs root or writes elsewhere under /usr.

donate_clanker_root := "/usr/share/ublue-os/donate-clanker"

# Start donating clanker cycles to the hive — foreground, Ctrl-C to stop.
# Usage: ujust donate-clanker
#        TOOL=copilot ujust donate-clanker
#        CLANKER_SRC=~/src/some-repo ujust donate-clanker
#        CLANKER_SRC=https://github.com/org/repo ujust donate-clanker
donate-clanker TOOL="claude" CLANKER_SRC="":
    #!/usr/bin/env bash
    set -euo pipefail
    export TOOL="{{TOOL}}"
    export CLANKER_SRC="{{CLANKER_SRC}}"

    BOOTSTRAP="{{donate_clanker_root}}/bin/donate-clanker-bootstrap.sh"
    if [[ ! -x "$BOOTSTRAP" ]]; then
      echo "ERROR: ${BOOTSTRAP} not found. See just/61-donate-clanker.just for local install steps." >&2
      exit 1
    fi
    "$BOOTSTRAP"

    if ! systemctl --user cat donate-clanker.container &>/dev/null; then
      echo "ERROR: quadlet did not generate a unit. Run:" >&2
      echo "  /usr/lib/systemd/system-generators/podman-system-generator --user --dryrun" >&2
      exit 1
    fi

    # Foreground-equivalent: start the unit, follow its logs in *this*
    # terminal, and always stop it again on exit — Ctrl-C, normal exit, or
    # an unexpected kill of this script all hit the same trap.
    cleanup() {
      echo ""
      echo "Stopping donate-clanker..."
      systemctl --user stop donate-clanker.service >/dev/null 2>&1 || true
    }
    trap cleanup EXIT INT TERM

    systemctl --user start donate-clanker.service
    echo "✓ donate-clanker is running and consuming your ${TOOL} tokens."
    echo "  Stop any time with Ctrl-C, or: ujust donate-clanker-stop"
    echo ""
    journalctl --user -u donate-clanker.service -f --no-hostname &
    JPID=$!
    # Exit the log follower as soon as the unit itself stops (e.g. the
    # container's CLI/relay exited on its own), instead of hanging forever.
    while systemctl --user is-active --quiet donate-clanker.service; do sleep 2; done
    kill "$JPID" 2>/dev/null || true

# Safety net: stop and fully clean up donate-clanker, whether or not it's
# currently tracked as running. Recovers from a closed terminal / crashed
# shell that left the unit or container behind.
donate-clanker-stop:
    #!/usr/bin/env bash
    set -euo pipefail
    systemctl --user stop donate-clanker.service >/dev/null 2>&1 || true
    systemctl --user reset-failed donate-clanker.service >/dev/null 2>&1 || true
    # Belt-and-suspenders: quadlet-managed containers are named deterministically,
    # so a stray container (e.g. after a systemd crash) can still be found and removed.
    podman rm -f donate-clanker >/dev/null 2>&1 || true
    echo "donate-clanker stopped and cleaned up."
    echo "Current state:"
    systemctl --user is-active donate-clanker.service 2>&1 || true
    podman ps -a --filter name=donate-clanker --format '  {{{{.Names}}}}\t{{{{.Status}}}}'
