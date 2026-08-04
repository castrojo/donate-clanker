import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListResourcesRequestSchema,
  ListToolsRequestSchema,
  ReadResourceRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';
import { snapshotConfigFromEnvironment, getSnapshot } from './snapshot.js';

// Substituted by scripts/build.mjs. The server therefore imports no
// filesystem module and holds no read or write path at runtime.
declare const __OPS_CONTROL_PANEL_HTML__: string;

const RESOURCE_URI = 'ui://bluefin-ops-control-panel/main';
const RESOURCE_MIME_TYPE = 'text/html;profile=mcp-app';
const OPEN_PANEL_TOOL = 'show_bluefin_ops_control_panel';
const SNAPSHOT_TOOL = 'get_ops_control_panel_snapshot';
const EMPTY_OBJECT_SCHEMA = {
  type: 'object' as const,
  properties: {},
  additionalProperties: false,
};

const resourceMetadata = {
  ui: {
    csp: {
      connectDomains: [],
      resourceDomains: [],
      frameDomains: [],
      baseUriDomains: [],
    },
    prefersBorder: true,
  },
};
const runtimeProcess = (globalThis as {
  process?: { stderr?: { write: (message: string) => void }; exitCode?: number };
}).process;

const server = new Server(
  { name: '@projectbluefin/ops-control-panel', version: '0.0.0' },
  { capabilities: { resources: {}, tools: {} } },
);

server.setRequestHandler(ListToolsRequestSchema, () => ({
  tools: [
    {
      name: OPEN_PANEL_TOOL,
      description: 'Open the read-only Bluefin operations control panel.',
      inputSchema: EMPTY_OBJECT_SCHEMA,
      annotations: { readOnlyHint: true },
    },
    {
      name: SNAPSHOT_TOOL,
      description: 'Read one aggregate Bluefin operations evidence snapshot.',
      inputSchema: EMPTY_OBJECT_SCHEMA,
      annotations: { readOnlyHint: true },
    },
  ],
}));

server.setRequestHandler(ListResourcesRequestSchema, () => ({
  resources: [
    {
      uri: RESOURCE_URI,
      name: 'Bluefin Ops Control Panel',
      mimeType: RESOURCE_MIME_TYPE,
      _meta: resourceMetadata,
    },
  ],
}));

async function start(): Promise<void> {
  const html = __OPS_CONTROL_PANEL_HTML__;
  const snapshotConfig = snapshotConfigFromEnvironment();

  server.setRequestHandler(ReadResourceRequestSchema, (request) => {
    if (request.params.uri !== RESOURCE_URI) {
      throw new Error('Requested resource is unavailable.');
    }

    return {
      contents: [
        {
          uri: RESOURCE_URI,
          mimeType: RESOURCE_MIME_TYPE,
          text: html,
          _meta: resourceMetadata,
        },
      ],
    };
  });

  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    switch (request.params.name) {
      case OPEN_PANEL_TOOL:
        return {
          content: [{ type: 'text', text: 'Opening Bluefin Ops Control Panel.' }],
          _meta: { ui: { resourceUri: RESOURCE_URI } },
        };
      case SNAPSHOT_TOOL:
        return {
          content: [{ type: 'text', text: JSON.stringify(await getSnapshot(snapshotConfig)) }],
        };
      default:
        return {
          content: [{ type: 'text', text: 'Unknown tool.' }],
          isError: true,
        };
    }
  });

  await server.connect(new StdioServerTransport());
}

start().catch(() => {
  runtimeProcess?.stderr?.write('Unable to start the Bluefin Ops Control Panel MCP server.\n');
  if (runtimeProcess) {
    runtimeProcess.exitCode = 1;
  }
});
