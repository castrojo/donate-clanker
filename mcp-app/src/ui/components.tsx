import type { ReactNode } from 'react';
import type { DataState, EvidenceSource } from '../contracts.js';

type StatusMarkProps = {
  state: DataState<unknown>;
};

type EvidenceCellProps = {
  label: string;
  state: DataState<unknown>;
  stampState?: DataState<unknown>;
  children: ReactNode;
  className?: string;
};

type LedgerPanelProps = {
  title: string;
  eyebrow: string;
  children: ReactNode;
  className?: string;
};

type FactValueProps<T> = {
  state: DataState<T>;
  render?: (value: T) => ReactNode;
};

type ExternalLinkProps = {
  href: string;
  label: string;
  children: ReactNode;
};

const statusLabels = {
  known: 'Confirmed',
  empty: 'None reported',
  unknown: 'Unknown',
  stale: 'Stale',
} as const;

function formatObservedAt(observedAt: string | undefined): string {
  if (!observedAt) {
    return 'no observation';
  }

  const date = new Date(observedAt);
  if (Number.isNaN(date.valueOf())) {
    return 'invalid timestamp';
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

function sourceLabel(source: EvidenceSource): string {
  return source === 'hive' ? 'Hive' : 'GitHub';
}

export function StatusMark({ state }: StatusMarkProps): ReactNode {
  const label = statusLabels[state.kind];

  return (
    <span
      aria-label={`Evidence status: ${label}`}
      className={`status-mark status-mark--${state.kind}`}
    >
      <span aria-hidden="true" className="status-mark__shape" />
      <span>{label}</span>
    </span>
  );
}

export function EvidenceCell({
  label,
  state,
  stampState = state,
  children,
  className = '',
}: EvidenceCellProps): ReactNode {
  const observedAt = stampState.kind === 'unknown' ? undefined : stampState.observedAt;

  return (
    <div className={`evidence-cell evidence-cell--${state.kind} ${className}`.trim()}>
      <div className="evidence-cell__body">
        <div className="evidence-cell__label-row">
          <span className="evidence-cell__label">{label}</span>
          <StatusMark state={state} />
        </div>
        <div className="evidence-cell__value">{children}</div>
      </div>
      <span
        aria-label={`Source ${sourceLabel(stampState.source)}; observed ${formatObservedAt(observedAt)}`}
        className="source-stamp"
      >
        <span>{sourceLabel(stampState.source)}</span>
        <span>{formatObservedAt(observedAt)}</span>
      </span>
    </div>
  );
}

export function LedgerPanel({
  title,
  eyebrow,
  children,
  className = '',
}: LedgerPanelProps): ReactNode {
  const headingId = `panel-${title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`;

  return (
    <section aria-labelledby={headingId} className={`ledger-panel ${className}`.trim()}>
      <header className="ledger-panel__header">
        <p className="ledger-panel__eyebrow">{eyebrow}</p>
        <h2 id={headingId}>{title}</h2>
      </header>
      <div className="ledger-panel__content">{children}</div>
    </section>
  );
}

export function FactValue<T>({ state, render = String }: FactValueProps<T>): ReactNode {
  switch (state.kind) {
    case 'known':
      return <>{render(state.value)}</>;
    case 'empty':
      return <>None reported</>;
    case 'unknown':
      return <>Unknown — {state.reason}</>;
    case 'stale':
      return <>Stale — {render(state.value)}</>;
  }
}

export function ExternalLink({ href, label, children }: ExternalLinkProps): ReactNode {
  return (
    <a aria-label={label} href={href} rel="noreferrer" target="_blank">
      {children}
    </a>
  );
}
