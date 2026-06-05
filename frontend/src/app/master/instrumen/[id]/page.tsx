"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, Lock } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { MakerReviewerApproverPanel } from "@/components/blips/MakerReviewerApproverPanel";

import { instrumenApi } from "@/lib/api/instrumen.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { InstrumenDetail } from "@/lib/schemas/instrumen.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailRow({
  label,
  value,
  className,
}: {
  label: string;
  value: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-0.5", className)}>
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
        {label}
      </span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

function formatDt(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return format(parseISO(iso), "dd MMM yyyy, HH:mm 'WIB'");
  } catch {
    return iso;
  }
}

const idr = (s: string | null | undefined) => {
  if (!s) return "—";
  const n = parseFloat(s);
  if (isNaN(n)) return s;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 4,
  }).format(n);
};

const TIPE_LABELS: Record<string, string> = {
  DEPOSITO: "Deposito",
  OBLIGASI: "Obligasi",
  SAHAM: "Saham",
  REKSADANA: "Reksa Dana",
  SBN: "Surat Berharga Negara",
  SPN: "Surat Perbendaharaan Negara",
  SUKUK: "Sukuk",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function InstrumenDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["instrumen", id],
    queryFn: () => instrumenApi.get(id),
    enabled: !!id,
  });

  const item: InstrumenDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["instrumen", id] });
    void queryClient.invalidateQueries({ queryKey: ["instrumen"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await instrumenApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Instrumen ${item.kodeInstrumen} berhasil disubmit untuk review. Menunggu Treasury Approver.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReview = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await instrumenApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Instrumen ${item.kodeInstrumen} berhasil di-review. Status: ${res.data.currentState}.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleApprove = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await instrumenApi.approve(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Instrumen ${item.kodeInstrumen} berhasil disetujui. Instrumen aktif dan siap untuk ECL.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReject = async (comment: string) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await instrumenApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `Instrumen ${item.kodeInstrumen} dikembalikan ke maker dengan alasan penolakan.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!item) return;
    setDeleting(true);
    try {
      await instrumenApi.delete(id, uuidv4());
      notify.destructive(`Instrumen ${item.kodeInstrumen} berhasil dihapus.`);
      router.push("/master/instrumen");
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setDeleting(false);
      setDeleteOpen(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Loading / error
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-80 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data instrumen {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/instrumen">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("instrumen") && isDraft;
  const canDelete = perms.canDelete("instrumen") && isDraft;
  const canSubmit = perms.canSubmit("instrumen") && isDraft;
  const isKlasifikasiLocked = !!item.klasifikasiLockedAt;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/instrumen" className="hover:underline">
          Instrumen
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">
          {item.kodeInstrumen}
        </span>
      </nav>

      {/* Klasifikasi locked banner */}
      {isKlasifikasiLocked && item.klasifikasiLockedAt && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4"
        >
          <Lock className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
          <div className="text-sm text-amber-800">
            <p className="font-semibold">Klasifikasi terkunci</p>
            <p>
              Terkunci sejak{" "}
              <span className="font-medium">
                {formatDt(item.klasifikasiLockedAt)}
              </span>
              . Edit klasifikasi via workflow SPPI/BM (Phase 4).
            </p>
          </div>
        </div>
      )}

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            {item.nama}{" "}
            <span className="text-muted-foreground">
              ({item.kodeInstrumen})
            </span>
          </h1>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/instrumen/${id}/edit`}>
                <Pencil className="mr-1.5 h-4 w-4" aria-hidden />
                Edit
              </Link>
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="h-9 w-9"
                aria-label="Aksi lainnya"
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canSubmit && (
                <DropdownMenuItem
                  disabled={workflowSubmitting}
                  onClick={handleSubmit}
                >
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/instrumen/${id}/history`}>
                  Lihat Riwayat Audit
                </Link>
              </DropdownMenuItem>
              {canDelete && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => setDeleteOpen(true)}
                  >
                    Hapus
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Body: 2-col layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        {/* Left: detail cards */}
        <div className="space-y-6">
          {/* Identitas */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Identitas Instrumen
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Kode"
                  value={
                    <code className="font-mono font-bold">
                      {item.kodeInstrumen}
                    </code>
                  }
                />
                <DetailRow label="Tipe" value={TIPE_LABELS[item.tipeInstrumen] ?? item.tipeInstrumen} />
                <DetailRow
                  label="Nama"
                  value={item.nama}
                  className="col-span-2"
                />
                <DetailRow label="Sub Tipe" value={item.subTipe || "—"} />
                <DetailRow label="ISIN" value={item.isin ?? "—"} />
              </div>
            </CardContent>
          </Card>

          {/* Counterparty & Kustodian */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Counterparty &amp; Kustodian
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Counterparty ID"
                  value={
                    <code className="text-xs font-mono">{item.counterpartyId}</code>
                  }
                />
                <DetailRow
                  label="Portofolio ID"
                  value={
                    <code className="text-xs font-mono">{item.portofolioId}</code>
                  }
                />
                <DetailRow label="Mata Uang" value={item.mataUang} />
                {item.manajerInvestasiId && (
                  <DetailRow
                    label="Manajer Investasi"
                    value={
                      <code className="text-xs font-mono">
                        {item.manajerInvestasiId}
                      </code>
                    }
                  />
                )}
                {item.bankKustodianId && (
                  <DetailRow
                    label="Bank Kustodian"
                    value={
                      <code className="text-xs font-mono">
                        {item.bankKustodianId}
                      </code>
                    }
                  />
                )}
              </div>
            </CardContent>
          </Card>

          {/* Finansial */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Periode &amp; Finansial
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Nominal"
                  value={
                    <span className="font-mono tabular-nums">
                      {idr(item.nominal)}
                    </span>
                  }
                />
                <DetailRow
                  label="Jumlah Lot"
                  value={item.jumlahLot ?? "—"}
                />
                <DetailRow
                  label="Tgl Penempatan"
                  value={item.tanggalPenempatan}
                />
                <DetailRow
                  label="Tgl Jatuh Tempo"
                  value={item.tanggalJatuhTempo ?? "—"}
                />
                {item.kupon !== null && (
                  <DetailRow
                    label="Kupon"
                    value={
                      <span className="font-mono">
                        {item.kupon}%
                      </span>
                    }
                  />
                )}
                {item.frekuensiBunga && (
                  <DetailRow
                    label="Frekuensi Bunga"
                    value={item.frekuensiBunga.replace(/_/g, " ")}
                  />
                )}
                <DetailRow
                  label="Auto Renewal"
                  value={item.autoRenewalFlag ? "Ya" : "Tidak"}
                />
                <DetailRow
                  label="Status"
                  value={
                    <span
                      className={cn(
                        "text-sm font-medium",
                        item.status === "AKTIF"
                          ? "text-green-700"
                          : "text-muted-foreground",
                      )}
                    >
                      {item.status.replace(/_/g, " ")}
                    </span>
                  }
                />
              </div>
            </CardContent>
          </Card>

          {/* Klasifikasi PSAK 71 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center gap-2">
                <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                  Klasifikasi PSAK 71
                </CardTitle>
                {isKlasifikasiLocked && (
                  <span className="flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                    <Lock className="h-3 w-3" aria-hidden />
                    Terkunci
                  </span>
                )}
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Klasifikasi PSAK 71"
                  value={
                    item.klasifikasiPsak71 ? (
                      <span className="font-mono font-bold">
                        {item.klasifikasiPsak71}
                      </span>
                    ) : (
                      <span className="text-muted-foreground italic text-xs">
                        Belum ditetapkan (Phase 4)
                      </span>
                    )
                  }
                />
                <DetailRow
                  label="BM Category"
                  value={item.bmCategory?.replace(/_/g, "&") ?? "—"}
                />
                <DetailRow
                  label="SPPI Result"
                  value={item.sppiResult ?? "Belum diuji"}
                />
                <DetailRow
                  label="FVOCI Election"
                  value={item.fvociElection ? "Ya (irrevocable)" : "Tidak"}
                />
                {item.klasifikasiLockedAt && (
                  <DetailRow
                    label="Terkunci Sejak"
                    value={formatDt(item.klasifikasiLockedAt)}
                    className="col-span-2"
                  />
                )}
              </div>
            </CardContent>
          </Card>

          {/* EIR */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                EIR &amp; Amortisasi
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="EIR Awal"
                  value={
                    item.eirAwal ? (
                      <span className="font-mono">
                        {(parseFloat(item.eirAwal) * 100).toFixed(4)}%
                      </span>
                    ) : (
                      "—"
                    )
                  }
                />
                <DetailRow
                  label="Tgl EIR Computed"
                  value={item.tanggalEirComputed ?? "—"}
                />
                <DetailRow
                  label="Premium/Diskonto"
                  value={
                    <span className="font-mono tabular-nums">
                      {idr(item.premiumDiskonto)}
                    </span>
                  }
                />
                <DetailRow
                  label="Biaya Transaksi Capitalized"
                  value={
                    <span className="font-mono tabular-nums">
                      {idr(item.biayaTransaksi)}
                    </span>
                  }
                />
                <DetailRow
                  label="EIR Method"
                  value={item.eirMethodFlag ? "Ya" : "Tidak"}
                />
                <DetailRow
                  label="Day Count Convention"
                  value={item.dayCountConvention || "—"}
                />
              </div>
            </CardContent>
          </Card>

          {/* Metadata */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Metadata
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Dibuat oleh" value={item.createdBy} />
                <DetailRow
                  label="Dibuat pada"
                  value={formatDt(item.createdAt)}
                />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy ?? "—"} />
                <DetailRow
                  label="Diperbarui pada"
                  value={formatDt(item.updatedAt)}
                />
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/instrumen/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: workflow panel */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {item.workflow ? (
                <MakerReviewerApproverPanel
                  workflowData={item.workflow}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onSubmit={handleSubmit}
                  onReview={handleReview}
                  onApprove={handleApprove}
                  onReject={handleReject}
                />
              ) : (
                <div className="space-y-2">
                  <p className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    Proses Persetujuan
                  </p>
                  <Separator />
                  <p className="text-sm text-muted-foreground">
                    Data belum disubmit ke workflow.
                  </p>
                  {canSubmit && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={workflowSubmitting}
                      onClick={handleSubmit}
                    >
                      {workflowSubmitting
                        ? "Memproses..."
                        : "Kirim untuk Review"}
                    </Button>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Instrumen {item.kodeInstrumen}?</DialogTitle>
            <DialogDescription>
              Instrumen <strong>{item.nama}</strong> ({item.kodeInstrumen}) akan
              dihapus dari sistem (soft-delete).
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(false)}
              disabled={deleting}
            >
              Batal
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleting}
            >
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
