#!/usr/bin/env node
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListResourcesRequestSchema,
  ListToolsRequestSchema,
  ReadResourceRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { buildSnapshot } from "./snapshot.js";

const resourceUri = "ui://bluefin-operations/tactical-board";
const directory = dirname(fileURLToPath(import.meta.url));
const template = await readFile(join(directory, "index.html"), "utf8");
let snapshot;

function render(snapshotValue) {
  return template.replace(
    "__SNAPSHOT__",
    JSON.stringify(snapshotValue).replaceAll("<", "\\u003c"),
  );
}

const server = new Server(
  { name: "bluefin-operations", version: "1.0.0" },
  { capabilities: { tools: {}, resources: {} } },
);

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [{
    name: "show_bluefin_operations",
    description: "Show the read-only Bluefin Operations Tactical Board.",
    inputSchema: { type: "object", properties: {}, required: [] },
  }],
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  if (request.params.name !== "show_bluefin_operations") {
    throw new Error(`Unknown tool: ${request.params.name}`);
  }
  snapshot = await buildSnapshot({ hiveHub: process.env.HIVE_HUB });
  return {
    content: [{ type: "text", text: "Bluefin Operations Tactical Board opened." }],
    _meta: { ui: { resourceUri } },
  };
});

server.setRequestHandler(ListResourcesRequestSchema, async () => ({
  resources: [{
    uri: resourceUri,
    name: "Bluefin Operations Tactical Board",
    mimeType: "text/html;profile=mcp-app",
  }],
}));

server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
  if (request.params.uri !== resourceUri) throw new Error("Unknown resource");
  snapshot ??= await buildSnapshot({ hiveHub: process.env.HIVE_HUB });
  return {
    contents: [{
      uri: resourceUri,
      mimeType: "text/html;profile=mcp-app",
      text: render(snapshot),
      _meta: {
        ui: {
          csp: {
            connectDomains: [],
            resourceDomains: [],
            frameDomains: [],
            baseUriDomains: [],
          },
          prefersBorder: true,
        },
      },
    }],
  };
});

await server.connect(new StdioServerTransport());
