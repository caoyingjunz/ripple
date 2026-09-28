import { isDesktopMode } from "../api/base";
import type { ClientFrame, ServerFrame, WsStatus } from "./frames";
import { BrowserTransport, ElectronTransport, type Transport } from "./transport";

type FrameListener = (frame: ServerFrame) => void;

/**
 * App 级 WS 单例：登录后 connect、登出 disconnect（由 ws/bridge.ts 驱动），
 * 心跳 25s 一跳；状态帧透传给全部订阅者。
 */
class WsClient {
  private transport: Transport | null = null;
  private stopEvents: (() => void) | null = null;
  private listeners = new Set<FrameListener>();
  private status: WsStatus = "disconnected";
  private pingTimer: number | null = null;

  connect(apiBase: string, token: string): void {
    this.disconnect();
    const transport = isDesktopMode() ? new ElectronTransport() : new BrowserTransport();
    this.stopEvents = transport.onEvent((frame) => this.dispatch(frame));
    this.transport = transport;
    transport.connect(apiBase, token);
    this.pingTimer = window.setInterval(() => this.send({ type: "ping" }), 25_000);
  }

  disconnect(): void {
    if (this.pingTimer !== null) {
      window.clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
    this.stopEvents?.();
    this.stopEvents = null;
    this.transport?.disconnect();
    this.transport = null;
    if (this.status !== "disconnected") {
      this.status = "disconnected";
      this.emit({ type: "ws.status", status: "disconnected" });
    }
  }

  send(payload: ClientFrame): void {
    this.transport?.send(payload);
  }

  /** 仅在已连接时可走 WS 发送，否则调用方退回 REST。 */
  isSendable(): boolean {
    return this.status === "connected";
  }

  onFrame(cb: FrameListener): () => void {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  }

  private dispatch(frame: unknown): void {
    const f = frame as ServerFrame;
    if (!f || typeof f !== "object" || typeof f.type !== "string") return;
    if (f.type === "ws.status") this.status = f.status;
    this.emit(f);
  }

  private emit(frame: ServerFrame): void {
    for (const cb of [...this.listeners]) cb(frame);
  }
}

export const wsClient = new WsClient();
