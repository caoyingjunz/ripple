import { useState, type FormEvent } from "react";
import { Link, Navigate } from "react-router-dom";
import { getApiBase, isDesktopMode } from "../api/base";
import { useSessionStore } from "../stores/session";

const USERNAME_RE = /^[a-z0-9_]{3,32}$/;

export function RegisterPage() {
  const user = useSessionStore((s) => s.user);
  const register = useSessionStore((s) => s.register);
  const desktop = isDesktopMode();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [server, setServer] = useState(getApiBase() || "http://127.0.0.1:8080");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (user) return <Navigate to="/" replace />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    const name = username.trim();
    const pass = password;
    const disp = displayName.trim();
    if (!USERNAME_RE.test(name)) {
      setError("用户名需为 3-32 位小写字母、数字或下划线");
      return;
    }
    if (pass.length < 6) {
      setError("密码至少 6 位");
      return;
    }
    if (disp.length > 32) {
      setError("昵称最多 32 字");
      return;
    }
    const base = server.trim();
    if (desktop && !base) {
      setError("请填写服务器地址");
      return;
    }
    setBusy(true);
    try {
      await register(name, pass, disp, desktop ? base : undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="page splash">
      <div className="splash-panel">
        <p className="brand">Ripple</p>
        <h1>注册</h1>
        <p className="lede">取一个用户名，开始你的漂流。</p>
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
              autoComplete="new-password"
            />
          </label>
          <label>
            昵称（可选）
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="默认同用户名" />
          </label>
          {error && <p className="error">{error}</p>}
          <button type="submit" disabled={busy}>
            {busy ? "注册中…" : "注册并进入"}
          </button>
        </form>
        <p className="hint">
          已有账号？
          <Link to="/login" className="textlink">
            返回登录
          </Link>
        </p>
      </div>
    </div>
  );
}
