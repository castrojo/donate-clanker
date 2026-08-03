import { mkdir, mkdtemp, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { fetchOpenPullRequests } from './lib/github.mjs';
import { buildQueue } from './lib/queue.mjs';

function parseRepositories(value) {
  if (typeof value !== 'string') {
    throw new TypeError('QUEUE_REPOSITORIES must be configured');
  }
  const repositories = value
    .split(',')
    .map((repository) => repository.trim())
    .filter(Boolean);
  if (
    repositories.length === 0 ||
    repositories.some((repository) => !/^[A-Za-z0-9_.-]+$/.test(repository))
  ) {
    throw new TypeError('QUEUE_REPOSITORIES must list repository names');
  }
  return repositories;
}

export async function generateSnapshots({
  fetch = globalThis.fetch,
  owner,
  repositories,
  token,
  outputDirectory,
  generatedAt = new Date().toISOString(),
}) {
  if (typeof outputDirectory !== 'string' || outputDirectory === '') {
    throw new TypeError('outputDirectory must be configured');
  }

  const pullRequests = await fetchOpenPullRequests({
    fetch,
    owner,
    repositories,
    token,
  });
  const queue = buildQueue({ pullRequests, generatedAt });
  await mkdir(outputDirectory, { recursive: true });
  const temporaryDirectory = await mkdtemp(
    path.join(outputDirectory, '.agent-pr-queue-'),
  );
  const temporaryMarkdown = path.join(temporaryDirectory, 'queue.md');
  const temporaryJson = path.join(temporaryDirectory, 'queue.json');

  await writeFile(temporaryMarkdown, queue.markdown);
  await writeFile(temporaryJson, queue.json);
  await rename(temporaryMarkdown, path.join(outputDirectory, 'queue.md'));
  await rename(temporaryJson, path.join(outputDirectory, 'queue.json'));
  await rm(temporaryDirectory, { recursive: true, force: true });
  return queue;
}

async function main() {
  if (process.env.QUEUE_OWNER !== 'projectbluefin') {
    throw new TypeError('QUEUE_OWNER must be projectbluefin');
  }

  await generateSnapshots({
    owner: process.env.QUEUE_OWNER,
    repositories: parseRepositories(process.env.QUEUE_REPOSITORIES),
    token: process.env.GH_TOKEN,
    outputDirectory: path.resolve('public'),
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
