#!/usr/bin/env bash
# The FSDK base ships no findutils, so image/bin/find is the `find` the
# contributor runtime and the agent both get. Hive's relay prunes stale /tmp
# entries with a full GNU expression every ten minutes and discards its own
# stderr, so a shim that rejects those predicates fails invisibly and /tmp
# grows for the life of the session. Pin the exact upstream expression here.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

find_shim="$repo_root/image/bin/find"
user="$(id -un)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

stale="$(date -d '3 hours ago' +%Y%m%d%H%M)"

seed() {
	rm -rf "${work:?}"/*
	mkdir -p "$work"/{stale-dir,tmux-1000,claude-x,node-abc,fresh-dir}
	touch "$work"/{a.out,b.html,c.txt}
	touch -t "$stale" \
		"$work"/stale-dir "$work"/tmux-1000 "$work"/claude-x "$work"/node-abc \
		"$work"/a.out "$work"/b.html "$work"/c.txt
}

fail() {
	echo "::error file=image/bin/find::$1" >&2
	exit 1
}

# The exact directory expression from Hive's contributor-relay.sh.
seed
selected="$("$find_shim" "$work" -maxdepth 1 -type d -user "$user" \
	-not -name 'tmux-*' -not -name 'claude-*' -not -name 'node-*' \
	-not -name '.' -mmin +60 | sed "s#^$work/*##" | sort | tr '\n' ' ')"
[ "$selected" = "stale-dir " ] ||
	fail "Hive's stale-directory expression selected '${selected}', expected 'stale-dir '"

# -exec ... + must actually run, and must not take the exclusions with it.
seed
"$find_shim" "$work" -maxdepth 1 -type d -user "$user" \
	-not -name 'tmux-*' -not -name 'claude-*' -not -name 'node-*' \
	-not -name '.' -mmin +60 -exec rm -rf {} +
[ ! -e "$work/stale-dir" ] || fail '-exec ... + did not remove the stale directory'
for kept in tmux-1000 claude-x node-abc fresh-dir; do
	[ -d "$work/$kept" ] || fail "-exec ... + removed excluded directory $kept"
done

# GNU binds -a tighter than -o. Hive's file expression relies on that
# precedence, so the shim must group the same way rather than ANDing all terms.
seed
matched="$("$find_shim" "$work" -maxdepth 1 -type f -user "$user" \
	-name '*.out' -o -name '*.html' -mmin +60 | sed "s#^$work/*##" | sort | tr '\n' ' ')"
[ "$matched" = "a.out b.html " ] ||
	fail "Hive's stale-file expression selected '${matched}', expected 'a.out b.html '"

# Ordinary agent exploration must keep working.
seed
mkdir -p "$work/sub/deep" && touch "$work/sub/deep/x.txt"
[ "$("$find_shim" "$work" -type f -name '*.txt' | wc -l)" -eq 2 ] ||
	fail 'recursive -type f -name lost matches'
[ "$("$find_shim" "$work" -maxdepth 1 -type f | wc -l)" -eq 3 ] ||
	fail '-maxdepth 1 -type f is not limited to the top level'

# An unhandled predicate must fail loudly instead of reporting a wrong answer.
if "$find_shim" "$work" -size +1k >/dev/null 2>&1; then
	fail 'an unsupported predicate silently returned success'
fi

# cmp is Hive's ten-minute knowledge comparison; -s must stay exit-status only.
cmp_shim="$repo_root/image/bin/cmp"
printf 'same\n' >"$work/left"
cp "$work/left" "$work/right"
"$cmp_shim" -s "$work/left" "$work/right" || fail 'cmp -s reported identical files as different'
printf 'other\n' >"$work/right"
if "$cmp_shim" -s "$work/left" "$work/right"; then
	fail 'cmp -s reported differing files as identical'
fi
[ -z "$("$cmp_shim" -s "$work/left" "$work/right" 2>&1 || true)" ] ||
	fail 'cmp -s printed output'

echo "✓ find and cmp match the semantics Hive and the agent depend on."
