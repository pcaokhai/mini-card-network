import { create } from "zustand";

export interface IsoFieldVM {
  de: string;
  easyName: string;
  technicalName: string;
  format: string;
  value: string;
  raw?: string;
}

export interface SegmentVM {
  key: string;
  text: string;
}

export interface DecodedMessage {
  mti: string;
  primaryBitmap: string;
  secondaryBitmap: string | null;
  segments: SegmentVM[];
  fields: IsoFieldVM[];
}

interface MessageLabState {
  raw: string;
  decoded: DecodedMessage | null;
  selectedKey: string | null;
  setRaw: (raw: string) => void;
  setDecoded: (decoded: DecodedMessage | null) => void;
  select: (key: string | null) => void;
}

/** MCN-104-AC1: one selection, shared by RawSegments, BitmapGrid and FieldTable. */
export const useMessageLab = create<MessageLabState>()((set) => ({
  raw: "",
  decoded: null,
  selectedKey: null,
  setRaw: (raw) => set({ raw }),
  setDecoded: (decoded) => set({ decoded, selectedKey: null }),
  select: (key) => set({ selectedKey: key }),
}));
