import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { BottleThreadView } from "../api/types";
import { useConversationsStore } from "../stores/conversations";
import { wsClient } from "../ws/ws-client";

export function BottleThreadPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const [thread, setThread] = useState<BottleThreadView | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const bottom = useRef<HTMLDivElement>(null);

  async function load() {
    try {
      const t = await api.getThread(id);
      setThread(t);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载瓶中信失败");
    }
  }

  useEffect(() => {
    setThread(null);
    setError("");
    void load();
    // REST 发送 + bottle.message WS 事件都直接重拉 thread（轮次进度以服务端为准）
    return wsClient.onFrame((f) => {
      if (f.type === "bottle.message" && f.thread_id === id) void load();
      if (f.type === "bottle.revealed" && f.thread_id === id) {
        useConversationsStore.getState().scheduleRefresh();
        nav(`/chat/${f.conversation_id}`);
      }
    });
  }, [id]);

  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [thread?.messages.length]);

  async function send(e: FormEvent) {
    e.preventDefault();
    const body = text.trim();
    if (!body || !thread || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.bottleMessage(id, body);
      setText("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送失败");
    } finally {
      setBusy(false);
    }
  }

  async function reveal() {
    setError("");
    try {
      const r = await api.reveal(id);
      useConversationsStore.getState().scheduleRefresh();
      nav(`/chat/${r.conversation_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function close() {
    if (!window.confirm("结束后这段匿名对话不可继续，确认？")) return;
    setError("");
    try {
      await api.closeThread(id);
      nav("/bottle");
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  if (!thread && !error) return <div className="page pad">加载中…</div>;

  const canSend =
    !!thread && !thread.closed && !thread.revealed && thread.round_count < thread.max_rounds;

  return (
    <div className="page chat-room">
      <header className="topbar">
        <Link to="/bottle" className="linkish">
          ← 海边
        </Link>
        <h2>匿名瓶中信</h2>
        <span className="muted">{thread?.my_alias}</span>
      </header>
      {error && <p className="error pad">{error}</p>}
      {thread && (
        <>
          <p className="pad muted">
            {thread.revealed
              ? "已公开身份，对话转入私聊。"
              : `已交流 ${thread.round_count}/${thread.max_rounds} 轮 · 身份对彼此隐藏`}
          </p>
          <div className="bottle-content pad">
            <span className="muted">瓶中信：</span>
            {thread.bottle_content}
          </div>
          <div className="messages">
            {thread.messages.map((m) => (
              <div key={m.id} className={`bubble ${m.mine ? "mine" : "theirs"}`}>
                <span className="alias">{m.alias}</span>
                <p>{m.body}</p>
              </div>
            ))}
            <div ref={bottom} />
          </div>
          <div className="row pad">
            {thread.revealed ? (
              thread.conversation_id && (
                <button type="button" onClick={() => nav(`/chat/${thread.conversation_id}`)}>
                  前往私聊
                </button>
              )
            ) : (
              <>
                <button type="button" onClick={() => void reveal()}>
                  公开身份并转私聊
                </button>
                <button type="button" className="secondary" onClick={() => void close()}>
                  结束
                </button>
              </>
            )}
          </div>
          {canSend ? (
            <form className="composer" onSubmit={send}>
              <input
                value={text}
                onChange={(e) => setText(e.target.value)}
                placeholder="匿名回复…"
                disabled={busy}
              />
              <button type="submit" disabled={busy}>
                发送
              </button>
            </form>
          ) : (
            !thread.revealed && (
              <p className="pad muted">匿名轮次已结束或已关闭，可公开身份继续。</p>
            )
          )}
        </>
      )}
    </div>
  );
}
