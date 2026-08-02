#!/usr/bin/env bash
# Hive's stock contributor script reads this export without authentication.
# The hosted Project Bluefin hub instead exposes the same Markdown through its
# GitHub-authenticated v1 API. Keep the rewrite exact so other Hive deployments
# retain the stock request and refresh behavior.
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
