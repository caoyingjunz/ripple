import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Avatar } from "../components/Avatar";
import type { ConversationSummary } from "../api/types";
import { useConversationsStore } from "../stores/conversations";
import { usePresenceStore } from "../stores/presence";
import { fmtListTime } from "../utils/helpers";

export function ChatListPage() {
  const list = useConversationsStore((s) => s.list);
  const listLoaded = useConversationsStore((s) => s.listLoaded);
  const listError = useConversationsStore((s) => s.listError);
  const [q, setQ] = useState("");

  useEffect(() => {
    void useConversationsStore.getState().refreshList();
  }, []);

  const filtered = useMemo(() => {
    const kw = q.trim().toLowerCase();
    if (!kw) return list;
    return list.filter((c) => {
      const name =
        c.type === "single"
          ? `${c.peer?.display_name ?? ""} ${c.peer?.username ?? ""}`
          : (c.name ?? "");
      return name.toLowerCase().includes(kw);
    });
  }, [list, q]);

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/" className="linkish">
          ← 首页
        </Link>
        <h2>会话</h2>
        <Link to="/friends" className="linkish">
          好友
        </Link>
      </header>
      <div className="searchbar">
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索会话…" />
      </div>
      <div className="pad-actions">
        <Link to="/group/create" className="btn-link">
          发起群聊
        </Link>
      </div>
      {listError && <p className="error pad">{listError}</p>}
      <ul className="list">
        {listLoaded && filtered.length === 0 && (
          <li className="empty">还没有会话。去「好友」页给好友发消息，或去漂流瓶偶遇。</li>
        )}
        {filtered.map((c) => (
          <ConvRow key={c.id} conv={c} />
        ))}
      </ul>
    </div>
  );
}

function ConvRow({ conv }: { conv: ConversationSummary }) {
  const online = usePresenceStore((s) => (conv.peer ? s.online.has(conv.peer.id) : false));
  const name = conv.type === "single" ? (conv.peer?.display_name ?? "私聊") : (conv.name ?? "群聊");
  const time = conv.last_message_at ? fmtListTime(conv.last_message_at) : "";

  return (
    <li>
      <Link to={`/chat/${conv.id}`} className="list-row">
        <span className="avatar-wrap">
          {conv.peer ? (
            <Avatar user={conv.peer} />
          ) : (
            <Avatar user={{ display_name: name, avatar_color: "#127a8a" }} />
          )}
          {conv.type === "single" && <span className={`presence-dot${online ? " on" : ""}`} />}
        </span>
        <div className="row-main">
          <strong className="row-title">
            {name}
            {conv.pinned && <span className="chip">置顶</span>}
            {conv.muted && <span className="chip">静音</span>}
          </strong>
          <p className="row-preview">{conv.last_message_preview ?? "开始聊聊吧"}</p>
        </div>
        <div className="row-side">
          {time && <span className="row-time">{time}</span>}
          {conv.unread > 0 && (
            <span className={`badge${conv.muted ? " dot" : ""}`}>
              {conv.muted ? "" : conv.unread > 99 ? "99+" : conv.unread}
            </span>
          )}
        </div>
      </Link>
    </li>
  );
}
