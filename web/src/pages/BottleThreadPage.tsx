import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { BottleMessage, BottleThread } from "../api/types";
import { useWS } from "../ws/useWS";

export function BottleThreadPage() {
  const { id = "" } = useParams();
  const [thread, setThread] = useState<BottleThread | null>(null);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const nav = useNavigate();

  useEffect(() => {
    api
      .getThread(id)
      .then(setThread)
      .catch((e) => setError(e.message));
  }, [id]);

  useWS((data) => {
    if (data.type === "bottle.message" && data.thread_id === id) {
      const m = data.message as BottleMessage;
      setThread((t) => {
        if (!t) return t;
        if (t.messages.some((x) => x.id === m.id)) return t;
        return { ...t, messages: [...t.messages, m], round_count: t.round_count + (m.mine ? 0 : 1) };
      });
    }
    if (data.type === "bottle.revealed" && data.thread_id === id) {
      nav(`/chat/${data.conversation_id}`);
    }
  });

  async function send(e: FormEvent) {
    e.preventDefault();
    if (!text.trim() || !thread) return;
    try {
      const m = await api.bottleMessage(id, text.trim());
      setText("");
      setThread((t) =>
        t
          ? {
              ...t,
              messages: t.messages.some((x) => x.id === m.id) ? t.messages : [...t.messages, m],
              round_count: t.round_count + 1,
            }
          : t,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送失败");
    }
  }

  async function reveal() {
    try {
      const r = await api.reveal(id);
      nav(`/chat/${r.conversation_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "失败");
    }
  }

  async function close() {
    try {
      await api.closeThread(id);
      nav("/bottle");
    } catch (err) {
      setError(err instanceof Error ? err.message : "失败");
    }
  }

  if (!thread && !error) return <div className="page pad">加载中…</div>;

  const canSend = thread && !thread.closed && !thread.revealed && thread.round_count < thread.max_rounds;

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
            已交流 {thread.round_count}/{thread.max_rounds} 轮 · 身份对彼此隐藏
          </p>
          <div className="messages">
            {thread.messages.map((m) => (
              <div key={m.id} className={`bubble ${m.mine ? "mine" : "theirs"}`}>
                <span className="alias">{m.alias}</span>
                <p>{m.body}</p>
              </div>
            ))}
          </div>
          <div className="row pad">
            <button type="button" onClick={reveal}>
              公开身份并转私聊
            </button>
            <button type="button" className="secondary" onClick={close}>
              结束
            </button>
          </div>
          {canSend ? (
            <form className="composer" onSubmit={send}>
              <input value={text} onChange={(e) => setText(e.target.value)} placeholder="匿名回复…" />
              <button type="submit">发送</button>
            </form>
          ) : (
            <p className="pad muted">匿名轮次已结束或已关闭，可公开身份继续。</p>
          )}
        </>
      )}
    </div>
  );
}
