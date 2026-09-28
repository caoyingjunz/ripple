import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { Avatar } from "../components/Avatar";
import { useSessionStore } from "../stores/session";

export function ProfilePage() {
  const user = useSessionStore((s) => s.user);
  const [displayName, setDisplayName] = useState("");
  const [signature, setSignature] = useState("");
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (user) {
      setDisplayName(user.display_name);
      setSignature(user.signature);
    }
  }, [user?.id]);

  async function save(e: FormEvent) {
    e.preventDefault();
    const disp = displayName.trim();
    if (!disp) {
      setError("昵称不能为空");
      return;
    }
    if (disp.length > 32) {
      setError("昵称最多 32 字");
      return;
    }
    if (signature.length > 128) {
      setError("签名最多 128 字");
      return;
    }
    setBusy(true);
    setError("");
    setMsg("");
    try {
      const updated = await api.updateMe({ display_name: disp, signature });
      useSessionStore.setState({ user: updated });
      setMsg("已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function onAvatar(file: File | null) {
    if (!file || uploading) return;
    setUploading(true);
    setError("");
    setMsg("");
    try {
      await api.uploadAvatar(file);
      await useSessionStore.getState().refreshMe();
      setMsg("头像已更新");
    } catch (err) {
      setError(err instanceof Error ? err.message : "上传失败");
    } finally {
      setUploading(false);
    }
  }

  if (!user) return null;

  return (
    <div className="page">
      <header className="topbar">
        <Link to="/" className="linkish">
          ← 首页
        </Link>
        <h2>编辑资料</h2>
        <span />
      </header>
      {error && <p className="error pad-top">{error}</p>}
      {msg && <p className="ok pad-top">{msg}</p>}
      <section className="pad-block avatar-edit">
        <Avatar user={user} size={64} />
        <label className="file-btn big" title="更换头像">
          {uploading ? "…" : "换头像"}
          <input
            type="file"
            accept="image/*"
            hidden
            onChange={(e) => {
              void onAvatar(e.target.files?.[0] ?? null);
              e.target.value = "";
            }}
          />
        </label>
      </section>
      <form className="stack pad-block" onSubmit={save}>
        <label>
          用户名（不可修改）
          <input value={user.username} disabled />
        </label>
        <label>
          昵称
          <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </label>
        <label>
          签名
          <textarea
            value={signature}
            onChange={(e) => setSignature(e.target.value)}
            rows={3}
            placeholder="写一句话介绍自己…"
          />
        </label>
        <button type="submit" disabled={busy || uploading}>
          {busy ? "保存中…" : "保存"}
        </button>
      </form>
    </div>
  );
}
