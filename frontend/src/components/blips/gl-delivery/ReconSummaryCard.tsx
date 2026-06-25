"use client";

import * as React from "react";
import { CheckCircle2, XCircle, Loader2, Clock, ChevronDown, ChevronRight, type LucideIcon } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type {
  GlReconciliationReport,
  GlReconMismatchLine,
} from "@/lib/schemas/gl-delivery.schema";
import { RECON_STATUS_LABELS } from "@/lib/schemas/gl-delivery.schema";
import { ReconMismatchTypeBadge } from "./ReconMismatchTypeBadge";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
});

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

// ---------------------------------------------------------------------------
// Status config (4 states A/B/C/D from design spec §4)
// ---------------------------------------------------------------------------

interface StatusConfig {
  icon: LucideIcon;
  colorClass: string;
  badgeVariant: "default" | "secondary" | "destructive" | "outline";
}

const STATUS_CONFIG: Record<string, StatusConfig> = {
  MATCH: {
    icon: CheckCircle2,
    colorClass: "text-green-600",
    badgeVariant: "default",
  },
  MISMATCH: {
    icon: XCircle,
    colorClass: "text-red-600",
    badgeVariant: "destructive",
  },
  RUNNING: {
    icon: Loader2,
    colorClass: "text-blue-600",
    badgeVariant: "secondary",
  },
  PENDING: {
    icon: Clock,
    colorClass: "text-muted-foreground",
    badgeVariant: "outline",
  },
};

// ---------------------------------------------------------------------------
// Mismatch detail table
// ---------------------------------------------------------------------------

interface MismatchTableProps {
  items: GlReconMismatchLine[];
}

function MismatchTable({ items }: MismatchTableProps) {
  const [expanded, setExpanded] = React.useState(false);
  const visible = expanded ? items : items.slice(0, 5);
  const hasMore = items.length > 5;

  if (items.length === 0) return null;

  return (
    <div className="mt-3 space-y-2">
      <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
        Mismatch Detail ({items.length} akun)
      </p>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-xs">
          <thead>
            <tr className="bg-muted/50 border-b">
              <th className="px-2 py-1.5 text-left font-medium">Tipe</th>
              <th className="px-2 py-1.5 text-left font-medium">Kode Akun</th>
              <th className="px-2 py-1.5 text-left font-medium">Nama Akun</th>
              <th className="px-2 py-1.5 text-right font-medium">Nilai BLIPS</th>
              <th className="px-2 py-1.5 text-right font-medium">Nilai GL Host</th>
              <th className="px-2 py-1.5 text-right font-medium">Selisih</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((item, idx) => (
              <tr key={idx} className="border-b last:border-0 hover:bg-muted/30">
                <td className="px-2 py-1.5">
                  <ReconMismatchTypeBadge type={item.mismatchType} />
                </td>
                <td className="px-2 py-1.5 font-mono">{item.kodeAkun}</td>
                <td className="px-2 py-1.5 max-w-[200px] truncate">{item.namaAkun ?? "—"}</td>
                <td className="px-2 py-1.5 text-right font-mono">
                  {IDR.format(item.blipsAmountIdr)}
                </td>
                <td className="px-2 py-1.5 text-right font-mono">
                  {IDR.format(item.glHostAmountIdr)}
                </td>
                <td className={cn(
                  "px-2 py-1.5 text-right font-mono",
                  item.deltaIdr !== 0 ? "text-red-600" : "",
                )}>
                  {IDR.format(item.deltaIdr)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {hasMore && (
        <button
          type="button"
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          {expanded ? (
            <><ChevronDown className="h-3 w-3" aria-hidden="true" /> Sembunyikan</>
          ) : (
            <><ChevronRight className="h-3 w-3" aria-hidden="true" /> Tampilkan semua {items.length} akun</>
          )}
        </button>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ReconSummaryCardProps {
  report: GlReconciliationReport;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S4-AC1, S4-AC2, S4-AC3, S4-AC4)
// ---------------------------------------------------------------------------

export function ReconSummaryCard({ report, className }: ReconSummaryCardProps) {
  const config =
    STATUS_CONFIG[report.status] ?? STATUS_CONFIG["PENDING"];
  const Icon = config.icon;
  const label = RECON_STATUS_LABELS[report.status] ?? report.status;
  const isRunning = report.status === "RUNNING";

  return (
    <Card className={cn("overflow-hidden", className)}>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm">Rekonsiliasi GL</CardTitle>
          <div className="flex items-center gap-2">
            <Icon
              className={cn(
                "h-4 w-4",
                config.colorClass,
                isRunning && "animate-spin motion-reduce:animate-none",
              )}
              aria-hidden="true"
            />
            <Badge variant={config.badgeVariant}>{label}</Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-1 space-y-3">
        {/* Meta */}
        <dl className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm">
          <dt className="text-muted-foreground">Tanggal Rekon</dt>
          <dd className="font-mono">{report.tanggalRekonsiliasi}</dd>
          <dt className="text-muted-foreground">Dijalankan</dt>
          <dd>{fmtDate(report.generatedAt)}</dd>
          <dt className="text-muted-foreground">Akun Dicek</dt>
          <dd>{report.totalAkunChecked}</dd>
          <dt className="text-muted-foreground">Mismatch</dt>
          <dd className={report.totalMismatchCount > 0 ? "text-red-600 font-medium" : ""}>
            {report.totalMismatchCount}
          </dd>
          <dt className="text-muted-foreground">Total BLIPS (IDR)</dt>
          <dd className="font-mono text-xs">{IDR.format(report.blipsTotalIdr)}</dd>
          <dt className="text-muted-foreground">Total GL Host (IDR)</dt>
          <dd className="font-mono text-xs">{IDR.format(report.glHostTotalIdr)}</dd>
          <dt className="text-muted-foreground">Selisih (IDR)</dt>
          <dd className={cn("font-mono text-xs", report.deltaIdr !== 0 ? "text-red-600 font-medium" : "")}>
            {IDR.format(report.deltaIdr)}
          </dd>
        </dl>

        {/* STATE A — COMPLETED (no mismatch) */}
        {report.status === "COMPLETED" && (
          <div className="flex items-center gap-2 rounded border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-800">
            <CheckCircle2 className="h-4 w-4 shrink-0" aria-hidden="true" />
            <p>
              Semua akun BLIPS cocok dengan GL Host. Tidak ada selisih.
            </p>
          </div>
        )}

        {/* STATE B — COMPLETED_WITH_MISMATCH */}
        {report.status === "COMPLETED_WITH_MISMATCH" && (
          <>
            <div className="flex items-center gap-2 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
              <XCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
              <p>
                Ditemukan {report.totalMismatchCount} akun dengan selisih antara BLIPS dan GL Host.
                Lakukan investigasi sebelum hard-close.
              </p>
            </div>
            {report.mismatchLines && <MismatchTable items={report.mismatchLines} />}
          </>
        )}

        {/* STATE C — RUNNING */}
        {report.status === "RUNNING" && (
          <div className="flex items-center gap-2 text-sm text-blue-700">
            <Loader2
              className="h-4 w-4 animate-spin motion-reduce:animate-none shrink-0"
              aria-hidden="true"
            />
            <p>Proses rekonsiliasi sedang berjalan...</p>
          </div>
        )}

        {/* STATE D — PENDING */}
        {report.status === "PENDING" && (
          <p className="text-sm text-muted-foreground">
            Rekonsiliasi belum pernah dijalankan untuk tanggal ini.
            Klik &ldquo;Jalankan Rekonsiliasi&rdquo; untuk memulai.
          </p>
        )}

        {/* STATE E — FAILED */}
        {report.status === "FAILED" && (
          <div className="flex items-center gap-2 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
            <XCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
            <p>
              Rekonsiliasi gagal. GL Host tidak dapat dijangkau atau terjadi error saat query.
              Coba jalankan ulang atau hubungi IT Admin.
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
