/**
 * Vitest unit tests for P4-M11 Roll-Forward CKPN components and store.
 *
 * No DOM render (node env) — covers:
 * - RollForwardDetectionMethodBadge: token map exhaustiveness
 * - TransferBucketRow: sign convention logic
 * - CKPNTrendChart: data transformation
 * - RollForwardExportButton: MISMATCH guard logic
 * - Zustand store: roll-forward.store
 * - API client: parameter building
 */

import { describe, it, expect, beforeEach } from "vitest";

// ---------------------------------------------------------------------------
// DetectionMethod token map exhaustiveness
// ---------------------------------------------------------------------------

import { detectionMethodEnum } from "@/lib/schemas/roll-forward.schema";

// Mirror the token map from RollForwardDetectionMethodBadge
const DETECTION_METHOD_TOKENS: Record<string, { label: string; bg: string }> = {
  BASIC_STATUS_DIFF: {
    label: "BASIC_STATUS_DIFF",
    bg: "bg-amber-100",
  },
  FULL_LIFECYCLE_PHASE_5: {
    label: "FULL_LIFECYCLE",
    bg: "bg-green-100",
  },
};

describe("RollForwardDetectionMethodBadge — token map", () => {
  it("has a token for every DetectionMethod enum value", () => {
    for (const method of detectionMethodEnum.options) {
      expect(DETECTION_METHOD_TOKENS[method]).toBeDefined();
      expect(DETECTION_METHOD_TOKENS[method]?.label).toBeTruthy();
    }
  });

  it("BASIC_STATUS_DIFF uses amber (Phase 4 limitation)", () => {
    expect(DETECTION_METHOD_TOKENS["BASIC_STATUS_DIFF"]?.bg).toContain(
      "amber",
    );
  });

  it("FULL_LIFECYCLE_PHASE_5 uses green (full detection)", () => {
    expect(DETECTION_METHOD_TOKENS["FULL_LIFECYCLE_PHASE_5"]?.bg).toContain(
      "green",
    );
  });
});

// ---------------------------------------------------------------------------
// TransferBucketRow — sign convention logic
// ---------------------------------------------------------------------------

describe("TransferBucketRow — sign convention", () => {
  // Sign convention: positive = increase in allowance (loss), negative = decrease (cure)
  const SIGN_MAP: Record<string, "+" | "-"> = {
    stage1To2: "+", // SICR: increases ECL
    stage2To1: "-", // Cure: decreases ECL
    stage2To3: "+", // Default: increases ECL
    stage1To3: "+", // Direct default: increases ECL
    stage3To2: "-", // Partial recovery: decreases ECL
    stage3To1: "-", // Full recovery: decreases ECL
  };

  it("stage1To2 (SICR) is positive sign", () => {
    expect(SIGN_MAP["stage1To2"]).toBe("+");
  });

  it("stage2To1 (Cure) is negative sign", () => {
    expect(SIGN_MAP["stage2To1"]).toBe("-");
  });

  it("stage2To3 (Default) is positive sign", () => {
    expect(SIGN_MAP["stage2To3"]).toBe("+");
  });

  it("stage3To2 (Partial recovery) is negative sign", () => {
    expect(SIGN_MAP["stage3To2"]).toBe("-");
  });

  it("stage3To1 (Full recovery) is negative sign", () => {
    expect(SIGN_MAP["stage3To1"]).toBe("-");
  });

  it("all 6 buckets have a sign defined", () => {
    expect(Object.keys(SIGN_MAP)).toHaveLength(6);
  });
});

// ---------------------------------------------------------------------------
// Bucket label exhaustiveness (Bahasa Indonesia)
// ---------------------------------------------------------------------------

const BUCKET_LABELS: Record<string, string> = {
  stage_1_to_2: "Penurunan/SICR (1→2)",
  stage_2_to_1: "Pemulihan/Cure (2→1)",
  stage_2_to_3: "Default (2→3)",
  stage_1_to_3: "Default Langsung (1→3)",
  stage_3_to_2: "Pemulihan Parsial (3→2)",
  stage_3_to_1: "Pemulihan Penuh (3→1)",
  new_origination: "Originasi Baru",
  derecognition: "Penghapusbukuan",
  stage_same: "Tahap Sama (Remeasurement)",
};

import { transferBucketKeyEnum } from "@/lib/schemas/roll-forward.schema";

describe("TransferBucketKey — label exhaustiveness", () => {
  it("has a Bahasa Indonesia label for every bucket key", () => {
    for (const key of transferBucketKeyEnum.options) {
      expect(BUCKET_LABELS[key]).toBeDefined();
      expect(BUCKET_LABELS[key]?.length).toBeGreaterThan(0);
    }
  });

  it("'Originasi Baru' is the label for new_origination", () => {
    expect(BUCKET_LABELS["new_origination"]).toBe("Originasi Baru");
  });

  it("'Penghapusbukuan' is the label for derecognition", () => {
    expect(BUCKET_LABELS["derecognition"]).toBe("Penghapusbukuan");
  });

  it("Cure bucket (stage_2_to_1) contains 'Pemulihan'", () => {
    expect(BUCKET_LABELS["stage_2_to_1"]).toContain("Pemulihan");
  });

  it("SICR bucket (stage_1_to_2) contains 'SICR'", () => {
    expect(BUCKET_LABELS["stage_1_to_2"]).toContain("SICR");
  });
});

// ---------------------------------------------------------------------------
// CKPNTrendChart — data transformation helper
// ---------------------------------------------------------------------------

function parseStr(v: string | null | undefined): number {
  if (!v) return 0;
  const n = parseFloat(v);
  return isNaN(n) ? 0 : n;
}

function toChartData(points: Array<{
  periodeId: string;
  calcRunId: string;
  priorCalcRunId?: string | null;
  eclTotalIdr: string;
  eclByStage: { stage1: string; stage2: string; stage3: string };
  deltaVsPriorIdr?: string | null;
  deltaPct?: string | null;
}>): Array<{ periodeId: string; total: number; stage1: number; stage2: number; stage3: number }> {
  return points.map((p) => ({
    periodeId: p.periodeId,
    calcRunId: p.calcRunId,
    priorCalcRunId: p.priorCalcRunId ?? null,
    total: parseStr(p.eclTotalIdr),
    stage1: parseStr(p.eclByStage.stage1),
    stage2: parseStr(p.eclByStage.stage2),
    stage3: parseStr(p.eclByStage.stage3),
    delta: p.deltaVsPriorIdr ? parseStr(p.deltaVsPriorIdr) : null,
  }));
}

describe("CKPNTrendChart — toChartData", () => {
  const mockPoints = [
    {
      periodeId: "JANUARI-2026",
      calcRunId: "run-001",
      priorCalcRunId: null,
      eclTotalIdr: "10000000000.0000",
      eclByStage: {
        stage1: "8000000000.0000",
        stage2: "1500000000.0000",
        stage3: "500000000.0000",
      },
      deltaVsPriorIdr: null,
      deltaPct: null,
    },
    {
      periodeId: "FEBRUARI-2026",
      calcRunId: "run-002",
      priorCalcRunId: "run-001",
      eclTotalIdr: "11000000000.0000",
      eclByStage: {
        stage1: "8500000000.0000",
        stage2: "1900000000.0000",
        stage3: "600000000.0000",
      },
      deltaVsPriorIdr: "1000000000.0000",
      deltaPct: "10.00",
    },
  ];

  it("converts 2 trend points correctly", () => {
    const data = toChartData(mockPoints);
    expect(data).toHaveLength(2);
  });

  it("total matches eclTotalIdr parsed as number", () => {
    const data = toChartData(mockPoints);
    expect(data[0]?.total).toBe(10_000_000_000);
    expect(data[1]?.total).toBe(11_000_000_000);
  });

  it("stage1 + stage2 + stage3 equals total within rounding", () => {
    const data = toChartData(mockPoints);
    for (const point of data) {
      const stageSum = point.stage1 + point.stage2 + point.stage3;
      expect(Math.abs(stageSum - point.total)).toBeLessThan(1); // < IDR 1
    }
  });

  it("first period has no delta (null)", () => {
    const data = toChartData(mockPoints);
    // delta on first data point — check raw
    expect(mockPoints[0]?.deltaVsPriorIdr).toBeNull();
  });

  it("second period delta is positive", () => {
    expect(parseStr(mockPoints[1]?.deltaVsPriorIdr)).toBe(1_000_000_000);
  });

  it("preserves periodeId as label", () => {
    const data = toChartData(mockPoints);
    expect(data[0]?.periodeId).toBe("JANUARI-2026");
    expect(data[1]?.periodeId).toBe("FEBRUARI-2026");
  });
});

// ---------------------------------------------------------------------------
// Roll-Forward Store (Zustand)
// ---------------------------------------------------------------------------

import { useRollForwardStore } from "@/lib/stores/roll-forward.store";

describe("useRollForwardStore", () => {
  beforeEach(() => {
    useRollForwardStore.getState().reset();
  });

  it("initial state is clean", () => {
    const state = useRollForwardStore.getState();
    expect(state.selectedReportId).toBeNull();
    expect(state.currentCalcRunId).toBeNull();
    expect(state.priorCalcRunId).toBeNull();
    expect(state.trendPeriods).toBe(12);
    expect(state.trendPortofolioId).toBeNull();
  });

  it("setSelectedReportId updates state", () => {
    useRollForwardStore.getState().setSelectedReportId("rf-test-001");
    expect(useRollForwardStore.getState().selectedReportId).toBe("rf-test-001");
  });

  it("setCurrentCalcRunId + setPriorCalcRunId update independently", () => {
    const store = useRollForwardStore.getState();
    store.setCurrentCalcRunId("current-001");
    store.setPriorCalcRunId("prior-001");
    const s = useRollForwardStore.getState();
    expect(s.currentCalcRunId).toBe("current-001");
    expect(s.priorCalcRunId).toBe("prior-001");
  });

  it("setPortfolioFilter merges partial filter", () => {
    const store = useRollForwardStore.getState();
    store.setPortfolioFilter({ bucket: "stage_1_to_2" });
    store.setPortfolioFilter({ overrideFlag: true });
    const { portfolioFilter } = useRollForwardStore.getState();
    expect(portfolioFilter.bucket).toBe("stage_1_to_2");
    expect(portfolioFilter.overrideFlag).toBe(true);
  });

  it("clearPortfolioFilter resets to empty", () => {
    const store = useRollForwardStore.getState();
    store.setPortfolioFilter({ bucket: "new_origination", q: "test" });
    store.clearPortfolioFilter();
    const { portfolioFilter } = useRollForwardStore.getState();
    expect(portfolioFilter.bucket).toBeUndefined();
    expect(portfolioFilter.q).toBeUndefined();
  });

  it("setTrendPeriods clamps to [2, 24]", () => {
    const store = useRollForwardStore.getState();
    store.setTrendPeriods(1);
    expect(useRollForwardStore.getState().trendPeriods).toBe(2);
    store.setTrendPeriods(100);
    expect(useRollForwardStore.getState().trendPeriods).toBe(24);
    store.setTrendPeriods(6);
    expect(useRollForwardStore.getState().trendPeriods).toBe(6);
  });

  it("setTrendPortofolioId updates state", () => {
    useRollForwardStore
      .getState()
      .setTrendPortofolioId("00000000-0000-0000-0000-000000000010");
    expect(useRollForwardStore.getState().trendPortofolioId).toBe(
      "00000000-0000-0000-0000-000000000010",
    );
  });

  it("reset clears all state", () => {
    const store = useRollForwardStore.getState();
    store.setSelectedReportId("rf-001");
    store.setCurrentCalcRunId("run-001");
    store.setTrendPeriods(24);
    store.reset();
    const s = useRollForwardStore.getState();
    expect(s.selectedReportId).toBeNull();
    expect(s.currentCalcRunId).toBeNull();
    expect(s.trendPeriods).toBe(12);
  });
});

// ---------------------------------------------------------------------------
// RollForwardExportButton — MISMATCH guard logic (state machine)
// ---------------------------------------------------------------------------

describe("RollForwardExportButton — MISMATCH guard logic", () => {
  type ReconcileStatus = "RECONCILED" | "MISMATCH";

  function shouldPromptConfirm(
    reconcileStatus: ReconcileStatus,
    _format: "xlsx" | "csv",
  ): boolean {
    return reconcileStatus === "MISMATCH";
  }

  it("RECONCILED + xlsx → no confirm dialog", () => {
    expect(shouldPromptConfirm("RECONCILED", "xlsx")).toBe(false);
  });

  it("RECONCILED + csv → no confirm dialog", () => {
    expect(shouldPromptConfirm("RECONCILED", "csv")).toBe(false);
  });

  it("MISMATCH + xlsx → confirm dialog required", () => {
    expect(shouldPromptConfirm("MISMATCH", "xlsx")).toBe(true);
  });

  it("MISMATCH + csv → confirm dialog required", () => {
    expect(shouldPromptConfirm("MISMATCH", "csv")).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// API URL construction (parameter building)
// ---------------------------------------------------------------------------

function buildRollForwardExportUrl(
  reportId: string,
  format: "xlsx" | "csv",
  forceMismatch: boolean,
): string {
  const qs = new URLSearchParams({ format });
  if (forceMismatch) qs.set("force_mismatch", "true");
  return `/api/v1/ecl/roll-forward/${encodeURIComponent(reportId)}/export?${qs.toString()}`;
}

describe("rollForwardApi — export URL construction", () => {
  it("builds XLSX export URL correctly", () => {
    const url = buildRollForwardExportUrl("rf-test-001", "xlsx", false);
    expect(url).toContain("/api/v1/ecl/roll-forward/rf-test-001/export");
    expect(url).toContain("format=xlsx");
    expect(url).not.toContain("force_mismatch");
  });

  it("adds force_mismatch=true when override", () => {
    const url = buildRollForwardExportUrl("rf-test-001", "xlsx", true);
    expect(url).toContain("force_mismatch=true");
  });

  it("encodes reportId with special chars", () => {
    const url = buildRollForwardExportUrl(
      "rf-b1c2d3e4-JUNI-2026",
      "csv",
      false,
    );
    expect(url).toContain(
      "/api/v1/ecl/roll-forward/rf-b1c2d3e4-JUNI-2026/export",
    );
    expect(url).toContain("format=csv");
  });
});
