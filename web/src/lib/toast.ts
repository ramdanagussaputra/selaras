// Minimal toast store built on an external store, so any module (e.g. a mutation
// onError) can raise a toast without prop-drilling and the <Toaster /> renders
// them. Toasts auto-dismiss; this is intentionally tiny — no dependency needed.

import { useSyncExternalStore } from 'react';

export interface Toast {
  id: number;
  message: string;
}

let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) {
    listener();
  }
}

export function showToast(message: string): void {
  const toast: Toast = { id: nextId++, message };
  toasts = [...toasts, toast];
  emit();
  setTimeout(() => dismissToast(toast.id), 4000);
}

export function dismissToast(toastId: number): void {
  toasts = toasts.filter((toast) => toast.id !== toastId);
  emit();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot(): Toast[] {
  return toasts;
}

export function useToasts(): Toast[] {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
