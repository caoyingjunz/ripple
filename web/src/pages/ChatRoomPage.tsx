import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { resolveUrl } from "../api/base";
import { Avatar } from "../components/Avatar";
import { useConversationsStore, type ChatEntry } from "../stores/conversations";
import { usePresenceStore } from "../stores/presence";
import { useSessionStore } from "../stores/session";
import { useWsStatusStore } from "../stores/wsStatus";
import { wsClient } from "../ws/ws-client";
import { fmtTime, newId } from "../utils/helpers";

type PendingEntry = Extract<ChatEntry, { kind: "pending" }>;

const REVOKE_WINDOW_MS = 120_000;

export function ChatRoomPage() {
  const { id = "" } = useParams();
  const user = useSessionStore((s) => s.user);
  const conv = useConversationsStore((s) => s.list.find((c) => c.id === id));
  const entries = useConversationsStore((s) => s.messagesByConv[id]);
  const hasMore = useConversationsStore((s) => s.hasMoreByConv[id] ?? false);
  const loadingOlder = useConversationsStore((s) => s.loadingOlderByConv[id] ?? false);
  const openError = useConversationsStore((s) => (s.currentId === id ? s.openError : null));
  const peerOnline = usePresenceStore((s) => (conv?.peer ? s.online.has(conv.peer.id) : false));
  const wsStatus = useWsStatusStore((s) => s.status);
  const wsError = useWsStatusStore((s) => s.error);

  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const restore = useRef<{ h: number; t: number } | null>(null);

  useEffect(() => {
    stick.current = true;
    void useConversationsStore.getState().openConversation(id);
    return () => useConversationsStore.getState().closeConversation();
  }, [id]);

  // 窗口聚焦即补一次已读
  useEffect(() => {
    const onFocus = () => void useConversationsStore.getState().markReadIfBehind(id);
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [id]);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    if (restore.current) {
      const { h, t } = restore.current;
      restore.current = null;
      el.scrollTop = el.scrollHeight - h + t;
      return;
    }
    if (stick.current) el.scrollTop = el.scrollHeight;
  }, [entries]);

  function onScroll() {
    const el = scroller.current;
    if (!el) return;
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    if (el.scrollTop < 60 && hasMore && !loadingOlder) {
      restore.current = { h: el.scrollHeight, t: el.scrollTop };
      void useConversationsStore.getState().loadOlder(id).then((ok) => {
        if (!ok) restore.current = null;
      });
    }
  }

  /** 乐观发送：pending 气泡先进 store，WS 优先（ack 合并），断线退回 REST。 */
  function sendPayload(msgType: "text" | "image", body: string, imageUrl?: string) {
    const st = useConversationsStore.getState();
    const cmid = newId();
    st.addPending(id, { client_msg_id: cmid, type: msgType, body, image_url: imageUrl });
    stick.current = true;
    if (wsClient.isSendable()) {
      wsClient.send({
        type: "message.send",
        client_msg_id: cmid,
        conversation_id: id,
        msg_type: msgType,
        body,
        image_url: imageUrl,
      });
    } else {
      api
        .sendMessage(id, { type: msgType, body, image_url: imageUrl })
        .then((m) => useConversationsStore.getState().ackPending(cmid, m))
        .catch(() => useConversationsStore.getState().failPending(cmid));
    }
  }

  function sendText(e: FormEvent) {
    e.preventDefault();
    const body = text.trim();
    if (!body) return;
    setText("");
    setError("");
    sendPayload("text", body);
  }

  async function onImage(file: File | null) {
    if (!file || uploading) return;
    setUploading(true);
    setError("");
    try {
      const { url } = await api.upload(file);
      sendPayload("image", "", url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "上传失败");
    } finally {
      setUploading(false);
    }
  }

  function resend(entry: PendingEntry) {
    const st = useConversationsStore.getState();
    st.removePending(id, entry.client_msg_id);
    sendPayload(entry.type === "image" ? "image" : "text", entry.body, entry.image_url);
  }

  async function revoke(messageId: string) {
    setError("");
    try {
      const r = await api.revokeMessage(messageId);
      useConversationsStore
        .getState()
        .applyRevoked(id, messageId, r.message.revoked_at ?? Date.now());
    } catch (err) {
      setError(err instanceof Error ? err.message : "撤回失败");
    }
  }

  const single = conv?.type === "single";
  const title = single ? (conv?.peer?.display_name ?? "私聊") : (conv?.name ?? "群聊");

  function renderEntry(e: ChatEntry) {
    if (e.kind === "pending") {
      return (
        <div key={e.client_msg_id} className={`bubble mine pending${e.failed ? " failed" : ""}`}>
          {e.type === "image" && e.image_url ? (
            <img src={resolveUrl(e.image_url)} alt="" className="chat-img" />
          ) : (
            <p>{e.body}</p>
          )}
          <span className="bubble-meta">
            {e.failed ? (
              <>
                发送失败
                <button type="button" className="linkish mini" onClick={() => resend(e)}>
                  重发
                </button>
                <button
                  type="button"
                  className="linkish mini"
                  onClick={() =>
                    useConversationsStore.getState().removePending(id, e.client_msg_id)
                  }
                >
                  删除
                </button>
              </>
            ) : (
              "发送中…"
            )}
          </span>
        </div>
      );
    }
    const m = e.message;
    if (m.type === "system" || m.revoked_at) {
      return <div key={m.id} className="sysmsg">{m.revoked_at ? "已撤回" : m.body}</div>;
    }
    const mine = m.sender_id === user?.id;
    const canRevoke = mine && Date.now() - m.created_at < REVOKE_WINDOW_MS;
    return (
      <div key={m.id} className={`bubble ${mine ? "mine" : "theirs"}`}>
        {m.type === "image" && m.image_url ? (
          <img src={resolveUrl(m.image_url)} alt="" className="chat-img" />
        ) : (
          <p>{m.body}</p>
        )}
        {canRevoke && (
          <button type="button" className="revoke-btn" onClick={() => void revoke(m.id)}>
            撤回
          </button>
        )}
        <span className="bubble-time">{fmtTime(m.created_at)}</span>
      </div>
    );
  }

  return (
    <div className="page chat-room">
      <header className="topbar">
        <Link to="/chat" className="linkish">
          ← 会话
        </Link>
        <h2 className="room-title">
          <Link to={`/conversation/${id}/info`} className="room-title-link">
            {conv?.peer ? <Avatar user={conv.peer} size={28} /> : null}
            <span>{title}</span>
            {single && (
              <span className={`presence-text${peerOnline ? " on" : ""}`}>
                {peerOnline ? "· 在线" : "· 离线"}
              </span>
            )}
            {!single && conv && <span className="presence-text">{`· ${conv.member_count} 人`}</span>}
          </Link>
        </h2>
        <span
          className={`conn-dot${wsStatus === "connected" ? " on" : ""}`}
          title={wsStatus === "connected" ? "实时通道已连接" : "实时通道连接中…"}
        />
      </header>
      {(error || openError || wsError) && (
        <p className="error pad">{error || openError || wsError}</p>
      )}
      <div className="messages" ref={scroller} onScroll={onScroll}>
        {loadingOlder && <div className="sysmsg">加载更早消息…</div>}
        {entries?.map((e) => renderEntry(e))}
        {!entries && !openError && <div className="sysmsg">加载中…</div>}
      </div>
      <form className="composer" onSubmit={sendText}>
        <label className="file-btn" title="发送图片">
          {uploading ? "…" : "图"}
          <input
            type="file"
            accept="image/*"
            hidden
            onChange={(e) => {
              void onImage(e.target.files?.[0] ?? null);
              e.target.value = "";
            }}
          />
        </label>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="说点什么…" />
        <button type="submit" disabled={uploading}>
          发送
        </button>
      </form>
    </div>
  );
}
