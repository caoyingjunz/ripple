import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import type { ReactNode } from "react";
import { BottomNav } from "./components/BottomNav";
import { useSessionStore } from "./stores/session";
import { BottleSeaPage } from "./pages/BottleSeaPage";
import { BottleThreadPage } from "./pages/BottleThreadPage";
import { ChatListPage } from "./pages/ChatListPage";
import { ChatRoomPage } from "./pages/ChatRoomPage";
import { ConversationInfoPage } from "./pages/ConversationInfoPage";
import { FriendsPage } from "./pages/FriendsPage";
import { GroupCreatePage } from "./pages/GroupCreatePage";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";
import { ProfilePage } from "./pages/ProfilePage";
import { RegisterPage } from "./pages/RegisterPage";

function Private({ children }: { children: ReactNode }) {
  const user = useSessionStore((s) => s.user);
  const loading = useSessionStore((s) => s.loading);
  if (loading) return <div className="page pad">加载中…</div>;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

/** 底部导航仅在三个列表页显示（聊天室/瓶子对话等详情页不显示）。 */
const NAV_PATHS = new Set(["/chat", "/friends", "/bottle"]);

export default function App() {
  const { pathname } = useLocation();
  const showNav = NAV_PATHS.has(pathname);
  return (
    <>
      <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
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
        path="/friends"
        element={
          <Private>
            <FriendsPage />
          </Private>
        }
      />
      <Route
        path="/group/create"
        element={
          <Private>
            <GroupCreatePage />
          </Private>
        }
      />
      <Route
        path="/conversation/:id/info"
        element={
          <Private>
            <ConversationInfoPage />
          </Private>
        }
      />
      <Route
        path="/profile"
        element={
          <Private>
            <ProfilePage />
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
      <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      {showNav && <BottomNav />}
    </>
  );
}
