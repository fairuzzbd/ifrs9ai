"use client";

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, AlertTriangle, ExternalLink } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { MtmStatusBadge } from "@/components/blips/mtm/MtmStatusBadge";
import { MtmDeviationBadge } from "@/components/blips/mtm/MtmDeviationBadge";
import { MtmStaleBadge } from "@/components/blips/mtm/MtmStaleBadge";
import { MtmSourceBadge } from "@/components/blips/mtm/MtmSourceBadge";
import { MtmRoutingBadge } from "@/components/blips/mtm/MtmRoutingBadge";
import { MtmPriceHistoryChart } from "@/components/blips/mtm/MtmPriceHistoryChart";
import { MtmOverrideApproveDialog } from "@/components/blips/mtm/MtmOverrideApproveDialog";
import { MtmOverrideRejectDialog } from "@/components/blips/mtm/MtmOverrideRejectDialog";

import { mtmListApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  MTM_STATUS_LABELS,
  MTM_KLASIFIKASI_LABELS,
  HARGA_SUMBER_LABELS,
} from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 4 });
const IDR_SHORT = new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 0 });

function LabelValue({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detail content
// ---------------------------------------------------------------------------

function MtmDetailContent() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const perms = usePermissions();
  const queryClient = useQueryClient();

  const [approveOpen, setApproveOpen] = React.useState(false);
  const [rejectOpen, setRejectOpen] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: mtmQueryKeys.detail(params.id),
    queryFn: () => mtmListApi.get(params.id),
    staleTime: 30_000,
  });

  const detail = data?.data;

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (isError || !detail) {
    return (
      <div className="container mx-auto py-6 text-center space-y-3">
        <p className="text-muted-foreground">Data MTM tidak ditemukan atau terjadi kesalahan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  const canOverride = perms.can("mtm.override");
  const allowsAction = ["PENDING_REVIEW", "STALE_PRICE"].includes(detail.status) && !detail.lockedFlag;
  const isStaleNoUpload = detail.status === "STALE_PRICE";

  return (
    <div className="container mx-auto py-6 space-y-5">
      {/* Breadcrumb + back */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2"
          onClick={() => router.back()}
          aria-label="Kembali ke daftar MTM"
        >
          <ArrowLeft className="h-3.5 w-3.5" aria-hidden />
          Kembali
        </Button>
        <span>/</span>
        <Link href="/mtm" className="hover:underline">MTM Harian</Link>
        <span>/</span>
        <span className="text-foreground font-medium font-mono truncate max-w-[200px]">{detail.instrumenKode}</span>
      </nav>

      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2 flex-wrap">
            <h1 className="text-2xl font-semibold font-mono">{detail.instrumenKode}</h1>
            <MtmStatusBadge status={detail.status} />
            {detail.deviationFlag && (
              <MtmDeviationBadge deltaPct={detail.deltaPct} thresholdPct={5} />
            )}
          </div>
          <p className="text-muted-foreground mt-0.5">{detail.instrumenNama}</p>
        </div>

        {/* Action buttons */}
        {canOverride && allowsAction && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={isStaleNoUpload}
              title={isStaleNoUpload ? "Upload harga terbaru terlebih dahulu sebelum menyetujui." : undefined}
              onClick={() => setApproveOpen(true)}
            >
              Override: Setuju
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => setRejectOpen(true)}
            >
              Override: Tolak
            </Button>
          </div>
        )}
      </div>

      {/* Deviation warning */}
      {detail.deviationFlag && (
        <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
          <div>
            <p className="font-semibold">Deviasi Harga Signifikan</p>
            <p>
              Delta {detail.deltaPct >= 0 ? "+" : ""}{detail.deltaPct.toFixed(2)}% melebihi threshold. Verifikasi dari sumber primer (IBPA / Bloomberg) diperlukan sebelum approve.
            </p>
          </div>
        </div>
      )}

      {/* Stale warning */}
      {detail.stalePriceFlag && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
          <MtmStaleBadge
            hargaAgeDays={detail.hargaAgeDays}
            stalePriceReason={detail.stalePriceFlag ? "HARGA_TIDAK_TERSEDIA" : "HARGA_TIDAK_TERSEDIA"}
            escalated={detail.hargaAgeDays > 7}
          />
          <p className="text-xs text-amber-700 mt-1">
            Upload harga terbaru via halaman <Link href="/mtm/upload" className="underline">Upload Manual</Link> untuk mengatasi peringatan ini.
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Left: detail info */}
        <div className="lg:col-span-2 space-y-5">
          {/* Core info */}
          <section className="rounded-lg border p-5 space-y-4" aria-labelledby="info-heading">
            <h2 id="info-heading" className="text-base font-semibold">Informasi MTM</h2>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-3">
              <LabelValue label="Tanggal MTM">{detail.tanggalMtm}</LabelValue>
              <LabelValue label="Klasifikasi">
                <MtmRoutingBadge
                  eventCodes={detail.jurnalEventCodes ?? (detail.jurnalEventCode ? [detail.jurnalEventCode] : [])}
                  klasifikasi={detail.klasifikasiSnapshot}
                />
              </LabelValue>
              <LabelValue label="Sumber Harga">
                <MtmSourceBadge source={detail.hargaSumber} />
              </LabelValue>
              <LabelValue label="Harga Pasar (IDR)">{IDR.format(detail.hargaPasarIdr)}</LabelValue>
              {detail.hargaPasarFcy && detail.kursTengah && (
                <>
                  <LabelValue label="Harga Pasar (FCY)">
                    {detail.hargaPasarFcy.toFixed(4)}
                  </LabelValue>
                  <LabelValue label="Kurs Tengah BI">
                    {IDR_SHORT.format(detail.kursTengah)}
                  </LabelValue>
                </>
              )}
              <LabelValue label="Harga Buku (IDR)">{IDR.format(detail.hargaBukuIdr)}</LabelValue>
              <LabelValue label="Delta (IDR)">{IDR.format(detail.deltaIdr)}</LabelValue>
              <LabelValue label="Delta (%)">
                <span className={detail.deltaPct >= 0 ? "text-green-700 font-mono" : "text-red-700 font-mono"}>
                  {detail.deltaPct >= 0 ? "+" : ""}{detail.deltaPct.toFixed(4)}%
                </span>
              </LabelValue>
              <LabelValue label="Umur Harga">{detail.hargaAgeDays} hari</LabelValue>
              <LabelValue label="Tanggal Harga">{detail.hargaTanggal}</LabelValue>
              {detail.treatmentSnapshot && (
                <LabelValue label="Treatment">{detail.treatmentSnapshot}</LabelValue>
              )}
            </dl>
          </section>

          {/* Jurnal info */}
          {(detail.jurnalEntryId || detail.jurnalEventCodes) && (
            <section className="rounded-lg border p-5 space-y-3" aria-labelledby="jurnal-heading">
              <h2 id="jurnal-heading" className="text-base font-semibold">Jurnal</h2>
              <dl className="grid grid-cols-2 gap-x-6 gap-y-3">
                {detail.jurnalEventCodes && (
                  <LabelValue label="Event Code">
                    {detail.jurnalEventCodes.join(", ")}
                  </LabelValue>
                )}
                {detail.jurnalEntryId && (
                  <LabelValue label="Jurnal Entry">
                    <Link
                      href={`/jurnal/${detail.jurnalEntryId}`}
                      className="inline-flex items-center gap-1 text-primary hover:underline text-sm"
                    >
                      Lihat Jurnal <ExternalLink className="h-3.5 w-3.5" aria-hidden />
                    </Link>
                  </LabelValue>
                )}
              </dl>
            </section>
          )}

          {/* Override history */}
          {(detail.overrideApproverId || detail.overrideComment) && (
            <section className="rounded-lg border p-5 space-y-3" aria-labelledby="override-heading">
              <h2 id="override-heading" className="text-base font-semibold">Riwayat Override</h2>
              <dl className="grid grid-cols-2 gap-x-6 gap-y-3">
                <LabelValue label="Status Override">
                  {MTM_STATUS_LABELS[detail.status]}
                </LabelValue>
                {detail.overrideAt && (
                  <LabelValue label="Waktu Override">{detail.overrideAt}</LabelValue>
                )}
                {detail.overrideComment && (
                  <div className="col-span-2">
                    <dt className="text-xs font-medium text-muted-foreground">Komentar</dt>
                    <dd className="text-sm mt-0.5 rounded bg-muted/40 px-3 py-2 whitespace-pre-wrap">{detail.overrideComment}</dd>
                  </div>
                )}
              </dl>
              <p className="text-xs text-muted-foreground">
                SoD: uploader_id ≠ override_approver_id (DEC-017)
              </p>
            </section>
          )}

          {/* Upload batch link */}
          {detail.uploadBatchId && (
            <div className="text-sm">
              <span className="text-muted-foreground">Batch Upload: </span>
              <Link
                href={`/mtm/upload/batch/${detail.uploadBatchId}`}
                className="inline-flex items-center gap-1 text-primary hover:underline"
              >
                Lihat Detail Batch <ExternalLink className="h-3.5 w-3.5" aria-hidden />
              </Link>
            </div>
          )}

          {/* Price history chart */}
          <section className="rounded-lg border p-5 space-y-3" aria-labelledby="chart-heading">
            <h2 id="chart-heading" className="text-base font-semibold">Riwayat Harga (30 Hari)</h2>
            <MtmPriceHistoryChart
              instrumenId={detail.instrumenId}
              instrumenKode={detail.instrumenKode}
              tanggalMtm={detail.tanggalMtm}
            />
          </section>
        </div>

        {/* Right: metadata */}
        <div className="space-y-5">
          <section className="rounded-lg border p-5 space-y-3" aria-labelledby="meta-heading">
            <h2 id="meta-heading" className="text-base font-semibold">Metadata</h2>
            <dl className="space-y-3">
              <LabelValue label="ID">{detail.id}</LabelValue>
              <LabelValue label="Instrumen ID">{detail.instrumenId}</LabelValue>
              <LabelValue label="Periode">{detail.periodeBulananId}</LabelValue>
              <Separator />
              <LabelValue label="Dibuat">{detail.createdAt}</LabelValue>
              <LabelValue label="Diperbarui">{detail.updatedAt}</LabelValue>
              <LabelValue label="Row Version">{detail.rowVersion}</LabelValue>
              <Separator />
              <LabelValue label="Locked">
                {detail.lockedFlag ? (
                  <span className="text-amber-700 text-xs font-medium">Ya — Periode Hard-Closed</span>
                ) : (
                  <span className="text-muted-foreground text-xs">Tidak</span>
                )}
              </LabelValue>
              {detail.cronJobId && (
                <LabelValue label="Cron Job">{detail.cronJobId}</LabelValue>
              )}
            </dl>
          </section>
        </div>
      </div>

      {/* Override dialogs */}
      <MtmOverrideApproveDialog
        open={approveOpen}
        onOpenChange={setApproveOpen}
        mtmId={detail.id}
        instrumenKode={detail.instrumenKode}
        tanggalMtm={detail.tanggalMtm}
        deviationFlag={detail.deviationFlag}
        deltaPct={detail.deltaPct}
        thresholdPct={5}
        onSuccess={() => {
          void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.detail(detail.id) });
          void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
        }}
      />
      <MtmOverrideRejectDialog
        open={rejectOpen}
        onOpenChange={setRejectOpen}
        mtmId={detail.id}
        instrumenKode={detail.instrumenKode}
        tanggalMtm={detail.tanggalMtm}
        onSuccess={() => {
          void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.detail(detail.id) });
          void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
        }}
      />
    </div>
  );
}

export default function MtmDetailPage() {
  return (
    <Suspense>
      <MtmDetailContent />
    </Suspense>
  );
}
