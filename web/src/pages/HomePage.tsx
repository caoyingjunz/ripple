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
        <span className="brand-sm">Ripple</span>
        <div className="userchip">
          {user && <Avatar user={user} size={28} />}
          <Link to="/profile" className="linkish">
            {user?.display_name}
          </Link>
          <button type="button" className="linkish" onClick={logout}>
            退出
          </button>
        </div>
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
