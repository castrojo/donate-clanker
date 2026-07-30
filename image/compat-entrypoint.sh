#!/bin/sh
set -eu

config_root="${DONATE_CLANKER_CONFIG_DIR:-/config}"
workspace="${DONATE_CLANKER_WORKSPACE_DIR:-/workspace}"

if [ ! -d "$config_root" ]; then
	echo "donate-clanker: missing /config mount" >&2
	exit 1
fi
if [ ! -d "$workspace" ]; then
	echo "donate-clanker: missing /workspace mount" >&2
	exit 1
fi

mkdir -p "$HOME/.config"
rm -rf "$HOME/.config/hive" "$HOME/.config/gh" "$HOME/workspace"

hive_config="$config_root"
if [ -d "$config_root/hive" ]; then
	hive_config="$config_root/hive"
fi
ln -s "$hive_config" "$HOME/.config/hive"

if [ -d "$config_root/github" ]; then
	ln -s "$config_root/github" "$HOME/.config/gh"
elif [ -d "$config_root/gh" ]; then
	ln -s "$config_root/gh" "$HOME/.config/gh"
fi

# The upstream agent expects the GitHub token as an environment variable,
# while the compact portable contract mounts the Hive directory as /config.
auth_env="$hive_config/gh-auth.env"
if [ -f "$auth_env" ]; then
	set -a
	. "$auth_env"
	set +a
fi

ln -s "$workspace" "$HOME/workspace"

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
trap cleanup EXIT INT TERM

/usr/local/bin/contributor-agent.sh "$@" &
agent_pid=$!

attempts=0
while ! tmux has-session -t contributor 2>/dev/null; do
	if ! kill -0 "$agent_pid" 2>/dev/null; then
		wait "$agent_pid"
		exit $?
	fi
	attempts=$((attempts + 1))
	if [ "$attempts" -ge 100 ]; then
		echo "donate-clanker: contributor session did not start" >&2
		exit 1
	fi
	sleep 0.1
done

tmux attach-session -t contributor
