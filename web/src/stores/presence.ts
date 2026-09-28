import { create } from "zustand";
import type { User } from "../api/types";

/** 在线 uid 集合：由 presence.change 事件驱动，rest 响应中的 status 字段做种子。 */
type PresenceState = {
  online: Set<string>;
  setOnline: (uid: string) => void;
  setOffline: (uid: string) => void;
  applyUsers: (users: Pick<User, "id" | "status">[]) => void;
  clear: () => void;
};

export const usePresenceStore = create<PresenceState>((set) => ({
  online: new Set<string>(),
  setOnline: (uid) =>
    set((s) => {
      if (s.online.has(uid)) return s;
      return { online: new Set(s.online).add(uid) };
    }),
  setOffline: (uid) =>
    set((s) => {
      if (!s.online.has(uid)) return s;
      const next = new Set(s.online);
      next.delete(uid);
      return { online: next };
    }),
  applyUsers: (users) =>
    set((s) => {
      let changed = false;
      const next = new Set(s.online);
      for (const u of users) {
        const had = next.has(u.id);
        if (u.status === "online") {
          if (!had) {
            next.add(u.id);
            changed = true;
          }
        } else if (had) {
          next.delete(u.id);
          changed = true;
        }
      }
      return changed ? { online: next } : s;
    }),
  clear: () => set({ online: new Set<string>() }),
}));
