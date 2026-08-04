// Behavioural half of tests/mcp-app-contract.sh.
//
// Spawns the built stdio server, completes a real MCP handshake, and asserts
// the served tool and resource surface. Only tools/list and resources/list are
// issued: listing a tool does not call it, so this probe never reaches Hive or
// GitHub and needs no network.
import { spawn } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const serverBundle = path.join(root, 'dist', 'server.js');

const EXPECTED_TOOLS = [
  'show_bluefin_ops_control_panel',
  'get_ops_control_panel_snapshot',
];
const EXPECTED_RESOURCE_URI = 'ui://bluefin-ops-control-panel/main';
const EXPECTED_MIME_TYPE = 'text/html;profile=mcp-app';
const CSP_DOMAIN_LISTS = [
  'connectDomains',
  'resourceDomains',
  'frameDomains',
  'baseUriDomains',
];
const RESPONSE_TIMEOUT_MS = 20_000;

let failures = 0;

function fail(message) {
  console.error(`::error file=mcp-app/dist/server.js::${message}`);
  failures += 1;
}

function check(condition, message) {
  if (!condition) {
    fail(message);
  }
}

// A deliberately bare environment: no Hive base URL, no GitHub owner, no
// token. The server must serve its full surface without any of them.
const childEnvironment = {
  PATH: process.env.PATH ?? '',
  HOME: process.env.HOME ?? root,
};

const child = spawn(process.execPath, [serverBundle], {
  env: childEnvironment,
  stdio: ['pipe', 'pipe', 'pipe'],
});

const pending = new Map();
let stderrText = '';
let buffer = '';

child.stderr.setEncoding('utf8');
child.stderr.on('data', (chunk) => {
  stderrText += chunk;
});

child.stdout.setEncoding('utf8');
child.stdout.on('data', (chunk) => {
  buffer += chunk;
  let newline = buffer.indexOf('\n');
  while (newline !== -1) {
    const line = buffer.slice(0, newline).trim();
    buffer = buffer.slice(newline + 1);
    newline = buffer.indexOf('\n');
    if (!line) {
      continue;
    }

    let message;
    try {
      message = JSON.parse(line);
    } catch {
      fail(`server wrote a non-JSON line to stdout: ${line.slice(0, 120)}`);
      continue;
    }

    const resolve = pending.get(message.id);
    if (resolve) {
      pending.delete(message.id);
      resolve(message);
    }
  }
});

let nextId = 1;

function send(payload) {
  child.stdin.write(`${JSON.stringify(payload)}\n`);
}

function request(method, params = {}) {
  const id = nextId++;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`${method} did not answer within ${RESPONSE_TIMEOUT_MS}ms`));
    }, RESPONSE_TIMEOUT_MS);

    pending.set(id, (message) => {
      clearTimeout(timer);
      if (message.error) {
        reject(new Error(`${method} returned an error: ${JSON.stringify(message.error)}`));
        return;
      }
      resolve(message.result ?? {});
    });

    send({ jsonrpc: '2.0', id, method, params });
  });
}

function isEmptyList(value) {
  return Array.isArray(value) && value.length === 0;
}

async function main() {
  await request('initialize', {
    protocolVersion: '2025-06-18',
    capabilities: {},
    clientInfo: { name: 'mcp-app-contract', version: '0.0.0' },
  });
  send({ jsonrpc: '2.0', method: 'notifications/initialized', params: {} });

  const tools = (await request('tools/list')).tools ?? [];
  const toolNames = tools.map((tool) => tool?.name).sort();
  check(
    JSON.stringify(toolNames) === JSON.stringify([...EXPECTED_TOOLS].sort()),
    `tools/list must serve exactly ${EXPECTED_TOOLS.join(', ')}; got ${toolNames.join(', ') || '<none>'}`,
  );

  for (const tool of tools) {
    const name = tool?.name ?? '<unnamed>';
    check(
      tool?.annotations?.readOnlyHint === true,
      `tool ${name} must declare annotations.readOnlyHint: true`,
    );
    const schema = tool?.inputSchema ?? {};
    check(schema.type === 'object', `tool ${name} input schema must be an object schema`);
    check(
      Object.keys(schema.properties ?? {}).length === 0,
      `tool ${name} must accept no input properties`,
    );
    check(
      schema.additionalProperties === false,
      `tool ${name} input schema must set additionalProperties: false`,
    );
  }

  const resources = (await request('resources/list')).resources ?? [];
  check(resources.length === 1, `resources/list must serve exactly one resource; got ${resources.length}`);

  const resource = resources[0] ?? {};
  check(
    resource.uri === EXPECTED_RESOURCE_URI,
    `resource URI must be ${EXPECTED_RESOURCE_URI}; got ${resource.uri}`,
  );
  check(
    resource.mimeType === EXPECTED_MIME_TYPE,
    `resource MIME type must be ${EXPECTED_MIME_TYPE}; got ${resource.mimeType}`,
  );

  const csp = resource._meta?.ui?.csp ?? {};
  for (const list of CSP_DOMAIN_LISTS) {
    check(
      isEmptyList(csp[list]),
      `resource CSP ${list} must be present and empty; got ${JSON.stringify(csp[list])}`,
    );
  }

  const html = (await request('resources/read', { uri: EXPECTED_RESOURCE_URI })).contents?.[0];
  check(
    typeof html?.text === 'string' && html.text.includes('<!doctype html>'),
    'resource read must return the inlined panel HTML',
  );
}

main()
  .catch((error) => {
    fail(error instanceof Error ? error.message : String(error));
  })
  .finally(() => {
    child.stdin.end();
    child.kill();
    if (stderrText.trim()) {
      console.error(`server stderr: ${stderrText.trim()}`);
    }
    if (failures > 0) {
      process.exitCode = 1;
      return;
    }
    console.log('✓ mcp-app stdio surface holds.');
  });
