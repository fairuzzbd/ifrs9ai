/**
 * Zustand store for Roll-Forward CKPN (P4-M11).
 *
 * Persists selected runs and filter state for deep-link friendly navigation.
 */

import { create } from "zustand";

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

interface RollForwardFilter {
  bucket?: string;
  overrideFlag?: boolean;
  q?: string;
}

interface RollForwardState {
  /** Currently viewed report ID */
  selectedReportId: string | null;
  setSelectedReportId: (id: string | null) => void;

  /** Current calc run being compared */
  currentCalcRunId: string | null;
  setCurrentCalcRunId: (id: string | null) => void;

  /** Prior calc run for comparison (null = first period) */
  priorCalcRunId: string | null;
  setPriorCalcRunId: (id: string | null) => void;

  /** Drill-down filter state for portfolio instrument table */
  portfolioFilter: RollForwardFilter;
  setPortfolioFilter: (filter: Partial<RollForwardFilter>) => void;
  clearPortfolioFilter: () => void;

  /** CKPN trend periods selector */
  trendPeriods: number;
  setTrendPeriods: (periods: number) => void;

  /** Selected portfolio for trend filter */
  trendPortofolioId: string | null;
  setTrendPortofolioId: (id: string | null) => void;

  /** Reset all state */
  reset: () => void;
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

const DEFAULT_FILTER: RollForwardFilter = {};

export const useRollForwardStore = create<RollForwardState>()((set) => ({
  selectedReportId: null,
  setSelectedReportId: (id) => set({ selectedReportId: id }),

  currentCalcRunId: null,
  setCurrentCalcRunId: (id) => set({ currentCalcRunId: id }),

  priorCalcRunId: null,
  setPriorCalcRunId: (id) => set({ priorCalcRunId: id }),

  portfolioFilter: DEFAULT_FILTER,
  setPortfolioFilter: (filter) =>
    set((s) => ({ portfolioFilter: { ...s.portfolioFilter, ...filter } })),
  clearPortfolioFilter: () => set({ portfolioFilter: DEFAULT_FILTER }),

  trendPeriods: 12,
  setTrendPeriods: (periods) =>
    set({ trendPeriods: Math.min(24, Math.max(2, periods)) }),

  trendPortofolioId: null,
  setTrendPortofolioId: (id) => set({ trendPortofolioId: id }),

  reset: () =>
    set({
      selectedReportId: null,
      currentCalcRunId: null,
      priorCalcRunId: null,
      portfolioFilter: DEFAULT_FILTER,
      trendPeriods: 12,
      trendPortofolioId: null,
    }),
}));
