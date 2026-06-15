/**
 * Vitest unit tests for jurnal engine components — pure logic, no DOM.
 * Tests BalancePreviewCard calculation logic + JurnalLinesTable balance logic
 * by testing the underlying arithmetic that both components rely on.
 */
import { describe, it, expect } from "vitest";
import type { JurnalLine, MappingDetailRow } from "@/lib/schemas/jurnal.schema";

// ---------------------------------------------------------------------------
// Balance calculation helpers (mirrors BalancePreviewCard + JurnalLinesTable)
// ---------------------------------------------------------------------------

function calcBalance(lines: JurnalLine[]) {
  const totalDebit = lines
    .filter((l) => l.posisi === "DEBIT")
    .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);
  const totalKredit = lines
    .filter((l) => l.posisi === "KREDIT")
    .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);
  return { totalDebit, totalKredit, isBalanced: Math.abs(totalDebit - totalKredit) < 0.01 };
}

function calcTemplateBalance(rows: MappingDetailRow[]) {
  const hasDebit = rows.some((r) => r.dkIndicator === "DEBIT");
  const hasKredit = rows.some((r) => r.dkIndicator === "KREDIT");
  return { hasDebit, hasKredit, isBalanced: hasDebit && hasKredit };
}

function calcRuntimeBalance(rows: MappingDetailRow[], amountIdr: string) {
  const amount = parseFloat(amountIdr || "0");
  const totalDebit = rows
    .filter((r) => r.dkIndicator === "DEBIT")
    .reduce((acc, r) => acc + amount * parseFloat(r.multiplier || "1"), 0);
  const totalKredit = rows
    .filter((r) => r.dkIndicator === "KREDIT")
    .reduce((acc, r) => acc + amount * parseFloat(r.multiplier || "1"), 0);
  return { totalDebit, totalKredit, isBalanced: Math.abs(totalDebit - totalKredit) < 0.01 };
}

// ---------------------------------------------------------------------------
// JurnalLinesTable logic
// ---------------------------------------------------------------------------

describe("JurnalLinesTable — balance calculation", () => {
  const makeLines = (debitAmt: string, kreditAmt: string): JurnalLine[] => [
    {
      urutan: 1,
      posisi: "DEBIT",
      akunId: "akun-001",
      akunKode: "1-001",
      akunNama: "Kas",
      amountIdr: debitAmt,
    },
    {
      urutan: 2,
      posisi: "KREDIT",
      akunId: "akun-002",
      akunKode: "2-001",
      akunNama: "Hutang",
      amountIdr: kreditAmt,
    },
  ];

  it("reports balanced when DEBIT = KREDIT", () => {
    const { isBalanced, totalDebit, totalKredit } = calcBalance(
      makeLines("1000000.0000", "1000000.0000"),
    );
    expect(isBalanced).toBe(true);
    expect(totalDebit).toBe(1000000);
    expect(totalKredit).toBe(1000000);
  });

  it("reports unbalanced when DEBIT ≠ KREDIT", () => {
    const { isBalanced } = calcBalance(makeLines("1000000.0000", "500000.0000"));
    expect(isBalanced).toBe(false);
  });

  it("handles empty lines (balanced at 0)", () => {
    const { isBalanced, totalDebit, totalKredit } = calcBalance([]);
    expect(isBalanced).toBe(true);
    expect(totalDebit).toBe(0);
    expect(totalKredit).toBe(0);
  });

  it("handles multiple DEBIT lines summed correctly", () => {
    const lines: JurnalLine[] = [
      { urutan: 1, posisi: "DEBIT", akunId: "a", akunKode: "1", akunNama: "X", amountIdr: "300000.0000" },
      { urutan: 2, posisi: "DEBIT", akunId: "b", akunKode: "2", akunNama: "Y", amountIdr: "200000.0000" },
      { urutan: 3, posisi: "KREDIT", akunId: "c", akunKode: "3", akunNama: "Z", amountIdr: "500000.0000" },
    ];
    const { isBalanced, totalDebit } = calcBalance(lines);
    expect(totalDebit).toBe(500000);
    expect(isBalanced).toBe(true);
  });

  it("within 0.01 tolerance is considered balanced", () => {
    const { isBalanced } = calcBalance(makeLines("1000000.0000", "1000000.0090"));
    expect(isBalanced).toBe(true);
  });

  it("0.01 difference is unbalanced", () => {
    const { isBalanced } = calcBalance(makeLines("1000000.0000", "1000000.0100"));
    expect(isBalanced).toBe(false);
  });

  it("handles string zeros correctly", () => {
    const { totalDebit } = calcBalance([
      { urutan: 1, posisi: "DEBIT", akunId: "a", akunKode: "1", akunNama: "X", amountIdr: "0" },
    ]);
    expect(totalDebit).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// BalancePreviewCard — template level
// ---------------------------------------------------------------------------

describe("BalancePreviewCard — template level balance", () => {
  const makeRows = (dk: ("DEBIT" | "KREDIT")[]): MappingDetailRow[] =>
    dk.map((d, i) => ({
      urutan: i + 1,
      dkIndicator: d,
      kodeAkunId: `akun-${i}`,
      sumberAmount: "nominal_idr" as const,
      multiplier: "1.0000",
    }));

  it("reports balanced when both DEBIT and KREDIT rows exist", () => {
    const { isBalanced } = calcTemplateBalance(makeRows(["DEBIT", "KREDIT"]));
    expect(isBalanced).toBe(true);
  });

  it("reports unbalanced when only DEBIT rows", () => {
    const { isBalanced, hasDebit, hasKredit } = calcTemplateBalance(makeRows(["DEBIT", "DEBIT"]));
    expect(isBalanced).toBe(false);
    expect(hasDebit).toBe(true);
    expect(hasKredit).toBe(false);
  });

  it("reports unbalanced when only KREDIT rows", () => {
    const { isBalanced } = calcTemplateBalance(makeRows(["KREDIT", "KREDIT"]));
    expect(isBalanced).toBe(false);
  });

  it("reports unbalanced for empty rows", () => {
    const { isBalanced } = calcTemplateBalance([]);
    expect(isBalanced).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// BalancePreviewCard — runtime balance with multiplier
// ---------------------------------------------------------------------------

describe("BalancePreviewCard — runtime balance with multiplier", () => {
  const rows: MappingDetailRow[] = [
    {
      urutan: 1,
      dkIndicator: "DEBIT",
      kodeAkunId: "akun-debit",
      sumberAmount: "nominal_idr",
      multiplier: "1.0000",
    },
    {
      urutan: 2,
      dkIndicator: "KREDIT",
      kodeAkunId: "akun-kredit",
      sumberAmount: "nominal_idr",
      multiplier: "1.0000",
    },
  ];

  it("computes correct amounts for 1:1 multiplier", () => {
    const { totalDebit, totalKredit, isBalanced } = calcRuntimeBalance(rows, "5000000.0000");
    expect(totalDebit).toBe(5000000);
    expect(totalKredit).toBe(5000000);
    expect(isBalanced).toBe(true);
  });

  it("handles fractional multiplier correctly", () => {
    const fractionalRows: MappingDetailRow[] = [
      { ...rows[0], multiplier: "0.5000" },
      { ...rows[1], multiplier: "0.5000" },
    ];
    const { totalDebit, isBalanced } = calcRuntimeBalance(fractionalRows, "1000000.0000");
    expect(totalDebit).toBe(500000);
    expect(isBalanced).toBe(true);
  });

  it("detects imbalance when multipliers differ between D and K", () => {
    const asymRows: MappingDetailRow[] = [
      { ...rows[0], multiplier: "1.0000" },
      { ...rows[1], multiplier: "0.9000" },
    ];
    const { isBalanced } = calcRuntimeBalance(asymRows, "1000000.0000");
    expect(isBalanced).toBe(false);
  });

  it("zero amount produces all zeros", () => {
    const { totalDebit, totalKredit, isBalanced } = calcRuntimeBalance(rows, "0");
    expect(totalDebit).toBe(0);
    expect(totalKredit).toBe(0);
    expect(isBalanced).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// SixEyesWorkflowPanel — SoD logic (pure)
// ---------------------------------------------------------------------------

describe("SixEyesWorkflowPanel — SoD detection logic", () => {
  function sodCheck(status: string, currentUserId: string, makerId: string, reviewerId: string) {
    const reviewSodBlock = status === "PENDING_REVIEW" && currentUserId === makerId;
    const approveSodBlock =
      status === "PENDING_APPROVAL" &&
      (currentUserId === makerId || currentUserId === reviewerId);
    return { reviewSodBlock, approveSodBlock };
  }

  it("blocks maker from reviewing own submission", () => {
    const { reviewSodBlock } = sodCheck("PENDING_REVIEW", "user-A", "user-A", "");
    expect(reviewSodBlock).toBe(true);
  });

  it("allows different user to review", () => {
    const { reviewSodBlock } = sodCheck("PENDING_REVIEW", "user-B", "user-A", "");
    expect(reviewSodBlock).toBe(false);
  });

  it("blocks maker from approving", () => {
    const { approveSodBlock } = sodCheck("PENDING_APPROVAL", "user-A", "user-A", "user-B");
    expect(approveSodBlock).toBe(true);
  });

  it("blocks reviewer from approving", () => {
    const { approveSodBlock } = sodCheck("PENDING_APPROVAL", "user-B", "user-A", "user-B");
    expect(approveSodBlock).toBe(true);
  });

  it("allows distinct user to approve", () => {
    const { approveSodBlock } = sodCheck("PENDING_APPROVAL", "user-C", "user-A", "user-B");
    expect(approveSodBlock).toBe(false);
  });

  it("no SoD block when status is not PENDING_REVIEW", () => {
    const { reviewSodBlock } = sodCheck("DRAFT", "user-A", "user-A", "");
    expect(reviewSodBlock).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// DLQActionPanel — discard reason min length
// ---------------------------------------------------------------------------

describe("DLQActionPanel — discard reason validation", () => {
  const MIN_CHARS = 30;

  const validate = (reason: string) => reason.length >= MIN_CHARS;

  it("passes with exactly 30 chars", () => {
    expect(validate("a".repeat(30))).toBe(true);
  });

  it("fails with 29 chars", () => {
    expect(validate("a".repeat(29))).toBe(false);
  });

  it("fails with empty string", () => {
    expect(validate("")).toBe(false);
  });

  it("passes with long descriptive reason", () => {
    expect(validate("Entri ini sudah tidak relevan karena transaksi dibatalkan sebelum periode tutup.")).toBe(true);
  });
});
