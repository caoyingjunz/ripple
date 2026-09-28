import type { BottleMessageView, FriendRequest, Message, User } from "../api/types";

/** 传输层合成的内部连接状态帧（非服务端协议，仅前端消费）。 */
export type WsStatus = "connected" | "reconnecting" | "disconnected";

export type ClientFrame =
  | { type: "ping" }
  | {
      type: "message.send";
      client_msg_id: string;
      conversation_id: string;
      msg_type: "text" | "image";
      body: string;
      image_url?: string;
    };

export type ServerFrame =
  | { type: "pong" }
  | { type: "ws.status"; status: WsStatus }
  | { type: "message.ack"; client_msg_id: string; message: Message }
  | { type: "message.new"; message: Message }
  | { type: "message.revoked"; conversation_id: string; message_id: string; revoked_at: number }
  | { type: "message.read"; conversation_id: string; user_id: string; seq: number }
  | { type: "presence.change"; user_id: string; online: boolean }
  | { type: "friend.request"; action: "pending" | "accepted"; request: FriendRequest }
  | {
      type: "conversation.updated";
      conversation_id: string;
      event: "created" | "renamed" | "members_added" | "member_removed" | "left";
      actor_id: string;
      user_ids?: string[];
    }
  | { type: "bottle.message"; thread_id: string; message: BottleMessageView }
  | { type: "bottle.revealed"; thread_id: string; conversation_id: string; peer: User }
  | { type: "error"; message: string; client_msg_id?: string };
