import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Message } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { useWS } from "../ws/useWS";

export function ChatRoomPage() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const [messages, setMessages] = useState<Message[]>([]);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const bottom = useRef<HTMLDivElement>(null);

  useEffect(() => {
    api
      .messages(id)
      .then((r) => setMessages(r.messages || []))
      .catch((e) => setError(e.message));
  }, [id]);

  useWS((data) => {
    if (data.type === "chat.message") {
      const m = data.message as Message;
      if (m.conversation_id === id) {
        setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]));
      }
    }
  });

  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  async function send(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    const body = text.trim();
    setText("");
    try {
      const m = await api.sendMessage(id, { type: "text", body });
      setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]));
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送失败");
    }
  }

  async function onImage(file: File | null) {
    if (!file) return;
    try {
      const { url } = await api.upload(file);
      const m = await api.sendMessage(id, { type: "image", body: "", image_url: url });
      setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]));
    } catch (err) {
      setError(err instanceof Error ? err.message : "上传失败");
    }
  }

  return (
    <div className="page chat-room">
      <header className="topbar">
        <Link to="/chat" className="linkish">
          ← 会话
        </Link>
        <h2>私聊</h2>
        <span />
      </header>
      {error && <p className="error pad">{error}</p>}
      <div className="messages">
        {messages.map((m) => {
          const mine = m.sender_id === user?.id;
          return (
            <div key={m.id} className={`bubble ${mine ? "mine" : "theirs"}`}>
              {m.type === "image" && m.image_url ? (
                <img src={m.image_url} alt="" className="chat-img" />
              ) : (
                <p>{m.body}</p>
              )}
            </div>
          );
        })}
        <div ref={bottom} />
      </div>
      <form className="composer" onSubmit={send}>
        <label className="file-btn">
          图
          <input type="file" accept="image/*" hidden onChange={(e) => onImage(e.target.files?.[0] || null)} />
        </label>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="说点什么…" />
        <button type="submit">发送</button>
      </form>
    </div>
  );
}
