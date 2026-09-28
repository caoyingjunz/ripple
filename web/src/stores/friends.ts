import { create } from "zustand";
import { api } from "../api/client";
import type { FriendRequest, FriendsOverview } from "../api/types";
import { usePresenceStore } from "./presence";

type FriendsState = FriendsOverview & {
  loaded: boolean;
  refreshing: boolean;
  refresh: () => Promise<void>;
  applyRequestEvent: (action: "pending" | "accepted", request: FriendRequest) => void;
  reset: () => void;
};

export const useFriendsStore = create<FriendsState>((set) => ({
  friends: [],
  pending_in: [],
  pending_out: [],
  blocked: [],
  loaded: false,
  refreshing: false,

  async refresh() {
    if (useFriendsStore.getState().refreshing) return;
    set({ refreshing: true });
    try {
      const r = await api.friends();
      usePresenceStore.getState().applyUsers(r.friends);
      set({
        friends: r.friends,
        pending_in: r.pending_in,
        pending_out: r.pending_out,
        blocked: r.blocked,
        loaded: true,
      });
    } finally {
      set({ refreshing: false });
    }
  },

  applyRequestEvent(action, request) {
    set((s) => {
      if (action === "pending") {
        if (s.pending_in.some((r) => r.id === request.id)) return s;
        return { pending_in: [...s.pending_in, request] };
      }
      // accepted：request.user = 新好友
      const friends = s.friends.some((f) => f.id === request.user.id)
        ? s.friends
        : [...s.friends, request.user];
      return {
        friends,
        pending_out: s.pending_out.filter((r) => r.user.id !== request.user.id),
        pending_in: s.pending_in.filter((r) => r.user.id !== request.user.id),
      };
    });
    usePresenceStore.getState().applyUsers([request.user]);
  },

  reset() {
    set({
      friends: [],
      pending_in: [],
      pending_out: [],
      blocked: [],
      loaded: false,
      refreshing: false,
    });
  },
}));
