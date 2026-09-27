import type { Bottle, BottleMessage, BottleThread, Conversation, Message, User } from "./types";

const TOKEN_KEY = "ripple_token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null) {
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
  const res = await fetch(path, { ...init, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || res.statusText);
  }
  return data as T;
}

export const api = {
  login: (username: string, password: string) =>
    request<{ token: string; user: User }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  me: () => request<User>("/api/me"),
  conversations: () => request<{ conversations: Conversation[] }>("/api/conversations"),
  messages: (id: string) => request<{ messages: Message[] }>(`/api/conversations/${id}/messages`),
  sendMessage: (id: string, body: { type: string; body: string; image_url?: string }) =>
    request<Message>(`/api/conversations/${id}/messages`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  upload: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return request<{ url: string }>("/api/media/upload", { method: "POST", body: fd });
  },
  throwBottle: (content: string) =>
    request<Bottle>("/api/bottles", { method: "POST", body: JSON.stringify({ content }) }),
  pickBottle: () => request<BottleThread>("/api/bottles/pick", { method: "POST", body: "{}" }),
  myBottles: () => request<{ bottles: Bottle[] }>("/api/bottles/mine"),
  getThread: (id: string) => request<BottleThread>(`/api/bottles/threads/${id}`),
  bottleMessage: (id: string, body: string) =>
    request<BottleMessage>(`/api/bottles/threads/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ body }),
    }),
  reveal: (id: string) =>
    request<{ conversation_id: string; peer: User }>(`/api/bottles/threads/${id}/reveal`, {
      method: "POST",
      body: "{}",
    }),
  closeThread: (id: string) =>
    request<{ status: string }>(`/api/bottles/threads/${id}/close`, {
      method: "POST",
      body: "{}",
    }),
};
