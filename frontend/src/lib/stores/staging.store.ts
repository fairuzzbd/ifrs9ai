import { create } from "zustand";
import type { StagingOverrideProposal } from "@/lib/schemas/staging.schema";

interface StagingState {
  /** Instrumen ID yang sedang dilihat staging-nya */
  selectedInstrumenId: string | null;
  setSelectedInstrumenId: (id: string | null) => void;

  /** Override proposal yang sedang dalam proses workflow */
  currentOverrideProposal: StagingOverrideProposal | null;
  setCurrentOverrideProposal: (p: StagingOverrideProposal | null) => void;

  /** Job ID re-staging yang sedang berjalan */
  activeJobId: string | null;
  setActiveJobId: (jobId: string | null) => void;
}

export const useStagingStore = create<StagingState>((set) => ({
  selectedInstrumenId: null,
  setSelectedInstrumenId: (id) => set({ selectedInstrumenId: id }),

  currentOverrideProposal: null,
  setCurrentOverrideProposal: (p) => set({ currentOverrideProposal: p }),

  activeJobId: null,
  setActiveJobId: (jobId) => set({ activeJobId: jobId }),
}));
