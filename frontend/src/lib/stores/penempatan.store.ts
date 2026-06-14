import { create } from "zustand";
import type { PenempatanDeposito, EirPreviewResult } from "@/lib/schemas/penempatan.schema";
import type { JobState } from "@/components/blips/JobProgressPanel";

// ---------------------------------------------------------------------------
// Filter shape
// ---------------------------------------------------------------------------

export interface PenempatanFilters {
  q?: string;
  workflowStatus?: string;
  counterpartyBankId?: string;
  tipeInstrumen?: string;
  klasifikasiPsak71?: string;
  tanggalPenempatan?: string;
  periodeId?: string;
  nominalIdr?: string;
}

export interface SortSpec {
  col: string;
  dir: "asc" | "desc";
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

interface PenempatanState {
  // List state
  listFilters: PenempatanFilters;
  listSort: SortSpec[];
  listCursor: string | null;
  setListFilters: (filters: Partial<PenempatanFilters>) => void;
  clearListFilters: () => void;
  setListSort: (sort: SortSpec[]) => void;
  setListCursor: (cursor: string | null) => void;

  // Detail state
  currentPenempatan: PenempatanDeposito | null;
  setCurrentPenempatan: (p: PenempatanDeposito | null) => void;

  // EIR preview state
  eirPreview: EirPreviewResult | null;
  eirPreviewLoading: boolean;
  setEirPreview: (result: EirPreviewResult | null) => void;
  setEirPreviewLoading: (loading: boolean) => void;

  // Active jobs (EIR compute, export, etc)
  activeJobs: Record<string, JobState>;
  updateJob: (jobId: string, state: Partial<JobState>) => void;
  clearJob: (jobId: string) => void;

  // Current workflow action loading
  actionLoading: boolean;
  setActionLoading: (loading: boolean) => void;
}

export const usePenempatanStore = create<PenempatanState>((set) => ({
  // List
  listFilters: {},
  listSort: [{ col: "created_at", dir: "desc" }],
  listCursor: null,
  setListFilters: (filters) =>
    set((state) => ({ listFilters: { ...state.listFilters, ...filters }, listCursor: null })),
  clearListFilters: () => set({ listFilters: {}, listCursor: null }),
  setListSort: (sort) => set({ listSort: sort, listCursor: null }),
  setListCursor: (cursor) => set({ listCursor: cursor }),

  // Detail
  currentPenempatan: null,
  setCurrentPenempatan: (p) => set({ currentPenempatan: p }),

  // EIR preview
  eirPreview: null,
  eirPreviewLoading: false,
  setEirPreview: (result) => set({ eirPreview: result }),
  setEirPreviewLoading: (loading) => set({ eirPreviewLoading: loading }),

  // Jobs
  activeJobs: {},
  updateJob: (jobId, jobState) =>
    set((state) => ({
      activeJobs: {
        ...state.activeJobs,
        [jobId]: { ...state.activeJobs[jobId], ...jobState } as JobState,
      },
    })),
  clearJob: (jobId) =>
    set((state) => {
      const { [jobId]: _removed, ...rest } = state.activeJobs;
      return { activeJobs: rest };
    }),

  // Action loading
  actionLoading: false,
  setActionLoading: (loading) => set({ actionLoading: loading }),
}));
