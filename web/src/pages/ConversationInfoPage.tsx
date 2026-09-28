import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import { Avatar } from "../components/Avatar";
import type { ConversationDetail } from "../api/types";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";
import { usePresenceStore } from "../stores/presence";
import { useSessionStore } from "../stores/session";
import { wsClient } from "../ws/ws-client";

export function ConversationInfoPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const me = useSessionStore((s) => s.user);
  const friends = useFriendsStore((s) => s.friends);
  const friendsLoaded = useFriendsStore((s) => s.loaded);

  const [detail, setDetail] = useState<ConversationDetail | null>(null);
  const [nameDraft, setNameDraft] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteSel, setInviteSel] = useState<Set<string>>(new Set());

  async function load() {
    try {
      const d = await api.conversationDetail(id);
      setDetail(d);
      setNameDraft(d.conversation.name ?? "");
      usePresenceStore.getState().applyUsers(d.members.map((m) => m.user));
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载会话信息失败");
    }
  }

  useEffect(() => {
    setDetail(null);
    setError("");
    void load();
    // 会话成员/名称变化时重拉明细
    return wsClient.onFrame((f) => {
      if (f.type === "conversation.updated" && f.conversation_id === id) void load();
    });
  }, [id]);

  useEffect(() => {
    if (!friendsLoaded) void useFriendsStore.getState().refresh();
  }, [friendsLoaded]);

  const conv = detail?.conversation;
  const isGroup = conv?.type === "group";
  const isOwner = !!me && conv?.owner_id === me.id;
  const memberIds = new Set(detail?.members.map((m) => m.user.id) ?? []);
  const invitable = friends.filter((f) => !memberIds.has(f.id));

  function toggleInvite(uid: string) {
    setInviteSel((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else next.add(uid);
      return next;
    });
  }

  async function rename() {
    const n = nameDraft.trim();
    if (!n) {
      setError("群名不能为空");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.renameConversation(id, n);
      await useConversationsStore.getState().refreshList();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "改名失败");
    } finally {
      setBusy(false);
    }
  }

  async function invite() {
    if (inviteSel.size === 0) {
      setError("请选择要邀请的好友");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.addMembers(id, [...inviteSel]);
      setInviteOpen(false);
      setInviteSel(new Set());
      await useConversationsStore.getState().refreshList();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "邀请失败");
    } finally {
      setBusy(false);
    }
  }

  async function removeMember(uid: string, name: string) {
    if (!window.confirm(`确认将「${name}」移出群聊？`)) return;
    try {
      await api.removeMember(id, uid);
      await useConversationsStore.getState().refreshList();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "移出失败");
    }
  }

  async function leave() {
    if (!window.confirm("退出后你将不再收到该群消息，确认退出群聊？")) return;
    try {
      await api.leaveConversation(id);
      await useConversationsStore.getState().refreshList();
      nav("/chat");
    } catch (err) {
      setError(err instanceof Error ? err.message : "退出失败");
    }
  }

  if (!detail && !error) return <div className="page pad">加载中…</div>;

  return (
    <div className="page">
      <header className="topbar">
        <Link to={`/chat/${id}`} className="linkish">
          ← 返回聊天
        </Link>
        <h2>{isGroup ? "群资料" : "私聊资料"}</h2>
        <span />
      </header>
      {error && <p className="error pad">{error}</p>}
      {conv && (
        <>
          {!isGroup && conv.peer && (
            <section className="info-grid pad-block">
              <div className="peer-card">
                <PeerPresence user={conv.peer} />
                <div className="row-main">
                  <strong>{conv.peer.display_name}</strong>
                  <p className="muted">@{conv.peer.username}</p>
                  <p>{conv.peer.signature || "这个人还没有签名"}</p>
                </div>
              </div>
            </section>
          )}
          {isGroup && (
            <section className="info-grid">
              <div className="kv-row">
                <span className="kv-key">群名</span>
                {isOwner ? (
                  <span className="inline-form">
                    <input value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} />
                    <button
                      type="button"
                      className="mini-btn"
                      disabled={busy}
                      onClick={() => void rename()}
                    >
                      保存
                    </button>
                  </span>
                ) : (
                  <span>{conv.name}</span>
                )}
              </div>
              <div className="kv-row">
                <span className="kv-key">成员</span>
                <span>{conv.member_count} 人（群主 {isOwner ? "是你" : "另有其人"}）</span>
              </div>
            </section>
          )}
          {isGroup && (
            <section className="pad-block">
              <h3 className="section-title">
                成员列表
                {isOwner && (
                  <button
                    type="button"
                    className="mini-btn inline-right"
                    onClick={() => setInviteOpen((v) => !v)}
                  >
                    {inviteOpen ? "收起邀请" : "邀请好友"}
                  </button>
                )}
              </h3>
              {inviteOpen && (
                <div className="invite-panel">
                  {invitable.length === 0 && <p className="muted">没有可邀请的好友（都在群里了）</p>}
                  {invitable.map((f) => (
                    <label key={f.id} className={`pick-row${inviteSel.has(f.id) ? " picked" : ""}`}>
                      <input
                        type="checkbox"
                        checked={inviteSel.has(f.id)}
                        onChange={() => toggleInvite(f.id)}
                      />
                      <Avatar user={f} size={32} />
                      <span className="pick-name">{f.display_name}</span>
                      <span className="muted inline">@{f.username}</span>
                    </label>
                  ))}
                  <button
                    type="button"
                    className="mini-btn"
                    disabled={busy}
                    onClick={() => void invite()}
                  >
                    确认邀请（{inviteSel.size}）
                  </button>
                </div>
              )}
              <ul className="list">
                {detail?.members.map((m) => (
                  <li key={m.user.id}>
                    <div className="list-row">
                      <Avatar user={m.user} />
                      <div className="row-main">
                        <strong>
                          {m.user.display_name}
                          {m.role === "owner" && <span className="chip">群主</span>}
                          {me && m.user.id === me.id && <span className="chip">我</span>}
                        </strong>
                        <p className="muted-inline">@{m.user.username}</p>
                      </div>
                      {isOwner && me && m.user.id !== me.id && (
                        <div className="row-actions">
                          <button
                            type="button"
                            className="mini-btn ghost"
                            onClick={() => void removeMember(m.user.id, m.user.display_name)}
                          >
                            移出
                          </button>
                        </div>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
              {!isOwner && (
                <div className="pad-actions">
                  <button type="button" className="secondary" onClick={() => void leave()}>
                    退出群聊
                  </button>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
}

function PeerPresence({ user }: { user: NonNullable<ConversationDetail["conversation"]["peer"]> }) {
  const online = usePresenceStore((s) => s.online.has(user.id));
  return (
    <span className="avatar-wrap">
      <Avatar user={user} />
      <span className={`presence-dot${online ? " on" : ""}`} />
    </span>
  );
}
