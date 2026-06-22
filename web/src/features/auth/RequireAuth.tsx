import { Navigate, Outlet } from 'react-router';

import { useAuth } from './authContext';

// RequireAuth gates protected routes: it waits for the bootstrap refresh, then
// renders the child routes when authenticated or redirects to /login otherwise.
export function RequireAuth() {
  const { status } = useAuth();

  if (status === 'loading') {
    return <p className="p-6 text-slate-600">Loading…</p>;
  }

  if (status === 'unauthenticated') {
    return <Navigate to="/login" replace />;
  }

  return <Outlet />;
}
