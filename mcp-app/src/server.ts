import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';

const server = new Server(
  { name: '@projectbluefin/ops-control-panel', version: '0.0.0' },
  { capabilities: { resources: {}, tools: {} } },
);

await server.connect(new StdioServerTransport());
