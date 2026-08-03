import type {
  DataState,
  GitHubConfig,
  GitHubEvidence,
  GitHubReviewStatus,
  GitHubWorkItem,
} from '../contracts.js';

const SOURCE = 'github' as const;
const REQUEST_TIMEOUT_MS = 8_000;
const GITHUB_API_BASE_URL = 'https://api.github.com';

type JsonObject = Record<string, unknown>;
type Environment = Record<string, string | undefined>;

function runtimeEnvironment(): Environment {
  return (globalThis as { process?: { env?: Environment } }).process?.env ?? {};
}

function unknown<T>(reason: string): DataState<T> {
  return { kind: 'unknown', source: SOURCE, reason };
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function validIdentifier(value: string | undefined): value is string {
  return Boolean(value && /^[A-Za-z0-9_.-]+$/.test(value));
}

function selectedApiToken(environment: Environment): string | undefined {
  return (
    environment.REVIEW_GH_TOKEN?.trim() ||
    environment.GH_TOKEN?.trim() ||
    undefined
  );
}

export function githubAuthAvailable(environment: Environment = runtimeEnvironment()): boolean {
  return Boolean(selectedApiToken(environment));
}

function apiToken(): string | undefined {
  return selectedApiToken(runtimeEnvironment());
}

function requestHeaders(): HeadersInit {
  const token = apiToken();
  return token ? { accept: 'application/vnd.github+json', authorization: `Bearer ${token}` } : {
    accept: 'application/vnd.github+json',
  };
}

function reviewStatus(item: JsonObject): GitHubReviewStatus {
  if (!('requested_reviewers' in item) && !('requested_teams' in item)) {
    return 'unknown';
  }

  if (!Array.isArray(item.requested_reviewers) || !Array.isArray(item.requested_teams)) {
    return 'unknown';
  }

  return item.requested_reviewers.length + item.requested_teams.length > 0
    ? 'review-requested'
    : 'not-requested';
}

function workItem(value: unknown, observedAt: string): GitHubWorkItem | undefined {
  if (
    !isObject(value) ||
    typeof value.title !== 'string' ||
    typeof value.number !== 'number' ||
    !Number.isSafeInteger(value.number) ||
    value.number < 1 ||
    typeof value.html_url !== 'string' ||
    !Array.isArray(value.labels)
  ) {
    return undefined;
  }

  try {
    const url = new URL(value.html_url);
    if (!['http:', 'https:'].includes(url.protocol)) {
      return undefined;
    }
  } catch {
    return undefined;
  }

  const labels = value.labels.map((label) =>
    isObject(label) && typeof label.name === 'string' ? label.name : undefined,
  );
  if (labels.some((label) => label === undefined)) {
    return undefined;
  }

  return {
    title: value.title,
    number: value.number,
    url: value.html_url,
    labels: labels as string[],
    reviewStatus: reviewStatus(value),
    observedAt,
  };
}

async function fetchWorkItems(
  config: GitHubConfig,
  repository: string,
  resource: 'pulls' | 'issues',
): Promise<GitHubWorkItem[] | DataState<never>> {
  if (!validIdentifier(config.owner) || !validIdentifier(repository)) {
    return unknown('repository configuration is invalid');
  }

  const url = new URL(`/repos/${config.owner}/${repository}/${resource}`, GITHUB_API_BASE_URL);
  url.searchParams.set('state', 'open');

  try {
    const response = await fetch(url, {
      headers: requestHeaders(),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
    if (!response.ok) {
      return unknown('request was unavailable');
    }

    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      return unknown('response was malformed');
    }

    if (!Array.isArray(payload)) {
      return unknown('response was unsupported');
    }

    const observedAt = new Date().toISOString();
    const items = payload
      .filter((item) => resource !== 'issues' || !isObject(item) || !('pull_request' in item))
      .map((item) => workItem(item, observedAt));

    return items.some((item) => item === undefined)
      ? unknown('response was malformed')
      : items as GitHubWorkItem[];
  } catch (error) {
    return unknown(error instanceof DOMException && error.name === 'TimeoutError'
      ? 'request timed out'
      : 'request was unavailable');
  }
}

async function collectResource(
  config: GitHubConfig,
  resource: 'pulls' | 'issues',
): Promise<DataState<GitHubWorkItem[]>> {
  if (!validIdentifier(config.owner) || config.repositories.length === 0) {
    return unknown('repository configuration is missing');
  }

  try {
    const results = await Promise.all(
      config.repositories.map((repository) => fetchWorkItems(config, repository, resource)),
    );
    const items: GitHubWorkItem[] = [];
    for (const result of results) {
      if (!Array.isArray(result)) {
        return result;
      }
      items.push(...result);
    }

    const observedAt = new Date().toISOString();
    return items.length === 0
      ? { kind: 'empty', observedAt, source: SOURCE }
      : { kind: 'known', value: items, observedAt, source: SOURCE };
  } catch {
    return unknown('source was unavailable');
  }
}

export async function collectGitHubEvidence(config: GitHubConfig): Promise<GitHubEvidence> {
  try {
    const [pullRequests, issues] = await Promise.all([
      collectResource(config, 'pulls'),
      collectResource(config, 'issues'),
    ]);
    return { pullRequests, issues };
  } catch {
    return {
      pullRequests: unknown('source was unavailable'),
      issues: unknown('source was unavailable'),
    };
  }
}
