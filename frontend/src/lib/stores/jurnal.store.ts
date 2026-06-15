import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { MappingWorkflowStatus, DlqStatus } from "@/lib/schemas/jurnal.schema";

// ---------------------------------------------------------------------------
// Filter state types
// ---------------------------------------------------------------------------

interface MappingFilters {
  q: string;
  kategoriEvent: string;
  workflowStatus: string;
  aktifFlag: string;
  sort: string;
}

interface JurnalFilters {
  q: string;
  periodeId: string;
  eventCode: string;
  statusInternal: string;
  sort: string;
}

interface DlqFilters {
  q: string;
  status: string;
  errorCode: string;
  sort: string;
}

// ---------------------------------------------------------------------------
// Store state
// ---------------------------------------------------------------------------

interface JurnalState {
  // Selected IDs
  selectedMappingId: string | null;
  selectedJurnalId: string | null;
  selectedDlqId: string | null;

  // Current workflow state (for optimistic updates)
  currentMappingStatus: MappingWorkflowStatus | null;
  currentDlqStatus: DlqStatus | null;

  // Filters (persisted per page)
  mappingFilters: MappingFilters;
  jurnalFilters: JurnalFilters;
  dlqFilters: DlqFilters;

  // DLQ badge count (for global notification)
  dlqFailedCount: number;

  // Actions
  setSelectedMappingId: (id: string | null) => void;
  setSelectedJurnalId: (id: string | null) => void;
  setSelectedDlqId: (id: string | null) => void;
  setCurrentMappingStatus: (status: MappingWorkflowStatus | null) => void;
  setCurrentDlqStatus: (status: DlqStatus | null) => void;
  setMappingFilters: (filters: Partial<MappingFilters>) => void;
  setJurnalFilters: (filters: Partial<JurnalFilters>) => void;
  setDlqFilters: (filters: Partial<DlqFilters>) => void;
  setDlqFailedCount: (count: number) => void;
  resetMappingFilters: () => void;
  resetJurnalFilters: () => void;
  resetDlqFilters: () => void;
}

// ---------------------------------------------------------------------------
// Default filters
// ---------------------------------------------------------------------------

const DEFAULT_MAPPING_FILTERS: MappingFilters = {
  q: "",
  kategoriEvent: "",
  workflowStatus: "",
  aktifFlag: "",
  sort: "updated_at:desc",
};

const DEFAULT_JURNAL_FILTERS: JurnalFilters = {
  q: "",
  periodeId: "",
  eventCode: "",
  statusInternal: "",
  sort: "tanggal_posting:desc",
};

const DEFAULT_DLQ_FILTERS: DlqFilters = {
  q: "",
  status: "FAILED",
  errorCode: "",
  sort: "last_attempt_at:desc",
};

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export const useJurnalStore = create<JurnalState>()(
  persist(
    (set) => ({
      selectedMappingId: null,
      selectedJurnalId: null,
      selectedDlqId: null,
      currentMappingStatus: null,
      currentDlqStatus: null,
      mappingFilters: DEFAULT_MAPPING_FILTERS,
      jurnalFilters: DEFAULT_JURNAL_FILTERS,
      dlqFilters: DEFAULT_DLQ_FILTERS,
      dlqFailedCount: 0,

      setSelectedMappingId: (id) => set({ selectedMappingId: id }),
      setSelectedJurnalId: (id) => set({ selectedJurnalId: id }),
      setSelectedDlqId: (id) => set({ selectedDlqId: id }),
      setCurrentMappingStatus: (status) => set({ currentMappingStatus: status }),
      setCurrentDlqStatus: (status) => set({ currentDlqStatus: status }),

      setMappingFilters: (filters) =>
        set((s) => ({ mappingFilters: { ...s.mappingFilters, ...filters } })),
      setJurnalFilters: (filters) =>
        set((s) => ({ jurnalFilters: { ...s.jurnalFilters, ...filters } })),
      setDlqFilters: (filters) =>
        set((s) => ({ dlqFilters: { ...s.dlqFilters, ...filters } })),

      setDlqFailedCount: (count) => set({ dlqFailedCount: count }),

      resetMappingFilters: () => set({ mappingFilters: DEFAULT_MAPPING_FILTERS }),
      resetJurnalFilters: () => set({ jurnalFilters: DEFAULT_JURNAL_FILTERS }),
      resetDlqFilters: () => set({ dlqFilters: DEFAULT_DLQ_FILTERS }),
    }),
    {
      name: "blips-jurnal-store",
      partialize: (s) => ({
        mappingFilters: s.mappingFilters,
        jurnalFilters: s.jurnalFilters,
        dlqFilters: s.dlqFilters,
      }),
    },
  ),
);
