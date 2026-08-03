import type {
  HiveEndpoints,
  OpsSnapshot,
  SnapshotConfig,
} from './contracts.js';
import { collectGitHubEvidence, githubAuthAvailable } from './sources/github.js';
import { collectHiveEvidence } from './sources/hive.js';

type Environment = Record<string, string | undefined>;

function runtimeEnvironment(): Environment {
  return (globalThis as { process?: { env?: Environment } }).process?.env ?? {};
}

function pathFromEnvironment(value: string | undefined): string | undefined {
  if (
    !value ||
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\') ||
    value.includes('?') ||
    value.includes('#') ||
    value.split('/').includes('..')
  ) {
    return undefined;
  }

  return value;
}

export function snapshotConfigFromEnvironment(
  environment: Environment = runtimeEnvironment(),
): SnapshotConfig {
  const endpoints: HiveEndpoints = {
    knowledgeExport: '/api/knowledge/export',
    connectivity: pathFromEnvironment(environment.HIVE_CONNECTIVITY_PATH),
    contributors: pathFromEnvironment(environment.HIVE_CONTRIBUTORS_PATH),
    actionableCount: pathFromEnvironment(environment.HIVE_ACTIONABLE_COUNT_PATH),
    provenance: pathFromEnvironment(environment.HIVE_PROVENANCE_PATH),
  };
  return {
    hive: {
      baseUrl: environment.HIVE_API_BASE_URL,
      endpoints,
    },
    github: {
      owner: environment.BLUEFIN_GITHUB_OWNER,
      repositories: (environment.BLUEFIN_GITHUB_REPOSITORIES ?? '')
        .split(',')
        .map((repository) => repository.trim())
        .filter(Boolean),
      tokenAvailable: githubAuthAvailable(environment),
    },
  };
}

export async function getSnapshot(config: SnapshotConfig): Promise<OpsSnapshot> {
  const [hive, github] = await Promise.all([
    collectHiveEvidence(config.hive),
    collectGitHubEvidence(config.github),
  ]);

  return {
    mode: hive.connectivity.kind === 'known' ? 'live' : 'manual',
    generatedAt: new Date().toISOString(),
    hive,
    github,
    githubAuthAvailable: config.github.tokenAvailable,
  };
}
