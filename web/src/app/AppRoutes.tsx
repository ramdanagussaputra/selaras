import { Route, Routes } from 'react-router';

import { LoginPage } from '../features/auth/LoginPage';
import { BoardListPage } from '../features/board/BoardListPage';
import { BoardPage } from '../features/board/BoardPage';
import { Layout } from './Layout';

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<BoardListPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/boards/:id" element={<BoardPage />} />
      </Route>
    </Routes>
  );
}
