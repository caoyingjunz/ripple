import { Link } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export function HomePage() {
  const { user, logout } = useAuth();
  return (
    <div className="page home">
      <header className="topbar">
        <span className="brand-sm">Ripple</span>
        <div className="userchip">
          <span className="dot" style={{ background: user?.avatar_color }} />
          {user?.display_name}
          <button type="button" className="linkish" onClick={logout}>
            退出
          </button>
        </div>
      </header>
      <main className="home-hero">
        <h1 className="brand-hero">Ripple</h1>
        <p className="lede">聊天与漂流瓶，同等重要的两种相遇方式。</p>
        <div className="dual">
          <Link className="portal chat-portal" to="/chat">
            <span className="portal-title">聊天</span>
            <span className="portal-desc">与认识的人继续对话，支持文字与图片。</span>
          </Link>
          <Link className="portal bottle-portal" to="/bottle">
            <span className="portal-title">漂流瓶</span>
            <span className="portal-desc">把文字扔进海里，匿名来回，再决定是否公开身份。</span>
          </Link>
        </div>
      </main>
    </div>
  );
}
