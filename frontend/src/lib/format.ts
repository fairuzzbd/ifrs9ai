/**
 * Shared formatting utilities for BLIPS IFRS9 frontend.
 * IDR amounts, dates, percentages.
 */

// ---------------------------------------------------------------------------
// Currency
// ---------------------------------------------------------------------------

const idrFullFmt = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

const idrPreciseFmt = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 2,
  maximumFractionDigits: 4,
});

/**
 * Format a number as IDR currency.
 * @param value raw number (IDR)
 * @param precise if true, show 4 decimal places (detail views)
 */
export function formatIDR(value: number, precise = false): string {
  return precise ? idrPreciseFmt.format(value) : idrFullFmt.format(value);
}

/**
 * Format IDR with abbreviation for dashboard KPI cards:
 * 1_000_000_000 → "Rp 1 M" (miliar), 1_000_000 → "Rp 1 Jt"
 */
export function formatIDRAbbrev(value: number): string {
  if (Math.abs(value) >= 1_000_000_000_000) {
    return `Rp ${(value / 1_000_000_000_000).toFixed(1).replace(".", ",")} T`;
  }
  if (Math.abs(value) >= 1_000_000_000) {
    return `Rp ${(value / 1_000_000_000).toFixed(1).replace(".", ",")} M`;
  }
  if (Math.abs(value) >= 1_000_000) {
    return `Rp ${(value / 1_000_000).toFixed(1).replace(".", ",")} Jt`;
  }
  return formatIDR(value);
}

/**
 * Parse a decimal string (shopspring/decimal JSON) to number.
 * Returns 0 if input is null/undefined/NaN.
 */
export function parseDecimal(value: string | number | null | undefined): number {
  if (value == null) return 0;
  const n = typeof value === "number" ? value : parseFloat(value);
  return isNaN(n) ? 0 : n;
}

// ---------------------------------------------------------------------------
// Date / time
// ---------------------------------------------------------------------------

const jakartaFmt = new Intl.DateTimeFormat("id-ID", {
  timeZone: "Asia/Jakarta",
  dateStyle: "short",
  timeStyle: "short",
});

const jakartaDateFmt = new Intl.DateTimeFormat("id-ID", {
  timeZone: "Asia/Jakarta",
  dateStyle: "short",
});

const jakartaTimeFmt = new Intl.DateTimeFormat("id-ID", {
  timeZone: "Asia/Jakarta",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return jakartaFmt.format(new Date(iso));
  } catch {
    return iso;
  }
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return jakartaDateFmt.format(new Date(iso));
  } catch {
    return iso;
  }
}

export function formatTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return jakartaTimeFmt.format(new Date(iso));
  } catch {
    return iso;
  }
}

/**
 * Returns "HH:MM:SS" timestamp string in WIB for display.
 */
export function formatLastUpdated(date: Date): string {
  return jakartaFmt.format(date);
}

// ---------------------------------------------------------------------------
// Percentage
// ---------------------------------------------------------------------------

export function formatPct(value: number, decimals = 2): string {
  return `${value.toFixed(decimals)}%`;
}

// ---------------------------------------------------------------------------
// Duration
// ---------------------------------------------------------------------------

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null || seconds < 0) return "—";
  if (seconds < 60) return `${Math.round(seconds)}d`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return `${m}m ${s}d`;
}
