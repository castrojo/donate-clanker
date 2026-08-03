#!/usr/bin/env bash
# pi container entrypoint.
#
# Two modes:
#   Hive contributor — connects to a Hive hub (like the hosted Project
#     Bluefin instance) via contributor-agent.sh for ongoing task-driven work.
#   Standalone — runs pi directly in a tmux session for attended use.
#
# DeepSeek is the default provider. Pass DEEPSEEK_API_KEY in the environment.
set -euo pipefail

note() { printf 'pi-container: %s\n' "$1" >&2; }

# ── Provider defaults ────────────────────────────────────────────────────
PI_PROVIDER="${PI_PROVIDER:-deepseek}"
PI_MODEL="${PI_MODEL:-deepseek-v4-flash}"
PI_THINKING="${PI_THINKING:-}"
PI_ARGS=()

if [ -n "$PI_PROVIDER" ]; then
	PI_ARGS+=(--provider "$PI_PROVIDER")
fi
if [ -n "$PI_MODEL" ]; then
	PI_ARGS+=(--model "$PI_MODEL")
fi
if [ -n "$PI_THINKING" ]; then
	PI_ARGS+=(--thinking "$PI_THINKING")
fi

if [ -n "${PI_EXTRA_ARGS:-}" ]; then
	IFS=' ' read -r -a extra_args <<< "$PI_EXTRA_ARGS"
	PI_ARGS+=("${extra_args[@]}")
fi

# ── Terminfo ─────────────────────────────────────────────────────────────
if command -v tic >/dev/null 2>&1 && [ -f /tmp/xterm-256color.src ]; then
	tic -x -o /usr/share/terminfo /tmp/xterm-256color.src 2>/dev/null || true
	rm -f /tmp/xterm-256color.src
fi

# ── Host identity ────────────────────────────────────────────────────────
if [ -f /home/dev/.gitconfig ] && [ ! -f /home/dev/.gitconfig.applied ]; then
	git config --global include.path /home/dev/.gitconfig 2>/dev/null || true
	touch /home/dev/.gitconfig.applied
fi

# ── Hive entrypoint hooks ────────────────────────────────────────────────
if [ -d /etc/hive/entrypoint.d ]; then
	for hook in /etc/hive/entrypoint.d/*.sh; do
		[ -x "$hook" ] && source "$hook"
	done
fi

# ── Mode: Hive contributor ───────────────────────────────────────────────
# When HIVE_HUB and HIVE_REGISTRATION_TOKEN are set, run as a Hive
# contributor that receives tasks from the hub on an ongoing basis.
if [ -n "${HIVE_HUB:-}" ] && [ -n "${HIVE_REGISTRATION_TOKEN:-}" ] && \
   [ -f /usr/local/bin/contributor-agent.sh ]; then

	export AGENT_BACKEND="${AGENT_BACKEND:-pi}"
	export AGENT_MODEL="$PI_MODEL"
	export HIVE_CONTRIBUTOR_MODE=true
	export HIVE_CONTAINER_NAME="${HIVE_CONTAINER_NAME:-pi}"

	note "Hive contributor mode"
	note "  hub:     ${HIVE_HUB}"
	note "  backend: ${AGENT_BACKEND}"
	note "  model:   ${AGENT_MODEL}"
	note "  tmux:    podman exec -it pi tmux attach -t contributor"

	# contributor-agent.sh handles the full lifecycle: relay, tmux,
	# knowledge refresh, and agent launch.
	exec /usr/local/bin/contributor-agent.sh
fi

# ── Mode: Standalone ─────────────────────────────────────────────────────
PI_MODE="${PI_MODE:-interactive}"

case "$PI_MODE" in
interactive)
	note "standalone pi in tmux session 'pi' (provider: ${PI_PROVIDER}, model: ${PI_MODEL})"
	note "  attach:  podman exec -it pi tmux attach -t pi"
	note "  detach:  Ctrl-b d"
	note "  stop:    podman stop pi"

	TMUX_TERM="${TMUX_TERM:-xterm-256color}"
	export TERM="$TMUX_TERM"
	SESSION="pi"

	agent_pid=
	cleanup() {
		local status=$?
		if [ -n "${agent_pid:-}" ] && kill -0 "$agent_pid" 2>/dev/null; then
			kill "$agent_pid" 2>/dev/null || true
			wait "$agent_pid" 2>/dev/null || true
		fi
		tmux kill-session -t "$SESSION" 2>/dev/null || true
		exit "$status"
	}
	trap cleanup EXIT HUP INT TERM

	tmux new-session -d -s "$SESSION" \
		pi "${PI_ARGS[@]}" 2>/dev/null &
	agent_pid=$!

	attempts=0
	while ! tmux has-session -t "$SESSION" 2>/dev/null; do
		if ! kill -0 "$agent_pid" 2>/dev/null; then
			wait "$agent_pid" 2>/dev/null || true
			exit $?
		fi
		attempts=$((attempts + 1))
		[ "$attempts" -ge 600 ] && { note "pi tmux session did not start"; exit 1; }
		sleep 0.1
	done

	note "pi session running."
	if [ -t 0 ] && [ -t 1 ]; then
		tmux attach-session -t "$SESSION"
		note "tmux detached; pi remains in the foreground."
		wait "$agent_pid"
	else
		note "no tty; following pi without attaching."
		wait "$agent_pid"
	fi
	;;

headless)
	note "pi headless mode: processing PI_PROMPT and exiting"
	[ -z "${PI_PROMPT:-}" ] && { note "ERROR: PI_PROMPT is required in headless mode."; exit 1; }
	exec pi -p "${PI_ARGS[@]}" "$PI_PROMPT"
	;;

rpc)
	note "pi RPC mode: serving on stdin/stdout"
	exec pi --mode rpc "${PI_ARGS[@]}"
	;;

*)
	note "ERROR: unknown PI_MODE=${PI_MODE} (use interactive, headless, or rpc)"
	exit 1
	;;
esac
