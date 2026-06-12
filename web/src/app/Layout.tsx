import { Link, Outlet } from 'react-router';

import { HealthBadge } from '../components/HealthBadge';
import { useApiHealth } from './useApiHealth';

// App shell: header with nav + live API health, content via <Outlet />.
export function Layout() {
  const health = useApiHealth();

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3">
        <nav className="flex items-center gap-4">
          <Link to="/" className="text-lg font-semibold text-sky-600">
            Selaras
          </Link>
          <Link to="/login" className="text-sm text-slate-600 hover:text-slate-900">
            Login
          </Link>
        </nav>
        <HealthBadge health={health} />
      </header>
      <main className="mx-auto max-w-5xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  );
}
