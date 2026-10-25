import { NavLink } from "react-router-dom";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";

/**
 * 移动端底部导航（QQ 式）：消息 / 联系人 / 漂流瓶。
 * 仅在列表页（/chat /friends /bottle）挂载；桌面端经 CSS 隐藏（B 类窄屏才显示）。
 */
export function BottomNav() {
  const totalUnread = useConversationsStore((s) =>
    s.list.reduce((n, c) => n + (c.muted ? 0 : c.unread), 0),
  );
  const pendingIn = useFriendsStore((s) => s.pending_in.length);

  return (
    <nav className="bottom-nav" aria-label="主导航">
      <NavLink to="/chat" end className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
        <span className="nav-icon" aria-hidden>
          <svg viewBox="0 0 24 24" width="22" height="22">
            <path
              d="M4 5h16a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H9l-4.2 3.4A.6.6 0 0 1 4 18.9V6a1 1 0 0 1 0-1Z"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinejoin="round"
            />
            <circle cx="9" cy="10.5" r="1" fill="currentColor" />
            <circle cx="13" cy="10.5" r="1" fill="currentColor" />
            <circle cx="17" cy="10.5" r="1" fill="currentColor" />
          </svg>
          {totalUnread > 0 && (
            <span className="nav-badge">{totalUnread > 99 ? "99+" : totalUnread}</span>
          )}
        </span>
        <span className="nav-label">消息</span>
      </NavLink>
      <NavLink to="/friends" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
        <span className="nav-icon" aria-hidden>
          <svg viewBox="0 0 24 24" width="22" height="22">
            <circle cx="10" cy="8" r="3.2" fill="none" stroke="currentColor" strokeWidth="1.7" />
            <path
              d="M4.5 19c.6-3.2 2.8-5 5.5-5s4.9 1.8 5.5 5"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
            />
            <path
              d="M17 7.5v5M14.5 10h5"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
            />
          </svg>
          {pendingIn > 0 && <span className="nav-badge">{pendingIn}</span>}
        </span>
        <span className="nav-label">联系人</span>
      </NavLink>
      <NavLink to="/bottle" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
        <span className="nav-icon" aria-hidden>
          <svg viewBox="0 0 24 24" width="22" height="22">
            <path
              d="M14.5 3.5 20.5 9.5M16 5l3 3M8.5 9.5c-2.6 1.4-4 4.2-4 7.3 0 1.6.3 2.9.8 3.7.8.3 2.1.5 3.7.5 3.1 0 5.9-1.4 7.3-4M8.5 9.5l6 6"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <circle cx="10.2" cy="13.8" r="2.6" fill="none" stroke="currentColor" strokeWidth="1.7" />
          </svg>
        </span>
        <span className="nav-label">漂流瓶</span>
      </NavLink>
    </nav>
  );
}
