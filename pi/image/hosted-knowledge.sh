#!/usr/bin/env bash
# Hosted Bluefin knowledge hook.
#
# The hosted Project Bluefin Hive exposes its knowledge export through an
# authenticated v1 API instead of the unauthenticated stock endpoint.
# Rewrite curl calls transparently when the hub is the Bluefin hosted
# instance so the contributor-agent.sh knowledge refresh loop works.
set -euo pipefail

hosted_hub="wss://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute"
if [[ "${HIVE_HUB:-}" != "$hosted_hub" ]] || [[ -z "${GH_TOKEN:-}" ]]; then
	return 0
fi

hub_http="${HIVE_HUB/wss:\/\//https://}"
knowledge_export="${hub_http%/contribute}/api/knowledge/export"
hosted_knowledge="${hub_http%/contribute}/api/v1/knowledge"
curl_binary="$(type -P curl)"

curl() {
	local argument rewritten=()
	for argument in "$@"; do
		if [[ "$argument" == "$knowledge_export" ]]; then
			rewritten+=("$hosted_knowledge")
		else
			rewritten+=("$argument")
		fi
	done
	if [[ " ${rewritten[*]} " != *" $hosted_knowledge "* ]]; then
		command "$curl_binary" "${rewritten[@]}"
		return
	fi
	command "$curl_binary" --header "Authorization: Bearer ${GH_TOKEN}" "${rewritten[@]}"
}
