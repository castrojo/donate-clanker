const API_BASE_URL = 'https://api.github.com';
const FAILED_CHECK_CONCLUSIONS = new Set([
  'action_required',
  'cancelled',
  'failure',
  'startup_failure',
  'timed_out',
]);

function validIdentifier(value, name) {
  if (typeof value !== 'string' || !/^[A-Za-z0-9_.-]+$/.test(value)) {
    throw new TypeError(`${name} must be an identifier`);
  }
  return value;
}

function requestHeaders(token) {
  const headers = { accept: 'application/vnd.github+json' };
  if (typeof token === 'string' && token.trim() !== '') {
    headers.authorization = `Bearer ${token}`;
  }
  return headers;
}

async function requestJson(fetch, url, token, context) {
  const response = await fetch(url, { headers: requestHeaders(token) });
  if (!response.ok) {
    throw new Error(`GitHub request failed for ${context}: HTTP ${response.status}`);
  }

  try {
    return await response.json();
  } catch {
    throw new Error(`GitHub response was malformed for ${context}`);
  }
}

function reviewState(result) {
  if (result.status !== 'fulfilled' || !Array.isArray(result.value)) {
    return 'unknown';
  }
  return result.value.some(
    (review) => review && typeof review === 'object' && review.state === 'APPROVED',
  )
    ? 'approved'
    : 'review_required';
}

function checkState(result) {
  if (
    result.status !== 'fulfilled' ||
    typeof result.value !== 'object' ||
    result.value === null ||
    !Array.isArray(result.value.check_runs)
  ) {
    return 'unknown';
  }

  if (
    result.value.check_runs.some(
      (check) =>
        check &&
        typeof check === 'object' &&
        FAILED_CHECK_CONCLUSIONS.has(check.conclusion),
    )
  ) {
    return 'failure';
  }

  return result.value.check_runs.every(
    (check) =>
      check &&
      typeof check === 'object' &&
      (check.status === undefined || check.status === 'completed'),
  )
    ? 'success'
    : 'unknown';
}

function mergeableState(result) {
  if (
    result.status !== 'fulfilled' ||
    typeof result.value !== 'object' ||
    result.value === null
  ) {
    return 'unknown';
  }
  if (result.value.mergeable_state === 'clean') {
    return 'clean';
  }
  if (result.value.mergeable_state === 'dirty') {
    return 'dirty';
  }
  return 'unknown';
}

function pullRequestFields(value, repository) {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`GitHub response was malformed for ${repository} pull requests`);
  }
  if (
    !Number.isSafeInteger(value.number) ||
    typeof value.title !== 'string' ||
    typeof value.html_url !== 'string' ||
    typeof value.updated_at !== 'string' ||
    !Array.isArray(value.labels) ||
    typeof value.head !== 'object' ||
    value.head === null ||
    typeof value.head.sha !== 'string'
  ) {
    throw new Error(`GitHub response was malformed for ${repository} pull requests`);
  }

  const labels = value.labels.map((label) =>
    label && typeof label === 'object' && typeof label.name === 'string'
      ? label.name
      : undefined,
  );
  if (labels.some((label) => label === undefined)) {
    throw new Error(`GitHub response was malformed for ${repository} pull requests`);
  }

  return {
    number: value.number,
    title: value.title,
    url: value.html_url,
    updatedAt: value.updated_at,
    labels,
    sha: value.head.sha,
  };
}

async function enrichPullRequest({ fetch, owner, repository, token, pullRequest }) {
  const base = new URL(
    `/repos/${owner}/${repository}/pulls/${pullRequest.number}`,
    API_BASE_URL,
  );
  const reviews = new URL(`${base.pathname}/reviews`, API_BASE_URL);
  const checks = new URL(
    `/repos/${owner}/${repository}/commits/${pullRequest.sha}/check-runs`,
    API_BASE_URL,
  );
  const [details, reviewData, checkData] = await Promise.allSettled([
    requestJson(fetch, base, token, `${repository}#${pullRequest.number} details`),
    requestJson(fetch, reviews, token, `${repository}#${pullRequest.number} reviews`),
    requestJson(fetch, checks, token, `${repository}#${pullRequest.number} checks`),
  ]);

  return {
    repository: `${owner}/${repository}`,
    number: pullRequest.number,
    title: pullRequest.title,
    url: pullRequest.url,
    updatedAt: pullRequest.updatedAt,
    labels: pullRequest.labels,
    reviewState: reviewState(reviewData),
    mergeableState: mergeableState(details),
    checkState: checkState(checkData),
  };
}

async function pullRequestsForRepository({ fetch, owner, repository, token }) {
  const pullRequests = [];
  for (let page = 1; ; page += 1) {
    const url = new URL(`/repos/${owner}/${repository}/pulls`, API_BASE_URL);
    url.searchParams.set('state', 'open');
    url.searchParams.set('per_page', '100');
    url.searchParams.set('page', String(page));
    const payload = await requestJson(fetch, url, token, `${owner}/${repository} pull requests`);
    if (!Array.isArray(payload)) {
      throw new Error(`GitHub response was malformed for ${owner}/${repository} pull requests`);
    }
    pullRequests.push(...payload.map((item) => pullRequestFields(item, repository)));
    if (payload.length < 100) {
      break;
    }
  }

  return Promise.all(
    pullRequests.map((pullRequest) =>
      enrichPullRequest({ fetch, owner, repository, token, pullRequest }),
    ),
  );
}

export async function fetchOpenPullRequests({
  fetch = globalThis.fetch,
  owner,
  repositories,
  token,
}) {
  if (typeof fetch !== 'function') {
    throw new TypeError('fetch must be a function');
  }
  validIdentifier(owner, 'owner');
  if (!Array.isArray(repositories) || repositories.length === 0) {
    throw new TypeError('repositories must be a non-empty array');
  }

  const validatedRepositories = repositories.map((repository) =>
    validIdentifier(repository, 'repository'),
  );
  const pullRequestGroups = await Promise.all(
    validatedRepositories.map((repository) =>
      pullRequestsForRepository({ fetch, owner, repository, token }),
    ),
  );
  return pullRequestGroups.flat();
}
