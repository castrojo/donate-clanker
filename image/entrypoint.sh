#!/usr/bin/env bash
# review container entrypoint.
#
# This wraps Hive's contributor runtime instead of replacing it. Hive owns the
# contributor WebSocket protocol, task selection, the tmux session, prompt
# injection and output capture; everything below is context setup that happens
# before handing control over.
set -euo pipefail

note() { printf 'review: %s\n' "$1" >&2; }

# Validate the caller's requested provider before any other startup work so an
# unsupported setting always gets the same actionable answer.
if [ -n "${GOOSE_PROVIDER:-}" ] && [ "$GOOSE_PROVIDER" != github_copilot ]; then
	note "ERROR: GOOSE_PROVIDER=${GOOSE_PROVIDER} is not supported — review supports GitHub Copilot only."
	note "  Unset GOOSE_PROVIDER or set GOOSE_PROVIDER=github_copilot."
	exit 1
fi

hive_config="${HOME}/.config/hive"
if [ ! -f "${hive_config}/contributor.env" ]; then
	note "missing ${hive_config}/contributor.env"
	note "  mount your Hive config, or run: just contribute-setup goose"
	exit 1
fi

note 'Bluefin Operations | contributor runtime starting'

# --- Goose configuration -----------------------------------------------------
#
# Hive's contributor-agent.sh unconditionally overwrites
# ~/.config/goose/config.yaml at every startup, so our configuration cannot live
# there. GOOSE_PATH_ROOT relocates Goose's config/data/state roots, which lets
# the controlled config survive untouched.
export GOOSE_PATH_ROOT="${REVIEW_GOOSE_ROOT:-/opt/bluefin/goose}"

# Goose resolves environment before file, so the launcher's passthrough wins
# over anything in the controlled config. This image is intentionally
# Copilot-only; accepting another value would leave Hive to start a provider
# whose credential path this launcher does not support.
export GOOSE_PROVIDER=github_copilot

# Goose refuses to start without a model. Keep the direct-image fallback in
# sync with the launcher's default for users who invoke this image directly.
if [ -z "${GOOSE_MODEL:-}" ]; then
	GOOSE_MODEL="gpt-4.1"
	note "GOOSE_MODEL not set; defaulting to ${GOOSE_MODEL} for GitHub Copilot"
fi
export GOOSE_MODEL

# No desktop keyring exists in a container; without this Goose fails to store or
# read provider secrets and falls back inconsistently.
export GOOSE_DISABLE_KEYRING=1

# Goose asks an interactive telemetry question on first run. Hive drives the
# CLI with simulated keystrokes, so an unanswered prompt hangs the agent.
export GOOSE_TELEMETRY_ENABLED="${GOOSE_TELEMETRY_ENABLED:-false}"

# Hive refreshes an org knowledge export every ten minutes and symlinks it to
# ~/CLAUDE.md and ~/.goose-instructions.md. Goose reads neither by default -- its
# context filenames are AGENTS.md and .goosehints -- so the export never reaches
# the model. Naming CLAUDE.md here makes that existing mechanism work.
export CONTEXT_FILE_NAMES="${CONTEXT_FILE_NAMES:-[\"AGENTS.md\",\".goosehints\",\"CLAUDE.md\"]}"

# Native skills advertise their descriptions at session start, but their bodies
# load on demand. Keep this small policy in every turn so the agent routes into
# the global inventory and each cloned repository's own skill catalog.
export GOOSE_MOIM_MESSAGE_FILE="${GOOSE_MOIM_MESSAGE_FILE:-/opt/bluefin/local-agent-policy.md}"

# --- Git hooks ---------------------------------------------------------------
#
# Hive's entrypoint sets user.name, user.email and credential.helper with
# `git config --global`, which writes individual keys and leaves core.hooksPath
# intact. Hooks are ergonomics only: --no-verify bypasses all of them.
if [ -d /opt/bluefin/git-hooks ]; then
	git config --global core.hooksPath /opt/bluefin/git-hooks || true
fi

# --- Context7 preflight ------------------------------------------------------
#
# Goose only warns and continues when an extension fails to start, so a session
# can look healthy while Context7 is unreachable. Probe it here and say so
# plainly; the local agent policy already tells the agent to proceed on local
# evidence when Context7 is unavailable.
if command -v curl >/dev/null 2>&1; then
	# Probe with a real MCP `initialize` call. A bare GET returns 405 and a
	# `ping` is rejected before the session exists, so either would report a
	# healthy server as unreachable. Streamable HTTP also requires the
	# text/event-stream Accept type.
	if curl --silent --show-error --fail --max-time 8 --output /dev/null \
		--request POST \
		--header 'Content-Type: application/json' \
		--header 'Accept: application/json, text/event-stream' \
		--data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"review","version":"1"}}}' \
		https://mcp.context7.com/mcp 2>/dev/null; then
		note 'Context7 reachable'
	else
		note 'Context7 unreachable; continuing on local repository evidence only'
	fi
fi

skills_root="${HOME}/.agents/skills"
if [ -d "$skills_root" ]; then
	shopt -s nullglob
	skills=("$skills_root"/*/SKILL.md)
	note "${#skills[@]} org skills available (load one with /<skill-name>)"
fi

validation_tools=(bats shellcheck systemd-analyze pre-commit just podman)
missing_validation_tools=()
for validation_tool in "${validation_tools[@]}"; do
	if ! command -v "$validation_tool" >/dev/null 2>&1; then
		missing_validation_tools+=("$validation_tool")
	fi
done
if ((${#missing_validation_tools[@]})); then
	note "validation tools unavailable: ${missing_validation_tools[*]}"
fi

# --- Hand over to Hive -------------------------------------------------------
#
# contributor-agent.sh creates the tmux session named "contributor", starts the
# relay, and launches Goose by keystroke injection. Attaching to that session is
# Hive's own documented flow. Running it in the foreground is deliberate: the
# launcher never backgrounds or detaches the agent.
# The attach client must describe the terminal that actually renders tmux.
# FSDK includes only the narrow terminfo set compiled in the image, so fall
# back to xterm when the caller's terminal is not available.
tmux_fallback_term=xterm-256color
if ! infocmp "${TERM:-}" >/dev/null 2>&1; then
	note "TERM=${TERM:-<unset>} has no terminfo; using ${tmux_fallback_term}"
	export TERM="$tmux_fallback_term"
fi
agent_pid=
cleanup() {
	status=$?
	if [ -n "$agent_pid" ] && kill -0 "$agent_pid" 2>/dev/null; then
		kill "$agent_pid" 2>/dev/null || true
		wait "$agent_pid" 2>/dev/null || true
	fi
	tmux kill-session -t contributor 2>/dev/null || true
	exit "$status"
}
trap cleanup EXIT HUP INT TERM

/usr/local/bin/contributor-agent.sh "$@" &
agent_pid=$!

attempts=0
while ! tmux has-session -t contributor 2>/dev/null; do
	if ! kill -0 "$agent_pid" 2>/dev/null; then
		wait "$agent_pid"
		exit $?
	fi
	attempts=$((attempts + 1))
	if [ "$attempts" -ge 600 ]; then
		note 'contributor session did not start'
		note "tmux readiness diagnostics: TMUX=${TMUX:-<unset>} TMUX_TMPDIR=${TMUX_TMPDIR:-<unset>}"
		tmux_state="$(tmux ls 2>&1 || true)"
		note "tmux readiness diagnostics: ${tmux_state//$'\n'/; }"
		exit 1
	fi
	sleep 0.1
done

# Attach only when there is a terminal. Without this an unattended run would
# fail on `tmux attach`, which refuses to run without a tty.
if [ -t 0 ] && [ -t 1 ]; then
	tmux attach-session -t contributor
	note 'tmux detached; the agent remains foreground in this terminal. Press Ctrl-C or close this terminal to stop it.'
	wait "$agent_pid"
else
	note 'no tty; following the agent without attaching'
	wait "$agent_pid"
fi
