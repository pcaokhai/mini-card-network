import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

export type DisplayMode = "easy" | "expert";

interface DisplayModeState {
  mode: DisplayMode;
  setMode: (mode: DisplayMode) => void;
}

/** MCN-004-AC2: one app-wide Easy/Expert switch, persisted per browser. */
export const useDisplayMode = create<DisplayModeState>()(
  persist((set) => ({ mode: "easy", setMode: (mode) => set({ mode }) }), {
    name: "mcn.display-mode",
    storage: createJSONStorage(() => localStorage),
  }),
);
