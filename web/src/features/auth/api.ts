// Auth API calls. login/register/logout hit the (unauthenticated) /auth routes
// and rely on the refresh cookie; fetchMe goes through authFetch so it carries
// the Bearer token and benefits from the refresh-once interceptor.

import { apiFetch } from '../../lib/api';
import { authFetch, setAccessToken } from '../../lib/authClient';

export interface User {
  id: string;
  email: string;
  displayName: string;
  createdAt: string;
}

interface LoginResponse {
  accessToken: string;
  user: User;
}

interface UserEnvelope {
  user: User;
}

const jsonHeaders = { 'Content-Type': 'application/json' };

export async function login(email: string, password: string): Promise<User> {
  const response = await apiFetch<LoginResponse>('/api/v1/auth/login', {
    method: 'POST',
    credentials: 'include',
    headers: jsonHeaders,
    body: JSON.stringify({ email, password }),
  });
  setAccessToken(response.accessToken);
  return response.user;
}

export async function register(
  email: string,
  password: string,
  displayName: string,
): Promise<User> {
  const response = await apiFetch<UserEnvelope>('/api/v1/auth/register', {
    method: 'POST',
    credentials: 'include',
    headers: jsonHeaders,
    body: JSON.stringify({ email, password, displayName }),
  });
  return response.user;
}

export async function logout(): Promise<void> {
  await apiFetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' });
  setAccessToken(null);
}

export async function fetchMe(): Promise<User> {
  const response = await authFetch<UserEnvelope>('/api/v1/me');
  return response.user;
}
