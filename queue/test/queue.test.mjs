import assert from 'node:assert/strict';
import test from 'node:test';

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
