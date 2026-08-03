import assert from "node:assert/strict";
import test from "node:test";
import { buildSnapshot } from "./snapshot.js";

test("uses only canonical Hive status and safe contributor links", async () => {
  const snapshot = await buildSnapshot({
    hiveHub: "wss://hive.example/contribute?registration_token=secret",
    agentMdPath: "/missing",
    fetchImpl: async (url) => {
      assert.equal(url, "https://hive.example/api/contribute/status");
      return new Response(JSON.stringify({
        hub: "online",
        active_contributors: 2,
        total_registered: 4,
        actionable_items: 8,
      }));
    },
  });
  assert.deepEqual(snapshot.hive, {
    state: "online",
    activeContributors: 2,
    registeredContributors: 4,
    actionableItems: 8,
    contributorUrl: "https://hive.example/contribute",
    dashboardUrl: "https://hive.example/",
    manualIssuesUrl:
      "https://github.com/issues?q=is%3Aopen+is%3Aissue+org%3Aprojectbluefin",
  });
  assert.equal(snapshot.knowledge.state, "unavailable");
});

test("reports unavailable rather than inventing Hive state", async () => {
  const snapshot = await buildSnapshot({
    hiveHub: "wss://hive.example/contribute",
    agentMdPath: "/missing",
    fetchImpl: async () => {
      throw new Error("offline");
    },
  });
  assert.equal(snapshot.hive.state, "unavailable");
  assert.equal(snapshot.hive.activeContributors, null);
});
