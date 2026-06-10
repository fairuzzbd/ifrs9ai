"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
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

import { mappingJurnalApi } from "@/lib/api/mapping-jurnal.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type {
  MappingJurnalDetail,
  MappingJurnalDetailItem,
} from "@/lib/schemas/mapping-jurnal.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5">
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

const KATEGORI_LABELS: Record<string, string> = {
  PENEMPATAN: "Penempatan",
  MTM: "Mark-to-Market",
  BUNGA_AKRUAL: "Bunga Akrual",
  BUNGA_TERIMA: "Bunga Diterima",
  JATUH_TEMPO: "Jatuh Tempo",
  PENJUALAN: "Penjualan",
  REKLASIFIKASI: "Reklasifikasi",
  ECL_IMPAIRMENT: "ECL / Impairment",
  FX_REVALUATION: "Revaluasi FX",
  AMORTISASI_EIR: "Amortisasi EIR",
  OTHER: "Lainnya",
};

// ---------------------------------------------------------------------------
// Detail table (read-only)
// ---------------------------------------------------------------------------

function DetailTable({ details }: { details: MappingJurnalDetailItem[] }) {
  if (details.length === 0) {
    return (
      <p className="text-sm text-muted-foreground italic">
        Tidak ada detail jurnal.
      </p>
    );
  }

  // Compute balance
  const debitSum = details
    .filter((d) => d.dkIndicator === "DEBIT")
    .reduce((acc, d) => acc + parseFloat(d.multiplier), 0);
  const kreditSum = details
    .filter((d) => d.dkIndicator === "KREDIT")
    .reduce((acc, d) => acc + parseFloat(d.multiplier), 0);
  const isBalanced = Math.abs(debitSum - kreditSum) < 1e-8;

  return (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="border-b bg-muted/50">
            <tr>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                No.
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                Akun CoA
              </th>
              <th className="px-3 py-2 text-center font-medium text-muted-foreground">
                D/K
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                Sumber Amount
              </th>
              <th className="px-3 py-2 text-right font-medium text-muted-foreground">
                Multiplier
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                Mata Uang
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                Filter Klasifikasi
              </th>
            </tr>
          </thead>
          <tbody>
            {details
              .sort((a, b) => a.urutan - b.urutan)
              .map((d) => (
                <tr key={d.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-3 py-2 text-muted-foreground">
                    {d.urutan}
                  </td>
                  <td className="px-3 py-2">
                    <div>
                      <span className="font-mono text-xs text-muted-foreground">
                        {d.kodeAkunDisplay}
                      </span>
                      {d.namaAkun && (
                        <p className="text-xs text-foreground">{d.namaAkun}</p>
                      )}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-center">
                    <Badge
                      variant="outline"
                      className={cn(
                        "text-xs font-mono",
                        d.dkIndicator === "DEBIT"
                          ? "border-blue-300 bg-blue-50 text-blue-700"
                          : "border-orange-300 bg-orange-50 text-orange-700",
                      )}
                    >
                      {d.dkIndicator === "DEBIT" ? "D" : "K"}
                    </Badge>
                  </td>
                  <td className="px-3 py-2 text-xs text-muted-foreground">
                    {d.sumberAmount.replace(/_/g, " ")}
                  </td>
                  <td className="px-3 py-2 text-right font-mono text-sm">
                    {d.multiplier}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">
                    {d.matauangPosting}
                  </td>
                  <td className="px-3 py-2 text-xs text-muted-foreground">
                    {d.klasifikasiFilter ?? "Semua"}
                  </td>
                </tr>
              ))}
          </tbody>
          <tfoot className="border-t bg-muted/30">
            <tr>
              <td colSpan={4} className="px-3 py-2 text-xs text-muted-foreground text-right">
                Total:
              </td>
              <td className="px-3 py-2">
                <div className="text-right text-xs space-y-0.5">
                  <div className="text-blue-700">
                    D: <span className="font-mono">{debitSum.toFixed(8)}</span>
                  </div>
                  <div className="text-orange-700">
                    K: <span className="font-mono">{kreditSum.toFixed(8)}</span>
                  </div>
                </div>
              </td>
              <td colSpan={2} className="px-3 py-2">
                <span
                  className={cn(
                    "text-xs font-medium",
                    isBalanced ? "text-green-700" : "text-destructive",
                  )}
                >
                  {isBalanced ? "Seimbang" : "Tidak Seimbang"}
                </span>
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function MappingJurnalDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["mapping-jurnal", id],
    queryFn: () => mappingJurnalApi.get(id),
    enabled: !!id,
  });

  const item: MappingJurnalDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["mapping-jurnal", id] });
    void queryClient.invalidateQueries({ queryKey: ["mapping-jurnal"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await mappingJurnalApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Mapping jurnal "${item.namaEvent}" berhasil disubmit untuk review.`,
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
      const res = await mappingJurnalApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Mapping jurnal "${item.namaEvent}" berhasil di-review. Status: ${res.data.currentState}.`,
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
      await mappingJurnalApi.approve(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Mapping jurnal "${item.namaEvent}" berhasil disetujui dan aktif.`,
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
      await mappingJurnalApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `Mapping jurnal "${item.namaEvent}" dikembalikan ke maker.`,
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
      await mappingJurnalApi.delete(id, uuidv4());
      notify.destructive(`Mapping jurnal "${item.namaEvent}" berhasil dihapus.`);
      router.push("/master/mapping-jurnal");
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
  // Loading / error states
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-96" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data mapping jurnal.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/mapping-jurnal">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("mapping_jurnal") && isDraft;
  const canDelete = perms.canDelete("mapping_jurnal") && isDraft;
  const canSubmit = perms.canSubmit("mapping_jurnal") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/mapping-jurnal" className="hover:underline">
          Mapping Jurnal
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.eventCode}</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3 flex-wrap">
          <h1 className="text-2xl font-semibold">{item.namaEvent}</h1>
          <Badge variant="outline" className="text-xs">
            {KATEGORI_LABELS[item.kategoriEvent] ?? item.kategoriEvent}
          </Badge>
          <WorkflowStatusBadge status={item.workflowStatus} />
          {!item.aktifFlag && (
            <Badge
              variant="outline"
              className="text-xs text-muted-foreground border-muted"
            >
              Tidak Aktif
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/mapping-jurnal/${id}/edit`}>
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
                  onClick={() => void handleSubmit()}
                  disabled={workflowSubmitting}
                >
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/mapping-jurnal/${id}/history`}>
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
        {/* Left: detail */}
        <div className="space-y-6">
          {/* Header details */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Header
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Event ID Kode"
                  value={
                    <code className="font-mono font-bold">
                      {item.eventIdKode}
                    </code>
                  }
                />
                <DetailRow
                  label="Event Code"
                  value={
                    <code className="font-mono">{item.eventCode}</code>
                  }
                />
                <DetailRow label="Nama Event" value={item.namaEvent} />
                <DetailRow
                  label="Kategori"
                  value={
                    KATEGORI_LABELS[item.kategoriEvent] ?? item.kategoriEvent
                  }
                />
                <DetailRow
                  label="Trigger Source"
                  value={item.triggerSource.replace(/_/g, " ")}
                />
                <DetailRow
                  label="Status Aktif"
                  value={
                    <span
                      className={
                        item.aktifFlag
                          ? "text-green-700 font-medium"
                          : "text-muted-foreground"
                      }
                    >
                      {item.aktifFlag ? "Ya" : "Tidak"}
                    </span>
                  }
                />
                <div className="col-span-2">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide block mb-1">
                    Tipe Instrumen Berlaku
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {item.tipeInstrumenBerlaku.map((t) => (
                      <Badge key={t} variant="outline" className="text-xs">
                        {t}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div className="col-span-2">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide block mb-1">
                    Klasifikasi Berlaku
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {item.klasifikasiBerlaku.map((k) => (
                      <Badge key={k} variant="outline" className="text-xs">
                        {k}
                      </Badge>
                    ))}
                  </div>
                </div>
                {item.catatan && (
                  <div className="col-span-2">
                    <DetailRow
                      label="Catatan"
                      value={
                        <span className="whitespace-pre-wrap text-sm">
                          {item.catatan}
                        </span>
                      }
                    />
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Detail table */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Jurnal ({item.details.length} baris)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <DetailTable details={item.details} />
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
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/mapping-jurnal/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: 4-eyes workflow panel */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {item.workflow ? (
                <MakerReviewerApproverPanel
                  workflowData={item.workflow}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onSubmit={() => void handleSubmit()}
                  onReview={(c) => void handleReview(c)}
                  onApprove={(c) => void handleApprove(c)}
                  onReject={(c) => void handleReject(c)}
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
                      onClick={() => void handleSubmit()}
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

      {/* Delete dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Mapping Jurnal?</DialogTitle>
            <DialogDescription>
              Mapping jurnal <strong>{item.namaEvent}</strong> ({item.eventCode}
              ) akan dihapus dari sistem (soft-delete).
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
              onClick={() => void handleDelete()}
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
