import type {
  DataState,
  HiveConfig,
  HiveConnectivity,
  HiveContributor,
  HiveEvidence,
  PromptProvenance,
} from '../contracts.js';

const SOURCE = 'hive' as const;
const REQUEST_TIMEOUT_MS = 8_000;

type JsonObject = Record<string, unknown>;

function unknown<T>(reason: string): DataState<T> {
  return { kind: 'unknown', source: SOURCE, reason };
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function configuredUrl(baseUrl: string | undefined, path: string | undefined): URL | undefined {
  if (
    !baseUrl ||
    !path ||
    !path.startsWith('/') ||
    path.startsWith('//') ||
    path.includes('\\') ||
    path.includes('?') ||
    path.includes('#') ||
    path.split('/').includes('..')
  ) {
    return undefined;
  }

  try {
    const base = new URL(baseUrl);
    if (
      !['http:', 'https:'].includes(base.protocol) ||
      base.username ||
      base.password
    ) {
      return undefined;
    }

    return new URL(path, base);
  } catch {
    return undefined;
  }
}

async function requestJson(path: string | undefined, config: HiveConfig): Promise<
  { value: unknown; observedAt: string } | DataState<never>
> {
  const url = configuredUrl(config.baseUrl, path);
  if (!url) {
    return unknown('endpoint is not configured');
  }

  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
    if (!response.ok) {
      return unknown('request was unavailable');
    }

    try {
      return { value: await response.json(), observedAt: new Date().toISOString() };
    } catch {
      return unknown('response was malformed');
    }
  } catch (error) {
    return unknown(error instanceof DOMException && error.name === 'TimeoutError'
      ? 'request timed out'
      : 'request was unavailable');
  }
}

async function requestKnowledgeTimestamp(
  config: HiveConfig,
): Promise<DataState<string>> {
  const url = configuredUrl(config.baseUrl, config.endpoints.knowledgeExport);
  if (!url) {
    return unknown('endpoint is not configured');
  }

  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
    if (!response.ok) {
      return unknown('request was unavailable');
    }

    const timestamp = response.headers.get('last-modified');
    if (!timestamp || Number.isNaN(Date.parse(timestamp))) {
      return unknown('response did not provide a knowledge timestamp');
    }

    return {
      kind: 'known',
      value: new Date(timestamp).toISOString(),
      observedAt: new Date().toISOString(),
      source: SOURCE,
    };
  } catch (error) {
    return unknown(error instanceof DOMException && error.name === 'TimeoutError'
      ? 'request timed out'
      : 'request was unavailable');
  }
}

function validConnectivity(
  response: { value: unknown; observedAt: string } | DataState<never>,
  path: string | undefined,
): DataState<HiveConnectivity> {
  if ('kind' in response) {
    return response;
  }

  if (!isObject(response.value) || response.value.connected !== true || !path) {
    return unknown('response was unsupported');
  }

  return {
    kind: 'known',
    value: { endpoint: path },
    observedAt: response.observedAt,
    source: SOURCE,
  };
}

function validContributors(
  response: { value: unknown; observedAt: string } | DataState<never>,
): DataState<HiveContributor[]> {
  if ('kind' in response) {
    return response;
  }

  if (!isObject(response.value) || !Array.isArray(response.value.contributors)) {
    return unknown('response was unsupported');
  }

  const contributors = response.value.contributors.map((contributor) => {
    if (!isObject(contributor) || typeof contributor.id !== 'string' || typeof contributor.label !== 'string') {
      return undefined;
    }
    return { id: contributor.id, label: contributor.label };
  });

  if (contributors.some((contributor) => contributor === undefined)) {
    return unknown('response was malformed');
  }

  return contributors.length === 0
    ? { kind: 'empty', observedAt: response.observedAt, source: SOURCE }
    : {
        kind: 'known',
        value: contributors as HiveContributor[],
        observedAt: response.observedAt,
        source: SOURCE,
      };
}

function validCount(
  response: { value: unknown; observedAt: string } | DataState<never>,
): DataState<number> {
  if ('kind' in response) {
    return response;
  }

  if (
    !isObject(response.value) ||
    typeof response.value.count !== 'number' ||
    !Number.isSafeInteger(response.value.count) ||
    response.value.count < 0
  ) {
    return unknown('response was unsupported');
  }

  return {
    kind: 'known',
    value: response.value.count,
    observedAt: response.observedAt,
    source: SOURCE,
  };
}

function validProvenance(
  response: { value: unknown; observedAt: string } | DataState<never>,
): DataState<PromptProvenance> {
  if ('kind' in response) {
    return response;
  }

  if (
    !isObject(response.value) ||
    typeof response.value.source !== 'string' ||
    typeof response.value.revision !== 'string'
  ) {
    return unknown('response was unsupported');
  }

  return {
    kind: 'known',
    value: { source: response.value.source, revision: response.value.revision },
    observedAt: response.observedAt,
    source: SOURCE,
  };
}

export async function collectHiveEvidence(config: HiveConfig): Promise<HiveEvidence> {
  try {
    const [connectivity, contributors, actionableCount, knowledgeExportedAt, promptProvenance] =
      await Promise.all([
        requestJson(config.endpoints.connectivity, config).then((response) =>
          validConnectivity(response, config.endpoints.connectivity),
        ),
        requestJson(config.endpoints.contributors, config).then(validContributors),
        requestJson(config.endpoints.actionableCount, config).then(validCount),
        requestKnowledgeTimestamp(config),
        requestJson(config.endpoints.provenance, config).then(validProvenance),
      ]);

    return {
      connectivity,
      contributors,
      actionableCount,
      knowledgeExportedAt,
      promptProvenance,
    };
  } catch {
    return {
      connectivity: unknown('source was unavailable'),
      contributors: unknown('source was unavailable'),
      actionableCount: unknown('source was unavailable'),
      knowledgeExportedAt: unknown('source was unavailable'),
      promptProvenance: unknown('source was unavailable'),
    };
  }
}
