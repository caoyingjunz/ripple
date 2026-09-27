import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Conversation } from "../api/types";

export function ChatListPage() {
  const [list, setList] = useState<Conversation[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .conversations()
      .then((r) => setList(r.conversations || []))
      .catch((e) => setError(e.message));
  }, []);

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/" className="linkish">
          ← 首页
        </Link>
        <h2>会话</h2>
        <span />
      </header>
      {error && <p className="error pad">{error}</p>}
      <ul className="list">
        {list.length === 0 && <li className="empty">还没有会话。去漂流瓶偶遇后公开身份即可开始。</li>}
        {list.map((c) => (
          <li key={c.id}>
            <Link to={`/chat/${c.id}`} className="list-row">
              <span className="avatar" style={{ background: c.peer.avatar_color }}>
                {c.peer.display_name.slice(0, 1)}
              </span>
              <div>
                <strong>{c.peer.display_name}</strong>
                <p>{c.last_message || "开始聊聊吧"}</p>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
