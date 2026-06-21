"use client";

import * as React from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";

import { Button } from "@/components/ui/button";
import { AkrualStatusBadge } from "@/components/blips/akrual/AkrualStatusBadge";
import { AkrualStageBadge } from "@/components/blips/akrual/AkrualStageBadge";
import { AkrualJenisBadge } from "@/components/blips/akrual/AkrualJenisBadge";
import { StaleStagingBadge } from "@/components/blips/akrual/StaleStagingBadge";
import { AkrualOverrideStaleDialog } from "@/components/blips/akrual/AkrualOverrideStaleDialog";

import { akrualDetailApi, akrualQueryKeys } from "@/lib/api/akrual.api";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// IDR formatters
// ---------------------------------------------------------------------------

const IDR_FULL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function formatIDR(val: string | null | undefined): string {
  if (!val) return "—";
  const n = parseFloat(val);
  return isNaN(n) ? "—" : IDR_FULL.format(n);
}

function formatEIR(val: string | null | undefined): string {
  if (!val) return "—";
  const n = parseFloat(val);
  return isNaN(n) ? "—" : `${(n * 100).toFixed(6)}%`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AkrualDetailPage() {
  const { id } = useParams<{ id: string }>();
  const perms = usePermissions();
  const queryClient = useQueryClient();
  const [overrideOpen, setOverrideOpen] = React.useState(false);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: akrualQueryKeys.detail(id),
    queryFn: () => akrualDetailApi.get(id),
    enabled: !!id,
  });

  const akrual = data?.data;

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 text-center text-muted-foreground" aria-live="polite">
        Memuat detail akrual...
      </div>
    );
  }

  if (isError || !akrual) {
    return (
      <div className="container mx-auto py-6" role="alert">
        <div className="rounded-lg border border-red-200 bg-red-50 p-8 text-center">
          <p className="text-red-700 mb-2">Gagal memuat detail akrual.</p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Coba Lagi
          </Button>
        </div>
      </div>
    );
  }

  const isStale = akrual.staleStagingFlag;
  const canOverride = perms.can("akrual.override_stale") && akrual.status === "PENDING_STALE_REVIEW";

  const DetailRow = ({ label, value }: { label: string; value: React.ReactNode }) => (
    <div className="flex flex-col sm:flex-row sm:items-start gap-1">
      <span className="text-sm text-muted-foreground w-48 flex-shrink-0">{label}</span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link href="/transaksi/akrual" aria-label="Kembali ke daftar akrual">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
            Kembali
          </Button>
        </Link>
        <div>
          <h1 className="text-xl font-bold">Detail Akrual — {akrual.instrumenKode}</h1>
          <p className="text-xs text-muted-foreground font-mono">{akrual.id}</p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <AkrualStatusBadge status={akrual.status} />
          {canOverride && (
            <Button
              size="sm"
              onClick={() => setOverrideOpen(true)}
              aria-label="Override staging stale"
            >
              Override Staging Stale
            </Button>
          )}
        </div>
      </div>

      {/* Stale alert */}
      {isStale && (
        <div
          className="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 flex items-center gap-3"
          role="alert"
        >
          <StaleStagingBadge />
          <p className="text-sm text-amber-800">
            ECL sealed run terakhir melebihi batas staleness. Akrual ini dalam status{" "}
            <strong>PENDING_STALE_REVIEW</strong> — memerlukan konfirmasi ROLE-AKUN-CTL sebelum
            diposting.
          </p>
        </div>
      )}

      {/* Detail card */}
      <div className="rounded-lg border p-6 space-y-4">
        <h2 className="text-base font-semibold border-b pb-2">Informasi Akrual</h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8 gap-y-3">
          <DetailRow label="Instrumen" value={akrual.instrumenKode} />
          <DetailRow label="Tanggal Akrual" value={akrual.tanggalAkrual} />
          <DetailRow label="Jenis" value={<AkrualJenisBadge jenis={akrual.jenis} size="sm" />} />
          <DetailRow
            label="Stage ECL"
            value={
              <AkrualStageBadge
                stage={akrual.stage as 1 | 2 | 3 | null}
                netCarryingIdr={akrual.carryingBasis === "NET_CARRYING" ? akrual.carryingIdr : null}
                grossCarryingIdr={akrual.carryingBasis === "GROSS" ? akrual.carryingIdr : null}
              />
            }
          />
          <DetailRow
            label="Basis Carrying"
            value={
              <span className={akrual.carryingBasis === "NET_CARRYING" ? "text-red-600 font-semibold" : ""}>
                {akrual.carryingBasis === "NET_CARRYING"
                  ? "Net Carrying (Gross − ECL) per PSAK 71 §5.4.1(b)"
                  : "Gross Carrying Amount"}
              </span>
            }
          />
          <DetailRow label="Carrying (IDR)" value={formatIDR(akrual.carryingIdr)} />
          <DetailRow label="EIR" value={formatEIR(akrual.eirPersen)} />
          <DetailRow label="Bunga Kotor" value={formatIDR(akrual.bungaKotor)} />
          <DetailRow label="PPh" value={formatIDR(akrual.pph)} />
          <DetailRow label="Akrual Bersih" value={<strong>{formatIDR(akrual.bungaBersih)}</strong>} />
          <DetailRow label="Mata Uang" value={akrual.mataUang} />
          <DetailRow label="FX Rate ID" value={akrual.fxRateId ?? "IDR (tidak perlu konversi)"} />
          <DetailRow
            label="ECL Run Digunakan"
            value={akrual.eclRunIdUsed ?? (akrual.staleStagingFlag ? "Stale — tidak ada sealed run" : "—")}
          />
          <DetailRow label="Klasifikasi" value={akrual.klasifikasiSnapshot} />
          <DetailRow label="Jurnal Header" value={akrual.jurnalHeaderId ?? "Belum diposting"} />
          <DetailRow label="Dibuat" value={new Date(akrual.createdAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })} />
        </div>

        {/* Override info */}
        {akrual.overrideComment && (
          <div className="mt-4 rounded-md border border-blue-200 bg-blue-50 p-3 space-y-1">
            <p className="text-xs font-semibold text-blue-800">Override Staging Stale</p>
            <p className="text-xs text-blue-700">{akrual.overrideComment}</p>
            <p className="text-xs text-muted-foreground">By: {akrual.overrideUserId ?? "—"}</p>
          </div>
        )}
      </div>

      {/* Override dialog */}
      {overrideOpen && (
        <AkrualOverrideStaleDialog
          open={overrideOpen}
          onOpenChange={setOverrideOpen}
          akrualId={akrual.id}
          instrumenKode={akrual.instrumenKode}
          stage={akrual.stage as 1 | 2 | 3 | null}
          onSuccess={() => {
            setOverrideOpen(false);
            void queryClient.invalidateQueries({ queryKey: akrualQueryKeys.detail(akrual.id) });
          }}
        />
      )}
    </div>
  );
}
