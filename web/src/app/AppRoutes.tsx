import { Route, Routes } from 'react-router';

import { LoginPage } from '../features/auth/LoginPage';
import { RegisterPage } from '../features/auth/RegisterPage';
import { RequireAuth } from '../features/auth/RequireAuth';
import { BoardListPage } from '../features/board/BoardListPage';
import { BoardPage } from '../features/board/BoardPage';
import { Layout } from './Layout';

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route element={<RequireAuth />}>
          <Route path="/" element={<BoardListPage />} />
          <Route path="/boards/:id" element={<BoardPage />} />
        </Route>
      </Route>
    </Routes>
  );
}
