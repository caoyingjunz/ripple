import { useEffect, useRef } from "react";
import { getToken } from "../api/client";

type Handler = (data: Record<string, unknown>) => void;

export function useWS(onMessage: Handler) {
  const handler = useRef(onMessage);
  handler.current = onMessage;
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const token = getToken();
    if (!token) return;
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const url = `${proto}://${location.host}/api/ws?token=${encodeURIComponent(token)}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      try {
        handler.current(JSON.parse(ev.data));
      } catch {
        /* ignore */
      }
    };
    const ping = setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "ping" }));
    }, 25000);
    return () => {
      clearInterval(ping);
      ws.close();
    };
  }, []);

  return {
    send(payload: Record<string, unknown>) {
      wsRef.current?.send(JSON.stringify(payload));
    },
  };
}
