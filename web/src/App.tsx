import { Navigate, Route, Routes } from "react-router-dom";
import type { ReactNode } from "react";
import { AuthProvider, useAuth } from "./auth/AuthContext";
import { BottleSeaPage } from "./pages/BottleSeaPage";
import { BottleThreadPage } from "./pages/BottleThreadPage";
import { ChatListPage } from "./pages/ChatListPage";
import { ChatRoomPage } from "./pages/ChatRoomPage";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";

function Private({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="page pad">加载中…</div>;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/"
          element={
            <Private>
              <HomePage />
            </Private>
          }
        />
        <Route
          path="/chat"
          element={
            <Private>
              <ChatListPage />
            </Private>
          }
        />
        <Route
          path="/chat/:id"
          element={
            <Private>
              <ChatRoomPage />
            </Private>
          }
        />
        <Route
          path="/bottle"
          element={
            <Private>
              <BottleSeaPage />
            </Private>
          }
        />
        <Route
          path="/bottle/thread/:id"
          element={
            <Private>
              <BottleThreadPage />
            </Private>
          }
        />
      </Routes>
    </AuthProvider>
  );
}
