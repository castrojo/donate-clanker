import { stat } from "node:fs/promises";

const MANUAL_ISSUES_URL =
  "https://github.com/issues?q=is%3Aopen+is%3Aissue+org%3Aprojectbluefin";
const HIVE_PROMPT_URL =
  "https://github.com/kubestellar/hive/blob/835448c3cbef9f06d34dd3802548e1d1e16dbd2f/v2/pkg/dashboard/contribute_ws.go#L1242-L1251";
const BLUEFIN_POLICY_URL =
  "https://github.com/projectbluefin/donate-clanker/blob/main/image/config/local-agent-policy.md";

function urlsForHub(hiveHub) {
  const url = new URL(hiveHub);
  if (!["ws:", "wss:", "http:", "https:"].includes(url.protocol)) {
    throw new TypeError("HIVE_HUB must use an HTTP(S) or WebSocket scheme");
  }
  url.username = "";
  url.password = "";
  url.search = "";
  url.hash = "";
  url.protocol = url.protocol === "wss:" ? "https:" : url.protocol === "ws:" ? "http:" : url.protocol;
  url.pathname = "/contribute";
  const origin = url.origin;
  return {
    contributorUrl: url.toString(),
    dashboardUrl: `${origin}/`,
    statusUrl: `${origin}/api/contribute/status`,
  };
}

async function knowledge(agentMdPath) {
  try {
    const metadata = await stat(agentMdPath);
    return { state: "available", refreshedAt: metadata.mtime.toISOString() };
  } catch {
    return { state: "unavailable", refreshedAt: null };
  }
}

export async function buildSnapshot({
  hiveHub,
  agentMdPath = `${process.env.HOME}/agent.md`,
  fetchImpl = fetch,
} = {}) {
  const links = urlsForHub(hiveHub);
  const hive = {
    state: "unavailable",
    activeContributors: null,
    registeredContributors: null,
    actionableItems: null,
    contributorUrl: links.contributorUrl,
    dashboardUrl: links.dashboardUrl,
    manualIssuesUrl: MANUAL_ISSUES_URL,
  };

  try {
    const response = await fetchImpl(links.statusUrl, {
      signal: AbortSignal.timeout(8_000),
    });
    if (!response.ok) throw new Error(`Hive returned ${response.status}`);
    const status = await response.json();
    hive.state = status.hub === "online" ? "online" : "suspended";
    hive.activeContributors = Number.isInteger(status.active_contributors)
      ? status.active_contributors
      : null;
    hive.registeredContributors = Number.isInteger(status.total_registered)
      ? status.total_registered
      : null;
    hive.actionableItems = Number.isInteger(status.actionable_items)
      ? status.actionable_items
      : null;
  } catch {
    // The UI must clearly report unavailable telemetry rather than guess.
  }

  return {
    hive,
    knowledge: await knowledge(agentMdPath),
    sources: {
      hivePromptUrl: HIVE_PROMPT_URL,
      bluefinPolicyUrl: BLUEFIN_POLICY_URL,
    },
  };
}
