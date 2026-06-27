import { Link, Outlet, useNavigate } from 'react-router';

import { HealthBadge } from '../components/HealthBadge';
import { Toaster } from '../components/Toaster';
import { useAuth } from '../features/auth/authContext';
import { useApiHealth } from './useApiHealth';

// App shell: header with nav, the live API health badge, and the auth controls
// (user + log out when signed in, a Login link otherwise). Content via <Outlet />.
export function Layout() {
  const health = useApiHealth();
  const { user, status, logout } = useAuth();
  const navigate = useNavigate();

  function handleLogout() {
    void (async () => {
      await logout();
      navigate('/login');
    })();
  }

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3">
        <nav className="flex items-center gap-4">
          <Link to="/" className="text-lg font-semibold text-sky-600">
            Selaras
          </Link>
        </nav>
        <div className="flex items-center gap-4">
          <HealthBadge health={health} />
          {status === 'authenticated' && user ? (
            <>
              <span className="text-sm text-slate-600">{user.displayName}</span>
              <button
                type="button"
                onClick={handleLogout}
                className="text-sm text-slate-600 hover:text-slate-900"
              >
                Log out
              </button>
            </>
          ) : null}
          {status === 'unauthenticated' ? (
            <Link to="/login" className="text-sm text-slate-600 hover:text-slate-900">
              Login
            </Link>
          ) : null}
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-8">
        <Outlet />
      </main>
      <Toaster />
    </div>
  );
}
