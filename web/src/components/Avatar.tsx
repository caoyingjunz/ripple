import { resolveUrl } from "../api/base";

type AvatarUser = {
  display_name: string;
  avatar_color: string;
  avatar_url?: string | null;
};

/** 头像：有 avatar_url 用图片（按 apiBase 拼相对地址），否则色块 + 首字。 */
export function Avatar({ user, size = 40 }: { user: AvatarUser; size?: number }) {
  const style = { width: size, height: size, fontSize: Math.round(size * 0.42) };
  if (user.avatar_url) {
    return (
      <img
        className="avatar avatar-img"
        style={style}
        src={resolveUrl(user.avatar_url)}
        alt={user.display_name}
      />
    );
  }
  return (
    <span className="avatar" style={{ ...style, background: user.avatar_color || "#127a8a" }}>
      {user.display_name.slice(0, 1)}
    </span>
  );
}
