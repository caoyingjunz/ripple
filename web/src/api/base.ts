const SERVER_KEY = "ripple_server_base";

/**
 * API 基址抽象（收敛浏览器耦合，见 docs/implementation-spec.md §5.1）。
 * 空串 = 同源（web dev 走 vite proxy）；桌面模式由登录/注册页写入。
 */
export function getApiBase(): string {
  const stored = localStorage.getItem(SERVER_KEY);
  if (stored !== null) return stored;
  const fromEnv = import.meta.env.VITE_API_BASE;
  return typeof fromEnv === "string" ? fromEnv : "";
}

export function setApiBase(base: string): void {
  localStorage.setItem(SERVER_KEY, base);
}

/** 桌面模式 = window.ripple 注入存在。 */
export function isDesktopMode(): boolean {
  return typeof window !== "undefined" && window.ripple != null;
}

/** 上传接口返回相对地址 /uploads/x，渲染时按 apiBase 拼接。 */
export function resolveUrl(url: string | null | undefined): string {
  if (!url) return "";
  if (/^(https?:)?\/\//i.test(url) || url.startsWith("data:")) return url;
  const base = getApiBase().replace(/\/+$/, "");
  return base + url;
}
