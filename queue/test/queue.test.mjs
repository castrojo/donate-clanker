import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';

import { generateSnapshots } from '../generate.mjs';
import { fetchOpenPullRequests } from '../lib/github.mjs';
import { buildQueue } from '../lib/queue.mjs';

function pullRequest(overrides = {}) {
  return {
    repository: 'projectbluefin/bluefin',
    number: 1,
    title: 'fix: improve queue handling',
    url: 'https://github.com/projectbluefin/bluefin/pull/1',
    updatedAt: '2026-08-03T16:00:00Z',
    labels: ['quality'],
    reviewState: 'review_required',
    mergeableState: 'clean',
    checkState: 'success',
    ...overrides,
  };
}

test('classifies pull requests and ranks a repository batch deterministically', () => {
  const result = buildQueue({
    generatedAt: '2026-08-03T16:30:00Z',
    pullRequests: [
      pullRequest({ number: 1, checkState: 'failure' }),
      pullRequest({ number: 2, mergeableState: 'dirty' }),
      pullRequest({ number: 3 }),
      pullRequest({ number: 4, mergeableState: 'unknown' }),
      pullRequest({ number: 5, reviewState: 'approved' }),
      pullRequest({
        repository: 'projectbluefin/dakota',
        number: 6,
        title: 'docs: update queue notes',
        url: 'https://github.com/projectbluefin/dakota/pull/6',
      }),
    ],
  });

  const itemsByNumber = new Map(result.items.map((item) => [item.number, item]));

  assert.equal(itemsByNumber.get(1).recommended_action, 'fix-ci');
  assert.equal(itemsByNumber.get(2).recommended_action, 'resolve-conflicts');
  assert.equal(itemsByNumber.get(3).recommended_action, 'review');
  assert.equal(itemsByNumber.get(4).recommended_action, 'investigate');
  assert.equal(itemsByNumber.get(5).recommended_action, 'ready-for-human-merge');
  assert.deepEqual(
    result.items.map((item) => item.id),
    [
      'projectbluefin/bluefin#1',
      'projectbluefin/bluefin#2',
      'projectbluefin/bluefin#3',
      'projectbluefin/bluefin#4',
      'projectbluefin/bluefin#5',
      'projectbluefin/dakota#6',
    ],
  );
  assert.match(itemsByNumber.get(1).ranking_reasons[0], /5 open pull requests/);
});

test('renders matching deterministic Markdown and JSON artifacts', () => {
  const input = {
    generatedAt: '2026-08-03T16:30:00Z',
    pullRequests: [pullRequest()],
  };
  const first = buildQueue(input);
  const second = buildQueue(input);
  const document = JSON.parse(first.json);

  assert.equal(document.generated_at, input.generatedAt);
  assert.deepEqual(Object.keys(document.items[0]), [
    'id',
    'repository',
    'number',
    'url',
    'title',
    'updated_at',
    'labels',
    'review_state',
    'mergeable_state',
    'check_state',
    'recommended_action',
    'ranking_reasons',
  ]);
  assert.match(first.markdown, /Generated: 2026-08-03T16:30:00Z/);
  assert.match(first.markdown, /## projectbluefin\/bluefin/);
  assert.match(first.markdown, /https:\/\/github.com\/projectbluefin\/bluefin\/pull\/1/);
  assert.equal(first.markdown, second.markdown);
  assert.equal(first.json, second.json);
});

function jsonResponse(value, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function githubPullRequest(number, sha = `sha-${number}`) {
  return {
    number,
    title: `fix: pull request ${number}`,
    html_url: `https://github.com/projectbluefin/bluefin/pull/${number}`,
    updated_at: '2026-08-03T16:00:00Z',
    labels: [{ name: 'quality' }],
    head: { sha },
  };
}

test('fetches every pull-request page and derives GitHub evidence', async () => {
  const fetch = async (input) => {
    const url = new URL(input);
    if (url.pathname === '/repos/projectbluefin/bluefin/pulls') {
      return jsonResponse(
        url.searchParams.get('page') === '1'
          ? Array.from({ length: 100 }, (_, index) => githubPullRequest(index + 1))
          : [githubPullRequest(101)],
      );
    }
    if (url.pathname.endsWith('/reviews')) {
      return jsonResponse([]);
    }
    if (url.pathname.includes('/check-runs')) {
      return jsonResponse({ check_runs: [] });
    }
    if (url.pathname.includes('/pulls/')) {
      return jsonResponse({ mergeable_state: 'clean' });
    }
    throw new Error(`Unexpected request: ${url}`);
  };

  const pullRequests = await fetchOpenPullRequests({
    fetch,
    owner: 'projectbluefin',
    repositories: ['bluefin'],
  });

  assert.equal(pullRequests.length, 101);
  assert.equal(pullRequests[0].repository, 'projectbluefin/bluefin');
  assert.equal(pullRequests[0].number, 1);
  assert.equal(pullRequests[0].reviewState, 'review_required');
  assert.equal(pullRequests[0].mergeableState, 'clean');
  assert.equal(pullRequests[0].checkState, 'success');
  assert.equal(pullRequests[100].number, 101);
});

test('rejects failed list responses and preserves existing snapshots', async () => {
  const failingFetch = async () => jsonResponse({ message: 'unavailable' }, 503);
  await assert.rejects(
    () =>
      fetchOpenPullRequests({
        fetch: failingFetch,
        owner: 'projectbluefin',
        repositories: ['bluefin'],
      }),
    /GitHub request failed for projectbluefin\/bluefin pull requests/,
  );

  const outputDirectory = await mkdtemp(path.join(tmpdir(), 'agent-pr-queue-'));
  const markdownPath = path.join(outputDirectory, 'queue.md');
  const jsonPath = path.join(outputDirectory, 'queue.json');
  await writeFile(markdownPath, 'previous markdown\n');
  await writeFile(jsonPath, 'previous json\n');

  await assert.rejects(
    () =>
      generateSnapshots({
        fetch: failingFetch,
        owner: 'projectbluefin',
        repositories: ['bluefin'],
        outputDirectory,
      }),
    /GitHub request failed/,
  );

  assert.equal(await readFile(markdownPath, 'utf8'), 'previous markdown\n');
  assert.equal(await readFile(jsonPath, 'utf8'), 'previous json\n');
  await rm(outputDirectory, { recursive: true, force: true });
});
