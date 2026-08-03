export type EvidenceSource = 'hive' | 'github';

export type DataState<T> =
  | { kind: 'known'; value: T; observedAt: string; source: EvidenceSource }
  | { kind: 'empty'; observedAt: string; source: EvidenceSource }
  | { kind: 'unknown'; source: EvidenceSource; reason: string }
  | {
      kind: 'stale';
      value: T;
      observedAt: string;
      source: EvidenceSource;
      reason: string;
    };

export interface HiveConnectivity {
  endpoint: string;
}

export interface HiveContributor {
  id: string;
  label: string;
}

export interface PromptProvenance {
  source: string;
  revision: string;
}

export interface HiveEvidence {
  connectivity: DataState<HiveConnectivity>;
  contributors: DataState<HiveContributor[]>;
  actionableCount: DataState<number>;
  knowledgeExportedAt: DataState<string>;
  promptProvenance: DataState<PromptProvenance>;
}

export type GitHubReviewStatus =
  | 'review-requested'
  | 'not-requested'
  | 'unknown';

export interface GitHubWorkItem {
  title: string;
  number: number;
  url: string;
  labels: string[];
  reviewStatus: GitHubReviewStatus;
  observedAt: string;
}

export interface GitHubEvidence {
  pullRequests: DataState<GitHubWorkItem[]>;
  issues: DataState<GitHubWorkItem[]>;
}

export interface HiveEndpoints {
  knowledgeExport: '/api/knowledge/export';
  connectivity?: string;
  contributors?: string;
  actionableCount?: string;
  provenance?: string;
}

export interface HiveConfig {
  baseUrl?: string;
  endpoints: HiveEndpoints;
}

export interface GitHubConfig {
  owner?: string;
  repositories: string[];
  tokenAvailable: boolean;
}

export interface SnapshotConfig {
  hive: HiveConfig;
  github: GitHubConfig;
}

export interface OpsSnapshot {
  mode: 'live' | 'manual';
  generatedAt: string;
  hive: HiveEvidence;
  github: GitHubEvidence;
  githubAuthAvailable: boolean;
}
