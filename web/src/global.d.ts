export {};

declare global {
  interface RippleDesktopWs {
    connect(apiBase: string, token: string): void;
    disconnect(): void;
    send(payload: unknown): void;
    onEvent(cb: (frame: unknown) => void): () => void;
  }

  interface Window {
    /** 桌面壳注入（见 docs/implementation-spec.md §6.2，形状不得变更） */
    ripple?: {
      isDesktop: boolean;
      platform: string;
      ws: RippleDesktopWs;
    };
  }
}
