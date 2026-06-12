import { QueryClient } from '@tanstack/react-query';

// Single QueryClient instance: TanStack Query is the only owner of server
// state (PRD state taxonomy).
export const queryClient = new QueryClient();
