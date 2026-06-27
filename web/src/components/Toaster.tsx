import { dismissToast, useToasts } from '../lib/toast';

// Toaster renders the active toasts in a fixed corner stack. Each is dismissible
// and also clears itself on a timer (see lib/toast).
export function Toaster() {
  const toasts = useToasts();
  if (toasts.length === 0) {
    return null;
  }

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2" role="status" aria-live="polite">
      {toasts.map((toast) => (
        <button
          key={toast.id}
          type="button"
          onClick={() => dismissToast(toast.id)}
          className="max-w-sm rounded-md bg-slate-900 px-4 py-2 text-left text-sm text-white shadow-lg"
        >
          {toast.message}
        </button>
      ))}
    </div>
  );
}
