import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { Bottle } from "../api/types";
import { useSessionStore } from "../stores/session";

const STATUS_TEXT: Record<Bottle["status"], string> = {
  floating: "漂流中",
  picked: "对话中",
  closed: "已结束",
};

export function BottleSeaPage() {
  const nav = useNavigate();
  const me = useSessionStore((s) => s.user);
  const [content, setContent] = useState("");
  const [bottles, setBottles] = useState<Bottle[]>([]);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  async function refresh() {
    const r = await api.myBottles();
    setBottles(r.bottles || []);
  }

  useEffect(() => {
    refresh().catch((e) => setError(e.message));
  }, []);

  async function throwBottle(e: FormEvent) {
    e.preventDefault();
    const c = content.trim();
    if (!c) return;
    setError("");
    try {
      await api.throwBottle(c);
      setContent("");
      setMsg("瓶子已漂向远方");
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "扔回失败");
    }
  }

  async function pick() {
    setError("");
    setMsg("");
    try {
      const t = await api.pickBottle();
      nav(`/bottle/thread/${t.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "海里暂时没有瓶子");
    }
  }

  return (
    <div className="page sea">
      <header className="topbar">
        <Link to="/" className="linkish">
          ← 首页
        </Link>
        <h2>漂流瓶</h2>
        <span />
      </header>
      <section className="sea-stage">
        <div className="wave" />
        <div className="bottle-float" aria-hidden />
        <form className="throw-form" onSubmit={throwBottle}>
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder="写下想扔进海里的话…"
            rows={3}
            required
          />
          <div className="row">
            <button type="submit">扔一个瓶子</button>
            <button type="button" className="secondary" onClick={() => void pick()}>
              捡一个瓶子
            </button>
          </div>
        </form>
        {msg && <p className="ok">{msg}</p>}
        {error && <p className="error">{error}</p>}
      </section>
      <section className="pad">
        <h3>我的瓶子</h3>
        <ul className="list">
          {bottles.length === 0 && <li className="empty">还没有记录</li>}
          {bottles.map((b) => {
            const role = b.thrower_id === me?.id ? "我扔的" : "我捡的";
            const body = (
              <div className="row-main">
                <strong>
                  {role} · {STATUS_TEXT[b.status] ?? b.status}
                </strong>
                <p className="row-preview">{b.content}</p>
              </div>
            );
            return (
              <li key={b.id}>
                {b.thread_id ? (
                  <Link to={`/bottle/thread/${b.thread_id}`} className="list-row">
                    {body}
                    {(b.unread ?? 0) > 0 && (
                      <div className="row-side">
                        <span className="badge">{(b.unread ?? 0) > 99 ? "99+" : b.unread}</span>
                      </div>
                    )}
                  </Link>
                ) : (
                  <div className="list-row">{body}</div>
                )}
              </li>
            );
          })}
        </ul>
      </section>
    </div>
  );
}
