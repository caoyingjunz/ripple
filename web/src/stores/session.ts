import { create } from "zustand";
import { api, getToken, setToken } from "../api/client";
import { setApiBase } from "../api/base";
import type { User } from "../api/types";
import { useConversationsStore } from "./conversations";
import { useFriendsStore } from "./friends";
import { usePresenceStore } from "./presence";

type SessionState = {
  token: string | null;
  user: User | null;
  loading: boolean;
  /** serverBase 仅桌面模式传入（登录/注册页的服务器地址输入）。 */
  login: (username: string, password: string, serverBase?: string) => Promise<void>;
  register: (
    username: string,
    password: string,
    displayName: string,
    serverBase?: string,
  ) => Promise<void>;
  refreshMe: () => Promise<void>;
  logout: () => void;
};

export const useSessionStore = create<SessionState>((set) => ({
  token: getToken(),
  user: null,
  loading: !!getToken(),

  async login(username, password, serverBase) {
    if (serverBase !== undefined) setApiBase(serverBase);
    const res = await api.login(username, password);
    setToken(res.token);
    set({ token: res.token, user: res.user });
    const presence = usePresenceStore.getState();
    presence.applyUsers([res.user]);
    presence.setOnline(res.user.id);
  },

  async register(username, password, displayName, serverBase) {
    if (serverBase !== undefined) setApiBase(serverBase);
    const res = await api.register(username, password, displayName || undefined);
    setToken(res.token);
    set({ token: res.token, user: res.user });
    usePresenceStore.getState().setOnline(res.user.id);
  },

  async refreshMe() {
    const user = await api.me();
    set({ user });
    const presence = usePresenceStore.getState();
    presence.applyUsers([user]);
    presence.setOnline(user.id);
  },

  logout() {
    setToken(null);
    useConversationsStore.getState().reset();
    useFriendsStore.getState().reset();
    usePresenceStore.getState().clear();
    set({ token: null, user: null, loading: false });
  },
}));

/** 启动时校验本地 token（main.tsx 调一次）。 */
export async function bootstrapSession(): Promise<void> {
  if (!getToken()) {
    useSessionStore.setState({ loading: false });
    return;
  }
  try {
    const user = await api.me();
    useSessionStore.setState({ user, loading: false });
    const presence = usePresenceStore.getState();
    presence.applyUsers([user]);
    presence.setOnline(user.id);
  } catch {
    setToken(null);
    useSessionStore.setState({ token: null, loading: false });
  }
}
