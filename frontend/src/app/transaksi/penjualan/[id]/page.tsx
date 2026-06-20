"use client";

import * as React from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PenjualanStatusBadge } from "@/components/blips/penjualan/PenjualanStatusBadge";
import { PenjualanJenisBadge } from "@/components/blips/penjualan/PenjualanJenisBadge";
import { PenjualanOCIRecycleBadge } from "@/components/blips/penjualan/PenjualanOCIRecycleBadge";
import { PenjualanBMRiskBadge } from "@/components/blips/penjualan/PenjualanBMRiskBadge";
import { PenjualanRoutingBadge } from "@/components/blips/penjualan/PenjualanRoutingBadge";
import { PenjualanPreviewPanel } from "@/components/blips/penjualan/PenjualanPreviewPanel";
import { PenjualanApproveDialog } from "@/components/blips/penjualan/PenjualanApproveDialog";
import { PenjualanRejectDialog } from "@/components/blips/penjualan/PenjualanRejectDialog";
import { penjualanDetailApi, penjualanQueryKeys } from "@/lib/api/penjualan.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { KlasifikasiPsak71, JenisDisposal } from "@/lib/schemas/penjualan.schema";

// ---------------------------------------------------------------------------
// IDR formatter
// ---------------------------------------------------------------------------

const IDR_DETAIL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function fmt(v?: string | null): string {
  if (!v) return "-";
  const n = parseFloat(v);
  return isNaN(n) ? v : IDR_DETAIL.format(n);
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PenjualanDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [approveOpen, setApproveOpen] = React.useState(false);
  const [rejectOpen, setRejectOpen] = React.useState(false);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: penjualanQueryKeys.detail(id),
    queryFn: () => penjualanDetailApi.get(id),
    staleTime: 30_000,
  });

  const penjualan = data?.data;

  const canApproveOrReject =
    perms.canApprove?.("transaksi") &&
    penjualan?.status === "PENDING_APPROVAL";

  if (isLoading) {
    return (
      <div className="container mx-auto py-10 text-center text-muted-foreground" aria-busy="true">
        Memuat data penjualan...
      </div>
    );
  }

  if (isError || !penjualan) {
    return (
      <div className="container mx-auto py-10 text-center">
        <p className="text-red-600">Gagal memuat data penjualan.</p>
        <Button onClick={() => void refetch()} className="mt-4">Coba Lagi</Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/transaksi/penjualan">
            <Button variant="ghost" size="sm" aria-label="Kembali ke daftar penjualan">
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              Kembali
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold">
              Detail Penjualan — {penjualan.instrumenKode}
            </h1>
            <p className="text-xs text-muted-foreground font-mono">{penjualan.id}</p>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={() => void refetch()} aria-label="Muat ulang data">
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>

      {/* Status + badges */}
      <div className="flex flex-wrap gap-2 items-center">
        <PenjualanStatusBadge status={penjualan.status} />
        <PenjualanJenisBadge jenis={penjualan.jenisDisposal} />
        <PenjualanOCIRecycleBadge
          klasifikasi={penjualan.klasifikasiSnapshot as KlasifikasiPsak71}
          ociRecycled={penjualan.ociRecycled}
        />
        {penjualan.bmViolationRisk && penjualan.bmViolationPct && (
          <PenjualanBMRiskBadge
            flag="BM_VIOLATION_RISK"
            pct={penjualan.bmViolationPct}
          />
        )}
        {penjualan.status === "PENDING_BM_REVIEW" && (
          <PenjualanBMRiskBadge flag="BM_VIOLATION_BLOCK" pct={penjualan.bmViolationPct ?? undefined} />
        )}
      </div>

      {/* Routing */}
      {penjualan.jurnalEventCode && (
        <div>
          <span className="text-xs text-muted-foreground mr-2">Event Code:</span>
          <PenjualanRoutingBadge
            klasifikasi={penjualan.klasifikasiSnapshot as KlasifikasiPsak71}
            jenisDisposal={penjualan.jenisDisposal as JenisDisposal}
          />
        </div>
      )}

      {/* Core data */}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm border rounded-md p-4 bg-slate-50">
        <dt className="text-muted-foreground">Klasifikasi PSAK 71</dt>
        <dd className="font-mono font-medium">{penjualan.klasifikasiSnapshot}</dd>

        <dt className="text-muted-foreground">Qty Terjual</dt>
        <dd className="font-mono">{penjualan.qtyTerjual}</dd>

        <dt className="text-muted-foreground">Qty Holding (sebelum)</dt>
        <dd className="font-mono">{penjualan.qtyHoldingPre}</dd>

        <dt className="text-muted-foreground">Qty Holding (sesudah)</dt>
        <dd className="font-mono">{penjualan.qtyHoldingPost ?? "-"}</dd>

        <dt className="text-muted-foreground">Proceeds IDR</dt>
        <dd className="font-mono font-semibold">{fmt(penjualan.proceedIdr)}</dd>

        <dt className="text-muted-foreground">Cost Basis</dt>
        <dd className="font-mono">{fmt(penjualan.costBasis)}</dd>

        <dt className="text-muted-foreground">Realized G/L</dt>
        <dd className={`font-mono font-semibold ${parseFloat(penjualan.realizedGl ?? "0") >= 0 ? "text-green-700" : "text-red-600"}`}>
          {fmt(penjualan.realizedGl)}
        </dd>

        <dt className="text-muted-foreground">OCI Recycled</dt>
        <dd className="font-mono">{penjualan.ociRecycled ? fmt(penjualan.ociRecycled) : "N/A"}</dd>

        <dt className="text-muted-foreground">Tanggal Eksekusi</dt>
        <dd>{penjualan.tanggalEksekusi}</dd>

        <dt className="text-muted-foreground">Dibuat</dt>
        <dd>{new Date(penjualan.createdAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}</dd>
      </dl>

      {/* OCI no-recycling note */}
      {penjualan.noRecyclingNote && (
        <div className="rounded border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700">
          {penjualan.noRecyclingNote}
        </div>
      )}

      {/* Approve / reject comment */}
      {penjualan.approveComment && (
        <div className="rounded border border-green-200 bg-green-50 px-3 py-2 text-xs text-green-800">
          <strong>Komentar Persetujuan:</strong> {penjualan.approveComment}
        </div>
      )}
      {penjualan.rejectReason && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
          <strong>Alasan Penolakan:</strong> {penjualan.rejectReason}
        </div>
      )}

      {/* Preview panel */}
      {penjualan.preview && (
        <PenjualanPreviewPanel
          preview={penjualan.preview}
          jenisDisposal={penjualan.jenisDisposal as JenisDisposal}
        />
      )}

      {/* Workflow action bar — persona gated + SoD */}
      {canApproveOrReject && (
        <div className="flex gap-3 border-t pt-4">
          <Button
            onClick={() => setApproveOpen(true)}
            aria-label={`Setujui penjualan ${penjualan.instrumenKode}`}
          >
            Setujui
          </Button>
          <Button
            variant="destructive"
            onClick={() => setRejectOpen(true)}
            aria-label={`Tolak penjualan ${penjualan.instrumenKode}`}
          >
            Tolak
          </Button>
        </div>
      )}

      {approveOpen && (
        <PenjualanApproveDialog
          open={approveOpen}
          onOpenChange={setApproveOpen}
          penjualanId={penjualan.id}
          instrumenKode={penjualan.instrumenKode}
          makerId={penjualan.makerId}
          onSuccess={() => {
            void queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.detail(id) });
          }}
        />
      )}

      {rejectOpen && (
        <PenjualanRejectDialog
          open={rejectOpen}
          onOpenChange={setRejectOpen}
          penjualanId={penjualan.id}
          instrumenKode={penjualan.instrumenKode}
          onSuccess={() => {
            void queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.detail(id) });
          }}
        />
      )}
    </div>
  );
}
