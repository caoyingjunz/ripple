import { useEffect } from "react";
import { Link } from "react-router-dom";
import { Avatar } from "../components/Avatar";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";
import { useSessionStore } from "../stores/session";

export function HomePage() {
  const user = useSessionStore((s) => s.user);
  const logout = useSessionStore((s) => s.logout);
  const totalUnread = useConversationsStore((s) =>
    s.list.reduce((n, c) => n + (c.muted ? 0 : c.unread), 0),
  );
  const pendingInCount = useFriendsStore((s) => s.pending_in.length);

  useEffect(() => {
    void useConversationsStore.getState().refreshList();
    if (!useFriendsStore.getState().loaded) void useFriendsStore.getState().refresh();
  }, []);

  return (
    <div className="page home">
      <header className="topbar">
        <Link to="/profile" className="topbar-me" title="我的资料">
          {user && <Avatar user={user} size={30} />}
          <span className="topbar-me-name">{user?.display_name}</span>
        </Link>
        <span />
        <button type="button" className="topbar-plus" title="退出登录" onClick={logout}>
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden>
            <path
              d="M14 4H6a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h8M10.5 12H20m0 0-3.2-3.2M20 12l-3.2 3.2"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </button>
      </header>
      <main className="home-hero">
        <h1 className="brand-hero">Ripple</h1>
        <p className="lede">聊天、好友与漂流瓶——三种相遇方式，一样重要。</p>
        <div className="tri">
          <Link className="portal chat-portal" to="/chat">
            <div className="portal-row">
              <span className="portal-title">聊天</span>
              {totalUnread > 0 && (
                <span className="portal-badge">{totalUnread > 99 ? "99+" : totalUnread}</span>
              )}
            </div>
            <span className="portal-desc">会话列表：单聊与群聊，文字与图片。</span>
          </Link>
          <Link className="portal friends-portal" to="/friends">
            <div className="portal-row">
              <span className="portal-title">好友</span>
              {pendingInCount > 0 && <span className="portal-badge">{pendingInCount}</span>}
            </div>
            <span className="portal-desc">搜索用户、处理申请，或从好友发起聊天。</span>
          </Link>
          <Link className="portal bottle-portal" to="/bottle">
            <div className="portal-row">
              <span className="portal-title">漂流瓶</span>
            </div>
            <span className="portal-desc">匿名来回六轮，再决定是否公开身份。</span>
          </Link>
        </div>
      </main>
    </div>
  );
}
