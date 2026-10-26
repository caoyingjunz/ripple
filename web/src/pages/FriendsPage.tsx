import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { Avatar } from "../components/Avatar";
import type { User, UserSearchItem } from "../api/types";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";
import { usePresenceStore } from "../stores/presence";
import { useSessionStore } from "../stores/session";

type Tab = "friends" | "requests" | "blocked";

export function FriendsPage() {
  const nav = useNavigate();
  const me = useSessionStore((s) => s.user);
  const searchRef = useRef<HTMLInputElement>(null);
  const friends = useFriendsStore((s) => s.friends);
  const pendingIn = useFriendsStore((s) => s.pending_in);
  const pendingOut = useFriendsStore((s) => s.pending_out);
  const blocked = useFriendsStore((s) => s.blocked);
  const loaded = useFriendsStore((s) => s.loaded);

  const [tab, setTab] = useState<Tab>("friends");
  const [q, setQ] = useState("");
  const [results, setResults] = useState<UserSearchItem[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!loaded) void useFriendsStore.getState().refresh();
  }, []);

  function note(text: string) {
    setMsg(text);
    setError("");
  }

  async function search() {
    const kw = q.trim();
    if (!kw) return;
    setSearching(true);
    setError("");
    try {
      const r = await api.searchUsers(kw);
      usePresenceStore.getState().applyUsers(r.users.map((x) => x.user));
      setResults(r.users);
    } catch (err) {
      setError(err instanceof Error ? err.message : "搜索失败");
    } finally {
      setSearching(false);
    }
  }

  async function afterAction(rerunSearch: boolean) {
    await useFriendsStore.getState().refresh();
    if (rerunSearch) await search();
  }

  async function addFriend(uid: string) {
    try {
      const r = await api.addFriend(uid);
      note(r.status === "accepted" ? "对方也申请了你，已直接成为好友" : "申请已发送");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送申请失败");
    }
  }

  function findPendingIn(uid: string): string | null {
    const req = useFriendsStore.getState().pending_in.find((r) => r.user.id === uid);
    return req?.id ?? null;
  }

  async function acceptFromSearch(uid: string) {
    let reqId = findPendingIn(uid);
    if (!reqId) {
      await useFriendsStore.getState().refresh();
      reqId = findPendingIn(uid);
    }
    if (!reqId) {
      setError("找不到对应的申请记录，请刷新后重试");
      return;
    }
    await acceptReq(reqId);
  }

  async function acceptReq(reqId: string) {
    try {
      await api.acceptFriend(reqId);
      note("已添加好友");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function declineFromSearch(uid: string) {
    let reqId = findPendingIn(uid);
    if (!reqId) {
      await useFriendsStore.getState().refresh();
      reqId = findPendingIn(uid);
    }
    if (!reqId) {
      setError("找不到对应的申请记录");
      return;
    }
    try {
      await api.declineFriend(reqId);
      note("已拒绝");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function declineReq(reqId: string) {
    try {
      await api.declineFriend(reqId);
      note("已拒绝");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function removeFriend(uid: string, name: string) {
    if (!window.confirm(`确认删除好友「${name}」？`)) return;
    try {
      await api.deleteFriend(uid);
      note("已删除好友");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "删除失败");
    }
  }

  async function cancelRequest(uid: string) {
    try {
      await api.deleteFriend(uid);
      note("已撤销申请");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "撤销失败");
    }
  }

  async function blockUser(uid: string, name: string) {
    if (!window.confirm(`拉黑「${name}」后双方将无法互相发消息，确认？`)) return;
    try {
      await api.blockFriend(uid);
      note("已拉黑");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "拉黑失败");
    }
  }

  async function unblockUser(uid: string) {
    try {
      await api.unblockFriend(uid);
      note("已解除拉黑");
      await afterAction(results !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "解除失败");
    }
  }

  async function messageUser(uid: string) {
    setError("");
    try {
      const c = await api.createSingle(uid);
      await useConversationsStore.getState().refreshList();
      nav(`/chat/${c.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "发起会话失败");
    }
  }

  function renderSearchActions(item: UserSearchItem) {
    const u = item.user;
    switch (item.relation) {
      case "none":
        return (
          <button type="button" className="mini-btn" onClick={() => void addFriend(u.id)}>
            加好友
          </button>
        );
      case "pending_out":
        return (
          <button type="button" className="mini-btn ghost" onClick={() => void cancelRequest(u.id)}>
            撤销申请
          </button>
        );
      case "pending_in":
        return (
          <>
            <button type="button" className="mini-btn" onClick={() => void acceptFromSearch(u.id)}>
              同意
            </button>
            <button type="button" className="mini-btn ghost" onClick={() => void declineFromSearch(u.id)}>
              拒绝
            </button>
          </>
        );
      case "accepted":
        return (
          <>
            <button type="button" className="mini-btn" onClick={() => void messageUser(u.id)}>
              发消息
            </button>
            <button
              type="button"
              className="mini-btn ghost"
              onClick={() => void removeFriend(u.id, u.display_name)}
            >
              删除
            </button>
          </>
        );
      case "blocked":
        return (
          <button type="button" className="mini-btn ghost" onClick={() => void unblockUser(u.id)}>
            解除拉黑
          </button>
        );
    }
  }

  const relationText: Record<string, string> = {
    none: "",
    pending_out: "已申请",
    pending_in: "待你处理",
    accepted: "好友",
    blocked: "已拉黑",
  };

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/profile" className="topbar-me" title="我的资料">
          {me && <Avatar user={me} size={30} />}
          <span className="topbar-me-name">联系人</span>
        </Link>
        <span />
        <button
          type="button"
          className="topbar-plus"
          title="添加好友"
          onClick={() => searchRef.current?.focus()}
        >
          <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden>
            <circle cx="10" cy="8" r="3.2" fill="none" stroke="currentColor" strokeWidth="1.7" />
            <path
              d="M4.5 19c.6-3.2 2.8-5 5.5-5s4.9 1.8 5.5 5"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
            />
            <path d="M17 7.5v5M14.5 10h5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
          </svg>
        </button>
      </header>
      <div className="searchbar">
        <input
          ref={searchRef}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void search();
          }}
          placeholder="搜索"
        />
        <button type="button" className="secondary" disabled={searching} onClick={() => void search()}>
          {searching ? "搜索中…" : "搜索"}
        </button>
      </div>
      {msg && <p className="ok pad-top">{msg}</p>}
      {error && <p className="error pad-top">{error}</p>}
      {results !== null && (
        <section className="pad-block">
          <h3 className="section-title">搜索结果</h3>
          <ul className="list">
            {results.length === 0 && <li className="empty">没有找到相关用户</li>}
            {results.map((item) => (
              <li key={item.user.id}>
                <div className="list-row">
                  <Avatar user={item.user} />
                  <div className="row-main">
                    <strong>
                      {item.user.display_name}
                      <span className="muted inline"> @{item.user.username}</span>
                    </strong>
                    <p>{item.user.signature || relationText[item.relation] || " "}</p>
                  </div>
                  <div className="row-actions">{renderSearchActions(item)}</div>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
      <div className="entry-list">
        <button type="button" className="entry-row" onClick={() => setTab("requests")}>
          <span className="entry-icon" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20">
              <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="1.6" />
              <path d="M12 8v5M12 16h.01" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
            </svg>
          </span>
          <span className="entry-title">新朋友</span>
          {pendingIn.length > 0 && <span className="badge">{pendingIn.length}</span>}
          <span className="entry-chevron" aria-hidden>›</span>
        </button>
        <button type="button" className="entry-row" onClick={() => setTab("blocked")}>
          <span className="entry-icon" aria-hidden>
            <svg viewBox="0 0 24 24" width="20" height="20">
              <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="1.6" />
              <path d="M6 6l12 12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </span>
          <span className="entry-title">黑名单</span>
          {blocked.length > 0 && <span className="badge gray">{blocked.length}</span>}
          <span className="entry-chevron" aria-hidden>›</span>
        </button>
      </div>
      <div className="tab-bar">
        <button
          type="button"
          className={`tab${tab === "friends" ? " active" : ""}`}
          onClick={() => setTab("friends")}
        >
          好友 {friends.length > 0 ? `(${friends.length})` : ""}
        </button>
        <button
          type="button"
          className={`tab${tab === "requests" ? " active" : ""}`}
          onClick={() => setTab("requests")}
        >
          申请{" "}
          {pendingIn.length + pendingOut.length > 0 ? `(${pendingIn.length + pendingOut.length})` : ""}
        </button>
        <button
          type="button"
          className={`tab${tab === "blocked" ? " active" : ""}`}
          onClick={() => setTab("blocked")}
        >
          黑名单 {blocked.length > 0 ? `(${blocked.length})` : ""}
        </button>
      </div>
      <ul className="list">
        {tab === "friends" && friends.length === 0 && (
          <li className="empty">还没有好友。用上方搜索找到人，发个申请吧。</li>
        )}
        {tab === "friends" &&
          friends.map((f) => (
            <FriendRow key={f.id} user={f}>
              <button type="button" className="mini-btn" onClick={() => void messageUser(f.id)}>
                发消息
              </button>
              <button
                type="button"
                className="mini-btn ghost"
                onClick={() => void removeFriend(f.id, f.display_name)}
              >
                删除
              </button>
              <button
                type="button"
                className="mini-btn ghost"
                onClick={() => void blockUser(f.id, f.display_name)}
              >
                拉黑
              </button>
            </FriendRow>
          ))}
        {tab === "requests" && pendingIn.length === 0 && pendingOut.length === 0 && (
          <li className="empty">没有待处理的好友申请</li>
        )}
        {tab === "requests" &&
          pendingIn.map((r) => (
            <FriendRow key={r.id} user={r.user}>
              <button type="button" className="mini-btn" onClick={() => void acceptReq(r.id)}>
                同意
              </button>
              <button type="button" className="mini-btn ghost" onClick={() => void declineReq(r.id)}>
                拒绝
              </button>
            </FriendRow>
          ))}
        {tab === "requests" &&
          pendingOut.map((r) => (
            <FriendRow key={r.id} user={r.user}>
              <button type="button" className="mini-btn ghost" onClick={() => void cancelRequest(r.user.id)}>
                撤销申请
              </button>
            </FriendRow>
          ))}
        {tab === "blocked" && blocked.length === 0 && <li className="empty">黑名单为空</li>}
        {tab === "blocked" &&
          blocked.map((b) => (
            <FriendRow key={b.id} user={b}>
              <button type="button" className="mini-btn ghost" onClick={() => void unblockUser(b.id)}>
                解除拉黑
              </button>
            </FriendRow>
          ))}
      </ul>
    </div>
  );
}

function FriendRow({ user, children }: { user: User; children: ReactNode }) {
  const online = usePresenceStore((s) => s.online.has(user.id));
  return (
    <li>
      <div className="list-row">
        <span className="avatar-wrap">
          <Avatar user={user} />
          <span className={`presence-dot${online ? " on" : ""}`} />
        </span>
        <div className="row-main">
          <strong>
            {user.display_name}
            <span className="muted inline"> @{user.username}</span>
          </strong>
          <p className="presence-line">
            <span className={`presence-status${online ? " on" : ""}`}>{online ? "在线" : "离线"}</span>
            {user.signature && <span className="muted inline"> · {user.signature}</span>}
          </p>
        </div>
        <div className="row-actions">{children}</div>
      </div>
    </li>
  );
}
