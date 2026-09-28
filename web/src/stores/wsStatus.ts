import { create } from "zustand";
import type { WsStatus } from "../ws/frames";

type WsStatusState = {
  status: WsStatus;
  error: string | null;
  setStatus: (s: WsStatus) => void;
  setError: (e: string | null) => void;
};

export const useWsStatusStore = create<WsStatusState>((set) => ({
  status: "disconnected",
  error: null,
  setStatus: (status) => set({ status, error: null }),
  setError: (error) => set({ error }),
}));
