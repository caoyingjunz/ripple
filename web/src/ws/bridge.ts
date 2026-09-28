import { getToken } from "../api/client";
import { getApiBase } from "../api/base";
import { useConversationsStore } from "../stores/conversations";
import { useFriendsStore } from "../stores/friends";
import { usePresenceStore } from "../stores/presence";
import { useSessionStore } from "../stores/session";
import { useWsStatusStore } from "../stores/wsStatus";
import type { ServerFrame } from "./frames";
import { wsClient } from "./ws-client";

let installed = false;
let hasConnectedOnce = false;

/**
 * WS 桥：登录态驱动连接生命周期（登录 connect / 登出 disconnect），
 * 服务端帧分发到各 store；断线重连（ws.status connected 且非首连）触发增量补拉。
 * main.tsx 调一次 initWsBridge()。
 */
export function initWsBridge(): void {
  if (installed) return;
  installed = true;

  let prevUserId: string | null = null;
  useSessionStore.subscribe((state) => {
    const uid = state.user?.id ?? null;
    if (uid === prevUserId) return;
    prevUserId = uid;
    if (uid) {
      wsClient.connect(getApiBase(), getToken() ?? "");
    } else {
      wsClient.disconnect();
      hasConnectedOnce = false;
    }
  });

  wsClient.onFrame((frame) => handleFrame(frame));
}

function handleFrame(frame: ServerFrame): void {
  switch (frame.type) {
    case "ws.status": {
      useWsStatusStore.getState().setStatus(frame.status);
      if (frame.status === "connected") {
        if (!hasConnectedOnce) {
          hasConnectedOnce = true; // 首连：初始加载已覆盖，无需补拉
        } else {
          const mineId = useSessionStore.getState().user?.id ?? null;
          const convs = useConversationsStore.getState();
          convs.scheduleRefresh();
          void convs.catchUpAll(mineId);
          void useFriendsStore.getState().refresh();
        }
      } else {
        // 断线/重连中：结果未知的乐观消息标记为失败（可重发）
        useConversationsStore.getState().markPendingIndeterminate();
      }
      return;
    }
    case "message.ack":
      useConversationsStore.getState().ackPending(frame.client_msg_id, frame.message);
      return;
    case "message.new": {
      const mineId = useSessionStore.getState().user?.id ?? null;
      const convs = useConversationsStore.getState();
      convs.mergeIncoming(frame.message, frame.message.sender_id === mineId);
      // 打开中的会话收到新消息且窗口聚焦 → 立即上报已读
      if (frame.message.conversation_id === convs.currentId && document.hasFocus()) {
        void convs.markReadIfBehind(frame.message.conversation_id);
      }
      return;
    }
    case "message.revoked":
      useConversationsStore
        .getState()
        .applyRevoked(frame.conversation_id, frame.message_id, frame.revoked_at);
      return;
    case "presence.change": {
      const presence = usePresenceStore.getState();
      if (frame.online) presence.setOnline(frame.user_id);
      else presence.setOffline(frame.user_id);
      return;
    }
    case "friend.request":
      useFriendsStore.getState().applyRequestEvent(frame.action, frame.request);
      return;
    case "conversation.updated":
      useConversationsStore.getState().scheduleRefresh();
      return;
    case "bottle.revealed":
      // reveal 产生新私聊会话；跳转由 BottleThreadPage 自己处理
      useConversationsStore.getState().scheduleRefresh();
      return;
    case "error":
      if (frame.client_msg_id) {
        useConversationsStore.getState().failPending(frame.client_msg_id);
      } else {
        useWsStatusStore.getState().setError(frame.message);
      }
      return;
    case "pong":
    case "message.read": // 已读水位由列表刷新兜底，暂无逐条展示
    case "bottle.message": // BottleThreadPage 订阅处理
      return;
  }
}
