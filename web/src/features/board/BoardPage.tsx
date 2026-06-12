import { useParams } from 'react-router';

// Placeholder route — the real board view lands in M3.
export function BoardPage() {
  const { id } = useParams<{ id: string }>();

  return (
    <section>
      <h1 className="text-2xl font-semibold">Board {id}</h1>
      <p className="mt-2 text-slate-600">The kanban view arrives in milestone M3.</p>
    </section>
  );
}
