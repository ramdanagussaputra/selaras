import { useQuery } from '@tanstack/react-query';

import { ApiError, apiFetch } from '../lib/api';

export type ApiHealth = 'checking' | 'ok' | 'degraded' | 'unreachable';

interface HealthzResponse {
  status: string;
}

// Polls GET /healthz through the lib/api wrapper: "ok" means API + DB are up,
// "degraded" means the API answered 503 (DB down), "unreachable" means no API.
export function useApiHealth(): ApiHealth {
  const { data, isPending } = useQuery({
    queryKey: ['healthz'],
    queryFn: async (): Promise<ApiHealth> => {
      try {
        const body = await apiFetch<HealthzResponse>('/healthz');
        return body.status === 'ok' ? 'ok' : 'degraded';
      } catch (err) {
        if (err instanceof ApiError && err.status === 503) {
          return 'degraded';
        }
        return 'unreachable';
      }
    },
    refetchInterval: 30_000,
  });

  return isPending ? 'checking' : (data ?? 'unreachable');
}
