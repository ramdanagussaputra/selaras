import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import { ApiError } from '../../lib/api';
import { AuthContext, type AuthContextValue } from './authContext';
import { LoginPage } from './LoginPage';

function renderLogin(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
  const value: AuthContextValue = {
    user: null,
    status: 'unauthenticated',
    login: vi.fn().mockResolvedValue(undefined),
    register: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };

  render(
    <MemoryRouter>
      <AuthContext.Provider value={value}>
        <LoginPage />
      </AuthContext.Provider>
    </MemoryRouter>,
  );
  return value;
}

function fillAndSubmit(): void {
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'user@example.com' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'longenough' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
}

describe('LoginPage', () => {
  it('renders the sign-in form', () => {
    renderLogin();
    expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
  });

  it('submits the entered credentials to the login action', () => {
    const value = renderLogin();
    fillAndSubmit();
    expect(value.login).toHaveBeenCalledWith('user@example.com', 'longenough');
  });

  it('shows the server message when login fails', async () => {
    const login = vi.fn().mockRejectedValue(
      new ApiError(401, { error: { code: 'INVALID_CREDENTIALS', message: 'invalid email or password' } }),
    );
    renderLogin({ login });

    fillAndSubmit();

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid email or password');
  });
});
