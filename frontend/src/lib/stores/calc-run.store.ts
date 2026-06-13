import { create } from "zustand";
import type { CalcRun } from "@/lib/schemas/calc-run.schema";

export type SealModalState =
  | "closed"
  | "request"
  | "approve-confirm"
  | "approve-mfa"
  | "reject"
  | "cancel";

export interface CalcRunListFilters {
  periodeId: string;
  status: string;
  createdBy: string;
  q: string;
}

interface CalcRunState {
  /** Currently viewed calc run detail */
  activeCalcRun: CalcRun | null;
  setActiveCalcRun: (run: CalcRun | null) => void;

  /** Seal workflow modal state machine */
  sealModalState: SealModalState;
  setSealModalState: (state: SealModalState) => void;

  /** Comment accumulated in approve-confirm step before MFA */
  pendingApproveComment: string;
  setPendingApproveComment: (c: string) => void;

  /** Active background job ID */
  activeJobId: string | null;
  setActiveJobId: (jobId: string | null) => void;

  /** List filter state */
  listFilters: CalcRunListFilters;
  setListFilters: (f: Partial<CalcRunListFilters>) => void;

  /** Create modal open */
  createModalOpen: boolean;
  setCreateModalOpen: (open: boolean) => void;
}

export const useCalcRunStore = create<CalcRunState>((set) => ({
  activeCalcRun: null,
  setActiveCalcRun: (run) => set({ activeCalcRun: run }),

  sealModalState: "closed",
  setSealModalState: (state) => set({ sealModalState: state }),

  pendingApproveComment: "",
  setPendingApproveComment: (c) => set({ pendingApproveComment: c }),

  activeJobId: null,
  setActiveJobId: (jobId) => set({ activeJobId: jobId }),

  listFilters: { periodeId: "", status: "", createdBy: "", q: "" },
  setListFilters: (f) =>
    set((s) => ({ listFilters: { ...s.listFilters, ...f } })),

  createModalOpen: false,
  setCreateModalOpen: (open) => set({ createModalOpen: open }),
}));
