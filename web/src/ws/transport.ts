/** 传输层抽象（docs/implementation-spec.md §5.2）。 */

export interface Transport {
  connect(apiBase: string, token: string): void;
  disconnect(): void;
  send(payload: unknown): void;
  onEvent(cb: (frame: unknown) => void): () => void;
}

export function buildWsUrl(apiBase: string, token: string): string {
  const base = apiBase || window.location.origin;
  const url = new URL(base);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = url.pathname.replace(/\/+$/, "") + "/api/ws";
  url.search = `token=${encodeURIComponent(token)}`;
  return url.toString();
}

/**
 * 浏览器直连：原生 WebSocket + 指数退避重连（1s 起倍增，上限 30s，±20% 抖动）。
 * 连上/断开时合成内部帧 {type:'ws.status'} 抛给上层。
 */
export class BrowserTransport implements Transport {
  private url = "";
  private ws: WebSocket | null = null;
  private cb: ((frame: unknown) => void) | null = null;
  private stopped = true;
  private retry = 0;
  private timer: number | null = null;

  connect(apiBase: string, token: string): void {
    this.hardStop();
    this.url = buildWsUrl(apiBase, token);
    this.stopped = false;
    this.retry = 0;
    this.open();
  }

  disconnect(): void {
    // 主动断开不合成 reconnecting 帧（由 WsClient 统一发 disconnected）
    this.hardStop();
  }

  private hardStop(): void {
    this.stopped = true;
    if (this.timer !== null) {
      window.clearTimeout(this.timer);
      this.timer = null;
    }
    const ws = this.ws;
    this.ws = null;
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.onerror = null;
      try {
        ws.close();
      } catch {
        /* ignore */
      }
    }
  }

  private open(): void {
    const ws = new WebSocket(this.url);
    this.ws = ws;
    ws.onopen = () => {
      this.retry = 0;
      this.cb?.({ type: "ws.status", status: "connected" });
    };
    ws.onmessage = (ev) => {
      let frame: unknown = null;
      try {
        frame = JSON.parse(ev.data as string);
      } catch {
        return;
      }
      if (frame) this.cb?.(frame);
    };
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      this.cb?.({ type: "ws.status", status: "reconnecting" });
      if (!this.stopped) this.scheduleReconnect();
    };
    ws.onerror = () => {
      /* onclose 会跟进 */
    };
  }

  private scheduleReconnect(): void {
    const base = Math.min(30_000, 1_000 * 2 ** this.retry);
    this.retry += 1;
    const delay = base * (0.8 + Math.random() * 0.4); // ±20% 抖动
    this.timer = window.setTimeout(() => {
      this.timer = null;
      this.open();
    }, delay);
  }

  send(payload: unknown): void {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(payload));
  }

  onEvent(cb: (frame: unknown) => void): () => void {
    this.cb = cb;
    return () => {
      if (this.cb === cb) this.cb = null;
    };
  }
}

function safeParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return null;
  }
}

/**
 * 桌面壳：window.ripple.ws 直通。不自带重连（主进程负责），
 * 透传主进程的 ws.status 帧；帧兼容字符串与对象两种形态。
 */
export class ElectronTransport implements Transport {
  connect(apiBase: string, token: string): void {
    const ws = window.ripple?.ws;
    if (!ws) return;
    ws.connect(apiBase, token);
  }

  disconnect(): void {
    window.ripple?.ws?.disconnect();
  }

  send(payload: unknown): void {
    window.ripple?.ws?.send(payload);
  }

  onEvent(cb: (frame: unknown) => void): () => void {
    const ws = window.ripple?.ws;
    if (!ws) return () => undefined;
    return ws.onEvent((raw) => {
      const frame = typeof raw === "string" ? safeParse(raw) : raw;
      if (frame) cb(frame);
    });
  }
}
