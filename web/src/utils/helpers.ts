export function newId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `id-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function pad(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

/** 时间戳为毫秒（服务端 UnixMilli）。 */
export function fmtTime(ts: number): string {
  const d = new Date(ts);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtListTime(ts: number): string {
  const d = new Date(ts);
  if (d.toDateString() === new Date().toDateString()) return fmtTime(ts);
  return `${d.getMonth() + 1}-${d.getDate()}`;
}
