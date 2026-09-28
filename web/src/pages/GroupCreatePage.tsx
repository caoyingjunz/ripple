import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { Avatar } from "../components/Avatar";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";

export function GroupCreatePage() {
  const nav = useNavigate();
  const friends = useFriendsStore((s) => s.friends);
  const loaded = useFriendsStore((s) => s.loaded);
  const [name, setName] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!loaded) void useFriendsStore.getState().refresh();
  }, []);

  function toggle(uid: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else next.add(uid);
      return next;
    });
  }

  async function create(e: FormEvent) {
    e.preventDefault();
    const n = name.trim();
    if (!n) {
      setError("请填写群名");
      return;
    }
    if (n.length > 32) {
      setError("群名最多 32 字");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const c = await api.createGroup(n, [...selected]);
      await useConversationsStore.getState().refreshList();
      nav(`/chat/${c.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/chat" className="linkish">
          ← 会话
        </Link>
        <h2>发起群聊</h2>
        <span />
      </header>
      <form className="stack pad-block" onSubmit={create}>
        <label>
          群名（1-32 字）
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="给群聊起个名字" />
        </label>
        <h3 className="section-title">选择好友（可选，建群后也可再邀请）</h3>
        <div className="pick-list">
          {friends.length === 0 && <p className="muted">还没有好友，先去好友页添加吧。</p>}
          {friends.map((f) => (
            <label key={f.id} className={`pick-row${selected.has(f.id) ? " picked" : ""}`}>
              <input
                type="checkbox"
                checked={selected.has(f.id)}
                onChange={() => toggle(f.id)}
              />
              <Avatar user={f} size={32} />
              <span className="pick-name">{f.display_name}</span>
              <span className="muted inline">@{f.username}</span>
            </label>
          ))}
        </div>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy}>
          {busy ? "创建中…" : `创建群聊${selected.size > 0 ? `（含我共 ${selected.size + 1} 人）` : ""}`}
        </button>
      </form>
    </div>
  );
}
