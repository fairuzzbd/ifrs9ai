import { create } from "zustand";
import type { EIRAmendmentProposal } from "@/lib/schemas/eir.schema";

interface EIRState {
  /** Instrumen ID yang sedang dilihat EIR-nya */
  selectedInstrumenId: string | null;
  setSelectedInstrumenId: (id: string | null) => void;

  /** Amendment proposal yang sedang dalam proses workflow */
  currentAmendmentProposal: EIRAmendmentProposal | null;
  setCurrentAmendmentProposal: (p: EIRAmendmentProposal | null) => void;

  /** Versi schedule yang sedang dipilih di version selector */
  selectedScheduleVersion: number | null;
  setSelectedScheduleVersion: (v: number | null) => void;

  /** Job ID drift detection / bulk recompute yang sedang berjalan */
  activeDriftJobId: string | null;
  setActiveDriftJobId: (jobId: string | null) => void;
}

export const useEIRStore = create<EIRState>((set) => ({
  selectedInstrumenId: null,
  setSelectedInstrumenId: (id) => set({ selectedInstrumenId: id }),

  currentAmendmentProposal: null,
  setCurrentAmendmentProposal: (p) => set({ currentAmendmentProposal: p }),

  selectedScheduleVersion: null,
  setSelectedScheduleVersion: (v) => set({ selectedScheduleVersion: v }),

  activeDriftJobId: null,
  setActiveDriftJobId: (jobId) => set({ activeDriftJobId: jobId }),
}));
