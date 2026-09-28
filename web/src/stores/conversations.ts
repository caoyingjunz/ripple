import { create } from "zustand";
import { api } from "../api/client";
import type { ConversationSummary, Message, MessageType } from "../api/types";
import { usePresenceStore } from "./presence";

/**
 * 聊天条目：真实消息（已落库、有 seq/id）或乐观 pending（client_msg_id 关联 ack）。
 * pending 排在真实消息之后，ack 到达后按 client_msg_id 合并替换。
 */
export type ChatEntry =
  | { kind: "real"; message: Message }
  | {
      kind: "pending";
      client_msg_id: string;
      convId: string;
      type: MessageType;
      body: string;
      image_url?: string;
      created_at: number;
      failed: boolean;
    };

type RealEntry = { kind: "real"; message: Message };

function isReal(e: ChatEntry): e is RealEntry {
  return e.kind === "real";
}

function entrySeq(e: ChatEntry): number {
  return e.kind === "real" ? e.message.seq : Number.MAX_SAFE_INTEGER;
}

function entryTime(e: ChatEntry): number {
  return e.kind === "real" ? e.message.created_at : e.created_at;
}

function sortEntries(entries: ChatEntry[]): ChatEntry[] {
  return [...entries].sort((a, b) => entrySeq(a) - entrySeq(b) || entryTime(a) - entryTime(b));
}

export function previewOf(m: Pick<Message, "type" | "body" | "revoked_at">): string {
  if (m.revoked_at) return "[已撤回]";
  if (m.type === "image") return "[图片]";
  if (m.type === "system") return m.body;
  const body = m.body.trim();
  return body.length > 40 ? `${body.slice(0, 40)}…` : body;
}

function sortList(list: ConversationSummary[]): ConversationSummary[] {
  return [...list].sort(
    (a, b) =>
      Number(b.pinned) - Number(a.pinned) ||
      (b.last_message_at ?? b.created_at) - (a.last_message_at ?? a.created_at),
  );
}

let refreshTimer: number | null = null;

type ConversationsState = {
  list: ConversationSummary[];
  listLoaded: boolean;
  listError: string | null;
  currentId: string | null;
  openError: string | null;
  messagesByConv: Record<string, ChatEntry[]>;
  hasMoreByConv: Record<string, boolean>;
  loadingOlderByConv: Record<string, boolean>;

  refreshList: () => Promise<void>;
  /** WS 事件后的节流重拉（600ms 内合并突发）。 */
  scheduleRefresh: () => void;
  openConversation: (id: string) => Promise<void>;
  closeConversation: () => void;
  loadOlder: (id: string) => Promise<boolean>;
  addPending: (
    convId: string,
    p: { client_msg_id: string; type: MessageType; body: string; image_url?: string },
  ) => void;
  removePending: (convId: string, clientMsgId: string) => void;
  ackPending: (clientMsgId: string, message: Message) => void;
  failPending: (clientMsgId: string) => void;
  /** 断线时把仍处于 pending 的乐观消息标记为失败（结果未知，可重发）。 */
  markPendingIndeterminate: () => void;
  mergeIncoming: (m: Message, mine: boolean) => void;
  mergeBatch: (convId: string, messages: Message[], mineId: string | null) => void;
  applyRevoked: (conversationId: string, messageId: string, revokedAt: number) => void;
  markReadIfBehind: (convId: string) => Promise<void>;
  /** 断线重连补拉：对已加载消息的会话按 after_seq 增量补齐（按 message.id 去重）。 */
  catchUpAll: (mineId: string | null) => Promise<void>;
  reset: () => void;
};

export const useConversationsStore = create<ConversationsState>((set, get) => ({
  list: [],
  listLoaded: false,
  listError: null,
  currentId: null,
  openError: null,
  messagesByConv: {},
  hasMoreByConv: {},
  loadingOlderByConv: {},

  async refreshList() {
    try {
      const r = await api.conversations();
      const peers = r.conversations
        .map((c) => c.peer)
        .filter((p): p is NonNullable<typeof p> => !!p);
      usePresenceStore.getState().applyUsers(peers);
      set({ list: sortList(r.conversations), listLoaded: true, listError: null });
    } catch (err) {
      set({ listLoaded: true, listError: err instanceof Error ? err.message : "加载会话失败" });
    }
  },

  scheduleRefresh() {
    if (refreshTimer !== null) return;
    refreshTimer = window.setTimeout(() => {
      refreshTimer = null;
      void useConversationsStore.getState().refreshList();
    }, 600);
  },

  async openConversation(id) {
    set({ currentId: id, openError: null });
    if (!get().listLoaded) await get().refreshList();
    if (!get().list.some((c) => c.id === id)) {
      // 深链直开：列表里还没有该会话，拉明细补进去
      try {
        const d = await api.conversationDetail(id);
        usePresenceStore.getState().applyUsers(d.members.map((m) => m.user));
        set((s) => ({
          list: sortList([d.conversation, ...s.list.filter((c) => c.id !== id)]),
        }));
      } catch (err) {
        set({ openError: err instanceof Error ? err.message : "会话不存在或无权访问" });
        return;
      }
    }
    if (!get().messagesByConv[id]) {
      try {
        const r = await api.fetchMessages(id, { limit: 50 });
        set((s) => ({
          messagesByConv: {
            ...s.messagesByConv,
            [id]: r.messages.map((m) => ({ kind: "real" as const, message: m })),
          },
          hasMoreByConv: { ...s.hasMoreByConv, [id]: r.has_more },
        }));
      } catch (err) {
        set({ openError: err instanceof Error ? err.message : "加载消息失败" });
        return;
      }
    }
    void get().markReadIfBehind(id);
  },

  closeConversation() {
    set({ currentId: null });
  },

  async loadOlder(id) {
    const s = get();
    if (s.loadingOlderByConv[id]) return false;
    const arr = s.messagesByConv[id];
    const seqs = (arr ?? []).filter(isReal).map((e) => e.message.seq);
    if (!seqs.length) return false;
    set((st) => ({ loadingOlderByConv: { ...st.loadingOlderByConv, [id]: true } }));
    try {
      const r = await api.fetchMessages(id, { before_seq: Math.min(...seqs), limit: 50 });
      set((st) => {
        const cur = st.messagesByConv[id] ?? [];
        const known = new Set(cur.filter(isReal).map((e) => e.message.id));
        const older = r.messages
          .filter((m) => !known.has(m.id))
          .map((m) => ({ kind: "real" as const, message: m }));
        return {
          messagesByConv: { ...st.messagesByConv, [id]: sortEntries([...older, ...cur]) },
          hasMoreByConv: { ...st.hasMoreByConv, [id]: r.has_more },
          loadingOlderByConv: { ...st.loadingOlderByConv, [id]: false },
        };
      });
      return true;
    } catch {
      set((st) => ({ loadingOlderByConv: { ...st.loadingOlderByConv, [id]: false } }));
      return false;
    }
  },

  addPending(convId, p) {
    set((s) => {
      const arr = s.messagesByConv[convId] ?? [];
      const entry: ChatEntry = {
        kind: "pending",
        ...p,
        convId,
        created_at: Date.now(),
        failed: false,
      };
      return { messagesByConv: { ...s.messagesByConv, [convId]: [...arr, entry] } };
    });
  },

  removePending(convId, clientMsgId) {
    set((s) => {
      const arr = s.messagesByConv[convId];
      if (!arr) return s;
      return {
        messagesByConv: {
          ...s.messagesByConv,
          [convId]: arr.filter((e) => !(e.kind === "pending" && e.client_msg_id === clientMsgId)),
        },
      };
    });
  },

  ackPending(clientMsgId, message) {
    set((s) => {
      const arr = s.messagesByConv[message.conversation_id];
      if (!arr) return s;
      const rest = arr.filter(
        (e) => !(e.kind === "pending" && e.client_msg_id === clientMsgId),
      );
      const exists = rest.some((e) => isReal(e) && e.message.id === message.id);
      const next = exists ? rest : sortEntries([...rest, { kind: "real", message }]);
      const list = s.list.some((c) => c.id === message.conversation_id)
        ? sortList(
            s.list.map((c) =>
              c.id === message.conversation_id
                ? {
                    ...c,
                    last_seq: Math.max(c.last_seq, message.seq),
                    last_message_preview: previewOf(message),
                    last_message_at: message.created_at,
                  }
                : c,
            ),
          )
        : s.list;
      return {
        messagesByConv: { ...s.messagesByConv, [message.conversation_id]: next },
        list,
      };
    });
  },

  failPending(clientMsgId) {
    set((s) => {
      let changed = false;
      const nextByConv: Record<string, ChatEntry[]> = {};
      for (const [cid, arr] of Object.entries(s.messagesByConv)) {
        const marked = arr.map((e) => {
          if (e.kind === "pending" && e.client_msg_id === clientMsgId && !e.failed) {
            changed = true;
            return { ...e, failed: true };
          }
          return e;
        });
        nextByConv[cid] = marked;
      }
      return changed ? { messagesByConv: nextByConv } : s;
    });
  },

  markPendingIndeterminate() {
    set((s) => {
      let changed = false;
      const nextByConv: Record<string, ChatEntry[]> = {};
      for (const [cid, arr] of Object.entries(s.messagesByConv)) {
        const marked = arr.map((e) => {
          if (e.kind === "pending" && !e.failed) {
            changed = true;
            return { ...e, failed: true };
          }
          return e;
        });
        nextByConv[cid] = marked;
      }
      return changed ? { messagesByConv: nextByConv } : s;
    });
  },

  mergeIncoming(m, mine) {
    const alreadyInList = get().list.some((c) => c.id === m.conversation_id);
    set((s) => {
      const arr = s.messagesByConv[m.conversation_id];
      if (arr && arr.some((e) => isReal(e) && e.message.id === m.id)) return s; // 去重
      const messagesByConv = arr
        ? {
            ...s.messagesByConv,
            [m.conversation_id]: sortEntries([...arr, { kind: "real", message: m }]),
          }
        : s.messagesByConv;
      const isOpen = s.currentId === m.conversation_id;
      const list = alreadyInList
        ? sortList(
            s.list.map((c) =>
              c.id === m.conversation_id
                ? {
                    ...c,
                    last_seq: Math.max(c.last_seq, m.seq),
                    last_message_preview: previewOf(m),
                    last_message_at: m.created_at,
                    unread: mine || isOpen ? c.unread : c.unread + 1,
                  }
                : c,
            ),
          )
        : s.list;
      return { messagesByConv, list };
    });
    if (!alreadyInList) get().scheduleRefresh();
  },

  mergeBatch(convId, messages, mineId) {
    set((s) => {
      const arr = s.messagesByConv[convId];
      if (!arr) return s;
      const known = new Set(arr.filter(isReal).map((e) => e.message.id));
      const fresh = messages.filter((m) => !known.has(m.id)); // 按 message.id 去重
      if (!fresh.length) return s;
      const entries = sortEntries([
        ...arr,
        ...fresh.map((m) => ({ kind: "real" as const, message: m })),
      ]);
      const lastMsg = fresh.reduce((acc, m) => (m.seq > acc.seq ? m : acc), fresh[0]);
      const notMineCount = fresh.filter((m) => m.sender_id !== mineId).length;
      const isOpen = s.currentId === convId;
      const list = s.list.some((c) => c.id === convId)
        ? sortList(
            s.list.map((c) =>
              c.id === convId
                ? {
                    ...c,
                    last_seq: Math.max(c.last_seq, lastMsg.seq),
                    last_message_preview: previewOf(lastMsg),
                    last_message_at: lastMsg.created_at,
                    unread: isOpen ? c.unread : c.unread + notMineCount,
                  }
                : c,
            ),
          )
        : s.list;
      return { messagesByConv: { ...s.messagesByConv, [convId]: entries }, list };
    });
  },

  applyRevoked(conversationId, messageId, revokedAt) {
    set((s) => {
      const arr = s.messagesByConv[conversationId];
      const target = arr?.find((e) => isReal(e) && e.message.id === messageId);
      if (!target || !isReal(target)) return s;
      const messagesByConv = {
        ...s.messagesByConv,
        [conversationId]: (arr ?? []).map((e) =>
          isReal(e) && e.message.id === messageId
            ? { kind: "real" as const, message: { ...e.message, revoked_at: revokedAt, body: "", image_url: null } }
            : e,
        ),
      };
      const list = s.list.map((c) =>
        c.id === conversationId && c.last_seq === target.message.seq
          ? { ...c, last_message_preview: "[已撤回]" }
          : c,
      );
      return { messagesByConv, list };
    });
  },

  async markReadIfBehind(convId) {
    const s = get();
    const conv = s.list.find((c) => c.id === convId);
    const realSeqs = (s.messagesByConv[convId] ?? []).filter(isReal).map((e) => e.message.seq);
    const target = Math.max(conv?.last_seq ?? 0, ...realSeqs, 0);
    if (!target || target <= (conv?.last_read_seq ?? 0)) {
      if (conv && conv.unread > 0) {
        set((st) => ({ list: st.list.map((c) => (c.id === convId ? { ...c, unread: 0 } : c)) }));
      }
      return;
    }
    try {
      const r = await api.markRead(convId, target);
      set((st) => ({
        list: st.list.map((c) =>
          c.id === convId
            ? { ...c, unread: 0, last_read_seq: Math.max(c.last_read_seq, r.last_read_seq) }
            : c,
        ),
      }));
    } catch {
      /* 已读上报失败不影响本地阅读 */
    }
  },

  async catchUpAll(mineId) {
    const s = get();
    const jobs: Promise<void>[] = [];
    for (const [convId, arr] of Object.entries(s.messagesByConv)) {
      const seqs = arr.filter(isReal).map((e) => e.message.seq);
      if (!seqs.length) continue;
      const after = Math.max(...seqs);
      jobs.push(
        api
          .fetchMessages(convId, { after_seq: after, limit: 100 })
          .then((r) => useConversationsStore.getState().mergeBatch(convId, r.messages, mineId))
          .catch(() => undefined),
      );
    }
    await Promise.all(jobs);
  },

  reset() {
    if (refreshTimer !== null) {
      window.clearTimeout(refreshTimer);
      refreshTimer = null;
    }
    set({
      list: [],
      listLoaded: false,
      listError: null,
      currentId: null,
      openError: null,
      messagesByConv: {},
      hasMoreByConv: {},
      loadingOlderByConv: {},
    });
  },
}));
