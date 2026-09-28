import { useState, type FormEvent } from "react";
import { Link, Navigate } from "react-router-dom";
import { getApiBase, isDesktopMode } from "../api/base";
import { useSessionStore } from "../stores/session";

const demos = ["alice", "bob", "carol", "dave", "erin"];

export function LoginPage() {
  const user = useSessionStore((s) => s.user);
  const login = useSessionStore((s) => s.login);
  const desktop = isDesktopMode();
  const [username, setUsername] = useState(desktop ? "" : "alice");
  const [password, setPassword] = useState(desktop ? "" : "demo123");
  const [server, setServer] = useState(getApiBase() || "http://127.0.0.1:8080");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (user) return <Navigate to="/" replace />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    const base = server.trim();
    if (desktop && !base) {
      setError("请填写服务器地址");
      return;
    }
    setBusy(true);
    try {
      await login(username.trim(), password, desktop ? base : undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="page splash">
      <div className="splash-panel">
        <p className="brand">Ripple</p>
        <h1>登录</h1>
        <p className="lede">把心事扔进海里，或与认识的人继续交谈。</p>
        <form onSubmit={onSubmit} className="stack">
          {desktop && (
            <label>
              服务器地址
              <input
                value={server}
                onChange={(e) => setServer(e.target.value)}
                placeholder="http://127.0.0.1:8080"
                autoComplete="url"
              />
            </label>
          )}
          <label>
            用户名
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          </label>
          <label>
            密码
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
          </label>
          {error && <p className="error">{error}</p>}
          <button type="submit" disabled={busy}>
            {busy ? "登录中…" : "进入"}
          </button>
        </form>
        <p className="hint">
          还没有账号？
          <Link to="/register" className="textlink">
            注册一个
          </Link>
        </p>
        {!desktop && <p className="hint">演示账号：{demos.join(" / ")}，密码均为 demo123</p>}
      </div>
    </div>
  );
}
