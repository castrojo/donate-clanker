import { build } from 'esbuild';
import { mkdir, readFile, rm, writeFile, access } from 'node:fs/promises';
import { constants as fsConstants } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcDir = path.join(root, 'src');
const distDir = path.join(root, 'dist');
const serverEntry = path.join(srcDir, 'server.ts');
const clientEntry = path.join(srcDir, 'ui', 'main.tsx');
const templatePath = path.join(srcDir, 'ui', 'index.html');

const fallbackServerSource = `
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';

const server = new Server(
  { name: '@projectbluefin/ops-control-panel', version: '0.0.0' },
  { capabilities: { resources: {}, tools: {} } },
);

await server.connect(new StdioServerTransport());
`;

const fallbackClientSource = `
import React from 'react';
import { createRoot } from 'react-dom/client';

const host = document.getElementById('root');

if (host) {
  createRoot(host).render(
    React.createElement(
      'main',
      { className: 'ops-control-panel' },
      React.createElement('h1', null, 'Bluefin Ops Control Panel'),
      React.createElement('p', null, 'Package scaffold ready.')
    ),
  );
}
`;

async function exists(filePath) {
  try {
    await access(filePath, fsConstants.F_OK);
    return true;
  } catch {
    return false;
  }
}

async function bundleNode(entryPath, fallbackSource) {
  const result = await build({
    entryPoints: (await exists(entryPath)) ? [entryPath] : undefined,
    stdin: (await exists(entryPath))
      ? undefined
      : {
          contents: fallbackSource,
          resolveDir: srcDir,
          sourcefile: path.basename(entryPath),
          loader: 'ts',
        },
    bundle: true,
    platform: 'node',
    format: 'esm',
    target: 'es2022',
    write: false,
  });

  return result.outputFiles[0].text;
}

async function bundleBrowser(entryPath, fallbackSource) {
  const result = await build({
    entryPoints: (await exists(entryPath)) ? [entryPath] : undefined,
    stdin: (await exists(entryPath))
      ? undefined
      : {
          contents: fallbackSource,
          resolveDir: path.dirname(entryPath),
          sourcefile: path.basename(entryPath),
          loader: 'ts',
        },
    bundle: true,
    platform: 'browser',
    format: 'esm',
    target: 'es2022',
    write: false,
  });

  return result.outputFiles[0].text;
}

async function main() {
  await rm(distDir, { recursive: true, force: true });
  await mkdir(distDir, { recursive: true });

  const [serverBundle, clientBundle, template] = await Promise.all([
    bundleNode(serverEntry, fallbackServerSource),
    bundleBrowser(clientEntry, fallbackClientSource),
    readFile(templatePath, 'utf8'),
  ]);

  const safeClientBundle = clientBundle
    .replace(/<\/script/gi, '<\\/script')
    .replace(/<!--/g, '<\\!--');

  await writeFile(path.join(distDir, 'server.js'), serverBundle);
  await writeFile(
    path.join(distDir, 'index.html'),
    template.replace(
      '<!--CLIENT_BUNDLE-->',
      `<script type="module">\n${safeClientBundle}\n</script>`,
    ),
  );
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
