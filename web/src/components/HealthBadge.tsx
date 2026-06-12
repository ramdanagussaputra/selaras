import type { ApiHealth } from '../app/useApiHealth';

interface HealthBadgeProps {
  readonly health: ApiHealth;
}

const styles: Record<ApiHealth, string> = {
  checking: 'bg-slate-100 text-slate-600',
  ok: 'bg-emerald-100 text-emerald-700',
  degraded: 'bg-amber-100 text-amber-700',
  unreachable: 'bg-rose-100 text-rose-700',
};

const labels: Record<ApiHealth, string> = {
  checking: 'checking…',
  ok: 'API: ok',
  degraded: 'API: degraded',
  unreachable: 'API: unreachable',
};

export function HealthBadge({ health }: HealthBadgeProps) {
  return (
    <span className={`rounded-full px-3 py-1 text-xs font-medium ${styles[health]}`}>
      {labels[health]}
    </span>
  );
}
