"use client";

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { RenewalStatusBadge } from "@/components/blips/renewal/RenewalStatusBadge";
import { RenewalSkemaBadge } from "@/components/blips/renewal/RenewalSkemaBadge";
import { RenewalEIRBadge } from "@/components/blips/renewal/RenewalEIRBadge";
import { RenewalPreviewPanel } from "@/components/blips/renewal/RenewalPreviewPanel";
import { RenewalApproveDialog } from "@/components/blips/renewal/RenewalApproveDialog";
import { RenewalRejectDialog } from "@/components/blips/renewal/RenewalRejectDialog";

import { renewalDetailApi, renewalQueryKeys } from "@/lib/api/renewal.api";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// IDR formatter — full precision for detail views
// ---------------------------------------------------------------------------

const IDR_FULL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function formatIDR(value: string | undefined | null): string {
  if (!value) return "—";
  const n = parseFloat(value);
  if (isNaN(n)) return "—";
  return IDR_FULL.format(n);
}

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

function RenewalDetailContent() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const perms = usePermissions();
  const queryClient = useQueryClient();

  const [approveOpen, setApproveOpen] = React.useState(false);
  const [rejectOpen, setRejectOpen] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: renewalQueryKeys.detail(params.id),
    queryFn: () => renewalDetailApi.get(params.id),
    staleTime: 30_000,
  });

  const renewal = data?.data;

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (isError || !renewal) {
    return (
      <div className="container mx-auto py-10 text-center text-muted-foreground">
        <p>Data renewal tidak ditemukan atau terjadi kesalahan.</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={() => router.back()}>
          Kembali
        </Button>
      </div>
    );
  }

  // Persona gating: show approve/reject only for approver + PENDING_APPROVAL + SoD
  const isPending = renewal.status === "PENDING_APPROVAL";
  const canApprove = perms.canApprove("transaksi");
  const isMaker = renewal.makerId === perms.userId;
  const showWorkflowActions = canApprove && isPending && !isMaker;

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/transaksi/renewal" className="hover:underline">
          Renewal Deposito
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{renewal.instrumenLamaKode}</span>
      </nav>

      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href="/transaksi/renewal" aria-label="Kembali ke daftar renewal">
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            </Link>
          </Button>
          <div>
            <h1 className="text-xl font-semibold font-mono">{renewal.instrumenLamaKode}</h1>
            <p className="text-sm text-muted-foreground">ID: {renewal.id}</p>
          </div>
          <RenewalStatusBadge status={renewal.status} />
          <RenewalSkemaBadge skema={renewal.skema} size="sm" />
        </div>

        {/* Workflow action bar — absent from DOM if not eligible */}
        {showWorkflowActions && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => setRejectOpen(true)}
            >
              Tolak
            </Button>
            <Button
              size="sm"
              onClick={() => setApproveOpen(true)}
            >
              Setujui Renewal
            </Button>
          </div>
        )}
      </div>

      <Separator />

      {/* Main info grid */}
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-4">
          <h2 className="text-base font-semibold">Informasi Renewal</h2>
          <dl className="grid grid-cols-2 gap-4">
            <LabelValue label="Instrumen Lama">
              <span className="font-mono">{renewal.instrumenLamaKode}</span>
            </LabelValue>
            <LabelValue label="Instrumen Baru">
              {renewal.instrumenBaruId ? (
                <Link
                  href={`/master/instrumen/${renewal.instrumenBaruId}`}
                  className="text-primary hover:underline inline-flex items-center gap-1 font-mono text-xs"
                  aria-label="Lihat instrumen baru"
                >
                  {renewal.instrumenBaruId.slice(0, 8)}...
                  <ExternalLink className="h-3 w-3" aria-hidden="true" />
                </Link>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </LabelValue>
            <LabelValue label="Tenor Baru">
              <span className="font-mono">{renewal.tenorBaruBulan} bulan</span>
            </LabelValue>
            <LabelValue label="Rate Baru">
              <span className="font-mono">
                {parseFloat(renewal.rateBaruPersen).toFixed(4)}% p.a.
              </span>
            </LabelValue>
            <LabelValue label="Tanggal Efektif">
              <span>{renewal.tanggalEfektifBaru}</span>
            </LabelValue>
            <LabelValue label="Tanggal Jatuh Tempo Baru">
              <span>{renewal.tanggalJatuhTempoBaru ?? "—"}</span>
            </LabelValue>
            <LabelValue label="Pokok Lama">
              <span className="font-mono">{formatIDR(renewal.pokokLama)}</span>
            </LabelValue>
            <LabelValue label="Pokok Baru">
              <span className="font-mono">{formatIDR(renewal.pokokBaru)}</span>
            </LabelValue>
            <LabelValue label="Bunga Bersih">
              <span className="font-mono">{formatIDR(renewal.bungaBersih)}</span>
            </LabelValue>
            {renewal.eirBaru && (
              <LabelValue label="EIR Baru">
                <RenewalEIRBadge eirBaru={renewal.eirBaru} />
              </LabelValue>
            )}
          </dl>

          {/* Reject reason */}
          {renewal.status === "REJECTED" && renewal.rejectReason && (
            <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800 mt-2">
              <strong>Alasan Penolakan:</strong> {renewal.rejectReason}
            </div>
          )}

          {/* Approval note */}
          {renewal.status === "POSTED" && renewal.approveReason && (
            <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-800 mt-2">
              <strong>Komentar Approver:</strong> {renewal.approveReason}
            </div>
          )}
        </div>

        {/* Preview panel */}
        {renewal.preview && (
          <RenewalPreviewPanel
            preview={renewal.preview}
            tanggalJatuhTempoBaru={renewal.tanggalJatuhTempoBaru}
            showSchedule
          />
        )}
      </div>

      {/* Link to jurnal */}
      {renewal.jurnalEntryId && (
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Jurnal RENEWAL_DEPOSITO:</span>
          <Link
            href={`/jurnal/${renewal.jurnalEntryId}`}
            className="text-primary hover:underline inline-flex items-center gap-1"
            aria-label="Lihat jurnal entry"
          >
            <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
            Lihat Jurnal
          </Link>
        </div>
      )}

      {/* Dialogs */}
      <RenewalApproveDialog
        open={approveOpen}
        onOpenChange={setApproveOpen}
        renewalId={renewal.id}
        instrumenKode={renewal.instrumenLamaKode}
        makerId={renewal.makerId}
        onSuccess={() => {
          setApproveOpen(false);
          void queryClient.invalidateQueries({ queryKey: renewalQueryKeys.detail(renewal.id) });
        }}
      />

      <RenewalRejectDialog
        open={rejectOpen}
        onOpenChange={setRejectOpen}
        renewalId={renewal.id}
        instrumenKode={renewal.instrumenLamaKode}
        onSuccess={() => {
          setRejectOpen(false);
          void queryClient.invalidateQueries({ queryKey: renewalQueryKeys.detail(renewal.id) });
        }}
      />
    </div>
  );
}

export default function RenewalDetailPage() {
  return (
    <Suspense>
      <RenewalDetailContent />
    </Suspense>
  );
}
