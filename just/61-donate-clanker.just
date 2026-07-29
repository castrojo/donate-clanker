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
# TOOL defaults to auto-detect: the first installed+authenticated CLI, in
# the order defined by bin/lib-detect.sh (claude, copilot, goose).
# Usage: ujust donate-clanker
#        TOOL=copilot ujust donate-clanker
#        CLANKER_SRC=~/src/some-repo ujust donate-clanker
#        CLANKER_SRC=https://github.com/org/repo ujust donate-clanker
donate-clanker TOOL="" CLANKER_SRC="":
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
    # Bootstrap resolved TOOL (explicit or auto-detected) and recorded it —
    # read it back since auto-detection happens in that separate process.
    RESOLVED_TOOL="$(cat "${HOME}/.local/state/donate-clanker/resolved-tool" 2>/dev/null || echo "${TOOL:-unknown}")"

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
    echo "✓ donate-clanker is running and consuming your ${RESOLVED_TOOL} tokens."
    echo "  Stop any time with Ctrl-C, or: ujust donate-clanker-stop"
    echo ""
    journalctl --user -u donate-clanker.service -f --no-hostname &
    JPID=$!
    # Exit the log follower as soon as the unit itself stops (e.g. the
    # container's CLI/relay exited on its own), instead of hanging forever.
    while systemctl --user is-active --quiet donate-clanker.service; do sleep 2; done
    kill "$JPID" 2>/dev/null || true

# Preflight check: is this machine actually ready for `ujust donate-clanker`?
# Never starts the container — read-only diagnostics only.
donate-clanker-doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    source "{{donate_clanker_root}}/bin/lib-detect.sh" 2>/dev/null || {
      echo "ERROR: {{donate_clanker_root}}/bin/lib-detect.sh not found." >&2
      exit 1
    }
    pass=0; fail=0
    check() {
      local label="$1"; shift
      if "$@" &>/dev/null; then echo "  ✓ ${label}"; pass=$((pass+1));
      else echo "  ✗ ${label}"; fail=$((fail+1)); fi
    }

    echo "=== Runtime ==="
    check "podman installed" command -v podman
    check "podman rootless works (podman info)" podman info
    check "quadlet generator present" test -x /usr/lib/systemd/system-generators/podman-system-generator
    check "user systemd instance reachable" systemctl --user status
    echo ""

    echo "=== Upstream hive setup ==="
    check "gh installed" command -v gh
    check "gh authenticated" gh auth status
    check "contribute-setup completed (~/.config/hive/contributor.env)" test -f "${HOME}/.config/hive/contributor.env"
    echo ""

    echo "=== CLI backends (quadlet/tools/*.conf) ==="
    ready_tool=""
    for t in "${DONATE_CLANKER_TOOL_ORDER[@]}"; do
      if tool_installed "$t"; then
        if tool_authenticated "$t"; then
          echo "  ✓ ${t}: installed + authenticated"
          [[ -z "$ready_tool" ]] && ready_tool="$t"
        else
          echo "  ~ ${t}: installed, NOT authenticated — $(tool_fixit_hint "$t")"
        fi
      else
        echo "  ✗ ${t}: not installed — $(tool_install_hint "$t")"
      fi
    done
    echo ""

    if [[ -n "$ready_tool" ]]; then
      echo "TOOL would auto-detect to: ${ready_tool}"
    else
      echo "No CLI is both installed and authenticated yet — TOOL auto-detect would fail."
    fi
    echo ""
    echo "${pass} checks passed, ${fail} failed."
    [[ "$fail" -eq 0 ]] || exit 1


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
