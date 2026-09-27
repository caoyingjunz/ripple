import { useState, type FormEvent } from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

const demos = ["alice", "bob", "carol", "dave", "erin"];

export function LoginPage() {
  const { user, login } = useAuth();
  const [username, setUsername] = useState("alice");
  const [password, setPassword] = useState("demo123");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (user) return <Navigate to="/" replace />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await login(username, password);
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
        <h1>漂流瓶与私聊</h1>
        <p className="lede">把心事扔进海里，或与偶遇的人继续交谈。</p>
        <form onSubmit={onSubmit} className="stack">
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
        <p className="hint">演示账号：{demos.join(" / ")}，密码均为 demo123</p>
      </div>
    </div>
  );
}
