import { getApiBase } from "./base";
import type {
  Bottle,
  BottleMessageView,
  BottleThreadView,
  ConversationDetail,
  ConversationSummary,
  FriendsOverview,
  Message,
  User,
  UserSearchItem,
} from "./types";

const TOKEN_KEY = "ripple_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null): void {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(getApiBase() + path, { ...init, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || res.statusText);
  }
  return data as T;
}

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

export const api = {
  // ---- 鉴权与资料（§3.1） ----
  login: (username: string, password: string) =>
    request<{ token: string; user: User }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  register: (username: string, password: string, display_name?: string) =>
    request<{ token: string; user: User }>("/api/auth/register", {
      method: "POST",
      body: JSON.stringify({ username, password, display_name }),
    }),
  me: () => request<User>("/api/me"),
  updateMe: (patch: { display_name?: string; signature?: string }) =>
    request<User>("/api/me", { method: "PUT", body: JSON.stringify(patch) }),
  uploadAvatar: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return request<{ avatar_url: string }>("/api/me/avatar", { method: "POST", body: fd });
  },

  // ---- 用户与好友（§3.2） ----
  searchUsers: (q: string) =>
    request<{ users: UserSearchItem[] }>(`/api/users/search${query({ q })}`),
  friends: () => request<FriendsOverview>("/api/friends"),
  addFriend: (user_id: string) =>
    request<{ status: "pending" | "accepted"; friend: User }>("/api/friends/requests", {
      method: "POST",
      body: JSON.stringify({ user_id }),
    }),
  acceptFriend: (id: string) =>
    request<{ friend: User }>(`/api/friends/requests/${id}/accept`, { method: "POST", body: "{}" }),
  declineFriend: (id: string) =>
    request<{ status: "declined" }>(`/api/friends/requests/${id}/decline`, {
      method: "POST",
      body: "{}",
    }),
  deleteFriend: (user_id: string) =>
    request<{ status: "removed" }>(`/api/friends/${user_id}`, { method: "DELETE" }),
  blockFriend: (user_id: string) =>
    request<{ status: "blocked" }>(`/api/friends/${user_id}/block`, { method: "POST", body: "{}" }),
  unblockFriend: (user_id: string) =>
    request<{ status: "unblocked" }>(`/api/friends/${user_id}/block`, { method: "DELETE" }),

  // ---- 会话（§3.3） ----
  conversations: () => request<{ conversations: ConversationSummary[] }>("/api/conversations"),
  createSingle: (user_id: string) =>
    request<ConversationSummary>("/api/conversations", {
      method: "POST",
      body: JSON.stringify({ type: "single", user_id }),
    }),
  createGroup: (name: string, member_ids: string[]) =>
    request<ConversationSummary>("/api/conversations", {
      method: "POST",
      body: JSON.stringify({ type: "group", name, member_ids }),
    }),
  conversationDetail: (id: string) => request<ConversationDetail>(`/api/conversations/${id}`),
  renameConversation: (id: string, name: string) =>
    request<ConversationSummary>(`/api/conversations/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name }),
    }),
  addMembers: (id: string, user_ids: string[]) =>
    request<{ members: User[] }>(`/api/conversations/${id}/members`, {
      method: "POST",
      body: JSON.stringify({ user_ids }),
    }),
  removeMember: (id: string, uid: string) =>
    request<{ status: "removed" }>(`/api/conversations/${id}/members/${uid}`, {
      method: "DELETE",
    }),
  leaveConversation: (id: string) =>
    request<{ status: "left" }>(`/api/conversations/${id}/leave`, { method: "POST", body: "{}" }),
  fetchMessages: (
    id: string,
    opts: { after_seq?: number; before_seq?: number; limit?: number } = {},
  ) =>
    request<{ messages: Message[]; has_more: boolean }>(
      `/api/conversations/${id}/messages${query(opts)}`,
    ),
  sendMessage: (id: string, body: { type: "text" | "image"; body: string; image_url?: string }) =>
    request<Message>(`/api/conversations/${id}/messages`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  markRead: (id: string, seq: number) =>
    request<{ last_read_seq: number }>(`/api/conversations/${id}/read`, {
      method: "POST",
      body: JSON.stringify({ seq }),
    }),
  revokeMessage: (id: string) =>
    request<{ message: Message }>(`/api/messages/${id}/revoke`, { method: "POST", body: "{}" }),

  // ---- 媒体 ----
  upload: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return request<{ url: string }>("/api/media/upload", { method: "POST", body: fd });
  },

  // ---- 漂流瓶（§3.4） ----
  throwBottle: (content: string) =>
    request<Bottle>("/api/bottles", { method: "POST", body: JSON.stringify({ content }) }),
  pickBottle: () => request<BottleThreadView>("/api/bottles/pick", { method: "POST", body: "{}" }),
  myBottles: () => request<{ bottles: Bottle[] }>("/api/bottles/mine"),
  getThread: (id: string) => request<BottleThreadView>(`/api/bottles/threads/${id}`),
  bottleMessage: (id: string, body: string) =>
    request<BottleMessageView>(`/api/bottles/threads/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ body }),
    }),
  reveal: (id: string) =>
    request<{ conversation_id: string; peer: User }>(`/api/bottles/threads/${id}/reveal`, {
      method: "POST",
      body: "{}",
    }),
  closeThread: (id: string) =>
    request<{ status: string }>(`/api/bottles/threads/${id}/close`, { method: "POST", body: "{}" }),
};
