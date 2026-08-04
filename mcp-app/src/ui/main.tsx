import { type ReactNode, useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import type {
  DataState,
  EvidenceSource,
  GitHubEvidence,
  GitHubWorkItem,
  HiveConnectivity,
  HiveContributor,
  HiveEvidence,
  OpsSnapshot,
  PromptProvenance,
} from '../contracts.js';
import {
  EvidenceCell,
  ExternalLink,
  FactValue,
  LedgerPanel,
  StatusMark,
} from './components.js';
import './styles.css';

const SNAPSHOT_TOOL = 'get_ops_control_panel_snapshot';
const INITIAL_REASON = 'waiting for the evidence bridge';
const FACTORY_POLICY_URL =
  'https://github.com/projectbluefin/review/blob/main/image/config/local-agent-policy.md';
const HIVE_DASHBOARD_URL = 'https://hive.projectbluefin.io/';
const COMPLETION_EVIDENCE_UNAVAILABLE = unknown<string>(
  'hive',
  'completion evidence is unavailable from the current source model',
);

type JsonRpcResult = {
  content?: Array<{ type?: string; text?: string }>;
};

type JsonRpcMessage = {
  id?: number;
  result?: JsonRpcResult;
  error?: { message?: string };
};

function unknown<T>(source: EvidenceSource, reason: string): DataState<T> {
  return { kind: 'unknown', source, reason };
}

function unknownSnapshot(reason: string): OpsSnapshot {
  return {
    mode: 'manual',
    generatedAt: '',
    hive: {
      connectivity: unknown('hive', reason),
      contributors: unknown('hive', reason),
      actionableCount: unknown('hive', reason),
      knowledgeExportedAt: unknown('hive', reason),
      promptProvenance: unknown('hive', reason),
    },
    github: {
      pullRequests: unknown('github', reason),
      issues: unknown('github', reason),
    },
    githubAuthAvailable: false,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isEvidenceSource(value: unknown): value is EvidenceSource {
  return value === 'hive' || value === 'github';
}

function isDataState<T>(
  value: unknown,
  isValue: (candidate: unknown) => candidate is T,
): value is DataState<T> {
  if (!isRecord(value) || !isEvidenceSource(value.source)) {
    return false;
  }

  switch (value.kind) {
    case 'known':
      return typeof value.observedAt === 'string' && isValue(value.value);
    case 'empty':
      return typeof value.observedAt === 'string';
    case 'unknown':
      return typeof value.reason === 'string';
    case 'stale':
      return (
        typeof value.observedAt === 'string' &&
        typeof value.reason === 'string' &&
        isValue(value.value)
      );
    default:
      return false;
  }
}

function isHiveConnectivity(value: unknown): value is HiveConnectivity {
  return isRecord(value) && typeof value.endpoint === 'string';
}

function isHiveContributor(value: unknown): value is HiveContributor {
  return isRecord(value) && typeof value.id === 'string' && typeof value.label === 'string';
}

function isPromptProvenance(value: unknown): value is PromptProvenance {
  return isRecord(value) && typeof value.source === 'string' && typeof value.revision === 'string';
}

function isGitHubWorkItem(value: unknown): value is GitHubWorkItem {
  return (
    isRecord(value) &&
    typeof value.title === 'string' &&
    typeof value.number === 'number' &&
    typeof value.url === 'string' &&
    Array.isArray(value.labels) &&
    value.labels.every((label) => typeof label === 'string') &&
    (value.reviewStatus === 'review-requested' ||
      value.reviewStatus === 'not-requested' ||
      value.reviewStatus === 'unknown') &&
    typeof value.observedAt === 'string'
  );
}

function isArrayOf<T>(
  value: unknown,
  isItem: (candidate: unknown) => candidate is T,
): value is T[] {
  return Array.isArray(value) && value.every(isItem);
}

function isHiveEvidence(value: unknown): value is HiveEvidence {
  return (
    isRecord(value) &&
    isDataState(value.connectivity, isHiveConnectivity) &&
    isDataState(value.contributors, (contributors): contributors is HiveContributor[] =>
      isArrayOf(contributors, isHiveContributor),
    ) &&
    isDataState(value.actionableCount, (count): count is number => typeof count === 'number') &&
    isDataState(value.knowledgeExportedAt, (timestamp): timestamp is string => typeof timestamp === 'string') &&
    isDataState(value.promptProvenance, isPromptProvenance)
  );
}

function isGitHubEvidence(value: unknown): value is GitHubEvidence {
  return (
    isRecord(value) &&
    isDataState(value.pullRequests, (items): items is GitHubWorkItem[] =>
      isArrayOf(items, isGitHubWorkItem),
    ) &&
    isDataState(value.issues, (items): items is GitHubWorkItem[] => isArrayOf(items, isGitHubWorkItem))
  );
}

// The referrer of an inline-framed resource is the host document that framed
// it, so its origin is the only host identity available before the host
// speaks. Returns the wildcard only when that origin is opaque or absent.
function hostDocumentOrigin(): string {
  try {
    const { origin } = new URL(document.referrer);
    return origin && origin !== 'null' ? origin : '*';
  } catch {
    return '*';
  }
}

function isOpsSnapshot(value: unknown): value is OpsSnapshot {
  return (
    isRecord(value) &&
    (value.mode === 'live' || value.mode === 'manual') &&
    typeof value.generatedAt === 'string' &&
    isHiveEvidence(value.hive) &&
    isGitHubEvidence(value.github) &&
    typeof value.githubAuthAvailable === 'boolean'
  );
}

class HostBridge {
  private initialized = false;

  private nextId = 1;

  private readonly parent = window.parent;

  // Outbound target origin. The host frames this resource as inline HTML, so
  // this document's own origin is opaque and the host origin is not statically
  // knowable. Seed it from the referring host document, then narrow it to the
  // origin the browser reports on the first message the host actually sends. A
  // wildcard survives only where the host origin itself serialises to 'null',
  // which postMessage rejects as a target.
  private targetOrigin = hostDocumentOrigin();

  private readonly pending = new Map<
    number,
    { resolve: (result: JsonRpcResult) => void; reject: () => void }
  >();

  constructor() {
    window.addEventListener('message', (event: MessageEvent<JsonRpcMessage>) => {
      if (event.source !== this.parent) {
        return;
      }

      if (event.origin && event.origin !== 'null') {
        this.targetOrigin = event.origin;
      }

      if (typeof event.data?.id !== 'number') {
        return;
      }

      const pending = this.pending.get(event.data.id);
      if (!pending) {
        return;
      }

      this.pending.delete(event.data.id);
      if (event.data.error) {
        pending.reject();
        return;
      }

      pending.resolve(event.data.result ?? {});
    });
  }

  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    await this.request('ui/initialize', { appCapabilities: {} });
    this.notify('ui/notifications/initialized', {});
    this.initialized = true;
  }

  async getSnapshot(): Promise<OpsSnapshot> {
    const result = await this.request('tools/call', {
      name: SNAPSHOT_TOOL,
      arguments: {},
    });
    const text = result.content?.find((item) => item.type === 'text')?.text;

    if (!text) {
      throw new Error('Snapshot response was empty');
    }

    const snapshot: unknown = JSON.parse(text);
    if (!isOpsSnapshot(snapshot)) {
      throw new Error('Snapshot response was malformed');
    }

    return snapshot;
  }

  private notify(method: string, params: Record<string, never>): void {
    this.parent.postMessage({ jsonrpc: '2.0', method, params }, this.targetOrigin);
  }

  private request(method: string, params: Record<string, unknown>): Promise<JsonRpcResult> {
    const id = this.nextId++;

    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.parent.postMessage({ jsonrpc: '2.0', id, method, params }, this.targetOrigin);
    });
  }
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.valueOf())) {
    return 'Evidence pending';
  }

  return date.toLocaleString(undefined, {
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    month: 'short',
    timeZoneName: 'short',
    year: 'numeric',
  });
}

function formatKnowledgeAge(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) {
    return 'timestamp malformed';
  }

  const minutes = Math.max(0, Math.floor((Date.now() - date.valueOf()) / 60_000));
  if (minutes < 60) {
    return `${minutes} min old`;
  }

  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    return `${hours} hr old`;
  }

  return `${Math.floor(hours / 24)} d old`;
}

function observationEvidence<T>(state: DataState<T>): DataState<string> {
  switch (state.kind) {
    case 'known':
      return { ...state, value: state.observedAt };
    case 'empty':
      return state;
    case 'unknown':
      return state;
    case 'stale':
      return { ...state, value: state.observedAt };
  }
}

function driftEvidence(state: DataState<string>): DataState<string> {
  const value = 'not established by an export timestamp';
  switch (state.kind) {
    case 'known':
      return { kind: 'unknown', source: state.source, reason: value };
    case 'empty':
      return state;
    case 'unknown':
      return state;
    case 'stale':
      return { ...state, value };
  }
}

function promptValue(value: PromptProvenance): string {
  return `${value.source} @ ${value.revision}`;
}

function contributorValue(
  contributors: Array<{ id: string; label: string }>,
): ReactNode {
  return (
    <span className="contributor-list">
      {contributors.map((contributor) => (
        <span key={contributor.id}>
          {contributor.label} <span className="contributor-list__id">({contributor.id})</span>
        </span>
      ))}
    </span>
  );
}

function pullRequestValue(items: GitHubWorkItem[]): ReactNode {
  return (
    <span className="work-item-list">
      {items.map((item) => (
        <span className="work-item" key={item.url}>
          <ExternalLink
            href={item.url}
            label={`Open pull request ${item.number}: ${item.title} in a new tab`}
          >
            PR #{item.number}
          </ExternalLink>
          <span className="work-item__title">{item.title}</span>
          <span className="work-item__review">review: {item.reviewStatus}</span>
          <span className="work-item__stamp">{formatTimestamp(item.observedAt)}</span>
        </span>
      ))}
    </span>
  );
}

function issueValue(items: GitHubWorkItem[]): ReactNode {
  return (
    <span className="work-item-list">
      {items.map((item) => (
        <span className="work-item" key={item.url}>
          <ExternalLink
            href={item.url}
            label={`Open issue ${item.number}: ${item.title} in a new tab`}
          >
            Issue #{item.number}
          </ExternalLink>
          <span className="work-item__title">{item.title}</span>
          <span className="work-item__stamp">{formatTimestamp(item.observedAt)}</span>
        </span>
      ))}
    </span>
  );
}

function OpsControlPanel(): ReactNode {
  const bridge = useRef<HostBridge | null>(null);
  const [snapshot, setSnapshot] = useState<OpsSnapshot>(() => unknownSnapshot(INITIAL_REASON));
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [bridgeReady, setBridgeReady] = useState(false);

  const refreshEvidence = async (): Promise<void> => {
    if (isRefreshing || !bridge.current) {
      return;
    }

    setIsRefreshing(true);
    try {
      setSnapshot(await bridge.current.getSnapshot());
    } catch {
      setSnapshot(unknownSnapshot('the evidence request did not return a usable snapshot'));
    } finally {
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    const connect = async (): Promise<void> => {
      try {
        bridge.current = new HostBridge();
        await bridge.current.initialize();
        setBridgeReady(true);
        await refreshEvidence();
      } catch {
        setSnapshot(unknownSnapshot('the evidence bridge did not initialize'));
      }
    };

    void connect();
  }, []);

  const modeLabel = bridgeReady
    ? snapshot.mode === 'live'
      ? 'Live evidence'
      : 'Manual Mode'
    : 'Bridge pending';
  const modeState = bridgeReady
    ? snapshot.hive.connectivity
    : unknown('hive', INITIAL_REASON);
  const manualState = !bridgeReady
    ? 'is-pending'
    : snapshot.mode === 'manual'
      ? 'is-active'
      : 'is-standby';
  const manualEyebrow = !bridgeReady
    ? 'Fallback pending'
    : snapshot.mode === 'manual'
      ? 'Fallback active'
      : 'Fallback standby';

  return (
    <main aria-labelledby="ledger-title" className="tactical-ledger">
      <header className="ledger-header">
        <div>
          <p className="eyebrow">Bluefin contributor evidence / read-only</p>
          <h1 id="ledger-title">Tactical Ledger</h1>
          <p className="ledger-header__subtitle">
            Orient from live Hive and GitHub evidence. This view does not direct work.
          </p>
        </div>
        <div className="ledger-header__controls">
          <div className="mode-readout">
            <StatusMark state={modeState} />
            <span>{modeLabel}</span>
          </div>
          <dl className="snapshot-time">
            <dt>Snapshot</dt>
            <dd>{formatTimestamp(snapshot.generatedAt)}</dd>
          </dl>
          <button
            disabled={isRefreshing || !bridgeReady}
            onClick={() => void refreshEvidence()}
            type="button"
          >
            {isRefreshing ? 'Refreshing evidence…' : 'Refresh all evidence'}
          </button>
        </div>
      </header>

      <div aria-label="Evidence signals" className="signal-strip">
        <EvidenceCell label="Hive connectivity / endpoint" state={snapshot.hive.connectivity}>
          <FactValue state={snapshot.hive.connectivity} render={(value) => value.endpoint} />
        </EvidenceCell>
        <EvidenceCell label="Active contributors" state={snapshot.hive.contributors}>
          <FactValue state={snapshot.hive.contributors} render={contributorValue} />
        </EvidenceCell>
        <EvidenceCell label="Actionable count" state={snapshot.hive.actionableCount}>
          <FactValue state={snapshot.hive.actionableCount} />
        </EvidenceCell>
        <EvidenceCell label="Knowledge age" state={snapshot.hive.knowledgeExportedAt}>
          <FactValue state={snapshot.hive.knowledgeExportedAt} render={formatKnowledgeAge} />
        </EvidenceCell>
      </div>

      <div className="ledger-grid">
        <LedgerPanel eyebrow="Hive source" title="Connection & knowledge">
          <div className="panel-grid">
            <EvidenceCell label="Last contact" state={snapshot.hive.connectivity}>
              <FactValue
                state={observationEvidence(snapshot.hive.connectivity)}
                render={formatTimestamp}
              />
            </EvidenceCell>
            <EvidenceCell
              label="Skill / doc drift"
              stampState={snapshot.hive.knowledgeExportedAt}
              state={driftEvidence(snapshot.hive.knowledgeExportedAt)}
            >
              <FactValue state={driftEvidence(snapshot.hive.knowledgeExportedAt)} />
            </EvidenceCell>
          </div>
        </LedgerPanel>

        <LedgerPanel eyebrow="Hive source" title="Prompt provenance">
          <EvidenceCell label="Prompt source & revision" state={snapshot.hive.promptProvenance}>
            <FactValue state={snapshot.hive.promptProvenance} render={promptValue} />
          </EvidenceCell>
        </LedgerPanel>

        <LedgerPanel eyebrow="GitHub source" title="Pull requests & review">
          <EvidenceCell label="Open PR / review links" state={snapshot.github.pullRequests}>
            <FactValue state={snapshot.github.pullRequests} render={pullRequestValue} />
          </EvidenceCell>
        </LedgerPanel>

        <LedgerPanel eyebrow="Reconciliation" title="Completion evidence unavailable">
          <div className="reconciliation">
            <EvidenceCell label="Completion evidence" state={COMPLETION_EVIDENCE_UNAVAILABLE}>
              <FactValue state={COMPLETION_EVIDENCE_UNAVAILABLE} />
            </EvidenceCell>
            <EvidenceCell label="GitHub truth" state={snapshot.github.pullRequests}>
              <FactValue
                state={snapshot.github.pullRequests}
                render={(items) => `${items.length} open pull request${items.length === 1 ? '' : 's'} visible`}
              />
            </EvidenceCell>
          </div>
          <p className="reconciliation__note">
            GitHub merge and review state are distinct GitHub facts. VM guests lack GitHub
            identity mapping, so the current source model cannot establish completion.
          </p>
        </LedgerPanel>

        <LedgerPanel
          className={`manual-mode manual-mode--${manualState}`}
          eyebrow={manualEyebrow}
          title="Manual Mode"
        >
          <p className="manual-mode__note">
            Canonical issue links remain available when Hive telemetry cannot orient a review.
            This panel does not select, rank, or assign work.
          </p>
          <EvidenceCell label="GitHub issue links" state={snapshot.github.issues}>
            <FactValue state={snapshot.github.issues} render={issueValue} />
          </EvidenceCell>
        </LedgerPanel>
      </div>

      <footer className="ledger-footer">
        <span>Evidence rail records source and observation time for every live fact.</span>
        <nav aria-label="Reference links">
          <ExternalLink
            href={FACTORY_POLICY_URL}
            label="Open Bluefin factory policy in a new tab"
          >
            Bluefin factory policy
          </ExternalLink>
          <ExternalLink
            href={HIVE_DASHBOARD_URL}
            label="Open hosted Hive dashboard in a new tab"
          >
            Hosted Hive dashboard
          </ExternalLink>
        </nav>
      </footer>
    </main>
  );
}

const host = document.getElementById('root');

if (host) {
  createRoot(host).render(<OpsControlPanel />);
}
